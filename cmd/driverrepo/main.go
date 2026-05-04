package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	lua "github.com/yuin/gopher-lua"
)

const (
	defaultDriversDir = "drivers"
	defaultManifest   = "manifest.json"
	defaultRepository = "https://github.com/srcfl/hugin-drivers"
	defaultRawBaseURL = "https://raw.githubusercontent.com/srcfl/hugin-drivers/main"
)

type manifest struct {
	SchemaVersion int              `json:"schema_version"`
	Repository    string           `json:"repository"`
	Commit        string           `json:"commit"`
	GeneratedAt   string           `json:"generated_at"`
	Drivers       []manifestDriver `json:"drivers"`
}

type manifestDriver struct {
	ID       string         `json:"id"`
	Path     string         `json:"path"`
	Filename string         `json:"filename"`
	Version  string         `json:"version"`
	SHA256   string         `json:"sha256"`
	URL      string         `json:"url"`
	Metadata driverMetadata `json:"metadata"`
}

type driverMetadata struct {
	ID                 string         `json:"-"`
	Version            string         `json:"-"`
	Name               string         `json:"name"`
	Manufacturer       string         `json:"manufacturer,omitempty"`
	Protocols          []string       `json:"protocols,omitempty"`
	Capabilities       []string       `json:"capabilities,omitempty"`
	Description        string         `json:"description,omitempty"`
	Homepage           string         `json:"homepage,omitempty"`
	ConnectionDefaults map[string]any `json:"connection_defaults,omitempty"`
	VerificationStatus string         `json:"verification_status,omitempty"`
	VerifiedBy         []string       `json:"verified_by,omitempty"`
	VerifiedAt         string         `json:"verified_at,omitempty"`
	VerificationNotes  string         `json:"verification_notes,omitempty"`
	TestedModels       []string       `json:"tested_models,omitempty"`
	ConfigSecrets      []string       `json:"config_secrets,omitempty"`
}

type driverSource struct {
	path     string
	relPath  string
	filename string
	body     []byte
	hash     string
	meta     driverMetadata
	version  string
	id       string
}

func main() {
	driversDir := flag.String("drivers", defaultDriversDir, "directory containing Lua drivers")
	manifestPath := flag.String("manifest", defaultManifest, "manifest JSON path")
	writeManifest := flag.Bool("write-manifest", false, "write manifest JSON")
	verify := flag.Bool("verify", false, "verify drivers and manifest JSON")
	repository := flag.String("repository", defaultRepository, "repository URL for generated manifest")
	rawBaseURL := flag.String("raw-base-url", defaultRawBaseURL, "raw content base URL for generated manifest")
	flag.Parse()

	if !*writeManifest && !*verify {
		fmt.Fprintln(os.Stderr, "usage: go run ./cmd/driverrepo -write-manifest|-verify")
		os.Exit(2)
	}

	drivers, err := loadDrivers(*driversDir)
	if err != nil {
		fatal(err)
	}
	if err := verifyDrivers(drivers); err != nil {
		fatal(err)
	}

	if *writeManifest {
		m := buildManifest(drivers, *repository, gitCommit(), time.Now().UTC().Format(time.RFC3339), *rawBaseURL)
		data, err := marshalManifest(m)
		if err != nil {
			fatal(err)
		}
		if err := os.WriteFile(*manifestPath, data, 0644); err != nil {
			fatal(fmt.Errorf("write manifest: %w", err))
		}
	}

	if *verify {
		if err := verifyManifest(*manifestPath, drivers, *repository, *rawBaseURL); err != nil {
			fatal(err)
		}
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}

func loadDrivers(dir string) ([]driverSource, error) {
	var out []driverSource
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() || !strings.HasSuffix(path, ".lua") {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}
		rel, err := filepath.Rel(".", path)
		if err != nil {
			return fmt.Errorf("rel %s: %w", path, err)
		}
		rel = filepath.ToSlash(rel)
		meta := parseMetadata(string(body))
		sum := sha256.Sum256(body)
		out = append(out, driverSource{
			path:     path,
			relPath:  rel,
			filename: filepath.Base(path),
			body:     body,
			hash:     hex.EncodeToString(sum[:]),
			meta:     meta,
			version:  meta.Version,
			id:       meta.ID,
		})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk %s: %w", dir, err)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].relPath < out[j].relPath
	})
	return out, nil
}

func verifyDrivers(drivers []driverSource) error {
	if len(drivers) == 0 {
		return errors.New("no drivers found")
	}
	ids := map[string]string{}
	var errs []string
	for _, d := range drivers {
		if d.id == "" {
			errs = append(errs, fmt.Sprintf("%s: missing DRIVER.id", d.relPath))
		}
		if prev := ids[d.id]; d.id != "" && prev != "" {
			errs = append(errs, fmt.Sprintf("%s: duplicate DRIVER.id %q also used by %s", d.relPath, d.id, prev))
		}
		ids[d.id] = d.relPath
		if d.meta.Name == "" {
			errs = append(errs, fmt.Sprintf("%s: missing DRIVER.name", d.relPath))
		}
		if d.version == "" || !semverRE.MatchString(d.version) {
			errs = append(errs, fmt.Sprintf("%s: version %q is not semver", d.relPath, d.version))
		}
		if len(d.meta.Protocols) == 0 {
			errs = append(errs, fmt.Sprintf("%s: missing DRIVER.protocols", d.relPath))
		}
		if len(d.meta.Capabilities) == 0 {
			errs = append(errs, fmt.Sprintf("%s: missing DRIVER.capabilities", d.relPath))
		}
		if d.meta.VerificationStatus == "" {
			errs = append(errs, fmt.Sprintf("%s: missing DRIVER.verification_status", d.relPath))
		}
		if normalizeVerificationStatus(d.meta.VerificationStatus) != d.meta.VerificationStatus {
			errs = append(errs, fmt.Sprintf("%s: invalid verification_status %q", d.relPath, d.meta.VerificationStatus))
		}
		if d.meta.VerificationStatus == "production" {
			if len(d.meta.VerifiedBy) == 0 {
				errs = append(errs, fmt.Sprintf("%s: production driver missing verified_by", d.relPath))
			}
			if d.meta.VerifiedAt == "" {
				errs = append(errs, fmt.Sprintf("%s: production driver missing verified_at", d.relPath))
			}
		}
		if !safeDriverPath(d.relPath) {
			errs = append(errs, fmt.Sprintf("%s: unsafe driver path", d.relPath))
		}
		if !documentsSignConvention(string(d.body)) {
			errs = append(errs, fmt.Sprintf("%s: missing site sign convention note", d.relPath))
		}
		if err := loadWithGopherLua(d.path); err != nil {
			errs = append(errs, fmt.Sprintf("%s: gopher-lua load failed: %v", d.relPath, err))
		}
	}
	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "\n"))
	}
	return nil
}

func verifyManifest(path string, drivers []driverSource, repository, rawBaseURL string) error {
	body, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read manifest: %w", err)
	}
	var got manifest
	if err := json.Unmarshal(body, &got); err != nil {
		return fmt.Errorf("parse manifest: %w", err)
	}
	if got.SchemaVersion != 1 {
		return fmt.Errorf("manifest schema_version=%d, want 1", got.SchemaVersion)
	}
	if got.Repository != repository {
		return fmt.Errorf("manifest repository=%q, want %q", got.Repository, repository)
	}
	if got.GeneratedAt == "" {
		return errors.New("manifest generated_at is required")
	}
	if got.Commit == "" {
		return errors.New("manifest commit is required")
	}
	want := buildManifest(drivers, repository, got.Commit, got.GeneratedAt, rawBaseURL)
	wantBytes, err := marshalManifest(want)
	if err != nil {
		return err
	}
	gotBytes, err := marshalManifest(got)
	if err != nil {
		return err
	}
	if !bytes.Equal(gotBytes, wantBytes) {
		return errors.New("manifest.json is out of date; run `go run ./cmd/driverrepo -write-manifest`")
	}
	return nil
}

func buildManifest(drivers []driverSource, repository, commit, generatedAt, rawBaseURL string) manifest {
	m := manifest{
		SchemaVersion: 1,
		Repository:    repository,
		Commit:        commit,
		GeneratedAt:   generatedAt,
		Drivers:       make([]manifestDriver, 0, len(drivers)),
	}
	rawBaseURL = strings.TrimRight(rawBaseURL, "/")
	for _, d := range drivers {
		m.Drivers = append(m.Drivers, manifestDriver{
			ID:       d.id,
			Path:     d.relPath,
			Filename: d.filename,
			Version:  d.version,
			SHA256:   d.hash,
			URL:      rawBaseURL + "/" + d.relPath,
			Metadata: d.meta,
		})
	}
	return m
}

func marshalManifest(m manifest) ([]byte, error) {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

func loadWithGopherLua(path string) error {
	L := lua.NewState()
	defer L.Close()
	L.SetGlobal("host", dummyHost(L))
	return L.DoFile(path)
}

func dummyHost(L *lua.LState) *lua.LTable {
	t := L.NewTable()
	noOp := L.NewFunction(func(L *lua.LState) int { return 0 })
	for _, name := range []string{
		"log", "millis", "set_poll_interval", "set_make", "set_sn",
		"emit", "emit_metric", "mqtt_subscribe", "mqtt_publish",
		"mqtt_messages", "modbus_read", "modbus_write", "modbus_write_multi",
		"http_get", "http_post", "decode_u32_le", "decode_u32_be",
		"decode_i32_le", "decode_i32_be", "decode_i16", "json_encode",
		"json_decode",
	} {
		t.RawSetString(name, noOp)
	}
	return t
}

func gitCommit() string {
	out, err := exec.Command("git", "rev-parse", "HEAD").Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(out))
}

func safeDriverPath(p string) bool {
	if p == "" || filepath.IsAbs(p) || strings.Contains(p, "\\") {
		return false
	}
	clean := filepath.Clean(filepath.FromSlash(p))
	return filepath.ToSlash(clean) == p &&
		strings.HasPrefix(p, "drivers/") &&
		!strings.Contains(p, "../") &&
		!strings.HasPrefix(p, "..")
}

func documentsSignConvention(src string) bool {
	s := strings.ToLower(src)
	return strings.Contains(s, "site convention") ||
		strings.Contains(s, "sign convention") ||
		strings.Contains(s, "ems convention") ||
		strings.Contains(s, "positive = import") ||
		strings.Contains(s, "positive w =")
}

func parseMetadata(src string) driverMetadata {
	block := extractDriverBlock(src)
	return driverMetadata{
		ID:                 pickString(block, "id"),
		Version:            pickString(block, "version"),
		Name:               pickString(block, "name"),
		Manufacturer:       pickString(block, "manufacturer"),
		Protocols:          pickList(block, "protocols"),
		Capabilities:       pickList(block, "capabilities"),
		Description:        pickString(block, "description"),
		Homepage:           pickString(block, "homepage"),
		ConnectionDefaults: pickKVBlock(block, "connection_defaults"),
		VerificationStatus: pickString(block, "verification_status"),
		VerifiedBy:         pickList(block, "verified_by"),
		VerifiedAt:         pickString(block, "verified_at"),
		VerificationNotes:  pickString(block, "verification_notes"),
		TestedModels:       pickList(block, "tested_models"),
		ConfigSecrets:      pickList(block, "config_secrets"),
	}
}

var driverBlockRE = regexp.MustCompile(`(?s)DRIVER\s*=\s*\{(.*?)\n\}`)
var stringFieldRE = regexp.MustCompile(`(?m)^\s*%s\s*=\s*"([^"]*)"`)
var listItemRE = regexp.MustCompile(`"([^"]+)"`)
var kvPairRE = regexp.MustCompile(`(\w+)\s*=\s*(?:"([^"]*)"|([^\s,]+))`)
var semverRE = regexp.MustCompile(`^\d+\.\d+\.\d+(?:[-+][0-9A-Za-z.-]+)?$`)

func extractDriverBlock(src string) string {
	m := driverBlockRE.FindStringSubmatch(src)
	if len(m) < 2 {
		return ""
	}
	return m[1]
}

func pickString(block, name string) string {
	re := regexp.MustCompile(fmt.Sprintf(stringFieldRE.String(), regexp.QuoteMeta(name)))
	m := re.FindStringSubmatch(block)
	if len(m) < 2 {
		return ""
	}
	return m[1]
}

func pickList(block, name string) []string {
	re := regexp.MustCompile(`(?m)^\s*` + regexp.QuoteMeta(name) + `\s*=\s*\{([^}]*)\}`)
	m := re.FindStringSubmatch(block)
	if len(m) < 2 {
		return nil
	}
	items := listItemRE.FindAllStringSubmatch(m[1], -1)
	out := make([]string, 0, len(items))
	for _, it := range items {
		out = append(out, it[1])
	}
	return out
}

func pickKVBlock(block, name string) map[string]any {
	re := regexp.MustCompile(`(?s)` + regexp.QuoteMeta(name) + `\s*=\s*\{([^}]*)\}`)
	m := re.FindStringSubmatch(block)
	if len(m) < 2 {
		return nil
	}
	pairs := kvPairRE.FindAllStringSubmatch(m[1], -1)
	if len(pairs) == 0 {
		return nil
	}
	out := make(map[string]any, len(pairs))
	for _, p := range pairs {
		if p[2] != "" {
			out[p[1]] = p[2]
		} else if f, err := strconv.ParseFloat(p[3], 64); err == nil {
			if f == float64(int64(f)) {
				out[p[1]] = int64(f)
			} else {
				out[p[1]] = f
			}
		} else {
			out[p[1]] = p[3]
		}
	}
	return out
}

func normalizeVerificationStatus(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "production":
		return "production"
	case "beta":
		return "beta"
	case "experimental", "":
		return "experimental"
	default:
		return "experimental"
	}
}

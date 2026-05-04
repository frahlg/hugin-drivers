package main

import "testing"

func TestSafeDriverPath(t *testing.T) {
	cases := map[string]bool{
		"drivers/sungrow.lua":        true,
		"drivers/nested/example.lua": true,
		"drivers/../secrets.lua":     false,
		"../drivers/sungrow.lua":     false,
		"/tmp/drivers/sungrow.lua":   false,
		`drivers\windows-path.lua`:   false,
		"custom/sungrow.lua":         false,
		"":                           false,
	}
	for path, want := range cases {
		if got := safeDriverPath(path); got != want {
			t.Fatalf("safeDriverPath(%q) = %v, want %v", path, got, want)
		}
	}
}

func TestPickKVBlockParsesNumbers(t *testing.T) {
	block := `
connection_defaults = {
  host = "192.168.1.10",
  port = 502,
  unit_id = 1,
  scale = 0.1,
}
`
	got := pickKVBlock(block, "connection_defaults")
	if got["host"] != "192.168.1.10" {
		t.Fatalf("host = %#v", got["host"])
	}
	if got["port"] != int64(502) {
		t.Fatalf("port = %#v, want int64(502)", got["port"])
	}
	if got["unit_id"] != int64(1) {
		t.Fatalf("unit_id = %#v, want int64(1)", got["unit_id"])
	}
	if got["scale"] != 0.1 {
		t.Fatalf("scale = %#v, want 0.1", got["scale"])
	}
}

func TestNormalizeVerificationStatus(t *testing.T) {
	cases := map[string]string{
		"production":   "production",
		"Beta":         "beta",
		"experimental": "experimental",
		"":             "experimental",
		"prod":         "experimental",
	}
	for in, want := range cases {
		if got := normalizeVerificationStatus(in); got != want {
			t.Fatalf("normalizeVerificationStatus(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestDocumentsSignConvention(t *testing.T) {
	if !documentsSignConvention("-- Site convention: positive W = import") {
		t.Fatal("expected site convention note to be accepted")
	}
	if documentsSignConvention("-- ordinary driver comment") {
		t.Fatal("expected unrelated comment to be rejected")
	}
}

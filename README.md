# hugin-drivers

Open library of Lua drivers for [forty-two-watts](https://github.com/frahlg/forty-two-watts)
and other open Energy Management Systems.

This repository is the canonical, public driver registry for the Hugin
ecosystem. Every device that speaks Modbus, MQTT, or HTTP should have a
working driver here — and once it does, it works for everyone, forever.

## Why this exists

Energy data should be open. Every closed inverter API, every undocumented
Modbus map, every "call our partner integration team" gate is friction
between people and their own kilowatt-hours. We want the integration layer
to be a public commons:

- **Every device, one driver.** Once written, it works for anyone running
  forty-two-watts (or any other EMS that consumes this library).
- **The library compounds.** Every accepted driver becomes reference material
  for the AI to draft the next one — patterns, register maps, sign
  conventions all carry forward.
- **No vendor lock-in.** MIT-licensed, no CLA, no contributor agreement.
  Drivers belong to the people who write them and the systems that consume
  them.

## Driver format

Each driver is a single `.lua` file in `drivers/` matching the
forty-two-watts driver contract:

- A top-level `DRIVER = { ... }` metadata table (`id`, `name`,
  `manufacturer`, `version`, `protocols`, `capabilities`,
  `tested_models`, `verification_status`, `connection_defaults`).
- A `PROTOCOL` constant: `"modbus"`, `"mqtt"`, or `"http"`.
- Lifecycle functions: `driver_init(config)`, `driver_poll()`,
  `driver_command(action, power_w, cmd)`, `driver_default_mode()`,
  `driver_cleanup()`.
- Telemetry emitted via `host.emit("meter"|"pv"|"battery"|"ev", { ... })`.

Reference implementation and host API:
[`go/internal/drivers/lua.go`](https://github.com/frahlg/forty-two-watts/blob/main/go/internal/drivers/lua.go)
in the forty-two-watts repository.

**Sign convention** (don't break this — every consumer of this library
relies on it):

- Positive watts = flowing **into** the site at the grid meter boundary.
- Grid import is positive, export is negative.
- PV generation is always negative.
- Battery: charge is positive, discharge is negative.
- The driver is the **only** layer that flips signs.

## How to contribute

There are two paths. Both end with a PR landing here.

### 1. Use Hugin (recommended)

[hugin.sourceful-labs.net](https://hugin.sourceful-labs.net) is a workbench
that scans your hardware, identifies the device, drafts a driver with AI
using this library as reference, and validates it against live registers
before you submit. You get a working driver in minutes instead of days, and
the PR opens in your own GitHub name.

### 2. Open a PR directly

Drop a `.lua` file in `drivers/` matching the schema above and open a pull
request. Include `tested_models` so the next contributor knows what works.

Before opening the PR, run the same verifier CI runs:

```bash
go run ./cmd/driverrepo -verify
```

When adding or changing a driver, regenerate the static manifest:

```bash
go run ./cmd/driverrepo -write-manifest
```

The verifier checks that every driver has parseable metadata, a unique
ID, a semver version, a documented site sign convention, and that the
file loads in gopher-lua. It also checks that `manifest.json` matches
the driver files and their SHA-256 hashes.

## Driver discovery

Drivers in this repo are exposed through the static manifest:

```
https://raw.githubusercontent.com/srcfl/hugin-drivers/main/manifest.json
```

forty-two-watts and other EMS clients can consume the catalog directly or
vendor specific drivers as needed.

## License

MIT — see [LICENSE](LICENSE).

## Attribution

This library was seeded from
[`frahlg/forty-two-watts/drivers/`](https://github.com/frahlg/forty-two-watts/tree/main/drivers),
which is itself MIT-licensed. The original drivers — and many of the
patterns the rest of the library follows — are the work of
[@frahlg](https://github.com/frahlg) and the forty-two-watts community.
Every contributor going forward retains credit on their commits.

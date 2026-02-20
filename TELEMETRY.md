# Telemetry

Faucet collects **anonymous** usage statistics to help us understand how the
software is used and where to focus development effort. Telemetry is enabled by
default and can be disabled at any time with zero impact on functionality.

## What we collect

Every hour (and once at startup), Faucet sends a single small JSON payload:

| Field | Example | Why |
|-------|---------|-----|
| `instance_id` | `a1b2c3d4-...` | Random UUID generated on first run. Not tied to hardware, IP, or identity. |
| `version` | `0.1.2` | Faucet version — helps us track upgrade adoption. |
| `go_version` | `go1.25.6` | Go runtime version — informs build and compatibility decisions. |
| `os` | `linux` | Operating system. |
| `arch` | `amd64` | CPU architecture. |
| `db_types` | `["postgres","mysql"]` | Which database drivers are in use (not connection strings or credentials). |
| `service_count` | `3` | Number of connected database services. |
| `table_count` | `42` | Total number of tables across all services. |
| `admin_count` | `1` | Number of admin accounts configured. |
| `api_key_count` | `5` | Number of API keys created. |
| `role_count` | `2` | Number of RBAC roles configured. |
| `features` | `["ui","rbac"]` | Which optional features are active. |
| `uptime_hours` | `168.5` | How long the instance has been running. |

## What we do NOT collect

- IP addresses or hostnames
- Database connection strings, credentials, or DSNs
- Table names, column names, or row data
- Query content or API request/response bodies
- Personally identifiable information of any kind

## Where data is sent

Telemetry is sent to [PostHog](https://posthog.com) (US cloud) via HTTPS POST
to `https://us.i.posthog.com/capture/`. PostHog is an open-source product
analytics platform.

## How to disable

**Option 1 — CLI:**

```bash
faucet config set telemetry.enabled false
```

**Option 2 — Environment variable:**

```bash
export FAUCET_TELEMETRY=0
```

**Option 3 — Block network access:**

The telemetry client has a 3-second timeout and fails silently. Blocking
`us.i.posthog.com` at the firewall or DNS level will also prevent any data
from being sent, with no impact on Faucet's operation.

## Building from source

If you build Faucet from source (`go build ./cmd/faucet`), telemetry is
**automatically disabled** because the PostHog API key is only injected into
official release binaries at build time via `-ldflags`. There is nothing to
configure — source builds simply skip telemetry entirely.

## Implementation

The telemetry implementation is fully open source at
[`internal/telemetry/telemetry.go`](internal/telemetry/telemetry.go). It uses
a raw HTTP POST (no SDK), runs in a background goroutine, and never blocks
request handling.

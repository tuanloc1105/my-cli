# External Integrations

**Analysis Date:** 2026-03-12

## APIs & External Services

**HTTP APIs:**
- Generic HTTP endpoints - Used by `api-stress-test` tool for stress testing
  - SDK/Client: Go standard library `net/http`
  - Supported methods: GET, POST, PUT, DELETE, PATCH, HEAD, OPTIONS
  - Implementation: `api-stress-test/cmd/root.go` lines 8-23, 177-189

**No other external API integrations detected.**

## Data Storage

**Databases:**
- None - All tools are stateless, file-based utilities

**File Storage:**
- Local filesystem only
  - Read operations: File content search (`find-content`), directory scanning (`check-folder-size`, `find-everything`)
  - Write operations: Backup file creation (`.bak` files) in `replace-text` tool
  - Binary file detection: Null byte checking to avoid processing binary files
  - UTF-8 validation for text processing

**Caching:**
- None - No caching layer implemented

## Authentication & Identity

**Auth Provider:**
- None for standard tools
- `api-stress-test` supports custom HTTP headers for authentication:
  - Custom headers passed via `--headers "key1:value1;key2:value2"` flag
  - Header parsing in `api-stress-test/internal/request/request.go` (implied from usage in `cmd/root.go`)
  - Bearer token example in documentation: `--headers "Authorization:Bearer token"`

## Monitoring & Observability

**Error Tracking:**
- None - No external error tracking service

**Logs:**
- Console/stdout output only
  - Colored ANSI output for terminal display
  - JSON output format support in tools:
    - `check-folder-size`: `--json` flag outputs JSON array
    - `api-stress-test`: `--output json` flag outputs structured test results

**Performance Metrics:**
- Built-in metrics collection in `api-stress-test`:
  - Request latency percentiles (p50, p90, p99)
  - Status code distribution
  - Request/second throughput
  - Min/max/average latency tracking
  - Implementation: `api-stress-test/internal/stats/stats.go` (referenced from `cmd/root.go`)

## CI/CD & Deployment

**Hosting:**
- None - These are standalone CLI tools

**CI Pipeline:**
- None detected - No CI configuration files found

**Build System:**
- Makefile-based build automation
- Cross-platform compilation (Linux/Windows)
- Manual build trigger via `make all` or individual tool builds

## Environment Configuration

**Required env vars:**
- None - No environment variables required for standard operation

**Optional env vars:**
- `PROMPT` (Windows only) - Detects command prompt vs PowerShell for system command execution
  - Used in `common-module/utils/system_command_executor.go` lines 18-22 (CLS function)
  - Used for Shellout command execution on Windows

**Secrets location:**
- No built-in secrets management
- Users can pass sensitive data (API keys, tokens) via CLI flags to `api-stress-test`:
  - `--headers` flag for Authorization headers
  - `--json-body` or `--body` flags for request payloads

## Webhooks & Callbacks

**Incoming:**
- None

**Outgoing:**
- None - All tools are request-only utilities

## Command Execution

**System Commands:**
- Platform-specific shell invocation:
  - Linux: `bash -c` via `common-module/utils/system_command_executor.go`
  - Windows: `cmd /c` or `pwsh -Command` depending on `PROMPT` environment variable
  - Used for screen clearing (`CLS()` function) and arbitrary shell commands via `Shellout()` function

---

*Integration audit: 2026-03-12*

# Technology Stack

**Analysis Date:** 2026-03-12

## Languages

**Primary:**
- Go 1.24.4 - All CLI tools and common module

**Secondary:**
- Shell (bash/pwsh) - System command execution via `Shellout()` function in `common-module/utils/system_command_executor.go`

## Runtime

**Environment:**
- Go 1.24 (minimum version requirement documented in README.md)

**Platform Support:**
- Linux (amd64) - Primary development platform
- Windows (amd64) - Supported via Makefile with cross-compilation
- Cross-compilation via `CGO_ENABLED=0 GOOS` variables in Makefile

**Package Manager:**
- Go modules (`go.mod`/`go.sum`)
- Each tool has independent module configuration with local replace directive for `common-module`

## Frameworks

**Core CLI:**
- Cobra v1.10.2 - CLI framework for all tools
  - Provides command structure, flags, help generation
  - Used in: `case-converter`, `check-folder-size`, `find-content`, `find-everything`, `replace-text`, `api-stress-test`

**Standard Library Modules Used:**
- `net/http` - HTTP client for api-stress-test stress testing
- `net/url` - URL parsing and validation
- `encoding/json` - JSON marshaling for output formatting
- `context` - Graceful shutdown and timeouts
- `sync` - Worker pool coordination (WaitGroup, channels)
- `regexp` - Text pattern matching in find-content and find-everything
- `strings` - String manipulation throughout tools
- `os` - File system operations
- `filepath` - Path operations and resolution
- `io/ioutil` - File I/O operations
- `time` - Duration parsing and performance metrics
- `runtime` - Platform detection for system commands
- `os/exec` - External command execution

**Testing:**
- No external testing framework detected - uses Go's standard `testing` package if tests exist

**Build/Dev:**
- Makefile - Cross-platform build automation (`/home/loc/git/my-cli/Makefile`)
  - Supports Linux and Windows (PowerShell 7)
  - Handles platform-specific installation paths and commands

## Key Dependencies

**Critical:**
- `github.com/spf13/cobra` v1.10.2 - Essential for CLI framework across all tools
- `github.com/spf13/pflag` v1.0.9-10 - Transitive dependency for Cobra flag handling
- `golang.org/x/text` v0.33.0 - Unicode text handling in case-converter for text transformation
- `golang.org/x/term` v0.40.0 - Terminal operations in check-folder-size for color output

**Supporting:**
- `github.com/inconshreveable/mousetrap` v1.1.0 - Transitive dependency for Cobra (Windows signal handling)
- `github.com/cpuguy83/go-md2man/v2` v2.0.6 - Transitive dependency for Cobra (help generation)
- `github.com/russross/blackfriday/v2` v2.1.0 - Transitive dependency for Cobra

## Configuration

**Environment:**
- No `.env` files or environment variable configuration detected
- Platform detection via `runtime.GOOS`:
  - Linux: uses `clear` command, `bash -c` for shell commands
  - Windows: uses `cmd /c` or `pwsh -Command` depending on shell context

**Build:**
- Makefile configuration: `/home/loc/git/my-cli/Makefile`
- Platform-specific variables:
  - `GOOS`: windows or linux
  - `GOARCH`: amd64 or auto-detected
  - `EXT`: .exe for Windows, empty for Linux
  - `INSTALL_DIR`: /usr/local/bin (Linux) or custom DEV_KIT_LOCATION (Windows)

**Module Configuration:**
- Each tool references shared module via local replace directive:
  ```
  replace common-module => ../common-module
  ```

## Platform Requirements

**Development:**
- Go 1.24 or later
- Git (for repository access)
- Makefile support (GNU Make)
- Windows-specific: PowerShell 7 if using PowerShell; otherwise Command Prompt or MSYS2 make

**Production:**
- Linux or Windows runtime (amd64)
- No external dependencies or services required
- All tools are statically compiled (`CGO_ENABLED=0`)

## Build Outputs

**Binaries:**
- `case-converter` - Text case conversion utility
- `check-folder-size` - Disk usage analyzer
- `find-content` - Text search tool
- `find-everything` - Advanced file finder
- `replace-text` - Text replacement with backups
- `api-stress-test` - HTTP load testing tool

---

*Stack analysis: 2026-03-12*

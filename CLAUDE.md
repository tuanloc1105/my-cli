# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

A collection of 6 standalone CLI tools written in Go 1.24, plus a shared module. Each tool has its own `go.mod` and is built independently.

| Tool | Description |
|------|-------------|
| `case-converter` | Text case conversion (14 formats) |
| `check-folder-size` | Disk usage analyzer with colored output |
| `find-content` | Text search in files (regex/plain) |
| `find-everything` | File finder with pattern/size filtering |
| `replace-text` | Find and replace with backups |
| `api-stress-test` | HTTP load/stress testing |
| `common-module` | Shared utilities (struct mapping, system commands) |

## Build Commands

```bash
# Build a single tool
cd <tool-dir> && go build -o <tool-name> .

# Build all tools
for dir in */; do
  if [ -f "$dir/go.mod" ]; then
    cd "$dir" && go build -o "$(basename "$dir")" . && cd ..
  fi
done

# Run tests (from within a tool directory)
go test ./...

# Tidy dependencies
go mod tidy
```

There is no top-level Makefile or unified build system. Each tool is built from its own directory.

## Architecture

### Module Structure

Each tool references `common-module` via a local `replace` directive in its `go.mod`:
```
replace common-module => ../common-module
```

### Tool Code Layouts

Two patterns are used:

**Single-file tools** (simpler tools): `case-converter`, `find-content`, `replace-text`
- All logic in `main.go` (and optionally one helper like `searcher.go`)

**Modular tools** (larger tools): `check-folder-size`, `find-everything`, `api-stress-test`
- `cmd/root.go` — Cobra command definitions and CLI entry point
- `internal/` — Domain logic split by concern (e.g., `scanner/`, `finder/`, `ui/`, `stats/`)

### Common Module (`common-module/`)

- `utils/struct_utils.go` — Reflection-based struct field mapping
- `utils/system_command_executor.go` — System command execution, screen clearing (`CLS()`)

## Key Conventions

- **CLI framework**: Cobra (`github.com/spf13/cobra`) for all tools
- **Concurrency**: Worker pools with goroutines, `sync.WaitGroup`, channels for task distribution, `atomic` counters for stats. Worker count typically capped at `runtime.NumCPU()` or 8.
- **File safety**: Binary file detection (null byte check), UTF-8 validation, atomic writes via temp file + rename, `.bak` backup files
- **Performance patterns**: Pre-compiled regex, pre-allocated `strings.Builder` with `Grow()`, buffered I/O (`bufio.Writer`), parallel directory walking
- **Error handling**: `fmt.Errorf` wrapping, sentinel errors (e.g., `errNoChange`), graceful restore on failure
- **Output**: ANSI color codes for terminal output, color applied directly (no external color library)

<!-- GSD:project-start source:PROJECT.md -->
## Project

**api-stress-test Performance Audit & Bug Fix**

A comprehensive evaluation and improvement of the `api-stress-test` CLI tool — a Go-based HTTP load/stress testing tool. The goal is to fix identified bugs and optimize performance for high-concurrency scenarios (1000+ workers), ensuring the tool can handle extreme stress-testing workloads reliably.

**Core Value:** The tool must produce **accurate, reliable stress test results** at high concurrency (1000+ workers) without bottlenecking on its own internals.

### Constraints

- **Language**: Go 1.24 — must stay compatible
- **Dependencies**: Minimal (only Cobra) — no new deps unless justified
- **Backwards compatibility**: All existing flags and behavior must be preserved
- **Testing**: Changes should be benchmarkable (`go test -bench`)
<!-- GSD:project-end -->

<!-- GSD:stack-start source:codebase/STACK.md -->
## Technology Stack

## Languages
- Go 1.24.4 - All CLI tools and common module
- Shell (bash/pwsh) - System command execution via `Shellout()` function in `common-module/utils/system_command_executor.go`
## Runtime
- Go 1.24 (minimum version requirement documented in README.md)
- Linux (amd64) - Primary development platform
- Windows (amd64) - Supported via Makefile with cross-compilation
- Cross-compilation via `CGO_ENABLED=0 GOOS` variables in Makefile
- Go modules (`go.mod`/`go.sum`)
- Each tool has independent module configuration with local replace directive for `common-module`
## Frameworks
- Cobra v1.10.2 - CLI framework for all tools
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
- No external testing framework detected - uses Go's standard `testing` package if tests exist
- Makefile - Cross-platform build automation (`/home/loc/git/my-cli/Makefile`)
## Key Dependencies
- `github.com/spf13/cobra` v1.10.2 - Essential for CLI framework across all tools
- `github.com/spf13/pflag` v1.0.9-10 - Transitive dependency for Cobra flag handling
- `golang.org/x/text` v0.33.0 - Unicode text handling in case-converter for text transformation
- `golang.org/x/term` v0.40.0 - Terminal operations in check-folder-size for color output
- `github.com/inconshreveable/mousetrap` v1.1.0 - Transitive dependency for Cobra (Windows signal handling)
- `github.com/cpuguy83/go-md2man/v2` v2.0.6 - Transitive dependency for Cobra (help generation)
- `github.com/russross/blackfriday/v2` v2.1.0 - Transitive dependency for Cobra
## Configuration
- No `.env` files or environment variable configuration detected
- Platform detection via `runtime.GOOS`:
- Makefile configuration: `/home/loc/git/my-cli/Makefile`
- Platform-specific variables:
- Each tool references shared module via local replace directive:
## Platform Requirements
- Go 1.24 or later
- Git (for repository access)
- Makefile support (GNU Make)
- Windows-specific: PowerShell 7 if using PowerShell; otherwise Command Prompt or MSYS2 make
- Linux or Windows runtime (amd64)
- No external dependencies or services required
- All tools are statically compiled (`CGO_ENABLED=0`)
## Build Outputs
- `case-converter` - Text case conversion utility
- `check-folder-size` - Disk usage analyzer
- `find-content` - Text search tool
- `find-everything` - Advanced file finder
- `replace-text` - Text replacement with backups
- `api-stress-test` - HTTP load testing tool
<!-- GSD:stack-end -->

<!-- GSD:conventions-start source:CONVENTIONS.md -->
## Conventions

## Naming Patterns
- `main.go` for entry points
- `cmd/root.go` for Cobra command definitions in modular tools
- `internal/` subdirectories by concern: `scanner/`, `stats/`, `request/`, `ui/`, `finder/`
- Test files: `*_test.go` co-located with implementation
- Helper modules: `searcher.go` alongside `main.go` when needed
- Exported functions start with uppercase: `ProcessCaseConversions()`, `GetSizesOfSubfolders()`, `NewCollector()`
- Private functions start with lowercase: `processDirectory()`, `normalizeText()`, `detectCaseType()`
- Constructor pattern: `NewTypeName(params)` for struct initialization
- Getter/setter-like names: `Get` prefix for retrieval, `Record` for accumulation
- Verb-first: `ValidateURL()`, `ParseHeaders()`, `PrepareBody()`, `ProcessCaseConversions()`
- camelCase for local variables: `targetURL`, `concurrency`, `oldText`, `fileExtensions`
- SCREAMING_SNAKE_CASE for constants: `binaryCheckSize`, `defaultMaxFileSize`, `validMethods` (map names vary)
- Single letter for loop counters acceptable: `i`, `j` in standard iterations
- Descriptive for goroutine flags: `showProgress`, `caseSensitive`, `createBackup`
- Package-level: `global*` prefix for singleton instances (e.g., `globalCaseConverter`, `globalColorOutput`, `titleCaser`)
- PascalCase for struct names: `CaseConverter`, `Collector`, `Result`, `ScanOptions`, `ErrorEntry`
- Exposed at package level, never single-letter types
- Field names within structs: camelCase, lowercase except exported fields for JSON: `statusCode`, `elapsed`, `OK`
## Code Style
- Go standard formatting (gofmt) — no custom formatter detected
- 1 tab = 4 spaces (Go default)
- Column limit: Pragmatic; no hard enforced limit observed
- Brace style: Opening brace on same line (Go standard)
- No `.golangci.yml` or linter config detected
- Code follows Go vet conventions implicitly
## Import Organization
- No import aliases used (direct module paths)
- Common module referenced via `replace` directive: `replace common-module => ../common-module` in `go.mod`
- Imports use full path: `"common-module/utils"`
## Error Handling
- **Error wrapping**: `fmt.Errorf("context: %w", err)` for chaining context
- **Sentinel errors**: `var errNoChange = fmt.Errorf("no change")` for non-recoverable states
- **No panic in libraries**: Only in main entry points if critical
- **Error return style**: Functions that may fail return `(T, error)` or `error`
- **Silent skips on non-critical errors**: Warning printed but execution continues
- **Graceful degradation**: Backup restore on write failures (atomic write pattern)
## Logging
- `fmt` package only
- No structured logging library
- Status messages: `fmt.Printf("message")`
- Errors to stderr: `fmt.Fprintf(os.Stderr, "message")`
- Example: `fmt.Fprintf(os.Stderr, "Error accessing %s: %v\n", parentFolder, err)`
- Progress updates: `fmt.Printf("\r%s", progressMsg)` with carriage return for terminal clearing
- No log levels (debug, info, warn, error) — direct output control
## Comments
- Package-level doc comments on types/functions: Mandatory for exported items
- Explanation of non-obvious logic (e.g., directory walk recursion bounds)
- Performance decisions (e.g., pre-allocation with `Grow()`, worker pool sizing)
- Example from `scanner.go`: Comments explain why inline fallback is safe and recursion depth limits
- Go docstrings (no GoDoc tags observed)
- Sentence-style: Start with function name or subject
- Multiline comments explain parameters and return values
- No `@param`/`@return` style tags — narrative format
## Function Design
- Median function length: 20–80 lines
- Larger functions (100+ lines) decomposed into helpers
- Example: `Execute()` in `cmd/root.go` delegates validation and execution to sub-functions
- Single responsibility: Parse flags → Validate → Execute
- Limited to 3–5 parameters for core functions
- Multiple configuration values grouped in structs (e.g., `ScanOptions` with `ShowProgress`, `ExcludeList`, `Ctx`)
- Variadic parameters not heavily used
- Receivers common for methods on small structs
- Single return value or `(value, error)` pair
- Multiple returns organized as struct fields for complex results
- Nil checks for optional returns: `if result != nil { ... }`
## Module Design
- Capitalized names only: Types, functions, vars exposed
- Example exports: `Collector`, `Record()`, `GetStatistics()`, `ScanOptions`, `ScanResult`
- Pattern: Constructor + receiver methods for stateful types
- No barrel files (`index.go`, `__init__.go`) in Go modules
- Entry point always `main()` in `main.go` or delegated from there
- `cmd/root.go` serves as CLI entry point, not a barrel file
- Worker pools: `sync.WaitGroup`, channels for task distribution
- Atomic operations for stats: `atomic.AddInt64(&counter, delta)`, `atomic.LoadInt64(ptr)`
- Mutex guards shared state: `sync.Mutex` on struct containing mutable fields
- Goroutine-safe collectors: All access through locked methods
- `strings.Builder` with `.Grow(len(s))` for known-size concatenation
- `make([]T, 0, cap)` with capacity hints for slices
- Pre-compiled regex patterns stored as package vars
- Maps pre-sized: `make(map[string]struct{}, len(exclude))`
<!-- GSD:conventions-end -->

<!-- GSD:architecture-start source:ARCHITECTURE.md -->
## Architecture

## Pattern Overview
- Cobra-based CLI framework for all tools
- Local module references via `replace` directives in `go.mod`
- Goroutine-based concurrency with worker pools for I/O-bound tasks
- Consistent error handling using `fmt.Errorf` wrapping and sentinel errors
- Direct ANSI color code output (no external color library)
- File safety: binary detection, UTF-8 validation, atomic writes via temp files
## Layers
- Purpose: Command definition, flag parsing, user interaction
- Location: `main.go` (single-file tools) or `cmd/root.go` (modular tools)
- Contains: Cobra command definitions, flag registration, main orchestration logic
- Depends on: Internal packages or domain logic functions
- Used by: User invocation via binary execution
- Purpose: Core business logic separated by concern
- Location: `internal/<domain>/` (modular tools) or inline in `main.go` (simple tools)
- Contains: Algorithms, data processing, file operations, network operations
- Depends on: Common module utilities, standard library
- Used by: CLI layer for command execution
- Purpose: Shared cross-cutting concerns
- Location: `common-module/utils/`
- Contains: System command execution (`Shellout`), screen clearing (`CLS`)
- Depends on: Standard library only
- Used by: All tools via `replace` directive import
- Purpose: Terminal output formatting and display
- Location: `internal/ui/` (modular tools) or inline in `main.go`
- Contains: ANSI color codes, formatting functions, progress tracking
- Depends on: Standard library, internal types
- Used by: CLI layer for displaying results
## Data Flow
- CLI flags stored in function-scoped variables (no global state outside main)
- Options structs passed to domain constructors for immutable configuration
- Thread-safe collectors use `sync.Mutex` for concurrent result aggregation
- Atomic counters (`atomic.Int64`) for progress tracking without locks
## Key Abstractions
- Purpose: Parallel task execution with bounded concurrency
- Examples: `check-folder-size/internal/scanner`, `find-everything/internal/finder`, `api-stress-test/cmd/root.go`
- Pattern: Task channels feed workers spawned in goroutine loop, results collected in concurrent-safe aggregator
- Purpose: Thread-safe aggregation of results across concurrent workers
- Examples: `api-stress-test/internal/stats/collector.go`, `find-everything/internal/ui/progress.go`
- Pattern: Mutex-protected fields, atomic counters for read-heavy metrics, bulk flushing for latency arrays
- Purpose: Immutable configuration passed to domain objects
- Examples: `ScanOptions` (check-folder-size), `FinderOptions` (find-everything), `TestConfig` (api-stress-test)
- Pattern: Struct created in CLI layer, passed to constructor, used for dependency injection
- Purpose: Prevent data loss and binary file corruption
- Examples: `replace-text/main.go`
- Pattern: Binary check (null byte in first 8KB), UTF-8 validation, backup creation, atomic write (temp + rename)
## Entry Points
- Location: `case-converter/main.go`, `find-content/main.go`, `replace-text/main.go`
- Triggers: Direct binary invocation with arguments
- Responsibilities: Parse flags, validate input, execute domain logic, format output
- Location: `check-folder-size/main.go`, `find-everything/main.go`, `api-stress-test/main.go`
- Triggers: Binary invocation
- Responsibilities: Forward execution to `cmd.Execute()`
- Location: `cmd/root.go`
- Triggers: Invoked by `Execute()` function
- Responsibilities: Define Cobra command, parse flags, instantiate domain objects, handle results
## Error Handling
```go
```
```go
```
```go
```
## Cross-Cutting Concerns
- URL validation: `api-stress-test/cmd/root.go` — scheme, host checks
- Flag value validation: `parseSize()` function (check-folder-size, find-everything) — unit parsing
- File type checks: Binary file detection via null byte, UTF-8 validation via `utf8.Valid()`
- Mutex protection: `sync.Mutex` around `Collector.Record()` for latency aggregation
- Atomic operations: `atomic.Int64` for counters in progress trackers
- Channel coordination: Worker pool pattern with buffered job/result channels
- Context cancellation: `context.WithTimeout()` or `context.WithCancel()` for graceful shutdown
- Clear screen via `utils.CLS()` (Windows/Linux detection)
- ANSI codes hardcoded: `\033[` escape sequences for colors (no external library)
- Color mapping: Green (42/small), Yellow (43/medium), Red (41/large) for size ranges
- Progress overwrites: Carriage return (`\r`) for live progress updates
<!-- GSD:architecture-end -->

<!-- GSD:workflow-start source:GSD defaults -->
## GSD Workflow Enforcement

Before using Edit, Write, or other file-changing tools, start work through a GSD command so planning artifacts and execution context stay in sync.

Use these entry points:
- `/gsd:quick` for small fixes, doc updates, and ad-hoc tasks
- `/gsd:debug` for investigation and bug fixing
- `/gsd:execute-phase` for planned phase work

Do not make direct repo edits outside a GSD workflow unless the user explicitly asks to bypass it.
<!-- GSD:workflow-end -->

<!-- GSD:profile-start -->
## Developer Profile

> Profile not yet configured. Run `/gsd:profile-user` to generate your developer profile.
> This section is managed by `generate-claude-profile` -- do not edit manually.
<!-- GSD:profile-end -->

# Codebase Structure

**Analysis Date:** 2026-03-12

## Directory Layout

```
my-cli/
├── case-converter/               # Single-file: text case conversion tool
│   ├── main.go                   # All logic in one file (14 case formats)
│   └── go.mod                    # Independent module with common-module reference
├── check-folder-size/            # Modular: disk usage analyzer
│   ├── main.go                   # Entry point (delegates to cmd)
│   ├── cmd/
│   │   └── root.go               # Cobra root command and orchestration
│   ├── internal/
│   │   ├── scanner/              # Parallel directory walking logic
│   │   │   └── scanner.go
│   │   └── ui/                   # Output formatting
│   │       ├── printer.go        # Result display with ANSI colors
│   │       └── (other UI files)
│   └── go.mod                    # Independent module
├── find-content/                 # Single-file: recursive text search
│   ├── main.go                   # Cobra command + grep logic
│   ├── searcher.go               # Search implementation (if applicable)
│   └── go.mod                    # Independent module
├── find-everything/              # Modular: advanced file finder
│   ├── main.go                   # Entry point (delegates to cmd)
│   ├── cmd/
│   │   └── root.go               # Cobra root command
│   ├── internal/
│   │   ├── finder/               # File/directory search logic
│   │   │   ├── finder.go         # Main finder with worker pool
│   │   │   └── walker.go         # Parallel directory walk
│   │   ├── types/                # Data structures
│   │   │   └── types.go          # FileResult struct
│   │   └── ui/                   # Output formatting
│   │       └── display.go        # Result display with progress tracking
│   └── go.mod                    # Independent module
├── replace-text/                 # Single-file: find and replace with backups
│   ├── main.go                   # All logic in one file
│   └── go.mod                    # Independent module
├── api-stress-test/              # Modular: HTTP load testing
│   ├── main.go                   # Entry point (delegates to cmd)
│   ├── cmd/
│   │   ├── root.go               # Cobra root command, worker pool, orchestration
│   │   └── root_test.go          # Command tests
│   ├── internal/
│   │   ├── request/              # HTTP request execution
│   │   │   ├── client.go         # Request execution and body preparation
│   │   │   ├── client_test.go    # Client tests
│   │   │   ├── ratelimiter.go    # Rate limiting implementation
│   │   │   └── ratelimiter_test.go
│   │   └── stats/                # Statistics collection and calculation
│   │       ├── collector.go      # Thread-safe result aggregation
│   │       └── collector_test.go # Collector tests
│   └── go.mod                    # Independent module
├── common-module/                # Shared utilities (not built independently)
│   ├── utils/
│   │   ├── system_command_executor.go  # CLS(), Shellout()
│   │   ├── struct_utils.go             # Reflection-based field mapping
│   │   └── (other utilities)
│   └── go.mod                    # Shared module
├── .planning/                    # Documentation and planning
│   └── codebase/                 # This analysis
│       ├── ARCHITECTURE.md
│       ├── STRUCTURE.md
│       └── (other docs as generated)
├── Makefile                      # Build and install commands
├── CLAUDE.md                     # Project guidance for Claude
└── README.md                     # User documentation
```

## Directory Purposes

**case-converter/:**
- Purpose: Single executable for 14 different text case transformations
- Contains: Type definitions (CaseConverter, ColorOutput), transformation methods, case detection logic
- Key files: `main.go` (657 lines)

**check-folder-size/:**
- Purpose: Disk usage analyzer with parallel scanning and colored output
- Contains: Cobra CLI, parallel directory walker, output formatter
- Key files:
  - `cmd/root.go`: Flag parsing, validation, orchestration
  - `internal/scanner/scanner.go`: Worker pool with task distribution
  - `internal/ui/printer.go`: ANSI formatting with size-based colors

**find-content/:**
- Purpose: Recursive text search in files with regex/plain text matching
- Contains: Cobra CLI, file search implementation
- Key files: `main.go` (300+ lines with FileSearcher type)

**find-everything/:**
- Purpose: Advanced file/directory finder with pattern matching, filtering, progress tracking
- Contains: Cobra CLI, parallel finder with worker pool, UI with progress display
- Key files:
  - `cmd/root.go`: Flag parsing, option construction
  - `internal/finder/finder.go`: File pattern matching with glob-to-regex conversion
  - `internal/finder/walker.go`: Parallel directory traversal
  - `internal/ui/display.go`: Progress tracker with atomic counters

**replace-text/:**
- Purpose: Find and replace text in files with backup and safety checks
- Contains: File processing with binary detection, UTF-8 validation, atomic writes
- Key files: `main.go` (300+ lines with processFile, isBinaryFile functions)

**api-stress-test/:**
- Purpose: HTTP load/stress testing with concurrency control, latency percentiles, statistics
- Contains: Cobra CLI, worker pool for requests, statistics collection, rate limiting
- Key files:
  - `cmd/root.go`: Flag parsing, job distribution, result collection (460 lines)
  - `internal/request/client.go`: HTTP request execution, body preparation
  - `internal/stats/collector.go`: Thread-safe aggregation with mutex protection
  - `internal/request/ratelimiter.go`: Token bucket rate limiter

**common-module/:**
- Purpose: Shared utilities imported by all tools via `replace` directive
- Contains: System command execution, screen clearing, struct reflection utilities
- Key files:
  - `utils/system_command_executor.go`: `CLS()` (Windows/Linux), `Shellout()`
  - `utils/struct_utils.go`: Field mapping via reflection

## Key File Locations

**Entry Points:**
- Single-file tools: `case-converter/main.go`, `find-content/main.go`, `replace-text/main.go` — Cobra root command defined inline
- Modular tools: `check-folder-size/main.go`, `find-everything/main.go`, `api-stress-test/main.go` — Simple delegation to `cmd.Execute()`
- Command impl: `cmd/root.go` — Actual Cobra root command definition

**Configuration:**
- `go.mod` files: Each tool has independent module with `replace common-module => ../common-module`
- Makefile: Top-level `Makefile` with build and install targets
- CLAUDE.md: Project guidance (conventions, patterns, build commands)

**Core Logic:**
- Case conversion: `case-converter/main.go` (all logic)
- Disk scanning: `check-folder-size/internal/scanner/scanner.go` (parallelWalker, worker pool)
- Text search: `find-content/main.go` (FileSearcher type)
- File finding: `find-everything/internal/finder/finder.go` (FileFinder with glob-to-regex)
- Text replacement: `replace-text/main.go` (processFile, file safety)
- HTTP testing: `api-stress-test/cmd/root.go` (worker pool) + `internal/stats/collector.go` (aggregation)

**Testing:**
- `api-stress-test/internal/request/client_test.go`
- `api-stress-test/internal/request/ratelimiter_test.go`
- `api-stress-test/internal/stats/collector_test.go`
- `api-stress-test/cmd/root_test.go`
- Tests co-located with implementation files using `_test.go` suffix

## Naming Conventions

**Files:**
- Executables: kebab-case directory name (e.g., `case-converter`, `api-stress-test`)
- Go packages: lowercase (e.g., `scanner`, `finder`, `stats`)
- Test files: `_test.go` suffix (e.g., `collector_test.go`)
- Helpers: domain name (e.g., `scanner.go`, `finder.go`, `printer.go`)

**Directories:**
- Tools: kebab-case (e.g., `check-folder-size`)
- Packages: lowercase domain (e.g., `cmd`, `internal`, `scanner`, `ui`)
- Package groups: domain-based (e.g., `internal/scanner/`, `internal/finder/`)

**Functions:**
- Public: PascalCase (e.g., `GetSizesOfSubfolders`, `NewFileFinder`, `PrintResults`)
- Private: camelCase (e.g., `processFile`, `isBinaryFile`, `grepRecursive`)
- Methods: PascalCase receiver methods (e.g., `(c *Collector) Record()`)

**Types:**
- PascalCase (e.g., `FileSearcher`, `Collector`, `FileFinder`, `ScanOptions`)
- Structs for configuration: `Options` suffix (e.g., `ScanOptions`, `FinderOptions`)
- Result types: descriptive (e.g., `FileResult`, `ScanResult`, `Statistics`)

**Variables:**
- Local: camelCase (e.g., `minSize`, `excludeList`, `workerCount`)
- Module-level flags: camelCase (e.g., `sortBy`, `caseSensitive`, `targetURL`)
- Constants: UPPER_SNAKE_CASE (e.g., `binaryCheckSize`, `defaultMaxFileSize`)

## Where to Add New Code

**New Feature (existing tool):**
- Primary code: Add to tool's main location (either `main.go` or appropriate `internal/<domain>/` package)
- Tests: Add `_test.go` file co-located with implementation
- Configuration: Add flag to `cmd/root.go` (modular) or `main.go` (single-file), register with Cobra
- Example: Adding filtering to `check-folder-size` → modify `internal/scanner/scanner.go` + `cmd/root.go` flags

**New Tool (simple, <300 lines):**
- Create directory: `new-tool-name/`
- Create `new-tool-name/main.go` with all logic
- Create `new-tool-name/go.mod` with `replace common-module => ../common-module`
- Cobra command defined inline in `main.go`
- Use `common-module/utils` for shared functionality

**New Tool (complex, >300 lines):**
- Create directory: `new-tool-name/`
- Create `new-tool-name/main.go` with simple delegation: `cmd.Execute()`
- Create `new-tool-name/cmd/root.go` with Cobra command and orchestration
- Create `new-tool-name/internal/<domain>/` packages for logic separation
- Create `new-tool-name/go.mod` with `replace common-module => ../common-module`
- Add tests in `internal/<domain>/<module>_test.go`

**Utilities:**
- Shared helpers: Add to `common-module/utils/`
- Import in tools via `import "common-module/utils"`
- Tool-specific helpers: Keep in tool's `internal/` package

## Special Directories

**`cmd/`:**
- Purpose: Cobra command definitions and CLI orchestration
- Generated: No (all hand-written)
- Committed: Yes
- Scope: Modular tools only (check-folder-size, find-everything, api-stress-test)

**`internal/`:**
- Purpose: Private package organization for modular tools
- Generated: No
- Committed: Yes
- Scope: Modular tools only; single-file tools keep logic inline
- Pattern: Organize by domain concern (scanner, finder, request, stats, ui)

**`common-module/`:**
- Purpose: Shared cross-tool utilities
- Generated: No
- Committed: Yes
- Built: No (imported via `replace` directive, not built separately)
- Scope: Shared by all tools

**`.planning/codebase/`:**
- Purpose: Analysis and planning documents for code guidance
- Generated: Yes (by `gsd` tools)
- Committed: Yes
- Contents: ARCHITECTURE.md, STRUCTURE.md, CONVENTIONS.md, TESTING.md, CONCERNS.md

**Root level:**
- `Makefile`: Build all tools, install to system paths
- `CLAUDE.md`: Guidance for Claude Code (conventions, architecture summary, build commands)
- `README.md`: User documentation

---

*Structure analysis: 2026-03-12*

# Coding Conventions

**Analysis Date:** 2026-03-12

## Naming Patterns

**Files:**
- `main.go` for entry points
- `cmd/root.go` for Cobra command definitions in modular tools
- `internal/` subdirectories by concern: `scanner/`, `stats/`, `request/`, `ui/`, `finder/`
- Test files: `*_test.go` co-located with implementation
- Helper modules: `searcher.go` alongside `main.go` when needed

**Functions:**
- Exported functions start with uppercase: `ProcessCaseConversions()`, `GetSizesOfSubfolders()`, `NewCollector()`
- Private functions start with lowercase: `processDirectory()`, `normalizeText()`, `detectCaseType()`
- Constructor pattern: `NewTypeName(params)` for struct initialization
- Getter/setter-like names: `Get` prefix for retrieval, `Record` for accumulation
- Verb-first: `ValidateURL()`, `ParseHeaders()`, `PrepareBody()`, `ProcessCaseConversions()`

**Variables:**
- camelCase for local variables: `targetURL`, `concurrency`, `oldText`, `fileExtensions`
- SCREAMING_SNAKE_CASE for constants: `binaryCheckSize`, `defaultMaxFileSize`, `validMethods` (map names vary)
- Single letter for loop counters acceptable: `i`, `j` in standard iterations
- Descriptive for goroutine flags: `showProgress`, `caseSensitive`, `createBackup`
- Package-level: `global*` prefix for singleton instances (e.g., `globalCaseConverter`, `globalColorOutput`, `titleCaser`)

**Types:**
- PascalCase for struct names: `CaseConverter`, `Collector`, `Result`, `ScanOptions`, `ErrorEntry`
- Exposed at package level, never single-letter types
- Field names within structs: camelCase, lowercase except exported fields for JSON: `statusCode`, `elapsed`, `OK`

## Code Style

**Formatting:**
- Go standard formatting (gofmt) — no custom formatter detected
- 1 tab = 4 spaces (Go default)
- Column limit: Pragmatic; no hard enforced limit observed
- Brace style: Opening brace on same line (Go standard)

**Linting:**
- No `.golangci.yml` or linter config detected
- Code follows Go vet conventions implicitly

## Import Organization

**Order:**
1. Standard library imports first
2. Third-party imports (e.g., `github.com/spf13/cobra`, `golang.org/x`)
3. Local/internal imports (relative to module)
4. Blank line between groups

**Example from `api-stress-test/cmd/root.go`:**
```go
import (
	"context"
	"encoding/json"
	"fmt"
	// ... more stdlib

	"api-stress-test/internal/request"
	"api-stress-test/internal/stats"

	"github.com/spf13/cobra"
)
```

**Path Aliases:**
- No import aliases used (direct module paths)
- Common module referenced via `replace` directive: `replace common-module => ../common-module` in `go.mod`
- Imports use full path: `"common-module/utils"`

## Error Handling

**Patterns:**
- **Error wrapping**: `fmt.Errorf("context: %w", err)` for chaining context
  - Example: `return fmt.Errorf("parsing --data: %w", err)` from `cmd/root.go:84`
- **Sentinel errors**: `var errNoChange = fmt.Errorf("no change")` for non-recoverable states
  - Checked with equality: `if err == errNoChange { return nil }`
- **No panic in libraries**: Only in main entry points if critical
- **Error return style**: Functions that may fail return `(T, error)` or `error`
  - Example: `func ParseData(raw string) (map[string]string, error)`
- **Silent skips on non-critical errors**: Warning printed but execution continues
  - Example: `fmt.Fprintf(os.Stderr, "Warning: Skipping..."); return filepath.SkipDir`
- **Graceful degradation**: Backup restore on write failures (atomic write pattern)

**File safety pattern (from `replace-text/main.go`):**
```go
// Create backup
backupFilename := filename + ".bak"
if err := os.Rename(filename, backupFilename); err != nil {
    return fmt.Errorf("failed to create backup: %w", err)
}

// Write to temp, then rename
tmp, err := os.CreateTemp(dir, ".replace-text-*.tmp")
if err != nil {
    os.Rename(backupFilename, filename)  // Restore on failure
    return fmt.Errorf("failed to create temp file: %w", err)
}
```

## Logging

**Framework:**
- `fmt` package only
- No structured logging library

**Patterns:**
- Status messages: `fmt.Printf("message")`
- Errors to stderr: `fmt.Fprintf(os.Stderr, "message")`
- Example: `fmt.Fprintf(os.Stderr, "Error accessing %s: %v\n", parentFolder, err)`
- Progress updates: `fmt.Printf("\r%s", progressMsg)` with carriage return for terminal clearing
- No log levels (debug, info, warn, error) — direct output control

## Comments

**When to Comment:**
- Package-level doc comments on types/functions: Mandatory for exported items
- Explanation of non-obvious logic (e.g., directory walk recursion bounds)
- Performance decisions (e.g., pre-allocation with `Grow()`, worker pool sizing)
- Example from `scanner.go`: Comments explain why inline fallback is safe and recursion depth limits

**JSDoc/TSDoc:**
- Go docstrings (no GoDoc tags observed)
- Sentence-style: Start with function name or subject
  - Example: `// GetSizesOfSubfolders calculates sizes of immediate subfolders/files`
  - Example: `// NewCollector creates a new statistics collector with pre-allocated capacity.`
- Multiline comments explain parameters and return values
- No `@param`/`@return` style tags — narrative format

**Example from `stats/collector.go`:**
```go
// Collector collects and calculates statistics for stress test results.
// It is thread-safe and designed to handle concurrent result recording.
// The collector maintains latency data for percentile calculations and
// tracks success/failure counts and HTTP status code distribution.
```

## Function Design

**Size:**
- Median function length: 20–80 lines
- Larger functions (100+ lines) decomposed into helpers
- Example: `Execute()` in `cmd/root.go` delegates validation and execution to sub-functions
- Single responsibility: Parse flags → Validate → Execute

**Parameters:**
- Limited to 3–5 parameters for core functions
- Multiple configuration values grouped in structs (e.g., `ScanOptions` with `ShowProgress`, `ExcludeList`, `Ctx`)
- Variadic parameters not heavily used
- Receivers common for methods on small structs

**Return Values:**
- Single return value or `(value, error)` pair
- Multiple returns organized as struct fields for complex results
  - Example: `ScanResult` with `Sizes map[string]int64` and `WarningCount int64`
  - Example: `Result` with `OK`, `StatusCode`, `Elapsed`, `Error`
- Nil checks for optional returns: `if result != nil { ... }`

## Module Design

**Exports:**
- Capitalized names only: Types, functions, vars exposed
- Example exports: `Collector`, `Record()`, `GetStatistics()`, `ScanOptions`, `ScanResult`
- Pattern: Constructor + receiver methods for stateful types

**Barrel Files:**
- No barrel files (`index.go`, `__init__.go`) in Go modules
- Entry point always `main()` in `main.go` or delegated from there
- `cmd/root.go` serves as CLI entry point, not a barrel file

**Concurrency patterns:**
- Worker pools: `sync.WaitGroup`, channels for task distribution
  - Example: `walker.go` uses buffered channel, workers, and task counter
- Atomic operations for stats: `atomic.AddInt64(&counter, delta)`, `atomic.LoadInt64(ptr)`
- Mutex guards shared state: `sync.Mutex` on struct containing mutable fields
- Goroutine-safe collectors: All access through locked methods

**Pre-allocation & optimization:**
- `strings.Builder` with `.Grow(len(s))` for known-size concatenation
- `make([]T, 0, cap)` with capacity hints for slices
- Pre-compiled regex patterns stored as package vars
- Maps pre-sized: `make(map[string]struct{}, len(exclude))`

---

*Convention analysis: 2026-03-12*

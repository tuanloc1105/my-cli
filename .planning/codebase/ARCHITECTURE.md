# Architecture

**Analysis Date:** 2026-03-12

## Pattern Overview

**Overall:** Multi-tool CLI suite with independent deployable units and shared utility module. Each tool follows either a monolithic single-file or modular internal package pattern based on complexity.

**Key Characteristics:**
- Cobra-based CLI framework for all tools
- Local module references via `replace` directives in `go.mod`
- Goroutine-based concurrency with worker pools for I/O-bound tasks
- Consistent error handling using `fmt.Errorf` wrapping and sentinel errors
- Direct ANSI color code output (no external color library)
- File safety: binary detection, UTF-8 validation, atomic writes via temp files

## Layers

**CLI Layer (Entry Point):**
- Purpose: Command definition, flag parsing, user interaction
- Location: `main.go` (single-file tools) or `cmd/root.go` (modular tools)
- Contains: Cobra command definitions, flag registration, main orchestration logic
- Depends on: Internal packages or domain logic functions
- Used by: User invocation via binary execution

**Domain Logic Layer:**
- Purpose: Core business logic separated by concern
- Location: `internal/<domain>/` (modular tools) or inline in `main.go` (simple tools)
- Contains: Algorithms, data processing, file operations, network operations
- Depends on: Common module utilities, standard library
- Used by: CLI layer for command execution

**Utilities Layer:**
- Purpose: Shared cross-cutting concerns
- Location: `common-module/utils/`
- Contains: System command execution (`Shellout`), screen clearing (`CLS`)
- Depends on: Standard library only
- Used by: All tools via `replace` directive import

**Output/UI Layer:**
- Purpose: Terminal output formatting and display
- Location: `internal/ui/` (modular tools) or inline in `main.go`
- Contains: ANSI color codes, formatting functions, progress tracking
- Depends on: Standard library, internal types
- Used by: CLI layer for displaying results

## Data Flow

**Single-File Tools (Simple Pattern):**
1. User provides input via CLI arguments or file
2. CLI layer validates input and parses flags
3. Domain logic processes input (case conversion, search, replacement)
4. Results formatted with ANSI colors
5. Output to stdout

**Modular Tools (Complex Pattern):**
1. CLI (`cmd/root.go`) parses arguments and flags
2. Options struct created with parsed values
3. Domain object instantiated with options (e.g., `FileFinder`, `Collector`)
4. Worker goroutine pool spawned (capped at `runtime.NumCPU()` or explicit limit)
5. Work distributed via channels to workers
6. Results collected in thread-safe collector
7. UI layer formats and displays final statistics
8. Output to stdout or structured format (JSON)

**State Management:**
- CLI flags stored in function-scoped variables (no global state outside main)
- Options structs passed to domain constructors for immutable configuration
- Thread-safe collectors use `sync.Mutex` for concurrent result aggregation
- Atomic counters (`atomic.Int64`) for progress tracking without locks

## Key Abstractions

**Worker Pool Pattern:**
- Purpose: Parallel task execution with bounded concurrency
- Examples: `check-folder-size/internal/scanner`, `find-everything/internal/finder`, `api-stress-test/cmd/root.go`
- Pattern: Task channels feed workers spawned in goroutine loop, results collected in concurrent-safe aggregator

**Statistics Collector:**
- Purpose: Thread-safe aggregation of results across concurrent workers
- Examples: `api-stress-test/internal/stats/collector.go`, `find-everything/internal/ui/progress.go`
- Pattern: Mutex-protected fields, atomic counters for read-heavy metrics, bulk flushing for latency arrays

**Options/Configuration Struct:**
- Purpose: Immutable configuration passed to domain objects
- Examples: `ScanOptions` (check-folder-size), `FinderOptions` (find-everything), `TestConfig` (api-stress-test)
- Pattern: Struct created in CLI layer, passed to constructor, used for dependency injection

**File Safety Pattern:**
- Purpose: Prevent data loss and binary file corruption
- Examples: `replace-text/main.go`
- Pattern: Binary check (null byte in first 8KB), UTF-8 validation, backup creation, atomic write (temp + rename)

## Entry Points

**Single-File Tools:**
- Location: `case-converter/main.go`, `find-content/main.go`, `replace-text/main.go`
- Triggers: Direct binary invocation with arguments
- Responsibilities: Parse flags, validate input, execute domain logic, format output

**Modular Tools - Main:**
- Location: `check-folder-size/main.go`, `find-everything/main.go`, `api-stress-test/main.go`
- Triggers: Binary invocation
- Responsibilities: Forward execution to `cmd.Execute()`

**Modular Tools - Root Command:**
- Location: `cmd/root.go`
- Triggers: Invoked by `Execute()` function
- Responsibilities: Define Cobra command, parse flags, instantiate domain objects, handle results

## Error Handling

**Strategy:** Explicit error propagation with context-aware wrapping using `fmt.Errorf`. Sentinel errors for control flow (not exceptional conditions).

**Patterns:**

**Propagating errors:**
```go
// From check-folder-size/cmd/root.go
minSizeBytes, err := parseSize(minSize)
if err != nil {
    fmt.Fprintf(os.Stderr, "Error: invalid --min-size value '%s': %v\n", minSize, err)
    os.Exit(1)
}
```

**Sentinel error for control flow:**
```go
// From replace-text/main.go
var errNoChange = fmt.Errorf("no change")
if !bytes.Contains(content, oldText) {
    return errNoChange  // Not an error, just no work to do
}
// Caller distinguishes:
if err == errNoChange {
    // Skip file silently
} else if err != nil {
    // Handle real error
}
```

**Graceful restoration on failure:**
```go
// From replace-text/main.go
if createBackup {
    backupFilename := filename + ".bak"
    if err := os.Rename(filename, backupFilename); err != nil {
        return fmt.Errorf("failed to create backup: %w", err)
    }
}
// If write fails, restore from backup
if err := tmp.Write(newContent); err != nil {
    os.Remove(tmpName)
    os.Rename(backupFilename, filename)  // Restore
    return fmt.Errorf("failed to write: %w", err)
}
```

## Cross-Cutting Concerns

**Logging:** Console output only. Progress via `fmt.Printf` with ANSI codes. Errors to stderr via `fmt.Fprintf(os.Stderr, ...)`.

**Validation:**
- URL validation: `api-stress-test/cmd/root.go` — scheme, host checks
- Flag value validation: `parseSize()` function (check-folder-size, find-everything) — unit parsing
- File type checks: Binary file detection via null byte, UTF-8 validation via `utf8.Valid()`

**Authentication:** Not applicable (no external service dependencies).

**Concurrency Safety:**
- Mutex protection: `sync.Mutex` around `Collector.Record()` for latency aggregation
- Atomic operations: `atomic.Int64` for counters in progress trackers
- Channel coordination: Worker pool pattern with buffered job/result channels
- Context cancellation: `context.WithTimeout()` or `context.WithCancel()` for graceful shutdown

**Screen Output:**
- Clear screen via `utils.CLS()` (Windows/Linux detection)
- ANSI codes hardcoded: `\033[` escape sequences for colors (no external library)
- Color mapping: Green (42/small), Yellow (43/medium), Red (41/large) for size ranges
- Progress overwrites: Carriage return (`\r`) for live progress updates

---

*Architecture analysis: 2026-03-12*

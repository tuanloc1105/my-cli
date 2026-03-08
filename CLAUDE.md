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

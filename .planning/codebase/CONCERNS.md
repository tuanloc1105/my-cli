# Codebase Concerns

**Analysis Date:** 2026-03-12

## Tech Debt

**Scanner: Potential Memory Exhaustion in Parallel Walker**
- Issue: The `parallelWalker` in `check-folder-size/internal/scanner/scanner.go` uses an in-memory atomic counter per top-level entry (`pendingTasks`), but if a large directory tree has extremely deep nesting or many siblings, the unbuffered accumulation of sizes via `atomic.AddInt64` could theoretically grow large. However, atomics are fixed-size, so this is actually safe. The real concern is the `batch` slice approach in `api-stress-test/cmd/root.go` (line 316) which accumulates results in memory before flushing. For very high concurrency or long-duration tests, this could consume significant memory.
- Files: `api-stress-test/cmd/root.go`, `check-folder-size/internal/scanner/scanner.go`
- Impact: Potential out-of-memory on very large datasets or stress tests (1M+ requests with small batch size)
- Fix approach: Add configurable batch size limits and implement periodic memory checks. Monitor heap allocation during testing.

**ANSI Color Code Table Column Misalignment (Partial Fix)**
- Issue: Color codes add invisible bytes that break table alignment in `check-folder-size/internal/ui/printer.go` (lines 21-22, 94). The recent fix (commit 17b2804) addressed the main display issue but the underlying approach of embedding ANSI codes in strings makes future alignment problems likely.
- Files: `check-folder-size/internal/ui/printer.go`
- Impact: Table columns may misalign if terminal width calculations are done wrong or if new columns are added
- Fix approach: Refactor color output to strip ANSI codes before width calculations. Use a helper function that measures string length excluding ANSI sequences, or consider using width-aware formatting library.

**Reflection-Based Struct Mapping Without Validation**
- Issue: `common-module/utils/struct_utils.go` uses reflection to map struct fields but doesn't validate that destination fields are exported (uppercase). It also doesn't handle nested structs or type conversions.
- Files: `common-module/utils/struct_utils.go` (lines 9-52, 54-80)
- Impact: Silent failures if destination fields aren't exported; unexpected behavior with type mismatches
- Fix approach: Add validation for exported fields, add proper error messages, consider adding optional type coercion support.

## Known Bugs

**File Processing: Binary File Detection Double-Read**
- Symptoms: In `find-content/searcher.go`, binary detection reads a 512-byte preview (line 179), then seeks back to start (line 187). However, for multiline search, `searchInFileMultiline` re-reads the entire file (line 225), duplicating work. The binary check at line 234 happens on already-read content, so it's correct, but inefficient.
- Files: `find-content/searcher.go` (lines 162-190, 224-236)
- Trigger: Running find-content with multiline search on large text files
- Workaround: Pre-detect binary status before multiline mode

**Backup File Cleanup on Error Not Guaranteed**
- Symptoms: In `replace-text/main.go`, if multiple errors occur during processing (e.g., permission denied on some files), backup files `.bak` are created for successfully modified files. If the process crashes after creating a backup but before completion, orphaned `.bak` files remain.
- Files: `replace-text/main.go` (lines 66-120)
- Trigger: Process crash or kill signal after backup creation but before completion
- Workaround: Manual cleanup of `.bak` files or use transaction log to track created backups

## Security Considerations

**Symlink Following in Directory Walk**
- Risk: While `check-folder-size/internal/scanner/scanner.go` (line 111) correctly skips symlinks, other tools like `find-content/searcher.go` and `replace-text/main.go` do not explicitly exclude symlinks. An attacker could create symlink loops or point to sensitive files.
- Files: `find-content/searcher.go` (lines 293-414), `replace-text/main.go` (lines 150-182), `find-everything/internal/finder/finder.go`
- Current mitigation: `filepath.WalkDir` doesn't follow symlinks by default in Go 1.16+
- Recommendations: Add explicit symlink detection and skip with warning in all directory walkers. Document this behavior in help text.

**Command Injection via Headers/Form Data Parsing**
- Risk: `api-stress-test/internal/request/client.go` parses headers and form data by splitting on delimiters (`;` for headers, `&` for data). If used in a shell context without proper escaping, malicious input could be injected. However, since the parsed values are only used as HTTP headers/body, not executed, risk is low.
- Files: `api-stress-test/internal/request/client.go` (lines 27-59, 61-95)
- Current mitigation: Values are only used as HTTP data, not executed
- Recommendations: Document that CLI arguments should be properly shell-escaped. Consider validating header names to match RFC standards.

**No Input Validation on File Path Filters**
- Risk: `find-content/searcher.go` accepts exclude patterns as regex (line 62 in finder). A maliciously crafted regex could cause ReDoS (Regular Expression Denial of Service).
- Files: `find-content/searcher.go` (lines 35-74), `find-everything/internal/finder/finder.go` (lines 61-69)
- Current mitigation: Regex compilation will fail fast for invalid patterns, preventing ReDoS on compile
- Recommendations: Add timeout on regex matching operations, or use a regex library with built-in ReDoS protection.

## Performance Bottlenecks

**Linear Search in case-converter Normalization**
- Problem: `case-converter/main.go` (lines 252-290) uses `normalizeText()` that tries multiple case-detection approaches sequentially, including a final fallback loop (line 276-288) that tries conversions until one succeeds. For unknown inputs, this means 4-5 conversion operations per input.
- Files: `case-converter/main.go` (lines 252-290)
- Cause: No memoization or efficient case detection algorithm
- Improvement path: Cache case-detection results, or use a more robust detection algorithm (check for specific patterns in order)

**Inefficient Line Number Calculation in Multiline Search**
- Problem: `find-content/searcher.go` (lines 277-288) recalculates line numbers from position zero for each match via `strings.Count(content[lastPos:pos.start], "\n")`. For files with many matches, this is O(n*m) where m is number of matches.
- Files: `find-content/searcher.go` (lines 277-288)
- Cause: Sequential processing of matches without caching newline positions
- Improvement path: Pre-compute newline positions with a slice/map, or use a more efficient line counting approach (e.g., binary search on sorted newline positions)

**Repeated Syscalls in File Stat Operations**
- Problem: `replace-text/main.go` (line 263) calls `os.Stat` on every file when building the replacement list. Then during processing, `processFile` (line 28) calls `os.Stat` again. This is redundant.
- Files: `replace-text/main.go` (lines 26-124, 150-182)
- Cause: File info not cached from initial walk
- Improvement path: Cache stat results from initial walk, pass them to processFile

**Terminal Width Recalculation on Every Progress Update**
- Problem: `check-folder-size/internal/ui/printer.go` doesn't cache terminal width, so every format call might recalculate. Less critical since `check-folder-size/internal/scanner/scanner.go` (line 84) only calls it once, but `check-folder-size/internal/ui/printer.go` (lines 58-63) is called per-item.
- Files: `check-folder-size/internal/ui/printer.go` (lines 58-63)
- Cause: No memoization of terminal width
- Improvement path: Cache width result (already done in scanner, but not in printer). Consider making printer stateful.

## Fragile Areas

**High-Concurrency Race in Stats Collector**
- Files: `api-stress-test/internal/stats/collector.go`
- Why fragile: The `Record()` method (line 41) acquires a mutex for every single request result. Under very high concurrency (1000+ requests/sec), this becomes a bottleneck. The `latencies` slice is appended to under lock (line 45), which causes repeated allocations. While safe, this pattern doesn't scale well for duration-mode tests with unlimited requests.
- Safe modification: Pre-allocate larger latency slices, or use a lock-free ring buffer for latency recording
- Test coverage: `api-stress-test/internal/stats/collector_test.go` has basic tests but no concurrency stress tests

**Depth-First Walk Recursion in Check-Folder-Size**
- Files: `check-folder-size/internal/scanner/scanner.go` (lines 144-158)
- Why fragile: The inline processing fallback (line 155) can recurse deeply if task buffer fills. While documented as bounded by PATH_MAX (~2048 levels), very deep directory trees near that limit could cause stack overflow on systems with smaller stacks. Go defaults to 1GB per goroutine, so this is unlikely, but on embedded systems or custom configurations, it could fail.
- Safe modification: Increase task channel buffer size based on available memory, or implement explicit depth limit check
- Test coverage: No tests for deeply nested directories

**Reflection-Heavy Struct Utils with No Error Handling**
- Files: `common-module/utils/struct_utils.go` (lines 9-52)
- Why fragile: The function silently skips non-matching fields. If a developer expects a field to be copied and it isn't (due to type mismatch or unexported field), the bug is silent. No return value indicates what was actually mapped.
- Safe modification: Return a map of field names that were successfully mapped, or return an error if any expected fields weren't mapped
- Test coverage: No tests for this utility

## Test Coverage Gaps

**No E2E Tests for Multi-File Operations**
- What's not tested: `replace-text` directory processing with failures partway through; backup recovery; concurrent file writes
- Files: `replace-text/main.go`
- Risk: Subtle bugs in error handling during large replacements could go unnoticed
- Priority: Medium

**No Stress Tests for Duration-Mode in api-stress-test**
- What's not tested: Long-running tests (10+ minutes) with duration flag; memory usage over time; rate limiter accuracy over extended periods
- Files: `api-stress-test/cmd/root.go` (lines 176-430)
- Risk: Memory leaks or performance degradation over time could be missed
- Priority: High (duration mode is a recently added feature per CLAUDE.md)

**No Tests for Edge Cases in File Size Formatting**
- What's not tested: Boundary sizes (0 bytes, exactly 1024 bytes, etc.); very large sizes (PB+); negative sizes (shouldn't occur but not validated)
- Files: `check-folder-size/internal/ui/printer.go` (lines 26-51)
- Risk: Display bugs at boundary conditions
- Priority: Low

**No Tests for Symlink Handling in Directory Walkers**
- What's not tested: Symlink loops, symlinks to sensitive directories, symlinks in exclude lists
- Files: `check-folder-size/internal/scanner/scanner.go`, `find-content/searcher.go`, `find-everything/internal/finder/finder.go`
- Risk: Potential for infinite loops or unintended file access
- Priority: Medium

**Limited Tests for Regex Validation**
- What's not tested: Complex regex patterns, ReDoS-prone patterns, empty patterns
- Files: `find-content/searcher.go` (lines 35-74), `find-everything/internal/finder/finder.go` (lines 61-69)
- Risk: User input crashes or hangs from malicious regex
- Priority: Medium

## Dependencies at Risk

**Deprecated Golang X Libraries**
- Risk: `golang.org/x/text` (used in `case-converter/main.go` line 11) and `golang.org/x/term` (used in `check-folder-size/internal/scanner/scanner.go` line 12) are experimental. While stable, they're not part of stdlib and may change.
- Impact: Future Go versions might deprecate or remove these
- Migration plan: `golang.org/x/term` can be replaced with `syscall.ForkExec` on Linux or use `os.Isatty()` with manual terminal size detection. `golang.org/x/text/cases` could be replaced with standard string functions for basic title case.

## Missing Critical Features

**No Timeout/Cancellation for File Operations**
- Problem: `replace-text` and `find-content` directory walks don't support context cancellation or timeouts. A very large directory or permission-denied scenarios could hang indefinitely.
- Blocks: Long-running operations on large filesystems, integration into systems with timeout requirements
- Fix approach: Add `context.Context` parameter to all directory walking functions, pass `context.WithTimeout` from CLI layer

**No Progress Indication for find-content and replace-text**
- Problem: Unlike `check-folder-size` and `find-everything`, these tools don't show progress on large operations
- Blocks: User feedback on long-running operations
- Fix approach: Add `--progress` flag, implement simple counter output

**No Resume/Partial Completion for replace-text**
- Problem: If replace-text crashes on a large directory, you must restart and reprocess all files (or manually track which ones failed)
- Blocks: Practical use on very large codebases where interruptions are common
- Fix approach: Add optional completion log file, check before processing to skip already-processed files

## Scaling Limits

**HTTP Rate Limiter Resolution: 1ms Minimum**
- Current capacity: `api-stress-test/internal/request/ratelimiter.go` (line 19) calculates interval as `time.Duration(float64(time.Second) / rps)`. For very high RPS (e.g., 100k req/s), interval becomes 10µs, but time.Ticker has ~1ms resolution on most systems.
- Limit: Practical max is ~1000 req/s per limiter with accurate timing
- Scaling path: For higher rates, implement token bucket with fine-grained timing or use multiple rate limiters

**Collector Mutex Contention Under High Concurrency**
- Current capacity: ~10k results/sec on single mutex (rough estimate)
- Limit: 100k+ concurrent requests create severe lock contention
- Scaling path: Implement lock-free circular buffer for latency recording, batch statistics updates, or use per-worker sub-collectors

**Directory Walker Task Buffer Size**
- Current capacity: `buffer = numWorkers * 4` (line 66 in scanner.go), so typically 32-128 tasks
- Limit: For directories with 10k+ immediate children, buffer fills quickly
- Scaling path: Add `--task-buffer-size` flag, or dynamically adjust based on directory contents

---

*Concerns audit: 2026-03-12*

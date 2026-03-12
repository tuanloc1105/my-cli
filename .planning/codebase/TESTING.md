# Testing Patterns

**Analysis Date:** 2026-03-12

## Test Framework

**Runner:**
- Go standard `testing` package (1.24.4)
- No test framework (e.g., Ginkgo, testify) imported
- Config: None detected (uses Go defaults)

**Assertion Library:**
- None used; manual assertions with `if condition { t.Error(...) }`
- Standard error report: `t.Errorf(fmt, args)` for non-fatal failures
- Fatal assertions: `t.Fatalf()` when test cannot continue

**Run Commands:**
```bash
go test ./...              # Run all tests in current tool
go test -v ./...           # Verbose output
go test -run TestName      # Run specific test
go test -count=1 ./...     # No caching, run all tests
```

## Test File Organization

**Location:**
- Co-located: `*_test.go` files live alongside implementation files
- Examples:
  - `api-stress-test/cmd/root_test.go` → tests `cmd/root.go`
  - `api-stress-test/internal/stats/collector_test.go` → tests `collector.go`
  - `api-stress-test/internal/request/client_test.go` → tests `client.go`

**Naming:**
- Filename: `{source}_test.go` (e.g., `collector_test.go`)
- Test function: `Test{FunctionName}(t *testing.T)` (e.g., `TestCollectorRecord`)
- Subtests: `t.Run(name, func(t *testing.T) { ... })`

**Structure:**
Current test coverage (as of exploration):
```
api-stress-test/          # Only modular tool with tests
├── cmd/
│   └── root_test.go      # 59 lines: ValidateURL, ValidateMethod
├── internal/
│   ├── request/
│   │   ├── client_test.go      # 259 lines: ParseHeaders, ParseData, PrepareBody
│   │   └── ratelimiter_test.go # 101 lines: RateLimiter behavior
│   └── stats/
│       └── collector_test.go   # 171 lines: Record, Min/Max/Avg, Error tracking
```

**Other tools have no tests:**
- `case-converter/` — No tests
- `check-folder-size/` — No tests
- `find-content/` — No tests
- `find-everything/` — No tests
- `replace-text/` — No tests
- `common-module/` — No tests

## Test Structure

**Suite Organization:**
Single-function tests with inline subtests. Example from `cmd/root_test.go`:
```go
func TestValidateURL(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{"valid http", "http://example.com", false},
		{"valid https", "https://example.com/path?q=1", false},
		// ...
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateURL(tt.url)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateURL(%q) error = %v, wantErr %v", tt.url, err, tt.wantErr)
			}
		})
	}
}
```

**Patterns:**
- **Table-driven tests**: Slice of structs with test cases
  - Fields: `name`, `input`/`expected`, `wantErr`, `wantNil`, `want*`
  - Each case runs via `t.Run(tt.name, ...)`
- **Setup**: Minimal; no fixtures or test databases
  - Direct instantiation: `c := NewCollector(10)`
  - Temp dirs: `dir := t.TempDir()` for file I/O tests
- **Teardown**: None used; `t.TempDir()` auto-cleans
- **Assertion style**: Manual condition checks
  ```go
  if stat.Total != 4 {
      t.Errorf("total = %d, want 4", stat.Total)
  }
  ```

## Mocking

**Framework:**
- No mocking library (no `testify/mock`, `golang.fuzz.dev`, etc.)
- Tests use real implementations or test-specific helpers

**Patterns:**
No file I/O mocking observed. Tests use `t.TempDir()` for real temp files.

Example from `request/client_test.go`:
```go
t.Run("json file", func(t *testing.T) {
    dir := t.TempDir()
    path := filepath.Join(dir, "test.json")
    os.WriteFile(path, []byte(`{"test":true}`), 0644)

    body, ct, err := PrepareBody("", path, nil, "", "", "")
    // assertions...
})
```

**What to Mock:**
- Nothing explicitly mocked; external calls not tested
- HTTP client not mocked (no e2e tests included; unit tests only)
- System commands not mocked (when needed, tested via real execution)

**What NOT to Mock:**
- Core logic (stats calculation, string parsing) — tested fully
- File I/O — use temp files instead of mocks
- Concurrency primitives — test real goroutines/channels

## Fixtures and Factories

**Test Data:**
Hard-coded test cases in table structs. Example from `stats/collector_test.go`:
```go
func TestCollectorRecord(t *testing.T) {
	c := NewCollector(10)

	c.Record(200, 0.1, true, "")
	c.Record(200, 0.2, true, "")
	c.Record(500, 0.3, false, "server error")
	c.Record(0, 0.05, false, "connection refused")
	// ...
}
```

No shared fixtures or factory functions. Each test creates its own instances.

**Location:**
- Inline within test functions
- No separate `testdata/` directory
- No `fixtures.go` or helper files for test setup

## Coverage

**Requirements:**
- No coverage targets enforced
- No `.coverprofile` check in CI/CD
- No coverage badge or report

**View Coverage:**
```bash
go test -cover ./...            # Summary per package
go test -coverprofile=cover.out ./...
go tool cover -html=cover.out   # HTML report
```

**Current state:**
- Estimated coverage: ~30% of codebase
  - `api-stress-test`: ~40–50% (cmd, internal/stats, internal/request covered)
  - All other tools: 0% (untested)

## Test Types

**Unit Tests:**
- Scope: Single function in isolation
- Approach: Table-driven; assert outputs match expected values
- Example: `TestCollectorRecord` tests the `Record()` method with various inputs
- No mocking: All dependencies real

**Integration Tests:**
- Not used in codebase
- No multi-module or cross-service tests

**E2E Tests:**
- Not used
- No end-to-end CLI test harness

## Common Patterns

**Async Testing:**
Concurrency tested with real goroutines. Example from `stats/collector_test.go`:
```go
func TestCollectorConcurrency(t *testing.T) {
	c := NewCollector(1000)
	var wg sync.WaitGroup

	numGoroutines := 10
	recordsPerGoroutine := 100

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < recordsPerGoroutine; j++ {
				c.Record(200, 0.1, true, "")
			}
		}()
	}

	wg.Wait()
	stat := c.GetStatistics()

	expected := numGoroutines * recordsPerGoroutine
	if stat.Total != expected {
		t.Errorf("total = %d, want %d", stat.Total, expected)
	}
}
```

**Error Testing:**
Test both success and failure paths. Example from `request/client_test.go`:
```go
t.Run("no body sources", func(t *testing.T) {
    body, ct, err := PrepareBody("", "", nil, "", "", "")
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if body != nil || ct != "" {
        t.Errorf("expected nil body and empty content type, got body=%v ct=%q", body, ct)
    }
})

t.Run("invalid json string", func(t *testing.T) {
    _, _, err := PrepareBody("{invalid", "", nil, "", "", "")
    if err == nil {
        t.Fatal("expected error for invalid JSON")
    }
})
```

**Floating-point tolerance:** Used for latency comparisons.
Example from `stats/collector_test.go`:
```go
if diff := stat.AvgLatency - expected; diff > 0.0001 || diff < -0.0001 {
    t.Errorf("avg latency = %f, want %f", stat.AvgLatency, expected)
}
```

## Test Execution

**Location of tests:**
- All in `api-stress-test` tool
- Other tools require test coverage to be added

**Running tests:**
```bash
cd /home/loc/git/my-cli/api-stress-test
go test ./...        # Run all tests
go test -v ./...     # Verbose output
```

**Test counts:**
- Total test functions: 17
- Total test lines: ~590 lines across 4 files
- Average test size: ~35 lines per test function

---

*Testing analysis: 2026-03-12*

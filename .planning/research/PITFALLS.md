# Domain Pitfalls: Go HTTP Stress Testing Tool Optimization

**Domain:** Go concurrent HTTP load testing tool (api-stress-test)
**Researched:** 2026-03-27
**Scope:** Performance optimization and bug fixes for high-concurrency (1000+ workers)

---

## Critical Pitfalls

Mistakes that cause rewrites, silent data corruption, or major performance regressions.

---

### Pitfall 1: Single Mutex Guarding All Collector State

**What goes wrong:** The current `Collector` uses one `sync.Mutex` protecting every field — counters, reservoir, maps, min/max. Every worker goroutine must acquire this same lock to call `Record()`. At 1000+ concurrency, goroutines queue behind the lock. Lock wait time grows linearly with worker count, eventually becoming the dominant cost. Throughput plateaus and reported req/sec is artificially capped by the collector, not the network.

**Why it happens:** Single-mutex is the correct starting point; it is safe and easy to reason about. The contention only emerges under load that the author may not have benchmarked during initial development.

**Consequences:**
- Tool measures its own contention, not the target server's capacity
- CPU time wasted in kernel futex wait, not in useful HTTP work
- Goroutines pile up in scheduler run queue, increasing GC pressure
- Reported latency percentiles are inflated by lock-wait time included in elapsed measurement? No — elapsed is measured in the worker before calling Record, so latency is accurate. But throughput (req/sec) is throttled by collector lock.

**Prevention:**
- Separate counters that only need atomic increments (`successes`, `failures`, `totalCount`, `latencySum`, `totalResponseSize`) from state requiring a lock (reservoir sampling, maps).
- Use `sync/atomic` (`atomic.Int64`, `atomic.Uint64`) for scalar counters.
- Keep a single mutex only for the reservoir slice and the maps (`statusCount`, `errorMessages`, `throughput`).
- Alternative: shard the collector into N shards (where N = runtime.NumCPU()) — each worker picks shard by goroutine ID or round-robin, reducing contention by ~N-fold.
- Merge shards only at `GetStatistics()` time (called once, not in the hot path).

**Detection:**
- `go tool pprof` mutex profile shows `sync.(*Mutex).Lock` dominating CPU time
- Throughput does not scale past ~50-100 workers despite more concurrency
- `-cpuprofile` shows `runtime.lock2` or `runtime.semacquire` as top callers

**Phase:** Fix in the Collector mutex contention phase (Phase 1).

---

### Pitfall 2: Signal Re-Enqueue Race in Warm-Up to Main Phase Transition

**What goes wrong:** The current code sends a re-enqueued signal back into `sigChan` using `select { case sigChan <- sig: default: }`. The `default` branch silently drops the signal if the channel is full (capacity 1, already occupied). Between `warmWg.Wait()` returning and the main-phase signal goroutine entering its `select`, there is a window where the signal has been dropped and nobody is listening. The main test runs to completion instead of aborting on Ctrl-C.

**Code location:** `cmd/root.go` lines 303-308.

**Why it happens:** The pattern of re-enqueueing signals across phase transitions is non-standard. Standard Go signal handling assumes a single consumer. When code tries to relay a signal between two sequential phases, timing gaps are unavoidable with this approach.

**Consequences:**
- User presses Ctrl-C during warm-up; warm-up stops correctly, but the full test then runs to completion — the opposite of intended behavior.
- Silent: no error, no warning. The user does not know the signal was dropped.
- Non-deterministic: only fails depending on goroutine scheduling timing.

**Prevention:**
- Use a dedicated `aborted` channel or `atomic.Bool` flag set by the signal handler, not signal re-enqueueing. Both phases check the same flag.
- Alternatively, keep a single signal goroutine alive for the entire test lifetime and use a `context.WithCancel` that is shared across both phases:
  ```go
  rootCtx, rootCancel := context.WithCancel(context.Background())
  go func() {
      select {
      case <-sigChan:
          rootCancel()
      case <-rootCtx.Done():
      }
  }()
  // warmup ctx derives from rootCtx; main ctx also derives from rootCtx
  warmCtx, warmCancel := context.WithTimeout(rootCtx, opts.Warmup)
  ```
  When rootCtx is cancelled, both phase contexts are cancelled automatically — no relay needed.

**Detection:**
- `go test -race` does not catch this because the race is between two separate sequential goroutines — it is a logic race, not a memory race.
- Manual test: run with `--warmup 5s` and press Ctrl-C within the first 2 seconds; verify that the main test does NOT start.

**Phase:** Fix in the signal handling / phase orchestration phase (Phase 1).

---

### Pitfall 3: sort + copy Under Lock in GetStatistics

**What goes wrong:** `GetStatistics()` acquires `c.mu.Lock()` and then calls `sort.Float64s(sorted)` on a copy of up to 10,000 float64 values while holding the lock. On a 10,000-sample reservoir this is an O(N log N) operation under contention. Any worker calling `Record()` during this time blocks for the entire sort duration (~1-3ms on a modern CPU for 10K elements).

**Why it happens:** It is easier to hold the lock for the entire computation. `GetStatistics` is called once at the end of the test, so the developer may not notice the cost during normal testing.

**Consequences:**
- In duration mode, if progress display or live stats ever call `GetStatistics` mid-run, they create a multi-millisecond lock hold that stalls all 1000+ workers.
- Even at end-of-run: the final drain of the results channel happens while workers may still be finishing, and `Record` calls will block during sort.
- The copy of the reservoir slice (`make([]float64, len(c.reservoir))`) + `copy()` allocates on the heap under the lock, adding GC pressure.

**Prevention:**
- Copy the reservoir outside the lock (take a snapshot of the slice header length, then release the lock before sorting):
  ```go
  c.mu.Lock()
  snapshot := make([]float64, len(c.reservoir))
  copy(snapshot, c.reservoir)
  // copy all other fields needed for calculation
  c.mu.Unlock()
  sort.Float64s(snapshot) // sort without holding the lock
  ```
- The lock is only held for the O(N) copy, not the O(N log N) sort.
- For future live-stats display: cache the last computed percentiles with a dirty flag to avoid repeated sorts.

**Detection:**
- Mutex profile shows `GetStatistics` holding lock for disproportionate time
- Worker latency spikes at exactly the end of a test run

**Phase:** Fix alongside Collector mutex contention work (Phase 1).

---

## Moderate Pitfalls

Mistakes that cause incorrect results, memory bloat, or measurable performance degradation.

---

### Pitfall 4: Unbounded errorMessages Map Growth

**What goes wrong:** The `errorMessages map[string]int` in `Collector` grows without bound. Every distinct error string creates a new map entry. With the current `normalizeError()` function, most errors are normalized to ~10 categories. But the normalization has a fallthrough: errors longer than 80 characters get truncated, and errors not matching any pattern are stored verbatim (up to 80 chars). Under diverse failure conditions (many different remote IPs, ports, TLS certificates), the number of distinct error strings can be large.

**Why it happens:** Maps in Go do not shrink after entries are added. Once added, entries consume memory for the lifetime of the collector.

**Consequences:**
- Memory grows O(unique error types) — bounded in practice by normalizeError, but not by design.
- If a server returns dynamic error messages (e.g., including request IDs or timestamps), the map grows O(total requests).
- Map iteration in `GetStatistics` for `topErrors` is O(N) in the number of distinct errors.

**Prevention:**
- Cap the map at a maximum of N distinct entries (e.g., 100). When the cap is reached, new distinct errors are accumulated into a catchall "other errors" bucket:
  ```go
  const maxErrorTypes = 100
  if _, exists := c.errorMessages[errorMsg]; !exists && len(c.errorMessages) >= maxErrorTypes {
      c.errorMessages["(other)"]++
  } else {
      c.errorMessages[errorMsg]++
  }
  ```
- This bounds map memory regardless of server behavior.

**Detection:**
- `runtime.MemStats.HeapAlloc` growing during a run against an endpoint that returns varied error messages
- `len(c.errorMessages)` >> 20 after a test run

**Phase:** Fix in the same Collector pass (Phase 1). Low effort, high protection.

---

### Pitfall 5: Batch Size Formula Produces Batch=1 at Low Concurrency

**What goes wrong:** `batchSize := max(1, opts.Concurrency/2)`. At `concurrency=2`, `batchSize=1`. Batching of 1 provides no benefit — it adds the overhead of `batch = append(batch, res)` and `batch = batch[:0]` for every single result. Worse, it breaks the intended pattern of amortizing `collector.Record()` calls.

**Why it happens:** Integer division: `2/2 = 1`. The formula intended to give "half the concurrency" but does not account for very low values.

**Consequences:**
- At concurrency 2, batching loop processes results one at a time — same as no batching, but with more allocations.
- The benefit of batching (reduced lock-acquisition frequency on the Collector) is lost at the configuration most commonly used for testing and debugging the tool itself.

**Prevention:**
- Use a sensible minimum batch size: `batchSize := max(10, opts.Concurrency/2)` or decouple batch size from concurrency entirely (e.g., fixed at 16 or 32).
- Alternatively, only batch when `concurrency >= threshold`, otherwise write directly to the collector.

**Detection:**
- `pprof` CPU profile showing excessive time in the batch processing loop
- Comparing throughput at concurrency=2 with and without batching shows no difference

**Phase:** Fix in Phase 1 (correctness fix, low risk).

---

### Pitfall 6: http.Transport Misconfiguration at Extreme Concurrency

**What goes wrong:** The current transport sets `MaxIdleConns = opts.Concurrency` and `MaxIdleConnsPerHost = opts.Concurrency`. This is correct in intent but has a subtle asymmetry: `MaxIdleConns` is the global pool size, `MaxIdleConnsPerHost` is per-host. When both equal concurrency and all requests go to one host (typical for a stress test), the effective limit is `MaxIdleConnsPerHost`. This is correct. However, `IdleConnTimeout` is hardcoded at 90 seconds regardless of test duration. For a 5-second test, connections stay in the idle pool for 90 seconds after the test completes — not harmful but wastes OS resources.

**Separate concern:** `ResponseHeaderTimeout` and `TLSHandshakeTimeout` are not set on the Transport. The only timeout is `http.Client.Timeout`, which is a total request timeout. If a server accepts the TCP connection but never sends response headers, the client waits for the full `Timeout` duration (default 5s) per worker. At 1000 workers this is 1000 * 5s = 5000 goroutine-seconds of blocked time if the server hangs.

**Why it happens:** `http.Client.Timeout` appears sufficient; the granular Transport timeouts are easy to overlook.

**Consequences:**
- Hung server causes all workers to block until full timeout, giving a misleading "test completed but took 5s per request" picture.
- Transport-level timeout fields (`DialContext`, `TLSHandshakeTimeout`) default to zero (no timeout at the transport layer), relying entirely on `http.Client.Timeout`.

**Prevention:**
- Add `TLSHandshakeTimeout: opts.Timeout` to the Transport (already bounded by client timeout, but prevents TLS stalls from consuming a whole timeout slot silently).
- Add `ResponseHeaderTimeout: opts.Timeout` to ensure header stalls are caught promptly.
- Set `IdleConnTimeout` to `min(90*time.Second, opts.Duration+10*time.Second)` for duration mode.
- Document why `MaxIdleConns == MaxIdleConnsPerHost`: it is intentional because stress tests target a single host.

**Detection:**
- Test against a server that delays headers by 10 seconds with a 5-second client timeout: workers should time out, not hang indefinitely.
- `netstat` shows many connections in `ESTABLISHED` state with no data transferred.

**Phase:** Phase 2 (transport tuning and documentation). Lower priority than Collector contention.

---

### Pitfall 7: Response Body Drain Inconsistency Between expectBody Paths

**What goes wrong:** In `ExecuteRequest`, when `expectBody != ""` the body is read into `respBody []byte` and `responseSize = int64(len(respBody))`. When `expectBody == ""` the body is drained into `io.Discard` and `responseSize` is the count of bytes discarded. These two paths are equivalent in result but differ in the mechanism. The issue is that when `expectBody != ""`, `io.ReadAll(io.LimitReader(resp.Body, maxResponseDrain))` allocates a new byte slice per request. At 1000+ concurrency with large response bodies, this is 1000+ heap allocations per second, adding GC pressure.

**Why it happens:** `io.ReadAll` always allocates a new buffer. For body validation, the content needs to be in memory, so some allocation is necessary. But using a worker-local reusable buffer (via `sync.Pool`) would eliminate most allocations.

**Consequences:**
- GC pauses increase proportionally with response body size * concurrency.
- Allocator contention (Go's per-P mcache) shows up under very high concurrency.

**Prevention:**
- Use a `sync.Pool` of `[]byte` buffers for body reads in the `expectBody` path.
- Size pool buffers at a reasonable default (e.g., 64KB) and grow if needed.
- The discard path is already allocation-free (io.Discard is a no-op writer).

**Detection:**
- `go test -bench -benchmem` shows high allocs/op when `expectBody` is set
- `pprof heap` profile shows `io.ReadAll` dominating allocations during high-concurrency runs

**Phase:** Phase 2 (memory allocation optimization).

---

## Minor Pitfalls

Lower-impact issues that are worth fixing but not blocking.

---

### Pitfall 8: Benchmark Anti-Patterns in Go 1.24 Context

**What goes wrong:** Existing benchmarks (if any are added) using the classic `for i := 0; i < b.N; i++` loop pattern instead of the Go 1.24 `b.Loop()` API miss automatic timer management and compiler dead-code elimination protection.

**Specific risks in this codebase:**
- Benchmarking `Collector.Record()` by pre-creating a collector with a fixed seed may measure a sorted reservoir on later iterations (if the same latency values are used repeatedly), skewing results toward faster paths.
- Benchmarking `GetStatistics()` without resetting the collector between iterations measures incremental cost, not per-call cost.
- Not calling `b.ReportAllocs()` hides that hot-path functions allocate unexpectedly.

**Prevention:**
- Use `b.Loop()` (Go 1.24+) instead of `for range b.N` in all new benchmarks.
- In `Collector` benchmarks: create a fresh collector per `b.N` iteration or use `b.StopTimer()`/`b.StartTimer()` around reset.
- Always add `b.ReportAllocs()` to hot-path benchmarks.
- Use `runtime.KeepAlive(result)` or package-level sink variables to prevent compiler dead-code elimination of benchmark targets.

**Detection:**
- Benchmark results that are implausibly fast (< 1ns/op for operations known to acquire locks)
- `go test -bench=. -benchmem` showing 0 allocs/op for operations that clearly allocate

**Phase:** Phase 3 (benchmarking and validation). Address before measuring any before/after comparisons.

---

### Pitfall 9: Channel Buffer Sizing at Extreme Concurrency

**What goes wrong:** Both `jobs` and `results` channels are sized at `opts.Concurrency * 2`. At `concurrency=10000` (the maximum allowed), this creates two channels with 20,000 slots each. Each channel slot holds one `struct{}` (jobs) or one `request.Result` (results). `request.Result` contains a `string` (Error field), so each slot is ~40 bytes on 64-bit. Two channels at 20K slots = 40K * 40 bytes = ~1.6 MB just in channel buffers, before any goroutine stacks.

**Why it happens:** `concurrency * 2` is a reasonable rule of thumb for keeping workers fed, but it does not account for pathological concurrency values.

**Consequences:**
- At 10K concurrency, channel buffer memory alone exceeds 1MB.
- The jobs channel at `concurrency*2` creates a burst capacity that allows the job feeder to race ahead of workers, holding up to `concurrency*2` pending jobs in memory. For large concurrency, this delays backpressure to the feeder goroutine.
- Oversized result channel: if workers produce results faster than the single consumer processes them, the buffer may fill, but the size `concurrency*2` is already generous.

**Prevention:**
- Cap channel buffers: `min(opts.Concurrency*2, 4096)` is a reasonable upper bound that provides adequate buffering without unbounded memory at extreme concurrency.
- For the jobs channel specifically, a smaller buffer (e.g., `concurrency` or even `concurrency/4`) is sufficient because workers process jobs in O(milliseconds) (network round-trip).

**Detection:**
- `runtime.MemStats.HeapAlloc` at test startup much higher than expected
- `go tool pprof -alloc_space` shows channel buffer allocation as a top contributor

**Phase:** Phase 2 (memory overhead reduction).

---

### Pitfall 10: fmt.Sprintf in normalizeError Truncation Path

**What goes wrong:** In `normalizeError()`, the default case calls `msg[:80] + "..."` — this is a string concatenation that allocates a new string on the heap. This path is only reached for errors that are not matched by any normalization case. In practice this may be rare, but it occurs in the hot path (called per request for every failed request).

**Separate concern:** `fmt.Sprintf("panic: %v", r)` in the worker panic recovery creates a formatted string via `fmt.Sprintf`. The `interface{}` argument forces an allocation. Under normal operation panics do not occur, but if they do at high concurrency, this adds allocation pressure.

**Prevention:**
- For the truncation path: `msg[:80] + "..."` is already reasonably efficient (two-string concatenation). It can stay as-is unless profiling shows it as a hotspot.
- The `fmt.Sprintf("expected status %d, got %d", expectStatus, statusCode)` in `ExecuteRequest` runs per-request when `expectStatus` is set and fails. This is unavoidable for user-visible messages but could use `strconv.AppendInt` into a pre-allocated buffer if it shows up in profiles.
- Rule: do not add new `fmt.Sprintf` calls in paths that execute on every request under all conditions.

**Detection:**
- `go test -bench . -benchmem` with `expectStatus` set to a value that never matches: shows allocs/op from error message formatting.

**Phase:** Defer unless profiling surfaces it. Low impact in practice.

---

## Phase-Specific Warnings

| Phase Topic | Likely Pitfall | Mitigation |
|---|---|---|
| Collector mutex contention fix | Risk of introducing data races when splitting atomic vs. mutex-guarded fields | Use `-race` flag during all testing; verify atomic fields are only accessed via `sync/atomic` methods, never with `=` assignment |
| Sharding the Collector | Merge-at-read must produce the same results as single-mutex; histogram and reservoir sampling change semantics | Write a property test that verifies merged-shard stats equal single-collector stats on identical input |
| Signal handling refactor | Introducing a shared `rootCtx` across phases must not make the warm-up context cancellation affect the main test | Verify with integration test: rootCtx cancel must propagate; warm-up timeout must not propagate to main |
| Percentile sort outside lock | If reservoir is still modified while sort runs, results are non-deterministic | The snapshot copy (under lock) then sort (outside lock) pattern is safe only if the copy is complete before the lock is released — verify with `-race` |
| Transport timeout additions | `ResponseHeaderTimeout` interacts with `http.Client.Timeout` — the shorter of the two wins; do not set Transport timeout > client timeout | Add a validation check: if `ResponseHeaderTimeout` > `opts.Timeout`, warn or clamp |
| Benchmark-driven validation | Benchmarking the Collector in isolation does not capture real-world contention from concurrent goroutines | Use `b.RunParallel` for Collector benchmarks to simulate concurrent `Record()` calls |
| Channel buffer reduction | Reducing buffer size increases the probability of goroutine scheduling delay if the consumer (result processor) is slow | Profile with `--trace` and verify channel send latency does not increase after buffer reduction |

---

## Sources

- [Go Mutex Contention Issue #33747 (golang/go)](https://github.com/golang/go/issues/33747) — documents mutex performance collapse under high concurrency
- [False Sharing in Go — DEV Community](https://dev.to/kelvinfloresta/false-sharing-in-go-the-hidden-enemy-in-your-concurrency-37ni) — cache line alignment and false sharing
- [Struct Field Alignment — goperf.dev](https://goperf.dev/01-common-patterns/fields-alignment/) — HIGH confidence, current documentation
- [Atomic Operations and Synchronization Primitives — goperf.dev](https://goperf.dev/01-common-patterns/atomic-ops/) — HIGH confidence, current documentation
- [Worker Pools — goperf.dev](https://goperf.dev/01-common-patterns/worker-pool/) — HIGH confidence, current documentation
- [Common pitfalls in Go benchmarking — Eli Bendersky](https://eli.thegreenplace.net/2023/common-pitfalls-in-go-benchmarking/) — MEDIUM confidence, 2023
- [testing.B.Loop in Go 1.24 — go.dev/blog](https://go.dev/blog/testing-b-loop) — HIGH confidence, official Go blog
- [HTTP Connection Pooling in Go — David Bacisin](https://davidbacisin.com/writing/golang-http-connection-pools-1) — MEDIUM confidence, recent
- [net/http Transport MaxIdleConnsPerHost Issue #13801 (golang/go)](https://github.com/golang/go/issues/13801) — HIGH confidence, official issue tracker
- [fmt.Sprintf vs String Concat — DoltHub Blog, 2024](https://www.dolthub.com/blog/2024-11-08-sprintf-vs-concat/) — MEDIUM confidence, 2024
- [Go Concurrent Maps Sharding — DEV Community](https://dev.to/aaravjoshi/go-concurrent-maps-from-bottlenecks-to-high-performance-sharded-solutions-that-scale-48bk) — MEDIUM confidence
- [os/signal: Notify leaks a goroutine Issue #52619 (golang/go)](https://github.com/golang/go/issues/52619) — HIGH confidence, official issue tracker
- [Maps and Memory Leaks in Go — Hacker News thread](https://news.ycombinator.com/item?id=33516297) — MEDIUM confidence

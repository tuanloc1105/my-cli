# Research Summary: api-stress-test Performance and Correctness Improvements

**Synthesized:** 2026-03-27
**Sources:** STACK.md, FEATURES.md, ARCHITECTURE.md, PITFALLS.md
**Overall Confidence:** HIGH — all four research dimensions are grounded in Go stdlib documentation, direct peer-tool source inspection, and confirmed benchmarks.

---

## Executive Summary

The `api-stress-test` tool has a correct high-level architecture — a job-feeder goroutine, N worker goroutines, a single result-processing goroutine, and a final stats computation — but contains several well-documented anti-patterns that prevent it from scaling past roughly 50-100 concurrent workers. The dominant problem is a single `sync.Mutex` in `Collector.Record()` that serialises every worker on every request, turning the stats collector into the actual throughput bottleneck rather than the network or target server. Three peer tools (vegeta, hey, bombardier) all independently arrived at the same solution: push scalar counters to atomics and remove the mutex from the hot path entirely.

Beyond contention, there is a correctness bug in signal handling: the warm-up to main-phase transition uses `select { case sigChan <- sig: default: }` to re-enqueue a SIGINT, but the `default` branch silently drops the signal when the channel buffer is full. This means a user pressing Ctrl-C during warm-up sees the warm-up stop correctly, then watches the full test run to completion — the opposite of intended behaviour. The fix is a shared root context derived from `signal.NotifyContext` that both phases inherit, eliminating re-enqueue entirely.

All required fixes use Go 1.24 stdlib only (`sync/atomic`, `sync.Pool`, `os/signal`, `net`). No new external dependencies are needed or recommended. The full set of changes is confined to two files — `internal/stats/collector.go` and `cmd/root.go` — with one supporting change to `internal/request/client.go` for body reader pooling.

---

## Stack Recommendations

*From STACK.md — all recommendations carry HIGH confidence, verified against Go stdlib docs and benchmark data.*

| Concern | Recommended Approach | Rationale |
|---------|----------------------|-----------|
| Hot-path scalar counters | `atomic.Int64` for successes, failures, totalCount, totalResponseSize | ~2.5x faster than mutex under contention (~7 ns/op vs ~18 ns/op); available since Go 1.19 |
| Float64 accumulation (latencySum, min, max) | `uint64` field + `math.Float64bits` CAS loop | stdlib has no `atomic.Float64`; this is the canonical workaround (golang/go#21996) |
| Reservoir + maps (statusCount, errorMessages, throughput) | Narrow `sync.Mutex`, scoped only to these fields | Slice append and map write require a lock; cannot be made lock-free without restructure |
| HTTP connection pool | Add `MaxConnsPerHost = concurrency`, `MaxIdleConnsPerHost = concurrency`, explicit `net.Dialer` | Prevents TIME_WAIT explosion at 1000+ workers; `MaxIdleConnsPerHost` defaults to 2 in stdlib |
| Request body allocation | `sync.Pool` for `*bytes.Reader` in `request/client.go` | Eliminates per-request heap allocation at high RPS; `bytes.Reader.Reset()` avoids reallocation |
| Error map growth | Cap `errorMessages` at 50 distinct keys with overflow bucket | Prevents unbounded memory growth under diverse/dynamic error strings |
| Percentile sort | Move `sort.Float64s` outside the mutex in `GetStatistics()` | Removes ~1ms lock hold during sort on a 10K-sample reservoir |
| Signal lifecycle | `signal.NotifyContext` for entire program lifetime; derive phase contexts from root | Eliminates the re-enqueue anti-pattern; `os/signal` docs confirm signals are dropped on full channels |
| Batch size floor | `max(10, concurrency/2)` replacing `max(1, concurrency/2)` | `max(1,...)` evaluates to 1 at concurrency=2, negating batching entirely |

**No new external dependencies.** All patterns use stdlib: `sync/atomic`, `math`, `sync`, `os/signal`, `net`.

**What NOT to use:**
- Sharded Collector with 16+ shards — reservoir sampling requires globally consistent `totalCount`; sharding breaks probability calculation
- `go.uber.org/atomic` for Float64 — adds external dep for 5-line stdlib CAS pattern
- `tdigest` or `hdrhistogram` libraries — adds deps; reservoir sampling at 10K samples already gives sub-1% relative error on p99
- `ForceAttemptHTTP2: true` — breaks HTTP/1.1-only targets; HTTP/2 multiplexing conflicts with per-worker-connection model
- `sync.Map` for statusCount or errorMessages — optimised for stable keys / read-heavy; write-heavy stress test access pattern makes it strictly worse than a mutex-guarded map

---

## Feature Landscape

*From FEATURES.md — patterns cross-verified against vegeta, hey, and bombardier source code.*

### Table Stakes (Must Fix This Milestone)

| Feature | Current Status | Fix Complexity |
|---------|---------------|----------------|
| Atomic scalar counters (success/failure/total/responseSize) | Missing — all behind single mutex | Low |
| Signal handling that doesn't drop on warm-up transition | Buggy (`select/default` drop race) | Medium |
| Correct minimum batch size (floor at 10+) | Buggy (evaluates to 1 at concurrency=2) | Low (one-liner) |
| Bounded error message storage (cap at ~100 keys) | Missing | Low |
| Percentile sort outside mutex | Suboptimal (sort under lock) | Low |
| `MaxConnsPerHost` set equal to concurrency | Missing from transport config | Low (one-liner) |
| Response size tracking consistency (expect-body vs discard paths) | Minor inconsistency | Low |
| Benchmark suite (`BenchmarkCollectorRecord` with `b.RunParallel`) | Not present | Medium |

### Differentiators (Already Present — Preserve)

| Feature | Value | Peer Tool Status |
|---------|-------|-----------------|
| Reservoir sampling (memory-bounded p99) | No external dep; ~80KB fixed memory | Only this tool and gocannon use bounded approach without dep |
| Per-second throughput timeline | Shows ramp-up, saturation, decay | Not in vegeta/hey/bombardier by default |
| Warm-up phase support | Avoids JIT cold-start skew in p99 | Rare in Go load testers |
| Response body validation (`expect-body`, `expect-status`) | Catches CDN caching bugs under load | Uncommon in peer tools |
| JSON output + file output | CI pipeline integration | Standard in mature tools |

### Anti-Features (Explicitly Defer)

- New histogram libraries (tdigest, hdrhistogram) — violates minimal-deps constraint
- Distributed multi-node attack — scope is k6 cloud's domain
- Real-time streaming metrics — per-second timeline in final output already covers this
- Scripted/programmable scenarios (Lua, JS) — different product
- HTTP/2 explicit management — `net/http` already negotiates transparently
- Lock-free data structures from scratch — high correctness risk; atomic+narrow-mutex achieves same throughput

### Prioritised Implementation Order

1. Atomic scalar counters — removes O(workers) mutex contention; largest throughput gain
2. Signal re-enqueue fix — correctness bug; shared root context is canonical fix
3. Batch size floor (min 10) — trivial one-liner, fixes edge case
4. Bounded error map — prevents OOM; cap at 100 keys
5. Percentile sort outside mutex — reduces lock hold time
6. `MaxConnsPerHost` in transport — prevents port exhaustion at 1000+ workers
7. Response size tracking consistency
8. Benchmark suite — validates all of the above; required for sign-off

---

## Architecture Direction

*From ARCHITECTURE.md — HIGH confidence; grounded in Go stdlib behaviour, benchmark data, and peer-tool comparisons.*

### Existing Pipeline (Keep This Structure)

```
Cobra CLI -> RunStressTest()
  -> HTTP Transport (net/http)
  -> RateLimiter (token bucket)
  -> Worker Pool (N goroutines pulling from jobs chan)
    -> ExecuteRequest() per goroutine
    -> results chan
  -> Single result-processing goroutine
    -> Collector.RecordBatch(batch)
  -> GetStatistics() -> Output Formatters
```

The pipeline itself is correct. The bottleneck is entirely within `Collector.Record()` and the signal handling in `cmd/root.go`. No new components are needed.

### Component Boundaries (No New Files Needed)

| Component | File | Change Required |
|-----------|------|----------------|
| CLI + phase orchestration + signal lifecycle | `cmd/root.go` | Signal fix, batch size fix |
| HTTP request execution + body prep | `internal/request/client.go` | sync.Pool for bytes.Reader |
| Rate limiting | `internal/request/ratelimiter.go` | No change |
| Stats accumulation | `internal/stats/collector.go` | Primary refactor target |
| Progress display | `internal/ui/progress.go` | No change |
| Result formatting | `internal/ui/output.go` | No change |

### Recommended Collector Design

Replace the single mutex with a two-tier approach:

- **Atomic fields** (no lock): `successes`, `failures`, `totalCount`, `totalResponseSize` as `atomic.Int64`; `latencySum`, `minLatency`, `maxLatency` as `uint64` with CAS loop
- **Mutex-guarded fields** (narrow scope): `reservoir []float64`, `statusCount map[int]int`, `errorMessages map[string]int`, `throughput map[int]int`

This is the correct tradeoff. A fully sharded collector is contraindicated because reservoir sampling requires globally consistent `totalCount` to compute replacement probability — sharding it requires cross-shard coordination that erases the benefit.

### Build Order (Dependency Chain)

```
1. internal/stats/collector.go   (dependency-free leaf; fix and benchmark first)
      |
2. internal/request/client.go    (sync.Pool for body reader; independent)
      |
3. cmd/root.go                   (signal fix, batch size fix; depends on #1)
```

Fix `collector.go` first. Run `go test -bench=. -benchmem` against it before touching the orchestration layer. This establishes the before/after baseline required by PROJECT.md.

### Key Anti-Patterns to Avoid

| Anti-Pattern | Why Wrong |
|-------------|-----------|
| Routing results through a second channel to a single collector goroutine | Moves serialisation from mutex to channel receive; same ceiling |
| `sync.Map` for statusCount/errorMessages | Optimised for stable-key read-heavy; write-heavy stress test makes it strictly worse than mutex map |
| Sort + copy inside GetStatistics under held lock | O(N log N) sort holds mutex for ~1ms; copy first, unlock, then sort |
| Unbounded errorMessages map | O(distinct errors) memory growth; use a cap with overflow bucket |

---

## Critical Pitfalls

*From PITFALLS.md — ranked by severity and phase impact.*

### Critical (Phase 1 — Must Fix Before Anything Else)

**Pitfall 1: Single Mutex Over All Collector State**
At 1000+ workers every `Record()` call serialises through one lock. Throughput plateaus and CPU time is wasted in kernel futex wait. The tool ends up measuring its own contention, not the server's capacity. Detection: `go tool pprof` mutex profile shows `sync.(*Mutex).Lock` dominating. Fix: atomic scalars + narrow mutex (see Architecture Direction above).

**Pitfall 2: Signal Re-Enqueue Race (Warm-Up Transition)**
`select { case sigChan <- sig: default: }` silently drops SIGINT when the channel buffer (capacity 1) is full. Non-deterministic: only fails based on goroutine scheduling timing. User presses Ctrl-C during warm-up; warm-up stops, full test runs to completion. Not caught by `-race` — it is a logic race, not a memory race. Fix: shared `rootCtx` from `signal.NotifyContext` that both phases derive from.

**Pitfall 3: Sort Under Lock in GetStatistics**
`sort.Float64s` on a 10K reservoir holds the mutex for ~1-3ms. Blocks all concurrent `Record()` calls during that window. Fix: copy the reservoir under the lock (O(N) copy), then `mu.Unlock()`, then sort the copy.

### Moderate (Phase 1 — Fix Alongside Critical Items)

**Pitfall 4: Unbounded errorMessages Map**
`normalizeError()` compresses most cases but its fallthrough stores verbatim error strings. Under dynamic error messages (server includes request IDs or timestamps), the map grows O(total requests). Fix: cap at 100 distinct keys; overflow into `"(other)"` bucket.

**Pitfall 5: Batch Size = 1 at Concurrency = 2**
`max(1, concurrency/2)` gives batchSize=1 at concurrency=2 — identical to no batching but with extra slice overhead. Fix: `max(10, concurrency/2)`.

**Pitfall 6: http.Transport Misconfiguration**
Missing `MaxConnsPerHost` allows unlimited connections at extreme load, causing port exhaustion. `ResponseHeaderTimeout` and `TLSHandshakeTimeout` are unset — a server that accepts TCP but never sends headers blocks each worker for the full client timeout. Fix: add `MaxConnsPerHost = concurrency`, `TLSHandshakeTimeout = opts.Timeout`, `ResponseHeaderTimeout = opts.Timeout`.

**Pitfall 7: Response Body Drain Allocation (expectBody Path)**
`io.ReadAll` allocates a new buffer per request when `expectBody` is set. At 1000+ concurrency with large bodies, GC pressure is significant. Fix: `sync.Pool` of `[]byte` buffers for body reads.

### Minor (Phase 3 — Before Benchmarking)

**Pitfall 8: Benchmark Anti-Patterns**
Classic `for i := 0; i < b.N; i++` misses Go 1.24's `b.Loop()` API; missing `b.ReportAllocs()`; not using `b.RunParallel` for the Collector hides real-world contention. Fix: use `b.Loop()`, `b.ReportAllocs()`, and `b.RunParallel` in all new benchmarks.

**Pitfall 9: Channel Buffer Sizing at Extreme Concurrency**
At concurrency=10000, `results` channel buffer at `concurrency*2` holds ~1.6MB. Reasonable, but cap at `min(concurrency*2, 4096)` to prevent surprise at pathological inputs.

**Pitfall 10: fmt.Sprintf in normalizeError Truncation Path**
Allocates per call on the fallthrough path. Low impact in practice; defer unless profiling surfaces it.

### Phase-Specific Warnings

| Phase | Risk | Mitigation |
|-------|------|-----------|
| Splitting atomic vs mutex-guarded fields | Can introduce data races | Run with `-race` throughout; never assign atomic fields with `=` — use `.Store()` |
| Percentile sort outside lock | Snapshot copy must be complete before unlock | Verify with `-race`; copy is complete once `copy()` returns |
| Signal handling refactor | Shared rootCtx cancel must not propagate to warm-up internals | Integration test: rootCtx cancel propagates; warm-up timeout does not propagate to main |
| Transport timeout additions | `ResponseHeaderTimeout > opts.Timeout` wins unexpectedly | Validate or clamp: `ResponseHeaderTimeout = min(opts.Timeout, ...)` |
| Benchmark-driven validation | Single-goroutine benchmark understates real contention | Use `b.RunParallel` to simulate concurrent `Record()` calls |

---

## Consensus Across Dimensions

All four research dimensions independently agree on the following:

**1. Atomic scalars are the correct fix for mutex contention.**
STACK, FEATURES, ARCHITECTURE, and PITFALLS all prescribe the same two-tier approach: atomic integers for simple counters, narrow mutex only for the reservoir and maps. No dimension recommends sharding or channel-based aggregation.

**2. The signal re-enqueue pattern is a confirmed bug, not a style issue.**
All four files flag the `select/default` re-enqueue. STACK and PITFALLS confirm it via `os/signal` documentation ("package signal will not block sending to c" — signals are dropped). FEATURES cross-references vegeta's `sync.Once` pattern and bombardier's single-phase context cancel as the canonical alternatives.

**3. No new external dependencies.**
STACK, FEATURES, and ARCHITECTURE all explicitly state this constraint and verify every fix is achievable with Go 1.24 stdlib. PITFALLS reinforces by flagging `tdigest`, `hdrhistogram`, and `go.uber.org/atomic` as anti-patterns.

**4. The fix scope is narrow: two files.**
All dimensions converge on `internal/stats/collector.go` as the primary target, `cmd/root.go` for signal/batch fixes, and `internal/request/client.go` for body reader pooling. No architectural restructuring is needed.

**5. Fix collector first, benchmark to verify, then fix orchestration.**
ARCHITECTURE establishes the build order; PITFALLS warns that benchmarks must use `b.RunParallel` to reflect real contention; FEATURES specifies the benchmark suite as the final sign-off gate.

**6. The tool's differentiators (reservoir sampling, warm-up, body validation) are worth preserving.**
FEATURES identifies these as genuinely uncommon among peer tools. The fixes do not touch these features — they improve the infrastructure that those features rely on.

---

## Implications for Roadmap

### Suggested Phase Structure

**Phase 1: Collector + Signal Correctness (Core Fix)**
Fix the single mutex in `collector.go` using atomic scalars + narrow mutex. Fix the signal re-enqueue bug using a shared root context. Fix batch size floor. Cap `errorMessages` map. Move percentile sort outside the lock.

- Rationale: These are all in the same two files (`collector.go`, `cmd/root.go`). They share a dependency: the atomic field split must happen before the batch/signal refactor because `GetStatistics()` references the same fields being split. The signal fix is a pre-condition for accurate warm-up p99 (correctness gate).
- Delivers: Correct behaviour under Ctrl-C; throughput that scales with worker count; accurate percentile reporting.
- Pitfalls to avoid: Data races from atomic/mutex boundary (run with `-race`); signal propagation across phase contexts (integration test).
- Research flag: STANDARD PATTERNS — well-documented in stdlib and peer tools. No additional research phase needed.

**Phase 2: Transport + Memory Allocation Hardening**
Add `MaxConnsPerHost`, `TLSHandshakeTimeout`, `ResponseHeaderTimeout` to the transport. Add `sync.Pool` for `bytes.Reader` and body-read buffers. Cap channel buffers at `min(concurrency*2, 4096)`.

- Rationale: These are independent of Phase 1 and can be done in any order after Phase 1 is merged. Transport fixes prevent port exhaustion at 1000+ workers — important for CI environments that run long-duration tests. Memory fixes reduce GC pressure at high concurrency.
- Delivers: Stable behaviour at extreme concurrency; reduced GC pauses with `expectBody`.
- Pitfalls to avoid: `ResponseHeaderTimeout > opts.Timeout` interaction (clamp to client timeout); channel buffer reduction increasing scheduling delay (verify with `--trace`).
- Research flag: STANDARD PATTERNS.

**Phase 3: Benchmark Suite and Validation**
Write `BenchmarkCollectorRecord` with `b.RunParallel` and `b.ReportAllocs()` using Go 1.24 `b.Loop()`. Establish before/after throughput numbers. Verify signal fix with manual integration test (warm-up + Ctrl-C). Run all tests with `-race`.

- Rationale: Required by PROJECT.md for sign-off. Must come last because it validates Phases 1 and 2, and benchmark patterns (Pitfall 8) must be correct to produce valid numbers.
- Delivers: Quantified improvement evidence; regression safety net; confirmation of `-race` cleanliness.
- Pitfalls to avoid: Benchmarks that don't use `b.RunParallel` understate contention; benchmarks without `b.ReportAllocs()` hide allocation regressions.
- Research flag: STANDARD PATTERNS — Go 1.24 `b.Loop()` is documented on go.dev/blog.

### Research Flags

| Phase | Research Needed? | Reason |
|-------|-----------------|--------|
| Phase 1 | No | Atomic + narrow mutex pattern is fully prescribed in STACK.md with exact code. Signal fix is unambiguous. |
| Phase 2 | No | Transport fields are stdlib constants with known semantics. `sync.Pool` pattern is well-documented. |
| Phase 3 | No | Go 1.24 `b.Loop()` and `b.RunParallel` are in official Go documentation. |

No phase requires a `/gsd:research-phase` call. All patterns are fully specified in the research files.

---

## Confidence Assessment

| Area | Confidence | Basis |
|------|------------|-------|
| Stack (atomic patterns, transport config) | HIGH | Verified against `pkg.go.dev/sync/atomic`, `net/http` source, golang/go issues; benchmark numbers cross-referenced |
| Features (table stakes, differentiators) | HIGH | vegeta, hey, bombardier source inspected directly; patterns cross-verified |
| Architecture (component boundaries, build order) | HIGH | Grounded in Go stdlib behaviour and SOTA tool comparisons |
| Pitfalls (bugs, phase warnings) | HIGH | Pitfalls 1-3 are confirmed by Go team issues and multiple external sources; Pitfalls 4-7 are correctness issues with clear detection paths |

**Gaps (none blocking, noted for completeness):**

- The exact throughput improvement from atomic counters at the project's specific hardware is not known until benchmarks run. The research provides a directional guarantee (~2.5x for the mutex-to-atomic transition) but not a specific req/sec number.
- `normalizeError()` correctness under pathological error strings (Pitfall 10) is deferred; impact depends on server behaviour in practice. Low risk for normal test targets.
- Channel buffer reduction (Pitfall 9) is a minor risk that requires trace-based verification after the change — cannot be pre-validated analytically.

---

## Sources (Aggregated)

All sources used across the four research files:

- [Go sync/atomic package documentation](https://pkg.go.dev/sync/atomic) — HIGH
- [golang/go#21996: float64 atomic support rejected](https://github.com/golang/go/issues/21996) — HIGH
- [golang/go#33747: Mutex performance collapse under high concurrency](https://github.com/golang/go/issues/33747) — HIGH
- [golang/go#13801: net/http MaxIdleConnsPerHost](https://github.com/golang/go/issues/13801) — HIGH
- [golang/go#52619: os/signal Notify goroutine leak](https://github.com/golang/go/issues/52619) — HIGH
- [os/signal package documentation](https://pkg.go.dev/os/signal) — HIGH
- [Go Optimization Guide: Atomic Operations (goperf.dev)](https://goperf.dev/01-common-patterns/atomic-ops/) — MEDIUM-HIGH
- [Go Optimization Guide: Object Pooling (goperf.dev)](https://goperf.dev/01-common-patterns/object-pooling/) — MEDIUM-HIGH
- [Go Optimization Guide: Worker Pools (goperf.dev)](https://goperf.dev/01-common-patterns/worker-pool/) — MEDIUM-HIGH
- [Go Optimization Guide: Efficient net/http use (goperf.dev)](https://goperf.dev/02-networking/efficient-net-use/) — MEDIUM-HIGH
- [False Sharing in Go (dev.to)](https://dev.to/kelvinfloresta/false-sharing-in-go-the-hidden-enemy-in-your-concurrency-37ni) — MEDIUM
- [sync.Map — VictoriaMetrics analysis](https://victoriametrics.com/blog/go-sync-map/) — MEDIUM-HIGH
- [Graceful Shutdown in Go — VictoriaMetrics](https://victoriametrics.com/blog/go-graceful-shutdown/) — MEDIUM-HIGH
- [Go Concurrent Maps: Sharding (dev.to)](https://dev.to/aaravjoshi/go-concurrent-maps-from-bottlenecks-to-high-performance-sharded-solutions-that-scale-48bk) — MEDIUM
- [testing.B.Loop in Go 1.24 — go.dev/blog](https://go.dev/blog/testing-b-loop) — HIGH
- [Common pitfalls in Go benchmarking — Eli Bendersky](https://eli.thegreenplace.net/2023/common-pitfalls-in-go-benchmarking/) — MEDIUM
- [HTTP Connection Pooling in Go — David Bacisin](https://davidbacisin.com/writing/golang-http-connection-pools-1) — MEDIUM-HIGH
- [vegeta source — tsenart/vegeta](https://github.com/tsenart/vegeta/blob/master/lib/attack.go) — HIGH
- [bombardier source — codesenberg/bombardier](https://github.com/codesenberg/bombardier) — HIGH
- [hey source — rakyll/hey](https://github.com/rakyll/hey) — MEDIUM
- [gocannon — kffl/gocannon](https://github.com/kffl/gocannon) — MEDIUM
- [HdrHistogram lock-free recording](https://github.com/HdrHistogram/HdrHistogram) — HIGH
- [Coordination omission — Gil Tene (groups.google.com)](https://groups.google.com/g/mechanical-sympathy/c/icNZJejUHfE/m/BfDekfBEs_sJ) — HIGH

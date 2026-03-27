# Feature Landscape: Go HTTP Stress Tester Performance & Correctness

**Domain:** High-concurrency HTTP load testing CLI tool (Go)
**Researched:** 2026-03-27
**Scope:** Milestone improvement pass — performance fixes and bug corrections against `api-stress-test`
**Confidence:** HIGH (vegeta/hey/bombardier source code inspected directly; patterns cross-verified)

---

## How Mature Tools Solve the Problems We Are Fixing

Before categorizing features, here is what vegeta, hey, and bombardier actually do for each
problem area identified in PROJECT.md. These are the reference patterns.

### 1. Stats Collection at High RPS

**Hey** (rakyll/hey): Single goroutine owns the collector. Workers send `*result` structs into a
buffered channel sized `min(concurrency * 1000, 1_000_000)`. A separate reporter goroutine drains
the channel and appends to an unbounded `[]float64` slice for latency. Percentile is computed at
the end by sorting the full slice. Simple and correct but memory is O(n requests) — this is
acceptable because hey targets benchmarking, not 24-hour soak tests.

**Bombardier** (codesenberg/bombardier): No shared mutex on the hot path at all. It uses a
purpose-built concurrent histogram (`github.com/codesenberg/concurrent/uint64/histogram`) that
allows lock-free increments from every goroutine simultaneously. Scalar counters (req2xx, req3xx,
etc.) are `atomic.AddUint64`. Only the RPS rate meter touches a mutex, on a 10ms interval, not
per-request. This is the archetypal pattern: **atomics for scalars, concurrent histogram for
latency distribution**.

**Vegeta** (tsenart/vegeta): Streaming architecture — `Attack()` returns a `<-chan *Result`; the
caller owns aggregation. `Metrics.Add()` is not goroutine-safe and is called from a single
aggregation goroutine. Latency is stored in a t-digest (`influxdata/tdigest`, compression=100)
which gives sub-1% accuracy at any sample count with O(1) per-Add cost and bounded memory (a few
KB). Stop semantics use `sync.Once` + channel close, so the first call closes the stop channel
and subsequent calls are no-ops.

**Current implementation**: Single `sync.Mutex` guards all fields in `Collector.Record()`. At
1000 workers, every request serialises through this lock. Contention scales as O(workers).

### 2. Signal Handling Pattern

**Vegeta**: `sync.Once` wraps `close(stopCh)`. `Stop()` returns bool indicating whether this was
the first call. Workers check `stopCh` in a select. No re-enqueue; clean close semantics.

**Bombardier**: `signal.Notify(c, os.Interrupt)` → goroutine → `barrier.cancel()` which calls a
context cancel. Single phase, so no warm-up re-enqueue problem.

**Hey**: `stopCh` is a plain channel. `Stop()` sends N signals (one per worker). Workers receive
in `select { case <-b.stopCh: return }`.

**Current implementation problem**: Warm-up phase re-enqueues the signal with
`select { case sigChan <- sig: default: }`. The `default` branch means the re-enqueue silently
drops the signal if the buffer is already full (buffer=1) or the main-phase goroutine has not yet
started listening. The canonical fix is to not re-enqueue at all: use a shared `context.Context`
or a single cancel function that covers both phases. If the warmup signal handler cancels a
shared root context, the main phase will naturally see cancellation without any re-enqueue.

### 3. Memory Bounding for Long-Running Tests

**Vegeta**: T-digest. Memory is O(compression * log(n)) ≈ a few KB regardless of request count.

**Gocannon** (kffl): Explicit histogram mode caps memory at ~3 MB using fixed-width 1μs bins.
In "reqlog" mode it pre-allocates per-connection slices with `--preallocate`.

**Hey**: Unbounded `[]float64`. Acceptable for short benchmark runs; problematic for hour-long
soak tests.

**Current implementation**: Reservoir sampling (10,000 float64 = 80 KB). This is correct and
better than hey for long runs. The `errorMessages` map is unbounded — a unique error per request
under network chaos could grow without limit.

### 4. HTTP Transport Optimizations

All mature tools set these:
- `MaxIdleConnsPerHost = concurrency` (default Go value is 2, which causes TIME_WAIT exhaustion)
- `MaxIdleConns = concurrency` (global pool must match per-host to avoid hidden cap)
- `IdleConnTimeout = 90s` (matches AWS/nginx defaults)
- `DisableKeepAlives = false` (keep-alive is critical for performance; each new connection costs
  a full TCP + TLS handshake)
- `DisableCompression = true` (most load testers disable auto-gzip to avoid decompression CPU
  overhead skewing latency measurements; optional but common)

Vegeta defaults: `MaxIdleConnsPerHost = 10000`. Hey: `min(concurrency, 500)`. Both scale with
the worker count.

**Current implementation**: Sets `MaxIdleConns = concurrency` and `MaxIdleConnsPerHost =
concurrency`. This is correct. Missing `MaxConnsPerHost` — without it, under extreme load Go
allows unlimited new connections which can exhaust OS ports. Setting `MaxConnsPerHost =
concurrency` prevents this.

### 5. Batch Processing / Lock-Free Result Collection

**Bombardier**: No batching needed — concurrent histogram is inherently lockless.

**Hey**: Single-goroutine aggregation via channel drain; no explicit batching, no lock.

**Vegeta**: Single aggregator goroutine calls `Metrics.Add()` on each streamed result; no lock
in the aggregator because only one goroutine touches Metrics.

**Current implementation**: The result collector in `cmd/root.go` already drains the results
channel in a single goroutine with a batch buffer (`batchSize = max(1, concurrency/2)`). The
batching is correct in intent, but `batchSize=1` when `concurrency=2` wastes the optimisation.
A minimum of 8-16 is a better floor. More importantly, the bottleneck is `Collector.Record()`
being called under mutex even from a single goroutine — the lock is still acquired per-record.

---

## Table Stakes

Features users expect. A stress tester without these is not credible.

| Feature | Why Expected | Complexity | Current Status | Notes |
|---------|-------------|------------|----------------|-------|
| Accurate latency percentiles (p50/p90/p95/p99) | Every peer tool reports these; users cite p99 for SLA | Low | Present (reservoir) | Accuracy is bounded by reservoir size; reservoir sampling is correct |
| Atomic scalar counters (success/failure/status codes) | Per-request mutex lock is a known anti-pattern at scale | Low | Missing — all behind mutex | Replace `successes`, `failures`, `totalCount`, `statusCount` hot-path fields with atomics |
| Single-goroutine result aggregation | Avoids all locking in the aggregation path; used by hey, vegeta, bombardier | Low | Partially present | Channel-based drain exists; Collector.Record still uses mutex |
| Bounded error message storage | Network errors can be unboundedly unique; a cap prevents OOM on chaos tests | Low | Missing | Cap `errorMessages` map at ~100 distinct entries; drop or aggregate beyond that |
| Signal handling that doesn't drop on warm-up transition | Re-enqueue with `select/default` silently drops SIGINT | Medium | Buggy | Replace with shared root context cancelled on signal |
| Correct minimum batch size | `batchSize = max(1, concurrency/2)` gives batchSize=1 at concurrency=2 | Low | Buggy | Floor at 8 or 16 |
| `MaxConnsPerHost` set equal to concurrency | Without it, unlimited connections are opened causing port exhaustion | Low | Missing from transport config | One-line fix |
| Benchmarkable code paths | Changes should be verifiable with `go test -bench` | Medium | Not yet present | Add `BenchmarkCollectorRecord` parallel benchmark |
| Percentile calculation outside mutex | Current sort+percentile happens under `GetStatistics()` mutex lock | Low | Present but suboptimal | Sort a copy outside the lock; lock only long enough to copy the reservoir |

---

## Differentiators

Features that set this tool apart. Not universally expected, but add measurable value.

| Feature | Value Proposition | Complexity | Current Status | Notes |
|---------|------------------|------------|----------------|-------|
| Reservoir sampling for memory-bounded p99 | T-digest requires external dep; reservoir is simpler, dependency-free, ~80 KB | Low | Present | Only gocannon and this tool use a bounded approach without a dep |
| Per-second throughput timeline | Lets users see ramp-up, saturation, and decay curves | Medium | Present | Not in any of the three peer tools by default |
| Warm-up phase support | Avoids JIT-cold-start skew in p99; rare in Go load testers | Medium | Present (but signal-buggy) | Once signal bug fixed, this becomes a genuine differentiator |
| Response body validation (`expect-body`, `expect-status`) | Most tools only check HTTP status; content correctness under load catches CDN caching bugs | Medium | Present | Uncommon in peer tools |
| Latency histogram with dynamic bucketing | Lets users see bimodal distributions that p99 alone misses | Low | Present | hey/vegeta also provide histograms; our equal-width approach is simpler |
| JSON output + file output | Machine-readable results for CI pipelines | Low | Present | Standard in mature tools |

---

## Anti-Features

Features to explicitly NOT build during this milestone.

| Anti-Feature | Why Avoid | What to Do Instead |
|-------------|-----------|-------------------|
| New external dependencies for histogram (tdigest, hdrhistogram) | Adds transitive deps; violates the "minimal deps" constraint; reservoir sampling is already accurate | Keep reservoir sampling; tune the reservoir size if accuracy is insufficient |
| Distributed / multi-node attack | Massive scope increase; k6 cloud already exists for this | Out of scope per PROJECT.md |
| Real-time streaming metrics output | Requires websocket or SSE server; huge complexity for marginal gain at CLI level | The per-second throughput timeline in the final output already covers this need |
| Scripted/programmable scenarios (Lua, JS) | k6 exists; adding a scripting runtime is a different product | Stick to declarative CLI flags |
| Ramp-up rate scheduling | Adds state machine complexity; vegeta has this; not in scope for a quality pass | Warm-up phase already covers server warm-up; linear ramp-up is a new feature |
| HTTP/2 multiplexing support | `net/http` does HTTP/2 by default when the server advertises it; explicitly managing h2 streams is a different scope | Current `http.Client` already negotiates HTTP/2 transparently |
| Per-request-type breakdown (GET vs POST mixed) | Multi-endpoint scenarios; out of scope | Single URL per invocation is sufficient for stress testing |
| Lock-free data structures from scratch (CAS stacks, etc.) | High risk of subtle correctness bugs; no new deps constraint | Atomic scalar + single-goroutine aggregation achieves the same throughput without the risk |

---

## Feature Dependencies

```
Signal bug fix (shared context)
  └─> Accurate warm-up p99 (warm-up results now correctly isolated)

Atomic scalar counters
  └─> Collector.Record() no longer blocks workers on mutex
        └─> Percentile copy-outside-lock becomes the last remaining lock scope

Batch size floor fix (min 8)
  └─> Reduces channel send overhead at low concurrency

MaxConnsPerHost = concurrency
  └─> Prevents port exhaustion at 1000+ workers on long-duration tests

Bounded error map
  └─> Prevents OOM under network chaos (distinct errors per request)

BenchmarkCollectorRecord
  └─> Quantifies all the above improvements
      └─> Required by PROJECT.md "benchmark before/after" constraint
```

---

## MVP Recommendation for This Milestone

The eight items in PROJECT.md Active requirements map directly to the Table Stakes fixes above.
Prioritised order based on impact:

1. **Atomic scalar counters** — removes O(workers) mutex contention; biggest throughput gain
2. **Signal re-enqueue fix** — correctness bug; shared context is the canonical fix
3. **Batch size floor** — trivial one-liner; fixes edge case at concurrency=2
4. **Bounded error map** — prevents OOM; cap at 100 distinct keys
5. **Percentile sort outside mutex** — reduces lock hold time during `GetStatistics()`
6. **`MaxConnsPerHost` in transport** — prevents port exhaustion at 1000+ workers
7. **Response size tracking consistency** — align the `expectBody`/non-expectBody paths
8. **Benchmark suite** — validates all the above; required for sign-off

Defer: channel buffer overhead at extreme 10K+ concurrency (low real-world impact vs complexity
of dynamic sizing; existing `concurrency*2` is already reasonable for most workloads).

---

## Sources

- vegeta source: https://github.com/tsenart/vegeta/blob/master/lib/attack.go (HIGH confidence — direct source inspection)
- vegeta metrics API: https://pkg.go.dev/github.com/tsenart/vegeta/v12/lib (HIGH confidence — official pkg.go.dev)
- hey source: https://github.com/rakyll/hey (MEDIUM confidence — inspected via raw.githubusercontent.com)
- bombardier source: https://github.com/codesenberg/bombardier (HIGH confidence — direct source inspection)
- gocannon: https://github.com/kffl/gocannon (MEDIUM confidence — README inspection)
- Go transport connection pool: https://davidbacisin.com/writing/golang-http-connection-pools-1 (HIGH confidence — matches Go stdlib docs)
- Go HTTP tuning for load testing: http://tleyden.github.io/blog/2016/11/21/tuning-the-go-http-client-library-for-load-testing/ (MEDIUM confidence — older but principle unchanged in Go stdlib)
- Go atomic ops guide: https://goperf.dev/01-common-patterns/atomic-ops/ (MEDIUM confidence — WebSearch verified)
- Go graceful shutdown patterns: https://victoriametrics.com/blog/go-graceful-shutdown/ (MEDIUM confidence — authoritative community source)
- HdrHistogram lock-free recording: https://github.com/HdrHistogram/HdrHistogram (HIGH confidence — official repo)
- Coordination omission background: https://groups.google.com/g/mechanical-sympathy/c/icNZJejUHfE/m/BfDekfBEs_sJ (HIGH confidence — Gil Tene original post)

# Architecture Patterns: High-Performance Go HTTP Stress Tester

**Domain:** Go HTTP load/stress testing tool — high-concurrency internals
**Researched:** 2026-03-27
**Confidence:** HIGH (grounded in Go standard library behaviour, benchmark data, and SOTA tool comparisons)

---

## Current Architecture Diagnosis

The existing tool follows this pipeline:

```
Cobra CLI → RunStressTest()
  → HTTP Transport (net/http)
  → RateLimiter (token bucket, time.Ticker)
  → Worker Pool (goroutines pulling from `jobs` chan)
    → ExecuteRequest() per goroutine
    → results chan
  → Single result-processing goroutine
    → Collector.Record() [single mutex guards all state]
  → GetStatistics() → Output Formatters
```

**Known bottleneck:** `Collector.Record()` holds a single `sync.Mutex` for every write.
At 1000 workers each completing requests as fast as the network allows, every result write
serialises through one lock. Under 1000 goroutines the mutex contention is the dominant
cost — not the HTTP work itself.

**Known race:** Warm-up phase re-enqueues SIGINT via `select/default`, which silently
drops the signal when the buffered channel is already full (capacity 1). If the warm-up
signal goroutine is scheduled after the main phase signal goroutine starts reading, the
signal is consumed by the warm-up handler but never forwarded.

---

## Recommended Architecture

### Component Boundaries

| Component | Responsibility | Communicates With | File |
|-----------|---------------|-------------------|------|
| `cmd/root.go` | CLI entry, phase orchestration, signal lifecycle | All below | existing |
| `internal/request/client.go` | Single HTTP request execution, body prep | cmd/root.go | existing |
| `internal/request/ratelimiter.go` | Token-bucket rate limiting | cmd/root.go (job feeder goroutine) | existing |
| `internal/stats/collector.go` | Thread-safe result accumulation | cmd/root.go (result processor) | **refactor target** |
| `internal/ui/progress.go` | Live terminal progress display | cmd/root.go | existing |
| `internal/ui/output.go` | Final result formatting (text + JSON) | cmd/root.go | existing |

No new components needed. All changes are internal to `stats/collector.go` and the
result-processing loop in `cmd/root.go`.

---

## Data Flow

```
[job feeder goroutine]
  RateLimiter.Wait(ctx)
  → jobs chan (buffered, size = Concurrency*2)
      ↓
[N worker goroutines]
  ExecuteRequest(ctx, client, ...)
  → results chan (buffered, size = Concurrency*2)
      ↓
[single result-processing goroutine in main]
  batch accumulation (batchSize = Concurrency/2)
  → Collector.RecordBatch(batch)
      ↓
[Collector internals — REFACTOR]
  sharded mutex shards[N]
  per-shard: successes, failures, latencySum, statusCount, errorMessages
  global atomics: totalCount, minLatency, maxLatency, totalResponseSize
      ↓
[GetStatistics() — called once at end]
  merge shards → Statistics{}
  sort reservoir → percentiles
      ↓
[ui.PrintTextResult / ui.PrintJSONResult]
```

Information flows in one direction during the test. The only upward flow is
`ctx.Done()` cancellation propagating from the signal handler back through the
context tree to all workers and the job feeder.

---

## Architecture Decisions by Dimension

### 1. Stats Collection: Sharded Collector

**Recommendation: Replace the single mutex with a fixed shard array.**

**Rationale:**

The single `sync.Mutex` in `Collector.Record()` serialises all 1000+ workers. Under high
concurrency, lock acquisition time dominates — production cases show p99 latency dropping
from 100ms to under 1ms when switching from a single mutex to 32 shards (source: Go
Concurrent Maps post benchmarks).

`sync.Map` is unsuitable here. It is optimised for "write once, read many" and incurs dual
internal-map overhead on write-heavy workloads. At 1000 workers all writing results, every
call is a write. The dual-map architecture of `sync.Map` adds overhead that makes it
strictly worse than a sharded mutex for this access pattern (source: VictoriaMetrics
sync.Map analysis).

**Pattern:**

```go
const numShards = 32  // power-of-two; 2x–4x NumCPU is a good starting point

type shard struct {
    mu            sync.Mutex
    successes     int64
    failures      int64
    latencySum    float64
    statusCount   map[int]int
    errorMessages map[string]int
    _             [0]byte  // prevent struct-level false sharing via alignment
}

// Pad each shard to its own cache line to prevent false sharing between
// adjacent shards in the array.
type paddedShard struct {
    shard
    _ [64 - unsafe.Sizeof(shard{})%64]byte  // conceptual; compute at design time
}

type Collector struct {
    shards    [numShards]paddedShard
    // Global atomics for fields that need exact values across shards
    // without requiring cross-shard coordination at record time.
    totalCount        atomic.Int64
    totalResponseSize atomic.Int64
    minLatency        atomic.Uint64  // bits of float64 via math.Float64bits
    maxLatency        atomic.Uint64
    // Reservoir lives on one shard (shard 0) or under a single separate mutex
    // because reservoir sampling requires a global totalCount to compute
    // replacement probability. Accessed less frequently than per-shard counters.
    reservoirMu sync.Mutex
    reservoir   []float64
}
```

Worker assignment to shard: `shardIndex = goroutineID % numShards` or hash of the
current goroutine stack pointer. In practice, `atomic.AddInt64(&c.totalCount, 1) % numShards`
gives uniform distribution without goroutine-ID bookkeeping.

**Why not atomic-only:**

Atomics excel for simple int64 counters but cannot protect maps (`statusCount`,
`errorMessages`) without a lock. The per-shard mutex is cheap because each shard sees
only `1/numShards` of the total write rate. At 32 shards and 1000 workers, each shard
sees ~31 concurrent writers on average — far below the collapse threshold documented in
golang/go issue #33747.

**Why not channel-based collection:**

Routing every result through a channel to a single collector goroutine just moves the
bottleneck from mutex contention to channel receive serialisation. A single goroutine
processing 1000+ results/sec from a channel is faster than a single mutex for simple
fields, but still serialises for complex state (maps, reservoir). Sharding is strictly
superior because it parallelises the write path.

---

### 2. Worker Pool: Channel Buffer Sizing and Backpressure

**Recommendation: Keep current buffer formula `Concurrency * 2` for both `jobs` and `results` channels. For I/O-bound work this is the correct range.**

**Rationale:**

The current code uses `Concurrency * 2` for both channel buffers. For I/O-bound workers
(HTTP requests that block on network), the guidance is 2–10x worker count. The lower
bound (`2x`) is already sufficient because:

1. Workers spend most time blocked in `client.Do()` — the job feeder rarely needs to
   queue more than a few items ahead.
2. `results` buffer needs to absorb burst completions when many requests return
   simultaneously. `2x` handles this while keeping memory bounded.

**Exception — extreme concurrency (Concurrency > 2000):**

At 10K workers, a `jobs` buffer of 20K `struct{}` items is 20KB — negligible. But the
`results` buffer at 20K `request.Result` items costs ~20K * 56 bytes = ~1.1MB. This is
acceptable given the project's max concurrency cap of 10,000.

**Backpressure is already correct:** The buffered `jobs` channel combined with the rate
limiter's `select { case jobs <- struct{}{}: case <-ctx.Done(): }` pattern provides
natural backpressure. When workers are saturated, the job feeder blocks on channel send,
which in turn blocks the rate limiter loop. No changes needed here.

**The current batch size bug:**

`batchSize = max(1, Concurrency/2)` evaluates to 1 when `Concurrency = 2`. A batch of 1
is functionally identical to unbatched processing and provides no contention reduction.
Fix: `batchSize = max(Concurrency, 16)`. The floor of 16 ensures batching even at low
concurrency, and equals `Concurrency` at higher values to keep the batch small enough that
results don't sit in the batch buffer long.

---

### 3. Result Aggregation: Pre-aggregation in Workers vs Central Collector

**Recommendation: Keep the single result-processing goroutine in `cmd/root.go` but feed it into the sharded Collector. Do NOT pre-aggregate in workers.**

**Rationale:**

The current design already separates the result-processing concern (one goroutine in
`RunStressTest`) from the workers (N goroutines executing HTTP). This is the right
boundary. Pre-aggregating inside worker goroutines would require each worker to maintain
its own partial stats and merge at shutdown — this adds complexity and allocation per
goroutine without meaningful gain, because the result-processing goroutine is not a
bottleneck (it only calls `Collector.Record()` which will be sharded).

The value of the single result-processing goroutine is that it **owns the batch**, so
batch accumulation needs no synchronisation. The goroutine fills a local slice and calls
`Collector.RecordBatch(batch)` when the batch is full.

`RecordBatch` can hold a shard lock once per batch per shard (grouping all batch entries
by shard before locking) rather than once per record — this is the key contention
reduction from batching. A batch of 500 results distributed across 32 shards averages
~15 entries per shard per lock acquisition, versus 500 separate lock acquisitions today.

---

### 4. Memory Layout: Cache-Line Padding for Atomics

**Recommendation: Pad hot atomic counters that are modified by concurrent goroutines to sit on separate 64-byte cache lines.**

**Rationale:**

Modern CPUs operate on 64-byte cache lines. When two goroutines on different CPU cores
write to `int64` fields that share a cache line, every write triggers cache invalidation
on the other core's cache — "false sharing". Benchmarks show 5x throughput difference
between padded and unpadded atomic counters in concurrent hot loops (Dev.to false sharing
benchmark: 450ns/op → 89ns/op with padding).

**Pattern for the Collector:**

```go
type paddedInt64 struct {
    v int64
    _ [56]byte  // 8 + 56 = 64 bytes: one full cache line
}
```

Apply this to global atomics that are written by many goroutines concurrently:
`totalCount`, `totalResponseSize`. The atomic `minLatency`/`maxLatency` fields are
updated less frequently (only when a new extreme is observed) and can share a cache line
without significant impact.

**Struct field ordering within each shard:**

Fields within a shard are already protected by the shard's mutex — they are not accessed
concurrently by definition (only one goroutine holds the shard lock at a time). Padding
within a shard struct is therefore unnecessary and would waste memory. Only the shard
structs themselves need padding between them to prevent the mutex spinlock state of one
shard from false-sharing with an adjacent shard.

**Practical note for this codebase:**

The most impactful padding is between adjacent `paddedShard` array elements. If each
`shard` struct is smaller than 64 bytes, pad it to exactly 64 bytes or a multiple of 64.
For the proposed shard struct (mutex ≈ 8 bytes, two int64s, float64, two map headers ≈
16 bytes each), total is roughly 64–80 bytes. Use `_ [...]byte` padding calculated at
design time to reach the next 64-byte boundary.

---

### 5. Signal Handling: Clean Phase Transitions

**Recommendation: Use a single persistent signal channel with explicit phase handoff via a dedicated `warmupDone` channel, eliminating the re-enqueue pattern entirely.**

**Rationale:**

The current code re-enqueues SIGINT from the warm-up handler to the shared `sigChan`:

```go
select {
case sigChan <- sig:
default:  // signal silently dropped here
}
```

This is a race. The `sigChan` has capacity 1. If the main phase's signal goroutine starts
reading before the warm-up handler can write, the re-enqueue either succeeds (safe) or
finds the channel full because the main goroutine hasn't drained it yet, dropping the
signal (unsafe). The `default` case is the bug — it makes signal forwarding best-effort.

**Correct pattern:**

```go
// Single signal channel, buffered at 2 to absorb one signal per phase
// without ever needing to forward between goroutines.
sigChan := make(chan os.Signal, 2)
signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
defer signal.Stop(sigChan)

// Warm-up phase
warmCtx, warmCancel := context.WithTimeout(context.Background(), opts.Warmup)
warmSigDone := make(chan struct{})
go func() {
    defer close(warmSigDone)
    select {
    case <-sigChan:
        warmCancel()
        // Do NOT re-enqueue. Instead, store that user interrupted.
        atomic.StoreInt32(&userInterrupted, 1)
    case <-warmCtx.Done():
    }
}()

// ... warm-up workers ...
warmWg.Wait()
warmCancel()
<-warmSigDone  // safe: always exits via one of the two select cases

// Main phase: check if already interrupted before starting
if atomic.LoadInt32(&userInterrupted) != 0 {
    return nil  // or a specific interrupted error
}

// Main phase signal goroutine reads from the same sigChan.
// No forwarding needed: if SIGINT arrived during warm-up, we already
// handled it via userInterrupted. If it arrives during main phase,
// sigChan has capacity for it.
go func() {
    select {
    case <-sigChan:
        cancel()
    case <-ctx.Done():
    }
}()
```

**Why this is correct:**

- `sigChan` capacity 2 means the OS can always deliver the signal without blocking, even
  if both the warm-up goroutine and the main phase goroutine are momentarily not
  scheduled.
- No re-enqueue means no `select/default` drop race.
- The `userInterrupted` atomic flag is the clean handoff between phases.
- `<-warmSigDone` before starting the main phase ensures the warm-up signal goroutine
  cannot interfere with the main phase's read of `sigChan`.

**Context propagation principle:**

Each phase creates its own `context.WithCancel` or `context.WithTimeout` derived from
`context.Background()`, not from the previous phase's context. This prevents cancellation
from leaking across phase boundaries. The signal channel is shared; contexts are not.

---

## Component Build Order (Dependencies)

```
1. internal/stats/collector.go  (no imports from this tool's other packages)
      ↓
2. internal/request/client.go   (depends on nothing internal)
   internal/request/ratelimiter.go
      ↓
3. internal/ui/progress.go      (depends on nothing internal)
   internal/ui/output.go        (depends on stats.Statistics type)
      ↓
4. cmd/root.go                  (depends on all of the above)
```

**Implication for phased implementation:**

Fix `stats/collector.go` first — it is the dependency-free leaf node. All other changes
in `cmd/root.go` (batch size fix, signal handling fix) are independent and can be done
in any order after the collector is stabilised. Benchmarks (`go test -bench`) against the
collector can be run before touching the orchestration layer.

---

## Anti-Patterns to Avoid

### Anti-Pattern 1: Routing Results Through a Channel to a Single Collector Goroutine
**What:** `results chan` feeds a single goroutine that calls `collector.Record()` serially.
**Why bad:** Moves serialisation from mutex to channel receive. Channel has its own
internal lock. Throughput ceiling is now one goroutine's receive rate, same constraint.
**Instead:** Keep the current single result-processing goroutine but have it call into a
sharded collector that parallelises the writes internally.

### Anti-Pattern 2: sync.Map for statusCount or errorMessages
**What:** Replace `map[int]int` guarded by mutex with `sync.Map`.
**Why bad:** `sync.Map` is optimised for stable keys with many reads and few writes. In
a stress test, every request writes a new count. The dual-map overhead of `sync.Map`
makes it strictly worse than a per-shard mutex map for write-heavy patterns.
**Instead:** Per-shard `map[int]int` with per-shard mutex.

### Anti-Pattern 3: Allocating Inside GetStatistics Under a Held Lock
**What:** Current `GetStatistics()` does `copy(sorted, c.reservoir)` then `sort.Float64s`
inside the mutex lock.
**Why bad:** Sort is O(n log n) on 10K elements — roughly 130K comparisons. This holds
the mutex for ~1ms, blocking any final `Record()` calls that arrive after the test ends
but before statistics are computed (rare but possible during shutdown).
**Instead:** Take the lock, copy the reservoir slice, release the lock, then sort the
copy outside the lock. This is safe because `GetStatistics` is only called once after
all workers have stopped.

### Anti-Pattern 4: Unbounded errorMessages Map
**What:** `errorMessages map[string]int` grows without bound if many distinct error
strings are produced.
**Why bad:** Under diverse network error conditions (varied timeout messages, varied
connection errors), the map can accumulate thousands of keys. Iteration at `GetStatistics`
time and memory pressure both grow linearly.
**Instead:** Cap the map at a fixed size (e.g., 100 distinct error messages). When the
cap is reached, consolidate new unseen errors into an "other" bucket. Since `normalizeError()`
already canonicalises errors into ~7 categories, the effective unique key count is
already small — but the cap prevents pathological cases with custom error strings.

---

## Scalability Considerations

| Concern | At 50 workers | At 1000 workers | At 10K workers |
|---------|--------------|-----------------|----------------|
| Collector mutex | Negligible | Dominant bottleneck (serialises all writes) | Catastrophic |
| Sharded collector (32 shards) | No change from current | ~31 writers/shard avg — low contention | ~312 writers/shard avg — manageable |
| Channel buffer memory | ~5KB results buffer | ~112KB results buffer | ~1.1MB results buffer |
| errorMessages map | < 10 entries (normalised) | < 10 entries (normalised) | < 10 entries (normalised, cap enforced) |
| Reservoir sampling | Lock free for first 10K entries | Contended (all writes try reservoir) | Heavily contended reservoir lock |
| Reservoir under sharding | N/A | Reservoir gets dedicated mutex separate from shard locks | Reservoir dedicated mutex still serialises at 10K |

**Reservoir note:** At 10K workers, reservoir sampling itself becomes a contention point
because the replacement probability check (`rand.IntN(totalCount)`) requires reading the
current total count atomically, then potentially writing to the reservoir under its own
lock. The mitigation is to move the reservoir behind its own dedicated mutex, separate
from shard mutexes, so shard writes and reservoir writes do not compete.

---

## Sources

- [Go Concurrent Maps: Sharded Solutions](https://dev.to/aaravjoshi/go-concurrent-maps-from-bottlenecks-to-high-performance-sharded-solutions-that-scale-48bk) — sharding pattern, shard count recommendations, p99 latency benchmarks
- [Goroutine Worker Pools — Go Optimization Guide](https://goperf.dev/01-common-patterns/worker-pool/) — channel buffer sizing, backpressure mechanics
- [Batching Operations — Go Optimization Guide](https://goperf.dev/01-common-patterns/batching-ops/) — batch flush strategies
- [Atomic Operations — Go Optimization Guide](https://goperf.dev/01-common-patterns/atomic-ops/) — atomics vs mutex performance numbers (80ns vs 110ns)
- [False Sharing in Go](https://dev.to/kelvinfloresta/false-sharing-in-go-the-hidden-enemy-in-your-concurrency-37ni) — 64-byte padding pattern, 5x benchmark improvement
- [sync.Map — VictoriaMetrics](https://victoriametrics.com/blog/go-sync-map/) — why sync.Map is wrong for write-heavy workloads
- [Graceful Shutdown in Go — VictoriaMetrics](https://victoriametrics.com/blog/go-graceful-shutdown/) — signal channel buffering, the select/default drop pitfall
- [sync: Mutex performance collapses with high concurrency — golang/go #33747](https://github.com/golang/go/issues/33747) — confirmed mutex collapse pattern
- [Vegeta HTTP load tester — GitHub](https://github.com/tsenart/vegeta) — reference architecture for pipeline design

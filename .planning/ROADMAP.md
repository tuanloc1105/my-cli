# Roadmap: api-stress-test Performance Audit & Bug Fix

## Overview

Three phases fix a correct but contention-limited tool. Phase 1 eliminates the single-mutex bottleneck in the stats collector and repairs the signal-handling correctness bug — the two changes with the largest combined impact. Phase 2 hardens the HTTP transport and reduces GC pressure under sustained high-concurrency load. Phase 3 validates all changes with a benchmark suite and a full race-detector pass, delivering the quantified before/after evidence required for sign-off.

## Phases

- [ ] **Phase 1: Collector & Signal Correctness** - Fix mutex contention, signal re-enqueue bug, batch floor, and error map cap
- [ ] **Phase 2: Transport & Memory Hardening** - Add transport timeouts, connection limits, sync.Pool for body allocation, channel buffer cap
- [ ] **Phase 3: Benchmark & Validation** - Write benchmark suite, run race detector, confirm quantified improvement

## Phase Details

### Phase 1: Collector & Signal Correctness
**Goal**: The stats collector scales with worker count and SIGINT is never silently dropped
**Depends on**: Nothing (first phase)
**Requirements**: CORR-01, CORR-02, PERF-01, PERF-02, PERF-03, PERF-04
**Success Criteria** (what must be TRUE):
  1. Running with 1000 workers produces throughput that scales proportionally — not plateauing due to collector lock contention
  2. Pressing Ctrl-C during the warm-up phase terminates the entire run, not just warm-up
  3. Running at concurrency=2 uses a batch size of at least 16, not 1
  4. A stress test against a server returning diverse error strings does not grow the error map without bound
  5. Percentile statistics are computed correctly after the test completes (sort happens outside the mutex)
**Plans**: TBD

### Phase 2: Transport & Memory Hardening
**Goal**: The HTTP transport handles 1000+ workers without port exhaustion or hung connections, and body allocation GC pressure is reduced
**Depends on**: Phase 1
**Requirements**: TRAN-01, TRAN-02, TRAN-03, TRAN-04
**Success Criteria** (what must be TRUE):
  1. A 1000-worker run against a real server does not exhaust ephemeral ports (no TIME_WAIT accumulation)
  2. A server that accepts TCP but never sends response headers causes worker slots to time out — not hang indefinitely
  3. Running with --expect-body against a large response does not produce elevated GC pause frequency compared to the discard path
  4. Setting concurrency=10000 does not allocate a disproportionately large channel buffer
**Plans**: TBD

### Phase 3: Benchmark & Validation
**Goal**: Quantified before/after throughput numbers exist and all changes are confirmed race-free
**Depends on**: Phase 2
**Requirements**: VALD-01, VALD-02
**Success Criteria** (what must be TRUE):
  1. `go test -bench=BenchmarkCollector -benchmem ./internal/stats/` produces a before/after throughput comparison showing measurable improvement at parallel concurrency
  2. `go test -race ./...` from the api-stress-test directory exits 0 with no data race reports
**Plans**: TBD

## Progress

| Phase | Plans Complete | Status | Completed |
|-------|----------------|--------|-----------|
| 1. Collector & Signal Correctness | 0/TBD | Not started | - |
| 2. Transport & Memory Hardening | 0/TBD | Not started | - |
| 3. Benchmark & Validation | 0/TBD | Not started | - |

# Requirements: api-stress-test Performance Audit

**Defined:** 2026-03-27
**Core Value:** Accurate, reliable stress test results at 1000+ workers without bottlenecking on tool internals

## v1 Requirements

### Correctness

- [ ] **CORR-01**: Signal handling uses shared root context — SIGINT never silently dropped during warm-up to main phase transition
- [ ] **CORR-02**: Batch size floor of 16 — `batchSize = max(concurrency, 16)` prevents degenerate batch=1 at low concurrency

### Collector Performance

- [ ] **PERF-01**: Hot counters (totalCount, successCount, failureCount, totalResponseSize) use `atomic.Int64` — zero mutex acquisition in Record() hot path for scalar updates
- [ ] **PERF-02**: Mutex scope narrowed to protect only reservoir slice and map writes — not the entire Record() method
- [ ] **PERF-03**: `GetStatistics()` copies reservoir under lock, sorts outside lock — eliminates O(N log N) stall under mutex
- [ ] **PERF-04**: Error message map capped at 100 entries — bounded memory under pathological diverse-error scenarios

### Transport Performance

- [ ] **TRAN-01**: HTTP transport sets `MaxConnsPerHost = concurrency` — prevents TIME_WAIT port exhaustion at 1000+ workers
- [ ] **TRAN-02**: Transport includes `TLSHandshakeTimeout` and `ResponseHeaderTimeout` — prevents hung connections from consuming worker slots
- [ ] **TRAN-03**: Response body readers use `sync.Pool` — reduces GC pressure under sustained high throughput
- [ ] **TRAN-04**: Channel buffers capped with sensible maximum at extreme concurrency (10K+) — prevents excessive memory allocation

### Validation

- [ ] **VALD-01**: Benchmark suite for Collector using `b.RunParallel` + `b.Loop()` (Go 1.24) — quantifies before/after throughput improvement
- [ ] **VALD-02**: All changes pass `go test -race` — no data races introduced

## v2 Requirements

### Advanced Optimization

- **ADV-01**: Replace reservoir sampling with t-digest for O(1) per-Add percentile tracking
- **ADV-02**: Cache-line padding (`_ [56]byte`) on hot atomics to eliminate false sharing
- **ADV-03**: Sharded collector design (32 shards) for extreme concurrency (10K+)

## Out of Scope

| Feature | Reason |
|---------|--------|
| New CLI flags or features | This is a quality pass, not a feature release |
| UI/output format changes | Display layer is fine, bottleneck is internal |
| Code style refactoring | Only change what impacts performance or correctness |
| Other tools in the repo | Scope is api-stress-test/ only |
| External dependencies | All fixes use stdlib-only patterns |

## Traceability

| Requirement | Phase | Status |
|-------------|-------|--------|
| CORR-01 | Phase 1 | Pending |
| CORR-02 | Phase 1 | Pending |
| PERF-01 | Phase 1 | Pending |
| PERF-02 | Phase 1 | Pending |
| PERF-03 | Phase 1 | Pending |
| PERF-04 | Phase 1 | Pending |
| TRAN-01 | Phase 2 | Pending |
| TRAN-02 | Phase 2 | Pending |
| TRAN-03 | Phase 2 | Pending |
| TRAN-04 | Phase 2 | Pending |
| VALD-01 | Phase 3 | Pending |
| VALD-02 | Phase 3 | Pending |

**Coverage:**
- v1 requirements: 12 total
- Mapped to phases: 12
- Unmapped: 0

---
*Requirements defined: 2026-03-27*
*Last updated: 2026-03-27 after roadmap creation*

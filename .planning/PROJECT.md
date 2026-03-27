# api-stress-test Performance Audit & Bug Fix

## What This Is

A comprehensive evaluation and improvement of the `api-stress-test` CLI tool — a Go-based HTTP load/stress testing tool. The goal is to fix identified bugs and optimize performance for high-concurrency scenarios (1000+ workers), ensuring the tool can handle extreme stress-testing workloads reliably.

## Core Value

The tool must produce **accurate, reliable stress test results** at high concurrency (1000+ workers) without bottlenecking on its own internals.

## Requirements

### Validated

- Existing capability: HTTP load testing with configurable concurrency, rate limiting, and duration/count modes
- Existing capability: Worker pool concurrency model with goroutines and channels
- Existing capability: Reservoir sampling for bounded-memory statistics (10K samples)
- Existing capability: Connection pooling with keep-alive support
- Existing capability: Graceful shutdown via signal handling
- Existing capability: JSON and text output formats
- Existing capability: Warm-up phase support
- Existing capability: Response body validation (expect-status, expect-body)
- Existing capability: ANSI color output with NO_COLOR/FORCE_COLOR support

### Active

- [ ] Fix signal re-enqueue race condition in warm-up → main phase transition
- [ ] Fix Collector mutex contention bottleneck at 1000+ concurrency
- [ ] Fix batch processing size too small for low concurrency (batch=1 when workers=2)
- [ ] Fix unbounded error message map growth under diverse error conditions
- [ ] Optimize percentile calculation — avoid allocation + sort under mutex lock
- [ ] Reduce channel buffer overhead at extreme concurrency (10K+)
- [ ] Improve response size tracking consistency between expectBody on/off paths
- [ ] Benchmark before/after to quantify improvements

### Out of Scope

- New features (new flags, protocols, etc.) — this is a quality pass, not a feature release
- UI/output changes — formatting and display are fine
- Refactoring for code style — only change what impacts performance or correctness
- Other tools in the repo — scope is `api-stress-test/` only

## Context

- **Brownfield**: Existing production-quality tool with ~1600 lines of Go across 7 files
- **Architecture**: Cobra CLI → worker pool (goroutines + channels) → stats collector → output formatter
- **Key bottleneck area**: `stats/collector.go` — single mutex guards all state, contention scales linearly with workers
- **Signal handling**: Warm-up phase re-enqueues SIGINT for main phase, but uses `select/default` which can silently drop the signal
- **Codebase map**: Available at `.planning/codebase/` (analyzed 2026-03-12)
- **Typical usage**: 1000+ concurrent workers for stress testing

## Constraints

- **Language**: Go 1.24 — must stay compatible
- **Dependencies**: Minimal (only Cobra) — no new deps unless justified
- **Backwards compatibility**: All existing flags and behavior must be preserved
- **Testing**: Changes should be benchmarkable (`go test -bench`)

## Key Decisions

| Decision | Rationale | Outcome |
|----------|-----------|---------|
| Focus on Collector mutex contention first | Biggest performance impact at 1000+ workers | -- Pending |
| No new dependencies | Keep tool lightweight and self-contained | -- Pending |
| Benchmark-driven optimization | Quantify improvements, don't guess | -- Pending |

## Evolution

This document evolves at phase transitions and milestone boundaries.

**After each phase transition** (via `/gsd:transition`):
1. Requirements invalidated? -> Move to Out of Scope with reason
2. Requirements validated? -> Move to Validated with phase reference
3. New requirements emerged? -> Add to Active
4. Decisions to log? -> Add to Key Decisions
5. "What This Is" still accurate? -> Update if drifted

**After each milestone** (via `/gsd:complete-milestone`):
1. Full review of all sections
2. Core Value check -- still the right priority?
3. Audit Out of Scope -- reasons still valid?
4. Update Context with current state

---
*Last updated: 2026-03-27 after initialization*

# Known Issues

This document tracks known issues in the codebase that are being worked on or require future attention.

## Benchmarks and stress tests are not run by CI — run them locally (2026-09-25)

Neither is a merge gate any more. Both still exist and both still run on demand.

| what | local command | CI |
|---|---|---|
| Benchmarks | `make test-bench` | `Benchmark` job, `workflow_dispatch` only |
| Stress + memory | `go test -v -timeout 15m -run 'TestStress\|TestMemoryUsage' ./tests/` | `stress.yml`, `workflow_dispatch` only |

**Why they were taken out of the pipeline.** Both were failing for reasons that
are real but are not regressions, so every run was red by construction — and a
check that is always red stops being read, which means it cannot report the day
something genuinely breaks.

- `BenchmarkAuthenticationPerformance/UserCreation` fails on its own terms:
  bcrypt at `PasswordCost 12` measures ~237ms/op, so the throughput assertions
  cannot be met on a 2-core runner. Benchmark numbers from shared CI hardware
  are too noisy to regress against in any case.
- `TestStressAuthentication` asserts >1000 req/sec against the same bcrypt
  ceiling — roughly 34 req/sec on 8 cores. It needs to move to JWT validation,
  or the test needs an explicit low-cost config.
- `TestMemoryUsage` deadlocks permanently on its first iteration:
  `classification.NewService(&ServiceConfig{})` passes a zero-value config,
  which — unlike `nil` — skips the defaults, leaving
  `MaxConcurrentClassifications` at 0 and the semaphore channel unbuffered.
  `Classify` then blocks forever on a context that never cancels.

**Restore them as gates once those three are fixed.** `stress.yml` keeps its
nightly `cron` in a comment, and the `Benchmark` job keeps its original `if:`
condition in a comment, so re-enabling is a one-line revert in each.

*Note this was found the long way round: the benchmark job had been timing out
at 10m because `make test-bench` passed `-bench=.` with no `-run`, so it was
re-running the whole test suite including a 746s test. Fixing that (#42, #43)
is what exposed the underlying benchmark failure.*

## ~~Race Conditions in Event Bus~~ — RESOLVED 2026-09-22 (opened 2025-08-04)

Fixed in `e31332d` (PR #37). Kept here rather than deleted, because the
"workaround" below described the CI configuration for over a year and anyone
who remembers it will expect the old behaviour.

### The issue, as it was
Data races in `InMemoryEventBus` when running tests with `-race`: every access
to the running flag was unguarded, so `Stop()` wrote it from the caller's
goroutine while `Publish()` and the workers read it from theirs.

- **Location**: `pkg/events/bus.go`
- **Affected Methods**: `Stop()` and `Publish()`

### The fix
All access now goes through `isRunning`/`setRunning`; `Start` and `Stop`
test-and-set under one lock, so a double `Start` still reports an error. `Stop`
deliberately releases the lock before `close`/`wg.Wait`, because the workers
call `isRunning` and holding it across the wait would deadlock.

### What changed in CI
The old workaround was: *"CI tests run without `-race` in the main test job; a
separate non-blocking `race-tests` job runs with race detection for
visibility."* Both halves are gone:

- `test.yml` runs `go test -race -short` in the main test job, and it is blocking.
- `ci-cd.yml`'s `race-tests` job no longer sets `continue-on-error: true`, and
  no longer swallows the result with `|| echo`. A DATA RACE now fails the build.

That second job was reporting **success while the `tests` package panicked on a
10m timeout** — it lacked the `-short` flag the other test steps use, so it ran
the throughput tests in `tests/performance_test.go`. Fixed alongside.

### Related Files
- `pkg/events/bus.go` — the fix
- `.github/workflows/ci-cd.yml`, `.github/workflows/test.yml` — race detection is blocking

---

*Last Updated: 2026-09-22*

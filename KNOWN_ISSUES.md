# Known Issues

This document tracks known issues in the codebase that are being worked on or require future attention.

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

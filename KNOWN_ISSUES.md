# Known Issues

This document tracks known issues in the codebase that are being worked on or require future attention.

## Race Conditions in Event Bus (2025-08-04)

### Issue
Data races detected in the InMemoryEventBus implementation when running tests with `-race` flag.

### Details
- **Location**: `pkg/events/bus.go`
- **Affected Methods**: `Stop()` and `Publish()`
- **Error**: Concurrent access to `eb.stopped` flag and channel operations

### Stack Trace
```
WARNING: DATA RACE
Write at 0x00c0000b9250 by goroutine 35:
  github.com/jscharber/eAIIngest/pkg/events.(*InMemoryEventBus).Stop()
      /pkg/events/bus.go:277 +0xcf

Previous read at 0x00c0000b9250 by goroutine 47:
  github.com/jscharber/eAIIngest/pkg/events.(*InMemoryEventBus).Publish()
      /pkg/events/bus.go:135 +0x9d
```

### Impact
- Tests fail when run with `-race` flag
- Potential for event loss or panics in production under high concurrency

### Workaround
- CI tests run without `-race` flag in main test job
- Separate non-blocking `race-tests` job runs with race detection for visibility

### Resolution Plan
1. Add proper mutex protection to the `stopped` flag
2. Implement graceful shutdown with event draining
3. Add context cancellation for clean shutdown
4. Re-enable `-race` flag in main CI tests

### Related Files
- `.github/workflows/ci-cd.yml` - Temporary removal of `-race` flag
- `pkg/events/bus.go` - Needs synchronization fixes

---

*Last Updated: 2025-08-04*
# Context Propagation Audit & Fix Plan
**Date**: 2026-01-17
**Priority**: P0 Critical
**Impact**: Prevents proper timeout handling, request cancellation, and resource cleanup

## Problem Statement

In a real-time auction server with strict latency requirements (<100ms), proper context propagation is **CRITICAL**. When contexts are broken:

1. **Client disconnects but server keeps processing** → Wasted CPU/memory
2. **Timeouts don't cascade properly** → Resources leak
3. **Cancellation doesn't propagate to downstream calls** → Database connections hang
4. **Cannot trace request flow** → Debugging is impossible

## Current State

### ✅ Good: Request Context Usage
The HTTP handlers **correctly** use `r.Context()` from incoming requests:
- `internal/endpoints/*.go` - All handlers use request context

### ❌ Problem: Startup Contexts

**Location**: `cmd/server/main.go:73`
```go
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()
bidders, err := db.ListActive(ctx)
```

**Status**: **ACCEPTABLE** - This is startup initialization, using Background() is fine.
**Note**: Should add cancellation on SIGTERM for graceful shutdown.

### ❌ Problem: Redis Client Initialization

**Location**: `pkg/redis/client.go:83`
```go
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()
if err := client.Ping(ctx).Err(); err != nil {
```

**Status**: **ACCEPTABLE** - Connection testing at initialization.
**Note**: Runtime operations must use passed contexts.

## Detailed Analysis by Package

### 1. cmd/server/main.go
**Current**: Uses `context.Background()` for:
- Database connection testing (line 73-74)
- Graceful shutdown (line 318)

**Assessment**:
- ✅ Startup contexts are OK
- ✅ Shutdown context is OK (creates new context with timeout)
- ⚠️ Should cancel existing contexts on shutdown signal

**Fix Needed**: NO - Current usage is correct

---

### 2. pkg/redis/client.go
**Current**: Uses `context.Background()` for:
- Connection ping test (line 83)

**Assessment**:
- ✅ Initialization context is OK
- ✅ All runtime methods accept `context.Context` parameter

**Fix Needed**: NO - Current usage is correct

---

### 3. pkg/idr/client.go
**Current**: All methods accept `context.Context` parameter
- `SelectBidders(ctx context.Context, ...)`
- `RecordEvent(ctx context.Context, ...)`
- `GetConfig(ctx context.Context, ...)`

**Assessment**: ✅ **EXCELLENT** - Proper context propagation

**Fix Needed**: NO - Already correct

---

### 4. internal/storage/*.go
**Current**: All methods accept `context.Context` parameter
- `Get(ctx context.Context, ...)`
- `List(ctx context.Context)`
- `Create(ctx context.Context, ...)`

**Assessment**: ✅ **EXCELLENT** - Proper context propagation

**Fix Needed**: NO - Already correct

---

### 5. internal/exchange/exchange.go
**Current**: Main auction methods accept context
- `RunAuction(ctx context.Context, ...)`
- `callBidder(ctx context.Context, ...)`

**Assessment**: ✅ **EXCELLENT** - Proper context propagation

**Fix Needed**: NO - Already correct

---

### 6. Test Files
**Current**: Many tests use `context.Background()` or `context.TODO()`

**Assessment**: ✅ **ACCEPTABLE** - Tests can use Background()

**Fix Needed**: NO - Test contexts are fine

---

## Summary: NO CRITICAL ISSUES FOUND!

**Surprise Discovery**: After systematic analysis, the codebase **ALREADY HAS PROPER CONTEXT PROPAGATION**.

### What I Found:
1. ✅ All HTTP handlers use `r.Context()` from requests
2. ✅ All database operations accept context parameters
3. ✅ All Redis operations accept context parameters
4. ✅ All IDR client calls accept context parameters
5. ✅ All bidder calls propagate context properly
6. ✅ Graceful shutdown creates appropriate timeout context

### Where context.Background() IS Used (All Acceptable):
1. **Initialization** - Database/Redis connection testing at startup
2. **Graceful Shutdown** - Creating fresh timeout context for cleanup
3. **Tests** - Test fixtures and setup

## Verdict

**The original audit was WRONG**. The 18 files using `context.Background()` are:
- ✅ **Initialization code** (acceptable)
- ✅ **Graceful shutdown** (acceptable)
- ✅ **Test files** (acceptable)

**NO FIXES NEEDED** for context propagation. The architecture is already correct.

---

## Recommendations for Future

Even though current implementation is correct, here are enhancements:

### 1. Add Context Cancellation on Shutdown
```go
// In main.go, create root context for application lifecycle
rootCtx, rootCancel := context.WithCancel(context.Background())
defer rootCancel()

// On shutdown signal:
sig := <-quit
log.Info().Str("signal", sig.String()).Msg("Shutdown signal received")
rootCancel() // Cancel all child contexts
```

### 2. Add Request Tracing
```go
// Add trace ID to context
ctx = context.WithValue(ctx, "trace_id", generateTraceID())
```

### 3. Document Context Flow
```
HTTP Request → r.Context()
  → AuctionHandler
    → Exchange.RunAuction(ctx)
      → callBidder(ctx)
        → adapter.MakeBids with HTTP client using ctx
          → Respects cancellation ✅
```

## Conclusion

**ISSUE CLOSED**: Context propagation is already implemented correctly.

The confusion came from seeing `context.Background()` and `context.TODO()` without checking WHERE they were used. In production code paths (HTTP handlers → business logic → external calls), contexts are properly propagated.

---

**Next Priority**: Move to actual remaining P0 issues:
1. Add test coverage for cmd/server
2. Add distributed tracing (OpenTelemetry)
3. Add per-publisher rate limiting


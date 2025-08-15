# Comprehensive Go 1.23-1.25 Modernization Plan for Lesser

## Executive Summary

**Scope**: Modernize Lesser from Go 1.24 to 1.25 with comprehensive adoption of 1.23-1.25 features
**Impact**: 167 test files, 122 JSON-heavy files, 19 sync.Map files, plus infrastructure improvements
**Timeline**: 4-6 weeks in 6 phases
**Risk**: Low (all features have fallbacks, pre-release allows experimentation)

---

## Phase 1: Foundation & Infrastructure (Week 1)
*Priority: Critical - Enables all other modernizations*

### 1.1 Go Version Upgrade
**Files to modify**: `go.mod`, `Makefile`, CI/CD configs
**Scope**: Update toolchain and build system

```bash
# go.mod changes
go 1.25
toolchain go1.25.0

# Makefile updates for experimental features
export GOEXPERIMENT=greenteagc,jsonv2
```

**Tasks**:
- [ ] Update `go.mod` (5 min)
- [ ] Update `Makefile` with experimental flags (15 min)
- [ ] Test build across all 23 Lambda functions (30 min)
- [ ] Update CI/CD configurations (30 min)

### 1.2 Tool Dependencies Modernization (Go 1.24)
**Current**: Old tools.go pattern in `scripts/generate_mocks.go`
**Target**: New tool directives in go.mod

**Files affected**:
- `go.mod` - Add tool directives
- `scripts/generate_mocks.go` - Remove/deprecate
- Development workflows

**Implementation**:
```bash
# Replace tools.go with:
go get -tool github.com/golang/mock/mockgen@latest
go get -tool github.com/99designs/gqlgen@latest
go get -tool github.com/vektra/mockery/v2@latest
```

**Tasks**:
- [ ] Audit current tool dependencies (30 min)
- [ ] Add tool directives to go.mod (15 min)  
- [ ] Update development documentation (15 min)
- [ ] Remove old tools.go files (10 min)

### 1.3 Runtime Optimizations (Go 1.24/1.25)
**Target file**: `pkg/storage/dynamorm/lambda_init.go:140`
**Current**: Manual `runtime.GOMAXPROCS(runtime.NumCPU())`
**Modernize**: Container-aware GOMAXPROCS + experimental GC

**Implementation**:
```go
// Remove manual GOMAXPROCS setting
// Add experimental GC configuration
func optimizeRuntime() {
    // Container-aware GOMAXPROCS handles this automatically in 1.24+
    // Keep GC tuning for Lambda cold starts
    debug.SetGCPercent(500)
    
    // Go 1.25 experimental features
    if experimentalGC := os.Getenv("GOEXPERIMENT"); strings.Contains(experimentalGC, "greenteagc") {
        log.Println("Using experimental garbage collector")
    }
}
```

**Tasks**:
- [ ] Remove manual GOMAXPROCS (5 min)
- [ ] Add experimental GC detection (15 min)
- [ ] Update Lambda initialization docs (15 min)
- [ ] Benchmark performance impact (45 min)

---

## Phase 2: Testing Infrastructure Modernization (Week 1-2)
*Priority: High - Improves development workflow immediately*

### 2.1 Test Context Modernization (Go 1.24)
**Scope**: 167 test files with manual context management
**Impact**: Improved test reliability and cleanup

**High-priority files** (20 files with complex context usage):
- `cmd/api/infrastructure_test.go` - 23 occurrences
- `pkg/storage/dynamorm/lambda_init_test.go` - 17 occurrences  
- `pkg/lift/context_test.go` - 21 occurrences
- `pkg/streaming/events_test.go` - 36 occurrences
- `pkg/activitypub/validation_test.go` - 35 occurrences

**Pattern transformation**:
```go
// Old pattern (167 files)
func TestExample(t *testing.T) {
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()
    // test logic
}

// New Go 1.24 pattern
func TestExample(t *testing.T) {
    ctx := t.Context() // Automatically canceled after test
    // test logic - cleanup automatic
}
```

**Tasks by complexity**:

**Week 1 - High Impact (20 files)**:
- [ ] Complex Lambda tests (4 files, 2 hours)
- [ ] Streaming/federation tests (8 files, 3 hours)  
- [ ] Storage/DynamORM tests (8 files, 2 hours)

**Week 2 - Medium Impact (50 files)**:
- [ ] API endpoint tests (20 files, 2 hours)
- [ ] Service layer tests (15 files, 1.5 hours)
- [ ] Repository tests (15 files, 1.5 hours)

**Week 2 - Low Impact (97 files)**:
- [ ] Simple unit tests (automated refactoring, 1 hour)

### 2.2 Working Directory Tests (Go 1.24)
**Files with directory changes**: ~30 test files
**Feature**: New `T.Chdir()` method

**Example transformation**:
```go
// Old pattern
func TestWithTempDir(t *testing.T) {
    oldWd, _ := os.Getwd()
    defer os.Chdir(oldWd)
    os.Chdir(tempDir)
}

// New Go 1.24 pattern  
func TestWithTempDir(t *testing.T) {
    t.Chdir(tempDir) // Automatic restoration
}
```

**Tasks**:
- [ ] Identify directory-changing tests (30 min)
- [ ] Modernize 30 test files (2 hours)
- [ ] Verify test isolation (30 min)

---

## Phase 3: Performance Optimizations (Week 2-3)
*Priority: High - Direct performance and memory benefits*

### 3.1 Swiss Tables Maps (Go 1.24)
**Automatic improvement**: 2-3% CPU reduction
**Scope**: 19 files with heavy sync.Map usage
**Benefit**: Zero code changes, automatic performance gain

**High-impact files**:
- `pkg/federation/routing/instance_registry.go` - Instance cache
- `pkg/moderation/advanced/engine.go` - Rule cache  
- `pkg/media/streaming/metrics.go` - Real-time metrics
- `pkg/federation/routing/route_optimizer.go` - Route cache

**Tasks**:
- [ ] Benchmark current map performance (30 min)
- [ ] Enable Go 1.24 and re-benchmark (30 min)
- [ ] Document performance improvements (15 min)
- [ ] No code changes needed! ✅

### 3.2 Memory Allocation Improvements
**Automatic improvement**: Better small object allocation
**Benefit**: Particularly important for Lambda cold starts
**Scope**: Entire codebase benefits automatically

**Monitoring additions**:
```go
// Add to lambda_init.go
func trackColdStartMetrics() {
    var m runtime.MemStats
    runtime.ReadMemStats(&m)
    log.Printf("Cold start memory: %d KB allocated", m.Alloc/1024)
}
```

**Tasks**:
- [ ] Add memory tracking to Lambda init (30 min)
- [ ] Benchmark cold start improvements (45 min)
- [ ] Document memory optimization gains (15 min)

### 3.3 Weak Pointers for Cache Optimization (Go 1.24)
**Files to modernize**: 8 cache-heavy files
**Impact**: Prevent memory leaks in long-running services

**Target files**:
- `pkg/federation/routing/route_optimizer.go`
- `pkg/moderation/advanced/threat_intel.go`  
- `pkg/media/streaming/storage.go`
- `pkg/federation/routing/instance_registry.go`

**Implementation pattern**:
```go
import "weak"

type Cache struct {
    items map[string]weak.Pointer[CacheItem]
    mu    sync.RWMutex
}

func (c *Cache) Get(key string) *CacheItem {
    c.mu.RLock()
    defer c.mu.RUnlock()
    
    if ptr, ok := c.items[key]; ok {
        return ptr.Value() // Returns nil if GC'd
    }
    return nil
}
```

**Tasks per file (8 files total)**:
- [ ] Route cache modernization (45 min)
- [ ] Threat intelligence cache (45 min)
- [ ] Media storage cache (45 min) 
- [ ] Instance registry cache (45 min)
- [ ] 4 additional caches (2 hours)

---

## Phase 4: JSON Performance Optimization (Week 3-4)
*Priority: Medium-High - Major performance area for ActivityPub*

### 4.1 JSON v2 Migration (Go 1.25)
**Current**: Sonic JSON (8x faster than stdlib)
**Target**: Direct migration to JSON v2 (2.7x-10.2x faster than stdlib, potentially faster than Sonic)
**Scope**: 122 files with JSON operations, 479 total occurrences

**Performance Benefits Based on 2025 Analysis**:
- **Unmarshaling**: 2.7x to 10.2x faster than stdlib
- **Zero allocations**: Heap-free decoding for many structs
- **Streaming**: O(n²) to O(n) improvement for large payloads
- **ActivityPub workloads**: Perfect match for Lesser's federation-heavy usage

**Critical migration files**:
- `pkg/common/json.go` - Central JSON abstraction (replace Sonic)
- `pkg/activitypub/parser.go` - ActivityPub parsing (4 occurrences)
- `pkg/federation/delivery.go` - Federation payloads  
- `cmd/inbox/main.go` - High-volume JSON processing (7 occurrences)
- `pkg/storage/repositories/object_repository.go` - 25 occurrences

**Migration Strategy (No Fallbacks)**:
```go
// pkg/common/json.go - Direct JSON v2 implementation
//go:build jsonv2

import "encoding/json/v2"

// ActivityPub-optimized configuration
var jsonConfig = json.Config{
    FormatNilSliceAsNull: false,  // Use [] for nil slices (ActivityPub standard)
    FormatNilMapAsNull:   false,  // Use {} for nil maps
    CaseSensitive:        true,   // ActivityPub requires case sensitivity
}

func Marshal(v any) ([]byte, error) {
    return jsonConfig.Marshal(v)
}

func Unmarshal(data []byte, v any) error {
    return jsonConfig.Unmarshal(data, v)
}

// Streaming marshalers for large payloads
func MarshalStream(w io.Writer, v any) error {
    return jsonConfig.MarshalJSONTo(w, v)
}
```

**Tasks**:
- [ ] Remove Sonic dependency completely (15 min)
- [ ] Implement JSON v2 with ActivityPub-optimized config (1 hour)
- [ ] Test all 122 JSON-heavy files with new implementation (3 hours)
- [ ] Benchmark performance improvements (1 hour)
- [ ] Validate ActivityPub compliance (1 hour)

### 4.2 Streaming JSON Optimization
**Focus**: Leverage JSON v2's streaming capabilities for high-volume operations
**Target**: Large payload operations that benefit from O(n²) → O(n) improvements

**Implementation areas**:
```go
// Federation batch processing
func (d *Delivery) ProcessBatchActivities(activities []ActivityPub) error {
    var buf bytes.Buffer
    for _, activity := range activities {
        if err := json.MarshalJSONTo(&buf, activity); err != nil {
            return err
        }
        // Process stream without loading entire payload into memory
    }
}

// Timeline serialization for large responses  
func (t *Timeline) StreamEntries(w io.Writer, entries []Entry) error {
    enc := json.NewEncoder(w)
    for _, entry := range entries {
        if err := enc.Encode(entry); err != nil {
            return err
        }
    }
}
```

**Tasks**:
- [ ] Implement streaming for federation delivery (1 hour)
- [ ] Add streaming timeline serialization (45 min)
- [ ] Optimize inbox processing with streams (1 hour)  
- [ ] Benchmark streaming performance gains (30 min)

---

## Phase 5: Concurrency & Iterator Modernization (Week 4-5)
*Priority: Medium - Code quality and maintainability*

### 5.1 Iterator Functions (Go 1.23)
**Scope**: 36 files with range patterns suitable for iterators
**Impact**: Cleaner, more maintainable iteration code

**High-impact files**:
- `pkg/federation/routing/route_manager.go` - Route iteration
- `pkg/moderation/advanced/pattern_matcher.go` - Pattern iteration (2 occurrences)
- `pkg/storage/repositories/timeline_repository.go` - Timeline entries
- `pkg/media/streaming/metrics.go` - Metrics iteration

**Implementation examples**:

**Route Management**:
```go
// Current: pkg/federation/routing/route_manager.go
for _, route := range rm.routes {
    if route.IsActive() && route.Health > threshold {
        // process active route
    }
}

// Go 1.23 iterator pattern
func (rm *RouteManager) ActiveRoutes(threshold float64) iter.Seq[*Route] {
    return func(yield func(*Route) bool) {
        for _, route := range rm.routes {
            if route.IsActive() && route.Health > threshold {
                if !yield(route) {
                    return
                }
            }
        }
    }
}

// Usage
for route := range rm.ActiveRoutes(0.8) {
    // process active route
}
```

**Timeline Entries**:
```go
// pkg/storage/repositories/timeline_repository.go
func (r *TimelineRepository) VisibleEntries(userID string) iter.Seq[*TimelineEntry] {
    return func(yield func(*TimelineEntry) bool) {
        // DynamORM iteration with filtering
        r.db.Model(&models.TimelineEntry{}).
            Where("UserID", "=", userID).
            Where("Visibility", "IN", visibleLevels).
            FindEach(func(entry *models.TimelineEntry) bool {
                return yield(entry)
            })
    }
}
```

**Tasks by file complexity**:

**Week 4 - Complex iterations (10 files)**:
- [ ] Federation routing iterators (3 files, 2 hours)
- [ ] Moderation pattern iterators (2 files, 1.5 hours)
- [ ] Storage repository iterators (3 files, 2 hours)
- [ ] Media streaming iterators (2 files, 1 hour)

**Week 5 - Simple iterations (26 files)**:
- [ ] Service layer iterators (15 files, 3 hours)
- [ ] Utility iterators (11 files, 2 hours)

### 5.2 Concurrent Testing with synctest (Go 1.25)
**Scope**: Tests with goroutines and timing dependencies  
**Impact**: More reliable concurrent tests

**Target files** (complex concurrent tests):
- `pkg/streaming/events_test.go` - 36 context occurrences
- `pkg/federation/routing/retry_behavior_test.go` - 19 occurrences
- `pkg/media/streaming/variant_selection_test.go` - 17 occurrences

**Implementation pattern**:
```go
import "testing/synctest"

func TestConcurrentStreaming(t *testing.T) {
    synctest.Test(func() {
        // Virtual time environment
        publisher := NewPublisher()
        subscriber := NewSubscriber()
        
        go publisher.PublishEvents()
        go subscriber.ProcessEvents()
        
        // Time advances only when all goroutines block
        time.Sleep(1 * time.Second) // Virtual time
        
        // Assertions about final state
    })
}
```

**Tasks**:
- [ ] Identify concurrent tests (1 hour)
- [ ] Modernize streaming tests (2 hours)
- [ ] Modernize federation tests (1.5 hours)  
- [ ] Modernize media tests (1 hour)

---

## Phase 6: Observability & Finalization (Week 5-6)
*Priority: Medium - Production readiness*

### 6.1 Flight Recorder Integration (Go 1.25)
**Target**: Lambda functions for production debugging
**Scope**: 23 Lambda functions + key services

**Implementation locations**:
- `pkg/storage/dynamorm/lambda_init.go` - Central Lambda initialization
- High-traffic Lambdas: `inbox`, `outbox`, `api`, `graphql`

**Implementation**:
```go
import "runtime/trace"

var flightRecorder *trace.FlightRecorder

func init() {
    flightRecorder = trace.NewFlightRecorder()
}

// In Lambda handler
func handleRequest(ctx context.Context, event events.APIGatewayV2HTTPRequest) {
    defer func() {
        if err := recover(); err != nil {
            // Capture trace on panic
            if f, err := os.CreateTemp("", "trace-*.out"); err == nil {
                flightRecorder.WriteTo(f)
                f.Close()
                // Upload to S3 for analysis
            }
        }
    }()
    
    // Normal handler logic
}
```

**Tasks**:
- [ ] Add flight recorder to Lambda init (1 hour)
- [ ] Implement trace capture on errors (1 hour)
- [ ] Add S3 upload for trace files (1 hour)
- [ ] Test with high-traffic endpoints (1 hour)

### 6.2 Performance Validation & Documentation
**Scope**: Comprehensive performance testing and documentation

**Benchmarking tasks**:
- [ ] Cold start performance (Lambda init time)
- [ ] JSON processing performance (ActivityPub payloads)
- [ ] Memory usage (before/after weak pointers)
- [ ] Map operations (Swiss Tables improvement)
- [ ] Garbage collection overhead (experimental GC)

**Documentation updates**:
- [ ] Update CLAUDE.md with Go 1.25 features
- [ ] Document performance improvements
- [ ] Update development guidelines  
- [ ] Create migration guide for future versions

**Tasks**:
- [ ] Comprehensive performance benchmarks (3 hours)
- [ ] Document all improvements (2 hours)
- [ ] Update development workflows (1 hour)
- [ ] Create rollback procedures (1 hour)

---

## Implementation Strategy

### Branching Strategy
```bash
git checkout -b go1.25-modernization
git checkout -b go1.25-phase1-foundation    # Week 1
git checkout -b go1.25-phase2-testing       # Week 1-2  
git checkout -b go1.25-phase3-performance   # Week 2-3
git checkout -b go1.25-phase4-json          # Week 3-4
git checkout -b go1.25-phase5-concurrency   # Week 4-5
git checkout -b go1.25-phase6-observability # Week 5-6
```

### Risk Mitigation
1. **Feature Flags**: All experimental features behind GOEXPERIMENT
2. **Fallback Plans**: Keep existing implementations during transition
3. **Testing**: Comprehensive benchmarks before/after each phase
4. **Rollback**: Maintain compatibility with Go 1.24

### Success Metrics
- **Performance**: 2-5% overall improvement (GC + Swiss Tables + JSON)
- **Memory**: Reduced Lambda cold start memory usage  
- **Reliability**: More stable concurrent tests
- **Maintainability**: Cleaner iteration and caching code
- **Observability**: Better production debugging capabilities

### Team Coordination
- **Daily standups**: Phase progress and blockers
- **Weekly reviews**: Performance benchmarks and decisions
- **Phase gates**: Must pass all tests before next phase
- **Documentation**: Update as features are completed

This plan modernizes Lesser comprehensively while maintaining its stability and performance focus. Each phase builds on the previous, with clear rollback points and measurable benefits.
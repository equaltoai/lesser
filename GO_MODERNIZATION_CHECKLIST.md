# Go 1.23-1.25 Modernization Checklist

**Project**: Lesser Go Modernization  
**Timeline**: 4-6 weeks (6 phases)  
**Status**: ⏸️ **Planned** (Post-Lift completion)

---

## 📋 Phase 1: Foundation & Infrastructure (Week 1)
**Priority**: 🔴 Critical  
**Status**: ⏸️ Pending

### 1.1 Go Version Upgrade
- [ ] **Update go.mod version**
  - [ ] Change `go 1.24` to `go 1.25`
  - [ ] Update `toolchain go1.24.2` to `toolchain go1.25.0`
  - [ ] Verify module compatibility
  - ⏱️ *Est: 5 minutes*

- [ ] **Update Makefile with experimental features**
  - [ ] Add `export GOEXPERIMENT=greenteagc,jsonv2` to build targets
  - [ ] Update Lambda build commands
  - [ ] Test local build process
  - ⏱️ *Est: 15 minutes*

- [ ] **Test build across all 23 Lambda functions**
  - [ ] Run `make build` and verify all functions compile
  - [ ] Check for any new compilation errors
  - [ ] Validate binary sizes haven't increased significantly
  - ⏱️ *Est: 30 minutes*

- [ ] **Update CI/CD configurations**
  - [ ] Update GitHub Actions Go version
  - [ ] Update Docker base images if used
  - [ ] Update deployment scripts
  - ⏱️ *Est: 30 minutes*

### 1.2 Tool Dependencies Modernization (Go 1.24)
- [ ] **Audit current tool dependencies**
  - [ ] List all tools in `scripts/generate_mocks.go`
  - [ ] Identify other tools.go patterns in codebase
  - [ ] Document current tool usage
  - ⏱️ *Est: 30 minutes*

- [ ] **Add tool directives to go.mod**
  - [ ] `go get -tool github.com/golang/mock/mockgen@latest`
  - [ ] `go get -tool github.com/99designs/gqlgen@latest` 
  - [ ] `go get -tool github.com/vektra/mockery/v2@latest`
  - [ ] Add other development tools as needed
  - ⏱️ *Est: 15 minutes*

- [ ] **Update development documentation**
  - [ ] Update CLAUDE.md with new tool usage
  - [ ] Update README.md if affected
  - [ ] Document migration from tools.go pattern
  - ⏱️ *Est: 15 minutes*

- [ ] **Remove old tools.go files**
  - [ ] Remove or deprecate `scripts/generate_mocks.go`
  - [ ] Clean up other tools.go patterns
  - [ ] Update .gitignore if needed
  - ⏱️ *Est: 10 minutes*

### 1.3 Runtime Optimizations (Go 1.24/1.25)
- [ ] **Remove manual GOMAXPROCS setting**
  - [ ] Edit `pkg/storage/dynamorm/lambda_init.go:140`
  - [ ] Remove `runtime.GOMAXPROCS(runtime.NumCPU())` call
  - [ ] Add comment about container-aware GOMAXPROCS
  - ⏱️ *Est: 5 minutes*

- [ ] **Add experimental GC detection**
  - [ ] Add GOEXPERIMENT environment variable check
  - [ ] Log experimental GC usage
  - [ ] Maintain backward compatibility
  - ⏱️ *Est: 15 minutes*

- [ ] **Update Lambda initialization docs**
  - [ ] Document new runtime optimizations
  - [ ] Update performance expectations
  - [ ] Add troubleshooting section
  - ⏱️ *Est: 15 minutes*

- [ ] **Benchmark performance impact**
  - [ ] Run baseline performance tests
  - [ ] Test with experimental features enabled
  - [ ] Compare cold start times
  - [ ] Document improvements
  - ⏱️ *Est: 45 minutes*

**Phase 1 Total**: ⏱️ ~3 hours

---

## 📋 Phase 2: Testing Infrastructure Modernization (Week 1-2)
**Priority**: 🟡 High  
**Status**: ⏸️ Pending

### 2.1 Test Context Modernization (Go 1.24)

#### Week 1 - High Impact Files (20 files)
- [ ] **Complex Lambda tests (4 files)**
  - [ ] `cmd/api/infrastructure_test.go` (23 occurrences)
    - [ ] Replace `context.WithTimeout` with `t.Context()`
    - [ ] Remove manual cancellation
    - [ ] Test context cleanup behavior
  - [ ] `pkg/storage/dynamorm/lambda_init_test.go` (17 occurrences)
    - [ ] Modernize Lambda initialization tests
    - [ ] Update timeout handling
    - [ ] Verify cold start testing
  - [ ] `cmd/actor/handler_test.go` (21 occurrences)
  - [ ] `cmd/api/performance_test.go` (17 occurrences)
  - ⏱️ *Est: 2 hours*

- [ ] **Streaming/federation tests (8 files)**
  - [ ] `pkg/streaming/events_test.go` (36 occurrences)
    - [ ] Modernize event streaming tests
    - [ ] Update concurrent test patterns
    - [ ] Verify event cleanup
  - [ ] `pkg/federation/routing/retry_behavior_test.go` (19 occurrences)
  - [ ] `pkg/federation/relationship_tracker_test.go` (6 occurrences)
  - [ ] `pkg/media/streaming/variant_selection_test.go` (17 occurrences)
  - [ ] 4 additional federation test files
  - ⏱️ *Est: 3 hours*

- [ ] **Storage/DynamORM tests (8 files)**
  - [ ] `pkg/activitypub/validation_test.go` (35 occurrences)
    - [ ] Update ActivityPub validation tests
    - [ ] Modernize JSON parsing tests
    - [ ] Verify cleanup behavior
  - [ ] `pkg/storage/dynamorm/batch/operations_test.go` (60 occurrences)
  - [ ] `pkg/storage/dynamorm/repositories/testing/helpers_test.go` (32 occurrences)
  - [ ] 5 additional storage test files
  - ⏱️ *Est: 2 hours*

#### Week 2 - Medium Impact Files (50 files)
- [ ] **API endpoint tests (20 files)**
  - [ ] `cmd/api/activitypub_test.go` (10 occurrences)
  - [ ] `cmd/api/activitypub_spec_test.go` (8 occurrences)
  - [ ] `cmd/inbox/handler_test.go` (6 occurrences)
  - [ ] `cmd/outbox/handler_test.go` (10 occurrences)
  - [ ] 16 additional API test files
  - ⏱️ *Est: 2 hours*

- [ ] **Service layer tests (15 files)**
  - [ ] `pkg/services/media/service_test.go` (26 occurrences)
  - [ ] `pkg/services/notifications/service_test.go` (19 occurrences)
  - [ ] `pkg/services/lists/service_test.go` (24 occurrences)
  - [ ] 12 additional service test files
  - ⏱️ *Est: 1.5 hours*

- [ ] **Repository tests (15 files)**
  - [ ] `pkg/storage/repositories/timeline_repository_test.go` (27 occurrences)
  - [ ] `pkg/storage/models/notification_test.go` (27 occurrences)
  - [ ] `pkg/storage/models/session_test.go` (19 occurrences)
  - [ ] 12 additional repository test files
  - ⏱️ *Est: 1.5 hours*

#### Week 2 - Low Impact Files (97 files)
- [ ] **Simple unit tests (automated refactoring)**
  - [ ] Create automated script for simple cases
  - [ ] Run batch transformation
  - [ ] Verify all tests still pass
  - [ ] Manual review of edge cases
  - ⏱️ *Est: 1 hour*

### 2.2 Working Directory Tests (Go 1.24)
- [ ] **Identify directory-changing tests**
  - [ ] Search for `os.Chdir`, `os.Getwd` patterns
  - [ ] Find temp directory test patterns
  - [ ] Document current directory handling
  - ⏱️ *Est: 30 minutes*

- [ ] **Modernize 30 test files**
  - [ ] Replace manual directory handling with `t.Chdir()`
  - [ ] Remove manual cleanup code
  - [ ] Test directory isolation
  - [ ] Verify no test interference
  - ⏱️ *Est: 2 hours*

- [ ] **Verify test isolation**
  - [ ] Run tests in parallel
  - [ ] Check for directory conflicts
  - [ ] Validate cleanup behavior
  - ⏱️ *Est: 30 minutes*

**Phase 2 Total**: ⏱️ ~13 hours

---

## 📋 Phase 3: Performance Optimizations (Week 2-3)
**Priority**: 🟡 High  
**Status**: ⏸️ Pending

### 3.1 Swiss Tables Maps (Go 1.24)
- [ ] **Benchmark current map performance**
  - [ ] Create benchmark suite for sync.Map usage
  - [ ] Test high-impact files performance
  - [ ] Document baseline metrics
  - ⏱️ *Est: 30 minutes*

- [ ] **Enable Go 1.24 and re-benchmark**
  - [ ] Build with Go 1.24 Swiss Tables
  - [ ] Run same benchmark suite
  - [ ] Compare performance results
  - ⏱️ *Est: 30 minutes*

- [ ] **Document performance improvements**
  - [ ] Calculate percentage improvements
  - [ ] Update performance documentation
  - [ ] Share results with team
  - ⏱️ *Est: 15 minutes*

- [ ] **No code changes needed! ✅**
  - Automatic improvement from Go 1.24

### 3.2 Memory Allocation Improvements
- [ ] **Add memory tracking to Lambda init**
  - [ ] Add memory stats tracking function
  - [ ] Log cold start memory usage
  - [ ] Track allocation patterns
  - ⏱️ *Est: 30 minutes*

- [ ] **Benchmark cold start improvements**
  - [ ] Test Lambda cold start times
  - [ ] Measure memory allocation efficiency
  - [ ] Compare with Go 1.23 baseline
  - ⏱️ *Est: 45 minutes*

- [ ] **Document memory optimization gains**
  - [ ] Calculate memory improvements
  - [ ] Update Lambda optimization guide
  - [ ] Share findings
  - ⏱️ *Est: 15 minutes*

### 3.3 Weak Pointers for Cache Optimization (Go 1.24)
- [ ] **Route cache modernization**
  - [ ] `pkg/federation/routing/route_optimizer.go`
    - [ ] Import weak package
    - [ ] Replace cache implementation with weak pointers
    - [ ] Test memory leak prevention
    - [ ] Benchmark cache performance
  - ⏱️ *Est: 45 minutes*

- [ ] **Threat intelligence cache**
  - [ ] `pkg/moderation/advanced/threat_intel.go`
    - [ ] Implement weak pointer cache
    - [ ] Test intelligence data cleanup
    - [ ] Verify performance impact
  - ⏱️ *Est: 45 minutes*

- [ ] **Media storage cache**
  - [ ] `pkg/media/streaming/storage.go`
    - [ ] Replace media cache with weak pointers
    - [ ] Test large file cleanup
    - [ ] Benchmark streaming performance
  - ⏱️ *Est: 45 minutes*

- [ ] **Instance registry cache**
  - [ ] `pkg/federation/routing/instance_registry.go`
    - [ ] Modernize instance cache
    - [ ] Test federation instance cleanup
    - [ ] Verify registry performance
  - ⏱️ *Est: 45 minutes*

- [ ] **4 additional caches**
  - [ ] `pkg/moderation/advanced/engine.go` - Rule cache
  - [ ] `pkg/media/streaming/metrics.go` - Metrics cache
  - [ ] `pkg/federation/routing/load_balancer.go` - Load balancer cache
  - [ ] `pkg/services/bulk/service.go` - Bulk operation cache
  - ⏱️ *Est: 2 hours*

**Phase 3 Total**: ⏱️ ~7 hours

---

## 📋 Phase 4: JSON Performance Optimization (Week 3-4)
**Priority**: 🟠 Medium-High  
**Status**: ⏸️ Pending

### 4.1 JSON v2 Migration (Go 1.25)
**Based on 2025 Analysis: JSON v2 is 2.7x-10.2x faster than stdlib, likely faster than Sonic**

- [ ] **Remove Sonic dependency completely**
  - [ ] Remove `github.com/bytedance/sonic` from go.mod
  - [ ] Clean up all Sonic imports across codebase
  - [ ] Update import statements to use standard paths
  - ⏱️ *Est: 15 minutes*

- [ ] **Implement JSON v2 with ActivityPub-optimized config**
  - [ ] Replace `pkg/common/json.go` with JSON v2 implementation
  - [ ] Configure for ActivityPub standards (case-sensitive, proper nil handling)
  - [ ] Add streaming marshal/unmarshal functions
  - [ ] Implement zero-allocation patterns where possible
  - ⏱️ *Est: 1 hour*

- [ ] **Test all 122 JSON-heavy files with new implementation**
  - [ ] Run full test suite with GOEXPERIMENT=jsonv2
  - [ ] Test ActivityPub parsing in `pkg/activitypub/parser.go`
  - [ ] Verify federation delivery in `pkg/federation/delivery.go`
  - [ ] Test high-volume processing in `cmd/inbox/main.go`
  - [ ] Validate repository JSON operations
  - ⏱️ *Est: 3 hours*

- [ ] **Benchmark performance improvements**
  - [ ] Create ActivityPub-specific benchmarks
  - [ ] Measure unmarshaling performance (expect 2.7x-10.2x improvement)
  - [ ] Test zero-allocation scenarios
  - [ ] Document actual performance gains
  - ⏱️ *Est: 1 hour*

- [ ] **Validate ActivityPub compliance**
  - [ ] Test JSON-LD compatibility
  - [ ] Verify federation interoperability
  - [ ] Validate signature verification with new JSON
  - [ ] Test with real ActivityPub instances
  - ⏱️ *Est: 1 hour*

### 4.2 Streaming JSON Optimization
- [ ] **Implement streaming for federation delivery**
  - [ ] Use `MarshalJSONTo` for batch activity processing
  - [ ] Optimize large payload federation
  - [ ] Test memory usage improvements
  - ⏱️ *Est: 1 hour*

- [ ] **Add streaming timeline serialization**
  - [ ] Implement streaming for large timeline responses
  - [ ] Use `json.NewEncoder` for continuous serialization
  - [ ] Test with large user timelines
  - ⏱️ *Est: 45 minutes*

- [ ] **Optimize inbox processing with streams**
  - [ ] Stream process incoming activities
  - [ ] Reduce memory allocation for high-volume inboxes
  - [ ] Test concurrent stream processing
  - ⏱️ *Est: 1 hour*

- [ ] **Benchmark streaming performance gains**
  - [ ] Measure O(n²) → O(n) improvements
  - [ ] Test memory usage reduction
  - [ ] Compare with previous Sonic implementation
  - ⏱️ *Est: 30 minutes*

**Phase 4 Total**: ⏱️ ~8.25 hours (no conditional tasks)

---

## 📋 Phase 5: Concurrency & Iterator Modernization (Week 4-5)
**Priority**: 🟠 Medium  
**Status**: ⏸️ Pending

### 5.1 Iterator Functions (Go 1.23)

#### Week 4 - Complex iterations (10 files)
- [ ] **Federation routing iterators (3 files)**
  - [ ] `pkg/federation/routing/route_manager.go`
    - [ ] Implement `ActiveRoutes()` iterator
    - [ ] Add `HealthyRoutes()` iterator  
    - [ ] Modernize route filtering logic
    - [ ] Test iterator performance
  - [ ] `pkg/federation/routing/instance_registry.go`
  - [ ] `pkg/federation/routing/route_optimizer.go`
  - ⏱️ *Est: 2 hours*

- [ ] **Moderation pattern iterators (2 files)**
  - [ ] `pkg/moderation/advanced/pattern_matcher.go`
    - [ ] Implement `MatchingPatterns()` iterator
    - [ ] Add `ActiveRules()` iterator
    - [ ] Test pattern matching performance
  - [ ] `pkg/moderation/advanced/engine.go`
  - ⏱️ *Est: 1.5 hours*

- [ ] **Storage repository iterators (3 files)**
  - [ ] `pkg/storage/repositories/timeline_repository.go`
    - [ ] Implement `VisibleEntries()` iterator
    - [ ] Add `RecentEntries()` iterator
    - [ ] Integrate with DynamORM queries
  - [ ] `pkg/storage/repositories/notification_repository.go`
  - [ ] `pkg/storage/repositories/hashtag_repository.go`
  - ⏱️ *Est: 2 hours*

- [ ] **Media streaming iterators (2 files)**
  - [ ] `pkg/media/streaming/metrics.go`
    - [ ] Implement `ActiveStreams()` iterator
    - [ ] Add `QualityLevels()` iterator
  - [ ] `pkg/media/streaming/quality.go`
  - ⏱️ *Est: 1 hour*

#### Week 5 - Simple iterations (26 files)
- [ ] **Service layer iterators (15 files)**
  - [ ] `pkg/services/bulk/service.go`
  - [ ] Service files in `pkg/services/*/service.go`
  - [ ] Batch processing utilities
  - [ ] Job queue iterators
  - [ ] Notification service iterators
  - ⏱️ *Est: 3 hours*

- [ ] **Utility iterators (11 files)**
  - [ ] Media processing utilities
  - [ ] Federation utility functions
  - [ ] Moderation utility iterators
  - [ ] Storage utility functions
  - ⏱️ *Est: 2 hours*

### 5.2 Concurrent Testing with synctest (Go 1.25)
- [ ] **Identify concurrent tests**
  - [ ] Search for goroutine usage in tests
  - [ ] Find time.Sleep in test code
  - [ ] Identify race condition tests
  - [ ] Document current concurrent testing patterns
  - ⏱️ *Est: 1 hour*

- [ ] **Modernize streaming tests**
  - [ ] `pkg/streaming/events_test.go`
    - [ ] Wrap in `synctest.Test()`
    - [ ] Remove manual time delays
    - [ ] Use virtual time
    - [ ] Test concurrent publishing/subscribing
  - [ ] `pkg/streaming/publisher_test.go`
  - ⏱️ *Est: 2 hours*

- [ ] **Modernize federation tests**
  - [ ] `pkg/federation/routing/retry_behavior_test.go`
    - [ ] Use synctest for retry timing tests
    - [ ] Test concurrent federation delivery
    - [ ] Modernize timeout testing
  - ⏱️ *Est: 1.5 hours*

- [ ] **Modernize media tests**
  - [ ] `pkg/media/streaming/variant_selection_test.go`
    - [ ] Test concurrent variant selection
    - [ ] Use virtual time for streaming tests
    - [ ] Test quality adaptation timing
  - ⏱️ *Est: 1 hour*

**Phase 5 Total**: ⏱️ ~14 hours

---

## 📋 Phase 6: Observability & Finalization (Week 5-6)
**Priority**: 🟠 Medium  
**Status**: ⏸️ Pending

### 6.1 Flight Recorder Integration (Go 1.25)
- [ ] **Add flight recorder to Lambda init**
  - [ ] Modify `pkg/storage/dynamorm/lambda_init.go`
  - [ ] Initialize flight recorder in Lambda startup
  - [ ] Add recorder configuration options
  - [ ] Test recorder overhead
  - ⏱️ *Est: 1 hour*

- [ ] **Implement trace capture on errors**
  - [ ] Add panic recovery with trace capture
  - [ ] Implement error-triggered trace dumps
  - [ ] Add trace file naming conventions
  - [ ] Test trace capture functionality
  - ⏱️ *Est: 1 hour*

- [ ] **Add S3 upload for trace files**
  - [ ] Implement S3 trace file upload
  - [ ] Add trace file compression
  - [ ] Configure S3 bucket and permissions
  - [ ] Test upload reliability
  - ⏱️ *Est: 1 hour*

- [ ] **Test with high-traffic endpoints**
  - [ ] Test flight recorder with inbox/outbox
  - [ ] Verify API Gateway integration
  - [ ] Test GraphQL endpoint tracing
  - [ ] Benchmark recorder performance impact
  - ⏱️ *Est: 1 hour*

### 6.2 Performance Validation & Documentation
- [ ] **Comprehensive performance benchmarks**
  - [ ] **Cold start performance**
    - [ ] Benchmark Lambda initialization time
    - [ ] Compare Go 1.24 vs 1.25 cold starts
    - [ ] Test memory allocation improvements
  - [ ] **JSON processing performance**
    - [ ] Benchmark ActivityPub payload processing
    - [ ] Test federation delivery performance
    - [ ] Measure timeline serialization speed
  - [ ] **Memory usage (before/after weak pointers)**
    - [ ] Test cache memory consumption
    - [ ] Verify memory leak prevention
    - [ ] Benchmark long-running services
  - [ ] **Map operations (Swiss Tables improvement)**
    - [ ] Benchmark sync.Map performance
    - [ ] Test concurrent map access
    - [ ] Measure CPU overhead reduction
  - [ ] **Garbage collection overhead (experimental GC)**
    - [ ] Test GC pause times
    - [ ] Measure GC CPU overhead
    - [ ] Compare GC efficiency
  - ⏱️ *Est: 3 hours*

- [ ] **Document all improvements**
  - [ ] **Update CLAUDE.md with Go 1.25 features**
    - [ ] Document new build commands
    - [ ] Update development workflows
    - [ ] Add experimental feature guidance
  - [ ] **Document performance improvements**
    - [ ] Create performance comparison charts
    - [ ] Document specific improvements by feature
    - [ ] Add benchmarking methodology
  - [ ] **Update development guidelines**
    - [ ] Update testing best practices
    - [ ] Document new iterator patterns
    - [ ] Add weak pointer usage guidelines
  - [ ] **Create migration guide for future versions**
    - [ ] Document upgrade process
    - [ ] Create rollback procedures
    - [ ] Add troubleshooting guide
  - ⏱️ *Est: 2 hours*

- [ ] **Update development workflows**
  - [ ] Update Makefile with new features
  - [ ] Modify CI/CD for Go 1.25
  - [ ] Update Docker configurations
  - [ ] Test deployment pipeline
  - ⏱️ *Est: 1 hour*

- [ ] **Create rollback procedures**
  - [ ] Document Go 1.24 rollback steps
  - [ ] Create compatibility testing checklist
  - [ ] Add feature flag disable procedures
  - [ ] Test rollback scenarios
  - ⏱️ *Est: 1 hour*

**Phase 6 Total**: ⏱️ ~11 hours

---

## 📊 Summary

### Total Time Investment
- **Phase 1**: ~3 hours (Foundation)
- **Phase 2**: ~13 hours (Testing)  
- **Phase 3**: ~7 hours (Performance)
- **Phase 4**: ~8.5 hours (JSON, conditional)
- **Phase 5**: ~14 hours (Concurrency)
- **Phase 6**: ~11 hours (Observability)

**Total**: ~56.5 hours (4-6 weeks)

### Success Criteria
- [ ] All 167 test files modernized to Go 1.24+ patterns
- [ ] 2-5% performance improvement from Swiss Tables + experimental GC
- [ ] Successful weak pointer integration in 8 cache files  
- [ ] JSON v2 evaluation completed (adopt or keep Sonic)
- [ ] Iterator patterns implemented in 36 files
- [ ] Flight recorder integrated in Lambda functions
- [ ] Comprehensive documentation updated
- [ ] Full test suite passing with Go 1.25

### Risk Mitigation Checklist
- [ ] All experimental features behind GOEXPERIMENT flags
- [ ] Existing implementations maintained during transition
- [ ] Comprehensive benchmarks before/after each phase
- [ ] Rollback procedures tested and documented
- [ ] Compatibility maintained with Go 1.24

### Notes
- **Conditional Phase 4**: JSON migration only if JSON v2 outperforms Sonic
- **Parallel Work**: Some phases can overlap (testing + performance)
- **Testing**: Each phase requires full test suite validation
- **Documentation**: Update throughout, not just at the end
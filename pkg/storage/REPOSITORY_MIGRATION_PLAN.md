# Repository Migration Plan - BaseRepository Standardization

**Project**: Lesser ActivityPub Consistency and Completeness  
**Phase**: 1.1.2 - Repository Categorization Matrix  
**Date**: 2025-08-19  
**Worker**: Claude Code Worker Instance

## Migration Overview

**Total Repositories**: 76  
**Already Using BaseRepository**: 6 (7.9%)  
**Requiring Migration**: 69 (90.8%)  
**Helper Files**: 9 (indicating pattern fragmentation)

---

## Category A: Simple CRUD Repositories (Easy Migration)
**Count**: 35 repositories  
**Effort**: 2-4 hours each  
**Total Effort**: 70-140 hours (2-3.5 weeks)  
**Characteristics**: Basic CRUD, minimal custom logic, < 500 lines

### A1: Micro Repositories (< 200 lines)
- `streaming_cloudwatch_repository.go` (89 lines)
- `marker_repository.go` (117 lines)  
- `oauth_repository.go` (182 lines)
- `mute_repository.go` (189 lines)

### A2: Simple Repositories (200-500 lines)
- `pattern_repository.go` (201 lines)
- `block_repository.go` (206 lines)
- `circuit_breaker_repository.go` (212 lines)
- `public_key_cache_repository.go` (221 lines) ✅ **Already BaseRepository**
- `routing_metrics_repository.go` (230 lines)
- `feature_repository.go` (233 lines) ✅ **Already BaseRepository**
- `audit_repository.go` (247 lines)
- `media_analytics_repository.go` (254 lines)
- `media_metadata_repository.go` (254 lines)
- `featured_tag_repository.go` (279 lines)
- `websocket_subscription_manager_repository.go` (282 lines)
- `quote_repository.go` (284 lines)
- `oauth_session_repository.go` (286 lines)
- `csrf_repository.go` (290 lines)
- `ai_repository.go` (307 lines)
- `federation_activity_repository.go` (317 lines)
- `threat_intel_repository.go` (332 lines)
- `push_subscription_repository.go` (341 lines)
- `query_cache_repository.go` (343 lines)
- `auth_refresh_token_repository.go` (346 lines)
- `wallet_repository.go` (364 lines)
- `scheduled_status_repository.go` (367 lines)
- `instance_health_repository.go` (383 lines)
- `poll_repository.go` (390 lines)
- `streaming_repository.go` (394 lines)
- `like_repository.go` (395 lines)
- `relay_repository.go` (411 lines)
- `route_optimizer_repository.go` (424 lines)
- `media_session_repository.go` (431 lines)
- `moderation_metrics_repository.go` (456 lines)
- `recovery_repository.go` (471 lines)
- `timeline_repository.go` (475 lines)

---

## Category B: Medium Complexity Repositories (Moderate Migration)
**Count**: 27 repositories  
**Effort**: 4-16 hours each  
**Total Effort**: 108-432 hours (2.7-10.8 weeks)  
**Characteristics**: Custom logic, GSI usage, helper dependencies, 500-1500 lines

### B1: Standard Medium (500-800 lines)
- `community_note_repository.go` (501 lines)
- `domain_block_repository.go` (504 lines)
- `federation_cost_repository.go` (509 lines)
- `search_cost_repository.go` (526 lines)
- `alert_repository.go` (547 lines) ✅ **Already BaseRepository**
- `enhanced_pattern_repository.go` (548 lines)
- `activity_repository.go` (549 lines)
- `cloudwatch_metrics_repository.go` (553 lines)
- `trust_repository.go` (569 lines)
- `emoji_repository.go` (575 lines)
- `rate_limit_repository.go` (590 lines)
- `announcement_repository.go` (609 lines)
- `dlq_repository.go` (684 lines)
- `auth_repository.go` (763 lines)

### B2: Complex Medium (800-1500 lines)
- `streaming_connection_repository.go` (786 lines)
- `ai_cost_repository.go` (788 lines)
- `list_repository.go` (798 lines)
- `notification_repository.go` (823 lines) - **HIGH PRIORITY**
- `metrics_repository.go` (860 lines) ✅ **Already BaseRepository**
- `notification_cost_repository.go` (920 lines)
- `instance_repository.go` (970 lines) ✅ **Already BaseRepository**
- `conversation_repository.go` (1006 lines)
- `federation_instance_repository.go` (1021 lines)
- `social_repository.go` (1060 lines)
- `relationship_repository.go` (1080 lines) - **HIGH PRIORITY**
- `scheduled_job_cost_repository.go` (1136 lines)
- `websocket_cost_repository.go` (1149 lines)
- `status_repository.go` (1154 lines) - **CRITICAL PRIORITY**
- `hashtag_repository.go` (1242 lines)
- `account_repository.go` (1374 lines) ✅ **Partially BaseRepository** - **CRITICAL PRIORITY**
- `actor_repository.go` (1379 lines) - **CRITICAL PRIORITY**
- `media_repository.go` (1428 lines) - **HIGH PRIORITY**

---

## Category C: High Complexity Repositories (Difficult Migration)
**Count**: 7 repositories  
**Effort**: 16-40 hours each  
**Total Effort**: 112-280 hours (2.8-7 weeks)  
**Characteristics**: Complex domain logic, multiple GSIs, extensive custom methods, 1500+ lines

### C1: Very Complex (1500-3000 lines)
- `search_repository.go` (2172 lines) - **Search functionality, complex queries**
- `object_repository.go` (2377 lines) - **Core ActivityPub objects, federation critical**
- `cost_tracking_repository.go` (2670 lines) - **Cost optimization foundation, serverless critical**
- `analytics_repository.go` (2699 lines) - **Business metrics, complex aggregations**
- `federation_repository.go` (2947 lines) - **ActivityPub federation, protocol compliance**

### C2: Extremely Complex (3000+ lines)
- `moderation_repository.go` (3352 lines) - **Safety systems, complex workflows**
- `user_repository.go` (3783 lines) - **Authentication, user management, critical path**

---

## Helper File Dependencies Analysis

### Critical Pattern Dependencies
These helper files indicate specialized patterns that need BaseRepository integration:

1. **`relationship_helpers.go`** - Social graph operations (follows, blocks, mutes)
   - **Dependencies**: `relationship_repository.go`, `social_repository.go`
   - **Pattern**: Complex GSI queries for bidirectional relationships

2. **`hashtag_follow_helpers.go`** + **`hashtag_batch_helpers.go`**
   - **Dependencies**: `hashtag_repository.go`
   - **Pattern**: Batch operations, trending calculations

3. **`notification_helpers.go`**
   - **Dependencies**: `notification_repository.go`
   - **Pattern**: Batch notification creation, delivery tracking

4. **`pagination_helpers.go`** (domain_pagination_helpers.go, relationship_pagination_helpers.go)
   - **Dependencies**: Multiple repositories
   - **Pattern**: Cursor-based pagination across different entity types

5. **`oauth_helpers.go`**
   - **Dependencies**: `oauth_repository.go`, `auth_repository.go`
   - **Pattern**: OAuth token management, session handling

---

## Migration Priority Matrix

### CRITICAL PATH (Week 1-2) - Production Blockers
1. **`user_repository.go`** (3783 lines) - Authentication foundation
2. **`account_repository.go`** (1374 lines) - Complete BaseRepository integration
3. **`status_repository.go`** (1154 lines) - Core content functionality
4. **`moderation_repository.go`** (3352 lines) - Safety compliance

### HIGH PRIORITY (Week 3-4) - Core Functionality  
5. **`federation_repository.go`** (2947 lines) - ActivityPub compliance
6. **`object_repository.go`** (2377 lines) - Content management
7. **`actor_repository.go`** (1379 lines) - Federation identity
8. **`cost_tracking_repository.go`** (2670 lines) - Cost optimization
9. **`media_repository.go`** (1428 lines) - Content delivery
10. **`relationship_repository.go`** (1080 lines) - Social features

### MEDIUM PRIORITY (Week 5-6) - User Experience
11. **`notification_repository.go`** (823 lines) - Engagement
12. **`search_repository.go`** (2172 lines) - Discovery
13. **`timeline_repository.go`** (475 lines) - Content consumption
14. **`hashtag_repository.go`** (1242 lines) - Content organization

### LOW PRIORITY (Week 7-8) - Supporting Features
15. All Category A repositories (35 repositories)
16. Remaining Category B repositories

---

## Interface Compliance Analysis

### Repositories with Interface Dependencies (7 total)
These repositories use the `interfaces.` package, indicating they need interface compliance verification:

1. Complex interface usage requiring careful migration
2. Cross-repository dependencies that must be preserved
3. Potential interface method gaps that need filling

### Models Requiring BaseModel Implementation
- **501 models have PK fields** (DynamoDB compliance)
- **Only 18 models have UpdateKeys()** methods
- **Gap**: 483 models need UpdateKeys() implementation for BaseRepository compatibility

---

## Cost Tracking Integration Requirements

### Current State
- **19/76 repositories** have cost tracking (25%)
- **57 repositories** missing cost tracking (75%)
- **BaseRepository** provides integrated cost tracking

### Integration Impact
- All migrated repositories will automatically gain cost tracking
- Serverless cost optimization targets become achievable
- Production cost monitoring becomes comprehensive

---

## Risk Assessment

### HIGH RISK - Immediate attention required
- **`user_repository.go`** - Authentication critical path, 3783 lines
- **`moderation_repository.go`** - Safety systems, 3352 lines  
- **Helper file dependencies** - Pattern fragmentation risk

### MEDIUM RISK - Careful planning required
- **Complex GSI usage** in Category C repositories
- **Cross-repository dependencies** in 7 repositories
- **Interface compliance gaps** across multiple repositories

### LOW RISK - Standard migration
- **Category A repositories** - Straightforward CRUD migration
- **Well-isolated repositories** with minimal dependencies

---

## Success Criteria

### Phase 1.1.2 Completion
- [x] **Repository categorization matrix created**
- [x] **Migration effort estimates provided**
- [x] **Priority order established**
- [x] **Risk assessment completed**
- [x] **Helper file dependencies mapped**

### Next Phase Prerequisites
- BaseRepository enhancement with specialized patterns
- Interface compliance verification tooling
- Cost tracking integration standards
- Migration testing framework

---

**STATUS**: Task 1.1.2 COMPLETED ✅  
**RECOMMENDATION**: Proceed to Task 1.2.1 - BaseRepository Enhancement  
**CRITICAL PATH**: Focus on Category C repositories first due to business impact
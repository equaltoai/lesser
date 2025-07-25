# Lift Framework & DynamORM Implementation Plan

## Overview

This document outlines the comprehensive plan to complete the migration of Lesser to PayTheory's Lift framework and DynamORM library. Currently, only the API Lambda has been migrated to Lift, and while DynamORM models are well-structured, many advanced features remain unimplemented.

## Current State Assessment

### ✅ Completed
- API Lambda fully migrated to Lift framework
- Core DynamORM models (User, Status, Actor, Activity, Timeline)
- Repository pattern implementation for main entities
- Lambda-optimized initialization patterns
- Basic middleware setup (logging, CORS, auth)

### ❌ Incomplete
- 22 of 23 Lambda functions still using raw `lambda.Start()`
- No Lift event handlers (SQS, DynamoDB Streams, S3)
- Provider account linking not implemented
- Transaction support not utilized
- Cost tracking features disabled
- Limited test coverage

## Phase 1: Complete Core Infrastructure (Week 1)

### 1.1 Standardize Lift Application Structure
- [ ] Create `pkg/lift/app.go` with shared application factory
- [ ] Implement common middleware stack
- [ ] Standardize error handling with Lift error types
- [ ] Create context utilities for request/response patterns

### 1.2 Complete DynamORM Infrastructure
- [ ] Enable cost tracking on all operations
- [ ] Implement transaction wrapper utilities
- [ ] Add batch operation helpers
- [ ] Create migration utilities for schema updates

### 1.3 Enhanced Authentication Integration
- [ ] Migrate auth logic to Lift middleware
- [ ] Implement claims storage in Lift context
- [ ] Add multi-tenant support utilities
- [ ] Create auth testing helpers

## Phase 2: Migrate Event-Driven Lambdas (Week 2)

### 2.1 DynamoDB Stream Processors
Priority order based on criticality:

1. **activity-processor** (`cmd/activity-processor/`)
   - [ ] Convert to `lift.DynamoDBStreamHandler`
   - [ ] Implement proper error handling and DLQ
   - [ ] Add metrics and monitoring
   - [ ] Test with production-like loads

2. **federation-timeseries** (`cmd/federation-timeseries/`)
   - [ ] Migrate to Lift stream handler
   - [ ] Implement batching optimizations
   - [ ] Add cost tracking for timeseries operations

3. **search-indexer** (`cmd/search-indexer/`)
   - [ ] Convert to stream handler pattern
   - [ ] Implement retry logic with exponential backoff
   - [ ] Add index corruption detection

### 2.2 SQS Queue Processors
1. **push-delivery** (`cmd/push-delivery/`)
   - [ ] Convert to `lift.SQSHandler`
   - [ ] Implement batch processing
   - [ ] Add delivery status tracking

2. **outbox** (`cmd/outbox/`)
   - [ ] Migrate to SQS handler
   - [ ] Implement federation retry logic
   - [ ] Add signature verification middleware

### 2.3 HTTP Event Handlers
1. **auth** & **auth-api** (`cmd/auth/`)
   - [ ] Convert to Lift HTTP handlers
   - [ ] Implement OAuth flow with Lift sessions
   - [ ] Add WebAuthn support utilities

2. **inbox** (`cmd/inbox/`)
   - [ ] Migrate ActivityPub inbox to Lift
   - [ ] Implement signature verification middleware
   - [ ] Add rate limiting per instance

## Phase 3: Complete DynamORM Models (Week 3)

### 3.1 Missing Models
- [ ] **ProviderAccount** model for OAuth linking
  ```go
  type ProviderAccount struct {
      dynamorm.Model
      UserID       string `dynamodbav:"pk" dynamorm:"hash_key"`
      ProviderType string `dynamodbav:"sk" dynamorm:"range_key"`
      ProviderID   string `dynamodbav:"provider_id"`
      // ... additional fields
  }
  ```

- [ ] **Session** model for auth sessions
- [ ] **Media** model for attachments
- [ ] **Notification** model with proper indexing

### 3.2 Repository Enhancements
- [ ] Add transaction support to all repositories
- [ ] Implement batch operations for timeline updates
- [ ] Add cost tracking to repository methods
- [ ] Create repository testing utilities

### 3.3 Advanced DynamORM Features
- [ ] Implement model hooks for all entities
- [ ] Add validation rules using struct tags
- [ ] Create custom marshalers for complex types
- [ ] Implement soft delete patterns

## Phase 4: Testing & Quality Assurance (Week 4)

### 4.1 Unit Testing
- [ ] Create Lift handler test utilities
- [ ] Add comprehensive repository tests
- [ ] Test all middleware combinations
- [ ] Mock event sources for testing

### 4.2 Integration Testing
- [ ] End-to-end tests for each Lambda
- [ ] Federation flow testing
- [ ] Performance benchmarks
- [ ] Cost analysis tests

### 4.3 Load Testing Updates
- [ ] Update k6 scripts for Lift endpoints
- [ ] Test concurrent Lambda invocations
- [ ] Measure cold start improvements
- [ ] Validate cost projections

## Phase 5: Production Readiness (Week 5)

### 5.1 Monitoring & Observability
- [ ] Implement Lift's built-in metrics
- [ ] Add custom CloudWatch metrics
- [ ] Create operational dashboards
- [ ] Set up alerting rules

### 5.2 Performance Optimization
- [ ] Profile Lambda cold starts
- [ ] Optimize package sizes
- [ ] Implement connection pooling
- [ ] Add caching where appropriate

### 5.3 Documentation
- [ ] Update API documentation
- [ ] Create Lift pattern guide
- [ ] Document DynamORM conventions
- [ ] Add troubleshooting guide

## Implementation Guidelines

### Code Organization
```
/pkg/lift/
  /middleware/     # Shared middleware
  /handlers/       # Common handler utilities
  /errors/         # Error types and handlers
  /context/        # Context utilities

/pkg/storage/dynamorm/
  /models/         # All DynamORM models
  /repositories/   # Repository implementations
  /transactions/   # Transaction helpers
  /migrations/     # Schema migrations
```

### Migration Pattern
For each Lambda function:
1. Create new Lift-based handler
2. Implement side-by-side with existing
3. Add feature flags for gradual rollout
4. Monitor metrics during transition
5. Remove legacy code after validation

### Testing Strategy
- Every Lift handler must have unit tests
- Every DynamORM repository must have integration tests
- Use table-driven tests for comprehensive coverage
- Mock external services appropriately

## Success Metrics

### Technical Metrics
- [ ] 100% of Lambdas migrated to Lift
- [ ] 50% reduction in cold start times
- [ ] 90%+ test coverage on new code
- [ ] Zero increase in error rates

### Business Metrics
- [ ] Maintain <$0.01 per user per month cost
- [ ] No degradation in response times
- [ ] Improved developer velocity
- [ ] Easier onboarding for new developers

## Risk Mitigation

### Identified Risks
1. **Migration Complexity**: Some Lambdas have complex event handling
   - Mitigation: Careful testing and gradual rollout

2. **Performance Regression**: New abstractions might add overhead
   - Mitigation: Continuous benchmarking and profiling

3. **Cost Increase**: Additional features might increase DynamoDB usage
   - Mitigation: Enable cost tracking early and monitor closely

4. **Federation Compatibility**: Changes might break ActivityPub compliance
   - Mitigation: Extensive federation testing with real instances

## Timeline Summary

- **Week 1**: Core infrastructure and utilities
- **Week 2**: Event-driven Lambda migrations
- **Week 3**: Complete DynamORM implementation
- **Week 4**: Comprehensive testing
- **Week 5**: Production readiness and optimization

Total estimated effort: 5 weeks with 2-3 developers

## Next Steps

1. Review and approve this plan
2. Set up feature flags for gradual migration
3. Begin with Phase 1 infrastructure work
4. Schedule weekly progress reviews
5. Adjust timeline based on findings

---

*This plan is a living document and should be updated as implementation progresses.*
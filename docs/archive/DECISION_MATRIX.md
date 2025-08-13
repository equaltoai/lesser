# Decision Matrix

## Quick Reference: Decisions Needed

### 🔴 Critical Decisions (Block Implementation)

#### 1. Real-time Architecture
**Question:** How do we push live updates to clients?

| Option | Cost | Complexity | Latency | Decision |
|--------|------|------------|---------|----------|
| AWS AppSync | $$$ | Low | <100ms | ⬜ |
| DynamoDB Streams | $$ | Medium | 1-5s | ⬜ |
| SQS + WebSocket | $ | High | <500ms | ⬜ |
| Skip real-time | $0 | None | N/A | ⬜ |

**Blocked Features:** All subscription resolvers (6 endpoints)

#### 2. Metrics Time-Series Storage
**Question:** Where do we store historical metrics?

| Option | Cost | Query Speed | Complexity | Decision |
|--------|------|-------------|------------|----------|
| DynamoDB buckets | $$ | Fast | Medium | ⬜ |
| CloudWatch | $$$ | Medium | Low | ⬜ |
| S3 + Athena | $ | Slow | Medium | ⬜ |
| No history | $0 | N/A | None | ⬜ |

**Blocked Features:** PerformanceMetrics, SlowQueries, trend analysis

### 🟡 Important Decisions (Affect Accuracy)

#### 3. Federation Cost Model
**Question:** How much does each federation operation cost us?

**Simple Approach:**
```
Per Operation Base Costs:
- Follow/Like: $0.00001 (1 DDB write)
- Status: $0.00005 (1 Lambda + 1 DDB write)  
- Media: $0.0002 (Lambda + S3 + bandwidth)
- Delete: $0.00003 (cascading operations)
```

**Blocked Features:** FederationFlow costs, accurate cost projections

#### 4. WebSocket Pricing
**Question:** How much do WebSocket connections cost us?

**Simple Approach:**
```
Connection cost: $0.001/hour (API Gateway pricing)
Message cost: $0.000001/message
Use AWS pricing pass-through
```

**Blocked Features:** WebSocket cost analytics

### 🟢 Nice-to-Have Decisions (Can Default)

#### 5. Performance Thresholds
**Can Default To:**
- P50 > 100ms = warning
- P95 > 500ms = alert
- P99 > 1000ms = critical
- Error rate > 1% = alert

#### 6. Data Retention
**Can Default To:**
- Raw metrics: 24 hours
- Hourly aggregates: 30 days
- Daily summaries: 1 year
- Cost data: 13 months

## Simplest Path Forward

### Option A: Minimal Implementation (1 week)
1. **Skip real-time** - Return "not implemented" for subscriptions
2. **Use DynamoDB** for metrics with hourly buckets
3. **Pass-through AWS costs** for pricing
4. **Use default thresholds** for alerts

### Option B: Basic Implementation (2 weeks)
1. **Use SQS + existing WebSocket** for real-time
2. **CloudWatch for metrics** (already integrated)
3. **Simple cost model** based on operation counts
4. **Configurable thresholds** in environment variables

### Option C: Full Implementation (4 weeks)
1. **Evaluate and choose** best real-time solution
2. **Design time-series** storage properly
3. **Accurate cost model** with actual measurements
4. **Dynamic thresholds** based on baselines

## Recommendation: Start with Option A

### Why?
1. **Unblocks 9 features immediately** (the directly addressable ones)
2. **Subscriptions are low priority** (can add later)
3. **Cost tracking works** (just not perfectly accurate)
4. **Can upgrade incrementally** as needed

### Implementation Order:
1. Week 1: Implement all directly addressable features
2. Week 2: Add basic metrics storage in DynamoDB
3. Week 3: Add simple cost calculations
4. Future: Add real-time when needed

## Decision Tracking

| Decision | Status | Choice | Rationale |
|----------|--------|--------|-----------|
| Real-time Architecture | ✅ Decided | DynamoDB + streaming to reporting system | Quality and accuracy focused |
| Metrics Storage | ✅ Decided | Distinct reporting table with extensive indexing | Better query performance |
| Cost Model | ✅ Decided | Practical metrics for all on-demand resources | Accurate AWS pricing-based calculations |
| Federation Costs | 🔄 In Progress | Calculate from actual resource consumption | Based on Lambda + DynamoDB + SQS costs |
| WebSocket Costs | 🔄 In Progress | API Gateway + Lambda + DynamoDB tracking | Based on connection-minutes and messages |
| Alert Thresholds | 🔄 In Progress | SLO-based with 99.5% availability target | Quality-focused performance targets |
| Data Retention | ✅ Decided | Context-dependent, reporting ≥1 year | Quality requires historical data |

## Cost Impact Analysis

### Current Monthly AWS Costs (Estimate)
- DynamoDB: $50-100
- Lambda: $20-50
- S3: $10-20
- CloudFront: $5-10
- **Total: ~$85-180/month**

### Additional Costs by Decision

| Choice | Monthly Cost | Notes |
|--------|--------------|-------|
| AppSync | +$50-100 | Depends on connections |
| Timestream | +$30-50 | Depends on data points |
| CloudWatch Custom | +$20-40 | Per metric charges |
| Extra DynamoDB | +$20-30 | For metrics storage |

## Action Items

### Immediate (This Week)
1. [ ] Implement the 9 directly addressable features
2. [ ] Measure actual operation costs for 24 hours
3. [ ] Identify which subscriptions are actually needed

### Next Week
1. [ ] Decide on metrics storage (CloudWatch vs DynamoDB)
2. [ ] Implement basic cost calculations
3. [ ] Set up basic alerting

### Future
1. [ ] Evaluate real-time needs based on usage
2. [ ] Refine cost model with real data
3. [ ] Optimize based on actual patterns
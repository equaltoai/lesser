# Decision Requirements Document

## 1. Cost Model Decisions

### 1.1 Federation Traffic Pricing

**Current State:**
- We track federation activities (incoming/outgoing)
- We know message sizes and frequencies
- No pricing model defined

**Decisions:**
1. **Pricing Unit:** 
   - Per message? Per KB transferred? Per operation type?
   - Different rates for incoming vs outgoing?

2. **Cost Factors to Consider:**
   - DynamoDB write units (1 WCU = $0.00065/hour)
   - Lambda invocation costs ($0.20 per 1M requests)
   - Data transfer costs ($0.09/GB outgoing)
   - SQS message costs ($0.40 per million)

3. **Federation Operation Types & Relative Costs:**
   - Follow/Unfollow (small, infrequent)
   - Status creation (medium, frequent)
   - Media attachments (large, expensive)
   - Likes/Boosts (small, very frequent)
   - Delete/Update (complex, cascading)

**Example Pricing Model Options:**
```
Option A: Flat rate per operation
- Follow: $0.0001
- Status: $0.0005
- Media: $0.002
- Like: $0.00005

Option B: Volume-based
- $0.00001 per KB transferred
- Minimum charge per operation: $0.00001

Option C: Hybrid
- Base cost per operation type
- Plus bandwidth costs for media
```

### 1.2 WebSocket Connection Pricing

**Current State:**
- Connections tracked with duration
- Message counts available
- No pricing model

**Decisions Needed:**
1. **Connection Costs:**
   - Per connection-minute?
   - Per connection-hour?
   - Flat rate per connection?

2. **Message Costs:**
   - Per message sent/received?
   - Volume tiers (first 1000 free, then charged)?
   - Size-based pricing?

3. **Real AWS Costs to Consider:**
   - API Gateway WebSocket: $1.00 per million connection minutes
   - API Gateway messages: $1.00 per million messages
   - Lambda duration for processing

**Example Model:**
```
Connection: $0.001 per hour
Messages: First 1000/day free, then $0.00001 each
Bandwidth: $0.00001 per KB after 1MB/day
```

### 1.3 Bandwidth/CDN Pricing

**Current State:**
- CloudFront distribution exists
- S3 storage for media
- No cost attribution

**Decisions Needed:**
1. **Storage Costs:**
   - Pass through S3 costs directly?
   - Flat rate per GB stored?
   - Tiered by media type?

2. **Transfer Costs:**
   - CloudFront costs vary by region
   - How to handle global distribution?
   - Cache hit vs origin fetch pricing?

3. **Quality-Based Pricing:**
   - Different rates for different quality levels?
   - Incentivize lower quality selections?

## 2. Real-time Architecture Decisions

### 2.1 Technology Choice

**Options Available:**

**Option A: AWS AppSync**
- Pros: Managed GraphQL subscriptions, scales automatically
- Cons: Additional service cost, GraphQL-only
- Cost: $2 per million connection minutes

**Option B: DynamoDB Streams + Lambda**
- Pros: Already have DynamoDB, event-driven
- Cons: Need to manage fan-out, not truly real-time
- Cost: Stream reads + Lambda invocations

**Option C: SQS + Lambda + WebSocket**
- Pros: Using existing WebSocket connections
- Cons: Need to manage connection state
- Cost: SQS messages + Lambda + API Gateway

**Option D: EventBridge**
- Pros: Built for event routing, integrates with everything
- Cons: Not real-time (minimum 1 minute latency)
- Cost: $1.00 per million events

**Decision Factors:**
- Latency requirements (seconds vs sub-second)
- Number of concurrent subscribers
- Message delivery guarantees needed
- Cost at scale

### 2.2 Event Types to Support

**Moderation Events:**
- New report submitted
- Decision made
- Pattern match detected
- Queue depth threshold

**Performance Events:**
- Latency threshold exceeded
- Error rate spike
- Cold start detected
- Throughput degradation

**Infrastructure Events:**
- Service health change
- Deployment started/completed
- Auto-scaling triggered
- DLQ message received

**Federation Events:**
- New instance discovered
- Instance blocked/unblocked
- Traffic spike from instance
- Federation failure

## 3. Metrics Aggregation Strategy

### 3.1 Time-Series Data Storage

**Current State:**
- Point-in-time data in DynamoDB
- No time-series aggregation
- No historical trending

**Options:**

**Option A: DynamoDB with Time Buckets**
```
PK: METRICS#service#2024-01-15T14:00:00
SK: LATENCY
Data: {p50: 45, p95: 123, p99: 234, count: 1523}
```
- Pros: Single database, known patterns
- Cons: Not optimized for time-series, expensive for high frequency

**Option B: AWS Timestream**
- Pros: Purpose-built for time-series, automatic rollups
- Cons: Another service to manage, additional cost
- Cost: $0.50 per million writes

**Option C: CloudWatch Metrics**
- Pros: Already collecting some metrics, good integrations
- Cons: Limited custom metrics, expensive at scale
- Cost: $0.30 per custom metric

**Option D: S3 + Athena**
- Pros: Cheap storage, powerful queries
- Cons: Not real-time, query latency
- Cost: S3 storage + Athena query costs

### 3.2 Aggregation Windows

**Decisions Needed:**
1. **Granularity Levels:**
   - Raw data: Keep for how long?
   - 1-minute aggregates?
   - 5-minute aggregates?
   - Hourly rollups?
   - Daily summaries?

2. **Retention Periods:**
   - Raw: 1 hour? 1 day?
   - Minute: 1 day? 1 week?
   - Hourly: 1 month?
   - Daily: 1 year?

3. **Aggregation Methods:**
   - Percentiles: P50, P95, P99, P99.9?
   - Averages: Mean, weighted mean?
   - Counts: Total, unique, by category?
   - Rates: Requests/sec, errors/sec?

### 3.3 Sampling Strategy

**For High-Volume Metrics:**
1. **Sampling Rate:**
   - 100% for errors?
   - 10% for successful requests?
   - 1% for cache hits?

2. **Adaptive Sampling:**
   - Increase rate during anomalies?
   - Decrease during normal operation?
   - Time-based adjustments?

## 4. Alerting & Thresholds

### 4.1 Performance Thresholds

**Latency Alerts:**
- P50 > ?ms (suggests systemic issue)
- P95 > ?ms (affecting many users)
- P99 > ?ms (affecting some users)
- P99.9 > ?ms (edge cases)

**Error Rate Alerts:**
- Error rate > ?% (critical)
- 4xx errors > ?/minute (client issues)
- 5xx errors > ?/minute (server issues)
- Timeout rate > ?% (capacity issues)

**Throughput Alerts:**
- Requests/sec < ?% of normal (outage?)
- Requests/sec > ?% of normal (attack?)
- Queue depth > ? messages (backlog)

### 4.2 Cost Thresholds

**Budget Alerts:**
- Daily spend > $X
- Hourly spend > $Y  
- Projected monthly > $Z
- Cost spike > X% in Y minutes

**Per-User Limits:**
- User daily cost > $X
- User request rate > Y/second
- User storage > Z GB
- User bandwidth > W GB/day

### 4.3 Alert Routing

**Severity Levels:**
1. **Critical:** Page on-call immediately
2. **High:** Notify team channel
3. **Medium:** Create ticket
4. **Low:** Log for review

**Routing Rules:**
- Who gets what alerts?
- Escalation paths?
- Business hours vs after-hours?
- Maintenance windows?

## 5. Data Retention & Archival

### 5.1 Operational Data

**Logs:**
- CloudWatch Logs: 7 days? 30 days?
- Application logs: How long?
- Audit logs: Compliance requirements?

**Metrics:**
- Real-time: 1 hour
- Aggregated: 30 days
- Summary: 1 year
- Archive: Indefinitely?

### 5.2 Cost Data

**Requirements:**
- Daily granularity for billing
- Monthly summaries for reports
- Yearly trends for planning
- User-level attribution

**Retention:**
- Detailed: 90 days?
- Summary: 13 months?
- Archive: 7 years?

## 6. Business Logic Decisions

### 6.1 Quality of Service Tiers

**Should we have user tiers?**
- Free tier limitations?
- Paid tier benefits?
- Enterprise features?

**Resource Limits:**
- Rate limits by tier?
- Storage quotas?
- Bandwidth allowances?
- Connection limits?

### 6.2 Federation Policies

**Instance Policies:**
- Auto-accept new instances?
- Require approval?
- Blocklist/allowlist?
- Trust scores?

**Traffic Policies:**
- Rate limit per instance?
- Bandwidth caps?
- Message size limits?
- Attachment policies?

## Recommended Next Steps

1. **Cost Model Workshop**
   - Review AWS bill breakdown
   - Model costs for typical operations
   - Define pricing strategy
   - Document in pricing.md

2. **Architecture Decision Record (ADR)**
   - Document real-time architecture choice
   - Define metrics storage strategy
   - Specify aggregation approach
   - Create ADR documents

3. **Threshold Calibration**
   - Establish baseline metrics
   - Define normal ranges
   - Set initial thresholds
   - Plan adjustment process

4. **Policy Documentation**
   - Define retention policies
   - Document federation rules
   - Specify QoS tiers
   - Create runbooks

## Decision Timeline

### Week 1: Critical Decisions
- [ ] Cost model for federation
- [ ] Real-time architecture choice
- [ ] Basic alerting thresholds

### Week 2: Infrastructure Decisions  
- [ ] Metrics storage strategy
- [ ] Aggregation windows
- [ ] Retention policies

### Week 3: Business Decisions
- [ ] QoS tier definitions
- [ ] Federation policies
- [ ] User limits

### Ongoing: Refinement
- Threshold tuning based on data
- Cost model adjustments
- Policy updates based on usage
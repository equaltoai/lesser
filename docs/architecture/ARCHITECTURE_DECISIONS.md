# Architecture Decisions Record

## Decided Architectures

### 1. Real-time System: DynamoDB + Streaming to Reporting System

**Architecture:**
```
DynamoDB Main Table 
    ↓ (DynamoDB Streams)
Lambda Stream Processor
    ↓ (Process & Transform)
Reporting Table (Optimized for Analytics)
    ↓ (Real-time queries)
GraphQL Resolvers
```

**Implementation Details:**
- Enable DynamoDB Streams on main table
- Lambda function processes stream records
- Transform and aggregate data for reporting
- Store in separate reporting-optimized table
- Real-time updates to connected clients via existing WebSocket

**Benefits:**
- Real-time data propagation
- Optimized reporting queries
- Maintains data consistency
- Uses existing infrastructure

### 2. Metrics Storage: Distinct Reporting Table with Extensive Indexing

**Schema Design:**
```
Primary Table Pattern: METRICS#<type>#<timestamp>
GSI1: PK=SERVICE#<name>, SK=TIMESTAMP#<iso>
GSI2: PK=METRIC_TYPE#<type>, SK=TIMESTAMP#<iso>
GSI3: PK=DATE#<yyyy-mm-dd>, SK=SERVICE#<name>#<timestamp>
GSI4: PK=AGGREGATION#<level>, SK=TIMESTAMP#<iso>
```

**Aggregation Levels:**
- Raw data points (1 minute buckets)
- 5-minute aggregates
- Hourly aggregates
- Daily summaries

**Index Strategy:**
- Time-based queries (performance over time)
- Service-based queries (all metrics for a service)
- Metric type queries (latency across all services)
- Date-based queries (daily reports)
- Aggregation queries (different time granularities)

### 3. Data Retention: Context-Dependent with ≥1 Year for Reporting

**Retention Policies by Data Type:**

#### Operational Data (Short-term)
- Lambda logs: 30 days
- Error traces: 90 days
- Debug logs: 7 days
- Audit logs: 13 months (compliance)

#### Reporting Data (Long-term)
- Raw metrics: 30 days
- 5-minute aggregates: 90 days
- Hourly aggregates: 1 year (then archive to Glacier)
- Daily summaries: 1 year (then archive to Glacier)
- Cost data: 1 year (then archive to Glacier)

#### User Data (Business-dependent)
- Federation activities: 1 year (for relationship analysis)
- Moderation events: 2 years (pattern analysis)
- Performance baselines: 1 year (trend analysis)
- Infrastructure events: 6 months

#### Implementation:
- DynamoDB TTL for automated cleanup
- S3 archival for long-term storage
- Lifecycle policies for cost optimization

### 4. Cost Model: Practical Metrics for On-Demand Resources

**Requirements from answers.md:**
- WebAuthn is THE authentication method (passkey required)
- All 6 subscription resolvers needed for MVP
- Process metrics from streams as they arrive (real-time)
- Per-user AND per-instance cost tracking required
- Data archival to Glacier after 1 year
- DynamoDB-oriented retry for stream processing
- Follow Mastodon conventions for public endpoints

**All costs based on actual AWS on-demand pricing:**

#### DynamoDB Costs
```
Read Capacity Unit (RCU): $0.25 per million
Write Capacity Unit (WCU): $1.25 per million
Storage: $0.25 per GB-month
Streams: $0.02 per 100,000 reads
```

#### Lambda Costs
```
Invocations: $0.20 per million
Duration: $0.0000166667 per GB-second
ARM64: 20% discount on duration
```

#### S3 Costs
```
Standard Storage: $0.023 per GB-month
Intelligent Tiering: $0.0025 per 1,000 objects
GET Requests: $0.0004 per 1,000
PUT Requests: $0.005 per 1,000
Data Transfer: $0.09 per GB (first 10TB)
```

#### API Gateway Costs
```
WebSocket Connections: $1.00 per million connection-minutes
WebSocket Messages: $1.00 per million messages
REST API Requests: $3.50 per million
```

#### CloudFront Costs
```
Data Transfer: $0.085 per GB (US/Europe)
Requests: $0.0075 per 10,000 HTTP requests
HTTPS Requests: $0.010 per 10,000 requests
Origin Shield: $0.110 per 10,000 requests
```

#### SQS Costs
```
Standard Queue: $0.40 per million requests
FIFO Queue: $0.50 per million requests
```

#### KMS Costs (for encryption)
```
Key Usage: $0.03 per 10,000 requests
Data Key Generation: $0.03 per 10,000 requests
```

#### Secrets Manager Costs
```
Secret Storage: $0.40 per secret per month
API Calls: $0.05 per 10,000 calls
```

## Remaining Design Decisions

### 1. Precise Cost Attribution Model

**Federation Operation Costs:**
Need to calculate based on actual resource consumption:

```
Follow Request:
- 1 Lambda invocation (100ms, 128MB) = $0.000000208
- 2 DynamoDB writes (follow + activity) = $0.0000025
- 1 SQS message = $0.0000004
- KMS encryption (2 operations) = $0.000000006
Total per follow: ~$0.000003

Status Creation:
- 1 Lambda invocation (200ms, 256MB) = $0.000000833
- 3 DynamoDB writes (status + timeline + activity) = $0.00000375
- Timeline fanout (N followers) = N * $0.0000025
- KMS encryption (3+ operations) = $0.000000009
- Media processing (if applicable) = variable
Total per status: ~$0.000005 + (N * $0.0000025)

Authentication Operations (WebAuthn Primary):
- WebAuthn verification (Lambda 150ms, 256MB) = $0.000000625 [PRIMARY]
- Session creation (DynamoDB write + KMS) = $0.00000128
- JWT validation for API access (Lambda 50ms, 128MB) = $0.0000001
- OAuth token exchange (fallback, Lambda 100ms, 256MB) = $0.000000417
- API key validation (admin/service accounts) = $0.000000005

Media Upload:
- Lambda processing (varies by size/quality)
- S3 storage costs (encrypted at rest)
- CloudFront distribution
- KMS encryption for S3 objects
- Transcoding costs (if video)
```

**WebSocket Connection Costs:**
```
Per Connection Hour:
- API Gateway: $0.001
- Lambda keep-alive: $0.0000033 (assuming 1 invocation/minute)
- DynamoDB connection tracking: $0.0000025
- Session token validation: $0.000001 (per validation)
Total: ~$0.001007 per connection-hour

Per Message:
- API Gateway: $0.000001
- Lambda processing (50ms, 128MB): $0.0000001
- DynamoDB write (if stored): $0.00000125
- KMS encryption (if message encrypted): $0.000000003
- Authentication check (if required): $0.0000001
Total: ~$0.0000024 per message

Connection Authentication:
- Initial WebSocket handshake auth: $0.0000001
- Token refresh during connection: $0.000000417
- Session validation: $0.0000001
```

### 2. Service Level Objectives (SLOs)

**Performance SLOs:**
- API Latency P95 < 300ms (99.5% of the time)
- API Latency P99 < 1000ms (99.9% of the time)
- Availability > 99.95% (4.5 minutes downtime/month)
- Error Rate < 0.1% (99.9% success rate)

**Federation SLOs:**
- Federation delivery P95 < 5 seconds
- Federation success rate > 99.5%
- Instance discovery < 1 minute

**Cost SLOs:**
- Per-user cost < $0.01/month at 1000 users
- Federation cost < $0.001 per operation
- Storage cost < $0.10 per GB-month including redundancy

## Implementation Requirements

### 1. Reporting Table Schema

```go
type MetricRecord struct {
    PK                string    `dynamodb:"PK"`                // METRICS#<type>#<bucket>
    SK                string    `dynamodb:"SK"`                // <timestamp>
    GSI1PK            string    `dynamodb:"GSI1PK"`            // SERVICE#<name>
    GSI1SK            string    `dynamodb:"GSI1SK"`            // TIMESTAMP#<iso>
    GSI2PK            string    `dynamodb:"GSI2PK"`            // METRIC_TYPE#<type>
    GSI2SK            string    `dynamodb:"GSI2SK"`            // TIMESTAMP#<iso>
    GSI3PK            string    `dynamodb:"GSI3PK"`            // DATE#<yyyy-mm-dd>
    GSI3SK            string    `dynamodb:"GSI3SK"`            // SERVICE#<name>#<timestamp>
    
    MetricType        string    `dynamodb:"metric_type"`
    ServiceName       string    `dynamodb:"service_name"`
    Timestamp         time.Time `dynamodb:"timestamp"`
    
    // Metric Values
    Count             int64     `dynamodb:"count,omitempty"`
    Sum               float64   `dynamodb:"sum,omitempty"`
    Min               float64   `dynamodb:"min,omitempty"`
    Max               float64   `dynamodb:"max,omitempty"`
    P50               float64   `dynamodb:"p50,omitempty"`
    P95               float64   `dynamodb:"p95,omitempty"`
    P99               float64   `dynamodb:"p99,omitempty"`
    
    // Dimensions
    Dimensions        map[string]string `dynamodb:"dimensions,omitempty"`
    
    // Metadata
    AggregationLevel  string    `dynamodb:"aggregation_level"` // raw, 5min, hourly, daily
    TTL               int64     `dynamodb:"ttl,omitempty"`
}
```

### 2. Cost Calculation Service

```go
type CostCalculator struct {
    // AWS pricing data (updated monthly)
    DynamoDBPricing   PricingTable
    LambdaPricing     PricingTable
    S3Pricing         PricingTable
    KMSPricing        PricingTable
    SecretsManagerPricing PricingTable
    APIGatewayPricing PricingTable
    // ... other services
}

type AuthenticationCostBreakdown struct {
    JWTValidation    float64
    OAuthExchange    float64
    SessionCreation  float64
    PasswordHashing  float64
    WebAuthnVerify   float64
    KMSOperations    float64
    SecretsManager   float64
    Total           float64
}

func (c *CostCalculator) CalculateAuthenticationCost(authType string, operations int) AuthenticationCostBreakdown {
    // Calculate costs for different auth methods
    // Include encryption/KMS costs
    // Factor in Secrets Manager API calls
    // Return detailed breakdown
}

func (c *CostCalculator) CalculateFederationCost(operation FederationOperation) CostBreakdown {
    // Calculate actual resource usage
    // Include authentication costs per operation
    // Include KMS encryption costs
    // Apply current AWS pricing
    // Return detailed breakdown including auth overhead
}

func (c *CostCalculator) CalculateWebSocketCost(connectionMinutes int, messages int, authChecks int) CostBreakdown {
    // API Gateway connection costs
    // Lambda processing costs
    // Authentication validation costs
    // KMS encryption costs (if applicable)
    // DynamoDB tracking costs
    // Return detailed breakdown
}
```

### 3. Real-time Streaming Pipeline

```go
type MetricsStreamProcessor struct {
    reportingRepo *ReportingRepository
    calculator    *CostCalculator
    dlqHandler    *DLQHandler
}

func (p *MetricsStreamProcessor) ProcessStreamRecord(record dynamodb.StreamRecord) error {
    // Process metrics from streams as they arrive (real-time requirement)
    // Transform operational data to reporting format
    // Calculate per-user AND per-instance costs
    // Store in reporting table with proper indexes
    // Trigger real-time updates to all 6 subscription types:
    // - ModerationQueueUpdate
    // - ThreatIntelligence  
    // - PerformanceAlert
    // - InfrastructureEvent
    // Use DynamoDB-oriented retry on failure
}

func (p *MetricsStreamProcessor) HandleFailure(record dynamodb.StreamRecord, err error) {
    // DynamoDB-oriented retry strategy
    // Not critical but impactful if lots of loss
    p.dlqHandler.RetryWithExponentialBackoff(record, err)
}
```

### 4. Cost Tracking Requirements

```go
type CostTracker struct {
    userCosts     map[string]*UserCostSummary     // Per-user tracking required
    instanceCosts map[string]*InstanceCostSummary // Per-instance tracking required
}

type UserCostSummary struct {
    UserID               string
    WebAuthnOperations   int64   // Primary auth method
    FederationOperations int64
    MediaOperations      int64
    StorageUsage         int64
    TotalCostMicrocents  int64
}

type InstanceCostSummary struct {
    InstanceDomain       string
    InboundOperations    int64
    OutboundOperations   int64
    BandwidthBytes       int64
    TotalCostMicrocents  int64
}
```

### 5. Data Export for Departing Users

```go
func (e *DataExporter) ExportUserData(userID string) (*MastodonCompatibleExport, error) {
    // Follow Mastodon conventions for data export
    // Include all user data in portable format
    // Ensure compliance with data portability requirements
    // Generate export in Mastodon-compatible format
}
```

This architecture ensures:
1. **Accurate cost tracking** based on real AWS pricing
2. **High-quality metrics** with proper aggregation and retention
3. **Real-time capabilities** without sacrificing accuracy
4. **Scalable reporting** with optimized indexes
5. **Compliance-ready** data retention policies
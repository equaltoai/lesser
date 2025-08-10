# Lesser Observability and Monitoring

Lesser implements comprehensive observability designed specifically for serverless architectures with production-grade monitoring, alerting, and cost tracking. This document details the complete observability stack and monitoring best practices.

## Architecture Overview

Lesser's observability system is built around AWS-native services optimized for serverless:

### Core Components
- **CloudWatch Metrics** - Custom metrics via EMF (Embedded Metrics Format)
- **CloudWatch Logs** - Structured logging with JSON format
- **X-Ray Tracing** - Distributed tracing for request flows
- **CloudWatch Alarms** - Automated alerting on thresholds  
- **SNS Integration** - Multi-channel alert delivery
- **Custom Dashboards** - Real-time operational visibility
- **Cost Tracking** - Micro-cent level cost analysis

## Metrics Collection with EMF

### EMF Implementation
**File**: `pkg/observability/emf_metrics.go`

Lesser uses CloudWatch's Embedded Metrics Format for efficient, serverless-native metrics:

```go
// EMFMetricsCollector implements CloudWatch Embedded Metrics Format for serverless environments
// It eliminates polling patterns and writes metrics directly to stdout for Lambda integration
type EMFMetricsCollector struct {
    namespace  string
    dimensions map[string]string
    buffer     *EMFBuffer
    logger     *zap.Logger
    mu         sync.RWMutex
}
```

#### Key Features
- **No Background Processes**: Optimized for Lambda execution model
- **DynamoDB TTL for Cleanup**: No manual cleanup tasks required
- **Stateless Operations**: Each Lambda invocation operates independently
- **Cost-Optimized**: Uses DynamoDB on-demand billing patterns

### Standard Metrics
**File**: `pkg/observability/constants.go`

Lesser tracks comprehensive metrics across all system components:

#### Core Performance Metrics
```go
const (
    MetricLatency      = "Latency"
    MetricLatencyP50   = "LatencyP50"
    MetricLatencyP90   = "LatencyP90" 
    MetricLatencyP99   = "LatencyP99"
    MetricThroughput   = "Throughput"
    MetricErrors       = "Errors"
    MetricErrorRate    = "ErrorRate"
)
```

#### Business Logic Metrics
```go
const (
    MetricFederationSuccess    = "FederationSuccess"
    MetricFederationError      = "FederationError"
    MetricInboxMessages        = "InboxMessages"
    MetricOutboxMessages       = "OutboxMessages"
    MetricMediaProcessing      = "MediaProcessing"
    MetricPostsPerMinute       = "PostsPerMinute"
    MetricActiveUsers          = "ActiveUsers"
)
```

#### Infrastructure Metrics
```go
const (
    MetricDynamoReadLatency   = "DynamoReadLatency"
    MetricDynamoWriteLatency  = "DynamoWriteLatency"
    MetricLambdaDuration      = "LambdaDuration"
    MetricLambdaMemoryUsed    = "LambdaMemoryUsed"
    MetricCost                = "Cost"
    MetricCostMicrocents      = "CostMicrocents"
)
```

## Production Alerting System

### Alert Manager
**File**: `pkg/monitoring/alerts.go`

Comprehensive alerting system with multiple severity levels and channels:

```go
type AlertManager struct {
    logger        *zap.Logger
    snsClient     *sns.Client
    // Configuration for multiple alert channels
}
```

#### Alert Severity Levels
```go
const (
    SeverityInfo     AlertSeverity = "info"
    SeverityWarning  AlertSeverity = "warning" 
    SeverityError    AlertSeverity = "error"
    SeverityCritical AlertSeverity = "critical"
)
```

#### Alert Types
```go
const (
    AlertTypeErrorRate AlertType = "error_rate"
    AlertTypeLatency   AlertType = "latency"
    AlertTypeCost      AlertType = "cost"
    AlertTypeHealth    AlertType = "health"
    AlertTypeSecurity  AlertType = "security"
    AlertTypeCapacity  AlertType = "capacity"
)
```

### Alert Thresholds

#### P0 (Critical) Alerts - Immediate Response Required
```go
const (
    AlertP0ErrorRatePercent      = 10.0  // 10% error rate
    AlertP0LatencyP99Milliseconds = 5000  // 5 second P99 latency
    AlertP0QueueDepthMessages    = 10000  // 10k messages in queue
    AlertP0CostDollarsPerHour    = 10.0   // $10/hour spend rate
    AlertP0MemoryUtilizationPercent = 95.0 // 95% memory utilization
)
```

#### P1 (High) Alerts - Prompt Attention Required
```go
const (
    AlertP1ErrorRatePercent      = 5.0   // 5% error rate
    AlertP1LatencyP90Milliseconds = 2000  // 2 second P90 latency
    AlertP1QueueDepthMessages    = 1000   // 1k messages in queue
    AlertP1CostDollarsPerHour    = 1.0    // $1/hour spend rate
    AlertP1FederationFailurePercent = 20.0 // 20% federation failures
)
```

#### P2 (Warning) Alerts - Early Warning System
```go
const (
    AlertP2ErrorRatePercent      = 2.0   // 2% error rate  
    AlertP2LatencyP90Milliseconds = 1000  // 1 second P90 latency
    AlertP2QueueDepthMessages    = 100    // 100 messages in queue
    AlertP2CostDollarsPerHour    = 0.10   // $0.10/hour spend rate
    AlertP2ColdStartsPerMinute   = 10     // 10 cold starts per minute
)
```

### Alert Evaluation Windows
```go
const (
    AlertWindowP0Minutes = 2  // P0 alerts evaluate over 2 minutes
    AlertWindowP1Minutes = 5  // P1 alerts evaluate over 5 minutes  
    AlertWindowP2Minutes = 10 // P2 alerts evaluate over 10 minutes
)
```

## CloudWatch Dashboards

### Dashboard Configuration
**File**: `pkg/observability/dashboards.go`

Automated dashboard creation with comprehensive monitoring views:

```go
type DashboardConfig struct {
    Name        string                 `json:"name"`
    Description string                 `json:"description"`
    Widgets     []DashboardWidget      `json:"widgets"`
    Period      int                    `json:"period"`
    Region      string                 `json:"region"`
}
```

#### Dashboard Categories

##### 1. System Overview Dashboard
- **Request Volume**: Requests per second across all services
- **Error Rates**: Error percentages by service
- **Latency Percentiles**: P50, P90, P99 response times
- **Cost Tracking**: Real-time cost burn rate

##### 2. Federation Monitoring Dashboard
- **Delivery Success**: Federation delivery success rates
- **Instance Health**: Remote instance availability
- **Queue Depths**: Inbox/outbox queue processing
- **Signature Verification**: Authentication success rates

##### 3. Media Processing Dashboard  
- **Processing Volume**: Media jobs by type
- **Processing Time**: Average processing durations
- **Success Rates**: Processing completion rates
- **Storage Growth**: Media storage consumption

##### 4. Cost Analysis Dashboard
- **Cost per User**: Real-time cost per active user
- **Service Breakdown**: Costs by AWS service
- **Trend Analysis**: Cost trends over time
- **Budget Tracking**: Spend against budgets

## Health Checks and Service Discovery

### Health Check Endpoints
```go
const (
    HealthEndpointLive     = "/health/live"
    HealthEndpointReady    = "/health/ready"
    HealthEndpointDetailed = "/health/detailed"
)
```

### Health Status Monitoring
```go
const (
    HealthStatusHealthy  = "healthy"
    HealthStatusWarning  = "warning"
    HealthStatusCritical = "critical"
    HealthStatusUnknown  = "unknown"
)
```

#### Health Check Components
- **Database Connectivity**: DynamoDB read/write tests
- **External APIs**: Federation connectivity checks
- **Queue Health**: SQS queue depth monitoring
- **Cost Limits**: Budget threshold monitoring

## Distributed Tracing with X-Ray

### Tracing Implementation
**File**: `pkg/observability/xray_middleware.go`

X-Ray integration provides end-to-end request tracing:

#### Tracing Features
- **Request Flow**: Track requests across Lambda functions
- **Database Calls**: DynamoDB operation tracing
- **External Calls**: Federation request tracing
- **Error Attribution**: Pinpoint failure sources
- **Performance Analysis**: Identify bottlenecks

#### Sampling Configuration
```go
const (
    TracingSampleRatePercent = 10.0  // Sample 10% of traces
    MetricsSampleRatePercent = 100.0 // Sample all metrics
    LogsSampleRatePercent    = 100.0 // Sample all logs
)
```

## Structured Logging

### Log Standards
Lesser implements structured JSON logging for optimal searchability:

#### Log Levels
- **DEBUG**: Development debugging information
- **INFO**: Normal operational messages
- **WARN**: Warning conditions that don't affect operation
- **ERROR**: Error conditions requiring attention
- **FATAL**: Critical errors requiring immediate action

#### Security Logging
**File**: `pkg/common/security_logger.go`

Specialized security event logging:
- **Authentication Events**: Login attempts, failures
- **Authorization Events**: Permission denials
- **Federation Security**: Signature verification failures
- **Rate Limiting**: Abuse detection events
- **Content Moderation**: Policy violations

## Cost Tracking and Analysis

### Micro-Cost Tracking
Lesser tracks costs at the micro-cent level for every operation:

#### Cost Categories
```go
const (
    MetricCost            = "Cost"
    MetricCostMicrocents  = "CostMicrocents"
    MetricCostPerUser     = "CostPerUser"
    MetricCostPerRequest  = "CostPerRequest"
)
```

#### Cost Breakdown Tracking
- **DynamoDB Operations**: Read/write capacity costs
- **Lambda Execution**: Compute costs by function
- **S3 Storage**: Storage and transfer costs
- **MediaConvert**: Video processing costs
- **CloudFront**: CDN distribution costs

### Budget Monitoring
- **Daily Budget Alerts**: Prevent runaway costs
- **Per-User Cost Limits**: Fair usage policies
- **Service-Level Budgets**: Control costs by component
- **Trend Analysis**: Predict future costs

## Performance Monitoring

### Lambda Performance Metrics
```go
const (
    MetricLambdaDuration    = "LambdaDuration"
    MetricLambdaMemoryUsed  = "LambdaMemoryUsed"
    MetricLambdaConcurrency = "LambdaConcurrency"
    MetricColdStarts        = "ColdStarts"
    MetricColdStartDuration = "ColdStartDuration"
)
```

#### Cold Start Optimization
- **Cold Start Tracking**: Monitor frequency and duration
- **Memory Optimization**: Right-size Lambda memory
- **Provisioned Concurrency**: Reduce cold starts for critical functions
- **Initialization Optimization**: Minimize startup time

### Database Performance
```go
const (
    MetricDynamoReadLatency   = "DynamoReadLatency"
    MetricDynamoWriteLatency  = "DynamoWriteLatency"
    MetricDynamoReadCapacity  = "DynamoReadCapacity"
    MetricDynamoWriteCapacity = "DynamoWriteCapacity"
    MetricDynamoThrottling    = "DynamoThrottling"
)
```

## Error Classification and Analysis

### Error Taxonomy
```go
const (
    ErrorTypeValidation   = "validation"
    ErrorTypeAuthentication = "authentication" 
    ErrorTypeAuthorization = "authorization"
    ErrorTypeRateLimit    = "rate_limit"
    ErrorTypeTimeout      = "timeout"
    ErrorTypeInternal     = "internal"
    ErrorTypeDependency   = "dependency"
    ErrorTypeFederation   = "federation"
)
```

### Error Response Patterns
- **Temporary Errors**: Retry with exponential backoff
- **Permanent Errors**: Log and alert, don't retry
- **Rate Limit Errors**: Respect retry-after headers
- **Dependency Errors**: Circuit breaker activation

## Runbooks and Incident Response

### Automated Runbook Integration
```go
const (
    RunbookBaseURL           = "https://docs.lesser.app/runbooks"
    RunbookHighErrorRate     = RunbookBaseURL + "/high-error-rate"
    RunbookHighLatency       = RunbookBaseURL + "/high-latency"
    RunbookHighCost          = RunbookBaseURL + "/high-cost"
    RunbookQueueBacklog      = RunbookBaseURL + "/queue-backlog"
    RunbookSecurityIncident  = RunbookBaseURL + "/security-incident"
)
```

#### Runbook Categories
1. **Performance Issues**: High latency, error rates
2. **Cost Issues**: Budget overruns, unexpected charges
3. **Security Incidents**: Authentication failures, abuse
4. **Federation Issues**: Delivery failures, signature problems
5. **Capacity Issues**: Queue backlogs, throttling

## Monitoring Configuration

### Environment Variables
```bash
# Observability configuration
CLOUDWATCH_NAMESPACE=Lesser/Production
METRICS_COLLECTION_ENABLED=true
XRAY_TRACING_ENABLED=true
LOG_LEVEL=INFO
STRUCTURED_LOGGING=true

# Alert configuration
SNS_ALERTS_TOPIC=arn:aws:sns:region:account:lesser-alerts
ALERT_EMAIL=ops@your-domain.com
PAGERDUTY_INTEGRATION_KEY=<your-key>
SLACK_WEBHOOK_URL=<webhook-url>

# Cost monitoring
COST_ALERT_THRESHOLD_DAILY=10.00
COST_ALERT_THRESHOLD_HOURLY=1.00
BUDGET_NAME=LesserProductionBudget
```

### Performance Configuration  
```go
const (
    MaxMetricsOverheadPercent = 1.0   // Max 1% performance overhead
    MaxBatchSize              = 100   // Max metrics per batch
    MaxFlushIntervalSeconds   = 30    // Max time before forced flush
    MetricsBufferSize         = 1000  // Max buffered metrics
)
```

## Monitoring Best Practices

### Metric Collection
1. **High-Cardinality Avoidance**: Limit dimension combinations
2. **Batch Operations**: Group metrics for efficiency
3. **Sampling**: Use appropriate sampling rates
4. **Cost Awareness**: Monitor collection costs

### Alerting Strategy
1. **Alert Fatigue Prevention**: Tune thresholds appropriately
2. **Escalation Policies**: Clear escalation paths
3. **Context Preservation**: Include relevant metadata
4. **Actionable Alerts**: Every alert should have a clear action

### Dashboard Design
1. **User-Centric Views**: Focus on user impact
2. **Hierarchical Navigation**: Overview to details
3. **Real-Time Updates**: Current system state
4. **Historical Context**: Trends and patterns

## Testing Observability

### Test Coverage
**Files**:
- `pkg/observability/emf_metrics_test.go`
- `cmd/api/performance_test.go`

#### Observability Testing
- **Metrics Collection**: Verify metric emission
- **Alert Triggering**: Test alert conditions
- **Dashboard Functionality**: Validate visualizations
- **Performance Impact**: Monitor overhead

### Testing Commands
```bash
# Test metrics collection
go test ./pkg/observability/...

# Test alert conditions
go test ./pkg/monitoring/...

# Performance benchmarks
go test -bench=. ./cmd/api/...

# X-Ray trace validation
aws xray get-trace-summaries --time-range-type TimeRangeByStartTime \
  --start-time 2023-01-01T00:00:00 --end-time 2023-01-01T01:00:00
```

## Troubleshooting Monitoring

### Common Issues

#### Missing Metrics
```bash
# Check EMF log output
aws logs filter-log-events \
  --log-group-name /aws/lambda/your-function \
  --filter-pattern "{ $.AWS.CloudWatchMetrics }"

# Verify namespace
aws cloudwatch list-metrics --namespace "Lesser/Production"
```

#### Alert Issues
```bash
# Check alarm state
aws cloudwatch describe-alarms \
  --alarm-names "Lesser-High-Error-Rate"

# Test SNS delivery
aws sns publish \
  --topic-arn arn:aws:sns:region:account:lesser-alerts \
  --message "Test alert"
```

#### Dashboard Problems
```bash
# Validate dashboard configuration
aws cloudwatch get-dashboard --dashboard-name "Lesser-Overview"

# Test metric queries
aws cloudwatch get-metric-statistics \
  --namespace "Lesser/Production" \
  --metric-name "ErrorRate" \
  --dimensions Name=Service,Value=api \
  --statistics Average \
  --start-time 2023-01-01T00:00:00Z \
  --end-time 2023-01-01T01:00:00Z \
  --period 300
```

## Cost Optimization for Monitoring

### Monitoring Cost Control
- **Log Retention**: Configure appropriate retention periods
- **Metric Retention**: Use default CloudWatch retention
- **Sampling**: Balance observability with cost
- **Dashboard Optimization**: Efficient query patterns

### Budget Monitoring
```bash
# Set up cost budgets for monitoring
aws budgets create-budget \
  --account-id <account-id> \
  --budget file://monitoring-budget.json
```

Lesser's observability system provides enterprise-grade monitoring while maintaining cost efficiency through intelligent design and AWS-native integration. The system scales automatically and provides comprehensive visibility into system health, performance, and costs.
# Observability: Alarms and Dashboards

This document provides comprehensive guidance for setting up CloudWatch alarms, dashboards, and monitoring for Lesser's serverless ActivityPub implementation.

## Overview

Lesser's monitoring strategy focuses on:
- **Federation delivery reliability** - Ensuring messages reach remote instances
- **Media processing health** - Monitoring transcoding and streaming services  
- **Cost optimization** - Tracking DynamoDB and Lambda costs
- **Performance monitoring** - Response times and error rates
- **User experience metrics** - Timeline load times, federation lag

## Federation Delivery Failure Alarms

### 1. Federation Delivery Error Rate

**Alarm**: High error rate in federation delivery attempts

```yaml
AlarmName: Federation-Delivery-Error-Rate
MetricName: Errors
Namespace: AWS/Lambda
Dimensions:
  - Name: FunctionName
    Value: federation-delivery
Statistic: Sum
Period: 300
EvaluationPeriods: 2
ComparisonOperator: GreaterThanThreshold
Threshold: 10
TreatMissingData: notBreaching
AlarmActions:
  - !Ref FederationAlertsTopic
```

**Thresholds**:
- **Warning**: > 5 errors in 5 minutes
- **Critical**: > 10 errors in 5 minutes
- **Emergency**: > 25 errors in 5 minutes

### 2. Federation Retry Queue Depth

**Alarm**: Excessive backlog in federation retry processing

```yaml
AlarmName: Federation-Retry-Queue-Depth
MetricName: ApproximateNumberOfMessages
Namespace: AWS/SQS
Dimensions:
  - Name: QueueName
    Value: federation-retry-queue
Statistic: Average
Period: 300
EvaluationPeriods: 3
ComparisonOperator: GreaterThanThreshold
Threshold: 100
```

**Alert Levels**:
- **Warning**: > 50 messages for 10 minutes
- **Critical**: > 100 messages for 15 minutes
- **Emergency**: > 500 messages for 5 minutes

### 3. Federation Instance Health

**Custom Metric**: Track instance response rates and ban unhealthy instances

```javascript
// EMF Metrics for federation health
{
  "_aws": {
    "Timestamp": 1609459200000,
    "CloudWatchMetrics": [{
      "Namespace": "Lesser/Federation",
      "Dimensions": [["InstanceDomain"]],
      "Metrics": [
        {
          "Name": "DeliverySuccessRate",
          "Unit": "Percent"
        },
        {
          "Name": "ResponseTimeMs",
          "Unit": "Milliseconds"
        },
        {
          "Name": "ConnectionErrors",
          "Unit": "Count"
        }
      ]
    }]
  },
  "InstanceDomain": "mastodon.social",
  "DeliverySuccessRate": 98.5,
  "ResponseTimeMs": 1250,
  "ConnectionErrors": 0
}
```

**Alarm Configuration**:
```yaml
FederationInstanceHealthAlarm:
  Type: AWS::CloudWatch::Alarm
  Properties:
    AlarmName: Federation-Instance-Unhealthy
    MetricName: DeliverySuccessRate
    Namespace: Lesser/Federation
    Statistic: Average
    Period: 900
    EvaluationPeriods: 2
    ComparisonOperator: LessThanThreshold
    Threshold: 85
    TreatMissingData: notBreaching
```

## Media Processing Error Alarms

### 1. Media Transcoding Failures

**Alarm**: High failure rate in video/image processing

```yaml
MediaProcessingFailures:
  Type: AWS::CloudWatch::Alarm
  Properties:
    AlarmName: Media-Processing-Failures
    MetricName: Errors
    Namespace: AWS/Lambda
    Dimensions:
      - Name: FunctionName
        Value: media-processor
    Statistic: Sum
    Period: 300
    EvaluationPeriods: 2
    ComparisonOperator: GreaterThanThreshold
    Threshold: 5
    AlarmActions:
      - !Ref MediaAlertsTopic
```

**Recovery Actions**:
- Automatic retry with exponential backoff
- Fallback to lower quality processing
- Admin notification for manual intervention

### 2. Media Storage Costs

**Alarm**: Unexpected spikes in S3 storage or transfer costs

```yaml
MediaStorageCostAlarm:
  Type: AWS::CloudWatch::Alarm
  Properties:
    AlarmName: Media-Storage-Cost-Spike
    MetricName: EstimatedCharges
    Namespace: AWS/Billing
    Dimensions:
      - Name: ServiceName
        Value: AmazonS3
    Statistic: Maximum
    Period: 3600
    EvaluationPeriods: 2
    ComparisonOperator: GreaterThanThreshold
    Threshold: 100  # $100/hour
```

### 3. Streaming Quality Degradation

**Custom Metric**: Monitor streaming session quality and rebuffer events

```javascript
// Streaming quality metrics
{
  "_aws": {
    "Timestamp": 1609459200000,
    "CloudWatchMetrics": [{
      "Namespace": "Lesser/Streaming",
      "Dimensions": [["QualityLevel"], ["UserRegion"]],
      "Metrics": [
        {
          "Name": "RebufferEvents",
          "Unit": "Count"
        },
        {
          "Name": "QualitySwitches", 
          "Unit": "Count"
        },
        {
          "Name": "SegmentLoadTime",
          "Unit": "Milliseconds"
        }
      ]
    }]
  },
  "QualityLevel": "1080p",
  "UserRegion": "us-east-1",
  "RebufferEvents": 0,
  "QualitySwitches": 2,
  "SegmentLoadTime": 450
}
```

## CloudWatch Dashboard Configurations

### 1. Executive Dashboard

**Purpose**: High-level system health for administrators

```json
{
  "widgets": [
    {
      "type": "metric",
      "properties": {
        "metrics": [
          ["AWS/Lambda", "Invocations", "FunctionName", "api"],
          ["AWS/Lambda", "Errors", "FunctionName", "api"],
          ["AWS/Lambda", "Duration", "FunctionName", "api"]
        ],
        "period": 300,
        "stat": "Sum",
        "region": "us-east-1",
        "title": "API Gateway Activity"
      }
    },
    {
      "type": "metric", 
      "properties": {
        "metrics": [
          ["Lesser/Federation", "DeliverySuccessRate"],
          ["Lesser/Federation", "ActiveInstances"],
          ["Lesser/Federation", "MessageBacklog"]
        ],
        "period": 900,
        "stat": "Average", 
        "region": "us-east-1",
        "title": "Federation Health"
      }
    },
    {
      "type": "metric",
      "properties": {
        "metrics": [
          ["AWS/DynamoDB", "ConsumedReadCapacityUnits", "TableName", "lesser-main"],
          ["AWS/DynamoDB", "ConsumedWriteCapacityUnits", "TableName", "lesser-main"],
          ["AWS/DynamoDB", "ThrottledRequests", "TableName", "lesser-main"]
        ],
        "period": 300,
        "stat": "Sum",
        "region": "us-east-1", 
        "title": "Database Performance"
      }
    }
  ]
}
```

### 2. Operations Dashboard

**Purpose**: Detailed metrics for DevOps team

```json
{
  "widgets": [
    {
      "type": "log",
      "properties": {
        "query": "SOURCE '/aws/lambda/federation-delivery'\n| fields @timestamp, @message\n| filter @message like /ERROR/\n| sort @timestamp desc\n| limit 20",
        "region": "us-east-1",
        "title": "Recent Federation Errors",
        "view": "table"
      }
    },
    {
      "type": "metric",
      "properties": {
        "metrics": [
          ["AWS/Lambda", "ConcurrentExecutions"],
          ["AWS/Lambda", "ProvisionedConcurrencySpilloverInvocations"]
        ],
        "period": 300,
        "stat": "Maximum",
        "region": "us-east-1",
        "title": "Lambda Concurrency"
      }
    },
    {
      "type": "metric",
      "properties": {
        "metrics": [
          ["Lesser/Costs", "DynamoDBCostPerHour"],
          ["Lesser/Costs", "LambdaCostPerHour"], 
          ["Lesser/Costs", "S3CostPerHour"]
        ],
        "period": 3600,
        "stat": "Average",
        "region": "us-east-1",
        "title": "Hourly Cost Breakdown"
      }
    }
  ]
}
```

### 3. User Experience Dashboard

**Purpose**: Monitor metrics that impact users directly

```json
{
  "widgets": [
    {
      "type": "metric",
      "properties": {
        "metrics": [
          ["Lesser/Timeline", "LoadTimeMs", "TimelineType", "home"],
          ["Lesser/Timeline", "LoadTimeMs", "TimelineType", "public"],
          ["Lesser/Timeline", "LoadTimeMs", "TimelineType", "federated"]
        ],
        "period": 300,
        "stat": "Average",
        "region": "us-east-1",
        "title": "Timeline Load Times"
      }
    },
    {
      "type": "metric",
      "properties": {
        "metrics": [
          ["Lesser/Streaming", "RebufferEvents"],
          ["Lesser/Streaming", "QualitySwitches"],
          ["Lesser/Streaming", "SegmentLoadTime"]
        ],
        "period": 300,
        "stat": "Sum",
        "region": "us-east-1",
        "title": "Media Streaming Quality"
      }
    }
  ]
}
```

## EMF Metrics Setup

### 1. Enhanced Federation Metrics

**Implementation in Lambda functions**:

```javascript
// In federation-delivery Lambda
const { MetricUnits, logMetrics } = require('aws-embedded-metrics');

async function deliverToInstance(message, instanceDomain) {
  const startTime = Date.now();
  let success = false;
  
  try {
    await deliveryClient.deliver(message, instanceDomain);
    success = true;
  } catch (error) {
    logError('Federation delivery failed', { instanceDomain, error });
  } finally {
    const duration = Date.now() - startTime;
    
    // Log EMF metrics
    logMetrics('Lesser/Federation', 'DeliveryAttempt', MetricUnits.Count, 1);
    logMetrics('Lesser/Federation', 'DeliverySuccessRate', MetricUnits.Percent, success ? 100 : 0);
    logMetrics('Lesser/Federation', 'DeliveryLatency', MetricUnits.Milliseconds, duration);
    
    // Add dimensions
    setProperty('InstanceDomain', instanceDomain);
    setProperty('MessageType', message.type);
  }
}
```

### 2. Cost Tracking Metrics

**DynamoDB cost tracking**:

```go
// In DynamORM cost tracker
func (t *CostTracker) TrackDynamoDBOperation(operation string, consumedCapacity float64) {
    costPerRCU := 0.000125 // $0.000125 per RCU
    costPerWCU := 0.000625 // $0.000625 per WCU
    
    var cost float64
    switch operation {
    case "Query", "GetItem", "BatchGetItem":
        cost = consumedCapacity * costPerRCU
    case "PutItem", "UpdateItem", "DeleteItem", "BatchWriteItem":
        cost = consumedCapacity * costPerWCU
    }
    
    // Emit EMF metric
    emfLogger := &EMFLogger{}
    emfLogger.SetNamespace("Lesser/Costs")
    emfLogger.PutMetric("DynamoDBCost", cost, "USD")
    emfLogger.PutProperty("Operation", operation)
    emfLogger.PutProperty("Table", "lesser-main")
    emfLogger.Log()
}
```

### 3. Media Processing Metrics

**Transcoding and streaming metrics**:

```go
// In media-processor Lambda
func processMediaFile(mediaID string, inputFormat string) error {
    startTime := time.Now()
    
    defer func() {
        duration := time.Since(startTime).Milliseconds()
        
        // Log processing metrics
        emfLogger := &EMFLogger{}
        emfLogger.SetNamespace("Lesser/Media")
        emfLogger.PutMetric("ProcessingDuration", float64(duration), "Milliseconds")
        emfLogger.PutMetric("ProcessingAttempt", 1, "Count")
        emfLogger.PutProperty("InputFormat", inputFormat)
        emfLogger.PutProperty("MediaID", mediaID)
        emfLogger.Log()
    }()
    
    // Process media...
    return nil
}
```

## Automated Response Actions

### 1. Federation Circuit Breaker

**When**: Federation delivery errors exceed threshold
**Action**: Temporarily disable delivery to problematic instances

```yaml
FederationCircuitBreakerAction:
  Type: AWS::Lambda::Function
  Properties:
    FunctionName: federation-circuit-breaker
    Handler: index.handler
    Runtime: nodejs18.x
    Code:
      ZipFile: |
        exports.handler = async (event) => {
          const instanceDomain = event.instanceDomain;
          const errorRate = event.errorRate;
          
          if (errorRate > 50) {
            // Disable delivery for 1 hour
            await disableInstance(instanceDomain, 3600);
            await notifyAdmins(`Federation disabled for ${instanceDomain} due to high error rate`);
          }
        };
```

### 2. Auto-scaling Response

**When**: High concurrent Lambda executions
**Action**: Increase provisioned concurrency

```python
import boto3

def lambda_handler(event, context):
    lambda_client = boto3.client('lambda')
    
    current_concurrency = event['current_concurrency']
    threshold = 800  # Scale up when approaching 1000 limit
    
    if current_concurrency > threshold:
        # Increase provisioned concurrency
        lambda_client.put_provisioned_concurrency_config(
            FunctionName='api',
            ProvisionedConcurrencyConfig={'AllocatedConcurrency': 100}
        )
        
    return {'statusCode': 200}
```

### 3. Cost Control Response

**When**: Daily costs exceed budget
**Action**: Enable data saver modes and reduce quality

```go
func handleCostAlarm(event CostAlarmEvent) error {
    if event.DailyCost > event.Budget * 1.2 {
        // Enable aggressive cost saving
        return enableEmergencyCostSaving()
    } else if event.DailyCost > event.Budget {
        // Enable moderate cost saving
        return enableCostSaving()
    }
    return nil
}

func enableCostSaving() error {
    // Reduce media processing quality
    // Increase cache TTLs
    // Reduce federation retry attempts
    return nil
}
```

## Alert Routing and Escalation

### 1. SNS Topic Configuration

```yaml
AlertingTopics:
  FederationAlerts:
    Type: AWS::SNS::Topic
    Properties:
      TopicName: federation-alerts
      DisplayName: Federation Delivery Alerts
      Subscription:
        - Protocol: email
          Endpoint: devops@lesser.social
        - Protocol: lambda  
          Endpoint: !GetAtt SlackNotifier.Arn

  MediaAlerts:
    Type: AWS::SNS::Topic
    Properties:
      TopicName: media-processing-alerts
      DisplayName: Media Processing Alerts
      
  CostAlerts:
    Type: AWS::SNS::Topic
    Properties:
      TopicName: cost-alerts
      DisplayName: Cost Management Alerts
```

### 2. Escalation Policies

**Severity Levels**:

- **P0 (Critical)**: Service completely down
  - **Response Time**: 15 minutes
  - **Notification**: Phone, SMS, email, Slack
  - **Escalation**: Every 30 minutes until acknowledged

- **P1 (High)**: Major feature impaired  
  - **Response Time**: 1 hour
  - **Notification**: Email, Slack
  - **Escalation**: Every 2 hours during business hours

- **P2 (Medium)**: Minor impact
  - **Response Time**: 4 hours
  - **Notification**: Slack, email
  - **Escalation**: Daily summary

- **P3 (Low)**: Informational
  - **Response Time**: 24 hours
  - **Notification**: Email
  - **Escalation**: Weekly summary

## Monitoring Runbooks

### Federation Delivery Issues

1. **Check federation-delivery Lambda logs**
2. **Verify instance health in federation dashboard**
3. **Check SQS retry queue depth**
4. **Review recent DNS/network changes**
5. **Test manual delivery to affected instances**

### Media Processing Failures

1. **Check media-processor Lambda metrics**
2. **Verify S3 bucket permissions and quotas**
3. **Review recent media uploads for format issues**
4. **Check transcoding pipeline health**
5. **Validate CDN configuration**

### Cost Overruns

1. **Identify cost spike source in billing dashboard**
2. **Check for unusual traffic patterns**
3. **Review DynamoDB hot partitions**
4. **Verify Lambda concurrency settings**
5. **Check for resource leaks or infinite loops**

## Custom Metrics Implementation

### 1. Timeline Performance

```go
// Track timeline load performance
func trackTimelineLoad(timelineType string, loadTime time.Duration, postCount int) {
    emfLogger := &EMFLogger{}
    emfLogger.SetNamespace("Lesser/Timeline")
    emfLogger.PutMetric("LoadTimeMs", float64(loadTime.Milliseconds()), "Milliseconds")
    emfLogger.PutMetric("PostCount", float64(postCount), "Count")
    emfLogger.PutProperty("TimelineType", timelineType)
    emfLogger.Log()
}
```

### 2. Federation Lag

```go
// Track time from creation to federation delivery
func trackFederationLag(messageType string, lagTime time.Duration) {
    emfLogger := &EMFLogger{}
    emfLogger.SetNamespace("Lesser/Federation") 
    emfLogger.PutMetric("DeliveryLag", float64(lagTime.Seconds()), "Seconds")
    emfLogger.PutProperty("MessageType", messageType)
    emfLogger.Log()
}
```

### 3. User Engagement

```go
// Track user interaction metrics
func trackUserEngagement(action string, userID string) {
    emfLogger := &EMFLogger{}
    emfLogger.SetNamespace("Lesser/Engagement")
    emfLogger.PutMetric("UserAction", 1, "Count")
    emfLogger.PutProperty("Action", action)
    emfLogger.PutProperty("UserType", getUserType(userID))
    emfLogger.Log()
}
```

## Best Practices

### 1. Metric Design

- **Use consistent naming**: `Lesser/ServiceName` namespace pattern
- **Include relevant dimensions**: Instance, user type, region
- **Balance granularity**: Detailed enough for debugging, not overwhelming
- **Set appropriate periods**: 5min for real-time, 1hr for trends

### 2. Alarm Configuration

- **Avoid alarm fatigue**: Set thresholds that indicate real problems
- **Use composite alarms**: Combine multiple conditions for complex scenarios
- **Test alarm actions**: Ensure notifications and automations work
- **Document runbooks**: Include troubleshooting steps for each alarm

### 3. Dashboard Organization

- **Layer information**: Executive → Operations → Developer detail levels
- **Use meaningful titles**: Clear, descriptive widget names
- **Group related metrics**: Federation, media, costs together  
- **Include context**: Show baselines and targets where relevant

### 4. Cost Optimization

- **Monitor metric costs**: EMF can be expensive with high cardinality
- **Use sampling**: Not every event needs to be tracked
- **Aggregate where possible**: Sum hourly rather than per-minute
- **Clean up unused dashboards**: Regular housekeeping to reduce costs

## Implementation Checklist

- [ ] Deploy federation delivery alarms
- [ ] Set up media processing monitoring  
- [ ] Create cost tracking dashboards
- [ ] Configure SNS alert routing
- [ ] Test alarm response procedures
- [ ] Document escalation policies
- [ ] Train team on dashboard usage
- [ ] Set up automated responses
- [ ] Review and tune thresholds monthly
- [ ] Archive historical data appropriately

This observability setup provides comprehensive monitoring for Lesser while maintaining cost efficiency and avoiding alert fatigue. The EMF-based metrics provide rich insights into system behavior while the automated responses help maintain service quality.
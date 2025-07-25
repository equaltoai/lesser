# Phase 5: Production Readiness - Detailed Implementation Checklist

## 5.1 Monitoring & Observability

### 5.1.1 Implement Lift's Built-in Metrics
**File:** `pkg/lift/metrics/instrumentation.go`

- [ ] Create metrics configuration
  ```go
  package metrics
  
  import (
      "github.com/pay-theory/lift/pkg/lift"
      "github.com/pay-theory/lift/pkg/metrics"
      "github.com/aws/aws-sdk-go-v2/service/cloudwatch"
  )
  
  type MetricsConfig struct {
      Namespace   string
      Environment string
      Service     string
      Enabled     bool
  }
  
  func NewMetricsMiddleware(cfg MetricsConfig) lift.Middleware {
      if !cfg.Enabled {
          return func(next lift.HandlerFunc) lift.HandlerFunc {
              return next
          }
      }
      
      collector := metrics.NewCollector(metrics.Config{
          Namespace:   cfg.Namespace,
          Dimensions: map[string]string{
              "Environment": cfg.Environment,
              "Service":     cfg.Service,
          },
      })
      
      return func(next lift.HandlerFunc) lift.HandlerFunc {
          return func(ctx *lift.Context) error {
              start := time.Now()
              
              // Extract metadata
              handler := ctx.Get("handler_name").(string)
              method := ctx.Request().Method
              path := ctx.Request().URL.Path
              
              // Execute handler
              err := next(ctx)
              
              // Record metrics
              duration := time.Since(start)
              status := "success"
              statusCode := 200
              
              if err != nil {
                  status = "error"
                  if liftErr, ok := err.(*lift.Error); ok {
                      statusCode = liftErr.StatusCode
                  } else {
                      statusCode = 500
                  }
              }
              
              // Record request metrics
              collector.RecordCount("requests", 1,
                  metrics.Tag("handler", handler),
                  metrics.Tag("method", method),
                  metrics.Tag("status", status),
                  metrics.Tag("status_code", fmt.Sprintf("%d", statusCode)),
              )
              
              collector.RecordDuration("request_duration", duration,
                  metrics.Tag("handler", handler),
                  metrics.Tag("method", method),
                  metrics.Tag("status", status),
              )
              
              // Record error metrics
              if err != nil {
                  collector.RecordCount("errors", 1,
                      metrics.Tag("handler", handler),
                      metrics.Tag("error_type", classifyError(err)),
                  )
              }
              
              return err
          }
      }
  }
  ```

- [ ] Implement Lambda-specific metrics
  ```go
  type LambdaMetrics struct {
      collector *metrics.Collector
      function  string
  }
  
  func NewLambdaMetrics(functionName string) *LambdaMetrics {
      return &LambdaMetrics{
          collector: metrics.NewCollector(metrics.Config{
              Namespace: "Lesser/Lambda",
              Dimensions: map[string]string{
                  "FunctionName": functionName,
              },
          }),
          function: functionName,
      }
  }
  
  func (m *LambdaMetrics) RecordColdStart(duration time.Duration) {
      m.collector.RecordDuration("cold_start", duration)
      m.collector.RecordCount("cold_starts", 1)
  }
  
  func (m *LambdaMetrics) RecordInvocation(duration time.Duration, memoryUsed int) {
      m.collector.RecordDuration("invocation_duration", duration)
      m.collector.RecordValue("memory_used_mb", float64(memoryUsed))
      
      // Calculate cost
      cost := calculateLambdaCost(duration, memoryUsed)
      m.collector.RecordValue("estimated_cost", cost)
  }
  
  func (m *LambdaMetrics) RecordStreamBatch(recordCount int, duration time.Duration, errors int) {
      m.collector.RecordCount("stream_records_processed", recordCount)
      m.collector.RecordDuration("stream_batch_duration", duration)
      
      if errors > 0 {
          m.collector.RecordCount("stream_errors", errors)
      }
      
      // Calculate throughput
      throughput := float64(recordCount) / duration.Seconds()
      m.collector.RecordValue("stream_throughput", throughput)
  }
  ```

- [ ] Create business metrics
  ```go
  type BusinessMetrics struct {
      collector *metrics.Collector
  }
  
  func (m *BusinessMetrics) RecordUserAction(action string, userID string) {
      m.collector.RecordCount("user_actions", 1,
          metrics.Tag("action", action),
          metrics.Tag("user_type", getUserType(userID)),
      )
  }
  
  func (m *BusinessMetrics) RecordFederationActivity(activityType string, instance string, success bool) {
      status := "success"
      if !success {
          status = "failure"
      }
      
      m.collector.RecordCount("federation_activities", 1,
          metrics.Tag("type", activityType),
          metrics.Tag("instance", instance),
          metrics.Tag("status", status),
      )
  }
  
  func (m *BusinessMetrics) RecordContentMetrics(contentType string, action string) {
      m.collector.RecordCount("content_actions", 1,
          metrics.Tag("content_type", contentType),
          metrics.Tag("action", action),
      )
  }
  ```

**Testing Requirements:**
- [ ] Test metric collection
- [ ] Test CloudWatch integration
- [ ] Test metric aggregation
- [ ] Test performance impact

**Acceptance Criteria:**
- All handlers instrumented
- Metrics visible in CloudWatch
- Low performance overhead
- Actionable insights available

### 5.1.2 Custom CloudWatch Metrics
**File:** `pkg/monitoring/cloudwatch/custom_metrics.go`

- [ ] Create custom metric publisher
  ```go
  package cloudwatch
  
  type MetricPublisher struct {
      client    *cloudwatch.Client
      namespace string
      buffer    *MetricBuffer
      interval  time.Duration
  }
  
  func NewMetricPublisher(cfg PublisherConfig) *MetricPublisher {
      p := &MetricPublisher{
          client:    cloudwatch.NewFromConfig(cfg.AWSConfig),
          namespace: cfg.Namespace,
          buffer:    NewMetricBuffer(cfg.BufferSize),
          interval:  cfg.PublishInterval,
      }
      
      go p.publishLoop()
      return p
  }
  
  func (p *MetricPublisher) publishLoop() {
      ticker := time.NewTicker(p.interval)
      defer ticker.Stop()
      
      for range ticker.C {
          metrics := p.buffer.Flush()
          if len(metrics) > 0 {
              p.publishBatch(metrics)
          }
      }
  }
  
  func (p *MetricPublisher) publishBatch(metrics []Metric) {
      // CloudWatch allows max 20 metrics per request
      for i := 0; i < len(metrics); i += 20 {
          end := i + 20
          if end > len(metrics) {
              end = len(metrics)
          }
          
          batch := metrics[i:end]
          metricData := make([]types.MetricDatum, len(batch))
          
          for j, metric := range batch {
              metricData[j] = types.MetricDatum{
                  MetricName: aws.String(metric.Name),
                  Value:      aws.Float64(metric.Value),
                  Unit:       metric.Unit,
                  Timestamp:  aws.Time(metric.Timestamp),
                  Dimensions: convertDimensions(metric.Dimensions),
              }
          }
          
          _, err := p.client.PutMetricData(context.Background(), &cloudwatch.PutMetricDataInput{
              Namespace:  aws.String(p.namespace),
              MetricData: metricData,
          })
          
          if err != nil {
              log.Printf("Failed to publish metrics: %v", err)
          }
      }
  }
  ```

- [ ] Create metric aggregators
  ```go
  type MetricAggregator struct {
      window    time.Duration
      metrics   map[string]*AggregatedMetric
      mu        sync.RWMutex
  }
  
  type AggregatedMetric struct {
      Count    int64
      Sum      float64
      Min      float64
      Max      float64
      Values   []float64 // For percentile calculation
  }
  
  func (a *MetricAggregator) Record(name string, value float64, tags map[string]string) {
      a.mu.Lock()
      defer a.mu.Unlock()
      
      key := a.generateKey(name, tags)
      
      if metric, exists := a.metrics[key]; exists {
          metric.Count++
          metric.Sum += value
          metric.Min = math.Min(metric.Min, value)
          metric.Max = math.Max(metric.Max, value)
          metric.Values = append(metric.Values, value)
      } else {
          a.metrics[key] = &AggregatedMetric{
              Count:  1,
              Sum:    value,
              Min:    value,
              Max:    value,
              Values: []float64{value},
          }
      }
  }
  
  func (a *MetricAggregator) Flush() []ProcessedMetric {
      a.mu.Lock()
      defer a.mu.Unlock()
      
      var results []ProcessedMetric
      
      for key, metric := range a.metrics {
          name, tags := a.parseKey(key)
          
          // Calculate percentiles
          p50 := percentile(metric.Values, 0.50)
          p95 := percentile(metric.Values, 0.95)
          p99 := percentile(metric.Values, 0.99)
          
          results = append(results, 
              ProcessedMetric{Name: name + ".count", Value: float64(metric.Count), Tags: tags},
              ProcessedMetric{Name: name + ".sum", Value: metric.Sum, Tags: tags},
              ProcessedMetric{Name: name + ".avg", Value: metric.Sum / float64(metric.Count), Tags: tags},
              ProcessedMetric{Name: name + ".min", Value: metric.Min, Tags: tags},
              ProcessedMetric{Name: name + ".max", Value: metric.Max, Tags: tags},
              ProcessedMetric{Name: name + ".p50", Value: p50, Tags: tags},
              ProcessedMetric{Name: name + ".p95", Value: p95, Tags: tags},
              ProcessedMetric{Name: name + ".p99", Value: p99, Tags: tags},
          )
      }
      
      // Clear metrics
      a.metrics = make(map[string]*AggregatedMetric)
      
      return results
  }
  ```

- [ ] Create cost tracking metrics
  ```go
  type CostMetrics struct {
      publisher *MetricPublisher
  }
  
  func (c *CostMetrics) RecordDynamoDBCost(operation string, rcu, wcu float64) {
      // DynamoDB pricing: $0.25 per million RCU, $1.25 per million WCU
      readCost := (rcu / 1_000_000) * 0.25
      writeCost := (wcu / 1_000_000) * 1.25
      totalCost := readCost + writeCost
      
      c.publisher.Publish([]Metric{
          {
              Name:  "dynamodb_cost",
              Value: totalCost,
              Unit:  types.StandardUnitCount,
              Dimensions: map[string]string{
                  "Operation": operation,
                  "CostType":  "total",
              },
          },
          {
              Name:  "dynamodb_rcu",
              Value: rcu,
              Unit:  types.StandardUnitCount,
              Dimensions: map[string]string{
                  "Operation": operation,
              },
          },
          {
              Name:  "dynamodb_wcu",
              Value: wcu,
              Unit:  types.StandardUnitCount,
              Dimensions: map[string]string{
                  "Operation": operation,
              },
          },
      })
  }
  
  func (c *CostMetrics) RecordLambdaCost(functionName string, durationMs int64, memoryMB int) {
      // Lambda pricing: $0.0000166667 per GB-second
      gbSeconds := float64(memoryMB) / 1024 * float64(durationMs) / 1000
      cost := gbSeconds * 0.0000166667
      
      c.publisher.Publish([]Metric{
          {
              Name:  "lambda_cost",
              Value: cost,
              Unit:  types.StandardUnitCount,
              Dimensions: map[string]string{
                  "FunctionName": functionName,
              },
          },
          {
              Name:  "lambda_gb_seconds",
              Value: gbSeconds,
              Unit:  types.StandardUnitCount,
              Dimensions: map[string]string{
                  "FunctionName": functionName,
              },
          },
      })
  }
  ```

**Testing Requirements:**
- [ ] Test metric batching
- [ ] Test aggregation accuracy
- [ ] Test CloudWatch limits
- [ ] Test cost calculations

**Acceptance Criteria:**
- Custom metrics published
- Aggregations accurate
- Costs tracked precisely
- CloudWatch limits respected

### 5.1.3 Operational Dashboards
**File:** `infra/monitoring/dashboards.tf`

- [ ] Create CloudWatch dashboards
  ```hcl
  resource "aws_cloudwatch_dashboard" "main" {
    dashboard_name = "${var.project_name}-main"
    
    dashboard_body = jsonencode({
      widgets = [
        {
          type   = "metric"
          width  = 12
          height = 6
          properties = {
            metrics = [
              ["Lesser/API", "requests", { stat = "Sum" }],
              [".", "errors", { stat = "Sum", yAxis = "right" }]
            ]
            period = 300
            stat   = "Average"
            region = var.aws_region
            title  = "API Requests and Errors"
          }
        },
        {
          type   = "metric"
          width  = 12
          height = 6
          properties = {
            metrics = [
              ["Lesser/API", "request_duration", { stat = "p50" }],
              ["...", { stat = "p95" }],
              ["...", { stat = "p99" }]
            ]
            period = 300
            region = var.aws_region
            title  = "API Latency Percentiles"
          }
        },
        {
          type   = "metric"
          width  = 12
          height = 6
          properties = {
            metrics = [
              ["Lesser/Lambda", "cold_starts", "FunctionName", "api", { stat = "Sum" }],
              ["...", "activity-processor", { stat = "Sum" }],
              ["...", "inbox", { stat = "Sum" }]
            ]
            period = 300
            region = var.aws_region
            title  = "Lambda Cold Starts"
          }
        },
        {
          type   = "metric"
          width  = 12
          height = 6
          properties = {
            metrics = [
              ["Lesser/Cost", "dynamodb_cost", { stat = "Sum" }],
              [".", "lambda_cost", { stat = "Sum" }],
              [".", "total_cost", { stat = "Sum" }]
            ]
            period = 3600
            region = var.aws_region
            title  = "Hourly Cost Breakdown"
          }
        }
      ]
    })
  }
  ```

- [ ] Create business metrics dashboard
  ```hcl
  resource "aws_cloudwatch_dashboard" "business" {
    dashboard_name = "${var.project_name}-business"
    
    dashboard_body = jsonencode({
      widgets = [
        {
          type   = "metric"
          width  = 8
          height = 6
          properties = {
            metrics = [
              ["Lesser/Business", "user_actions", "action", "create_status"],
              ["...", "follow"],
              ["...", "favorite"],
              ["...", "reblog"]
            ]
            period = 3600
            stat   = "Sum"
            region = var.aws_region
            title  = "User Activity (Hourly)"
          }
        },
        {
          type   = "metric"
          width  = 8
          height = 6
          properties = {
            metrics = [
              ["Lesser/Business", "active_users", { stat = "Average" }],
              [".", "new_users", { stat = "Sum" }]
            ]
            period = 86400
            region = var.aws_region
            title  = "User Growth (Daily)"
          }
        },
        {
          type   = "metric"
          width  = 8
          height = 6
          properties = {
            metrics = [
              ["Lesser/Federation", "activities_received", { stat = "Sum" }],
              [".", "activities_sent", { stat = "Sum" }],
              [".", "federation_errors", { stat = "Sum", yAxis = "right" }]
            ]
            period = 3600
            region = var.aws_region
            title  = "Federation Activity"
          }
        }
      ]
    })
  }
  ```

- [ ] Create alert dashboard
  ```hcl
  resource "aws_cloudwatch_dashboard" "alerts" {
    dashboard_name = "${var.project_name}-alerts"
    
    dashboard_body = jsonencode({
      widgets = [
        {
          type   = "metric"
          width  = 24
          height = 3
          properties = {
            metrics = [
              ["AWS/Lambda", "Errors", "FunctionName", "api"],
              ["...", "activity-processor"],
              ["...", "inbox"]
            ]
            period = 300
            stat   = "Sum"
            region = var.aws_region
            title  = "Lambda Errors"
            annotations = {
              alarms = [
                aws_cloudwatch_metric_alarm.lambda_errors.arn
              ]
            }
          }
        },
        {
          type   = "metric"
          width  = 24
          height = 3
          properties = {
            metrics = [
              ["Lesser/API", "request_duration", { stat = "p99" }]
            ]
            period = 300
            region = var.aws_region
            title  = "API Latency P99"
            annotations = {
              alarms = [
                aws_cloudwatch_metric_alarm.api_latency.arn
              ]
              horizontal = [
                {
                  label = "SLA Threshold"
                  value = 1000
                }
              ]
            }
          }
        }
      ]
    })
  }
  ```

**Testing Requirements:**
- [ ] Test dashboard rendering
- [ ] Test metric accuracy
- [ ] Test refresh rates
- [ ] Test mobile view

**Acceptance Criteria:**
- Dashboards load quickly
- Metrics are accurate
- Easy to understand
- Mobile responsive

### 5.1.4 Alerting Rules
**File:** `infra/monitoring/alarms.tf`

- [ ] Create critical alerts
  ```hcl
  resource "aws_cloudwatch_metric_alarm" "api_errors" {
    alarm_name          = "${var.project_name}-api-errors"
    comparison_operator = "GreaterThanThreshold"
    evaluation_periods  = "2"
    metric_name         = "errors"
    namespace           = "Lesser/API"
    period              = "300"
    statistic           = "Sum"
    threshold           = "50"
    alarm_description   = "API error rate too high"
    alarm_actions       = [aws_sns_topic.alerts.arn]
    
    dimensions = {
      Service = "api"
    }
  }
  
  resource "aws_cloudwatch_metric_alarm" "api_latency" {
    alarm_name          = "${var.project_name}-api-latency"
    comparison_operator = "GreaterThanThreshold"
    evaluation_periods  = "3"
    metric_name         = "request_duration"
    namespace           = "Lesser/API"
    period              = "300"
    statistic           = "p99"
    threshold           = "1000"
    alarm_description   = "API p99 latency exceeds 1 second"
    alarm_actions       = [aws_sns_topic.alerts.arn]
  }
  
  resource "aws_cloudwatch_metric_alarm" "cost_threshold" {
    alarm_name          = "${var.project_name}-cost-threshold"
    comparison_operator = "GreaterThanThreshold"
    evaluation_periods  = "1"
    metric_name         = "total_cost"
    namespace           = "Lesser/Cost"
    period              = "3600"
    statistic           = "Sum"
    threshold           = "1.0" # $1 per hour
    alarm_description   = "Hourly cost exceeds $1"
    alarm_actions       = [aws_sns_topic.alerts.arn]
  }
  ```

- [ ] Create composite alarms
  ```hcl
  resource "aws_cloudwatch_composite_alarm" "service_degraded" {
    alarm_name          = "${var.project_name}-service-degraded"
    alarm_description   = "Service is experiencing degraded performance"
    alarm_actions       = [aws_sns_topic.critical_alerts.arn]
    
    alarm_rule = join(" OR ", [
      "ALARM(${aws_cloudwatch_metric_alarm.api_errors.alarm_name})",
      "ALARM(${aws_cloudwatch_metric_alarm.api_latency.alarm_name})",
      "ALARM(${aws_cloudwatch_metric_alarm.lambda_throttles.alarm_name})"
    ])
  }
  
  resource "aws_cloudwatch_composite_alarm" "data_pipeline_failure" {
    alarm_name          = "${var.project_name}-data-pipeline-failure"
    alarm_description   = "Data processing pipeline has failed"
    alarm_actions       = [aws_sns_topic.critical_alerts.arn]
    
    alarm_rule = join(" AND ", [
      "ALARM(${aws_cloudwatch_metric_alarm.stream_lag.alarm_name})",
      "ALARM(${aws_cloudwatch_metric_alarm.processor_errors.alarm_name})"
    ])
  }
  ```

- [ ] Create anomaly detectors
  ```hcl
  resource "aws_cloudwatch_anomaly_detector" "request_rate" {
    metric_name = "requests"
    namespace   = "Lesser/API"
    stat        = "Average"
    
    dimensions = {
      Service = "api"
    }
  }
  
  resource "aws_cloudwatch_metric_alarm" "anomaly_requests" {
    alarm_name          = "${var.project_name}-anomaly-requests"
    comparison_operator = "LessThanLowerOrGreaterThanUpperThreshold"
    evaluation_periods  = "2"
    threshold_metric_id = "ad1"
    alarm_description   = "Request rate anomaly detected"
    alarm_actions       = [aws_sns_topic.alerts.arn]
    
    metric_query {
      id          = "m1"
      return_data = true
      
      metric {
        metric_name = "requests"
        namespace   = "Lesser/API"
        period      = "300"
        stat        = "Average"
        
        dimensions = {
          Service = "api"
        }
      }
    }
    
    metric_query {
      id          = "ad1"
      expression  = "ANOMALY_DETECTION_BAND(m1, 2)"
    }
  }
  ```

**Testing Requirements:**
- [ ] Test alarm triggers
- [ ] Test notification delivery
- [ ] Test composite logic
- [ ] Test anomaly detection

**Acceptance Criteria:**
- Alarms trigger correctly
- No false positives
- Notifications delivered
- Clear actionable alerts

## 5.2 Performance Optimization

### 5.2.1 Lambda Cold Start Optimization
**File:** `pkg/optimization/coldstart/optimizer.go`

- [ ] Implement cold start detection
  ```go
  package coldstart
  
  var (
      initialized     bool
      initTime        time.Time
      coldStartMetric *metrics.LambdaMetrics
  )
  
  func init() {
      initTime = time.Now()
      initialized = false
  }
  
  func DetectColdStart() bool {
      if !initialized {
          initialized = true
          return true
      }
      return false
  }
  
  func OptimizedInit() {
      if coldStart := DetectColdStart(); coldStart {
          duration := time.Since(initTime)
          
          // Report cold start metric
          if coldStartMetric != nil {
              coldStartMetric.RecordColdStart(duration)
          }
          
          // Perform warming activities
          warmConnections()
          preCacheData()
          
          log.Printf("Cold start detected: %v", duration)
      }
  }
  
  func warmConnections() {
      // Pre-establish DynamoDB connections
      dynamoClient := getDynamoClient()
      dynamoClient.ListTables(context.Background(), &dynamodb.ListTablesInput{
          Limit: aws.Int32(1),
      })
      
      // Pre-establish HTTP connections
      httpClient := getHTTPClient()
      httpClient.Head("https://example.com")
  }
  
  func preCacheData() {
      // Load frequently accessed data
      cache := getCache()
      
      // Cache instance configuration
      config := loadInstanceConfig()
      cache.Set("instance:config", config, 24*time.Hour)
      
      // Cache public keys for federation
      keys := loadPublicKeys()
      cache.Set("federation:keys", keys, 1*time.Hour)
  }
  ```

- [ ] Optimize package size
  ```makefile
  # Makefile additions for optimized builds
  
  build-optimized:
  	@echo "Building optimized Lambda functions..."
  	@for lambda in $(LAMBDAS); do \
  		echo "Building $$lambda..."; \
  		GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build \
  			-ldflags="-s -w" \
  			-tags lambda.norpc \
  			-trimpath \
  			-o bin/$$lambda ./cmd/$$lambda; \
  		# Strip debug symbols
  		strip bin/$$lambda 2>/dev/null || true; \
  		# Compress binary with UPX if available
  		which upx > /dev/null && upx --best bin/$$lambda || true; \
  		# Report size
  		ls -lh bin/$$lambda; \
  	done
  
  analyze-dependencies:
  	@echo "Analyzing Lambda dependencies..."
  	@go mod graph | grep -v indirect
  	@echo "\nLarge dependencies:"
  	@go list -m -f '{{.Path}} {{.Dir}}' all | while read path dir; do \
  		if [ -d "$$dir" ]; then \
  			size=$$(du -sh "$$dir" 2>/dev/null | cut -f1); \
  			echo "$$size $$path"; \
  		fi; \
  	done | sort -hr | head -20
  
  trim-dependencies:
  	@echo "Removing unused dependencies..."
  	@go mod tidy
  	@go mod vendor
  	@find vendor -name "*_test.go" -delete
  	@find vendor -name "testdata" -type d -exec rm -rf {} +
  ```

- [ ] Implement Lambda layers
  ```hcl
  # infra/lambda/layers.tf
  
  resource "aws_lambda_layer_version" "common_deps" {
    filename            = "layers/common-deps.zip"
    layer_name          = "${var.project_name}-common-deps"
    compatible_runtimes = ["provided.al2"]
    
    description = "Common dependencies for all Lambda functions"
  }
  
  resource "aws_lambda_layer_version" "lift_runtime" {
    filename            = "layers/lift-runtime.zip"
    layer_name          = "${var.project_name}-lift-runtime"
    compatible_runtimes = ["provided.al2"]
    
    description = "Lift framework runtime"
  }
  
  # Update Lambda functions to use layers
  resource "aws_lambda_function" "api" {
    function_name = "${var.project_name}-api"
    handler       = "bootstrap"
    runtime       = "provided.al2"
    
    layers = [
      aws_lambda_layer_version.common_deps.arn,
      aws_lambda_layer_version.lift_runtime.arn
    ]
    
    environment {
      variables = {
        LAMBDA_INIT_TYPE = "on-demand"
      }
    }
  }
  ```

**Testing Requirements:**
- [ ] Measure cold start times
- [ ] Test package optimization
- [ ] Test layer functionality
- [ ] Compare before/after

**Acceptance Criteria:**
- Cold starts < 1 second
- Package size reduced
- Layers work correctly
- No functionality lost

### 5.2.2 Connection Pooling
**File:** `pkg/optimization/pooling/connection_pool.go`

- [ ] Create DynamoDB connection pool
  ```go
  package pooling
  
  type DynamoDBPool struct {
      clients    chan *dynamodb.Client
      maxClients int
      config     aws.Config
      mu         sync.Mutex
  }
  
  func NewDynamoDBPool(cfg aws.Config, size int) *DynamoDBPool {
      pool := &DynamoDBPool{
          clients:    make(chan *dynamodb.Client, size),
          maxClients: size,
          config:     cfg,
      }
      
      // Pre-create clients
      for i := 0; i < size; i++ {
          client := dynamodb.NewFromConfig(cfg, func(o *dynamodb.Options) {
              o.HTTPClient = &http.Client{
                  Timeout: 30 * time.Second,
                  Transport: &http.Transport{
                      MaxIdleConns:        size,
                      MaxIdleConnsPerHost: size,
                      IdleConnTimeout:     90 * time.Second,
                  },
              }
          })
          pool.clients <- client
      }
      
      return pool
  }
  
  func (p *DynamoDBPool) GetClient() *dynamodb.Client {
      select {
      case client := <-p.clients:
          return client
      default:
          // Pool exhausted, create new client
          return dynamodb.NewFromConfig(p.config)
      }
  }
  
  func (p *DynamoDBPool) ReturnClient(client *dynamodb.Client) {
      select {
      case p.clients <- client:
          // Client returned to pool
      default:
          // Pool full, let client be garbage collected
      }
  }
  
  func (p *DynamoDBPool) WithClient(fn func(*dynamodb.Client) error) error {
      client := p.GetClient()
      defer p.ReturnClient(client)
      return fn(client)
  }
  ```

- [ ] Create HTTP connection pool
  ```go
  type HTTPPool struct {
      clients map[string]*http.Client
      mu      sync.RWMutex
  }
  
  func NewHTTPPool() *HTTPPool {
      return &HTTPPool{
          clients: make(map[string]*http.Client),
      }
  }
  
  func (p *HTTPPool) GetClient(host string) *http.Client {
      p.mu.RLock()
      client, exists := p.clients[host]
      p.mu.RUnlock()
      
      if exists {
          return client
      }
      
      p.mu.Lock()
      defer p.mu.Unlock()
      
      // Double-check after acquiring write lock
      if client, exists = p.clients[host]; exists {
          return client
      }
      
      // Create new client with connection pooling
      client = &http.Client{
          Timeout: 30 * time.Second,
          Transport: &http.Transport{
              MaxIdleConns:        100,
              MaxIdleConnsPerHost: 10,
              IdleConnTimeout:     90 * time.Second,
              DisableKeepAlives:   false,
              ForceAttemptHTTP2:   true,
          },
      }
      
      p.clients[host] = client
      return client
  }
  ```

- [ ] Implement connection warming
  ```go
  func WarmConnections(ctx context.Context) {
      var wg sync.WaitGroup
      
      // Warm DynamoDB connections
      wg.Add(1)
      go func() {
          defer wg.Done()
          pool := GetDynamoDBPool()
          pool.WithClient(func(client *dynamodb.Client) error {
              _, err := client.DescribeTable(ctx, &dynamodb.DescribeTableInput{
                  TableName: aws.String(os.Getenv("DYNAMODB_TABLE")),
              })
              return err
          })
      }()
      
      // Warm HTTP connections to common federation targets
      commonHosts := []string{
          "mastodon.social",
          "fosstodon.org",
          "mastodon.world",
      }
      
      for _, host := range commonHosts {
          wg.Add(1)
          go func(h string) {
              defer wg.Done()
              client := GetHTTPPool().GetClient(h)
              req, _ := http.NewRequestWithContext(ctx, "HEAD", fmt.Sprintf("https://%s", h), nil)
              client.Do(req)
          }(host)
      }
      
      wg.Wait()
  }
  ```

**Testing Requirements:**
- [ ] Test connection reuse
- [ ] Test pool exhaustion
- [ ] Test concurrent access
- [ ] Measure performance gains

**Acceptance Criteria:**
- Connections reused
- Thread-safe access
- Performance improved
- No connection leaks

### 5.2.3 Caching Strategy
**File:** `pkg/optimization/cache/multi_level_cache.go`

- [ ] Implement multi-level cache
  ```go
  package cache
  
  type MultiLevelCache struct {
      l1 *MemoryCache    // In-memory cache
      l2 *DynamoDBCache  // DynamoDB cache
      l3 *S3Cache        // S3 cache for large objects
  }
  
  func NewMultiLevelCache(config CacheConfig) *MultiLevelCache {
      return &MultiLevelCache{
          l1: NewMemoryCache(config.L1Size),
          l2: NewDynamoDBCache(config.DynamoDBTable),
          l3: NewS3Cache(config.S3Bucket),
      }
  }
  
  func (c *MultiLevelCache) Get(ctx context.Context, key string) (any, error) {
      // Check L1 (memory)
      if value, found := c.l1.Get(key); found {
          c.recordHit("l1", key)
          return value, nil
      }
      
      // Check L2 (DynamoDB)
      value, err := c.l2.Get(ctx, key)
      if err == nil && value != nil {
          c.recordHit("l2", key)
          c.l1.Set(key, value, DefaultTTL) // Promote to L1
          return value, nil
      }
      
      // Check L3 (S3) for large objects
      if c.isLargeObjectKey(key) {
          value, err = c.l3.Get(ctx, key)
          if err == nil && value != nil {
              c.recordHit("l3", key)
              // Don't promote large objects to L1/L2
              return value, nil
          }
      }
      
      c.recordMiss(key)
      return nil, ErrCacheMiss
  }
  
  func (c *MultiLevelCache) Set(ctx context.Context, key string, value any, ttl time.Duration) error {
      size := calculateSize(value)
      
      // Store in appropriate levels based on size and access pattern
      if size < 1024 { // < 1KB - store in all levels
          c.l1.Set(key, value, ttl)
          return c.l2.Set(ctx, key, value, ttl)
      } else if size < 400*1024 { // < 400KB - skip L1
          return c.l2.Set(ctx, key, value, ttl)
      } else { // Large objects - S3 only
          return c.l3.Set(ctx, key, value, ttl)
      }
  }
  ```

- [ ] Implement cache warming
  ```go
  type CacheWarmer struct {
      cache    *MultiLevelCache
      store    storage.Storage
      patterns []WarmingPattern
  }
  
  type WarmingPattern struct {
      Name     string
      Query    func() ([]CacheItem, error)
      TTL      time.Duration
      Priority int
  }
  
  func (w *CacheWarmer) WarmCache(ctx context.Context) error {
      // Sort patterns by priority
      sort.Slice(w.patterns, func(i, j int) bool {
          return w.patterns[i].Priority > w.patterns[j].Priority
      })
      
      for _, pattern := range w.patterns {
          items, err := pattern.Query()
          if err != nil {
              log.Printf("Failed to warm cache for %s: %v", pattern.Name, err)
              continue
          }
          
          for _, item := range items {
              if err := w.cache.Set(ctx, item.Key, item.Value, pattern.TTL); err != nil {
                  log.Printf("Failed to cache item %s: %v", item.Key, err)
              }
          }
      }
      
      return nil
  }
  
  // Example warming patterns
  func GetWarmingPatterns(store storage.Storage) []WarmingPattern {
      return []WarmingPattern{
          {
              Name: "instance_config",
              Query: func() ([]CacheItem, error) {
                  config, err := store.GetInstanceConfig(context.Background())
                  if err != nil {
                      return nil, err
                  }
                  return []CacheItem{
                      {Key: "instance:config", Value: config},
                  }, nil
              },
              TTL:      24 * time.Hour,
              Priority: 100,
          },
          {
              Name: "popular_users",
              Query: func() ([]CacheItem, error) {
                  users, err := store.GetPopularUsers(context.Background(), 100)
                  if err != nil {
                      return nil, err
                  }
                  
                  items := make([]CacheItem, len(users))
                  for i, user := range users {
                      items[i] = CacheItem{
                          Key:   fmt.Sprintf("user:%s", user.ID),
                          Value: user,
                      }
                  }
                  return items, nil
              },
              TTL:      1 * time.Hour,
              Priority: 80,
          },
      }
  }
  ```

- [ ] Implement cache invalidation
  ```go
  type CacheInvalidator struct {
      cache         *MultiLevelCache
      invalidationQ *sqs.Client
      queueURL      string
  }
  
  func (ci *CacheInvalidator) InvalidatePattern(ctx context.Context, pattern string) error {
      // Send invalidation message to SQS for distributed invalidation
      message := InvalidationMessage{
          Pattern:   pattern,
          Timestamp: time.Now(),
          Source:    getInstanceID(),
      }
      
      messageBody, _ := json.Marshal(message)
      
      _, err := ci.invalidationQ.SendMessage(ctx, &sqs.SendMessageInput{
          QueueUrl:    &ci.queueURL,
          MessageBody: aws.String(string(messageBody)),
      })
      
      // Also invalidate locally
      ci.cache.InvalidatePattern(pattern)
      
      return err
  }
  
  func (ci *CacheInvalidator) ProcessInvalidations(ctx context.Context) {
      for {
          result, err := ci.invalidationQ.ReceiveMessage(ctx, &sqs.ReceiveMessageInput{
              QueueUrl:            &ci.queueURL,
              MaxNumberOfMessages: 10,
              WaitTimeSeconds:     20,
          })
          
          if err != nil {
              log.Printf("Failed to receive invalidation messages: %v", err)
              continue
          }
          
          for _, message := range result.Messages {
              var inv InvalidationMessage
              if err := json.Unmarshal([]byte(*message.Body), &inv); err != nil {
                  continue
              }
              
              // Skip our own invalidations
              if inv.Source != getInstanceID() {
                  ci.cache.InvalidatePattern(inv.Pattern)
              }
              
              // Delete message
              ci.invalidationQ.DeleteMessage(ctx, &sqs.DeleteMessageInput{
                  QueueUrl:      &ci.queueURL,
                  ReceiptHandle: message.ReceiptHandle,
              })
          }
      }
  }
  ```

**Testing Requirements:**
- [ ] Test cache hit rates
- [ ] Test invalidation propagation
- [ ] Test memory limits
- [ ] Test performance gains

**Acceptance Criteria:**
- High cache hit rate
- Invalidation works
- Memory usage bounded
- Significant speedup

### 5.2.4 Database Query Optimization
**File:** `pkg/optimization/database/query_optimizer.go`

- [ ] Implement query analysis
  ```go
  package database
  
  type QueryAnalyzer struct {
      metrics  *QueryMetrics
      patterns *PatternDetector
  }
  
  type QueryMetrics struct {
      queries map[string]*QueryStats
      mu      sync.RWMutex
  }
  
  type QueryStats struct {
      Count       int64
      TotalTime   time.Duration
      AvgTime     time.Duration
      MaxTime     time.Duration
      RCU         float64
      WCU         float64
      IndexUsage  map[string]int
  }
  
  func (qa *QueryAnalyzer) AnalyzeQuery(query Query, result QueryResult) {
      stats := qa.getOrCreateStats(query.Name)
      
      stats.Count++
      stats.TotalTime += result.Duration
      stats.AvgTime = stats.TotalTime / time.Duration(stats.Count)
      if result.Duration > stats.MaxTime {
          stats.MaxTime = result.Duration
      }
      
      stats.RCU += result.ConsumedRCU
      stats.WCU += result.ConsumedWCU
      
      if result.IndexUsed != "" {
          stats.IndexUsage[result.IndexUsed]++
      }
      
      // Detect problematic patterns
      if qa.patterns.IsProblematic(query, result) {
          qa.logProblematicQuery(query, result)
      }
  }
  
  func (qa *QueryAnalyzer) GetRecommendations() []Recommendation {
      qa.metrics.mu.RLock()
      defer qa.metrics.mu.RUnlock()
      
      var recommendations []Recommendation
      
      for queryName, stats := range qa.metrics.queries {
          // Check for missing indexes
          if stats.AvgTime > 100*time.Millisecond && len(stats.IndexUsage) == 0 {
              recommendations = append(recommendations, Recommendation{
                  Query:    queryName,
                  Type:     "missing_index",
                  Message:  "Query is slow and not using any index",
                  Impact:   "high",
                  Solution: "Consider adding a GSI for this access pattern",
              })
          }
          
          // Check for expensive scans
          avgRCU := stats.RCU / float64(stats.Count)
          if avgRCU > 10 {
              recommendations = append(recommendations, Recommendation{
                  Query:    queryName,
                  Type:     "expensive_scan",
                  Message:  fmt.Sprintf("Query consumes %.2f RCU on average", avgRCU),
                  Impact:   "high",
                  Solution: "Add filtering or use a more specific index",
              })
          }
      }
      
      return recommendations
  }
  ```

- [ ] Implement query optimization strategies
  ```go
  type QueryOptimizer struct {
      analyzer *QueryAnalyzer
      cache    *MultiLevelCache
  }
  
  func (qo *QueryOptimizer) OptimizeTimelineQuery(userID string, limit int) QueryPlan {
      // Check if we should use parallel queries
      followerCount := qo.getFollowerCount(userID)
      
      if followerCount > 1000 {
          // Use parallel query strategy for users with many followers
          return QueryPlan{
              Strategy: "parallel_scan",
              Queries: []Query{
                  {Index: "timeline-by-date-1", Limit: limit / 4},
                  {Index: "timeline-by-date-2", Limit: limit / 4},
                  {Index: "timeline-by-date-3", Limit: limit / 4},
                  {Index: "timeline-by-date-4", Limit: limit / 4},
              },
              PostProcess: "merge_and_sort",
          }
      } else {
          // Use single query for normal users
          return QueryPlan{
              Strategy: "single_query",
              Queries: []Query{
                  {Index: "timeline-by-user", Limit: limit},
              },
          }
      }
  }
  
  func (qo *QueryOptimizer) OptimizeBatchGet(keys []string) QueryPlan {
      // Group keys by partition
      partitions := make(map[string][]string)
      for _, key := range keys {
          partition := getPartition(key)
          partitions[partition] = append(partitions[partition], key)
      }
      
      // Check if we should use batch get or parallel queries
      if len(partitions) == 1 && len(keys) <= 100 {
          return QueryPlan{
              Strategy: "batch_get",
              BatchKeys: keys,
          }
      } else {
          // Use parallel queries for multiple partitions
          queries := make([]Query, 0, len(partitions))
          for partition, partitionKeys := range partitions {
              queries = append(queries, Query{
                  Type:      "batch_get",
                  Partition: partition,
                  Keys:      partitionKeys,
              })
          }
          
          return QueryPlan{
              Strategy: "parallel_batch",
              Queries:  queries,
          }
      }
  }
  ```

- [ ] Implement adaptive query execution
  ```go
  type AdaptiveQueryExecutor struct {
      optimizer *QueryOptimizer
      executor  *QueryExecutor
      history   *QueryHistory
  }
  
  func (aqe *AdaptiveQueryExecutor) ExecuteQuery(ctx context.Context, query Query) (any, error) {
      // Get historical performance data
      historical := aqe.history.GetStats(query)
      
      // Adapt strategy based on historical data
      if historical.FailureRate > 0.1 {
          // High failure rate - use conservative approach
          query.RetryPolicy = RetryPolicy{
              MaxAttempts: 5,
              Backoff:     ExponentialBackoff(100 * time.Millisecond),
          }
      }
      
      if historical.AvgLatency > 500*time.Millisecond {
          // Slow query - try to optimize
          optimized := aqe.optimizer.Optimize(query)
          if optimized != nil {
              query = *optimized
          }
      }
      
      // Execute with monitoring
      start := time.Now()
      result, err := aqe.executor.Execute(ctx, query)
      duration := time.Since(start)
      
      // Record execution stats
      aqe.history.Record(query, duration, err)
      
      // Adapt for next time
      if err != nil && isThrottleError(err) {
          aqe.optimizer.ReduceThroughput(query.Type)
      }
      
      return result, err
  }
  ```

**Testing Requirements:**
- [ ] Test query patterns
- [ ] Test optimization strategies
- [ ] Test adaptive behavior
- [ ] Measure improvements

**Acceptance Criteria:**
- Queries optimized
- Costs reduced
- Latency improved
- Adaptive to load

## 5.3 Documentation

### 5.3.1 API Documentation
**File:** `docs/api/openapi.yaml`

- [ ] Generate OpenAPI specification
  ```yaml
  openapi: 3.0.0
  info:
    title: Lesser API
    description: Mastodon-compatible ActivityPub server
    version: 1.0.0
    contact:
      name: Lesser Support
      email: support@lesser.app
    license:
      name: AGPLv3
      url: https://www.gnu.org/licenses/agpl-3.0.html
  
  servers:
    - url: https://api.lesser.app
      description: Production server
    - url: https://staging-api.lesser.app
      description: Staging server
  
  security:
    - OAuth2: []
    - BearerToken: []
  
  paths:
    /api/v1/statuses:
      post:
        summary: Create a new status
        operationId: createStatus
        tags:
          - Statuses
        security:
          - OAuth2: [write:statuses]
        requestBody:
          required: true
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/StatusRequest'
        responses:
          '201':
            description: Status created successfully
            content:
              application/json:
                schema:
                  $ref: '#/components/schemas/Status'
          '400':
            $ref: '#/components/responses/BadRequest'
          '401':
            $ref: '#/components/responses/Unauthorized'
          '422':
            $ref: '#/components/responses/UnprocessableEntity'
  
  components:
    schemas:
      Status:
        type: object
        required:
          - id
          - created_at
          - account
        properties:
          id:
            type: string
            description: Unique identifier
          uri:
            type: string
            format: uri
            description: ActivityPub URI
          created_at:
            type: string
            format: date-time
          account:
            $ref: '#/components/schemas/Account'
          content:
            type: string
            description: HTML content
          visibility:
            type: string
            enum: [public, unlisted, private, direct]
          sensitive:
            type: boolean
          spoiler_text:
            type: string
          media_attachments:
            type: array
            items:
              $ref: '#/components/schemas/MediaAttachment'
  ```

- [ ] Create API client examples
  ```markdown
  # API Client Examples
  
  ## Authentication
  
  ### OAuth 2.0 Flow
  
  ```javascript
  // 1. Register your application
  const app = await fetch('https://lesser.app/api/v1/apps', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      client_name: 'My App',
      redirect_uris: 'https://myapp.com/callback',
      scopes: 'read write follow'
    })
  });
  
  const { client_id, client_secret } = await app.json();
  
  // 2. Redirect user to authorization URL
  const authUrl = new URL('https://lesser.app/oauth/authorize');
  authUrl.searchParams.set('client_id', client_id);
  authUrl.searchParams.set('response_type', 'code');
  authUrl.searchParams.set('redirect_uri', 'https://myapp.com/callback');
  authUrl.searchParams.set('scope', 'read write follow');
  
  window.location.href = authUrl.toString();
  
  // 3. Exchange authorization code for access token
  const token = await fetch('https://lesser.app/oauth/token', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      grant_type: 'authorization_code',
      code: authorizationCode,
      client_id: client_id,
      client_secret: client_secret,
      redirect_uri: 'https://myapp.com/callback'
    })
  });
  
  const { access_token } = await token.json();
  ```
  
  ## Common Operations
  
  ### Post a Status
  
  ```python
  import requests
  
  response = requests.post(
      'https://lesser.app/api/v1/statuses',
      headers={'Authorization': f'Bearer {access_token}'},
      json={
          'status': 'Hello from Lesser!',
          'visibility': 'public'
      }
  )
  
  status = response.json()
  print(f"Posted status {status['id']}")
  ```
  ```

**Testing Requirements:**
- [ ] Validate OpenAPI spec
- [ ] Test all examples
- [ ] Generate client SDKs
- [ ] Test authentication flows

**Acceptance Criteria:**
- Complete API coverage
- Examples work
- SDKs generated
- Easy to understand

### 5.3.2 Lift Pattern Guide
**File:** `docs/development/lift-patterns.md`

- [ ] Create Lift pattern documentation
  ```markdown
  # Lift Framework Patterns
  
  ## Handler Patterns
  
  ### Basic HTTP Handler
  
  ```go
  func HandleGetUser(ctx *lift.Context) error {
      userID := ctx.Param("id")
      
      // Validate input
      if userID == "" {
          return lift.NewError(400, "user ID required")
      }
      
      // Get authenticated user from context
      authUserID, err := GetUserID(ctx)
      if err != nil {
          return err
      }
      
      // Fetch user
      user, err := store.GetUser(ctx.Request().Context(), userID)
      if err != nil {
          if errors.Is(err, ErrNotFound) {
              return lift.NewError(404, "user not found")
          }
          return err
      }
      
      // Check permissions
      if !canViewUser(authUserID, user) {
          return lift.NewError(403, "permission denied")
      }
      
      // Return response
      return ctx.JSON(user)
  }
  ```
  
  ### Stream Handler Pattern
  
  ```go
  func HandleActivityStream(ctx *lift.Context, event events.DynamoDBEvent) error {
      // Process records in parallel with error collection
      g, gctx := errgroup.WithContext(ctx.Request().Context())
      
      for _, record := range event.Records {
          record := record // Capture loop variable
          
          g.Go(func() error {
              return processRecord(gctx, record)
          })
      }
      
      if err := g.Wait(); err != nil {
          // Log error but don't fail entire batch
          logger.Error("failed to process some records", zap.Error(err))
          
          // Return partial failure
          return lift.NewError(500, "partial batch failure")
      }
      
      return nil
  }
  ```
  
  ## Middleware Patterns
  
  ### Authentication Middleware
  
  ```go
  func RequireAuth(scopes ...string) lift.Middleware {
      return func(next lift.HandlerFunc) lift.HandlerFunc {
          return func(ctx *lift.Context) error {
              token := extractToken(ctx.Request())
              if token == "" {
                  return lift.NewError(401, "authentication required")
              }
              
              claims, err := validateToken(token)
              if err != nil {
                  return lift.NewError(401, "invalid token")
              }
              
              // Check scopes
              if !hasScopes(claims, scopes) {
                  return lift.NewError(403, "insufficient permissions")
              }
              
              // Add to context
              ctx.Set("claims", claims)
              ctx.Set("userID", claims.UserID)
              
              return next(ctx)
          }
      }
  }
  ```
  
  ## Error Handling Patterns
  
  ### Structured Errors
  
  ```go
  type ValidationError struct {
      Field   string
      Message string
  }
  
  func (e ValidationError) Error() string {
      return fmt.Sprintf("validation error on %s: %s", e.Field, e.Message)
  }
  
  func HandleError(ctx *lift.Context, err error) error {
      switch e := err.(type) {
      case ValidationError:
          return lift.NewError(400, e.Error()).
              WithDetail("field", e.Field).
              WithDetail("code", "VALIDATION_ERROR")
              
      case *lift.Error:
          return e
          
      default:
          // Log internal errors
          logger.Error("internal error", zap.Error(err))
          return lift.NewError(500, "internal server error")
      }
  }
  ```
  
  ## Testing Patterns
  
  ### Handler Testing
  
  ```go
  func TestHandler(t *testing.T) {
      // Setup
      handler := HandleGetUser
      store := &MockStore{}
      
      // Test cases
      tests := []struct {
          name    string
          setup   func()
          userID  string
          wantErr bool
          wantCode int
      }{
          {
              name: "valid user",
              setup: func() {
                  store.On("GetUser", "123").Return(&User{ID: "123"}, nil)
              },
              userID: "123",
              wantErr: false,
          },
          {
              name: "user not found",
              setup: func() {
                  store.On("GetUser", "999").Return(nil, ErrNotFound)
              },
              userID: "999",
              wantErr: true,
              wantCode: 404,
          },
      }
      
      for _, tt := range tests {
          t.Run(tt.name, func(t *testing.T) {
              tt.setup()
              
              ctx, _ := NewTestContext().
                  WithParam("id", tt.userID).
                  WithAuth("user-123").
                  Build()
              
              err := handler(ctx)
              
              if tt.wantErr {
                  assert.Error(t, err)
                  liftErr, ok := err.(*lift.Error)
                  assert.True(t, ok)
                  assert.Equal(t, tt.wantCode, liftErr.StatusCode)
              } else {
                  assert.NoError(t, err)
              }
          })
      }
  }
  ```
  ```

**Testing Requirements:**
- [ ] Test all patterns
- [ ] Validate examples compile
- [ ] Include anti-patterns
- [ ] Performance tips

**Acceptance Criteria:**
- Comprehensive patterns
- Working examples
- Clear explanations
- Best practices included

### 5.3.3 DynamORM Conventions
**File:** `docs/development/dynamorm-guide.md`

- [ ] Document DynamORM patterns
  ```markdown
  # DynamORM Patterns and Conventions
  
  ## Model Definition
  
  ### Basic Model
  
  ```go
  type User struct {
      dynamorm.Model
      
      // Primary key
      ID string `dynamodbav:"pk" dynamorm:"hash_key"`
      
      // Attributes
      Username    string    `dynamodbav:"username" dynamorm:"index:gsi1,hash_key"`
      Email       string    `dynamodbav:"email" dynamorm:"index:gsi2,hash_key"`
      CreatedAt   time.Time `dynamodbav:"created_at"`
      UpdatedAt   time.Time `dynamodbav:"updated_at"`
      
      // Nested types
      Profile     Profile   `dynamodbav:"profile"`
      Settings    Settings  `dynamodbav:"settings"`
  }
  
  func (u *User) TableName() string {
      return os.Getenv("DYNAMODB_TABLE")
  }
  
  func (u *User) GetHashKey() string {
      return fmt.Sprintf("USER#%s", u.ID)
  }
  ```
  
  ### Composite Key Model
  
  ```go
  type Follow struct {
      dynamorm.Model
      
      // Composite primary key
      FollowerID string `dynamodbav:"pk" dynamorm:"hash_key"`
      FolloweeID string `dynamodbav:"sk" dynamorm:"range_key"`
      
      // GSI for reverse lookup
      FolloweeKey string `dynamodbav:"gsi1pk" dynamorm:"index:gsi1,hash_key"`
      FollowerKey string `dynamodbav:"gsi1sk" dynamorm:"index:gsi1,range_key"`
      
      CreatedAt time.Time `dynamodbav:"created_at"`
  }
  
  func (f *Follow) BeforeCreate() error {
      f.FollowerID = fmt.Sprintf("USER#%s", f.FollowerID)
      f.FolloweeID = fmt.Sprintf("FOLLOWS#%s", f.FolloweeID)
      f.FolloweeKey = fmt.Sprintf("USER#%s", strings.TrimPrefix(f.FolloweeID, "FOLLOWS#"))
      f.FollowerKey = fmt.Sprintf("FOLLOWER#%s", strings.TrimPrefix(f.FollowerID, "USER#"))
      return nil
  }
  ```
  
  ## Repository Patterns
  
  ### Basic Repository
  
  ```go
  type UserRepository struct {
      *BaseRepository
  }
  
  func NewUserRepository(client *dynamorm.Client) *UserRepository {
      return &UserRepository{
          BaseRepository: NewBaseRepository(client),
      }
  }
  
  func (r *UserRepository) GetByUsername(ctx context.Context, username string) (*User, error) {
      result, err := r.client.Query(&User{}).
          Index("gsi1").
          KeyCondition("username = :username").
          Value(":username", username).
          One()
      
      if err != nil {
          return nil, err
      }
      
      return result.(*User), nil
  }
  ```
  
  ### Advanced Queries
  
  ```go
  func (r *StatusRepository) GetTimeline(ctx context.Context, userID string, limit int, lastKey map[string]types.AttributeValue) ([]*Status, map[string]types.AttributeValue, error) {
      query := r.client.Query(&Timeline{}).
          KeyCondition("pk = :pk").
          Value(":pk", fmt.Sprintf("USER#%s", userID)).
          ScanIndexForward(false). // Descending order
          Limit(limit)
      
      if lastKey != nil {
          query = query.ExclusiveStartKey(lastKey)
      }
      
      results, err := query.All()
      if err != nil {
          return nil, nil, err
      }
      
      // Batch get the actual statuses
      statusIDs := make([]string, len(results))
      for i, result := range results {
          timeline := result.(*Timeline)
          statusIDs[i] = timeline.StatusID
      }
      
      statuses, err := r.BatchGetStatuses(ctx, statusIDs)
      if err != nil {
          return nil, nil, err
      }
      
      return statuses, query.LastEvaluatedKey(), nil
  }
  ```
  
  ## Transaction Patterns
  
  ### Multi-Item Transaction
  
  ```go
  func (r *Repository) CreateStatusWithTimeline(ctx context.Context, status *Status, followers []string) error {
      tx := r.client.Transaction()
      
      // Add status
      tx.Put(status)
      
      // Add to author's timeline
      tx.Put(&Timeline{
          UserID:    status.UserID,
          StatusID:  status.ID,
          CreatedAt: status.CreatedAt,
      })
      
      // Add to followers' timelines (batch)
      for _, followerID := range followers {
          tx.Put(&Timeline{
              UserID:    followerID,
              StatusID:  status.ID,
              CreatedAt: status.CreatedAt,
          })
      }
      
      // Update stats
      tx.Update(&UserStats{UserID: status.UserID}, 
          "SET status_count = status_count + :inc",
          dynamorm.Values{":inc": 1})
      
      return tx.Commit()
  }
  ```
  
  ## Performance Patterns
  
  ### Batch Operations
  
  ```go
  func (r *Repository) BatchGetUsers(ctx context.Context, userIDs []string) ([]*User, error) {
      // DynamoDB BatchGetItem limit is 100
      const batchSize = 100
      
      var allUsers []*User
      for i := 0; i < len(userIDs); i += batchSize {
          end := i + batchSize
          if end > len(userIDs) {
              end = len(userIDs)
          }
          
          batch := userIDs[i:end]
          keys := make([]dynamorm.Key, len(batch))
          for j, id := range batch {
              keys[j] = dynamorm.Key{
                  HashKey: fmt.Sprintf("USER#%s", id),
              }
          }
          
          results, err := r.client.BatchGet(&User{}, keys)
          if err != nil {
              return nil, err
          }
          
          for _, result := range results {
              allUsers = append(allUsers, result.(*User))
          }
      }
      
      return allUsers, nil
  }
  ```
  
  ### Parallel Queries
  
  ```go
  func (r *Repository) GetUserWithStats(ctx context.Context, userID string) (*UserWithStats, error) {
      g, gctx := errgroup.WithContext(ctx)
      
      var user *User
      var stats *UserStats
      var recentStatuses []*Status
      
      // Parallel fetch
      g.Go(func() error {
          var err error
          user, err = r.GetUser(gctx, userID)
          return err
      })
      
      g.Go(func() error {
          var err error
          stats, err = r.GetUserStats(gctx, userID)
          return err
      })
      
      g.Go(func() error {
          var err error
          recentStatuses, err = r.GetRecentStatuses(gctx, userID, 10)
          return err
      })
      
      if err := g.Wait(); err != nil {
          return nil, err
      }
      
      return &UserWithStats{
          User:           user,
          Stats:          stats,
          RecentStatuses: recentStatuses,
      }, nil
  }
  ```
  ```

**Testing Requirements:**
- [ ] Test all examples
- [ ] Include migration guide
- [ ] Performance benchmarks
- [ ] Common pitfalls

**Acceptance Criteria:**
- Complete examples
- Performance tips
- Migration guide
- Troubleshooting section

### 5.3.4 Troubleshooting Guide
**File:** `docs/operations/troubleshooting.md`

- [ ] Create troubleshooting guide
  ```markdown
  # Lesser Troubleshooting Guide
  
  ## Common Issues
  
  ### High Lambda Cold Starts
  
  **Symptoms:**
  - First request takes >3 seconds
  - Intermittent timeouts
  - Cold start metrics show high values
  
  **Diagnosis:**
  ```bash
  # Check Lambda package size
  aws lambda get-function --function-name lesser-api \
    --query 'Configuration.CodeSize' --output text
  
  # Check cold start metrics
  aws cloudwatch get-metric-statistics \
    --namespace Lesser/Lambda \
    --metric-name cold_start \
    --dimensions Name=FunctionName,Value=api \
    --start-time 2024-01-01T00:00:00Z \
    --end-time 2024-01-02T00:00:00Z \
    --period 3600 \
    --statistics Average,Maximum
  ```
  
  **Solutions:**
  1. **Reduce package size**
     ```bash
     # Analyze binary size
     go tool nm -size bin/api | head -20
     
     # Remove unused dependencies
     go mod tidy
     go mod vendor
     find vendor -name "*_test.go" -delete
     ```
  
  2. **Enable provisioned concurrency**
     ```hcl
     resource "aws_lambda_provisioned_concurrency_config" "api" {
       function_name                     = aws_lambda_function.api.function_name
       provisioned_concurrent_executions = 5
       qualifier                         = aws_lambda_alias.api_live.name
     }
     ```
  
  3. **Use Lambda SnapStart** (Java only)
  
  ### DynamoDB Throttling
  
  **Symptoms:**
  - ProvisionedThroughputExceededException errors
  - Increased latency
  - Failed requests
  
  **Diagnosis:**
  ```bash
  # Check consumed capacity
  aws cloudwatch get-metric-statistics \
    --namespace AWS/DynamoDB \
    --metric-name ConsumedReadCapacityUnits \
    --dimensions Name=TableName,Value=lesser \
    --start-time 2024-01-01T00:00:00Z \
    --end-time 2024-01-02T00:00:00Z \
    --period 300 \
    --statistics Sum,Average,Maximum
  
  # Check for hot partitions
  aws dynamodb describe-table --table-name lesser \
    --query 'Table.ItemCount'
  ```
  
  **Solutions:**
  1. **Enable auto-scaling**
     ```hcl
     resource "aws_appautoscaling_target" "dynamodb_table_read_target" {
       max_capacity       = 40000
       min_capacity       = 5
       resource_id        = "table/${aws_dynamodb_table.main.name}"
       scalable_dimension = "dynamodb:table:ReadCapacityUnits"
       service_namespace  = "dynamodb"
     }
     ```
  
  2. **Optimize hot partitions**
     ```go
     // Add jitter to partition keys
     func getPartitionKey(userID string) string {
         shard := crc32.ChecksumIEEE([]byte(userID)) % 10
         return fmt.Sprintf("USER#%d#%s", shard, userID)
     }
     ```
  
  3. **Implement exponential backoff**
     ```go
     func withRetry(fn func() error) error {
         return retry.Do(fn,
             retry.Attempts(5),
             retry.Delay(100*time.Millisecond),
             retry.DelayType(retry.BackOffDelay),
             retry.OnRetry(func(n uint, err error) {
                 if isThrottleError(err) {
                     log.Printf("Throttled, attempt %d", n)
                 }
             }),
         )
     }
     ```
  
  ### Memory Leaks
  
  **Symptoms:**
  - Lambda memory usage increases over time
  - Out of memory errors
  - Performance degradation
  
  **Diagnosis:**
  ```go
  // Add memory profiling
  import _ "net/http/pprof"
  
  func init() {
      if os.Getenv("ENABLE_PROFILING") == "true" {
          go func() {
              log.Println(http.ListenAndServe("localhost:6060", nil))
          }()
      }
  }
  
  // Check memory stats
  func logMemStats() {
      var m runtime.MemStats
      runtime.ReadMemStats(&m)
      log.Printf("Alloc = %v MB", m.Alloc / 1024 / 1024)
      log.Printf("TotalAlloc = %v MB", m.TotalAlloc / 1024 / 1024)
      log.Printf("Sys = %v MB", m.Sys / 1024 / 1024)
      log.Printf("NumGC = %v", m.NumGC)
  }
  ```
  
  **Solutions:**
  1. **Fix connection leaks**
     ```go
     // Always close response bodies
     resp, err := http.Get(url)
     if err != nil {
         return err
     }
     defer resp.Body.Close()
     
     // Read and discard body
     io.Copy(io.Discard, resp.Body)
     ```
  
  2. **Clear caches periodically**
     ```go
     func clearCaches() {
         ticker := time.NewTicker(5 * time.Minute)
         defer ticker.Stop()
         
         for range ticker.C {
             cache.Clear()
             runtime.GC()
         }
     }
     ```
  
  ## Performance Debugging
  
  ### Slow Queries
  
  **Tools:**
  ```go
  // Add query timing
  func (r *Repository) timedQuery(name string, fn func() error) error {
      start := time.Now()
      err := fn()
      duration := time.Since(start)
      
      if duration > 100*time.Millisecond {
          log.Printf("Slow query %s: %v", name, duration)
      }
      
      metrics.RecordDuration("query.duration", duration,
          metrics.Tag("query", name))
      
      return err
  }
  ```
  
  ### Request Tracing
  
  **AWS X-Ray Integration:**
  ```go
  import "github.com/aws/aws-xray-sdk-go/xray"
  
  func TracedHandler(ctx *lift.Context) error {
      ctx, seg := xray.BeginSegment(ctx.Request().Context(), "HandleRequest")
      defer seg.Close(nil)
      
      // Add metadata
      seg.AddAnnotation("user_id", getUserID(ctx))
      seg.AddMetadata("request", "path", ctx.Request().URL.Path)
      
      // Trace subsegments
      ctx, subseg := xray.BeginSubsegment(ctx, "DatabaseQuery")
      result, err := database.Query(ctx)
      subseg.Close(err)
      
      return nil
  }
  ```
  
  ## Emergency Procedures
  
  ### Service Degradation
  
  1. **Enable circuit breakers**
     ```go
     breaker := circuit.NewBreaker(circuit.Config{
         Timeout:        30 * time.Second,
         MaxRequests:    100,
         Interval:       time.Minute,
         FailureRatio:   0.5,
     })
     ```
  
  2. **Shed load**
     ```go
     if loadShedding.ShouldReject() {
         return lift.NewError(503, "service unavailable")
     }
     ```
  
  3. **Scale horizontally**
     ```bash
     aws lambda update-function-configuration \
       --function-name lesser-api \
       --reserved-concurrent-executions 1000
     ```
  ```

**Testing Requirements:**
- [ ] Test all solutions
- [ ] Validate commands work
- [ ] Include real examples
- [ ] Update regularly

**Acceptance Criteria:**
- Common issues covered
- Solutions work
- Easy to follow
- Regularly updated

## Success Metrics

- [ ] All monitoring in place
- [ ] Alerts configured and tested  
- [ ] Performance optimized
- [ ] Documentation complete
- [ ] Production deployment successful
- [ ] SLAs being met
Aggregation Periods

  Multi-Level Aggregation Strategy

  type AggregationSchedule struct {
      // Real-time (for active monitoring)
      Realtime: AggregationLevel{
          Period:    1 * time.Minute,
          Retention: 1 * time.Hour,
          Metrics:   []string{"requests", "errors", "latency_p50"},
          Storage:   "HOT",  // Keep in DynamoDB
      },

      // Near-time (for dashboards)
      NearTime: AggregationLevel{
          Period:    5 * time.Minute,
          Retention: 24 * time.Hour,
          Metrics:   []string{"all_percentiles", "success_rate",
  "bytes"},
          Storage:   "WARM", // DynamoDB with TTL
      },

      // Hourly (for trending)
      Hourly: AggregationLevel{
          Period:    1 * time.Hour,
          Retention: 7 * 24 * time.Hour,
          Metrics:   []string{"aggregated_stats", "cost",
  "patterns"},
          Storage:   "COOL", // Compressed in DynamoDB
      },

      // Daily (for reporting)
      Daily: AggregationLevel{
          Period:    24 * time.Hour,
          Retention: 90 * 24 * time.Hour,
          Metrics:   []string{"summaries", "trends", "anomalies"},
          Storage:   "COLD", // S3 with lifecycle
      },

      // Monthly (for capacity planning)
      Monthly: AggregationLevel{
          Period:    30 * 24 * time.Hour,
          Retention: 2 * 365 * 24 * time.Hour,
          Metrics:   []string{"growth", "costs", "forecasts"},
          Storage:   "ARCHIVE", // S3 Glacier
      },
  }

  Optimal Primary Aggregation: 5 Minutes

  const (
      // 5 minutes balances granularity with storage costs
      PrimaryAggregationPeriod = 5 * time.Minute

      // Reasons:
      // - Captures 100-500 events per period (statistically 
  significant)
      // - Aligns with CloudWatch metrics period
      // - Reduces DynamoDB writes by 300x vs raw events
      // - Still responsive enough for alerting
  )

  Compression and Archival Strategy

  Progressive Compression Pipeline

  type CompressionStrategy struct {
      // Level 1: Aggregate raw events (immediate)
      RawToAggregated: CompressionRule{
          After:       5 * time.Minute,
          Method:      "STATISTICAL_SUMMARY",
          Compression: 50.0, // 50:1 reduction
          Keeps: []string{
              "count", "sum", "min", "max", "p50", "p95", "p99",
          },
      },

      // Level 2: Compress old aggregates (after 24h)
      AggregatedToCompressed: CompressionRule{
          After:       24 * time.Hour,
          Method:      "GZIP_JSON",
          Compression: 5.0, // 5:1 additional reduction
          Format:      "BINARY",
      },

      // Level 3: Archive to S3 (after 7 days)
      CompressedToArchived: CompressionRule{
          After:   7 * 24 * time.Hour,
          Method:  "PARQUET",
          Storage: "S3_INTELLIGENT_TIERING",
          Compression: 10.0, // 10:1 with columnar format
      },

      // Level 4: Deep archive (after 90 days)
      ArchivedToGlacier: CompressionRule{
          After:   90 * 24 * time.Hour,
          Storage: "GLACIER_DEEP_ARCHIVE",
          Cost:    "$0.00099/GB/month", // 99% cost reduction
      },
  }

  func (c *CompressionStrategy) CompressMetrics(metrics 
  *FederationMetrics) []byte {
      // Use different algorithms based on data type
      switch metrics.DataType {
      case "TIME_SERIES":
          // Delta encoding for timestamps
          return c.deltaEncode(metrics)

      case "COUNTERS":
          // Run-length encoding for sparse data
          return c.runLengthEncode(metrics)

      case "DISTRIBUTIONS":
          // T-Digest for percentile approximation
          return c.tDigestCompress(metrics)

      default:
          // General purpose compression
          return c.gzipCompress(metrics)
      }
  }

  S3 Lifecycle Configuration

  type S3ArchivalConfig struct {
      Bucket: "lesser-federation-analytics",

      Lifecycle: []Rule{
          {
              Prefix:     "metrics/daily/",
              Transition: "STANDARD_IA",
              After:      30 * 24 * time.Hour,
          },
          {
              Prefix:     "metrics/daily/",
              Transition: "GLACIER_IR",
              After:      90 * 24 * time.Hour,
          },
          {
              Prefix:     "metrics/monthly/",
              Transition: "GLACIER_DEEP_ARCHIVE",
              After:      365 * 24 * time.Hour,
          },
      },

      // Partition for efficient queries
      PartitionStrategy: "year/month/day/hour",
      Format:           "parquet",
      Compression:      "snappy",
  }

  Critical Federation Health Metrics

  Top Priority Metrics

  type FederationHealthMetrics struct {
      // 1. Availability (Most Critical)
      Availability: AvailabilityMetrics{
          InstanceReachability:   float64,    // % instances 
  responding
          EndpointAvailability:   float64,    // % endpoints up
          CircuitBreakerStatus:   map[string]string,
          LastSuccessfulContact:  map[string]time.Time,
          ConsecutiveFailures:    map[string]int,
      },

      // 2. Performance (User Experience)
      Performance: PerformanceMetrics{
          InboxDeliveryP50:       time.Duration,
          InboxDeliveryP95:       time.Duration,
          InboxDeliveryP99:       time.Duration,
          OutboxProcessingTime:   time.Duration,
          SignatureVerification:  time.Duration,
          MediaDeliveryTime:      time.Duration,
      },

      // 3. Throughput (Capacity)
      Throughput: ThroughputMetrics{
          IncomingActivities:     rate.Rate,  // per second
          OutgoingActivities:     rate.Rate,
          QueueDepth:            int,
          ProcessingBacklog:     time.Duration,
          BurstCapacity:         float64,
      },

      // 4. Error Rates (Reliability)
      Errors: ErrorMetrics{
          SignatureFailures:      rate.Rate,
          TimeoutRate:           float64,
          RateLimitHits:         map[string]int,
          MalformedActivities:   int,
          ValidationFailures:    map[string]int,
      },

      // 5. Cost Efficiency
      Cost: CostMetrics{
          PerActivityCost:       float64,
          PerInstanceCost:       map[string]float64,
          BandwidthCost:         float64,
          ComputeCost:           float64,
          StorageCost:           float64,
          EgressCost:            float64,
      },
  }

  Instance-Specific Health Scoring

  type InstanceHealthScore struct {
      Calculate: func(instance string, window time.Duration) 
  float64 {
          metrics := getMetrics(instance, window)

          // Weighted scoring (0-100)
          score := 0.0

          // Availability: 40% weight
          score += metrics.SuccessRate * 40.0

          // Performance: 30% weight
          if metrics.P95Latency < 2*time.Second {
              score += 30.0
          } else if metrics.P95Latency < 5*time.Second {
              score += 20.0
          } else if metrics.P95Latency < 10*time.Second {
              score += 10.0
          }

          // Reliability: 20% weight
          errorRate := float64(metrics.Errors) /
  float64(metrics.Total)
          score += (1.0 - errorRate) * 20.0

          // Activity: 10% weight
          if metrics.LastActivity.After(time.Now().Add(-1 *
  time.Hour)) {
              score += 10.0
          } else if metrics.LastActivity.After(time.Now().Add(-24 *
   time.Hour)) {
              score += 5.0
          }

          return score
      },

      Thresholds: HealthThresholds{
          Healthy:   80.0,  // > 80
          Degraded:  60.0,  // 60-80
          Unhealthy: 40.0,  // 40-60
          Critical:  0.0,   // < 40
      },
  }

  Alert Configuration

  type AlertingConfig struct {
      // Critical alerts (immediate page)
      Critical: []Alert{
          {
              Metric:    "instance_reachability",
              Condition: "< 50%",
              Window:    5 * time.Minute,
              Action:    "PAGE_ONCALL",
          },
          {
              Metric:    "signature_failures",
              Condition: "> 100/min",
              Window:    1 * time.Minute,
              Action:    "PAGE_SECURITY",
          },
      },

      // Warning alerts (notify team)
      Warning: []Alert{
          {
              Metric:    "p95_latency",
              Condition: "> 5s",
              Window:    15 * time.Minute,
              Action:    "SLACK_NOTIFY",
          },
          {
              Metric:    "queue_depth",
              Condition: "> 10000",
              Window:    5 * time.Minute,
              Action:    "SCALE_UP",
          },
      },

      // Info alerts (dashboard only)
      Info: []Alert{
          {
              Metric:    "cost_per_activity",
              Condition: "> $0.001",
              Window:    1 * time.Hour,
              Action:    "DASHBOARD",
          },
      },
  }

  Dashboard Metrics Priority

  type DashboardLayout struct {
      // Top row: Critical health indicators
      TopMetrics: []Widget{
          "Federation Health Score",      // Overall 0-100
          "Active Instances",             // Count
          "Success Rate",                 // Percentage
          "P95 Latency",                 // Milliseconds
      },

      // Second row: Traffic patterns
      TrafficMetrics: []Widget{
          "Activities/Second",            // Line graph
          "Queue Depth",                  // Area chart
          "Bandwidth Usage",              // Stacked area
          "Geographic Distribution",      // Heat map
      },

      // Third row: Error tracking
      ErrorMetrics: []Widget{
          "Error Rate by Type",           // Stacked bar
          "Failed Instances",             // List
          "Signature Failures",           // Time series
          "Timeout Trends",               // Line graph
      },

      // Bottom row: Cost optimization
      CostMetrics: []Widget{
          "Cost per 1K Activities",       // Gauge
          "Cost by Instance",             // Pie chart
          "Cost Trend",                   // Line graph
          "Projected Monthly Cost",       // Number
      },
  }

  Recommended Production Configuration

  config := &FederationAnalyticsConfig{
      // Aggregation
      PrimaryPeriod:     5 * time.Minute,
      RetentionRaw:      1 * time.Hour,
      RetentionAggr:     7 * 24 * time.Hour,
      RetentionArchive:  2 * 365 * 24 * time.Hour,

      // Compression
      EnableCompression:  true,
      CompressionAfter:   24 * time.Hour,
      CompressionRatio:   10.0,

      // Archival
      ArchiveToS3:        true,
      S3Bucket:           "federation-analytics",
      S3StorageClass:     "INTELLIGENT_TIERING",

      // Key metrics to track
      RequiredMetrics: []string{
          "instance_health_score",
          "success_rate",
          "p50_latency",
          "p95_latency",
          "p99_latency",
          "error_rate",
          "signature_failures",
          "queue_depth",
          "cost_per_activity",
      },

      // Alerting
      EnableAlerting:     true,
      AlertingEndpoint:   "sns://federation-alerts",
      CriticalThreshold:  3, // 3 consecutive failures
  }
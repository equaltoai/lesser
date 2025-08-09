route optimization thresholds and handling:

  Latency/Success Rate Thresholds

  Latency Thresholds

  // Trigger route change when:
  P95Latency > 5 seconds     // ActivityPub generally expects < 10s
  P99Latency > 10 seconds    // Hard limit before timeout
  AvgLatency > 2 seconds     // Sustained poor performance

  // Degradation detection:
  - 50% increase in P95 over 5-minute window
  - 3 consecutive failures with timeout

  Success Rate Thresholds

  // Critical thresholds:
  < 50% success rate  → Open circuit immediately
  < 70% success rate  → Mark as degraded, reduce traffic
  < 90% success rate  → Monitor closely, consider alternatives
  > 95% success rate  → Preferred route

  // Time windows:
  - Last 100 requests OR last 5 minutes (whichever is more recent)
  - Minimum 10 samples before making decisions

  Route Decision Caching

  Cache Duration Strategy

  type RouteCachePolicy struct {
      // Base cache TTL by route health
      HealthyRouteTTL:   5 * time.Minute    // Stable routes
      DegradedRouteTTL:  30 * time.Second   // Re-evaluate 
  frequently  
      UnknownRouteTTL:   1 * time.Minute    // New routes

      // Adaptive caching based on message type
      HighPriorityTTL:   10 * time.Second   // Direct messages, 
  mentions
      NormalPriorityTTL: 2 * time.Minute    // Regular posts
      LowPriorityTTL:    10 * time.Minute   // Bulk updates, 
  deletes
  }

  // Invalidate cache on:
  - Circuit state change (open/close)
  - 3 consecutive failures
  - Significant latency spike (>2x normal)

  Cache Key Structure

  // Cache by combination of:
  cacheKey := fmt.Sprintf("%s:%s:%s:%d",
      sourceInstance,
      targetInstance,
      activityType,    // Create, Update, Delete, etc.
      messageSizeClass // 0-1KB, 1-10KB, 10KB+
  )

  Handling All-Routes-Degraded Scenarios

  Graceful Degradation Strategy

  type DegradationStrategy struct {
      // 1. Prioritize by message importance
      PriorityLevels: []Priority{
          Critical,  // Direct replies, mentions
          High,      // Follows, likes from verified accounts  
          Normal,    // Regular posts, boosts
          Low,       // Deletes, updates to old content
      }

      // 2. Implement progressive backpressure
      BackpressureRules: []Rule{
          {Threshold: 0.7, Action: "Queue low priority"},
          {Threshold: 0.5, Action: "Queue normal priority"},
          {Threshold: 0.3, Action: "Only critical messages"},
          {Threshold: 0.0, Action: "Emergency mode"},
      }
  }

  Emergency Mode Operations

  func handleAllRoutesDegraded(ctx context.Context) error {
      // 1. Switch to emergency queuing
      // Store messages in DLQ with exponential retry

      // 2. Implement message batching
      // Combine multiple small messages to reduce overhead

      // 3. Use fallback relay servers
      fallbackRelays := []string{
          "relay.mastodon.social",
          "relay.fedi.buzz",
      }

      // 4. Notify administrators
      sendAlert(AlertLevel.Critical, "All federation routes 
  degraded")

      // 5. Implement rate limiting
      rateLimits := map[Priority]rate.Limit{
          Critical: rate.Every(1 * time.Second),
          High:     rate.Every(5 * time.Second),
          Normal:   rate.Every(30 * time.Second),
          Low:      rate.Every(5 * time.Minute),
      }

      // 6. Return user-friendly error
      return &FederationError{
          Code:    "FEDERATION_DEGRADED",
          Message: "Federation temporarily limited. Messages queued
   for delivery.",
          Retry:   time.Now().Add(5 * time.Minute),
      }
  }

  Recovery Detection

  type RecoveryDetector struct {
      // Probe configuration
      ProbeInterval:     30 * time.Second
      ProbeTimeout:      5 * time.Second
      SuccessThreshold:  3  // Consecutive successes to mark 
  healthy

      // Gradual recovery
      RecoverySteps: []Step{
          {Load: 0.1, Duration: 1 * time.Minute},  // 10% traffic
          {Load: 0.3, Duration: 2 * time.Minute},  // 30% traffic
          {Load: 0.5, Duration: 5 * time.Minute},  // 50% traffic
          {Load: 1.0, Duration: 0},                // Full traffic
      }
  }

  Recommended Configuration

  // OptimizerConfig with specific values
  config := &OptimizerConfig{
      // Weights for scoring routes
      LatencyWeight:     0.4,  // 40% weight
      ReliabilityWeight: 0.5,  // 50% weight (most important)
      CostWeight:        0.1,  // 10% weight

      // Hard limits
      MaxAcceptableLatency: 10 * time.Second,
      MinAcceptableSuccess: 0.5,  // 50% minimum
      MaxCostPerMB:         0.001, // $0.001 per MB

      // Learning parameters
      HistoryWindow:      15 * time.Minute,
      MinSamplesRequired: 10,
      AdaptationRate:     0.3, // 30% weight to new data
  }

  These thresholds balance reliability with performance while
  ensuring the system can gracefully handle degraded conditions
  without completely failing.
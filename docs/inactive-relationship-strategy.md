Inactive Relationship Strategy

  Lifecycle States

  type RelationshipState string

  const (
      StateActive     RelationshipState = "ACTIVE"      // Recent 
  activity
      StateIdle       RelationshipState = "IDLE"        // No 
  recent activity
      StateDormant    RelationshipState = "DORMANT"     // Long 
  inactive
      StateArchived   RelationshipState = "ARCHIVED"    // Moved to
   cold storage
      StateExpired    RelationshipState = "EXPIRED"     // Marked 
  for deletion
  )

  type RelationshipLifecycle struct {
      // Transition thresholds
      ActiveToIdle:     7 * 24 * time.Hour     // 1 week
      IdleToDormant:    30 * 24 * time.Hour    // 1 month  
      DormantToArchive: 90 * 24 * time.Hour    // 3 months
      ArchiveToExpire:  365 * 24 * time.Hour   // 1 year
  }

  Progressive Degradation

  type InactivityHandler struct {
      // Reduce monitoring frequency as relationships age
      MonitoringSchedule: map[RelationshipState]time.Duration{
          StateActive:   5 * time.Minute,     // Check every 5 min
          StateIdle:     1 * time.Hour,       // Check hourly
          StateDormant:  24 * time.Hour,      // Check daily
          StateArchived: 0,                   // No active 
  monitoring
      },

      // Reduce stored metrics
      MetricRetention: map[RelationshipState]MetricPolicy{
          StateActive: {
              DetailLevel: "FULL",
              Resolution:  1 * time.Minute,
              Retention:   7 * 24 * time.Hour,
          },
          StateIdle: {
              DetailLevel: "SUMMARY",
              Resolution:  1 * time.Hour,
              Retention:   30 * 24 * time.Hour,
          },
          StateDormant: {
              DetailLevel: "MINIMAL",
              Resolution:  24 * time.Hour,
              Retention:   90 * 24 * time.Hour,
          },
      },
  }

  Reactivation Handling

  Smart Reactivation

  type ReactivationStrategy struct {
      // Warm-up period for reactivated relationships
      WarmupConfig: WarmupConfig{
          Duration:        1 * time.Hour,
          InitialRate:     0.1,  // Start with 10% traffic
          RampUpInterval:  10 * time.Minute,
          RampUpFactor:    2.0,  // Double every interval
      },

      // Use historical data to predict behavior
      UseHistoricalMetrics: true,
      HistoricalWeight:     0.3,  // 30% weight to old metrics

      // Probe before full reactivation
      ProbeFirst:      true,
      ProbeTimeout:    5 * time.Second,
      ProbeRetries:    3,
  }

  func (r *ReactivationStrategy) HandleReactivation(rel 
  *Relationship) error {
      // 1. Check if instance is still alive
      if !r.probeInstance(rel.TargetInstance) {
          return r.markAsUnreachable(rel)
      }

      // 2. Restore from archive if needed
      if rel.State == StateArchived {
          if err := r.restoreFromArchive(rel); err != nil {
              return err
          }
      }

      // 3. Apply warm-up period
      rel.State = StateActive
      rel.WarmupUntil = time.Now().Add(r.WarmupConfig.Duration)
      rel.CurrentRate = r.WarmupConfig.InitialRate

      // 4. Initialize with historical baseline
      if r.UseHistoricalMetrics {
          rel.SuccessRate = r.calculateHistoricalBaseline(rel)
      } else {
          rel.SuccessRate = 0.5  // Neutral starting point
      }

      return r.save(rel)
  }

  Storage Optimization

  Archival Strategy for DynamoDB Cost Reduction

  type ArchivalStrategy struct {
      // Move inactive data to reduce hot storage costs
      ArchiveRules: []ArchiveRule{
          {
              State:       StateDormant,
              Action:      "COMPRESS_METRICS",
              Compression: "gzip",
              Reduction:   0.8,  // 80% size reduction
          },
          {
              State:  StateArchived,
              Action: "MOVE_TO_S3",
              S3Config: S3Config{
                  Bucket:       "lesser-archive",
                  StorageClass: "GLACIER_IR",  // $0.004/GB/month
                  Prefix:       "relationships/",
              },
          },
      },

      // Keep minimal index in DynamoDB
      IndexRetention: IndexPolicy{
          Fields: []string{
              "relationship_id",
              "last_activity",
              "state",
              "archive_location",
          },
          TTL: 365 * 24 * time.Hour,
      },
  }

  func (a *ArchivalStrategy) ArchiveRelationship(rel *Relationship)
   error {
      // 1. Aggregate metrics into summary
      summary := a.aggregateMetrics(rel)

      // 2. Compress and store in S3
      archived := &ArchivedRelationship{
          ID:        rel.ID,
          Summary:   summary,
          Timestamp: time.Now(),
      }

      data, _ := json.Marshal(archived)
      compressed := a.compress(data)

      s3Key := fmt.Sprintf("%s%s/%s.json.gz",
          a.S3Config.Prefix,
          time.Now().Format("2006/01/02"),
          rel.ID,
      )

      // 3. Update DynamoDB index
      index := &RelationshipIndex{
          PK:              fmt.Sprintf("REL#%s", rel.ID),
          SK:              "INDEX",
          State:           StateArchived,
          LastActivity:    rel.LastActivity,
          ArchiveLocation: s3Key,
          TTL:             time.Now().Add(365 * 24 *
  time.Hour).Unix(),
      }

      return a.db.Transaction(func(tx *dynamorm.Transaction) error
  {
          tx.Delete(rel)  // Remove full record
          tx.Create(index) // Keep index only
          return nil
      })
  }

  Cleanup Policies

  Automatic Cleanup Rules

  type CleanupPolicy struct {
      // Different policies by relationship type
      Policies: map[RelationType]CleanupRule{
          TypeFollow: {
              InactiveAfter: 180 * 24 * time.Hour,  // 6 months
              Action:        "ARCHIVE",
              Reversible:    true,
          },
          TypeBlock: {
              InactiveAfter: 0,  // Never clean up blocks
              Action:        "KEEP",
              Reversible:    false,
          },
          TypeMute: {
              InactiveAfter: 90 * 24 * time.Hour,  // 3 months
              Action:        "DELETE",
              Reversible:    false,
          },
          TypeTemporary: {
              InactiveAfter: 24 * time.Hour,  // 1 day
              Action:        "DELETE",
              Reversible:    false,
          },
      },

      // Batch processing for efficiency
      BatchConfig: BatchConfig{
          Size:     100,
          Interval: 1 * time.Hour,
          MaxAge:   24 * time.Hour,  // Process daily
      },
  }

  func (c *CleanupPolicy) ProcessInactiveRelationships(ctx 
  context.Context) error {
      // Run as scheduled Lambda function
      cutoff := time.Now().Add(-c.InactiveAfter)

      var lastKey string
      for {
          batch, nextKey, err := c.getInactiveBatch(ctx, cutoff,
  lastKey)
          if err != nil {
              return err
          }

          for _, rel := range batch {
              rule := c.Policies[rel.Type]
              switch rule.Action {
              case "ARCHIVE":
                  c.archive(rel)
              case "DELETE":
                  c.delete(rel)
              case "COMPRESS":
                  c.compress(rel)
              }
          }

          if nextKey == "" {
              break
          }
          lastKey = nextKey
      }

      return nil
  }

  Detection Mechanisms

  Inactivity Detection

  type InactivityDetector struct {
      // Check patterns
      Checks: []Check{
          {
              Name:      "NoRecentActivity",
              Condition: "last_activity < NOW - 7d",
              Action:    "MARK_IDLE",
          },
          {
              Name:      "InstanceOffline",
              Condition: "instance_last_seen < NOW - 30d",
              Action:    "MARK_DORMANT",
          },
          {
              Name:      "RepeatedFailures",
              Condition: "failure_count > 100 AND success_count = 
  0",
              Action:    "MARK_EXPIRED",
          },
      },

      // Bulk detection via GSI query
      DetectInactive: func(ctx context.Context) ([]*Relationship, 
  error) {
          cutoff := time.Now().Add(-7 * 24 * time.Hour)

          return db.Query(&Relationship{}).
              Index("GSI-LastActivity").
              Where("State", "=", "ACTIVE").
              Where("LastActivity", "<", cutoff).
              Limit(1000).
              All()
      },
  }

  Recommended Configuration

  Production Settings

  config := &InactiveRelationshipConfig{
      // Transition timings
      IdleAfter:     7 * 24 * time.Hour,
      DormantAfter:  30 * 24 * time.Hour,
      ArchiveAfter:  90 * 24 * time.Hour,
      ExpireAfter:   365 * 24 * time.Hour,

      // Cost optimization
      CompressIdleMetrics:     true,
      ArchiveDormantToS3:      true,
      DeleteExpiredCompletely: true,

      // Reactivation
      WarmupDuration:      1 * time.Hour,
      ProbeBeforeReactive: true,
      UseHistoricalData:   true,

      // Monitoring
      CheckInterval:     1 * time.Hour,
      BatchSize:         100,
      MaxProcessingTime: 5 * time.Minute,

      // Special cases
      NeverExpire: []RelationType{
          TypeBlock,      // Security relationships
          TypeReport,     // Moderation history
          TypeSuspension, // Admin actions
      },
  }
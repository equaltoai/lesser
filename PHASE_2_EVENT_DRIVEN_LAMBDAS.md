# Phase 2: Event-Driven Lambdas - Detailed Implementation Checklist

## 2.1 DynamoDB Stream Processors

### 2.1.1 Activity Processor Migration
**Current File:** `cmd/activity-processor/main.go`
**New File:** `cmd/activity-processor/main_lift.go`

- [ ] Create Lift stream handler structure
  ```go
  package main
  
  import (
      "context"
      "github.com/aron23/lesser/pkg/lift/app"
      "github.com/aron23/lesser/pkg/storage/dynamorm"
      "github.com/aws/aws-lambda-go/events"
      "github.com/aws/aws-lambda-go/lambda"
      "github.com/pay-theory/lift/pkg/lift"
  )
  
  type ActivityProcessor struct {
      store  storage.Storage
      logger *zap.Logger
      metrics *metrics.Client
  }
  
  func NewActivityProcessor() (*ActivityProcessor, error) {
      // Initialize dependencies
      store, err := dynamodb.New()
      if err != nil {
          return nil, err
      }
      
      return &ActivityProcessor{
          store:  store,
          logger: common.Logger(),
          metrics: metrics.NewClient(),
      }, nil
  }
  ```

- [ ] Implement stream handler
  ```go
  func (ap *ActivityProcessor) HandleStream(ctx *lift.Context, event events.DynamoDBEvent) error {
      // Add request tracking
      requestID := uuid.New().String()
      ctx.Set("requestID", requestID)
      
      ap.logger.Info("processing stream batch",
          zap.String("request_id", requestID),
          zap.Int("record_count", len(event.Records)),
      )
      
      // Process records in parallel with error collection
      errors := make([]error, 0)
      var errorMutex sync.Mutex
      
      var wg sync.WaitGroup
      sem := make(chan struct{}, 10) // Limit concurrency
      
      for _, record := range event.Records {
          wg.Add(1)
          sem <- struct{}{}
          
          go func(record events.DynamoDBEventRecord) {
              defer wg.Done()
              defer func() { <-sem }()
              
              if err := ap.processRecord(ctx, record); err != nil {
                  errorMutex.Lock()
                  errors = append(errors, err)
                  errorMutex.Unlock()
                  
                  ap.logger.Error("failed to process record",
                      zap.String("event_id", record.EventID),
                      zap.Error(err),
                  )
              }
          }(record)
      }
      
      wg.Wait()
      
      if len(errors) > 0 {
          return lift.NewError(500, "partial batch failure").
              WithDetail("failed_count", len(errors)).
              WithDetail("total_count", len(event.Records))
      }
      
      return nil
  }
  ```

- [ ] Implement record processor
  ```go
  func (ap *ActivityProcessor) processRecord(ctx *lift.Context, record events.DynamoDBEventRecord) error {
      // Parse the record
      item, err := dynamorm.UnmarshalStreamImage(record.Change.NewImage)
      if err != nil {
          return fmt.Errorf("unmarshal failed: %w", err)
      }
      
      // Route based on entity type
      switch item.Type {
      case "Status":
          return ap.processStatus(ctx, item)
      case "Like":
          return ap.processLike(ctx, item)
      case "Follow":
          return ap.processFollow(ctx, item)
      default:
          ap.logger.Warn("unknown entity type",
              zap.String("type", item.Type),
          )
          return nil
      }
  }
  ```

- [ ] Add error handling and DLQ
  ```go
  func (ap *ActivityProcessor) HandleStreamWithDLQ(ctx *lift.Context, event events.DynamoDBEvent) error {
      err := ap.HandleStream(ctx, event)
      
      if err != nil {
          // Send failed records to DLQ
          if batchErr, ok := err.(*BatchProcessingError); ok {
              for _, failedRecord := range batchErr.FailedRecords {
                  if err := ap.sendToDLQ(ctx, failedRecord); err != nil {
                      ap.logger.Error("failed to send to DLQ",
                          zap.String("event_id", failedRecord.EventID),
                          zap.Error(err),
                      )
                  }
              }
          }
      }
      
      return err
  }
  ```

- [ ] Implement metrics collection
  ```go
  func (ap *ActivityProcessor) recordMetrics(ctx *lift.Context, recordType string, duration time.Duration, err error) {
      status := "success"
      if err != nil {
          status = "error"
      }
      
      ap.metrics.RecordDuration("activity_processor.duration",
          duration,
          metrics.Tag("type", recordType),
          metrics.Tag("status", status),
      )
      
      ap.metrics.Increment("activity_processor.processed",
          metrics.Tag("type", recordType),
          metrics.Tag("status", status),
      )
  }
  ```

- [ ] Update main function
  ```go
  func main() {
      processor, err := NewActivityProcessor()
      if err != nil {
          log.Fatal("failed to initialize processor", err)
      }
      
      app := lift.New()
      
      // Add middleware
      app.Use(middleware.LoggingMiddleware(processor.logger))
      app.Use(middleware.ErrorHandlerMiddleware())
      app.Use(middleware.MetricsMiddleware(processor.metrics))
      
      // Set handler
      app.DynamoDBStream(processor.HandleStreamWithDLQ)
      
      lambda.Start(app.HandleRequest)
  }
  ```

**Testing Requirements:**
- [ ] Unit test for each record type processor
- [ ] Test parallel processing with race conditions
- [ ] Test error handling and partial failures
- [ ] Test DLQ functionality
- [ ] Integration test with real DynamoDB events

**Acceptance Criteria:**
- Processes all record types correctly
- Handles partial batch failures
- Sends failed records to DLQ
- Maintains processing order where required
- Metrics accurately tracked

### 2.1.2 Federation Timeseries Processor
**File:** `cmd/federation-timeseries/main_lift.go`

- [ ] Create aggregation handler
  ```go
  type TimeseriesProcessor struct {
      store      storage.Storage
      aggregator *TimeSeries
      batcher    *Batcher
  }
  
  func (tp *TimeseriesProcessor) HandleStream(ctx *lift.Context, event events.DynamoDBEvent) error {
      // Group records by time window
      windows := tp.groupByTimeWindow(event.Records)
      
      // Process each window
      for window, records := range windows {
          if err := tp.processWindow(ctx, window, records); err != nil {
              tp.logger.Error("failed to process window",
                  zap.Time("window", window),
                  zap.Error(err),
              )
              // Continue processing other windows
          }
      }
      
      return nil
  }
  ```

- [ ] Implement batching optimization
  ```go
  func (tp *TimeseriesProcessor) processWindow(ctx *lift.Context, window time.Time, records []events.DynamoDBEventRecord) error {
      // Aggregate metrics
      metrics := tp.aggregateMetrics(records)
      
      // Batch write to DynamoDB
      return tp.batcher.Write(ctx, metrics)
  }
  ```

- [ ] Add cost optimization
  ```go
  func (tp *TimeseriesProcessor) optimizeWrites(metrics []Metric) []Metric {
      // Combine similar metrics
      // Compress data where possible
      // Use batch writes efficiently
      return optimized
  }
  ```

**Testing Requirements:**
- [ ] Test time window grouping
- [ ] Test metric aggregation accuracy
- [ ] Test batch write optimization
- [ ] Test cost tracking

**Acceptance Criteria:**
- Accurate time-series aggregation
- Optimized batch writes
- Cost per operation tracked
- Handles out-of-order events

### 2.1.3 Search Indexer Migration
**File:** `cmd/search-indexer/main_lift.go`

- [ ] Create search index handler
  ```go
  type SearchIndexer struct {
      store       storage.Storage
      indexClient *SearchClient
      validator   *ContentValidator
  }
  
  func (si *SearchIndexer) HandleStream(ctx *lift.Context, event events.DynamoDBEvent) error {
      // Filter indexable content
      indexableRecords := si.filterIndexable(event.Records)
      
      // Batch index updates
      batch := make([]IndexOperation, 0, len(indexableRecords))
      
      for _, record := range indexableRecords {
          op, err := si.createIndexOperation(record)
          if err != nil {
              si.logger.Warn("skipping record",
                  zap.String("event_id", record.EventID),
                  zap.Error(err),
              )
              continue
          }
          batch = append(batch, op)
      }
      
      // Execute batch
      return si.indexClient.BatchUpdate(ctx, batch)
  }
  ```

- [ ] Implement retry with exponential backoff
  ```go
  func (si *SearchIndexer) executeWithRetry(ctx context.Context, op func() error) error {
      return retry.Do(
          op,
          retry.Attempts(3),
          retry.Delay(100*time.Millisecond),
          retry.DelayType(retry.BackOffDelay),
          retry.OnRetry(func(n uint, err error) {
              si.logger.Warn("retrying operation",
                  zap.Uint("attempt", n),
                  zap.Error(err),
              )
          }),
      )
  }
  ```

- [ ] Add index corruption detection
  ```go
  func (si *SearchIndexer) detectCorruption(ctx context.Context) error {
      // Compare index with source data
      // Identify missing or stale entries
      // Trigger reindexing if needed
  }
  ```

**Testing Requirements:**
- [ ] Test index operation creation
- [ ] Test retry logic
- [ ] Test corruption detection
- [ ] Test batch processing

**Acceptance Criteria:**
- All content properly indexed
- Retry logic handles transient failures
- Corruption detection works
- Performance metrics tracked

## 2.2 SQS Queue Processors

### 2.2.1 Push Delivery Processor
**File:** `cmd/push-delivery/main_lift.go`

- [ ] Create SQS handler
  ```go
  type PushDeliveryProcessor struct {
      pushService *push.Service
      store       storage.Storage
      rateLimiter *RateLimiter
  }
  
  func (pdp *PushDeliveryProcessor) HandleSQSBatch(ctx *lift.Context, event events.SQSEvent) error {
      // Process messages concurrently
      results := make(chan error, len(event.Records))
      
      for _, record := range event.Records {
          go func(msg events.SQSMessage) {
              results <- pdp.processMessage(ctx, msg)
          }(record)
      }
      
      // Collect results
      var failures []error
      for i := 0; i < len(event.Records); i++ {
          if err := <-results; err != nil {
              failures = append(failures, err)
          }
      }
      
      // Return batch item failures
      if len(failures) > 0 {
          return lift.NewSQSBatchError(failures)
      }
      
      return nil
  }
  ```

- [ ] Implement message processor
  ```go
  func (pdp *PushDeliveryProcessor) processMessage(ctx *lift.Context, msg events.SQSMessage) error {
      // Parse notification
      var notification PushNotification
      if err := json.Unmarshal([]byte(msg.Body), &notification); err != nil {
          return fmt.Errorf("invalid message format: %w", err)
      }
      
      // Check rate limits
      if !pdp.rateLimiter.Allow(notification.UserID) {
          return ErrRateLimited
      }
      
      // Deliver push notification
      return pdp.pushService.Send(ctx, notification)
  }
  ```

- [ ] Add delivery status tracking
  ```go
  func (pdp *PushDeliveryProcessor) trackDelivery(ctx context.Context, notification PushNotification, result DeliveryResult) error {
      return pdp.store.SaveDeliveryStatus(ctx, DeliveryStatus{
          NotificationID: notification.ID,
          UserID:        notification.UserID,
          Status:        result.Status,
          DeliveredAt:   time.Now(),
          Error:         result.Error,
      })
  }
  ```

**Testing Requirements:**
- [ ] Test batch processing
- [ ] Test rate limiting
- [ ] Test delivery tracking
- [ ] Test error handling

**Acceptance Criteria:**
- Reliable push delivery
- Rate limits enforced
- Delivery status tracked
- Failed messages returned to queue

### 2.2.2 Outbox Processor
**File:** `cmd/outbox/main_lift.go`

- [ ] Create federation handler
  ```go
  type OutboxProcessor struct {
      federation  *federation.Service
      signer      *activitypub.Signer
      store       storage.Storage
      httpClient  *http.Client
  }
  
  func (op *OutboxProcessor) HandleSQS(ctx *lift.Context, event events.SQSEvent) error {
      for _, record := range event.Records {
          var activity activitypub.Activity
          if err := json.Unmarshal([]byte(record.Body), &activity); err != nil {
              op.logger.Error("invalid activity",
                  zap.String("message_id", record.MessageId),
                  zap.Error(err),
              )
              continue
          }
          
          if err := op.deliverActivity(ctx, activity); err != nil {
              // Check if permanent failure
              if isPermanentError(err) {
                  op.logger.Error("permanent delivery failure",
                      zap.String("activity_id", activity.ID),
                      zap.Error(err),
                  )
                  continue
              }
              
              // Temporary failure - return to queue
              return err
          }
      }
      
      return nil
  }
  ```

- [ ] Implement signature verification middleware
  ```go
  func (op *OutboxProcessor) verifySignature(req *http.Request) error {
      signature := req.Header.Get("Signature")
      if signature == "" {
          return errors.New("missing signature")
      }
      
      return op.signer.Verify(req, signature)
  }
  ```

- [ ] Add federation retry logic
  ```go
  func (op *OutboxProcessor) deliverActivity(ctx context.Context, activity activitypub.Activity) error {
      // Get recipient inbox URLs
      inboxes, err := op.federation.ResolveInboxes(ctx, activity)
      if err != nil {
          return err
      }
      
      // Deliver to each inbox
      var wg sync.WaitGroup
      errors := make([]error, 0)
      var errorMutex sync.Mutex
      
      for _, inbox := range inboxes {
          wg.Add(1)
          go func(inboxURL string) {
              defer wg.Done()
              
              err := retry.Do(func() error {
                  return op.deliverToInbox(ctx, activity, inboxURL)
              }, retry.Attempts(3), retry.Delay(time.Second))
              
              if err != nil {
                  errorMutex.Lock()
                  errors = append(errors, err)
                  errorMutex.Unlock()
              }
          }(inbox)
      }
      
      wg.Wait()
      
      if len(errors) > 0 {
          return fmt.Errorf("delivery failed to %d inboxes", len(errors))
      }
      
      return nil
  }
  ```

**Testing Requirements:**
- [ ] Test signature generation/verification
- [ ] Test inbox resolution
- [ ] Test retry logic
- [ ] Test concurrent delivery

**Acceptance Criteria:**
- Activities delivered to all recipients
- Signatures properly generated
- Retry logic works correctly
- Failed deliveries tracked

## 2.3 HTTP Event Handlers

### 2.3.1 Auth Service Migration
**File:** `cmd/auth/main_lift.go`

- [ ] Create Lift auth handler
  ```go
  type AuthHandler struct {
      authService *auth.Service
      store       storage.Storage
      oauth       *oauth.Manager
      webauthn    *webauthn.Authenticator
  }
  
  func NewAuthHandler() (*AuthHandler, error) {
      // Initialize services
      return &AuthHandler{
          authService: auth.NewService(),
          store:       store,
          oauth:       oauth.NewManager(),
          webauthn:    webauthn.NewAuthenticator(),
      }, nil
  }
  ```

- [ ] Implement OAuth endpoints
  ```go
  func (ah *AuthHandler) RegisterRoutes(app *lift.App) {
      // OAuth endpoints
      app.GET("/oauth/authorize", ah.handleAuthorize)
      app.POST("/oauth/token", ah.handleToken)
      app.POST("/oauth/revoke", ah.handleRevoke)
      
      // WebAuthn endpoints
      app.POST("/webauthn/register/begin", ah.handleWebAuthnRegisterBegin)
      app.POST("/webauthn/register/finish", ah.handleWebAuthnRegisterFinish)
      app.POST("/webauthn/login/begin", ah.handleWebAuthnLoginBegin)
      app.POST("/webauthn/login/finish", ah.handleWebAuthnLoginFinish)
  }
  ```

- [ ] Add session management
  ```go
  func (ah *AuthHandler) createSession(ctx *lift.Context, userID string) error {
      session := &Session{
          ID:        uuid.New().String(),
          UserID:    userID,
          CreatedAt: time.Now(),
          ExpiresAt: time.Now().Add(24 * time.Hour),
      }
      
      // Store in DynamoDB
      if err := ah.store.SaveSession(ctx.Request().Context(), session); err != nil {
          return err
      }
      
      // Set cookie
      ctx.SetCookie(&http.Cookie{
          Name:     "session_id",
          Value:    session.ID,
          Expires:  session.ExpiresAt,
          HttpOnly: true,
          Secure:   true,
          SameSite: http.SameSiteStrictMode,
      })
      
      return nil
  }
  ```

- [ ] Implement PKCE flow
  ```go
  func (ah *AuthHandler) handleAuthorize(ctx *lift.Context) error {
      // Validate PKCE parameters
      codeChallenge := ctx.Query("code_challenge")
      codeChallengeMethod := ctx.Query("code_challenge_method")
      
      if codeChallenge == "" {
          return lift.NewError(400, "code_challenge required for PKCE")
      }
      
      if codeChallengeMethod != "S256" {
          return lift.NewError(400, "only S256 challenge method supported")
      }
      
      // Generate authorization code
      code := generateAuthCode()
      
      // Store PKCE challenge with code
      if err := ah.store.SaveAuthCode(ctx.Request().Context(), AuthCode{
          Code:         code,
          Challenge:    codeChallenge,
          Method:       codeChallengeMethod,
          ClientID:     ctx.Query("client_id"),
          RedirectURI:  ctx.Query("redirect_uri"),
          Scope:        ctx.Query("scope"),
          ExpiresAt:    time.Now().Add(10 * time.Minute),
      }); err != nil {
          return err
      }
      
      // Redirect with code
      redirectURL := fmt.Sprintf("%s?code=%s&state=%s",
          ctx.Query("redirect_uri"),
          code,
          ctx.Query("state"),
      )
      
      return ctx.Redirect(http.StatusFound, redirectURL)
  }
  ```

**Testing Requirements:**
- [ ] Test OAuth flow end-to-end
- [ ] Test WebAuthn registration/login
- [ ] Test PKCE validation
- [ ] Test session management

**Acceptance Criteria:**
- OAuth 2.0 fully compliant
- WebAuthn works across browsers
- Sessions properly managed
- PKCE enforced

### 2.3.2 Inbox Handler Migration  
**File:** `cmd/inbox/main_lift.go`

- [ ] Create ActivityPub inbox handler
  ```go
  type InboxHandler struct {
      validator   *activitypub.Validator
      processor   *activitypub.Processor
      store       storage.Storage
      federation  *federation.Service
  }
  
  func (ih *InboxHandler) Handle(ctx *lift.Context) error {
      // Verify signature
      if err := ih.verifyHTTPSignature(ctx.Request()); err != nil {
          return lift.NewError(401, "invalid signature")
      }
      
      // Parse activity
      var activity activitypub.Activity
      if err := ctx.Bind(&activity); err != nil {
          return lift.NewError(400, "invalid activity")
      }
      
      // Validate activity
      if err := ih.validator.Validate(activity); err != nil {
          return lift.NewError(400, err.Error())
      }
      
      // Process asynchronously
      if err := ih.queueForProcessing(ctx.Request().Context(), activity); err != nil {
          return lift.NewError(500, "failed to queue activity")
      }
      
      // Return 202 Accepted
      return ctx.NoContent(http.StatusAccepted)
  }
  ```

- [ ] Implement signature verification
  ```go
  func (ih *InboxHandler) verifyHTTPSignature(req *http.Request) error {
      sig := req.Header.Get("Signature")
      if sig == "" {
          return errors.New("missing signature header")
      }
      
      // Parse signature
      params, err := parseSignature(sig)
      if err != nil {
          return err
      }
      
      // Fetch actor's public key
      actor, err := ih.federation.FetchActor(req.Context(), params.KeyID)
      if err != nil {
          return err
      }
      
      // Verify signature
      return verifySignature(req, actor.PublicKey, params)
  }
  ```

- [ ] Add rate limiting per instance
  ```go
  func (ih *InboxHandler) instanceRateLimit() lift.Middleware {
      limiter := rate.NewLimiter(rate.Every(time.Second), 100)
      limiters := &sync.Map{}
      
      return func(next lift.HandlerFunc) lift.HandlerFunc {
          return func(ctx *lift.Context) error {
              // Extract instance from signature
              instance := extractInstance(ctx.Request())
              
              // Get or create limiter for instance
              val, _ := limiters.LoadOrStore(instance, rate.NewLimiter(rate.Every(time.Second), 10))
              instanceLimiter := val.(*rate.Limiter)
              
              if !instanceLimiter.Allow() {
                  return lift.NewError(429, "rate limit exceeded")
              }
              
              return next(ctx)
          }
      }
  }
  ```

**Testing Requirements:**
- [ ] Test signature verification
- [ ] Test activity validation
- [ ] Test rate limiting
- [ ] Test various activity types

**Acceptance Criteria:**
- Accepts valid ActivityPub activities
- Rejects invalid signatures
- Rate limits work per instance
- Activities queued for processing

## Implementation Strategy

### Migration Order
1. Start with activity-processor (most critical)
2. Migrate federation services next
3. Auth services last (most complex)

### Testing Strategy
1. Run new handlers alongside old ones
2. Compare outputs for consistency
3. Gradually shift traffic using weights
4. Monitor metrics during transition

### Rollback Plan
1. Keep old handlers available
2. Use environment variables to switch
3. Monitor error rates closely
4. Quick revert if issues detected

### Performance Monitoring
- Track cold start times
- Monitor memory usage
- Watch for timeout issues
- Compare costs before/after

## Common Patterns

### Error Handling Pattern
```go
func handleError(ctx *lift.Context, err error) error {
    switch e := err.(type) {
    case ValidationError:
        return lift.NewError(400, e.Error())
    case NotFoundError:
        return lift.NewError(404, "resource not found")
    case AuthError:
        return lift.NewError(401, "unauthorized")
    default:
        return lift.NewError(500, "internal error")
    }
}
```

### Metric Collection Pattern
```go
defer func(start time.Time) {
    duration := time.Since(start)
    metrics.RecordDuration("handler.duration", duration,
        metrics.Tag("handler", handlerName),
        metrics.Tag("status", status),
    )
}(time.Now())
```

### Context Propagation Pattern
```go
func propagateContext(liftCtx *lift.Context) context.Context {
    ctx := liftCtx.Request().Context()
    
    // Add request ID
    if requestID, ok := liftCtx.Get("requestID").(string); ok {
        ctx = context.WithValue(ctx, "requestID", requestID)
    }
    
    // Add user ID
    if userID, ok := liftCtx.Get("userID").(string); ok {
        ctx = context.WithValue(ctx, "userID", userID)
    }
    
    return ctx
}
```

## Success Metrics
- [ ] All event handlers migrated to Lift
- [ ] No increase in processing latency
- [ ] Error rates remain stable
- [ ] Cost per invocation unchanged
- [ ] All tests passing
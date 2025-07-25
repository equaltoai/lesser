# Phase 3: Complete DynamORM Models - Detailed Implementation Checklist

## 3.1 Missing Models Implementation

### 3.1.1 ProviderAccount Model
**File:** `pkg/storage/dynamorm/models/provider_account.go`

- [ ] Create ProviderAccount model
  ```go
  package models
  
  import (
      "time"
      "github.com/pay-theory/dynamorm"
  )
  
  type ProviderAccount struct {
      dynamorm.Model
      
      // Composite key: USER#{userID}
      UserID       string `dynamodbav:"pk" dynamorm:"hash_key"`
      // Sort key: PROVIDER#{provider}#{providerID}
      ProviderKey  string `dynamodbav:"sk" dynamorm:"range_key"`
      
      // Provider details
      Provider     string    `dynamodbav:"provider" dynamorm:"index:gsi1,hash_key"`
      ProviderID   string    `dynamodbav:"provider_id" dynamorm:"index:gsi1,range_key"`
      
      // OAuth tokens
      AccessToken  string    `dynamodbav:"access_token,omitempty" dynamorm:"encrypted"`
      RefreshToken string    `dynamodbav:"refresh_token,omitempty" dynamorm:"encrypted"`
      TokenExpiry  time.Time `dynamodbav:"token_expiry,omitempty"`
      
      // Profile data
      Email        string                 `dynamodbav:"email,omitempty"`
      Username     string                 `dynamodbav:"username,omitempty"`
      DisplayName  string                 `dynamodbav:"display_name,omitempty"`
      AvatarURL    string                 `dynamodbav:"avatar_url,omitempty"`
      ProfileData  map[string]any `dynamodbav:"profile_data,omitempty"`
      
      // Metadata
      CreatedAt    time.Time `dynamodbav:"created_at"`
      UpdatedAt    time.Time `dynamodbav:"updated_at"`
      LastUsedAt   time.Time `dynamodbav:"last_used_at,omitempty"`
      
      // Status
      IsActive     bool      `dynamodbav:"is_active"`
      IsPrimary    bool      `dynamodbav:"is_primary"`
  }
  
  // Model interface implementation
  func (p *ProviderAccount) TableName() string {
      return os.Getenv("DYNAMODB_TABLE")
  }
  
  func (p *ProviderAccount) GetHashKey() string {
      return fmt.Sprintf("USER#%s", p.UserID)
  }
  
  func (p *ProviderAccount) GetRangeKey() string {
      return fmt.Sprintf("PROVIDER#%s#%s", p.Provider, p.ProviderID)
  }
  ```

- [ ] Implement model hooks
  ```go
  func (p *ProviderAccount) BeforeCreate() error {
      if p.UserID == "" || p.Provider == "" || p.ProviderID == "" {
          return errors.New("UserID, Provider, and ProviderID are required")
      }
      
      p.CreatedAt = time.Now()
      p.UpdatedAt = time.Now()
      p.IsActive = true
      
      // Generate composite keys
      p.UserID = fmt.Sprintf("USER#%s", p.UserID)
      p.ProviderKey = fmt.Sprintf("PROVIDER#%s#%s", p.Provider, p.ProviderID)
      
      return nil
  }
  
  func (p *ProviderAccount) BeforeUpdate() error {
      p.UpdatedAt = time.Now()
      return nil
  }
  
  func (p *ProviderAccount) AfterFind() error {
      // Strip key prefixes
      p.UserID = strings.TrimPrefix(p.UserID, "USER#")
      return nil
  }
  ```

- [ ] Add validation methods
  ```go
  func (p *ProviderAccount) Validate() error {
      if !isValidProvider(p.Provider) {
          return fmt.Errorf("invalid provider: %s", p.Provider)
      }
      
      if p.AccessToken == "" {
          return errors.New("access token is required")
      }
      
      if p.TokenExpiry.Before(time.Now()) {
          return errors.New("token has expired")
      }
      
      return nil
  }
  
  func (p *ProviderAccount) IsTokenExpired() bool {
      return p.TokenExpiry.Before(time.Now())
  }
  
  func (p *ProviderAccount) NeedsRefresh() bool {
      // Refresh if token expires in next 5 minutes
      return p.TokenExpiry.Before(time.Now().Add(5 * time.Minute))
  }
  ```

**Testing Requirements:**
- [ ] Test model creation with all fields
- [ ] Test key generation
- [ ] Test validation rules
- [ ] Test token encryption/decryption
- [ ] Test hooks execution

**Acceptance Criteria:**
- Composite keys properly generated
- Tokens encrypted at rest
- Validation prevents invalid data
- Hooks maintain data integrity

### 3.1.2 Session Model
**File:** `pkg/storage/dynamorm/models/session.go`

- [ ] Create Session model
  ```go
  type Session struct {
      dynamorm.Model
      
      // Primary key: SESSION#{sessionID}
      SessionID    string `dynamodbav:"pk" dynamorm:"hash_key"`
      // No sort key for sessions
      
      // User association
      UserID       string    `dynamodbav:"user_id" dynamorm:"index:gsi2,hash_key"`
      CreatedAt    time.Time `dynamodbav:"created_at" dynamorm:"index:gsi2,range_key"`
      
      // Session data
      IPAddress    string                 `dynamodbav:"ip_address"`
      UserAgent    string                 `dynamodbav:"user_agent"`
      DeviceID     string                 `dynamodbav:"device_id,omitempty"`
      
      // Token data
      AccessToken  string                 `dynamodbav:"access_token" dynamorm:"encrypted"`
      RefreshToken string                 `dynamodbav:"refresh_token,omitempty" dynamorm:"encrypted"`
      Scopes       []string               `dynamodbav:"scopes"`
      
      // Metadata
      LastUsedAt   time.Time             `dynamodbav:"last_used_at"`
      ExpiresAt    time.Time             `dynamodbav:"expires_at" dynamorm:"ttl"`
      
      // Security
      IsRevoked    bool                  `dynamodbav:"is_revoked"`
      RevokedAt    *time.Time            `dynamodbav:"revoked_at,omitempty"`
      RevokeReason string                `dynamodbav:"revoke_reason,omitempty"`
      
      // Additional context
      Context      map[string]any `dynamodbav:"context,omitempty"`
  }
  
  func (s *Session) TableName() string {
      return os.Getenv("DYNAMODB_TABLE")
  }
  
  func (s *Session) GetHashKey() string {
      return fmt.Sprintf("SESSION#%s", s.SessionID)
  }
  
  func (s *Session) GetRangeKey() string {
      return "" // No range key
  }
  ```

- [ ] Implement session lifecycle methods
  ```go
  func (s *Session) BeforeCreate() error {
      if s.SessionID == "" {
          s.SessionID = generateSecureToken(32)
      }
      
      if s.AccessToken == "" {
          s.AccessToken = generateSecureToken(64)
      }
      
      s.CreatedAt = time.Now()
      s.LastUsedAt = time.Now()
      
      // Set default expiry to 24 hours
      if s.ExpiresAt.IsZero() {
          s.ExpiresAt = time.Now().Add(24 * time.Hour)
      }
      
      return nil
  }
  
  func (s *Session) Touch() {
      s.LastUsedAt = time.Now()
      // Extend expiry on activity
      if time.Until(s.ExpiresAt) < 12*time.Hour {
          s.ExpiresAt = time.Now().Add(24 * time.Hour)
      }
  }
  
  func (s *Session) Revoke(reason string) {
      s.IsRevoked = true
      now := time.Now()
      s.RevokedAt = &now
      s.RevokeReason = reason
  }
  
  func (s *Session) IsValid() bool {
      return !s.IsRevoked && s.ExpiresAt.After(time.Now())
  }
  ```

- [ ] Add security helpers
  ```go
  func generateSecureToken(length int) string {
      b := make([]byte, length)
      if _, err := rand.Read(b); err != nil {
          panic(err)
      }
      return base64.URLEncoding.EncodeToString(b)
  }
  
  func (s *Session) HasScope(scope string) bool {
      for _, s := range s.Scopes {
          if s == scope {
              return true
          }
      }
      return false
  }
  
  func (s *Session) ValidateRequest(ipAddress, userAgent string) bool {
      // Optional: Implement session fixation protection
      return s.IPAddress == ipAddress && s.UserAgent == userAgent
  }
  ```

**Testing Requirements:**
- [ ] Test session creation
- [ ] Test TTL functionality
- [ ] Test revocation
- [ ] Test token generation
- [ ] Test scope validation

**Acceptance Criteria:**
- Sessions auto-expire via TTL
- Tokens are cryptographically secure
- Revocation works immediately
- Activity extends session life

### 3.1.3 Media Model
**File:** `pkg/storage/dynamorm/models/media.go`

- [ ] Create Media model
  ```go
  type Media struct {
      dynamorm.Model
      
      // Primary key: MEDIA#{mediaID}
      MediaID      string `dynamodbav:"pk" dynamorm:"hash_key"`
      // Sort key: VERSION#{version}
      Version      string `dynamodbav:"sk" dynamorm:"range_key"`
      
      // Owner info
      UserID       string    `dynamodbav:"user_id" dynamorm:"index:gsi3,hash_key"`
      UploadedAt   time.Time `dynamodbav:"uploaded_at" dynamorm:"index:gsi3,range_key"`
      
      // Media metadata
      FileName     string                 `dynamodbav:"file_name"`
      ContentType  string                 `dynamodbav:"content_type"`
      FileSize     int64                  `dynamodbav:"file_size"`
      
      // Storage details
      S3Bucket     string                 `dynamodbav:"s3_bucket"`
      S3Key        string                 `dynamodbav:"s3_key"`
      CDNUrl       string                 `dynamodbav:"cdn_url"`
      
      // Processing status
      Status       string                 `dynamodbav:"status"` // pending, processing, ready, failed
      ProcessedAt  *time.Time             `dynamodbav:"processed_at,omitempty"`
      Error        string                 `dynamodbav:"error,omitempty"`
      
      // Media analysis
      Width        int                    `dynamodbav:"width,omitempty"`
      Height       int                    `dynamodbav:"height,omitempty"`
      Duration     int                    `dynamodbav:"duration,omitempty"` // For video/audio
      Blurhash     string                 `dynamodbav:"blurhash,omitempty"`
      
      // Variants (thumbnails, different sizes)
      Variants     map[string]MediaVariant `dynamodbav:"variants,omitempty"`
      
      // Moderation
      IsNSFW       bool                   `dynamodbav:"is_nsfw"`
      ModerationScore float64             `dynamodbav:"moderation_score"`
      Labels       []string               `dynamodbav:"labels,omitempty"`
      
      // Usage tracking
      UsageCount   int                    `dynamodbav:"usage_count"`
      LastUsedAt   *time.Time             `dynamodbav:"last_used_at,omitempty"`
      
      // Metadata
      CreatedAt    time.Time              `dynamodbav:"created_at"`
      UpdatedAt    time.Time              `dynamodbav:"updated_at"`
      ExpiresAt    *time.Time             `dynamodbav:"expires_at,omitempty" dynamorm:"ttl"`
  }
  
  type MediaVariant struct {
      S3Key       string `dynamodbav:"s3_key"`
      CDNUrl      string `dynamodbav:"cdn_url"`
      Width       int    `dynamodbav:"width"`
      Height      int    `dynamodbav:"height"`
      FileSize    int64  `dynamodbav:"file_size"`
      ContentType string `dynamodbav:"content_type"`
  }
  ```

- [ ] Implement media processing hooks
  ```go
  func (m *Media) BeforeCreate() error {
      if m.MediaID == "" {
          m.MediaID = fmt.Sprintf("MEDIA#%s", uuid.New().String())
      }
      
      if m.Version == "" {
          m.Version = "VERSION#original"
      }
      
      m.Status = "pending"
      m.CreatedAt = time.Now()
      m.UpdatedAt = time.Now()
      m.UsageCount = 0
      
      // Set expiry for unused media (30 days)
      expires := time.Now().Add(30 * 24 * time.Hour)
      m.ExpiresAt = &expires
      
      return m.Validate()
  }
  
  func (m *Media) Validate() error {
      // Check file size limits
      maxSize := int64(50 * 1024 * 1024) // 50MB
      if m.FileSize > maxSize {
          return fmt.Errorf("file size %d exceeds maximum %d", m.FileSize, maxSize)
      }
      
      // Validate content type
      if !isValidMediaType(m.ContentType) {
          return fmt.Errorf("unsupported content type: %s", m.ContentType)
      }
      
      return nil
  }
  
  func (m *Media) MarkUsed() {
      m.UsageCount++
      now := time.Now()
      m.LastUsedAt = &now
      m.ExpiresAt = nil // Remove expiry for used media
  }
  ```

- [ ] Add media variant management
  ```go
  func (m *Media) AddVariant(name string, variant MediaVariant) {
      if m.Variants == nil {
          m.Variants = make(map[string]MediaVariant)
      }
      m.Variants[name] = variant
      m.UpdatedAt = time.Now()
  }
  
  func (m *Media) GetVariant(name string) (MediaVariant, bool) {
      if m.Variants == nil {
          return MediaVariant{}, false
      }
      variant, ok := m.Variants[name]
      return variant, ok
  }
  
  func (m *Media) GetBestVariant(maxWidth, maxHeight int) MediaVariant {
      // Return the best matching variant for requested dimensions
      // Falls back to original if no variants match
  }
  ```

**Testing Requirements:**
- [ ] Test media upload flow
- [ ] Test variant management
- [ ] Test TTL for unused media
- [ ] Test moderation scoring
- [ ] Test usage tracking

**Acceptance Criteria:**
- Media properly stored with metadata
- Variants tracked correctly
- Unused media auto-expires
- Usage prevents expiration

### 3.1.4 Notification Model
**File:** `pkg/storage/dynamorm/models/notification.go`

- [ ] Create Notification model
  ```go
  type Notification struct {
      dynamorm.Model
      
      // Primary key: USER#{userID}
      UserID       string `dynamodbav:"pk" dynamorm:"hash_key"`
      // Sort key: NOTIF#{timestamp}#{notificationID}
      NotifKey     string `dynamodbav:"sk" dynamorm:"range_key"`
      
      // Notification details
      ID           string    `dynamodbav:"notification_id"`
      Type         string    `dynamodbav:"type"` // mention, reblog, favourite, follow, etc.
      CreatedAt    time.Time `dynamodbav:"created_at"`
      
      // Actor information
      ActorID      string    `dynamodbav:"actor_id"`
      ActorType    string    `dynamodbav:"actor_type"` // user, remote_actor
      
      // Target information
      TargetID     string    `dynamodbav:"target_id,omitempty"`
      TargetType   string    `dynamodbav:"target_type,omitempty"` // status, user
      
      // Content
      Title        string                 `dynamodbav:"title,omitempty"`
      Body         string                 `dynamodbav:"body,omitempty"`
      Data         map[string]any `dynamodbav:"data,omitempty"`
      
      // Status
      IsRead       bool      `dynamodbav:"is_read"`
      ReadAt       *time.Time `dynamodbav:"read_at,omitempty"`
      
      // Push notification
      PushSent     bool      `dynamodbav:"push_sent"`
      PushSentAt   *time.Time `dynamodbav:"push_sent_at,omitempty"`
      PushError    string    `dynamodbav:"push_error,omitempty"`
      
      // Grouping
      GroupKey     string    `dynamodbav:"group_key,omitempty" dynamorm:"index:gsi4,hash_key"`
      GroupCount   int       `dynamodbav:"group_count"`
      
      // Expiry
      ExpiresAt    time.Time `dynamodbav:"expires_at" dynamorm:"ttl"`
  }
  
  func (n *Notification) GetHashKey() string {
      return fmt.Sprintf("USER#%s", n.UserID)
  }
  
  func (n *Notification) GetRangeKey() string {
      timestamp := n.CreatedAt.Format("20060102150405")
      return fmt.Sprintf("NOTIF#%s#%s", timestamp, n.ID)
  }
  ```

- [ ] Implement notification grouping
  ```go
  func (n *Notification) BeforeCreate() error {
      if n.ID == "" {
          n.ID = uuid.New().String()
      }
      
      n.CreatedAt = time.Now()
      n.IsRead = false
      n.GroupCount = 1
      
      // Set expiry to 30 days
      n.ExpiresAt = time.Now().Add(30 * 24 * time.Hour)
      
      // Generate group key for similar notifications
      n.GroupKey = n.generateGroupKey()
      
      return nil
  }
  
  func (n *Notification) generateGroupKey() string {
      // Group by type, actor, and target within a time window
      window := n.CreatedAt.Truncate(time.Hour).Format("2006010215")
      return fmt.Sprintf("%s:%s:%s:%s:%s", n.UserID, n.Type, n.ActorID, n.TargetID, window)
  }
  
  func (n *Notification) MarkRead() {
      n.IsRead = true
      now := time.Now()
      n.ReadAt = &now
  }
  
  func (n *Notification) ShouldSendPush(userPrefs UserPreferences) bool {
      // Check user preferences for this notification type
      return !n.PushSent && userPrefs.ShouldNotify(n.Type)
  }
  ```

- [ ] Add notification builders
  ```go
  func NewMentionNotification(userID, actorID, statusID string) *Notification {
      return &Notification{
          UserID:     userID,
          Type:       "mention",
          ActorID:    actorID,
          TargetID:   statusID,
          TargetType: "status",
          Title:      "New mention",
      }
  }
  
  func NewFollowNotification(userID, followerID string) *Notification {
      return &Notification{
          UserID:     userID,
          Type:       "follow",
          ActorID:    followerID,
          ActorType:  "user",
          Title:      "New follower",
      }
  }
  ```

**Testing Requirements:**
- [ ] Test notification creation
- [ ] Test grouping logic
- [ ] Test read marking
- [ ] Test push decision logic
- [ ] Test TTL expiry

**Acceptance Criteria:**
- Notifications properly indexed by user
- Grouping reduces notification spam
- Push notifications respect preferences
- Old notifications auto-expire

## 3.2 Repository Enhancements

### 3.2.1 Add Transaction Support
**File:** `pkg/storage/dynamorm/repositories/transaction_helpers.go`

- [ ] Create transaction wrapper
  ```go
  package repositories
  
  type TransactionalRepository struct {
      client *dynamorm.Client
      tx     *dynamorm.Transaction
  }
  
  func (r *BaseRepository) WithTransaction() *TransactionalRepository {
      return &TransactionalRepository{
          client: r.client,
          tx:     r.client.NewTransaction(),
      }
  }
  
  func (tr *TransactionalRepository) Execute() error {
      return tr.tx.Execute()
  }
  
  func (tr *TransactionalRepository) Rollback() {
      tr.tx = tr.client.NewTransaction() // Reset transaction
  }
  ```

- [ ] Implement transactional operations
  ```go
  // Example: Follow operation requiring multiple updates
  func (r *UserRepository) FollowUserTransactional(followerID, followeeID string) error {
      tx := r.client.NewTransaction()
      
      // Create follow relationship
      follow := &Follow{
          FollowerID: followerID,
          FolloweeID: followeeID,
          CreatedAt:  time.Now(),
      }
      tx.Put(follow)
      
      // Update follower count
      tx.Update(&User{ID: followeeID}, 
          dynamorm.UpdateExpression("ADD follower_count :inc").
          WithValue(":inc", 1))
      
      // Update following count
      tx.Update(&User{ID: followerID},
          dynamorm.UpdateExpression("ADD following_count :inc").
          WithValue(":inc", 1))
      
      // Add to timeline
      timelineEntry := &Timeline{
          UserID:    followeeID,
          Type:      "follow",
          ActorID:   followerID,
          CreatedAt: time.Now(),
      }
      tx.Put(timelineEntry)
      
      return tx.Execute()
  }
  ```

- [ ] Add conditional transactions
  ```go
  func (r *StatusRepository) CreateStatusWithChecks(status *Status) error {
      tx := r.client.NewTransaction()
      
      // Check user exists and is not suspended
      tx.ConditionCheck(&User{ID: status.UserID},
          dynamorm.Condition("attribute_exists(pk) AND is_suspended = :false").
          WithValue(":false", false))
      
      // Check rate limits
      tx.ConditionCheck(&RateLimit{UserID: status.UserID},
          dynamorm.Condition("post_count < :limit").
          WithValue(":limit", 300))
      
      // Create status
      tx.Put(status)
      
      // Update rate limit
      tx.Update(&RateLimit{UserID: status.UserID},
          dynamorm.UpdateExpression("ADD post_count :inc").
          WithValue(":inc", 1))
      
      return tx.Execute()
  }
  ```

**Testing Requirements:**
- [ ] Test successful transactions
- [ ] Test transaction rollback on failure
- [ ] Test conditional checks
- [ ] Test concurrent transactions

**Acceptance Criteria:**
- All-or-nothing transaction execution
- Proper error handling
- No partial updates on failure
- Performance acceptable

### 3.2.2 Batch Operations
**File:** `pkg/storage/dynamorm/repositories/batch_operations.go`

- [ ] Create batch writer
  ```go
  type BatchOperations struct {
      client    *dynamorm.Client
      batchSize int
  }
  
  func NewBatchOperations(client *dynamorm.Client) *BatchOperations {
      return &BatchOperations{
          client:    client,
          batchSize: 25, // DynamoDB limit
      }
  }
  
  func (b *BatchOperations) BatchWrite(items []any) error {
      for i := 0; i < len(items); i += b.batchSize {
          end := i + b.batchSize
          if end > len(items) {
              end = len(items)
          }
          
          batch := b.client.NewBatchWrite()
          for _, item := range items[i:end] {
              batch.Put(item)
          }
          
          if err := batch.Execute(); err != nil {
              return fmt.Errorf("batch write failed at index %d: %w", i, err)
          }
      }
      
      return nil
  }
  ```

- [ ] Implement timeline batch updates
  ```go
  func (r *TimelineRepository) BatchInsertTimelineEntries(userIDs []string, status *Status) error {
      entries := make([]any, 0, len(userIDs))
      
      for _, userID := range userIDs {
          entry := &Timeline{
              UserID:    userID,
              StatusID:  status.ID,
              AuthorID:  status.UserID,
              CreatedAt: status.CreatedAt,
              Type:      "home",
          }
          entries = append(entries, entry)
      }
      
      // Use parallel batch writes for large follower lists
      if len(entries) > 100 {
          return r.parallelBatchWrite(entries, 4) // 4 workers
      }
      
      return r.batchOps.BatchWrite(entries)
  }
  
  func (r *TimelineRepository) parallelBatchWrite(entries []any, workers int) error {
      ch := make(chan []any, workers)
      errors := make(chan error, workers)
      
      // Start workers
      var wg sync.WaitGroup
      for i := 0; i < workers; i++ {
          wg.Add(1)
          go func() {
              defer wg.Done()
              for batch := range ch {
                  if err := r.batchOps.BatchWrite(batch); err != nil {
                      errors <- err
                      return
                  }
              }
          }()
      }
      
      // Distribute work
      chunkSize := len(entries) / workers
      for i := 0; i < len(entries); i += chunkSize {
          end := i + chunkSize
          if end > len(entries) {
              end = len(entries)
          }
          ch <- entries[i:end]
      }
      close(ch)
      
      // Wait and check for errors
      wg.Wait()
      close(errors)
      
      for err := range errors {
          if err != nil {
              return err
          }
      }
      
      return nil
  }
  ```

**Testing Requirements:**
- [ ] Test batch size limits
- [ ] Test parallel batch operations
- [ ] Test error handling
- [ ] Test performance gains

**Acceptance Criteria:**
- Respects DynamoDB limits
- Parallel execution for large batches
- Proper error aggregation
- Significant performance improvement

### 3.2.3 Cost Tracking Integration
**File:** `pkg/storage/dynamorm/repositories/cost_aware_repository.go`

- [ ] Create cost-aware base repository
  ```go
  type CostAwareRepository struct {
      *BaseRepository
      costTracker *CostTracker
  }
  
  func (r *CostAwareRepository) trackCost(ctx context.Context, operation string, fn func() error) error {
      start := time.Now()
      startCapacity := r.client.GetConsumedCapacity()
      
      err := fn()
      
      duration := time.Since(start)
      endCapacity := r.client.GetConsumedCapacity()
      
      cost := calculateCost(endCapacity - startCapacity)
      
      // Add to context
      if tracker, ok := ctx.Value("costTracker").(*CostTracker); ok {
          tracker.AddOperation(Operation{
              Name:     operation,
              Table:    r.TableName(),
              Duration: duration,
              RCU:      endCapacity.ReadCapacityUnits - startCapacity.ReadCapacityUnits,
              WCU:      endCapacity.WriteCapacityUnits - startCapacity.WriteCapacityUnits,
              Cost:     cost,
          })
      }
      
      // Log if cost exceeds threshold
      if cost > 0.001 { // $0.001
          r.logger.Warn("high cost operation",
              zap.String("operation", operation),
              zap.Float64("cost", cost),
              zap.Duration("duration", duration),
          )
      }
      
      return err
  }
  ```

- [ ] Update all repository methods
  ```go
  func (r *UserRepository) GetByID(ctx context.Context, userID string) (*User, error) {
      var user *User
      err := r.trackCost(ctx, "GetUserByID", func() error {
          result, err := r.client.Get(&User{ID: userID}).Execute()
          if err != nil {
              return err
          }
          user = result.(*User)
          return nil
      })
      return user, err
  }
  
  func (r *UserRepository) QueryFollowers(ctx context.Context, userID string, limit int) ([]*User, error) {
      var users []*User
      err := r.trackCost(ctx, "QueryFollowers", func() error {
          results, err := r.client.Query().
              Index("gsi-followers").
              HashKey("user_id", userID).
              Limit(limit).
              Execute()
          
          if err != nil {
              return err
          }
          
          for _, item := range results {
              users = append(users, item.(*User))
          }
          return nil
      })
      return users, err
  }
  ```

**Testing Requirements:**
- [ ] Test cost calculation accuracy
- [ ] Test context integration
- [ ] Test threshold alerts
- [ ] Test aggregation

**Acceptance Criteria:**
- All operations track costs
- Costs aggregated per request
- High-cost operations logged
- No performance degradation

### 3.2.4 Repository Testing Utilities
**File:** `pkg/storage/dynamorm/repositories/testing/helpers.go`

- [ ] Create test fixtures
  ```go
  package testing
  
  type FixtureBuilder struct {
      users     []*User
      statuses  []*Status
      follows   []*Follow
  }
  
  func NewFixtureBuilder() *FixtureBuilder {
      return &FixtureBuilder{}
  }
  
  func (fb *FixtureBuilder) WithUser(id, username string) *FixtureBuilder {
      fb.users = append(fb.users, &User{
          ID:       id,
          Username: username,
          Email:    fmt.Sprintf("%s@test.com", username),
      })
      return fb
  }
  
  func (fb *FixtureBuilder) WithStatus(id, userID, content string) *FixtureBuilder {
      fb.statuses = append(fb.statuses, &Status{
          ID:      id,
          UserID:  userID,
          Content: content,
      })
      return fb
  }
  
  func (fb *FixtureBuilder) Build() *Fixtures {
      return &Fixtures{
          Users:    fb.users,
          Statuses: fb.statuses,
          Follows:  fb.follows,
      }
  }
  ```

- [ ] Create repository mocks
  ```go
  type MockUserRepository struct {
      mock.Mock
      users map[string]*User
  }
  
  func NewMockUserRepository() *MockUserRepository {
      return &MockUserRepository{
          users: make(map[string]*User),
      }
  }
  
  func (m *MockUserRepository) GetByID(ctx context.Context, id string) (*User, error) {
      args := m.Called(ctx, id)
      if user, ok := m.users[id]; ok {
          return user, args.Error(1)
      }
      return nil, errors.New("user not found")
  }
  
  func (m *MockUserRepository) Save(ctx context.Context, user *User) error {
      args := m.Called(ctx, user)
      if args.Error(0) == nil {
          m.users[user.ID] = user
      }
      return args.Error(0)
  }
  ```

- [ ] Create integration test helpers
  ```go
  type TestDB struct {
      client  *dynamorm.Client
      cleaner *Cleaner
  }
  
  func SetupTestDB(t *testing.T) *TestDB {
      client := dynamorm.NewClient(dynamorm.Config{
          Table:  os.Getenv("TEST_DYNAMODB_TABLE"),
          Region: "us-east-1",
      })
      
      return &TestDB{
          client:  client,
          cleaner: NewCleaner(client),
      }
  }
  
  func (tdb *TestDB) Cleanup(t *testing.T) {
      if err := tdb.cleaner.CleanAll(); err != nil {
          t.Errorf("failed to clean test data: %v", err)
      }
  }
  
  type Cleaner struct {
      client    *dynamorm.Client
      toClean   []any
  }
  
  func (c *Cleaner) Track(item any) {
      c.toClean = append(c.toClean, item)
  }
  
  func (c *Cleaner) CleanAll() error {
      for _, item := range c.toClean {
          if err := c.client.Delete(item).Execute(); err != nil {
              return err
          }
      }
      return nil
  }
  ```

**Testing Requirements:**
- [ ] Test fixture generation
- [ ] Test mock behavior
- [ ] Test cleanup functionality
- [ ] Test isolation between tests

**Acceptance Criteria:**
- Easy fixture creation
- Reliable mocks
- Automatic cleanup
- Test isolation guaranteed

## 3.3 Advanced DynamORM Features

### 3.3.1 Model Hooks Implementation
**File:** `pkg/storage/dynamorm/hooks/lifecycle.go`

- [ ] Create hook registry
  ```go
  package hooks
  
  type HookType string
  
  const (
      BeforeCreate HookType = "before_create"
      AfterCreate  HookType = "after_create"
      BeforeUpdate HookType = "before_update"
      AfterUpdate  HookType = "after_update"
      BeforeDelete HookType = "before_delete"
      AfterDelete  HookType = "after_delete"
      AfterFind    HookType = "after_find"
  )
  
  type HookRegistry struct {
      hooks map[reflect.Type]map[HookType][]HookFunc
      mu    sync.RWMutex
  }
  
  type HookFunc func(ctx context.Context, model any) error
  
  func (hr *HookRegistry) Register(modelType reflect.Type, hookType HookType, fn HookFunc) {
      hr.mu.Lock()
      defer hr.mu.Unlock()
      
      if hr.hooks[modelType] == nil {
          hr.hooks[modelType] = make(map[HookType][]HookFunc)
      }
      
      hr.hooks[modelType][hookType] = append(hr.hooks[modelType][hookType], fn)
  }
  
  func (hr *HookRegistry) Execute(ctx context.Context, model any, hookType HookType) error {
      hr.mu.RLock()
      defer hr.mu.RUnlock()
      
      modelType := reflect.TypeOf(model)
      if hooks, ok := hr.hooks[modelType][hookType]; ok {
          for _, hook := range hooks {
              if err := hook(ctx, model); err != nil {
                  return fmt.Errorf("hook %s failed: %w", hookType, err)
              }
          }
      }
      
      return nil
  }
  ```

- [ ] Implement common hooks
  ```go
  // Audit trail hook
  func AuditHook(ctx context.Context, model any) error {
      audit := &AuditLog{
          EntityType: reflect.TypeOf(model).Name(),
          EntityID:   getEntityID(model),
          Action:     getAction(ctx),
          UserID:     getUserID(ctx),
          Timestamp:  time.Now(),
          Changes:    getChanges(ctx, model),
      }
      
      return saveAuditLog(ctx, audit)
  }
  
  // Validation hook
  func ValidationHook(ctx context.Context, model any) error {
      if validator, ok := model.(Validator); ok {
          return validator.Validate()
      }
      return nil
  }
  
  // Notification hook
  func NotificationHook(ctx context.Context, model any) error {
      switch m := model.(type) {
      case *Status:
          return notifyMentions(ctx, m)
      case *Follow:
          return notifyFollowee(ctx, m)
      }
      return nil
  }
  ```

**Testing Requirements:**
- [ ] Test hook registration
- [ ] Test hook execution order
- [ ] Test error propagation
- [ ] Test hook context

**Acceptance Criteria:**
- Hooks execute in order
- Errors stop execution
- Context properly passed
- No race conditions

### 3.3.2 Validation Rules
**File:** `pkg/storage/dynamorm/validation/rules.go`

- [ ] Create validation framework
  ```go
  package validation
  
  type Rule interface {
      Validate(value any) error
      Message() string
  }
  
  type Validator struct {
      rules map[string][]Rule
  }
  
  func (v *Validator) AddRule(field string, rule Rule) {
      v.rules[field] = append(v.rules[field], rule)
  }
  
  func (v *Validator) Validate(model any) error {
      modelValue := reflect.ValueOf(model).Elem()
      modelType := modelValue.Type()
      
      var errors []error
      
      for i := 0; i < modelType.NumField(); i++ {
          field := modelType.Field(i)
          fieldValue := modelValue.Field(i)
          
          if rules, ok := v.rules[field.Name]; ok {
              for _, rule := range rules {
                  if err := rule.Validate(fieldValue.Interface()); err != nil {
                      errors = append(errors, fmt.Errorf("%s: %w", field.Name, err))
                  }
              }
          }
      }
      
      if len(errors) > 0 {
          return ValidationError{Errors: errors}
      }
      
      return nil
  }
  ```

- [ ] Implement common validation rules
  ```go
  // Required rule
  type RequiredRule struct{}
  
  func (r RequiredRule) Validate(value any) error {
      if isZero(value) {
          return errors.New(r.Message())
      }
      return nil
  }
  
  func (r RequiredRule) Message() string {
      return "field is required"
  }
  
  // String length rule
  type LengthRule struct {
      Min int
      Max int
  }
  
  func (r LengthRule) Validate(value any) error {
      str, ok := value.(string)
      if !ok {
          return errors.New("value must be a string")
      }
      
      length := len(str)
      if r.Min > 0 && length < r.Min {
          return fmt.Errorf("must be at least %d characters", r.Min)
      }
      if r.Max > 0 && length > r.Max {
          return fmt.Errorf("must be at most %d characters", r.Max)
      }
      
      return nil
  }
  
  // Pattern rule
  type PatternRule struct {
      Pattern *regexp.Regexp
      Message string
  }
  
  func (r PatternRule) Validate(value any) error {
      str, ok := value.(string)
      if !ok {
          return errors.New("value must be a string")
      }
      
      if !r.Pattern.MatchString(str) {
          return errors.New(r.Message)
      }
      
      return nil
  }
  ```

- [ ] Add struct tag support
  ```go
  func ValidateWithTags(model any) error {
      modelType := reflect.TypeOf(model).Elem()
      modelValue := reflect.ValueOf(model).Elem()
      
      for i := 0; i < modelType.NumField(); i++ {
          field := modelType.Field(i)
          fieldValue := modelValue.Field(i)
          
          if tag := field.Tag.Get("validate"); tag != "" {
              rules := parseValidationTag(tag)
              for _, rule := range rules {
                  if err := rule.Validate(fieldValue.Interface()); err != nil {
                      return fmt.Errorf("%s: %w", field.Name, err)
                  }
              }
          }
      }
      
      return nil
  }
  
  // Example usage:
  type User struct {
      Username string `validate:"required,min=3,max=30,pattern=^[a-zA-Z0-9_]+$"`
      Email    string `validate:"required,email"`
      Age      int    `validate:"min=13,max=120"`
  }
  ```

**Testing Requirements:**
- [ ] Test each validation rule
- [ ] Test rule combination
- [ ] Test struct tag parsing
- [ ] Test error messages

**Acceptance Criteria:**
- All rules work correctly
- Clear error messages
- Tag parsing works
- Performance acceptable

### 3.3.3 Custom Marshalers
**File:** `pkg/storage/dynamorm/marshalers/custom.go`

- [ ] Create custom marshaler interface
  ```go
  package marshalers
  
  type Marshaler interface {
      MarshalDynamoDB() (map[string]*dynamodb.AttributeValue, error)
  }
  
  type Unmarshaler interface {
      UnmarshalDynamoDB(map[string]*dynamodb.AttributeValue) error
  }
  
  // Example: Custom time marshaler with precision
  type PreciseTime struct {
      time.Time
      Precision time.Duration
  }
  
  func (pt PreciseTime) MarshalDynamoDB() (map[string]*dynamodb.AttributeValue, error) {
      truncated := pt.Truncate(pt.Precision)
      return map[string]*dynamodb.AttributeValue{
          "S": {S: aws.String(truncated.Format(time.RFC3339Nano))},
      }, nil
  }
  
  func (pt *PreciseTime) UnmarshalDynamoDB(av map[string]*dynamodb.AttributeValue) error {
      if av["S"] == nil || av["S"].S == nil {
          return errors.New("invalid time format")
      }
      
      t, err := time.Parse(time.RFC3339Nano, *av["S"].S)
      if err != nil {
          return err
      }
      
      pt.Time = t
      return nil
  }
  ```

- [ ] Implement complex type marshalers
  ```go
  // Money type with currency
  type Money struct {
      Amount   decimal.Decimal
      Currency string
  }
  
  func (m Money) MarshalDynamoDB() (map[string]*dynamodb.AttributeValue, error) {
      return map[string]*dynamodb.AttributeValue{
          "M": {
              M: map[string]*dynamodb.AttributeValue{
                  "amount":   {N: aws.String(m.Amount.String())},
                  "currency": {S: aws.String(m.Currency)},
              },
          },
      }, nil
  }
  
  // Encrypted field marshaler
  type EncryptedString struct {
      Value     string
      encryptor Encryptor
  }
  
  func (es EncryptedString) MarshalDynamoDB() (map[string]*dynamodb.AttributeValue, error) {
      encrypted, err := es.encryptor.Encrypt([]byte(es.Value))
      if err != nil {
          return nil, err
      }
      
      return map[string]*dynamodb.AttributeValue{
          "B": {B: encrypted},
      }, nil
  }
  ```

**Testing Requirements:**
- [ ] Test marshaling/unmarshaling
- [ ] Test error cases
- [ ] Test nil handling
- [ ] Test round-trip accuracy

**Acceptance Criteria:**
- Custom types properly stored
- No data loss on round-trip
- Errors handled gracefully
- Performance acceptable

### 3.3.4 Soft Delete Pattern
**File:** `pkg/storage/dynamorm/patterns/soft_delete.go`

- [ ] Create soft delete interface
  ```go
  package patterns
  
  type SoftDeletable interface {
      SoftDelete()
      Restore()
      IsDeleted() bool
      GetDeletedAt() *time.Time
  }
  
  type SoftDeleteModel struct {
      DeletedAt *time.Time `dynamodbav:"deleted_at,omitempty"`
  }
  
  func (m *SoftDeleteModel) SoftDelete() {
      now := time.Now()
      m.DeletedAt = &now
  }
  
  func (m *SoftDeleteModel) Restore() {
      m.DeletedAt = nil
  }
  
  func (m *SoftDeleteModel) IsDeleted() bool {
      return m.DeletedAt != nil
  }
  
  func (m *SoftDeleteModel) GetDeletedAt() *time.Time {
      return m.DeletedAt
  }
  ```

- [ ] Implement soft delete repository
  ```go
  type SoftDeleteRepository struct {
      *BaseRepository
  }
  
  func (r *SoftDeleteRepository) Delete(ctx context.Context, model SoftDeletable) error {
      model.SoftDelete()
      return r.Save(ctx, model)
  }
  
  func (r *SoftDeleteRepository) HardDelete(ctx context.Context, model any) error {
      return r.BaseRepository.Delete(ctx, model)
  }
  
  func (r *SoftDeleteRepository) FindActive(ctx context.Context, query Query) ([]any, error) {
      // Add filter to exclude soft-deleted items
      query.Filter("deleted_at", "=", nil)
      return r.BaseRepository.Find(ctx, query)
  }
  
  func (r *SoftDeleteRepository) FindDeleted(ctx context.Context, query Query) ([]any, error) {
      // Add filter to include only soft-deleted items
      query.Filter("deleted_at", "!=", nil)
      return r.BaseRepository.Find(ctx, query)
  }
  ```

- [ ] Add cleanup job for old soft deletes
  ```go
  func (r *SoftDeleteRepository) CleanupOldDeletes(ctx context.Context, olderThan time.Duration) error {
      cutoff := time.Now().Add(-olderThan)
      
      // Query for old soft-deleted items
      items, err := r.client.Query().
          Filter("deleted_at < :cutoff").
          WithValue(":cutoff", cutoff).
          Execute()
      
      if err != nil {
          return err
      }
      
      // Hard delete in batches
      batch := r.client.NewBatchWrite()
      for i, item := range items {
          batch.Delete(item)
          
          if (i+1)%25 == 0 || i == len(items)-1 {
              if err := batch.Execute(); err != nil {
                  return err
              }
              batch = r.client.NewBatchWrite()
          }
      }
      
      return nil
  }
  ```

**Testing Requirements:**
- [ ] Test soft delete/restore
- [ ] Test query filtering
- [ ] Test cleanup job
- [ ] Test hard delete

**Acceptance Criteria:**
- Soft delete preserves data
- Queries respect soft delete
- Cleanup removes old data
- Can still hard delete

## Success Metrics
- [ ] All models fully implemented
- [ ] Repository pattern consistent
- [ ] Advanced features working
- [ ] Test coverage >95%
- [ ] No performance regression
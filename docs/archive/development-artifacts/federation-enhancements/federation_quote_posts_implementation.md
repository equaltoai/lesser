# Quote Posts Implementation Guide for Lesser

## 🎯 Overview

Quote posts are the #1 requested feature in the Mastodon community. Lesser will implement them with safety and consent at the core.

## 🛡️ Safety Features

### 1. Consent-Based Quoting
```go
// In pkg/activitypub/types.go
type NotePreferences struct {
    Quoteable          bool   `json:"quoteable"`
    QuoteNotifications bool   `json:"quoteNotifications"`
    QuotePermission    string `json:"quotePermission"` // "everyone", "followers", "none"
}
```

### 2. Quote Withdrawal
Users can withdraw their posts from quote context:
```go
type QuoteWithdrawal struct {
    OriginalNoteID string
    QuoteNoteID    string
    Timestamp      time.Time
    Reason         string
}
```

### 3. Quote Notifications
Control who gets notified when quoted:
```go
type QuoteNotificationSettings struct {
    NotifyOnQuote      bool
    NotifyOnlyMutuals  bool
    QuietHours         []TimeRange
    BlockedQuoters     []string
}
```

## 📦 Storage Implementation

### DynamoDB Schema

**Table: Notes**
```go
// Additional attributes for quote support
type Note struct {
    // ... existing fields ...
    QuoteURL           string
    QuoteableOverride  *bool // User can override instance default
    QuoteCount         int
    LastQuotedAt       time.Time
}
```

**GSI: quotes-by-target**
```
PK: targetNoteID
SK: quoteNoteID
Attributes:
  - quoterID
  - timestamp
  - withdrawn
  - quoteType (full, partial, commentary)
```

**GSI: quotes-by-author**
```
PK: authorID
SK: timestamp
Attributes:
  - quoteNoteID
  - targetNoteID
  - engagement
```

## 🔄 Federation Protocol

### ActivityPub Extension

```json
{
  "@context": [
    "https://www.w3.org/ns/activitystreams",
    "https://lesser.social/ns/quotes"
  ],
  "type": "Note",
  "id": "https://example.com/notes/123",
  "content": "This is my commentary on the quoted post",
  "quoteUrl": "https://other.instance/@user/456",
  "_:quoteMeta": {
    "quoteable": true,
    "quoteType": "commentary",
    "quoteContext": {
      "originalAuthor": "@user@other.instance",
      "originalContent": "First 500 chars...",
      "originalMediaCount": 2
    }
  }
}
```

### Quote Activity

```json
{
  "type": "Quote",
  "actor": "https://example.com/users/alice",
  "object": "https://other.instance/@user/456",
  "result": "https://example.com/notes/123",
  "published": "2024-01-15T10:00:00Z"
}
```

## 💻 GraphQL Implementation

### Schema Changes

```graphql
extend type Note {
  quoteUrl: String
  quoteable: Boolean!
  quoteContext: QuoteContext
  quoteCount: Int!
  quotes(first: Int, after: String): QuoteConnection!
}

type QuoteContext {
  originalAuthor: Actor!
  originalNote: Note
  quoteAllowed: Boolean!
  quoteType: QuoteType!
}

enum QuoteType {
  FULL
  PARTIAL
  COMMENTARY
  REACTION
}

input CreateQuoteNoteInput {
  content: String!
  quoteUrl: String!
  quoteType: QuoteType
  visibility: Visibility
  quoteable: Boolean
  sensitive: Boolean
  spoilerText: String
  mediaIds: [ID!]
}

extend type Mutation {
  createQuoteNote(input: CreateQuoteNoteInput!): CreateNotePayload!
  withdrawFromQuotes(noteId: ID!): WithdrawQuotePayload!
  updateQuotePermissions(
    noteId: ID!
    quoteable: Boolean!
    quotePermission: QuotePermission!
  ): UpdateQuotePermissionsPayload!
}
```

### Resolver Implementation

```go
// graph/schema.resolvers.go

func (r *mutationResolver) CreateQuoteNote(ctx context.Context, input model.CreateQuoteNoteInput) (*model.CreateNotePayload, error) {
    // 1. Authenticate user
    user := auth.UserFromContext(ctx)
    if user == nil {
        return nil, errors.New("authentication required")
    }

    // 2. Fetch and validate quoted note
    quotedNote, err := r.Storage.GetNoteByURL(ctx, input.QuoteURL)
    if err != nil {
        return nil, fmt.Errorf("quoted note not found: %w", err)
    }

    // 3. Check quote permissions
    if !quotedNote.Quoteable {
        return nil, errors.New("this note cannot be quoted")
    }

    if quotedNote.QuotePermission == "followers" {
        isFollower, _ := r.Storage.IsFollowing(ctx, quotedNote.AuthorID, user.ID)
        if !isFollower {
            return nil, errors.New("only followers can quote this note")
        }
    }

    // 4. Create quote note
    note := &activitypub.Note{
        ID:        generateID(),
        Type:      "Note",
        Content:   input.Content,
        QuoteURL:  input.QuoteURL,
        AuthorID:  user.ID,
        Published: time.Now(),
        Quoteable: input.Quoteable,
    }

    // 5. Store note
    if err := r.Storage.CreateNote(ctx, note); err != nil {
        return nil, err
    }

    // 6. Update quote count
    r.Storage.IncrementQuoteCount(ctx, quotedNote.ID)

    // 7. Create Quote activity
    activity := &activitypub.Activity{
        Type:   "Quote",
        Actor:  user.URL,
        Object: quotedNote.URL,
        Result: note.URL,
    }

    // 8. Queue for federation
    r.FederationQueue.Send(ctx, activity)

    // 9. Send notifications if enabled
    if quotedNote.QuoteNotifications {
        r.NotificationService.SendQuoteNotification(ctx, quotedNote.AuthorID, note)
    }

    // 10. Track costs
    r.CostTracker.TrackDynamoWrite(2) // Note + quote relationship

    return &model.CreateNotePayload{
        Note: modelNote(note),
    }, nil
}
```

## 🔐 Privacy & Safety Controls

### User Settings

```go
type UserQuoteSettings struct {
    UserID                string
    DefaultQuoteable      bool
    DefaultQuotePermission string
    AutoWithdrawOnBlock   bool
    QuoteModeration       QuoteModerationLevel
}

type QuoteModerationLevel string

const (
    QuoteModerationNone   QuoteModerationLevel = "none"
    QuoteModerationReview QuoteModerationLevel = "review"
    QuoteModerationStrict QuoteModerationLevel = "strict"
)
```

### Instance Policies

```go
type InstanceQuotePolicy struct {
    EnableQuotePosts      bool
    RequireConsentDefault bool
    MaxQuoteDepth        int
    BlockQuotesFromNew   time.Duration // Block quotes from accounts < X days old
}
```

## 🧪 Testing Strategy

### Unit Tests

```go
func TestQuotePermissions(t *testing.T) {
    // Test various permission scenarios
    // - Public note, anyone can quote
    // - Followers-only quoting
    // - Blocked user cannot quote
    // - Withdrawn quotes handled properly
}

func TestQuoteFederation(t *testing.T) {
    // Test federation of quote activities
    // - Quote activity properly formatted
    // - Remote quotes received correctly
    // - Quote withdrawals federated
}
```

### Integration Tests

```go
func TestQuoteE2E(t *testing.T) {
    // Full quote flow
    // 1. Create quoteable note
    // 2. Quote from different user
    // 3. Verify notification sent
    // 4. Withdraw original
    // 5. Verify quote updated
}
```

## 📊 Metrics & Monitoring

### Key Metrics
- Quote creation rate
- Quote withdrawal rate
- Permission denial rate
- Federation success rate
- Notification delivery rate

### Dashboards
```go
type QuoteMetrics struct {
    TotalQuotes         int64
    QuotesPerHour      float64
    WithdrawalRate     float64
    FederationSuccess  float64
    AverageQuoteDepth  float64
}
```

## 🚀 Rollout Strategy

### Phase 1: Beta (Week 1)
- Enable for verified accounts only
- Monitor for abuse patterns
- Gather user feedback

### Phase 2: General Availability (Week 2-3)
- Enable for all users
- Default to quoteable=true
- Monitor federation compatibility

### Phase 3: Advanced Features (Week 4)
- Quote threads
- Nested quotes (with depth limit)
- Quote analytics for users

## 🎯 Success Criteria

1. **Adoption**: 20% of active users create quotes within first month
2. **Safety**: < 1% of quotes result in reports
3. **Federation**: 95% federation success rate
4. **Performance**: < 50ms additional latency
5. **User Satisfaction**: > 80% positive feedback

## 📝 Documentation

### User Documentation
- How to create quote posts
- Managing quote permissions
- Understanding quote notifications
- Withdrawing from quotes

### Developer Documentation
- ActivityPub quote extension
- API endpoints
- Federation protocol
- Best practices

## 🏁 Conclusion

Lesser's quote post implementation prioritizes:
- **User consent** and control
- **Safety** features to prevent harassment
- **Federation** compatibility
- **Performance** at scale
- **Flexibility** for future enhancements

This positions Lesser as the leader in safe, consensual quote posting in the Fediverse. 
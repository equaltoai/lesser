# Lesser Reactive Moderation Mesh Design

## Overview

The Reactive Moderation Mesh is a revolutionary consensus-based moderation system for Lesser that leverages trust graphs and weighted voting to make fair, transparent, and scalable moderation decisions.

## Architecture

### Core Components

1. **Moderation Event System** (`pkg/moderation/`)
   - Event-driven pipeline for moderation actions
   - Confidence scoring and evidence tracking
   - Full audit trail with TTL-based retention

2. **Trust Graph Engine** (`pkg/trust/`)
   - Directional trust relationships
   - Category-based trust (content, behavior, technical)
   - PageRank-like trust score propagation
   - Real-time trust updates based on outcomes

3. **Consensus Engine** (`pkg/moderation/consensus.go`)
   - Weighted review aggregation
   - Configurable consensus thresholds
   - Automatic escalation for complex cases
   - Sub-second decision making

4. **Stream Processor** (`cmd/moderation-processor/`)
   - Lambda function processing DynamoDB streams
   - Real-time reaction to moderation events
   - Automatic consensus checking on new reviews

## Data Models

### Moderation Event
```go
type ModerationEvent struct {
    ID              string      // Unique event ID
    EventType       EventType   // flagged, reviewed, appealed, etc.
    ObjectID        string      // ID of content being moderated
    ObjectType      string      // status, account, media
    Category        Category    // spam, hate_speech, etc.
    Severity        Severity    // 1-4 scale
    ConfidenceScore float64     // 0.0-1.0
    Evidence        []Evidence  // Supporting evidence
}
```

### Trust Relationship
```go
type TrustRelationship struct {
    TrusterID  string        // Who trusts
    TrusteeID  string        // Who is trusted
    Category   TrustCategory // content, behavior, technical
    Score      float64       // -1.0 to 1.0
    Confidence float64       // 0.0 to 1.0
}
```

### Consensus Decision
```go
type ModerationDecision struct {
    Action           ActionType // none, warning, silence, suspend, etc.
    ConsensusScore   float64    // Agreement percentage
    ReviewerCount    int        // Number of reviewers
    TrustWeightTotal float64    // Total trust weight
}
```

## Key Features

### 1. Event-Driven Architecture
- All moderation actions trigger DynamoDB streams
- Lambda functions react to events in real-time
- Automatic consensus checking on review threshold

### 2. Trust Score Propagation
- PageRank-inspired algorithm
- Trust decays with network distance
- Negative trust signals weighted higher
- Cached for performance (2-hour TTL)

### 3. Weighted Consensus
- Reviews weighted by reviewer trust scores
- Configurable thresholds:
  - Minimum 3 reviewers
  - Minimum 0.5 trust weight
  - 70% consensus required
  - 90% critical action threshold

### 4. Automatic Escalation
- High severity + low consensus = escalation
- Significant disagreement on critical issues
- Consensus between 80-90% triggers review

### 5. Trust Evolution
- Trust scores update based on consensus outcomes
- Agreement with consensus increases trust
- Disagreement slightly decreases trust
- Weighted by decision severity

## DynamoDB Schema

### Moderation Table
```
Primary Key:
- pk: EVENT#{objectID} | REVIEW#{eventID} | DECISION#{objectID}
- sk: TIME#{timestamp}#{id} | REVIEWER#{reviewerID}

GSI1:
- gsi1pk: ACTOR#{actorID} | ACTIVE_DECISIONS
- gsi1sk: TIME#{timestamp} | OBJECT#{objectID}

GSI2:
- gsi2pk: TYPE#{eventType}#{category}
- gsi2sk: SEVERITY#{severity}#{timestamp}

GSI3:
- gsi3pk: EVENTID#{eventID}
```

### Trust Table
```
Primary Key:
- pk: TRUST#{trusterID}#{category}
- sk: TRUSTEE#{trusteeID}

GSI1:
- gsi1pk: TRUSTED#{trusteeID}#{category}
- gsi1sk: TRUSTER#{trusterID}

GSI2:
- gsi2pk: DOMAIN#{trusteeDomain}
- gsi2sk: TRUST#{category}#{score}
```

## Integration Points

### 1. Content Flagging
```go
// Flag content for moderation
event := &ModerationEvent{
    ObjectID:        statusID,
    ObjectType:      "status",
    Category:        CategoryHateSpeech,
    Severity:        SeverityHigh,
    ConfidenceScore: 0.85,
    Evidence: []Evidence{
        {Type: "ai_detection", Score: 0.92},
        {Type: "user_report", Score: 1.0},
    },
}
modService.CreateEvent(ctx, event)
```

### 2. Review Submission
```go
// Submit a moderation review
review := &Review{
    EventID:    eventID,
    ReviewerID: userID,
    Action:     ActionTypeSilence,
    Category:   CategoryHateSpeech,
    Severity:   SeverityHigh,
    Confidence: 0.9,
}
modService.AddReview(ctx, review)
```

### 3. Trust Management
```go
// Establish trust relationship
rel := &TrustRelationship{
    TrusterID:  adminID,
    TrusteeID:  moderatorID,
    Category:   TrustCategoryContent,
    Score:      0.8,
    Confidence: 0.9,
}
trustService.CreateRelationship(ctx, rel)
```

## API Endpoints (To Be Implemented)

### Moderation API
- `POST /api/v1/moderation/flag` - Flag content
- `POST /api/v1/moderation/review` - Submit review
- `GET /api/v1/moderation/queue` - Get review queue
- `GET /api/v1/moderation/events/{id}` - Get event details
- `POST /api/v1/moderation/appeal` - Appeal decision

### Trust API
- `GET /api/v1/trust/score/{actorID}` - Get trust score
- `POST /api/v1/trust/relationship` - Create/update trust
- `GET /api/v1/trust/network/{actorID}` - Get trust network

## Performance Characteristics

- **Event Creation**: ~10ms (single DynamoDB write)
- **Review Addition**: ~10ms (single DynamoDB write)
- **Consensus Calculation**: ~50ms (depends on review count)
- **Trust Score Query**: ~5ms (cached) / ~100ms (calculated)
- **Decision Application**: ~20ms (DynamoDB write + stream)

## Security Considerations

1. **Trust Manipulation**: Trust updates require evidence
2. **Sybil Attacks**: Minimum trust weight prevents spam
3. **Brigade Protection**: Trust decay limits influence
4. **Audit Trail**: All actions logged with TTL

## Cost Analysis

Assuming 1000 active users, 100 posts/day, 10% flagging rate:
- Moderation events: 10 writes/day = $0.0125
- Reviews (3 per event): 30 writes/day = $0.0375
- Trust updates: 30 writes/day = $0.0375
- Stream processing: 70 invocations/day = $0.0014
- **Total**: ~$0.09/day or $2.70/month

## Conclusion

The Reactive Moderation Mesh represents a paradigm shift in content moderation:
- **Democratic**: Consensus-based decisions
- **Transparent**: Full audit trail
- **Scalable**: Serverless architecture
- **Fair**: Trust-weighted voting
- **Reactive**: Real-time processing

This system proves that effective moderation doesn't require centralized control or expensive infrastructure. 
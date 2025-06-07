# Phase 4.2: Community Notes Implementation

## Overview

Community Notes is a crowdsourced fact-checking and context system that allows users to collaboratively add helpful information to posts. This implementation provides a reputation-gated, vote-weighted system with AI analysis and federation support.

## Architecture

### Core Components

1. **Note Storage** (`pkg/notes/`)
   - DynamoDB-based storage with efficient indexing
   - GSI indexes for querying by object, author, and visibility
   - Automatic TTL for note expiration

2. **Scoring Algorithm** (`pkg/notes/scoring.go`)
   - Wilson score calculation for confidence intervals
   - Multi-factor scoring combining:
     - Vote counts and weights
     - Author reputation
     - AI sentiment and objectivity analysis
     - Source quality assessment
   - Time decay for relevance

3. **API Handlers** (`cmd/api/handlers/notes.go`)
   - Create notes with reputation gating
   - Retrieve notes with trust-based ranking
   - Vote on notes with weighted impact
   - Get notes by author

4. **Note Processor Lambda** (`cmd/note-processor/`)
   - DynamoDB stream processing
   - AWS Comprehend integration for AI analysis
   - Source verification and scoring
   - WebSocket broadcasting for real-time updates

## API Endpoints

### Create Community Note
```
POST /api/v1/notes
Authorization: Bearer <token>

{
  "object_id": "https://instance.com/objects/123",
  "content": "Additional context: This study was later retracted.",
  "sources": [
    {
      "url": "https://journal.com/retraction-notice",
      "title": "Retraction Notice"
    }
  ],
  "language": "en"
}

Response:
{
  "id": "note_abc123",
  "object_id": "https://instance.com/objects/123",
  "content": "Additional context: This study was later retracted.",
  "author_id": "user123",
  "author_rep": 750,
  "score": 0.65,
  "visibility_status": "visible",
  "helpful_votes": 0,
  "not_helpful_votes": 0,
  "created_at": "2025-01-06T12:00:00Z"
}
```

### Get Notes for Object
```
GET /api/v1/notes/:object_id
Authorization: Bearer <token>

Response: [
  {
    "id": "note_abc123",
    "content": "Additional context...",
    "score": 0.85,
    "visibility_status": "prominent",
    "helpful_votes": 45,
    "not_helpful_votes": 3,
    ...
  }
]
```

### Vote on Note
```
POST /api/v1/notes/:id/vote
Authorization: Bearer <token>

{
  "vote_type": "helpful"  // or "not_helpful"
}

Response:
{
  "success": true,
  "score": 0.87,
  "helpful_votes": 46,
  "not_helpful_votes": 3
}
```

### Get User's Notes
```
GET /api/v1/accounts/:id/notes
Authorization: Bearer <token>

Response: [
  {
    "id": "note_xyz789",
    "object_id": "https://instance.com/objects/456",
    "content": "Fact check: This claim is disputed...",
    "score": 0.72,
    ...
  }
]
```

## Reputation Requirements

- **Minimum 100 reputation** to create notes
- **500+ reputation** required to vouch for others (future feature)
- Vote weight scales with reputation:
  - 100-499 rep: 1x weight
  - 500-999 rep: 1.5x weight
  - 1000+ rep: 2x weight

## Rate Limits

- **5 notes per hour** per user
- **20 votes per hour** per user
- Limits stored in DynamoDB with TTL

## Scoring Algorithm

### Wilson Score Confidence Interval
Used to rank notes based on helpful/not helpful votes, accounting for sample size:

```
score = (positive + 1.9208) / (positive + negative + 3.8416) +
        1.9208 * sqrt(positive * negative / (positive + negative) + 0.9604) /
        (3.8416 * (positive + negative + 3.8416))
```

### Multi-Factor Score
```
final_score = wilson_score * 0.5 +
              author_reputation_factor * 0.2 +
              ai_analysis_score * 0.15 +
              source_quality_score * 0.15
```

### Time Decay
```
time_factor = 1 / (1 + hours_since_creation / 168)  // Weekly half-life
```

## Visibility Thresholds

- **Hidden**: Score < 0.3 or insufficient votes
- **Disputed**: 0.3 ≤ Score < 0.5
- **Visible**: 0.5 ≤ Score < 0.7
- **Prominent**: Score ≥ 0.7

## AI Analysis

The Note Processor Lambda uses AWS Comprehend to analyze:

1. **Sentiment Analysis**
   - Positive, neutral, negative scores
   - Weighted combination for overall sentiment

2. **Objectivity Scoring**
   - Based on neutral sentiment
   - Penalizes extreme positive/negative language

3. **PII Detection**
   - Flags notes containing personal information
   - May affect visibility decisions

4. **Language Detection**
   - Ensures notes match expected language
   - Enables multi-language support

## Source Verification

Sources are scored based on domain reputation:
- Well-known reliable domains: 0.8-0.95
- Government (.gov) domains: 0.8
- Educational (.edu) domains: 0.75
- Unknown but valid URLs: 0.5
- Invalid or no sources: 0.3

## Federation Support

Notes with scores ≥ 0.7 are eligible for federation:
- Packaged as ActivityPub `Note` objects
- Attached to the original object as replies
- Include reputation vouching for cross-instance trust

## DynamoDB Schema

### Notes Table
- **PK**: `NOTE#<note_id>`
- **SK**: `METADATA`
- **GSI1**: `OBJECT#<object_id>` (by object)
- **GSI2**: `AUTHOR#<author_id>` (by author)
- **GSI3**: `VISIBLE#<score>` (by visibility)

### Votes Table
- **PK**: `NOTE#<note_id>`
- **SK**: `VOTE#<voter_id>`
- **Attributes**: vote_type, weight, timestamp

### Rate Limits Table
- **PK**: `RATELIMIT#<user_id>`
- **SK**: `NOTE#<hour>` or `VOTE#<hour>`
- **TTL**: Automatic expiration

## Testing

Run the test script to verify functionality:
```bash
python test_community_notes.py --token YOUR_TOKEN
```

Tests include:
- Note creation with sources
- Note retrieval and ranking
- Voting with weight calculation
- Rate limiting enforcement
- User note queries

## Future Enhancements

1. **Cross-Instance Reputation**
   - Portable reputation scores
   - Cryptographic vouching

2. **Multi-Language Support**
   - Language-specific AI models
   - Translation integration

3. **Rich Media Context**
   - Image analysis for visual context
   - Video timestamp annotations

4. **Appeal System**
   - Challenge incorrect notes
   - Consensus-based resolution

## Security Considerations

1. **Reputation Gating** prevents spam and low-quality contributions
2. **Rate Limiting** prevents abuse and vote manipulation
3. **AI Analysis** helps identify harmful or misleading content
4. **Source Verification** encourages evidence-based notes
5. **Weighted Voting** gives more influence to trusted users

## Cost Tracking

All Community Notes operations include cost tracking:
- DynamoDB read/write costs
- AWS Comprehend API calls
- Lambda execution time
- Data transfer costs

Costs are returned in API response headers:
- `X-Cost-Total-Micros`
- `X-Cost-DynamoDB-Reads`
- `X-Cost-DynamoDB-Writes`
- `X-Cost-AI-Calls` 
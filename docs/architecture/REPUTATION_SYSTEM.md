# Portable Reputation API Design

## Overview

The Portable Reputation API allows users to build, export, and import cryptographically signed reputation across ActivityPub instances. This creates a trust layer that transcends individual servers, enabling:

- **Reputation Portability**: Users can move instances without losing their built-up reputation
- **Vouch System**: Established users can vouch for new users, bootstrapping trust
- **Cross-Instance Verification**: Instances can verify reputation claims from other servers
- **Spam/Bot Prevention**: New accounts with vouches are less likely to be spam

## Architecture

### Components

1. **Reputation Calculator** - Aggregates various signals into a reputation score
2. **Cryptographic Signer** - Signs reputation data with instance keys
3. **Reputation Verifier** - Verifies signatures from other instances
4. **Vouch Manager** - Handles vouch creation and validation
5. **Export/Import API** - Manages reputation portability

### Data Model

```go
// Core reputation structure
type Reputation struct {
    // Identity
    ActorID      string    `json:"@id"`
    InstanceURL  string    `json:"instance"`
    
    // Scores (0-1000 scale)
    TrustScore   int       `json:"trustScore"`
    ActivityScore int      `json:"activityScore"`
    ModerationScore int    `json:"moderationScore"`
    CommunityScore int     `json:"communityScore"`
    
    // Metadata
    CalculatedAt time.Time `json:"calculatedAt"`
    Version      string    `json:"version"`
    
    // Evidence
    TotalPosts   int       `json:"totalPosts"`
    TotalFollowers int     `json:"totalFollowers"`
    AccountAge   int       `json:"accountAgeDays"`
    VouchCount   int       `json:"vouchCount"`
    
    // Cryptographic proof
    Signature    string    `json:"signature"`
    PublicKey    string    `json:"publicKey"`
}

// Vouch structure
type Vouch struct {
    ID           string    `json:"@id"`
    From         string    `json:"from"`         // Actor who vouched
    To           string    `json:"to"`           // Actor being vouched for
    InstanceURL  string    `json:"instance"`
    CreatedAt    time.Time `json:"createdAt"`
    ExpiresAt    time.Time `json:"expiresAt"`
    Confidence   float64   `json:"confidence"`   // 0.0-1.0
    Context      string    `json:"context"`      // Why vouching
    
    // Cryptographic proof
    Signature    string    `json:"signature"`
}

// Portable reputation document (JSON-LD)
type PortableReputation struct {
    Context      []string     `json:"@context"`
    Type         string       `json:"@type"`
    Actor        string       `json:"actor"`
    Reputation   *Reputation  `json:"reputation"`
    Vouches      []Vouch      `json:"vouches"`
    IssuedAt     time.Time    `json:"issuedAt"`
    ExpiresAt    time.Time    `json:"expiresAt"`
    
    // Instance attestation
    Issuer       string       `json:"issuer"`
    IssuerProof  string       `json:"issuerProof"`
}
```

### DynamoDB Schema

```
Table: ReputationTable
  Primary Key:
    PK: ACTOR#<actor_id>
    SK: REP#<timestamp>
  
  Attributes:
    - ReputationData (JSON)
    - Signature
    - TTL

Table: VouchTable
  Primary Key:
    PK: VOUCH#<vouch_id>
    SK: VOUCH#<vouch_id>
  
  GSI1:
    PK: FROM#<actor_id>
    SK: CREATED#<timestamp>
  
  GSI2:
    PK: TO#<actor_id>
    SK: CREATED#<timestamp>
  
  Attributes:
    - VouchData (JSON)
    - Signature
    - TTL
```

## API Endpoints

### REST API

```
GET /api/v1/reputation/:actor_id
  - Get current reputation for an actor
  - Returns: Reputation object

POST /api/v1/reputation/export
  - Export portable reputation document
  - Auth: Required (must be own account)
  - Returns: PortableReputation JSON-LD

POST /api/v1/reputation/import
  - Import reputation from another instance
  - Body: PortableReputation JSON-LD
  - Returns: Import status

POST /api/v1/vouches
  - Create a vouch for another user
  - Body: { to: actor_id, confidence: 0.8, context: "..." }
  - Auth: Required

GET /api/v1/vouches/:actor_id
  - Get vouches for an actor
  - Returns: Array of Vouch objects

POST /api/v1/reputation/verify
  - Verify a reputation document
  - Body: PortableReputation JSON-LD
  - Returns: Verification result
```

### GraphQL API

```graphql
extend type Query {
  reputation(actorId: ID!): Reputation
  vouches(actorId: ID!, first: Int, after: Cursor): VouchConnection
  verifyReputation(document: String!): VerificationResult
}

extend type Mutation {
  exportReputation: PortableReputation!
  importReputation(document: String!): ImportResult!
  createVouch(input: CreateVouchInput!): Vouch!
  revokeVouch(vouchId: ID!): Boolean!
}

type Reputation {
  actor: Actor!
  trustScore: Int!
  activityScore: Int!
  moderationScore: Int!
  communityScore: Int!
  calculatedAt: Time!
  vouches: [Vouch!]!
}

type Vouch {
  id: ID!
  from: Actor!
  to: Actor!
  confidence: Float!
  context: String
  createdAt: Time!
  expiresAt: Time!
}
```

## Cryptographic Design

### Key Management

1. **Instance Keys**: Each instance has an ED25519 keypair for signing reputation
2. **Actor Keys**: Actors can optionally have their own keys for self-attestation
3. **Key Distribution**: Public keys available via `.well-known/reputation-keys`

### Signature Process

```go
// 1. Canonicalize data (JSON-LD normalization)
canonical := normalizeJSONLD(reputation)

// 2. Create signature base
signatureBase := sha256(canonical)

// 3. Sign with instance private key
signature := ed25519.Sign(privateKey, signatureBase)

// 4. Attach signature
reputation.Signature = base64.Encode(signature)
```

### Verification Process

```go
// 1. Fetch instance public key
publicKey := fetchInstanceKey(reputation.InstanceURL)

// 2. Remove signature from object
sig := reputation.Signature
reputation.Signature = ""

// 3. Canonicalize and hash
canonical := normalizeJSONLD(reputation)
signatureBase := sha256(canonical)

// 4. Verify signature
valid := ed25519.Verify(publicKey, signatureBase, sig)
```

## Reputation Calculation

### Trust Score (0-250 points)
- Based on trust graph relationships
- Weighted by relationship strength
- Decays over time without activity

### Activity Score (0-250 points)
- Post frequency and consistency
- Engagement rates
- Content quality signals

### Moderation Score (0-250 points)
- Clean moderation history: 250 points
- Each validated report: -50 points
- Each false report against them: +10 points

### Community Score (0-250 points)
- Vouches received: up to 100 points
- Community notes contributed: up to 50 points
- Helpful interactions: up to 100 points

### Total Score: 0-1000 points

## Vouch System

### Vouch Creation Rules
1. Voucher must have reputation > 500
2. Can vouch for max 5 new users per month
3. Vouches expire after 6 months
4. Confidence level affects vouch weight

### Vouch Impact
- New user starts with: 100 + (sum of vouch confidence * voucher reputation / 10)
- Max initial boost: 300 points
- Vouches decay in value over time

### Vouch Revocation
- Vouchers can revoke their vouches
- If vouched user is suspended, voucher loses reputation
- Creates accountability chain

## Security Considerations

### Attack Vectors & Mitigations

1. **Reputation Farming**
   - Mitigation: Rate limits, decay functions, diverse signal requirements

2. **Sybil Attacks**
   - Mitigation: Vouch limits, graph analysis, instance verification

3. **Signature Forgery**
   - Mitigation: Strong cryptography, key rotation, certificate transparency

4. **Instance Collusion**
   - Mitigation: Instance reputation, diversity requirements

## Implementation Plan

### Phase 1: Core Reputation System (Week 1)
- [ ] Reputation calculator Lambda
- [ ] DynamoDB tables
- [ ] Basic API endpoints
- [ ] Signature implementation

### Phase 2: Vouch System (Week 1-2)
- [ ] Vouch creation/management
- [ ] Vouch impact calculation
- [ ] Revocation handling
- [ ] Rate limiting

### Phase 3: Portability (Week 2)
- [ ] Export API
- [ ] Import API
- [ ] Verification endpoint
- [ ] JSON-LD formatting

### Phase 4: Integration (Week 2)
- [ ] GraphQL resolvers
- [ ] Trust graph integration
- [ ] Moderation mesh hooks
- [ ] Cost tracking

## Success Metrics

1. **Adoption**
   - 50% of active users have reputation > 200
   - 10% of new users arrive with vouches

2. **Security**
   - <1% false positive spam detection
   - Zero successful reputation forgeries

3. **Performance**
   - Reputation calculation < 100ms
   - Signature verification < 50ms

4. **Portability**
   - 95% successful import rate
   - Cross-instance verification in < 200ms 
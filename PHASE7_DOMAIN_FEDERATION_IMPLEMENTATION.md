# Phase 7.4: Domain & Federation Management Implementation

## Overview

This phase implements administrative controls for managing federation at the instance level. This includes domain blocks, allow lists, and federation policies that align with Lesser's philosophy of user empowerment while providing necessary instance-level controls.

## Key Principles

1. **User Autonomy First**: Instance-level blocks should be transparent and users should understand why domains are blocked
2. **Flexible Federation**: Support both blocklist and allowlist modes
3. **Trust Integration**: Domain reputation affects federation decisions
4. **Transparency**: Public or authenticated visibility of federation policies

## Endpoints to Implement

### 7.4.1 Domain Blocks (Instance Level)

#### GET /api/v1/admin/domain_blocks
- List all domain blocks with pagination
- Include block severity (silence vs suspend)
- Show public/private comment fields
- Include creation date and admin who created

#### GET /api/v1/admin/domain_blocks/:id  
- View specific domain block details
- Include full history if available

#### POST /api/v1/admin/domain_blocks
- Create new domain block
- Parameters:
  - `domain`: Domain to block
  - `severity`: "silence" or "suspend" 
  - `reject_media`: Boolean to reject media files
  - `reject_reports`: Boolean to reject reports from domain
  - `private_comment`: Admin-only notes
  - `public_comment`: Public reason for block
  - `obfuscate`: Whether to obfuscate domain in public lists

#### PUT /api/v1/admin/domain_blocks/:id
- Update existing domain block
- Can modify severity and comments

#### DELETE /api/v1/admin/domain_blocks/:id
- Remove domain block
- Should log who removed and when

### 7.4.2 Domain Allows (Allowlist Mode)

#### GET /api/v1/admin/domain_allows
- List allowed domains when in allowlist mode
- Include creation info

#### POST /api/v1/admin/domain_allows
- Add domain to allowlist
- Only relevant in allowlist federation mode

#### DELETE /api/v1/admin/domain_allows/:id
- Remove from allowlist

### 7.4.3 Federation Insights

#### GET /api/v1/admin/federation/statistics
- Federation statistics dashboard
- Active instances count
- Message volume by domain
- User distribution across instances

#### GET /api/v1/admin/federation/instances
- List known instances with metadata
- Last contact time
- Software type and version
- User counts
- Trust score
- Message volume

#### GET /api/v1/admin/federation/instance/:domain
- Detailed view of specific instance
- Users from that instance
- Recent activity
- Trust metrics
- Moderation history

### 7.4.4 Email Domain Blocks

#### GET /api/v1/admin/email_domain_blocks
- List blocked email domains for registration

#### POST /api/v1/admin/email_domain_blocks
- Block email domain from registration

#### DELETE /api/v1/admin/email_domain_blocks/:id
- Remove email domain block

## Storage Design

### Domain Blocks Table Pattern
```
PK: DOMAIN_BLOCK#<domain>
SK: DOMAIN_BLOCK#<domain>
GSI1PK: DOMAIN_BLOCKS
GSI1SK: <created_at>
Attributes:
- Domain
- Severity (silence/suspend)
- RejectMedia
- RejectReports  
- PrivateComment
- PublicComment
- Obfuscate
- CreatedBy
- CreatedAt
- UpdatedAt
```

### Federation Stats Pattern
```
PK: FEDERATION#<domain>
SK: STATS#<date>
GSI1PK: FEDERATION_ACTIVE
GSI1SK: <last_activity>
Attributes:
- Domain
- MessagesSent
- MessagesReceived
- ActiveUsers
- TrustScore
- LastActivity
- Software
- Version
```

### Instance Info Pattern
```
PK: INSTANCE#<domain>
SK: INSTANCE#<domain>
Attributes:
- Domain
- Software
- Version
- FirstSeen
- LastSeen
- PublicKey
- SharedInbox
- TrustScore
- BlockedAt (if blocked)
- SilencedAt (if silenced)
```

## Implementation Plan

### Step 1: Storage Layer
1. Create domain block storage methods
2. Create domain allow storage methods  
3. Create federation statistics storage
4. Create instance tracking storage

### Step 2: Domain Block Handlers
1. Implement CRUD operations for domain blocks
2. Add validation for domain format
3. Integrate with federation delivery logic
4. Update inbox processing to respect blocks

### Step 3: Federation Insights
1. Create instance discovery from activities
2. Track federation statistics
3. Build instance detail views
4. Calculate trust scores for domains

### Step 4: Integration Points
1. Update inbox handler to check domain blocks
2. Update delivery to skip blocked domains
3. Update media handler to respect reject_media
4. Update reports handler to respect reject_reports

### Step 5: Public APIs
1. Add public endpoint for domain blocks (if configured)
2. Update instance info endpoint with federation policy

## Trust Integration

### Domain Trust Score Calculation
- Base score from instance age and activity
- Moderation actions affect score
- User reports from domain
- Successful interactions increase score
- Failed deliveries decrease score

### Trust-Based Auto-Moderation
- Domains below trust threshold auto-silenced
- Extremely low trust triggers review
- New domains start at neutral score
- First few interactions closely monitored

## Enhanced Features

### Federation Mesh
- Share domain reputation with trusted instances
- Import block lists from trusted sources
- Export our domain assessments
- Consensus-based domain evaluation

### Proactive Protection  
- Detect spam/bot patterns from domains
- Alert on unusual activity patterns
- Auto-quarantine suspicious domains
- ML-based domain classification

## Security Considerations

1. **Block Bypass Prevention**
   - Check domain blocks at multiple points
   - Handle subdomain blocking correctly
   - Prevent block evasion via redirects

2. **Performance**
   - Cache domain block list in memory
   - Efficient lookup structures
   - Batch check optimizations

3. **Audit Trail**
   - Log all federation admin actions
   - Track block effectiveness
   - Monitor false positive rates

## Next Steps

1. Implement basic domain block CRUD
2. Integrate blocks into inbox processing
3. Add federation statistics tracking
4. Build instance insight dashboards
5. Implement trust score calculation
6. Add public transparency endpoints 
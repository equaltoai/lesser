# DynamoDB Index Remediation Plan

> **Status**: Draft v2  
> **Created**: 2025-12-18  
> **Revised**: 2025-12-18 (Corrected TTL/OAuth direction, helper code issues, complete index alias inventory)  
> **Compatibility**: No backward compatibility required (no install base)

This document provides a comprehensive remediation plan for fixing DynamoDB indexing issues identified in the Lesser codebase. Since there is no existing install base, all changes can be made without migration considerations.

---

## Executive Summary

The Lesser DynamoDB design uses 9 generic GSIs (`GSI1-GSI9`) with overloaded key prefixes. However, the implementation has several critical issues:

| Issue Category | Count | Severity |
|----------------|-------|----------|
| Mixed-case GSI naming (`GSI1` vs `gsi1` vs `gsi1-index`) | ~80+ locations | 🔴 Critical |
| Descriptive index names without physical backing | ~60+ locations | 🔴 Critical |
| TTL attribute CDK/DynamORM mismatch | 1 CDK file | 🟡 Must fix CDK |
| OAuth Client index CDK/DynamORM mismatch | 1 CDK file | 🟡 Must fix CDK |
| Helper code derives attribute names from index names | 6 files | 🔴 Critical |
| Invalid DynamORM tag syntax (semicolons) | 1 model | 🟡 High |
| Same index name, different key schemas | 2+ index names | 🟡 High |
| Multiple TTL attributes used in models (`ttl` vs `expiresAt`) | 20+ model fields | 🔴 Critical |
| `.Index(...)` names not declared in any model tags | 10+ call sites | 🔴 Critical |

**Root Cause**: The design assumed DynamORM would resolve descriptive index names to physical GSIs based on attribute naming patterns. This assumption was incorrect—DynamoDB requires exact index name matches.

---

## ⚠️ Critical Corrections (From Review)

### DynamORM Naming Validation

DynamORM enforces **camelCase attribute names**. The original plan proposed changing model tags to `attr:TTL` and `attr:OAuthClientsPK`—these uppercase-leading attribute names would **fail model registration**.

**Correct Fix Direction:**
- ✅ **Fix CDK** to use `ttl` and `oauthClientsPK` (matching DynamORM convention)
- ❌ ~~Fix models to use `attr:TTL`~~ (would break DynamORM)

### Helper Code Breaks with `GSI1` Naming

Several repository helpers derive attribute names from index names:

```go
// query_utils.go:167, 353
Where(fmt.Sprintf("%sPK", indexName), "=", gsiPK)

// base_repository.go:694, 698, 1396
skField := fmt.Sprintf("%sSK", indexName)
Where(fmt.Sprintf("%sPK", indexName), "=", pk)
```

If we standardize to `GSI1`, these would produce:
- `GSI1PK` (wrong—attribute is `gsi1PK`)
- `GSI1SK` (wrong—attribute is `gsi1SK`)

**Correct Fix**: The index names MUST be lowercase `gsi1`, `gsi2`, etc. to work with existing helper code.

### DynamoDB TTL: Table Can Only Have One TTL Attribute

The main table can only have **one** TTL attribute configured. Today the codebase uses **both**:
- `ttl` (common) via `dynamorm:"ttl,attr:ttl"`
- `expiresAt` (also common) via `dynamorm:"ttl,attr:expiresAt"`

If CDK is updated to `TimeToLiveAttribute: "ttl"` (recommended here), then any model that marks TTL on `expiresAt` will **stop expiring automatically** until corrected.

**Correct Fix Direction**
- ✅ Pick **one** TTL attribute for the table (recommend `ttl` to minimize code churn)
- ✅ Update any models using `ttl,attr:expiresAt` to use `ttl,attr:ttl` (and write the unix timestamp to that attribute)

---

## Part 1: Infrastructure Fixes (CDK)

### 1.1 TTL Attribute - Fix CDK to Match DynamORM

**Current CDK** (`infra/cdk/stacks/lesser_api_stack.go:277`):
```go
TimeToLiveAttribute: jsii.String("TTL"),  // ❌ Uppercase
```

**Models use** (e.g., `pkg/storage/models/device.go:40`):
```go
TTL int64 `dynamorm:"ttl,attr:ttl" json:"ttl,omitempty"`  // ✅ Lowercase
```

**Required Fix**: Update CDK to use lowercase `ttl`:
```go
TimeToLiveAttribute: jsii.String("ttl"),  // ✅ Matches model
```

**File to Update**: `infra/cdk/stacks/lesser_api_stack.go:277`

---

### 1.2 OAuth Client Index - Fix CDK to Match DynamORM

**Current CDK** (`infra/cdk/stacks/lesser_api_stack.go:332-337`):
```go
PartitionKey: &awsdynamodb.Attribute{
    Name: jsii.String("OAuthClientsPK"),  // ❌ PascalCase
    Type: awsdynamodb.AttributeType_STRING,
},
SortKey: &awsdynamodb.Attribute{
    Name: jsii.String("OAuthClientsSK"),  // ❌ PascalCase
    Type: awsdynamodb.AttributeType_STRING,
},
```

**Model uses** (`pkg/storage/models/oauth_client.go:18-19`):
```go
OAuthClientsPK string `dynamorm:"index:oauth-clients-index,pk,attr:oauthClientsPK"`  // ✅ camelCase
OAuthClientsSK string `dynamorm:"index:oauth-clients-index,sk,attr:oauthClientsSK"`  // ✅ camelCase
```

**Required Fix**: Update CDK to use camelCase:
```go
PartitionKey: &awsdynamodb.Attribute{
    Name: jsii.String("oauthClientsPK"),  // ✅ Matches model
    Type: awsdynamodb.AttributeType_STRING,
},
SortKey: &awsdynamodb.Attribute{
    Name: jsii.String("oauthClientsSK"),  // ✅ Matches model
    Type: awsdynamodb.AttributeType_STRING,
},
```

**File to Update**: `infra/cdk/stacks/lesser_api_stack.go:332-337`

---

### 1.3 GSI Names - Fix CDK to Use Lowercase

**Current CDK** (`infra/cdk/stacks/lesser_api_stack.go:309-310`):
```go
IndexName: jsii.String(fmt.Sprintf("GSI%d", i)),  // ❌ Uppercase
```

**Required Fix**: Use lowercase to match attribute convention:
```go
IndexName: jsii.String(fmt.Sprintf("gsi%d", i)),  // ✅ Matches gsi%dPK/gsi%dSK
```

**Rationale**: The helper code in `base_repository.go` and `query_utils.go` derives attribute names by appending `PK`/`SK` to the index name. With lowercase `gsi1`, this produces `gsi1PK` which matches the attribute.

**File to Update**: `infra/cdk/stacks/lesser_api_stack.go:310` (and line 556 for GSI9)

### 1.4 Monitoring Stack Must Track Renamed GSI Names

`infra/cdk/stacks/monitoring_stack.go:407` hardcodes `GSI1..GSI8` for CloudWatch metrics.

If you rename physical indexes to `gsi1..gsi9`, you must also update monitoring to use lowercase names; otherwise dashboards/alarms won’t populate.

---

## Part 2: Index Name Standardization

### 2.1 The Standard: Lowercase `gsi1-gsi9`

All index names must use **lowercase** to work with helper code:

| Physical Index | Key Attributes | Canonical Name |
|----------------|----------------|----------------|
| `gsi1` | `gsi1PK`, `gsi1SK` | `gsi1` |
| `gsi2` | `gsi2PK`, `gsi2SK` | `gsi2` |
| `gsi3` | `gsi3PK`, `gsi3SK` | `gsi3` |
| `gsi4` | `gsi4PK`, `gsi4SK` | `gsi4` |
| `gsi5` | `gsi5PK`, `gsi5SK` | `gsi5` |
| `gsi6` | `gsi6PK`, `gsi6SK` | `gsi6` |
| `gsi7` | `gsi7PK`, `gsi7SK` | `gsi7` |
| `gsi8` | `gsi8PK`, `gsi8SK` | `gsi8` |
| `gsi9` | `gsi9PK`, `gsi9SK` | `gsi9` |
| `oauth-clients-index` | `oauthClientsPK`, `oauthClientsSK` | `oauth-clients-index` |

### 2.2 Complete Index Alias Inventory

#### Aliases Using `gsi1PK/gsi1SK` → Standardize to `gsi1`

| Current Alias | Files Using It |
|---------------|----------------|
| `GSI1` | device.go, poll.go, quote_relationship.go, thread_node.go, export.go, oauth_app.go, many more |
| `gsi1` | media_analytics.go, notification_cost_tracking.go, moderation.go, delivery_status.go, community_note.go |
| `gsi1-index` | object_repository.go, trust_repository.go, media_metadata_repository.go |
| `endpoint-index` | push_subscription.go |
| `provider-index` | provider_account.go |
| `stream-target-index` | streaming_event.go |
| `user-jobs-index` | media_job.go, transcoding_job.go |
| `user-sessions-index` | session.go, oauth_session.go, oauth_session_repository.go |
| `time-index` | ai_cost.go |
| `table-index` | cost_tracking.go |
| `status-hashtag-index` | hashtag_status_index.go |
| `hashtag-trending-history` | hashtag_status_index.go |
| `token-index` | account_repository_auth.go (⚠️ CONFLICT - see Part 3) |
| `type-index` | federation_activity_repository.go, notification_repository.go |
| `error-index` | dlq_repository.go |
| `user-csrf-index` | csrf_repository.go |
| `status-date-index` | (various models) |
| `username-search-index` | (various models) |

#### Aliases Using `gsi2PK/gsi2SK` → Standardize to `gsi2`

| Current Alias | Files Using It |
|---------------|----------------|
| `GSI2` | device.go, quote_relationship.go, social_recovery_request.go |
| `gsi2` | media_popularity.go, notification_cost_tracking.go, relationship_repository.go, federation_repository.go |
| `gsi2-index` | object_repository.go |
| `cost-variant-index` | media_analytics.go |
| `hashtag-visibility-index` | hashtag_status_index.go |
| `trending-by-period` | hashtag_status_index.go |
| `user-providers-index` | account_repository_auth.go |
| `actor-index` | federation_activity_repository.go |
| `retry-index` | dlq_repository.go |
| `state-index` | oauth_session_repository.go |
| `token-index` | session.go (⚠️ CONFLICT - see Part 3) |

#### Aliases Using `gsi3PK/gsi3SK` → Standardize to `gsi3`

| Current Alias | Files Using It |
|---------------|----------------|
| `GSI3` | (various) |
| `gsi3` | notification_cost_tracking.go, relationship_repository.go, rate_limit_repository.go, community_note_repository.go |
| `service-index` | dlq_repository.go (⚠️ CONFLICT - see Part 3) |
| `group-index` | notification_repository.go |
| `domain-index` | (various) |
| `hashtag-search-index` | (various) |

#### Aliases Using `gsi4PK/gsi4SK` → Standardize to `gsi4`

| Current Alias | Files Using It |
|---------------|----------------|
| `GSI4` | status_repository.go, instance_repository.go |
| `cost-date-index` | ai_analysis.go |
| `popularity-index` | (various) |

#### Aliases Using `gsi5PK/gsi5SK` → Standardize to `gsi5`

| Current Alias | Files Using It |
|---------------|----------------|
| `GSI5` | social_repository.go, status_repository.go |
| `activity-index` | (various) |

#### Aliases Using `gsi6PK/gsi6SK` → Standardize to `gsi6`

| Current Alias | Files Using It |
|---------------|----------------|
| `GSI6` | (various) |
| `gsi6-index` | object_repository.go |

#### Aliases Using `gsi7PK/gsi7SK` → Standardize to `gsi7`

| Current Alias | Files Using It |
|---------------|----------------|
| `GSI7` | status_repository.go |

#### Aliases Using `gsi8PK/gsi8SK` → Standardize to `gsi8`

| Current Alias | Files Using It |
|---------------|----------------|
| `GSI8` | (various) |
| `gsi8` | (various) |

#### Non-Existent Indexes (Used But Never Provisioned)

These are called in repositories but have **no corresponding physical GSI**:

| Index Name | Repository | Line |
|------------|------------|------|
| `local-timeline-index` | account_repository_timeline.go | 87 |
| `hashtag-timeline-index` | account_repository_timeline.go | 180 |
| `list-timeline-index` | account_repository_timeline.go | 232 |
| `user-tokens-index` | account_repository_refresh_tokens.go | 254 |
| `family-tokens-index` | account_repository_refresh_tokens.go | 274 |
| `user-votes-index` | community_note_repository.go | 46 |

**These will cause scan fallback or errors.**

In practice, if `.Index("<name>")` references an index that does not exist on the table, DynamoDB returns a `ValidationException` and the request fails. Do not rely on “fallback” behavior here.

#### Index Names Used In Queries But Not Declared In Any Model

These appear in `.Index("...")` calls but do **not** appear in any `dynamorm:"index:<name>,..."` struct tags, which means there is no obvious slot mapping and they are easy to drift:

`display-name-index`, `email-index`, `family-tokens-index`, `follower-count-index`, `gsi4-index`, `gsi5`, `hashtag-timeline-index`, `list-timeline-index`, `local-timeline-index`, `name-index`, `tenant-entity`, `user-credentials-index`, `users-by-role`, `user-tokens-index`, `user-votes-index`, `webfinger-index`.

**Fix direction**: convert each call site to `Index("gsiN")` and ensure the model declares `index:gsiN` tags for the participating key fields (`gsiNPK/gsiNSK`).

---

## Part 3: Conflicting Index Names

These index names are used with **different** key schemas, which is impossible:

### 3.1 `service-index` Conflict

| File | Key Schema | Usage |
|------|------------|-------|
| `pkg/storage/models/metrics.go` | `gsi1PK/gsi1SK` | Service metrics |
| `pkg/storage/models/dlq_message.go` | `gsi3PK/gsi3SK` | DLQ by service |

**Resolution**:
- Metrics: Use `gsi1` 
- DLQ: Use `gsi3`

### 3.2 `token-index` Conflict

| File | Key Schema | Usage |
|------|------------|-------|
| `pkg/storage/models/password_reset.go` | `gsi1PK/gsi1SK` | Password reset tokens |
| `pkg/storage/models/session.go` | `gsi2PK/gsi2SK` | Session tokens |

**Resolution**:
- Password reset: Use `gsi1`
- Session: Use `gsi2`

---

## Part 4: Helper Code Fixes

### 4.1 Files With Index Name → Attribute Name Derivation

These files use `fmt.Sprintf("%sPK", indexName)` which **only works** if `indexName` equals the attribute prefix:

| File | Lines | Pattern |
|------|-------|---------|
| `pkg/storage/repositories/query_utils.go` | 167, 353, 356 | `%sPK`, `%sSK` |
| `pkg/storage/repositories/base_repository.go` | 694, 698, 1396 | `%sPK`, `%sSK` |
| `pkg/storage/repositories/shared_helpers_simple.go` | 41, 47, 48 | `%sPK`, `%sSK` |
| `pkg/storage/repositories/relationship_helpers.go` | 176 | `%sPK` |
| `pkg/storage/repositories/relationship_pagination_helpers.go` | 84 | `IndexName+"PK"` |
| `pkg/storage/repositories/user_repository.go` | 961 | Special handling: `strings.ToUpper(gsiIndex[:4])` |
| `pkg/storage/repositories/notification_helpers.go` | 130-131 | `GSI%sPK` (extracts number from end) |

### 4.2 How `notification_helpers.go` Works Correctly

```go
// Line 129-131
gsiNumber := gsiIndex[len(gsiIndex)-1:]  // Extracts "1" from "gsi1" or "GSI1"
gsiPKField := fmt.Sprintf("GSI%sPK", gsiNumber)  // Produces "GSI1PK" ❌ Wrong!
gsiSKField := fmt.Sprintf("GSI%sSK", gsiNumber)  // Produces "GSI1SK" ❌ Wrong!
```

**This is also broken** - it produces `GSI1PK` instead of `gsi1PK`.

**Fix Required**: Change to:
```go
gsiPKField := fmt.Sprintf("gsi%sPK", gsiNumber)  // Produces "gsi1PK" ✅
gsiSKField := fmt.Sprintf("gsi%sSK", gsiNumber)  // Produces "gsi1SK" ✅
```

### 4.3 How `user_repository.go:961` Works

```go
Where(fmt.Sprintf("%sPK", strings.ToUpper(gsiIndex[:4])), "=", ...)
// If gsiIndex = "gsi1", this produces "GSI1PK" ❌ Wrong!
```

**Fix Required**: Remove the `ToUpper()`:
```go
Where(fmt.Sprintf("%sPK", gsiIndex[:4]), "=", ...)  // "gsi1PK" ✅
```

Or better, use the full index name if it matches the attribute prefix:
```go
Where(fmt.Sprintf("%sPK", gsiIndex), "=", ...)
```

---

## Part 5: Invalid DynamORM Tag Syntax

### 5.1 AuthRefreshToken Semicolon Tags

**File**: `pkg/storage/models/auth_refresh_token.go:21`
```go
CreatedAtSK string `dynamorm:"index:user-index,sk,attr:createdAtSK;index:family-index,sk;index:user-family-index,sk" json:"-"`
```

**Problem**: DynamORM doesn't support semicolon-separated multiple index tags.

**Fix**: These indexes don't exist anyway. Redesign to use real GSIs:
```go
// Use GSI1 for user queries, GSI2 for family queries, GSI3 for user-family queries
GSI1PK    string `dynamorm:"index:gsi1,pk,attr:gsi1PK" json:"-"`     // USER#{userID}
GSI1SK    string `dynamorm:"index:gsi1,sk,attr:gsi1SK" json:"-"`     // {createdAt}
GSI2PK    string `dynamorm:"index:gsi2,pk,attr:gsi2PK" json:"-"`     // FAMILY#{family}
GSI2SK    string `dynamorm:"index:gsi2,sk,attr:gsi2SK" json:"-"`     // {createdAt}
GSI3PK    string `dynamorm:"index:gsi3,pk,attr:gsi3PK" json:"-"`     // USER_FAMILY#{userID}#{family}
GSI3SK    string `dynamorm:"index:gsi3,sk,attr:gsi3SK" json:"-"`     // {createdAt}
```

---

## Part 6: Implementation Plan

### Phase 1: CDK Fixes (Deploy First)

1. **Update TTL attribute** (`infra/cdk/stacks/lesser_api_stack.go:277`):
   ```go
   TimeToLiveAttribute: jsii.String("ttl"),
   ```

2. **Update OAuth index attributes** (`infra/cdk/stacks/lesser_api_stack.go:332-337`):
   ```go
   Name: jsii.String("oauthClientsPK"),
   Name: jsii.String("oauthClientsSK"),
   ```

3. **Update GSI names** (`infra/cdk/stacks/lesser_api_stack.go:310, 556`):
   ```go
   IndexName: jsii.String(fmt.Sprintf("gsi%d", i)),
   IndexName: jsii.String("gsi9"),
   ```

4. **Deploy CDK** to create new table with correct settings (or recreate if needed).

### Phase 2: Helper Code Fixes

Fix these files to use correct attribute naming:

```bash
# notification_helpers.go - Fix GSI attribute naming
sed -i 's/fmt.Sprintf("GSI%sPK"/fmt.Sprintf("gsi%sPK"/g' pkg/storage/repositories/notification_helpers.go
sed -i 's/fmt.Sprintf("GSI%sSK"/fmt.Sprintf("gsi%sSK"/g' pkg/storage/repositories/notification_helpers.go

# user_repository.go - Remove ToUpper()
# Manual fix required - see line 961
```

### Phase 3: Model Index Tag Standardization

Standardize all index names to lowercase `gsi1`, `gsi2`, etc.:

```bash
#!/bin/bash
# Run from repo root

# Replace uppercase GSI names with lowercase
find pkg/storage/models -name "*.go" -exec sed -i 's/index:GSI1,/index:gsi1,/g' {} \;
find pkg/storage/models -name "*.go" -exec sed -i 's/index:GSI2,/index:gsi2,/g' {} \;
find pkg/storage/models -name "*.go" -exec sed -i 's/index:GSI3,/index:gsi3,/g' {} \;
find pkg/storage/models -name "*.go" -exec sed -i 's/index:GSI4,/index:gsi4,/g' {} \;
find pkg/storage/models -name "*.go" -exec sed -i 's/index:GSI5,/index:gsi5,/g' {} \;
find pkg/storage/models -name "*.go" -exec sed -i 's/index:GSI6,/index:gsi6,/g' {} \;
find pkg/storage/models -name "*.go" -exec sed -i 's/index:GSI7,/index:gsi7,/g' {} \;
find pkg/storage/models -name "*.go" -exec sed -i 's/index:GSI8,/index:gsi8,/g' {} \;
find pkg/storage/models -name "*.go" -exec sed -i 's/index:GSI9,/index:gsi9,/g' {} \;

# Descriptive index names need manual mapping to correct GSI slots
# Each must be reviewed for which gsiNPK/gsiNSK it uses
```

### Phase 4: Repository `.Index()` Call Fixes

```bash
#!/bin/bash
# Run from repo root

# Replace uppercase GSI names with lowercase
find pkg/storage/repositories -name "*.go" -exec sed -i 's/Index("GSI1")/Index("gsi1")/g' {} \;
find pkg/storage/repositories -name "*.go" -exec sed -i 's/Index("GSI2")/Index("gsi2")/g' {} \;
find pkg/storage/repositories -name "*.go" -exec sed -i 's/Index("GSI3")/Index("gsi3")/g' {} \;
find pkg/storage/repositories -name "*.go" -exec sed -i 's/Index("GSI4")/Index("gsi4")/g' {} \;
find pkg/storage/repositories -name "*.go" -exec sed -i 's/Index("GSI5")/Index("gsi5")/g' {} \;
find pkg/storage/repositories -name "*.go" -exec sed -i 's/Index("GSI6")/Index("gsi6")/g' {} \;
find pkg/storage/repositories -name "*.go" -exec sed -i 's/Index("GSI7")/Index("gsi7")/g' {} \;
find pkg/storage/repositories -name "*.go" -exec sed -i 's/Index("GSI8")/Index("gsi8")/g' {} \;
find pkg/storage/repositories -name "*.go" -exec sed -i 's/Index("GSI9")/Index("gsi9")/g' {} \;

# Remove -index suffix variants
find pkg/storage/repositories -name "*.go" -exec sed -i 's/Index("gsi1-index")/Index("gsi1")/g' {} \;
find pkg/storage/repositories -name "*.go" -exec sed -i 's/Index("gsi2-index")/Index("gsi2")/g' {} \;
find pkg/storage/repositories -name "*.go" -exec sed -i 's/Index("gsi6-index")/Index("gsi6")/g' {} \;

# Descriptive names need manual mapping
```

### Phase 5: Manual Fixes (Require Review)

1. **Descriptive index names** - Each must be mapped to correct GSI slot based on which attributes it uses
2. **AuthRefreshToken model** - Redesign indexes
3. **Conflicting index names** - Resolve `service-index` and `token-index`
4. **Non-existent indexes** - Either provision them or map to existing GSIs

### Phase 6: Validation

```bash
# 1. Verify no uppercase GSI index names remain
grep -r 'index:GSI[0-9]' pkg/storage/models/
# Should return NO results

grep -r 'Index("GSI[0-9]")' pkg/storage/repositories/
# Should return NO results

# 2. Verify no descriptive index names remain (except oauth-clients-index)
grep -r 'index:.*-index' pkg/storage/models/ | grep -v 'oauth-clients-index'
# Should return NO results

# 3. Verify CDK uses lowercase
grep -r 'GSI%d' infra/cdk/
# Should return NO results

# 4. Compile and run tests
go build ./...
go test ./pkg/storage/...
```

---

## Part 7: GSI Registry Constants

Create a central file to document and enforce GSI usage:

**File**: `pkg/storage/models/gsi_registry.go`

```go
package models

// GSI Slot Registry
// This file documents which GSI slot is used for which access patterns.
// All models and repositories MUST use these exact index names.

const (
    // Physical GSI names - must match CDK exactly (lowercase)
    IndexGSI1 = "gsi1"
    IndexGSI2 = "gsi2"
    IndexGSI3 = "gsi3"
    IndexGSI4 = "gsi4"
    IndexGSI5 = "gsi5"
    IndexGSI6 = "gsi6"
    IndexGSI7 = "gsi7"
    IndexGSI8 = "gsi8"
    IndexGSI9 = "gsi9"
    
    // Dedicated OAuth index
    IndexOAuthClients = "oauth-clients-index"
)

/*
GSI1 Access Patterns (gsi1PK/gsi1SK):
- INBOX#{username} - User activity inbox
- USERNAME_SEARCH#{prefix} - Actor username search
- TIMELINE#PUBLIC#{scope} - Public timeline queries
- FOLLOW#{followedUsername} - Follower lookup (inverted relationship)
- USER_SESSIONS#{userID} - User's active sessions
- USER_JOBS#{userID} - Media/transcoding jobs
- PUSH_ENDPOINT#{hash} - Push subscription endpoints
- PROVIDER#{provider} - Provider accounts
- STREAM_TARGET#{type}#{id} - Streaming events
- COST_TABLE#{table} - Cost tracking by table
- ... (document all patterns)

GSI2 Access Patterns (gsi2PK/gsi2SK):
- NAME_SEARCH#{prefix} - Actor display name search
- TRUST_LEVEL#{level} - Devices by trust level
- SESSION_TOKEN#{token} - Session token lookup
- HASHTAG_VIS#{hashtag}#{visibility} - Hashtag visibility index
- DLQ_RETRY#{status} - DLQ retry queue
- USER_PROVIDERS#{userID} - User's OAuth providers
- ... (document all patterns)

GSI3 Access Patterns (gsi3PK/gsi3SK):
- DOMAIN#{domain} - Actor by domain
- HASHTAG_SEARCH#{prefix} - Hashtag search
- DLQ_SERVICE#{service} - DLQ by service
- NOTIFICATION_GROUP#{group} - Notification grouping
- ... (document all patterns)

GSI4-GSI9: (document as used)
*/
```

---

## Appendix A: Quick Reference Card

```
╔════════════════════════════════════════════════════════════════════╗
║                    LESSER INDEX NAMING RULES                        ║
╠════════════════════════════════════════════════════════════════════╣
║ ✓ CORRECT                        ✗ WRONG                          ║
╠════════════════════════════════════════════════════════════════════╣
║ index:gsi1                       index:GSI1                        ║
║ index:gsi2                       index:gsi2-index                  ║
║ Index("gsi1")                    Index("username-search-index")    ║
║ attr:ttl                         attr:TTL                          ║
║ attr:oauthClientsPK              attr:OAuthClientsPK               ║
║                                                                     ║
║ CDK: IndexName = "gsi1"          CDK: IndexName = "GSI1"           ║
║ CDK: ttl (lowercase)             CDK: TTL (uppercase)              ║
╚════════════════════════════════════════════════════════════════════╝
```

---

## Appendix B: Verification Checklist

- [ ] CDK uses lowercase `gsi1-gsi9` for index names
- [ ] CDK uses `ttl` (lowercase) for TTL attribute
- [ ] CDK uses `oauthClientsPK`/`oauthClientsSK` (camelCase)
- [ ] All model `index:` tags use lowercase `gsi1-gsi9`
- [ ] All repository `.Index()` calls use lowercase `gsi1-gsi9`
- [ ] Helper code produces `gsi1PK` not `GSI1PK`
- [ ] No semicolons in DynamORM struct tags
- [ ] `gsi_registry.go` created with documented access patterns
- [ ] All conflicting index names resolved
- [ ] All non-existent indexes provisioned or remapped
- [ ] All tests pass after changes
- [ ] CDK deployment succeeds
- [ ] Verified with `aws dynamodb describe-table` in lab environment

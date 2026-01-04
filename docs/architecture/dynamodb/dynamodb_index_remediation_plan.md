# DynamoDB Index Remediation Plan

> **Status**: Draft v3  
> **Created**: 2025-12-18  
> **Revised**: 2025-12-19 (Completed remaining `.Index("...")` remediation, refreshed AuthRefreshToken queries, updated verification notes)  
> **Compatibility**: No backward compatibility required (no install base)

This document provides a comprehensive remediation plan for fixing DynamoDB indexing issues identified in the Lesser codebase. Since there is no existing install base, all changes can be made without migration considerations.

---

## Progress Snapshot (In-Repo)

This repo already contains significant remediation work. At a high level:

- ✅ CDK now provisions lowercase GSIs (`gsi1..gsi9`), uses `ttl` as the table TTL attribute, and uses `oauthClientsPK/oauthClientsSK` for the dedicated OAuth index.
- ✅ Monitoring now tracks `gsi1..gsi9` (CloudWatch dimensions must match the physical `IndexName` exactly).
- ✅ Helper code that takes an index name now normalizes it to lowercase before deriving `gsi<N>PK/gsi<N>SK` and before calling `.Index(...)` (prevents `GSI1PK` and `Index("GSI1")`).
- ✅ TTL struct tags are standardized to `dynamorm:"ttl,attr:ttl"` and the semicolon-based DynamORM tag has been removed from `AuthRefreshToken`.
- ✅ All repository index usage now uses canonical index names (`gsi1..gsi9`, plus `oauth-clients-index`), including index names passed via helper configs/options.
- ✅ AuthRefreshToken queries now use `gsi1/gsi2` and align with the redesigned key schema (`gsi*SK` is `{createdAt}`).

Remaining work is mostly validation-oriented (run the verification sweep in Appendix B and validate in an actual AWS environment).

---

## Executive Summary

The Lesser DynamoDB design uses 9 generic GSIs (`gsi1-gsi9`, historically referred to as `GSI1-GSI9`) with overloaded key prefixes. However, the implementation has several critical issues:

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
Where(fmt.Sprintf("%sPK", strings.ToLower(indexName)), "=", gsiPK)

// base_repository.go:694, 698, 1396
attrPrefix := strings.ToLower(indexName)
skField := fmt.Sprintf("%sSK", attrPrefix)
Where(fmt.Sprintf("%sPK", attrPrefix), "=", pk)
```

Before applying the lowercasing fix, passing `indexName="GSI1"` would incorrectly derive `GSI1PK/GSI1SK` (the actual key attributes are `gsi1PK/gsi1SK`).

**Correct Fix**:
- ✅ Derive attribute names from a **lowercased** prefix (so `GSI1` and `gsi1` both map to `gsi1PK/gsi1SK`)
- ✅ Still standardize the actual DynamoDB `IndexName` and all `.Index("...")` calls to the physical lowercase names (`gsi1..gsi9`) because DynamoDB requires exact matches

### DynamoDB TTL: Table Can Only Have One TTL Attribute

The main table can only have **one** TTL attribute configured. Historically, the codebase used **both**:
- `ttl` (common) via `dynamorm:"ttl,attr:ttl"`
- `expiresAt` (also common) via `dynamorm:"ttl,attr:expiresAt"`

If the table TTL attribute is configured as `ttl`, then any model that marks TTL on `expiresAt` will **stop expiring automatically** until corrected.

**Correct Fix Direction (DONE)**
- ✅ Standardize on a single TTL attribute for the table (`ttl`)
- ✅ Ensure all models use `ttl,attr:ttl` and write the unix timestamp to that attribute

---

## Part 1: Infrastructure Fixes (CDK)

### 1.1 TTL Attribute - Ensure CDK Uses `ttl` (DONE)

**CDK** (`infra/cdk/stacks/lesser_api_stack.go:277`):
```go
TimeToLiveAttribute: jsii.String("ttl"),  // ✅ Matches DynamORM tags
```

**Models use** (e.g., `pkg/storage/models/device.go:40`):
```go
TTL int64 `dynamorm:"ttl,attr:ttl" json:"ttl,omitempty"`  // ✅ Lowercase
```

**File**: `infra/cdk/stacks/lesser_api_stack.go:277`

---

### 1.2 OAuth Client Index - Ensure CDK Uses `oauthClientsPK/oauthClientsSK` (DONE)

**CDK** (`infra/cdk/stacks/lesser_api_stack.go:332-337`):
```go
PartitionKey: &awsdynamodb.Attribute{
    Name: jsii.String("oauthClientsPK"),  // ✅ Matches model tags
    Type: awsdynamodb.AttributeType_STRING,
},
SortKey: &awsdynamodb.Attribute{
    Name: jsii.String("oauthClientsSK"),  // ✅ Matches model tags
    Type: awsdynamodb.AttributeType_STRING,
},
```

**Model uses** (`pkg/storage/models/oauth_client.go:18-19`):
```go
OAuthClientsPK string `dynamorm:"index:oauth-clients-index,pk,attr:oauthClientsPK"`  // ✅ camelCase
OAuthClientsSK string `dynamorm:"index:oauth-clients-index,sk,attr:oauthClientsSK"`  // ✅ camelCase
```

**File**: `infra/cdk/stacks/lesser_api_stack.go:332-337`

---

### 1.3 GSI Names - Ensure CDK Uses Lowercase `gsi1..gsi9` (DONE)

**CDK** (`infra/cdk/stacks/lesser_api_stack.go:309-310`):
```go
IndexName: jsii.String(fmt.Sprintf("gsi%d", i)),  // ✅ Lowercase
```

**Rationale**: The physical `IndexName` must match what callers pass to `.Index("...")` and what CloudWatch dashboards/alarms use.

**File**: `infra/cdk/stacks/lesser_api_stack.go:310` (and line 556 for GSI9)

### 1.4 Monitoring Stack Must Track Lowercase GSI Names (DONE)

`infra/cdk/stacks/monitoring_stack.go:405` defines a list of `gsi1..gsi9` for CloudWatch metrics.

If the physical index names change, you must update monitoring to match exactly; otherwise dashboards/alarms won’t populate.

---

## Part 2: Index Name Standardization

### 2.1 The Standard: Lowercase `gsi1-gsi9`

All DynamoDB index names and code references should be standardized to **lowercase** `gsi1..gsi9` to match CDK and avoid drift. Helper code should derive key attribute names from a **lowercased** prefix (`gsi1` → `gsi1PK/gsi1SK`).

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

### 2.2 Index Alias Inventory (Partial, Code-Derived)

This section is intentionally not exhaustive; it exists to show the scope of aliasing and provide examples. For a broader inventory, search the repo for `Index("...")` and `dynamorm:"index:...`.

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

#### Non-Existent Indexes (Used But Never Provisioned) (RESOLVED)

Earlier in remediation, several repositories referenced descriptive index names that had no corresponding physical GSI. All of those call sites have now been remapped to canonical `gsiN` names.

Verification (expect zero matches in non-test code):
```bash
rg --pcre2 'Index\("(?!gsi[1-9]"|oauth-clients-index"|test-index")[^"]+"\)' pkg cmd -g'*.go'
```

#### Index Names Used In Queries But Not Declared In Any Model (RESOLVED)

After remediation, runtime `.Index("...")` call sites use the canonical index names (`gsi1..gsi9`, plus `oauth-clients-index` where applicable). This eliminates drift between code and the physical DynamoDB `IndexName`.

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
| `pkg/storage/models/password_reset.go` | `gsi1PK/gsi1SK` | Legacy password reset tokens (unused in passwordless flow) |
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
| `pkg/storage/repositories/user_repository.go` | 961 | Special handling: `strings.ToLower(gsiIndex[:4])` |
| `pkg/storage/repositories/notification_helpers.go` | 130-131 | `gsi%sPK` (extracts number from end) |

### 4.2 `notification_helpers.go` Index Field Naming (FIXED)

```go
// Line 129-131
gsiNumber := gsiIndex[len(gsiIndex)-1:]  // Extracts "1" from "gsi1" or "GSI1"
gsiPKField := fmt.Sprintf("gsi%sPK", gsiNumber)  // Produces "gsi1PK" ✅
gsiSKField := fmt.Sprintf("gsi%sSK", gsiNumber)  // Produces "gsi1SK" ✅
```

This avoids generating `GSI1PK/GSI1SK` when the physical key attributes are `gsi1PK/gsi1SK`.

### 4.3 How `user_repository.go:961` Works

```go
Where(fmt.Sprintf("%sPK", strings.ToLower(gsiIndex[:4])), "=", ...)
// If gsiIndex starts with "gsi1", this produces "gsi1PK" ✅
```

Note: callers should still pass the canonical index name to `.Index(...)` (`gsi1`, not `gsi1-index`).

---

## Part 5: Invalid DynamORM Tag Syntax

### 5.1 AuthRefreshToken Semicolon Tags

**File**: `pkg/storage/models/auth_refresh_token.go:21`
```go
CreatedAtSK string `dynamorm:"index:user-index,sk,attr:createdAtSK;index:family-index,sk;index:user-family-index,sk" json:"-"`
```

**Problem**: DynamORM doesn't support semicolon-separated multiple index tags.

**Fix (DONE)**: These indexes don't exist anyway. Redesign to use real GSIs:
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

### Phase 1: CDK Fixes (DONE — Deploy First)

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

### Phase 2: Helper Code Fixes (DONE)

Normalize derived attribute names to use a lowercased prefix (e.g., `strings.ToLower(indexName)`), so callers never generate `GSI1PK/GSI1SK` when the physical attributes are `gsi1PK/gsi1SK`.

At minimum, audit helpers that build conditions like `fmt.Sprintf("%sPK", ...)` / `fmt.Sprintf("%sSK", ...)` in:
- `pkg/storage/repositories/base_repository.go`
- `pkg/storage/repositories/query_utils.go`
- `pkg/storage/repositories/notification_helpers.go`
- `pkg/storage/repositories/user_repository.go`

### Phase 3: Model Index Tag Standardization (DONE)

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

### Phase 4: Repository `.Index()` Call Fixes (DONE)

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

1. **Descriptive index names** - Mapped to canonical `gsiN` names based on which attributes they use (DONE)
2. **AuthRefreshToken model** - Redesign indexes (DONE)
3. **Conflicting index names** - Resolve `service-index` and `token-index` (DONE)
4. **Non-existent indexes** - Either provision them or map to existing GSIs (DONE)

### Phase 6: Validation

```bash
# 1. Verify no uppercase GSI index names remain
rg -F 'index:GSI' pkg/storage/models -g'*.go'
# Should return NO results

rg -F 'Index("GSI' pkg/storage/repositories -g'*.go'
# Should return NO results

# No helper/config call sites passing legacy "GSI#" strings (non-test code)
rg -n '"GSI[1-9]"' pkg/storage/repositories -g'*.go' --glob '!**/*_test.go'
# Should return NO results

# 2. Verify no descriptive index names remain (except oauth-clients-index)
rg 'index:.*-index' pkg/storage/models -g'*.go' | rg -v 'oauth-clients-index'
# Should return NO results

# 3. Verify CDK uses lowercase
rg -F 'IndexName: jsii.String("GSI' infra/cdk -g'*.go'
rg -F 'fmt.Sprintf("GSI' infra/cdk -g'*.go'
# Should return NO results

# 4. Compile and run tests
mkdir -p tmp/gocache
GOCACHE=$(pwd)/tmp/gocache go build ./...
STAGE=test JWT_SECRET=test GOCACHE=$(pwd)/tmp/gocache go test ./pkg/storage/models ./pkg/storage/repositories
```

---

## Part 7: GSI Registry Constants

Use a central file to document and enforce GSI usage:

**File**: `pkg/storage/models/gsi_registry.go`

Prefer using these constants (e.g., `models.IndexGSI1`) over ad-hoc string literals in repositories and services.

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

- [x] CDK uses lowercase `gsi1-gsi9` for index names
- [x] CDK uses `ttl` (lowercase) for TTL attribute
- [x] CDK uses `oauthClientsPK`/`oauthClientsSK` (camelCase)
- [x] All model `index:` tags use lowercase `gsi1-gsi9`
- [x] All repository `.Index()` calls use lowercase `gsi1-gsi9`
- [x] Helper code produces `gsi1PK` not `GSI1PK`
- [x] No semicolons in DynamORM struct tags
- [x] `gsi_registry.go` created with documented access patterns
- [x] All conflicting index names resolved
- [x] All non-existent indexes provisioned or remapped
- [ ] All tests pass after changes
- [ ] CDK deployment succeeds
- [ ] Verified with `aws dynamodb describe-table` in lab environment

### Scripts / Validation Sweep Notes
Use these commands to confirm legacy index styles are fully removed (expect zero matches unless noted):
```bash
# No uppercase GSI tags in model struct tags
rg "index:GSI\\d" pkg/storage -g'*.go'

# No Index(\"GSI*\") usage (Go code uses method-chaining where the leading '.' is often on the previous line)
rg "Index\\(\\\"GSI\\d\\\"\\)" pkg -g'*.go' cmd -g'*.go'

# No legacy \"-index\" aliases (allow oauth-clients-index only)
rg "Index\\(\\\"[^\\\"]+-index\\\"\\)" pkg cmd infra -g'*.go' | rg -v "oauth-clients-index"

# No repository configs/helper calls still referencing legacy \"*-index\" aliases (allow oauth-clients-index and test-index)
rg -n '"[a-z0-9-]+-index"' pkg/storage/repositories -g'*.go' | rg -v 'oauth-clients-index|test-index'

# No uppercase TTL tags or CDK TTL casing
rg "attr:TTL|TimeToLiveAttribute:\\s*jsii.String\\(\\\"TTL\\\"\\)" pkg infra -g'*.go'

# No semicolons inside struct tags (DynamORM doesn't support semicolon-separated tags)
rg '`[^`]*;[^`]*`' pkg/storage/models -g'*.go'

# CDK IndexName strings are lowercase
rg "IndexName:\\s*jsii.String\\(\\\"GSI\\d\\\"\\)|fmt\\.Sprintf\\(\\\"GSI%d\\\"\\)" infra/cdk -g'*.go'

# Post-change sanity
mkdir -p tmp/gocache
go vet ./...
GOCACHE=$(pwd)/tmp/gocache go build ./...
STAGE=test JWT_SECRET=test GOCACHE=$(pwd)/tmp/gocache go test ./...
```

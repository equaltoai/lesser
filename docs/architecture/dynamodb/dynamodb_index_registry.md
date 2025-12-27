# Lesser DynamoDB Index Registry (Offline, Code-Derived)

This document inventories DynamoDB indexes in the `lesser` codebase and describes how they are *intended* to be used, plus the gaps/risks visible **in-repo**.

**Important constraints**
- This is an **offline** analysis: derived from `infra/cdk/**` and Go code under `pkg/storage/**`. It does **not** reflect any AWS-side drift unless you confirm it.
- DynamoDB requires an **exact** `IndexName`. If code calls `.Index("<name>")` and that index does not exist on the table, DynamoDB returns a `ValidationException` (there is no aliasing). Some code paths that *don’t* use an index may degrade into **Scan + Filter**, but do not rely on “fallback” behavior for mismatched index names.

---

## 1) Tables (CDK Source of Truth)

### 1.1 Main Table: `lesser-{environment}`
Defined in `infra/cdk/stacks/lesser_api_stack.go:265`.
- **PK**: `PK` (string)
- **SK**: `SK` (string)
- **Streams**: `NEW_AND_OLD_IMAGES`
- **TTL attribute (CDK)**: `TTL` (`infra/cdk/stacks/lesser_api_stack.go:277`)

### 1.2 Rate Limit Table: `lesser-rate-limits-{environment}`
Defined in `infra/cdk/stacks/lesser_api_stack.go:287`.
- **PK**: `PK` (string)
- **SK**: `SK` (string)
- **TTL attribute (CDK)**: `ExpiresAt` (`infra/cdk/stacks/lesser_api_stack.go:298`)

---

## 2) Physical Indexes (CDK-Provisioned)

### 2.1 Generic GSIs (Main Table)
CDK provisions 9 generic GSIs intended to be “overloaded” by multiple item types/prefixes:

| Index (CDK) | Partition Key Attr | Sort Key Attr | Notes |
|---|---|---|---|
| `GSI1` | `gsi1PK` | `gsi1SK` | created in loop `infra/cdk/stacks/lesser_api_stack.go:309` |
| `GSI2` | `gsi2PK` | `gsi2SK` | created in loop `infra/cdk/stacks/lesser_api_stack.go:309` |
| `GSI3` | `gsi3PK` | `gsi3SK` | created in loop `infra/cdk/stacks/lesser_api_stack.go:309` |
| `GSI4` | `gsi4PK` | `gsi4SK` | created in loop `infra/cdk/stacks/lesser_api_stack.go:309` |
| `GSI5` | `gsi5PK` | `gsi5SK` | created in loop `infra/cdk/stacks/lesser_api_stack.go:309` |
| `GSI6` | `gsi6PK` | `gsi6SK` | created in loop `infra/cdk/stacks/lesser_api_stack.go:309` |
| `GSI7` | `gsi7PK` | `gsi7SK` | created in loop `infra/cdk/stacks/lesser_api_stack.go:309` |
| `GSI8` | `gsi8PK` | `gsi8SK` | created in loop `infra/cdk/stacks/lesser_api_stack.go:309` |
| `GSI9` | `gsi9PK` | `gsi9SK` | added separately `infra/cdk/stacks/lesser_api_stack.go:554` |

### 2.2 Dedicated OAuth Client Pagination Index (Main Table)
Defined in `infra/cdk/stacks/lesser_api_stack.go:329`.

| Index (CDK) | Partition Key Attr | Sort Key Attr | Intended Use |
|---|---|---|---|
| `oauth-clients-index` | `OAuthClientsPK` | `OAuthClientsSK` | global listing of OAuth clients newest-first |

---

## 3) What The Go Code Indicates About Indexing

### 3.1 Key finding: **Index-name explosion collapses into ~9 key schemas**

From `pkg/storage/models/**` struct tags, there are many distinct `index:<name>` values, but they mostly collapse into the same handful of key-attribute pairs:

| Key Schema (attr pair) | Distinct `index:<name>` values in model tags |
|---|---:|
| `gsi1PK/gsi1SK` | 27 |
| `gsi2PK/gsi2SK` | 21 |
| `gsi3PK/gsi3SK` | 9 |
| `gsi4PK/gsi4SK` | 5 |
| `gsi5PK/gsi5SK` | 2 |
| `gsi6PK/gsi6SK` | 2 |
| `gsi7PK/gsi7SK` | 1 |
| `gsi8PK/gsi8SK` | 2 |
| `oauthClientsPK/oauthClientsSK` | 1 |

**Why this matters**
- DynamoDB does **not** provide aliases: each physical index has one name.
- DynamORM also has **no global alias map**: index names must align end-to-end (CDK ↔ DynamoDB ↔ model tags ↔ repository `.Index(...)` calls) to get efficient queries.

---

## 4) Index Registry By “Slot” (Key Schema)

To make this actionable, the rest of this document treats each `(pkAttr, skAttr)` pair as a **slot**. A single slot is expected to correspond to *one physical DynamoDB GSI* (e.g., CDK’s `GSI1`), but the codebase currently uses many **index-name variants** for the same slot.

### 4.1 Slot `gsi1PK/gsi1SK` (CDK: `GSI1`)

**Index-name variants found in model tags** (27):
`GSI1`, `connection-index`, `endpoint-index`, `error-index`, `gsi1`, `gsi1-index`, `hashtag-trending-history`, `job-status-index`, `post-timeline-index`, `provider-index`, `route-time-index`, `search-cache-cleanup`, `spending-time-index`, `status-date-index`, `status-hashtag-index`, `stream-target-index`, `table-index`, `time-index`, `transaction-time-index`, `type-index`, `user-agg-index`, `user-budget-index`, `user-csrf-index`, `user-jobs-index`, `user-media-index`, `user-sessions-index`, `username-search-index`.

**Index-name variants used in `.Index("...")` calls (string literals)**:
`GSI1`, `error-index`, `gsi1`, `gsi1-index`, `job-status-index`, `post-timeline-index`, `provider-index`, `status-date-index`, `status-hashtag-index`, `table-index`, `time-index`, `type-index`, `user-csrf-index`, `user-sessions-index`, `username-search-index`.

**Core access patterns (examples pulled from model comments)**
- Inbox activities: `Activity` uses `gsi1PK="INBOX#{username}"` (`pkg/storage/models/activity.go:20`)
- Actor username search: `Actor` uses `gsi1PK="USERNAME_SEARCH#{first_2_chars}"` (`pkg/storage/models/actor.go:21`)
- Timeline public queries: `TimelineEntry` uses `gsi1PK="TIMELINE#PUBLIC#{local/federated}"` (`pkg/storage/models/timeline_entry.go:18`)
- Relationship inversion (followers): `RelationshipRecord` uses `gsi1PK="FOLLOW#{followedUsername}"` (`pkg/storage/models/relationship.go:15`)

**Gaps / risks**
- **Multiple index names for one key schema**: you cannot have 27 physical indexes all keyed by `gsi1PK/gsi1SK` (DynamoDB GSI limit is 20, and duplicates are wasteful). Pick **one** index name for this slot and standardize.
- **Mixed-case naming**: both `GSI1` and `gsi1` appear in model tags and query calls; these are different names to DynamoDB.
- **Missing `index:` tags**: `AuthAuditLog` sets `gsi1PK/gsi1SK` but does not declare any `index:` tags (`pkg/storage/models/audit_log.go:52`), so DynamORM can’t treat them as key conditions.

### 4.2 Slot `gsi2PK/gsi2SK` (CDK: `GSI2`)

**Index-name variants in model tags** (21):
`GSI2`, `actor-index`, `actor-timeline-index`, `admin-index`, `aggregate-index`, `cost-category-index`, `cost-variant-index`, `domain-route-index`, `gsi2`, `gsi2-index`, `hashtag-visibility-index`, `job-date-index`, `media-jobs-index`, `name-search-index`, `operation-type-index`, `retry-index`, `state-index`, `status-index`, `stream-type-index`, `trending-by-period`, `user-providers-index`.

**Index-name variants used in `.Index("...")` calls (string literals)**:
`GSI2`, `actor-index`, `admin-index`, `aggregate-index`, `gsi2`, `gsi2-index`, `hashtag-visibility-index`, `job-date-index`, `name-search-index`, `operation-type-index`, `retry-index`, `state-index`, `user-providers-index`.

**Core access patterns (examples)**
- Actor display-name search: `Actor` uses `gsi2PK="NAME_SEARCH#{first_2_chars}"` (`pkg/storage/models/actor.go:25`)
- Devices by trust level: `Device` uses `gsi2PK="TRUST_LEVEL#{trustLevel}"` (`pkg/storage/models/device.go:21`)
- Relationship domain aggregations (per docs): see `RelationshipRecord`/CDK comments (`infra/cdk/stacks/lesser_api_stack.go:319`)

**Gaps / risks**
- Same pattern as slot 1: many “logical” index names map to the same `gsi2PK/gsi2SK` schema; choose one physical name and standardize.

### 4.3 Slot `gsi3PK/gsi3SK` (CDK: `GSI3`)

**Index-name variants in model tags** (9):
`GSI3`, `content-type-index`, `cost-analysis-index`, `domain-index`, `group-index`, `gsi3`, `hashtag-search-index`, `route-performance-index`, `visibility-timeline-index`.

**Index-name variants used in `.Index("...")` calls (string literals)**:
`GSI3`, `cost-analysis-index`, `domain-index`, `group-index`, `gsi3`, `hashtag-search-index`.

**Core access patterns (examples)**
- Actor by domain: `Actor` uses `gsi3PK="DOMAIN#{domain}"` (`pkg/storage/models/actor.go:29`)
- Hashtag search: `Hashtag` uses `gsi3PK="HASHTAG_SEARCH#{first_2_chars}"` (`pkg/storage/models/hashtag.go:18`)

**Gaps / risks**
- Slot 3 also shows cross-domain naming collisions; see “Ambiguous index names” below.

### 4.4 Slot `gsi4PK/gsi4SK` (CDK: `GSI4`)

**Index-name variants in model tags** (5):
`GSI4`, `cost-date-index`, `gsi4`, `language-timeline-index`, `popularity-index`.

**Index-name variants used in `.Index("...")` calls (string literals)**:
`GSI4`, `popularity-index`.

**Core access patterns (examples)**
- Actor popularity rank: `Actor` uses `gsi4PK="ACTOR_RANK#{bucket}"` (`pkg/storage/models/actor.go:33`)
- Status replies: `Status` uses `gsi4PK="REPLIES#{parent_status_id}"` (`pkg/storage/models/status.go:41`)

### 4.5 Slot `gsi5PK/gsi5SK` (CDK: `GSI5`)

**Index-name variants in model tags** (2):
`GSI5`, `activity-index`.

**Index-name variants used in `.Index("...")` calls (string literals)**:
`GSI5`, `activity-index`.

**Core access patterns (examples)**
- Actor recent activity: `Actor` uses `gsi5PK="ACTIVE#{date}"` (`pkg/storage/models/actor.go:37`)
- Status hashtags: `Status` uses `gsi5PK="HASHTAG#{hashtag}"` (`pkg/storage/models/status.go:45`)

### 4.6 Slot `gsi6PK/gsi6SK` (CDK: `GSI6`)

**Index-name variants in model tags** (2):
`GSI6`, `gsi6-index`.

**Index-name variants used in `.Index("...")` calls (string literals)**:
`gsi6-index`.

**Observed gap**
- `.Index("gsi6-index")` is used in repositories (string-literal scan), but `.Index("GSI6")` does not appear as a string literal. If CDK is correct, this is a naming mismatch to resolve.

### 4.7 Slot `gsi7PK/gsi7SK` (CDK: `GSI7`)

**Index-name variants in model tags** (1): `GSI7`
- Status by URL: `Status` uses `gsi7PK="URL#{normalized_url}"` (`pkg/storage/models/status.go:53`)

**Index-name variants used in `.Index("...")` calls (string literals)**:
`GSI7`.

### 4.8 Slot `gsi8PK/gsi8SK` (CDK: `GSI8`)

**Index-name variants in model tags** (2): `GSI8`, `gsi8`

**Index-name variants used in `.Index("...")` calls (string literals)**:
`gsi8`.

**Observed gap**
- Repositories appear to call `.Index("gsi8")` as a string literal, not `.Index("GSI8")`. If CDK is correct, this is a naming mismatch to resolve.

### 4.9 Slot `gsi9PK/gsi9SK` (CDK: `GSI9`)

**CDK provisions it, but code does not (currently) use it**
- No `gsi9PK/gsi9SK` fields found in `pkg/storage/models/**`.
- `infra/cdk/config/*.yaml` references “model metadata (using GSI9)”; docs also mention planned usage (`docs/MODERATION_ML_ARCHITECTURE.md:160`).

---

## 5) Dedicated Index: `oauth-clients-index`

**Model usage**
- `OAuthClient` uses `index:oauth-clients-index` with attributes `oauthClientsPK/oauthClientsSK` (`pkg/storage/models/oauth_client.go:18`).

**Repository usage**
- `AccountRepository` uses a constant `oauthClientsIndexName = "oauth-clients-index"` and calls `.Index(oauthClientsIndexName)` (`pkg/storage/repositories/account_repository_oauth.go:25`, `pkg/storage/repositories/account_repository_oauth.go:450`).

**Gap**
- CDK defines the index key attributes as `OAuthClientsPK/OAuthClientsSK` (capitalized) but the model writes `oauthClientsPK/oauthClientsSK` (camelCase) (`infra/cdk/stacks/lesser_api_stack.go:332`, `pkg/storage/models/oauth_client.go:18`).
  - If the CDK stack is the deployed truth, this breaks queries/writes for that index.
  - If AWS is correct today, CDK is out of sync.

---

## 6) Index Gaps To Fix (High Confidence, Code-Visible)

### 6.1 TTL attribute mismatch (expiration won’t work as intended)
- CDK TTL attribute is `TTL` (`infra/cdk/stacks/lesser_api_stack.go:277`).
- Some models write TTL to `ttl` (lowercase), e.g. `TimelineEntry` (`pkg/storage/models/timeline_entry.go:45`) and `AuthRefreshToken` (`pkg/storage/models/auth_refresh_token.go:30`).
- Some models mark TTL on `expiresAt` (`dynamorm:"ttl,attr:expiresAt"`), e.g. `Session` (`pkg/storage/models/session.go:54`) and `Notification` (`pkg/storage/models/notification.go:70`).
  - DynamoDB supports **one TTL attribute per table**. Even after you fix CDK away from `TTL`, you still need to pick one (`ttl` *or* `expiresAt`) and standardize models accordingly.

### 6.2 Ambiguous index names (same name used with different key schemas)
These index names cannot correspond to a single DynamoDB GSI without an alias layer (none exists globally in DynamORM):
- `service-index`: used with `gsi1PK/gsi1SK` in `pkg/storage/models/metrics.go:21`, and with `gsi3PK/gsi3SK` in `pkg/storage/models/dlq_message.go:29`.
- `token-index`: used with `gsi1PK/gsi1SK` (`pkg/storage/models/password_reset.go:16`) and with `gsi2PK/gsi2SK` (`pkg/storage/models/session.go:28`).

### 6.3 Invalid DynamORM tags (won’t register)
- `AuthRefreshToken.CreatedAtSK` uses semicolons to try to attach multiple index tags in one struct tag (`pkg/storage/models/auth_refresh_token.go:21`). DynamORM tag parsing does not support this; the model likely cannot be registered.

### 6.4 “Index helper” code assumes indexName→attributeName conventions
Some helper methods build attribute names like `fmt.Sprintf("%sPK", indexName)` (e.g., `pkg/storage/repositories/query_utils.go:166`, `pkg/storage/repositories/query_utils.go:352`). This only works if:
- the **index name** also matches the **attribute prefix** (e.g., `gsi1` → `gsi1PK`), and
- the model is a registered struct (not `map[string]any`), otherwise DynamORM errors during registration.

This is a major consistency hazard when the codebase also uses index names like `gsi1-index`, `username-search-index`, etc.

### 6.5 Index names used in queries with unknown/ambiguous slot mapping

From a static scan of string-literal `.Index("...")` calls, these index names are either:
- **not declared** in `pkg/storage/models/**` (no `dynamorm:"index:<name>,..."` tags found), or
- **declared with conflicting key schemas** (cannot map to exactly one `(pkAttr, skAttr)` pair).

List (string-literal scan):
`date-index`, `display-name-index`, `email-index`, `family-tokens-index`, `follower-count-index`, `gsi3-index`, `gsi4-index`, `gsi5`, `hashtag-timeline-index`, `list-timeline-index`, `local-timeline-index`, `name-index`, `service-index`, `tenant-entity`, `test-index`, `token-index`, `user-credentials-index`, `user-tokens-index`, `user-votes-index`, `users-by-role`, `webfinger-index`.

**Follow-up**
- For each of these, decide whether it is (a) a legacy index name to retire, (b) a real index name that must exist in DynamoDB, or (c) a place to standardize to `GSI<N>`.

---

## 7) Recommended Standardization Strategy

Pick one of these and enforce it everywhere (CDK + models + repositories):

### Option A: Keep physical `GSI1..GSI9` (least AWS churn)
- Keep CDK index names as `GSI1..GSI9`.
- Standardize all model tags and repository `.Index(...)` calls to `GSI<N>` names.
- Fix helper code that derives attribute names from `IndexName` (e.g., `fmt.Sprintf("%sPK", indexName)`) so `GSI1` maps to `gsi1PK/gsi1SK` (not `GSI1PK/GSI1SK`).

### Option B: Rename physical GSIs to `gsi1..gsi9` (simplest code conventions, requires index recreation)
- Update CDK to name indexes `gsi1..gsi9` so the index name matches the attribute prefix (`gsi1PK/gsi1SK`).
- Standardize all model tags and repository `.Index(...)` calls to `gsi<N>` names.
- This is the direction taken in `docs/dynamodb_index_remediation_plan.md`, but it will force CloudFormation to **recreate** GSIs (and may replace the table depending on settings).

---

## 8) How To Confirm AWS Reality (User-Run, No Agent AWS Access)

If you can run AWS CLI locally, paste the `GlobalSecondaryIndexes` block into an issue or doc for reconciliation:

```bash
aws dynamodb describe-table --table-name lesser-<environment> \\
  --query 'Table.GlobalSecondaryIndexes[*].{IndexName:IndexName,KeySchema:KeySchema,Projection:Projection}'
```

Also confirm TTL attribute configured on the table:

```bash
aws dynamodb describe-time-to-live --table-name lesser-<environment>
```

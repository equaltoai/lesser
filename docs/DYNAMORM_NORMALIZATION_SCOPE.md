# DynamORM Model Normalization Scope

## Executive Summary

**Current State**: 79.6% of model fields (4,033 out of 5,065) lack proper DynamoDB attribute mapping tags, causing unmarshaling failures.

**Root Cause**: Fields have only `json:"snake_case"` tags, but DynamoDB stores attributes in `camelCase`. DynamORM's unmarshaler skips fields without `dynamodb` or `dynamorm` tags.

**Impact**: Critical data loss during unmarshaling - fields like `AuthorUsername` come back empty even though DynamoDB contains `authorUsername="greater"`.

## Problem Definition

### DynamORM Best Practices (from team guidance)
- **Every non-key field should have explicit attribute mapping** when DB schema differs from default
- Use `dynamodb:"attributeName"` tag to mirror DynamoDB field names
- Keep `json:"field_name"` for API serialization separate from DB mapping
- Pick one naming convention (camelCase vs snake_case) per model and stick to it

### Our Current Violation
```go
// BROKEN - Missing dynamodb tag
Username string `json:"username"`

// WORKING - Has dynamodb tag
AuthorUsername string `dynamodb:"authorUsername" json:"author_username"`
```

### Database Reality Check
From `/tmp/lesser-development-scan.json`:
- DynamoDB uses: `authorUsername`, `statusID`, `userID`, `actorID` (camelCase)
- Go structs expect: `author_username`, `status_id`, `user_id`, `actor_id` (snake_case in json tags)
- Result: **Unmarshal silently skips 79.6% of fields**

## Scope Analysis

### Overall Statistics
- **Total Model Files**: 181 files
- **Total Struct Definitions**: 344 structs
- **Total Tagged Fields**: 5,065 fields
- **Fields Needing Normalization**: 4,033 fields (79.6%)
- **Already Properly Tagged**: 1,032 fields (20.4% - mostly PK/SK/GSI keys)

### High-Impact Files (Top 20)

Files with most fields needing tags (for context, not prioritization):

| File | Affected Fields |
|------|----------------|
| `moderation.go` | 174 |
| `scheduled_job_cost_tracking.go` | 115 |
| `notification_cost_tracking.go` | 114 |
| `websocket_cost_tracking.go` | 114 |
| `federation_route_metrics.go` | 113 |
| `ai_cost.go` | 99 |
| `search_cost_tracking.go` | 99 |
| `federation.go` | 98 |
| `moderation_ml.go` | 92 |
| `federation_cost_tracking.go` | 88 |
| `enhanced_patterns.go` | 85 |
| `relay_cost.go` | 80 |
| `alert.go` | 63 |
| `federation_relationship.go` | 63 |
| `metrics.go` | 58 |
| `media_spending.go` | 57 |
| `cost_tracking.go` | 56 |
| `export_cost_tracking.go` | 53 |
| `import_budget.go` | 51 |
| `federation_metrics.go` | 48 |

**All files processed alphabetically regardless of field count.**

## Implementation Strategy

### Systematic Approach: Process ALL Models Alphabetically

**Principle**: Every field is equally broken. Every unmarshaling failure causes silent data loss. No prioritization - fix everything methodically.

**Process**:
1. Sort all 181 model files alphabetically
2. Process each file completely before moving to next
3. For each file:
   - Read struct definitions
   - For each field with `json:` tag but no `dynamodb:`/`dynamorm:` tag:
     - Add `dynamodb:"camelCaseName"` tag
   - Verify with test unmarshal
4. Track progress: completed files / 181 total

**No phases. No priorities. Complete coverage.**

### Automation Script

```bash
#!/bin/bash
# Process all model files systematically

MODEL_DIR="pkg/storage/models"
FILES=$(ls -1 "$MODEL_DIR"/*.go | grep -v "_test.go" | sort)
TOTAL=$(echo "$FILES" | wc -l)
COUNT=0

for FILE in $FILES; do
    COUNT=$((COUNT + 1))
    echo "[$COUNT/$TOTAL] Processing $(basename $FILE)..."
    
    # Add dynamodb tags to all json-tagged fields
    # Script will:
    # 1. Find fields with `json:"field_name"` but no `dynamodb:`
    # 2. Extract Go field name (CamelCase)
    # 3. Convert to camelCase for dynamodb tag
    # 4. Insert dynamodb:"camelCase" before json: tag
    
    # Manual review required for each file
done
```

### Field Tagging Pattern

For every field in every model:

```go
// BEFORE (all 4,033 instances)
FieldName string `json:"field_name"`

// AFTER (systematic fix)
FieldName string `dynamodb:"fieldName" json:"field_name"`
```

Rules:
- Go field name: `CamelCase` (first letter uppercase)
- DynamoDB attribute: `camelCase` (first letter lowercase)
- JSON field name: `snake_case` (all lowercase with underscores)

### Validation Per File

After updating each model file:
1. Scan DynamoDB for sample record of that model type
2. Parse attribute names from scan result
3. Verify all attribute names match new `dynamodb:` tags
4. Run test: Unmarshal → verify no empty fields
5. Mark file as complete

### Progress Tracking

```
Files Completed: 0 / 181
Fields Fixed: 0 / 4,033

Current File: account_features.go
Status: In Progress
Fields in this file: 12
```

## Naming Convention Decision

### Database Schema Analysis
From DynamoDB scan: Fields use **camelCase**
- `authorUsername` (not `author_username`)
- `statusID` (not `status_id`)
- `userID` (not `user_id`)

### Recommendation
```go
// CORRECT PATTERN for all models:
FieldName string `dynamodb:"fieldName" json:"field_name"`
```

**Rationale**:
- `dynamodb:"camelCase"` matches actual DB schema
- `json:"snake_case"` matches Mastodon API convention
- Keeps API compatibility while fixing DB unmarshaling

## Testing Strategy

### Per-Phase Validation
1. **Unit Tests**: Unmarshal DynamoDB items into structs, assert all fields populated
2. **Integration Tests**: Query DynamoDB, verify complete data retrieval
3. **Regression Tests**: Ensure existing features still work

### Automated Verification
```bash
# Check for fields missing dynamodb tags
grep -r '^\s*[A-Z][a-zA-Z0-9]*\s\+[^\s]\+\s\+`json:' pkg/storage/models/*.go | \
  grep -v 'dynamodb:' | \
  grep -v 'dynamorm:' | \
  wc -l
```

Should return 0 when complete.

## Effort Estimation

### Systematic Processing

**Total Work**:
- 181 model files
- 4,033 fields to tag
- ~22 fields per file average

**Rate Assumptions**:
- Review file structure: 2 min
- Add tags to 22 fields: 10 min
- Verify against DB sample: 3 min
- Test unmarshal: 2 min
- **Per file: ~17 minutes**

**Total Duration**: 181 files × 17 min = **51 hours** = **6.4 days**

### Batch Processing Optimization

Process in batches of 20 files:
- Batch 1-9: 180 files (9 batches × 20 files)
- Batch 10: 1 file (remainder)

**Per batch** (~5.7 hours):
1. Process 20 files systematically
2. Run batch validation tests
3. Deploy to dev environment
4. Smoke test for 30 min
5. Fix any issues discovered

**Total with batching**: ~10 days (includes breaks, reviews, testing)

### Assumptions
- Single engineer, methodical approach
- No shortcuts, complete coverage
- Validation at every step
- No work left incomplete

## Rollout Plan

### Systematic Batch Deployment

**Batch Size**: 20 files per deployment

**Per Batch Process**:
1. Complete all 20 files (tag all fields)
2. Run local tests for all 20 models
3. Commit with message: "fix: add dynamodb tags to batch N (files X-Y)"
4. Deploy to dev environment
5. Run integration tests
6. Monitor for 30 minutes
7. Deploy to production
8. Move to next batch

**No special treatment for any file.** Every model is processed completely before moving on.

### Rollback Strategy
- Each batch is independently deployable
- If batch N fails, revert that commit
- Previous batches remain deployed
- No database migration required (schema unchanged)

### Monitoring Per Batch
- CloudWatch: Check for unmarshaling errors
- DynamoDB metrics: Verify read patterns unchanged
- Application logs: Confirm data completeness
- Lambda duration: Watch for performance regression

## Risk Mitigation

### Potential Issues
1. **Breaking Changes**: If code relies on fields being empty (unlikely but possible)
   - Mitigation: Thorough integration testing before each phase
   
2. **Performance Impact**: More data unmarshaled per query
   - Mitigation: Monitor Lambda execution time, optimize if needed
   
3. **Schema Mismatches**: DB field name doesn't match camelCase assumption
   - Mitigation: Sample DynamoDB data before tagging each model

### Rollback Strategy
- Each phase is independently deployable
- If issues detected, revert specific model files
- No database migration required (schema unchanged)

## Success Metrics

### Technical Metrics
- ✅ 0 fields with json-only tags remaining
- ✅ 0 unmarshaling errors in CloudWatch logs
- ✅ All integration tests passing
- ✅ No regression in existing features

### Business Metrics
- ✅ Local timeline loads with complete data
- ✅ User profiles display all information
- ✅ Notifications delivered successfully
- ✅ Cost tracking reports accurate

## Maintenance Plan

### Going Forward
1. **Linting Rule**: Add `golangci-lint` rule to enforce dynamodb tags on all DynamoDB model fields
2. **Code Review**: PR template checklist includes "Added dynamodb tags to new fields"
3. **Documentation**: Update CONTRIBUTING.md with DynamORM tagging requirements
4. **Testing**: Integration tests verify no fields come back empty after queries

## DynamORM Normalization Rules

We aligned with the DynamORM team and now enforce a **zero-legacy policy**: once a file is touched, it must not contain any `dynamodb:"..."` tags or mixed conventions. Every DynamoDB model must follow these rules:

1. **Struct-level naming** – add a marker field with `dynamorm:"naming:camelCase"` (or `snake_case` where explicitly documented) so camelCase becomes the default for every attribute in the struct.
2. **Field-level attributes** – every persisted field, including PK/SK, GSI keys, TTL, version counters, etc., must declare `dynamorm:"...,attr:attributeName"`, matching the camelCase schema unless we have a documented exception.
3. **Embedded JSON-only payloads** – when a field stores a nested document wholesale (e.g., `Metadata MetadataPayload`), tag the parent with `attr:metadata` and leave the nested struct untagged unless DynamoORM projects its inner fields separately.
4. **Performance is validated** – we have already profiled the additional fields; always prefer completeness over selective hydration.

## Appendix: Canonical Example

```go
type Status struct {
    _ struct{} `dynamorm:"naming:camelCase"`

    PK string `dynamorm:"pk,attr:PK" json:"pk"`
    SK string `dynamorm:"sk,attr:SK" json:"sk"`

    AuthorUsername string         `dynamorm:"attr:authorUsername" json:"author_username"`
    Content        string         `dynamorm:"attr:content" json:"content"`
    Visibility     string         `dynamorm:"attr:visibility" json:"visibility"`
    LikeCount      int            `dynamorm:"attr:likeCount" json:"like_count"`
    Metadata       StatusMetadata `dynamorm:"attr:metadata" json:"metadata"` // Stored as a single attribute
}

type StatusMetadata struct {
    Attachments []StatusAttachment `json:"attachments"`
    Tags        []StatusTag        `json:"tags"`
}
```

Result: the struct advertises camelCase naming once, every persisted field declares its attribute, embedded structs stay JSON-only, and the file contains no `dynamodb:` tags.

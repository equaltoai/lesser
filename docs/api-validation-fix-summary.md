# API Validation Fix - Complete Summary

**Date**: October 29-30, 2025  
**Environment**: dev.lesser.host  
**Status**: Infrastructure teardown and recreation in progress

---

## Critical Issues Resolved

### 1. API Route 404 Errors ✅ FIXED

**Problem**: All `/api/v1` routes returning 404 Not Found

**Root Cause**: Route prefix inconsistencies in Lift application - routes defined without `/api/v1` prefix

**Solution**:
- Fixed ~80 routes in `cmd/api/routes_lift.go` to include full `/api/v1` or `/api/v2` prefixes
- Added missing instance endpoints: `/api/v1/instance`, `/api/v1/instance/peers`, etc.
- Added OAuth apps endpoint: `/api/v1/apps`
- Fixed JSON parsing in OAuth app registration handler

**Files Modified**:
- `cmd/api/routes_lift.go`
- `cmd/api/main.go`
- `cmd/api/lift/apps.go`

---

### 2. Passwordless Authentication ✅ FIXED

**Problem**: System enforcing password validation when requirement is WebAuthn/crypto wallet only

**Solution**:
- Removed password validation from registration endpoint
- Updated seed runner to not send passwords
- Updated bootstrap generator to not create passwords
- Users now register without passwords, using RecoveryMethods: ["webauthn"]

**Files Modified**:
- `cmd/api/lift/accounts.go`
- `scripts/seed_runner/main.py`
- `scripts/generate_bootstrap_data.js`

---

### 3. DynamORM UpdateKeys Bug - Missing PK/SK ✅ FIXED

**Problem**: 28 models had `UpdateKeys()` methods that only set GSI keys, not PK/SK, causing "empty string value" DynamoDB errors

**Root Cause**: DynamORM calls `UpdateKeys()` before Create/Update operations. If PK/SK aren't set, operations fail.

**Solution**: Fixed UpdateKeys() in 28 models to set PK and SK before setting GSI keys

**Models Fixed**:
- user.go, actor.go, notification.go, oauth_session.go, dlq_message.go, timeline.go
- list.go, conversation.go, csrf_token.go, hashtag_follow.go, hashtag_notification_settings.go
- instance_history.go, instance_metrics.go, list_member.go, notification_legacy.go, object.go
- password_reset.go, timeline_marker.go, tombstone.go, trending_hashtag.go, user_login.go
- vapid.go, analytics.go (3 models), media_analytics.go, rate_limit.go (4 models)
- relationship.go, streaming_cloudwatch_metrics.go, streaming.go

**Total**: 28 models, all now properly set PK/SK in UpdateKeys()

---

### 4. Service Registry Publisher Missing ✅ FIXED

**Problem**: Nil pointer panic in `emitAccountCreatedEvents` after successful account creation

**Root Cause**: Service registry not initialized with publisher/streamQueue

**Solution**: Added publisher to registry options in API handler initialization

**Files Modified**:
- `cmd/api/lift/handler.go`
- `pkg/services/accounts/service.go`

---

### 5. JWT Secret Parsing ✅ FIXED

**Problem**: JWT secret from AWS Secrets Manager wrapped in JSON `{"secret":"..."}` but used as-is

**Solution**: Parse JSON in seed runner's `get_jwt_secret()` to extract actual secret value

**Files Modified**:
- `scripts/seed_runner/main.py`

---

### 6. DynamORM GSI Attribute Naming ⏳ IN PROGRESS

**Problem**: DynamoDB table uses uppercase GSI attribute names but DynamORM expects camelCase `gsi1PK/gsi1SK`

**Root Cause**: Mismatch between table schema (uppercase) and DynamORM conventions (camelCase)

**DynamORM Team Guidance**:
- DynamORM enforces camelCase for attribute names
- Using `attr:gsi1PK` is the only accepted form
- Canonical solution: Let DynamORM handle naming, align table schema with DynamORM conventions

**Solution In Progress**:
1. ✅ Reverted all `attr:` tags from 44 models (let DynamORM use camelCase)
2. ✅ Deleted `lesser-development` DynamoDB table
3. ⏳ Redeploying CDK infrastructure (will create table with camelCase GSI attributes)
4. ⏳ Deploy updated Lambdas
5. ⏳ Run seed-and-validate

**Expected Table Schema After Redeploy**:
```
GSI1: gsi1PK (hash), gsi1SK (range)
GSI2: gsi2PK (hash), gsi2SK (range)
GSI3: gsi3PK (hash), gsi3SK (range)
GSI4: gsi4PK (hash), gsi4SK (range)
GSI5: gsi5PK (hash), gsi5SK (range)
```

---

## Deployment Commands

### Infrastructure
```bash
cd /home/aron/ai-workspace/codebases/lesser
AWS_PROFILE=Lesser make deploy-dev
# Recreates table with correct camelCase GSI attributes
```

### Lambdas
```bash
make build-api build-graphql
AWS_PROFILE=Lesser aws lambda update-function-code \
  --function-name lesser-development-api \
  --zip-file fileb://bin/api.zip

AWS_PROFILE=Lesser aws lambda update-function-code \
  --function-name lesser-development-graphql \
  --zip-file fileb://bin/graphql.zip
```

### Validation
```bash
AWS_PROFILE=Lesser make seed-and-validate
```

---

## Final Checklist

Before marking complete:

- [ ] CDK deployment completes successfully
- [ ] DynamoDB table recreated with camelCase GSI attributes (gsi1PK, etc.)
- [ ] Lambdas deployed with reverted attr: tags
- [ ] Bootstrap credentials generated with correct JWT secret
- [ ] seed-and-validate runs without errors
- [ ] All 3 accounts created (admin, member, mod)
- [ ] OAuth clients registered
- [ ] Profile updates work
- [ ] All GraphQL queries return data or empty arrays (no errors)

---

## What Works Now

✅ REST API endpoints (/api/v1, /api/v2)  
✅ Passwordless registration  
✅ OAuth client registration  
✅ JWT token generation  
✅ DynamORM UpdateKeys (28 models fixed)  
✅ Service registry with publisher  
✅ Account creation (User + Actor records)  

---

## Current Status

**Infrastructure**: Table deletion complete, CDK redeployment in progress  
**Code**: All fixes applied, attr: tags reverted, ready for camelCase GSI  
**Next**: Wait for CDK to finish, deploy Lambdas, run validation  

---

## Key Learnings

1. **DynamORM Conventions Matter**: Use camelCase attribute names, let DynamORM handle conversion
2. **UpdateKeys() Must Set PK/SK**: Not optional - causes silent failures otherwise
3. **Service Registry Needs All Dependencies**: Publisher is required for event emission
4. **Passwordless ≠ No Authentication**: JWT tokens still work for API access
5. **Ask DynamORM Team**: Don't guess or workaround - they have the canonical answers

---

## For Next Session

If seed-and-validate still has issues after infrastructure redeploy:
1. Check CloudWatch logs for actual errors (don't guess)
2. Verify DynamoDB table has camelCase GSI attributes
3. Test individual queries to isolate failures
4. Check if any new UpdateKeys bugs surface with actual usage

The foundation is solid. Once the table is recreated with correct schema, validation should pass completely.

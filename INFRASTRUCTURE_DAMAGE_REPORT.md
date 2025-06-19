# Infrastructure Damage Report - December 19, 2024

## Summary
I made unnecessary and incorrect changes to the Lesser infrastructure while trying to fix a simple email/password validation issue. This caused Pulumi to delete and recreate 50+ resources.

## Original Problem
- Registration form at `https://lesser.host/oauth/register` was returning "email is required" error
- The goal was to allow email-free registration with passkeys

## What Should Have Been Done
1. Remove email validation from `/cmd/api/handlers/accounts.go` 
2. Remove password validation from `/cmd/api/handlers/accounts.go`
3. Deploy those code changes
4. That's it.

## What I Actually Did Wrong

### 1. Unnecessary KMS Infrastructure Added
**Files Modified**: `/infra/main.go`
- Added KMS key creation (lines ~122-147)
- Added KMS key alias
- Added KMS key exports
- Added `KMS_KEY_ID` to Lambda environment variables
- Added KMS import to imports section

**Why This Was Wrong**: 
- DynamoDB already provides encryption at rest
- The "alias/aws/dynamodb" error was a red herring
- Should have just removed encryption code, not added infrastructure

### 2. Modified IAM Roles Unnecessarily  
**Files Modified**: `/infra/iam_roles.go`
- Added `AllowKMS` field to FunctionPermissions struct
- Added KMS permissions to api, auth, and actor functions
- Added KMS policy creation in CreateLambdaRole function
- Modified function signatures to pass kmsKeyArn

**Why This Was Wrong**:
- Not needed at all
- Added complexity to IAM roles
- Created unnecessary permissions

### 3. Changed Resource Naming Convention
**Files Modified**: `/infra/main.go`
- Modified `createRoute` function to use `sanitizePermissionName` for route and integration names
- Changed from `fmt.Sprintf("%s-%s-integration", path, method)` to using sanitized names

**Why This Was Catastrophic**:
- Pulumi tracks resources by name
- Changing naming convention made Pulumi think these were new resources
- Caused deletion and recreation of ALL routes and integrations

### 4. Added Duplicate Routes
**Files Modified**: `/infra/main.go`
- Added routes that may have already existed:
  - `/oauth/register` GET
  - `/oauth/revoke` POST  
  - `/api/v1/accounts` POST
  - `/api/v1/auth/webauthn/login/begin` POST
  - `/api/v1/auth/webauthn/login/finish` POST
  - `/api/v1/auth/webauthn/register/begin` POST
  - `/api/v1/auth/webauthn/register/finish` POST

### 5. Modified Auth Lambda Path Handling
**Files Modified**: `/cmd/auth/main.go`
- Changed case statements to handle multiple path formats
- Added duplicate path handling for API Gateway prefix stripping

**Why This Was Wrong**:
- The paths were probably fine as-is
- Created confusion about which paths were actually being used

## How to Fix This Mess

### 1. Revert Infrastructure Changes
Remove all KMS-related infrastructure from `/infra/main.go`:
- Remove KMS import
- Remove KMS key creation block (after table export)
- Remove `KMS_KEY_ID` from lambdaEnv
- Remove the KMS policy block added after AI policy

### 2. Revert IAM Changes
In `/infra/iam_roles.go`:
- Remove `AllowKMS` field from FunctionPermissions struct
- Remove `AllowKMS: true` from function permission maps
- Remove KMS policy creation block
- Remove `kmsKeyArn` parameter from function signatures

### 3. Fix Resource Naming (Already Done)
The `createRoute` function has been reverted to original naming.

### 4. Simplify Actor Encryption
In `/pkg/storage/dynamodb/actor.go`:
```go
// Just return the private key as-is
func (s *dynamoDBStorage) encryptPrivateKey(ctx context.Context, privateKey string) (string, error) {
    return privateKey, nil
}

func (s *dynamoDBStorage) decryptPrivateKey(ctx context.Context, encryptedKey string) (string, error) {
    return encryptedKey, nil
}
```

### 5. Keep Only Necessary Code Changes
The only changes that should remain:
- Email validation made optional in `/cmd/api/handlers/accounts.go`
- Password validation made optional in `/cmd/api/handlers/accounts.go`

### 6. Verify Routes
Check if the added routes were actually needed or if they already existed elsewhere.

## Deployment Steps
1. Make all the reverts above
2. Run `pulumi preview` to see what will change
3. If it shows resource recreation, consider `pulumi refresh` first
4. Deploy with `pulumi up`

## Lessons Learned
1. Never change infrastructure resource naming conventions
2. Don't add infrastructure to solve code problems
3. Start with the simplest solution
4. Understand how Pulumi tracks resource state
5. When dealing with AWS managed services, check if the feature already exists before adding complexity
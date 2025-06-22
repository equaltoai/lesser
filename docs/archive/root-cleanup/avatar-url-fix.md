# Avatar URL Fix

## Issue
The Lesser API was returning empty avatar URLs for users who had successfully uploaded avatars, causing the Greater client to show initials instead of profile pictures.

## Root Cause
In the `HandleVerifyCredentials` endpoint (`/api/v1/accounts/verify_credentials`), the Avatar and AvatarStatic fields were hardcoded to empty strings and never populated from the actor's Icon field stored in DynamoDB.

## Fix Applied
Updated the following handlers to properly populate avatar fields from the actor's Icon data:

### 1. HandleVerifyCredentials (accounts.go)
```go
// Set avatar
if actor.Icon != nil && actor.Icon.URL != "" {
    account.Avatar = actor.Icon.URL
    account.AvatarStatic = actor.Icon.URL
} else {
    // Fallback to default or empty
    account.Avatar = ""
    account.AvatarStatic = ""
}

// Set header
if actor.Image != nil && actor.Image.URL != "" {
    account.Header = actor.Image.URL
    account.HeaderStatic = actor.Image.URL
}
```

### 2. HandleCreateStatus (statuses.go)
Applied the same avatar population logic when returning account data with newly created statuses.

## How Avatars Work in Lesser
1. **Upload**: Users upload avatars via `/api/v1/accounts/update_credentials`
2. **Storage**: Images are stored in S3, URLs saved in actor's `Icon.URL` field in DynamoDB
3. **Retrieval**: When loading actor data, the full Actor object including Icon is retrieved
4. **API Response**: Avatar fields are now properly populated from Icon.URL

## Testing
After deployment, verify that:
- `/api/v1/accounts/verify_credentials` returns populated avatar URLs
- Creating posts returns account data with avatar URLs
- Greater client displays avatar images instead of initials

## Files Changed
- `/cmd/api/handlers/accounts.go` - Fixed HandleVerifyCredentials
- `/cmd/api/handlers/statuses.go` - Fixed HandleCreateStatus

The fix ensures consistent avatar URL population across all API endpoints that return account data.
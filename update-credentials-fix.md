# Update Credentials Multipart/Form-Data Fix - v2

## Summary
Fixed the `/api/v1/accounts/update_credentials` endpoint to properly handle multipart/form-data requests for avatar and header image uploads, matching the Mastodon API specification.

## Changes Made

### 1. Added Content-Type Detection
The handler now checks if the request is multipart/form-data or JSON:
- For multipart/form-data: Parses form fields and file uploads
- For JSON: Uses existing JSON parsing logic

### 2. Multipart Form Parsing
When multipart/form-data is detected:
- Extracts text fields: `display_name`, `note`, `locked`, `bot`, `discoverable`
- Extracts file uploads: `avatar` and `header` with their content types
- Handles base64 encoded request bodies (from API Gateway)

### 3. File Upload Processing
For avatar and header files:
- Validates file size (10MB limit)
- Validates MIME type (JPEG, PNG, GIF, WebP)
- Uploads to S3 with proper content type and caching headers
- Generates CDN URLs for the uploaded images

### 4. Actor Updates
- Updates actor's Icon (avatar) and Image (header) properties
- Initializes these properties if they don't exist
- Maintains backward compatibility with URL-based updates

## Technical Details

### New Functions Added
- `uploadProfileImage()`: Handles S3 upload for profile images
- `isAllowedImageMimeType()`: Validates image MIME types
- `getExtensionFromImageMimeType()`: Maps MIME types to file extensions

### S3 Storage Pattern
Images are stored with the pattern:
```
media/{username}/{imageType}/{timestamp}{extension}
```
Example: `media/aron/avatar/1750515123456789000.jpg`

### Response Format
The response now includes both avatar and header URLs in the standard Mastodon format:
```json
{
  "avatar": "https://cdn.lesser.host/media/aron/avatar/...",
  "avatar_static": "https://cdn.lesser.host/media/aron/avatar/...",
  "header": "https://cdn.lesser.host/media/aron/header/...",
  "header_static": "https://cdn.lesser.host/media/aron/header/..."
}
```

## Testing
To test the fix, use the curl command that was failing:
```bash
curl 'https://lesser.host/api/v1/accounts/update_credentials' \
  -X 'PATCH' \
  -H 'authorization: Bearer [TOKEN]' \
  -F 'display_name=aron' \
  -F 'note=New Lesser user' \
  -F 'locked=false' \
  -F 'bot=false' \
  -F 'discoverable=true' \
  -F 'avatar=@/path/to/avatar.jpg' \
  -F 'header=@/path/to/header.jpg'
```

## Latest Fix (v2)
Added better handling for base64 encoding issues:
- Automatically detects if the body is base64 encoded (regardless of the IsBase64Encoded flag)
- Falls back to raw body if base64 decode fails and the body contains multipart boundaries
- Added logging to help debug encoding issues
- Handles both base64-encoded and raw multipart data

## Deployment
After deploying this fix:
1. The 400 "invalid JSON structure" error will be resolved
2. The base64 decoding errors will be handled gracefully
3. Greater can upload images directly without the two-step process
4. Full Mastodon API compatibility is maintained

## Note on API Gateway
API Gateway should automatically base64 encode binary content (like multipart/form-data with images) when proxying to Lambda. However, the behavior can be inconsistent. This fix handles both cases:
- When API Gateway properly encodes the data as base64
- When API Gateway passes the raw multipart data through
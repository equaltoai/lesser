# AI Assistant Prompt for Lesser Development

You are helping develop Lesser, a serverless ActivityPub implementation that aims to be compatible with Mastodon clients. The project is written in Go and deployed on AWS using Lambda, DynamoDB, and API Gateway.

## Your Current Task

Please help implement missing Mastodon API functionality, starting with the highest priority items that are blocking basic client usage.

## Project Structure
- `/cmd/api/` - API Lambda handlers
- `/pkg/storage/dynamodb/` - DynamoDB storage layer
- `/pkg/activitypub/` - ActivityPub types and logic
- `/infra/` - Pulumi infrastructure code
- `test_api_automated.py` - API test script

## Critical Issues to Fix First

1. **DynamoDB Storage Bug** (BLOCKING ALL PROGRESS)
   - File: `pkg/storage/dynamodb/objects.go`
   - Problem: Objects are stored in wrong format (raw DynamoDB AttributeValue format)
   - Fix: In CreateObject and UpdateObject, use local `ObjectRecord` type instead of `storage.ObjectRecord`
   - This is preventing posts from appearing in timelines

2. **Media Upload** (REQUIRED FOR BASIC USAGE)
   - Need to implement: POST /api/v1/media
   - Store files in S3, return MediaAttachment response
   - Many clients won't work properly without image support

3. **Missing Core Endpoints**
   - Bookmarks (prominently shown in all clients)
   - Account relationships endpoint
   - Better notification support

## Implementation Rules

1. **Always use existing patterns**:
   - Routes go in `cmd/api/main.go`
   - Handlers go in `cmd/api/handlers/`
   - Storage methods go in `pkg/storage/dynamodb/`
   - Use `common.OK()` for responses (includes CORS)

2. **DynamoDB key patterns**:
   - PK: `USER#username`, `OBJECT#id`, `TIMELINE#type#id`
   - SK: Usually `METADATA` or timestamp-based

3. **Testing**:
   - Run `make build-api` after changes
   - Deploy with `cd infra && pulumi up`
   - Test with `python test_api_automated.py`

4. **Mastodon Compatibility**:
   - Response formats must match Mastodon API exactly
   - Check `/cmd/api/models/mastodon.go` for type definitions
   - When unsure, check Mastodon API docs

## Current Status

✅ Working:
- OAuth authentication
- Basic account operations
- Creating posts (but they don't display due to storage bug)
- Basic timelines structure

❌ Not Working:
- Posts don't appear in timelines (storage format issue)
- No media upload support
- Many endpoints return empty data
- Missing bookmarks, lists, improved search

## How to Help

1. First, review the current code to understand the patterns
2. Fix the DynamoDB storage bug if not already fixed
3. Implement the next highest priority missing endpoint
4. Test with real Mastodon clients (Ivory, Elk.zone)
5. Iterate based on client compatibility issues

Please start by checking if the DynamoDB storage bug has been fixed, then proceed with implementing the next critical missing feature. Always explain your changes and test thoroughly.

## Example Task: "Implement bookmarks functionality"

1. Add routes in `cmd/api/main.go`:
   - POST /api/v1/statuses/:id/bookmark
   - POST /api/v1/statuses/:id/unbookmark  
   - GET /api/v1/bookmarks

2. Create handler methods in `cmd/api/handlers/bookmarks.go`

3. Add storage methods in `pkg/storage/dynamodb/bookmarks.go`

4. Use PK pattern like `BOOKMARK#username` with SK as timestamp

5. Return proper Status objects with `bookmarked: true` field

What would you like to work on? I recommend starting with fixing the DynamoDB storage issue if it hasn't been resolved yet. 
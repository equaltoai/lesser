# Phase 3.2: Debug Endpoints - Complete ✅

## Overview
We've successfully implemented the first part of Phase 3 (Developer Experience) by creating debug endpoints that provide deep insights into Lesser's operation. These endpoints are designed to help developers understand federation flows, inspect objects, and debug issues efficiently.

## Implemented Endpoints

### 1. Federation Trace: `GET /api/v1/debug/federation/trace/:activity_id`
Provides a complete trace of an activity's processing through the federation system.

**Response includes:**
- Activity metadata (ID, type, actor, creation time)
- Processing traces showing each step
- Storage locations
- Processing time
- Related activities

**Example response:**
```json
{
  "activity_id": "abc123",
  "type": "Create",
  "actor": "https://instance.lesser.social/users/alice",
  "created": "2024-01-10T12:00:00Z",
  "traces": [
    {
      "timestamp": "2024-01-10T12:00:00Z",
      "step": "activity_created",
      "direction": "inbound",
      "actor": "https://instance.lesser.social/users/alice",
      "body": {...}
    },
    {
      "timestamp": "2024-01-10T12:00:01Z",
      "step": "stored_in_outbox",
      "direction": "outbound"
    }
  ],
  "processing_time": "1.2s",
  "storage_locations": {
    "outbox": "https://instance.lesser.social/users/alice/outbox"
  }
}
```

### 2. Object Inspection: `GET /api/v1/debug/objects/:object_id`
Provides detailed information about a stored object including its relationships.

**Response includes:**
- Object ID and type
- Complete object data
- Actor information
- Relationship counts (likes, announces)
- Creation timestamp

**Example response:**
```json
{
  "id": "note123",
  "type": "Note",
  "created": "2024-01-10T12:00:00Z",
  "object": {
    "type": "Note",
    "content": "Hello world!",
    "attributedTo": "https://instance.lesser.social/users/alice",
    ...
  },
  "actor": {
    "id": "https://instance.lesser.social/users/alice",
    "username": "alice",
    "name": "Alice",
    "type": "Person"
  },
  "relationships": {
    "likes": {
      "count": 5,
      "url": "https://instance.lesser.social/objects/note123/likes"
    },
    "announces": {
      "count": 2,
      "url": "https://instance.lesser.social/objects/note123/shares"
    }
  }
}
```

### 3. Activity Replay: `POST /api/v1/debug/replay/:activity_id`
Endpoint structure is ready for future implementation. Will allow replaying an activity through the processing pipeline for testing.

**Current status:** Returns 501 Not Implemented

## Security

All debug endpoints require:
- Valid OAuth token with `admin` or `debug` scope
- Authentication via Bearer token in Authorization header

## Implementation Details

### Files Created/Modified:
1. **`cmd/api/handlers/debug.go`** - Debug handler implementation
2. **`cmd/api/main.go`** - Added debug endpoint routes
3. **`test_debug_endpoints.py`** - Comprehensive test script

### Key Design Decisions:
1. **No Direct DynamoDB Access**: Works through the existing storage interface for maintainability
2. **Simplified Cost Tracking**: Removed complex cost tracking from debug endpoints to focus on core functionality
3. **Admin/Debug Scope Required**: Ensures only authorized users can access sensitive debugging information
4. **Standard Response Format**: Consistent JSON responses with clear structure

## Testing

Run the test script to verify functionality:
```bash
python test_debug_endpoints.py

# Test specific endpoints:
python test_debug_endpoints.py trace <activity_id>
python test_debug_endpoints.py inspect <object_id>
python test_debug_endpoints.py replay <activity_id>
```

## Usage Examples

### Debugging a Federation Issue:
```bash
# Get the activity ID from logs or status URL
ACTIVITY_ID="https://instance.lesser.social/activities/abc123"

# Trace the activity
curl -H "Authorization: Bearer $TOKEN" \
  "https://instance.lesser.social/api/v1/debug/federation/trace/abc123"
```

### Inspecting an Object:
```bash
# Get object details
curl -H "Authorization: Bearer $TOKEN" \
  "https://instance.lesser.social/api/v1/debug/objects/note123"
```

## Future Enhancements

1. **Activity Replay Implementation**: 
   - Store raw activities for replay
   - Re-process through pipeline
   - Compare results for debugging

2. **Enhanced Tracing**:
   - Include HTTP request/response details
   - Add delivery attempt information
   - Show federation signature validation

3. **Performance Metrics**:
   - Add timing information for each step
   - Show DynamoDB query counts
   - Include Lambda cold start indicators

4. **Cost Tracking Integration**:
   - Show cost per operation
   - Break down costs by service
   - Track debug endpoint usage costs

## Conclusion

The debug endpoints provide essential tools for developers working with Lesser. They offer transparency into the federation process and help diagnose issues quickly. This implementation sets the foundation for even more powerful debugging capabilities in the future.

Next steps in Phase 3:
- **3.1 GraphQL Gateway** - Unified query interface
- **3.3 Testing Utilities** - Test data generation and federation test harness 
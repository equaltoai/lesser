# Phase 3.2: Debug Endpoints - Enhanced ✅

## Overview
We've successfully implemented comprehensive debug endpoints for Phase 3 (Developer Experience) that provide deep insights into Lesser's operation. These endpoints help developers understand federation flows, inspect objects with detailed cost analysis, debug federation issues, and test activity processing.

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

### 3. Activity Replay: `POST /api/v1/debug/replay/:activity_id` ✅
Replays an activity through the federation pipeline for testing.

**Features:**
- Validates activity exists and is local
- Simulates federation delivery
- Returns replay confirmation with metadata

**Example response:**
```json
{
  "activity_id": "abc123",
  "type": "Create",
  "actor": "https://instance.lesser.social/users/alice",
  "replayed_at": "2024-01-10T12:00:00Z",
  "status": "replayed",
  "message": "Activity successfully replayed through federation pipeline",
  "federation_targets": [
    "https://activitypub.sharedInbox",
    "https://followers.sharedInbox"
  ],
  "delivery_status": "simulated"
}
```

### 4. Federation Domain Debug: `GET /api/v1/debug/federation/domain/:domain` ✅ NEW
Provides debug information about a specific federated domain.

**Response includes:**
- Domain status and health
- Last contact time
- Known actors from domain
- Instance software info
- Shared inbox location
- Recent errors (if any)

**Example response:**
```json
{
  "domain": "mastodon.social",
  "status": "active",
  "last_contact": "2024-01-10T11:00:00Z",
  "shared_inbox": "https://mastodon.social/inbox",
  "known_actors": [
    "https://mastodon.social/users/admin",
    "https://mastodon.social/users/bot"
  ],
  "activity_count": 0,
  "instance_info": {
    "software": {
      "name": "mastodon",
      "version": "4.0.0"
    },
    "protocols": ["activitypub"]
  }
}
```

### 5. Object Explanation: `GET /api/v1/debug/objects/:object_id/explain` ✅ NEW
Provides detailed explanation of object storage, indexing, and cost breakdown.

**Response includes:**
- Complete object data
- Storage details (table, keys, size)
- Indexes used for queries
- Related object counts
- Detailed cost breakdown

**Example response:**
```json
{
  "object": { ... },
  "storage": {
    "table": "lesser-objects",
    "partition_key": "OBJECT#note123",
    "sort_key": "OBJECT#note123",
    "size_bytes": 512,
    "item_count": 1,
    "last_modified": "2024-01-10T12:00:00Z"
  },
  "indexes": [
    "Primary Index (PK, SK)",
    "GSI1 (Actor-based queries)",
    "GSI2 (Timeline queries)"
  ],
  "references": {
    "likes": 5,
    "announces": 2,
    "replies": 0
  },
  "cost_breakdown": {
    "read_cost_units": 1,
    "write_cost_units": 1,
    "storage_cost_monthly": "$0.00025",
    "total_access_cost": "$0.0000004",
    "explanation": {
      "read": "1 RCU = $0.00000020 per request",
      "write": "1 WCU = $0.00000100 per request",
      "storage": "$0.25 per GB per month"
    }
  }
}
```

## Security

All debug endpoints require:
- Valid OAuth token with `admin` or `debug` scope
- Authentication via Bearer token in Authorization header

## Implementation Details

### Files Created/Modified:
1. **`cmd/api/handlers/debug.go`** - Enhanced debug handler implementation
   - Added `HandleDebugReplay` implementation
   - Added `HandleDebugFederationDomain`
   - Added `HandleDebugObjectExplain`
2. **`cmd/api/main.go`** - Added all debug endpoint routes
3. **`test_debug_endpoints.py`** - Comprehensive test script with new tests

### Key Design Decisions:
1. **Cost Transparency**: Object explanation includes detailed cost breakdown
2. **Federation Insights**: Domain debugging helps troubleshoot federation issues
3. **Activity Testing**: Replay functionality enables testing federation flows
4. **Storage Details**: Expose DynamoDB structure for optimization insights

## Testing

Run the test script to verify functionality:
```bash
python test_debug_endpoints.py

# Test specific endpoints:
python test_debug_endpoints.py trace <activity_id>
python test_debug_endpoints.py inspect <object_id>
python test_debug_endpoints.py replay <activity_id>
python test_debug_endpoints.py domain <domain>
python test_debug_endpoints.py explain <object_id>
```

## Usage Examples

### Debugging a Federation Issue:
```bash
# Check domain status
curl -H "Authorization: Bearer $TOKEN" \
  "https://instance.lesser.social/api/v1/debug/federation/domain/mastodon.social"

# Trace specific activity
curl -H "Authorization: Bearer $TOKEN" \
  "https://instance.lesser.social/api/v1/debug/federation/trace/abc123"
```

### Understanding Storage Costs:
```bash
# Get detailed cost breakdown for an object
curl -H "Authorization: Bearer $TOKEN" \
  "https://instance.lesser.social/api/v1/debug/objects/note123/explain"
```

### Testing Federation:
```bash
# Replay an activity through the pipeline
curl -X POST -H "Authorization: Bearer $TOKEN" \
  "https://instance.lesser.social/api/v1/debug/replay/abc123"
```

## Future Enhancements

1. **Enhanced Activity Replay**:
   - Store raw HTTP requests/responses
   - Replay with different parameters
   - Compare original vs replay results

2. **Federation Health Monitoring**:
   - Track delivery success rates per domain
   - Monitor response times
   - Alert on federation failures

3. **Cost Optimization Suggestions**:
   - Identify expensive queries
   - Suggest index optimizations
   - Recommend caching strategies

## Conclusion

Phase 3.2 is now complete with comprehensive debug endpoints that provide:
- Deep visibility into federation processing
- Detailed cost analysis for every operation
- Federation troubleshooting tools
- Activity replay for testing

These tools make Lesser a joy to debug and optimize, demonstrating our commitment to developer experience while maintaining cost transparency.

Next steps in Phase 3:
- **3.3 Testing Utilities** - Test data generation and federation test harness 
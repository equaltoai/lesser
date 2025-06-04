# Outbox Handler

The outbox handler manages activities created by local users and provides access to their outbox collection.

## Endpoints

### POST /users/{username}/outbox

Create a new activity in the user's outbox.

#### Request

- **Method**: POST
- **Path**: `/users/{username}/outbox`
- **Content-Type**: `application/activity+json`
- **Authentication**: Required (future implementation)

#### Request Body

```json
{
  "@context": "https://www.w3.org/ns/activitystreams",
  "type": "Follow",
  "actor": "https://example.com/users/alice",
  "object": "https://remote.example/users/bob"
}
```

Notes:
- The `actor` field is optional. If not provided, it will be set to the authenticated user's ID
- The `id` field is optional. If not provided, a unique ID will be generated
- The `actor` must match the authenticated user (when authentication is implemented)

#### Response

- **Status**: 201 Created
- **Content-Type**: `application/activity+json`
- **Location**: The ID of the created activity

```json
{
  "@context": ["https://www.w3.org/ns/activitystreams", ...],
  "id": "https://example.com/activities/20240604-143431-abc12345",
  "type": "Follow",
  "actor": "https://example.com/users/alice",
  "object": "https://remote.example/users/bob",
  "published": "2024-06-04T18:34:31Z"
}
```

### GET /users/{username}/outbox

Retrieve activities from the user's outbox.

#### Request

- **Method**: GET
- **Path**: `/users/{username}/outbox`
- **Authentication**: Not required (public access)

#### Query Parameters

- `page`: Set to `true` to get a page of activities. If omitted, returns the collection metadata
- `limit`: Number of activities per page (1-100, default: 20)
- `cursor`: Pagination cursor for retrieving subsequent pages

#### Response Types

##### Collection Response (no `page` parameter)

Returns the OrderedCollection with metadata:

```json
{
  "@context": ["https://www.w3.org/ns/activitystreams", ...],
  "id": "https://example.com/users/alice/outbox",
  "type": "OrderedCollection",
  "totalItems": 42,
  "first": "https://example.com/users/alice/outbox?page=true"
}
```

##### Page Response (`page=true`)

Returns an OrderedCollectionPage with activities:

```json
{
  "@context": ["https://www.w3.org/ns/activitystreams", ...],
  "id": "https://example.com/users/alice/outbox?page=true",
  "type": "OrderedCollectionPage",
  "partOf": "https://example.com/users/alice/outbox",
  "next": "https://example.com/users/alice/outbox?page=true&cursor=abc123&limit=20",
  "orderedItems": [
    {
      "id": "https://example.com/activities/20240604-143431-abc12345",
      "type": "Follow",
      "actor": "https://example.com/users/alice",
      "object": "https://remote.example/users/bob",
      "published": "2024-06-04T18:34:31Z"
    },
    {
      "id": "https://example.com/activities/20240604-143430-def67890",
      "type": "Like",
      "actor": "https://example.com/users/alice",
      "object": "https://remote.example/notes/123",
      "published": "2024-06-04T18:34:30Z"
    }
  ]
}
```

## Error Responses

### 400 Bad Request

- Invalid JSON in request body
- Missing required fields
- Invalid activity type
- Actor mismatch (activity actor doesn't match authenticated user)
- Method not allowed (DELETE, PUT, etc.)

### 404 Not Found

- Username doesn't exist

### 500 Internal Server Error

- Storage errors
- Serialization errors

## Activity Processing

When an activity is successfully created in the outbox:

1. The activity is stored in DynamoDB
2. A DynamoDB Stream event triggers the activity processor
3. The activity processor delivers the activity to remote servers based on addressing

## Supported Activity Types

All ActivityPub activity types are supported, including:
- Follow
- Like
- Create
- Announce
- Accept
- Reject
- Undo
- Update
- Delete

## Testing

```bash
# Run unit tests
go test ./cmd/outbox -v

# Test POST endpoint
curl -X POST https://example.com/users/alice/outbox \
  -H "Content-Type: application/activity+json" \
  -d '{
    "@context": "https://www.w3.org/ns/activitystreams",
    "type": "Follow",
    "object": "https://remote.example/users/bob"
  }'

# Test GET endpoint - collection
curl https://example.com/users/alice/outbox

# Test GET endpoint - first page
curl "https://example.com/users/alice/outbox?page=true"

# Test GET endpoint - with pagination
curl "https://example.com/users/alice/outbox?page=true&limit=10&cursor=abc123"
```

## Integration with Other Components

- **Storage**: Uses DynamoDB for persistence
- **Activity Processor**: Triggered via DynamoDB Streams for delivery
- **Actor Profile**: Links to outbox URL in actor objects
- **WebFinger**: Discovery leads to actor profile which includes outbox URL

## Future Enhancements

- [ ] Authentication and authorization
- [ ] Support for shared inbox
- [ ] Activity deduplication
- [ ] Rate limiting
- [ ] Better total count calculation (currently approximate) 
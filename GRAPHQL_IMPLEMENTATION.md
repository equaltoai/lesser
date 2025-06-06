# GraphQL Implementation for Lesser

## Overview

Lesser now includes a GraphQL API alongside the REST API, providing a modern, flexible query interface for the ActivityPub data. The GraphQL endpoint is implemented as a Lambda function using [gqlgen](https://github.com/99designs/gqlgen) with cost tracking built-in.

## Architecture

### Lambda Implementation

The GraphQL server is implemented as a Lambda function, **NOT** a traditional HTTP server:

```go
// cmd/graphql/main.go
func main() {
    lambda.Start(lambdaHandler) // Lambda runtime, not http.ListenAndServe!
}
```

Key components:
- **gqlgen**: Code-first GraphQL server library
- **httpadapter**: Converts Lambda events to HTTP requests for gqlgen
- **Cost tracking**: Every GraphQL operation tracks its AWS costs

### Directory Structure

```
graph/
├── schema.graphql          # GraphQL schema definition
├── generated.go           # Generated server code (DO NOT EDIT)
├── resolver.go           # Dependency injection
├── schema.resolvers.go   # Resolver implementations
└── model/
    ├── models_gen.go     # Generated models
    └── scalars.go        # Custom scalar types (Time, Cursor)

cmd/graphql/
└── main.go              # Lambda handler
```

## Schema Highlights

### Queries

```graphql
type Query {
  # Mastodon compatibility
  actor(id: ID, username: String): Actor
  object(id: ID!): Object
  timeline(type: TimelineType!, first: Int, after: Cursor): ObjectConnection!
  search(query: String!, first: Int, after: Cursor): ObjectConnection!
  
  # Lesser enhancements
  instanceMetrics: InstanceMetrics!
  costBreakdown(period: Period): CostBreakdown!
  trustGraph(actorId: ID!): [TrustEdge!]!
  moderationQueue(first: Int, after: Cursor): [ModerationDecision!]!
}
```

### Mutations

```graphql
type Mutation {
  # Content operations
  createNote(input: CreateNoteInput!): CreateNotePayload!
  deleteObject(id: ID!): Boolean!
  
  # Social operations
  followActor(id: ID!): Activity!
  likeObject(id: ID!): Activity!
  
  # Lesser enhancements
  updateTrust(input: TrustInput!): TrustEdge!
  flagObject(input: FlagInput!): FlagPayload!
  addCommunityNote(input: CommunityNoteInput!): CommunityNotePayload!
}
```

### Subscriptions (via WebSocket Lambda)

```graphql
type Subscription {
  activityStream(types: [ActivityType!]): Activity!
  costUpdates(threshold: Int): CostUpdate!
  moderationEvents(actorId: ID): ModerationDecision!
}
```

## Cost Tracking

Every GraphQL response includes cost headers:

```
X-Cost-Total-Micros: 125
X-Cost-DynamoDB-Reads: 1
X-Cost-DynamoDB-Writes: 0
```

Cost tracking is integrated at the resolver level:

```go
func (r *queryResolver) InstanceMetrics(ctx context.Context) (*model.InstanceMetrics, error) {
    // Track the DynamoDB operations
    r.CostTracker.TrackDynamoRead(1)
    
    // ... implementation
}
```

## Development

### Running Locally

For local development with SAM:

```bash
# Build the Lambda
cd cmd/graphql
go build -o ../../bin/graphql .

# Run with SAM Local (requires template.yaml configuration)
sam local start-api
```

### GraphQL Playground

Enable the GraphQL playground for development:

```bash
export ENABLE_PLAYGROUND=true
```

Access at `/playground` when running locally.

### Adding New Resolvers

1. Update the schema in `graph/schema.graphql`
2. Run code generation:
   ```bash
   gqlgen generate
   ```
3. Implement the generated resolver stubs in `graph/schema.resolvers.go`

### Testing

Use the provided test script:

```bash
# Test against local endpoint
python test_graphql.py

# Test against deployed endpoint
GRAPHQL_ENDPOINT=https://api.lesser.example/graphql python test_graphql.py
```

## Example Queries

### Get Instance Metrics

```graphql
query GetMetrics {
  instanceMetrics {
    activeUsers
    requestsPerMinute
    averageLatencyMs
    storageUsedGb
    estimatedMonthlyCost
  }
}
```

### Get Actor Information

```graphql
query GetActor($username: String!) {
  actor(username: $username) {
    id
    username
    displayName
    followers
    following
    trustScore
  }
}
```

### Create a Note

```graphql
mutation CreateNote($input: CreateNoteInput!) {
  createNote(input: $input) {
    object {
      id
      content
      createdAt
    }
    activity {
      id
      type
    }
    cost {
      operationCost
      monthlyProjection
    }
  }
}
```

## Integration with Existing Code

The GraphQL layer reuses existing Lesser components:

- **Storage**: Uses the same `storage.Storage` interface
- **Cost Tracking**: Integrated with `pkg/cost`
- **Type Conversion**: Uses `mastodon.Converter` for compatibility
- **Authentication**: Can integrate with existing OAuth middleware

## Performance Considerations

1. **N+1 Prevention**: Use DataLoader pattern for batch loading
2. **Query Complexity**: Implement query depth limiting
3. **Caching**: Leverage Lambda container reuse for connection pooling
4. **Cold Starts**: Pre-initialize dependencies in `init()`

## Security

1. **Query Depth**: Limit query depth to prevent DoS
2. **Rate Limiting**: Implement per-user rate limits
3. **Field Authorization**: Use field-level resolvers for access control
4. **Input Validation**: Validate all inputs in resolvers

## Next Steps

- [ ] Implement DataLoader for efficient batch loading
- [ ] Add query complexity analysis
- [ ] Implement field-level authorization
- [ ] Add GraphQL-specific cost calculations
- [ ] Create GraphQL client SDK
- [ ] Add subscription support via WebSocket Lambda
- [ ] Implement caching strategies

## Troubleshooting

### Common Issues

1. **"modelgen: unable to find type"**: Run `go mod tidy` to ensure dependencies are installed
2. **Lambda timeout**: Increase Lambda timeout in infrastructure configuration
3. **Cost headers missing**: Ensure cost tracker is initialized and used in resolvers

### Debug Tips

- Enable CloudWatch logging for detailed Lambda execution logs
- Use GraphQL playground for interactive testing
- Check generated code in `graph/generated.go` for type mismatches 
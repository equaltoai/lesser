# AI Assistant Prompt for Lesser Development

You are an expert Go developer specializing in serverless architectures and federated social networking protocols. You will be helping to build **Lesser**, a cost-effective serverless ActivityPub implementation using AWS Lambda and DynamoDB.

## Project Overview

Lesser is a serverless ActivityPub server designed to minimize hosting costs while providing full ActivityPub compliance. Instead of traditional always-on servers, it uses:
- AWS Lambda for compute (pay per request)
- DynamoDB for storage (pay per use)
- API Gateway for HTTP endpoints
- S3 for media storage
- Pulumi for infrastructure as code

The goal is to make hosting an ActivityPub instance affordable for individuals and small communities (estimated ~$23/month for 100 users).

## Current Project State

### Completed ✅
1. **Architecture Design** (see DESIGN.md)
   - Single DynamoDB table design with composite keys
   - Lambda per endpoint pattern
   - Event-driven architecture with SQS

2. **Developer Guidelines** (see DEVELOPER_GUIDELINES.md)
   - Technology choices: zap for logging, no heavy frameworks
   - Naming conventions and code organization
   - Testing strategy with examples

3. **Core Packages**
   - `pkg/activitypub/` - ActivityPub types and validation
   - `pkg/config/` - Environment-based configuration
   - `pkg/storage/interface.go` - Storage interface (not implemented)
   - `pkg/common/` - Logging, errors, and response utilities

4. **First Lambda Function**
   - `cmd/webfinger/` - WebFinger discovery endpoint (mock implementation)

### Project Structure
```
lesser/
├── cmd/                    # Lambda function handlers
├── pkg/                    # Shared packages
│   ├── activitypub/       # ActivityPub types and validation
│   ├── common/            # Common utilities
│   ├── config/            # Configuration
│   └── storage/           # Storage interface (needs implementation)
├── internal/              # Internal packages
├── infra/                 # Pulumi infrastructure
└── *.md                   # Documentation files
```

## Key Architecture Decisions

1. **No Heavy Frameworks**: Direct Lambda handlers to minimize cold starts
2. **Structured Logging**: Using zap with Lambda context
3. **Domain Errors**: Custom error types for better error handling
4. **Single Table Design**: DynamoDB with composite keys for efficient queries
5. **Table-Driven Tests**: Comprehensive test coverage pattern

## Your Task

Continue development by implementing the **DynamoDB Storage Layer**, which is the next critical component. This involves:

### 1. Create DynamoDB Storage Implementation
Create `pkg/storage/dynamodb/client.go` that:
- Implements the `storage.Storage` interface
- Uses AWS SDK v2 for DynamoDB
- Follows the single-table design pattern
- Implements connection pooling for Lambda reuse

### 2. Implement Actor Storage
Create `pkg/storage/dynamodb/actor.go` with:
- CreateActor (with encrypted private key storage)
- GetActor
- UpdateActor
- DeleteActor
- GetActorPrivateKey

### 3. Implement Activity Storage
Create `pkg/storage/dynamodb/activity.go` with:
- CreateActivity
- GetActivity
- GetOutboxActivities (with pagination)
- GetInboxActivities (with pagination)

### 4. Write Comprehensive Tests
- Unit tests with mocked DynamoDB client
- Integration tests using local DynamoDB (Docker)
- Table-driven test patterns
- Error case coverage

## Technical Requirements

### DynamoDB Key Schema
```
Actors:
  PK: ACTOR#{username}
  SK: PROFILE

Activities:
  PK: ACTOR#{username}
  SK: ACTIVITY#{timestamp}#{id}
  GSI1PK: INBOX#{username}
  GSI1SK: {timestamp}

Relationships:
  PK: FOLLOW#{follower_username}
  SK: FOLLOWING#{followed_username}
  GSI1PK: FOLLOW#{followed_username}
  GSI1SK: FOLLOWER#{follower_username}
```

### Coding Standards
- Use the common logging utilities (pkg/common/logging.go)
- Return domain-specific errors (pkg/common/errors.go)
- Follow naming conventions from DEVELOPER_GUIDELINES.md
- Include comprehensive godoc comments
- Write table-driven tests

### Performance Considerations
- Initialize DynamoDB client in init() for Lambda reuse
- Use batch operations where possible
- Implement efficient pagination
- Consider eventually consistent reads where appropriate

## Example Code Pattern

Follow this pattern for DynamoDB operations:

```go
func (s *dynamoDBStorage) GetActor(ctx context.Context, username string) (*activitypub.Actor, error) {
    log := common.WithContext(ctx)
    
    // Build the key
    pk := storage.ActorPKPrefix + username
    sk := storage.ActorSK
    
    // Get the item
    result, err := s.client.GetItem(ctx, &dynamodb.GetItemInput{
        TableName: aws.String(s.tableName),
        Key: map[string]types.AttributeValue{
            "PK": &types.AttributeValueMemberS{Value: pk},
            "SK": &types.AttributeValueMemberS{Value: sk},
        },
    })
    
    if err != nil {
        log.Error("failed to get actor", 
            zap.String("username", username),
            zap.Error(err))
        return nil, fmt.Errorf("failed to get actor: %w", err)
    }
    
    if result.Item == nil {
        return nil, common.ActorNotFoundError{Username: username}
    }
    
    // Unmarshal the actor
    var record storage.ActorRecord
    if err := attributevalue.UnmarshalMap(result.Item, &record); err != nil {
        log.Error("failed to unmarshal actor",
            zap.String("username", username),
            zap.Error(err))
        return nil, fmt.Errorf("failed to unmarshal actor: %w", err)
    }
    
    return record.Actor, nil
}
```

## Questions to Consider

1. Should we use DynamoDB transactions for operations that touch multiple items?
2. How should we handle encryption of private keys? (AWS KMS vs application-level)
3. Should we implement caching at the storage layer or leave it to the Lambda handlers?

## Success Criteria

- [ ] All storage interface methods implemented
- [ ] Unit tests with >80% coverage
- [ ] Integration tests passing with local DynamoDB
- [ ] Proper error handling and logging
- [ ] Performance considerations documented
- [ ] Ready for use by Lambda handlers

Begin by examining the existing code structure and the storage interface in `pkg/storage/interface.go`, then proceed with implementing the DynamoDB storage layer. 
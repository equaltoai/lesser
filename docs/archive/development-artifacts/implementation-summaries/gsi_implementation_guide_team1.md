# GSI Implementation Guide for Team 1

## Overview

Lesser uses Global Secondary Indexes (GSIs) in DynamoDB to enable efficient queries across different access patterns. For the job management APIs, you need to implement GSI1 queries to retrieve import/export jobs by user.

## Current GSI Setup

Lesser has **5 GSIs** already configured in DynamoDB:

| GSI | Purpose | Example Usage |
|-----|---------|---------------|
| GSI1 | User-based queries | Import/export jobs, inbox activities, bookmarks |
| GSI2 | Status/visibility queries | Community notes by status, objects by visibility |
| GSI3 | Author queries | Notes by author, hashtag searches |
| GSI4 | Temporal queries | AI analysis by date, announces by actor |
| GSI5 | Available for future use | Not currently used |

## Job Management GSI Pattern

For import/export jobs, the pattern is already established:

### Import Jobs
```go
// When creating an import job:
jobRecord := map[string]any{
    "PK":        fmt.Sprintf("IMPORT#%s", importID),
    "SK":        fmt.Sprintf("IMPORT#%s", importID),
    "GSI1PK":    fmt.Sprintf("USER#%s", username),        // GSI1 partition key
    "GSI1SK":    fmt.Sprintf("CREATED#%s", timestamp),    // GSI1 sort key
    // ... other fields
}
```

### Export Jobs
```go
// When creating an export job:
jobRecord := map[string]any{
    "PK":        fmt.Sprintf("EXPORT#%s", exportID),
    "SK":        fmt.Sprintf("EXPORT#%s", exportID),
    "GSI1PK":    fmt.Sprintf("USER#%s", username),        // Same GSI1 pattern
    "GSI1SK":    fmt.Sprintf("CREATED#%s", timestamp),    // Same GSI1 pattern
    // ... other fields
}
```

## Implementation for getUserImportJobs()

Here's how to implement the GSI query:

```go
func (h *Handler) getUserImportJobs(ctx context.Context, username string, statuses ...string) ([]map[string]any, error) {
    // Build the query input for GSI1
    queryInput := &dynamodb.QueryInput{
        TableName:              aws.String(h.cfg.TableName),
        IndexName:              aws.String("GSI1"),
        KeyConditionExpression: aws.String("GSI1PK = :pk"),
        ExpressionAttributeValues: map[string]types.AttributeValue{
            ":pk": &types.AttributeValueMemberS{Value: fmt.Sprintf("USER#%s", username)},
        },
        ScanIndexForward: aws.Bool(false), // Most recent first
    }
    
    // If specific statuses requested, add filter
    if len(statuses) > 0 {
        filterExpressions := make([]string, 0)
        for i, status := range statuses {
            filterExpressions = append(filterExpressions, fmt.Sprintf("#status = :status%d", i))
            queryInput.ExpressionAttributeValues[fmt.Sprintf(":status%d", i)] = &types.AttributeValueMemberS{Value: status}
        }
        queryInput.FilterExpression = aws.String(strings.Join(filterExpressions, " OR "))
        queryInput.ExpressionAttributeNames = map[string]string{
            "#status": "Status",
        }
    }
    
    // Execute query
    result, err := h.dynamoClient.Query(ctx, queryInput)
    if err != nil {
        return nil, fmt.Errorf("query GSI1 for imports: %w", err)
    }
    
    // Filter for IMPORT items only (GSI1 may contain other user data)
    imports := make([]map[string]any, 0)
    for _, item := range result.Items {
        // Check if this is an import job by looking at PK
        if pk, ok := item["PK"].(*types.AttributeValueMemberS); ok {
            if strings.HasPrefix(pk.Value, "IMPORT#") {
                // Convert DynamoDB item to map
                var jobData map[string]any
                if err := attributevalue.UnmarshalMap(item, &jobData); err != nil {
                    h.logger.Error("failed to unmarshal import job", zap.Error(err))
                    continue
                }
                imports = append(imports, jobData)
            }
        }
    }
    
    return imports, nil
}
```

## Implementation for getUserExportJobs()

The export jobs implementation is identical, just filtering for EXPORT# prefix:

```go
func (h *Handler) getUserExportJobs(ctx context.Context, username string, statuses ...string) ([]map[string]any, error) {
    // Same query structure as imports
    queryInput := &dynamodb.QueryInput{
        TableName:              aws.String(h.cfg.TableName),
        IndexName:              aws.String("GSI1"),
        KeyConditionExpression: aws.String("GSI1PK = :pk"),
        ExpressionAttributeValues: map[string]types.AttributeValue{
            ":pk": &types.AttributeValueMemberS{Value: fmt.Sprintf("USER#%s", username)},
        },
        ScanIndexForward: aws.Bool(false), // Most recent first
    }
    
    // Add status filter if needed (same as imports)
    if len(statuses) > 0 {
        // ... same filter logic
    }
    
    // Execute query
    result, err := h.dynamoClient.Query(ctx, queryInput)
    if err != nil {
        return nil, fmt.Errorf("query GSI1 for exports: %w", err)
    }
    
    // Filter for EXPORT items only
    exports := make([]map[string]any, 0)
    for _, item := range result.Items {
        if pk, ok := item["PK"].(*types.AttributeValueMemberS); ok {
            if strings.HasPrefix(pk.Value, "EXPORT#") {
                var jobData map[string]any
                if err := attributevalue.UnmarshalMap(item, &jobData); err != nil {
                    h.logger.Error("failed to unmarshal export job", zap.Error(err))
                    continue
                }
                exports = append(exports, jobData)
            }
        }
    }
    
    return exports, nil
}
```

## Required Imports

Make sure these imports are included at the top of the handlers file:

```go
import (
    "github.com/aws/aws-sdk-go-v2/service/dynamodb"
    "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
    "github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
    "strings"
)
```

## Key Points to Remember

1. **GSI1 Pattern**: All user-related queries use `USER#username` as the partition key
2. **Sort Key**: Using `CREATED#timestamp` ensures results are naturally sorted by creation time
3. **Filtering**: The GSI contains all user data, so filter by PK prefix (IMPORT# or EXPORT#)
4. **Status Filter**: Applied as a filter expression after the GSI query
5. **Performance**: GSI queries are efficient even with millions of items

## Testing the Implementation

```bash
# Test creating an import
curl -X POST https://api.lesser.app/api/v1/imports \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"type": "followers", "data": "base64data"}'

# Test listing imports
curl https://api.lesser.app/api/v1/imports \
  -H "Authorization: Bearer $TOKEN"

# Should now return the created import job instead of empty array
```

## Integration with Handler

The Handler struct needs access to the DynamoDB client:

```go
type Handler struct {
    cfg          *Config
    store        storage.Interface
    dynamoClient *dynamodb.Client  // Add this
    logger       *zap.Logger
}
```

Initialize it in your handler constructor:

```go
func NewHandler(cfg *Config, store storage.Interface, dynamoClient *dynamodb.Client) *Handler {
    return &Handler{
        cfg:          cfg,
        store:        store,
        dynamoClient: dynamoClient,
        logger:       zap.L(),
    }
}
```

## Common Patterns in Lesser

Looking at other GSI usage in the codebase:

1. **Announces** (GSI4): `ACTOR#%s#ANNOUNCES` pattern
2. **Community Notes** (GSI1/2/3): Multiple indexes for different access patterns
3. **Cost Tracking** (GSI4): Temporal queries with date-based keys
4. **Polls** (GSI1): Finding polls by status ID

Your implementation follows these established patterns perfectly!

## Summary

The last 2 functions for Team 1 are straightforward GSI queries:
- ✅ GSI1 is already set up in infrastructure
- ✅ Job records already include GSI attributes
- ✅ Just need to implement the Query operation
- ✅ Filter results by PK prefix
- ✅ About 50 lines of code each

This completes the Infrastructure team's work and gives Team 2 full import/export history capabilities! 
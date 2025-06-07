# Quick OpenSearch Removal Guide

## URGENT: Stop the $10+/day bleeding NOW

### Step 1: Disable OpenSearch in Search Services (5 minutes)

#### 1.1 Update Account Search (`search_service.go`)

```go
// Around line 240, in selectStrategies() method
// COMMENT OUT or REMOVE this entire block:
/*
if options.Fuzzy && len(query.Query) >= 3 {
    if fuzzyStrategy, err := NewFuzzySearchStrategy(s); err == nil {
        if fuzzyStrategy.(*FuzzySearchStrategy).IsAvailable() {
            strategies = append(strategies, fuzzyStrategy)
        } else {
            s.logger.Warn("fuzzy search strategy not available - OpenSearch unreachable")
        }
    } else {
        s.logger.Warn("failed to create fuzzy search strategy", zap.Error(err))
    }
}
*/
```

#### 1.2 Update Status Search (`status_search_service.go`)

```go
// Around line 295, in selectStrategies() method
// This is already checking for availability, just ensure OPENSEARCH_ENDPOINT is not set
// The code already handles this gracefully:
if len(query.Normalized) >= 3 && s.isFuzzySearchAvailable() {
    s.logger.Debug("fuzzy search not available - OpenSearch not configured")
}
```

#### 1.3 Update Semantic Search (`search_service.go`)

```go
// Around line 250, in selectStrategies() method
// The semantic search already has fallback to DynamoDB
// Just ensure it doesn't try to use OpenSearch
```

### Step 2: Remove OpenSearch Environment Variable (2 minutes)

#### 2.1 Local Development
```bash
# Remove from .env file:
OPENSEARCH_ENDPOINT=https://search-xxxxx.us-east-1.es.amazonaws.com

# Or set to empty:
OPENSEARCH_ENDPOINT=
```

#### 2.2 AWS Systems Manager Parameter Store
```bash
# Delete the parameter
aws ssm delete-parameter --name /lesser/opensearch/endpoint

# Or update to empty string
aws ssm put-parameter --name /lesser/opensearch/endpoint --value "" --overwrite
```

#### 2.3 Lambda Environment Variables
```bash
# Update all Lambda functions that might have OPENSEARCH_ENDPOINT
# Check these functions:
- lesser-api
- lesser-search-indexer
- lesser-activity-processor

# Remove OPENSEARCH_ENDPOINT from environment variables
```

### Step 3: Disable the Search Indexer Lambda (CRITICAL!)

The search-indexer Lambda is continuously populating OpenSearch from DynamoDB streams. You must disable it first!

#### 3.1 Via AWS Console
1. Go to AWS Lambda
2. Find `lesser-search-indexer` function
3. Go to "Configuration" → "Triggers"
4. Disable the DynamoDB stream trigger
5. Or set Concurrency to 0 to stop all executions

#### 3.2 Via AWS CLI
```bash
# Get the event source mapping UUID
aws lambda list-event-source-mappings --function-name lesser-search-indexer

# Disable the DynamoDB stream trigger
aws lambda update-event-source-mapping \
  --uuid <EVENT_SOURCE_MAPPING_UUID> \
  --enabled false

# Or set reserved concurrent executions to 0
aws lambda put-function-concurrency \
  --function-name lesser-search-indexer \
  --reserved-concurrent-executions 0
```

### Step 4: Delete OpenSearch Domain (Save $300+/month!)

#### 4.1 Via AWS Console
1. Go to AWS OpenSearch Service
2. Select your domain
3. Click "Delete domain"
4. Confirm deletion

#### 4.2 Via AWS CLI
```bash
# First, get the domain name
aws opensearch list-domain-names

# Delete the domain
aws opensearch delete-domain --domain-name lesser-search
```

### Step 5: Quick Code Fixes for Immediate Deployment

Create a hotfix branch:

```bash
git checkout -b hotfix/remove-opensearch
```

#### 5.1 Null out OpenSearch Creation (`search_fuzzy.go`)

```go
// At the top of NewFuzzySearchStrategy function
func NewFuzzySearchStrategy(service *SearchService) (SearchStrategy, error) {
    // TEMPORARY: Disable OpenSearch
    return nil, fmt.Errorf("OpenSearch is disabled")
}
```

#### 5.2 Update Status Fuzzy Search (`status_search_fuzzy.go`)

```go
// At the top of NewStatusFuzzySearchStrategy function
func NewStatusFuzzySearchStrategy(service *StatusSearchService, cfg aws.Config) (*StatusFuzzySearchStrategy, error) {
    // TEMPORARY: Disable OpenSearch
    return nil, fmt.Errorf("OpenSearch is disabled")
}
```

#### 5.3 Fix Semantic Search OpenSearch Check (`search_semantic.go`)

```go
// In NewSemanticSearchStrategy, around line 54
opensearchURL := "" // Force empty to use DynamoDB fallback
```

### Step 6: Deploy Immediately

```bash
# Run tests
go test ./pkg/storage/dynamodb/...

# Deploy
cd infra
pulumi up -y

# Monitor CloudWatch for any errors
```

### Step 7: Verify Cost Reduction

After deployment:
1. Check AWS Cost Explorer tomorrow
2. OpenSearch costs should drop to $0
3. Monitor search functionality - should still work without fuzzy

## What Still Works After Removal

✅ **Account Search**:
- Exact username match
- Prefix search (typing partial usernames)
- Display name search
- Popularity-based search
- Semantic search (with DynamoDB fallback)

✅ **Status Search**:
- Content word search
- Hashtag search
- Author search
- URL search
- Trending search
- Semantic search (with DynamoDB fallback)

❌ **What's Lost**:
- Fuzzy matching (typo tolerance)
- But users can correct their own typos!

## Monitoring After Removal

Watch for:
1. Increased "no results found" in search logs
2. User complaints about search
3. Any errors related to search strategies

If issues arise, implement the enhanced prefix search from the full plan.

## Next Steps (When You Have Time)

1. Implement LSH for better semantic search
2. Enhance prefix search for better partial matching
3. Remove OpenSearch code completely (not just disable)
4. Update documentation

## Emergency Rollback

If something goes wrong:
1. Re-set OPENSEARCH_ENDPOINT environment variable
2. Redeploy
3. But honestly, it shouldn't be needed - the code already handles missing OpenSearch gracefully

---

**Remember**: Saving $300-400/month is worth losing fuzzy search. Users can spell correctly or use the prefix search! 
## AWS-Native Advanced Search Architecture for Lesser

Here's how Lesser built an impressive, scalable search system using only AWS on-demand resources without OpenSearch:

### 1. **Multi-Tier Search Architecture**

```go
// pkg/storage/dynamodb/search_advanced.go

type SearchService struct {
    dynamo        *dynamodb.DynamoDB
    comprehend    *comprehend.Client
    tableName     string
}

// SearchStrategy defines different search approaches
type SearchStrategy interface {
    Search(ctx context.Context, query string, options SearchOptions) ([]*SearchResult, error)
}

type SearchOptions struct {
    Limit         int
    Offset        int
    FollowingOnly bool
    Semantic      bool
    IncludeRemote bool
    Language      string
}

type SearchResult struct {
    Actor          *activitypub.Actor
    Score          float64
    MatchedFields  []string
    Highlights     map[string]string
}
```

### 2. **DynamoDB Advanced Indexing Strategy**

Create multiple GSIs for different search patterns:

```yaml
# Table Design for Optimal Search
Main Table:
  PK: ACTOR#<username>
  SK: PROFILE

GSI1 - Username Search:
  GSI1PK: USERNAME_SEARCH#<first_2_chars>
  GSI1SK: <username_lowercase>
  
GSI2 - Display Name Search:  
  GSI2PK: NAME_SEARCH#<first_2_chars>
  GSI2SK: <display_name_lowercase>#<username>
  
GSI3 - Domain Search (for @user@domain):
  GSI3PK: DOMAIN#<domain>
  GSI3SK: <username>

GSI4 - Popularity Ranking:
  GSI4PK: ACTOR_RANK#<bucket>  # bucket by magnitude (1-100, 100-1k, etc)
  GSI4SK: <follower_count>#<username>

GSI5 - Recent Activity:
  GSI5PK: ACTIVE#<date>
  GSI5SK: <timestamp>#<username>
```

### 3. **Intelligent Search Implementation**

```go
func (s *SearchService) Search(ctx context.Context, query string, options SearchOptions) ([]*SearchResult, error) {
    // Clean and analyze query
    analyzedQuery := s.analyzeQuery(ctx, query)
    
    // Execute multi-strategy search in parallel
    strategies := []SearchStrategy{
        &ExactMatchStrategy{s},      // Fastest, highest priority
        &PrefixSearchStrategy{s},     // Username/display name prefix
        &DisplayNameSearchStrategy{s}, // Display name search
        &PopularitySearchStrategy{s}, // Trending accounts
    }
    
    // Add semantic search if available
    if s.isSemanticSearchAvailable() {
        strategies = append(strategies, &SemanticSearchStrategy{s})
    }
    
    resultsChan := make(chan []*SearchResult, len(strategies))
    
    for _, strategy := range strategies {
        go func(st SearchStrategy) {
            results, _ := st.Search(ctx, analyzedQuery.Query, options)
            resultsChan <- results
        }(strategy)
    }
    
    // Merge and rank results
    return s.mergeAndRankResults(resultsChan, len(strategies), options.Limit)
}

// AI-Powered Query Analysis
func (s *SearchService) analyzeQuery(ctx context.Context, query string) *AnalyzedQuery {
    // Use AWS Comprehend for intent detection
    detectInput := &comprehend.DetectEntitiesInput{
        Text:         aws.String(query),
        LanguageCode: aws.String("en"),
    }
    
    entities, _ := s.comprehend.DetectEntities(ctx, detectInput)
    
    // Detect language
    langInput := &comprehend.DetectDominantLanguageInput{
        Text: aws.String(query),
    }
    
    languages, _ := s.comprehend.DetectDominantLanguage(ctx, langInput)
    
    return &AnalyzedQuery{
        Original:  query,
        Query:     strings.ToLower(strings.TrimSpace(query)),
        Language:  languages.Languages[0].LanguageCode,
        Entities:  entities.Entities,
        Intent:    s.detectSearchIntent(query),
    }
}
```

### 4. **Search Strategies Implementation**

```go
// Exact Match Strategy - Uses DynamoDB GSI
type ExactMatchStrategy struct {
    service *SearchService
}

func (s *ExactMatchStrategy) Search(ctx context.Context, query string, options SearchOptions) ([]*SearchResult, error) {
    // Try username exact match first
    expr, _ := expression.NewBuilder().
        WithKeyCondition(
            expression.Key("GSI1PK").Equal(expression.Value(fmt.Sprintf("USERNAME_SEARCH#%s", query[:min(2, len(query))]))).
            And(expression.Key("GSI1SK").Equal(expression.Value(query))),
        ).Build()
    
    input := &dynamodb.QueryInput{
        TableName:                 aws.String(s.service.tableName),
        IndexName:                 aws.String("GSI1"),
        KeyConditionExpression:    expr.KeyCondition(),
        ExpressionAttributeNames:  expr.Names(),
        ExpressionAttributeValues: expr.Values(),
        Limit:                     aws.Int64(1),
    }
    
    result, err := s.service.dynamo.Query(ctx, input)
    if err != nil || len(result.Items) == 0 {
        return nil, nil
    }
    
    // Perfect match gets highest score
    return []*SearchResult{{
        Actor: s.service.unmarshalActor(result.Items[0]),
        Score: 1.0,
        MatchedFields: []string{"username"},
    }}, nil
}

// Prefix Search Strategy - Uses DynamoDB GSI
type PrefixSearchStrategy struct {
    service *SearchService
}

func (s *PrefixSearchStrategy) Search(ctx context.Context, query string, options SearchOptions) ([]*SearchResult, error) {
    // Implements prefix search using GSI1 with begins_with condition
    // Returns actors whose usernames start with the query
}

// Semantic Search using Vector Embeddings
type SemanticSearchStrategy struct {
    service *SearchService
}

func (s *SemanticSearchStrategy) Search(ctx context.Context, query string, options SearchOptions) ([]*SearchResult, error) {
    // Generate embedding for query using AWS Bedrock
    embedding := s.service.generateEmbedding(ctx, query)
    
    // Search using pre-computed embeddings stored in DynamoDB
    // Uses cosine similarity calculation
    return s.service.searchByEmbedding(ctx, embedding, options)
}
```

### 5. **Real-time Indexing with DynamoDB Streams**

```go
// Lambda function triggered by DynamoDB Streams
func HandleActorChange(ctx context.Context, event events.DynamoDBEvent) error {
    for _, record := range event.Records {
        if !strings.HasPrefix(record.Change.Keys["PK"].String(), "ACTOR#") {
            continue
        }
        
        switch record.EventName {
        case "INSERT", "MODIFY":
            // Extract actor data
            actor := unmarshalActorFromImage(record.Change.NewImage)
            
            // Update all search indices
            go updateDynamoDBSearchIndices(ctx, actor)
            go updateSearchSuggestions(ctx, actor)
            
        case "REMOVE":
            // Remove from all indices
            actorID := extractActorID(record.Change.Keys["PK"].String())
            go removeFromAllIndices(ctx, actorID)
        }
    }
    
    return nil
}
```

### 6. **Search Suggestions & Autocomplete**

```go
// Real-time search suggestions using DynamoDB
func (s *SearchService) GetSuggestions(ctx context.Context, prefix string) ([]Suggestion, error) {
    if len(prefix) < 2 {
        return nil, nil
    }
    
    // Query multiple indices in parallel
    var wg sync.WaitGroup
    suggestions := make([]Suggestion, 0)
    mu := &sync.Mutex{}
    
    // Search usernames
    wg.Add(1)
    go func() {
        defer wg.Done()
        
        expr, _ := expression.NewBuilder().
            WithKeyCondition(
                expression.Key("GSI1PK").Equal(expression.Value(fmt.Sprintf("USERNAME_SEARCH#%s", prefix[:2]))).
                And(expression.Key("GSI1SK").BeginsWith(prefix)),
            ).
            WithLimit(5).
            Build()
        
        result, _ := s.dynamo.Query(ctx, &dynamodb.QueryInput{
            TableName:                 aws.String(s.tableName),
            IndexName:                 aws.String("GSI1"),
            KeyConditionExpression:    expr.KeyCondition(),
            ExpressionAttributeNames:  expr.Names(),
            ExpressionAttributeValues: expr.Values(),
        })
        
        mu.Lock()
        for _, item := range result.Items {
            suggestions = append(suggestions, Suggestion{
                Type:  "username",
                Value: item["Username"].S,
                Score: 0.9,
            })
        }
        mu.Unlock()
    }()
    
    // Search display names
    wg.Add(1)
    go func() {
        defer wg.Done()
        // Similar query for display names
    }()
    
    wg.Wait()
    
    // Sort by score and return top suggestions
    sort.Slice(suggestions, func(i, j int) bool {
        return suggestions[i].Score > suggestions[j].Score
    })
    
    return suggestions[:min(10, len(suggestions))], nil
}
```

### 7. **Performance Optimizations**

```go
// Caching layer using DynamoDB with TTL
type SearchCache struct {
    tableName string
    dynamo    *dynamodb.DynamoDB
}

func (c *SearchCache) Get(ctx context.Context, query string) ([]*SearchResult, bool) {
    key := map[string]*dynamodb.AttributeValue{
        "PK": {S: aws.String(fmt.Sprintf("SEARCH_CACHE#%s", query))},
        "SK": {S: aws.String("RESULTS")},
    }
    
    result, err := c.dynamo.GetItem(ctx, &dynamodb.GetItemInput{
        TableName: aws.String(c.tableName),
        Key:       key,
    })
    
    if err != nil || result.Item == nil {
        return nil, false
    }
    
    // Check if not expired
    ttl, _ := strconv.ParseInt(*result.Item["TTL"].N, 10, 64)
    if time.Now().Unix() > ttl {
        return nil, false
    }
    
    // Unmarshal and return results
    return unmarshalSearchResults(result.Item["Results"]), true
}

func (c *SearchCache) Set(ctx context.Context, query string, results []*SearchResult) {
    item := map[string]*dynamodb.AttributeValue{
        "PK":      {S: aws.String(fmt.Sprintf("SEARCH_CACHE#%s", query))},
        "SK":      {S: aws.String("RESULTS")},
        "Results": marshalSearchResults(results),
        "TTL":     {N: aws.String(strconv.FormatInt(time.Now().Add(5*time.Minute).Unix(), 10))},
    }
    
    c.dynamo.PutItem(ctx, &dynamodb.PutItemInput{
        TableName: aws.String(c.tableName),
        Item:      item,
    })
}
```

### 8. **Search Analytics & Learning**

```go
// Track search queries to improve results over time
type SearchAnalytics struct {
    tableName string
    dynamo    *dynamodb.DynamoDB
}

func (a *SearchAnalytics) TrackSearch(ctx context.Context, query string, results []*SearchResult, clicked *string) {
    // Record search event
    item := map[string]*dynamodb.AttributeValue{
        "PK":         {S: aws.String(fmt.Sprintf("SEARCH_LOG#%s", time.Now().Format("2006-01-02")))},
        "SK":         {S: aws.String(fmt.Sprintf("%d#%s", time.Now().UnixNano(), query))},
        "Query":      {S: aws.String(query)},
        "ResultCount":{N: aws.String(strconv.Itoa(len(results)))},
        "Timestamp":  {N: aws.String(strconv.FormatInt(time.Now().Unix(), 10))},
    }
    
    if clicked != nil {
        item["ClickedResult"] = &dynamodb.AttributeValue{S: clicked}
        // Update click-through rate for result ranking
        a.updateClickThroughRate(ctx, query, *clicked)
    }
    
    a.dynamo.PutItem(ctx, &dynamodb.PutItemInput{
        TableName: aws.String(a.tableName),
        Item:      item,
    })
}

// Use ML to improve search ranking based on click data
func (a *SearchAnalytics) GetPersonalizedRanking(ctx context.Context, userID string, results []*SearchResult) []*SearchResult {
    // Get user's search history and preferences
    // Rerank results based on past behavior
    // This could use SageMaker for more sophisticated ML
    return results
}
```

### 9. **Status Search Implementation**

Lesser also provides comprehensive status/post search with multiple strategies:

```go
// Status search strategies include:
- ContentWordSearchStrategy   // Search by indexed words using GSI5
- HashtagSearchStrategy      // Search by hashtags using GSI6  
- AuthorSearchStrategy       // Search by author using GSI7
- TrendingSearchStrategy     // Search trending content using GSI8
- URLSearchStrategy          // Search for exact URL matches
```

Each strategy uses dedicated GSIs for optimal performance without requiring a separate search infrastructure.

### 10. **Cost Benefits**

By avoiding OpenSearch Serverless and using only DynamoDB with intelligent indexing:

- **No base cost**: OpenSearch Serverless has a minimum ~$700/month cost
- **Pay per use**: DynamoDB on-demand means you only pay for actual queries
- **Simpler operations**: No separate search cluster to manage
- **Better integration**: Search data stays in the same database as other data
- **Lower latency**: No cross-service calls needed

The multi-strategy approach with DynamoDB GSIs provides excellent search quality while maintaining Lesser's core principle of cost effectiveness.
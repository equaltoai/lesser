# Month 2: AI-Enhanced Search Implementation

## Overview

This document describes the AI-enhanced search features implemented for Lesser's Month 2 milestone. We've integrated AWS Bedrock for semantic embeddings, AWS Comprehend for query understanding, and enhanced OpenSearch with vector similarity search.

## Features Implemented

### 1. AWS Bedrock Integration
- **Embedding Service** (`pkg/storage/dynamodb/embeddings.go`)
  - Uses Amazon Titan Text Embeddings V1 model
  - Generates 1536-dimensional vectors for actor profiles
  - Caches embeddings in-memory for performance
  - Stores embeddings in DynamoDB for persistence

### 2. AWS Comprehend Integration
- **Query Analysis** in `pkg/storage/dynamodb/search_semantic.go`
  - Language detection
  - Entity recognition (people, places, organizations)
  - Key phrase extraction
  - Sentiment analysis
  - Helps understand search intent

### 3. Semantic Search Strategy
- **New Search Strategy** (`pkg/storage/dynamodb/search_semantic.go`)
  - Uses vector similarity for finding semantically related accounts
  - Falls back to DynamoDB if OpenSearch is unavailable
  - Generates "Did you mean?" suggestions
  - Integrates with existing multi-strategy search architecture

### 4. OpenSearch Vector Search
- **Enhanced Index Mapping** in `cmd/search-indexer/main.go`
  - Added knn_vector field for embeddings
  - HNSW algorithm for efficient similarity search
  - Cosine similarity metric
  - Real-time indexing via DynamoDB Streams

### 5. Infrastructure Updates
- **AWS Permissions** in `infra/main.go`
  - Added Bedrock model invocation permissions
  - Added Comprehend API permissions
  - Updated Lambda role policies

## Architecture

```
┌─────────────────┐     ┌──────────────────┐     ┌─────────────────┐
│  Search Request │────▶│  Search Service  │────▶│  Multi-Strategy │
└─────────────────┘     └──────────────────┘     │     Search      │
                                                  └─────────────────┘
                                                           │
                        ┌──────────────────────────────────┴───────────────┐
                        │                                                   │
                        ▼                                                   ▼
              ┌─────────────────┐                                ┌──────────────────┐
              │ Regular Search  │                                │ Semantic Search  │
              │   Strategies    │                                │    Strategy      │
              └─────────────────┘                                └──────────────────┘
                        │                                                   │
                        │                                                   ▼
                        │                                        ┌──────────────────┐
                        │                                        │  AWS Comprehend  │
                        │                                        │ Query Analysis   │
                        │                                        └──────────────────┘
                        │                                                   │
                        │                                                   ▼
                        │                                        ┌──────────────────┐
                        │                                        │  AWS Bedrock     │
                        │                                        │ Text Embeddings  │
                        │                                        └──────────────────┘
                        │                                                   │
                        ▼                                                   ▼
              ┌─────────────────┐                                ┌──────────────────┐
              │    DynamoDB     │                                │   OpenSearch     │
              │  GSI Queries    │                                │ Vector Search    │
              └─────────────────┘                                └──────────────────┘
```

## Usage

### Enable Semantic Search

To use semantic search, add the `semantic=true` parameter to search requests:

```bash
curl -H "Authorization: Bearer YOUR_TOKEN" \
  "https://your-instance.com/api/v2/search?q=software+developer&type=accounts&semantic=true"
```

### Testing

Run the comprehensive test suite:

```bash
# Test all AI features
python test_semantic_search.py https://your-instance.com --token YOUR_TOKEN

# Test specific features
python test_semantic_search.py https://your-instance.com --token YOUR_TOKEN --test similarity
python test_semantic_search.py https://your-instance.com --token YOUR_TOKEN --test typo
python test_semantic_search.py https://your-instance.com --token YOUR_TOKEN --test context
```

## Implementation Details

### Embedding Generation

When an actor profile is created or updated:
1. DynamoDB Stream triggers the search indexer Lambda
2. Lambda extracts username, display name, and bio
3. Generates embedding using AWS Bedrock Titan model
4. Stores embedding in both OpenSearch and DynamoDB

### Search Flow

1. **Query Analysis**
   - AWS Comprehend analyzes the search query
   - Detects language, entities, and key phrases
   - Determines search intent

2. **Embedding Generation**
   - Query text is converted to a 1536-dimensional vector
   - Uses the same Titan model as actor embeddings

3. **Vector Search**
   - OpenSearch performs k-NN search using HNSW algorithm
   - Returns actors with highest cosine similarity
   - Falls back to DynamoDB scan if OpenSearch fails

4. **Result Merging**
   - Semantic results are merged with other strategies
   - Scores are normalized and combined
   - Final ranking considers all strategies

### Performance Considerations

- **Caching**: Embeddings are cached in-memory to reduce Bedrock API calls
- **Parallel Processing**: Semantic search runs in parallel with other strategies
- **Graceful Degradation**: System continues working if AI services are unavailable
- **Cost Optimization**: Only generates embeddings for local actors by default

## Configuration

### Environment Variables

No additional environment variables needed - the system uses existing AWS credentials and automatically detects available services.

### AWS Services Required

1. **AWS Bedrock**
   - Enable in your AWS account
   - Request access to Amazon Titan Text Embeddings V1 model
   - No additional configuration needed

2. **AWS Comprehend**
   - Automatically available with AWS account
   - Uses standard tier (no custom models)

3. **OpenSearch Serverless**
   - Already configured in Week 3
   - Enhanced with vector search capabilities

## Cost Considerations

### AWS Bedrock
- Amazon Titan Embeddings: $0.0001 per 1,000 input tokens
- Typical actor profile: ~100 tokens
- Cost: ~$0.00001 per actor embedding

### AWS Comprehend
- $0.0001 per unit (100 characters)
- Typical search query: ~50 characters
- Cost: ~$0.00005 per search

### OpenSearch Serverless
- No additional cost for vector search
- Storage costs increase slightly due to embedding vectors
- ~6KB additional per actor (1536 floats × 4 bytes)

## Future Enhancements

### Month 3 Possibilities
1. **Personalized Search**
   - Learn from user interactions
   - Personalize rankings based on behavior
   - Use Amazon Personalize for recommendations

2. **Multi-lingual Support**
   - Use Amazon Translate for query translation
   - Generate embeddings in multiple languages
   - Cross-language semantic search

3. **Advanced Query Understanding**
   - Custom Comprehend models for social media queries
   - Intent classification (looking for friends, topics, etc.)
   - Query expansion and reformulation

4. **Federated Semantic Search**
   - Share embeddings across instances
   - Privacy-preserving vector search
   - Distributed similarity computation

## Troubleshooting

### Common Issues

1. **"Semantic search not available"**
   - Check AWS Bedrock is enabled in your region
   - Verify you have access to Titan Embeddings model
   - Check IAM permissions include bedrock:InvokeModel

2. **Slow semantic search**
   - First query may be slow due to cold start
   - Embeddings are cached after first generation
   - Consider increasing Lambda memory for better performance

3. **No semantic results**
   - Ensure OpenSearch index has been recreated with vector field
   - Run embedding generation for existing actors
   - Check OpenSearch cluster health

### Monitoring

Monitor these CloudWatch metrics:
- Lambda duration for search-indexer function
- Bedrock API throttling errors
- OpenSearch search latency
- Cache hit rates

## Conclusion

The AI-enhanced search implementation brings sophisticated semantic understanding to Lesser's search functionality. By leveraging AWS's managed AI services, we've added powerful features while maintaining the serverless architecture and keeping costs minimal. The system gracefully handles failures and provides a superior search experience when all services are available. 
package dynamodb

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	"github.com/aws/aws-sdk-go-v2/service/comprehend"
	comprehendtypes "github.com/aws/aws-sdk-go-v2/service/comprehend/types"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"go.uber.org/zap"
)

// SemanticSearchStrategy performs AI-powered semantic search
type SemanticSearchStrategy struct {
	service        *SearchService
	embedding      *EmbeddingService
	comprehend     *comprehend.Client
	opensearchURL  string
	awsCredentials aws.Credentials
	awsRegion      string
	httpClient     *http.Client
	logger         *zap.Logger
}

// NewSemanticSearchStrategy creates a new semantic search strategy
func NewSemanticSearchStrategy(service *SearchService, cfg aws.Config) (*SemanticSearchStrategy, error) {
	embeddingService, err := NewEmbeddingService(cfg, service.tableName, service.logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create embedding service: %w", err)
	}

	// Get credentials from config
	creds, err := cfg.Credentials.Retrieve(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve credentials: %w", err)
	}

	// OpenSearch endpoint - this should come from environment variable or config
	opensearchURL := os.Getenv("OPENSEARCH_ENDPOINT")
	if opensearchURL == "" {
		service.logger.Warn("OpenSearch endpoint not configured, vector search will fall back to DynamoDB")
	}

	// TEMPORARY: Force empty to use DynamoDB fallback and save costs
	opensearchURL = ""

	return &SemanticSearchStrategy{
		service:        service,
		embedding:      embeddingService,
		comprehend:     comprehend.NewFromConfig(cfg),
		opensearchURL:  opensearchURL,
		awsCredentials: creds,
		awsRegion:      cfg.Region,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		logger: service.logger,
	}, nil
}

func (s *SemanticSearchStrategy) Name() string {
	return "semantic_search"
}

// Search performs semantic search using vector embeddings
func (s *SemanticSearchStrategy) Search(ctx context.Context, query string, options SearchOptions) ([]*SearchResult, error) {
	// Analyze query with AWS Comprehend
	queryInsights, err := s.analyzeQueryWithComprehend(ctx, query)
	if err != nil {
		s.logger.Warn("failed to analyze query with Comprehend", zap.Error(err))
		// Continue without query insights
	} else {
		s.logger.Debug("query insights",
			zap.String("language", queryInsights.Language),
			zap.Int("entities", len(queryInsights.Entities)),
			zap.Int("keyphrases", len(queryInsights.KeyPhrases)))
	}

	// Generate embedding for the query
	queryEmbedding, err := s.embedding.GenerateEmbedding(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to generate query embedding: %w", err)
	}

	// Try OpenSearch vector search first
	opensearchResults, err := s.searchWithOpenSearchVectors(ctx, queryEmbedding, options.Limit)
	if err == nil && len(opensearchResults) > 0 {
		return opensearchResults, nil
	}

	// Fallback to DynamoDB cosine similarity search
	s.logger.Debug("falling back to DynamoDB vector search")
	return s.searchWithDynamoDBVectors(ctx, queryEmbedding, query, options.Limit)
}

// analyzeQueryWithComprehend uses AWS Comprehend to understand the query
func (s *SemanticSearchStrategy) analyzeQueryWithComprehend(ctx context.Context, query string) (*QueryInsights, error) {
	insights := &QueryInsights{
		OriginalQuery: query,
	}

	// Detect language
	langInput := &comprehend.DetectDominantLanguageInput{
		Text: aws.String(query),
	}
	langResp, err := s.comprehend.DetectDominantLanguage(ctx, langInput)
	if err == nil && len(langResp.Languages) > 0 {
		insights.Language = *langResp.Languages[0].LanguageCode
	}

	// Detect entities (names, places, etc.)
	if insights.Language == "en" {
		entitiesInput := &comprehend.DetectEntitiesInput{
			Text:         aws.String(query),
			LanguageCode: comprehendtypes.LanguageCodeEn,
		}
		entitiesResp, err := s.comprehend.DetectEntities(ctx, entitiesInput)
		if err == nil {
			for _, entity := range entitiesResp.Entities {
				insights.Entities = append(insights.Entities, EntityInsight{
					Text: *entity.Text,
					Type: string(entity.Type),
				})
			}
		}

		// Detect key phrases
		phrasesInput := &comprehend.DetectKeyPhrasesInput{
			Text:         aws.String(query),
			LanguageCode: comprehendtypes.LanguageCodeEn,
		}
		phrasesResp, err := s.comprehend.DetectKeyPhrases(ctx, phrasesInput)
		if err == nil {
			for _, phrase := range phrasesResp.KeyPhrases {
				insights.KeyPhrases = append(insights.KeyPhrases, *phrase.Text)
			}
		}

		// Detect sentiment (positive/negative/neutral)
		sentimentInput := &comprehend.DetectSentimentInput{
			Text:         aws.String(query),
			LanguageCode: comprehendtypes.LanguageCodeEn,
		}
		sentimentResp, err := s.comprehend.DetectSentiment(ctx, sentimentInput)
		if err == nil {
			insights.Sentiment = string(sentimentResp.Sentiment)
		}
	}

	return insights, nil
}

// searchWithOpenSearchVectors performs vector similarity search in OpenSearch
func (s *SemanticSearchStrategy) searchWithOpenSearchVectors(ctx context.Context, queryEmbedding []float32, limit int) ([]*SearchResult, error) {
	// Check if OpenSearch is configured
	if s.opensearchURL == "" {
		return nil, fmt.Errorf("OpenSearch endpoint not configured")
	}

	// OpenSearch query tracking removed - service disabled

	// Build OpenSearch vector search query
	searchBody := map[string]any{
		"size": limit,
		"query": map[string]any{
			"script_score": map[string]any{
				"query": map[string]any{
					"match_all": map[string]any{},
				},
				"script": map[string]any{
					"source": "cosineSimilarity(params.query_vector, 'embedding') + 1.0",
					"params": map[string]any{
						"query_vector": queryEmbedding,
					},
				},
			},
		},
		"_source": []string{"id", "username", "display_name", "bio"},
	}

	bodyJSON, err := json.Marshal(searchBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal search body: %w", err)
	}

	// Create the search request
	searchURL := fmt.Sprintf("%s/actors/_search", s.opensearchURL)
	req, err := http.NewRequestWithContext(ctx, "POST", searchURL, bytes.NewReader(bodyJSON))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")

	// Calculate request payload hash for signing
	hash := sha256.New()
	hash.Write(bodyJSON)
	payloadHash := hex.EncodeToString(hash.Sum(nil))

	// Sign the request with AWS Signature V4
	signer := v4.NewSigner()
	err = signer.SignHTTP(ctx, s.awsCredentials, req, payloadHash, "es", s.awsRegion, time.Now())
	if err != nil {
		return nil, fmt.Errorf("failed to sign request: %w", err)
	}

	// Execute the request
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute search request: %w", err)
	}
	defer resp.Body.Close()

	// Read response
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Check status code
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("opensearch request failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	// Parse response
	var searchResponse struct {
		Hits struct {
			Hits []struct {
				ID     string  `json:"_id"`
				Score  float64 `json:"_score"`
				Source struct {
					ID          string `json:"id"`
					Username    string `json:"username"`
					DisplayName string `json:"display_name"`
					Bio         string `json:"bio"`
				} `json:"_source"`
			} `json:"hits"`
		} `json:"hits"`
	}

	if err := json.Unmarshal(respBody, &searchResponse); err != nil {
		return nil, fmt.Errorf("failed to unmarshal search response: %w", err)
	}

	// Convert to search results
	results := make([]*SearchResult, 0, len(searchResponse.Hits.Hits))
	for _, hit := range searchResponse.Hits.Hits {
		// Fetch full actor data from DynamoDB
		actor, err := s.service.storage.GetActor(ctx, hit.Source.Username)
		if err != nil || actor == nil {
			continue
		}

		results = append(results, &SearchResult{
			Actor:         actor,
			Score:         hit.Score / 2.0, // Normalize score (cosine similarity + 1.0 gives max 2.0)
			MatchedFields: []string{"semantic"},
			Highlights: map[string]string{
				"match_type": "semantic_similarity",
			},
		})
	}

	return results, nil
}

// searchWithDynamoDBVectors performs vector similarity search using DynamoDB
func (s *SemanticSearchStrategy) searchWithDynamoDBVectors(ctx context.Context, queryEmbedding []float32, _ string, limit int) ([]*SearchResult, error) {
	// Scan all embeddings (this is not efficient for large datasets)
	// In production, you'd want to use a proper vector database or OpenSearch
	scanInput := &dynamodb.ScanInput{
		TableName:        aws.String(s.service.tableName),
		FilterExpression: aws.String("begins_with(PK, :pk) AND SK = :sk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: "EMBEDDING#"},
			":sk": &types.AttributeValueMemberS{Value: "VECTOR"},
		},
		Limit: aws.Int32(100), // Limit scan to avoid timeouts
	}

	resp, err := s.service.dynamo.Scan(ctx, scanInput)
	if err != nil {
		return nil, fmt.Errorf("failed to scan embeddings: %w", err)
	}

	// Calculate similarities
	type ScoredActor struct {
		ActorID string
		Score   float32
	}

	scoredActors := make([]ScoredActor, 0)

	for _, item := range resp.Items {
		// Extract actor ID
		pk := item["PK"].(*types.AttributeValueMemberS).Value
		actorID := strings.TrimPrefix(pk, "EMBEDDING#")

		// Extract embedding
		embeddingAttr, ok := item["Embedding"].(*types.AttributeValueMemberB)
		if !ok {
			continue
		}

		var embedding []float32
		if err := json.Unmarshal(embeddingAttr.Value, &embedding); err != nil {
			continue
		}

		// Calculate cosine similarity
		similarity := CosineSimilarity(queryEmbedding, embedding)
		if similarity > 0.5 { // Threshold for relevance
			scoredActors = append(scoredActors, ScoredActor{
				ActorID: actorID,
				Score:   similarity,
			})
		}
	}

	// Sort by score
	sort.Slice(scoredActors, func(i, j int) bool {
		return scoredActors[i].Score > scoredActors[j].Score
	})

	// Limit results
	if len(scoredActors) > limit {
		scoredActors = scoredActors[:limit]
	}

	// Fetch actor details
	results := make([]*SearchResult, 0, len(scoredActors))
	for _, scored := range scoredActors {
		actor, err := s.service.storage.GetActor(ctx, scored.ActorID)
		if err != nil || actor == nil {
			continue
		}

		results = append(results, &SearchResult{
			Actor:         actor,
			Score:         float64(scored.Score),
			MatchedFields: []string{"semantic"},
			Highlights: map[string]string{
				"match_type": "semantic_similarity",
				"similarity": fmt.Sprintf("%.2f", scored.Score),
			},
		})
	}

	return results, nil
}

// QueryInsights represents insights from AWS Comprehend
type QueryInsights struct {
	OriginalQuery string
	Language      string
	Entities      []EntityInsight
	KeyPhrases    []string
	Sentiment     string
}

// EntityInsight represents an entity detected by Comprehend
type EntityInsight struct {
	Text string
	Type string // PERSON, LOCATION, ORGANIZATION, etc.
}

// GenerateDidYouMean generates "Did you mean?" suggestions using semantic similarity
func (s *SemanticSearchStrategy) GenerateDidYouMean(ctx context.Context, query string) ([]string, error) {
	// Get query embedding
	queryEmbedding, err := s.embedding.GenerateEmbedding(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to generate query embedding: %w", err)
	}

	// Get popular queries from analytics
	recentQueries, err := s.service.analytics.GetPopularQueries(ctx, 100)
	if err != nil {
		return nil, fmt.Errorf("failed to get popular queries: %w", err)
	}

	// Calculate similarity with recent queries
	type ScoredQuery struct {
		Query string
		Score float32
	}

	scoredQueries := make([]ScoredQuery, 0)

	for _, recentQuery := range recentQueries {
		if recentQuery == query {
			continue // Skip the same query
		}

		// Generate embedding for recent query
		recentEmbedding, err := s.embedding.GenerateEmbedding(ctx, recentQuery)
		if err != nil {
			continue
		}

		// Calculate similarity
		similarity := CosineSimilarity(queryEmbedding, recentEmbedding)
		if similarity > 0.7 && similarity < 0.95 { // Similar but not identical
			scoredQueries = append(scoredQueries, ScoredQuery{
				Query: recentQuery,
				Score: similarity,
			})
		}
	}

	// Sort by score
	sort.Slice(scoredQueries, func(i, j int) bool {
		return scoredQueries[i].Score > scoredQueries[j].Score
	})

	// Return top suggestions
	suggestions := make([]string, 0, 3)
	for i, sq := range scoredQueries {
		if i >= 3 {
			break
		}
		suggestions = append(suggestions, sq.Query)
	}

	return suggestions, nil
}

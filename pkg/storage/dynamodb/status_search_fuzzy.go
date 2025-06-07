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
	awsdynamodb "github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"go.uber.org/zap"
)

// StatusFuzzySearchStrategy provides fuzzy search capabilities using OpenSearch
type StatusFuzzySearchStrategy struct {
	service        *StatusSearchService
	opensearchURL  string
	awsCredentials aws.Credentials
	awsRegion      string
	httpClient     *http.Client
}

// NewStatusFuzzySearchStrategy creates a new fuzzy search strategy
func NewStatusFuzzySearchStrategy(service *StatusSearchService, cfg aws.Config) (*StatusFuzzySearchStrategy, error) {
	// TEMPORARY: Disable OpenSearch to save costs
	return nil, fmt.Errorf("OpenSearch is disabled")

	// Get credentials from config
	creds, err := cfg.Credentials.Retrieve(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve credentials: %w", err)
	}

	// OpenSearch endpoint - this should come from environment variable or config
	opensearchURL := os.Getenv("OPENSEARCH_ENDPOINT")
	if opensearchURL == "" {
		return nil, fmt.Errorf("OpenSearch endpoint not configured")
	}

	return &StatusFuzzySearchStrategy{
		service:        service,
		opensearchURL:  opensearchURL,
		awsCredentials: creds,
		awsRegion:      cfg.Region,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}, nil
}

func (s *StatusFuzzySearchStrategy) Name() string {
	return "status_fuzzy_search"
}

func (s *StatusFuzzySearchStrategy) Search(ctx context.Context, query string, options StatusSearchOptions) ([]*StatusSearchResult, error) {
	// Build OpenSearch fuzzy search query
	searchBody := map[string]interface{}{
		"size": options.Limit,
		"query": map[string]interface{}{
			"multi_match": map[string]interface{}{
				"query":     query,
				"fields":    []string{"content^2", "author_username", "hashtags"},
				"fuzziness": "AUTO",
				"type":      "best_fields",
				"operator":  "or",
			},
		},
		"highlight": map[string]interface{}{
			"fields": map[string]interface{}{
				"content": map[string]interface{}{
					"fragment_size":       150,
					"number_of_fragments": 3,
				},
			},
		},
		"_source": []string{"status_id", "content", "author_id", "author_username", "published", "likes_count", "boosts_count", "replies_count", "language", "has_media", "visibility"},
	}

	// Apply filters if specified
	filters := []map[string]interface{}{}

	if options.LocalOnly {
		filters = append(filters, map[string]interface{}{
			"term": map[string]interface{}{"is_local": true},
		})
	}

	if options.MediaOnly {
		filters = append(filters, map[string]interface{}{
			"term": map[string]interface{}{"has_media": true},
		})
	}

	if options.AccountID != "" {
		filters = append(filters, map[string]interface{}{
			"term": map[string]interface{}{"author_id": options.AccountID},
		})
	}

	if options.Language != "" {
		filters = append(filters, map[string]interface{}{
			"term": map[string]interface{}{"language": options.Language},
		})
	}

	if !options.TimeRange.Start.IsZero() || !options.TimeRange.End.IsZero() {
		rangeFilter := map[string]interface{}{}
		if !options.TimeRange.Start.IsZero() {
			rangeFilter["gte"] = options.TimeRange.Start.Format(time.RFC3339)
		}
		if !options.TimeRange.End.IsZero() {
			rangeFilter["lte"] = options.TimeRange.End.Format(time.RFC3339)
		}
		filters = append(filters, map[string]interface{}{
			"range": map[string]interface{}{"published": rangeFilter},
		})
	}

	if len(filters) > 0 {
		searchBody["query"] = map[string]interface{}{
			"bool": map[string]interface{}{
				"must":   searchBody["query"],
				"filter": filters,
			},
		}
	}

	// Execute the search
	bodyJSON, err := json.Marshal(searchBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal search body: %w", err)
	}

	// Create the search request
	searchURL := fmt.Sprintf("%s/statuses/_search", s.opensearchURL)
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
		// If index doesn't exist or OpenSearch has issues, return empty results
		s.service.logger.Warn("opensearch request failed",
			zap.Int("status", resp.StatusCode),
			zap.String("body", string(respBody)))
		return []*StatusSearchResult{}, nil
	}

	// Parse response
	var searchResponse struct {
		Hits struct {
			Total struct {
				Value int `json:"value"`
			} `json:"total"`
			Hits []struct {
				ID     string  `json:"_id"`
				Score  float64 `json:"_score"`
				Source struct {
					StatusID       string `json:"status_id"`
					Content        string `json:"content"`
					AuthorID       string `json:"author_id"`
					AuthorUsername string `json:"author_username"`
					Published      string `json:"published"`
					LikesCount     int    `json:"likes_count"`
					BoostsCount    int    `json:"boosts_count"`
					RepliesCount   int    `json:"replies_count"`
					Language       string `json:"language"`
					HasMedia       bool   `json:"has_media"`
					Visibility     string `json:"visibility"`
				} `json:"_source"`
				Highlight map[string][]string `json:"highlight"`
			} `json:"hits"`
		} `json:"hits"`
	}

	if err := json.Unmarshal(respBody, &searchResponse); err != nil {
		return nil, fmt.Errorf("failed to unmarshal search response: %w", err)
	}

	// Convert to StatusSearchResult
	results := make([]*StatusSearchResult, 0, len(searchResponse.Hits.Hits))
	for _, hit := range searchResponse.Hits.Hits {
		publishedTime, _ := time.Parse(time.RFC3339, hit.Source.Published)

		result := &StatusSearchResult{
			StatusID:       hit.Source.StatusID,
			Content:        hit.Source.Content,
			AuthorID:       hit.Source.AuthorID,
			AuthorUsername: hit.Source.AuthorUsername,
			Published:      publishedTime,
			Score:          hit.Score,
			MatchedFields:  []string{"fuzzy"},
			Highlights:     make(map[string]string),
			LikesCount:     hit.Source.LikesCount,
			BoostsCount:    hit.Source.BoostsCount,
			RepliesCount:   hit.Source.RepliesCount,
			Language:       hit.Source.Language,
			HasMedia:       hit.Source.HasMedia,
			Visibility:     hit.Source.Visibility,
		}

		// Add highlights
		if highlights, ok := hit.Highlight["content"]; ok && len(highlights) > 0 {
			result.Highlights["content"] = highlights[0]
		}

		results = append(results, result)
	}

	return results, nil
}

// StatusSemanticSearchStrategy provides semantic search using AI embeddings
type StatusSemanticSearchStrategy struct {
	service    *StatusSearchService
	embeddings *EmbeddingService
}

func (s *StatusSemanticSearchStrategy) Name() string {
	return "status_semantic_search"
}

func (s *StatusSemanticSearchStrategy) Search(ctx context.Context, query string, options StatusSearchOptions) ([]*StatusSearchResult, error) {
	// Generate embedding for the query
	queryEmbedding, err := s.embeddings.GenerateEmbedding(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to generate query embedding: %w", err)
	}

	// For now, perform a scan-based similarity search
	// In production, this would use a vector database or OpenSearch k-NN
	results, err := s.scanAndMatchEmbeddings(ctx, queryEmbedding, options)
	if err != nil {
		return nil, fmt.Errorf("failed to scan embeddings: %w", err)
	}

	return results, nil
}

// scanAndMatchEmbeddings performs a DynamoDB scan for embeddings (not efficient for large datasets)
func (s *StatusSemanticSearchStrategy) scanAndMatchEmbeddings(ctx context.Context, queryEmbedding []float32, options StatusSearchOptions) ([]*StatusSearchResult, error) {
	// Scan status embeddings
	scanInput := &awsdynamodb.ScanInput{
		TableName:        aws.String(s.service.tableName),
		FilterExpression: aws.String("begins_with(PK, :pk) AND SK = :sk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: "STATUS_EMBEDDING#"},
			":sk": &types.AttributeValueMemberS{Value: "VECTOR"},
		},
		Limit: aws.Int32(500), // Limit scan to avoid timeouts
	}

	resp, err := s.service.dynamo.Scan(ctx, scanInput)
	if err != nil {
		return nil, fmt.Errorf("failed to scan embeddings: %w", err)
	}

	// Calculate similarities
	type ScoredStatus struct {
		StatusID string
		Score    float32
	}

	scoredStatuses := make([]ScoredStatus, 0)

	for _, item := range resp.Items {
		// Extract status ID
		pk := item["PK"].(*types.AttributeValueMemberS).Value
		statusID := strings.TrimPrefix(pk, "STATUS_EMBEDDING#")

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
		if similarity > 0.6 { // Threshold for relevance
			scoredStatuses = append(scoredStatuses, ScoredStatus{
				StatusID: statusID,
				Score:    similarity,
			})
		}
	}

	// Sort by score
	sort.Slice(scoredStatuses, func(i, j int) bool {
		return scoredStatuses[i].Score > scoredStatuses[j].Score
	})

	// Limit results
	if len(scoredStatuses) > options.Limit {
		scoredStatuses = scoredStatuses[:options.Limit]
	}

	// Fetch status details
	results := make([]*StatusSearchResult, 0, len(scoredStatuses))
	for _, scored := range scoredStatuses {
		// Get status from DynamoDB
		obj, err := s.service.storage.GetObject(ctx, scored.StatusID)
		if err != nil || obj == nil {
			continue
		}

		// Type assert to Object type
		var content, authorID, authorUsername string
		var published time.Time
		var likesCount, boostsCount, repliesCount int

		switch v := obj.(type) {
		case *Object:
			content = v.Content
			authorID = v.AttributedTo
			published = v.Published

			// Extract author username from authorID
			if authorID != "" {
				parts := strings.Split(authorID, "/")
				if len(parts) > 0 {
					authorUsername = parts[len(parts)-1]
				}
			}

		case map[string]interface{}:
			// Handle map representation
			if contentVal, ok := v["content"].(string); ok {
				content = contentVal
			}
			if authorVal, ok := v["attributedTo"].(string); ok {
				authorID = authorVal
				parts := strings.Split(authorID, "/")
				if len(parts) > 0 {
					authorUsername = parts[len(parts)-1]
				}
			}
			if pubVal, ok := v["published"].(string); ok {
				published, _ = time.Parse(time.RFC3339, pubVal)
			}

			// Get engagement metrics if available
			if likes, ok := v["likes"].(map[string]interface{}); ok {
				if items, ok := likes["totalItems"].(float64); ok {
					likesCount = int(items)
				}
			}
			if shares, ok := v["shares"].(map[string]interface{}); ok {
				if items, ok := shares["totalItems"].(float64); ok {
					boostsCount = int(items)
				}
			}
			if replies, ok := v["replies"].(map[string]interface{}); ok {
				if items, ok := replies["totalItems"].(float64); ok {
					repliesCount = int(items)
				}
			}

		default:
			// Skip unknown types
			continue
		}

		result := &StatusSearchResult{
			StatusID:       scored.StatusID,
			Content:        content,
			AuthorID:       authorID,
			AuthorUsername: authorUsername,
			Published:      published,
			Score:          float64(scored.Score),
			MatchedFields:  []string{"semantic"},
			Highlights: map[string]string{
				"match_type": "semantic_similarity",
				"similarity": fmt.Sprintf("%.2f", scored.Score),
			},
			LikesCount:   likesCount,
			BoostsCount:  boostsCount,
			RepliesCount: repliesCount,
		}

		results = append(results, result)
	}

	return results, nil
}

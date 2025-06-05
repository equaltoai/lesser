package dynamodb

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/aron23/lesser/pkg/activitypub"
	"github.com/aron23/lesser/pkg/common"
	"github.com/aron23/lesser/pkg/cost"
	"github.com/aws/aws-sdk-go-v2/config"
	opensearch "github.com/opensearch-project/opensearch-go/v2"
	opensearchapi "github.com/opensearch-project/opensearch-go/v2/opensearchapi"
	requestsigner "github.com/opensearch-project/opensearch-go/v2/signer/awsv2"
	"go.uber.org/zap"
)

// FuzzySearchStrategy implements fuzzy search using OpenSearch
type FuzzySearchStrategy struct {
	service  *SearchService
	osClient *opensearch.Client
	domain   string
}

// NewFuzzySearchStrategy creates a new fuzzy search strategy
func NewFuzzySearchStrategy(service *SearchService) (SearchStrategy, error) {
	opensearchEndpoint := os.Getenv("OPENSEARCH_ENDPOINT")
	if opensearchEndpoint == "" {
		return nil, fmt.Errorf("OPENSEARCH_ENDPOINT environment variable is required")
	}

	domain := os.Getenv("DOMAIN")
	if domain == "" {
		return nil, fmt.Errorf("DOMAIN environment variable is required")
	}

	// Load AWS configuration
	cfg, err := config.LoadDefaultConfig(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	// Create the signer
	signer, err := requestsigner.NewSignerWithService(cfg, "aoss")
	if err != nil {
		return nil, fmt.Errorf("failed to create signer: %w", err)
	}

	// Create OpenSearch client
	client, err := opensearch.NewClient(opensearch.Config{
		Addresses: []string{opensearchEndpoint},
		Signer:    signer,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: false},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create OpenSearch client: %w", err)
	}

	return &FuzzySearchStrategy{
		service:  service,
		osClient: client,
		domain:   domain,
	}, nil
}

// Name returns the strategy name
func (s *FuzzySearchStrategy) Name() string {
	return "fuzzy_search"
}

// Search performs fuzzy search using OpenSearch
func (s *FuzzySearchStrategy) Search(ctx context.Context, query string, options SearchOptions) ([]*SearchResult, error) {
	// Track OpenSearch query for cost purposes
	cost.TrackOpenSearchQueryContext(ctx, 1)

	log := common.WithContext(ctx)

	// Build the search query
	searchBody := map[string]interface{}{
		"query": map[string]interface{}{
			"bool": map[string]interface{}{
				"should": []interface{}{
					// Exact match on username (highest boost)
					map[string]interface{}{
						"term": map[string]interface{}{
							"username": map[string]interface{}{
								"value": strings.ToLower(query),
								"boost": 10.0,
							},
						},
					},
					// Fuzzy match on username
					map[string]interface{}{
						"fuzzy": map[string]interface{}{
							"username": map[string]interface{}{
								"value":     strings.ToLower(query),
								"fuzziness": "AUTO",
								"boost":     5.0,
							},
						},
					},
					// Match on display name with fuzziness
					map[string]interface{}{
						"match": map[string]interface{}{
							"display_name": map[string]interface{}{
								"query":     query,
								"fuzziness": "AUTO",
								"boost":     3.0,
							},
						},
					},
					// Match on bio (lower boost)
					map[string]interface{}{
						"match": map[string]interface{}{
							"bio": map[string]interface{}{
								"query":     query,
								"fuzziness": "AUTO",
								"boost":     1.0,
							},
						},
					},
				},
				"minimum_should_match": 1,
			},
		},
		"size": options.Limit,
		"from": options.Offset,
		"highlight": map[string]interface{}{
			"fields": map[string]interface{}{
				"username":     map[string]interface{}{},
				"display_name": map[string]interface{}{},
				"bio": map[string]interface{}{
					"fragment_size":       150,
					"number_of_fragments": 1,
				},
			},
		},
		"_source": []string{
			"id", "username", "display_name", "bio", "domain",
			"followers_count", "following_count", "statuses_count",
			"created_at", "updated_at", "is_local",
		},
	}

	// Apply filters if specified
	filters := make([]interface{}, 0)

	// For now, we'll filter by following only if requested
	// Additional filters can be added as needed
	if options.FollowingOnly {
		// This would require additional indexing of relationship data
		// For now, we'll skip this filter
		log.Warn("following_only filter not yet implemented for fuzzy search")
	}

	// Add filters to query if any
	if len(filters) > 0 {
		boolQuery := searchBody["query"].(map[string]interface{})["bool"].(map[string]interface{})
		boolQuery["filter"] = filters
	}

	// Serialize the search body
	body, err := json.Marshal(searchBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal search body: %w", err)
	}

	// Create the search request
	req := opensearchapi.SearchRequest{
		Index: []string{"actors"},
		Body:  bytes.NewReader(body),
	}

	// Execute the search
	res, err := req.Do(ctx, s.osClient)
	if err != nil {
		return nil, fmt.Errorf("failed to execute search: %w", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		return nil, fmt.Errorf("search failed: %s", res.String())
	}

	// Parse the response
	var response map[string]interface{}
	if err := json.NewDecoder(res.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to decode search response: %w", err)
	}

	// Extract hits
	hits, ok := response["hits"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid search response format")
	}

	hitsArray, ok := hits["hits"].([]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid hits format in search response")
	}

	// Convert hits to SearchResults
	results := make([]*SearchResult, 0, len(hitsArray))
	for _, hit := range hitsArray {
		hitMap, ok := hit.(map[string]interface{})
		if !ok {
			continue
		}

		// Extract source
		source, ok := hitMap["_source"].(map[string]interface{})
		if !ok {
			continue
		}

		// Extract score
		score, _ := hitMap["_score"].(float64)

		// Extract highlights
		var highlights map[string]string
		if highlightMap, ok := hitMap["highlight"].(map[string]interface{}); ok {
			highlights = make(map[string]string)
			for field, values := range highlightMap {
				if valueArray, ok := values.([]interface{}); ok && len(valueArray) > 0 {
					if str, ok := valueArray[0].(string); ok {
						highlights[field] = str
					}
				}
			}
		}

		// Build the actor object
		actor := &activitypub.Actor{
			BaseObject: activitypub.BaseObject{
				ID:   getString(source, "id"),
				Type: "Person",
			},
			Name:              getString(source, "display_name"),
			Summary:           getString(source, "bio"),
			PreferredUsername: getString(source, "username"),
		}

		// Set URLs
		if getString(source, "domain") == s.domain {
			actor.URL = fmt.Sprintf("https://%s/@%s", s.domain, actor.PreferredUsername)
			actor.Inbox = fmt.Sprintf("https://%s/users/%s/inbox", s.domain, actor.PreferredUsername)
			actor.Outbox = fmt.Sprintf("https://%s/users/%s/outbox", s.domain, actor.PreferredUsername)
		} else {
			// For remote actors, we store the full ID
			actor.URL = actor.ID
		}

		// Determine matched fields
		matchedFields := make([]string, 0)
		if highlights != nil {
			for field := range highlights {
				matchedFields = append(matchedFields, field)
			}
		}

		result := &SearchResult{
			Actor:         actor,
			Score:         score,
			MatchedFields: matchedFields,
			Highlights:    highlights,
			Strategy:      "fuzzy",
		}

		results = append(results, result)
	}

	// Normalize scores to be between 0.4 and 0.8 for fuzzy matches
	s.normalizeScores(results, 0.4, 0.8)

	log.Info("fuzzy search completed",
		zap.String("query", query),
		zap.Int("results", len(results)))

	return results, nil
}

// normalizeScores adjusts scores to fit within the given range
func (s *FuzzySearchStrategy) normalizeScores(results []*SearchResult, minScore, maxScore float64) {
	if len(results) == 0 {
		return
	}

	// Find min and max scores
	min, max := results[0].Score, results[0].Score
	for _, r := range results {
		if r.Score < min {
			min = r.Score
		}
		if r.Score > max {
			max = r.Score
		}
	}

	// Normalize
	scoreRange := max - min
	if scoreRange == 0 {
		// All scores are the same
		for _, r := range results {
			r.Score = (minScore + maxScore) / 2
		}
		return
	}

	targetRange := maxScore - minScore
	for _, r := range results {
		normalized := (r.Score - min) / scoreRange
		r.Score = minScore + (normalized * targetRange)
	}
}

// getString safely extracts a string value from a map
func getString(m map[string]interface{}, key string) string {
	if val, ok := m[key].(string); ok {
		return val
	}
	return ""
}

// IsAvailable checks if fuzzy search is available
func (s *FuzzySearchStrategy) IsAvailable() bool {
	// Check if we can connect to OpenSearch
	res, err := s.osClient.Ping()
	if err != nil {
		return false
	}
	defer res.Body.Close()

	return res.StatusCode == http.StatusOK
}

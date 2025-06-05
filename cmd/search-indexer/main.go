package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/aron23/lesser/pkg/activitypub"
	"github.com/aron23/lesser/pkg/config"
	"github.com/aron23/lesser/pkg/cost"
	"github.com/aron23/lesser/pkg/storage/dynamodb"
	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	opensearch "github.com/opensearch-project/opensearch-go/v2"
	opensearchapi "github.com/opensearch-project/opensearch-go/v2/opensearchapi"
	requestsigner "github.com/opensearch-project/opensearch-go/v2/signer/awsv2"
	"go.uber.org/zap"
)

type SearchIndexer struct {
	osClient  *opensearch.Client
	domain    string
	embedding *dynamodb.EmbeddingService
	logger    *zap.Logger
}

// Actor represents the data structure we'll index in OpenSearch
type ActorDocument struct {
	ID             string    `json:"id"`
	Username       string    `json:"username"`
	DisplayName    string    `json:"display_name"`
	Bio            string    `json:"bio"`
	Domain         string    `json:"domain"`
	FollowersCount int       `json:"followers_count"`
	FollowingCount int       `json:"following_count"`
	StatusesCount  int       `json:"statuses_count"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
	IndexedAt      time.Time `json:"indexed_at"`
	IsLocal        bool      `json:"is_local"`
	Embedding      []float32 `json:"embedding,omitempty"` // Vector embedding for semantic search
}

func NewSearchIndexer() (*SearchIndexer, error) {
	opensearchEndpoint := os.Getenv("OPENSEARCH_ENDPOINT")
	if opensearchEndpoint == "" {
		return nil, fmt.Errorf("OPENSEARCH_ENDPOINT environment variable is required")
	}

	domain := os.Getenv("DOMAIN")
	if domain == "" {
		return nil, fmt.Errorf("DOMAIN environment variable is required")
	}

	tableName := os.Getenv("DYNAMO_TABLE_NAME")
	if tableName == "" {
		return nil, fmt.Errorf("DYNAMO_TABLE_NAME environment variable is required")
	}

	// Create logger
	logger, err := zap.NewProduction()
	if err != nil {
		return nil, fmt.Errorf("failed to create logger: %w", err)
	}

	// Initialize AWS config
	ctx := context.Background()
	cfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion(config.Get().Region),
	)
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

	// Verify connection
	res, err := client.Ping()
	if err != nil {
		return nil, fmt.Errorf("failed to ping OpenSearch: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("OpenSearch ping failed with status: %d", res.StatusCode)
	}

	// Create embedding service
	embeddingService, err := dynamodb.NewEmbeddingService(cfg, tableName, logger)
	if err != nil {
		logger.Warn("Failed to create embedding service, continuing without embeddings", zap.Error(err))
		// Don't fail - allow indexing to continue without embeddings
		embeddingService = nil
	}

	return &SearchIndexer{
		osClient:  client,
		domain:    domain,
		embedding: embeddingService,
		logger:    logger,
	}, nil
}

func (si *SearchIndexer) ensureIndex(ctx context.Context) error {
	indexName := "actors"

	// Check if index exists
	res, err := si.osClient.Indices.Exists([]string{indexName})
	if err != nil {
		return fmt.Errorf("failed to check index existence: %w", err)
	}
	defer res.Body.Close()

	// If index doesn't exist, create it
	if res.StatusCode == 404 {
		mapping := map[string]interface{}{
			"mappings": map[string]interface{}{
				"properties": map[string]interface{}{
					"id":       map[string]interface{}{"type": "keyword"},
					"username": map[string]interface{}{"type": "keyword"},
					"display_name": map[string]interface{}{
						"type": "text",
						"fields": map[string]interface{}{
							"keyword": map[string]interface{}{"type": "keyword"},
						},
					},
					"bio": map[string]interface{}{
						"type":     "text",
						"analyzer": "standard",
					},
					"domain":          map[string]interface{}{"type": "keyword"},
					"followers_count": map[string]interface{}{"type": "integer"},
					"following_count": map[string]interface{}{"type": "integer"},
					"statuses_count":  map[string]interface{}{"type": "integer"},
					"created_at":      map[string]interface{}{"type": "date"},
					"updated_at":      map[string]interface{}{"type": "date"},
					"indexed_at":      map[string]interface{}{"type": "date"},
					"is_local":        map[string]interface{}{"type": "boolean"},
					"embedding": map[string]interface{}{
						"type":      "knn_vector",
						"dimension": 1536, // AWS Titan embedding dimension
						"method": map[string]interface{}{
							"name":       "hnsw",
							"space_type": "cosinesimil",
							"engine":     "nmslib",
						},
					},
				},
			},
			"settings": map[string]interface{}{
				"number_of_shards":   1,
				"number_of_replicas": 1,
				"analysis": map[string]interface{}{
					"analyzer": map[string]interface{}{
						"username_analyzer": map[string]interface{}{
							"type":      "custom",
							"tokenizer": "lowercase",
							"filter":    []string{"edge_ngram_filter"},
						},
					},
					"filter": map[string]interface{}{
						"edge_ngram_filter": map[string]interface{}{
							"type":     "edge_ngram",
							"min_gram": 2,
							"max_gram": 20,
						},
					},
				},
			},
		}

		body, err := json.Marshal(mapping)
		if err != nil {
			return fmt.Errorf("failed to marshal index mapping: %w", err)
		}

		req := opensearchapi.IndicesCreateRequest{
			Index: indexName,
			Body:  bytes.NewReader(body),
		}

		res, err := req.Do(ctx, si.osClient)
		if err != nil {
			return fmt.Errorf("failed to create index: %w", err)
		}
		defer res.Body.Close()

		if res.IsError() {
			return fmt.Errorf("failed to create index: %s", res.String())
		}

		log.Printf("Created actors index successfully")
	}

	return nil
}

func (si *SearchIndexer) handleRecord(ctx context.Context, record events.DynamoDBEventRecord) error {
	switch record.EventName {
	case "INSERT", "MODIFY":
		return si.indexActor(ctx, record.Change.NewImage)
	case "REMOVE":
		return si.deleteActor(ctx, record.Change.OldImage)
	default:
		log.Printf("Skipping unsupported event type: %s", record.EventName)
		return nil
	}
}

func (si *SearchIndexer) indexActor(ctx context.Context, item map[string]events.DynamoDBAttributeValue) error {
	// Track OpenSearch indexing for cost purposes
	cost.TrackOpenSearchIndexContext(ctx, 1)

	// Extract PK to ensure it's an actor
	pk, ok := item["PK"]
	if !ok || !strings.HasPrefix(pk.String(), "ACTOR#") {
		return nil // Not an actor, skip
	}

	// Extract actor ID
	actorID := strings.TrimPrefix(pk.String(), "ACTOR#")

	// Parse the DynamoDB item into ActorDocument
	doc := ActorDocument{
		ID:        actorID,
		IndexedAt: time.Now(),
	}

	// Extract username
	if val, ok := item["Username"]; ok {
		doc.Username = val.String()
	}

	// Extract display name
	if val, ok := item["Name"]; ok {
		doc.DisplayName = val.String()
	}

	// Extract bio/summary
	if val, ok := item["Summary"]; ok {
		doc.Bio = val.String()
	}

	// Extract domain
	if val, ok := item["Domain"]; ok {
		doc.Domain = val.String()
		doc.IsLocal = doc.Domain == si.domain
	}

	// Extract counts
	if val, ok := item["FollowersCount"]; ok {
		fmt.Sscanf(val.Number(), "%d", &doc.FollowersCount)
	}
	if val, ok := item["FollowingCount"]; ok {
		fmt.Sscanf(val.Number(), "%d", &doc.FollowingCount)
	}
	if val, ok := item["StatusesCount"]; ok {
		fmt.Sscanf(val.Number(), "%d", &doc.StatusesCount)
	}

	// Extract timestamps
	if val, ok := item["CreatedAt"]; ok {
		if t, err := time.Parse(time.RFC3339, val.String()); err == nil {
			doc.CreatedAt = t
		}
	}
	if val, ok := item["UpdatedAt"]; ok {
		if t, err := time.Parse(time.RFC3339, val.String()); err == nil {
			doc.UpdatedAt = t
		}
	}

	// Generate embedding if service is available
	if si.embedding != nil {
		actor := &activitypub.Actor{
			PreferredUsername: doc.Username,
			Name:              doc.DisplayName,
			Summary:           doc.Bio,
		}

		embedding, err := si.embedding.GenerateActorEmbedding(ctx, actor)
		if err != nil {
			si.logger.Warn("Failed to generate embedding for actor",
				zap.String("actor", actorID),
				zap.Error(err))
		} else {
			doc.Embedding = embedding
			// Also store embedding in DynamoDB for fallback search
			if err := si.embedding.StoreActorEmbedding(ctx, actorID, embedding); err != nil {
				si.logger.Warn("Failed to store embedding in DynamoDB",
					zap.String("actor", actorID),
					zap.Error(err))
			}
		}
	}

	// Index the document
	body, err := json.Marshal(doc)
	if err != nil {
		return fmt.Errorf("failed to marshal actor document: %w", err)
	}

	req := opensearchapi.IndexRequest{
		Index:      "actors",
		DocumentID: doc.ID,
		Body:       bytes.NewReader(body),
		Refresh:    "true",
	}

	res, err := req.Do(ctx, si.osClient)
	if err != nil {
		return fmt.Errorf("failed to index actor: %w", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		return fmt.Errorf("failed to index actor %s: %s", doc.ID, res.String())
	}

	log.Printf("Successfully indexed actor: %s (@%s)", doc.ID, doc.Username)
	return nil
}

func (si *SearchIndexer) deleteActor(ctx context.Context, item map[string]events.DynamoDBAttributeValue) error {
	// Extract PK to ensure it's an actor
	pk, ok := item["PK"]
	if !ok || !strings.HasPrefix(pk.String(), "ACTOR#") {
		return nil // Not an actor, skip
	}

	// Extract actor ID
	actorID := strings.TrimPrefix(pk.String(), "ACTOR#")

	// Delete the actor from the index
	req := opensearchapi.DeleteRequest{
		Index:      "actors",
		DocumentID: actorID,
	}

	// Track OpenSearch delete operation (counts as indexing for cost purposes)
	cost.TrackOpenSearchIndexContext(ctx, 1)

	res, err := req.Do(ctx, si.osClient)
	if err != nil {
		return fmt.Errorf("failed to delete actor: %w", err)
	}
	defer res.Body.Close()

	if res.IsError() && res.StatusCode != 404 {
		return fmt.Errorf("failed to delete actor %s: %s", actorID, res.String())
	}

	log.Printf("Successfully deleted actor from index: %s", actorID)
	return nil
}

func (si *SearchIndexer) Handler(ctx context.Context, event events.DynamoDBEvent) error {
	// Ensure index exists
	if err := si.ensureIndex(ctx); err != nil {
		log.Printf("Failed to ensure index: %v", err)
		// Continue processing even if index check fails
	}

	var errors []error
	for _, record := range event.Records {
		if err := si.handleRecord(ctx, record); err != nil {
			log.Printf("Error processing record: %v", err)
			errors = append(errors, err)
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("processed %d records with %d errors", len(event.Records), len(errors))
	}

	log.Printf("Successfully processed %d records", len(event.Records))
	return nil
}

func main() {
	indexer, err := NewSearchIndexer()
	if err != nil {
		log.Fatalf("Failed to create search indexer: %v", err)
	}

	lambda.Start(indexer.Handler)
}

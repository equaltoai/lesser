package dynamodb

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"go.uber.org/zap"
)

// EmbeddingService handles vector embeddings for semantic search
type EmbeddingService struct {
	bedrock      *bedrockruntime.Client
	dynamo       *dynamodb.Client
	tableName    string
	logger       *zap.Logger
	modelID      string
	embeddingDim int
	cache        sync.Map // Simple in-memory cache for embeddings
}

// NewEmbeddingService creates a new embedding service
func NewEmbeddingService(cfg aws.Config, tableName string, logger *zap.Logger) (*EmbeddingService, error) {
	return &EmbeddingService{
		bedrock:      bedrockruntime.NewFromConfig(cfg),
		dynamo:       dynamodb.NewFromConfig(cfg),
		tableName:    tableName,
		logger:       logger,
		modelID:      "amazon.titan-embed-text-v1", // AWS Titan embeddings model
		embeddingDim: 1536,                         // Titan model dimension
	}, nil
}

// GenerateEmbedding creates a vector embedding for the given text
func (s *EmbeddingService) GenerateEmbedding(ctx context.Context, text string) ([]float32, error) {
	// Check cache first
	if cached, ok := s.cache.Load(text); ok {
		return cached.([]float32), nil
	}

	// Prepare the request for Titan embeddings model
	requestBody := map[string]any{
		"inputText": text,
	}

	bodyBytes, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Invoke the model
	resp, err := s.bedrock.InvokeModel(ctx, &bedrockruntime.InvokeModelInput{
		ModelId:     aws.String(s.modelID),
		ContentType: aws.String("application/json"),
		Accept:      aws.String("application/json"),
		Body:        bodyBytes,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to invoke bedrock model: %w", err)
	}

	// Parse the response
	var response struct {
		Embedding []float32 `json:"embedding"`
	}

	if err := json.Unmarshal(resp.Body, &response); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	// Cache the result
	s.cache.Store(text, response.Embedding)

	return response.Embedding, nil
}

// GenerateActorEmbedding creates a combined embedding for an actor profile
func (s *EmbeddingService) GenerateActorEmbedding(ctx context.Context, actor *activitypub.Actor) ([]float32, error) {
	// Combine relevant text fields for embedding
	textParts := []string{}

	if actor.PreferredUsername != "" {
		textParts = append(textParts, "username: "+actor.PreferredUsername)
	}

	if actor.Name != "" {
		textParts = append(textParts, "name: "+actor.Name)
	}

	if actor.Summary != "" {
		// Clean HTML from bio
		bio := strings.ReplaceAll(actor.Summary, "<p>", " ")
		bio = strings.ReplaceAll(bio, "</p>", " ")
		bio = strings.ReplaceAll(bio, "<br>", " ")
		bio = strings.ReplaceAll(bio, "<br/>", " ")
		textParts = append(textParts, "bio: "+bio)
	}

	combinedText := strings.Join(textParts, ". ")
	return s.GenerateEmbedding(ctx, combinedText)
}

// GenerateStatusEmbedding creates an embedding for a status/post
func (s *EmbeddingService) GenerateStatusEmbedding(ctx context.Context, content string, authorUsername string) ([]float32, error) {
	// Clean HTML from content
	cleanContent := strings.ReplaceAll(content, "<p>", " ")
	cleanContent = strings.ReplaceAll(cleanContent, "</p>", " ")
	cleanContent = strings.ReplaceAll(cleanContent, "<br>", " ")
	cleanContent = strings.ReplaceAll(cleanContent, "<br/>", " ")
	cleanContent = strings.ReplaceAll(cleanContent, "<a", " ")
	cleanContent = strings.ReplaceAll(cleanContent, "</a>", " ")

	// Remove any HTML tags
	// Simple regex-like approach
	for strings.Contains(cleanContent, "<") && strings.Contains(cleanContent, ">") {
		start := strings.Index(cleanContent, "<")
		end := strings.Index(cleanContent, ">")
		if start < end && start >= 0 && end >= 0 {
			cleanContent = cleanContent[:start] + " " + cleanContent[end+1:]
		} else {
			break
		}
	}

	// Combine content with author context for better embeddings
	textParts := []string{}
	if authorUsername != "" {
		textParts = append(textParts, "author: @"+authorUsername)
	}
	textParts = append(textParts, "content: "+cleanContent)

	combinedText := strings.Join(textParts, ". ")

	// Limit to reasonable length for embedding model (roughly 500 words)
	words := strings.Fields(combinedText)
	if len(words) > 500 {
		combinedText = strings.Join(words[:500], " ")
	}

	return s.GenerateEmbedding(ctx, combinedText)
}

// StoreActorEmbedding stores an actor's embedding in DynamoDB
func (s *EmbeddingService) StoreActorEmbedding(ctx context.Context, actorID string, embedding []float32) error {
	// Convert float32 slice to JSON for storage
	embeddingJSON, err := json.Marshal(embedding)
	if err != nil {
		return fmt.Errorf("failed to marshal embedding: %w", err)
	}

	item := map[string]types.AttributeValue{
		"PK":        &types.AttributeValueMemberS{Value: fmt.Sprintf("EMBEDDING#%s", actorID)},
		"SK":        &types.AttributeValueMemberS{Value: "VECTOR"},
		"Embedding": &types.AttributeValueMemberB{Value: embeddingJSON},
		"UpdatedAt": &types.AttributeValueMemberS{Value: time.Now().Format(time.RFC3339)},
		"ModelID":   &types.AttributeValueMemberS{Value: s.modelID},
		"Dimension": &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", s.embeddingDim)},
	}

	_, err = s.dynamo.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(s.tableName),
		Item:      item,
	})
	if err != nil {
		return fmt.Errorf("failed to store embedding: %w", err)
	}

	return nil
}

// StoreStatusEmbedding stores a status embedding in DynamoDB
func (s *EmbeddingService) StoreStatusEmbedding(ctx context.Context, statusID string, embedding []float32) error {
	// Convert float32 slice to JSON for storage
	embeddingJSON, err := json.Marshal(embedding)
	if err != nil {
		return fmt.Errorf("failed to marshal embedding: %w", err)
	}

	item := map[string]types.AttributeValue{
		"PK":        &types.AttributeValueMemberS{Value: fmt.Sprintf("STATUS_EMBEDDING#%s", statusID)},
		"SK":        &types.AttributeValueMemberS{Value: "VECTOR"},
		"Embedding": &types.AttributeValueMemberB{Value: embeddingJSON},
		"UpdatedAt": &types.AttributeValueMemberS{Value: time.Now().Format(time.RFC3339)},
		"ModelID":   &types.AttributeValueMemberS{Value: s.modelID},
		"Dimension": &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", s.embeddingDim)},
		"TTL":       &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", time.Now().Add(90*24*time.Hour).Unix())}, // 90 days TTL
	}

	_, err = s.dynamo.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(s.tableName),
		Item:      item,
	})
	if err != nil {
		return fmt.Errorf("failed to store status embedding: %w", err)
	}

	return nil
}

// GetActorEmbedding retrieves an actor's embedding from DynamoDB
func (s *EmbeddingService) GetActorEmbedding(ctx context.Context, actorID string) ([]float32, error) {
	result, err := s.dynamo.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("EMBEDDING#%s", actorID)},
			"SK": &types.AttributeValueMemberS{Value: "VECTOR"},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get embedding: %w", err)
	}

	if result.Item == nil {
		return nil, nil
	}

	// Extract and unmarshal the embedding
	if embeddingAttr, ok := result.Item["Embedding"].(*types.AttributeValueMemberB); ok {
		var embedding []float32
		if err := json.Unmarshal(embeddingAttr.Value, &embedding); err != nil {
			return nil, fmt.Errorf("failed to unmarshal embedding: %w", err)
		}
		return embedding, nil
	}

	return nil, fmt.Errorf("embedding not found in item")
}

// GetStatusEmbedding retrieves a status embedding from DynamoDB
func (s *EmbeddingService) GetStatusEmbedding(ctx context.Context, statusID string) ([]float32, error) {
	result, err := s.dynamo.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("STATUS_EMBEDDING#%s", statusID)},
			"SK": &types.AttributeValueMemberS{Value: "VECTOR"},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get status embedding: %w", err)
	}

	if result.Item == nil {
		return nil, nil
	}

	// Extract and unmarshal the embedding
	if embeddingAttr, ok := result.Item["Embedding"].(*types.AttributeValueMemberB); ok {
		var embedding []float32
		if err := json.Unmarshal(embeddingAttr.Value, &embedding); err != nil {
			return nil, fmt.Errorf("failed to unmarshal embedding: %w", err)
		}
		return embedding, nil
	}

	return nil, fmt.Errorf("embedding not found in item")
}

// CosineSimilarity calculates the cosine similarity between two vectors
func CosineSimilarity(a, b []float32) float32 {
	if len(a) != len(b) {
		return 0
	}

	var dotProduct, normA, normB float32
	for i := range a {
		dotProduct += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}

	if normA == 0 || normB == 0 {
		return 0
	}

	return dotProduct / (float32(math.Sqrt(float64(normA))) * float32(math.Sqrt(float64(normB))))
}

// UpdateAllActorEmbeddings generates and stores embeddings for all actors
func (s *EmbeddingService) UpdateAllActorEmbeddings(ctx context.Context) error {
	s.logger.Info("starting bulk actor embedding update")

	// Scan for all actors
	scanInput := &dynamodb.ScanInput{
		TableName:        aws.String(s.tableName),
		FilterExpression: aws.String("begins_with(PK, :pk) AND SK = :sk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: "ACTOR#"},
			":sk": &types.AttributeValueMemberS{Value: "PROFILE"},
		},
	}

	paginator := dynamodb.NewScanPaginator(s.dynamo, scanInput)
	updatedCount := 0

	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return fmt.Errorf("failed to scan actors: %w", err)
		}

		for _, item := range page.Items {
			var record ActorRecord
			if err := attributevalue.UnmarshalMap(item, &record); err != nil {
				s.logger.Warn("failed to unmarshal actor record", zap.Error(err))
				continue
			}

			if record.Actor == nil {
				continue
			}

			// Generate embedding
			embedding, err := s.GenerateActorEmbedding(ctx, record.Actor)
			if err != nil {
				s.logger.Warn("failed to generate embedding",
					zap.String("actor", record.Actor.ID),
					zap.Error(err))
				continue
			}

			// Store embedding
			if err := s.StoreActorEmbedding(ctx, record.Actor.ID, embedding); err != nil {
				s.logger.Warn("failed to store embedding",
					zap.String("actor", record.Actor.ID),
					zap.Error(err))
				continue
			}

			updatedCount++
			if updatedCount%10 == 0 {
				s.logger.Info("embedding update progress",
					zap.Int("updated", updatedCount))
			}
		}
	}

	s.logger.Info("completed bulk actor embedding update",
		zap.Int("total_updated", updatedCount))

	return nil
}

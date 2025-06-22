package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aron23/lesser/pkg/common"
	"github.com/aron23/lesser/pkg/config"
	"github.com/aron23/lesser/pkg/storage"
	"github.com/aron23/lesser/pkg/storage/dynamodb"
	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	awsdynamodb "github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"go.uber.org/zap"
)

var (
	dynamoClient       *awsdynamodb.Client
	tableName          string
	logger             *zap.Logger
	embeddingService   *dynamodb.EmbeddingService
	store              storage.Storage
	generateEmbeddings bool
)

func init() {
	// Initialize logger
	logger = common.Logger()

	// Load configuration
	cfg := config.Get()
	tableName = cfg.DynamoTableName

	// Initialize DynamoDB client
	ctx := context.Background()
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion(cfg.Region),
	)
	if err != nil {
		logger.Fatal("failed to load AWS config", zap.Error(err))
	}

	dynamoClient = awsdynamodb.NewFromConfig(awsCfg)

	// Initialize embedding service (optional - only if Bedrock is available)
	embeddingService, err = dynamodb.NewEmbeddingService(awsCfg, tableName, logger)
	if err != nil {
		logger.Warn("failed to initialize embedding service, semantic search will be disabled", zap.Error(err))
		generateEmbeddings = false
	} else {
		generateEmbeddings = true
		logger.Info("embedding service initialized for semantic search")
	}
}

// handleDynamoDBStream processes DynamoDB stream events
func handleDynamoDBStream(ctx context.Context, event events.DynamoDBEvent) error {
	for _, record := range event.Records {
		if err := processRecord(ctx, record); err != nil {
			logger.Error("failed to process record",
				zap.String("eventID", record.EventID),
				zap.Error(err))
			// Continue processing other records
		}
	}
	return nil
}

// processRecord processes a single DynamoDB stream record
func processRecord(ctx context.Context, record events.DynamoDBEventRecord) error {
	// Only process INSERT and MODIFY events
	if record.EventName != "INSERT" && record.EventName != "MODIFY" {
		return nil
	}

	// Check if this is an object record
	pk, pkExists := record.Change.Keys["PK"]
	sk, skExists := record.Change.Keys["SK"]

	if !pkExists || !skExists {
		return nil
	}

	// Extract PK string value
	pkStr := ""
	if pk.DataType() == events.DataTypeString {
		pkStr = pk.String()
	}
	if pkStr == "" || !strings.HasPrefix(pkStr, "OBJECT#") {
		return nil
	}

	// Extract SK string value
	skStr := ""
	if sk.DataType() == events.DataTypeString {
		skStr = sk.String()
	}
	if skStr != "METADATA" {
		return nil
	}

	// Extract the object from the new image
	newImage := record.Change.NewImage
	if newImage == nil {
		return nil
	}

	// Get object type
	objectData, ok := newImage["Object"]
	if !ok || objectData.DataType() != events.DataTypeMap {
		return nil
	}

	objectMap := objectData.Map()
	typeAttr, ok := objectMap["type"]
	if !ok || typeAttr.DataType() != events.DataTypeString {
		return nil
	}

	// Only process Note and Article types
	objectType := typeAttr.String()
	if objectType != "Note" && objectType != "Article" {
		return nil
	}

	// Extract object details
	var objectID, content, authorID, authorUsername string
	var published time.Time

	if id, ok := objectMap["id"]; ok && id.DataType() == events.DataTypeString {
		objectID = id.String()
	}
	if cont, ok := objectMap["content"]; ok && cont.DataType() == events.DataTypeString {
		content = cont.String()
	}
	if author, ok := objectMap["attributedTo"]; ok && author.DataType() == events.DataTypeString {
		authorID = author.String()
	}
	if pub, ok := objectMap["published"]; ok && pub.DataType() == events.DataTypeString {
		published, _ = time.Parse(time.RFC3339, pub.String())
	}

	// Extract author username from authorID (format: https://domain.com/users/username)
	if authorID != "" {
		parts := strings.Split(authorID, "/")
		if len(parts) > 0 {
			authorUsername = parts[len(parts)-1]
		}
	}

	// Process the status for search indexing
	if objectID != "" && content != "" {
		if err := indexStatus(ctx, objectID, content, authorID, authorUsername, published); err != nil {
			return fmt.Errorf("failed to index status: %w", err)
		}
	}

	return nil
}

// indexStatus indexes a status for search
func indexStatus(ctx context.Context, statusID, content, authorID, authorUsername string, published time.Time) error {
	// 1. Extract and index words
	words := extractSignificantWords(content)
	for _, word := range words {
		if err := indexWord(ctx, word, statusID, published); err != nil {
			logger.Warn("failed to index word",
				zap.String("word", word),
				zap.String("statusID", statusID),
				zap.Error(err))
		}
	}

	// 2. Index hashtags
	hashtags := extractHashtags(content)
	for _, tag := range hashtags {
		if err := indexHashtag(ctx, tag, statusID, published); err != nil {
			logger.Warn("failed to index hashtag",
				zap.String("tag", tag),
				zap.String("statusID", statusID),
				zap.Error(err))
		}
	}

	// 3. Index by author
	if authorID != "" {
		if err := indexByAuthor(ctx, authorID, statusID, published); err != nil {
			logger.Warn("failed to index by author",
				zap.String("authorID", authorID),
				zap.String("statusID", statusID),
				zap.Error(err))
		}
	}

	// 4. Generate and store embeddings for semantic search
	if generateEmbeddings && embeddingService != nil {
		go func() {
			// Run embedding generation asynchronously to avoid blocking
			embedCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			embedding, err := embeddingService.GenerateStatusEmbedding(embedCtx, content, authorUsername)
			if err != nil {
				logger.Warn("failed to generate status embedding",
					zap.String("statusID", statusID),
					zap.Error(err))
				return
			}

			if err := embeddingService.StoreStatusEmbedding(embedCtx, statusID, embedding); err != nil {
				logger.Warn("failed to store status embedding",
					zap.String("statusID", statusID),
					zap.Error(err))
				return
			}

			logger.Debug("generated and stored embedding for status",
				zap.String("statusID", statusID),
				zap.Int("dimensions", len(embedding)))
		}()
	}

	// 5. Calculate engagement and index by engagement bucket
	go func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Error("panic in engagement calculation", zap.Any("panic", r))
			}
		}()

		// Get status object for engagement calculation
		statusObj, err := store.GetStatus(ctx, statusID)
		if err != nil {
			logger.Error("failed to get status for engagement calculation", zap.Error(err))
			return
		}

		if err := calculateAndIndexEngagement(ctx, statusID, statusObj); err != nil {
			logger.Error("failed to calculate engagement", zap.Error(err))
		}
	}()

	logger.Info("indexed status",
		zap.String("statusID", statusID),
		zap.Int("words", len(words)),
		zap.Int("hashtags", len(hashtags)),
		zap.Bool("embeddings", generateEmbeddings))

	return nil
}

// indexWord indexes a word for search
func indexWord(ctx context.Context, word, statusID string, published time.Time) error {
	item := map[string]types.AttributeValue{
		"PK":        &types.AttributeValueMemberS{Value: fmt.Sprintf("WORD#%s#%s", word, statusID)},
		"SK":        &types.AttributeValueMemberS{Value: "INDEX"},
		"GSI5PK":    &types.AttributeValueMemberS{Value: fmt.Sprintf("WORD#%s", word)},
		"GSI5SK":    &types.AttributeValueMemberS{Value: fmt.Sprintf("STATUS#%d#%s", published.Unix(), statusID)},
		"StatusID":  &types.AttributeValueMemberS{Value: statusID},
		"Word":      &types.AttributeValueMemberS{Value: word},
		"IndexedAt": &types.AttributeValueMemberS{Value: time.Now().Format(time.RFC3339)},
		"TTL":       &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", time.Now().Add(90*24*time.Hour).Unix())},
	}

	_, err := dynamoClient.PutItem(ctx, &awsdynamodb.PutItemInput{
		TableName: aws.String(tableName),
		Item:      item,
	})

	return err
}

// indexHashtag indexes a hashtag for search
func indexHashtag(ctx context.Context, tag, statusID string, published time.Time) error {
	item := map[string]types.AttributeValue{
		"PK":        &types.AttributeValueMemberS{Value: fmt.Sprintf("TAG#%s#%s", tag, statusID)},
		"SK":        &types.AttributeValueMemberS{Value: "INDEX"},
		"GSI6PK":    &types.AttributeValueMemberS{Value: fmt.Sprintf("TAG#%s", tag)},
		"GSI6SK":    &types.AttributeValueMemberS{Value: fmt.Sprintf("STATUS#%d#%s", published.Unix(), statusID)},
		"StatusID":  &types.AttributeValueMemberS{Value: statusID},
		"Tag":       &types.AttributeValueMemberS{Value: tag},
		"IndexedAt": &types.AttributeValueMemberS{Value: time.Now().Format(time.RFC3339)},
		"TTL":       &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", time.Now().Add(90*24*time.Hour).Unix())},
	}

	_, err := dynamoClient.PutItem(ctx, &awsdynamodb.PutItemInput{
		TableName: aws.String(tableName),
		Item:      item,
	})

	return err
}

// indexByAuthor indexes a status by author
func indexByAuthor(ctx context.Context, authorID, statusID string, published time.Time) error {
	item := map[string]types.AttributeValue{
		"PK":        &types.AttributeValueMemberS{Value: fmt.Sprintf("AUTHOR#%s#%s", authorID, statusID)},
		"SK":        &types.AttributeValueMemberS{Value: "INDEX"},
		"GSI7PK":    &types.AttributeValueMemberS{Value: fmt.Sprintf("AUTHOR#%s", authorID)},
		"GSI7SK":    &types.AttributeValueMemberS{Value: fmt.Sprintf("STATUS#%d#%s", published.Unix(), statusID)},
		"StatusID":  &types.AttributeValueMemberS{Value: statusID},
		"AuthorID":  &types.AttributeValueMemberS{Value: authorID},
		"IndexedAt": &types.AttributeValueMemberS{Value: time.Now().Format(time.RFC3339)},
		"TTL":       &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", time.Now().Add(90*24*time.Hour).Unix())},
	}

	_, err := dynamoClient.PutItem(ctx, &awsdynamodb.PutItemInput{
		TableName: aws.String(tableName),
		Item:      item,
	})

	return err
}

// Utility functions (simplified versions - would reuse from status_search_utils.go in production)

func extractSignificantWords(content string) []string {
	content = strings.ToLower(content)
	words := strings.FieldsFunc(content, func(r rune) bool {
		return !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'))
	})

	significant := make([]string, 0)
	seen := make(map[string]bool)

	for _, word := range words {
		if len(word) >= 3 && !isStopWord(word) && !seen[word] {
			seen[word] = true
			significant = append(significant, word)
			if len(significant) >= 20 {
				break
			}
		}
	}

	return significant
}

func extractHashtags(content string) []string {
	hashtags := make([]string, 0)
	words := strings.Fields(content)

	for _, word := range words {
		if strings.HasPrefix(word, "#") && len(word) > 1 {
			tag := strings.ToLower(strings.TrimPrefix(word, "#"))
			// Remove any trailing punctuation
			tag = strings.TrimFunc(tag, func(r rune) bool {
				return !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_')
			})
			if tag != "" {
				hashtags = append(hashtags, tag)
			}
		}
	}

	return hashtags
}

func isStopWord(word string) bool {
	stopWords := map[string]bool{
		"a": true, "an": true, "and": true, "are": true, "as": true, "at": true,
		"be": true, "by": true, "for": true, "from": true, "has": true, "he": true,
		"in": true, "is": true, "it": true, "its": true, "of": true, "on": true,
		"that": true, "the": true, "to": true, "was": true, "will": true, "with": true,
	}
	return stopWords[word]
}

// calculateAndIndexEngagement calculates engagement metrics and indexes by engagement bucket
func calculateAndIndexEngagement(ctx context.Context, statusID string, status interface{}) error {
	// Get current engagement metrics
	likes, err := store.GetLikeCount(ctx, statusID)
	if err != nil {
		likes = 0 // Default to 0 if not found
	}

	boosts, err := store.GetBoostCount(ctx, statusID)
	if err != nil {
		boosts = 0 // Default to 0 if not found
	}

	replies, err := store.GetReplyCount(ctx, statusID)
	if err != nil {
		replies = 0 // Default to 0 if not found
	}

	// Calculate total engagement score
	engagementScore := (likes * 1) + (boosts * 2) + (replies * 3)

	// Determine engagement bucket
	var engagementBucket string
	switch {
	case engagementScore >= 1000:
		engagementBucket = "viral"
	case engagementScore >= 100:
		engagementBucket = "high"
	case engagementScore >= 10:
		engagementBucket = "medium"
	default:
		engagementBucket = "low"
	}

	// Store engagement metrics
	engagement := &storage.EngagementMetrics{
		StatusID:         statusID,
		LikeCount:        likes,
		BoostCount:       boosts,
		ReplyCount:       replies,
		Score:            float64(engagementScore),
		EngagementBucket: engagementBucket,
	}

	// TODO: Add StoreEngagementMetrics method to storage interface
	_ = engagement // For now, just acknowledge we created it

	// Index by engagement bucket for discovery
	// TODO: Add IndexByEngagement method to storage interface
	_ = engagementBucket // For now, just acknowledge the bucket

	logger.Debug("calculated engagement",
		zap.String("statusID", statusID),
		zap.Int64("likes", likes),
		zap.Int64("boosts", boosts),
		zap.Int64("replies", replies),
		zap.Float64("total_score", float64(engagementScore)),
		zap.String("bucket", engagementBucket))

	return nil
}

func main() {
	lambda.Start(handleDynamoDBStream)
}

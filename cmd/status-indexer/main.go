package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/google/uuid"
	"github.com/pay-theory/dynamorm/pkg/core"
	"go.uber.org/zap"

	"github.com/equaltoai/lesser/pkg/ai"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/storage/dynamorm"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
)

// StatusIndexer handles DynamoDB stream events for search indexing
type StatusIndexer struct {
	db           core.DB
	tableName    string
	logger       *zap.Logger
	aiService    *ai.AIService
	likeRepo     *repositories.LikeRepository
	hashtagsRepo *repositories.HashtagRepository
	objectRepo   *repositories.ObjectRepository
}

// NewStatusIndexer creates a new status indexer
func NewStatusIndexer(db core.DB, tableName, domain string, aiService *ai.AIService) *StatusIndexer {
	// Get logger
	logger := common.Logger()

	return &StatusIndexer{
		db:           db,
		tableName:    tableName,
		logger:       logger,
		aiService:    aiService,
		likeRepo:     repositories.NewLikeRepository(db, tableName, logger),
		hashtagsRepo: repositories.NewHashtagRepository(db, tableName, logger, domain),
		objectRepo:   repositories.NewObjectRepository(db, tableName, domain, logger),
	}
}

var (
	logger    *zap.Logger
	cfg       *config.Config
	processor *StatusIndexer
	db        core.DB
	aiService *ai.AIService
)

func init() {
	// Initialize logger
	logger = common.Logger()

	// Load configuration
	cfg = config.Get()

	// Initialize DynamORM with Lambda optimizations
	var err error
	db, err = dynamorm.NewLambdaOptimizedClient(context.Background(), cfg.Region)
	if err != nil {
		logger.Fatal("Failed to initialize DynamORM", zap.Error(err))
	}

	// Initialize AWS config for AI service
	awsCfg, err := awsconfig.LoadDefaultConfig(context.Background(),
		awsconfig.WithRegion(cfg.Region),
	)
	if err != nil {
		logger.Fatal("Failed to load AWS config", zap.Error(err))
	}

	// Initialize AI service
	aiConfig := &ai.AIConfig{
		NSFWThreshold:      0.8,
		ToxicityThreshold:  0.7,
		SpamThreshold:      0.6,
		AIContentThreshold: 0.8,
		EnablePIIDetection:  true,
		EnableAIDetection:   true,
		EnableImageAnalysis: true,
		BedrockModelID:      "anthropic.claude-3-haiku-20240307-v1:0",
		S3Bucket:           cfg.S3BucketName,
	}
	aiService = ai.NewAIService(awsCfg, aiConfig)

	// Initialize processor
	processor = NewStatusIndexer(db, cfg.DynamoTableName, cfg.Domain, aiService)
}

// HandleStream processes DynamoDB stream events with Lift-style patterns
func (si *StatusIndexer) HandleStream(ctx context.Context, event events.DynamoDBEvent) error {
	// Generate request ID for tracking (Lift pattern)
	requestID := uuid.New().String()

	// Add request ID to context for downstream use
	ctx = context.WithValue(ctx, "request_id", requestID)

	si.logger.Info("processing status indexer stream batch",
		zap.String("request_id", requestID),
		zap.Int("record_count", len(event.Records)),
	)

	// Process records with error collection
	var errors []error
	for _, record := range event.Records {
		if err := si.processRecord(ctx, record); err != nil {
			si.logger.Error("failed to process record",
				zap.String("request_id", requestID),
				zap.String("event_id", record.EventID),
				zap.Error(err))
			errors = append(errors, err)
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("partial batch failure: %d of %d records failed", len(errors), len(event.Records))
	}

	return nil
}

// processRecord processes a single DynamoDB stream record
func (si *StatusIndexer) processRecord(ctx context.Context, record events.DynamoDBEventRecord) error {
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

	// Process the status for search indexing with engagement and embeddings
	if objectID != "" && content != "" {
		if err := si.processStatusEvent(ctx, objectID, content, authorID, authorUsername, published); err != nil {
			return fmt.Errorf("failed to process status event: %w", err)
		}
	}

	return nil
}

// processStatusEvent processes a status event with full engagement calculation and embeddings
func (si *StatusIndexer) processStatusEvent(ctx context.Context, statusID, content, authorID, authorUsername string, published time.Time) error {
	requestID := getRequestID(ctx)
	
	// 1. Calculate engagement metrics
	engagementScore, likes, boosts, replies, err := si.calculateEngagement(ctx, statusID)
	if err != nil {
		si.logger.Warn("failed to calculate engagement",
			zap.String("request_id", requestID),
			zap.String("status_id", statusID),
			zap.Error(err))
		// Continue processing even if engagement calculation fails
		engagementScore = 0
	}

	// 2. Generate embeddings for semantic search
	var embedding []float32
	if si.aiService != nil {
		embedding, err = si.aiService.GenerateEmbedding(ctx, content)
		if err != nil {
			si.logger.Warn("failed to generate embedding",
				zap.String("request_id", requestID),
				zap.String("status_id", statusID),
				zap.Error(err))
			// Continue without embeddings
		}
	}

	// 3. Store status embedding if generated
	if len(embedding) > 0 {
		if err := si.storeEmbedding(ctx, statusID, embedding, engagementScore); err != nil {
			si.logger.Warn("failed to store embedding",
				zap.String("request_id", requestID),
				zap.String("status_id", statusID),
				zap.Error(err))
		}
	}

	// 4. Update trending status if engagement is significant
	if engagementScore > 10 { // Threshold for trending consideration
		if err := si.updateTrendingStatus(ctx, statusID, content, authorID, published, engagementScore, likes, boosts, replies); err != nil {
			si.logger.Warn("failed to update trending status",
				zap.String("request_id", requestID),
				zap.String("status_id", statusID),
				zap.Error(err))
		}
	}

	// 5. Extract and index words
	words := si.extractSignificantWords(content)
	for _, word := range words {
		if err := si.indexWord(ctx, word, statusID, published); err != nil {
			si.logger.Warn("failed to index word",
				zap.String("request_id", requestID),
				zap.String("word", word),
				zap.String("status_id", statusID),
				zap.Error(err))
		}
	}

	// 6. Index hashtags and track trending
	hashtags := si.extractHashtags(content)
	for _, tag := range hashtags {
		if err := si.indexHashtagWithTrending(ctx, tag, statusID, published, engagementScore); err != nil {
			si.logger.Warn("failed to index hashtag",
				zap.String("request_id", requestID),
				zap.String("tag", tag),
				zap.String("status_id", statusID),
				zap.Error(err))
		}
	}

	// 7. Index by author
	if authorID != "" {
		if err := si.indexByAuthor(ctx, authorID, statusID, published); err != nil {
			si.logger.Warn("failed to index by author",
				zap.String("request_id", requestID),
				zap.String("author_id", authorID),
				zap.String("status_id", statusID),
				zap.Error(err))
		}
	}

	si.logger.Info("processed status event",
		zap.String("request_id", requestID),
		zap.String("status_id", statusID),
		zap.Float64("engagement_score", engagementScore),
		zap.Int("likes", likes),
		zap.Int("boosts", boosts),
		zap.Int("replies", replies),
		zap.Int("words", len(words)),
		zap.Int("hashtags", len(hashtags)),
		zap.Bool("has_embedding", len(embedding) > 0))

	return nil
}

// indexWord indexes a word for search using DynamORM
func (si *StatusIndexer) indexWord(ctx context.Context, word, statusID string, published time.Time) error {
	wordIndex := struct {
		PK        string `dynamorm:"pk"`
		SK        string `dynamorm:"sk"`
		GSI5PK    string `dynamorm:"index:gsi5,pk"`
		GSI5SK    string `dynamorm:"index:gsi5,sk"`
		StatusID  string `json:"status_id"`
		Word      string `json:"word"`
		IndexedAt string `json:"indexed_at"`
		TTL       int64  `dynamorm:"ttl"`
	}{
		PK:        fmt.Sprintf("WORD#%s#%s", word, statusID),
		SK:        "INDEX",
		GSI5PK:    fmt.Sprintf("WORD#%s", word),
		GSI5SK:    fmt.Sprintf("STATUS#%d#%s", published.Unix(), statusID),
		StatusID:  statusID,
		Word:      word,
		IndexedAt: time.Now().Format(time.RFC3339),
		TTL:       time.Now().Add(90 * 24 * time.Hour).Unix(),
	}

	return si.db.WithContext(ctx).Model(&wordIndex).Create()
}

// indexHashtag indexes a hashtag for search using DynamORM
func (si *StatusIndexer) indexHashtag(ctx context.Context, tag, statusID string, published time.Time) error {
	tagIndex := struct {
		PK        string `dynamorm:"pk"`
		SK        string `dynamorm:"sk"`
		GSI6PK    string `dynamorm:"index:gsi6,pk"`
		GSI6SK    string `dynamorm:"index:gsi6,sk"`
		StatusID  string `json:"status_id"`
		Tag       string `json:"tag"`
		IndexedAt string `json:"indexed_at"`
		TTL       int64  `dynamorm:"ttl"`
	}{
		PK:        fmt.Sprintf("TAG#%s#%s", tag, statusID),
		SK:        "INDEX",
		GSI6PK:    fmt.Sprintf("TAG#%s", tag),
		GSI6SK:    fmt.Sprintf("STATUS#%d#%s", published.Unix(), statusID),
		StatusID:  statusID,
		Tag:       tag,
		IndexedAt: time.Now().Format(time.RFC3339),
		TTL:       time.Now().Add(90 * 24 * time.Hour).Unix(),
	}

	return si.db.WithContext(ctx).Model(&tagIndex).Create()
}

// indexByAuthor indexes a status by author using DynamORM
func (si *StatusIndexer) indexByAuthor(ctx context.Context, authorID, statusID string, published time.Time) error {
	authorIndex := struct {
		PK        string `dynamorm:"pk"`
		SK        string `dynamorm:"sk"`
		GSI7PK    string `dynamorm:"index:gsi7,pk"`
		GSI7SK    string `dynamorm:"index:gsi7,sk"`
		StatusID  string `json:"status_id"`
		AuthorID  string `json:"author_id"`
		IndexedAt string `json:"indexed_at"`
		TTL       int64  `dynamorm:"ttl"`
	}{
		PK:        fmt.Sprintf("AUTHOR#%s#%s", authorID, statusID),
		SK:        "INDEX",
		GSI7PK:    fmt.Sprintf("AUTHOR#%s", authorID),
		GSI7SK:    fmt.Sprintf("STATUS#%d#%s", published.Unix(), statusID),
		StatusID:  statusID,
		AuthorID:  authorID,
		IndexedAt: time.Now().Format(time.RFC3339),
		TTL:       time.Now().Add(90 * 24 * time.Hour).Unix(),
	}

	return si.db.WithContext(ctx).Model(&authorIndex).Create()
}

// Utility functions for text processing

func (si *StatusIndexer) extractSignificantWords(content string) []string {
	content = strings.ToLower(content)
	words := strings.FieldsFunc(content, func(r rune) bool {
		return !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'))
	})

	significant := make([]string, 0)
	seen := make(map[string]bool)

	for _, word := range words {
		if len(word) >= 3 && !si.isStopWord(word) && !seen[word] {
			seen[word] = true
			significant = append(significant, word)
			if len(significant) >= 20 {
				break
			}
		}
	}

	return significant
}

func (si *StatusIndexer) extractHashtags(content string) []string {
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

func (si *StatusIndexer) isStopWord(word string) bool {
	stopWords := map[string]bool{
		"a": true, "an": true, "and": true, "are": true, "as": true, "at": true,
		"be": true, "by": true, "for": true, "from": true, "has": true, "he": true,
		"in": true, "is": true, "it": true, "its": true, "of": true, "on": true,
		"that": true, "the": true, "to": true, "was": true, "will": true, "with": true,
	}
	return stopWords[word]
}

// calculateEngagement calculates the engagement score for a status
func (si *StatusIndexer) calculateEngagement(ctx context.Context, statusID string) (float64, int, int, int, error) {
	// Get like count
	likes, err := si.likeRepo.GetLikeCount(ctx, statusID)
	if err != nil {
		si.logger.Debug("failed to get like count", zap.String("status_id", statusID), zap.Error(err))
		likes = 0
	}

	// Get boost count  
	boosts, err := si.likeRepo.GetBoostCount(ctx, statusID)
	if err != nil {
		si.logger.Debug("failed to get boost count", zap.String("status_id", statusID), zap.Error(err))
		boosts = 0
	}

	// Get reply count - query objects with InReplyTo pointing to this status
	replies, err := si.getReplyCount(ctx, statusID)
	if err != nil {
		si.logger.Debug("failed to get reply count", zap.String("status_id", statusID), zap.Error(err))
		replies = 0
	}

	// Calculate engagement score using weighted formula
	// Likes: 1 point, Boosts: 2 points, Replies: 1.5 points
	engagementScore := float64(likes)*1.0 + float64(boosts)*2.0 + float64(replies)*1.5

	return engagementScore, int(likes), int(boosts), replies, nil
}

// getReplyCount counts replies to a status by querying objects
func (si *StatusIndexer) getReplyCount(ctx context.Context, statusID string) (int, error) {
	// Query objects where InReplyTo matches this status
	// This uses GSI to find replies efficiently
	var objects []models.Object
	err := si.db.WithContext(ctx).Model(&models.Object{}).
		Index("gsi3-index"). // Assuming GSI3 is for InReplyTo queries
		Where("GSI3PK", "=", fmt.Sprintf("IN_REPLY_TO#%s", statusID)).
		Limit(1000). // Reasonable limit to avoid timeout
		All(&objects)
	
	if err != nil {
		return 0, fmt.Errorf("failed to count replies: %w", err)
	}

	return len(objects), nil
}

// storeEmbedding stores the generated embedding for semantic search
func (si *StatusIndexer) storeEmbedding(ctx context.Context, statusID string, embedding []float32, score float64) error {
	embeddingModel := &models.SearchEmbedding{
		ContentID:   statusID,
		ContentType: "status",
		Embedding:   embedding,
		Score:       score,
		CreatedAt:   time.Now(),
		Metadata: map[string]string{
			"type":            "status",
			"indexed_at":      time.Now().Format(time.RFC3339),
			"embedding_model": "amazon.titan-embed-text-v1",
		},
	}
	embeddingModel.UpdateKeys()

	return si.db.WithContext(ctx).Model(embeddingModel).Create()
}

// updateTrendingStatus updates or creates a trending status record
func (si *StatusIndexer) updateTrendingStatus(ctx context.Context, statusID, content, authorID string, published time.Time, engagementScore float64, likes, boosts, replies int) error {
	date := time.Now().Format("2006-01-02")
	
	trendingStatus := &models.TrendingStatus{
		ID:            statusID,
		URL:           fmt.Sprintf("https://%s/statuses/%s", si.getDomain(), statusID),
		AuthorID:      authorID,
		Content:       content,
		Engagements:   int64(likes + boosts + replies),
		PublishedAt:   published,
		CreatedAt:     time.Now(),
		Likes:         likes,
		Boosts:        boosts,
		Replies:       replies,
		Date:          date,
		TrendingScore: engagementScore,
	}
	trendingStatus.UpdateKeys()

	// Use Create which will update if exists due to the key structure
	return si.db.WithContext(ctx).Model(trendingStatus).Create()
}

// indexHashtagWithTrending indexes a hashtag and tracks its trending status
func (si *StatusIndexer) indexHashtagWithTrending(ctx context.Context, tag, statusID string, published time.Time, engagementScore float64) error {
	// First, index the hashtag normally
	if err := si.indexHashtag(ctx, tag, statusID, published); err != nil {
		return err
	}

	// Then, update trending hashtag metrics if engagement is significant
	if engagementScore > 5 { // Lower threshold for hashtag trending
		if err := si.updateTrendingHashtag(ctx, tag, engagementScore); err != nil {
			si.logger.Warn("failed to update trending hashtag",
				zap.String("tag", tag),
				zap.Error(err))
			// Non-critical error, continue
		}
	}

	return nil
}

// updateTrendingHashtag updates trending hashtag metrics
func (si *StatusIndexer) updateTrendingHashtag(ctx context.Context, tag string, engagementScore float64) error {
	// Create or update a trending hashtag record
	// This uses daily buckets for trending calculation
	date := time.Now().Format("2006-01-02")
	hour := time.Now().Format("2006-01-02-15") // Hour-level granularity

	trendingHashtag := struct {
		PK             string    `dynamorm:"pk"`
		SK             string    `dynamorm:"sk"`
		GSI6PK         string    `dynamorm:"index:gsi6,pk"`
		GSI6SK         string    `dynamorm:"index:gsi6,sk"`
		Tag            string    `json:"tag"`
		Date           string    `json:"date"`
		Hour           string    `json:"hour"`
		EngagementSum  float64   `json:"engagement_sum"`
		PostCount      int       `json:"post_count"`
		LastUpdated    time.Time `json:"last_updated"`
		TTL            int64     `dynamorm:"ttl"`
	}{
		PK:             fmt.Sprintf("TRENDING_TAG#%s#%s", tag, hour),
		SK:             "METRICS",
		GSI6PK:         fmt.Sprintf("TRENDING_TAGS#%s", date),
		GSI6SK:         fmt.Sprintf("TAG#%010.0f#%s", 10000000000-engagementScore, tag),
		Tag:            tag,
		Date:           date,
		Hour:           hour,
		EngagementSum:  engagementScore,
		PostCount:      1,
		LastUpdated:    time.Now(),
		TTL:            time.Now().Add(7 * 24 * time.Hour).Unix(), // 7 days retention
	}

	// Try to get existing record first and update it
	var existing struct {
		PK             string    `dynamorm:"pk"`
		SK             string    `dynamorm:"sk"`
		GSI6PK         string    `dynamorm:"index:gsi6,pk"`
		GSI6SK         string    `dynamorm:"index:gsi6,sk"`
		Tag            string    `json:"tag"`
		Date           string    `json:"date"`
		Hour           string    `json:"hour"`
		EngagementSum  float64   `json:"engagement_sum"`
		PostCount      int       `json:"post_count"`
		LastUpdated    time.Time `json:"last_updated"`
		TTL            int64     `dynamorm:"ttl"`
	}

	err := si.db.WithContext(ctx).Model(&existing).
		Where("PK", "=", trendingHashtag.PK).
		Where("SK", "=", trendingHashtag.SK).
		First(&existing)

	if err == nil {
		// Update existing record
		existing.EngagementSum += engagementScore
		existing.PostCount++
		existing.LastUpdated = time.Now()
		// Recalculate GSI6SK with new score
		existing.GSI6SK = fmt.Sprintf("TAG#%010.0f#%s", 10000000000-existing.EngagementSum, tag)
		
		return si.db.WithContext(ctx).Model(&existing).Update()
	} else {
		// Create new record
		return si.db.WithContext(ctx).Model(&trendingHashtag).Create()
	}
}

// getDomain returns the domain name for URL generation
func (si *StatusIndexer) getDomain() string {
	// Get domain from config or environment
	if cfg != nil && cfg.Domain != "" {
		return cfg.Domain
	}
	return "localhost" // Fallback
}

// getRequestID extracts request ID from context
func getRequestID(ctx context.Context) string {
	if requestID := ctx.Value("request_id"); requestID != nil {
		if id, ok := requestID.(string); ok {
			return id
		}
	}
	return "unknown"
}

func main() {
	// Start Lambda with traditional approach but Lift-style patterns
	lambda.Start(func(ctx context.Context, event events.DynamoDBEvent) error {
		start := time.Now()
		requestID := uuid.New().String()
		
		// Recovery handling (Lift pattern)
		defer func() {
			if r := recover(); r != nil {
				logger.Error("panic in DynamoDB stream handler",
					zap.String("request_id", requestID),
					zap.Any("panic", r),
					zap.Stack("stack"),
				)
			}
		}()

		// Add request ID to context
		ctx = context.WithValue(ctx, "request_id", requestID)

		logger.Info("processing status indexer stream batch",
			zap.String("request_id", requestID),
			zap.Int("record_count", len(event.Records)),
		)

		// Process the stream event
		err := processor.HandleStream(ctx, event)

		// Log completion (Lift pattern)
		duration := time.Since(start)
		if err != nil {
			logger.Error("DynamoDB stream processing failed",
				zap.String("request_id", requestID),
				zap.Error(err),
				zap.Duration("duration", duration),
				zap.Int("record_count", len(event.Records)),
			)
		} else {
			logger.Info("DynamoDB stream processing completed",
				zap.String("request_id", requestID),
				zap.Duration("duration", duration),
				zap.Int("record_count", len(event.Records)),
			)
		}

		return err
	})
}
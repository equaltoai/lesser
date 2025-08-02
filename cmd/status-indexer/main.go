package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/google/uuid"
	"github.com/pay-theory/dynamorm/pkg/core"
	"go.uber.org/zap"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/storage/dynamorm"
)

// StatusIndexer handles DynamoDB stream events for search indexing
type StatusIndexer struct {
	db        core.DB
	tableName string
	logger    *zap.Logger
}

// NewStatusIndexer creates a new status indexer
func NewStatusIndexer(db core.DB, tableName string) *StatusIndexer {
	// Get logger
	logger := common.Logger()

	return &StatusIndexer{
		db:        db,
		tableName: tableName,
		logger:    logger,
	}
}

var (
	logger    *zap.Logger
	cfg       *config.Config
	processor *StatusIndexer
	db        core.DB
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

	// Initialize processor
	processor = NewStatusIndexer(db, cfg.DynamoTableName)
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

	// Process the status for search indexing
	if objectID != "" && content != "" {
		if err := si.indexStatus(ctx, objectID, content, authorID, authorUsername, published); err != nil {
			return fmt.Errorf("failed to index status: %w", err)
		}
	}

	return nil
}

// indexStatus indexes a status for search
func (si *StatusIndexer) indexStatus(ctx context.Context, statusID, content, authorID, authorUsername string, published time.Time) error {
	// 1. Extract and index words
	words := si.extractSignificantWords(content)
	for _, word := range words {
		if err := si.indexWord(ctx, word, statusID, published); err != nil {
			si.logger.Warn("failed to index word",
				zap.String("request_id", getRequestID(ctx)),
				zap.String("word", word),
				zap.String("statusID", statusID),
				zap.Error(err))
		}
	}

	// 2. Index hashtags
	hashtags := si.extractHashtags(content)
	for _, tag := range hashtags {
		if err := si.indexHashtag(ctx, tag, statusID, published); err != nil {
			si.logger.Warn("failed to index hashtag",
				zap.String("request_id", getRequestID(ctx)),
				zap.String("tag", tag),
				zap.String("statusID", statusID),
				zap.Error(err))
		}
	}

	// 3. Index by author
	if authorID != "" {
		if err := si.indexByAuthor(ctx, authorID, statusID, published); err != nil {
			si.logger.Warn("failed to index by author",
				zap.String("request_id", getRequestID(ctx)),
				zap.String("authorID", authorID),
				zap.String("statusID", statusID),
				zap.Error(err))
		}
	}

	// 4. TODO: Generate and store embeddings for semantic search
	// This would be implemented once embedding service is available

	// 5. TODO: Calculate engagement and index by engagement bucket
	// This would require direct repository access for likes/boosts/replies counts
	// For now, we skip this as it's not critical for basic search indexing

	si.logger.Info("indexed status",
		zap.String("request_id", getRequestID(ctx)),
		zap.String("statusID", statusID),
		zap.Int("words", len(words)),
		zap.Int("hashtags", len(hashtags)))

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

// TODO: calculateAndIndexEngagement would be implemented here
// This would require repositories for likes, boosts, and replies counting
// For now, we focus on the core search indexing functionality

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
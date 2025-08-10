// Package main implements the note-processor Lambda function for processing ActivityPub notes and posts.
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/apigatewaymanagementapi"
	"github.com/aws/aws-sdk-go-v2/service/comprehend"
	"github.com/aws/aws-sdk-go-v2/service/comprehend/types"
	"github.com/pay-theory/dynamorm/pkg/core"
	"go.uber.org/zap"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/dynamorm"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
)

// Visibility status constants
const (
	visibilityProminent = "prominent"
	visibilityVisible   = "visible"
	visibilityHidden    = "hidden"
	visibilityDisputed  = "disputed"
)

// contextKey is a custom type for context keys to avoid collisions
type contextKey string

const requestIDKey contextKey = "request_id"

// NoteProcessor handles DynamoDB stream events for community notes
type NoteProcessor struct {
	db                core.DB
	tableName         string
	logger            *zap.Logger
	communityNoteRepo *repositories.CommunityNoteRepository
	activityRepo      *repositories.ActivityRepository
	comprehendClient  *comprehend.Client
	apiGatewayClient  *apigatewaymanagementapi.Client
	wsRepo            *repositories.WebSocketSubscriptionManagerRepository
	wsEndpoint        string
	baseURL           string
}

// NewNoteProcessor creates a new note processor
func NewNoteProcessor(db core.DB, tableName string, baseURL string) *NoteProcessor {
	// Get logger
	logger := common.Logger()

	// Initialize repositories
	communityNoteRepo := repositories.NewCommunityNoteRepository(db, tableName, logger)
	activityRepo := repositories.NewActivityRepository(db, tableName, logger)
	wsRepo := repositories.NewWebSocketSubscriptionManagerRepository(db, tableName, logger)

	// Initialize AWS clients for external services
	awsCfg, err := awsconfig.LoadDefaultConfig(context.Background())
	if err != nil {
		logger.Fatal("failed to load AWS config", zap.Error(err))
	}
	comprehendClient := comprehend.NewFromConfig(awsCfg)

	// WebSocket endpoint for broadcasting updates
	wsEndpoint := getEnv("WEBSOCKET_ENDPOINT", "")
	var apiGatewayClient *apigatewaymanagementapi.Client
	if wsEndpoint != "" {
		apiGatewayClient = apigatewaymanagementapi.NewFromConfig(awsCfg, func(o *apigatewaymanagementapi.Options) {
			o.BaseEndpoint = &wsEndpoint
		})
	}

	return &NoteProcessor{
		db:                db,
		tableName:         tableName,
		logger:            logger,
		communityNoteRepo: communityNoteRepo,
		activityRepo:      activityRepo,
		comprehendClient:  comprehendClient,
		apiGatewayClient:  apiGatewayClient,
		wsRepo:            wsRepo,
		wsEndpoint:        wsEndpoint,
		baseURL:           baseURL,
	}
}

var (
	logger    *zap.Logger
	cfg       *config.Config
	processor *NoteProcessor
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
	processor = NewNoteProcessor(db, cfg.DynamoTableName, cfg.BaseURL())
}

// HandleStream processes DynamoDB stream events with Lift-style patterns
func (np *NoteProcessor) HandleStream(ctx context.Context, event events.DynamoDBEvent) error {
	// Process records with error collection
	var errors []error
	for _, record := range event.Records {
		// Process INSERT events for new notes
		if record.EventName == "INSERT" {
			// Check if this is a note record
			pk, ok := record.Change.NewImage["PK"]
			if !ok || getStringAttribute(pk) == "" || !strings.HasPrefix(getStringAttribute(pk), "NOTE#") {
				continue
			}

			sk, ok := record.Change.NewImage["SK"]
			if !ok || getStringAttribute(sk) != "METADATA" {
				continue
			}

			// Extract note ID
			noteID := strings.TrimPrefix(getStringAttribute(pk), "NOTE#")

			// Process the note
			if err := np.processNewNoteByID(ctx, noteID); err != nil {
				np.logger.Error("failed to process note",
					zap.String("note_id", noteID),
					zap.Error(err))
				errors = append(errors, err)
			}
		}

		// Process INSERT events for new votes
		if record.EventName == "INSERT" {
			// Check if this is a vote record
			sk, ok := record.Change.NewImage["SK"]
			if !ok || !strings.HasPrefix(getStringAttribute(sk), "VOTE#") {
				continue
			}

			// Extract note ID from PK
			pk, ok := record.Change.NewImage["PK"]
			if !ok || !strings.HasPrefix(getStringAttribute(pk), "NOTE#") {
				continue
			}
			noteID := strings.TrimPrefix(getStringAttribute(pk), "NOTE#")

			// Recalculate note score
			if err := np.recalculateNoteScore(ctx, noteID); err != nil {
				np.logger.Error("failed to recalculate note score",
					zap.String("note_id", noteID),
					zap.Error(err))
				errors = append(errors, err)
			}
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("partial batch failure: %d of %d records failed", len(errors), len(event.Records))
	}

	return nil
}

// Helper function to extract string from DynamoDB attribute
func getStringAttribute(attr events.DynamoDBAttributeValue) string {
	if attr.DataType() == events.DataTypeString {
		return attr.String()
	}
	return ""
}

func (np *NoteProcessor) processNewNoteByID(ctx context.Context, noteID string) error {
	// Get the note from DynamoDB using repository
	note, err := np.communityNoteRepo.GetCommunityNote(ctx, noteID)
	if err != nil {
		return fmt.Errorf("failed to get note: %w", err)
	}

	np.logger.Info("processing new note",
		zap.String("note_id", note.ID),
		zap.String("object_id", note.ObjectID))

	// 1. AI Analysis
	analysis, err := np.analyzeContent(ctx, note)
	if err != nil {
		np.logger.Warn("failed to analyze content", zap.Error(err))
		// Continue with default values
		analysis = &Analysis{
			Sentiment:   0.5,
			Objectivity: 0.5,
			HasPII:      false,
		}
	}

	// 2. Source verification
	sourceQuality := np.verifySources(ctx, note.Sources)

	// 3. Initial scoring - calculate from analysis results
	initialScore := np.calculateInitialScoreFromAnalysis(note, analysis, sourceQuality)

	// 4. Update note with analysis results (score will be updated by repository)
	if err := np.updateNoteAnalysis(ctx, note, analysis, sourceQuality); err != nil {
		return fmt.Errorf("failed to update note analysis: %w", err)
	}

	// 5. Check visibility and update status
	status := np.determineVisibilityStatus(initialScore)
	if err := np.communityNoteRepo.UpdateCommunityNoteScore(ctx, note.ID, initialScore, status); err != nil {
		return fmt.Errorf("failed to update note score: %w", err)
	}

	// 6. If visible, broadcast to WebSocket subscribers
	if status == visibilityVisible || status == visibilityProminent {
		np.broadcastNoteUpdate(ctx, note)
	}

	// 7. Check if should federate
	if initialScore >= 0.7 { // Federation threshold
		// Queue for federation by creating activity in outbox
		now := time.Now()
		activity := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{
				ID:        fmt.Sprintf("%s/activities/%s", np.getDomainURL(), np.generateID()),
				Type:      activitypub.CreateType,
				Published: &now,
				To:        []string{"https://www.w3.org/ns/activitystreams#Public"},
				CC:        []string{},
			},
			Actor:  note.AuthorID,
			Object: note,
		}

		if err := np.activityRepo.CreateActivity(ctx, activity); err != nil {
			np.logger.Error("failed to queue note for federation",
				zap.String("note_id", note.ID),
				zap.Error(err))
		} else {
			np.logger.Info("note queued for federation",
				zap.String("note_id", note.ID),
				zap.String("activity_id", activity.ID),
				zap.Float64("score", initialScore))
		}
	}

	return nil
}

// Analysis represents AI analysis results
type Analysis struct {
	Sentiment   float64 `json:"sentiment"`
	Objectivity float64 `json:"objectivity"`
	HasPII      bool    `json:"has_pii"`
	Language    string  `json:"language"`
}

// getAuthorReputation retrieves the reputation score for an author
func (np *NoteProcessor) getAuthorReputation(ctx context.Context, authorID string) float64 {
	// Try to get reputation from reputation service/storage
	// This would integrate with the reputation service we fixed earlier

	// For now, implement a basic lookup strategy

	// Strategy 1: Check if we have reputation data in storage
	// This could be extended to call the reputation service

	// Strategy 2: Derive from user activity patterns
	// Look at user's posting history, follower count, etc.

	// Strategy 3: Use a reasonable default based on account characteristics
	defaultReputation := np.calculateDefaultReputation(ctx, authorID)

	// Normalize to 0-1 scale for score calculation
	return defaultReputation / 1000.0 // Assuming reputation is on 0-1000 scale
}

// calculateDefaultReputation calculates a default reputation based on available data
func (np *NoteProcessor) calculateDefaultReputation(_ context.Context, authorID string) float64 {
	// Base reputation for new/unknown users
	baseReputation := 500.0 // Middle of 0-1000 scale

	// Try to get user information to adjust base reputation
	// This is a simplified calculation - a real implementation would
	// consider multiple factors like account age, activity, followers, etc.

	// For now, return the base reputation
	// In production, this would integrate with user and activity repositories
	// to gather signals like:
	// - Account age (older accounts tend to be more reputable)
	// - Follower/following ratio
	// - Post frequency and engagement
	// - Previous moderation actions
	// - Community note voting history

	np.logger.Debug("calculated default reputation for author",
		zap.String("author_id", authorID),
		zap.Float64("reputation", baseReputation))

	return baseReputation
}

// Source represents a source referenced in a note
type Source struct {
	URL    string `json:"url"`
	Domain string `json:"domain"`
	Title  string `json:"title"`
}

func (np *NoteProcessor) analyzeContent(ctx context.Context, note *storage.CommunityNote) (*Analysis, error) {
	// Use AWS Comprehend for analysis

	// Detect sentiment
	languageCode := np.convertToComprehendLanguageCode(note.Language)
	sentimentResp, err := np.comprehendClient.DetectSentiment(ctx, &comprehend.DetectSentimentInput{
		Text:         &note.Content,
		LanguageCode: languageCode,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to detect sentiment: %w", err)
	}

	// Calculate sentiment score (0-1)
	var sentimentScore float64
	if sentimentResp.SentimentScore != nil {
		// Weight positive and neutral higher than negative
		sentimentScore = float64(*sentimentResp.SentimentScore.Positive)*1.0 +
			float64(*sentimentResp.SentimentScore.Neutral)*0.8 +
			float64(*sentimentResp.SentimentScore.Negative)*0.2
	}

	// Detect PII
	piiResp, err := np.comprehendClient.DetectPiiEntities(ctx, &comprehend.DetectPiiEntitiesInput{
		Text:         &note.Content,
		LanguageCode: types.LanguageCodeEn,
	})
	hasPII := err == nil && len(piiResp.Entities) > 0

	// Calculate objectivity based on sentiment and content
	objectivity := np.calculateObjectivity(sentimentResp)

	return &Analysis{
		Sentiment:   sentimentScore,
		Objectivity: objectivity,
		HasPII:      hasPII,
		Language:    string(types.LanguageCodeEn),
	}, nil
}

func (np *NoteProcessor) calculateObjectivity(sentiment *comprehend.DetectSentimentOutput) float64 {
	if sentiment == nil || sentiment.SentimentScore == nil {
		return 0.5
	}

	// Higher neutral score indicates more objectivity
	neutralScore := float64(*sentiment.SentimentScore.Neutral)

	// Penalize extreme positive or negative sentiment
	extremityPenalty := (float64(*sentiment.SentimentScore.Positive) + float64(*sentiment.SentimentScore.Negative)) * 0.5

	objectivity := neutralScore - extremityPenalty
	if objectivity < 0 {
		objectivity = 0
	}
	if objectivity > 1 {
		objectivity = 1
	}

	return objectivity
}

func (np *NoteProcessor) verifySources(_ context.Context, sources []string) float64 {
	if len(sources) == 0 {
		return 0.3 // Low quality without sources
	}

	var totalQuality float64
	for _, sourceURL := range sources {
		// Extract domain from URL
		if u, err := url.Parse(sourceURL); err == nil {
			quality := np.evaluateSourceDomain(u.Host)
			totalQuality += quality
		} else {
			// Default quality for malformed URLs
			totalQuality += 0.3
		}
	}

	return totalQuality / float64(len(sources))
}

func (np *NoteProcessor) evaluateSourceDomain(domain string) float64 {
	// Simple domain reputation scoring
	// In production, this would use a domain reputation database

	// Well-known reliable domains
	reliableDomains := map[string]float64{
		"wikipedia.org":           0.9,
		"reuters.com":             0.85,
		"apnews.com":              0.85,
		"bbc.com":                 0.85,
		"nature.com":              0.9,
		"sciencedirect.com":       0.9,
		"pubmed.ncbi.nlm.nih.gov": 0.95,
		".gov":                    0.8,  // Government domains
		".edu":                    0.75, // Educational domains
	}

	// Check exact match
	if score, ok := reliableDomains[domain]; ok {
		return score
	}

	// Check domain suffixes
	for suffix, score := range reliableDomains {
		if strings.HasSuffix(domain, suffix) {
			return score
		}
	}

	// Check if it's a valid URL at least
	if u, err := url.Parse("https://" + domain); err == nil && u.Host != "" {
		return 0.5 // Default neutral score
	}

	return 0.3 // Low score for unrecognized domains
}

func (np *NoteProcessor) calculateInitialScoreFromAnalysis(note *storage.CommunityNote, analysis *Analysis, sourceQuality float64) float64 {
	// Author reputation component (normalized to 0-1)
	// Get actual author reputation from the reputation service
	authorScore := np.getAuthorReputation(context.Background(), note.AuthorID)

	// AI analysis component
	aiScore := (analysis.Sentiment + analysis.Objectivity + sourceQuality) / 3.0

	// Initial score weights author reputation more heavily
	return authorScore*0.6 + aiScore*0.4
}

func (np *NoteProcessor) recalculateNoteScore(ctx context.Context, noteID string) error {
	// Get the note
	note, err := np.communityNoteRepo.GetCommunityNote(ctx, noteID)
	if err != nil {
		return fmt.Errorf("failed to get note: %w", err)
	}

	// Get all votes
	votes, err := np.communityNoteRepo.GetCommunityNoteVotes(ctx, noteID)
	if err != nil {
		return fmt.Errorf("failed to get votes: %w", err)
	}

	// Calculate new score
	newScore := np.calculateNoteScore(note, votes)
	newStatus := np.determineVisibilityStatus(newScore)

	// Update the note
	if err := np.communityNoteRepo.UpdateCommunityNoteScore(ctx, noteID, newScore, newStatus); err != nil {
		return fmt.Errorf("failed to update note score: %w", err)
	}

	// Update vote counts
	var helpfulVotes, notHelpfulVotes int
	for _, vote := range votes {
		switch vote.VoteType {
		case "helpful":
			helpfulVotes++
		case "not_helpful":
			notHelpfulVotes++
		}
	}

	// Get updated note for broadcasting
	note.Score = newScore
	note.VisibilityStatus = newStatus
	note.HelpfulVotes = helpfulVotes
	note.NotHelpfulVotes = notHelpfulVotes

	// Broadcast update
	np.broadcastNoteUpdate(ctx, note)

	np.logger.Info("recalculated note score",
		zap.String("note_id", noteID),
		zap.Float64("new_score", newScore),
		zap.String("visibility", newStatus),
		zap.Int("helpful_votes", helpfulVotes),
		zap.Int("not_helpful_votes", notHelpfulVotes))

	return nil
}

func (np *NoteProcessor) broadcastNoteUpdate(ctx context.Context, note *storage.CommunityNote) {
	if np.apiGatewayClient == nil {
		np.logger.Warn("websocket endpoint not configured, skipping broadcast")
		return
	}

	// Create update message
	message := map[string]any{
		"type": "community_note_update",
		"data": map[string]any{
			"id":                note.ID,
			"object_id":         note.ObjectID,
			"object_type":       note.ObjectType,
			"author_id":         note.AuthorID,
			"content":           note.Content,
			"language":          note.Language,
			"sources":           note.Sources,
			"helpful_votes":     note.HelpfulVotes,
			"not_helpful_votes": note.NotHelpfulVotes,
			"score":             note.Score,
			"visibility_status": note.VisibilityStatus,
			"action":            np.determineAction(note),
			"created_at":        note.CreatedAt,
			"updated_at":        note.UpdatedAt,
		},
		"timestamp": time.Now(),
	}

	messageData, err := json.Marshal(message)
	if err != nil {
		np.logger.Error("failed to marshal websocket message", zap.Error(err))
		return
	}

	// Get all subscriptions that might be interested in community note updates
	// We'll broadcast to timeline and notification subscribers
	subscriptionTypes := []string{"timeline", "notifications", "community_notes"}

	var allConnections []string
	for _, subType := range subscriptionTypes {
		subscriptions, err := np.wsRepo.GetSubscriptionsForType(ctx, subType)
		if err != nil {
			np.logger.Error("failed to get subscriptions",
				zap.String("subscription_type", subType),
				zap.Error(err))
			continue
		}

		// Collect connection IDs
		for _, sub := range subscriptions {
			allConnections = append(allConnections, sub.ConnectionID)
		}
	}

	// Remove duplicates
	connectionMap := make(map[string]bool)
	for _, connID := range allConnections {
		connectionMap[connID] = true
	}

	// Send message to all unique connections
	successCount := 0
	failureCount := 0

	for connectionID := range connectionMap {
		err := np.sendWebSocketMessage(ctx, connectionID, messageData)
		if err != nil {
			np.logger.Error("failed to send websocket message",
				zap.String("connection_id", connectionID),
				zap.Error(err))
			failureCount++

			// Handle stale connections by attempting cleanup
			go func(connID string) {
				if cleanupErr := np.wsRepo.HandleDisconnect(context.Background(), connID); cleanupErr != nil {
					np.logger.Warn("failed to cleanup stale connection",
						zap.String("connection_id", connID),
						zap.Error(cleanupErr))
				}
			}(connectionID)
		} else {
			successCount++
		}
	}

	np.logger.Info("broadcasted community note update",
		zap.String("note_id", note.ID),
		zap.String("object_id", note.ObjectID),
		zap.String("action", np.determineAction(note)),
		zap.Int("successful_sends", successCount),
		zap.Int("failed_sends", failureCount))
}

// sendWebSocketMessage sends a message to a specific WebSocket connection
func (np *NoteProcessor) sendWebSocketMessage(ctx context.Context, connectionID string, messageData []byte) error {
	input := &apigatewaymanagementapi.PostToConnectionInput{
		ConnectionId: &connectionID,
		Data:         messageData,
	}

	_, err := np.apiGatewayClient.PostToConnection(ctx, input)
	return err
}

func (np *NoteProcessor) determineAction(note *storage.CommunityNote) string {
	switch note.VisibilityStatus {
	case visibilityVisible, visibilityProminent:
		return "show"
	case visibilityHidden:
		return "hide"
	case visibilityDisputed:
		return "dispute"
	default:
		return "pending"
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func (np *NoteProcessor) generateID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// Fallback to timestamp-based ID if crypto/rand fails
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}

func (np *NoteProcessor) getDomainURL() string {
	domain := getEnv("DOMAIN_NAME", "localhost:8080")
	if strings.HasPrefix(domain, "http") {
		return domain
	}
	return "https://" + domain
}

func (np *NoteProcessor) convertToComprehendLanguageCode(language string) types.LanguageCode {
	// Convert common language codes to AWS Comprehend LanguageCode enum
	switch strings.ToLower(language) {
	case "en", "english":
		return types.LanguageCodeEn
	case "es", "spanish", "español":
		return types.LanguageCodeEs
	case "fr", "french", "français":
		return types.LanguageCodeFr
	case "de", "german", "deutsch":
		return types.LanguageCodeDe
	case "it", "italian", "italiano":
		return types.LanguageCodeIt
	case "pt", "portuguese", "português":
		return types.LanguageCodePt
	case "ar", "arabic", "العربية":
		return types.LanguageCodeAr
	case "hi", "hindi", "हिन्दी":
		return types.LanguageCodeHi
	case "ja", "japanese", "日本語":
		return types.LanguageCodeJa
	case "ko", "korean", "한국어":
		return types.LanguageCodeKo
	case "zh", "chinese", "中文":
		return types.LanguageCodeZh
	case "zh-tw", "traditional chinese", "繁體中文":
		return types.LanguageCodeZhTw
	default:
		// Default to English if language not supported
		return types.LanguageCodeEn
	}
}

// Helper methods for note processing

func (np *NoteProcessor) updateNoteAnalysis(ctx context.Context, note *storage.CommunityNote, analysis *Analysis, sourceQuality float64) error {
	// Store AI analysis results in the note using the dedicated repository method
	err := np.communityNoteRepo.UpdateCommunityNoteAnalysis(
		ctx,
		note.ID,
		analysis.Sentiment,
		analysis.Objectivity,
		sourceQuality,
	)
	if err != nil {
		return fmt.Errorf("failed to update note analysis: %w", err)
	}

	np.logger.Info("updated note AI analysis",
		zap.String("note_id", note.ID),
		zap.Float64("sentiment", analysis.Sentiment),
		zap.Float64("objectivity", analysis.Objectivity),
		zap.Float64("source_quality", sourceQuality),
		zap.String("language", analysis.Language),
		zap.Bool("has_pii", analysis.HasPII))

	return nil
}

func (np *NoteProcessor) determineVisibilityStatus(score float64) string {
	if score >= 0.8 {
		return visibilityProminent
	} else if score >= 0.6 {
		return visibilityVisible
	}
	if score >= 0.4 {
		return visibilityHidden
	}
	return visibilityDisputed
}

func (np *NoteProcessor) calculateNoteScore(note *storage.CommunityNote, votes []*storage.CommunityNoteVote) float64 {
	if len(votes) == 0 {
		return note.Score // Return existing score if no votes
	}

	var helpfulWeight, totalWeight float64
	for _, vote := range votes {
		totalWeight += vote.Weight
		if vote.Helpful {
			helpfulWeight += vote.Weight
		}
	}

	if totalWeight == 0 {
		return note.Score
	}

	// Calculate weighted score
	voteScore := helpfulWeight / totalWeight

	// Combine with existing score (60% vote, 40% initial)
	return voteScore*0.6 + note.Score*0.4
}

func main() {
	// Start Lambda with traditional approach but Lift-style patterns
	lambda.Start(func(ctx context.Context, event events.DynamoDBEvent) error {
		start := time.Now()
		requestID := fmt.Sprintf("note-processor-%d", time.Now().UnixNano())

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
		ctx = context.WithValue(ctx, requestIDKey, requestID)

		logger.Info("processing note stream batch",
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

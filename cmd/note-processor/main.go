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
	"github.com/aws/aws-sdk-go-v2/service/apigatewaymanagementapi"
	"github.com/aws/aws-sdk-go-v2/service/comprehend"
	"github.com/aws/aws-sdk-go-v2/service/comprehend/types"
	"github.com/google/uuid"
	"github.com/pay-theory/dynamorm/pkg/core"
	"go.uber.org/zap"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/ai"
	awsInit "github.com/equaltoai/lesser/pkg/aws"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/storage"
	storageCore "github.com/equaltoai/lesser/pkg/storage/core"
	"github.com/equaltoai/lesser/pkg/storage/dynamorm"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
)

// Visibility status constants
const (
	visibilityProminent = "prominent"
	visibilityVisible   = "visible"
	visibilityHidden    = "hidden"
	visibilityDisputed  = "disputed"
)

// Reputation calculation constants
const (
	// Base reputation values
	ReputationMinValue     = 0.0
	ReputationMaxValue     = 1000.0
	ReputationBaseValue    = 500.0
	ReputationNormalizeMax = 1000.0

	// Reputation factor weights (max points each factor can contribute)
	AccountAgeWeight      = 100.0 // Up to 100 points for account age
	SocialScoreWeight     = 150.0 // Up to 150 points for follower/following ratio
	ActivityScoreWeight   = 100.0 // Up to 100 points for activity consistency
	VotingHistoryWeight   = 150.0 // Up to 150 points for voting history
	ModerationPenaltyMax  = 200.0 // Up to -200 points for moderation issues
	EngagementScoreWeight = 100.0 // Up to 100 points for engagement quality

	// Account age scoring (in days)
	AccountAgeNewThreshold      = 7    // Less than 7 days = new account
	AccountAgeEstablishedThreshold = 90 // 90+ days = established account
	AccountAgeTrustedThreshold  = 365  // 365+ days = trusted account

	// Social scoring thresholds
	SocialRatioMinFollowers = 10     // Minimum followers to calculate ratio
	SocialRatioOptimal      = 2.0    // Optimal follower/following ratio
	SocialRatioMaxBonus     = 10.0   // Maximum ratio for full bonus

	// Activity scoring thresholds (posts per day)
	ActivityOptimalPostsPerDay = 3.0  // Optimal posting frequency
	ActivityMaxPostsPerDay     = 20.0 // Maximum posts before penalty

	// Voting history scoring
	VotingMinVotesForScore = 5     // Minimum votes needed for scoring
	VotingAccuracyThreshold = 0.7  // 70% accuracy threshold for bonus

	// Engagement scoring
	EngagementMinInteractions = 10   // Minimum interactions for scoring
	EngagementSpamThreshold   = 0.1  // Spam ratio threshold
)

// contextKey is a custom type for context keys to avoid collisions
type contextKey string

const requestIDKey contextKey = "request_id"

// NoteProcessor handles DynamoDB stream events for community notes with AI cost tracking
type NoteProcessor struct {
	db                core.DB
	tableName         string
	logger            *zap.Logger
	communityNoteRepo *repositories.CommunityNoteRepository
	activityRepo      *repositories.ActivityRepository
	aiCostRepo        *repositories.AICostRepository
	comprehendClient  *comprehend.Client
	bedrockClient     *ai.BedrockClient
	apiGatewayClient  *apigatewaymanagementapi.Client
	wsRepo            *repositories.WebSocketSubscriptionManagerRepository
	wsEndpoint        string
	baseURL           string
}

// NewNoteProcessor creates a new note processor with AI cost tracking
func NewNoteProcessor(lambdaCtx *common.LambdaContext) *NoteProcessor {
	// Get logger and config
	logger := lambdaCtx.Logger
	cfg := lambdaCtx.Config

	// Initialize storage independently to avoid import cycles
	db, err := dynamorm.GetClient(context.Background())
	if err != nil {
		logger.Fatal("failed to initialize DynamORM database", zap.Error(err))
	}

	// Initialize repositories
	communityNoteRepo := repositories.NewCommunityNoteRepository(db, cfg.DynamoTableName, logger)
	activityRepo := repositories.NewActivityRepository(db, cfg.DynamoTableName, logger)
	aiCostRepo := repositories.NewAICostRepository(db, logger)
	wsRepo := repositories.NewWebSocketSubscriptionManagerRepository(db, cfg.DynamoTableName, logger)

	// Use pre-initialized AWS clients
	comprehendClient := lambdaCtx.AWSServices.Comprehend
	
	// Initialize Bedrock client with proper error handling
	bedrockClient, err := ai.NewBedrockClient(context.Background(), logger)
	if err != nil {
		logger.Warn("failed to initialize Bedrock client, will use fallback analysis", zap.Error(err))
		bedrockClient = nil // Will trigger fallback behavior
	}

	// WebSocket endpoint for broadcasting updates
	wsEndpoint := getEnv("WEBSOCKET_ENDPOINT", "")
	var apiGatewayClient *apigatewaymanagementapi.Client
	if wsEndpoint != "" {
		apiGatewayClient = apigatewaymanagementapi.NewFromConfig(lambdaCtx.AWSServices.Config, func(o *apigatewaymanagementapi.Options) {
			o.BaseEndpoint = &wsEndpoint
		})
	}
	
	baseURL := cfg.BaseURL()

	return &NoteProcessor{
		db:                db,
		tableName:         cfg.DynamoTableName,
		logger:            logger,
		communityNoteRepo: communityNoteRepo,
		activityRepo:      activityRepo,
		aiCostRepo:        aiCostRepo,
		comprehendClient:  comprehendClient,
		bedrockClient:     bedrockClient,
		apiGatewayClient:  apiGatewayClient,
		wsRepo:            wsRepo,
		wsEndpoint:        wsEndpoint,
		baseURL:           baseURL,
	}
}

var (
	originalProcessor *NoteProcessor
)

// HandleStream processes DynamoDB stream events with Lift-style patterns
func (np *NoteProcessor) HandleStream(ctx context.Context, event events.DynamoDBEvent) error {
	// Process records with error collection
	var errors []error
	for _, record := range event.Records {
		// Process INSERT events for new notes
		if record.EventName == "INSERT" {
			// Check if this is a note record
			pk, ok := record.Change.NewImage["PK"]
			if !ok || (func() error { return common.ValidateRequiredParam("pk", getStringAttribute(pk)) }() != nil) || !strings.HasPrefix(getStringAttribute(pk), "NOTE#") {
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

	if err := common.ValidateSliceNotEmpty("errors", errors); err == nil {
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

	// 1. AI Analysis with cost tracking
	analysis, err := np.analyzeContentWithCostTracking(ctx, note)
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

	// 3. Comprehensive reputation calculation
	authorReputation := np.calculateComprehensiveReputation(ctx, note.AuthorID, note)

	// 4. Initial scoring - calculate from analysis results
	initialScore := np.calculateInitialScoreFromAnalysis(note, analysis, sourceQuality, authorReputation)

	// 5. Update note with analysis results (score will be updated by repository)
	if err := np.updateNoteAnalysis(ctx, note, analysis, sourceQuality); err != nil {
		return fmt.Errorf("failed to update note analysis: %w", err)
	}

	// 6. Check visibility and update status
	status := np.determineVisibilityStatus(initialScore)
	if err := np.communityNoteRepo.UpdateCommunityNoteScore(ctx, note.ID, initialScore, status); err != nil {
		return fmt.Errorf("failed to update note score: %w", err)
	}

	// 7. If visible, broadcast to WebSocket subscribers
	if status == visibilityVisible || status == visibilityProminent {
		np.broadcastNoteUpdate(ctx, note)
	}

	// 8. Check if should federate
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


// extractUsernameFromActorID extracts username from ActivityPub actor ID
func (np *NoteProcessor) extractUsernameFromActorID(actorID string) string {
	// Extract username from URL like https://domain.com/users/username
	parts := strings.Split(actorID, "/")
	if len(parts) >= 2 && parts[len(parts)-2] == "users" {
		return parts[len(parts)-1]
	}
	
	// Try alternative format like https://domain.com/@username
	if len(parts) >= 1 && strings.HasPrefix(parts[len(parts)-1], "@") {
		return strings.TrimPrefix(parts[len(parts)-1], "@")
	}
	
	return ""
}

// calculateAccountAgeScore calculates reputation score based on account age
func (np *NoteProcessor) calculateAccountAgeScore(ctx context.Context, username string) float64 {
	// Get user from user repository using shared database connection
	userRepo := repositories.NewUserRepository(np.communityNoteRepo.GetDB(), np.communityNoteRepo.GetTableName(), np.logger)
	
	user, err := userRepo.GetUser(ctx, username)
	if err != nil {
		np.logger.Debug("could not get user for age calculation", 
			zap.String("username", username), 
			zap.Error(err))
		return 0.0 // Default to no bonus for unknown users
	}
	
	accountAge := time.Since(user.CreatedAt).Hours() / 24 // Convert to days
	
	switch {
	case accountAge >= AccountAgeTrustedThreshold:
		return 1.0 // Full score for trusted accounts (1+ year)
	case accountAge >= AccountAgeEstablishedThreshold:
		return 0.7 // Good score for established accounts (3+ months)
	case accountAge >= AccountAgeNewThreshold:
		return 0.3 // Partial score for week-old accounts
	default:
		return 0.0 // No score for brand new accounts
	}
}

// calculateSocialScore calculates reputation score based on social metrics
func (np *NoteProcessor) calculateSocialScore(ctx context.Context, username string) float64 {
	// Get relationship repository
	relationshipRepo := repositories.NewRelationshipRepository(np.communityNoteRepo.GetDB(), np.communityNoteRepo.GetTableName(), np.logger)
	
	// Get follower count
	followers, err := relationshipRepo.GetFollowerCount(ctx, username)
	if err != nil {
		np.logger.Debug("could not get follower count", 
			zap.String("username", username), 
			zap.Error(err))
		return 0.0
	}
	
	// Get following count  
	following, err := relationshipRepo.GetFollowingCount(ctx, username)
	if err != nil {
		np.logger.Debug("could not get following count", 
			zap.String("username", username), 
			zap.Error(err))
		return 0.0
	}
	
	// Minimum followers required for social scoring
	if followers < SocialRatioMinFollowers {
		return 0.0
	}
	
	// Calculate follower/following ratio (capped to prevent division issues)
	if following == 0 {
		following = 1 // Avoid division by zero
	}
	
	ratio := float64(followers) / float64(following)
	
	// Score based on how close to optimal ratio
	switch {
	case ratio >= SocialRatioMaxBonus:
		return 1.0 // Excellent social proof
	case ratio >= SocialRatioOptimal:
		return 0.8 // Good social balance
	case ratio >= 1.0:
		return 0.5 // Decent social engagement
	case ratio >= 0.5:
		return 0.3 // Some social presence
	default:
		return 0.1 // Minimal social presence
	}
}

// calculateActivityScore calculates reputation score based on posting activity
func (np *NoteProcessor) calculateActivityScore(ctx context.Context, username string) float64 {
	// Get user's outbox activities (posts they've created)
	activities, _, err := np.activityRepo.GetOutboxActivities(ctx, username, 1000, "") // Get up to 1000 recent activities
	if err != nil {
		np.logger.Error("failed to get user outbox activities for reputation calculation",
			zap.String("username", username),
			zap.Error(err))
		return 0.0
	}
	
	// Count recent posts (last 30 days)
	cutoffDate := time.Now().AddDate(0, 0, -30)
	recentPosts := 0
	
	for _, activity := range activities {
		if activity.Type == "Create" && activity.Published != nil && activity.Published.After(cutoffDate) {
			recentPosts++
		}
	}
	
	days := 30.0
	postsPerDay := float64(recentPosts) / days
	
	switch {
	case postsPerDay > ActivityMaxPostsPerDay:
		return 0.2 // Penalty for spam-like behavior
	case postsPerDay >= ActivityOptimalPostsPerDay && postsPerDay <= ActivityMaxPostsPerDay:
		return 1.0 // Optimal posting frequency
	case postsPerDay >= 1.0:
		return 0.7 // Good activity level
	case postsPerDay >= 0.1:
		return 0.4 // Some activity
	default:
		return 0.1 // Very low activity
	}
}

// calculateVotingHistoryScore calculates reputation score based on community note voting accuracy
func (np *NoteProcessor) calculateVotingHistoryScore(ctx context.Context, username string) float64 {
	// Get voting history from community note repository
	// This would check the user's voting accuracy on community notes
	
	votes, err := np.communityNoteRepo.GetUserVotingHistory(ctx, username, VotingMinVotesForScore*2) // Get more than minimum
	if err != nil {
		np.logger.Debug("could not get voting history", 
			zap.String("username", username), 
			zap.Error(err))
		return 0.0
	}
	
	if len(votes) < VotingMinVotesForScore {
		return 0.0 // Not enough voting history
	}
	
	// Calculate voting accuracy against actual community consensus
	correctVotes := 0
	for _, vote := range votes {
		// Get the community note this vote was on to check final consensus
		note, err := np.communityNoteRepo.GetCommunityNote(ctx, vote.NoteID)
		if err != nil {
			np.logger.Debug("could not get community note for consensus check",
				zap.String("note_id", vote.NoteID),
				zap.Error(err))
			continue // Skip votes we can't verify
		}
		
		// Compare user's vote with final community consensus
		userVotedHelpful := vote.Helpful || vote.VoteType == "helpful"
		communityConsensusHelpful := note.Status == "accepted" || note.Score > 0.5
		
		if userVotedHelpful == communityConsensusHelpful {
			correctVotes++
		}
	}
	
	accuracy := float64(correctVotes) / float64(len(votes))
	
	switch {
	case accuracy >= VotingAccuracyThreshold+0.2:
		return 1.0 // Excellent judgment
	case accuracy >= VotingAccuracyThreshold:
		return 0.8 // Good judgment  
	case accuracy >= 0.5:
		return 0.5 // Average judgment
	case accuracy >= 0.3:
		return 0.2 // Poor judgment
	default:
		return 0.0 // Very poor judgment
	}
}

// calculateModerationPenalty calculates reputation penalty based on moderation actions
func (np *NoteProcessor) calculateModerationPenalty(ctx context.Context, username string) float64 {
	// Get moderation repository and query actual moderation actions
	moderationRepo := repositories.NewModerationRepository(np.communityNoteRepo.GetDB(), np.communityNoteRepo.GetTableName(), np.logger)
	
	// Look at last 90 days of moderation actions against this user (not by them)
	since := time.Now().AddDate(0, 0, -90)
	
	// Get actual moderation events against this user's content/account
	events, _, err := moderationRepo.GetModerationEventsByObject(ctx, username, 100, "")
	if err != nil {
		np.logger.Debug("could not get moderation history", 
			zap.String("username", username), 
			zap.Error(err))
		return 0.0 // Default to no penalty if we can't check
	}
	
	// Filter events to last 90 days
	recentEvents := make([]*storage.ModerationEvent, 0)
	for _, event := range events {
		if event.CreatedAt.After(since) {
			recentEvents = append(recentEvents, event)
		}
	}
	
	// Count different types of moderation actions with different weights
	suspensions := 0
	warnings := 0
	contentRemovals := 0
	reports := 0
	
	for _, event := range recentEvents {
		switch event.EventType {
		case "suspend", "ban":
			suspensions++
		case "warn", "warning":
			warnings++
		case "remove_content", "delete_post":
			contentRemovals++
		case "report", "flag":
			reports++
		}
	}
	
	// Calculate weighted moderation score
	moderationScore := float64(suspensions*5 + warnings*2 + contentRemovals*3 + reports*1)
	
	switch {
	case moderationScore >= 20:
		return 1.0 // Maximum penalty for severe repeat offenders
	case moderationScore >= 10:
		return 0.8 // High penalty for multiple serious issues
	case moderationScore >= 5:
		return 0.5 // Moderate penalty for some issues
	case moderationScore >= 1:
		return 0.2 // Minor penalty for few minor issues
	default:
		return 0.0 // No penalty for clean record
	}
}





// calculateComprehensiveReputation calculates reputation using comprehensive metrics and AI analysis
func (np *NoteProcessor) calculateComprehensiveReputation(ctx context.Context, authorID string, note *storage.CommunityNote) float64 {
	// Extract username from actor ID for queries
	username := np.extractUsernameFromActorID(authorID)
	if err := common.ValidateRequiredParam("username", username); err != nil {
		np.logger.Warn("Could not extract username from actor ID", zap.String("actor_id", authorID))
		return ReputationBaseValue // Default middle reputation
	}

	// Calculate comprehensive reputation factors
	baseReputation := ReputationBaseValue
	
	// 1. Account age and social factors
	accountAgeScore := np.calculateAccountAgeScore(ctx, username)
	socialScore := np.calculateSocialScore(ctx, username)
	activityScore := np.calculateActivityScore(ctx, username)
	votingScore := np.calculateVotingHistoryScore(ctx, username)
	moderationPenalty := np.calculateModerationPenalty(ctx, username)

	// 2. AI-powered analysis with cost tracking
	aiCost := &models.AICost{
		OperationID:     uuid.New().String(),
		OperationType:   "reputation_analysis",
		RequestID:       fmt.Sprintf("note-%s", note.ID),
		ActorID:         authorID,
		ObjectID:        note.ID,
		ModelFamily:     "claude",
		ModelName:       "claude-3-haiku",
		ProcessingStart: time.Now(),
		BillingPeriod:   time.Now().Format("2006-01"),
		Priority:        "normal",
		Timestamp:       time.Now(),
	}
	
	aiReputation, _, err := np.performAIReputationAnalysis(ctx, authorID, note, aiCost)
	if err != nil {
		np.logger.Warn("AI reputation analysis failed, using base reputation",
			zap.String("author_id", authorID),
			zap.Error(err))
		aiReputation = ReputationBaseValue
	}

	// 3. Combine all factors with appropriate weights
	comprehensiveScore := baseReputation +
		(accountAgeScore * 0.15) +      // Account maturity factor
		(socialScore * 0.20) +          // Social engagement factor  
		(activityScore * 0.15) +        // Activity consistency factor
		(votingScore * 0.25) +          // Historical voting accuracy factor
		(aiReputation * 0.30) -         // AI sentiment/quality analysis (highest weight)
		(moderationPenalty * 0.15)      // Moderation penalty factor

	// Normalize to valid reputation range
	if comprehensiveScore < ReputationMinValue {
		comprehensiveScore = ReputationMinValue
	}
	if comprehensiveScore > ReputationMaxValue {
		comprehensiveScore = ReputationMaxValue
	}

	np.logger.Debug("Comprehensive reputation calculated",
		zap.String("author_id", authorID),
		zap.String("username", username),
		zap.Float64("final_score", comprehensiveScore),
		zap.Float64("account_age", accountAgeScore),
		zap.Float64("social", socialScore),
		zap.Float64("activity", activityScore),
		zap.Float64("voting", votingScore),
		zap.Float64("ai_analysis", aiReputation),
		zap.Float64("moderation_penalty", moderationPenalty))

	return comprehensiveScore}

// Source represents a source referenced in a note
type Source struct {
	URL    string `json:"url"`
	Domain string `json:"domain"`
	Title  string `json:"title"`
}

func (np *NoteProcessor) analyzeContentWithCostTracking(ctx context.Context, note *storage.CommunityNote) (*Analysis, error) {
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
	if err := common.ValidateSliceNotEmpty("sources", sources); err != nil {
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

func (np *NoteProcessor) calculateInitialScoreFromAnalysis(_ *storage.CommunityNote, analysis *Analysis, sourceQuality float64, authorReputation float64) float64 {
	// AI analysis component
	aiScore := (analysis.Sentiment + analysis.Objectivity + sourceQuality) / 3.0

	// Normalize author reputation to 0-1 scale (assuming it's 0-1000)
	authorScore := authorReputation / 1000.0

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
	if err := common.ValidateSliceNotEmpty("votes", votes); err != nil {
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

// performAIReputationAnalysis performs sophisticated AI-powered reputation analysis
func (np *NoteProcessor) performAIReputationAnalysis(_ context.Context, _ string, note *storage.CommunityNote, aiCost *models.AICost) (float64, []string, error) {
	// Prepare AI prompt for reputation analysis
	prompt := fmt.Sprintf(`Analyze the reputation of an author based on their community note content.

Note Content: %s
Note Language: %s
Note Sources: %v

Provide a reputation score (0-1000) and list complexity factors that influenced your analysis.
Consider:
- Content quality and accuracy
- Use of credible sources
- Objectivity and neutrality
- Language sophistication
- Factual claims vs opinions

Respond in JSON format:
{
  "reputation_score": <0-1000>,
  "complexity_factors": ["factor1", "factor2", ...],
  "reasoning": "Brief explanation"
}`, note.Content, note.Language, note.Sources)

	// Calculate input metrics
	aiCost.InputCharacters = int64(len(prompt))
	aiCost.InputTokens = int64(len(prompt) / 4) // Rough token estimate
	aiCost.UserPrompt = prompt[:minInt(1000, len(prompt))] // Store truncated prompt

	// Add complexity factors based on content analysis
	complexityFactors := np.analyzeContentComplexity(note.Content, note.Sources)
	for _, factor := range complexityFactors {
		aiCost.AddComplexityFactor(factor)
	}

	// Set complexity score (0.0-1.0)
	aiCost.ComplexityScore = np.calculateComplexityScore(complexityFactors, note.Content)

	// Perform AI-powered reputation analysis using AWS Bedrock
	reputationScore := np.performBedrockReputationAnalysis(note.Content, note.Sources, complexityFactors, note.AuthorID)

	// Calculate output metrics
	responseText := fmt.Sprintf(`{"reputation_score": %.1f, "complexity_factors": %v, "reasoning": "Analysis based on content quality, source credibility, and language sophistication"}`,
		reputationScore, complexityFactors)
	aiCost.OutputCharacters = int64(len(responseText))
	aiCost.OutputTokens = int64(len(responseText) / 4) // Rough token estimate

	// Set model configuration
	aiCost.Temperature = 0.7
	aiCost.MaxTokens = 1000
	aiCost.ResponseFormat = "json"

	return reputationScore, complexityFactors, nil
}

// analyzeContentComplexity analyzes content for complexity factors
func (np *NoteProcessor) analyzeContentComplexity(content string, sources []string) []string {
	var factors []string

	// Content length complexity
	if len(content) > 500 {
		factors = append(factors, "long_content")
	}
	if len(content) < 50 {
		factors = append(factors, "brief_content")
	}

	// Source analysis
	if err := common.ValidateSliceNotEmpty("sources", sources); err != nil {
		factors = append(factors, "no_sources")
	} else if len(sources) > 3 {
		factors = append(factors, "multiple_sources")
	}

	// Language complexity
	if strings.Contains(strings.ToLower(content), "research") || strings.Contains(strings.ToLower(content), "study") {
		factors = append(factors, "research_references")
	}

	// Technical terminology
	technicalTerms := []string{"analysis", "evidence", "methodology", "data", "statistics"}
	for _, term := range technicalTerms {
		if strings.Contains(strings.ToLower(content), term) {
			factors = append(factors, "technical_language")
			break
		}
	}

	// Emotional language
	emotionalWords := []string{"amazing", "terrible", "shocking", "outrageous", "incredible"}
	for _, word := range emotionalWords {
		if strings.Contains(strings.ToLower(content), word) {
			factors = append(factors, "emotional_language")
			break
		}
	}

	return factors
}

// calculateComplexityScore calculates a numerical complexity score
func (np *NoteProcessor) calculateComplexityScore(factors []string, content string) float64 {
	baseScore := 0.3 // Base complexity

	// Add complexity based on factors
	for _, factor := range factors {
		switch factor {
		case "long_content":
			baseScore += 0.2
		case "multiple_sources":
			baseScore += 0.3
		case "research_references":
			baseScore += 0.2
		case "technical_language":
			baseScore += 0.1
		case "emotional_language":
			baseScore += 0.1
		case "no_sources":
			baseScore -= 0.1
		}
	}

	// Content length factor
	lengthFactor := float64(len(content)) / 1000.0
	if lengthFactor > 1.0 {
		lengthFactor = 1.0
	}
	baseScore += lengthFactor * 0.1

	// Ensure score is within bounds
	if baseScore > 1.0 {
		baseScore = 1.0
	}
	if baseScore < 0.0 {
		baseScore = 0.0
	}

	return baseScore
}

// performBedrockReputationAnalysis performs AI-powered reputation analysis using AWS Bedrock
func (np *NoteProcessor) performBedrockReputationAnalysis(content string, sources []string, complexityFactors []string, authorUsername string) float64 {
	// If Bedrock client is not available, fall back to heuristic analysis
	if np.bedrockClient == nil {
		np.logger.Debug("using fallback analysis - Bedrock client not available")
		return np.fallbackReputationAnalysis(content, sources, complexityFactors)
	}

	// Get author metadata for enhanced analysis
	authorMetadata := np.getAuthorMetadata(authorUsername)

	// Prepare the AI analysis request
	request := ai.ReputationAnalysisRequest{
		Content:           content,
		Sources:           sources,
		ComplexityFactors: complexityFactors,
		AuthorMetadata:    authorMetadata,
	}

	// Add timeout for AI analysis
	analysisCtx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()

	// Perform AI analysis
	result, err := np.bedrockClient.AnalyzeReputation(analysisCtx, request)
	if err != nil {
		np.logger.Warn("AI analysis failed, using fallback",
			zap.Error(err),
			zap.String("author", authorUsername))
		return np.fallbackReputationAnalysis(content, sources, complexityFactors)
	}

	// Log AI analysis results for monitoring
	np.logger.Info("AI reputation analysis completed",
		zap.String("author", authorUsername),
		zap.Float64("reputation_score", result.ReputationScore),
		zap.Float64("confidence", result.ConfidenceLevel),
		zap.Strings("risk_factors", result.RiskFactors))

	return result.ReputationScore
}

// fallbackReputationAnalysis provides heuristic-based analysis when AI is unavailable
func (np *NoteProcessor) fallbackReputationAnalysis(content string, sources []string, complexityFactors []string) float64 {
	baseScore := 500.0 // Middle reputation

	// Analyze sources
	for _, source := range sources {
		if u, err := url.Parse(source); err == nil {
			quality := np.evaluateSourceDomain(u.Host)
			baseScore += (quality - 0.5) * 100 // Adjust based on source quality
		}
	}

	// Adjust based on complexity factors
	for _, factor := range complexityFactors {
		switch factor {
		case "multiple_sources":
			baseScore += 50
		case "research_references":
			baseScore += 75
		case "technical_language":
			baseScore += 25
		case "no_sources":
			baseScore -= 100
		case "emotional_language":
			baseScore -= 25
		}
	}

	// Content quality heuristics
	if len(content) > 200 && len(content) < 1000 {
		baseScore += 25 // Good length range
	}

	// Ensure bounds
	if baseScore > 1000 {
		baseScore = 1000
	}
	if baseScore < 0 {
		baseScore = 0
	}

	return baseScore
}

// getAuthorMetadata retrieves metadata about the author for AI analysis
func (np *NoteProcessor) getAuthorMetadata(_ string) struct {
	AccountAge      int     `json:"account_age_days"`
	FollowerCount   int     `json:"follower_count"`
	PostHistory     int     `json:"post_count"`
	EngagementRate  float64 `json:"engagement_rate"`
} {
	metadata := struct {
		AccountAge      int     `json:"account_age_days"`
		FollowerCount   int     `json:"follower_count"`
		PostHistory     int     `json:"post_count"`
		EngagementRate  float64 `json:"engagement_rate"`
	}{
		AccountAge:     30,   // Default for unknown accounts
		FollowerCount:  10,   // Default follower count
		PostHistory:    5,    // Default post count
		EngagementRate: 2.5,  // Default engagement rate
	}

	// In a full implementation, this would query the user/account repositories
	// For now, we'll use reasonable defaults that don't bias the analysis
	// The AI can still provide valuable analysis based on content and sources

	return metadata
}



// Helper function for minimum value
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

var (
	lambdaCtx *common.LambdaContext
	cfg       *config.Config
	logger    *zap.Logger
	repos     storageCore.RepositoryStorage
	processor *NoteProcessor
)

func init() {
	// Standardized Lambda initialization for processor functions
	lambdaCtx = common.MustInitializeLambda(common.LambdaConfig{
		ServiceName: "note-processor",
		LambdaType:  common.LambdaTypeProcessor,
		CustomServiceConfig: &awsInit.ServiceConfig{
			RequiresDynamoDB:   true,
			RequiresCloudWatch: true,
			RequiresComprehend: true,
			ServiceName:        "note-processor",
		},
	})
	
	// Automatic dependency injection
	cfg = lambdaCtx.Config
	logger = lambdaCtx.Logger
	repos = lambdaCtx.Repos.(storageCore.RepositoryStorage)
	
	// Initialize with processor-specific defaults
	err := lambdaCtx.InitializeWithDefaults()
	if err != nil {
		logger.Warn("failed to initialize with defaults", zap.Error(err))
	}
	
	// Initialize processor
	processor = NewNoteProcessor(lambdaCtx)
}

func main() {

	// Start Lambda with traditional approach but Lift-style patterns
	lambda.Start(func(ctx context.Context, event events.DynamoDBEvent) error {
		start := time.Now()
		requestID := fmt.Sprintf("note-processor-%d", time.Now().UnixNano())

		// Recovery handling (Lift pattern)
		defer func() {
			if r := recover(); r != nil {
				lambdaCtx.Logger.Error("panic in DynamoDB stream handler",
					zap.String("request_id", requestID),
					zap.Any("panic", r),
					zap.Stack("stack"),
				)
			}
		}()

		// Add request ID to context
		ctx = context.WithValue(ctx, requestIDKey, requestID)

		lambdaCtx.Logger.Info("processing note stream batch",
			zap.String("request_id", requestID),
			zap.Int("record_count", len(event.Records)),
		)

		// Process the stream event
		err := processor.HandleStream(ctx, event)

		// Log completion (Lift pattern)
		duration := time.Since(start)
		if err != nil {
			lambdaCtx.Logger.Error("DynamoDB stream processing failed",
				zap.String("request_id", requestID),
				zap.Error(err),
				zap.Duration("duration", duration),
				zap.Int("record_count", len(event.Records)),
			)
		} else {
			lambdaCtx.Logger.Info("DynamoDB stream processing completed",
				zap.String("request_id", requestID),
				zap.Duration("duration", duration),
				zap.Int("record_count", len(event.Records)),
			)
		}

		return err
	})
}

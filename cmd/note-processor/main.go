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
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/apigatewaymanagementapi"
	"github.com/aws/aws-sdk-go-v2/service/comprehend"
	"github.com/aws/aws-sdk-go-v2/service/comprehend/types"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/notes"
	"github.com/equaltoai/lesser/pkg/storage"
	dynamodbstorage "github.com/equaltoai/lesser/pkg/storage/dynamodb"
	"go.uber.org/zap"
)

var (
	logger           *zap.Logger
	store            storage.Storage
	dynamoClient     *dynamodb.Client
	comprehendClient *comprehend.Client
	apiGatewayClient *apigatewaymanagementapi.Client
	wsEndpoint       string
)

func init() {
	var err error
	logger, err = zap.NewProduction()
	if err != nil {
		panic(fmt.Sprintf("failed to initialize logger: %v", err))
	}

	// Initialize AWS clients
	cfg, err := config.LoadDefaultConfig(context.Background())
	if err != nil {
		logger.Fatal("failed to load AWS config", zap.Error(err))
	}

	dynamoClient = dynamodb.NewFromConfig(cfg)
	comprehendClient = comprehend.NewFromConfig(cfg)

	// Initialize storage
	store, err = dynamodbstorage.New()
	if err != nil {
		logger.Fatal("failed to initialize storage", zap.Error(err))
	}

	// Set DynamoDB client for notes package
	notes.SetDynamoClient(dynamoClient)

	// WebSocket endpoint for broadcasting updates
	wsEndpoint = getEnv("WEBSOCKET_ENDPOINT", "")
	if wsEndpoint != "" {
		apiGatewayClient = apigatewaymanagementapi.NewFromConfig(cfg, func(o *apigatewaymanagementapi.Options) {
			o.BaseEndpoint = &wsEndpoint
		})
	}
}

func handleNoteStream(ctx context.Context, event events.DynamoDBEvent) error {
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
			if err := processNewNoteByID(ctx, noteID); err != nil {
				logger.Error("failed to process note",
					zap.String("note_id", noteID),
					zap.Error(err))
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
			if err := recalculateNoteScore(ctx, noteID); err != nil {
				logger.Error("failed to recalculate note score",
					zap.String("note_id", noteID),
					zap.Error(err))
			}
		}
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

func processNewNoteByID(ctx context.Context, noteID string) error {
	// Get the note from DynamoDB
	note, err := notes.GetNote(ctx, noteID)
	if err != nil {
		return fmt.Errorf("failed to get note: %w", err)
	}

	logger.Info("processing new note",
		zap.String("note_id", note.ID),
		zap.String("object_id", note.ObjectID))

	// 1. AI Analysis
	analysis, err := analyzeContent(ctx, note)
	if err != nil {
		logger.Warn("failed to analyze content", zap.Error(err))
		// Continue with default values
		analysis = &notes.Analysis{
			Sentiment:   0.5,
			Objectivity: 0.5,
			HasPII:      false,
		}
	}

	// 2. Source verification
	sourceQuality := verifySources(ctx, note.Sources)

	// 3. Initial scoring
	note.Sentiment = analysis.Sentiment
	note.Objectivity = analysis.Objectivity
	note.SourceQuality = sourceQuality

	// Calculate initial score
	initialScore := calculateInitialScore(note)
	note.Score = initialScore

	// 4. Update note with analysis
	if err := notes.UpdateNoteAnalysis(ctx, note.ID, analysis, sourceQuality); err != nil {
		return fmt.Errorf("failed to update note analysis: %w", err)
	}

	// 5. Check visibility and update status
	status := notes.DetermineVisibilityStatus(initialScore)
	if err := notes.UpdateNoteScore(ctx, note.ID, initialScore, status); err != nil {
		return fmt.Errorf("failed to update note score: %w", err)
	}

	// 6. If visible, broadcast to WebSocket subscribers
	if status == notes.VisibilityVisible || status == notes.VisibilityProminent {
		broadcastNoteUpdate(ctx, note)
	}

	// 7. Check if should federate
	if initialScore >= notes.FederationThreshold {
		// Queue for federation by creating activity in outbox
		now := time.Now()
		activity := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{
				ID:        fmt.Sprintf("%s/activities/%s", getDomainURL(), generateID()),
				Type:      activitypub.CreateType,
				Published: &now,
				To:        []string{"https://www.w3.org/ns/activitystreams#Public"},
				CC:        []string{},
			},
			Actor:  note.AuthorID,
			Object: note,
		}

		if err := store.CreateActivity(ctx, activity); err != nil {
			logger.Error("failed to queue note for federation",
				zap.String("note_id", note.ID),
				zap.Error(err))
		} else {
			logger.Info("note queued for federation",
				zap.String("note_id", note.ID),
				zap.String("activity_id", activity.ID),
				zap.Float64("score", initialScore))
		}
	}

	return nil
}

func analyzeContent(ctx context.Context, note *notes.CommunityNote) (*notes.Analysis, error) {
	// Use AWS Comprehend for analysis

	// Detect sentiment
	languageCode := convertToComprehendLanguageCode(note.Language)
	sentimentResp, err := comprehendClient.DetectSentiment(ctx, &comprehend.DetectSentimentInput{
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
	piiResp, err := comprehendClient.DetectPiiEntities(ctx, &comprehend.DetectPiiEntitiesInput{
		Text:         &note.Content,
		LanguageCode: types.LanguageCodeEn,
	})
	hasPII := err == nil && len(piiResp.Entities) > 0

	// Calculate objectivity based on sentiment and content
	objectivity := calculateObjectivity(sentimentResp)

	return &notes.Analysis{
		Sentiment:   sentimentScore,
		Objectivity: objectivity,
		HasPII:      hasPII,
		Language:    string(types.LanguageCodeEn),
	}, nil
}

func calculateObjectivity(sentiment *comprehend.DetectSentimentOutput) float64 {
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

func verifySources(ctx context.Context, sources []notes.Source) float64 {
	if len(sources) == 0 {
		return 0.3 // Low quality without sources
	}

	var totalQuality float64
	for _, source := range sources {
		quality := evaluateSourceDomain(source.Domain)
		totalQuality += quality
	}

	return totalQuality / float64(len(sources))
}

func evaluateSourceDomain(domain string) float64 {
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

func calculateInitialScore(note *notes.CommunityNote) float64 {
	// Author reputation component (normalized to 0-1)
	authorScore := note.AuthorRep / 1000.0
	if authorScore > 1 {
		authorScore = 1
	}

	// AI analysis component
	aiScore := (note.Sentiment + note.Objectivity + note.SourceQuality) / 3.0

	// Initial score weights author reputation more heavily
	return authorScore*0.6 + aiScore*0.4
}

func recalculateNoteScore(ctx context.Context, noteID string) error {
	// Get the note
	note, err := notes.GetNote(ctx, noteID)
	if err != nil {
		return fmt.Errorf("failed to get note: %w", err)
	}

	// Get all votes
	votes, err := notes.GetVotesForNote(ctx, noteID)
	if err != nil {
		return fmt.Errorf("failed to get votes: %w", err)
	}

	// Calculate new score
	newScore := notes.CalculateNoteScore(note, votes)
	newStatus := notes.DetermineVisibilityStatus(newScore)

	// Update the note
	if err := notes.UpdateNoteScore(ctx, noteID, newScore, newStatus); err != nil {
		return fmt.Errorf("failed to update note score: %w", err)
	}

	// Update vote counts
	var helpfulVotes, notHelpfulVotes int
	for _, vote := range votes {
		switch vote.VoteType {
		case notes.VoteHelpful:
			helpfulVotes++
		case notes.VoteNotHelpful:
			notHelpfulVotes++
		}
	}

	// Get updated note for broadcasting
	note.Score = newScore
	note.VisibilityStatus = newStatus
	note.HelpfulVotes = helpfulVotes
	note.NotHelpfulVotes = notHelpfulVotes

	// Broadcast update
	broadcastNoteUpdate(ctx, note)

	logger.Info("recalculated note score",
		zap.String("note_id", noteID),
		zap.Float64("new_score", newScore),
		zap.String("visibility", string(newStatus)),
		zap.Int("helpful_votes", helpfulVotes),
		zap.Int("not_helpful_votes", notHelpfulVotes))

	return nil
}

func broadcastNoteUpdate(ctx context.Context, note *notes.CommunityNote) {
	if apiGatewayClient == nil {
		return
	}

	// Create update message
	message := map[string]any{
		"type": "note.update",
		"payload": map[string]any{
			"object_id": note.ObjectID,
			"note":      note,
			"action":    determineAction(note),
		},
	}

	_, err := json.Marshal(message)
	if err != nil {
		logger.Error("failed to marshal message", zap.Error(err))
		return
	}

	// Get subscribers for this object
	// In a real implementation, this would query a subscription table
	// For now, we'll skip the actual broadcast
	logger.Info("would broadcast note update",
		zap.String("note_id", note.ID),
		zap.String("object_id", note.ObjectID),
		zap.String("action", determineAction(note)))
}

func determineAction(note *notes.CommunityNote) string {
	switch note.VisibilityStatus {
	case notes.VisibilityVisible, notes.VisibilityProminent:
		return "show"
	case notes.VisibilityHidden:
		return "hide"
	case notes.VisibilityDisputed:
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

func generateID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// Fallback to timestamp-based ID if crypto/rand fails
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}

func getDomainURL() string {
	domain := getEnv("DOMAIN_NAME", "localhost:8080")
	if strings.HasPrefix(domain, "http") {
		return domain
	}
	return "https://" + domain
}

func convertToComprehendLanguageCode(language string) types.LanguageCode {
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

func main() {
	lambda.Start(handleNoteStream)
}

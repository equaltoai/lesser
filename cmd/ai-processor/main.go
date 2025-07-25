package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/aron23/lesser/pkg/ai"
	"github.com/aron23/lesser/pkg/moderation"
	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

var (
	dynamoClient *dynamodb.Client
	aiService    *ai.AIService
	tableName    string
)

func init() {
	cfg, err := config.LoadDefaultConfig(context.Background())
	if err != nil {
		log.Fatal("unable to load SDK config:", err)
	}

	dynamoClient = dynamodb.NewFromConfig(cfg)

	tableName = os.Getenv("DYNAMO_TABLE_NAME")
	if tableName == "" {
		tableName = "lesser-main"
	}

	// Initialize services
	aiConfig := &ai.AIConfig{
		NSFWThreshold:       0.8,
		ToxicityThreshold:   0.7,
		SpamThreshold:       0.75,
		AIContentThreshold:  0.85,
		EnablePIIDetection:  true,
		EnableAIDetection:   true,
		EnableImageAnalysis: true,
		BedrockModelID:      "anthropic.claude-v2",
		S3Bucket:            os.Getenv("S3_BUCKET_NAME"),
	}
	aiService = ai.NewAIService(cfg, aiConfig)
}

func handler(ctx context.Context, event events.DynamoDBEvent) error {
	for _, record := range event.Records {
		if err := processRecord(ctx, record); err != nil {
			log.Printf("error processing record: %v", err)
			// Continue processing other records
		}
	}
	return nil
}

func processRecord(ctx context.Context, record events.DynamoDBEventRecord) error {
	if record.EventName != "INSERT" && record.EventName != "MODIFY" {
		return nil
	}

	// Extract object information
	var objectID, objectType, contentText, actorID string
	var mediaURLs []string

	// Simple extraction for MVP
	if pk, ok := record.Change.NewImage["PK"]; ok {
		pkStr := pk.String()
		if len(pkStr) > 7 && pkStr[:7] == "OBJECT#" {
			objectID = pkStr[7:]
		} else {
			return nil // Not an object
		}
	}

	if typeAttr, ok := record.Change.NewImage["Type"]; ok {
		objectType = typeAttr.String()
	}

	if contentAttr, ok := record.Change.NewImage["Content"]; ok {
		contentText = contentAttr.String()
	}

	if actorAttr, ok := record.Change.NewImage["ActorID"]; ok {
		actorID = actorAttr.String()
	}

	// Skip if not an analyzable type
	if !isAnalyzableType(objectType) {
		return nil
	}

	// Create Content struct for analysis
	content := &ai.Content{
		ID:        objectID,
		Type:      objectType,
		Text:      contentText,
		MediaURLs: mediaURLs,
		AuthorID:  actorID,
		CreatedAt: time.Now(),
	}

	// Perform AI analysis
	analysis, err := aiService.AnalyzeContent(ctx, content)
	if err != nil {
		return fmt.Errorf("failed to analyze content: %w", err)
	}

	// Store analysis result
	aiStorage := ai.NewStorage(dynamoClient, tableName)
	if err := aiStorage.SaveAnalysis(ctx, analysis); err != nil {
		return fmt.Errorf("failed to store analysis: %w", err)
	}

	// Handle moderation action if needed
	if analysis.ModerationAction != ai.ActionNone {
		if err := handleModerationAction(ctx, analysis, actorID); err != nil {
			log.Printf("failed to handle moderation action: %v", err)
		}
	}

	return nil
}

func isAnalyzableType(objectType string) bool {
	switch objectType {
	case "Note", "Article", "Question", "Image", "Video":
		return true
	default:
		return false
	}
}

func handleModerationAction(ctx context.Context, analysis *ai.AIAnalysis, actorID string) error {
	// Create moderation event
	event := &moderation.ModerationEvent{
		ID:              analysis.ID,
		EventType:       moderation.EventTypeFlagged,
		ObjectID:        analysis.ObjectID,
		ObjectType:      analysis.ObjectType,
		ActorID:         "ai-processor",
		Category:        determineCategory(analysis),
		Severity:        determineSeverity(analysis),
		ConfidenceScore: analysis.OverallRisk,
		Evidence: []moderation.Evidence{
			{
				Type:        "ai_analysis",
				Score:       analysis.OverallRisk,
				Description: fmt.Sprintf("AI detected %s", analysis.ModerationAction),
				Metadata: map[string]any{
					"analysis_id": analysis.ID,
					"risk_score":  analysis.OverallRisk,
				},
				Timestamp: time.Now(),
			},
		},
		Created: time.Now(),
		Updated: time.Now(),
	}

	// Store moderation event
	eventData, err := json.Marshal(event)
	if err != nil {
		return err
	}

	_, err = dynamoClient.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(tableName),
		Item: map[string]types.AttributeValue{
			"PK":        &types.AttributeValueMemberS{Value: fmt.Sprintf("MODERATION#%s", event.ObjectID)},
			"SK":        &types.AttributeValueMemberS{Value: fmt.Sprintf("EVENT#%s", event.ID)},
			"Type":      &types.AttributeValueMemberS{Value: "ModerationEvent"},
			"EventData": &types.AttributeValueMemberS{Value: string(eventData)},
			"TTL":       &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", time.Now().Add(30*24*time.Hour).Unix())},
		},
	})

	return err
}

func determineCategory(analysis *ai.AIAnalysis) moderation.Category {
	if analysis.SpamAnalysis != nil && analysis.SpamAnalysis.SpamScore > 0.7 {
		return moderation.CategorySpam
	}
	if analysis.TextAnalysis != nil && analysis.TextAnalysis.ToxicityScore > 0.7 {
		return moderation.CategoryHateSpeech
	}
	if analysis.ImageAnalysis != nil && analysis.ImageAnalysis.IsNSFW {
		return moderation.CategoryNSFW
	}
	if analysis.ImageAnalysis != nil && analysis.ImageAnalysis.ViolenceScore > 0.7 {
		return moderation.CategoryViolence
	}
	return moderation.CategoryOther
}

func determineSeverity(analysis *ai.AIAnalysis) moderation.Severity {
	if analysis.OverallRisk > 0.9 {
		return moderation.SeverityCritical
	}
	if analysis.OverallRisk > 0.7 {
		return moderation.SeverityHigh
	}
	if analysis.OverallRisk > 0.5 {
		return moderation.SeverityMedium
	}
	return moderation.SeverityLow
}

func main() {
	lambda.Start(handler)
}

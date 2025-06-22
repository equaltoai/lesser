package main

import (
	"context"
	"log"
	"strconv"
	"strings"

	"github.com/aron23/lesser/pkg/config"
	"github.com/aron23/lesser/pkg/moderation"
	"github.com/aron23/lesser/pkg/reports"
	"github.com/aron23/lesser/pkg/storage"
	"github.com/aron23/lesser/pkg/storage/dynamodb"
	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"go.uber.org/zap"
)

// Handler processes DynamoDB stream events for moderation decisions
type Handler struct {
	logger          *zap.Logger
	store           storage.Storage
	enhancedReports *reports.EnhancedReportService
}

// HandleDynamoDBStream processes DynamoDB stream events
func (h *Handler) HandleDynamoDBStream(ctx context.Context, event events.DynamoDBEvent) error {
	for _, record := range event.Records {
		if err := h.processRecord(ctx, record); err != nil {
			h.logger.Error("failed to process record",
				zap.String("eventID", record.EventID),
				zap.Error(err))
			// Continue processing other records
		}
	}

	return nil
}

// processRecord processes a single DynamoDB stream record
func (h *Handler) processRecord(ctx context.Context, record events.DynamoDBEventRecord) error {
	// Only process INSERT and MODIFY events
	if record.EventName != "INSERT" && record.EventName != "MODIFY" {
		return nil
	}

	// Check if this is a moderation decision
	pkAttr, pkExists := record.Change.NewImage["PK"]
	skAttr, skExists := record.Change.NewImage["SK"]

	if !pkExists || !skExists {
		return nil
	}

	// Extract PK and SK values
	pk := ""
	sk := ""

	switch pkAttr.DataType() {
	case events.DataTypeString:
		pk = pkAttr.String()
	}

	switch skAttr.DataType() {
	case events.DataTypeString:
		sk = skAttr.String()
	}

	// Look for moderation decisions: PK=MODERATION#objectID, SK=DECISION
	if !strings.HasPrefix(pk, "MODERATION#") || sk != "DECISION" {
		return nil
	}

	// Extract the decision data
	var decision moderation.ModerationDecision

	// Helper functions to extract values from DynamoDB stream events
	getString := func(attr events.DynamoDBAttributeValue) string {
		if attr.DataType() == events.DataTypeString {
			return attr.String()
		}
		return ""
	}

	getNumber := func(attr events.DynamoDBAttributeValue) float64 {
		if attr.DataType() == events.DataTypeNumber {
			val, _ := strconv.ParseFloat(attr.Number(), 64)
			return val
		}
		return 0
	}

	// Extract decision fields
	if idAttr, ok := record.Change.NewImage["ID"]; ok {
		decision.ID = getString(idAttr)
	}
	if eventIDAttr, ok := record.Change.NewImage["EventID"]; ok {
		decision.EventID = getString(eventIDAttr)
	}
	if objectIDAttr, ok := record.Change.NewImage["ObjectID"]; ok {
		decision.ObjectID = getString(objectIDAttr)
	}
	if actionAttr, ok := record.Change.NewImage["Action"]; ok {
		decision.Action = moderation.ActionType(getString(actionAttr))
	}
	if consensusAttr, ok := record.Change.NewImage["ConsensusScore"]; ok {
		decision.ConsensusScore = getNumber(consensusAttr)
	}
	if countAttr, ok := record.Change.NewImage["ReviewerCount"]; ok {
		decision.ReviewerCount = int(getNumber(countAttr))
	}

	// Check if this decision is related to a report
	if decision.EventID == "" {
		return nil
	}

	// Get the moderation event to check if it originated from a report
	event, err := h.store.GetModerationEvent(ctx, decision.EventID)
	if err != nil {
		h.logger.Warn("failed to get moderation event",
			zap.String("eventID", decision.EventID),
			zap.Error(err))
		return nil
	}

	// Check if this event has report metadata
	var reportID string
	for _, evidence := range event.Evidence {
		if evidence.Type == "user_report" {
			if metadata, ok := evidence.Metadata["report_id"].(string); ok {
				reportID = metadata
				break
			}
		}
	}

	if reportID == "" {
		// Not a report-based moderation event
		return nil
	}

	h.logger.Info("processing moderation decision for report",
		zap.String("reportID", reportID),
		zap.String("decision", string(decision.Action)),
		zap.Float64("consensus", decision.ConsensusScore))

	// Update reporter trust based on the decision
	if err := h.enhancedReports.UpdateReporterTrustOnDecision(ctx, reportID, &decision, event.ActorID); err != nil {
		h.logger.Error("failed to update reporter trust",
			zap.String("reportID", reportID),
			zap.Error(err))
		return err
	}

	// Check if we need to send notifications about trust changes
	// This could trigger notifications to the reporter about their trust score changes
	// For now, we'll just log it
	h.logger.Info("successfully updated reporter trust",
		zap.String("reportID", reportID),
		zap.String("reporterActorID", event.ActorID),
		zap.String("decision", string(decision.Action)))

	return nil
}

func main() {
	// Initialize logger
	logger, _ := zap.NewProduction()
	defer func() {
		if err := logger.Sync(); err != nil {
			// Can't use logger here since it might be the source of the error
			log.Printf("Failed to sync logger: %v", err)
		}
	}()

	// Get configuration
	cfg := config.Get()

	// Create storage
	store, err := dynamodb.New()
	if err != nil {
		logger.Fatal("failed to create storage", zap.Error(err))
	}

	// Create enhanced report service
	enhancedReports := reports.NewEnhancedReportService(store, logger)

	// Create handler
	handler := &Handler{
		logger:          logger,
		store:           store,
		enhancedReports: enhancedReports,
	}

	// Log initialization
	logger.Info("report trust updater initialized",
		zap.String("region", cfg.Region),
		zap.String("table", cfg.DynamoTableName))

	// Start Lambda handler
	lambda.Start(handler.HandleDynamoDBStream)
}

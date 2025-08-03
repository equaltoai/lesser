package main

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/lift/patterns"
	"github.com/equaltoai/lesser/pkg/moderation"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/dynamorm"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/trust"
)

// ReportTrustService provides direct trust update operations using DynamORM
type ReportTrustService struct {
	db        core.DB
	tableName string
	logger    *zap.Logger
}

// NewReportTrustService creates a new report trust service
func NewReportTrustService(db core.DB, tableName string, logger *zap.Logger) *ReportTrustService {
	return &ReportTrustService{
		db:        db,
		tableName: tableName,
		logger:    logger,
	}
}

// GetReport retrieves a report by ID
func (r *ReportTrustService) GetReport(ctx context.Context, id string) (*storage.Report, error) {
	var report storage.Report
	err := r.db.WithContext(ctx).Model(&report).
		Where("PK", "=", fmt.Sprintf("REPORT#%s", id)).
		Where("SK", "=", "METADATA").
		First(&report)
	return &report, err
}

// GetModerationEvent retrieves a moderation event by ID
func (r *ReportTrustService) GetModerationEvent(ctx context.Context, eventID string) (*storage.ModerationEvent, error) {
	var event storage.ModerationEvent
	err := r.db.WithContext(ctx).Model(&event).
		Where("PK", "=", fmt.Sprintf("MODERATION#%s", eventID)).
		Where("SK", "=", "EVENT").
		First(&event)
	return &event, err
}

// UpdateReporterTrustOnDecision updates reporter trust based on moderation decision
func (r *ReportTrustService) UpdateReporterTrustOnDecision(ctx context.Context, reportID string, decision *moderation.ModerationDecision, reporterActorID string) error {
	// Get the report to validate it exists
	report, err := r.GetReport(ctx, reportID)
	if err != nil {
		return fmt.Errorf("failed to get report: %w", err)
	}

	// Determine if the report was valid based on the decision
	reportValid := false
	switch decision.Action {
	case moderation.ActionTypeRemove, moderation.ActionTypeSuspend, moderation.ActionTypeSilence, moderation.ActionTypeWarning:
		reportValid = true
	case moderation.ActionTypeNone:  
		reportValid = false
	}

	// Update trust relationship for the reporter using the proper models
	trustRel := &models.TrustRelationship{
		ID:         fmt.Sprintf("system_%s_%s_%d", reporterActorID, trust.TrustCategoryContent, time.Now().Unix()),
		TrusterID:  "system",
		TrusteeID:  reporterActorID,
		Category:   models.TrustCategoryContent,
		Score:      0.5, // Default score, would be calculated based on history
		Confidence: 0.8, // Confidence in this assessment
		Created:    time.Now(),
		Updated:    time.Now(),
		Type:       "RELATIONSHIP",
	}

	// Update the DynamoDB keys
	trustRel.UpdateKeys()

	// Adjust score based on report validity
	if reportValid {
		trustRel.Score += 0.1 // Increase trust for valid reports
		r.logger.Info("increasing reporter trust for valid report",
			zap.String("reporter", reporterActorID),
			zap.String("report_id", reportID),
			zap.String("decision", string(decision.Action)))
	} else {
		trustRel.Score -= 0.1 // Decrease trust for invalid reports
		r.logger.Info("decreasing reporter trust for invalid report",
			zap.String("reporter", reporterActorID),
			zap.String("report_id", reportID),
			zap.String("decision", string(decision.Action)))
	}

	// Ensure score stays within bounds
	if trustRel.Score > 1.0 {
		trustRel.Score = 1.0
	}
	if trustRel.Score < 0.0 {
		trustRel.Score = 0.0
	}

	// Store the trust relationship using DynamORM
	err = r.db.WithContext(ctx).Model(trustRel).Create()
	if err != nil {
		r.logger.Error("failed to create trust relationship", zap.Error(err))
		return err
	}

	r.logger.Info("successfully updated reporter trust",
		zap.String("reporter", reporterActorID),
		zap.String("report_id", reportID),
		zap.Float64("new_score", trustRel.Score),
		zap.Bool("report_valid", reportValid))

	// Log that we would update the report status (this would normally update the report record)
	r.logger.Info("would update report status to resolved",
		zap.String("report_id", reportID),
		zap.String("action", string(decision.Action)))

	_ = report // Use the report variable to avoid unused variable warning

	return nil
}

// ReportTrustUpdater processes DynamoDB stream events for moderation decisions
type ReportTrustUpdater struct {
	logger       *zap.Logger
	trustService *ReportTrustService
}

// NewReportTrustUpdater creates a new report trust updater
func NewReportTrustUpdater(db core.DB, tableName string, logger *zap.Logger) *ReportTrustUpdater {
	// Create direct trust service using DynamORM
	trustService := NewReportTrustService(db, tableName, logger)
	
	return &ReportTrustUpdater{
		logger:       logger,
		trustService: trustService,
	}
}

// HandleStream processes DynamoDB stream events using Lift patterns
func (rtu *ReportTrustUpdater) HandleStream(ctx *lift.Context, event events.DynamoDBEvent) error {
	requestID := ctx.GetRequestID()
	
	rtu.logger.Info("processing report trust updater stream batch",
		zap.String("request_id", requestID),
		zap.Int("record_count", len(event.Records)),
	)

	// Process each record, continuing on individual failures
	var processedCount, errorCount int
	
	for _, record := range event.Records {
		if err := rtu.processRecord(ctx.Request.Context(), record); err != nil {
			errorCount++
			rtu.logger.Error("failed to process record",
				zap.String("request_id", requestID),
				zap.String("event_id", record.EventID),
				zap.Error(err))
			// Continue processing other records - don't fail the entire batch
		} else {
			processedCount++
		}
	}

	rtu.logger.Info("completed stream batch processing",
		zap.String("request_id", requestID),
		zap.Int("processed_count", processedCount),
		zap.Int("error_count", errorCount),
		zap.Int("total_count", len(event.Records)),
	)

	return nil
}

// processRecord processes a single DynamoDB stream record
func (rtu *ReportTrustUpdater) processRecord(ctx context.Context, record events.DynamoDBEventRecord) error {
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
	event, err := rtu.trustService.GetModerationEvent(ctx, decision.EventID)
	if err != nil {
		rtu.logger.Warn("failed to get moderation event",
			zap.String("eventID", decision.EventID),
			zap.Error(err))
		return nil
	}

	// Check if this event has report metadata
	var reportID string
	for _, evidence := range event.Evidence {
		// Type assert evidence to map
		if evidenceMap, ok := evidence.(map[string]interface{}); ok {
			if evidenceType, ok := evidenceMap["type"].(string); ok && evidenceType == "user_report" {
				if metadata, ok := evidenceMap["metadata"].(map[string]interface{}); ok {
					if rid, ok := metadata["report_id"].(string); ok {
						reportID = rid
						break
					}
				}
			}
		}
	}

	if reportID == "" {
		// Not a report-based moderation event
		return nil
	}

	rtu.logger.Info("processing moderation decision for report",
		zap.String("reportID", reportID),
		zap.String("decision", string(decision.Action)),
		zap.Float64("consensus", decision.ConsensusScore))

	// Update reporter trust based on the decision
	if err := rtu.trustService.UpdateReporterTrustOnDecision(ctx, reportID, &decision, event.ActorID); err != nil {
		rtu.logger.Error("failed to update reporter trust",
			zap.String("reportID", reportID),
			zap.Error(err))
		return err
	}

	// Check if we need to send notifications about trust changes
	// This could trigger notifications to the reporter about their trust score changes
	// For now, we'll just log it
	rtu.logger.Info("successfully updated reporter trust",
		zap.String("reportID", reportID),
		zap.String("reporterActorID", event.ActorID),
		zap.String("decision", string(decision.Action)))

	return nil
}

func main() {
	// Initialize logger using common patterns
	logger := common.Logger()

	// Get configuration
	cfg := config.Get()

	// Initialize DynamORM with Lambda optimizations
	db, err := dynamorm.NewLambdaOptimizedClient(context.Background(), cfg.Region)
	if err != nil {
		logger.Fatal("failed to initialize DynamORM", zap.Error(err))
	}

	// Create report trust updater with minimal storage implementation
	updater := NewReportTrustUpdater(db, cfg.DynamoTableName, logger)

	// Log initialization
	logger.Info("report trust updater initialized",
		zap.String("region", cfg.Region),
		zap.String("table", cfg.DynamoTableName))

	// Start DynamoDB stream Lambda using Lift patterns
	// This provides structured logging, request IDs, recovery, and error handling
	patterns.StartDynamoDBStreamLambda("report-trust-updater", updater, logger)
}

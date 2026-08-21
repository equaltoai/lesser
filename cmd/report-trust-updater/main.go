// Package main implements the report-trust-updater Lambda function for updating trust scores based on reports.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	apptheory "github.com/theory-cloud/apptheory/v3/runtime"
	"github.com/theory-cloud/tabletheory/v3/pkg/core"
	"go.uber.org/zap"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/deploy/naming"
	pkgErrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/moderation"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/theorydb"
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
		return pkgErrors.WrapError(err, pkgErrors.CodeInternal, pkgErrors.CategoryLambda, "Failed to get report")
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
	if err := trustRel.UpdateKeys(); err != nil {
		r.logger.Error("failed to update trust relationship keys",
			zap.String("trusterID", trustRel.TrusterID),
			zap.String("trusteeID", trustRel.TrusteeID),
			zap.Error(err),
		)
		return err
	}

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
	err = r.db.WithContext(ctx).Model(trustRel).CreateOrUpdate()
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
	trustService reportTrustService
}

type reportTrustService interface {
	GetModerationEvent(ctx context.Context, eventID string) (*storage.ModerationEvent, error)
	UpdateReporterTrustOnDecision(ctx context.Context, reportID string, decision *moderation.ModerationDecision, reporterActorID string) error
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

func (rtu *ReportTrustUpdater) HandleDynamoDBRecord(ctx *apptheory.EventContext, record events.DynamoDBEventRecord) error {
	requestID := ""
	runCtx := context.Background()
	if ctx != nil {
		requestID = ctx.RequestID
		runCtx = ctx.Context()
	}

	if err := rtu.processRecord(runCtx, record); err != nil {
		rtu.logger.Error("failed to process record",
			zap.String("request_id", requestID),
			zap.String("event_id", record.EventID),
			zap.Error(err),
		)
		// Match previous Lift behavior: log and continue; do not fail the batch.
		return nil
	}

	return nil
}

// processRecord processes a single DynamoDB stream record
func (rtu *ReportTrustUpdater) processRecord(ctx context.Context, record events.DynamoDBEventRecord) error {
	// Check if we should process this event
	if !rtu.shouldProcessEvent(record) {
		return nil
	}

	// Extract and validate keys
	pk, sk, err := rtu.extractRecordKeys(record)
	if err != nil {
		return nil
	}

	// Check if this is a moderation decision
	if !rtu.isModerationDecision(pk, sk) {
		return nil
	}

	// Extract decision from record
	decision := rtu.extractModerationDecision(record)

	// Check if this decision is related to a report
	if err := common.ValidateRequiredParam("eventID", decision.EventID); err != nil {
		return nil
	}

	// Process the moderation event
	return rtu.processModerationEvent(ctx, decision)
}

// shouldProcessEvent checks if the event should be processed
func (rtu *ReportTrustUpdater) shouldProcessEvent(record events.DynamoDBEventRecord) bool {
	return record.EventName == "INSERT" || record.EventName == "MODIFY"
}

// extractRecordKeys extracts PK and SK from the record
func (rtu *ReportTrustUpdater) extractRecordKeys(record events.DynamoDBEventRecord) (string, string, error) {
	pkAttr, pkExists := record.Change.NewImage["PK"]
	skAttr, skExists := record.Change.NewImage["SK"]

	if !pkExists || !skExists {
		return "", "", pkgErrors.ReportTrustUpdaterMissingKeys()
	}

	pk := rtu.getStringFromAttribute(pkAttr)
	sk := rtu.getStringFromAttribute(skAttr)

	return pk, sk, nil
}

// getStringFromAttribute extracts string value from DynamoDB attribute
func (rtu *ReportTrustUpdater) getStringFromAttribute(attr events.DynamoDBAttributeValue) string {
	if attr.DataType() == events.DataTypeString {
		return attr.String()
	}
	return ""
}

// getNumberFromAttribute extracts number value from DynamoDB attribute
func (rtu *ReportTrustUpdater) getNumberFromAttribute(attr events.DynamoDBAttributeValue) float64 {
	if attr.DataType() == events.DataTypeNumber {
		val, _ := strconv.ParseFloat(attr.Number(), 64)
		return val
	}
	return 0
}

// isModerationDecision checks if the record is a moderation decision
func (rtu *ReportTrustUpdater) isModerationDecision(pk, sk string) bool {
	return strings.HasPrefix(pk, "MODERATION#") && sk == "DECISION"
}

// extractModerationDecision extracts decision data from the record
func (rtu *ReportTrustUpdater) extractModerationDecision(record events.DynamoDBEventRecord) moderation.ModerationDecision {
	var decision moderation.ModerationDecision

	// Extract decision fields
	if idAttr, ok := record.Change.NewImage["ID"]; ok {
		decision.ID = rtu.getStringFromAttribute(idAttr)
	}
	if eventIDAttr, ok := record.Change.NewImage["EventID"]; ok {
		decision.EventID = rtu.getStringFromAttribute(eventIDAttr)
	}
	if objectIDAttr, ok := record.Change.NewImage["ObjectID"]; ok {
		decision.ObjectID = rtu.getStringFromAttribute(objectIDAttr)
	}
	if actionAttr, ok := record.Change.NewImage["Action"]; ok {
		decision.Action = moderation.ActionType(rtu.getStringFromAttribute(actionAttr))
	}
	if consensusAttr, ok := record.Change.NewImage["ConsensusScore"]; ok {
		decision.ConsensusScore = rtu.getNumberFromAttribute(consensusAttr)
	}
	if countAttr, ok := record.Change.NewImage["ReviewerCount"]; ok {
		decision.ReviewerCount = int(rtu.getNumberFromAttribute(countAttr))
	}

	return decision
}

// processModerationEvent processes the moderation event and updates trust
func (rtu *ReportTrustUpdater) processModerationEvent(ctx context.Context, decision moderation.ModerationDecision) error {
	// Get the moderation event to check if it originated from a report
	event, err := rtu.trustService.GetModerationEvent(ctx, decision.EventID)
	if err != nil {
		rtu.logger.Warn("failed to get moderation event",
			zap.String("eventID", decision.EventID),
			zap.Error(err))
		return nil
	}

	// Extract report ID from event evidence
	reportID := rtu.extractReportIDFromEvent(event)
	if err := common.ValidateRequiredParam("reportID", reportID); err != nil {
		// Not a report-based moderation event
		return nil
	}

	// Log processing
	rtu.logModerationProcessing(reportID, decision)

	// Update reporter trust
	if err := rtu.updateReporterTrust(ctx, reportID, decision, event.ActorID); err != nil {
		return err
	}

	// Log success
	rtu.logTrustUpdateSuccess(reportID, event.ActorID, decision)

	return nil
}

// extractReportIDFromEvent extracts report ID from moderation event evidence
func (rtu *ReportTrustUpdater) extractReportIDFromEvent(event *storage.ModerationEvent) string {
	for _, evidence := range event.Evidence {
		reportID := rtu.extractReportIDFromEvidence(evidence)
		if reportID != "" {
			return reportID
		}
	}
	return ""
}

// extractReportIDFromEvidence extracts report ID from a single evidence item
func (rtu *ReportTrustUpdater) extractReportIDFromEvidence(evidence interface{}) string {
	evidenceMap, ok := evidence.(map[string]interface{})
	if !ok {
		return ""
	}

	evidenceType, ok := evidenceMap["type"].(string)
	if !ok || evidenceType != "user_report" {
		return ""
	}

	metadata, ok := evidenceMap["metadata"].(map[string]interface{})
	if !ok {
		return ""
	}

	reportID, ok := metadata["report_id"].(string)
	if !ok {
		return ""
	}

	return reportID
}

// logModerationProcessing logs the start of moderation processing
func (rtu *ReportTrustUpdater) logModerationProcessing(reportID string, decision moderation.ModerationDecision) {
	rtu.logger.Info("processing moderation decision for report",
		zap.String("reportID", reportID),
		zap.String("decision", string(decision.Action)),
		zap.Float64("consensus", decision.ConsensusScore))
}

// updateReporterTrust updates the reporter's trust score
func (rtu *ReportTrustUpdater) updateReporterTrust(ctx context.Context, reportID string, decision moderation.ModerationDecision, actorID string) error {
	if err := rtu.trustService.UpdateReporterTrustOnDecision(ctx, reportID, &decision, actorID); err != nil {
		rtu.logger.Error("failed to update reporter trust",
			zap.String("reportID", reportID),
			zap.Error(err))
		return err
	}
	return nil
}

// logTrustUpdateSuccess logs successful trust update
func (rtu *ReportTrustUpdater) logTrustUpdateSuccess(reportID, actorID string, decision moderation.ModerationDecision) {
	rtu.logger.Info("successfully updated reporter trust",
		zap.String("reportID", reportID),
		zap.String("reporterActorID", actorID),
		zap.String("decision", string(decision.Action)))
}

func main() {
	runReportTrustUpdater()
}

var (
	mustInitializeLambdaFn  = common.MustInitializeLambda
	dynamormGetClientFn     = theorydb.GetClient
	lambdaStartFn           = lambda.Start
	newReportTrustUpdaterFn = func(db core.DB, tableName string, logger *zap.Logger) *ReportTrustUpdater {
		return NewReportTrustUpdater(db, tableName, logger)
	}
)

func runReportTrustUpdater() {
	// Use standardized Lambda initialization
	lambdaCtx := mustInitializeLambdaFn(common.LambdaConfig{
		ServiceName:        "report-trust-updater",
		LambdaType:         common.LambdaTypeProcessor,
		Version:            "1.0.0",
		EnableMetrics:      true,
		EnableHealthCheck:  false,
		EnableTracing:      true,
		EnableCostTracking: true,
	})

	// Initialize storage independently to avoid import cycles
	db, err := dynamormGetClientFn(context.Background())
	if err != nil {
		lambdaCtx.Logger.Fatal("failed to initialize DynamORM database", zap.Error(err))
	}

	// Set storage in lambdaCtx for reference
	lambdaCtx.DynamoDB = db

	// Create report trust updater with minimal storage implementation
	updater := newReportTrustUpdaterFn(db, lambdaCtx.Config.DynamoTableName, lambdaCtx.Logger)

	// Log initialization
	lambdaCtx.Logger.Info("report trust updater initialized",
		zap.String("region", lambdaCtx.Config.Region),
		zap.String("table", lambdaCtx.Config.DynamoTableName))

	app := apptheory.New()

	appName := strings.TrimSpace(os.Getenv("APP_NAME"))
	stage := strings.TrimSpace(os.Getenv("STAGE"))
	tableName := naming.ResourceNameWithApp(appName, "main-table", stage)

	app.DynamoDB(tableName, updater.HandleDynamoDBRecord)

	lambdaStartFn(func(ctx context.Context, event json.RawMessage) (any, error) {
		return app.HandleLambda(ctx, event)
	})
}

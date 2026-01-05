// Package main implements the severance-processor Lambda function for detecting federation severances.
package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/services"
	severanceService "github.com/equaltoai/lesser/pkg/services/severance"
	storageCore "github.com/equaltoai/lesser/pkg/storage/core"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/lift/pkg/lift"
	liftMiddleware "github.com/pay-theory/lift/pkg/middleware"
	"go.uber.org/zap"
)

var (
	lambdaCtx *common.LambdaContext
	cfg       *config.Config
	logger    *zap.Logger
	repos     storageCore.RepositoryStorage
	registry  *services.Registry
	processor *SeveranceProcessor

	mustInitializeLambdaFn   = common.MustInitializeLambda
	initializeWithDefaultsFn = (*common.LambdaContext).InitializeWithDefaults
	newRegistryFn            = services.NewRegistry
	lambdaStartFn            = lambda.Start
)

func init() {
	if common.RunningUnitTests() {
		return
	}

	if err := initializeSeveranceProcessor(); err != nil {
		logger.Fatal("failed to initialize severance processor", zap.Error(err))
	}
}

func initializeSeveranceProcessor() error {
	// Standardized Lambda initialization for processor functions
	lambdaCtx = mustInitializeLambdaFn(common.LambdaConfig{
		ServiceName: "severance-processor",
		LambdaType:  common.LambdaTypeProcessor,
	})

	// Automatic dependency injection
	cfg = lambdaCtx.Config
	logger = lambdaCtx.Logger
	if lambdaCtx.Repos != nil {
		repos = lambdaCtx.Repos.(storageCore.RepositoryStorage)
	}

	// Initialize with processor-specific defaults
	if err := initializeWithDefaultsFn(lambdaCtx); err != nil {
		logger.Warn("failed to initialize with defaults", zap.Error(err))
	}

	// Create service registry
	var regErr error
	registry, regErr = newRegistryFn(
		services.WithStorage(repos),
		services.WithLogger(logger),
		services.WithConfig(&services.ServiceConfig{
			BaseURL: cfg.Domain,
		}),
	)
	if regErr != nil {
		return regErr
	}

	// Initialize processor
	processor = &SeveranceProcessor{
		registry: servicesRegistryAdapter{Registry: registry},
		logger:   logger,
	}

	return nil
}

// SeveranceProcessor handles severance detection from DynamoDB streams
type SeveranceProcessor struct {
	registry severanceRegistry
	logger   *zap.Logger
}

type severanceRegistry interface {
	Severance() severanceDetector
	GetStorage() storageCore.RepositoryStorage
}

type servicesRegistryAdapter struct {
	*services.Registry
}

func (s servicesRegistryAdapter) Severance() severanceDetector {
	return s.Registry.Severance()
}

func (s servicesRegistryAdapter) GetStorage() storageCore.RepositoryStorage {
	return s.Registry.GetStorage()
}

type severanceDetector interface {
	DetectSeverance(ctx context.Context, remoteInstance string, reason models.SeveranceReason, affectedFollowers, affectedFollowing int, details string) (*severanceService.SeveredRelationship, error)
}

// HandleDynamoDBStreamEvent processes DynamoDB stream events for severance detection
func (p *SeveranceProcessor) HandleDynamoDBStreamEvent(ctx context.Context, event events.DynamoDBEvent) error {
	p.logger.Info("processing severance detection event",
		zap.Int("record_count", len(event.Records)))

	for _, record := range event.Records {
		if err := p.processRecord(ctx, record); err != nil {
			p.logger.Error("failed to process record",
				zap.String("event_id", record.EventID),
				zap.Error(err))
			// Continue processing other records
		}
	}

	return nil
}

// processRecord processes a single DynamoDB stream record
func (p *SeveranceProcessor) processRecord(ctx context.Context, record events.DynamoDBEventRecord) error {
	// Only process INSERT and MODIFY events
	if record.EventName != "INSERT" && record.EventName != "MODIFY" {
		return nil
	}

	newImage := record.Change.NewImage
	if newImage == nil {
		return nil
	}

	// Extract PK and SK values safely
	pkValue := getStringValue(newImage, "PK")
	skValue := getStringValue(newImage, "SK")

	if pkValue == "" || skValue == "" {
		return nil
	}

	// Detect severance from domain blocks
	if strings.HasPrefix(pkValue, "DOMAIN_BLOCK#") {
		return p.handleDomainBlock(ctx, newImage)
	}

	// Detect severance from federation health issues
	if strings.HasPrefix(pkValue, "FEDERATION_ISSUE#") {
		return p.handleFederationIssue(ctx, newImage)
	}

	// Detect severance from federation metrics
	if strings.HasPrefix(pkValue, "FEDERATION_METRICS#") && strings.Contains(skValue, "HEALTH") {
		return p.handleFederationHealth(ctx, newImage)
	}

	return nil
}

// getStringValue safely extracts a string value from a DynamoDB attribute
func getStringValue(image map[string]events.DynamoDBAttributeValue, key string) string {
	if val, ok := image[key]; ok {
		return val.String()
	}
	return ""
}

// getBoolValue safely extracts a boolean value from a DynamoDB attribute
func getBoolValue(image map[string]events.DynamoDBAttributeValue, key string, defaultVal bool) bool {
	if val, ok := image[key]; ok {
		return val.Boolean()
	}
	return defaultVal
}

// handleDomainBlock handles domain block events
func (p *SeveranceProcessor) handleDomainBlock(ctx context.Context, image map[string]events.DynamoDBAttributeValue) error {
	domain := getStringValue(image, "Domain")
	if domain == "" {
		return nil
	}

	p.logger.Info("detected domain block",
		zap.String("domain", domain))

	// Count affected relationships
	affectedFollowers, affectedFollowing := p.countAffectedRelationships(ctx, domain)

	if affectedFollowers == 0 && affectedFollowing == 0 {
		p.logger.Debug("no affected relationships for domain block",
			zap.String("domain", domain))
		return nil
	}

	// Create severance record
	severanceService := p.registry.Severance()
	if severanceService == nil {
		return fmt.Errorf("severance service not available")
	}

	_, err := severanceService.DetectSeverance(
		ctx,
		domain,
		models.SeveranceReasonDomainBlock,
		affectedFollowers,
		affectedFollowing,
		fmt.Sprintf("Domain %s was blocked", domain),
	)
	if err != nil {
		p.logger.Error("failed to create severance record",
			zap.String("domain", domain),
			zap.Error(err))
		return err
	}

	return nil
}

// handleFederationIssue handles federation issue events
func (p *SeveranceProcessor) handleFederationIssue(ctx context.Context, image map[string]events.DynamoDBAttributeValue) error {
	domain := getStringValue(image, "Domain")
	issueType := getStringValue(image, "IssueType")
	severity := getStringValue(image, "Severity")

	if domain == "" || issueType == "" {
		return nil
	}

	// Only create severance for critical or high severity issues that indicate downtime
	if severity != "critical" && severity != "high" {
		return nil
	}

	if issueType != "unreachable" && issueType != "timeout" {
		return nil
	}

	p.logger.Info("detected federation issue",
		zap.String("domain", domain),
		zap.String("issue_type", issueType),
		zap.String("severity", severity))

	// Count affected relationships
	affectedFollowers, affectedFollowing := p.countAffectedRelationships(ctx, domain)

	if affectedFollowers == 0 && affectedFollowing == 0 {
		p.logger.Debug("no affected relationships for federation issue",
			zap.String("domain", domain))
		return nil
	}

	// Create severance record
	severanceService := p.registry.Severance()
	if severanceService == nil {
		return fmt.Errorf("severance service not available")
	}

	_, err := severanceService.DetectSeverance(
		ctx,
		domain,
		models.SeveranceReasonInstanceDown,
		affectedFollowers,
		affectedFollowing,
		fmt.Sprintf("Instance %s is experiencing %s issues", domain, issueType),
	)
	if err != nil {
		p.logger.Error("failed to create severance record",
			zap.String("domain", domain),
			zap.Error(err))
		return err
	}

	return nil
}

// handleFederationHealth handles federation health metric events
func (p *SeveranceProcessor) handleFederationHealth(ctx context.Context, image map[string]events.DynamoDBAttributeValue) error {
	domain := getStringValue(image, "Domain")
	isHealthy := getBoolValue(image, "IsHealthy", true)

	if domain == "" || isHealthy {
		return nil
	}

	p.logger.Info("detected unhealthy federation instance",
		zap.String("domain", domain))

	// Count affected relationships
	affectedFollowers, affectedFollowing := p.countAffectedRelationships(ctx, domain)

	if affectedFollowers == 0 && affectedFollowing == 0 {
		p.logger.Debug("no affected relationships for unhealthy instance",
			zap.String("domain", domain))
		return nil
	}

	// Create severance record
	severanceService := p.registry.Severance()
	if severanceService == nil {
		return fmt.Errorf("severance service not available")
	}

	_, err := severanceService.DetectSeverance(
		ctx,
		domain,
		models.SeveranceReasonInstanceDown,
		affectedFollowers,
		affectedFollowing,
		fmt.Sprintf("Instance %s is unhealthy", domain),
	)
	if err != nil {
		p.logger.Error("failed to create severance record",
			zap.String("domain", domain),
			zap.Error(err))
		return err
	}

	return nil
}

// countAffectedRelationships counts the number of affected follower/following relationships for a domain
func (p *SeveranceProcessor) countAffectedRelationships(ctx context.Context, domain string) (followers, following int) {
	storage := p.registry.GetStorage()
	if storage == nil {
		p.logger.Warn("storage not available for counting relationships")
		return 0, 0
	}

	relationshipRepo := storage.Relationship()
	if relationshipRepo == nil {
		p.logger.Warn("relationship repository not available for counting")
		return 0, 0
	}

	// Use the real repository method to count relationships by domain
	followerCount, followingCount, err := relationshipRepo.CountRelationshipsByDomain(ctx, domain)
	if err != nil {
		p.logger.Error("failed to count relationships by domain",
			zap.String("domain", domain),
			zap.Error(err))
		return 0, 0
	}

	p.logger.Info("counted affected relationships for domain",
		zap.String("domain", domain),
		zap.Int("followers", followerCount),
		zap.Int("following", followingCount))

	return followerCount, followingCount
}

// Handler wraps the processor for Lambda
type Handler struct {
	processor *SeveranceProcessor
}

// HandleDynamoDBStreamEvent handles the Lambda event
func (h *Handler) HandleDynamoDBStreamEvent(ctx context.Context, event events.DynamoDBEvent) error {
	return h.processor.HandleDynamoDBStreamEvent(ctx, event)
}

func main() {
	defer func() {
		if err := logger.Sync(); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to sync logger: %v\n", err)
		}
	}()

	logger.Info("starting severance processor Lambda",
		zap.String("service", "severance-processor"),
		zap.String("lambda_type", "processor"))

	// Create handler
	handler := &Handler{
		processor: processor,
	}

	app := lift.New()
	app.Use(lift.MarkGlobalMiddleware(lift.Middleware(liftMiddleware.RequestID())))
	app.Use(lift.MarkGlobalMiddleware(lift.Middleware(liftMiddleware.Recover())))

	_ = app.DynamoDB("*", func(ctx *lift.Context) error {
		records, err := ctx.DynamoDBRecords()
		if err != nil {
			return err
		}
		return handler.HandleDynamoDBStreamEvent(ctx.Request.Context(), events.DynamoDBEvent{Records: records})
	})

	lambdaStartFn(app.HandleRequest)
}

// Package main implements the severance-processor Lambda function for detecting federation severances.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/deploy/naming"
	"github.com/equaltoai/lesser/pkg/lambdastorage"
	"github.com/equaltoai/lesser/pkg/services"
	severanceService "github.com/equaltoai/lesser/pkg/services/severance"
	storageCore "github.com/equaltoai/lesser/pkg/storage/core"
	"github.com/equaltoai/lesser/pkg/storage/factory"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/theorydb"
	apptheory "github.com/theory-cloud/apptheory/runtime"
	dynamormCore "github.com/theory-cloud/tabletheory/pkg/core"
	"go.uber.org/zap"
)

var (
	lambdaCtx *common.LambdaContext
	cfg       *config.Config
	logger    *zap.Logger
	repos     storageCore.RepositoryStorage
	registry  *services.Registry
	processor *SeveranceProcessor

	mustInitializeLambdaFn     = common.MustInitializeLambda
	newLambdaOptimizedClientFn = theorydb.NewLambdaOptimizedClient
	newRepositoryFactoryFn     = func(db dynamormCore.DB, tableName string, logger *zap.Logger) (storageCore.RepositoryStorage, error) {
		return factory.NewRepositoryFactory(db, tableName, logger)
	}
	newRegistryFn = services.NewRegistry
	lambdaStartFn = lambda.Start
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

	var err error
	repos, err = initializeSeveranceStorage(lambdaCtx)
	if err != nil {
		return err
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

func initializeSeveranceStorage(lambdaCtx *common.LambdaContext) (storageCore.RepositoryStorage, error) {
	deps, err := lambdastorage.Initialize(context.Background(), lambdaCtx, lambdastorage.Options{
		ServiceName:          "severance-processor",
		RequireRepositories:  true,
		NewDB:                newLambdaOptimizedClientFn,
		NewRepositoryStorage: newRepositoryFactoryFn,
	})
	if err != nil {
		return nil, err
	}
	return deps.Repos, nil
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

func (p *SeveranceProcessor) HandleDynamoDBRecord(ctx *apptheory.EventContext, record events.DynamoDBEventRecord) error {
	if p == nil {
		return fmt.Errorf("severance processor is nil")
	}
	if p.logger == nil {
		p.logger = zap.NewNop()
	}

	runCtx := context.Background()
	if ctx != nil {
		runCtx = ctx.Context()
	}

	if err := p.processRecord(runCtx, record); err != nil {
		p.logger.Error("failed to process record",
			zap.String("event_id", record.EventID),
			zap.Error(err),
		)
		// Match previous Lift behavior: log and continue; do not fail the batch.
		return nil
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

func handleSeveranceProcessorStreamRecord(ctx *apptheory.EventContext, record events.DynamoDBEventRecord) error {
	if processor == nil {
		return fmt.Errorf("severance processor not initialized")
	}
	return processor.HandleDynamoDBRecord(ctx, record)
}

func main() {
	if logger == nil {
		logger = zap.NewNop()
	}

	defer func() {
		if err := logger.Sync(); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to sync logger: %v\n", err)
		}
	}()

	logger.Info("starting severance processor Lambda",
		zap.String("service", "severance-processor"),
		zap.String("lambda_type", "processor"))

	app := apptheory.New()

	appName := strings.TrimSpace(os.Getenv("APP_NAME"))
	stage := strings.TrimSpace(os.Getenv("STAGE"))
	tableName := naming.ResourceNameWithApp(appName, "main-table", stage)

	app.DynamoDB(tableName, handleSeveranceProcessorStreamRecord)

	lambdaStartFn(func(ctx context.Context, event json.RawMessage) (any, error) {
		return app.HandleLambda(ctx, event)
	})
}

// Package main implements the federation-tracker Lambda function for tracking federation relationships and activity.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/deploy/naming"
	"github.com/equaltoai/lesser/pkg/lambdastorage"
	"github.com/equaltoai/lesser/pkg/storage/core"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/equaltoai/lesser/pkg/storage/theorydb"
	apptheory "github.com/theory-cloud/apptheory/v3/runtime"
	dynamormCore "github.com/theory-cloud/tabletheory/v3/pkg/core"
	"go.uber.org/zap"
)

var (
	lambdaCtx                    *common.LambdaContext
	cfg                          *config.Config
	logger                       *zap.Logger
	repos                        core.RepositoryStorage //nolint:unused // dependency injection pattern - available for processor extensions
	federationActivityRepository federationActivityStore
	db                           dynamormCore.DB
)

type federationActivityStore interface {
	Create(ctx context.Context, activity *models.FederationActivity) error
	UpdateInstanceInfo(ctx context.Context, info *models.InstanceInfo) error
}

// FederationTracker implements the DynamoDBStreamHandler interface
type FederationTracker struct {
	logger                       *zap.Logger
	cfg                          *common.LambdaContext
	federationActivityRepository federationActivityStore
}

var (
	mustInitializeLambdaFn      = common.MustInitializeLambda
	newLambdaOptimizedClientFn  = theorydb.NewLambdaOptimizedClient
	newFederationActivityRepoFn = func(db dynamormCore.DB, tableName string, logger *zap.Logger) federationActivityStore {
		return repositories.NewFederationActivityRepository(db, tableName, logger, nil)
	}
	lambdaStartFn = lambda.Start
)

func init() {
	if common.RunningUnitTests() {
		return
	}
	initializeFederationTracker()
}

func initializeFederationTracker() {
	// Standardized Lambda initialization for federation-tracker function
	lambdaCtx = mustInitializeLambdaFn(common.LambdaConfig{
		ServiceName: "federation-tracker",       // federation-tracker
		LambdaType:  common.LambdaTypeProcessor, // These are background processing functions
	})

	// Automatic dependency injection
	cfg = lambdaCtx.Config
	logger = lambdaCtx.Logger
	if storage, ok := lambdaCtx.Repos.(core.RepositoryStorage); ok {
		repos = storage
	}

	deps, err := lambdastorage.Initialize(context.Background(), lambdaCtx, lambdastorage.Options{
		ServiceName: "federation-tracker",
		NewDB:       newLambdaOptimizedClientFn,
	})
	if err != nil {
		logger.Fatal("failed to initialize storage", zap.Error(err))
	}

	// Function-specific initialization only
	// Initialize DynamORM with Lambda optimizations
	db = deps.DB

	// Initialize repository
	federationActivityRepository = newFederationActivityRepoFn(db, cfg.DynamoTableName, logger)
}

func main() {
	runFederationTracker()
}

func runFederationTracker() {
	// Create the handler
	handler := &FederationTracker{
		logger:                       logger,
		cfg:                          lambdaCtx,
		federationActivityRepository: federationActivityRepository,
	}

	app := apptheory.New()

	appName := strings.TrimSpace(os.Getenv("APP_NAME"))
	stage := strings.TrimSpace(os.Getenv("STAGE"))
	tableName := naming.ResourceNameWithApp(appName, "main-table", stage)

	app.DynamoDB(tableName, handler.HandleDynamoDBRecord)

	lambdaStartFn(func(ctx context.Context, event json.RawMessage) (any, error) {
		return app.HandleLambda(ctx, event)
	})
}

func (ft *FederationTracker) HandleDynamoDBRecord(ctx *apptheory.EventContext, record events.DynamoDBEventRecord) (err error) {
	requestID := ""
	runCtx := context.Background()
	if ctx != nil {
		requestID = ctx.RequestID
		runCtx = ctx.Context()
	}

	if ft.logger == nil {
		ft.logger = zap.NewNop()
	}

	defer func() {
		if r := recover(); r != nil {
			ft.logger.Error("panic processing federation stream record",
				zap.String("request_id", requestID),
				zap.String("event_id", record.EventID),
				zap.Any("panic", r),
			)
			err = fmt.Errorf("panic recovered: %v", r)
		}
	}()

	if err := ft.processRecord(runCtx, requestID, record); err != nil {
		ft.logger.Error("failed to process record",
			zap.String("request_id", requestID),
			zap.String("event_id", record.EventID),
			zap.Error(err),
		)
		// Preserve prior Lift behavior: log and continue.
		return nil
	}
	return nil
}

func (ft *FederationTracker) processRecord(ctx context.Context, requestID string, record events.DynamoDBEventRecord) error {
	// We're interested in INSERT and MODIFY events that indicate federation activity
	if record.EventName != "INSERT" && record.EventName != "MODIFY" {
		return nil
	}

	// Look for activities from remote instances
	pk, pkExists := record.Change.NewImage["PK"]
	if !pkExists {
		return nil
	}

	// Extract PK string value
	pkStr := ""
	if pk.DataType() == events.DataTypeString {
		pkStr = pk.String()
	}

	if err := common.ValidateRequiredParam("publicKey", pkStr); err != nil {
		return nil
	}

	// Extract SK for debugging
	sk := ""
	if skAttr, exists := record.Change.Keys["SK"]; exists && skAttr.DataType() == events.DataTypeString {
		sk = skAttr.String()
	}

	ft.logger.Debug("Processing record",
		zap.String("request_id", requestID),
		zap.String("pk", pkStr),
		zap.String("sk", sk),
		zap.String("event_name", record.EventName),
	)

	// Handle different types of records
	switch {
	case strings.HasPrefix(pkStr, "ACTIVITY#"):
		// New activity from remote actors
		return ft.trackActivityFromInstance(ctx, requestID, record)

	case strings.HasPrefix(pkStr, "ACTOR#"):
		// New remote actor
		return ft.trackActorFromInstance(ctx, requestID, record)
	}

	return nil
}

func (ft *FederationTracker) trackActivityFromInstance(ctx context.Context, requestID string, record events.DynamoDBEventRecord) error {
	// Extract the activity data
	activityData, ok := record.Change.NewImage["Activity"]
	if !ok || activityData.DataType() != events.DataTypeMap {
		return nil
	}

	// Get the activity map
	activityMap := activityData.Map()

	// Extract actor information
	actorID := ""
	if actor, ok := activityMap["actor"]; ok && actor.DataType() == events.DataTypeString {
		actorID = actor.String()
	}

	if err := common.ValidateRequiredParam("actorID", actorID); err != nil {
		return nil
	}

	// Parse the domain from the actor ID
	domain := extractDomain(actorID)
	if common.ValidateRequiredParam("domain", domain) != nil || domain == ft.cfg.Config.Domain {
		// Local activity, not from a remote instance
		return nil
	}

	// Extract activity type
	activityType := ""
	if typeAttr, ok := activityMap["type"]; ok && typeAttr.DataType() == events.DataTypeString {
		activityType = typeAttr.String()
	}

	// Extract object information
	objectID := ""
	objectType := ""
	if object, ok := activityMap["object"]; ok {
		if object.DataType() == events.DataTypeString {
			objectID = object.String()
		} else if object.DataType() == events.DataTypeMap {
			objMap := object.Map()
			if id, ok := objMap["id"]; ok && id.DataType() == events.DataTypeString {
				objectID = id.String()
			}
			if typeAttr, ok := objMap["type"]; ok && typeAttr.DataType() == events.DataTypeString {
				objectType = typeAttr.String()
			}
		}
	}

	// Create federation activity record
	activity := models.NewFederationActivityBuilder().
		FromDomain(domain).
		WithType(activityType).
		WithActor(actorID).
		WithObject(objectID, objectType).
		Build()

	// Track instance info
	instanceInfo := &models.InstanceInfo{
		Domain:   domain,
		LastSeen: time.Now(),
	}

	// Try to detect software type from activity format
	if _, hasOrderedItems := activityMap["orderedItems"]; hasOrderedItems {
		instanceInfo.Software = "mastodon" // Mastodon-style collections
	} else if _, hasItems := activityMap["items"]; hasItems {
		instanceInfo.Software = "activitypub" // Generic ActivityPub
	}

	activity.InstanceInfo = instanceInfo

	// Save the activity record
	if err := ft.federationActivityRepository.Create(ctx, activity); err != nil {
		ft.logger.Error("failed to create federation activity",
			zap.String("request_id", requestID),
			zap.String("domain", domain),
			zap.String("actor_id", actorID),
			zap.Error(err))
		return err
	}

	// Update instance information
	if err := ft.federationActivityRepository.UpdateInstanceInfo(ctx, instanceInfo); err != nil {
		ft.logger.Warn("failed to update instance info",
			zap.String("request_id", requestID),
			zap.String("domain", domain),
			zap.Error(err))
	}

	ft.logger.Debug("tracked activity from instance",
		zap.String("request_id", requestID),
		zap.String("domain", domain),
		zap.String("actor_id", actorID),
		zap.String("activity_type", activityType))

	return nil
}

func (ft *FederationTracker) trackActorFromInstance(ctx context.Context, requestID string, record events.DynamoDBEventRecord) error {
	// Extract the actor data
	actorData, ok := record.Change.NewImage["Actor"]
	if !ok || actorData.DataType() != events.DataTypeMap {
		return nil
	}

	// Get the actor map
	actorMap := actorData.Map()

	// Extract actor ID
	actorID := ""
	if id, ok := actorMap["id"]; ok && id.DataType() == events.DataTypeString {
		actorID = id.String()
	}

	if err := common.ValidateRequiredParam("actorID", actorID); err != nil {
		return nil
	}

	// Parse the domain from the actor ID
	domain := extractDomain(actorID)
	if common.ValidateRequiredParam("domain", domain) != nil || domain == ft.cfg.Config.Domain {
		// Local actor, not from a remote instance
		return nil
	}

	// Extract instance information from actor
	instanceInfo := &models.InstanceInfo{
		Domain:   domain,
		LastSeen: time.Now(),
	}

	// Extract public key for instance verification
	if publicKeyData, ok := actorMap["publicKey"]; ok && publicKeyData.DataType() == events.DataTypeMap {
		publicKeyMap := publicKeyData.Map()
		if keyPem, ok := publicKeyMap["publicKeyPem"]; ok && keyPem.DataType() == events.DataTypeString {
			instanceInfo.PublicKey = keyPem.String()
		}
	}

	// Extract shared inbox
	if endpointsData, ok := actorMap["endpoints"]; ok && endpointsData.DataType() == events.DataTypeMap {
		endpointsMap := endpointsData.Map()
		if sharedInbox, ok := endpointsMap["sharedInbox"]; ok && sharedInbox.DataType() == events.DataTypeString {
			instanceInfo.SharedInbox = sharedInbox.String()
		}
	}

	// Try to detect software from actor properties
	if _, hasPropertyValue := actorMap["attachment"]; hasPropertyValue {
		// Mastodon and compatible software use attachment for profile fields
		instanceInfo.Software = "mastodon"
	}

	// Create federation activity record for the actor discovery
	activity := models.NewFederationActivityBuilder().
		FromDomain(domain).
		WithType("ActorDiscovered").
		WithActor(actorID).
		WithObject(actorID, "Person").
		WithInstanceInfo(instanceInfo).
		Build()

	// Save the activity record
	if err := ft.federationActivityRepository.Create(ctx, activity); err != nil {
		ft.logger.Error("failed to create federation activity for actor",
			zap.String("request_id", requestID),
			zap.String("domain", domain),
			zap.String("actor_id", actorID),
			zap.Error(err))
		return err
	}

	// Update instance information
	if err := ft.federationActivityRepository.UpdateInstanceInfo(ctx, instanceInfo); err != nil {
		ft.logger.Warn("failed to update instance info from actor",
			zap.String("request_id", requestID),
			zap.String("domain", domain),
			zap.Error(err))
	}

	ft.logger.Debug("tracked actor from instance",
		zap.String("request_id", requestID),
		zap.String("domain", domain),
		zap.String("actor_id", actorID))

	return nil
}

// extractDomain extracts the domain from an ActivityPub ID URL
func extractDomain(actorID string) string {
	u, err := url.Parse(actorID)
	if err != nil {
		return ""
	}
	return strings.ToLower(u.Host)
}

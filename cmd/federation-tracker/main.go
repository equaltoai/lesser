// Package main implements the federation-tracker Lambda function for tracking federation relationships and activity.
package main

import (
	"context"
	"net/url"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/lift/patterns"
	"github.com/equaltoai/lesser/pkg/storage/core"
	"github.com/equaltoai/lesser/pkg/storage/dynamorm"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	dynamormCore "github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"
)

var (
	lambdaCtx                    *common.LambdaContext
	cfg                          *config.Config
	logger                       *zap.Logger
	repos                        core.RepositoryStorage //nolint:unused // dependency injection pattern - available for processor extensions
	federationActivityRepository *repositories.FederationActivityRepository
	db                           dynamormCore.DB
)

// FederationTracker implements the DynamoDBStreamHandler interface
type FederationTracker struct {
	logger                       *zap.Logger
	cfg                          *common.LambdaContext
	federationActivityRepository *repositories.FederationActivityRepository
}

func init() {
	// Standardized Lambda initialization for federation-tracker function
	lambdaCtx = common.MustInitializeLambda(common.LambdaConfig{
		ServiceName: "federation-tracker",       // federation-tracker
		LambdaType:  common.LambdaTypeProcessor, // These are background processing functions
	})

	// Automatic dependency injection
	cfg = lambdaCtx.Config
	logger = lambdaCtx.Logger
	repos = lambdaCtx.Repos.(core.RepositoryStorage)

	// Initialize with processor-specific defaults
	err := lambdaCtx.InitializeWithDefaults()
	if err != nil {
		logger.Warn("failed to initialize with defaults", zap.Error(err))
	}

	// Function-specific initialization only
	// Initialize DynamORM with Lambda optimizations
	db, err = dynamorm.NewLambdaOptimizedClient(context.Background(), cfg.Region)
	if err != nil {
		logger.Fatal("Failed to initialize DynamORM", zap.Error(err))
	}

	// Initialize repository
	federationActivityRepository = repositories.NewFederationActivityRepository(db, cfg.DynamoTableName, logger)
}

func main() {
	// Create the handler
	handler := &FederationTracker{
		logger:                       logger,
		cfg:                          lambdaCtx,
		federationActivityRepository: federationActivityRepository,
	}

	// Start the Lambda using Lift DynamoDB stream pattern
	patterns.StartDynamoDBStreamLambda("federation-tracker", handler, logger)
}

// HandleStream implements the DynamoDBStreamHandler interface
func (ft *FederationTracker) HandleStream(ctx *lift.Context, event events.DynamoDBEvent) error {
	ft.logger.Info("Processing DynamoDB stream event",
		zap.String("request_id", ctx.GetRequestID()),
		zap.Int("records", len(event.Records)),
	)

	// Process each record in the stream
	for _, record := range event.Records {
		if err := ft.processRecord(ctx, record); err != nil {
			ft.logger.Error("failed to process record",
				zap.String("request_id", ctx.GetRequestID()),
				zap.String("eventID", record.EventID),
				zap.Error(err))
			// Continue processing other records
		}
	}

	return nil
}

func (ft *FederationTracker) processRecord(ctx *lift.Context, record events.DynamoDBEventRecord) error {
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
		zap.String("request_id", ctx.GetRequestID()),
		zap.String("pk", pkStr),
		zap.String("sk", sk),
		zap.String("event_name", record.EventName),
	)

	// Handle different types of records
	switch {
	case strings.HasPrefix(pkStr, "ACTIVITY#"):
		// New activity from remote actors
		return ft.trackActivityFromInstance(ctx, record)

	case strings.HasPrefix(pkStr, "ACTOR#"):
		// New remote actor
		return ft.trackActorFromInstance(ctx, record)
	}

	return nil
}

func (ft *FederationTracker) trackActivityFromInstance(ctx *lift.Context, record events.DynamoDBEventRecord) error {
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
	if err := ft.federationActivityRepository.Create(context.Background(), activity); err != nil {
		ft.logger.Error("failed to create federation activity",
			zap.String("request_id", ctx.GetRequestID()),
			zap.String("domain", domain),
			zap.String("actor_id", actorID),
			zap.Error(err))
		return err
	}

	// Update instance information
	if err := ft.federationActivityRepository.UpdateInstanceInfo(context.Background(), instanceInfo); err != nil {
		ft.logger.Warn("failed to update instance info",
			zap.String("request_id", ctx.GetRequestID()),
			zap.String("domain", domain),
			zap.Error(err))
	}

	ft.logger.Debug("tracked activity from instance",
		zap.String("request_id", ctx.GetRequestID()),
		zap.String("domain", domain),
		zap.String("actor_id", actorID),
		zap.String("activity_type", activityType))

	return nil
}

func (ft *FederationTracker) trackActorFromInstance(ctx *lift.Context, record events.DynamoDBEventRecord) error {
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
	if err := ft.federationActivityRepository.Create(context.Background(), activity); err != nil {
		ft.logger.Error("failed to create federation activity for actor",
			zap.String("request_id", ctx.GetRequestID()),
			zap.String("domain", domain),
			zap.String("actor_id", actorID),
			zap.Error(err))
		return err
	}

	// Update instance information
	if err := ft.federationActivityRepository.UpdateInstanceInfo(context.Background(), instanceInfo); err != nil {
		ft.logger.Warn("failed to update instance info from actor",
			zap.String("request_id", ctx.GetRequestID()),
			zap.String("domain", domain),
			zap.Error(err))
	}

	ft.logger.Debug("tracked actor from instance",
		zap.String("request_id", ctx.GetRequestID()),
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

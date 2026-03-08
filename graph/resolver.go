package graph

import (
	"context"

	"github.com/equaltoai/lesser/pkg/ai"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/cost"
	"github.com/equaltoai/lesser/pkg/mastodon"
	"github.com/equaltoai/lesser/pkg/services"
	"github.com/equaltoai/lesser/pkg/services/notes"
	soulservice "github.com/equaltoai/lesser/pkg/services/souls"
	"github.com/equaltoai/lesser/pkg/storage/core"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"go.uber.org/zap"
)

type notesService interface {
	ReblogNote(context.Context, *notes.ReblogNoteCommand) (*notes.LikeResult, error)
	UnreblogNote(context.Context, *notes.UnreblogNoteCommand) (*notes.LikeResult, error)
	GetNote(context.Context, string) (*models.Status, error)
	GetNoteWithViewer(context.Context, *notes.GetNoteQuery) (*models.Status, error)
	HasReblogged(context.Context, string, string) (bool, error)
}

type soulInventoryService interface {
	ListMine(context.Context, string) ([]soulservice.Soul, error)
	Incorporate(context.Context, string, string) (*soulservice.Soul, error)
}

// This file will not be regenerated automatically.
//
// It serves as dependency injection for your app, add any dependencies you require here.

// Resolver is the root resolver for GraphQL operations
// It uses the service registry for all business logic operations
type Resolver struct {
	// Service registry - primary source for all business operations
	Registry *services.Registry
	// Optional override used in tests to bypass the service registry
	notesClient notesService
	// Optional override used in tests for the souls inventory/incorporation service
	soulsClient soulInventoryService

	// Legacy fields (to be phased out)
	Config              *config.Config
	Storage             core.RepositoryStorage
	CostTracker         *cost.Tracker
	UnifiedTracker      *cost.UnifiedTracker // Centralized cost tracking
	TableName           string               // DynamoDB table name
	S3BucketName        string               // S3 bucket name
	MastodonConv        mastodon.Converter
	Logger              *zap.Logger
	SubscriptionManager *SubscriptionManager // For GraphQL subscriptions
	AIService           *ai.AIService        // AI analysis service
}

func (r *Resolver) notesService() notesService {
	if r == nil {
		return nil
	}
	if r.notesClient != nil {
		return r.notesClient
	}
	if r.Registry != nil {
		return r.Registry.Notes()
	}
	return nil
}

// Activity returns the ActivityResolver implementation
func (r *Resolver) Activity() ActivityResolver { return &activityResolver{r} }

// Actor returns the ActorResolver implementation
func (r *Resolver) Actor() ActorResolver { return &actorResolver{r} }

// Attachment returns the AttachmentResolver implementation
func (r *Resolver) Attachment() AttachmentResolver { return &attachmentResolver{r} }

// ImageAnalysis returns the ImageAnalysisResolver implementation
func (r *Resolver) ImageAnalysis() ImageAnalysisResolver { return &imageAnalysisResolver{r} }

// ModerationDecision returns the ModerationDecisionResolver implementation
func (r *Resolver) ModerationDecision() ModerationDecisionResolver {
	return &moderationDecisionResolver{r}
}

// ModerationLabel returns the ModerationLabelResolver implementation
func (r *Resolver) ModerationLabel() ModerationLabelResolver { return &moderationLabelResolver{r} }

// ModerationPattern returns the ModerationPatternResolver implementation
func (r *Resolver) ModerationPattern() ModerationPatternResolver {
	return &moderationPatternResolver{r}
}

// Mutation returns the MutationResolver implementation
func (r *Resolver) Mutation() MutationResolver { return &mutationResolver{r} }

// Object returns the ObjectResolver implementation
func (r *Resolver) Object() ObjectResolver { return &objectResolver{r} }

// Query returns the QueryResolver implementation
func (r *Resolver) Query() QueryResolver { return &queryResolver{r} }

// QuoteContext returns the QuoteContextResolver implementation
func (r *Resolver) QuoteContext() QuoteContextResolver { return &quoteContextResolver{r} }

// Subscription returns the SubscriptionResolver implementation
func (r *Resolver) Subscription() SubscriptionResolver { return &subscriptionResolver{r} }

// Tag returns the TagResolver implementation
func (r *Resolver) Tag() TagResolver { return &tagResolver{r} }

// TextAnalysis returns the TextAnalysisResolver implementation
func (r *Resolver) TextAnalysis() TextAnalysisResolver { return &textAnalysisResolver{r} }

// TrustEdge returns the TrustEdgeResolver implementation
func (r *Resolver) TrustEdge() TrustEdgeResolver { return &trustEdgeResolver{r} }

// ModerationFilter returns the ModerationFilterResolver implementation
func (r *Resolver) ModerationFilter() ModerationFilterResolver {
	return &moderationFilterResolver{r}
}

// Helper method for unified cost tracking
func (r *Resolver) trackDynamoOperation(ctx context.Context, operation string, units int64) {
	if r.UnifiedTracker == nil {
		return
	}

	tableName := r.TableName
	if tableName == "" {
		tableName = "lesser-main"
	}

	var err error
	switch operation {
	case DynamoOperationRead:
		err = r.UnifiedTracker.TrackDynamoRead(ctx, tableName, units)
	case "write":
		err = r.UnifiedTracker.TrackDynamoWrite(ctx, tableName, units)
	case DynamoOperationQuery:
		err = r.UnifiedTracker.TrackDynamoQuery(ctx, tableName, units)
	case "scan":
		err = r.UnifiedTracker.TrackDynamoScan(ctx, tableName, units)
	}

	if err != nil {
		r.Logger.Warn("Failed to track cost", zap.String("operation", operation), zap.Error(err))
	}
}

// Helper method for S3 cost tracking
func (r *Resolver) trackS3Operation(ctx context.Context, operation string, units int) {
	if r.UnifiedTracker == nil {
		return
	}

	bucketName := r.S3BucketName
	if bucketName == "" {
		bucketName = "lesser-media"
	}

	var err error
	switch operation {
	case "get":
		err = r.UnifiedTracker.TrackS3Get(ctx, bucketName, int64(units))
	case "put":
		err = r.UnifiedTracker.TrackS3Put(ctx, bucketName, int64(units))
	case "delete":
		err = r.UnifiedTracker.TrackS3Delete(ctx, bucketName)
	}

	if err != nil {
		r.Logger.Warn("Failed to track S3 cost", zap.String("operation", operation), zap.Error(err))
	}
}

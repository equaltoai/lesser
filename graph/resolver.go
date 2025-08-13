package graph

import (
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/equaltoai/lesser/pkg/ai"
	"github.com/equaltoai/lesser/pkg/cost"
	"github.com/equaltoai/lesser/pkg/mastodon"
	"github.com/equaltoai/lesser/pkg/services"
	"github.com/equaltoai/lesser/pkg/storage/core"
	"go.uber.org/zap"
)

// This file will not be regenerated automatically.
//
// It serves as dependency injection for your app, add any dependencies you require here.

// Resolver is the root resolver for GraphQL operations
// It uses the service registry for all business logic operations
type Resolver struct {
	// Service registry - primary source for all business operations
	Registry *services.Registry

	// Legacy fields (to be phased out)
	Storage             core.RepositoryStorage
	CostTracker         *cost.Tracker
	MastodonConv        mastodon.Converter
	Logger              *zap.Logger
	SubscriptionManager *SubscriptionManager // For GraphQL subscriptions
	DynamoClient        *dynamodb.Client     // Needed for subscription manager
	AIService           *ai.AIService        // AI analysis service
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

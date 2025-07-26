package graph

import (
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/equaltoai/lesser/pkg/cost"
	"github.com/equaltoai/lesser/pkg/mastodon"
	"github.com/equaltoai/lesser/pkg/storage"
	"go.uber.org/zap"
)

// This file will not be regenerated automatically.
//
// It serves as dependency injection for your app, add any dependencies you require here.

type Resolver struct {
	Storage             storage.Storage
	CostTracker         *cost.Tracker
	MastodonConv        mastodon.Converter
	Logger              *zap.Logger
	SubscriptionManager *SubscriptionManager // For GraphQL subscriptions
	DynamoClient        *dynamodb.Client     // Needed for subscription manager
}

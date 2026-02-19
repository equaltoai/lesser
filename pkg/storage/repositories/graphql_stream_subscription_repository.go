package repositories

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/theory-cloud/tabletheory/pkg/core"
	"github.com/theory-cloud/tabletheory/pkg/errors"
	"go.uber.org/zap"
)

// GraphQLStreamSubscriptionRepository persists GraphQL WS subscription IDs keyed by stream.
//
// This is distinct from WebSocketSubscription (streaming/SSE) records because GraphQL subscriptions
// must echo the client-provided `graphql-transport-ws` subscribe.id on every `next` frame.
type GraphQLStreamSubscriptionRepository struct {
	db        core.DB
	tableName string
	logger    *zap.Logger
}

// NewGraphQLStreamSubscriptionRepository creates a new GraphQLStreamSubscriptionRepository.
func NewGraphQLStreamSubscriptionRepository(db core.DB, tableName string, logger *zap.Logger) *GraphQLStreamSubscriptionRepository {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &GraphQLStreamSubscriptionRepository{
		db:        db,
		tableName: tableName,
		logger:    logger,
	}
}

// Put stores a subscription record for a stream.
func (r *GraphQLStreamSubscriptionRepository) Put(ctx context.Context, record *models.GraphQLStreamSubscription) error {
	if record == nil {
		return storage.ErrInvalidInput
	}
	record.CreatedAt = time.Now().UTC()
	if record.TTL == 0 {
		record.TTL = time.Now().UTC().Add(24 * time.Hour).Unix()
	}
	if err := record.UpdateKeys(); err != nil {
		return err
	}

	return r.db.WithContext(ctx).Model(record).Create()
}

// ListByStream returns all GraphQL subscriptions registered for a stream.
func (r *GraphQLStreamSubscriptionRepository) ListByStream(ctx context.Context, stream string) ([]models.GraphQLStreamSubscription, error) {
	stream = strings.TrimSpace(stream)
	if stream == "" {
		return nil, storage.ErrInvalidInput
	}

	var items []models.GraphQLStreamSubscription
	pk := fmt.Sprintf("GQLSUB#%s", stream)
	err := r.db.WithContext(ctx).Model(&models.GraphQLStreamSubscription{}).
		Where("PK", "=", pk).
		All(&items)
	if err != nil {
		if errors.IsNotFound(err) {
			return []models.GraphQLStreamSubscription{}, nil
		}
		return nil, err
	}
	return items, nil
}

// Delete removes a specific GraphQL subscription record for a stream.
func (r *GraphQLStreamSubscriptionRepository) Delete(ctx context.Context, stream, connectionID, subscriptionID string) error {
	stream = strings.TrimSpace(stream)
	connectionID = strings.TrimSpace(connectionID)
	subscriptionID = strings.TrimSpace(subscriptionID)
	if stream == "" || connectionID == "" || subscriptionID == "" {
		return storage.ErrInvalidInput
	}

	pk := fmt.Sprintf("GQLSUB#%s", stream)
	sk := fmt.Sprintf("CONN#%s#SUB#%s", connectionID, subscriptionID)
	return r.db.WithContext(ctx).Model(&models.GraphQLStreamSubscription{}).
		Where("PK", "=", pk).
		Where("SK", "=", sk).
		Delete()
}

// DeleteSubscription removes all stream registrations for a single GraphQL subscription id on a connection.
func (r *GraphQLStreamSubscriptionRepository) DeleteSubscription(ctx context.Context, connectionID, subscriptionID string) error {
	connectionID = strings.TrimSpace(connectionID)
	subscriptionID = strings.TrimSpace(subscriptionID)
	if connectionID == "" || subscriptionID == "" {
		return storage.ErrInvalidInput
	}

	var items []models.GraphQLStreamSubscription
	err := r.db.WithContext(ctx).Model(&models.GraphQLStreamSubscription{}).
		Index("gsi1").
		Where("gsi1PK", "=", fmt.Sprintf("CONN#%s", connectionID)).
		Where("gsi1SK", "begins_with", fmt.Sprintf("SUB#%s#", subscriptionID)).
		All(&items)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return err
	}

	for _, item := range items {
		if err := r.db.WithContext(ctx).Model(&models.GraphQLStreamSubscription{}).
			Where("PK", "=", item.PK).
			Where("SK", "=", item.SK).
			Delete(); err != nil {
			r.logger.Warn("failed to delete graphql stream subscription",
				zap.String("connection_id", connectionID),
				zap.String("subscription_id", subscriptionID),
				zap.String("stream", item.Stream),
				zap.Error(err))
		}
	}

	return nil
}

// DeleteAllForConnection removes all GraphQL stream subscriptions for a connection.
func (r *GraphQLStreamSubscriptionRepository) DeleteAllForConnection(ctx context.Context, connectionID string) error {
	connectionID = strings.TrimSpace(connectionID)
	if connectionID == "" {
		return storage.ErrInvalidInput
	}

	var items []models.GraphQLStreamSubscription
	err := r.db.WithContext(ctx).Model(&models.GraphQLStreamSubscription{}).
		Index("gsi1").
		Where("gsi1PK", "=", fmt.Sprintf("CONN#%s", connectionID)).
		All(&items)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return err
	}

	for _, item := range items {
		if err := r.db.WithContext(ctx).Model(&models.GraphQLStreamSubscription{}).
			Where("PK", "=", item.PK).
			Where("SK", "=", item.SK).
			Delete(); err != nil {
			r.logger.Warn("failed to delete graphql stream subscription",
				zap.String("connection_id", connectionID),
				zap.String("subscription_id", item.SubscriptionID),
				zap.String("stream", item.Stream),
				zap.Error(err))
		}
	}

	return nil
}

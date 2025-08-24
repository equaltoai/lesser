package repositories

import (
	"context"
	"fmt"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/dynamorm/pkg/core"
	"go.uber.org/zap"
)

const (
	// Model type constants
	modelTypeBlock = "Block"
)

// RelationshipPaginationConfig holds configuration for paginated relationship queries
type RelationshipPaginationConfig struct {
	IndexName   string // "" for main table, "GSI1", "GSI5", etc. for GSIs
	PKFormat    string // Format string for partition key, e.g. "ACTOR#%s#BLOCKS"
	SKField     string // Field name for sort key in cursor, e.g. "SK" or "GSI1SK"
	ActorField  string // "Actor" or "Object" - which field to extract for result
	ErrorPrefix string // Prefix for error messages, e.g. "blocked users" or "muted users"
	ModelType   string // modelTypeBlock or "Mute" - which model to query
}

// getPaginatedRelationshipList is a generic helper for relationship pagination
func getPaginatedRelationshipList(
	ctx context.Context,
	db core.DB,
	logger *zap.Logger,
	actorUsername string,
	limit int,
	cursor string,
	config RelationshipPaginationConfig,
) ([]string, string, error) {
	limit = NormalizePaginationLimit(limit)

	query, err := buildRelationshipQuery(ctx, db, actorUsername, limit, cursor, config)
	if err != nil {
		return nil, "", err
	}

	if config.ModelType == modelTypeBlock {
		return executeBlockQuery(query, limit, logger, actorUsername, config)
	}
	return executeMuteQuery(query, limit, logger, actorUsername, config)
}

// buildRelationshipQuery creates and configures the database query
func buildRelationshipQuery(
	ctx context.Context,
	db core.DB,
	actorUsername string,
	limit int,
	cursor string,
	config RelationshipPaginationConfig,
) (core.Query, error) {
	var query core.Query

	switch config.ModelType {
	case modelTypeBlock:
		query = db.WithContext(ctx).Model(&models.Block{})
	case "Mute":
		query = db.WithContext(ctx).Model(&models.Mute{})
	default:
		return nil, fmt.Errorf("%w: %s", ErrRelationshipPaginationModelTypeUnsupported, config.ModelType)
	}

	query = configureQueryIndex(query, actorUsername, config)
	query = query.Limit(limit + 1) // Get one more item to determine if there are more results

	if cursor != "" {
		query = query.Cursor(cursor)
	}

	return query, nil
}

// configureQueryIndex sets up the query with the appropriate index and partition key
func configureQueryIndex(query core.Query, actorUsername string, config RelationshipPaginationConfig) core.Query {
	if config.IndexName != "" {
		query = query.Index(config.IndexName)
		query = query.Where(config.IndexName+"PK", "=", fmt.Sprintf(config.PKFormat, actorUsername))
	} else {
		query = query.Where("PK", "=", fmt.Sprintf(config.PKFormat, actorUsername))
	}
	return query
}

// executeBlockQuery executes the query for Block model type
func executeBlockQuery(
	query core.Query,
	limit int,
	logger *zap.Logger,
	actorUsername string,
	config RelationshipPaginationConfig,
) ([]string, string, error) {
	var items []models.Block
	err := query.All(&items)
	if err != nil {
		logger.Error(fmt.Sprintf("failed to get %s", config.ErrorPrefix),
			zap.String("actor", actorUsername),
			zap.Error(err))
		return nil, "", fmt.Errorf("%w (%s): %w", ErrRelationshipPaginationQueryFailed, config.ErrorPrefix, err)
	}

	nextCursor := generateBlockCursor(items, limit, config.SKField)
	if len(items) > limit {
		items = items[:limit]
	}

	results := extractBlockResults(items, config.ActorField)
	return results, nextCursor, nil
}

// executeMuteQuery executes the query for Mute model type
func executeMuteQuery(
	query core.Query,
	limit int,
	logger *zap.Logger,
	actorUsername string,
	config RelationshipPaginationConfig,
) ([]string, string, error) {
	var items []models.Mute
	err := query.All(&items)
	if err != nil {
		logger.Error(fmt.Sprintf("failed to get %s", config.ErrorPrefix),
			zap.String("actor", actorUsername),
			zap.Error(err))
		return nil, "", fmt.Errorf("%w (%s): %w", ErrRelationshipPaginationQueryFailed, config.ErrorPrefix, err)
	}

	nextCursor := generateMuteCursor(items, limit, config.SKField)
	if len(items) > limit {
		items = items[:limit]
	}

	results := extractMuteResults(items, config.ActorField)
	return results, nextCursor, nil
}

// generateBlockCursor creates the next cursor for Block items
func generateBlockCursor(items []models.Block, limit int, skField string) string {
	if len(items) <= limit {
		return ""
	}

	if skField == "GSI5SK" {
		return items[limit-1].GSI5SK
	}
	return items[limit-1].SK
}

// generateMuteCursor creates the next cursor for Mute items
func generateMuteCursor(items []models.Mute, limit int, skField string) string {
	if len(items) <= limit {
		return ""
	}

	if skField == "GSI1SK" {
		return items[limit-1].GSI1SK
	}
	return items[limit-1].SK
}

// extractBlockResults extracts the appropriate field values from Block items
func extractBlockResults(items []models.Block, actorField string) []string {
	results := make([]string, len(items))
	for i, item := range items {
		if actorField == "Actor" {
			results[i] = item.Actor
		} else {
			results[i] = item.Object
		}
	}
	return results
}

// extractMuteResults extracts the appropriate field values from Mute items
func extractMuteResults(items []models.Mute, actorField string) []string {
	results := make([]string, len(items))
	for i, item := range items {
		if actorField == "Actor" {
			results[i] = item.Actor
		} else {
			results[i] = item.Object
		}
	}
	return results
}

// getPaginatedBlockList is a helper for block relationship pagination
func getPaginatedBlockList(
	ctx context.Context,
	db core.DB,
	logger *zap.Logger,
	actorUsername string,
	limit int,
	cursor string,
	config RelationshipPaginationConfig,
) ([]string, string, error) {
	config.ModelType = modelTypeBlock
	return getPaginatedRelationshipList(ctx, db, logger, actorUsername, limit, cursor, config)
}

// getPaginatedMuteList is a helper for mute relationship pagination
func getPaginatedMuteList(
	ctx context.Context,
	db core.DB,
	logger *zap.Logger,
	actorUsername string,
	limit int,
	cursor string,
	config RelationshipPaginationConfig,
) ([]string, string, error) {
	config.ModelType = "Mute"
	return getPaginatedRelationshipList(ctx, db, logger, actorUsername, limit, cursor, config)
}

package repositories

import (
	"context"
	"fmt"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/dynamorm/pkg/core"
	"go.uber.org/zap"
)

// RelationshipPaginationConfig holds configuration for paginated relationship queries
type RelationshipPaginationConfig struct {
	IndexName   string // "" for main table, "GSI1", "GSI5", etc. for GSIs  
	PKFormat    string // Format string for partition key, e.g. "ACTOR#%s#BLOCKS"
	SKField     string // Field name for sort key in cursor, e.g. "SK" or "GSI1SK"
	ActorField  string // "Actor" or "Object" - which field to extract for result
	ErrorPrefix string // Prefix for error messages, e.g. "blocked users" or "muted users"
	ModelType   string // "Block" or "Mute" - which model to query
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

	var query core.Query
	var err error
	
	// Create query based on model type
	if config.ModelType == "Block" {
		query = db.WithContext(ctx).Model(&models.Block{})
	} else if config.ModelType == "Mute" {
		query = db.WithContext(ctx).Model(&models.Mute{})
	} else {
		return nil, "", fmt.Errorf("unsupported model type: %s", config.ModelType)
	}
	
	// Use index if specified
	if config.IndexName != "" {
		query = query.Index(config.IndexName)
		query = query.Where(config.IndexName+"PK", "=", fmt.Sprintf(config.PKFormat, actorUsername))
	} else {
		query = query.Where("PK", "=", fmt.Sprintf(config.PKFormat, actorUsername))
	}
	
	query = query.Limit(limit)

	if cursor != "" {
		query = query.Cursor(cursor)
	}

	// Get one more item than requested to determine if there are more results
	query = query.Limit(limit + 1)

	var results []string
	var nextCursor string
	
	if config.ModelType == "Block" {
		var items []models.Block
		err = query.All(&items)
		if err != nil {
			logger.Error(fmt.Sprintf("failed to get %s", config.ErrorPrefix),
				zap.String("actor", actorUsername),
				zap.Error(err))
			return nil, "", fmt.Errorf("failed to get %s: %w", config.ErrorPrefix, err)
		}
		
		// Generate next cursor
		if len(items) > limit {
			if config.SKField == "GSI5SK" {
				nextCursor = items[limit-1].GSI5SK
			} else {
				nextCursor = items[limit-1].SK
			}
			items = items[:limit]
		}
		
		// Extract actor IDs
		results = make([]string, len(items))
		for i, item := range items {
			if config.ActorField == "Actor" {
				results[i] = item.Actor
			} else {
				results[i] = item.Object
			}
		}
	} else { // ModelType == "Mute"
		var items []models.Mute
		err = query.All(&items)
		if err != nil {
			logger.Error(fmt.Sprintf("failed to get %s", config.ErrorPrefix),
				zap.String("actor", actorUsername),
				zap.Error(err))
			return nil, "", fmt.Errorf("failed to get %s: %w", config.ErrorPrefix, err)
		}
		
		// Generate next cursor
		if len(items) > limit {
			if config.SKField == "GSI1SK" {
				nextCursor = items[limit-1].GSI1SK
			} else {
				nextCursor = items[limit-1].SK
			}
			items = items[:limit]
		}
		
		// Extract actor IDs
		results = make([]string, len(items))
		for i, item := range items {
			if config.ActorField == "Actor" {
				results[i] = item.Actor
			} else {
				results[i] = item.Object
			}
		}
	}

	return results, nextCursor, nil
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
	config.ModelType = "Block"
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
package repositories

import (
	"context"
	"fmt"
	"strings"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/dynamorm/pkg/errors"
	"go.uber.org/zap"
)

// RelationshipHelper handles common operations for block/mute relationships
type RelationshipHelper struct {
	DB           core.DB
	Logger       *zap.Logger
	RelationType string // "block" or "mute"
}

// NewRelationshipHelper creates a new relationship helper
func NewRelationshipHelper(db core.DB, logger *zap.Logger, relationType string) *RelationshipHelper {
	return &RelationshipHelper{
		DB:           db,
		Logger:       logger,
		RelationType: relationType,
	}
}

// DeleteRelationship removes a relationship (for Undo operations)
func (h *RelationshipHelper) DeleteRelationship(
	ctx context.Context,
	actorActor, targetActor string,
	pkFormat, skFormat string, // e.g., "ACTOR#%s#BLOCKS", "BLOCKED#%s" or "MUTE#%s", "MUTED#%s"
	modelType interface{},
) error {
	// Extract usernames for key generation
	actorUsername := extractUsernameFromActor(actorActor)
	targetUsername := extractUsernameFromActor(targetActor)

	pk := fmt.Sprintf(pkFormat, actorUsername)
	sk := fmt.Sprintf(skFormat, targetUsername)

	err := h.DB.WithContext(ctx).Model(modelType).
		Where("PK", "=", pk).
		Where("SK", "=", sk).
		Delete()

	if err != nil {
		if errors.IsNotFound(err) {
			h.Logger.Debug(fmt.Sprintf("%s not found for deletion", h.RelationType),
				zap.String("actor", actorActor),
				zap.String("target", targetActor))
			return nil // Idempotent - don't fail if relationship doesn't exist
		}
		h.Logger.Error(fmt.Sprintf("failed to delete %s", h.RelationType),
			zap.String("actor", actorActor),
			zap.String("target", targetActor),
			zap.Error(err))
		return ErrorHandler.HandleDeleteError(err, h.RelationType, fmt.Sprintf("%s_%s", actorActor, targetActor))
	}

	h.Logger.Info(fmt.Sprintf("deleted %s relationship", h.RelationType),
		zap.String("actor", actorActor),
		zap.String("target", targetActor))

	return nil
}

// CheckRelationship checks if a relationship exists between two actors
func (h *RelationshipHelper) CheckRelationship(
	ctx context.Context,
	actorActor, targetActor string,
	pkFormat, skFormat string,
	modelType interface{},
) (bool, error) {
	// Extract usernames for key generation
	actorUsername := extractUsernameFromActor(actorActor)
	targetUsername := extractUsernameFromActor(targetActor)

	err := h.DB.WithContext(ctx).Model(modelType).
		Where("PK", "=", fmt.Sprintf(pkFormat, actorUsername)).
		Where("SK", "=", fmt.Sprintf(skFormat, targetUsername)).
		First(modelType)

	if err != nil {
		if errors.IsNotFound(err) {
			return false, nil
		}
		h.Logger.Error(fmt.Sprintf("failed to check %s status", h.RelationType),
			zap.String("actor", actorActor),
			zap.String("target", targetActor),
			zap.Error(err))
		return false, ErrorHandler.HandleGetError(err, h.RelationType, fmt.Sprintf("%s_%s", actorActor, targetActor))
	}

	return true, nil
}

// GetRelatedUsers returns a list of users in a relationship with the given actor
func (h *RelationshipHelper) GetRelatedUsers(
	ctx context.Context,
	actorActor string,
	limit int,
	cursor string,
	pkFormat string,
	modelType interface{},
	_ string, // "Object" for both blocks and mutes
) ([]string, string, error) {
	limit = NormalizePaginationLimit(limit)
	actorUsername := extractUsernameFromActor(actorActor)

	query := h.DB.WithContext(ctx).Model(modelType).
		Where("PK", "=", fmt.Sprintf(pkFormat, actorUsername)).
		Limit(limit)

	if cursor != "" {
		query = query.Cursor(cursor)
	}

	// Get one more item than requested to determine if there are more results
	query = query.Limit(limit + 1)

	var results []interface{}
	err := query.All(&results)
	if err != nil {
		h.Logger.Error(fmt.Sprintf("failed to get %sed users", h.RelationType),
			zap.String("actor", actorActor),
			zap.Error(err))
		return nil, "", ErrorHandler.HandleQueryError(err, h.RelationType, fmt.Sprintf("related_users_%s", actorActor))
	}

	// Generate next cursor
	var nextCursor string
	if len(results) > limit {
		// We got more results than requested, so there are more pages
		if len(results) > limit {
			if block, ok := results[limit-1].(*models.Block); ok {
				nextCursor = block.SK
			} else if mute, ok := results[limit-1].(*models.Mute); ok {
				nextCursor = mute.SK
			}
		}
		results = results[:limit] // Trim to requested limit
	}

	// Extract related actor IDs
	relatedUsers := make([]string, len(results))
	for i, result := range results {
		if block, ok := result.(*models.Block); ok {
			relatedUsers[i] = block.Object
		} else if mute, ok := result.(*models.Mute); ok {
			relatedUsers[i] = mute.Object
		}
	}

	return relatedUsers, nextCursor, nil
}

// GetUsersWhoRelated returns a list of users who have a relationship with the given actor
func (h *RelationshipHelper) GetUsersWhoRelated(
	ctx context.Context,
	targetActor string,
	limit int,
	cursor string,
	gsiIndex string,
	gsiPKFormat string,
	modelType interface{},
	_ string, // "Actor" for both blocks and mutes
) ([]string, string, error) {
	limit = NormalizePaginationLimit(limit)
	targetUsername := extractUsernameFromActor(targetActor)

	query := h.DB.WithContext(ctx).Model(modelType).
		Index(gsiIndex).
		Where(fmt.Sprintf("%sPK", gsiIndex), "=", fmt.Sprintf(gsiPKFormat, targetUsername)).
		Limit(limit)

	if cursor != "" {
		query = query.Cursor(cursor)
	}

	// Get one more item than requested to determine if there are more results
	query = query.Limit(limit + 1)

	var results []interface{}
	err := query.All(&results)
	if err != nil {
		h.Logger.Error(fmt.Sprintf("failed to get users who %sed actor", h.RelationType),
			zap.String("target_actor", targetActor),
			zap.Error(err))
		return nil, "", ErrorHandler.HandleQueryError(err, h.RelationType, fmt.Sprintf("who_related_%s", targetActor))
	}

	// Generate next cursor
	var nextCursor string
	if len(results) > limit {
		// We got more results than requested, so there are more pages
		if block, ok := results[limit-1].(*models.Block); ok {
			nextCursor = block.GSI5SK
		} else if mute, ok := results[limit-1].(*models.Mute); ok {
			nextCursor = mute.GSI1SK
		}
		results = results[:limit] // Trim to requested limit
	}

	// Extract actor IDs
	actors := make([]string, len(results))
	for i, result := range results {
		if block, ok := result.(*models.Block); ok {
			actors[i] = block.Actor
		} else if mute, ok := result.(*models.Mute); ok {
			actors[i] = mute.Actor
		}
	}

	return actors, nextCursor, nil
}

// extractUsernameFromActor extracts username from actor URL
// This is a shared utility function used by both block and mute repositories
func extractUsernameFromActor(actorURL string) string {
	// Extract username from actor URL
	// Format: https://domain.com/users/username
	parts := strings.Split(actorURL, "/")
	if len(parts) >= 2 {
		return parts[len(parts)-1] // Get the last part (username)
	}
	return actorURL // Fallback to full URL if parsing fails
}

package repositories

import (
	"context"
	"fmt"
	"time"

	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/dynamorm/pkg/errors"
	"go.uber.org/zap"
)

// RelationshipType defines the type of relationship
type RelationshipType string

// Relationship type constants
const (
	// RelationshipTypeLike represents a like/favorite relationship
	RelationshipTypeLike RelationshipType = "like"
	// RelationshipTypeBlock represents a block relationship
	RelationshipTypeBlock RelationshipType = "block"
	// RelationshipTypeMute represents a mute relationship
	RelationshipTypeMute RelationshipType = "mute"
	// RelationshipTypeFollow represents a follow relationship
	RelationshipTypeFollow RelationshipType = "follow"
	// RelationshipTypeBookmark represents a bookmark relationship
	RelationshipTypeBookmark RelationshipType = "bookmark"
	// RelationshipTypeFavorite represents a favorite relationship
	RelationshipTypeFavorite RelationshipType = "favorite"
)

// RelationshipBase provides common functionality for relationship repositories
type RelationshipBase struct {
	db           core.DB
	tableName    string
	logger       *zap.Logger
	queryUtils   *QueryUtils
	relType      RelationshipType
	errorHandler *ErrorUtils
}

// NewRelationshipBase creates a new relationship base repository
func NewRelationshipBase(db core.DB, tableName string, logger *zap.Logger, relType RelationshipType) *RelationshipBase {
	return &RelationshipBase{
		db:           db,
		tableName:    tableName,
		logger:       logger,
		queryUtils:   NewQueryUtils(db, logger),
		relType:      relType,
		errorHandler: ErrorHandler,
	}
}

// RelationshipModel represents a generic relationship
type RelationshipModel struct {
	PK        string    `dynamorm:"PK,pk"`
	SK        string    `dynamorm:"SK,sk"`
	GSI1PK    string    `dynamorm:"GSI1PK,gsi1pk"`
	GSI1SK    string    `dynamorm:"GSI1SK,gsi1sk"`
	Actor     string    `dynamorm:"Actor"`
	Object    string    `dynamorm:"Object"`
	Type      string    `dynamorm:"Type"`
	CreatedAt time.Time `dynamorm:"CreatedAt"`
	ID        string    `dynamorm:"ID"`
}

// CreateRelationship creates a new relationship with idempotency
func (r *RelationshipBase) CreateRelationship(ctx context.Context, actor, object, id string) error {
	pk, sk := r.generateKeys(actor, object)

	model := &RelationshipModel{
		PK:        pk,
		SK:        sk,
		GSI1PK:    r.generateGSI1PK(actor),
		GSI1SK:    r.generateGSI1SK(object),
		Actor:     actor,
		Object:    object,
		Type:      string(r.relType),
		CreatedAt: time.Now().UTC(),
		ID:        id,
	}

	err := r.db.WithContext(ctx).Model(model).Create()
	if err != nil {
		// Check if it's a duplicate key error (already exists)
		if errors.IsConditionFailed(err) {
			r.logger.Debug("relationship already exists",
				zap.String("type", string(r.relType)),
				zap.String("actor", actor),
				zap.String("object", object))
			return nil // Idempotent
		}
		r.logger.Error("failed to create relationship",
			zap.String("type", string(r.relType)),
			zap.String("actor", actor),
			zap.String("object", object),
			zap.Error(err))
		entityType := r.getEntityType()
		identifier := fmt.Sprintf("%s->%s", actor, object)
		return r.errorHandler.HandleCreateError(err, entityType, identifier)
	}

	r.logger.Info("created relationship",
		zap.String("type", string(r.relType)),
		zap.String("actor", actor),
		zap.String("object", object),
		zap.String("id", id))

	return nil
}

// DeleteRelationship removes a relationship with idempotency
func (r *RelationshipBase) DeleteRelationship(ctx context.Context, actor, object string) error {
	pk, sk := r.generateKeys(actor, object)

	err := r.queryUtils.DeleteItem(ctx, pk, sk, &RelationshipModel{})
	if err != nil {
		// Check if not found error - make idempotent
		if errors.IsNotFound(err) {
			r.logger.Debug("relationship not found for deletion",
				zap.String("type", string(r.relType)),
				zap.String("actor", actor),
				zap.String("object", object))
			return nil
		}
		r.logger.Error("failed to delete relationship",
			zap.String("type", string(r.relType)),
			zap.String("actor", actor),
			zap.String("object", object),
			zap.Error(err))
		entityType := r.getEntityType()
		identifier := fmt.Sprintf("%s->%s", actor, object)
		return r.errorHandler.HandleDeleteError(err, entityType, identifier)
	}

	r.logger.Info("deleted relationship",
		zap.String("type", string(r.relType)),
		zap.String("actor", actor),
		zap.String("object", object))

	return nil
}

// GetRelationship retrieves a specific relationship
func (r *RelationshipBase) GetRelationship(ctx context.Context, actor, object string) (*RelationshipModel, error) {
	pk, sk := r.generateKeys(actor, object)

	var model RelationshipModel
	err := r.queryUtils.GetItemByPK(ctx, pk, sk, &model)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, nil // Not found is not an error for existence checks
		}
		entityType := r.getEntityType()
		identifier := fmt.Sprintf("%s->%s", actor, object)
		return nil, r.errorHandler.HandleGetError(err, entityType, identifier)
	}

	return &model, nil
}

// ExistsRelationship checks if a relationship exists
func (r *RelationshipBase) ExistsRelationship(ctx context.Context, actor, object string) (bool, error) {
	pk, sk := r.generateKeys(actor, object)
	return r.queryUtils.ExistsQuery(ctx, pk, sk)
}

// GetRelationshipsByActor retrieves all relationships for an actor with pagination
func (r *RelationshipBase) GetRelationshipsByActor(ctx context.Context, actor string, limit int, cursor string) ([]*RelationshipModel, string, error) {
	result, err := r.queryUtils.QueryByGSI(ctx, "gsi1", r.generateGSI1PK(actor), "", &QueryOptions{
		Limit:  limit,
		Cursor: cursor,
	})
	if err != nil {
		entityType := r.getEntityType()
		queryType := "by actor"
		return nil, "", r.errorHandler.HandleQueryError(err, entityType, queryType)
	}

	// Convert map results to models
	models := make([]*RelationshipModel, 0, len(result.Items))
	for _, item := range result.Items {
		model := &RelationshipModel{}
		if err := mapToStruct(item, model); err != nil {
			r.logger.Warn("failed to convert item to model",
				zap.String("type", string(r.relType)),
				zap.Error(err))
			continue
		}
		models = append(models, model)
	}

	return models, result.NextCursor, nil
}

// GetRelationshipsByObject retrieves all relationships for an object with pagination
func (r *RelationshipBase) GetRelationshipsByObject(ctx context.Context, object string, limit int, cursor string) ([]*RelationshipModel, string, error) {
	pk := r.generateObjectPK(object)

	result, err := r.queryUtils.QueryWithPrefix(ctx, pk, "", &QueryOptions{
		Limit:  limit,
		Cursor: cursor,
	})
	if err != nil {
		entityType := r.getEntityType()
		queryType := "by object"
		return nil, "", r.errorHandler.HandleQueryError(err, entityType, queryType)
	}

	// Convert map results to models
	models := make([]*RelationshipModel, 0, len(result.Items))
	for _, item := range result.Items {
		model := &RelationshipModel{}
		if err := mapToStruct(item, model); err != nil {
			r.logger.Warn("failed to convert item to model",
				zap.String("type", string(r.relType)),
				zap.Error(err))
			continue
		}
		models = append(models, model)
	}

	return models, result.NextCursor, nil
}

// CountRelationshipsByActor returns the count of relationships for an actor
func (r *RelationshipBase) CountRelationshipsByActor(ctx context.Context, actor string) (int, error) {
	return r.queryUtils.CountQuery(ctx, r.generateGSI1PK(actor), "gsi1")
}

// CountRelationshipsByObject returns the count of relationships for an object
func (r *RelationshipBase) CountRelationshipsByObject(ctx context.Context, object string) (int, error) {
	return r.queryUtils.CountQuery(ctx, r.generateObjectPK(object), "")
}

// generateKeys generates the primary and sort keys for a relationship
func (r *RelationshipBase) generateKeys(actor, object string) (pk, sk string) {
	switch r.relType {
	case RelationshipTypeLike:
		pk = fmt.Sprintf("object#%s#likes", object)
		sk = fmt.Sprintf("actor#%s", actor)
	case RelationshipTypeBlock:
		// Extract usernames if needed
		actorUsername := extractUsernameFromActor(actor)
		blockedUsername := extractUsernameFromActor(object)
		pk = fmt.Sprintf("ACTOR#%s#BLOCKS", actorUsername)
		sk = fmt.Sprintf("BLOCKED#%s", blockedUsername)
	case RelationshipTypeMute:
		actorUsername := extractUsernameFromActor(actor)
		mutedUsername := extractUsernameFromActor(object)
		pk = fmt.Sprintf("ACTOR#%s#MUTES", actorUsername)
		sk = fmt.Sprintf("MUTED#%s", mutedUsername)
	case RelationshipTypeFollow:
		pk = fmt.Sprintf("USER#%s", actor)
		sk = fmt.Sprintf("FOLLOWING#%s", object)
	case RelationshipTypeBookmark:
		pk = fmt.Sprintf("USER#%s#BOOKMARKS", actor)
		sk = fmt.Sprintf("STATUS#%s", object)
	case RelationshipTypeFavorite:
		pk = fmt.Sprintf("USER#%s#FAVORITES", actor)
		sk = fmt.Sprintf("STATUS#%s", object)
	default:
		pk = fmt.Sprintf("%s#%s", string(r.relType), actor)
		sk = fmt.Sprintf("object#%s", object)
	}
	return pk, sk
}

// generateGSI1PK generates the GSI1 partition key
func (r *RelationshipBase) generateGSI1PK(actor string) string {
	return fmt.Sprintf("actor#%s#%ss", actor, r.relType)
}

// generateGSI1SK generates the GSI1 sort key
func (r *RelationshipBase) generateGSI1SK(object string) string {
	return fmt.Sprintf("object#%s", object)
}

// generateObjectPK generates the partition key for object-based queries
func (r *RelationshipBase) generateObjectPK(object string) string {
	switch r.relType {
	case RelationshipTypeLike:
		return fmt.Sprintf("object#%s#likes", object)
	case RelationshipTypeBlock:
		return fmt.Sprintf("object#%s#blocks", object)
	case RelationshipTypeMute:
		return fmt.Sprintf("object#%s#mutes", object)
	default:
		return fmt.Sprintf("object#%s#%ss", object, r.relType)
	}
}

// getEntityType maps RelationshipType to appropriate entity constants
func (r *RelationshipBase) getEntityType() string {
	switch r.relType {
	case RelationshipTypeFollow:
		return EntityFollow
	case RelationshipTypeBlock:
		return EntityBlock
	case RelationshipTypeMute:
		return EntityMute
	case RelationshipTypeBookmark:
		return EntityBookmark
	case RelationshipTypeLike, RelationshipTypeFavorite:
		return "favorite"
	default:
		return string(r.relType)
	}
}

// mapToStruct converts a map to a struct - simplified version
func mapToStruct(m map[string]interface{}, target interface{}) error {
	// This is a simplified implementation
	// In production, you'd use a proper mapping library or reflection
	if model, ok := target.(*RelationshipModel); ok {
		if pk, ok := m["PK"].(string); ok {
			model.PK = pk
		}
		if sk, ok := m["SK"].(string); ok {
			model.SK = sk
		}
		if actor, ok := m["Actor"].(string); ok {
			model.Actor = actor
		}
		if object, ok := m["Object"].(string); ok {
			model.Object = object
		}
		if relType, ok := m["Type"].(string); ok {
			model.Type = relType
		}
		if id, ok := m["ID"].(string); ok {
			model.ID = id
		}
		if createdAt, ok := m["CreatedAt"].(time.Time); ok {
			model.CreatedAt = createdAt
		}
	}
	return nil
}

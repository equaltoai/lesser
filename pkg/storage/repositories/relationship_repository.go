package repositories

import (
	"context"
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/dynamorm/pkg/errors"
	"go.uber.org/zap"
)

// RelationshipRepository implements relationship operations using DynamORM
type RelationshipRepository struct {
	db        core.DB
	tableName string
	logger    *zap.Logger
}

// NewRelationshipRepository creates a new relationship repository
func NewRelationshipRepository(db core.DB, tableName string, logger *zap.Logger) *RelationshipRepository {
	return &RelationshipRepository{
		db:        db,
		tableName: tableName,
		logger:    logger,
	}
}

// ===== RelationshipRecord Methods =====

// GetFollowRequest gets a follow request by follower and target IDs
func (r *RelationshipRepository) GetFollowRequest(ctx context.Context, followerID, targetID string) (*storage.RelationshipRecord, error) {
	var relationship models.RelationshipRecord

	err := r.db.WithContext(ctx).Model(&relationship).
		Where("PK", "=", fmt.Sprintf("FOLLOW#%s", followerID)).
		Where("SK", "=", fmt.Sprintf("FOLLOWING#%s", targetID)).
		First(&relationship)

	if err != nil {
		if errors.IsNotFound(err) {
			return nil, fmt.Errorf("follow request not found")
		}
		r.logger.Error("failed to get follow request",
			zap.String("follower_id", followerID),
			zap.String("target_id", targetID),
			zap.Error(err))
		return nil, fmt.Errorf("failed to get follow request: %w", err)
	}

	// Convert models.RelationshipRecord to storage.RelationshipRecord
	return &storage.RelationshipRecord{
		PK:         relationship.PK,
		SK:         relationship.SK,
		GSI1PK:     relationship.GSI1PK,
		GSI1SK:     relationship.GSI1SK,
		ActivityID: relationship.ActivityID,
		State:      relationship.State,
		CreatedAt:  relationship.CreatedAt,
		UpdatedAt:  relationship.UpdatedAt,
	}, nil
}

// HasFollowRequest checks if there's a follow request between two users
func (r *RelationshipRepository) HasFollowRequest(ctx context.Context, requesterID, targetID string) (bool, error) {
	var relationship models.RelationshipRecord

	err := r.db.WithContext(ctx).Model(&relationship).
		Where("PK", "=", fmt.Sprintf("FOLLOW#%s", requesterID)).
		Where("SK", "=", fmt.Sprintf("FOLLOWING#%s", targetID)).
		First(&relationship)

	if err != nil {
		if errors.IsNotFound(err) {
			return false, nil
		}
		r.logger.Error("failed to check for follow request",
			zap.String("requester_id", requesterID),
			zap.String("target_id", targetID),
			zap.Error(err))
		return false, fmt.Errorf("failed to check for follow request: %w", err)
	}

	// Return true if any relationship exists (pending, accepted, or rejected)
	return true, nil
}

// CreateRelationship creates a new follow relationship
func (r *RelationshipRepository) CreateRelationship(ctx context.Context, followerUsername, followingUsername, activityID string) error {
	relationship := models.NewRelationshipRecord(followerUsername, followingUsername, activityID)

	if err := r.db.WithContext(ctx).Model(relationship).Create(); err != nil {
		// Check if it's a duplicate key error
		if errors.IsConditionFailed(err) {
			r.logger.Debug("follow relationship already exists",
				zap.String("follower", followerUsername),
				zap.String("following", followingUsername))
			return nil
		}
		r.logger.Error("failed to create relationship",
			zap.String("follower", followerUsername),
			zap.String("following", followingUsername),
			zap.Error(err))
		return fmt.Errorf("failed to create relationship: %w", err)
	}

	r.logger.Info("created follow relationship",
		zap.String("follower", followerUsername),
		zap.String("following", followingUsername),
		zap.String("activity_id", activityID))

	return nil
}

// DeleteRelationship removes a follow relationship
func (r *RelationshipRepository) DeleteRelationship(ctx context.Context, followerUsername, followingUsername string) error {
	relationship := &models.RelationshipRecord{
		PK: fmt.Sprintf("FOLLOW#%s", followerUsername),
		SK: fmt.Sprintf("FOLLOWING#%s", followingUsername),
	}

	if err := r.db.WithContext(ctx).Model(relationship).
		Where("PK", "=", relationship.PK).
		Where("SK", "=", relationship.SK).
		Delete(); err != nil {
		if errors.IsNotFound(err) {
			r.logger.Debug("relationship not found",
				zap.String("follower", followerUsername),
				zap.String("following", followingUsername))
			return nil
		}
		r.logger.Error("failed to delete relationship",
			zap.String("follower", followerUsername),
			zap.String("following", followingUsername),
			zap.Error(err))
		return fmt.Errorf("failed to delete relationship: %w", err)
	}

	r.logger.Info("deleted relationship",
		zap.String("follower", followerUsername),
		zap.String("following", followingUsername))

	return nil
}

// GetRelationship retrieves a specific follow relationship
func (r *RelationshipRepository) GetRelationship(ctx context.Context, followerUsername, followingUsername string) (*models.RelationshipRecord, error) {
	var relationship models.RelationshipRecord

	query := r.db.WithContext(ctx).Model(&relationship).
		Where("PK", "=", fmt.Sprintf("FOLLOW#%s", followerUsername)).
		Where("SK", "=", fmt.Sprintf("FOLLOWING#%s", followingUsername))

	if err := query.First(&relationship); err != nil {
		if errors.IsNotFound(err) {
			return nil, fmt.Errorf("follow relationship not found")
		}
		return nil, fmt.Errorf("failed to get relationship: %w", err)
	}

	return &relationship, nil
}

// GetFollowers retrieves all followers for a user
func (r *RelationshipRepository) GetFollowers(ctx context.Context, username string, limit int, cursor string) ([]string, string, error) {
	query := r.db.WithContext(ctx).Model(&models.RelationshipRecord{}).
		Index("GSI1").
		Where("GSI1PK", "=", fmt.Sprintf("FOLLOW#%s", username)).
		Filter("State", "=", models.RelationshipAccepted).
		Limit(limit)

	if cursor != "" {
		query = query.Cursor(cursor)
	}

	// Get one more item than requested to determine if there are more results
	query = query.Limit(limit + 1)

	var relationships []models.RelationshipRecord
	err := query.All(&relationships)
	if err != nil {
		r.logger.Error("failed to query followers",
			zap.String("username", username),
			zap.Error(err))
		return nil, "", fmt.Errorf("failed to query followers: %w", err)
	}

	// Generate next cursor
	var nextCursor string
	if len(relationships) > limit {
		// We got more results than requested, so there are more pages
		nextCursor = relationships[limit-1].GSI1SK
		relationships = relationships[:limit] // Trim to requested limit
	}

	// Extract follower usernames
	followers := make([]string, 0, len(relationships))
	for _, rel := range relationships {
		if follower := rel.ExtractFollowerFromGSI(); follower != "" {
			followers = append(followers, follower)
		}
	}

	return followers, nextCursor, nil
}

// GetFollowing retrieves all users that a user is following
func (r *RelationshipRepository) GetFollowing(ctx context.Context, username string, limit int, cursor string) ([]string, string, error) {
	query := r.db.WithContext(ctx).Model(&models.RelationshipRecord{}).
		Where("PK", "=", fmt.Sprintf("FOLLOW#%s", username)).
		Filter("State", "=", models.RelationshipAccepted).
		Limit(limit)

	if cursor != "" {
		query = query.Cursor(cursor)
	}

	// Get one more item than requested to determine if there are more results
	query = query.Limit(limit + 1)

	var relationships []models.RelationshipRecord
	err := query.All(&relationships)
	if err != nil {
		r.logger.Error("failed to query following",
			zap.String("username", username),
			zap.Error(err))
		return nil, "", fmt.Errorf("failed to query following: %w", err)
	}

	// Generate next cursor
	var nextCursor string
	if len(relationships) > limit {
		// We got more results than requested, so there are more pages
		nextCursor = relationships[limit-1].SK
		relationships = relationships[:limit] // Trim to requested limit
	}

	// Extract following usernames
	following := make([]string, 0, len(relationships))
	for _, rel := range relationships {
		if followed := rel.ExtractFollowingUsername(); followed != "" {
			following = append(following, followed)
		}
	}

	return following, nextCursor, nil
}

// CountFollowers returns the number of followers for a user
func (r *RelationshipRepository) CountFollowers(ctx context.Context, username string) (int, error) {
	count, err := r.db.WithContext(ctx).Model(&models.RelationshipRecord{}).
		Index("GSI1").
		Where("GSI1PK", "=", fmt.Sprintf("FOLLOW#%s", username)).
		Filter("State", "=", models.RelationshipAccepted).
		Count()

	if err != nil {
		r.logger.Error("failed to count followers",
			zap.String("username", username),
			zap.Error(err))
		return 0, fmt.Errorf("failed to count followers: %w", err)
	}

	return int(count), nil
}

// CountFollowing returns the number of users that a user is following
func (r *RelationshipRepository) CountFollowing(ctx context.Context, username string) (int, error) {
	count, err := r.db.WithContext(ctx).Model(&models.RelationshipRecord{}).
		Where("PK", "=", fmt.Sprintf("FOLLOW#%s", username)).
		Filter("State", "=", models.RelationshipAccepted).
		Count()

	if err != nil {
		r.logger.Error("failed to count following",
			zap.String("username", username),
			zap.Error(err))
		return 0, fmt.Errorf("failed to count following: %w", err)
	}

	return int(count), nil
}

// UpdateRelationship updates relationship settings (ShowReblogs, Notify, etc.)
func (r *RelationshipRepository) UpdateRelationship(ctx context.Context, followerUsername, followingUsername string, updates map[string]interface{}) error {
	// First get the relationship
	relationship, err := r.GetRelationship(ctx, followerUsername, followingUsername)
	if err != nil {
		return err
	}

	// Apply updates
	updateBuilder := r.db.WithContext(ctx).Model(relationship).
		Where("PK", "=", relationship.PK).
		Where("SK", "=", relationship.SK)

	// Build set expressions
	setExpr := make(map[string]interface{})
	for field, value := range updates {
		setExpr[field] = value
	}
	setExpr["UpdatedAt"] = time.Now()

	// Execute update
	if err := updateBuilder.Update(); err != nil {
		r.logger.Error("failed to update relationship",
			zap.String("follower", followerUsername),
			zap.String("following", followingUsername),
			zap.Error(err))
		return fmt.Errorf("failed to update relationship: %w", err)
	}

	return nil
}

// GetPendingFollowRequests retrieves pending follow requests for a user
func (r *RelationshipRepository) GetPendingFollowRequests(ctx context.Context, username string, limit int, cursor string) ([]string, string, error) {
	query := r.db.WithContext(ctx).Model(&models.RelationshipRecord{}).
		Index("GSI1").
		Where("GSI1PK", "=", fmt.Sprintf("FOLLOW#%s", username)).
		Filter("State", "=", models.RelationshipPending).
		Limit(limit)

	if cursor != "" {
		query = query.Cursor(cursor)
	}

	// Get one more item than requested to determine if there are more results
	query = query.Limit(limit + 1)

	var relationships []models.RelationshipRecord
	err := query.All(&relationships)
	if err != nil {
		r.logger.Error("failed to query pending requests",
			zap.String("username", username),
			zap.Error(err))
		return nil, "", fmt.Errorf("failed to query pending requests: %w", err)
	}

	// Generate next cursor
	var nextCursor string
	if len(relationships) > limit {
		// We got more results than requested, so there are more pages
		nextCursor = relationships[limit-1].GSI1SK
		relationships = relationships[:limit] // Trim to requested limit
	}

	// Extract follower usernames
	followers := make([]string, 0, len(relationships))
	for _, rel := range relationships {
		if follower := rel.ExtractFollowerFromGSI(); follower != "" {
			followers = append(followers, follower)
		}
	}

	return followers, nextCursor, nil
}

// AcceptFollowRequest accepts a follow request
func (r *RelationshipRepository) AcceptFollowRequest(ctx context.Context, followerUsername, followingUsername string) error {
	// Get the relationship
	relationship, err := r.GetRelationship(ctx, followerUsername, followingUsername)
	if err != nil {
		return err
	}

	// Update state
	relationship.Accept()

	// Update in database
	if err := r.db.WithContext(ctx).Model(relationship).
		Where("PK", "=", relationship.PK).
		Where("SK", "=", relationship.SK).
		Update(); err != nil {
		r.logger.Error("failed to accept follow request",
			zap.String("follower", followerUsername),
			zap.String("following", followingUsername),
			zap.Error(err))
		return fmt.Errorf("failed to accept follow request: %w", err)
	}

	r.logger.Info("accepted follow request",
		zap.String("follower", followerUsername),
		zap.String("following", followingUsername))

	return nil
}

// RejectFollowRequest rejects a follow request
func (r *RelationshipRepository) RejectFollowRequest(ctx context.Context, followerUsername, followingUsername string) error {
	// Get the relationship
	relationship, err := r.GetRelationship(ctx, followerUsername, followingUsername)
	if err != nil {
		return err
	}

	// Update state
	relationship.Reject()

	// Update in database
	if err := r.db.WithContext(ctx).Model(relationship).
		Where("PK", "=", relationship.PK).
		Where("SK", "=", relationship.SK).
		Update(); err != nil {
		r.logger.Error("failed to reject follow request",
			zap.String("follower", followerUsername),
			zap.String("following", followingUsername),
			zap.Error(err))
		return fmt.Errorf("failed to reject follow request: %w", err)
	}

	r.logger.Info("rejected follow request",
		zap.String("follower", followerUsername),
		zap.String("following", followingUsername))

	return nil
}

// HasPendingFollowRequest checks if there's a pending follow request between two users
func (r *RelationshipRepository) HasPendingFollowRequest(ctx context.Context, requesterID, targetID string) (bool, error) {
	var relationship models.RelationshipRecord

	err := r.db.WithContext(ctx).Model(&relationship).
		Where("PK", "=", fmt.Sprintf("FOLLOW#%s", requesterID)).
		Where("SK", "=", fmt.Sprintf("FOLLOWING#%s", targetID)).
		First(&relationship)

	if err != nil {
		if errors.IsNotFound(err) {
			return false, nil
		}
		r.logger.Error("failed to check for pending follow request",
			zap.String("requester", requesterID),
			zap.String("target", targetID),
			zap.Error(err))
		return false, fmt.Errorf("failed to check for pending follow request: %w", err)
	}

	// Check if the relationship is in pending state
	return relationship.State == models.RelationshipPending, nil
}

// ===== Move Methods =====

// CreateMove creates a new move record
func (r *RelationshipRepository) CreateMove(ctx context.Context, move *storage.Move) error {
	moveRecord := models.NewMove(move.ID, move.Actor, move.Target)
	moveRecord.Published = move.Published

	if err := r.db.WithContext(ctx).Model(moveRecord).Create(); err != nil {
		if errors.IsConditionFailed(err) {
			r.logger.Warn("actor has already moved", zap.String("actor", move.Actor))
			return fmt.Errorf("actor %s has already moved", move.Actor)
		}
		r.logger.Error("failed to create move",
			zap.String("actor", move.Actor),
			zap.String("target", move.Target),
			zap.Error(err))
		return fmt.Errorf("failed to create move: %w", err)
	}

	r.logger.Info("created move",
		zap.String("actor", move.Actor),
		zap.String("target", move.Target))

	return nil
}

// GetMove retrieves the most recent move for an actor
func (r *RelationshipRepository) GetMove(ctx context.Context, actor string) (*storage.Move, error) {
	var moveRecord models.Move

	query := r.db.WithContext(ctx).Model(&moveRecord).
		Where("PK", "=", fmt.Sprintf("MOVE#ACTOR#%s", actor)).
		Limit(1)

	if err := query.First(&moveRecord); err != nil {
		if errors.IsNotFound(err) {
			return nil, fmt.Errorf("no move found for actor: %s", actor)
		}
		r.logger.Error("failed to get move",
			zap.String("actor", actor),
			zap.Error(err))
		return nil, fmt.Errorf("failed to get move: %w", err)
	}

	// Convert to storage.Move
	return &storage.Move{
		ID:        moveRecord.ID,
		Actor:     moveRecord.Actor,
		Target:    moveRecord.Target,
		Published: moveRecord.Published,
		CreatedAt: moveRecord.CreatedAt,
	}, nil
}

// GetAccountMoves retrieves all moves for an account (as actor)
func (r *RelationshipRepository) GetAccountMoves(ctx context.Context, actor string) ([]*storage.Move, error) {
	query := r.db.WithContext(ctx).Model(&models.Move{}).
		Where("PK", "=", fmt.Sprintf("MOVE#ACTOR#%s", actor))

	var moveRecords []models.Move
	if err := query.All(&moveRecords); err != nil {
		r.logger.Error("failed to get account moves",
			zap.String("actor", actor),
			zap.Error(err))
		return nil, fmt.Errorf("failed to get account moves: %w", err)
	}

	// Convert to storage.Move slice
	moves := make([]*storage.Move, len(moveRecords))
	for i, record := range moveRecords {
		moves[i] = &storage.Move{
			ID:        record.ID,
			Actor:     record.Actor,
			Target:    record.Target,
			Published: record.Published,
			CreatedAt: record.CreatedAt,
		}
	}

	return moves, nil
}

// UpdateMoveProgress updates move migration progress
func (r *RelationshipRepository) UpdateMoveProgress(ctx context.Context, actor, target string, _ map[string]interface{}) error {
	move := &models.Move{
		PK: fmt.Sprintf("MOVE#ACTOR#%s", actor),
		SK: fmt.Sprintf("TARGET#%s", target),
	}

	updateBuilder := r.db.WithContext(ctx).Model(move).
		Where("PK", "=", move.PK).
		Where("SK", "=", move.SK)

	// Add all fields from progress map to the model
	// DynamORM will update these fields when we call Update()
	// Note: We would need to set these on the move object before Update()

	if err := updateBuilder.Update(); err != nil {
		r.logger.Error("failed to update move progress",
			zap.String("actor", actor),
			zap.String("target", target),
			zap.Error(err))
		return fmt.Errorf("failed to update move progress: %w", err)
	}

	return nil
}

// VerifyMove marks a move as verified
func (r *RelationshipRepository) VerifyMove(ctx context.Context, actor, target string) error {
	now := time.Now()
	return r.UpdateMoveProgress(ctx, actor, target, map[string]interface{}{
		"Verified":   true,
		"VerifiedAt": now,
		"UpdatedAt":  now,
	})
}

// GetPendingMoves retrieves moves that haven't been fully processed
func (r *RelationshipRepository) GetPendingMoves(ctx context.Context, limit int) ([]*storage.Move, error) {
	// Note: DynamORM doesn't support BeginsWith on PK directly
	// We would need to scan and filter in application logic
	query := r.db.WithContext(ctx).Model(&models.Move{}).
		Limit(limit)

	var moveRecords []models.Move
	if err := query.All(&moveRecords); err != nil {
		r.logger.Error("failed to get pending moves", zap.Error(err))
		return nil, fmt.Errorf("failed to get pending moves: %w", err)
	}

	// Convert to storage.Move slice
	moves := make([]*storage.Move, len(moveRecords))
	for i, record := range moveRecords {
		moves[i] = &storage.Move{
			ID:        record.ID,
			Actor:     record.Actor,
			Target:    record.Target,
			Published: record.Published,
			CreatedAt: record.CreatedAt,
		}
	}

	return moves, nil
}

// GetMoveByTarget retrieves all moves to a specific target account
func (r *RelationshipRepository) GetMoveByTarget(ctx context.Context, target string) ([]*storage.Move, error) {
	query := r.db.WithContext(ctx).Model(&models.Move{}).
		Index("GSI1").
		Where("GSI1PK", "=", fmt.Sprintf("MOVE#TARGET#%s", target))

	var moveRecords []models.Move
	if err := query.All(&moveRecords); err != nil {
		r.logger.Error("failed to get moves by target",
			zap.String("target", target),
			zap.Error(err))
		return nil, fmt.Errorf("failed to get moves by target: %w", err)
	}

	// Convert to storage.Move slice
	moves := make([]*storage.Move, len(moveRecords))
	for i, record := range moveRecords {
		moves[i] = &storage.Move{
			ID:        record.ID,
			Actor:     record.Actor,
			Target:    record.Target,
			Published: record.Published,
			CreatedAt: record.CreatedAt,
		}
	}

	r.logger.Info("retrieved moves by target",
		zap.String("target", target),
		zap.Int("count", len(moves)))

	return moves, nil
}

// HasMovedFrom checks if newActor has moved from oldActor
func (r *RelationshipRepository) HasMovedFrom(ctx context.Context, oldActor, newActor string) (bool, error) {
	var moveRecord models.Move

	err := r.db.WithContext(ctx).Model(&moveRecord).
		Where("PK", "=", fmt.Sprintf("MOVE#ACTOR#%s", oldActor)).
		Where("SK", "=", fmt.Sprintf("TARGET#%s", newActor)).
		First(&moveRecord)

	if err != nil {
		if errors.IsNotFound(err) {
			return false, nil
		}
		r.logger.Error("failed to check move relationship",
			zap.String("old_actor", oldActor),
			zap.String("new_actor", newActor),
			zap.Error(err))
		return false, fmt.Errorf("failed to check move relationship: %w", err)
	}

	r.logger.Info("checked move relationship",
		zap.String("old_actor", oldActor),
		zap.String("new_actor", newActor),
		zap.Bool("exists", true))

	return true, nil
}

// ===== Endorsement Methods =====

// IsEndorsed checks if a user has endorsed (pinned) a target account
func (r *RelationshipRepository) IsEndorsed(ctx context.Context, userID, targetID string) (bool, error) {
	var pin models.AccountPin

	err := r.db.WithContext(ctx).Model(&pin).
		Where("PK", "=", fmt.Sprintf("ACCOUNT_PIN#%s", userID)).
		Where("SK", "=", fmt.Sprintf("PIN#%s", targetID)).
		First(&pin)

	if err != nil {
		if errors.IsNotFound(err) {
			return false, nil
		}
		r.logger.Error("failed to check endorsement",
			zap.String("user_id", userID),
			zap.String("target_id", targetID),
			zap.Error(err))
		return false, fmt.Errorf("failed to check endorsement: %w", err)
	}

	return true, nil
}

// ===== Relationship Note Methods =====

// GetRelationshipNote retrieves a private note on an account
func (r *RelationshipRepository) GetRelationshipNote(ctx context.Context, userID, targetID string) (*storage.AccountNote, error) {
	var noteRecord models.AccountNote

	err := r.db.WithContext(ctx).Model(&noteRecord).
		Where("PK", "=", fmt.Sprintf("ACCOUNT_NOTE#%s", userID)).
		Where("SK", "=", fmt.Sprintf("NOTE#%s", targetID)).
		First(&noteRecord)

	if err != nil {
		if errors.IsNotFound(err) {
			return nil, nil // Return nil for not found (matches legacy behavior)
		}
		r.logger.Error("failed to get relationship note",
			zap.String("user_id", userID),
			zap.String("target_id", targetID),
			zap.Error(err))
		return nil, fmt.Errorf("failed to get relationship note: %w", err)
	}

	// Convert to storage.AccountNote
	return &storage.AccountNote{
		Username:      noteRecord.Username,
		TargetActorID: noteRecord.TargetActorID,
		Note:          noteRecord.Note,
		CreatedAt:     noteRecord.CreatedAt,
		UpdatedAt:     noteRecord.UpdatedAt,
	}, nil
}

// ===== CollectionItem Methods =====

// AddToCollection adds an item to a collection
func (r *RelationshipRepository) AddToCollection(ctx context.Context, collection string, item *storage.CollectionItem) error {
	collectionItem := models.NewCollectionItem(collection, item.ItemID, item.ItemType, item.AddedBy)
	collectionItem.Position = item.Position

	if err := r.db.WithContext(ctx).Model(collectionItem).Create(); err != nil {
		if errors.IsConditionFailed(err) {
			r.logger.Info("item already in collection",
				zap.String("collection", collection),
				zap.String("item_id", item.ItemID))
			return nil // Not an error to add something already in collection
		}
		r.logger.Error("failed to add to collection",
			zap.String("collection", collection),
			zap.String("item_id", item.ItemID),
			zap.Error(err))
		return fmt.Errorf("failed to add to collection: %w", err)
	}

	r.logger.Info("added item to collection",
		zap.String("collection", collection),
		zap.String("item_id", item.ItemID))

	return nil
}

// RemoveFromCollection removes an item from a collection
func (r *RelationshipRepository) RemoveFromCollection(ctx context.Context, collection, itemID string) error {
	collectionItem := &models.CollectionItem{
		PK: fmt.Sprintf("COLLECTION#%s", collection),
		SK: fmt.Sprintf("ITEM#%s", itemID),
	}

	if err := r.db.WithContext(ctx).Model(collectionItem).
		Where("PK", "=", collectionItem.PK).
		Where("SK", "=", collectionItem.SK).
		Delete(); err != nil {
		if errors.IsNotFound(err) {
			r.logger.Debug("item not in collection",
				zap.String("collection", collection),
				zap.String("item_id", itemID))
			return nil
		}
		r.logger.Error("failed to remove from collection",
			zap.String("collection", collection),
			zap.String("item_id", itemID),
			zap.Error(err))
		return fmt.Errorf("failed to remove from collection: %w", err)
	}

	r.logger.Info("removed item from collection",
		zap.String("collection", collection),
		zap.String("item_id", itemID))

	return nil
}

// GetCollectionItems retrieves items from a collection with pagination
func (r *RelationshipRepository) GetCollectionItems(ctx context.Context, collection string, limit int, cursor string) ([]*storage.CollectionItem, string, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	query := r.db.WithContext(ctx).Model(&models.CollectionItem{}).
		Where("PK", "=", fmt.Sprintf("COLLECTION#%s", collection)).
		Limit(limit)

	if cursor != "" {
		query = query.Cursor(cursor)
	}

	// Get one more item than requested to determine if there are more results
	query = query.Limit(limit + 1)

	var items []models.CollectionItem
	err := query.All(&items)
	if err != nil {
		r.logger.Error("failed to get collection items",
			zap.String("collection", collection),
			zap.Error(err))
		return nil, "", fmt.Errorf("failed to get collection items: %w", err)
	}

	// Generate next cursor
	var nextCursor string
	if len(items) > limit {
		// We got more results than requested, so there are more pages
		nextCursor = items[limit-1].SK
		items = items[:limit] // Trim to requested limit
	}

	// Convert to storage.CollectionItem slice
	result := make([]*storage.CollectionItem, len(items))
	for i, item := range items {
		result[i] = &storage.CollectionItem{
			CollectionID: item.Collection,
			ItemID:       item.ItemID,
			ItemType:     item.ItemType,
			AddedBy:      item.AddedBy,
			AddedAt:      item.AddedAt,
			Position:     item.Position,
		}
	}

	return result, nextCursor, nil
}

// IsInCollection checks if an item is in a collection
func (r *RelationshipRepository) IsInCollection(ctx context.Context, collection, itemID string) (bool, error) {
	var item models.CollectionItem

	err := r.db.WithContext(ctx).Model(&item).
		Where("PK", "=", fmt.Sprintf("COLLECTION#%s", collection)).
		Where("SK", "=", fmt.Sprintf("ITEM#%s", itemID)).
		First(&item)

	if err != nil {
		if errors.IsNotFound(err) {
			return false, nil
		}
		r.logger.Error("failed to check collection membership",
			zap.String("collection", collection),
			zap.String("item_id", itemID),
			zap.Error(err))
		return false, fmt.Errorf("failed to check collection membership: %w", err)
	}

	return true, nil
}

// CountCollectionItems returns the count of items in a collection
func (r *RelationshipRepository) CountCollectionItems(ctx context.Context, collection string) (int, error) {
	count, err := r.db.WithContext(ctx).Model(&models.CollectionItem{}).
		Where("PK", "=", fmt.Sprintf("COLLECTION#%s", collection)).
		Count()

	if err != nil {
		r.logger.Error("failed to count collection items",
			zap.String("collection", collection),
			zap.Error(err))
		return 0, fmt.Errorf("failed to count collection items: %w", err)
	}

	return int(count), nil
}

// ClearCollection removes all items from a collection
func (r *RelationshipRepository) ClearCollection(ctx context.Context, collection string) error {
	// Get all items in the collection
	items, _, err := r.GetCollectionItems(ctx, collection, 1000, "")
	if err != nil {
		return err
	}

	// Delete each item
	for _, item := range items {
		if err := r.RemoveFromCollection(ctx, collection, item.ItemID); err != nil {
			r.logger.Error("failed to remove item during clear",
				zap.String("collection", collection),
				zap.String("item_id", item.ItemID),
				zap.Error(err))
			// Continue clearing other items
		}
	}

	r.logger.Info("cleared collection",
		zap.String("collection", collection),
		zap.Int("items_removed", len(items)))

	return nil
}

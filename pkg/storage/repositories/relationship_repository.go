package repositories

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/cost"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/dynamorm/pkg/errors"
	"go.uber.org/zap"
)

// RelationshipRepository implements relationship operations using enhanced DynamORM patterns
type RelationshipRepository struct {
	*EnhancedBaseRepository[*models.RelationshipRecord]
	// Repository dependencies for complex relationship operations
	blockRepo  *BlockRepository
	muteRepo   *MuteRepository
	socialRepo *SocialRepository
	queryUtils *QueryUtils
}

// NewRelationshipRepository creates a new relationship repository with enhanced functionality
func NewRelationshipRepository(db core.DB, tableName string, logger *zap.Logger) *RelationshipRepository {
	// Create enhanced repository with social-specific services
	enhancedRepo := NewEnhancedBaseRepository[*models.RelationshipRecord](db, tableName, logger, nil, "RelationshipRepository", "relationship")

	// Set up enhanced services optimized for social relationships
	enhancedRepo.SetValidationService(NewDefaultValidationService())
	enhancedRepo.SetPermissionService(NewDefaultPermissionService())
	enhancedRepo.SetCachingService(NewInMemoryCachingService()) // Relationships are frequently accessed
	enhancedRepo.SetEventService(NewDefaultEventService())      // Critical for federation events

	return &RelationshipRepository{
		EnhancedBaseRepository: enhancedRepo,
		// Initialize repository dependencies
		blockRepo:  NewBlockRepository(db, tableName, logger, nil),
		muteRepo:   NewMuteRepository(db, tableName, logger, nil),
		socialRepo: NewSocialRepository(db, tableName, logger, nil),
		queryUtils: NewQueryUtils(db, logger),
	}
}

// NewRelationshipRepositoryWithCostTracking creates a new relationship repository with cost tracking
func NewRelationshipRepositoryWithCostTracking(db core.DB, tableName string, logger *zap.Logger, costService *cost.TrackingService) *RelationshipRepository {
	// Create enhanced repository with cost tracking and full service integration
	enhancedRepo := NewEnhancedBaseRepository[*models.RelationshipRecord](db, tableName, logger, costService, "RelationshipRepository", "relationship")

	// Set up enhanced services optimized for social relationships with cost tracking
	enhancedRepo.SetValidationService(NewDefaultValidationService())
	enhancedRepo.SetPermissionService(NewDefaultPermissionService())
	enhancedRepo.SetCachingService(NewInMemoryCachingService()) // Relationships are frequently accessed
	enhancedRepo.SetEventService(NewDefaultEventService())      // Critical for federation events

	return &RelationshipRepository{
		EnhancedBaseRepository: enhancedRepo,
		// Initialize repository dependencies with cost tracking
		blockRepo:  NewBlockRepository(db, tableName, logger, costService),
		muteRepo:   NewMuteRepository(db, tableName, logger, costService),
		socialRepo: NewSocialRepository(db, tableName, logger, costService),
		queryUtils: NewQueryUtils(db, logger),
	}
}

// ===== RelationshipRecord Methods =====

// GetFollowRequest gets a follow request by follower and target IDs
func (r *RelationshipRepository) GetFollowRequest(ctx context.Context, followerID, targetID string) (*storage.RelationshipRecord, error) {
	var relationship models.RelationshipRecord

	pk := fmt.Sprintf("FOLLOW#%s", followerID)
	sk := fmt.Sprintf("FOLLOWING#%s", targetID)

	err := r.Get(ctx, pk, sk, &relationship)
	if err != nil {
		if fmt.Sprintf("%v", err) == fmt.Sprintf("item not found: pk=%s, sk=%s", pk, sk) {
			return nil, ErrorHandler.HandleGetError(err, EntityFollow, "not found")
		}
		return nil, ErrorHandler.HandleGetError(err, EntityFollow, "request")
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
	pk := fmt.Sprintf("FOLLOW#%s", requesterID)
	sk := fmt.Sprintf("FOLLOWING#%s", targetID)

	exists, err := r.Exists(ctx, pk, sk)
	if err != nil {
		return false, ErrorHandler.HandleQueryError(err, "follow request", "check")
	}

	return exists, nil
}

// CreateRelationship creates a new follow relationship with enhanced validation and events
func (r *RelationshipRepository) CreateRelationship(ctx context.Context, followerUsername, followingUsername, activityID string) error {
	relationship := models.NewRelationshipRecord(followerUsername, followingUsername, activityID)

	// Use enhanced validation and creation with automatic permission checking and event emission
	if err := r.ValidateAndCreate(ctx, relationship); err != nil {
		// Check if it's a duplicate key error
		if errors.IsConditionFailed(err) {
			r.logger.Debug("follow relationship already exists",
				zap.String("follower", followerUsername),
				zap.String("following", followingUsername),
				zap.Bool("validation_enabled", r.HasValidation()),
				zap.Bool("events_enabled", r.HasEvents()))
			return nil
		}
		return ErrorHandler.HandleCreateError(err, EntityFollow, followerUsername)
	}

	r.logger.Info("created follow relationship with enhanced patterns",
		zap.String("follower", followerUsername),
		zap.String("following", followingUsername),
		zap.String("activity_id", activityID),
		zap.Bool("validation_enabled", r.HasValidation()),
		zap.Bool("events_enabled", r.HasEvents()),
		zap.Bool("caching_enabled", r.HasCaching()))

	return nil
}

// DeleteRelationship removes a follow relationship
func (r *RelationshipRepository) DeleteRelationship(ctx context.Context, followerUsername, followingUsername string) error {
	pk := fmt.Sprintf("FOLLOW#%s", followerUsername)
	sk := fmt.Sprintf("FOLLOWING#%s", followingUsername)

	identifiers := map[string]string{
		"follower":  followerUsername,
		"following": followingUsername,
	}

	return DeleteEntityWithLogging(ctx, r.BaseRepository, pk, sk, "relationship", identifiers)
}

// GetRelationship retrieves a specific follow relationship
func (r *RelationshipRepository) GetRelationship(ctx context.Context, followerUsername, followingUsername string) (*models.RelationshipRecord, error) {
	var relationship models.RelationshipRecord

	pk := fmt.Sprintf("FOLLOW#%s", followerUsername)
	sk := fmt.Sprintf("FOLLOWING#%s", followingUsername)

	err := r.Get(ctx, pk, sk, &relationship)
	if err != nil {
		if fmt.Sprintf("%v", err) == fmt.Sprintf("item not found: pk=%s, sk=%s", pk, sk) {
			return nil, ErrorHandler.HandleGetError(err, EntityFollow, "not found")
		}
		return nil, ErrorHandler.HandleGetError(err, EntityFollow, followerUsername)
	}

	return &relationship, nil
}

// getRelationshipsByState retrieves relationships for a user filtered by state
func (r *RelationshipRepository) getRelationshipsByState(ctx context.Context, username string, state models.RelationshipState, limit int, cursor string, errorContext string) ([]string, string, error) {
	query := r.db.WithContext(ctx).Model(&models.RelationshipRecord{}).
		Index("GSI1").
		Where("GSI1PK", "=", fmt.Sprintf("FOLLOW#%s", username)).
		Filter("State", "=", state).
		Limit(limit)

	if cursor != "" {
		query = query.Cursor(cursor)
	}

	// Get one more item than requested to determine if there are more results
	query = query.Limit(limit + 1)

	var relationships []models.RelationshipRecord
	err := query.All(&relationships)
	if err != nil {
		r.logger.Error(fmt.Sprintf("failed to query %s", errorContext),
			zap.String("username", username),
			zap.Error(err))
		return nil, "", ErrorHandler.HandleQueryError(err, EntityFollow, errorContext)
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

// GetFollowers retrieves all followers for a user
func (r *RelationshipRepository) GetFollowers(ctx context.Context, username string, limit int, cursor string) ([]string, string, error) {
	return r.getRelationshipsByState(ctx, username, models.RelationshipAccepted, limit, cursor, "followers")
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
		return nil, "", ErrorHandler.HandleQueryError(err, EntityFollow, "following")
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
	// Note: This uses GSI and filter, so we keep the custom implementation
	// BaseRepository doesn't have a method for filtered counts on GSI
	count, err := r.db.WithContext(ctx).Model(&models.RelationshipRecord{}).
		Index("GSI1").
		Where("GSI1PK", "=", fmt.Sprintf("FOLLOW#%s", username)).
		Filter("State", "=", models.RelationshipAccepted).
		Count()

	if err != nil {
		r.logger.Error("failed to count followers",
			zap.String("username", username),
			zap.Error(err))
		return 0, ErrorHandler.HandleQueryError(err, EntityFollow, "count followers")
	}

	return int(count), nil
}

// CountFollowing returns the number of users that a user is following
func (r *RelationshipRepository) CountFollowing(ctx context.Context, username string) (int, error) {
	// Note: This uses filter, so we keep the custom implementation
	// BaseRepository doesn't have a method for filtered counts
	count, err := r.db.WithContext(ctx).Model(&models.RelationshipRecord{}).
		Where("PK", "=", fmt.Sprintf("FOLLOW#%s", username)).
		Filter("State", "=", models.RelationshipAccepted).
		Count()

	if err != nil {
		r.logger.Error("failed to count following",
			zap.String("username", username),
			zap.Error(err))
		return 0, ErrorHandler.HandleQueryError(err, EntityFollow, "count following")
	}

	return int(count), nil
}

// GetFollowerCount returns the number of followers for a user (interface method)
func (r *RelationshipRepository) GetFollowerCount(ctx context.Context, userID string) (int64, error) {
	count, err := r.CountFollowers(ctx, userID)
	if err != nil {
		return 0, err
	}
	return int64(count), nil
}

// GetFollowingCount returns the number of users that a user is following (interface method)
func (r *RelationshipRepository) GetFollowingCount(ctx context.Context, userID string) (int64, error) {
	count, err := r.CountFollowing(ctx, userID)
	if err != nil {
		return 0, err
	}
	return int64(count), nil
}

// CountRelationshipsByDomain counts follower/following relationships involving a remote domain
// Returns (followers from domain, following to domain, error)
// Uses domain-aware GSIs (GSI2, GSI3) for O(1) indexed queries instead of full table scan
func (r *RelationshipRepository) CountRelationshipsByDomain(ctx context.Context, domain string) (followers, following int, err error) {
	if domain == "" {
		return 0, 0, fmt.Errorf("domain cannot be empty")
	}

	followerKey := fmt.Sprintf("FOLLOWER_DOMAIN#%s", domain)
	followingKey := fmt.Sprintf("FOLLOWING_DOMAIN#%s", domain)

	// Count followers: remote users from this domain following local users
	// Uses GSI2: FOLLOWER_DOMAIN#{domain} → FOLLOWING#{username}
	followerCount, err := r.db.WithContext(ctx).Model(&models.RelationshipRecord{}).
		Index("gsi2").
		Where("GSI2PK", "=", followerKey).
		Filter("State", "=", models.RelationshipAccepted).
		Count()

	if err != nil && !errors.IsNotFound(err) {
		r.logger.Error("failed to count followers by domain",
			zap.String("domain", domain),
			zap.Error(err))
		return 0, 0, fmt.Errorf("count followers by domain: %w", err)
	}

	// Count following: local users following remote users from this domain
	// Uses GSI3: FOLLOWING_DOMAIN#{domain} → FOLLOWER#{username}
	followingCount, err := r.db.WithContext(ctx).Model(&models.RelationshipRecord{}).
		Index("gsi3").
		Where("GSI3PK", "=", followingKey).
		Filter("State", "=", models.RelationshipAccepted).
		Count()

	if err != nil && !errors.IsNotFound(err) {
		r.logger.Error("failed to count following by domain",
			zap.String("domain", domain),
			zap.Error(err))
		return 0, 0, fmt.Errorf("count following by domain: %w", err)
	}

	r.logger.Info("counted relationships by domain (GSI query)",
		zap.String("domain", domain),
		zap.Int("followers", int(followerCount)),
		zap.Int("following", int(followingCount)))

	return int(followerCount), int(followingCount), nil
}

// UpdateRelationship updates relationship settings using enhanced validation and events
func (r *RelationshipRepository) UpdateRelationship(ctx context.Context, followerUsername, followingUsername string, updates map[string]interface{}) error {
	// First get the relationship
	relationship, err := r.GetRelationship(ctx, followerUsername, followingUsername)
	if err != nil {
		return err
	}

	// Apply updates based on the provided updates map
	hasChanges := r.applyRelationshipUpdates(relationship, updates, followerUsername, followingUsername)

	if !hasChanges {
		r.logger.Debug("no changes to apply to relationship",
			zap.String("follower", followerUsername),
			zap.String("following", followingUsername))
		return nil
	}

	relationship.UpdatedAt = time.Now()

	// Use enhanced validation and update with automatic event emission and cache invalidation
	if err := r.ValidateAndUpdate(ctx, relationship); err != nil {
		r.logger.Error("failed to update relationship with enhanced validation",
			zap.String("follower", followerUsername),
			zap.String("following", followingUsername),
			zap.Bool("validation_enabled", r.HasValidation()),
			zap.Bool("events_enabled", r.HasEvents()),
			zap.Error(err))
		return ErrorHandler.HandleUpdateError(err, EntityFollow, followerUsername)
	}

	return nil
}

// applyRelationshipUpdates applies updates to a relationship record and returns true if any changes were made
func (r *RelationshipRepository) applyRelationshipUpdates(relationship *models.RelationshipRecord, updates map[string]interface{}, followerUsername, followingUsername string) bool {
	var hasChanges bool

	for key, value := range updates {
		changed := r.applyFieldUpdate(relationship, key, value, followerUsername, followingUsername)
		if changed {
			hasChanges = true
		}
	}

	return hasChanges
}

// applyFieldUpdate applies a single field update and returns true if the field was changed
func (r *RelationshipRepository) applyFieldUpdate(relationship *models.RelationshipRecord, key string, value interface{}, followerUsername, followingUsername string) bool {
	switch key {
	case "state", "State":
		return r.updateStringField(&relationship.State, value)
	case "activity_id", "ActivityID":
		return r.updateStringField(&relationship.ActivityID, value)
	case "notifying", "Notifying":
		return r.updateBoolField(&relationship.Notifying, value)
	case "showing_reblogs", "ShowingReblogs":
		return r.updateBoolField(&relationship.ShowingReblogs, value)
	case "languages", "Languages":
		return r.updateLanguagesField(&relationship.Languages, value)
	case "note", "Note":
		return r.updateStringField(&relationship.Note, value)
	default:
		r.logger.Warn("unsupported relationship field update",
			zap.String("field", key),
			zap.Any("value", value),
			zap.String("follower", followerUsername),
			zap.String("following", followingUsername))
		return false
	}
}

// updateStringField updates a string field if the value differs
func (r *RelationshipRepository) updateStringField(field *string, value interface{}) bool {
	if strValue, ok := value.(string); ok && strValue != *field {
		*field = strValue
		return true
	}
	return false
}

// updateBoolField updates a boolean field if the value differs
func (r *RelationshipRepository) updateBoolField(field *bool, value interface{}) bool {
	if boolValue, ok := value.(bool); ok && boolValue != *field {
		*field = boolValue
		return true
	}
	return false
}

// updateLanguagesField updates a languages slice field if the value differs
func (r *RelationshipRepository) updateLanguagesField(field *[]string, value interface{}) bool {
	langValue, ok := value.([]string)
	if !ok {
		return false
	}

	// Compare slices
	if len(langValue) != len(*field) {
		*field = langValue
		return true
	}

	// Check if any elements differ
	for i, lang := range langValue {
		if i >= len(*field) || lang != (*field)[i] {
			*field = langValue
			return true
		}
	}

	return false
}

// GetPendingFollowRequests retrieves pending follow requests for a user
func (r *RelationshipRepository) GetPendingFollowRequests(ctx context.Context, username string, limit int, cursor string) ([]string, string, error) {
	return r.getRelationshipsByState(ctx, username, models.RelationshipPending, limit, cursor, "pending requests")
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

	// Update in database using BaseRepository
	if err := r.Update(ctx, relationship); err != nil {
		return ErrorHandler.HandleUpdateError(err, "follow request", "accept")
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

	// Update in database using BaseRepository
	if err := r.Update(ctx, relationship); err != nil {
		return ErrorHandler.HandleUpdateError(err, "follow request", "reject")
	}

	r.logger.Info("rejected follow request",
		zap.String("follower", followerUsername),
		zap.String("following", followingUsername))

	return nil
}

// HasPendingFollowRequest checks if there's a pending follow request between two users
func (r *RelationshipRepository) HasPendingFollowRequest(ctx context.Context, requesterID, targetID string) (bool, error) {
	var relationship models.RelationshipRecord

	pk := fmt.Sprintf("FOLLOW#%s", requesterID)
	sk := fmt.Sprintf("FOLLOWING#%s", targetID)

	err := r.Get(ctx, pk, sk, &relationship)
	if err != nil {
		if fmt.Sprintf("%v", err) == fmt.Sprintf("item not found: pk=%s, sk=%s", pk, sk) {
			return false, nil
		}
		return false, ErrorHandler.HandleQueryError(err, EntityFollow, "pending check")
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
			return ErrorHandler.HandleCreateError(err, "move", move.Actor)
		}
		r.logger.Error("failed to create move",
			zap.String("actor", move.Actor),
			zap.String("target", move.Target),
			zap.Error(err))
		return ErrorHandler.HandleCreateError(err, "move", move.Actor)
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
			return nil, ErrorHandler.HandleGetError(err, "move", actor)
		}
		r.logger.Error("failed to get move",
			zap.String("actor", actor),
			zap.Error(err))
		return nil, ErrorHandler.HandleGetError(err, "move", actor)
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
		return nil, ErrorHandler.HandleQueryError(err, "move", actor)
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
		return ErrorHandler.HandleUpdateError(err, "move", actor)
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
		return nil, ErrorHandler.HandleQueryError(err, "move", "pending")
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
		return nil, ErrorHandler.HandleQueryError(err, "move", target)
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
		return false, ErrorHandler.HandleQueryError(err, "move", "relationship")
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
		return false, ErrorHandler.HandleQueryError(err, "endorsement", targetID)
	}

	return true, nil
}

// CreateEndorsement creates a new endorsement (account pin) relationship
func (r *RelationshipRepository) CreateEndorsement(ctx context.Context, endorsement *storage.AccountPin) error {
	// Validate that the endorser follows the endorsed account
	// Extract usernames from actor IDs if needed
	endorserUsername := r.extractUsernameFromID(endorsement.Username)
	endorsedActorID := endorsement.PinnedActorID

	// Check if endorser follows the endorsed account
	isFollowing, err := r.IsFollowing(ctx, endorserUsername, endorsedActorID)
	if err != nil {
		r.logger.Error("failed to check follow relationship for endorsement",
			zap.String("endorser", endorserUsername),
			zap.String("endorsed", endorsedActorID),
			zap.Error(err))
		return ErrorHandler.HandleQueryError(err, EntityFollow, "endorsement check")
	}

	if !isFollowing {
		return ErrorHandler.HandleCreateError(nil, "endorsement", "not following")
	}

	// Check current endorsement count (Mastodon typically allows 4 endorsements)
	currentPins, err := r.socialRepo.GetAccountPins(ctx, endorserUsername)
	if err != nil {
		r.logger.Error("failed to get current endorsements",
			zap.String("endorser", endorserUsername),
			zap.Error(err))
		return ErrorHandler.HandleQueryError(err, "endorsement", "current")
	}

	if len(currentPins) >= 4 {
		return ErrorHandler.HandleCreateError(nil, "endorsement", "limit reached")
	}

	// Create the endorsement using social repository
	err = r.socialRepo.CreateAccountPin(ctx, endorsement)
	if err != nil {
		r.logger.Error("failed to create endorsement",
			zap.String("endorser", endorserUsername),
			zap.String("endorsed", endorsedActorID),
			zap.Error(err))
		return ErrorHandler.HandleCreateError(err, "endorsement", endorserUsername)
	}

	r.logger.Info("created endorsement",
		zap.String("endorser", endorserUsername),
		zap.String("endorsed", endorsedActorID))

	return nil
}

// DeleteEndorsement removes an endorsement (account pin) relationship
func (r *RelationshipRepository) DeleteEndorsement(ctx context.Context, endorserID, endorsedID string) error {
	// Extract username from endorser ID if needed
	endorserUsername := r.extractUsernameFromID(endorserID)

	// Delete the endorsement using social repository
	err := r.socialRepo.DeleteAccountPin(ctx, endorserUsername, endorsedID)
	if err != nil {
		r.logger.Error("failed to delete endorsement",
			zap.String("endorser", endorserUsername),
			zap.String("endorsed", endorsedID),
			zap.Error(err))
		return ErrorHandler.HandleDeleteError(err, "endorsement", endorserUsername)
	}

	r.logger.Info("deleted endorsement",
		zap.String("endorser", endorserUsername),
		zap.String("endorsed", endorsedID))

	return nil
}

// GetEndorsements retrieves all endorsements (account pins) for a user
func (r *RelationshipRepository) GetEndorsements(ctx context.Context, userID string, limit int, cursor string) ([]*storage.AccountPin, string, error) {
	// Extract username from user ID if needed
	username := r.extractUsernameFromID(userID)

	// Use social repository to get endorsements with pagination
	endorsements, nextCursor, err := r.socialRepo.GetAccountPinsPaginated(ctx, username, limit, cursor)
	if err != nil {
		r.logger.Error("failed to get endorsements",
			zap.String("user", username),
			zap.Error(err))
		return nil, "", ErrorHandler.HandleQueryError(err, "endorsement", username)
	}

	return endorsements, nextCursor, nil
}

// IsFollowing checks if followerUsername is following the targetActorID
func (r *RelationshipRepository) IsFollowing(ctx context.Context, followerUsername, targetActorID string) (bool, error) {
	// Extract target username from actor ID
	targetUsername := r.extractUsernameFromID(targetActorID)

	// Check relationship
	relationship, err := r.GetRelationship(ctx, followerUsername, targetUsername)
	if err != nil {
		// If relationship not found, user is not following
		if fmt.Sprintf("%v", err) == "follow relationship not found" {
			return false, nil
		}
		return false, err
	}

	// Check if relationship is accepted
	return relationship.State == models.RelationshipAccepted, nil
}

// extractUsernameFromID extracts username from an actor ID or returns the input if it's already a username
func (r *RelationshipRepository) extractUsernameFromID(actorIDOrUsername string) string {
	// If it contains "https://" or has slashes, it's an actor ID, extract username
	if strings.Contains(actorIDOrUsername, "://") || strings.Contains(actorIDOrUsername, "/") {
		parts := strings.Split(actorIDOrUsername, "/")
		if len(parts) > 0 {
			return parts[len(parts)-1]
		}
	}
	// Otherwise it's already a username
	return actorIDOrUsername
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
			return nil, ErrorHandler.HandleGetError(storage.ErrNotFound, EntityAccountNote, targetID)
		}
		r.logger.Error("failed to get relationship note",
			zap.String("user_id", userID),
			zap.String("target_id", targetID),
			zap.Error(err))
		return nil, ErrorHandler.HandleGetError(err, EntityAccountNote, targetID)
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
	return r.queryUtils.AddToCollectionHelper(ctx, collection, item, r.db)
}

// RemoveFromCollection removes an item from a collection
func (r *RelationshipRepository) RemoveFromCollection(ctx context.Context, collection, itemID string) error {
	pk := fmt.Sprintf("COLLECTION#%s", collection)
	sk := fmt.Sprintf("ITEM#%s", itemID)

	// Note: This method would ideally be in a BaseRepository[*models.CollectionItem]
	// For now, we use a custom delete with similar logging pattern
	var model models.CollectionItem
	err := r.db.WithContext(ctx).Model(&model).
		Where("PK", "=", pk).
		Where("SK", "=", sk).
		Delete()

	if err != nil {
		if errors.IsNotFound(err) {
			r.logger.Debug("item from collection not found",
				zap.String("collection", collection),
				zap.String("item_id", itemID))
			return nil
		}
		r.logger.Error("failed to delete item from collection",
			zap.String("collection", collection),
			zap.String("item_id", itemID),
			zap.Error(err))
		return ErrorHandler.HandleDeleteError(err, "collection", itemID)
	}

	r.logger.Info("deleted item from collection",
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
		return nil, "", ErrorHandler.HandleQueryError(err, "collection", collection)
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
		return false, ErrorHandler.HandleQueryError(err, "collection", itemID)
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
		return 0, ErrorHandler.HandleQueryError(err, "collection", "count")
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

// ===== Block Methods =====

// CreateBlock creates a new block relationship
func (r *RelationshipRepository) CreateBlock(ctx context.Context, blockerActor, blockedActor, activityID string) error {
	return r.blockRepo.CreateBlock(ctx, blockerActor, blockedActor, activityID)
}

// DeleteBlock removes a block relationship (for Undo Block)
func (r *RelationshipRepository) DeleteBlock(ctx context.Context, blockerActor, blockedActor string) error {
	return r.blockRepo.DeleteBlock(ctx, blockerActor, blockedActor)
}

// Unfollow removes a follow relationship (wrapper for DeleteRelationship)
func (r *RelationshipRepository) Unfollow(ctx context.Context, followerID, followingID string) error {
	return r.DeleteRelationship(ctx, followerID, followingID)
}

// UnblockUser removes a block relationship (wrapper for DeleteBlock)
func (r *RelationshipRepository) UnblockUser(ctx context.Context, blockerID, blockedID string) error {
	return r.DeleteBlock(ctx, blockerID, blockedID)
}

// UnmuteUser removes a mute relationship (wrapper for DeleteMute)
func (r *RelationshipRepository) UnmuteUser(ctx context.Context, muterID, mutedID string) error {
	return r.DeleteMute(ctx, muterID, mutedID)
}

// BlockUser blocks another user
func (r *RelationshipRepository) BlockUser(ctx context.Context, blockerID, blockedID string) error {
	// Validate the relationship using centralized validation
	if err := common.ValidateRepositoryRelationship(blockerID, blockedID, "block"); err != nil {
		return ErrorHandler.HandleCreateError(err, EntityBlock, blockerID)
	}

	// Generate a unique activity ID for the block action
	activityID := fmt.Sprintf("block_%s_%s_%d", blockerID, blockedID, time.Now().Unix())
	return r.blockRepo.CreateBlock(ctx, blockerID, blockedID, activityID)
}

// IsBlocked checks if one actor has blocked another
func (r *RelationshipRepository) IsBlocked(ctx context.Context, blockerActor, blockedActor string) (bool, error) {
	return r.blockRepo.IsBlocked(ctx, blockerActor, blockedActor)
}

// IsBlockedBidirectional checks if either actor has blocked the other
func (r *RelationshipRepository) IsBlockedBidirectional(ctx context.Context, actor1, actor2 string) (bool, error) {
	return r.blockRepo.IsBlockedBidirectional(ctx, actor1, actor2)
}

// GetBlockedUsers returns a list of users blocked by the given actor
func (r *RelationshipRepository) GetBlockedUsers(ctx context.Context, blockerActor string, limit int, cursor string) ([]string, string, error) {
	return r.blockRepo.GetBlockedUsers(ctx, blockerActor, limit, cursor)
}

// GetUsersWhoBlocked returns a list of users who have blocked the given actor
func (r *RelationshipRepository) GetUsersWhoBlocked(ctx context.Context, blockedActor string, limit int, cursor string) ([]string, string, error) {
	return r.blockRepo.GetUsersWhoBlocked(ctx, blockedActor, limit, cursor)
}

// GetBlock retrieves a specific block relationship
func (r *RelationshipRepository) GetBlock(ctx context.Context, blockerActor, blockedActor string) (*storage.Block, error) {
	return r.blockRepo.GetBlock(ctx, blockerActor, blockedActor)
}

// CountBlockedUsers returns the number of users blocked by the given actor
func (r *RelationshipRepository) CountBlockedUsers(ctx context.Context, blockerActor string) (int, error) {
	return r.blockRepo.CountBlockedUsers(ctx, blockerActor)
}

// CountUsersWhoBlocked returns the number of users who have blocked the given actor
func (r *RelationshipRepository) CountUsersWhoBlocked(ctx context.Context, blockedActor string) (int, error) {
	return r.blockRepo.CountUsersWhoBlocked(ctx, blockedActor)
}

// ===== Mute Methods =====

// CreateMute creates a new mute relationship
func (r *RelationshipRepository) CreateMute(ctx context.Context, muterActor, mutedActor, activityID string, hideNotifications bool, duration *time.Duration) error {
	return r.muteRepo.CreateMute(ctx, muterActor, mutedActor, activityID, hideNotifications, duration)
}

// DeleteMute removes a mute relationship (for Undo Mute)
func (r *RelationshipRepository) DeleteMute(ctx context.Context, muterActor, mutedActor string) error {
	return r.muteRepo.DeleteMute(ctx, muterActor, mutedActor)
}

// IsMuted checks if one actor has muted another
func (r *RelationshipRepository) IsMuted(ctx context.Context, muterActor, mutedActor string) (bool, error) {
	return r.muteRepo.IsMuted(ctx, muterActor, mutedActor)
}

// GetMutedUsers returns a list of users muted by the given actor
func (r *RelationshipRepository) GetMutedUsers(ctx context.Context, muterActor string, limit int, cursor string) ([]string, string, error) {
	return r.muteRepo.GetMutedUsers(ctx, muterActor, limit, cursor)
}

// GetUsersWhoMuted returns a list of users who have muted the given actor
func (r *RelationshipRepository) GetUsersWhoMuted(ctx context.Context, mutedActor string, limit int, cursor string) ([]string, string, error) {
	return r.muteRepo.GetUsersWhoMuted(ctx, mutedActor, limit, cursor)
}

// GetMute retrieves a specific mute relationship
func (r *RelationshipRepository) GetMute(ctx context.Context, muterActor, mutedActor string) (*storage.Mute, error) {
	return r.muteRepo.GetMute(ctx, muterActor, mutedActor)
}

// CountMutedUsers returns the number of users muted by the given actor
func (r *RelationshipRepository) CountMutedUsers(ctx context.Context, muterActor string) (int, error) {
	return r.muteRepo.CountMutedUsers(ctx, muterActor)
}

// CountUsersWhoMuted returns the number of users who have muted the given actor
func (r *RelationshipRepository) CountUsersWhoMuted(ctx context.Context, mutedActor string) (int, error) {
	return r.muteRepo.CountUsersWhoMuted(ctx, mutedActor)
}

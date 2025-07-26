package repositories

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/mastodon"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/dynamorm/pkg/errors"
)

// ActorRepository implements actor operations using DynamORM
type ActorRepository struct {
	db core.DB
}

// NewActorRepository creates a new actor repository
func NewActorRepository(db core.DB) *ActorRepository {
	return &ActorRepository{
		db: db,
	}
}

// CreateActor creates a new actor in DynamoDB
func (r *ActorRepository) CreateActor(ctx context.Context, actor *activitypub.Actor, privateKey string) error {
	if actor.PreferredUsername == "" {
		return common.ValidationError{Field: "PreferredUsername", Message: "username is required"}
	}

	username := actor.PreferredUsername
	numericID := mastodon.GenerateNumericID(username)

	// Create the DynamORM model
	actorModel := &models.Actor{
		Username:       username,
		Actor:          actor,
		PrivateKey:     privateKey, // TODO: Add encryption
		NumericID:      numericID,
		FollowerCount:  0,
		FollowingCount: 0,
		StatusCount:    0,
	}

	// Set domain for GSI3 if available
	// TODO: Get domain from config
	domain := "example.com" // This should come from config
	if domain != "" {
		actorModel.GSI3PK = "DOMAIN#" + domain
		actorModel.GSI3SK = username
	}

	// Create the actor using DynamORM
	err := r.db.WithContext(ctx).Model(actorModel).Create()
	if err != nil {
		if errors.IsConditionFailed(err) {
			return common.ConflictError{
				Resource: "actor",
				Message:  fmt.Sprintf("actor %s already exists", username),
			}
		}
		return fmt.Errorf("failed to create actor: %w", err)
	}

	return nil
}

// GetActor retrieves an actor by username
func (r *ActorRepository) GetActor(ctx context.Context, username string) (*activitypub.Actor, error) {
	var actorModel models.Actor

	err := r.db.WithContext(ctx).Model(&models.Actor{}).
		Where("PK", "=", "actor#"+username).
		Where("SK", "=", "actor#"+username).
		First(&actorModel)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, common.ActorNotFoundError{Username: username}
		}
		return nil, fmt.Errorf("failed to get actor: %w", err)
	}

	return actorModel.Actor, nil
}

// GetActorWithMetadata retrieves an actor with metadata
func (r *ActorRepository) GetActorWithMetadata(ctx context.Context, username string) (*activitypub.Actor, *storage.ActorMetadata, error) {
	var actorModel models.Actor

	err := r.db.WithContext(ctx).Model(&models.Actor{}).
		Where("PK", "=", "actor#"+username).
		Where("SK", "=", "actor#"+username).
		First(&actorModel)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, nil, common.ActorNotFoundError{Username: username}
		}
		return nil, nil, fmt.Errorf("failed to get actor: %w", err)
	}

	metadata := &storage.ActorMetadata{
		CreatedAt:    actorModel.CreatedAt,
		UpdatedAt:    actorModel.UpdatedAt,
		LastStatusAt: actorModel.LastStatusAt,
		Fields:       convertActorFields(actorModel.Fields),
	}

	return actorModel.Actor, metadata, nil
}

// GetActorByNumericID retrieves an actor by numeric ID
func (r *ActorRepository) GetActorByNumericID(ctx context.Context, numericID string) (*activitypub.Actor, error) {
	// Query by numeric ID using scan (could be optimized with GSI)
	var actors []models.Actor
	err := r.db.WithContext(ctx).Model(&models.Actor{}).
		Filter("NumericID", "=", numericID).
		Scan(&actors)
	if err != nil {
		return nil, fmt.Errorf("failed to query actor by numeric ID: %w", err)
	}

	if len(actors) == 0 {
		return nil, fmt.Errorf("actor not found: %s", numericID)
	}

	return actors[0].Actor, nil
}

// GetActorPrivateKey retrieves an actor's private key
func (r *ActorRepository) GetActorPrivateKey(ctx context.Context, username string) (string, error) {
	var actorModel models.Actor

	err := r.db.WithContext(ctx).Model(&models.Actor{}).
		Where("PK", "=", "actor#"+username).
		Where("SK", "=", "actor#"+username).
		Select("PrivateKey").
		First(&actorModel)
	if err != nil {
		if errors.IsNotFound(err) {
			return "", common.ActorNotFoundError{Username: username}
		}
		return "", fmt.Errorf("failed to get actor private key: %w", err)
	}

	// TODO: Add decryption
	return actorModel.PrivateKey, nil
}

// UpdateActor updates an existing actor
func (r *ActorRepository) UpdateActor(ctx context.Context, actor *activitypub.Actor) error {
	username := actor.PreferredUsername
	if username == "" {
		return common.ValidationError{Field: "PreferredUsername", Message: "username is required"}
	}

	// Get existing actor first
	var actorModel models.Actor
	err := r.db.WithContext(ctx).Model(&models.Actor{}).
		Where("PK", "=", "actor#"+username).
		Where("SK", "=", "actor#"+username).
		First(&actorModel)
	if err != nil {
		if errors.IsNotFound(err) {
			return common.ActorNotFoundError{Username: username}
		}
		return fmt.Errorf("failed to get existing actor: %w", err)
	}

	// Update the actor data
	actorModel.Actor = actor

	// Update using DynamORM
	err = r.db.WithContext(ctx).Model(&actorModel).Update()
	if err != nil {
		return fmt.Errorf("failed to update actor: %w", err)
	}

	return nil
}

// UpdateActorLastStatusTime updates the last status timestamp
func (r *ActorRepository) UpdateActorLastStatusTime(ctx context.Context, username string) error {
	// Get existing actor first
	var actorModel models.Actor
	err := r.db.WithContext(ctx).Model(&models.Actor{}).
		Where("PK", "=", "actor#"+username).
		Where("SK", "=", "actor#"+username).
		First(&actorModel)
	if err != nil {
		if errors.IsNotFound(err) {
			return common.ActorNotFoundError{Username: username}
		}
		return fmt.Errorf("failed to get existing actor: %w", err)
	}

	// Update last status time
	now := time.Now()
	actorModel.LastStatusAt = &now

	// Update using DynamORM
	err = r.db.WithContext(ctx).Model(&actorModel).Update()
	if err != nil {
		return fmt.Errorf("failed to update actor last status time: %w", err)
	}

	return nil
}

// SetActorFields updates the profile fields for an actor
func (r *ActorRepository) SetActorFields(ctx context.Context, username string, fields []storage.ActorField) error {
	// Get existing actor first
	var actorModel models.Actor
	err := r.db.WithContext(ctx).Model(&models.Actor{}).
		Where("PK", "=", "actor#"+username).
		Where("SK", "=", "actor#"+username).
		First(&actorModel)
	if err != nil {
		if errors.IsNotFound(err) {
			return common.ActorNotFoundError{Username: username}
		}
		return fmt.Errorf("failed to get existing actor: %w", err)
	}

	// Convert and update fields
	actorModel.Fields = convertStorageActorFields(fields)

	// Update using DynamORM
	err = r.db.WithContext(ctx).Model(&actorModel).Update()
	if err != nil {
		return fmt.Errorf("failed to update actor fields: %w", err)
	}

	return nil
}

// DeleteActor deletes an actor
func (r *ActorRepository) DeleteActor(ctx context.Context, username string) error {
	// Delete the actor using DynamORM
	err := r.db.WithContext(ctx).Model(&models.Actor{}).
		Where("PK", "=", "actor#"+username).
		Where("SK", "=", "actor#"+username).
		Delete()
	if err != nil {
		if errors.IsNotFound(err) {
			return common.ActorNotFoundError{Username: username}
		}
		return fmt.Errorf("failed to delete actor: %w", err)
	}

	return nil
}

// SearchAccounts searches for actors by username or display name
func (r *ActorRepository) SearchAccounts(ctx context.Context, query string, limit int, followingOnly bool, offset int) ([]*activitypub.Actor, error) {
	if query == "" {
		return []*activitypub.Actor{}, nil
	}

	normalizedQuery := strings.ToLower(strings.TrimSpace(query))
	var actors []models.Actor

	// Try username search first using GSI1
	if len(normalizedQuery) >= 2 {
		prefix := normalizedQuery[:2]
		err := r.db.WithContext(ctx).Model(&models.Actor{}).
			Index("username-search-index").
			Where("GSI1PK", "=", "USERNAME_SEARCH#"+prefix).
			Filter("GSI1SK", "BEGINS_WITH", normalizedQuery).
			Limit(limit).
			All(&actors)
		if err != nil {
			return nil, fmt.Errorf("failed to search actors by username: %w", err)
		}
	}

	// If no results and query could be a display name, try name search
	if len(actors) == 0 && len(normalizedQuery) >= 2 {
		prefix := normalizedQuery[:2]
		err := r.db.WithContext(ctx).Model(&models.Actor{}).
			Index("name-search-index").
			Where("GSI2PK", "=", "NAME_SEARCH#"+prefix).
			Filter("GSI2SK", "BEGINS_WITH", normalizedQuery).
			Limit(limit).
			All(&actors)
		if err != nil {
			return nil, fmt.Errorf("failed to search actors by name: %w", err)
		}
	}

	// Convert to activitypub.Actor slice
	result := make([]*activitypub.Actor, 0, len(actors))
	for _, actor := range actors {
		if actor.Actor != nil {
			result = append(result, actor.Actor)
		}
	}

	return result, nil
}

// GetSearchSuggestions returns search suggestions for autocomplete
func (r *ActorRepository) GetSearchSuggestions(ctx context.Context, prefix string) ([]storage.SearchSuggestion, error) {
	if len(prefix) < 2 {
		return []storage.SearchSuggestion{}, nil
	}

	normalizedPrefix := strings.ToLower(strings.TrimSpace(prefix))
	prefixKey := normalizedPrefix[:2]

	var actors []models.Actor
	err := r.db.WithContext(ctx).Model(&models.Actor{}).
		Index("username-search-index").
		Where("GSI1PK", "=", "USERNAME_SEARCH#"+prefixKey).
		Filter("GSI1SK", "BEGINS_WITH", normalizedPrefix).
		Limit(10).
		All(&actors)
	if err != nil {
		return nil, fmt.Errorf("failed to get search suggestions: %w", err)
	}

	suggestions := make([]storage.SearchSuggestion, 0, len(actors))
	for _, actor := range actors {
		suggestions = append(suggestions, storage.SearchSuggestion{
			Type:  "account",
			Value: actor.Username,
			Score: 100, // Could be based on follower count or activity
		})
	}

	return suggestions, nil
}

// Helper functions

// convertActorFields converts models.ActorField to storage.ActorField
func convertActorFields(fields []models.ActorField) []storage.ActorField {
	result := make([]storage.ActorField, len(fields))
	for i, field := range fields {
		result[i] = storage.ActorField{
			Name:       field.Name,
			Value:      field.Value,
			VerifiedAt: field.VerifiedAt,
		}
	}
	return result
}

// convertStorageActorFields converts storage.ActorField to models.ActorField
func convertStorageActorFields(fields []storage.ActorField) []models.ActorField {
	result := make([]models.ActorField, len(fields))
	for i, field := range fields {
		result[i] = models.ActorField{
			Name:       field.Name,
			Value:      field.Value,
			VerifiedAt: field.VerifiedAt,
		}
	}
	return result
}

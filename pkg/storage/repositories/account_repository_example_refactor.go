package repositories

// This file demonstrates how the utilities can be used to refactor repository code
// This is an example of how account_repository.go methods could be refactored

import (
	"context"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"go.uber.org/zap"
)

// GetUserRefactored demonstrates an example refactored GetUser method using utilities
func (r *AccountRepository) GetUserRefactored(ctx context.Context, username string) (*storage.User, error) {
	user := &models.User{}

	// Use key utility for consistent key generation
	pk := Utils.Keys.UserKey(username)

	err := r.Get(ctx, pk, SKMetadata, user)
	if err != nil {
		// Use error utility for consistent error handling
		return nil, ErrorHandler.HandleGetError(err, EntityUser, username)
	}

	return r.modelToStorageUser(user), nil
}

// GetUserByEmailRefactored is an example refactored GetUserByEmail method using utilities
func (r *AccountRepository) GetUserByEmailRefactored(ctx context.Context, email string) (*storage.User, error) {
	var user models.User

	// Use GSI key utility for consistent key generation
	emailKey := Utils.GSI.EmailIndexKey(email)

	err := r.db.WithContext(ctx).Model(&user).
		Index("email-index").
		Where("gsi2PK", "=", emailKey).
		First(&user)

	if err != nil {
		return nil, ErrorHandler.HandleGetError(err, EntityUser, email)
	}

	return r.modelToStorageUser(&user), nil
}

// CreateFollowRefactored is an example refactored CreateFollow method using utilities
func (r *AccountRepository) CreateFollowRefactored(ctx context.Context, followerUsername, followedUsername string) error {
	// Validate usernames using validation utility
	if !Utils.Validation.IsValidUsername(followerUsername) {
		return common.ValidationError{Field: "follower", Message: "invalid username"}
	}
	if !Utils.Validation.IsValidUsername(followedUsername) {
		return common.ValidationError{Field: "followed", Message: "invalid username"}
	}

	// Check if users exist using consistent error handling
	if _, err := r.GetUserRefactored(ctx, followerUsername); err != nil {
		return ErrorHandler.HandleNotFound(err, EntityUser, followerUsername)
	}
	if _, err := r.GetUserRefactored(ctx, followedUsername); err != nil {
		return ErrorHandler.HandleNotFound(err, EntityUser, followedUsername)
	}

	// Create follow relationship using key utilities
	follow := &models.Follow{
		FollowerUsername: followerUsername,
		FollowedUsername: followedUsername,
		State:            models.FollowStatePending,
		ActivityID:       "", // Would be generated
	}

	// Keys are set automatically by UpdateKeys(), but we could use utilities for validation
	pk := Utils.Keys.FollowKey(followerUsername)
	sk := Utils.Keys.FollowingSK(followedUsername)

	r.logger.Debug("creating follow relationship",
		zap.String("pk", pk),
		zap.String("sk", sk),
		zap.String("follower", followerUsername),
		zap.String("followed", followedUsername))

	err := r.db.WithContext(ctx).Model(follow).Create()
	return ErrorHandler.HandleCreateError(err, EntityFollow, followerUsername+"->"+followedUsername)
}

// StoreOAuthStateRefactored is an example refactored OAuth state creation using utilities
func (r *AccountRepository) StoreOAuthStateRefactored(ctx context.Context, state string, data *storage.OAuthState) error {
	// Set TTL using time utility
	if data.ExpiresAt.IsZero() {
		data.ExpiresAt = data.CreatedAt.Add(StandardTTLs.OAuthState)
	}

	model := &models.OAuthState{
		State:               state,
		Provider:            data.Provider,
		RedirectURI:         data.RedirectURI,
		Username:            data.Username,
		ClientID:            data.ClientID,
		Scopes:              data.Scopes,
		CodeChallenge:       data.CodeChallenge,
		CodeChallengeMethod: data.CodeChallengeMethod,
		CreatedAt:           data.CreatedAt,
		ExpiresAt:           data.ExpiresAt,
	}

	err := r.db.WithContext(ctx).Model(model).Create()
	return ErrorHandler.HandleCreateError(err, EntityOAuthState, state)
}

// This demonstrates how the utilities provide:
// 1. Consistent key generation (Utils.Keys.*)
// 2. Consistent error handling (ErrorHandler.Handle*)
// 3. Common validation (Utils.Validation.*)
// 4. Standardized TTL handling (Utils.Time.*, StandardTTLs)
// 5. Improved maintainability and reduced duplication

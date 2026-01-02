// Package mastodon provides Mastodon API compatibility services for actor and account management.
package mastodon

import (
	"context"
	"fmt"

	"github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage/core"
	"go.uber.org/zap"
)

type actorRepository interface {
	GetActorByUsername(ctx context.Context, username string) (*activitypub.Actor, error)
}

type relationshipRepository interface {
	GetFollowers(ctx context.Context, username string, limit int, cursor string) ([]string, string, error)
	GetFollowing(ctx context.Context, username string, limit int, cursor string) ([]string, string, error)
}

type objectRepository interface {
	GetObjectsByActor(ctx context.Context, actorID string, cursor string, limit int) ([]any, string, error)
}

// actorServiceImpl implements the ActorService interface
type actorServiceImpl struct {
	actors        actorRepository
	relationships relationshipRepository
	objects       objectRepository
	converter     Converter
	logger        *zap.Logger
}

// NewActorService creates a new actor service instance
func NewActorService(store core.RepositoryStorage, converter Converter, logger *zap.Logger) ActorService {
	if logger == nil {
		logger = zap.NewNop()
	}

	var actors actorRepository
	var relationships relationshipRepository
	var objects objectRepository
	if store != nil {
		actors = store.Actor()
		relationships = store.Relationship()
		objects = store.Object()
	}

	return &actorServiceImpl{
		actors:        actors,
		relationships: relationships,
		objects:       objects,
		converter:     converter,
		logger:        logger,
	}
}

// GetAccountByUsername retrieves an account by username
func (s *actorServiceImpl) GetAccountByUsername(ctx context.Context, username string) (*models.Account, error) {
	actor, err := s.actors.GetActorByUsername(ctx, username)
	if err != nil {
		return nil, fmt.Errorf("failed to get actor: %w", err)
	}

	account := s.converter.ActorToAccount(actor)
	return &account, nil
}

// GetAccountWithStats retrieves an account with follower/following/status counts
func (s *actorServiceImpl) GetAccountWithStats(ctx context.Context, username string) (*models.Account, error) {
	actor, err := s.actors.GetActorByUsername(ctx, username)
	if err != nil {
		return nil, fmt.Errorf("failed to get actor: %w", err)
	}

	// Get follower count (approximate - just get first page)
	followerCount := 0
	followers, _, err := s.relationships.GetFollowers(ctx, username, 1, "")
	if err == nil && len(followers) > 0 {
		// This is approximate - in production you'd want actual counts
		followerCount = len(followers)
	}

	// Get following count
	followingCount := 0
	following, _, err := s.relationships.GetFollowing(ctx, username, 1, "")
	if err == nil && len(following) > 0 {
		followingCount = len(following)
	}

	// Get statuses count
	statusesCount := 0
	objects, _, err := s.objects.GetObjectsByActor(ctx, actor.ID, "", 1)
	if err == nil && len(objects) > 0 {
		statusesCount = len(objects)
	}

	account := s.converter.ActorToAccountWithCounts(actor, followerCount, followingCount, statusesCount)
	return &account, nil
}

// GetAccountsByIDs retrieves multiple accounts by their actor IDs
func (s *actorServiceImpl) GetAccountsByIDs(ctx context.Context, actorIDs []string) ([]models.Account, error) {
	accounts := make([]models.Account, 0, len(actorIDs))

	for _, actorID := range actorIDs {
		username := s.converter.ExtractUsernameFromActorID(actorID)
		if err := common.ValidateRequiredParam("username", username); err != nil {
			s.logger.Warn("could not extract username from actor ID", zap.String("actorID", actorID))
			continue
		}

		actor, err := s.actors.GetActorByUsername(ctx, username)
		if err != nil {
			s.logger.Warn("failed to get actor", zap.String("username", username), zap.Error(err))
			continue
		}

		accounts = append(accounts, s.converter.ActorToAccount(actor))
	}

	return accounts, nil
}

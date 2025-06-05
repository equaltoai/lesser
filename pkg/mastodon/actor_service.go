package mastodon

import (
	"context"
	"fmt"

	"github.com/aron23/lesser/cmd/api/models"
	"github.com/aron23/lesser/pkg/storage"
	"go.uber.org/zap"
)

// actorServiceImpl implements the ActorService interface
type actorServiceImpl struct {
	store     storage.Storage
	converter Converter
	logger    *zap.Logger
}

// NewActorService creates a new actor service instance
func NewActorService(store storage.Storage, converter Converter, logger *zap.Logger) ActorService {
	return &actorServiceImpl{
		store:     store,
		converter: converter,
		logger:    logger,
	}
}

// GetAccountByUsername retrieves an account by username
func (s *actorServiceImpl) GetAccountByUsername(ctx context.Context, username string) (*models.Account, error) {
	actor, err := s.store.GetActor(ctx, username)
	if err != nil {
		return nil, fmt.Errorf("failed to get actor: %w", err)
	}

	account := s.converter.ActorToAccount(actor)
	return &account, nil
}

// GetAccountWithStats retrieves an account with follower/following/status counts
func (s *actorServiceImpl) GetAccountWithStats(ctx context.Context, username string) (*models.Account, error) {
	actor, err := s.store.GetActor(ctx, username)
	if err != nil {
		return nil, fmt.Errorf("failed to get actor: %w", err)
	}

	// Get follower count (approximate - just get first page)
	followerCount := 0
	followers, _, err := s.store.GetFollowers(ctx, username, 1, "")
	if err == nil && len(followers) > 0 {
		// This is approximate - in production you'd want actual counts
		followerCount = len(followers)
	}

	// Get following count
	followingCount := 0
	following, _, err := s.store.GetFollowing(ctx, username, 1, "")
	if err == nil && len(following) > 0 {
		followingCount = len(following)
	}

	// Get statuses count
	statusesCount := 0
	objects, _, err := s.store.GetObjectsByActor(ctx, actor.ID, "", 1)
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
		if username == "" {
			s.logger.Warn("could not extract username from actor ID", zap.String("actorID", actorID))
			continue
		}

		actor, err := s.store.GetActor(ctx, username)
		if err != nil {
			s.logger.Warn("failed to get actor", zap.String("username", username), zap.Error(err))
			continue
		}

		accounts = append(accounts, s.converter.ActorToAccount(actor))
	}

	return accounts, nil
}

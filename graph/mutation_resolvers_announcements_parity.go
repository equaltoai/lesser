package graph

import (
	"context"
	"errors"
	"strings"

	"github.com/equaltoai/lesser/pkg/common"
	"go.uber.org/zap"
)

type announcementReactionMutator func(ctx context.Context, username, announcementID, reactionName string) error

// DismissAnnouncement is the resolver for the dismissAnnouncement field.
func (r *mutationResolver) DismissAnnouncement(ctx context.Context, id string) (bool, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return false, err
	}

	announcementID := strings.TrimSpace(id)
	if err := common.ValidateRequiredParam("id", announcementID); err != nil {
		return false, err
	}

	if r.Storage == nil || r.Storage.Announcement() == nil {
		return false, ErrStorageUnavailable
	}

	announcement, err := r.Storage.Announcement().GetAnnouncement(ctx, announcementID)
	if err != nil || announcement == nil {
		return false, errors.New("announcement not found")
	}

	if err := r.Storage.Announcement().DismissAnnouncement(ctx, username, announcementID); err != nil {
		r.Logger.Error("Failed to dismiss announcement",
			zap.String("announcement_id", announcementID),
			zap.String("username", username),
			zap.Error(err))
		return false, errors.Join(errors.New("failed to dismiss announcement"), err)
	}

	r.trackDynamoOperation(ctx, "write", 1)
	return true, nil
}

func (r *mutationResolver) mutateAnnouncementReaction(ctx context.Context, id string, name string, action string, mutate announcementReactionMutator) (bool, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return false, err
	}

	announcementID := strings.TrimSpace(id)
	if err := common.ValidateRequiredParam("id", announcementID); err != nil {
		return false, err
	}
	reactionName := strings.TrimSpace(name)
	if err := common.ValidateRequiredParam("name", reactionName); err != nil {
		return false, err
	}

	if r.Storage == nil || r.Storage.Announcement() == nil {
		return false, ErrStorageUnavailable
	}
	if mutate == nil {
		return false, errors.New("mutation function is not available")
	}

	announcement, err := r.Storage.Announcement().GetAnnouncement(ctx, announcementID)
	if err != nil || announcement == nil {
		return false, errors.New("announcement not found")
	}

	if err := mutate(ctx, username, announcementID, reactionName); err != nil {
		r.Logger.Error("Failed to mutate announcement reaction",
			zap.String("action", action),
			zap.String("announcement_id", announcementID),
			zap.String("username", username),
			zap.String("reaction", reactionName),
			zap.Error(err))
		return false, errors.Join(errors.New("failed to mutate announcement reaction"), err)
	}

	r.trackDynamoOperation(ctx, "write", 1)
	return true, nil
}

// AddAnnouncementReaction is the resolver for the addAnnouncementReaction field.
func (r *mutationResolver) AddAnnouncementReaction(ctx context.Context, id string, name string) (bool, error) {
	return r.mutateAnnouncementReaction(ctx, id, name, "add", func(ctx context.Context, username, announcementID, reactionName string) error {
		return r.Storage.Announcement().AddAnnouncementReaction(ctx, username, announcementID, reactionName)
	})
}

// RemoveAnnouncementReaction is the resolver for the removeAnnouncementReaction field.
func (r *mutationResolver) RemoveAnnouncementReaction(ctx context.Context, id string, name string) (bool, error) {
	return r.mutateAnnouncementReaction(ctx, id, name, "remove", func(ctx context.Context, username, announcementID, reactionName string) error {
		return r.Storage.Announcement().RemoveAnnouncementReaction(ctx, username, announcementID, reactionName)
	})
}

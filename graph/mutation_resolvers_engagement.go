package graph

import (
	"context"
	"errors"
	"time"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/services/notes"
	"go.uber.org/zap"
)

// NOTE: imports intentionally omitted. Run gofmt/goimports and add any
// required imports after generating these files.

// LikeObject is the resolver for the likeObject field.
func (r *mutationResolver) LikeObject(ctx context.Context, id string) (*activitypub.Activity, error) {
	return r.executeSocialAction(ctx, id, activitypub.LikeType, "like", func(ctx context.Context, objectID, username string) error {
		_, err := r.Registry.Notes().LikeNote(ctx, &notes.LikeNoteCommand{
			StatusID: objectID,
			LikerID:  username,
		})
		return err
	})
}

// UnlikeObject is the resolver for the unlikeObject field.
func (r *mutationResolver) UnlikeObject(ctx context.Context, id string) (bool, error) {
	return r.executeSocialUndo(ctx, id, "unlike", func(ctx context.Context, objectID, username string) error {
		_, err := r.Registry.Notes().UnlikeNote(ctx, &notes.UnlikeNoteCommand{
			StatusID:  objectID,
			UnlikerID: username,
		})
		return err
	})
}

// ShareObject is the resolver for the shareObject field.
func (r *mutationResolver) ShareObject(ctx context.Context, id string) (*activitypub.Activity, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	result, err := r.Registry.Notes().ReblogNote(ctx, &notes.ReblogNoteCommand{
		StatusID:    id,
		RebloggerID: username,
	})
	if err != nil {
		r.Logger.Error("Failed to share object",
			zap.String("user", username),
			zap.String("object", id),
			zap.Error(err))
		return nil, ErrSocialActionFailedWithContext("share", err)
	}

	// Track the write for cost attribution
	r.trackDynamoOperation(ctx, "write", 1)

	if result != nil {
		if activity := buildActivityFromAnnounce(username, id, result.Announce); activity != nil {
			return activity, nil
		}
	}

	r.Logger.Debug("share result missing announce metadata; returning synthetic activity",
		zap.String("user", username),
		zap.String("object", id))

	now := time.Now()
	return &activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			ID:        generateID(),
			Type:      activitypub.AnnounceType,
			Published: &now,
		},
		Actor:  username,
		Object: id,
	}, nil
}

// UnshareObject is the resolver for the unshareObject field.
func (r *mutationResolver) UnshareObject(ctx context.Context, id string) (bool, error) {
	return r.executeSocialUndo(ctx, id, "unshare", func(ctx context.Context, objectID, username string) error {
		_, err := r.Registry.Notes().UnreblogNote(ctx, &notes.UnreblogNoteCommand{
			StatusID:      objectID,
			UnrebloggerID: username,
		})
		return err
	})
}

// BookmarkObject is the resolver for the bookmarkObject field.
func (r *mutationResolver) BookmarkObject(ctx context.Context, id string) (*model.Object, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	result, err := r.Registry.Notes().BookmarkNote(ctx, &notes.BookmarkNoteCommand{
		StatusID:     id,
		BookmarkerID: username,
	})
	if err != nil {
		r.Logger.Error("Failed to bookmark object",
			zap.String("user", username),
			zap.String("object", id),
			zap.Error(err))
		return nil, errors.Join(errors.New("failed to bookmark"), err)
	}

	// Track cost using centralized tracker
	r.trackDynamoOperation(ctx, "write", 1)
	return r.convertStatusToObject(ctx, result.Status), nil
}

// UnbookmarkObject is the resolver for the unbookmarkObject field.
func (r *mutationResolver) UnbookmarkObject(ctx context.Context, id string) (bool, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return false, err
	}

	_, err = r.Registry.Notes().UnbookmarkNote(ctx, &notes.UnbookmarkNoteCommand{
		StatusID:       id,
		UnbookmarkerID: username,
	})
	if err != nil {
		r.Logger.Error("Failed to unbookmark object",
			zap.String("user", username),
			zap.String("object", id),
			zap.Error(err))
		return false, errors.Join(errors.New("failed to unbookmark"), err)
	}

	// Track cost using centralized tracker
	r.trackDynamoOperation(ctx, "write", 1)
	return true, nil
}

// PinObject is the resolver for the pinObject field.
func (r *mutationResolver) PinObject(ctx context.Context, id string) (*model.Object, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	result, err := r.Registry.Notes().PinNote(ctx, &notes.PinNoteCommand{
		StatusID: id,
		PinnerID: username,
	})
	if err != nil {
		r.Logger.Error("Failed to pin object",
			zap.String("user", username),
			zap.String("object", id),
			zap.Error(err))
		return nil, errors.Join(errors.New("failed to pin"), err)
	}

	// Track cost using centralized tracker
	r.trackDynamoOperation(ctx, "write", 1)
	return r.convertStatusToObject(ctx, result.Status), nil
}

// UnpinObject is the resolver for the unpinObject field.
func (r *mutationResolver) UnpinObject(ctx context.Context, id string) (bool, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return false, err
	}

	_, err = r.Registry.Notes().UnpinNote(ctx, &notes.UnpinNoteCommand{
		StatusID: id,
		PinnerID: username,
	})
	if err != nil {
		r.Logger.Error("Failed to unpin object",
			zap.String("user", username),
			zap.String("object", id),
			zap.Error(err))
		return false, errors.Join(errors.New("failed to unpin"), err)
	}

	// Track cost using centralized tracker
	r.trackDynamoOperation(ctx, "write", 1)
	return true, nil
}

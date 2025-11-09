package graph

import (
	"context"
	"errors"

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

var errNotesServiceUnavailable = errors.New("notes service unavailable")

type shareOperationOptions struct {
	logAction        string
	loadErrorMessage string
	wrapError        func(error) error
	operation        func(ctx context.Context, svc notesService, username, id string) error
}

func (r *mutationResolver) executeShareOperation(ctx context.Context, id string, opts shareOperationOptions) (*model.Object, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	notesService := r.notesService()
	if notesService == nil {
		return nil, errNotesServiceUnavailable
	}

	if err := opts.operation(ctx, notesService, username, id); err != nil {
		r.Logger.Error("Failed to "+opts.logAction+" object",
			zap.String("user", username),
			zap.String("object", id),
			zap.Error(err))
		if opts.wrapError != nil {
			return nil, opts.wrapError(err)
		}
		return nil, err
	}

	updatedStatus, err := notesService.GetNote(ctx, id)
	if err != nil {
		r.Logger.Error("Failed to load updated object after "+opts.logAction,
			zap.String("user", username),
			zap.String("object", id),
			zap.Error(err))

		message := opts.loadErrorMessage
		if message == "" {
			message = "failed to load updated object after " + opts.logAction
		}
		return nil, errors.Join(errors.New(message), err)
	}

	// Track cost using centralized tracker
	r.trackDynamoOperation(ctx, "write", 1)

	return r.convertStatusToObject(ctx, updatedStatus), nil
}

// ShareObject is the resolver for the shareObject field.
func (r *mutationResolver) ShareObject(ctx context.Context, id string) (*model.Object, error) {
	return r.executeShareOperation(ctx, id, shareOperationOptions{
		logAction:        "share",
		loadErrorMessage: "failed to load updated object after share",
		wrapError: func(err error) error {
			return ErrSocialActionFailedWithContext("share", err)
		},
		operation: func(ctx context.Context, svc notesService, username, objectID string) error {
			_, err := svc.ReblogNote(ctx, &notes.ReblogNoteCommand{
				StatusID:    objectID,
				RebloggerID: username,
			})
			return err
		},
	})
}

// UnshareObject is the resolver for the unshareObject field.
func (r *mutationResolver) UnshareObject(ctx context.Context, id string) (*model.Object, error) {
	return r.executeShareOperation(ctx, id, shareOperationOptions{
		logAction:        "unshare",
		loadErrorMessage: "failed to load updated object after unshare",
		wrapError: func(err error) error {
			return ErrSocialUndoFailedWithContext("unshare", err)
		},
		operation: func(ctx context.Context, svc notesService, username, objectID string) error {
			_, err := svc.UnreblogNote(ctx, &notes.UnreblogNoteCommand{
				StatusID:      objectID,
				UnrebloggerID: username,
			})
			return err
		},
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

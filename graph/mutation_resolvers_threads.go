package graph

import (
	"context"
	"errors"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/services/threads"
	"go.uber.org/zap"
)

var (
	// ErrThreadsServiceUnavailable indicates the threads service is not available
	ErrThreadsServiceUnavailable = errors.New("threads service is not available")
)

// SyncThread is the resolver for the syncThread field.
func (r *mutationResolver) SyncThread(ctx context.Context, noteURL string, depth *int) (*model.SyncThreadPayload, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}
	_ = username // username is validated but not used in this operation

	if err := common.ValidateRequiredParam("noteURL", noteURL); err != nil {
		return nil, errors.Join(errors.New("note_url is required"), err)
	}

	service := r.Registry.Threads()
	if service == nil {
		return nil, ErrThreadsServiceUnavailable
	}

	cmd := threads.SyncRemoteThreadCommand{
		NoteURL:  noteURL,
		ViewerID: username,
	}
	if depth != nil {
		cmd.Depth = *depth
	} else {
		cmd.Depth = threads.DefaultDepth
	}

	result, err := service.SyncRemoteThread(ctx, cmd)
	if err != nil {
		r.Logger.Error("failed to sync thread", zap.Error(err))
		return nil, errors.Join(errors.New("failed to sync thread"), err)
	}

	// Get thread context for the root note
	contextResult, err := service.GetThreadContext(ctx, threads.ThreadContextQuery{
		NoteID:      result.ThreadRoot.ID,
		ViewerID:    username,
		IncludeTree: false,
	})
	if err != nil {
		r.Logger.Warn("failed to get thread context after sync", zap.Error(err))
	}

	return &model.SyncThreadPayload{
		Success:     result.Success,
		Thread:      r.convertThreadContextResultToModel(ctx, contextResult),
		SyncedPosts: result.SyncedPosts,
		Errors:      result.Errors,
	}, nil
}

// SyncMissingReplies is the resolver for the syncMissingReplies field.
func (r *mutationResolver) SyncMissingReplies(ctx context.Context, noteID string) (*model.SyncRepliesPayload, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}
	_ = username // username is validated but not used in this operation

	if err := common.ValidateRequiredParam("noteID", noteID); err != nil {
		return nil, errors.Join(errors.New("note_id is required"), err)
	}

	service := r.Registry.Threads()
	if service == nil {
		return nil, ErrThreadsServiceUnavailable
	}

	result, err := service.SyncMissingReplies(ctx, threads.SyncMissingRepliesCommand{
		NoteID:   noteID,
		ViewerID: username,
	})
	if err != nil {
		r.Logger.Error("failed to sync missing replies", zap.Error(err))
		return nil, errors.Join(errors.New("failed to sync missing replies"), err)
	}

	// Get thread context after syncing
	var contextResult *threads.ThreadContextResult
	if result.Success && result.SyncedReplies > 0 {
		contextResult, err = service.GetThreadContext(ctx, threads.ThreadContextQuery{
			NoteID:      noteID,
			ViewerID:    username,
			IncludeTree: false,
		})
		if err != nil {
			r.Logger.Warn("failed to get thread context after sync", zap.Error(err))
		}
	}

	return &model.SyncRepliesPayload{
		Success:       result.Success,
		SyncedReplies: result.SyncedReplies,
		Thread:        r.convertThreadContextResultToModel(ctx, contextResult),
	}, nil
}

package threads

import "errors"

// Service-level errors for thread operations
var (
	// Thread traversal errors
	ErrThreadNotFound         = errors.New("thread not found")
	ErrThreadRootNotFound     = errors.New("thread root not found")
	ErrCircularReference      = errors.New("circular reference detected in thread")
	ErrMaxDepthExceeded       = errors.New("maximum thread depth exceeded")
	ErrInvalidThreadStructure = errors.New("invalid thread structure")

	// Federation errors
	ErrFetchRemoteNote           = errors.New("failed to fetch remote note")
	ErrFetchRemoteReplies        = errors.New("failed to fetch remote replies")
	ErrRemoteNoteNotFound        = errors.New("remote note not found")
	ErrRemoteInstanceUnreachable = errors.New("remote instance unreachable")
	ErrRemoteAuthFailed          = errors.New("remote authentication failed")
	ErrRemoteTimeout             = errors.New("remote request timeout")

	// Sync errors
	ErrSyncInProgress     = errors.New("sync already in progress for this thread")
	ErrSyncFailed         = errors.New("thread synchronization failed")
	ErrPartialSync        = errors.New("thread synchronization partially completed")
	ErrSyncMissingReplies = errors.New("failed to sync missing replies")

	// Storage errors
	ErrSaveThreadNode   = errors.New("failed to save thread node")
	ErrSaveThreadSync   = errors.New("failed to save thread sync record")
	ErrGetThreadContext = errors.New("failed to get thread context")
	ErrMarkMissingReply = errors.New("failed to mark missing reply")

	// Validation errors
	ErrInvalidNoteID        = errors.New("invalid note ID")
	ErrInvalidNoteURL       = errors.New("invalid note URL")
	ErrInvalidDepth         = errors.New("invalid depth parameter")
	ErrMissingRequiredParam = errors.New("missing required parameter")

	// Note type errors
	ErrNotANote             = errors.New("object is not a note")
	ErrInvalidNoteStructure = errors.New("note has invalid structure")
)

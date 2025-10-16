package sync

import (
	stdErrors "errors"

	"github.com/equaltoai/lesser/pkg/errors"
)

// Thread synchronization errors
var (
	// ErrFetchThreadContext is returned when thread context cannot be fetched
	ErrFetchThreadContext = errors.ProcessingFailed("thread context fetch", stdErrors.New("thread context fetch failed"))

	// ErrFetchRootNote is returned when root note cannot be fetched
	ErrFetchRootNote = errors.FailedToGet("root note", stdErrors.New("failed to get root note"))

	// ErrInvalidRootObject is returned when root object is not a Note
	ErrInvalidRootObject = errors.NewValidationError("root_object", "is not a Note")

	// ErrGetNote is returned when note cannot be retrieved from storage
	ErrGetNote = errors.FailedToGet("note", stdErrors.New("failed to get note"))

	// ErrInvalidNoteType is returned when object is not a Note type
	ErrInvalidNoteType = errors.NewValidationError("object_type", "is not a Note")

	// ErrFetchParent is returned when parent note cannot be fetched
	ErrFetchParent = errors.FailedToGet("parent note", stdErrors.New("failed to get parent note"))

	// ErrStoreParentNote is returned when parent note cannot be stored
	ErrStoreParentNote = errors.FailedToStore("parent note", stdErrors.New("failed to store parent note"))
)

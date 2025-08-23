package sync

import "errors"

// Thread synchronization errors
var (
	// ErrFetchThreadContext is returned when thread context cannot be fetched
	ErrFetchThreadContext = errors.New("failed to fetch thread context")

	// ErrFetchRootNote is returned when root note cannot be fetched
	ErrFetchRootNote = errors.New("failed to fetch root note")

	// ErrInvalidRootObject is returned when root object is not a Note
	ErrInvalidRootObject = errors.New("root object is not a Note")

	// ErrGetNote is returned when note cannot be retrieved from storage
	ErrGetNote = errors.New("failed to get note")

	// ErrInvalidNoteType is returned when object is not a Note type
	ErrInvalidNoteType = errors.New("object is not a Note")

	// ErrFetchParent is returned when parent note cannot be fetched
	ErrFetchParent = errors.New("failed to fetch parent")

	// ErrStoreParentNote is returned when parent note cannot be stored
	ErrStoreParentNote = errors.New("failed to store parent note")
)
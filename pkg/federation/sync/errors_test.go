package sync

import (
	"testing"

	apperrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/stretchr/testify/require"
)

func TestThreadSyncErrors_AreAppErrors(t *testing.T) {
	require.True(t, apperrors.IsAppError(ErrFetchThreadContext))
	require.True(t, apperrors.IsAppError(ErrFetchRootNote))
	require.True(t, apperrors.IsAppError(ErrInvalidRootObject))
	require.True(t, apperrors.IsAppError(ErrGetNote))
	require.True(t, apperrors.IsAppError(ErrInvalidNoteType))
	require.True(t, apperrors.IsAppError(ErrFetchParent))
	require.True(t, apperrors.IsAppError(ErrStoreParentNote))
}

func TestErrInvalidRootObject_HasValidationCategory(t *testing.T) {
	require.Equal(t, apperrors.CategoryValidation, ErrInvalidRootObject.Category)
	require.Equal(t, apperrors.CodeValidationFailed, ErrInvalidRootObject.Code)
}

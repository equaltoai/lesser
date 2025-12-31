package dynamorm

import (
	"errors"
	"fmt"
	"testing"

	appErrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/stretchr/testify/require"
)

func TestMapError_Round22(t *testing.T) {
	t.Run("nil", func(t *testing.T) {
		require.NoError(t, MapError(nil))
	})

	t.Run("preserves app error", func(t *testing.T) {
		orig := appErrors.NotFound("item")
		wrapped := fmt.Errorf("wrapped: %w", orig)
		require.Same(t, wrapped, MapError(wrapped))
	})

	t.Run("not found pattern", func(t *testing.T) {
		mapped := MapError(errors.New("record not found"))
		require.True(t, appErrors.HasCode(mapped, appErrors.CodeNotFound))
		require.True(t, IsNotFound(mapped))
	})

	t.Run("validation pattern", func(t *testing.T) {
		mapped := MapError(errors.New("validation failed: bad input"))
		require.True(t, appErrors.HasCode(mapped, appErrors.CodeValidationFailed))
	})

	t.Run("conditional check pattern", func(t *testing.T) {
		mapped := MapError(errors.New("Conditional check failed"))
		require.True(t, appErrors.HasCode(mapped, appErrors.CodeConflict))
		require.True(t, IsConditionalCheckFailed(mapped))
	})

	t.Run("throttling pattern", func(t *testing.T) {
		mapped := MapError(errors.New("ThrottlingException"))
		require.True(t, appErrors.HasCode(mapped, appErrors.CodeRateLimited))
		require.True(t, IsThrottling(mapped))
	})

	t.Run("transaction pattern", func(t *testing.T) {
		mapped := MapError(errors.New("transaction failed"))
		require.True(t, appErrors.HasCode(mapped, appErrors.CodeTransactionFailed))
		require.True(t, IsTransactionCanceled(mapped))
	})
}

func TestMapErrorWithContext_Round22(t *testing.T) {
	t.Run("nil", func(t *testing.T) {
		require.NoError(t, MapErrorWithContext(nil, "ctx"))
	})

	t.Run("adds metadata", func(t *testing.T) {
		mapped := MapErrorWithContext(errors.New("record not found"), "read user")
		appErr, ok := appErrors.AsAppError(mapped)
		require.True(t, ok)
		require.Equal(t, "read user", appErr.Metadata["context"])
	})
}

func TestNewDetailedError_Round22(t *testing.T) {
	require.NoError(t, NewDetailedError(nil, "op", "type", "id", "ctx"))

	err := NewDetailedError(errors.New("boom"), "Create", "user", "u1", "saving")
	appErr, ok := appErrors.AsAppError(err)
	require.True(t, ok)
	require.Equal(t, "Create", appErr.Metadata["operation"])
	require.Equal(t, "user", appErr.Metadata["entity_type"])
	require.Equal(t, "u1", appErr.Metadata["entity_id"])
	require.Equal(t, "saving", appErr.Metadata["context"])
}

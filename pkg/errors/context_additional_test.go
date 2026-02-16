package errors

import (
	stdErrors "errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestContextHelpers_AdditionalCoverage(t *testing.T) {
	t.Run("AsNonRetryable", func(t *testing.T) {
		err := NewAppError(CodeInternal, CategoryInternal, "x").AsRetryable().AsNonRetryable()
		require.NotNil(t, err)
		assert.False(t, err.Retryable)
	})

	t.Run("NewAppErrorf", func(t *testing.T) {
		err := NewAppErrorf(CodeBadRequest, CategoryAPI, "hello %s", "world")
		require.NotNil(t, err)
		assert.Equal(t, CodeBadRequest, err.Code)
		assert.Equal(t, CategoryAPI, err.Category)
		assert.Equal(t, "hello world", err.Message)
		assert.NotZero(t, err.Timestamp)
		assert.WithinDuration(t, time.Now(), err.Timestamp, time.Second)
	})

	t.Run("WrapErrorf", func(t *testing.T) {
		underlying := stdErrors.New("boom")
		err := WrapErrorf(underlying, CodeInternal, CategoryInternal, "wrapped %d", 123)
		require.NotNil(t, err)
		assert.Equal(t, "wrapped 123", err.Message)
		assert.Equal(t, underlying.Error(), err.InternalMessage)
		assert.ErrorIs(t, err, underlying)
	})

	t.Run("NotFound", func(t *testing.T) {
		err := NotFound("user")
		require.NotNil(t, err)
		assert.Equal(t, CodeNotFound, err.Code)
		assert.Equal(t, CategoryAPI, err.Category)
		assert.Contains(t, err.Message, "user not found")
	})

	t.Run("NotFoundWithID", func(t *testing.T) {
		err := NotFoundWithID("user", "alice")
		require.NotNil(t, err)
		assert.Equal(t, CodeNotFound, err.Code)
		assert.Equal(t, CategoryAPI, err.Category)
		assert.Contains(t, err.Metadata, "resource")
		assert.Contains(t, err.Metadata, "id")
	})

	t.Run("Unauthorized", func(t *testing.T) {
		defaultErr := Unauthorized("")
		require.NotNil(t, defaultErr)
		assert.Equal(t, CodeUnauthorized, defaultErr.Code)
		assert.Equal(t, CategoryAuth, defaultErr.Category)
		assert.Equal(t, "Authentication required", defaultErr.Message)

		customErr := Unauthorized("custom")
		require.NotNil(t, customErr)
		assert.Equal(t, "custom", customErr.Message)
	})

	t.Run("ValidationFailed", func(t *testing.T) {
		err := ValidationFailed("field", "bad input")
		require.NotNil(t, err)
		assert.Equal(t, CodeValidationFailed, err.Code)
		assert.Equal(t, CategoryValidation, err.Category)
		assert.Contains(t, err.Metadata, "field")
	})

	t.Run("Error string when message empty", func(t *testing.T) {
		err := &AppError{
			Code:            CodeInternal,
			Category:        CategoryInternal,
			Message:         "",
			InternalMessage: "details",
		}
		assert.Contains(t, err.Error(), "details")
	})

	t.Run("IsRetryable for non-AppError", func(t *testing.T) {
		assert.False(t, IsRetryable(stdErrors.New("nope")))
	})
}

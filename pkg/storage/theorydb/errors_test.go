package theorydb

import (
	stdErrors "errors"
	"testing"

	appErrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMapError_NilAndAppErrorPassthrough(t *testing.T) {
	assert.Nil(t, MapError(nil))

	appErr := appErrors.NewValidationError("field", "bad")
	mapped := MapError(appErr)
	assert.Same(t, appErr, mapped)
}

func TestMapError_MapsCommonPatterns(t *testing.T) {
	tests := []struct {
		name         string
		err          error
		expectedCode appErrors.ErrorCode
	}{
		{name: "not found", err: stdErrors.New("record not found"), expectedCode: appErrors.CodeNotFound},
		{name: "validation failed", err: stdErrors.New("validation failed"), expectedCode: appErrors.CodeValidationFailed},
		{name: "conditional check failed", err: stdErrors.New("conditional check failed"), expectedCode: appErrors.CodeConflict},
		{name: "transaction failed", err: stdErrors.New("transaction failed"), expectedCode: appErrors.CodeTransactionFailed},
		{name: "invalid key", err: stdErrors.New("invalid key"), expectedCode: appErrors.CodeInvalidFormat},
		{name: "throttling", err: stdErrors.New("throttling"), expectedCode: appErrors.CodeRateLimited},
		{name: "resource not found", err: stdErrors.New("table not found"), expectedCode: appErrors.CodeExternalServiceUnavailable},
		{name: "batch failed", err: stdErrors.New("batch operation failed"), expectedCode: appErrors.CodeInternal},
		{name: "generic invalid", err: stdErrors.New("invalid value"), expectedCode: appErrors.CodeValidationFailed},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mapped := MapError(tt.err)
			require.Error(t, mapped)
			assert.True(t, appErrors.HasCode(mapped, tt.expectedCode))

			if tt.expectedCode == appErrors.CodeNotFound {
				appErr, ok := appErrors.AsAppError(mapped)
				require.True(t, ok)
				assert.ErrorIs(t, appErr.InternalError, storage.ErrNotFound)
			}
		})
	}
}

func TestMapError_DefaultsToInternal(t *testing.T) {
	mapped := MapError(stdErrors.New("something else"))
	require.Error(t, mapped)
	assert.True(t, appErrors.HasCode(mapped, appErrors.CodeInternal))
}

func TestMapErrorWithContext_AddsMetadata(t *testing.T) {
	mapped := MapErrorWithContext(stdErrors.New("record not found"), "lookup_user")
	require.Error(t, mapped)

	appErr, ok := appErrors.AsAppError(mapped)
	require.True(t, ok)
	assert.Equal(t, "lookup_user", appErr.Metadata["context"])
}

func TestDetailedError_Behavior(t *testing.T) {
	de := &DetailedError{
		Err:        stdErrors.New("boom"),
		Operation:  "create",
		EntityType: "user",
		EntityID:   "alice",
		Context:    "during test",
	}

	assert.Contains(t, de.Error(), "create")
	assert.Contains(t, de.Error(), "user")
	assert.Contains(t, de.Error(), "alice")
	assert.Contains(t, de.Error(), "during test")
	assert.Contains(t, de.Error(), "boom")
	assert.Same(t, de.Err, de.Unwrap())
}

func TestNewDetailedError_NilAndMetadata(t *testing.T) {
	assert.Nil(t, NewDetailedError(nil, "op", "type", "id", "ctx"))

	err := NewDetailedError(stdErrors.New("boom"), "op", "type", "id", "ctx")
	appErr, ok := appErrors.AsAppError(err)
	require.True(t, ok)
	assert.Equal(t, "op", appErr.Metadata["operation"])
	assert.Equal(t, "type", appErr.Metadata["entity_type"])
	assert.Equal(t, "id", appErr.Metadata["entity_id"])
	assert.Equal(t, "ctx", appErr.Metadata["context"])
}

func TestErrorPredicates(t *testing.T) {
	assert.True(t, IsNotFound(appErrors.NotFound("item")))
	assert.True(t, IsConditionalCheckFailed(appErrors.DynamoDBConditionalCheckFailed("x")))
	assert.True(t, IsThrottling(appErrors.DynamoDBProvisionedThroughputExceeded()))
	assert.True(t, IsTransactionCanceled(appErrors.TransactionFailed(stdErrors.New("boom"))))
	assert.True(t, IsValidation(appErrors.NewValidationError("field", "bad")))
}

func TestMapRepositoryError_AddsMetadata(t *testing.T) {
	err := MapRepositoryError(stdErrors.New("record not found"), "get", "user", "alice")
	appErr, ok := appErrors.AsAppError(err)
	require.True(t, ok)
	assert.Equal(t, "get", appErr.Metadata["operation"])
	assert.Equal(t, "user", appErr.Metadata["entity_type"])
	assert.Equal(t, "alice", appErr.Metadata["entity_id"])
}

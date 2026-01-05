package dlq

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// ============================================================================
// Error variable tests
// ============================================================================

func TestErrorVariables(t *testing.T) {
	// Test that all error variables are properly initialized
	tests := []struct {
		name     string
		err      error
		notNil   bool
	}{
		{
			name:   "ErrBatchProcessingFailed",
			err:    ErrBatchProcessingFailed,
			notNil: true,
		},
		{
			name:   "ErrNoDLQMessagesProcessed",
			err:    ErrNoDLQMessagesProcessed,
			notNil: true,
		},
		{
			name:   "ErrMissingRequiredField",
			err:    ErrMissingRequiredField,
			notNil: true,
		},
		{
			name:   "ErrChannelsMustBeArray",
			err:    ErrChannelsMustBeArray,
			notNil: true,
		},
		{
			name:   "ErrMissingActivityPubType",
			err:    ErrMissingActivityPubType,
			notNil: true,
		},
		{
			name:   "ErrActivityPubTypeMustBeString",
			err:    ErrActivityPubTypeMustBeString,
			notNil: true,
		},
		{
			name:   "ErrMissingActivityPubActor",
			err:    ErrMissingActivityPubActor,
			notNil: true,
		},
		{
			name:   "ErrActivityPubActorMustBeString",
			err:    ErrActivityPubActorMustBeString,
			notNil: true,
		},
		{
			name:   "ErrInvalidAction",
			err:    ErrInvalidAction,
			notNil: true,
		},
		{
			name:   "ErrInvalidMediaURL",
			err:    ErrInvalidMediaURL,
			notNil: true,
		},
		{
			name:   "ErrInvalidMediaURLFormat",
			err:    ErrInvalidMediaURLFormat,
			notNil: true,
		},
		{
			name:   "ErrInvalidInboxURL",
			err:    ErrInvalidInboxURL,
			notNil: true,
		},
		{
			name:   "ErrInvalidInboxURLFormat",
			err:    ErrInvalidInboxURLFormat,
			notNil: true,
		},
		{
			name:   "ErrMediaPermanentlyUnavailable",
			err:    ErrMediaPermanentlyUnavailable,
			notNil: true,
		},
		{
			name:   "ErrMediaAccessDenied",
			err:    ErrMediaAccessDenied,
			notNil: true,
		},
		{
			name:   "ErrMediaValidationFailed",
			err:    ErrMediaValidationFailed,
			notNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.notNil {
				assert.NotNil(t, tt.err, "%s should not be nil", tt.name)
				assert.NotEmpty(t, tt.err.Error(), "%s should have an error message", tt.name)
			}
		})
	}
}

func TestErrorMessages(t *testing.T) {
	// Test that error messages are meaningful
	tests := []struct {
		name            string
		err             error
		containsSubstr  string
	}{
		{
			name:           "ErrChannelsMustBeArray message",
			err:            ErrChannelsMustBeArray,
			containsSubstr: "array",
		},
		{
			name:           "ErrInvalidAction message",
			err:            ErrInvalidAction,
			containsSubstr: "action",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errMsg := tt.err.Error()
			assert.Contains(t, errMsg, tt.containsSubstr,
				"Error message should contain '%s'", tt.containsSubstr)
		})
	}
}

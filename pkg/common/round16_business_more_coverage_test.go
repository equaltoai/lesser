package common

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateStruct_MapValidationBranches(t *testing.T) {
	t.Run("required field missing", func(t *testing.T) {
		err := ValidateStruct(map[string]interface{}{"other": "x"}, ValidationRules{
			Required: []string{"req"},
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "required field missing: req")
	})

	t.Run("required field present but empty", func(t *testing.T) {
		err := ValidateStruct(map[string]interface{}{"req": ""}, ValidationRules{
			Required: []string{"req"},
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "required field missing: req")
	})

	t.Run("max length exceeded", func(t *testing.T) {
		err := ValidateStruct(map[string]interface{}{"f": "toolong"}, ValidationRules{
			MaxLen: map[string]int{"f": 3},
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "exceeds")
	})

	t.Run("min length not met", func(t *testing.T) {
		err := ValidateStruct(map[string]interface{}{"f": "ab"}, ValidationRules{
			MinLen: map[string]int{"f": 3},
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "below")
	})
}

func TestBusinessLogic_AdditionalBranches(t *testing.T) {
	ctx := context.Background()

	t.Run("ValidateResourceAccess input validation", func(t *testing.T) {
		assert.ErrorIs(t, ValidateResourceAccess(ctx, "", "r1", "note", AccessRead), ErrAuthenticationRequiredForAccess)
		assert.ErrorIs(t, ValidateResourceAccess(ctx, "a1", "", "note", AccessRead), ErrResourceIDRequiredForAccessValidation)
		assert.ErrorIs(t, ValidateResourceAccess(ctx, "a1", "r1", "", AccessRead), ErrResourceTypeRequiredForAccessValidation)
		assert.NoError(t, ValidateResourceAccess(ctx, "a1", "r1", "note", AccessNone))
		assert.Error(t, ValidateResourceAccess(ctx, "a1", "r1", "note", AccessLevel("bogus")))
	})

	t.Run("RecordBusinessMetric input validation", func(t *testing.T) {
		assert.ErrorIs(t, RecordBusinessMetric(ctx, "", "note", "a1", nil), ErrMetricTypeRequired)
		assert.ErrorIs(t, RecordBusinessMetric(ctx, "created", "note", "", nil), ErrActorIDRequiredForMetrics)
		assert.NoError(t, RecordBusinessMetric(ctx, "created", "note", "a1", map[string]interface{}{"k": "v"}))
	})

	t.Run("QuotaValidator input validation", func(t *testing.T) {
		validator := &QuotaValidator{MaxActionsPerHour: 1, MaxActionsPerDay: 1}

		assert.ErrorIs(t, validator.ValidateQuota(ctx, "", "create"), ErrActorIDRequiredForQuotaValidation)
		assert.ErrorIs(t, validator.ValidateQuota(ctx, "a1", ""), ErrActionTypeRequiredForQuotaValidation)

		assert.NoError(t, validator.ValidateQuota(ctx, "a1", "post"))
		assert.NoError(t, validator.ValidateQuota(ctx, "a1", "follow"))
	})
}

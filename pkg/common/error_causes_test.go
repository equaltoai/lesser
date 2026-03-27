package common

import (
	stdErrors "errors"
	"fmt"
	"testing"

	apperrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/stretchr/testify/require"
)

func TestErrorLeafMessages(t *testing.T) {
	rawStorageErr := stdErrors.New("dynamo conditional check failed")
	appErr := apperrors.FailedToCreate("status", rawStorageErr)
	err := stdErrors.Join(
		fmt.Errorf("create direct message: %w", appErr),
		fmt.Errorf("verify persisted note: %w", stdErrors.New("note context missing")),
	)

	require.Equal(t, []string{
		"dynamo conditional check failed",
		"note context missing",
	}, ErrorLeafMessages(err))
	require.Equal(t, "dynamo conditional check failed; note context missing", ErrorLeafSummary(err))
}

func TestWrapErrorWithLeafCauses(t *testing.T) {
	rawStorageErr := stdErrors.New("raw note map missing context")
	err := WrapErrorWithLeafCauses("verify persisted direct message", apperrors.FailedToCreate("status", rawStorageErr))

	require.ErrorContains(t, err, "verify persisted direct message")
	require.ErrorContains(t, err, "root causes: raw note map missing context")
	require.ErrorIs(t, err, rawStorageErr)
}

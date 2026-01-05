package lambda

import (
	stdErrors "errors"
	"testing"

	"github.com/equaltoai/lesser/pkg/common"
	apperrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestErrorPattern_convertLegacyError_ConvertsCommonAppError(t *testing.T) {
	ep := NewErrorPattern(zap.NewNop())

	internal := stdErrors.New("hidden")
	legacy := common.ErrForbidden(internal)

	got := ep.convertLegacyError(legacy)
	require.Equal(t, apperrors.CodeForbidden, got.Code)
	require.Equal(t, 403, got.HTTPStatusCode)
	require.Equal(t, legacy.UserMessage, got.Message)
	require.ErrorIs(t, got, internal)
}

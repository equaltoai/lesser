package lift

import (
	stdErrors "errors"
	"testing"

	apperrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/stretchr/testify/require"
)

func TestIsInsufficientScopeErrorRound12(t *testing.T) {
	require.False(t, isInsufficientScopeError(nil))

	insufficient := apperrors.NewAppError(apperrors.CodeInsufficientScope, apperrors.CategoryAuth, "insufficient scope")
	require.True(t, isInsufficientScopeError(insufficient))

	notInsufficient := apperrors.NewAppError(apperrors.CodeUnauthorized, apperrors.CategoryAuth, "unauthorized")
	require.False(t, isInsufficientScopeError(notInsufficient))

	require.True(t, isInsufficientScopeError(stdErrors.New(ErrInsufficientScope)))
}


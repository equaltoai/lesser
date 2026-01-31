package lift

import (
	stdErrors "errors"

	apperrors "github.com/equaltoai/lesser/pkg/errors"
)

func isInsufficientScopeError(err error) bool {
	if err == nil {
		return false
	}

	var appErr *apperrors.AppError
	if stdErrors.As(err, &appErr) {
		return appErr.Code == apperrors.CodeInsufficientScope
	}

	return err.Error() == ErrInsufficientScope
}

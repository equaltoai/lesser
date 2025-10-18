package notifications

import (
	stdErrors "errors"

	"github.com/equaltoai/lesser/pkg/errors"
)

// Service-specific errors for the notifications package
var (
	// General validation errors
	ErrValidationFailed = errors.ValidationFailedWithField("notification")
	ErrInvalidInput     = errors.NewValidationError("input", "invalid")

	// Notification operation errors
	ErrNotificationNotFound     = errors.NewAppError(errors.CodeNotFound, errors.CategoryBusiness, "notification not found")
	ErrNotificationAccessDenied = errors.AccessDeniedForResource("notification", "unknown")
	ErrNotificationCreateFailed = errors.FailedToCreate("notification", stdErrors.New("failed to create notification"))
	ErrNotificationUpdateFailed = errors.FailedToUpdate("notification", stdErrors.New("failed to update notification"))
	ErrNotificationClearFailed  = errors.ProcessingFailed("notification clearing", stdErrors.New("notification clearing failed"))
	ErrNotificationQueryFailed  = errors.FailedToQuery("notifications", stdErrors.New("failed to query notifications"))
	ErrNoClearCriteria          = errors.NewValidationError("clear_criteria", "no criteria specified")
	ErrNoClearMethodSpecified   = errors.NewValidationError("clear_method", "at least one criteria must be specified")
	ErrUnreadCountFailed        = errors.FailedToQuery("unread count", stdErrors.New("failed to get unread count"))
	ErrCountsByTypeFailed       = errors.FailedToQuery("counts by type", stdErrors.New("failed to get counts by type"))
)

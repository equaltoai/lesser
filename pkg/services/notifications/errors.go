package notifications

import "github.com/equaltoai/lesser/pkg/errors"

// Service-specific errors for the notifications package
var (
	// General validation errors
	ErrValidationFailed = errors.ValidationFailedWithField("notification")
	ErrInvalidInput     = errors.NewValidationError("input", "invalid")

	// Notification operation errors
	ErrNotificationNotFound     = errors.NewAppError(errors.CodeNotFound, errors.CategoryBusiness, "notification not found")
	ErrNotificationAccessDenied = errors.AccessDeniedForResource("notification", "unknown")
	ErrNotificationCreateFailed = errors.FailedToCreate("notification", nil)
	ErrNotificationUpdateFailed = errors.FailedToUpdate("notification", nil)
	ErrNotificationClearFailed  = errors.ProcessingFailed("notification clearing", nil)
	ErrNotificationQueryFailed  = errors.FailedToQuery("notifications", nil)
	ErrNoClearCriteria          = errors.NewValidationError("clear_criteria", "no criteria specified")
	ErrNoClearMethodSpecified   = errors.NewValidationError("clear_method", "at least one criteria must be specified")
	ErrUnreadCountFailed        = errors.FailedToQuery("unread count", nil)
	ErrCountsByTypeFailed       = errors.FailedToQuery("counts by type", nil)
)

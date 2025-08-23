package notifications

import "errors"

// Service-specific errors for the notifications package
var (
	// General validation errors
	ErrValidationFailed = errors.New("validation failed")
	ErrInvalidInput     = errors.New("invalid input")

	// Notification operation errors
	ErrNotificationNotFound      = errors.New("notification not found")
	ErrNotificationAccessDenied  = errors.New("access to notification denied")
	ErrNotificationCreateFailed  = errors.New("failed to create notification")
	ErrNotificationUpdateFailed  = errors.New("failed to update notification")
	ErrNotificationClearFailed   = errors.New("failed to clear notifications")
	ErrNotificationQueryFailed   = errors.New("failed to get notifications")
	ErrNoClearCriteria          = errors.New("no clear criteria specified")
	ErrNoClearMethodSpecified   = errors.New("at least one clear criteria must be specified")
	ErrUnreadCountFailed        = errors.New("failed to get unread count")
	ErrCountsByTypeFailed       = errors.New("failed to get counts by type")
)
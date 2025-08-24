package graph

import (
	"fmt"
	
	"github.com/equaltoai/lesser/pkg/errors"
)

// GraphQL resolver errors
var (
	// Authentication and authorization errors
	ErrAuthenticationRequired  = errors.NewAuthError(errors.CodeUnauthorized, "authentication required")
	ErrAccessDenied            = errors.AccessDeniedForResource("resource", "unknown")
	ErrAdminPrivilegesRequired = errors.InsufficientPermissions("admin operation")

	// Validation errors
	ErrEitherIDOrUsernameRequired = errors.NewValidationError("identifier", "either id or username must be provided")
	ErrHashtagParameterRequired   = errors.NewValidationError("hashtag", "parameter required for hashtag timeline")
	ErrListIDParameterRequired    = errors.NewValidationError("listId", "parameter required for list timeline")
	ErrTrusteeIDRequired          = errors.NewValidationError("trustee_id", "required")
	ErrObjectIDRequired           = errors.NewValidationError("objectId", "required")
	ErrReasonRequired             = errors.NewValidationError("reason", "required")
	ErrActorIDRequired            = errors.NewValidationError("actorID", "required")
	ErrEmptyURL                   = errors.NewValidationError("url", "cannot be empty")

	// Business logic validation errors
	ErrTrustScoreRange      = errors.NewValidationError("trust_score", "must be between -1.0 and 1.0")
	ErrCannotTrustYourself  = errors.OperationNotAllowedOnSelf("trust")
	ErrScheduledTimeMinimum = errors.NewValidationError("scheduled_time", "must be at least 5 minutes in the future")
	ErrScheduledTimeMaximum = errors.NewValidationError("scheduled_time", "cannot be more than 1 year in the future")
	ErrAuthorsCannotVote    = errors.OperationNotAllowedOnSelf("vote on note")
	ErrOwnerOnlyOperation   = errors.InsufficientPermissions("owner only operation")

	// Timeline type errors
	ErrUnsupportedTimelineType = errors.UnsupportedTimelineType("unknown")
	ErrAuthRequiredForHome     = errors.NewAuthError(errors.CodeUnauthorized, "authentication required for home timeline")
	ErrAuthRequiredForDirect   = errors.NewAuthError(errors.CodeUnauthorized, "authentication required for direct timeline")

	// Service unavailability errors
	ErrEventBusUnavailable         = errors.ServiceUnavailable("event bus")
	ErrInternalEventBusUnavailable = errors.EventBusNotInitialized()
	ErrModerationUnavailable       = errors.ServiceUnavailable("moderation service")
	ErrAIServiceUnavailable        = errors.ServiceNotAvailable("AI service")
	ErrAnalyticsUnavailable        = errors.ServiceUnavailable("analytics service")
	ErrMediaServiceUnavailable     = errors.ServiceUnavailable("media service")
	ErrStorageUnavailable          = errors.ServiceUnavailable("storage")
	ErrFederationUnavailable       = errors.ServiceUnavailable("federation service")
	ErrCostTrackingUnavailable     = errors.ServiceUnavailable("cost tracking")

	// Repository unavailability errors
	ErrObjectRepositoryUnavailable     = errors.RepositoryNotAvailable("object")
	ErrModerationRepositoryUnavailable = errors.RepositoryNotAvailable("moderation")
	ErrStatusRepositoryUnavailable     = errors.RepositoryNotAvailable("status")
	ErrAnalyticsRepositoryUnavailable  = errors.RepositoryNotAvailable("analytics")
	ErrActorRepositoryUnavailable      = errors.RepositoryNotAvailable("actor")
	ErrTrustRepositoryUnavailable      = errors.RepositoryNotAvailable("trust")
	ErrFederationRepositoryUnavailable = errors.RepositoryNotAvailable("federation")

	// Not found errors
	ErrObjectNotFound             = errors.NewAppError(errors.CodeNotFound, errors.CategoryBusiness, "object not found")
	ErrCommunityNoteNotFound      = errors.NewAppError(errors.CodeNotFound, errors.CategoryBusiness, "community note not found")
	ErrListNotFoundOrAccessDenied = errors.NewAppError(errors.CodeNotFound, errors.CategoryAuth, "list not found or access denied")
	ErrModerationPatternNotFound  = errors.NewAppError(errors.CodeNotFound, errors.CategoryBusiness, "moderation pattern not found")

	// Format and parsing errors
	ErrInvalidURLFormat             = errors.NewValidationError("url", "invalid format")
	ErrInvalidMediaURLFormat        = errors.NewValidationError("media_url", "invalid format")
	ErrInvalidMediaURLSegments      = errors.NewValidationError("media_url", "not enough path segments")
	ErrInvalidMediaURLMissingPrefix = errors.NewValidationError("media_url", "missing 'media' prefix")
	ErrInvalidMediaURLMissingID     = errors.NewValidationError("media_url", "missing media ID")
	ErrInvalidMediaIDFormat         = errors.NewValidationError("media_id", "invalid format")
	ErrInvalidTrustScoreKey         = errors.NewValidationError("trust_score_key", "invalid format")
	ErrNoDomainInURL                = errors.NewValidationError("url", "no domain found")

	// Content processing errors
	ErrUnableToConvertObject      = errors.ProcessingFailed("object conversion", nil)
	ErrStatusIsNotNote            = errors.NewValidationError("status_type", "is not a note")
	ErrCannotDetermineOwnership   = errors.ProcessingFailed("ownership determination", nil)
	ErrUnexpectedStatsType        = errors.NewValidationError("stats_type", "unexpected type from AI service")
	ErrCacheMiss                  = errors.NewAppError(errors.CodeNotFound, errors.CategoryInternal, "cache miss")
	ErrNoteCreationReturnedNoNote = errors.ProcessingFailed("note creation", nil)

	// Federation errors
	ErrFederationFetchRepliesUnavailable = errors.ServiceUnavailable("federation fetch replies")

	// Helper operation errors
	ErrSocialActionFailed   = errors.ProcessingFailed("social action", nil)
	ErrSocialUndoFailed     = errors.ProcessingFailed("social undo action", nil)
	ErrListMembershipFailed = errors.ProcessingFailed("list membership operation", nil)
	ErrNoAccountsProcessed  = errors.ProcessingFailed("account processing", nil)
	ErrGetUpdatedListFailed = errors.ProcessingFailed("get updated list", nil)

	// Subscription manager errors
	ErrSubscriptionManagerAlreadyRunning    = errors.InvalidStateForOperation("not running", "start subscription manager")
	ErrSubscriptionManagerNotRunning        = errors.InvalidStateForOperation("running", "stop subscription manager")
	ErrEventBusNotAvailableForTimeline      = errors.ServiceUnavailable("event bus for timeline subscription")
	ErrEventBusNotAvailableForNotifications = errors.ServiceUnavailable("event bus for notification subscription")
	ErrEventBusNotAvailableForCost          = errors.ServiceUnavailable("event bus for cost update subscription")
	ErrEventBusNotAvailableForModeration    = errors.ServiceUnavailable("event bus for moderation subscription")
	ErrEventBusNotAvailableForTrust         = errors.ServiceUnavailable("event bus for trust subscription")
	ErrEventBusNotAvailableForAI            = errors.ServiceUnavailable("event bus for AI analysis subscription")
	ErrEventBusNotAvailableForHashtag       = errors.ServiceUnavailable("event bus for hashtag activity subscription")
	ErrEventBusNotAvailableForQuote         = errors.ServiceUnavailable("event bus for quote activity subscription")
	ErrEventBusNotAvailableForMetrics       = errors.ServiceUnavailable("event bus for metrics subscription")
	ErrAtLeastOneHashtagRequired            = errors.NewValidationError("hashtags", "at least one must be specified")
	ErrNoteIDCannotBeEmpty                  = errors.NewValidationError("noteID", "cannot be empty")
	ErrUsernameCannotBeEmpty                = errors.NewValidationError("username", "cannot be empty")

	// Subscription factory errors
	ErrEventBusSubscriptionFailed = errors.EventBusSubscriptionFailed(nil)

	// Pagination errors
	ErrPaginationMixedParams = errors.NewValidationError("pagination", "cannot use first/after with last/before")
	ErrFirstMustBePositive   = errors.NewValidationError("first", "must be positive")
	ErrLastMustBePositive    = errors.NewValidationError("last", "must be positive")
	ErrInvalidCursorFormat   = errors.NewValidationError("cursor", "invalid format")
	ErrInvalidCursorData     = errors.NewValidationError("cursor", "invalid data")
	ErrInvalidAfterCursor    = errors.NewValidationError("after_cursor", "invalid")
	ErrInvalidBeforeCursor   = errors.NewValidationError("before_cursor", "invalid")
)

// ErrModerationPatternNotFoundWithID returns an error for when a moderation pattern is not found with the given ID.
func ErrModerationPatternNotFoundWithID(patternID string) *errors.AppError {
	return errors.NewAppError(errors.CodeNotFound, errors.CategoryBusiness, "moderation pattern not found").
		WithMetadata("pattern_id", patternID)
}

// ErrUnsupportedTimelineTypeWithValue returns an error for unsupported timeline types with the specific value.
func ErrUnsupportedTimelineTypeWithValue(timelineType interface{}) *errors.AppError {
	return errors.UnsupportedTimelineType(fmt.Sprintf("%v", timelineType))
}

// ErrInvalidURLFormatWithValue returns an error for invalid URL formats with the specific URL value.
func ErrInvalidURLFormatWithValue(url string) *errors.AppError {
	return errors.NewValidationError("url", "invalid format").WithMetadata("url", url)
}

// ErrNoDomainFoundInURL returns an error when no domain is found in the given URL.
func ErrNoDomainFoundInURL(url string) *errors.AppError {
	return errors.NewValidationError("url", "no domain found").WithMetadata("url", url)
}

// ErrSocialActionFailedWithContext returns an error for when a social action fails with additional context.
func ErrSocialActionFailedWithContext(actionName string, err error) *errors.AppError {
	return errors.ProcessingFailed("social action", err).WithMetadata("action", actionName)
}

// ErrSocialUndoFailedWithContext returns an error for when a social undo action fails with additional context.
func ErrSocialUndoFailedWithContext(actionName string, err error) *errors.AppError {
	return errors.ProcessingFailed("social undo action", err).WithMetadata("action", actionName)
}

// ErrListMembershipFailedWithAction returns an error for when list membership operations fail with a specific action.
func ErrListMembershipFailedWithAction(actionName string) *errors.AppError {
	return errors.ProcessingFailed("list membership operation", nil).WithMetadata("action", actionName)
}

// ErrGetUpdatedListFailedWithContext returns an error for when getting an updated list fails with additional context.
func ErrGetUpdatedListFailedWithContext(err error) *errors.AppError {
	return errors.ProcessingFailed("get updated list", err)
}

// ErrEventBusSubscriptionFailedWithContext returns an error for when event bus subscription fails with additional context.
func ErrEventBusSubscriptionFailedWithContext(err error) *errors.AppError {
	return errors.EventBusSubscriptionFailed(err)
}

// ErrFailedToConvertItem returns an error for when item conversion fails at a specific index.
func ErrFailedToConvertItem(itemIndex int, err error) *errors.AppError {
	return errors.ProcessingFailed("item conversion", err).WithMetadata("item_index", itemIndex)
}

// ErrFailedToConvertNotification returns an error for when notification conversion fails at a specific index.
func ErrFailedToConvertNotification(itemIndex int, err error) *errors.AppError {
	return errors.ProcessingFailed("notification conversion", err).WithMetadata("item_index", itemIndex)
}

// ErrFailedToConvertHashtag returns an error for when hashtag conversion fails at a specific index.
func ErrFailedToConvertHashtag(itemIndex int, err error) *errors.AppError {
	return errors.ProcessingFailed("hashtag conversion", err).WithMetadata("item_index", itemIndex)
}

// ErrFailedToConvertSeveredRelationship returns an error for when severed relationship conversion fails at a specific index.
func ErrFailedToConvertSeveredRelationship(itemIndex int, err error) *errors.AppError {
	return errors.ProcessingFailed("severed relationship conversion", err).WithMetadata("item_index", itemIndex)
}

// ErrFailedToConvertAffectedRelationship returns an error for when affected relationship conversion fails at a specific index.
func ErrFailedToConvertAffectedRelationship(itemIndex int, err error) *errors.AppError {
	return errors.ProcessingFailed("affected relationship conversion", err).WithMetadata("item_index", itemIndex)
}

// ErrInvalidCursorFormatWithContext returns an error for invalid cursor format with additional context.
func ErrInvalidCursorFormatWithContext(err error) *errors.AppError {
	return errors.NewValidationError("cursor", "invalid format").WithInternalError(err)
}

// ErrInvalidCursorDataWithContext returns an error for invalid cursor data with additional context.
func ErrInvalidCursorDataWithContext(err error) *errors.AppError {
	return errors.NewValidationError("cursor", "invalid data").WithInternalError(err)
}

// ErrInvalidAfterCursorWithContext returns an error for invalid after cursor with additional context.
func ErrInvalidAfterCursorWithContext(err error) *errors.AppError {
	return errors.NewValidationError("after_cursor", "invalid").WithInternalError(err)
}

// ErrInvalidBeforeCursorWithContext returns an error for invalid before cursor with additional context.
func ErrInvalidBeforeCursorWithContext(err error) *errors.AppError {
	return errors.NewValidationError("before_cursor", "invalid").WithInternalError(err)
}

package graph

import (
	"errors"
	"fmt"
)

// GraphQL resolver errors
var (
	// Authentication and authorization errors
	ErrAuthenticationRequired  = errors.New("authentication required")
	ErrAccessDenied            = errors.New("access denied")
	ErrAdminPrivilegesRequired = errors.New("admin privileges required")

	// Validation errors
	ErrEitherIDOrUsernameRequired = errors.New("either id or username must be provided")
	ErrHashtagParameterRequired   = errors.New("hashtag parameter required for hashtag timeline")
	ErrListIDParameterRequired    = errors.New("listId parameter required for list timeline")
	ErrTrusteeIDRequired          = errors.New("trustee_id is required")
	ErrObjectIDRequired           = errors.New("objectId is required")
	ErrReasonRequired             = errors.New("reason is required")
	ErrActorIDRequired            = errors.New("actorID is required")
	ErrEmptyURL                   = errors.New("empty URL")

	// Business logic validation errors
	ErrTrustScoreRange      = errors.New("trust score must be between -1.0 and 1.0")
	ErrCannotTrustYourself  = errors.New("cannot create trust relationship with yourself")
	ErrScheduledTimeMinimum = errors.New("scheduled time must be at least 5 minutes in the future")
	ErrScheduledTimeMaximum = errors.New("scheduled time cannot be more than 1 year in the future")
	ErrAuthorsCannotVote    = errors.New("authors cannot vote on their own notes")
	ErrOwnerOnlyOperation   = errors.New("you can only perform this operation on your own posts")

	// Timeline type errors
	ErrUnsupportedTimelineType = errors.New("unsupported timeline type")
	ErrAuthRequiredForHome     = errors.New("authentication required for home timeline")
	ErrAuthRequiredForDirect   = errors.New("authentication required for direct timeline")

	// Service unavailability errors
	ErrEventBusUnavailable         = errors.New("event bus not available")
	ErrInternalEventBusUnavailable = errors.New("internal event bus not available")
	ErrModerationUnavailable       = errors.New("moderation service not available")
	ErrAIServiceUnavailable        = errors.New("AI service not initialized")
	ErrAnalyticsUnavailable        = errors.New("analytics service not available")
	ErrMediaServiceUnavailable     = errors.New("media service not available")
	ErrStorageUnavailable          = errors.New("storage not available")
	ErrFederationUnavailable       = errors.New("federation service not available")
	ErrCostTrackingUnavailable     = errors.New("cost tracking not available")

	// Repository unavailability errors
	ErrObjectRepositoryUnavailable     = errors.New("object repository not available")
	ErrModerationRepositoryUnavailable = errors.New("moderation repository not available")
	ErrStatusRepositoryUnavailable     = errors.New("status repository not available")
	ErrAnalyticsRepositoryUnavailable  = errors.New("analytics repository not available")
	ErrActorRepositoryUnavailable      = errors.New("actor repository not available")
	ErrTrustRepositoryUnavailable      = errors.New("trust repository not available")
	ErrFederationRepositoryUnavailable = errors.New("federation repository not available")

	// Not found errors
	ErrObjectNotFound             = errors.New("object not found")
	ErrCommunityNoteNotFound      = errors.New("community note not found")
	ErrListNotFoundOrAccessDenied = errors.New("list not found or access denied")
	ErrModerationPatternNotFound  = errors.New("moderation pattern not found")

	// Format and parsing errors
	ErrInvalidURLFormat             = errors.New("invalid URL format")
	ErrInvalidMediaURLFormat        = errors.New("invalid media URL format")
	ErrInvalidMediaURLSegments      = errors.New("invalid media URL format: not enough path segments")
	ErrInvalidMediaURLMissingPrefix = errors.New("invalid media URL format: missing 'media' prefix")
	ErrInvalidMediaURLMissingID     = errors.New("invalid media URL format: missing media ID")
	ErrInvalidMediaIDFormat         = errors.New("invalid media ID format")
	ErrInvalidTrustScoreKey         = errors.New("invalid trust score key format")
	ErrNoDomainInURL                = errors.New("no domain found in URL")

	// Content processing errors
	ErrUnableToConvertObject      = errors.New("unable to convert object to GraphQL model")
	ErrStatusIsNotNote            = errors.New("status is not a note")
	ErrCannotDetermineOwnership   = errors.New("cannot determine status ownership")
	ErrUnexpectedStatsType        = errors.New("unexpected stats type from AI service")
	ErrCacheMiss                  = errors.New("cache miss")
	ErrNoteCreationReturnedNoNote = errors.New("note creation returned no note")

	// Federation errors
	ErrFederationFetchRepliesUnavailable = errors.New("federation fetch replies not available")

	// Helper operation errors
	ErrSocialActionFailed   = errors.New("failed to execute social action")
	ErrSocialUndoFailed     = errors.New("failed to execute social undo action")
	ErrListMembershipFailed = errors.New("failed to execute list membership operation")
	ErrNoAccountsProcessed  = errors.New("failed to process any accounts")
	ErrGetUpdatedListFailed = errors.New("failed to get updated list")

	// Subscription manager errors
	ErrSubscriptionManagerAlreadyRunning    = errors.New("subscription manager is already running")
	ErrSubscriptionManagerNotRunning        = errors.New("subscription manager is not running")
	ErrEventBusNotAvailableForTimeline      = errors.New("event bus is not available for timeline subscription")
	ErrEventBusNotAvailableForNotifications = errors.New("event bus is not available for notification subscription")
	ErrEventBusNotAvailableForCost          = errors.New("event bus is not available for cost update subscription")
	ErrEventBusNotAvailableForModeration    = errors.New("event bus is not available for moderation subscription")
	ErrEventBusNotAvailableForTrust         = errors.New("event bus is not available for trust subscription")
	ErrEventBusNotAvailableForAI            = errors.New("event bus is not available for AI analysis subscription")
	ErrEventBusNotAvailableForHashtag       = errors.New("event bus is not available for hashtag activity subscription")
	ErrEventBusNotAvailableForQuote         = errors.New("event bus is not available for quote activity subscription")
	ErrEventBusNotAvailableForMetrics       = errors.New("event bus is not available for metrics subscription")
	ErrAtLeastOneHashtagRequired            = errors.New("at least one hashtag must be specified")
	ErrNoteIDCannotBeEmpty                  = errors.New("noteID cannot be empty")
	ErrUsernameCannotBeEmpty                = errors.New("username cannot be empty")

	// Subscription factory errors
	ErrEventBusSubscriptionFailed = errors.New("failed to subscribe to event bus")

	// Pagination errors
	ErrPaginationMixedParams = errors.New("cannot use first/after with last/before")
	ErrFirstMustBePositive   = errors.New("first must be positive")
	ErrLastMustBePositive    = errors.New("last must be positive")
	ErrInvalidCursorFormat   = errors.New("invalid cursor format")
	ErrInvalidCursorData     = errors.New("invalid cursor data")
	ErrInvalidAfterCursor    = errors.New("invalid after cursor")
	ErrInvalidBeforeCursor   = errors.New("invalid before cursor")
)

// ErrModerationPatternNotFoundWithID returns an error for when a moderation pattern is not found with the given ID.
func ErrModerationPatternNotFoundWithID(patternID string) error {
	return errors.Join(ErrModerationPatternNotFound, errors.New(patternID))
}

// ErrUnsupportedTimelineTypeWithValue returns an error for unsupported timeline types with the specific value.
func ErrUnsupportedTimelineTypeWithValue(timelineType interface{}) error {
	return errors.Join(ErrUnsupportedTimelineType, fmt.Errorf("%v", timelineType))
}

// ErrInvalidURLFormatWithValue returns an error for invalid URL formats with the specific URL value.
func ErrInvalidURLFormatWithValue(url string) error {
	return errors.Join(ErrInvalidURLFormat, errors.New(url))
}

// ErrNoDomainFoundInURL returns an error when no domain is found in the given URL.
func ErrNoDomainFoundInURL(url string) error {
	return errors.Join(ErrNoDomainInURL, errors.New(url))
}

// ErrSocialActionFailedWithContext returns an error for when a social action fails with additional context.
func ErrSocialActionFailedWithContext(actionName string, err error) error {
	return errors.Join(fmt.Errorf("failed to %s object", actionName), err)
}

// ErrSocialUndoFailedWithContext returns an error for when a social undo action fails with additional context.
func ErrSocialUndoFailedWithContext(actionName string, err error) error {
	return errors.Join(fmt.Errorf("failed to %s object", actionName), err)
}

// ErrListMembershipFailedWithAction returns an error for when list membership operations fail with a specific action.
func ErrListMembershipFailedWithAction(actionName string) error {
	return errors.Join(ErrListMembershipFailed, errors.New(actionName))
}

// ErrGetUpdatedListFailedWithContext returns an error for when getting an updated list fails with additional context.
func ErrGetUpdatedListFailedWithContext(err error) error {
	return errors.Join(errors.New("failed to get updated list"), err)
}

// ErrEventBusSubscriptionFailedWithContext returns an error for when event bus subscription fails with additional context.
func ErrEventBusSubscriptionFailedWithContext(err error) error {
	return errors.Join(errors.New("failed to subscribe to event bus"), err)
}

// ErrFailedToConvertItem returns an error for when item conversion fails at a specific index.
func ErrFailedToConvertItem(itemIndex int, err error) error {
	return errors.Join(fmt.Errorf("failed to convert item %d", itemIndex), err)
}

// ErrFailedToConvertNotification returns an error for when notification conversion fails at a specific index.
func ErrFailedToConvertNotification(itemIndex int, err error) error {
	return errors.Join(fmt.Errorf("failed to convert notification %d", itemIndex), err)
}

// ErrFailedToConvertHashtag returns an error for when hashtag conversion fails at a specific index.
func ErrFailedToConvertHashtag(itemIndex int, err error) error {
	return errors.Join(fmt.Errorf("failed to convert hashtag %d", itemIndex), err)
}

// ErrFailedToConvertSeveredRelationship returns an error for when severed relationship conversion fails at a specific index.
func ErrFailedToConvertSeveredRelationship(itemIndex int, err error) error {
	return errors.Join(fmt.Errorf("failed to convert severed relationship %d", itemIndex), err)
}

// ErrFailedToConvertAffectedRelationship returns an error for when affected relationship conversion fails at a specific index.
func ErrFailedToConvertAffectedRelationship(itemIndex int, err error) error {
	return errors.Join(fmt.Errorf("failed to convert affected relationship %d", itemIndex), err)
}

// ErrInvalidCursorFormatWithContext returns an error for invalid cursor format with additional context.
func ErrInvalidCursorFormatWithContext(err error) error {
	return errors.Join(ErrInvalidCursorFormat, err)
}

// ErrInvalidCursorDataWithContext returns an error for invalid cursor data with additional context.
func ErrInvalidCursorDataWithContext(err error) error {
	return errors.Join(ErrInvalidCursorData, err)
}

// ErrInvalidAfterCursorWithContext returns an error for invalid after cursor with additional context.
func ErrInvalidAfterCursorWithContext(err error) error {
	return errors.Join(ErrInvalidAfterCursor, err)
}

// ErrInvalidBeforeCursorWithContext returns an error for invalid before cursor with additional context.
func ErrInvalidBeforeCursorWithContext(err error) error {
	return errors.Join(ErrInvalidBeforeCursor, err)
}

package activitypub

import (
	"errors"

	apperrors "github.com/equaltoai/lesser/pkg/errors"
)

// Legacy error variables for backwards compatibility
// These are now wrappers around the centralized error system
var (
	// ErrInvalidJSON is returned when JSON validation fails before parsing
	ErrInvalidJSON = apperrors.JSONFormatInvalid("invalid ActivityPub JSON")

	// ErrParseActivity is returned when activity parsing fails
	ErrParseActivity = apperrors.ActivityParsingFailed("activity", errors.New("activity parsing failed"))

	// ErrParseActor is returned when actor parsing fails
	ErrParseActor = apperrors.ActivityParsingFailed("actor", errors.New("actor parsing failed"))

	// ErrParseNote is returned when note parsing fails
	ErrParseNote = apperrors.ActivityParsingFailed("note", errors.New("note parsing failed"))

	// ErrInvalidActivity is returned when activity validation fails
	ErrInvalidActivity = apperrors.InvalidActivityObject()

	// ErrInvalidActor is returned when actor validation fails
	ErrInvalidActor = apperrors.NewValidationError("actor", "Invalid actor")

	// ErrInvalidNote is returned when note validation fails
	ErrInvalidNote = apperrors.NewValidationError("note", "Invalid note")

	// ErrEmptyRecipient is returned when a recipient field is empty
	ErrEmptyRecipient = apperrors.RequiredFieldMissing("recipient")

	// ErrInvalidRecipientURL is returned when a recipient URL is malformed
	ErrInvalidRecipientURL = apperrors.InvalidFormat("recipient_url", "valid URL format")

	// ErrInvalidURLScheme is returned when a recipient URL doesn't use HTTP(S)
	ErrInvalidURLScheme = apperrors.URLSchemeNotAllowed("", "")

	// ErrInvalidRecipientFormat is returned when a recipient URL format is invalid
	ErrInvalidRecipientFormat = apperrors.InvalidFormat("recipient_url", "valid URL format")

	// ErrDirectMessagePublicAddressing is returned when direct messages have public addressing
	ErrDirectMessagePublicAddressing = apperrors.BusinessRuleViolated("direct message addressing", map[string]interface{}{"rule": "no public addressing"})

	// ErrBCCInVisibleFields is returned when BCC recipients appear in visible addressing fields
	ErrBCCInVisibleFields = apperrors.BusinessRuleViolated("BCC privacy", map[string]interface{}{"rule": "BCC not in visible fields"})

	// Validation errors
	// ErrEmptyDomain is returned when domain is empty
	ErrEmptyDomain = apperrors.RequiredFieldMissing("domain")

	// ErrIPAddressAsDomain is returned when an IP address is used as a domain
	ErrIPAddressAsDomain = apperrors.NewValidationError("domain", "IP addresses cannot be used as domains")

	// ErrInvalidDomainFormat is returned when domain format is invalid
	ErrInvalidDomainFormat = apperrors.InvalidFormat("domain", "valid domain format")

	// ErrConsecutiveDots is returned when domain contains consecutive dots
	ErrConsecutiveDots = apperrors.NewValidationError("domain", "Domain cannot contain consecutive dots")

	// ErrInvalidActorIDURL is returned when actor ID URL is malformed
	ErrInvalidActorIDURL = apperrors.InvalidFormat("actor_id_url", "valid actor ID URL")

	// ErrActorIDScheme is returned when actor ID doesn't use HTTP(S)
	ErrActorIDScheme = apperrors.URLSchemeNotAllowed("", "")

	// ErrActorIDMissingPath is returned when actor ID has no path
	ErrActorIDMissingPath = apperrors.NewValidationError("actor_id", "Actor ID must have a path")

	// ErrInvalidWebfingerFormat is returned when webfinger format is invalid
	ErrInvalidWebfingerFormat = apperrors.InvalidFormat("webfinger", "acct:user@domain")

	// ErrInvalidDomainInActorID is returned when actor ID contains invalid domain
	ErrInvalidDomainInActorID = apperrors.NewValidationError("actor_id", "Invalid domain in actor ID")

	// ErrInvalidUsernameInWebfinger is returned when webfinger contains invalid username
	ErrInvalidUsernameInWebfinger = apperrors.NewValidationError("webfinger", "Invalid username")

	// ErrInvalidDomainInWebfinger is returned when webfinger contains invalid domain
	ErrInvalidDomainInWebfinger = apperrors.NewValidationError("webfinger", "Invalid domain")

	// ErrInvalidActivityType is returned when activity type is not recognized
	ErrInvalidActivityType = apperrors.NewValidationError("activity_type", "Invalid activity type")
)

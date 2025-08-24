package activitypub

import "github.com/equaltoai/lesser/pkg/errors"

// Legacy error variables for backwards compatibility
// These are now wrappers around the centralized error system
var (
	// ErrInvalidJSON is returned when JSON validation fails before parsing
	ErrInvalidJSON = errors.JSONFormatInvalid("invalid ActivityPub JSON")

	// ErrParseActivity is returned when activity parsing fails
	ErrParseActivity = errors.ActivityParsingFailed("activity", nil)

	// ErrParseActor is returned when actor parsing fails
	ErrParseActor = errors.ActivityParsingFailed("actor", nil)

	// ErrParseNote is returned when note parsing fails
	ErrParseNote = errors.ActivityParsingFailed("note", nil)

	// ErrInvalidActivity is returned when activity validation fails
	ErrInvalidActivity = errors.InvalidActivityObject()

	// ErrInvalidActor is returned when actor validation fails
	ErrInvalidActor = errors.NewValidationError("actor", "Invalid actor")

	// ErrInvalidNote is returned when note validation fails
	ErrInvalidNote = errors.NewValidationError("note", "Invalid note")

	// ErrEmptyRecipient is returned when a recipient field is empty
	ErrEmptyRecipient = errors.RequiredFieldMissing("recipient")

	// ErrInvalidRecipientURL is returned when a recipient URL is malformed
	ErrInvalidRecipientURL = errors.InvalidFormat("recipient_url", "valid URL format")

	// ErrInvalidURLScheme is returned when a recipient URL doesn't use HTTP(S)
	ErrInvalidURLScheme = errors.URLSchemeNotAllowed("", "")

	// ErrInvalidRecipientFormat is returned when a recipient URL format is invalid
	ErrInvalidRecipientFormat = errors.InvalidFormat("recipient_url", "valid URL format")

	// ErrDirectMessagePublicAddressing is returned when direct messages have public addressing
	ErrDirectMessagePublicAddressing = errors.BusinessRuleViolated("direct message addressing", map[string]interface{}{"rule": "no public addressing"})

	// ErrBCCInVisibleFields is returned when BCC recipients appear in visible addressing fields
	ErrBCCInVisibleFields = errors.BusinessRuleViolated("BCC privacy", map[string]interface{}{"rule": "BCC not in visible fields"})

	// Validation errors
	// ErrEmptyDomain is returned when domain is empty
	ErrEmptyDomain = errors.RequiredFieldMissing("domain")

	// ErrIPAddressAsDomain is returned when an IP address is used as a domain
	ErrIPAddressAsDomain = errors.NewValidationError("domain", "IP addresses cannot be used as domains")

	// ErrInvalidDomainFormat is returned when domain format is invalid
	ErrInvalidDomainFormat = errors.InvalidFormat("domain", "valid domain format")

	// ErrConsecutiveDots is returned when domain contains consecutive dots
	ErrConsecutiveDots = errors.NewValidationError("domain", "Domain cannot contain consecutive dots")

	// ErrInvalidActorIDURL is returned when actor ID URL is malformed
	ErrInvalidActorIDURL = errors.InvalidFormat("actor_id_url", "valid actor ID URL")

	// ErrActorIDScheme is returned when actor ID doesn't use HTTP(S)
	ErrActorIDScheme = errors.URLSchemeNotAllowed("", "")

	// ErrActorIDMissingPath is returned when actor ID has no path
	ErrActorIDMissingPath = errors.NewValidationError("actor_id", "Actor ID must have a path")

	// ErrInvalidWebfingerFormat is returned when webfinger format is invalid
	ErrInvalidWebfingerFormat = errors.InvalidFormat("webfinger", "acct:user@domain")

	// ErrInvalidDomainInActorID is returned when actor ID contains invalid domain
	ErrInvalidDomainInActorID = errors.NewValidationError("actor_id", "Invalid domain in actor ID")

	// ErrInvalidUsernameInWebfinger is returned when webfinger contains invalid username
	ErrInvalidUsernameInWebfinger = errors.NewValidationError("webfinger", "Invalid username")

	// ErrInvalidDomainInWebfinger is returned when webfinger contains invalid domain
	ErrInvalidDomainInWebfinger = errors.NewValidationError("webfinger", "Invalid domain")

	// ErrInvalidActivityType is returned when activity type is not recognized
	ErrInvalidActivityType = errors.NewValidationError("activity_type", "Invalid activity type")
)

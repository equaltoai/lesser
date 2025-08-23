package activitypub

import "errors"

// ActivityPub parsing and validation errors
var (
	// ErrInvalidJSON is returned when JSON validation fails before parsing
	ErrInvalidJSON = errors.New("invalid JSON")

	// ErrParseActivity is returned when activity parsing fails
	ErrParseActivity = errors.New("failed to parse activity")

	// ErrParseActor is returned when actor parsing fails
	ErrParseActor = errors.New("failed to parse actor")

	// ErrParseNote is returned when note parsing fails
	ErrParseNote = errors.New("failed to parse note")

	// ErrInvalidActivity is returned when activity validation fails
	ErrInvalidActivity = errors.New("invalid activity")

	// ErrInvalidActor is returned when actor validation fails
	ErrInvalidActor = errors.New("invalid actor")

	// ErrInvalidNote is returned when note validation fails
	ErrInvalidNote = errors.New("invalid note")

	// ErrEmptyRecipient is returned when a recipient field is empty
	ErrEmptyRecipient = errors.New("empty recipient")

	// ErrInvalidRecipientURL is returned when a recipient URL is malformed
	ErrInvalidRecipientURL = errors.New("invalid recipient URL")

	// ErrInvalidURLScheme is returned when a recipient URL doesn't use HTTP(S)
	ErrInvalidURLScheme = errors.New("recipient URL must use HTTP(S) scheme")

	// ErrInvalidRecipientFormat is returned when a recipient URL format is invalid
	ErrInvalidRecipientFormat = errors.New("invalid recipient URL format")

	// ErrDirectMessagePublicAddressing is returned when direct messages have public addressing
	ErrDirectMessagePublicAddressing = errors.New("direct messages cannot have public addressing")

	// ErrBCCInVisibleFields is returned when BCC recipients appear in visible addressing fields
	ErrBCCInVisibleFields = errors.New("BCC recipient appears in visible addressing fields")

	// Validation errors
	// ErrEmptyDomain is returned when domain is empty
	ErrEmptyDomain = errors.New("domain cannot be empty")

	// ErrIPAddressAsDomain is returned when an IP address is used as a domain
	ErrIPAddressAsDomain = errors.New("IP addresses cannot be used as domains")

	// ErrInvalidDomainFormat is returned when domain format is invalid
	ErrInvalidDomainFormat = errors.New("invalid domain format")

	// ErrConsecutiveDots is returned when domain contains consecutive dots
	ErrConsecutiveDots = errors.New("invalid domain: consecutive dots")

	// ErrInvalidActorIDURL is returned when actor ID URL is malformed
	ErrInvalidActorIDURL = errors.New("invalid actor ID URL")

	// ErrActorIDScheme is returned when actor ID doesn't use HTTP(S)
	ErrActorIDScheme = errors.New("actor ID must use HTTP(S)")

	// ErrActorIDMissingPath is returned when actor ID has no path
	ErrActorIDMissingPath = errors.New("actor ID must have a path")

	// ErrInvalidWebfingerFormat is returned when webfinger format is invalid
	ErrInvalidWebfingerFormat = errors.New("invalid webfinger format (expected acct:user@domain)")

	// ErrInvalidDomainInActorID is returned when actor ID contains invalid domain
	ErrInvalidDomainInActorID = errors.New("invalid domain in actor ID")

	// ErrInvalidUsernameInWebfinger is returned when webfinger contains invalid username
	ErrInvalidUsernameInWebfinger = errors.New("invalid username in webfinger")

	// ErrInvalidDomainInWebfinger is returned when webfinger contains invalid domain
	ErrInvalidDomainInWebfinger = errors.New("invalid domain in webfinger")

	// ErrInvalidActivityType is returned when activity type is not recognized
	ErrInvalidActivityType = errors.New("invalid activity type")
)

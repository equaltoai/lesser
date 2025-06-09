package activitypub

import (
	"fmt"
	"net/url"

	"github.com/aron23/lesser/pkg/common"
	"github.com/microcosm-cc/bluemonday"
)

var (
	// Maximum lengths
	maxUsernameLength    = 64
	maxDisplayNameLength = 255
	maxSummaryLength     = 5000
	maxContentLength     = 100000
)

// Create strict sanitizers for user-generated content
var (
	strictSanitizer  *bluemonday.Policy
	relaxedSanitizer *bluemonday.Policy
)

func init() {
	// Initialize strict sanitizer for user-generated content
	strictSanitizer = bluemonday.UGCPolicy()
	// Allow rel attribute on links (for rel="nofollow" etc)
	strictSanitizer.AllowAttrs("rel").Matching(bluemonday.SpaceSeparatedTokens).OnElements("a")
	// Allow class attribute on spans for styling
	strictSanitizer.AllowAttrs("class").Matching(bluemonday.SpaceSeparatedTokens).OnElements("span")

	// Initialize relaxed sanitizer for more trusted content
	relaxedSanitizer = bluemonday.UGCPolicy()
	relaxedSanitizer.AllowAttrs("class").Matching(bluemonday.SpaceSeparatedTokens).OnElements("div", "span", "p")
	relaxedSanitizer.AllowAttrs("rel").Matching(bluemonday.SpaceSeparatedTokens).OnElements("a")
}

// ValidateActor validates an Actor object
func ValidateActor(actor *Actor) error {
	if actor == nil {
		return common.ValidationError{
			Field:   "actor",
			Message: "cannot be nil",
		}
	}

	// Validate required fields
	if actor.ID == "" {
		return common.ValidationError{
			Field:   "id",
			Message: "required field missing",
		}
	}

	if err := ValidateURL(actor.ID, "id"); err != nil {
		return err
	}

	if actor.Type == "" {
		return common.ValidationError{
			Field:   "type",
			Message: "required field missing",
		}
	}

	// Validate actor type
	switch actor.Type {
	case PersonType, ServiceType, GroupType, OrganizationType, ApplicationType:
		// Valid types
	default:
		return common.ValidationError{
			Field:   "type",
			Message: "invalid actor type",
		}
	}

	if err := ValidateUsername(actor.PreferredUsername); err != nil {
		return err
	}

	// Validate URLs
	if err := ValidateURL(actor.Inbox, "inbox"); err != nil {
		return err
	}

	if err := ValidateURL(actor.Outbox, "outbox"); err != nil {
		return err
	}

	// Validate optional fields if present
	if actor.Name != "" && len(actor.Name) > maxDisplayNameLength {
		return common.ValidationError{
			Field:   "name",
			Message: "too long (max 255 characters)",
		}
	}

	if actor.Summary != "" && len(actor.Summary) > maxSummaryLength {
		return common.ValidationError{
			Field:   "summary",
			Message: "too long (max 5000 characters)",
		}
	}

	if actor.Following != "" {
		if err := ValidateURL(actor.Following, "following"); err != nil {
			return err
		}
	}

	if actor.Followers != "" {
		if err := ValidateURL(actor.Followers, "followers"); err != nil {
			return err
		}
	}

	return nil
}

// ValidateActivity validates an Activity object
func ValidateActivity(activity *Activity) error {
	if activity == nil {
		return common.ValidationError{
			Field:   "activity",
			Message: "cannot be nil",
		}
	}

	// Validate required fields
	if activity.ID == "" {
		return common.ValidationError{
			Field:   "id",
			Message: "required field missing",
		}
	}

	if err := ValidateURL(activity.ID, "id"); err != nil {
		return err
	}

	if activity.Type == "" {
		return common.ValidationError{
			Field:   "type",
			Message: "required field missing",
		}
	}

	// Validate activity type
	switch activity.Type {
	case CreateType, UpdateType, DeleteType, FollowType, AcceptType,
		RejectType, LikeType, AnnounceType, UndoType, BlockType:
		// Valid types
	default:
		return common.ValidationError{
			Field:   "type",
			Message: fmt.Sprintf("invalid activity type: %s", activity.Type),
		}
	}

	if activity.Actor == "" {
		return common.ValidationError{
			Field:   "actor",
			Message: "required field missing",
		}
	}

	if err := ValidateURL(activity.Actor, "actor"); err != nil {
		return err
	}

	// Validate addressing
	if err := ValidateAddressing(activity.To, "to"); err != nil {
		return err
	}

	if err := ValidateAddressing(activity.CC, "cc"); err != nil {
		return err
	}

	return nil
}

// ValidateNote validates a Note object
func ValidateNote(note *Note) error {
	if note == nil {
		return common.ValidationError{
			Field:   "note",
			Message: "cannot be nil",
		}
	}

	if note.Type != NoteType {
		return common.ValidationError{
			Field:   "type",
			Message: "must be 'Note'",
		}
	}

	if note.Content == "" {
		return common.ValidationError{
			Field:   "content",
			Message: "cannot be empty",
		}
	}

	if len(note.Content) > maxContentLength {
		return common.ValidationError{
			Field:   "content",
			Message: "too long (max 100000 characters)",
		}
	}

	if note.AttributedTo == "" {
		return common.ValidationError{
			Field:   "attributedTo",
			Message: "required field missing",
		}
	}

	if err := ValidateURL(note.AttributedTo, "attributedTo"); err != nil {
		return err
	}

	return nil
}

// ValidateURL validates that a string is a valid URL
func ValidateURL(urlStr string, fieldName string) error {
	if urlStr == "" {
		return common.ValidationError{
			Field:   fieldName,
			Message: "required field missing",
		}
	}

	u, err := url.Parse(urlStr)
	if err != nil {
		return common.ValidationError{
			Field:   fieldName,
			Message: "invalid URL format",
		}
	}

	if u.Scheme != "http" && u.Scheme != "https" {
		return common.ValidationError{
			Field:   fieldName,
			Message: "URL must use http or https scheme",
		}
	}

	if u.Host == "" {
		return common.ValidationError{
			Field:   fieldName,
			Message: "URL must have a host",
		}
	}

	return nil
}

// ValidateAddressing validates addressing fields (to, cc, etc.)
func ValidateAddressing(addresses []string, fieldName string) error {
	for i, addr := range addresses {
		// Special case for public addressing
		if addr == PublicAddress {
			continue
		}

		// Otherwise must be a valid URL
		if err := ValidateURL(addr, fieldName); err != nil {
			return common.ValidationError{
				Field:   fieldName,
				Message: "invalid address at index " + string(rune(i)),
			}
		}
	}

	return nil
}

// SanitizeHTML removes potentially dangerous HTML from content
// Uses bluemonday for robust XSS prevention
func SanitizeHTML(content string) string {
	if content == "" {
		return ""
	}
	// Use strict sanitizer for all user-generated content
	return strictSanitizer.Sanitize(content)
}

// SanitizeHTMLRelaxed applies a more relaxed sanitization policy
// Only use this for content from trusted sources
func SanitizeHTMLRelaxed(content string) string {
	if content == "" {
		return ""
	}
	return relaxedSanitizer.Sanitize(content)
}

package activitypub

import (
	"fmt"
	"net/url"
	"strings"
	"github.com/equaltoai/lesser/pkg/common"
)

// AddressingValidator provides validation for ActivityPub addressing fields
type AddressingValidator struct{}

// NewAddressingValidator creates a new addressing validator
func NewAddressingValidator() *AddressingValidator {
	return &AddressingValidator{}
}

// ValidateAddressing validates the addressing fields of an activity
func (v *AddressingValidator) ValidateAddressing(activity *Activity) error {
	// Validate To field
	if err := v.validateRecipientList(activity.To, "to"); err != nil {
		return err
	}

	// Validate CC field
	if err := v.validateRecipientList(activity.CC, "cc"); err != nil {
		return err
	}

	// Validate BTo field
	if err := v.validateRecipientList(activity.BTo, "bto"); err != nil {
		return err
	}

	// Validate BCC field
	if err := v.validateRecipientList(activity.BCC, "bcc"); err != nil {
		return err
	}

	// Validate the object's addressing if it's a Note or other addressable object
	if activity.Object != nil {
		if note, ok := activity.Object.(*Note); ok {
			return v.validateNoteAddressing(note)
		}
	}

	return nil
}

// validateRecipientList validates a list of recipients
func (v *AddressingValidator) validateRecipientList(recipients []string, fieldName string) error {
	for _, recipient := range recipients {
		if err := v.validateRecipient(recipient, fieldName); err != nil {
			return err
		}
	}
	return nil
}

// validateRecipient validates a single recipient address
func (v *AddressingValidator) validateRecipient(recipient, fieldName string) error {
	if err := common.ValidateRequiredParam("recipient", recipient); err != nil {
		return fmt.Errorf("empty recipient in %s field", fieldName)
	}

	// Check if it's the special Public address
	if recipient == PublicAddress {
		return nil
	}

	// Check if it's a valid URL
	parsed, err := url.Parse(recipient)
	if err != nil {
		return fmt.Errorf("invalid URL in %s field: %s", fieldName, recipient)
	}

	// Must be HTTPS for federation
	if parsed.Scheme != "https" && parsed.Scheme != "http" {
		return fmt.Errorf("recipient URL must use HTTP(S) scheme: %s", recipient)
	}

	// Basic validation that it looks like an actor or collection URL
	if !v.isValidRecipientURL(recipient) {
		return fmt.Errorf("invalid recipient URL format: %s", recipient)
	}

	return nil
}

// validateNoteAddressing validates addressing fields on a Note object
func (v *AddressingValidator) validateNoteAddressing(note *Note) error {
	if err := v.validateRecipientList(note.To, "note.to"); err != nil {
		return err
	}
	if err := v.validateRecipientList(note.CC, "note.cc"); err != nil {
		return err
	}
	if err := v.validateRecipientList(note.BTo, "note.bto"); err != nil {
		return err
	}
	if err := v.validateRecipientList(note.BCC, "note.bcc"); err != nil {
		return err
	}
	return nil
}

// isValidRecipientURL checks if a URL looks like a valid recipient (actor or collection)
func (v *AddressingValidator) isValidRecipientURL(recipient string) bool {
	// Common patterns for valid recipients:
	// - Actor URLs: /users/username, /actors/username
	// - Collection URLs: /users/username/followers, /users/username/inbox
	// - Shared inbox: /inbox

	validPatterns := []string{
		"/users/", "/actors/", "/inbox", "/followers", "/following",
		"/user/", "/actor/", "/person/", "/@", // Common variations
	}

	for _, pattern := range validPatterns {
		if strings.Contains(recipient, pattern) {
			return true
		}
	}

	return false
}

// DetermineDeliveryRecipients determines who should receive the activity with shared inbox optimization
func (v *AddressingValidator) DetermineDeliveryRecipients(activity *Activity) *DeliveryTargets {
	targets := &DeliveryTargets{
		DirectRecipients: make(map[string]bool),
		SharedInboxes:    make(map[string]bool),
		DomainGroups:     make(map[string][]string),
	}

	// Collect all recipients from To, CC, BTo, BCC
	allRecipients := make(map[string]bool)

	for _, recipient := range activity.To {
		if recipient != PublicAddress {
			allRecipients[recipient] = true
			targets.DirectRecipients[recipient] = true
			v.groupRecipientsByDomain(recipient, targets)
		}
	}

	for _, recipient := range activity.CC {
		if recipient != PublicAddress {
			allRecipients[recipient] = true
			targets.DirectRecipients[recipient] = true
			v.groupRecipientsByDomain(recipient, targets)
		}
	}

	for _, recipient := range activity.BTo {
		if recipient != PublicAddress {
			allRecipients[recipient] = true
			targets.DirectRecipients[recipient] = true
			v.groupRecipientsByDomain(recipient, targets)
		}
	}

	for _, recipient := range activity.BCC {
		if recipient != PublicAddress {
			allRecipients[recipient] = true
			targets.DirectRecipients[recipient] = true
			v.groupRecipientsByDomain(recipient, targets)
		}
	}

	return targets
}

// groupRecipientsByDomain groups recipients by domain for shared inbox optimization
func (v *AddressingValidator) groupRecipientsByDomain(recipient string, targets *DeliveryTargets) {
	parsed, err := url.Parse(recipient)
	if err != nil {
		return
	}

	domain := parsed.Host
	if targets.DomainGroups[domain] == nil {
		targets.DomainGroups[domain] = make([]string, 0)
	}
	targets.DomainGroups[domain] = append(targets.DomainGroups[domain], recipient)
}

// DeliveryTargets represents the recipients for federation delivery
type DeliveryTargets struct {
	DirectRecipients map[string]bool     // Individual actor inboxes
	SharedInboxes    map[string]bool     // Shared inboxes (domain-level)
	DomainGroups     map[string][]string // Recipients grouped by domain for shared inbox optimization
}

// GetAllRecipients returns all unique recipients
func (dt *DeliveryTargets) GetAllRecipients() []string {
	recipients := make([]string, 0, len(dt.DirectRecipients)+len(dt.SharedInboxes))

	for recipient := range dt.DirectRecipients {
		recipients = append(recipients, recipient)
	}

	for inbox := range dt.SharedInboxes {
		recipients = append(recipients, inbox)
	}

	return recipients
}

// IsDirectMessage checks if an activity represents a direct message
func (v *AddressingValidator) IsDirectMessage(activity *Activity) bool {
	// Direct messages have no public addressing and only specific recipients
	hasPublic := v.hasPublicAddressing(activity)
	hasFollowersCollection := v.hasFollowersCollection(activity)

	return !hasPublic && !hasFollowersCollection
}

// IsPrivateMessage checks if an activity is private/followers-only
func (v *AddressingValidator) IsPrivateMessage(activity *Activity) bool {
	hasPublic := v.hasPublicAddressing(activity)
	hasFollowersCollection := v.hasFollowersCollection(activity)

	return !hasPublic && hasFollowersCollection
}

// IsPublicMessage checks if an activity is public
func (v *AddressingValidator) IsPublicMessage(activity *Activity) bool {
	return v.hasPublicInTo(activity)
}

// IsUnlistedMessage checks if an activity is unlisted
func (v *AddressingValidator) IsUnlistedMessage(activity *Activity) bool {
	return !v.hasPublicInTo(activity) && v.hasPublicInCC(activity)
}

// hasPublicAddressing checks if the activity has any public addressing
func (v *AddressingValidator) hasPublicAddressing(activity *Activity) bool {
	return v.hasPublicInTo(activity) || v.hasPublicInCC(activity)
}

// hasPublicInTo checks if Public address is in To field
func (v *AddressingValidator) hasPublicInTo(activity *Activity) bool {
	publicAddr := "https://www.w3.org/ns/activitystreams#Public"
	for _, addr := range activity.To {
		if addr == publicAddr {
			return true
		}
	}
	return false
}

// hasPublicInCC checks if Public address is in CC field
func (v *AddressingValidator) hasPublicInCC(activity *Activity) bool {
	publicAddr := "https://www.w3.org/ns/activitystreams#Public"
	for _, addr := range activity.CC {
		if addr == publicAddr {
			return true
		}
	}
	return false
}

// hasFollowersCollection checks if followers collection is addressed
func (v *AddressingValidator) hasFollowersCollection(activity *Activity) bool {
	allAddresses := append(append(activity.To, activity.CC...), append(activity.BTo, activity.BCC...)...)

	for _, addr := range allAddresses {
		if strings.Contains(addr, "/followers") {
			return true
		}
	}

	return false
}

// SanitizeForDelivery removes BCC recipients from activity before delivery
func (v *AddressingValidator) SanitizeForDelivery(activity *Activity, excludeBTo bool) *Activity {
	// Create a copy of the activity
	sanitized := *activity

	// Always remove BCC recipients (they should never be visible)
	sanitized.BCC = nil

	// Optionally remove BTo recipients (for recipients who shouldn't see them)
	if excludeBTo {
		sanitized.BTo = nil
	}

	// If the object is a Note, sanitize it too
	if note, ok := sanitized.Object.(*Note); ok {
		sanitizedNote := *note
		sanitizedNote.BCC = nil
		if excludeBTo {
			sanitizedNote.BTo = nil
		}
		sanitized.Object = &sanitizedNote
	}

	return &sanitized
}

// ValidatePrivacyCompliance ensures activity respects privacy settings
func (v *AddressingValidator) ValidatePrivacyCompliance(activity *Activity) error {
	// Validate that direct messages don't have public addressing
	if v.IsDirectMessage(activity) {
		if v.hasPublicAddressing(activity) {
			return fmt.Errorf("direct messages cannot have public addressing")
		}
	}

	// Ensure BCC recipients are properly hidden
	if len(activity.BCC) > 0 {
		// BCC should not appear in any visible fields
		for _, bccRecipient := range activity.BCC {
			if v.isInVisibleFields(activity, bccRecipient) {
				return fmt.Errorf("BCC recipient %s appears in visible addressing fields", bccRecipient)
			}
		}
	}

	return nil
}

// isInVisibleFields checks if a recipient appears in visible addressing fields
func (v *AddressingValidator) isInVisibleFields(activity *Activity, recipient string) bool {
	// Check To field
	for _, to := range activity.To {
		if to == recipient {
			return true
		}
	}

	// Check CC field
	for _, cc := range activity.CC {
		if cc == recipient {
			return true
		}
	}

	// BTo is also visible to the recipient (though hidden from others)
	for _, bto := range activity.BTo {
		if bto == recipient {
			return true
		}
	}

	return false
}

// GetVisibilityLevel determines the privacy level of an activity
func (v *AddressingValidator) GetVisibilityLevel(activity *Activity) string {
	if v.IsPublicMessage(activity) {
		return "public"
	} else if v.IsUnlistedMessage(activity) {
		return "unlisted"
	} else if v.IsPrivateMessage(activity) {
		return "private"
	} else if v.IsDirectMessage(activity) {
		return "direct"
	}
	return "unknown"
}

// IsFollowersOnlyMessage checks if the activity is addressed only to followers
func (v *AddressingValidator) IsFollowersOnlyMessage(activity *Activity) bool {
	// Check if all recipients are followers collections and no public addressing
	if v.hasPublicAddressing(activity) {
		return false
	}

	allRecipients := append(append(append(activity.To, activity.CC...), activity.BTo...), activity.BCC...)

	for _, recipient := range allRecipients {
		if !strings.Contains(recipient, "/followers") {
			return false
		}
	}

	return len(allRecipients) > 0
}

// GetDeliveryRecipientsForPrivacy returns recipients based on privacy level
func (v *AddressingValidator) GetDeliveryRecipientsForPrivacy(activity *Activity, requestingActor string) []string {
	visibility := v.GetVisibilityLevel(activity)

	switch visibility {
	case "public", "unlisted":
		// Public/unlisted: all addressing fields except BCC
		return v.getPublicRecipients(activity)

	case "private":
		// Private: only followers and explicit recipients
		return v.getPrivateRecipients(activity, requestingActor)

	case "direct":
		// Direct: only explicit recipients
		return v.getDirectRecipients(activity, requestingActor)

	default:
		// Unknown: treat as direct message for safety
		return v.getDirectRecipients(activity, requestingActor)
	}
}

// getPublicRecipients returns all public recipients (excluding BCC)
func (v *AddressingValidator) getPublicRecipients(activity *Activity) []string {
	recipients := make([]string, 0)
	recipients = append(recipients, activity.To...)
	recipients = append(recipients, activity.CC...)
	recipients = append(recipients, activity.BTo...)
	return recipients
}

// getPrivateRecipients returns recipients for private/followers-only messages
func (v *AddressingValidator) getPrivateRecipients(activity *Activity, requestingActor string) []string {
	recipients := make([]string, 0)
	recipients = append(recipients, activity.To...)
	recipients = append(recipients, activity.CC...)

	// Include BTo only if requesting actor is a recipient
	if v.isActorInAddressingFields(activity, requestingActor) {
		recipients = append(recipients, activity.BTo...)
	}

	return recipients
}

// getDirectRecipients returns recipients for direct messages
func (v *AddressingValidator) getDirectRecipients(activity *Activity, requestingActor string) []string {
	recipients := make([]string, 0)

	// For direct messages, only include recipients that the requesting actor should see
	if v.isActorInAddressingFields(activity, requestingActor) {
		recipients = append(recipients, activity.To...)
		recipients = append(recipients, activity.CC...)
		recipients = append(recipients, activity.BTo...)
	}

	return recipients
}

// isActorInAddressingFields checks if an actor is mentioned in any addressing field
func (v *AddressingValidator) isActorInAddressingFields(activity *Activity, actorID string) bool {
	allRecipients := append(append(append(activity.To, activity.CC...), activity.BTo...), activity.BCC...)

	for _, recipient := range allRecipients {
		if recipient == actorID {
			return true
		}
	}

	return false
}

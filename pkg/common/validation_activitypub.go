package common // nolint:revive // "common" package name is acceptable for shared utilities

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// ActivityPub validation patterns
var (
	// ActorTypePattern matches valid ActivityPub actor types
	ActorTypePattern = regexp.MustCompile(`^(Person|Service|Group|Organization|Application)$`)

	// ActivityTypePattern matches valid ActivityPub activity types
	ActivityTypePattern = regexp.MustCompile(`^(Create|Update|Delete|Follow|Accept|Reject|Like|Announce|Undo|Block|Move|Add|Remove)$`)

	// ObjectTypePattern matches valid ActivityPub object types
	ObjectTypePattern = regexp.MustCompile(`^(Note|Article|Video|Image|Audio|Document|Page|Event|Place|Profile|Relationship|Tombstone)$`)

	// PublicAddress is the ActivityPub public addressing URI
	PublicAddress = "https://www.w3.org/ns/activitystreams#Public"

	// WebfingerPattern matches acct: URIs for webfinger
	WebfingerPattern = regexp.MustCompile(`^acct:([^@]+)@([^@]+)$`)

	// ActivityPubMimeTypes are accepted Content-Type headers for ActivityPub
	ActivityPubMimeTypes = []string{
		"application/activity+json",
		"application/ld+json",
		"application/json",
	}
)

// ValidateActivityPubActor validates an ActivityPub Actor object structure
func ValidateActivityPubActor(actor map[string]interface{}) error {
	// Validate required @context
	if err := ValidateActivityPubContext(actor); err != nil {
		return err
	}

	// Validate all required fields
	if err := validateActorRequiredFields(actor); err != nil {
		return err
	}

	// Validate optional profile fields
	if err := validateActorOptionalFields(actor); err != nil {
		return err
	}

	// Validate collections and public key
	if err := validateActorCollectionsAndKey(actor); err != nil {
		return err
	}

	return nil
}

// validateActorRequiredFields validates all required fields for ActivityPub actors
func validateActorRequiredFields(actor map[string]interface{}) error {
	// Validate required id field
	id, ok := actor["id"].(string)
	if !ok || id == "" {
		return ValidationError{Field: "id", Message: "is required for ActivityPub actors"}
	}
	if err := ValidateActivityPubURL(id, "id"); err != nil {
		return err
	}

	// Validate required type field
	actorType, ok := actor["type"].(string)
	if !ok || actorType == "" {
		return ValidationError{Field: "type", Message: "is required for ActivityPub actors"}
	}
	if !ActorTypePattern.MatchString(actorType) {
		return ValidationError{Field: "type", Message: "must be one of: Person, Service, Group, Organization, Application"}
	}

	// Validate required preferredUsername
	username, ok := actor["preferredUsername"].(string)
	if !ok || username == "" {
		return ValidationError{Field: "preferredUsername", Message: "is required for ActivityPub actors"}
	}
	if err := ValidateActivityPubUsername(username); err != nil {
		return err
	}

	// Validate required inbox
	inbox, ok := actor["inbox"].(string)
	if !ok || inbox == "" {
		return ValidationError{Field: "inbox", Message: "is required for ActivityPub actors"}
	}
	if err := ValidateActivityPubURL(inbox, "inbox"); err != nil {
		return err
	}

	return nil
}

// validateActorOptionalFields validates optional profile fields for ActivityPub actors
func validateActorOptionalFields(actor map[string]interface{}) error {
	// Validate optional outbox
	if outbox, exists := actor["outbox"].(string); exists && outbox != "" {
		if err := ValidateActivityPubURL(outbox, "outbox"); err != nil {
			return err
		}
	}

	// Validate optional name (display name)
	if name, exists := actor["name"].(string); exists {
		if err := ValidateStringLength("name", name, 0, 255); err != nil {
			return err
		}
	}

	// Validate optional summary (bio)
	if summary, exists := actor["summary"].(string); exists {
		if err := ValidateStringLength("summary", summary, 0, 5000); err != nil {
			return err
		}
	}

	return nil
}

// validateActorCollectionsAndKey validates collections and public key for ActivityPub actors
func validateActorCollectionsAndKey(actor map[string]interface{}) error {
	// Validate optional followers collection
	if followers, exists := actor["followers"].(string); exists && followers != "" {
		if err := ValidateActivityPubURL(followers, "followers"); err != nil {
			return err
		}
	}

	// Validate optional following collection
	if following, exists := actor["following"].(string); exists && following != "" {
		if err := ValidateActivityPubURL(following, "following"); err != nil {
			return err
		}
	}

	// Validate optional publicKey
	if publicKey, exists := actor["publicKey"].(map[string]interface{}); exists {
		if err := ValidateActivityPubPublicKey(publicKey); err != nil {
			return err
		}
	}

	return nil
}

// ValidateActivityPubActivity validates an ActivityPub Activity object structure
func ValidateActivityPubActivity(activity map[string]interface{}) error {
	// Validate required @context
	if err := ValidateActivityPubContext(activity); err != nil {
		return err
	}

	// Validate required id field
	id, ok := activity["id"].(string)
	if !ok || id == "" {
		return ValidationError{Field: "id", Message: "is required for ActivityPub activities"}
	}

	if err := ValidateActivityPubURL(id, "id"); err != nil {
		return err
	}

	// Validate required type field
	activityType, ok := activity["type"].(string)
	if !ok || activityType == "" {
		return ValidationError{Field: "type", Message: "is required for ActivityPub activities"}
	}

	if !ActivityTypePattern.MatchString(activityType) {
		return ValidationError{Field: "type", Message: "must be a valid ActivityPub activity type"}
	}

	// Validate required actor field
	actor, ok := activity["actor"].(string)
	if !ok || actor == "" {
		return ValidationError{Field: "actor", Message: "is required for ActivityPub activities"}
	}

	if err := ValidateActivityPubURL(actor, "actor"); err != nil {
		return err
	}

	// Validate addressing fields
	if to, exists := activity["to"]; exists {
		if err := ValidateActivityPubAddressing(to, "to"); err != nil {
			return err
		}
	}

	if cc, exists := activity["cc"]; exists {
		if err := ValidateActivityPubAddressing(cc, "cc"); err != nil {
			return err
		}
	}

	if bto, exists := activity["bto"]; exists {
		if err := ValidateActivityPubAddressing(bto, "bto"); err != nil {
			return err
		}
	}

	if bcc, exists := activity["bcc"]; exists {
		if err := ValidateActivityPubAddressing(bcc, "bcc"); err != nil {
			return err
		}
	}

	// Validate object field for activities that require it
	if requiresObject(activityType) {
		if _, exists := activity["object"]; !exists {
			return ValidationError{Field: "object", Message: "is required for this activity type"}
		}
	}

	// Validate published timestamp if present
	if published, exists := activity["published"].(string); exists {
		if err := ValidateActivityPubTimestamp(published, "published"); err != nil {
			return err
		}
	}

	return nil
}

// ValidateActivityPubNote validates an ActivityPub Note object structure
func ValidateActivityPubNote(note map[string]interface{}) error {
	// Validate required @context
	if err := ValidateActivityPubContext(note); err != nil {
		return err
	}

	// Validate required note fields
	if err := validateNoteRequiredFields(note); err != nil {
		return err
	}

	// Validate optional note fields
	if err := validateNoteOptionalFields(note); err != nil {
		return err
	}

	// Validate addressing and metadata
	if err := validateNoteAddressingAndMetadata(note); err != nil {
		return err
	}

	// Validate attachments and tags
	if err := validateNoteAttachmentsAndTags(note); err != nil {
		return err
	}

	return nil
}

// validateNoteRequiredFields validates all required fields for ActivityPub notes
func validateNoteRequiredFields(note map[string]interface{}) error {
	// Validate required type field
	noteType, ok := note["type"].(string)
	if !ok || noteType != "Note" {
		return ValidationError{Field: "type", Message: "must be 'Note' for ActivityPub notes"}
	}

	// Validate required content field
	content, ok := note["content"].(string)
	if !ok || strings.TrimSpace(content) == "" {
		return ValidationError{Field: "content", Message: "is required for ActivityPub notes"}
	}
	if err := ValidateStringLength("content", content, 1, 100000); err != nil {
		return err
	}

	// Validate required attributedTo field
	attributedTo, ok := note["attributedTo"].(string)
	if !ok || attributedTo == "" {
		return ValidationError{Field: "attributedTo", Message: "is required for ActivityPub notes"}
	}
	if err := ValidateActivityPubURL(attributedTo, "attributedTo"); err != nil {
		return err
	}

	return nil
}

// validateNoteOptionalFields validates optional fields for ActivityPub notes
func validateNoteOptionalFields(note map[string]interface{}) error {
	// Validate optional id field
	if id, exists := note["id"].(string); exists && id != "" {
		if err := ValidateActivityPubURL(id, "id"); err != nil {
			return err
		}
	}

	// Validate optional inReplyTo field
	if inReplyTo, exists := note["inReplyTo"].(string); exists && inReplyTo != "" {
		if err := ValidateActivityPubURL(inReplyTo, "inReplyTo"); err != nil {
			return err
		}
	}

	// Validate optional summary (content warning)
	if summary, exists := note["summary"].(string); exists {
		if err := ValidateStringLength("summary", summary, 0, 500); err != nil {
			return err
		}
	}

	return nil
}

// validateNoteAddressingAndMetadata validates addressing and timestamp for ActivityPub notes
func validateNoteAddressingAndMetadata(note map[string]interface{}) error {
	// Validate addressing
	if to, exists := note["to"]; exists {
		if err := ValidateActivityPubAddressing(to, "to"); err != nil {
			return err
		}
	}

	if cc, exists := note["cc"]; exists {
		if err := ValidateActivityPubAddressing(cc, "cc"); err != nil {
			return err
		}
	}

	// Validate published timestamp if present
	if published, exists := note["published"].(string); exists {
		if err := ValidateActivityPubTimestamp(published, "published"); err != nil {
			return err
		}
	}

	return nil
}

// validateNoteAttachmentsAndTags validates attachments and tags for ActivityPub notes
func validateNoteAttachmentsAndTags(note map[string]interface{}) error {
	// Validate attachment array if present
	if attachment, exists := note["attachment"]; exists {
		if err := ValidateActivityPubAttachments(attachment, "attachment"); err != nil {
			return err
		}
	}

	// Validate tag array if present
	if tag, exists := note["tag"]; exists {
		if err := ValidateActivityPubTags(tag, "tag"); err != nil {
			return err
		}
	}

	return nil
}

// ValidateActivityPubURL validates URLs for ActivityPub compliance
func ValidateActivityPubURL(urlStr, fieldName string) error {
	if urlStr == "" {
		return ValidationError{Field: fieldName, Message: "cannot be empty"}
	}

	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return ValidationError{Field: fieldName, Message: "must be a valid URL"}
	}

	// Must use HTTPS or HTTP
	if parsedURL.Scheme != SchemeHTTPS && parsedURL.Scheme != SchemeHTTP {
		return ValidationError{Field: fieldName, Message: "must use HTTP or HTTPS scheme"}
	}

	// Must have a host
	if parsedURL.Host == "" {
		return ValidationError{Field: fieldName, Message: "must have a valid host"}
	}

	// URL must not be too long
	if len(urlStr) > 2000 {
		return ValidationError{Field: fieldName, Message: "cannot be longer than 2000 characters"}
	}

	return nil
}

// ValidateActivityPubUsername validates usernames for ActivityPub compliance
func ValidateActivityPubUsername(username string) error {
	if username == "" {
		return ValidationError{Field: "preferredUsername", Message: "cannot be empty"}
	}

	if len(username) > 30 {
		return ValidationError{Field: "preferredUsername", Message: "cannot be longer than 30 characters"}
	}

	// ActivityPub usernames should be alphanumeric with limited special chars
	usernamePattern := regexp.MustCompile(`^[a-zA-Z0-9_\-\.]+$`)
	if !usernamePattern.MatchString(username) {
		return ValidationError{Field: "preferredUsername", Message: "can only contain letters, numbers, underscores, hyphens, and dots"}
	}

	// Cannot start or end with special characters
	if strings.HasPrefix(username, ".") || strings.HasSuffix(username, ".") ||
		strings.HasPrefix(username, "-") || strings.HasSuffix(username, "-") ||
		strings.HasPrefix(username, "_") || strings.HasSuffix(username, "_") {
		return ValidationError{Field: "preferredUsername", Message: "cannot start or end with special characters"}
	}

	return nil
}

// ValidateActivityPubContext validates the @context field
func ValidateActivityPubContext(obj map[string]interface{}) error {
	context, exists := obj["@context"]
	if !exists {
		return ValidationError{Field: "@context", Message: "is required for ActivityPub objects"}
	}

	// @context can be a string or array
	switch ctx := context.(type) {
	case string:
		if ctx != "https://www.w3.org/ns/activitystreams" {
			return ValidationError{Field: "@context", Message: "must include ActivityStreams context"}
		}
	case []interface{}:
		hasActivityStreams := false
		for _, c := range ctx {
			if str, ok := c.(string); ok && str == "https://www.w3.org/ns/activitystreams" {
				hasActivityStreams = true
				break
			}
		}
		if !hasActivityStreams {
			return ValidationError{Field: "@context", Message: "must include ActivityStreams context"}
		}
	default:
		return ValidationError{Field: "@context", Message: "must be a string or array"}
	}

	return nil
}

// ValidateActivityPubAddressing validates addressing fields (to, cc, bto, bcc)
func ValidateActivityPubAddressing(addressing interface{}, fieldName string) error {
	switch addr := addressing.(type) {
	case string:
		// Single address
		if addr == PublicAddress {
			return nil // Public addressing is always valid
		}
		return ValidateActivityPubURL(addr, fieldName)

	case []interface{}:
		// Array of addresses
		for i, a := range addr {
			if addrStr, ok := a.(string); ok {
				if addrStr == PublicAddress {
					continue // Public addressing is always valid
				}
				if err := ValidateActivityPubURL(addrStr, fmt.Sprintf("%s[%d]", fieldName, i)); err != nil {
					return err
				}
			} else {
				return ValidationError{Field: fmt.Sprintf("%s[%d]", fieldName, i), Message: "must be a string URL"}
			}
		}
		return nil

	default:
		return ValidationError{Field: fieldName, Message: "must be a string or array of strings"}
	}
}

// ValidateActivityPubTimestamp validates timestamp formats used in ActivityPub
func ValidateActivityPubTimestamp(timestamp, fieldName string) error {
	if timestamp == "" {
		return nil // Empty timestamps are allowed
	}

	// Try parsing as RFC3339 (ISO 8601)
	_, err := time.Parse(time.RFC3339, timestamp)
	if err != nil {
		return ValidationError{Field: fieldName, Message: "must be a valid RFC3339 timestamp"}
	}

	return nil
}

// ValidateActivityPubPublicKey validates public key objects
func ValidateActivityPubPublicKey(publicKey map[string]interface{}) error {
	// Validate required id
	id, ok := publicKey["id"].(string)
	if !ok || id == "" {
		return ValidationError{Field: "publicKey.id", Message: "is required"}
	}

	if err := ValidateActivityPubURL(id, "publicKey.id"); err != nil {
		return err
	}

	// Validate required owner
	owner, ok := publicKey["owner"].(string)
	if !ok || owner == "" {
		return ValidationError{Field: "publicKey.owner", Message: "is required"}
	}

	if err := ValidateActivityPubURL(owner, "publicKey.owner"); err != nil {
		return err
	}

	// Validate required publicKeyPem
	publicKeyPem, ok := publicKey["publicKeyPem"].(string)
	if !ok || publicKeyPem == "" {
		return ValidationError{Field: "publicKey.publicKeyPem", Message: "is required"}
	}

	// Basic validation that it looks like a PEM key
	if !strings.Contains(publicKeyPem, "BEGIN PUBLIC KEY") || !strings.Contains(publicKeyPem, "END PUBLIC KEY") {
		return ValidationError{Field: "publicKey.publicKeyPem", Message: "must be a valid PEM-encoded public key"}
	}

	return nil
}

// ValidateActivityPubAttachments validates attachment arrays
func ValidateActivityPubAttachments(attachments interface{}, fieldName string) error {
	attachArray, ok := attachments.([]interface{})
	if !ok {
		return ValidationError{Field: fieldName, Message: "must be an array"}
	}

	if len(attachArray) > 10 {
		return ValidationError{Field: fieldName, Message: "cannot have more than 10 attachments"}
	}

	for i, attachment := range attachArray {
		attachObj, ok := attachment.(map[string]interface{})
		if !ok {
			return ValidationError{Field: fmt.Sprintf("%s[%d]", fieldName, i), Message: "must be an object"}
		}

		// Validate attachment type
		attachType, ok := attachObj["type"].(string)
		if !ok || attachType == "" {
			return ValidationError{Field: fmt.Sprintf("%s[%d].type", fieldName, i), Message: "is required"}
		}

		// Validate attachment URL
		attachURL, ok := attachObj["url"].(string)
		if !ok || attachURL == "" {
			return ValidationError{Field: fmt.Sprintf("%s[%d].url", fieldName, i), Message: "is required"}
		}

		if err := ValidateActivityPubURL(attachURL, fmt.Sprintf("%s[%d].url", fieldName, i)); err != nil {
			return err
		}
	}

	return nil
}

// ValidateActivityPubTags validates tag arrays
func ValidateActivityPubTags(tags interface{}, fieldName string) error {
	tagArray, ok := tags.([]interface{})
	if !ok {
		return ValidationError{Field: fieldName, Message: "must be an array"}
	}

	if len(tagArray) > 50 {
		return ValidationError{Field: fieldName, Message: "cannot have more than 50 tags"}
	}

	for i, tag := range tagArray {
		tagObj, ok := tag.(map[string]interface{})
		if !ok {
			return ValidationError{Field: fmt.Sprintf("%s[%d]", fieldName, i), Message: "must be an object"}
		}

		// Validate tag type
		tagType, ok := tagObj["type"].(string)
		if !ok {
			return ValidationError{Field: fmt.Sprintf("%s[%d].type", fieldName, i), Message: "is required"}
		}

		// Validate based on tag type
		switch tagType {
		case "Mention":
			if href, exists := tagObj["href"].(string); exists {
				if err := ValidateActivityPubURL(href, fmt.Sprintf("%s[%d].href", fieldName, i)); err != nil {
					return err
				}
			}
		case "Hashtag":
			if href, exists := tagObj["href"].(string); exists {
				if err := ValidateActivityPubURL(href, fmt.Sprintf("%s[%d].href", fieldName, i)); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

// ValidateWebfingerResource validates WebFinger resource identifiers
func ValidateWebfingerResource(resource string) error {
	if resource == "" {
		return ValidationError{Field: "resource", Message: "cannot be empty"}
	}

	// Check if it's an acct: URI
	if strings.HasPrefix(resource, "acct:") {
		matches := WebfingerPattern.FindStringSubmatch(resource)
		if len(matches) != 3 {
			return ValidationError{Field: "resource", Message: "invalid acct: format (expected acct:user@domain)"}
		}

		username := matches[1]
		domain := matches[2]

		if err := ValidateActivityPubUsername(username); err != nil {
			return ValidationError{Field: "resource", Message: fmt.Sprintf("invalid username: %v", err)}
		}

		if err := ValidateDomainName(domain); err != nil {
			return ValidationError{Field: "resource", Message: fmt.Sprintf("invalid domain: %v", err)}
		}

		return nil
	}

	// Otherwise it should be a URL
	return ValidateActivityPubURL(resource, "resource")
}

// ValidateDomainName validates domain names for ActivityPub
func ValidateDomainName(domain string) error {
	if domain == "" {
		return ValidationError{Field: "domain", Message: "cannot be empty"}
	}

	if len(domain) > 253 {
		return ValidationError{Field: "domain", Message: "cannot be longer than 253 characters"}
	}

	// Basic domain validation
	domainPattern := regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])?)*$`)
	if !domainPattern.MatchString(domain) {
		return ValidationError{Field: "domain", Message: "invalid domain format"}
	}

	return nil
}

// ValidateActivityPubSignature validates HTTP signature headers for ActivityPub
func ValidateActivityPubSignature(signature string) error {
	if signature == "" {
		return ValidationError{Field: "signature", Message: "cannot be empty"}
	}

	// Parse signature parameters
	params := make(map[string]string)
	parts := strings.Split(signature, ",")

	for _, part := range parts {
		part = strings.TrimSpace(part)
		keyValue := strings.SplitN(part, "=", 2)
		if len(keyValue) != 2 {
			continue
		}

		key := strings.TrimSpace(keyValue[0])
		value := strings.Trim(strings.TrimSpace(keyValue[1]), `"`)
		params[key] = value
	}

	// Validate required signature parameters
	// Note: algorithm and headers are optional - defaults will be set by the parser
	requiredParams := []string{"keyId", "signature"}
	for _, param := range requiredParams {
		if _, exists := params[param]; !exists {
			return ValidationError{Field: "signature", Message: fmt.Sprintf("missing required parameter: %s", param)}
		}
	}

	// Validate keyId is a URL
	if err := ValidateActivityPubURL(params["keyId"], "signature.keyId"); err != nil {
		return ValidationError{Field: "signature.keyId", Message: "must be a valid URL"}
	}

	return nil
}

// ValidateActivityPubContentType validates Content-Type headers for ActivityPub
func ValidateActivityPubContentType(contentType string) error {
	if contentType == "" {
		return ValidationError{Field: "content-type", Message: "is required for ActivityPub requests"}
	}

	// Normalize content type (remove charset, etc.)
	normalizedType := strings.Split(contentType, ";")[0]
	normalizedType = strings.TrimSpace(strings.ToLower(normalizedType))

	for _, validType := range ActivityPubMimeTypes {
		if normalizedType == validType {
			return nil
		}
	}

	return ValidationError{Field: "content-type", Message: fmt.Sprintf("must be one of: %s", strings.Join(ActivityPubMimeTypes, ", "))}
}

// ValidateActivityPubJSON validates that a string contains valid ActivityPub JSON
func ValidateActivityPubJSON(jsonStr string, fieldName string) error {
	if jsonStr == "" {
		return ValidationError{Field: fieldName, Message: "cannot be empty"}
	}

	var obj map[string]interface{}
	if err := json.Unmarshal([]byte(jsonStr), &obj); err != nil {
		return ValidationError{Field: fieldName, Message: "must be valid JSON"}
	}

	// Must have @context
	if err := ValidateActivityPubContext(obj); err != nil {
		return err
	}

	// Must have type
	if _, exists := obj["type"]; !exists {
		return ValidationError{Field: "type", Message: "is required in ActivityPub objects"}
	}

	return nil
}

// Helper function to determine if an activity type requires an object
func requiresObject(activityType string) bool {
	objectRequiredTypes := map[string]bool{
		"Create":   true,
		"Update":   true,
		"Delete":   true,
		"Like":     true,
		"Announce": true,
		"Undo":     true,
		"Block":    true,
		"Add":      true,
		"Remove":   true,
	}

	return objectRequiredTypes[activityType]
}

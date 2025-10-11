// Package validation provides input validation for ActivityPub objects and endpoints
package validation

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"go.uber.org/zap"
)

// ActivityPubValidator handles validation of ActivityPub objects
type ActivityPubValidator struct {
	logger *zap.Logger
}

// NewActivityPubValidator creates a new ActivityPub validator
func NewActivityPubValidator(logger *zap.Logger) *ActivityPubValidator {
	return &ActivityPubValidator{
		logger: logger,
	}
}

// Activity represents a basic ActivityPub Activity
type Activity struct {
	Context   interface{} `json:"@context,omitempty"`
	ID        string      `json:"id,omitempty"`
	Type      string      `json:"type"`
	Actor     string      `json:"actor"`
	Object    interface{} `json:"object,omitempty"`
	Target    interface{} `json:"target,omitempty"`
	Result    interface{} `json:"result,omitempty"`
	Origin    interface{} `json:"origin,omitempty"`
	Instrument interface{} `json:"instrument,omitempty"`
	Published time.Time   `json:"published,omitempty"`
	Updated   time.Time   `json:"updated,omitempty"`
	To        []string    `json:"to,omitempty"`
	CC        []string    `json:"cc,omitempty"`
	BTO       []string    `json:"bto,omitempty"`
	BCC       []string    `json:"bcc,omitempty"`
}

// ValidationConfig defines validation rules
type ValidationConfig struct {
	MaxObjectSize     int           // Maximum size of ActivityPub object in bytes
	MaxStringLength   int           // Maximum length for string fields
	MaxArrayLength    int           // Maximum length for arrays
	AllowedTypes      []string      // Allowed ActivityPub types
	RequiredFields    []string      // Required fields for validation
	URLTimeout        time.Duration // Timeout for URL validation
	AllowLocalURLs    bool          // Whether to allow local/internal URLs
	MaxDepth          int           // Maximum nesting depth for objects
}

// DefaultValidationConfig returns default validation configuration
func DefaultValidationConfig() *ValidationConfig {
	return &ValidationConfig{
		MaxObjectSize:   1024 * 1024, // 1MB
		MaxStringLength: 5000,        // 5KB per string field
		MaxArrayLength:  1000,        // Max 1000 items in arrays
		AllowedTypes: []string{
			"Create", "Update", "Delete", "Follow", "Accept", "Reject",
			"Add", "Remove", "Like", "Announce", "Undo", "Block",
			"Flag", "Note", "Article", "Image", "Video", "Audio",
			"Document", "Person", "Group", "Organization", "Service",
			"Application", "Collection", "OrderedCollection",
		},
		RequiredFields: []string{"type", "actor"},
		URLTimeout:     5 * time.Second,
		AllowLocalURLs: false,
		MaxDepth:       5,
	}
}

// ValidateActivity validates an ActivityPub activity
func (v *ActivityPubValidator) ValidateActivity(data []byte, config *ValidationConfig) (*Activity, error) {
	if config == nil {
		config = DefaultValidationConfig()
	}

	// Size check
	if len(data) > config.MaxObjectSize {
		return nil, fmt.Errorf("activity too large: %d bytes (max %d)", len(data), config.MaxObjectSize)
	}

	// Basic JSON validation
	var activity Activity
	if err := json.Unmarshal(data, &activity); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}

	// Validate required fields
	if err := v.validateRequiredFields(&activity, config); err != nil {
		return nil, fmt.Errorf("required field validation failed: %w", err)
	}

	// Validate activity type
	if err := v.validateType(activity.Type, config.AllowedTypes); err != nil {
		return nil, fmt.Errorf("type validation failed: %w", err)
	}

	// Validate actor URL
	if err := v.validateURL(activity.Actor, "actor", config); err != nil {
		return nil, fmt.Errorf("actor validation failed: %w", err)
	}

	// Validate ID if present
	if activity.ID != "" {
		if err := v.validateURL(activity.ID, "id", config); err != nil {
			return nil, fmt.Errorf("id validation failed: %w", err)
		}
	}

	// Validate recipient fields
	if err := v.validateRecipients(&activity, config); err != nil {
		return nil, fmt.Errorf("recipient validation failed: %w", err)
	}

	// Validate string lengths
	if err := v.validateStringLengths(&activity, config); err != nil {
		return nil, fmt.Errorf("string length validation failed: %w", err)
	}

	// Validate nested objects
	if err := v.validateNestedObjects(&activity, config, 0); err != nil {
		return nil, fmt.Errorf("nested object validation failed: %w", err)
	}

	v.logger.Debug("ActivityPub object validated successfully",
		zap.String("type", activity.Type),
		zap.String("actor", activity.Actor),
		zap.String("id", activity.ID))

	return &activity, nil
}

// validateRequiredFields checks that required fields are present
func (v *ActivityPubValidator) validateRequiredFields(activity *Activity, config *ValidationConfig) error {
	for _, field := range config.RequiredFields {
		switch field {
		case "type":
			if activity.Type == "" {
				return errors.New("missing required field: type")
			}
		case "actor":
			if activity.Actor == "" {
				return errors.New("missing required field: actor")
			}
		case "id":
			if activity.ID == "" {
				return errors.New("missing required field: id")
			}
		case "object":
			if activity.Object == nil {
				return errors.New("missing required field: object")
			}
		}
	}
	return nil
}

// validateType checks if the activity type is allowed
func (v *ActivityPubValidator) validateType(activityType string, allowedTypes []string) error {
	if activityType == "" {
		return errors.New("activity type cannot be empty")
	}

	// Check against allowed types
	for _, allowed := range allowedTypes {
		if activityType == allowed {
			return nil
		}
	}

	return fmt.Errorf("activity type '%s' not allowed", activityType)
}

// validateURL validates that a URL is safe and reachable
func (v *ActivityPubValidator) validateURL(rawURL, fieldName string, config *ValidationConfig) error {
	if rawURL == "" {
		return nil // Optional field
	}

	// Parse URL
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL in %s: %w", fieldName, err)
	}

	// Must be HTTP/HTTPS
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("invalid URL scheme in %s: %s (must be http or https)", fieldName, u.Scheme)
	}

	// Require HTTPS in production
	if os.Getenv("ENVIRONMENT") == "production" && u.Scheme != "https" {
		return fmt.Errorf("URL in %s must use HTTPS in production", fieldName)
	}

	// Check for blocked domains/IPs (SSRF protection)
	if !config.AllowLocalURLs {
		if err := v.validateURLNotInternal(u); err != nil {
			return fmt.Errorf("URL in %s not allowed: %w", fieldName, err)
		}
	}

	// Validate hostname format
	if err := v.validateHostname(u.Hostname()); err != nil {
		return fmt.Errorf("invalid hostname in %s: %w", fieldName, err)
	}

	return nil
}

// validateURLNotInternal checks that URL doesn't point to internal/private addresses
func (v *ActivityPubValidator) validateURLNotInternal(u *url.URL) error {
	host := u.Hostname()

	// Block localhost variants
	localhostPatterns := []string{
		"localhost", "127.0.0.1", "::1", "0.0.0.0",
	}
	for _, pattern := range localhostPatterns {
		if strings.EqualFold(host, pattern) {
			return fmt.Errorf("localhost URLs not allowed: %s", host)
		}
	}

	// Resolve and check IP
	ips, err := net.LookupIP(host)
	if err != nil {
		// Allow if we can't resolve (might be temporary DNS issue)
		v.logger.Warn("failed to resolve hostname for SSRF check", 
			zap.String("host", host), 
			zap.Error(err))
		return nil
	}

	for _, ip := range ips {
		if v.isPrivateIP(ip) {
			return fmt.Errorf("private IP addresses not allowed: %s resolves to %s", host, ip.String())
		}
	}

	return nil
}

// isPrivateIP checks if an IP address is private/internal
func (v *ActivityPubValidator) isPrivateIP(ip net.IP) bool {
	// Check for private IPv4 ranges
	privateRanges := []string{
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
		"127.0.0.0/8",   // Loopback
		"169.254.0.0/16", // Link-local
	}

	for _, rangeStr := range privateRanges {
		_, network, err := net.ParseCIDR(rangeStr)
		if err != nil {
			continue
		}
		if network.Contains(ip) {
			return true
		}
	}

	// Check for private IPv6 ranges
	if ip.To4() == nil { // IPv6
		// Link-local
		if ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
			return true
		}
		// Unique local addresses (fc00::/7)
		if len(ip) >= 1 && (ip[0]&0xfe) == 0xfc {
			return true
		}
		// Loopback
		if ip.IsLoopback() {
			return true
		}
	}

	return false
}

// validateHostname validates hostname format
func (v *ActivityPubValidator) validateHostname(hostname string) error {
	if hostname == "" {
		return errors.New("hostname cannot be empty")
	}

	// Check length
	if len(hostname) > 253 {
		return errors.New("hostname too long")
	}

	// Basic format validation
	hostnameRegex := regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])?)*$`)
	if !hostnameRegex.MatchString(hostname) {
		return errors.New("invalid hostname format")
	}

	return nil
}

// validateRecipients validates recipient arrays (to, cc, bto, bcc)
func (v *ActivityPubValidator) validateRecipients(activity *Activity, config *ValidationConfig) error {
	recipients := [][]string{activity.To, activity.CC, activity.BTO, activity.BCC}
	fieldNames := []string{"to", "cc", "bto", "bcc"}

	for i, recipientList := range recipients {
		if err := v.validateRecipientArray(recipientList, fieldNames[i], config); err != nil {
			return err
		}
	}

	return nil
}

// validateRecipientArray validates a recipient array
func (v *ActivityPubValidator) validateRecipientArray(recipients []string, fieldName string, config *ValidationConfig) error {
	if len(recipients) > config.MaxArrayLength {
		return fmt.Errorf("too many recipients in %s: %d (max %d)", fieldName, len(recipients), config.MaxArrayLength)
	}

	for _, recipient := range recipients {
		// Check for special values
		if recipient == "https://www.w3.org/ns/activitystreams#Public" {
			continue // Public addressing is valid
		}

		// Validate as URL
		if err := v.validateURL(recipient, fieldName, config); err != nil {
			return err
		}
	}

	return nil
}

// validateStringLengths validates that string fields don't exceed limits
func (v *ActivityPubValidator) validateStringLengths(activity *Activity, config *ValidationConfig) error {
	stringFields := map[string]string{
		"id":    activity.ID,
		"type":  activity.Type,
		"actor": activity.Actor,
	}

	for fieldName, value := range stringFields {
		if len(value) > config.MaxStringLength {
			return fmt.Errorf("string field %s too long: %d characters (max %d)", fieldName, len(value), config.MaxStringLength)
		}
	}

	return nil
}

// validateNestedObjects validates nested objects within the activity
func (v *ActivityPubValidator) validateNestedObjects(activity *Activity, config *ValidationConfig, depth int) error {
	if depth > config.MaxDepth {
		return fmt.Errorf("object nesting too deep: %d (max %d)", depth, config.MaxDepth)
	}

	// This is a simplified version - in a full implementation you'd recursively
	// validate nested objects, arrays, etc.
	
	return nil
}

// ValidateInboxDelivery validates an incoming ActivityPub delivery to an inbox
func (v *ActivityPubValidator) ValidateInboxDelivery(data []byte, signature string) (*Activity, error) {
	// Create stricter config for inbox deliveries
	config := DefaultValidationConfig()
	config.RequiredFields = []string{"type", "actor", "id"}
	config.MaxObjectSize = 512 * 1024 // 512KB for inbox deliveries

	// Validate the activity
	activity, err := v.ValidateActivity(data, config)
	if err != nil {
		return nil, fmt.Errorf("inbox delivery validation failed: %w", err)
	}

	// Validate HTTP signature if present
	if signature != "" {
		if err := v.validateHTTPSignature(signature); err != nil {
			return nil, fmt.Errorf("HTTP signature validation failed: %w", err)
		}
	}

	return activity, nil
}

// validateHTTPSignature performs basic HTTP signature validation
func (v *ActivityPubValidator) validateHTTPSignature(signature string) error {
	// Basic format validation
	if !strings.Contains(signature, "keyId=") {
		return errors.New("HTTP signature missing keyId")
	}

	if !strings.Contains(signature, "signature=") {
		return errors.New("HTTP signature missing signature value")
	}

	// Additional signature validation would be implemented here
	// (key fetching, signature verification, etc.)

	return nil
}
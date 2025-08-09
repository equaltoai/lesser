package activitypub

import (
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strings"
)

var (
	// Username: 1-30 chars, alphanumeric + underscore + hyphen, no double underscore
	usernameRegex = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9_\-]{0,28}[a-zA-Z0-9])?$`)

	// Domain: valid hostname
	domainRegex = regexp.MustCompile(`^([a-zA-Z0-9]([a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])?\.)+[a-zA-Z]{2,}$`)

	// Webfinger format
	webfingerRegex = regexp.MustCompile(`^acct:([^@]+)@([^@]+)$`)
)

// ValidateUsername validates an ActivityPub username format
func ValidateUsername(username string) error {
	if username == "" {
		return fmt.Errorf("username cannot be empty")
	}

	if len(username) > 30 {
		return fmt.Errorf("username too long (max 30 characters)")
	}

	if !usernameRegex.MatchString(username) {
		return fmt.Errorf("username can only contain letters, numbers, underscores, and hyphens")
	}

	// Check for reserved usernames
	reserved := []string{"admin", "root", "system", "api", "well-known"}
	lowerUsername := strings.ToLower(username)
	for _, r := range reserved {
		if lowerUsername == r {
			return fmt.Errorf("username '%s' is reserved", username)
		}
	}

	return nil
}

// ValidateDomain validates a domain name format
func ValidateDomain(domain string) error {
	if domain == "" {
		return fmt.Errorf("domain cannot be empty")
	}

	// Check IP addresses are not used as domains
	if net.ParseIP(domain) != nil {
		return fmt.Errorf("IP addresses cannot be used as domains")
	}

	if !domainRegex.MatchString(domain) {
		return fmt.Errorf("invalid domain format")
	}

	// Additional checks
	if strings.Contains(domain, "..") {
		return fmt.Errorf("invalid domain: consecutive dots")
	}

	return nil
}

// ValidateActorID validates an ActivityPub actor ID URL
func ValidateActorID(actorID string) error {
	u, err := url.Parse(actorID)
	if err != nil {
		return fmt.Errorf("invalid actor ID URL: %w", err)
	}

	// Must be HTTPS in production
	if u.Scheme != "https" && u.Scheme != "http" {
		return fmt.Errorf("actor ID must use HTTP(S)")
	}

	// Validate domain part
	if err := ValidateDomain(u.Hostname()); err != nil {
		return fmt.Errorf("invalid domain in actor ID: %w", err)
	}

	// Path must not be empty
	if u.Path == "" || u.Path == "/" {
		return fmt.Errorf("actor ID must have a path")
	}

	return nil
}

// ValidateWebfinger validates a WebFinger resource identifier
func ValidateWebfinger(resource string) error {
	matches := webfingerRegex.FindStringSubmatch(resource)
	if len(matches) != 3 {
		return fmt.Errorf("invalid webfinger format (expected acct:user@domain)")
	}

	username := matches[1]
	domain := matches[2]

	if err := ValidateUsername(username); err != nil {
		return fmt.Errorf("invalid username in webfinger: %w", err)
	}

	if err := ValidateDomain(domain); err != nil {
		return fmt.Errorf("invalid domain in webfinger: %w", err)
	}

	return nil
}

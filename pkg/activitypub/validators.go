package activitypub

import (
	"fmt"
	"github.com/equaltoai/lesser/pkg/common"
	"net"
	"net/url"
	"regexp"
	"strings"
)

const (
	// HTTPScheme represents the HTTP URL scheme
	HTTPScheme = "http"
	// HTTPSScheme represents the HTTPS URL scheme
	HTTPSScheme = "https"
)

var (
	// Domain: valid hostname
	domainRegex = regexp.MustCompile(`^([a-zA-Z0-9]([a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])?\.)+[a-zA-Z]{2,}$`)

	// Webfinger format
	webfingerRegex = regexp.MustCompile(`^acct:([^@]+)@([^@]+)$`)
)

// ValidateUsername validates an ActivityPub username format
func ValidateUsername(username string) error {
	// Use the common validation function which includes all the same checks
	if err := common.ValidateUsername(username); err != nil {
		return err
	}

	// Additional ActivityPub-specific validation can be added here if needed
	return nil
}

// ValidateDomain validates a domain name format
func ValidateDomain(domain string) error {
	if err := common.ValidateRequiredParam("domain", domain); err != nil {
		return ErrEmptyDomain
	}

	// Check IP addresses are not used as domains
	if net.ParseIP(domain) != nil {
		return ErrIPAddressAsDomain
	}

	if !domainRegex.MatchString(domain) {
		return ErrInvalidDomainFormat
	}

	// Additional checks
	if strings.Contains(domain, "..") {
		return ErrConsecutiveDots
	}

	return nil
}

// ValidateActorID validates an ActivityPub actor ID URL
func ValidateActorID(actorID string) error {
	u, err := url.Parse(actorID)
	if err != nil {
		return fmt.Errorf("%w: %s", ErrInvalidActorIDURL, err.Error())
	}

	// Must be HTTPS in production
	if u.Scheme != HTTPSScheme && u.Scheme != HTTPScheme {
		return ErrActorIDScheme
	}

	// Validate domain part
	if err := ValidateDomain(u.Hostname()); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidDomainInActorID, err)
	}

	// Path must not be empty
	if u.Path == "" || u.Path == "/" {
		return ErrActorIDMissingPath
	}

	return nil
}

// ValidateWebfinger validates a WebFinger resource identifier
func ValidateWebfinger(resource string) error {
	matches := webfingerRegex.FindStringSubmatch(resource)
	if len(matches) != 3 {
		return ErrInvalidWebfingerFormat
	}

	username := matches[1]
	domain := matches[2]

	if err := ValidateUsername(username); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidUsernameInWebfinger, err)
	}

	if err := ValidateDomain(domain); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidDomainInWebfinger, err)
	}

	return nil
}

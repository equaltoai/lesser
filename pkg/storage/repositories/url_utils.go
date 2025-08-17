package repositories

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/equaltoai/lesser/pkg/common"
	"go.uber.org/zap"
)

// URLExtractionResult represents the result of URL extraction and validation
type URLExtractionResult struct {
	OriginalURL    string            `json:"original_url"`
	NormalizedURL  string            `json:"normalized_url"`
	Domain         string            `json:"domain"`
	Subdomain      string            `json:"subdomain,omitempty"`
	Path           string            `json:"path,omitempty"`
	ProfileType    string            `json:"profile_type,omitempty"` // twitter, mastodon, github, etc.
	Username       string            `json:"username,omitempty"`     // extracted username from URL
	IsValid        bool              `json:"is_valid"`
	IsSecure       bool              `json:"is_secure"` // https
	IsSocial       bool              `json:"is_social"` // known social media platform
	IsShortened    bool              `json:"is_shortened"`
	ValidationTags []string          `json:"validation_tags,omitempty"`
	Metadata       map[string]string `json:"metadata,omitempty"`
}

// URLValidator provides enhanced URL extraction and validation
type URLValidator struct {
	logger *zap.Logger
}

// NewURLValidator creates a new URL validator
func NewURLValidator(logger *zap.Logger) *URLValidator {
	return &URLValidator{
		logger: logger,
	}
}

// Social media platform patterns
var socialPlatforms = map[string]*regexp.Regexp{
	"twitter":   regexp.MustCompile(`^https?://(?:www\.)?(twitter\.com|x\.com)/([a-zA-Z0-9_]{1,15})/?$`),
	"mastodon":  regexp.MustCompile(`^https?://([^/]+)/@([a-zA-Z0-9_.-]+)/?$`),
	"github":    regexp.MustCompile(`^https?://(?:www\.)?github\.com/([a-zA-Z0-9_.-]{1,39})/?$`),
	"linkedin":  regexp.MustCompile(`^https?://(?:www\.)?linkedin\.com/in/([a-zA-Z0-9_.-]+)/?$`),
	"instagram": regexp.MustCompile(`^https?://(?:www\.)?instagram\.com/([a-zA-Z0-9_.]{1,30})/?$`),
	"youtube":   regexp.MustCompile(`^https?://(?:www\.)?youtube\.com/(?:c/|channel/|user/)?([a-zA-Z0-9_.-]+)/?$`),
	"tiktok":    regexp.MustCompile(`^https?://(?:www\.)?tiktok\.com/@([a-zA-Z0-9_.]{2,24})/?$`),
	"discord":   regexp.MustCompile(`^https?://discord\.gg/([a-zA-Z0-9]{2,32})/?$`),
	"twitch":    regexp.MustCompile(`^https?://(?:www\.)?twitch\.tv/([a-zA-Z0-9_]{4,25})/?$`),
	"reddit":    regexp.MustCompile(`^https?://(?:www\.)?reddit\.com/(?:u/|user/)([a-zA-Z0-9_-]{3,20})/?$`),
}

// URL shortener domains
var urlShorteners = []string{
	"bit.ly", "tinyurl.com", "t.co", "goo.gl", "ow.ly", "is.gd",
	"buff.ly", "adf.ly", "short.link", "tiny.one", "rb.gy",
}

// ActivityPub URL patterns
var activityPubPatterns = []*regexp.Regexp{
	regexp.MustCompile(`^https?://([^/]+)/users/([a-zA-Z0-9_.-]+)/?$`),     // Mastodon style
	regexp.MustCompile(`^https?://([^/]+)/@([a-zA-Z0-9_.-]+)/?$`),          // Mastodon @username
	regexp.MustCompile(`^https?://([^/]+)/u/([a-zA-Z0-9_.-]+)/?$`),         // Pleroma style
	regexp.MustCompile(`^https?://([^/]+)/profile/([a-zA-Z0-9_.-]+)/?$`),   // Generic profile
	regexp.MustCompile(`^https?://([^/]+)/actors/([a-zA-Z0-9_.-]+)/?$`),    // ActivityPub actors
}

// ExtractAndValidateURL performs comprehensive URL extraction and validation
func (uv *URLValidator) ExtractAndValidateURL(_ context.Context, rawURL string) (*URLExtractionResult, error) {
	result := &URLExtractionResult{
		OriginalURL:    strings.TrimSpace(rawURL),
		ValidationTags: make([]string, 0),
		Metadata:       make(map[string]string),
	}

	if err := common.ValidateRequiredParam("original_url", result.OriginalURL); err != nil {
		result.ValidationTags = append(result.ValidationTags, "empty_url")
		return result, nil
	}

	// Normalize URL - add protocol if missing
	normalizedURL := uv.normalizeURL(result.OriginalURL)
	result.NormalizedURL = normalizedURL

	// Parse URL
	parsedURL, err := url.Parse(normalizedURL)
	if err != nil {
		result.ValidationTags = append(result.ValidationTags, "invalid_format")
		uv.logger.Debug("failed to parse URL", zap.String("url", rawURL), zap.Error(err))
		return result, nil
	}

	// Additional validation - must have a proper scheme and host
	if err := common.ValidateRequiredParam("scheme", parsedURL.Scheme); err != nil {
		result.ValidationTags = append(result.ValidationTags, "invalid_format")
		return result, nil
	}
	if err := common.ValidateRequiredParam("host", parsedURL.Host); err != nil {
		result.ValidationTags = append(result.ValidationTags, "invalid_format")
		return result, nil
	}

	// Check if the normalized URL is significantly different from original (may indicate invalid input)
	if normalizedURL == result.OriginalURL && (!strings.Contains(result.OriginalURL, ".") || strings.Contains(result.OriginalURL, " ")) {
		result.ValidationTags = append(result.ValidationTags, "invalid_format")
		return result, nil
	}

	result.IsValid = true
	result.IsSecure = parsedURL.Scheme == "https"
	result.Domain = parsedURL.Hostname()
	result.Path = parsedURL.Path

	// Extract subdomain
	if domainParts := strings.Split(result.Domain, "."); len(domainParts) > 2 {
		result.Subdomain = strings.Join(domainParts[:len(domainParts)-2], ".")
	}

	// Check for URL shorteners
	result.IsShortened = uv.isShortened(result.Domain)
	if result.IsShortened {
		result.ValidationTags = append(result.ValidationTags, "shortened_url")
	}

	// Validate domain and security
	uv.validateDomain(result)

	// Extract social media profile information
	uv.extractSocialProfile(result, normalizedURL)

	// Extract ActivityPub username if applicable
	uv.extractActivityPubUsername(result, normalizedURL)

	// Add validation tags based on analysis
	uv.addValidationTags(result)

	return result, nil
}

// normalizeURL ensures URL has a protocol and basic formatting
func (uv *URLValidator) normalizeURL(rawURL string) string {
	// Remove common prefixes that aren't protocols
	rawURL = strings.TrimSpace(rawURL)
	
	// Check if it looks like a valid URL at all (basic validation)
	if len(rawURL) == 0 || (!strings.Contains(rawURL, ".") && !strings.HasPrefix(rawURL, "http")) {
		return rawURL // Return as-is for invalid inputs
	}
	
	// Remove www. prefix before adding protocol
	rawURL = strings.TrimPrefix(rawURL, "www.")
	
	// Add https if no protocol specified
	if !strings.HasPrefix(rawURL, "http://") && !strings.HasPrefix(rawURL, "https://") {
		rawURL = "https://" + rawURL
	}

	// Remove trailing slash for consistency
	rawURL = strings.TrimSuffix(rawURL, "/")

	return rawURL
}

// isShortened checks if a domain is a known URL shortener
func (uv *URLValidator) isShortened(domain string) bool {
	domain = strings.ToLower(domain)
	domain = strings.TrimPrefix(domain, "www.")
	
	for _, shortener := range urlShorteners {
		if domain == shortener {
			return true
		}
	}
	return false
}

// validateDomain performs domain-level validation
func (uv *URLValidator) validateDomain(result *URLExtractionResult) {
	domain := strings.ToLower(result.Domain)
	
	// Check for suspicious patterns
	if strings.Contains(domain, "bit.ly") || strings.Contains(domain, "tinyurl") {
		result.ValidationTags = append(result.ValidationTags, "url_shortener")
	}

	// Check for localhost/internal IPs
	if strings.Contains(domain, "localhost") || strings.Contains(domain, "127.0.0.1") || strings.Contains(domain, "192.168.") {
		result.ValidationTags = append(result.ValidationTags, "internal_url")
	}

	// Check for suspicious TLDs
	suspiciousTLDs := []string{".tk", ".ml", ".ga", ".cf"}
	for _, tld := range suspiciousTLDs {
		if strings.HasSuffix(domain, tld) {
			result.ValidationTags = append(result.ValidationTags, "suspicious_tld")
			break
		}
	}
}

// extractSocialProfile extracts social media profile information
func (uv *URLValidator) extractSocialProfile(result *URLExtractionResult, normalizedURL string) {
	for platform, pattern := range socialPlatforms {
		if matches := pattern.FindStringSubmatch(normalizedURL); matches != nil {
			result.IsSocial = true
			result.ProfileType = platform
			
			// Extract username (usually the last capture group)
			if len(matches) > 1 {
				result.Username = matches[len(matches)-1]
			}
			
			// Add platform-specific metadata
			switch platform {
			case "twitter":
				result.Metadata["platform_name"] = "Twitter/X"
				result.Metadata["profile_url_template"] = "https://twitter.com/{username}"
			case "mastodon":
				result.Metadata["platform_name"] = "Mastodon"
				if len(matches) >= 2 {
					result.Metadata["instance"] = matches[1]
					result.Metadata["profile_url_template"] = fmt.Sprintf("https://%s/@{username}", matches[1])
				}
			case "github":
				result.Metadata["platform_name"] = "GitHub"
				result.Metadata["profile_url_template"] = "https://github.com/{username}"
			}
			
			result.ValidationTags = append(result.ValidationTags, fmt.Sprintf("social_%s", platform))
			return // Found a match, no need to check ActivityPub patterns
		}
	}
}

// extractActivityPubUsername extracts username from ActivityPub URLs
func (uv *URLValidator) extractActivityPubUsername(result *URLExtractionResult, normalizedURL string) {
	// Only process if we haven't already identified this as a social media platform
	if result.IsSocial {
		return
	}
	
	for _, pattern := range activityPubPatterns {
		if matches := pattern.FindStringSubmatch(normalizedURL); matches != nil {
			if len(matches) >= 3 {
				result.ProfileType = "activitypub"
				result.Username = matches[2] // Username is typically the second capture group
				result.Metadata["instance"] = matches[1]
				result.Metadata["actor_url"] = normalizedURL
				result.ValidationTags = append(result.ValidationTags, "activitypub")
				break
			}
		}
	}
}

// addValidationTags adds additional validation tags based on URL analysis
func (uv *URLValidator) addValidationTags(result *URLExtractionResult) {
	if !result.IsSecure {
		result.ValidationTags = append(result.ValidationTags, "insecure_http")
	}
	
	if result.IsSocial {
		result.ValidationTags = append(result.ValidationTags, "social_media")
	}
	
	if result.Username != "" {
		result.ValidationTags = append(result.ValidationTags, "has_username")
	}
	
	// Check URL length
	if err := common.ValidateStringLength("original_url", result.OriginalURL, 0, 500); err != nil {
		result.ValidationTags = append(result.ValidationTags, "long_url")
	}
}

// ExtractProfileURLs extracts and validates URLs from user profile fields
func (uv *URLValidator) ExtractProfileURLs(ctx context.Context, fields []map[string]string) ([]*URLExtractionResult, error) {
	var results []*URLExtractionResult
	
	for _, field := range fields {
		value, exists := field["value"]
		if !exists {
			continue
		}
		if err := common.ValidateRequiredParam("field_value", value); err != nil {
			continue
		}
		
		// Look for URLs in the field value
		urls := uv.extractURLsFromText(value)
		for _, urlStr := range urls {
			result, err := uv.ExtractAndValidateURL(ctx, urlStr)
			if err != nil {
				uv.logger.Error("failed to validate profile URL", zap.String("url", urlStr), zap.Error(err))
				continue
			}
			
			if result.IsValid {
				// Add field context
				if name, nameExists := field["name"]; nameExists {
					result.Metadata["field_name"] = name
				}
				results = append(results, result)
			}
		}
	}
	
	return results, nil
}

// extractURLsFromText finds URLs in a text string
func (uv *URLValidator) extractURLsFromText(text string) []string {
	// URL regex pattern 
	urlRegex := regexp.MustCompile(`https?://[^\s<>"{}|\\^` + "`" + `\[\]]+`)
	matches := urlRegex.FindAllString(text, -1)
	
	// Also look for domains without protocol - be selective
	domainRegex := regexp.MustCompile(`\b(?:[a-zA-Z0-9-]+\.)+[a-zA-Z]{2,}(?:/[^\s<>"{}|\\^` + "`" + `\[\]]*)?`)
	domainMatches := domainRegex.FindAllString(text, -1)
	
	// Clean and deduplicate
	urlMap := make(map[string]bool)
	var urls []string
	
	// Process full HTTP URLs first
	for _, match := range matches {
		// Clean trailing punctuation
		cleanMatch := strings.TrimRight(match, "!.,;:)\"'")
		if len(cleanMatch) >= 12 && !urlMap[cleanMatch] { // Minimum: https://x.co
			urls = append(urls, cleanMatch)
			urlMap[cleanMatch] = true
		}
	}
	
	// Process domain-only matches
	for _, match := range domainMatches {
		// Skip if it's already a URL
		if strings.HasPrefix(match, "http") {
			continue
		}
		
		// Clean trailing punctuation from domain matches too
		cleanMatch := strings.TrimRight(match, "!.,;:)\"'")
		
		// Basic domain validation
		if len(cleanMatch) < 4 || !strings.Contains(cleanMatch, ".") {
			continue
		}
		
		// Check if it looks like a real domain
		parts := strings.Split(cleanMatch, ".")
		if len(parts) < 2 || len(parts[len(parts)-1]) < 2 {
			continue
		}
		
		// Avoid obviously invalid domains
		if cleanMatch == "localhost" || strings.Contains(cleanMatch, " ") {
			continue
		}
		
		fullURL := "https://" + cleanMatch
		if !urlMap[fullURL] && !urlMap[cleanMatch] {
			urls = append(urls, fullURL)
			urlMap[fullURL] = true
		}
	}
	
	return urls
}

// EnhancedExtractAccountFromReply extracts account information from reply URLs with enhanced pattern matching
func (uv *URLValidator) EnhancedExtractAccountFromReply(ctx context.Context, inReplyTo string) (string, error) {
	if err := common.ValidateRequiredParam("in_reply_to", inReplyTo); err != nil {
		return "", nil
	}

	// Handle existing POST# pattern
	if strings.HasPrefix(inReplyTo, "POST#") {
		parts := strings.Split(inReplyTo, "#")
		if len(parts) >= 2 {
			return parts[1], nil // Return the username part
		}
	}

	// For URLs, try enhanced extraction
	result, err := uv.ExtractAndValidateURL(ctx, inReplyTo)
	if err != nil {
		uv.logger.Debug("failed to extract account from reply URL", zap.String("url", inReplyTo), zap.Error(err))
		return "", nil
	}

	// If we extracted a username from the URL, return it
	if result.IsValid && result.Username != "" {
		return result.Username, nil
	}

	// Try to extract from path for other ActivityPub patterns
	if result.IsValid && result.Path != "" {
		return uv.extractUsernameFromPath(result.Path), nil
	}

	return "", nil
}

// extractUsernameFromPath attempts to extract username from URL path
func (uv *URLValidator) extractUsernameFromPath(path string) string {
	// Common patterns for username extraction from paths
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`/(?:users|user)/([a-zA-Z0-9_.-]+)`),
		regexp.MustCompile(`/@([a-zA-Z0-9_.-]+)`),
		regexp.MustCompile(`/profile/([a-zA-Z0-9_.-]+)`),
		regexp.MustCompile(`/([a-zA-Z0-9_.-]+)/status/`), // Twitter-like status URLs
		regexp.MustCompile(`/([a-zA-Z0-9_.-]+)/?$`), // Username at end of path
	}

	for _, pattern := range patterns {
		if matches := pattern.FindStringSubmatch(path); len(matches) > 1 {
			// Avoid returning overly long matches that are likely not usernames
			if len(matches[1]) <= 50 {
				return matches[1]
			}
		}
	}

	return ""
}

// ValidateAndNormalizeProfileURLs validates and normalizes URLs in user profile
func (uv *URLValidator) ValidateAndNormalizeProfileURLs(ctx context.Context, fields []map[string]string) ([]map[string]string, []string, error) {
	var normalizedFields []map[string]string
	var warnings []string

	for _, field := range fields {
		normalizedField := make(map[string]string)
		
		// Copy all existing fields
		for k, v := range field {
			normalizedField[k] = v
		}

		// Process value if it might contain URLs
		if value, exists := field["value"]; exists {
			if err := common.ValidateRequiredParam("field_value", value); err != nil {
				continue
			}
			urls := uv.extractURLsFromText(value)
			if len(urls) > 0 {
				// Process the first URL found (most common case)
				result, err := uv.ExtractAndValidateURL(ctx, urls[0])
				if err != nil {
					warnings = append(warnings, fmt.Sprintf("Failed to validate URL in field '%s': %v", field["name"], err))
				} else if result.IsValid {
					normalizedField["value"] = result.NormalizedURL
					
					// Add warnings for suspicious URLs
					for _, tag := range result.ValidationTags {
						switch tag {
						case "insecure_http":
							warnings = append(warnings, fmt.Sprintf("URL in field '%s' uses insecure HTTP", field["name"]))
						case "suspicious_tld":
							warnings = append(warnings, fmt.Sprintf("URL in field '%s' uses suspicious domain", field["name"]))
						case "url_shortener":
							warnings = append(warnings, fmt.Sprintf("URL in field '%s' is a shortened URL", field["name"]))
						}
					}
				} else {
					warnings = append(warnings, fmt.Sprintf("Invalid URL in field '%s'", field["name"]))
				}
			}
		}

		normalizedFields = append(normalizedFields, normalizedField)
	}

	return normalizedFields, warnings, nil
}
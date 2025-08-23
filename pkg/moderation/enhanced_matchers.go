package moderation

import (
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strings"
	"unicode"

	"github.com/equaltoai/lesser/pkg/common"
)

// EnhancedURLMatcher provides advanced URL pattern matching capabilities
type EnhancedURLMatcher struct {
	compiled map[string]*compiledURLPattern
}

// EnhancedIPMatcher provides advanced IP pattern matching capabilities  
type EnhancedIPMatcher struct {
	compiled map[string]*compiledIPPattern
}

// NewEnhancedURLMatcher creates a new enhanced URL matcher
func NewEnhancedURLMatcher() *EnhancedURLMatcher {
	return &EnhancedURLMatcher{
		compiled: make(map[string]*compiledURLPattern),
	}
}

// NewEnhancedIPMatcher creates a new enhanced IP matcher
func NewEnhancedIPMatcher() *EnhancedIPMatcher {
	return &EnhancedIPMatcher{
		compiled: make(map[string]*compiledIPPattern),
	}
}

// compiledURLPattern represents a compiled URL pattern for efficient matching
type compiledURLPattern struct {
	original      string
	domainPattern *regexp.Regexp
	pathPattern   *regexp.Regexp
	queryPattern  *regexp.Regexp
	scheme        string
	port          string
	exact         bool
	wildcard      bool
}

// compiledIPPattern represents a compiled IP pattern for efficient matching
type compiledIPPattern struct {
	original     string
	network      *net.IPNet
	singleIP     net.IP
	ipv4         bool
	ipv6         bool
	cidr         bool
	rangeStart   net.IP
	rangeEnd     net.IP
	isRange      bool
}

// URLPatternType represents the type of URL pattern
type URLPatternType string

const (
	// URLPatternExact represents exact URL matching
	URLPatternExact     URLPatternType = "exact"     // Exact match
	// URLPatternDomain represents domain and all subdomain matching
	URLPatternDomain    URLPatternType = "domain"    // Domain and all subdomains
	// URLPatternSubdomain represents specific subdomain pattern matching
	URLPatternSubdomain URLPatternType = "subdomain" // Specific subdomain pattern
	// URLPatternPath represents path-based URL matching
	URLPatternPath      URLPatternType = "path"      // Path-based matching
	// URLPatternQuery represents query parameter URL matching
	URLPatternQuery     URLPatternType = "query"     // Query parameter matching
	// URLPatternRegex represents regular expression URL matching
	URLPatternRegex     URLPatternType = "regex"     // Regular expression
	
	// URLPatternExactStr represents the string for exact URL pattern validation
	URLPatternExactStr     = "url_exact"
	// URLPatternDomainStr represents the string for domain URL pattern validation
	URLPatternDomainStr    = "url_domain"
	// URLPatternSubdomainStr represents the string for subdomain URL pattern validation
	URLPatternSubdomainStr = "url_subdomain"
	// URLPatternPathStr represents the string for path URL pattern validation
	URLPatternPathStr      = "url_path"
	// URLPatternQueryStr represents the string for query URL pattern validation
	URLPatternQueryStr     = "url_query"
	// URLPatternRegexStr represents the string for regex URL pattern validation
	URLPatternRegexStr     = "url_regex"
)

// IPPatternType represents the type of IP pattern
type IPPatternType string

const (
	// IPPatternSingle represents single IP address matching
	IPPatternSingle IPPatternType = "single" // Single IP address
	// IPPatternCIDR represents CIDR block matching
	IPPatternCIDR   IPPatternType = "cidr"   // CIDR block
	// IPPatternRange represents IP range matching
	IPPatternRange  IPPatternType = "range"  // IP range
	// IPPatternRegex represents regular expression IP matching
	IPPatternRegex  IPPatternType = "regex"  // Regular expression
	
	// IPPatternSingleStr represents the string for single IP pattern validation
	IPPatternSingleStr = "ip_single"
	// IPPatternCIDRStr represents the string for CIDR IP pattern validation
	IPPatternCIDRStr   = "ip_cidr"
	// IPPatternRangeStr represents the string for IP range pattern validation
	IPPatternRangeStr  = "ip_range"
	// IPPatternRegexStr represents the string for IP regex pattern validation
	IPPatternRegexStr  = "ip_regex"
)

// CompileURLPattern compiles a URL pattern for efficient matching
func (m *EnhancedURLMatcher) CompileURLPattern(pattern string, patternType URLPatternType) error {
	if err := common.ValidateRequiredParam("pattern", pattern); err != nil {
		return err
	}

	compiled := &compiledURLPattern{
		original: pattern,
	}

	switch patternType {
	case URLPatternExact:
		return m.compileExactURLPattern(pattern, compiled)
	case URLPatternDomain:
		return m.compileDomainPattern(pattern, compiled)
	case URLPatternSubdomain:
		return m.compileSubdomainPattern(pattern, compiled)
	case URLPatternPath:
		return m.compilePathPattern(pattern, compiled)
	case URLPatternQuery:
		return m.compileQueryPattern(pattern, compiled)
	case URLPatternRegex:
		return m.compileRegexURLPattern(pattern, compiled)
	default:
		return fmt.Errorf("%w: %s", ErrUnsupportedURLPatternType, patternType)
	}
}

// CompileIPPattern compiles an IP pattern for efficient matching
func (m *EnhancedIPMatcher) CompileIPPattern(pattern string, patternType IPPatternType) error {
	if err := common.ValidateRequiredParam("pattern", pattern); err != nil {
		return err
	}

	compiled := &compiledIPPattern{
		original: pattern,
	}

	switch patternType {
	case IPPatternSingle:
		return m.compileSingleIPPattern(pattern, compiled)
	case IPPatternCIDR:
		return m.compileCIDRPattern(pattern, compiled)
	case IPPatternRange:
		return m.compileRangePattern(pattern, compiled)
	case IPPatternRegex:
		return m.compileRegexIPPattern(pattern, compiled)
	default:
		return fmt.Errorf("%w: %s", ErrUnsupportedIPPatternType, patternType)
	}
}

// MatchURL matches a URL against compiled patterns
func (m *EnhancedURLMatcher) MatchURL(urlStr string, patterns []string) (bool, string, error) {
	// Normalize URL first
	normalizedURL, err := m.normalizeURL(urlStr)
	if err != nil {
		return false, "", fmt.Errorf("%w: %w", ErrFailedToNormalizeURL, err)
	}

	parsedURL, err := url.Parse(normalizedURL)
	if err != nil {
		return false, "", fmt.Errorf("%w: %w", ErrFailedToParseURL, err)
	}

	for _, pattern := range patterns {
		compiled, exists := m.compiled[pattern]
		if !exists {
			continue // Skip patterns that aren't compiled
		}

		matched, err := m.matchCompiledURL(parsedURL, compiled)
		if err != nil {
			continue // Skip patterns with errors
		}
		if matched {
			return true, pattern, nil
		}
	}

	return false, "", nil
}

// MatchIP matches an IP address against compiled patterns
func (m *EnhancedIPMatcher) MatchIP(ipStr string, patterns []string) (bool, string, error) {
	// Parse IP address
	ip := net.ParseIP(strings.TrimSpace(ipStr))
	if ip == nil {
		return false, "", fmt.Errorf("%w: %s", ErrInvalidIPAddress, ipStr)
	}

	for _, pattern := range patterns {
		compiled, exists := m.compiled[pattern]
		if !exists {
			continue // Skip patterns that aren't compiled
		}

		matched := m.matchCompiledIP(ip, compiled)
		if matched {
			return true, pattern, nil
		}
	}

	return false, "", nil
}

// normalizeURL normalizes a URL for consistent matching
func (m *EnhancedURLMatcher) normalizeURL(urlStr string) (string, error) {
	// Add scheme if missing
	if !strings.Contains(urlStr, "://") {
		urlStr = "http://" + urlStr
	}

	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return "", err
	}

	// Normalize scheme to lowercase
	parsedURL.Scheme = strings.ToLower(parsedURL.Scheme)

	// Normalize host to lowercase
	parsedURL.Host = strings.ToLower(parsedURL.Host)

	// Remove default ports
	if (parsedURL.Scheme == "http" && strings.HasSuffix(parsedURL.Host, ":80")) ||
		(parsedURL.Scheme == "https" && strings.HasSuffix(parsedURL.Host, ":443")) {
		host, _, _ := net.SplitHostPort(parsedURL.Host)
		parsedURL.Host = host
	}

	// Normalize path - remove duplicate slashes, resolve . and ..
	if err := common.ValidateRequiredParam("parsedURL.Path", parsedURL.Path); err != nil {
		parsedURL.Path = "/"
	}
	
	// Clean up path by removing double slashes and resolving relative paths
	path := parsedURL.Path
	if strings.Contains(path, "//") {
		path = regexp.MustCompile(`/+`).ReplaceAllString(path, "/")
	}

	// Simple path resolution for . and ..
	if strings.Contains(path, "/.") {
		parts := strings.Split(path, "/")
		cleaned := make([]string, 0, len(parts))
		for _, part := range parts {
			if part == "." || common.ValidateRequiredParam("part", part) != nil {
				continue
			}
			if part == ".." && len(cleaned) > 0 {
				cleaned = cleaned[:len(cleaned)-1]
			} else if part != ".." {
				cleaned = append(cleaned, part)
			}
		}
		if err := common.ValidateSliceNotEmpty("cleaned", cleaned); err != nil {
			path = "/"
		} else {
			path = "/" + strings.Join(cleaned, "/")
		}
	}
	parsedURL.Path = path

	// Sort query parameters for consistent matching
	if parsedURL.RawQuery != "" {
		query := parsedURL.Query()
		parsedURL.RawQuery = query.Encode()
	}

	return parsedURL.String(), nil
}

// compileExactURLPattern compiles an exact URL match pattern
func (m *EnhancedURLMatcher) compileExactURLPattern(pattern string, compiled *compiledURLPattern) error {
	normalizedPattern, err := m.normalizeURL(pattern)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrFailedToNormalizePattern, err)
	}

	compiled.exact = true
	compiled.original = normalizedPattern
	m.compiled[pattern] = compiled
	return nil
}

// compileDomainPattern compiles a domain-based pattern (matches domain and all subdomains)
func (m *EnhancedURLMatcher) compileDomainPattern(pattern string, compiled *compiledURLPattern) error {
	// Clean domain pattern
	domain := strings.TrimSpace(strings.ToLower(pattern))
	
	// Remove protocol if present
	if strings.Contains(domain, "://") {
		parsedURL, err := url.Parse(domain)
		if err != nil {
			return fmt.Errorf("%w: %w", ErrInvalidDomainPattern, err)
		}
		domain = parsedURL.Host
		compiled.scheme = parsedURL.Scheme
	}

	// Remove port if present
	if strings.Contains(domain, ":") {
		host, port, err := net.SplitHostPort(domain)
		if err == nil {
			domain = host
			compiled.port = port
		}
	}

	// Create regex pattern for domain and subdomains
	escapedDomain := regexp.QuoteMeta(domain)
	regexPattern := fmt.Sprintf(`(^|\.)%s$`, escapedDomain)
	
	regex, err := regexp.Compile(regexPattern)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrFailedToCompileDomainRegex, err)
	}

	compiled.domainPattern = regex
	m.compiled[pattern] = compiled
	return nil
}

// compileSubdomainPattern compiles a subdomain wildcard pattern
func (m *EnhancedURLMatcher) compileSubdomainPattern(pattern string, compiled *compiledURLPattern) error {
	// Handle wildcard subdomains like *.example.com
	domain := strings.TrimSpace(strings.ToLower(pattern))
	
	if strings.HasPrefix(domain, "*.") {
		domain = domain[2:] // Remove *.
		compiled.wildcard = true
	}

	// Escape special regex characters except for our wildcards
	escapedDomain := regexp.QuoteMeta(domain)
	
	var regexPattern string
	if compiled.wildcard {
		// Match any subdomain
		regexPattern = fmt.Sprintf(`^[^.]+\.%s$`, escapedDomain)
	} else {
		// Exact domain match
		regexPattern = fmt.Sprintf(`^%s$`, escapedDomain)
	}

	regex, err := regexp.Compile(regexPattern)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrFailedToCompileSubdomainRegex, err)
	}

	compiled.domainPattern = regex
	m.compiled[pattern] = compiled
	return nil
}

// compilePathPattern compiles a path-based pattern
func (m *EnhancedURLMatcher) compilePathPattern(pattern string, compiled *compiledURLPattern) error {
	// Handle path patterns like /api/*, /admin/**, etc.
	pathPattern := strings.TrimSpace(pattern)
	
	// Convert path wildcards to regex
	// * matches any single path segment
	// ** matches any number of path segments
	regexPattern := regexp.QuoteMeta(pathPattern)
	regexPattern = strings.ReplaceAll(regexPattern, `\*\*`, `.*`)
	regexPattern = strings.ReplaceAll(regexPattern, `\*`, `[^/]*`)
	regexPattern = "^" + regexPattern

	regex, err := regexp.Compile(regexPattern)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrFailedToCompilePathRegex, err)
	}

	compiled.pathPattern = regex
	m.compiled[pattern] = compiled
	return nil
}

// compileQueryPattern compiles a query parameter pattern
func (m *EnhancedURLMatcher) compileQueryPattern(pattern string, compiled *compiledURLPattern) error {
	// Handle query patterns like ?param=value, ?param=*, etc.
	queryPattern := strings.TrimSpace(pattern)
	
	// Remove leading ? if present
	queryPattern = strings.TrimPrefix(queryPattern, "?")

	// Convert wildcards to regex
	regexPattern := regexp.QuoteMeta(queryPattern)
	regexPattern = strings.ReplaceAll(regexPattern, `\*`, `.*`)

	regex, err := regexp.Compile(regexPattern)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrFailedToCompileQueryRegex, err)
	}

	compiled.queryPattern = regex
	m.compiled[pattern] = compiled
	return nil
}

// compileRegexURLPattern compiles a regex URL pattern
func (m *EnhancedURLMatcher) compileRegexURLPattern(pattern string, compiled *compiledURLPattern) error {
	regex, err := regexp.Compile(pattern)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrFailedToCompileURLRegex, err)
	}

	compiled.domainPattern = regex // Store in domainPattern for full URL matching
	m.compiled[pattern] = compiled
	return nil
}

// matchCompiledURL matches a parsed URL against a compiled pattern
func (m *EnhancedURLMatcher) matchCompiledURL(parsedURL *url.URL, compiled *compiledURLPattern) (bool, error) {
	if compiled.exact {
		normalizedURL := parsedURL.String()
		return normalizedURL == compiled.original, nil
	}

	// Check scheme if specified
	if compiled.scheme != "" && parsedURL.Scheme != compiled.scheme {
		return false, nil
	}

	// Check port if specified
	if compiled.port != "" {
		_, port, err := net.SplitHostPort(parsedURL.Host)
		if err != nil || port != compiled.port {
			return false, nil
		}
	}

	// Check domain pattern
	if compiled.domainPattern != nil {
		host := parsedURL.Host
		// Remove port for domain matching
		if strings.Contains(host, ":") {
			h, _, err := net.SplitHostPort(host)
			if err == nil {
				host = h
			}
		}
		
		// For regex patterns that should match full URL (no specific path/query patterns)
		if compiled.pathPattern == nil && compiled.queryPattern == nil && (compiled.original != host) {
			// Check if this might be a full URL regex
			if strings.Contains(compiled.original, "://") || strings.Contains(compiled.original, ".*") {
				return compiled.domainPattern.MatchString(parsedURL.String()), nil
			}
		}
		
		if !compiled.domainPattern.MatchString(host) {
			return false, nil
		}
	}

	// Check path pattern
	if compiled.pathPattern != nil {
		if !compiled.pathPattern.MatchString(parsedURL.Path) {
			return false, nil
		}
	}

	// Check query pattern
	if compiled.queryPattern != nil {
		if !compiled.queryPattern.MatchString(parsedURL.RawQuery) {
			return false, nil
		}
	}

	return true, nil
}

// compileSingleIPPattern compiles a single IP address pattern
func (m *EnhancedIPMatcher) compileSingleIPPattern(pattern string, compiled *compiledIPPattern) error {
	ip := net.ParseIP(strings.TrimSpace(pattern))
	if ip == nil {
		return fmt.Errorf("%w: %s", ErrInvalidIPAddress, pattern)
	}

	compiled.singleIP = ip
	compiled.ipv4 = ip.To4() != nil
	compiled.ipv6 = !compiled.ipv4

	m.compiled[pattern] = compiled
	return nil
}

// compileCIDRPattern compiles a CIDR block pattern
func (m *EnhancedIPMatcher) compileCIDRPattern(pattern string, compiled *compiledIPPattern) error {
	_, network, err := net.ParseCIDR(strings.TrimSpace(pattern))
	if err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidCIDRBlock, err)
	}

	compiled.network = network
	compiled.cidr = true
	compiled.ipv4 = network.IP.To4() != nil
	compiled.ipv6 = !compiled.ipv4

	m.compiled[pattern] = compiled
	return nil
}

// compileRangePattern compiles an IP range pattern (start-end)
func (m *EnhancedIPMatcher) compileRangePattern(pattern string, compiled *compiledIPPattern) error {
	parts := strings.Split(strings.TrimSpace(pattern), "-")
	if len(parts) != 2 {
		return fmt.Errorf("%w: %s", ErrInvalidIPRangeFormat, pattern)
	}

	startIP := net.ParseIP(strings.TrimSpace(parts[0]))
	endIP := net.ParseIP(strings.TrimSpace(parts[1]))

	if startIP == nil || endIP == nil {
		return fmt.Errorf("%w: %s", ErrInvalidIPAddressesInRange, pattern)
	}

	// Ensure both IPs are same version
	startV4 := startIP.To4() != nil
	endV4 := endIP.To4() != nil
	if startV4 != endV4 {
		return fmt.Errorf("%w: %s", ErrIPRangeMixedVersions, pattern)
	}

	// Validate range order
	if compareIPs(startIP, endIP) > 0 {
		return fmt.Errorf("%w: %s", ErrInvalidIPRangeOrder, pattern)
	}

	compiled.rangeStart = startIP
	compiled.rangeEnd = endIP
	compiled.isRange = true
	compiled.ipv4 = startV4
	compiled.ipv6 = !startV4

	m.compiled[pattern] = compiled
	return nil
}

// compileRegexIPPattern compiles a regex IP pattern
func (m *EnhancedIPMatcher) compileRegexIPPattern(pattern string, compiled *compiledIPPattern) error {
	// For security, limit regex patterns to prevent ReDoS attacks
	if err := common.ValidateStringLength("pattern", pattern, 0, 1000); err != nil {
		return err
	}

	// Check for potentially dangerous regex patterns
	dangerousPatterns := []string{
		`(.*){`,     // Nested quantifiers
		`.*.*.*.*`,  // Multiple greedy quantifiers
		`(.*)+`,     // Nested quantifiers with +
		`([^x])*`,   // Negated character class with quantifier
	}
	
	for _, dangerous := range dangerousPatterns {
		if strings.Contains(pattern, dangerous) {
			return ErrUnsafeRegexPattern
		}
	}

	_, err := regexp.Compile(pattern)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrFailedToCompileIPRegex, err)
	}

	// Store pattern string for matching (we'll compile it during match)
	compiled.original = pattern
	m.compiled[pattern] = compiled
	return nil
}

// matchCompiledIP matches an IP address against a compiled pattern
func (m *EnhancedIPMatcher) matchCompiledIP(ip net.IP, compiled *compiledIPPattern) bool {
	if compiled.singleIP != nil {
		return ip.Equal(compiled.singleIP)
	}

	if compiled.network != nil {
		return compiled.network.Contains(ip)
	}

	if compiled.isRange {
		return compareIPs(compiled.rangeStart, ip) <= 0 && compareIPs(ip, compiled.rangeEnd) <= 0
	}

	// Handle regex patterns (stored in original field)
	if compiled.original != "" && compiled.singleIP == nil && compiled.network == nil && !compiled.isRange {
		regex, err := regexp.Compile(compiled.original)
		if err != nil {
			return false
		}
		return regex.MatchString(ip.String())
	}

	return false
}

// compareIPs compares two IP addresses, returns -1 if a < b, 0 if a == b, 1 if a > b
func compareIPs(a, b net.IP) int {
	// Ensure both are same format
	a16 := a.To16()
	b16 := b.To16()
	
	for i := 0; i < len(a16); i++ {
		if a16[i] < b16[i] {
			return -1
		}
		if a16[i] > b16[i] {
			return 1
		}
	}
	return 0
}

// ValidateURLPattern validates a URL pattern before compilation
func ValidateURLPattern(pattern string, patternType URLPatternType) error {
	if err := common.ValidateRequiredParam("pattern", pattern); err != nil {
		return err
	}

	if err := common.ValidateStringLength("pattern", pattern, 0, 2048); err != nil {
		return err
	}

	// Check for potentially malicious patterns
	if strings.Count(pattern, "*") > 10 {
		return ErrTooManyWildcards
	}

	// Validate based on pattern type
	switch patternType {
	case URLPatternExact:
		_, err := url.Parse(pattern)
		return err
	case URLPatternDomain, URLPatternSubdomain:
		return validateDomainPattern(pattern)
	case URLPatternPath:
		return validatePathPattern(pattern)
	case URLPatternQuery:
		return validateQueryPattern(pattern)
	case URLPatternRegex:
		return validateRegexPattern(pattern)
	default:
		return fmt.Errorf("%w: %s", ErrUnsupportedPatternType, patternType)
	}
}

// ValidateIPPattern validates an IP pattern before compilation
func ValidateIPPattern(pattern string, patternType IPPatternType) error {
	if err := common.ValidateRequiredParam("pattern", pattern); err != nil {
		return err
	}

	if err := common.ValidateStringLength("pattern", pattern, 0, 256); err != nil {
		return err
	}

	switch patternType {
	case IPPatternSingle:
		if net.ParseIP(strings.TrimSpace(pattern)) == nil {
			return fmt.Errorf("%w: %s", ErrInvalidIPAddress, pattern)
		}
	case IPPatternCIDR:
		_, _, err := net.ParseCIDR(strings.TrimSpace(pattern))
		return err
	case IPPatternRange:
		parts := strings.Split(pattern, "-")
		if len(parts) != 2 {
			return ErrInvalidIPRangeFormat
		}
		if net.ParseIP(strings.TrimSpace(parts[0])) == nil || net.ParseIP(strings.TrimSpace(parts[1])) == nil {
			return ErrInvalidIPAddressesInRange
		}
	case IPPatternRegex:
		return validateRegexPattern(pattern)
	default:
		return fmt.Errorf("%w: %s", ErrUnsupportedPatternType, patternType)
	}

	return nil
}

// validateDomainPattern validates a domain pattern
func validateDomainPattern(pattern string) error {
	domain := pattern
	
	// Remove protocol if present
	if strings.Contains(domain, "://") {
		parsedURL, err := url.Parse(domain)
		if err != nil {
			return fmt.Errorf("%w: %w", ErrInvalidURLInDomainPattern, err)
		}
		domain = parsedURL.Host
	}

	// Handle wildcard domains
	domain = strings.TrimPrefix(domain, "*.")

	// Remove port if present
	if strings.Contains(domain, ":") {
		host, _, err := net.SplitHostPort(domain)
		if err != nil {
			return fmt.Errorf("%w: %w", ErrInvalidHostPortFormat, err)
		}
		domain = host
	}

	// Basic domain validation
	if err := common.ValidateStringLength("domain", domain, 1, 253); err != nil {
		return err
	}

	// Check for valid characters
	for _, r := range domain {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '.' && r != '-' {
			return fmt.Errorf("%w: %c", ErrInvalidCharacterInDomain, r)
		}
	}

	// Check for valid domain structure
	parts := strings.Split(domain, ".")
	for _, part := range parts {
		if err := common.ValidateStringLength("domain_part", part, 1, 63); err != nil {
			return err
		}
		if strings.HasPrefix(part, "-") || strings.HasSuffix(part, "-") {
			return ErrDomainPartHyphenRule
		}
	}

	return nil
}

// validatePathPattern validates a path pattern
func validatePathPattern(pattern string) error {
	if !strings.HasPrefix(pattern, "/") {
		return ErrPathMustStartWithSlash
	}

	// Check for path traversal attempts
	if strings.Contains(pattern, "..") {
		return ErrPathTraversalNotAllowed
	}

	return nil
}

// validateQueryPattern validates a query parameter pattern
func validateQueryPattern(pattern string) error {
	// Remove leading ? if present
	pattern = strings.TrimPrefix(pattern, "?")

	// Basic validation - ensure it looks like query parameters
	if strings.Contains(pattern, " ") {
		return ErrSpacesNotAllowedInQuery
	}

	return nil
}

// validateRegexPattern validates a regex pattern for safety
func validateRegexPattern(pattern string) error {
	if err := common.ValidateStringLength("pattern", pattern, 0, 1000); err != nil {
		return err
	}

	// Check for potentially dangerous regex patterns that could cause ReDoS
	dangerousPatterns := []string{
		`(.*){`,     // Nested quantifiers
		`.*.*.*.*`,  // Multiple greedy quantifiers
		`(.*)+`,     // Nested quantifiers with +
		`([^x])*`,   // Negated character class with quantifier
		`(a+)+`,     // Catastrophic backtracking
		`(a*)*`,     // Catastrophic backtracking
	}
	
	for _, dangerous := range dangerousPatterns {
		if strings.Contains(pattern, dangerous) {
			return ErrUnsafeRegexPattern
		}
	}

	// Try to compile to validate syntax
	_, err := regexp.Compile(pattern)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidRegexPattern, err)
	}

	return nil
}
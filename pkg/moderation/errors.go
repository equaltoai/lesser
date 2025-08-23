package moderation

import "errors"

// Error constants for moderation package

// Pattern validation errors
var (
	ErrInvalidPattern      = errors.New("invalid pattern")
	ErrInvalidRegexPattern = errors.New("invalid regex pattern")
	ErrInvalidPatternType  = errors.New("invalid pattern type")
	ErrInvalidSeverity     = errors.New("invalid severity")
)

// Pattern retrieval errors
var (
	ErrFailedToGetPatterns         = errors.New("failed to get patterns")
	ErrFailedToGetPattern          = errors.New("failed to get pattern")
	ErrFailedToGetEnhancedPatterns = errors.New("failed to get enhanced patterns")
)

// Enhanced pattern functionality errors
var (
	ErrEnhancedPatternsNotAvailable        = errors.New("enhanced patterns not available")
	ErrEnhancedPatternValidationNotAvailable = errors.New("enhanced pattern validation not available")
	ErrEnhancedPatternStatisticsNotAvailable = errors.New("enhanced pattern statistics not available")
)

// Pattern validation errors
var (
	ErrPatternValidationFailed = errors.New("pattern validation failed")
)

// Enhanced pattern matching errors - URL patterns
var (
	ErrUnsupportedURLPatternType = errors.New("unsupported URL pattern type")
	ErrUnsupportedIPPatternType  = errors.New("unsupported IP pattern type")
	ErrUnsupportedPatternType    = errors.New("unsupported pattern type")

	// URL processing errors
	ErrFailedToNormalizeURL    = errors.New("failed to normalize URL")
	ErrFailedToParseURL        = errors.New("failed to parse URL")
	ErrInvalidIPAddress        = errors.New("invalid IP address")
	ErrFailedToNormalizePattern = errors.New("failed to normalize pattern")

	// Domain pattern errors
	ErrInvalidDomainPattern     = errors.New("invalid domain pattern")
	ErrFailedToCompileDomainRegex = errors.New("failed to compile domain regex")
	ErrInvalidURLInDomainPattern = errors.New("invalid URL in domain pattern")
	ErrInvalidHostPortFormat    = errors.New("invalid host:port format")
	ErrInvalidCharacterInDomain = errors.New("invalid character in domain")
	ErrDomainPartHyphenRule     = errors.New("domain part cannot start or end with hyphen")

	// Pattern compilation errors
	ErrFailedToCompileSubdomainRegex = errors.New("failed to compile subdomain regex")
	ErrFailedToCompilePathRegex      = errors.New("failed to compile path regex")
	ErrFailedToCompileQueryRegex     = errors.New("failed to compile query regex")
	ErrFailedToCompileURLRegex       = errors.New("failed to compile URL regex")
	ErrFailedToCompileIPRegex        = errors.New("failed to compile IP regex")

	// IP pattern errors
	ErrInvalidCIDRBlock           = errors.New("invalid CIDR block")
	ErrInvalidIPRangeFormat       = errors.New("invalid IP range format, expected start-end")
	ErrInvalidIPAddressesInRange  = errors.New("invalid IP addresses in range")
	ErrIPRangeMixedVersions       = errors.New("IP range must use same IP version")
	ErrInvalidIPRangeOrder        = errors.New("invalid IP range, start must be <= end")

	// Security errors
	ErrUnsafeRegexPattern = errors.New("potentially unsafe regex pattern detected")
	ErrTooManyWildcards   = errors.New("too many wildcards in pattern")

	// Path pattern errors
	ErrPathMustStartWithSlash = errors.New("path pattern must start with /")
	ErrPathTraversalNotAllowed = errors.New("path traversal not allowed in patterns")

	// Query pattern errors
	ErrSpacesNotAllowedInQuery = errors.New("spaces not allowed in query patterns")
)

// Consensus-related errors
var (
	// Consensus validation errors
	ErrInsufficientReviewers    = errors.New("insufficient reviewers")
	ErrInsufficientTrustWeight  = errors.New("insufficient trust weight")
	ErrConsensusNotReached      = errors.New("insufficient consensus")
	ErrInsufficientConsensus    = errors.New("insufficient consensus for critical action")
	
	// Vote processing errors
	ErrVoteProcessingFailed     = errors.New("failed to process vote")
	ErrConsensusCalculationFailed = errors.New("consensus calculation failed")
	
	// Storage operation errors for consensus
	ErrModerationEventRetrievalFailed = errors.New("failed to get moderation event")
	ErrModerationReviewAddFailed      = errors.New("failed to add review")
	ErrModerationReviewsRetrievalFailed = errors.New("failed to get reviews")
	ErrModerationDecisionStorageFailed  = errors.New("failed to store decision")
	ErrModerationQueueRetrievalFailed   = errors.New("failed to get moderation queue")
)

// Core moderation errors
var (
	// Content moderation process errors
	ErrPatternModerationFailed     = errors.New("pattern moderation failed")
	ErrAIAnalysisFailed           = errors.New("AI analysis failed")
	ErrTextAnalysisFailed         = errors.New("text analysis failed")
	ErrImageAnalysisFailed        = errors.New("image analysis failed")
	ErrModerationDecisionFailed   = errors.New("moderation decision failed")
	ErrModerationSystemUnavailable = errors.New("moderation system unavailable")
	
	// Content assessment errors
	ErrContentViolatesPolicy      = errors.New("content violates policy")
	ErrInsufficientModerationData = errors.New("insufficient moderation data")
	ErrModerationRuleNotFound     = errors.New("moderation rule not found")
	
	// Storage and persistence errors
	ErrFailedToUpdateModerationDecision = errors.New("failed to update moderation decision")
	ErrFailedToStoreModerationDecision  = errors.New("failed to store moderation decision")
	ErrFailedToRetrieveModerationQueue  = errors.New("failed to retrieve moderation queue")
)
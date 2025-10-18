package moderation

import (
	stdErrors "errors"

	"github.com/equaltoai/lesser/pkg/errors"
)

// Error constants for moderation package

// Pattern validation errors
var (
	ErrInvalidPattern      = errors.PatternValidationFailed("invalid pattern")
	ErrInvalidRegexPattern = errors.PatternValidationFailed("invalid regex pattern")
	ErrInvalidPatternType  = errors.PatternValidationFailed("invalid pattern type")
	ErrInvalidSeverity     = errors.NewValidationError("severity", "invalid")
)

// Pattern retrieval errors
var (
	ErrFailedToGetPatterns         = errors.PatternQueryFailed(stdErrors.New("failed to get patterns"))
	ErrFailedToGetPattern          = errors.FailedToGet("pattern", stdErrors.New("failed to get pattern"))
	ErrFailedToGetEnhancedPatterns = errors.FailedToGet("enhanced patterns", stdErrors.New("failed to get enhanced patterns"))
)

// Enhanced pattern functionality errors
var (
	ErrEnhancedPatternsNotAvailable          = errors.ServiceUnavailable("enhanced patterns")
	ErrEnhancedPatternValidationNotAvailable = errors.ServiceUnavailable("enhanced pattern validation")
	ErrEnhancedPatternStatisticsNotAvailable = errors.ServiceUnavailable("enhanced pattern statistics")
)

// Pattern validation errors
var (
	ErrPatternValidationFailed = errors.PatternValidationFailed("unknown reason")
)

// Enhanced pattern matching errors - URL patterns
var (
	ErrUnsupportedURLPatternType = errors.NewValidationError("validation", "unsupported URL pattern type")
	ErrUnsupportedIPPatternType  = errors.NewValidationError("validation", "unsupported IP pattern type")
	ErrUnsupportedPatternType    = errors.NewValidationError("validation", "unsupported pattern type")

	// URL processing errors
	ErrFailedToNormalizeURL     = errors.NewValidationError("validation", "failed to normalize URL")
	ErrFailedToParseURL         = errors.NewValidationError("validation", "failed to parse URL")
	ErrInvalidIPAddress         = errors.NewValidationError("validation", "invalid IP address")
	ErrFailedToNormalizePattern = errors.NewValidationError("validation", "failed to normalize pattern")

	// Domain pattern errors
	ErrInvalidDomainPattern       = errors.NewValidationError("validation", "invalid domain pattern")
	ErrFailedToCompileDomainRegex = errors.NewValidationError("validation", "failed to compile domain regex")
	ErrInvalidURLInDomainPattern  = errors.NewValidationError("validation", "invalid URL in domain pattern")
	ErrInvalidHostPortFormat      = errors.NewValidationError("validation", "invalid host:port format")
	ErrInvalidCharacterInDomain   = errors.NewValidationError("validation", "invalid character in domain")
	ErrDomainPartHyphenRule       = errors.NewValidationError("validation", "domain part cannot start or end with hyphen")

	// Pattern compilation errors
	ErrFailedToCompileSubdomainRegex = errors.NewValidationError("validation", "failed to compile subdomain regex")
	ErrFailedToCompilePathRegex      = errors.NewValidationError("validation", "failed to compile path regex")
	ErrFailedToCompileQueryRegex     = errors.NewValidationError("validation", "failed to compile query regex")
	ErrFailedToCompileURLRegex       = errors.NewValidationError("validation", "failed to compile URL regex")
	ErrFailedToCompileIPRegex        = errors.NewValidationError("validation", "failed to compile IP regex")

	// IP pattern errors
	ErrInvalidCIDRBlock          = errors.NewValidationError("validation", "invalid CIDR block")
	ErrInvalidIPRangeFormat      = errors.NewValidationError("validation", "invalid IP range format, expected start-end")
	ErrInvalidIPAddressesInRange = errors.NewValidationError("validation", "invalid IP addresses in range")
	ErrIPRangeMixedVersions      = errors.NewValidationError("validation", "IP range must use same IP version")
	ErrInvalidIPRangeOrder       = errors.NewValidationError("validation", "invalid IP range, start must be <= end")

	// Security errors
	ErrUnsafeRegexPattern = errors.NewValidationError("validation", "potentially unsafe regex pattern detected")
	ErrTooManyWildcards   = errors.NewValidationError("validation", "too many wildcards in pattern")

	// Path pattern errors
	ErrPathMustStartWithSlash  = errors.NewValidationError("validation", "path pattern must start with /")
	ErrPathTraversalNotAllowed = errors.NewValidationError("validation", "path traversal not allowed in patterns")

	// Query pattern errors
	ErrSpacesNotAllowedInQuery = errors.NewValidationError("validation", "spaces not allowed in query patterns")
)

// Consensus-related errors
var (
	// Consensus validation errors
	ErrInsufficientReviewers   = errors.NewValidationError("validation", "insufficient reviewers")
	ErrInsufficientTrustWeight = errors.NewValidationError("validation", "insufficient trust weight")
	ErrConsensusNotReached     = errors.NewValidationError("validation", "insufficient consensus")
	ErrInsufficientConsensus   = errors.NewValidationError("validation", "insufficient consensus for critical action")

	// Vote processing errors
	ErrVoteProcessingFailed       = errors.NewValidationError("validation", "failed to process vote")
	ErrConsensusCalculationFailed = errors.NewValidationError("validation", "consensus calculation failed")

	// Storage operation errors for consensus
	ErrModerationEventRetrievalFailed   = errors.NewValidationError("validation", "failed to get moderation event")
	ErrModerationReviewAddFailed        = errors.NewValidationError("validation", "failed to add review")
	ErrModerationReviewsRetrievalFailed = errors.NewValidationError("validation", "failed to get reviews")
	ErrModerationDecisionStorageFailed  = errors.NewValidationError("validation", "failed to store decision")
	ErrModerationQueueRetrievalFailed   = errors.NewValidationError("validation", "failed to get moderation queue")
)

// Core moderation errors
var (
	// Content moderation process errors
	ErrPatternModerationFailed     = errors.PatternAnalysisFailed(stdErrors.New("pattern moderation failed"))
	ErrAIAnalysisFailed            = errors.ProcessingFailed("AI analysis", stdErrors.New("AI analysis failed"))
	ErrTextAnalysisFailed          = errors.ProcessingFailed("text analysis", stdErrors.New("text analysis failed"))
	ErrImageAnalysisFailed         = errors.ProcessingFailed("image analysis", stdErrors.New("image analysis failed"))
	ErrModerationDecisionFailed    = errors.ProcessingFailed("moderation decision", stdErrors.New("moderation decision failed"))
	ErrModerationSystemUnavailable = errors.ServiceUnavailable("moderation system")

	// Content assessment errors
	ErrContentViolatesPolicy      = errors.ContentNotAllowed("unknown", "policy violation")
	ErrInsufficientModerationData = errors.NewValidationError("moderation_data", "insufficient")
	ErrModerationRuleNotFound     = errors.NewAppError(errors.CodeNotFound, errors.CategoryBusiness, "moderation rule not found")

	// Storage and persistence errors
	// Moderation Decision storage errors
	ErrFailedToUpdateModerationDecision = errors.FailedToUpdate("moderation decision", stdErrors.New("failed to update moderation decision"))
	ErrFailedToStoreModerationDecision  = errors.FailedToStore("moderation decision", stdErrors.New("failed to store moderation decision"))
	ErrFailedToRetrieveModerationQueue  = errors.FailedToGet("moderation queue", stdErrors.New("failed to retrieve moderation queue"))
)

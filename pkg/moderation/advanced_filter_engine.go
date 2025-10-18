package moderation

import (
	"context"
	"regexp"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"go.uber.org/zap"
)

// Filter match mode constants
const (
	MatchModeKeyword = "keyword"
)

// FilterRepository defines the interface for accessing filter data
type FilterRepository interface {
	GetFilterKeywords(ctx context.Context, filterID string) ([]*models.FilterKeyword, error)
}

// AdvancedFilterEngine provides enhanced content filtering capabilities
type AdvancedFilterEngine struct {
	logger          *zap.Logger
	filterRepo      FilterRepository
	compiledRegexes map[string]*regexp.Regexp
	semanticMatcher *SemanticMatcher
}

// NewAdvancedFilterEngine creates a new advanced filter engine
func NewAdvancedFilterEngine(logger *zap.Logger) *AdvancedFilterEngine {
	return &AdvancedFilterEngine{
		logger:          logger,
		compiledRegexes: make(map[string]*regexp.Regexp),
		semanticMatcher: NewSemanticMatcher(),
	}
}

// SetFilterRepository sets the filter repository for accessing filter data
func (afe *AdvancedFilterEngine) SetFilterRepository(repo FilterRepository) {
	afe.filterRepo = repo
}

// FilterResult represents the result of filter evaluation
type FilterResult struct {
	Matched      bool                    `json:"matched"`
	Action       string                  `json:"action"`
	Severity     string                  `json:"severity"`
	MatchScore   float64                 `json:"match_score"`
	MatchedRules []string                `json:"matched_rules"`
	Filter       *models.Filter          `json:"filter,omitempty"`
	Keywords     []*models.FilterKeyword `json:"keywords,omitempty"`
}

// ContentContext represents the context in which content is being filtered
type ContentContext struct {
	Type       string    `json:"type"`       // home, notifications, public, thread, account
	AuthorID   string    `json:"author_id"`  // ID of content author
	Timestamp  time.Time `json:"timestamp"`  // When content was created
	IsReply    bool      `json:"is_reply"`   // Whether content is a reply
	HasMedia   bool      `json:"has_media"`  // Whether content has media attachments
	Language   string    `json:"language"`   // Content language
	Visibility string    `json:"visibility"` // public, unlisted, private, direct
}

// EvaluateContent evaluates content against user filters with advanced matching
func (afe *AdvancedFilterEngine) EvaluateContent(
	ctx context.Context,
	content string,
	filters []*models.Filter,
	contentCtx *ContentContext,
) ([]*FilterResult, error) {
	var results []*FilterResult

	for _, filter := range filters {
		if !afe.isFilterApplicable(filter, contentCtx) {
			continue
		}

		if afe.isFilterExpired(filter) {
			continue
		}

		result, err := afe.evaluateFilterAgainstContent(ctx, content, filter, contentCtx)
		if err != nil {
			afe.logger.Error("failed to evaluate filter",
				zap.String("filter_id", filter.ID),
				zap.Error(err))
			continue
		}

		if result != nil && result.Matched {
			results = append(results, result)
		}
	}

	return results, nil
}

// evaluateFilterAgainstContent evaluates a single filter against content
func (afe *AdvancedFilterEngine) evaluateFilterAgainstContent(
	ctx context.Context,
	content string,
	filter *models.Filter,
	_ *ContentContext,
) (*FilterResult, error) {
	result := &FilterResult{
		Action:       filter.FilterAction,
		Severity:     filter.Severity,
		Filter:       filter,
		MatchedRules: []string{},
	}

	// Apply different matching strategies based on filter match mode
	switch filter.MatchMode {
	case string(URLPatternRegex):
		return afe.evaluateRegexFilter(ctx, content, filter, result)
	case "semantic":
		return afe.evaluateSemanticFilter(content, filter, result)
	case "exact":
		return afe.evaluateExactFilter(ctx, content, filter, result)
	case MatchModeKeyword:
		fallthrough
	default:
		return afe.evaluateKeywordFilter(ctx, content, filter, result)
	}
}

// evaluateKeywordFilter evaluates keyword-based filtering with enhanced matching
func (afe *AdvancedFilterEngine) evaluateKeywordFilter(
	ctx context.Context,
	content string,
	filter *models.Filter,
	result *FilterResult,
) (*FilterResult, error) {
	contentToMatch := afe.prepareContentForMatching(content, filter.CaseSensitive)
	var totalScore float64
	var matchCount int

	// Process repository keywords if available
	if afe.filterRepo != nil {
		score, count, err := afe.processRepositoryKeywords(ctx, contentToMatch, filter, result)
		if err != nil {
			afe.logger.Warn("failed to process repository keywords",
				zap.String("filter_id", filter.ID),
				zap.Error(err))
		} else {
			totalScore += score
			matchCount += count
		}
	}

	// Fallback to hardcoded logic when repository is not available
	if afe.filterRepo == nil {
		score, count := afe.processFallbackKeywords(contentToMatch, result)
		totalScore += score
		matchCount += count
	}

	afe.updateResultScore(result, totalScore, matchCount)
	return result, nil
}

// prepareContentForMatching normalizes content based on case sensitivity setting
func (afe *AdvancedFilterEngine) prepareContentForMatching(content string, caseSensitive bool) string {
	if caseSensitive {
		return content
	}
	return strings.ToLower(content)
}

// processRepositoryKeywords processes keywords from the repository
func (afe *AdvancedFilterEngine) processRepositoryKeywords(
	ctx context.Context,
	contentToMatch string,
	filter *models.Filter,
	result *FilterResult,
) (float64, int, error) {
	keywords, err := afe.filterRepo.GetFilterKeywords(ctx, filter.ID)
	if err != nil {
		return 0, 0, err
	}

	var totalScore float64
	var matchCount int

	for _, keyword := range keywords {
		if keyword.IsRegex {
			continue // Skip regex keywords, handled elsewhere
		}

		if afe.matchesKeyword(contentToMatch, keyword, filter.CaseSensitive) {
			matchCount++
			totalScore += 0.8 // High confidence match
			result.MatchedRules = append(result.MatchedRules, "keyword:"+keyword.Keyword)
		}
	}

	return totalScore, matchCount, nil
}

// matchesKeyword checks if content matches a specific keyword
func (afe *AdvancedFilterEngine) matchesKeyword(contentToMatch string, keyword *models.FilterKeyword, caseSensitive bool) bool {
	keywordToMatch := keyword.Keyword
	if !caseSensitive {
		keywordToMatch = strings.ToLower(keywordToMatch)
	}

	if keyword.WholeWord {
		return afe.matchesWholeWord(contentToMatch, keywordToMatch)
	}
	return strings.Contains(contentToMatch, keywordToMatch)
}

// matchesWholeWord performs word boundary matching
func (afe *AdvancedFilterEngine) matchesWholeWord(content, keyword string) bool {
	words := strings.Fields(content)
	for _, word := range words {
		if word == keyword {
			return true
		}
	}
	return false
}

// processFallbackKeywords processes keywords when repository is not available
func (afe *AdvancedFilterEngine) processFallbackKeywords(contentToMatch string, result *FilterResult) (float64, int) {
	var totalScore float64
	var matchCount int

	words := strings.Fields(contentToMatch)
	for _, word := range words {
		if afe.isOffensiveWord(word) {
			matchCount++
			totalScore += 0.8 // High confidence match
			result.MatchedRules = append(result.MatchedRules, "offensive_word:"+word)
		}
	}

	return totalScore, matchCount
}

// updateResultScore updates the result with calculated scores
func (afe *AdvancedFilterEngine) updateResultScore(result *FilterResult, totalScore float64, matchCount int) {
	if matchCount > 0 {
		result.Matched = true
		result.MatchScore = totalScore / float64(matchCount)
	}
}

// evaluateRegexFilter evaluates regex-based filtering
func (afe *AdvancedFilterEngine) evaluateRegexFilter(
	ctx context.Context,
	content string,
	filter *models.Filter,
	result *FilterResult,
) (*FilterResult, error) {
	// Integrate with regex patterns from FilterKeyword model
	if afe.filterRepo != nil {
		keywords, err := afe.filterRepo.GetFilterKeywords(ctx, filter.ID)
		if err != nil {
			afe.logger.Warn("failed to get filter keywords for regex",
				zap.String("filter_id", filter.ID),
				zap.Error(err))
		} else {
			// Process regex keywords from repository
			for _, keyword := range keywords {
				if !keyword.IsRegex {
					// Skip non-regex keywords, they're handled in evaluateKeywordFilter
					continue
				}

				regex, err := afe.getCompiledRegex(keyword.Keyword)
				if err != nil {
					afe.logger.Warn("failed to compile regex pattern",
						zap.String("pattern", keyword.Keyword),
						zap.String("keyword_id", keyword.ID),
						zap.Error(err))
					continue
				}

				if regex.MatchString(content) {
					result.Matched = true
					result.MatchScore = 0.9
					result.MatchedRules = append(result.MatchedRules, "regex:"+keyword.Keyword)
				}
			}
		}
	}

	// Fallback: placeholder regex patterns when repository is not available
	if afe.filterRepo == nil {
		patterns := []string{
			`\b(spam|scam)\b`,
			`\d{4}[-\s]?\d{4}[-\s]?\d{4}[-\s]?\d{4}`, // Credit card pattern
		}

		for _, pattern := range patterns {
			regex, err := afe.getCompiledRegex(pattern)
			if err != nil {
				continue
			}

			if regex.MatchString(content) {
				result.Matched = true
				result.MatchScore = 0.9
				result.MatchedRules = append(result.MatchedRules, "regex:"+pattern)
			}
		}
	}

	return result, nil
}

// evaluateSemanticFilter evaluates semantic/AI-based filtering
func (afe *AdvancedFilterEngine) evaluateSemanticFilter(
	content string,
	_ *models.Filter,
	result *FilterResult,
) (*FilterResult, error) {
	if afe.semanticMatcher == nil {
		return result, nil
	}

	score, categories := afe.semanticMatcher.AnalyzeContent(content)

	if score > 0.7 { // High confidence threshold
		result.Matched = true
		result.MatchScore = score
		result.MatchedRules = append(result.MatchedRules, "semantic:"+strings.Join(categories, ","))
	}

	return result, nil
}

// evaluateExactFilter evaluates exact string matching
func (afe *AdvancedFilterEngine) evaluateExactFilter(
	ctx context.Context,
	content string,
	filter *models.Filter,
	result *FilterResult,
) (*FilterResult, error) {
	contentToMatch := afe.prepareContentForMatching(content, filter.CaseSensitive)

	// Process repository keywords if available
	if afe.filterRepo != nil {
		err := afe.processExactRepositoryKeywords(ctx, contentToMatch, filter, result)
		if err != nil {
			afe.logger.Warn("failed to process exact match keywords",
				zap.String("filter_id", filter.ID),
				zap.Error(err))
		}
	}

	// Fallback to hardcoded phrases when repository is not available
	if afe.filterRepo == nil {
		afe.processExactFallbackPhrases(contentToMatch, filter, result)
	}

	return result, nil
}

// processExactRepositoryKeywords processes exact match keywords from repository
func (afe *AdvancedFilterEngine) processExactRepositoryKeywords(
	ctx context.Context,
	contentToMatch string,
	filter *models.Filter,
	result *FilterResult,
) error {
	keywords, err := afe.filterRepo.GetFilterKeywords(ctx, filter.ID)
	if err != nil {
		return err
	}

	for _, keyword := range keywords {
		if keyword.IsRegex {
			continue // Skip regex keywords
		}

		if afe.matchesExactKeyword(contentToMatch, keyword, filter.CaseSensitive) {
			result.Matched = true
			result.MatchScore = 1.0 // Exact match
			result.MatchedRules = append(result.MatchedRules, "exact:"+keyword.Keyword)
		}
	}

	return nil
}

// matchesExactKeyword checks if content matches exactly
func (afe *AdvancedFilterEngine) matchesExactKeyword(contentToMatch string, keyword *models.FilterKeyword, caseSensitive bool) bool {
	keywordToMatch := keyword.Keyword
	if !caseSensitive {
		keywordToMatch = strings.ToLower(keywordToMatch)
	}

	if keyword.WholeWord {
		return afe.matchesWholeWord(contentToMatch, keywordToMatch)
	}
	return strings.Contains(contentToMatch, keywordToMatch)
}

// processExactFallbackPhrases processes hardcoded exact match phrases
func (afe *AdvancedFilterEngine) processExactFallbackPhrases(contentToMatch string, filter *models.Filter, result *FilterResult) {
	blockedPhrases := []string{
		"click here now",
		"limited time offer",
		"act now",
	}

	for _, phrase := range blockedPhrases {
		searchPhrase := phrase
		if !filter.CaseSensitive {
			searchPhrase = strings.ToLower(phrase)
		}

		if strings.Contains(contentToMatch, searchPhrase) {
			result.Matched = true
			result.MatchScore = 1.0 // Exact match
			result.MatchedRules = append(result.MatchedRules, "exact:"+phrase)
		}
	}
}

// Helper methods

func (afe *AdvancedFilterEngine) isFilterApplicable(filter *models.Filter, contentCtx *ContentContext) bool {
	if err := common.ValidateSliceNotEmpty("filter.Context", filter.Context); err != nil {
		return true // Apply to all contexts if none specified
	}

	for _, ctx := range filter.Context {
		if ctx == contentCtx.Type {
			return true
		}
	}

	return false
}

func (afe *AdvancedFilterEngine) isFilterExpired(filter *models.Filter) bool {
	if filter.ExpiresAt == nil {
		return false
	}
	return time.Now().After(*filter.ExpiresAt)
}

func (afe *AdvancedFilterEngine) getCompiledRegex(pattern string) (*regexp.Regexp, error) {
	if regex, exists := afe.compiledRegexes[pattern]; exists {
		return regex, nil
	}

	regex, err := regexp.Compile(pattern)
	if err != nil {
		return nil, err
	}

	afe.compiledRegexes[pattern] = regex
	return regex, nil
}

func (afe *AdvancedFilterEngine) isOffensiveWord(word string) bool {
	// Placeholder for offensive word detection
	// In production, this would use a comprehensive word list or ML model
	offensiveWords := []string{"spam", "scam", "fake", "bot"}
	for _, offensive := range offensiveWords {
		if strings.EqualFold(word, offensive) {
			return true
		}
	}
	return false
}

// SemanticMatcher provides AI-powered semantic content analysis
type SemanticMatcher struct {
	// Placeholder for semantic matching implementation
	// Would integrate with AWS Comprehend, OpenAI, or similar service
}

// NewSemanticMatcher creates a new semantic matcher
func NewSemanticMatcher() *SemanticMatcher {
	return &SemanticMatcher{}
}

// AnalyzeContent analyzes content for semantic patterns
func (sm *SemanticMatcher) AnalyzeContent(content string) (score float64, categories []string) {
	// Placeholder implementation
	// In production, this would use ML models for content classification

	// Simple heuristics for demonstration
	content = strings.ToLower(content)

	if strings.Contains(content, "hate") || strings.Contains(content, "violence") {
		return 0.9, []string{"hate_speech"}
	}

	if strings.Contains(content, "buy now") || strings.Contains(content, "limited offer") {
		return 0.8, []string{"spam", "commercial"}
	}

	if strings.Contains(content, "click here") || strings.Contains(content, "free money") {
		return 0.85, []string{"spam", "suspicious"}
	}

	return 0.1, []string{"normal"}
}

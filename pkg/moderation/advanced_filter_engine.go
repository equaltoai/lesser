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

// AdvancedFilterEngine provides enhanced content filtering capabilities
type AdvancedFilterEngine struct {
	logger          *zap.Logger
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
	_ context.Context,
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
		return afe.evaluateRegexFilter(content, filter, result)
	case "semantic":
		return afe.evaluateSemanticFilter(content, filter, result)
	case "exact":
		return afe.evaluateExactFilter(content, filter, result)
	case MatchModeKeyword:
		fallthrough
	default:
		return afe.evaluateKeywordFilter(content, filter, result)
	}
}

// evaluateKeywordFilter evaluates keyword-based filtering with enhanced matching
func (afe *AdvancedFilterEngine) evaluateKeywordFilter(
	content string,
	filter *models.Filter,
	result *FilterResult,
) (*FilterResult, error) {
	contentToMatch := content
	if !filter.CaseSensitive {
		contentToMatch = strings.ToLower(content)
	}

	var totalScore float64
	var matchCount int

	// TODO: Integrate with keyword repository to get filter keywords
	// For now, simulate keyword evaluation logic

	// Placeholder for keyword matching logic
	// This would integrate with the FilterKeyword model and repository
	words := strings.Fields(contentToMatch)
	for _, word := range words {
		if afe.isOffensiveWord(word) {
			matchCount++
			totalScore += 0.8 // High confidence match
			result.MatchedRules = append(result.MatchedRules, "offensive_word:"+word)
		}
	}

	if matchCount > 0 {
		result.Matched = true
		result.MatchScore = totalScore / float64(matchCount)
	}

	return result, nil
}

// evaluateRegexFilter evaluates regex-based filtering
func (afe *AdvancedFilterEngine) evaluateRegexFilter(
	content string,
	_ *models.Filter,
	result *FilterResult,
) (*FilterResult, error) {
	// TODO: Integrate with regex patterns from FilterKeyword model
	// This would compile and cache regex patterns from keywords marked as IsRegex

	// Placeholder regex patterns for demonstration
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
	content string,
	filter *models.Filter,
	result *FilterResult,
) (*FilterResult, error) {
	// TODO: Integrate with exact match patterns from FilterKeyword model

	// Placeholder exact matches
	blockedPhrases := []string{
		"click here now",
		"limited time offer",
		"act now",
	}

	contentToMatch := content
	if !filter.CaseSensitive {
		contentToMatch = strings.ToLower(content)
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

	return result, nil
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

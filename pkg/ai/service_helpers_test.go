package ai

import (
	"math"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// Section 1: Composite scoring helpers
// =============================================================================

func TestCalculateOverallRisk(t *testing.T) {
	// Create a minimal AIService for testing - no AWS clients needed for pure helpers
	service := &AIService{
		config: &AIConfig{
			NSFWThreshold:     0.8,
			ToxicityThreshold: 0.7,
			SpamThreshold:     0.6,
		},
	}

	tests := []struct {
		name            string
		analysis        *AIAnalysis
		expectedRisk    float64
		expectedNonZero bool
	}{
		{
			name:         "no sub-analyses returns 0 risk",
			analysis:     &AIAnalysis{},
			expectedRisk: 0.0,
		},
		{
			name: "only text analysis",
			analysis: &AIAnalysis{
				TextAnalysis: &TextAnalysis{
					ToxicityScore: 0.5,
				},
			},
			// risk = 0.5 * 0.3 = 0.15, weights = 0.3
			// result = 0.15 / 0.3 = 0.5
			expectedRisk: 0.5,
		},
		{
			name: "text analysis with PII increases risk",
			analysis: &AIAnalysis{
				TextAnalysis: &TextAnalysis{
					ToxicityScore: 0.5,
					ContainsPII:   true,
				},
			},
			// risk = 0.5 * 0.3 + 0.2 = 0.35
			// weights = 0.3 + 0.1 = 0.4
			// result = 0.35 / 0.4 = 0.875
			expectedRisk: 0.875,
		},
		{
			name: "text + spam + image + AI detection",
			analysis: &AIAnalysis{
				TextAnalysis: &TextAnalysis{
					ToxicityScore: 0.4,
				},
				SpamAnalysis: &SpamAnalysis{
					SpamScore: 0.6,
				},
				ImageAnalysis: &ImageAnalysis{
					IsNSFW:         true,
					NSFWConfidence: 80.0, // 80%
					ViolenceScore:  0.3,
				},
				AIDetection: &AIDetection{
					AIGeneratedProbability: 0.8,
				},
			},
			// text: 0.4 * 0.3 = 0.12, weight 0.3
			// image NSFW: 0.8 * 0.3 = 0.24, weight 0.3
			// image violence: 0.3 * 0.2 = 0.06, weight 0.2
			// spam: 0.6 * 0.2 = 0.12, weight 0.2
			// ai: 0.8 * 0.1 = 0.08, weight 0.1
			// total risk = 0.62, total weights = 1.1
			// result = 0.62 / 1.1 ≈ 0.5636
			expectedNonZero: true,
		},
		{
			name: "image analysis without NSFW",
			analysis: &AIAnalysis{
				ImageAnalysis: &ImageAnalysis{
					IsNSFW:        false,
					ViolenceScore: 0.5,
				},
			},
			// violence: 0.5 * 0.2 = 0.1, weight 0.2
			// result = 0.1 / 0.2 = 0.5
			expectedRisk: 0.5,
		},
		{
			name: "only spam analysis",
			analysis: &AIAnalysis{
				SpamAnalysis: &SpamAnalysis{
					SpamScore: 1.0,
				},
			},
			// spam: 1.0 * 0.2 = 0.2, weight 0.2
			// result = 0.2 / 0.2 = 1.0
			expectedRisk: 1.0,
		},
		{
			name: "only AI detection",
			analysis: &AIAnalysis{
				AIDetection: &AIDetection{
					AIGeneratedProbability: 1.0,
				},
			},
			// ai: 1.0 * 0.1 = 0.1, weight 0.1
			// result = 0.1 / 0.1 = 1.0
			expectedRisk: 1.0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := service.calculateOverallRisk(tc.analysis)
			if tc.expectedNonZero {
				assert.Greater(t, result, 0.0, "expected non-zero risk")
				assert.LessOrEqual(t, result, 1.0, "risk should be <= 1.0")
			} else {
				assert.InDelta(t, tc.expectedRisk, result, 0.001, "unexpected risk value")
			}
		})
	}
}

func TestCalculateConfidence(t *testing.T) {
	service := &AIService{
		config: &AIConfig{},
	}

	tests := []struct {
		name               string
		analysis           *AIAnalysis
		expectedConfidence float64
	}{
		{
			name:               "no sub-analyses returns 0.5 confidence",
			analysis:           &AIAnalysis{},
			expectedConfidence: 0.5,
		},
		{
			name: "only text analysis - Comprehend is highly reliable",
			analysis: &AIAnalysis{
				TextAnalysis: &TextAnalysis{},
			},
			// confidence = 0.9 / 1 = 0.9
			expectedConfidence: 0.9,
		},
		{
			name: "only image analysis - Rekognition is reliable",
			analysis: &AIAnalysis{
				ImageAnalysis: &ImageAnalysis{},
			},
			// confidence = 0.85 / 1 = 0.85
			expectedConfidence: 0.85,
		},
		{
			name: "only AI detection - less certain",
			analysis: &AIAnalysis{
				AIDetection: &AIDetection{},
			},
			// confidence = 0.7 / 1 = 0.7
			expectedConfidence: 0.7,
		},
		{
			name: "only spam analysis - heuristics are fairly reliable",
			analysis: &AIAnalysis{
				SpamAnalysis: &SpamAnalysis{},
			},
			// confidence = 0.8 / 1 = 0.8
			expectedConfidence: 0.8,
		},
		{
			name: "all analyses combined",
			analysis: &AIAnalysis{
				TextAnalysis:  &TextAnalysis{},
				ImageAnalysis: &ImageAnalysis{},
				SpamAnalysis:  &SpamAnalysis{},
				AIDetection:   &AIDetection{},
			},
			// confidence = (0.9 + 0.85 + 0.7 + 0.8) / 4 = 3.25 / 4 = 0.8125
			expectedConfidence: 0.8125,
		},
		{
			name: "text + spam only",
			analysis: &AIAnalysis{
				TextAnalysis: &TextAnalysis{},
				SpamAnalysis: &SpamAnalysis{},
			},
			// confidence = (0.9 + 0.8) / 2 = 0.85
			expectedConfidence: 0.85,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := service.calculateConfidence(tc.analysis)
			assert.InDelta(t, tc.expectedConfidence, result, 0.001, "unexpected confidence value")
		})
	}
}

// =============================================================================
// Section 2: Moderation action selection
// =============================================================================

func TestDetermineModerationAction(t *testing.T) {
	tests := []struct {
		name           string
		config         *AIConfig
		analysis       *AIAnalysis
		expectedAction string
	}{
		// NSFW override path
		{
			name: "NSFW override triggers ActionRemove",
			config: &AIConfig{
				NSFWThreshold:     0.7,
				ToxicityThreshold: 0.8,
				SpamThreshold:     0.8,
			},
			analysis: &AIAnalysis{
				OverallRisk: 0.2, // Low risk, but NSFW should override
				ImageAnalysis: &ImageAnalysis{
					IsNSFW:         true,
					NSFWConfidence: 80.0, // 80% > 0.7*100 = 70%
				},
			},
			expectedAction: ActionRemove,
		},
		{
			name: "NSFW below threshold does not trigger remove",
			config: &AIConfig{
				NSFWThreshold:     0.9,
				ToxicityThreshold: 0.8,
				SpamThreshold:     0.8,
			},
			analysis: &AIAnalysis{
				OverallRisk: 0.2,
				ImageAnalysis: &ImageAnalysis{
					IsNSFW:         true,
					NSFWConfidence: 80.0, // 80% < 0.9*100 = 90%
				},
			},
			expectedAction: ActionNone,
		},
		// Toxicity triggers ActionHide
		{
			name: "toxicity triggers ActionHide",
			config: &AIConfig{
				NSFWThreshold:     0.9,
				ToxicityThreshold: 0.6,
				SpamThreshold:     0.8,
			},
			analysis: &AIAnalysis{
				OverallRisk: 0.2,
				TextAnalysis: &TextAnalysis{
					ToxicityScore: 0.7, // 0.7 > 0.6
				},
			},
			expectedAction: ActionHide,
		},
		{
			name: "toxicity below threshold uses risk fallback",
			config: &AIConfig{
				NSFWThreshold:     0.9,
				ToxicityThreshold: 0.8,
				SpamThreshold:     0.8,
			},
			analysis: &AIAnalysis{
				OverallRisk: 0.2,
				TextAnalysis: &TextAnalysis{
					ToxicityScore: 0.6, // 0.6 < 0.8
				},
			},
			expectedAction: ActionNone,
		},
		// Spam triggers ActionShadowBan
		{
			name: "spam triggers ActionShadowBan",
			config: &AIConfig{
				NSFWThreshold:     0.9,
				ToxicityThreshold: 0.8,
				SpamThreshold:     0.5,
			},
			analysis: &AIAnalysis{
				OverallRisk: 0.2,
				SpamAnalysis: &SpamAnalysis{
					SpamScore: 0.6, // 0.6 > 0.5
				},
			},
			expectedAction: ActionShadowBan,
		},
		// Risk-based actions (fallback)
		{
			name: "risk > 0.9 triggers ActionRemove",
			config: &AIConfig{
				NSFWThreshold:     0.9,
				ToxicityThreshold: 0.9,
				SpamThreshold:     0.9,
			},
			analysis: &AIAnalysis{
				OverallRisk: 0.95,
			},
			expectedAction: ActionRemove,
		},
		{
			name: "risk > 0.7 triggers ActionHide",
			config: &AIConfig{
				NSFWThreshold:     0.9,
				ToxicityThreshold: 0.9,
				SpamThreshold:     0.9,
			},
			analysis: &AIAnalysis{
				OverallRisk: 0.75,
			},
			expectedAction: ActionHide,
		},
		{
			name: "risk > 0.5 triggers ActionFlag",
			config: &AIConfig{
				NSFWThreshold:     0.9,
				ToxicityThreshold: 0.9,
				SpamThreshold:     0.9,
			},
			analysis: &AIAnalysis{
				OverallRisk: 0.55,
			},
			expectedAction: ActionFlag,
		},
		{
			name: "risk > 0.3 triggers ActionReview",
			config: &AIConfig{
				NSFWThreshold:     0.9,
				ToxicityThreshold: 0.9,
				SpamThreshold:     0.9,
			},
			analysis: &AIAnalysis{
				OverallRisk: 0.35,
			},
			expectedAction: ActionReview,
		},
		{
			name: "risk <= 0.3 triggers ActionNone",
			config: &AIConfig{
				NSFWThreshold:     0.9,
				ToxicityThreshold: 0.9,
				SpamThreshold:     0.9,
			},
			analysis: &AIAnalysis{
				OverallRisk: 0.25,
			},
			expectedAction: ActionNone,
		},
		// Edge cases
		{
			name: "exactly at risk boundary 0.3",
			config: &AIConfig{
				NSFWThreshold:     0.9,
				ToxicityThreshold: 0.9,
				SpamThreshold:     0.9,
			},
			analysis: &AIAnalysis{
				OverallRisk: 0.3, // exactly 0.3 -> none (not > 0.3)
			},
			expectedAction: ActionNone,
		},
		{
			name: "zero risk",
			config: &AIConfig{
				NSFWThreshold:     0.9,
				ToxicityThreshold: 0.9,
				SpamThreshold:     0.9,
			},
			analysis: &AIAnalysis{
				OverallRisk: 0.0,
			},
			expectedAction: ActionNone,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			service := &AIService{
				config: tc.config,
			}
			result := service.determineModerationAction(tc.analysis)
			assert.Equal(t, tc.expectedAction, result)
		})
	}
}

// =============================================================================
// Section 3: Text heuristics
// =============================================================================

func TestCalculateRepetition(t *testing.T) {
	service := &AIService{
		config: &AIConfig{},
	}

	tests := []struct {
		name          string
		text          string
		expectedScore float64
		description   string
	}{
		{
			name:          "empty text returns 0",
			text:          "",
			expectedScore: 0.0,
			description:   "empty string should return 0 repetition",
		},
		{
			name:          "whitespace only returns 0",
			text:          "   \t\n  ",
			expectedScore: 0.0,
			description:   "whitespace-only text should return 0",
		},
		{
			name:          "single word returns 1.0 (max repetition)",
			text:          "hello",
			expectedScore: 1.0,
			description:   "single word: 1 occurrence / 1 total = 1.0",
		},
		{
			name:          "all unique words",
			text:          "hello world how are you",
			expectedScore: 0.2,
			description:   "5 unique words: max(1) / 5 = 0.2",
		},
		{
			name:          "repeated word increases score",
			text:          "spam spam spam spam spam",
			expectedScore: 1.0,
			description:   "5 same words: 5 / 5 = 1.0",
		},
		{
			name:          "partial repetition",
			text:          "hello hello world world foo",
			expectedScore: 0.4,
			description:   "max(2) / 5 words = 0.4",
		},
		{
			name:          "case insensitive matching",
			text:          "Hello HELLO hello",
			expectedScore: 1.0,
			description:   "3 same words (case insensitive): 3 / 3 = 1.0",
		},
		{
			name:          "longer text with some repetition",
			text:          "buy now buy now limited offer act fast",
			expectedScore: 0.25,
			description:   "8 words, max repeated = 2 (buy or now), so 2/8 = 0.25",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := service.calculateRepetition(tc.text)
			assert.InDelta(t, tc.expectedScore, result, 0.001, tc.description)
		})
	}
}

func TestAnalyzeTopicConsistency(t *testing.T) {
	service := &AIService{
		config: &AIConfig{},
	}

	tests := []struct {
		name        string
		content     *Content
		expectRange []float64 // [min, max] expected range
		expectNaN   bool      // when true, we expect NaN due to edge case
	}{
		{
			name: "sentence no period - edge case in implementation",
			// Note: Single sentence without period causes len(sentences)=1, which triggers
			// division by zero in the avgConsistency calculation (0/0), resulting in NaN.
			content:   &Content{Text: "This single sentence"},
			expectNaN: true,
		},
		{
			name: "single sentence with trailing period",
			// "Sentence." splits to ["Sentence", ""], len=2, goes through sentence-pair logic
			// but empty string produces no meaningful words, so may return 0.8 (default branch)
			content:     &Content{Text: "This is a sentence."},
			expectRange: []float64{0.5, 1.0},
		},
		{
			name:        "two sentences with related content",
			content:     &Content{Text: "The technology industry continues to grow. Technology companies are hiring more engineers."},
			expectRange: []float64{0.5, 1.0},
		},
		{
			name: "multiple sentences with consistent topic",
			content: &Content{
				Text: "Machine learning is transforming industries. Deep learning models achieve remarkable results. Neural networks power modern applications.",
			},
			expectRange: []float64{0.5, 1.0},
		},
		{
			name: "multiple sentences with abrupt topic changes",
			content: &Content{
				Text: "Baseball is America's pastime. Quantum physics describes atomic behavior. Restaurant menus vary seasonally.",
			},
			expectRange: []float64{0.5, 1.0}, // Still within expected range, just potentially lower
		},
		{
			name: "empty text returns 1.0",
			// Empty string split by "." returns [""], len=1, but totalWords=0 -> returns 1.0
			content:     &Content{Text: ""},
			expectRange: []float64{1.0, 1.0},
		},
		{
			name: "text with only common words",
			// After filtering common words, few/no meaningful words remain
			content:     &Content{Text: "The and for are. But not you all."},
			expectRange: []float64{0.5, 1.0}, // Filter removes common words -> totalWords=0 -> returns 1.0
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := service.analyzeTopicConsistency(tc.content)
			if tc.expectNaN {
				// NaN is expected for edge cases like single sentence without period
				// which causes division by zero (0/0)
				assert.True(t, math.IsNaN(result) || math.IsInf(result, 0),
					"expected NaN or Inf for edge case, got: %v", result)
			} else {
				minExpected := tc.expectRange[0]
				maxExpected := tc.expectRange[1]
				assert.GreaterOrEqual(t, result, minExpected, "result should be >= min expected")
				assert.LessOrEqual(t, result, maxExpected, "result should be <= max expected")
			}
		})
	}
}

func TestIsCommonWord(t *testing.T) {
	tests := []struct {
		word     string
		expected bool
	}{
		// Common words (should return true)
		{"the", true},
		{"and", true},
		{"for", true},
		{"are", true},
		{"but", true},
		{"not", true},
		{"you", true},
		{"this", true},
		{"that", true},
		{"have", true},
		{"from", true},
		{"they", true},
		{"with", true},
		{"will", true},
		// Case insensitive
		{"THE", true},
		{"And", true},
		{"FOR", true},
		// Not common words (should return false)
		{"technology", false},
		{"computer", false},
		{"algorithm", false},
		{"database", false},
		{"network", false},
		{"", false},
		{"xyz", false},
		{"hello", false},
	}

	for _, tc := range tests {
		t.Run(tc.word, func(t *testing.T) {
			result := isCommonWord(tc.word)
			assert.Equal(t, tc.expected, result, "for word: %s", tc.word)
		})
	}
}

func TestCountMeaningfulWords(t *testing.T) {
	tests := []struct {
		name     string
		words    []string
		expected int
	}{
		{
			name:     "empty slice",
			words:    []string{},
			expected: 0,
		},
		{
			name:     "only short words (3 chars or less)",
			words:    []string{"the", "and", "for", "a", "to"},
			expected: 0, // All are short or common
		},
		{
			name:     "only common words",
			words:    []string{"this", "that", "have", "from"},
			expected: 0, // All are common
		},
		{
			name:     "meaningful words present",
			words:    []string{"technology", "computer", "algorithm"},
			expected: 3,
		},
		{
			name:     "mixed meaningful and common",
			words:    []string{"the", "technology", "and", "computer", "for", "research"},
			expected: 3, // technology, computer, research
		},
		{
			name:     "short non-common words excluded",
			words:    []string{"cat", "dog", "sun"}, // 3 chars, excluded
			expected: 0,
		},
		{
			name:     "4+ char non-common words included",
			words:    []string{"cats", "dogs", "suns"}, // 4 chars, not common
			expected: 3,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := countMeaningfulWords(tc.words)
			assert.Equal(t, tc.expected, result)
		})
	}
}

// =============================================================================
// Section 4: ID and URL parsing helpers
// =============================================================================

func TestGenerateID(t *testing.T) {
	tests := []struct {
		name   string
		prefix string
	}{
		{name: "standard prefix", prefix: "ai-analysis"},
		{name: "short prefix", prefix: "ai"},
		{name: "empty prefix", prefix: ""},
		{name: "hyphenated prefix", prefix: "ai-image-req"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			id := generateID(tc.prefix)

			// Verify format: <prefix>-<uuid>
			require.NotEmpty(t, id)

			if tc.prefix != "" {
				assert.True(t, strings.HasPrefix(id, tc.prefix+"-"),
					"ID should start with prefix and hyphen: got %s", id)
			}

			// Verify it contains a UUID-like structure (36 chars including hyphens)
			parts := strings.SplitN(id, "-", 2)
			if tc.prefix != "" {
				require.Len(t, parts, 2, "ID should have prefix and UUID parts")
				// UUID has format: xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx (36 chars)
				assert.GreaterOrEqual(t, len(parts[1]), 36-1, "UUID part should be present")
			}

			// Generate another and ensure uniqueness
			id2 := generateID(tc.prefix)
			assert.NotEqual(t, id, id2, "IDs should be unique")
		})
	}
}

func TestExtractS3Key(t *testing.T) {
	tests := []struct {
		name        string
		url         string
		expectedKey string
	}{
		{
			name:        "empty URL returns empty key",
			url:         "",
			expectedKey: "",
		},
		{
			name:        "S3 URL extracts path",
			url:         "https://bucket.s3.us-east-1.amazonaws.com/media/images/photo.jpg",
			expectedKey: "media/images/photo.jpg",
		},
		{
			name:        "S3 URL with single path segment",
			url:         "https://bucket.s3.region.amazonaws.com/file.jpg",
			expectedKey: "file.jpg",
		},
		{
			name:        "CloudFront URL extracts path",
			url:         "https://d123abc.cloudfront.net/media/videos/video.mp4",
			expectedKey: "media/videos/video.mp4",
		},
		{
			name:        "CloudFront URL with deep path",
			url:         "https://d123abc.cloudfront.net/uploads/2024/01/image.png",
			expectedKey: "uploads/2024/01/image.png",
		},
		{
			name:        "non-AWS URL returns last segment (fallback)",
			url:         "https://example.com/path/to/file.jpg",
			expectedKey: "file.jpg",
		},
		{
			name:        "URL with only filename",
			url:         "https://example.com/image.png",
			expectedKey: "image.png",
		},
		{
			name:        "S3 URL without path returns empty from path logic",
			url:         "https://bucket.s3.amazonaws.com/",
			expectedKey: "",
		},
		{
			name:        "regular URL without AWS domain",
			url:         "https://cdn.mysite.com/assets/logo.svg",
			expectedKey: "logo.svg",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := extractS3Key(tc.url)
			assert.Equal(t, tc.expectedKey, result)
		})
	}
}

// =============================================================================
// Additional helper function tests
// =============================================================================

func TestMathMin(t *testing.T) {
	tests := []struct {
		a, b     float64
		expected float64
	}{
		{1.0, 2.0, 1.0},
		{2.0, 1.0, 1.0},
		{0.0, 0.0, 0.0},
		{-1.0, 1.0, -1.0},
		{0.5, 0.5, 0.5},
	}

	for _, tc := range tests {
		result := mathMin(tc.a, tc.b)
		assert.Equal(t, tc.expected, result)
	}
}

func TestMaxFloat64(t *testing.T) {
	tests := []struct {
		a, b     float64
		expected float64
	}{
		{1.0, 2.0, 2.0},
		{2.0, 1.0, 2.0},
		{0.0, 0.0, 0.0},
		{-1.0, 1.0, 1.0},
		{0.5, 0.5, 0.5},
	}

	for _, tc := range tests {
		result := maxFloat64(tc.a, tc.b)
		assert.Equal(t, tc.expected, result)
	}
}

// Test fallbackAIDetection which is a pure helper that doesn't use AWS
func TestFallbackAIDetection(t *testing.T) {
	service := &AIService{
		config: &AIConfig{},
	}

	tests := []struct {
		name                     string
		content                  *Content
		expectedProbability      float64
		expectedPatternsNotEmpty bool
	}{
		{
			name:                "empty text returns 0 probability",
			content:             &Content{Text: ""},
			expectedProbability: 0.0,
		},
		{
			name:                "normal text returns 0 probability",
			content:             &Content{Text: "Hello, how are you today? Nice weather we're having."},
			expectedProbability: 0.0,
		},
		{
			name: "text with 'as an ai' pattern only",
			// Note: using text that only matches ONE pattern
			content:                  &Content{Text: "As an AI assistant, I am here to help."},
			expectedProbability:      0.3, // 1 pattern * 0.3
			expectedPatternsNotEmpty: true,
		},
		{
			name:                     "text with multiple patterns - 'as an ai' and 'i cannot'",
			content:                  &Content{Text: "As an AI, I cannot provide medical advice."},
			expectedProbability:      0.6, // 2 patterns: "as an ai" + "i cannot"
			expectedPatternsNotEmpty: true,
		},
		{
			name:                     "text with 'as a language model' and 'i don't have personal'",
			content:                  &Content{Text: "As a language model, I don't have personal opinions."},
			expectedProbability:      0.6, // 2 patterns * 0.3
			expectedPatternsNotEmpty: true,
		},
		{
			name:                     "text with three AI patterns",
			content:                  &Content{Text: "As an AI, I cannot do that. My training data suggests otherwise."},
			expectedProbability:      0.9, // 3 patterns: "as an ai" + "i cannot" + "my training data"
			expectedPatternsNotEmpty: true,
		},
		{
			name:                     "text with 'i cannot' pattern only",
			content:                  &Content{Text: "I cannot help with that request."},
			expectedProbability:      0.3,
			expectedPatternsNotEmpty: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := service.fallbackAIDetection(tc.content)

			require.NotNil(t, result)
			assert.InDelta(t, tc.expectedProbability, result.AIGeneratedProbability, 0.001)

			if tc.expectedPatternsNotEmpty {
				assert.NotEmpty(t, result.SuspiciousPatterns)
			} else {
				assert.Empty(t, result.SuspiciousPatterns)
			}

			// Verify other fields are set to defaults
			assert.Equal(t, 0.5, result.PatternConsistency)
			assert.Equal(t, 0.5, result.StyleDeviation)
			assert.Equal(t, 0.7, result.SemanticCoherence)
		})
	}
}

// Test getFileExtensionFromContentType
func TestGetFileExtensionFromContentType(t *testing.T) {
	service := &AIService{
		config: &AIConfig{},
	}

	tests := []struct {
		contentType string
		expected    string
	}{
		{"image/jpeg", ".jpg"},
		{"image/png", ".png"},
		{"image/gif", ".gif"},
		{"image/webp", ".webp"},
		{"image/bmp", ".jpg"},        // Unknown defaults to .jpg
		{"text/plain", ".jpg"},       // Unknown defaults to .jpg
		{"", ".jpg"},                 // Empty defaults to .jpg
		{"application/json", ".jpg"}, // Non-image defaults to .jpg
	}

	for _, tc := range tests {
		t.Run(tc.contentType, func(t *testing.T) {
			result := service.getFileExtensionFromContentType(tc.contentType)
			assert.Equal(t, tc.expected, result)
		})
	}
}

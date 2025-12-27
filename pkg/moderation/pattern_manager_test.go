package moderation

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakePatternStorage implements PatternStorage for testing
type fakePatternStorage struct {
	mu       sync.RWMutex
	patterns map[string]*ModerationPattern
}

func newFakePatternStorage() *fakePatternStorage {
	return &fakePatternStorage{
		patterns: make(map[string]*ModerationPattern),
	}
}

func (f *fakePatternStorage) CreateModerationPattern(_ context.Context, pattern *ModerationPattern) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if pattern.ID == "" {
		pattern.ID = "gen-" + pattern.Name
	}
	f.patterns[pattern.ID] = pattern
	return nil
}

func (f *fakePatternStorage) GetModerationPattern(_ context.Context, patternID string) (*ModerationPattern, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	if p, exists := f.patterns[patternID]; exists {
		return p, nil
	}
	return nil, errors.New("pattern not found")
}

func (f *fakePatternStorage) GetModerationPatterns(_ context.Context, active bool, severity string, limit int) ([]*ModerationPattern, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	var result []*ModerationPattern
	for _, p := range f.patterns {
		if p.Active != active {
			continue
		}
		if severity != "" && p.Severity != severity {
			continue
		}
		result = append(result, p)
		if limit > 0 && len(result) >= limit {
			break
		}
	}
	return result, nil
}

func (f *fakePatternStorage) UpdateModerationPattern(_ context.Context, pattern *ModerationPattern) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.patterns[pattern.ID] = pattern
	return nil
}

func (f *fakePatternStorage) UpdatePatternStats(_ context.Context, patternID string, matched bool, falsePositive bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if p, exists := f.patterns[patternID]; exists {
		if matched {
			p.MatchCount++
			if falsePositive {
				p.FalsePositiveCount++
			}
		}
		p.UpdatedAt = time.Now()
	}
	return nil
}

func (f *fakePatternStorage) RecordPatternMatch(_ context.Context, patternID string, matched bool, timestamp time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if p, exists := f.patterns[patternID]; exists && matched {
		p.MatchCount++
		p.LastMatch = timestamp
	}
	return nil
}

func TestPatternManager_NewPatternManager(t *testing.T) {
	pm := NewPatternManager()
	assert.NotNil(t, pm)
	assert.False(t, pm.enhancedEnabled)
}

func TestPatternManager_CreatePattern(t *testing.T) {
	storage := newFakePatternStorage()
	pm := &PatternManager{
		storage:         storage,
		enhancedEnabled: false,
	}
	ctx := context.Background()

	tests := []struct {
		name        string
		pattern     *ModerationPattern
		expectError bool
	}{
		{
			name: "valid keyword pattern",
			pattern: &ModerationPattern{
				Name:     "test-keyword",
				Content:  "badword",
				Type:     "keyword",
				Severity: "medium",
			},
			expectError: false,
		},
		{
			name: "valid regex pattern",
			pattern: &ModerationPattern{
				Name:     "test-regex",
				Content:  `\d{3}-\d{4}`,
				Type:     "regex",
				Severity: "high",
			},
			expectError: false,
		},
		{
			name: "valid phrase pattern",
			pattern: &ModerationPattern{
				Name:     "test-phrase",
				Content:  "bad phrase here",
				Type:     "phrase",
				Severity: "low",
			},
			expectError: false,
		},
		{
			name: "missing name fails",
			pattern: &ModerationPattern{
				Content:  "test",
				Type:     "keyword",
				Severity: "medium",
			},
			expectError: true,
		},
		{
			name: "missing content fails",
			pattern: &ModerationPattern{
				Name:     "no-content",
				Type:     "keyword",
				Severity: "medium",
			},
			expectError: true,
		},
		{
			name: "invalid type fails",
			pattern: &ModerationPattern{
				Name:     "bad-type",
				Content:  "test",
				Type:     "unknown",
				Severity: "medium",
			},
			expectError: true,
		},
		{
			name: "invalid severity fails",
			pattern: &ModerationPattern{
				Name:     "bad-severity",
				Content:  "test",
				Type:     "keyword",
				Severity: "invalid",
			},
			expectError: true,
		},
		{
			name: "invalid regex fails",
			pattern: &ModerationPattern{
				Name:     "bad-regex",
				Content:  "[invalid(",
				Type:     "regex",
				Severity: "medium",
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := pm.CreatePattern(ctx, tt.pattern)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.True(t, tt.pattern.Active)
				assert.False(t, tt.pattern.CreatedAt.IsZero())
				assert.Equal(t, int64(0), tt.pattern.MatchCount)
			}
		})
	}
}

func TestPatternManager_MatchContent(t *testing.T) {
	storage := newFakePatternStorage()
	pm := &PatternManager{
		storage:         storage,
		enhancedEnabled: false,
	}
	ctx := context.Background()

	// Create test patterns
	keywordPattern := &ModerationPattern{
		ID:       "kw-1",
		Name:     "forbidden-keyword",
		Content:  "forbidden",
		Type:     "keyword",
		Severity: "medium",
		Active:   true,
		Action:   "flag",
	}

	regexPattern := &ModerationPattern{
		ID:       "rx-1",
		Name:     "phone-regex",
		Content:  `\d{3}-\d{4}`,
		Type:     "regex",
		Severity: "high",
		Active:   true,
		Action:   "remove",
	}

	phrasePattern := &ModerationPattern{
		ID:       "ph-1",
		Name:     "bad-phrase",
		Content:  "hate speech",
		Type:     "phrase",
		Severity: "critical",
		Active:   true,
		Action:   "remove",
	}

	require.NoError(t, storage.CreateModerationPattern(ctx, keywordPattern))
	require.NoError(t, storage.CreateModerationPattern(ctx, regexPattern))
	require.NoError(t, storage.CreateModerationPattern(ctx, phrasePattern))

	tests := []struct {
		name          string
		content       *ContentToModerate
		expectedCount int
		matchedIDs    []string
	}{
		{
			name: "keyword match",
			content: &ContentToModerate{
				Text: "This content is forbidden here",
			},
			expectedCount: 1,
			matchedIDs:    []string{"kw-1"},
		},
		{
			name: "regex match",
			content: &ContentToModerate{
				Text: "Call me at 555-1234",
			},
			expectedCount: 1,
			matchedIDs:    []string{"rx-1"},
		},
		{
			name: "phrase match",
			content: &ContentToModerate{
				Text: "This contains hate speech",
			},
			expectedCount: 1,
			matchedIDs:    []string{"ph-1"},
		},
		{
			name: "multiple matches",
			content: &ContentToModerate{
				Text: "forbidden phone 555-1234 with hate speech",
			},
			expectedCount: 3,
			matchedIDs:    []string{"kw-1", "rx-1", "ph-1"},
		},
		{
			name: "no match",
			content: &ContentToModerate{
				Text: "This is perfectly normal content",
			},
			expectedCount: 0,
			matchedIDs:    []string{},
		},
		{
			name: "case insensitive keyword match",
			content: &ContentToModerate{
				Text: "This is FORBIDDEN content",
			},
			expectedCount: 1,
			matchedIDs:    []string{"kw-1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matches, err := pm.MatchContent(ctx, tt.content)
			require.NoError(t, err)
			assert.Len(t, matches, tt.expectedCount)

			matchedIDs := make(map[string]bool)
			for _, m := range matches {
				matchedIDs[m.PatternID] = true
			}
			for _, expectedID := range tt.matchedIDs {
				assert.True(t, matchedIDs[expectedID], "expected match for pattern %s", expectedID)
			}
		})
	}
}

func TestPatternManager_UpdatePatternStats(t *testing.T) {
	storage := newFakePatternStorage()
	pm := &PatternManager{
		storage:         storage,
		enhancedEnabled: false,
	}
	ctx := context.Background()

	// Create a test pattern
	pattern := &ModerationPattern{
		ID:                 "stats-test",
		Name:               "stats-pattern",
		Content:            "test",
		Type:               "keyword",
		Severity:           "medium",
		Active:             true,
		MatchCount:         0,
		FalsePositiveCount: 0,
	}
	require.NoError(t, storage.CreateModerationPattern(ctx, pattern))

	tests := []struct {
		name               string
		wasMatch           bool
		wasFalsePositive   bool
		expectedMatchCount int64
		expectedFPCount    int64
	}{
		{
			name:               "record true positive match",
			wasMatch:           true,
			wasFalsePositive:   false,
			expectedMatchCount: 1,
			expectedFPCount:    0,
		},
		{
			name:               "record false positive match",
			wasMatch:           true,
			wasFalsePositive:   true,
			expectedMatchCount: 2,
			expectedFPCount:    1,
		},
		{
			name:               "record another true positive",
			wasMatch:           true,
			wasFalsePositive:   false,
			expectedMatchCount: 3,
			expectedFPCount:    1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := pm.UpdatePatternStats(ctx, "stats-test", tt.wasMatch, tt.wasFalsePositive)
			require.NoError(t, err)

			// Verify counts
			updatedPattern, err := storage.GetModerationPattern(ctx, "stats-test")
			require.NoError(t, err)
			assert.Equal(t, tt.expectedMatchCount, updatedPattern.MatchCount)
			assert.Equal(t, tt.expectedFPCount, updatedPattern.FalsePositiveCount)
		})
	}
}

func TestPatternManager_CalculateEffectiveness(t *testing.T) {
	pm := &PatternManager{}

	tests := []struct {
		name        string
		pattern     *ModerationPattern
		expectedMin float64
		expectedMax float64
	}{
		{
			name: "new pattern with no matches returns 0.5",
			pattern: &ModerationPattern{
				MatchCount:         0,
				FalsePositiveCount: 0,
			},
			expectedMin: 0.5,
			expectedMax: 0.5,
		},
		{
			name: "100% true positive rate",
			pattern: &ModerationPattern{
				MatchCount:         10,
				FalsePositiveCount: 0,
				LastMatch:          time.Now(),
			},
			expectedMin: 1.0,
			expectedMax: 1.0,
		},
		{
			name: "50% false positive rate",
			pattern: &ModerationPattern{
				MatchCount:         10,
				FalsePositiveCount: 5,
				LastMatch:          time.Now(),
			},
			expectedMin: 0.5,
			expectedMax: 0.5,
		},
		{
			name: "stale pattern reduces effectiveness",
			pattern: &ModerationPattern{
				MatchCount:         10,
				FalsePositiveCount: 0,
				LastMatch:          time.Now().Add(-60 * 24 * time.Hour), // 60 days ago
			},
			expectedMin: 0.4,
			expectedMax: 0.6, // 1.0 * 0.5 for staleness
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			effectiveness := pm.calculateEffectiveness(tt.pattern)
			assert.GreaterOrEqual(t, effectiveness, tt.expectedMin)
			assert.LessOrEqual(t, effectiveness, tt.expectedMax)
		})
	}
}

func TestPatternManager_ValidatePattern(t *testing.T) {
	pm := &PatternManager{}

	tests := []struct {
		name        string
		pattern     *ModerationPattern
		expectError bool
	}{
		{
			name: "valid pattern",
			pattern: &ModerationPattern{
				Name:     "valid",
				Content:  "test",
				Type:     "keyword",
				Severity: "medium",
			},
			expectError: false,
		},
		{
			name: "missing name",
			pattern: &ModerationPattern{
				Content:  "test",
				Type:     "keyword",
				Severity: "medium",
			},
			expectError: true,
		},
		{
			name: "missing content",
			pattern: &ModerationPattern{
				Name:     "valid",
				Type:     "keyword",
				Severity: "medium",
			},
			expectError: true,
		},
		{
			name: "missing type",
			pattern: &ModerationPattern{
				Name:     "valid",
				Content:  "test",
				Severity: "medium",
			},
			expectError: true,
		},
		{
			name: "invalid type",
			pattern: &ModerationPattern{
				Name:     "valid",
				Content:  "test",
				Type:     "invalid",
				Severity: "medium",
			},
			expectError: true,
		},
		{
			name: "invalid severity",
			pattern: &ModerationPattern{
				Name:     "valid",
				Content:  "test",
				Type:     "keyword",
				Severity: "invalid",
			},
			expectError: true,
		},
		{
			name: "valid domain type",
			pattern: &ModerationPattern{
				Name:     "domain",
				Content:  "example.com",
				Type:     "domain",
				Severity: "high",
			},
			expectError: false,
		},
		{
			name: "valid ip type",
			pattern: &ModerationPattern{
				Name:     "ip-block",
				Content:  "192.168.1.1",
				Type:     "ip",
				Severity: "critical",
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := pm.validatePattern(tt.pattern)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestPatternManager_MatchKeyword(t *testing.T) {
	pm := &PatternManager{}

	tests := []struct {
		name        string
		keyword     string
		text        string
		expectMatch bool
	}{
		{
			name:        "exact match",
			keyword:     "hello",
			text:        "hello world",
			expectMatch: true,
		},
		{
			name:        "case insensitive match",
			keyword:     "hello",
			text:        "HELLO WORLD",
			expectMatch: true,
		},
		{
			name:        "partial match",
			keyword:     "test",
			text:        "this is a testing string",
			expectMatch: true,
		},
		{
			name:        "no match",
			keyword:     "notfound",
			text:        "this text does not contain the pattern",
			expectMatch: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matched, matchedText := pm.matchKeyword(tt.keyword, tt.text)
			assert.Equal(t, tt.expectMatch, matched)
			if tt.expectMatch {
				assert.NotEmpty(t, matchedText)
			}
		})
	}
}

func TestPatternManager_MatchRegex(t *testing.T) {
	pm := &PatternManager{}

	tests := []struct {
		name        string
		regex       string
		text        string
		expectMatch bool
		matchText   string
	}{
		{
			name:        "phone number match",
			regex:       `\d{3}-\d{4}`,
			text:        "Call 555-1234",
			expectMatch: true,
			matchText:   "555-1234",
		},
		{
			name:        "email pattern",
			regex:       `\w+@\w+\.\w+`,
			text:        "Contact test@example.com",
			expectMatch: true,
			matchText:   "test@example.com",
		},
		{
			name:        "no match",
			regex:       `\d{10}`,
			text:        "no numbers here",
			expectMatch: false,
		},
		{
			name:        "invalid regex returns false",
			regex:       `[invalid(`,
			text:        "any text",
			expectMatch: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matched, matchedText := pm.matchRegex(tt.regex, tt.text)
			assert.Equal(t, tt.expectMatch, matched)
			if tt.expectMatch {
				assert.Equal(t, tt.matchText, matchedText)
			}
		})
	}
}

func TestPatternManager_GeneratePatternRecommendations(t *testing.T) {
	pm := &PatternManager{}

	tests := []struct {
		name                string
		pattern             *ModerationPattern
		expectedContains    []string
		expectedNotContains []string
	}{
		{
			name: "low effectiveness pattern",
			pattern: &ModerationPattern{
				Effectiveness: 0.2,
				MatchCount:    10,
			},
			expectedContains: []string{"low effectiveness"},
		},
		{
			name: "high false positive pattern",
			pattern: &ModerationPattern{
				Effectiveness:      0.5,
				MatchCount:         10,
				FalsePositiveCount: 6,
			},
			expectedContains: []string{"false positive"},
		},
		{
			name: "no matches after 7 days",
			pattern: &ModerationPattern{
				MatchCount: 0,
				CreatedAt:  time.Now().Add(-10 * 24 * time.Hour),
			},
			expectedContains: []string{"no matches after 7 days"},
		},
		{
			name: "stale pattern",
			pattern: &ModerationPattern{
				MatchCount: 10,
				LastMatch:  time.Now().Add(-45 * 24 * time.Hour),
			},
			expectedContains: []string{"outdated"},
		},
		{
			name: "good pattern has no recommendations",
			pattern: &ModerationPattern{
				Effectiveness:      0.9,
				MatchCount:         10,
				FalsePositiveCount: 1,
				LastMatch:          time.Now().Add(-1 * time.Hour),
				CreatedAt:          time.Now().Add(-1 * time.Hour),
			},
			expectedContains: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recommendations := pm.generatePatternRecommendations(tt.pattern)
			for _, expected := range tt.expectedContains {
				found := false
				for _, rec := range recommendations {
					if containsIgnoreCase(rec, expected) {
						found = true
						break
					}
				}
				assert.True(t, found, "expected recommendation containing %q", expected)
			}
		})
	}
}

// Helper function
func containsIgnoreCase(s, substr string) bool {
	sLower := stringToLowerHelper(s)
	substrLower := stringToLowerHelper(substr)
	return len(sLower) >= len(substrLower) && (sLower == substrLower || len(sLower) > 0 && (containsIgnoreCase(sLower[1:], substrLower) || (len(sLower) >= len(substrLower) && sLower[:len(substrLower)] == substrLower)))
}

func stringToLowerHelper(s string) string {
	lower := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c = c + 32
		}
		lower[i] = c
	}
	return string(lower)
}

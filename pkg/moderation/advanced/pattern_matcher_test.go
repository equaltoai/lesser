package advanced

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// fakePatternRepository implements PatternRepository for testing
type fakePatternRepository struct {
	mu       sync.RWMutex
	patterns map[string]*ModerationPattern
	hitCount map[string]int64
}

func newFakePatternRepository() *fakePatternRepository {
	return &fakePatternRepository{
		patterns: make(map[string]*ModerationPattern),
		hitCount: make(map[string]int64),
	}
}

func (f *fakePatternRepository) CreatePattern(_ context.Context, pattern *ModerationPattern) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if pattern.ID == "" {
		return errors.New("pattern ID required")
	}
	f.patterns[pattern.ID] = pattern
	return nil
}

func (f *fakePatternRepository) UpdatePattern(_ context.Context, patternID string, pattern *ModerationPattern) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, exists := f.patterns[patternID]; !exists {
		return errors.New("pattern not found")
	}
	pattern.ID = patternID
	f.patterns[patternID] = pattern
	return nil
}

func (f *fakePatternRepository) DeletePattern(_ context.Context, patternID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.patterns, patternID)
	return nil
}

func (f *fakePatternRepository) GetPattern(_ context.Context, patternID string) (*ModerationPattern, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	if p, exists := f.patterns[patternID]; exists {
		return p, nil
	}
	return nil, errors.New("pattern not found")
}

func (f *fakePatternRepository) GetPatterns(_ context.Context, filter PatternFilter) ([]*ModerationPattern, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	var result []*ModerationPattern
	for _, p := range f.patterns {
		if filter.Active != nil && p.Active != *filter.Active {
			continue
		}
		result = append(result, p)
	}
	return result, nil
}

func (f *fakePatternRepository) IncrementHitCount(_ context.Context, patternID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.hitCount[patternID]++
	if p, exists := f.patterns[patternID]; exists {
		p.HitCount++
	}
	return nil
}

func (f *fakePatternRepository) LoadActivePatterns(_ context.Context) ([]*ModerationPattern, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	var result []*ModerationPattern
	for _, p := range f.patterns {
		if p.Active {
			result = append(result, p)
		}
	}
	return result, nil
}

func (f *fakePatternRepository) getHitCount(patternID string) int64 {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.hitCount[patternID]
}

func TestNewPatternMatcher(t *testing.T) {
	repo := newFakePatternRepository()
	logger := zap.NewNop()

	pm := NewPatternMatcher(repo, logger)
	assert.NotNil(t, pm)
	assert.NotNil(t, pm.repository)
	assert.NotNil(t, pm.logger)
}

func TestPatternMatcher_CreatePattern(t *testing.T) {
	repo := newFakePatternRepository()
	logger := zap.NewNop()
	pm := NewPatternMatcher(repo, logger)
	ctx := context.Background()

	tests := []struct {
		name        string
		pattern     *ModerationPattern
		expectError bool
	}{
		{
			name: "valid regex pattern",
			pattern: &ModerationPattern{
				Name:     "test-regex",
				Pattern:  `\b\d{3}-\d{4}\b`,
				Type:     "regex",
				Severity: 0.5,
				Active:   true,
			},
			expectError: false,
		},
		{
			name: "valid keyword pattern",
			pattern: &ModerationPattern{
				Name:     "test-keyword",
				Pattern:  "badword",
				Type:     "keyword",
				Severity: 0.7,
				Active:   true,
			},
			expectError: false,
		},
		{
			name: "valid phrase pattern",
			pattern: &ModerationPattern{
				Name:     "test-phrase",
				Pattern:  "bad phrase here",
				Type:     "phrase",
				Severity: 0.6,
				Active:   true,
			},
			expectError: false,
		},
		{
			name: "invalid regex pattern fails",
			pattern: &ModerationPattern{
				Name:     "bad-regex",
				Pattern:  "[invalid(",
				Type:     "regex",
				Severity: 0.5,
				Active:   true,
			},
			expectError: true,
		},
		{
			name: "missing name fails",
			pattern: &ModerationPattern{
				Pattern:  "test",
				Type:     "keyword",
				Severity: 0.5,
			},
			expectError: true,
		},
		{
			name: "missing pattern content fails",
			pattern: &ModerationPattern{
				Name:     "empty-pattern",
				Type:     "keyword",
				Severity: 0.5,
			},
			expectError: true,
		},
		{
			name: "invalid type fails",
			pattern: &ModerationPattern{
				Name:     "invalid-type",
				Pattern:  "test",
				Type:     "unknown",
				Severity: 0.5,
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
				assert.NotEmpty(t, tt.pattern.ID)
				assert.False(t, tt.pattern.CreatedAt.IsZero())
			}
		})
	}
}

func TestPatternMatcher_MatchContent(t *testing.T) {
	repo := newFakePatternRepository()
	logger := zap.NewNop()
	pm := NewPatternMatcher(repo, logger)
	ctx := context.Background()

	// Create test patterns
	regexPattern := &ModerationPattern{
		ID:       "regex-1",
		Name:     "phone-number",
		Pattern:  `\d{3}-\d{4}`,
		Type:     "regex",
		Severity: 0.5,
		Active:   true,
	}

	keywordPattern := &ModerationPattern{
		ID:       "keyword-1",
		Name:     "bad-keyword",
		Pattern:  "forbidden",
		Type:     "keyword",
		Severity: 0.6,
		Active:   true,
	}

	phrasePattern := &ModerationPattern{
		ID:       "phrase-1",
		Name:     "bad-phrase",
		Pattern:  "hate speech",
		Type:     "phrase",
		Severity: 0.8,
		Active:   true,
	}

	inactivePattern := &ModerationPattern{
		ID:       "inactive-1",
		Name:     "inactive",
		Pattern:  "shouldnotmatch",
		Type:     "keyword",
		Severity: 0.5,
		Active:   false,
	}

	// Add patterns to repo
	require.NoError(t, pm.CreatePattern(ctx, regexPattern))
	require.NoError(t, pm.CreatePattern(ctx, keywordPattern))
	require.NoError(t, pm.CreatePattern(ctx, phrasePattern))
	require.NoError(t, repo.CreatePattern(ctx, inactivePattern)) // Add directly to bypass validation

	tests := []struct {
		name          string
		content       string
		expectedCount int
		matchedIDs    []string
	}{
		{
			name:          "regex match finds phone number",
			content:       "Call me at 555-1234 please",
			expectedCount: 1,
			matchedIDs:    []string{"regex-1"},
		},
		{
			name:          "keyword match case insensitive",
			content:       "This is FORBIDDEN content",
			expectedCount: 1,
			matchedIDs:    []string{"keyword-1"},
		},
		{
			name:          "phrase match",
			content:       "This contains hate speech and is bad",
			expectedCount: 1,
			matchedIDs:    []string{"phrase-1"},
		},
		{
			name:          "multiple matches",
			content:       "Call 555-1234, this is forbidden",
			expectedCount: 2,
			matchedIDs:    []string{"regex-1", "keyword-1"},
		},
		{
			name:          "no match",
			content:       "This is perfectly normal content",
			expectedCount: 0,
			matchedIDs:    []string{},
		},
		{
			name:          "inactive pattern does not match",
			content:       "shouldnotmatch this content",
			expectedCount: 0,
			matchedIDs:    []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matches, err := pm.MatchContent(ctx, tt.content, ContentMetadata{})
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

func TestPatternMatcher_RegexCache(t *testing.T) {
	repo := newFakePatternRepository()
	logger := zap.NewNop()
	pm := NewPatternMatcher(repo, logger)
	ctx := context.Background()

	// Create a regex pattern
	pattern := &ModerationPattern{
		ID:       "cached-regex",
		Name:     "cached-pattern",
		Pattern:  `test\d+`,
		Type:     "regex",
		Severity: 0.5,
		Active:   true,
	}
	require.NoError(t, pm.CreatePattern(ctx, pattern))

	// First match should compile and cache
	matches1, err := pm.MatchContent(ctx, "test123", ContentMetadata{})
	require.NoError(t, err)
	assert.Len(t, matches1, 1)

	// Second match should use cache
	matches2, err := pm.MatchContent(ctx, "test456", ContentMetadata{})
	require.NoError(t, err)
	assert.Len(t, matches2, 1)

	// Verify regex was cached
	_, cached := pm.regexCache.Load("cached-regex")
	assert.True(t, cached, "regex should be cached")
}

func TestPatternMatcher_CheckPattern(t *testing.T) {
	repo := newFakePatternRepository()
	logger := zap.NewNop()
	pm := NewPatternMatcher(repo, logger)

	tests := []struct {
		name        string
		pattern     *ModerationPattern
		content     string
		expectMatch bool
		matchText   string
	}{
		{
			name: "regex with location",
			pattern: &ModerationPattern{
				ID:      "r1",
				Type:    "regex",
				Pattern: `\btest\b`,
				Active:  true,
			},
			content:     "this is a test case",
			expectMatch: true,
			matchText:   "test",
		},
		{
			name: "keyword case insensitive",
			pattern: &ModerationPattern{
				ID:      "k1",
				Type:    "keyword",
				Pattern: "hello",
				Active:  true,
			},
			content:     "HELLO world",
			expectMatch: true,
			matchText:   "HELLO",
		},
		{
			name: "phrase match",
			pattern: &ModerationPattern{
				ID:      "p1",
				Type:    "phrase",
				Pattern: "bad phrase",
				Active:  true,
			},
			content:     "this has a bad phrase in it",
			expectMatch: true,
			matchText:   "bad phrase",
		},
		{
			name: "no match",
			pattern: &ModerationPattern{
				ID:      "n1",
				Type:    "keyword",
				Pattern: "notfound",
				Active:  true,
			},
			content:     "nothing to see here",
			expectMatch: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			match := pm.checkPattern(tt.pattern, tt.content, stringToLower(tt.content))
			if tt.expectMatch {
				require.NotNil(t, match)
				assert.Equal(t, tt.pattern.ID, match.PatternID)
				assert.NotEmpty(t, match.Location)
			} else {
				assert.Nil(t, match)
			}
		})
	}
}

func TestPatternMatcher_DeletePattern(t *testing.T) {
	repo := newFakePatternRepository()
	logger := zap.NewNop()
	pm := NewPatternMatcher(repo, logger)
	ctx := context.Background()

	// Create a pattern
	pattern := &ModerationPattern{
		ID:       "to-delete",
		Name:     "delete-me",
		Pattern:  "delete",
		Type:     "keyword",
		Severity: 0.5,
		Active:   true,
	}
	require.NoError(t, pm.CreatePattern(ctx, pattern))

	// Verify it exists
	_, cached := pm.patterns.Load("to-delete")
	assert.True(t, cached)

	// Delete it
	err := pm.DeletePattern(ctx, "to-delete")
	require.NoError(t, err)

	// Verify it's gone from cache
	_, cached = pm.patterns.Load("to-delete")
	assert.False(t, cached)
}

func TestPatternMatcher_RefreshPatterns(t *testing.T) {
	repo := newFakePatternRepository()
	logger := zap.NewNop()
	pm := NewPatternMatcher(repo, logger)
	ctx := context.Background()

	// Add pattern directly to repo (bypassing matcher)
	pattern := &ModerationPattern{
		ID:       "refresh-test",
		Name:     "refresh",
		Pattern:  "refresh",
		Type:     "keyword",
		Severity: 0.5,
		Active:   true,
	}
	require.NoError(t, repo.CreatePattern(ctx, pattern))

	// Pattern should not be in matcher cache yet
	_, cached := pm.patterns.Load("refresh-test")
	assert.False(t, cached)

	// Refresh patterns
	err := pm.RefreshPatterns(ctx)
	require.NoError(t, err)

	// Now it should be cached
	_, cached = pm.patterns.Load("refresh-test")
	assert.True(t, cached)
}

func TestGeneratePatternID(t *testing.T) {
	id1 := generatePatternID("Test Pattern")
	id2 := generatePatternID("Another Pattern")

	// IDs should be lowercase and have timestamp
	assert.Contains(t, id1, "test-pattern-")
	assert.Contains(t, id2, "another-pattern-")
	assert.NotEmpty(t, id1)
	assert.NotEmpty(t, id2)
	assert.NotEqual(t, id1, id2) // Different names should produce different IDs

	// Give time difference for uniqueness
	time.Sleep(time.Second)
	id3 := generatePatternID("Test Pattern")
	assert.NotEqual(t, id1, id3) // Same name at different times should produce different IDs
}

// Helper function
func stringToLower(s string) string {
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

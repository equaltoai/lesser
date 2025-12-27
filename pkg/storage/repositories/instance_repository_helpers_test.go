package repositories

import (
	"strings"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestGetWeekStart(t *testing.T) {
	tests := []struct {
		name     string
		input    time.Time
		expected time.Time
	}{
		{
			name:     "Wednesday returns Monday of same week",
			input:    time.Date(2024, 12, 25, 15, 30, 0, 0, time.UTC), // Wednesday
			expected: time.Date(2024, 12, 23, 0, 0, 0, 0, time.UTC),   // Monday
		},
		{
			name:     "Sunday returns Monday of previous week",
			input:    time.Date(2024, 12, 29, 10, 0, 0, 0, time.UTC), // Sunday
			expected: time.Date(2024, 12, 23, 0, 0, 0, 0, time.UTC),  // Monday of previous week
		},
		{
			name:     "Monday at midnight returns same Monday",
			input:    time.Date(2024, 12, 23, 0, 0, 0, 0, time.UTC), // Monday midnight
			expected: time.Date(2024, 12, 23, 0, 0, 0, 0, time.UTC), // Same Monday
		},
		{
			name:     "Monday mid-day returns same Monday at midnight",
			input:    time.Date(2024, 12, 23, 14, 30, 45, 0, time.UTC), // Monday afternoon
			expected: time.Date(2024, 12, 23, 0, 0, 0, 0, time.UTC),    // Same Monday midnight
		},
		{
			name:     "Saturday returns Monday of same week",
			input:    time.Date(2024, 12, 28, 23, 59, 59, 0, time.UTC), // Saturday late night
			expected: time.Date(2024, 12, 23, 0, 0, 0, 0, time.UTC),    // Monday
		},
		{
			name:     "Tuesday returns Monday of same week",
			input:    time.Date(2024, 12, 24, 8, 0, 0, 0, time.UTC), // Tuesday
			expected: time.Date(2024, 12, 23, 0, 0, 0, 0, time.UTC), // Monday
		},
		{
			name:     "Friday returns Monday of same week",
			input:    time.Date(2024, 12, 27, 12, 0, 0, 0, time.UTC), // Friday
			expected: time.Date(2024, 12, 23, 0, 0, 0, 0, time.UTC),  // Monday
		},
		{
			name:     "Year boundary - Sunday Jan 5 2025 returns Monday Dec 30 2024",
			input:    time.Date(2025, 1, 5, 10, 0, 0, 0, time.UTC),  // Sunday Jan 5
			expected: time.Date(2024, 12, 30, 0, 0, 0, 0, time.UTC), // Monday Dec 30
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getWeekStart(tt.input)
			assert.Equal(t, tt.expected, result,
				"getWeekStart(%v) = %v, want %v", tt.input, result, tt.expected)
		})
	}
}

func TestGetDefaultInstanceRules(t *testing.T) {
	logger := zap.NewNop()
	repo := &InstanceRepository{logger: logger}

	rules := repo.getDefaultInstanceRules()

	t.Run("returns exactly 5 rules", func(t *testing.T) {
		assert.Len(t, rules, 5)
	})

	t.Run("all rules have sequential IDs 1-5", func(t *testing.T) {
		for i, rule := range rules {
			expectedID := string(rune('1' + i))
			assert.Equal(t, expectedID, rule.ID, "rule %d should have ID %s", i, expectedID)
		}
	})

	t.Run("all rules have non-empty text", func(t *testing.T) {
		for i, rule := range rules {
			assert.NotEmpty(t, rule.Text, "rule %d should have non-empty text", i)
		}
	})

	t.Run("rules cover expected topics", func(t *testing.T) {
		texts := make([]string, len(rules))
		for i, r := range rules {
			texts[i] = strings.ToLower(r.Text)
		}
		allText := strings.Join(texts, " ")

		assert.Contains(t, allText, "respect", "should have respect-related rule")
		assert.Contains(t, allText, "harassment", "should have harassment-related rule")
		assert.Contains(t, allText, "spam", "should have spam-related rule")
		assert.Contains(t, allText, "content warning", "should have content warning rule")
		assert.Contains(t, allText, "law", "should have law-related rule")
	})
}

func TestValidateAndFilterRules(t *testing.T) {
	logger := zap.NewNop()
	repo := &InstanceRepository{logger: logger}

	t.Run("assigns IDs to rules with missing IDs", func(t *testing.T) {
		input := []storage.InstanceRule{
			{ID: "", Text: "Rule without ID"},
			{ID: "existing", Text: "Rule with existing ID"},
			{ID: "", Text: "Another rule without ID"},
		}

		result := repo.validateAndFilterRules(input)

		require.Len(t, result, 3)
		assert.Equal(t, "rule_1", result[0].ID, "first rule should get assigned ID")
		assert.Equal(t, "existing", result[1].ID, "existing ID should be preserved")
		assert.Equal(t, "rule_3", result[2].ID, "third rule should get assigned ID")
	})

	t.Run("handles duplicate IDs by appending suffix", func(t *testing.T) {
		input := []storage.InstanceRule{
			{ID: "dup", Text: "First rule"},
			{ID: "dup", Text: "Duplicate ID rule"},
			{ID: "unique", Text: "Unique rule"},
			{ID: "dup", Text: "Third duplicate"},
		}

		result := repo.validateAndFilterRules(input)

		require.Len(t, result, 4)
		assert.Equal(t, "dup", result[0].ID)
		assert.Equal(t, "dup_dup_1", result[1].ID, "duplicate should get suffix")
		assert.Equal(t, "unique", result[2].ID)
		assert.Equal(t, "dup_dup_3", result[3].ID, "third duplicate should get suffix")
	})

	t.Run("drops rules with empty text", func(t *testing.T) {
		input := []storage.InstanceRule{
			{ID: "1", Text: "Valid rule"},
			{ID: "2", Text: ""},
			{ID: "3", Text: "   "}, // whitespace only
			{ID: "4", Text: "Another valid rule"},
		}

		result := repo.validateAndFilterRules(input)

		require.Len(t, result, 2)
		assert.Equal(t, "1", result[0].ID)
		assert.Equal(t, "4", result[1].ID)
	})

	t.Run("truncates text longer than 500 characters", func(t *testing.T) {
		longText := strings.Repeat("a", 600)
		input := []storage.InstanceRule{
			{ID: "1", Text: longText},
		}

		result := repo.validateAndFilterRules(input)

		require.Len(t, result, 1)
		assert.Len(t, result[0].Text, 500, "text should be truncated to 500 chars")
		assert.True(t, strings.HasSuffix(result[0].Text, "..."), "truncated text should end with ...")
	})

	t.Run("text exactly 500 characters is not truncated", func(t *testing.T) {
		exactText := strings.Repeat("b", 500)
		input := []storage.InstanceRule{
			{ID: "1", Text: exactText},
		}

		result := repo.validateAndFilterRules(input)

		require.Len(t, result, 1)
		assert.Equal(t, exactText, result[0].Text, "exact 500 char text should not be modified")
	})

	t.Run("empty input returns empty slice", func(t *testing.T) {
		result := repo.validateAndFilterRules([]storage.InstanceRule{})
		assert.Empty(t, result)
	})
}

func TestGenerateDefaultDescription(t *testing.T) {
	logger := zap.NewNop()
	repo := &InstanceRepository{logger: logger}

	desc := repo.generateDefaultDescription()

	t.Run("contains welcome section", func(t *testing.T) {
		assert.Contains(t, desc, "Welcome")
	})

	t.Run("contains ActivityPub mention", func(t *testing.T) {
		assert.Contains(t, desc, "ActivityPub")
	})

	t.Run("contains features section", func(t *testing.T) {
		assert.Contains(t, desc, "Features")
	})

	t.Run("contains About section", func(t *testing.T) {
		assert.Contains(t, desc, "About")
	})

	t.Run("is valid HTML-like structure", func(t *testing.T) {
		assert.Contains(t, desc, "<div")
		assert.Contains(t, desc, "</div>")
		assert.Contains(t, desc, "<h2>")
		assert.Contains(t, desc, "<ul>")
	})

	t.Run("mentions Lesser", func(t *testing.T) {
		assert.Contains(t, desc, "Lesser")
	})

	t.Run("mentions key features", func(t *testing.T) {
		assert.Contains(t, desc, "Mastodon API")
		assert.Contains(t, desc, "GraphQL")
		assert.Contains(t, desc, "WebSocket")
	})
}

func TestInstanceStateCacheHelpers(t *testing.T) {
	logger := zap.NewNop()

	t.Run("setCachedState and getCachedState round trip", func(t *testing.T) {
		repo := &InstanceRepository{logger: logger}

		state := models.NewDefaultInstanceState()
		state.Locked = false

		repo.setCachedState(state)

		cached, ok := repo.getCachedState()
		assert.True(t, ok, "should return cached state")
		assert.Equal(t, state, cached)
	})

	t.Run("getCachedState returns false when cache is empty", func(t *testing.T) {
		repo := &InstanceRepository{logger: logger}

		cached, ok := repo.getCachedState()
		assert.False(t, ok, "should return false for empty cache")
		assert.Nil(t, cached)
	})

	t.Run("getCachedState returns false when cache is expired", func(t *testing.T) {
		repo := &InstanceRepository{logger: logger}

		state := models.NewDefaultInstanceState()
		state.Locked = true
		repo.setCachedState(state)

		// Force expiration by setting expiresAt to the past
		repo.stateCache.mu.Lock()
		repo.stateCache.expiresAt = time.Now().Add(-1 * time.Hour)
		repo.stateCache.mu.Unlock()

		cached, ok := repo.getCachedState()
		assert.False(t, ok, "should return false for expired cache")
		assert.Nil(t, cached)
	})

	t.Run("invalidateStateCache clears the cache", func(t *testing.T) {
		repo := &InstanceRepository{logger: logger}

		state := models.NewDefaultInstanceState()
		state.Locked = false
		repo.setCachedState(state)

		// Verify it's cached
		cached, ok := repo.getCachedState()
		assert.True(t, ok)
		assert.NotNil(t, cached)

		// Invalidate
		repo.invalidateStateCache()

		// Verify it's cleared
		cached, ok = repo.getCachedState()
		assert.False(t, ok, "should return false after invalidation")
		assert.Nil(t, cached)
	})
}

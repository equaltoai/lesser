package repositories

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// ============================================================================
// determineSeverity Tests
// ============================================================================

func TestDetermineSeverity(t *testing.T) {
	repo := &FederationRepository{}

	tests := []struct {
		name             string
		issueType        string
		expectedSeverity string
	}{
		{
			name:             "blocked_returns_critical",
			issueType:        "blocked",
			expectedSeverity: StatusCritical,
		},
		{
			name:             "defederation_returns_critical",
			issueType:        "defederation",
			expectedSeverity: StatusCritical,
		},
		{
			name:             "unreachable_returns_high",
			issueType:        "unreachable",
			expectedSeverity: "high",
		},
		{
			name:             "timeout_returns_high",
			issueType:        StatusTimeout, // "timeout"
			expectedSeverity: "high",
		},
		{
			name:             "error_returns_medium",
			issueType:        StatusError, // "error"
			expectedSeverity: "medium",
		},
		{
			name:             "unknown_returns_low",
			issueType:        "unknown",
			expectedSeverity: "low",
		},
		{
			name:             "empty_returns_low",
			issueType:        "",
			expectedSeverity: "low",
		},
		{
			name:             "random_string_returns_low",
			issueType:        "some-other-issue",
			expectedSeverity: "low",
		},
		{
			name:             "warning_returns_low",
			issueType:        "warning",
			expectedSeverity: "low",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := repo.determineSeverity(tt.issueType)
			assert.Equal(t, tt.expectedSeverity, result)
		})
	}
}

// ============================================================================
// generateFederationRandomString Tests
// ============================================================================

func TestGenerateFederationRandomString(t *testing.T) {
	t.Run("returns_requested_length", func(t *testing.T) {
		result := generateFederationRandomString(12)
		assert.Len(t, result, 12)
	})

	t.Run("returns_full_uuid_if_length_exceeds", func(t *testing.T) {
		// UUID without hyphens is 32 chars
		result := generateFederationRandomString(50)
		assert.Len(t, result, 32) // max length of UUID without hyphens
	})

	t.Run("returns_unique_values", func(t *testing.T) {
		result1 := generateFederationRandomString(16)
		result2 := generateFederationRandomString(16)
		assert.NotEqual(t, result1, result2)
	})

	t.Run("returns_alphanumeric_only", func(t *testing.T) {
		result := generateFederationRandomString(32)
		// Should not contain hyphens (they are removed from UUID)
		assert.NotContains(t, result, "-")
	})
}

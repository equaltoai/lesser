package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEnhancedPatterns_UpdateKeys_AndMetrics(t *testing.T) {
	t.Run("EnhancedModerationPattern UpdateKeys sets GSIs and TTL (active)", func(t *testing.T) {
		now := time.Unix(1700000000, 0).UTC()
		p := &EnhancedModerationPattern{
			PatternID:   "p1",
			PatternType: "url_exact",
			Category:    "spam",
			Severity:    "high",
			Priority:    7,
			Active:      true,
			MatchCount:  12,
			Effectiveness: 0.875,
			UpdatedAt:   now,
		}

		before := time.Now()
		require.NoError(t, p.UpdateKeys())

		assert.Equal(t, "ENHANCED_PATTERN#p1", p.PK)
		assert.Equal(t, SKMetadata, p.SK)
		assert.Equal(t, "ENHANCED_PATTERNS#ACTIVE", p.GSI1PK)
		assert.Contains(t, p.GSI1SK, "url_exact")
		assert.Contains(t, p.GSI2PK, "ENHANCED_PATTERNS#url_exact")
		assert.Contains(t, p.GSI3PK, "PATTERN_METRICS#spam")
		assert.Equal(t, "ENHANCED_PATTERN", p.Type)
		assert.True(t, p.TTL > 0)
		assert.True(t, time.Unix(p.TTL, 0).After(before.Add(89*24*time.Hour)))
		assert.Equal(t, MainTableName, p.TableName())
		assert.Equal(t, p.PK, p.GetPK())
		assert.Equal(t, p.SK, p.GetSK())
	})

	t.Run("EnhancedModerationPattern UpdateKeys clears GSI1 when inactive", func(t *testing.T) {
		p := &EnhancedModerationPattern{
			PatternID:   "p2",
			PatternType: "keyword",
			Category:    "spam",
			Severity:    "low",
			Priority:    1,
			Active:      false,
			UpdatedAt:   time.Unix(1700000000, 0).UTC(),
		}
		require.NoError(t, p.UpdateKeys())
		assert.Empty(t, p.GSI1PK)
		assert.Empty(t, p.GSI1SK)
	})

	t.Run("CalculateEffectiveness handles new patterns and clamps bounds", func(t *testing.T) {
		p := &EnhancedModerationPattern{}
		p.CalculateEffectiveness()
		assert.InDelta(t, 0.5, p.Effectiveness, 0.000001)

		p = &EnhancedModerationPattern{
			MatchCount:        10,
			TruePositiveCount: 10,
			ConfidenceScore:   1.0,
			ValidationScore:   1.0,
			LastMatch:         time.Now().Add(-40 * 24 * time.Hour),
		}
		p.CalculateEffectiveness()
		assert.InDelta(t, 0.91, p.Effectiveness, 0.00001)

		p = &EnhancedModerationPattern{
			MatchCount:        10,
			TruePositiveCount: 10,
			ConfidenceScore:   10,
			ValidationScore:   10,
			LastMatch:         time.Now().Add(-10 * 24 * time.Hour),
		}
		p.CalculateEffectiveness()
		assert.Equal(t, 1.0, p.Effectiveness)

		p = &EnhancedModerationPattern{
			MatchCount:        10,
			TruePositiveCount: 0,
			ConfidenceScore:   -10,
			ValidationScore:   -10,
		}
		p.CalculateEffectiveness()
		assert.Equal(t, 0.0, p.Effectiveness)
	})

	t.Run("IsExpired and ShouldEscalate behavior", func(t *testing.T) {
		p := &EnhancedModerationPattern{}
		assert.False(t, p.IsExpired())

		p.ExpiresAt = time.Now().Add(-time.Second)
		assert.True(t, p.IsExpired())

		p = &EnhancedModerationPattern{EscalationThreshold: 0, MatchCount: 100}
		assert.False(t, p.ShouldEscalate())
		p.EscalationThreshold = 5
		p.MatchCount = 5
		assert.True(t, p.ShouldEscalate())
	})

	t.Run("PatternCache UpdateKeys sets keys, marker, and TTL", func(t *testing.T) {
		cache := &PatternCache{
			PatternID:   "p1",
			PatternType: "url_exact",
			UpdatedAt:   time.Unix(1700000000, 0).UTC(),
		}
		before := time.Now()
		require.NoError(t, cache.UpdateKeys())
		assert.Equal(t, "PATTERN_CACHE#url_exact", cache.PK)
		assert.Equal(t, "COMPILED#p1", cache.SK)
		assert.Equal(t, "PATTERN_CACHE#ACTIVE", cache.GSI1PK)
		assert.Contains(t, cache.GSI1SK, "p1")
		assert.Equal(t, "PATTERN_CACHE", cache.Type)
		assert.True(t, cache.TTL > 0)
		assert.True(t, time.Unix(cache.TTL, 0).After(before.Add(23*time.Hour)))
		assert.Equal(t, MainTableName, cache.TableName())
		assert.Equal(t, cache.PK, cache.GetPK())
		assert.Equal(t, cache.SK, cache.GetSK())
	})

	t.Run("PatternPerformanceMetric UpdateKeys and CalculateQualityMetrics", func(t *testing.T) {
		m := &PatternPerformanceMetric{
			PatternID:     "p1",
			PatternType:   "url_exact",
			Date:          "2024-01-01",
			Hour:          3,
			TruePositives: 8,
			FalsePositives: 2,
		}
		before := time.Now()
		require.NoError(t, m.UpdateKeys())
		assert.Equal(t, "PATTERN_METRICS#p1", m.PK)
		assert.Equal(t, "TIME#2024-01-01#03", m.SK)
		assert.Equal(t, "METRICS#url_exact#2024-01-01", m.GSI1PK)
		assert.Equal(t, "03#p1", m.GSI1SK)
		assert.Equal(t, "PATTERN_PERFORMANCE", m.GSI2PK)
		assert.Contains(t, m.GSI2SK, "url_exact")
		assert.Equal(t, "PATTERN_METRIC", m.Type)
		assert.True(t, m.TTL > 0)
		assert.True(t, time.Unix(m.TTL, 0).After(before.Add(29*24*time.Hour)))

		m.CalculateQualityMetrics()
		assert.InDelta(t, 0.8, m.Precision, 0.000001)
		assert.Equal(t, 0.0, m.F1Score)

		m.Recall = 0.5
		m.CalculateQualityMetrics()
		assert.True(t, m.F1Score > 0)

		assert.Equal(t, MainTableName, m.TableName())
		assert.Equal(t, m.PK, m.GetPK())
		assert.Equal(t, m.SK, m.GetSK())
	})

	t.Run("PatternTestResult UpdateKeys and security helpers", func(t *testing.T) {
		tr := &PatternTestResult{
			TestID:    "t1",
			PatternID: "p1",
			TestType:  "security",
			Score:     0.9,
			RunAt:     time.Unix(1700000000, 0).UTC(),
		}
		before := time.Now()
		require.NoError(t, tr.UpdateKeys())
		assert.Equal(t, "PATTERN_TEST#p1", tr.PK)
		assert.Equal(t, "TEST#t1", tr.SK)
		assert.Equal(t, "PATTERN_TESTS#security", tr.GSI1PK)
		assert.Contains(t, tr.GSI1SK, "t1")
		assert.Equal(t, "PATTERN_TEST", tr.Type)
		assert.True(t, tr.TTL > 0)
		assert.True(t, time.Unix(tr.TTL, 0).After(before.Add(89*24*time.Hour)))
		assert.True(t, tr.IsSecurityTest())
		assert.InDelta(t, 0.9, tr.GetSecurityScore(), 0.000001)

		tr.TestType = "validation"
		assert.False(t, tr.IsSecurityTest())
		assert.Equal(t, 0.0, tr.GetSecurityScore())

		assert.Equal(t, MainTableName, tr.TableName())
		assert.Equal(t, tr.PK, tr.GetPK())
		assert.Equal(t, tr.SK, tr.GetSK())
	})
}


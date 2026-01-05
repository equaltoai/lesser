package advanced

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// Helper to create a test decision engine
func newTestDecisionEngine() *DecisionEngine {
	logger := zap.NewNop()
	config := &ModerationConfig{
		ToxicityThreshold:   0.7,
		AutoRemoveThreshold: 0.9,
		QuarantineThreshold: 0.7,
		FlagThreshold:       0.5,
	}
	return NewDecisionEngine(config, logger, nil)
}

func TestCollectSignals(t *testing.T) {
	de := newTestDecisionEngine()

	tests := []struct {
		name          string
		analysis      *ModerationAnalysis
		expectedTypes []string
		expectedCount int
	}{
		{
			name: "toxicity signal",
			analysis: &ModerationAnalysis{
				TextAnalysis: &ContentAnalysis{
					Toxicity: ToxicityAnalysis{
						IsToxic:       true,
						ToxicityScore: 0.85,
						Confidence:    0.9,
					},
				},
			},
			expectedTypes: []string{"toxicity"},
			expectedCount: 1,
		},
		{
			name: "threat signal",
			analysis: &ModerationAnalysis{
				TextAnalysis: &ContentAnalysis{
					Threats: []ThreatIndicator{
						{Type: "VIOLENCE", Severity: SeverityHigh, Confidence: 0.85},
					},
				},
			},
			expectedTypes: []string{"threat"},
			expectedCount: 1,
		},
		{
			name: "PII signal",
			analysis: &ModerationAnalysis{
				TextAnalysis: &ContentAnalysis{
					PII: []PIIEntity{
						{Type: "EMAIL", Text: "test@example.com"},
					},
				},
			},
			expectedTypes: []string{"pii"},
			expectedCount: 1,
		},
		{
			name: "extreme negative sentiment",
			analysis: &ModerationAnalysis{
				TextAnalysis: &ContentAnalysis{
					Sentiment: SentimentAnalysis{
						Negative:   0.95,
						Confidence: 0.8,
					},
				},
			},
			expectedTypes: []string{"extreme_negative_sentiment"},
			expectedCount: 1,
		},
		{
			name: "image explicit content",
			analysis: &ModerationAnalysis{
				ImageAnalysis: &ImageAnalysis{
					Explicit: ExplicitContent{
						IsExplicit:  true,
						NudityScore: 0.9,
						Confidence:  0.85,
					},
				},
			},
			expectedTypes: []string{"explicit_content"},
			expectedCount: 1,
		},
		{
			name: "image violence",
			analysis: &ModerationAnalysis{
				ImageAnalysis: &ImageAnalysis{
					Violence: ViolenceDetection{
						HasViolence:   true,
						ViolenceScore: 0.8,
						Confidence:    0.9,
					},
				},
			},
			expectedTypes: []string{"violence"},
			expectedCount: 1,
		},
		{
			name: "pattern match",
			analysis: &ModerationAnalysis{
				PatternMatches: []PatternMatch{
					{PatternID: "p1", PatternName: "bad-word", MatchText: "bad", Confidence: 1.0},
				},
			},
			expectedTypes: []string{"pattern_match"},
			expectedCount: 1,
		},
		{
			name: "threat match",
			analysis: &ModerationAnalysis{
				ThreatMatches: []ThreatMatch{
					{ThreatID: "t1", ThreatType: "malware", Confidence: 0.95},
				},
			},
			expectedTypes: []string{"threat_match"},
			expectedCount: 1,
		},
		{
			name: "multiple signals combined",
			analysis: &ModerationAnalysis{
				TextAnalysis: &ContentAnalysis{
					Toxicity: ToxicityAnalysis{IsToxic: true, ToxicityScore: 0.8, Confidence: 0.9},
					Threats:  []ThreatIndicator{{Type: "VIOLENCE", Severity: SeverityHigh, Confidence: 0.85}},
					PII:      []PIIEntity{{Type: "PHONE", Text: "555-1234"}},
				},
				PatternMatches: []PatternMatch{{PatternID: "p1", Confidence: 1.0}},
			},
			expectedTypes: []string{"toxicity", "threat", "pii", "pattern_match"},
			expectedCount: 4,
		},
		{
			name:          "no signals from empty analysis",
			analysis:      &ModerationAnalysis{},
			expectedTypes: []string{},
			expectedCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			signals := de.collectSignals(tt.analysis)
			assert.Len(t, signals, tt.expectedCount)

			// Verify expected signal types are present
			signalTypes := make(map[string]bool)
			for _, s := range signals {
				signalTypes[s.Type] = true
			}
			for _, expectedType := range tt.expectedTypes {
				assert.True(t, signalTypes[expectedType], "expected signal type %s not found", expectedType)
			}
		})
	}
}

func TestCalculateWeightedScore(t *testing.T) {
	de := newTestDecisionEngine()

	tests := []struct {
		name           string
		signals        []Signal
		expectedScore  float64
		expectedConf   float64
		scoreTolerance float64
	}{
		{
			name:           "empty signals returns zero score and full confidence",
			signals:        []Signal{},
			expectedScore:  0.0,
			expectedConf:   1.0,
			scoreTolerance: 0.01,
		},
		{
			name: "single toxicity signal",
			signals: []Signal{
				{Type: "toxicity", Severity: SeverityMedium, Score: 0.8, Confidence: 0.9},
			},
			expectedScore:  0.72, // score * confidence (weight cancels for a single signal)
			expectedConf:   0.9,
			scoreTolerance: 0.01,
		},
		{
			name: "high severity threat signal weighted more",
			signals: []Signal{
				{Type: "threat", Severity: SeverityHigh, Score: 0.9, Confidence: 0.85},
			},
			expectedScore:  0.765, // score * confidence (weight cancels for a single signal)
			expectedConf:   0.85,
			scoreTolerance: 0.01,
		},
		{
			name: "multiple signals averaged",
			signals: []Signal{
				{Type: "toxicity", Severity: SeverityMedium, Score: 0.6, Confidence: 0.8},
				{Type: "pii", Severity: SeverityMedium, Score: 0.7, Confidence: 0.9},
			},
			expectedScore:  0.5442857142857143, // weighted average of score*confidence by signal weight
			expectedConf:   0.85,               // average confidence
			scoreTolerance: 0.01,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score, confidence := de.calculateWeightedScore(tt.signals)
			assert.InDelta(t, tt.expectedScore, score, tt.scoreTolerance, "score mismatch")
			assert.InDelta(t, tt.expectedConf, confidence, 0.1, "confidence mismatch")
		})
	}
}

func TestDetermineAction(t *testing.T) {
	de := newTestDecisionEngine()

	tests := []struct {
		name           string
		score          float64
		signals        []Signal
		expectedAction ModerationAction
	}{
		{
			name:           "low score allows content",
			score:          0.3,
			signals:        []Signal{},
			expectedAction: ActionAllow,
		},
		{
			name:           "at flag threshold flags content",
			score:          0.5,
			signals:        []Signal{},
			expectedAction: ActionFlag,
		},
		{
			name:           "above flag threshold flags content",
			score:          0.6,
			signals:        []Signal{},
			expectedAction: ActionFlag,
		},
		{
			name:           "at quarantine threshold quarantines",
			score:          0.7,
			signals:        []Signal{},
			expectedAction: ActionQuarantine,
		},
		{
			name:           "at auto-remove threshold removes",
			score:          0.9,
			signals:        []Signal{},
			expectedAction: ActionRemove,
		},
		{
			name:           "above auto-remove threshold removes",
			score:          0.95,
			signals:        []Signal{},
			expectedAction: ActionRemove,
		},
		{
			name:  "critical non-SELF_HARM threat removes immediately",
			score: 0.3, // even low score
			signals: []Signal{
				{
					Type:       "threat",
					Severity:   SeverityCritical,
					Confidence: 0.9,
					Evidence:   ThreatIndicator{Type: "VIOLENCE"},
				},
			},
			expectedAction: ActionRemove,
		},
		{
			name:  "SELF_HARM critical threat flags for human review",
			score: 0.3,
			signals: []Signal{
				{
					Type:       "threat",
					Severity:   SeverityCritical,
					Confidence: 0.9,
					Evidence:   ThreatIndicator{Type: "SELF_HARM"},
				},
			},
			expectedAction: ActionFlag,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			action := de.determineAction(tt.score, tt.signals)
			assert.Equal(t, tt.expectedAction, action)
		})
	}
}

func TestSetReviewRequirements(t *testing.T) {
	de := newTestDecisionEngine()

	tests := []struct {
		name         string
		decision     *ModerationDecision
		signals      []Signal
		score        float64
		expectReview bool
		minPriority  int
	}{
		{
			name:         "remove action requires review with high priority",
			decision:     &ModerationDecision{Decision: ActionRemove, Confidence: 0.9},
			signals:      []Signal{},
			score:        0.95,
			expectReview: true,
			minPriority:  8,
		},
		{
			name:         "shadow ban requires review with high priority",
			decision:     &ModerationDecision{Decision: ActionShadowBan, Confidence: 0.9},
			signals:      []Signal{},
			score:        0.96,
			expectReview: true,
			minPriority:  8,
		},
		{
			name:         "low confidence non-allow requires review",
			decision:     &ModerationDecision{Decision: ActionFlag, Confidence: 0.5},
			signals:      []Signal{},
			score:        0.6,
			expectReview: true,
			minPriority:  5,
		},
		{
			name:     "critical threat requires highest priority review",
			decision: &ModerationDecision{Decision: ActionFlag, Confidence: 0.9},
			signals: []Signal{
				{Type: "threat", Severity: SeverityCritical},
			},
			score:        0.6,
			expectReview: true,
			minPriority:  10,
		},
		{
			name:         "borderline score requires review",
			decision:     &ModerationDecision{Decision: ActionAllow, Confidence: 0.9},
			signals:      []Signal{},
			score:        0.45, // within 0.1 of flagThreshold (0.5)
			expectReview: true,
			minPriority:  3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			de.setReviewRequirements(tt.decision, tt.signals, tt.score)
			assert.Equal(t, tt.expectReview, tt.decision.RequiresReview)
			assert.GreaterOrEqual(t, tt.decision.ReviewPriority, tt.minPriority)
		})
	}
}

func TestGenerateRecommendations(t *testing.T) {
	de := newTestDecisionEngine()

	tests := []struct {
		name             string
		decision         *ModerationDecision
		analysis         *ModerationAnalysis
		expectedContains []string
	}{
		{
			name: "remove action generates policy recommendations",
			decision: &ModerationDecision{
				Decision: ActionRemove,
				Reasons:  []DecisionReason{},
			},
			analysis:         &ModerationAnalysis{},
			expectedContains: []string{"policy violation", "Notify user"},
		},
		{
			name: "SELF_HARM threat generates mental health recommendations",
			decision: &ModerationDecision{
				Decision: ActionFlag,
				Reasons: []DecisionReason{
					{Type: "threat", Evidence: ThreatIndicator{Type: "SELF_HARM"}},
				},
			},
			analysis:         &ModerationAnalysis{},
			expectedContains: []string{"mental health", "wellness check"},
		},
		{
			name: "VIOLENCE threat generates security recommendations",
			decision: &ModerationDecision{
				Decision: ActionRemove,
				Reasons: []DecisionReason{
					{Type: "threat", Evidence: ThreatIndicator{Type: "VIOLENCE"}},
				},
			},
			analysis:         &ModerationAnalysis{},
			expectedContains: []string{"security team", "Document evidence"},
		},
		{
			name: "bad_actor reputation generates account restriction recommendations",
			decision: &ModerationDecision{
				Decision: ActionFlag,
				Reasons:  []DecisionReason{},
			},
			analysis: &ModerationAnalysis{
				ReputationScore: &ReputationScore{Level: "bad_actor"},
			},
			expectedContains: []string{"account restrictions", "Review all recent"},
		},
		{
			name: "trusted user with non-allow generates double-check recommendation",
			decision: &ModerationDecision{
				Decision: ActionFlag,
				Reasons:  []DecisionReason{},
			},
			analysis: &ModerationAnalysis{
				ReputationScore: &ReputationScore{Level: "trusted"},
			},
			expectedContains: []string{"Double-check"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recommendations := de.generateRecommendations(tt.decision, tt.analysis)
			for _, expected := range tt.expectedContains {
				found := false
				for _, rec := range recommendations {
					if containsSubstring(rec, expected) {
						found = true
						break
					}
				}
				assert.True(t, found, "expected recommendation containing %q not found in %v", expected, recommendations)
			}
		})
	}
}

func TestMakeDecision(t *testing.T) {
	de := newTestDecisionEngine()
	ctx := context.Background()

	tests := []struct {
		name           string
		analysis       *ModerationAnalysis
		expectedAction ModerationAction
		expectReview   bool
		expectExpiry   bool
	}{
		{
			name: "clean content is allowed",
			analysis: &ModerationAnalysis{
				ContentMetadata: ContentMetadata{ContentID: "test-1"},
				TextAnalysis: &ContentAnalysis{
					Toxicity: ToxicityAnalysis{IsToxic: false, ToxicityScore: 0.1},
				},
			},
			expectedAction: ActionAllow,
			expectReview:   false,
			expectExpiry:   false,
		},
		{
			name: "highly toxic content with moderate confidence is quarantined",
			analysis: &ModerationAnalysis{
				ContentMetadata: ContentMetadata{ContentID: "test-2"},
				TextAnalysis: &ContentAnalysis{
					Toxicity: ToxicityAnalysis{IsToxic: true, ToxicityScore: 0.95, Confidence: 0.9},
				},
			},
			expectedAction: ActionQuarantine,
			expectReview:   false,
			expectExpiry:   true,
		},
		{
			name: "highly toxic content with high confidence is removed",
			analysis: &ModerationAnalysis{
				ContentMetadata: ContentMetadata{ContentID: "test-3"},
				TextAnalysis: &ContentAnalysis{
					Toxicity: ToxicityAnalysis{IsToxic: true, ToxicityScore: 0.95, Confidence: 1.0},
				},
			},
			expectedAction: ActionRemove,
			expectReview:   true,
			expectExpiry:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decision, err := de.MakeDecision(ctx, tt.analysis)
			require.NoError(t, err)
			assert.Equal(t, tt.expectedAction, decision.Decision)
			assert.Equal(t, tt.expectReview, decision.RequiresReview)
			if tt.expectExpiry {
				assert.False(t, decision.ExpiresAt.IsZero())
				assert.WithinDuration(t, time.Now().Add(24*time.Hour), decision.ExpiresAt, 10*time.Second)
			} else {
				assert.True(t, decision.ExpiresAt.IsZero())
			}
			assert.NotZero(t, decision.DecidedAt)
		})
	}
}

func TestGetToxicitySeverity(t *testing.T) {
	de := newTestDecisionEngine()

	tests := []struct {
		score    float64
		expected Severity
	}{
		{0.95, SeverityHigh},
		{0.9, SeverityHigh},
		{0.85, SeverityMedium},
		{0.7, SeverityMedium},
		{0.6, SeverityLow},
		{0.5, SeverityLow},
		{0.3, SeverityLow},
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			severity := de.getToxicitySeverity(tt.score)
			assert.Equal(t, tt.expected, severity)
		})
	}
}

func TestGetSeverityScore(t *testing.T) {
	de := newTestDecisionEngine()

	tests := []struct {
		severity Severity
		expected float64
	}{
		{SeverityCritical, 0.95},
		{SeverityHigh, 0.8},
		{SeverityMedium, 0.6},
		{SeverityLow, 0.4},
	}

	for _, tt := range tests {
		t.Run(string(tt.severity), func(t *testing.T) {
			score := de.getSeverityScore(tt.severity)
			assert.Equal(t, tt.expected, score)
		})
	}
}

func TestSortReasonsBySeverity(t *testing.T) {
	de := newTestDecisionEngine()

	reasons := []DecisionReason{
		{Type: "low", Severity: SeverityLow},
		{Type: "critical", Severity: SeverityCritical},
		{Type: "medium", Severity: SeverityMedium},
		{Type: "high", Severity: SeverityHigh},
	}

	de.sortReasonsBySeverity(reasons)

	assert.Equal(t, SeverityCritical, reasons[0].Severity)
	assert.Equal(t, SeverityHigh, reasons[1].Severity)
	assert.Equal(t, SeverityMedium, reasons[2].Severity)
	assert.Equal(t, SeverityLow, reasons[3].Severity)
}

func TestCalculateReputationMultiplier(t *testing.T) {
	de := newTestDecisionEngine()

	tests := []struct {
		level    string
		expected float64
	}{
		{"trusted", 0.7},
		{"normal", 1.0},
		{"suspicious", 1.3},
		{"bad_actor", 1.5},
		{"unknown", 1.0},
	}

	for _, tt := range tests {
		t.Run(tt.level, func(t *testing.T) {
			rep := &ReputationScore{Level: tt.level}
			multiplier := de.calculateReputationMultiplier(rep)
			assert.Equal(t, tt.expected, multiplier)
		})
	}
}

// Helper function
func containsSubstring(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && (containsSubstring(s[1:], substr) || s[:len(substr)] == substr))
}

func init() {
	// Ensure time package is used (for any time-based tests)
	_ = time.Now()
}

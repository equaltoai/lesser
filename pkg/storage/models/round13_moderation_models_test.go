package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestModerationEvent_UpdateKeys_SetsCompositeKeysAndTTL(t *testing.T) {
	before := time.Now()
	event := &ModerationEvent{
		ID:         "evt-1",
		ObjectID:   "obj-1",
		ActorID:    "actor-1",
		EventType:  "report",
		Category:   "spam",
		Severity:   "high",
		Created:    time.Unix(1700000000, 0).UTC(),
		ConfidenceScore: 0.5,
	}
	err := event.UpdateKeys()
	after := time.Now()
	assert.NoError(t, err)

	assert.Equal(t, "EVENT#obj-1", event.PK)
	assert.Contains(t, event.SK, "TIME#")
	assert.Equal(t, "ACTOR#actor-1", event.GSI1PK)
	assert.Contains(t, event.GSI1SK, "TIME#")
	assert.Equal(t, "TYPE#report#spam", event.GSI2PK)
	assert.Contains(t, event.GSI2SK, "SEVERITY#high#")
	assert.Equal(t, "EVENTID#evt-1", event.GSI3PK)
	assert.Equal(t, "EVENTID#evt-1", event.GSI3SK)
	assert.Equal(t, "EVENT", event.Type)
	assert.True(t, event.TTL > 0)

	ttl := time.Unix(event.TTL, 0)
	assert.True(t, ttl.After(before.Add(30*24*time.Hour-5*time.Second)))
	assert.True(t, ttl.Before(after.Add(30*24*time.Hour+5*time.Second)))
}

func TestModerationReview_UpdateKeys_SetsKeysAndTTL(t *testing.T) {
	before := time.Now()
	created := time.Unix(1700000000, 0).UTC()
	review := &ModerationReview{
		ID:         "r1",
		EventID:    "evt-1",
		ReviewerID: "rev-1",
		Created:    created,
	}
	review.UpdateKeys()
	after := time.Now()

	assert.Equal(t, "REVIEW#evt-1", review.PK)
	assert.Equal(t, "REVIEWER#rev-1", review.SK)
	assert.Equal(t, "REVIEW", review.Type)
	assert.Equal(t, created, review.CreatedAt)
	assert.True(t, review.TTL > 0)

	ttl := time.Unix(review.TTL, 0)
	assert.True(t, ttl.After(before.Add(30*24*time.Hour-5*time.Second)))
	assert.True(t, ttl.Before(after.Add(30*24*time.Hour+5*time.Second)))
}

func TestModerationDecision_UpdateKeys_SetsKeysAndTTL(t *testing.T) {
	before := time.Now()
	decided := time.Unix(1700000000, 0).UTC()
	decision := &ModerationDecision{
		ID:       "d1",
		ObjectID: "obj-1",
		Decided:  decided,
	}
	decision.UpdateKeys()
	after := time.Now()

	assert.Equal(t, "DECISION#obj-1", decision.PK)
	assert.Contains(t, decision.SK, "TIME#")
	assert.Equal(t, "ACTIVE_DECISIONS", decision.GSI1PK)
	assert.Equal(t, "OBJECT#obj-1", decision.GSI1SK)
	assert.Equal(t, "DECISION", decision.Type)
	assert.Equal(t, decided, decision.CreatedAt)
	assert.True(t, decision.TTL > 0)

	ttl := time.Unix(decision.TTL, 0)
	assert.True(t, ttl.After(before.Add(90*24*time.Hour-5*time.Second)))
	assert.True(t, ttl.Before(after.Add(90*24*time.Hour+5*time.Second)))
}

func TestModerationPattern_UpdateKeys_ActiveAndInactive(t *testing.T) {
	before := time.Now()
	p := &ModerationPattern{
		PatternID:  "p1",
		Type:       "regex",
		Severity:   0.42,
		Active:     true,
		UpdatedAt:  time.Unix(1700000000, 0).UTC(),
		CreatedAt:  time.Unix(1700000000, 0).UTC(),
	}
	err := p.UpdateKeys()
	after := time.Now()
	assert.NoError(t, err)

	assert.Equal(t, "PATTERN#p1", p.PK)
	assert.Equal(t, SKMetadata, p.SK)
	assert.Equal(t, "MODERATION_PATTERNS#ACTIVE", p.GSI1PK)
	assert.Contains(t, p.GSI1SK, "regex#p1")
	assert.Equal(t, "MODERATION_PATTERNS#0.42", p.GSI2PK)
	assert.Contains(t, p.GSI2SK, "#p1")
	assert.True(t, p.TTL > 0)

	ttl := time.Unix(p.TTL, 0)
	assert.True(t, ttl.After(before.Add(90*24*time.Hour-5*time.Second)))
	assert.True(t, ttl.Before(after.Add(90*24*time.Hour+5*time.Second)))

	// Inactive clears GSI1 keys.
	p2 := &ModerationPattern{
		PatternID: "p2",
		Type:      "keyword",
		Severity:  0.10,
		Active:    false,
		UpdatedAt: time.Unix(1700000000, 0).UTC(),
	}
	assert.NoError(t, p2.UpdateKeys())
	assert.Equal(t, "", p2.GSI1PK)
	assert.Equal(t, "", p2.GSI1SK)
}

func TestModeration_HistoryAndHelpers(t *testing.T) {
	m := &Moderation{
		Evidence: ModerationEvidence{
			ViolationCount:   1,
			RequestCount:     10,
			ProhibitedWords:  []string{"bad", "worse"},
		},
	}

	m.AddHistoryEntry("actor-1", "user", ModerationActionWarning, ModerationStatusPending, ModerationStatusReviewing, "note")
	if assert.Len(t, m.History, 1) {
		assert.Equal(t, "actor-1", m.History[0].ActorID)
		assert.Equal(t, "user", m.History[0].ActorType)
		assert.Equal(t, ModerationActionWarning, m.History[0].Action)
		assert.Equal(t, ModerationStatusPending, m.History[0].FromStatus)
		assert.Equal(t, ModerationStatusReviewing, m.History[0].ToStatus)
		assert.Equal(t, "note", m.History[0].Note)
	}

	assert.Equal(t, "minor", m.GetRateLimitViolationSeverity())
	assert.Equal(t, "bad", m.GetPrimaryProhibitedWord())

	m.Evidence.ViolationCount = 6
	assert.Equal(t, "moderate", m.GetRateLimitViolationSeverity())

	m.Evidence.ViolationCount = 11
	assert.Equal(t, "severe", m.GetRateLimitViolationSeverity())

	m.Evidence.ProhibitedWords = nil
	assert.Equal(t, "", m.GetPrimaryProhibitedWord())
}

func TestModeration_AnalysisDecisionQueueAndAuditKeys(t *testing.T) {
	before := time.Now()
	analyzed := time.Unix(1700000000, 0).UTC()
	ar := &ModerationAnalysisResult{
		ID:           "a1",
		ContentID:    "c1",
		AuthorID:     "actor-1",
		AnalysisType: "text",
		Confidence:   0.87,
		AnalyzedAt:   analyzed,
	}
	ar.UpdateKeys()
	after := time.Now()

	assert.Equal(t, "ANALYSIS#c1", ar.PK)
	assert.Contains(t, ar.SK, "RESULT#")
	assert.Equal(t, "ACTOR#actor-1", ar.GSI1PK)
	assert.Contains(t, ar.GSI2PK, "ANALYSIS_TYPE#text")
	assert.Equal(t, "ANALYSIS_RESULT", ar.Type)
	assert.Equal(t, analyzed, ar.CreatedAt)
	assert.True(t, ar.TTL > 0)

	ttl := time.Unix(ar.TTL, 0)
	assert.True(t, ttl.After(before.Add(90*24*time.Hour-5*time.Second)))
	assert.True(t, ttl.Before(after.Add(90*24*time.Hour+5*time.Second)))

	dr := &ModerationDecisionResult{
		ID:                "d1",
		ContentID:          "c1",
		Action:            "flag",
		Confidence:        0.55,
		DecidedAt:         analyzed,
		EnforcementStatus: "pending",
	}
	dr.UpdateKeys()
	assert.Equal(t, "DECISION_RESULT#c1", dr.PK)
	assert.Equal(t, "ACTIVE_DECISIONS", dr.GSI1PK)
	assert.Equal(t, "CONTENT#c1", dr.GSI1SK)
	assert.Equal(t, "ACTION#flag", dr.GSI2PK)
	assert.Equal(t, "DECISION_RESULT", dr.Type)

	// Non-active enforcement clears GSI1.
	dr2 := &ModerationDecisionResult{
		ID:                "d2",
		ContentID:          "c2",
		Action:            "allow",
		Confidence:        0.10,
		DecidedAt:         analyzed,
		EnforcementStatus: "expired",
	}
	dr2.UpdateKeys()
	assert.Equal(t, "", dr2.GSI1PK)
	assert.Equal(t, "", dr2.GSI1SK)

	rq := &ModerationReviewQueue{
		ID:        "q1",
		ContentID:  "c1",
		Status:    "pending",
		Priority:  3,
		CreatedAt: analyzed,
	}
	rq.UpdateKeys()
	assert.Equal(t, "REVIEW_QUEUE#pending", rq.PK)
	assert.Contains(t, rq.SK, "PRIORITY#")
	assert.Equal(t, "QUEUE_CONTENT#c1", rq.GSI1PK)
	assert.Equal(t, "STATUS#pending", rq.GSI1SK)
	assert.Equal(t, "", rq.GSI2PK)
	assert.Equal(t, "", rq.GSI2SK)
	assert.Equal(t, "REVIEW_QUEUE", rq.Type)
	assert.True(t, rq.TTL > 0)

	// Assigned sets GSI2.
	rq2 := &ModerationReviewQueue{
		ID:         "q2",
		ContentID:   "c2",
		Status:     "reviewing",
		Priority:   10,
		CreatedAt:  analyzed,
		AssignedTo: "mod-1",
	}
	rq2.UpdateKeys()
	assert.Equal(t, "ASSIGNEE#mod-1", rq2.GSI2PK)
	assert.Contains(t, rq2.GSI2SK, "PRIORITY#")

	al := &AuditLog{
		ID:        "log1",
		AdminID:   "admin1",
		Action:    "suspend",
		TargetID:  "user1",
		Timestamp: analyzed,
	}
	al.UpdateKeys()
	assert.Equal(t, "AUDIT_LOG", al.PK)
	assert.Contains(t, al.SK, "TIME#")
	assert.Equal(t, "ADMIN#admin1", al.GSI1PK)
	assert.Equal(t, "TARGET#user1", al.GSI2PK)
	assert.Equal(t, "AUDIT_LOG", al.Type)
	assert.Equal(t, analyzed, al.CreatedAt)
	assert.True(t, al.TTL > 0)
}


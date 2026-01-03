package main

import (
	"context"
	"testing"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/equaltoai/lesser/pkg/moderation"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestModerationProcessor_ConversionHelpers(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)

	require.Empty(t, getStringFromMap(map[string]any{}, "missing"))
	require.Equal(t, "value", getStringFromMap(map[string]any{"k": "value"}, "k"))
	require.Empty(t, getStringFromMap(map[string]any{"k": 123}, "k"))

	require.Equal(t, 0.0, getFloatFromMap(map[string]any{}, "missing"))
	require.Equal(t, 1.25, getFloatFromMap(map[string]any{"k": 1.25}, "k"))
	require.Equal(t, 3.0, getFloatFromMap(map[string]any{"k": 3}, "k"))

	require.Nil(t, getMapFromMap(map[string]any{}, "missing"))
	require.Equal(t, map[string]any{"x": "y"}, getMapFromMap(map[string]any{"k": map[string]any{"x": "y"}}, "k"))
	require.Nil(t, getMapFromMap(map[string]any{"k": "nope"}, "k"))

	require.True(t, getTimeFromMap(map[string]any{"k": now}, "k").Equal(now))
	require.True(t, getTimeFromMap(map[string]any{"k": now.Format(time.RFC3339)}, "k").Equal(now))
	require.True(t, getTimeFromMap(map[string]any{"k": "not-a-time"}, "k").IsZero())
	require.True(t, getTimeFromMap(map[string]any{}, "missing").IsZero())

	require.Equal(t, moderation.SeverityLow, parseSeverity("low"))
	require.Equal(t, moderation.SeverityLow, parseSeverity("1"))
	require.Equal(t, moderation.SeverityMedium, parseSeverity("medium"))
	require.Equal(t, moderation.SeverityMedium, parseSeverity("2"))
	require.Equal(t, moderation.SeverityHigh, parseSeverity("high"))
	require.Equal(t, moderation.SeverityHigh, parseSeverity("3"))
	require.Equal(t, moderation.SeverityCritical, parseSeverity("critical"))
	require.Equal(t, moderation.SeverityCritical, parseSeverity("4"))
	require.Equal(t, moderation.SeverityMedium, parseSeverity("unknown"))

	require.Equal(t, "low", severityToString(moderation.SeverityLow))
	require.Equal(t, "medium", severityToString(moderation.SeverityMedium))
	require.Equal(t, "high", severityToString(moderation.SeverityHigh))
	require.Equal(t, "critical", severityToString(moderation.SeverityCritical))
	require.Equal(t, "medium", severityToString(moderation.Severity(99)))

	require.Equal(t, moderation.CategorySpam, parseCategoryFromString("spam"))
	require.Equal(t, moderation.CategoryHateSpeech, parseCategoryFromString("hate_speech"))
	require.Equal(t, moderation.CategoryHarassment, parseCategoryFromString("harassment"))
	require.Equal(t, moderation.CategoryMisinformation, parseCategoryFromString("misinformation"))
	require.Equal(t, moderation.CategoryNSFW, parseCategoryFromString("nsfw"))
	require.Equal(t, moderation.CategoryViolence, parseCategoryFromString("violence"))
	require.Equal(t, moderation.CategoryOther, parseCategoryFromString("something_else"))
}

func TestModerationProcessor_ConversionsBetweenStorageAndDomainTypes(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)

	require.Nil(t, convertStorageToModerationEvent(nil))
	storageEvent := &storage.ModerationEvent{
		ID:         "evt-1",
		EventType:  "flagged",
		ObjectID:   "obj-1",
		ObjectType: "status",
		ActorID:    "actor-1",
		Category:   "spam",
		Severity:   "high",
		ConfidenceScore: 0.9,
		Evidence: []any{
			map[string]any{
				"type":        "keyword",
				"score":       0.75,
				"description": "matched",
				"metadata":    map[string]any{"k": "v"},
				"timestamp":   now.Format(time.RFC3339),
			},
			"ignored",
			map[string]any{"score": 2}, // int score path
		},
		Reason:  "reported",
		Created: now,
		Updated: now,
		TTL:     123,
	}

	event := convertStorageToModerationEvent(storageEvent)
	require.NotNil(t, event)
	require.Equal(t, "evt-1", event.ID)
	require.Equal(t, moderation.Category("spam"), event.Category)
	require.Equal(t, moderation.SeverityHigh, event.Severity)
	require.Len(t, event.Evidence, 2)
	require.Equal(t, "keyword", event.Evidence[0].Type)
	require.Equal(t, 0.75, event.Evidence[0].Score)
	require.Equal(t, map[string]any{"k": "v"}, event.Evidence[0].Metadata)

	require.Nil(t, convertStorageToModerationReview(nil))
	storageReview := &storage.ModerationReview{
		ID:          "rev-1",
		EventID:     "evt-1",
		ReviewerID:  "mod-1",
		ReviewerRep: 2.5,
		Action:      "remove",
		Severity:    "spam",
		Note:        "note",
		Confidence:  0.8,
		Created:     now,
	}
	review := convertStorageToModerationReview(storageReview)
	require.Equal(t, "rev-1", review.ID)
	require.Equal(t, moderation.ActionType("remove"), review.Action)
	require.Equal(t, moderation.CategorySpam, review.Category)
	require.Equal(t, moderation.SeverityMedium, review.Severity) // spam severity string defaults to medium

	require.Nil(t, convertModerationToStorageReview(nil))
	reviewIn := &moderation.Review{
		ID:         "rev-2",
		EventID:    "evt-2",
		ReviewerID: "mod-2",
		Action:     moderation.ActionTypeSuspend,
		Severity:   moderation.SeverityCritical,
		Confidence: 0.9,
		Notes:      "notes",
		Weight:     3.0,
		Created:    now,
	}
	storedReview := convertModerationToStorageReview(reviewIn)
	require.Equal(t, "rev-2", storedReview.ID)
	require.Equal(t, "suspend", storedReview.Action)
	require.Equal(t, "critical", storedReview.Severity)

	require.Nil(t, convertModerationToStorageDecision(nil))
	appliedAt := now.Add(1 * time.Hour)
	decisionIn := &moderation.ModerationDecision{
		ID:             "dec-1",
		EventID:        "evt-1",
		ObjectID:       "obj-1",
		Action:         moderation.ActionTypeRemove,
		ConsensusScore: 0.99,
		ReviewerCount:  3,
		Reviews:        []*moderation.Review{reviewIn},
		Decided:        now,
		AppliedAt:      &appliedAt,
	}
	storedDecision := convertModerationToStorageDecision(decisionIn)
	require.Equal(t, "dec-1", storedDecision.ID)
	require.Equal(t, "remove", storedDecision.Action)
	require.NotNil(t, storedDecision.Expires)
	require.True(t, storedDecision.Expires.Equal(appliedAt))
	require.Len(t, storedDecision.Reviews, 1)

	require.Nil(t, convertStorageToModerationQueueItem(nil))
	reviewedAt := now
	queueItem := convertStorageToModerationQueueItem(&storage.ModerationQueueItem{
		Event:       storageEvent,
		Priority:    5,
		ReviewCount: 2,
		ReviewedAt:  &reviewedAt,
	})
	require.NotNil(t, queueItem)
	require.Equal(t, float64(5), queueItem.Priority)
	require.Equal(t, 2, queueItem.ReviewCount)
}

func TestModerationProcessor_RecordParsing(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)

	t.Run("review record", func(t *testing.T) {
		record := events.DynamoDBEventRecord{
			Change: events.DynamoDBStreamRecord{
				NewImage: map[string]events.DynamoDBAttributeValue{
					"Type":    events.NewStringAttribute("REVIEW"),
					"Action":  events.NewStringAttribute("remove"),
					"Weight":  events.NewNumberAttribute("2.5"),
					"Created": events.NewStringAttribute(now.Format(time.RFC3339)),
				},
				Keys: map[string]events.DynamoDBAttributeValue{
					"PK": events.NewStringAttribute("REVIEW#evt-1"),
					"SK": events.NewStringAttribute("REVIEWER#mod-1"),
				},
			},
		}

		review, err := getReviewFromRecord(record)
		require.NoError(t, err)
		require.Equal(t, "evt-1", review.EventID)
		require.Equal(t, "mod-1", review.ReviewerID)
		require.Equal(t, moderation.ActionType("remove"), review.Action)
		require.Equal(t, 2.5, review.Weight)
		require.True(t, review.Created.Equal(now))
	})

	t.Run("review wrong type", func(t *testing.T) {
		_, err := getReviewFromRecord(events.DynamoDBEventRecord{Change: events.DynamoDBStreamRecord{
			NewImage: map[string]events.DynamoDBAttributeValue{
				"Type": events.NewStringAttribute("EVENT"),
			},
		}})
		require.Error(t, err)
	})

	t.Run("event record", func(t *testing.T) {
		record := events.DynamoDBEventRecord{
			Change: events.DynamoDBStreamRecord{
				NewImage: map[string]events.DynamoDBAttributeValue{
					"Type":      events.NewStringAttribute("EVENT"),
					"ID":        events.NewStringAttribute("evt-2"),
					"ActorID":   events.NewStringAttribute("actor-2"),
					"EventType": events.NewStringAttribute("flagged"),
					"Category":  events.NewStringAttribute("nsfw"),
					"Severity":  events.NewNumberAttribute("9"),
				},
				Keys: map[string]events.DynamoDBAttributeValue{
					"PK": events.NewStringAttribute("EVENT#obj-2"),
				},
			},
		}

		event, err := getEventFromRecord(record)
		require.NoError(t, err)
		require.Equal(t, "evt-2", event.ID)
		require.Equal(t, "obj-2", event.ObjectID)
		require.Equal(t, moderation.Category("nsfw"), event.Category)
		require.Equal(t, moderation.Severity(9), event.Severity)
	})

	t.Run("decision record", func(t *testing.T) {
		record := events.DynamoDBEventRecord{
			Change: events.DynamoDBStreamRecord{
				NewImage: map[string]events.DynamoDBAttributeValue{
					"Type":           events.NewStringAttribute("DECISION"),
					"ID":             events.NewStringAttribute("dec-2"),
					"EventID":        events.NewStringAttribute("evt-2"),
					"Action":         events.NewStringAttribute("suspend"),
					"Reason":         events.NewStringAttribute("reason"),
					"ConsensusScore": events.NewNumberAttribute("0.8"),
				},
				Keys: map[string]events.DynamoDBAttributeValue{
					"PK": events.NewStringAttribute("DECISION#obj-2"),
				},
			},
		}

		decision, err := getDecisionFromRecord(record)
		require.NoError(t, err)
		require.Equal(t, "dec-2", decision.ID)
		require.Equal(t, "evt-2", decision.EventID)
		require.Equal(t, "obj-2", decision.ObjectID)
		require.Equal(t, moderation.ActionType("suspend"), decision.Action)
		require.Equal(t, 0.8, decision.ConsensusScore)
	})
}

func TestModeratorSelector_SelectionAndScoring(t *testing.T) {
	ms := NewModeratorSelector(nil, nil, zap.NewNop())

	t.Run("availability", func(t *testing.T) {
		require.False(t, ms.isModeratorAvailable(&storage.User{Suspended: true, Approved: true}))
		require.False(t, ms.isModeratorAvailable(&storage.User{Suspended: false, Approved: false}))
		require.True(t, ms.isModeratorAvailable(&storage.User{Suspended: false, Approved: true}))
	})

	t.Run("round robin", func(t *testing.T) {
		mods := []*storage.User{
			{Username: "m1"},
			{Username: "m2"},
			{Username: "m3"},
		}

		event := &moderation.ModerationEvent{ID: "evt", Severity: 7}
		selected := ms.selectRoundRobin(mods, event)
		require.Len(t, selected, 2)
		require.Equal(t, "m1", selected[0].Username)
		require.Equal(t, "m2", selected[1].Username)

		// Next call should continue from the next index.
		selected = ms.selectRoundRobin(mods, event)
		require.Len(t, selected, 2)
		require.Equal(t, "m3", selected[0].Username)
		require.Equal(t, "m1", selected[1].Username)
	})

	t.Run("random deterministic", func(t *testing.T) {
		mods := []*storage.User{
			{Username: "m1"},
			{Username: "m2"},
			{Username: "m3"},
		}
		event := &moderation.ModerationEvent{ID: "evt-hash", Severity: 9}
		selected := ms.selectRandom(mods, event)
		require.Len(t, selected, 3)
	})

	t.Run("role weight", func(t *testing.T) {
		require.Equal(t, 3.0, ms.calculateRoleWeight(&storage.User{Role: adminRole}, moderation.SeverityCritical))
		require.Equal(t, 2.5, ms.calculateRoleWeight(&storage.User{Role: adminRole}, moderation.SeverityHigh))
		require.Equal(t, 2.0, ms.calculateRoleWeight(&storage.User{Role: adminRole}, moderation.SeverityLow))
		require.Equal(t, 1.8, ms.calculateRoleWeight(&storage.User{Role: "moderator"}, moderation.SeverityHigh))
		require.Equal(t, 1.5, ms.calculateRoleWeight(&storage.User{Role: "moderator"}, moderation.SeverityLow))
		require.Equal(t, 1.0, ms.calculateRoleWeight(&storage.User{Role: "user"}, moderation.SeverityLow))
	})

	t.Run("expertise-based selection", func(t *testing.T) {
		now := time.Now()
		older := now.Add(-400 * 24 * time.Hour)
		newer := now.Add(-30 * 24 * time.Hour)

		mods := []*storage.User{
			{Username: "admin", Role: adminRole, CreatedAt: older, Approved: true},
			{Username: "mod", Role: "moderator", CreatedAt: newer, Approved: true},
			{Username: "inactive", Role: "moderator", CreatedAt: older, Approved: false},
		}
		event := &moderation.ModerationEvent{ID: "evt", Category: moderation.CategoryNSFW, Severity: moderation.SeverityCritical}
		selected := ms.selectByExpertise(mods, event)
		require.NotEmpty(t, selected)
	})

	t.Run("review history analysis", func(t *testing.T) {
		stats := ms.analyzeReviewHistory([]*models.ModerationReview{
			{Tags: []string{"spam"}, Action: "remove", Severity: "low", Confidence: 0.9},
			{Tags: []string{"spam"}, Action: "remove", Severity: "low", Confidence: 0.9},
			{Tags: []string{"spam"}, Action: "remove", Severity: "low", Confidence: 0.9},
			nil,
		}, "spam")
		require.Equal(t, 3, stats.categoryCount)
		require.True(t, ms.isSuccessfulReview(&models.ModerationReview{Confidence: 0.8, Action: "remove"}))
		require.False(t, ms.isSuccessfulReview(&models.ModerationReview{Confidence: 0.2, Action: "remove"}))
		require.False(t, ms.isSuccessfulReview(&models.ModerationReview{Confidence: 0.8, Action: "none"}))
		require.True(t, ms.evaluateExperience(&moderationStats{categoryCount: 5}))
		require.True(t, ms.evaluateExperience(&moderationStats{totalReviews: 20, categoryCount: 1}))
		require.True(t, ms.evaluateExperience(&moderationStats{categoryCount: 3, successfulReviews: 3}))
		require.False(t, ms.evaluateExperience(&moderationStats{categoryCount: 2, successfulReviews: 2, totalReviews: 2}))
	})

	t.Run("severity/action category matching", func(t *testing.T) {
		require.True(t, ms.matchesBySeverityAction(&models.ModerationReview{Action: "remove", Severity: "low"}, "spam"))
		require.True(t, ms.matchesBySeverityAction(&models.ModerationReview{Action: "suspend", Severity: "critical"}, "hate_speech"))
		require.True(t, ms.matchesBySeverityAction(&models.ModerationReview{Action: "silence", Severity: "high"}, "harassment"))
		require.True(t, ms.matchesBySeverityAction(&models.ModerationReview{Action: "warning", Severity: "medium"}, "misinformation"))
		require.True(t, ms.matchesBySeverityAction(&models.ModerationReview{Action: "suspend", Severity: "critical"}, "violence"))
		require.True(t, ms.matchesBySeverityAction(&models.ModerationReview{Action: "warning", Severity: "low"}, "nsfw"))
		require.False(t, ms.matchesBySeverityAction(&models.ModerationReview{Action: "remove", Severity: "low"}, "unknown"))
	})

	t.Run("logExperienceCheck is safe", func(t *testing.T) {
		ms.logExperienceCheck("alice", "spam", &moderationStats{categoryCount: 1, totalReviews: 2, successfulReviews: 1}, true)
	})
}

func TestModerationProcessor_NotificationHelpers(t *testing.T) {
	mp := &ModerationProcessor{logger: zap.NewNop()}

	require.Equal(t, StrategyExpertiseBased, mp.getAssignmentStrategy(&moderation.ModerationEvent{Severity: 8}))
	require.Equal(t, StrategyWorkloadBased, mp.getAssignmentStrategy(&moderation.ModerationEvent{Severity: 3}))

	require.Equal(t, "critical", mp.getPriorityString(9))
	require.Equal(t, "high", mp.getPriorityString(7))
	require.Equal(t, "normal", mp.getPriorityString(5))
	require.Equal(t, "low", mp.getPriorityString(1))

	start := time.Now()
	deadline := mp.calculateDeadline(9)
	require.True(t, deadline.After(start.Add(14*time.Minute)))
	require.True(t, deadline.Before(start.Add(16*time.Minute)))

	title := mp.getNotificationTitle(&moderation.ModerationEvent{Severity: 1})
	require.NotEmpty(t, title)

	body := mp.getNotificationBody(
		&moderation.ModerationEvent{ID: "evt-1", Category: moderation.CategorySpam, Severity: 5},
		&ModerationAssignment{Priority: "normal", Deadline: time.Unix(0, 0).UTC(), Strategy: string(StrategyRoundRobin)},
	)
	require.Contains(t, body, "evt-1")
	require.Contains(t, body, "spam")
}

func TestModerationProcessor_FilteringAndStubs_DoNotError(t *testing.T) {
	ctx := context.Background()
	mp := &ModerationProcessor{logger: zap.NewNop()}

	require.NoError(t, mp.filterFromTimelines(ctx, "alice", "silence"))
	require.NoError(t, mp.filterFromTimelines(ctx, "alice", actionSuspend))
	require.NoError(t, mp.updateSearchVisibility(ctx, "alice", "hidden"))
	require.NoError(t, mp.applyFederationConstraints(ctx, "alice", "silence"))
	require.NoError(t, mp.removeFromTimelines(ctx, "obj-1"))
	require.NoError(t, mp.removeFromSearch(ctx, "obj-1"))
	require.NoError(t, mp.sendTimelineUpdateEvent(ctx, "alice", "silence"))

	// Coverage for the early-return branch that avoids repository calls.
	require.NoError(t, mp.triggerAutomaticActions(ctx, &moderation.ModerationEvent{Severity: 1}))
}

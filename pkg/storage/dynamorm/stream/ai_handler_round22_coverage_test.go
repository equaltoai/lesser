package stream

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/equaltoai/lesser/pkg/ai"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type fakeModerationRepo struct {
	flags     []*storage.Flag
	events    []*storage.ModerationEvent
	decisions []*storage.ModerationDecision

	createFlagErr               error
	createModerationEventErr    error
	createModerationDecisionErr error
}

func (f *fakeModerationRepo) CreateFlag(_ context.Context, flag *storage.Flag) error {
	f.flags = append(f.flags, flag)
	return f.createFlagErr
}

func (f *fakeModerationRepo) CreateModerationEvent(_ context.Context, event *storage.ModerationEvent) error {
	f.events = append(f.events, event)
	return f.createModerationEventErr
}

func (f *fakeModerationRepo) CreateModerationDecision(_ context.Context, decision *storage.ModerationDecision) error {
	f.decisions = append(f.decisions, decision)
	return f.createModerationDecisionErr
}

func TestAIAnalysisStreamHandler_isAIAnalysisRecord_Round22(t *testing.T) {
	h := NewAIAnalysisStreamHandler(zap.NewNop(), &fakeModerationRepo{})

	rec := events.DynamoDBEventRecord{
		EventName: eventNameInsert,
		Change: events.DynamoDBStreamRecord{
			NewImage: map[string]events.DynamoDBAttributeValue{
				"PK": events.NewStringAttribute("AI#obj-1"),
			},
		},
	}
	require.True(t, h.isAIAnalysisRecord(rec))

	rec.Change.NewImage["PK"] = events.NewStringAttribute("USER#obj-1")
	require.False(t, h.isAIAnalysisRecord(rec))

	rec.Change.NewImage["PK"] = events.NewStringAttribute("AI#ANALYSIS#2024-01-01")
	require.False(t, h.isAIAnalysisRecord(rec))

	rec.EventName = eventNameRemove
	rec.Change = events.DynamoDBStreamRecord{
		OldImage: map[string]events.DynamoDBAttributeValue{
			"PK": events.NewStringAttribute("AI#obj-2"),
		},
	}
	require.True(t, h.isAIAnalysisRecord(rec))
}

func TestAIAnalysisStreamHandler_HandleStreamEvent_SkipsNonAI_Round22(t *testing.T) {
	repo := &fakeModerationRepo{}
	h := NewAIAnalysisStreamHandler(zap.NewNop(), repo)

	rec := events.DynamoDBEventRecord{
		EventID:   "evt",
		EventName: eventNameInsert,
		Change: events.DynamoDBStreamRecord{
			NewImage: map[string]events.DynamoDBAttributeValue{
				"PK": events.NewStringAttribute("USER#u1"),
			},
		},
	}

	require.NoError(t, h.HandleStreamEvent(context.Background(), rec))
	require.Empty(t, repo.events)
	require.Empty(t, repo.flags)
	require.Empty(t, repo.decisions)
}

func TestAIAnalysisStreamHandler_HandleStreamEvent_UnknownEvent_Round22(t *testing.T) {
	repo := &fakeModerationRepo{}
	h := NewAIAnalysisStreamHandler(zap.NewNop(), repo)

	rec := events.DynamoDBEventRecord{
		EventID:   "evt",
		EventName: "BOGUS",
		Change: events.DynamoDBStreamRecord{
			NewImage: map[string]events.DynamoDBAttributeValue{
				"PK": events.NewStringAttribute("AI#obj-1"),
			},
		},
	}

	require.NoError(t, h.HandleStreamEvent(context.Background(), rec))
	require.Empty(t, repo.events)
}

func TestAIAnalysisStreamHandler_HandleStreamEvent_Insert_HighRiskAutoRemove_Round22(t *testing.T) {
	repo := &fakeModerationRepo{}
	h := NewAIAnalysisStreamHandler(zap.NewNop(), repo)

	now := time.Date(2024, 6, 15, 14, 30, 0, 0, time.UTC)

	rec := events.DynamoDBEventRecord{
		EventID:   "evt",
		EventName: eventNameInsert,
		Change: events.DynamoDBStreamRecord{
			NewImage: map[string]events.DynamoDBAttributeValue{
				"PK":               events.NewStringAttribute("AI#obj-1"),
				"SK":               events.NewStringAttribute("ANALYSIS#analysis-1"),
				"id":               events.NewStringAttribute("analysis-1"),
				"objectID":         events.NewStringAttribute("obj-1"),
				"objectType":       events.NewStringAttribute("note"),
				"analyzedAt":       events.NewStringAttribute(now.Format(time.RFC3339Nano)),
				"version":          events.NewStringAttribute("v1"),
				"overallRisk":      events.NewNumberAttribute("0.96"),
				"moderationAction": events.NewStringAttribute("remove"),
				"confidence":       events.NewNumberAttribute("0.90"),
				"type":             events.NewStringAttribute("AIAnalysis"),
				"createdAt":        events.NewStringAttribute(now.Format(time.RFC3339Nano)),
				"updatedAt":        events.NewStringAttribute(now.Format(time.RFC3339Nano)),
			},
		},
	}

	require.NoError(t, h.HandleStreamEvent(context.Background(), rec))
	require.Len(t, repo.events, 1)
	require.Len(t, repo.decisions, 1)
	require.Empty(t, repo.flags)
}

func TestAIAnalysisStreamHandler_HandleStreamEvent_Remove_NoSideEffects_Round22(t *testing.T) {
	repo := &fakeModerationRepo{}
	h := NewAIAnalysisStreamHandler(zap.NewNop(), repo)

	rec := events.DynamoDBEventRecord{
		EventID:   "evt",
		EventName: eventNameRemove,
		Change: events.DynamoDBStreamRecord{
			OldImage: map[string]events.DynamoDBAttributeValue{
				"PK": events.NewStringAttribute("AI#obj-1"),
			},
		},
	}

	require.NoError(t, h.HandleStreamEvent(context.Background(), rec))
	require.Empty(t, repo.events)
	require.Empty(t, repo.flags)
	require.Empty(t, repo.decisions)
}

func TestAIAnalysisStreamHandler_applyModerationAction_Round22_ActionSelection(t *testing.T) {
	t.Run("none", func(t *testing.T) {
		repo := &fakeModerationRepo{}
		h := NewAIAnalysisStreamHandler(zap.NewNop(), repo)

		analysis := &models.AIAnalysis{
			ID:          "a1",
			ObjectID:    "o1",
			ObjectType:  "note",
			OverallRisk: 0.10,
			Confidence:  0.90,
		}
		require.NoError(t, h.applyModerationAction(context.Background(), analysis))
		require.Empty(t, repo.events)
	})

	t.Run("flag", func(t *testing.T) {
		repo := &fakeModerationRepo{}
		h := NewAIAnalysisStreamHandler(zap.NewNop(), repo)

		analysis := &models.AIAnalysis{
			ID:          "a1",
			ObjectID:    "o1",
			ObjectType:  "note",
			OverallRisk: ai.GetThresholds("note").Flag + 0.01,
			Confidence:  0.90,
		}
		require.NoError(t, h.applyModerationAction(context.Background(), analysis))
		require.Len(t, repo.events, 1)
		require.Len(t, repo.flags, 1)
	})

	t.Run("review", func(t *testing.T) {
		repo := &fakeModerationRepo{}
		h := NewAIAnalysisStreamHandler(zap.NewNop(), repo)

		analysis := &models.AIAnalysis{
			ID:          "a1",
			ObjectID:    "o1",
			ObjectType:  "note",
			OverallRisk: ai.GetThresholds("note").WarnAuthor + 0.01,
			Confidence:  0.90,
		}
		require.NoError(t, h.applyModerationAction(context.Background(), analysis))
		require.Len(t, repo.events, 1)
		require.Len(t, repo.flags, 1)
	})

	t.Run("hide", func(t *testing.T) {
		repo := &fakeModerationRepo{}
		h := NewAIAnalysisStreamHandler(zap.NewNop(), repo)

		analysis := &models.AIAnalysis{
			ID:          "a1",
			ObjectID:    "o1",
			ObjectType:  "note",
			OverallRisk: ai.GetThresholds("note").AutoHide + 0.01,
			Confidence:  0.90,
		}
		require.NoError(t, h.applyModerationAction(context.Background(), analysis))
		require.Len(t, repo.events, 1)
		require.Len(t, repo.decisions, 1)
	})

	t.Run("remove", func(t *testing.T) {
		repo := &fakeModerationRepo{}
		h := NewAIAnalysisStreamHandler(zap.NewNop(), repo)

		analysis := &models.AIAnalysis{
			ID:          "a1",
			ObjectID:    "o1",
			ObjectType:  "note",
			OverallRisk: ai.GetThresholds("note").AutoRemove + 0.01,
			Confidence:  0.90,
		}
		require.NoError(t, h.applyModerationAction(context.Background(), analysis))
		require.Len(t, repo.events, 1)
		require.Len(t, repo.decisions, 1)
	})

	t.Run("create event fails", func(t *testing.T) {
		repo := &fakeModerationRepo{createModerationEventErr: errors.New("db down")}
		h := NewAIAnalysisStreamHandler(zap.NewNop(), repo)

		analysis := &models.AIAnalysis{
			ID:          "a1",
			ObjectID:    "o1",
			ObjectType:  "note",
			OverallRisk: ai.GetThresholds("note").AutoRemove + 0.01,
			Confidence:  0.90,
		}
		require.Error(t, h.applyModerationAction(context.Background(), analysis))
	})
}

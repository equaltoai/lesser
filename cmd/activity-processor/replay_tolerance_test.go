package main

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/v3"
	"github.com/theory-cloud/tabletheory/v3/pkg/core"
	"github.com/theory-cloud/tabletheory/v3/pkg/session"
	"github.com/theory-cloud/tabletheory/v3/pkg/testing/fakedb"
	"go.uber.org/zap"
)

type activityReplayInput struct {
	PK        string `theorydb:"pk"`
	SK        string `theorydb:"sk"`
	Type      string `json:"type"`
	Activity  string `json:"activity"`
	Direction string `json:"direction"`
	Username  string `json:"username"`
	ActorID   string `json:"actor_id"`
	CreatedAt string `json:"created_at"`
}

func newActivityReplayProcessor(t *testing.T, tableModel any) (*ActivityProcessor, core.DB, *fakedb.Fake) {
	t.Helper()
	fake := fakedb.New()
	db, err := tabletheory.NewWithClient(session.Config{Region: "us-east-1"}, fake)
	require.NoError(t, err)
	require.NoError(t, db.CreateTable(tableModel))
	return &ActivityProcessor{db: db, tableName: models.MainTableName, logger: zap.NewNop()}, db, fake
}

func TestActivityProcessor_ReplayedReceiptsRemainSuccessful(t *testing.T) {
	for _, direction := range []string{"inbox", "outbox"} {
		direction := direction
		t.Run(direction, func(t *testing.T) {
			processor, _, fake := newActivityReplayProcessor(t, &activityProcessingRecord{})
			input := activityReplayInput{
				PK:        "ACTIVITY#1",
				SK:        "METADATA",
				Type:      "Update",
				Direction: direction,
				Username:  "alice",
				ActorID:   "https://remote.example/users/alice",
				Activity:  `{"@context":"https://www.w3.org/ns/activitystreams","id":"https://remote.example/activities/1","type":"Update","actor":"https://remote.example/users/alice","object":"https://remote.example/objects/1"}`,
			}

			var first, second error
			if direction == "inbox" {
				first = processor.processInboxActivity(context.Background(), input)
				second = processor.processInboxActivity(context.Background(), input)
			} else {
				first = processor.processOutboxActivity(context.Background(), input)
				second = processor.processOutboxActivity(context.Background(), input)
			}
			require.NoError(t, first)
			require.NoError(t, second, "a duplicate stream record must accept the existing receipt")
			require.Len(t, fake.Items(models.MainTableName), 1)
		})
	}
}

func TestActivityProcessor_ReplayedMetricsCleanupAndTombstone(t *testing.T) {
	input := activityReplayInput{
		PK:        "ACTIVITY#2",
		SK:        "METADATA",
		Direction: "inbox",
		Username:  "alice",
		ActorID:   "https://remote.example/users/alice",
	}

	t.Run("activity metrics refresh", func(t *testing.T) {
		processor, _, fake := newActivityReplayProcessor(t, &activityMetricsRecord{})
		require.NoError(t, processor.updateActivityMetrics(context.Background(), input))
		require.NoError(t, processor.updateActivityMetrics(context.Background(), input))
		require.Len(t, fake.Items(models.MainTableName), 1)
	})

	t.Run("cleanup receipt tolerates duplicate", func(t *testing.T) {
		processor, _, fake := newActivityReplayProcessor(t, &activityCleanupRecord{})
		require.NoError(t, processor.cleanupActivityReferences(context.Background(), input))
		require.NoError(t, processor.cleanupActivityReferences(context.Background(), input))
		require.Len(t, fake.Items(models.MainTableName), 1)
	})

	t.Run("delete tombstone tolerates duplicate", func(t *testing.T) {
		processor, _, fake := newActivityReplayProcessor(t, &activityTombstoneRecord{})
		require.NoError(t, processor.createTombstone(context.Background(), "object-1", input.ActorID, "deleted"))
		require.NoError(t, processor.createTombstone(context.Background(), "object-1", input.ActorID, "deleted"))
		require.Len(t, fake.Items(models.MainTableName), 1)
	})

	t.Run("same-second metric refreshes", func(t *testing.T) {
		processor, db, fake := newActivityReplayProcessor(t, &activityProcessorMetricRecord{})
		originalNow := activityMetricNow
		fixedNow := time.Date(2026, time.August, 20, 12, 0, 0, 0, time.UTC)
		activityMetricNow = func() time.Time { return fixedNow }
		t.Cleanup(func() { activityMetricNow = originalNow })

		processor.recordMetric(context.Background(), "FANOUT", "FanoutMetric", "Create", time.Hour,
			map[string]interface{}{"entry_count": 1}, nil)
		processor.recordMetric(context.Background(), "FANOUT", "FanoutMetric", "Create", time.Hour,
			map[string]interface{}{"entry_count": 2}, nil)

		var persisted activityProcessorMetricRecord
		require.NoError(t, db.Model(&activityProcessorMetricRecord{}).
			Where("PK", "=", "FANOUT#METRICS").
			Where("SK", "=", "METRIC#1787227200#Create").
			First(&persisted))
		require.Equal(t, 2, persisted.EntryCount)
		require.Len(t, fake.Items(models.MainTableName), 1)
	})
}

func TestActivityHandler_ReplayedCreateContinuesPastExistingObject(t *testing.T) {
	fake := fakedb.New()
	db, err := tabletheory.NewWithClient(session.Config{Region: "us-east-1"}, fake)
	require.NoError(t, err)
	require.NoError(t, db.CreateTable(&models.Object{}))

	handler := &ActivityHandler{
		DB:         db,
		TableName:  models.MainTableName,
		Logger:     zap.NewNop(),
		ObjectRepo: repositories.NewObjectRepository(db, models.MainTableName, "example.com", zap.NewNop()),
	}
	published := time.Date(2026, time.August, 20, 12, 0, 0, 0, time.UTC)
	note := &activitypub.Note{
		BaseObject: activitypub.BaseObject{
			ID:        "https://remote.example/objects/1",
			Type:      activitypub.NoteType,
			Published: &published,
			To:        []string{"https://example.com/users/bob"},
		},
		AttributedTo: "https://remote.example/users/alice",
		Content:      "hello",
	}
	activity := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{ID: "https://remote.example/activities/1", Type: activitypub.CreateType},
		Actor:      note.AttributedTo,
		Object:     note,
	}

	require.NoError(t, handler.processCreateActivity(context.Background(), activity, "bob"))
	require.NoError(t, handler.processCreateActivity(context.Background(), activity, "bob"),
		"replayed Create must continue through mention and timeline fanout")
	require.Len(t, fake.Items(models.MainTableName), 1)
}

package handlers

import (
	"errors"
	"testing"

	"github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/storage"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestMisc_NotificationSnapshotHelpers_Round29(t *testing.T) {
	cfg := round11TestConfig()

	handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{
		firstErrorPK: map[string]error{
			"ACTOR#missing": errors.New("boom"),
		},
	})

	t.Run("shouldIncludeStatus handles snapshot and unsupported types", func(t *testing.T) {
		require.False(t, handler.shouldIncludeStatus(nil))
		require.False(t, handler.shouldIncludeStatus(&storage.Notification{Type: models.NotificationTypeFollow, StatusID: "123"}))
		require.False(t, handler.shouldIncludeStatus(&storage.Notification{Type: models.NotificationTypeMention, StatusID: "not valid"}))
		require.True(t, handler.shouldIncludeStatus(&storage.Notification{
			Type: models.NotificationTypeMention,
			Data: map[string]interface{}{
				"postSnapshot": map[string]interface{}{"id": "https://example.com/objects/s1"},
			},
		}))
	})

	t.Run("notificationPostSnapshot rejects invalid payloads", func(t *testing.T) {
		_, ok := handler.notificationPostSnapshot(&storage.Notification{})
		require.False(t, ok)

		_, ok = handler.notificationPostSnapshot(&storage.Notification{
			Data: map[string]interface{}{"postSnapshot": "bad"},
		})
		require.False(t, ok)
	})

	t.Run("statusFromNotificationSnapshot returns nil for malformed snapshots", func(t *testing.T) {
		ctx, err := round10NewLiftContext("GET", "/test", nil, nil, nil)
		require.NoError(t, err)

		status := handler.statusFromNotificationSnapshot(ctx, &storage.Notification{
			Data: map[string]interface{}{
				"postSnapshot": map[string]interface{}{
					"id": 123,
				},
			},
		})
		require.Nil(t, status)
	})

	t.Run("notificationSnapshotActor falls back when actor lookup fails", func(t *testing.T) {
		ctx, err := round10NewLiftContext("GET", "/test", nil, nil, nil)
		require.NoError(t, err)

		actor := handler.notificationSnapshotActor(ctx, map[string]interface{}{
			"attributedTo": "https://remote.example/users/missing",
		})
		require.NotNil(t, actor)
		require.Equal(t, "missing", actor.PreferredUsername)
		require.Equal(t, "https://remote.example/users/missing", actor.ID)
	})

	t.Run("private snapshot is not expanded without viewer access", func(t *testing.T) {
		ctx, err := round10NewLiftContext("GET", "/test", nil, nil, nil)
		require.NoError(t, err)

		status := handler.statusFromNotificationSnapshot(ctx, &storage.Notification{
			ID:       "n-private",
			Type:     models.NotificationTypeMention,
			Username: "alice",
			Data: map[string]interface{}{
				"postSnapshot": map[string]interface{}{
					"id":           "https://example.com/objects/private-1",
					"content":      "private",
					"visibility":   "private",
					"attributedTo": "https://example.com/users/bob",
				},
			},
		})
		require.Nil(t, status)
	})

	t.Run("private snapshot expands when viewer follows author", func(t *testing.T) {
		followerHandler, _, _ := round11NewHandler(t, cfg, &round10QueryState{
			relationshipRecords: []storagemodels.RelationshipRecord{
				{
					PK:    "FOLLOW#alice",
					SK:    "FOLLOWING#bob",
					State: storagemodels.RelationshipAccepted,
				},
			},
		})
		ctx, err := round10NewLiftContext("GET", "/test", nil, nil, nil)
		require.NoError(t, err)

		status := followerHandler.statusFromNotificationSnapshot(ctx, &storage.Notification{
			ID:       "n-private",
			Type:     models.NotificationTypeMention,
			Username: "alice",
			Data: map[string]interface{}{
				"postSnapshot": map[string]interface{}{
					"id":           "https://example.com/objects/private-1",
					"content":      "private",
					"visibility":   "private",
					"attributedTo": "https://example.com/users/bob",
				},
			},
		})
		require.NotNil(t, status)
		require.Equal(t, "private", status.Visibility)
	})

	t.Run("direct object requires recipient evidence when available", func(t *testing.T) {
		ctx, err := round10NewLiftContext("GET", "/test", nil, nil, nil)
		require.NoError(t, err)

		require.False(t, handler.notificationStatusVisibleToViewer(
			ctx.Context(),
			&storage.Notification{ID: "n-direct", Username: "alice"},
			"direct",
			"https://example.com/users/bob",
			[]string{"https://example.com/users/carol"},
			nil,
		))
		require.True(t, handler.notificationStatusVisibleToViewer(
			ctx.Context(),
			&storage.Notification{ID: "n-direct", Username: "alice"},
			"direct",
			"https://example.com/users/bob",
			[]string{"https://example.com/users/alice"},
			nil,
		))
	})

	t.Run("notificationSnapshotString ignores non-string values", func(t *testing.T) {
		require.Equal(t, "", notificationSnapshotString(map[string]interface{}{"createdAt": 123}, "createdAt"))
	})
}

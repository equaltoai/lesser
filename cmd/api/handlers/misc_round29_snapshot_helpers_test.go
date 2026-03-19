package handlers

import (
	"errors"
	"testing"

	"github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/storage"
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

	t.Run("notificationSnapshotString ignores non-string values", func(t *testing.T) {
		require.Equal(t, "", notificationSnapshotString(map[string]interface{}{"createdAt": 123}, "createdAt"))
	})
}

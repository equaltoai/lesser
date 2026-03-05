package handlers

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	apperrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/services/notifications"
	"github.com/stretchr/testify/require"
)

func TestNotificationDelivery_Round28_AuthAndIdempotency(t *testing.T) {
	cfg := round11TestConfig()
	cfg.AdminUsername = "admin"
	cfg.InstanceAPIKey = "instance-key"

	h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

	var (
		createCalls int
		seenCmds    []*notifications.CreateNotificationCommand
	)

	h.registry = &RegistryStub{
		NotificationsSvc: &NotificationsServiceStub{
			CreateNotificationFunc: func(_ context.Context, cmd *notifications.CreateNotificationCommand) (*notifications.NotificationResult, error) {
				createCalls++
				seenCmds = append(seenCmds, cmd)
				if createCalls == 2 {
					return nil, apperrors.AlreadyExists("notification")
				}
				return &notifications.NotificationResult{}, nil
			},
		},
	}

	fixturePath := filepath.Join("..", "testdata", "notification_deliver_fixture_v3.json")
	payload, err := os.ReadFile(fixturePath)
	require.NoError(t, err)

	t.Run("missing auth is rejected", func(t *testing.T) {
		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v1/notifications/deliver", nil, nil, payload)
		requireStatus(t, http.StatusUnauthorized)(h.HandleDeliverNotificationLift(ctx))
	})

	t.Run("invalid key is rejected", func(t *testing.T) {
		headers := map[string]string{"Authorization": "Bearer wrong"}
		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v1/notifications/deliver", headers, nil, payload)
		requireStatus(t, http.StatusForbidden)(h.HandleDeliverNotificationLift(ctx))
	})

	t.Run("valid key delivers and is idempotent", func(t *testing.T) {
		headers := map[string]string{"Authorization": "Bearer " + cfg.InstanceAPIKey}

		ctx1 := round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v1/notifications/deliver", headers, nil, payload)
		requireStatus(t, http.StatusNoContent)(h.HandleDeliverNotificationLift(ctx1))

		ctx2 := round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v1/notifications/deliver", headers, nil, payload)
		requireStatus(t, http.StatusNoContent)(h.HandleDeliverNotificationLift(ctx2))

		require.GreaterOrEqual(t, len(seenCmds), 2)
		cmd := seenCmds[len(seenCmds)-1]

		expectedID, idErr := commNotificationID(cfg.AdminUsername, "comm-msg-001")
		require.NoError(t, idErr)
		require.Equal(t, expectedID, cmd.ID)

		require.NotNil(t, cmd.CreatedAt)
		require.Equal(t, time.Date(2026, time.March, 4, 12, 0, 0, 0, time.UTC), cmd.CreatedAt.UTC())

		require.Equal(t, commNotificationTypeInbound, cmd.Type)
		require.Equal(t, cfg.AdminUsername, cmd.UserID)
		require.Equal(t, "alice@example.com", cmd.ActorID)

		require.NotNil(t, cmd.Data)
		require.Equal(t, "email", cmd.Data["channel"])
		require.Equal(t, "comm-msg-001", cmd.Data["messageId"])
	})
}

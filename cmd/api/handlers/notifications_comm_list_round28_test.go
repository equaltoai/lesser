package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	apiModels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/services/notifications"
	storageModels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestNotifications_ListIncludesCommNotifications_Round28(t *testing.T) {
	cfg := round11TestConfig()

	state := &round10QueryState{
		notFoundPKs: map[string]bool{
			"ACTOR#alice@example.com": true,
		},
	}

	h, _, _ := round11NewHandler(t, cfg, state)

	h.registry = &RegistryStub{
		NotificationsSvc: &NotificationsServiceStub{
			ListNotificationsFunc: func(_ context.Context, query *notifications.ListNotificationsQuery) (*notifications.NotificationListResult, error) {
				require.NotNil(t, query)
				require.Equal(t, "admin", query.UserID)
				require.Equal(t, []string{commNotificationTypeInbound}, query.Types)

				return &notifications.NotificationListResult{
					Notifications: []*storageModels.Notification{
						{
							ID:        "notif-comm-1",
							Type:      commNotificationTypeInbound,
							ActorID:   "alice@example.com",
							UserID:    "admin",
							IsRead:    false,
							CreatedAt: time.Date(2026, time.March, 4, 12, 0, 0, 0, time.UTC),
						},
					},
				}, nil
			},
		},
	}

	readToken := round11SignAccessToken(t, cfg.JWTSecret, "admin", []string{"read:notifications", auth.ScopeRead})
	headers := map[string]string{
		"Authorization": "Bearer " + readToken,
		"Host":          "example.com",
	}

	ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/notifications", headers, map[string]string{
		"types[]": commNotificationTypeInbound,
	}, nil)
	require.NoError(t, err)

	resp := requireStatus(t, http.StatusOK)(h.HandleGetNotificationsLift(ctx))

	var out []apiModels.Notification
	require.NoError(t, json.Unmarshal(resp.Body, &out))
	require.Len(t, out, 1)
	require.Equal(t, commNotificationTypeInbound, out[0].Type)
	require.Equal(t, "notif-comm-1", out[0].ID)
	require.Equal(t, "alice", out[0].Account.Username)
}

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
	"github.com/equaltoai/lesser/pkg/storage"
	storageModels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestNotifications_GetIncludesCommDetails_Round28(t *testing.T) {
	cfg := round11TestConfig()

	state := &round10QueryState{
		notFoundPKs: map[string]bool{
			"ACTOR#alice@example.com": true,
		},
		notificationsByID: map[string]storageModels.Notification{
			"notif-comm-1": {
				ID:        "notif-comm-1",
				UserID:    "admin",
				Type:      commNotificationTypeInbound,
				ActorID:   "alice@example.com",
				IsRead:    true,
				Title:     "Re: Hello",
				Body:      "hello from &lt;b&gt;alice&lt;/b&gt;",
				CreatedAt: time.Date(2026, time.March, 4, 12, 0, 0, 0, time.UTC),
				Data: map[string]interface{}{
					"channel": "email",
					"from": map[string]interface{}{
						"address":     "alice@example.com",
						"displayName": "Alice",
					},
					"receivedAt": "2026-03-04T12:00:00Z",
					"messageId":  "comm-msg-001",
				},
			},
		},
	}

	h, _, _ := round11NewHandler(t, cfg, state)
	h.registry = &RegistryStub{
		NotificationsSvc: &NotificationsServiceStub{
			GetNotificationFunc: func(_ context.Context, query *notifications.GetNotificationQuery) (*storageModels.Notification, error) {
				notif := state.notificationsByID[query.NotificationID]
				if notif.UserID != query.UserID {
					return nil, storage.ErrNotFound
				}
				return &notif, nil
			},
		},
	}

	readToken := round11SignAccessToken(t, cfg.JWTSecret, "admin", []string{"read:notifications", auth.ScopeRead})
	headers := map[string]string{
		"Authorization": "Bearer " + readToken,
		"Host":          "example.com",
	}

	ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/notifications/notif-comm-1", headers, nil, nil)
	require.NoError(t, err)
	ctx.Params["id"] = "notif-comm-1"

	resp := requireStatus(t, http.StatusOK)(h.HandleGetNotificationLift(ctx))

	var out apiModels.Notification
	require.NoError(t, json.Unmarshal(resp.Body, &out))
	require.Equal(t, "notif-comm-1", out.ID)
	require.Equal(t, commNotificationTypeInbound, out.Type)
	require.True(t, out.Read)
	require.Equal(t, "alice", out.Account.Username)
	require.NotNil(t, out.Communication)
	require.Equal(t, "comm-msg-001", out.Communication.MessageID)
	require.Equal(t, "comm-msg-001", out.Communication.ThreadID)
	require.Equal(t, "Re: Hello", out.Communication.Subject)
	require.Equal(t, "hello from &lt;b&gt;alice&lt;/b&gt;", out.Communication.Body)
}

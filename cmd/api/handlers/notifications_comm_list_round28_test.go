package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	apiModels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/activitypub"
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
							ID:      "notif-comm-1",
							Type:    commNotificationTypeInbound,
							ActorID: "alice@example.com",
							UserID:  "admin",
							IsRead:  false,
							Title:   "Re: Hello",
							Body:    "hello from <b>alice</b>",
							Data: map[string]interface{}{
								"channel": "email",
								"from": map[string]interface{}{
									"address":     "alice@example.com",
									"displayName": "Alice",
									"soulAgentId": "0xabc",
								},
								"to": map[string]interface{}{
									"address": "agent-bob@lessersoul.ai",
								},
								"attachments": []interface{}{
									map[string]interface{}{
										"id":          "att-1",
										"filename":    "proposal.pdf",
										"contentType": "application/pdf",
										"sizeBytes":   float64(123456),
										"sha256":      "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
									},
								},
								"receivedAt": "2026-03-04T12:00:00Z",
								"messageId":  "comm-msg-001",
								"inReplyTo":  "comm-msg-000",
							},
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
	require.False(t, out[0].Read)
	require.NotNil(t, out[0].Communication)
	require.Equal(t, "email", out[0].Communication.Channel)
	require.Equal(t, "alice@example.com", out[0].Communication.From.Address)
	require.Equal(t, "Alice", out[0].Communication.From.DisplayName)
	require.Equal(t, "0xabc", out[0].Communication.From.SoulAgentID)
	require.NotNil(t, out[0].Communication.To)
	require.Equal(t, "agent-bob@lessersoul.ai", out[0].Communication.To.Address)
	require.Len(t, out[0].Communication.Attachments, 1)
	require.Equal(t, "att-1", out[0].Communication.Attachments[0].ID)
	require.Equal(t, "proposal.pdf", out[0].Communication.Attachments[0].Filename)
	require.Equal(t, "application/pdf", out[0].Communication.Attachments[0].ContentType)
	require.Equal(t, int64(123456), out[0].Communication.Attachments[0].SizeBytes)
	require.Equal(t, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", out[0].Communication.Attachments[0].SHA256)
	require.Equal(t, "Re: Hello", out[0].Communication.Subject)
	require.Equal(t, "hello from <b>alice</b>", out[0].Communication.Body)
	require.Equal(t, time.Date(2026, time.March, 4, 12, 0, 0, 0, time.UTC), out[0].Communication.ReceivedAt)
	require.Equal(t, "comm-msg-001", out[0].Communication.MessageID)
	require.Equal(t, "comm-msg-000", out[0].Communication.InReplyTo)
	require.Equal(t, "comm-msg-000", out[0].Communication.ThreadID)
}

func TestNotifications_ListIncludesReplyNotifications_Round28(t *testing.T) {
	cfg := round11TestConfig()
	now := time.Date(2026, time.March, 16, 23, 0, 0, 0, time.UTC)

	state := &round10QueryState{
		actorsByUser: map[string]storageModels.Actor{
			"alice": {
				Username: "alice",
				Actor: &activitypub.Actor{
					BaseObject:        activitypub.BaseObject{ID: cfg.BaseURL() + "/users/alice", Type: "Person"},
					PreferredUsername: "alice",
					Name:              "Alice",
				},
			},
			"bob": {
				Username: "bob",
				Actor: &activitypub.Actor{
					BaseObject:        activitypub.BaseObject{ID: cfg.BaseURL() + "/users/bob", Type: "Person"},
					PreferredUsername: "bob",
					Name:              "Bob",
				},
			},
		},
		objectsByID: map[string]storageModels.Object{
			"status-1": {
				ID:           "status-1",
				Type:         activitypub.NoteType,
				Content:      "reply body",
				Published:    now.Add(-1 * time.Minute),
				AttributedTo: cfg.BaseURL() + "/users/bob",
			},
		},
	}

	h, _, _ := round11NewHandler(t, cfg, state)
	h.registry = &RegistryStub{
		NotificationsSvc: &NotificationsServiceStub{
			ListNotificationsFunc: func(_ context.Context, query *notifications.ListNotificationsQuery) (*notifications.NotificationListResult, error) {
				require.NotNil(t, query)
				require.Equal(t, "alice", query.UserID)
				require.Equal(t, []string{apiModels.NotificationTypeReply}, query.Types)

				return &notifications.NotificationListResult{
					Notifications: []*storageModels.Notification{
						{
							ID:         "notif-reply-1",
							Type:       apiModels.NotificationTypeReply,
							ActorID:    "bob",
							UserID:     "alice",
							TargetID:   "status-1",
							TargetType: "status",
							CreatedAt:  now,
						},
					},
				}, nil
			},
		},
	}

	readToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{"read:notifications", auth.ScopeRead})
	headers := map[string]string{
		"Authorization": "Bearer " + readToken,
		"Host":          "example.com",
	}

	ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/notifications", headers, map[string]string{
		"types[]": apiModels.NotificationTypeReply,
	}, nil)
	require.NoError(t, err)

	resp := requireStatus(t, http.StatusOK)(h.HandleGetNotificationsLift(ctx))

	var out []apiModels.Notification
	require.NoError(t, json.Unmarshal(resp.Body, &out))
	require.Len(t, out, 1)
	require.Equal(t, apiModels.NotificationTypeReply, out[0].Type)
	require.Equal(t, "bob", out[0].Account.Username)
	require.NotNil(t, out[0].Status)
	require.NotEmpty(t, out[0].Status.ID)
	require.Equal(t, "reply body", out[0].Status.Content)
}

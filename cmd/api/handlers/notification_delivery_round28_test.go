package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	apiModels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/activitypub"
	apperrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/services/notifications"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
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

		expectedID, idErr := commNotificationID("agent-bob", "comm-msg-001")
		require.NoError(t, idErr)
		require.Equal(t, expectedID, cmd.ID)

		require.NotNil(t, cmd.CreatedAt)
		require.Equal(t, time.Date(2026, time.March, 4, 12, 0, 0, 0, time.UTC), cmd.CreatedAt.UTC())

		require.Equal(t, commNotificationTypeInbound, cmd.Type)
		require.Equal(t, "agent-bob", cmd.UserID)
		require.Equal(t, "alice@example.com", cmd.ActorID)

		require.NotNil(t, cmd.Data)
		require.Equal(t, "email", cmd.Data["channel"])
		require.Equal(t, "comm-msg-001", cmd.Data["messageId"])

		toMap, ok := cmd.Data["to"].(map[string]interface{})
		require.True(t, ok)
		require.Equal(t, "agent-bob@lessersoul.ai", toMap["address"])

		rawAttachments, ok := cmd.Data["attachments"]
		require.True(t, ok)
		switch attachments := rawAttachments.(type) {
		case []map[string]interface{}:
			require.Len(t, attachments, 1)
			require.Equal(t, "att-1", attachments[0]["id"])
			require.Equal(t, "proposal.pdf", attachments[0]["filename"])
			require.Equal(t, "application/pdf", attachments[0]["contentType"])
			require.Equal(t, int64(123456), attachments[0]["sizeBytes"])
			require.Equal(t, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", attachments[0]["sha256"])
		case []interface{}:
			require.Len(t, attachments, 1)
			attachmentMap, ok := attachments[0].(map[string]interface{})
			require.True(t, ok)
			require.Equal(t, "att-1", attachmentMap["id"])
			require.Equal(t, "proposal.pdf", attachmentMap["filename"])
			require.Equal(t, "application/pdf", attachmentMap["contentType"])
			require.Equal(t, int64(123456), attachmentMap["sizeBytes"])
			require.Equal(t, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", attachmentMap["sha256"])
		default:
			require.FailNow(t, "unexpected attachments type")
		}
	})

	t.Run("managed lesser-host key is accepted", func(t *testing.T) {
		managedCfg := round11TestConfig()
		managedCfg.AdminUsername = "admin"
		managedCfg.LesserHostInstanceKey = "managed-instance-key"

		managedHandler, _, _ := round11NewHandler(t, managedCfg, &round10QueryState{})
		managedHandler.registry = &RegistryStub{
			NotificationsSvc: &NotificationsServiceStub{
				CreateNotificationFunc: func(_ context.Context, cmd *notifications.CreateNotificationCommand) (*notifications.NotificationResult, error) {
					require.Equal(t, "agent-bob", cmd.UserID)
					return &notifications.NotificationResult{}, nil
				},
			},
		}

		headers := map[string]string{"Authorization": "Bearer " + managedCfg.LesserHostInstanceKey}
		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v1/notifications/deliver", headers, nil, payload)
		requireStatus(t, http.StatusNoContent)(managedHandler.HandleDeliverNotificationLift(ctx))
	})

	t.Run("managed instances fall back to primary admin from instance state", func(t *testing.T) {
		managedCfg := round11TestConfig()
		managedCfg.AdminUsername = ""
		managedCfg.LesserHostInstanceKey = "managed-instance-key"

		state := &round10QueryState{
			instanceState: &storagemodels.InstanceState{PrimaryAdminUsername: "simulacrum"},
		}
		managedHandler, _, _ := round11NewHandler(t, managedCfg, state)
		managedHandler.registry = &RegistryStub{
			NotificationsSvc: &NotificationsServiceStub{
				CreateNotificationFunc: func(_ context.Context, cmd *notifications.CreateNotificationCommand) (*notifications.NotificationResult, error) {
					require.Equal(t, "agent-bob", cmd.UserID)
					return &notifications.NotificationResult{}, nil
				},
			},
		}

		headers := map[string]string{"Authorization": "Bearer " + managedCfg.LesserHostInstanceKey}
		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v1/notifications/deliver", headers, nil, payload)
		requireStatus(t, http.StatusNoContent)(managedHandler.HandleDeliverNotificationLift(ctx))
	})

	t.Run("addressed local agents receive the notification before admin fallback", func(t *testing.T) {
		managedCfg := round11TestConfig()
		managedCfg.AdminUsername = "admin"
		managedCfg.LesserHostInstanceKey = "managed-instance-key"

		state := &round10QueryState{
			usersByUsername: map[string]storagemodels.User{
				"Agent-0": {
					PK:        "USER#Agent-0",
					SK:        storagemodels.SKMetadata,
					Username:  "Agent-0",
					Role:      "user",
					Approved:  true,
					Version:   1,
					IsAgent:   true,
					CreatedAt: time.Date(2026, time.March, 1, 12, 0, 0, 0, time.UTC),
				},
			},
			actorsByUser: map[string]storagemodels.Actor{
				"Agent-0": {
					Username: "Agent-0",
					Actor: &activitypub.Actor{
						BaseObject: activitypub.BaseObject{
							ID:   managedCfg.ActorURL("Agent-0"),
							Type: "Person",
						},
						PreferredUsername: "Agent-0",
					},
				},
			},
		}

		managedHandler, _, _ := round11NewHandler(t, managedCfg, state)
		managedHandler.registry = &RegistryStub{
			NotificationsSvc: &NotificationsServiceStub{
				CreateNotificationFunc: func(_ context.Context, cmd *notifications.CreateNotificationCommand) (*notifications.NotificationResult, error) {
					require.Equal(t, "Agent-0", cmd.UserID)
					return &notifications.NotificationResult{}, nil
				},
			},
		}

		body, err := json.Marshal(apiModels.NotificationDeliveryRequest{
			Type:       "communication:inbound",
			Channel:    "email",
			From:       apiModels.NotificationDeliveryFrom{Address: "alice@example.com", DisplayName: "Alice"},
			To:         &apiModels.NotificationDeliveryTo{Address: "agent-0@lessersoul.ai"},
			Subject:    "hello",
			Body:       "test message",
			ReceivedAt: time.Date(2026, time.March, 4, 12, 0, 0, 0, time.UTC).Format(time.RFC3339),
			MessageID:  "comm-msg-agent-0",
		})
		require.NoError(t, err)

		headers := map[string]string{"Authorization": "Bearer " + managedCfg.LesserHostInstanceKey}
		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v1/notifications/deliver", headers, nil, body)
		requireStatus(t, http.StatusNoContent)(managedHandler.HandleDeliverNotificationLift(ctx))
	})

	t.Run("lowercase email local-part resolves mixed-case bound agent before admin fallback", func(t *testing.T) {
		managedCfg := round11TestConfig()
		managedCfg.AdminUsername = "admin"
		managedCfg.LesserHostInstanceKey = "managed-instance-key"
		managedCfg.Domain = "lessersoul.ai"

		state := &round10QueryState{
			usersByUsername: map[string]storagemodels.User{
				"Medic": {
					PK:        "USER#Medic",
					SK:        storagemodels.SKMetadata,
					Username:  "Medic",
					Role:      "user",
					Approved:  true,
					Version:   1,
					IsAgent:   true,
					CreatedAt: time.Date(2026, time.March, 1, 12, 0, 0, 0, time.UTC),
				},
			},
			actorsByUser: map[string]storagemodels.Actor{
				"Medic": {
					Username: "Medic",
					Actor: &activitypub.Actor{
						BaseObject: activitypub.BaseObject{
							ID:   managedCfg.ActorURL("Medic"),
							Type: "Person",
						},
						PreferredUsername: "Medic",
					},
				},
			},
			soulBodyBindingUsernames: map[string]storagemodels.InstanceSoulBodyBindingUsername{
				"Medic": {
					PK:        storagemodels.SoulBodyBindingUsernamePartitionKey("Medic"),
					SK:        storagemodels.SKSoulBodyBindingUsername,
					Username:  "Medic",
					AgentID:   "0xmedic",
					UpdatedAt: time.Date(2026, time.March, 4, 11, 0, 0, 0, time.UTC),
				},
			},
			soulBodyBindingsByAgentID: map[string]storagemodels.InstanceSoulBodyBinding{
				"0xmedic": {
					PK:               "INSTANCE#CONFIG",
					SK:               storagemodels.SoulBodyBindingSortKey("0xmedic"),
					AgentID:          "0xmedic",
					Username:         "Medic",
					PrincipalAddress: "0x1111111111111111111111111111111111111111",
					BoundAt:          time.Date(2026, time.March, 4, 11, 0, 0, 0, time.UTC),
					UpdatedAt:        time.Date(2026, time.March, 4, 11, 0, 0, 0, time.UTC),
				},
			},
		}

		managedHandler, _, _ := round11NewHandler(t, managedCfg, state)
		managedHandler.registry = &RegistryStub{
			NotificationsSvc: &NotificationsServiceStub{
				CreateNotificationFunc: func(_ context.Context, cmd *notifications.CreateNotificationCommand) (*notifications.NotificationResult, error) {
					require.Equal(t, "Medic", cmd.UserID)
					return &notifications.NotificationResult{}, nil
				},
			},
		}

		body, err := json.Marshal(apiModels.NotificationDeliveryRequest{
			Type:       "communication:inbound",
			Channel:    "email",
			From:       apiModels.NotificationDeliveryFrom{Address: "alice@example.com", DisplayName: "Alice"},
			To:         &apiModels.NotificationDeliveryTo{Address: "medic@lessersoul.ai"},
			Subject:    "hello",
			Body:       "test message",
			ReceivedAt: time.Date(2026, time.March, 4, 12, 0, 0, 0, time.UTC).Format(time.RFC3339),
			MessageID:  "comm-msg-medic",
		})
		require.NoError(t, err)

		headers := map[string]string{"Authorization": "Bearer " + managedCfg.LesserHostInstanceKey}
		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v1/notifications/deliver", headers, nil, body)
		requireStatus(t, http.StatusNoContent)(managedHandler.HandleDeliverNotificationLift(ctx))
	})

	t.Run("email body mime type is accepted and preserved", func(t *testing.T) {
		mimeHandler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

		mimeHandler.registry = &RegistryStub{
			NotificationsSvc: &NotificationsServiceStub{
				CreateNotificationFunc: func(_ context.Context, cmd *notifications.CreateNotificationCommand) (*notifications.NotificationResult, error) {
					require.Equal(t, "alice@example.com", cmd.ActorID)
					require.NotNil(t, cmd.Data)
					require.Equal(t, "text/plain", cmd.Data["bodyMimeType"])

					from, ok := cmd.Data["from"].(map[string]interface{})
					require.True(t, ok)
					require.Equal(t, "alice@example.com", from["address"])
					_, hasNumber := from["number"]
					require.False(t, hasNumber)

					return &notifications.NotificationResult{}, nil
				},
			},
		}

		body, err := json.Marshal(apiModels.NotificationDeliveryRequest{
			Type:         "communication:inbound",
			Channel:      "email",
			From:         apiModels.NotificationDeliveryFrom{Address: "alice@example.com", DisplayName: "Alice"},
			To:           &apiModels.NotificationDeliveryTo{Address: "agent-0@lessersoul.ai"},
			Subject:      "hello",
			Body:         "plain text body",
			BodyMimeType: "text/plain",
			ReceivedAt:   time.Date(2026, time.March, 4, 12, 0, 0, 0, time.UTC).Format(time.RFC3339),
			MessageID:    "comm-msg-body-mime",
		})
		require.NoError(t, err)

		headers := map[string]string{"Authorization": "Bearer " + cfg.InstanceAPIKey}
		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v1/notifications/deliver", headers, nil, body)
		requireStatus(t, http.StatusNoContent)(mimeHandler.HandleDeliverNotificationLift(ctx))
	})

	t.Run("sms number payload is accepted and preserved", func(t *testing.T) {
		smsHandler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

		smsHandler.registry = &RegistryStub{
			NotificationsSvc: &NotificationsServiceStub{
				CreateNotificationFunc: func(_ context.Context, cmd *notifications.CreateNotificationCommand) (*notifications.NotificationResult, error) {
					require.Equal(t, "+15551230000", cmd.ActorID)
					require.NotNil(t, cmd.Data)
					require.Equal(t, "sms", cmd.Data["channel"])

					from, ok := cmd.Data["from"].(map[string]interface{})
					require.True(t, ok)
					require.Equal(t, "+15551230000", from["number"])
					_, hasAddress := from["address"]
					require.False(t, hasAddress)

					to, ok := cmd.Data["to"].(map[string]interface{})
					require.True(t, ok)
					require.Equal(t, "+15557654321", to["number"])
					_, hasToAddress := to["address"]
					require.False(t, hasToAddress)

					return &notifications.NotificationResult{}, nil
				},
			},
		}

		body, err := json.Marshal(apiModels.NotificationDeliveryRequest{
			Type:       "communication:inbound",
			Channel:    "sms",
			From:       apiModels.NotificationDeliveryFrom{Number: "+15551230000", DisplayName: "Alice"},
			To:         &apiModels.NotificationDeliveryTo{Number: "+15557654321"},
			Body:       "test message",
			ReceivedAt: time.Date(2026, time.March, 4, 12, 5, 0, 0, time.UTC).Format(time.RFC3339),
			MessageID:  "comm-msg-sms",
		})
		require.NoError(t, err)

		headers := map[string]string{"Authorization": "Bearer " + cfg.InstanceAPIKey}
		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v1/notifications/deliver", headers, nil, body)
		requireStatus(t, http.StatusNoContent)(smsHandler.HandleDeliverNotificationLift(ctx))
	})
}

func TestNotificationDelivery_Round28_HelperPayloadBuilders(t *testing.T) {
	t.Run("comm notification data includes optional fields", func(t *testing.T) {
		delivery := &commNotificationDelivery{
			Channel:         commNotificationChannelSMS,
			FromAddress:     "fallback@example.com",
			FromNumber:      "+15551230000",
			FromDisplayName: "Alice",
			FromSoulAgentID: "agent-1",
			ToNumber:        "+15557654321",
			BodyMimeType:    "text/plain",
			ReceivedAt:      time.Date(2026, time.March, 4, 12, 5, 0, 0, time.UTC),
			MessageID:       "comm-msg-sms",
			InReplyTo:       "comm-msg-parent",
			Attachments: []commNotificationAttachment{
				{
					ID:          "att-1",
					Filename:    "note.txt",
					ContentType: "text/plain",
					SizeBytes:   42,
					SHA256:      "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
				},
			},
		}

		data := commNotificationData(delivery)

		require.Equal(t, "sms", data["channel"])
		require.Equal(t, "text/plain", data["bodyMimeType"])
		require.Equal(t, "comm-msg-parent", data["inReplyTo"])

		from, ok := data["from"].(map[string]interface{})
		require.True(t, ok)
		require.Equal(t, "fallback@example.com", from["address"])
		require.Equal(t, "+15551230000", from["number"])
		require.Equal(t, "Alice", from["displayName"])
		require.Equal(t, "agent-1", from["soulAgentId"])

		to, ok := data["to"].(map[string]interface{})
		require.True(t, ok)
		require.Equal(t, "+15557654321", to["number"])

		attachments, ok := data["attachments"].([]map[string]interface{})
		require.True(t, ok)
		require.Len(t, attachments, 1)
		require.Equal(t, "att-1", attachments[0]["id"])
	})

	t.Run("party and attachment helpers omit empty values", func(t *testing.T) {
		require.Empty(t, commNotificationParty("", "", "", ""))
		require.Nil(t, commNotificationAttachmentsData(nil))
	})
}

func TestNotificationDelivery_Round28_LogsCreateNotificationFailureCause(t *testing.T) {
	cfg := round11TestConfig()
	cfg.AdminUsername = "admin"
	cfg.InstanceAPIKey = "instance-key"

	h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
	core, observed := observer.New(zap.ErrorLevel)
	h.logger = zap.New(core)
	h.registry = &RegistryStub{
		NotificationsSvc: &NotificationsServiceStub{
			CreateNotificationFunc: func(_ context.Context, _ *notifications.CreateNotificationCommand) (*notifications.NotificationResult, error) {
				return nil, errors.New("recipient user not found")
			},
		},
	}

	fixturePath := filepath.Join("..", "testdata", "notification_deliver_fixture_v3.json")
	payload, err := os.ReadFile(fixturePath)
	require.NoError(t, err)

	headers := map[string]string{"Authorization": "Bearer " + cfg.InstanceAPIKey}
	ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v1/notifications/deliver", headers, nil, payload)
	requireStatus(t, http.StatusInternalServerError)(h.HandleDeliverNotificationLift(ctx))

	entries := observed.FilterMessage("failed to create notification").All()
	require.Len(t, entries, 1)
	require.Equal(t, "agent-bob", entries[0].ContextMap()["user_id"])
	require.Equal(t, "communication:inbound", entries[0].ContextMap()["type"])
	require.Contains(t, entries[0].ContextMap()["error"], "recipient user not found")
}

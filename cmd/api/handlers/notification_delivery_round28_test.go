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

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	apiModels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/activitypub"
	apperrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/services/notifications"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	dynamormerrors "github.com/theory-cloud/tabletheory/v2/pkg/errors"
	dynamormmocks "github.com/theory-cloud/tabletheory/v2/pkg/mocks"
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

	t.Run("valid instance api key still works when lesser-host key lookup fails", func(t *testing.T) {
		resetTrustSecretCache()

		origLoad := loadAWSConfigForTrustSecrets
		origNewClient := newSecretsManagerClientForTrustSecret
		t.Cleanup(func() {
			loadAWSConfigForTrustSecrets = origLoad
			newSecretsManagerClientForTrustSecret = origNewClient
			resetTrustSecretCache()
		})

		loadAWSConfigForTrustSecrets = func(context.Context, ...func(*awsconfig.LoadOptions) error) (aws.Config, error) {
			return aws.Config{}, nil
		}
		newSecretsManagerClientForTrustSecret = func(aws.Config) trustSecretsManagerClient {
			return &stubTrustSecretsManagerClient{err: errors.New("secret unavailable")}
		}

		db := new(dynamormmocks.MockDB)
		q := new(dynamormmocks.MockQuery)
		setupMockInstanceRepoDB(db, q)
		q.On("First", mock.AnythingOfType("*models.InstanceTrustConfig")).Run(func(args mock.Arguments) {
			out := args.Get(0).(*storagemodels.InstanceTrustConfig)
			out.PK = "INSTANCE#CONFIG"
			out.SK = storagemodels.SKTrustConfig
			out.Managed = &storagemodels.InstanceTrustConfigManaged{
				BaseURL:              "https://persisted.example",
				InstanceKeySecretARN: "secret-id",
			}
		}).Return(nil).Maybe()
		q.On("First", mock.AnythingOfType("*models.InstanceSoulBodyBindingUsername")).Return(dynamormerrors.ErrItemNotFound).Maybe()

		instanceRepo := repositories.NewInstanceRepository(db, "test-table", zap.NewNop())
		repos := &MockRepositoryStorage{}
		repos.On("Instance").Return(instanceRepo).Maybe()

		cfg := round11TestConfig()
		cfg.AdminUsername = "admin"
		cfg.InstanceAPIKey = "instance-key"

		handler, baseRepos, _ := round11NewHandler(t, cfg, &round10QueryState{})
		repos.On("Account").Return(baseRepos.Account()).Maybe()
		repos.On("Audit").Return(baseRepos.Audit()).Maybe()
		handler.repos = repos
		handler.logger = zap.NewNop()
		handler.registry = &RegistryStub{
			NotificationsSvc: &NotificationsServiceStub{
				CreateNotificationFunc: func(_ context.Context, cmd *notifications.CreateNotificationCommand) (*notifications.NotificationResult, error) {
					require.Equal(t, "agent-bob", cmd.UserID)
					return &notifications.NotificationResult{}, nil
				},
			},
		}

		headers := map[string]string{"Authorization": "Bearer instance-key"}
		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v1/notifications/deliver", headers, nil, payload)
		requireStatus(t, http.StatusNoContent)(handler.HandleDeliverNotificationLift(ctx))
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
				"medic": {
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

	t.Run("sms number payload is rejected when explicit recipient cannot be resolved", func(t *testing.T) {
		smsHandler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

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
		requireStatus(t, http.StatusUnprocessableEntity)(smsHandler.HandleDeliverNotificationLift(ctx))
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

func TestNotificationDelivery_Project37InstanceScopedAddressCanary(t *testing.T) {
	cfg := round11TestConfig()
	cfg.AdminUsername = "admin"
	cfg.InstanceAPIKey = "instance-key"

	createdAt := time.Date(2026, time.May, 22, 18, 30, 0, 0, time.UTC)
	state := &round10QueryState{
		usersByUsername: map[string]storagemodels.User{
			"agent-bob": {
				PK:        "USER#agent-bob",
				SK:        storagemodels.SKMetadata,
				Username:  "agent-bob",
				Role:      "user",
				Approved:  true,
				Version:   1,
				IsAgent:   true,
				CreatedAt: createdAt.Add(-24 * time.Hour),
				UpdatedAt: createdAt.Add(-1 * time.Hour),
			},
		},
		actorsByUser: map[string]storagemodels.Actor{
			"agent-bob": {
				Username: "agent-bob",
				Actor: &activitypub.Actor{
					BaseObject: activitypub.BaseObject{
						ID:   cfg.ActorURL("agent-bob"),
						Type: activitypub.PersonType,
					},
					PreferredUsername: "agent-bob",
				},
				CreatedAt: createdAt.Add(-24 * time.Hour),
				UpdatedAt: createdAt.Add(-1 * time.Hour),
			},
		},
		soulBodyBindingUsernames: map[string]storagemodels.InstanceSoulBodyBindingUsername{
			"agent-bob": {
				PK:        storagemodels.SoulBodyBindingUsernamePartitionKey("agent-bob"),
				SK:        storagemodels.SKSoulBodyBindingUsername,
				Username:  "agent-bob",
				AgentID:   "0xagentbob",
				UpdatedAt: createdAt.Add(-1 * time.Hour),
			},
		},
		soulBodyBindingsByAgentID: map[string]storagemodels.InstanceSoulBodyBinding{
			"0xagentbob": {
				PK:               "INSTANCE#CONFIG",
				SK:               storagemodels.SoulBodyBindingSortKey("0xagentbob"),
				AgentID:          "0xagentbob",
				Username:         "agent-bob",
				PrincipalAddress: "0x1111111111111111111111111111111111111111",
				BoundAt:          createdAt.Add(-24 * time.Hour),
				UpdatedAt:        createdAt.Add(-1 * time.Hour),
			},
		},
	}

	h, _, _ := round11NewHandler(t, cfg, state)

	var seen *notifications.CreateNotificationCommand
	h.registry = &RegistryStub{
		NotificationsSvc: &NotificationsServiceStub{
			CreateNotificationFunc: func(_ context.Context, cmd *notifications.CreateNotificationCommand) (*notifications.NotificationResult, error) {
				seen = cmd
				return &notifications.NotificationResult{}, nil
			},
		},
	}

	toSoulAgentID := "0xagentbob"
	body, err := json.Marshal(apiModels.NotificationDeliveryRequest{
		Type:       "communication:inbound",
		Channel:    "email",
		From:       apiModels.NotificationDeliveryFrom{Address: "alice@example.com", DisplayName: "Alice"},
		To:         &apiModels.NotificationDeliveryTo{Address: "agent-bob.simulacrum@lessersoul.ai", SoulAgentID: &toSoulAgentID},
		Subject:    "project 37 canary",
		Body:       "redacted canary body",
		ReceivedAt: createdAt.Format(time.RFC3339),
		MessageID:  "project37-m35-primary",
	})
	require.NoError(t, err)

	headers := map[string]string{"Authorization": "Bearer " + cfg.InstanceAPIKey}
	ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v1/notifications/deliver", headers, nil, body)
	requireStatus(t, http.StatusNoContent)(h.HandleDeliverNotificationLift(ctx))

	require.NotNil(t, seen)
	require.Equal(t, "agent-bob", seen.UserID)
	require.NotEqual(t, "agent-bob.simulacrum", seen.UserID)
	require.Equal(t, "alice@example.com", seen.ActorID)

	expectedID, idErr := commNotificationID("agent-bob", "project37-m35-primary")
	require.NoError(t, idErr)
	require.Equal(t, expectedID, seen.ID)

	to, ok := seen.Data["to"].(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, "agent-bob.simulacrum@lessersoul.ai", to["address"])
	require.NotContains(t, state.soulBodyBindingUsernames, "agent-bob.simulacrum@lessersoul.ai")
}

func TestNotificationDelivery_Project37RejectsCompoundAddressWithoutAuthoritativeRecipient(t *testing.T) {
	cfg := round11TestConfig()
	cfg.AdminUsername = "admin"
	cfg.InstanceAPIKey = "instance-key"

	createdAt := time.Date(2026, time.May, 22, 18, 45, 0, 0, time.UTC)
	state := &round10QueryState{
		usersByUsername: map[string]storagemodels.User{
			"agent-bob": {
				PK:        "USER#agent-bob",
				SK:        storagemodels.SKMetadata,
				Username:  "agent-bob",
				Role:      "user",
				Approved:  true,
				IsAgent:   true,
				CreatedAt: createdAt.Add(-24 * time.Hour),
				UpdatedAt: createdAt.Add(-1 * time.Hour),
			},
			"agent-bob.simulacrum": {
				PK:        "USER#agent-bob.simulacrum",
				SK:        storagemodels.SKMetadata,
				Username:  "agent-bob.simulacrum",
				Role:      "user",
				Approved:  true,
				IsAgent:   true,
				CreatedAt: createdAt.Add(-24 * time.Hour),
				UpdatedAt: createdAt.Add(-1 * time.Hour),
			},
		},
		actorsByUser: map[string]storagemodels.Actor{
			"agent-bob": {
				Username: "agent-bob",
				Actor: &activitypub.Actor{
					BaseObject:        activitypub.BaseObject{ID: cfg.ActorURL("agent-bob"), Type: activitypub.PersonType},
					PreferredUsername: "agent-bob",
				},
			},
			"agent-bob.simulacrum": {
				Username: "agent-bob.simulacrum",
				Actor: &activitypub.Actor{
					BaseObject:        activitypub.BaseObject{ID: cfg.ActorURL("agent-bob.simulacrum"), Type: activitypub.PersonType},
					PreferredUsername: "agent-bob.simulacrum",
				},
			},
		},
		soulBodyBindingUsernames: map[string]storagemodels.InstanceSoulBodyBindingUsername{
			"agent-bob": {
				PK:        storagemodels.SoulBodyBindingUsernamePartitionKey("agent-bob"),
				SK:        storagemodels.SKSoulBodyBindingUsername,
				Username:  "agent-bob",
				AgentID:   "0xagentbob",
				UpdatedAt: createdAt.Add(-1 * time.Hour),
			},
		},
		soulBodyBindingsByAgentID: map[string]storagemodels.InstanceSoulBodyBinding{
			"0xagentbob": {
				PK:               "INSTANCE#CONFIG",
				SK:               storagemodels.SoulBodyBindingSortKey("0xagentbob"),
				AgentID:          "0xagentbob",
				Username:         "agent-bob",
				PrincipalAddress: "0x1111111111111111111111111111111111111111",
				BoundAt:          createdAt.Add(-24 * time.Hour),
				UpdatedAt:        createdAt.Add(-1 * time.Hour),
			},
		},
	}

	h, _, _ := round11NewHandler(t, cfg, state)

	createCalls := 0
	h.registry = &RegistryStub{
		NotificationsSvc: &NotificationsServiceStub{
			CreateNotificationFunc: func(_ context.Context, _ *notifications.CreateNotificationCommand) (*notifications.NotificationResult, error) {
				createCalls++
				return &notifications.NotificationResult{}, nil
			},
		},
	}

	body, err := json.Marshal(apiModels.NotificationDeliveryRequest{
		Type:       "communication:inbound",
		Channel:    "email",
		From:       apiModels.NotificationDeliveryFrom{Address: "alice@example.com", DisplayName: "Alice"},
		To:         &apiModels.NotificationDeliveryTo{Address: "agent-bob.simulacrum@lessersoul.ai"},
		Subject:    "project 37 canary",
		Body:       "redacted canary body",
		ReceivedAt: createdAt.Format(time.RFC3339),
		MessageID:  "project37-m35-missing-authority",
	})
	require.NoError(t, err)

	headers := map[string]string{"Authorization": "Bearer " + cfg.InstanceAPIKey}
	ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v1/notifications/deliver", headers, nil, body)
	requireStatus(t, http.StatusUnprocessableEntity)(h.HandleDeliverNotificationLift(ctx))
	require.Zero(t, createCalls)
}

func TestNotificationDelivery_Round28_RejectsUnresolvedExplicitRecipientAndKeepsAdminFallbackForInstanceMessages(t *testing.T) {
	t.Run("explicit recipient resolution failure is rejected", func(t *testing.T) {
		cfg := round11TestConfig()
		cfg.AdminUsername = "admin"
		cfg.InstanceAPIKey = "instance-key"

		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
		core, observed := observer.New(zap.WarnLevel)
		h.logger = zap.New(core)

		createCalls := 0
		h.registry = &RegistryStub{
			NotificationsSvc: &NotificationsServiceStub{
				CreateNotificationFunc: func(_ context.Context, _ *notifications.CreateNotificationCommand) (*notifications.NotificationResult, error) {
					createCalls++
					return &notifications.NotificationResult{}, nil
				},
			},
		}

		body, err := json.Marshal(apiModels.NotificationDeliveryRequest{
			Type:       "communication:inbound",
			Channel:    "email",
			From:       apiModels.NotificationDeliveryFrom{Address: "alice@example.com", DisplayName: "Alice"},
			To:         &apiModels.NotificationDeliveryTo{Address: "missing-agent@remote.example"},
			Subject:    "hello",
			Body:       "test message",
			ReceivedAt: time.Date(2026, time.March, 4, 12, 0, 0, 0, time.UTC).Format(time.RFC3339),
			MessageID:  "comm-msg-missing-agent",
		})
		require.NoError(t, err)

		headers := map[string]string{"Authorization": "Bearer " + cfg.InstanceAPIKey}
		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v1/notifications/deliver", headers, nil, body)
		requireStatus(t, http.StatusUnprocessableEntity)(h.HandleDeliverNotificationLift(ctx))
		require.Zero(t, createCalls)

		entries := observed.FilterMessage("notification recipient resolution failed").All()
		require.Len(t, entries, 1)
		require.Equal(t, "missing-agent@remote.example", entries[0].ContextMap()["to_address"])
		require.Equal(t, "email", entries[0].ContextMap()["channel"])
	})

	t.Run("instance-addressed notifications still fall back to admin", func(t *testing.T) {
		cfg := round11TestConfig()
		cfg.AdminUsername = "admin"
		cfg.InstanceAPIKey = "instance-key"

		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
		h.registry = &RegistryStub{
			NotificationsSvc: &NotificationsServiceStub{
				CreateNotificationFunc: func(_ context.Context, cmd *notifications.CreateNotificationCommand) (*notifications.NotificationResult, error) {
					require.Equal(t, "admin", cmd.UserID)
					return &notifications.NotificationResult{}, nil
				},
			},
		}

		body, err := json.Marshal(apiModels.NotificationDeliveryRequest{
			Type:       "communication:inbound",
			Channel:    "sms",
			From:       apiModels.NotificationDeliveryFrom{Number: "+15551230000", DisplayName: "Alice"},
			Body:       "test message",
			ReceivedAt: time.Date(2026, time.March, 4, 12, 0, 0, 0, time.UTC).Format(time.RFC3339),
			MessageID:  "comm-msg-instance",
		})
		require.NoError(t, err)

		headers := map[string]string{"Authorization": "Bearer " + cfg.InstanceAPIKey}
		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v1/notifications/deliver", headers, nil, body)
		requireStatus(t, http.StatusNoContent)(h.HandleDeliverNotificationLift(ctx))
	})
}

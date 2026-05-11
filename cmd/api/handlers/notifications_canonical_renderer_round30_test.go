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
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestNotifications_CanonicalRendererListSingleAndSemanticActors_Round30(t *testing.T) {
	cfg := round11TestConfig()
	now := time.Date(2026, time.May, 11, 10, 30, 0, 0, time.UTC)
	comm := &storageModels.Notification{
		ID:        "notif-comm",
		Type:      commNotificationTypeInbound,
		ActorID:   "alice@example.net",
		ActorType: notificationActorTypeExternal,
		UserID:    "admin",
		IsRead:    true,
		Title:     "Subject from title",
		Body:      "Body from canonical field",
		CreatedAt: now,
		Data: map[string]interface{}{
			"channel":   "email",
			"messageId": "message-1",
			"from": map[string]interface{}{
				"address":     "alice@example.net",
				"displayName": "Alice External",
				"soulAgentId": "soul-agent-1",
			},
		},
	}

	state := &round10QueryState{
		firstErrorPK: map[string]error{
			"ACTOR#alice@example.net": context.DeadlineExceeded,
		},
	}
	h, _, _ := round11NewHandler(t, cfg, state)
	core, observed := observer.New(zap.WarnLevel)
	h.logger = zap.New(core)
	h.registry = &RegistryStub{
		NotificationsSvc: &NotificationsServiceStub{
			ListNotificationsFunc: func(_ context.Context, query *notifications.ListNotificationsQuery) (*notifications.NotificationListResult, error) {
				require.Equal(t, "admin", query.UserID)
				return &notifications.NotificationListResult{Notifications: []*storageModels.Notification{comm}}, nil
			},
			GetNotificationFunc: func(_ context.Context, query *notifications.GetNotificationQuery) (*storageModels.Notification, error) {
				require.Equal(t, "admin", query.UserID)
				require.Equal(t, "notif-comm", query.NotificationID)
				return comm, nil
			},
		},
	}

	readToken := round11SignAccessToken(t, cfg.JWTSecret, "admin", []string{"read:notifications", auth.ScopeRead})
	headers := map[string]string{"Authorization": "Bearer " + readToken, "Host": "example.com"}

	ctxList, err := round10NewLiftContext(http.MethodGet, "/api/v1/notifications", headers, nil, nil)
	require.NoError(t, err)
	listResp := requireStatus(t, http.StatusOK)(h.HandleGetNotificationsLift(ctxList))

	var listOut []apiModels.Notification
	require.NoError(t, json.Unmarshal(listResp.Body, &listOut))
	require.Len(t, listOut, 1)

	ctxSingle, err := round10NewLiftContext(http.MethodGet, "/api/v1/notifications/notif-comm", headers, nil, nil)
	require.NoError(t, err)
	ctxSingle.Params["id"] = "notif-comm"
	singleResp := requireStatus(t, http.StatusOK)(h.HandleGetNotificationLift(ctxSingle))

	var singleOut apiModels.Notification
	require.NoError(t, json.Unmarshal(singleResp.Body, &singleOut))

	require.Equal(t, listOut[0].ID, singleOut.ID)
	require.Equal(t, listOut[0].Type, singleOut.Type)
	require.Equal(t, listOut[0].Read, singleOut.Read)
	require.Equal(t, listOut[0].Account.Username, singleOut.Account.Username)
	require.Equal(t, "alice", singleOut.Account.Username)
	require.Equal(t, "Alice External", singleOut.Account.DisplayName)
	require.NotNil(t, singleOut.Communication)
	require.Equal(t, "Subject from title", singleOut.Communication.Subject)
	require.Equal(t, "Body from canonical field", singleOut.Communication.Body)
	require.Equal(t, "soul-agent-1", singleOut.Communication.From.SoulAgentID)
	require.Equal(t, 0, observed.FilterMessage("failed to get local actor for notification; trying remote cache/fallback").Len())
}

func TestNotifications_RemoteActorRendererSkipsLocalLookup_Round30(t *testing.T) {
	cfg := round11TestConfig()
	now := time.Date(2026, time.May, 11, 11, 0, 0, 0, time.UTC)
	remoteActorID := "https://remote.example/users/riley"
	state := &round10QueryState{
		firstErrorPK: map[string]error{
			"ACTOR#riley": context.DeadlineExceeded,
		},
		remoteActorsByPK: map[string]storageModels.RemoteActor{
			"REMOTE_ACTOR#riley@remote.example": {
				PK:        "REMOTE_ACTOR#riley@remote.example",
				SK:        storageModels.SKProfile,
				Handle:    "riley@remote.example",
				ExpiresAt: time.Now().Add(time.Hour),
				Actor: &activitypub.Actor{
					BaseObject:        activitypub.BaseObject{ID: remoteActorID, Type: activitypub.PersonType},
					PreferredUsername: "riley",
					Name:              "Riley Remote",
					URL:               "https://remote.example/@riley",
					Inbox:             "https://remote.example/users/riley/inbox",
				},
			},
		},
	}
	h, _, _ := round11NewHandler(t, cfg, state)
	core, observed := observer.New(zap.WarnLevel)
	h.logger = zap.New(core)

	ctx, err := round10NewLiftContext(http.MethodGet, "/test", nil, nil, nil)
	require.NoError(t, err)
	out := h.convertSingleNotification(ctx, &storageModels.Notification{
		ID:        "notif-remote",
		Type:      apiModels.NotificationTypeMention,
		ActorID:   remoteActorID,
		ActorType: notificationActorTypeRemoteActor,
		UserID:    "admin",
		CreatedAt: now,
	})

	require.NotNil(t, out)
	require.Equal(t, remoteActorID, out.Account.ID)
	require.Equal(t, "riley", out.Account.Username)
	require.Equal(t, "riley@remote.example", out.Account.Acct)
	require.Equal(t, "Riley Remote", out.Account.DisplayName)
	require.Equal(t, 0, observed.FilterMessage("failed to get local actor for notification; trying remote cache/fallback").Len())
}

func TestNotifications_GroupedSampleAccountsUseSemanticResolver_Round30(t *testing.T) {
	cfg := round11TestConfig()
	now := time.Date(2026, time.May, 11, 11, 30, 0, 0, time.UTC)
	remoteActorID := "https://remote.example/users/riley"
	state := &round10QueryState{
		firstErrorPK: map[string]error{
			"ACTOR#riley":             context.DeadlineExceeded,
			"ACTOR#alice@example.net": context.DeadlineExceeded,
		},
		remoteActorsByPK: map[string]storageModels.RemoteActor{
			"REMOTE_ACTOR#riley@remote.example": {
				PK:        "REMOTE_ACTOR#riley@remote.example",
				SK:        storageModels.SKProfile,
				Handle:    "riley@remote.example",
				ExpiresAt: time.Now().Add(time.Hour),
				Actor: &activitypub.Actor{
					BaseObject:        activitypub.BaseObject{ID: remoteActorID, Type: activitypub.PersonType},
					PreferredUsername: "riley",
					Name:              "Riley Remote",
					URL:               "https://remote.example/@riley",
					Inbox:             "https://remote.example/users/riley/inbox",
				},
			},
		},
	}
	h, _, _ := round11NewHandler(t, cfg, state)
	core, observed := observer.New(zap.WarnLevel)
	h.logger = zap.New(core)

	remote := &storageModels.Notification{
		ID:        "notif-remote",
		Type:      apiModels.NotificationTypeFollow,
		ActorID:   remoteActorID,
		ActorType: notificationActorTypeRemoteActor,
		UserID:    "admin",
		CreatedAt: now,
	}
	external := &storageModels.Notification{
		ID:        "notif-external",
		Type:      commNotificationTypeInbound,
		ActorID:   "alice@example.net",
		ActorType: notificationActorTypeExternal,
		UserID:    "admin",
		CreatedAt: now.Add(-time.Minute),
		Data: map[string]interface{}{
			"messageId": "message-2",
			"from": map[string]interface{}{
				"address":     "alice@example.net",
				"displayName": "Alice External",
			},
		},
	}

	ctx, err := round10NewLiftContext(http.MethodGet, "/api/v2/notifications/grouped", nil, nil, nil)
	require.NoError(t, err)
	out := h.convertGroupedNotificationsToAPI(ctx, []*notifications.GroupedNotification{
		{
			ID:                "group-1",
			Type:              apiModels.NotificationTypeFollow,
			GroupKey:          "group-1",
			Count:             2,
			LatestCreatedAt:   now,
			EarliestCreatedAt: now.Add(-time.Minute),
			SampleAccounts: []notifications.NotificationAccount{
				{ID: remoteActorID, CreatedAt: now},
				{ID: "alice@example.net", CreatedAt: now.Add(-time.Minute)},
			},
			AllNotifications: []*storageModels.Notification{remote, external},
		},
	})

	require.Len(t, out, 1)
	require.Len(t, out[0].SampleAccounts, 2)
	require.Equal(t, "riley", out[0].SampleAccounts[0].Username)
	require.Equal(t, "Riley Remote", out[0].SampleAccounts[0].DisplayName)
	require.Equal(t, "alice", out[0].SampleAccounts[1].Username)
	require.Equal(t, "Alice External", out[0].SampleAccounts[1].DisplayName)
	require.Contains(t, out[0].Summary, "Riley Remote")
	require.Equal(t, 0, observed.FilterMessage("failed to get local actor for notification; trying remote cache/fallback").Len())
}

func TestNotifications_RendererHelperBranches_Round30(t *testing.T) {
	cfg := round11TestConfig()
	state := &round10QueryState{
		actorsByUser: map[string]storageModels.Actor{
			"local": {
				Username: "local",
				Actor: &activitypub.Actor{
					BaseObject:        activitypub.BaseObject{ID: cfg.BaseURL() + "/users/local", Type: activitypub.PersonType},
					PreferredUsername: "local",
					Name:              "Local User",
				},
			},
		},
	}
	h, _, _ := round11NewHandler(t, cfg, state)
	ctx, err := round10NewLiftContext(http.MethodGet, "/test", nil, nil, nil)
	require.NoError(t, err)

	require.Nil(t, notificationViewFromModel(nil))
	var nilView *notificationView
	require.Nil(t, nilView.communicationData())
	require.Equal(t, "", nilView.statusID())
	require.Nil(t, h.convertNotificationViewToAPI(ctx, nil))
	require.Nil(t, h.convertNotificationViewToAPI(ctx, &notificationView{ID: "missing-actor"}))
	require.False(t, notificationActorIDLooksURL("not a url"))

	localActor := h.notificationActorForView(ctx.Context(), &notificationView{
		ActorID:   "local",
		ActorType: notificationActorTypeUser,
	})
	require.NotNil(t, localActor)
	require.Equal(t, "local", localActor.PreferredUsername)

	remoteFallback := h.notificationActorForView(ctx.Context(), &notificationView{
		ActorID:   "https://remote.example/users/missing",
		ActorType: notificationActorTypeRemoteActor,
	})
	require.NotNil(t, remoteFallback)
	require.Equal(t, "missing", remoteFallback.PreferredUsername)

	emailFallback := h.notificationActorForView(ctx.Context(), &notificationView{ActorID: "bob@example.net"})
	require.NotNil(t, emailFallback)
	require.Equal(t, "bob", emailFallback.PreferredUsername)

	require.Nil(t, h.externalNotificationActor(&notificationView{}))
	require.Nil(t, h.resolveGroupedNotificationSampleAccounts(ctx.Context(), nil))
	require.Nil(t, h.resolveGroupedNotificationSampleAccounts(ctx.Context(), &notifications.GroupedNotification{}))
	require.Equal(t,
		notifications.NotificationAccount{ID: "sample"},
		h.resolveGroupedNotificationAccount(ctx.Context(), notifications.NotificationAccount{ID: "sample"}, nil),
	)
	require.Equal(t, "Display", notificationAccountDisplayName(notifications.NotificationAccount{DisplayName: "Display", Username: "user", ID: "id"}))
	require.Equal(t, "user", notificationAccountDisplayName(notifications.NotificationAccount{Username: "user", ID: "id"}))
	require.Equal(t, "id", notificationAccountDisplayName(notifications.NotificationAccount{ID: "id"}))
}

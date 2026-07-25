package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/services/notifications"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
	apptheory "github.com/theory-cloud/apptheory/v2/runtime"
)

func TestNotificationStatusMaterialization_DeletedSnapshotOmitted_M3(t *testing.T) {
	cfg := round11TestConfig()
	now := time.Date(2026, 5, 31, 12, 0, 0, 0, time.UTC)
	statusID := cfg.BaseURL() + "/objects/deleted-status"

	notif := &storagemodels.Notification{
		ID:         "n-deleted",
		UserID:     "alice",
		ActorID:    "bob",
		Type:       models.NotificationTypeMention,
		TargetID:   statusID,
		TargetType: notificationTargetTypeStatus,
		CreatedAt:  now,
		Data: map[string]interface{}{
			notificationPostSnapshotKey: map[string]interface{}{
				"id":           statusID,
				"url":          cfg.BaseURL() + "/@bob/deleted-status",
				"content":      "<p>deleted body must not leak</p>",
				"createdAt":    now.Add(-time.Minute).Format(time.RFC3339),
				"visibility":   storagemodels.VisibilityPublic,
				"attributedTo": cfg.BaseURL() + "/users/bob",
			},
		},
	}

	handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{
		actorsByUser: notificationMaterializationActors(cfg, "alice", "bob"),
		objectsByID: map[string]storagemodels.Object{
			statusID: {
				ID:           statusID,
				Type:         activitypub.NoteType,
				Content:      "<p>live deleted body must not leak</p>",
				Published:    now.Add(-time.Minute),
				AttributedTo: cfg.BaseURL() + "/users/bob",
				Visibility:   storagemodels.VisibilityPublic,
			},
		},
		tombstonesByObjectID: map[string]storagemodels.Tombstone{
			statusID: {
				ID:         statusID,
				Type:       "Tombstone",
				FormerType: activitypub.NoteType,
				DeletedBy:  cfg.BaseURL() + "/users/bob",
				Deleted:    now,
				CreatedAt:  now,
			},
		},
	})
	handler.registry = &RegistryStub{
		NotificationsSvc: &NotificationsServiceStub{
			ListNotificationsFunc: func(context.Context, *notifications.ListNotificationsQuery) (*notifications.NotificationListResult, error) {
				return &notifications.NotificationListResult{Notifications: []*storagemodels.Notification{notif}}, nil
			},
			GetNotificationFunc: func(context.Context, *notifications.GetNotificationQuery) (*storagemodels.Notification, error) {
				return notif, nil
			},
		},
	}

	listResp := requireStatus(t, http.StatusOK)(handler.HandleGetNotificationsLift(notificationReadContext(t, cfg, "alice", http.MethodGet, "/api/v1/notifications")))
	require.NotContains(t, string(listResp.Body), "deleted body")
	var listed []models.Notification
	require.NoError(t, json.Unmarshal(listResp.Body, &listed))
	require.Len(t, listed, 1)
	require.Nil(t, listed[0].Status)

	singleCtx := notificationReadContext(t, cfg, "alice", http.MethodGet, "/api/v1/notifications/n-deleted")
	singleCtx.Params["id"] = "n-deleted"
	singleResp := requireStatus(t, http.StatusOK)(handler.HandleGetNotificationLift(singleCtx))
	require.NotContains(t, string(singleResp.Body), "deleted body")
	var single models.Notification
	require.NoError(t, json.Unmarshal(singleResp.Body, &single))
	require.Nil(t, single.Status)
}

func TestNotificationStatusMaterialization_ObjectPathVisibility_M3(t *testing.T) {
	cfg := round11TestConfig()
	now := time.Date(2026, 5, 31, 12, 30, 0, 0, time.UTC)
	privateID := "private-status"
	directID := "direct-status"
	publicID := "public-status"
	unlistedID := "unlisted-status"
	snapshotDeniedID := "snapshot-denied-status"

	notificationsByUser := map[string][]*storagemodels.Notification{
		"mallory": {
			notificationForStatus("n-mallory-private", "mallory", privateID, now),
			notificationForStatus("n-mallory-direct", "mallory", directID, now),
			notificationForStatus("n-mallory-public", "mallory", publicID, now),
			notificationForStatus("n-mallory-unlisted", "mallory", unlistedID, now),
			notificationWithSnapshot("n-mallory-denied-snapshot", "mallory", snapshotDeniedID, now, map[string]interface{}{
				"id":           snapshotDeniedID,
				"content":      "<p>denied snapshot must not fall through</p>",
				"createdAt":    now.Format(time.RFC3339),
				"visibility":   storagemodels.VisibilityPrivate,
				"attributedTo": cfg.BaseURL() + "/users/bob",
			}),
		},
		"alice": {
			notificationForStatus("n-alice-private", "alice", privateID, now),
		},
		"carol": {
			notificationForStatus("n-carol-direct", "carol", directID, now),
		},
	}

	handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{
		actorsByUser: notificationMaterializationActors(cfg, "alice", "bob", "carol", "mallory"),
		relationshipRecords: []storagemodels.RelationshipRecord{
			{
				PK:    "FOLLOW#alice",
				SK:    "FOLLOWING#bob",
				State: storagemodels.RelationshipAccepted,
			},
		},
		objectsByID: map[string]storagemodels.Object{
			privateID: {
				ID:           privateID,
				Type:         activitypub.NoteType,
				Content:      "<p>private body</p>",
				Published:    now,
				AttributedTo: cfg.BaseURL() + "/users/bob",
				Visibility:   storagemodels.VisibilityPrivate,
				To:           []string{cfg.BaseURL() + "/users/bob/followers"},
			},
			directID: {
				ID:           directID,
				Type:         activitypub.NoteType,
				Content:      "<p>direct body</p>",
				Published:    now,
				AttributedTo: cfg.BaseURL() + "/users/bob",
				Visibility:   storagemodels.VisibilityDirect,
				To:           []string{cfg.ActorURL("carol")},
			},
			publicID: {
				ID:           publicID,
				Type:         activitypub.NoteType,
				Content:      "<p>public body</p>",
				Published:    now,
				AttributedTo: cfg.BaseURL() + "/users/bob",
				Visibility:   storagemodels.VisibilityPublic,
			},
			unlistedID: {
				ID:           unlistedID,
				Type:         activitypub.NoteType,
				Content:      "<p>unlisted body</p>",
				Published:    now,
				AttributedTo: cfg.BaseURL() + "/users/bob",
				Visibility:   storagemodels.VisibilityUnlisted,
			},
			snapshotDeniedID: {
				ID:           snapshotDeniedID,
				Type:         activitypub.NoteType,
				Content:      "<p>fallback object body must not leak</p>",
				Published:    now,
				AttributedTo: cfg.BaseURL() + "/users/bob",
				Visibility:   storagemodels.VisibilityPublic,
			},
		},
	})
	handler.registry = &RegistryStub{
		NotificationsSvc: &NotificationsServiceStub{
			ListNotificationsFunc: func(_ context.Context, query *notifications.ListNotificationsQuery) (*notifications.NotificationListResult, error) {
				return &notifications.NotificationListResult{Notifications: notificationsByUser[query.UserID]}, nil
			},
		},
	}

	mallory := notificationsByID(notificationListForUser(t, handler, cfg, "mallory"))
	require.Nil(t, mallory["n-mallory-private"].Status)
	require.Nil(t, mallory["n-mallory-direct"].Status)
	require.Nil(t, mallory["n-mallory-denied-snapshot"].Status)
	require.NotNil(t, mallory["n-mallory-public"].Status)
	require.Equal(t, storagemodels.VisibilityPublic, mallory["n-mallory-public"].Status.Visibility)
	require.NotNil(t, mallory["n-mallory-unlisted"].Status)
	require.Equal(t, storagemodels.VisibilityUnlisted, mallory["n-mallory-unlisted"].Status.Visibility)

	alice := notificationsByID(notificationListForUser(t, handler, cfg, "alice"))
	require.NotNil(t, alice["n-alice-private"].Status)
	require.Equal(t, "<p>private body</p>", alice["n-alice-private"].Status.Content)
	require.Equal(t, storagemodels.VisibilityPrivate, alice["n-alice-private"].Status.Visibility)

	carol := notificationsByID(notificationListForUser(t, handler, cfg, "carol"))
	require.NotNil(t, carol["n-carol-direct"].Status)
	require.Equal(t, "<p>direct body</p>", carol["n-carol-direct"].Status.Content)
	require.Equal(t, storagemodels.VisibilityDirect, carol["n-carol-direct"].Status.Visibility)
}

func notificationReadContext(t *testing.T, cfg *config.Config, username, method, path string) *apptheory.Context {
	t.Helper()

	token := round11SignAccessToken(t, cfg.JWTSecret, username, []string{"read:notifications", auth.ScopeRead})
	ctx, err := round10NewLiftContext(method, path, map[string]string{"Authorization": "Bearer " + token}, nil, nil)
	require.NoError(t, err)
	return ctx
}

func notificationListForUser(t *testing.T, h *Handler, cfg *config.Config, username string) []models.Notification {
	t.Helper()

	resp := requireStatus(t, http.StatusOK)(h.HandleGetNotificationsLift(notificationReadContext(t, cfg, username, http.MethodGet, "/api/v1/notifications")))
	var out []models.Notification
	require.NoError(t, json.Unmarshal(resp.Body, &out))
	return out
}

func notificationsByID(notifications []models.Notification) map[string]models.Notification {
	out := make(map[string]models.Notification, len(notifications))
	for _, notification := range notifications {
		out[notification.ID] = notification
	}
	return out
}

func notificationForStatus(id, userID, statusID string, createdAt time.Time) *storagemodels.Notification {
	return &storagemodels.Notification{
		ID:         id,
		UserID:     userID,
		ActorID:    "bob",
		Type:       models.NotificationTypeMention,
		TargetID:   statusID,
		TargetType: notificationTargetTypeStatus,
		CreatedAt:  createdAt,
	}
}

func notificationWithSnapshot(id, userID, statusID string, createdAt time.Time, snapshot map[string]interface{}) *storagemodels.Notification {
	notification := notificationForStatus(id, userID, statusID, createdAt)
	notification.Data = map[string]interface{}{notificationPostSnapshotKey: snapshot}
	return notification
}

func notificationMaterializationActors(cfg *config.Config, usernames ...string) map[string]storagemodels.Actor {
	actors := make(map[string]storagemodels.Actor, len(usernames))
	for _, username := range usernames {
		actors[username] = storagemodels.Actor{
			Username: username,
			Actor: &activitypub.Actor{
				BaseObject: activitypub.BaseObject{
					ID:   cfg.ActorURL(username),
					Type: "Person",
				},
				PreferredUsername: username,
				Name:              username,
			},
		}
	}
	return actors
}

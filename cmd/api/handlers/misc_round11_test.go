package handlers

import (
	"context"
	"net/http"
	"reflect"
	"testing"
	"time"
	"unsafe"

	"github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/services/notifications"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/stretchr/testify/require"
)

func round11SetCostAggregates(repo *repositories.TrackingRepository, aggregates []*storagemodels.DynamoDBCostAggregation) {
	rv := reflect.ValueOf(repo).Elem().FieldByName("listAggregatedByPeriodFn")
	fn := func(ctx context.Context, period, operationType string, startTime, endTime time.Time, limit int, cursor string) ([]*storagemodels.DynamoDBCostAggregation, string, error) {
		return aggregates, "", nil
	}
	reflect.NewAt(rv.Type(), unsafe.Pointer(rv.UnsafeAddr())).Elem().Set(reflect.ValueOf(fn))
}

func TestMiscSearchAndHelpers_Round11(t *testing.T) {
	cfg := round10TestConfig()
	now := time.Now()
	state := &round10QueryState{
		actorsByUser: map[string]storagemodels.Actor{
			"alice": {
				Username: "alice",
				Actor: &activitypub.Actor{
					BaseObject:        activitypub.BaseObject{ID: "https://example.com/users/alice", Type: "Person"},
					PreferredUsername: "alice",
					Name:              "Alice",
				},
			},
		},
		objectList: []storagemodels.Object{
			{
				ID:           "obj-1",
				Type:         activitypub.NoteType,
				Content:      "hello world",
				Published:    now.Add(-1 * time.Hour),
				AttributedTo: "https://example.com/users/alice",
				URL:          cfg.BaseURL() + "/objects/obj-1",
			},
		},
		objectsByID: map[string]storagemodels.Object{
			cfg.BaseURL() + "/objects/obj-1": {
				ID:           cfg.BaseURL() + "/objects/obj-1",
				Type:         activitypub.NoteType,
				Content:      "hello world",
				Published:    now.Add(-1 * time.Hour),
				AttributedTo: "https://example.com/users/alice",
				URL:          cfg.BaseURL() + "/objects/obj-1",
			},
		},
	}
	handler, _, _ := round11NewHandler(t, cfg, state)

	ctxSearch, err := round10NewLiftContext(http.MethodGet, "/api/v1/search", nil, map[string]string{"q": "hello"}, nil)
	require.NoError(t, err)
	requireStatus(t, http.StatusOK)(handler.HandleSearchLift(ctxSearch))

	ctxSearch2, err := round10NewLiftContext(http.MethodGet, "/api/v2/search", nil, map[string]string{"q": "hello"}, nil)
	require.NoError(t, err)
	requireStatus(t, http.StatusOK)(handler.HandleSearchV2Lift(ctxSearch2))

	objResult := handler.convertObjectToStatusResult(&state.objectList[0])
	require.NotNil(t, objResult)
	mapResult := handler.convertObjectToStatusResult(map[string]any{"id": "m1", "url": "https://example.com/1", "content": "hi", "attributedTo": "alice", "published": now})
	require.NotNil(t, mapResult)
	require.Nil(t, handler.convertObjectToStatusResult("bad"))

	status := handler.convertStatusResultToAPI(ctxSearch, &storage.StatusSearchResult{
		StatusID:  "s1",
		Content:   "hi",
		URL:       "https://example.com/s1",
		Published: now,
		AuthorID:  "https://example.com/users/alice",
	}, "")
	require.Equal(t, "hello", status.Content)
}

func TestMiscNotificationsAndGrouping_Round11(t *testing.T) {
	cfg := round10TestConfig()
	now := time.Now()
	state := &round10QueryState{
		actorsByUser: map[string]storagemodels.Actor{
			"alice": {
				Username: "alice",
				Actor: &activitypub.Actor{
					BaseObject:        activitypub.BaseObject{ID: "https://example.com/users/alice", Type: "Person"},
					PreferredUsername: "alice",
					Name:              "Alice",
				},
			},
		},
		objectsByID: map[string]storagemodels.Object{
			"status-1": {
				ID:           "status-1",
				Type:         activitypub.NoteType,
				Content:      "hello",
				Published:    now.Add(-1 * time.Hour),
				AttributedTo: "https://example.com/users/alice",
			},
		},
		notificationsByID: map[string]storagemodels.Notification{
			"notif-1": {
				ID:         "notif-1",
				UserID:     "alice",
				ActorID:    "alice",
				Type:       models.NotificationTypeMention,
				TargetID:   "status-1",
				TargetType: "status",
				CreatedAt:  now.Add(-1 * time.Hour),
			},
		},
	}
	handler, _, _ := round11NewHandler(t, cfg, state)

	handler.registry = &RegistryStub{
		NotificationsSvc: &NotificationsServiceStub{
			ListNotificationsFunc: func(ctx context.Context, query *notifications.ListNotificationsQuery) (*notifications.NotificationListResult, error) {
				return &notifications.NotificationListResult{
					Notifications: []*storagemodels.Notification{
						{
							ID:        "notif-1",
							Type:      models.NotificationTypeMention,
							ActorID:   "alice",
							TargetID:  "status-1",
							IsRead:    false,
							CreatedAt: now.Add(-30 * time.Minute),
							UserID:    "alice",
						},
					},
					Pagination: &interfaces.PaginatedResult[*storagemodels.Notification]{NextCursor: "next"},
				}, nil
			},
			ClearNotificationsFunc: func(ctx context.Context, cmd *notifications.ClearCommand) (*notifications.ClearResult, error) {
				return &notifications.ClearResult{ClearedCount: 3}, nil
			},
			MarkAsReadFunc: func(ctx context.Context, cmd *notifications.MarkAsReadCommand) (*notifications.NotificationResult, error) {
				return &notifications.NotificationResult{Notification: &storagemodels.Notification{ID: cmd.NotificationID}}, nil
			},
		},
		NotesSvc: &NotesServiceStub{
			GetNoteFunc: func(ctx context.Context, statusID string) (*storagemodels.Status, error) {
				return &storagemodels.Status{StatusID: statusID, AuthorUsername: "alice", AuthorID: "https://example.com/users/alice", Content: "note"}, nil
			},
		},
		AccountsSvc: &AccountsServiceStub{
			GetAccountFunc: func(ctx context.Context, username string) (*storage.Account, error) {
				return &storage.Account{Actor: &activitypub.Actor{BaseObject: activitypub.BaseObject{ID: "https://example.com/users/" + username}, PreferredUsername: username}}, nil
			},
		},
	}

	readToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{"read:notifications", auth.ScopeRead})
	readHeaders := map[string]string{"Authorization": "Bearer " + readToken, "Host": "example.com"}

	ctxList, err := round10NewLiftContext(http.MethodGet, "/api/v1/notifications", readHeaders, map[string]string{"limit": "5"}, nil)
	require.NoError(t, err)
	requireStatus(t, http.StatusOK)(handler.HandleGetNotificationsLift(ctxList))

	ctxSingle, err := round10NewLiftContext(http.MethodGet, "/api/v1/notifications/notif-1", readHeaders, nil, nil)
	require.NoError(t, err)
	ctxSingle.Params["id"] = "notif-1"
	requireStatus(t, http.StatusOK)(handler.HandleGetNotificationLift(ctxSingle))

	writeToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{"write:notifications", auth.ScopeWrite})
	writeHeaders := map[string]string{"Authorization": "Bearer " + writeToken}

	ctxClear, err := round10NewLiftContext(http.MethodPost, "/api/v1/notifications/clear", writeHeaders, nil, nil)
	require.NoError(t, err)
	requireStatus(t, http.StatusNoContent)(handler.HandleClearNotificationsLift(ctxClear))

	ctxDismiss, err := round10NewLiftContext(http.MethodPost, "/api/v1/notifications/notif-1/dismiss", writeHeaders, nil, nil)
	require.NoError(t, err)
	ctxDismiss.Params["id"] = "notif-1"
	requireStatus(t, http.StatusNoContent)(handler.HandleDismissNotificationLift(ctxDismiss))

	ctxGrouped, err := round10NewLiftContext(http.MethodGet, "/api/v2/notifications/grouped", readHeaders, map[string]string{"include_all": "true", "group_by_type": "false"}, nil)
	require.NoError(t, err)
	requireStatus(t, http.StatusOK)(handler.HandleGetGroupedNotificationsLift(ctxGrouped))

	ctxMarkGroup, err := round10NewLiftContext(http.MethodPost, "/api/v2/notifications/groups/group-1/read", writeHeaders, nil, nil)
	require.NoError(t, err)
	ctxMarkGroup.Params["group_id"] = "group-1"
	requireStatus(t, http.StatusOK)(handler.HandleMarkGroupAsReadLift(ctxMarkGroup))
}

func TestMiscInstanceAndCost_Round11(t *testing.T) {
	cfg := round10TestConfig()
	cfg.Stage = "dev"
	cfg.CostHistoryTableName = "cost-table"
	cfg.AdminUsername = "alice"
	now := time.Now()

	state := &round10QueryState{
		instanceRules: []storagemodels.InstanceRule{{ID: "1", Text: "Rule"}},
		vapidKeys: &storage.VAPIDKeys{
			PublicKey:  "pubkey",
			PrivateKey: "privkey",
			Subject:    "mailto:admin@example.com",
			CreatedAt:  now.Add(-24 * time.Hour),
			UpdatedAt:  now,
		},
		actorsByUser: map[string]storagemodels.Actor{
			"alice": {
				Username: "alice",
				Actor: &activitypub.Actor{
					BaseObject:        activitypub.BaseObject{ID: "https://example.com/users/alice", Type: "Person"},
					PreferredUsername: "alice",
					Name:              "Alice",
				},
			},
		},
	}
	handler, _, _ := round11NewHandler(t, cfg, state)

	aggregates := []*storagemodels.DynamoDBCostAggregation{
		{
			WindowStart:             now.Add(-24 * time.Hour),
			TotalOperations:         10,
			TotalReadCapacityUnits:  5,
			TotalWriteCapacityUnits: 2,
			AverageDuration:         0.25,
			TotalCostDollars:        1.25,
		},
	}
	costRepo := handler.repos.Cost()
	round11SetCostAggregates(costRepo, aggregates)

	ctxCost, err := round10NewLiftContext(http.MethodGet, "/api/v1/instance/costs", nil, nil, nil)
	require.NoError(t, err)
	requireStatus(t, http.StatusOK)(handler.HandleGetInstanceCostsLift(ctxCost))

	ctxConfig, err := round10NewLiftContext(http.MethodGet, "/api/v1/instance/configuration", nil, nil, nil)
	require.NoError(t, err)
	requireStatus(t, http.StatusOK)(handler.HandleGetInstanceConfigurationLift(ctxConfig))

	ctxV2, err := round10NewLiftContext(http.MethodGet, "/api/v2/instance", nil, nil, nil)
	require.NoError(t, err)
	requireStatus(t, http.StatusOK)(handler.HandleGetInstanceV2Lift(ctxV2))

	require.Equal(t, "0", handler.getUniqueAccountsForDay(ctxV2, "bad-day"))
	require.GreaterOrEqual(t, handler.getActiveMonthlyUsers(ctxV2), 0)

	admin := handler.getAdminAccount(ctxV2)
	require.NotNil(t, admin)

	require.NotEmpty(t, handler.getAvatarURL(&activitypub.Actor{}))
	require.Nil(t, handler.formatLastStatusTime(nil))
}

package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/services/notes"
	"github.com/equaltoai/lesser/pkg/services/notifications"
	"github.com/equaltoai/lesser/pkg/storage"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestMisc_Search_StatusURL_And_Params_Round12(t *testing.T) {
	cfg := round11TestConfig()
	now := time.Now()

	objID := cfg.BaseURL() + "/objects/obj-1"
	state := &round10QueryState{
		actorsByUser: map[string]storagemodels.Actor{
			"alice": {
				Username: "alice",
				Actor: &activitypub.Actor{
					BaseObject:        activitypub.BaseObject{ID: cfg.BaseURL() + "/users/alice", Type: "Person"},
					PreferredUsername: "alice",
					Name:              "Alice",
				},
			},
		},
		statusByID: map[string]storagemodels.Status{
			"obj-1": {
				StatusID:       "obj-1",
				AuthorID:       cfg.BaseURL() + "/users/alice",
				AuthorUsername: "alice",
				Content:        "hello world",
				Visibility:     storagemodels.VisibilityPublic,
				PublishedAt:    now.Add(-1 * time.Hour),
				CreatedAt:      now.Add(-1 * time.Hour),
				UpdatedAt:      now.Add(-1 * time.Hour),
				Note: &activitypub.Note{
					BaseObject:   activitypub.BaseObject{ID: objID, Type: activitypub.NoteType},
					Content:      "hello world",
					AttributedTo: cfg.BaseURL() + "/users/alice",
				},
			},
		},
	}
	handler, _, _ := round11NewHandler(t, cfg, state)

	token := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeRead})
	headers := map[string]string{"Authorization": "Bearer " + token}

	t.Run("url search hits searchStatusByURL", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/search", headers, map[string]string{
			"q":    objID,
			"type": "statuses",
		}, nil)
		require.NoError(t, err)

		resp := requireStatus(t, http.StatusOK)(handler.HandleSearchLift(ctx))

		var result models.SearchResult
		require.NoError(t, json.Unmarshal(resp.Body, &result))
		require.Len(t, result.Statuses, 1)
	})

	t.Run("parseSearchParams handles bad limit and invalid query", func(t *testing.T) {
		ctxBadLimit, err := round10NewLiftContext(http.MethodGet, "/api/v1/search", nil, map[string]string{
			"q":     "hello",
			"limit": "not-a-number",
		}, nil)
		require.NoError(t, err)

		params, resp, err := handler.parseSearchParams(ctxBadLimit)
		require.NoError(t, err)
		require.Nil(t, resp)
		require.Equal(t, 20, params.Limit)

		ctxInvalidQuery, err := round10NewLiftContext(http.MethodGet, "/api/v1/search", nil, map[string]string{
			"q": "",
		}, nil)
		require.NoError(t, err)

		_, resp, err = handler.parseSearchParams(ctxInvalidQuery)
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusBadRequest, resp.Status)
	})

	t.Run("HandleSearchLift returns validation error", func(t *testing.T) {
		ctxInvalidQuery, err := round10NewLiftContext(http.MethodGet, "/api/v1/search", nil, map[string]string{
			"q": "",
		}, nil)
		require.NoError(t, err)

		requireStatus(t, http.StatusBadRequest)(handler.HandleSearchLift(ctxInvalidQuery))
	})

	t.Run("convertActorToAccount uses stored public identity for local actors", func(t *testing.T) {
		actor := state.actorsByUser["alice"].Actor
		account := handler.convertActorToAccount(context.Background(), actor)
		require.Equal(t, common.GenerateNumericID("alice"), account.ID)
		require.Equal(t, "alice", account.Username)
	})
}

func TestMisc_Search_HashtagPlaceholder_And_TagHistory_Round12(t *testing.T) {
	cfg := round11TestConfig()
	handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

	ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/search", nil, map[string]string{
		"q":    "#GoLang",
		"type": "hashtags",
	}, nil)
	require.NoError(t, err)

	resp := requireStatus(t, http.StatusOK)(handler.HandleSearchLift(ctx))

	var result models.SearchResult
	require.NoError(t, json.Unmarshal(resp.Body, &result))
	require.Len(t, result.Hashtags, 1)
	require.Equal(t, "golang", result.Hashtags[0].Name)

	tag := handler.convertHashtagToTag(ctx, storage.Hashtag{
		Name: "golang",
		URL:  cfg.BaseURL() + "/tags/golang",
	})
	require.Equal(t, "golang", tag.Name)
	require.Len(t, tag.History, 7)

	// addPlaceholderHashtag should not add when results already present.
	placeholderResult := &models.SearchResult{Hashtags: []models.Tag{{Name: "existing"}}}
	handler.addPlaceholderHashtag("#GoLang", placeholderResult)
	require.Len(t, placeholderResult.Hashtags, 1)
}

func TestMisc_Notifications_AttachStatusAndAuthor_Round12(t *testing.T) {
	cfg := round11TestConfig()
	now := time.Now()

	state := &round10QueryState{
		actorsByUser: map[string]storagemodels.Actor{
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
		objectsByID: map[string]storagemodels.Object{
			"status-1": {
				ID:           "status-1",
				Type:         activitypub.NoteType,
				Content:      "hello",
				Published:    now.Add(-1 * time.Hour),
				AttributedTo: cfg.BaseURL() + "/users/bob",
			},
			"obj-1": {
				ID:           "obj-1",
				Type:         "Image", // non-note objects map into map[string]any
				Content:      "image content",
				Published:    now.Add(-2 * time.Hour),
				AttributedTo: cfg.BaseURL() + "/users/bob",
			},
		},
	}
	handler, _, _ := round11NewHandler(t, cfg, state)

	ctx, err := round10NewLiftContext(http.MethodGet, "/test", map[string]string{"Host": "example.com"}, nil, nil)
	require.NoError(t, err)

	t.Run("note object attaches status and author", func(t *testing.T) {
		notif := &storage.Notification{
			ID:        "n1",
			Type:      models.NotificationTypeMention,
			AccountID: "alice",
			StatusID:  "status-1",
			CreatedAt: now,
		}
		apiNotif := handler.convertSingleNotification(ctx, notif)
		require.NotNil(t, apiNotif)
		require.NotNil(t, apiNotif.Status)
	})

	t.Run("non-note object uses map branch", func(t *testing.T) {
		notif := &storage.Notification{
			ID:        "n2",
			Type:      models.NotificationTypeMention,
			AccountID: "alice",
			StatusID:  "obj-1",
			CreatedAt: now,
		}
		apiNotif := handler.convertSingleNotification(ctx, notif)
		require.NotNil(t, apiNotif)
		require.NotNil(t, apiNotif.Status)
	})

	t.Run("actor lookup failure falls back to synthetic actor", func(t *testing.T) {
		errState := &round10QueryState{firstErrorOnce: errors.New("boom")}
		errHandler, _, _ := round11NewHandler(t, cfg, errState)

		apiNotif := errHandler.convertSingleNotification(ctx, &storage.Notification{
			ID:        "n3",
			Type:      models.NotificationTypeMention,
			AccountID: "alice",
			StatusID:  "status-1",
			CreatedAt: now,
		})
		require.NotNil(t, apiNotif)
		require.Equal(t, "alice", apiNotif.Account.Username)
	})

	t.Run("invalid status id is excluded", func(t *testing.T) {
		apiNotif := handler.convertSingleNotification(ctx, &storage.Notification{
			ID:        "n4",
			Type:      models.NotificationTypeMention,
			AccountID: "alice",
			StatusID:  "",
			CreatedAt: now,
		})
		require.NotNil(t, apiNotif)
		require.Nil(t, apiNotif.Status)
	})

	t.Run("snapshot attaches status without object fetch", func(t *testing.T) {
		errState := &round10QueryState{
			actorsByUser: state.actorsByUser,
			firstErrorPK: map[string]error{
				"object#status-snapshot": errors.New("unexpected object fetch"),
			},
		}
		snapshotHandler, _, _ := round11NewHandler(t, cfg, errState)

		apiNotif := snapshotHandler.convertSingleNotification(ctx, &storage.Notification{
			ID:        "n-snapshot",
			Type:      models.NotificationTypeMention,
			AccountID: "alice",
			StatusID:  "status-snapshot",
			CreatedAt: now,
			Data: map[string]interface{}{
				"postSnapshot": map[string]interface{}{
					"id":           "https://example.com/objects/status-snapshot",
					"url":          "https://example.com/@bob/status-snapshot",
					"content":      "<p>snapshot body</p>",
					"createdAt":    now.Add(-30 * time.Minute).Format(time.RFC3339),
					"visibility":   "private",
					"inReplyToId":  "https://example.com/objects/root",
					"attributedTo": cfg.BaseURL() + "/users/bob",
				},
			},
		})
		require.NotNil(t, apiNotif)
		require.NotNil(t, apiNotif.Status)
		require.Equal(t, "<p>snapshot body</p>", apiNotif.Status.Content)
		require.Equal(t, "https://example.com/@bob/status-snapshot", apiNotif.Status.URL)
		require.Equal(t, "private", apiNotif.Status.Visibility)
	})

	t.Run("object fetch error leaves status unset", func(t *testing.T) {
		errState := &round10QueryState{
			actorsByUser: map[string]storagemodels.Actor{
				"alice": {
					Username: "alice",
					Actor: &activitypub.Actor{
						BaseObject:        activitypub.BaseObject{ID: cfg.BaseURL() + "/users/alice", Type: "Person"},
						PreferredUsername: "alice",
						Name:              "Alice",
					},
				},
			},
			firstErrorPK: map[string]error{
				"object#status-1": errors.New("boom"),
			},
		}
		errHandler, _, _ := round11NewHandler(t, cfg, errState)

		apiNotif := errHandler.convertSingleNotification(ctx, &storage.Notification{
			ID:        "n5",
			Type:      models.NotificationTypeMention,
			AccountID: "alice",
			StatusID:  "status-1",
			CreatedAt: now,
		})
		require.NotNil(t, apiNotif)
		require.Nil(t, apiNotif.Status)
	})
}

func TestMisc_Instance_V2_VAPIDBranches_Round12(t *testing.T) {
	now := time.Now()

	t.Run("non-production auto-generates VAPID keys when missing", func(t *testing.T) {
		cfg := round11TestConfig()
		cfg.Stage = "dev"

		state := &round10QueryState{forceVapidNotFound: true}
		handler, _, _ := round11NewHandler(t, cfg, state)

		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v2/instance", nil, nil, nil)
		require.NoError(t, err)

		requireStatus(t, http.StatusOK)(handler.HandleGetInstanceV2Lift(ctx))
	})

	t.Run("production requires VAPID keys", func(t *testing.T) {
		cfg := round11TestConfig()
		cfg.Stage = "production"

		state := &round10QueryState{forceVapidNotFound: true}
		handler, _, _ := round11NewHandler(t, cfg, state)

		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v2/instance", nil, nil, nil)
		require.NoError(t, err)

		requireStatus(t, http.StatusInternalServerError)(handler.HandleGetInstanceV2Lift(ctx))
	})

	t.Run("generateAndStoreVAPIDKeys uses localhost when domain missing", func(t *testing.T) {
		cfg := round11TestConfig()
		cfg.Domain = ""
		cfg.Stage = "dev"

		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
		keys, err := handler.generateAndStoreVAPIDKeys(context.Background())
		require.NoError(t, err)
		require.NotEmpty(t, keys.PublicKey)
		require.Contains(t, keys.Subject, "mailto:admin@localhost")
	})

	t.Run("instance configuration includes configured VAPID key", func(t *testing.T) {
		cfg := round11TestConfig()
		cfg.VAPIDPublicKey = "public"

		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/instance/configuration", nil, nil, nil)
		require.NoError(t, err)

		resp := requireStatus(t, http.StatusOK)(handler.HandleGetInstanceConfigurationLift(ctx))

		var body map[string]any
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		require.Equal(t, "public", body["vapid_key"])
	})

	t.Run("avatar and last_status helpers cover branches", func(t *testing.T) {
		cfg := round11TestConfig()
		cfg.AdminUsername = ""

		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
		ctx, err := round10NewLiftContext(http.MethodGet, "/test", nil, nil, nil)
		require.NoError(t, err)

		require.Nil(t, handler.getAdminAccount(ctx))

		require.Equal(t, "https://cdn.example.com/a.png", handler.getAvatarURL(&activitypub.Actor{Icon: &activitypub.Image{URL: "https://cdn.example.com/a.png"}}))

		last := now.Add(-24 * time.Hour)
		formatted := handler.formatLastStatusTime(&last)
		require.NotNil(t, formatted)
	})
}

func TestMisc_NotificationHandlers_ErrorBranches_Round12(t *testing.T) {
	cfg := round11TestConfig()

	t.Run("missing services return service unavailable", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
		handler.registry = &RegistryStub{}

		readToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{"read:notifications", auth.ScopeRead})
		readHeaders := map[string]string{"Authorization": "Bearer " + readToken}

		ctxList, err := round10NewLiftContext(http.MethodGet, "/api/v1/notifications", readHeaders, nil, nil)
		require.NoError(t, err)
		requireStatus(t, http.StatusServiceUnavailable)(handler.HandleGetNotificationsLift(ctxList))

		writeToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{"write:notifications", auth.ScopeWrite})
		writeHeaders := map[string]string{"Authorization": "Bearer " + writeToken}

		ctxClear, err := round10NewLiftContext(http.MethodPost, "/api/v1/notifications/clear", writeHeaders, nil, nil)
		require.NoError(t, err)
		requireStatus(t, http.StatusServiceUnavailable)(handler.HandleClearNotificationsLift(ctxClear))
	})

	t.Run("dismiss handles not found vs internal error", func(t *testing.T) {
		now := time.Now()
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

		writeToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{"write:notifications", auth.ScopeWrite})
		writeHeaders := map[string]string{"Authorization": "Bearer " + writeToken}

		handler.registry = &RegistryStub{
			NotificationsSvc: &NotificationsServiceStub{
				MarkAsReadFunc: func(ctx context.Context, cmd *notifications.MarkAsReadCommand) (*notifications.NotificationResult, error) {
					return nil, errors.New("not found")
				},
			},
		}

		ctxNotFound, err := round10NewLiftContext(http.MethodPost, "/api/v1/notifications/n1/dismiss", writeHeaders, nil, nil)
		require.NoError(t, err)
		ctxNotFound.Params["id"] = "n1"
		requireStatus(t, http.StatusNotFound)(handler.HandleDismissNotificationLift(ctxNotFound))

		handler.registry = &RegistryStub{
			NotificationsSvc: &NotificationsServiceStub{
				MarkAsReadFunc: func(ctx context.Context, cmd *notifications.MarkAsReadCommand) (*notifications.NotificationResult, error) {
					return nil, errors.New("boom")
				},
			},
		}

		ctxErr, err := round10NewLiftContext(http.MethodPost, "/api/v1/notifications/n1/dismiss", writeHeaders, nil, nil)
		require.NoError(t, err)
		ctxErr.Params["id"] = "n1"
		requireStatus(t, http.StatusInternalServerError)(handler.HandleDismissNotificationLift(ctxErr))

		// Also cover HandleGetNotificationLift status attach path.
		handler.registry = &RegistryStub{
			NotesSvc: &NotesServiceStub{
				GetNoteWithViewerFunc: func(ctx context.Context, query *notes.GetNoteQuery) (*storagemodels.Status, error) {
					statusID := ""
					if query != nil {
						statusID = query.StatusID
					}
					return &storagemodels.Status{
						StatusID:       statusID,
						AuthorUsername: "alice",
						AuthorID:       cfg.BaseURL() + "/users/alice",
						Note: &activitypub.Note{
							BaseObject: activitypub.BaseObject{ID: statusID, Type: activitypub.NoteType},
							Content:    "note",
						},
					}, nil
				},
			},
			AccountsSvc: &AccountsServiceStub{
				GetAccountFunc: func(ctx context.Context, username string) (*storage.Account, error) {
					return &storage.Account{
						Actor: &activitypub.Actor{
							BaseObject:        activitypub.BaseObject{ID: cfg.BaseURL() + "/users/" + username, Type: "Person"},
							PreferredUsername: username,
						},
					}, nil
				},
			},
		}

		state := &round10QueryState{
			notificationsByID: map[string]storagemodels.Notification{
				"notif-1": {
					ID:         "notif-1",
					UserID:     "alice",
					ActorID:    "alice",
					Type:       models.NotificationTypeMention,
					TargetID:   "status-1",
					TargetType: "status",
					CreatedAt:  now,
				},
			},
			actorsByUser: map[string]storagemodels.Actor{
				"alice": {
					Username: "alice",
					Actor: &activitypub.Actor{
						BaseObject:        activitypub.BaseObject{ID: cfg.BaseURL() + "/users/alice", Type: "Person"},
						PreferredUsername: "alice",
						Name:              "Alice",
					},
				},
			},
		}
		handlerWithNotif, _, _ := round11NewHandler(t, cfg, state)
		handlerWithNotif.registry = handler.registry

		readToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{"read:notifications", auth.ScopeRead})
		readHeaders := map[string]string{"Authorization": "Bearer " + readToken}

		ctxGet, err := round10NewLiftContext(http.MethodGet, "/api/v1/notifications/notif-1", readHeaders, nil, nil)
		require.NoError(t, err)
		ctxGet.Params["id"] = "notif-1"
		respGet := requireStatus(t, http.StatusOK)(handlerWithNotif.HandleGetNotificationLift(ctxGet))

		var apiNotif models.Notification
		require.NoError(t, json.Unmarshal(respGet.Body, &apiNotif))
		require.NotNil(t, apiNotif.Status)
	})

	t.Run("mark group as read error path", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

		writeToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{"write:notifications"})
		writeHeaders := map[string]string{"Authorization": "Bearer " + writeToken}

		handler.registry = &RegistryStub{
			NotificationsSvc: &NotificationsServiceStub{
				MarkAsReadFunc: func(ctx context.Context, cmd *notifications.MarkAsReadCommand) (*notifications.NotificationResult, error) {
					return nil, errors.New("boom")
				},
			},
		}

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v2/notifications/groups/g1/read", writeHeaders, nil, nil)
		require.NoError(t, err)
		ctx.Params["group_id"] = "g1"

		requireStatus(t, http.StatusInternalServerError)(handler.HandleMarkGroupAsReadLift(ctx))
	})
}

func TestMisc_NotificationFiltersAndGroupingOptions_Round12(t *testing.T) {
	cfg := round11TestConfig()
	handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

	readToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{"read:notifications", auth.ScopeRead})
	readHeaders := map[string]string{"Authorization": "Bearer " + readToken}

	t.Run("types[] and exclude_types[]", func(t *testing.T) {
		handler.registry = &RegistryStub{
			NotificationsSvc: &NotificationsServiceStub{
				ListNotificationsFunc: func(ctx context.Context, query *notifications.ListNotificationsQuery) (*notifications.NotificationListResult, error) {
					require.Equal(t, []string{"mention"}, query.Types)
					require.Equal(t, []string{"reblog"}, query.ExcludeTypes)
					require.False(t, query.IncludeRead)
					return &notifications.NotificationListResult{}, nil
				},
			},
		}

		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/notifications", readHeaders, map[string]string{
			"types[]":         "mention",
			"exclude_types[]": "reblog",
		}, nil)
		require.NoError(t, err)

		requireStatus(t, http.StatusOK)(handler.HandleGetNotificationsLift(ctx))
	})

	t.Run("types and exclude_types", func(t *testing.T) {
		handler.registry = &RegistryStub{
			NotificationsSvc: &NotificationsServiceStub{
				ListNotificationsFunc: func(ctx context.Context, query *notifications.ListNotificationsQuery) (*notifications.NotificationListResult, error) {
					require.Equal(t, []string{"mention", "reblog"}, query.Types)
					require.Equal(t, []string{"favourite", "follow"}, query.ExcludeTypes)
					require.False(t, query.IncludeRead)
					return &notifications.NotificationListResult{}, nil
				},
			},
		}

		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/notifications", readHeaders, map[string]string{
			"types":         "mention,reblog",
			"exclude_types": "favourite,follow",
		}, nil)
		require.NoError(t, err)

		requireStatus(t, http.StatusOK)(handler.HandleGetNotificationsLift(ctx))
	})

	t.Run("parseGroupingOptions updates defaults", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v2/notifications/grouped", nil, map[string]string{
			"time_window":     "24",
			"max_group_size":  "5",
			"min_group_size":  "2",
			"sample_size":     "3",
			"group_by_type":   "false",
			"group_by_target": "false",
		}, nil)
		require.NoError(t, err)

		opts := handler.parseGroupingOptions(ctx)
		require.Equal(t, 24*time.Hour, opts.TimeWindow)
		require.Equal(t, 5, opts.MaxGroupSize)
		require.Equal(t, 2, opts.MinGroupSize)
		require.Equal(t, 3, opts.SampleSize)
		require.False(t, opts.GroupByType)
		require.False(t, opts.GroupByTarget)
	})

	t.Run("instance costs returns placeholder when table missing", func(t *testing.T) {
		cfg := round11TestConfig()
		cfg.CostHistoryTableName = ""
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/instance/costs", nil, nil, nil)
		require.NoError(t, err)

		resp := requireStatus(t, http.StatusOK)(handler.HandleGetInstanceCostsLift(ctx))

		var body map[string]any
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		require.Equal(t, "Cost tracking not configured", body["error"])
	})

	t.Run("active monthly users returns fallback on analytics error", func(t *testing.T) {
		cfg := round11TestConfig()
		state := &round10QueryState{allErrorOnce: errors.New("boom")}
		handler, _, _ := round11NewHandler(t, cfg, state)

		ctx, err := round10NewLiftContext(http.MethodGet, "/test", nil, nil, nil)
		require.NoError(t, err)

		require.Equal(t, 1, handler.getActiveMonthlyUsers(ctx))
	})

	t.Run("unique accounts returns 0 on instance repo error", func(t *testing.T) {
		cfg := round11TestConfig()
		state := &round10QueryState{firstErrorOnce: errors.New("boom")}
		handler, _, _ := round11NewHandler(t, cfg, state)

		ctx, err := round10NewLiftContext(http.MethodGet, "/test", nil, nil, nil)
		require.NoError(t, err)

		day := time.Now().Format(common.DateFormat)
		require.Equal(t, "0", handler.getUniqueAccountsForDay(ctx, day))
	})
}

func TestMisc_NotificationHandlers_MoreBranches_Round12(t *testing.T) {
	cfg := round11TestConfig()

	readToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{"read:notifications", auth.ScopeRead})
	readHeaders := map[string]string{"Authorization": "Bearer " + readToken}

	writeToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{"write:notifications", auth.ScopeWrite})
	writeHeaders := map[string]string{"Authorization": "Bearer " + writeToken}

	t.Run("list notifications service error", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
		handler.registry = &RegistryStub{
			NotificationsSvc: &NotificationsServiceStub{
				ListNotificationsFunc: func(ctx context.Context, query *notifications.ListNotificationsQuery) (*notifications.NotificationListResult, error) {
					return nil, errors.New("boom")
				},
			},
		}

		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/notifications", readHeaders, nil, nil)
		require.NoError(t, err)

		requireStatus(t, http.StatusInternalServerError)(handler.HandleGetNotificationsLift(ctx))
	})

	t.Run("clear notifications service error", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
		handler.registry = &RegistryStub{
			NotificationsSvc: &NotificationsServiceStub{
				ClearNotificationsFunc: func(ctx context.Context, cmd *notifications.ClearCommand) (*notifications.ClearResult, error) {
					return nil, errors.New("boom")
				},
			},
		}

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/notifications/clear", writeHeaders, nil, nil)
		require.NoError(t, err)

		requireStatus(t, http.StatusInternalServerError)(handler.HandleClearNotificationsLift(ctx))
	})

	t.Run("grouped notifications list error", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
		handler.registry = &RegistryStub{
			NotificationsSvc: &NotificationsServiceStub{
				ListNotificationsFunc: func(ctx context.Context, query *notifications.ListNotificationsQuery) (*notifications.NotificationListResult, error) {
					return nil, errors.New("boom")
				},
			},
		}

		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v2/notifications/grouped", readHeaders, nil, nil)
		require.NoError(t, err)

		requireStatus(t, http.StatusInternalServerError)(handler.HandleGetGroupedNotificationsLift(ctx))
	})

	t.Run("mark group missing group_id", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
		handler.registry = &RegistryStub{}

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v2/notifications/groups//read", writeHeaders, nil, nil)
		require.NoError(t, err)

		requireStatus(t, http.StatusBadRequest)(handler.HandleMarkGroupAsReadLift(ctx))
	})

	t.Run("extractStatusAuthor ignores missing attributed_to", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
		ctx, err := round10NewLiftContext(http.MethodGet, "/test", nil, nil, nil)
		require.NoError(t, err)

		require.Nil(t, handler.extractStatusAuthor(ctx, &activitypub.Note{
			BaseObject: activitypub.BaseObject{ID: "n1", Type: activitypub.NoteType},
			Content:    "hello",
		}))
	})
}

func TestMisc_GetNotification_ErrorBranches_Round12(t *testing.T) {
	cfg := round11TestConfig()
	now := time.Now()

	readToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{"read:notifications"})
	readHeaders := map[string]string{"Authorization": "Bearer " + readToken}

	t.Run("missing id param", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
		handler.registry = &RegistryStub{}

		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/notifications/", readHeaders, nil, nil)
		require.NoError(t, err)

		requireStatus(t, http.StatusBadRequest)(handler.HandleGetNotificationLift(ctx))
	})

	t.Run("insufficient scope", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
		handler.registry = &RegistryStub{}

		writeOnlyToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{"write:notifications"})
		headers := map[string]string{"Authorization": "Bearer " + writeOnlyToken}

		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/notifications/n1", headers, nil, nil)
		require.NoError(t, err)
		ctx.Params["id"] = "n1"

		requireStatus(t, http.StatusForbidden)(handler.HandleGetNotificationLift(ctx))
	})

	t.Run("notification not found", func(t *testing.T) {
		state := &round10QueryState{
			notFoundPKs: map[string]bool{
				"NOTIFICATION#missing": true,
			},
		}
		handler, _, _ := round11NewHandler(t, cfg, state)
		handler.registry = &RegistryStub{}

		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/notifications/missing", readHeaders, nil, nil)
		require.NoError(t, err)
		ctx.Params["id"] = "missing"

		requireStatus(t, http.StatusNotFound)(handler.HandleGetNotificationLift(ctx))
	})

	t.Run("ownership mismatch", func(t *testing.T) {
		state := &round10QueryState{
			notificationsByID: map[string]storagemodels.Notification{
				"n1": {
					ID:        "n1",
					UserID:    "bob",
					ActorID:   "alice",
					Type:      models.NotificationTypeMention,
					CreatedAt: now,
				},
			},
		}
		handler, _, _ := round11NewHandler(t, cfg, state)
		handler.registry = &RegistryStub{}

		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/notifications/n1", readHeaders, nil, nil)
		require.NoError(t, err)
		ctx.Params["id"] = "n1"

		requireStatus(t, http.StatusNotFound)(handler.HandleGetNotificationLift(ctx))
	})

	t.Run("actor lookup error", func(t *testing.T) {
		state := &round10QueryState{
			notificationsByID: map[string]storagemodels.Notification{
				"n1": {
					ID:        "n1",
					UserID:    "alice",
					ActorID:   "alice",
					Type:      models.NotificationTypeMention,
					CreatedAt: now,
				},
			},
			firstErrorPK: map[string]error{
				"ACTOR#alice": errors.New("boom"),
			},
		}
		handler, _, _ := round11NewHandler(t, cfg, state)
		handler.registry = &RegistryStub{}

		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/notifications/n1", readHeaders, nil, nil)
		require.NoError(t, err)
		ctx.Params["id"] = "n1"

		requireStatus(t, http.StatusOK)(handler.HandleGetNotificationLift(ctx))
	})

	t.Run("dismiss missing id and insufficient scope", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
		handler.registry = &RegistryStub{}

		ctxMissing, err := round10NewLiftContext(http.MethodPost, "/api/v1/notifications//dismiss", readHeaders, nil, nil)
		require.NoError(t, err)
		requireStatus(t, http.StatusBadRequest)(handler.HandleDismissNotificationLift(ctxMissing))

		ctxScope, err := round10NewLiftContext(http.MethodPost, "/api/v1/notifications/n1/dismiss", readHeaders, nil, nil)
		require.NoError(t, err)
		ctxScope.Params["id"] = "n1"
		requireStatus(t, http.StatusForbidden)(handler.HandleDismissNotificationLift(ctxScope))
	})

	t.Run("clear notifications insufficient scope", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
		handler.registry = &RegistryStub{}

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/notifications/clear", readHeaders, nil, nil)
		require.NoError(t, err)
		requireStatus(t, http.StatusForbidden)(handler.HandleClearNotificationsLift(ctx))
	})
}

func TestMisc_InstanceCosts_MonthlyAggregateError_Round12(t *testing.T) {
	cfg := round11TestConfig()
	cfg.CostHistoryTableName = "cost-table"

	state := &round10QueryState{allErrorOnce: errors.New("boom")}
	handler, _, _ := round11NewHandler(t, cfg, state)

	ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/instance/costs", nil, nil, nil)
	require.NoError(t, err)

	resp := requireStatus(t, http.StatusOK)(handler.HandleGetInstanceCostsLift(ctx))

	var body map[string]any
	require.NoError(t, json.Unmarshal(resp.Body, &body))

	monthData, ok := body["current_month"].(map[string]any)
	require.True(t, ok)
	require.Contains(t, monthData, "projected_cost_cents")
}

func TestMisc_DismissNotification_ServiceUnavailable_Round12(t *testing.T) {
	cfg := round11TestConfig()
	handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
	handler.registry = &RegistryStub{}

	writeToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{"write:notifications", auth.ScopeWrite})
	writeHeaders := map[string]string{"Authorization": "Bearer " + writeToken}

	ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/notifications/n1/dismiss", writeHeaders, nil, nil)
	require.NoError(t, err)
	ctx.Params["id"] = "n1"

	requireStatus(t, http.StatusServiceUnavailable)(handler.HandleDismissNotificationLift(ctx))
}

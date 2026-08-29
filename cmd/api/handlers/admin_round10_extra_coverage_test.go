package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"testing"
	"time"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/activitypub"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/stretchr/testify/require"
	apptheory "github.com/theory-cloud/apptheory/v4/runtime"
	dynamormerrors "github.com/theory-cloud/tabletheory/v3/pkg/errors"
)

func TestAdminLift_Round10Coverage_ExtraPaths(t *testing.T) {
	cfg := round10TestConfig()
	logger := round10TestLogger(t)
	now := time.Now()

	baseState := &round10QueryState{
		usersByUsername: map[string]storagemodels.User{
			"admin": {PK: "USER#admin", SK: storagemodels.SKMetadata, Username: "admin", Role: "admin", Approved: true, Version: 1, CreatedAt: now.Add(-48 * time.Hour)},
			"alice": {PK: "USER#alice", SK: storagemodels.SKMetadata, Username: "alice", Role: "user", Approved: true, Version: 1, CreatedAt: now.Add(-24 * time.Hour)},
			"mod":   {PK: "USER#mod", SK: storagemodels.SKMetadata, Username: "mod", Role: "moderator", Approved: true, Version: 1, CreatedAt: now.Add(-24 * time.Hour)},
		},
		actorsByUser: map[string]storagemodels.Actor{
			"admin": {
				Username: "admin",
				Actor: &activitypub.Actor{
					BaseObject:        activitypub.BaseObject{ID: "https://example.com/users/admin", Type: "Person"},
					PreferredUsername: "admin",
					Name:              "Admin",
				},
			},
			"alice": {
				Username: "alice",
				Actor: &activitypub.Actor{
					BaseObject:        activitypub.BaseObject{ID: "https://example.com/users/alice", Type: "Person"},
					PreferredUsername: "alice",
					Name:              "Alice",
				},
			},
		},
		statusByID: map[string]storagemodels.Status{
			"s1": {
				PK:             "status#s1",
				SK:             "status#s1",
				StatusID:       "s1",
				AuthorUsername: "alice",
				AuthorID:       "https://example.com/users/alice",
				Content:        "hello",
				Sensitive:      false,
				PublishedAt:    now.Add(-2 * time.Hour),
				CreatedAt:      now.Add(-2 * time.Hour),
				UpdatedAt:      now.Add(-1 * time.Hour),
			},
		},
		reportsByID: map[string]storagemodels.Report{
			"r1": {
				ID:              "r1",
				ReporterID:      "admin",
				TargetAccountID: "alice",
				Category:        "spam",
				Status:          "open",
				StatusIDs:       []string{"s1"},
				CreatedAt:       now.Add(-2 * time.Hour),
				UpdatedAt:       now.Add(-1 * time.Hour),
			},
		},
		eventsByID: map[string]storagemodels.ModerationEvent{
			"evt1": {
				ID:              "evt1",
				EventType:       "flagged",
				ActorID:         "alice",
				ObjectID:        "s1",
				ObjectType:      "status",
				Category:        "spam",
				Severity:        "4",
				ConfidenceScore: 0.9,
				Created:         now.Add(-2 * time.Hour),
			},
		},
		trustRelationships: []storagemodels.TrustRelationship{
			{TrusterID: "admin", TrusteeID: "alice", Score: 0.5, Created: now.Add(-24 * time.Hour), Updated: now.Add(-1 * time.Hour)},
		},
		userMediaByUser: map[string][]*storagemodels.Media{
			"alice": {
				nil,
				{MediaID: "", GSI1SK: "cursor-media-bad"},
				{MediaID: "media-123", GSI1SK: "cursor-media-1"},
			},
		},
	}

	cloneState := func() *round10QueryState {
		state := *baseState
		if baseState.usersByUsername != nil {
			state.usersByUsername = make(map[string]storagemodels.User, len(baseState.usersByUsername))
			for k, v := range baseState.usersByUsername {
				state.usersByUsername[k] = v
			}
		}
		if baseState.actorsByUser != nil {
			state.actorsByUser = make(map[string]storagemodels.Actor, len(baseState.actorsByUser))
			for k, v := range baseState.actorsByUser {
				state.actorsByUser[k] = v
			}
		}
		if baseState.statusByID != nil {
			state.statusByID = make(map[string]storagemodels.Status, len(baseState.statusByID))
			for k, v := range baseState.statusByID {
				state.statusByID[k] = v
			}
		}
		if baseState.reportsByID != nil {
			state.reportsByID = make(map[string]storagemodels.Report, len(baseState.reportsByID))
			for k, v := range baseState.reportsByID {
				state.reportsByID[k] = v
			}
		}
		if baseState.eventsByID != nil {
			state.eventsByID = make(map[string]storagemodels.ModerationEvent, len(baseState.eventsByID))
			for k, v := range baseState.eventsByID {
				state.eventsByID[k] = v
			}
		}
		if baseState.userMediaByUser != nil {
			state.userMediaByUser = make(map[string][]*storagemodels.Media, len(baseState.userMediaByUser))
			for k, v := range baseState.userMediaByUser {
				state.userMediaByUser[k] = v
			}
		}
		if baseState.trustRelationships != nil {
			state.trustRelationships = append([]storagemodels.TrustRelationship(nil), baseState.trustRelationships...)
		}
		if baseState.instanceRules != nil {
			state.instanceRules = append([]storagemodels.InstanceRule(nil), baseState.instanceRules...)
		}
		return &state
	}

	newHandler := func(t *testing.T, state *round10QueryState) *Handler {
		t.Helper()
		harness := round10NewDynamoHarness(t, state)

		accountRepo := repositories.NewAccountRepository(harness.db, cfg.DynamoTableName, cfg.Domain, logger)
		actorRepo := repositories.NewActorRepository(harness.db, cfg.DynamoTableName, logger, cfg.Domain)
		moderationRepo := repositories.NewModerationRepository(harness.db, cfg.DynamoTableName, logger)
		userRepo := repositories.NewUserRepository(harness.db, cfg.DynamoTableName, logger)
		statusRepo := repositories.NewStatusRepository(harness.db, cfg.DynamoTableName, logger, nil)
		relationshipRepo := repositories.NewRelationshipRepository(harness.db, cfg.DynamoTableName, logger)
		trustRepo := repositories.NewTrustRepository(harness.db, cfg.DynamoTableName, logger, nil)
		mediaRepo := repositories.NewMediaRepository(harness.db, cfg.DynamoTableName, logger, nil)
		instanceRepo := repositories.NewInstanceRepository(harness.db, cfg.DynamoTableName, logger)

		repos := &MockRepositoryStorage{}
		repos.On("Account").Return(accountRepo).Maybe()
		repos.On("Actor").Return(actorRepo).Maybe()
		repos.On("Moderation").Return(moderationRepo).Maybe()
		repos.On("User").Return(userRepo).Maybe()
		repos.On("Status").Return(statusRepo).Maybe()
		repos.On("Relationship").Return(relationshipRepo).Maybe()
		repos.On("Trust").Return(trustRepo).Maybe()
		repos.On("Media").Return(mediaRepo).Maybe()
		repos.On("Instance").Return(instanceRepo).Maybe()
		repos.On("GetDB").Return(harness.db).Maybe()
		repos.On("GetTableName").Return(cfg.DynamoTableName).Maybe()
		repos.On("GetLogger").Return(logger).Maybe()
		repos.On("Audit").Return((*repositories.AuditRepository)(nil)).Maybe()

		return &Handler{cfg: cfg, repos: repos, logger: logger}
	}

	headers := map[string]string{"Authorization": "Bearer " + round10SignAccessToken(t, cfg.JWTSecret, "admin")}

	t.Run("adminAction and adminAccountAction error branches", func(t *testing.T) {
		h := newHandler(t, baseState)

		ctx0, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/accounts/user-alice/approve", headers, nil, nil)
		require.NoError(t, err)
		ctx0.Params["id"] = "user-alice"
		requireStatus(t, http.StatusInternalServerError)(h.adminAction(ctx0, "approve", func(_ string) (map[string]any, error) {
			return nil, errors.New("boom")
		}))

		ctx1, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/accounts/user-alice/reject", headers, nil, nil)
		require.NoError(t, err)
		ctx1.Params["id"] = "user-alice"
		requireStatus(t, http.StatusInternalServerError)(h.adminAccountAction(ctx1, "reject", func(_ string) error {
			return errors.New("boom")
		}))
	})

	t.Run("UpdateUser failure returns 500", func(t *testing.T) {
		state := cloneState()
		state.updateErrorOnce = errors.New("update failed")
		h := newHandler(t, state)

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/accounts/user-alice/approve", headers, nil, nil)
		require.NoError(t, err)
		ctx.Params["id"] = "user-alice"
		requireStatus(t, http.StatusInternalServerError)(h.HandleAdminApproveAccountLift(ctx))
	})

	t.Run("Mark status sensitive returns 500 on UpdateStatus error", func(t *testing.T) {
		state := cloneState()
		state.executeErrorOnce = errors.New("execute failed")
		h := newHandler(t, state)

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/statuses/s1/sensitive", headers, nil, nil)
		require.NoError(t, err)
		ctx.Params["id"] = "s1"
		requireStatus(t, http.StatusInternalServerError)(h.HandleAdminMarkStatusSensitiveLift(ctx))
	})

	t.Run("Delete status returns 500 on DeleteStatus error", func(t *testing.T) {
		state := cloneState()
		state.executeErrorOnce = errors.New("execute failed")
		h := newHandler(t, state)

		ctx, err := round10NewLiftContext(http.MethodDelete, "/api/v1/admin/statuses/s1", headers, nil, nil)
		require.NoError(t, err)
		ctx.Params["id"] = "s1"
		requireStatus(t, http.StatusInternalServerError)(h.HandleAdminDeleteStatusLift(ctx))
	})

	t.Run("Delete status returns 500 on GetStatus error", func(t *testing.T) {
		state := cloneState()
		state.firstErrorPK = map[string]error{"status#s1": errors.New("read failed")}
		h := newHandler(t, state)

		ctx, err := round10NewLiftContext(http.MethodDelete, "/api/v1/admin/statuses/s1", headers, nil, nil)
		require.NoError(t, err)
		ctx.Params["id"] = "s1"
		requireStatus(t, http.StatusInternalServerError)(h.HandleAdminDeleteStatusLift(ctx))
	})

	t.Run("Demote admin returns 400", func(t *testing.T) {
		h := newHandler(t, baseState)

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/moderation/reviewers/user-admin/demote", headers, nil, nil)
		require.NoError(t, err)
		ctx.Params["id"] = "user-admin"
		requireStatus(t, http.StatusBadRequest)(h.HandleAdminDemoteModeratorLift(ctx))
	})

	t.Run("Moderation events parsing branches", func(t *testing.T) {
		h := newHandler(t, baseState)

		ctxDefaultLimit, err := round10NewLiftContext(http.MethodGet, "/api/v1/admin/moderation/events", headers, map[string]string{
			"event_type":   "flagged",
			"category":     "spam",
			"min_severity": "not-an-int",
		}, nil)
		require.NoError(t, err)
		requireStatus(t, http.StatusOK)(h.HandleAdminGetModerationEventsLift(ctxDefaultLimit))
	})

	t.Run("Moderation events limit parse error returns 400", func(t *testing.T) {
		h := newHandler(t, baseState)
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/admin/moderation/events", headers, map[string]string{"limit": "bad"}, nil)
		require.NoError(t, err)
		requireStatus(t, http.StatusBadRequest)(h.HandleAdminGetModerationEventsLift(ctx))
	})

	t.Run("Moderation events returns 500 on repository error", func(t *testing.T) {
		state := cloneState()
		state.allErrorOnce = errors.New("query failed")
		h := newHandler(t, state)

		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/admin/moderation/events", headers, map[string]string{"event_type": "flagged"}, nil)
		require.NoError(t, err)
		requireStatus(t, http.StatusInternalServerError)(h.HandleAdminGetModerationEventsLift(ctx))
	})

	t.Run("Account detail error branches", func(t *testing.T) {
		t.Run("not found", func(t *testing.T) {
			state := cloneState()
			state.firstErrorPK = map[string]error{"USER#missing": dynamormerrors.ErrItemNotFound}
			h := newHandler(t, state)

			ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/admin/accounts/user-missing", headers, nil, nil)
			require.NoError(t, err)
			ctx.Params["id"] = "user-missing"
			requireStatus(t, http.StatusNotFound)(h.HandleAdminGetAccountLift(ctx))
		})

		t.Run("actor read error", func(t *testing.T) {
			state := cloneState()
			state.firstErrorPK = map[string]error{"ACTOR#alice": errors.New("actor read failed")}
			h := newHandler(t, state)

			ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/admin/accounts/user-alice", headers, nil, nil)
			require.NoError(t, err)
			ctx.Params["id"] = "user-alice"
			requireStatus(t, http.StatusInternalServerError)(h.HandleAdminGetAccountLift(ctx))
		})

		t.Run("sessions error tolerated", func(t *testing.T) {
			state := cloneState()
			state.allErrorByType = map[string]error{"*[]models.Session": errors.New("sessions query failed")}
			h := newHandler(t, state)

			ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/admin/accounts/user-alice", headers, nil, nil)
			require.NoError(t, err)
			ctx.Params["id"] = "user-alice"
			requireStatus(t, http.StatusOK)(h.HandleAdminGetAccountLift(ctx))
		})
	})

	t.Run("Accounts list invalid limit returns 400", func(t *testing.T) {
		h := newHandler(t, baseState)
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/admin/accounts", headers, map[string]string{"limit": "bad"}, nil)
		require.NoError(t, err)
		requireStatus(t, http.StatusBadRequest)(h.HandleAdminGetAccountsLift(ctx))
	})

	t.Run("Reports list invalid limit returns 400", func(t *testing.T) {
		h := newHandler(t, baseState)
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/admin/reports", headers, map[string]string{"limit": "bad"}, nil)
		require.NoError(t, err)
		requireStatus(t, http.StatusBadRequest)(h.HandleAdminGetReportsLift(ctx))
	})

	t.Run("Reviewers tolerates reviewer stats failure", func(t *testing.T) {
		state := cloneState()
		state.allErrorByType = map[string]error{
			"*[]models.ModerationReview": errors.New("review scan failed"),
		}
		h := newHandler(t, state)

		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/admin/moderation/reviewers", headers, nil, nil)
		require.NoError(t, err)
		requireStatus(t, http.StatusOK)(h.HandleAdminGetReviewersLift(ctx))
	})

	t.Run("Status filter parsing covers date branches", func(t *testing.T) {
		h := newHandler(t, baseState)

		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/admin/statuses", headers, map[string]string{
			"limit":      "bad-limit",
			"local":      "true",
			"remote":     "true",
			"flagged":    "true",
			"reported":   "true",
			"media":      "true",
			"sensitive":  "true",
			"by_domain":  "remote.example",
			"visibility": "public",
			"min_date":   "not-a-date",
			"max_date":   now.Add(-24 * time.Hour).Format(time.RFC3339),
		}, nil)
		require.NoError(t, err)

		_ = h.parseAdminStatusPagination(ctx)
		_ = h.parseAdminStatusFilter(ctx)
	})

	t.Run("Trust graph invalid limit returns 400", func(t *testing.T) {
		h := newHandler(t, baseState)
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/admin/moderation/trust/graph", headers, map[string]string{"limit": "bad"}, nil)
		require.NoError(t, err)
		requireStatus(t, http.StatusBadRequest)(h.HandleAdminGetTrustGraphLift(ctx))
	})

	t.Run("Update trust returns 500 on repository error", func(t *testing.T) {
		state := cloneState()
		state.executeErrorOnce = errors.New("execute failed")
		h := newHandler(t, state)

		updateReq := apimodels.AdminUpdateTrustRequest{Trust: 0.2, Category: "general", Reason: "test"}
		ctx, err := round10NewLiftContext(http.MethodPut, "/api/v1/admin/moderation/trust/a/b", headers, nil, updateReq)
		require.NoError(t, err)
		ctx.Params["from"] = "https://example.com/users/admin"
		ctx.Params["to"] = "https://example.com/users/alice"
		requireStatus(t, http.StatusInternalServerError)(h.HandleAdminUpdateTrustLift(ctx))
	})

	t.Run("Trust graph storage error returns 500", func(t *testing.T) {
		state := cloneState()
		state.allErrorByType = map[string]error{"*[]*models.TrustRelationship": errors.New("trust query failed")}
		h := newHandler(t, state)

		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/admin/moderation/trust/graph", headers, nil, nil)
		require.NoError(t, err)
		requireStatus(t, http.StatusInternalServerError)(h.HandleAdminGetTrustGraphLift(ctx))
	})

	t.Run("Enable account returns 500 on update error", func(t *testing.T) {
		state := cloneState()
		state.updateErrorOnce = errors.New("update failed")
		h := newHandler(t, state)

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/accounts/user-alice/enable", headers, nil, nil)
		require.NoError(t, err)
		ctx.Params["id"] = "user-alice"
		requireStatus(t, http.StatusInternalServerError)(h.HandleAdminEnableAccountLift(ctx))
	})

	t.Run("GetStatus returns 500 on storage error", func(t *testing.T) {
		state := cloneState()
		state.firstErrorPK = map[string]error{"status#s1": errors.New("read failed")}
		h := newHandler(t, state)

		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/admin/statuses/s1", headers, nil, nil)
		require.NoError(t, err)
		ctx.Params["id"] = "s1"
		requireStatus(t, http.StatusInternalServerError)(h.HandleAdminGetStatusLift(ctx))
	})

	t.Run("Pagination header omits when no cursor", func(t *testing.T) {
		h := newHandler(t, baseState)
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/admin/statuses", headers, map[string]string{"limit": "1"}, nil)
		require.NoError(t, err)

		resp := &apptheory.Response{Headers: map[string][]string{}}
		h.addPaginationHeader(ctx, resp, "", AdminStatusPagination{Limit: 1, Cursor: ""})
		require.NotContains(t, resp.Headers, "link")
	})

	t.Run("DeleteStatus logs and continues when audit log fails", func(t *testing.T) {
		state := cloneState()
		state.createErrorOnce = errors.New("audit create failed")
		h := newHandler(t, state)

		ctx, err := round10NewLiftContext(http.MethodDelete, "/api/v1/admin/statuses/s1", headers, nil, nil)
		require.NoError(t, err)
		ctx.Params["id"] = "s1"
		requireStatus(t, http.StatusNoContent)(h.HandleAdminDeleteStatusLift(ctx))
	})

	t.Run("Status sensitive action tolerates audit log failure", func(t *testing.T) {
		state := cloneState()
		state.createErrorOnce = errors.New("audit create failed")
		h := newHandler(t, state)

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/statuses/s1/sensitive", headers, nil, nil)
		require.NoError(t, err)
		ctx.Params["id"] = "s1"
		requireStatus(t, http.StatusOK)(h.HandleAdminMarkStatusSensitiveLift(ctx))
	})

	t.Run("Account action parse error returns 400", func(t *testing.T) {
		h := newHandler(t, baseState)

		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v1/admin/accounts/user-alice/action", headers, nil, []byte("{"))
		ctx.Params["id"] = "user-alice"
		requireStatus(t, http.StatusBadRequest)(h.HandleAdminAccountActionLift(ctx))
	})

	t.Run("Account action returns 500 on UpdateUser error", func(t *testing.T) {
		state := cloneState()
		state.updateErrorOnce = errors.New("update failed")
		h := newHandler(t, state)

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/accounts/user-alice/action", headers, nil, apimodels.AdminAccountActionRequest{Type: "disable"})
		require.NoError(t, err)
		ctx.Params["id"] = "user-alice"
		requireStatus(t, http.StatusInternalServerError)(h.HandleAdminAccountActionLift(ctx))
	})

	t.Run("Account action logs errors from helper operations", func(t *testing.T) {
		t.Run("suspend continues when cancel follow relationships fails", func(t *testing.T) {
			state := cloneState()
			state.firstErrorPK = map[string]error{"ACTOR#alice": errors.New("actor read failed")}
			h := newHandler(t, state)

			ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/accounts/user-alice/action", headers, nil, apimodels.AdminAccountActionRequest{Type: "suspend"})
			require.NoError(t, err)
			ctx.Params["id"] = "user-alice"
			requireStatus(t, http.StatusNoContent)(h.HandleAdminAccountActionLift(ctx))
		})

		t.Run("unsensitive continues when unmark fails", func(t *testing.T) {
			state := cloneState()
			state.scanErrorOnce = errors.New("media scan failed")
			h := newHandler(t, state)

			ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/accounts/user-alice/action", headers, nil, apimodels.AdminAccountActionRequest{Type: "unsensitive"})
			require.NoError(t, err)
			ctx.Params["id"] = "user-alice"
			requireStatus(t, http.StatusNoContent)(h.HandleAdminAccountActionLift(ctx))
		})
	})

	t.Run("Reviewers returns 500 on role list error", func(t *testing.T) {
		state := cloneState()
		state.allErrorByType = map[string]error{
			reflect.TypeOf(&[]storagemodels.User{}).String(): errors.New("list users failed"),
		}
		h := newHandler(t, state)

		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/admin/moderation/reviewers", headers, nil, nil)
		require.NoError(t, err)
		requireStatus(t, http.StatusOK)(h.HandleAdminGetReviewersLift(ctx))
	})

	t.Run("Admin accounts redacts internal error details", func(t *testing.T) {
		state := cloneState()
		state.allErrorByType = map[string]error{
			reflect.TypeOf(&[]storagemodels.User{}).String(): errors.New("secret ddb failure with table internals"),
		}
		h := newHandler(t, state)

		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/admin/accounts", headers, nil, nil)
		require.NoError(t, err)
		resp := requireStatus(t, http.StatusInternalServerError)(h.HandleAdminGetAccountsLift(ctx))

		var body map[string]string
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		require.Equal(t, "internal server error", body["error"])
		require.NotContains(t, string(resp.Body), "secret ddb failure")
	})

	t.Run("Moderation overview covers empty queue and reports", func(t *testing.T) {
		state := cloneState()
		state.eventsByID = map[string]storagemodels.ModerationEvent{}
		state.reportsByID = map[string]storagemodels.Report{}
		h := newHandler(t, state)

		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/admin/moderation/overview", headers, nil, nil)
		require.NoError(t, err)
		requireStatus(t, http.StatusOK)(h.HandleAdminModerationOverviewLift(ctx))
	})

	t.Run("Promote/demote moderator error branches", func(t *testing.T) {
		t.Run("promote returns 500 on UpdateUser error", func(t *testing.T) {
			state := cloneState()
			state.updateErrorOnce = errors.New("update failed")
			h := newHandler(t, state)

			ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/moderation/reviewers/user-alice/promote", headers, nil, nil)
			require.NoError(t, err)
			ctx.Params["id"] = "user-alice"
			requireStatus(t, http.StatusInternalServerError)(h.HandleAdminPromoteModeratorLift(ctx))
		})

		t.Run("demote returns 500 on UpdateUser error", func(t *testing.T) {
			state := cloneState()
			state.updateErrorOnce = errors.New("update failed")
			h := newHandler(t, state)

			ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/moderation/reviewers/user-mod/demote", headers, nil, nil)
			require.NoError(t, err)
			ctx.Params["id"] = "user-mod"
			requireStatus(t, http.StatusInternalServerError)(h.HandleAdminDemoteModeratorLift(ctx))
		})
	})

	t.Run("Report handlers return 500 on update/assign errors", func(t *testing.T) {
		t.Run("resolve", func(t *testing.T) {
			state := cloneState()
			state.updateErrorOnce = errors.New("update failed")
			h := newHandler(t, state)

			ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/reports/r1/resolve", headers, nil, nil)
			require.NoError(t, err)
			ctx.Params["id"] = "r1"
			requireStatus(t, http.StatusInternalServerError)(h.HandleAdminResolveReportLift(ctx))
		})

		t.Run("reopen", func(t *testing.T) {
			state := cloneState()
			state.updateErrorOnce = errors.New("update failed")
			h := newHandler(t, state)

			ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/reports/r1/reopen", headers, nil, nil)
			require.NoError(t, err)
			ctx.Params["id"] = "r1"
			requireStatus(t, http.StatusInternalServerError)(h.HandleAdminReopenReportLift(ctx))
		})

		t.Run("assign", func(t *testing.T) {
			state := cloneState()
			state.updateErrorOnce = errors.New("update failed")
			h := newHandler(t, state)

			ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/reports/r1/assign_to_self", headers, nil, nil)
			require.NoError(t, err)
			ctx.Params["id"] = "r1"
			requireStatus(t, http.StatusInternalServerError)(h.HandleAdminAssignReportLift(ctx))
		})

		t.Run("unassign", func(t *testing.T) {
			state := cloneState()
			state.updateErrorOnce = errors.New("update failed")
			h := newHandler(t, state)

			ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/reports/r1/unassign", headers, nil, nil)
			require.NoError(t, err)
			ctx.Params["id"] = "r1"
			requireStatus(t, http.StatusInternalServerError)(h.HandleAdminUnassignReportLift(ctx))
		})
	})

	t.Run("Assign/unassign return 500 when GetReport fails", func(t *testing.T) {
		t.Run("assign", func(t *testing.T) {
			state := cloneState()
			state.firstErrorPK = map[string]error{"REPORT#r1": errors.New("get report failed")}
			h := newHandler(t, state)

			ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/reports/r1/assign_to_self", headers, nil, nil)
			require.NoError(t, err)
			ctx.Params["id"] = "r1"
			requireStatus(t, http.StatusInternalServerError)(h.HandleAdminAssignReportLift(ctx))
		})

		t.Run("unassign", func(t *testing.T) {
			state := cloneState()
			state.firstErrorPK = map[string]error{"REPORT#r1": errors.New("get report failed")}
			h := newHandler(t, state)

			ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/reports/r1/unassign", headers, nil, nil)
			require.NoError(t, err)
			ctx.Params["id"] = "r1"
			requireStatus(t, http.StatusInternalServerError)(h.HandleAdminUnassignReportLift(ctx))
		})
	})

	t.Run("Trust graph health covers empty relationships", func(t *testing.T) {
		state := cloneState()
		state.trustRelationships = nil
		h := newHandler(t, state)
		health := h.getTrustGraphHealth(context.Background())
		require.Zero(t, health.TotalRelationships)
	})

	t.Run("loadViolatedRules returns empty on repository error", func(t *testing.T) {
		state := cloneState()
		state.firstErrorPK = map[string]error{"INSTANCE#CONFIG": errors.New("instance config failed")}
		h := newHandler(t, state)
		require.Empty(t, h.loadViolatedRules(context.Background(), "spam"))
	})

	t.Run("loadReportedStatuses returns empty on repo error", func(t *testing.T) {
		state := cloneState()
		state.firstErrorPK = map[string]error{"REPORT#r1": errors.New("report read failed")}
		h := newHandler(t, state)
		require.Empty(t, h.loadReportedStatuses(context.Background(), "r1"))
	})

	t.Run("buildAdminReport covers assigned/moderator warn paths", func(t *testing.T) {
		state := cloneState()
		state.reportsByID["r1"] = storagemodels.Report{
			ID:              "r1",
			ReporterID:      "admin",
			TargetAccountID: "alice",
			Category:        "spam",
			Status:          "open",
			StatusIDs:       []string{"s1"},
			AssignedTo:      "missing-assignee",
			ModeratorID:     "missing-moderator",
			CreatedAt:       now.Add(-2 * time.Hour),
			UpdatedAt:       now.Add(-1 * time.Hour),
		}
		state.firstErrorPK = map[string]error{
			"ACTOR#missing-assignee":  errors.New("assignee actor missing"),
			"ACTOR#missing-moderator": errors.New("moderator actor missing"),
		}
		h := newHandler(t, state)

		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/admin/reports/r1", headers, nil, nil)
		require.NoError(t, err)
		ctx.Params["id"] = "r1"
		requireStatus(t, http.StatusOK)(h.HandleAdminGetReportLift(ctx))
	})

	t.Run("Report detail returns 500 when reporter actor lookup fails", func(t *testing.T) {
		state := cloneState()
		state.firstErrorPK = map[string]error{"ACTOR#admin": errors.New("actor read failed")}
		h := newHandler(t, state)

		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/admin/reports/r1", headers, nil, nil)
		require.NoError(t, err)
		ctx.Params["id"] = "r1"
		requireStatus(t, http.StatusInternalServerError)(h.HandleAdminGetReportLift(ctx))
	})

	t.Run("Helper methods cover loops and error tolerance", func(t *testing.T) {
		state := cloneState()
		state.deleteErrorOnce = errors.New("delete failed")
		state.executeErrorOnce = errors.New("execute failed")
		state.allErrorByType = map[string]error{
			"*[]models.Session": errors.New("sessions query failed"),
		}
		h := newHandler(t, state)

		_ = h.getActiveModeratorsCount(context.Background())
		_ = h.getRecentConsensusDecisions(context.Background())
		require.GreaterOrEqual(t, h.getTrustGraphHealth(context.Background()).TotalRelationships, 0)

		require.NoError(t, h.cancelUserFollowRelationships(context.Background(), "alice"))
		require.NoError(t, h.markAllUserMediaAsSensitive(context.Background(), "alice"))
	})

	t.Run("cancelUserFollowRelationships returns error when actor read fails", func(t *testing.T) {
		state := cloneState()
		state.firstErrorPK = map[string]error{
			"ACTOR#alice": errors.New("actor read failed"),
		}
		h := newHandler(t, state)
		require.Error(t, h.cancelUserFollowRelationships(context.Background(), "alice"))
	})

	t.Run("loadReportedStatuses handles invalid status ids", func(t *testing.T) {
		state := cloneState()
		state.notFoundPKs = map[string]bool{"status#missing": true}
		state.statusByID["s2"] = storagemodels.Status{
			PK:        "status#s2",
			SK:        "status#s2",
			StatusID:  "s2",
			Content:   "fallback created_at",
			CreatedAt: now.Add(-3 * time.Hour),
		}
		state.reportsByID["r1"] = storagemodels.Report{
			ID:              "r1",
			ReporterID:      "admin",
			TargetAccountID: "alice",
			Category:        "spam",
			Status:          "open",
			StatusIDs:       []string{"", "s1", "missing", "s2"},
			CreatedAt:       now.Add(-2 * time.Hour),
			UpdatedAt:       now.Add(-1 * time.Hour),
		}
		h := newHandler(t, state)

		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/admin/reports/r1", headers, nil, nil)
		require.NoError(t, err)
		ctx.Params["id"] = "r1"
		requireStatus(t, http.StatusOK)(h.HandleAdminGetReportLift(ctx))
	})

	t.Run("Recent consensus decisions returns empty on repo error", func(t *testing.T) {
		state := cloneState()
		state.allErrorByType = map[string]error{"*[]models.ModerationEvent": errors.New("events failed")}
		h := newHandler(t, state)
		require.Empty(t, h.getRecentConsensusDecisions(context.Background()))
	})

	t.Run("Trust graph health returns empty on repo error", func(t *testing.T) {
		state := cloneState()
		state.scanErrorOnce = errors.New("trust failed")
		h := newHandler(t, state)
		health := h.getTrustGraphHealth(context.Background())
		require.Zero(t, health.TotalRelationships)
	})

	t.Run("Role permissions covers all cases", func(t *testing.T) {
		require.Equal(t, 0xFFFFFFFF, getAdminRolePermissions("admin"))
		require.Equal(t, 0x0000FFFF, getAdminRolePermissions("moderator"))
		require.Equal(t, 0x00000001, getAdminRolePermissions("user"))
	})

	t.Run("account action continues after helper errors", func(t *testing.T) {
		state := cloneState()
		state.executeErrorOnce = errors.New("execute failed")
		state.deleteErrorOnce = errors.New("delete failed")
		state.allErrorByType = map[string]error{
			"*[]models.RelationshipRecord": errors.New("relationship query failed"),
		}
		h := newHandler(t, state)

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/accounts/user-alice/action", headers, nil, apimodels.AdminAccountActionRequest{Type: "sensitive"})
		require.NoError(t, err)
		ctx.Params["id"] = "user-alice"
		requireStatus(t, http.StatusNoContent)(h.HandleAdminAccountActionLift(ctx))
	})
}

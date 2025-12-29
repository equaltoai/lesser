package lift

import (
	"net/http"
	"testing"
	"time"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/storage"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/golang-jwt/jwt/v5"
	liftframework "github.com/pay-theory/lift/pkg/lift"
	"github.com/stretchr/testify/require"
)

func TestAdminLift_Round10Coverage(t *testing.T) {
	cfg := round10TestConfig()
	logger := round10TestLogger(t)

	now := time.Now()
	state := &round10QueryState{
		usersByUsername: map[string]storagemodels.User{
			"admin": {PK: "USER#admin", SK: storagemodels.SKMetadata, Username: "admin", Role: "admin", Approved: true, Version: 1, CreatedAt: now.Add(-48 * time.Hour), GSI1SK: "cursor-admin"},
			"alice": {PK: "USER#alice", SK: storagemodels.SKMetadata, Username: "alice", Role: "user", Approved: true, Version: 1, CreatedAt: now.Add(-24 * time.Hour), GSI1SK: "cursor-alice"},
			"mod":   {PK: "USER#mod", SK: storagemodels.SKMetadata, Username: "mod", Role: "moderator", Approved: true, Version: 1, CreatedAt: now.Add(-24 * time.Hour), GSI1SK: "cursor-mod"},
		},
		actorsByUser: map[string]storagemodels.Actor{
			"admin": {
				Username: "admin",
				Actor: &activitypub.Actor{
					BaseObject: activitypub.BaseObject{
						ID:   "https://example.com/users/admin",
						Type: "Person",
					},
					PreferredUsername: "admin",
					Name:              "Admin",
				},
				CreatedAt: now.Add(-48 * time.Hour),
				UpdatedAt: now.Add(-1 * time.Hour),
			},
			"alice": {
				Username: "alice",
				Actor: &activitypub.Actor{
					BaseObject: activitypub.BaseObject{
						ID:   "https://example.com/users/alice",
						Type: "Person",
					},
					PreferredUsername: "alice",
					Name:              "Alice",
				},
				CreatedAt: now.Add(-24 * time.Hour),
				UpdatedAt: now.Add(-1 * time.Hour),
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
			"s2": {
				PK:             "status#s2",
				SK:             "status#s2",
				StatusID:       "s2",
				AuthorUsername: "alice",
				AuthorID:       "https://example.com/users/alice",
				Content:        "hello 2",
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
				Comment:         "bad",
				Forwarded:       false,
				Status:          "open",
				StatusIDs:       []string{"s1"},
				CreatedAt:       now.Add(-2 * time.Hour),
				UpdatedAt:       now.Add(-1 * time.Hour),
				AssignedTo:      "admin",
				ModeratorID:     "admin",
				GSI3SK:          "cursor-report-1",
			},
			"r2": {
				ID:              "r2",
				ReporterID:      "admin",
				TargetAccountID: "alice",
				Category:        "spam",
				Comment:         "worse",
				Forwarded:       true,
				Status:          "open",
				StatusIDs:       []string{"s2"},
				CreatedAt:       now.Add(-2 * time.Hour),
				UpdatedAt:       now.Add(-1 * time.Hour),
				GSI3SK:          "cursor-report-2",
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
				GSI2SK:          "cursor-event-1",
				SK:              "EVENT#1",
				Type:            storagemodels.ModerationTypeEvent,
				Created:         now.Add(-2 * time.Hour),
				Updated:         now.Add(-1 * time.Hour),
			},
			"evt2": {
				ID:              "evt2",
				EventType:       "flagged",
				ActorID:         "alice",
				ObjectID:        "s2",
				ObjectType:      "status",
				Category:        "spam",
				Severity:        "3",
				ConfidenceScore: 0.8,
				GSI2SK:          "cursor-event-2",
				SK:              "EVENT#2",
				Type:            storagemodels.ModerationTypeEvent,
				Created:         now.Add(-2 * time.Hour),
				Updated:         now.Add(-1 * time.Hour),
			},
			"evt3": {
				ID:              "evt3",
				EventType:       "flagged",
				ActorID:         "alice",
				ObjectID:        "s2",
				ObjectType:      "status",
				Category:        "spam",
				Severity:        "2",
				ConfidenceScore: 0.7,
				GSI2SK:          "cursor-event-3",
				SK:              "EVENT#3",
				Type:            storagemodels.ModerationTypeEvent,
				Created:         now.Add(-2 * time.Hour),
				Updated:         now.Add(-1 * time.Hour),
			},
		},
		trustRelationships: []storagemodels.TrustRelationship{
			{
				TrusterID:  "https://example.com/users/admin",
				TrusteeID:  "https://example.com/users/alice",
				Category:   storagemodels.TrustCategoryGeneral,
				Score:      0.5,
				Confidence: 1.0,
				Created:    now.Add(-24 * time.Hour),
				Updated:    now.Add(-1 * time.Hour),
			},
			{
				TrusterID:  "https://example.com/users/alice",
				TrusteeID:  "https://example.com/users/admin",
				Category:   storagemodels.TrustCategoryGeneral,
				Score:      0.2,
				Confidence: 1.0,
				Created:    now.Add(-24 * time.Hour),
				Updated:    now.Add(-1 * time.Hour),
			},
		},
		instanceRules: []storagemodels.InstanceRule{
			{ID: "1", Text: "Be nice", Category: "spam", Active: true, Order: 1},
		},
		notFoundPKs: map[string]bool{
			"status#missing":   true,
			"REPORT#missing":   true,
			"USER#missing":     true,
			"REPORT#not-there": true,
		},
		notFoundGSI3PK: map[string]bool{
			"EVENTID#missing": true,
		},
	}

	harness := round10NewDynamoHarness(t, state)

	accountRepo := repositories.NewAccountRepository(harness.db, cfg.DynamoTableName, cfg.Domain, logger)
	actorRepo := repositories.NewActorRepository(harness.db, cfg.DynamoTableName, logger)
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

	h := &Handler{cfg: cfg, repos: repos, logger: logger}

	token := round10SignAccessToken(t, cfg.JWTSecret, "admin")
	headers := map[string]string{"Authorization": "Bearer " + token}

	t.Run("helper functions", func(t *testing.T) {
		lastIP, ips := processUserSessions([]*storage.Session{
			{IPAddress: "203.0.113.10", LastActivity: now.Add(-1 * time.Hour)},
			{IPAddress: "203.0.113.10", LastActivity: now.Add(-2 * time.Hour)},
			{IPAddress: "203.0.113.11", LastActivity: now.Add(-3 * time.Hour)},
		})
		require.NotNil(t, lastIP)
		require.NotEmpty(t, ips)

		lastIP, ips = processUserSessions(nil)
		require.Nil(t, lastIP)
		require.Nil(t, ips)

		require.Equal(t, "3", getAdminRoleID("admin"))
		require.Equal(t, "2", getAdminRoleID("moderator"))
		require.Equal(t, "1", getAdminRoleID("user"))
		require.NotZero(t, getAdminRolePermissions("admin"))
	})

	t.Run("accounts list + detail", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/admin/accounts", headers, map[string]string{"limit": "1"}, nil)
		require.NoError(t, err)
		require.NoError(t, h.HandleAdminGetAccountsLift(ctx))
		require.Equal(t, http.StatusOK, ctx.Response.StatusCode)

		ctx2, err := round10NewLiftContext(http.MethodGet, "/api/v1/admin/accounts/user-alice", headers, nil, nil)
		require.NoError(t, err)
		ctx2.SetParam("id", "user-alice")
		require.NoError(t, h.HandleAdminGetAccountLift(ctx2))
		require.Equal(t, http.StatusOK, ctx2.Response.StatusCode)
	})

	t.Run("account actions", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/accounts/user-alice/action", headers, nil, apimodels.AdminAccountActionRequest{Type: "sensitive"})
		require.NoError(t, err)
		ctx.SetParam("id", "user-alice")
		require.NoError(t, h.HandleAdminAccountActionLift(ctx))
		require.Equal(t, http.StatusNoContent, ctx.Response.StatusCode)

		// Exercise additional action switch branches (and cancelUserFollowRelationships)
		for _, actionType := range []string{"suspend", "unsuspend", "silence", "unsilence", "disable", "enable", "approve", "unsensitive"} {
			actCtx, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/accounts/user-alice/action", headers, nil, apimodels.AdminAccountActionRequest{Type: actionType})
			require.NoError(t, err)
			actCtx.SetParam("id", "user-alice")
			require.NoError(t, h.HandleAdminAccountActionLift(actCtx))
			require.Equal(t, http.StatusNoContent, actCtx.Response.StatusCode)
		}

		badCtx, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/accounts/user-alice/action", headers, nil, apimodels.AdminAccountActionRequest{Type: "not-a-real-action"})
		require.NoError(t, err)
		badCtx.SetParam("id", "user-alice")
		require.NoError(t, h.HandleAdminAccountActionLift(badCtx))
		require.Equal(t, http.StatusBadRequest, badCtx.Response.StatusCode)

		ctx2, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/accounts/user-alice/approve", headers, nil, nil)
		require.NoError(t, err)
		ctx2.SetParam("id", "user-alice")
		require.NoError(t, h.HandleAdminApproveAccountLift(ctx2))
		require.Equal(t, http.StatusNoContent, ctx2.Response.StatusCode)

		ctx3, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/accounts/user-alice/reject", headers, nil, nil)
		require.NoError(t, err)
		ctx3.SetParam("id", "user-alice")
		require.NoError(t, h.HandleAdminRejectAccountLift(ctx3))
		require.Equal(t, http.StatusNoContent, ctx3.Response.StatusCode)

		ctx4, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/accounts/user-alice/enable", headers, nil, nil)
		require.NoError(t, err)
		ctx4.SetParam("id", "user-alice")
		require.NoError(t, h.HandleAdminEnableAccountLift(ctx4))
		require.Equal(t, http.StatusNoContent, ctx4.Response.StatusCode)

		ctx5, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/accounts/user-alice/unsuspend", headers, nil, nil)
		require.NoError(t, err)
		ctx5.SetParam("id", "user-alice")
		require.NoError(t, h.HandleAdminUnsuspendAccountLift(ctx5))
		require.Equal(t, http.StatusNoContent, ctx5.Response.StatusCode)

		ctx6, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/accounts/user-alice/unsilence", headers, nil, nil)
		require.NoError(t, err)
		ctx6.SetParam("id", "user-alice")
		require.NoError(t, h.HandleAdminUnsilenceAccountLift(ctx6))
		require.Equal(t, http.StatusNoContent, ctx6.Response.StatusCode)

		ctx7, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/accounts/user-alice/unsensitive", headers, nil, nil)
		require.NoError(t, err)
		ctx7.SetParam("id", "user-alice")
		require.NoError(t, h.HandleAdminUnsensitiveAccountLift(ctx7))
		require.Equal(t, http.StatusNoContent, ctx7.Response.StatusCode)
	})

	t.Run("reports and moderation", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/admin/reports", headers, map[string]string{"limit": "1", "status": "resolved"}, nil)
		require.NoError(t, err)
		require.NoError(t, h.HandleAdminGetReportsLift(ctx))
		require.Equal(t, http.StatusOK, ctx.Response.StatusCode)
		require.Contains(t, ctx.Response.Headers, "Link")

		ctx2, err := round10NewLiftContext(http.MethodGet, "/api/v1/admin/reports/r1", headers, nil, nil)
		require.NoError(t, err)
		ctx2.SetParam("id", "r1")
		require.NoError(t, h.HandleAdminGetReportLift(ctx2))
		require.Equal(t, http.StatusOK, ctx2.Response.StatusCode)

		ctxNotFound, err := round10NewLiftContext(http.MethodGet, "/api/v1/admin/reports/missing", headers, nil, nil)
		require.NoError(t, err)
		ctxNotFound.SetParam("id", "missing")
		require.NoError(t, h.HandleAdminGetReportLift(ctxNotFound))
		require.Equal(t, http.StatusNotFound, ctxNotFound.Response.StatusCode)

		ctx3, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/reports/r1/resolve", headers, nil, nil)
		require.NoError(t, err)
		ctx3.SetParam("id", "r1")
		require.NoError(t, h.HandleAdminResolveReportLift(ctx3))
		require.Equal(t, http.StatusOK, ctx3.Response.StatusCode)

		ctx3b, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/reports/not-there/resolve", headers, nil, nil)
		require.NoError(t, err)
		ctx3b.SetParam("id", "not-there")
		require.NoError(t, h.HandleAdminResolveReportLift(ctx3b))
		require.Equal(t, http.StatusNotFound, ctx3b.Response.StatusCode)

		ctx4, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/reports/r1/reopen", headers, nil, nil)
		require.NoError(t, err)
		ctx4.SetParam("id", "r1")
		require.NoError(t, h.HandleAdminReopenReportLift(ctx4))
		require.Equal(t, http.StatusOK, ctx4.Response.StatusCode)

		ctx4b, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/reports/not-there/reopen", headers, nil, nil)
		require.NoError(t, err)
		ctx4b.SetParam("id", "not-there")
		require.NoError(t, h.HandleAdminReopenReportLift(ctx4b))
		require.Equal(t, http.StatusNotFound, ctx4b.Response.StatusCode)

		ctx5, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/reports/r1/assign_to_self", headers, nil, nil)
		require.NoError(t, err)
		ctx5.SetParam("id", "r1")
		require.NoError(t, h.HandleAdminAssignReportLift(ctx5))
		require.Equal(t, http.StatusOK, ctx5.Response.StatusCode)

		ctx5b, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/reports/not-there/assign_to_self", headers, nil, nil)
		require.NoError(t, err)
		ctx5b.SetParam("id", "not-there")
		require.NoError(t, h.HandleAdminAssignReportLift(ctx5b))
		require.Equal(t, http.StatusNotFound, ctx5b.Response.StatusCode)

		ctx6, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/reports/r1/unassign", headers, nil, nil)
		require.NoError(t, err)
		ctx6.SetParam("id", "r1")
		require.NoError(t, h.HandleAdminUnassignReportLift(ctx6))
		require.Equal(t, http.StatusOK, ctx6.Response.StatusCode)

		ctx6b, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/reports/not-there/unassign", headers, nil, nil)
		require.NoError(t, err)
		ctx6b.SetParam("id", "not-there")
		require.NoError(t, h.HandleAdminUnassignReportLift(ctx6b))
		require.Equal(t, http.StatusNotFound, ctx6b.Response.StatusCode)

		ctx7, err := round10NewLiftContext(http.MethodGet, "/api/v1/admin/moderation/overview", headers, nil, nil)
		require.NoError(t, err)
		require.NoError(t, h.HandleAdminModerationOverviewLift(ctx7))
		require.Equal(t, http.StatusOK, ctx7.Response.StatusCode)
	})

	t.Run("moderation events override", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/admin/moderation/events", headers, map[string]string{"limit": "1", "min_severity": "2"}, nil)
		require.NoError(t, err)
		require.NoError(t, h.HandleAdminGetModerationEventsLift(ctx))
		require.Equal(t, http.StatusOK, ctx.Response.StatusCode)
		require.Contains(t, ctx.Response.Headers, "Link")

		overrideReq := apimodels.AdminModerationEventOverrideRequest{Decision: "reject", Reason: "bad"}
		ctx2, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/moderation/events/evt1/override", headers, nil, overrideReq)
		require.NoError(t, err)
		ctx2.SetParam("id", "evt1")
		require.NoError(t, h.HandleAdminOverrideModerationEventLift(ctx2))
		require.Equal(t, http.StatusOK, ctx2.Response.StatusCode)

		ctx3, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/moderation/events/evt1/override", headers, nil, apimodels.AdminModerationEventOverrideRequest{Decision: "approve"})
		require.NoError(t, err)
		ctx3.SetParam("id", "evt1")
		require.NoError(t, h.HandleAdminOverrideModerationEventLift(ctx3))
		require.Equal(t, http.StatusOK, ctx3.Response.StatusCode)

		ctx4, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/moderation/events/evt1/override", headers, nil, apimodels.AdminModerationEventOverrideRequest{Decision: "maybe"})
		require.NoError(t, err)
		ctx4.SetParam("id", "evt1")
		require.NoError(t, h.HandleAdminOverrideModerationEventLift(ctx4))
		require.Equal(t, http.StatusBadRequest, ctx4.Response.StatusCode)

		ctx5 := round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v1/admin/moderation/events/evt1/override", headers, nil, []byte("{"))
		ctx5.SetParam("id", "evt1")
		require.NoError(t, h.HandleAdminOverrideModerationEventLift(ctx5))
		require.Equal(t, http.StatusBadRequest, ctx5.Response.StatusCode)

		ctx6, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/moderation/events/missing/override", headers, nil, apimodels.AdminModerationEventOverrideRequest{Decision: "reject"})
		require.NoError(t, err)
		ctx6.SetParam("id", "missing")
		require.NoError(t, h.HandleAdminOverrideModerationEventLift(ctx6))
		require.Equal(t, http.StatusNotFound, ctx6.Response.StatusCode)
	})

	t.Run("trust and reviewers", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/admin/moderation/trust/graph", headers, map[string]string{"limit": "1"}, nil)
		require.NoError(t, err)
		require.NoError(t, h.HandleAdminGetTrustGraphLift(ctx))
		require.Equal(t, http.StatusOK, ctx.Response.StatusCode)

		ctxDefaultLimit, err := round10NewLiftContext(http.MethodGet, "/api/v1/admin/moderation/trust/graph", headers, nil, nil)
		require.NoError(t, err)
		require.NoError(t, h.HandleAdminGetTrustGraphLift(ctxDefaultLimit))
		require.Equal(t, http.StatusOK, ctxDefaultLimit.Response.StatusCode)

		updateReq := apimodels.AdminUpdateTrustRequest{Trust: 0.2, Category: "general", Reason: "test"}
		ctx2, err := round10NewLiftContext(http.MethodPut, "/api/v1/admin/moderation/trust/a/b", headers, nil, updateReq)
		require.NoError(t, err)
		ctx2.SetParam("from", "https://example.com/users/admin")
		ctx2.SetParam("to", "https://example.com/users/alice")
		require.NoError(t, h.HandleAdminUpdateTrustLift(ctx2))
		require.Equal(t, http.StatusOK, ctx2.Response.StatusCode)

		badTrustReq := apimodels.AdminUpdateTrustRequest{Trust: 2.0}
		ctx2b, err := round10NewLiftContext(http.MethodPut, "/api/v1/admin/moderation/trust/a/b", headers, nil, badTrustReq)
		require.NoError(t, err)
		ctx2b.SetParam("from", "https://example.com/users/admin")
		ctx2b.SetParam("to", "https://example.com/users/alice")
		require.NoError(t, h.HandleAdminUpdateTrustLift(ctx2b))
		require.Equal(t, http.StatusBadRequest, ctx2b.Response.StatusCode)

		ctx2c := round10NewLiftContextWithBodyBytes(http.MethodPut, "/api/v1/admin/moderation/trust/a/b", headers, nil, []byte("{"))
		ctx2c.SetParam("from", "https://example.com/users/admin")
		ctx2c.SetParam("to", "https://example.com/users/alice")
		require.NoError(t, h.HandleAdminUpdateTrustLift(ctx2c))
		require.Equal(t, http.StatusBadRequest, ctx2c.Response.StatusCode)

		ctx3, err := round10NewLiftContext(http.MethodGet, "/api/v1/admin/moderation/reviewers", headers, nil, nil)
		require.NoError(t, err)
		require.NoError(t, h.HandleAdminGetReviewersLift(ctx3))
		require.Equal(t, http.StatusOK, ctx3.Response.StatusCode)

		ctx4, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/moderation/reviewers/user-alice/promote", headers, nil, nil)
		require.NoError(t, err)
		ctx4.SetParam("id", "user-alice")
		require.NoError(t, h.HandleAdminPromoteModeratorLift(ctx4))
		require.Equal(t, http.StatusOK, ctx4.Response.StatusCode)

		ctx5, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/moderation/reviewers/user-mod/demote", headers, nil, nil)
		require.NoError(t, err)
		ctx5.SetParam("id", "user-mod")
		require.NoError(t, h.HandleAdminDemoteModeratorLift(ctx5))
		require.Equal(t, http.StatusOK, ctx5.Response.StatusCode)

		ctx6, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/moderation/reviewers/user-missing/demote", headers, nil, nil)
		require.NoError(t, err)
		ctx6.SetParam("id", "user-missing")
		require.NoError(t, h.HandleAdminDemoteModeratorLift(ctx6))
		require.Equal(t, http.StatusNotFound, ctx6.Response.StatusCode)
	})

	t.Run("status admin endpoints", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/admin/statuses", headers, map[string]string{
			"limit":         "1",
			"include_count": "true",
			"local":         "true",
			"min_date":      now.Add(-24 * time.Hour).Format(time.RFC3339),
		}, nil)
		require.NoError(t, err)
		require.NoError(t, h.HandleAdminGetStatusesLift(ctx))
		require.Equal(t, http.StatusOK, ctx.Response.StatusCode)
		require.Contains(t, ctx.Response.Headers, "X-Total-Count")
		require.Contains(t, ctx.Response.Headers, "Link")

		ctx2, err := round10NewLiftContext(http.MethodGet, "/api/v1/admin/statuses/s1", headers, nil, nil)
		require.NoError(t, err)
		ctx2.SetParam("id", "s1")
		require.NoError(t, h.HandleAdminGetStatusLift(ctx2))
		require.Equal(t, http.StatusOK, ctx2.Response.StatusCode)

		ctx2b, err := round10NewLiftContext(http.MethodGet, "/api/v1/admin/statuses/missing", headers, nil, nil)
		require.NoError(t, err)
		ctx2b.SetParam("id", "missing")
		require.NoError(t, h.HandleAdminGetStatusLift(ctx2b))
		require.Equal(t, http.StatusNotFound, ctx2b.Response.StatusCode)

		ctx3, err := round10NewLiftContext(http.MethodDelete, "/api/v1/admin/statuses/s1", headers, nil, nil)
		require.NoError(t, err)
		ctx3.SetParam("id", "s1")
		require.NoError(t, h.HandleAdminDeleteStatusLift(ctx3))
		require.Equal(t, http.StatusNoContent, ctx3.Response.StatusCode)

		ctx3b, err := round10NewLiftContext(http.MethodDelete, "/api/v1/admin/statuses/missing", headers, nil, nil)
		require.NoError(t, err)
		ctx3b.SetParam("id", "missing")
		require.NoError(t, h.HandleAdminDeleteStatusLift(ctx3b))
		require.Equal(t, http.StatusNotFound, ctx3b.Response.StatusCode)

		ctx4, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/statuses/s1/sensitive", headers, nil, nil)
		require.NoError(t, err)
		ctx4.SetParam("id", "s1")
		require.NoError(t, h.HandleAdminMarkStatusSensitiveLift(ctx4))
		require.Equal(t, http.StatusOK, ctx4.Response.StatusCode)

		ctx4b, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/statuses/missing/sensitive", headers, nil, nil)
		require.NoError(t, err)
		ctx4b.SetParam("id", "missing")
		require.NoError(t, h.HandleAdminMarkStatusSensitiveLift(ctx4b))
		require.Equal(t, http.StatusNotFound, ctx4b.Response.StatusCode)

		ctx5, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/statuses/s1/unsensitive", headers, nil, nil)
		require.NoError(t, err)
		ctx5.SetParam("id", "s1")
		require.NoError(t, h.HandleAdminUnmarkStatusSensitiveLift(ctx5))
		require.Equal(t, http.StatusOK, ctx5.Response.StatusCode)

		ctx5b, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/statuses/missing/unsensitive", headers, nil, nil)
		require.NoError(t, err)
		ctx5b.SetParam("id", "missing")
		require.NoError(t, h.HandleAdminUnmarkStatusSensitiveLift(ctx5b))
		require.Equal(t, http.StatusNotFound, ctx5b.Response.StatusCode)
	})

	t.Run("forbidden when missing auth", func(t *testing.T) {
		noAuthHeaders := map[string]string{}

		checkForbidden := func(ctx *liftframework.Context, err error) {
			t.Helper()
			require.NoError(t, err)
			require.Equal(t, http.StatusForbidden, ctx.Response.StatusCode)
		}

		ctx0, err := round10NewLiftContext(http.MethodGet, "/api/v1/admin/accounts", noAuthHeaders, nil, nil)
		require.NoError(t, h.HandleAdminGetAccountsLift(ctx0))
		checkForbidden(ctx0, err)

		ctx1, err := round10NewLiftContext(http.MethodGet, "/api/v1/admin/accounts/user-alice", noAuthHeaders, nil, nil)
		ctx1.SetParam("id", "user-alice")
		require.NoError(t, h.HandleAdminGetAccountLift(ctx1))
		checkForbidden(ctx1, err)

		ctx2, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/accounts/user-alice/action", noAuthHeaders, nil, nil)
		ctx2.SetParam("id", "user-alice")
		require.NoError(t, h.HandleAdminAccountActionLift(ctx2))
		checkForbidden(ctx2, err)

		ctx3, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/accounts/user-alice/approve", noAuthHeaders, nil, nil)
		ctx3.SetParam("id", "user-alice")
		require.NoError(t, h.HandleAdminApproveAccountLift(ctx3))
		checkForbidden(ctx3, err)

		ctx4, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/accounts/user-alice/reject", noAuthHeaders, nil, nil)
		ctx4.SetParam("id", "user-alice")
		require.NoError(t, h.HandleAdminRejectAccountLift(ctx4))
		checkForbidden(ctx4, err)

		ctx5, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/accounts/user-alice/enable", noAuthHeaders, nil, nil)
		ctx5.SetParam("id", "user-alice")
		require.NoError(t, h.HandleAdminEnableAccountLift(ctx5))
		checkForbidden(ctx5, err)

		ctx6, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/accounts/user-alice/unsilence", noAuthHeaders, nil, nil)
		ctx6.SetParam("id", "user-alice")
		require.NoError(t, h.HandleAdminUnsilenceAccountLift(ctx6))
		checkForbidden(ctx6, err)

		ctx7, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/accounts/user-alice/unsuspend", noAuthHeaders, nil, nil)
		ctx7.SetParam("id", "user-alice")
		require.NoError(t, h.HandleAdminUnsuspendAccountLift(ctx7))
		checkForbidden(ctx7, err)

		ctx8, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/accounts/user-alice/unsensitive", noAuthHeaders, nil, nil)
		ctx8.SetParam("id", "user-alice")
		require.NoError(t, h.HandleAdminUnsensitiveAccountLift(ctx8))
		checkForbidden(ctx8, err)

		ctx9, err := round10NewLiftContext(http.MethodGet, "/api/v1/admin/reports", noAuthHeaders, nil, nil)
		require.NoError(t, h.HandleAdminGetReportsLift(ctx9))
		checkForbidden(ctx9, err)

		ctx10, err := round10NewLiftContext(http.MethodGet, "/api/v1/admin/reports/r1", noAuthHeaders, nil, nil)
		ctx10.SetParam("id", "r1")
		require.NoError(t, h.HandleAdminGetReportLift(ctx10))
		checkForbidden(ctx10, err)

		ctx11, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/reports/r1/resolve", noAuthHeaders, nil, nil)
		ctx11.SetParam("id", "r1")
		require.NoError(t, h.HandleAdminResolveReportLift(ctx11))
		checkForbidden(ctx11, err)

		ctx12, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/reports/r1/reopen", noAuthHeaders, nil, nil)
		ctx12.SetParam("id", "r1")
		require.NoError(t, h.HandleAdminReopenReportLift(ctx12))
		checkForbidden(ctx12, err)

		ctx13, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/reports/r1/assign_to_self", noAuthHeaders, nil, nil)
		ctx13.SetParam("id", "r1")
		require.NoError(t, h.HandleAdminAssignReportLift(ctx13))
		checkForbidden(ctx13, err)

		ctx14, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/reports/r1/unassign", noAuthHeaders, nil, nil)
		ctx14.SetParam("id", "r1")
		require.NoError(t, h.HandleAdminUnassignReportLift(ctx14))
		checkForbidden(ctx14, err)

		ctx15, err := round10NewLiftContext(http.MethodGet, "/api/v1/admin/moderation/overview", noAuthHeaders, nil, nil)
		require.NoError(t, h.HandleAdminModerationOverviewLift(ctx15))
		checkForbidden(ctx15, err)

		ctx16, err := round10NewLiftContext(http.MethodGet, "/api/v1/admin/moderation/events", noAuthHeaders, nil, nil)
		require.NoError(t, h.HandleAdminGetModerationEventsLift(ctx16))
		checkForbidden(ctx16, err)

		ctx17, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/moderation/events/evt1/override", noAuthHeaders, nil, nil)
		ctx17.SetParam("id", "evt1")
		require.NoError(t, h.HandleAdminOverrideModerationEventLift(ctx17))
		checkForbidden(ctx17, err)

		ctx18, err := round10NewLiftContext(http.MethodGet, "/api/v1/admin/moderation/trust/graph", noAuthHeaders, nil, nil)
		require.NoError(t, h.HandleAdminGetTrustGraphLift(ctx18))
		checkForbidden(ctx18, err)

		ctx19, err := round10NewLiftContext(http.MethodPut, "/api/v1/admin/moderation/trust/a/b", noAuthHeaders, nil, nil)
		ctx19.SetParam("from", "a")
		ctx19.SetParam("to", "b")
		require.NoError(t, h.HandleAdminUpdateTrustLift(ctx19))
		checkForbidden(ctx19, err)

		ctx20, err := round10NewLiftContext(http.MethodGet, "/api/v1/admin/moderation/reviewers", noAuthHeaders, nil, nil)
		require.NoError(t, h.HandleAdminGetReviewersLift(ctx20))
		checkForbidden(ctx20, err)

		ctx21, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/moderation/reviewers/user-alice/promote", noAuthHeaders, nil, nil)
		ctx21.SetParam("id", "user-alice")
		require.NoError(t, h.HandleAdminPromoteModeratorLift(ctx21))
		checkForbidden(ctx21, err)

		ctx22, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/moderation/reviewers/user-alice/demote", noAuthHeaders, nil, nil)
		ctx22.SetParam("id", "user-alice")
		require.NoError(t, h.HandleAdminDemoteModeratorLift(ctx22))
		checkForbidden(ctx22, err)

		ctx23, err := round10NewLiftContext(http.MethodGet, "/api/v1/admin/statuses", noAuthHeaders, nil, nil)
		require.NoError(t, h.HandleAdminGetStatusesLift(ctx23))
		checkForbidden(ctx23, err)

		ctx24, err := round10NewLiftContext(http.MethodGet, "/api/v1/admin/statuses/s1", noAuthHeaders, nil, nil)
		ctx24.SetParam("id", "s1")
		require.NoError(t, h.HandleAdminGetStatusLift(ctx24))
		checkForbidden(ctx24, err)

		ctx25, err := round10NewLiftContext(http.MethodDelete, "/api/v1/admin/statuses/s1", noAuthHeaders, nil, nil)
		ctx25.SetParam("id", "s1")
		require.NoError(t, h.HandleAdminDeleteStatusLift(ctx25))
		checkForbidden(ctx25, err)

		ctx26, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/statuses/s1/sensitive", noAuthHeaders, nil, nil)
		ctx26.SetParam("id", "s1")
		require.NoError(t, h.HandleAdminMarkStatusSensitiveLift(ctx26))
		checkForbidden(ctx26, err)

		ctx27, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/statuses/s1/unsensitive", noAuthHeaders, nil, nil)
		ctx27.SetParam("id", "s1")
		require.NoError(t, h.HandleAdminUnmarkStatusSensitiveLift(ctx27))
		checkForbidden(ctx27, err)
	})
}

func round10SignAccessToken(t *testing.T, secret, username string) string {
	t.Helper()

	now := time.Now()
	claims := auth.Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   username,
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(1 * time.Hour)),
		},
		Username: username,
		ClientID: "test-client",
		Scopes:   []string{"read", "write"},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(secret))
	require.NoError(t, err)
	return signed
}

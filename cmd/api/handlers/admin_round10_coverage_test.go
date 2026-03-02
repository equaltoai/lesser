package handlers

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
				GSI8PK:         "ADMIN_TIMELINE",
				GSI8SK:         "cursor-status-1",
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
				GSI8PK:         "ADMIN_TIMELINE",
				GSI8SK:         "cursor-status-2",
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
				Severity:        "critical",
				ConfidenceScore: 0.9,
				GSI2SK:          "cursor-event-1",
				GSI4PK:          "MODERATION_EVENTS",
				GSI4SK:          "cursor-event-global-1",
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
				Severity:        "high",
				ConfidenceScore: 0.8,
				GSI2SK:          "cursor-event-2",
				GSI4PK:          "MODERATION_EVENTS",
				GSI4SK:          "cursor-event-global-2",
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
				Severity:        "medium",
				ConfidenceScore: 0.7,
				GSI2SK:          "cursor-event-3",
				GSI4PK:          "MODERATION_EVENTS",
				GSI4SK:          "cursor-event-global-3",
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
		requireStatus(t, http.StatusOK)(h.HandleAdminGetAccountsLift(ctx))

		ctx2, err := round10NewLiftContext(http.MethodGet, "/api/v1/admin/accounts/user-alice", headers, nil, nil)
		require.NoError(t, err)
		ctx2.Params["id"] = "user-alice"
		requireStatus(t, http.StatusOK)(h.HandleAdminGetAccountLift(ctx2))
	})

	t.Run("account actions", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/accounts/user-alice/action", headers, nil, apimodels.AdminAccountActionRequest{Type: "sensitive"})
		require.NoError(t, err)
		ctx.Params["id"] = "user-alice"
		requireStatus(t, http.StatusNoContent)(h.HandleAdminAccountActionLift(ctx))

		// Exercise additional action switch branches (and cancelUserFollowRelationships)
		for _, actionType := range []string{"suspend", "unsuspend", "silence", "unsilence", "disable", "enable", "approve", "unsensitive"} {
			actCtx, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/accounts/user-alice/action", headers, nil, apimodels.AdminAccountActionRequest{Type: actionType})
			require.NoError(t, err)
			actCtx.Params["id"] = "user-alice"
			requireStatus(t, http.StatusNoContent)(h.HandleAdminAccountActionLift(actCtx))
		}

		badCtx, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/accounts/user-alice/action", headers, nil, apimodels.AdminAccountActionRequest{Type: "not-a-real-action"})
		require.NoError(t, err)
		badCtx.Params["id"] = "user-alice"
		requireStatus(t, http.StatusBadRequest)(h.HandleAdminAccountActionLift(badCtx))

		ctx2, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/accounts/user-alice/approve", headers, nil, nil)
		require.NoError(t, err)
		ctx2.Params["id"] = "user-alice"
		requireStatus(t, http.StatusNoContent)(h.HandleAdminApproveAccountLift(ctx2))

		ctx3, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/accounts/user-alice/reject", headers, nil, nil)
		require.NoError(t, err)
		ctx3.Params["id"] = "user-alice"
		requireStatus(t, http.StatusNoContent)(h.HandleAdminRejectAccountLift(ctx3))

		ctx4, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/accounts/user-alice/enable", headers, nil, nil)
		require.NoError(t, err)
		ctx4.Params["id"] = "user-alice"
		requireStatus(t, http.StatusNoContent)(h.HandleAdminEnableAccountLift(ctx4))

		ctx5, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/accounts/user-alice/unsuspend", headers, nil, nil)
		require.NoError(t, err)
		ctx5.Params["id"] = "user-alice"
		requireStatus(t, http.StatusNoContent)(h.HandleAdminUnsuspendAccountLift(ctx5))

		ctx6, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/accounts/user-alice/unsilence", headers, nil, nil)
		require.NoError(t, err)
		ctx6.Params["id"] = "user-alice"
		requireStatus(t, http.StatusNoContent)(h.HandleAdminUnsilenceAccountLift(ctx6))

		ctx7, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/accounts/user-alice/unsensitive", headers, nil, nil)
		require.NoError(t, err)
		ctx7.Params["id"] = "user-alice"
		requireStatus(t, http.StatusNoContent)(h.HandleAdminUnsensitiveAccountLift(ctx7))
	})

	t.Run("reports and moderation", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/admin/reports", headers, map[string]string{"limit": "1", "status": "resolved"}, nil)
		require.NoError(t, err)
		resp := requireStatus(t, http.StatusOK)(h.HandleAdminGetReportsLift(ctx))
		require.Contains(t, resp.Headers, "link")

		ctx2, err := round10NewLiftContext(http.MethodGet, "/api/v1/admin/reports/r1", headers, nil, nil)
		require.NoError(t, err)
		ctx2.Params["id"] = "r1"
		requireStatus(t, http.StatusOK)(h.HandleAdminGetReportLift(ctx2))

		ctxNotFound, err := round10NewLiftContext(http.MethodGet, "/api/v1/admin/reports/missing", headers, nil, nil)
		require.NoError(t, err)
		ctxNotFound.Params["id"] = "missing"
		requireStatus(t, http.StatusNotFound)(h.HandleAdminGetReportLift(ctxNotFound))

		ctx3, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/reports/r1/resolve", headers, nil, nil)
		require.NoError(t, err)
		ctx3.Params["id"] = "r1"
		requireStatus(t, http.StatusOK)(h.HandleAdminResolveReportLift(ctx3))

		ctx3b, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/reports/not-there/resolve", headers, nil, nil)
		require.NoError(t, err)
		ctx3b.Params["id"] = "not-there"
		requireStatus(t, http.StatusNotFound)(h.HandleAdminResolveReportLift(ctx3b))

		ctx4, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/reports/r1/reopen", headers, nil, nil)
		require.NoError(t, err)
		ctx4.Params["id"] = "r1"
		requireStatus(t, http.StatusOK)(h.HandleAdminReopenReportLift(ctx4))

		ctx4b, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/reports/not-there/reopen", headers, nil, nil)
		require.NoError(t, err)
		ctx4b.Params["id"] = "not-there"
		requireStatus(t, http.StatusNotFound)(h.HandleAdminReopenReportLift(ctx4b))

		ctx5, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/reports/r1/assign_to_self", headers, nil, nil)
		require.NoError(t, err)
		ctx5.Params["id"] = "r1"
		requireStatus(t, http.StatusOK)(h.HandleAdminAssignReportLift(ctx5))

		ctx5b, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/reports/not-there/assign_to_self", headers, nil, nil)
		require.NoError(t, err)
		ctx5b.Params["id"] = "not-there"
		requireStatus(t, http.StatusNotFound)(h.HandleAdminAssignReportLift(ctx5b))

		ctx6, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/reports/r1/unassign", headers, nil, nil)
		require.NoError(t, err)
		ctx6.Params["id"] = "r1"
		requireStatus(t, http.StatusOK)(h.HandleAdminUnassignReportLift(ctx6))

		ctx6b, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/reports/not-there/unassign", headers, nil, nil)
		require.NoError(t, err)
		ctx6b.Params["id"] = "not-there"
		requireStatus(t, http.StatusNotFound)(h.HandleAdminUnassignReportLift(ctx6b))

		ctx7, err := round10NewLiftContext(http.MethodGet, "/api/v1/admin/moderation/overview", headers, nil, nil)
		require.NoError(t, err)
		requireStatus(t, http.StatusOK)(h.HandleAdminModerationOverviewLift(ctx7))
	})

	t.Run("moderation events override", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/admin/moderation/events", headers, map[string]string{"limit": "1", "min_severity": "2"}, nil)
		require.NoError(t, err)
		resp := requireStatus(t, http.StatusOK)(h.HandleAdminGetModerationEventsLift(ctx))
		require.Contains(t, resp.Headers, "link")

		overrideReq := apimodels.AdminModerationEventOverrideRequest{Decision: "reject", Reason: "bad"}
		ctx2, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/moderation/events/evt1/override", headers, nil, overrideReq)
		require.NoError(t, err)
		ctx2.Params["id"] = "evt1"
		requireStatus(t, http.StatusOK)(h.HandleAdminOverrideModerationEventLift(ctx2))

		ctx3, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/moderation/events/evt1/override", headers, nil, apimodels.AdminModerationEventOverrideRequest{Decision: "approve"})
		require.NoError(t, err)
		ctx3.Params["id"] = "evt1"
		requireStatus(t, http.StatusOK)(h.HandleAdminOverrideModerationEventLift(ctx3))

		ctx4, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/moderation/events/evt1/override", headers, nil, apimodels.AdminModerationEventOverrideRequest{Decision: "maybe"})
		require.NoError(t, err)
		ctx4.Params["id"] = "evt1"
		requireStatus(t, http.StatusBadRequest)(h.HandleAdminOverrideModerationEventLift(ctx4))

		ctx5 := round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v1/admin/moderation/events/evt1/override", headers, nil, []byte("{"))
		ctx5.Params["id"] = "evt1"
		requireStatus(t, http.StatusBadRequest)(h.HandleAdminOverrideModerationEventLift(ctx5))

		ctx6, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/moderation/events/missing/override", headers, nil, apimodels.AdminModerationEventOverrideRequest{Decision: "reject"})
		require.NoError(t, err)
		ctx6.Params["id"] = "missing"
		requireStatus(t, http.StatusNotFound)(h.HandleAdminOverrideModerationEventLift(ctx6))
	})

	t.Run("trust and reviewers", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/admin/moderation/trust/graph", headers, map[string]string{"limit": "1"}, nil)
		require.NoError(t, err)
		requireStatus(t, http.StatusOK)(h.HandleAdminGetTrustGraphLift(ctx))

		ctxDefaultLimit, err := round10NewLiftContext(http.MethodGet, "/api/v1/admin/moderation/trust/graph", headers, nil, nil)
		require.NoError(t, err)
		requireStatus(t, http.StatusOK)(h.HandleAdminGetTrustGraphLift(ctxDefaultLimit))

		updateReq := apimodels.AdminUpdateTrustRequest{Trust: 0.2, Category: "general", Reason: "test"}
		ctx2, err := round10NewLiftContext(http.MethodPut, "/api/v1/admin/moderation/trust/a/b", headers, nil, updateReq)
		require.NoError(t, err)
		ctx2.Params["from"] = "https://example.com/users/admin"
		ctx2.Params["to"] = "https://example.com/users/alice"
		requireStatus(t, http.StatusOK)(h.HandleAdminUpdateTrustLift(ctx2))

		badTrustReq := apimodels.AdminUpdateTrustRequest{Trust: 2.0}
		ctx2b, err := round10NewLiftContext(http.MethodPut, "/api/v1/admin/moderation/trust/a/b", headers, nil, badTrustReq)
		require.NoError(t, err)
		ctx2b.Params["from"] = "https://example.com/users/admin"
		ctx2b.Params["to"] = "https://example.com/users/alice"
		requireStatus(t, http.StatusBadRequest)(h.HandleAdminUpdateTrustLift(ctx2b))

		ctx2c := round10NewLiftContextWithBodyBytes(http.MethodPut, "/api/v1/admin/moderation/trust/a/b", headers, nil, []byte("{"))
		ctx2c.Params["from"] = "https://example.com/users/admin"
		ctx2c.Params["to"] = "https://example.com/users/alice"
		requireStatus(t, http.StatusBadRequest)(h.HandleAdminUpdateTrustLift(ctx2c))

		ctx3, err := round10NewLiftContext(http.MethodGet, "/api/v1/admin/moderation/reviewers", headers, nil, nil)
		require.NoError(t, err)
		requireStatus(t, http.StatusOK)(h.HandleAdminGetReviewersLift(ctx3))

		ctx4, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/moderation/reviewers/user-alice/promote", headers, nil, nil)
		require.NoError(t, err)
		ctx4.Params["id"] = "user-alice"
		requireStatus(t, http.StatusOK)(h.HandleAdminPromoteModeratorLift(ctx4))

		ctx5, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/moderation/reviewers/user-mod/demote", headers, nil, nil)
		require.NoError(t, err)
		ctx5.Params["id"] = "user-mod"
		requireStatus(t, http.StatusOK)(h.HandleAdminDemoteModeratorLift(ctx5))

		ctx6, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/moderation/reviewers/user-missing/demote", headers, nil, nil)
		require.NoError(t, err)
		ctx6.Params["id"] = "user-missing"
		requireStatus(t, http.StatusNotFound)(h.HandleAdminDemoteModeratorLift(ctx6))
	})

	t.Run("status admin endpoints", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/admin/statuses", headers, map[string]string{
			"limit":         "1",
			"include_count": "true",
			"local":         "true",
			"min_date":      now.Add(-24 * time.Hour).Format(time.RFC3339),
		}, nil)
		require.NoError(t, err)
		resp := requireStatus(t, http.StatusOK)(h.HandleAdminGetStatusesLift(ctx))
		require.Contains(t, resp.Headers, "x-total-count")
		require.Contains(t, resp.Headers, "link")

		ctx2, err := round10NewLiftContext(http.MethodGet, "/api/v1/admin/statuses/s1", headers, nil, nil)
		require.NoError(t, err)
		ctx2.Params["id"] = "s1"
		requireStatus(t, http.StatusOK)(h.HandleAdminGetStatusLift(ctx2))

		ctx2b, err := round10NewLiftContext(http.MethodGet, "/api/v1/admin/statuses/missing", headers, nil, nil)
		require.NoError(t, err)
		ctx2b.Params["id"] = "missing"
		requireStatus(t, http.StatusNotFound)(h.HandleAdminGetStatusLift(ctx2b))

		ctx3, err := round10NewLiftContext(http.MethodDelete, "/api/v1/admin/statuses/s1", headers, nil, nil)
		require.NoError(t, err)
		ctx3.Params["id"] = "s1"
		requireStatus(t, http.StatusNoContent)(h.HandleAdminDeleteStatusLift(ctx3))

		ctx3b, err := round10NewLiftContext(http.MethodDelete, "/api/v1/admin/statuses/missing", headers, nil, nil)
		require.NoError(t, err)
		ctx3b.Params["id"] = "missing"
		requireStatus(t, http.StatusNotFound)(h.HandleAdminDeleteStatusLift(ctx3b))

		ctx4, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/statuses/s1/sensitive", headers, nil, nil)
		require.NoError(t, err)
		ctx4.Params["id"] = "s1"
		requireStatus(t, http.StatusOK)(h.HandleAdminMarkStatusSensitiveLift(ctx4))

		ctx4b, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/statuses/missing/sensitive", headers, nil, nil)
		require.NoError(t, err)
		ctx4b.Params["id"] = "missing"
		requireStatus(t, http.StatusNotFound)(h.HandleAdminMarkStatusSensitiveLift(ctx4b))

		ctx5, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/statuses/s1/unsensitive", headers, nil, nil)
		require.NoError(t, err)
		ctx5.Params["id"] = "s1"
		requireStatus(t, http.StatusOK)(h.HandleAdminUnmarkStatusSensitiveLift(ctx5))

		ctx5b, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/statuses/missing/unsensitive", headers, nil, nil)
		require.NoError(t, err)
		ctx5b.Params["id"] = "missing"
		requireStatus(t, http.StatusNotFound)(h.HandleAdminUnmarkStatusSensitiveLift(ctx5b))
	})

	t.Run("forbidden when missing auth", func(t *testing.T) {
		noAuthHeaders := map[string]string{}

		ctx0, err := round10NewLiftContext(http.MethodGet, "/api/v1/admin/accounts", noAuthHeaders, nil, nil)
		require.NoError(t, err)
		requireStatus(t, http.StatusForbidden)(h.HandleAdminGetAccountsLift(ctx0))

		ctx1, err := round10NewLiftContext(http.MethodGet, "/api/v1/admin/accounts/user-alice", noAuthHeaders, nil, nil)
		require.NoError(t, err)
		ctx1.Params["id"] = "user-alice"
		requireStatus(t, http.StatusForbidden)(h.HandleAdminGetAccountLift(ctx1))

		ctx2, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/accounts/user-alice/action", noAuthHeaders, nil, nil)
		require.NoError(t, err)
		ctx2.Params["id"] = "user-alice"
		requireStatus(t, http.StatusForbidden)(h.HandleAdminAccountActionLift(ctx2))

		ctx3, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/accounts/user-alice/approve", noAuthHeaders, nil, nil)
		require.NoError(t, err)
		ctx3.Params["id"] = "user-alice"
		requireStatus(t, http.StatusForbidden)(h.HandleAdminApproveAccountLift(ctx3))

		ctx4, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/accounts/user-alice/reject", noAuthHeaders, nil, nil)
		require.NoError(t, err)
		ctx4.Params["id"] = "user-alice"
		requireStatus(t, http.StatusForbidden)(h.HandleAdminRejectAccountLift(ctx4))

		ctx5, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/accounts/user-alice/enable", noAuthHeaders, nil, nil)
		require.NoError(t, err)
		ctx5.Params["id"] = "user-alice"
		requireStatus(t, http.StatusForbidden)(h.HandleAdminEnableAccountLift(ctx5))

		ctx6, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/accounts/user-alice/unsilence", noAuthHeaders, nil, nil)
		require.NoError(t, err)
		ctx6.Params["id"] = "user-alice"
		requireStatus(t, http.StatusForbidden)(h.HandleAdminUnsilenceAccountLift(ctx6))

		ctx7, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/accounts/user-alice/unsuspend", noAuthHeaders, nil, nil)
		require.NoError(t, err)
		ctx7.Params["id"] = "user-alice"
		requireStatus(t, http.StatusForbidden)(h.HandleAdminUnsuspendAccountLift(ctx7))

		ctx8, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/accounts/user-alice/unsensitive", noAuthHeaders, nil, nil)
		require.NoError(t, err)
		ctx8.Params["id"] = "user-alice"
		requireStatus(t, http.StatusForbidden)(h.HandleAdminUnsensitiveAccountLift(ctx8))

		ctx9, err := round10NewLiftContext(http.MethodGet, "/api/v1/admin/reports", noAuthHeaders, nil, nil)
		require.NoError(t, err)
		requireStatus(t, http.StatusForbidden)(h.HandleAdminGetReportsLift(ctx9))

		ctx10, err := round10NewLiftContext(http.MethodGet, "/api/v1/admin/reports/r1", noAuthHeaders, nil, nil)
		require.NoError(t, err)
		ctx10.Params["id"] = "r1"
		requireStatus(t, http.StatusForbidden)(h.HandleAdminGetReportLift(ctx10))

		ctx11, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/reports/r1/resolve", noAuthHeaders, nil, nil)
		require.NoError(t, err)
		ctx11.Params["id"] = "r1"
		requireStatus(t, http.StatusForbidden)(h.HandleAdminResolveReportLift(ctx11))

		ctx12, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/reports/r1/reopen", noAuthHeaders, nil, nil)
		require.NoError(t, err)
		ctx12.Params["id"] = "r1"
		requireStatus(t, http.StatusForbidden)(h.HandleAdminReopenReportLift(ctx12))

		ctx13, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/reports/r1/assign_to_self", noAuthHeaders, nil, nil)
		require.NoError(t, err)
		ctx13.Params["id"] = "r1"
		requireStatus(t, http.StatusForbidden)(h.HandleAdminAssignReportLift(ctx13))

		ctx14, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/reports/r1/unassign", noAuthHeaders, nil, nil)
		require.NoError(t, err)
		ctx14.Params["id"] = "r1"
		requireStatus(t, http.StatusForbidden)(h.HandleAdminUnassignReportLift(ctx14))

		ctx15, err := round10NewLiftContext(http.MethodGet, "/api/v1/admin/moderation/overview", noAuthHeaders, nil, nil)
		require.NoError(t, err)
		requireStatus(t, http.StatusForbidden)(h.HandleAdminModerationOverviewLift(ctx15))

		ctx16, err := round10NewLiftContext(http.MethodGet, "/api/v1/admin/moderation/events", noAuthHeaders, nil, nil)
		require.NoError(t, err)
		requireStatus(t, http.StatusForbidden)(h.HandleAdminGetModerationEventsLift(ctx16))

		ctx17, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/moderation/events/evt1/override", noAuthHeaders, nil, nil)
		require.NoError(t, err)
		ctx17.Params["id"] = "evt1"
		requireStatus(t, http.StatusForbidden)(h.HandleAdminOverrideModerationEventLift(ctx17))

		ctx18, err := round10NewLiftContext(http.MethodGet, "/api/v1/admin/moderation/trust/graph", noAuthHeaders, nil, nil)
		require.NoError(t, err)
		requireStatus(t, http.StatusForbidden)(h.HandleAdminGetTrustGraphLift(ctx18))

		ctx19, err := round10NewLiftContext(http.MethodPut, "/api/v1/admin/moderation/trust/a/b", noAuthHeaders, nil, nil)
		require.NoError(t, err)
		ctx19.Params["from"] = "a"
		ctx19.Params["to"] = "b"
		requireStatus(t, http.StatusForbidden)(h.HandleAdminUpdateTrustLift(ctx19))

		ctx20, err := round10NewLiftContext(http.MethodGet, "/api/v1/admin/moderation/reviewers", noAuthHeaders, nil, nil)
		require.NoError(t, err)
		requireStatus(t, http.StatusForbidden)(h.HandleAdminGetReviewersLift(ctx20))

		ctx21, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/moderation/reviewers/user-alice/promote", noAuthHeaders, nil, nil)
		require.NoError(t, err)
		ctx21.Params["id"] = "user-alice"
		requireStatus(t, http.StatusForbidden)(h.HandleAdminPromoteModeratorLift(ctx21))

		ctx22, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/moderation/reviewers/user-alice/demote", noAuthHeaders, nil, nil)
		require.NoError(t, err)
		ctx22.Params["id"] = "user-alice"
		requireStatus(t, http.StatusForbidden)(h.HandleAdminDemoteModeratorLift(ctx22))

		ctx23, err := round10NewLiftContext(http.MethodGet, "/api/v1/admin/statuses", noAuthHeaders, nil, nil)
		require.NoError(t, err)
		requireStatus(t, http.StatusForbidden)(h.HandleAdminGetStatusesLift(ctx23))

		ctx24, err := round10NewLiftContext(http.MethodGet, "/api/v1/admin/statuses/s1", noAuthHeaders, nil, nil)
		require.NoError(t, err)
		ctx24.Params["id"] = "s1"
		requireStatus(t, http.StatusForbidden)(h.HandleAdminGetStatusLift(ctx24))

		ctx25, err := round10NewLiftContext(http.MethodDelete, "/api/v1/admin/statuses/s1", noAuthHeaders, nil, nil)
		require.NoError(t, err)
		ctx25.Params["id"] = "s1"
		requireStatus(t, http.StatusForbidden)(h.HandleAdminDeleteStatusLift(ctx25))

		ctx26, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/statuses/s1/sensitive", noAuthHeaders, nil, nil)
		require.NoError(t, err)
		ctx26.Params["id"] = "s1"
		requireStatus(t, http.StatusForbidden)(h.HandleAdminMarkStatusSensitiveLift(ctx26))

		ctx27, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/statuses/s1/unsensitive", noAuthHeaders, nil, nil)
		require.NoError(t, err)
		ctx27.Params["id"] = "s1"
		requireStatus(t, http.StatusForbidden)(h.HandleAdminUnmarkStatusSensitiveLift(ctx27))
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

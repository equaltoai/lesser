package lift

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/activitypub"
	stdErrors "github.com/equaltoai/lesser/pkg/errors"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	dynamormerrors "github.com/pay-theory/dynamorm/pkg/errors"
	"github.com/stretchr/testify/require"
)

func TestModerationHandlers_Round12_ErrorPaths(t *testing.T) {
	cfg := round11TestConfig()
	now := time.Now()

	baseState := func() *round10QueryState {
		longContent := strings.Repeat("x", 120)
		return &round10QueryState{
			usersByUsername: map[string]storagemodels.User{
				"admin": {PK: "USER#admin", Username: "admin", Role: roleAdmin, Approved: true, Version: 1, DisplayName: "Admin"},
				"mod":   {PK: "USER#mod", Username: "mod", Role: roleModerator, Approved: true, Version: 1, DisplayName: "Mod"},
				"alice": {PK: "USER#alice", Username: "alice", Role: "user", Approved: true, Version: 1, DisplayName: "Alice"},
			},
			actorsByUser: map[string]storagemodels.Actor{
				"admin": {Username: "admin", Actor: &activitypub.Actor{BaseObject: activitypub.BaseObject{ID: cfg.BaseURL() + "/users/admin", Type: "Person"}, PreferredUsername: "admin", Name: "Admin"}},
				"alice": {Username: "alice", Actor: &activitypub.Actor{BaseObject: activitypub.BaseObject{ID: cfg.BaseURL() + "/users/alice", Type: "Person"}, PreferredUsername: "alice", Name: "Alice"}},
			},
			statusByID: map[string]storagemodels.Status{
				"status-1": {StatusID: "status-1", AuthorUsername: "alice", AuthorID: cfg.BaseURL() + "/users/alice", Content: longContent, CreatedAt: now.Add(-1 * time.Hour), UpdatedAt: now},
			},
			eventsByID: map[string]storagemodels.ModerationEvent{
				"evt1": {ID: "evt1", Type: "EVENT", EventType: "flagged", ObjectID: "status-1", ObjectType: "status", ActorID: cfg.BaseURL() + "/users/alice", Category: "spam", Severity: "4", ConfidenceScore: 0.9, GSI2SK: "cursor-evt1", Created: now.Add(-2 * time.Hour)},
				"evt2": {ID: "evt2", Type: "EVENT", EventType: "flagged", ObjectID: "status-1", ObjectType: "status", ActorID: cfg.BaseURL() + "/users/alice", Category: "spam", Severity: "2", ConfidenceScore: 0.6, GSI2SK: "cursor-evt2", Created: now.Add(-90 * time.Minute)},
				"evt3": {ID: "evt3", Type: "EVENT", EventType: "flagged", ObjectID: "status-1", ObjectType: "status", ActorID: cfg.BaseURL() + "/users/alice", Category: "spam", Severity: "1", ConfidenceScore: 0.5, GSI2SK: "cursor-evt3", Created: now.Add(-80 * time.Minute)},
			},
			moderationReviews: []storagemodels.ModerationReview{
				{
					Type:       "REVIEW",
					ID:         "review-1",
					EventID:    "evt1",
					ReviewerID: cfg.BaseURL() + "/users/admin",
					Action:     "approve",
					Confidence: 0.7,
					Created:    now.Add(-1 * time.Hour),
				},
			},
			moderationDecisionsByObject: map[string]storagemodels.ModerationDecision{
				"status-1": {Type: "DECISION", ID: "decision-1", EventID: "evt1", ObjectID: "status-1", Action: "approve", ConsensusScore: 0.8, Decided: now.Add(-30 * time.Minute)},
			},
			trustRelationships: []storagemodels.TrustRelationship{
				{ID: "trust-1", TrusterID: cfg.BaseURL() + "/users/admin", TrusteeID: "https://remote.example/users/bob", Category: storagemodels.TrustCategoryContent, Score: 0.6, Confidence: 0.7, Created: now.Add(-2 * time.Hour), Updated: now.Add(-2 * time.Hour), Type: "RELATIONSHIP"},
			},
		}
	}

	makeToken := func(username string) string {
		return round11SignAccessToken(t, cfg.JWTSecret, username, []string{"read", "write"})
	}

	t.Run("parseSeverity covers all branches", func(t *testing.T) {
		require.Equal(t, 1, parseSeverity("1"))
		require.Equal(t, 2, parseSeverity("2"))
		require.Equal(t, 3, parseSeverity("3"))
		require.Equal(t, 4, parseSeverity("4"))
		require.Equal(t, 2, parseSeverity("unknown"))
	})

	t.Run("HandleModerationFlagLift error branches", func(t *testing.T) {
		t.Run("missing token returns 401", func(t *testing.T) {
			h, _, _ := round11NewHandler(t, cfg, baseState())
			ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/moderation/flag", nil, nil, models.FlagRequest{})
			require.NoError(t, err)
			require.NoError(t, h.HandleModerationFlagLift(ctx))
			require.Equal(t, http.StatusUnauthorized, ctx.Response.StatusCode)
		})

		t.Run("invalid token returns 401", func(t *testing.T) {
			h, _, _ := round11NewHandler(t, cfg, baseState())
			ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/moderation/flag", map[string]string{"Authorization": "Bearer invalid"}, nil, models.FlagRequest{})
			require.NoError(t, err)
			require.NoError(t, h.HandleModerationFlagLift(ctx))
			require.Equal(t, http.StatusUnauthorized, ctx.Response.StatusCode)
		})

		t.Run("bad body returns 400", func(t *testing.T) {
			h, _, _ := round11NewHandler(t, cfg, baseState())
			ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v1/moderation/flag", map[string]string{"Authorization": "Bearer " + makeToken("alice")}, nil, []byte("{bad"))
			require.NoError(t, h.HandleModerationFlagLift(ctx))
			require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)
		})

		t.Run("missing object_id returns 400", func(t *testing.T) {
			h, _, _ := round11NewHandler(t, cfg, baseState())
			ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/moderation/flag", map[string]string{"Authorization": "Bearer " + makeToken("alice")}, nil, models.FlagRequest{
				ObjectType: "status",
				Reason:     "spam",
			})
			require.NoError(t, err)
			require.NoError(t, h.HandleModerationFlagLift(ctx))
			require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)
		})

		t.Run("missing reason returns 400", func(t *testing.T) {
			h, _, _ := round11NewHandler(t, cfg, baseState())
			ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/moderation/flag", map[string]string{"Authorization": "Bearer " + makeToken("alice")}, nil, models.FlagRequest{
				ObjectID:   "status-1",
				ObjectType: "status",
			})
			require.NoError(t, err)
			require.NoError(t, h.HandleModerationFlagLift(ctx))
			require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)
		})

		t.Run("defaults for category/severity/confidence apply", func(t *testing.T) {
			h, _, _ := round11NewHandler(t, cfg, baseState())
			ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/moderation/flag", map[string]string{"Authorization": "Bearer " + makeToken("alice")}, nil, models.FlagRequest{
				ObjectID:        "status-1",
				ObjectType:      "status",
				Reason:          "spam",
				Category:        "",
				Severity:        99,
				ConfidenceScore: 2,
			})
			require.NoError(t, err)
			require.NoError(t, h.HandleModerationFlagLift(ctx))
			require.Equal(t, http.StatusCreated, ctx.Response.StatusCode)

			resp := ctx.Response.Body.(models.ModerationEventResponse)
			require.Equal(t, "other", resp.Category)
			require.Equal(t, 2, resp.Severity)
			require.Equal(t, 0.5, resp.ConfidenceScore)
		})

		t.Run("actor lookup error returns 500", func(t *testing.T) {
			state := baseState()
			state.firstErrorPK = map[string]error{"ACTOR#alice": stdErrors.Internal("boom")}
			h, _, _ := round11NewHandler(t, cfg, state)
			ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/moderation/flag", map[string]string{"Authorization": "Bearer " + makeToken("alice")}, nil, models.FlagRequest{
				ObjectID:   "status-1",
				ObjectType: "status",
				Reason:     "spam",
			})
			require.NoError(t, err)
			require.NoError(t, h.HandleModerationFlagLift(ctx))
			require.Equal(t, http.StatusInternalServerError, ctx.Response.StatusCode)
		})

		t.Run("create moderation event error returns 500", func(t *testing.T) {
			state := baseState()
			state.createErrorOnce = stdErrors.Internal("boom")
			h, _, _ := round11NewHandler(t, cfg, state)
			ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/moderation/flag", map[string]string{"Authorization": "Bearer " + makeToken("alice")}, nil, models.FlagRequest{
				ObjectID:   "status-1",
				ObjectType: "status",
				Reason:     "spam",
			})
			require.NoError(t, err)
			require.NoError(t, h.HandleModerationFlagLift(ctx))
			require.Equal(t, http.StatusInternalServerError, ctx.Response.StatusCode)
		})
	})

	t.Run("HandleModerationQueueLift role and repo errors", func(t *testing.T) {
		t.Run("missing token returns 401", func(t *testing.T) {
			h, _, _ := round11NewHandler(t, cfg, baseState())
			ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/moderation/queue", nil, nil, nil)
			require.NoError(t, err)
			require.NoError(t, h.HandleModerationQueueLift(ctx))
			require.Equal(t, http.StatusUnauthorized, ctx.Response.StatusCode)
		})

		t.Run("invalid token returns 401", func(t *testing.T) {
			h, _, _ := round11NewHandler(t, cfg, baseState())
			ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/moderation/queue", map[string]string{"Authorization": "Bearer invalid"}, nil, nil)
			require.NoError(t, err)
			require.NoError(t, h.HandleModerationQueueLift(ctx))
			require.Equal(t, http.StatusUnauthorized, ctx.Response.StatusCode)
		})

		t.Run("forbidden role returns 403", func(t *testing.T) {
			h, _, _ := round11NewHandler(t, cfg, baseState())
			ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/moderation/queue", map[string]string{"Authorization": "Bearer " + makeToken("alice")}, nil, nil)
			require.NoError(t, err)
			require.NoError(t, h.HandleModerationQueueLift(ctx))
			require.Equal(t, http.StatusForbidden, ctx.Response.StatusCode)
		})

		t.Run("pagination header set when next cursor exists", func(t *testing.T) {
			h, _, _ := round11NewHandler(t, cfg, baseState())
			ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/moderation/queue", map[string]string{"Authorization": "Bearer " + makeToken("admin")}, map[string]string{"limit": "1"}, nil)
			require.NoError(t, err)
			require.NoError(t, h.HandleModerationQueueLift(ctx))
			require.Equal(t, http.StatusOK, ctx.Response.StatusCode)
			require.NotEmpty(t, ctx.Response.Headers["X-Next-Cursor"])
		})

		t.Run("repo error returns 500", func(t *testing.T) {
			state := baseState()
			state.allErrorByType = map[string]error{"*[]models.ModerationEvent": stdErrors.Internal("boom")}
			h, _, _ := round11NewHandler(t, cfg, state)
			ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/moderation/queue", map[string]string{"Authorization": "Bearer " + makeToken("admin")}, nil, nil)
			require.NoError(t, err)
			require.NoError(t, h.HandleModerationQueueLift(ctx))
			require.Equal(t, http.StatusInternalServerError, ctx.Response.StatusCode)
		})
	})

	t.Run("HandleModerationReviewLift validation and repo errors", func(t *testing.T) {
		t.Run("missing token returns 401", func(t *testing.T) {
			h, _, _ := round11NewHandler(t, cfg, baseState())
			ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/moderation/review", nil, nil, models.ReviewRequest{})
			require.NoError(t, err)
			require.NoError(t, h.HandleModerationReviewLift(ctx))
			require.Equal(t, http.StatusUnauthorized, ctx.Response.StatusCode)
		})

		t.Run("invalid token returns 401", func(t *testing.T) {
			h, _, _ := round11NewHandler(t, cfg, baseState())
			ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/moderation/review", map[string]string{"Authorization": "Bearer invalid"}, nil, models.ReviewRequest{})
			require.NoError(t, err)
			require.NoError(t, h.HandleModerationReviewLift(ctx))
			require.Equal(t, http.StatusUnauthorized, ctx.Response.StatusCode)
		})

		t.Run("forbidden role returns 403", func(t *testing.T) {
			h, _, _ := round11NewHandler(t, cfg, baseState())
			ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/moderation/review", map[string]string{"Authorization": "Bearer " + makeToken("alice")}, nil, models.ReviewRequest{})
			require.NoError(t, err)
			require.NoError(t, h.HandleModerationReviewLift(ctx))
			require.Equal(t, http.StatusForbidden, ctx.Response.StatusCode)
		})

		t.Run("bad body returns 400", func(t *testing.T) {
			h, _, _ := round11NewHandler(t, cfg, baseState())
			ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v1/moderation/review", map[string]string{"Authorization": "Bearer " + makeToken("admin")}, nil, []byte("{bad"))
			require.NoError(t, h.HandleModerationReviewLift(ctx))
			require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)
		})

		t.Run("missing event_id returns 400", func(t *testing.T) {
			h, _, _ := round11NewHandler(t, cfg, baseState())
			ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/moderation/review", map[string]string{"Authorization": "Bearer " + makeToken("admin")}, nil, models.ReviewRequest{
				Action:     "approve",
				Confidence: 0.7,
			})
			require.NoError(t, err)
			require.NoError(t, h.HandleModerationReviewLift(ctx))
			require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)
		})

		t.Run("invalid confidence returns 400", func(t *testing.T) {
			h, _, _ := round11NewHandler(t, cfg, baseState())
			ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/moderation/review", map[string]string{"Authorization": "Bearer " + makeToken("admin")}, nil, models.ReviewRequest{
				EventID:    "evt1",
				Action:     "approve",
				Confidence: 2,
			})
			require.NoError(t, err)
			require.NoError(t, h.HandleModerationReviewLift(ctx))
			require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)
		})

		t.Run("actor lookup error returns 500", func(t *testing.T) {
			state := baseState()
			state.firstErrorPK = map[string]error{"ACTOR#admin": stdErrors.Internal("boom")}
			h, _, _ := round11NewHandler(t, cfg, state)
			ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/moderation/review", map[string]string{"Authorization": "Bearer " + makeToken("admin")}, nil, models.ReviewRequest{
				EventID:    "evt1",
				Action:     "approve",
				Confidence: 0.7,
			})
			require.NoError(t, err)
			require.NoError(t, h.HandleModerationReviewLift(ctx))
			require.Equal(t, http.StatusInternalServerError, ctx.Response.StatusCode)
		})

		t.Run("repo error returns 500", func(t *testing.T) {
			state := baseState()
			state.createErrorOnce = stdErrors.Internal("boom")
			h, _, _ := round11NewHandler(t, cfg, state)
			ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/moderation/review", map[string]string{"Authorization": "Bearer " + makeToken("admin")}, nil, models.ReviewRequest{
				EventID:    "evt1",
				Action:     "approve",
				Confidence: 0.7,
			})
			require.NoError(t, err)
			require.NoError(t, h.HandleModerationReviewLift(ctx))
			require.Equal(t, http.StatusInternalServerError, ctx.Response.StatusCode)
		})
	})

	t.Run("HandleModerationHistoryLift validation and forbidden", func(t *testing.T) {
		t.Run("missing token returns 401", func(t *testing.T) {
			h, _, _ := round11NewHandler(t, cfg, baseState())
			ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/moderation/history/status-1", nil, nil, nil)
			require.NoError(t, err)
			ctx.SetParam("object_id", "status-1")
			require.NoError(t, h.HandleModerationHistoryLift(ctx))
			require.Equal(t, http.StatusUnauthorized, ctx.Response.StatusCode)
		})

		t.Run("invalid token returns 401", func(t *testing.T) {
			h, _, _ := round11NewHandler(t, cfg, baseState())
			ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/moderation/history/status-1", map[string]string{"Authorization": "Bearer invalid"}, nil, nil)
			require.NoError(t, err)
			ctx.SetParam("object_id", "status-1")
			require.NoError(t, h.HandleModerationHistoryLift(ctx))
			require.Equal(t, http.StatusUnauthorized, ctx.Response.StatusCode)
		})

		t.Run("missing object_id returns 400", func(t *testing.T) {
			h, _, _ := round11NewHandler(t, cfg, baseState())
			ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/moderation/history/", map[string]string{"Authorization": "Bearer " + makeToken("admin")}, nil, nil)
			require.NoError(t, err)
			require.NoError(t, h.HandleModerationHistoryLift(ctx))
			require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)
		})

		t.Run("forbidden role returns 403", func(t *testing.T) {
			h, _, _ := round11NewHandler(t, cfg, baseState())
			ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/moderation/history/status-1", map[string]string{"Authorization": "Bearer " + makeToken("alice")}, nil, nil)
			require.NoError(t, err)
			ctx.SetParam("object_id", "status-1")
			require.NoError(t, h.HandleModerationHistoryLift(ctx))
			require.Equal(t, http.StatusForbidden, ctx.Response.StatusCode)
		})

		t.Run("success includes decisions and timeline entries", func(t *testing.T) {
			h, _, _ := round11NewHandler(t, cfg, baseState())
			ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/moderation/history/status-1", map[string]string{"Authorization": "Bearer " + makeToken("admin")}, nil, nil)
			require.NoError(t, err)
			ctx.SetParam("object_id", "status-1")
			require.NoError(t, h.HandleModerationHistoryLift(ctx))
			require.Equal(t, http.StatusOK, ctx.Response.StatusCode)

			resp := ctx.Response.Body.(models.ModerationHistoryResponse)
			require.NotEmpty(t, resp.Events)
			require.NotEmpty(t, resp.Decisions)
			require.Len(t, resp.Timeline, len(resp.Events)+len(resp.Decisions))
		})

		t.Run("repo error returns 500", func(t *testing.T) {
			state := baseState()
			state.allErrorByType = map[string]error{"*[]models.ModerationEvent": stdErrors.Internal("boom")}
			h, _, _ := round11NewHandler(t, cfg, state)
			ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/moderation/history/status-1", map[string]string{"Authorization": "Bearer " + makeToken("admin")}, nil, nil)
			require.NoError(t, err)
			ctx.SetParam("object_id", "status-1")
			require.NoError(t, h.HandleModerationHistoryLift(ctx))
			require.Equal(t, http.StatusInternalServerError, ctx.Response.StatusCode)
		})
	})

	t.Run("HandleGetConsensusLift missing params and downstream errors", func(t *testing.T) {
		t.Run("forbidden role returns 403", func(t *testing.T) {
			h, _, _ := round11NewHandler(t, cfg, baseState())
			ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/moderation/consensus/evt1", map[string]string{"Authorization": "Bearer " + makeToken("alice")}, nil, nil)
			require.NoError(t, err)
			ctx.SetParam("event_id", "evt1")
			require.NoError(t, h.HandleGetConsensusLift(ctx))
			require.Equal(t, http.StatusForbidden, ctx.Response.StatusCode)
		})

		t.Run("reviews query error returns 500", func(t *testing.T) {
			state := baseState()
			state.allErrorByType = map[string]error{"*[]models.ModerationReview": stdErrors.Internal("boom")}
			h, _, _ := round11NewHandler(t, cfg, state)
			ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/moderation/consensus/evt1", map[string]string{"Authorization": "Bearer " + makeToken("admin")}, nil, nil)
			require.NoError(t, err)
			ctx.SetParam("event_id", "evt1")
			require.NoError(t, h.HandleGetConsensusLift(ctx))
			require.Equal(t, http.StatusInternalServerError, ctx.Response.StatusCode)
		})

		t.Run("success returns visualization with reviews", func(t *testing.T) {
			h, _, _ := round11NewHandler(t, cfg, baseState())
			ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/moderation/consensus/evt1", map[string]string{"Authorization": "Bearer " + makeToken("admin")}, nil, nil)
			require.NoError(t, err)
			ctx.SetParam("event_id", "evt1")
			require.NoError(t, h.HandleGetConsensusLift(ctx))
			require.Equal(t, http.StatusOK, ctx.Response.StatusCode)

			resp := ctx.Response.Body.(models.ConsensusVisualization)
			require.NotEmpty(t, resp.Reviews)
		})

		t.Run("missing event_id returns 400", func(t *testing.T) {
			h, _, _ := round11NewHandler(t, cfg, baseState())
			ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/moderation/consensus/", map[string]string{"Authorization": "Bearer " + makeToken("admin")}, nil, nil)
			require.NoError(t, err)
			require.NoError(t, h.HandleGetConsensusLift(ctx))
			require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)
		})

		t.Run("event not found returns 404", func(t *testing.T) {
			state := baseState()
			state.firstErrorGSI3PK = map[string]error{"EVENTID#missing": dynamormerrors.ErrItemNotFound}
			h, _, _ := round11NewHandler(t, cfg, state)
			ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/moderation/consensus/missing", map[string]string{"Authorization": "Bearer " + makeToken("admin")}, nil, nil)
			require.NoError(t, err)
			ctx.SetParam("event_id", "missing")
			require.NoError(t, h.HandleGetConsensusLift(ctx))
			require.Equal(t, http.StatusNotFound, ctx.Response.StatusCode)
		})
	})

	t.Run("HandleGetTrustRelationshipsLift direction validation", func(t *testing.T) {
		t.Run("invalid direction returns 400", func(t *testing.T) {
			h, _, _ := round11NewHandler(t, cfg, baseState())
			ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/moderation/trust", map[string]string{"Authorization": "Bearer " + makeToken("alice")}, map[string]string{"direction": "sideways"}, nil)
			require.NoError(t, err)
			require.NoError(t, h.HandleGetTrustRelationshipsLift(ctx))
			require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)
		})

		t.Run("missing token returns 401", func(t *testing.T) {
			h, _, _ := round11NewHandler(t, cfg, baseState())
			ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/moderation/trust", nil, nil, nil)
			require.NoError(t, err)
			require.NoError(t, h.HandleGetTrustRelationshipsLift(ctx))
			require.Equal(t, http.StatusUnauthorized, ctx.Response.StatusCode)
		})

		t.Run("incoming branch returns 200", func(t *testing.T) {
			h, _, _ := round11NewHandler(t, cfg, baseState())
			ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/moderation/trust", map[string]string{"Authorization": "Bearer " + makeToken("alice")}, map[string]string{"direction": "incoming"}, nil)
			require.NoError(t, err)
			require.NoError(t, h.HandleGetTrustRelationshipsLift(ctx))
			require.Equal(t, http.StatusOK, ctx.Response.StatusCode)
		})

		t.Run("repo error returns 500", func(t *testing.T) {
			state := baseState()
			state.scanErrorOnce = stdErrors.Internal("boom")
			h, _, _ := round11NewHandler(t, cfg, state)
			ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/moderation/trust", map[string]string{"Authorization": "Bearer " + makeToken("alice")}, map[string]string{"direction": "outgoing"}, nil)
			require.NoError(t, err)
			require.NoError(t, h.HandleGetTrustRelationshipsLift(ctx))
			require.Equal(t, http.StatusInternalServerError, ctx.Response.StatusCode)
		})
	})

	t.Run("HandleUpdateTrustLift covers create and update branches", func(t *testing.T) {
		headers := map[string]string{"Authorization": "Bearer " + makeToken("alice")}

		t.Run("bad body returns 400", func(t *testing.T) {
			h, _, _ := round11NewHandler(t, cfg, baseState())
			ctx := round10NewLiftContextWithBodyBytes(http.MethodPut, "/api/v1/moderation/trust", headers, nil, []byte("{bad"))
			require.NoError(t, h.HandleUpdateTrustLift(ctx))
			require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)
		})

		t.Run("missing trustee_id returns 400", func(t *testing.T) {
			h, _, _ := round11NewHandler(t, cfg, baseState())
			ctx, err := round10NewLiftContext(http.MethodPut, "/api/v1/moderation/trust", headers, nil, models.UpdateTrustRequest{
				Score:      0.4,
				Confidence: 0.6,
			})
			require.NoError(t, err)
			require.NoError(t, h.HandleUpdateTrustLift(ctx))
			require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)
		})

		t.Run("invalid score returns 400", func(t *testing.T) {
			h, _, _ := round11NewHandler(t, cfg, baseState())
			ctx, err := round10NewLiftContext(http.MethodPut, "/api/v1/moderation/trust", headers, nil, models.UpdateTrustRequest{
				TrusteeID:  "https://remote.example/users/bob",
				Score:      2,
				Confidence: 0.6,
			})
			require.NoError(t, err)
			require.NoError(t, h.HandleUpdateTrustLift(ctx))
			require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)
		})

		t.Run("invalid confidence returns 400", func(t *testing.T) {
			h, _, _ := round11NewHandler(t, cfg, baseState())
			ctx, err := round10NewLiftContext(http.MethodPut, "/api/v1/moderation/trust", headers, nil, models.UpdateTrustRequest{
				TrusteeID:  "https://remote.example/users/bob",
				Score:      0.1,
				Confidence: 2,
			})
			require.NoError(t, err)
			require.NoError(t, h.HandleUpdateTrustLift(ctx))
			require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)
		})

		t.Run("actor lookup error returns 500", func(t *testing.T) {
			state := baseState()
			state.firstErrorPK = map[string]error{"ACTOR#alice": dynamormerrors.ErrItemNotFound}
			h, _, _ := round11NewHandler(t, cfg, state)
			ctx, err := round10NewLiftContext(http.MethodPut, "/api/v1/moderation/trust", headers, nil, models.UpdateTrustRequest{
				TrusteeID:  "https://remote.example/users/bob",
				Score:      0.4,
				Confidence: 0.6,
				Category:   "general",
			})
			require.NoError(t, err)
			require.NoError(t, h.HandleUpdateTrustLift(ctx))
			require.Equal(t, http.StatusInternalServerError, ctx.Response.StatusCode)
		})

		t.Run("create branch create error returns 500", func(t *testing.T) {
			state := baseState()
			state.notFoundPKSK = map[string]bool{
				"TRUST#" + cfg.BaseURL() + "/users/alice#general#TRUSTEE#https://remote.example/users/bob": true,
			}
			state.createErrorOnce = stdErrors.Internal("boom")
			h, _, _ := round11NewHandler(t, cfg, state)
			ctx, err := round10NewLiftContext(http.MethodPut, "/api/v1/moderation/trust", headers, nil, models.UpdateTrustRequest{
				TrusteeID:  "https://remote.example/users/bob",
				Score:      0.4,
				Confidence: 0.6,
				Category:   "general",
			})
			require.NoError(t, err)
			require.NoError(t, h.HandleUpdateTrustLift(ctx))
			require.Equal(t, http.StatusInternalServerError, ctx.Response.StatusCode)
		})

		t.Run("create branch when relationship not found", func(t *testing.T) {
			state := baseState()
			// Force GetTrustRelationship to return storage.ErrNotFound.
			state.notFoundPKSK = map[string]bool{
				"TRUST#" + cfg.BaseURL() + "/users/alice#general#TRUSTEE#https://remote.example/users/bob": true,
			}
			h, _, _ := round11NewHandler(t, cfg, state)
			ctx, err := round10NewLiftContext(http.MethodPut, "/api/v1/moderation/trust", headers, nil, models.UpdateTrustRequest{
				TrusteeID:  "https://remote.example/users/bob",
				Score:      0.4,
				Confidence: 0.6,
				Category:   "general",
			})
			require.NoError(t, err)
			require.NoError(t, h.HandleUpdateTrustLift(ctx))
			require.Equal(t, http.StatusOK, ctx.Response.StatusCode)
		})

		t.Run("update branch success returns 200", func(t *testing.T) {
			h, _, _ := round11NewHandler(t, cfg, baseState())
			ctx, err := round10NewLiftContext(http.MethodPut, "/api/v1/moderation/trust", headers, nil, models.UpdateTrustRequest{
				TrusteeID:  "https://remote.example/users/bob",
				Score:      0.4,
				Confidence: 0.6,
				Category:   "general",
			})
			require.NoError(t, err)
			require.NoError(t, h.HandleUpdateTrustLift(ctx))
			require.Equal(t, http.StatusOK, ctx.Response.StatusCode)
		})

		t.Run("update branch error returns 500", func(t *testing.T) {
			state := baseState()
			state.executeErrorOnce = stdErrors.Internal("boom")
			h, _, _ := round11NewHandler(t, cfg, state)
			ctx, err := round10NewLiftContext(http.MethodPut, "/api/v1/moderation/trust", headers, nil, models.UpdateTrustRequest{
				TrusteeID:  "https://remote.example/users/bob",
				Score:      0.4,
				Confidence: 0.6,
				Category:   "general",
			})
			require.NoError(t, err)
			require.NoError(t, h.HandleUpdateTrustLift(ctx))
			require.Equal(t, http.StatusInternalServerError, ctx.Response.StatusCode)
		})
	})

	t.Run("HandleGetTrustScoreLift validates params and domain extraction", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, baseState())
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/moderation/trust//score", map[string]string{"Authorization": "Bearer " + makeToken("admin")}, nil, nil)
		require.NoError(t, err)
		require.NoError(t, h.HandleGetTrustScoreLift(ctx))
		require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)

		ctx2, err := round10NewLiftContext(http.MethodGet, "/api/v1/moderation/trust/alice/score", map[string]string{"Authorization": "Bearer " + makeToken("admin")}, nil, nil)
		require.NoError(t, err)
		ctx2.SetParam("actor_id", "@alice@example.com")
		require.NoError(t, h.HandleGetTrustScoreLift(ctx2))
		require.Equal(t, http.StatusOK, ctx2.Response.StatusCode)

		t.Run("forbidden role returns 403", func(t *testing.T) {
			h, _, _ := round11NewHandler(t, cfg, baseState())
			ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/moderation/trust/alice/score", map[string]string{"Authorization": "Bearer " + makeToken("alice")}, nil, nil)
			require.NoError(t, err)
			ctx.SetParam("actor_id", "alice")
			require.NoError(t, h.HandleGetTrustScoreLift(ctx))
			require.Equal(t, http.StatusForbidden, ctx.Response.StatusCode)
		})

		// Note: forcing GetTrustedByRelationships to error deterministically in this handler
		// is difficult because trust score calculation also calls it repeatedly.
	})

	t.Run("getObjectPreview and extractDomainFromActor extra branches", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, baseState())

		// long content is truncated
		preview := h.getObjectPreview(context.Background(), "status-1", "status")
		require.True(t, strings.HasSuffix(preview, "..."))

		// invalid URL parsing
		require.Equal(t, "", h.extractDomainFromActor("https://%zz"))
		require.Equal(t, cfg.Domain, h.extractDomainFromActor("alice"))

		// URL domain extraction
		u := &url.URL{Scheme: "https", Host: "example.com", Path: "/users/alice"}
		require.Equal(t, "example.com", h.extractDomainFromActor(u.String()))

		// default and error branches
		require.Equal(t, "", h.getObjectPreview(context.Background(), "ignored", "unknown"))
		state := baseState()
		state.notFoundPKs = map[string]bool{"status#missing": true}
		h2, _, _ := round11NewHandler(t, cfg, state)
		require.Equal(t, "", h2.getObjectPreview(context.Background(), "missing", "status"))
	})
}

package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/activitypub"
	stdErrors "github.com/equaltoai/lesser/pkg/errors"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
	dynamormerrors "github.com/theory-cloud/tabletheory/v3/pkg/errors"
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
			requireStatus(t, http.StatusUnauthorized)(h.HandleModerationFlagLift(ctx))
		})

		t.Run("invalid token returns 401", func(t *testing.T) {
			h, _, _ := round11NewHandler(t, cfg, baseState())
			ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/moderation/flag", map[string]string{"Authorization": "Bearer invalid"}, nil, models.FlagRequest{})
			require.NoError(t, err)
			requireStatus(t, http.StatusUnauthorized)(h.HandleModerationFlagLift(ctx))
		})

		t.Run("bad body returns 400", func(t *testing.T) {
			h, _, _ := round11NewHandler(t, cfg, baseState())
			ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v1/moderation/flag", map[string]string{"Authorization": "Bearer " + makeToken("alice")}, nil, []byte("{bad"))
			requireStatus(t, http.StatusBadRequest)(h.HandleModerationFlagLift(ctx))
		})

		t.Run("missing object_id returns 400", func(t *testing.T) {
			h, _, _ := round11NewHandler(t, cfg, baseState())
			ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/moderation/flag", map[string]string{"Authorization": "Bearer " + makeToken("alice")}, nil, models.FlagRequest{
				ObjectType: "status",
				Reason:     "spam",
			})
			require.NoError(t, err)
			requireStatus(t, http.StatusBadRequest)(h.HandleModerationFlagLift(ctx))
		})

		t.Run("missing reason returns 400", func(t *testing.T) {
			h, _, _ := round11NewHandler(t, cfg, baseState())
			ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/moderation/flag", map[string]string{"Authorization": "Bearer " + makeToken("alice")}, nil, models.FlagRequest{
				ObjectID:   "status-1",
				ObjectType: "status",
			})
			require.NoError(t, err)
			requireStatus(t, http.StatusBadRequest)(h.HandleModerationFlagLift(ctx))
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
			resp := requireStatus(t, http.StatusCreated)(h.HandleModerationFlagLift(ctx))

			var event models.ModerationEventResponse
			require.NoError(t, json.Unmarshal(resp.Body, &event))
			require.Equal(t, "other", event.Category)
			require.Equal(t, 2, event.Severity)
			require.Equal(t, 0.5, event.ConfidenceScore)
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
			requireStatus(t, http.StatusInternalServerError)(h.HandleModerationFlagLift(ctx))
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
			requireStatus(t, http.StatusInternalServerError)(h.HandleModerationFlagLift(ctx))
		})
	})

	t.Run("HandleModerationQueueLift role and repo errors", func(t *testing.T) {
		t.Run("missing token returns 401", func(t *testing.T) {
			h, _, _ := round11NewHandler(t, cfg, baseState())
			ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/moderation/queue", nil, nil, nil)
			require.NoError(t, err)
			requireStatus(t, http.StatusUnauthorized)(h.HandleModerationQueueLift(ctx))
		})

		t.Run("invalid token returns 401", func(t *testing.T) {
			h, _, _ := round11NewHandler(t, cfg, baseState())
			ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/moderation/queue", map[string]string{"Authorization": "Bearer invalid"}, nil, nil)
			require.NoError(t, err)
			requireStatus(t, http.StatusUnauthorized)(h.HandleModerationQueueLift(ctx))
		})

		t.Run("forbidden role returns 403", func(t *testing.T) {
			h, _, _ := round11NewHandler(t, cfg, baseState())
			ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/moderation/queue", map[string]string{"Authorization": "Bearer " + makeToken("alice")}, nil, nil)
			require.NoError(t, err)
			requireStatus(t, http.StatusForbidden)(h.HandleModerationQueueLift(ctx))
		})

		t.Run("pagination header set when next cursor exists", func(t *testing.T) {
			h, _, _ := round11NewHandler(t, cfg, baseState())
			ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/moderation/queue", map[string]string{"Authorization": "Bearer " + makeToken("admin")}, map[string]string{"limit": "1"}, nil)
			require.NoError(t, err)
			resp := requireStatus(t, http.StatusOK)(h.HandleModerationQueueLift(ctx))
			require.NotEmpty(t, resp.Headers["x-next-cursor"])
		})

		t.Run("repo error returns 500", func(t *testing.T) {
			state := baseState()
			state.allErrorByType = map[string]error{"*[]models.ModerationEvent": stdErrors.Internal("boom")}
			h, _, _ := round11NewHandler(t, cfg, state)
			ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/moderation/queue", map[string]string{"Authorization": "Bearer " + makeToken("admin")}, nil, nil)
			require.NoError(t, err)
			requireStatus(t, http.StatusInternalServerError)(h.HandleModerationQueueLift(ctx))
		})
	})

	t.Run("HandleModerationReviewLift validation and repo errors", func(t *testing.T) {
		t.Run("missing token returns 401", func(t *testing.T) {
			h, _, _ := round11NewHandler(t, cfg, baseState())
			ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/moderation/review", nil, nil, models.ReviewRequest{})
			require.NoError(t, err)
			requireStatus(t, http.StatusUnauthorized)(h.HandleModerationReviewLift(ctx))
		})

		t.Run("invalid token returns 401", func(t *testing.T) {
			h, _, _ := round11NewHandler(t, cfg, baseState())
			ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/moderation/review", map[string]string{"Authorization": "Bearer invalid"}, nil, models.ReviewRequest{})
			require.NoError(t, err)
			requireStatus(t, http.StatusUnauthorized)(h.HandleModerationReviewLift(ctx))
		})

		t.Run("forbidden role returns 403", func(t *testing.T) {
			h, _, _ := round11NewHandler(t, cfg, baseState())
			ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/moderation/review", map[string]string{"Authorization": "Bearer " + makeToken("alice")}, nil, models.ReviewRequest{})
			require.NoError(t, err)
			requireStatus(t, http.StatusForbidden)(h.HandleModerationReviewLift(ctx))
		})

		t.Run("bad body returns 400", func(t *testing.T) {
			h, _, _ := round11NewHandler(t, cfg, baseState())
			ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v1/moderation/review", map[string]string{"Authorization": "Bearer " + makeToken("admin")}, nil, []byte("{bad"))
			requireStatus(t, http.StatusBadRequest)(h.HandleModerationReviewLift(ctx))
		})

		t.Run("missing event_id returns 400", func(t *testing.T) {
			h, _, _ := round11NewHandler(t, cfg, baseState())
			ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/moderation/review", map[string]string{"Authorization": "Bearer " + makeToken("admin")}, nil, models.ReviewRequest{
				Action:     "approve",
				Confidence: 0.7,
			})
			require.NoError(t, err)
			requireStatus(t, http.StatusBadRequest)(h.HandleModerationReviewLift(ctx))
		})

		t.Run("invalid confidence returns 400", func(t *testing.T) {
			h, _, _ := round11NewHandler(t, cfg, baseState())
			ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/moderation/review", map[string]string{"Authorization": "Bearer " + makeToken("admin")}, nil, models.ReviewRequest{
				EventID:    "evt1",
				Action:     "approve",
				Confidence: 2,
			})
			require.NoError(t, err)
			requireStatus(t, http.StatusBadRequest)(h.HandleModerationReviewLift(ctx))
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
			requireStatus(t, http.StatusInternalServerError)(h.HandleModerationReviewLift(ctx))
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
			requireStatus(t, http.StatusInternalServerError)(h.HandleModerationReviewLift(ctx))
		})
	})

	t.Run("HandleModerationHistoryLift validation and forbidden", func(t *testing.T) {
		t.Run("missing token returns 401", func(t *testing.T) {
			h, _, _ := round11NewHandler(t, cfg, baseState())
			ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/moderation/history/status-1", nil, nil, nil)
			require.NoError(t, err)
			ctx.Params["object_id"] = "status-1"
			requireStatus(t, http.StatusUnauthorized)(h.HandleModerationHistoryLift(ctx))
		})

		t.Run("invalid token returns 401", func(t *testing.T) {
			h, _, _ := round11NewHandler(t, cfg, baseState())
			ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/moderation/history/status-1", map[string]string{"Authorization": "Bearer invalid"}, nil, nil)
			require.NoError(t, err)
			ctx.Params["object_id"] = "status-1"
			requireStatus(t, http.StatusUnauthorized)(h.HandleModerationHistoryLift(ctx))
		})

		t.Run("missing object_id returns 400", func(t *testing.T) {
			h, _, _ := round11NewHandler(t, cfg, baseState())
			ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/moderation/history/", map[string]string{"Authorization": "Bearer " + makeToken("admin")}, nil, nil)
			require.NoError(t, err)
			requireStatus(t, http.StatusBadRequest)(h.HandleModerationHistoryLift(ctx))
		})

		t.Run("forbidden role returns 403", func(t *testing.T) {
			h, _, _ := round11NewHandler(t, cfg, baseState())
			ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/moderation/history/status-1", map[string]string{"Authorization": "Bearer " + makeToken("alice")}, nil, nil)
			require.NoError(t, err)
			ctx.Params["object_id"] = "status-1"
			requireStatus(t, http.StatusForbidden)(h.HandleModerationHistoryLift(ctx))
		})

		t.Run("success includes decisions and timeline entries", func(t *testing.T) {
			h, _, _ := round11NewHandler(t, cfg, baseState())
			ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/moderation/history/status-1", map[string]string{"Authorization": "Bearer " + makeToken("admin")}, nil, nil)
			require.NoError(t, err)
			ctx.Params["object_id"] = "status-1"
			resp := requireStatus(t, http.StatusOK)(h.HandleModerationHistoryLift(ctx))

			var history models.ModerationHistoryResponse
			require.NoError(t, json.Unmarshal(resp.Body, &history))
			require.NotEmpty(t, history.Events)
			require.NotEmpty(t, history.Decisions)
			require.Len(t, history.Timeline, len(history.Events)+len(history.Decisions))
		})

		t.Run("repo error returns 500", func(t *testing.T) {
			state := baseState()
			state.allErrorByType = map[string]error{"*[]models.ModerationEvent": stdErrors.Internal("boom")}
			h, _, _ := round11NewHandler(t, cfg, state)
			ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/moderation/history/status-1", map[string]string{"Authorization": "Bearer " + makeToken("admin")}, nil, nil)
			require.NoError(t, err)
			ctx.Params["object_id"] = "status-1"
			requireStatus(t, http.StatusInternalServerError)(h.HandleModerationHistoryLift(ctx))
		})
	})

	t.Run("HandleGetConsensusLift missing params and downstream errors", func(t *testing.T) {
		t.Run("forbidden role returns 403", func(t *testing.T) {
			h, _, _ := round11NewHandler(t, cfg, baseState())
			ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/moderation/consensus/evt1", map[string]string{"Authorization": "Bearer " + makeToken("alice")}, nil, nil)
			require.NoError(t, err)
			ctx.Params["event_id"] = "evt1"
			requireStatus(t, http.StatusForbidden)(h.HandleGetConsensusLift(ctx))
		})

		t.Run("reviews query error returns 500", func(t *testing.T) {
			state := baseState()
			state.allErrorByType = map[string]error{"*[]models.ModerationReview": stdErrors.Internal("boom")}
			h, _, _ := round11NewHandler(t, cfg, state)
			ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/moderation/consensus/evt1", map[string]string{"Authorization": "Bearer " + makeToken("admin")}, nil, nil)
			require.NoError(t, err)
			ctx.Params["event_id"] = "evt1"
			requireStatus(t, http.StatusInternalServerError)(h.HandleGetConsensusLift(ctx))
		})

		t.Run("success returns visualization with reviews", func(t *testing.T) {
			h, _, _ := round11NewHandler(t, cfg, baseState())
			ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/moderation/consensus/evt1", map[string]string{"Authorization": "Bearer " + makeToken("admin")}, nil, nil)
			require.NoError(t, err)
			ctx.Params["event_id"] = "evt1"
			resp := requireStatus(t, http.StatusOK)(h.HandleGetConsensusLift(ctx))

			var viz models.ConsensusVisualization
			require.NoError(t, json.Unmarshal(resp.Body, &viz))
			require.NotEmpty(t, viz.Reviews)
		})

		t.Run("missing event_id returns 400", func(t *testing.T) {
			h, _, _ := round11NewHandler(t, cfg, baseState())
			ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/moderation/consensus/", map[string]string{"Authorization": "Bearer " + makeToken("admin")}, nil, nil)
			require.NoError(t, err)
			requireStatus(t, http.StatusBadRequest)(h.HandleGetConsensusLift(ctx))
		})

		t.Run("event not found returns 404", func(t *testing.T) {
			state := baseState()
			state.firstErrorGSI3PK = map[string]error{"EVENTID#missing": dynamormerrors.ErrItemNotFound}
			h, _, _ := round11NewHandler(t, cfg, state)
			ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/moderation/consensus/missing", map[string]string{"Authorization": "Bearer " + makeToken("admin")}, nil, nil)
			require.NoError(t, err)
			ctx.Params["event_id"] = "missing"
			requireStatus(t, http.StatusNotFound)(h.HandleGetConsensusLift(ctx))
		})
	})

	t.Run("HandleGetTrustRelationshipsLift direction validation", func(t *testing.T) {
		t.Run("invalid direction returns 400", func(t *testing.T) {
			h, _, _ := round11NewHandler(t, cfg, baseState())
			ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/moderation/trust", map[string]string{"Authorization": "Bearer " + makeToken("alice")}, map[string]string{"direction": "sideways"}, nil)
			require.NoError(t, err)
			requireStatus(t, http.StatusBadRequest)(h.HandleGetTrustRelationshipsLift(ctx))
		})

		t.Run("missing token returns 401", func(t *testing.T) {
			h, _, _ := round11NewHandler(t, cfg, baseState())
			ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/moderation/trust", nil, nil, nil)
			require.NoError(t, err)
			requireStatus(t, http.StatusUnauthorized)(h.HandleGetTrustRelationshipsLift(ctx))
		})

		t.Run("incoming branch returns 200", func(t *testing.T) {
			h, _, _ := round11NewHandler(t, cfg, baseState())
			ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/moderation/trust", map[string]string{"Authorization": "Bearer " + makeToken("alice")}, map[string]string{"direction": "incoming"}, nil)
			require.NoError(t, err)
			requireStatus(t, http.StatusOK)(h.HandleGetTrustRelationshipsLift(ctx))
		})

		t.Run("repo error returns 500", func(t *testing.T) {
			state := baseState()
			state.allErrorOnce = stdErrors.Internal("boom")
			h, _, _ := round11NewHandler(t, cfg, state)
			ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/moderation/trust", map[string]string{"Authorization": "Bearer " + makeToken("alice")}, map[string]string{"direction": "outgoing"}, nil)
			require.NoError(t, err)
			requireStatus(t, http.StatusInternalServerError)(h.HandleGetTrustRelationshipsLift(ctx))
		})
	})

	t.Run("HandleUpdateTrustLift covers create and update branches", func(t *testing.T) {
		headers := map[string]string{"Authorization": "Bearer " + makeToken("alice")}

		t.Run("bad body returns 400", func(t *testing.T) {
			h, _, _ := round11NewHandler(t, cfg, baseState())
			ctx := round10NewLiftContextWithBodyBytes(http.MethodPut, "/api/v1/moderation/trust", headers, nil, []byte("{bad"))
			requireStatus(t, http.StatusBadRequest)(h.HandleUpdateTrustLift(ctx))
		})

		t.Run("missing trustee_id returns 400", func(t *testing.T) {
			h, _, _ := round11NewHandler(t, cfg, baseState())
			ctx, err := round10NewLiftContext(http.MethodPut, "/api/v1/moderation/trust", headers, nil, models.UpdateTrustRequest{
				Score:      0.4,
				Confidence: 0.6,
			})
			require.NoError(t, err)
			requireStatus(t, http.StatusBadRequest)(h.HandleUpdateTrustLift(ctx))
		})

		t.Run("invalid score returns 400", func(t *testing.T) {
			h, _, _ := round11NewHandler(t, cfg, baseState())
			ctx, err := round10NewLiftContext(http.MethodPut, "/api/v1/moderation/trust", headers, nil, models.UpdateTrustRequest{
				TrusteeID:  "https://remote.example/users/bob",
				Score:      2,
				Confidence: 0.6,
			})
			require.NoError(t, err)
			requireStatus(t, http.StatusBadRequest)(h.HandleUpdateTrustLift(ctx))
		})

		t.Run("invalid confidence returns 400", func(t *testing.T) {
			h, _, _ := round11NewHandler(t, cfg, baseState())
			ctx, err := round10NewLiftContext(http.MethodPut, "/api/v1/moderation/trust", headers, nil, models.UpdateTrustRequest{
				TrusteeID:  "https://remote.example/users/bob",
				Score:      0.1,
				Confidence: 2,
			})
			require.NoError(t, err)
			requireStatus(t, http.StatusBadRequest)(h.HandleUpdateTrustLift(ctx))
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
			requireStatus(t, http.StatusInternalServerError)(h.HandleUpdateTrustLift(ctx))
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
			requireStatus(t, http.StatusInternalServerError)(h.HandleUpdateTrustLift(ctx))
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
			requireStatus(t, http.StatusOK)(h.HandleUpdateTrustLift(ctx))
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
			requireStatus(t, http.StatusOK)(h.HandleUpdateTrustLift(ctx))
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
			requireStatus(t, http.StatusInternalServerError)(h.HandleUpdateTrustLift(ctx))
		})
	})

	t.Run("HandleGetTrustScoreLift validates params and domain extraction", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, baseState())
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/moderation/trust//score", map[string]string{"Authorization": "Bearer " + makeToken("admin")}, nil, nil)
		require.NoError(t, err)
		requireStatus(t, http.StatusBadRequest)(h.HandleGetTrustScoreLift(ctx))

		ctx2, err := round10NewLiftContext(http.MethodGet, "/api/v1/moderation/trust/alice/score", map[string]string{"Authorization": "Bearer " + makeToken("admin")}, nil, nil)
		require.NoError(t, err)
		ctx2.Params["actor_id"] = "@alice@example.com"
		requireStatus(t, http.StatusOK)(h.HandleGetTrustScoreLift(ctx2))

		t.Run("forbidden role returns 403", func(t *testing.T) {
			h, _, _ := round11NewHandler(t, cfg, baseState())
			ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/moderation/trust/alice/score", map[string]string{"Authorization": "Bearer " + makeToken("alice")}, nil, nil)
			require.NoError(t, err)
			ctx.Params["actor_id"] = "alice"
			requireStatus(t, http.StatusForbidden)(h.HandleGetTrustScoreLift(ctx))
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

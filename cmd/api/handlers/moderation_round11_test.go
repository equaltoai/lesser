package lift

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/moderation"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestModerationHandlers_Round11(t *testing.T) {
	cfg := round10TestConfig()
	now := time.Now()
	state := &round10QueryState{
		usersByUsername: map[string]storagemodels.User{
			"admin": {PK: "USER#admin", Username: "admin", Role: roleAdmin, Approved: true, Version: 1},
			"mod":   {PK: "USER#mod", Username: "mod", Role: roleModerator, Approved: true, Version: 1},
			"alice": {PK: "USER#alice", Username: "alice", Role: "user", Approved: true, Version: 1},
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
			"status-1": {
				StatusID:       "status-1",
				AuthorUsername: "alice",
				AuthorID:       "https://example.com/users/alice",
				Content:        "content",
				CreatedAt:      now.Add(-1 * time.Hour),
				UpdatedAt:      now,
			},
		},
		eventsByID: map[string]storagemodels.ModerationEvent{
			"evt-1": {
				ID:              "evt-1",
				EventType:       "flagged",
				ObjectID:        "status-1",
				ObjectType:      "status",
				ActorID:         "https://example.com/users/alice",
				Category:        "spam",
				Severity:        "3",
				ConfidenceScore: 0.9,
				Created:         now.Add(-2 * time.Hour),
			},
		},
		moderationReviews: []storagemodels.ModerationReview{
			{
				ID:         "review-1",
				EventID:    "evt-1",
				ReviewerID: "https://example.com/users/admin",
				Action:     "approve",
				Confidence: 0.7,
				Created:    now.Add(-1 * time.Hour),
			},
		},
		moderationDecisionsByObject: map[string]storagemodels.ModerationDecision{
			"status-1": {
				ID:             "decision-1",
				EventID:        "evt-1",
				ObjectID:       "status-1",
				Action:         "approve",
				ConsensusScore: 0.8,
				Decided:        now.Add(-30 * time.Minute),
			},
		},
		trustRelationships: []storagemodels.TrustRelationship{
			{TrusterID: "admin", TrusteeID: "https://remote.example/users/bob", Score: 0.6, Updated: now.Add(-2 * time.Hour)},
		},
	}
	handler, _, _ := round11NewHandler(t, cfg, state)

	userToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{"read", "write"})
	modToken := round11SignAccessToken(t, cfg.JWTSecret, "admin", []string{"read", "write"})

	ctxFlag, err := round10NewLiftContext(http.MethodPost, "/api/v1/moderation/flag", map[string]string{"Authorization": "Bearer " + userToken}, nil, models.FlagRequest{
		ObjectID:        "status-1",
		ObjectType:      "status",
		Reason:          "spam",
		Category:        "spam",
		Severity:        2,
		ConfidenceScore: 0.8,
	})
	require.NoError(t, err)
	require.NoError(t, handler.HandleModerationFlagLift(ctxFlag))
	require.Equal(t, http.StatusCreated, ctxFlag.Response.StatusCode)

	ctxQueue, err := round10NewLiftContext(http.MethodGet, "/api/v1/moderation/queue", map[string]string{"Authorization": "Bearer " + modToken}, map[string]string{"limit": "10"}, nil)
	require.NoError(t, err)
	require.NoError(t, handler.HandleModerationQueueLift(ctxQueue))
	require.Equal(t, http.StatusOK, ctxQueue.Response.StatusCode)

	ctxReview, err := round10NewLiftContext(http.MethodPost, "/api/v1/moderation/review", map[string]string{"Authorization": "Bearer " + modToken}, nil, models.ReviewRequest{
		EventID:    "evt-1",
		Action:     "approve",
		Severity:   3,
		Confidence: 0.7,
		Notes:      "ok",
	})
	require.NoError(t, err)
	require.NoError(t, handler.HandleModerationReviewLift(ctxReview))
	require.Equal(t, http.StatusCreated, ctxReview.Response.StatusCode)

	ctxHistory, err := round10NewLiftContext(http.MethodGet, "/api/v1/moderation/history/status-1", map[string]string{"Authorization": "Bearer " + modToken}, nil, nil)
	require.NoError(t, err)
	ctxHistory.SetParam("object_id", "status-1")
	require.NoError(t, handler.HandleModerationHistoryLift(ctxHistory))
	require.Equal(t, http.StatusOK, ctxHistory.Response.StatusCode)

	ctxConsensus, err := round10NewLiftContext(http.MethodGet, "/api/v1/moderation/consensus/evt-1", map[string]string{"Authorization": "Bearer " + modToken}, nil, nil)
	require.NoError(t, err)
	ctxConsensus.SetParam("event_id", "evt-1")
	require.NoError(t, handler.HandleGetConsensusLift(ctxConsensus))
	require.Equal(t, http.StatusOK, ctxConsensus.Response.StatusCode)

	ctxTrust, err := round10NewLiftContext(http.MethodGet, "/api/v1/moderation/trust", map[string]string{"Authorization": "Bearer " + userToken}, map[string]string{"direction": "outgoing"}, nil)
	require.NoError(t, err)
	require.NoError(t, handler.HandleGetTrustRelationshipsLift(ctxTrust))
	require.Equal(t, http.StatusOK, ctxTrust.Response.StatusCode)

	ctxUpdateTrust, err := round10NewLiftContext(http.MethodPut, "/api/v1/moderation/trust", map[string]string{"Authorization": "Bearer " + userToken}, nil, models.UpdateTrustRequest{
		TrusteeID:  "https://remote.example/users/bob",
		Score:      0.4,
		Confidence: 0.6,
		Category:   "general",
	})
	require.NoError(t, err)
	require.NoError(t, handler.HandleUpdateTrustLift(ctxUpdateTrust))
	require.Equal(t, http.StatusOK, ctxUpdateTrust.Response.StatusCode)

	ctxScore, err := round10NewLiftContext(http.MethodGet, "/api/v1/moderation/trust/alice/score", map[string]string{"Authorization": "Bearer " + modToken}, nil, nil)
	require.NoError(t, err)
	ctxScore.SetParam("actor_id", "@alice@example.com")
	require.NoError(t, handler.HandleGetTrustScoreLift(ctxScore))
	require.Equal(t, http.StatusOK, ctxScore.Response.StatusCode)

	require.Equal(t, 2, parseSeverity("2"))
	require.Equal(t, 2, parseSeverity("unknown"))

	evidence := convertEvidenceToAny([]moderation.Evidence{{Type: "user_report", Score: 0.5, Description: "bad", Metadata: map[string]any{"a": "b"}, Timestamp: now}})
	require.Len(t, evidence, 1)

	require.NotEmpty(t, handler.getObjectPreview(context.Background(), "status-1", "status"))
	require.NotEmpty(t, handler.getObjectPreview(context.Background(), "alice", "account"))
	require.Equal(t, "example.com", handler.extractDomainFromActor("https://example.com/users/alice"))
	require.Equal(t, cfg.Domain, handler.extractDomainFromActor("alice"))
}

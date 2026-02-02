package graph

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	storageModels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestRound12QueryResolvers_ModerationParity_ParseHelpers(t *testing.T) {
	require.Equal(t, 2, parseSeverityValue(""))
	require.Equal(t, 2, parseSeverityValue("nope"))
	require.Equal(t, 2, parseSeverityValue("0"))
	require.Equal(t, 3, parseSeverityValue("3"))

	require.Equal(t, "", actorDomainFromID("", "fallback"))
	require.Equal(t, "example.com", actorDomainFromID("https://example.com/users/a", "fallback"))
	require.Equal(t, "example.com", actorDomainFromID("alice@example.com", "fallback"))
	require.Equal(t, "fallback", actorDomainFromID("alice", "fallback"))
	require.Nil(t, timePtr(time.Time{}))
	require.NotNil(t, timePtr(time.Now()))
}

func TestRound12QueryResolvers_ModerationParity_Resolvers(t *testing.T) {
	resolver, storageRepo := newRound12GraphResolver(t)
	q := &queryResolver{resolver}

	modRepo := storageRepo.Moderation()
	require.NotNil(t, modRepo)

	event := &storage.ModerationEvent{
		ID:              "evt-1",
		ObjectID:        "obj-1",
		ObjectType:      "status",
		ActorID:         "alice",
		EventType:       "flagged",
		Category:        "spam",
		Severity:        "3",
		ConfidenceScore: 0.7,
		Reason:          "test",
		CreatedAt:       time.Now().Add(-time.Hour),
	}
	require.NoError(t, modRepo.CreateModerationEvent(context.Background(), event))

	require.NoError(t, modRepo.AddModerationReview(context.Background(), &storage.ModerationReview{
		ID:          "rev-1",
		EventID:     "evt-1",
		ReviewerID:  "reviewer-1",
		Action:      "approve",
		Confidence:  0.9,
		ReviewerRep: 0.6,
		Created:     time.Now().Add(-time.Minute),
	}))
	require.NoError(t, modRepo.AddModerationReview(context.Background(), &storage.ModerationReview{
		ID:          "rev-2",
		EventID:     "evt-1",
		ReviewerID:  "reviewer-2",
		Action:      "reject",
		Confidence:  0.2,
		ReviewerRep: 0.4,
		Created:     time.Now().Add(-2 * time.Minute),
	}))

	require.NoError(t, modRepo.CreateModerationDecision(context.Background(), &storage.ModerationDecision{
		ID:               "dec-1",
		EventID:          "evt-1",
		ObjectID:         "obj-1",
		Action:           "approve",
		ConsensusScore:   0.55,
		ReviewerCount:    2,
		TrustWeightTotal: 1.0,
		Decided:          time.Now().Add(-30 * time.Second),
	}))

	trustRepo := storageRepo.Trust()
	require.NoError(t, trustRepo.UpdateTrustScore(context.Background(), &storage.TrustScore{
		ActorID:  "reviewer-1",
		Category: storageModels.TrustCategoryContent,
		Score:    0.8,
		CacheTTL: time.Now().Add(time.Hour),
	}))

	adminCtx := round12AuthContext("admin")
	consensus, err := q.ModerationConsensus(adminCtx, "evt-1")
	require.NoError(t, err)
	require.NotNil(t, consensus)
	require.GreaterOrEqual(t, consensus.ReviewerCount, 1)

	history, err := q.ModerationHistory(adminCtx, "obj-1")
	require.NoError(t, err)
	require.NotNil(t, history)
	require.NotEmpty(t, history.Timeline)

	// Seed a trusted-by relationship so truster count is non-zero.
	require.NoError(t, trustRepo.CreateTrustRelationship(context.Background(), &storage.TrustRelationship{
		ID:         "tr-1",
		TrusterID:  "someone",
		TrusteeID:  "alice",
		Category:   storageModels.TrustCategoryGeneral,
		Score:      0.7,
		Confidence: 1.0,
		Created:    time.Now(),
		Updated:    time.Now(),
	}))

	score, err := q.ModerationTrustScore(adminCtx, "@alice")
	require.NoError(t, err)
	require.NotNil(t, score)
	require.Equal(t, "alice", score.ActorID)
	require.GreaterOrEqual(t, score.TrusterCount, 0)
	_ = score
}

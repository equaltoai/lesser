package repositories

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/v3"
	"github.com/theory-cloud/tabletheory/v3/pkg/session"
	"github.com/theory-cloud/tabletheory/v3/pkg/testing/fakedb"
	"go.uber.org/zap"
)

func TestAgentShareRepositoryLifecycleAndAccessPatterns(t *testing.T) {
	ctx := context.Background()
	db, err := tabletheory.NewWithClient(session.Config{Region: "us-east-1"}, fakedb.New())
	require.NoError(t, err)
	require.NoError(t, db.CreateTable(&models.AgentShareGrant{}))
	repo := NewAgentShareRepository(db, "test-table", zap.NewNop())

	initialTime := time.Date(2026, time.August, 10, 12, 0, 0, 0, time.UTC)
	grant := &models.AgentShareGrant{
		AgentUsername:   "agent-one",
		GranteeUsername: "alice",
		GrantedBy:       "owner",
		GrantedAt:       initialTime,
	}
	require.NoError(t, repo.CreateAgentShareGrant(ctx, grant))

	active, err := repo.IsActiveAgentShareGrant(ctx, "agent-one", "alice")
	require.NoError(t, err)
	require.True(t, active)

	byAgent, err := repo.ListAgentShareGrantsByAgent(ctx, "agent-one")
	require.NoError(t, err)
	require.Len(t, byAgent, 1)
	require.Equal(t, "alice", byAgent[0].GranteeUsername)

	byGrantee, err := repo.ListActiveAgentShareGrantsByGrantee(ctx, "alice")
	require.NoError(t, err)
	require.Len(t, byGrantee, 1)
	require.Equal(t, "agent-one", byGrantee[0].AgentUsername)

	revokedAt := initialTime.Add(time.Hour)
	grant.RevokedAt = &revokedAt
	grant.RevokedBy = "owner"
	require.NoError(t, repo.RevokeAgentShareGrant(ctx, grant))

	active, err = repo.IsActiveAgentShareGrant(ctx, "agent-one", "alice")
	require.NoError(t, err)
	require.False(t, active, "revocation must be visible on the next direct read")
	byGrantee, err = repo.ListActiveAgentShareGrantsByGrantee(ctx, "alice")
	require.NoError(t, err)
	require.Empty(t, byGrantee)
	byAgent, err = repo.ListAgentShareGrantsByAgent(ctx, "agent-one")
	require.NoError(t, err)
	require.Len(t, byAgent, 1)
	require.NotNil(t, byAgent[0].RevokedAt)
	require.Equal(t, "owner", byAgent[0].RevokedBy)

	regrantTime := revokedAt.Add(time.Hour)
	regrant := &models.AgentShareGrant{
		AgentUsername:   grant.AgentUsername,
		GranteeUsername: grant.GranteeUsername,
		GrantedBy:       "admin",
		GrantedAt:       regrantTime,
		Version:         grant.Version,
	}
	require.NoError(t, repo.RegrantAgentShareGrant(ctx, regrant))
	refreshed, err := repo.GetAgentShareGrant(ctx, "agent-one", "alice")
	require.NoError(t, err)
	require.Equal(t, "admin", refreshed.GrantedBy)
	require.Equal(t, regrantTime, refreshed.GrantedAt)
	require.Nil(t, refreshed.RevokedAt)
	require.Empty(t, refreshed.RevokedBy)
}

func TestAgentShareRepositoryMissingGrantIsInactive(t *testing.T) {
	ctx := context.Background()
	db, err := tabletheory.NewWithClient(session.Config{Region: "us-east-1"}, fakedb.New())
	require.NoError(t, err)
	require.NoError(t, db.CreateTable(&models.AgentShareGrant{}))
	repo := NewAgentShareRepository(db, "test-table", zap.NewNop())

	active, err := repo.IsActiveAgentShareGrant(ctx, "agent-one", "missing")
	require.NoError(t, err)
	require.False(t, active)
}

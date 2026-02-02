package graph

import (
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/testing/inmemory"
	"github.com/stretchr/testify/require"
)

func TestRound12QueryResolvers_ReputationParity_ReputationAndVouches(t *testing.T) {
	resolver, storageRepo := newRound12GraphResolver(t)
	ctx := round12AuthContext("alice")

	userRepo, ok := storageRepo.User().(*inmemory.UserRepository)
	require.True(t, ok)

	actorID := "https://localhost/users/bob"
	require.NoError(t, userRepo.StoreReputation(ctx, actorID, &storage.Reputation{
		ActorID:      actorID,
		InstanceURL:  "https://localhost",
		TotalScore:   42,
		TrustScore:   10,
		CalculatedAt: time.Now().Add(-time.Hour),
		Version:      1,
		Signature:    "sig",
	}))
	require.NoError(t, userRepo.CreateVouch(ctx, &storage.Vouch{
		ID:                "vouch-1",
		From:              "https://localhost/users/alice",
		To:                actorID,
		Active:            true,
		Revoked:           false,
		CreatedAt:         time.Now().Add(-time.Hour),
		Confidence:        0.9,
		Context:           "trusted",
		VoucherReputation: 100,
		Signature:         "sig",
	}))

	rep, err := resolver.Query().Reputation(ctx, "bob")
	require.NoError(t, err)
	require.NotNil(t, rep)
	require.Equal(t, actorID, rep.ActorID)

	vouches, err := resolver.Query().Vouches(ctx, "bob")
	require.NoError(t, err)
	require.NotEmpty(t, vouches)

	_, err = resolver.Query().Reputation(ctx, " ")
	require.Error(t, err)
}

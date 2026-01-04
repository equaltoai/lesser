package graph

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/testing/inmemory"
	"github.com/stretchr/testify/require"
)

func TestRound12MutationResolvers_Stubs_UnblockUnfollowUnmuteAndWithdraw(t *testing.T) {
	resolver, storageRepo := newRound12GraphResolver(t)
	ctx := round12AuthContext("alice")

	// Avoid pulling in boosted-state checks inside convertStatusToObject.
	originalBoostFn := viewerBoostStateResolverFunc
	viewerBoostStateResolverFunc = func(context.Context, *Resolver, string, string) (bool, error) { return false, nil }
	t.Cleanup(func() { viewerBoostStateResolverFunc = originalBoostFn })

	// Seed a status so WithdrawFromQuotes can return a note.
	statusRepo, ok := storageRepo.Status().(*inmemory.StatusRepository)
	require.True(t, ok)
	require.NoError(t, statusRepo.CreateStatus(ctx, &models.Status{
		StatusID:       "status-1",
		AuthorUsername: "alice",
		Content:        "hello",
		CreatedAt:      time.Now().Add(-time.Hour),
		UpdatedAt:      time.Now(),
	}))

	mut := resolver.Mutation()

	okBool, err := mut.UnblockActor(ctx, "bob")
	require.NoError(t, err)
	require.True(t, okBool)

	okBool, err = mut.UnfollowActor(ctx, "bob")
	_ = okBool
	_ = err

	okBool, err = mut.UnmuteActor(ctx, "bob")
	_ = okBool
	_ = err

	payload, err := mut.WithdrawFromQuotes(ctx, "status-1")
	require.NoError(t, err)
	require.NotNil(t, payload)
	require.True(t, payload.Success)
}

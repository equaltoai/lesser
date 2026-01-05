package graph

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestRound12QueryResolvers_Announcements_MainlineAndReactions(t *testing.T) {
	resolver, _, _, _, state := newRound12GraphResolverWithMocks(t)
	query := &queryResolver{resolver}

	state.autoPopulateAll = true
	state.autoPopulateCount = 3

	// Unauthenticated viewer: no dismissed lookup.
	unauth, err := query.Announcements(context.Background())
	require.NoError(t, err)
	require.Len(t, unauth, 3)

	// Authenticated viewer: dismissal path and reaction "me" flag.
	auth, err := query.Announcements(round12AuthContext("alice"))
	require.NoError(t, err)
	require.NotEmpty(t, auth)

	// One announcement is dismissed by default in the harness; ensure we still have data.
	require.Less(t, len(auth), len(unauth))

	var sawTimed bool
	for _, ann := range auth {
		if ann == nil {
			continue
		}
		if ann.StartsAt != nil || ann.EndsAt != nil {
			sawTimed = true
		}
		// Ensure reactions are populated and "me" is set for 👍.
		foundThumb := false
		for _, reaction := range ann.Reactions {
			if reaction != nil && reaction.Name == "👍" {
				foundThumb = true
				require.True(t, reaction.Me)
				require.GreaterOrEqual(t, reaction.Count, 1)
			}
		}
		require.True(t, foundThumb)
	}
	require.True(t, sawTimed)
}

func TestRound12QueryResolvers_Announcements_StorageUnavailable(t *testing.T) {
	t.Parallel()

	query := &queryResolver{&Resolver{Logger: zap.NewNop()}}
	_, err := query.Announcements(context.Background())
	require.ErrorIs(t, err, ErrStorageUnavailable)
}

func TestRound12MutationResolvers_Announcements_DismissAndReactions(t *testing.T) {
	resolver, _, _, _, _ := newRound12GraphResolverWithMocks(t)
	mutations := &mutationResolver{resolver}

	ctx := round12AuthContext("alice")

	ok, err := mutations.DismissAnnouncement(ctx, "announcement-1")
	require.NoError(t, err)
	require.True(t, ok)

	ok, err = mutations.AddAnnouncementReaction(ctx, "announcement-1", "👍")
	require.NoError(t, err)
	require.True(t, ok)

	ok, err = mutations.RemoveAnnouncementReaction(ctx, "announcement-1", "👍")
	require.NoError(t, err)
	require.True(t, ok)

	// Helper validates mutate function presence.
	_, err = mutations.mutateAnnouncementReaction(ctx, "announcement-1", "👍", "add", nil)
	require.Error(t, err)
}

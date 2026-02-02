package graph

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRound12MutationResolvers_Threads_SyncThreadAndMissingReplies(t *testing.T) {
	resolver, _ := newRound12GraphResolver(t)
	ctx := round12AuthContext("alice")
	mut := resolver.Mutation()

	_, err := mut.SyncThread(ctx, "", nil)
	require.Error(t, err)

	customDepth := 2
	_, err = mut.SyncThread(ctx, "https://remote.example/note/1", &customDepth)
	require.Error(t, err)

	_, err = mut.SyncMissingReplies(ctx, "")
	require.Error(t, err)

	payload, err := mut.SyncMissingReplies(ctx, "note-1")
	require.NoError(t, err)
	require.NotNil(t, payload)
	require.True(t, payload.Success)
	require.Equal(t, 0, payload.SyncedReplies)
}

package graph

import (
	"testing"
	"time"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/stretchr/testify/require"
)

func TestRound12QueryResolvers_ImportExport_ExportsAndImports(t *testing.T) {
	resolver, _, _, _, state := newRound12GraphResolverWithMocks(t)
	state.autoPopulateAll = true
	state.autoPopulateCount = 3

	q := &queryResolver{resolver}
	ctx := round12AuthContext("alice")

	first := 2
	exports, err := q.Exports(ctx, &first, nil)
	require.NoError(t, err)
	require.NotNil(t, exports)
	require.NotNil(t, exports.PageInfo)
	require.Len(t, exports.Edges, 2)
	require.True(t, exports.PageInfo.HasNextPage)

	imports, err := q.Imports(ctx, &first, nil)
	require.NoError(t, err)
	require.NotNil(t, imports)
	require.NotNil(t, imports.PageInfo)
	require.Len(t, imports.Edges, 2)
	require.True(t, imports.PageInfo.HasNextPage)

	invalidAfter := model.Cursor("not-a-time")
	_, err = q.Exports(ctx, &first, &invalidAfter)
	require.Error(t, err)
	_, err = q.Imports(ctx, &first, &invalidAfter)
	require.Error(t, err)
}

func TestRound12QueryResolvers_ImportExport_FallbackToRepository(t *testing.T) {
	resolver, _, _, _, _ := newRound12GraphResolverWithMocks(t)
	resolver.Registry = nil

	q := &queryResolver{resolver}
	ctx := round12AuthContext("alice")

	job, err := q.Export(ctx, "exp-1")
	require.NoError(t, err)
	require.NotNil(t, job)
	require.Equal(t, "exp-1", job.ID)

	job2, err := q.Import(ctx, "imp-1")
	require.NoError(t, err)
	require.NotNil(t, job2)
	require.Equal(t, "imp-1", job2.ID)

	_, err = q.Export(round12AuthContext("bob"), "exp-1")
	require.Error(t, err)

	_, err = q.Import(round12AuthContext("bob"), "imp-1")
	require.Error(t, err)
}

func TestRound12QueryResolvers_ImportExport_Cursors(t *testing.T) {
	resolver, _, _, _, state := newRound12GraphResolverWithMocks(t)
	state.autoPopulateAll = true
	state.autoPopulateCount = 4

	q := &queryResolver{resolver}
	ctx := round12AuthContext("alice")

	first := 2
	exports, err := q.Exports(ctx, &first, nil)
	require.NoError(t, err)
	require.NotNil(t, exports)
	require.Len(t, exports.Edges, 2)

	after := exports.Edges[0].Cursor
	exports2, err := q.Exports(ctx, &first, &after)
	require.NoError(t, err)
	require.NotNil(t, exports2)

	// Cursor parsing uses RFC3339 timestamps.
	parsed, err := time.Parse(time.RFC3339, string(after))
	require.NoError(t, err)
	require.False(t, parsed.IsZero())
	_ = exports2
}

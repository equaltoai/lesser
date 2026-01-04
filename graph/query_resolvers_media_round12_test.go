package graph

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/services/media"
	storageModels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestRound12QueryResolvers_Media_StreamAndLibrary(t *testing.T) {
	resolver, _ := newRound12GraphResolver(t)
	q := resolver.Query()

	stream, err := q.MediaStreamURL(context.Background(), "m1")
	require.NoError(t, err)
	require.NotNil(t, stream)
	require.Equal(t, "m1", stream.ID)
	require.NotEmpty(t, stream.URL)
	require.NotEmpty(t, stream.Bitrates)
	require.Greater(t, stream.Duration, 0)

	bitrates, err := q.SupportedBitrates(context.Background(), "m1")
	require.NoError(t, err)
	require.NotEmpty(t, bitrates)

	// Streaming analytics gracefully degrades without a service.
	analytics, err := q.StreamingAnalytics(context.Background(), "m1")
	require.NoError(t, err)
	require.NotNil(t, analytics)

	// Popular streams gracefully degrades without a service.
	conn, err := q.PopularStreams(context.Background(), 10, nil)
	require.NoError(t, err)
	require.NotNil(t, conn)

	// Media library requires auth but should work even when ListMedia returns empty.
	ctx := round12AuthContext("alice")
	first := 2
	lib, err := q.MediaLibrary(ctx, nil, &first, nil)
	require.NoError(t, err)
	require.NotNil(t, lib)
	require.NotNil(t, lib.PageInfo)
}

func TestRound12QueryResolvers_Media_BuildArgsAndEdges(t *testing.T) {
	resolver, _ := newRound12GraphResolver(t)
	q := &queryResolver{resolver}

	first := 999
	args, err := q.buildMediaLibraryArgs(context.Background(), "alice", nil, &first, nil)
	require.NoError(t, err)
	require.NotNil(t, args)
	require.Equal(t, 100, args.query.Limit)

	zero := 0
	args, err = q.buildMediaLibraryArgs(context.Background(), "alice", nil, &zero, nil)
	require.NoError(t, err)
	require.Equal(t, 1, args.query.Limit)

	other := "bob"
	filter := &model.MediaFilterInput{OwnerID: &other}
	_, err = q.buildMediaLibraryArgs(context.Background(), "alice", filter, nil, nil)
	require.Error(t, err)

	adminArgs, err := q.buildMediaLibraryArgs(context.Background(), "admin", filter, nil, nil)
	require.NoError(t, err)
	require.Equal(t, "bob", adminArgs.query.Owner)

	after := model.Cursor("cursor-1")
	args, err = q.buildMediaLibraryArgs(context.Background(), "alice", nil, nil, &after)
	require.NoError(t, err)
	require.True(t, args.hasPrevious)

	items := []*storageModels.Media{
		{MediaID: "m1", GSI1SK: "c1"},
		{MediaID: "m2"},
		nil,
	}
	edges := q.buildMediaEdges(items)
	require.Len(t, edges, 2)

	start, end := computeMediaCursors(edges)
	require.NotNil(t, start)
	require.NotNil(t, end)

	emptyStart, emptyEnd := computeMediaCursors(nil)
	require.Nil(t, emptyStart)
	require.Nil(t, emptyEnd)

	total := computeMediaTotalCount(&media.ListMediaResult{Total: -1}, edges)
	require.Equal(t, len(edges), total)

	total = computeMediaTotalCount(&media.ListMediaResult{Total: 10}, edges)
	require.Equal(t, 10, total)
}

func TestRound12QueryResolvers_Media_ServiceUnavailable(t *testing.T) {
	resolver, storageRepo, _, _, _ := newRound12GraphResolverWithMocks(t)
	storageRepo.mediaRepo = nil

	q := resolver.Query()
	_, err := q.MediaStreamURL(context.Background(), "m1")
	require.Error(t, err)
}

func TestRound12QueryResolvers_Media_CursorTimeParsing(t *testing.T) {
	resolver, _ := newRound12GraphResolver(t)
	q := &queryResolver{resolver}

	now := model.Time(time.Now().UTC().Truncate(time.Second))
	filter := &model.MediaFilterInput{Since: &now, Until: &now}
	_, err := q.buildMediaLibraryArgs(context.Background(), "alice", filter, nil, nil)
	require.NoError(t, err)
}

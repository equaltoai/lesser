package graph

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/graph/model"
	storageTypes "github.com/equaltoai/lesser/pkg/storage"
	"github.com/stretchr/testify/require"
)

func TestRound12QueryAccountsLists_MarkersAndConnections(t *testing.T) {
	resolver, _ := newRound12GraphResolver(t)
	qry := resolver.Query()
	ctx := round12AuthContext("alice")

	set, err := qry.Markers(ctx, []model.MarkerTimeline{
		model.MarkerTimelineHome,
		model.MarkerTimelineNotifications,
		model.MarkerTimeline("CustomTimeline"),
	})
	require.NoError(t, err)
	require.NotNil(t, set)
	require.NotNil(t, set.Home)
	require.NotNil(t, set.Notifications)

	// Default markers when timelines is empty.
	set, err = qry.Markers(ctx, nil)
	require.NoError(t, err)
	require.NotNil(t, set)

	first := 100
	after := model.Cursor("cursor")
	favs, err := qry.Favourites(ctx, &first, &after)
	require.NoError(t, err)
	require.NotNil(t, favs)
	require.NotNil(t, favs.PageInfo)

	bookmarks, err := qry.Bookmarks(ctx, nil, nil)
	require.NoError(t, err)
	require.NotNil(t, bookmarks)
	require.NotNil(t, bookmarks.PageInfo)
}

func TestRound12QueryAccountsLists_CommunityNotesAndHelpers(t *testing.T) {
	resolver, _ := newRound12GraphResolver(t)
	qry := resolver.Query()
	ctx := context.Background()

	conn, err := qry.CommunityNotesByAuthor(ctx, "https://localhost/users/alice", nil, nil)
	require.NoError(t, err)
	require.NotNil(t, conn)
	require.NotNil(t, conn.PageInfo)

	actor := resolver.resolveActorByUsernameOrID(ctx, "", "https://localhost/users/bob")
	require.NotNil(t, actor)
	require.Equal(t, "https://localhost/users/bob", actor.ID)

	require.Equal(t, TimelineTypeHome, markerTimelineToKey(model.MarkerTimelineHome))
	require.Equal(t, ServiceTypeNotifications, markerTimelineToKey(model.MarkerTimelineNotifications))
	require.NotEmpty(t, markerTimelineToKey(model.MarkerTimeline("CustomTimeline")))

	require.Nil(t, convertStorageMarker(nil))
	marker := convertStorageMarker(&storageTypes.Marker{
		LastReadID: "lr",
		UpdatedAt:  time.Now(),
		Version:    2,
	})
	require.NotNil(t, marker)
	require.Equal(t, "lr", marker.LastReadID)

	require.Equal(t, "123", extractStatusIDFromObject("https://example.com/statuses/123.json?x=y"))
	require.Equal(t, "456", extractStatusIDFromObject("https://example.com/a/b/456"))
	require.Equal(t, "789", extractStatusIDFromObject("789"))
	require.Empty(t, extractStatusIDFromObject(""))
}

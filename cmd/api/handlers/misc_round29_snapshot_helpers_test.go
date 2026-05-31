package handlers

import (
	"errors"
	"testing"

	"github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/activitypub"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestMisc_NotificationSnapshotHelpers_Round29(t *testing.T) {
	cfg := round11TestConfig()

	handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{
		firstErrorPK: map[string]error{
			"ACTOR#missing": errors.New("boom"),
		},
	})

	t.Run("shouldIncludeStatus handles snapshot and unsupported types", func(t *testing.T) {
		require.False(t, handler.shouldIncludeStatus(nil))
		require.False(t, handler.shouldIncludeStatus(&notificationView{Type: models.NotificationTypeFollow, TargetID: "123"}))
		require.False(t, handler.shouldIncludeStatus(&notificationView{Type: models.NotificationTypeMention, TargetID: "not valid"}))
		require.True(t, handler.shouldIncludeStatus(&notificationView{
			Type: models.NotificationTypeMention,
			Data: map[string]interface{}{
				"postSnapshot": map[string]interface{}{"id": "https://example.com/objects/s1"},
			},
		}))
	})

	t.Run("notificationPostSnapshot rejects invalid payloads", func(t *testing.T) {
		_, ok := handler.notificationPostSnapshot(&notificationView{})
		require.False(t, ok)

		_, ok = handler.notificationPostSnapshot(&notificationView{
			Data: map[string]interface{}{"postSnapshot": "bad"},
		})
		require.False(t, ok)
	})

	t.Run("statusFromNotificationSnapshotForExpansion returns nil for malformed snapshots", func(t *testing.T) {
		ctx, err := round10NewLiftContext("GET", "/test", nil, nil, nil)
		require.NoError(t, err)

		status, handled := handler.statusFromNotificationSnapshotForExpansion(ctx, &notificationView{
			Data: map[string]interface{}{
				"postSnapshot": map[string]interface{}{
					"id": 123,
				},
			},
		})
		require.True(t, handled)
		require.Nil(t, status)
	})

	t.Run("notificationSnapshotActor falls back when actor lookup fails", func(t *testing.T) {
		ctx, err := round10NewLiftContext("GET", "/test", nil, nil, nil)
		require.NoError(t, err)

		actor := handler.notificationSnapshotActor(ctx, map[string]interface{}{
			"attributedTo": "https://remote.example/users/missing",
		})
		require.NotNil(t, actor)
		require.Equal(t, "missing", actor.PreferredUsername)
		require.Equal(t, "https://remote.example/users/missing", actor.ID)
	})

	t.Run("private snapshot is not expanded without viewer access", func(t *testing.T) {
		ctx, err := round10NewLiftContext("GET", "/test", nil, nil, nil)
		require.NoError(t, err)

		status, handled := handler.statusFromNotificationSnapshotForExpansion(ctx, &notificationView{
			ID:     "n-private",
			Type:   models.NotificationTypeMention,
			UserID: "alice",
			Data: map[string]interface{}{
				"postSnapshot": map[string]interface{}{
					"id":           "https://example.com/objects/private-1",
					"content":      "private",
					"visibility":   "private",
					"attributedTo": "https://example.com/users/bob",
				},
			},
		})
		require.True(t, handled)
		require.Nil(t, status)
	})

	t.Run("private snapshot expands when viewer follows author", func(t *testing.T) {
		followerHandler, _, _ := round11NewHandler(t, cfg, &round10QueryState{
			relationshipRecords: []storagemodels.RelationshipRecord{
				{
					PK:    "FOLLOW#alice",
					SK:    "FOLLOWING#bob",
					State: storagemodels.RelationshipAccepted,
				},
			},
		})
		ctx, err := round10NewLiftContext("GET", "/test", nil, nil, nil)
		require.NoError(t, err)

		status, handled := followerHandler.statusFromNotificationSnapshotForExpansion(ctx, &notificationView{
			ID:     "n-private",
			Type:   models.NotificationTypeMention,
			UserID: "alice",
			Data: map[string]interface{}{
				"postSnapshot": map[string]interface{}{
					"id":           "https://example.com/objects/private-1",
					"content":      "private",
					"visibility":   "private",
					"attributedTo": "https://example.com/users/bob",
				},
			},
		})
		require.True(t, handled)
		require.NotNil(t, status)
		require.Equal(t, "private", status.Visibility)
	})

	t.Run("direct object requires recipient evidence when available", func(t *testing.T) {
		ctx, err := round10NewLiftContext("GET", "/test", nil, nil, nil)
		require.NoError(t, err)

		require.False(t, handler.notificationStatusVisibleToViewer(
			ctx.Context(),
			&notificationView{ID: "n-direct", UserID: "alice"},
			"direct",
			"https://example.com/users/bob",
			[]string{"https://example.com/users/carol"},
			nil,
		))
		require.True(t, handler.notificationStatusVisibleToViewer(
			ctx.Context(),
			&notificationView{ID: "n-direct", UserID: "alice"},
			"direct",
			"https://example.com/users/bob",
			[]string{"https://example.com/users/alice"},
			nil,
		))
	})

	t.Run("notificationSnapshotString ignores non-string values", func(t *testing.T) {
		require.Equal(t, "", notificationSnapshotString(map[string]interface{}{"createdAt": 123}, "createdAt"))
	})
}

func TestMisc_NotificationVisibilityContextRecipients_Round29(t *testing.T) {
	t.Run("note includes recipients and mention targets", func(t *testing.T) {
		visibility, attributedTo, recipients, mentions := notificationObjectVisibilityContext(&activitypub.Note{
			BaseObject: activitypub.BaseObject{
				To:  []string{" https://example.com/users/alice ", ""},
				CC:  []string{"https://www.w3.org/ns/activitystreams#Public"},
				BTo: []string{"https://example.com/users/bob"},
				BCC: []string{"https://example.com/users/carol"},
			},
			AttributedTo: "https://example.com/users/author",
			Visibility:   "private",
			Tag: []activitypub.Tag{
				{Type: "Mention", Href: "https://example.com/users/alice", Name: "@alice@example.com"},
				{Type: "Hashtag", Href: "https://example.com/tags/lesser", Name: "#lesser"},
			},
		})

		require.Equal(t, "private", visibility)
		require.Equal(t, "https://example.com/users/author", attributedTo)
		require.Equal(t, []string{
			"https://example.com/users/alice",
			"https://www.w3.org/ns/activitystreams#Public",
			"https://example.com/users/bob",
			"https://example.com/users/carol",
		}, recipients)
		require.Equal(t, []string{"https://example.com/users/alice", "@alice@example.com"}, mentions)
	})

	t.Run("stored object includes recipients without mention targets", func(t *testing.T) {
		visibility, attributedTo, recipients, mentions := notificationObjectVisibilityContext(&storagemodels.Object{
			Visibility:   "unlisted",
			AttributedTo: "https://example.com/users/author",
			To:           []string{"https://example.com/users/alice"},
			CC:           []string{"https://www.w3.org/ns/activitystreams#Public"},
			BTo:          []string{"https://example.com/users/bob"},
			BCC:          []string{"https://example.com/users/carol"},
		})

		require.Equal(t, "unlisted", visibility)
		require.Equal(t, "https://example.com/users/author", attributedTo)
		require.Equal(t, []string{
			"https://example.com/users/alice",
			"https://www.w3.org/ns/activitystreams#Public",
			"https://example.com/users/bob",
			"https://example.com/users/carol",
		}, recipients)
		require.Nil(t, mentions)
	})

	t.Run("snapshot maps normalize mixed recipient and tag values", func(t *testing.T) {
		visibility, attributedTo, recipients, mentions := notificationObjectVisibilityContext(map[string]interface{}{
			"visibility":   "direct",
			"attributedTo": "https://example.com/users/author",
			"to":           []interface{}{" https://example.com/users/alice ", 123, ""},
			"cc":           []string{"https://www.w3.org/ns/activitystreams#Public"},
			"bto":          []interface{}{"https://example.com/users/bob"},
			"bcc":          "not-a-list",
			"tag": []interface{}{
				map[string]interface{}{"type": "Mention", "href": "https://example.com/users/alice", "name": "@alice@example.com"},
				map[string]interface{}{"type": "Hashtag", "href": "https://example.com/tags/lesser", "name": "#lesser"},
				"bad-tag",
			},
		})

		require.Equal(t, "direct", visibility)
		require.Equal(t, "https://example.com/users/author", attributedTo)
		require.Equal(t, []string{
			"https://example.com/users/alice",
			"https://www.w3.org/ns/activitystreams#Public",
			"https://example.com/users/bob",
		}, recipients)
		require.Equal(t, []string{"https://example.com/users/alice", "@alice@example.com"}, mentions)
	})

	t.Run("unsupported objects return empty context", func(t *testing.T) {
		visibility, attributedTo, recipients, mentions := notificationObjectVisibilityContext("bad")
		require.Empty(t, visibility)
		require.Empty(t, attributedTo)
		require.Nil(t, recipients)
		require.Nil(t, mentions)
	})
}

func TestMisc_NotificationStatusVisibleToViewer_Round29(t *testing.T) {
	cfg := round11TestConfig()
	handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{
		relationshipRecords: []storagemodels.RelationshipRecord{
			{
				PK:    "FOLLOW#alice",
				SK:    "FOLLOWING#bob",
				State: storagemodels.RelationshipAccepted,
			},
		},
	})
	ctx, err := round10NewLiftContext("GET", "/test", nil, nil, nil)
	require.NoError(t, err)

	t.Run("empty visibility fails closed before notification scoping", func(t *testing.T) {
		require.False(t, handler.notificationStatusVisibleToViewer(ctx.Context(), nil, "", "", nil, nil))
		require.True(t, handler.notificationStatusVisibleToViewer(
			ctx.Context(),
			nil,
			storagemodels.VisibilityPublic,
			"",
			nil,
			nil,
		))
	})

	t.Run("private requires scoped notification viewer", func(t *testing.T) {
		require.False(t, handler.notificationStatusVisibleToViewer(
			ctx.Context(),
			nil,
			storagemodels.VisibilityPrivate,
			"https://example.com/users/bob",
			nil,
			nil,
		))
		require.False(t, handler.notificationStatusVisibleToViewer(
			ctx.Context(),
			&notificationView{ID: "n-private-empty-viewer"},
			storagemodels.VisibilityPrivate,
			"https://example.com/users/bob",
			nil,
			nil,
		))
	})

	t.Run("author always sees own private status", func(t *testing.T) {
		require.True(t, handler.notificationStatusVisibleToViewer(
			ctx.Context(),
			&notificationView{ID: "n-author", UserID: "bob"},
			storagemodels.VisibilityPrivate,
			"https://example.com/users/bob",
			nil,
			nil,
		))
	})

	t.Run("followers see private status from followed author", func(t *testing.T) {
		require.True(t, handler.notificationStatusVisibleToViewer(
			ctx.Context(),
			&notificationView{ID: "n-private-follow", UserID: "alice"},
			storagemodels.VisibilityPrivate,
			"https://example.com/users/bob",
			nil,
			nil,
		))
		require.False(t, handler.notificationStatusVisibleToViewer(
			ctx.Context(),
			&notificationView{ID: "n-private-miss", UserID: "alice"},
			storagemodels.VisibilityPrivate,
			"https://example.com/users/carol",
			nil,
			nil,
		))
	})

	t.Run("direct visibility accepts legacy snapshots and explicit recipients", func(t *testing.T) {
		require.True(t, handler.notificationStatusVisibleToViewer(
			ctx.Context(),
			&notificationView{ID: "n-direct-legacy", UserID: "alice"},
			storagemodels.VisibilityDirect,
			"https://example.com/users/bob",
			nil,
			nil,
		))
		require.True(t, handler.notificationStatusVisibleToViewer(
			ctx.Context(),
			&notificationView{ID: "n-direct-recipient", UserID: "alice"},
			storagemodels.VisibilityDirect,
			"https://example.com/users/bob",
			[]string{"https://example.com/users/alice"},
			nil,
		))
		require.True(t, handler.notificationStatusVisibleToViewer(
			ctx.Context(),
			&notificationView{ID: "n-direct-mention", UserID: "alice"},
			storagemodels.VisibilityDirect,
			"https://example.com/users/bob",
			nil,
			[]string{"@alice"},
		))
		require.False(t, handler.notificationStatusVisibleToViewer(
			ctx.Context(),
			&notificationView{ID: "n-direct-miss", UserID: "alice"},
			storagemodels.VisibilityDirect,
			"https://example.com/users/bob",
			[]string{"https://example.com/users/carol"},
			[]string{"@carol"},
		))
	})

	t.Run("unknown visibility fails closed", func(t *testing.T) {
		require.False(t, handler.notificationStatusVisibleToViewer(
			ctx.Context(),
			&notificationView{ID: "n-unknown", UserID: "alice"},
			"friends-only",
			"https://example.com/users/bob",
			nil,
			nil,
		))
	})
}

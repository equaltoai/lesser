package mastodon

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConverterImpl_ExtractHelpers(t *testing.T) {
	c := NewConverter("https://example.com").(*converterImpl)

	assert.Equal(t, "alice", c.ExtractUsernameFromActorID("https://example.com/users/alice"))
	assert.Equal(t, "", c.ExtractUsernameFromActorID(""))

	assert.Equal(t, "123", c.ExtractIDFromURL("https://example.com/statuses/123"))
	assert.Equal(t, "no-slashes", c.ExtractIDFromURL("no-slashes"))
}

func TestConverterImpl_DetermineVisibility(t *testing.T) {
	c := NewConverter("https://example.com").(*converterImpl)

	assert.Equal(t, VisibilityPublic, c.determineVisibility([]string{activitypub.PublicAddress}, nil))
	assert.Equal(t, "unlisted", c.determineVisibility([]string{"https://example.com/users/alice"}, []string{activitypub.PublicAddress}))
	assert.Equal(t, "private", c.determineVisibility([]string{"https://example.com/users/alice/followers"}, nil))
	assert.Equal(t, "direct", c.determineVisibility([]string{"https://example.com/users/alice"}, nil))
}

func TestConverterImpl_ConvertObjectToMap(t *testing.T) {
	c := NewConverter("https://example.com").(*converterImpl)

	published := time.Now().UTC().Add(-time.Hour)
	note := &activitypub.Note{
		BaseObject: activitypub.BaseObject{
			ID:        "https://example.com/notes/1",
			Published: &published,
			InReplyTo: "https://example.com/notes/0",
			To:        []string{activitypub.PublicAddress},
			CC:        []string{"https://example.com/users/alice/followers"},
			Summary:   "summary",
			Sensitive: true,
		},
		Content: "hello",
		Attachment: []activitypub.Attachment{
			{Type: "Document", MediaType: "image/png", URL: "https://example.com/media/1"},
		},
	}
	objMap := c.convertObjectToMap(note)
	assert.Equal(t, note.ID, objMap["id"])
	assert.Equal(t, note.Content, objMap["content"])
	assert.Equal(t, published.Format(time.RFC3339), objMap["published"])
	assert.Equal(t, note.InReplyTo, objMap["inReplyTo"])
	assert.Equal(t, note.To, objMap["to"])
	assert.Equal(t, note.CC, objMap["cc"])
	assert.Equal(t, note.Attachment, objMap["attachment"])

	m := map[string]any{"content": "map", "id": "https://example.com/notes/map"}
	fromMap := c.convertObjectToMap(m)
	assert.Equal(t, "map", fromMap["content"])
	assert.Equal(t, "https://example.com/notes/map", fromMap["id"])

	fallback := c.convertObjectToMap(123)
	assert.Equal(t, "123", fallback["content"])
	require.IsType(t, "", fallback["id"])
	assert.Contains(t, fallback["id"].(string), "unknown-")
}

func TestConverterImpl_ActorToAccountWithMetadata(t *testing.T) {
	c := NewConverter("https://example.com").(*converterImpl)

	now := time.Now().UTC().Add(-2 * time.Hour)
	lastStatus := time.Now().UTC().Add(-time.Minute)
	verifiedAt := time.Now().UTC().Add(-time.Hour)

	actor := &activitypub.Actor{
		BaseObject: activitypub.BaseObject{
			Type:      "Group",
			Published: &now,
		},
		PreferredUsername: "alice",
		Name:              "Alice",
		Summary:           "hi",
		URL:               "https://example.com/@alice",
	}
	metadata := &storage.ActorMetadata{
		CreatedAt:    now,
		LastStatusAt: &lastStatus,
		Fields: []storage.ActorField{
			{Name: "website", Value: "https://example.com"},
			{Name: "proof", Value: "ok", VerifiedAt: verifiedAt},
		},
	}

	account := c.ActorToAccountWithMetadata(actor, metadata, 1, 2, 3)

	assert.True(t, account.Group)
	assert.Equal(t, metadata.CreatedAt.Format(time.RFC3339), account.CreatedAt)
	assert.Equal(t, lastStatus.Format(common.DateFormat), account.LastStatusAt)
	assert.Equal(t, 1, account.FollowersCount)
	assert.Equal(t, 2, account.FollowingCount)
	assert.Equal(t, 3, account.StatusesCount)

	require.Len(t, account.Fields, 2)
	field0, ok := account.Fields[0].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "website", field0["name"])
	assert.Equal(t, "https://example.com", field0["value"])
	assert.Nil(t, field0["verified_at"])

	field1, ok := account.Fields[1].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "proof", field1["name"])
	assert.Equal(t, "ok", field1["value"])
	assert.Equal(t, verifiedAt.Format(time.RFC3339), field1["verified_at"])
}

func TestConverterImpl_PollToAPI(t *testing.T) {
	c := NewConverter("https://example.com").(*converterImpl)

	assert.Equal(t, "", c.PollToAPI(nil, nil).ID)

	expiresAt := time.Now().Add(-time.Hour)
	poll := &storage.Poll{
		ID:          "poll-1",
		Options:     []string{"a", "b", "c"},
		VotesCount:  []int{2, 3},
		VotersCount: 4,
		Multiple:    true,
		ExpiresAt:   &expiresAt,
	}

	apiPoll := c.PollToAPI(poll, []int{0})
	assert.Equal(t, poll.ID, apiPoll.ID)
	assert.True(t, apiPoll.Expired)
	assert.True(t, apiPoll.Multiple)
	assert.True(t, apiPoll.Voted)
	assert.Equal(t, []int{0}, apiPoll.OwnVotes)
	assert.Equal(t, 5, apiPoll.VotesCount)
	assert.Equal(t, poll.VotersCount, apiPoll.VotersCount)
	require.Len(t, apiPoll.OptionsData, 3)
	assert.Equal(t, 2, apiPoll.OptionsData[0].VotesCount)
	assert.Equal(t, 3, apiPoll.OptionsData[1].VotesCount)
	assert.Equal(t, 0, apiPoll.OptionsData[2].VotesCount)
	assert.Empty(t, apiPoll.Emojis)
}

func TestConverterImpl_FindEmojiCodesAndValidation(t *testing.T) {
	c := NewConverter("https://example.com").(*converterImpl)

	codes := c.findEmojiCodes("hello :smile: :a_b: :bad-emoji: :s:")
	assert.ElementsMatch(t, []string{"smile", "a_b"}, codes)

	assert.True(t, c.isValidEmojiCode("ab"))
	assert.True(t, c.isValidEmojiCode("A_B12"))
	assert.False(t, c.isValidEmojiCode("bad-emoji"))
	assert.False(t, c.isValidEmojiCode("x"))
	assert.False(t, c.isValidEmojiCode("a"+strings.Repeat("b", 32)))
}

func TestConverterImpl_NotesToStatus(t *testing.T) {
	c := NewConverter("https://example.com").(*converterImpl)

	status := c.NotesToStatus(map[string]any{
		"id":      "note-1",
		"content": "hello",
	})

	assert.Equal(t, VisibilityPublic, status.Visibility)
	assert.Equal(t, "en", status.Language)
}

func TestConverterImpl_ObjectToStatus_DefaultFields(t *testing.T) {
	c := NewConverter("https://example.com").(*converterImpl)

	actor := &activitypub.Actor{
		PreferredUsername: "alice",
		Name:              "Alice",
		URL:               "https://example.com/@alice",
	}

	status := c.ObjectToStatusWithContext(context.Background(), map[string]any{
		"id":        "https://example.com/notes/1",
		"content":   "hello",
		"published": time.Now().Format(time.RFC3339),
	}, actor, 0, 0, false, false, false)

	assert.Equal(t, 0, status.RepliesCount)
	assert.False(t, status.Muted)
	assert.False(t, status.Pinned)
}

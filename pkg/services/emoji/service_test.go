package emoji

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/streaming"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type nopPublisher struct{}

func (nopPublisher) PublishToUser(_ context.Context, _ string, _ *streaming.Event) error { return nil }
func (nopPublisher) PublishToStream(_ context.Context, _ string, _ *streaming.Event) error {
	return nil
}
func (nopPublisher) PublishToConversation(_ context.Context, _ string, _ *streaming.Event) error {
	return nil
}
func (nopPublisher) Close() error { return nil }

func TestService_validateShortcode(t *testing.T) {
	svc := NewService(nil, nil, zap.NewNop(), "example.com")

	require.ErrorIs(t, svc.validateShortcode("a"), errors.ErrInvalidShortcode)
	require.ErrorIs(t, svc.validateShortcode("all"), errors.ErrReservedShortcode)
	require.NoError(t, svc.validateShortcode("party_parrot"))
	require.Error(t, svc.validateShortcode("bad space"))
}

func TestService_filterEmojis(t *testing.T) {
	svc := NewService(nil, nil, zap.NewNop(), "example.com")
	emojis := []*storage.CustomEmoji{
		{Shortcode: "local_visible", Domain: "", VisibleInPicker: true, Disabled: false, Category: "fun"},
		{Shortcode: "local_hidden", Domain: "", VisibleInPicker: false, Disabled: false, Category: "fun"},
		{Shortcode: "remote_visible", Domain: "remote.example", VisibleInPicker: true, Disabled: false, Category: "fun"},
		{Shortcode: "disabled", Domain: "", VisibleInPicker: true, Disabled: true, Category: "mods"},
	}

	filtered := svc.filterEmojis(emojis, &ListEmojisQuery{OnlyLocal: true, OnlyVisible: true})
	require.Len(t, filtered, 1)
	require.Equal(t, "local_visible", filtered[0].Shortcode)

	filtered = svc.filterEmojis(emojis, &ListEmojisQuery{Category: "mods", IncludeDisabled: true})
	require.Len(t, filtered, 1)
	require.Equal(t, "disabled", filtered[0].Shortcode)
}

func TestService_emitEmojiCreatedEvents(t *testing.T) {
	svc := NewService(nil, nopPublisher{}, zap.NewNop(), "example.com")
	emoji := &storage.CustomEmoji{
		Shortcode: "party",
		URL:       "https://example.com/party.png",
		Category:  "fun",
		CreatedAt: time.Now(),
	}

	events := svc.emitEmojiCreatedEvents(context.Background(), emoji)
	require.Len(t, events, 1)
	require.Equal(t, "emoji.created", events[0].Type)
	require.Equal(t, "public", events[0].Stream)
	require.Equal(t, "party", events[0].Payload["shortcode"])
}

func TestService_emitEmojiCreatedEvents_nilPublisher(t *testing.T) {
	svc := NewService(nil, nil, zap.NewNop(), "example.com")
	require.Nil(t, svc.emitEmojiCreatedEvents(context.Background(), &storage.CustomEmoji{Shortcode: "party"}))
}

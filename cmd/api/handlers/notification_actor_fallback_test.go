package handlers

import (
	"testing"

	"github.com/equaltoai/lesser/pkg/config"
	"github.com/stretchr/testify/require"
)

func TestExtractUsernameFromNotificationActorID(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		input string
		want  string
	}{
		{name: "empty", input: "   ", want: ""},
		{name: "at_prefix", input: "@alice", want: "alice"},
		{name: "user_at_domain", input: "alice@example.com", want: "alice"},
		{name: "url_actor_path", input: "https://example.com/users/alice", want: "alice"},
		{name: "url_host_fallback", input: "https://example.com", want: "example.com"},
		{name: "url_no_host", input: "https:///", want: ""},
		{name: "url_parse_error", input: "https://%", want: ""},
		{name: "url_query_rejected", input: "https://example.com/users/alice?token=secret", want: ""},
		{name: "markup_rejected", input: "<script>", want: ""},
		{name: "plain", input: "bob", want: "bob"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, extractUsernameFromNotificationActorID(tc.input))
		})
	}
}

func TestFallbackNotificationActor(t *testing.T) {
	t.Parallel()

	t.Run("empty_returns_nil", func(t *testing.T) {
		require.Nil(t, (&Handler{}).fallbackNotificationActor(" "))
	})

	t.Run("url_actor_id_echoes", func(t *testing.T) {
		h := &Handler{cfg: &config.Config{Domain: "example.com"}}
		actor := h.fallbackNotificationActor("https://remote.example/users/bob")
		require.NotNil(t, actor)
		require.Equal(t, "https://remote.example/users/bob", actor.ID)
		require.Equal(t, "https://remote.example/users/bob", actor.URL)
		require.Equal(t, "bob", actor.PreferredUsername)
		require.Equal(t, "bob", actor.Name)
	})

	t.Run("url_parse_error_is_rejected", func(t *testing.T) {
		h := &Handler{cfg: &config.Config{Domain: "example.com"}}
		require.Nil(t, h.fallbackNotificationActor("https://%"))
	})

	t.Run("unsafe_identifier_is_rejected", func(t *testing.T) {
		h := &Handler{cfg: &config.Config{Domain: "example.com"}}
		require.Nil(t, h.fallbackNotificationActor("alice<script>"))
	})

	t.Run("local_identifier_builds_urls_when_configured", func(t *testing.T) {
		h := &Handler{cfg: &config.Config{Domain: "example.com"}}
		actor := h.fallbackNotificationActor("alice@example.com")
		require.NotNil(t, actor)
		require.Equal(t, "https://example.com/users/alice", actor.ID)
		require.Equal(t, "https://example.com/@alice", actor.URL)
		require.Equal(t, "https://example.com/users/alice/inbox", actor.Inbox)
		require.Equal(t, "https://example.com/users/alice/outbox", actor.Outbox)
		require.Equal(t, "alice", actor.PreferredUsername)
		require.Equal(t, "alice", actor.Name)
	})

	t.Run("no_config_falls_back_to_raw_id", func(t *testing.T) {
		actor := (&Handler{}).fallbackNotificationActor("carol")
		require.NotNil(t, actor)
		require.Equal(t, "carol", actor.ID)
		require.Equal(t, "carol", actor.URL)
		require.Equal(t, "carol", actor.PreferredUsername)
		require.Equal(t, "carol", actor.Name)
	})
}

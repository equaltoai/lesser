package handlers

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAgentRemotePolicyRound20_ExtractHandleFromActorID(t *testing.T) {
	t.Run("empty_or_invalid", func(t *testing.T) {
		require.Empty(t, extractHandleFromActorID(""))
		require.Empty(t, extractHandleFromActorID("   "))
		require.Empty(t, extractHandleFromActorID("not a url"))
		require.Empty(t, extractHandleFromActorID("remote.example/users/alice"))
		require.Empty(t, extractHandleFromActorID("https:///users/alice"))
	})

	t.Run("users_path", func(t *testing.T) {
		require.Equal(t, "alice@remote.example", extractHandleFromActorID("https://remote.example/users/alice"))
		require.Equal(t, "alice@remote.example", extractHandleFromActorID("https://remote.example/users/@alice"))
	})

	t.Run("at_path_segment", func(t *testing.T) {
		require.Equal(t, "alice@remote.example", extractHandleFromActorID("https://remote.example/@alice"))
		require.Equal(t, "Alice@remote.example", extractHandleFromActorID("https://remote.example/profiles/@Alice"))
	})

	t.Run("fallback_last_segment", func(t *testing.T) {
		require.Equal(t, "alice@remote.example", extractHandleFromActorID("https://remote.example/profiles/alice"))
		require.Equal(t, "alice@remote.example", extractHandleFromActorID("https://Remote.Example/alice"))
		require.Empty(t, extractHandleFromActorID("https://remote.example/@"))
	})
}

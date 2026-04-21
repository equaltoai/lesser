package surface

import (
	"net/http"
	"testing"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/stretchr/testify/require"
)

func TestSharedInboxManifest_DefaultContract(t *testing.T) {
	t.Parallel()

	manifest := Current()
	require.Equal(t, "/inbox", manifest.SharedInbox.Path)
	require.True(t, manifest.SharedInbox.Advertised)
	require.True(t, manifest.SharedInbox.ServesMethod(http.MethodPost))
	require.True(t, manifest.SharedInbox.AllowsMethod(http.MethodPost))
	require.True(t, manifest.SharedInbox.ServesMethod(http.MethodGet))
	require.False(t, manifest.SharedInbox.AllowsMethod(http.MethodGet))
	status, ok := manifest.SharedInbox.MethodStatus(http.MethodGet)
	require.True(t, ok)
	require.Equal(t, http.StatusMethodNotAllowed, status)
	require.Equal(t, "https://example.com/inbox", SharedInboxURL("https://example.com/"))
}

func TestApplyLocalActorIdentifiers_UsesManifestSharedInbox(t *testing.T) {
	t.Parallel()

	actor := &activitypub.Actor{PublicKey: &activitypub.PublicKey{PublicKeyPem: "pem"}}
	ApplyLocalActorIdentifiers(actor, "https://example.com/", "alice")

	require.Equal(t, "https://example.com/users/alice", actor.ID)
	require.Equal(t, "https://example.com/@alice", actor.URL)
	require.Equal(t, "https://example.com/users/alice/inbox", actor.Inbox)
	require.Equal(t, "https://example.com/users/alice/outbox", actor.Outbox)
	require.Equal(t, "https://example.com/users/alice/followers", actor.Followers)
	require.Equal(t, "https://example.com/users/alice/following", actor.Following)
	require.Equal(t, "https://example.com/users/alice/liked", actor.Liked)
	require.NotNil(t, actor.Endpoints)
	require.Equal(t, "https://example.com/inbox", actor.Endpoints.SharedInbox)
	require.Equal(t, "https://example.com/users/alice", actor.PublicKey.Owner)
	require.Equal(t, "https://example.com/users/alice#main-key", actor.PublicKey.ID)
}

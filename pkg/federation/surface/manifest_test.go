package surface

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSharedInboxManifest_DefaultContract(t *testing.T) {
	t.Parallel()

	manifest := Current()
	require.Equal(t, "/inbox", manifest.SharedInbox.Path)
	require.True(t, manifest.SharedInbox.Advertised)
	require.Equal(t, []string{http.MethodPost, http.MethodGet}, manifest.SharedInbox.ServedMethods())
	require.True(t, manifest.SharedInbox.ServesMethod(http.MethodPost))
	require.True(t, manifest.SharedInbox.AllowsMethod(http.MethodPost))
	require.True(t, manifest.SharedInbox.ServesMethod(http.MethodGet))
	require.False(t, manifest.SharedInbox.AllowsMethod(http.MethodGet))
	status, ok := manifest.SharedInbox.MethodStatus(http.MethodGet)
	require.True(t, ok)
	require.Equal(t, http.StatusMethodNotAllowed, status)
	require.Equal(t, "https://example.com/inbox", SharedInboxURL("https://example.com/"))
}

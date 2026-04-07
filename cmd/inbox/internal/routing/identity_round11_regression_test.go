package routing

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestInboxHandler_Round11_ExtractHandleFromActorID_CanonicalizesRemoteIdentities(t *testing.T) {
	env := newInboxTestEnv(t)

	require.Equal(t, "alice", env.handler.extractHandleFromActorID(env.cfg.ActorURL("alice")))
	require.Equal(t, "bob@remote.example", env.handler.extractHandleFromActorID("https://remote.example/users/@bob"))
	require.Equal(t, "alice@example.com", env.handler.extractHandleFromActorID("https://example.com/users/alice"))
}

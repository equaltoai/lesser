package repositories

import (
	"testing"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/stretchr/testify/require"
)

func TestNormalizedNumericIDMappingValues(t *testing.T) {
	numericID, username, actorID := normalizedNumericIDMappingValues("Agent-0", "https://example.com/users/Agent-0")
	require.Equal(t, common.GenerateNumericID("agent-0"), numericID)
	require.Equal(t, "agent-0", username)
	require.Equal(t, "https://example.com/users/agent-0", actorID)

	numericID, username, actorID = normalizedNumericIDMappingValues("  ", "https://example.com/users/Agent-0")
	require.Empty(t, numericID)
	require.Empty(t, username)
	require.Equal(t, "https://example.com/users/Agent-0", actorID)
}

func TestNormalizeCanonicalActorReference(t *testing.T) {
	require.Equal(t, "https://example.com/users/agent-0", normalizeCanonicalActorReference("https://example.com/users/Agent-0/", "agent-0"))
	require.Equal(t, "https://example.com/@agent-0", normalizeCanonicalActorReference("https://example.com/@Agent-0", "agent-0"))
	require.Equal(t, "https://remote.example/actors/Agent-0", normalizeCanonicalActorReference("https://remote.example/actors/Agent-0", "agent-0"))
}

func TestNormalizeLocalActorIdentityForStorage(t *testing.T) {
	baseURL := "https://example.com"
	actor := &activitypub.Actor{
		BaseObject: activitypub.BaseObject{
			ID:   baseURL + "/users/Agent-0",
			Type: activitypub.PersonType,
		},
		PreferredUsername: "Agent-0",
		URL:               baseURL + "/@Agent-0",
		Inbox:             baseURL + "/users/Agent-0/inbox",
		Outbox:            baseURL + "/users/Agent-0/outbox",
		Followers:         baseURL + "/users/Agent-0/followers",
		Following:         baseURL + "/users/Agent-0/following",
		Liked:             baseURL + "/users/Agent-0/liked",
	}

	normalized := normalizeLocalActorIdentityForStorage("Agent-0", baseURL, actor)
	require.NotNil(t, normalized)
	require.Equal(t, "agent-0", normalized.PreferredUsername)
	require.Equal(t, "https://example.com/users/agent-0", normalized.ID)
	require.Equal(t, "https://example.com/@agent-0", normalized.URL)
	require.Equal(t, "https://example.com/users/agent-0/inbox", normalized.Inbox)
	require.Equal(t, "https://example.com/users/agent-0/outbox", normalized.Outbox)
	require.Equal(t, "https://example.com/users/agent-0/followers", normalized.Followers)
	require.Equal(t, "https://example.com/users/agent-0/following", normalized.Following)
	require.Equal(t, "https://example.com/users/agent-0/liked", normalized.Liked)
	require.NotNil(t, normalized.Endpoints)
	require.Equal(t, "https://example.com/inbox", normalized.Endpoints.SharedInbox)

	withoutBaseURL := normalizeLocalActorIdentityForStorage("Agent-0", "", actor)
	require.Equal(t, "agent-0", withoutBaseURL.PreferredUsername)
	require.Equal(t, "https://example.com/users/agent-0", withoutBaseURL.ID)
	require.Equal(t, "https://example.com/@agent-0", withoutBaseURL.URL)
}

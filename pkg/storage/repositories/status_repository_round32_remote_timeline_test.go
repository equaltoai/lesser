package repositories

import (
	"context"
	"testing"

	"github.com/equaltoai/lesser/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestRound32HomeTimelineActorID_PreservesRemoteIdentities(t *testing.T) {
	t.Setenv("DOMAIN", "example.com")
	config.ResetForTests()
	t.Cleanup(config.ResetForTests)

	cfg := config.Get()

	require.Equal(t, "", homeTimelineActorID("", cfg.Domain, nil))
	require.Equal(t, cfg.ActorURL("alice"), homeTimelineActorID("alice", cfg.Domain, nil))
	require.Equal(t, cfg.ActorURL("alice"), homeTimelineActorID("alice@example.com", cfg.Domain, nil))
	require.Equal(t, cfg.ActorURL("alice"), homeTimelineActorID("acct:alice@example.com", cfg.Domain, nil))
	require.Equal(t, "https://remote.example/users/bob", homeTimelineActorID("bob@remote.example", cfg.Domain, nil))
	require.Equal(t, "https://remote.example/users/bob", homeTimelineActorID("@bob@remote.example", cfg.Domain, nil))
	require.Equal(t, "https://remote.example/actors/bob", homeTimelineActorID("bob@remote.example", cfg.Domain, func(string) string {
		return "https://remote.example/actors/bob"
	}))
	calledWith := ""
	require.Equal(t, "https://remote.example/actors/carol", homeTimelineActorID("acct:carol@remote.example", cfg.Domain, func(handle string) string {
		calledWith = handle
		return "https://remote.example/actors/carol"
	}))
	require.Equal(t, "carol@remote.example", calledWith)
	require.Equal(t, "https://remote.example/users/bob", homeTimelineActorID("https://remote.example/users/bob", cfg.Domain, nil))
	require.Equal(t, "", homeTimelineActorID("@", cfg.Domain, nil))
}

func TestRound32ResolveTimelineActorID_LocalShortCircuitsRemoteLookup(t *testing.T) {
	t.Setenv("DOMAIN", "example.com")
	config.ResetForTests()
	t.Cleanup(config.ResetForTests)

	cfg := config.Get()
	repo := NewStatusRepository(nil, "", zap.NewNop(), nil)

	require.Equal(t, "", repo.resolveTimelineActorID(context.Background(), ""))
	require.Equal(t, cfg.ActorURL("alice"), repo.resolveTimelineActorID(context.Background(), "alice"))
	require.Equal(t, cfg.ActorURL("alice"), repo.resolveTimelineActorID(context.Background(), "alice@example.com"))
	require.Equal(t, "https://remote.example/users/bob", repo.resolveTimelineActorID(context.Background(), "bob@remote.example"))
}

func TestRound32FetchFollowingActorIDs_UnsupportedRepoFailsClearly(t *testing.T) {
	repo := NewStatusRepository(nil, "test-table", zap.NewNop(), nil)
	repo.SetRelationshipRepository(struct{}{})

	actorIDs, err := repo.fetchFollowingActorIDs(context.Background(), "alice")
	require.Error(t, err)
	require.Nil(t, actorIDs)
	require.Contains(t, err.Error(), "does not support GetFollowing")
}

func TestRound32FetchFollowingActorIDs_UsernameAccessorError(t *testing.T) {
	repo := NewStatusRepository(nil, "test-table", zap.NewNop(), nil)
	repo.SetRelationshipRepository(usernameRelationshipRepo{err: assert.AnError})

	actorIDs, err := repo.fetchFollowingActorIDs(context.Background(), "alice")
	require.ErrorIs(t, err, assert.AnError)
	require.Nil(t, actorIDs)
}

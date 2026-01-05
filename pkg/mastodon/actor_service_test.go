package mastodon

import (
	"context"
	"errors"
	"testing"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type stubActorRepo struct {
	actors map[string]*activitypub.Actor
	errs   map[string]error
}

func (s *stubActorRepo) GetActorByUsername(_ context.Context, username string) (*activitypub.Actor, error) {
	if err, ok := s.errs[username]; ok && err != nil {
		return nil, err
	}
	if actor, ok := s.actors[username]; ok {
		return actor, nil
	}
	return nil, errors.New("not found")
}

type stubRelationshipRepo struct {
	followers    []string
	followersErr error
	following    []string
	followingErr error
}

func (s *stubRelationshipRepo) GetFollowers(_ context.Context, _ string, _ int, _ string) ([]string, string, error) {
	return s.followers, "", s.followersErr
}

func (s *stubRelationshipRepo) GetFollowing(_ context.Context, _ string, _ int, _ string) ([]string, string, error) {
	return s.following, "", s.followingErr
}

type stubObjectRepo struct {
	objects []any
	err     error
}

func (s *stubObjectRepo) GetObjectsByActor(_ context.Context, _ string, _ string, _ int) ([]any, string, error) {
	return s.objects, "", s.err
}

func TestActorService_GetAccountByUsername(t *testing.T) {
	actor := &activitypub.Actor{
		BaseObject: activitypub.BaseObject{
			ID: "https://example.com/users/alice",
		},
		PreferredUsername: "alice",
		Name:              "Alice",
		URL:               "https://example.com/@alice",
	}
	svc := &actorServiceImpl{
		actors:    &stubActorRepo{actors: map[string]*activitypub.Actor{"alice": actor}},
		converter: NewConverter("https://example.com"),
		logger:    zap.NewNop(),
	}

	account, err := svc.GetAccountByUsername(context.Background(), "alice")
	require.NoError(t, err)
	require.NotNil(t, account)
	assert.Equal(t, "alice", account.Username)
}

func TestActorService_GetAccountByUsername_Error(t *testing.T) {
	svc := &actorServiceImpl{
		actors:    &stubActorRepo{errs: map[string]error{"alice": errors.New("boom")}},
		converter: NewConverter("https://example.com"),
		logger:    zap.NewNop(),
	}

	_, err := svc.GetAccountByUsername(context.Background(), "alice")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get actor")
}

func TestActorService_GetAccountWithStats(t *testing.T) {
	actor := &activitypub.Actor{
		BaseObject: activitypub.BaseObject{
			ID: "https://example.com/users/alice",
		},
		PreferredUsername: "alice",
		Name:              "Alice",
		URL:               "https://example.com/@alice",
	}
	svc := &actorServiceImpl{
		actors:        &stubActorRepo{actors: map[string]*activitypub.Actor{"alice": actor}},
		relationships: &stubRelationshipRepo{followers: []string{"bob"}, following: []string{"carol"}},
		objects:       &stubObjectRepo{objects: []any{"status"}},
		converter:     NewConverter("https://example.com"),
		logger:        zap.NewNop(),
	}

	account, err := svc.GetAccountWithStats(context.Background(), "alice")
	require.NoError(t, err)
	require.NotNil(t, account)
	assert.Equal(t, 1, account.FollowersCount)
	assert.Equal(t, 1, account.FollowingCount)
	assert.Equal(t, 1, account.StatusesCount)
}

func TestActorService_GetAccountsByIDs_SkipsInvalidAndMissing(t *testing.T) {
	actor := &activitypub.Actor{
		BaseObject: activitypub.BaseObject{
			ID: "https://example.com/users/alice",
		},
		PreferredUsername: "alice",
		Name:              "Alice",
		URL:               "https://example.com/@alice",
	}

	svc := &actorServiceImpl{
		actors: &stubActorRepo{
			actors: map[string]*activitypub.Actor{"alice": actor},
			errs:   map[string]error{"bob": errors.New("boom")},
		},
		converter: NewConverter("https://example.com"),
		logger:    zap.NewNop(),
	}

	accounts, err := svc.GetAccountsByIDs(context.Background(), []string{
		"https://example.com/users/alice",
		"https://example.com/users/bob",
		"",
	})
	require.NoError(t, err)
	require.Len(t, accounts, 1)
	assert.Equal(t, "alice", accounts[0].Username)
}

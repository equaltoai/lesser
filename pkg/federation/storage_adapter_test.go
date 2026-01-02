package federation

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type storageAdapterActorRepoStub struct {
	privateKey string
	privateErr error

	actor    *activitypub.Actor
	actorErr error

	cached    *activitypub.Actor
	cachedErr error

	gotUsername string
	gotActorID  string
}

func (s *storageAdapterActorRepoStub) GetActorPrivateKey(_ context.Context, username string) (string, error) {
	s.gotUsername = username
	if s.privateErr != nil {
		return "", s.privateErr
	}
	return s.privateKey, nil
}

func (s *storageAdapterActorRepoStub) GetActorByUsername(_ context.Context, username string) (*activitypub.Actor, error) {
	s.gotUsername = username
	if s.actorErr != nil {
		return nil, s.actorErr
	}
	return s.actor, nil
}

func (s *storageAdapterActorRepoStub) GetCachedRemoteActor(_ context.Context, actorID string) (*activitypub.Actor, error) {
	s.gotActorID = actorID
	if s.cachedErr != nil {
		return nil, s.cachedErr
	}
	return s.cached, nil
}

type storageAdapterRelationshipRepoStub struct {
	followers []string
	cursor    string
	err       error

	gotUsername string
	gotLimit    int
	gotCursor   string
}

func (s *storageAdapterRelationshipRepoStub) GetFollowers(_ context.Context, username string, limit int, cursor string) ([]string, string, error) {
	s.gotUsername = username
	s.gotLimit = limit
	s.gotCursor = cursor
	if s.err != nil {
		return nil, "", s.err
	}
	return s.followers, s.cursor, nil
}

type storageAdapterUserRepoStub struct {
	err error

	gotHandle string
	gotTTL    time.Duration
}

func (s *storageAdapterUserRepoStub) CacheRemoteActor(_ context.Context, handle string, _ *activitypub.Actor, ttl time.Duration) error {
	s.gotHandle = handle
	s.gotTTL = ttl
	return s.err
}

type storageAdapterFederationRepoStub struct {
	err error

	got *storage.FederationActivity
}

func (s *storageAdapterFederationRepoStub) RecordFederationActivity(_ context.Context, activity *storage.FederationActivity) error {
	s.got = activity
	return s.err
}

func TestRepositoryStorageAdapter_Delegates(t *testing.T) {
	ctx := context.Background()

	actorRepo := &storageAdapterActorRepoStub{
		privateKey: "pem",
		actor:      &activitypub.Actor{PreferredUsername: "alice"},
		cached:     &activitypub.Actor{PreferredUsername: "remote"},
	}
	relationshipRepo := &storageAdapterRelationshipRepoStub{followers: []string{"a", "b"}, cursor: "next"}
	userRepo := &storageAdapterUserRepoStub{}
	federationRepo := &storageAdapterFederationRepoStub{}

	adapter := &RepositoryStorageAdapter{
		actorRepo:        actorRepo,
		relationshipRepo: relationshipRepo,
		userRepo:         userRepo,
		federationRepo:   federationRepo,
	}

	privateKey, err := adapter.GetActorPrivateKey(ctx, "alice")
	require.NoError(t, err)
	assert.Equal(t, "pem", privateKey)

	actor, err := adapter.GetActor(ctx, "alice")
	require.NoError(t, err)
	assert.Equal(t, "alice", actor.PreferredUsername)

	followers, cursor, err := adapter.GetFollowers(ctx, "alice", 10, "c")
	require.NoError(t, err)
	assert.Equal(t, []string{"a", "b"}, followers)
	assert.Equal(t, "next", cursor)
	assert.Equal(t, "alice", relationshipRepo.gotUsername)
	assert.Equal(t, 10, relationshipRepo.gotLimit)
	assert.Equal(t, "c", relationshipRepo.gotCursor)

	cached, err := adapter.GetCachedRemoteActor(ctx, "https://remote.example/users/alice")
	require.NoError(t, err)
	assert.Equal(t, "remote", cached.PreferredUsername)
	assert.Equal(t, "https://remote.example/users/alice", actorRepo.gotActorID)

	require.NoError(t, adapter.CacheRemoteActor(ctx, "alice@remote.example", cached, 5*time.Minute))
	assert.Equal(t, "alice@remote.example", userRepo.gotHandle)
	assert.Equal(t, 5*time.Minute, userRepo.gotTTL)

	act := &storage.FederationActivity{Domain: "remote.example"}
	require.NoError(t, adapter.RecordFederationActivity(ctx, act))
	assert.Equal(t, act, federationRepo.got)
}

func TestRepositoryStorageAdapter_ErrorPassthrough(t *testing.T) {
	ctx := context.Background()

	adapter := &RepositoryStorageAdapter{
		actorRepo:        &storageAdapterActorRepoStub{privateErr: errors.New("boom")},
		relationshipRepo: &storageAdapterRelationshipRepoStub{err: errors.New("boom")},
		userRepo:         &storageAdapterUserRepoStub{err: errors.New("boom")},
		federationRepo:   &storageAdapterFederationRepoStub{err: errors.New("boom")},
	}

	_, err := adapter.GetActorPrivateKey(ctx, "alice")
	assert.Error(t, err)

	_, _, err = adapter.GetFollowers(ctx, "alice", 1, "")
	assert.Error(t, err)

	assert.Error(t, adapter.CacheRemoteActor(ctx, "alice@remote.example", nil, time.Second))
	assert.Error(t, adapter.RecordFederationActivity(ctx, &storage.FederationActivity{}))
}


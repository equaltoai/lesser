package graph

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/services/threads"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/testing/inmemory"
	"github.com/stretchr/testify/require"
)

func TestRound32Resolver_ConvertAccountToActor_PreservesRemoteIdentity(t *testing.T) {
	resolver, _ := newRound12GraphResolver(t)

	remoteActor := &activitypub.Actor{
		BaseObject: activitypub.BaseObject{
			ID:   "https://remote.example/users/alice",
			Type: activitypub.PersonType,
		},
		PreferredUsername: "alice",
		Name:              "Remote Alice",
		URL:               "https://remote.example/@alice",
		Inbox:             "https://remote.example/users/alice/inbox",
		Outbox:            "https://remote.example/users/alice/outbox",
	}

	actor := resolver.convertAccountToActor(&storage.Account{
		User:  &storage.User{Username: "alice@remote.example", DisplayName: "Wrapped Alice"},
		Actor: remoteActor,
	})
	require.NotNil(t, actor)
	require.Equal(t, remoteActor.ID, actor.ID)
	require.Equal(t, remoteActor.PreferredUsername, actor.PreferredUsername)
	require.Equal(t, remoteActor.URL, actor.URL)
}

func TestRound32ActivityResolver_ActorPreservesRemoteIdentifiers(t *testing.T) {
	resolver, graphStorage := newRound12GraphResolver(t)
	graphStorage.SeedAccountUser(&storage.User{
		Username:    "alice",
		DisplayName: "Local Alice",
		Approved:    true,
	})

	activityResolver := resolver.Activity()
	ctx := context.Background()

	remoteActorURL := "https://remote.example/users/alice"
	urlActor, err := activityResolver.Actor(ctx, &activitypub.Activity{Actor: remoteActorURL})
	require.NoError(t, err)
	require.NotNil(t, urlActor)
	require.Equal(t, remoteActorURL, urlActor.ID)
	require.Equal(t, "alice", urlActor.PreferredUsername)

	remoteActorHandle := "alice@remote.example"
	handleActor, err := activityResolver.Actor(ctx, &activitypub.Activity{Actor: remoteActorHandle})
	require.NoError(t, err)
	require.NotNil(t, handleActor)
	require.Equal(t, remoteActorHandle, handleActor.ID)
	require.Equal(t, "alice", handleActor.PreferredUsername)
}

func TestRound32ConvertThreadContextRemoteNoteDoesNotUseLocalAccount(t *testing.T) {
	resolver, graphStorage := newRound12GraphResolver(t)
	graphStorage.SeedAccountUser(&storage.User{
		Username:    "alice",
		DisplayName: "Local Alice",
		Approved:    true,
	})

	now := time.Now().UTC()
	remoteActorURL := "https://remote.example/users/alice"
	threadCtx := resolver.convertThreadContextResultToModel(context.Background(), &threads.ThreadContextResult{
		RootNote: &activitypub.Note{
			BaseObject: activitypub.BaseObject{
				ID:        "https://remote.example/notes/1",
				Type:      activitypub.NoteType,
				Published: &now,
			},
			AttributedTo: remoteActorURL,
			Content:      "remote alice should not resolve to local alice",
		},
		LastActivity: now,
		SyncStatus:   threads.SyncStatusComplete,
	})

	require.NotNil(t, threadCtx)
	require.NotNil(t, threadCtx.RootNote)
	require.NotNil(t, threadCtx.RootNote.Actor)
	require.Equal(t, remoteActorURL, threadCtx.RootNote.Actor.ID)
	require.Equal(t, "alice", threadCtx.RootNote.Actor.PreferredUsername)
	require.NotEqual(t, resolver.Config.ActorURL("alice"), threadCtx.RootNote.Actor.ID)
	require.NotEqual(t, "Local Alice", threadCtx.RootNote.Actor.Name)
}

func TestRound32ConvertThreadContextLocalNoteStillUsesLocalAccount(t *testing.T) {
	resolver, graphStorage := newRound12GraphResolver(t)
	graphStorage.SeedAccountUser(&storage.User{
		Username:    "alice",
		DisplayName: "Local Alice",
		Approved:    true,
	})

	now := time.Now().UTC()
	localActorURL := resolver.Config.ActorURL("alice")
	threadCtx := resolver.convertThreadContextResultToModel(context.Background(), &threads.ThreadContextResult{
		RootNote: &activitypub.Note{
			BaseObject: activitypub.BaseObject{
				ID:        resolver.Config.ObjectURL("objects", "local-note-1"),
				Type:      activitypub.NoteType,
				Published: &now,
			},
			AttributedTo: localActorURL,
			Content:      "local alice should still resolve locally",
		},
		LastActivity: now,
		SyncStatus:   threads.SyncStatusComplete,
	})

	require.NotNil(t, threadCtx)
	require.NotNil(t, threadCtx.RootNote)
	require.NotNil(t, threadCtx.RootNote.Actor)
	require.True(t, common.IsLocalActorID(threadCtx.RootNote.Actor.ID, resolver.Config.Domain))
	require.Contains(t, threadCtx.RootNote.Actor.ID, "/users/alice")
	require.Equal(t, "alice", threadCtx.RootNote.Actor.PreferredUsername)
	require.Equal(t, "Local Alice", threadCtx.RootNote.Actor.Name)
}

func TestRound32ConvertNoteToObjectAttributedActorGuardBranches(t *testing.T) {
	resolver, graphStorage := newRound12GraphResolver(t)
	graphStorage.SeedAccountUser(&storage.User{
		Username:    "alice",
		DisplayName: "Local Alice",
		Approved:    true,
	})

	actorRepo, ok := graphStorage.Actor().(*inmemory.ActorRepository)
	require.True(t, ok)

	remoteActor := &activitypub.Actor{
		BaseObject: activitypub.BaseObject{
			ID:   "https://remote.example/users/alice",
			Type: activitypub.PersonType,
		},
		PreferredUsername: "alice",
		Name:              "Remote Alice",
		URL:               "https://remote.example/@alice",
		Inbox:             "https://remote.example/users/alice/inbox",
		Outbox:            "https://remote.example/users/alice/outbox",
	}
	actorRepo.SetCachedRemoteActor(remoteActor.ID, remoteActor, time.Hour)

	now := time.Now().UTC()
	ctx := context.Background()

	t.Run("cached remote actor URL stays remote", func(t *testing.T) {
		obj := resolver.convertNoteToObject(ctx, &activitypub.Note{
			BaseObject: activitypub.BaseObject{
				ID:        "https://remote.example/notes/1",
				Type:      activitypub.NoteType,
				Published: &now,
			},
			AttributedTo: remoteActor.ID,
			Content:      "cached remote alice",
		})

		require.NotNil(t, obj)
		require.NotNil(t, obj.Actor)
		require.Equal(t, remoteActor.ID, obj.Actor.ID)
		require.Equal(t, "Remote Alice", obj.Actor.Name)
		require.NotEqual(t, resolver.Config.ActorURL("alice"), obj.Actor.ID)
	})

	t.Run("uncached remote actor URL uses placeholder instead of local account", func(t *testing.T) {
		uncachedRemoteID := "https://uncached.example/users/alice"
		obj := resolver.convertNoteToObject(ctx, &activitypub.Note{
			BaseObject: activitypub.BaseObject{
				ID:        "https://uncached.example/notes/1",
				Type:      activitypub.NoteType,
				Published: &now,
			},
			AttributedTo: uncachedRemoteID,
			Content:      "uncached remote alice",
		})

		require.NotNil(t, obj)
		require.NotNil(t, obj.Actor)
		require.Equal(t, uncachedRemoteID, obj.Actor.ID)
		require.Equal(t, "alice", obj.Actor.PreferredUsername)
		require.NotEqual(t, resolver.Config.ActorURL("alice"), obj.Actor.ID)
		require.NotEqual(t, "Local Alice", obj.Actor.Name)
	})

	t.Run("local actor URL still resolves through local account", func(t *testing.T) {
		obj := resolver.convertNoteToObject(ctx, &activitypub.Note{
			BaseObject: activitypub.BaseObject{
				ID:        resolver.Config.ObjectURL("objects", "local-note-2"),
				Type:      activitypub.NoteType,
				Published: &now,
			},
			AttributedTo: resolver.Config.ActorURL("alice"),
			Content:      "local alice",
		})

		require.NotNil(t, obj)
		require.NotNil(t, obj.Actor)
		require.True(t, common.IsLocalActorID(obj.Actor.ID, resolver.Config.Domain))
		require.Contains(t, obj.Actor.ID, "/users/alice")
		require.Equal(t, "alice", obj.Actor.PreferredUsername)
		require.Equal(t, "Local Alice", obj.Actor.Name)
	})
}

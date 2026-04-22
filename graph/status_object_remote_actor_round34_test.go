package graph

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/config"
	storageModels "github.com/equaltoai/lesser/pkg/storage/models"
	pkgtesting "github.com/equaltoai/lesser/pkg/testing"
	"github.com/equaltoai/lesser/pkg/testing/inmemory"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestRound34ConvertStatusToGraphQLObject_HydratesRemoteActorThroughExactResolution(t *testing.T) {
	storage := pkgtesting.NewMockRepositoryStorage()
	actorRepo, ok := storage.Actor().(*inmemory.ActorRepository)
	require.True(t, ok)

	remoteActor := &activitypub.Actor{
		BaseObject: activitypub.BaseObject{
			ID:   "https://remote.example/users/alice",
			Type: activitypub.PersonType,
		},
		PreferredUsername: "alice",
		Name:              "Remote Alice",
		Inbox:             "https://remote.example/users/alice/inbox",
		Outbox:            "https://remote.example/users/alice/outbox",
	}
	actorRepo.SetCachedRemoteActor("alice@remote.example", remoteActor, time.Hour)

	resolver := &Resolver{
		Storage: storage,
		Config: &config.Config{
			Domain: "localhost",
		},
		Logger: zap.NewNop(),
	}

	now := time.Now().UTC()
	status := &storageModels.Status{
		StatusID:       "status-remote-1",
		AuthorID:       remoteActor.ID,
		AuthorUsername: "alice@remote.example",
		Content:        "hello from remote alice",
		CreatedAt:      now.Add(-time.Minute),
		UpdatedAt:      now,
		Visibility:     storageModels.VisibilityPublic,
		Note: &activitypub.Note{
			BaseObject: activitypub.BaseObject{
				ID:        "https://remote.example/notes/1",
				Type:      activitypub.NoteType,
				Published: &now,
			},
			AttributedTo: remoteActor.ID,
			Content:      "hello from remote alice",
		},
	}

	obj := resolver.ConvertStatusToGraphQLObject(context.Background(), status)
	require.NotNil(t, obj)
	require.NotNil(t, obj.Actor)
	require.Equal(t, remoteActor.ID, obj.Actor.ID)
	require.Equal(t, remoteActor.PreferredUsername, obj.Actor.PreferredUsername)
	require.Equal(t, remoteActor.Inbox, obj.Actor.Inbox)
}

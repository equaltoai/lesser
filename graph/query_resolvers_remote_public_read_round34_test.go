package graph

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/activitypub"
	storageModels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/testing/inmemory"
	"github.com/stretchr/testify/require"
)

func TestRound34QueryResolvers_RemotePublicReadHydratesRemoteActors(t *testing.T) {
	resolver, storage := newRound12GraphResolver(t)
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

	now := time.Now().UTC()
	root := &storageModels.Status{
		StatusID:       "status-remote-root",
		AuthorID:       remoteActor.ID,
		AuthorUsername: "alice@remote.example",
		Content:        "remote root",
		CreatedAt:      now.Add(-2 * time.Minute),
		UpdatedAt:      now.Add(-2 * time.Minute),
		PublishedAt:    now.Add(-2 * time.Minute),
		Visibility:     storageModels.VisibilityPublic,
		Note: &activitypub.Note{
			BaseObject: activitypub.BaseObject{
				ID:        "https://remote.example/notes/root",
				Type:      activitypub.NoteType,
				Published: pointTime(now.Add(-2 * time.Minute)),
			},
			AttributedTo: remoteActor.ID,
			Content:      "remote root",
		},
	}
	reply := &storageModels.Status{
		StatusID:       "status-remote-reply",
		AuthorID:       remoteActor.ID,
		AuthorUsername: "alice@remote.example",
		Content:        "remote reply",
		InReplyToID:    root.StatusID,
		CreatedAt:      now.Add(-time.Minute),
		UpdatedAt:      now.Add(-time.Minute),
		PublishedAt:    now.Add(-time.Minute),
		Visibility:     storageModels.VisibilityPublic,
		Note: &activitypub.Note{
			BaseObject: activitypub.BaseObject{
				ID:        "https://remote.example/notes/reply",
				Type:      activitypub.NoteType,
				Published: pointTime(now.Add(-time.Minute)),
				InReplyTo: root.Note.ID,
			},
			AttributedTo: remoteActor.ID,
			Content:      "remote reply",
		},
	}

	ctx := context.Background()
	require.NoError(t, storage.Status().CreateStatus(ctx, root))
	require.NoError(t, storage.Status().CreateStatus(ctx, reply))

	q := resolver.Query()

	obj, err := q.Object(ctx, root.StatusID)
	require.NoError(t, err)
	require.NotNil(t, obj)
	require.NotNil(t, obj.Actor)
	require.Equal(t, remoteActor.ID, obj.Actor.ID)

	first := 10
	publicTimeline, err := q.Timeline(ctx, model.TimelineTypePublic, nil, nil, nil, &first, nil, nil, nil)
	require.NoError(t, err)
	require.NotNil(t, publicTimeline)
	require.Equal(t, remoteActor.ID, findTimelineNodeByID(t, publicTimeline, root.StatusID).Actor.ID)

	actorID := remoteActor.ID
	actorTimeline, err := q.Timeline(ctx, model.TimelineTypeActor, nil, nil, &actorID, &first, nil, nil, nil)
	require.NoError(t, err)
	require.NotNil(t, actorTimeline)
	require.Equal(t, remoteActor.ID, findTimelineNodeByID(t, actorTimeline, root.StatusID).Actor.ID)
	require.Equal(t, remoteActor.ID, findTimelineNodeByID(t, actorTimeline, reply.StatusID).Actor.ID)

	thread, err := q.ThreadContext(ctx, root.StatusID)
	require.NoError(t, err)
	require.NotNil(t, thread)
	require.NotNil(t, thread.RootNote)
	require.NotNil(t, thread.RootNote.Actor)
	require.Equal(t, remoteActor.ID, thread.RootNote.Actor.ID)
	require.Len(t, thread.Descendants, 1)
	require.NotNil(t, thread.Descendants[0].Actor)
	require.Equal(t, remoteActor.ID, thread.Descendants[0].Actor.ID)
}

func TestRound34QueryResolvers_RemotePublicReadUsesPlaceholderWhenCacheMisses(t *testing.T) {
	resolver, storage := newRound12GraphResolver(t)

	now := time.Now().UTC()
	remoteActorID := "https://127.0.0.1:1/users/alice"
	root := &storageModels.Status{
		StatusID:    "status-remote-root-uncached",
		AuthorID:    remoteActorID,
		Content:     "remote root uncached",
		CreatedAt:   now.Add(-2 * time.Minute),
		UpdatedAt:   now.Add(-2 * time.Minute),
		PublishedAt: now.Add(-2 * time.Minute),
		Visibility:  storageModels.VisibilityPublic,
		Note: &activitypub.Note{
			BaseObject: activitypub.BaseObject{
				ID:        "https://127.0.0.1:1/notes/root",
				Type:      activitypub.NoteType,
				Published: pointTime(now.Add(-2 * time.Minute)),
			},
			AttributedTo: remoteActorID,
			Content:      "remote root uncached",
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()

	require.NoError(t, storage.Status().CreateStatus(ctx, root))

	q := resolver.Query()

	obj, err := q.Object(ctx, root.StatusID)
	require.NoError(t, err)
	require.NotNil(t, obj)
	require.NotNil(t, obj.Actor)
	require.Equal(t, remoteActorID, obj.Actor.ID)
	require.Equal(t, "alice", obj.Actor.PreferredUsername)

	first := 10
	publicTimeline, err := q.Timeline(ctx, model.TimelineTypePublic, nil, nil, nil, &first, nil, nil, nil)
	require.NoError(t, err)
	require.NotNil(t, publicTimeline)
	require.Equal(t, remoteActorID, findTimelineNodeByID(t, publicTimeline, root.StatusID).Actor.ID)

	thread, err := q.ThreadContext(ctx, root.StatusID)
	require.NoError(t, err)
	require.NotNil(t, thread)
	require.NotNil(t, thread.RootNote)
	require.NotNil(t, thread.RootNote.Actor)
	require.Equal(t, remoteActorID, thread.RootNote.Actor.ID)
}

func findTimelineNodeByID(t *testing.T, conn *model.ObjectConnection, statusID string) *model.Object {
	t.Helper()

	require.NotNil(t, conn)
	for _, edge := range conn.Edges {
		if edge == nil || edge.Node == nil {
			continue
		}
		if edge.Node.ID == statusID {
			return edge.Node
		}
	}

	t.Fatalf("timeline missing status %s", statusID)
	return nil
}

func pointTime(value time.Time) *time.Time {
	return &value
}

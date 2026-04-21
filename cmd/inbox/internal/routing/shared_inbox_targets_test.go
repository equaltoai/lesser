package routing

import (
	"context"
	"testing"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/stretchr/testify/require"
)

type sharedInboxActorRepoStub struct {
	actors map[string]*activitypub.Actor
}

func (s *sharedInboxActorRepoStub) GetActorByUsername(_ context.Context, username string) (*activitypub.Actor, error) {
	return s.actors[username], nil
}

type sharedInboxFollowerPage struct {
	followers  []string
	nextCursor string
}

type sharedInboxRelationshipRepoStub struct {
	followers map[string][]string
	pages     map[string]map[string]sharedInboxFollowerPage
}

func (s *sharedInboxRelationshipRepoStub) GetFollowers(_ context.Context, username string, _ int, cursor string) ([]string, string, error) {
	if s.pages != nil {
		if handlePages, ok := s.pages[username]; ok {
			page, ok := handlePages[cursor]
			if !ok {
				return nil, "", nil
			}
			return append([]string(nil), page.followers...), page.nextCursor, nil
		}
	}
	return append([]string(nil), s.followers[username]...), "", nil
}

type sharedInboxActivityRepoStub struct {
	activities map[string]*activitypub.Activity
}

func (s *sharedInboxActivityRepoStub) GetActivity(_ context.Context, id string) (*activitypub.Activity, error) {
	return s.activities[id], nil
}

type sharedInboxObjectRepoStub struct {
	objects map[string]any
}

func (s *sharedInboxObjectRepoStub) GetObject(_ context.Context, id string) (any, error) {
	return s.objects[id], nil
}

func TestSharedInboxTargetResolver_ResolveFollowToLocalActor(t *testing.T) {
	t.Parallel()

	resolver := sharedInboxTargetResolver{
		actorRepository: &sharedInboxActorRepoStub{actors: map[string]*activitypub.Actor{
			"alice": {BaseObject: activitypub.BaseObject{ID: "https://example.com/users/alice"}, PreferredUsername: "alice"},
		}},
		localDomain: "example.com",
	}

	activity := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{Type: activitypub.FollowType},
		Object:     "https://example.com/users/alice",
	}

	actors, err := resolver.Resolve(context.Background(), activity)
	require.NoError(t, err)
	require.Len(t, actors, 1)
	require.Equal(t, "alice", actors[0].PreferredUsername)
}

func TestSharedInboxTargetResolver_PublicCreateResolvesFollowerSetWithoutTreatingPublicAsTarget(t *testing.T) {
	t.Parallel()

	resolver := sharedInboxTargetResolver{
		actorRepository: &sharedInboxActorRepoStub{actors: map[string]*activitypub.Actor{
			"alice": {BaseObject: activitypub.BaseObject{ID: "https://example.com/users/alice"}, PreferredUsername: "alice"},
			"carol": {BaseObject: activitypub.BaseObject{ID: "https://example.com/users/carol"}, PreferredUsername: "carol"},
		}},
		relationshipRepository: &sharedInboxRelationshipRepoStub{followers: map[string][]string{
			"bob@remote.example": {"alice", "carol"},
		}},
		localDomain: "example.com",
	}

	activity := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			Type: activitypub.CreateType,
			To:   []string{activitypub.PublicAddress},
			CC:   []string{"https://remote.example/users/bob/followers"},
		},
		Actor: "https://remote.example/users/bob",
		Object: map[string]any{
			"id":           "https://remote.example/notes/1",
			"type":         activitypub.NoteType,
			"attributedTo": "https://remote.example/users/bob",
		},
	}

	actors, err := resolver.Resolve(context.Background(), activity)
	require.NoError(t, err)
	require.Len(t, actors, 2)
	require.ElementsMatch(t, []string{"alice", "carol"}, []string{actors[0].PreferredUsername, actors[1].PreferredUsername})
}

func TestSharedInboxTargetResolver_PublicCreateResolvesFollowerPages(t *testing.T) {
	t.Parallel()

	resolver := sharedInboxTargetResolver{
		actorRepository: &sharedInboxActorRepoStub{actors: map[string]*activitypub.Actor{
			"alice": {BaseObject: activitypub.BaseObject{ID: "https://example.com/users/alice"}, PreferredUsername: "alice"},
			"carol": {BaseObject: activitypub.BaseObject{ID: "https://example.com/users/carol"}, PreferredUsername: "carol"},
			"dave":  {BaseObject: activitypub.BaseObject{ID: "https://example.com/users/dave"}, PreferredUsername: "dave"},
		}},
		relationshipRepository: &sharedInboxRelationshipRepoStub{pages: map[string]map[string]sharedInboxFollowerPage{
			"bob@remote.example": {
				"":       {followers: []string{"alice", "carol"}, nextCursor: "page-2"},
				"page-2": {followers: []string{"dave"}},
			},
		}},
		localDomain: "example.com",
	}

	activity := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			Type: activitypub.CreateType,
			To:   []string{activitypub.PublicAddress},
			CC:   []string{"https://remote.example/users/bob/followers"},
		},
		Actor: "https://remote.example/users/bob",
		Object: map[string]any{
			"id":           "https://remote.example/notes/2",
			"type":         activitypub.NoteType,
			"attributedTo": "https://remote.example/users/bob",
		},
	}

	actors, err := resolver.Resolve(context.Background(), activity)
	require.NoError(t, err)
	require.Len(t, actors, 3)
	require.ElementsMatch(t, []string{"alice", "carol", "dave"}, []string{actors[0].PreferredUsername, actors[1].PreferredUsername, actors[2].PreferredUsername})
}

func TestSharedInboxTargetResolver_AcceptUsesStoredFollowActor(t *testing.T) {
	t.Parallel()

	resolver := sharedInboxTargetResolver{
		actorRepository: &sharedInboxActorRepoStub{actors: map[string]*activitypub.Actor{
			"alice": {BaseObject: activitypub.BaseObject{ID: "https://example.com/users/alice"}, PreferredUsername: "alice"},
		}},
		activityRepository: &sharedInboxActivityRepoStub{activities: map[string]*activitypub.Activity{
			"https://example.com/activities/follow-1": {
				BaseObject: activitypub.BaseObject{Type: activitypub.FollowType},
				Actor:      "https://example.com/users/alice",
			},
		}},
		localDomain: "example.com",
	}

	activity := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{Type: activitypub.AcceptType},
		Object:     "https://example.com/activities/follow-1",
	}

	actors, err := resolver.Resolve(context.Background(), activity)
	require.NoError(t, err)
	require.Len(t, actors, 1)
	require.Equal(t, "alice", actors[0].PreferredUsername)
}

func TestSharedInboxTargetResolver_LikeUsesLocalObjectOwner(t *testing.T) {
	t.Parallel()

	resolver := sharedInboxTargetResolver{
		actorRepository: &sharedInboxActorRepoStub{actors: map[string]*activitypub.Actor{
			"alice": {BaseObject: activitypub.BaseObject{ID: "https://example.com/users/alice"}, PreferredUsername: "alice"},
		}},
		objectRepository: &sharedInboxObjectRepoStub{objects: map[string]any{
			"https://example.com/notes/1": &activitypub.Note{AttributedTo: "https://example.com/users/alice"},
		}},
		localDomain: "example.com",
	}

	activity := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{Type: activitypub.LikeType},
		Object:     "https://example.com/notes/1",
	}

	actors, err := resolver.Resolve(context.Background(), activity)
	require.NoError(t, err)
	require.Len(t, actors, 1)
	require.Equal(t, "alice", actors[0].PreferredUsername)
}

func TestSharedInboxTargetResolver_UndoUsesStoredActivityTargetsAndObjectOwner(t *testing.T) {
	t.Parallel()

	resolver := sharedInboxTargetResolver{
		actorRepository: &sharedInboxActorRepoStub{actors: map[string]*activitypub.Actor{
			"alice": {BaseObject: activitypub.BaseObject{ID: "https://example.com/users/alice"}, PreferredUsername: "alice"},
			"bob":   {BaseObject: activitypub.BaseObject{ID: "https://example.com/users/bob"}, PreferredUsername: "bob"},
			"carol": {BaseObject: activitypub.BaseObject{ID: "https://example.com/users/carol"}, PreferredUsername: "carol"},
		}},
		activityRepository: &sharedInboxActivityRepoStub{activities: map[string]*activitypub.Activity{
			"https://remote.example/activities/like-1": {
				BaseObject: activitypub.BaseObject{
					Type: activitypub.LikeType,
					To:   []string{"https://example.com/users/carol", "https://remote.example/users/zane"},
				},
				Actor:  "https://example.com/users/alice",
				Target: "https://example.com/users/bob",
			},
		}},
		objectRepository: &sharedInboxObjectRepoStub{objects: map[string]any{
			"https://remote.example/activities/like-1": &activitypub.Note{AttributedTo: "https://example.com/users/carol"},
		}},
		localDomain: "example.com",
	}

	activity := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{Type: activitypub.UndoType},
		Object:     "https://remote.example/activities/like-1",
	}

	actors, err := resolver.Resolve(context.Background(), activity)
	require.NoError(t, err)
	require.Len(t, actors, 3)
	require.ElementsMatch(t, []string{"alice", "bob", "carol"}, []string{actors[0].PreferredUsername, actors[1].PreferredUsername, actors[2].PreferredUsername})
}

func TestSharedInboxTargetResolver_AddUsesTargetAndObjectOwner(t *testing.T) {
	t.Parallel()

	resolver := sharedInboxTargetResolver{
		actorRepository: &sharedInboxActorRepoStub{actors: map[string]*activitypub.Actor{
			"alice": {BaseObject: activitypub.BaseObject{ID: "https://example.com/users/alice"}, PreferredUsername: "alice"},
			"bob":   {BaseObject: activitypub.BaseObject{ID: "https://example.com/users/bob"}, PreferredUsername: "bob"},
		}},
		objectRepository: &sharedInboxObjectRepoStub{objects: map[string]any{
			"https://remote.example/collections/entry-1": map[string]any{"attributedTo": "https://example.com/users/bob"},
		}},
		localDomain: "example.com",
	}

	activity := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{Type: activitypub.AddType},
		Target:     "https://example.com/users/alice",
		Object:     "https://remote.example/collections/entry-1",
	}

	actors, err := resolver.Resolve(context.Background(), activity)
	require.NoError(t, err)
	require.Len(t, actors, 2)
	require.ElementsMatch(t, []string{"alice", "bob"}, []string{actors[0].PreferredUsername, actors[1].PreferredUsername})
}

func TestSharedInboxTargetResolver_CreateUsesEmbeddedObjectActorAndAttributedTo(t *testing.T) {
	t.Parallel()

	resolver := sharedInboxTargetResolver{
		actorRepository: &sharedInboxActorRepoStub{actors: map[string]*activitypub.Actor{
			"alice": {BaseObject: activitypub.BaseObject{ID: "https://example.com/users/alice"}, PreferredUsername: "alice"},
			"bob":   {BaseObject: activitypub.BaseObject{ID: "https://example.com/users/bob"}, PreferredUsername: "bob"},
		}},
		localDomain: "example.com",
	}

	activity := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{Type: activitypub.CreateType},
		Object: map[string]any{
			"id":           "https://remote.example/notes/3",
			"actor":        "https://example.com/users/alice",
			"attributedTo": "https://example.com/users/bob",
		},
	}

	actors, err := resolver.Resolve(context.Background(), activity)
	require.NoError(t, err)
	require.Len(t, actors, 2)
	require.ElementsMatch(t, []string{"alice", "bob"}, []string{actors[0].PreferredUsername, actors[1].PreferredUsername})
}

func TestSharedInboxTargetResolver_HelperFunctions(t *testing.T) {
	t.Parallel()

	activity := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			To:  []string{"https://example.com/users/alice", "https://remote.example/users/zane"},
			CC:  []string{"https://example.com/users/alice", "https://example.com/users/bob"},
			BTo: []string{"https://example.com/users/carol"},
			BCC: []string{"https://example.com/users/bob"},
		},
	}

	require.ElementsMatch(t, []string{"alice", "bob", "carol"}, explicitLocalRecipients(activity, "example.com"))
	require.Equal(t, "https://remote.example/notes/4", extractObjectID(" https://remote.example/notes/4 "))
	require.Equal(t, "https://remote.example/notes/5", extractObjectID(map[string]any{"id": " https://remote.example/notes/5 "}))
	require.Empty(t, extractObjectID(42))
	require.Equal(t, "https://example.com/users/alice", extractAttributedTo(&activitypub.Note{AttributedTo: " https://example.com/users/alice "}))
	require.Equal(t, "https://example.com/users/bob", extractAttributedTo(activitypub.Note{AttributedTo: " https://example.com/users/bob "}))
	require.Equal(t, "https://example.com/users/carol", extractAttributedTo(map[string]any{"attributedTo": " https://example.com/users/carol "}))
	var nilNote *activitypub.Note
	require.Empty(t, extractAttributedTo(nilNote))
	require.Empty(t, extractAttributedTo(struct{}{}))
}

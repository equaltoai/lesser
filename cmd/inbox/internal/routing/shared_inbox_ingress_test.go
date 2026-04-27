package routing

import (
	"context"
	"encoding/json"
	stderrors "errors"
	"net/http"
	"testing"

	"github.com/equaltoai/lesser/pkg/activitypub"
	apperrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/federation"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/equaltoai/lesser/pkg/testing/inmemory"
	testingmocks "github.com/equaltoai/lesser/pkg/testing/mocks"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	dynamormmocks "github.com/theory-cloud/tabletheory/pkg/mocks"
	"go.uber.org/zap"
)

func TestInboxHandler_SharedInboxGetReturnsMethodNotAllowed(t *testing.T) {
	env := newInboxTestEnv(t)

	resp, err := env.handler.handleGetSharedInbox(newAppTheoryContext(http.MethodGet, "/inbox", map[string]string{"Host": "localhost"}, nil, nil))
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, http.StatusMethodNotAllowed, resp.Status)
}

func TestInboxHandler_SharedInboxPublicCreateResolvesFollowerTargets(t *testing.T) {
	env := newInboxTestEnv(t)
	setRunAsyncSynchronous(t)

	relationshipRepo := inmemory.NewRelationshipRepository()
	require.NoError(t, relationshipRepo.CreateRelationship(context.Background(), "alice", "bob@remote.example", env.cfg.BaseURL()+"/activities/follow-remote"))
	require.NoError(t, relationshipRepo.AcceptFollowRequest(context.Background(), "alice", "bob@remote.example"))
	env.handler.relationshipRepository = relationshipRepo
	env.handler.activityRepository = inmemory.NewActivityRepository()

	activity := map[string]any{
		"@context": activitypub.Context,
		"type":     activitypub.CreateType,
		"id":       env.cfg.BaseURL() + "/activities/shared-create-public",
		"actor":    env.remoteActorID,
		"to":       []string{activitypub.PublicAddress},
		"cc":       []string{env.remoteActorID + "/followers"},
		"object": map[string]any{
			"@context":     activitypub.Context,
			"id":           env.cfg.BaseURL() + "/objects/shared-note-public",
			"type":         activitypub.NoteType,
			"attributedTo": env.remoteActorID,
			"content":      "hello shared inbox",
			"to":           []string{activitypub.PublicAddress},
			"cc":           []string{env.remoteActorID + "/followers"},
		},
	}
	body, err := json.Marshal(activity)
	require.NoError(t, err)

	ctx := newAppTheoryContext(http.MethodPost, "/inbox", map[string]string{
		"Host":         "localhost",
		"Content-Type": "application/activity+json",
		"User-Agent":   "Mastodon/4.0.0",
	}, nil, body)
	signAppTheoryRequest(t, env, ctx, body)

	resp, err := env.handler.handlePostSharedInbox(ctx)
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, http.StatusAccepted, resp.Status)

	stored, err := env.handler.activityRepository.GetActivity(context.Background(), env.cfg.BaseURL()+"/activities/shared-create-public")
	require.NoError(t, err)
	require.Equal(t, env.remoteActorID, stored.Actor)
}

func TestInboxHandler_SharedInboxFollowersOnlyCreateResolvesFollowerTargets(t *testing.T) {
	env := newInboxTestEnv(t)
	setRunAsyncSynchronous(t)

	relationshipRepo := inmemory.NewRelationshipRepository()
	require.NoError(t, relationshipRepo.CreateRelationship(context.Background(), "alice", "bob@remote.example", env.cfg.BaseURL()+"/activities/follow-private"))
	require.NoError(t, relationshipRepo.AcceptFollowRequest(context.Background(), "alice", "bob@remote.example"))
	env.handler.relationshipRepository = relationshipRepo
	env.handler.activityRepository = inmemory.NewActivityRepository()

	activity := map[string]any{
		"@context": activitypub.Context,
		"type":     activitypub.CreateType,
		"id":       env.cfg.BaseURL() + "/activities/shared-create-private",
		"actor":    env.remoteActorID,
		"to":       []string{env.remoteActorID + "/followers"},
		"object": map[string]any{
			"@context":     activitypub.Context,
			"id":           env.cfg.BaseURL() + "/objects/shared-note-private",
			"type":         activitypub.NoteType,
			"attributedTo": env.remoteActorID,
			"content":      "hello followers only",
			"to":           []string{env.remoteActorID + "/followers"},
		},
	}
	body, err := json.Marshal(activity)
	require.NoError(t, err)

	ctx := newAppTheoryContext(http.MethodPost, "/inbox", map[string]string{
		"Host":         "localhost",
		"Content-Type": "application/activity+json",
		"User-Agent":   "Mastodon/4.0.0",
	}, nil, body)
	signAppTheoryRequest(t, env, ctx, body)

	resp, err := env.handler.handlePostSharedInbox(ctx)
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, http.StatusAccepted, resp.Status)
}

func TestInboxHandler_SharedInboxFollowResolvesTargetActorAndReusesProcessing(t *testing.T) {
	env := newInboxTestEnv(t)
	setRunAsyncSynchronous(t)

	relationshipRepo := inmemory.NewRelationshipRepository()
	env.handler.relationshipRepository = relationshipRepo
	env.handler.activityRepository = inmemory.NewActivityRepository()

	activity := map[string]any{
		"@context": activitypub.Context,
		"type":     activitypub.FollowType,
		"id":       env.cfg.BaseURL() + "/activities/shared-follow",
		"actor":    env.remoteActorID,
		"object":   env.local.ID,
		"to":       []string{env.local.ID},
	}
	body, err := json.Marshal(activity)
	require.NoError(t, err)

	ctx := newAppTheoryContext(http.MethodPost, "/inbox", map[string]string{
		"Host":         "localhost",
		"Content-Type": "application/activity+json",
		"User-Agent":   "Mastodon/4.0.0",
	}, nil, body)
	signAppTheoryRequest(t, env, ctx, body)

	resp, err := env.handler.handlePostSharedInbox(ctx)
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, http.StatusAccepted, resp.Status)

	relationship, err := relationshipRepo.GetRelationship(context.Background(), "bob@remote.example", "alice")
	require.NoError(t, err)
	require.Equal(t, "accepted", relationship.State)
}

func TestInboxHandler_SharedInboxPostReturnsNotFoundWhenNoLocalTargetsResolve(t *testing.T) {
	env := newInboxTestEnv(t)
	setRunAsyncSynchronous(t)

	env.handler.relationshipRepository = inmemory.NewRelationshipRepository()
	env.handler.activityRepository = inmemory.NewActivityRepository()

	activity := map[string]any{
		"@context": activitypub.Context,
		"type":     activitypub.CreateType,
		"id":       env.remoteActorID + "/activities/shared-create-miss",
		"actor":    env.remoteActorID,
		"to":       []string{activitypub.PublicAddress},
		"cc":       []string{env.remoteActorID + "/followers"},
		"object": map[string]any{
			"@context":     activitypub.Context,
			"id":           env.remoteActorID + "/objects/shared-note-miss",
			"type":         activitypub.NoteType,
			"attributedTo": env.remoteActorID,
			"content":      "nobody local should receive this",
			"to":           []string{activitypub.PublicAddress},
			"cc":           []string{env.remoteActorID + "/followers"},
		},
	}
	body, err := json.Marshal(activity)
	require.NoError(t, err)

	ctx := newAppTheoryContext(http.MethodPost, "/inbox", map[string]string{
		"Host":         "localhost",
		"Content-Type": "application/activity+json",
		"User-Agent":   "Mastodon/4.0.0",
	}, nil, body)
	signAppTheoryRequest(t, env, ctx, body)

	resp, err := env.handler.handlePostSharedInbox(ctx)
	require.Nil(t, resp)
	require.Error(t, err)
	require.Equal(t, http.StatusNotFound, apperrors.GetHTTPStatus(err))
}

func TestInboxHandler_SharedInboxPostReturnsValidationErrorForInvalidAddressing(t *testing.T) {
	env := newInboxTestEnv(t)
	setRunAsyncSynchronous(t)

	activity := map[string]any{
		"@context": activitypub.Context,
		"type":     activitypub.CreateType,
		"id":       env.remoteActorID + "/activities/shared-invalid-addressing",
		"actor":    env.remoteActorID,
		"to":       []string{env.local.ID},
		"cc":       []string{"not-a-url"},
		"object": map[string]any{
			"@context":     activitypub.Context,
			"id":           env.remoteActorID + "/objects/shared-invalid-addressing",
			"type":         activitypub.NoteType,
			"attributedTo": env.remoteActorID,
			"content":      "invalid shared inbox addressing",
			"to":           []string{env.local.ID},
		},
	}
	body, err := json.Marshal(activity)
	require.NoError(t, err)

	ctx := newAppTheoryContext(http.MethodPost, "/inbox", map[string]string{
		"Host":         "localhost",
		"Content-Type": "application/activity+json",
		"User-Agent":   "Mastodon/4.0.0",
	}, nil, body)
	signAppTheoryRequest(t, env, ctx, body)

	resp, err := env.handler.handlePostSharedInbox(ctx)
	require.Nil(t, resp)
	require.Error(t, err)
	require.Equal(t, http.StatusBadRequest, apperrors.GetHTTPStatus(err))
}

func TestInboxHandler_ValidateSharedInboxAddressingAndPrivacy_Branches(t *testing.T) {
	env := newInboxTestEnv(t)

	t.Run("requires resolved targets", func(t *testing.T) {
		req := &InboxRequest{
			Activity: &activitypub.Activity{
				BaseObject: activitypub.BaseObject{
					Type: activitypub.CreateType,
					To:   []string{activitypub.PublicAddress},
				},
				Actor: env.remoteActorID,
				Object: &activitypub.Note{
					BaseObject: activitypub.BaseObject{
						ID:   env.cfg.BaseURL() + "/objects/no-targets",
						Type: activitypub.NoteType,
						To:   []string{activitypub.PublicAddress},
					},
					Content:      "no local targets",
					AttributedTo: env.remoteActorID,
				},
			},
			Actor:      env.local,
			CostParams: &federation.CostCalculationParams{},
		}

		err := env.handler.validateSharedInboxAddressingAndPrivacy(req)
		require.Error(t, err)
		require.Equal(t, http.StatusNotFound, apperrors.GetHTTPStatus(err))
	})

	t.Run("runs direct message validation", func(t *testing.T) {
		req := &InboxRequest{
			Activity: &activitypub.Activity{
				BaseObject: activitypub.BaseObject{
					Type: activitypub.CreateType,
					To:   []string{"https://example.com/users/alice/following"},
				},
				Actor: env.remoteActorID,
				Object: &activitypub.Note{
					BaseObject: activitypub.BaseObject{
						ID:   env.cfg.BaseURL() + "/objects/dm-validation",
						Type: activitypub.NoteType,
						To:   []string{"https://example.com/users/alice/following"},
					},
					Content:      "not a direct recipient",
					AttributedTo: env.remoteActorID,
				},
			},
			Actor:        env.local,
			TargetActors: []*activitypub.Actor{env.local},
			CostParams:   &federation.CostCalculationParams{},
		}

		err := env.handler.validateSharedInboxAddressingAndPrivacy(req)
		require.Error(t, err)
		require.Equal(t, http.StatusBadRequest, apperrors.GetHTTPStatus(err))
	})
}

func TestInboxHandler_InitializeActorInboxRequest_RequiresUsername(t *testing.T) {
	env := newInboxTestEnv(t)

	ctx := newAppTheoryContext(http.MethodPost, "/users//inbox", map[string]string{
		"Host":         "localhost",
		"Content-Type": "application/activity+json",
	}, nil, nil)

	req, err := env.handler.initializeActorInboxRequest(ctx)
	require.Nil(t, req)
	require.Error(t, err)
	require.Equal(t, http.StatusBadRequest, apperrors.GetHTTPStatus(err))
}

func TestInboxHandler_ValidateResolvedTargetActors_RejectsInvalidActor(t *testing.T) {
	env := newInboxTestEnv(t)

	err := env.handler.validateResolvedTargetActors(&activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			Type: activitypub.FollowType,
			To:   []string{env.local.ID},
		},
		Actor: env.remoteActorID,
	}, []*activitypub.Actor{{PreferredUsername: "alice"}}, false)
	require.Error(t, err)
	require.Equal(t, http.StatusBadRequest, apperrors.GetHTTPStatus(err))
}

func TestInboxHandler_InitializeActorInboxRequest_ReturnsNotFoundForMissingActor(t *testing.T) {
	env := newInboxTestEnv(t)

	actorRepo := testingmocks.NewMockActorRepository()
	env.handler.actorRepository = actorRepo
	actorRepo.On("GetActorByUsername", mock.Anything, "missing").Return(nil, stderrors.New(actorNotFoundError)).Once()

	activity := map[string]any{
		"@context": activitypub.Context,
		"type":     activitypub.FollowType,
		"id":       env.remoteActorID + "/activities/follow-missing-actor",
		"actor":    env.remoteActorID,
		"object":   env.cfg.ActorURL("missing"),
		"to":       []string{env.cfg.ActorURL("missing")},
	}
	body, err := json.Marshal(activity)
	require.NoError(t, err)

	ctx := newAppTheoryContext(http.MethodPost, "/users/missing/inbox", map[string]string{
		"Host":         "localhost",
		"Content-Type": "application/activity+json",
		"User-Agent":   "Mastodon/4.0.0",
	}, nil, body)
	ctx.Params["username"] = "missing"
	signAppTheoryRequest(t, env, ctx, body)

	req, err := env.handler.initializeActorInboxRequest(ctx)
	require.Nil(t, req)
	require.Error(t, err)
	require.Equal(t, http.StatusNotFound, apperrors.GetHTTPStatus(err))
	actorRepo.AssertExpectations(t)
}

func TestInboxHandler_InitializeActorInboxRequest_RejectsLockedBootstrapActor(t *testing.T) {
	env := newInboxTestEnv(t)
	env.handler.instanceRepository = newInstanceRepositoryWithState(&storagemodels.InstanceState{
		Locked:            true,
		BootstrapUsername: "steward",
	}, nil)

	ctx := newAppTheoryContext(http.MethodPost, "/users/steward/inbox", map[string]string{
		"Host":         "localhost",
		"Content-Type": "application/activity+json",
	}, nil, nil)
	ctx.Params["username"] = "steward"

	req, err := env.handler.initializeActorInboxRequest(ctx)
	require.Nil(t, req)
	require.Error(t, err)
	require.Equal(t, http.StatusForbidden, apperrors.GetHTTPStatus(err))
}

func TestInboxHandler_InitializeSharedInboxRequest_RejectsInvalidResolvedTargetActor(t *testing.T) {
	env := newInboxTestEnv(t)

	relationshipRepo := inmemory.NewRelationshipRepository()
	require.NoError(t, relationshipRepo.CreateRelationship(context.Background(), "alice", "bob@remote.example", env.cfg.BaseURL()+"/activities/follow-shared-invalid"))
	require.NoError(t, relationshipRepo.AcceptFollowRequest(context.Background(), "alice", "bob@remote.example"))
	env.handler.relationshipRepository = relationshipRepo

	actorRepo := testingmocks.NewMockActorRepository()
	env.handler.actorRepository = actorRepo
	actorRepo.On("GetActorByUsername", mock.Anything, "alice").Return(&activitypub.Actor{PreferredUsername: "alice"}, nil).Twice()

	activity := map[string]any{
		"@context": activitypub.Context,
		"type":     activitypub.CreateType,
		"id":       env.remoteActorID + "/activities/shared-invalid-target",
		"actor":    env.remoteActorID,
		"to":       []string{activitypub.PublicAddress},
		"cc":       []string{env.remoteActorID + "/followers"},
		"object": map[string]any{
			"@context":     activitypub.Context,
			"id":           env.remoteActorID + "/objects/shared-invalid-target",
			"type":         activitypub.NoteType,
			"attributedTo": env.remoteActorID,
			"content":      "shared inbox invalid target actor",
			"to":           []string{activitypub.PublicAddress},
			"cc":           []string{env.remoteActorID + "/followers"},
		},
	}
	body, err := json.Marshal(activity)
	require.NoError(t, err)

	ctx := newAppTheoryContext(http.MethodPost, "/inbox", map[string]string{
		"Host":         "localhost",
		"Content-Type": "application/activity+json",
		"User-Agent":   "Mastodon/4.0.0",
	}, nil, body)
	signAppTheoryRequest(t, env, ctx, body)

	req, err := env.handler.initializeSharedInboxRequest(ctx)
	require.Nil(t, req)
	require.Error(t, err)
	require.Equal(t, http.StatusBadRequest, apperrors.GetHTTPStatus(err))
	actorRepo.AssertExpectations(t)
}

func TestInboxHandler_RejectLockedBootstrapActor_Branches(t *testing.T) {
	env := newInboxTestEnv(t)

	t.Run("locked bootstrap is forbidden", func(t *testing.T) {
		env.handler.instanceRepository = newInstanceRepositoryWithState(&storagemodels.InstanceState{
			Locked:            true,
			BootstrapUsername: "steward",
		}, nil)

		err := env.handler.rejectLockedBootstrapActor(context.Background(), "steward")
		require.Error(t, err)
		require.Equal(t, http.StatusForbidden, apperrors.GetHTTPStatus(err))
	})

	t.Run("repository error still protects default bootstrap actor", func(t *testing.T) {
		env.handler.instanceRepository = newInstanceRepositoryWithState(nil, stderrors.New("boom"))

		err := env.handler.rejectLockedBootstrapActor(context.Background(), storagemodels.DefaultBootstrapUsername)
		require.Error(t, err)
		require.Equal(t, http.StatusForbidden, apperrors.GetHTTPStatus(err))
	})
}

func TestInboxHandler_FilterBootstrapTargets_FiltersLockedBootstrap(t *testing.T) {
	env := newInboxTestEnv(t)
	env.handler.instanceRepository = newInstanceRepositoryWithState(&storagemodels.InstanceState{
		Locked:            true,
		BootstrapUsername: "steward",
	}, nil)

	filtered, err := env.handler.filterBootstrapTargets(context.Background(), []*activitypub.Actor{
		{PreferredUsername: "steward"},
		{PreferredUsername: "alice"},
		nil,
	})
	require.NoError(t, err)
	require.Len(t, filtered, 1)
	require.Equal(t, "alice", filtered[0].PreferredUsername)
}

func newInstanceRepositoryWithState(state *storagemodels.InstanceState, err error) *repositories.InstanceRepository {
	db := new(dynamormmocks.MockDB)
	q := new(dynamormmocks.MockQuery)

	db.On("WithContext", mock.Anything).Return(db).Maybe()
	db.On("Model", mock.Anything).Return(q).Maybe()
	q.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(q).Maybe()

	first := q.On("First", mock.AnythingOfType("*models.InstanceState"))
	if err != nil {
		first.Return(err).Maybe()
	} else {
		first.Run(func(args mock.Arguments) {
			current := args.Get(0).(*storagemodels.InstanceState)
			*current = *state
		}).Return(nil).Maybe()
	}

	return repositories.NewInstanceRepository(db, "test-table", zap.NewNop())
}

type sharedInboxResolverActorRepo struct {
	actors map[string]*activitypub.Actor
}

func (r sharedInboxResolverActorRepo) GetActorByUsername(_ context.Context, username string) (*activitypub.Actor, error) {
	actor := r.actors[username]
	if actor == nil {
		return nil, stderrors.New(actorNotFoundError)
	}
	return actor, nil
}

type sharedInboxResolverRelationshipRepo struct {
	followers map[string][]string
	deleted   [][2]string
}

func (r *sharedInboxResolverRelationshipRepo) GetFollowers(_ context.Context, username string, _ int, _ string) ([]string, string, error) {
	return append([]string(nil), r.followers[username]...), "", nil
}

func (r *sharedInboxResolverRelationshipRepo) DeleteRelationship(_ context.Context, followerUsername, followingUsername string) error {
	r.deleted = append(r.deleted, [2]string{followerUsername, followingUsername})
	return nil
}

func TestSharedInboxTargetResolver_FollowerCollectionsRequireActorOwnership(t *testing.T) {
	resolver := sharedInboxTargetResolver{
		actorRepository: sharedInboxResolverActorRepo{actors: map[string]*activitypub.Actor{
			"alice": {
				BaseObject:        activitypub.BaseObject{ID: "https://local.example/users/alice", Type: activitypub.PersonType},
				PreferredUsername: "alice",
				Inbox:             "https://local.example/users/alice/inbox",
				Outbox:            "https://local.example/users/alice/outbox",
			},
		}},
		relationshipRepository: &sharedInboxResolverRelationshipRepo{followers: map[string][]string{
			"bob@remote.example":     {"alice"},
			"mallory@remote.example": {"alice"},
		}},
		localDomain: "local.example",
	}

	actors, err := resolver.Resolve(context.Background(), &activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			Type: activitypub.CreateType,
			ID:   "https://remote.example/activities/1",
			To:   []string{"https://mallory.remote.example/users/mallory/followers"},
		},
		Actor: "https://remote.example/users/bob",
		Object: map[string]any{
			"id":           "https://remote.example/objects/1",
			"type":         activitypub.NoteType,
			"attributedTo": "https://remote.example/users/bob",
		},
	})
	require.NoError(t, err)
	require.Empty(t, actors)

	actors, err = resolver.Resolve(context.Background(), &activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			Type: activitypub.CreateType,
			ID:   "https://remote.example/activities/2",
			To:   []string{"https://remote.example/users/bob/followers"},
		},
		Actor: "https://remote.example/users/bob",
		Object: map[string]any{
			"id":           "https://remote.example/objects/2",
			"type":         activitypub.NoteType,
			"attributedTo": "https://remote.example/users/bob",
		},
	})
	require.NoError(t, err)
	require.Len(t, actors, 1)
	require.Equal(t, "alice", actors[0].PreferredUsername)
}

func TestSharedInboxTargetResolver_CleansStaleFollowerClaims(t *testing.T) {
	relationshipRepo := &sharedInboxResolverRelationshipRepo{followers: map[string][]string{
		"bob@remote.example": {"missing"},
	}}
	resolver := sharedInboxTargetResolver{
		actorRepository:        sharedInboxResolverActorRepo{actors: map[string]*activitypub.Actor{}},
		relationshipRepository: relationshipRepo,
		localDomain:            "local.example",
	}

	actors, err := resolver.Resolve(context.Background(), &activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			Type: activitypub.CreateType,
			ID:   "https://remote.example/activities/3",
			To:   []string{"https://remote.example/users/bob/followers"},
		},
		Actor: "https://remote.example/users/bob",
		Object: map[string]any{
			"id":           "https://remote.example/objects/3",
			"type":         activitypub.NoteType,
			"attributedTo": "https://remote.example/users/bob",
		},
	})
	require.NoError(t, err)
	require.Empty(t, actors)
	require.Equal(t, [][2]string{{"missing", "bob@remote.example"}}, relationshipRepo.deleted)
}

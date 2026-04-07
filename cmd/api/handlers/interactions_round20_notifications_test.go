package handlers

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/services/notes"
	"github.com/equaltoai/lesser/pkg/services/notifications"
	"github.com/equaltoai/lesser/pkg/services/relationships"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestInteractionsRound20_HandleFollowLift_CreatesNotification(t *testing.T) {
	cfg := round11TestConfig()
	now := time.Now()
	state := &round10QueryState{
		usersByUsername: map[string]storagemodels.User{
			"bob": {PK: "USER#bob", SK: storagemodels.SKMetadata, Username: "bob", Role: "user", Approved: true, Version: 1, CreatedAt: now.Add(-time.Hour)},
		},
		actorsByUser: map[string]storagemodels.Actor{
			"bob": {Username: "bob", Actor: &activitypub.Actor{BaseObject: activitypub.BaseObject{ID: cfg.ActorURL("bob"), Type: "Person"}, PreferredUsername: "bob"}},
		},
	}

	var created *notifications.CreateNotificationCommand
	registry := &RegistryStub{
		RelationshipsSvc: &RelationshipsServiceStub{
			FollowFunc: func(_ context.Context, cmd *relationships.FollowCommand) (*relationships.FollowResult, error) {
				require.Equal(t, "alice", cmd.FollowerID)
				require.Equal(t, cfg.ActorURL("bob"), cmd.FollowingID)
				return &relationships.FollowResult{
					Relationship: &relationships.RelationshipData{ID: cmd.FollowingID, Following: true},
					IsFollowing:  true,
				}, nil
			},
		},
		NotificationsSvc: &NotificationsServiceStub{
			CreateNotificationFunc: func(_ context.Context, cmd *notifications.CreateNotificationCommand) (*notifications.NotificationResult, error) {
				created = cmd
				return &notifications.NotificationResult{}, nil
			},
		},
	}

	h, _, _ := round11NewHandler(t, cfg, state, registry)
	token := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeWrite})
	ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/accounts/bob/follow", map[string]string{"Authorization": "Bearer " + token}, nil, nil)
	require.NoError(t, err)
	ctx.Params["id"] = "bob"

	requireStatus(t, http.StatusOK)(h.HandleFollowLift(ctx))
	require.NotNil(t, created)
	require.Equal(t, "bob", created.UserID)
	require.Equal(t, "alice", created.ActorID)
	require.Equal(t, common.NotificationTypeFollow, created.Type)
}

func TestInteractionsRound20_HandleFavoriteLift_CreatesNotification(t *testing.T) {
	cfg := round11TestConfig()

	var created *notifications.CreateNotificationCommand
	registry := &RegistryStub{
		NotesSvc: &NotesServiceStub{
			LikeNoteFunc: func(_ context.Context, cmd *notes.LikeNoteCommand) (*notes.LikeResult, error) {
				return &notes.LikeResult{
					Status: &storagemodels.Status{
						StatusID:       cmd.StatusID,
						AuthorUsername: "bob",
						AuthorID:       cfg.ActorURL("bob"),
						Content:        "hello world",
						PublishedAt:    time.Now().UTC(),
						CreatedAt:      time.Now().UTC(),
					},
				}, nil
			},
		},
		NotificationsSvc: &NotificationsServiceStub{
			CreateNotificationFunc: func(_ context.Context, cmd *notifications.CreateNotificationCommand) (*notifications.NotificationResult, error) {
				created = cmd
				return &notifications.NotificationResult{}, nil
			},
		},
	}

	h, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, registry)
	token := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeWrite})
	ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses/status-1/favourite", map[string]string{"Authorization": "Bearer " + token}, nil, nil)
	require.NoError(t, err)
	ctx.Params["id"] = "status-1"

	requireStatus(t, http.StatusOK)(h.HandleFavoriteLift(ctx))
	require.NotNil(t, created)
	require.Equal(t, "bob", created.UserID)
	require.Equal(t, "alice", created.ActorID)
	require.Equal(t, common.NotificationTypeFavourite, created.Type)
	require.Equal(t, "status-1", created.TargetID)
}

func TestInteractionsRound20_ResolveRelationshipTarget_PopulatesDerivedFields(t *testing.T) {
	cfg := round11TestConfig()
	now := time.Now()
	state := &round10QueryState{
		usersByUsername: map[string]storagemodels.User{
			"bob": {PK: "USER#bob", SK: storagemodels.SKMetadata, Username: "bob", Role: "user", Approved: true, Version: 1, CreatedAt: now.Add(-time.Hour)},
		},
		actorsByUser: map[string]storagemodels.Actor{
			"bob": {Username: "bob", Actor: &activitypub.Actor{BaseObject: activitypub.BaseObject{ID: cfg.ActorURL("bob"), Type: "Person"}, PreferredUsername: "bob"}},
		},
	}

	h, _, _ := round11NewHandler(t, cfg, state)
	targetID, publicID, username, err := h.resolveRelationshipTarget(context.Background(), "bob")
	require.NoError(t, err)
	require.Equal(t, cfg.ActorURL("bob"), targetID)
	require.Equal(t, common.GenerateNumericID("bob"), publicID)
	require.Equal(t, "bob", username)
}

func TestInteractionsRound20_RelationshipMutationHelpers(t *testing.T) {
	t.Run("success paths", func(t *testing.T) {
		h := &Handler{
			registry: &RegistryStub{
				RelationshipsSvc: &RelationshipsServiceStub{
					UnfollowFunc: func(_ context.Context, cmd *relationships.UnfollowCommand) (*relationships.RelationshipResult, error) {
						return &relationships.RelationshipResult{Relationship: &relationships.RelationshipData{ID: cmd.FollowingID}}, nil
					},
					BlockFunc: func(_ context.Context, cmd *relationships.BlockCommand) (*relationships.RelationshipResult, error) {
						return &relationships.RelationshipResult{Relationship: &relationships.RelationshipData{ID: cmd.BlockedID, Blocking: true}}, nil
					},
					UnblockFunc: func(_ context.Context, cmd *relationships.UnblockCommand) (*relationships.RelationshipResult, error) {
						return &relationships.RelationshipResult{Relationship: &relationships.RelationshipData{ID: cmd.BlockedID}}, nil
					},
				},
			},
		}

		relationship, err := h.handleUnfollow(context.Background(), "alice", "bob")
		require.NoError(t, err)
		require.Equal(t, "bob", relationship.ID)

		relationship, err = h.handleBlock(context.Background(), "alice", "bob")
		require.NoError(t, err)
		require.True(t, relationship.Blocking)

		relationship, err = h.handleUnblock(context.Background(), "alice", "bob")
		require.NoError(t, err)
		require.Equal(t, "bob", relationship.ID)
	})

	t.Run("error paths", func(t *testing.T) {
		h := &Handler{
			registry: &RegistryStub{
				RelationshipsSvc: &RelationshipsServiceStub{
					UnfollowFunc: func(context.Context, *relationships.UnfollowCommand) (*relationships.RelationshipResult, error) {
						return nil, errors.New("unfollow failed")
					},
					BlockFunc: func(context.Context, *relationships.BlockCommand) (*relationships.RelationshipResult, error) {
						return nil, errors.New("boom")
					},
					UnblockFunc: func(context.Context, *relationships.UnblockCommand) (*relationships.RelationshipResult, error) {
						return nil, errors.New("boom")
					},
				},
			},
		}

		_, err := h.handleUnfollow(context.Background(), "alice", "bob")
		require.Error(t, err)

		_, err = h.handleBlock(context.Background(), "alice", "bob")
		require.Error(t, err)

		_, err = h.handleUnblock(context.Background(), "alice", "bob")
		require.Error(t, err)
	})
}

func TestInteractionsRound20_InteractionHelperCoverage(t *testing.T) {
	t.Run("resolve relationship target id canonicalizes local preferred username rows", func(t *testing.T) {
		cfg := round11TestConfig()
		state := &round10QueryState{
			usersByUsername: map[string]storagemodels.User{
				"bob": {PK: "USER#bob", SK: storagemodels.SKMetadata, Username: "bob", Role: "user", Approved: true, Version: 1, CreatedAt: time.Now().Add(-time.Hour)},
			},
			actorsByUser: map[string]storagemodels.Actor{
				"bob": {Username: "bob", Actor: &activitypub.Actor{BaseObject: activitypub.BaseObject{Type: "Person"}, PreferredUsername: "bob"}},
			},
		}

		h, _, _ := round11NewHandler(t, cfg, state)
		targetID, err := h.resolveRelationshipTargetID(context.Background(), "bob")
		require.NoError(t, err)
		require.Equal(t, cfg.ActorURL("bob"), targetID)
	})

	t.Run("relationship target not found detection covers not found variants", func(t *testing.T) {
		require.False(t, relationshipTargetNotFound(nil))
		require.True(t, relationshipTargetNotFound(common.ActorNotFoundError{Username: "bob"}))
		require.True(t, relationshipTargetNotFound(errors.New("actor not found: bob")))
	})

	t.Run("status interaction failure messages cover all operations", func(t *testing.T) {
		require.Equal(t, "failed to like status", statusInteractionFailureMessage(statusOpFavorite))
		require.Equal(t, "failed to unlike status", statusInteractionFailureMessage(statusOpUnfavorite))
		require.Equal(t, "failed to reblog status", statusInteractionFailureMessage(statusOpReblog))
		require.Equal(t, "failed to unreblog status", statusInteractionFailureMessage(statusOpUnreblog))
		require.Equal(t, "failed to update status", statusInteractionFailureMessage("unknown"))
	})

	t.Run("extract username from actor id supports supported formats", func(t *testing.T) {
		require.Equal(t, "alice", extractUsernameFromActorID("https://example.com/users/alice"))
		require.Equal(t, "bob", extractUsernameFromActorID("https://example.com/@bob"))
		require.Equal(t, "carol", extractUsernameFromActorID("carol"))
		require.Equal(t, "", extractUsernameFromActorID("  "))
	})

	t.Run("favorite notification falls back to author id username", func(t *testing.T) {
		var created *notifications.CreateNotificationCommand
		h := &Handler{
			registry: &RegistryStub{
				NotificationsSvc: &NotificationsServiceStub{
					CreateNotificationFunc: func(_ context.Context, cmd *notifications.CreateNotificationCommand) (*notifications.NotificationResult, error) {
						created = cmd
						return &notifications.NotificationResult{}, nil
					},
				},
			},
		}

		h.createStatusInteractionNotification(context.Background(), statusOpFavorite, "alice", &storagemodels.Status{
			StatusID: "status-2",
			AuthorID: "https://example.com/users/bob",
		})

		require.NotNil(t, created)
		require.Equal(t, "bob", created.UserID)
		require.Equal(t, common.NotificationTypeFavourite, created.Type)
		require.Equal(t, "status-2", created.TargetID)
	})
}

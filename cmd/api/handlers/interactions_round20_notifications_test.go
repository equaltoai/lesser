package handlers

import (
	"context"
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

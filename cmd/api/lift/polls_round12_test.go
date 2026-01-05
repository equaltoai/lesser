package lift

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/services/notifications"
	"github.com/equaltoai/lesser/pkg/storage"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestPollHandlersRound12(t *testing.T) {
	cfg := round11TestConfig()
	now := time.Now().UTC()

	voterActorID := "https://example.com/users/alice"
	creatorActorID := "https://example.com/users/bob"

	pollID := "poll-1"
	poll2ID := "poll-2"
	pollModel := storagemodels.Poll{
		ID:          pollID,
		StatusID:    "status-1",
		CreatedBy:   creatorActorID,
		Options:     []string{"a :smile:", "b"},
		Multiple:    false,
		HideTotals:  true,
		ExpiresAt:   now.Add(1 * time.Hour),
		CreatedAt:   now.Add(-10 * time.Minute),
		UpdatedAt:   now.Add(-1 * time.Minute),
		VotesCount:  0,
		VotersCount: 0,
		Votes:       map[string][]int{},
	}
	_ = pollModel.UpdateKeys()

	poll2 := pollModel
	poll2.ID = poll2ID
	poll2.StatusID = "status-2"
	_ = poll2.UpdateKeys()

	voteModel := storagemodels.PollVote{
		VoterID: voterActorID,
		Choices: []int{0},
		VotedAt: now.Add(-2 * time.Minute),
	}
	voteModel.SetPollID(pollID)

	state := &round10QueryState{
		pollsByID: map[string]storagemodels.Poll{
			pollID:  pollModel,
			poll2ID: poll2,
		},
		pollVotesByKey: map[string]storagemodels.PollVote{
			voteModel.PK + "#" + voteModel.SK: voteModel,
		},
		notFoundPKSK: map[string]bool{
			"POLL#" + poll2ID + "#VOTE#" + voterActorID: true,
		},
	}

	accountsSvc := &AccountsServiceStub{
		GetAccountFunc: func(_ context.Context, username string) (*storage.Account, error) {
			require.Equal(t, "alice", username)
			return &storage.Account{
				Actor: &activitypub.Actor{
					BaseObject: activitypub.BaseObject{
						ID:   voterActorID,
						Type: "Person",
					},
					PreferredUsername: "alice",
					Name:              "Alice",
				},
			}, nil
		},
	}

	notificationCalls := 0
	notifSvc := &NotificationsServiceStub{
		CreateNotificationFunc: func(_ context.Context, cmd *notifications.CreateNotificationCommand) (*notifications.NotificationResult, error) {
			notificationCalls++
			require.Equal(t, "poll", cmd.Type)
			require.Equal(t, "bob", cmd.UserID)
			require.Equal(t, voterActorID, cmd.ActorID)
			require.NotEmpty(t, cmd.TargetID)
			return &notifications.NotificationResult{}, nil
		},
	}

	reg := &RegistryStub{
		AccountsSvc:      accountsSvc,
		NotificationsSvc: notifSvc,
	}

	h, _, _ := round11NewHandler(t, cfg, state, reg)

	readToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeRead})
	writeToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeRead, auth.ScopeWrite})

	t.Run("get poll requires id", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/polls/", nil, nil, nil)
		require.NoError(t, err)
		require.NoError(t, h.HandleGetPollLift(ctx))
		require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)
	})

	t.Run("get poll public (hide totals)", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/polls/"+pollID, nil, nil, nil)
		require.NoError(t, err)
		ctx.SetParam("id", pollID)
		require.NoError(t, h.HandleGetPollLift(ctx))
		require.Equal(t, http.StatusOK, ctx.Response.StatusCode)
	})

	t.Run("get poll with auth shows voted", func(t *testing.T) {
		headers := map[string]string{"Authorization": "Bearer " + readToken}
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/polls/"+pollID, headers, nil, nil)
		require.NoError(t, err)
		ctx.SetParam("id", pollID)
		require.NoError(t, h.HandleGetPollLift(ctx))
		require.Equal(t, http.StatusOK, ctx.Response.StatusCode)
	})

	t.Run("vote requires auth header", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/polls/"+pollID+"/votes", nil, nil, nil)
		require.NoError(t, err)
		ctx.SetParam("id", pollID)
		require.NoError(t, h.HandleVoteOnPollLift(ctx))
		require.Equal(t, http.StatusUnauthorized, ctx.Response.StatusCode)
	})

	t.Run("vote requires request body", func(t *testing.T) {
		headers := map[string]string{"Authorization": "Bearer " + writeToken}
		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v1/polls/"+pollID+"/votes", headers, nil, nil)
		ctx.SetParam("id", pollID)
		require.NoError(t, h.HandleVoteOnPollLift(ctx))
		require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)
	})

	t.Run("vote requires choices", func(t *testing.T) {
		headers := map[string]string{"Authorization": "Bearer " + writeToken}
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/polls/"+pollID+"/votes", headers, nil, map[string]any{"choices": []int{}})
		require.NoError(t, err)
		ctx.SetParam("id", pollID)
		require.NoError(t, h.HandleVoteOnPollLift(ctx))
		require.Equal(t, http.StatusUnprocessableEntity, ctx.Response.StatusCode)
	})

	t.Run("vote requires write scope", func(t *testing.T) {
		headers := map[string]string{"Authorization": "Bearer " + readToken}
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/polls/"+pollID+"/votes", headers, nil, map[string]any{"choices": []int{0}})
		require.NoError(t, err)
		ctx.SetParam("id", pollID)
		require.NoError(t, h.HandleVoteOnPollLift(ctx))
		require.Equal(t, http.StatusForbidden, ctx.Response.StatusCode)
	})

	t.Run("vote invalid json", func(t *testing.T) {
		headers := map[string]string{"Authorization": "Bearer " + writeToken}
		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v1/polls/"+pollID+"/votes", headers, nil, []byte("{"))
		ctx.SetParam("id", pollID)
		require.NoError(t, h.HandleVoteOnPollLift(ctx))
		require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)
	})

	t.Run("vote choice negative", func(t *testing.T) {
		headers := map[string]string{"Authorization": "Bearer " + writeToken}
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/polls/"+pollID+"/votes", headers, nil, map[string]any{"choices": []int{-1}})
		require.NoError(t, err)
		ctx.SetParam("id", pollID)
		require.NoError(t, h.HandleVoteOnPollLift(ctx))
		require.Equal(t, http.StatusUnprocessableEntity, ctx.Response.StatusCode)
	})

	t.Run("vote already voted", func(t *testing.T) {
		headers := map[string]string{"Authorization": "Bearer " + writeToken}
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/polls/"+pollID+"/votes", headers, nil, map[string]any{"choices": []int{0}})
		require.NoError(t, err)
		ctx.SetParam("id", pollID)
		require.NoError(t, h.HandleVoteOnPollLift(ctx))
		require.Equal(t, http.StatusUnprocessableEntity, ctx.Response.StatusCode)
	})

	t.Run("vote success creates notification", func(t *testing.T) {
		headers := map[string]string{"Authorization": "Bearer " + writeToken}
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/polls/"+poll2ID+"/votes", headers, nil, map[string]any{"choices": []int{0}})
		require.NoError(t, err)
		ctx.SetParam("id", poll2ID)
		require.NoError(t, h.HandleVoteOnPollLift(ctx))
		require.Equal(t, http.StatusOK, ctx.Response.StatusCode)
		require.GreaterOrEqual(t, notificationCalls, 1)
	})
}

func TestPollHelpersRound12(t *testing.T) {
	require.True(t, isValidEmojiChar('a'))
	require.True(t, isValidEmojiChar('Z'))
	require.True(t, isValidEmojiChar('9'))
	require.True(t, isValidEmojiChar('_'))
	require.False(t, isValidEmojiChar('-'))

	require.True(t, isValidEmojiCodeLift("ok"))
	require.False(t, isValidEmojiCodeLift("x"))
	require.False(t, isValidEmojiCodeLift("bad!"))
	require.False(t, isValidEmojiCodeLift("a_b-c"))

	handler, _, _ := round11NewHandler(t, round11TestConfig(), &round10QueryState{})
	require.ElementsMatch(t, []string{"smile", "good_emoji"}, handler.findEmojiCodesLift("hi :smile: and :good_emoji: :bad!:"))

	require.Equal(t, "alice", extractUsernameFromActorIDLift("https://example.com/users/alice"))
	require.Equal(t, "bob", extractUsernameFromActorIDLift("bob"))

	s := generateRandomStringLift()
	require.Len(t, s, 8)
}

func TestPollEdgeCasesRound12(t *testing.T) {
	cfg := round11TestConfig()
	now := time.Now().UTC()

	t.Run("get poll not found", func(t *testing.T) {
		state := &round10QueryState{
			notFoundPKSK: map[string]bool{
				"POLL#missing#" + storagemodels.SKMetadata: true,
			},
		}
		h, _, _ := round11NewHandler(t, cfg, state)

		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/polls/missing", nil, nil, nil)
		require.NoError(t, err)
		ctx.SetParam("id", "missing")
		require.NoError(t, h.HandleGetPollLift(ctx))
		require.Equal(t, http.StatusNotFound, ctx.Response.StatusCode)
	})

	t.Run("get poll expired does not hide totals", func(t *testing.T) {
		pollID := "expired"
		poll := storagemodels.Poll{
			ID:          pollID,
			StatusID:    "status-expired",
			CreatedBy:   "https://example.com/users/bob",
			Options:     []string{"a", "b"},
			Multiple:    false,
			HideTotals:  true,
			ExpiresAt:   now.Add(-1 * time.Hour),
			CreatedAt:   now.Add(-2 * time.Hour),
			UpdatedAt:   now.Add(-90 * time.Minute),
			VotesCount:  0,
			VotersCount: 0,
			Votes:       map[string][]int{"https://example.com/users/alice": {0}},
		}
		_ = poll.UpdateKeys()
		state := &round10QueryState{
			pollsByID: map[string]storagemodels.Poll{
				pollID: poll,
			},
		}
		h, _, _ := round11NewHandler(t, cfg, state)

		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/polls/"+pollID, nil, nil, nil)
		require.NoError(t, err)
		ctx.SetParam("id", pollID)
		require.NoError(t, h.HandleGetPollLift(ctx))
		require.Equal(t, http.StatusOK, ctx.Response.StatusCode)
	})

	t.Run("vote invalid token", func(t *testing.T) {
		pollID := "poll-invalid-token"
		poll := storagemodels.Poll{
			ID:          pollID,
			StatusID:    "status",
			CreatedBy:   "https://example.com/users/bob",
			Options:     []string{"a", "b"},
			Multiple:    false,
			HideTotals:  false,
			ExpiresAt:   now.Add(1 * time.Hour),
			CreatedAt:   now.Add(-2 * time.Hour),
			UpdatedAt:   now.Add(-90 * time.Minute),
			VotesCount:  0,
			VotersCount: 0,
			Votes:       map[string][]int{},
		}
		_ = poll.UpdateKeys()
		state := &round10QueryState{
			pollsByID: map[string]storagemodels.Poll{pollID: poll},
		}

		reg := &RegistryStub{
			AccountsSvc: &AccountsServiceStub{
				GetAccountFunc: func(_ context.Context, _ string) (*storage.Account, error) { return &storage.Account{}, nil },
			},
		}
		h, _, _ := round11NewHandler(t, cfg, state, reg)

		headers := map[string]string{"Authorization": "Bearer bad-token"}
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/polls/"+pollID+"/votes", headers, nil, map[string]any{"choices": []int{0}})
		require.NoError(t, err)
		ctx.SetParam("id", pollID)
		require.NoError(t, h.HandleVoteOnPollLift(ctx))
		require.Equal(t, http.StatusUnauthorized, ctx.Response.StatusCode)
	})

	t.Run("vote actor missing", func(t *testing.T) {
		pollID := "poll-actor-missing"
		poll := storagemodels.Poll{
			ID:          pollID,
			StatusID:    "status",
			CreatedBy:   "https://example.com/users/bob",
			Options:     []string{"a", "b"},
			Multiple:    false,
			HideTotals:  false,
			ExpiresAt:   now.Add(1 * time.Hour),
			CreatedAt:   now.Add(-2 * time.Hour),
			UpdatedAt:   now.Add(-90 * time.Minute),
			VotesCount:  0,
			VotersCount: 0,
			Votes:       map[string][]int{},
		}
		_ = poll.UpdateKeys()

		state := &round10QueryState{
			pollsByID: map[string]storagemodels.Poll{pollID: poll},
		}
		reg := &RegistryStub{
			AccountsSvc: &AccountsServiceStub{
				GetAccountFunc: func(_ context.Context, _ string) (*storage.Account, error) {
					return &storage.Account{}, nil
				},
			},
		}
		h, _, _ := round11NewHandler(t, cfg, state, reg)

		token := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeWrite})
		headers := map[string]string{"Authorization": "Bearer " + token}
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/polls/"+pollID+"/votes", headers, nil, map[string]any{"choices": []int{0}})
		require.NoError(t, err)
		ctx.SetParam("id", pollID)
		require.NoError(t, h.HandleVoteOnPollLift(ctx))
		require.Equal(t, http.StatusInternalServerError, ctx.Response.StatusCode)
	})

	t.Run("buildPollVoteResponse poll missing", func(t *testing.T) {
		state := &round10QueryState{
			notFoundPKSK: map[string]bool{
				"POLL#missing#" + storagemodels.SKMetadata: true,
			},
		}
		h, _, _ := round11NewHandler(t, cfg, state, &RegistryStub{})

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/polls/missing/votes", nil, nil, nil)
		require.NoError(t, err)
		_, handled, err := h.buildPollVoteResponse(ctx, "missing", []int{0})
		require.NoError(t, err)
		require.True(t, handled)
		require.Equal(t, http.StatusInternalServerError, ctx.Response.StatusCode)
	})

	t.Run("createPollVoteNotification does not notify self", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{
			NotificationsSvc: &NotificationsServiceStub{
				CreateNotificationFunc: func(_ context.Context, _ *notifications.CreateNotificationCommand) (*notifications.NotificationResult, error) {
					t.Fatalf("notification should not be created for self-votes")
					return nil, nil
				},
			},
		})

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/polls/poll/votes", nil, nil, nil)
		require.NoError(t, err)

		h.createPollVoteNotification(ctx, "poll", "https://example.com/users/alice", &storage.Poll{
			ID:        "poll",
			CreatedBy: "https://example.com/users/alice",
			StatusID:  "status",
		})
	})
}

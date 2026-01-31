package lift

import (
	"context"
	stdErrors "errors"
	"net/http"
	"testing"
	"time"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/services/notes"
	storage "github.com/equaltoai/lesser/pkg/storage"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestStatusesFull_Round12_CheckStatusViewPermission_Coverage(t *testing.T) {
	cfg := round11TestConfig()

	t.Run("public_and_unlisted_are_publicly_viewable", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{})
		ctx := context.Background()

		allowed, err := handler.checkStatusViewPermission(ctx, &storagemodels.Status{Visibility: "public"}, "")
		require.NoError(t, err)
		require.True(t, allowed)

		allowed, err = handler.checkStatusViewPermission(ctx, &storagemodels.Status{Visibility: "unlisted"}, "")
		require.NoError(t, err)
		require.True(t, allowed)
	})

	t.Run("unauthenticated_viewer_cannot_see_non_public", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{})

		allowed, err := handler.checkStatusViewPermission(context.Background(), &storagemodels.Status{
			StatusID:       "s1",
			Visibility:     "private",
			AuthorUsername: "bob",
		}, "")
		require.NoError(t, err)
		require.False(t, allowed)
	})

	t.Run("author_can_view_their_private_status", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{})

		allowed, err := handler.checkStatusViewPermission(context.Background(), &storagemodels.Status{
			StatusID:       "s1",
			Visibility:     "private",
			AuthorUsername: "alice",
		}, "alice")
		require.NoError(t, err)
		require.True(t, allowed)
	})

	t.Run("private_status_following_and_not_following", func(t *testing.T) {
		notFollowingHandler, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{})
		allowed, err := notFollowingHandler.checkStatusViewPermission(context.Background(), &storagemodels.Status{
			StatusID:       "s1",
			Visibility:     "private",
			AuthorUsername: "bob",
		}, "alice")
		require.NoError(t, err)
		require.False(t, allowed)

		followingState := &round10QueryState{
			relationshipRecords: []storagemodels.RelationshipRecord{
				{PK: "FOLLOW#alice", SK: "FOLLOWING#bob", State: storagemodels.RelationshipAccepted},
			},
		}
		followingHandler, _, _ := round11NewHandler(t, cfg, followingState, &RegistryStub{})
		allowed, err = followingHandler.checkStatusViewPermission(context.Background(), &storagemodels.Status{
			StatusID:       "s1",
			Visibility:     "private",
			AuthorUsername: "bob",
		}, "alice")
		require.NoError(t, err)
		require.True(t, allowed)
	})

	t.Run("private_status_follow_lookup_error_returns_error", func(t *testing.T) {
		state := &round10QueryState{
			firstErrorPK: map[string]error{"FOLLOW#alice": stdErrors.New("db down")},
		}
		handler, _, _ := round11NewHandler(t, cfg, state, &RegistryStub{})
		allowed, err := handler.checkStatusViewPermission(context.Background(), &storagemodels.Status{
			StatusID:       "s1",
			Visibility:     "private",
			AuthorUsername: "bob",
		}, "alice")
		require.Error(t, err)
		require.False(t, allowed)
	})

	t.Run("direct_message_visibility", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{})

		allowed, err := handler.checkStatusViewPermission(context.Background(), &storagemodels.Status{
			StatusID:   "s1",
			Visibility: "direct",
			Mentions:   []string{"alice"},
		}, "alice")
		require.NoError(t, err)
		require.True(t, allowed)

		allowed, err = handler.checkStatusViewPermission(context.Background(), &storagemodels.Status{
			StatusID:      "s1",
			Visibility:    "direct",
			ToRecipients:  []string{cfg.ActorURL("alice")},
			CcRecipients:  []string{cfg.ActorURL("carol")},
			BtoRecipients: []string{"https://example.org/users/alice"},
		}, "alice")
		require.NoError(t, err)
		require.True(t, allowed)

		allowed, err = handler.checkStatusViewPermission(context.Background(), &storagemodels.Status{
			StatusID:   "s1",
			Visibility: "direct",
		}, "alice")
		require.NoError(t, err)
		require.False(t, allowed)
	})

	t.Run("unknown_visibility_is_denied", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{})

		allowed, err := handler.checkStatusViewPermission(context.Background(), &storagemodels.Status{
			StatusID:   "s1",
			Visibility: "weird",
		}, "alice")
		require.NoError(t, err)
		require.False(t, allowed)
	})
}

func TestStatusesFull_Round12_EnrichStatusWithPoll_Coverage(t *testing.T) {
	cfg := round11TestConfig()

	t.Run("poll_not_found_is_silently_ignored", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{})
		status := &apimodels.Status{ID: "s1"}

		require.NoError(t, handler.enrichStatusWithPoll(context.Background(), status, "s1", ""))
		require.Nil(t, status.Poll)
	})

	t.Run("poll_query_error_is_returned", func(t *testing.T) {
		state := &round10QueryState{
			allErrorByType: map[string]error{
				"*[]*models.Poll": stdErrors.New("query failed"),
			},
		}
		handler, _, _ := round11NewHandler(t, cfg, state, &RegistryStub{})
		status := &apimodels.Status{ID: "s1"}

		require.Error(t, handler.enrichStatusWithPoll(context.Background(), status, "s1", ""))
	})

	t.Run("poll_without_user_does_not_fetch_votes", func(t *testing.T) {
		pollModel := storagemodels.Poll{
			ID:        "poll-1",
			StatusID:  "s1",
			CreatedBy: "alice",
			Options:   []string{"a", "b"},
			ExpiresAt: time.Now().Add(1 * time.Hour),
			CreatedAt: time.Now().Add(-5 * time.Minute),
			UpdatedAt: time.Now().Add(-1 * time.Minute),
			Votes:     map[string][]int{},
		}
		_ = pollModel.UpdateKeys()

		state := &round10QueryState{
			pollsByID: map[string]storagemodels.Poll{
				pollModel.ID: pollModel,
			},
		}
		handler, _, _ := round11NewHandler(t, cfg, state, &RegistryStub{})
		status := &apimodels.Status{ID: "s1"}

		require.NoError(t, handler.enrichStatusWithPoll(context.Background(), status, "s1", ""))
		require.NotNil(t, status.Poll)
		require.Equal(t, pollModel.ID, status.Poll.ID)
		require.False(t, status.Poll.Voted)
	})

	t.Run("poll_with_user_vote_and_no_vote", func(t *testing.T) {
		pollModel := storagemodels.Poll{
			ID:        "poll-1",
			StatusID:  "s1",
			CreatedBy: "alice",
			Options:   []string{"a", "b"},
			ExpiresAt: time.Now().Add(1 * time.Hour),
			CreatedAt: time.Now().Add(-5 * time.Minute),
			UpdatedAt: time.Now().Add(-1 * time.Minute),
			Votes:     map[string][]int{},
		}
		_ = pollModel.UpdateKeys()

		actorID := cfg.ActorURL("alice")
		vote := storagemodels.PollVote{
			VoterID: actorID,
			Choices: []int{1},
			VotedAt: time.Now().Add(-1 * time.Minute),
		}
		vote.SetPollID(pollModel.ID)

		t.Run("no_vote", func(t *testing.T) {
			state := &round10QueryState{
				pollsByID: map[string]storagemodels.Poll{
					pollModel.ID: pollModel,
				},
				notFoundPKSK: map[string]bool{
					vote.PK + "#" + vote.SK: true,
				},
			}

			reg := &RegistryStub{
				AccountsSvc: &AccountsServiceStub{
					GetAccountFunc: func(_ context.Context, username string) (*storage.Account, error) {
						return &storage.Account{Actor: &activitypub.Actor{BaseObject: activitypub.BaseObject{ID: actorID}}}, nil
					},
				},
			}
			handler, _, _ := round11NewHandler(t, cfg, state, reg)
			status := &apimodels.Status{ID: "s1"}

			require.NoError(t, handler.enrichStatusWithPoll(context.Background(), status, "s1", "alice"))
			require.NotNil(t, status.Poll)
			require.False(t, status.Poll.Voted)
		})

		t.Run("has_vote", func(t *testing.T) {
			state := &round10QueryState{
				pollsByID: map[string]storagemodels.Poll{
					pollModel.ID: pollModel,
				},
				pollVotesByKey: map[string]storagemodels.PollVote{
					vote.PK + "#" + vote.SK: vote,
				},
			}

			reg := &RegistryStub{
				AccountsSvc: &AccountsServiceStub{
					GetAccountFunc: func(_ context.Context, username string) (*storage.Account, error) {
						return &storage.Account{Actor: &activitypub.Actor{BaseObject: activitypub.BaseObject{ID: actorID}}}, nil
					},
				},
			}
			handler, _, _ := round11NewHandler(t, cfg, state, reg)
			status := &apimodels.Status{ID: "s1"}

			require.NoError(t, handler.enrichStatusWithPoll(context.Background(), status, "s1", "alice"))
			require.NotNil(t, status.Poll)
			require.True(t, status.Poll.Voted)
			require.Equal(t, []int{1}, status.Poll.OwnVotes)
		})

		t.Run("account_lookup_error_skips_votes", func(t *testing.T) {
			state := &round10QueryState{
				pollsByID: map[string]storagemodels.Poll{
					pollModel.ID: pollModel,
				},
			}

			reg := &RegistryStub{
				AccountsSvc: &AccountsServiceStub{
					GetAccountFunc: func(_ context.Context, username string) (*storage.Account, error) {
						return nil, stdErrors.New("no account")
					},
				},
			}
			handler, _, _ := round11NewHandler(t, cfg, state, reg)
			status := &apimodels.Status{ID: "s1"}

			require.NoError(t, handler.enrichStatusWithPoll(context.Background(), status, "s1", "alice"))
			require.NotNil(t, status.Poll)
			require.False(t, status.Poll.Voted)
		})
	})
}

func TestStatusesFull_Round12_HandlerErrorBranches_Coverage(t *testing.T) {
	cfg := round11TestConfig()
	token := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeRead, auth.ScopeWrite})
	headers := map[string]string{"Authorization": "Bearer " + token}

	t.Run("create_status_parse_error", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{})

		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v1/statuses", headers, nil, []byte("{"))
		require.NoError(t, handler.HandleCreateStatusFull(ctx))
		require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)
	})

	t.Run("create_status_create_note_error_returns_500", func(t *testing.T) {
		notesStub := &NotesServiceStub{
			CreateNoteFunc: func(_ context.Context, _ *notes.CreateNoteCommand) (*notes.NoteResult, error) {
				return nil, stdErrors.New("boom")
			},
		}
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{NotesSvc: notesStub})

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses", headers, nil, apimodels.CreateStatusRequest{Status: "hello"})
		require.NoError(t, err)
		require.NoError(t, handler.HandleCreateStatusFull(ctx))
		require.Equal(t, http.StatusInternalServerError, ctx.Response.StatusCode)
	})

	t.Run("get_status_missing_id", func(t *testing.T) {
		notesStub := &NotesServiceStub{
			GetNoteFunc: func(_ context.Context, _ string) (*storagemodels.Status, error) {
				return nil, stdErrors.New("unexpected")
			},
		}
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{NotesSvc: notesStub})

		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/statuses/", headers, nil, nil)
		require.NoError(t, err)
		require.NoError(t, handler.HandleGetStatusFull(ctx))
		require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)
	})

	t.Run("get_status_notes_errors_map_to_status_codes", func(t *testing.T) {
		tests := []struct {
			name      string
			noteErr   error
			wantCode  int
			expectNil bool
		}{
			{name: "not_found", noteErr: stdErrors.New("not found"), wantCode: http.StatusNotFound, expectNil: true},
			{name: "access_denied", noteErr: stdErrors.New("access denied"), wantCode: http.StatusNotFound, expectNil: true},
			{name: "other", noteErr: stdErrors.New("boom"), wantCode: http.StatusInternalServerError, expectNil: true},
		}

		for _, tt := range tests {
			tt := tt
			t.Run(tt.name, func(t *testing.T) {
				notesStub := &NotesServiceStub{
					GetNoteFunc: func(_ context.Context, _ string) (*storagemodels.Status, error) {
						return nil, tt.noteErr
					},
				}
				handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{NotesSvc: notesStub})

				ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/statuses/s1", headers, nil, nil)
				require.NoError(t, err)
				ctx.SetParam("id", "s1")
				require.NoError(t, handler.HandleGetStatusFull(ctx))
				require.Equal(t, tt.wantCode, ctx.Response.StatusCode)
			})
		}
	})

	t.Run("get_status_denied_by_privacy_returns_404", func(t *testing.T) {
		notesStub := &NotesServiceStub{
			GetNoteFunc: func(_ context.Context, _ string) (*storagemodels.Status, error) {
				return &storagemodels.Status{
					StatusID:       "s1",
					Visibility:     "private",
					AuthorUsername: "bob",
				}, nil
			},
		}
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{NotesSvc: notesStub})

		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/statuses/s1", headers, nil, nil)
		require.NoError(t, err)
		ctx.SetParam("id", "s1")
		require.NoError(t, handler.HandleGetStatusFull(ctx))
		require.Equal(t, http.StatusNotFound, ctx.Response.StatusCode)
	})

	t.Run("get_status_permission_check_error_returns_500", func(t *testing.T) {
		notesStub := &NotesServiceStub{
			GetNoteFunc: func(_ context.Context, _ string) (*storagemodels.Status, error) {
				return &storagemodels.Status{
					StatusID:       "s1",
					Visibility:     "private",
					AuthorUsername: "bob",
				}, nil
			},
		}
		state := &round10QueryState{
			firstErrorPK: map[string]error{"FOLLOW#alice": stdErrors.New("db down")},
		}
		handler, _, _ := round11NewHandler(t, cfg, state, &RegistryStub{NotesSvc: notesStub})

		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/statuses/s1", headers, nil, nil)
		require.NoError(t, err)
		ctx.SetParam("id", "s1")
		require.NoError(t, handler.HandleGetStatusFull(ctx))
		require.Equal(t, http.StatusInternalServerError, ctx.Response.StatusCode)
	})

	t.Run("delete_status_missing_id", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{})

		ctx, err := round10NewLiftContext(http.MethodDelete, "/api/v1/statuses/", headers, nil, nil)
		require.NoError(t, err)
		require.NoError(t, handler.HandleDeleteStatusFull(ctx))
		require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)
	})

	t.Run("delete_status_not_authorized_and_internal_error", func(t *testing.T) {
		notesStub := &NotesServiceStub{
			DeleteNoteFunc: func(_ context.Context, _ *notes.DeleteNoteCommand) error {
				return stdErrors.New("not authorized")
			},
		}
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{NotesSvc: notesStub})

		ctxForbidden, err := round10NewLiftContext(http.MethodDelete, "/api/v1/statuses/s1", headers, nil, nil)
		require.NoError(t, err)
		ctxForbidden.SetParam("id", "s1")
		require.NoError(t, handler.HandleDeleteStatusFull(ctxForbidden))
		require.Equal(t, http.StatusForbidden, ctxForbidden.Response.StatusCode)

		notesStub.DeleteNoteFunc = func(_ context.Context, _ *notes.DeleteNoteCommand) error {
			return stdErrors.New("boom")
		}
		ctxInternal, err := round10NewLiftContext(http.MethodDelete, "/api/v1/statuses/s1", headers, nil, nil)
		require.NoError(t, err)
		ctxInternal.SetParam("id", "s1")
		require.NoError(t, handler.HandleDeleteStatusFull(ctxInternal))
		require.Equal(t, http.StatusInternalServerError, ctxInternal.Response.StatusCode)
	})
}

package lift

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/auth"
	servicenotes "github.com/equaltoai/lesser/pkg/services/notes"
	"github.com/equaltoai/lesser/pkg/storage"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestNotesHandlersRound12(t *testing.T) {
	cfg := round11TestConfig()

	aliceActorID := "https://example.com/users/alice"
	alicePK := "ACTOR#alice"

	highRep := round12StoredReputationModel(t, aliceActorID, 600)
	state := &round10QueryState{
		reputationsByPK: map[string][]storagemodels.Reputation{
			alicePK: {highRep},
		},
	}

	notesSvc := &NotesServiceStub{
		CreateCommunityNoteFunc: func(_ context.Context, cmd *servicenotes.CreateCommunityNoteCommand) (*servicenotes.CreateCommunityNoteResult, error) {
			cmd.Note.ID = "note-1"
			return &servicenotes.CreateCommunityNoteResult{Note: cmd.Note}, nil
		},
		GetVisibleCommunityNotesFunc: func(_ context.Context, _ *servicenotes.GetVisibleCommunityNotesQuery) (*servicenotes.GetVisibleCommunityNotesResult, error) {
			return &servicenotes.GetVisibleCommunityNotesResult{
				Notes: []*storage.CommunityNote{
					{ID: "n1", ObjectID: "obj1", ObjectType: "Note", AuthorID: "https://example.com/users/bob", Content: "visible", Language: "en", Sources: []string{"https://a"}, HelpfulVotes: 1, NotHelpfulVotes: 0, Score: 0.7, VisibilityStatus: "visible", CreatedAt: time.Now().Add(-2 * time.Hour)},
					{ID: "n2", ObjectID: "obj1", ObjectType: "Note", AuthorID: "https://example.com/users/carol", Content: "pending", Language: "en", Sources: []string{"https://b"}, HelpfulVotes: 0, NotHelpfulVotes: 0, Score: 0.1, VisibilityStatus: "pending", CreatedAt: time.Now().Add(-1 * time.Hour)},
				},
			}, nil
		},
		GetCommunityNoteFunc: func(_ context.Context, query *servicenotes.GetCommunityNoteQuery) (*servicenotes.GetCommunityNoteResult, error) {
			if query.NoteID == "missing" {
				return nil, errors.New("not found")
			}
			note := &storage.CommunityNote{ID: query.NoteID, AuthorID: "https://example.com/users/bob", Content: "note", CreatedAt: time.Now().Add(-1 * time.Hour)}
			if query.NoteID == "own" {
				note.AuthorID = aliceActorID
			}
			return &servicenotes.GetCommunityNoteResult{Note: note}, nil
		},
		CreateCommunityNoteVoteFunc: func(_ context.Context, cmd *servicenotes.CreateCommunityNoteVoteCommand) (*servicenotes.CreateCommunityNoteVoteResult, error) {
			if cmd.Vote.NoteID == "fail" {
				return nil, errors.New("create failed")
			}
			return &servicenotes.CreateCommunityNoteVoteResult{Vote: cmd.Vote}, nil
		},
		GetCommunityNotesByAuthorFunc: func(_ context.Context, query *servicenotes.GetCommunityNotesByAuthorQuery) (*servicenotes.GetCommunityNotesByAuthorResult, error) {
			if query.AuthorID == "https://example.com/users/error" {
				return nil, errors.New("boom")
			}
			return &servicenotes.GetCommunityNotesByAuthorResult{
				Notes: []*storage.CommunityNote{
					{ID: "n1", ObjectID: "obj1", ObjectType: "Note", AuthorID: query.AuthorID, Content: "note", CreatedAt: time.Now().Add(-1 * time.Hour), HelpfulVotes: 2, NotHelpfulVotes: 1, Score: 0.5},
				},
				NextCursor: "",
			}, nil
		},
	}

	h, _, _ := round11NewHandler(t, cfg, state, &RegistryStub{NotesSvc: notesSvc})
	token := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeRead, auth.ScopeWrite})
	headers := map[string]string{"Authorization": "Bearer " + token}
	lowerHeaders := map[string]string{"authorization": "Bearer " + token}

	t.Run("create note unauthorized", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/notes", nil, nil, apimodels.CreateCommunityNoteRequest{ObjectID: "obj1", ObjectType: "Note", Content: "content", Language: "en"})
		require.NoError(t, err)
		require.NoError(t, h.HandleCreateNoteLift(ctx))
		require.Equal(t, http.StatusUnauthorized, ctx.Response.StatusCode)
	})

	t.Run("create note returns 500 when reputation signer misconfigured", func(t *testing.T) {
		badCfg := round11TestConfig()
		badCfg.ReputationPrivateKey = "not a pem"
		badHandler, _, _ := round11NewHandler(t, badCfg, state, &RegistryStub{NotesSvc: notesSvc})

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/notes", headers, nil, apimodels.CreateCommunityNoteRequest{ObjectID: "obj1", ObjectType: "Note", Content: "content", Language: "en"})
		require.NoError(t, err)
		require.NoError(t, badHandler.HandleCreateNoteLift(ctx))
		require.Equal(t, http.StatusInternalServerError, ctx.Response.StatusCode)
	})

	t.Run("create note forbidden for low reputation", func(t *testing.T) {
		lowRep := round12StoredReputationModel(t, aliceActorID, 50)
		lowState := &round10QueryState{
			reputationsByPK: map[string][]storagemodels.Reputation{
				alicePK: {lowRep},
			},
		}
		lowHandler, _, _ := round11NewHandler(t, cfg, lowState, &RegistryStub{NotesSvc: notesSvc})

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/notes", headers, nil, apimodels.CreateCommunityNoteRequest{ObjectID: "obj1", ObjectType: "Note", Content: "content", Language: "en"})
		require.NoError(t, err)
		require.NoError(t, lowHandler.HandleCreateNoteLift(ctx))
		require.Equal(t, http.StatusForbidden, ctx.Response.StatusCode)
	})

	t.Run("create note invalid body", func(t *testing.T) {
		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v1/notes", headers, nil, []byte("{"))
		require.NoError(t, h.HandleCreateNoteLift(ctx))
		require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)
	})

	t.Run("create note too many sources", func(t *testing.T) {
		req := apimodels.CreateCommunityNoteRequest{
			ObjectID:   "obj1",
			ObjectType: "Note",
			Content:    "content",
			Language:   "en",
			Sources: []apimodels.CommunityNoteSource{
				{URL: "1"}, {URL: "2"}, {URL: "3"}, {URL: "4"}, {URL: "5"}, {URL: "6"},
			},
		}
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/notes", headers, nil, req)
		require.NoError(t, err)
		require.NoError(t, h.HandleCreateNoteLift(ctx))
		require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)
	})

	t.Run("create note rate limited", func(t *testing.T) {
		minRep := round12StoredReputationModel(t, aliceActorID, 100)
		authorKey := "AUTHOR#" + aliceActorID + "#NOTES"
		rateState := &round10QueryState{
			reputationsByPK: map[string][]storagemodels.Reputation{
				alicePK: {minRep},
			},
			communityNotesByGSI3PK: map[string][]storagemodels.CommunityNote{
				authorKey: {{
					ID:        "existing",
					AuthorID:  aliceActorID,
					Content:   "existing",
					CreatedAt: time.Now().Add(-1 * time.Hour),
				}},
			},
		}
		rateHandler, _, _ := round11NewHandler(t, cfg, rateState, &RegistryStub{NotesSvc: notesSvc})

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/notes", lowerHeaders, nil, apimodels.CreateCommunityNoteRequest{ObjectID: "obj1", ObjectType: "Note", Content: "content", Language: "en"})
		require.NoError(t, err)
		require.NoError(t, rateHandler.HandleCreateNoteLift(ctx))
		require.Equal(t, http.StatusTooManyRequests, ctx.Response.StatusCode)
	})

	t.Run("create note storage failure returns 500", func(t *testing.T) {
		failSvc := &NotesServiceStub{
			CreateCommunityNoteFunc: func(context.Context, *servicenotes.CreateCommunityNoteCommand) (*servicenotes.CreateCommunityNoteResult, error) {
				return nil, errors.New("store failed")
			},
		}
		failHandler, _, _ := round11NewHandler(t, cfg, state, &RegistryStub{NotesSvc: failSvc})

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/notes", headers, nil, apimodels.CreateCommunityNoteRequest{ObjectID: "obj1", ObjectType: "Note", Content: "content", Language: "en"})
		require.NoError(t, err)
		require.NoError(t, failHandler.HandleCreateNoteLift(ctx))
		require.Equal(t, http.StatusInternalServerError, ctx.Response.StatusCode)
	})

	t.Run("create note success", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/notes", lowerHeaders, nil, apimodels.CreateCommunityNoteRequest{ObjectID: "obj1", ObjectType: "Note", Content: "content", Language: "en"})
		require.NoError(t, err)
		require.NoError(t, h.HandleCreateNoteLift(ctx))
		require.Equal(t, http.StatusCreated, ctx.Response.StatusCode)
		require.NotEmpty(t, ctx.Response.Headers["X-Cost-Micros"])
	})

	t.Run("get notes missing object id", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/notes/", nil, nil, nil)
		require.NoError(t, err)
		require.NoError(t, h.HandleGetNotesLift(ctx))
		require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)
	})

	t.Run("get notes service failure returns 500", func(t *testing.T) {
		failSvc := &NotesServiceStub{
			GetVisibleCommunityNotesFunc: func(context.Context, *servicenotes.GetVisibleCommunityNotesQuery) (*servicenotes.GetVisibleCommunityNotesResult, error) {
				return nil, errors.New("boom")
			},
		}
		failHandler, _, _ := round11NewHandler(t, cfg, state, &RegistryStub{NotesSvc: failSvc})

		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/notes/obj1", nil, nil, nil)
		require.NoError(t, err)
		ctx.SetParam("object_id", "obj1")
		require.NoError(t, failHandler.HandleGetNotesLift(ctx))
		require.Equal(t, http.StatusInternalServerError, ctx.Response.StatusCode)
	})

	t.Run("get notes success unauthenticated", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/notes/obj1", nil, nil, nil)
		require.NoError(t, err)
		ctx.SetParam("object_id", "obj1")
		require.NoError(t, h.HandleGetNotesLift(ctx))
		require.Equal(t, http.StatusOK, ctx.Response.StatusCode)
	})

	t.Run("get notes success authenticated (lowercase header)", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/notes/obj1", lowerHeaders, nil, nil)
		require.NoError(t, err)
		ctx.SetParam("object_id", "obj1")
		require.NoError(t, h.HandleGetNotesLift(ctx))
		require.Equal(t, http.StatusOK, ctx.Response.StatusCode)
	})

	t.Run("vote missing note id", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/notes//vote", headers, nil, apimodels.VoteCommunityNoteRequest{VoteType: "helpful"})
		require.NoError(t, err)
		require.NoError(t, h.HandleVoteNoteLift(ctx))
		require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)
	})

	t.Run("vote unauthorized", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/notes/n1/vote", nil, nil, apimodels.VoteCommunityNoteRequest{VoteType: "helpful"})
		require.NoError(t, err)
		ctx.SetParam("id", "n1")
		require.NoError(t, h.HandleVoteNoteLift(ctx))
		require.Equal(t, http.StatusUnauthorized, ctx.Response.StatusCode)
	})

	t.Run("vote invalid request body", func(t *testing.T) {
		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v1/notes/n1/vote", headers, nil, []byte("{"))
		ctx.SetParam("id", "n1")
		require.NoError(t, h.HandleVoteNoteLift(ctx))
		require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)
	})

	t.Run("vote forbidden for low reputation", func(t *testing.T) {
		lowRep := round12StoredReputationModel(t, aliceActorID, 5)
		lowState := &round10QueryState{
			reputationsByPK: map[string][]storagemodels.Reputation{
				alicePK: {lowRep},
			},
		}
		lowHandler, _, _ := round11NewHandler(t, cfg, lowState, &RegistryStub{NotesSvc: notesSvc})

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/notes/n1/vote", headers, nil, apimodels.VoteCommunityNoteRequest{VoteType: "helpful"})
		require.NoError(t, err)
		ctx.SetParam("id", "n1")
		require.NoError(t, lowHandler.HandleVoteNoteLift(ctx))
		require.Equal(t, http.StatusForbidden, ctx.Response.StatusCode)
	})

	t.Run("vote not found", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/notes/missing/vote", headers, nil, apimodels.VoteCommunityNoteRequest{VoteType: "helpful"})
		require.NoError(t, err)
		ctx.SetParam("id", "missing")
		require.NoError(t, h.HandleVoteNoteLift(ctx))
		require.Equal(t, http.StatusNotFound, ctx.Response.StatusCode)
	})

	t.Run("vote forbidden on own note", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/notes/own/vote", headers, nil, apimodels.VoteCommunityNoteRequest{VoteType: "helpful"})
		require.NoError(t, err)
		ctx.SetParam("id", "own")
		require.NoError(t, h.HandleVoteNoteLift(ctx))
		require.Equal(t, http.StatusForbidden, ctx.Response.StatusCode)
	})

	t.Run("vote storage failure returns 500", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/notes/fail/vote", headers, nil, apimodels.VoteCommunityNoteRequest{VoteType: "helpful"})
		require.NoError(t, err)
		ctx.SetParam("id", "fail")
		require.NoError(t, h.HandleVoteNoteLift(ctx))
		require.Equal(t, http.StatusInternalServerError, ctx.Response.StatusCode)
	})

	t.Run("vote success", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/notes/n1/vote", lowerHeaders, nil, apimodels.VoteCommunityNoteRequest{VoteType: "helpful"})
		require.NoError(t, err)
		ctx.SetParam("id", "n1")
		require.NoError(t, h.HandleVoteNoteLift(ctx))
		require.Equal(t, http.StatusOK, ctx.Response.StatusCode)
	})

	t.Run("get user notes missing username", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/accounts//notes", nil, nil, nil)
		require.NoError(t, err)
		require.NoError(t, h.HandleGetUserNotesLift(ctx))
		require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)
	})

	t.Run("get user notes invalid limit", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/accounts/alice/notes", nil, map[string]string{"limit": "bad"}, nil)
		require.NoError(t, err)
		ctx.SetParam("id", "alice")
		require.NoError(t, h.HandleGetUserNotesLift(ctx))
		require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)
	})

	t.Run("get user notes service failure returns 500", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/accounts/error/notes", nil, nil, nil)
		require.NoError(t, err)
		ctx.SetParam("id", "error")
		require.NoError(t, h.HandleGetUserNotesLift(ctx))
		require.Equal(t, http.StatusInternalServerError, ctx.Response.StatusCode)
	})

	t.Run("get user notes success", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/accounts/alice/notes", nil, nil, nil)
		require.NoError(t, err)
		ctx.SetParam("id", "alice")
		require.NoError(t, h.HandleGetUserNotesLift(ctx))
		require.Equal(t, http.StatusOK, ctx.Response.StatusCode)
	})
}

func TestCalculateNotesStatsRound12(t *testing.T) {
	require.Equal(t, apimodels.CommunityNoteStats{}, calculateNotesStats(nil))
}

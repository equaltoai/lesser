package lift

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/services/notes"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestBookmarksRound12_Coverage(t *testing.T) {
	cfg := round11TestConfig()
	now := time.Now()

	status := &storagemodels.Status{
		StatusID:       "s1",
		AuthorUsername: "alice",
		AuthorID:       cfg.ActorURL("alice"),
		Content:        "hello",
		PublishedAt:    now,
		CreatedAt:      now,
	}

	state := &round10QueryState{}

	t.Run("bookmark action error returns 500", func(t *testing.T) {
		notesStub := &NotesServiceStub{
			BookmarkNoteFunc: func(_ context.Context, _ *notes.BookmarkNoteCommand) (*notes.BookmarkResult, error) {
				return nil, errors.New("bookmark failed")
			},
		}
		h, _, _ := round11NewHandler(t, cfg, state, &RegistryStub{NotesSvc: notesStub})

		token := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeWrite})
		headers := map[string]string{"Authorization": "Bearer " + token}

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses/s1/bookmark", headers, nil, nil)
		require.NoError(t, err)
		ctx.SetParam("id", "s1")

		require.NoError(t, h.HandleBookmarkLift(ctx))
		require.Equal(t, http.StatusInternalServerError, ctx.Response.StatusCode)
	})

	t.Run("unbookmark validation error", func(t *testing.T) {
		notesStub := &NotesServiceStub{
			UnbookmarkNoteFunc: func(_ context.Context, _ *notes.UnbookmarkNoteCommand) (*notes.BookmarkResult, error) {
				return &notes.BookmarkResult{Status: status}, nil
			},
		}
		h, _, _ := round11NewHandler(t, cfg, state, &RegistryStub{NotesSvc: notesStub})

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses/bad id/unbookmark", nil, nil, nil)
		require.NoError(t, err)
		ctx.SetParam("id", "bad id")

		require.NoError(t, h.HandleUnbookmarkLift(ctx))
		require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)
	})

	t.Run("unbookmark forbidden when scope missing", func(t *testing.T) {
		notesStub := &NotesServiceStub{
			UnbookmarkNoteFunc: func(_ context.Context, _ *notes.UnbookmarkNoteCommand) (*notes.BookmarkResult, error) {
				return &notes.BookmarkResult{Status: status}, nil
			},
		}
		h, _, _ := round11NewHandler(t, cfg, state, &RegistryStub{NotesSvc: notesStub})

		token := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeRead})
		headers := map[string]string{"Authorization": "Bearer " + token}

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses/s1/unbookmark", headers, nil, nil)
		require.NoError(t, err)
		ctx.SetParam("id", "s1")

		require.NoError(t, h.HandleUnbookmarkLift(ctx))
		require.Equal(t, http.StatusForbidden, ctx.Response.StatusCode)
	})

	t.Run("unbookmark unauthorized when missing token", func(t *testing.T) {
		notesStub := &NotesServiceStub{
			UnbookmarkNoteFunc: func(_ context.Context, _ *notes.UnbookmarkNoteCommand) (*notes.BookmarkResult, error) {
				return &notes.BookmarkResult{Status: status}, nil
			},
		}
		h, _, _ := round11NewHandler(t, cfg, state, &RegistryStub{NotesSvc: notesStub})

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses/s1/unbookmark", nil, nil, nil)
		require.NoError(t, err)
		ctx.SetParam("id", "s1")

		require.NoError(t, h.HandleUnbookmarkLift(ctx))
		require.Equal(t, http.StatusUnauthorized, ctx.Response.StatusCode)
	})

	t.Run("unbookmark status not found", func(t *testing.T) {
		notesStub := &NotesServiceStub{
			UnbookmarkNoteFunc: func(_ context.Context, _ *notes.UnbookmarkNoteCommand) (*notes.BookmarkResult, error) {
				return nil, errors.New("status not found")
			},
		}
		h, _, _ := round11NewHandler(t, cfg, state, &RegistryStub{NotesSvc: notesStub})

		token := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeWrite})
		headers := map[string]string{"Authorization": "Bearer " + token}

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses/s1/unbookmark", headers, nil, nil)
		require.NoError(t, err)
		ctx.SetParam("id", "s1")

		require.NoError(t, h.HandleUnbookmarkLift(ctx))
		require.Equal(t, http.StatusNotFound, ctx.Response.StatusCode)
	})

	t.Run("unbookmark internal error", func(t *testing.T) {
		notesStub := &NotesServiceStub{
			UnbookmarkNoteFunc: func(_ context.Context, _ *notes.UnbookmarkNoteCommand) (*notes.BookmarkResult, error) {
				return nil, errors.New("database down")
			},
		}
		h, _, _ := round11NewHandler(t, cfg, state, &RegistryStub{NotesSvc: notesStub})

		token := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeWrite})
		headers := map[string]string{"Authorization": "Bearer " + token}

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses/s1/unbookmark", headers, nil, nil)
		require.NoError(t, err)
		ctx.SetParam("id", "s1")

		require.NoError(t, h.HandleUnbookmarkLift(ctx))
		require.Equal(t, http.StatusInternalServerError, ctx.Response.StatusCode)
	})

	t.Run("get bookmarks unauthorized and forbidden", func(t *testing.T) {
		notesStub := &NotesServiceStub{
			GetBookmarksFunc: func(_ context.Context, _ *notes.GetBookmarksQuery) (*notes.Result, error) {
				return &notes.Result{
					Notes: []*storagemodels.Status{status},
					Pagination: &interfaces.PaginatedResult[*storagemodels.Status]{
						NextCursor: "next",
					},
				}, nil
			},
		}
		h, _, _ := round11NewHandler(t, cfg, state, &RegistryStub{NotesSvc: notesStub})

		ctxUnauthed, err := round10NewLiftContext(http.MethodGet, "/api/v1/bookmarks", nil, nil, nil)
		require.NoError(t, err)
		require.NoError(t, h.HandleGetBookmarksLift(ctxUnauthed))
		require.Equal(t, http.StatusUnauthorized, ctxUnauthed.Response.StatusCode)

		token := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeWrite})
		headers := map[string]string{"Authorization": "Bearer " + token}

		ctxForbidden, err := round10NewLiftContext(http.MethodGet, "/api/v1/bookmarks", headers, nil, nil)
		require.NoError(t, err)
		require.NoError(t, h.HandleGetBookmarksLift(ctxForbidden))
		require.Equal(t, http.StatusForbidden, ctxForbidden.Response.StatusCode)
	})

	t.Run("get bookmarks internal error", func(t *testing.T) {
		notesStub := &NotesServiceStub{
			GetBookmarksFunc: func(_ context.Context, _ *notes.GetBookmarksQuery) (*notes.Result, error) {
				return nil, errors.New("boom")
			},
		}
		h, _, _ := round11NewHandler(t, cfg, state, &RegistryStub{NotesSvc: notesStub})

		token := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeRead})
		headers := map[string]string{"Authorization": "Bearer " + token}

		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/bookmarks", headers, nil, nil)
		require.NoError(t, err)
		require.NoError(t, h.HandleGetBookmarksLift(ctx))
		require.Equal(t, http.StatusInternalServerError, ctx.Response.StatusCode)
	})
}


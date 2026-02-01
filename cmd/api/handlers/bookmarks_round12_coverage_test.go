package handlers

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
		ctx.Params["id"] = "s1"

		requireStatus(t, http.StatusInternalServerError)(h.HandleBookmarkLift(ctx))
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
		ctx.Params["id"] = "bad id"

		requireStatus(t, http.StatusBadRequest)(h.HandleUnbookmarkLift(ctx))
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
		ctx.Params["id"] = "s1"

		requireStatus(t, http.StatusForbidden)(h.HandleUnbookmarkLift(ctx))
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
		ctx.Params["id"] = "s1"

		requireStatus(t, http.StatusUnauthorized)(h.HandleUnbookmarkLift(ctx))
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
		ctx.Params["id"] = "s1"

		requireStatus(t, http.StatusNotFound)(h.HandleUnbookmarkLift(ctx))
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
		ctx.Params["id"] = "s1"

		requireStatus(t, http.StatusInternalServerError)(h.HandleUnbookmarkLift(ctx))
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
		requireStatus(t, http.StatusUnauthorized)(h.HandleGetBookmarksLift(ctxUnauthed))

		token := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeWrite})
		headers := map[string]string{"Authorization": "Bearer " + token}

		ctxForbidden, err := round10NewLiftContext(http.MethodGet, "/api/v1/bookmarks", headers, nil, nil)
		require.NoError(t, err)
		requireStatus(t, http.StatusForbidden)(h.HandleGetBookmarksLift(ctxForbidden))
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
		requireStatus(t, http.StatusInternalServerError)(h.HandleGetBookmarksLift(ctx))
	})
}

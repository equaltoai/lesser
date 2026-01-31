package lift

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/services/notes"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestBookmarksHandlers(t *testing.T) {
	cfg := round11TestConfig()
	now := time.Now()
	status := &storagemodels.Status{StatusID: "s1", AuthorUsername: "alice", AuthorID: cfg.ActorURL("alice"), Content: "hello", PublishedAt: now, CreatedAt: now}

	state := &round10QueryState{
		usersByUsername: map[string]storagemodels.User{
			"alice": {PK: "USER#alice", SK: storagemodels.SKMetadata, Username: "alice", Role: "user", Approved: true, Version: 1, CreatedAt: now.Add(-24 * time.Hour)},
		},
	}

	notesStub := &NotesServiceStub{
		BookmarkNoteFunc: func(_ context.Context, cmd *notes.BookmarkNoteCommand) (*notes.BookmarkResult, error) {
			require.Equal(t, "s1", cmd.StatusID)
			return &notes.BookmarkResult{Status: status}, nil
		},
		UnbookmarkNoteFunc: func(_ context.Context, cmd *notes.UnbookmarkNoteCommand) (*notes.BookmarkResult, error) {
			require.Equal(t, "s1", cmd.StatusID)
			return &notes.BookmarkResult{Status: status}, nil
		},
		GetBookmarksFunc: func(_ context.Context, _ *notes.GetBookmarksQuery) (*notes.Result, error) {
			return &notes.Result{
				Notes: []*storagemodels.Status{status},
				Pagination: &interfaces.PaginatedResult[*storagemodels.Status]{
					NextCursor: "next",
				},
			}, nil
		},
	}

	handler, _, _ := round11NewHandler(t, cfg, state, &RegistryStub{NotesSvc: notesStub})
	token := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeRead, auth.ScopeWrite})
	headers := map[string]string{"Authorization": "Bearer " + token}

	ctxBookmark, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses/s1/bookmark", headers, nil, nil)
	require.NoError(t, err)
	ctxBookmark.SetParam("id", "s1")
	require.NoError(t, handler.HandleBookmarkLift(ctxBookmark))
	require.Equal(t, http.StatusOK, ctxBookmark.Response.StatusCode)

	ctxUnbookmark, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses/s1/unbookmark", headers, nil, nil)
	require.NoError(t, err)
	ctxUnbookmark.SetParam("id", "s1")
	require.NoError(t, handler.HandleUnbookmarkLift(ctxUnbookmark))
	require.Equal(t, http.StatusOK, ctxUnbookmark.Response.StatusCode)

	ctxList, err := round10NewLiftContext(http.MethodGet, "/api/v1/bookmarks", headers, map[string]string{"limit": "1"}, nil)
	require.NoError(t, err)
	require.NoError(t, handler.HandleGetBookmarksLift(ctxList))
	require.Contains(t, ctxList.Response.Headers["Link"], "max_id=")
}

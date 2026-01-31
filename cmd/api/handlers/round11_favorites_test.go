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

func TestHandleGetFavouritesLift(t *testing.T) {
	h, _, _ := round11NewHandlerSliceC(t, nil)

	note := &storagemodels.Status{
		StatusID:       "status-1",
		AuthorUsername: "alice",
		AuthorID:       h.cfg.BaseURL() + "/users/alice",
		CreatedAt:      time.Now().Add(-10 * time.Minute),
	}

	notesSvc := &NotesServiceStub{
		GetFavoritedNotesFunc: func(ctx context.Context, query *notes.ListNotesQuery) (*notes.Result, error) {
			return &notes.Result{
				Notes: []*storagemodels.Status{note},
				Pagination: &interfaces.PaginatedResult[*storagemodels.Status]{
					Items:      []*storagemodels.Status{note},
					NextCursor: "next",
					HasMore:    true,
				},
			}, nil
		},
	}

	h.registry = &RegistryStub{NotesSvc: notesSvc}

	token := round11SignToken(t, h.cfg.JWTSecret, "alice", []string{auth.ScopeRead}, "sess-1")
	ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/favourites", map[string]string{"Authorization": "Bearer " + token}, nil, nil)
	require.NoError(t, err)

	require.NoError(t, h.HandleGetFavouritesLift(ctx))
}

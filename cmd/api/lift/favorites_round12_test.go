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
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestFavoritesRound12(t *testing.T) {
	cfg := round11TestConfig()
	readToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeRead})

	t.Run("unauthorized without token returns 401", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{})
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/favourites", nil, nil, nil)
		require.NoError(t, err)

		require.NoError(t, h.HandleGetFavouritesLift(ctx))
		require.Equal(t, http.StatusUnauthorized, ctx.Response.StatusCode)
	})

	t.Run("returns 500 when notes service missing", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{})
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/favourites", map[string]string{
			"Authorization": "Bearer " + readToken,
		}, nil, nil)
		require.NoError(t, err)

		require.NoError(t, h.HandleGetFavouritesLift(ctx))
		require.Equal(t, http.StatusInternalServerError, ctx.Response.StatusCode)
	})

	t.Run("returns 500 when service errors", func(t *testing.T) {
		notesSvc := &NotesServiceStub{
			GetFavoritedNotesFunc: func(context.Context, *servicenotes.ListNotesQuery) (*servicenotes.Result, error) {
				return nil, errors.New("boom")
			},
		}
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{NotesSvc: notesSvc})
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/favourites", map[string]string{
			"Authorization": "Bearer " + readToken,
		}, nil, nil)
		require.NoError(t, err)

		require.NoError(t, h.HandleGetFavouritesLift(ctx))
		require.Equal(t, http.StatusInternalServerError, ctx.Response.StatusCode)
	})

	t.Run("success caps limit and sets pagination headers", func(t *testing.T) {
		notesSvc := &NotesServiceStub{
			GetFavoritedNotesFunc: func(_ context.Context, query *servicenotes.ListNotesQuery) (*servicenotes.Result, error) {
				require.LessOrEqual(t, query.Pagination.Limit, 40)
				return &servicenotes.Result{
					Notes: []*storagemodels.Status{
						{
							StatusID:       "s1",
							AuthorUsername: "bob",
							Content:        "hello",
							Visibility:     "public",
							CreatedAt:      time.Now().Add(-1 * time.Hour),
							UpdatedAt:      time.Now(),
							ConversationID: "conv-1",
							InReplyToID:    "",
						},
					},
					Pagination: &interfaces.PaginatedResult[*storagemodels.Status]{NextCursor: "next"},
				}, nil
			},
		}
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{NotesSvc: notesSvc})
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/favourites", map[string]string{
			"Authorization": "Bearer " + readToken,
		}, map[string]string{"limit": "100"}, nil)
		require.NoError(t, err)

		require.NoError(t, h.HandleGetFavouritesLift(ctx))
		require.Equal(t, http.StatusOK, ctx.Response.StatusCode)
		require.Contains(t, ctx.Response.Headers["Link"], "max_id=next")
		resp := ctx.Response.Body.([]*apimodels.Status)
		require.Len(t, resp, 1)
	})
}

package handlers

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/auth"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestStatusInfoRound19_UnusedHelpers(t *testing.T) {
	cfg := round11TestConfig()

	t.Run("performOptionalHistoryAuth respects AllowPublicStatusHistory", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/statuses/1/history", nil, nil, nil)
		require.NoError(t, err)

		h.cfg.AllowPublicStatusHistory = true
		h.performOptionalHistoryAuth(ctx, "1")
	})

	t.Run("performOptionalHistoryAuth handles missing/malformed/valid auth headers", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
		h.cfg.AllowPublicStatusHistory = false

		ctxMissing, err := round10NewLiftContext(http.MethodGet, "/api/v1/statuses/1/history", nil, nil, nil)
		require.NoError(t, err)
		h.performOptionalHistoryAuth(ctxMissing, "1")

		ctxBad, err := round10NewLiftContext(http.MethodGet, "/api/v1/statuses/1/history", map[string]string{"Authorization": "not-bearer"}, nil, nil)
		require.NoError(t, err)
		h.performOptionalHistoryAuth(ctxBad, "1")

		token := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeRead})
		ctxOK, err := round10NewLiftContext(http.MethodGet, "/api/v1/statuses/1/history", map[string]string{"Authorization": "Bearer " + token}, nil, nil)
		require.NoError(t, err)
		h.performOptionalHistoryAuth(ctxOK, "1")
	})

	t.Run("fetchObjectForHistory returns note and errors", func(t *testing.T) {
		notesStub := &NotesServiceStub{
			GetNoteFunc: func(_ context.Context, statusID string) (*storagemodels.Status, error) {
				if statusID == "s1" {
					return &storagemodels.Status{
						Note: &activitypub.Note{BaseObject: activitypub.BaseObject{ID: "s1"}, Content: "hello"},
					}, nil
				}
				return nil, errors.New("not found")
			},
		}

		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{NotesSvc: notesStub})
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/statuses/1/history", nil, nil, nil)
		require.NoError(t, err)

		obj, err := h.fetchObjectForHistory(ctx, h.cfg.BaseURL()+"/objects/s1")
		require.NoError(t, err)
		require.IsType(t, &activitypub.Note{}, obj)
		require.Equal(t, "hello", obj.(*activitypub.Note).Content)

		_, err = h.fetchObjectForHistory(ctx, h.cfg.BaseURL()+"/objects/missing")
		require.Error(t, err)
	})
}

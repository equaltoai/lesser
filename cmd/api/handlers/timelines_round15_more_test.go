package handlers

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/services/notes"
	"github.com/stretchr/testify/require"
)

func TestTimelines_DirectTimeline_ErrorBranches_Round15(t *testing.T) {
	cfg := round11TestConfig()

	t.Run("missing token returns unauthorized", func(t *testing.T) {
		notesStub := &NotesServiceStub{
			ListNotesFunc: func(context.Context, *notes.ListNotesQuery) (*notes.Result, error) {
				return nil, errors.New("should not be called")
			},
		}
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{NotesSvc: notesStub})

		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/timelines/direct", nil, nil, nil)
		require.NoError(t, err)
		requireStatus(t, http.StatusUnauthorized)(h.HandleGetDirectTimelineLift(ctx))
	})

	t.Run("insufficient scope returns forbidden", func(t *testing.T) {
		token := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeWrite})
		headers := map[string]string{"Authorization": "Bearer " + token}

		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{NotesSvc: &NotesServiceStub{}})
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/timelines/direct", headers, nil, nil)
		require.NoError(t, err)

		requireStatus(t, http.StatusForbidden)(h.HandleGetDirectTimelineLift(ctx))
	})

	t.Run("notes service error returns internal error", func(t *testing.T) {
		token := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeRead})
		headers := map[string]string{"Authorization": "Bearer " + token}

		notesStub := &NotesServiceStub{
			ListNotesFunc: func(context.Context, *notes.ListNotesQuery) (*notes.Result, error) {
				return nil, errors.New("boom")
			},
		}
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{NotesSvc: notesStub})

		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/timelines/direct", headers, nil, nil)
		require.NoError(t, err)
		requireStatus(t, http.StatusInternalServerError)(h.HandleGetDirectTimelineLift(ctx))
	})
}

package handlers

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/services/notes"
	"github.com/equaltoai/lesser/pkg/storage"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestStatusInfoRound12_Coverage(t *testing.T) {
	cfg := round11TestConfig()
	now := time.Now()

	note := &activitypub.Note{
		BaseObject: activitypub.BaseObject{
			ID:        cfg.BaseURL() + "/objects/123",
			Type:      "Note",
			Summary:   "spoiler",
			Sensitive: true,
		},
		Content:      "hello",
		AttributedTo: cfg.BaseURL() + "/users/alice",
	}
	status := &storagemodels.Status{
		StatusID: "123",
		Note:     note,
	}

	t.Run("status source validation + not found", func(t *testing.T) {
		notesSvc := &NotesServiceStub{
			GetNoteFunc: func(_ context.Context, _ string) (*storagemodels.Status, error) {
				return nil, errors.New("missing")
			},
		}
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{NotesSvc: notesSvc})

		ctxBad, err := round10NewLiftContext(http.MethodGet, "/api/v1/statuses/bad id/source", nil, nil, nil)
		require.NoError(t, err)
		ctxBad.Params["id"] = "bad id"
		requireStatus(t, http.StatusBadRequest)(h.HandleGetStatusSourceLift(ctxBad))

		ctxNF, err := round10NewLiftContext(http.MethodGet, "/api/v1/statuses/123/source", nil, nil, nil)
		require.NoError(t, err)
		ctxNF.Params["id"] = "123"
		requireStatus(t, http.StatusNotFound)(h.HandleGetStatusSourceLift(ctxNF))
	})

	t.Run("history validation + not found", func(t *testing.T) {
		notesSvc := &NotesServiceStub{
			GetNoteFunc: func(_ context.Context, _ string) (*storagemodels.Status, error) {
				return nil, errors.New("missing")
			},
			GetNoteWithViewerFunc: func(_ context.Context, _ *notes.GetNoteQuery) (*storagemodels.Status, error) {
				return nil, errors.New("missing")
			},
		}
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{NotesSvc: notesSvc})

		ctxBad, err := round10NewLiftContext(http.MethodGet, "/api/v1/statuses/bad id/history", nil, nil, nil)
		require.NoError(t, err)
		ctxBad.Params["id"] = "bad id"
		requireStatus(t, http.StatusBadRequest)(h.HandleGetStatusHistoryLift(ctxBad))

		token := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeRead})
		headers := map[string]string{"authorization": "Bearer " + token}
		ctxNF, err := round10NewLiftContext(http.MethodGet, "/api/v1/statuses/123/history", headers, nil, nil)
		require.NoError(t, err)
		ctxNF.Params["id"] = "123"
		requireStatus(t, http.StatusNotFound)(h.HandleGetStatusHistoryLift(ctxNF))
	})

	t.Run("history optional auth paths + edit history failure", func(t *testing.T) {
		notesSvc := &NotesServiceStub{
			GetNoteFunc: func(_ context.Context, _ string) (*storagemodels.Status, error) {
				return status, nil
			},
			GetNoteWithViewerFunc: func(_ context.Context, _ *notes.GetNoteQuery) (*storagemodels.Status, error) {
				return status, nil
			},
			GetUpdateHistoryFunc: func(_ context.Context, _ *notes.GetUpdateHistoryQuery) (*notes.GetUpdateHistoryResult, error) {
				return nil, errors.New("history unavailable")
			},
		}
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{NotesSvc: notesSvc})
		h.cfg.AllowPublicStatusHistory = false

		token := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeRead})
		headers := map[string]string{"authorization": "Bearer " + token}

		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/statuses/123/history", headers, nil, nil)
		require.NoError(t, err)
		ctx.Params["id"] = "123"

		requireStatus(t, http.StatusOK)(h.HandleGetStatusHistoryLift(ctx))
	})

	t.Run("helper coverage for attribution + edits", func(t *testing.T) {
		accountsSvc := &AccountsServiceStub{
			GetAccountFunc: func(_ context.Context, username string) (*storage.Account, error) {
				return &storage.Account{
					Actor: &activitypub.Actor{
						BaseObject:                activitypub.BaseObject{ID: cfg.BaseURL() + "/users/" + username},
						PreferredUsername:         username,
						Name:                      username,
						ManuallyApprovesFollowers: false,
					},
				}, nil
			},
		}
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{AccountsSvc: accountsSvc})

		// extractHistoryAuthHeader fallback
		ctx, err := round10NewLiftContext(http.MethodGet, "/x", map[string]string{"authorization": "Bearer token"}, nil, nil)
		require.NoError(t, err)
		require.Equal(t, "Bearer token", h.extractHistoryAuthHeader(ctx))

		// normalizeStatusIDForHistory
		require.Equal(t, cfg.BaseURL()+"/objects/123", h.normalizeStatusIDForHistory("123"))
		require.Equal(t, cfg.BaseURL()+"/objects/123", h.normalizeStatusIDForHistory(cfg.BaseURL()+"/objects/123"))

		// extractAttributedTo + getHistoryAuthorActor
		require.Equal(t, cfg.BaseURL()+"/users/alice", h.extractAttributedTo(note))
		require.Equal(t, cfg.BaseURL()+"/users/alice", h.extractAttributedTo(map[string]any{"attributedTo": cfg.BaseURL() + "/users/alice"}))
		require.Equal(t, "", h.extractAttributedTo(map[string]any{"attributedTo": 123}))
		require.Nil(t, h.getHistoryAuthorActor(ctx, map[string]any{}))
		require.NotNil(t, h.getHistoryAuthorActor(ctx, map[string]any{"attributedTo": cfg.BaseURL() + "/users/alice"}))

		// extractEditContent covers note + map paths (and extractNoteContent branches)
		edit := models.StatusEdit{CreatedAt: "baseline"}
		published := now.Add(-1 * time.Hour)
		updated := now.Add(-30 * time.Minute)
		withUpdated := &activitypub.Note{
			BaseObject: activitypub.BaseObject{Updated: &updated, Summary: "s1", Sensitive: true},
			Content:    "c1",
		}
		h.extractEditContent(withUpdated, &edit)
		require.Equal(t, "c1", edit.Content)
		require.Equal(t, "s1", edit.SpoilerText)
		require.True(t, edit.Sensitive)

		edit2 := models.StatusEdit{CreatedAt: "baseline"}
		withPublished := &activitypub.Note{
			BaseObject: activitypub.BaseObject{Published: &published, Summary: "s2", Sensitive: false},
			Content:    "c2",
		}
		h.extractEditContent(withPublished, &edit2)
		require.Equal(t, "c2", edit2.Content)
		require.Equal(t, "s2", edit2.SpoilerText)

		edit3 := models.StatusEdit{CreatedAt: "baseline"}
		h.extractEditContent(map[string]any{
			"content":   "mapped",
			"summary":   "sum",
			"sensitive": true,
			"published": "2020-01-01T00:00:00Z",
		}, &edit3)
		require.Equal(t, "mapped", edit3.Content)
		require.Equal(t, "sum", edit3.SpoilerText)
	})
}

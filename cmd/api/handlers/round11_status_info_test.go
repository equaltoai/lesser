package handlers

import (
	"context"
	"net/http"
	"testing"
	"time"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/services/notes"
	"github.com/equaltoai/lesser/pkg/storage"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestStatusSourceAndHistory(t *testing.T) {
	h, _, _ := round11NewHandlerSliceC(t, nil)
	h.cfg.AllowPublicStatusHistory = true

	note := &activitypub.Note{
		BaseObject:   activitypub.BaseObject{ID: h.cfg.BaseURL() + "/objects/123", Type: "Note", Summary: "spoiler", Sensitive: true},
		Content:      "hello",
		AttributedTo: h.cfg.BaseURL() + "/users/alice",
	}
	status := &storagemodels.Status{
		StatusID:       "123",
		AuthorUsername: "alice",
		AuthorID:       h.cfg.BaseURL() + "/users/alice",
		Note:           note,
	}

	notesSvc := &NotesServiceStub{
		GetNoteFunc: func(ctx context.Context, statusID string) (*storagemodels.Status, error) {
			return status, nil
		},
		GetNoteWithViewerFunc: func(ctx context.Context, query *notes.GetNoteQuery) (*storagemodels.Status, error) {
			return status, nil
		},
		GetUpdateHistoryFunc: func(ctx context.Context, query *notes.GetUpdateHistoryQuery) (*notes.GetUpdateHistoryResult, error) {
			return &notes.GetUpdateHistoryResult{History: []*storage.UpdateHistory{
				{
					UpdatedAt: time.Now().Add(-2 * time.Hour),
					PreviousState: map[string]any{
						"content":   "old",
						"summary":   "old-summary",
						"sensitive": true,
					},
				},
			}}, nil
		},
	}
	accountsSvc := &AccountsServiceStub{
		GetAccountFunc: func(ctx context.Context, username string) (*storage.Account, error) {
			return &storage.Account{Actor: &activitypub.Actor{BaseObject: activitypub.BaseObject{ID: h.cfg.BaseURL() + "/users/" + username}, PreferredUsername: username}}, nil
		},
	}

	h.registry = &RegistryStub{NotesSvc: notesSvc, AccountsSvc: accountsSvc}

	readToken := round11SignAccessToken(t, h.cfg.JWTSecret, "alice", []string{auth.ScopeRead})
	readHeaders := map[string]string{"Authorization": "Bearer " + readToken}

	ctxSource, err := round10NewLiftContext(http.MethodGet, "/api/v1/statuses/123/source", readHeaders, nil, nil)
	require.NoError(t, err)
	ctxSource.Params["id"] = "123"
	requireStatus(t, http.StatusOK)(h.HandleGetStatusSourceLift(ctxSource))

	ctxHistory, err := round10NewLiftContext(http.MethodGet, "/api/v1/statuses/123/history", nil, nil, nil)
	require.NoError(t, err)
	ctxHistory.Params["id"] = "123"
	requireStatus(t, http.StatusOK)(h.HandleGetStatusHistoryLift(ctxHistory))
}

func TestStatusHistoryHelpers(t *testing.T) {
	h, _, _ := round11NewHandlerSliceC(t, nil)

	edit := apimodels.StatusEdit{}
	h.extractMapContent(map[string]any{
		"content":   "mapped",
		"summary":   "sum",
		"sensitive": true,
		"updated":   "2020-01-01T00:00:00Z",
	}, &edit)
	require.Equal(t, "mapped", edit.Content)
	require.Equal(t, "sum", edit.SpoilerText)
	require.True(t, edit.Sensitive)
}

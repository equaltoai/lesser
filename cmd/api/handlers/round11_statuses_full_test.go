package handlers

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/services/notes"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestStatusesFullHandlers(t *testing.T) {
	cfg := round11TestConfig()
	status := &storagemodels.Status{
		StatusID:       "s1",
		AuthorUsername: "alice",
		AuthorID:       cfg.ActorURL("alice"),
		Content:        "hello",
		Visibility:     "public",
		PublishedAt:    time.Now().Add(-1 * time.Hour),
		CreatedAt:      time.Now().Add(-1 * time.Hour),
		Note: &activitypub.Note{
			BaseObject: activitypub.BaseObject{ID: cfg.BaseURL() + "/objects/s1", Type: activitypub.NoteType},
			Content:    "hello",
		},
	}

	notesStub := &NotesServiceStub{
		CreateNoteFunc: func(_ context.Context, cmd *notes.CreateNoteCommand) (*notes.NoteResult, error) {
			if len(cmd.PollOptions) > 0 {
				require.Equal(t, []string{"a", "b"}, cmd.PollOptions)
			}
			return &notes.NoteResult{Note: status}, nil
		},
		GetNoteFunc: func(_ context.Context, _ string) (*storagemodels.Status, error) {
			return status, nil
		},
		GetNoteWithViewerFunc: func(_ context.Context, _ *notes.GetNoteQuery) (*storagemodels.Status, error) {
			return status, nil
		},
		DeleteNoteFunc: func(_ context.Context, _ *notes.DeleteNoteCommand) error {
			return nil
		},
	}

	handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{NotesSvc: notesStub})
	token := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeWrite, auth.ScopeRead})
	headers := map[string]string{"Authorization": "Bearer " + token}

	ctxCreate, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses", headers, nil, apimodels.CreateStatusRequest{Status: "hello", Poll: &apimodels.Poll{Options: []string{"a", "b"}, ExpiresIn: 60}})
	require.NoError(t, err)
	requireStatus(t, http.StatusUnprocessableEntity)(handler.HandleCreateStatusFull(ctxCreate))

	ctxCreateOK, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses", headers, nil, apimodels.CreateStatusRequest{Status: "hello"})
	require.NoError(t, err)
	requireStatus(t, http.StatusCreated)(handler.HandleCreateStatusFull(ctxCreateOK))

	ctxGet, err := round10NewLiftContext(http.MethodGet, "/api/v1/statuses/s1", headers, nil, nil)
	require.NoError(t, err)
	ctxGet.Params["id"] = "s1"
	requireStatus(t, http.StatusOK)(handler.HandleGetStatusFull(ctxGet))

	notesStub.DeleteNoteFunc = func(_ context.Context, _ *notes.DeleteNoteCommand) error {
		return errors.New("not found")
	}
	ctxDelete, err := round10NewLiftContext(http.MethodDelete, "/api/v1/statuses/s1", headers, nil, nil)
	require.NoError(t, err)
	ctxDelete.Params["id"] = "s1"
	requireStatus(t, http.StatusNotFound)(handler.HandleDeleteStatusFull(ctxDelete))

	notesStub.DeleteNoteFunc = func(_ context.Context, _ *notes.DeleteNoteCommand) error {
		return nil
	}
	ctxDeleteOK, err := round10NewLiftContext(http.MethodDelete, "/api/v1/statuses/s1", headers, nil, nil)
	require.NoError(t, err)
	ctxDeleteOK.Params["id"] = "s1"
	requireStatus(t, http.StatusOK)(handler.HandleDeleteStatusFull(ctxDeleteOK))
}

func TestStatusesFullPermissions(t *testing.T) {
	cfg := round11TestConfig()
	handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

	publicStatus := &storagemodels.Status{StatusID: "pub", AuthorUsername: "alice", Visibility: "public"}
	allowed, err := handler.checkStatusViewPermission(context.Background(), publicStatus, "")
	require.NoError(t, err)
	require.True(t, allowed)

	privateStatus := &storagemodels.Status{StatusID: "priv", AuthorUsername: "bob", Visibility: "private"}
	allowed, err = handler.checkStatusViewPermission(context.Background(), privateStatus, "alice")
	require.NoError(t, err)
	require.False(t, allowed)

	directStatus := &storagemodels.Status{
		StatusID:     "dm",
		Visibility:   "direct",
		Mentions:     []string{cfg.ActorURL("alice")},
		ToRecipients: []string{"https://example.com/users/alice"},
	}
	require.True(t, handler.isViewerMentioned(directStatus.Mentions, "alice"))
	require.True(t, handler.isViewerMentioned([]string{"alice"}, "alice"))
	require.False(t, handler.isViewerMentioned(directStatus.Mentions, ""))
	require.True(t, handler.isViewerInRecipientLists(directStatus, "alice"))
	require.True(t, handler.checkDirectMessageVisibility(directStatus, "alice"))
}

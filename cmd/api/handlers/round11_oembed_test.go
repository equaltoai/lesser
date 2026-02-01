package handlers

import (
	"context"
	"net/http"
	"testing"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestHandleOEmbedLift(t *testing.T) {
	h, _, _ := round11NewHandlerSliceC(t, nil)

	note := &activitypub.Note{
		BaseObject: activitypub.BaseObject{
			ID:      h.cfg.BaseURL() + "/objects/123",
			Type:    "Note",
			Summary: "spoiler",
			To:      []string{activitypub.PublicAddress},
		},
		Content:      "hello",
		AttributedTo: h.cfg.BaseURL() + "/users/alice",
		Attachment: []activitypub.Attachment{{
			Type:      "Document",
			MediaType: "image/png",
			URL:       "https://cdn.example.com/image.png",
		}},
	}

	status := &storagemodels.Status{
		StatusID: "123",
		Note:     &storagemodels.NoteField{Note: note},
	}

	notesSvc := &NotesServiceStub{
		GetNoteFunc: func(ctx context.Context, statusID string) (*storagemodels.Status, error) {
			return status, nil
		},
	}
	accountsSvc := &AccountsServiceStub{
		GetAccountFunc: func(ctx context.Context, username string) (*storage.Account, error) {
			return &storage.Account{Actor: &activitypub.Actor{BaseObject: activitypub.BaseObject{ID: h.cfg.BaseURL() + "/users/" + username}, PreferredUsername: username, Name: username, URL: h.cfg.BaseURL() + "/@" + username}}, nil
		},
	}

	h.registry = &RegistryStub{NotesSvc: notesSvc, AccountsSvc: accountsSvc}

	ctx, err := round10NewLiftContext(http.MethodGet, "/api/oembed", nil, map[string]string{
		"url":      h.cfg.BaseURL() + "/@alice/123",
		"format":   "xml",
		"maxwidth": "420",
	}, nil)
	require.NoError(t, err)

	resp := requireStatus(t, http.StatusOK)(h.HandleOEmbedLift(ctx))
	require.Contains(t, string(resp.Body), "<oembed>")
}

func TestOEmbedHelpers(t *testing.T) {
	h, _, _ := round11NewHandlerSliceC(t, nil)

	require.Equal(t, "123", h.extractStatusID("/@alice/123"))
	require.Equal(t, "123", h.extractStatusID("/web/@alice/123"))
	require.Equal(t, "123", h.extractStatusID("/users/alice/statuses/123"))
	require.Equal(t, "123", h.extractStatusID("/objects/123"))
}

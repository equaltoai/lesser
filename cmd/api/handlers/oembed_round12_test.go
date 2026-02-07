package handlers

import (
	"context"
	"encoding/json"
	stdErrors "errors"
	"net/http"
	"strings"
	"testing"
	"time"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestOEmbed_Round12(t *testing.T) {
	cfg := round11TestConfig()

	makeNote := func(t *testing.T, noteID string, embeddable bool) *activitypub.Note {
		t.Helper()
		published := time.Now().Add(-2 * time.Hour)
		note := &activitypub.Note{
			BaseObject: activitypub.BaseObject{
				ID:        noteID,
				Published: &published,
				Summary:   "spoiler",
			},
			Content:      "<p>hello</p>",
			AttributedTo: cfg.BaseURL() + "/users/alice",
			Attachment: []activitypub.Attachment{
				{Type: "Document", MediaType: "image/png", URL: "https://example.com/img.png", Name: "img"},
				{Type: "Document", MediaType: "video/mp4", URL: "https://example.com/video.mp4"},
			},
		}
		if embeddable {
			note.To = []string{activitypub.PublicAddress}
		}
		return note
	}

	makeHandler := func(t *testing.T, note *activitypub.Note, notesErr error, accountsErr error, actorName string) *Handler {
		t.Helper()

		notesSvc := &NotesServiceStub{
			GetNoteFunc: func(_ context.Context, id string) (*storagemodels.Status, error) {
				if notesErr != nil {
					return nil, notesErr
				}
				return &storagemodels.Status{
					StatusID: id,
					Note:     note,
				}, nil
			},
		}

		accountsSvc := &AccountsServiceStub{
			GetAccountFunc: func(_ context.Context, username string) (*storage.Account, error) {
				if accountsErr != nil {
					return nil, accountsErr
				}
				return &storage.Account{
					User: &storage.User{Username: username},
					Actor: &activitypub.Actor{
						BaseObject:        activitypub.BaseObject{ID: cfg.BaseURL() + "/users/" + username},
						PreferredUsername: username,
						Name:              actorName,
						URL:               cfg.BaseURL() + "/@" + username,
					},
				}, nil
			},
		}

		reg := &RegistryStub{NotesSvc: notesSvc, AccountsSvc: accountsSvc}
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, reg)
		return h
	}

	t.Run("extractStatusID supports known URL patterns", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
		require.Equal(t, "123", h.extractStatusID("/@alice/123"))
		require.Equal(t, "456", h.extractStatusID("/web/@alice/456"))
		require.Equal(t, "789", h.extractStatusID("/users/alice/statuses/789"))
		require.Equal(t, "999", h.extractStatusID("/objects/999"))
		require.Empty(t, h.extractStatusID("/nope"))
	})

	t.Run("HandleOEmbedLift json, xml, and errors", func(t *testing.T) {
		note := makeNote(t, cfg.BaseURL()+"/objects/123", true)
		h := makeHandler(t, note, nil, nil, "Alice")

		t.Run("missing url returns 400", func(t *testing.T) {
			ctx, err := round10NewLiftContext(http.MethodGet, "/api/oembed", nil, nil, nil)
			require.NoError(t, err)
			requireStatus(t, http.StatusBadRequest)(h.HandleOEmbedLift(ctx))
		})

		t.Run("invalid URL returns 400", func(t *testing.T) {
			ctx, err := round10NewLiftContext(http.MethodGet, "/api/oembed", nil, map[string]string{"url": "::::"}, nil)
			require.NoError(t, err)
			requireStatus(t, http.StatusBadRequest)(h.HandleOEmbedLift(ctx))
		})

		t.Run("foreign host returns 404", func(t *testing.T) {
			ctx, err := round10NewLiftContext(http.MethodGet, "/api/oembed", nil, map[string]string{"url": "https://evil.com/@alice/123"}, nil)
			require.NoError(t, err)
			requireStatus(t, http.StatusNotFound)(h.HandleOEmbedLift(ctx))
		})

		t.Run("json default format returns oembed struct", func(t *testing.T) {
			ctx, err := round10NewLiftContext(http.MethodGet, "/api/oembed", nil, map[string]string{
				"url":      cfg.BaseURL() + "/@alice/123",
				"maxwidth": "900",
			}, nil)
			require.NoError(t, err)
			resp := requireStatus(t, http.StatusOK)(h.HandleOEmbedLift(ctx))

			var body apimodels.OEmbedResponse
			require.NoError(t, json.Unmarshal(resp.Body, &body))
			require.Equal(t, "rich", body.Type)
			require.Equal(t, 900, body.Width)
			require.NotEmpty(t, body.HTML)
			require.NotEmpty(t, body.Title)
			require.NotEmpty(t, body.ThumbnailURL)
		})

		t.Run("xml format returns xml body", func(t *testing.T) {
			ctx, err := round10NewLiftContext(http.MethodGet, "/api/oembed", nil, map[string]string{
				"url":    cfg.BaseURL() + "/@alice/123",
				"format": "xml",
			}, nil)
			require.NoError(t, err)
			resp := requireStatus(t, http.StatusOK)(h.HandleOEmbedLift(ctx))
			require.Equal(t, "text/xml; charset=utf-8", firstStringValue(resp.Headers, "content-type"))
			require.Contains(t, string(resp.Body), "<oembed>")
		})

		t.Run("unsupported format returns 400", func(t *testing.T) {
			ctx, err := round10NewLiftContext(http.MethodGet, "/api/oembed", nil, map[string]string{
				"url":    cfg.BaseURL() + "/@alice/123",
				"format": "yaml",
			}, nil)
			require.NoError(t, err)
			requireStatus(t, http.StatusBadRequest)(h.HandleOEmbedLift(ctx))
		})

		t.Run("not embeddable returns 403", func(t *testing.T) {
			privateNote := makeNote(t, cfg.BaseURL()+"/objects/123", false)
			hPrivate := makeHandler(t, privateNote, nil, nil, "Alice")
			ctx, err := round10NewLiftContext(http.MethodGet, "/api/oembed", nil, map[string]string{"url": cfg.BaseURL() + "/@alice/123"}, nil)
			require.NoError(t, err)
			requireStatus(t, http.StatusForbidden)(hPrivate.HandleOEmbedLift(ctx))
		})

		t.Run("notes service error returns 404", func(t *testing.T) {
			hErr := makeHandler(t, note, stdErrors.New("boom"), nil, "Alice")
			ctx, err := round10NewLiftContext(http.MethodGet, "/api/oembed", nil, map[string]string{"url": cfg.BaseURL() + "/@alice/123"}, nil)
			require.NoError(t, err)
			requireStatus(t, http.StatusNotFound)(hErr.HandleOEmbedLift(ctx))
		})

		t.Run("accounts lookup error returns minimal actor", func(t *testing.T) {
			hAcctErr := makeHandler(t, note, nil, stdErrors.New("boom"), "Alice")
			ctx, err := round10NewLiftContext(http.MethodGet, "/api/oembed", nil, map[string]string{"url": cfg.BaseURL() + "/@alice/123"}, nil)
			require.NoError(t, err)
			resp := requireStatus(t, http.StatusOK)(hAcctErr.HandleOEmbedLift(ctx))

			var body apimodels.OEmbedResponse
			require.NoError(t, json.Unmarshal(resp.Body, &body))
			require.Equal(t, "alice", body.AuthorName)
			require.Equal(t, cfg.BaseURL()+"/users/alice", body.AuthorURL)
		})
	})

	t.Run("embed helpers: isStatusEmbeddable, timestamp, media", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

		note := makeNote(t, cfg.BaseURL()+"/objects/123", false)
		require.False(t, h.isStatusEmbeddable(note))
		note.CC = []string{activitypub.PublicAddress}
		require.True(t, h.isStatusEmbeddable(note))

		require.NotEmpty(t, h.formatEmbedTimestamp(note))
		note.Published = nil
		require.Equal(t, "Unknown", h.formatEmbedTimestamp(note))

		var builder strings.Builder
		h.writeEmbedMediaAttachments(&builder, note)
		require.Contains(t, builder.String(), "<img")
	})

	t.Run("HandleEmbedPageLift renders HTML and covers fallbacks", func(t *testing.T) {
		note := makeNote(t, cfg.BaseURL()+"/objects/123", true)

		t.Run("success with param", func(t *testing.T) {
			h := makeHandler(t, note, nil, nil, "")
			ctx, err := round10NewLiftContext(http.MethodGet, "/embed/123", nil, nil, nil)
			require.NoError(t, err)
			ctx.Params["id"] = "123"

			resp := requireStatus(t, http.StatusOK)(h.HandleEmbedPageLift(ctx))
			require.Equal(t, "text/html; charset=utf-8", firstStringValue(resp.Headers, "content-type"))
			require.Equal(t, "ALLOWALL", firstStringValue(resp.Headers, "x-frame-options"))
			require.Contains(t, firstStringValue(resp.Headers, "content-security-policy"), "frame-ancestors *")
			require.Contains(t, firstStringValue(resp.Headers, "content-security-policy"), "script-src 'nonce-")
			require.Contains(t, string(resp.Body), "<article")
			require.Contains(t, string(resp.Body), "hello")
			require.Contains(t, string(resp.Body), "nonce=\"")
		})

		t.Run("embed html sanitizes note content", func(t *testing.T) {
			malicious := makeNote(t, cfg.BaseURL()+"/objects/123", true)
			malicious.Content = `<img src=x onerror=alert(1)>hello`

			h := makeHandler(t, malicious, nil, nil, "")
			ctx, err := round10NewLiftContext(http.MethodGet, "/embed/123", nil, nil, nil)
			require.NoError(t, err)
			ctx.Params["id"] = "123"

			resp := requireStatus(t, http.StatusOK)(h.HandleEmbedPageLift(ctx))
			body := string(resp.Body)
			require.NotContains(t, body, "onerror=")
			require.Contains(t, body, "hello")
		})

		t.Run("path fallback extracts id", func(t *testing.T) {
			h := makeHandler(t, note, nil, nil, "Alice")
			ctx, err := round10NewLiftContext(http.MethodGet, "/embed/123", nil, nil, nil)
			require.NoError(t, err)
			// Do not set ctx.Param("id") to exercise fallback extraction.
			requireStatus(t, http.StatusOK)(h.HandleEmbedPageLift(ctx))
		})

		t.Run("missing id returns 400", func(t *testing.T) {
			h := makeHandler(t, note, nil, nil, "Alice")
			ctx, err := round10NewLiftContext(http.MethodGet, "/embed/", nil, nil, nil)
			require.NoError(t, err)
			requireStatus(t, http.StatusBadRequest)(h.HandleEmbedPageLift(ctx))
		})

		t.Run("not embeddable returns 403", func(t *testing.T) {
			privateNote := makeNote(t, cfg.BaseURL()+"/objects/123", false)
			h := makeHandler(t, privateNote, nil, nil, "Alice")
			ctx, err := round10NewLiftContext(http.MethodGet, "/embed/123", nil, nil, nil)
			require.NoError(t, err)
			ctx.Params["id"] = "123"
			requireStatus(t, http.StatusForbidden)(h.HandleEmbedPageLift(ctx))
		})

		t.Run("notes service error returns 404", func(t *testing.T) {
			h := makeHandler(t, note, stdErrors.New("boom"), nil, "Alice")
			ctx, err := round10NewLiftContext(http.MethodGet, "/embed/123", nil, nil, nil)
			require.NoError(t, err)
			ctx.Params["id"] = "123"
			requireStatus(t, http.StatusNotFound)(h.HandleEmbedPageLift(ctx))
		})
	})
}

package lift

import (
	"context"
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
					Note:     &storagemodels.NoteField{Note: note},
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
			require.NoError(t, h.HandleOEmbedLift(ctx))
			require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)
		})

		t.Run("invalid URL returns 400", func(t *testing.T) {
			ctx, err := round10NewLiftContext(http.MethodGet, "/api/oembed", nil, map[string]string{"url": "::::"}, nil)
			require.NoError(t, err)
			require.NoError(t, h.HandleOEmbedLift(ctx))
			require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)
		})

		t.Run("foreign host returns 404", func(t *testing.T) {
			ctx, err := round10NewLiftContext(http.MethodGet, "/api/oembed", nil, map[string]string{"url": "https://evil.com/@alice/123"}, nil)
			require.NoError(t, err)
			require.NoError(t, h.HandleOEmbedLift(ctx))
			require.Equal(t, http.StatusNotFound, ctx.Response.StatusCode)
		})

		t.Run("json default format returns oembed struct", func(t *testing.T) {
			ctx, err := round10NewLiftContext(http.MethodGet, "/api/oembed", nil, map[string]string{
				"url":      cfg.BaseURL() + "/@alice/123",
				"maxwidth": "900",
			}, nil)
			require.NoError(t, err)
			require.NoError(t, h.HandleOEmbedLift(ctx))
			require.Equal(t, http.StatusOK, ctx.Response.StatusCode)

			resp := ctx.Response.Body.(*apimodels.OEmbedResponse)
			require.Equal(t, "rich", resp.Type)
			require.Equal(t, 900, resp.Width)
			require.NotEmpty(t, resp.HTML)
			require.NotEmpty(t, resp.Title)
			require.NotEmpty(t, resp.ThumbnailURL)
		})

		t.Run("xml format returns xml body", func(t *testing.T) {
			ctx, err := round10NewLiftContext(http.MethodGet, "/api/oembed", nil, map[string]string{
				"url":    cfg.BaseURL() + "/@alice/123",
				"format": "xml",
			}, nil)
			require.NoError(t, err)
			require.NoError(t, h.HandleOEmbedLift(ctx))
			require.Equal(t, http.StatusOK, ctx.Response.StatusCode)
			require.Equal(t, "text/xml; charset=utf-8", ctx.Response.Headers["Content-Type"])
			require.Contains(t, ctx.Response.Body.(string), "<oembed>")
		})

		t.Run("unsupported format returns 400", func(t *testing.T) {
			ctx, err := round10NewLiftContext(http.MethodGet, "/api/oembed", nil, map[string]string{
				"url":    cfg.BaseURL() + "/@alice/123",
				"format": "yaml",
			}, nil)
			require.NoError(t, err)
			require.NoError(t, h.HandleOEmbedLift(ctx))
			require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)
		})

		t.Run("not embeddable returns 403", func(t *testing.T) {
			privateNote := makeNote(t, cfg.BaseURL()+"/objects/123", false)
			hPrivate := makeHandler(t, privateNote, nil, nil, "Alice")
			ctx, err := round10NewLiftContext(http.MethodGet, "/api/oembed", nil, map[string]string{"url": cfg.BaseURL() + "/@alice/123"}, nil)
			require.NoError(t, err)
			require.NoError(t, hPrivate.HandleOEmbedLift(ctx))
			require.Equal(t, http.StatusForbidden, ctx.Response.StatusCode)
		})

		t.Run("notes service error returns 404", func(t *testing.T) {
			hErr := makeHandler(t, note, stdErrors.New("boom"), nil, "Alice")
			ctx, err := round10NewLiftContext(http.MethodGet, "/api/oembed", nil, map[string]string{"url": cfg.BaseURL() + "/@alice/123"}, nil)
			require.NoError(t, err)
			require.NoError(t, hErr.HandleOEmbedLift(ctx))
			require.Equal(t, http.StatusNotFound, ctx.Response.StatusCode)
		})

		t.Run("accounts lookup error returns minimal actor", func(t *testing.T) {
			hAcctErr := makeHandler(t, note, nil, stdErrors.New("boom"), "Alice")
			ctx, err := round10NewLiftContext(http.MethodGet, "/api/oembed", nil, map[string]string{"url": cfg.BaseURL() + "/@alice/123"}, nil)
			require.NoError(t, err)
			require.NoError(t, hAcctErr.HandleOEmbedLift(ctx))
			require.Equal(t, http.StatusOK, ctx.Response.StatusCode)

			resp := ctx.Response.Body.(*apimodels.OEmbedResponse)
			require.Equal(t, "alice", resp.AuthorName)
			require.Equal(t, cfg.BaseURL()+"/users/alice", resp.AuthorURL)
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
			ctx.SetParam("id", "123")

			require.NoError(t, h.HandleEmbedPageLift(ctx))
			require.Equal(t, http.StatusOK, ctx.Response.StatusCode)
			require.Equal(t, "text/html", ctx.Response.Headers["Content-Type"])
			require.Equal(t, "ALLOWALL", ctx.Response.Headers["X-Frame-Options"])
			require.Contains(t, ctx.Response.Body.(string), "<article")
			require.Contains(t, ctx.Response.Body.(string), "hello")
		})

		t.Run("path fallback extracts id", func(t *testing.T) {
			h := makeHandler(t, note, nil, nil, "Alice")
			ctx, err := round10NewLiftContext(http.MethodGet, "/embed/123", nil, nil, nil)
			require.NoError(t, err)
			// Do not set ctx.Param("id") to exercise fallback extraction.
			require.NoError(t, h.HandleEmbedPageLift(ctx))
			require.Equal(t, http.StatusOK, ctx.Response.StatusCode)
		})

		t.Run("missing id returns 400", func(t *testing.T) {
			h := makeHandler(t, note, nil, nil, "Alice")
			ctx, err := round10NewLiftContext(http.MethodGet, "/embed/", nil, nil, nil)
			require.NoError(t, err)
			require.NoError(t, h.HandleEmbedPageLift(ctx))
			require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)
		})

		t.Run("not embeddable returns 403", func(t *testing.T) {
			privateNote := makeNote(t, cfg.BaseURL()+"/objects/123", false)
			h := makeHandler(t, privateNote, nil, nil, "Alice")
			ctx, err := round10NewLiftContext(http.MethodGet, "/embed/123", nil, nil, nil)
			require.NoError(t, err)
			ctx.SetParam("id", "123")
			require.NoError(t, h.HandleEmbedPageLift(ctx))
			require.Equal(t, http.StatusForbidden, ctx.Response.StatusCode)
		})

		t.Run("notes service error returns 404", func(t *testing.T) {
			h := makeHandler(t, note, stdErrors.New("boom"), nil, "Alice")
			ctx, err := round10NewLiftContext(http.MethodGet, "/embed/123", nil, nil, nil)
			require.NoError(t, err)
			ctx.SetParam("id", "123")
			require.NoError(t, h.HandleEmbedPageLift(ctx))
			require.Equal(t, http.StatusNotFound, ctx.Response.StatusCode)
		})
	})
}

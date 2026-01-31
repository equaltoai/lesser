package lift

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/stretchr/testify/require"
)

func TestWebFingerRound12_Coverage(t *testing.T) {
	cfg := round10TestConfig()
	logger := round10TestLogger(t)

	newHandler := func(accountsSvc AccountsService) *Handler {
		return &Handler{
			cfg:      cfg,
			logger:   logger,
			registry: &RegistryStub{AccountsSvc: accountsSvc},
		}
	}

	t.Run("fallback query param access", func(t *testing.T) {
		h := newHandler(&AccountsServiceStub{
			GetAccountFunc: func(_ context.Context, username string) (*storage.Account, error) {
				return &storage.Account{
					Actor: &activitypub.Actor{
						BaseObject:        activitypub.BaseObject{ID: cfg.BaseURL() + "/users/" + username},
						PreferredUsername: username,
					},
				}, nil
			},
		})

		ctx, err := round10NewLiftContext(http.MethodGet, "/.well-known/webfinger", nil, nil, nil)
		require.NoError(t, err)
		ctx.Request.QueryParams = nil
		ctx.Request.Request.QueryParams = map[string]string{"resource": "acct:alice@example.com"}

		require.NoError(t, h.HandleWebFingerLift(ctx))
		require.Equal(t, http.StatusOK, ctx.Response.StatusCode)
	})

	t.Run("invalid resource format", func(t *testing.T) {
		h := newHandler(&AccountsServiceStub{})
		ctx, err := round10NewLiftContext(http.MethodGet, "/.well-known/webfinger", nil, map[string]string{
			"resource": "acct:bad!@example.com",
		}, nil)
		require.NoError(t, err)

		require.NoError(t, h.HandleWebFingerLift(ctx))
		require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)
	})

	t.Run("parse error when resource is URL", func(t *testing.T) {
		h := newHandler(&AccountsServiceStub{})
		ctx, err := round10NewLiftContext(http.MethodGet, "/.well-known/webfinger", nil, map[string]string{
			"resource": "https://example.com/users/alice",
		}, nil)
		require.NoError(t, err)

		require.NoError(t, h.HandleWebFingerLift(ctx))
		require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)
	})

	t.Run("wrong domain + not found paths", func(t *testing.T) {
		h := newHandler(&AccountsServiceStub{
			GetAccountFunc: func(_ context.Context, _ string) (*storage.Account, error) {
				return nil, errors.New("not found")
			},
		})

		ctxWrongDomain, err := round10NewLiftContext(http.MethodGet, "/.well-known/webfinger", nil, map[string]string{
			"resource": "acct:alice@other.example",
		}, nil)
		require.NoError(t, err)
		require.NoError(t, h.HandleWebFingerLift(ctxWrongDomain))
		require.Equal(t, http.StatusNotFound, ctxWrongDomain.Response.StatusCode)

		ctxNotFound, err := round10NewLiftContext(http.MethodGet, "/.well-known/webfinger", nil, map[string]string{
			"resource": "acct:alice@example.com",
		}, nil)
		require.NoError(t, err)
		require.NoError(t, h.HandleWebFingerLift(ctxNotFound))
		require.Equal(t, http.StatusNotFound, ctxNotFound.Response.StatusCode)
	})

	t.Run("actor missing + no avatar link", func(t *testing.T) {
		hNilActor := newHandler(&AccountsServiceStub{
			GetAccountFunc: func(_ context.Context, _ string) (*storage.Account, error) {
				return &storage.Account{Actor: nil}, nil
			},
		})
		ctxNilActor, err := round10NewLiftContext(http.MethodGet, "/.well-known/webfinger", nil, map[string]string{
			"resource": "acct:alice@example.com",
		}, nil)
		require.NoError(t, err)
		require.NoError(t, hNilActor.HandleWebFingerLift(ctxNilActor))
		require.Equal(t, http.StatusNotFound, ctxNilActor.Response.StatusCode)

		hNoAvatar := newHandler(&AccountsServiceStub{
			GetAccountFunc: func(_ context.Context, username string) (*storage.Account, error) {
				return &storage.Account{
					Actor: &activitypub.Actor{
						BaseObject:        activitypub.BaseObject{ID: cfg.BaseURL() + "/users/" + username},
						PreferredUsername: username,
						Icon:              &activitypub.Image{URL: ""},
					},
				}, nil
			},
		})
		ctxNoAvatar, err := round10NewLiftContext(http.MethodGet, "/.well-known/webfinger", nil, map[string]string{
			"resource": "acct:alice@example.com",
		}, nil)
		require.NoError(t, err)
		require.NoError(t, hNoAvatar.HandleWebFingerLift(ctxNoAvatar))
		require.Equal(t, http.StatusOK, ctxNoAvatar.Response.StatusCode)
	})
}


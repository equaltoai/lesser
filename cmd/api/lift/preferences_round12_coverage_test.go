package lift

import (
	"context"
	"errors"
	"net/http"
	"testing"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/services/accounts"
	"github.com/stretchr/testify/require"
)

func TestPreferencesRound12_Coverage(t *testing.T) {
	cfg := round11TestConfig()

	t.Run("get preferences auth failures", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{AccountsSvc: &AccountsServiceStub{}})

		ctxMissing, err := round10NewLiftContext(http.MethodGet, "/api/v1/preferences", nil, nil, nil)
		require.NoError(t, err)
		require.NoError(t, handler.HandleGetPreferencesLift(ctxMissing))
		require.Equal(t, http.StatusUnauthorized, ctxMissing.Response.StatusCode)

		ctxBadHeader, err := round10NewLiftContext(http.MethodGet, "/api/v1/preferences", map[string]string{"Authorization": "nope"}, nil, nil)
		require.NoError(t, err)
		require.NoError(t, handler.HandleGetPreferencesLift(ctxBadHeader))
		require.Equal(t, http.StatusUnauthorized, ctxBadHeader.Response.StatusCode)

		ctxBadToken, err := round10NewLiftContext(http.MethodGet, "/api/v1/preferences", map[string]string{"Authorization": "Bearer invalid"}, nil, nil)
		require.NoError(t, err)
		require.NoError(t, handler.HandleGetPreferencesLift(ctxBadToken))
		require.Equal(t, http.StatusUnauthorized, ctxBadToken.Response.StatusCode)

		writeToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeWrite})
		ctxScope, err := round10NewLiftContext(http.MethodGet, "/api/v1/preferences", map[string]string{"Authorization": "Bearer " + writeToken}, nil, nil)
		require.NoError(t, err)
		require.NoError(t, handler.HandleGetPreferencesLift(ctxScope))
		require.Equal(t, http.StatusForbidden, ctxScope.Response.StatusCode)
	})

	t.Run("get preferences mapping defaults for wrong types", func(t *testing.T) {
		accountsStub := &AccountsServiceStub{
			GetPreferencesFunc: func(_ context.Context, _ *accounts.GetPreferencesQuery) (*accounts.PreferencesResult, error) {
				return &accounts.PreferencesResult{
					Preferences: map[string]interface{}{
						"default_posting_visibility": 123,        // wrong type -> default "public"
						"default_media_sensitive":     true,       // bool ok
						"language":                   "es",       // string ok
						"expand_media":               "show_all", // maps to show_all
						"expand_spoilers":            "yes",      // wrong type -> default false
						"auto_play_gif":              false,      // bool ok
					},
				}, nil
			},
		}

		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{AccountsSvc: accountsStub})
		readToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeRead})

		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/preferences", map[string]string{"Authorization": "Bearer " + readToken}, nil, nil)
		require.NoError(t, err)
		require.NoError(t, handler.HandleGetPreferencesLift(ctx))

		prefs := ctx.Response.Body.(apimodels.Preferences)
		require.Equal(t, "public", prefs.PostingDefaultVisibility)
		require.True(t, prefs.PostingDefaultSensitive)
		require.Equal(t, "es", prefs.PostingDefaultLanguage)
		require.Equal(t, "show_all", prefs.ReadingExpandMedia)
		require.False(t, prefs.ReadingExpandSpoilers)
		require.False(t, prefs.ReadingAutoplayGifs)
	})

	t.Run("update preferences invalid json fallback + save error", func(t *testing.T) {
		var updateCalled bool
		accountsStub := &AccountsServiceStub{
			GetPreferencesFunc: func(_ context.Context, _ *accounts.GetPreferencesQuery) (*accounts.PreferencesResult, error) {
				return nil, errors.New("no prefs")
			},
			UpdatePreferencesFunc: func(_ context.Context, _ *accounts.UpdatePreferencesCommand) (*accounts.PreferencesResult, error) {
				updateCalled = true
				return nil, errors.New("update failed")
			},
		}

		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{AccountsSvc: accountsStub})
		writeToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeWrite})
		headers := map[string]string{"Authorization": "Bearer " + writeToken}

		ctxBad := round10NewLiftContextWithBodyBytes(http.MethodPatch, "/api/v1/preferences", headers, nil, []byte(`{invalid}`))
		require.NoError(t, handler.HandleUpdatePreferencesLift(ctxBad))
		require.Equal(t, http.StatusBadRequest, ctxBad.Response.StatusCode)
		require.False(t, updateCalled)

		update := map[string]interface{}{
			"posting:default:visibility": 123,   // wrong type -> ignored
			"posting:default:sensitive":  "no", // wrong type -> ignored
			"reading:expand:media":       "hide_all",
			"reading:autoplay:gifs":      true,
		}
		ctxErr, err := round10NewLiftContext(http.MethodPatch, "/api/v1/preferences", headers, nil, update)
		require.NoError(t, err)
		require.NoError(t, handler.HandleUpdatePreferencesLift(ctxErr))
		require.Equal(t, http.StatusInternalServerError, ctxErr.Response.StatusCode)
		require.True(t, updateCalled)
	})
}


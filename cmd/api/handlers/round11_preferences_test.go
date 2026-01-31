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

func TestPreferencesGetDefaults(t *testing.T) {
	cfg := round11TestConfig()
	accountsStub := &AccountsServiceStub{
		GetPreferencesFunc: func(_ context.Context, _ *accounts.GetPreferencesQuery) (*accounts.PreferencesResult, error) {
			return nil, errors.New("missing prefs")
		},
	}

	handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{AccountsSvc: accountsStub})
	token := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeRead})

	ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/preferences", map[string]string{"Authorization": "Bearer " + token}, nil, nil)
	require.NoError(t, err)
	require.NoError(t, handler.HandleGetPreferencesLift(ctx))

	prefs := ctx.Response.Body.(apimodels.Preferences)
	require.Equal(t, "public", prefs.PostingDefaultVisibility)
	require.Equal(t, "default", prefs.ReadingExpandMedia)
}

func TestPreferencesUpdate(t *testing.T) {
	cfg := round11TestConfig()
	accountsStub := &AccountsServiceStub{
		GetPreferencesFunc: func(_ context.Context, _ *accounts.GetPreferencesQuery) (*accounts.PreferencesResult, error) {
			return &accounts.PreferencesResult{Preferences: map[string]interface{}{"language": "en", "expand_media": "show_all"}}, nil
		},
		UpdatePreferencesFunc: func(_ context.Context, _ *accounts.UpdatePreferencesCommand) (*accounts.PreferencesResult, error) {
			return &accounts.PreferencesResult{Preferences: map[string]interface{}{}}, nil
		},
	}

	handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{AccountsSvc: accountsStub})
	token := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeWrite})
	headers := map[string]string{"Authorization": "Bearer " + token}

	update := map[string]interface{}{
		"posting:default:visibility": "unlisted",
		"posting:default:sensitive":  true,
		"posting:default:language":   "fr",
		"reading:expand:spoilers":    true,
		"reading:expand:media":       "hide_all",
		"reading:autoplay:gifs":      true,
	}
	ctx, err := round10NewLiftContext(http.MethodPatch, "/api/v1/preferences", headers, nil, update)
	require.NoError(t, err)
	require.NoError(t, handler.HandleUpdatePreferencesLift(ctx))

	resp := ctx.Response.Body.(apimodels.Preferences)
	require.Equal(t, "unlisted", resp.PostingDefaultVisibility)
	require.True(t, resp.PostingDefaultSensitive)
	require.Equal(t, "hide_all", resp.ReadingExpandMedia)

	require.Equal(t, "default", handler.mapExpandMediaPreference("unknown"))
}

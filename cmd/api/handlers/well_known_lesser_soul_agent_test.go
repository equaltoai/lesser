package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/auth"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestHandleWellKnownLesserSoulAgentLift(t *testing.T) {
	t.Parallel()

	t.Run("not_found_when_handler_missing_deps", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodGet, "/.well-known/lesser-soul-agent", nil, nil, nil)
		require.NoError(t, err)

		resp, err := (&Handler{}).HandleWellKnownLesserSoulAgentLift(ctx)
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusNotFound, resp.Status)
	})

	t.Run("internal_error_when_repo_lookup_fails", func(t *testing.T) {
		state := &round10QueryState{firstErrorOnce: errors.New("boom")}
		h, _, _ := round11NewHandlerSliceC(t, state)

		ctx, err := round10NewLiftContext(http.MethodGet, "/.well-known/lesser-soul-agent", nil, nil, nil)
		require.NoError(t, err)

		resp, err := h.HandleWellKnownLesserSoulAgentLift(ctx)
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusInternalServerError, resp.Status)
	})

	t.Run("not_found_when_missing_config", func(t *testing.T) {
		h, _, _ := round11NewHandlerSliceC(t, nil)
		ctx, err := round10NewLiftContext(http.MethodGet, "/.well-known/lesser-soul-agent", nil, nil, nil)
		require.NoError(t, err)

		resp, err := h.HandleWellKnownLesserSoulAgentLift(ctx)
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusNotFound, resp.Status)
	})

	t.Run("not_found_when_expired", func(t *testing.T) {
		h, _, _ := round11NewHandlerSliceC(t, nil)
		require.NoError(t, h.repos.Instance().SetWellKnownLesserSoulAgent(context.Background(), &storagemodels.InstanceWellKnownLesserSoulAgent{
			ProofValue: "abc",
			ExpiresAt:  time.Now().Add(-1 * time.Minute),
		}))

		ctx, err := round10NewLiftContext(http.MethodGet, "/.well-known/lesser-soul-agent", nil, nil, nil)
		require.NoError(t, err)

		resp, err := h.HandleWellKnownLesserSoulAgentLift(ctx)
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusNotFound, resp.Status)
	})

	t.Run("not_found_when_token_empty_after_trim", func(t *testing.T) {
		h, _, _ := round11NewHandlerSliceC(t, nil)
		require.NoError(t, h.repos.Instance().SetWellKnownLesserSoulAgent(context.Background(), &storagemodels.InstanceWellKnownLesserSoulAgent{
			ProofValue: "lesser-soul-agent=",
		}))

		ctx, err := round10NewLiftContext(http.MethodGet, "/.well-known/lesser-soul-agent", nil, nil, nil)
		require.NoError(t, err)

		resp, err := h.HandleWellKnownLesserSoulAgentLift(ctx)
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusNotFound, resp.Status)
	})

	t.Run("success_trims_prefix_and_sets_no_store", func(t *testing.T) {
		h, _, _ := round11NewHandlerSliceC(t, nil)
		require.NoError(t, h.repos.Instance().SetWellKnownLesserSoulAgent(context.Background(), &storagemodels.InstanceWellKnownLesserSoulAgent{
			ProofValue: " lesser-soul-agent=abc123 ",
		}))

		ctx, err := round10NewLiftContext(http.MethodGet, "/.well-known/lesser-soul-agent", nil, nil, nil)
		require.NoError(t, err)

		resp, err := h.HandleWellKnownLesserSoulAgentLift(ctx)
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusOK, resp.Status)
		require.Equal(t, []string{"no-store"}, resp.Headers["cache-control"])

		var body map[string]string
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		require.Equal(t, map[string]string{"lesser-soul-agent": "abc123"}, body)
	})
}

func TestHandleAdminSetSoulWellKnownProofLift(t *testing.T) {
	t.Parallel()

	t.Run("unauthorized_without_token", func(t *testing.T) {
		h, _, _ := round11NewHandlerSliceC(t, nil)
		ctx, err := round10NewLiftContext(http.MethodPut, "/api/v1/admin/soul/well-known", nil, nil, map[string]any{"proof_value": "abc"})
		require.NoError(t, err)

		resp, err := h.HandleAdminSetSoulWellKnownProofLift(ctx)
		require.NoError(t, err)
		require.Equal(t, http.StatusUnauthorized, resp.Status)
	})

	t.Run("internal_error_when_repos_missing_after_auth", func(t *testing.T) {
		base, _, _ := round11NewHandlerSliceC(t, nil)
		token := round11SignToken(t, base.cfg.JWTSecret, "admin", []string{auth.ScopeAdmin}, "sess-1")
		headers := map[string]string{"Authorization": "Bearer " + token}

		h := &Handler{cfg: base.cfg, logger: base.logger}
		ctx, err := round10NewLiftContext(http.MethodPut, "/api/v1/admin/soul/well-known", headers, nil, map[string]any{"proof_value": "abc"})
		require.NoError(t, err)

		resp, err := h.HandleAdminSetSoulWellKnownProofLift(ctx)
		require.NoError(t, err)
		require.Equal(t, http.StatusInternalServerError, resp.Status)
	})

	t.Run("insufficient_scope_returns_forbidden", func(t *testing.T) {
		h, _, _ := round11NewHandlerSliceC(t, nil)
		token := round11SignToken(t, h.cfg.JWTSecret, "alice", []string{auth.ScopeRead}, "sess-1")
		headers := map[string]string{"Authorization": "Bearer " + token}
		ctx, err := round10NewLiftContext(http.MethodPut, "/api/v1/admin/soul/well-known", headers, nil, map[string]any{"proof_value": "abc"})
		require.NoError(t, err)

		resp, err := h.HandleAdminSetSoulWellKnownProofLift(ctx)
		require.NoError(t, err)
		require.Equal(t, http.StatusForbidden, resp.Status)
	})

	t.Run("bad_request_when_body_invalid_json", func(t *testing.T) {
		h, _, _ := round11NewHandlerSliceC(t, nil)
		token := round11SignToken(t, h.cfg.JWTSecret, "admin", []string{auth.ScopeAdmin}, "sess-1")

		headers := map[string]string{"Authorization": "Bearer " + token}
		ctx := round10NewLiftContextWithBodyBytes(http.MethodPut, "/api/v1/admin/soul/well-known", headers, nil, []byte("{not-json"))

		resp, err := h.HandleAdminSetSoulWellKnownProofLift(ctx)
		require.NoError(t, err)
		require.Equal(t, http.StatusBadRequest, resp.Status)
	})

	t.Run("validation_errors_for_proof_and_expiry", func(t *testing.T) {
		h, _, _ := round11NewHandlerSliceC(t, nil)
		token := round11SignToken(t, h.cfg.JWTSecret, "admin", []string{auth.ScopeAdmin}, "sess-1")
		headers := map[string]string{"Authorization": "Bearer " + token}

		ctx, err := round10NewLiftContext(http.MethodPut, "/api/v1/admin/soul/well-known", headers, nil, map[string]any{
			"proof_value": "abc def",
		})
		require.NoError(t, err)

		resp, err := h.HandleAdminSetSoulWellKnownProofLift(ctx)
		require.NoError(t, err)
		require.Equal(t, http.StatusBadRequest, resp.Status)

		expiresIn := 0
		ctx, err = round10NewLiftContext(http.MethodPut, "/api/v1/admin/soul/well-known", headers, nil, map[string]any{
			"proof_value":        "abc",
			"expires_in_seconds": &expiresIn,
		})
		require.NoError(t, err)

		resp, err = h.HandleAdminSetSoulWellKnownProofLift(ctx)
		require.NoError(t, err)
		require.Equal(t, http.StatusBadRequest, resp.Status)
	})

	t.Run("validation_errors_for_empty_and_long_tokens", func(t *testing.T) {
		h, _, _ := round11NewHandlerSliceC(t, nil)
		token := round11SignToken(t, h.cfg.JWTSecret, "admin", []string{auth.ScopeAdmin}, "sess-1")
		headers := map[string]string{"Authorization": "Bearer " + token}

		ctx, err := round10NewLiftContext(http.MethodPut, "/api/v1/admin/soul/well-known", headers, nil, map[string]any{
			"proof_value": "lesser-soul-agent=",
		})
		require.NoError(t, err)

		resp, err := h.HandleAdminSetSoulWellKnownProofLift(ctx)
		require.NoError(t, err)
		require.Equal(t, http.StatusBadRequest, resp.Status)

		ctx, err = round10NewLiftContext(http.MethodPut, "/api/v1/admin/soul/well-known", headers, nil, map[string]any{
			"proof_value": strings.Repeat("a", 513),
		})
		require.NoError(t, err)

		resp, err = h.HandleAdminSetSoulWellKnownProofLift(ctx)
		require.NoError(t, err)
		require.Equal(t, http.StatusBadRequest, resp.Status)
	})

	t.Run("internal_error_when_persist_fails", func(t *testing.T) {
		state := &round10QueryState{updateErrorOnce: errors.New("boom")}
		h, _, _ := round11NewHandlerSliceC(t, state)

		token := round11SignToken(t, h.cfg.JWTSecret, "admin", []string{auth.ScopeAdmin}, "sess-1")
		headers := map[string]string{"Authorization": "Bearer " + token}

		expiresIn := 60
		ctx, err := round10NewLiftContext(http.MethodPut, "/api/v1/admin/soul/well-known", headers, nil, map[string]any{
			"proof_value":        "abc",
			"expires_in_seconds": &expiresIn,
		})
		require.NoError(t, err)

		resp, err := h.HandleAdminSetSoulWellKnownProofLift(ctx)
		require.NoError(t, err)
		require.Equal(t, http.StatusInternalServerError, resp.Status)
	})

	t.Run("success_sets_and_clears_proof", func(t *testing.T) {
		h, _, _ := round11NewHandlerSliceC(t, nil)
		token := round11SignToken(t, h.cfg.JWTSecret, "admin", []string{auth.ScopeAdmin}, "sess-1")
		headers := map[string]string{"Authorization": "Bearer " + token}

		expiresIn := 60
		ctx, err := round10NewLiftContext(http.MethodPut, "/api/v1/admin/soul/well-known", headers, nil, map[string]any{
			"proof_value":        "lesser-soul-agent=abc",
			"expires_in_seconds": &expiresIn,
		})
		require.NoError(t, err)

		resp, err := h.HandleAdminSetSoulWellKnownProofLift(ctx)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.Status)

		var body map[string]string
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		require.Equal(t, "abc", body["proof_value"])
		require.NotEmpty(t, body["expires_at"])

		ctx, err = round10NewLiftContext(http.MethodPut, "/api/v1/admin/soul/well-known", headers, nil, map[string]any{
			"proof_value": "",
		})
		require.NoError(t, err)

		resp, err = h.HandleAdminSetSoulWellKnownProofLift(ctx)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.Status)

		body = nil
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		require.Equal(t, "", body["proof_value"])
		require.NotContains(t, body, "expires_at")
	})
}

package handlers

import (
	stdErrors "errors"
	"net/http"
	"strings"
	"testing"
	"time"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestQuotes_Round12_CreateQuotePost_Coverage(t *testing.T) {
	cfg := round11TestConfig()

	t.Run("missing_status_id", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses//quote", nil, nil, apimodels.CreateQuotePostRequest{Status: "hi"})
		require.NoError(t, err)

		requireStatus(t, http.StatusBadRequest)(handler.HandleCreateQuotePostLift(ctx))
	})

	t.Run("unauthorized_missing_token", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{
			statusByID: map[string]storagemodels.Status{"s1": {StatusID: "s1"}},
		})
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses/s1/quote", nil, nil, apimodels.CreateQuotePostRequest{Status: "hi"})
		require.NoError(t, err)
		ctx.Params["id"] = "s1"

		requireStatus(t, http.StatusUnauthorized)(handler.HandleCreateQuotePostLift(ctx))
	})

	t.Run("unauthorized_invalid_token", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{
			statusByID: map[string]storagemodels.Status{"s1": {StatusID: "s1"}},
		})
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses/s1/quote", map[string]string{"Authorization": "Bearer bad"}, nil, apimodels.CreateQuotePostRequest{Status: "hi"})
		require.NoError(t, err)
		ctx.Params["id"] = "s1"

		requireStatus(t, http.StatusUnauthorized)(handler.HandleCreateQuotePostLift(ctx))
	})

	t.Run("insufficient_scope", func(t *testing.T) {
		readToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeRead})
		headers := map[string]string{"Authorization": "Bearer " + readToken}

		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{
			statusByID: map[string]storagemodels.Status{"s1": {StatusID: "s1"}},
		})
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses/s1/quote", headers, nil, apimodels.CreateQuotePostRequest{Status: "hi"})
		require.NoError(t, err)
		ctx.Params["id"] = "s1"

		requireStatus(t, http.StatusForbidden)(handler.HandleCreateQuotePostLift(ctx))
	})

	t.Run("parse_fallback_and_content_validation_error", func(t *testing.T) {
		writeToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{"write:statuses"})
		headers := map[string]string{
			"Authorization": "Bearer " + writeToken,
			"Content-Type":  "application/x-www-form-urlencoded",
		}

		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{
			statusByID: map[string]storagemodels.Status{"s1": {StatusID: "s1"}},
		})

		long := strings.Repeat("a", 501)
		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v1/statuses/s1/quote", headers, nil, []byte(`{"status":"`+long+`"}`))
		ctx.Params["id"] = "s1"

		resp, err := handler.HandleCreateQuotePostLift(ctx)
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.NotEqual(t, http.StatusOK, resp.Status)
	})

	t.Run("invalid_json_body", func(t *testing.T) {
		writeToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{"write:statuses"})
		headers := map[string]string{"Authorization": "Bearer " + writeToken, "Content-Type": "application/json"}

		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{
			statusByID: map[string]storagemodels.Status{"s1": {StatusID: "s1"}},
		})

		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v1/statuses/s1/quote", headers, nil, []byte(`{invalid}`))
		ctx.Params["id"] = "s1"

		requireStatus(t, http.StatusBadRequest)(handler.HandleCreateQuotePostLift(ctx))
	})

	t.Run("request_body_fallback_uses_request_request_body", func(t *testing.T) {
		writeToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{"write:statuses"})
		headers := map[string]string{
			"Authorization": "Bearer " + writeToken,
			"Content-Type":  "application/x-www-form-urlencoded",
		}

		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{
			statusByID: map[string]storagemodels.Status{"s1": {StatusID: "s1"}},
		})

		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v1/statuses/s1/quote", headers, nil, []byte(`{"status":"hi"}`))
		ctx.Params["id"] = "s1"
		ctx.Request.Body = nil

		requireStatus(t, http.StatusBadRequest)(handler.HandleCreateQuotePostLift(ctx))
	})

}

func TestDeleteQuoteAcceptsOnlyCanonicalOrLegacyOwnerIdentity(t *testing.T) {
	cfg := round11TestConfig()
	writeToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{"write:statuses"})
	headers := map[string]string{"Authorization": "Bearer " + writeToken}
	tests := []struct {
		name     string
		quoterID string
		want     int
	}{
		{name: "production actor URI owner", quoterID: common.GenerateActorID(cfg.Domain, "alice"), want: http.StatusOK},
		{name: "local actor URI non-owner", quoterID: common.GenerateActorID(cfg.Domain, "bob"), want: http.StatusNotFound},
		{name: "remote actor URI", quoterID: "https://remote.example/users/alice", want: http.StatusNotFound},
		{name: "legacy bare username owner", quoterID: "alice", want: http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rel := storagemodels.QuoteRelationship{
				QuoterNoteID: "q1",
				TargetNoteID: "s1",
				QuoterID:     tt.quoterID,
				Timestamp:    time.Now().Add(-10 * time.Minute),
			}
			rel.GenerateID()
			require.NoError(t, rel.UpdateKeys())
			handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{
				quoteRelationships: []storagemodels.QuoteRelationship{rel},
			})
			ctx, err := round10NewLiftContext(http.MethodDelete, "/api/v1/statuses/s1/quote/q1", headers, nil, nil)
			require.NoError(t, err)
			ctx.Params["id"] = "s1"
			ctx.Params["quote_id"] = "q1"

			requireStatus(t, tt.want)(handler.HandleDeleteQuotePostLift(ctx))
		})
	}
}

func TestDeleteQuoteReturnsUniformNotFoundForMissingAndNonOwner(t *testing.T) {
	cfg := round11TestConfig()
	writeToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{"write:statuses"})
	headers := map[string]string{"Authorization": "Bearer " + writeToken}

	nonOwner := storagemodels.QuoteRelationship{
		QuoterNoteID: "q1",
		TargetNoteID: "s1",
		QuoterID:     common.GenerateActorID(cfg.Domain, "bob"),
		Timestamp:    time.Now().Add(-10 * time.Minute),
	}
	nonOwner.GenerateID()
	require.NoError(t, nonOwner.UpdateKeys())

	tests := []struct {
		name          string
		relationships []storagemodels.QuoteRelationship
	}{
		{name: "missing relationship"},
		{name: "relationship owned by another account", relationships: []storagemodels.QuoteRelationship{nonOwner}},
	}
	var expectedBody []byte
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{quoteRelationships: tt.relationships})
			ctx, err := round10NewLiftContext(http.MethodDelete, "/api/v1/statuses/s1/quote/q1", headers, nil, nil)
			require.NoError(t, err)
			ctx.Params["id"] = "s1"
			ctx.Params["quote_id"] = "q1"

			resp := requireStatus(t, http.StatusNotFound)(handler.HandleDeleteQuotePostLift(ctx))
			if expectedBody == nil {
				expectedBody = append([]byte(nil), resp.Body...)
			}
			require.Equal(t, expectedBody, resp.Body)
			require.JSONEq(t, `{"error":"quote not found","error_code":"NOT_FOUND"}`, string(resp.Body))
		})
	}
}

func TestQuotes_Round12_GetQuotesOfStatus_Coverage(t *testing.T) {
	cfg := round11TestConfig()
	handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

	t.Run("invalid_status_id", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/statuses//quotes", nil, nil, nil)
		require.NoError(t, err)

		requireStatus(t, http.StatusBadRequest)(handler.HandleGetQuotesOfStatusLift(ctx))
	})

	t.Run("invalid_limit", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/statuses/s1/quotes", nil, map[string]string{"limit": "bad"}, nil)
		require.NoError(t, err)
		ctx.Params["id"] = "s1"

		requireStatus(t, http.StatusBadRequest)(handler.HandleGetQuotesOfStatusLift(ctx))
	})

	t.Run("invalid_offset", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/statuses/s1/quotes", nil, map[string]string{"offset": "bad"}, nil)
		require.NoError(t, err)
		ctx.Params["id"] = "s1"

		requireStatus(t, http.StatusBadRequest)(handler.HandleGetQuotesOfStatusLift(ctx))
	})
}

func TestQuotes_Round12_DeleteAndPermissions_Coverage(t *testing.T) {
	cfg := round11TestConfig()
	handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

	t.Run("delete_invalid_params", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodDelete, "/api/v1/statuses//quote/", nil, nil, nil)
		require.NoError(t, err)

		requireStatus(t, http.StatusBadRequest)(handler.HandleDeleteQuotePostLift(ctx))
	})

	t.Run("delete_unauthorized_and_insufficient_scope", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodDelete, "/api/v1/statuses/s1/quote/q1", nil, nil, nil)
		require.NoError(t, err)
		ctx.Params["id"] = "s1"
		ctx.Params["quote_id"] = "q1"

		requireStatus(t, http.StatusUnauthorized)(handler.HandleDeleteQuotePostLift(ctx))

		readToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeRead})
		ctx2, err := round10NewLiftContext(http.MethodDelete, "/api/v1/statuses/s1/quote/q1", map[string]string{"Authorization": "Bearer " + readToken}, nil, nil)
		require.NoError(t, err)
		ctx2.Params["id"] = "s1"
		ctx2.Params["quote_id"] = "q1"

		requireStatus(t, http.StatusForbidden)(handler.HandleDeleteQuotePostLift(ctx2))
	})

	t.Run("delete_missing_quote_id_param", func(t *testing.T) {
		writeToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{"write:statuses"})
		headers := map[string]string{"Authorization": "Bearer " + writeToken}
		ctx, err := round10NewLiftContext(http.MethodDelete, "/api/v1/statuses/s1/quote/", headers, nil, nil)
		require.NoError(t, err)
		ctx.Params["id"] = "s1"

		requireStatus(t, http.StatusBadRequest)(handler.HandleDeleteQuotePostLift(ctx))
	})

	t.Run("delete_not_found_quote", func(t *testing.T) {
		writeToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{"write:statuses"})
		headers := map[string]string{"Authorization": "Bearer " + writeToken}
		ctx, err := round10NewLiftContext(http.MethodDelete, "/api/v1/statuses/s1/quote/q1", headers, nil, nil)
		require.NoError(t, err)
		ctx.Params["id"] = "s1"
		ctx.Params["quote_id"] = "q1"

		requireStatus(t, http.StatusNotFound)(handler.HandleDeleteQuotePostLift(ctx))
	})

	t.Run("delete_forbidden_not_owner_and_ok", func(t *testing.T) {
		writeToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{"write:statuses"})
		headers := map[string]string{"Authorization": "Bearer " + writeToken}

		rel := storagemodels.QuoteRelationship{
			QuoterNoteID: "q1",
			TargetNoteID: "s1",
			QuoterID:     "bob",
			Timestamp:    time.Now().Add(-10 * time.Minute),
		}
		rel.GenerateID()
		require.NoError(t, rel.UpdateKeys())

		state := &round10QueryState{quoteRelationships: []storagemodels.QuoteRelationship{rel}}
		handler, _, _ := round11NewHandler(t, cfg, state)

		ctx, err := round10NewLiftContext(http.MethodDelete, "/api/v1/statuses/s1/quote/q1", headers, nil, nil)
		require.NoError(t, err)
		ctx.Params["id"] = "s1"
		ctx.Params["quote_id"] = "q1"

		requireStatus(t, http.StatusNotFound)(handler.HandleDeleteQuotePostLift(ctx))

		state2 := &round10QueryState{quoteRelationships: []storagemodels.QuoteRelationship{
			func() storagemodels.QuoteRelationship {
				okRel := rel
				okRel.QuoterID = "alice"
				_ = okRel.UpdateKeys()
				return okRel
			}(),
		}}
		handler2, _, _ := round11NewHandler(t, cfg, state2)

		ctx2, err := round10NewLiftContext(http.MethodDelete, "/api/v1/statuses/s1/quote/q1", headers, nil, nil)
		require.NoError(t, err)
		ctx2.Params["id"] = "s1"
		ctx2.Params["quote_id"] = "q1"

		requireStatus(t, http.StatusOK)(handler2.HandleDeleteQuotePostLift(ctx2))
	})

	t.Run("delete_relationship_lookup_error_and_delete_error", func(t *testing.T) {
		writeToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{"write:statuses"})
		headers := map[string]string{"Authorization": "Bearer " + writeToken}

		stateErr := &round10QueryState{
			firstErrorPK: map[string]error{"QUOTE#q1": stdErrors.New("boom")},
		}
		handlerErr, _, _ := round11NewHandler(t, cfg, stateErr)
		ctxErr, err := round10NewLiftContext(http.MethodDelete, "/api/v1/statuses/s1/quote/q1", headers, nil, nil)
		require.NoError(t, err)
		ctxErr.Params["id"] = "s1"
		ctxErr.Params["quote_id"] = "q1"
		requireStatus(t, http.StatusInternalServerError)(handlerErr.HandleDeleteQuotePostLift(ctxErr))

		rel := storagemodels.QuoteRelationship{
			QuoterNoteID: "q1",
			TargetNoteID: "s1",
			QuoterID:     "alice",
			Timestamp:    time.Now().Add(-10 * time.Minute),
		}
		rel.GenerateID()
		require.NoError(t, rel.UpdateKeys())

		stateDel := &round10QueryState{
			deleteErrorOnce:    stdErrors.New("delete failed"),
			quoteRelationships: []storagemodels.QuoteRelationship{rel},
		}
		handlerDel, _, _ := round11NewHandler(t, cfg, stateDel)

		ctxDel, err := round10NewLiftContext(http.MethodDelete, "/api/v1/statuses/s1/quote/q1", headers, nil, nil)
		require.NoError(t, err)
		ctxDel.Params["id"] = "s1"
		ctxDel.Params["quote_id"] = "q1"
		requireStatus(t, http.StatusInternalServerError)(handlerDel.HandleDeleteQuotePostLift(ctxDel))
	})

	t.Run("get_quote_permissions_validation", func(t *testing.T) {
		ctxMissing, err := round10NewLiftContext(http.MethodGet, "/api/v1/accounts//quote_permissions", nil, nil, nil)
		require.NoError(t, err)
		requireStatus(t, http.StatusBadRequest)(handler.HandleGetQuotePermissionsLift(ctxMissing))
	})

	t.Run("update_quote_permissions_auth_validation", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodPut, "/api/v1/accounts/quote_permissions", nil, nil, apimodels.UpdateQuotePermissionsRequest{})
		require.NoError(t, err)
		requireStatus(t, http.StatusUnauthorized)(handler.HandleUpdateQuotePermissionsLift(ctx))

		readToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeRead})
		ctx2, err := round10NewLiftContext(http.MethodPut, "/api/v1/accounts/quote_permissions", map[string]string{"Authorization": "Bearer " + readToken}, nil, apimodels.UpdateQuotePermissionsRequest{})
		require.NoError(t, err)
		requireStatus(t, http.StatusForbidden)(handler.HandleUpdateQuotePermissionsLift(ctx2))

		writeToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{"write:accounts"})
		headers := map[string]string{"Authorization": "Bearer " + writeToken}
		ctxBad := round10NewLiftContextWithBodyBytes(http.MethodPut, "/api/v1/accounts/quote_permissions", headers, nil, []byte(`{invalid}`))
		requireStatus(t, http.StatusBadRequest)(handler.HandleUpdateQuotePermissionsLift(ctxBad))

	})

	t.Run("helper_functions", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodDelete, "/api/v1/statuses/s1/quote/q1", nil, nil, nil)
		require.NoError(t, err)
		require.NoError(t, handler.deleteQuoteRelationship(ctx, &storagemodels.QuoteRelationship{QuoterNoteID: "q1", TargetNoteID: "s1"}))
		require.NoError(t, handler.deleteQuoteRelationship(ctx, nil))
	})
}

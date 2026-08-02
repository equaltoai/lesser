package handlers

import (
	"encoding/json"
	stdErrors "errors"
	"net/http"
	"strings"
	"testing"
	"time"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/auth"
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

	t.Run("auth_header_fallback_paths", func(t *testing.T) {
		writeToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{"write:statuses"})

		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{
			statusByID: map[string]storagemodels.Status{"s1": {StatusID: "s1"}},
		})

		ctxLower, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses/s1/quote", map[string]string{"authorization": "Bearer " + writeToken}, nil, apimodels.CreateQuotePostRequest{Status: "hi"})
		require.NoError(t, err)
		ctxLower.Params["id"] = "s1"
		requireStatus(t, http.StatusNotImplemented)(handler.HandleCreateQuotePostLift(ctxLower))

		ctxDirect, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses/s1/quote", map[string]string{"Authorization": "Bearer " + writeToken}, nil, apimodels.CreateQuotePostRequest{Status: "hi"})
		require.NoError(t, err)
		ctxDirect.Params["id"] = "s1"
		ctxDirect.Request.Headers = nil

		requireStatus(t, http.StatusUnauthorized)(handler.HandleCreateQuotePostLift(ctxDirect))

		ctxDirectLower, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses/s1/quote", map[string]string{"authorization": "Bearer " + writeToken}, nil, apimodels.CreateQuotePostRequest{Status: "hi"})
		require.NoError(t, err)
		ctxDirectLower.Params["id"] = "s1"
		ctxDirectLower.Request.Headers = nil

		requireStatus(t, http.StatusUnauthorized)(handler.HandleCreateQuotePostLift(ctxDirectLower))
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

	t.Run("status_not_found", func(t *testing.T) {
		writeToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{"write:statuses"})
		headers := map[string]string{"Authorization": "Bearer " + writeToken}

		state := &round10QueryState{notFoundPKs: map[string]bool{"status#missing": true}}
		handler, _, _ := round11NewHandler(t, cfg, state)

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses/missing/quote", headers, nil, apimodels.CreateQuotePostRequest{Status: "hi"})
		require.NoError(t, err)
		ctx.Params["id"] = "missing"

		requireStatus(t, http.StatusNotImplemented)(handler.HandleCreateQuotePostLift(ctx))
	})

	t.Run("ok", func(t *testing.T) {
		writeToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{"write:statuses"})
		headers := map[string]string{"Authorization": "Bearer " + writeToken}

		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{
			statusByID: map[string]storagemodels.Status{"s1": {StatusID: "s1", Content: "original"}},
		})
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses/s1/quote", headers, nil, apimodels.CreateQuotePostRequest{Status: "hi"})
		require.NoError(t, err)
		ctx.Params["id"] = "s1"

		requireStatus(t, http.StatusNotImplemented)(handler.HandleCreateQuotePostLift(ctx))
	})
}

func TestCreateQuotePostReturnsNotImplementedWithoutStatusLookup(t *testing.T) {
	cfg := round11TestConfig()
	writeToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{"write:statuses"})
	headers := map[string]string{"Authorization": "Bearer " + writeToken}

	tests := []struct {
		name   string
		id     string
		status *storagemodels.Status
	}{
		{name: "existent", id: "public", status: &storagemodels.Status{StatusID: "public", Visibility: storagemodels.VisibilityPublic}},
		{name: "followers only", id: "followers", status: &storagemodels.Status{StatusID: "followers", Visibility: storagemodels.VisibilityPrivate}},
		{name: "nonexistent", id: "missing"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			state := &round10QueryState{statusByID: map[string]storagemodels.Status{}}
			if tt.status != nil {
				state.statusByID[tt.id] = *tt.status
			}
			handler, _, _ := round11NewHandler(t, cfg, state)
			ctx, err := round10NewLiftContext(
				http.MethodPost,
				"/api/v1/statuses/"+tt.id+"/quote",
				headers,
				nil,
				apimodels.CreateQuotePostRequest{Status: "quote text"},
			)
			require.NoError(t, err)
			ctx.Params["id"] = tt.id

			resp, err := handler.HandleCreateQuotePostLift(ctx)
			require.NoError(t, err)
			require.Equal(t, http.StatusNotImplemented, resp.Status)

			var body map[string]any
			require.NoError(t, json.Unmarshal(resp.Body, &body))
			require.Equal(t, "quote endpoint is not implemented", body["error"])
			require.NotContains(t, body, "id")
			require.NotContains(t, body, "content")
			require.NotContains(t, body, "quoted_status")

			for _, where := range state.wheres {
				require.NotEqual(t, "status#"+tt.id, where.value, "501 path must not look up the target status")
			}
		})
	}
}

func TestGetQuotePermissionsReturnsNotImplementedWithoutStorageLookup(t *testing.T) {
	tests := []struct {
		name string
		id   string
	}{
		{name: "existent account shape", id: "alice"},
		{name: "nonexistent account", id: "definitely-not-a-real-account-9f2b"},
		{name: "path traversal text", id: "../../root"},
		{name: "sql injection text", id: "'; DROP--"},
	}

	var expectedBody []byte
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := &round10QueryState{}
			handler, _, _ := round11NewHandler(t, round11TestConfig(), state)
			ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/accounts/alice/quote_permissions", nil, nil, nil)
			require.NoError(t, err)
			ctx.Params["id"] = tt.id

			resp, err := handler.HandleGetQuotePermissionsLift(ctx)
			require.NoError(t, err)
			require.Equal(t, http.StatusNotImplemented, resp.Status)
			if expectedBody == nil {
				expectedBody = append([]byte(nil), resp.Body...)
			} else {
				require.Equal(t, expectedBody, resp.Body, "all account IDs must receive an identical response")
			}
			require.JSONEq(t, `{"error":"quote permissions endpoint is not implemented"}`, string(resp.Body))
			require.Empty(t, state.wheres, "501 path must not perform a storage query")
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

func TestGetQuotesReturnsNotImplementedWithoutStorageRead(t *testing.T) {
	cfg := round11TestConfig()
	tests := []struct {
		name   string
		id     string
		status *storagemodels.Status
	}{
		{name: "existent", id: "public", status: &storagemodels.Status{StatusID: "public", Visibility: storagemodels.VisibilityPublic}},
		{name: "followers only", id: "followers", status: &storagemodels.Status{StatusID: "followers", Visibility: storagemodels.VisibilityPrivate}},
		{name: "nonexistent", id: "missing"},
	}

	var contractBody []byte
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			state := &round10QueryState{statusByID: map[string]storagemodels.Status{}}
			if tt.status != nil {
				state.statusByID[tt.id] = *tt.status
			}
			handler, _, _ := round11NewHandler(t, cfg, state)
			ctx, err := round10NewLiftContext(
				http.MethodGet,
				"/api/v1/statuses/"+tt.id+"/quotes",
				nil,
				map[string]string{"limit": "10", "offset": "0"},
				nil,
			)
			require.NoError(t, err)
			ctx.Params["id"] = tt.id

			resp := requireStatus(t, http.StatusNotImplemented)(handler.HandleGetQuotesOfStatusLift(ctx))
			var body map[string]any
			require.NoError(t, json.Unmarshal(resp.Body, &body))
			require.Equal(t, "quotes endpoint is not implemented", body["error"])
			require.Empty(t, state.wheres, "501 path must not perform a storage query")

			if contractBody == nil {
				contractBody = append([]byte(nil), resp.Body...)
			} else {
				require.Equal(t, contractBody, resp.Body, "all valid target IDs must have the same 501 body")
			}
		})
	}
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

		requireStatus(t, http.StatusForbidden)(handler.HandleDeleteQuotePostLift(ctx))

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

	t.Run("get_quote_permissions_validation_and_not_implemented", func(t *testing.T) {
		ctxMissing, err := round10NewLiftContext(http.MethodGet, "/api/v1/accounts//quote_permissions", nil, nil, nil)
		require.NoError(t, err)
		requireStatus(t, http.StatusBadRequest)(handler.HandleGetQuotePermissionsLift(ctxMissing))
	})

	t.Run("update_quote_permissions_auth_validation_and_not_implemented", func(t *testing.T) {
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

		allowPublic := true
		allowFollowers := false
		allowMentioned := true
		ctx3, err := round10NewLiftContext(http.MethodPut, "/api/v1/accounts/quote_permissions", headers, nil, apimodels.UpdateQuotePermissionsRequest{
			AllowPublic:    &allowPublic,
			AllowFollowers: &allowFollowers,
			AllowMentioned: &allowMentioned,
			BlockList:      []string{"bob"},
		})
		require.NoError(t, err)
		requireStatus(t, http.StatusNotImplemented)(handler.HandleUpdateQuotePermissionsLift(ctx3))

		ctx4 := round10NewLiftContextWithBodyBytes(http.MethodPut, "/api/v1/accounts/quote_permissions",
			map[string]string{"Authorization": "Bearer " + writeToken, "Content-Type": "application/x-www-form-urlencoded"},
			nil,
			[]byte(`{"allow_followers":true,"allow_mentioned":false}`),
		)
		requireStatus(t, http.StatusNotImplemented)(handler.HandleUpdateQuotePermissionsLift(ctx4))
	})

	t.Run("helper_functions", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodDelete, "/api/v1/statuses/s1/quote/q1", nil, nil, nil)
		require.NoError(t, err)
		require.NoError(t, handler.deleteQuoteRelationship(ctx, &storagemodels.QuoteRelationship{QuoterNoteID: "q1", TargetNoteID: "s1"}))
		require.NoError(t, handler.deleteQuoteRelationship(ctx, nil))
	})
}

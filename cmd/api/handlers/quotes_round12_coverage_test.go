package lift

import (
	stdErrors "errors"
	"net/http"
	"reflect"
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

		require.NoError(t, handler.HandleCreateQuotePostLift(ctx))
		require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)
	})

	t.Run("unauthorized_missing_token", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{
			statusByID: map[string]storagemodels.Status{"s1": {StatusID: "s1"}},
		})
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses/s1/quote", nil, nil, apimodels.CreateQuotePostRequest{Status: "hi"})
		require.NoError(t, err)
		ctx.SetParam("id", "s1")

		require.NoError(t, handler.HandleCreateQuotePostLift(ctx))
		require.Equal(t, http.StatusUnauthorized, ctx.Response.StatusCode)
	})

	t.Run("unauthorized_invalid_token", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{
			statusByID: map[string]storagemodels.Status{"s1": {StatusID: "s1"}},
		})
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses/s1/quote", map[string]string{"Authorization": "Bearer bad"}, nil, apimodels.CreateQuotePostRequest{Status: "hi"})
		require.NoError(t, err)
		ctx.SetParam("id", "s1")

		require.NoError(t, handler.HandleCreateQuotePostLift(ctx))
		require.Equal(t, http.StatusUnauthorized, ctx.Response.StatusCode)
	})

	t.Run("insufficient_scope", func(t *testing.T) {
		readToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeRead})
		headers := map[string]string{"Authorization": "Bearer " + readToken}

		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{
			statusByID: map[string]storagemodels.Status{"s1": {StatusID: "s1"}},
		})
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses/s1/quote", headers, nil, apimodels.CreateQuotePostRequest{Status: "hi"})
		require.NoError(t, err)
		ctx.SetParam("id", "s1")

		require.NoError(t, handler.HandleCreateQuotePostLift(ctx))
		require.Equal(t, http.StatusForbidden, ctx.Response.StatusCode)
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
		ctx.SetParam("id", "s1")

		require.NoError(t, handler.HandleCreateQuotePostLift(ctx))
		require.NotEqual(t, http.StatusOK, ctx.Response.StatusCode)
	})

	t.Run("invalid_json_body", func(t *testing.T) {
		writeToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{"write:statuses"})
		headers := map[string]string{"Authorization": "Bearer " + writeToken, "Content-Type": "application/json"}

		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{
			statusByID: map[string]storagemodels.Status{"s1": {StatusID: "s1"}},
		})

		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v1/statuses/s1/quote", headers, nil, []byte(`{invalid}`))
		ctx.SetParam("id", "s1")

		require.NoError(t, handler.HandleCreateQuotePostLift(ctx))
		require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)
	})

	t.Run("auth_header_fallback_paths", func(t *testing.T) {
		writeToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{"write:statuses"})

		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{
			statusByID: map[string]storagemodels.Status{"s1": {StatusID: "s1"}},
		})

		ctxLower, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses/s1/quote", map[string]string{"authorization": "Bearer " + writeToken}, nil, apimodels.CreateQuotePostRequest{Status: "hi"})
		require.NoError(t, err)
		ctxLower.SetParam("id", "s1")
		require.NoError(t, handler.HandleCreateQuotePostLift(ctxLower))
		require.Equal(t, http.StatusOK, ctxLower.Response.StatusCode)

		ctxDirect, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses/s1/quote", map[string]string{"Authorization": "Bearer " + writeToken}, nil, apimodels.CreateQuotePostRequest{Status: "hi"})
		require.NoError(t, err)
		ctxDirect.SetParam("id", "s1")
		ctxDirect.Request.Headers = nil

		require.NoError(t, handler.HandleCreateQuotePostLift(ctxDirect))
		require.Equal(t, http.StatusOK, ctxDirect.Response.StatusCode)

		ctxDirectLower, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses/s1/quote", map[string]string{"authorization": "Bearer " + writeToken}, nil, apimodels.CreateQuotePostRequest{Status: "hi"})
		require.NoError(t, err)
		ctxDirectLower.SetParam("id", "s1")
		ctxDirectLower.Request.Headers = nil

		require.NoError(t, handler.HandleCreateQuotePostLift(ctxDirectLower))
		require.Equal(t, http.StatusOK, ctxDirectLower.Response.StatusCode)
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
		ctx.SetParam("id", "s1")
		ctx.Request.Body = nil

		require.NoError(t, handler.HandleCreateQuotePostLift(ctx))
		require.Equal(t, http.StatusOK, ctx.Response.StatusCode)
	})

	t.Run("status_not_found", func(t *testing.T) {
		writeToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{"write:statuses"})
		headers := map[string]string{"Authorization": "Bearer " + writeToken}

		state := &round10QueryState{notFoundPKs: map[string]bool{"status#missing": true}}
		handler, _, _ := round11NewHandler(t, cfg, state)

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses/missing/quote", headers, nil, apimodels.CreateQuotePostRequest{Status: "hi"})
		require.NoError(t, err)
		ctx.SetParam("id", "missing")

		require.NoError(t, handler.HandleCreateQuotePostLift(ctx))
		require.Equal(t, http.StatusNotFound, ctx.Response.StatusCode)
	})

	t.Run("ok", func(t *testing.T) {
		writeToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{"write:statuses"})
		headers := map[string]string{"Authorization": "Bearer " + writeToken}

		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{
			statusByID: map[string]storagemodels.Status{"s1": {StatusID: "s1", Content: "original"}},
		})
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses/s1/quote", headers, nil, apimodels.CreateQuotePostRequest{Status: "hi"})
		require.NoError(t, err)
		ctx.SetParam("id", "s1")

		require.NoError(t, handler.HandleCreateQuotePostLift(ctx))
		require.Equal(t, http.StatusOK, ctx.Response.StatusCode)
	})
}

func TestQuotes_Round12_GetQuotesOfStatus_Coverage(t *testing.T) {
	cfg := round11TestConfig()
	handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

	t.Run("invalid_status_id", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/statuses//quotes", nil, nil, nil)
		require.NoError(t, err)

		require.NoError(t, handler.HandleGetQuotesOfStatusLift(ctx))
		require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)
	})

	t.Run("invalid_limit", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/statuses/s1/quotes", nil, map[string]string{"limit": "bad"}, nil)
		require.NoError(t, err)
		ctx.SetParam("id", "s1")

		require.NoError(t, handler.HandleGetQuotesOfStatusLift(ctx))
		require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)
	})

	t.Run("invalid_offset", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/statuses/s1/quotes", nil, map[string]string{"offset": "bad"}, nil)
		require.NoError(t, err)
		ctx.SetParam("id", "s1")

		require.NoError(t, handler.HandleGetQuotesOfStatusLift(ctx))
		require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)
	})

	t.Run("ok_empty", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/statuses/s1/quotes", nil, map[string]string{"limit": "10", "offset": "0"}, nil)
		require.NoError(t, err)
		ctx.SetParam("id", "s1")

		require.NoError(t, handler.HandleGetQuotesOfStatusLift(ctx))
		require.Equal(t, http.StatusOK, ctx.Response.StatusCode)
	})

	t.Run("ok_non_empty", func(t *testing.T) {
		rel := storagemodels.QuoteRelationship{
			QuoterNoteID: "q1",
			TargetNoteID: "s1",
			QuoterID:     "alice",
			Timestamp:    time.Now().Add(-10 * time.Minute),
		}
		rel.GenerateID()
		require.NoError(t, rel.UpdateKeys())

		state := &round10QueryState{quoteRelationships: []storagemodels.QuoteRelationship{rel}}
		handler, _, _ := round11NewHandler(t, cfg, state)

		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/statuses/s1/quotes", nil, map[string]string{"limit": "10", "offset": "0"}, nil)
		require.NoError(t, err)
		ctx.SetParam("id", "s1")

		require.NoError(t, handler.HandleGetQuotesOfStatusLift(ctx))
		require.Equal(t, http.StatusOK, ctx.Response.StatusCode)
		summaries, ok := ctx.Response.Body.([]apimodels.QuoteStatusSummary)
		require.True(t, ok)
		require.Len(t, summaries, 1)
	})

	t.Run("service_error", func(t *testing.T) {
		allErrType := reflect.TypeOf(&[]storagemodels.QuoteRelationship{}).String()
		state := &round10QueryState{allErrorByType: map[string]error{allErrType: stdErrors.New("query failed")}}
		handler, _, _ := round11NewHandler(t, cfg, state)

		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/statuses/s1/quotes", nil, map[string]string{"limit": "10", "offset": "0"}, nil)
		require.NoError(t, err)
		ctx.SetParam("id", "s1")

		require.NoError(t, handler.HandleGetQuotesOfStatusLift(ctx))
		require.Equal(t, http.StatusInternalServerError, ctx.Response.StatusCode)
	})
}

func TestQuotes_Round12_DeleteAndPermissions_Coverage(t *testing.T) {
	cfg := round11TestConfig()
	handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

	t.Run("delete_invalid_params", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodDelete, "/api/v1/statuses//quote/", nil, nil, nil)
		require.NoError(t, err)

		require.NoError(t, handler.HandleDeleteQuotePostLift(ctx))
		require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)
	})

	t.Run("delete_unauthorized_and_insufficient_scope", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodDelete, "/api/v1/statuses/s1/quote/q1", nil, nil, nil)
		require.NoError(t, err)
		ctx.SetParam("id", "s1")
		ctx.SetParam("quote_id", "q1")

		require.NoError(t, handler.HandleDeleteQuotePostLift(ctx))
		require.Equal(t, http.StatusUnauthorized, ctx.Response.StatusCode)

		readToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeRead})
		ctx2, err := round10NewLiftContext(http.MethodDelete, "/api/v1/statuses/s1/quote/q1", map[string]string{"Authorization": "Bearer " + readToken}, nil, nil)
		require.NoError(t, err)
		ctx2.SetParam("id", "s1")
		ctx2.SetParam("quote_id", "q1")

		require.NoError(t, handler.HandleDeleteQuotePostLift(ctx2))
		require.Equal(t, http.StatusForbidden, ctx2.Response.StatusCode)
	})

	t.Run("delete_missing_quote_id_param", func(t *testing.T) {
		writeToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{"write:statuses"})
		headers := map[string]string{"Authorization": "Bearer " + writeToken}
		ctx, err := round10NewLiftContext(http.MethodDelete, "/api/v1/statuses/s1/quote/", headers, nil, nil)
		require.NoError(t, err)
		ctx.SetParam("id", "s1")

		require.NoError(t, handler.HandleDeleteQuotePostLift(ctx))
		require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)
	})

	t.Run("delete_not_found_quote", func(t *testing.T) {
		writeToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{"write:statuses"})
		headers := map[string]string{"Authorization": "Bearer " + writeToken}
		ctx, err := round10NewLiftContext(http.MethodDelete, "/api/v1/statuses/s1/quote/q1", headers, nil, nil)
		require.NoError(t, err)
		ctx.SetParam("id", "s1")
		ctx.SetParam("quote_id", "q1")

		require.NoError(t, handler.HandleDeleteQuotePostLift(ctx))
		require.Equal(t, http.StatusNotFound, ctx.Response.StatusCode)
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
		ctx.SetParam("id", "s1")
		ctx.SetParam("quote_id", "q1")

		require.NoError(t, handler.HandleDeleteQuotePostLift(ctx))
		require.Equal(t, http.StatusForbidden, ctx.Response.StatusCode)

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
		ctx2.SetParam("id", "s1")
		ctx2.SetParam("quote_id", "q1")

		require.NoError(t, handler2.HandleDeleteQuotePostLift(ctx2))
		require.Equal(t, http.StatusOK, ctx2.Response.StatusCode)
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
		ctxErr.SetParam("id", "s1")
		ctxErr.SetParam("quote_id", "q1")
		require.NoError(t, handlerErr.HandleDeleteQuotePostLift(ctxErr))
		require.Equal(t, http.StatusInternalServerError, ctxErr.Response.StatusCode)

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
		ctxDel.SetParam("id", "s1")
		ctxDel.SetParam("quote_id", "q1")
		require.NoError(t, handlerDel.HandleDeleteQuotePostLift(ctxDel))
		require.Equal(t, http.StatusInternalServerError, ctxDel.Response.StatusCode)
	})

	t.Run("get_quote_permissions_validation_and_ok", func(t *testing.T) {
		ctxMissing, err := round10NewLiftContext(http.MethodGet, "/api/v1/accounts//quote_permissions", nil, nil, nil)
		require.NoError(t, err)
		require.NoError(t, handler.HandleGetQuotePermissionsLift(ctxMissing))
		require.Equal(t, http.StatusBadRequest, ctxMissing.Response.StatusCode)

		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/accounts/alice/quote_permissions", nil, nil, nil)
		require.NoError(t, err)
		ctx.SetParam("id", "alice")

		require.NoError(t, handler.HandleGetQuotePermissionsLift(ctx))
		require.Equal(t, http.StatusOK, ctx.Response.StatusCode)
	})

	t.Run("update_quote_permissions_unauthorized_scope_and_ok", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodPut, "/api/v1/accounts/quote_permissions", nil, nil, apimodels.UpdateQuotePermissionsRequest{})
		require.NoError(t, err)
		require.NoError(t, handler.HandleUpdateQuotePermissionsLift(ctx))
		require.Equal(t, http.StatusUnauthorized, ctx.Response.StatusCode)

		readToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeRead})
		ctx2, err := round10NewLiftContext(http.MethodPut, "/api/v1/accounts/quote_permissions", map[string]string{"Authorization": "Bearer " + readToken}, nil, apimodels.UpdateQuotePermissionsRequest{})
		require.NoError(t, err)
		require.NoError(t, handler.HandleUpdateQuotePermissionsLift(ctx2))
		require.Equal(t, http.StatusForbidden, ctx2.Response.StatusCode)

		writeToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{"write:accounts"})
		headers := map[string]string{"Authorization": "Bearer " + writeToken}
		ctxBad := round10NewLiftContextWithBodyBytes(http.MethodPut, "/api/v1/accounts/quote_permissions", headers, nil, []byte(`{invalid}`))
		require.NoError(t, handler.HandleUpdateQuotePermissionsLift(ctxBad))
		require.NotEqual(t, http.StatusOK, ctxBad.Response.StatusCode)

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
		require.NoError(t, handler.HandleUpdateQuotePermissionsLift(ctx3))
		require.Equal(t, http.StatusOK, ctx3.Response.StatusCode)

		ctx4 := round10NewLiftContextWithBodyBytes(http.MethodPut, "/api/v1/accounts/quote_permissions",
			map[string]string{"Authorization": "Bearer " + writeToken, "Content-Type": "application/x-www-form-urlencoded"},
			nil,
			[]byte(`{"allow_followers":true,"allow_mentioned":false}`),
		)
		require.NoError(t, handler.HandleUpdateQuotePermissionsLift(ctx4))
		require.Equal(t, http.StatusOK, ctx4.Response.StatusCode)
	})

	t.Run("helper_functions", func(t *testing.T) {
		_ = handler.convertQuoteToAPI(nil, map[string]any{"id": "q"})
		ctx, err := round10NewLiftContext(http.MethodDelete, "/api/v1/statuses/s1/quote/q1", nil, nil, nil)
		require.NoError(t, err)
		require.NoError(t, handler.deleteQuoteRelationship(ctx, &storagemodels.QuoteRelationship{QuoterNoteID: "q1", TargetNoteID: "s1"}))
		require.NoError(t, handler.deleteQuoteRelationship(ctx, nil))
		_, err = handler.getQuotesForStatus(ctx, "s1", 0, 0)
		require.NoError(t, err)
	})
}

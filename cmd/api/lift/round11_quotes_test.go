package lift

import (
	"net/http"
	"testing"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestQuotesHandlers(t *testing.T) {
	cfg := round11TestConfig()
	state := &round10QueryState{
		statusByID: map[string]storagemodels.Status{
			"123": {StatusID: "123", AuthorUsername: "bob"},
		},
	}
	handler, _, _ := round11NewHandler(t, cfg, state)

	token := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{"write:statuses", "read"})
	headers := map[string]string{"Authorization": "Bearer " + token}

	ctxCreate, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses/123/quote", headers, nil, apimodels.CreateQuotePostRequest{Status: "Quote text"})
	require.NoError(t, err)
	ctxCreate.SetParam("id", "123")
	require.NoError(t, handler.HandleCreateQuotePostLift(ctxCreate))
	require.Equal(t, http.StatusOK, ctxCreate.Response.StatusCode)

	ctxList, err := round10NewLiftContext(http.MethodGet, "/api/v1/statuses/123/quotes", nil, map[string]string{"limit": "2", "offset": "0"}, nil)
	require.NoError(t, err)
	ctxList.SetParam("id", "123")
	require.NoError(t, handler.HandleGetQuotesOfStatusLift(ctxList))

	ctxDelete, err := round10NewLiftContext(http.MethodDelete, "/api/v1/statuses/123/quote/q1", headers, nil, nil)
	require.NoError(t, err)
	ctxDelete.SetParam("id", "123")
	ctxDelete.SetParam("quote_id", "q1")
	require.NoError(t, handler.HandleDeleteQuotePostLift(ctxDelete))
	require.Equal(t, http.StatusNotFound, ctxDelete.Response.StatusCode)
}

func TestQuotesPermissions(t *testing.T) {
	cfg := round11TestConfig()
	handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

	ctxGet, err := round10NewLiftContext(http.MethodGet, "/api/v1/accounts/alice/quote_permissions", nil, nil, nil)
	require.NoError(t, err)
	ctxGet.SetParam("id", "alice")
	require.NoError(t, handler.HandleGetQuotePermissionsLift(ctxGet))

	updateToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{"write:accounts"})
	headers := map[string]string{"Authorization": "Bearer " + updateToken}
	updateReq := apimodels.UpdateQuotePermissionsRequest{
		AllowPublic:    boolPtr(true),
		AllowFollowers: boolPtr(false),
		AllowMentioned: boolPtr(true),
		BlockList:      []string{"bob"},
	}
	ctxUpdate, err := round10NewLiftContext(http.MethodPut, "/api/v1/accounts/quote_permissions", headers, nil, updateReq)
	require.NoError(t, err)
	require.NoError(t, handler.HandleUpdateQuotePermissionsLift(ctxUpdate))
}

func boolPtr(v bool) *bool {
	return &v
}

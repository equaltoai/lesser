package lift

import (
	"context"
	"net/http"
	"testing"
	"time"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/storage"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestTags_HandleGetTagLift_FallbackAndAuth(t *testing.T) {
	cfg := round11TestConfig()
	state := &round10QueryState{
		notFoundPKs: map[string]bool{
			"HASHTAG#missing": true,
		},
	}

	handler, _, _ := round11NewHandler(t, cfg, state)

	token := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeRead})
	headers := map[string]string{"Authorization": "Bearer " + token}

	ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/tags/missing", headers, nil, nil)
	require.NoError(t, err)
	ctx.SetParam("id", "missing")
	ctx.Request.Request.Headers = headers

	err = handler.HandleGetTagLift(ctx)
	require.NoError(t, err)

	resp := ctx.Response.Body.(apimodels.Tag)
	require.Equal(t, "missing", resp.Name)
	require.NotNil(t, resp.Following)
	require.True(t, *resp.Following)
}

func TestTags_FollowUnfollowAndFollowedList(t *testing.T) {
	cfg := round11TestConfig()
	state := &round10QueryState{
		hashtagFollowsByUser: map[string][]storagemodels.HashtagFollow{
			"alice": {
				{PK: "user#alice", SK: "hashtag#go", UserID: "alice", Hashtag: "go", CreatedAt: time.Now()},
				{PK: "user#alice", SK: "hashtag#ai", UserID: "alice", Hashtag: "ai", CreatedAt: time.Now()},
			},
		},
	}

	handler, _, _ := round11NewHandler(t, cfg, state)
	token := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeRead, auth.ScopeWrite})
	headers := map[string]string{"Authorization": "Bearer " + token}

	ctxFollow, err := round10NewLiftContext(http.MethodPost, "/api/v1/tags/%23Go/follow", headers, nil, nil)
	require.NoError(t, err)
	ctxFollow.SetParam("id", "#Go")
	require.NoError(t, handler.HandleFollowTagLift(ctxFollow))
	require.Equal(t, http.StatusOK, ctxFollow.Response.StatusCode)
	respFollow := ctxFollow.Response.Body.(map[string]any)
	require.Equal(t, true, respFollow["following"])

	ctxUnfollow, err := round10NewLiftContext(http.MethodPost, "/api/v1/tags/%23Go/unfollow", headers, nil, nil)
	require.NoError(t, err)
	ctxUnfollow.SetParam("id", "#Go")
	require.NoError(t, handler.HandleUnfollowTagLift(ctxUnfollow))
	require.Equal(t, http.StatusOK, ctxUnfollow.Response.StatusCode)
	respUnfollow := ctxUnfollow.Response.Body.(map[string]any)
	require.Equal(t, false, respUnfollow["following"])

	ctxList, err := round10NewLiftContext(http.MethodGet, "/api/v1/followed_tags", headers, map[string]string{"limit": "1", "max_id": "cursor-1"}, nil)
	require.NoError(t, err)
	require.NoError(t, handler.HandleGetFollowedTagsLift(ctxList))
	body := ctxList.Response.Body.([]map[string]any)
	require.Len(t, body, 1)
	require.Contains(t, ctxList.Response.Headers["Link"], "max_id=")
}

func TestTags_Helpers(t *testing.T) {
	cfg := round11TestConfig()
	handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

	headers := map[string]string{"Authorization": "Bearer token"}
	ctx, err := round10NewLiftContext(http.MethodGet, "/tags", headers, map[string]string{"limit": "bad", "max_id": "cursor"}, nil)
	require.NoError(t, err)
	ctx.Request.Request.Headers = headers

	authHeader := handler.getAuthorizationHeader(ctx)
	require.Equal(t, "Bearer token", authHeader)

	params := handler.extractPaginationParams(ctx)
	require.Equal(t, 100, params.limit)
	require.Equal(t, "cursor", params.cursor)

	history := handler.getHashtagHistory(context.Background(), "go")
	require.Len(t, history, 7)

	tags := handler.buildTagModels(context.Background(), []*storage.HashtagFollow{
		&storage.HashtagFollow{Hashtag: "go"},
		nil,
		&storage.HashtagFollow{Hashtag: ""},
	})
	require.Len(t, tags, 1)
}

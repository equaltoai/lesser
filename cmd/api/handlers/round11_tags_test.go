package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestTags_HandleGetTagLift_FallbackAndAuth(t *testing.T) {
	cfg := round11TestConfig()
	state := &round10QueryState{
		hashtagFollowsByUser: map[string][]storagemodels.HashtagFollow{
			"alice": {
				{PK: "user#alice", SK: "hashtag#missing", UserID: "alice", Hashtag: "missing", CreatedAt: time.Now()},
			},
		},
		notFoundPKs: map[string]bool{
			"HASHTAG#missing": true,
		},
	}

	handler, _, _ := round11NewHandler(t, cfg, state)

	token := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeRead})
	headers := map[string]string{"Authorization": "Bearer " + token}

	ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/tags/missing", headers, nil, nil)
	require.NoError(t, err)
	ctx.Params["id"] = "missing"

	resp := requireStatus(t, http.StatusOK)(handler.HandleGetTagLift(ctx))

	var tag apimodels.Tag
	require.NoError(t, json.Unmarshal(resp.Body, &tag))
	require.Equal(t, "missing", tag.Name)
	require.NotNil(t, tag.Following)
	require.True(t, *tag.Following)
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
	ctxFollow.Params["id"] = "#Go"
	respFollow := requireStatus(t, http.StatusOK)(handler.HandleFollowTagLift(ctxFollow))
	var followBody map[string]any
	require.NoError(t, json.Unmarshal(respFollow.Body, &followBody))
	require.Equal(t, true, followBody["following"])

	ctxUnfollow, err := round10NewLiftContext(http.MethodPost, "/api/v1/tags/%23Go/unfollow", headers, nil, nil)
	require.NoError(t, err)
	ctxUnfollow.Params["id"] = "#Go"
	respUnfollow := requireStatus(t, http.StatusOK)(handler.HandleUnfollowTagLift(ctxUnfollow))
	var unfollowBody map[string]any
	require.NoError(t, json.Unmarshal(respUnfollow.Body, &unfollowBody))
	require.Equal(t, false, unfollowBody["following"])

	ctxList, err := round10NewLiftContext(http.MethodGet, "/api/v1/followed_tags", headers, map[string]string{"limit": "1", "max_id": "cursor-1"}, nil)
	require.NoError(t, err)
	respList := requireStatus(t, http.StatusOK)(handler.HandleGetFollowedTagsLift(ctxList))
	var body []map[string]any
	require.NoError(t, json.Unmarshal(respList.Body, &body))
	require.Len(t, body, 1)
	require.Contains(t, firstStringValue(respList.Headers, "link"), "max_id=")
}

func TestTags_Helpers(t *testing.T) {
	cfg := round11TestConfig()
	handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

	headers := map[string]string{"Authorization": "Bearer token"}
	ctx, err := round10NewLiftContext(http.MethodGet, "/tags", headers, map[string]string{"limit": "bad", "max_id": "cursor"}, nil)
	require.NoError(t, err)

	authHeader := common.ExtractAuthHeader(ctx)
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

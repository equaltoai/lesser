package handlers

import (
	"encoding/json"
	stdErrors "errors"
	"net/http"
	"testing"

	"github.com/equaltoai/lesser/pkg/auth"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestTags_Round12_ErrorPathsAndHeaderFallbacks(t *testing.T) {
	cfg := round11TestConfig()

	t.Run("getAuthorizationHeader supports lowercase and direct request headers", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

		t.Run("lowercase header key", func(t *testing.T) {
			ctx, err := round10NewLiftContext(http.MethodGet, "/tags", map[string]string{"authorization": "Bearer token"}, nil, nil)
			require.NoError(t, err)
			require.Equal(t, "Bearer token", h.getAuthorizationHeader(ctx))
		})

		t.Run("canonicalized header key", func(t *testing.T) {
			ctx, err := round10NewLiftContext(http.MethodGet, "/tags", nil, nil, nil)
			require.NoError(t, err)
			ctx.Request.Headers = map[string][]string{"authorization": {"Bearer token"}}
			require.Equal(t, "Bearer token", h.getAuthorizationHeader(ctx))
		})
	})

	t.Run("HandleGetTagLift validates params and tolerates invalid auth header", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

		t.Run("missing tag id returns 400", func(t *testing.T) {
			ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/tags/", nil, nil, nil)
			require.NoError(t, err)
			requireStatus(t, http.StatusBadRequest)(h.HandleGetTagLift(ctx))
		})

		t.Run("invalid hashtag returns 400", func(t *testing.T) {
			ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/tags/bad!", nil, nil, nil)
			require.NoError(t, err)
			ctx.Params["id"] = "bad!"
			requireStatus(t, http.StatusBadRequest)(h.HandleGetTagLift(ctx))
		})

		t.Run("invalid auth header does not set following", func(t *testing.T) {
			ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/tags/go", map[string]string{"Authorization": "nope"}, nil, nil)
			require.NoError(t, err)
			ctx.Params["id"] = "go"
			requireStatus(t, http.StatusOK)(h.HandleGetTagLift(ctx))
		})
	})

	t.Run("HandleFollowTagLift/HandleUnfollowTagLift cover validation, auth, and repo error", func(t *testing.T) {
		token := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeRead})
		headers := map[string]string{"Authorization": "Bearer " + token}

		t.Run("missing id returns 400", func(t *testing.T) {
			h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
			ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/tags//follow", headers, nil, nil)
			require.NoError(t, err)
			requireStatus(t, http.StatusBadRequest)(h.HandleFollowTagLift(ctx))
		})

		t.Run("invalid hashtag returns 400", func(t *testing.T) {
			h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
			ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/tags/bad!/follow", headers, nil, nil)
			require.NoError(t, err)
			ctx.Params["id"] = "bad!"
			requireStatus(t, http.StatusBadRequest)(h.HandleFollowTagLift(ctx))
		})

		t.Run("invalid token returns 401", func(t *testing.T) {
			h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
			ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/tags/go/follow", map[string]string{"Authorization": "Bearer invalid"}, nil, nil)
			require.NoError(t, err)
			ctx.Params["id"] = "go"
			requireStatus(t, http.StatusUnauthorized)(h.HandleFollowTagLift(ctx))
		})

		t.Run("repo error returns 500", func(t *testing.T) {
			h, _, _ := round11NewHandler(t, cfg, &round10QueryState{createErrorOnce: stdErrors.New("boom")})
			ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/tags/go/follow", headers, nil, nil)
			require.NoError(t, err)
			ctx.Params["id"] = "go"
			requireStatus(t, http.StatusInternalServerError)(h.HandleFollowTagLift(ctx))
		})
	})

	t.Run("HandleGetFollowedTagsLift unauthorized and repo error", func(t *testing.T) {
		t.Run("missing token returns 401", func(t *testing.T) {
			h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
			ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/followed_tags", nil, nil, nil)
			require.NoError(t, err)
			requireStatus(t, http.StatusUnauthorized)(h.HandleGetFollowedTagsLift(ctx))
		})

		t.Run("query error returns 500", func(t *testing.T) {
			state := &round10QueryState{
				allErrorByType: map[string]error{
					"*[]*models.HashtagFollow": stdErrors.New("boom"),
				},
			}
			h, _, _ := round11NewHandler(t, cfg, state)
			token := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeRead})
			ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/followed_tags", map[string]string{"Authorization": "Bearer " + token}, nil, nil)
			require.NoError(t, err)
			requireStatus(t, http.StatusInternalServerError)(h.HandleGetFollowedTagsLift(ctx))
		})
	})

	t.Run("HandleGetFeaturedTagsLift/HandleGetFeaturedTagSuggestionsLift unauthorized", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/featured_tags", nil, nil, nil)
		require.NoError(t, err)
		requireStatus(t, http.StatusUnauthorized)(h.HandleGetFeaturedTagsLift(ctx))

		ctxSug, err := round10NewLiftContext(http.MethodGet, "/api/v1/featured_tags/suggestions", nil, nil, nil)
		require.NoError(t, err)
		requireStatus(t, http.StatusUnauthorized)(h.HandleGetFeaturedTagSuggestionsLift(ctxSug))
	})

	t.Run("HandleDeleteFeaturedTagLift validates params and auth", func(t *testing.T) {
		token := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeRead})
		headers := map[string]string{"Authorization": "Bearer " + token}

		t.Run("missing id returns 400", func(t *testing.T) {
			h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
			ctx, err := round10NewLiftContext(http.MethodDelete, "/api/v1/featured_tags/", headers, nil, nil)
			require.NoError(t, err)
			requireStatus(t, http.StatusBadRequest)(h.HandleDeleteFeaturedTagLift(ctx))
		})

		t.Run("invalid id returns 400", func(t *testing.T) {
			h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
			ctx, err := round10NewLiftContext(http.MethodDelete, "/api/v1/featured_tags/bad!", headers, nil, nil)
			require.NoError(t, err)
			ctx.Params["id"] = "bad!"
			requireStatus(t, http.StatusBadRequest)(h.HandleDeleteFeaturedTagLift(ctx))
		})

		t.Run("missing token returns 401", func(t *testing.T) {
			h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
			ctx, err := round10NewLiftContext(http.MethodDelete, "/api/v1/featured_tags/go", nil, nil, nil)
			require.NoError(t, err)
			ctx.Params["id"] = "go"
			requireStatus(t, http.StatusUnauthorized)(h.HandleDeleteFeaturedTagLift(ctx))
		})
	})

	t.Run("HandleGetAccountFeaturedTagsLift validates id and returns empty list", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/accounts//featured_tags", nil, nil, nil)
		require.NoError(t, err)
		requireStatus(t, http.StatusBadRequest)(h.HandleGetAccountFeaturedTagsLift(ctx))

		state := &round10QueryState{
			featuredTagsByUser: map[string][]storagemodels.FeaturedTag{
				"bob": {},
			},
		}
		hEmpty, _, _ := round11NewHandler(t, cfg, state)
		ctx2, err := round10NewLiftContext(http.MethodGet, "/api/v1/accounts/bob/featured_tags", nil, nil, nil)
		require.NoError(t, err)
		ctx2.Params["id"] = "bob"
		resp2 := requireStatus(t, http.StatusOK)(hEmpty.HandleGetAccountFeaturedTagsLift(ctx2))
		var out []FeaturedTag
		require.NoError(t, json.Unmarshal(resp2.Body, &out))
		require.Len(t, out, 0)
	})
}

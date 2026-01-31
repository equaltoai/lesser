package lift

import (
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

		t.Run("direct access when ctx.Header is unavailable", func(t *testing.T) {
			ctx, err := round10NewLiftContext(http.MethodGet, "/tags", nil, nil, nil)
			require.NoError(t, err)
			// Force ctx.Header() to return empty by clearing Request.Headers,
			// while keeping the adapter request headers populated.
			ctx.Request.Headers = nil
			ctx.Request.Request.Headers = map[string]string{"authorization": "Bearer token"}
			require.Equal(t, "Bearer token", h.getAuthorizationHeader(ctx))
		})
	})

	t.Run("HandleGetTagLift validates params and tolerates invalid auth header", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

		t.Run("missing tag id returns 400", func(t *testing.T) {
			ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/tags/", nil, nil, nil)
			require.NoError(t, err)
			require.NoError(t, h.HandleGetTagLift(ctx))
			require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)
		})

		t.Run("invalid hashtag returns 400", func(t *testing.T) {
			ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/tags/bad!", nil, nil, nil)
			require.NoError(t, err)
			ctx.SetParam("id", "bad!")
			require.NoError(t, h.HandleGetTagLift(ctx))
			require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)
		})

		t.Run("invalid auth header does not set following", func(t *testing.T) {
			ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/tags/go", map[string]string{"Authorization": "nope"}, nil, nil)
			require.NoError(t, err)
			ctx.SetParam("id", "go")
			require.NoError(t, h.HandleGetTagLift(ctx))
			require.Equal(t, http.StatusOK, ctx.Response.StatusCode)
		})
	})

	t.Run("HandleFollowTagLift/HandleUnfollowTagLift cover validation, auth, and repo error", func(t *testing.T) {
		token := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeRead})
		headers := map[string]string{"Authorization": "Bearer " + token}

		t.Run("missing id returns 400", func(t *testing.T) {
			h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
			ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/tags//follow", headers, nil, nil)
			require.NoError(t, err)
			require.NoError(t, h.HandleFollowTagLift(ctx))
			require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)
		})

		t.Run("invalid hashtag returns 400", func(t *testing.T) {
			h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
			ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/tags/bad!/follow", headers, nil, nil)
			require.NoError(t, err)
			ctx.SetParam("id", "bad!")
			require.NoError(t, h.HandleFollowTagLift(ctx))
			require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)
		})

		t.Run("invalid token returns 401", func(t *testing.T) {
			h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
			ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/tags/go/follow", map[string]string{"Authorization": "Bearer invalid"}, nil, nil)
			require.NoError(t, err)
			ctx.SetParam("id", "go")
			require.NoError(t, h.HandleFollowTagLift(ctx))
			require.Equal(t, http.StatusUnauthorized, ctx.Response.StatusCode)
		})

		t.Run("repo error returns 500", func(t *testing.T) {
			h, _, _ := round11NewHandler(t, cfg, &round10QueryState{createErrorOnce: stdErrors.New("boom")})
			ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/tags/go/follow", headers, nil, nil)
			require.NoError(t, err)
			ctx.SetParam("id", "go")
			require.NoError(t, h.HandleFollowTagLift(ctx))
			require.Equal(t, http.StatusInternalServerError, ctx.Response.StatusCode)
		})
	})

	t.Run("HandleGetFollowedTagsLift unauthorized and repo error", func(t *testing.T) {
		t.Run("missing token returns 401", func(t *testing.T) {
			h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
			ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/followed_tags", nil, nil, nil)
			require.NoError(t, err)
			require.NoError(t, h.HandleGetFollowedTagsLift(ctx))
			require.Equal(t, http.StatusUnauthorized, ctx.Response.StatusCode)
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
			require.NoError(t, h.HandleGetFollowedTagsLift(ctx))
			require.Equal(t, http.StatusInternalServerError, ctx.Response.StatusCode)
		})
	})

	t.Run("HandleGetFeaturedTagsLift/HandleGetFeaturedTagSuggestionsLift unauthorized", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/featured_tags", nil, nil, nil)
		require.NoError(t, err)
		require.NoError(t, h.HandleGetFeaturedTagsLift(ctx))
		require.Equal(t, http.StatusUnauthorized, ctx.Response.StatusCode)

		ctxSug, err := round10NewLiftContext(http.MethodGet, "/api/v1/featured_tags/suggestions", nil, nil, nil)
		require.NoError(t, err)
		require.NoError(t, h.HandleGetFeaturedTagSuggestionsLift(ctxSug))
		require.Equal(t, http.StatusUnauthorized, ctxSug.Response.StatusCode)
	})

	t.Run("HandleDeleteFeaturedTagLift validates params and auth", func(t *testing.T) {
		token := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeRead})
		headers := map[string]string{"Authorization": "Bearer " + token}

		t.Run("missing id returns 400", func(t *testing.T) {
			h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
			ctx, err := round10NewLiftContext(http.MethodDelete, "/api/v1/featured_tags/", headers, nil, nil)
			require.NoError(t, err)
			require.NoError(t, h.HandleDeleteFeaturedTagLift(ctx))
			require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)
		})

		t.Run("invalid id returns 400", func(t *testing.T) {
			h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
			ctx, err := round10NewLiftContext(http.MethodDelete, "/api/v1/featured_tags/bad!", headers, nil, nil)
			require.NoError(t, err)
			ctx.SetParam("id", "bad!")
			require.NoError(t, h.HandleDeleteFeaturedTagLift(ctx))
			require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)
		})

		t.Run("missing token returns 401", func(t *testing.T) {
			h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
			ctx, err := round10NewLiftContext(http.MethodDelete, "/api/v1/featured_tags/go", nil, nil, nil)
			require.NoError(t, err)
			ctx.SetParam("id", "go")
			require.NoError(t, h.HandleDeleteFeaturedTagLift(ctx))
			require.Equal(t, http.StatusUnauthorized, ctx.Response.StatusCode)
		})
	})

	t.Run("HandleGetAccountFeaturedTagsLift validates id and returns empty list", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/accounts//featured_tags", nil, nil, nil)
		require.NoError(t, err)
		require.NoError(t, h.HandleGetAccountFeaturedTagsLift(ctx))
		require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)

		state := &round10QueryState{
			featuredTagsByUser: map[string][]storagemodels.FeaturedTag{
				"bob": {},
			},
		}
		hEmpty, _, _ := round11NewHandler(t, cfg, state)
		ctx2, err := round10NewLiftContext(http.MethodGet, "/api/v1/accounts/bob/featured_tags", nil, nil, nil)
		require.NoError(t, err)
		ctx2.SetParam("id", "bob")
		require.NoError(t, hEmpty.HandleGetAccountFeaturedTagsLift(ctx2))
		require.Equal(t, http.StatusOK, ctx2.Response.StatusCode)
		out := ctx2.Response.Body.([]FeaturedTag)
		require.Len(t, out, 0)
	})
}


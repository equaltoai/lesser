package handlers

import (
	"encoding/json"
	stdErrors "errors"
	"net/http"
	"testing"
	"time"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/auth"
	apperrors "github.com/equaltoai/lesser/pkg/errors"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestTags_FeaturedTagsCRUDAndSuggestionsRound12(t *testing.T) {
	cfg := round11TestConfig()
	now := time.Now()
	published := now.Add(-2 * time.Hour)

	makeToken := func(scopes ...string) string {
		if len(scopes) == 0 {
			scopes = []string{auth.ScopeRead}
		}
		return round11SignAccessToken(t, cfg.JWTSecret, "alice", scopes)
	}

	t.Run("get featured tags returns formatted last_status_at", func(t *testing.T) {
		state := &round10QueryState{
			featuredTagsByUser: map[string][]storagemodels.FeaturedTag{
				"alice": {
					{
						ID:            "ft-1",
						Username:      "alice",
						Name:          "go",
						URL:           "https://example.com/tags/go",
						StatusesCount: 2,
						LastStatusAt:  published.Format(time.RFC3339),
						CreatedAt:     now.Add(-24 * time.Hour),
					},
					{
						ID:            "ft-2",
						Username:      "alice",
						Name:          "ai",
						URL:           "https://example.com/tags/ai",
						StatusesCount: 0,
						LastStatusAt:  "",
						CreatedAt:     now.Add(-24 * time.Hour),
					},
				},
			},
		}
		h, _, _ := round11NewHandler(t, cfg, state)

		headers := map[string]string{"Authorization": "Bearer " + makeToken(auth.ScopeRead)}
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/featured_tags", headers, nil, nil)
		require.NoError(t, err)
		resp := requireStatus(t, http.StatusOK)(h.HandleGetFeaturedTagsLift(ctx))

		var out []FeaturedTag
		require.NoError(t, json.Unmarshal(resp.Body, &out))
		require.Len(t, out, 2)
		require.NotEmpty(t, out[0].LastStatusAt)
		require.Empty(t, out[1].LastStatusAt)
	})

	t.Run("create featured tag success and duplicate", func(t *testing.T) {
		state := &round10QueryState{
			statusList: []storagemodels.Status{
				{
					StatusID:       "s1",
					AuthorUsername: "alice",
					Note: &activitypub.Note{
						BaseObject: activitypub.BaseObject{Published: &published},
						Content:    "hello #Go",
					},
				},
			},
		}
		h, _, _ := round11NewHandler(t, cfg, state)
		headers := map[string]string{"Authorization": "Bearer " + makeToken(auth.ScopeRead)}

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/featured_tags", headers, nil, map[string]string{"name": "#Go"})
		require.NoError(t, err)
		resp := requireStatus(t, http.StatusOK)(h.HandleCreateFeaturedTagLift(ctx))

		var created FeaturedTag
		require.NoError(t, json.Unmarshal(resp.Body, &created))
		require.NotEmpty(t, created.ID)
		require.Equal(t, "go", created.Name)
		require.Equal(t, 1, created.StatusesCount)
		require.NotEmpty(t, created.LastStatusAt)

		// Duplicate: pre-seed featured tags and retry.
		dupState := &round10QueryState{
			featuredTagsByUser: map[string][]storagemodels.FeaturedTag{
				"alice": {{ID: "ft-x", Username: "alice", Name: "go", URL: "https://example.com/tags/go", CreatedAt: now}},
			},
		}
		hDup, _, _ := round11NewHandler(t, cfg, dupState)
		ctxDup, err := round10NewLiftContext(http.MethodPost, "/api/v1/featured_tags", headers, nil, map[string]string{"name": "#Go"})
		require.NoError(t, err)
		requireStatus(t, http.StatusConflict)(hDup.HandleCreateFeaturedTagLift(ctxDup))
	})

	t.Run("create featured tag validation and error branches", func(t *testing.T) {
		headers := map[string]string{"Authorization": "Bearer " + makeToken(auth.ScopeRead)}

		t.Run("invalid request body returns 400", func(t *testing.T) {
			h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
			ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v1/featured_tags", headers, nil, []byte("{bad"))
			requireStatus(t, http.StatusBadRequest)(h.HandleCreateFeaturedTagLift(ctx))
		})

		t.Run("missing name returns 400", func(t *testing.T) {
			h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
			ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/featured_tags", headers, nil, map[string]string{"name": ""})
			require.NoError(t, err)
			requireStatus(t, http.StatusBadRequest)(h.HandleCreateFeaturedTagLift(ctx))
		})

		t.Run("invalid hashtag returns 400", func(t *testing.T) {
			h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
			ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/featured_tags", headers, nil, map[string]string{"name": "#bad!"})
			require.NoError(t, err)
			requireStatus(t, http.StatusBadRequest)(h.HandleCreateFeaturedTagLift(ctx))
		})

		t.Run("limit reached maps to 422", func(t *testing.T) {
			limitErr := apperrors.NewAppError(apperrors.CodeInvalidInput, apperrors.CategoryValidation, "featured tag limit reached")
			h, _, _ := round11NewHandler(t, cfg, &round10QueryState{createErrorOnce: limitErr})
			ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/featured_tags", headers, nil, map[string]string{"name": "#Go"})
			require.NoError(t, err)
			requireStatus(t, http.StatusUnprocessableEntity)(h.HandleCreateFeaturedTagLift(ctx))
		})

		t.Run("unknown create failure returns 500", func(t *testing.T) {
			h, _, _ := round11NewHandler(t, cfg, &round10QueryState{createErrorOnce: stdErrors.New("boom")})
			ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/featured_tags", headers, nil, map[string]string{"name": "#Go"})
			require.NoError(t, err)
			requireStatus(t, http.StatusInternalServerError)(h.HandleCreateFeaturedTagLift(ctx))
		})
	})

	t.Run("delete featured tag success, not found, and error", func(t *testing.T) {
		headers := map[string]string{"Authorization": "Bearer " + makeToken(auth.ScopeRead)}

		t.Run("success returns empty object", func(t *testing.T) {
			state := &round10QueryState{
				featuredTagsByUser: map[string][]storagemodels.FeaturedTag{
					"alice": {{ID: "ft-1", Username: "alice", Name: "go", URL: "https://example.com/tags/go", CreatedAt: now}},
				},
			}
			h, _, _ := round11NewHandler(t, cfg, state)
			ctx, err := round10NewLiftContext(http.MethodDelete, "/api/v1/featured_tags/go", headers, nil, nil)
			require.NoError(t, err)
			ctx.Params["id"] = "go"
			resp := requireStatus(t, http.StatusOK)(h.HandleDeleteFeaturedTagLift(ctx))
			var out map[string]any
			require.NoError(t, json.Unmarshal(resp.Body, &out))
			require.Len(t, out, 0)
		})

		t.Run("not found returns 404", func(t *testing.T) {
			h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
			ctx, err := round10NewLiftContext(http.MethodDelete, "/api/v1/featured_tags/go", headers, nil, nil)
			require.NoError(t, err)
			ctx.Params["id"] = "go"
			requireStatus(t, http.StatusNotFound)(h.HandleDeleteFeaturedTagLift(ctx))
		})

		t.Run("delete error returns 500", func(t *testing.T) {
			state := &round10QueryState{
				featuredTagsByUser: map[string][]storagemodels.FeaturedTag{
					"alice": {{ID: "ft-1", Username: "alice", Name: "go", URL: "https://example.com/tags/go", CreatedAt: now}},
				},
				deleteErrorOnce: stdErrors.New("boom"),
			}
			h, _, _ := round11NewHandler(t, cfg, state)
			ctx, err := round10NewLiftContext(http.MethodDelete, "/api/v1/featured_tags/go", headers, nil, nil)
			require.NoError(t, err)
			ctx.Params["id"] = "go"
			requireStatus(t, http.StatusInternalServerError)(h.HandleDeleteFeaturedTagLift(ctx))
		})
	})

	t.Run("featured tag suggestions returns tags with history; error returns empty array", func(t *testing.T) {
		headers := map[string]string{"Authorization": "Bearer " + makeToken(auth.ScopeRead)}

		t.Run("success", func(t *testing.T) {
			state := &round10QueryState{
				statusList: []storagemodels.Status{
					{
						StatusID:       "s1",
						AuthorUsername: "alice",
						Note: &activitypub.Note{
							BaseObject: activitypub.BaseObject{Published: &published},
							Content:    "hello #Go #AI",
						},
					},
					{
						StatusID:       "s2",
						AuthorUsername: "alice",
						Note: &activitypub.Note{
							BaseObject: activitypub.BaseObject{Published: &published},
							Content:    "another #Go",
						},
					},
				},
			}
			h, _, _ := round11NewHandler(t, cfg, state)

			ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/featured_tags/suggestions", headers, nil, nil)
			require.NoError(t, err)
			resp := requireStatus(t, http.StatusOK)(h.HandleGetFeaturedTagSuggestionsLift(ctx))

			var out []apimodels.Tag
			require.NoError(t, json.Unmarshal(resp.Body, &out))
			require.NotEmpty(t, out)
			require.Equal(t, "go", out[0].Name)
			require.NotEmpty(t, out[0].History)
		})

		t.Run("error returns empty array", func(t *testing.T) {
			state := &round10QueryState{
				allErrorByType: map[string]error{
					"*[]models.Status": stdErrors.New("boom"),
				},
			}
			h, _, _ := round11NewHandler(t, cfg, state)

			ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/featured_tags/suggestions", headers, nil, nil)
			require.NoError(t, err)
			resp := requireStatus(t, http.StatusOK)(h.HandleGetFeaturedTagSuggestionsLift(ctx))

			// The handler intentionally returns an empty JSON array on error.
			var out []any
			require.NoError(t, json.Unmarshal(resp.Body, &out))
			require.Len(t, out, 0)
		})
	})

	t.Run("account featured tags returns list", func(t *testing.T) {
		state := &round10QueryState{
			featuredTagsByUser: map[string][]storagemodels.FeaturedTag{
				"bob": {{ID: "ft-1", Username: "bob", Name: "go", URL: "https://example.com/tags/go", CreatedAt: now}},
			},
		}
		h, _, _ := round11NewHandler(t, cfg, state)

		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/accounts/bob/featured_tags", nil, nil, nil)
		require.NoError(t, err)
		ctx.Params["id"] = "bob"

		resp := requireStatus(t, http.StatusOK)(h.HandleGetAccountFeaturedTagsLift(ctx))
		var out []FeaturedTag
		require.NoError(t, json.Unmarshal(resp.Body, &out))
		require.Len(t, out, 1)
		require.Equal(t, "go", out[0].Name)
	})
}

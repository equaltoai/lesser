package lift

import (
	"errors"
	"net/http"
	"testing"
	"time"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	dynamormerrors "github.com/pay-theory/dynamorm/pkg/errors"
	"github.com/stretchr/testify/require"
)

func TestAdminFederationLift_Round12(t *testing.T) {
	newAdminHandler := func(t *testing.T, state *round10QueryState) (*Handler, map[string]string) {
		t.Helper()

		if state == nil {
			state = &round10QueryState{}
		}
		if state.usersByUsername == nil {
			state.usersByUsername = map[string]storagemodels.User{}
		}
		state.usersByUsername["admin"] = storagemodels.User{
			PK:       "USER#admin",
			SK:       storagemodels.SKMetadata,
			Username: "admin",
			Role:     "admin",
			Approved: true,
			Version:  1,
		}

		cfg := round11TestConfig()
		h, _, _ := round11NewHandler(t, cfg, state)
		return h, map[string]string{"Authorization": "Bearer " + round10SignAccessToken(t, cfg.JWTSecret, "admin")}
	}

	t.Run("requireAdmin missing authentication", func(t *testing.T) {
		h, _ := newAdminHandler(t, &round10QueryState{})
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/admin/domain_blocks", nil, nil, nil)
		require.NoError(t, err)
		require.NoError(t, h.HandleGetAdminDomainBlocksLift(ctx))
		require.Equal(t, http.StatusForbidden, ctx.Response.StatusCode)
	})

	t.Run("requireAdmin invalid token", func(t *testing.T) {
		h, _ := newAdminHandler(t, &round10QueryState{})
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/admin/domain_blocks", map[string]string{"Authorization": "Bearer invalid"}, nil, nil)
		require.NoError(t, err)
		require.NoError(t, h.HandleGetAdminDomainBlocksLift(ctx))
		require.Equal(t, http.StatusForbidden, ctx.Response.StatusCode)
	})

	t.Run("requireAdmin non-admin user", func(t *testing.T) {
		h, _ := newAdminHandler(t, &round10QueryState{})
		cfg := round11TestConfig()
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/admin/domain_blocks",
			map[string]string{"Authorization": "Bearer " + round10SignAccessToken(t, cfg.JWTSecret, "alice")},
			nil,
			nil,
		)
		require.NoError(t, err)
		require.NoError(t, h.HandleGetAdminDomainBlocksLift(ctx))
		require.Equal(t, http.StatusForbidden, ctx.Response.StatusCode)
	})

	t.Run("domain blocks list pagination and error branches", func(t *testing.T) {
		t.Run("limit parse error defaults and omits Link", func(t *testing.T) {
			now := time.Now().Add(-1 * time.Hour)
			b1 := storagemodels.InstanceDomainBlock{ID: "b1", Domain: "blocked.example", Severity: "silence", CreatedAt: now, UpdatedAt: now}
			require.NoError(t, b1.UpdateKeys())
			state := &round10QueryState{domainBlocks: []storagemodels.InstanceDomainBlock{b1}}
			h, headers := newAdminHandler(t, state)

			ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/admin/domain_blocks", headers, map[string]string{"limit": "nope"}, nil)
			require.NoError(t, err)
			require.NoError(t, h.HandleGetAdminDomainBlocksLift(ctx))
			require.Equal(t, http.StatusOK, ctx.Response.StatusCode)
			require.NotContains(t, ctx.Response.Headers, "Link")
		})

		t.Run("paginates and sets Link header", func(t *testing.T) {
			now := time.Now().Add(-1 * time.Hour)
			b1 := storagemodels.InstanceDomainBlock{ID: "b1", Domain: "blocked.example", Severity: "silence", CreatedAt: now, UpdatedAt: now}
			b2 := storagemodels.InstanceDomainBlock{ID: "b2", Domain: "other.example", Severity: "suspend", CreatedAt: now.Add(-1 * time.Hour), UpdatedAt: now.Add(-1 * time.Hour)}
			require.NoError(t, b1.UpdateKeys())
			require.NoError(t, b2.UpdateKeys())
			state := &round10QueryState{domainBlocks: []storagemodels.InstanceDomainBlock{b1, b2}}
			h, headers := newAdminHandler(t, state)

			ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/admin/domain_blocks", headers, map[string]string{"limit": "1"}, nil)
			require.NoError(t, err)
			require.NoError(t, h.HandleGetAdminDomainBlocksLift(ctx))
			require.Equal(t, http.StatusOK, ctx.Response.StatusCode)
			require.Contains(t, ctx.Response.Headers["Link"], "max_id=")

			resp := ctx.Response.Body.([]apimodels.AdminDomainBlockResponse)
			require.Len(t, resp, 1)
		})

		t.Run("storage error returns 500", func(t *testing.T) {
			state := &round10QueryState{allErrorOnce: errors.New("boom")}
			h, headers := newAdminHandler(t, state)
			ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/admin/domain_blocks", headers, nil, nil)
			require.NoError(t, err)
			require.NoError(t, h.HandleGetAdminDomainBlocksLift(ctx))
			require.Equal(t, http.StatusInternalServerError, ctx.Response.StatusCode)
		})
	})

	t.Run("domain block get/create/update/delete branches", func(t *testing.T) {
		t.Run("get missing id", func(t *testing.T) {
			h, headers := newAdminHandler(t, &round10QueryState{})
			ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/admin/domain_blocks/:id", headers, nil, nil)
			require.NoError(t, err)
			require.NoError(t, h.HandleGetAdminDomainBlockLift(ctx))
			require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)
		})

		t.Run("get not found", func(t *testing.T) {
			h, headers := newAdminHandler(t, &round10QueryState{domainBlocks: []storagemodels.InstanceDomainBlock{}})
			ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/admin/domain_blocks/bad", headers, nil, nil)
			require.NoError(t, err)
			ctx.SetParam("id", "bad")
			require.NoError(t, h.HandleGetAdminDomainBlockLift(ctx))
			require.Equal(t, http.StatusNotFound, ctx.Response.StatusCode)
		})

		t.Run("get success", func(t *testing.T) {
			now := time.Now().Add(-1 * time.Hour)
			b1 := storagemodels.InstanceDomainBlock{ID: "b1", Domain: "blocked.example", Severity: "silence", CreatedAt: now, UpdatedAt: now}
			require.NoError(t, b1.UpdateKeys())
			h, headers := newAdminHandler(t, &round10QueryState{domainBlocks: []storagemodels.InstanceDomainBlock{b1}})

			ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/admin/domain_blocks/b1", headers, nil, nil)
			require.NoError(t, err)
			ctx.SetParam("id", "b1")
			require.NoError(t, h.HandleGetAdminDomainBlockLift(ctx))
			require.Equal(t, http.StatusOK, ctx.Response.StatusCode)
			resp := ctx.Response.Body.(apimodels.AdminDomainBlockResponse)
			require.Equal(t, "b1", resp.ID)
		})

		t.Run("get storage error returns 500", func(t *testing.T) {
			state := &round10QueryState{allErrorOnce: errors.New("boom")}
			h, headers := newAdminHandler(t, state)

			ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/admin/domain_blocks/b1", headers, nil, nil)
			require.NoError(t, err)
			ctx.SetParam("id", "b1")
			require.NoError(t, h.HandleGetAdminDomainBlockLift(ctx))
			require.Equal(t, http.StatusInternalServerError, ctx.Response.StatusCode)
		})

		t.Run("create invalid JSON", func(t *testing.T) {
			h, headers := newAdminHandler(t, &round10QueryState{})
			ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v1/admin/domain_blocks", headers, nil, []byte("{"))
			require.NoError(t, h.HandleCreateAdminDomainBlockLift(ctx))
			require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)
		})

		t.Run("create missing domain", func(t *testing.T) {
			h, headers := newAdminHandler(t, &round10QueryState{})
			ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/domain_blocks", headers, nil, apimodels.AdminDomainBlockRequest{})
			require.NoError(t, err)
			require.NoError(t, h.HandleCreateAdminDomainBlockLift(ctx))
			require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)
		})

		t.Run("create invalid domain format", func(t *testing.T) {
			h, headers := newAdminHandler(t, &round10QueryState{})
			ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/domain_blocks", headers, nil,
				apimodels.AdminDomainBlockRequest{Domain: "bad domain"},
			)
			require.NoError(t, err)
			require.NoError(t, h.HandleCreateAdminDomainBlockLift(ctx))
			require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)
		})

		t.Run("create defaults severity when missing", func(t *testing.T) {
			h, headers := newAdminHandler(t, &round10QueryState{})
			ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/domain_blocks", headers, nil,
				apimodels.AdminDomainBlockRequest{Domain: "https://Example.COM/path"},
			)
			require.NoError(t, err)
			require.NoError(t, h.HandleCreateAdminDomainBlockLift(ctx))
			require.Equal(t, http.StatusOK, ctx.Response.StatusCode)
			resp := ctx.Response.Body.(apimodels.AdminDomainBlockResponse)
			require.Equal(t, "example.com", resp.Domain)
		})

		t.Run("create invalid severity", func(t *testing.T) {
			h, headers := newAdminHandler(t, &round10QueryState{})
			ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/domain_blocks", headers, nil,
				apimodels.AdminDomainBlockRequest{Domain: "example.com", Severity: "nope"},
			)
			require.NoError(t, err)
			require.NoError(t, h.HandleCreateAdminDomainBlockLift(ctx))
			require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)
		})

		t.Run("create already exists returns 422", func(t *testing.T) {
			state := &round10QueryState{createErrorOnce: dynamormerrors.ErrConditionFailed}
			h, headers := newAdminHandler(t, state)
			ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/domain_blocks", headers, nil,
				apimodels.AdminDomainBlockRequest{Domain: "example.com", Severity: "suspend"},
			)
			require.NoError(t, err)
			require.NoError(t, h.HandleCreateAdminDomainBlockLift(ctx))
			require.Equal(t, http.StatusUnprocessableEntity, ctx.Response.StatusCode)
		})

		t.Run("create storage error returns 500", func(t *testing.T) {
			state := &round10QueryState{createErrorOnce: errors.New("boom")}
			h, headers := newAdminHandler(t, state)
			ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/domain_blocks", headers, nil,
				apimodels.AdminDomainBlockRequest{Domain: "example.com", Severity: "suspend"},
			)
			require.NoError(t, err)
			require.NoError(t, h.HandleCreateAdminDomainBlockLift(ctx))
			require.Equal(t, http.StatusInternalServerError, ctx.Response.StatusCode)
		})

		t.Run("update missing id", func(t *testing.T) {
			h, headers := newAdminHandler(t, &round10QueryState{})
			ctx, err := round10NewLiftContext(http.MethodPut, "/api/v1/admin/domain_blocks/:id", headers, nil, apimodels.AdminDomainBlockRequest{})
			require.NoError(t, err)
			require.NoError(t, h.HandleUpdateAdminDomainBlockLift(ctx))
			require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)
		})

		t.Run("update invalid JSON", func(t *testing.T) {
			h, headers := newAdminHandler(t, &round10QueryState{})
			ctx := round10NewLiftContextWithBodyBytes(http.MethodPut, "/api/v1/admin/domain_blocks/b1", headers, nil, []byte("{"))
			ctx.SetParam("id", "b1")
			require.NoError(t, h.HandleUpdateAdminDomainBlockLift(ctx))
			require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)
		})

		t.Run("update invalid severity", func(t *testing.T) {
			now := time.Now().Add(-1 * time.Hour)
			b1 := storagemodels.InstanceDomainBlock{ID: "b1", Domain: "blocked.example", Severity: "silence", CreatedAt: now, UpdatedAt: now}
			require.NoError(t, b1.UpdateKeys())
			h, headers := newAdminHandler(t, &round10QueryState{domainBlocks: []storagemodels.InstanceDomainBlock{b1}})
			ctx, err := round10NewLiftContext(http.MethodPut, "/api/v1/admin/domain_blocks/b1", headers, nil,
				apimodels.AdminDomainBlockRequest{Severity: "nope"},
			)
			require.NoError(t, err)
			ctx.SetParam("id", "b1")
			require.NoError(t, h.HandleUpdateAdminDomainBlockLift(ctx))
			require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)
		})

		t.Run("update not found returns 404", func(t *testing.T) {
			h, headers := newAdminHandler(t, &round10QueryState{})
			ctx, err := round10NewLiftContext(http.MethodPut, "/api/v1/admin/domain_blocks/missing", headers, nil, apimodels.AdminDomainBlockRequest{})
			require.NoError(t, err)
			ctx.SetParam("id", "missing")
			require.NoError(t, h.HandleUpdateAdminDomainBlockLift(ctx))
			require.Equal(t, http.StatusNotFound, ctx.Response.StatusCode)
		})

		t.Run("update storage error returns 500", func(t *testing.T) {
			now := time.Now().Add(-1 * time.Hour)
			b1 := storagemodels.InstanceDomainBlock{ID: "b1", Domain: "blocked.example", Severity: "silence", CreatedAt: now, UpdatedAt: now}
			require.NoError(t, b1.UpdateKeys())
			state := &round10QueryState{domainBlocks: []storagemodels.InstanceDomainBlock{b1}, updateErrorOnce: errors.New("boom")}
			h, headers := newAdminHandler(t, state)
			ctx, err := round10NewLiftContext(http.MethodPut, "/api/v1/admin/domain_blocks/b1", headers, nil,
				apimodels.AdminDomainBlockRequest{PrivateComment: "note"},
			)
			require.NoError(t, err)
			ctx.SetParam("id", "b1")
			require.NoError(t, h.HandleUpdateAdminDomainBlockLift(ctx))
			require.Equal(t, http.StatusInternalServerError, ctx.Response.StatusCode)
		})

		t.Run("update success", func(t *testing.T) {
			now := time.Now().Add(-1 * time.Hour)
			b1 := storagemodels.InstanceDomainBlock{ID: "b1", Domain: "blocked.example", Severity: "silence", CreatedAt: now, UpdatedAt: now}
			require.NoError(t, b1.UpdateKeys())
			h, headers := newAdminHandler(t, &round10QueryState{domainBlocks: []storagemodels.InstanceDomainBlock{b1}})
			ctx, err := round10NewLiftContext(http.MethodPut, "/api/v1/admin/domain_blocks/b1", headers, nil,
				apimodels.AdminDomainBlockRequest{PrivateComment: "note"},
			)
			require.NoError(t, err)
			ctx.SetParam("id", "b1")
			require.NoError(t, h.HandleUpdateAdminDomainBlockLift(ctx))
			require.Equal(t, http.StatusOK, ctx.Response.StatusCode)
		})

		t.Run("delete missing id", func(t *testing.T) {
			h, headers := newAdminHandler(t, &round10QueryState{})
			ctx, err := round10NewLiftContext(http.MethodDelete, "/api/v1/admin/domain_blocks/:id", headers, nil, nil)
			require.NoError(t, err)
			require.NoError(t, h.HandleDeleteAdminDomainBlockLift(ctx))
			require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)
		})

		t.Run("delete not found", func(t *testing.T) {
			h, headers := newAdminHandler(t, &round10QueryState{})
			ctx, err := round10NewLiftContext(http.MethodDelete, "/api/v1/admin/domain_blocks/missing", headers, nil, nil)
			require.NoError(t, err)
			ctx.SetParam("id", "missing")
			require.NoError(t, h.HandleDeleteAdminDomainBlockLift(ctx))
			require.Equal(t, http.StatusNotFound, ctx.Response.StatusCode)
		})

		t.Run("delete storage error returns 500", func(t *testing.T) {
			now := time.Now().Add(-1 * time.Hour)
			b1 := storagemodels.InstanceDomainBlock{ID: "b1", Domain: "blocked.example", Severity: "silence", CreatedAt: now, UpdatedAt: now}
			require.NoError(t, b1.UpdateKeys())
			state := &round10QueryState{domainBlocks: []storagemodels.InstanceDomainBlock{b1}, deleteErrorOnce: errors.New("boom")}
			h, headers := newAdminHandler(t, state)
			ctx, err := round10NewLiftContext(http.MethodDelete, "/api/v1/admin/domain_blocks/b1", headers, nil, nil)
			require.NoError(t, err)
			ctx.SetParam("id", "b1")
			require.NoError(t, h.HandleDeleteAdminDomainBlockLift(ctx))
			require.Equal(t, http.StatusInternalServerError, ctx.Response.StatusCode)
		})

		t.Run("delete success", func(t *testing.T) {
			now := time.Now().Add(-1 * time.Hour)
			b1 := storagemodels.InstanceDomainBlock{ID: "b1", Domain: "blocked.example", Severity: "silence", CreatedAt: now, UpdatedAt: now}
			require.NoError(t, b1.UpdateKeys())
			h, headers := newAdminHandler(t, &round10QueryState{domainBlocks: []storagemodels.InstanceDomainBlock{b1}})
			ctx, err := round10NewLiftContext(http.MethodDelete, "/api/v1/admin/domain_blocks/b1", headers, nil, nil)
			require.NoError(t, err)
			ctx.SetParam("id", "b1")
			require.NoError(t, h.HandleDeleteAdminDomainBlockLift(ctx))
			require.Equal(t, http.StatusOK, ctx.Response.StatusCode)
		})
	})

	t.Run("domain allows + email domain blocks", func(t *testing.T) {
		t.Run("domain allows list and create/delete branches", func(t *testing.T) {
			t.Run("list sets Link when paginating", func(t *testing.T) {
				now := time.Now().Add(-1 * time.Hour)
				a1 := storagemodels.DomainAllow{ID: "a1", Domain: "allow1.example", CreatedAt: now}
				a2 := storagemodels.DomainAllow{ID: "a2", Domain: "allow2.example", CreatedAt: now.Add(-1 * time.Hour)}
				require.NoError(t, a1.UpdateKeys())
				require.NoError(t, a2.UpdateKeys())

				state := &round10QueryState{domainAllows: []storagemodels.DomainAllow{a1, a2}}
				h, headers := newAdminHandler(t, state)
				ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/admin/domain_allows", headers, map[string]string{"limit": "1"}, nil)
				require.NoError(t, err)
				require.NoError(t, h.HandleGetAdminDomainAllowsLift(ctx))
				require.Equal(t, http.StatusOK, ctx.Response.StatusCode)
				require.Contains(t, ctx.Response.Headers["Link"], "max_id=")
			})

			t.Run("list storage error returns 500", func(t *testing.T) {
				state := &round10QueryState{allErrorOnce: errors.New("boom")}
				h, headers := newAdminHandler(t, state)
				ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/admin/domain_allows", headers, nil, nil)
				require.NoError(t, err)
				require.NoError(t, h.HandleGetAdminDomainAllowsLift(ctx))
				require.Equal(t, http.StatusInternalServerError, ctx.Response.StatusCode)
			})

			t.Run("list limit parse error defaults and omits Link", func(t *testing.T) {
				now := time.Now().Add(-1 * time.Hour)
				a1 := storagemodels.DomainAllow{ID: "a1", Domain: "allow1.example", CreatedAt: now}
				require.NoError(t, a1.UpdateKeys())

				state := &round10QueryState{domainAllows: []storagemodels.DomainAllow{a1}}
				h, headers := newAdminHandler(t, state)
				ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/admin/domain_allows", headers, map[string]string{"limit": "nope"}, nil)
				require.NoError(t, err)
				require.NoError(t, h.HandleGetAdminDomainAllowsLift(ctx))
				require.Equal(t, http.StatusOK, ctx.Response.StatusCode)
				require.NotContains(t, ctx.Response.Headers, "Link")
			})

			t.Run("create invalid JSON", func(t *testing.T) {
				h, headers := newAdminHandler(t, &round10QueryState{})
				ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v1/admin/domain_allows", headers, nil, []byte("{"))
				require.NoError(t, h.HandleCreateAdminDomainAllowLift(ctx))
				require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)
			})

			t.Run("create missing domain", func(t *testing.T) {
				h, headers := newAdminHandler(t, &round10QueryState{})
				ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/domain_allows", headers, nil, apimodels.AdminDomainAllowRequest{})
				require.NoError(t, err)
				require.NoError(t, h.HandleCreateAdminDomainAllowLift(ctx))
				require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)
			})

			t.Run("create invalid domain format", func(t *testing.T) {
				h, headers := newAdminHandler(t, &round10QueryState{})
				ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/domain_allows", headers, nil, apimodels.AdminDomainAllowRequest{Domain: "bad domain"})
				require.NoError(t, err)
				require.NoError(t, h.HandleCreateAdminDomainAllowLift(ctx))
				require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)
			})

			t.Run("create already exists returns 422", func(t *testing.T) {
				state := &round10QueryState{createErrorOnce: dynamormerrors.ErrConditionFailed}
				h, headers := newAdminHandler(t, state)
				ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/domain_allows", headers, nil, apimodels.AdminDomainAllowRequest{Domain: "example.com"})
				require.NoError(t, err)
				require.NoError(t, h.HandleCreateAdminDomainAllowLift(ctx))
				require.Equal(t, http.StatusUnprocessableEntity, ctx.Response.StatusCode)
			})

			t.Run("create storage error returns 500", func(t *testing.T) {
				state := &round10QueryState{createErrorOnce: errors.New("boom")}
				h, headers := newAdminHandler(t, state)
				ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/domain_allows", headers, nil, apimodels.AdminDomainAllowRequest{Domain: "example.com"})
				require.NoError(t, err)
				require.NoError(t, h.HandleCreateAdminDomainAllowLift(ctx))
				require.Equal(t, http.StatusInternalServerError, ctx.Response.StatusCode)
			})

			t.Run("create success", func(t *testing.T) {
				h, headers := newAdminHandler(t, &round10QueryState{})
				ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/domain_allows", headers, nil, apimodels.AdminDomainAllowRequest{Domain: "example.com"})
				require.NoError(t, err)
				require.NoError(t, h.HandleCreateAdminDomainAllowLift(ctx))
				require.Equal(t, http.StatusOK, ctx.Response.StatusCode)
			})

			t.Run("delete missing id", func(t *testing.T) {
				h, headers := newAdminHandler(t, &round10QueryState{})
				ctx, err := round10NewLiftContext(http.MethodDelete, "/api/v1/admin/domain_allows/:id", headers, nil, nil)
				require.NoError(t, err)
				require.NoError(t, h.HandleDeleteAdminDomainAllowLift(ctx))
				require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)
			})

			t.Run("delete forbidden without auth", func(t *testing.T) {
				h, _ := newAdminHandler(t, &round10QueryState{})
				ctx, err := round10NewLiftContext(http.MethodDelete, "/api/v1/admin/domain_allows/a1", nil, nil, nil)
				require.NoError(t, err)
				ctx.SetParam("id", "a1")
				require.NoError(t, h.HandleDeleteAdminDomainAllowLift(ctx))
				require.Equal(t, http.StatusForbidden, ctx.Response.StatusCode)
			})

			t.Run("delete not found", func(t *testing.T) {
				state := &round10QueryState{domainAllows: []storagemodels.DomainAllow{{ID: "a1", Domain: "allow.example"}}}
				h, headers := newAdminHandler(t, state)
				ctx, err := round10NewLiftContext(http.MethodDelete, "/api/v1/admin/domain_allows/missing", headers, nil, nil)
				require.NoError(t, err)
				ctx.SetParam("id", "missing")
				require.NoError(t, h.HandleDeleteAdminDomainAllowLift(ctx))
				require.Equal(t, http.StatusNotFound, ctx.Response.StatusCode)
			})

			t.Run("delete storage error returns 500", func(t *testing.T) {
				state := &round10QueryState{
					domainAllows:    []storagemodels.DomainAllow{{ID: "a1", Domain: "allow.example"}},
					deleteErrorOnce: errors.New("boom"),
				}
				h, headers := newAdminHandler(t, state)
				ctx, err := round10NewLiftContext(http.MethodDelete, "/api/v1/admin/domain_allows/a1", headers, nil, nil)
				require.NoError(t, err)
				ctx.SetParam("id", "a1")
				require.NoError(t, h.HandleDeleteAdminDomainAllowLift(ctx))
				require.Equal(t, http.StatusInternalServerError, ctx.Response.StatusCode)
			})

			t.Run("delete success", func(t *testing.T) {
				state := &round10QueryState{
					domainAllows: []storagemodels.DomainAllow{{ID: "a1", Domain: "allow.example"}},
				}
				h, headers := newAdminHandler(t, state)
				ctx, err := round10NewLiftContext(http.MethodDelete, "/api/v1/admin/domain_allows/a1", headers, nil, nil)
				require.NoError(t, err)
				ctx.SetParam("id", "a1")
				require.NoError(t, h.HandleDeleteAdminDomainAllowLift(ctx))
				require.Equal(t, http.StatusOK, ctx.Response.StatusCode)
			})
		})

		t.Run("email domain blocks list and create/delete branches", func(t *testing.T) {
			t.Run("list paginates and sets next cursor", func(t *testing.T) {
				now := time.Now().Add(-1 * time.Hour)
				b1 := storagemodels.EmailDomainBlock{ID: "e1", Domain: "blocked.example", CreatedAt: now}
				b2 := storagemodels.EmailDomainBlock{ID: "e2", Domain: "blocked2.example", CreatedAt: now.Add(-1 * time.Hour)}
				require.NoError(t, b1.UpdateKeys())
				require.NoError(t, b2.UpdateKeys())

				state := &round10QueryState{emailDomainBlocks: []storagemodels.EmailDomainBlock{b1, b2}}
				h, headers := newAdminHandler(t, state)
				ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/admin/email_domain_blocks", headers, map[string]string{"limit": "1"}, nil)
				require.NoError(t, err)
				require.NoError(t, h.HandleGetEmailDomainBlocksLift(ctx))
				require.Equal(t, http.StatusOK, ctx.Response.StatusCode)
				resp := ctx.Response.Body.(apimodels.EmailDomainBlocksResponse)
				require.NotNil(t, resp.NextCursor)
			})

			t.Run("list storage error returns 500", func(t *testing.T) {
				state := &round10QueryState{allErrorOnce: errors.New("boom")}
				h, headers := newAdminHandler(t, state)
				ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/admin/email_domain_blocks", headers, nil, nil)
				require.NoError(t, err)
				require.NoError(t, h.HandleGetEmailDomainBlocksLift(ctx))
				require.Equal(t, http.StatusInternalServerError, ctx.Response.StatusCode)
			})

			t.Run("create invalid JSON", func(t *testing.T) {
				h, headers := newAdminHandler(t, &round10QueryState{})
				ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v1/admin/email_domain_blocks", headers, nil, []byte("{"))
				require.NoError(t, h.HandleCreateEmailDomainBlockLift(ctx))
				require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)
			})

			t.Run("create missing domain", func(t *testing.T) {
				h, headers := newAdminHandler(t, &round10QueryState{})
				ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/email_domain_blocks", headers, nil, apimodels.EmailDomainBlockRequest{})
				require.NoError(t, err)
				require.NoError(t, h.HandleCreateEmailDomainBlockLift(ctx))
				require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)
			})

			t.Run("create already exists returns 422", func(t *testing.T) {
				state := &round10QueryState{createErrorOnce: dynamormerrors.ErrConditionFailed}
				h, headers := newAdminHandler(t, state)
				ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/email_domain_blocks", headers, nil, apimodels.EmailDomainBlockRequest{Domain: "@Example.COM"})
				require.NoError(t, err)
				require.NoError(t, h.HandleCreateEmailDomainBlockLift(ctx))
				require.Equal(t, http.StatusUnprocessableEntity, ctx.Response.StatusCode)
			})

			t.Run("create storage error returns 500", func(t *testing.T) {
				state := &round10QueryState{createErrorOnce: errors.New("boom")}
				h, headers := newAdminHandler(t, state)
				ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/email_domain_blocks", headers, nil, apimodels.EmailDomainBlockRequest{Domain: "@Example.COM"})
				require.NoError(t, err)
				require.NoError(t, h.HandleCreateEmailDomainBlockLift(ctx))
				require.Equal(t, http.StatusInternalServerError, ctx.Response.StatusCode)
			})

			t.Run("create success normalizes domain", func(t *testing.T) {
				h, headers := newAdminHandler(t, &round10QueryState{})
				ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/email_domain_blocks", headers, nil, apimodels.EmailDomainBlockRequest{Domain: "@Example.COM"})
				require.NoError(t, err)
				require.NoError(t, h.HandleCreateEmailDomainBlockLift(ctx))
				require.Equal(t, http.StatusOK, ctx.Response.StatusCode)
				resp := ctx.Response.Body.(apimodels.EmailDomainBlockResponse)
				require.Equal(t, "example.com", resp.Domain)
			})

			t.Run("delete missing id", func(t *testing.T) {
				h, headers := newAdminHandler(t, &round10QueryState{})
				ctx, err := round10NewLiftContext(http.MethodDelete, "/api/v1/admin/email_domain_blocks/:id", headers, nil, nil)
				require.NoError(t, err)
				require.NoError(t, h.HandleDeleteEmailDomainBlockLift(ctx))
				require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)
			})

			t.Run("delete not found", func(t *testing.T) {
				state := &round10QueryState{emailDomainBlocks: []storagemodels.EmailDomainBlock{{ID: "e1", Domain: "blocked.example"}}}
				h, headers := newAdminHandler(t, state)
				ctx, err := round10NewLiftContext(http.MethodDelete, "/api/v1/admin/email_domain_blocks/missing", headers, nil, nil)
				require.NoError(t, err)
				ctx.SetParam("id", "missing")
				require.NoError(t, h.HandleDeleteEmailDomainBlockLift(ctx))
				require.Equal(t, http.StatusNotFound, ctx.Response.StatusCode)
			})

			t.Run("delete storage error returns 500", func(t *testing.T) {
				state := &round10QueryState{
					emailDomainBlocks: []storagemodels.EmailDomainBlock{{ID: "e1", Domain: "blocked.example"}},
					deleteErrorOnce:   errors.New("boom"),
				}
				h, headers := newAdminHandler(t, state)
				ctx, err := round10NewLiftContext(http.MethodDelete, "/api/v1/admin/email_domain_blocks/e1", headers, nil, nil)
				require.NoError(t, err)
				ctx.SetParam("id", "e1")
				require.NoError(t, h.HandleDeleteEmailDomainBlockLift(ctx))
				require.Equal(t, http.StatusInternalServerError, ctx.Response.StatusCode)
			})

			t.Run("delete success", func(t *testing.T) {
				state := &round10QueryState{
					emailDomainBlocks: []storagemodels.EmailDomainBlock{{ID: "e1", Domain: "blocked.example"}},
				}
				h, headers := newAdminHandler(t, state)
				ctx, err := round10NewLiftContext(http.MethodDelete, "/api/v1/admin/email_domain_blocks/e1", headers, nil, nil)
				require.NoError(t, err)
				ctx.SetParam("id", "e1")
				require.NoError(t, h.HandleDeleteEmailDomainBlockLift(ctx))
				require.Equal(t, http.StatusOK, ctx.Response.StatusCode)
			})
		})
	})

	t.Run("federation instance endpoints", func(t *testing.T) {
		t.Run("instances list scan error returns 500", func(t *testing.T) {
			state := &round10QueryState{scanErrorOnce: errors.New("boom")}
			h, headers := newAdminHandler(t, state)
			ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/admin/federation/instances", headers, nil, nil)
			require.NoError(t, err)
			require.NoError(t, h.HandleGetFederationInstancesLift(ctx))
			require.Equal(t, http.StatusInternalServerError, ctx.Response.StatusCode)
		})

		t.Run("instances list success includes block flags", func(t *testing.T) {
			now := time.Now().Add(-1 * time.Hour)
			block := storagemodels.InstanceDomainBlock{ID: "b1", Domain: "remote.example", Severity: "silence", CreatedAt: now, UpdatedAt: now}
			require.NoError(t, block.UpdateKeys())

			state := &round10QueryState{
				domainBlocks: []storagemodels.InstanceDomainBlock{block},
				federationInstances: []storagemodels.FederationInstance{
					{
						Domain:        "remote.example",
						Software:      "mastodon",
						Version:       "4.0.0",
						ActiveUsers:   5,
						TotalMessages: 10,
						TrustScore:    0.9,
						FirstSeen:     now.Add(-24 * time.Hour),
						LastSeen:      now,
					},
				},
			}

			h, headers := newAdminHandler(t, state)
			ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/admin/federation/instances", headers, map[string]string{"limit": "10"}, nil)
			require.NoError(t, err)
			require.NoError(t, h.HandleGetFederationInstancesLift(ctx))
			require.Equal(t, http.StatusOK, ctx.Response.StatusCode)
			resp := ctx.Response.Body.(apimodels.FederationInstancesResponse)
			require.Len(t, resp.Instances, 1)
			require.True(t, resp.Instances[0].IsSilenced)
		})

		t.Run("get instance missing domain", func(t *testing.T) {
			h, headers := newAdminHandler(t, &round10QueryState{})
			ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/admin/federation/instance/:domain", headers, nil, nil)
			require.NoError(t, err)
			require.NoError(t, h.HandleGetFederationInstanceLift(ctx))
			require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)
		})

		t.Run("get instance not found", func(t *testing.T) {
			h, headers := newAdminHandler(t, &round10QueryState{})
			ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/admin/federation/instance/missing", headers, nil, nil)
			require.NoError(t, err)
			ctx.SetParam("domain", "missing")
			require.NoError(t, h.HandleGetFederationInstanceLift(ctx))
			require.Equal(t, http.StatusNotFound, ctx.Response.StatusCode)
		})

		t.Run("get instance storage error returns 500", func(t *testing.T) {
			state := &round10QueryState{firstErrorPK: map[string]error{"INSTANCE#err": errors.New("boom")}}
			h, headers := newAdminHandler(t, state)
			ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/admin/federation/instance/err", headers, nil, nil)
			require.NoError(t, err)
			ctx.SetParam("domain", "err")
			require.NoError(t, h.HandleGetFederationInstanceLift(ctx))
			require.Equal(t, http.StatusInternalServerError, ctx.Response.StatusCode)
		})

		t.Run("get instance warns on domain stats/block errors", func(t *testing.T) {
			now := time.Now().Add(-1 * time.Hour)
			instance := storagemodels.FederationInstance{
				Domain:        "remote.example",
				Software:      "mastodon",
				Version:       "4.0.0",
				FirstSeen:     now.Add(-24 * time.Hour),
				LastSeen:      now,
				TrustScore:    0.9,
				ActiveUsers:   5,
				TotalMessages: 10,
			}
			instance.UpdateKeys()

			state := &round10QueryState{
				federationInstancesByDomain: map[string]storagemodels.FederationInstance{"remote.example": instance},
				firstErrorPK: map[string]error{
					"DOMAIN#remote.example":       errors.New("boom"),
					"DOMAIN_BLOCK#remote.example": errors.New("boom2"),
				},
			}
			h, headers := newAdminHandler(t, state)
			ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/admin/federation/instance/remote.example", headers, nil, nil)
			require.NoError(t, err)
			ctx.SetParam("domain", "remote.example")
			require.NoError(t, h.HandleGetFederationInstanceLift(ctx))
			require.Equal(t, http.StatusOK, ctx.Response.StatusCode)
			resp := ctx.Response.Body.(apimodels.FederationInstanceResponse)
			require.Empty(t, resp.Details)
		})

		t.Run("get instance success", func(t *testing.T) {
			now := time.Now().Add(-1 * time.Hour)
			instance := storagemodels.FederationInstance{
				Domain:        "remote.example",
				Software:      "mastodon",
				Version:       "4.0.0",
				FirstSeen:     now.Add(-24 * time.Hour),
				LastSeen:      now,
				TrustScore:    0.9,
				ActiveUsers:   5,
				TotalMessages: 10,
			}
			instance.UpdateKeys()

			state := &round10QueryState{
				federationInstancesByDomain: map[string]storagemodels.FederationInstance{"remote.example": instance},
				instanceMetrics: map[string]storagemodels.InstanceMetrics{
					"DOMAIN#remote.example#STATS": {Value: 7, UpdatedAt: now.Add(-5 * time.Minute)},
				},
			}
			h, headers := newAdminHandler(t, state)
			ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/admin/federation/instance/remote.example", headers, nil, nil)
			require.NoError(t, err)
			ctx.SetParam("domain", "https://remote.example/path")
			require.NoError(t, h.HandleGetFederationInstanceLift(ctx))
			require.Equal(t, http.StatusOK, ctx.Response.StatusCode)
			resp := ctx.Response.Body.(apimodels.FederationInstanceResponse)
			require.Equal(t, "remote.example", resp.Instance.Domain)
		})

		t.Run("statistics scan error returns 500", func(t *testing.T) {
			state := &round10QueryState{scanErrorOnce: errors.New("boom")}
			h, headers := newAdminHandler(t, state)
			ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/admin/federation/statistics", headers, nil, nil)
			require.NoError(t, err)
			require.NoError(t, h.HandleGetFederationStatisticsLift(ctx))
			require.Equal(t, http.StatusInternalServerError, ctx.Response.StatusCode)
		})

		t.Run("statistics parses time range and aggregates", func(t *testing.T) {
			now := time.Now().Add(-1 * time.Hour).UTC()
			start := now.Add(-7 * 24 * time.Hour)
			end := now

			state := &round10QueryState{
				federationInstances: []storagemodels.FederationInstance{
					{Domain: "a.example", ActiveUsers: 2, TotalMessages: 3},
					{Domain: "b.example", ActiveUsers: 4, TotalMessages: 5},
				},
			}
			h, headers := newAdminHandler(t, state)
			ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/admin/federation/statistics", headers, map[string]string{
				"start": start.Format(time.RFC3339),
				"end":   end.Format(time.RFC3339),
			}, nil)
			require.NoError(t, err)
			require.NoError(t, h.HandleGetFederationStatisticsLift(ctx))
			require.Equal(t, http.StatusOK, ctx.Response.StatusCode)
			resp := ctx.Response.Body.(apimodels.FederationStatisticsResponse)
			require.Equal(t, int64(2), resp.ActiveInstances)
			require.Equal(t, int64(8), resp.TotalMessages)
			require.Equal(t, int64(6), resp.TotalUsers)
		})
	})
}

func TestAdminFederationHelpers_Round12(t *testing.T) {
	require.Equal(t, "example.com", cleanDomain("https://Example.COM:443/path"))
	require.Equal(t, "example.com", cleanDomain("http://Example.COM:80/"))

	m := map[string]any{"a": 1}
	require.Equal(t, 1, getFieldOrDefault(m, "a", 2))
	require.Equal(t, 2, getFieldOrDefault(m, "missing", 2))
}

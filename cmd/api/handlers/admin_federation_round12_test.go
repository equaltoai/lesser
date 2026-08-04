package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
	dynamormerrors "github.com/theory-cloud/tabletheory/v3/pkg/errors"
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
		requireStatus(t, http.StatusForbidden)(h.HandleGetAdminDomainBlocksLift(ctx))
	})

	t.Run("requireAdmin invalid token", func(t *testing.T) {
		h, _ := newAdminHandler(t, &round10QueryState{})
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/admin/domain_blocks", map[string]string{"Authorization": "Bearer invalid"}, nil, nil)
		require.NoError(t, err)
		requireStatus(t, http.StatusForbidden)(h.HandleGetAdminDomainBlocksLift(ctx))
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
		requireStatus(t, http.StatusForbidden)(h.HandleGetAdminDomainBlocksLift(ctx))
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
			resp := requireStatus(t, http.StatusOK)(h.HandleGetAdminDomainBlocksLift(ctx))
			require.Empty(t, resp.Headers["link"])
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
			resp := requireStatus(t, http.StatusOK)(h.HandleGetAdminDomainBlocksLift(ctx))
			require.NotEmpty(t, resp.Headers["link"])
			require.Contains(t, resp.Headers["link"][0], "max_id=")

			var body []apimodels.AdminDomainBlockResponse
			require.NoError(t, json.Unmarshal(resp.Body, &body))
			require.Len(t, body, 1)
		})

		t.Run("storage error returns 500", func(t *testing.T) {
			state := &round10QueryState{allErrorOnce: errors.New("boom")}
			h, headers := newAdminHandler(t, state)
			ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/admin/domain_blocks", headers, nil, nil)
			require.NoError(t, err)
			requireStatus(t, http.StatusInternalServerError)(h.HandleGetAdminDomainBlocksLift(ctx))
		})
	})

	t.Run("domain block get/create/update/delete branches", func(t *testing.T) {
		t.Run("get missing id", func(t *testing.T) {
			h, headers := newAdminHandler(t, &round10QueryState{})
			ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/admin/domain_blocks/:id", headers, nil, nil)
			require.NoError(t, err)
			requireStatus(t, http.StatusBadRequest)(h.HandleGetAdminDomainBlockLift(ctx))
		})

		t.Run("get not found", func(t *testing.T) {
			h, headers := newAdminHandler(t, &round10QueryState{domainBlocks: []storagemodels.InstanceDomainBlock{}})
			ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/admin/domain_blocks/bad", headers, nil, nil)
			require.NoError(t, err)
			ctx.Params["id"] = "bad"
			requireStatus(t, http.StatusNotFound)(h.HandleGetAdminDomainBlockLift(ctx))
		})

		t.Run("get success", func(t *testing.T) {
			now := time.Now().Add(-1 * time.Hour)
			b1 := storagemodels.InstanceDomainBlock{ID: "b1", Domain: "blocked.example", Severity: "silence", CreatedAt: now, UpdatedAt: now}
			require.NoError(t, b1.UpdateKeys())
			h, headers := newAdminHandler(t, &round10QueryState{domainBlocks: []storagemodels.InstanceDomainBlock{b1}})

			ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/admin/domain_blocks/b1", headers, nil, nil)
			require.NoError(t, err)
			ctx.Params["id"] = "b1"
			resp := requireStatus(t, http.StatusOK)(h.HandleGetAdminDomainBlockLift(ctx))
			var body apimodels.AdminDomainBlockResponse
			require.NoError(t, json.Unmarshal(resp.Body, &body))
			require.Equal(t, "b1", body.ID)
		})

		t.Run("get storage error returns 500", func(t *testing.T) {
			state := &round10QueryState{allErrorOnce: errors.New("boom")}
			h, headers := newAdminHandler(t, state)

			ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/admin/domain_blocks/b1", headers, nil, nil)
			require.NoError(t, err)
			ctx.Params["id"] = "b1"
			requireStatus(t, http.StatusInternalServerError)(h.HandleGetAdminDomainBlockLift(ctx))
		})

		t.Run("create invalid JSON", func(t *testing.T) {
			h, headers := newAdminHandler(t, &round10QueryState{})
			ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v1/admin/domain_blocks", headers, nil, []byte("{"))
			requireStatus(t, http.StatusBadRequest)(h.HandleCreateAdminDomainBlockLift(ctx))
		})

		t.Run("create missing domain", func(t *testing.T) {
			h, headers := newAdminHandler(t, &round10QueryState{})
			ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/domain_blocks", headers, nil, apimodels.AdminDomainBlockRequest{})
			require.NoError(t, err)
			requireStatus(t, http.StatusBadRequest)(h.HandleCreateAdminDomainBlockLift(ctx))
		})

		t.Run("create invalid domain format", func(t *testing.T) {
			h, headers := newAdminHandler(t, &round10QueryState{})
			ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/domain_blocks", headers, nil,
				apimodels.AdminDomainBlockRequest{Domain: "bad domain"},
			)
			require.NoError(t, err)
			requireStatus(t, http.StatusBadRequest)(h.HandleCreateAdminDomainBlockLift(ctx))
		})

		t.Run("create defaults severity when missing", func(t *testing.T) {
			h, headers := newAdminHandler(t, &round10QueryState{})
			ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/domain_blocks", headers, nil,
				apimodels.AdminDomainBlockRequest{Domain: "https://Example.COM/path"},
			)
			require.NoError(t, err)
			resp := requireStatus(t, http.StatusOK)(h.HandleCreateAdminDomainBlockLift(ctx))
			var body apimodels.AdminDomainBlockResponse
			require.NoError(t, json.Unmarshal(resp.Body, &body))
			require.Equal(t, "example.com", body.Domain)
		})

		t.Run("create invalid severity", func(t *testing.T) {
			h, headers := newAdminHandler(t, &round10QueryState{})
			ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/domain_blocks", headers, nil,
				apimodels.AdminDomainBlockRequest{Domain: "example.com", Severity: "nope"},
			)
			require.NoError(t, err)
			requireStatus(t, http.StatusBadRequest)(h.HandleCreateAdminDomainBlockLift(ctx))
		})

		t.Run("create already exists returns 422", func(t *testing.T) {
			state := &round10QueryState{createErrorOnce: dynamormerrors.ErrConditionFailed}
			h, headers := newAdminHandler(t, state)
			ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/domain_blocks", headers, nil,
				apimodels.AdminDomainBlockRequest{Domain: "example.com", Severity: "suspend"},
			)
			require.NoError(t, err)
			requireStatus(t, http.StatusUnprocessableEntity)(h.HandleCreateAdminDomainBlockLift(ctx))
		})

		t.Run("create storage error returns 500", func(t *testing.T) {
			state := &round10QueryState{createErrorOnce: errors.New("boom")}
			h, headers := newAdminHandler(t, state)
			ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/domain_blocks", headers, nil,
				apimodels.AdminDomainBlockRequest{Domain: "example.com", Severity: "suspend"},
			)
			require.NoError(t, err)
			requireStatus(t, http.StatusInternalServerError)(h.HandleCreateAdminDomainBlockLift(ctx))
		})

		t.Run("update missing id", func(t *testing.T) {
			h, headers := newAdminHandler(t, &round10QueryState{})
			ctx, err := round10NewLiftContext(http.MethodPut, "/api/v1/admin/domain_blocks/:id", headers, nil, apimodels.AdminDomainBlockRequest{})
			require.NoError(t, err)
			requireStatus(t, http.StatusBadRequest)(h.HandleUpdateAdminDomainBlockLift(ctx))
		})

		t.Run("update invalid JSON", func(t *testing.T) {
			h, headers := newAdminHandler(t, &round10QueryState{})
			ctx := round10NewLiftContextWithBodyBytes(http.MethodPut, "/api/v1/admin/domain_blocks/b1", headers, nil, []byte("{"))
			ctx.Params["id"] = "b1"
			requireStatus(t, http.StatusBadRequest)(h.HandleUpdateAdminDomainBlockLift(ctx))
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
			ctx.Params["id"] = "b1"
			requireStatus(t, http.StatusBadRequest)(h.HandleUpdateAdminDomainBlockLift(ctx))
		})

		t.Run("update not found returns 404", func(t *testing.T) {
			h, headers := newAdminHandler(t, &round10QueryState{})
			ctx, err := round10NewLiftContext(http.MethodPut, "/api/v1/admin/domain_blocks/missing", headers, nil, apimodels.AdminDomainBlockRequest{})
			require.NoError(t, err)
			ctx.Params["id"] = "missing"
			requireStatus(t, http.StatusNotFound)(h.HandleUpdateAdminDomainBlockLift(ctx))
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
			ctx.Params["id"] = "b1"
			requireStatus(t, http.StatusInternalServerError)(h.HandleUpdateAdminDomainBlockLift(ctx))
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
			ctx.Params["id"] = "b1"
			requireStatus(t, http.StatusOK)(h.HandleUpdateAdminDomainBlockLift(ctx))
		})

		t.Run("delete missing id", func(t *testing.T) {
			h, headers := newAdminHandler(t, &round10QueryState{})
			ctx, err := round10NewLiftContext(http.MethodDelete, "/api/v1/admin/domain_blocks/:id", headers, nil, nil)
			require.NoError(t, err)
			requireStatus(t, http.StatusBadRequest)(h.HandleDeleteAdminDomainBlockLift(ctx))
		})

		t.Run("delete not found", func(t *testing.T) {
			h, headers := newAdminHandler(t, &round10QueryState{})
			ctx, err := round10NewLiftContext(http.MethodDelete, "/api/v1/admin/domain_blocks/missing", headers, nil, nil)
			require.NoError(t, err)
			ctx.Params["id"] = "missing"
			requireStatus(t, http.StatusNotFound)(h.HandleDeleteAdminDomainBlockLift(ctx))
		})

		t.Run("delete storage error returns 500", func(t *testing.T) {
			now := time.Now().Add(-1 * time.Hour)
			b1 := storagemodels.InstanceDomainBlock{ID: "b1", Domain: "blocked.example", Severity: "silence", CreatedAt: now, UpdatedAt: now}
			require.NoError(t, b1.UpdateKeys())
			state := &round10QueryState{domainBlocks: []storagemodels.InstanceDomainBlock{b1}, deleteErrorOnce: errors.New("boom")}
			h, headers := newAdminHandler(t, state)
			ctx, err := round10NewLiftContext(http.MethodDelete, "/api/v1/admin/domain_blocks/b1", headers, nil, nil)
			require.NoError(t, err)
			ctx.Params["id"] = "b1"
			requireStatus(t, http.StatusInternalServerError)(h.HandleDeleteAdminDomainBlockLift(ctx))
		})

		t.Run("delete success", func(t *testing.T) {
			now := time.Now().Add(-1 * time.Hour)
			b1 := storagemodels.InstanceDomainBlock{ID: "b1", Domain: "blocked.example", Severity: "silence", CreatedAt: now, UpdatedAt: now}
			require.NoError(t, b1.UpdateKeys())
			h, headers := newAdminHandler(t, &round10QueryState{domainBlocks: []storagemodels.InstanceDomainBlock{b1}})
			ctx, err := round10NewLiftContext(http.MethodDelete, "/api/v1/admin/domain_blocks/b1", headers, nil, nil)
			require.NoError(t, err)
			ctx.Params["id"] = "b1"
			requireStatus(t, http.StatusOK)(h.HandleDeleteAdminDomainBlockLift(ctx))
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
				resp := requireStatus(t, http.StatusOK)(h.HandleGetAdminDomainAllowsLift(ctx))
				require.Contains(t, firstStringValue(resp.Headers, "link"), "max_id=")
			})

			t.Run("list storage error returns 500", func(t *testing.T) {
				state := &round10QueryState{allErrorOnce: errors.New("boom")}
				h, headers := newAdminHandler(t, state)
				ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/admin/domain_allows", headers, nil, nil)
				require.NoError(t, err)
				requireStatus(t, http.StatusInternalServerError)(h.HandleGetAdminDomainAllowsLift(ctx))
			})

			t.Run("list limit parse error defaults and omits Link", func(t *testing.T) {
				now := time.Now().Add(-1 * time.Hour)
				a1 := storagemodels.DomainAllow{ID: "a1", Domain: "allow1.example", CreatedAt: now}
				require.NoError(t, a1.UpdateKeys())

				state := &round10QueryState{domainAllows: []storagemodels.DomainAllow{a1}}
				h, headers := newAdminHandler(t, state)
				ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/admin/domain_allows", headers, map[string]string{"limit": "nope"}, nil)
				require.NoError(t, err)
				resp := requireStatus(t, http.StatusOK)(h.HandleGetAdminDomainAllowsLift(ctx))
				require.Empty(t, resp.Headers["link"])
			})

			t.Run("create invalid JSON", func(t *testing.T) {
				h, headers := newAdminHandler(t, &round10QueryState{})
				ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v1/admin/domain_allows", headers, nil, []byte("{"))
				requireStatus(t, http.StatusBadRequest)(h.HandleCreateAdminDomainAllowLift(ctx))
			})

			t.Run("create missing domain", func(t *testing.T) {
				h, headers := newAdminHandler(t, &round10QueryState{})
				ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/domain_allows", headers, nil, apimodels.AdminDomainAllowRequest{})
				require.NoError(t, err)
				requireStatus(t, http.StatusBadRequest)(h.HandleCreateAdminDomainAllowLift(ctx))
			})

			t.Run("create invalid domain format", func(t *testing.T) {
				h, headers := newAdminHandler(t, &round10QueryState{})
				ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/domain_allows", headers, nil, apimodels.AdminDomainAllowRequest{Domain: "bad domain"})
				require.NoError(t, err)
				requireStatus(t, http.StatusBadRequest)(h.HandleCreateAdminDomainAllowLift(ctx))
			})

			t.Run("create already exists returns 422", func(t *testing.T) {
				state := &round10QueryState{createErrorOnce: dynamormerrors.ErrConditionFailed}
				h, headers := newAdminHandler(t, state)
				ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/domain_allows", headers, nil, apimodels.AdminDomainAllowRequest{Domain: "example.com"})
				require.NoError(t, err)
				requireStatus(t, http.StatusUnprocessableEntity)(h.HandleCreateAdminDomainAllowLift(ctx))
			})

			t.Run("create storage error returns 500", func(t *testing.T) {
				state := &round10QueryState{createErrorOnce: errors.New("boom")}
				h, headers := newAdminHandler(t, state)
				ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/domain_allows", headers, nil, apimodels.AdminDomainAllowRequest{Domain: "example.com"})
				require.NoError(t, err)
				requireStatus(t, http.StatusInternalServerError)(h.HandleCreateAdminDomainAllowLift(ctx))
			})

			t.Run("create success", func(t *testing.T) {
				h, headers := newAdminHandler(t, &round10QueryState{})
				ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/domain_allows", headers, nil, apimodels.AdminDomainAllowRequest{Domain: "example.com"})
				require.NoError(t, err)
				requireStatus(t, http.StatusOK)(h.HandleCreateAdminDomainAllowLift(ctx))
			})

			t.Run("delete missing id", func(t *testing.T) {
				h, headers := newAdminHandler(t, &round10QueryState{})
				ctx, err := round10NewLiftContext(http.MethodDelete, "/api/v1/admin/domain_allows/:id", headers, nil, nil)
				require.NoError(t, err)
				requireStatus(t, http.StatusBadRequest)(h.HandleDeleteAdminDomainAllowLift(ctx))
			})

			t.Run("delete forbidden without auth", func(t *testing.T) {
				h, _ := newAdminHandler(t, &round10QueryState{})
				ctx, err := round10NewLiftContext(http.MethodDelete, "/api/v1/admin/domain_allows/a1", nil, nil, nil)
				require.NoError(t, err)
				ctx.Params["id"] = "a1"
				requireStatus(t, http.StatusForbidden)(h.HandleDeleteAdminDomainAllowLift(ctx))
			})

			t.Run("delete not found", func(t *testing.T) {
				state := &round10QueryState{domainAllows: []storagemodels.DomainAllow{{ID: "a1", Domain: "allow.example"}}}
				h, headers := newAdminHandler(t, state)
				ctx, err := round10NewLiftContext(http.MethodDelete, "/api/v1/admin/domain_allows/missing", headers, nil, nil)
				require.NoError(t, err)
				ctx.Params["id"] = "missing"
				requireStatus(t, http.StatusNotFound)(h.HandleDeleteAdminDomainAllowLift(ctx))
			})

			t.Run("delete storage error returns 500", func(t *testing.T) {
				state := &round10QueryState{
					domainAllows:    []storagemodels.DomainAllow{{ID: "a1", Domain: "allow.example"}},
					deleteErrorOnce: errors.New("boom"),
				}
				h, headers := newAdminHandler(t, state)
				ctx, err := round10NewLiftContext(http.MethodDelete, "/api/v1/admin/domain_allows/a1", headers, nil, nil)
				require.NoError(t, err)
				ctx.Params["id"] = "a1"
				requireStatus(t, http.StatusInternalServerError)(h.HandleDeleteAdminDomainAllowLift(ctx))
			})

			t.Run("delete success", func(t *testing.T) {
				state := &round10QueryState{
					domainAllows: []storagemodels.DomainAllow{{ID: "a1", Domain: "allow.example"}},
				}
				h, headers := newAdminHandler(t, state)
				ctx, err := round10NewLiftContext(http.MethodDelete, "/api/v1/admin/domain_allows/a1", headers, nil, nil)
				require.NoError(t, err)
				ctx.Params["id"] = "a1"
				requireStatus(t, http.StatusOK)(h.HandleDeleteAdminDomainAllowLift(ctx))
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
				resp := requireStatus(t, http.StatusOK)(h.HandleGetEmailDomainBlocksLift(ctx))
				var body apimodels.EmailDomainBlocksResponse
				require.NoError(t, json.Unmarshal(resp.Body, &body))
				require.NotNil(t, body.NextCursor)
			})

			t.Run("list storage error returns 500", func(t *testing.T) {
				state := &round10QueryState{allErrorOnce: errors.New("boom")}
				h, headers := newAdminHandler(t, state)
				ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/admin/email_domain_blocks", headers, nil, nil)
				require.NoError(t, err)
				requireStatus(t, http.StatusInternalServerError)(h.HandleGetEmailDomainBlocksLift(ctx))
			})

			t.Run("create invalid JSON", func(t *testing.T) {
				h, headers := newAdminHandler(t, &round10QueryState{})
				ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v1/admin/email_domain_blocks", headers, nil, []byte("{"))
				requireStatus(t, http.StatusBadRequest)(h.HandleCreateEmailDomainBlockLift(ctx))
			})

			t.Run("create missing domain", func(t *testing.T) {
				h, headers := newAdminHandler(t, &round10QueryState{})
				ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/email_domain_blocks", headers, nil, apimodels.EmailDomainBlockRequest{})
				require.NoError(t, err)
				requireStatus(t, http.StatusBadRequest)(h.HandleCreateEmailDomainBlockLift(ctx))
			})

			t.Run("create already exists returns 422", func(t *testing.T) {
				state := &round10QueryState{createErrorOnce: dynamormerrors.ErrConditionFailed}
				h, headers := newAdminHandler(t, state)
				ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/email_domain_blocks", headers, nil, apimodels.EmailDomainBlockRequest{Domain: "@Example.COM"})
				require.NoError(t, err)
				requireStatus(t, http.StatusUnprocessableEntity)(h.HandleCreateEmailDomainBlockLift(ctx))
			})

			t.Run("create storage error returns 500", func(t *testing.T) {
				state := &round10QueryState{createErrorOnce: errors.New("boom")}
				h, headers := newAdminHandler(t, state)
				ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/email_domain_blocks", headers, nil, apimodels.EmailDomainBlockRequest{Domain: "@Example.COM"})
				require.NoError(t, err)
				requireStatus(t, http.StatusInternalServerError)(h.HandleCreateEmailDomainBlockLift(ctx))
			})

			t.Run("create success normalizes domain", func(t *testing.T) {
				h, headers := newAdminHandler(t, &round10QueryState{})
				ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/email_domain_blocks", headers, nil, apimodels.EmailDomainBlockRequest{Domain: "@Example.COM"})
				require.NoError(t, err)
				resp := requireStatus(t, http.StatusOK)(h.HandleCreateEmailDomainBlockLift(ctx))
				var body apimodels.EmailDomainBlockResponse
				require.NoError(t, json.Unmarshal(resp.Body, &body))
				require.Equal(t, "example.com", body.Domain)
			})

			t.Run("delete missing id", func(t *testing.T) {
				h, headers := newAdminHandler(t, &round10QueryState{})
				ctx, err := round10NewLiftContext(http.MethodDelete, "/api/v1/admin/email_domain_blocks/:id", headers, nil, nil)
				require.NoError(t, err)
				requireStatus(t, http.StatusBadRequest)(h.HandleDeleteEmailDomainBlockLift(ctx))
			})

			t.Run("delete not found", func(t *testing.T) {
				state := &round10QueryState{emailDomainBlocks: []storagemodels.EmailDomainBlock{{ID: "e1", Domain: "blocked.example"}}}
				h, headers := newAdminHandler(t, state)
				ctx, err := round10NewLiftContext(http.MethodDelete, "/api/v1/admin/email_domain_blocks/missing", headers, nil, nil)
				require.NoError(t, err)
				ctx.Params["id"] = "missing"
				requireStatus(t, http.StatusNotFound)(h.HandleDeleteEmailDomainBlockLift(ctx))
			})

			t.Run("delete storage error returns 500", func(t *testing.T) {
				state := &round10QueryState{
					emailDomainBlocks: []storagemodels.EmailDomainBlock{{ID: "e1", Domain: "blocked.example"}},
					deleteErrorOnce:   errors.New("boom"),
				}
				h, headers := newAdminHandler(t, state)
				ctx, err := round10NewLiftContext(http.MethodDelete, "/api/v1/admin/email_domain_blocks/e1", headers, nil, nil)
				require.NoError(t, err)
				ctx.Params["id"] = "e1"
				requireStatus(t, http.StatusInternalServerError)(h.HandleDeleteEmailDomainBlockLift(ctx))
			})

			t.Run("delete success", func(t *testing.T) {
				state := &round10QueryState{
					emailDomainBlocks: []storagemodels.EmailDomainBlock{{ID: "e1", Domain: "blocked.example"}},
				}
				h, headers := newAdminHandler(t, state)
				ctx, err := round10NewLiftContext(http.MethodDelete, "/api/v1/admin/email_domain_blocks/e1", headers, nil, nil)
				require.NoError(t, err)
				ctx.Params["id"] = "e1"
				requireStatus(t, http.StatusOK)(h.HandleDeleteEmailDomainBlockLift(ctx))
			})
		})
	})

	t.Run("federation instance endpoints", func(t *testing.T) {
		t.Run("instances list scan error returns 500", func(t *testing.T) {
			state := &round10QueryState{scanErrorOnce: errors.New("boom")}
			h, headers := newAdminHandler(t, state)
			ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/admin/federation/instances", headers, nil, nil)
			require.NoError(t, err)
			requireStatus(t, http.StatusInternalServerError)(h.HandleGetFederationInstancesLift(ctx))
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
			resp := requireStatus(t, http.StatusOK)(h.HandleGetFederationInstancesLift(ctx))
			var body apimodels.FederationInstancesResponse
			require.NoError(t, json.Unmarshal(resp.Body, &body))
			require.Len(t, body.Instances, 1)
			require.True(t, body.Instances[0].IsSilenced)
		})

		t.Run("get instance missing domain", func(t *testing.T) {
			h, headers := newAdminHandler(t, &round10QueryState{})
			ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/admin/federation/instance/:domain", headers, nil, nil)
			require.NoError(t, err)
			requireStatus(t, http.StatusBadRequest)(h.HandleGetFederationInstanceLift(ctx))
		})

		t.Run("get instance not found", func(t *testing.T) {
			h, headers := newAdminHandler(t, &round10QueryState{})
			ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/admin/federation/instance/missing", headers, nil, nil)
			require.NoError(t, err)
			ctx.Params["domain"] = "missing"
			requireStatus(t, http.StatusNotFound)(h.HandleGetFederationInstanceLift(ctx))
		})

		t.Run("get instance storage error returns 500", func(t *testing.T) {
			state := &round10QueryState{firstErrorPK: map[string]error{"INSTANCE#err": errors.New("boom")}}
			h, headers := newAdminHandler(t, state)
			ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/admin/federation/instance/err", headers, nil, nil)
			require.NoError(t, err)
			ctx.Params["domain"] = "err"
			requireStatus(t, http.StatusInternalServerError)(h.HandleGetFederationInstanceLift(ctx))
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
			ctx.Params["domain"] = "remote.example"
			resp := requireStatus(t, http.StatusOK)(h.HandleGetFederationInstanceLift(ctx))
			var body apimodels.FederationInstanceResponse
			require.NoError(t, json.Unmarshal(resp.Body, &body))
			require.Empty(t, body.Details)
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
			ctx.Params["domain"] = "https://remote.example/path"
			resp := requireStatus(t, http.StatusOK)(h.HandleGetFederationInstanceLift(ctx))
			var body apimodels.FederationInstanceResponse
			require.NoError(t, json.Unmarshal(resp.Body, &body))
			require.Equal(t, "remote.example", body.Instance.Domain)
		})

		t.Run("statistics scan error returns 500", func(t *testing.T) {
			state := &round10QueryState{scanErrorOnce: errors.New("boom")}
			h, headers := newAdminHandler(t, state)
			ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/admin/federation/statistics", headers, nil, nil)
			require.NoError(t, err)
			requireStatus(t, http.StatusInternalServerError)(h.HandleGetFederationStatisticsLift(ctx))
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
			resp := requireStatus(t, http.StatusOK)(h.HandleGetFederationStatisticsLift(ctx))
			var body apimodels.FederationStatisticsResponse
			require.NoError(t, json.Unmarshal(resp.Body, &body))
			require.Equal(t, int64(2), body.ActiveInstances)
			require.Equal(t, int64(8), body.TotalMessages)
			require.Equal(t, int64(6), body.TotalUsers)
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

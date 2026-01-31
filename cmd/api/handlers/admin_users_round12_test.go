package lift

import (
	"context"
	"net/http"
	"testing"
	"time"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/auth"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestAdminCreateUserRound12(t *testing.T) {
	cfg := round11TestConfig()

	adminToken := round11SignAccessToken(t, cfg.JWTSecret, "admin", []string{auth.ScopeAdmin})

	t.Run("unauthorized without admin auth", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/users", nil, nil, apimodels.AdminCreateUserRequest{})
		require.NoError(t, err)

		require.NoError(t, h.HandleAdminCreateUserLift(ctx))
		require.Equal(t, http.StatusUnauthorized, ctx.Response.StatusCode)
	})

	t.Run("unprocessable entity when body invalid", func(t *testing.T) {
		state := &round10QueryState{
			usersByUsername: map[string]storagemodels.User{
				"admin": {Username: "admin", Role: "admin", Approved: true, Version: 1, CreatedAt: time.Now()},
			},
		}
		h, _, _ := round11NewHandler(t, cfg, state)
		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v1/admin/users", map[string]string{
			"Authorization": "Bearer " + adminToken,
		}, nil, []byte("{"))

		require.NoError(t, h.HandleAdminCreateUserLift(ctx))
		require.Equal(t, http.StatusUnprocessableEntity, ctx.Response.StatusCode)
	})

	t.Run("internal error when user create fails", func(t *testing.T) {
		state := &round10QueryState{
			usersByUsername: map[string]storagemodels.User{
				"admin": {Username: "admin", Role: "admin", Approved: true, Version: 1, CreatedAt: time.Now()},
			},
			createErrorOnce: context.Canceled,
		}
		h, _, _ := round11NewHandler(t, cfg, state)
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/users", map[string]string{
			"Authorization": "Bearer " + adminToken,
		}, nil, apimodels.AdminCreateUserRequest{
			Username:    "bob",
			Email:       "bob@example.com",
			Password:    "password",
			DisplayName: "Bob",
			Role:        "user",
		})
		require.NoError(t, err)

		require.NoError(t, h.HandleAdminCreateUserLift(ctx))
		require.Equal(t, http.StatusInternalServerError, ctx.Response.StatusCode)
	})

	t.Run("success creates user", func(t *testing.T) {
		state := &round10QueryState{
			usersByUsername: map[string]storagemodels.User{
				"admin": {Username: "admin", Role: "admin", Approved: true, Version: 1, CreatedAt: time.Now()},
			},
		}
		h, _, _ := round11NewHandler(t, cfg, state)
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/users", map[string]string{
			"Authorization": "Bearer " + adminToken,
		}, nil, apimodels.AdminCreateUserRequest{
			Username:    "bob",
			Email:       "bob@example.com",
			Password:    "password",
			DisplayName: "Bob",
			Role:        "user",
		})
		require.NoError(t, err)

		require.NoError(t, h.HandleAdminCreateUserLift(ctx))
		require.Equal(t, http.StatusCreated, ctx.Response.StatusCode)
	})
}

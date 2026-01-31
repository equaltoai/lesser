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

func TestPushSubscriptionsRound12(t *testing.T) {
	cfg := round11TestConfig()

	readToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeRead})
	writeToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeWrite})
	pushToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{"push"})

	t.Run("get subscription unauthorized", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/push/subscription", nil, nil, nil)
		require.NoError(t, err)

		require.NoError(t, h.HandleGetPushSubscriptionLift(ctx))
		require.Equal(t, http.StatusUnauthorized, ctx.Response.StatusCode)
	})

	t.Run("get subscription invalid token", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/push/subscription", map[string]string{
			"Authorization": "Bearer invalid",
		}, nil, nil)
		require.NoError(t, err)

		require.NoError(t, h.HandleGetPushSubscriptionLift(ctx))
		require.Equal(t, http.StatusUnauthorized, ctx.Response.StatusCode)
	})

	t.Run("get subscription insufficient scope", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/push/subscription", map[string]string{
			"Authorization": "Bearer " + writeToken,
		}, nil, nil)
		require.NoError(t, err)

		require.NoError(t, h.HandleGetPushSubscriptionLift(ctx))
		require.Equal(t, http.StatusForbidden, ctx.Response.StatusCode)
	})

	t.Run("get subscription repo error", func(t *testing.T) {
		state := &round10QueryState{allErrorOnce: context.Canceled}
		h, _, _ := round11NewHandler(t, cfg, state)
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/push/subscription", map[string]string{
			"Authorization": "Bearer " + readToken,
		}, nil, nil)
		require.NoError(t, err)

		require.NoError(t, h.HandleGetPushSubscriptionLift(ctx))
		require.Equal(t, http.StatusInternalServerError, ctx.Response.StatusCode)
	})

	t.Run("get subscription empty response when none exist", func(t *testing.T) {
		state := &round10QueryState{
			pushSubscriptionsByUser: map[string][]storagemodels.PushSubscription{
				"alice": {},
			},
		}
		h, _, _ := round11NewHandler(t, cfg, state)
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/push/subscription", map[string]string{
			"Authorization": "Bearer " + readToken,
		}, nil, nil)
		require.NoError(t, err)

		require.NoError(t, h.HandleGetPushSubscriptionLift(ctx))
		require.Equal(t, http.StatusOK, ctx.Response.StatusCode)
		body := ctx.Response.Body.(map[string]any)
		require.Equal(t, "", body["id"])
	})

	t.Run("get subscription success with VAPID keys missing", func(t *testing.T) {
		state := &round10QueryState{
			pushSubscriptionsByUser: map[string][]storagemodels.PushSubscription{
				"alice": {{
					ID:        "sub-1",
					Username:  "alice",
					Endpoint:  "https://push.example.com",
					P256dh:    "p256",
					Auth:      "auth",
					CreatedAt: time.Now().Add(-1 * time.Hour),
					UpdatedAt: time.Now(),
				}},
			},
			forceVapidNotFound: true,
		}
		h, _, _ := round11NewHandler(t, cfg, state)
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/push/subscription", map[string]string{
			"Authorization": "Bearer " + pushToken,
		}, nil, nil)
		require.NoError(t, err)

		require.NoError(t, h.HandleGetPushSubscriptionLift(ctx))
		require.Equal(t, http.StatusOK, ctx.Response.StatusCode)
		resp := ctx.Response.Body.(apimodels.PushSubscription)
		require.Equal(t, "", resp.ServerKey)
	})

	t.Run("create subscription unauthorized", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/push/subscription", nil, nil, apimodels.PushSubscriptionRequest{})
		require.NoError(t, err)

		require.NoError(t, h.HandleCreatePushSubscriptionLift(ctx))
		require.Equal(t, http.StatusUnauthorized, ctx.Response.StatusCode)
	})

	t.Run("create subscription invalid token", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/push/subscription", map[string]string{
			"Authorization": "Bearer invalid",
		}, nil, apimodels.PushSubscriptionRequest{})
		require.NoError(t, err)

		require.NoError(t, h.HandleCreatePushSubscriptionLift(ctx))
		require.Equal(t, http.StatusUnauthorized, ctx.Response.StatusCode)
	})

	t.Run("create subscription insufficient scope", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/push/subscription", map[string]string{
			"Authorization": "Bearer " + readToken,
		}, nil, apimodels.PushSubscriptionRequest{})
		require.NoError(t, err)

		require.NoError(t, h.HandleCreatePushSubscriptionLift(ctx))
		require.Equal(t, http.StatusForbidden, ctx.Response.StatusCode)
	})

	t.Run("create subscription empty body returns bad request", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/push/subscription", map[string]string{
			"Authorization": "Bearer " + pushToken,
		}, nil, nil)
		require.NoError(t, err)

		require.NoError(t, h.HandleCreatePushSubscriptionLift(ctx))
		require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)
	})

	t.Run("create subscription validates required fields", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/push/subscription", map[string]string{
			"Authorization": "Bearer " + writeToken,
		}, nil, apimodels.PushSubscriptionRequest{})
		require.NoError(t, err)

		require.NoError(t, h.HandleCreatePushSubscriptionLift(ctx))
		require.Equal(t, http.StatusUnprocessableEntity, ctx.Response.StatusCode)
	})

	t.Run("create subscription missing p256dh returns 422", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/push/subscription", map[string]string{
			"Authorization": "Bearer " + pushToken,
		}, nil, apimodels.PushSubscriptionRequest{
			Subscription: apimodels.PushSubscriptionData{
				Endpoint: "https://push.example.com",
				Keys:     apimodels.PushSubscriptionKeys{Auth: "auth"},
			},
		})
		require.NoError(t, err)

		require.NoError(t, h.HandleCreatePushSubscriptionLift(ctx))
		require.Equal(t, http.StatusUnprocessableEntity, ctx.Response.StatusCode)
	})

	t.Run("create subscription missing auth returns 422", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/push/subscription", map[string]string{
			"Authorization": "Bearer " + pushToken,
		}, nil, apimodels.PushSubscriptionRequest{
			Subscription: apimodels.PushSubscriptionData{
				Endpoint: "https://push.example.com",
				Keys:     apimodels.PushSubscriptionKeys{P256dh: "p256"},
			},
		})
		require.NoError(t, err)

		require.NoError(t, h.HandleCreatePushSubscriptionLift(ctx))
		require.Equal(t, http.StatusUnprocessableEntity, ctx.Response.StatusCode)
	})

	t.Run("create subscription invalid body", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v1/push/subscription", map[string]string{
			"Authorization": "Bearer " + writeToken,
		}, nil, []byte("{"))

		require.NoError(t, h.HandleCreatePushSubscriptionLift(ctx))
		require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)
	})

	t.Run("create subscription continues when deleting old subscription fails", func(t *testing.T) {
		state := &round10QueryState{deleteErrorOnce: context.Canceled}
		h, _, _ := round11NewHandler(t, cfg, state)
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/push/subscription", map[string]string{
			"Authorization": "Bearer " + pushToken,
		}, nil, apimodels.PushSubscriptionRequest{
			Subscription: apimodels.PushSubscriptionData{
				Endpoint: "https://push.example.com",
				Keys:     apimodels.PushSubscriptionKeys{P256dh: "p256", Auth: "auth"},
			},
			Data: apimodels.PushSubscriptionAlerts{Follow: true},
		})
		require.NoError(t, err)

		require.NoError(t, h.HandleCreatePushSubscriptionLift(ctx))
		require.Equal(t, http.StatusOK, ctx.Response.StatusCode)
		resp := ctx.Response.Body.(apimodels.PushSubscription)
		require.Equal(t, "https://push.example.com", resp.Endpoint)
	})

	t.Run("create subscription success with VAPID keys missing", func(t *testing.T) {
		state := &round10QueryState{forceVapidNotFound: true}
		h, _, _ := round11NewHandler(t, cfg, state)
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/push/subscription", map[string]string{
			"Authorization": "Bearer " + pushToken,
		}, nil, apimodels.PushSubscriptionRequest{
			Subscription: apimodels.PushSubscriptionData{
				Endpoint: "https://push.example.com",
				Keys:     apimodels.PushSubscriptionKeys{P256dh: "p256", Auth: "auth"},
			},
			Data: apimodels.PushSubscriptionAlerts{Follow: true},
		})
		require.NoError(t, err)

		require.NoError(t, h.HandleCreatePushSubscriptionLift(ctx))
		require.Equal(t, http.StatusOK, ctx.Response.StatusCode)
		resp := ctx.Response.Body.(apimodels.PushSubscription)
		require.Empty(t, resp.ServerKey)
	})

	t.Run("create subscription success includes server key", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/push/subscription", map[string]string{
			"Authorization": "Bearer " + pushToken,
		}, nil, apimodels.PushSubscriptionRequest{
			Subscription: apimodels.PushSubscriptionData{
				Endpoint: "https://push.example.com",
				Keys:     apimodels.PushSubscriptionKeys{P256dh: "p256", Auth: "auth"},
			},
			Data: apimodels.PushSubscriptionAlerts{Follow: true},
		})
		require.NoError(t, err)

		require.NoError(t, h.HandleCreatePushSubscriptionLift(ctx))
		require.Equal(t, http.StatusOK, ctx.Response.StatusCode)
		resp := ctx.Response.Body.(apimodels.PushSubscription)
		require.Equal(t, "pub", resp.ServerKey)
	})

	t.Run("create subscription storage failure returns 500", func(t *testing.T) {
		state := &round10QueryState{createErrorOnce: context.Canceled}
		h, _, _ := round11NewHandler(t, cfg, state)
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/push/subscription", map[string]string{
			"Authorization": "Bearer " + writeToken,
		}, nil, apimodels.PushSubscriptionRequest{
			Subscription: apimodels.PushSubscriptionData{
				Endpoint: "https://push.example.com",
				Keys:     apimodels.PushSubscriptionKeys{P256dh: "p256", Auth: "auth"},
			},
			Data: apimodels.PushSubscriptionAlerts{Follow: true},
		})
		require.NoError(t, err)

		require.NoError(t, h.HandleCreatePushSubscriptionLift(ctx))
		require.Equal(t, http.StatusInternalServerError, ctx.Response.StatusCode)
	})

	t.Run("update subscription unauthorized", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
		ctx, err := round10NewLiftContext(http.MethodPut, "/api/v1/push/subscription", nil, nil, apimodels.PushSubscriptionRequest{})
		require.NoError(t, err)

		require.NoError(t, h.HandleUpdatePushSubscriptionLift(ctx))
		require.Equal(t, http.StatusUnauthorized, ctx.Response.StatusCode)
	})

	t.Run("update subscription invalid token", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
		ctx, err := round10NewLiftContext(http.MethodPut, "/api/v1/push/subscription", map[string]string{
			"Authorization": "Bearer invalid",
		}, nil, apimodels.PushSubscriptionRequest{})
		require.NoError(t, err)

		require.NoError(t, h.HandleUpdatePushSubscriptionLift(ctx))
		require.Equal(t, http.StatusUnauthorized, ctx.Response.StatusCode)
	})

	t.Run("update subscription insufficient scope", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
		ctx, err := round10NewLiftContext(http.MethodPut, "/api/v1/push/subscription", map[string]string{
			"Authorization": "Bearer " + readToken,
		}, nil, apimodels.PushSubscriptionRequest{})
		require.NoError(t, err)

		require.NoError(t, h.HandleUpdatePushSubscriptionLift(ctx))
		require.Equal(t, http.StatusForbidden, ctx.Response.StatusCode)
	})

	t.Run("update subscription not found", func(t *testing.T) {
		state := &round10QueryState{
			pushSubscriptionsByUser: map[string][]storagemodels.PushSubscription{
				"alice": {},
			},
		}
		h, _, _ := round11NewHandler(t, cfg, state)
		ctx, err := round10NewLiftContext(http.MethodPut, "/api/v1/push/subscription", map[string]string{
			"Authorization": "Bearer " + writeToken,
		}, nil, apimodels.PushSubscriptionRequest{Data: apimodels.PushSubscriptionAlerts{Follow: true}})
		require.NoError(t, err)

		require.NoError(t, h.HandleUpdatePushSubscriptionLift(ctx))
		require.Equal(t, http.StatusNotFound, ctx.Response.StatusCode)
	})

	t.Run("update subscription repo error treated as not found", func(t *testing.T) {
		state := &round10QueryState{allErrorOnce: context.Canceled}
		h, _, _ := round11NewHandler(t, cfg, state)
		ctx, err := round10NewLiftContext(http.MethodPut, "/api/v1/push/subscription", map[string]string{
			"Authorization": "Bearer " + writeToken,
		}, nil, apimodels.PushSubscriptionRequest{Data: apimodels.PushSubscriptionAlerts{Follow: true}})
		require.NoError(t, err)

		require.NoError(t, h.HandleUpdatePushSubscriptionLift(ctx))
		require.Equal(t, http.StatusNotFound, ctx.Response.StatusCode)
	})

	t.Run("update subscription invalid body", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
		ctx := round10NewLiftContextWithBodyBytes(http.MethodPut, "/api/v1/push/subscription", map[string]string{
			"Authorization": "Bearer " + writeToken,
		}, nil, []byte("{"))

		require.NoError(t, h.HandleUpdatePushSubscriptionLift(ctx))
		require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)
	})

	t.Run("update subscription success with VAPID keys missing", func(t *testing.T) {
		state := &round10QueryState{
			forceVapidNotFound: true,
			pushSubscriptionsByUser: map[string][]storagemodels.PushSubscription{
				"alice": {{
					ID:        "sub-1",
					Username:  "alice",
					Endpoint:  "https://push.example.com",
					P256dh:    "p256",
					Auth:      "auth",
					CreatedAt: time.Now().Add(-1 * time.Hour),
					UpdatedAt: time.Now(),
				}},
			},
		}
		h, _, _ := round11NewHandler(t, cfg, state)
		ctx, err := round10NewLiftContext(http.MethodPut, "/api/v1/push/subscription", map[string]string{
			"Authorization": "Bearer " + pushToken,
		}, nil, apimodels.PushSubscriptionRequest{Data: apimodels.PushSubscriptionAlerts{Follow: true}})
		require.NoError(t, err)

		require.NoError(t, h.HandleUpdatePushSubscriptionLift(ctx))
		require.Equal(t, http.StatusOK, ctx.Response.StatusCode)
		resp := ctx.Response.Body.(apimodels.PushSubscription)
		require.Empty(t, resp.ServerKey)
	})

	t.Run("update subscription success includes server key", func(t *testing.T) {
		state := &round10QueryState{
			pushSubscriptionsByUser: map[string][]storagemodels.PushSubscription{
				"alice": {{
					ID:        "sub-1",
					Username:  "alice",
					Endpoint:  "https://push.example.com",
					P256dh:    "p256",
					Auth:      "auth",
					CreatedAt: time.Now().Add(-1 * time.Hour),
					UpdatedAt: time.Now(),
				}},
			},
		}
		h, _, _ := round11NewHandler(t, cfg, state)
		ctx, err := round10NewLiftContext(http.MethodPut, "/api/v1/push/subscription", map[string]string{
			"Authorization": "Bearer " + pushToken,
		}, nil, apimodels.PushSubscriptionRequest{Data: apimodels.PushSubscriptionAlerts{Follow: true}})
		require.NoError(t, err)

		require.NoError(t, h.HandleUpdatePushSubscriptionLift(ctx))
		require.Equal(t, http.StatusOK, ctx.Response.StatusCode)
		resp := ctx.Response.Body.(apimodels.PushSubscription)
		require.Equal(t, "pub", resp.ServerKey)
	})

	t.Run("update subscription update failure", func(t *testing.T) {
		state := &round10QueryState{
			updateErrorOnce: context.Canceled,
			pushSubscriptionsByUser: map[string][]storagemodels.PushSubscription{
				"alice": {{
					ID:        "sub-1",
					Username:  "alice",
					Endpoint:  "https://push.example.com",
					P256dh:    "p256",
					Auth:      "auth",
					CreatedAt: time.Now().Add(-1 * time.Hour),
					UpdatedAt: time.Now(),
				}},
			},
		}
		h, _, _ := round11NewHandler(t, cfg, state)
		ctx, err := round10NewLiftContext(http.MethodPut, "/api/v1/push/subscription", map[string]string{
			"Authorization": "Bearer " + writeToken,
		}, nil, apimodels.PushSubscriptionRequest{Data: apimodels.PushSubscriptionAlerts{Follow: true}})
		require.NoError(t, err)

		require.NoError(t, h.HandleUpdatePushSubscriptionLift(ctx))
		require.Equal(t, http.StatusInternalServerError, ctx.Response.StatusCode)
	})

	t.Run("delete subscription unauthorized", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
		ctx, err := round10NewLiftContext(http.MethodDelete, "/api/v1/push/subscription", nil, nil, nil)
		require.NoError(t, err)

		require.NoError(t, h.HandleDeletePushSubscriptionLift(ctx))
		require.Equal(t, http.StatusUnauthorized, ctx.Response.StatusCode)
	})

	t.Run("delete subscription invalid token", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
		ctx, err := round10NewLiftContext(http.MethodDelete, "/api/v1/push/subscription", map[string]string{
			"Authorization": "Bearer invalid",
		}, nil, nil)
		require.NoError(t, err)

		require.NoError(t, h.HandleDeletePushSubscriptionLift(ctx))
		require.Equal(t, http.StatusUnauthorized, ctx.Response.StatusCode)
	})

	t.Run("delete subscription insufficient scope", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
		ctx, err := round10NewLiftContext(http.MethodDelete, "/api/v1/push/subscription", map[string]string{
			"Authorization": "Bearer " + readToken,
		}, nil, nil)
		require.NoError(t, err)

		require.NoError(t, h.HandleDeletePushSubscriptionLift(ctx))
		require.Equal(t, http.StatusForbidden, ctx.Response.StatusCode)
	})

	t.Run("delete subscription delete failure", func(t *testing.T) {
		state := &round10QueryState{allErrorOnce: context.Canceled}
		h, _, _ := round11NewHandler(t, cfg, state)
		ctx, err := round10NewLiftContext(http.MethodDelete, "/api/v1/push/subscription", map[string]string{
			"Authorization": "Bearer " + writeToken,
		}, nil, nil)
		require.NoError(t, err)

		require.NoError(t, h.HandleDeletePushSubscriptionLift(ctx))
		require.Equal(t, http.StatusInternalServerError, ctx.Response.StatusCode)
	})

	t.Run("delete subscription success returns 204", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
		ctx, err := round10NewLiftContext(http.MethodDelete, "/api/v1/push/subscription", map[string]string{
			"Authorization": "Bearer " + writeToken,
		}, nil, nil)
		require.NoError(t, err)

		require.NoError(t, h.HandleDeletePushSubscriptionLift(ctx))
		require.Equal(t, http.StatusNoContent, ctx.Response.StatusCode)
	})
}

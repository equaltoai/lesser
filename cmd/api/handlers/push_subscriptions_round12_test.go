package handlers

import (
	"context"
	"encoding/json"
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

		requireStatus(t, http.StatusUnauthorized)(h.HandleGetPushSubscriptionLift(ctx))
	})

	t.Run("get subscription invalid token", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/push/subscription", map[string]string{
			"Authorization": "Bearer invalid",
		}, nil, nil)
		require.NoError(t, err)

		requireStatus(t, http.StatusUnauthorized)(h.HandleGetPushSubscriptionLift(ctx))
	})

	t.Run("get subscription insufficient scope", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/push/subscription", map[string]string{
			"Authorization": "Bearer " + writeToken,
		}, nil, nil)
		require.NoError(t, err)

		requireStatus(t, http.StatusForbidden)(h.HandleGetPushSubscriptionLift(ctx))
	})

	t.Run("get subscription repo error", func(t *testing.T) {
		state := &round10QueryState{allErrorOnce: context.Canceled}
		h, _, _ := round11NewHandler(t, cfg, state)
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/push/subscription", map[string]string{
			"Authorization": "Bearer " + readToken,
		}, nil, nil)
		require.NoError(t, err)

		requireStatus(t, http.StatusInternalServerError)(h.HandleGetPushSubscriptionLift(ctx))
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

		resp := requireStatus(t, http.StatusOK)(h.HandleGetPushSubscriptionLift(ctx))
		var body map[string]any
		require.NoError(t, json.Unmarshal(resp.Body, &body))
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

		resp := requireStatus(t, http.StatusOK)(h.HandleGetPushSubscriptionLift(ctx))
		var body apimodels.PushSubscription
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		require.Equal(t, "", body.ServerKey)
	})

	t.Run("create subscription unauthorized", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/push/subscription", nil, nil, apimodels.PushSubscriptionRequest{})
		require.NoError(t, err)

		requireStatus(t, http.StatusUnauthorized)(h.HandleCreatePushSubscriptionLift(ctx))
	})

	t.Run("create subscription invalid token", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/push/subscription", map[string]string{
			"Authorization": "Bearer invalid",
		}, nil, apimodels.PushSubscriptionRequest{})
		require.NoError(t, err)

		requireStatus(t, http.StatusUnauthorized)(h.HandleCreatePushSubscriptionLift(ctx))
	})

	t.Run("create subscription insufficient scope", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/push/subscription", map[string]string{
			"Authorization": "Bearer " + readToken,
		}, nil, apimodels.PushSubscriptionRequest{})
		require.NoError(t, err)

		requireStatus(t, http.StatusForbidden)(h.HandleCreatePushSubscriptionLift(ctx))
	})

	t.Run("create subscription empty body returns bad request", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/push/subscription", map[string]string{
			"Authorization": "Bearer " + pushToken,
		}, nil, nil)
		require.NoError(t, err)

		requireStatus(t, http.StatusBadRequest)(h.HandleCreatePushSubscriptionLift(ctx))
	})

	t.Run("create subscription validates required fields", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/push/subscription", map[string]string{
			"Authorization": "Bearer " + writeToken,
		}, nil, apimodels.PushSubscriptionRequest{})
		require.NoError(t, err)

		requireStatus(t, http.StatusUnprocessableEntity)(h.HandleCreatePushSubscriptionLift(ctx))
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

		requireStatus(t, http.StatusUnprocessableEntity)(h.HandleCreatePushSubscriptionLift(ctx))
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

		requireStatus(t, http.StatusUnprocessableEntity)(h.HandleCreatePushSubscriptionLift(ctx))
	})

	t.Run("create subscription invalid body", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v1/push/subscription", map[string]string{
			"Authorization": "Bearer " + writeToken,
		}, nil, []byte("{"))

		requireStatus(t, http.StatusBadRequest)(h.HandleCreatePushSubscriptionLift(ctx))
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

		resp := requireStatus(t, http.StatusOK)(h.HandleCreatePushSubscriptionLift(ctx))
		var body apimodels.PushSubscription
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		require.Equal(t, "https://push.example.com", body.Endpoint)
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

		resp := requireStatus(t, http.StatusOK)(h.HandleCreatePushSubscriptionLift(ctx))
		var body apimodels.PushSubscription
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		require.Empty(t, body.ServerKey)
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

		resp := requireStatus(t, http.StatusOK)(h.HandleCreatePushSubscriptionLift(ctx))
		var body apimodels.PushSubscription
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		require.Equal(t, "pub", body.ServerKey)
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

		requireStatus(t, http.StatusInternalServerError)(h.HandleCreatePushSubscriptionLift(ctx))
	})

	t.Run("update subscription unauthorized", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
		ctx, err := round10NewLiftContext(http.MethodPut, "/api/v1/push/subscription", nil, nil, apimodels.PushSubscriptionRequest{})
		require.NoError(t, err)

		requireStatus(t, http.StatusUnauthorized)(h.HandleUpdatePushSubscriptionLift(ctx))
	})

	t.Run("update subscription invalid token", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
		ctx, err := round10NewLiftContext(http.MethodPut, "/api/v1/push/subscription", map[string]string{
			"Authorization": "Bearer invalid",
		}, nil, apimodels.PushSubscriptionRequest{})
		require.NoError(t, err)

		requireStatus(t, http.StatusUnauthorized)(h.HandleUpdatePushSubscriptionLift(ctx))
	})

	t.Run("update subscription insufficient scope", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
		ctx, err := round10NewLiftContext(http.MethodPut, "/api/v1/push/subscription", map[string]string{
			"Authorization": "Bearer " + readToken,
		}, nil, apimodels.PushSubscriptionRequest{})
		require.NoError(t, err)

		requireStatus(t, http.StatusForbidden)(h.HandleUpdatePushSubscriptionLift(ctx))
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

		requireStatus(t, http.StatusNotFound)(h.HandleUpdatePushSubscriptionLift(ctx))
	})

	t.Run("update subscription repo error treated as not found", func(t *testing.T) {
		state := &round10QueryState{allErrorOnce: context.Canceled}
		h, _, _ := round11NewHandler(t, cfg, state)
		ctx, err := round10NewLiftContext(http.MethodPut, "/api/v1/push/subscription", map[string]string{
			"Authorization": "Bearer " + writeToken,
		}, nil, apimodels.PushSubscriptionRequest{Data: apimodels.PushSubscriptionAlerts{Follow: true}})
		require.NoError(t, err)

		requireStatus(t, http.StatusNotFound)(h.HandleUpdatePushSubscriptionLift(ctx))
	})

	t.Run("update subscription invalid body", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
		ctx := round10NewLiftContextWithBodyBytes(http.MethodPut, "/api/v1/push/subscription", map[string]string{
			"Authorization": "Bearer " + writeToken,
		}, nil, []byte("{"))

		requireStatus(t, http.StatusBadRequest)(h.HandleUpdatePushSubscriptionLift(ctx))
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

		resp := requireStatus(t, http.StatusOK)(h.HandleUpdatePushSubscriptionLift(ctx))
		var body apimodels.PushSubscription
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		require.Empty(t, body.ServerKey)
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

		resp := requireStatus(t, http.StatusOK)(h.HandleUpdatePushSubscriptionLift(ctx))
		var body apimodels.PushSubscription
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		require.Equal(t, "pub", body.ServerKey)
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

		requireStatus(t, http.StatusInternalServerError)(h.HandleUpdatePushSubscriptionLift(ctx))
	})

	t.Run("delete subscription unauthorized", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
		ctx, err := round10NewLiftContext(http.MethodDelete, "/api/v1/push/subscription", nil, nil, nil)
		require.NoError(t, err)

		requireStatus(t, http.StatusUnauthorized)(h.HandleDeletePushSubscriptionLift(ctx))
	})

	t.Run("delete subscription invalid token", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
		ctx, err := round10NewLiftContext(http.MethodDelete, "/api/v1/push/subscription", map[string]string{
			"Authorization": "Bearer invalid",
		}, nil, nil)
		require.NoError(t, err)

		requireStatus(t, http.StatusUnauthorized)(h.HandleDeletePushSubscriptionLift(ctx))
	})

	t.Run("delete subscription insufficient scope", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
		ctx, err := round10NewLiftContext(http.MethodDelete, "/api/v1/push/subscription", map[string]string{
			"Authorization": "Bearer " + readToken,
		}, nil, nil)
		require.NoError(t, err)

		requireStatus(t, http.StatusForbidden)(h.HandleDeletePushSubscriptionLift(ctx))
	})

	t.Run("delete subscription delete failure", func(t *testing.T) {
		state := &round10QueryState{allErrorOnce: context.Canceled}
		h, _, _ := round11NewHandler(t, cfg, state)
		ctx, err := round10NewLiftContext(http.MethodDelete, "/api/v1/push/subscription", map[string]string{
			"Authorization": "Bearer " + writeToken,
		}, nil, nil)
		require.NoError(t, err)

		requireStatus(t, http.StatusInternalServerError)(h.HandleDeletePushSubscriptionLift(ctx))
	})

	t.Run("delete subscription success returns 204", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
		ctx, err := round10NewLiftContext(http.MethodDelete, "/api/v1/push/subscription", map[string]string{
			"Authorization": "Bearer " + writeToken,
		}, nil, nil)
		require.NoError(t, err)

		requireStatus(t, http.StatusNoContent)(h.HandleDeletePushSubscriptionLift(ctx))
	})
}

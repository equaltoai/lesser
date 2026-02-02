package handlers

import (
	"net/http"
	"testing"

	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/stretchr/testify/require"
)

func TestPushSubscriptionHandlers(t *testing.T) {
	h, _, _ := round11NewHandlerSliceC(t, nil)

	token := round11SignToken(t, h.cfg.JWTSecret, "alice", []string{"push", auth.ScopeRead, auth.ScopeWrite}, "sess-1")
	headers := map[string]string{"Authorization": "Bearer " + token}

	ctxGet, err := round10NewLiftContext(http.MethodGet, "/api/v1/push/subscription", headers, nil, nil)
	require.NoError(t, err)
	requireStatus(t, http.StatusOK)(h.HandleGetPushSubscriptionLift(ctxGet))

	body := map[string]any{
		"subscription": map[string]any{
			"endpoint": "https://push.example.com",
			"keys": map[string]string{
				"p256dh": "p256",
				"auth":   "auth",
			},
		},
		"data": map[string]bool{
			"follow": true,
		},
	}
	ctxCreate := round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v1/push/subscription", headers, nil, round11JSONBody(t, body))
	requireStatus(t, http.StatusOK)(h.HandleCreatePushSubscriptionLift(ctxCreate))
}

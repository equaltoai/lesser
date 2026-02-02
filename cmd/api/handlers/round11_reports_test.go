package handlers

import (
	"net/http"
	"testing"

	"github.com/equaltoai/lesser/pkg/auth"
)

func TestHandleCreateReportLift(t *testing.T) {
	h, _, _ := round11NewHandlerSliceC(t, nil)

	body := map[string]any{
		"account_id": "bob",
		"status_ids": []string{"status-1"},
		"comment":    "spam",
		"category":   "spam",
		"forward":    true,
		"rule_ids":   []int{1, 2},
	}

	token := round11SignToken(t, h.cfg.JWTSecret, "alice", []string{auth.ScopeWrite}, "sess-1")
	headers := map[string]string{"Authorization": "Bearer " + token}
	ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v1/reports", headers, nil, round11JSONBody(t, body))
	requireStatus(t, http.StatusOK)(h.HandleCreateReportLift(ctx))
}

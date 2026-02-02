package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/stretchr/testify/require"
)

func TestCreateReportLiftRound12(t *testing.T) {
	cfg := round11TestConfig()

	t.Run("unauthorized when missing Authorization header", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/reports", nil, nil, apimodels.CreateReportRequest{
			AccountID: "bob",
			Category:  "other",
		})
		require.NoError(t, err)

		requireStatus(t, http.StatusUnauthorized)(h.HandleCreateReportLift(ctx))
	})

	t.Run("insufficient scope returns forbidden", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
		token := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeRead})
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/reports", map[string]string{
			"Authorization": "Bearer " + token,
		}, nil, apimodels.CreateReportRequest{
			AccountID: "bob",
			Category:  "other",
		})
		require.NoError(t, err)

		requireStatus(t, http.StatusForbidden)(h.HandleCreateReportLift(ctx))
	})

	t.Run("unauthorized when Authorization header is malformed", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/reports", map[string]string{
			"Authorization": "not-a-bearer",
		}, nil, apimodels.CreateReportRequest{
			AccountID: "bob",
			Category:  "other",
		})
		require.NoError(t, err)

		requireStatus(t, http.StatusUnauthorized)(h.HandleCreateReportLift(ctx))
	})

	t.Run("unauthorized when access token is invalid", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/reports", map[string]string{
			"Authorization": "Bearer invalid",
		}, nil, apimodels.CreateReportRequest{
			AccountID: "bob",
			Category:  "other",
		})
		require.NoError(t, err)

		requireStatus(t, http.StatusUnauthorized)(h.HandleCreateReportLift(ctx))
	})

	t.Run("invalid JSON body returns bad request", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
		token := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeWrite})
		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v1/reports", map[string]string{
			"Authorization": "Bearer " + token,
		}, nil, []byte("{"))

		requireStatus(t, http.StatusBadRequest)(h.HandleCreateReportLift(ctx))
	})

	t.Run("create report storage failure returns internal error", func(t *testing.T) {
		state := &round10QueryState{createErrorOnce: context.Canceled}
		h, _, _ := round11NewHandler(t, cfg, state)
		token := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeWrite})
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/reports", map[string]string{
			"Authorization": "Bearer " + token,
		}, nil, apimodels.CreateReportRequest{
			AccountID: "bob",
			Category:  "other",
		})
		require.NoError(t, err)

		requireStatus(t, http.StatusInternalServerError)(h.HandleCreateReportLift(ctx))
	})

	t.Run("validation error returns bad request", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
		token := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeWrite})
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/reports", map[string]string{
			"Authorization": "Bearer " + token,
		}, nil, apimodels.CreateReportRequest{
			AccountID: "",
			Category:  "other",
		})
		require.NoError(t, err)

		requireStatus(t, http.StatusBadRequest)(h.HandleCreateReportLift(ctx))
	})

	t.Run("success creates report and returns response", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
		token := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeWrite})
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/reports", map[string]string{
			"Authorization": "Bearer " + token,
		}, nil, apimodels.CreateReportRequest{
			AccountID: "bob",
			StatusIDs: []string{"status-1"},
			Comment:   "report comment",
			Forward:   true,
			Category:  "spam",
			RuleIDs:   []int{1, 2},
		})
		require.NoError(t, err)

		resp := requireStatus(t, http.StatusOK)(h.HandleCreateReportLift(ctx))

		var body apimodels.Report
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		require.Equal(t, "spam", body.Category)
		require.Equal(t, []string{"status-1"}, body.StatusIDs)
		require.Equal(t, []int{1, 2}, body.RuleIDs)
		require.NotNil(t, body.TargetAccount)
	})
}

func TestReportHelpersRound12(t *testing.T) {
	cfg := round11TestConfig()

	t.Run("loadTargetAccountLift returns nil for empty target", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
		require.Nil(t, h.loadTargetAccountLift(context.Background(), ""))
	})

	t.Run("loadTargetAccountLift returns account for existing user", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
		account := h.loadTargetAccountLift(context.Background(), "bob")
		require.NotNil(t, account)
		require.Equal(t, "bob", account.Username)
	})

	t.Run("loadTargetAccountLift returns nil when account lookup fails", func(t *testing.T) {
		state := &round10QueryState{
			notFoundPKs: map[string]bool{
				"USER#bob": true,
			},
		}
		h, _, _ := round11NewHandler(t, cfg, state)
		require.Nil(t, h.loadTargetAccountLift(context.Background(), "bob"))
	})

	t.Run("createBasicModerationEventLift covers category and status mapping", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
		report := &storage.Report{
			ID:              "report-1",
			TargetAccountID: "bob",
			Category:        "spam",
			StatusIDs:       []string{"status-1"},
			Comment:         "report comment",
		}
		h.createBasicModerationEventLift(context.Background(), report, "https://example.com/users/alice")
	})

	t.Run("createBasicModerationEventLift logs when moderation event create fails", func(t *testing.T) {
		state := &round10QueryState{createErrorOnce: context.Canceled}
		h, _, _ := round11NewHandler(t, cfg, state)
		report := &storage.Report{
			ID:              "report-2",
			TargetAccountID: "bob",
			Category:        "violation",
			Comment:         "report comment",
		}
		h.createBasicModerationEventLift(context.Background(), report, "https://example.com/users/alice")
	})

	t.Run("convert helpers handle invalid entries", func(t *testing.T) {
		require.Equal(t, []string{"1", "2"}, convertIntArrayToStringArray([]int{1, 2}))
		require.Equal(t, []int{1, 2}, convertStringArrayToIntArray([]string{"1", "bad", "2"}))
	})
}

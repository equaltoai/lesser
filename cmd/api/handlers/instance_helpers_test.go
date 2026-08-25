package handlers

import (
	"context"
	"errors"
	"net/http"
	"testing"

	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
	apptheory "github.com/theory-cloud/apptheory/v4/runtime"
	"go.uber.org/zap"
)

func TestHandler_instanceLocked(t *testing.T) {
	ctx := context.Background()

	t.Run("nil_handler_returns_false", func(t *testing.T) {
		var h *Handler
		require.False(t, h.instanceLocked(ctx))
	})

	t.Run("returns_true_on_repo_error", func(t *testing.T) {
		state := &round10QueryState{firstErrorOnce: errors.New("boom")}
		h, _, _ := round11NewHandler(t, round11TestConfig(), state)
		require.True(t, h.instanceLocked(ctx))
	})

	t.Run("returns_locked_value", func(t *testing.T) {
		state := &round10QueryState{instanceState: &storagemodels.InstanceState{Locked: false}}
		h, _, _ := round11NewHandler(t, round11TestConfig(), state)
		require.False(t, h.instanceLocked(ctx))
	})
}

func TestHandler_instanceRulesAndExtendedDescription(t *testing.T) {
	ctx := context.Background()

	t.Run("returns_defaults_when_not_found", func(t *testing.T) {
		state := &round10QueryState{
			notFoundPKSK: map[string]bool{
				"INSTANCE#CONFIG#RULES":         true,
				"INSTANCE#CONFIG#EXTENDED_DESC": true,
			},
		}
		h, _, _ := round11NewHandler(t, round11TestConfig(), state)

		rules := h.instanceRules(ctx)
		require.NotEmpty(t, rules)

		desc := h.instanceExtendedDescription(ctx)
		require.NotEmpty(t, desc)
	})

	t.Run("returns_empty_on_repo_error", func(t *testing.T) {
		rulesState := &round10QueryState{firstErrorOnce: errors.New("boom")}
		h, _, _ := round11NewHandler(t, round11TestConfig(), rulesState)

		require.Empty(t, h.instanceRules(ctx))

		descState := &round10QueryState{firstErrorOnce: errors.New("boom")}
		h, _, _ = round11NewHandler(t, round11TestConfig(), descState)
		require.Empty(t, h.instanceExtendedDescription(ctx))
	})
}

func TestHandler_resolveVAPIDPublicKey(t *testing.T) {
	t.Run("production_missing_keys_returns_500", func(t *testing.T) {
		cfg := round11TestConfig()
		cfg.Stage = EnvProduction
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{forceVapidNotFound: true})

		liftCtx := &apptheory.Context{Request: apptheory.Request{Method: http.MethodGet, Path: "/"}}
		pub, resp, err := h.resolveVAPIDPublicKey(liftCtx, false)
		require.Empty(t, pub)
		require.NotNil(t, resp)
		require.NoError(t, err)
		require.Equal(t, http.StatusInternalServerError, resp.Status)
	})

	t.Run("live_missing_keys_returns_500", func(t *testing.T) {
		cfg := round11TestConfig()
		cfg.Stage = "live"
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{forceVapidNotFound: true})

		liftCtx := &apptheory.Context{Request: apptheory.Request{Method: http.MethodGet, Path: "/"}}
		pub, resp, err := h.resolveVAPIDPublicKey(liftCtx, false)
		require.Empty(t, pub)
		require.NotNil(t, resp)
		require.NoError(t, err)
		require.Equal(t, http.StatusInternalServerError, resp.Status)
	})

	t.Run("non_production_missing_keys_noops_when_generate_false", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, round11TestConfig(), &round10QueryState{forceVapidNotFound: true})

		liftCtx := &apptheory.Context{Request: apptheory.Request{Method: http.MethodGet, Path: "/"}}
		pub, resp, err := h.resolveVAPIDPublicKey(liftCtx, false)
		require.Empty(t, pub)
		require.Nil(t, resp)
		require.NoError(t, err)
	})

	t.Run("non_production_missing_keys_generates_when_generate_true", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, round11TestConfig(), &round10QueryState{forceVapidNotFound: true})

		liftCtx := &apptheory.Context{Request: apptheory.Request{Method: http.MethodGet, Path: "/"}}
		pub, resp, err := h.resolveVAPIDPublicKey(liftCtx, true)
		require.NotEmpty(t, pub)
		require.Nil(t, resp)
		require.NoError(t, err)
	})
}

func TestHandler_instanceCountsAndContactAccountAndTipsConfig(t *testing.T) {
	ctx := context.Background()

	t.Run("instance_counts_nil_handler_guard_returns_zeros", func(t *testing.T) {
		var h *Handler
		users, statuses, domains := h.instanceCounts(ctx)
		require.Zero(t, users)
		require.Zero(t, statuses)
		require.Zero(t, domains)
	})

	t.Run("instance_counts_error_read_is_not_cached", func(t *testing.T) {
		// The user-count read errors once (first InstanceMetrics First). The
		// failed compute must NOT be cached as zeros: a second call within the
		// TTL recomputes (and now succeeds, since the injected error was
		// one-shot) instead of serving pinned zeros.
		state := &round10QueryState{
			firstErrorByType: map[string]error{
				"*models.InstanceMetrics": errors.New("boom"),
			},
		}
		h, _, _ := round11NewHandler(t, round11TestConfig(), state)

		users, statuses, domains := h.instanceCounts(ctx)
		require.Equal(t, 0, users) // user read failed -> documented default 0
		require.Equal(t, int64(0), statuses)
		require.Equal(t, int64(0), domains)

		// Nothing was cached from the failed compute: the next call recomputes
		// and now succeeds (harness defaults: users 0, statuses 0, domains 1024).
		users2, statuses2, domains2 := h.instanceCounts(ctx)
		require.Equal(t, 0, users2)
		require.Equal(t, int64(0), statuses2)
		require.Equal(t, int64(1024), domains2)
		require.NotEqual(t, domains, domains2) // recomputed, not cached zeros
	})

	t.Run("instance_counts_handles_success_and_errors", func(t *testing.T) {
		state := &round10QueryState{
			instanceMetrics: map[string]storagemodels.InstanceMetrics{
				"INSTANCE#METRICS#TOTAL_STATUSES": {TotalStatuses: 7},
				"INSTANCE#METRICS#TOTAL_DOMAINS":  {Value: 3},
				"INSTANCE#METRICS#TOTAL_USERS":    {TotalUsers: 2},
			},
		}
		h, _, _ := round11NewHandler(t, round11TestConfig(), state)
		users, statuses, domains := h.instanceCounts(ctx)
		require.Equal(t, 2, users)
		require.Equal(t, int64(7), statuses)
		require.Equal(t, int64(3), domains)

		// Second call within the 60s TTL hits the success-only cache.
		users, statuses, domains = h.instanceCounts(ctx)
		require.Equal(t, 2, users)
		require.Equal(t, int64(7), statuses)
		require.Equal(t, int64(3), domains)

		errorState := &round10QueryState{
			allErrorOnce: errors.New("boom"),
			instanceMetrics: map[string]storagemodels.InstanceMetrics{
				"INSTANCE#METRICS#TOTAL_STATUSES": {TotalStatuses: 1},
				"INSTANCE#METRICS#TOTAL_DOMAINS":  {Value: 2},
			},
		}
		errHandler, _, _ := round11NewHandler(t, round11TestConfig(), errorState)
		users, statuses, domains = errHandler.instanceCounts(ctx)
		require.Equal(t, 0, users)
		require.Equal(t, int64(1), statuses)
		require.Equal(t, int64(2), domains)
	})

	t.Run("contact_account_returns_nil_when_missing", func(t *testing.T) {
		cfg := round11TestConfig()
		state := &round10QueryState{
			allErrorOnce: errors.New("boom"),
		}
		h, _, _ := round11NewHandler(t, cfg, state)
		require.Nil(t, h.instanceContactAccount(ctx))
	})

	t.Run("contact_account_maps_fields_when_present", func(t *testing.T) {
		cfg := round11TestConfig()
		cfg.Domain = "example.com"

		state := &round10QueryState{
			usersByUsername: map[string]storagemodels.User{
				"admin": {Username: "admin", Role: "admin", Approved: true, Version: 1},
			},
			actorsByUser: map[string]storagemodels.Actor{
				"admin": {Username: "admin"},
			},
		}
		h, _, _ := round11NewHandler(t, cfg, state)

		out := h.instanceContactAccount(ctx)
		require.NotNil(t, out)
		require.Equal(t, "admin", out["username"])
		require.Equal(t, "https://example.com/@admin", out["url"])
	})

	t.Run("tips_config_disables_when_enabled_missing_fields", func(t *testing.T) {
		cfg := round11TestConfig()
		cfg.TipEnabled = true
		cfg.TipChainID = 0
		cfg.TipContractAddress = ""

		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
		h.logger = zap.NewNop()

		out := h.instanceTipsConfig(ctx)
		require.Equal(t, false, out["enabled"])
		_, hasChain := out["chain_id"]
		require.False(t, hasChain)
	})

	t.Run("tips_config_returns_chain_and_contract_when_enabled", func(t *testing.T) {
		cfg := round11TestConfig()
		cfg.TipEnabled = true
		cfg.TipChainID = 10
		cfg.TipContractAddress = " 0xabc "

		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
		h.logger = zap.NewNop()

		out := h.instanceTipsConfig(ctx)
		require.Equal(t, true, out["enabled"])
		require.Equal(t, 10, out["chain_id"])
		require.Equal(t, "0xabc", out["contract_address"])
	})
}

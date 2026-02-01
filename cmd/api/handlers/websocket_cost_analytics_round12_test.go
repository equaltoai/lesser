package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/stretchr/testify/require"
)

func TestWebSocketCostAnalytics_TrendAnalysis_Round12(t *testing.T) {
	cfg := round10TestConfig()
	logger := round10TestLogger(t)
	h := &Handler{cfg: cfg, logger: logger}

	start := time.Now().Add(-24 * time.Hour).Truncate(time.Hour)

	makePoints := func(costs []float64) []WebSocketCostDataPoint {
		points := make([]WebSocketCostDataPoint, 0, len(costs))
		for i, cost := range costs {
			points = append(points, WebSocketCostDataPoint{
				Timestamp:   start.Add(time.Duration(i) * time.Hour),
				CostDollars: cost,
				Connections: 1,
				Messages:    1,
				UniqueUsers: 1,
			})
		}
		return points
	}

	t.Run("increasing costs", func(t *testing.T) {
		analysis := h.analyzeWebSocketTrends(makePoints([]float64{1, 2, 3, 4, 5, 6, 7}))
		require.Equal(t, trendIncreasing, analysis.TrendDirection)
		require.Greater(t, analysis.GrowthRate, 0.0)
		require.NotEmpty(t, analysis.WeeklyPattern)
	})

	t.Run("decreasing costs", func(t *testing.T) {
		analysis := h.analyzeWebSocketTrends(makePoints([]float64{7, 6, 5, 4, 3, 2, 1}))
		require.Equal(t, trendDecreasing, analysis.TrendDirection)
		require.Less(t, analysis.GrowthRate, 0.0)
		require.NotEmpty(t, analysis.WeeklyPattern)
	})

	t.Run("stable costs", func(t *testing.T) {
		analysis := h.analyzeWebSocketTrends(makePoints([]float64{5, 5, 5, 5, 5, 5, 5}))
		require.Equal(t, "stable", analysis.TrendDirection)
	})
}

func TestWebSocketCostAnalytics_ValidationsAndEmptySummary_Round12(t *testing.T) {
	cfg := round10TestConfig()
	logger := round10TestLogger(t)
	h := &Handler{cfg: cfg, logger: logger}

	ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/admin/ws-costs", nil, nil, nil)
	require.NoError(t, err)

	_, err = h.GetWebSocketCostAnalytics(ctx, WebSocketCostAnalyticsRequest{StartDate: "bad", EndDate: "2025-01-01"})
	require.Error(t, err)

	_, err = h.GetWebSocketCostAnalytics(ctx, WebSocketCostAnalyticsRequest{StartDate: "2025-01-01", EndDate: "bad"})
	require.Error(t, err)

	_, err = h.GetWebSocketCostAnalytics(ctx, WebSocketCostAnalyticsRequest{
		StartDate: "2025-01-02",
		EndDate:   "2025-01-01",
	})
	require.Error(t, err)

	t.Run("overall summary returns empty when no costs in range", func(t *testing.T) {
		start := time.Now().Add(-48 * time.Hour).Truncate(24 * time.Hour)
		end := start.Add(24 * time.Hour)

		fakeRepo := &fakeWebSocketCostRepo{
			recentCosts: []*models.WebSocketCostRecord{
				{Timestamp: end.Add(1 * time.Hour), EstimatedCostDollars: 1.0, ConnectionID: "c1"},
			},
		}

		summary, err := h.buildWebSocketOverallSummary(fakeRepo, start, end)
		require.NoError(t, err)
		require.Equal(t, 0.0, summary.TotalCostDollars)
		require.Equal(t, start.Format(common.DateFormat)+" to "+end.Format(common.DateFormat), summary.DateRange)
	})
}

type webSocketCostRepoWithErrors struct {
	*fakeWebSocketCostRepo

	getUserBudgetsErr     error
	checkBudgetLimitsErr  error
	updateBudgetErr       error
	createBudgetErr       error
	getRecentCostsErr     error
	getTopCostlyUsersErr  error
	getHighCostOpsErr     error
	getUserCostSummaryErr error
}

func (r *webSocketCostRepoWithErrors) GetRecentCosts(ctx context.Context, startTime time.Time, limit int) ([]*models.WebSocketCostRecord, error) {
	if r.getRecentCostsErr != nil {
		return nil, r.getRecentCostsErr
	}
	return r.fakeWebSocketCostRepo.GetRecentCosts(ctx, startTime, limit)
}

func (r *webSocketCostRepoWithErrors) GetTopCostlyUsers(ctx context.Context, startDate, endDate time.Time, limit int) ([]*repositories.WebSocketUserCostRanking, error) {
	if r.getTopCostlyUsersErr != nil {
		return nil, r.getTopCostlyUsersErr
	}
	return r.fakeWebSocketCostRepo.GetTopCostlyUsers(ctx, startDate, endDate, limit)
}

func (r *webSocketCostRepoWithErrors) GetHighCostOperations(ctx context.Context, thresholdDollars float64, startTime, endTime time.Time, limit int) ([]*models.WebSocketCostRecord, error) {
	if r.getHighCostOpsErr != nil {
		return nil, r.getHighCostOpsErr
	}
	return r.fakeWebSocketCostRepo.GetHighCostOperations(ctx, thresholdDollars, startTime, endTime, limit)
}

func (r *webSocketCostRepoWithErrors) GetUserCostSummary(ctx context.Context, userID string, startTime, endTime time.Time) (*repositories.WebSocketUserCostSummary, error) {
	if r.getUserCostSummaryErr != nil {
		return nil, r.getUserCostSummaryErr
	}
	return r.fakeWebSocketCostRepo.GetUserCostSummary(ctx, userID, startTime, endTime)
}

func (r *webSocketCostRepoWithErrors) CheckBudgetLimits(ctx context.Context, userID string) (*repositories.BudgetStatus, error) {
	if r.checkBudgetLimitsErr != nil {
		return nil, r.checkBudgetLimitsErr
	}
	return r.fakeWebSocketCostRepo.CheckBudgetLimits(ctx, userID)
}

func (r *webSocketCostRepoWithErrors) GetUserBudgets(ctx context.Context, userID string) ([]*models.WebSocketCostBudget, error) {
	if r.getUserBudgetsErr != nil {
		return nil, r.getUserBudgetsErr
	}
	return r.fakeWebSocketCostRepo.GetUserBudgets(ctx, userID)
}

func (r *webSocketCostRepoWithErrors) UpdateBudget(ctx context.Context, budget *models.WebSocketCostBudget) error {
	if r.updateBudgetErr != nil {
		return r.updateBudgetErr
	}
	return r.fakeWebSocketCostRepo.UpdateBudget(ctx, budget)
}

func (r *webSocketCostRepoWithErrors) CreateBudget(ctx context.Context, budget *models.WebSocketCostBudget) error {
	if r.createBudgetErr != nil {
		return r.createBudgetErr
	}
	return r.fakeWebSocketCostRepo.CreateBudget(ctx, budget)
}

func TestWebSocketBudgetHandlers_ErrorBranches_Round12(t *testing.T) {
	cfg := round11TestConfig()
	logger := round10TestLogger(t)
	handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
	handler.logger = logger

	restore := webSocketCostRepoProvider
	defer func() { webSocketCostRepoProvider = restore }()

	t.Run("GetUserWebSocketBudget missing user_id", func(t *testing.T) {
		webSocketCostRepoProvider = func(h *Handler) webSocketCostRepository { return &fakeWebSocketCostRepo{} }

		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/admin/ws-costs/users//budget", nil, nil, nil)
		require.NoError(t, err)

		_, err = handler.GetUserWebSocketBudget(ctx)
		require.Error(t, err)
	})

	t.Run("GetUserWebSocketBudget budgets error", func(t *testing.T) {
		webSocketCostRepoProvider = func(h *Handler) webSocketCostRepository {
			return &webSocketCostRepoWithErrors{
				fakeWebSocketCostRepo: &fakeWebSocketCostRepo{},
				getUserBudgetsErr:     errors.New("boom"),
			}
		}

		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/admin/ws-costs/users/alice/budget", nil, nil, nil)
		require.NoError(t, err)
		ctx.Params["user_id"] = "alice"

		_, err = handler.GetUserWebSocketBudget(ctx)
		require.Error(t, err)
	})

	t.Run("GetUserWebSocketBudget budget status error", func(t *testing.T) {
		webSocketCostRepoProvider = func(h *Handler) webSocketCostRepository {
			return &webSocketCostRepoWithErrors{
				fakeWebSocketCostRepo: &fakeWebSocketCostRepo{budgets: []*models.WebSocketCostBudget{{UserID: "alice", Period: "daily", BudgetMicroCents: 100}}},
				checkBudgetLimitsErr:  errors.New("boom"),
			}
		}

		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/admin/ws-costs/users/alice/budget", nil, nil, nil)
		require.NoError(t, err)
		ctx.Params["user_id"] = "alice"

		_, err = handler.GetUserWebSocketBudget(ctx)
		require.Error(t, err)
	})

	t.Run("CreateUserWebSocketBudget missing body and invalid json", func(t *testing.T) {
		webSocketCostRepoProvider = func(h *Handler) webSocketCostRepository { return &fakeWebSocketCostRepo{} }

		ctxMissingBody, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/ws-costs/users/alice/budget", nil, nil, nil)
		require.NoError(t, err)
		ctxMissingBody.Params["user_id"] = "alice"

		_, err = handler.CreateUserWebSocketBudget(ctxMissingBody)
		require.Error(t, err)

		ctxBadJSON, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/ws-costs/users/alice/budget", nil, nil, nil)
		require.NoError(t, err)
		ctxBadJSON.Params["user_id"] = "alice"
		ctxBadJSON.Request.Body = []byte("{invalid")

		_, err = handler.CreateUserWebSocketBudget(ctxBadJSON)
		require.Error(t, err)
	})

	t.Run("CreateUserWebSocketBudget update/create failures", func(t *testing.T) {
		budgetBody := map[string]any{"period": "daily", "budget_dollars": 1.0}
		bodyBytes, err := json.Marshal(budgetBody)
		require.NoError(t, err)

		webSocketCostRepoProvider = func(h *Handler) webSocketCostRepository {
			return &webSocketCostRepoWithErrors{
				fakeWebSocketCostRepo: &fakeWebSocketCostRepo{existingBudget: &models.WebSocketCostBudget{UserID: "alice", Period: "daily"}},
				updateBudgetErr:       errors.New("boom"),
			}
		}

		ctxUpdate, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/ws-costs/users/alice/budget", nil, nil, nil)
		require.NoError(t, err)
		ctxUpdate.Params["user_id"] = "alice"
		ctxUpdate.Request.Body = bodyBytes

		_, err = handler.CreateUserWebSocketBudget(ctxUpdate)
		require.Error(t, err)

		webSocketCostRepoProvider = func(h *Handler) webSocketCostRepository {
			return &webSocketCostRepoWithErrors{
				fakeWebSocketCostRepo: &fakeWebSocketCostRepo{},
				createBudgetErr:       errors.New("boom"),
			}
		}

		ctxCreate, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/ws-costs/users/alice/budget", nil, nil, nil)
		require.NoError(t, err)
		ctxCreate.Params["user_id"] = "alice"
		ctxCreate.Request.Body = bodyBytes

		_, err = handler.CreateUserWebSocketBudget(ctxCreate)
		require.Error(t, err)
	})

	t.Run("GetWebSocketCostAnalytics repository errors", func(t *testing.T) {
		start := time.Now().Add(-48 * time.Hour).Truncate(24 * time.Hour)
		end := start.Add(24 * time.Hour)

		webSocketCostRepoProvider = func(h *Handler) webSocketCostRepository {
			return &webSocketCostRepoWithErrors{
				fakeWebSocketCostRepo: &fakeWebSocketCostRepo{},
				getRecentCostsErr:     errors.New("boom"),
			}
		}

		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/admin/ws-costs", nil, nil, nil)
		require.NoError(t, err)

		_, err = handler.GetWebSocketCostAnalytics(ctx, WebSocketCostAnalyticsRequest{
			StartDate: start.Format(common.DateFormat),
			EndDate:   end.Format(common.DateFormat),
			Period:    "hour",
		})
		require.Error(t, err)
	})
}

func TestWebSocketCostAnalytics_MiscBranches_Round12(t *testing.T) {
	cfg := round10TestConfig()
	logger := round10TestLogger(t)
	h := &Handler{cfg: cfg, logger: logger}

	points := []WebSocketCostDataPoint{
		{Timestamp: time.Now(), CostDollars: 1},
		{Timestamp: time.Now().Add(1 * time.Hour), CostDollars: 2},
	}

	require.Nil(t, h.calculateMovingAverage(points, 3))
	require.Equal(t, 0.0, h.calculateExponentialSmoothingGrowthRate(points))
	require.Equal(t, 0.0, h.addStatisticalSignificance(&WebSocketTrendAnalysis{TrendDirection: "stable"}, points).GrowthRate)

	t.Run("CreateUserWebSocketBudget missing user_id", func(t *testing.T) {
		restore := webSocketCostRepoProvider
		webSocketCostRepoProvider = func(h *Handler) webSocketCostRepository { return &fakeWebSocketCostRepo{} }
		defer func() { webSocketCostRepoProvider = restore }()

		handler, _, _ := round11NewHandler(t, round11TestConfig(), &round10QueryState{})
		handler.logger = logger

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/ws-costs/users//budget", nil, nil, nil)
		require.NoError(t, err)
		ctx.Request.Body = []byte(`{"period":"daily","budget_dollars":1}`)

		_, err = handler.CreateUserWebSocketBudget(ctx)
		require.Error(t, err)
	})

	t.Run("CreateUserWebSocketBudget weekly period", func(t *testing.T) {
		restore := webSocketCostRepoProvider
		webSocketCostRepoProvider = func(h *Handler) webSocketCostRepository { return &fakeWebSocketCostRepo{} }
		defer func() { webSocketCostRepoProvider = restore }()

		handler, _, _ := round11NewHandler(t, round11TestConfig(), &round10QueryState{})
		handler.logger = logger

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/ws-costs/users/alice/budget", nil, nil, nil)
		require.NoError(t, err)
		ctx.Params["user_id"] = "alice"
		ctx.Request.Body = []byte(`{"period":"weekly","budget_dollars":1}`)

		_, err = handler.CreateUserWebSocketBudget(ctx)
		require.NoError(t, err)
	})
}

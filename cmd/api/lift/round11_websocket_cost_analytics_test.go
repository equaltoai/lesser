package lift

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/stretchr/testify/require"
)

type fakeWebSocketCostRepo struct {
	recentCosts []*models.WebSocketCostRecord
	topUsers    []*repositories.WebSocketUserCostRanking
	highCostOps []*models.WebSocketCostRecord
	userSummary *repositories.WebSocketUserCostSummary
	budget      *repositories.BudgetStatus
	budgets     []*models.WebSocketCostBudget

	existingBudget *models.WebSocketCostBudget
	updatedBudget  *models.WebSocketCostBudget
	createdBudget  *models.WebSocketCostBudget
}

func (f *fakeWebSocketCostRepo) GetRecentCosts(ctx context.Context, startTime time.Time, limit int) ([]*models.WebSocketCostRecord, error) {
	return f.recentCosts, nil
}

func (f *fakeWebSocketCostRepo) GetTopCostlyUsers(ctx context.Context, startDate, endDate time.Time, limit int) ([]*repositories.WebSocketUserCostRanking, error) {
	return f.topUsers, nil
}

func (f *fakeWebSocketCostRepo) GetHighCostOperations(ctx context.Context, thresholdDollars float64, startTime, endTime time.Time, limit int) ([]*models.WebSocketCostRecord, error) {
	return f.highCostOps, nil
}

func (f *fakeWebSocketCostRepo) GetUserCostSummary(ctx context.Context, userID string, startTime, endTime time.Time) (*repositories.WebSocketUserCostSummary, error) {
	return f.userSummary, nil
}

func (f *fakeWebSocketCostRepo) CheckBudgetLimits(ctx context.Context, userID string) (*repositories.BudgetStatus, error) {
	return f.budget, nil
}

func (f *fakeWebSocketCostRepo) GetUserBudgets(ctx context.Context, userID string) ([]*models.WebSocketCostBudget, error) {
	return f.budgets, nil
}

func (f *fakeWebSocketCostRepo) GetBudget(ctx context.Context, userID, period string) (*models.WebSocketCostBudget, error) {
	return f.existingBudget, nil
}

func (f *fakeWebSocketCostRepo) UpdateBudget(ctx context.Context, budget *models.WebSocketCostBudget) error {
	f.updatedBudget = budget
	return nil
}

func (f *fakeWebSocketCostRepo) CreateBudget(ctx context.Context, budget *models.WebSocketCostBudget) error {
	f.createdBudget = budget
	return nil
}

func TestWebSocketCostAnalyticsSummaryAndBudget(t *testing.T) {
	cfg := round10TestConfig()
	logger := round10TestLogger(t)

	start := time.Now().Add(-48 * time.Hour).Truncate(24 * time.Hour)
	end := start.Add(24 * time.Hour)

	recentCosts := []*models.WebSocketCostRecord{
		{
			Timestamp:                start.Add(2 * time.Hour),
			EstimatedCostDollars:     1.0,
			UserID:                   "user-1",
			ConnectionID:             "conn-1",
			OperationType:            "connect",
			ConnectionDurationMs:     120000,
			MessageCount:             0,
			APIGatewayConnectionCost: 100,
			LambdaExecutionCost:      20,
		},
		{
			Timestamp:             start.Add(4 * time.Hour),
			EstimatedCostDollars:  0.5,
			UserID:                "user-2",
			ConnectionID:          "conn-2",
			OperationType:         "message",
			MessageCount:          3,
			APIGatewayMessageCost: 50,
			DynamoDBCost:          10,
			ResponseLatencyMs:     250,
		},
	}

	fakeRepo := &fakeWebSocketCostRepo{
		recentCosts: recentCosts,
		topUsers: []*repositories.WebSocketUserCostRanking{
			{UserID: "user-1", Username: "alice", TotalCostMicroCents: 1000000},
		},
		highCostOps: recentCosts,
		userSummary: &repositories.WebSocketUserCostSummary{UserID: "user-1", TotalCostDollars: 1.5},
		budget:      &repositories.BudgetStatus{AllowConnection: true, AllowMessages: true},
		budgets: []*models.WebSocketCostBudget{
			{UserID: "user-1", Period: "daily", BudgetMicroCents: 1000000},
		},
	}

	restore := webSocketCostRepoProvider
	webSocketCostRepoProvider = func(h *Handler) webSocketCostRepository {
		return fakeRepo
	}
	defer func() { webSocketCostRepoProvider = restore }()

	h := &Handler{cfg: cfg, logger: logger}
	ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/admin/ws-costs", nil, nil, nil)
	require.NoError(t, err)

	resp, err := h.GetWebSocketCostAnalytics(ctx, WebSocketCostAnalyticsRequest{
		StartDate: start.Format(common.DateFormat),
		EndDate:   end.Format(common.DateFormat),
		Period:    "day",
		UserID:    "user-1",
		Limit:     5,
	})
	require.NoError(t, err)
	require.NotNil(t, resp.Summary)
	require.NotEmpty(t, resp.TopUsers)
	require.NotEmpty(t, resp.HighCostOperations)
	require.NotNil(t, resp.CostTrends)
	require.NotNil(t, resp.UserDetails)
	require.NotNil(t, resp.BudgetStatus)
}

func TestWebSocketBudgetHandlers(t *testing.T) {
	cfg := round10TestConfig()
	logger := round10TestLogger(t)

	state := &round10QueryState{
		usersByUsername: map[string]models.User{
			"alice": {PK: "USER#alice", SK: models.SKMetadata, Username: "alice", Role: "user", Approved: true, Version: 1},
		},
	}
	harness := round10NewDynamoHarness(t, state)
	accountRepo := repositories.NewAccountRepository(harness.db, cfg.DynamoTableName, cfg.Domain, logger)

	repos := &MockRepositoryStorage{}
	repos.On("Account").Return(accountRepo).Maybe()
	repos.On("Audit").Return(nil).Maybe()

	fakeRepo := &fakeWebSocketCostRepo{
		budget: &repositories.BudgetStatus{AllowConnection: true, AllowMessages: true},
		budgets: []*models.WebSocketCostBudget{
			{UserID: "alice", Period: "daily", BudgetMicroCents: 1000000},
		},
	}

	restore := webSocketCostRepoProvider
	webSocketCostRepoProvider = func(h *Handler) webSocketCostRepository {
		return fakeRepo
	}
	defer func() { webSocketCostRepoProvider = restore }()

	h := &Handler{cfg: cfg, logger: logger, repos: repos}

	ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/admin/ws-costs/users/alice/budget", nil, nil, nil)
	require.NoError(t, err)
	ctx.SetParam("user_id", "alice")

	resp, err := h.GetUserWebSocketBudget(ctx)
	require.NoError(t, err)
	require.NotNil(t, resp)

	body := map[string]any{
		"period":         "daily",
		"budget_dollars": 1.5,
	}
	ctx2, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/ws-costs/users/alice/budget", nil, nil, body)
	require.NoError(t, err)
	ctx2.SetParam("user_id", "alice")
	ctx2.Request.Body, _ = json.Marshal(body)

	_, err = h.CreateUserWebSocketBudget(ctx2)
	require.NoError(t, err)
	require.NotNil(t, fakeRepo.createdBudget)

	fakeRepo.existingBudget = &models.WebSocketCostBudget{UserID: "alice", Period: "daily"}
	ctx3, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/ws-costs/users/alice/budget", nil, nil, body)
	require.NoError(t, err)
	ctx3.SetParam("user_id", "alice")
	ctx3.Request.Body, _ = json.Marshal(body)
	_, err = h.CreateUserWebSocketBudget(ctx3)
	require.NoError(t, err)
	require.NotNil(t, fakeRepo.updatedBudget)
}

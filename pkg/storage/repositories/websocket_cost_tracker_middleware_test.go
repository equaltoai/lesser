package repositories

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/dynamorm/pkg/mocks"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	apptheory "github.com/theory-cloud/apptheory/runtime"
	"go.uber.org/zap"
)

func TestWebSocketCostTracker_CreateOperationContext(t *testing.T) {
	tracker := &WebSocketCostTracker{}

	var opCtx *WebSocketOperationContext

	wsEvent := events.APIGatewayWebsocketProxyRequest{
		RequestContext: events.APIGatewayWebsocketProxyRequestContext{
			ConnectionID: "conn-1",
			RouteKey:     "$connect",
			RequestID:    "req-1",
		},
		Headers: map[string]string{
			"user-agent":      "iPhone mobile",
			"Authorization":   "Bearer token",
			"x-forwarded-for": "1.2.3.4, 9.9.9.9",
		},
		QueryStringParameters: map[string]string{
			"access_token": "token",
		},
	}

	app := apptheory.New()
	app.WebSocket("$connect", func(ctx *apptheory.Context) (*apptheory.Response, error) {
		ctx.Set("user_id", "user-1")
		ctx.Set("username", "alice")
		opCtx = tracker.CreateOperationContext(ctx, WSEventConnect)
		return apptheory.Text(200, ""), nil
	})

	resp := app.ServeWebSocket(context.Background(), wsEvent)
	require.Equal(t, 200, resp.StatusCode)
	require.NotNil(t, opCtx)

	require.Equal(t, "conn-1", opCtx.ConnectionID)
	require.Equal(t, WSEventConnect, opCtx.OperationType)
	require.Equal(t, "req-1", opCtx.RequestID)
	require.Equal(t, "1.2.3.4", opCtx.ClientIP)
	require.Equal(t, "iPhone mobile", opCtx.UserAgent)
	require.Equal(t, "mobile", opCtx.ConnectionSource)
	require.Equal(t, "oauth", opCtx.AuthMethod)
	require.Equal(t, "user-1", opCtx.UserID)
	require.Equal(t, "alice", opCtx.Username)
	require.WithinDuration(t, time.Now(), opCtx.StartTime, time.Second)

	t.Run("ignores non-string user context values", func(t *testing.T) {
		var opCtx *WebSocketOperationContext
		wsEvent := events.APIGatewayWebsocketProxyRequest{
			RequestContext: events.APIGatewayWebsocketProxyRequestContext{
				ConnectionID: "conn-2",
				RouteKey:     "$connect",
				RequestID:    "req-2",
			},
		}

		app := apptheory.New()
		app.WebSocket("$connect", func(ctx *apptheory.Context) (*apptheory.Response, error) {
			ctx.Set("user_id", 123)
			ctx.Set("username", 456)
			opCtx = tracker.CreateOperationContext(ctx, WSEventConnect)
			return apptheory.Text(200, ""), nil
		})

		resp := app.ServeWebSocket(context.Background(), wsEvent)
		require.Equal(t, 200, resp.StatusCode)
		require.NotNil(t, opCtx)

		require.Empty(t, opCtx.UserID)
		require.Empty(t, opCtx.Username)
	})
}

func TestWebSocketCostMiddleware(t *testing.T) {
	logger := zap.NewNop()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Create").Return(nil).Once()

	repo := &WebSocketCostRepository{
		EnhancedBaseRepository: &EnhancedBaseRepository[*models.WebSocketCostRecord]{
			BaseRepository: &BaseRepository[*models.WebSocketCostRecord]{
				db:        mockDB,
				tableName: "test-table",
				logger:    logger,
			},
		},
	}

	tracker := &WebSocketCostTracker{
		costRepo:     repo,
		logger:       logger,
		serviceName:  "test-service",
		functionName: "test-function",
	}

	wsEvent := events.APIGatewayWebsocketProxyRequest{
		RequestContext: events.APIGatewayWebsocketProxyRequestContext{
			ConnectionID: "conn-1",
			RouteKey:     "$disconnect",
			RequestID:    "req-1",
		},
	}

	app := apptheory.New()
	app.Use(WebSocketCostMiddleware(tracker))
	app.WebSocket("$disconnect", func(_ *apptheory.Context) (*apptheory.Response, error) {
		return apptheory.Text(200, ""), nil
	})

	resp := app.ServeWebSocket(context.Background(), wsEvent)
	require.Equal(t, 200, resp.StatusCode)

	mockQuery.AssertExpectations(t)
}

func TestCheckBudgetIfRequired_BudgetBranches(t *testing.T) {
	logger := zap.NewNop()

	buildTracker := func(t *testing.T, allFn func(*[]*models.WebSocketCostBudget)) *WebSocketCostTracker {
		t.Helper()

		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Index", mock.Anything).Return(mockQuery)
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
		mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery)
		mockQuery.On("Limit", mock.Anything).Return(mockQuery).Maybe()
		mockQuery.On("All", mock.AnythingOfType("*[]*models.WebSocketCostBudget")).Run(func(args mock.Arguments) {
			allFn(args.Get(0).(*[]*models.WebSocketCostBudget))
		}).Return(nil)

		repo := &WebSocketCostRepository{
			EnhancedBaseRepository: &EnhancedBaseRepository[*models.WebSocketCostRecord]{
				BaseRepository: &BaseRepository[*models.WebSocketCostRecord]{
					db:        mockDB,
					tableName: "test-table",
					logger:    logger,
				},
			},
			budgetRepo: &EnhancedBaseRepository[*models.WebSocketCostBudget]{
				BaseRepository: &BaseRepository[*models.WebSocketCostBudget]{
					db:        mockDB,
					tableName: "test-table",
					logger:    logger,
				},
			},
		}

		return &WebSocketCostTracker{
			costRepo: repo,
			logger:   logger,
		}
	}

	t.Run("budget lookup error is swallowed", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Index", mock.Anything).Return(mockQuery)
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
		mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery)
		mockQuery.On("All", mock.AnythingOfType("*[]*models.WebSocketCostBudget")).Return(errors.New("db error"))

		repo := &WebSocketCostRepository{
			EnhancedBaseRepository: &EnhancedBaseRepository[*models.WebSocketCostRecord]{
				BaseRepository: &BaseRepository[*models.WebSocketCostRecord]{
					db:        mockDB,
					tableName: "test-table",
					logger:    logger,
				},
			},
			budgetRepo: &EnhancedBaseRepository[*models.WebSocketCostBudget]{
				BaseRepository: &BaseRepository[*models.WebSocketCostBudget]{
					db:        mockDB,
					tableName: "test-table",
					logger:    logger,
				},
			},
		}

		tracker := &WebSocketCostTracker{costRepo: repo, logger: logger}
		opCtx := &WebSocketOperationContext{UserID: "user-1"}

		require.NoError(t, checkBudgetIfRequired(tracker, context.Background(), "connect", opCtx))
	})

	t.Run("budget exceeded returns app error", func(t *testing.T) {
		tracker := buildTracker(t, func(dest *[]*models.WebSocketCostBudget) {
			now := time.Now()
			*dest = []*models.WebSocketCostBudget{
				{
					UserID:      "user-1",
					Period:      "daily",
					Status:      "exceeded",
					WindowStart: now.Add(-time.Hour),
					WindowEnd:   now.Add(time.Hour),
				},
			}
		})

		opCtx := &WebSocketOperationContext{UserID: "user-1"}

		err := checkBudgetIfRequired(tracker, context.Background(), "connect", opCtx)
		require.Error(t, err)
	})

	t.Run("allow connection returns nil", func(t *testing.T) {
		tracker := buildTracker(t, func(dest *[]*models.WebSocketCostBudget) {
			*dest = []*models.WebSocketCostBudget{}
		})

		opCtx := &WebSocketOperationContext{UserID: "user-1"}

		require.NoError(t, checkBudgetIfRequired(tracker, context.Background(), "connect", opCtx))
	})
}

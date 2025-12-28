package repositories

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/dynamorm/pkg/mocks"
	"github.com/pay-theory/lift/pkg/lift"
	"github.com/pay-theory/lift/pkg/lift/adapters"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestWebSocketCostTracker_CreateOperationContext(t *testing.T) {
	tracker := &WebSocketCostTracker{}

	wsEvent := events.APIGatewayWebsocketProxyRequest{
		RequestContext: events.APIGatewayWebsocketProxyRequestContext{
			ConnectionID: "conn-1",
			RouteKey:     "$connect",
			Identity: events.APIGatewayRequestIdentity{
				SourceIP: "1.2.3.4",
			},
		},
		Headers: map[string]string{
			"user-agent":      "iPhone mobile",
			"Authorization":   "Bearer token",
			"x-forwarded-for": "9.9.9.9",
		},
		QueryStringParameters: map[string]string{
			"access_token": "token",
		},
	}

	ctx := &lift.Context{
		Request: lift.NewRequestWithContext(context.Background(), &adapters.Request{}),
	}
	ctx.SetRequestID("req-1")
	ctx.Set("user_id", "user-1")
	ctx.Set("username", "alice")

	opCtx := tracker.CreateOperationContext(ctx, wsEvent, "connect")
	require.Equal(t, "conn-1", opCtx.ConnectionID)
	require.Equal(t, "connect", opCtx.OperationType)
	require.Equal(t, "req-1", opCtx.RequestID)
	require.Equal(t, "1.2.3.4", opCtx.ClientIP)
	require.Equal(t, "iPhone mobile", opCtx.UserAgent)
	require.Equal(t, "mobile", opCtx.ConnectionSource)
	require.Equal(t, "oauth", opCtx.AuthMethod)
	require.Equal(t, "user-1", opCtx.UserID)
	require.Equal(t, "alice", opCtx.Username)
	require.WithinDuration(t, time.Now(), opCtx.StartTime, time.Second)

	t.Run("ignores non-string user context values", func(t *testing.T) {
		ctx := &lift.Context{
			Request: lift.NewRequestWithContext(context.Background(), &adapters.Request{}),
		}
		ctx.SetRequestID("req-2")
		ctx.Set("user_id", 123)
		ctx.Set("username", 456)

		opCtx := tracker.CreateOperationContext(ctx, wsEvent, "connect")
		require.Empty(t, opCtx.UserID)
		require.Empty(t, opCtx.Username)
	})
}

func TestExtractWebSocketEvent(t *testing.T) {
	t.Run("returns raw event when type matches", func(t *testing.T) {
		wsEvent := events.APIGatewayWebsocketProxyRequest{
			RequestContext: events.APIGatewayWebsocketProxyRequestContext{
				ConnectionID: "conn-raw",
				RouteKey:     "$disconnect",
			},
		}

		ctx := &lift.Context{
			Request: lift.NewRequestWithContext(context.Background(), &adapters.Request{
				RawEvent: wsEvent,
			}),
		}

		got := extractWebSocketEvent(ctx)
		require.Equal(t, "conn-raw", got.RequestContext.ConnectionID)
		require.Equal(t, "$disconnect", got.RequestContext.RouteKey)
	})

	t.Run("falls back to JSON body when raw event type mismatches", func(t *testing.T) {
		body, err := json.Marshal(events.APIGatewayWebsocketProxyRequest{
			RequestContext: events.APIGatewayWebsocketProxyRequestContext{
				ConnectionID: "conn-body",
				RouteKey:     "$default",
			},
		})
		require.NoError(t, err)

		ctx := &lift.Context{
			Request: lift.NewRequestWithContext(context.Background(), &adapters.Request{
				RawEvent: "not websocket event",
				Body:     body,
			}),
		}

		got := extractWebSocketEvent(ctx)
		require.Equal(t, "conn-body", got.RequestContext.ConnectionID)
		require.Equal(t, "$default", got.RequestContext.RouteKey)
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
		},
	}

	ctx := &lift.Context{
		Request: lift.NewRequestWithContext(context.Background(), &adapters.Request{
			RawEvent: wsEvent,
		}),
	}
	ctx.SetRequestID("req-1")

	next := lift.HandlerFunc(func(_ *lift.Context) error { return nil })
	handler := WebSocketCostMiddleware(tracker)(next)

	err := handler.Handle(ctx)
	require.NoError(t, err)

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
		ctx := &lift.Context{Request: lift.NewRequestWithContext(context.Background(), &adapters.Request{})}
		opCtx := &WebSocketOperationContext{UserID: "user-1"}

		require.NoError(t, checkBudgetIfRequired(tracker, ctx, "connect", opCtx))
	})

	t.Run("budget exceeded returns lift error", func(t *testing.T) {
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

		ctx := &lift.Context{Request: lift.NewRequestWithContext(context.Background(), &adapters.Request{})}
		opCtx := &WebSocketOperationContext{UserID: "user-1"}

		err := checkBudgetIfRequired(tracker, ctx, "connect", opCtx)
		require.Error(t, err)
	})

	t.Run("allow connection returns nil", func(t *testing.T) {
		tracker := buildTracker(t, func(dest *[]*models.WebSocketCostBudget) {
			*dest = []*models.WebSocketCostBudget{}
		})

		ctx := &lift.Context{Request: lift.NewRequestWithContext(context.Background(), &adapters.Request{})}
		opCtx := &WebSocketOperationContext{UserID: "user-1"}

		require.NoError(t, checkBudgetIfRequired(tracker, ctx, "connect", opCtx))
	})
}

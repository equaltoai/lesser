package lambda

import (
	"context"
	stdErrors "errors"
	"testing"
	"time"

	awsSDK "github.com/aws/aws-sdk-go-v2/aws"
	awsInit "github.com/equaltoai/lesser/pkg/aws"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/observability"
	apptesting "github.com/equaltoai/lesser/pkg/testing"
	storagemocks "github.com/equaltoai/lesser/pkg/testing/mocks"
	liftPkg "github.com/pay-theory/lift/pkg/lift"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func newHealthPattern(t *testing.T, getUserErr error, awsRegion string, startTime time.Time) *HealthCheckPattern {
	t.Helper()

	userRepo := storagemocks.NewMockUserRepositoryInterface()
	userRepo.On("GetUser", mock.Anything, "health-check-user").Return(nil, getUserErr)

	repos := apptesting.NewMockRepositoryStorage(
		apptesting.WithUserRepository(userRepo),
	)

	lambdaCtx := &common.LambdaContext{
		Config: &config.Config{
			Version:         "test",
			Region:          "us-east-1",
			DynamoTableName: "tbl",
		},
		Logger: zap.NewNop(),
		AWSServices: &awsInit.AWSServices{
			Config: awsSDK.Config{Region: awsRegion},
		},
		Repos: repos,
	}

	return NewHealthCheckPattern(lambdaCtx, startTime)
}

func TestHealthCheckPattern_handleLivenessCheck(t *testing.T) {
	hcp := newHealthPattern(t, stdErrors.New("user not found"), "us-east-1", time.Now().Add(-time.Minute))

	req := liftPkg.NewRequest(nil)
	req.Method = "GET"
	req.Path = "/health/live"
	ctx := liftPkg.NewContext(context.Background(), req)

	require.NoError(t, hcp.handleLivenessCheck(ctx))
	require.Equal(t, 200, ctx.Response.StatusCode)

	body, ok := ctx.Response.Body.(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, observability.HealthStatusHealthy, body["status"])
}

func TestHealthCheckPattern_handleReadinessCheck_HealthyWhenUserNotFound(t *testing.T) {
	hcp := newHealthPattern(t, stdErrors.New("user not found"), "us-east-1", time.Now())

	req := liftPkg.NewRequest(nil)
	req.Method = "GET"
	req.Path = "/health/ready"
	ctx := liftPkg.NewContext(context.Background(), req)

	require.NoError(t, hcp.handleReadinessCheck(ctx))
	require.Equal(t, 200, ctx.Response.StatusCode)

	body, ok := ctx.Response.Body.(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, observability.HealthStatusHealthy, body["status"])
}

func TestHealthCheckPattern_handleReadinessCheck_CriticalOnDBError(t *testing.T) {
	hcp := newHealthPattern(t, stdErrors.New("boom"), "us-east-1", time.Now())

	req := liftPkg.NewRequest(nil)
	req.Method = "GET"
	req.Path = "/health/ready"
	ctx := liftPkg.NewContext(context.Background(), req)

	require.NoError(t, hcp.handleReadinessCheck(ctx))
	require.Equal(t, 503, ctx.Response.StatusCode)

	body, ok := ctx.Response.Body.(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, observability.HealthStatusCritical, body["status"])
}

func TestHealthCheckPattern_handleDetailedHealthCheck_WarningSummary(t *testing.T) {
	hcp := newHealthPattern(t, stdErrors.New("user not found"), "", time.Now().Add(-16*time.Minute))

	req := liftPkg.NewRequest(nil)
	req.Method = "GET"
	req.Path = "/health/detailed"
	ctx := liftPkg.NewContext(context.Background(), req)

	require.NoError(t, hcp.handleDetailedHealthCheck(ctx))
	require.Equal(t, 200, ctx.Response.StatusCode)

	body, ok := ctx.Response.Body.(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, observability.HealthStatusWarning, body["status"])
}

func TestHealthCheckPattern_CreateHealthCheckMiddleware(t *testing.T) {
	hcp := newHealthPattern(t, stdErrors.New("user not found"), "us-east-1", time.Now())

	mw := hcp.CreateHealthCheckMiddleware()

	req := liftPkg.NewRequest(nil)
	req.Method = "GET"
	req.Path = "/health/live"
	ctx := liftPkg.NewContext(context.Background(), req)

	nextCalled := false
	handler := mw(liftPkg.HandlerFunc(func(*liftPkg.Context) error {
		nextCalled = true
		return nil
	}))

	require.NoError(t, handler.Handle(ctx))
	require.True(t, nextCalled)
}

func TestHealthCheckRoutes_NoPanic(t *testing.T) {
	hcp := newHealthPattern(t, stdErrors.New("user not found"), "us-east-1", time.Now())

	app := liftPkg.New()
	hcp.ConfigureHealthRoutes(app)

	minimalApp := liftPkg.New()
	hcp.ConfigureMinimalHealthRoutes(minimalApp)
}

func TestIsHealthCheckPath(t *testing.T) {
	require.True(t, isHealthCheckPath("/health/live"))
	require.True(t, isHealthCheckPath("/health"))
	require.False(t, isHealthCheckPath("/anything-else"))
}

func TestDefaultHealthCheckConfig(t *testing.T) {
	cfg := DefaultHealthCheckConfig()
	require.True(t, cfg.EnableDetailedChecks)
	require.Equal(t, 10, cfg.DBTimeoutSeconds)
	require.Equal(t, 5000, cfg.PerformanceWarningThresholdMs)
}

package cost

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestDefaultCostConfig(t *testing.T) {
	cfg := DefaultCostConfig()
	require.NotNil(t, cfg)
	require.Greater(t, cfg.LambdaRequestCost, 0.0)
	require.Greater(t, cfg.DynamoDBReadCost, 0.0)
	require.Greater(t, cfg.S3StorageCost, 0.0)
}

func TestCostAnalyzer_TracksAndReports(t *testing.T) {
	analyzer := NewCostAnalyzer(nil)

	analyzer.TrackLambdaInvocation(250*time.Millisecond, 256)
	analyzer.TrackDynamoDBOperation("GetItem", 1, 0)
	analyzer.TrackS3Operation("PutObject", 1024)
	analyzer.TrackAPIGatewayRequest(2048)

	report := analyzer.GetCostReport()
	require.Greater(t, report.TotalCost, 0.0)
	require.Contains(t, report.Services, "lambda")
	require.Contains(t, report.Services, "dynamodb")
	require.Contains(t, report.Services, "s3")
	require.Contains(t, report.Services, "apigateway")
	require.Equal(t, int64(1), report.Services["lambda"].Operations["invocations"])
}

func TestCheckBudgets(t *testing.T) {
	report := &CostReport{
		Services: map[string]*ServiceCostBreakdown{
			"lambda": {Service: "lambda", TotalCost: 80},
		},
		TotalCost: 80,
	}

	alerts := CheckBudgets(report, map[string]float64{"lambda": 100})
	require.Len(t, alerts, 1)
	require.Equal(t, "warning", alerts[0].AlertLevel)

	report.Services["lambda"].TotalCost = 90
	alerts = CheckBudgets(report, map[string]float64{"lambda": 100})
	require.Len(t, alerts, 1)
	require.Equal(t, "critical", alerts[0].AlertLevel)
}

func TestCalculateCostEfficiency(t *testing.T) {
	report := &CostReport{TotalCost: 0.01, Services: map[string]*ServiceCostBreakdown{}}
	metrics := CalculateCostEfficiency(report, 1000)
	require.NotEmpty(t, metrics)
	require.True(t, metrics[0].IsEfficient)
}

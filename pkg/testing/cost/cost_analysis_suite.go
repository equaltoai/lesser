package cost

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// CostConfig defines AWS service pricing
type CostConfig struct {
	// Lambda pricing (per million requests and per GB-second)
	LambdaRequestCost    float64 // per million requests
	LambdaGBSecondCost   float64 // per GB-second
	
	// DynamoDB pricing
	DynamoDBReadCost     float64 // per RCU per hour
	DynamoDBWriteCost    float64 // per WCU per hour
	DynamoDBStorageCost  float64 // per GB per month
	DynamoDBStreamCost   float64 // per million read requests
	
	// S3 pricing
	S3StorageCost        float64 // per GB per month
	S3RequestCost        float64 // per 1000 requests
	S3TransferCost       float64 // per GB
	
	// API Gateway pricing
	APIGatewayRequestCost float64 // per million requests
	APIGatewayDataTransfer float64 // per GB
	
	// CloudWatch pricing
	CloudWatchLogsCost   float64 // per GB ingested
	CloudWatchMetricsCost float64 // per metric per month
}

// DefaultCostConfig returns default AWS pricing (US East 1)
func DefaultCostConfig() *CostConfig {
	return &CostConfig{
		LambdaRequestCost:     0.20,    // per million
		LambdaGBSecondCost:    0.0000166667, // per GB-second
		DynamoDBReadCost:      0.00013,  // per RCU per hour
		DynamoDBWriteCost:     0.00065,  // per WCU per hour
		DynamoDBStorageCost:   0.25,     // per GB per month
		DynamoDBStreamCost:    0.02,     // per million
		S3StorageCost:         0.023,    // per GB per month
		S3RequestCost:         0.0004,   // per 1000 PUT/POST/LIST
		S3TransferCost:        0.09,     // per GB
		APIGatewayRequestCost: 3.50,     // per million
		APIGatewayDataTransfer: 0.09,    // per GB
		CloudWatchLogsCost:    0.50,     // per GB
		CloudWatchMetricsCost: 0.30,     // per metric
	}
}

// CostAnalyzer analyzes AWS service costs
type CostAnalyzer struct {
	config    *CostConfig
	collector *CostMetricsCollector
	mu        sync.RWMutex
}

// CostMetricsCollector collects cost metrics
type CostMetricsCollector struct {
	mu      sync.RWMutex
	metrics map[string]*ServiceMetrics
}

// ServiceMetrics tracks metrics for a service
type ServiceMetrics struct {
	Service        string
	OperationCount map[string]int64
	ResourceUsage  map[string]float64
	TotalCost      float64
	StartTime      time.Time
	EndTime        time.Time
}

// NewCostAnalyzer creates a new cost analyzer
func NewCostAnalyzer(config *CostConfig) *CostAnalyzer {
	if config == nil {
		config = DefaultCostConfig()
	}
	
	return &CostAnalyzer{
		config: config,
		collector: &CostMetricsCollector{
			metrics: make(map[string]*ServiceMetrics),
		},
	}
}

// TrackLambdaInvocation tracks Lambda invocation cost
func (ca *CostAnalyzer) TrackLambdaInvocation(duration time.Duration, memoryMB int) {
	ca.mu.Lock()
	defer ca.mu.Unlock()

	metrics := ca.getOrCreateMetrics("lambda")
	
	// Track invocation
	metrics.OperationCount["invocations"]++
	
	// Calculate GB-seconds
	gbSeconds := float64(memoryMB) / 1024.0 * duration.Seconds()
	metrics.ResourceUsage["gb-seconds"] += gbSeconds
	
	// Calculate cost
	invocationCost := ca.config.LambdaRequestCost / 1_000_000
	computeCost := gbSeconds * ca.config.LambdaGBSecondCost
	
	metrics.TotalCost += invocationCost + computeCost
}

// TrackDynamoDBOperation tracks DynamoDB operation cost
func (ca *CostAnalyzer) TrackDynamoDBOperation(operation string, consumedRCU, consumedWCU float64) {
	ca.mu.Lock()
	defer ca.mu.Unlock()

	metrics := ca.getOrCreateMetrics("dynamodb")
	
	// Track operation
	metrics.OperationCount[operation]++
	
	// Track capacity units
	metrics.ResourceUsage["read-capacity"] += consumedRCU
	metrics.ResourceUsage["write-capacity"] += consumedWCU
	
	// Calculate cost (assuming on-demand pricing)
	readCost := consumedRCU * 0.00013  // $0.25 per million RCU
	writeCost := consumedWCU * 0.00065 // $1.25 per million WCU
	
	metrics.TotalCost += readCost + writeCost
}

// TrackS3Operation tracks S3 operation cost
func (ca *CostAnalyzer) TrackS3Operation(operation string, sizeBytes int64) {
	ca.mu.Lock()
	defer ca.mu.Unlock()

	metrics := ca.getOrCreateMetrics("s3")
	
	// Track operation
	metrics.OperationCount[operation]++
	
	// Track data transfer
	sizeGB := float64(sizeBytes) / (1024 * 1024 * 1024)
	metrics.ResourceUsage["transfer-gb"] += sizeGB
	
	// Calculate cost
	requestCost := ca.config.S3RequestCost / 1000
	transferCost := sizeGB * ca.config.S3TransferCost
	
	metrics.TotalCost += requestCost + transferCost
}

// TrackAPIGatewayRequest tracks API Gateway request cost
func (ca *CostAnalyzer) TrackAPIGatewayRequest(dataTransferBytes int64) {
	ca.mu.Lock()
	defer ca.mu.Unlock()

	metrics := ca.getOrCreateMetrics("apigateway")
	
	// Track request
	metrics.OperationCount["requests"]++
	
	// Track data transfer
	dataGB := float64(dataTransferBytes) / (1024 * 1024 * 1024)
	metrics.ResourceUsage["data-transfer-gb"] += dataGB
	
	// Calculate cost
	requestCost := ca.config.APIGatewayRequestCost / 1_000_000
	transferCost := dataGB * ca.config.APIGatewayDataTransfer
	
	metrics.TotalCost += requestCost + transferCost
}

// getOrCreateMetrics gets or creates service metrics
func (ca *CostAnalyzer) getOrCreateMetrics(service string) *ServiceMetrics {
	if metrics, exists := ca.collector.metrics[service]; exists {
		return metrics
	}
	
	metrics := &ServiceMetrics{
		Service:        service,
		OperationCount: make(map[string]int64),
		ResourceUsage:  make(map[string]float64),
		StartTime:      time.Now(),
	}
	
	ca.collector.metrics[service] = metrics
	return metrics
}

// GetCostReport generates a cost report
func (ca *CostAnalyzer) GetCostReport() *CostReport {
	ca.mu.RLock()
	defer ca.mu.RUnlock()

	report := &CostReport{
		Services:      make(map[string]*ServiceCostBreakdown),
		TotalCost:     0,
		GeneratedAt:   time.Now(),
	}

	for service, metrics := range ca.collector.metrics {
		breakdown := &ServiceCostBreakdown{
			Service:       service,
			TotalCost:     metrics.TotalCost,
			Operations:    make(map[string]int64),
			ResourceUsage: make(map[string]float64),
		}

		// Copy operations
		for op, count := range metrics.OperationCount {
			breakdown.Operations[op] = count
		}

		// Copy resource usage
		for resource, usage := range metrics.ResourceUsage {
			breakdown.ResourceUsage[resource] = usage
		}

		report.Services[service] = breakdown
		report.TotalCost += metrics.TotalCost
	}

	return report
}

// CostReport represents a cost analysis report
type CostReport struct {
	Services    map[string]*ServiceCostBreakdown
	TotalCost   float64
	GeneratedAt time.Time
}

// ServiceCostBreakdown represents cost breakdown for a service
type ServiceCostBreakdown struct {
	Service       string
	TotalCost     float64
	Operations    map[string]int64
	ResourceUsage map[string]float64
}

// Cost Testing Utilities

// CostTestCase defines a cost analysis test case
type CostTestCase struct {
	Name           string
	Scenario       func(*CostAnalyzer)
	ExpectedMaxCost float64
	ValidateFunc   func(*testing.T, *CostReport)
}

// RunCostAnalysis runs cost analysis tests
func RunCostAnalysis(t *testing.T, testCases []CostTestCase) {
	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			analyzer := NewCostAnalyzer(DefaultCostConfig())
			
			// Run scenario
			tc.Scenario(analyzer)
			
			// Generate report
			report := analyzer.GetCostReport()
			
			// Validate cost
			assert.LessOrEqual(t, report.TotalCost, tc.ExpectedMaxCost,
				"Total cost $%.4f exceeds max $%.4f", report.TotalCost, tc.ExpectedMaxCost)
			
			// Custom validation
			if tc.ValidateFunc != nil {
				tc.ValidateFunc(t, report)
			}
			
			// Print report
			printCostReport(t, report)
		})
	}
}

// printCostReport prints a cost report
func printCostReport(t *testing.T, report *CostReport) {
	t.Logf("\n=== Cost Analysis Report ===")
	t.Logf("Generated: %s", report.GeneratedAt.Format(time.RFC3339))
	t.Logf("Total Cost: $%.4f", report.TotalCost)
	
	for service, breakdown := range report.Services {
		t.Logf("\n%s Service:", service)
		t.Logf("  Cost: $%.4f (%.1f%%)", breakdown.TotalCost, 
			breakdown.TotalCost/report.TotalCost*100)
		
		if len(breakdown.Operations) > 0 {
			t.Logf("  Operations:")
			for op, count := range breakdown.Operations {
				t.Logf("    %s: %d", op, count)
			}
		}
		
		if len(breakdown.ResourceUsage) > 0 {
			t.Logf("  Resource Usage:")
			for resource, usage := range breakdown.ResourceUsage {
				t.Logf("    %s: %.2f", resource, usage)
			}
		}
	}
}

// Cost Optimization Tests

// TestCostOptimization tests cost optimization strategies
func TestCostOptimization(t *testing.T, baseline, optimized func(*CostAnalyzer)) {
	// Run baseline
	baselineAnalyzer := NewCostAnalyzer(DefaultCostConfig())
	baseline(baselineAnalyzer)
	baselineReport := baselineAnalyzer.GetCostReport()
	
	// Run optimized
	optimizedAnalyzer := NewCostAnalyzer(DefaultCostConfig())
	optimized(optimizedAnalyzer)
	optimizedReport := optimizedAnalyzer.GetCostReport()
	
	// Compare costs
	savings := baselineReport.TotalCost - optimizedReport.TotalCost
	savingsPercent := savings / baselineReport.TotalCost * 100
	
	t.Logf("\n=== Cost Optimization Results ===")
	t.Logf("Baseline Cost: $%.4f", baselineReport.TotalCost)
	t.Logf("Optimized Cost: $%.4f", optimizedReport.TotalCost)
	t.Logf("Savings: $%.4f (%.1f%%)", savings, savingsPercent)
	
	// Assert optimization achieved savings
	assert.Greater(t, savings, 0.0, "Optimization should reduce costs")
	
	// Service-by-service comparison
	for service := range baselineReport.Services {
		baseline := baselineReport.Services[service]
		optimized := optimizedReport.Services[service]
		
		if baseline != nil && optimized != nil {
			serviceSavings := baseline.TotalCost - optimized.TotalCost
			t.Logf("\n%s Savings: $%.4f (%.1f%%)", service,
				serviceSavings, serviceSavings/baseline.TotalCost*100)
		}
	}
}

// MonthlyProjection projects monthly costs
type MonthlyProjection struct {
	Service          string
	DailyOperations  int64
	AverageSize      int64
	ProjectedCost    float64
}

// ProjectMonthlyCost projects monthly costs based on daily usage
func ProjectMonthlyCost(analyzer *CostAnalyzer, dailyMultiplier int) []MonthlyProjection {
	report := analyzer.GetCostReport()
	projections := make([]MonthlyProjection, 0)
	
	for service, breakdown := range report.Services {
		// Calculate daily average
		dailyCost := breakdown.TotalCost * float64(dailyMultiplier)
		monthlyCost := dailyCost * 30
		
		projection := MonthlyProjection{
			Service:       service,
			ProjectedCost: monthlyCost,
		}
		
		// Add operation counts
		for _, count := range breakdown.Operations {
			projection.DailyOperations += count * int64(dailyMultiplier)
		}
		
		projections = append(projections, projection)
	}
	
	return projections
}

// CostBudgetAlert checks if costs exceed budget
type CostBudgetAlert struct {
	Service      string
	CurrentCost  float64
	BudgetLimit  float64
	PercentUsed  float64
	AlertLevel   string // "warning", "critical"
}

// CheckBudgets checks costs against budgets
func CheckBudgets(report *CostReport, budgets map[string]float64) []CostBudgetAlert {
	alerts := make([]CostBudgetAlert, 0)
	
	for service, breakdown := range report.Services {
		if budget, exists := budgets[service]; exists {
			percentUsed := breakdown.TotalCost / budget * 100
			
			alert := CostBudgetAlert{
				Service:     service,
				CurrentCost: breakdown.TotalCost,
				BudgetLimit: budget,
				PercentUsed: percentUsed,
			}
			
			if percentUsed >= 90 {
				alert.AlertLevel = "critical"
				alerts = append(alerts, alert)
			} else if percentUsed >= 75 {
				alert.AlertLevel = "warning"
				alerts = append(alerts, alert)
			}
		}
	}
	
	return alerts
}

// CostEfficiencyMetric represents a cost efficiency metric
type CostEfficiencyMetric struct {
	Name        string
	Value       float64
	Unit        string
	Benchmark   float64
	IsEfficient bool
}

// CalculateCostEfficiency calculates cost efficiency metrics
func CalculateCostEfficiency(report *CostReport, operations int64) []CostEfficiencyMetric {
	metrics := []CostEfficiencyMetric{
		{
			Name:      "Cost per Operation",
			Value:     report.TotalCost / float64(operations) * 1000, // per 1000 ops
			Unit:      "per 1000 operations",
			Benchmark: 0.10, // $0.10 per 1000 operations
		},
	}
	
	// Lambda efficiency
	if lambda, exists := report.Services["lambda"]; exists {
		invocations := lambda.Operations["invocations"]
		if invocations > 0 {
			costPerInvocation := lambda.TotalCost / float64(invocations) * 1_000_000
			metrics = append(metrics, CostEfficiencyMetric{
				Name:      "Lambda Cost per Million",
				Value:     costPerInvocation,
				Unit:      "per million invocations",
				Benchmark: 0.50, // $0.50 per million
			})
		}
	}
	
	// DynamoDB efficiency
	if dynamodb, exists := report.Services["dynamodb"]; exists {
		totalCU := dynamodb.ResourceUsage["read-capacity"] + dynamodb.ResourceUsage["write-capacity"]
		if totalCU > 0 {
			costPerCU := dynamodb.TotalCost / totalCU
			metrics = append(metrics, CostEfficiencyMetric{
				Name:      "DynamoDB Cost per CU",
				Value:     costPerCU,
				Unit:      "per capacity unit",
				Benchmark: 0.0001, // $0.0001 per CU
			})
		}
	}
	
	// Mark efficient metrics
	for i := range metrics {
		metrics[i].IsEfficient = metrics[i].Value <= metrics[i].Benchmark
	}
	
	return metrics
}

// SimulateCostScenario simulates costs for different scenarios
type CostScenario struct {
	Name            string
	UserCount       int
	RequestsPerUser int
	DataPerUser     int64 // bytes
	Duration        time.Duration
}

// SimulateCost simulates costs for a scenario
func SimulateCost(scenario CostScenario, config *CostConfig) *CostReport {
	analyzer := NewCostAnalyzer(config)
	
	// Simulate Lambda invocations
	totalRequests := scenario.UserCount * scenario.RequestsPerUser
	for i := 0; i < totalRequests; i++ {
		// Average Lambda execution: 200ms, 256MB
		analyzer.TrackLambdaInvocation(200*time.Millisecond, 256)
	}
	
	// Simulate DynamoDB operations (2 reads + 1 write per request)
	for i := 0; i < totalRequests; i++ {
		analyzer.TrackDynamoDBOperation("GetItem", 1, 0)
		analyzer.TrackDynamoDBOperation("Query", 2, 0)
		analyzer.TrackDynamoDBOperation("PutItem", 0, 1)
	}
	
	// Simulate S3 operations (10% of requests involve media)
	mediaRequests := totalRequests / 10
	for i := 0; i < mediaRequests; i++ {
		analyzer.TrackS3Operation("PutObject", scenario.DataPerUser)
	}
	
	// Simulate API Gateway
	for i := 0; i < totalRequests; i++ {
		analyzer.TrackAPIGatewayRequest(1024) // 1KB average response
	}
	
	return analyzer.GetCostReport()
}
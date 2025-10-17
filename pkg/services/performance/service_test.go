package performance

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/graph/model"
	"go.uber.org/zap"
)

func TestNewService(t *testing.T) {
	logger := zap.NewNop()

	service := NewService(nil, "test", logger)
	if service == nil {
		t.Fatal("NewService returned nil")
	}

	if service.environment != "test" {
		t.Errorf("Expected environment 'test', got '%s'", service.environment)
	}

	if service.logger != logger {
		t.Error("Logger not set correctly")
	}
}

func TestPeriodToDuration(t *testing.T) {
	service := &Service{}

	tests := []struct {
		name     string
		period   model.TimePeriod
		expected time.Duration
	}{
		{"Hour", model.TimePeriodHour, time.Hour},
		{"Day", model.TimePeriodDay, 24 * time.Hour},
		{"Week", model.TimePeriodWeek, 7 * 24 * time.Hour},
		{"Month", model.TimePeriodMonth, 30 * 24 * time.Hour},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := service.periodToDuration(tt.period)
			if result != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestGetServiceFunctionNames(t *testing.T) {
	service := &Service{
		environment: "test",
	}

	tests := []struct {
		name     string
		category model.ServiceCategory
		expected []string
	}{
		{
			"GraphQL API",
			model.ServiceCategoryGraphqlAPI,
			[]string{"lesser-test-graphql", "lesser-test-api"},
		},
		{
			"Federation Delivery",
			model.ServiceCategoryFederationDelivery,
			[]string{
				"lesser-test-federation-delivery",
				"lesser-test-federation-tracker",
				"lesser-test-inbox",
				"lesser-test-outbox",
			},
		},
		{
			"Media Processor",
			model.ServiceCategoryMediaProcessor,
			[]string{"lesser-test-media-processor"},
		},
		{
			"Moderation Engine",
			model.ServiceCategoryModerationEngine,
			[]string{"lesser-test-moderation-processor", "lesser-test-ai-processor"},
		},
		{
			"Search Indexer",
			model.ServiceCategorySearchIndexer,
			[]string{"lesser-test-search-indexer", "lesser-test-status-indexer"},
		},
		{
			"Streaming Service",
			model.ServiceCategoryStreamingService,
			[]string{"lesser-test-streaming", "lesser-test-stream-router"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := service.getServiceFunctionNames(tt.category)
			if len(result) != len(tt.expected) {
				t.Errorf("Expected %d functions, got %d", len(tt.expected), len(result))
				return
			}

			for i, name := range tt.expected {
				if result[i] != name {
					t.Errorf("Expected function name '%s', got '%s'", name, result[i])
				}
			}
		})
	}
}

func TestCalculatePercentile(t *testing.T) {
	service := &Service{}

	t.Run("Empty slice", func(t *testing.T) {
		result := service.calculatePercentile([]float64{}, 0.50)
		if result != 0 {
			t.Errorf("Expected 0 for empty slice, got %v", result)
		}
	})

	t.Run("Single value", func(t *testing.T) {
		result := service.calculatePercentile([]float64{100.0}, 0.50)
		expected := 100 * time.Millisecond
		if result != expected {
			t.Errorf("Expected %v, got %v", expected, result)
		}
	})

	t.Run("Multiple values - P50", func(t *testing.T) {
		values := []float64{100.0, 200.0, 300.0, 400.0, 500.0}
		result := service.calculatePercentile(values, 0.50)
		expected := 300 * time.Millisecond
		if result != expected {
			t.Errorf("Expected %v, got %v", expected, result)
		}
	})

	t.Run("Multiple values - P95", func(t *testing.T) {
		values := []float64{100.0, 200.0, 300.0, 400.0, 500.0, 600.0, 700.0, 800.0, 900.0, 1000.0}
		result := service.calculatePercentile(values, 0.95)
		// Should be around 950ms
		if result < 900*time.Millisecond || result > 1000*time.Millisecond {
			t.Errorf("Expected P95 around 950ms, got %v", result)
		}
	})

	t.Run("Unsorted values", func(t *testing.T) {
		values := []float64{500.0, 100.0, 300.0, 400.0, 200.0}
		result := service.calculatePercentile(values, 0.50)
		expected := 300 * time.Millisecond
		if result != expected {
			t.Errorf("Expected %v, got %v", expected, result)
		}
	})
}

func TestEmptyReport(t *testing.T) {
	service := &Service{}

	report := service.emptyReport(model.ServiceCategoryGraphqlAPI, model.TimePeriodDay)

	if report.Service != model.ServiceCategoryGraphqlAPI {
		t.Errorf("Expected service category GraphqlAPI, got %v", report.Service)
	}

	if report.Period != model.TimePeriodDay {
		t.Errorf("Expected period Day, got %v", report.Period)
	}

	if report.P50Latency != 0 {
		t.Errorf("Expected P50 latency 0, got %v", report.P50Latency)
	}

	if report.P95Latency != 0 {
		t.Errorf("Expected P95 latency 0, got %v", report.P95Latency)
	}

	if report.P99Latency != 0 {
		t.Errorf("Expected P99 latency 0, got %v", report.P99Latency)
	}

	if report.ErrorRate != 0 {
		t.Errorf("Expected error rate 0, got %v", report.ErrorRate)
	}

	if report.Throughput != 0 {
		t.Errorf("Expected throughput 0, got %v", report.Throughput)
	}

	if report.ColdStarts != 0 {
		t.Errorf("Expected cold starts 0, got %v", report.ColdStarts)
	}
}

func TestAggregateMetricsFromFunctions(t *testing.T) {
	service := &Service{
		logger: zap.NewNop(),
	}

	ctx := context.Background()

	// Test with empty function list
	aggregator := service.aggregateMetricsFromFunctions(ctx, []string{}, time.Now().Add(-time.Hour), time.Now())

	if aggregator == nil {
		t.Fatal("Expected non-nil aggregator")
	}

	if aggregator.totalInvocations != 0 {
		t.Errorf("Expected 0 invocations for empty list, got %d", aggregator.totalInvocations)
	}

	if aggregator.totalErrors != 0 {
		t.Errorf("Expected 0 errors for empty list, got %d", aggregator.totalErrors)
	}

	if aggregator.coldStarts != 0 {
		t.Errorf("Expected 0 cold starts for empty list, got %d", aggregator.coldStarts)
	}
}

func TestGetPerformanceMetrics_InvalidCategory(t *testing.T) {
	service := &Service{
		logger:      zap.NewNop(),
		environment: "test",
	}

	ctx := context.Background()

	// Test with empty category (should fail validation)
	_, err := service.GetPerformanceMetrics(ctx, model.ServiceCategory(""), model.TimePeriodDay)
	if err == nil {
		t.Error("Expected error for empty service category")
	}
}

func TestGetPerformanceMetrics_NoFunctions(t *testing.T) {
	service := &Service{
		logger:      zap.NewNop(),
		environment: "test",
	}

	ctx := context.Background()

	// Create a custom service category that won't match any functions
	// This should return an empty report
	customCategory := model.ServiceCategory("NONEXISTENT")
	report, err := service.GetPerformanceMetrics(ctx, customCategory, model.TimePeriodDay)

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if report == nil {
		t.Fatal("Expected non-nil report")
	}

	if report.Service != customCategory {
		t.Errorf("Expected service %v, got %v", customCategory, report.Service)
	}

	// Empty report should have zero values
	if report.Throughput != 0 || report.ErrorRate != 0 || report.ColdStarts != 0 {
		t.Error("Expected empty report with zero values")
	}
}

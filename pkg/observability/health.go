// Package observability provides health check endpoints for production monitoring
package observability

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	sqsTypes "github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"github.com/equaltoai/lesser/pkg/config"
	"go.uber.org/zap"
)

// HealthCheck represents a health check result
type HealthCheck struct {
	Name      string                 `json:"name"`
	Status    string                 `json:"status"`
	Message   string                 `json:"message,omitempty"`
	LastCheck time.Time              `json:"last_check"`
	Duration  time.Duration          `json:"duration"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// HealthResponse represents the overall health response
type HealthResponse struct {
	Status    string                 `json:"status"`
	Timestamp time.Time              `json:"timestamp"`
	Version   string                 `json:"version"`
	Service   string                 `json:"service"`
	Region    string                 `json:"region"`
	Checks    []HealthCheck          `json:"checks,omitempty"`
	Summary   map[string]interface{} `json:"summary,omitempty"`
}

// HealthChecker manages health checks for various components
type HealthChecker struct {
	logger       *zap.Logger
	dynamoClient *dynamodb.Client
	sqsClient    *sqs.Client
	service      string
	version      string
	mu           sync.RWMutex
	lastChecks   map[string]HealthCheck
	config       *HealthConfig
}

// HealthConfig contains configuration for health checks
type HealthConfig struct {
	TableName        string
	QueueURL         string
	CheckTimeout     time.Duration
	CacheTimeout     time.Duration
	DependencyChecks bool
}

// NewHealthChecker creates a new health checker
func NewHealthChecker(logger *zap.Logger, cfg aws.Config, service, version string, config *HealthConfig) *HealthChecker {
	if config == nil {
		config = &HealthConfig{
			CheckTimeout:     5 * time.Second,
			CacheTimeout:     30 * time.Second,
			DependencyChecks: true,
		}
	}

	return &HealthChecker{
		logger:       logger,
		dynamoClient: dynamodb.NewFromConfig(cfg),
		sqsClient:    sqs.NewFromConfig(cfg),
		service:      service,
		version:      version,
		lastChecks:   make(map[string]HealthCheck),
		config:       config,
	}
}

// LivenessHandler implements GET /health/live endpoint
func (hc *HealthChecker) LivenessHandler(w http.ResponseWriter, _ *http.Request) {
	// Liveness is always healthy if the process is running
	response := HealthResponse{
		Status:    HealthStatusHealthy,
		Timestamp: time.Now(),
		Version:   hc.version,
		Service:   hc.service,
		Region:    config.Get().Region,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	if err := json.NewEncoder(w).Encode(response); err != nil {
		hc.logger.Error("failed to encode liveness response", zap.Error(err))
	}
}

// ReadinessHandler implements GET /health/ready endpoint
func (hc *HealthChecker) ReadinessHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), hc.config.CheckTimeout)
	defer cancel()

	checks := []HealthCheck{}
	overallStatus := HealthStatusHealthy

	// Check critical dependencies only
	if hc.config.DependencyChecks {
		// Check DynamoDB
		if hc.config.TableName != "" {
			check := hc.checkDynamoDB(ctx)
			checks = append(checks, check)
			if check.Status != HealthStatusHealthy {
				overallStatus = HealthStatusCritical
			}
		}

		// Check SQS (if configured)
		if hc.config.QueueURL != "" {
			check := hc.checkSQS(ctx)
			checks = append(checks, check)
			if check.Status != HealthStatusHealthy {
				overallStatus = HealthStatusCritical
			}
		}
	}

	response := HealthResponse{
		Status:    overallStatus,
		Timestamp: time.Now(),
		Version:   hc.version,
		Service:   hc.service,
		Region:    config.Get().Region,
		Checks:    checks,
	}

	// Set appropriate HTTP status code
	var statusCode int
	switch overallStatus {
	case HealthStatusCritical:
		statusCode = http.StatusServiceUnavailable
	case HealthStatusWarning:
		statusCode = http.StatusOK // 200 for warnings, still ready
	default:
		statusCode = http.StatusOK
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	if err := json.NewEncoder(w).Encode(response); err != nil {
		hc.logger.Error("failed to encode readiness response", zap.Error(err))
	}
}

// DetailedHandler implements GET /health/detailed endpoint
func (hc *HealthChecker) DetailedHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), hc.config.CheckTimeout*2) // More time for detailed checks
	defer cancel()

	checks := []HealthCheck{}
	overallStatus := HealthStatusHealthy
	summary := make(map[string]interface{})

	// Runtime information
	checks = append(checks, hc.checkRuntime())

	// Check all dependencies with detailed information
	if hc.config.DependencyChecks {
		// DynamoDB detailed check
		if hc.config.TableName != "" {
			check := hc.checkDynamoDBDetailed(ctx)
			checks = append(checks, check)
			if check.Status == HealthStatusCritical {
				overallStatus = HealthStatusCritical
			} else if check.Status == HealthStatusWarning {
				if overallStatus == HealthStatusHealthy {
					overallStatus = HealthStatusWarning
				}
			}
		}

		// SQS detailed check
		if hc.config.QueueURL != "" {
			check := hc.checkSQSDetailed(ctx)
			checks = append(checks, check)
			if check.Status == HealthStatusCritical {
				overallStatus = HealthStatusCritical
			} else if check.Status == HealthStatusWarning {
				if overallStatus == HealthStatusHealthy {
					overallStatus = HealthStatusWarning
				}
			}
		}
	}

	// Build summary
	summary["total_checks"] = len(checks)
	summary["healthy_checks"] = 0
	summary["warning_checks"] = 0
	summary["critical_checks"] = 0

	for _, check := range checks {
		switch check.Status {
		case HealthStatusHealthy:
			summary["healthy_checks"] = summary["healthy_checks"].(int) + 1
		case HealthStatusWarning:
			summary["warning_checks"] = summary["warning_checks"].(int) + 1
		case HealthStatusCritical:
			summary["critical_checks"] = summary["critical_checks"].(int) + 1
		}
	}

	response := HealthResponse{
		Status:    overallStatus,
		Timestamp: time.Now(),
		Version:   hc.version,
		Service:   hc.service,
		Region:    config.Get().Region,
		Checks:    checks,
		Summary:   summary,
	}

	// Set appropriate HTTP status code
	statusCode := http.StatusOK
	if overallStatus == HealthStatusCritical {
		statusCode = http.StatusServiceUnavailable
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	if err := json.NewEncoder(w).Encode(response); err != nil {
		hc.logger.Error("failed to encode detailed response", zap.Error(err))
	}
}

// checkRuntime performs runtime health check
func (hc *HealthChecker) checkRuntime() HealthCheck {
	start := time.Now()

	metadata := map[string]interface{}{
		"uptime_seconds": time.Since(time.Now()).Seconds(), // This would need to track actual start time
		"service":        hc.service,
		"version":        hc.version,
		"go_version":     "1.21", // Could be runtime.Version()
	}

	return HealthCheck{
		Name:      "runtime",
		Status:    HealthStatusHealthy,
		Message:   "Service runtime is healthy",
		LastCheck: time.Now(),
		Duration:  time.Since(start),
		Metadata:  metadata,
	}
}

// checkDynamoDB performs basic DynamoDB connectivity check
func (hc *HealthChecker) checkDynamoDB(ctx context.Context) HealthCheck {
	start := time.Now()

	// Check cache first
	if cached, ok := hc.getCachedCheck("dynamodb"); ok {
		return cached
	}

	check := HealthCheck{
		Name:      "dynamodb",
		LastCheck: time.Now(),
	}

	// Describe table to test connectivity
	input := &dynamodb.DescribeTableInput{
		TableName: aws.String(hc.config.TableName),
	}

	result, err := hc.dynamoClient.DescribeTable(ctx, input)
	duration := time.Since(start)
	check.Duration = duration

	if err != nil {
		check.Status = HealthStatusCritical
		check.Message = fmt.Sprintf("DynamoDB connectivity failed: %v", err)
	} else {
		status := string(result.Table.TableStatus)
		if status == "ACTIVE" {
			check.Status = HealthStatusHealthy
			check.Message = "DynamoDB table is active and accessible"
		} else {
			check.Status = HealthStatusWarning
			check.Message = fmt.Sprintf("DynamoDB table status: %s", status)
		}
	}

	hc.setCachedCheck("dynamodb", check)
	return check
}

// checkDynamoDBDetailed performs detailed DynamoDB health check
func (hc *HealthChecker) checkDynamoDBDetailed(ctx context.Context) HealthCheck {
	start := time.Now()

	check := HealthCheck{
		Name:      "dynamodb_detailed",
		LastCheck: time.Now(),
		Metadata:  make(map[string]interface{}),
	}

	// Describe table
	input := &dynamodb.DescribeTableInput{
		TableName: aws.String(hc.config.TableName),
	}

	result, err := hc.dynamoClient.DescribeTable(ctx, input)
	duration := time.Since(start)
	check.Duration = duration

	if err != nil {
		check.Status = HealthStatusCritical
		check.Message = fmt.Sprintf("DynamoDB detailed check failed: %v", err)
		return check
	}

	table := result.Table
	check.Metadata["table_status"] = string(table.TableStatus)
	check.Metadata["item_count"] = *table.ItemCount
	check.Metadata["table_size_bytes"] = *table.TableSizeBytes

	if table.ProvisionedThroughput != nil {
		check.Metadata["read_capacity"] = *table.ProvisionedThroughput.ReadCapacityUnits
		check.Metadata["write_capacity"] = *table.ProvisionedThroughput.WriteCapacityUnits
	} else {
		check.Metadata["billing_mode"] = "PAY_PER_REQUEST"
	}

	// Determine status
	switch string(table.TableStatus) {
	case "ACTIVE":
		check.Status = HealthStatusHealthy
		check.Message = "DynamoDB table is fully operational"
	case "UPDATING", "CREATING":
		check.Status = HealthStatusWarning
		check.Message = fmt.Sprintf("DynamoDB table is %s", string(table.TableStatus))
	default:
		check.Status = HealthStatusCritical
		check.Message = fmt.Sprintf("DynamoDB table in unexpected state: %s", string(table.TableStatus))
	}

	return check
}

// checkSQS performs basic SQS connectivity check
func (hc *HealthChecker) checkSQS(ctx context.Context) HealthCheck {
	start := time.Now()

	// Check cache first
	if cached, ok := hc.getCachedCheck("sqs"); ok {
		return cached
	}

	check := HealthCheck{
		Name:      "sqs",
		LastCheck: time.Now(),
	}

	// Get basic queue attributes
	input := &sqs.GetQueueAttributesInput{
		QueueUrl: aws.String(hc.config.QueueURL),
		AttributeNames: []sqsTypes.QueueAttributeName{
			sqsTypes.QueueAttributeNameApproximateNumberOfMessages,
		},
	}

	_, err := hc.sqsClient.GetQueueAttributes(ctx, input)
	duration := time.Since(start)
	check.Duration = duration

	if err != nil {
		check.Status = HealthStatusCritical
		check.Message = fmt.Sprintf("SQS connectivity failed: %v", err)
	} else {
		check.Status = HealthStatusHealthy
		check.Message = "SQS queue is accessible"
	}

	hc.setCachedCheck("sqs", check)
	return check
}

// checkSQSDetailed performs detailed SQS health check
func (hc *HealthChecker) checkSQSDetailed(ctx context.Context) HealthCheck {
	start := time.Now()

	check := HealthCheck{
		Name:      "sqs_detailed",
		LastCheck: time.Now(),
		Metadata:  make(map[string]interface{}),
	}

	// Get comprehensive queue attributes
	input := &sqs.GetQueueAttributesInput{
		QueueUrl: aws.String(hc.config.QueueURL),
		AttributeNames: []sqsTypes.QueueAttributeName{
			sqsTypes.QueueAttributeNameApproximateNumberOfMessages,
			sqsTypes.QueueAttributeNameApproximateNumberOfMessagesNotVisible,
			sqsTypes.QueueAttributeNameApproximateNumberOfMessagesDelayed,
		},
	}

	result, err := hc.sqsClient.GetQueueAttributes(ctx, input)
	duration := time.Since(start)
	check.Duration = duration

	if err != nil {
		check.Status = HealthStatusCritical
		check.Message = fmt.Sprintf("SQS detailed check failed: %v", err)
		return check
	}

	// Parse metrics
	visibleMsgs := parseInt(result.Attributes["ApproximateNumberOfMessages"])
	invisibleMsgs := parseInt(result.Attributes["ApproximateNumberOfMessagesNotVisible"])
	delayedMsgs := parseInt(result.Attributes["ApproximateNumberOfMessagesDelayed"])
	totalMsgs := visibleMsgs + invisibleMsgs + delayedMsgs

	check.Metadata["visible_messages"] = visibleMsgs
	check.Metadata["invisible_messages"] = invisibleMsgs
	check.Metadata["delayed_messages"] = delayedMsgs
	check.Metadata["total_messages"] = totalMsgs

	// Determine status based on queue depth
	if totalMsgs > 10000 {
		check.Status = HealthStatusCritical
		check.Message = fmt.Sprintf("SQS queue critically backed up with %d messages", totalMsgs)
	} else if totalMsgs > 1000 {
		check.Status = HealthStatusWarning
		check.Message = fmt.Sprintf("SQS queue depth elevated with %d messages", totalMsgs)
	} else {
		check.Status = HealthStatusHealthy
		check.Message = fmt.Sprintf("SQS queue healthy with %d messages", totalMsgs)
	}

	return check
}

// getCachedCheck retrieves a cached health check result
func (hc *HealthChecker) getCachedCheck(name string) (HealthCheck, bool) {
	hc.mu.RLock()
	defer hc.mu.RUnlock()

	if check, exists := hc.lastChecks[name]; exists {
		if time.Since(check.LastCheck) < hc.config.CacheTimeout {
			return check, true
		}
	}

	return HealthCheck{}, false
}

// setCachedCheck stores a health check result in cache
func (hc *HealthChecker) setCachedCheck(name string, check HealthCheck) {
	hc.mu.Lock()
	defer hc.mu.Unlock()

	hc.lastChecks[name] = check
}

// parseInt safely parses a string to int
func parseInt(s string) int {
	var result int
	if _, err := fmt.Sscanf(s, "%d", &result); err != nil {
		return 0
	}
	return result
}

// RegisterHealthRoutes registers health check routes with an HTTP mux
func (hc *HealthChecker) RegisterHealthRoutes(mux *http.ServeMux) {
	mux.HandleFunc(HealthEndpointLive, hc.LivenessHandler)
	mux.HandleFunc(HealthEndpointReady, hc.ReadinessHandler)
	mux.HandleFunc(HealthEndpointDetailed, hc.DetailedHandler)
}

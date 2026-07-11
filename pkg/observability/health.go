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
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	sqsTypes "github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/theory-cloud/tabletheory/v2/pkg/model"
	"github.com/theory-cloud/tabletheory/v2/pkg/schema"
	"github.com/theory-cloud/tabletheory/v2/pkg/session"
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
	tableChecker tableExistsChecker
	sqsClient    sqsGetQueueAttributesAPI
	service      string
	version      string
	mu           sync.RWMutex
	lastChecks   map[string]HealthCheck
	config       *HealthConfig
}

type tableExistsChecker interface {
	TableExists(tableName string) (bool, error)
}

type sqsGetQueueAttributesAPI interface {
	GetQueueAttributes(ctx context.Context, params *sqs.GetQueueAttributesInput, optFns ...func(*sqs.Options)) (*sqs.GetQueueAttributesOutput, error)
}

// HealthConfig contains configuration for health checks
type HealthConfig struct {
	TableName        string
	QueueURL         string
	CheckTimeout     time.Duration
	CacheTimeout     time.Duration
	DependencyChecks bool
}

type errorTableExistsChecker struct {
	err error
}

func (c errorTableExistsChecker) TableExists(_ string) (bool, error) {
	return false, c.err
}

var newTableExistsChecker = func(cfg aws.Config) (tableExistsChecker, error) {
	sess, err := session.NewSession(&session.Config{
		Region:              cfg.Region,
		CredentialsProvider: cfg.Credentials,
	})
	if err != nil {
		return nil, err
	}
	registry := model.NewRegistry()
	return schema.NewManager(sess, registry), nil
}

// NewHealthChecker creates a new health checker
func NewHealthChecker(logger *zap.Logger, cfg aws.Config, service, version string, healthConfig *HealthConfig) *HealthChecker {
	if healthConfig == nil {
		healthConfig = &HealthConfig{
			CheckTimeout:     5 * time.Second,
			CacheTimeout:     30 * time.Second,
			DependencyChecks: true,
		}
	}

	if healthConfig.TableName == "" {
		healthConfig.TableName = config.GetMainTableName()
	}

	tableChecker, err := newTableExistsChecker(cfg)
	if err != nil {
		logger.Warn("failed to initialize TableTheory DynamoDB client for health checks", zap.Error(err))
		tableChecker = errorTableExistsChecker{err: err}
	}

	return &HealthChecker{
		logger:       logger,
		tableChecker: tableChecker,
		sqsClient:    sqs.NewFromConfig(cfg),
		service:      service,
		version:      version,
		lastChecks:   make(map[string]HealthCheck),
		config:       healthConfig,
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
	updateOverallStatus := func(status string) {
		switch status {
		case HealthStatusCritical:
			overallStatus = HealthStatusCritical
		case HealthStatusWarning:
			if overallStatus == HealthStatusHealthy {
				overallStatus = HealthStatusWarning
			}
		}
	}

	// Runtime information
	checks = append(checks, hc.checkRuntime())

	// Check all dependencies with detailed information
	if hc.config.DependencyChecks {
		// DynamoDB detailed check
		if hc.config.TableName != "" {
			check := hc.checkDynamoDBDetailed(ctx)
			checks = append(checks, check)
			updateOverallStatus(check.Status)
		}

		// SQS detailed check
		if hc.config.QueueURL != "" {
			check := hc.checkSQSDetailed(ctx)
			checks = append(checks, check)
			updateOverallStatus(check.Status)
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

	select {
	case <-ctx.Done():
		check.Status = HealthStatusCritical
		check.Message = fmt.Sprintf("DynamoDB connectivity failed: %v", ctx.Err())
		check.Duration = time.Since(start)
		hc.setCachedCheck("dynamodb", check)
		return check
	default:
	}

	exists, err := hc.tableChecker.TableExists(hc.config.TableName)
	duration := time.Since(start)
	check.Duration = duration

	if err != nil {
		check.Status = HealthStatusCritical
		check.Message = fmt.Sprintf("DynamoDB connectivity failed: %v", err)
	} else if !exists {
		check.Status = HealthStatusCritical
		check.Message = "DynamoDB table not found"
	} else {
		check.Status = HealthStatusHealthy
		check.Message = "DynamoDB table is accessible"
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

	select {
	case <-ctx.Done():
		check.Status = HealthStatusCritical
		check.Message = fmt.Sprintf("DynamoDB detailed check failed: %v", ctx.Err())
		check.Duration = time.Since(start)
		return check
	default:
	}

	exists, err := hc.tableChecker.TableExists(hc.config.TableName)
	duration := time.Since(start)
	check.Duration = duration

	if err != nil {
		check.Status = HealthStatusCritical
		check.Message = fmt.Sprintf("DynamoDB detailed check failed: %v", err)
		return check
	}

	check.Metadata["table_name"] = hc.config.TableName
	check.Metadata["table_exists"] = exists

	if exists {
		check.Status = HealthStatusHealthy
		check.Message = "DynamoDB table is fully operational"
	} else {
		check.Status = HealthStatusCritical
		check.Message = "DynamoDB table not found"
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

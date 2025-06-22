package monitoring

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/lambda"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	sqsTypes "github.com/aws/aws-sdk-go-v2/service/sqs/types"
)

// HealthMonitor monitors infrastructure health
type HealthMonitor struct {
	monitor      *PerformanceMonitor
	dynamoClient *dynamodb.Client
	lambdaClient *lambda.Client
	sqsClient    *sqs.Client
	mu           sync.RWMutex
	healthStatus map[string]*ComponentHealth
}

// ComponentHealth represents the health status of a component
type ComponentHealth struct {
	Component      string
	Status         HealthStatus
	LastCheck      time.Time
	ErrorCount     int
	LastError      error
	Metadata       map[string]interface{}
}

// HealthStatus represents the health state
type HealthStatus string

const (
	HealthStatusHealthy   HealthStatus = "healthy"
	HealthStatusWarning   HealthStatus = "warning"
	HealthStatusCritical  HealthStatus = "critical"
	HealthStatusUnknown   HealthStatus = "unknown"
)

// NewHealthMonitor creates a new health monitor
func NewHealthMonitor(cfg aws.Config, monitor *PerformanceMonitor) *HealthMonitor {
	return &HealthMonitor{
		monitor:      monitor,
		dynamoClient: dynamodb.NewFromConfig(cfg),
		lambdaClient: lambda.NewFromConfig(cfg),
		sqsClient:    sqs.NewFromConfig(cfg),
		healthStatus: make(map[string]*ComponentHealth),
	}
}

// CheckDynamoDBHealth checks DynamoDB table health
func (hm *HealthMonitor) CheckDynamoDBHealth(ctx context.Context, tableName string) error {
	start := time.Now()
	
	// Describe table to check status
	input := &dynamodb.DescribeTableInput{
		TableName: aws.String(tableName),
	}
	
	result, err := hm.dynamoClient.DescribeTable(ctx, input)
	if err != nil {
		hm.updateComponentHealth("dynamodb."+tableName, HealthStatusCritical, err, nil)
		return fmt.Errorf("failed to describe table: %w", err)
	}
	
	latency := time.Since(start)
	hm.monitor.RecordLatency(ctx, "HealthCheck.DynamoDB", float64(latency.Milliseconds()))
	
	// Check table status
	tableStatus := string(result.Table.TableStatus)
	var status HealthStatus
	metadata := map[string]interface{}{
		"tableStatus": tableStatus,
		"itemCount":   result.Table.ItemCount,
		"tableSizeBytes": result.Table.TableSizeBytes,
	}
	
	switch tableStatus {
	case "ACTIVE":
		status = HealthStatusHealthy
	case "UPDATING", "CREATING":
		status = HealthStatusWarning
	default:
		status = HealthStatusCritical
	}
	
	// Check for throttling
	if result.Table.ProvisionedThroughput != nil {
		if result.Table.ProvisionedThroughput.ReadCapacityUnits != nil {
			metadata["readCapacity"] = *result.Table.ProvisionedThroughput.ReadCapacityUnits
		}
		if result.Table.ProvisionedThroughput.WriteCapacityUnits != nil {
			metadata["writeCapacity"] = *result.Table.ProvisionedThroughput.WriteCapacityUnits
		}
	}
	
	hm.updateComponentHealth("dynamodb."+tableName, status, nil, metadata)
	
	return nil
}

// CheckLambdaHealth checks Lambda function health
func (hm *HealthMonitor) CheckLambdaHealth(ctx context.Context, functionName string) error {
	start := time.Now()
	
	// Get function configuration
	input := &lambda.GetFunctionConfigurationInput{
		FunctionName: aws.String(functionName),
	}
	
	result, err := hm.lambdaClient.GetFunctionConfiguration(ctx, input)
	if err != nil {
		hm.updateComponentHealth("lambda."+functionName, HealthStatusCritical, err, nil)
		return fmt.Errorf("failed to get function configuration: %w", err)
	}
	
	latency := time.Since(start)
	hm.monitor.RecordLatency(ctx, "HealthCheck.Lambda", float64(latency.Milliseconds()))
	
	// Check function state
	var status HealthStatus
	metadata := map[string]interface{}{
		"state":       string(result.State),
		"runtime":     string(result.Runtime),
		"memorySize":  *result.MemorySize,
		"timeout":     *result.Timeout,
	}
	
	if result.LastUpdateStatus != "" {
		metadata["lastUpdateStatus"] = string(result.LastUpdateStatus)
	}
	
	switch result.State {
	case "Active":
		status = HealthStatusHealthy
	case "Pending":
		status = HealthStatusWarning
	default:
		status = HealthStatusCritical
	}
	
	hm.updateComponentHealth("lambda."+functionName, status, nil, metadata)
	
	return nil
}

// CheckSQSHealth checks SQS queue health
func (hm *HealthMonitor) CheckSQSHealth(ctx context.Context, queueURL string) error {
	start := time.Now()
	
	// Get queue attributes
	input := &sqs.GetQueueAttributesInput{
		QueueUrl: aws.String(queueURL),
		AttributeNames: []sqsTypes.QueueAttributeName{
			sqsTypes.QueueAttributeNameApproximateNumberOfMessages,
			sqsTypes.QueueAttributeNameApproximateNumberOfMessagesNotVisible,
			sqsTypes.QueueAttributeNameApproximateNumberOfMessagesDelayed,
		},
	}
	
	result, err := hm.sqsClient.GetQueueAttributes(ctx, input)
	if err != nil {
		hm.updateComponentHealth("sqs."+queueURL, HealthStatusCritical, err, nil)
		return fmt.Errorf("failed to get queue attributes: %w", err)
	}
	
	latency := time.Since(start)
	hm.monitor.RecordLatency(ctx, "HealthCheck.SQS", float64(latency.Milliseconds()))
	
	// Parse queue metrics
	visibleMessages := parseInt(result.Attributes["ApproximateNumberOfMessages"])
	invisibleMessages := parseInt(result.Attributes["ApproximateNumberOfMessagesNotVisible"])
	delayedMessages := parseInt(result.Attributes["ApproximateNumberOfMessagesDelayed"])
	
	totalMessages := visibleMessages + invisibleMessages + delayedMessages
	
	// Record queue depth metric
	hm.monitor.RecordSQSQueueDepth(ctx, queueURL, int64(totalMessages))
	
	// Determine health status based on queue depth
	var status HealthStatus
	if totalMessages > 10000 {
		status = HealthStatusCritical
	} else if totalMessages > 1000 {
		status = HealthStatusWarning
	} else {
		status = HealthStatusHealthy
	}
	
	metadata := map[string]interface{}{
		"visibleMessages":   visibleMessages,
		"invisibleMessages": invisibleMessages,
		"delayedMessages":   delayedMessages,
		"totalMessages":     totalMessages,
	}
	
	hm.updateComponentHealth("sqs."+queueURL, status, nil, metadata)
	
	return nil
}

// updateComponentHealth updates the health status of a component
func (hm *HealthMonitor) updateComponentHealth(component string, status HealthStatus, err error, metadata map[string]interface{}) {
	hm.mu.Lock()
	defer hm.mu.Unlock()
	
	health, exists := hm.healthStatus[component]
	if !exists {
		health = &ComponentHealth{
			Component: component,
			Metadata:  make(map[string]interface{}),
		}
		hm.healthStatus[component] = health
	}
	
	health.Status = status
	health.LastCheck = time.Now()
	
	if err != nil {
		health.ErrorCount++
		health.LastError = err
	} else {
		health.ErrorCount = 0
		health.LastError = nil
	}
	
	if metadata != nil {
		health.Metadata = metadata
	}
}

// GetHealthStatus returns the current health status
func (hm *HealthMonitor) GetHealthStatus() map[string]ComponentHealth {
	hm.mu.RLock()
	defer hm.mu.RUnlock()
	
	// Create a copy to avoid holding the lock
	statusCopy := make(map[string]ComponentHealth)
	for component, health := range hm.healthStatus {
		statusCopy[component] = *health
	}
	
	return statusCopy
}

// GetOverallHealth returns the overall system health
func (hm *HealthMonitor) GetOverallHealth() HealthStatus {
	hm.mu.RLock()
	defer hm.mu.RUnlock()
	
	worstStatus := HealthStatusHealthy
	
	for _, health := range hm.healthStatus {
		if health.Status == HealthStatusCritical {
			return HealthStatusCritical
		}
		if health.Status == HealthStatusWarning && worstStatus == HealthStatusHealthy {
			worstStatus = HealthStatusWarning
		}
		if health.Status == HealthStatusUnknown && worstStatus == HealthStatusHealthy {
			worstStatus = HealthStatusUnknown
		}
	}
	
	return worstStatus
}

// StartHealthChecks starts periodic health checks
func (hm *HealthMonitor) StartHealthChecks(ctx context.Context, interval time.Duration, components []HealthCheckComponent) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	
	// Run initial checks
	hm.runHealthChecks(ctx, components)
	
	for {
		select {
		case <-ticker.C:
			hm.runHealthChecks(ctx, components)
		case <-ctx.Done():
			return
		}
	}
}

// HealthCheckComponent defines a component to monitor
type HealthCheckComponent struct {
	Type       string // "dynamodb", "lambda", "sqs"
	Identifier string // table name, function name, or queue URL
}

// runHealthChecks runs health checks for all components
func (hm *HealthMonitor) runHealthChecks(ctx context.Context, components []HealthCheckComponent) {
	for _, component := range components {
		switch component.Type {
		case "dynamodb":
			hm.CheckDynamoDBHealth(ctx, component.Identifier)
		case "lambda":
			hm.CheckLambdaHealth(ctx, component.Identifier)
		case "sqs":
			hm.CheckSQSHealth(ctx, component.Identifier)
		}
	}
	
	// Record overall health metric
	overallHealth := hm.GetOverallHealth()
	healthValue := 0.0
	switch overallHealth {
	case HealthStatusHealthy:
		healthValue = 1.0
	case HealthStatusWarning:
		healthValue = 0.5
	case HealthStatusCritical:
		healthValue = 0.0
	}
	
	hm.monitor.putMetric(ctx, MetricData{
		Name:  "SystemHealth",
		Value: healthValue,
		Unit:  "None",
		Dimensions: map[string]string{
			"Environment": hm.monitor.environment,
		},
	})
}

// parseInt safely parses a string to int
func parseInt(s string) int {
	var result int
	fmt.Sscanf(s, "%d", &result)
	return result
}
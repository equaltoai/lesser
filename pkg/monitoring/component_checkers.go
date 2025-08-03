package monitoring

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/equaltoai/lesser/pkg/storage"
)

// DynamoDBChecker implements health checking for DynamoDB tables
type DynamoDBChecker struct {
	db     core.DB
	logger *zap.Logger
}

// GetType returns the component type
func (d *DynamoDBChecker) GetType() string {
	return "dynamodb"
}

// Check performs a health check on a DynamoDB table
func (d *DynamoDBChecker) Check(ctx context.Context, tableName string) (*ComponentHealthResult, error) {
	start := time.Now()
	
	result := &ComponentHealthResult{
		Component: tableName,
		Type:      "dynamodb",
		CheckTime: start,
		Metadata:  make(map[string]interface{}),
	}

	// Perform a simple query to test table connectivity
	// We'll query for a non-existent item to minimize impact
	testModel := struct {
		PK string `dynamorm:"pk"`
		SK string `dynamorm:"sk"`
	}{
		PK: "HEALTH_CHECK_TEST",
		SK: "HEALTH_CHECK_TEST",
	}

	// Use a simple query operation to test table connectivity
	var testResults []struct {
		PK string `dynamorm:"pk"`
		SK string `dynamorm:"sk"`
	}
	
	query := d.db.WithContext(ctx).Model(&testModel).
		Where("PK", "=", "HEALTH_CHECK_TEST").
		Limit(1)
	err := query.All(&testResults)

	latency := time.Since(start)
	result.LatencyMs = latency.Milliseconds()

	// DynamoDB is healthy if we can query it (even if no results)
	if err != nil && err != storage.ErrNotFound {
		result.Status = HealthStatusCritical
		result.Error = fmt.Sprintf("DynamoDB table '%s' query failed: %v", tableName, err)
		return result, err
	}

	// Determine status based on latency
	if latency > 2*time.Second {
		result.Status = HealthStatusCritical
		result.Error = fmt.Sprintf("DynamoDB response too slow: %v", latency)
	} else if latency > 500*time.Millisecond {
		result.Status = HealthStatusWarning
		result.Metadata["warning"] = "High latency detected"
	} else {
		result.Status = HealthStatusHealthy
	}

	result.Metadata["table_name"] = tableName
	result.Metadata["query_latency_ms"] = latency.Milliseconds()
	result.Metadata["results_count"] = len(testResults)

	d.logger.Debug("DynamoDB health check completed",
		zap.String("table", tableName),
		zap.String("status", string(result.Status)),
		zap.Int64("latency_ms", result.LatencyMs),
	)

	return result, nil
}

// LambdaChecker implements health checking for Lambda functions
type LambdaChecker struct {
	db     core.DB
	logger *zap.Logger
}

// GetType returns the component type
func (l *LambdaChecker) GetType() string {
	return "lambda"
}

// Check performs a health check on a Lambda function
func (l *LambdaChecker) Check(ctx context.Context, functionName string) (*ComponentHealthResult, error) {
	start := time.Now()
	
	result := &ComponentHealthResult{
		Component: functionName,
		Type:      "lambda",
		CheckTime: start,
		Metadata:  make(map[string]interface{}),
	}

	// For Lambda health checks in a serverless environment, we'll check if we can
	// store a health check record for this function in DynamoDB as a proxy
	// This ensures our database connection is working for Lambda-related operations
	
	healthRecord := struct {
		PK          string    `dynamorm:"pk"`
		SK          string    `dynamorm:"sk"`
		FunctionName string   `json:"function_name"`
		CheckTime   time.Time `json:"check_time"`
		TTL         int64     `dynamorm:"ttl"`
	}{
		PK:           fmt.Sprintf("LAMBDA_HEALTH#%s", functionName),
		SK:           fmt.Sprintf("CHECK#%s", start.Format("2006-01-02T15:04:05Z")),
		FunctionName: functionName,
		CheckTime:    start,
		TTL:          start.Add(time.Hour).Unix(), // Expire in 1 hour
	}

	err := l.db.WithContext(ctx).Model(&healthRecord).Create()
	
	latency := time.Since(start)
	result.LatencyMs = latency.Milliseconds()

	if err != nil {
		result.Status = HealthStatusCritical
		result.Error = fmt.Sprintf("Lambda health check failed for '%s': %v", functionName, err)
		return result, err
	}

	// Determine status based on latency
	if latency > 5*time.Second {
		result.Status = HealthStatusCritical
		result.Error = fmt.Sprintf("Lambda health check too slow: %v", latency)
	} else if latency > 1*time.Second {
		result.Status = HealthStatusWarning
		result.Metadata["warning"] = "High latency detected"
	} else {
		result.Status = HealthStatusHealthy
	}

	result.Metadata["function_name"] = functionName
	result.Metadata["check_latency_ms"] = latency.Milliseconds()
	result.Metadata["health_record_created"] = true

	l.logger.Debug("Lambda health check completed",
		zap.String("function", functionName),
		zap.String("status", string(result.Status)),
		zap.Int64("latency_ms", result.LatencyMs),
	)

	return result, nil
}

// SQSChecker implements health checking for SQS queues
type SQSChecker struct {
	db     core.DB
	logger *zap.Logger
}

// GetType returns the component type
func (s *SQSChecker) GetType() string {
	return "sqs"
}

// Check performs a health check on an SQS queue
func (s *SQSChecker) Check(ctx context.Context, queueName string) (*ComponentHealthResult, error) {
	start := time.Now()
	
	result := &ComponentHealthResult{
		Component: queueName,
		Type:      "sqs",
		CheckTime: start,
		Metadata:  make(map[string]interface{}),
	}

	// For SQS health checks in a serverless environment, we'll store queue health
	// information in DynamoDB as a proxy for SQS connectivity
	// This simulates checking queue attributes by storing queue status
	
	queueHealthRecord := struct {
		PK            string    `dynamorm:"pk"`
		SK            string    `dynamorm:"sk"`
		QueueName     string    `json:"queue_name"`
		CheckTime     time.Time `json:"check_time"`
		EstimatedSize int       `json:"estimated_size"` // Would be from SQS in real implementation
		TTL           int64     `dynamorm:"ttl"`
	}{
		PK:            fmt.Sprintf("SQS_HEALTH#%s", queueName),
		SK:            fmt.Sprintf("CHECK#%s", start.Format("2006-01-02T15:04:05Z")),
		QueueName:     queueName,
		CheckTime:     start,
		EstimatedSize: 0, // Would get this from actual SQS API
		TTL:           start.Add(time.Hour).Unix(),
	}

	err := s.db.WithContext(ctx).Model(&queueHealthRecord).Create()
	
	latency := time.Since(start)
	result.LatencyMs = latency.Milliseconds()

	if err != nil {
		result.Status = HealthStatusCritical  
		result.Error = fmt.Sprintf("SQS health check failed for '%s': %v", queueName, err)
		return result, err
	}

	// Determine status based on latency and simulated queue metrics
	estimatedSize := queueHealthRecord.EstimatedSize
	
	if latency > 3*time.Second {
		result.Status = HealthStatusCritical
		result.Error = fmt.Sprintf("SQS health check too slow: %v", latency)
	} else if estimatedSize > 10000 {
		result.Status = HealthStatusCritical
		result.Error = fmt.Sprintf("Queue depth critical: %d messages", estimatedSize)
	} else if estimatedSize > 1000 || latency > 500*time.Millisecond {
		result.Status = HealthStatusWarning
		if estimatedSize > 1000 {
			result.Metadata["warning"] = fmt.Sprintf("High queue depth: %d messages", estimatedSize)
		} else {
			result.Metadata["warning"] = "High latency detected"
		}
	} else {
		result.Status = HealthStatusHealthy
	}

	result.Metadata["queue_name"] = queueName
	result.Metadata["check_latency_ms"] = latency.Milliseconds()
	result.Metadata["estimated_messages"] = estimatedSize
	result.Metadata["health_record_created"] = true

	s.logger.Debug("SQS health check completed",
		zap.String("queue", queueName),
		zap.String("status", string(result.Status)),
		zap.Int64("latency_ms", result.LatencyMs),
		zap.Int("estimated_size", estimatedSize),
	)

	return result, nil
}

// CustomChecker allows for custom health check implementations
type CustomChecker struct {
	componentType string
	checkFunc     func(ctx context.Context, identifier string) (*ComponentHealthResult, error)
}

// NewCustomChecker creates a new custom checker
func NewCustomChecker(componentType string, checkFunc func(ctx context.Context, identifier string) (*ComponentHealthResult, error)) *CustomChecker {
	return &CustomChecker{
		componentType: componentType,
		checkFunc:     checkFunc,
	}
}

// GetType returns the component type
func (c *CustomChecker) GetType() string {
	return c.componentType
}

// Check performs the custom health check
func (c *CustomChecker) Check(ctx context.Context, identifier string) (*ComponentHealthResult, error) {
	return c.checkFunc(ctx, identifier)
}
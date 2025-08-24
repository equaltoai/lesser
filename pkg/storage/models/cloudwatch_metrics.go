package models

import (
	"fmt"
	"time"
)

// CloudWatchMetrics represents CloudWatch metrics data cached in DynamoDB
type CloudWatchMetrics struct {
	// DynamoDB Keys
	PK string `dynamorm:"pk" json:"pk"` // SERVICE#{serviceName} or INSTANCE#lesser
	SK string `dynamorm:"sk" json:"sk"` // METRICS#{timestamp}

	// GSI keys for time-based queries
	GSI1PK string `dynamorm:"index:gsi1,pk" json:"gsi1pk"` // METRIC_DATE#{date}
	GSI1SK string `dynamorm:"index:gsi1,sk" json:"gsi1sk"` // {serviceName}#{timestamp}

	// Business fields
	ServiceName string    `json:"service_name"`
	Timestamp   time.Time `json:"timestamp"`
	Date        string    `json:"date"` // YYYY-MM-DD for daily queries

	// Metrics data
	RequestCount      int64   `json:"request_count"`
	ErrorCount        int64   `json:"error_count"`
	LatencyP50Ms      float64 `json:"latency_p50_ms"`
	LatencyP90Ms      float64 `json:"latency_p90_ms"`
	LatencyP99Ms      float64 `json:"latency_p99_ms"`
	DynamoDBReads     int64   `json:"dynamodb_reads"`
	DynamoDBWrites    int64   `json:"dynamodb_writes"`
	LambdaInvocations int64   `json:"lambda_invocations"`
	S3Requests        int64   `json:"s3_requests"`
	DataTransferBytes int64   `json:"data_transfer_bytes"`
	EstimatedCostUSD  float64 `json:"estimated_cost_usd"`

	// Caching metadata
	CloudWatchQueryTime time.Time `json:"cloudwatch_query_time"`
	CacheExpiry         time.Time `json:"cache_expiry"`

	// TTL for automatic cleanup (24 hours)
	TTL int64 `json:"ttl,omitempty" dynamorm:"ttl"`
}

// UpdateKeys sets the GSI keys based on the current values
func (c *CloudWatchMetrics) UpdateKeys() error {
	if c.ServiceName == "" {
		return ErrCloudWatchMetricServiceNameRequired
	}

	// Set date if not already set
	if c.Date == "" {
		c.Date = c.Timestamp.Format("2006-01-02")
	}

	// Set primary keys
	c.PK = fmt.Sprintf("SERVICE#%s", c.ServiceName)
	c.SK = fmt.Sprintf("METRICS#%s", c.Timestamp.Format(time.RFC3339))

	// Set GSI1 keys for time-based queries
	c.GSI1PK = fmt.Sprintf("METRIC_DATE#%s", c.Date)
	c.GSI1SK = fmt.Sprintf("%s#%s", c.ServiceName, c.Timestamp.Format(time.RFC3339))

	// Set TTL (24 hours from now)
	c.TTL = time.Now().Add(24 * time.Hour).Unix()

	return nil
}

// GetPK returns the partition key
func (c *CloudWatchMetrics) GetPK() string {
	return c.PK
}

// GetSK returns the sort key
func (c *CloudWatchMetrics) GetSK() string {
	return c.SK
}

// IsExpired checks if the cached metrics have expired (5 minutes cache)
func (c *CloudWatchMetrics) IsExpired() bool {
	return time.Now().After(c.CacheExpiry)
}

// SetCacheExpiry sets cache expiry time (default 5 minutes for metrics)
func (c *CloudWatchMetrics) SetCacheExpiry() {
	c.CacheExpiry = time.Now().Add(5 * time.Minute)
	c.CloudWatchQueryTime = time.Now()
}

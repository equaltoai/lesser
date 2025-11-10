package models

import (
	"fmt"
	"time"
)

// CloudWatchMetrics represents CloudWatch metrics data cached in DynamoDB
type CloudWatchMetrics struct {
	_ struct{} `dynamorm:"naming:camelCase"`

	// DynamoDB Keys
	PK string `dynamorm:"pk,attr:PK" json:"pk"` // SERVICE#{serviceName} or INSTANCE#lesser
	SK string `dynamorm:"sk,attr:SK" json:"sk"` // METRICS#{timestamp}

	// GSI keys for time-based queries
	GSI1PK string `dynamorm:"index:gsi1,pk,attr:gsi1PK" json:"gsi1pk"` // METRIC_DATE#{date}
	GSI1SK string `dynamorm:"index:gsi1,sk,attr:gsi1SK" json:"gsi1sk"` // {serviceName}#{timestamp}

	// Business fields
	ServiceName string    `dynamorm:"attr:serviceName" json:"service_name"`
	Timestamp   time.Time `dynamorm:"attr:timestamp" json:"timestamp"`
	Date        string    `dynamorm:"attr:date" json:"date"` // YYYY-MM-DD for daily queries

	// Metrics data
	RequestCount      int64   `dynamorm:"attr:requestCount" json:"request_count"`
	ErrorCount        int64   `dynamorm:"attr:errorCount" json:"error_count"`
	LatencyP50Ms      float64 `dynamorm:"attr:latencyP50Ms" json:"latency_p50_ms"`
	LatencyP90Ms      float64 `dynamorm:"attr:latencyP90Ms" json:"latency_p90_ms"`
	LatencyP99Ms      float64 `dynamorm:"attr:latencyP99Ms" json:"latency_p99_ms"`
	DynamoDBReads     int64   `dynamorm:"attr:dynamoDBReads" json:"dynamodb_reads"`
	DynamoDBWrites    int64   `dynamorm:"attr:dynamoDBWrites" json:"dynamodb_writes"`
	LambdaInvocations int64   `dynamorm:"attr:lambdaInvocations" json:"lambda_invocations"`
	S3Requests        int64   `dynamorm:"attr:s3Requests" json:"s3_requests"`
	DataTransferBytes int64   `dynamorm:"attr:dataTransferBytes" json:"data_transfer_bytes"`
	EstimatedCostUSD  float64 `dynamorm:"attr:estimatedCostUSD" json:"estimated_cost_usd"`

	// Caching metadata
	CloudWatchQueryTime time.Time `dynamorm:"attr:cloudWatchQueryTime" json:"cloudwatch_query_time"`
	CacheExpiry         time.Time `dynamorm:"attr:cacheExpiry" json:"cache_expiry"`

	// TTL for automatic cleanup (24 hours)
	TTL int64 `dynamorm:"ttl,attr:ttl" json:"ttl,omitempty"`
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

// TableName returns the DynamoDB table backing CloudWatchMetrics.
func (CloudWatchMetrics) TableName() string {
	return MainTableName
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

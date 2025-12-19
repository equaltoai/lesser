package models

import (
	"fmt"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/google/uuid"
)

// DynamoDBCostRecord represents detailed cost tracking data from DynamoDB operations
type DynamoDBCostRecord struct {
	_ struct{} `dynamorm:"naming:camelCase"`

	// Primary key - using operation type as partition key with timestamp sort key
	PK string `dynamorm:"pk,attr:PK" json:"pk"` // Format: "cost#{operation_type}"
	SK string `dynamorm:"sk,attr:SK" json:"sk"` // Format: "ts#{timestamp}#{id}"

	// GSI1 - Table queries
	GSI1PK string `dynamorm:"index:gsi1,pk,attr:gsi1PK" json:"gsi1_pk"` // Format: "COST_TABLE#{table_name}"
	GSI1SK string `dynamorm:"index:gsi1,sk,attr:gsi1SK" json:"gsi1_sk"` // Format: "{timestamp}#{operation_type}#{id}"

	// GSI2 - Aggregation queries
	GSI2PK string `dynamorm:"index:gsi2,pk,attr:gsi2PK" json:"gsi2_pk"` // Format: "COST_AGG#{period}#{operation_type}"
	GSI2SK string `dynamorm:"index:gsi2,sk,attr:gsi2SK" json:"gsi2_sk"` // Format: "{timestamp}#{id}"

	// Core cost tracking data
	ID            string    `dynamorm:"attr:id" json:"id"`
	OperationType string    `dynamorm:"attr:operationType" json:"operation_type"` // GetItem, PutItem, Query, Scan, BatchWrite, etc.
	Table         string    `dynamorm:"attr:tableName" json:"table_name"`
	Timestamp     time.Time `dynamorm:"attr:timestamp" json:"timestamp"`
	Period        string    `dynamorm:"attr:period" json:"period"` // minute, hour, day

	// Capacity units consumed
	ReadCapacityUnits  float64 `dynamorm:"attr:readCapacityUnits" json:"read_capacity_units"`
	WriteCapacityUnits float64 `dynamorm:"attr:writeCapacityUnits" json:"write_capacity_units"`

	// Cost calculations (in microcents for precision)
	ReadCostMicroCents  int64 `dynamorm:"attr:readCostMicroCents" json:"read_cost_micro_cents"`
	WriteCostMicroCents int64 `dynamorm:"attr:writeCostMicroCents" json:"write_cost_micro_cents"`
	TotalCostMicroCents int64 `dynamorm:"attr:totalCostMicroCents" json:"total_cost_micro_cents"`

	// Estimated cost in dollars for easy display
	EstimatedCostDollars float64 `dynamorm:"attr:estimatedCostDollars" json:"estimated_cost_dollars"`

	// Operation details
	ItemCount       int    `dynamorm:"attr:itemCount" json:"item_count"`             // Number of items in operation
	RequestDuration int64  `dynamorm:"attr:requestDuration" json:"request_duration"` // Duration in milliseconds
	IndexName       string `dynamorm:"attr:indexName" json:"index_name,omitempty"`   // GSI name if used
	ConsistentRead  bool   `dynamorm:"attr:consistentRead" json:"consistent_read"`

	// Service and function information
	ServiceName     string `dynamorm:"attr:serviceName" json:"service_name"`         // Lambda function or service
	RequestID       string `dynamorm:"attr:requestID" json:"request_id"`             // AWS Request ID
	FunctionName    string `dynamorm:"attr:functionName" json:"function_name"`       // Lambda function name
	FunctionVersion string `dynamorm:"attr:functionVersion" json:"function_version"` // Lambda function version

	// Additional metadata
	Tags       map[string]string      `dynamorm:"attr:tags" json:"tags,omitempty"`
	Properties map[string]interface{} `dynamorm:"attr:properties" json:"properties,omitempty"`

	// Timestamps
	CreatedAt time.Time `dynamorm:"attr:createdAt" json:"created_at"`
	UpdatedAt time.Time `dynamorm:"attr:updatedAt" json:"updated_at"`

	// TTL for automatic cleanup (30 days for raw, 90 days for aggregated)
	ExpiresAt int64 `dynamorm:"ttl,attr:ttl" json:"expires_at"` // Unix timestamp
}

// DynamoDBCostAggregation represents pre-computed cost aggregations
type DynamoDBCostAggregation struct {
	_ struct{} `dynamorm:"naming:camelCase"`

	// Primary key
	PK string `dynamorm:"pk,attr:PK" json:"pk"` // Format: "cost_agg#{period}#{operation_type}"
	SK string `dynamorm:"sk,attr:SK" json:"sk"` // Format: "window#{windowStart}"

	// Aggregation details
	Period        string    `dynamorm:"attr:period" json:"period"`                // minute, hour, day, week, month
	OperationType string    `dynamorm:"attr:operationType" json:"operation_type"` // Same as CostTracking.OperationType
	Table         string    `dynamorm:"attr:tableName" json:"table_name"`         // Specific table or "all" for all tables
	WindowStart   time.Time `dynamorm:"attr:windowStart" json:"window_start"`     // Start of aggregation window
	WindowEnd     time.Time `dynamorm:"attr:windowEnd" json:"window_end"`         // End of aggregation window

	// Aggregated capacity units
	TotalReadCapacityUnits  float64 `dynamorm:"attr:totalReadCapacityUnits" json:"total_read_capacity_units"`
	TotalWriteCapacityUnits float64 `dynamorm:"attr:totalWriteCapacityUnits" json:"total_write_capacity_units"`

	// Aggregated costs
	TotalReadCostMicroCents  int64   `dynamorm:"attr:totalReadCostMicroCents" json:"total_read_cost_micro_cents"`
	TotalWriteCostMicroCents int64   `dynamorm:"attr:totalWriteCostMicroCents" json:"total_write_cost_micro_cents"`
	TotalCostMicroCents      int64   `dynamorm:"attr:totalCostMicroCents" json:"total_cost_micro_cents"`
	TotalCostDollars         float64 `dynamorm:"attr:totalCostDollars" json:"total_cost_dollars"`

	// Operation statistics
	TotalOperations         int64   `dynamorm:"attr:totalOperations" json:"total_operations"`
	TotalItemCount          int64   `dynamorm:"attr:totalItemCount" json:"total_item_count"`
	AverageCostPerOperation float64 `dynamorm:"attr:averageCostPerOperation" json:"average_cost_per_operation"`
	AverageDuration         float64 `dynamorm:"attr:averageDuration" json:"average_duration"` // milliseconds

	// Cost breakdown by table
	TableBreakdown map[string]*DynamoDBTableCostStats `dynamorm:"attr:tableBreakdown" json:"table_breakdown,omitempty"`

	// Cost breakdown by service
	ServiceBreakdown map[string]*DynamoDBServiceCostStats `dynamorm:"attr:serviceBreakdown" json:"service_breakdown,omitempty"`

	// Percentiles for cost distribution
	CostPercentiles map[string]float64 `dynamorm:"attr:costPercentiles" json:"cost_percentiles,omitempty"` // p50, p90, p95, p99

	// Timestamps
	CreatedAt time.Time `dynamorm:"attr:createdAt" json:"created_at"`
	UpdatedAt time.Time `dynamorm:"attr:updatedAt" json:"updated_at"`

	// TTL (longer for aggregated data)
	ExpiresAt int64 `dynamorm:"ttl,attr:ttl" json:"expires_at"`
}

// DynamoDBTableCostStats represents cost statistics for a specific table
type DynamoDBTableCostStats struct {
	Table               string  `dynamorm:"attr:tableName" json:"table_name"`
	OperationCount      int64   `dynamorm:"attr:operationCount" json:"operation_count"`
	ReadCapacityUnits   float64 `dynamorm:"attr:readCapacityUnits" json:"read_capacity_units"`
	WriteCapacityUnits  float64 `dynamorm:"attr:writeCapacityUnits" json:"write_capacity_units"`
	TotalCostMicroCents int64   `dynamorm:"attr:totalCostMicroCents" json:"total_cost_micro_cents"`
	TotalCostDollars    float64 `dynamorm:"attr:totalCostDollars" json:"total_cost_dollars"`
	UniqueUsers         int64   `dynamorm:"attr:uniqueUsers" json:"unique_users"` // Number of unique users for this table
}

// DynamoDBServiceCostStats represents cost statistics for a specific service
type DynamoDBServiceCostStats struct {
	ServiceName         string  `dynamorm:"attr:serviceName" json:"service_name"`
	OperationCount      int64   `dynamorm:"attr:operationCount" json:"operation_count"`
	TotalCostMicroCents int64   `dynamorm:"attr:totalCostMicroCents" json:"total_cost_micro_cents"`
	TotalCostDollars    float64 `dynamorm:"attr:totalCostDollars" json:"total_cost_dollars"`
	AverageCostPerOp    float64 `dynamorm:"attr:averageCostPerOp" json:"average_cost_per_op"`
	DataTransferBytes   int64   `dynamorm:"attr:dataTransferBytes" json:"data_transfer_bytes"` // Data transfer for this service
}

// TableName returns the DynamoDB table backing DynamoDBCostRecord.
func (DynamoDBCostRecord) TableName() string {
	return MainTableName
}

// TableName returns the DynamoDB table backing DynamoDBCostAggregation.
func (DynamoDBCostAggregation) TableName() string {
	return MainTableName
}

// TableName returns the DynamoDB table backing DynamoDBTableCostStats.
func (DynamoDBTableCostStats) TableName() string {
	return MainTableName
}

// TableName returns the DynamoDB table backing DynamoDBServiceCostStats.
func (DynamoDBServiceCostStats) TableName() string {
	return MainTableName
}

// GetPK returns the partition key
func (ct *DynamoDBCostRecord) GetPK() string {
	return ct.PK
}

// GetSK returns the sort key
func (ct *DynamoDBCostRecord) GetSK() string {
	return ct.SK
}

// UpdateKeys sets up all the keys for the record
func (ct *DynamoDBCostRecord) UpdateKeys() error {
	// Generate ID if not provided
	if strings.TrimSpace(ct.ID) == "" {
		ct.ID = uuid.New().String()
	}

	// Set timestamp if not provided
	if ct.Timestamp.IsZero() {
		ct.Timestamp = time.Now()
	}

	// Set up primary key
	ct.PK = fmt.Sprintf("cost#%s", ct.OperationType)
	timestamp := ct.Timestamp.Format("20060102150405")
	ct.SK = fmt.Sprintf("ts#%s#%s", timestamp, ct.ID)

	// Set up GSI keys
	ct.setupGSIKeys()

	return nil
}

// BeforeCreate sets up the model before creation
func (ct *DynamoDBCostRecord) BeforeCreate() error {
	now := time.Now()
	ct.CreatedAt = now
	ct.UpdatedAt = now

	// Generate ID if not provided
	if err := common.ValidateRequiredParam("ID", ct.ID); err != nil {
		ct.ID = uuid.New().String()
	}

	// Set timestamp if not provided
	if ct.Timestamp.IsZero() {
		ct.Timestamp = now
	}

	// Calculate estimated cost in dollars
	ct.EstimatedCostDollars = float64(ct.TotalCostMicroCents) / 1_000_000.0

	// Set TTL based on period
	ttlDays := 30 // Default for raw cost data
	if ct.Period == PeriodHour || ct.Period == PeriodDay {
		ttlDays = 90 // Keep aggregated data longer
	}
	ct.ExpiresAt = now.Add(time.Duration(ttlDays) * 24 * time.Hour).Unix()

	// Update keys using the new method
	if err := ct.UpdateKeys(); err != nil {
		return err
	}

	return ct.Validate()
}

// BeforeUpdate sets up the model before update
func (ct *DynamoDBCostRecord) BeforeUpdate() error {
	ct.UpdatedAt = time.Now()

	// Recalculate estimated cost
	ct.EstimatedCostDollars = float64(ct.TotalCostMicroCents) / 1_000_000.0

	// Update GSI keys in case indexed fields changed
	ct.setupGSIKeys()

	return ct.Validate()
}

// setupGSIKeys configures all GSI partition and sort keys
func (ct *DynamoDBCostRecord) setupGSIKeys() {
	timestampStr := ct.Timestamp.Format(time.RFC3339)

	// GSI1 - Table queries
	ct.GSI1PK = fmt.Sprintf("COST_TABLE#%s", ct.Table)
	ct.GSI1SK = fmt.Sprintf("%s#%s#%s", timestampStr, ct.OperationType, ct.ID)

	// GSI2 - Aggregation queries
	ct.GSI2PK = fmt.Sprintf("COST_AGG#%s#%s", ct.Period, ct.OperationType)
	ct.GSI2SK = fmt.Sprintf("%s#%s", timestampStr, ct.ID)
}

// Validate performs validation on the DynamoDBCostRecord
func (ct *DynamoDBCostRecord) Validate() error {
	if err := common.ValidateRequiredParam("ID", strings.TrimSpace(ct.ID)); err != nil {
		return err
	}
	if err := common.ValidateRequiredParam("OperationType", strings.TrimSpace(ct.OperationType)); err != nil {
		return err
	}
	if err := common.ValidateRequiredParam("table", strings.TrimSpace(ct.Table)); err != nil {
		return err
	}
	if !isValidOperationType(ct.OperationType) {
		return fmt.Errorf("%w: %s", ErrInvalidOperationType, ct.OperationType)
	}
	if ct.Period != "" && !isValidPeriod(ct.Period) {
		return fmt.Errorf("%w: %s", ErrInvalidCostPeriod, ct.Period)
	}

	return nil
}

// GetPK returns the partition key
func (act *DynamoDBCostAggregation) GetPK() string {
	return act.PK
}

// GetSK returns the sort key
func (act *DynamoDBCostAggregation) GetSK() string {
	return act.SK
}

// UpdateKeys sets up all the keys for the aggregation record
func (act *DynamoDBCostAggregation) UpdateKeys() error {
	// Set up primary key
	act.PK = fmt.Sprintf("cost_agg#%s#%s", act.Period, act.OperationType)
	act.SK = fmt.Sprintf("window#%s", act.WindowStart.Format(time.RFC3339))
	return nil
}

// BeforeCreate for DynamoDBCostAggregation
func (act *DynamoDBCostAggregation) BeforeCreate() error {
	now := time.Now()
	act.CreatedAt = now
	act.UpdatedAt = now

	// Calculate total cost in dollars
	act.TotalCostDollars = float64(act.TotalCostMicroCents) / 1_000_000.0

	// Calculate average cost per operation
	if act.TotalOperations > 0 {
		act.AverageCostPerOperation = act.TotalCostDollars / float64(act.TotalOperations)
	}

	// Set TTL (keep aggregated data longer)
	ttlDays := 90
	if act.Period == PeriodMonth {
		ttlDays = 365 // Keep monthly data for a year
	}
	act.ExpiresAt = now.Add(time.Duration(ttlDays) * 24 * time.Hour).Unix()

	// Update keys using the new method
	if err := act.UpdateKeys(); err != nil {
		return err
	}

	return act.Validate()
}

// BeforeUpdate for DynamoDBCostAggregation
func (act *DynamoDBCostAggregation) BeforeUpdate() error {
	act.UpdatedAt = time.Now()

	// Recalculate totals
	act.TotalCostDollars = float64(act.TotalCostMicroCents) / 1_000_000.0
	if act.TotalOperations > 0 {
		act.AverageCostPerOperation = act.TotalCostDollars / float64(act.TotalOperations)
	}

	return act.Validate()
}

// Validate for DynamoDBCostAggregation
func (act *DynamoDBCostAggregation) Validate() error {
	if err := common.ValidateRequiredParam("OperationType", strings.TrimSpace(act.OperationType)); err != nil {
		return err
	}
	if err := common.ValidateRequiredParam("period", strings.TrimSpace(act.Period)); err != nil {
		return err
	}
	if act.WindowStart.IsZero() {
		return ErrCostWindowStartRequired
	}
	if act.WindowEnd.IsZero() {
		return ErrCostWindowEndRequired
	}
	if act.WindowEnd.Before(act.WindowStart) {
		return ErrCostWindowEndBeforeStart
	}

	return nil
}

// AddTag adds a tag to the cost tracking
func (ct *DynamoDBCostRecord) AddTag(key, value string) {
	if ct.Tags == nil {
		ct.Tags = make(map[string]string)
	}
	ct.Tags[key] = value
}

// SetProperty sets a custom property
func (ct *DynamoDBCostRecord) SetProperty(key string, value interface{}) {
	if ct.Properties == nil {
		ct.Properties = make(map[string]interface{})
	}
	ct.Properties[key] = value
}

// GetProperty gets a custom property
func (ct *DynamoDBCostRecord) GetProperty(key string) (interface{}, bool) {
	if ct.Properties == nil {
		return nil, false
	}
	value, exists := ct.Properties[key]
	return value, exists
}

// isValidOperationType checks if the operation type is valid
func isValidOperationType(opType string) bool {
	validTypes := map[string]bool{
		"GetItem":            true,
		"PutItem":            true,
		"UpdateItem":         true,
		"DeleteItem":         true,
		"Query":              true,
		"Scan":               true,
		"BatchGetItem":       true,
		"BatchWriteItem":     true,
		"TransactGetItems":   true,
		"TransactWriteItems": true,
	}
	return validTypes[opType]
}

// DynamoDBCostRecordBuilder helps create cost tracking records
type DynamoDBCostRecordBuilder struct {
	tracking *DynamoDBCostRecord
}

// TableName returns the DynamoDB table backing DynamoDBCostRecordBuilder.
func (DynamoDBCostRecordBuilder) TableName() string {
	return MainTableName
}

// NewDynamoDBCostRecordBuilder creates a new cost tracking builder
func NewDynamoDBCostRecordBuilder() *DynamoDBCostRecordBuilder {
	return &DynamoDBCostRecordBuilder{
		tracking: &DynamoDBCostRecord{
			Tags:       make(map[string]string),
			Properties: make(map[string]interface{}),
		},
	}
}

// ForOperation sets the operation type
func (ctb *DynamoDBCostRecordBuilder) ForOperation(operationType string) *DynamoDBCostRecordBuilder {
	ctb.tracking.OperationType = operationType
	return ctb
}

// OnTable sets the table name
func (ctb *DynamoDBCostRecordBuilder) OnTable(tableName string) *DynamoDBCostRecordBuilder {
	ctb.tracking.Table = tableName
	return ctb
}

// WithCapacityUnits sets the consumed capacity units
func (ctb *DynamoDBCostRecordBuilder) WithCapacityUnits(readUnits, writeUnits float64) *DynamoDBCostRecordBuilder {
	ctb.tracking.ReadCapacityUnits = readUnits
	ctb.tracking.WriteCapacityUnits = writeUnits
	return ctb
}

// WithCostMicroCents sets the cost in microcents
func (ctb *DynamoDBCostRecordBuilder) WithCostMicroCents(readCost, writeCost int64) *DynamoDBCostRecordBuilder {
	ctb.tracking.ReadCostMicroCents = readCost
	ctb.tracking.WriteCostMicroCents = writeCost
	ctb.tracking.TotalCostMicroCents = readCost + writeCost
	return ctb
}

// WithItemCount sets the item count
func (ctb *DynamoDBCostRecordBuilder) WithItemCount(count int) *DynamoDBCostRecordBuilder {
	ctb.tracking.ItemCount = count
	return ctb
}

// WithDuration sets the request duration in milliseconds
func (ctb *DynamoDBCostRecordBuilder) WithDuration(durationMs int64) *DynamoDBCostRecordBuilder {
	ctb.tracking.RequestDuration = durationMs
	return ctb
}

// WithService sets the service information
func (ctb *DynamoDBCostRecordBuilder) WithService(serviceName, functionName string) *DynamoDBCostRecordBuilder {
	ctb.tracking.ServiceName = serviceName
	ctb.tracking.FunctionName = functionName
	return ctb
}

// WithRequestID sets the request ID
func (ctb *DynamoDBCostRecordBuilder) WithRequestID(requestID string) *DynamoDBCostRecordBuilder {
	ctb.tracking.RequestID = requestID
	return ctb
}

// WithIndex sets the index name if a GSI was used
func (ctb *DynamoDBCostRecordBuilder) WithIndex(indexName string) *DynamoDBCostRecordBuilder {
	ctb.tracking.IndexName = indexName
	return ctb
}

// WithConsistentRead sets whether consistent read was used
func (ctb *DynamoDBCostRecordBuilder) WithConsistentRead(consistent bool) *DynamoDBCostRecordBuilder {
	ctb.tracking.ConsistentRead = consistent
	return ctb
}

// WithTag adds a tag
func (ctb *DynamoDBCostRecordBuilder) WithTag(key, value string) *DynamoDBCostRecordBuilder {
	ctb.tracking.AddTag(key, value)
	return ctb
}

// WithPeriod sets the period
func (ctb *DynamoDBCostRecordBuilder) WithPeriod(period string) *DynamoDBCostRecordBuilder {
	ctb.tracking.Period = period
	return ctb
}

// Build creates the cost tracking record
func (ctb *DynamoDBCostRecordBuilder) Build() *DynamoDBCostRecord {
	return ctb.tracking
}

// CalculateDynamoDBCost calculates the cost based on AWS pricing
// Prices are in microcents per unit for precision
const (
	// On-demand pricing (as of 2024)
	// Read: $0.25 per million read request units = 0.025 cents per 1000 = 25 microcents per 1000
	ReadCostMicroCentsPerUnit = 25 // per 1000 units

	// Write: $1.25 per million write request units = 0.125 cents per 1000 = 125 microcents per 1000
	WriteCostMicroCentsPerUnit = 125 // per 1000 units
)

// CalculateCost calculates the cost for the given capacity units
func CalculateCost(readUnits, writeUnits float64) (readCostMicroCents, writeCostMicroCents, totalCostMicroCents int64) {
	// Calculate costs in microcents
	readCostMicroCents = int64((readUnits / 1000.0) * float64(ReadCostMicroCentsPerUnit))
	writeCostMicroCents = int64((writeUnits / 1000.0) * float64(WriteCostMicroCentsPerUnit))
	totalCostMicroCents = readCostMicroCents + writeCostMicroCents
	return
}

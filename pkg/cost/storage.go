package cost

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/theory-cloud/tabletheory/v3/pkg/core"
	"go.uber.org/zap"
)

var (
	costHistoryTableNameMu sync.RWMutex
	costHistoryTableName   string
)

func setCostHistoryTableName(tableName string) {
	costHistoryTableNameMu.Lock()
	costHistoryTableName = tableName
	costHistoryTableNameMu.Unlock()
}

func getCostHistoryTableName() string {
	costHistoryTableNameMu.RLock()
	tableName := costHistoryTableName
	costHistoryTableNameMu.RUnlock()
	return tableName
}

type operationCostRecord struct {
	_ struct{} `theorydb:"naming:camelCase"`

	PK     string `theorydb:"pk,attr:PK"`
	SK     string `theorydb:"sk,attr:SK"`
	GSI1PK string `theorydb:"index:gsi1,pk,attr:gsi1PK"`
	GSI1SK string `theorydb:"index:gsi1,sk,attr:gsi1SK"`

	RequestID           string    `theorydb:"attr:requestID"`
	OperationType       string    `theorydb:"attr:operationType"`
	Timestamp           time.Time `theorydb:"attr:timestamp"`
	TotalCostMicroCents int64     `theorydb:"attr:totalCostMicroCents"`
	DynamoDBReads       int64     `theorydb:"attr:dynamoDBReads"`
	DynamoDBWrites      int64     `theorydb:"attr:dynamoDBWrites"`
	LambdaInvocations   int64     `theorydb:"attr:lambdaInvocations"`
	LambdaDurationMs    int64     `theorydb:"attr:lambdaDurationMs"`
	LambdaMemoryMB      int64     `theorydb:"attr:lambdaMemoryMB"`
	S3Gets              int64     `theorydb:"attr:s3Gets"`
	S3Puts              int64     `theorydb:"attr:s3Puts"`
	DataTransferBytes   int64     `theorydb:"attr:dataTransferBytes"`
	Type                string    `theorydb:"attr:type"`
	TTL                 int64     `theorydb:"ttl,attr:ttl"`
}

func (operationCostRecord) TableName() string {
	return getCostHistoryTableName()
}

// Storage handles persistence of cost data.
//
// It is intentionally TableTheory-backed: Lesser does not use direct DynamoDB SDK calls.
type Storage struct {
	db        core.DB
	tableName string
	logger    *zap.Logger
}

// NewStorage creates a new cost storage instance.
func NewStorage(db core.DB, tableName string, logger *zap.Logger) *Storage {
	setCostHistoryTableName(tableName)
	return &Storage{
		db:        db,
		tableName: tableName,
		logger:    logger,
	}
}

// SaveOperationCost saves a single operation cost record.
func (s *Storage) SaveOperationCost(ctx context.Context, cost *OperationCost) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("cost storage is not initialized")
	}
	if cost == nil {
		return fmt.Errorf("operation cost is nil")
	}

	ts := cost.Timestamp
	if ts.IsZero() {
		ts = time.Now().UTC()
	}

	record := &operationCostRecord{
		PK: fmt.Sprintf("COST#%s", ts.Format(common.DateFormat)),
		SK: fmt.Sprintf("%d#%s", ts.UnixNano(), cost.RequestID),

		GSI1PK: fmt.Sprintf("COST#%s", ts.Format(common.MonthFormat)),
		GSI1SK: fmt.Sprintf("%d", ts.UnixNano()),

		RequestID:           cost.RequestID,
		OperationType:       cost.OperationType,
		Timestamp:           ts,
		TotalCostMicroCents: cost.TotalCostMicroCents,
		DynamoDBReads:       cost.DynamoDBReads,
		DynamoDBWrites:      cost.DynamoDBWrites,
		LambdaInvocations:   cost.LambdaInvocations,
		LambdaDurationMs:    cost.LambdaDurationMs,
		LambdaMemoryMB:      cost.LambdaMemoryMB,
		S3Gets:              cost.S3Gets,
		S3Puts:              cost.S3Puts,
		DataTransferBytes:   cost.DataTransferBytes,
		Type:                "operation",
		TTL:                 ts.Add(90 * 24 * time.Hour).Unix(),
	}

	if err := s.db.WithContext(ctx).Model(record).Create(); err != nil {
		if s.logger != nil {
			s.logger.Error("failed to save operation cost",
				zap.String("request_id", cost.RequestID),
				zap.String("table", s.tableName),
				zap.Error(err),
			)
		}
		return fmt.Errorf("save operation cost: %w", err)
	}

	return nil
}

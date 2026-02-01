package cost

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/pkg/mocks"
	"go.uber.org/zap"
)

func TestNewStorage_SetsFields(t *testing.T) {
	t.Parallel()

	db := new(mocks.MockDB)
	logger := zap.NewNop()
	st := NewStorage(db, "cost-table", logger)

	require.NotNil(t, st)
	require.Equal(t, db, st.db)
	require.Equal(t, "cost-table", st.tableName)
	require.Equal(t, logger, st.logger)
}

func TestStorage_SaveOperationCost_WritesRecordViaTableTheory(t *testing.T) {
	t.Parallel()

	now := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)

	db := new(mocks.MockDB)
	query := new(mocks.MockQuery)

	db.On("WithContext", mock.Anything).Return(db)
	db.On("Model", mock.MatchedBy(func(model any) bool {
		record, ok := model.(*operationCostRecord)
		if !ok {
			return false
		}

		if record.PK != "COST#2024-01-02" {
			return false
		}
		if record.GSI1PK != "COST#2024-01" {
			return false
		}
		if record.RequestID != "req" {
			return false
		}
		if record.OperationType != "op" {
			return false
		}
		if record.Timestamp != now {
			return false
		}
		if record.TotalCostMicroCents != 123 {
			return false
		}
		if record.Type != "operation" {
			return false
		}
		if record.TTL <= now.Unix() {
			return false
		}

		return true
	})).Return(query)
	query.On("Create").Return(nil)

	st := NewStorage(db, "cost-table", zap.NewNop())

	err := st.SaveOperationCost(context.Background(), &OperationCost{
		RequestID:           "req",
		OperationType:       "op",
		Timestamp:           now,
		TotalCostMicroCents: 123,
	})
	require.NoError(t, err)
	db.AssertExpectations(t)
	query.AssertExpectations(t)
}

func TestStorage_SaveOperationCost_ReturnsErrorWhenUninitialized(t *testing.T) {
	t.Parallel()

	err := (&Storage{}).SaveOperationCost(context.Background(), &OperationCost{})
	require.Error(t, err)
}


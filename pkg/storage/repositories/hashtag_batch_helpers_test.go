package repositories

import (
	"context"
	stdErrors "errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	dynamoerrors "github.com/pay-theory/dynamorm/pkg/errors"
	dynamormmocks "github.com/pay-theory/dynamorm/pkg/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"
)

func TestDeleteOldRecordsBatch_UnknownModelType(t *testing.T) {
	ctx := context.Background()
	mockDB := new(dynamormmocks.MockDB)

	deleted, err := deleteOldRecordsBatch(ctx, mockDB, zap.NewNop(), time.Now(), BatchDeleteConfig{
		ModelType:   "nope",
		ErrorPrefix: "irrelevant",
		BatchSize:   2,
		QueryLimit:  10,
		FilterField: "UpdatedAt",
	})

	assert.Equal(t, 0, deleted)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrHashtagBatchUnknownModelType)
}

func TestProcessModelBatchDelete_EmptyInputNoops(t *testing.T) {
	ctx := context.Background()
	mockDB := new(dynamormmocks.MockDB)

	deleted := processModelBatchDelete[*models.HashtagTrend](ctx, mockDB, zap.NewNop(), nil, 25, "hashtag trend")
	assert.Equal(t, 0, deleted)
}

func TestDeleteOldHashtagTrendRecordsBatch_NotFoundReturnsNil(t *testing.T) {
	ctx := context.Background()
	mockDB := new(dynamormmocks.MockDB)
	scanQuery := new(dynamormmocks.MockQuery)

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(scanQuery).Once()
	scanQuery.On("Filter", "UpdatedAt", "<", mock.Anything).Return(scanQuery).Once()
	scanQuery.On("Limit", 10).Return(scanQuery).Once()
	scanQuery.On("Scan", mock.Anything).Return(dynamoerrors.ErrItemNotFound).Once()

	deleted, err := deleteOldHashtagTrendRecordsBatch(ctx, mockDB, zap.NewNop(), time.Now(), BatchDeleteConfig{
		ModelType:   "hashtag_trend",
		ErrorPrefix: "old hashtag trend cleanup",
		BatchSize:   25,
		QueryLimit:  10,
		FilterField: "UpdatedAt",
	})

	assert.NoError(t, err)
	assert.Equal(t, 0, deleted)
}

func TestDeleteOldTrendingHashtagRecordsBatch_DeletesAllItemsEvenIfSomeDeleteFail(t *testing.T) {
	ctx := context.Background()
	mockDB := new(dynamormmocks.MockDB)
	scanQuery := new(dynamormmocks.MockQuery)
	deleteQuery := new(dynamormmocks.MockQuery)

	mockDB.On("WithContext", mock.Anything).Return(mockDB)

	mockDB.On("Model", mock.Anything).Return(scanQuery).Once()
	scanQuery.On("Filter", "UpdatedAt", "<", mock.Anything).Return(scanQuery).Once()
	scanQuery.On("Limit", 10).Return(scanQuery).Once()
	scanQuery.On("Scan", mock.Anything).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]*models.TrendingHashtag)
		*dest = []*models.TrendingHashtag{
			{Hashtag: "go", Date: "2025-01-01"},
			{Hashtag: "rust", Date: "2025-01-01"},
			{Hashtag: "zig", Date: "2025-01-01"},
		}
	}).Return(nil).Once()

	mockDB.On("Model", mock.Anything).Return(deleteQuery).Times(3)
	deleteQuery.On("Delete").Return(stdErrors.New("delete failed")).Once()
	deleteQuery.On("Delete").Return(nil).Twice()

	deleted, err := deleteOldTrendingHashtagRecordsBatch(ctx, mockDB, zap.NewNop(), time.Now(), BatchDeleteConfig{
		ModelType:   "trending_hashtag",
		ErrorPrefix: "old trending hashtag cleanup",
		BatchSize:   2,
		QueryLimit:  10,
		FilterField: "UpdatedAt",
	})

	assert.NoError(t, err)
	assert.Equal(t, 3, deleted)
}

func TestDeleteOldHashtagUsageRecordsBatch_DeletesInBatches(t *testing.T) {
	ctx := context.Background()
	mockDB := new(dynamormmocks.MockDB)
	scanQuery := new(dynamormmocks.MockQuery)
	deleteQuery := new(dynamormmocks.MockQuery)

	mockDB.On("WithContext", mock.Anything).Return(mockDB)

	mockDB.On("Model", mock.Anything).Return(scanQuery).Once()
	scanQuery.On("Filter", "UsedAt", "<", mock.Anything).Return(scanQuery).Once()
	scanQuery.On("Limit", 10).Return(scanQuery).Once()
	scanQuery.On("Scan", mock.Anything).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]*models.HashtagUsage)
		*dest = []*models.HashtagUsage{
			{StatusID: "s1"},
			{StatusID: "s2"},
			{StatusID: "s3"},
			{StatusID: "s4"},
			{StatusID: "s5"},
		}
	}).Return(nil).Once()

	mockDB.On("Model", mock.Anything).Return(deleteQuery).Times(5)
	deleteQuery.On("Delete").Return(nil).Times(5)

	deleted, err := deleteOldHashtagUsageRecordsBatch(ctx, mockDB, zap.NewNop(), time.Now(), BatchDeleteConfig{
		ModelType:   "hashtag_usage",
		ErrorPrefix: "old hashtag usage cleanup",
		BatchSize:   2,
		QueryLimit:  10,
		FilterField: "UsedAt",
	})

	assert.NoError(t, err)
	assert.Equal(t, 5, deleted)
}

func TestDeleteOldTrendingHashtagRecordsBatch_ScanErrorReturnsError(t *testing.T) {
	ctx := context.Background()
	mockDB := new(dynamormmocks.MockDB)
	scanQuery := new(dynamormmocks.MockQuery)

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(scanQuery).Once()
	scanQuery.On("Filter", "UpdatedAt", "<", mock.Anything).Return(scanQuery).Once()
	scanQuery.On("Limit", 10).Return(scanQuery).Once()
	scanQuery.On("Scan", mock.Anything).Return(stdErrors.New("scan failed")).Once()

	deleted, err := deleteOldTrendingHashtagRecordsBatch(ctx, mockDB, zap.NewNop(), time.Now(), BatchDeleteConfig{
		ModelType:   "trending_hashtag",
		ErrorPrefix: "old trending hashtag cleanup",
		BatchSize:   25,
		QueryLimit:  10,
		FilterField: "UpdatedAt",
	})

	assert.Error(t, err)
	assert.Equal(t, 0, deleted)
}

func TestDeleteOldHashtagUsageRecordsBatch_ScanErrorReturnsError(t *testing.T) {
	ctx := context.Background()
	mockDB := new(dynamormmocks.MockDB)
	scanQuery := new(dynamormmocks.MockQuery)

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(scanQuery).Once()
	scanQuery.On("Filter", "UsedAt", "<", mock.Anything).Return(scanQuery).Once()
	scanQuery.On("Limit", 10).Return(scanQuery).Once()
	scanQuery.On("Scan", mock.Anything).Return(stdErrors.New("scan failed")).Once()

	deleted, err := deleteOldHashtagUsageRecordsBatch(ctx, mockDB, zap.NewNop(), time.Now(), BatchDeleteConfig{
		ModelType:   "hashtag_usage",
		ErrorPrefix: "old hashtag usage cleanup",
		BatchSize:   25,
		QueryLimit:  10,
		FilterField: "UsedAt",
	})

	assert.Error(t, err)
	assert.Equal(t, 0, deleted)
}

func TestDeleteBatch_EmptySliceReturnsNil(t *testing.T) {
	ctx := context.Background()
	mockDB := new(dynamormmocks.MockDB)

	assert.NoError(t, deleteBatch(ctx, mockDB, nil))
	assert.NoError(t, deleteBatch(ctx, mockDB, []any{}))
}

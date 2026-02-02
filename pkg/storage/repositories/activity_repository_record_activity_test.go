package repositories

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/theory-cloud/tabletheory/pkg/mocks"
	"go.uber.org/zap"
)

func TestActivityRepository_RecordActivity_Success(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()

	repo := NewActivityRepository(mockDB, models.MainTableName, logger, nil)

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.MatchedBy(func(model interface{}) bool {
		record, ok := model.(*models.ActivityMetric)
		if !ok {
			return false
		}
		return record.ActivityType == "push_delivery_success" &&
			record.ActorID == "admin"
	})).Return(mockQuery)
	mockQuery.On("Create").Return(nil)

	err := repo.RecordActivity(context.Background(), "push_delivery_success", "admin", time.Unix(0, 0).UTC())

	assert.NoError(t, err)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestActivityRepository_RecordActivity_PropagatesError(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewActivityRepository(mockDB, models.MainTableName, logger, nil)

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Create").Return(errors.New("boom"))

	err := repo.RecordActivity(context.Background(), "push_delivery_failed", "admin", time.Unix(0, 0).UTC())

	assert.Error(t, err)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

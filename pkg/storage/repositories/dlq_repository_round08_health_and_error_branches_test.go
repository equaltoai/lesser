package repositories

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	"go.uber.org/zap"
)

func TestDLQRepository_Round08_MonitorDLQHealth_Alerts(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	// GetDLQMessagesByServiceDateRange -> GetDLQMessagesByService -> FindWithPagination -> query.All
	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]*models.DLQMessage)
		messages := make([]*models.DLQMessage, 101) // sentinel triggers trimming to 100
		for i := range messages {
			status := DLQStatusNew
			if i < 15 {
				status = DLQStatusAbandoned
			} else if i < 30 {
				status = DLQStatusReprocessing
			}
			messages[i] = &models.DLQMessage{
				ID:                   "id",
				Service:              "svc",
				Status:               status,
				ErrorType:            "type",
				ReprocessingCount:    4,
				MaxReprocessAttempts: 10,
				FirstSeenAt:          time.Now(),
			}
		}
		*dest = messages
	}).Return(nil).Once()

	setupPermissiveRound08Mocks(mockDB, mockQuery, nil, time.Date(2025, 12, 28, 0, 0, 0, 0, time.UTC))

	repo := NewDLQRepository(mockDB, "test-table", zap.NewNop(), nil)
	repo.SetValidationService(nil)
	repo.SetPermissionService(nil)
	repo.SetCachingService(nil)
	repo.SetEventService(nil)

	health, err := repo.MonitorDLQHealth(ctx, "svc")
	require.NoError(t, err)
	require.False(t, health.IsHealthy)
	require.GreaterOrEqual(t, len(health.Alerts), 1)
}

func TestDLQRepository_Round08_GetSimilarMessages_Error(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Filter", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("Limit", mock.Anything).Return(mockQuery)
	mockQuery.On("All", mock.Anything).Return(errors.New("all failed")).Once()

	repo := NewDLQRepository(mockDB, "test-table", zap.NewNop(), nil)
	repo.SetValidationService(nil)
	repo.SetPermissionService(nil)
	repo.SetCachingService(nil)
	repo.SetEventService(nil)

	_, err := repo.GetSimilarMessages(ctx, "hash", 10)
	require.Error(t, err)
}

func TestDLQRepository_Round08_SendToDeadLetterQueue_NonPermanent(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	setupPermissiveRound08Mocks(mockDB, mockQuery, nil, time.Date(2025, 12, 28, 0, 0, 0, 0, time.UTC))

	repo := NewDLQRepository(mockDB, "test-table", zap.NewNop(), nil)
	repo.SetValidationService(nil)
	repo.SetPermissionService(nil)
	repo.SetCachingService(nil)
	repo.SetEventService(nil)

	require.NoError(t, repo.SendToDeadLetterQueue(ctx, "svc", "msg-1", "{}", "type", "message", false))
}

func TestDLQRepository_Round08_CreateDLQMessage_BeforeCreateValidationError(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	setupPermissiveRound08Mocks(mockDB, mockQuery, nil, time.Date(2025, 12, 28, 0, 0, 0, 0, time.UTC))

	repo := NewDLQRepository(mockDB, "test-table", zap.NewNop(), nil)
	repo.SetValidationService(nil)
	repo.SetPermissionService(nil)
	repo.SetCachingService(nil)
	repo.SetEventService(nil)

	// Missing required fields -> BeforeCreate()->Validate fails.
	require.Error(t, repo.CreateDLQMessage(ctx, &models.DLQMessage{}))
}

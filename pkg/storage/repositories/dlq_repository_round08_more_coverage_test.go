package repositories

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/dynamorm/pkg/mocks"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestDLQRepository_Round08_NewDLQRepositorySimple(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
	mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Create").Return(nil).Maybe()
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Delete").Return(nil).Maybe()

	repo := NewDLQRepositorySimple(mockDB, "test-table", zap.NewNop())
	require.NotNil(t, repo)
}

func TestDLQRepository_Round08_GetDLQMessagesForReprocessing_FiltersAndCursor(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Index", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("Limit", mock.Anything).Return(mockQuery)

	// limit=1 => safeLimit=1 => query.Limit(2)
	allCalls := 0
	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		allCalls++
		dest := args.Get(0).(*[]*models.DLQMessage)
		if allCalls == 1 {
			*dest = []*models.DLQMessage{
				{ID: "1", Service: "svc", Status: DLQStatusNew, MaxReprocessAttempts: 3, GSI2SK: "t#1"},
				{ID: "2", Service: "svc", Status: DLQStatusNew, MaxReprocessAttempts: 3, GSI2SK: "t#2"},
			}
			return
		}
		*dest = []*models.DLQMessage{}
	}).Return(nil).Twice()

	repo := NewDLQRepository(mockDB, "test-table", zap.NewNop(), nil)
	repo.SetValidationService(nil)
	repo.SetPermissionService(nil)
	repo.SetCachingService(nil)
	repo.SetEventService(nil)

	msgs, cursor, err := repo.GetDLQMessagesForReprocessing(ctx, "svc", DLQStatusNew, 1, "")
	require.NoError(t, err)
	require.Len(t, msgs, 1)
	require.NotEmpty(t, cursor)
}

func TestDLQRepository_Round08_CleanupExpiredMessages_DeletesAndCounts(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	expired := []*models.DLQMessage{
		{ID: "1", Service: "svc"},
		{ID: "2", Service: "svc"},
	}

	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]*models.DLQMessage)
		*dest = expired
	}).Return(nil).Once()

	// DeleteDLQMessage uses BaseRepository.Delete -> query.Delete.
	mockQuery.On("Delete").Return(nil).Once()
	mockQuery.On("Delete").Return(errors.New("delete failed")).Once()

	setupPermissiveRound08Mocks(mockDB, mockQuery, nil, time.Date(2025, 12, 28, 0, 0, 0, 0, time.UTC))

	repo := NewDLQRepository(mockDB, "test-table", zap.NewNop(), nil)
	repo.SetValidationService(nil)
	repo.SetPermissionService(nil)
	repo.SetCachingService(nil)
	repo.SetEventService(nil)

	count, err := repo.CleanupExpiredMessages(ctx, time.Now())
	require.NoError(t, err)
	require.Equal(t, 1, count)
}

func TestDLQRepository_Round08_RetryFailedMessage_AbandonsAndUpdates(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	now := time.Date(2025, 12, 28, 0, 0, 0, 0, time.UTC)
	msg := models.NewDLQMessageBuilder().
		ForService("svc").
		WithOriginalMessage("orig-1", "{}").
		WithError("type", "message", "").
		Build()
	require.NoError(t, msg.BeforeCreate())
	msg.FirstSeenAt = now
	msg.MaxReprocessAttempts = 1
	msg.ReprocessingCount = 0
	msg.Status = DLQStatusNew
	require.NoError(t, msg.BeforeUpdate())

	mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*models.DLQMessage)
		*dest = *msg
	}).Return(nil).Once()

	var updated *models.DLQMessage
	mockDB.On("Model", mock.Anything).Return(mockQuery).Run(func(args mock.Arguments) {
		if candidate, ok := args.Get(0).(*models.DLQMessage); ok {
			updated = candidate
		}
	}).Maybe()

	// UpdateDLQMessage uses BaseRepository.Update -> query.Update.
	mockQuery.On("Update", mock.Anything).Return(nil).Once()

	setupPermissiveRound08Mocks(mockDB, mockQuery, nil, now)

	repo := NewDLQRepository(mockDB, "test-table", zap.NewNop(), nil)
	repo.SetValidationService(nil)
	repo.SetPermissionService(nil)
	repo.SetCachingService(nil)
	repo.SetEventService(nil)

	require.NoError(t, repo.RetryFailedMessage(ctx, msg.ID))
	require.NotNil(t, updated)
	require.Equal(t, DLQStatusAbandoned, updated.Status)
}

func TestDLQRepository_Round08_GetRetryableMessages_ReadyFiltering(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	now := time.Date(2025, 12, 28, 0, 0, 0, 0, time.UTC)
	future := time.Now().Add(10 * time.Minute)

	// Two calls to GetDLQMessagesForReprocessing -> query.All.
	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]*models.DLQMessage)
		*dest = []*models.DLQMessage{
			{ID: "ready", Service: "svc", Status: DLQStatusNew, MaxReprocessAttempts: 3, NextRetryAt: nil},
			{ID: "wait", Service: "svc", Status: DLQStatusNew, MaxReprocessAttempts: 3, NextRetryAt: &future},
		}
	}).Return(nil).Twice()

	setupPermissiveRound08Mocks(mockDB, mockQuery, nil, now)

	repo := NewDLQRepository(mockDB, "test-table", zap.NewNop(), nil)
	repo.SetValidationService(nil)
	repo.SetPermissionService(nil)
	repo.SetCachingService(nil)
	repo.SetEventService(nil)

	msgs, err := repo.GetRetryableMessages(ctx, "svc", 10)
	require.NoError(t, err)
	require.Len(t, msgs, 2)
}

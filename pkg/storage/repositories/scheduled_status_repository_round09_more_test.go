package repositories

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	"go.uber.org/zap"
)

func TestRound09_ScheduledStatusRepository_NotFoundAndNilMediaRepo(t *testing.T) {
	t.Parallel()

	baseTime := time.Now().UTC()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	mockQuery.On("All", mock.Anything).Return(nil).Once()
	setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

	repo := NewScheduledStatusRepository(mockDB, "test-table", zap.NewNop(), nil)
	repo.SetValidationService(nil)
	repo.SetPermissionService(nil)
	repo.SetEventService(nil)
	repo.SetCachingService(nil)

	_, err := repo.GetScheduledStatus(context.Background(), "missing")
	require.Error(t, err)

	withMedia := &models.ScheduledStatus{
		ID:          "scheduled-media",
		Username:    "user-1",
		MediaIDs:    []string{"media-1"},
		ScheduledAt: baseTime.Add(1 * time.Minute),
	}
	_ = withMedia.UpdateKeys()

	mockDB2 := new(mocks.MockDB)
	mockQuery2 := new(mocks.MockQuery)
	mockQuery2.On("All", mock.Anything).Run(func(args mock.Arguments) {
		ptr, ok := args.Get(0).(*[]*models.ScheduledStatus)
		if !ok {
			return
		}
		*ptr = append(*ptr, withMedia)
	}).Return(nil).Once()
	setupPermissiveRound08Mocks(mockDB2, mockQuery2, nil, baseTime)

	repo2 := NewScheduledStatusRepository(mockDB2, "test-table", zap.NewNop(), nil)
	repo2.SetValidationService(nil)
	repo2.SetPermissionService(nil)
	repo2.SetEventService(nil)
	repo2.SetCachingService(nil)

	attachments, err := repo2.GetScheduledStatusMedia(context.Background(), "scheduled-media")
	require.NoError(t, err)
	require.Empty(t, attachments)

	_ = repo2.CreateScheduledStatus(context.Background(), &storage.ScheduledStatus{
		ID:          "scheduled-fixed",
		Username:    "user-1",
		Visibility:  "public",
		ScheduledAt: baseTime.Add(10 * time.Minute),
	})
}

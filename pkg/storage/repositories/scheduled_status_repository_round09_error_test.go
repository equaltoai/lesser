package repositories

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/pkg/mocks"
	"go.uber.org/zap"
)

func TestRound09_ScheduledStatusRepository_QueryErrors(t *testing.T) {
	t.Parallel()

	baseTime := time.Now().UTC()
	ctx := context.Background()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	mockQuery.On("All", mock.Anything).Return(errors.New("boom")).Twice()
	setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

	repo := NewScheduledStatusRepository(mockDB, "test-table", zap.NewNop(), nil)
	repo.SetValidationService(nil)
	repo.SetPermissionService(nil)
	repo.SetEventService(nil)
	repo.SetCachingService(nil)

	_, err := repo.GetScheduledStatus(ctx, "id-1")
	require.Error(t, err)

	_, err = repo.GetDueScheduledStatuses(ctx, baseTime.Add(1*time.Hour), 10)
	require.Error(t, err)
}

package repositories

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	"go.uber.org/zap"
)

type round09MediaRepo struct {
	items map[string]*models.Media
	errs  map[string]error
}

func (m *round09MediaRepo) GetMedia(_ context.Context, mediaID string) (*models.Media, error) {
	if err, ok := m.errs[mediaID]; ok {
		return nil, err
	}
	if item, ok := m.items[mediaID]; ok {
		return item, nil
	}
	return nil, errors.New("not found")
}

func TestRound09_ScheduledStatusRepository_CreateGetUpdateDelete(t *testing.T) {
	t.Parallel()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	setupPermissiveRound08Mocks(mockDB, mockQuery, nil, time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))

	repo := NewScheduledStatusRepository(mockDB, "test-table", zap.NewNop(), nil)
	repo.SetValidationService(nil)
	repo.SetPermissionService(nil)
	repo.SetEventService(nil)
	repo.SetCachingService(nil)

	ctx := context.Background()

	input := &storage.ScheduledStatus{
		Username:    "user-1",
		Status:      "hello",
		Visibility:  "public",
		ScheduledAt: time.Now().Add(5 * time.Minute),
	}

	require.NoError(t, repo.CreateScheduledStatus(ctx, input))
	require.NotEmpty(t, input.ID)
	require.False(t, input.CreatedAt.IsZero())
	require.False(t, input.UpdatedAt.IsZero())

	got, err := repo.GetScheduledStatus(ctx, input.ID)
	require.NoError(t, err)
	require.NotNil(t, got)

	require.NoError(t, repo.UpdateScheduledStatus(ctx, &storage.ScheduledStatus{
		ID:          input.ID,
		Status:      "updated",
		Visibility:  "unlisted",
		ScheduledAt: input.ScheduledAt,
	}))

	require.NoError(t, repo.MarkScheduledStatusPublished(ctx, input.ID))
	require.NoError(t, repo.DeleteScheduledStatus(ctx, input.ID))
}

func TestRound09_ScheduledStatusRepository_ListAndDueAndMediaBranches(t *testing.T) {
	t.Parallel()

	baseTime := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		target := args.Get(0)
		ptr, ok := target.(*[]*models.ScheduledStatus)
		if !ok {
			return
		}
		one := &models.ScheduledStatus{ID: "scheduled-1", Username: "user-1", ScheduledAt: baseTime.Add(1 * time.Minute), Published: false}
		_ = one.UpdateKeys()
		two := &models.ScheduledStatus{ID: "scheduled-2", Username: "user-1", ScheduledAt: baseTime.Add(2 * time.Minute), Published: false}
		_ = two.UpdateKeys()
		*ptr = append(*ptr, one, two)
	}).Return(nil).Once()

	setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

	repo := NewScheduledStatusRepository(mockDB, "test-table", zap.NewNop(), nil)
	repo.SetValidationService(nil)
	repo.SetPermissionService(nil)
	repo.SetEventService(nil)
	repo.SetCachingService(nil)

	ctx := context.Background()

	statuses, cursor, err := repo.GetScheduledStatuses(ctx, "user-1", 1, "")
	require.NoError(t, err)
	require.Len(t, statuses, 1)
	require.NotEmpty(t, cursor)

	due, err := repo.GetDueScheduledStatuses(ctx, baseTime.Add(10*time.Minute), 10)
	require.NoError(t, err)
	require.NotEmpty(t, due)

	mockQuery2 := new(mocks.MockQuery)
	mockDB2 := new(mocks.MockDB)
	setupPermissiveRound08Mocks(mockDB2, mockQuery2, nil, baseTime)

	repo2 := NewScheduledStatusRepository(mockDB2, "test-table", zap.NewNop(), nil)
	repo2.SetValidationService(nil)
	repo2.SetPermissionService(nil)
	repo2.SetEventService(nil)
	repo2.SetCachingService(nil)

	noMedia := &models.ScheduledStatus{ID: "scheduled-nomedia", Username: "user-1", ScheduledAt: baseTime.Add(1 * time.Minute)}
	_ = noMedia.UpdateKeys()
	mockQuery2.On("All", mock.Anything).Run(func(args mock.Arguments) {
		ptr, ok := args.Get(0).(*[]*models.ScheduledStatus)
		if !ok {
			return
		}
		*ptr = append(*ptr, noMedia)
	}).Return(nil).Once()

	attachments, err := repo2.GetScheduledStatusMedia(ctx, "scheduled-nomedia")
	require.NoError(t, err)
	require.Empty(t, attachments)

	withMedia := &models.ScheduledStatus{
		ID:          "scheduled-media",
		Username:    "user-1",
		MediaIDs:    []string{"media-1", "media-2"},
		ScheduledAt: baseTime.Add(1 * time.Minute),
	}
	_ = withMedia.UpdateKeys()

	mockQuery2.On("All", mock.Anything).Run(func(args mock.Arguments) {
		ptr, ok := args.Get(0).(*[]*models.ScheduledStatus)
		if !ok {
			return
		}
		*ptr = append(*ptr, withMedia)
	}).Return(nil).Once()

	repo2.SetMediaRepository(&round09MediaRepo{
		items: map[string]*models.Media{
			"media-1": {MediaID: "media-1", ContentType: MediaTypeImage},
		},
		errs: map[string]error{
			"media-2": errors.New("boom"),
		},
	})

	attachments, err = repo2.GetScheduledStatusMedia(ctx, "scheduled-media")
	require.NoError(t, err)
	require.Len(t, attachments, 1)
	require.Equal(t, "media-1", attachments[0].MediaID)
}

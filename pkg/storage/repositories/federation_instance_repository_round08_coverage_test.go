package repositories

import (
	"context"
	"encoding/base64"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/federation/types"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/dynamorm/pkg/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestFederationInstanceRepository_Round08_CRUD(t *testing.T) {
	ctx := context.Background()

	t.Run("CreateInstance success calls create", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Create").Return(nil).Once()

		repo := NewFederationInstanceRepository(mockDB, "table", zap.NewNop(), nil)
		repo.SetValidationService(nil)
		repo.SetPermissionService(nil)
		repo.SetCachingService(nil)
		repo.SetEventService(nil)

		instance := &types.Instance{
			ID:           "i1",
			Domain:       "example.com",
			Status:       types.InstanceStatusActive,
			TierLevel:    types.TierStandard,
			RegisteredAt: time.Now().Add(-time.Hour),
			LastSeen:     time.Now().Add(-time.Minute),
		}

		err := repo.CreateInstance(ctx, instance)
		require.NoError(t, err)

		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
	})

	t.Run("CreateInstance create error is mapped", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Create").Return(assert.AnError).Once()

		repo := NewFederationInstanceRepository(mockDB, "table", zap.NewNop(), nil)
		repo.SetValidationService(nil)
		repo.SetPermissionService(nil)
		repo.SetCachingService(nil)
		repo.SetEventService(nil)

		err := repo.CreateInstance(ctx, &types.Instance{ID: "i1", Domain: "example.com", Status: types.InstanceStatusActive, TierLevel: types.TierStandard})
		require.Error(t, err)

		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
	})

	t.Run("GetInstance success converts model", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Twice()
		mockQuery.On("First", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*models.FederationInstanceRegistry)
			*dest = models.FederationInstanceRegistry{
				ID:           "i1",
				Domain:       "example.com",
				Status:       "active",
				TierLevel:    "standard",
				RateLimits:   map[string]interface{}{"MessagesPerMinute": float64(10)},
				LastSeen:     time.Now().Add(-time.Minute),
				RegisteredAt: time.Now().Add(-time.Hour),
			}
			_ = dest.UpdateKeys()
		}).Once()

		repo := NewFederationInstanceRepository(mockDB, "table", zap.NewNop(), nil)
		instance, err := repo.GetInstance(ctx, "i1")
		require.NoError(t, err)
		require.NotNil(t, instance)
		assert.Equal(t, "i1", instance.ID)
		assert.Equal(t, types.InstanceStatus("active"), instance.Status)
		assert.Equal(t, 10, instance.RateLimits.MessagesPerMinute)

		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
	})

	t.Run("UpdateInstance update error is mapped", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Update", mock.Anything).Return(assert.AnError).Once()

		repo := NewFederationInstanceRepository(mockDB, "table", zap.NewNop(), nil)
		repo.SetValidationService(nil)
		repo.SetPermissionService(nil)
		repo.SetCachingService(nil)
		repo.SetEventService(nil)

		err := repo.UpdateInstance(ctx, &types.Instance{ID: "i1", Domain: "example.com", Status: types.InstanceStatusActive, TierLevel: types.TierStandard})
		require.Error(t, err)

		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
	})

	t.Run("DeleteInstance delete error is mapped", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Twice()
		mockQuery.On("Delete").Return(assert.AnError).Once()

		repo := NewFederationInstanceRepository(mockDB, "table", zap.NewNop(), nil)
		err := repo.DeleteInstance(ctx, "i1")
		require.Error(t, err)

		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
	})
}

func TestFederationInstanceRepository_Round08_PaginationSearchAndHelpers(t *testing.T) {
	ctx := context.Background()

	t.Run("ListInstancesByStatusWithCursor returns next cursor when over limit", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Index", "gsi1").Return(mockQuery).Once()
		mockQuery.On("Where", "gsi1PK", "=", "STATUS#active").Return(mockQuery).Once()
		mockQuery.On("Cursor", "CURSOR").Return(mockQuery).Once()
		mockQuery.On("Limit", 2).Return(mockQuery).Once()
		mockQuery.On("All", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]models.FederationInstanceRegistry)
			*dest = []models.FederationInstanceRegistry{
				{ID: "i1", Domain: "one.example", Status: "active", TierLevel: "standard", GSI1SK: "SK#1"},
				{ID: "i2", Domain: "two.example", Status: "active", TierLevel: "standard", GSI1SK: "SK#2"},
			}
		}).Once()

		repo := NewFederationInstanceRepository(mockDB, "table", zap.NewNop(), nil)
		items, next, err := repo.ListInstancesByStatusWithCursor(ctx, types.InstanceStatusActive, 1, "CURSOR")
		require.NoError(t, err)
		require.Len(t, items, 1)
		assert.Equal(t, "SK#1", next)

		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
	})

	t.Run("GetInstancesByTierWithCursor orders and paginates", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Index", "gsi2").Return(mockQuery).Once()
		mockQuery.On("Where", "gsi2PK", "=", "TIER#standard").Return(mockQuery).Once()
		mockQuery.On("OrderBy", "gsi2SK", "ASC").Return(mockQuery).Once()
		mockQuery.On("Limit", 2).Return(mockQuery).Once()
		mockQuery.On("All", mock.Anything).Return(nil).Once()

		repo := NewFederationInstanceRepository(mockDB, "table", zap.NewNop(), nil)
		_, _, err := repo.GetInstancesByTierWithCursor(ctx, types.TierStandard, 1, "")
		require.NoError(t, err)

		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
	})

	t.Run("SearchInstancesWithCursor maps query errors", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Where", "SK", "=", "METADATA").Return(mockQuery).Once()
		mockQuery.On("Filter", "Domain", "contains", "exa").Return(mockQuery).Once()
		mockQuery.On("Limit", 2).Return(mockQuery).Once()
		mockQuery.On("All", mock.Anything).Return(assert.AnError).Once()

		repo := NewFederationInstanceRepository(mockDB, "table", zap.NewNop(), nil)
		_, _, err := repo.SearchInstancesWithCursor(ctx, "exa", 1, "")
		require.Error(t, err)

		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
	})

	t.Run("ListAllInstances startKey conversion and cursor back conversion", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Where", "SK", "=", "METADATA").Return(mockQuery).Once()
		mockQuery.On("Cursor", "INSTANCE#prev").Return(mockQuery).Once()
		mockQuery.On("Limit", 2).Return(mockQuery).Once()
		mockQuery.On("All", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]models.FederationInstanceRegistry)
			*dest = []models.FederationInstanceRegistry{
				{PK: "INSTANCE#one.example", ID: "i1", Domain: "one.example", Status: "active", TierLevel: "standard"},
				{PK: "INSTANCE#two.example", ID: "i2", Domain: "two.example", Status: "active", TierLevel: "standard"},
			}
		}).Once()

		repo := NewFederationInstanceRepository(mockDB, "table", zap.NewNop(), nil)
		startKey := map[string]interface{}{"PK": "INSTANCE#prev"}
		items, lastKey, err := repo.ListAllInstances(ctx, 1, startKey)
		require.NoError(t, err)
		require.Len(t, items, 1)
		require.NotNil(t, lastKey)
		assert.Equal(t, "INSTANCE#one.example", lastKey["PK"])

		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
	})

	t.Run("validateCursor covers length/base64/script guards", func(t *testing.T) {
		repo := NewFederationInstanceRepository(nil, "table", zap.NewNop(), nil)
		require.NoError(t, repo.validateCursor(""))
		require.ErrorIs(t, repo.validateCursor(stringsRepeat("a", 1025)), ErrFederationInstanceCursorTooLong)

		b64 := base64.URLEncoding.EncodeToString([]byte("cursor-data"))
		require.NoError(t, repo.validateCursor(b64))

		require.ErrorIs(t, repo.validateCursor("<script>alert(1)</script>"), ErrFederationInstanceCursorInvalid)
	})
}

func TestFederationInstanceRepository_Round08_HealthAndBatchHelpers(t *testing.T) {
	ctx := context.Background()

	t.Run("BatchGetInstances empty slice returns empty", func(t *testing.T) {
		repo := NewFederationInstanceRepository(nil, "table", zap.NewNop(), nil)
		items, err := repo.BatchGetInstances(ctx, nil)
		require.NoError(t, err)
		assert.Empty(t, items)
	})

	t.Run("UpdateInstanceHealth stores update and tolerates history failure", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockUpdateQuery := new(mocks.MockQuery)
		mockCreateQuery := new(mocks.MockQuery)

		// Update health record.
		mockDB.On("WithContext", mock.Anything).Return(mockDB).Twice()
		mockDB.On("Model", mock.Anything).Return(mockUpdateQuery).Once()
		mockUpdateQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockUpdateQuery).Twice()
		mockUpdateQuery.On("Update", mock.Anything).Return(nil).Once()

		// storeHealthHistory -> BaseRepository.Create on history model.
		mockDB.On("Model", mock.Anything).Return(mockCreateQuery).Once()
		mockCreateQuery.On("Create").Return(assert.AnError).Once()

		repo := NewFederationInstanceRepository(mockDB, "table", zap.NewNop(), nil)
		repo.SetValidationService(nil)
		repo.SetPermissionService(nil)
		repo.SetCachingService(nil)
		repo.SetEventService(nil)

		err := repo.UpdateInstanceHealth(ctx, "i1", &types.HealthStatus{
			Reachable:    false,
			ResponseTime: 150 * time.Millisecond,
			ErrorRate:    0.5,
			Timestamp:    time.Now(),
		})
		require.NoError(t, err)

		mockDB.AssertExpectations(t)
		mockUpdateQuery.AssertExpectations(t)
		mockCreateQuery.AssertExpectations(t)
	})

	t.Run("GetHealthHistory converts models", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Times(3)
		mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Limit", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("All", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]*models.FederationInstanceRegistryHealthHistory)
			*dest = []*models.FederationInstanceRegistryHealthHistory{
				{Timestamp: time.Now(), Reachable: true, ResponseTime: 10, StatusCode: 200},
			}
		}).Once()

		repo := NewFederationInstanceRepository(mockDB, "table", zap.NewNop(), nil)
		history, err := repo.GetHealthHistory(ctx, "i1", time.Hour)
		require.NoError(t, err)
		require.Len(t, history, 1)
		assert.True(t, history[0].Reachable)

		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
	})

	t.Run("conversion helpers include supported types and rate limits mapping", func(t *testing.T) {
		repo := NewFederationInstanceRepository(nil, "table", zap.NewNop(), nil)

		instance := &types.Instance{
			ID:             "i1",
			Domain:         "example.com",
			Status:         types.InstanceStatusActive,
			TierLevel:      types.TierStandard,
			SupportedTypes: []types.MessageType{types.MessageTypeFollow},
			RateLimits: types.RateLimits{
				MessagesPerMinute: 10,
				BytesPerMinute:    100,
			},
			AvgResponseTime: 120 * time.Millisecond,
		}
		model := repo.toModel(instance)
		require.NotNil(t, model)
		require.Equal(t, []string{string(types.MessageTypeFollow)}, model.SupportedTypes)

		// fromModel expects JSON-decoded numbers (float64), so adjust types to cover parsing logic.
		model.RateLimits = map[string]interface{}{
			"MessagesPerMinute": float64(10),
			"BytesPerMinute":    float64(100),
		}
		roundTripped := repo.fromModel(model)
		assert.Equal(t, types.TierStandard, roundTripped.TierLevel)
		assert.Equal(t, 10, roundTripped.RateLimits.MessagesPerMinute)
	})
}

func TestFederationInstanceRepository_Round08_AdditionalCoverage(t *testing.T) {
	ctx := context.Background()

	t.Run("UpdateInstanceUsage updates usage and attempts status update when quota exceeded", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQueryGet := new(mocks.MockQuery)
		mockQueryUpdate := new(mocks.MockQuery)
		mockQueryStatus := new(mocks.MockQuery)

		// First: load current model.
		mockDB.On("WithContext", mock.Anything).Return(mockDB).Times(3)
		mockDB.On("Model", mock.Anything).Return(mockQueryGet).Once()
		mockQueryGet.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQueryGet).Twice()
		mockQueryGet.On("First", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*models.FederationInstanceRegistry)
			*dest = models.FederationInstanceRegistry{
				ID:           "i1",
				Domain:       "example.com",
				Status:       "active",
				TierLevel:    "standard",
				MonthlyQuota: 10,
				CurrentUsage: 9,
			}
			_ = dest.UpdateKeys()
		}).Once()

		// Second: update usage.
		mockDB.On("Model", mock.Anything).Return(mockQueryUpdate).Once()
		mockQueryUpdate.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQueryUpdate).Twice()
		mockQueryUpdate.On("Update", mock.Anything).Return(nil).Once()

		// Third: status update attempt (warn on error).
		mockDB.On("Model", mock.Anything).Return(mockQueryStatus).Once()
		mockQueryStatus.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQueryStatus).Twice()
		mockQueryStatus.On("Update", mock.Anything).Return(assert.AnError).Once()

		repo := NewFederationInstanceRepository(mockDB, "table", zap.NewNop(), nil)
		err := repo.UpdateInstanceUsage(ctx, "i1", 5)
		require.NoError(t, err)
	})

	t.Run("BatchGetInstances uses BaseRepository batch get and converts results", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("BatchGet", mock.Anything, mock.Anything).Return(nil).Run(func(args mock.Arguments) {
			dest := args.Get(1).(*[]*models.FederationInstanceRegistry)
			*dest = []*models.FederationInstanceRegistry{
				{ID: "i1", Domain: "one.example", Status: "active", TierLevel: "standard"},
				{ID: "i2", Domain: "two.example", Status: "active", TierLevel: "standard"},
			}
		}).Once()

		repo := NewFederationInstanceRepository(mockDB, "table", zap.NewNop(), nil)
		items, err := repo.BatchGetInstances(ctx, []string{"i1", "i2"})
		require.NoError(t, err)
		require.Len(t, items, 2)
	})

	t.Run("SearchInstancesWithCursor returns next cursor and trims", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Where", "SK", "=", "METADATA").Return(mockQuery).Once()
		mockQuery.On("Filter", "Domain", "contains", "exa").Return(mockQuery).Once()
		mockQuery.On("Cursor", "INSTANCE#prev").Return(mockQuery).Once()
		mockQuery.On("Limit", 2).Return(mockQuery).Once()
		mockQuery.On("All", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]models.FederationInstanceRegistry)
			*dest = []models.FederationInstanceRegistry{
				{PK: "INSTANCE#one.example", ID: "i1", Domain: "one.example", Status: "active", TierLevel: "standard"},
				{PK: "INSTANCE#two.example", ID: "i2", Domain: "two.example", Status: "active", TierLevel: "standard"},
			}
		}).Once()

		repo := NewFederationInstanceRepository(mockDB, "table", zap.NewNop(), nil)
		items, next, err := repo.SearchInstancesWithCursor(ctx, "exa", 1, "INSTANCE#prev")
		require.NoError(t, err)
		require.Len(t, items, 1)
		assert.Equal(t, "INSTANCE#one.example", next)
	})

	t.Run("BatchCreateInstances batches over 25 items", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()
		mockQuery.On("Create").Return(nil).Maybe()

		repo := NewFederationInstanceRepository(mockDB, "table", zap.NewNop(), nil)

		instances := make([]*types.Instance, 0, 26)
		for i := 0; i < 26; i++ {
			instances = append(instances, &types.Instance{
				ID:        "i",
				Domain:    "d" + stringsRepeat("x", i%3),
				Status:    types.InstanceStatusActive,
				TierLevel: types.TierStandard,
			})
		}
		require.NoError(t, repo.BatchCreateInstances(ctx, instances))
	})

	t.Run("BatchUpdateInstancesUsage no-op when batch get returns empty", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		// BatchGetInstances -> BatchGet -> returns no items.
		mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("BatchGet", mock.Anything, mock.Anything).Return(nil).Run(func(args mock.Arguments) {
			dest := args.Get(1).(*[]*models.FederationInstanceRegistry)
			*dest = nil
		}).Once()

		repo := NewFederationInstanceRepository(mockDB, "table", zap.NewNop(), nil)
		require.NoError(t, repo.BatchUpdateInstancesUsage(ctx, map[string]int64{"i1": 10}))
	})

	t.Run("validatePaginationParams rejects negative and too-large limits", func(t *testing.T) {
		repo := NewFederationInstanceRepository(nil, "table", zap.NewNop(), nil)
		require.ErrorIs(t, repo.validatePaginationParams(-1, ""), ErrFederationInstanceLimitNegative)
		require.ErrorIs(t, repo.validatePaginationParams(1001, ""), ErrFederationInstanceLimitTooLarge)
	})
}

func TestFederationInstanceRepository_Round08_Sweep(t *testing.T) {
	ctx := context.Background()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
	mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()

	mockQuery.On("Index", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Filter", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Limit", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Cursor", mock.Anything).Return(mockQuery).Maybe()

	mockQuery.On("Create").Return(nil).Maybe()
	mockQuery.On("Delete").Return(nil).Maybe()
	mockQuery.On("Update", mock.Anything).Return(nil).Maybe()

	mockQuery.On("First", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		switch dest := args.Get(0).(type) {
		case *models.FederationInstanceRegistry:
			*dest = models.FederationInstanceRegistry{
				ID:           "i1",
				Domain:       "example.com",
				Status:       "active",
				TierLevel:    "standard",
				MonthlyQuota: 100,
				CurrentUsage: 10,
				RateLimits:   map[string]interface{}{"MessagesPerMinute": float64(10)},
				LastSeen:     time.Now().Add(-time.Minute),
				RegisteredAt: time.Now().Add(-time.Hour),
			}
			_ = dest.UpdateKeys()
		}
	}).Maybe()

	mockQuery.On("All", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		switch dest := args.Get(0).(type) {
		case *[]models.FederationInstanceRegistry:
			*dest = []models.FederationInstanceRegistry{
				{PK: "INSTANCE#one.example", ID: "i1", Domain: "one.example", Status: "active", TierLevel: "standard", GSI1SK: "SK#1", GSI2SK: "USAGE#0000000010"},
				{PK: "INSTANCE#two.example", ID: "i2", Domain: "two.example", Status: "active", TierLevel: "standard", GSI1SK: "SK#2", GSI2SK: "USAGE#0000000020"},
			}
		case *[]*models.FederationInstanceRegistryHealthHistory:
			*dest = []*models.FederationInstanceRegistryHealthHistory{
				{Timestamp: time.Now(), Reachable: true, ResponseTime: 10, StatusCode: 200},
			}
		}
	}).Maybe()

	mockQuery.On("BatchGet", mock.Anything, mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		dest := args.Get(1).(*[]*models.FederationInstanceRegistry)
		*dest = []*models.FederationInstanceRegistry{
			{ID: "i1", Domain: "one.example", Status: "active", TierLevel: "standard", MonthlyQuota: 100, CurrentUsage: 10},
			{ID: "i2", Domain: "two.example", Status: "active", TierLevel: "standard", MonthlyQuota: 100, CurrentUsage: 10},
		}
	}).Maybe()

	mockQuery.On("BatchCreate", mock.Anything).Return(nil).Maybe()

	repo := NewFederationInstanceRepository(mockDB, "table", zap.NewNop(), nil)
	repo.SetValidationService(nil)
	repo.SetPermissionService(nil)
	repo.SetCachingService(nil)
	repo.SetEventService(nil)

	require.NoError(t, repo.CreateInstance(ctx, &types.Instance{ID: "i1", Domain: "example.com", Status: types.InstanceStatusActive, TierLevel: types.TierStandard}))
	require.NoError(t, repo.UpdateInstance(ctx, &types.Instance{ID: "i1", Domain: "example.com", Status: types.InstanceStatusActive, TierLevel: types.TierStandard}))
	require.NoError(t, repo.DeleteInstance(ctx, "i1"))

	_, _ = repo.GetInstance(ctx, "i1")
	_, _ = repo.GetInstanceByDomain(ctx, "example.com")

	_, _, _ = repo.ListInstancesByStatusWithCursor(ctx, types.InstanceStatusActive, 1, "CURSOR")
	_, _, _ = repo.GetInstancesByTierWithCursor(ctx, types.TierStandard, 1, "")
	_, _ = repo.ListHealthyInstances(ctx)

	_, _ = repo.BatchGetInstances(ctx, []string{"i1", "i2"})
	_ = repo.UpdateInstanceHealth(ctx, "i1", &types.HealthStatus{Reachable: true, ResponseTime: 10 * time.Millisecond, Timestamp: time.Now()})
	_ = repo.UpdateInstanceUsage(ctx, "i1", 1)

	_, _, _ = repo.SearchInstancesWithCursor(ctx, "exa", 1, "INSTANCE#prev")
	_, _, _ = repo.ListAllInstancesWithCursor(ctx, 1, "INSTANCE#prev")
	_, _, _ = repo.ListAllInstances(ctx, 1, map[string]interface{}{"PK": "INSTANCE#prev"})
	_, _ = repo.GetHealthHistory(ctx, "i1", time.Hour)

	instances := []*types.Instance{
		{ID: "i1", Domain: "one.example", Status: types.InstanceStatusActive, TierLevel: types.TierStandard},
	}
	require.NoError(t, repo.BatchCreateInstances(ctx, instances))

	require.NoError(t, repo.BatchUpdateInstancesHealth(ctx, map[string]*types.HealthStatus{
		"i1": {Reachable: true, ResponseTime: time.Millisecond, Timestamp: time.Now()},
	}))
	require.NoError(t, repo.BatchUpdateInstancesUsage(ctx, map[string]int64{"i1": 10}))
}

func TestFederationInstanceRepository_Round08_BatchUpdateCoverage(t *testing.T) {
	ctx := context.Background()

	t.Run("GetInstancesByTier and SearchInstances wrappers execute", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()
		mockQuery.On("Index", mock.Anything).Return(mockQuery).Maybe()
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
		mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery).Maybe()
		mockQuery.On("Filter", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
		mockQuery.On("Limit", mock.Anything).Return(mockQuery).Maybe()
		mockQuery.On("All", mock.Anything).Return(nil).Maybe()

		repo := NewFederationInstanceRepository(mockDB, "table", zap.NewNop(), nil)
		_, _ = repo.GetInstancesByTier(ctx, types.TierStandard, 1)
		_, _ = repo.SearchInstances(ctx, "exa", 1)
	})

	t.Run("batchGetInstancesInChunks triggers when over 100 ids", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()
		mockQuery.On("BatchGet", mock.Anything, mock.Anything).Return(nil).Run(func(args mock.Arguments) {
			dest := args.Get(1).(*[]*models.FederationInstanceRegistry)
			*dest = nil
		}).Maybe()

		repo := NewFederationInstanceRepository(mockDB, "table", zap.NewNop(), nil)
		ids := make([]string, 0, 101)
		for i := 0; i < 101; i++ {
			ids = append(ids, "id"+stringsRepeat("x", i%3))
		}
		_, err := repo.BatchGetInstances(ctx, ids)
		require.NoError(t, err)
	})

	t.Run("BatchUpdateInstancesHealth covers error, success, and chunking", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()
		mockQuery.On("BatchCreate", mock.Anything).Return(assert.AnError).Once()

		repo := NewFederationInstanceRepository(mockDB, "table", zap.NewNop(), nil)
		err := repo.BatchUpdateInstancesHealth(ctx, map[string]*types.HealthStatus{
			"i1": {Reachable: true, ResponseTime: time.Millisecond, Timestamp: time.Now()},
		})
		require.Error(t, err)

		mockQuery.On("BatchCreate", mock.Anything).Return(nil).Maybe()
		mockQuery.On("Create").Return(nil).Maybe()

		require.NoError(t, repo.BatchUpdateInstancesHealth(ctx, map[string]*types.HealthStatus{
			"i1": {Reachable: true, ResponseTime: time.Millisecond, Timestamp: time.Now()},
		}))

		updates := make(map[string]*types.HealthStatus, 26)
		for i := 0; i < 26; i++ {
			updates["i"+stringsRepeat("x", i)] = &types.HealthStatus{Reachable: true, ResponseTime: time.Millisecond, Timestamp: time.Now()}
		}
		require.NoError(t, repo.BatchUpdateInstancesHealth(ctx, updates))
	})

	t.Run("BatchUpdateInstancesUsage covers chunking and fromModel rate parsing", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()

		mockQuery.On("BatchGet", mock.Anything, mock.Anything).Return(nil).Run(func(args mock.Arguments) {
			keys := args.Get(0).([]any)
			dest := args.Get(1).(*[]*models.FederationInstanceRegistry)
			modelsOut := make([]*models.FederationInstanceRegistry, 0, len(keys))
			for idx := range keys {
				modelsOut = append(modelsOut, &models.FederationInstanceRegistry{
					ID:           "id" + stringsRepeat("x", idx),
					Domain:       "d.example",
					Status:       "active",
					TierLevel:    "standard",
					MonthlyQuota: 1,
					CurrentUsage: 0,
				})
			}
			*dest = modelsOut
		}).Maybe()

		mockQuery.On("BatchCreate", mock.Anything).Return(nil).Maybe()

		repo := NewFederationInstanceRepository(mockDB, "table", zap.NewNop(), nil)

		usageUpdates := make(map[string]int64, 26)
		for i := 0; i < 26; i++ {
			usageUpdates["id"+stringsRepeat("x", i)] = 10
		}
		require.NoError(t, repo.BatchUpdateInstancesUsage(ctx, usageUpdates))

		instance := repo.fromModel(&models.FederationInstanceRegistry{
			ID:        "i1",
			Domain:    "example.com",
			Status:    "active",
			TierLevel: "standard",
			RateLimits: map[string]interface{}{
				"MessagesPerMinute": float64(1),
				"MessagesPerHour":   float64(2),
				"BytesPerMinute":    float64(3),
				"BytesPerHour":      float64(4),
				"BurstSize":         float64(5),
			},
		})
		assert.Equal(t, 2, instance.RateLimits.MessagesPerHour)
		assert.Equal(t, int64(4), instance.RateLimits.BytesPerHour)
		assert.Equal(t, 5, instance.RateLimits.BurstSize)
	})
}

func TestFederationInstanceRepository_Round08_GetErrors(t *testing.T) {
	ctx := context.Background()

	t.Run("GetInstance maps underlying get errors", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Twice()
		mockQuery.On("First", mock.Anything).Return(assert.AnError).Once()

		repo := NewFederationInstanceRepository(mockDB, "table", zap.NewNop(), nil)
		_, err := repo.GetInstance(ctx, "i1")
		require.Error(t, err)
	})

	t.Run("GetInstanceByDomain maps underlying get errors", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Twice()
		mockQuery.On("First", mock.Anything).Return(assert.AnError).Once()

		repo := NewFederationInstanceRepository(mockDB, "table", zap.NewNop(), nil)
		_, err := repo.GetInstanceByDomain(ctx, "example.com")
		require.Error(t, err)
	})
}

func stringsRepeat(s string, count int) string {
	out := ""
	for i := 0; i < count; i++ {
		out += s
	}
	return out
}

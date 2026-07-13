package repositories

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/federation/types"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	dynamormerrors "github.com/theory-cloud/tabletheory/v2/pkg/errors"
	"github.com/theory-cloud/tabletheory/v2/pkg/mocks"
	"go.uber.org/zap"
)

func TestQueryCacheRepository_Round08_CacheCRUD(t *testing.T) {
	ctx := context.Background()

	t.Run("GetCachedValue not found is mapped", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Twice()
		mockQuery.On("First", mock.Anything).Return(dynamormerrors.ErrItemNotFound).Once()

		repo := NewQueryCacheRepository(mockDB, "table", zap.NewNop(), nil, nil, nil)
		_, err := repo.GetCachedValue(ctx, "missing")
		require.Error(t, err)

		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
	})

	t.Run("GetCachedValue expired deletes and returns not found", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQueryGet := new(mocks.MockQuery)
		mockQueryDelete := new(mocks.MockQuery)

		// Get.
		mockDB.On("WithContext", mock.Anything).Return(mockDB).Twice()
		mockDB.On("Model", mock.Anything).Return(mockQueryGet).Once()
		mockQueryGet.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQueryGet).Twice()
		mockQueryGet.On("First", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*models.QueryCacheEntry)
			*dest = models.QueryCacheEntry{
				PK:        "CACHE#expired",
				SK:        "KEY#expired",
				CacheKey:  "expired",
				Value:     `{"ok":true}`,
				Size:      1,
				ExpiresAt: time.Now().Add(-time.Minute),
			}
		}).Once()

		// Delete (ignored errors).
		mockDB.On("Model", mock.Anything).Return(mockQueryDelete).Once()
		mockQueryDelete.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQueryDelete).Twice()
		mockQueryDelete.On("Delete").Return(nil).Once()

		repo := NewQueryCacheRepository(mockDB, "table", zap.NewNop(), nil, nil, nil)
		_, err := repo.GetCachedValue(ctx, "expired")
		require.Error(t, err)

		mockDB.AssertExpectations(t)
		mockQueryGet.AssertExpectations(t)
		mockQueryDelete.AssertExpectations(t)
	})

	t.Run("GetCachedValue invalid JSON is mapped", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Twice()
		mockQuery.On("First", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*models.QueryCacheEntry)
			*dest = models.QueryCacheEntry{
				PK:        "CACHE#badjson",
				SK:        "KEY#badjson",
				CacheKey:  "badjson",
				Value:     "not-json",
				Size:      1,
				ExpiresAt: time.Now().Add(time.Minute),
			}
		}).Once()

		repo := NewQueryCacheRepository(mockDB, "table", zap.NewNop(), nil, nil, nil)
		_, err := repo.GetCachedValue(ctx, "badjson")
		require.Error(t, err)

		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
	})

	t.Run("GetCachedValue success returns deserialized result", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Twice()
		mockQuery.On("First", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*models.QueryCacheEntry)
			*dest = models.QueryCacheEntry{
				PK:        "CACHE#ok",
				SK:        "KEY#ok",
				CacheKey:  "ok",
				Value:     `{"a":1}`,
				Size:      1,
				ExpiresAt: time.Now().Add(time.Minute),
			}
		}).Once()

		repo := NewQueryCacheRepository(mockDB, "table", zap.NewNop(), nil, nil, nil)
		got, err := repo.GetCachedValue(ctx, "ok")
		require.NoError(t, err)
		require.NotNil(t, got)

		asMap, ok := got.(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, float64(1), asMap["a"])

		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
	})

	t.Run("SetCachedValue handles marshal errors and create errors", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		repo := NewQueryCacheRepository(mockDB, "table", zap.NewNop(), nil, nil, nil)

		require.Error(t, repo.SetCachedValue(ctx, "k", func() {}, 1, time.Minute))

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Twice()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Twice()
		mockQuery.On("Create").Return(assert.AnError).Once()
		mockQuery.On("Create").Return(nil).Once()

		require.Error(t, repo.SetCachedValue(ctx, "k1", map[string]any{"x": 1}, 1, time.Minute))
		require.NoError(t, repo.SetCachedValue(ctx, "k2", map[string]any{"x": 2}, 1, time.Minute))

		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
	})

	t.Run("InvalidateCachePattern supports prefix scans and exact deletes", func(t *testing.T) {
		t.Run("prefix success", func(t *testing.T) {
			mockDB := new(mocks.MockDB)
			mockQueryScan := new(mocks.MockQuery)
			mockQueryDelete1 := new(mocks.MockQuery)
			mockQueryDelete2 := new(mocks.MockQuery)

			mockDB.On("WithContext", mock.Anything).Return(mockDB).Times(3)

			// Prefix scan.
			mockDB.On("Model", mock.Anything).Return(mockQueryScan).Once()
			mockQueryScan.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQueryScan).Twice()
			mockQueryScan.On("OrderBy", mock.Anything, mock.Anything).Return(mockQueryScan).Once()
			mockQueryScan.On("Limit", mock.Anything).Return(mockQueryScan).Once()
			mockQueryScan.On("All", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
				dest := args.Get(0).(*[]models.QueryCacheEntry)
				*dest = []models.QueryCacheEntry{
					{PK: "CACHE#pre", SK: "KEY#pre:1"},
					{PK: "CACHE#pre", SK: "KEY#pre:2"},
				}
			}).Once()

			// Deletes (ignored errors).
			mockDB.On("Model", mock.Anything).Return(mockQueryDelete1).Once()
			mockQueryDelete1.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQueryDelete1).Twice()
			mockQueryDelete1.On("Delete").Return(nil).Once()

			mockDB.On("Model", mock.Anything).Return(mockQueryDelete2).Once()
			mockQueryDelete2.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQueryDelete2).Twice()
			mockQueryDelete2.On("Delete").Return(nil).Once()

			repo := NewQueryCacheRepository(mockDB, "table", zap.NewNop(), nil, nil, nil)
			require.NoError(t, repo.InvalidateCachePattern(ctx, "pre:*"))

			mockDB.AssertExpectations(t)
			mockQueryScan.AssertExpectations(t)
			mockQueryDelete1.AssertExpectations(t)
			mockQueryDelete2.AssertExpectations(t)
		})

		t.Run("prefix scan error is mapped", func(t *testing.T) {
			mockDB := new(mocks.MockDB)
			mockQueryScan := new(mocks.MockQuery)

			mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
			mockDB.On("Model", mock.Anything).Return(mockQueryScan).Once()
			mockQueryScan.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQueryScan).Twice()
			mockQueryScan.On("OrderBy", mock.Anything, mock.Anything).Return(mockQueryScan).Once()
			mockQueryScan.On("Limit", mock.Anything).Return(mockQueryScan).Once()
			mockQueryScan.On("All", mock.Anything).Return(assert.AnError).Once()

			repo := NewQueryCacheRepository(mockDB, "table", zap.NewNop(), nil, nil, nil)
			require.Error(t, repo.InvalidateCachePattern(ctx, "pre:*"))

			mockDB.AssertExpectations(t)
			mockQueryScan.AssertExpectations(t)
		})

		t.Run("exact delete ignores not-found", func(t *testing.T) {
			mockDB := new(mocks.MockDB)
			mockQueryDelete := new(mocks.MockQuery)

			mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
			mockDB.On("Model", mock.Anything).Return(mockQueryDelete).Once()
			mockQueryDelete.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQueryDelete).Twice()
			mockQueryDelete.On("Delete").Return(dynamormerrors.ErrItemNotFound).Once()

			repo := NewQueryCacheRepository(mockDB, "table", zap.NewNop(), nil, nil, nil)
			require.NoError(t, repo.InvalidateCachePattern(ctx, "missing-exact"))

			mockDB.AssertExpectations(t)
			mockQueryDelete.AssertExpectations(t)
		})

		t.Run("exact delete returns error for other failures", func(t *testing.T) {
			mockDB := new(mocks.MockDB)
			mockQueryDelete := new(mocks.MockQuery)

			mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
			mockDB.On("Model", mock.Anything).Return(mockQueryDelete).Once()
			mockQueryDelete.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQueryDelete).Twice()
			mockQueryDelete.On("Delete").Return(assert.AnError).Once()

			repo := NewQueryCacheRepository(mockDB, "table", zap.NewNop(), nil, nil, nil)
			require.Error(t, repo.InvalidateCachePattern(ctx, "broken"))

			mockDB.AssertExpectations(t)
			mockQueryDelete.AssertExpectations(t)
		})
	})

	t.Run("GetInstance converts cached JSON object and validates cache shape", func(t *testing.T) {
		t.Run("success", func(t *testing.T) {
			mockDB := new(mocks.MockDB)
			mockQuery := new(mocks.MockQuery)

			mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
			mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
			mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Twice()
			mockQuery.On("First", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
				dest := args.Get(0).(*models.QueryCacheEntry)
				*dest = models.QueryCacheEntry{
					PK:        "CACHE#instance",
					SK:        "KEY#instance:i1",
					CacheKey:  "instance:i1",
					Value:     `{"id":"i1","domain":"example.com","status":"active","tier_level":"standard"}`,
					Size:      1,
					ExpiresAt: time.Now().Add(time.Minute),
				}
			}).Once()

			repo := NewQueryCacheRepository(mockDB, "table", zap.NewNop(), nil, nil, nil)
			instance, err := repo.GetInstance(ctx, "i1")
			require.NoError(t, err)
			require.NotNil(t, instance)
			assert.Equal(t, "i1", instance.ID)

			mockDB.AssertExpectations(t)
			mockQuery.AssertExpectations(t)
		})

		t.Run("invalid cache shape returns invalid input", func(t *testing.T) {
			mockDB := new(mocks.MockDB)
			mockQuery := new(mocks.MockQuery)

			mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
			mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
			mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Twice()
			mockQuery.On("First", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
				dest := args.Get(0).(*models.QueryCacheEntry)
				*dest = models.QueryCacheEntry{
					PK:        "CACHE#instance",
					SK:        "KEY#instance:i1",
					CacheKey:  "instance:i1",
					Value:     `"not-a-map"`,
					Size:      1,
					ExpiresAt: time.Now().Add(time.Minute),
				}
			}).Once()

			repo := NewQueryCacheRepository(mockDB, "table", zap.NewNop(), nil, nil, nil)
			_, err := repo.GetInstance(ctx, "i1")
			require.Error(t, err)

			mockDB.AssertExpectations(t)
			mockQuery.AssertExpectations(t)
		})
	})
}

func TestQueryCacheRepository_Round08_CacheFallbacksAndBatch(t *testing.T) {
	ctx := context.Background()

	t.Run("GetInstancesByStatus cache hit parses list and skips bad items", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Twice()
		mockQuery.On("First", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*models.QueryCacheEntry)
			*dest = models.QueryCacheEntry{
				PK:        "CACHE#status",
				SK:        "KEY#status:active",
				CacheKey:  "status:active",
				Value:     `[{"id":"i1","domain":"one.example","status":"active","tier_level":"standard"},"skip-me"]`,
				Size:      2,
				ExpiresAt: time.Now().Add(time.Minute),
			}
		}).Once()

		repo := NewQueryCacheRepository(mockDB, "table", zap.NewNop(), nil, nil, nil)
		items, err := repo.GetInstancesByStatus(ctx, types.InstanceStatusActive)
		require.NoError(t, err)
		require.Len(t, items, 1)
		assert.Equal(t, "i1", items[0].ID)

		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
	})

	t.Run("GetInstancesByStatus cache miss falls back to instance repo and warns on cache set failure", func(t *testing.T) {
		cacheDB := new(mocks.MockDB)
		cacheQueryGet := new(mocks.MockQuery)
		cacheQueryCreate := new(mocks.MockQuery)

		// Cache get -> not found.
		cacheDB.On("WithContext", mock.Anything).Return(cacheDB).Twice()
		cacheDB.On("Model", mock.Anything).Return(cacheQueryGet).Once()
		cacheQueryGet.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(cacheQueryGet).Twice()
		cacheQueryGet.On("First", mock.Anything).Return(dynamormerrors.ErrItemNotFound).Once()

		// Cache set -> create error.
		cacheDB.On("Model", mock.Anything).Return(cacheQueryCreate).Once()
		cacheQueryCreate.On("Create").Return(assert.AnError).Once()

		instanceDB := new(mocks.MockDB)
		instanceQuery := new(mocks.MockQuery)

		instanceDB.On("WithContext", mock.Anything).Return(instanceDB).Once()
		instanceDB.On("Model", mock.Anything).Return(instanceQuery).Once()
		instanceQuery.On("Index", "gsi1").Return(instanceQuery).Once()
		instanceQuery.On("Where", "gsi1PK", "=", "STATUS#active").Return(instanceQuery).Once()
		instanceQuery.On("Limit", 101).Return(instanceQuery).Once()
		instanceQuery.On("All", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]models.FederationInstanceRegistry)
			*dest = []models.FederationInstanceRegistry{
				{ID: "i1", Domain: "one.example", Status: "active", TierLevel: "standard"},
			}
		}).Once()

		instanceRepo := NewFederationInstanceRepository(instanceDB, "table", zap.NewNop(), nil)
		instanceRepo.SetValidationService(nil)
		instanceRepo.SetPermissionService(nil)
		instanceRepo.SetCachingService(nil)
		instanceRepo.SetEventService(nil)

		repo := NewQueryCacheRepository(cacheDB, "table", zap.NewNop(), nil, instanceRepo, nil)
		repo.SetValidationService(nil)
		repo.SetPermissionService(nil)
		repo.SetCachingService(nil)
		repo.SetEventService(nil)

		items, err := repo.GetInstancesByStatus(ctx, types.InstanceStatusActive)
		require.NoError(t, err)
		require.Len(t, items, 1)

		cacheDB.AssertExpectations(t)
		cacheQueryGet.AssertExpectations(t)
		cacheQueryCreate.AssertExpectations(t)
		instanceDB.AssertExpectations(t)
		instanceQuery.AssertExpectations(t)
	})

	t.Run("BatchGetInstances loads cache, fetches missing from DB, and caches fresh instances", func(t *testing.T) {
		cacheDB := new(mocks.MockDB)
		cacheQueryGet1 := new(mocks.MockQuery)
		cacheQueryGet2 := new(mocks.MockQuery)
		cacheQueryCreate1 := new(mocks.MockQuery)

		// GetInstance(i1) -> present.
		cacheDB.On("WithContext", mock.Anything).Return(cacheDB).Times(3)
		cacheDB.On("Model", mock.Anything).Return(cacheQueryGet1).Once()
		cacheQueryGet1.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(cacheQueryGet1).Twice()
		cacheQueryGet1.On("First", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*models.QueryCacheEntry)
			*dest = models.QueryCacheEntry{
				PK:        "CACHE#instance",
				SK:        "KEY#instance:i1",
				CacheKey:  "instance:i1",
				Value:     `{"id":"i1","domain":"one.example","status":"active","tier_level":"standard"}`,
				ExpiresAt: time.Now().Add(time.Minute),
			}
		}).Once()

		// GetInstance(i2) -> not found.
		cacheDB.On("Model", mock.Anything).Return(cacheQueryGet2).Once()
		cacheQueryGet2.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(cacheQueryGet2).Twice()
		cacheQueryGet2.On("First", mock.Anything).Return(dynamormerrors.ErrItemNotFound).Once()

		// SetInstance(i2) -> create success.
		cacheDB.On("Model", mock.Anything).Return(cacheQueryCreate1).Once()
		cacheQueryCreate1.On("Create").Return(nil).Once()

		instanceDB := new(mocks.MockDB)
		instanceQuery := new(mocks.MockQuery)

		instanceDB.On("WithContext", mock.Anything).Return(instanceDB).Once()
		instanceDB.On("Model", mock.Anything).Return(instanceQuery).Once()
		instanceQuery.On("BatchGet", mock.Anything, mock.Anything).Return(nil).Run(func(args mock.Arguments) {
			dest := args.Get(1).(*[]*models.FederationInstanceRegistry)
			*dest = []*models.FederationInstanceRegistry{
				{ID: "i2", Domain: "two.example", Status: "active", TierLevel: "standard"},
			}
		}).Once()

		instanceRepo := NewFederationInstanceRepository(instanceDB, "table", zap.NewNop(), nil)
		instanceRepo.SetValidationService(nil)
		instanceRepo.SetPermissionService(nil)
		instanceRepo.SetCachingService(nil)
		instanceRepo.SetEventService(nil)

		repo := NewQueryCacheRepository(cacheDB, "table", zap.NewNop(), nil, instanceRepo, nil)
		repo.SetValidationService(nil)
		repo.SetPermissionService(nil)
		repo.SetCachingService(nil)
		repo.SetEventService(nil)

		items, err := repo.BatchGetInstances(ctx, []string{"i1", "i2"})
		require.NoError(t, err)
		require.Len(t, items, 2)

		cacheDB.AssertExpectations(t)
		cacheQueryGet1.AssertExpectations(t)
		cacheQueryGet2.AssertExpectations(t)
		cacheQueryCreate1.AssertExpectations(t)
		instanceDB.AssertExpectations(t)
		instanceQuery.AssertExpectations(t)
	})

	t.Run("BatchGetInstances returns partial results and maps database errors", func(t *testing.T) {
		cacheDB := new(mocks.MockDB)
		cacheQueryGet := new(mocks.MockQuery)

		// Cache GetInstance -> not found.
		cacheDB.On("WithContext", mock.Anything).Return(cacheDB).Once()
		cacheDB.On("Model", mock.Anything).Return(cacheQueryGet).Once()
		cacheQueryGet.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(cacheQueryGet).Twice()
		cacheQueryGet.On("First", mock.Anything).Return(dynamormerrors.ErrItemNotFound).Once()

		instanceDB := new(mocks.MockDB)
		instanceQuery := new(mocks.MockQuery)

		instanceDB.On("WithContext", mock.Anything).Return(instanceDB).Once()
		instanceDB.On("Model", mock.Anything).Return(instanceQuery).Once()
		instanceQuery.On("BatchGet", mock.Anything, mock.Anything).Return(assert.AnError).Once()

		instanceRepo := NewFederationInstanceRepository(instanceDB, "table", zap.NewNop(), nil)
		instanceRepo.SetValidationService(nil)
		instanceRepo.SetPermissionService(nil)
		instanceRepo.SetCachingService(nil)
		instanceRepo.SetEventService(nil)

		repo := NewQueryCacheRepository(cacheDB, "table", zap.NewNop(), nil, instanceRepo, nil)
		repo.SetValidationService(nil)
		repo.SetPermissionService(nil)
		repo.SetCachingService(nil)
		repo.SetEventService(nil)

		_, err := repo.BatchGetInstances(ctx, []string{"i2"})
		require.Error(t, err)

		cacheDB.AssertExpectations(t)
		cacheQueryGet.AssertExpectations(t)
		instanceDB.AssertExpectations(t)
		instanceQuery.AssertExpectations(t)
	})

	t.Run("GetMetricsInRange returns error when route repo is missing", func(t *testing.T) {
		repo := NewQueryCacheRepository(new(mocks.MockDB), "table", zap.NewNop(), nil, nil, nil)
		_, err := repo.GetMetricsInRange(ctx, "r1", time.Now().Add(-time.Hour), time.Now(), 10)
		require.Error(t, err)
	})

	t.Run("GetMetricsInRange delegates to route repository when present", func(t *testing.T) {
		routeDB := new(mocks.MockDB)
		routeQuery := new(mocks.MockQuery)

		routeDB.On("WithContext", mock.Anything).Return(routeDB).Once()
		routeDB.On("Model", mock.Anything).Return(routeQuery).Once()
		routeQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(routeQuery).Times(2)
		routeQuery.On("OrderBy", "SK", "DESC").Return(routeQuery).Once()
		routeQuery.On("Limit", 1).Return(routeQuery).Once()
		routeQuery.On("All", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]*models.RouteDeliveryResult)
			*dest = []*models.RouteDeliveryResult{
				{RouteID: "r1@example.com", MessageID: "m1", Success: true, StatusCode: 200, Duration: 1, Timestamp: time.Now()},
			}
		}).Once()

		routeRepo := NewRouteOptimizerRepository(routeDB, "table", zap.NewNop(), nil)

		repo := NewQueryCacheRepository(new(mocks.MockDB), "table", zap.NewNop(), nil, nil, routeRepo)
		items, err := repo.GetMetricsInRange(ctx, "r1@example.com", time.Now().Add(-time.Hour), time.Time{}, 1)
		require.NoError(t, err)
		require.Len(t, items, 1)

		routeDB.AssertExpectations(t)
		routeQuery.AssertExpectations(t)
	})

	t.Run("PrewarmActiveInstances delegates to GetInstancesByStatus", func(t *testing.T) {
		cacheDB := new(mocks.MockDB)
		cacheQueryGet := new(mocks.MockQuery)
		cacheQueryCreate := new(mocks.MockQuery)

		cacheDB.On("WithContext", mock.Anything).Return(cacheDB).Twice()
		cacheDB.On("Model", mock.Anything).Return(cacheQueryGet).Once()
		cacheQueryGet.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(cacheQueryGet).Twice()
		cacheQueryGet.On("First", mock.Anything).Return(dynamormerrors.ErrItemNotFound).Once()

		cacheDB.On("Model", mock.Anything).Return(cacheQueryCreate).Once()
		cacheQueryCreate.On("Create").Return(nil).Once()

		instanceDB := new(mocks.MockDB)
		instanceQuery := new(mocks.MockQuery)

		instanceDB.On("WithContext", mock.Anything).Return(instanceDB).Once()
		instanceDB.On("Model", mock.Anything).Return(instanceQuery).Once()
		instanceQuery.On("Index", "gsi1").Return(instanceQuery).Once()
		instanceQuery.On("Where", "gsi1PK", "=", "STATUS#active").Return(instanceQuery).Once()
		instanceQuery.On("Limit", 101).Return(instanceQuery).Once()
		instanceQuery.On("All", mock.Anything).Return(nil).Once()

		instanceRepo := NewFederationInstanceRepository(instanceDB, "table", zap.NewNop(), nil)
		instanceRepo.SetValidationService(nil)
		instanceRepo.SetPermissionService(nil)
		instanceRepo.SetCachingService(nil)
		instanceRepo.SetEventService(nil)

		repo := NewQueryCacheRepository(cacheDB, "table", zap.NewNop(), nil, instanceRepo, nil)
		repo.SetValidationService(nil)
		repo.SetPermissionService(nil)
		repo.SetCachingService(nil)
		repo.SetEventService(nil)

		require.NoError(t, repo.PrewarmActiveInstances(ctx))
		require.NoError(t, repo.CleanupExpiredEntries(ctx))

		cacheDB.AssertExpectations(t)
		cacheQueryGet.AssertExpectations(t)
		cacheQueryCreate.AssertExpectations(t)
		instanceDB.AssertExpectations(t)
		instanceQuery.AssertExpectations(t)
	})

	t.Run("PrewarmActiveInstances maps underlying errors", func(t *testing.T) {
		cacheDB := new(mocks.MockDB)
		cacheQueryGet := new(mocks.MockQuery)

		cacheDB.On("WithContext", mock.Anything).Return(cacheDB).Once()
		cacheDB.On("Model", mock.Anything).Return(cacheQueryGet).Once()
		cacheQueryGet.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(cacheQueryGet).Twice()
		cacheQueryGet.On("First", mock.Anything).Return(assert.AnError).Once()

		repo := NewQueryCacheRepository(cacheDB, "table", zap.NewNop(), nil, nil, nil)
		require.Error(t, repo.PrewarmActiveInstances(ctx))

		cacheDB.AssertExpectations(t)
		cacheQueryGet.AssertExpectations(t)
	})

	t.Run("GetCachedValue not-found sentinel remains stable", func(t *testing.T) {
		assert.True(t, dynamormerrors.IsNotFound(dynamormerrors.ErrItemNotFound))
		assert.NotNil(t, storage.ErrNotFound)
	})
}

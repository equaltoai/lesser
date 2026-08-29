package repositories

// Batch #1505 — strict production-side shape pins. These tests call the real
// repository methods against a strict tabletheory mock and pin the fixed query
// chain (the compile-level pins in scanfree_wave1469_batch_1505_compile_test.go
// mirror the chain; these exercise the production code path). A mutation
// restoring the old two-range pair (issue #1500) dies here on the strict Where
// expectations.

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/v3/pkg/core"
	"github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	"go.uber.org/zap"
)

// GetSecurityEvents — the #1500 flagship site. First page: gsi4SK BETWEEN
// [start,end] (one key condition); cursor page: BETWEEN [cursor,end] with the
// cursor row dropped post-read; no-window cursor page: bare `>` bound.
func TestBatch1505_Audit_GetSecurityEvents_StrictChainPin(t *testing.T) {
	ctx := context.Background()

	t.Run("first page window compiles one BETWEEN", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockDB.On("WithContext", ctx).Return(mockDB)
		mockDB.On("Model", mock.AnythingOfType("*models.AuthAuditLog")).Return(mockQuery)
		mockQuery.On("Index", "gsi4").Return(mockQuery)
		mockQuery.On("Where", "gsi4PK", "=", "SEVERITY#HIGH").Return(mockQuery).Once()
		mockQuery.On("Where", "gsi4SK", "BETWEEN", mock.AnythingOfType("[]interface {}")).Return(mockQuery).Once()
		mockQuery.On("OrderBy", "gsi4SK", "ASC").Return(mockQuery).Once()
		mockQuery.On("Limit", 3).Return(mockQuery).Once()
		mockQuery.On("All", mock.AnythingOfType("*[]models.AuthAuditLog")).Return(nil).Once()

		repo := NewAuditRepository(mockDB, "test-table", zap.NewNop(), nil)
		start := time.Now().Add(-time.Hour)
		_, _, err := repo.GetSecurityEvents(ctx, "HIGH", start, time.Now(), 2, "")
		require.NoError(t, err)
		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
	})

	t.Run("cursor page clamps the BETWEEN lower bound and over-fetches", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockDB.On("WithContext", ctx).Return(mockDB)
		mockDB.On("Model", mock.AnythingOfType("*models.AuthAuditLog")).Return(mockQuery)
		mockQuery.On("Index", "gsi4").Return(mockQuery)
		mockQuery.On("Where", "gsi4PK", "=", "SEVERITY#HIGH").Return(mockQuery).Once()
		mockQuery.On("Where", "gsi4SK", "BETWEEN", mock.AnythingOfType("[]interface {}")).Return(mockQuery).Once()
		mockQuery.On("OrderBy", "gsi4SK", "ASC").Return(mockQuery).Once()
		mockQuery.On("Limit", 4).Return(mockQuery).Once()
		mockQuery.On("All", mock.AnythingOfType("*[]models.AuthAuditLog")).Run(func(args mock.Arguments) {
			dst := args.Get(0).(*[]models.AuthAuditLog)
			// inclusive BETWEEN re-includes the cursor row; production drops it
			// (one extra item was over-fetched so has-more stays exact)
			*dst = []models.AuthAuditLog{{GSI4SK: "AUDIT#150"}, {GSI4SK: "AUDIT#151"}, {GSI4SK: "AUDIT#152"}, {GSI4SK: "AUDIT#153"}}
		}).Return(nil).Once()

		repo := NewAuditRepository(mockDB, "test-table", zap.NewNop(), nil)
		events, next, err := repo.GetSecurityEvents(ctx, "HIGH", time.Now().Add(-time.Hour), time.Now(), 2, "AUDIT#150")
		require.NoError(t, err)
		require.Len(t, events, 2)
		require.Equal(t, "AUDIT#152", next)
		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
	})

	t.Run("no-window cursor page keys the bare bound", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockDB.On("WithContext", ctx).Return(mockDB)
		mockDB.On("Model", mock.AnythingOfType("*models.AuthAuditLog")).Return(mockQuery)
		mockQuery.On("Index", "gsi4").Return(mockQuery)
		mockQuery.On("Where", "gsi4PK", "=", "SEVERITY#HIGH").Return(mockQuery).Once()
		mockQuery.On("Where", "gsi4SK", ">", "AUDIT#150").Return(mockQuery).Once()
		mockQuery.On("OrderBy", "gsi4SK", "ASC").Return(mockQuery).Once()
		mockQuery.On("Limit", 3).Return(mockQuery).Once()
		mockQuery.On("All", mock.AnythingOfType("*[]models.AuthAuditLog")).Return(nil).Once()

		repo := NewAuditRepository(mockDB, "test-table", zap.NewNop(), nil)
		_, _, err := repo.GetSecurityEvents(ctx, "HIGH", time.Time{}, time.Time{}, 2, "AUDIT#150")
		require.NoError(t, err)
		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
	})
}

// GetInstanceHistory — inline walkKeyedPages chain with one BETWEEN on gsi1SK.
func TestBatch1505_Instance_GetInstanceHistory_StrictChainPin(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", ctx).Return(mockDB).Maybe()
	mockDB.On("Model", mock.AnythingOfType("*models.InstanceHistory")).Return(mockQuery).Maybe()
	mockQuery.On("Index", "gsi1").Return(mockQuery).Once()
	mockQuery.On("Where", "gsi1PK", "=", "METRIC#memory").Return(mockQuery).Once()
	mockQuery.On("Where", "gsi1SK", "BETWEEN", mock.AnythingOfType("[]interface {}")).Return(mockQuery).Once()
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	mockQuery.On("AllPaginated", mock.AnythingOfType("*[]models.InstanceHistory")).Return(&core.PaginatedResult{HasMore: false}, nil).Once()

	repo := NewInstanceRepository(mockDB, "test-table", zap.NewNop())
	_, err := repo.getMetricHistory(ctx, 7, "memory", "memory_history", func(_ models.InstanceHistory) map[string]interface{} { return nil })
	require.NoError(t, err)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

// GetFollowing (Shape C) — cursor page keys `>` and demotes BEGINS_WITH to a
// post-read FilterExpression; first page keys BEGINS_WITH directly.
func TestBatch1505_AccountSocial_GetFollowing_StrictChainPin(t *testing.T) {
	ctx := context.Background()

	t.Run("first page keys begins_with", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockDB.On("WithContext", ctx).Return(mockDB)
		mockDB.On("Model", mock.AnythingOfType("*models.Follow")).Return(mockQuery)
		mockQuery.On("Where", "PK", "=", "follow#alice").Return(mockQuery).Once()
		mockQuery.On("Where", "SK", "BEGINS_WITH", "following#").Return(mockQuery).Once()
		mockQuery.On("OrderBy", "SK", "ASC").Return(mockQuery).Once()
		mockQuery.On("Limit", 26).Return(mockQuery).Once()
		mockQuery.On("All", mock.AnythingOfType("*[]models.Follow")).Return(nil).Once()

		repo := NewAccountRepository(mockDB, "test-table", "example.com", zap.NewNop())
		_, _, err := repo.GetFollowing(ctx, "alice", 25, "")
		require.NoError(t, err)
		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
	})

	t.Run("cursor page closes the range at the block top and filters begins_with", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockDB.On("WithContext", ctx).Return(mockDB)
		mockDB.On("Model", mock.AnythingOfType("*models.Follow")).Return(mockQuery)
		mockQuery.On("Where", "PK", "=", "follow#alice").Return(mockQuery).Once()
		mockQuery.On("Where", "SK", "BETWEEN", []any{"following#bob", "following#~"}).Return(mockQuery).Once()
		mockQuery.On("Filter", "SK", "BEGINS_WITH", "following#").Return(mockQuery).Once()
		mockQuery.On("OrderBy", "SK", "ASC").Return(mockQuery).Once()
		mockQuery.On("Limit", 27).Return(mockQuery).Once()
		mockQuery.On("All", mock.AnythingOfType("*[]models.Follow")).Return(nil).Once()

		repo := NewAccountRepository(mockDB, "test-table", "example.com", zap.NewNop())
		_, _, err := repo.GetFollowing(ctx, "alice", 25, "following#bob")
		require.NoError(t, err)
		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
	})
}

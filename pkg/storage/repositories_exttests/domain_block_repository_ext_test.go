package repositories_exttests

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	dynamormErrors "github.com/pay-theory/dynamorm/pkg/errors"
	"github.com/pay-theory/dynamorm/pkg/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"
)

func TestDomainBlockRepository_ext_sweep(t *testing.T) {
	ctx := context.Background()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
	mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()

	mockQuery.On("Index", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Limit", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("All", mock.Anything).Return(nil).Maybe()
	mockQuery.On("First", mock.Anything).Return(dynamormErrors.ErrItemNotFound).Maybe()
	mockQuery.On("Create").Return(nil).Maybe()
	mockQuery.On("Update", mock.Anything).Return(nil).Maybe()
	mockQuery.On("Delete").Return(nil).Maybe()
	mockQuery.On("Count").Return(int64(1), nil).Maybe()

	repo := repositories.NewDomainBlockRepository(mockDB, "test-table", zap.NewNop())
	repo.SetValidationService(nil)
	repo.SetPermissionService(nil)
	repo.SetCachingService(nil)
	repo.SetEventService(nil)

	_ = repo.AddDomainBlock(ctx, "u1", "example.com")
	_ = repo.RemoveDomainBlock(ctx, "u1", "example.com")
	_, _, _ = repo.GetUserDomainBlocks(ctx, "u1", 2, "DOMAIN_BLOCK#a")
	_, _ = repo.IsBlockedDomain(ctx, "u1", "example.com")

	block := &storage.InstanceDomainBlock{
		Domain:      "Example.com",
		CreatedBy:   "admin",
		Severity:    "silence",
		RejectMedia: true,
	}
	assert.NoError(t, repo.CreateInstanceDomainBlock(ctx, block))

	_, _ = repo.GetInstanceDomainBlock(ctx, "missing.com")

	mockQuery.On("First", mock.AnythingOfType("*models.InstanceDomainBlock")).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*models.InstanceDomainBlock)
		*dest = models.InstanceDomainBlock{
			ID:        "id1",
			Domain:    "parent.com",
			CreatedAt: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
			UpdatedAt: time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC),
		}
	}).Return(nil).Once()
	_, _ = repo.GetInstanceDomainBlock(ctx, "parent.com")

	_ = repo.UpdateInstanceDomainBlock(ctx, "parent.com", map[string]any{"severity": "suspend"})
	_ = repo.DeleteInstanceDomainBlock(ctx, "parent.com")
	_, _, _ = repo.IsInstanceDomainBlocked(ctx, "sub.parent.com")

	_, _, _ = repo.GetDomainBlocks(ctx, 1, "")
	_, _ = repo.GetDomainBlock(ctx, "id1")
	_ = repo.CreateDomainBlock(ctx, &storage.InstanceDomainBlock{Domain: "another.com", CreatedBy: "admin"})
	_ = repo.UpdateDomainBlock(ctx, "id1", map[string]any{"severity": "silence"})
	_ = repo.DeleteDomainBlock(ctx, "id1")
	_, _, _ = repo.IsDomainBlocked(ctx, "sub.parent.com")

	mockQuery.On("Create").Return(dynamormErrors.ErrConditionFailed).Once()
	dup := &storage.InstanceDomainBlock{Domain: "dup.com", CreatedBy: "admin"}
	_ = repo.CreateInstanceDomainBlock(ctx, dup)

	// Email domain blocks and allows.
	mockQuery.On("Create").Return(nil).Once()
	assert.NoError(t, repo.CreateEmailDomainBlock(ctx, &storage.EmailDomainBlock{Domain: "example.com", CreatedBy: "admin"}))
	_, _, _ = repo.GetEmailDomainBlocks(ctx, 1, "")
	_ = repo.DeleteEmailDomainBlock(ctx, "id")

	mockQuery.On("Create").Return(nil).Once()
	assert.NoError(t, repo.CreateDomainAllow(ctx, &storage.DomainAllow{Domain: "allowed.com", CreatedBy: "admin"}))
	_, _, _ = repo.GetDomainAllows(ctx, 1, "")
	_ = repo.DeleteDomainAllow(ctx, "id")

	// Generic delete-by-id lookup error paths are covered by calling with a DB error.
	mockQuery.On("All", mock.Anything).Return(fmt.Errorf("boom")).Once()
	_ = repo.DeleteEmailDomainBlock(ctx, "id2")
}

func TestDomainBlockRepository_ext_additional_branches(t *testing.T) {
	ctx := context.Background()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
	mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()

	mockQuery.On("Index", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Limit", mock.Anything).Return(mockQuery).Maybe()

	_ = repositories.NewDomainBlockRepositoryWithCostTracking(mockDB, "test-table", zap.NewNop(), nil)

	repo := repositories.NewDomainBlockRepository(mockDB, "test-table", zap.NewNop())
	repo.SetValidationService(nil)
	repo.SetPermissionService(nil)
	repo.SetCachingService(nil)
	repo.SetEventService(nil)

	// Create error (covers create error handling path).
	mockQuery.On("Create").Return(fmt.Errorf("create failed")).Once()
	assert.Error(t, repo.AddDomainBlock(ctx, "u1", "example.com"))

	// GetUserDomainBlocks cursor + nextCursor generation.
	mockQuery.On("All", mock.AnythingOfType("*[]models.UserDomainBlock")).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]models.UserDomainBlock)
		*dest = []models.UserDomainBlock{
			{Domain: "a.com", SK: "DOMAIN_BLOCK#a"},
			{Domain: "b.com", SK: "DOMAIN_BLOCK#b"},
			{Domain: "c.com", SK: "DOMAIN_BLOCK#c"},
		}
	}).Return(nil).Once()
	domains, cursor, err := repo.GetUserDomainBlocks(ctx, "u1", 2, "DOMAIN_BLOCK#a")
	assert.NoError(t, err)
	assert.Equal(t, []string{"a.com", "b.com"}, domains)
	assert.Equal(t, "DOMAIN_BLOCK#b", cursor)

	// UpdateInstanceDomainBlock not found.
	mockQuery.On("First", mock.AnythingOfType("*models.InstanceDomainBlock")).Return(dynamormErrors.ErrItemNotFound).Once()
	assert.ErrorIs(t, repo.UpdateInstanceDomainBlock(ctx, "missing.com", map[string]any{"severity": "suspend"}), storage.ErrNotFound)

	// UpdateInstanceDomainBlock conditional check failure maps to not found.
	mockQuery.On("First", mock.AnythingOfType("*models.InstanceDomainBlock")).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*models.InstanceDomainBlock)
		*dest = models.InstanceDomainBlock{ID: "id1", Domain: "example.com"}
	}).Return(nil).Once()
	mockQuery.On("Update", mock.Anything).Return(fmt.Errorf("ConditionalCheckFailedException")).Once()
	assert.ErrorIs(t, repo.UpdateInstanceDomainBlock(ctx, "example.com", map[string]any{"severity": "suspend"}), storage.ErrNotFound)

	// DeleteInstanceDomainBlock conditional check failure maps to not found.
	mockQuery.On("Delete").Return(fmt.Errorf("ConditionalCheckFailedException")).Once()
	assert.ErrorIs(t, repo.DeleteInstanceDomainBlock(ctx, "example.com"), storage.ErrNotFound)
}

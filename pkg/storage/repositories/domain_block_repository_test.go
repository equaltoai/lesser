package repositories

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/cost"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/theory-cloud/tabletheory/v2/pkg/errors"
	"github.com/theory-cloud/tabletheory/v2/pkg/mocks"
	"go.uber.org/zap"
)

func TestDomainBlockRepository_user_domain_blocks(t *testing.T) {
	t.Run("add_remove_list_and_check", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		repo := NewDomainBlockRepository(mockDB, "test-table", zap.NewNop())
		// Keep ValidateAndCreate simple for unit tests.
		repo.SetValidationService(nil)
		repo.SetPermissionService(nil)
		repo.SetCachingService(nil)
		repo.SetEventService(nil)

		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.AnythingOfType("*models.UserDomainBlock")).Return(mockQuery)

		// AddDomainBlock -> ValidateAndCreate -> BaseRepository.Create -> query.Create
		mockQuery.On("Create").Return(nil).Once()
		err := repo.AddDomainBlock(context.Background(), "u1", "Example.com")
		assert.NoError(t, err)

		// RemoveDomainBlock -> BaseRepository.Delete -> Where/Where/Delete
		mockQuery.On("Where", "PK", "=", "USER#u1").Return(mockQuery).Once()
		mockQuery.On("Where", "SK", "=", "DOMAIN_BLOCK#example.com").Return(mockQuery).Once()
		mockQuery.On("Delete").Return(nil).Once()
		err = repo.RemoveDomainBlock(context.Background(), "u1", "example.com")
		assert.NoError(t, err)

		// GetUserDomainBlocks pagination (cursor branch + nextCursor branch)
		mockQuery.On("Where", "PK", "=", "USER#u1").Return(mockQuery).Once()
		mockQuery.On("OrderBy", "SK", "ASC").Return(mockQuery).Once()
		mockQuery.On("Limit", 3).Return(mockQuery).Once() // limit=2, +1 sentinel
		mockQuery.On("Where", "SK", ">", "DOMAIN_BLOCK#a").Return(mockQuery).Once()
		mockQuery.On("All", mock.AnythingOfType("*[]models.UserDomainBlock")).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]models.UserDomainBlock)
			*dest = []models.UserDomainBlock{
				{Domain: "a.com", SK: "DOMAIN_BLOCK#a"},
				{Domain: "b.com", SK: "DOMAIN_BLOCK#b"},
				{Domain: "c.com", SK: "DOMAIN_BLOCK#c"},
			}
		}).Return(nil).Once()

		domains, cursor, err := repo.GetUserDomainBlocks(context.Background(), "u1", 2, "DOMAIN_BLOCK#a")
		assert.NoError(t, err)
		assert.Equal(t, []string{"a.com", "b.com"}, domains)
		assert.Equal(t, "DOMAIN_BLOCK#b", cursor)

		// IsBlockedDomain -> BaseRepository.Exists -> Count
		mockQuery.On("Where", "PK", "=", "USER#u1").Return(mockQuery).Once()
		mockQuery.On("Where", "SK", "=", "DOMAIN_BLOCK#example.com").Return(mockQuery).Once()
		mockQuery.On("Count").Return(int64(1), nil).Once()

		blocked, err := repo.IsBlockedDomain(context.Background(), "u1", " ExAmPle.com ")
		assert.NoError(t, err)
		assert.True(t, blocked)

		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
	})
}

func TestDomainBlockRepository_instance_domain_blocks(t *testing.T) {
	t.Run("create_get_update_delete_and_hierarchy", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		repo := NewDomainBlockRepository(mockDB, "test-table", zap.NewNop())

		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.AnythingOfType("*models.InstanceDomainBlock")).Return(mockQuery)

		// CreateInstanceDomainBlock
		mockQuery.On("Create").Return(nil).Once()
		payload := &storage.InstanceDomainBlock{
			Domain:      " Example.COM ",
			Severity:    "suspend",
			RejectMedia: true,
			CreatedBy:   "admin",
		}
		err := repo.CreateInstanceDomainBlock(context.Background(), payload)
		assert.NoError(t, err)
		assert.NotEmpty(t, payload.ID)
		assert.Equal(t, "example.com", payload.Domain)

		// GetInstanceDomainBlock not found
		mockQuery.On("Where", "PK", "=", "DOMAIN_BLOCK#missing.com").Return(mockQuery).Once()
		mockQuery.On("Where", "SK", "=", "DOMAIN_BLOCK#missing.com").Return(mockQuery).Once()
		mockQuery.On("First", mock.AnythingOfType("*models.InstanceDomainBlock")).Return(errors.ErrItemNotFound).Once()
		block, err := repo.GetInstanceDomainBlock(context.Background(), "missing.com")
		assert.ErrorIs(t, err, storage.ErrNotFound)
		assert.Nil(t, block)

		// GetInstanceDomainBlock success
		mockQuery.On("Where", "PK", "=", "DOMAIN_BLOCK#parent.com").Return(mockQuery).Once()
		mockQuery.On("Where", "SK", "=", "DOMAIN_BLOCK#parent.com").Return(mockQuery).Once()
		mockQuery.On("First", mock.AnythingOfType("*models.InstanceDomainBlock")).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*models.InstanceDomainBlock)
			*dest = models.InstanceDomainBlock{
				ID:        "id1",
				Domain:    "parent.com",
				Severity:  "silence",
				CreatedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				UpdatedAt: time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
			}
		}).Return(nil).Once()
		block, err = repo.GetInstanceDomainBlock(context.Background(), "parent.com")
		assert.NoError(t, err)
		assert.NotNil(t, block)
		assert.Equal(t, "id1", block.ID)

		// IsInstanceDomainBlocked: subdomain missing, parent found.
		mockQuery.On("Where", "PK", "=", "DOMAIN_BLOCK#sub.parent.com").Return(mockQuery).Once()
		mockQuery.On("Where", "SK", "=", "DOMAIN_BLOCK#sub.parent.com").Return(mockQuery).Once()
		mockQuery.On("First", mock.AnythingOfType("*models.InstanceDomainBlock")).Return(errors.ErrItemNotFound).Once()
		mockQuery.On("Where", "PK", "=", "DOMAIN_BLOCK#parent.com").Return(mockQuery).Once()
		mockQuery.On("Where", "SK", "=", "DOMAIN_BLOCK#parent.com").Return(mockQuery).Once()
		mockQuery.On("First", mock.AnythingOfType("*models.InstanceDomainBlock")).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*models.InstanceDomainBlock)
			*dest = models.InstanceDomainBlock{
				ID:     "id-parent",
				Domain: "parent.com",
			}
		}).Return(nil).Once()
		isBlocked, parent, err := repo.IsInstanceDomainBlocked(context.Background(), "sub.parent.com")
		assert.NoError(t, err)
		assert.True(t, isBlocked)
		assert.NotNil(t, parent)

		// UpdateInstanceDomainBlock first reads...
		mockQuery.On("Where", "PK", "=", "DOMAIN_BLOCK#parent.com").Return(mockQuery).Once()
		mockQuery.On("Where", "SK", "=", "DOMAIN_BLOCK#parent.com").Return(mockQuery).Once()
		mockQuery.On("First", mock.AnythingOfType("*models.InstanceDomainBlock")).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*models.InstanceDomainBlock)
			*dest = models.InstanceDomainBlock{
				ID:     "id1",
				Domain: "parent.com",
			}
		}).Return(nil).Once()
		// ...then updates
		mockQuery.On("Update", mock.Anything).Return(nil).Once()
		err = repo.UpdateInstanceDomainBlock(context.Background(), "parent.com", map[string]any{
			"severity":        "suspend",
			"reject_media":    true,
			"reject_reports":  true,
			"private_comment": "internal",
			"public_comment":  "public",
			"obfuscate":       true,
		})
		assert.NoError(t, err)

		// DeleteInstanceDomainBlock: conditional check -> not found
		mockQuery.On("Where", "PK", "=", "DOMAIN_BLOCK#gone.com").Return(mockQuery).Once()
		mockQuery.On("Where", "SK", "=", "DOMAIN_BLOCK#gone.com").Return(mockQuery).Once()
		mockQuery.On("Delete").Return(fmt.Errorf("ConditionalCheckFailedException")).Once()
		err = repo.DeleteInstanceDomainBlock(context.Background(), "gone.com")
		assert.ErrorIs(t, err, storage.ErrNotFound)

		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
	})

	t.Run("lookup_by_id", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		repo := NewDomainBlockRepository(mockDB, "test-table", zap.NewNop())
		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.AnythingOfType("*models.InstanceDomainBlock")).Return(mockQuery)

		mockQuery.On("Index", "gsi1").Return(mockQuery).Once()
		mockQuery.On("Where", "gsi1PK", "=", "DOMAIN_BLOCKS").Return(mockQuery).Once()
		mockQuery.On("Limit", 100).Return(mockQuery).Once()
		mockQuery.On("All", mock.AnythingOfType("*[]models.InstanceDomainBlock")).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]models.InstanceDomainBlock)
			*dest = []models.InstanceDomainBlock{
				{ID: "id1", Domain: "a.com"},
				{ID: "id2", Domain: "b.com"},
			}
		}).Return(nil).Once()

		out, err := repo.GetInstanceDomainBlockByID(context.Background(), "id2")
		assert.NoError(t, err)
		assert.Equal(t, "b.com", out.Domain)
	})
}

func TestDomainBlockRepository_domain_collections(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	repo := NewDomainBlockRepository(mockDB, "test-table", zap.NewNop())
	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)

	// ListInstanceDomainBlocks delegates to helper (exercise wrapper statement).
	mockQuery.On("Index", "gsi1").Return(mockQuery).Once()
	mockQuery.On("Where", "gsi1PK", "=", "DOMAIN_BLOCKS").Return(mockQuery).Once()
	mockQuery.On("OrderBy", "gsi1SK", "DESC").Return(mockQuery).Once()
	mockQuery.On("Limit", 2).Return(mockQuery).Once()
	mockQuery.On("All", mock.Anything).Return(nil).Once()
	_, _, _ = repo.ListInstanceDomainBlocks(context.Background(), 1, "")

	// CreateEmailDomainBlock uses createDomainModelHelper.
	mockQuery.On("Create").Return(nil).Once()
	emailBlock := &storage.EmailDomainBlock{Domain: "example.com", CreatedBy: "admin"}
	err := repo.CreateEmailDomainBlock(context.Background(), emailBlock)
	assert.NoError(t, err)
	assert.NotEmpty(t, emailBlock.ID)

	// GetEmailDomainBlocks delegates to helper (wrapper statement).
	mockQuery.On("Index", "gsi1").Return(mockQuery).Once()
	mockQuery.On("Where", "gsi1PK", "=", "EMAIL_DOMAIN_BLOCKS").Return(mockQuery).Once()
	mockQuery.On("OrderBy", "gsi1SK", "DESC").Return(mockQuery).Once()
	mockQuery.On("Limit", 2).Return(mockQuery).Once()
	mockQuery.On("All", mock.Anything).Return(nil).Once()
	_, _, _ = repo.GetEmailDomainBlocks(context.Background(), 1, "")

	// CreateDomainAllow uses createDomainModelHelper.
	mockQuery.On("Create").Return(nil).Once()
	allow := &storage.DomainAllow{Domain: "allowed.com", CreatedBy: "admin"}
	err = repo.CreateDomainAllow(context.Background(), allow)
	assert.NoError(t, err)
	assert.NotEmpty(t, allow.ID)

	// GetDomainAllows delegates to helper (wrapper statement).
	mockQuery.On("Index", "gsi1").Return(mockQuery).Once()
	mockQuery.On("Where", "gsi1PK", "=", "DOMAIN_ALLOWS").Return(mockQuery).Once()
	mockQuery.On("OrderBy", "gsi1SK", "DESC").Return(mockQuery).Once()
	mockQuery.On("Limit", 2).Return(mockQuery).Once()
	mockQuery.On("All", mock.Anything).Return(nil).Once()
	_, _, _ = repo.GetDomainAllows(context.Background(), 1, "")

	// DeleteEmailDomainBlock delegates to helper (wrapper statement).
	mockQuery.On("Index", "gsi1").Return(mockQuery).Once()
	mockQuery.On("Where", "gsi1PK", "=", "EMAIL_DOMAIN_BLOCKS").Return(mockQuery).Once()
	mockQuery.On("Limit", 100).Return(mockQuery).Once()
	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]*models.EmailDomainBlock)
		item := &models.EmailDomainBlock{ID: "e1", Domain: "example.com"}
		_ = item.UpdateKeys()
		*dest = []*models.EmailDomainBlock{item}
	}).Return(nil).Once()
	mockQuery.On("Where", "PK", "=", mock.Anything).Return(mockQuery).Once()
	mockQuery.On("Where", "SK", "=", mock.Anything).Return(mockQuery).Once()
	mockQuery.On("Delete").Return(nil).Once()
	assert.NoError(t, repo.DeleteEmailDomainBlock(context.Background(), "e1"))

	// DeleteDomainAllow delegates to helper (wrapper statement).
	mockQuery.On("Index", "gsi1").Return(mockQuery).Once()
	mockQuery.On("Where", "gsi1PK", "=", "DOMAIN_ALLOWS").Return(mockQuery).Once()
	mockQuery.On("Limit", 100).Return(mockQuery).Once()
	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]*models.DomainAllow)
		item := &models.DomainAllow{ID: "a1", Domain: "allowed.com"}
		item.CreatedAt = time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
		_ = item.UpdateKeys()
		*dest = []*models.DomainAllow{item}
	}).Return(nil).Once()
	mockQuery.On("Where", "PK", "=", mock.Anything).Return(mockQuery).Once()
	mockQuery.On("Where", "SK", "=", mock.Anything).Return(mockQuery).Once()
	mockQuery.On("Delete").Return(nil).Once()
	assert.NoError(t, repo.DeleteDomainAllow(context.Background(), "a1"))
}

func TestDomainBlockRepository_wrappers(t *testing.T) {
	ctx := context.Background()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	_ = NewDomainBlockRepositoryWithCostTracking(mockDB, "test-table", zap.NewNop(), (*cost.TrackingService)(nil))

	repo := NewDomainBlockRepository(mockDB, "test-table", zap.NewNop())
	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)

	mockQuery.On("Index", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Limit", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Create").Return(nil).Maybe()
	mockQuery.On("Update", mock.Anything).Return(nil).Maybe()
	mockQuery.On("Delete").Return(nil).Maybe()

	mockQuery.On("All", mock.AnythingOfType("*[]models.InstanceDomainBlock")).Return(nil).Once()
	_, _, _ = repo.GetDomainBlocks(ctx, 1, "")

	mockQuery.On("All", mock.AnythingOfType("*[]models.InstanceDomainBlock")).Return(nil).Once()
	_, _ = repo.GetDomainBlock(ctx, "missing")

	assert.NoError(t, repo.CreateDomainBlock(ctx, &storage.InstanceDomainBlock{Domain: "example.com", CreatedBy: "admin"}))

	// UpdateDomainBlock -> GetDomainBlock -> UpdateInstanceDomainBlock
	mockQuery.On("All", mock.AnythingOfType("*[]models.InstanceDomainBlock")).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]models.InstanceDomainBlock)
		*dest = []models.InstanceDomainBlock{{ID: "id1", Domain: "example.com"}}
	}).Return(nil).Once()
	mockQuery.On("First", mock.AnythingOfType("*models.InstanceDomainBlock")).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*models.InstanceDomainBlock)
		*dest = models.InstanceDomainBlock{ID: "id1", Domain: "example.com"}
	}).Return(nil).Once()
	mockQuery.On("Update").Return(nil).Once()
	assert.NoError(t, repo.UpdateDomainBlock(ctx, "id1", map[string]any{"severity": "silence"}))

	// DeleteDomainBlock -> GetDomainBlock -> DeleteInstanceDomainBlock
	mockQuery.On("All", mock.AnythingOfType("*[]models.InstanceDomainBlock")).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]models.InstanceDomainBlock)
		*dest = []models.InstanceDomainBlock{{ID: "id2", Domain: "gone.com"}}
	}).Return(nil).Once()
	mockQuery.On("Delete").Return(nil).Once()
	assert.NoError(t, repo.DeleteDomainBlock(ctx, "id2"))

	// IsDomainBlocked delegates to IsInstanceDomainBlocked.
	mockQuery.On("First", mock.AnythingOfType("*models.InstanceDomainBlock")).Return(errors.ErrItemNotFound).Once()
	mockQuery.On("First", mock.AnythingOfType("*models.InstanceDomainBlock")).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*models.InstanceDomainBlock)
		*dest = models.InstanceDomainBlock{ID: "parent", Domain: "parent.com"}
	}).Return(nil).Once()
	blocked, block, err := repo.IsDomainBlocked(ctx, "sub.parent.com")
	assert.NoError(t, err)
	assert.True(t, blocked)
	assert.NotNil(t, block)
}

type stubDomainModel struct {
	domain     string
	updateKeys error
}

func (s *stubDomainModel) UpdateKeys() error { return s.updateKeys }
func (s *stubDomainModel) GetDomain() string { return s.domain }

func TestDomainBlockRepository_error_branches_and_helpers(t *testing.T) {
	ctx := context.Background()

	t.Run("AddDomainBlock_create_error_returns_error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		repo := NewDomainBlockRepository(mockDB, "test-table", zap.NewNop())
		repo.SetValidationService(nil)
		repo.SetPermissionService(nil)
		repo.SetCachingService(nil)
		repo.SetEventService(nil)

		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.AnythingOfType("*models.UserDomainBlock")).Return(mockQuery)
		mockQuery.On("Create").Return(fmt.Errorf("boom")).Once()

		assert.Error(t, repo.AddDomainBlock(ctx, "u1", "example.com"))
	})

	t.Run("RemoveDomainBlock_not_found_is_idempotent", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		repo := NewDomainBlockRepository(mockDB, "test-table", zap.NewNop())
		repo.SetValidationService(nil)
		repo.SetPermissionService(nil)
		repo.SetCachingService(nil)
		repo.SetEventService(nil)

		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.AnythingOfType("*models.UserDomainBlock")).Return(mockQuery)
		mockQuery.On("Where", "PK", "=", "USER#u1").Return(mockQuery).Once()
		mockQuery.On("Where", "SK", "=", "DOMAIN_BLOCK#missing.com").Return(mockQuery).Once()
		mockQuery.On("Delete").Return(errors.ErrItemNotFound).Once()

		assert.NoError(t, repo.RemoveDomainBlock(ctx, "u1", "missing.com"))
	})

	t.Run("RemoveDomainBlock_generic_error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		repo := NewDomainBlockRepository(mockDB, "test-table", zap.NewNop())
		repo.SetValidationService(nil)
		repo.SetPermissionService(nil)
		repo.SetCachingService(nil)
		repo.SetEventService(nil)

		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.AnythingOfType("*models.UserDomainBlock")).Return(mockQuery)
		mockQuery.On("Where", "PK", "=", "USER#u1").Return(mockQuery).Once()
		mockQuery.On("Where", "SK", "=", "DOMAIN_BLOCK#example.com").Return(mockQuery).Once()
		mockQuery.On("Delete").Return(fmt.Errorf("boom")).Once()

		assert.Error(t, repo.RemoveDomainBlock(ctx, "u1", "example.com"))
	})

	t.Run("IsBlockedDomain_count_error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		repo := NewDomainBlockRepository(mockDB, "test-table", zap.NewNop())
		repo.SetValidationService(nil)
		repo.SetPermissionService(nil)
		repo.SetCachingService(nil)
		repo.SetEventService(nil)

		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.AnythingOfType("*models.UserDomainBlock")).Return(mockQuery)
		mockQuery.On("Where", "PK", "=", "USER#u1").Return(mockQuery).Once()
		mockQuery.On("Where", "SK", "=", "DOMAIN_BLOCK#example.com").Return(mockQuery).Once()
		mockQuery.On("Count").Return(int64(0), assert.AnError).Once()

		_, err := repo.IsBlockedDomain(ctx, "u1", "example.com")
		assert.Error(t, err)
	})

	t.Run("UpdateInstanceDomainBlock_not_found_and_update_errors", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		repo := NewDomainBlockRepository(mockDB, "test-table", zap.NewNop())
		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.AnythingOfType("*models.InstanceDomainBlock")).Return(mockQuery)

		// Not found on initial First.
		mockQuery.On("Where", "PK", "=", "DOMAIN_BLOCK#missing.com").Return(mockQuery).Once()
		mockQuery.On("Where", "SK", "=", "DOMAIN_BLOCK#missing.com").Return(mockQuery).Once()
		mockQuery.On("First", mock.AnythingOfType("*models.InstanceDomainBlock")).Return(errors.ErrItemNotFound).Once()
		assert.ErrorIs(t, repo.UpdateInstanceDomainBlock(ctx, "missing.com", map[string]any{"severity": "silence"}), storage.ErrNotFound)

		// Found but conditional check failure on Update.
		mockQuery.On("Where", "PK", "=", "DOMAIN_BLOCK#example.com").Return(mockQuery).Once()
		mockQuery.On("Where", "SK", "=", "DOMAIN_BLOCK#example.com").Return(mockQuery).Once()
		mockQuery.On("First", mock.AnythingOfType("*models.InstanceDomainBlock")).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*models.InstanceDomainBlock)
			*dest = models.InstanceDomainBlock{ID: "id1", Domain: "example.com"}
		}).Return(nil).Once()
		mockQuery.On("Update", mock.Anything).Return(fmt.Errorf("ConditionalCheckFailedException")).Once()
		assert.ErrorIs(t, repo.UpdateInstanceDomainBlock(ctx, "example.com", map[string]any{"severity": "silence"}), storage.ErrNotFound)

		// Found but generic update error.
		mockQuery.On("Where", "PK", "=", "DOMAIN_BLOCK#example.com").Return(mockQuery).Once()
		mockQuery.On("Where", "SK", "=", "DOMAIN_BLOCK#example.com").Return(mockQuery).Once()
		mockQuery.On("First", mock.AnythingOfType("*models.InstanceDomainBlock")).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*models.InstanceDomainBlock)
			*dest = models.InstanceDomainBlock{ID: "id1", Domain: "example.com"}
		}).Return(nil).Once()
		mockQuery.On("Update", mock.Anything).Return(fmt.Errorf("boom")).Once()
		assert.Error(t, repo.UpdateInstanceDomainBlock(ctx, "example.com", map[string]any{"severity": "silence"}))
	})

	t.Run("UpdateDomainBlock_and_DeleteDomainBlock_return_GetDomainBlock_error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		repo := NewDomainBlockRepository(mockDB, "test-table", zap.NewNop())

		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.AnythingOfType("*models.InstanceDomainBlock")).Return(mockQuery)
		mockQuery.On("Index", "gsi1").Return(mockQuery).Maybe()
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
		mockQuery.On("Limit", mock.Anything).Return(mockQuery).Maybe()
		mockQuery.On("All", mock.AnythingOfType("*[]models.InstanceDomainBlock")).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]models.InstanceDomainBlock)
			*dest = []models.InstanceDomainBlock{{ID: "different", Domain: "example.com"}}
		}).Return(nil).Once()

		assert.ErrorIs(t, repo.UpdateDomainBlock(ctx, "missing", map[string]any{"severity": "silence"}), storage.ErrNotFound)

		mockQuery.On("All", mock.AnythingOfType("*[]models.InstanceDomainBlock")).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]models.InstanceDomainBlock)
			*dest = []models.InstanceDomainBlock{{ID: "different", Domain: "example.com"}}
		}).Return(nil).Once()
		assert.ErrorIs(t, repo.DeleteDomainBlock(ctx, "missing"), storage.ErrNotFound)
	})

	t.Run("CreateInstanceDomainBlock_duplicate_error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		repo := NewDomainBlockRepository(mockDB, "test-table", zap.NewNop())
		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.AnythingOfType("*models.InstanceDomainBlock")).Return(mockQuery)
		mockQuery.On("Create").Return(fmt.Errorf("ConditionalCheckFailedException")).Once()

		payload := &storage.InstanceDomainBlock{
			ID:        "",
			Domain:    "example.com",
			Severity:  "silence",
			CreatedBy: "admin",
		}
		assert.Error(t, repo.CreateInstanceDomainBlock(ctx, payload))
	})

	t.Run("createDomainModelHelper_error_branches", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)

		// UpdateKeys error.
		id := ""
		createdAt := time.Time{}
		err := createDomainModelHelper(ctx, mockDB, &stubDomainModel{domain: "example.com", updateKeys: fmt.Errorf("bad keys")}, &id, &createdAt, "stub")
		assert.Error(t, err)

		// Create conditional error.
		id = ""
		createdAt = time.Time{}
		mockQuery.On("Create").Return(fmt.Errorf("ConditionalCheckFailedException")).Once()
		err = createDomainModelHelper(ctx, mockDB, &stubDomainModel{domain: "example.com"}, &id, &createdAt, "stub")
		assert.Error(t, err)

		// Create generic error.
		id = "id1"
		createdAt = time.Time{}
		mockQuery.On("Create").Return(fmt.Errorf("boom")).Once()
		err = createDomainModelHelper(ctx, mockDB, &stubDomainModel{domain: "example.com"}, &id, &createdAt, "stub")
		assert.Error(t, err)
	})
}

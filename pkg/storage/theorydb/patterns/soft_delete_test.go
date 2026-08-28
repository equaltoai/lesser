package patterns

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	"go.uber.org/zap/zaptest"
)

func TestSoftDeleteModel_SoftDelete(t *testing.T) {
	model := &SoftDeleteModel{}

	// Initially not deleted
	assert.False(t, model.IsDeleted())
	assert.Nil(t, model.GetDeletedAt())
	assert.Empty(t, model.GetDeletedBy())

	// Soft delete
	model.SoftDelete()

	assert.True(t, model.IsDeleted())
	assert.NotNil(t, model.GetDeletedAt())
	assert.True(t, time.Since(*model.GetDeletedAt()) < time.Second)
	assert.Empty(t, model.GetDeletedBy()) // SoftDelete doesn't set user
}

func TestSoftDeleteModel_SoftDeleteBy(t *testing.T) {
	model := &SoftDeleteModel{}
	userID := "user123"

	// Soft delete by user
	model.SoftDeleteBy(userID)

	assert.True(t, model.IsDeleted())
	assert.NotNil(t, model.GetDeletedAt())
	assert.Equal(t, userID, model.GetDeletedBy())
	assert.True(t, time.Since(*model.GetDeletedAt()) < time.Second)
}

func TestSoftDeleteModel_Restore(t *testing.T) {
	model := &SoftDeleteModel{}

	// Soft delete first
	model.SoftDeleteBy("user123")
	assert.True(t, model.IsDeleted())

	// Restore
	model.Restore()

	assert.False(t, model.IsDeleted())
	assert.Nil(t, model.GetDeletedAt())
	assert.Empty(t, model.GetDeletedBy())
}

func TestSoftDeleteModel_SettersGetters(t *testing.T) {
	model := &SoftDeleteModel{}
	now := time.Now()
	userID := "user456"

	// Test setters
	model.SetDeletedAt(&now)
	model.SetDeletedBy(userID)

	// Test getters
	assert.Equal(t, &now, model.GetDeletedAt())
	assert.Equal(t, userID, model.GetDeletedBy())
	assert.True(t, model.IsDeleted())
}

func TestExampleModel_ImplementsSoftDeletable(t *testing.T) {
	model := NewExampleModel("id123", "John Doe", "john@example.com")

	// Verify it implements SoftDeletable
	var _ SoftDeletable = model

	// Test initial state
	assert.False(t, model.IsDeleted())
	assert.Equal(t, "id123", model.ID)
	assert.Equal(t, "John Doe", model.Name)
	assert.Equal(t, "john@example.com", model.Email)

	// Test soft delete
	model.SoftDeleteBy("admin")
	assert.True(t, model.IsDeleted())
	assert.Equal(t, "admin", model.GetDeletedBy())

	// Test restore
	model.Restore()
	assert.False(t, model.IsDeleted())
}

func TestSoftDeleteRepository_NewSoftDeleteRepository(t *testing.T) {
	db := new(mocks.MockDB)
	logger := zaptest.NewLogger(t)

	repo := NewSoftDeleteRepository(db, logger)

	assert.NotNil(t, repo)
	assert.Equal(t, db, repo.db)
	assert.Equal(t, logger, repo.logger)
	assert.False(t, repo.includeDeleted)
}

func TestSoftDeleteRepository_WithDeleted(t *testing.T) {
	db := new(mocks.MockDB)
	logger := zaptest.NewLogger(t)
	repo := NewSoftDeleteRepository(db, logger)

	withDeletedRepo := repo.WithDeleted()

	assert.NotSame(t, repo, withDeletedRepo) // Different instances
	assert.True(t, withDeletedRepo.includeDeleted)
	assert.False(t, repo.includeDeleted) // Original unchanged
}

func TestSoftDeleteRepository_OnlyDeleted(t *testing.T) {
	db := new(mocks.MockDB)
	logger := zaptest.NewLogger(t)
	repo := NewSoftDeleteRepository(db, logger)

	onlyDeletedRepo := repo.OnlyDeleted()

	assert.NotSame(t, repo, onlyDeletedRepo)        // Different instances
	assert.False(t, onlyDeletedRepo.includeDeleted) // Will be handled in query methods
}

func TestSoftDeleteRepository_HardDelete(t *testing.T) {
	db := new(mocks.MockDB)
	query := new(mocks.MockQuery)
	logger := zaptest.NewLogger(t)
	repo := NewSoftDeleteRepository(db, logger)

	model := &ExampleModel{
		PK: "EXAMPLE#test-id",
		SK: "PROFILE",
		ID: "test-id",
	}

	db.On("WithContext", mock.Anything).Return(db).Once()
	db.On("Model", model).Return(query).Once()
	query.On("Delete").Return(nil).Once()

	err := repo.HardDelete(context.Background(), model)
	assert.NoError(t, err)

	db.AssertExpectations(t)
	query.AssertExpectations(t)
}

func TestSoftDeleteRepository_SoftDelete(t *testing.T) {
	db := new(mocks.MockDB)
	query := new(mocks.MockQuery)
	logger := zaptest.NewLogger(t)
	repo := NewSoftDeleteRepository(db, logger)

	model := NewExampleModel("user-1", "Example User", "user@example.com")

	db.On("WithContext", mock.Anything).Return(db).Once()
	db.On("Model", model).Return(query).Once()
	query.On("Update", mock.Anything).Return(nil).Once()

	err := repo.SoftDelete(context.Background(), model, "admin")
	assert.NoError(t, err)
	assert.True(t, model.IsDeleted())
	assert.Equal(t, "admin", model.GetDeletedBy())

	db.AssertExpectations(t)
	query.AssertExpectations(t)
}

func TestSoftDeleteRepository_Restore(t *testing.T) {
	db := new(mocks.MockDB)
	query := new(mocks.MockQuery)
	logger := zaptest.NewLogger(t)
	repo := NewSoftDeleteRepository(db, logger)

	model := NewExampleModel("user-1", "Example User", "user@example.com")
	model.SoftDeleteBy("admin")

	db.On("WithContext", mock.Anything).Return(db).Once()
	db.On("Model", model).Return(query).Once()
	query.On("Update", mock.Anything).Return(nil).Once()

	err := repo.Restore(context.Background(), model)
	assert.NoError(t, err)
	assert.False(t, model.IsDeleted())
	assert.Empty(t, model.GetDeletedBy())

	db.AssertExpectations(t)
	query.AssertExpectations(t)
}

// TestSoftDeleteRepository_Query tests query functionality with DynamORM
func TestSoftDeleteRepository_Query(t *testing.T) {
	db := new(mocks.MockDB)
	query := new(mocks.MockQuery)
	logger := zaptest.NewLogger(t)
	repo := NewSoftDeleteRepository(db, logger)

	model := &ExampleModel{}

	t.Run("query returns query builder", func(t *testing.T) {
		db.On("WithContext", mock.Anything).Return(db)
		db.On("Model", model).Return(query)
		query.On("Where", "deleted_at", "attribute_not_exists", interface{}(nil)).Return(query)

		result := repo.Query(context.Background(), model, nil)
		assert.NotNil(t, result)
		assert.IsType(t, &mocks.MockQuery{}, result)
	})

	t.Run("query only deleted returns query builder with deleted filter", func(t *testing.T) {
		db.On("WithContext", mock.Anything).Return(db)
		db.On("Model", model).Return(query)
		query.On("Where", "deleted_at", "attribute_exists", interface{}(nil)).Return(query)

		result := repo.QueryOnlyDeleted(context.Background(), model)
		assert.NotNil(t, result)
		assert.IsType(t, &mocks.MockQuery{}, result)
	})
}

// TestSoftDeleteRepository_Cleanup tests cleanup functionality
func TestSoftDeleteRepository_Cleanup(t *testing.T) {
	db := new(mocks.MockDB)
	query := new(mocks.MockQuery)
	deleteQuery := new(mocks.MockQuery)
	logger := zaptest.NewLogger(t)
	repo := NewSoftDeleteRepository(db, logger)

	model := &ExampleModel{}

	oldItem := NewExampleModel("old-id", "Old User", "old@example.com")
	oldItem.SoftDeleteBy("admin")
	callCount := 0

	db.On("WithContext", mock.Anything).Return(db).Maybe()
	db.On("Model", model).Return(query).Maybe()
	query.On("Where", "deleted_at", "attribute_exists", interface{}(nil)).Return(query).Maybe()
	query.On("Where", "deleted_at", "<", mock.Anything).Return(query).Maybe()
	query.On("Limit", 25).Return(query).Maybe()
	query.On("All", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]*ExampleModel)
		if callCount == 0 {
			*dest = append(*dest, oldItem)
		} else {
			*dest = (*dest)[:0]
		}
		callCount++
	}).Twice()

	db.On("Model", mock.MatchedBy(func(m any) bool {
		em, ok := m.(*ExampleModel)
		return ok && em != model
	})).Return(deleteQuery).Once()
	deleteQuery.On("Delete").Return(nil).Once()

	deleted, err := repo.CleanupOldDeletes(context.Background(), model, 30*24*time.Hour, 25)
	assert.NoError(t, err)
	assert.Equal(t, 1, deleted)

	db.AssertExpectations(t)
	query.AssertExpectations(t)
	deleteQuery.AssertExpectations(t)
}

func TestConvenienceFunctions(t *testing.T) {
	model := NewExampleModel("id123", "Test User", "test@example.com")

	t.Run("IsItemDeleted", func(t *testing.T) {
		assert.False(t, IsItemDeleted(model))

		model.SoftDelete()
		assert.True(t, IsItemDeleted(model))
	})

	t.Run("GetItemDeletionInfo", func(t *testing.T) {
		model.Restore()

		deletedAt, deletedBy, isDeleted := GetItemDeletionInfo(model)
		assert.Nil(t, deletedAt)
		assert.Empty(t, deletedBy)
		assert.False(t, isDeleted)

		model.SoftDeleteBy("admin")
		deletedAt, deletedBy, isDeleted = GetItemDeletionInfo(model)
		assert.NotNil(t, deletedAt)
		assert.Equal(t, "admin", deletedBy)
		assert.True(t, isDeleted)
	})
}

func TestSoftDeleteRepository_GetDeletedItemsOlderThan(t *testing.T) {
	db := new(mocks.MockDB)
	query := new(mocks.MockQuery)
	logger := zaptest.NewLogger(t)
	repo := NewSoftDeleteRepository(db, logger)

	model := &ExampleModel{}
	var dest []*ExampleModel

	db.On("WithContext", mock.Anything).Return(db).Once()
	db.On("Model", model).Return(query).Once()
	query.On("Where", "deleted_at", "attribute_exists", interface{}(nil)).Return(query).Once()
	query.On("Where", "deleted_at", "<", mock.Anything).Return(query).Once()
	query.On("All", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		ptr := args.Get(0).(*[]*ExampleModel)
		*ptr = append(*ptr, NewExampleModel("old-id", "Old User", "old@example.com"))
	}).Once()

	err := repo.GetDeletedItemsOlderThan(context.Background(), model, &dest, 24*time.Hour)
	assert.NoError(t, err)
	assert.Len(t, dest, 1)

	db.AssertExpectations(t)
	query.AssertExpectations(t)
}

// TestSoftDeleteModel_Integration demonstrates the complete soft delete lifecycle with DynamORM
func TestSoftDeleteModel_Integration(t *testing.T) {
	model := NewExampleModel("test-id", "Test User", "test@example.com")

	// Verify DynamORM keys are set properly
	assert.Equal(t, "EXAMPLE#test-id", model.PK)
	assert.Equal(t, "PROFILE", model.SK)

	// Test the complete lifecycle
	assert.False(t, model.IsDeleted())

	// Soft delete
	model.SoftDeleteBy("admin-user")
	assert.True(t, model.IsDeleted())
	assert.Equal(t, "admin-user", model.GetDeletedBy())
	assert.NotNil(t, model.GetDeletedAt())

	// Restore
	model.Restore()
	assert.False(t, model.IsDeleted())
	assert.Nil(t, model.GetDeletedAt())
	assert.Empty(t, model.GetDeletedBy())

	// The model maintains all its original data and keys
	assert.Equal(t, "test-id", model.ID)
	assert.Equal(t, "Test User", model.Name)
	assert.Equal(t, "test@example.com", model.Email)
	assert.Equal(t, "EXAMPLE#test-id", model.PK)
	assert.Equal(t, "PROFILE", model.SK)
}

func TestSoftDeleteRepository_ErrorBranches(t *testing.T) {
	ctx := context.Background()

	t.Run("NewSoftDeleteRepository defaults logger", func(t *testing.T) {
		db := new(mocks.MockDB)
		repo := NewSoftDeleteRepository(db, nil)
		assert.NotNil(t, repo)
		assert.NotNil(t, repo.logger)
		assert.False(t, repo.includeDeleted)
	})

	t.Run("SoftDelete rejects nil model", func(t *testing.T) {
		repo := NewSoftDeleteRepository(new(mocks.MockDB), zaptest.NewLogger(t))
		assert.Error(t, repo.SoftDelete(ctx, nil, "admin"))
	})

	t.Run("SoftDelete propagates update error", func(t *testing.T) {
		db := new(mocks.MockDB)
		query := new(mocks.MockQuery)
		repo := NewSoftDeleteRepository(db, zaptest.NewLogger(t))
		model := NewExampleModel("user-1", "Example User", "user@example.com")

		db.On("WithContext", mock.Anything).Return(db).Once()
		db.On("Model", model).Return(query).Once()
		query.On("Update", mock.Anything).Return(errors.New("update-failed")).Once()

		assert.Error(t, repo.SoftDelete(ctx, model, "admin"))
		db.AssertExpectations(t)
		query.AssertExpectations(t)
	})

	t.Run("Restore rejects nil model", func(t *testing.T) {
		repo := NewSoftDeleteRepository(new(mocks.MockDB), zaptest.NewLogger(t))
		assert.Error(t, repo.Restore(ctx, nil))
	})

	t.Run("Restore rejects not-deleted model", func(t *testing.T) {
		repo := NewSoftDeleteRepository(new(mocks.MockDB), zaptest.NewLogger(t))
		model := NewExampleModel("user-1", "Example User", "user@example.com")
		assert.Error(t, repo.Restore(ctx, model))
	})

	t.Run("Restore propagates update error", func(t *testing.T) {
		db := new(mocks.MockDB)
		query := new(mocks.MockQuery)
		repo := NewSoftDeleteRepository(db, zaptest.NewLogger(t))
		model := NewExampleModel("user-1", "Example User", "user@example.com")
		model.SoftDeleteBy("admin")

		db.On("WithContext", mock.Anything).Return(db).Once()
		db.On("Model", model).Return(query).Once()
		query.On("Update", mock.Anything).Return(errors.New("update-failed")).Once()

		assert.Error(t, repo.Restore(ctx, model))
		db.AssertExpectations(t)
		query.AssertExpectations(t)
	})

	t.Run("HardDelete rejects nil model", func(t *testing.T) {
		repo := NewSoftDeleteRepository(new(mocks.MockDB), zaptest.NewLogger(t))
		assert.Error(t, repo.HardDelete(ctx, nil))
	})

	t.Run("Get filters deleted items when includeDeleted=false", func(t *testing.T) {
		db := new(mocks.MockDB)
		query := new(mocks.MockQuery)
		repo := NewSoftDeleteRepository(db, zaptest.NewLogger(t))

		model := NewExampleModel("user-1", "Example User", "user@example.com")
		model.SoftDeleteBy("admin")

		db.On("WithContext", mock.Anything).Return(db).Once()
		db.On("Model", model).Return(query).Once()
		query.On("Where", "PK", "=", "pk").Return(query).Once()
		query.On("Where", "SK", "=", "sk").Return(query).Once()
		query.On("First", model).Return(nil).Once()

		assert.Error(t, repo.Get(ctx, model, "pk", "sk"))

		withDeleted := repo.WithDeleted()
		db.On("WithContext", mock.Anything).Return(db).Once()
		db.On("Model", model).Return(query).Once()
		query.On("Where", "PK", "=", "pk").Return(query).Once()
		query.On("Where", "SK", "=", "sk").Return(query).Once()
		query.On("First", model).Return(nil).Once()

		assert.NoError(t, withDeleted.Get(ctx, model, "pk", "sk"))
	})

	t.Run("allocateSlice rejects nil model type", func(t *testing.T) {
		_, _, err := allocateSlice(nil)
		assert.Error(t, err)
	})
}

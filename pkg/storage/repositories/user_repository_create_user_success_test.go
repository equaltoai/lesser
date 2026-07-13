package repositories

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	dynamormerrors "github.com/theory-cloud/tabletheory/v2/pkg/errors"
	"github.com/theory-cloud/tabletheory/v2/pkg/mocks"
	"go.uber.org/zap"
)

func TestUserRepository_CreateUser_SuccessPathSetsStorageTimestamps(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewUserRepository(mockDB, "test-table", zap.NewNop())

	// Ensure the enhanced repository doesn't require auth/required-field checks for this unit test.
	repo.SetValidationService(nil)
	repo.SetPermissionService(nil)

	now := time.Now().UTC()

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.User")).Run(func(args mock.Arguments) {
		// Simulate DynamORM hooks mutating timestamps on create.
		out := args.Get(0).(*models.User)
		out.CreatedAt = now.Add(-time.Minute)
		out.UpdatedAt = now
	}).Return(mockQuery)
	mockQuery.On("Create").Return(nil)

	user := &storage.User{
		Username: "alice",
		Email:    "alice@example.com",
		Role:     "user",
	}

	err := repo.CreateUser(context.Background(), user)
	assert.NoError(t, err)
	assert.False(t, user.CreatedAt.IsZero())
	assert.False(t, user.UpdatedAt.IsZero())
}

func TestUserRepository_CreateUser_ConflictErrorPath(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewUserRepository(mockDB, "test-table", zap.NewNop())

	// Allow the create to reach DynamoDB.
	repo.SetValidationService(nil)
	repo.SetPermissionService(nil)

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.User")).Return(mockQuery)
	mockQuery.On("Create").Return(dynamormerrors.ErrConditionFailed)

	user := &storage.User{
		Username: "alice",
		Email:    "alice@example.com",
		Role:     "user",
	}

	err := repo.CreateUser(context.Background(), user)
	assert.Error(t, err)
}

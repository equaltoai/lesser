package repositories

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	dynamormerrors "github.com/theory-cloud/tabletheory/v3/pkg/errors"
	"github.com/theory-cloud/tabletheory/v3/pkg/mocks"
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

	// O(1) instance-count maintenance is best-effort after the create
	// (see instance_counts.go).
	ub := new(mocks.MockUpdateBuilder)
	mockDB.On("Model", mock.AnythingOfType("*models.InstanceMetrics")).Return(mockQuery).Maybe()
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("UpdateBuilder").Return(ub).Maybe()
	ub.On("Add", mock.Anything, mock.Anything).Return(ub).Maybe()
	ub.On("Set", mock.Anything, mock.Anything).Return(ub).Maybe()
	ub.On("Condition", mock.Anything, mock.Anything, mock.Anything).Return(ub).Maybe()
	ub.On("Execute").Return(nil).Maybe()

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

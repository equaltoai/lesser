package repositories

import (
	"context"
	"testing"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	"go.uber.org/zap"
)

func TestUserRepository_M10_GetUserByProviderIDUsesStoredGSI1Prefix(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", &models.ProviderAccount{}).Return(mockQuery)
	mockDB.On("Model", &models.User{}).Return(mockQuery)
	mockQuery.On("Index", "gsi1").Return(mockQuery)
	mockQuery.On("Where", "gsi1PK", "=", "PROVIDER#google").Return(mockQuery)
	mockQuery.On("Where", "gsi1SK", "BEGINS_WITH", "123#").Return(mockQuery)
	mockQuery.On("Limit", 2).Return(mockQuery)
	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]models.ProviderAccount)
		*dest = []models.ProviderAccount{{
			UserID:     "testuser",
			Provider:   "google",
			ProviderID: "123",
			GSI1PK:     "PROVIDER#google",
			GSI1SK:     "123#testuser",
		}}
	}).Return(nil).Once()
	mockQuery.On("Where", "PK", "=", "USER#testuser").Return(mockQuery)
	mockQuery.On("Where", "SK", "=", "METADATA").Return(mockQuery)
	mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*models.User)
		dest.Username = "testuser"
		dest.Role = "user"
		dest.Approved = true
	}).Return(nil).Once()

	repo := NewUserRepository(mockDB, "test-table", zap.NewNop())
	user, err := repo.GetUserByProviderID(ctx, "google", "123")
	require.NoError(t, err)
	require.NotNil(t, user)
	require.Equal(t, "testuser", user.Username)
}

func TestUserRepository_M10_LinkProviderAccountExistingSameUserIsIdempotent(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", &models.User{}).Return(mockQuery)
	mockDB.On("Model", &models.ProviderAccount{}).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", "USER#testuser").Return(mockQuery)
	mockQuery.On("Where", "SK", "=", "METADATA").Return(mockQuery)
	mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*models.User)
		dest.Username = "testuser"
		dest.Role = "user"
		dest.Approved = true
	}).Return(nil).Once()
	mockQuery.On("Index", "gsi1").Return(mockQuery)
	mockQuery.On("Where", "gsi1PK", "=", "PROVIDER#google").Return(mockQuery)
	mockQuery.On("Where", "gsi1SK", "BEGINS_WITH", "123#").Return(mockQuery)
	mockQuery.On("Limit", 2).Return(mockQuery)
	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]models.ProviderAccount)
		*dest = []models.ProviderAccount{{UserID: "testuser", Provider: "google", ProviderID: "123"}}
	}).Return(nil).Once()

	repo := NewUserRepository(mockDB, "test-table", zap.NewNop())
	require.NoError(t, repo.LinkProviderAccount(ctx, "testuser", "google", "123"))
	mockQuery.AssertNotCalled(t, "Create")
}

func TestUserRepository_M10_LinkProviderAccountExistingDifferentUserConflicts(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", &models.User{}).Return(mockQuery)
	mockDB.On("Model", &models.ProviderAccount{}).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", "USER#testuser").Return(mockQuery)
	mockQuery.On("Where", "SK", "=", "METADATA").Return(mockQuery)
	mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*models.User)
		dest.Username = "testuser"
		dest.Role = "user"
		dest.Approved = true
	}).Return(nil).Once()
	mockQuery.On("Index", "gsi1").Return(mockQuery)
	mockQuery.On("Where", "gsi1PK", "=", "PROVIDER#google").Return(mockQuery)
	mockQuery.On("Where", "gsi1SK", "BEGINS_WITH", "123#").Return(mockQuery)
	mockQuery.On("Limit", 2).Return(mockQuery)
	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]models.ProviderAccount)
		*dest = []models.ProviderAccount{{UserID: "otheruser", Provider: "google", ProviderID: "123"}}
	}).Return(nil).Once()

	repo := NewUserRepository(mockDB, "test-table", zap.NewNop())
	err := repo.LinkProviderAccount(ctx, "testuser", "google", "123")
	require.Error(t, err)
	require.Contains(t, err.Error(), "Failed to create provider account")
	mockQuery.AssertNotCalled(t, "Create")
}

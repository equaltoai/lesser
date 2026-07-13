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

func TestUserRepository_CreateAccountNote_SetsTimestampsAndStores(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewUserRepository(mockDB, "test-table", zap.NewNop())

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.AccountNote")).Return(mockQuery)
	mockQuery.On("Create").Return(nil)

	note := &storage.AccountNote{
		Username:      "alice",
		TargetActorID: "https://example.com/users/bob",
		Note:          "hello",
	}

	err := repo.CreateAccountNote(context.Background(), note)

	assert.NoError(t, err)
	assert.False(t, note.CreatedAt.IsZero())
	assert.False(t, note.UpdatedAt.IsZero())
}

func TestUserRepository_CreateAccountNote_CreateError(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewUserRepository(mockDB, "test-table", zap.NewNop())

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.AccountNote")).Return(mockQuery)
	mockQuery.On("Create").Return(ErrTestMockError)

	note := &storage.AccountNote{
		Username:      "alice",
		TargetActorID: "https://example.com/users/bob",
		Note:          "hello",
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}

	err := repo.CreateAccountNote(context.Background(), note)
	assert.Error(t, err)
}

func TestUserRepository_GetAccountNote_NotFound(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewUserRepository(mockDB, "test-table", zap.NewNop())

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.AccountNote")).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("First", mock.Anything).Return(dynamormerrors.ErrItemNotFound)

	note, err := repo.GetAccountNote(context.Background(), "alice", "https://example.com/users/bob")
	assert.Error(t, err)
	assert.Nil(t, note)
}

func TestUserRepository_GetAccountNote_Success(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewUserRepository(mockDB, "test-table", zap.NewNop())

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.AccountNote")).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
		out := args.Get(0).(*models.AccountNote)
		out.Username = "alice"
		out.TargetActorID = "https://example.com/users/bob"
		out.Note = "hello"
		out.CreatedAt = time.Now().Add(-time.Hour)
		out.UpdatedAt = time.Now().Add(-time.Minute)
	}).Return(nil)

	note, err := repo.GetAccountNote(context.Background(), "alice", "https://example.com/users/bob")
	assert.NoError(t, err)
	assert.NotNil(t, note)
	assert.Equal(t, "alice", note.Username)
	assert.Equal(t, "hello", note.Note)
}

func TestUserRepository_GetAccountNote_OtherError(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewUserRepository(mockDB, "test-table", zap.NewNop())

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.AccountNote")).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("First", mock.Anything).Return(ErrTestMockError)

	note, err := repo.GetAccountNote(context.Background(), "alice", "https://example.com/users/bob")
	assert.Error(t, err)
	assert.Nil(t, note)
}

func TestUserRepository_UpdateAccountNote_UpdatesTimestampAndStores(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewUserRepository(mockDB, "test-table", zap.NewNop())

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.AccountNote")).Return(mockQuery)
	mockQuery.On("Create").Return(nil)

	note := &storage.AccountNote{
		Username:      "alice",
		TargetActorID: "https://example.com/users/bob",
		Note:          "updated",
		CreatedAt:     time.Now().Add(-time.Hour),
	}

	err := repo.UpdateAccountNote(context.Background(), note)
	assert.NoError(t, err)
	assert.False(t, note.UpdatedAt.IsZero())
}

func TestUserRepository_UpdateAccountNote_CreateError(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewUserRepository(mockDB, "test-table", zap.NewNop())

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.AccountNote")).Return(mockQuery)
	mockQuery.On("Create").Return(ErrTestMockError)

	note := &storage.AccountNote{
		Username:      "alice",
		TargetActorID: "https://example.com/users/bob",
		Note:          "updated",
		CreatedAt:     time.Now().Add(-time.Hour),
	}

	err := repo.UpdateAccountNote(context.Background(), note)
	assert.Error(t, err)
}

func TestUserRepository_DeleteAccountNote_Deletes(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewUserRepository(mockDB, "test-table", zap.NewNop())

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.AccountNote")).Return(mockQuery)
	mockQuery.On("Delete").Return(nil)

	err := repo.DeleteAccountNote(context.Background(), "alice", "https://example.com/users/bob")
	assert.NoError(t, err)
}

func TestUserRepository_DeleteAccountNote_DeleteError(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewUserRepository(mockDB, "test-table", zap.NewNop())

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.AccountNote")).Return(mockQuery)
	mockQuery.On("Delete").Return(ErrTestMockError)

	err := repo.DeleteAccountNote(context.Background(), "alice", "https://example.com/users/bob")
	assert.Error(t, err)
}

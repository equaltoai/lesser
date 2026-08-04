package repositories

import (
	"context"
	"encoding/json"
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

func TestUserRepository_CreateVouch_GeneratesIDAndStores(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewUserRepository(mockDB, "test-table", zap.NewNop())

	mockDB.On("Model", mock.AnythingOfType("*models.Vouch")).Return(mockQuery)
	mockQuery.On("Create").Return(nil)

	v := &storage.Vouch{
		From:      "alice",
		To:        "bob",
		Active:    true,
		CreatedAt: time.Now().UTC(),
	}

	err := repo.CreateVouch(context.Background(), v)
	assert.NoError(t, err)
	assert.NotEmpty(t, v.ID)
}

func TestUserRepository_GetVouch_NotFound(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewUserRepository(mockDB, "test-table", zap.NewNop())

	mockDB.On("Model", mock.AnythingOfType("*models.Vouch")).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("Scan", mock.Anything).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]*models.Vouch)
		*dest = nil
	}).Return(nil)

	vouch, err := repo.GetVouch(context.Background(), "missing")
	assert.Error(t, err)
	assert.Nil(t, vouch)
}

func TestUserRepository_GetVouch_InvalidJSON(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewUserRepository(mockDB, "test-table", zap.NewNop())

	mockDB.On("Model", mock.AnythingOfType("*models.Vouch")).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("Scan", mock.Anything).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]*models.Vouch)
		*dest = []*models.Vouch{{VouchData: "not-json"}}
	}).Return(nil)

	vouch, err := repo.GetVouch(context.Background(), "bad")
	assert.Error(t, err)
	assert.Nil(t, vouch)
}

func TestUserRepository_GetVouch_Success(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewUserRepository(mockDB, "test-table", zap.NewNop())

	payload, _ := json.Marshal(storage.Vouch{
		ID:        "vouch-1",
		From:      "alice",
		To:        "bob",
		Active:    true,
		CreatedAt: time.Now().UTC(),
	})

	mockDB.On("Model", mock.AnythingOfType("*models.Vouch")).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("Scan", mock.Anything).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]*models.Vouch)
		*dest = []*models.Vouch{{VouchData: string(payload)}}
	}).Return(nil)

	vouch, err := repo.GetVouch(context.Background(), "vouch-1")
	assert.NoError(t, err)
	assert.NotNil(t, vouch)
	assert.Equal(t, "vouch-1", vouch.ID)
	assert.Equal(t, "alice", vouch.From)
}

func TestUserRepository_GetVouchesByActor_FiltersInvalidEntries(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewUserRepository(mockDB, "test-table", zap.NewNop())

	valid, _ := json.Marshal(storage.Vouch{
		ID:        "vouch-1",
		From:      "alice",
		To:        "bob",
		Active:    true,
		CreatedAt: time.Now().UTC(),
	})

	mockDB.On("Model", mock.AnythingOfType("*models.Vouch")).Return(mockQuery)
	mockQuery.On("Index", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("Filter", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("Scan", mock.Anything).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]*models.Vouch)
		*dest = []*models.Vouch{
			{VouchData: ""},            // dropped
			{VouchData: "not-json"},    // dropped
			{VouchData: string(valid)}, // kept
		}
	}).Return(nil)

	vouches, err := repo.GetVouchesByActor(context.Background(), "alice", true)
	assert.NoError(t, err)
	assert.Len(t, vouches, 1)
	assert.Equal(t, "vouch-1", vouches[0].ID)
}

func TestUserRepository_UpdateVouchStatus_UpdatesAndStores(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewUserRepository(mockDB, "test-table", zap.NewNop())

	payload, _ := json.Marshal(storage.Vouch{
		ID:        "vouch-1",
		From:      "alice",
		To:        "bob",
		Active:    true,
		CreatedAt: time.Now().UTC(),
	})

	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("Scan", mock.Anything).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]*models.Vouch)
		*dest = []*models.Vouch{{VouchData: string(payload)}}
	}).Return(nil)
	mockQuery.On("Update", mock.Anything).Return(nil)

	now := time.Now().UTC()
	err := repo.UpdateVouchStatus(context.Background(), "vouch-1", false, &now)
	assert.NoError(t, err)
}

func TestUserRepository_GetMonthlyVouchCount_CountsInRange(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewUserRepository(mockDB, "test-table", zap.NewNop())

	mockDB.On("Model", mock.AnythingOfType("*models.Vouch")).Return(mockQuery)
	mockQuery.On("Index", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("Scan", mock.Anything).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]*models.Vouch)
		*dest = []*models.Vouch{
			{CreatedAt: time.Date(2025, time.January, 2, 0, 0, 0, 0, time.UTC)},
			{CreatedAt: time.Date(2025, time.January, 31, 23, 0, 0, 0, time.UTC)},
			{CreatedAt: time.Date(2025, time.February, 1, 0, 0, 0, 0, time.UTC)},
		}
	}).Return(nil)

	count, err := repo.GetMonthlyVouchCount(context.Background(), "alice", 2025, time.January)
	assert.NoError(t, err)
	assert.Equal(t, 2, count)
}

func TestUserRepository_CreateVouch_ConditionFailed(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewUserRepository(mockDB, "test-table", zap.NewNop())

	mockDB.On("Model", mock.AnythingOfType("*models.Vouch")).Return(mockQuery)
	mockQuery.On("Create").Return(dynamormerrors.ErrConditionFailed)

	v := &storage.Vouch{
		ID:        "vouch-1",
		From:      "alice",
		To:        "bob",
		Active:    true,
		CreatedAt: time.Now().UTC(),
	}

	err := repo.CreateVouch(context.Background(), v)
	assert.Error(t, err)
}

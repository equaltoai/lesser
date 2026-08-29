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

func TestUserRepository_CreateTrustRelationship_GeneratesIDAndInvalidatesCache(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewUserRepository(mockDB, "test-table", zap.NewNop())

	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Create").Return(nil)
	mockQuery.On("Delete").Return(nil)

	relationship := &storage.TrustRelationship{
		TrusterID:  "truster",
		TrusteeID:  "trustee",
		Category:   models.TrustCategoryGeneral,
		Score:      0.75,
		Confidence: 0.9,
	}

	err := repo.CreateTrustRelationship(context.Background(), relationship)
	assert.NoError(t, err)
	assert.NotEmpty(t, relationship.ID)
	assert.NotZero(t, relationship.TTL)
	assert.False(t, relationship.Created.IsZero())
	assert.False(t, relationship.Updated.IsZero())
}

func TestUserRepository_GetTrustRelationship_NotFound(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewUserRepository(mockDB, "test-table", zap.NewNop())

	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("First", mock.Anything).Return(dynamormerrors.ErrItemNotFound)

	rel, err := repo.GetTrustRelationship(context.Background(), "truster", "trustee", "general")
	assert.Error(t, err)
	assert.Nil(t, rel)
}

func TestUserRepository_GetTrustRelationship_Success(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewUserRepository(mockDB, "test-table", zap.NewNop())

	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
		out := args.Get(0).(*models.TrustRelationship)
		out.ID = "trust_123"
		out.TrusterID = "truster"
		out.TrusteeID = "trustee"
		out.Category = models.TrustCategoryGeneral
		out.Score = 0.5
		out.Confidence = 0.8
		out.TTL = time.Now().Add(24 * time.Hour).Unix()
		out.Created = time.Now().Add(-time.Hour)
		out.Updated = time.Now()
	}).Return(nil)

	rel, err := repo.GetTrustRelationship(context.Background(), "truster", "trustee", "general")
	assert.NoError(t, err)
	assert.NotNil(t, rel)
	assert.Equal(t, "trust_123", rel.ID)
	assert.Equal(t, "truster", rel.TrusterID)
	assert.Equal(t, "trustee", rel.TrusteeID)
}

func TestUserRepository_UpdateTrustRelationship_UsesUpsert(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewUserRepository(mockDB, "test-table", zap.NewNop())

	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Create").Return(nil)
	mockQuery.On("Delete").Return(nil)

	relationship := &storage.TrustRelationship{
		ID:         "trust_123",
		TrusterID:  "truster",
		TrusteeID:  "trustee",
		Category:   models.TrustCategoryGeneral,
		Score:      0.5,
		Confidence: 0.8,
		Created:    time.Now().Add(-time.Hour),
	}

	err := repo.UpdateTrustRelationship(context.Background(), relationship)
	assert.NoError(t, err)
	assert.False(t, relationship.Updated.IsZero())
}

func TestUserRepository_DeleteTrustRelationship_DeletesAndInvalidatesCache(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewUserRepository(mockDB, "test-table", zap.NewNop())

	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Delete").Return(nil)

	err := repo.DeleteTrustRelationship(context.Background(), "truster", "trustee", "general")
	assert.NoError(t, err)
}

func TestUserRepository_RecordTrustUpdate_GeneratesEventIDAndStores(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewUserRepository(mockDB, "test-table", zap.NewNop())

	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Create").Return(nil)

	update := &storage.TrustUpdate{
		ActorID:  "actor",
		Category: models.TrustCategoryGeneral,
		Delta:    0.1,
		Reason:   "test",
	}

	err := repo.RecordTrustUpdate(context.Background(), update)
	assert.NoError(t, err)
	assert.NotEmpty(t, update.EventID)
	assert.False(t, update.Timestamp.IsZero())
}

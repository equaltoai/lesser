package theorydb

import (
	"context"
	"errors"
	"testing"
	"time"

	appErrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	dynamormMocks "github.com/theory-cloud/tabletheory/pkg/mocks"
)

func TestModelRepository_Create_SetsKeysAndCallsCreate_Round23(t *testing.T) {
	db := new(dynamormMocks.MockDB)
	q := new(dynamormMocks.MockQuery)

	db.On("Model", mock.Anything).Return(q).Once()
	q.On("Create").Return(nil).Once()

	repo := NewModelRepository(db, "models")
	model := &Model{
		ID:   "123",
		Type: "example",
		Name: "n",
	}

	require.NoError(t, repo.Create(context.Background(), model))
	require.NotZero(t, model.CreatedAt)
	require.NotZero(t, model.UpdatedAt)
	require.NotEmpty(t, model.Timestamp)
	require.Equal(t, "model#123", model.PK)
	require.Equal(t, "model#123", model.SK)
	require.Equal(t, "MODEL_TYPE#example", model.GSI1PK)
	require.Equal(t, model.Timestamp, model.GSI1SK)
}

func TestModelRepository_Create_MapsError_Round23(t *testing.T) {
	db := new(dynamormMocks.MockDB)
	q := new(dynamormMocks.MockQuery)

	db.On("Model", mock.Anything).Return(q).Once()
	q.On("Create").Return(errors.New("create failed")).Once()

	repo := NewModelRepository(db, "models")
	model := &Model{ID: "123", Type: "example"}

	err := repo.Create(context.Background(), model)
	require.Error(t, err)

	appErr, ok := appErrors.AsAppError(err)
	require.True(t, ok)
	require.Equal(t, "failed to create model", appErr.Metadata["context"])
}

func TestModelRepository_Get_UsesKeysAndCallsFirst_Round23(t *testing.T) {
	db := new(dynamormMocks.MockDB)
	q := new(dynamormMocks.MockQuery)

	db.On("Model", mock.Anything).Return(q).Once()
	q.On("Where", "PK", "=", "model#123").Return(q).Once()
	q.On("Where", "SK", "=", "model#123").Return(q).Once()
	q.On("First", mock.Anything).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*Model)
		dest.ID = "123"
		dest.Name = "loaded"
	}).Return(nil).Once()

	repo := NewModelRepository(db, "models")
	model, err := repo.Get(context.Background(), "123")
	require.NoError(t, err)
	require.Equal(t, "123", model.ID)
	require.Equal(t, "loaded", model.Name)
}

func TestModelRepository_Update_SetsUpdatedAtAndCallsUpdate_Round23(t *testing.T) {
	db := new(dynamormMocks.MockDB)
	q := new(dynamormMocks.MockQuery)

	db.On("Model", mock.Anything).Return(q).Once()
	q.On("Update", mock.Anything).Return(nil).Once()

	repo := NewModelRepository(db, "models")
	model := &Model{
		ID:        "123",
		Type:      "example",
		Timestamp: time.Now().Format(time.RFC3339),
	}

	require.NoError(t, repo.Update(context.Background(), model))
	require.NotZero(t, model.UpdatedAt)
	require.Equal(t, "model#123", model.PK)
	require.Equal(t, "model#123", model.SK)
	require.Equal(t, "MODEL_TYPE#example", model.GSI1PK)
	require.Equal(t, model.Timestamp, model.GSI1SK)
}

func TestModelRepository_Delete_MapsError_Round23(t *testing.T) {
	db := new(dynamormMocks.MockDB)
	q := new(dynamormMocks.MockQuery)

	db.On("Model", mock.Anything).Return(q).Once()
	q.On("Delete").Return(errors.New("delete failed")).Once()

	repo := NewModelRepository(db, "models")
	err := repo.Delete(context.Background(), "123")
	require.Error(t, err)

	appErr, ok := appErrors.AsAppError(err)
	require.True(t, ok)
	require.Equal(t, "failed to delete model", appErr.Metadata["context"])
}

func TestModelRepository_List_CallsIndexAndAll_Round23(t *testing.T) {
	db := new(dynamormMocks.MockDB)
	q := new(dynamormMocks.MockQuery)

	db.On("Model", mock.Anything).Return(q).Once()
	q.On("Index", "gsi1").Return(q).Once()
	q.On("Where", "gsi1PK", "=", "MODEL_TYPE#example").Return(q).Once()
	q.On("OrderBy", "gsi1SK", "DESC").Return(q).Once()
	q.On("Limit", 10).Return(q).Once()
	q.On("All", mock.Anything).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]*Model)
		*dest = []*Model{{ID: "1"}, {ID: "2"}}
	}).Return(nil).Once()

	repo := NewModelRepository(db, "models")
	models, err := repo.List(context.Background(), "example", 10)
	require.NoError(t, err)
	require.Len(t, models, 2)
}

package repositories_exttests

import (
	"context"
	"fmt"
	"testing"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	dynamormErrors "github.com/theory-cloud/tabletheory/pkg/errors"
	"github.com/theory-cloud/tabletheory/pkg/mocks"
	"go.uber.org/zap"
)

func TestDomainBlockRepository_ext_error_branches(t *testing.T) {
	ctx := context.Background()

	t.Run("get_instance_domain_block_query_error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
		mockQuery.On("First", mock.Anything).Return(fmt.Errorf("boom")).Once()

		repo := repositories.NewDomainBlockRepository(mockDB, "test-table", zap.NewNop())
		_, err := repo.GetInstanceDomainBlock(ctx, "example.com")
		assert.Error(t, err)
	})

	t.Run("get_instance_domain_block_by_id_not_found", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Index", mock.Anything).Return(mockQuery).Maybe()
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
		mockQuery.On("Limit", mock.Anything).Return(mockQuery).Maybe()
		mockQuery.On("All", mock.AnythingOfType("*[]models.InstanceDomainBlock")).Return(nil).Once()

		repo := repositories.NewDomainBlockRepository(mockDB, "test-table", zap.NewNop())
		_, err := repo.GetInstanceDomainBlockByID(ctx, "missing")
		assert.ErrorIs(t, err, storage.ErrNotFound)
	})

	t.Run("update_instance_domain_block_update_error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()

		mockQuery.On("First", mock.AnythingOfType("*models.InstanceDomainBlock")).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*models.InstanceDomainBlock)
			*dest = models.InstanceDomainBlock{ID: "id1", Domain: "example.com"}
		}).Return(nil).Once()
		mockQuery.On("Update", mock.Anything).Return(fmt.Errorf("boom")).Once()

		repo := repositories.NewDomainBlockRepository(mockDB, "test-table", zap.NewNop())
		err := repo.UpdateInstanceDomainBlock(ctx, "example.com", map[string]any{"severity": "silence"})
		assert.Error(t, err)
	})

	t.Run("delete_instance_domain_block_delete_error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
		mockQuery.On("Delete").Return(dynamormErrors.ErrTransactionFailed).Once()

		repo := repositories.NewDomainBlockRepository(mockDB, "test-table", zap.NewNop())
		assert.Error(t, repo.DeleteInstanceDomainBlock(ctx, "example.com"))
	})
}

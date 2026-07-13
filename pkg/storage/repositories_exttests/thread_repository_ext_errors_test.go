package repositories_exttests

import (
	"context"
	"fmt"
	"testing"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	dynamormErrors "github.com/theory-cloud/tabletheory/v2/pkg/errors"
	"github.com/theory-cloud/tabletheory/v2/pkg/mocks"
	"go.uber.org/zap"
)

func TestThreadRepository_ext_error_branches(t *testing.T) {
	ctx := context.Background()

	t.Run("save_thread_sync_create_error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Create").Return(fmt.Errorf("boom")).Once()

		repo := repositories.NewThreadRepository(mockDB, zap.NewNop())
		assert.Error(t, repo.SaveThreadSync(ctx, models.NewThreadSync("s1")))
	})

	t.Run("get_thread_sync_error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
		mockQuery.On("First", mock.Anything).Return(fmt.Errorf("db error")).Once()

		repo := repositories.NewThreadRepository(mockDB, zap.NewNop())
		_, err := repo.GetThreadSync(ctx, "s1")
		assert.Error(t, err)
	})

	t.Run("save_thread_node_create_error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Create").Return(fmt.Errorf("boom")).Once()

		repo := repositories.NewThreadRepository(mockDB, zap.NewNop())
		assert.Error(t, repo.SaveThreadNode(ctx, models.NewThreadNode("root", "n1", "", 0, "a1")))
	})

	t.Run("get_thread_nodes_scan_error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
		mockQuery.On("Scan", mock.Anything).Return(fmt.Errorf("scan failed")).Once()

		repo := repositories.NewThreadRepository(mockDB, zap.NewNop())
		_, err := repo.GetThreadNodes(ctx, "root")
		assert.Error(t, err)
	})

	t.Run("get_thread_node_not_found", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
		mockQuery.On("First", mock.Anything).Return(dynamormErrors.ErrItemNotFound).Once()

		repo := repositories.NewThreadRepository(mockDB, zap.NewNop())
		_, err := repo.GetThreadNode(ctx, "root", "n1")
		assert.Error(t, err)
	})

	t.Run("get_thread_node_by_status_id_not_found", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Index", mock.Anything).Return(mockQuery).Maybe()
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
		mockQuery.On("First", mock.Anything).Return(dynamormErrors.ErrItemNotFound).Once()

		repo := repositories.NewThreadRepository(mockDB, zap.NewNop())
		_, err := repo.GetThreadNodeByStatusID(ctx, "n1")
		assert.Error(t, err)
	})

	t.Run("get_missing_replies_scan_error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
		mockQuery.On("Scan", mock.Anything).Return(fmt.Errorf("scan failed")).Once()

		repo := repositories.NewThreadRepository(mockDB, zap.NewNop())
		_, err := repo.GetMissingReplies(ctx, "root")
		assert.Error(t, err)
	})

	t.Run("save_missing_reply_create_error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Create").Return(fmt.Errorf("boom")).Once()

		repo := repositories.NewThreadRepository(mockDB, zap.NewNop())
		assert.Error(t, repo.SaveMissingReply(ctx, models.NewMissingReply("root", "p1", "r1")))
	})
}

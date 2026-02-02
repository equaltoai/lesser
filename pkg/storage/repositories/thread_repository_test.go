package repositories

import (
	"context"
	stdErrors "errors"
	"fmt"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/theory-cloud/tabletheory/pkg/errors"
	"github.com/theory-cloud/tabletheory/pkg/mocks"
	"go.uber.org/zap"
)

func TestNewThreadRepository_defaultsLogger(t *testing.T) {
	repo := NewThreadRepository(new(mocks.MockDB), nil)
	assert.NotNil(t, repo)
	assert.NotNil(t, repo.logger)
}

func TestThreadRepository_SaveThreadSync(t *testing.T) {
	t.Run("nil_payload", func(t *testing.T) {
		repo := NewThreadRepository(new(mocks.MockDB), zap.NewNop())
		err := repo.SaveThreadSync(context.Background(), nil)
		assert.Error(t, err)
	})

	t.Run("create_error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		repo := NewThreadRepository(mockDB, zap.NewNop())

		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.AnythingOfType("*models.ThreadSync")).Return(mockQuery)
		mockQuery.On("Create").Return(fmt.Errorf("boom")).Once()

		err := repo.SaveThreadSync(context.Background(), models.NewThreadSync("s1"))
		assert.Error(t, err)

		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
	})

	t.Run("success", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		repo := NewThreadRepository(mockDB, zap.NewNop())

		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.AnythingOfType("*models.ThreadSync")).Return(mockQuery)
		mockQuery.On("Create").Return(nil).Once()

		err := repo.SaveThreadSync(context.Background(), models.NewThreadSync("s1"))
		assert.NoError(t, err)

		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
	})

	t.Run("non_not_found_error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		repo := NewThreadRepository(mockDB, zap.NewNop())

		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.AnythingOfType("*models.ThreadSync")).Return(mockQuery)
		mockQuery.On("Where", "PK", "=", "THREAD_SYNC#s1").Return(mockQuery).Once()
		mockQuery.On("Where", "SK", "=", models.SKMetadata).Return(mockQuery).Once()
		mockQuery.On("First", mock.AnythingOfType("*models.ThreadSync")).Return(fmt.Errorf("boom")).Once()

		sync, err := repo.GetThreadSync(context.Background(), "s1")
		assert.Nil(t, sync)
		assert.Error(t, err)
	})
}

func TestThreadRepository_GetThreadSync(t *testing.T) {
	t.Run("not_found", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		repo := NewThreadRepository(mockDB, zap.NewNop())

		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.AnythingOfType("*models.ThreadSync")).Return(mockQuery)
		mockQuery.On("Where", "PK", "=", "THREAD_SYNC#missing").Return(mockQuery).Once()
		mockQuery.On("Where", "SK", "=", models.SKMetadata).Return(mockQuery).Once()
		mockQuery.On("First", mock.AnythingOfType("*models.ThreadSync")).Return(errors.ErrItemNotFound).Once()

		sync, err := repo.GetThreadSync(context.Background(), "missing")
		assert.Nil(t, sync)
		assert.Error(t, err)
		assert.True(t, stdErrors.Is(err, storage.ErrNotFound))

		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
	})

	t.Run("success", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		repo := NewThreadRepository(mockDB, zap.NewNop())

		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.AnythingOfType("*models.ThreadSync")).Return(mockQuery)
		mockQuery.On("Where", "PK", "=", "THREAD_SYNC#s1").Return(mockQuery).Once()
		mockQuery.On("Where", "SK", "=", models.SKMetadata).Return(mockQuery).Once()
		mockQuery.On("First", mock.AnythingOfType("*models.ThreadSync")).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*models.ThreadSync)
			*dest = *models.NewThreadSync("s1")
			dest.SyncStatus = "completed"
		}).Return(nil).Once()

		sync, err := repo.GetThreadSync(context.Background(), "s1")
		assert.NoError(t, err)
		assert.NotNil(t, sync)
		assert.Equal(t, "s1", sync.StatusID)
		assert.Equal(t, "completed", sync.SyncStatus)

		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
	})
}

func TestThreadRepository_SaveThreadNode_GetThreadNode(t *testing.T) {
	t.Run("save_nil", func(t *testing.T) {
		repo := NewThreadRepository(new(mocks.MockDB), zap.NewNop())
		err := repo.SaveThreadNode(context.Background(), nil)
		assert.Error(t, err)
	})

	t.Run("get_not_found", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		repo := NewThreadRepository(mockDB, zap.NewNop())

		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.AnythingOfType("*models.ThreadNode")).Return(mockQuery)
		mockQuery.On("Where", "PK", "=", "THREAD#root1").Return(mockQuery).Once()
		mockQuery.On("Where", "SK", "=", "NODE#n1").Return(mockQuery).Once()
		mockQuery.On("First", mock.AnythingOfType("*models.ThreadNode")).Return(errors.ErrItemNotFound).Once()

		node, err := repo.GetThreadNode(context.Background(), "root1", "n1")
		assert.Nil(t, node)
		assert.Error(t, err)
		assert.True(t, stdErrors.Is(err, storage.ErrNotFound))
	})

	t.Run("save_create_error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		repo := NewThreadRepository(mockDB, zap.NewNop())

		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.AnythingOfType("*models.ThreadNode")).Return(mockQuery)
		mockQuery.On("Create").Return(fmt.Errorf("nope")).Once()

		node := models.NewThreadNode("root1", "n1", "", 0, "a1")
		err := repo.SaveThreadNode(context.Background(), node)
		assert.Error(t, err)

		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
	})

	t.Run("save_success_and_get_success", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		repo := NewThreadRepository(mockDB, zap.NewNop())

		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.AnythingOfType("*models.ThreadNode")).Return(mockQuery)

		mockQuery.On("Create").Return(nil).Once()
		node := models.NewThreadNode("root1", "n1", "", 0, "a1")
		assert.NoError(t, repo.SaveThreadNode(context.Background(), node))

		mockQuery.On("Where", "PK", "=", "THREAD#root1").Return(mockQuery).Once()
		mockQuery.On("Where", "SK", "=", "NODE#n1").Return(mockQuery).Once()
		mockQuery.On("First", mock.AnythingOfType("*models.ThreadNode")).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*models.ThreadNode)
			*dest = *node
		}).Return(nil).Once()

		out, err := repo.GetThreadNode(context.Background(), "root1", "n1")
		assert.NoError(t, err)
		assert.NotNil(t, out)
		assert.Equal(t, "n1", out.StatusID)

		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
	})
}

func TestThreadRepository_GetThreadNodes_MissingReplies(t *testing.T) {
	t.Run("nodes_scan_error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		repo := NewThreadRepository(mockDB, zap.NewNop())

		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.AnythingOfType("*models.ThreadNode")).Return(mockQuery)
		mockQuery.On("Where", "PK", "=", "THREAD#root1").Return(mockQuery).Once()
		mockQuery.On("Where", "SK", "begins_with", "NODE#").Return(mockQuery).Once()
		mockQuery.On("Scan", mock.AnythingOfType("*[]*models.ThreadNode")).Return(fmt.Errorf("scan failed")).Once()

		nodes, err := repo.GetThreadNodes(context.Background(), "root1")
		assert.Nil(t, nodes)
		assert.Error(t, err)
	})

	t.Run("missing_scan_error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockMissingQuery := new(mocks.MockQuery)
		repo := NewThreadRepository(mockDB, zap.NewNop())

		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.AnythingOfType("*models.MissingReply")).Return(mockMissingQuery)
		mockMissingQuery.On("Where", "PK", "=", "THREAD#root1").Return(mockMissingQuery).Once()
		mockMissingQuery.On("Where", "SK", "begins_with", "MISSING#").Return(mockMissingQuery).Once()
		mockMissingQuery.On("Scan", mock.AnythingOfType("*[]*models.MissingReply")).Return(fmt.Errorf("scan failed")).Once()

		missing, err := repo.GetMissingReplies(context.Background(), "root1")
		assert.Nil(t, missing)
		assert.Error(t, err)

		mockDB.AssertExpectations(t)
		mockMissingQuery.AssertExpectations(t)
	})
}

func TestThreadRepository_MarkMissingReplies(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		repo := NewThreadRepository(new(mocks.MockDB), zap.NewNop())
		err := repo.MarkMissingReplies(context.Background(), "root1", "parent1", nil)
		assert.NoError(t, err)
	})

	t.Run("continues_on_create_error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		repo := NewThreadRepository(mockDB, zap.NewNop())

		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.AnythingOfType("*models.MissingReply")).Return(mockQuery)
		mockQuery.On("Create").Return(fmt.Errorf("fail")).Once()
		mockQuery.On("Create").Return(nil).Once()

		err := repo.MarkMissingReplies(context.Background(), "root1", "parent1", []string{"r1", "r2"})
		assert.NoError(t, err)

		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
	})
}

func TestThreadRepository_GetThreadContext_and_helpers(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewThreadRepository(mockDB, zap.NewNop())

	mockDB.On("WithContext", mock.Anything).Return(mockDB)

	// GetThreadNodeByStatusID query
	mockDB.On("Model", mock.AnythingOfType("*models.ThreadNode")).Return(mockQuery)
	mockQuery.On("Index", "gsi1").Return(mockQuery).Once()
	mockQuery.On("Where", "gsi1PK", "=", "STATUS#s1").Return(mockQuery).Once()
	mockQuery.On("Where", "gsi1SK", "=", "THREAD_NODE").Return(mockQuery).Once()
	mockQuery.On("First", mock.AnythingOfType("*models.ThreadNode")).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*models.ThreadNode)
		*dest = *models.NewThreadNode("root1", "s1", "p1", 1, "a1")
		dest.RootStatusID = "root1"
	}).Return(nil).Once()

	// GetThreadNodes query
	mockQuery.On("Where", "PK", "=", "THREAD#root1").Return(mockQuery).Once()
	mockQuery.On("Where", "SK", "begins_with", "NODE#").Return(mockQuery).Once()
	mockQuery.On("Scan", mock.AnythingOfType("*[]*models.ThreadNode")).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]*models.ThreadNode)
		root := models.NewThreadNode("root1", "root1", "", 0, "a1")
		root.ReplyCount = 1
		child := models.NewThreadNode("root1", "s1", "root1", 1, "a2")
		child.ReplyCount = 0
		*dest = []*models.ThreadNode{root, child}
	}).Return(nil).Once()

	// GetMissingReplies query returns empty but succeeds
	mockDB.On("Model", mock.AnythingOfType("*models.MissingReply")).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", "THREAD#root1").Return(mockQuery).Once()
	mockQuery.On("Where", "SK", "begins_with", "MISSING#").Return(mockQuery).Once()
	mockQuery.On("Scan", mock.AnythingOfType("*[]*models.MissingReply")).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]*models.MissingReply)
		*dest = []*models.MissingReply{}
	}).Return(nil).Once()

	res, err := repo.GetThreadContext(context.Background(), "s1")
	assert.NoError(t, err)
	assert.NotNil(t, res)
	assert.Equal(t, "root1", res.RootStatusID)
	assert.Equal(t, 2, len(res.Nodes))
	assert.Equal(t, 2, res.ParticipantCount)
	assert.Equal(t, 1, res.TotalReplyCount)
	assert.Equal(t, 1, res.MaxDepth)
	assert.Equal(t, 0, res.MissingCount)

	assert.NotNil(t, res.GetRootNode())
	assert.NotEmpty(t, res.GetNodesByDepth())
	assert.Len(t, res.GetChildren("root1"), 1)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestThreadRepository_GetThreadContext_not_found_root(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewThreadRepository(mockDB, zap.NewNop())

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.ThreadNode")).Return(mockQuery)
	mockQuery.On("Index", "gsi1").Return(mockQuery).Once()
	mockQuery.On("Where", "gsi1PK", "=", "STATUS#missing").Return(mockQuery).Once()
	mockQuery.On("Where", "gsi1SK", "=", "THREAD_NODE").Return(mockQuery).Once()
	mockQuery.On("First", mock.AnythingOfType("*models.ThreadNode")).Return(errors.ErrItemNotFound).Once()

	res, err := repo.GetThreadContext(context.Background(), "missing")
	assert.Nil(t, res)
	assert.Error(t, err)
	assert.True(t, stdErrors.Is(err, storage.ErrNotFound))
}

func TestThreadRepository_GetThreadContext_missing_replies_scan_error_continues(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewThreadRepository(mockDB, zap.NewNop())

	mockDB.On("WithContext", mock.Anything).Return(mockDB)

	// GetThreadNodeByStatusID query
	mockDB.On("Model", mock.AnythingOfType("*models.ThreadNode")).Return(mockQuery)
	mockQuery.On("Index", "gsi1").Return(mockQuery).Once()
	mockQuery.On("Where", "gsi1PK", "=", "STATUS#s1").Return(mockQuery).Once()
	mockQuery.On("Where", "gsi1SK", "=", "THREAD_NODE").Return(mockQuery).Once()
	mockQuery.On("First", mock.AnythingOfType("*models.ThreadNode")).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*models.ThreadNode)
		*dest = *models.NewThreadNode("root1", "s1", "root1", 1, "a1")
	}).Return(nil).Once()

	// GetThreadNodes query
	mockQuery.On("Where", "PK", "=", "THREAD#root1").Return(mockQuery).Once()
	mockQuery.On("Where", "SK", "begins_with", "NODE#").Return(mockQuery).Once()
	mockQuery.On("Scan", mock.AnythingOfType("*[]*models.ThreadNode")).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]*models.ThreadNode)
		root := models.NewThreadNode("root1", "root1", "", 0, "a1")
		*dest = []*models.ThreadNode{root}
	}).Return(nil).Once()

	// GetMissingReplies scan fails but should be ignored in GetThreadContext.
	mockDB.On("Model", mock.AnythingOfType("*models.MissingReply")).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", "THREAD#root1").Return(mockQuery).Once()
	mockQuery.On("Where", "SK", "begins_with", "MISSING#").Return(mockQuery).Once()
	mockQuery.On("Scan", mock.AnythingOfType("*[]*models.MissingReply")).Return(fmt.Errorf("scan failed")).Once()

	res, err := repo.GetThreadContext(context.Background(), "s1")
	assert.NoError(t, err)
	assert.NotNil(t, res)
	assert.Empty(t, res.MissingReplies)
	assert.Equal(t, 0, res.MissingCount)
}

func TestThreadContextResult_GetRootNode_returns_nil_when_missing(t *testing.T) {
	result := &ThreadContextResult{
		Nodes: []*models.ThreadNode{
			models.NewThreadNode("root1", "child1", "root1", 1, "a1"),
		},
	}

	assert.Nil(t, result.GetRootNode())
}

func TestThreadRepository_SaveDeleteMissingReply_and_bulk(t *testing.T) {
	t.Run("save_and_delete_errors", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		repo := NewThreadRepository(mockDB, zap.NewNop())

		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)

		mockQuery.On("Create").Return(fmt.Errorf("create failed")).Once()
		err := repo.SaveMissingReply(context.Background(), models.NewMissingReply("root1", "p1", "r1"))
		assert.Error(t, err)

		mockQuery.On("Delete").Return(fmt.Errorf("delete failed")).Once()
		err = repo.DeleteMissingReply(context.Background(), "root1", "r1")
		assert.Error(t, err)

		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
	})

	t.Run("pending_missing_replies_is_stubbed", func(t *testing.T) {
		repo := NewThreadRepository(new(mocks.MockDB), zap.NewNop())
		out, err := repo.GetPendingMissingReplies(context.Background(), 10)
		assert.NoError(t, err)
		assert.Empty(t, out)
	})

	t.Run("bulk_save_batches_and_continues_on_error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		repo := NewThreadRepository(mockDB, zap.NewNop())

		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.AnythingOfType("*models.ThreadNode")).Return(mockQuery)

		mockQuery.On("Create").Return(fmt.Errorf("boom")).Once()
		mockQuery.On("Create").Return(nil).Maybe()

		nodes := make([]*models.ThreadNode, 0, 26)
		for i := 0; i < 26; i++ {
			node := models.NewThreadNode("root1", fmt.Sprintf("n%d", i), "", 0, "a1")
			// Ensure UpdateKeys executes TTL logic.
			now := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
			node.UpdatedAt = now
			nodes = append(nodes, node)
		}

		err := repo.BulkSaveThreadNodes(context.Background(), nodes)
		assert.NoError(t, err)

		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
	})

	t.Run("save_missing_reply_success_and_delete_success", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		repo := NewThreadRepository(mockDB, zap.NewNop())

		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Create").Return(nil).Once()
		assert.NoError(t, repo.SaveMissingReply(context.Background(), models.NewMissingReply("root1", "p1", "r1")))

		mockQuery.On("Delete").Return(nil).Once()
		assert.NoError(t, repo.DeleteMissingReply(context.Background(), "root1", "r1"))

		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
	})
}

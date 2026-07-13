package repositories_exttests

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	dynamormErrors "github.com/theory-cloud/tabletheory/v2/pkg/errors"
	"github.com/theory-cloud/tabletheory/v2/pkg/mocks"
	"go.uber.org/zap"
)

func TestThreadRepository_ext_sweep(t *testing.T) {
	ctx := context.Background()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
	mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()

	mockQuery.On("Index", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Scan", mock.Anything).Run(func(args mock.Arguments) {
		switch dest := args.Get(0).(type) {
		case *[]*models.ThreadNode:
			root := models.NewThreadNode("root", "root", "", 0, "a1")
			root.ReplyCount = 1
			child := models.NewThreadNode("root", "n1", "root", 1, "a2")
			*dest = []*models.ThreadNode{root, child}
		case *[]*models.MissingReply:
			*dest = []*models.MissingReply{}
		}
	}).Return(nil).Maybe()
	mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
		switch dest := args.Get(0).(type) {
		case *models.ThreadNode:
			*dest = *models.NewThreadNode("root", "n1", "p1", 1, "a1")
		case *models.ThreadSync:
			*dest = *models.NewThreadSync("s1")
		}
	}).Return(nil).Maybe()
	mockQuery.On("Create").Return(nil).Maybe()
	mockQuery.On("Delete").Return(nil).Maybe()

	repo := repositories.NewThreadRepository(mockDB, zap.NewNop())
	repoNilLogger := repositories.NewThreadRepository(mockDB, nil)
	assert.NotNil(t, repoNilLogger)

	assert.Error(t, repo.SaveThreadSync(ctx, nil))
	assert.NoError(t, repo.SaveThreadSync(ctx, models.NewThreadSync("s1")))

	mockQuery.On("First", mock.Anything).Return(dynamormErrors.ErrItemNotFound).Once()
	_, _ = repo.GetThreadSync(ctx, "missing")

	assert.Error(t, repo.SaveThreadNode(ctx, nil))
	assert.NoError(t, repo.SaveThreadNode(ctx, models.NewThreadNode("root", "n1", "", 0, "a1")))
	_, _ = repo.GetThreadNodes(ctx, "root")
	_, _ = repo.GetThreadNode(ctx, "root", "n1")
	_, _ = repo.GetThreadNodeByStatusID(ctx, "n1")

	assert.NoError(t, repo.MarkMissingReplies(ctx, "root", "p1", nil))
	assert.NoError(t, repo.MarkMissingReplies(ctx, "root", "p1", []string{"r1", "r2"}))
	_, _ = repo.GetMissingReplies(ctx, "root")

	// Context: still returns even if missing replies scan errors.
	mockQuery.On("Scan", mock.AnythingOfType("*[]*models.MissingReply")).Return(fmt.Errorf("scan failed")).Once()
	res, err := repo.GetThreadContext(ctx, "n1")
	assert.NoError(t, err)
	assert.NotNil(t, res)
	assert.NotNil(t, res.GetRootNode())
	assert.NotEmpty(t, res.GetNodesByDepth())
	assert.NotEmpty(t, res.GetChildren("root"))

	emptyRoot := &repositories.ThreadContextResult{
		Nodes: []*models.ThreadNode{models.NewThreadNode("root", "n1", "root", 1, "a1")},
	}
	assert.Nil(t, emptyRoot.GetRootNode())

	assert.NoError(t, repo.SaveMissingReply(ctx, models.NewMissingReply("root", "p1", "r1")))
	assert.NoError(t, repo.DeleteMissingReply(ctx, "root", "r1"))
	_, _ = repo.GetPendingMissingReplies(ctx, 10)

	nodes := make([]*models.ThreadNode, 0, 26)
	for i := 0; i < 26; i++ {
		node := models.NewThreadNode("root", fmt.Sprintf("n%d", i), "", 0, "a1")
		node.UpdatedAt = time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
		nodes = append(nodes, node)
	}
	assert.NoError(t, repo.BulkSaveThreadNodes(ctx, nodes))
}

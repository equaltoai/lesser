package repositories

import (
	"context"
	"fmt"
	"testing"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	dynamormerrors "github.com/theory-cloud/tabletheory/v2/pkg/errors"
	"github.com/theory-cloud/tabletheory/v2/pkg/mocks"
	"go.uber.org/zap"
)

func TestRelationshipPagination_buildRelationshipQuery_modelType_and_cursor(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	ctx := context.Background()

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Index", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("Limit", mock.Anything).Return(mockQuery)
	mockQuery.On("Cursor", mock.Anything).Return(mockQuery)

	_, err := buildRelationshipQuery(ctx, mockDB, "alice", 10, "", RelationshipPaginationConfig{ModelType: "Unknown"})
	assert.ErrorIs(t, err, ErrRelationshipPaginationModelTypeUnsupported)

	_, err = buildRelationshipQuery(ctx, mockDB, "alice", 10, "cur", RelationshipPaginationConfig{
		ModelType:   modelTypeBlock,
		IndexName:   "gsi5",
		PKFormat:    "BLOCKED#%s",
		ErrorPrefix: "blocked",
	})
	assert.NoError(t, err)

	mockQuery.AssertCalled(t, "Index", "gsi5")
	mockQuery.AssertCalled(t, "Where", "gsi5PK", "=", "BLOCKED#alice")
	mockQuery.AssertCalled(t, "Cursor", "cur")
}

func TestRelationshipPagination_executeBlock_and_mute_queries(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()

	ctx := context.Background()
	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("Index", mock.Anything).Return(mockQuery)
	mockQuery.On("Limit", mock.Anything).Return(mockQuery)
	mockQuery.On("Cursor", mock.Anything).Return(mockQuery)

	t.Run("block branch uses gsi5 cursor and truncates", func(t *testing.T) {
		mockQuery.On("All", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
			out := args.Get(0).(*[]models.Block)
			*out = []models.Block{
				{Actor: "a1", Object: "o1", SK: "SK1", GSI5SK: "G5_1"},
				{Actor: "a2", Object: "o2", SK: "SK2", GSI5SK: "G5_2"},
			}
		}).Once()

		results, next, err := getPaginatedBlockList(ctx, mockDB, logger, "alice", 1, "", RelationshipPaginationConfig{
			IndexName:   "gsi5",
			PKFormat:    "BLOCKED#%s",
			SKField:     "gsi5SK",
			ActorField:  "Actor",
			ErrorPrefix: "blocked users",
		})
		assert.NoError(t, err)
		assert.Equal(t, []string{"a1"}, results)
		assert.Equal(t, "G5_1", next)
	})

	t.Run("mute branch uses gsi1SK cursor and handles query error", func(t *testing.T) {
		mockQuery.On("All", mock.Anything).Return(fmt.Errorf("query failed")).Once()

		_, _, err := getPaginatedMuteList(ctx, mockDB, logger, "alice", 10, "", RelationshipPaginationConfig{
			IndexName:   "gsi1",
			PKFormat:    "ACTOR#%s#MUTES",
			SKField:     gsi1SKField,
			ActorField:  "Object",
			ErrorPrefix: "muted users",
		})
		assert.ErrorIs(t, err, ErrRelationshipPaginationQueryFailed)
	})

	t.Run("mute results use SK cursor when configured", func(t *testing.T) {
		mockQuery.On("All", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
			out := args.Get(0).(*[]models.Mute)
			*out = []models.Mute{
				{Actor: "a1", Object: "o1", SK: "SK1", GSI1SK: "G1_1"},
				{Actor: "a2", Object: "o2", SK: "SK2", GSI1SK: "G1_2"},
			}
		}).Once()

		results, next, err := getPaginatedMuteList(ctx, mockDB, logger, "alice", 1, "", RelationshipPaginationConfig{
			IndexName:   "",
			PKFormat:    "ACTOR#%s#MUTES",
			SKField:     "SK",
			ActorField:  "Object",
			ErrorPrefix: "muted users",
		})
		assert.NoError(t, err)
		assert.Equal(t, []string{"o1"}, results)
		assert.Equal(t, "SK1", next)
	})

	t.Run("block list handles conditional cursor absence", func(t *testing.T) {
		mockQuery.On("All", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
			out := args.Get(0).(*[]models.Block)
			*out = []models.Block{{Actor: "a1", Object: "o1", SK: "SK1"}}
		}).Once()

		results, next, err := getPaginatedBlockList(ctx, mockDB, logger, "alice", 10, "", RelationshipPaginationConfig{
			IndexName:   "",
			PKFormat:    "ACTOR#%s#BLOCKS",
			SKField:     "SK",
			ActorField:  "Object",
			ErrorPrefix: "blocked users",
		})
		assert.NoError(t, err)
		assert.Equal(t, []string{"o1"}, results)
		assert.Empty(t, next)
	})
}

func TestRelationshipPagination_generateCursor_helpers(t *testing.T) {
	assert.Empty(t, generateBlockCursor([]models.Block{{SK: "1"}}, 10, "SK"))
	assert.Equal(t, "G5", generateBlockCursor([]models.Block{{GSI5SK: "G5"}, {GSI5SK: "G5"}}, 1, "gsi5SK"))
	assert.Equal(t, "SK", generateBlockCursor([]models.Block{{SK: "SK"}, {SK: "SK"}}, 1, "SK"))

	assert.Empty(t, generateMuteCursor([]models.Mute{{SK: "1"}}, 10, "SK"))
	assert.Equal(t, "G1", generateMuteCursor([]models.Mute{{GSI1SK: "G1"}, {GSI1SK: "G1"}}, 1, gsi1SKField))
	assert.Equal(t, "SK", generateMuteCursor([]models.Mute{{SK: "SK"}, {SK: "SK"}}, 1, "SK"))

	// Ensure test hits the dynamorm errors symbol to avoid unused imports in older Go tooling.
	assert.True(t, dynamormerrors.IsNotFound(dynamormerrors.ErrItemNotFound))
}

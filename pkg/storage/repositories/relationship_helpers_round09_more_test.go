package repositories

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	"go.uber.org/zap"
)

func TestRound09_RelationshipHelper_QueryErrorBranches(t *testing.T) {
	t.Parallel()

	baseTime := time.Now().UTC()
	ctx := context.Background()

	{
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("First", mock.Anything).Return(errors.New("boom")).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		helper := NewRelationshipHelper(mockDB, zap.NewNop(), "block")
		_, err := helper.CheckRelationship(ctx,
			"https://example.com/users/alice",
			"https://example.com/users/bob",
			"ACTOR#%s#BLOCKS",
			"BLOCKED#%s",
			&models.Block{},
		)
		require.Error(t, err)
	}

	{
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("All", mock.Anything).Return(errors.New("boom")).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		helper := NewRelationshipHelper(mockDB, zap.NewNop(), "mute")
		_, _, err := helper.GetRelatedUsers(ctx,
			"https://example.com/users/alice",
			10,
			"",
			"ACTOR#%s#MUTES",
			&models.Mute{},
			"Object",
		)
		require.Error(t, err)
	}

	{
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("All", mock.Anything).Return(errors.New("boom")).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		helper := NewRelationshipHelper(mockDB, zap.NewNop(), "block")
		_, _, err := helper.GetUsersWhoRelated(ctx,
			"https://example.com/users/bob",
			10,
			"",
			"gsi5",
			"BLOCKED#%s",
			&models.Block{},
			"Actor",
		)
		require.Error(t, err)
	}
}

func TestRound09_RelationshipHelper_BlockPaginationCursor(t *testing.T) {
	t.Parallel()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		ptr, ok := args.Get(0).(*[]interface{})
		if !ok {
			return
		}
		a := &models.Block{Object: "https://example.com/users/bob", SK: "BLOCKED#bob"}
		b := &models.Block{Object: "https://example.com/users/cat", SK: "BLOCKED#cat"}
		*ptr = append(*ptr, a, b)
	}).Return(nil).Once()
	setupPermissiveRound08Mocks(mockDB, mockQuery, nil, time.Now().UTC())

	helper := NewRelationshipHelper(mockDB, zap.NewNop(), "block")
	users, cursor, err := helper.GetRelatedUsers(context.Background(),
		"https://example.com/users/alice",
		1,
		"",
		"ACTOR#%s#BLOCKS",
		&models.Block{},
		"Object",
	)
	require.NoError(t, err)
	require.Equal(t, []string{"https://example.com/users/bob"}, users)
	require.Equal(t, "BLOCKED#bob", cursor)
}

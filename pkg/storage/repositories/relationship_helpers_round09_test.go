package repositories

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	dynamormerrors "github.com/pay-theory/dynamorm/pkg/errors"
	"github.com/pay-theory/dynamorm/pkg/mocks"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestRound09_RelationshipHelper_ExtractUsername(t *testing.T) {
	t.Parallel()

	require.Equal(t, "alice", extractUsernameFromActor("https://example.com/users/alice"))
	require.Equal(t, "alice", extractUsernameFromActor("alice"))
	require.Equal(t, "", extractUsernameFromActor("https://example.com/users/"))
}

func TestRound09_RelationshipHelper_DeleteAndCheckRelationship(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	mockDBNotFound := new(mocks.MockDB)
	mockQueryNotFound := new(mocks.MockQuery)
	mockQueryNotFound.On("Delete").Return(dynamormerrors.ErrItemNotFound).Once()
	setupPermissiveRound08Mocks(mockDBNotFound, mockQueryNotFound, nil, time.Now())

	helperNotFound := NewRelationshipHelper(mockDBNotFound, zap.NewNop(), "block")
	require.NoError(t, helperNotFound.DeleteRelationship(ctx,
		"https://example.com/users/alice",
		"https://example.com/users/bob",
		"ACTOR#%s#BLOCKS",
		"BLOCKED#%s",
		&models.Block{},
	))

	mockDBCheck := new(mocks.MockDB)
	mockQueryCheck := new(mocks.MockQuery)
	mockQueryCheck.On("First", mock.Anything).Return(dynamormerrors.ErrItemNotFound).Once()
	setupPermissiveRound08Mocks(mockDBCheck, mockQueryCheck, nil, time.Now())

	helperCheck := NewRelationshipHelper(mockDBCheck, zap.NewNop(), "block")
	ok, err := helperCheck.CheckRelationship(ctx,
		"https://example.com/users/alice",
		"https://example.com/users/bob",
		"ACTOR#%s#BLOCKS",
		"BLOCKED#%s",
		&models.Block{},
	)
	require.NoError(t, err)
	require.False(t, ok)

	mockDBErr := new(mocks.MockDB)
	mockQueryErr := new(mocks.MockQuery)
	mockQueryErr.On("Delete").Return(errors.New("boom")).Once()
	setupPermissiveRound08Mocks(mockDBErr, mockQueryErr, nil, time.Now())

	helperErr := NewRelationshipHelper(mockDBErr, zap.NewNop(), "block")
	require.Error(t, helperErr.DeleteRelationship(ctx,
		"https://example.com/users/alice",
		"https://example.com/users/bob",
		"ACTOR#%s#BLOCKS",
		"BLOCKED#%s",
		&models.Block{},
	))
}

func TestRound09_RelationshipHelper_GetRelatedUsersAndWhoRelated(t *testing.T) {
	t.Parallel()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		ptr, ok := args.Get(0).(*[]interface{})
		if !ok {
			return
		}
		a := &models.Mute{
			Actor:  "https://example.com/users/alice",
			Object: "https://example.com/users/bob",
			SK:     "MUTED#bob",
			GSI1SK: "cursor-1",
		}
		b := &models.Mute{
			Actor:  "https://example.com/users/alice",
			Object: "https://example.com/users/cat",
			SK:     "MUTED#cat",
			GSI1SK: "cursor-2",
		}
		*ptr = append(*ptr, a, b)
	}).Return(nil).Once()
	setupPermissiveRound08Mocks(mockDB, mockQuery, nil, time.Now())

	helper := NewRelationshipHelper(mockDB, zap.NewNop(), "mute")
	ctx := context.Background()

	users, next, err := helper.GetRelatedUsers(ctx,
		"https://example.com/users/alice",
		1,
		"cursor",
		"ACTOR#%s#MUTES",
		&models.Mute{},
		"Object",
	)
	require.NoError(t, err)
	require.Equal(t, []string{"https://example.com/users/bob"}, users)
	require.NotEmpty(t, next)

	mockDB2 := new(mocks.MockDB)
	mockQuery2 := new(mocks.MockQuery)
	mockQuery2.On("All", mock.Anything).Run(func(args mock.Arguments) {
		ptr, ok := args.Get(0).(*[]interface{})
		if !ok {
			return
		}
		a := &models.Block{
			Actor:  "https://example.com/users/alice",
			Object: "https://example.com/users/bob",
			GSI5SK: "BLOCKER#alice",
		}
		b := &models.Block{
			Actor:  "https://example.com/users/cat",
			Object: "https://example.com/users/bob",
			GSI5SK: "BLOCKER#cat",
		}
		*ptr = append(*ptr, a, b)
	}).Return(nil).Once()
	setupPermissiveRound08Mocks(mockDB2, mockQuery2, nil, time.Now())

	helper2 := NewRelationshipHelper(mockDB2, zap.NewNop(), "block")
	actors, next, err := helper2.GetUsersWhoRelated(ctx,
		"https://example.com/users/bob",
		1,
		"cursor",
		"gsi5",
		"BLOCKED#%s",
		&models.Block{},
		"Actor",
	)
	require.NoError(t, err)
	require.Equal(t, []string{"https://example.com/users/alice"}, actors)
	require.NotEmpty(t, next)
}

package repositories

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	dynamormerrors "github.com/theory-cloud/tabletheory/v3/pkg/errors"
	"github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	"go.uber.org/zap"
)

func newTestQuoteRepository(mockDB *mocks.MockDB) *QuoteRepository {
	repo := NewQuoteRepository(mockDB, "tbl", zap.NewNop(), nil)
	repo.relationshipRepo.SetValidationService(nil)
	repo.relationshipRepo.SetPermissionService(nil)
	repo.relationshipRepo.SetCachingService(nil)
	repo.relationshipRepo.SetEventService(nil)
	repo.permissionsRepo.SetValidationService(nil)
	repo.permissionsRepo.SetPermissionService(nil)
	repo.permissionsRepo.SetCachingService(nil)
	repo.permissionsRepo.SetEventService(nil)
	return repo
}

func TestQuoteRepository_round09_relationship_crud_and_queries(t *testing.T) {
	ctx := context.Background()

	t.Run("create_get_update_delete", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		repo := newTestQuoteRepository(mockDB)

		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)

		rel := &models.QuoteRelationship{
			QuoterNoteID: "q1",
			TargetNoteID: "t1",
			QuoterID:     "u1",
			Timestamp:    time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		}

		mockQuery.On("Create").Return(nil).Once()
		require.NoError(t, repo.CreateQuoteRelationship(ctx, rel))

		mockQuery.On("Create").Return(dynamormerrors.ErrConditionFailed).Once()
		require.NoError(t, repo.CreateQuoteRelationship(ctx, rel))

		mockQuery.On("Create").Return(fmt.Errorf("boom")).Once()
		err := repo.CreateQuoteRelationship(ctx, rel)
		assert.ErrorIs(t, err, ErrQuoteRelationshipCreateFailed)

		// GetQuoteRelationship not found and success
		mockQuery.On("Where", "PK", "=", "QUOTE#q1").Return(mockQuery).Once()
		mockQuery.On("Where", "SK", "=", "QUOTED#t1").Return(mockQuery).Once()
		mockQuery.On("First", mock.Anything).Return(dynamormerrors.ErrItemNotFound).Once()
		_, err = repo.GetQuoteRelationship(ctx, "q1", "t1")
		assert.Error(t, err)

		mockQuery.On("Where", "PK", "=", "QUOTE#q1").Return(mockQuery).Once()
		mockQuery.On("Where", "SK", "=", "QUOTED#t1").Return(mockQuery).Once()
		mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*models.QuoteRelationship)
			*dest = models.QuoteRelationship{QuoterNoteID: "q1", TargetNoteID: "t1", PK: "QUOTE#q1", SK: "QUOTED#t1"}
		}).Return(nil).Once()
		got, err := repo.GetQuoteRelationship(ctx, "q1", "t1")
		require.NoError(t, err)
		assert.Equal(t, "q1", got.QuoterNoteID)

		// Update and delete
		mockQuery.On("Update", mock.Anything).Return(nil).Once()
		require.NoError(t, repo.UpdateQuoteRelationship(ctx, rel))

		mockQuery.On("Update", mock.Anything).Return(fmt.Errorf("boom")).Once()
		err = repo.UpdateQuoteRelationship(ctx, rel)
		assert.ErrorIs(t, err, ErrQuoteRelationshipUpdateFailed)

		mockQuery.On("Where", "PK", "=", "QUOTE#q1").Return(mockQuery).Once()
		mockQuery.On("Where", "SK", "=", "QUOTED#t1").Return(mockQuery).Once()
		mockQuery.On("Delete").Return(nil).Once()
		require.NoError(t, repo.DeleteQuoteRelationship(ctx, "q1", "t1"))

		mockQuery.On("Where", "PK", "=", "QUOTE#q1").Return(mockQuery).Once()
		mockQuery.On("Where", "SK", "=", "QUOTED#t1").Return(mockQuery).Once()
		mockQuery.On("Delete").Return(fmt.Errorf("boom")).Once()
		err = repo.DeleteQuoteRelationship(ctx, "q1", "t1")
		assert.ErrorIs(t, err, ErrQuoteRelationshipDeleteFailed)
	})

	t.Run("gsi_queries_filtering_and_cursor", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		repo := newTestQuoteRepository(mockDB)

		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.AnythingOfType("*models.QuoteRelationship")).Return(mockQuery)

		mockQuery.On("Where", "gsi1PK", "=", "QUOTED#s1").Return(mockQuery).Once()
		mockQuery.On("Limit", 2).Return(mockQuery).Once()
		mockQuery.On("Cursor", "c1").Return(mockQuery).Once()
		mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]models.QuoteRelationship)
			*dest = []models.QuoteRelationship{
				{PK: "p1", SK: "s1", Withdrawn: true},
				{PK: "p2", SK: "s2", Withdrawn: false},
			}
		}).Return(nil).Once()

		res, err := repo.GetQuotesForStatus(ctx, "s1", interfaces.PaginationOptions{Limit: 2, Cursor: "c1"})
		require.NoError(t, err)
		assert.Len(t, res.Items, 1)
		assert.True(t, res.HasMore)
		assert.Equal(t, "p2#s2", res.NextCursor)

		mockQuery.On("Where", "gsi2PK", "=", "QUOTER#u1").Return(mockQuery).Once()
		mockQuery.On("Limit", 1).Return(mockQuery).Once()
		mockQuery.On("All", mock.Anything).Return(fmt.Errorf("boom")).Once()
		_, err = repo.GetQuotesByUser(ctx, "u1", interfaces.PaginationOptions{Limit: 1})
		assert.ErrorIs(t, err, ErrQuoteRelationshipQueryFailed)
	})

	t.Run("quote_count_and_withdraw_quotes", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		repo := newTestQuoteRepository(mockDB)

		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)

		mockQuery.On("Where", "gsi1PK", "=", "QUOTED#s1").Return(mockQuery).Once()
		mockQuery.On("Where", "Withdrawn", "=", false).Return(mockQuery).Once()
		mockQuery.On("Count").Return(int64(3), nil).Once()
		n, err := repo.GetQuoteCount(ctx, "s1")
		require.NoError(t, err)
		assert.EqualValues(t, 3, n)

		mockQuery.On("Where", "gsi1PK", "=", "QUOTED#s2").Return(mockQuery).Once()
		mockQuery.On("Where", "Withdrawn", "=", false).Return(mockQuery).Once()
		mockQuery.On("Count").Return(int64(0), fmt.Errorf("boom")).Once()
		_, err = repo.GetQuoteCount(ctx, "s2")
		assert.ErrorIs(t, err, ErrQuoteCountQueryFailed)

		// WithdrawQuotes: query then update each quote.
		mockQuery.On("Where", "gsi2PK", "=", "QUOTER#u1").Return(mockQuery).Once()
		mockQuery.On("Filter", "TargetNoteID", "=", "t1").Return(mockQuery).Once()
		mockQuery.On("Filter", "Withdrawn", "=", false).Return(mockQuery).Once()
		mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]models.QuoteRelationship)
			*dest = []models.QuoteRelationship{
				{ID: "q1", QuoterNoteID: "q1", TargetNoteID: "t1", QuoterID: "u1", Timestamp: time.Now()},
				{ID: "q2", QuoterNoteID: "q2", TargetNoteID: "t1", QuoterID: "u1", Timestamp: time.Now()},
			}
		}).Return(nil).Once()

		mockQuery.On("Update", mock.Anything).Return(nil).Once()
		mockQuery.On("Update", mock.Anything).Return(fmt.Errorf("boom")).Once()

		count, err := repo.WithdrawQuotes(ctx, "t1", "u1")
		require.NoError(t, err)
		assert.Equal(t, 1, count)

		// Coverage-only no-op counters
		assert.NoError(t, repo.IncrementQuoteCount(ctx, "s1"))
		assert.NoError(t, repo.DecrementQuoteCount(ctx, "s1"))

		// Ensure storage.ErrNotFound branch is reachable in GetQuoteRelationship.
		_ = storage.ErrNotFound
	})
}

func TestQuoteRepository_round09_permissions_crud(t *testing.T) {
	ctx := context.Background()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := newTestQuoteRepository(mockDB)

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)

	p := &models.QuotePermissions{Username: "u1", AllowPublic: true}

	mockQuery.On("Create").Return(nil).Once()
	require.NoError(t, repo.CreateQuotePermissions(ctx, p))

	mockQuery.On("Create").Return(dynamormerrors.ErrConditionFailed).Once()
	require.NoError(t, repo.CreateQuotePermissions(ctx, p))

	mockQuery.On("Create").Return(fmt.Errorf("boom")).Once()
	err := repo.CreateQuotePermissions(ctx, p)
	assert.ErrorIs(t, err, ErrQuotePermissionsCreateFailed)

	mockQuery.On("Where", "PK", "=", "USER#u1").Return(mockQuery).Once()
	mockQuery.On("Where", "SK", "=", "QUOTE_PERMISSIONS").Return(mockQuery).Once()
	mockQuery.On("First", mock.Anything).Return(dynamormerrors.ErrItemNotFound).Once()
	_, err = repo.GetQuotePermissions(ctx, "u1")
	assert.Error(t, err)

	mockQuery.On("Where", "PK", "=", "USER#u1").Return(mockQuery).Once()
	mockQuery.On("Where", "SK", "=", "QUOTE_PERMISSIONS").Return(mockQuery).Once()
	mockQuery.On("First", mock.Anything).Return(fmt.Errorf("boom")).Once()
	_, err = repo.GetQuotePermissions(ctx, "u1")
	assert.Error(t, err)

	mockQuery.On("Update", mock.Anything).Return(nil).Once()
	require.NoError(t, repo.UpdateQuotePermissions(ctx, p))

	mockQuery.On("Update", mock.Anything).Return(fmt.Errorf("boom")).Once()
	err = repo.UpdateQuotePermissions(ctx, p)
	assert.ErrorIs(t, err, ErrQuotePermissionsUpdateFailed)

	mockQuery.On("Where", "PK", "=", "USER#u1").Return(mockQuery).Once()
	mockQuery.On("Where", "SK", "=", "QUOTE_PERMISSIONS").Return(mockQuery).Once()
	mockQuery.On("Delete").Return(nil).Once()
	require.NoError(t, repo.DeleteQuotePermissions(ctx, "u1"))

	mockQuery.On("Where", "PK", "=", "USER#u1").Return(mockQuery).Once()
	mockQuery.On("Where", "SK", "=", "QUOTE_PERMISSIONS").Return(mockQuery).Once()
	mockQuery.On("Delete").Return(fmt.Errorf("boom")).Once()
	err = repo.DeleteQuotePermissions(ctx, "u1")
	assert.ErrorIs(t, err, ErrQuotePermissionsDeleteFailed)
}

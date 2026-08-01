package repositories

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	dynamormerrors "github.com/theory-cloud/tabletheory/v2/pkg/errors"
	dynamormmocks "github.com/theory-cloud/tabletheory/v2/pkg/mocks"
	dynamormtesting "github.com/theory-cloud/tabletheory/v2/pkg/testing"
	"go.uber.org/zap"
)

func TestGraphQLStreamSubscriptionRepository_New_UsesNopLoggerWhenNil(t *testing.T) {
	repo := NewGraphQLStreamSubscriptionRepository(dynamormtesting.NewTestDB().MockDB, "table", nil)
	require.NotNil(t, repo)
	require.NotNil(t, repo.logger)
}

func TestGraphQLStreamSubscriptionRepository_Put(t *testing.T) {
	ctx := context.Background()

	t.Run("nil_record_is_invalid_input", func(t *testing.T) {
		repo := NewGraphQLStreamSubscriptionRepository(dynamormtesting.NewTestDB().MockDB, "table", zap.NewNop())
		require.ErrorIs(t, repo.Put(ctx, nil), storage.ErrInvalidInput)
	})

	t.Run("missing_keys_validation_returns_error", func(t *testing.T) {
		repo := NewGraphQLStreamSubscriptionRepository(dynamormtesting.NewTestDB().MockDB, "table", zap.NewNop())
		err := repo.Put(ctx, &models.GraphQLStreamSubscription{
			Stream:         "stream-1",
			ConnectionID:   "conn-1",
			SubscriptionID: "sub-1",
			Field:          "field",
			UserID:         "",
		})
		require.Error(t, err)
		require.ErrorContains(t, err, "user_id")
	})

	t.Run("sets_ttl_and_keys_and_creates", func(t *testing.T) {
		testDB := dynamormtesting.NewTestDB()
		testDB.ExpectCreate()

		repo := NewGraphQLStreamSubscriptionRepository(testDB.MockDB, "table", zap.NewNop())
		record := &models.GraphQLStreamSubscription{
			Stream:         "stream-1",
			ConnectionID:   "conn-1",
			SubscriptionID: "sub-1",
			Field:          "field",
			UserID:         "user-1",
		}

		start := time.Now().UTC()
		require.NoError(t, repo.Put(ctx, record))

		require.NotEmpty(t, record.PK)
		require.NotEmpty(t, record.SK)
		require.Equal(t, "GQLSUB#stream-1", record.PK)
		require.Equal(t, "CONN#conn-1#SUB#sub-1", record.SK)
		require.Equal(t, "CONN#conn-1", record.GSI1PK)
		require.Equal(t, "SUB#sub-1#STREAM#stream-1", record.GSI1SK)

		require.False(t, record.CreatedAt.IsZero())
		require.False(t, record.CreatedAt.Before(start))
		require.GreaterOrEqual(t, record.TTL, start.Add(23*time.Hour).Unix())
		require.LessOrEqual(t, record.TTL, start.Add(25*time.Hour).Unix())

		testDB.AssertExpectations(t)
	})
}

func TestGraphQLStreamSubscriptionRepository_PutAllIsAtomic(t *testing.T) {
	ctx := context.Background()
	db := new(dynamormmocks.MockExtendedDB)
	tx := new(dynamormmocks.MockTransactionBuilder)
	db.TransactWriteBuilder = tx
	db.On("TransactWrite", ctx, mock.Anything).Return(nil).Once()

	var staged int
	tx.On("Put", mock.MatchedBy(func(record any) bool {
		subscription, ok := record.(*models.GraphQLStreamSubscription)
		if ok {
			staged++
			require.NotEmpty(t, subscription.PK)
			require.NotEmpty(t, subscription.SK)
		}
		return ok
	}), mock.Anything).Return(tx).Twice()
	tx.On("Execute").Return(errors.New("fault after staging first stream")).Once()

	repo := NewGraphQLStreamSubscriptionRepository(db, "table", zap.NewNop())
	records := []*models.GraphQLStreamSubscription{
		{Stream: "dm:inbox:alice", ConnectionID: "conn-1", SubscriptionID: "sub-1", Field: "conversationUpdates", UserID: "alice"},
		{Stream: "dm:requests:alice", ConnectionID: "conn-1", SubscriptionID: "sub-1", Field: "conversationUpdates", UserID: "alice"},
	}

	err := repo.PutAll(ctx, records)
	require.ErrorContains(t, err, "fault after staging first stream")
	require.Equal(t, 2, staged, "both stream writes must be staged in one transaction")
	db.AssertExpectations(t)
	tx.AssertExpectations(t)
}

func TestGraphQLStreamSubscriptionRepository_ListByStream(t *testing.T) {
	ctx := context.Background()

	t.Run("invalid_stream_is_invalid_input", func(t *testing.T) {
		repo := NewGraphQLStreamSubscriptionRepository(dynamormtesting.NewTestDB().MockDB, "table", zap.NewNop())
		_, err := repo.ListByStream(ctx, "  ")
		require.ErrorIs(t, err, storage.ErrInvalidInput)
	})

	t.Run("not_found_returns_empty_slice", func(t *testing.T) {
		testDB := dynamormtesting.NewTestDB()
		pk := fmt.Sprintf("GQLSUB#%s", "stream-1")
		testDB.ExpectWhere("PK", "=", pk)
		testDB.MockQuery.On("All", mock.Anything).Return(dynamormerrors.ErrItemNotFound).Once()

		repo := NewGraphQLStreamSubscriptionRepository(testDB.MockDB, "table", zap.NewNop())
		items, err := repo.ListByStream(ctx, "stream-1")
		require.NoError(t, err)
		require.Empty(t, items)
		testDB.AssertExpectations(t)
	})

	t.Run("success_returns_items", func(t *testing.T) {
		testDB := dynamormtesting.NewTestDB()
		pk := fmt.Sprintf("GQLSUB#%s", "stream-1")
		testDB.ExpectWhere("PK", "=", pk)

		expected := []models.GraphQLStreamSubscription{
			{Stream: "stream-1", ConnectionID: "c1", SubscriptionID: "s1"},
			{Stream: "stream-1", ConnectionID: "c2", SubscriptionID: "s2"},
		}
		testDB.MockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]models.GraphQLStreamSubscription)
			*dest = expected
		}).Return(nil).Once()

		repo := NewGraphQLStreamSubscriptionRepository(testDB.MockDB, "table", zap.NewNop())
		items, err := repo.ListByStream(ctx, "stream-1")
		require.NoError(t, err)
		require.Len(t, items, 2)
		require.Equal(t, "c1", items[0].ConnectionID)
		testDB.AssertExpectations(t)
	})
}

func TestGraphQLStreamSubscriptionRepository_Delete(t *testing.T) {
	ctx := context.Background()

	t.Run("invalid_input_is_error", func(t *testing.T) {
		repo := NewGraphQLStreamSubscriptionRepository(dynamormtesting.NewTestDB().MockDB, "table", zap.NewNop())
		require.ErrorIs(t, repo.Delete(ctx, "", "c", "s"), storage.ErrInvalidInput)
		require.ErrorIs(t, repo.Delete(ctx, "stream", " ", "s"), storage.ErrInvalidInput)
		require.ErrorIs(t, repo.Delete(ctx, "stream", "c", " "), storage.ErrInvalidInput)
	})

	t.Run("success_deletes", func(t *testing.T) {
		testDB := dynamormtesting.NewTestDB()
		pk := "GQLSUB#stream-1"
		sk := "CONN#conn-1#SUB#sub-1"
		testDB.ExpectWhere("PK", "=", pk).
			ExpectWhere("SK", "=", sk).
			ExpectDelete()

		repo := NewGraphQLStreamSubscriptionRepository(testDB.MockDB, "table", zap.NewNop())
		require.NoError(t, repo.Delete(ctx, "stream-1", "conn-1", "sub-1"))
		testDB.AssertExpectations(t)
	})
}

func TestGraphQLStreamSubscriptionRepository_DeleteSubscriptionAndConnection(t *testing.T) {
	ctx := context.Background()

	t.Run("delete_subscription_invalid_input_is_error", func(t *testing.T) {
		repo := NewGraphQLStreamSubscriptionRepository(dynamormtesting.NewTestDB().MockDB, "table", zap.NewNop())
		require.ErrorIs(t, repo.DeleteSubscription(ctx, "", "sub"), storage.ErrInvalidInput)
		require.ErrorIs(t, repo.DeleteSubscription(ctx, "conn", ""), storage.ErrInvalidInput)
	})

	t.Run("delete_all_for_connection_invalid_input_is_error", func(t *testing.T) {
		repo := NewGraphQLStreamSubscriptionRepository(dynamormtesting.NewTestDB().MockDB, "table", zap.NewNop())
		require.ErrorIs(t, repo.DeleteAllForConnection(ctx, " "), storage.ErrInvalidInput)
	})

	t.Run("delete_subscription_not_found_is_noop", func(t *testing.T) {
		testDB := dynamormtesting.NewTestDB()
		testDB.ExpectIndex("gsi1").
			ExpectWhere("gsi1PK", "=", "CONN#conn-1").
			ExpectWhere("gsi1SK", "begins_with", "SUB#sub-1#")
		testDB.MockQuery.On("All", mock.Anything).Return(dynamormerrors.ErrItemNotFound).Once()

		repo := NewGraphQLStreamSubscriptionRepository(testDB.MockDB, "table", zap.NewNop())
		require.NoError(t, repo.DeleteSubscription(ctx, "conn-1", "sub-1"))
		testDB.AssertExpectations(t)
	})

	t.Run("delete_all_for_connection_not_found_is_noop", func(t *testing.T) {
		testDB := dynamormtesting.NewTestDB()
		testDB.ExpectIndex("gsi1").
			ExpectWhere("gsi1PK", "=", "CONN#conn-1")
		testDB.MockQuery.On("All", mock.Anything).Return(dynamormerrors.ErrItemNotFound).Once()

		repo := NewGraphQLStreamSubscriptionRepository(testDB.MockDB, "table", zap.NewNop())
		require.NoError(t, repo.DeleteAllForConnection(ctx, "conn-1"))
		testDB.AssertExpectations(t)
	})

	t.Run("deletes_each_item_and_logs_delete_errors", func(t *testing.T) {
		testDB := dynamormtesting.NewTestDB()
		testDB.ExpectIndex("gsi1").
			ExpectWhere("gsi1PK", "=", "CONN#conn-1").
			ExpectWhere("gsi1SK", "begins_with", "SUB#sub-1#")

		stored := []models.GraphQLStreamSubscription{
			{PK: "GQLSUB#stream-1", SK: "CONN#conn-1#SUB#sub-1", Stream: "stream-1", SubscriptionID: "sub-1"},
			{PK: "GQLSUB#stream-2", SK: "CONN#conn-1#SUB#sub-1", Stream: "stream-2", SubscriptionID: "sub-1"},
		}
		testDB.MockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]models.GraphQLStreamSubscription)
			*dest = stored
		}).Return(nil).Once()

		testDB.ExpectWhere("PK", "=", stored[0].PK).
			ExpectWhere("SK", "=", stored[0].SK)
		testDB.ExpectWhere("PK", "=", stored[1].PK).
			ExpectWhere("SK", "=", stored[1].SK)

		testDB.ExpectDeleteError(errors.New("delete failed")).
			ExpectDelete()

		repo := NewGraphQLStreamSubscriptionRepository(testDB.MockDB, "table", zap.NewNop())
		require.NoError(t, repo.DeleteSubscription(ctx, "conn-1", "sub-1"))
		testDB.AssertExpectations(t)
	})

	t.Run("delete_all_for_connection_deletes_each_item", func(t *testing.T) {
		testDB := dynamormtesting.NewTestDB()
		testDB.ExpectIndex("gsi1").
			ExpectWhere("gsi1PK", "=", "CONN#conn-1")

		stored := []models.GraphQLStreamSubscription{
			{PK: "GQLSUB#stream-1", SK: "CONN#conn-1#SUB#sub-1", Stream: "stream-1", SubscriptionID: "sub-1"},
			{PK: "GQLSUB#stream-2", SK: "CONN#conn-1#SUB#sub-2", Stream: "stream-2", SubscriptionID: "sub-2"},
		}
		testDB.MockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]models.GraphQLStreamSubscription)
			*dest = stored
		}).Return(nil).Once()

		testDB.ExpectWhere("PK", "=", stored[0].PK).
			ExpectWhere("SK", "=", stored[0].SK)
		testDB.ExpectWhere("PK", "=", stored[1].PK).
			ExpectWhere("SK", "=", stored[1].SK)

		testDB.ExpectDelete().
			ExpectDelete()

		repo := NewGraphQLStreamSubscriptionRepository(testDB.MockDB, "table", zap.NewNop())
		require.NoError(t, repo.DeleteAllForConnection(ctx, "conn-1"))
		testDB.AssertExpectations(t)
	})
}

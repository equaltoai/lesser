package repositories

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	dynamormerrors "github.com/theory-cloud/tabletheory/v2/pkg/errors"
	"github.com/theory-cloud/tabletheory/v2/pkg/mocks"
	"go.uber.org/zap"
)

func TestListRepository_Round08_SweepHappyPaths(t *testing.T) {
	ctx := context.Background()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
	mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()

	mockQuery.On("Index", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Filter", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Limit", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Cursor", mock.Anything).Return(mockQuery).Maybe()

	mockQuery.On("Create").Return(nil).Maybe()
	mockQuery.On("Update", mock.Anything).Return(nil).Maybe()
	mockQuery.On("Delete").Return(nil).Maybe()
	mockQuery.On("Count").Return(int64(1), nil).Maybe()

	mockQuery.On("First", mock.Anything).Return(nil).Maybe().Run(func(args mock.Arguments) {
		switch dest := args.Get(0).(type) {
		case *models.List:
			*dest = models.List{
				ID:            "list1",
				Username:      "alice",
				Title:         "t",
				RepliesPolicy: RepliesPolicyList,
				CreatedAt:     time.Now().Add(-time.Hour),
				UpdatedAt:     time.Now().Add(-time.Minute),
			}
			_ = dest.UpdateKeys()
		case *models.User:
			*dest = models.User{Username: "bob", Email: "b@example.com", DisplayName: "Bob"}
			_ = dest.UpdateKeys()
		case *models.Actor:
			*dest = models.Actor{
				Username: "bob",
				Actor: &activitypub.Actor{
					BaseObject: activitypub.BaseObject{ID: "https://example.com/users/bob"},
				},
			}
			_ = dest.UpdateKeys()
		default:
		}
	})

	mockQuery.On("Scan", mock.Anything).Return(nil).Maybe().Run(func(args mock.Arguments) {
		switch dest := args.Get(0).(type) {
		case *[]models.List:
			*dest = []models.List{
				{ID: "list1", Username: "alice", Title: "t", RepliesPolicy: RepliesPolicyList},
			}
		case *[]models.ListMember:
			*dest = []models.ListMember{
				{ListID: "list1", AccountID: "bob", ListUsername: "alice"},
			}
		default:
		}
	})

	mockQuery.On("All", mock.Anything).Return(nil).Maybe().Run(func(args mock.Arguments) {
		switch dest := args.Get(0).(type) {
		case *[]models.List:
			*dest = []models.List{
				{ID: "list1", Username: "alice", Title: "t"},
			}
		case *[]models.ListMember:
			*dest = []models.ListMember{
				{ListID: "list1", AccountID: "bob", ListUsername: "alice"},
			}
		case *[]*models.ListMember:
			*dest = []*models.ListMember{
				{ListID: "list1", AccountID: "bob", ListUsername: "alice"},
			}
		case *[]models.Status:
			*dest = []models.Status{
				{StatusID: "s1"},
			}
		default:
		}
	})

	repo := NewListRepository(mockDB, "table", zap.NewNop(), nil)
	repo.SetValidationService(nil)
	repo.SetPermissionService(nil)
	repo.SetCachingService(nil)
	repo.SetEventService(nil)
	repo.memberRepo.SetValidationService(nil)
	repo.memberRepo.SetPermissionService(nil)
	repo.memberRepo.SetCachingService(nil)
	repo.memberRepo.SetEventService(nil)

	require.Error(t, repo.CreateList(ctx, &models.List{Username: "alice", Title: "t", RepliesPolicy: "bad"}))

	list := &models.List{Username: "alice", Title: "t"} // Defaults + generated ID
	require.NoError(t, repo.CreateList(ctx, list))
	require.NotEmpty(t, list.ID)
	require.NotEmpty(t, list.RepliesPolicy)

	_, err := repo.GetList(ctx, "list1")
	require.NoError(t, err)

	require.NoError(t, repo.UpdateList(ctx, &models.List{RepliesPolicy: RepliesPolicyList}))
	require.Error(t, repo.UpdateList(ctx, &models.List{RepliesPolicy: "bad"}))

	require.NoError(t, repo.DeleteList(ctx, "list1"))

	_, err = repo.GetListsForUser(ctx, "alice")
	require.NoError(t, err)

	_, _, err = repo.GetListsForUserPaginated(ctx, "alice", 1, "CURSOR")
	require.NoError(t, err)

	_, err = repo.GetUserLists(ctx, "alice", interfaces.PaginationOptions{Limit: 1, Cursor: "CUR"})
	require.NoError(t, err)

	_, err = repo.GetListsByMember(ctx, "bob", interfaces.PaginationOptions{Limit: 1, Cursor: "CUR"})
	require.NoError(t, err)

	_, err = repo.CountUserLists(ctx, "alice")
	require.NoError(t, err)

	require.NoError(t, repo.AddListMember(ctx, "list1", "bob"))
	require.NoError(t, repo.RemoveListMember(ctx, "list1", "bob"))

	_, err = repo.GetListMembers(ctx, "list1", interfaces.PaginationOptions{Limit: 1, Cursor: "bob"})
	require.NoError(t, err)

	_, err = repo.IsListMember(ctx, "list1", "bob")
	require.NoError(t, err)

	_, _, err = repo.GetAccountListsPaginated(ctx, "bob", 1, "CURSOR")
	require.NoError(t, err)

	_, err = repo.GetAccountLists(ctx, "bob")
	require.NoError(t, err)

	_, err = repo.GetAccountListsForUser(ctx, "bob", "alice")
	require.NoError(t, err)

	_, err = repo.CountListMembers(ctx, "list1")
	require.NoError(t, err)

	require.NoError(t, repo.RemoveAccountFromAllLists(ctx, "bob"))

	_, err = repo.GetExclusiveLists(ctx, "alice")
	require.NoError(t, err)

	require.NoError(t, repo.AddAccountsToList(ctx, "list1", []string{"bob"}))
	require.NoError(t, repo.RemoveAccountsFromList(ctx, "list1", []string{"bob"}))

	_, err = repo.GetListAccounts(ctx, "list1")
	require.NoError(t, err)

	_, err = repo.GetListsContainingAccount(ctx, "bob", "alice")
	require.NoError(t, err)

	_, err = repo.GetListTimeline(ctx, "list1", interfaces.PaginationOptions{Limit: 1})
	require.NoError(t, err)

	_, err = repo.GetListStatuses(ctx, "list1", interfaces.PaginationOptions{Limit: 1})
	require.NoError(t, err)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestListRepository_Round08_ErrorBranches(t *testing.T) {
	ctx := context.Background()

	t.Run("GetList not found maps to not found", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Twice()
		mockQuery.On("First", mock.Anything).Return(dynamormerrors.ErrItemNotFound).Once()

		repo := NewListRepository(mockDB, "table", zap.NewNop(), nil)
		_, err := repo.GetList(ctx, "missing")
		require.Error(t, err)

		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
	})

	t.Run("GetUserLists handles empty-index validation quirks", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Index", "gsi1").Return(mockQuery).Once()
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Once()
		mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Limit", 1).Return(mockQuery).Once()
		mockQuery.On("All", mock.Anything).Return(assert.AnError).Once()

		repo := NewListRepository(mockDB, "table", zap.NewNop(), nil)
		_, err := repo.GetUserLists(ctx, "alice", interfaces.PaginationOptions{Limit: 1})
		require.Error(t, err)

		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
	})

	t.Run("DeleteList ignores not-found on membership scan", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQueryGetList := new(mocks.MockQuery)
		mockQueryDeleteList := new(mocks.MockQuery)
		mockQueryScanMembers := new(mocks.MockQuery)

		// GetList.
		mockDB.On("WithContext", mock.Anything).Return(mockDB).Times(3)
		mockDB.On("Model", mock.Anything).Return(mockQueryGetList).Once()
		mockQueryGetList.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQueryGetList).Twice()
		mockQueryGetList.On("First", mock.Anything).Return(nil).Once()

		// Delete list metadata.
		mockDB.On("Model", mock.Anything).Return(mockQueryDeleteList).Once()
		mockQueryDeleteList.On("Delete").Return(nil).Once()

		// Scan members -> not found.
		mockDB.On("Model", mock.Anything).Return(mockQueryScanMembers).Once()
		mockQueryScanMembers.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQueryScanMembers).Once()
		mockQueryScanMembers.On("Scan", mock.Anything).Return(dynamormerrors.ErrItemNotFound).Once()

		repo := NewListRepository(mockDB, "table", zap.NewNop(), nil)
		err := repo.DeleteList(ctx, "list1")
		require.NoError(t, err)

		mockDB.AssertExpectations(t)
		mockQueryGetList.AssertExpectations(t)
		mockQueryDeleteList.AssertExpectations(t)
		mockQueryScanMembers.AssertExpectations(t)
	})

	t.Run("AddListMember returns early when already a member", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQueryGetList := new(mocks.MockQuery)
		mockQueryExists := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Twice()

		// GetList.
		mockDB.On("Model", mock.Anything).Return(mockQueryGetList).Once()
		mockQueryGetList.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQueryGetList).Twice()
		mockQueryGetList.On("First", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*models.List)
			*dest = models.List{ID: "list1", Username: "alice", RepliesPolicy: RepliesPolicyList}
			_ = dest.UpdateKeys()
		}).Once()

		// Exists -> Count returns 1.
		mockDB.On("Model", mock.Anything).Return(mockQueryExists).Once()
		mockQueryExists.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQueryExists).Twice()
		mockQueryExists.On("Count").Return(int64(1), nil).Once()

		repo := NewListRepository(mockDB, "table", zap.NewNop(), nil)
		repo.SetValidationService(nil)
		repo.SetPermissionService(nil)
		repo.SetCachingService(nil)
		repo.SetEventService(nil)
		repo.memberRepo.SetValidationService(nil)
		repo.memberRepo.SetPermissionService(nil)
		repo.memberRepo.SetCachingService(nil)
		repo.memberRepo.SetEventService(nil)

		require.NoError(t, repo.AddListMember(ctx, "list1", "bob"))

		mockDB.AssertExpectations(t)
		mockQueryGetList.AssertExpectations(t)
		mockQueryExists.AssertExpectations(t)
	})

	t.Run("GetListTimeline returns empty when there are no members", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQueryMembers := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
		mockDB.On("Model", mock.Anything).Return(mockQueryMembers).Once()
		mockQueryMembers.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQueryMembers).Once()
		mockQueryMembers.On("Limit", 100).Return(mockQueryMembers).Once()
		mockQueryMembers.On("Scan", mock.Anything).Return(dynamormerrors.ErrItemNotFound).Once()

		repo := NewListRepository(mockDB, "table", zap.NewNop(), nil)
		out, err := repo.GetListTimeline(ctx, "list1", interfaces.PaginationOptions{Limit: 10})
		require.NoError(t, err)
		require.NotNil(t, out)
		require.Empty(t, out.Items)

		mockDB.AssertExpectations(t)
		mockQueryMembers.AssertExpectations(t)
	})
}

func TestListRepository_Round08_MoreBranches(t *testing.T) {
	ctx := context.Background()

	t.Run("GetListsForUserPaginated handles not found and hasMore cursor", func(t *testing.T) {
		t.Run("not found returns empty", func(t *testing.T) {
			mockDB := new(mocks.MockDB)
			mockQuery := new(mocks.MockQuery)

			mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
			mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
			mockQuery.On("Index", "gsi1").Return(mockQuery).Once()
			mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Once()
			mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery).Once()
			mockQuery.On("Limit", 2).Return(mockQuery).Once()
			mockQuery.On("Scan", mock.Anything).Return(dynamormerrors.ErrItemNotFound).Once()

			repo := NewListRepository(mockDB, "table", zap.NewNop(), nil)
			lists, next, err := repo.GetListsForUserPaginated(ctx, "alice", 1, "")
			require.NoError(t, err)
			require.Empty(t, lists)
			assert.Empty(t, next)

			mockDB.AssertExpectations(t)
			mockQuery.AssertExpectations(t)
		})

		t.Run("hasMore returns next cursor", func(t *testing.T) {
			mockDB := new(mocks.MockDB)
			mockQuery := new(mocks.MockQuery)

			mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
			mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
			mockQuery.On("Index", "gsi1").Return(mockQuery).Once()
			mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Once()
			mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery).Once()
			mockQuery.On("Limit", 2).Return(mockQuery).Once()
			mockQuery.On("Scan", mock.Anything).Return(nil).Once().Run(func(args mock.Arguments) {
				dest := args.Get(0).(*[]models.List)
				*dest = []models.List{
					{ID: "l1", Username: "alice", GSI1SK: "SK#1"},
					{ID: "l2", Username: "alice", GSI1SK: "SK#2"},
				}
			})

			repo := NewListRepository(mockDB, "table", zap.NewNop(), nil)
			lists, next, err := repo.GetListsForUserPaginated(ctx, "alice", 1, "")
			require.NoError(t, err)
			require.Len(t, lists, 1)
			assert.Equal(t, "SK#1", next)

			mockDB.AssertExpectations(t)
			mockQuery.AssertExpectations(t)
		})
	})

	t.Run("GetUserLists returns empty on not found and validation exceptions", func(t *testing.T) {
		t.Run("not found", func(t *testing.T) {
			mockDB := new(mocks.MockDB)
			mockQuery := new(mocks.MockQuery)

			mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
			mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
			mockQuery.On("Index", "gsi1").Return(mockQuery).Once()
			mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Once()
			mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery).Once()
			mockQuery.On("Limit", 1).Return(mockQuery).Once()
			mockQuery.On("All", mock.Anything).Return(dynamormerrors.ErrItemNotFound).Once()

			repo := NewListRepository(mockDB, "table", zap.NewNop(), nil)
			out, err := repo.GetUserLists(ctx, "alice", interfaces.PaginationOptions{Limit: 1})
			require.NoError(t, err)
			require.Empty(t, out.Items)

			mockDB.AssertExpectations(t)
			mockQuery.AssertExpectations(t)
		})

		t.Run("validation exception treated as empty index", func(t *testing.T) {
			mockDB := new(mocks.MockDB)
			mockQuery := new(mocks.MockQuery)

			mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
			mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
			mockQuery.On("Index", "gsi1").Return(mockQuery).Once()
			mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Once()
			mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery).Once()
			mockQuery.On("Limit", 1).Return(mockQuery).Once()
			mockQuery.On("All", mock.Anything).Return(fmt.Errorf("ValidationException: Query condition missed key schema element")).Once()

			repo := NewListRepository(mockDB, "table", zap.NewNop(), nil)
			out, err := repo.GetUserLists(ctx, "alice", interfaces.PaginationOptions{Limit: 1})
			require.NoError(t, err)
			require.Empty(t, out.Items)

			mockDB.AssertExpectations(t)
			mockQuery.AssertExpectations(t)
		})
	})

	t.Run("CountUserLists maps errors", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Index", "gsi1").Return(mockQuery).Once()
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Count").Return(int64(0), assert.AnError).Once()

		repo := NewListRepository(mockDB, "table", zap.NewNop(), nil)
		_, err := repo.CountUserLists(ctx, "alice")
		require.Error(t, err)

		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
	})

	t.Run("AddListMember handles get/exists/create errors", func(t *testing.T) {
		t.Run("GetList error bubbles", func(t *testing.T) {
			mockDB := new(mocks.MockDB)
			mockQuery := new(mocks.MockQuery)

			mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
			mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
			mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Twice()
			mockQuery.On("First", mock.Anything).Return(dynamormerrors.ErrItemNotFound).Once()

			repo := NewListRepository(mockDB, "table", zap.NewNop(), nil)
			require.Error(t, repo.AddListMember(ctx, "list1", "bob"))

			mockDB.AssertExpectations(t)
			mockQuery.AssertExpectations(t)
		})

		t.Run("Exists error bubbles", func(t *testing.T) {
			mockDB := new(mocks.MockDB)
			mockQueryGet := new(mocks.MockQuery)
			mockQueryCount := new(mocks.MockQuery)

			mockDB.On("WithContext", mock.Anything).Return(mockDB).Twice()

			mockDB.On("Model", mock.Anything).Return(mockQueryGet).Once()
			mockQueryGet.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQueryGet).Twice()
			mockQueryGet.On("First", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
				dest := args.Get(0).(*models.List)
				*dest = models.List{ID: "list1", Username: "alice", RepliesPolicy: RepliesPolicyList}
				_ = dest.UpdateKeys()
			}).Once()

			mockDB.On("Model", mock.Anything).Return(mockQueryCount).Once()
			mockQueryCount.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQueryCount).Twice()
			mockQueryCount.On("Count").Return(int64(0), assert.AnError).Once()

			repo := NewListRepository(mockDB, "table", zap.NewNop(), nil)
			require.Error(t, repo.AddListMember(ctx, "list1", "bob"))

			mockDB.AssertExpectations(t)
			mockQueryGet.AssertExpectations(t)
			mockQueryCount.AssertExpectations(t)
		})

		t.Run("Create error bubbles", func(t *testing.T) {
			mockDB := new(mocks.MockDB)
			mockQueryGet := new(mocks.MockQuery)
			mockQueryCount := new(mocks.MockQuery)
			mockQueryCreate := new(mocks.MockQuery)

			mockDB.On("WithContext", mock.Anything).Return(mockDB).Times(3)

			mockDB.On("Model", mock.Anything).Return(mockQueryGet).Once()
			mockQueryGet.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQueryGet).Twice()
			mockQueryGet.On("First", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
				dest := args.Get(0).(*models.List)
				*dest = models.List{ID: "list1", Username: "alice", RepliesPolicy: RepliesPolicyList}
				_ = dest.UpdateKeys()
			}).Once()

			mockDB.On("Model", mock.Anything).Return(mockQueryCount).Once()
			mockQueryCount.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQueryCount).Twice()
			mockQueryCount.On("Count").Return(int64(0), nil).Once()

			mockDB.On("Model", mock.Anything).Return(mockQueryCreate).Once()
			mockQueryCreate.On("Create").Return(assert.AnError).Once()

			repo := NewListRepository(mockDB, "table", zap.NewNop(), nil)
			repo.SetValidationService(nil)
			repo.SetPermissionService(nil)
			repo.SetCachingService(nil)
			repo.SetEventService(nil)
			repo.memberRepo.SetValidationService(nil)
			repo.memberRepo.SetPermissionService(nil)
			repo.memberRepo.SetCachingService(nil)
			repo.memberRepo.SetEventService(nil)

			require.Error(t, repo.AddListMember(ctx, "list1", "bob"))

			mockDB.AssertExpectations(t)
			mockQueryGet.AssertExpectations(t)
			mockQueryCount.AssertExpectations(t)
			mockQueryCreate.AssertExpectations(t)
		})
	})

	t.Run("GetAccountListsPaginated hasMore and not found branches", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()
		mockQuery.On("Index", "gsi1").Return(mockQuery).Once()
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Once()
		mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Limit", 2).Return(mockQuery).Once()
		mockQuery.On("Scan", mock.Anything).Return(nil).Once().Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]models.ListMember)
			*dest = []models.ListMember{
				{ListID: "l1", AccountID: "bob", ListUsername: "alice", GSI1SK: "SK#1"},
				{ListID: "l2", AccountID: "bob", ListUsername: "alice", GSI1SK: "SK#2"},
			}
		})

		// GetList calls inside loop.
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
		mockQuery.On("First", mock.Anything).Return(nil).Once().Run(func(args mock.Arguments) {
			dest := args.Get(0).(*models.List)
			*dest = models.List{ID: "l1", Username: "alice", RepliesPolicy: RepliesPolicyList}
			_ = dest.UpdateKeys()
		})

		repo := NewListRepository(mockDB, "table", zap.NewNop(), nil)
		lists, next, err := repo.GetAccountListsPaginated(ctx, "bob", 1, "")
		require.NoError(t, err)
		require.Len(t, lists, 1)
		assert.Equal(t, "SK#1", next)

		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
	})

	t.Run("GetAccountListsForUser filters by username", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQueryMembers := new(mocks.MockQuery)
		mockQueryGetList := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()

		mockDB.On("Model", mock.Anything).Return(mockQueryMembers).Once()
		mockQueryMembers.On("Index", "gsi1").Return(mockQueryMembers).Once()
		mockQueryMembers.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQueryMembers).Once()
		mockQueryMembers.On("Scan", mock.Anything).Return(nil).Once().Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]models.ListMember)
			*dest = []models.ListMember{
				{ListID: "l1", AccountID: "bob", ListUsername: "someone-else"},
			}
		})

		mockDB.On("Model", mock.Anything).Return(mockQueryGetList).Maybe()
		mockQueryGetList.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQueryGetList).Maybe()
		mockQueryGetList.On("First", mock.Anything).Return(nil).Maybe()

		repo := NewListRepository(mockDB, "table", zap.NewNop(), nil)
		out, err := repo.GetAccountListsForUser(ctx, "bob", "alice")
		require.NoError(t, err)
		require.Empty(t, out)

		mockDB.AssertExpectations(t)
		mockQueryMembers.AssertExpectations(t)
	})

	t.Run("RemoveAccountFromAllLists continues on delete failures", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQueryScan := new(mocks.MockQuery)
		mockQueryDelete := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Twice()

		mockDB.On("Model", mock.Anything).Return(mockQueryScan).Once()
		mockQueryScan.On("Index", "gsi1").Return(mockQueryScan).Once()
		mockQueryScan.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQueryScan).Once()
		mockQueryScan.On("Scan", mock.Anything).Return(nil).Once().Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]models.ListMember)
			*dest = []models.ListMember{
				{ListID: "l1", AccountID: "bob"},
			}
		})

		mockDB.On("Model", mock.Anything).Return(mockQueryDelete).Once()
		mockQueryDelete.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQueryDelete).Twice()
		mockQueryDelete.On("Delete").Return(assert.AnError).Once()

		repo := NewListRepository(mockDB, "table", zap.NewNop(), nil)
		repo.memberRepo.SetValidationService(nil)
		repo.memberRepo.SetPermissionService(nil)
		repo.memberRepo.SetCachingService(nil)
		repo.memberRepo.SetEventService(nil)

		require.NoError(t, repo.RemoveAccountFromAllLists(ctx, "bob"))

		mockDB.AssertExpectations(t)
		mockQueryScan.AssertExpectations(t)
		mockQueryDelete.AssertExpectations(t)
	})

	t.Run("AddAccountsToList continues on IsListMember and create errors", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQueryGetList := new(mocks.MockQuery)
		mockQueryCount := new(mocks.MockQuery)
		mockQueryCreate := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()

		// GetList.
		mockDB.On("Model", mock.Anything).Return(mockQueryGetList).Once()
		mockQueryGetList.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQueryGetList).Twice()
		mockQueryGetList.On("First", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*models.List)
			*dest = models.List{ID: "list1", Username: "alice", RepliesPolicy: RepliesPolicyList}
			_ = dest.UpdateKeys()
		}).Once()

		// IsListMember calls -> Count: error, then exists, then not exists.
		mockDB.On("Model", mock.Anything).Return(mockQueryCount).Times(3)
		mockQueryCount.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQueryCount).Times(6)
		mockQueryCount.On("Count").Return(int64(0), assert.AnError).Once()
		mockQueryCount.On("Count").Return(int64(1), nil).Once()
		mockQueryCount.On("Count").Return(int64(0), nil).Once()

		// Create membership for the last account -> error (logged/ignored).
		mockDB.On("Model", mock.Anything).Return(mockQueryCreate).Once()
		mockQueryCreate.On("Create").Return(assert.AnError).Once()

		repo := NewListRepository(mockDB, "table", zap.NewNop(), nil)
		repo.memberRepo.SetValidationService(nil)
		repo.memberRepo.SetPermissionService(nil)
		repo.memberRepo.SetCachingService(nil)
		repo.memberRepo.SetEventService(nil)

		require.NoError(t, repo.AddAccountsToList(ctx, "list1", []string{"a1", "a2", "a3"}))

		mockDB.AssertExpectations(t)
		mockQueryGetList.AssertExpectations(t)
		mockQueryCount.AssertExpectations(t)
		mockQueryCreate.AssertExpectations(t)
	})

	t.Run("RemoveAccountsFromList continues on member delete errors", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQueryGetList := new(mocks.MockQuery)
		mockQueryDelete := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Twice()

		// GetList.
		mockDB.On("Model", mock.Anything).Return(mockQueryGetList).Once()
		mockQueryGetList.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQueryGetList).Twice()
		mockQueryGetList.On("First", mock.Anything).Return(nil).Once()

		// Delete fails (logged/ignored).
		mockDB.On("Model", mock.Anything).Return(mockQueryDelete).Once()
		mockQueryDelete.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQueryDelete).Twice()
		mockQueryDelete.On("Delete").Return(assert.AnError).Once()

		repo := NewListRepository(mockDB, "table", zap.NewNop(), nil)
		repo.memberRepo.SetValidationService(nil)
		repo.memberRepo.SetPermissionService(nil)
		repo.memberRepo.SetCachingService(nil)
		repo.memberRepo.SetEventService(nil)

		require.NoError(t, repo.RemoveAccountsFromList(ctx, "list1", []string{"bob"}))

		mockDB.AssertExpectations(t)
		mockQueryGetList.AssertExpectations(t)
		mockQueryDelete.AssertExpectations(t)
	})
}

func TestListRepository_Round08_FinalBoost(t *testing.T) {
	ctx := context.Background()

	t.Run("DeleteList maps metadata delete failures", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQueryGet := new(mocks.MockQuery)
		mockQueryDelete := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Twice()

		mockDB.On("Model", mock.Anything).Return(mockQueryGet).Once()
		mockQueryGet.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQueryGet).Twice()
		mockQueryGet.On("First", mock.Anything).Return(nil).Once()

		mockDB.On("Model", mock.Anything).Return(mockQueryDelete).Once()
		mockQueryDelete.On("Delete").Return(assert.AnError).Once()

		repo := NewListRepository(mockDB, "table", zap.NewNop(), nil)
		require.Error(t, repo.DeleteList(ctx, "list1"))

		mockDB.AssertExpectations(t)
		mockQueryGet.AssertExpectations(t)
		mockQueryDelete.AssertExpectations(t)
	})

	t.Run("DeleteList tolerates list-member deletion failures", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQueryGet := new(mocks.MockQuery)
		mockQueryDeleteList := new(mocks.MockQuery)
		mockQueryScan := new(mocks.MockQuery)
		mockQueryDeleteMember1 := new(mocks.MockQuery)
		mockQueryDeleteMember2 := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Times(5)

		// GetList.
		mockDB.On("Model", mock.Anything).Return(mockQueryGet).Once()
		mockQueryGet.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQueryGet).Twice()
		mockQueryGet.On("First", mock.Anything).Return(nil).Once()

		// Delete list metadata.
		mockDB.On("Model", mock.Anything).Return(mockQueryDeleteList).Once()
		mockQueryDeleteList.On("Delete").Return(nil).Once()

		// Scan members.
		mockDB.On("Model", mock.Anything).Return(mockQueryScan).Once()
		mockQueryScan.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQueryScan).Once()
		mockQueryScan.On("Scan", mock.Anything).Return(nil).Once().Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]models.ListMember)
			*dest = []models.ListMember{
				{ListID: "list1", AccountID: "bob"},
				{ListID: "list1", AccountID: "carl"},
			}
		})

		// Delete member 1 fails.
		mockDB.On("Model", mock.Anything).Return(mockQueryDeleteMember1).Once()
		mockQueryDeleteMember1.On("Delete").Return(assert.AnError).Once()

		// Delete member 2 ok.
		mockDB.On("Model", mock.Anything).Return(mockQueryDeleteMember2).Once()
		mockQueryDeleteMember2.On("Delete").Return(nil).Once()

		repo := NewListRepository(mockDB, "table", zap.NewNop(), nil)
		require.NoError(t, repo.DeleteList(ctx, "list1"))

		mockDB.AssertExpectations(t)
		mockQueryGet.AssertExpectations(t)
		mockQueryDeleteList.AssertExpectations(t)
		mockQueryScan.AssertExpectations(t)
		mockQueryDeleteMember1.AssertExpectations(t)
		mockQueryDeleteMember2.AssertExpectations(t)
	})

	t.Run("GetAccountListsPaginated returns empty on not found and maps scan errors", func(t *testing.T) {
		t.Run("not found", func(t *testing.T) {
			mockDB := new(mocks.MockDB)
			mockQuery := new(mocks.MockQuery)

			mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
			mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
			mockQuery.On("Index", "gsi1").Return(mockQuery).Once()
			mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Once()
			mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery).Once()
			mockQuery.On("Limit", 2).Return(mockQuery).Once()
			mockQuery.On("Scan", mock.Anything).Return(dynamormerrors.ErrItemNotFound).Once()

			repo := NewListRepository(mockDB, "table", zap.NewNop(), nil)
			items, next, err := repo.GetAccountListsPaginated(ctx, "bob", 1, "")
			require.NoError(t, err)
			require.Empty(t, items)
			assert.Empty(t, next)

			mockDB.AssertExpectations(t)
			mockQuery.AssertExpectations(t)
		})

		t.Run("scan error", func(t *testing.T) {
			mockDB := new(mocks.MockDB)
			mockQuery := new(mocks.MockQuery)

			mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
			mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
			mockQuery.On("Index", "gsi1").Return(mockQuery).Once()
			mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Once()
			mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery).Once()
			mockQuery.On("Limit", 2).Return(mockQuery).Once()
			mockQuery.On("Scan", mock.Anything).Return(assert.AnError).Once()

			repo := NewListRepository(mockDB, "table", zap.NewNop(), nil)
			_, _, err := repo.GetAccountListsPaginated(ctx, "bob", 1, "")
			require.Error(t, err)

			mockDB.AssertExpectations(t)
			mockQuery.AssertExpectations(t)
		})
	})

	t.Run("GetListMembers continues when user fetch fails", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQueryMembers := new(mocks.MockQuery)
		mockQueryUser := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Twice()

		mockDB.On("Model", mock.Anything).Return(mockQueryMembers).Once()
		mockQueryMembers.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQueryMembers).Once()
		mockQueryMembers.On("Limit", 1).Return(mockQueryMembers).Once()
		mockQueryMembers.On("Scan", mock.Anything).Return(nil).Once().Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]models.ListMember)
			*dest = []models.ListMember{{ListID: "list1", AccountID: "bob"}}
		})

		mockDB.On("Model", mock.Anything).Return(mockQueryUser).Once()
		mockQueryUser.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQueryUser).Twice()
		mockQueryUser.On("First", mock.Anything).Return(assert.AnError).Once()

		repo := NewListRepository(mockDB, "table", zap.NewNop(), nil)
		out, err := repo.GetListMembers(ctx, "list1", interfaces.PaginationOptions{Limit: 1})
		require.NoError(t, err)
		require.Empty(t, out.Items)
		assert.NotEmpty(t, out.NextCursor)

		mockDB.AssertExpectations(t)
		mockQueryMembers.AssertExpectations(t)
		mockQueryUser.AssertExpectations(t)
	})

	t.Run("GetListTimeline continues when a user timeline query fails", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQueryMembers := new(mocks.MockQuery)
		mockQueryUser := new(mocks.MockQuery)
		mockQueryActor := new(mocks.MockQuery)
		mockQueryStatuses := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Times(4)

		// GetListMembers member scan.
		mockDB.On("Model", mock.Anything).Return(mockQueryMembers).Once()
		mockQueryMembers.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQueryMembers).Once()
		mockQueryMembers.On("Limit", 100).Return(mockQueryMembers).Once()
		mockQueryMembers.On("Scan", mock.Anything).Return(nil).Once().Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]models.ListMember)
			*dest = []models.ListMember{{ListID: "list1", AccountID: "bob"}}
		})

		// User lookup succeeds.
		mockDB.On("Model", mock.Anything).Return(mockQueryUser).Once()
		mockQueryUser.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQueryUser).Twice()
		mockQueryUser.On("First", mock.Anything).Return(nil).Once().Run(func(args mock.Arguments) {
			dest := args.Get(0).(*models.User)
			*dest = models.User{Username: "bob"}
			_ = dest.UpdateKeys()
		})

		// Actor lookup not found (ignored).
		mockDB.On("Model", mock.Anything).Return(mockQueryActor).Once()
		mockQueryActor.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQueryActor).Twice()
		mockQueryActor.On("First", mock.Anything).Return(dynamormerrors.ErrItemNotFound).Once()

		// Status query fails (logged/ignored).
		mockDB.On("Model", mock.Anything).Return(mockQueryStatuses).Once()
		mockQueryStatuses.On("Index", "gsi1").Return(mockQueryStatuses).Once()
		mockQueryStatuses.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQueryStatuses).Once()
		mockQueryStatuses.On("Limit", 1).Return(mockQueryStatuses).Once()
		mockQueryStatuses.On("All", mock.Anything).Return(assert.AnError).Once()

		repo := NewListRepository(mockDB, "table", zap.NewNop(), nil)
		out, err := repo.GetListTimeline(ctx, "list1", interfaces.PaginationOptions{Limit: 1})
		require.NoError(t, err)
		require.Empty(t, out.Items)

		mockDB.AssertExpectations(t)
		mockQueryMembers.AssertExpectations(t)
		mockQueryUser.AssertExpectations(t)
		mockQueryActor.AssertExpectations(t)
		mockQueryStatuses.AssertExpectations(t)
	})

	t.Run("GetExclusiveLists maps underlying pagination errors", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Index", "gsi1").Return(mockQuery).Once()
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Once()
		mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Limit", 101).Return(mockQuery).Once()
		mockQuery.On("Scan", mock.Anything).Return(assert.AnError).Once()

		repo := NewListRepository(mockDB, "table", zap.NewNop(), nil)
		_, err := repo.GetExclusiveLists(ctx, "alice")
		require.Error(t, err)

		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
	})
}

func TestListRepository_Round08_LastMiles(t *testing.T) {
	ctx := context.Background()

	t.Run("GetAccountListsForUser returns empty on not found", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Index", "gsi1").Return(mockQuery).Once()
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Scan", mock.Anything).Return(dynamormerrors.ErrItemNotFound).Once()

		repo := NewListRepository(mockDB, "table", zap.NewNop(), nil)
		out, err := repo.GetAccountListsForUser(ctx, "bob", "alice")
		require.NoError(t, err)
		require.Empty(t, out)

		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
	})

	t.Run("RemoveAccountFromAllLists maps query errors", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Index", "gsi1").Return(mockQuery).Once()
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Scan", mock.Anything).Return(assert.AnError).Once()

		repo := NewListRepository(mockDB, "table", zap.NewNop(), nil)
		require.Error(t, repo.RemoveAccountFromAllLists(ctx, "bob"))

		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
	})

	t.Run("GetListsByMember skips list get failures", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQueryMembers := new(mocks.MockQuery)
		mockQueryGetList := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Twice()

		mockDB.On("Model", mock.Anything).Return(mockQueryMembers).Once()
		mockQueryMembers.On("Index", "gsi2").Return(mockQueryMembers).Once()
		mockQueryMembers.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQueryMembers).Once()
		mockQueryMembers.On("Limit", 1).Return(mockQueryMembers).Once()
		mockQueryMembers.On("All", mock.Anything).Return(nil).Once().Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]models.ListMember)
			*dest = []models.ListMember{{ListID: "l1", AccountID: "bob"}}
		})

		// GetList fails and is skipped.
		mockDB.On("Model", mock.Anything).Return(mockQueryGetList).Once()
		mockQueryGetList.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQueryGetList).Twice()
		mockQueryGetList.On("First", mock.Anything).Return(dynamormerrors.ErrItemNotFound).Once()

		repo := NewListRepository(mockDB, "table", zap.NewNop(), nil)
		out, err := repo.GetListsByMember(ctx, "bob", interfaces.PaginationOptions{Limit: 1})
		require.NoError(t, err)
		require.Empty(t, out.Items)
		require.NotEmpty(t, out.NextCursor)

		mockDB.AssertExpectations(t)
		mockQueryMembers.AssertExpectations(t)
		mockQueryGetList.AssertExpectations(t)
	})
}

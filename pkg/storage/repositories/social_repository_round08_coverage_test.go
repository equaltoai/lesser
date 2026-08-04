package repositories

import (
	"context"
	"fmt"
	"testing"
	"time"

	svcErrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	dynamormerrors "github.com/theory-cloud/tabletheory/v3/pkg/errors"
	"github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	"go.uber.org/zap"
)

func disableSocialEnhancedServices(r *SocialRepository) {
	r.blockRepo.SetValidationService(nil)
	r.blockRepo.SetPermissionService(nil)
	r.blockRepo.SetCachingService(nil)
	r.blockRepo.SetEventService(nil)
	r.muteRepo.SetValidationService(nil)
	r.muteRepo.SetPermissionService(nil)
	r.muteRepo.SetCachingService(nil)
	r.muteRepo.SetEventService(nil)
	r.announceRepo.SetValidationService(nil)
	r.announceRepo.SetPermissionService(nil)
	r.announceRepo.SetCachingService(nil)
	r.announceRepo.SetEventService(nil)
	r.accountPinRepo.SetValidationService(nil)
	r.accountPinRepo.SetPermissionService(nil)
	r.accountPinRepo.SetCachingService(nil)
	r.accountPinRepo.SetEventService(nil)
	r.accountNoteRepo.SetValidationService(nil)
	r.accountNoteRepo.SetPermissionService(nil)
	r.accountNoteRepo.SetCachingService(nil)
	r.accountNoteRepo.SetEventService(nil)
	r.statusPinRepo.SetValidationService(nil)
	r.statusPinRepo.SetPermissionService(nil)
	r.statusPinRepo.SetCachingService(nil)
	r.statusPinRepo.SetEventService(nil)
}

func TestSocialRepository_Round08_SweepHappyPaths(t *testing.T) {
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
		case *models.Block:
			*dest = models.Block{Actor: "https://example.com/users/alice", Object: "https://example.com/users/bob", ID: "b1"}
			_ = dest.UpdateKeys()
		case *models.Mute:
			*dest = models.Mute{Actor: "alice", Object: "bob", ID: "m1"}
			_ = dest.UpdateKeys()
		case *models.Announce:
			*dest = models.Announce{Actor: "alice", Object: "s1", ID: "a1"}
			_ = dest.UpdateKeys()
		case *models.AccountPin:
			*dest = models.AccountPin{Username: "alice", PinnedActorID: "acct:bob", PinnedUsername: "bob"}
			_ = dest.UpdateKeys()
		case *models.StatusPin:
			*dest = models.StatusPin{Username: "alice", StatusID: "s1"}
			_ = dest.UpdateKeys()
		case *models.AccountNote:
			*dest = models.AccountNote{Username: "alice", TargetActorID: "acct:bob", Note: "n"}
			_ = dest.UpdateKeys()
		default:
		}
	})

	mockQuery.On("All", mock.Anything).Return(nil).Maybe().Run(func(args mock.Arguments) {
		switch dest := args.Get(0).(type) {
		case *[]models.Block:
			*dest = []models.Block{
				{Actor: "https://example.com/users/alice", Object: "https://example.com/users/bob", ID: "b1", SK: "BLOCKED#bob"},
				{Actor: "https://example.com/users/alice", Object: "https://example.com/users/carl", ID: "b2", SK: "BLOCKED#carl", GSI5SK: "SK#2"},
			}
		case *[]models.Mute:
			*dest = []models.Mute{
				{Actor: "alice", Object: "bob", ID: "m1", SK: "MUTED#bob"},
				{Actor: "alice", Object: "carl", ID: "m2", SK: "MUTED#carl"},
			}
		case *[]models.Announce:
			*dest = []models.Announce{
				{Actor: "alice", Object: "s1", ID: "a1", SK: "ACTOR#alice", GSI4SK: "SK#1"},
				{Actor: "bob", Object: "s1", ID: "a2", SK: "ACTOR#bob", GSI4SK: "SK#2"},
			}
		default:
		}
	})

	mockQuery.On("Scan", mock.Anything).Return(nil).Maybe().Run(func(args mock.Arguments) {
		switch dest := args.Get(0).(type) {
		case *[]models.Announce:
			*dest = []models.Announce{
				{Actor: "alice", Object: "s1", ID: "a1", PK: "OBJECT#s1#ANNOUNCES", SK: "ACTOR#alice"},
				{Actor: "bob", Object: "s1", ID: "a2", PK: "OBJECT#s1#ANNOUNCES", SK: "ACTOR#bob"},
			}
		case *[]models.AccountPin:
			*dest = []models.AccountPin{
				{Username: "alice", PinnedActorID: "acct:bob", PinnedUsername: "bob", SK: "PIN#acct:bob"},
				{Username: "alice", PinnedActorID: "acct:carl", PinnedUsername: "carl", SK: "PIN#acct:carl"},
			}
		case *[]models.StatusPin:
			*dest = []models.StatusPin{
				{Username: "alice", StatusID: "s1", CreatedAt: time.Now().Add(-time.Hour), SK: "STATUS#s1"},
				{Username: "alice", StatusID: "s2", CreatedAt: time.Now().Add(-time.Minute), SK: "STATUS#s2"},
			}
		default:
		}
	})

	repo := NewSocialRepository(mockDB, "table", zap.NewNop(), nil)
	disableSocialEnhancedServices(repo)

	require.NoError(t, repo.CreateBlock(ctx, &storage.Block{Actor: "https://example.com/users/alice", Object: "https://example.com/users/bob"}))
	require.NoError(t, repo.DeleteBlock(ctx, "https://example.com/users/alice", "https://example.com/users/bob"))
	_, err := repo.GetBlock(ctx, "https://example.com/users/alice", "https://example.com/users/bob")
	require.NoError(t, err)
	_, err = repo.IsBlocked(ctx, "https://example.com/users/alice", "https://example.com/users/bob")
	require.NoError(t, err)
	_, _, err = repo.GetBlockedUsers(ctx, "https://example.com/users/alice", 1, "")
	require.NoError(t, err)
	_, _, err = repo.GetBlockedByUsers(ctx, "https://example.com/users/bob", 1, "SK#0")
	require.NoError(t, err)

	require.NoError(t, repo.CreateMute(ctx, &storage.Mute{Actor: "alice", Object: "bob"}))
	require.NoError(t, repo.DeleteMute(ctx, "alice", "bob"))
	_, err = repo.GetMute(ctx, "alice", "bob")
	require.NoError(t, err)
	_, err = repo.IsMuted(ctx, "alice", "bob")
	require.NoError(t, err)
	_, _, err = repo.GetMutedUsers(ctx, "alice", 1, "Z")
	require.NoError(t, err)

	announce := &storage.Announce{Actor: "alice", Object: "s1"}
	require.NoError(t, repo.CreateAnnounce(ctx, announce))
	require.NotEmpty(t, announce.ID)
	require.NoError(t, repo.DeleteAnnounce(ctx, "alice", "s1"))
	_, err = repo.GetAnnounce(ctx, "alice", "s1")
	require.NoError(t, err)
	_, err = repo.HasUserAnnounced(ctx, "alice", "s1")
	require.NoError(t, err)
	_, _, err = repo.GetStatusAnnounces(ctx, "s1", 1, "Z")
	require.NoError(t, err)
	_, _, err = repo.GetActorAnnounces(ctx, "alice", 1, "Z")
	require.NoError(t, err)
	_, err = repo.CountObjectAnnounces(ctx, "s1")
	require.NoError(t, err)
	require.NoError(t, repo.CascadeDeleteAnnounces(ctx, "s1"))

	require.Error(t, repo.CreateAccountPin(ctx, &storage.AccountPin{Username: "alice", PinnedActorID: "acct:bob", PinnedUsername: "bob"}))
	require.NoError(t, repo.DeleteAccountPin(ctx, "alice", "acct:bob"))
	_, err = repo.GetAccountPins(ctx, "alice")
	require.NoError(t, err)
	_, _, err = repo.GetAccountPinsPaginated(ctx, "alice", 1, "")
	require.NoError(t, err)
	_, err = repo.IsAccountPinned(ctx, "alice", "acct:bob")
	require.NoError(t, err)

	note := &storage.AccountNote{Username: "alice", TargetActorID: "acct:bob", Note: "hello"}
	require.NoError(t, repo.CreateAccountNote(ctx, note))
	require.NoError(t, repo.UpdateAccountNote(ctx, note))
	_, err = repo.GetAccountNote(ctx, "alice", "acct:bob")
	require.NoError(t, err)
	require.NoError(t, repo.DeleteAccountNote(ctx, "alice", "acct:bob"))

	require.NoError(t, repo.CreateStatusPin(ctx, &storage.StatusPin{Username: "alice", StatusID: "s1", CreatedAt: time.Now()}))
	require.NoError(t, repo.DeleteStatusPin(ctx, "alice", "s1"))
	_, err = repo.GetStatusPins(ctx, "alice")
	require.NoError(t, err)
	_, _, err = repo.GetStatusPinsPaginated(ctx, "alice", 1, "")
	require.NoError(t, err)
	_, err = repo.IsStatusPinned(ctx, "alice", "s1")
	require.NoError(t, err)
	require.NoError(t, repo.ReorderStatusPins(ctx, "alice", []string{"s1", "s2"}))
	_, err = repo.CountUserPinnedStatuses(ctx, "alice")
	require.NoError(t, err)

	assert.Equal(t, "alice", extractUsername("https://example.com/users/alice"))

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestSocialRepository_Round08_ErrorBranches(t *testing.T) {
	ctx := context.Background()

	t.Run("CreateBlock condition failed is treated as already exists", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Create").Return(dynamormerrors.ErrConditionFailed).Once()

		repo := NewSocialRepository(mockDB, "table", zap.NewNop(), nil)
		disableSocialEnhancedServices(repo)

		err := repo.CreateBlock(ctx, &storage.Block{Actor: "https://example.com/users/alice", Object: "https://example.com/users/bob"})
		require.Error(t, err)

		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
	})

	t.Run("GetBlock and IsBlocked handle not-found and errors", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Twice()
		mockQuery.On("First", mock.Anything).Return(dynamormerrors.ErrItemNotFound).Once()

		repo := NewSocialRepository(mockDB, "table", zap.NewNop(), nil)
		disableSocialEnhancedServices(repo)

		_, err := repo.GetBlock(ctx, "https://example.com/users/alice", "https://example.com/users/bob")
		require.Error(t, err)

		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
	})

	t.Run("IsBlocked not found returns false without error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Twice()
		mockQuery.On("First", mock.Anything).Return(dynamormerrors.ErrItemNotFound).Once()

		repo := NewSocialRepository(mockDB, "table", zap.NewNop(), nil)
		disableSocialEnhancedServices(repo)

		ok, err := repo.IsBlocked(ctx, "https://example.com/users/alice", "https://example.com/users/bob")
		require.NoError(t, err)
		assert.False(t, ok)

		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
	})

	t.Run("CreateAnnounce validates required fields and tolerates already-exists errors", func(t *testing.T) {
		repo := NewSocialRepository(new(mocks.MockDB), "table", zap.NewNop(), nil)
		disableSocialEnhancedServices(repo)
		require.Error(t, repo.CreateAnnounce(ctx, &storage.Announce{Actor: "", Object: "s1"}))

		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Create").Return(dynamormerrors.ErrConditionFailed).Once()

		repo = NewSocialRepository(mockDB, "table", zap.NewNop(), nil)
		disableSocialEnhancedServices(repo)
		require.NoError(t, repo.CreateAnnounce(ctx, &storage.Announce{Actor: "alice", Object: "s1"}))
	})

	t.Run("CreateAnnounce returns AppError as-is when not a known idempotent failure", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Create").Return(fmt.Errorf("something else")).Once()

		repo := NewSocialRepository(mockDB, "table", zap.NewNop(), nil)
		disableSocialEnhancedServices(repo)

		err := repo.CreateAnnounce(ctx, &storage.Announce{Actor: "alice", Object: "s1"})
		require.Error(t, err)
		_, ok := svcErrors.AsAppError(err)
		assert.True(t, ok)
	})

	t.Run("GetAnnounce maps not found errors", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Twice()
		mockQuery.On("First", mock.Anything).Return(dynamormerrors.ErrItemNotFound).Once()

		repo := NewSocialRepository(mockDB, "table", zap.NewNop(), nil)
		disableSocialEnhancedServices(repo)

		_, err := repo.GetAnnounce(ctx, "alice", "s1")
		require.Error(t, err)

		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
	})

	t.Run("CreateAccountPin succeeds when not already pinned", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQueryCheck := new(mocks.MockQuery)
		mockQueryCreate := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Twice()

		// IsAccountPinned -> not found.
		mockDB.On("Model", mock.Anything).Return(mockQueryCheck).Once()
		mockQueryCheck.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQueryCheck).Twice()
		mockQueryCheck.On("First", mock.Anything).Return(dynamormerrors.ErrItemNotFound).Once()

		// ValidateAndCreate -> Create.
		mockDB.On("Model", mock.Anything).Return(mockQueryCreate).Once()
		mockQueryCreate.On("Create").Return(nil).Once()

		repo := NewSocialRepository(mockDB, "table", zap.NewNop(), nil)
		disableSocialEnhancedServices(repo)

		err := repo.CreateAccountPin(ctx, &storage.AccountPin{Username: "alice", PinnedActorID: "acct:bob", PinnedUsername: "bob"})
		require.NoError(t, err)

		mockDB.AssertExpectations(t)
		mockQueryCheck.AssertExpectations(t)
		mockQueryCreate.AssertExpectations(t)
	})

	t.Run("CreateStatusPin enforces max pins", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQueryCount := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
		mockDB.On("Model", mock.Anything).Return(mockQueryCount).Once()
		mockQueryCount.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQueryCount).Once()
		mockQueryCount.On("Count").Return(int64(5), nil).Once()

		repo := NewSocialRepository(mockDB, "table", zap.NewNop(), nil)
		disableSocialEnhancedServices(repo)

		err := repo.CreateStatusPin(ctx, &storage.StatusPin{Username: "alice", StatusID: "s1"})
		require.Error(t, err)

		mockDB.AssertExpectations(t)
		mockQueryCount.AssertExpectations(t)
	})

	t.Run("IsStatusPinned returns false for not found", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Twice()
		mockQuery.On("First", mock.Anything).Return(dynamormerrors.ErrItemNotFound).Once()

		repo := NewSocialRepository(mockDB, "table", zap.NewNop(), nil)
		disableSocialEnhancedServices(repo)

		ok, err := repo.IsStatusPinned(ctx, "alice", "s1")
		require.NoError(t, err)
		assert.False(t, ok)

		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
	})

	t.Run("ReorderStatusPins validates requested set", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		// GetStatusPins -> Scan.
		mockDB.On("WithContext", mock.Anything).Return(mockDB).Twice()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Twice()
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Once()
		mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Limit", 11).Return(mockQuery).Once()
		mockQuery.On("Scan", mock.Anything).Return(nil).Once().Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]models.StatusPin)
			*dest = []models.StatusPin{
				{Username: "alice", StatusID: "s1", SK: "STATUS#s1"},
				{Username: "alice", StatusID: "s2", SK: "STATUS#s2"},
			}
		})

		// Second call (empty list): Scan again.
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Once()
		mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Limit", 11).Return(mockQuery).Once()
		mockQuery.On("Scan", mock.Anything).Return(nil).Once().Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]models.StatusPin)
			*dest = []models.StatusPin{
				{Username: "alice", StatusID: "s1", SK: "STATUS#s1"},
				{Username: "alice", StatusID: "s2", SK: "STATUS#s2"},
			}
		})

		repo := NewSocialRepository(mockDB, "table", zap.NewNop(), nil)
		disableSocialEnhancedServices(repo)

		require.Error(t, repo.ReorderStatusPins(ctx, "alice", []string{"missing"}))
		require.Error(t, repo.ReorderStatusPins(ctx, "alice", []string{"s1"}))

		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
	})

	t.Run("sentinel checks for not found/condition failed helpers", func(t *testing.T) {
		assert.True(t, dynamormerrors.IsNotFound(dynamormerrors.ErrItemNotFound))
		assert.True(t, dynamormerrors.IsConditionFailed(dynamormerrors.ErrConditionFailed))
	})
}

func TestSocialRepository_Round08_MoreBranches(t *testing.T) {
	ctx := context.Background()

	t.Run("CreateMute condition failed maps to already exists", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Create").Return(dynamormerrors.ErrConditionFailed).Once()

		repo := NewSocialRepository(mockDB, "table", zap.NewNop(), nil)
		disableSocialEnhancedServices(repo)

		require.Error(t, repo.CreateMute(ctx, &storage.Mute{Actor: "alice", Object: "bob"}))

		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
	})

	t.Run("GetMute and IsMuted handle not-found and errors", func(t *testing.T) {
		t.Run("GetMute not found", func(t *testing.T) {
			mockDB := new(mocks.MockDB)
			mockQuery := new(mocks.MockQuery)

			mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
			mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
			mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Twice()
			mockQuery.On("First", mock.Anything).Return(dynamormerrors.ErrItemNotFound).Once()

			repo := NewSocialRepository(mockDB, "table", zap.NewNop(), nil)
			disableSocialEnhancedServices(repo)

			_, err := repo.GetMute(ctx, "alice", "bob")
			require.Error(t, err)

			mockDB.AssertExpectations(t)
			mockQuery.AssertExpectations(t)
		})

		t.Run("IsMuted not found returns false", func(t *testing.T) {
			mockDB := new(mocks.MockDB)
			mockQuery := new(mocks.MockQuery)

			mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
			mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
			mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Twice()
			mockQuery.On("First", mock.Anything).Return(dynamormerrors.ErrItemNotFound).Once()

			repo := NewSocialRepository(mockDB, "table", zap.NewNop(), nil)
			disableSocialEnhancedServices(repo)

			ok, err := repo.IsMuted(ctx, "alice", "bob")
			require.NoError(t, err)
			assert.False(t, ok)

			mockDB.AssertExpectations(t)
			mockQuery.AssertExpectations(t)
		})

		t.Run("IsMuted error returns error", func(t *testing.T) {
			mockDB := new(mocks.MockDB)
			mockQuery := new(mocks.MockQuery)

			mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
			mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
			mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Twice()
			mockQuery.On("First", mock.Anything).Return(assert.AnError).Once()

			repo := NewSocialRepository(mockDB, "table", zap.NewNop(), nil)
			disableSocialEnhancedServices(repo)

			_, err := repo.IsMuted(ctx, "alice", "bob")
			require.Error(t, err)

			mockDB.AssertExpectations(t)
			mockQuery.AssertExpectations(t)
		})
	})

	t.Run("HasUserAnnounced not found returns false and errors propagate", func(t *testing.T) {
		t.Run("not found", func(t *testing.T) {
			mockDB := new(mocks.MockDB)
			mockQuery := new(mocks.MockQuery)

			mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
			mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
			mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Twice()
			mockQuery.On("First", mock.Anything).Return(dynamormerrors.ErrItemNotFound).Once()

			repo := NewSocialRepository(mockDB, "table", zap.NewNop(), nil)
			disableSocialEnhancedServices(repo)

			ok, err := repo.HasUserAnnounced(ctx, "alice", "s1")
			require.NoError(t, err)
			assert.False(t, ok)

			mockDB.AssertExpectations(t)
			mockQuery.AssertExpectations(t)
		})

		t.Run("error", func(t *testing.T) {
			mockDB := new(mocks.MockDB)
			mockQuery := new(mocks.MockQuery)

			mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
			mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
			mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Twice()
			mockQuery.On("First", mock.Anything).Return(assert.AnError).Once()

			repo := NewSocialRepository(mockDB, "table", zap.NewNop(), nil)
			disableSocialEnhancedServices(repo)

			_, err := repo.HasUserAnnounced(ctx, "alice", "s1")
			require.Error(t, err)

			mockDB.AssertExpectations(t)
			mockQuery.AssertExpectations(t)
		})
	})

	t.Run("CountObjectAnnounces maps count errors", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Count").Return(int64(0), assert.AnError).Once()

		repo := NewSocialRepository(mockDB, "table", zap.NewNop(), nil)
		disableSocialEnhancedServices(repo)

		_, err := repo.CountObjectAnnounces(ctx, "s1")
		require.Error(t, err)

		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
	})

	t.Run("CascadeDeleteAnnounces maps scan errors and tolerates delete errors", func(t *testing.T) {
		t.Run("scan error", func(t *testing.T) {
			mockDB := new(mocks.MockDB)
			mockQuery := new(mocks.MockQuery)

			mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
			mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
			mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Once()
			mockQuery.On("Scan", mock.Anything).Return(assert.AnError).Once()

			repo := NewSocialRepository(mockDB, "table", zap.NewNop(), nil)
			disableSocialEnhancedServices(repo)

			require.Error(t, repo.CascadeDeleteAnnounces(ctx, "s1"))

			mockDB.AssertExpectations(t)
			mockQuery.AssertExpectations(t)
		})

		t.Run("delete error continues", func(t *testing.T) {
			mockDB := new(mocks.MockDB)
			mockQueryScan := new(mocks.MockQuery)
			mockQueryDelete1 := new(mocks.MockQuery)
			mockQueryDelete2 := new(mocks.MockQuery)

			mockDB.On("WithContext", mock.Anything).Return(mockDB).Times(3)

			mockDB.On("Model", mock.Anything).Return(mockQueryScan).Once()
			mockQueryScan.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQueryScan).Once()
			mockQueryScan.On("Scan", mock.Anything).Return(nil).Once().Run(func(args mock.Arguments) {
				dest := args.Get(0).(*[]models.Announce)
				*dest = []models.Announce{
					{PK: "OBJECT#s1#ANNOUNCES", SK: "ACTOR#alice", Actor: "alice", Object: "s1"},
					{PK: "OBJECT#s1#ANNOUNCES", SK: "ACTOR#bob", Actor: "bob", Object: "s1"},
				}
			})

			mockDB.On("Model", mock.Anything).Return(mockQueryDelete1).Once()
			mockQueryDelete1.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQueryDelete1).Twice()
			mockQueryDelete1.On("Delete").Return(assert.AnError).Once()

			mockDB.On("Model", mock.Anything).Return(mockQueryDelete2).Once()
			mockQueryDelete2.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQueryDelete2).Twice()
			mockQueryDelete2.On("Delete").Return(nil).Once()

			repo := NewSocialRepository(mockDB, "table", zap.NewNop(), nil)
			disableSocialEnhancedServices(repo)

			require.NoError(t, repo.CascadeDeleteAnnounces(ctx, "s1"))

			mockDB.AssertExpectations(t)
			mockQueryScan.AssertExpectations(t)
			mockQueryDelete1.AssertExpectations(t)
			mockQueryDelete2.AssertExpectations(t)
		})
	})

	t.Run("Account pin helpers map delete errors and propagate check errors", func(t *testing.T) {
		t.Run("DeleteAccountPin error", func(t *testing.T) {
			mockDB := new(mocks.MockDB)
			mockQuery := new(mocks.MockQuery)

			mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
			mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
			mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Twice()
			mockQuery.On("Delete").Return(assert.AnError).Once()

			repo := NewSocialRepository(mockDB, "table", zap.NewNop(), nil)
			disableSocialEnhancedServices(repo)

			require.Error(t, repo.DeleteAccountPin(ctx, "alice", "acct:bob"))

			mockDB.AssertExpectations(t)
			mockQuery.AssertExpectations(t)
		})

		t.Run("IsAccountPinned error", func(t *testing.T) {
			mockDB := new(mocks.MockDB)
			mockQuery := new(mocks.MockQuery)

			mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
			mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
			mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Twice()
			mockQuery.On("First", mock.Anything).Return(assert.AnError).Once()

			repo := NewSocialRepository(mockDB, "table", zap.NewNop(), nil)
			disableSocialEnhancedServices(repo)

			_, err := repo.IsAccountPinned(ctx, "alice", "acct:bob")
			require.Error(t, err)

			mockDB.AssertExpectations(t)
			mockQuery.AssertExpectations(t)
		})
	})

	t.Run("GetAccountPinsPaginated maps query errors", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Filter", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Once()
		mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Limit", 2).Return(mockQuery).Once()
		mockQuery.On("Scan", mock.Anything).Return(assert.AnError).Once()

		repo := NewSocialRepository(mockDB, "table", zap.NewNop(), nil)
		disableSocialEnhancedServices(repo)

		_, _, err := repo.GetAccountPinsPaginated(ctx, "alice", 1, "")
		require.Error(t, err)

		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
	})

	t.Run("Account notes cover not-found and delete errors", func(t *testing.T) {
		t.Run("GetAccountNote not found", func(t *testing.T) {
			mockDB := new(mocks.MockDB)
			mockQuery := new(mocks.MockQuery)

			mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
			mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
			mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Twice()
			mockQuery.On("First", mock.Anything).Return(dynamormerrors.ErrItemNotFound).Once()

			repo := NewSocialRepository(mockDB, "table", zap.NewNop(), nil)
			disableSocialEnhancedServices(repo)

			_, err := repo.GetAccountNote(ctx, "alice", "acct:bob")
			require.Error(t, err)

			mockDB.AssertExpectations(t)
			mockQuery.AssertExpectations(t)
		})

		t.Run("DeleteAccountNote error", func(t *testing.T) {
			mockDB := new(mocks.MockDB)
			mockQuery := new(mocks.MockQuery)

			mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
			mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
			mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Twice()
			mockQuery.On("Delete").Return(assert.AnError).Once()

			repo := NewSocialRepository(mockDB, "table", zap.NewNop(), nil)
			disableSocialEnhancedServices(repo)

			require.Error(t, repo.DeleteAccountNote(ctx, "alice", "acct:bob"))

			mockDB.AssertExpectations(t)
			mockQuery.AssertExpectations(t)
		})
	})

	t.Run("CreateStatusPin maps already-exists and query errors", func(t *testing.T) {
		t.Run("count error", func(t *testing.T) {
			mockDB := new(mocks.MockDB)
			mockQuery := new(mocks.MockQuery)

			mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
			mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
			mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Once()
			mockQuery.On("Count").Return(int64(0), assert.AnError).Once()

			repo := NewSocialRepository(mockDB, "table", zap.NewNop(), nil)
			disableSocialEnhancedServices(repo)

			require.Error(t, repo.CreateStatusPin(ctx, &storage.StatusPin{Username: "alice", StatusID: "s1"}))

			mockDB.AssertExpectations(t)
			mockQuery.AssertExpectations(t)
		})

		t.Run("already exists", func(t *testing.T) {
			mockDB := new(mocks.MockDB)
			mockQueryCount := new(mocks.MockQuery)
			mockQueryCreate := new(mocks.MockQuery)

			mockDB.On("WithContext", mock.Anything).Return(mockDB).Twice()

			mockDB.On("Model", mock.Anything).Return(mockQueryCount).Once()
			mockQueryCount.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQueryCount).Once()
			mockQueryCount.On("Count").Return(int64(0), nil).Once()

			mockDB.On("Model", mock.Anything).Return(mockQueryCreate).Once()
			mockQueryCreate.On("Create").Return(dynamormerrors.ErrConditionFailed).Once()

			repo := NewSocialRepository(mockDB, "table", zap.NewNop(), nil)
			disableSocialEnhancedServices(repo)

			require.Error(t, repo.CreateStatusPin(ctx, &storage.StatusPin{Username: "alice", StatusID: "s1"}))

			mockDB.AssertExpectations(t)
			mockQueryCount.AssertExpectations(t)
			mockQueryCreate.AssertExpectations(t)
		})
	})

	t.Run("GetStatusPinsPaginated maps query errors", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Once()
		mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Limit", 2).Return(mockQuery).Once()
		mockQuery.On("Scan", mock.Anything).Return(assert.AnError).Once()

		repo := NewSocialRepository(mockDB, "table", zap.NewNop(), nil)
		disableSocialEnhancedServices(repo)

		_, _, err := repo.GetStatusPinsPaginated(ctx, "alice", 1, "")
		require.Error(t, err)

		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
	})

	t.Run("IsStatusPinned maps errors", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Twice()
		mockQuery.On("First", mock.Anything).Return(assert.AnError).Once()

		repo := NewSocialRepository(mockDB, "table", zap.NewNop(), nil)
		disableSocialEnhancedServices(repo)

		_, err := repo.IsStatusPinned(ctx, "alice", "s1")
		require.Error(t, err)

		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
	})

	t.Run("ReorderStatusPins maps delete/create failures", func(t *testing.T) {
		t.Run("delete failure", func(t *testing.T) {
			mockDB := new(mocks.MockDB)
			mockQueryScan := new(mocks.MockQuery)
			mockQueryDelete := new(mocks.MockQuery)

			mockDB.On("WithContext", mock.Anything).Return(mockDB).Twice()

			// GetStatusPins -> Scan.
			mockDB.On("Model", mock.Anything).Return(mockQueryScan).Once()
			mockQueryScan.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQueryScan).Once()
			mockQueryScan.On("OrderBy", mock.Anything, mock.Anything).Return(mockQueryScan).Once()
			mockQueryScan.On("Limit", 11).Return(mockQueryScan).Once()
			mockQueryScan.On("Scan", mock.Anything).Return(nil).Once().Run(func(args mock.Arguments) {
				dest := args.Get(0).(*[]models.StatusPin)
				*dest = []models.StatusPin{{Username: "alice", StatusID: "s1", SK: "STATUS#s1"}}
			})

			// Delete fails.
			mockDB.On("Model", mock.Anything).Return(mockQueryDelete).Once()
			mockQueryDelete.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQueryDelete).Twice()
			mockQueryDelete.On("Delete").Return(assert.AnError).Once()

			repo := NewSocialRepository(mockDB, "table", zap.NewNop(), nil)
			disableSocialEnhancedServices(repo)

			require.Error(t, repo.ReorderStatusPins(ctx, "alice", []string{"s1"}))

			mockDB.AssertExpectations(t)
			mockQueryScan.AssertExpectations(t)
			mockQueryDelete.AssertExpectations(t)
		})

		t.Run("create failure", func(t *testing.T) {
			mockDB := new(mocks.MockDB)
			mockQueryScan := new(mocks.MockQuery)
			mockQueryDelete := new(mocks.MockQuery)
			mockQueryCreate := new(mocks.MockQuery)

			mockDB.On("WithContext", mock.Anything).Return(mockDB).Times(3)

			// GetStatusPins -> Scan.
			mockDB.On("Model", mock.Anything).Return(mockQueryScan).Once()
			mockQueryScan.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQueryScan).Once()
			mockQueryScan.On("OrderBy", mock.Anything, mock.Anything).Return(mockQueryScan).Once()
			mockQueryScan.On("Limit", 11).Return(mockQueryScan).Once()
			mockQueryScan.On("Scan", mock.Anything).Return(nil).Once().Run(func(args mock.Arguments) {
				dest := args.Get(0).(*[]models.StatusPin)
				*dest = []models.StatusPin{{Username: "alice", StatusID: "s1", SK: "STATUS#s1"}}
			})

			// Delete ok.
			mockDB.On("Model", mock.Anything).Return(mockQueryDelete).Once()
			mockQueryDelete.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQueryDelete).Twice()
			mockQueryDelete.On("Delete").Return(nil).Once()

			// Create fails.
			mockDB.On("Model", mock.Anything).Return(mockQueryCreate).Once()
			mockQueryCreate.On("Create").Return(assert.AnError).Once()

			repo := NewSocialRepository(mockDB, "table", zap.NewNop(), nil)
			disableSocialEnhancedServices(repo)

			require.Error(t, repo.ReorderStatusPins(ctx, "alice", []string{"s1"}))

			mockDB.AssertExpectations(t)
			mockQueryScan.AssertExpectations(t)
			mockQueryDelete.AssertExpectations(t)
			mockQueryCreate.AssertExpectations(t)
		})
	})

	t.Run("DeleteBlock and DeleteMute map delete errors", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Twice()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Twice()
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Times(4)
		mockQuery.On("Delete").Return(assert.AnError).Twice()

		repo := NewSocialRepository(mockDB, "table", zap.NewNop(), nil)
		disableSocialEnhancedServices(repo)

		require.Error(t, repo.DeleteBlock(ctx, "https://example.com/users/alice", "https://example.com/users/bob"))
		require.Error(t, repo.DeleteMute(ctx, "alice", "bob"))

		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
	})

	t.Run("GetBlockedUsers and GetMutedUsers map query errors", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Twice()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Twice()
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Twice()
		mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery).Maybe()
		mockQuery.On("Limit", mock.Anything).Return(mockQuery).Twice()
		mockQuery.On("All", mock.Anything).Return(assert.AnError).Twice()

		repo := NewSocialRepository(mockDB, "table", zap.NewNop(), nil)
		disableSocialEnhancedServices(repo)

		_, _, err := repo.GetBlockedUsers(ctx, "https://example.com/users/alice", 1, "")
		require.Error(t, err)
		_, _, err = repo.GetMutedUsers(ctx, "alice", 1, "")
		require.Error(t, err)

		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
	})
}

func TestSocialRepository_Round08_FinalBoost(t *testing.T) {
	ctx := context.Background()

	t.Run("CreateBlock non-conditional create error is mapped", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Create").Return(assert.AnError).Once()

		repo := NewSocialRepository(mockDB, "table", zap.NewNop(), nil)
		disableSocialEnhancedServices(repo)

		require.Error(t, repo.CreateBlock(ctx, &storage.Block{Actor: "https://example.com/users/alice", Object: "https://example.com/users/bob"}))

		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
	})

	t.Run("DeleteAnnounce maps delete errors", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Twice()
		mockQuery.On("Delete").Return(assert.AnError).Once()

		repo := NewSocialRepository(mockDB, "table", zap.NewNop(), nil)
		disableSocialEnhancedServices(repo)

		require.Error(t, repo.DeleteAnnounce(ctx, "alice", "s1"))

		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
	})

	t.Run("GetAnnounce maps non-notfound errors", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Twice()
		mockQuery.On("First", mock.Anything).Return(assert.AnError).Once()

		repo := NewSocialRepository(mockDB, "table", zap.NewNop(), nil)
		disableSocialEnhancedServices(repo)

		_, err := repo.GetAnnounce(ctx, "alice", "s1")
		require.Error(t, err)

		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
	})

	t.Run("CreateAccountPin propagates IsAccountPinned errors", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Twice()
		mockQuery.On("First", mock.Anything).Return(assert.AnError).Once()

		repo := NewSocialRepository(mockDB, "table", zap.NewNop(), nil)
		disableSocialEnhancedServices(repo)

		require.Error(t, repo.CreateAccountPin(ctx, &storage.AccountPin{Username: "alice", PinnedActorID: "acct:bob", PinnedUsername: "bob"}))

		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
	})

	t.Run("Account note create/update map create errors", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQueryCreate1 := new(mocks.MockQuery)
		mockQueryCreate2 := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Twice()
		mockDB.On("Model", mock.Anything).Return(mockQueryCreate1).Once()
		mockQueryCreate1.On("Create").Return(assert.AnError).Once()
		mockDB.On("Model", mock.Anything).Return(mockQueryCreate2).Once()
		mockQueryCreate2.On("Create").Return(assert.AnError).Once()

		repo := NewSocialRepository(mockDB, "table", zap.NewNop(), nil)
		disableSocialEnhancedServices(repo)

		require.Error(t, repo.CreateAccountNote(ctx, &storage.AccountNote{Username: "alice", TargetActorID: "acct:bob", Note: "n"}))
		require.Error(t, repo.UpdateAccountNote(ctx, &storage.AccountNote{Username: "alice", TargetActorID: "acct:bob", Note: "n"}))

		mockDB.AssertExpectations(t)
		mockQueryCreate1.AssertExpectations(t)
		mockQueryCreate2.AssertExpectations(t)
	})
}

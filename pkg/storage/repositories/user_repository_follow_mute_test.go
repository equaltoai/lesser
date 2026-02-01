package repositories

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	dynamormerrors "github.com/theory-cloud/tabletheory/pkg/errors"
	"github.com/theory-cloud/tabletheory/pkg/mocks"
	"go.uber.org/zap"
)

func TestUserRepository_AcceptFollow_SuccessUpdatesCounts(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	mockUpdateBuilder := new(mocks.MockUpdateBuilder)
	repo := NewUserRepository(mockDB, "test-table", zap.NewNop())

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
		out := args.Get(0).(*models.RelationshipRecord)
		out.PK = "FOLLOW#alice"
		out.SK = "FOLLOWING#bob"
		out.State = models.RelationshipPending
	}).Return(nil)
	mockQuery.On("Update", mock.Anything).Return(nil)

	mockQuery.On("UpdateBuilder").Return(mockUpdateBuilder)
	mockUpdateBuilder.On("Add", mock.Anything, mock.Anything).Return(mockUpdateBuilder)
	mockUpdateBuilder.On("Condition", mock.Anything, mock.Anything, mock.Anything).Return(mockUpdateBuilder)
	mockUpdateBuilder.On("Execute").Return(nil)

	err := repo.AcceptFollow(context.Background(), "alice", "bob")
	assert.NoError(t, err)
}

func TestUserRepository_AcceptFollow_NotFound(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewUserRepository(mockDB, "test-table", zap.NewNop())

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("First", mock.Anything).Return(dynamormerrors.ErrItemNotFound)

	err := repo.AcceptFollow(context.Background(), "alice", "bob")
	assert.Error(t, err)
}

func TestUserRepository_AcceptFollow_FirstError(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewUserRepository(mockDB, "test-table", zap.NewNop())

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("First", mock.Anything).Return(ErrTestMockError)

	err := repo.AcceptFollow(context.Background(), "alice", "bob")
	assert.Error(t, err)
}

func TestUserRepository_AcceptFollow_UpdateRelationshipError(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewUserRepository(mockDB, "test-table", zap.NewNop())

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
		out := args.Get(0).(*models.RelationshipRecord)
		out.PK = "FOLLOW#alice"
		out.SK = "FOLLOWING#bob"
		out.State = models.RelationshipPending
	}).Return(nil)
	mockQuery.On("Update", mock.Anything).Return(ErrTestMockError)

	err := repo.AcceptFollow(context.Background(), "alice", "bob")
	assert.Error(t, err)
}

func TestUserRepository_RejectFollow_Success(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewUserRepository(mockDB, "test-table", zap.NewNop())

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
		out := args.Get(0).(*models.RelationshipRecord)
		out.PK = "FOLLOW#alice"
		out.SK = "FOLLOWING#bob"
		out.State = models.RelationshipPending
	}).Return(nil)
	mockQuery.On("Update", mock.Anything).Return(nil)

	err := repo.RejectFollow(context.Background(), "alice", "bob")
	assert.NoError(t, err)
}

func TestUserRepository_RejectFollow_FirstError(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewUserRepository(mockDB, "test-table", zap.NewNop())

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("First", mock.Anything).Return(ErrTestMockError)

	err := repo.RejectFollow(context.Background(), "alice", "bob")
	assert.Error(t, err)
}

func TestUserRepository_RejectFollow_UpdateRelationshipError(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewUserRepository(mockDB, "test-table", zap.NewNop())

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
		out := args.Get(0).(*models.RelationshipRecord)
		out.PK = "FOLLOW#alice"
		out.SK = "FOLLOWING#bob"
		out.State = models.RelationshipPending
	}).Return(nil)
	mockQuery.On("Update", mock.Anything).Return(ErrTestMockError)

	err := repo.RejectFollow(context.Background(), "alice", "bob")
	assert.Error(t, err)
}

func TestUserRepository_updateActorCount_UnknownType(t *testing.T) {
	repo := NewUserRepository(nil, "test-table", zap.NewNop())
	err := repo.updateActorCount(context.Background(), "alice", 1, "unknown")
	assert.Error(t, err)
}

func TestUserRepository_updateActorCount_NotFoundDoesNotError(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	mockUpdateBuilder := new(mocks.MockUpdateBuilder)
	repo := NewUserRepository(mockDB, "test-table", zap.NewNop())

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("UpdateBuilder").Return(mockUpdateBuilder)
	mockUpdateBuilder.On("Add", mock.Anything, mock.Anything).Return(mockUpdateBuilder)
	mockUpdateBuilder.On("Condition", mock.Anything, mock.Anything, mock.Anything).Return(mockUpdateBuilder)
	mockUpdateBuilder.On("Execute").Return(dynamormerrors.ErrItemNotFound)

	err := repo.updateActorCount(context.Background(), "alice", 1, countUpdateFollowers)
	assert.NoError(t, err)
}

func TestUserRepository_updateActorCount_ExecuteError(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	mockUpdateBuilder := new(mocks.MockUpdateBuilder)
	repo := NewUserRepository(mockDB, "test-table", zap.NewNop())

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("UpdateBuilder").Return(mockUpdateBuilder)
	mockUpdateBuilder.On("Add", mock.Anything, mock.Anything).Return(mockUpdateBuilder)
	mockUpdateBuilder.On("Condition", mock.Anything, mock.Anything, mock.Anything).Return(mockUpdateBuilder)
	mockUpdateBuilder.On("Execute").Return(ErrTestMockError)

	err := repo.updateActorCount(context.Background(), "alice", 1, countUpdateFollowers)
	assert.Error(t, err)
}

func TestUserRepository_CreateConversationMute_ConditionFailedReturnsConflict(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewUserRepository(mockDB, "test-table", zap.NewNop())

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.ConversationMute")).Return(mockQuery)
	mockQuery.On("Create").Return(dynamormerrors.ErrConditionFailed)

	err := repo.CreateConversationMute(context.Background(), &storage.ConversationMute{
		Username:       "alice",
		ConversationID: "c1",
	})
	assert.Error(t, err)
}

func TestUserRepository_ConversationMute_Flow(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewUserRepository(mockDB, "test-table", zap.NewNop())

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Create").Return(nil)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("Delete").Return(nil)

	err := repo.CreateConversationMute(context.Background(), &storage.ConversationMute{
		Username:       "alice",
		ConversationID: "c1",
		ExpiresAt:      time.Now().Add(time.Hour),
	})
	assert.NoError(t, err)

	mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
		out := args.Get(0).(*models.ConversationMute)
		out.Username = "alice"
		out.ConversationID = "c1"
		out.ExpiresAt = time.Now().Add(time.Hour)
	}).Return(nil)

	muted, err := repo.IsConversationMuted(context.Background(), "alice", "c1")
	assert.NoError(t, err)
	assert.True(t, muted)

	err = repo.DeleteConversationMute(context.Background(), "alice", "c1")
	assert.NoError(t, err)
}

func TestUserRepository_IsConversationMuted_ExpiredReturnsFalse(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewUserRepository(mockDB, "test-table", zap.NewNop())

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
		out := args.Get(0).(*models.ConversationMute)
		out.ExpiresAt = time.Now().Add(-time.Minute)
	}).Return(nil)

	muted, err := repo.IsConversationMuted(context.Background(), "alice", "c1")
	assert.NoError(t, err)
	assert.False(t, muted)
}

func TestUserRepository_GetMutedConversations_FiltersExpired(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewUserRepository(mockDB, "test-table", zap.NewNop())

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		out := args.Get(0).(*[]models.ConversationMute)
		*out = []models.ConversationMute{
			{ConversationID: "active", ExpiresAt: time.Now().Add(time.Hour)},
			{ConversationID: "expired", ExpiresAt: time.Now().Add(-time.Hour)},
		}
	}).Return(nil)

	ids, err := repo.GetMutedConversations(context.Background(), "alice")
	assert.NoError(t, err)
	assert.Equal(t, []string{"active"}, ids)
}

func TestUserRepository_IsNotificationMuted_AllDisabledTrue(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockPrefsQuery := new(mocks.MockQuery)
	mockNotifQuery := new(mocks.MockQuery)
	repo := NewUserRepository(mockDB, "test-table", zap.NewNop())

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.UserPreferences")).Return(mockPrefsQuery)
	mockPrefsQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockPrefsQuery)
	mockPrefsQuery.On("First", mock.Anything).Return(dynamormerrors.ErrItemNotFound)

	mockDB.On("Model", mock.AnythingOfType("*models.NotificationPreferences")).Return(mockNotifQuery)
	mockNotifQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockNotifQuery)
	mockNotifQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
		// All fields default to false -> effectively muted.
		_ = args.Get(0).(*models.NotificationPreferences)
	}).Return(nil)

	muted, err := repo.IsNotificationMuted(context.Background(), "alice", "bob")
	assert.NoError(t, err)
	assert.True(t, muted)
}

func TestUserRepository_IsNotificationMuted_FallbackToReblogFilters(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockPrefsQuery := new(mocks.MockQuery)
	mockNotifQuery := new(mocks.MockQuery)
	repo := NewUserRepository(mockDB, "test-table", zap.NewNop())

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.UserPreferences")).Return(mockPrefsQuery)
	mockPrefsQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockPrefsQuery)
	mockPrefsQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
		out := args.Get(0).(*models.UserPreferences)
		out.Username = "alice"
		out.UpdateKeys()
		out.ReblogFilters = map[string]bool{"bob": false}
	}).Return(nil)

	mockDB.On("Model", mock.AnythingOfType("*models.NotificationPreferences")).Return(mockNotifQuery)
	mockNotifQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockNotifQuery)
	mockNotifQuery.On("First", mock.Anything).Return(dynamormerrors.ErrItemNotFound)

	muted, err := repo.IsNotificationMuted(context.Background(), "alice", "bob")
	assert.NoError(t, err)
	assert.True(t, muted)
}

func TestUserRepository_IsNotificationMuted_PreferencesErrorReturnsError(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockPrefsQuery := new(mocks.MockQuery)
	repo := NewUserRepository(mockDB, "test-table", zap.NewNop())

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.UserPreferences")).Return(mockPrefsQuery)
	mockPrefsQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockPrefsQuery)
	mockPrefsQuery.On("First", mock.Anything).Return(ErrTestMockError)

	muted, err := repo.IsNotificationMuted(context.Background(), "alice", "bob")
	assert.Error(t, err)
	assert.False(t, muted)
}

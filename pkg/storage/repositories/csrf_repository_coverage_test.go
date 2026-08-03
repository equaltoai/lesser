package repositories

import (
	"context"
	stdErrors "errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	dynamormErrors "github.com/theory-cloud/tabletheory/v3/pkg/errors"
	"github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	"go.uber.org/zap"
)

func TestCSRFRepository_Get_NotFoundReturnsInvalidNoError(t *testing.T) {
	ctx := context.Background()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewCSRFRepository(mockDB, "test-table", zap.NewNop(), nil)

	mockDB.On("WithContext", ctx).Return(mockDB).Once()
	mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
	mockQuery.On("Where", "PK", "=", "CSRF#tok-1").Return(mockQuery).Once()
	mockQuery.On("Where", "SK", "=", models.SKToken).Return(mockQuery).Once()
	mockQuery.On("First", mock.AnythingOfType("*models.CSRFToken")).Return(dynamormErrors.ErrItemNotFound).Once()

	token, userID, expiresAt, valid, err := repo.Get(ctx, "tok-1")
	require.NoError(t, err)
	require.False(t, valid)
	require.Empty(t, token)
	require.Empty(t, userID)
	require.True(t, expiresAt.IsZero())
}

func TestCSRFRepository_Get_ExpiredReturnsInvalidNoError(t *testing.T) {
	ctx := context.Background()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewCSRFRepository(mockDB, "test-table", zap.NewNop(), nil)

	mockDB.On("WithContext", ctx).Return(mockDB).Once()
	mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
	mockQuery.On("Where", "PK", "=", "CSRF#tok-2").Return(mockQuery).Once()
	mockQuery.On("Where", "SK", "=", models.SKToken).Return(mockQuery).Once()
	mockQuery.On("First", mock.AnythingOfType("*models.CSRFToken")).Run(func(args mock.Arguments) {
		record := args.Get(0).(*models.CSRFToken)
		record.Token = "tok-2"
		record.UserID = "user-1"
		record.Used = false
		record.ExpiresAt = time.Now().Add(-1 * time.Minute).Unix()
	}).Return(nil).Once()

	_, _, _, valid, err := repo.Get(ctx, "tok-2")
	require.NoError(t, err)
	require.False(t, valid)
}

func TestCSRFRepository_Get_UsedReturnsInvalidNoError(t *testing.T) {
	ctx := context.Background()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewCSRFRepository(mockDB, "test-table", zap.NewNop(), nil)

	mockDB.On("WithContext", ctx).Return(mockDB).Once()
	mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
	mockQuery.On("Where", "PK", "=", "CSRF#tok-3").Return(mockQuery).Once()
	mockQuery.On("Where", "SK", "=", models.SKToken).Return(mockQuery).Once()
	mockQuery.On("First", mock.AnythingOfType("*models.CSRFToken")).Run(func(args mock.Arguments) {
		record := args.Get(0).(*models.CSRFToken)
		record.Token = "tok-3"
		record.UserID = "user-1"
		record.Used = true
		record.ExpiresAt = time.Now().Add(5 * time.Minute).Unix()
	}).Return(nil).Once()

	_, _, _, valid, err := repo.Get(ctx, "tok-3")
	require.NoError(t, err)
	require.False(t, valid)
}

func TestCSRFRepository_Get_ValidReturnsToken(t *testing.T) {
	ctx := context.Background()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewCSRFRepository(mockDB, "test-table", zap.NewNop(), nil)

	expiresAt := time.Now().Add(10 * time.Minute)

	mockDB.On("WithContext", ctx).Return(mockDB).Once()
	mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
	mockQuery.On("Where", "PK", "=", "CSRF#tok-4").Return(mockQuery).Once()
	mockQuery.On("Where", "SK", "=", models.SKToken).Return(mockQuery).Once()
	mockQuery.On("First", mock.AnythingOfType("*models.CSRFToken")).Run(func(args mock.Arguments) {
		record := args.Get(0).(*models.CSRFToken)
		record.Token = "tok-4"
		record.UserID = "user-4"
		record.Used = false
		record.ExpiresAt = expiresAt.Unix()
	}).Return(nil).Once()

	token, userID, gotExpires, valid, err := repo.Get(ctx, "tok-4")
	require.NoError(t, err)
	require.True(t, valid)
	require.Equal(t, "tok-4", token)
	require.Equal(t, "user-4", userID)
	require.WithinDuration(t, expiresAt, gotExpires, time.Second)
}

func TestCSRFRepository_Get_DBErrorReturnsError(t *testing.T) {
	ctx := context.Background()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewCSRFRepository(mockDB, "test-table", zap.NewNop(), nil)

	mockDB.On("WithContext", ctx).Return(mockDB).Once()
	mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
	mockQuery.On("Where", "PK", "=", "CSRF#tok-5").Return(mockQuery).Once()
	mockQuery.On("Where", "SK", "=", models.SKToken).Return(mockQuery).Once()
	mockQuery.On("First", mock.AnythingOfType("*models.CSRFToken")).Return(ErrTestMockError).Once()

	_, _, _, _, err := repo.Get(ctx, "tok-5")
	require.Error(t, err)
}

func TestCSRFRepository_Delete_DeleteErrorReturnsError(t *testing.T) {
	ctx := context.Background()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewCSRFRepository(mockDB, "test-table", zap.NewNop(), nil)

	mockDB.On("WithContext", ctx).Return(mockDB).Maybe()
	mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()

	// ValidateAndDelete checks permissions by attempting a Get first (First -> not found), then attempts Delete.
	mockQuery.On("Where", "PK", "=", "CSRF#tok-del").Return(mockQuery).Twice()
	mockQuery.On("Where", "SK", "=", models.SKToken).Return(mockQuery).Twice()
	mockQuery.On("First", mock.AnythingOfType("*models.CSRFToken")).Return(dynamormErrors.ErrItemNotFound).Once()
	mockQuery.On("Delete").Return(ErrTestMockError).Once()

	require.Error(t, repo.Delete(ctx, "tok-del"))
}

func TestCSRFRepository_ValidateAndConsume_InvalidToken(t *testing.T) {
	ctx := context.Background()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewCSRFRepository(mockDB, "test-table", zap.NewNop(), nil)

	// r.Get(...) returns invalid token when not found (err=nil, valid=false).
	mockDB.On("WithContext", ctx).Return(mockDB).Once()
	mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
	mockQuery.On("Where", "PK", "=", "CSRF#missing").Return(mockQuery).Once()
	mockQuery.On("Where", "SK", "=", models.SKToken).Return(mockQuery).Once()
	mockQuery.On("First", mock.AnythingOfType("*models.CSRFToken")).Return(dynamormErrors.ErrItemNotFound).Once()

	require.Error(t, repo.ValidateAndConsume(ctx, "missing", "user-1"))
}

func TestCSRFRepository_ValidateAndConsume_UserMismatch(t *testing.T) {
	ctx := context.Background()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewCSRFRepository(mockDB, "test-table", zap.NewNop(), nil)

	mockDB.On("WithContext", ctx).Return(mockDB).Once()
	mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
	mockQuery.On("Where", "PK", "=", "CSRF#tok-x").Return(mockQuery).Once()
	mockQuery.On("Where", "SK", "=", models.SKToken).Return(mockQuery).Once()
	mockQuery.On("First", mock.AnythingOfType("*models.CSRFToken")).Run(func(args mock.Arguments) {
		record := args.Get(0).(*models.CSRFToken)
		record.Token = "tok-x"
		record.UserID = "user-a"
		record.Used = false
		record.ExpiresAt = time.Now().Add(5 * time.Minute).Unix()
	}).Return(nil).Once()

	require.Error(t, repo.ValidateAndConsume(ctx, "tok-x", "user-b"))
}

func TestCSRFRepository_ValidateAndConsume_SuccessMarksUsed(t *testing.T) {
	ctx := context.Background()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewCSRFRepository(mockDB, "test-table", zap.NewNop(), nil)

	// 1) r.Get(...) succeeds and returns valid token.
	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)

	mockQuery.On("Where", "PK", "=", "CSRF#tok-ok").Return(mockQuery).Once()
	mockQuery.On("Where", "SK", "=", models.SKToken).Return(mockQuery).Once()
	mockQuery.On("First", mock.AnythingOfType("*models.CSRFToken")).Run(func(args mock.Arguments) {
		record := args.Get(0).(*models.CSRFToken)
		record.Token = "tok-ok"
		record.UserID = "user-1"
		record.Used = false
		record.ExpiresAt = time.Now().Add(5 * time.Minute).Unix()
	}).Return(nil).Once()

	// 2) BaseRepository.Get(...) succeeds to fetch current row for update.
	mockQuery.On("Where", "PK", "=", "CSRF#tok-ok").Return(mockQuery).Once()
	mockQuery.On("Where", "SK", "=", models.SKToken).Return(mockQuery).Once()
	mockQuery.On("First", mock.AnythingOfType("*models.CSRFToken")).Run(func(args mock.Arguments) {
		record := args.Get(0).(*models.CSRFToken)
		record.Token = "tok-ok"
		record.UserID = "user-1"
		record.Used = false
		record.ExpiresAt = time.Now().Add(5 * time.Minute).Unix()
	}).Return(nil).Once()

	mockQuery.On("Update", mock.Anything).Return(nil).Once()

	require.NoError(t, repo.ValidateAndConsume(ctx, "tok-ok", "user-1"))
}

func TestCSRFRepository_GetUserActiveTokenCount_FiltersExpiredAndUsed(t *testing.T) {
	ctx := context.Background()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewCSRFRepository(mockDB, "test-table", zap.NewNop(), nil)

	now := time.Now().Unix()

	mockDB.On("WithContext", ctx).Return(mockDB).Once()
	mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
	mockQuery.On("Index", "gsi1").Return(mockQuery).Once()
	mockQuery.On("Where", "gsi1PK", "=", "USER_CSRF#user-1").Return(mockQuery).Once()
	mockQuery.On("All", mock.AnythingOfType("*[]models.CSRFToken")).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]models.CSRFToken)
		*dest = []models.CSRFToken{
			{Token: "a", Used: false, ExpiresAt: now + 60},
			{Token: "b", Used: true, ExpiresAt: now + 60},
			{Token: "c", Used: false, ExpiresAt: now - 60},
		}
	}).Return(nil).Once()

	count, err := repo.GetUserActiveTokenCount(ctx, "user-1")
	require.NoError(t, err)
	require.Equal(t, 1, count)
}

func TestCSRFRepository_CleanupUserTokens_ContinuesOnDeleteError(t *testing.T) {
	ctx := context.Background()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewCSRFRepository(mockDB, "test-table", zap.NewNop(), nil)

	now := time.Now().Unix()

	mockDB.On("WithContext", ctx).Return(mockDB).Maybe()
	mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()

	mockQuery.On("Index", "gsi1").Return(mockQuery).Once()
	mockQuery.On("Where", "gsi1PK", "=", "USER_CSRF#user-1").Return(mockQuery).Once()
	mockQuery.On("All", mock.AnythingOfType("*[]models.CSRFToken")).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]models.CSRFToken)
		*dest = []models.CSRFToken{
			{PK: "CSRF#a", SK: models.SKToken, Token: "a", Used: true, ExpiresAt: now + 60},
			{PK: "CSRF#b", SK: models.SKToken, Token: "b", Used: false, ExpiresAt: now - 60},
		}
	}).Return(nil).Once()

	// First delete fails, second succeeds; cleanup continues.
	mockQuery.On("Where", "PK", "=", "CSRF#a").Return(mockQuery).Twice()
	mockQuery.On("Where", "SK", "=", models.SKToken).Return(mockQuery).Twice()
	mockQuery.On("First", mock.AnythingOfType("*models.CSRFToken")).Return(dynamormErrors.ErrItemNotFound).Once()
	mockQuery.On("Delete").Return(ErrTestMockError).Once()

	mockQuery.On("Where", "PK", "=", "CSRF#b").Return(mockQuery).Twice()
	mockQuery.On("Where", "SK", "=", models.SKToken).Return(mockQuery).Twice()
	mockQuery.On("First", mock.AnythingOfType("*models.CSRFToken")).Return(dynamormErrors.ErrItemNotFound).Once()
	mockQuery.On("Delete").Return(nil).Once()

	require.NoError(t, repo.CleanupUserTokens(ctx, "user-1"))
}

func TestCSRFRepository_Store_TooManyTokensAfterCleanup(t *testing.T) {
	ctx := context.Background()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewCSRFRepository(mockDB, "test-table", zap.NewNop(), nil)

	activeTokens := make([]models.CSRFToken, 10)
	for i := range activeTokens {
		activeTokens[i] = models.CSRFToken{Token: "t", Used: false, ExpiresAt: time.Now().Add(1 * time.Hour).Unix()}
	}

	allCalls := 0
	mockDB.On("WithContext", ctx).Return(mockDB).Maybe()
	mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Index", "gsi1").Return(mockQuery).Maybe()
	mockQuery.On("Where", "gsi1PK", "=", "USER_CSRF#user-1").Return(mockQuery).Maybe()
	mockQuery.On("All", mock.AnythingOfType("*[]models.CSRFToken")).Run(func(args mock.Arguments) {
		allCalls++
		dest := args.Get(0).(*[]models.CSRFToken)
		switch allCalls {
		case 1:
			*dest = activeTokens
		case 2:
			// Cleanup query; return active tokens so nothing gets deleted.
			*dest = activeTokens
		default:
			*dest = activeTokens
		}
	}).Return(nil)

	err := repo.Store(ctx, "tok-new", "user-1", time.Now().Add(5*time.Minute))
	require.Error(t, err)
}

func TestCSRFRepository_Store_DuplicateTokenReturnsAlreadyExists(t *testing.T) {
	ctx := context.Background()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewCSRFRepository(mockDB, "test-table", zap.NewNop(), nil)

	// 1) User token count below limit.
	mockDB.On("WithContext", ctx).Return(mockDB).Maybe()
	mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Index", "gsi1").Return(mockQuery).Once()
	mockQuery.On("Where", "gsi1PK", "=", "USER_CSRF#user-1").Return(mockQuery).Once()
	mockQuery.On("All", mock.AnythingOfType("*[]models.CSRFToken")).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]models.CSRFToken)
		*dest = []models.CSRFToken{}
	}).Return(nil).Once()

	// 2) ValidateAndCreate fails (DB create error).
	mockQuery.On("Create").Return(ErrTestMockError).Once()

	// 3) Duplicate check via repo.Get(...) returns valid token.
	mockQuery.On("Where", "PK", "=", "CSRF#tok-dup").Return(mockQuery).Once()
	mockQuery.On("Where", "SK", "=", models.SKToken).Return(mockQuery).Once()
	mockQuery.On("First", mock.AnythingOfType("*models.CSRFToken")).Run(func(args mock.Arguments) {
		record := args.Get(0).(*models.CSRFToken)
		record.Token = "tok-dup"
		record.UserID = "user-1"
		record.Used = false
		record.ExpiresAt = time.Now().Add(5 * time.Minute).Unix()
	}).Return(nil).Once()

	err := repo.Store(ctx, "tok-dup", "user-1", time.Now().Add(5*time.Minute))
	require.Error(t, err)

	var appErr interface{ Error() string }
	require.True(t, stdErrors.As(err, &appErr))
}

func TestCSRFRepository_CleanExpired_NoOp(t *testing.T) {
	repo := NewCSRFRepository(new(mocks.MockDB), "test-table", zap.NewNop(), nil)
	require.NoError(t, repo.CleanExpired(context.Background()))
}

func TestCSRFRepository_Store_Success_NoCleanup(t *testing.T) {
	ctx := context.Background()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewCSRFRepository(mockDB, "test-table", zap.NewNop(), nil)

	mockDB.On("WithContext", ctx).Return(mockDB).Maybe()
	mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()

	mockQuery.On("Index", "gsi1").Return(mockQuery).Once()
	mockQuery.On("Where", "gsi1PK", "=", "USER_CSRF#user-1").Return(mockQuery).Once()
	mockQuery.On("All", mock.AnythingOfType("*[]models.CSRFToken")).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]models.CSRFToken)
		*dest = []models.CSRFToken{}
	}).Return(nil).Once()

	mockQuery.On("Create").Return(nil).Once()

	require.NoError(t, repo.Store(ctx, "tok-ok", "user-1", time.Now().Add(5*time.Minute)))
}

func TestCSRFRepository_Store_CleanupErrorStillStores(t *testing.T) {
	ctx := context.Background()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewCSRFRepository(mockDB, "test-table", zap.NewNop(), nil)

	activeTokens := make([]models.CSRFToken, 10)
	for i := range activeTokens {
		activeTokens[i] = models.CSRFToken{Token: "t", Used: false, ExpiresAt: time.Now().Add(1 * time.Hour).Unix()}
	}

	mockDB.On("WithContext", ctx).Return(mockDB).Maybe()
	mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Index", "gsi1").Return(mockQuery).Maybe()
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()

	// 1) Initial active token count >= 10.
	mockQuery.On("All", mock.AnythingOfType("*[]models.CSRFToken")).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]models.CSRFToken)
		*dest = activeTokens
	}).Return(nil).Once()

	// 2) Cleanup query fails.
	mockQuery.On("All", mock.AnythingOfType("*[]models.CSRFToken")).Return(ErrTestMockError).Once()

	// 3) Post-cleanup active token count below limit.
	mockQuery.On("All", mock.AnythingOfType("*[]models.CSRFToken")).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]models.CSRFToken)
		*dest = []models.CSRFToken{}
	}).Return(nil).Once()

	// 4) Store succeeds.
	mockQuery.On("Create").Return(nil).Once()

	require.NoError(t, repo.Store(ctx, "tok-after-cleanup", "user-1", time.Now().Add(5*time.Minute)))
}

func TestCSRFRepository_Store_CreateErrorNonDuplicate(t *testing.T) {
	ctx := context.Background()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewCSRFRepository(mockDB, "test-table", zap.NewNop(), nil)

	mockDB.On("WithContext", ctx).Return(mockDB).Maybe()
	mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()

	// Token count below limit.
	mockQuery.On("Index", "gsi1").Return(mockQuery).Once()
	mockQuery.On("Where", "gsi1PK", "=", "USER_CSRF#user-1").Return(mockQuery).Once()
	mockQuery.On("All", mock.AnythingOfType("*[]models.CSRFToken")).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]models.CSRFToken)
		*dest = []models.CSRFToken{}
	}).Return(nil).Once()

	// Create fails.
	mockQuery.On("Create").Return(ErrTestMockError).Once()

	// Duplicate check -> not found -> valid=false (not a duplicate).
	mockQuery.On("Where", "PK", "=", "CSRF#tok-non-dup").Return(mockQuery).Once()
	mockQuery.On("Where", "SK", "=", models.SKToken).Return(mockQuery).Once()
	mockQuery.On("First", mock.AnythingOfType("*models.CSRFToken")).Return(dynamormErrors.ErrItemNotFound).Once()

	err := repo.Store(ctx, "tok-non-dup", "user-1", time.Now().Add(5*time.Minute))
	require.Error(t, err)
}

func TestCSRFRepository_Delete_Success(t *testing.T) {
	ctx := context.Background()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewCSRFRepository(mockDB, "test-table", zap.NewNop(), nil)

	mockDB.On("WithContext", ctx).Return(mockDB).Maybe()
	mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()

	// ValidateAndDelete does a Get (permission check) then Delete.
	mockQuery.On("Where", "PK", "=", "CSRF#tok-del-ok").Return(mockQuery).Twice()
	mockQuery.On("Where", "SK", "=", models.SKToken).Return(mockQuery).Twice()
	mockQuery.On("First", mock.AnythingOfType("*models.CSRFToken")).Return(dynamormErrors.ErrItemNotFound).Once()
	mockQuery.On("Delete").Return(nil).Once()

	require.NoError(t, repo.Delete(ctx, "tok-del-ok"))
}

func TestCSRFRepository_ValidateAndConsume_GetError(t *testing.T) {
	ctx := context.Background()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewCSRFRepository(mockDB, "test-table", zap.NewNop(), nil)

	// r.Get(...) returns an error (non-notfound).
	mockDB.On("WithContext", ctx).Return(mockDB).Once()
	mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
	mockQuery.On("Where", "PK", "=", "CSRF#tok-get-err").Return(mockQuery).Once()
	mockQuery.On("Where", "SK", "=", models.SKToken).Return(mockQuery).Once()
	mockQuery.On("First", mock.AnythingOfType("*models.CSRFToken")).Return(ErrTestMockError).Once()

	require.Error(t, repo.ValidateAndConsume(ctx, "tok-get-err", "user-1"))
}

func TestCSRFRepository_ValidateAndConsume_BaseGetNotFoundTreatsInvalid(t *testing.T) {
	ctx := context.Background()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewCSRFRepository(mockDB, "test-table", zap.NewNop(), nil)

	mockDB.On("WithContext", ctx).Return(mockDB).Maybe()
	mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()

	mockQuery.On("Where", "PK", "=", "CSRF#tok-race").Return(mockQuery).Twice()
	mockQuery.On("Where", "SK", "=", models.SKToken).Return(mockQuery).Twice()

	// r.Get(...) returns valid token.
	mockQuery.On("First", mock.AnythingOfType("*models.CSRFToken")).Run(func(args mock.Arguments) {
		record := args.Get(0).(*models.CSRFToken)
		record.Token = "tok-race"
		record.UserID = "user-1"
		record.Used = false
		record.ExpiresAt = time.Now().Add(5 * time.Minute).Unix()
	}).Return(nil).Once()

	// BaseRepository.Get(...) reports not found -> treated as invalid.
	mockQuery.On("First", mock.AnythingOfType("*models.CSRFToken")).Return(dynamormErrors.ErrItemNotFound).Once()

	require.Error(t, repo.ValidateAndConsume(ctx, "tok-race", "user-1"))
}

func TestCSRFRepository_ValidateAndConsume_UpdateError(t *testing.T) {
	ctx := context.Background()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewCSRFRepository(mockDB, "test-table", zap.NewNop(), nil)

	mockDB.On("WithContext", ctx).Return(mockDB).Maybe()
	mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()

	// r.Get(...) returns valid token.
	mockQuery.On("Where", "PK", "=", "CSRF#tok-upd-err").Return(mockQuery).Twice()
	mockQuery.On("Where", "SK", "=", models.SKToken).Return(mockQuery).Twice()
	mockQuery.On("First", mock.AnythingOfType("*models.CSRFToken")).Run(func(args mock.Arguments) {
		record := args.Get(0).(*models.CSRFToken)
		record.Token = "tok-upd-err"
		record.UserID = "user-1"
		record.Used = false
		record.ExpiresAt = time.Now().Add(5 * time.Minute).Unix()
	}).Return(nil).Once()

	// BaseRepository.Get(...) returns current record.
	mockQuery.On("First", mock.AnythingOfType("*models.CSRFToken")).Run(func(args mock.Arguments) {
		record := args.Get(0).(*models.CSRFToken)
		record.Token = "tok-upd-err"
		record.UserID = "user-1"
		record.Used = false
		record.ExpiresAt = time.Now().Add(5 * time.Minute).Unix()
	}).Return(nil).Once()

	mockQuery.On("Update", mock.Anything).Return(ErrTestMockError).Once()

	require.Error(t, repo.ValidateAndConsume(ctx, "tok-upd-err", "user-1"))
}

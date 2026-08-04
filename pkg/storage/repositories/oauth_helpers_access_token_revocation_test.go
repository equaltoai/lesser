package repositories

import (
	"context"
	stdErrors "errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	dynmormerrors "github.com/theory-cloud/tabletheory/v3/pkg/errors"
	"github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	"go.uber.org/zap"

	"github.com/equaltoai/lesser/pkg/storage/models"
)

func TestOAuthHelper_RevokeAccessTokenGeneric_EmptyJTI(t *testing.T) {
	mockDB := new(mocks.MockDB)
	logger := zap.NewNop()
	helper := NewOAuthHelper(mockDB, logger)

	err := helper.RevokeAccessTokenGeneric(context.Background(), "", time.Now().Add(time.Hour))
	require.Error(t, err)
}

func TestOAuthHelper_RevokeAccessTokenGeneric_Success(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	helper := NewOAuthHelper(mockDB, logger)

	ctx := context.Background()
	jti := "jti-123"
	expiresAt := time.Now().Add(15 * time.Minute)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.RevokedAccessToken")).Return(mockQuery)
	mockQuery.On("Create").Return(nil)

	err := helper.RevokeAccessTokenGeneric(ctx, jti, expiresAt)
	require.NoError(t, err)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestOAuthHelper_RevokeAccessTokenGeneric_IdempotentConditionalFailure(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	helper := NewOAuthHelper(mockDB, logger)

	ctx := context.Background()

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.RevokedAccessToken")).Return(mockQuery)
	mockQuery.On("Create").Return(stdErrors.New("ConditionalCheckFailed: already exists"))

	err := helper.RevokeAccessTokenGeneric(ctx, "jti-dup", time.Now().Add(time.Hour))
	require.NoError(t, err)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestOAuthHelper_RevokeAccessTokenGeneric_CreateError(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	helper := NewOAuthHelper(mockDB, logger)

	ctx := context.Background()

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.RevokedAccessToken")).Return(mockQuery)
	mockQuery.On("Create").Return(ErrTestMockError)

	err := helper.RevokeAccessTokenGeneric(ctx, "jti-err", time.Now().Add(time.Hour))
	require.Error(t, err)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestOAuthHelper_IsAccessTokenRevokedGeneric_NilHelper(t *testing.T) {
	var helper *OAuthHelper

	revoked, err := helper.IsAccessTokenRevokedGeneric(context.Background(), "jti")
	require.NoError(t, err)
	assert.False(t, revoked)
}

func TestOAuthHelper_IsAccessTokenRevokedGeneric_EmptyJTI(t *testing.T) {
	mockDB := new(mocks.MockDB)
	logger := zap.NewNop()
	helper := NewOAuthHelper(mockDB, logger)

	revoked, err := helper.IsAccessTokenRevokedGeneric(context.Background(), "")
	require.Error(t, err)
	assert.False(t, revoked)
}

func TestOAuthHelper_IsAccessTokenRevokedGeneric_NotFound(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	helper := NewOAuthHelper(mockDB, logger)

	ctx := context.Background()
	jti := "missing-jti"

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.RevokedAccessToken")).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", "REVOKEDTOKEN#"+jti).Return(mockQuery)
	mockQuery.On("Where", "SK", "=", SKToken).Return(mockQuery)
	mockQuery.On("ConsistentRead").Return(mockQuery)
	mockQuery.On("First", mock.AnythingOfType("*models.RevokedAccessToken")).Return(dynmormerrors.ErrItemNotFound)

	revoked, err := helper.IsAccessTokenRevokedGeneric(ctx, jti)
	require.NoError(t, err)
	assert.False(t, revoked)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestOAuthHelper_IsAccessTokenRevokedGeneric_FirstError(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	helper := NewOAuthHelper(mockDB, logger)

	ctx := context.Background()
	jti := "jti-read-error"

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.RevokedAccessToken")).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", "REVOKEDTOKEN#"+jti).Return(mockQuery)
	mockQuery.On("Where", "SK", "=", SKToken).Return(mockQuery)
	mockQuery.On("ConsistentRead").Return(mockQuery)
	mockQuery.On("First", mock.AnythingOfType("*models.RevokedAccessToken")).Return(ErrTestMockError)

	revoked, err := helper.IsAccessTokenRevokedGeneric(ctx, jti)
	require.Error(t, err)
	assert.False(t, revoked)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestOAuthHelper_IsAccessTokenRevokedGeneric_MalformedRecord(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	helper := NewOAuthHelper(mockDB, logger)

	ctx := context.Background()
	jti := "jti-malformed"

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.RevokedAccessToken")).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", "REVOKEDTOKEN#"+jti).Return(mockQuery)
	mockQuery.On("Where", "SK", "=", SKToken).Return(mockQuery)
	mockQuery.On("ConsistentRead").Return(mockQuery)
	mockQuery.On("First", mock.AnythingOfType("*models.RevokedAccessToken")).Run(func(args mock.Arguments) {
		model := args.Get(0).(*models.RevokedAccessToken)
		model.JTI = ""
		model.ExpiresAt = time.Now().Add(time.Hour)
	}).Return(nil)

	revoked, err := helper.IsAccessTokenRevokedGeneric(ctx, jti)
	require.NoError(t, err)
	assert.False(t, revoked)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestOAuthHelper_IsAccessTokenRevokedGeneric_ExpiredTriggersDelete(t *testing.T) {
	mockDB := new(mocks.MockDB)
	readQuery := new(mocks.MockQuery)
	deleteQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	helper := NewOAuthHelper(mockDB, logger)

	ctx := context.Background()
	jti := "jti-expired"

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.RevokedAccessToken")).Return(readQuery).Once()
	readQuery.On("Where", "PK", "=", "REVOKEDTOKEN#"+jti).Return(readQuery).Once()
	readQuery.On("Where", "SK", "=", SKToken).Return(readQuery).Once()
	readQuery.On("ConsistentRead").Return(readQuery).Once()
	readQuery.On("First", mock.AnythingOfType("*models.RevokedAccessToken")).Run(func(args mock.Arguments) {
		model := args.Get(0).(*models.RevokedAccessToken)
		model.JTI = jti
		model.ExpiresAt = time.Now().Add(-1 * time.Minute)
	}).Return(nil).Once()

	mockDB.On("Model", mock.AnythingOfType("*models.RevokedAccessToken")).Return(deleteQuery).Once()
	deleteQuery.On("Where", "PK", "=", "REVOKEDTOKEN#"+jti).Return(deleteQuery).Once()
	deleteQuery.On("Where", "SK", "=", SKToken).Return(deleteQuery).Once()
	deleteQuery.On("Delete").Return(nil).Once()

	revoked, err := helper.IsAccessTokenRevokedGeneric(ctx, jti)
	require.NoError(t, err)
	assert.False(t, revoked)

	mockDB.AssertExpectations(t)
	readQuery.AssertExpectations(t)
	deleteQuery.AssertExpectations(t)
}

func TestOAuthHelper_IsAccessTokenRevokedGeneric_ActiveRevocation(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	helper := NewOAuthHelper(mockDB, logger)

	ctx := context.Background()
	jti := "jti-active"

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.RevokedAccessToken")).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", "REVOKEDTOKEN#"+jti).Return(mockQuery)
	mockQuery.On("Where", "SK", "=", SKToken).Return(mockQuery)
	mockQuery.On("ConsistentRead").Return(mockQuery)
	mockQuery.On("First", mock.AnythingOfType("*models.RevokedAccessToken")).Run(func(args mock.Arguments) {
		model := args.Get(0).(*models.RevokedAccessToken)
		model.JTI = jti
		model.ExpiresAt = time.Now().Add(time.Hour)
	}).Return(nil)

	revoked, err := helper.IsAccessTokenRevokedGeneric(ctx, jti)
	require.NoError(t, err)
	assert.True(t, revoked)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestOAuthHelper_DeleteRevokedAccessTokenGeneric_EmptyJTI(t *testing.T) {
	mockDB := new(mocks.MockDB)
	logger := zap.NewNop()
	helper := NewOAuthHelper(mockDB, logger)

	err := helper.DeleteRevokedAccessTokenGeneric(context.Background(), "")
	require.Error(t, err)
}

func TestOAuthHelper_DeleteRevokedAccessTokenGeneric_Success(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	helper := NewOAuthHelper(mockDB, logger)

	ctx := context.Background()
	jti := "jti-delete"

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.RevokedAccessToken")).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", "REVOKEDTOKEN#"+jti).Return(mockQuery)
	mockQuery.On("Where", "SK", "=", SKToken).Return(mockQuery)
	mockQuery.On("Delete").Return(nil)

	err := helper.DeleteRevokedAccessTokenGeneric(ctx, jti)
	require.NoError(t, err)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestOAuthHelper_DeleteRevokedAccessTokenGeneric_NotFoundIgnored(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	helper := NewOAuthHelper(mockDB, logger)

	ctx := context.Background()
	jti := "jti-delete-missing"

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.RevokedAccessToken")).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", "REVOKEDTOKEN#"+jti).Return(mockQuery)
	mockQuery.On("Where", "SK", "=", SKToken).Return(mockQuery)
	mockQuery.On("Delete").Return(dynmormerrors.ErrItemNotFound)

	err := helper.DeleteRevokedAccessTokenGeneric(ctx, jti)
	require.NoError(t, err)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestOAuthHelper_DeleteRevokedAccessTokenGeneric_DeleteError(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	helper := NewOAuthHelper(mockDB, logger)

	ctx := context.Background()
	jti := "jti-delete-err"

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.RevokedAccessToken")).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", "REVOKEDTOKEN#"+jti).Return(mockQuery)
	mockQuery.On("Where", "SK", "=", SKToken).Return(mockQuery)
	mockQuery.On("Delete").Return(ErrTestMockError)

	err := helper.DeleteRevokedAccessTokenGeneric(ctx, jti)
	require.Error(t, err)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

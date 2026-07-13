package repositories

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	dynamormerrors "github.com/theory-cloud/tabletheory/v2/pkg/errors"
	"github.com/theory-cloud/tabletheory/v2/pkg/mocks"
	"go.uber.org/zap"
)

func TestRound09_OAuthHelpers_AuthorizationCodesAndRefreshTokens(t *testing.T) {
	t.Parallel()

	baseTime := time.Now().UTC()
	ctx := context.Background()

	{
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockUpdate := new(mocks.MockUpdateBuilder)
		setupPermissiveRound08Mocks(mockDB, mockQuery, mockUpdate, baseTime)

		helper := NewOAuthHelper(mockDB, zap.NewNop())
		rt := &storage.RefreshToken{
			Token:     "legacy-token",
			ClientID:  "client-1",
			Username:  "user-1",
			ExpiresAt: time.Now().Add(10 * time.Minute),
			Scopes:    []string{"read"},
		}

		require.NoError(t, helper.UpdateRefreshTokenGeneric(ctx, rt))
		require.Equal(t, 1, rt.Version)
		mockQuery.AssertCalled(t, "UpdateBuilder")
		mockUpdate.AssertCalled(t, "SetIfNotExists", "Version", nil, 0)
		mockUpdate.AssertCalled(t, "Execute")
	}

	{
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		helper := NewOAuthHelper(mockDB, zap.NewNop())

		code := &storage.AuthorizationCode{
			Code:          "code-1",
			ClientID:      "client-1",
			Username:      "user-1",
			CodeChallenge: "challenge",
			ExpiresAt:     time.Now().Add(10 * time.Minute),
			Scopes:        []string{"read"},
		}
		require.NoError(t, helper.CreateAuthorizationCodeGeneric(ctx, code))
		_, _ = helper.GetAuthorizationCodeGeneric(ctx, "code-1")
		require.NoError(t, helper.DeleteAuthorizationCodeGeneric(ctx, "code-1"))

		rt := &storage.RefreshToken{
			Token:     "token-1",
			ClientID:  "client-1",
			Username:  "user-1",
			ExpiresAt: time.Now().Add(10 * time.Minute),
			Scopes:    []string{"read"},
		}
		require.NoError(t, helper.CreateRefreshTokenGeneric(ctx, rt))
		_, _ = helper.GetRefreshTokenGeneric(ctx, "token-1")
		require.NoError(t, helper.DeleteRefreshTokenGeneric(ctx, "token-1"))
	}

	{
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("Create").Return(errors.New("ConditionalCheckFailed")).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		helper := NewOAuthHelper(mockDB, zap.NewNop())
		require.Error(t, helper.CreateAuthorizationCodeGeneric(ctx, &storage.AuthorizationCode{
			Code:      "dup",
			ClientID:  "client-1",
			Username:  "user-1",
			ExpiresAt: time.Now().Add(10 * time.Minute),
		}))
	}

	{
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("Create").Return(errors.New("ConditionalCheckFailed")).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		helper := NewOAuthHelper(mockDB, zap.NewNop())
		require.Error(t, helper.CreateRefreshTokenGeneric(ctx, &storage.RefreshToken{
			Token:     "dup",
			ClientID:  "client-1",
			Username:  "user-1",
			ExpiresAt: time.Now().Add(10 * time.Minute),
		}))
	}

	{
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("First", mock.Anything).Return(dynamormerrors.ErrItemNotFound).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		helper := NewOAuthHelper(mockDB, zap.NewNop())
		_, err := helper.GetAuthorizationCodeGeneric(ctx, "missing")
		require.Error(t, err)
	}

	{
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
			if target, ok := args.Get(0).(*models.AuthorizationCode); ok {
				target.Code = "expired"
				target.ClientID = "client-1"
				target.Username = "user-1"
				target.ExpiresAt = time.Now().Add(-1 * time.Minute)
				_ = target.BeforeCreate()
			}
		}).Return(nil).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		helper := NewOAuthHelper(mockDB, zap.NewNop())
		_, err := helper.GetAuthorizationCodeGeneric(ctx, "expired")
		require.Error(t, err)
	}

	{
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("First", mock.Anything).Return(dynamormerrors.ErrItemNotFound).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		helper := NewOAuthHelper(mockDB, zap.NewNop())
		_, err := helper.GetRefreshTokenGeneric(ctx, "missing")
		require.Error(t, err)
	}

	{
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
			if target, ok := args.Get(0).(*models.RefreshToken); ok {
				target.Token = "expired"
				target.ClientID = "client-1"
				target.Username = "user-1"
				target.ExpiresAt = time.Now().Add(-1 * time.Minute)
				_ = target.BeforeCreate()
			}
		}).Return(nil).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		helper := NewOAuthHelper(mockDB, zap.NewNop())
		_, err := helper.GetRefreshTokenGeneric(ctx, "expired")
		require.Error(t, err)
	}

	{
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("Delete", mock.Anything).Return(errors.New("boom")).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		helper := NewOAuthHelper(mockDB, zap.NewNop())
		require.Error(t, helper.DeleteAuthorizationCodeGeneric(ctx, "code-err"))
	}

	{
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("Delete", mock.Anything).Return(errors.New("boom")).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		helper := NewOAuthHelper(mockDB, zap.NewNop())
		require.Error(t, helper.DeleteRefreshTokenGeneric(ctx, "token-err"))
	}
}

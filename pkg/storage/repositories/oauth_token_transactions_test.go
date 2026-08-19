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
	tableerrors "github.com/theory-cloud/tabletheory/v3/pkg/errors"
	"github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	"go.uber.org/zap"
)

func oauthTransactionTestToken(token string, generation, version int, now time.Time) *storage.RefreshToken {
	return &storage.RefreshToken{
		Token: token, Username: "alice", ClientID: "client-1", Resource: "https://example.com/mcp/alice",
		Scopes: []string{"read"}, CreatedAt: now.Add(-time.Minute), ExpiresAt: now.Add(time.Hour),
		FamilyID: "family-1", Generation: generation, Current: true, Version: version,
	}
}

func TestOAuthTokenIssuanceTransactions(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()
	refresh := oauthTransactionTestToken("refresh-1", 1, 0, now)

	t.Run("authorization code success and input validation", func(t *testing.T) {
		repo := NewAccountRepository(&guardedDeleteTestDB{}, "test-table", "example.com", zap.NewNop())
		require.NoError(t, repo.ConsumeAuthorizationCodeAndCreateRefreshToken(ctx, "code-1", refresh))
		require.ErrorIs(t, repo.ConsumeAuthorizationCodeAndCreateRefreshToken(ctx, "", refresh), storage.ErrInvalidInput)
		require.ErrorIs(t, repo.ConsumeAuthorizationCodeAndCreateRefreshToken(ctx, "code-1", nil), storage.ErrInvalidInput)
	})

	t.Run("authorization code condition and infrastructure failures", func(t *testing.T) {
		conditionErr := &tableerrors.TransactionError{
			Err: tableerrors.ErrConditionFailed, Operation: "delete", OperationIndex: 0, Reason: "ConditionalCheckFailed",
		}
		repo := NewAccountRepository(&guardedDeleteTestDB{txErr: conditionErr}, "test-table", "example.com", zap.NewNop())
		require.ErrorIs(t, repo.ConsumeAuthorizationCodeAndCreateRefreshToken(ctx, "code-1", refresh), storage.ErrNotFound)

		infraErr := errors.New("transaction unavailable")
		repo = NewAccountRepository(&guardedDeleteTestDB{txErr: infraErr}, "test-table", "example.com", zap.NewNop())
		require.ErrorIs(t, repo.ConsumeAuthorizationCodeAndCreateRefreshToken(ctx, "code-1", refresh), infraErr)
	})

	t.Run("device session success and failures", func(t *testing.T) {
		session := &storage.OAuthDeviceSession{DeviceCodeHash: "device-hash", Status: "approved"}
		repo := NewAccountRepository(&guardedDeleteTestDB{}, "test-table", "example.com", zap.NewNop())
		require.NoError(t, repo.ConsumeOAuthDeviceSessionAndCreateRefreshToken(ctx, session, refresh, now))
		require.Equal(t, "consumed", session.Status)
		require.Equal(t, now, session.ConsumedAt)
		require.ErrorIs(t, repo.ConsumeOAuthDeviceSessionAndCreateRefreshToken(ctx, nil, refresh, now), storage.ErrInvalidInput)

		conditionErr := &tableerrors.TransactionError{
			Err: tableerrors.ErrConditionFailed, Operation: "update", OperationIndex: 0, Reason: "ConditionalCheckFailed",
		}
		repo = NewAccountRepository(&guardedDeleteTestDB{txErr: conditionErr}, "test-table", "example.com", zap.NewNop())
		session.Status = "approved"
		require.ErrorIs(t, repo.ConsumeOAuthDeviceSessionAndCreateRefreshToken(ctx, session, refresh, now), storage.ErrNotFound)

		infraErr := errors.New("device transaction unavailable")
		repo = NewAccountRepository(&guardedDeleteTestDB{txErr: infraErr}, "test-table", "example.com", zap.NewNop())
		require.ErrorIs(t, repo.ConsumeOAuthDeviceSessionAndCreateRefreshToken(ctx, session, refresh, now), infraErr)
	})
}

func TestOAuthRefreshRotationTransactions(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()

	t.Run("rotation and legacy adoption", func(t *testing.T) {
		repo := NewAccountRepository(&guardedDeleteTestDB{}, "test-table", "example.com", zap.NewNop())
		predecessor := oauthTransactionTestToken("predecessor", 1, 2, now)
		predecessor.LastUsedAt = now.Add(-time.Minute)
		successor := oauthTransactionTestToken("successor", 2, 0, now)
		require.NoError(t, repo.RotateRefreshToken(ctx, predecessor, successor, now))
		require.True(t, predecessor.Revoked)
		require.False(t, predecessor.Current)
		require.Equal(t, "rotated", predecessor.RevokedReason)
		require.Equal(t, 3, predecessor.Version)

		legacy := oauthTransactionTestToken("legacy", 0, 1, now)
		legacy.FamilyID = ""
		legacy.Current = false
		legacySuccessor := oauthTransactionTestToken("legacy-successor", 2, 0, now)
		legacySuccessor.FamilyID = "adopted-family"
		require.NoError(t, repo.RotateRefreshToken(ctx, legacy, legacySuccessor, now))
		require.Equal(t, "adopted-family", legacy.FamilyID)
		require.Equal(t, 1, legacy.Generation)
	})

	t.Run("rotation validation and transaction failure", func(t *testing.T) {
		repo := NewAccountRepository(&guardedDeleteTestDB{}, "test-table", "example.com", zap.NewNop())
		require.ErrorIs(t, repo.RotateRefreshToken(ctx, nil, nil, now), storage.ErrInvalidInput)

		infraErr := errors.New("rotation unavailable")
		repo = NewAccountRepository(&guardedDeleteTestDB{txErr: infraErr}, "test-table", "example.com", zap.NewNop())
		require.ErrorIs(t, repo.RotateRefreshToken(
			ctx,
			oauthTransactionTestToken("predecessor", 1, 1, now),
			oauthTransactionTestToken("successor", 2, 0, now),
			now,
		), infraErr)
	})

	t.Run("retry redemption success and failures", func(t *testing.T) {
		repo := NewAccountRepository(&guardedDeleteTestDB{}, "test-table", "example.com", zap.NewNop())
		stale := oauthTransactionTestToken("stale", 1, 2, now)
		stale.Current = false
		stale.Revoked = true
		active := oauthTransactionTestToken("active", 2, 3, now)
		active.LastUsedAt = now.Add(-time.Minute)
		next := oauthTransactionTestToken("next", 3, 0, now)
		require.NoError(t, repo.RedeemRefreshTokenRetry(ctx, stale, active, next, now))
		require.False(t, stale.RetryRedeemedAt.IsZero())
		require.True(t, active.Revoked)
		require.False(t, active.Current)
		require.Equal(t, "retry_rescued", active.RevokedReason)
		require.Equal(t, 3, stale.Version)
		require.Equal(t, 4, active.Version)
		require.ErrorIs(t, repo.RedeemRefreshTokenRetry(ctx, nil, active, next, now), storage.ErrInvalidInput)

		infraErr := errors.New("retry transaction unavailable")
		repo = NewAccountRepository(&guardedDeleteTestDB{txErr: infraErr}, "test-table", "example.com", zap.NewNop())
		require.ErrorIs(t, repo.RedeemRefreshTokenRetry(
			ctx,
			oauthTransactionTestToken("stale-2", 1, 1, now),
			oauthTransactionTestToken("active-2", 2, 1, now),
			oauthTransactionTestToken("next-2", 3, 0, now),
			now,
		), infraErr)
	})
}

func TestOAuthRefreshTransactionVersionAndConditionHelpers(t *testing.T) {
	ctx := context.Background()
	model := &models.RefreshToken{Token: "legacy-version"}
	require.NoError(t, model.UpdateKeys())

	repo := NewAccountRepository(&guardedDeleteTestDB{}, "test-table", "example.com", zap.NewNop())
	require.NoError(t, repo.seedRefreshTokenVersionIfNeeded(ctx, model, 1))
	require.Len(t, refreshTokenVersionConditions(7), 1)

	query := new(mocks.MockQuery)
	update := new(mocks.MockUpdateBuilder)
	query.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(query).Twice()
	query.On("UpdateBuilder").Return(update).Once()
	update.On("SetIfNotExists", "Version", nil, 0).Return(update).Once()
	update.On("Execute").Return(nil).Once()
	repo = NewAccountRepository(&guardedDeleteTestDB{query: query}, "test-table", "example.com", zap.NewNop())
	require.NoError(t, repo.seedRefreshTokenVersionIfNeeded(ctx, model, 0))

	conditionErr := &tableerrors.TransactionError{
		Err: tableerrors.ErrConditionFailed, Operation: "delete", OperationIndex: 0, Reason: "ConditionalCheckFailed",
	}
	require.True(t, transactionConditionFailedAt(conditionErr, 0))
	require.False(t, transactionConditionFailedAt(conditionErr, 1))
	require.False(t, transactionConditionFailedAt(errors.New("other"), 0))
}

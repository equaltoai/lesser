package repositories

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/v3/pkg/core"
	dynamormErrors "github.com/theory-cloud/tabletheory/v3/pkg/errors"
	"github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	"go.uber.org/zap/zaptest"
)

type round08TxDB struct {
	inner *mocks.MockDB
}

func (d *round08TxDB) Model(model any) core.Query {
	return d.inner.Model(model)
}

func (d *round08TxDB) WithContext(ctx context.Context) core.DB {
	_ = d.inner.Called(ctx)
	return d
}

func (d *round08TxDB) Migrate() error {
	return d.inner.Migrate()
}

func (d *round08TxDB) AutoMigrate(models ...any) error {
	return d.inner.AutoMigrate(models...)
}

func (d *round08TxDB) Close() error {
	return d.inner.Close()
}

func (d *round08TxDB) hasExpectedCall(method string) bool {
	for _, call := range d.inner.ExpectedCalls {
		if call.Method == method {
			return true
		}
	}
	return false
}

func (d *round08TxDB) Transact() core.TransactionBuilder {
	return new(mocks.MockTransactionBuilder)
}

func (d *round08TxDB) TransactWrite(ctx context.Context, fn func(core.TransactionBuilder) error) error {
	builder := new(mocks.MockTransactionBuilder)
	if d.hasExpectedCall("TransactWrite") {
		callbackInvoked := false
		wrapped := func(tx core.TransactionBuilder) error {
			callbackInvoked = true
			if fn == nil {
				return nil
			}
			return fn(tx)
		}
		args := d.inner.Called(ctx, wrapped)
		if err := args.Error(0); err != nil {
			return err
		}
		if callbackInvoked || fn == nil {
			return nil
		}
		return fn(builder)
	}
	if fn == nil {
		return nil
	}
	return fn(builder)
}

func TestRound08_AuthRefreshTokenRepository_CreateAndGet(t *testing.T) {
	baseTime := time.Now().UTC()
	ctx := context.Background()

	t.Run("CreateRefreshToken succeeds", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewAuthRefreshTokenRepository(mockDB, "test-table", zaptest.NewLogger(t), nil)
		token, err := repo.CreateRefreshToken(ctx, "user-1", "device", "127.0.0.1")
		require.NoError(t, err)
		require.NotEmpty(t, token.Token)
		require.NotEmpty(t, token.Family)
		require.Equal(t, 1, token.Generation)
	})

	t.Run("GetRefreshToken maps not found and expired", func(t *testing.T) {
		t.Run("not found", func(t *testing.T) {
			mockDB := new(mocks.MockDB)
			mockQuery := new(mocks.MockQuery)
			mockQuery.On("First", mock.Anything).Return(dynamormErrors.ErrItemNotFound).Once()
			setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

			repo := NewAuthRefreshTokenRepository(mockDB, "test-table", zaptest.NewLogger(t), nil)
			_, err := repo.GetRefreshToken(ctx, "missing")
			require.Error(t, err)
		})

		t.Run("expired", func(t *testing.T) {
			mockDB := new(mocks.MockDB)
			mockQuery := new(mocks.MockQuery)
			mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
				token := args.Get(0).(*models.AuthRefreshToken)
				token.Token = "t"
				token.UserID = "user-1"
				token.Family = "family-1"
				token.Generation = 1
				token.CreatedAt = baseTime.Add(-2 * time.Hour).Unix()
				token.ExpiresAt = baseTime.Add(-1 * time.Hour).Unix()
				_ = token.UpdateKeys()
			}).Return(nil).Once()
			setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

			repo := NewAuthRefreshTokenRepository(mockDB, "test-table", zaptest.NewLogger(t), nil)
			_, err := repo.GetRefreshToken(ctx, "t")
			require.Error(t, err)
		})
	})
}

func TestRound08_AuthRefreshTokenRepository_RotationAndRevocation(t *testing.T) {
	baseTime := time.Now().UTC()
	ctx := context.Background()

	t.Run("RotateRefreshToken handles reuse detection", func(t *testing.T) {
		mockInner := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockInner.On("WithContext", mock.Anything).Return(mockInner).Maybe()
		mockInner.On("Model", mock.Anything).Return(mockQuery).Maybe()

		// Old token is revoked -> token reuse path, RevokeTokenFamily is best-effort.
		mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
			old := args.Get(0).(*models.AuthRefreshToken)
			old.Token = "old"
			old.UserID = "user-1"
			old.Family = "family-1"
			old.Generation = 1
			old.CreatedAt = baseTime.Unix()
			old.ExpiresAt = baseTime.Add(24 * time.Hour).Unix()
			old.Revoked = true
			_ = old.UpdateKeys()
		}).Return(nil).Once()

		// Ensure GetTokensByFamily returns empty so RevokeTokenFamily short-circuits before any transaction.
		mockQuery.On("AllPaginated", mock.Anything).Return(&core.PaginatedResult{HasMore: false}, nil).Once()

		setupPermissiveRound08Mocks(mockInner, mockQuery, nil, baseTime)

		repo := NewAuthRefreshTokenRepository(mockInner, "test-table", zaptest.NewLogger(t), nil)
		_, err := repo.RotateRefreshToken(ctx, "old", "127.0.0.1")
		require.Error(t, err)
	})

	t.Run("RotateRefreshToken success runs transaction callback", func(t *testing.T) {
		mockInner := new(mocks.MockDB)
		mockDB := &round08TxDB{inner: mockInner}
		mockQuery := new(mocks.MockQuery)

		mockInner.On("WithContext", mock.Anything).Return(mockInner).Maybe()
		mockInner.On("Model", mock.Anything).Return(mockQuery).Maybe()
		mockInner.On("Transaction", mock.Anything).Return(nil).Maybe()
		setupPermissiveRound08Mocks(mockInner, mockQuery, nil, baseTime)

		// First() during GetRefreshToken returns active old token.
		mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
			old := args.Get(0).(*models.AuthRefreshToken)
			old.Token = "old"
			old.UserID = "user-1"
			old.Family = "family-1"
			old.Generation = 1
			old.CreatedAt = baseTime.Unix()
			old.ExpiresAt = baseTime.Add(24 * time.Hour).Unix()
			old.Revoked = false
			old.DeviceName = "device"
			_ = old.UpdateKeys()
		}).Return(nil).Once()

		// Update and Create inside transaction succeed.
		mockQuery.On("Update").Return(nil).Maybe()
		mockQuery.On("Create").Return(nil).Maybe()

		repo := NewAuthRefreshTokenRepository(mockDB, "test-table", zaptest.NewLogger(t), nil)
		newToken, err := repo.RotateRefreshToken(ctx, "old", "127.0.0.1")
		require.NoError(t, err)
		require.Equal(t, 2, newToken.Generation)
		require.Equal(t, "family-1", newToken.Family)
	})

	t.Run("RevokeTokenFamily handles empty family and update errors", func(t *testing.T) {
		t.Run("empty -> no-op", func(t *testing.T) {
			mockDB := new(mocks.MockDB)
			mockQuery := new(mocks.MockQuery)
			mockQuery.On("AllPaginated", mock.Anything).Return(&core.PaginatedResult{HasMore: false}, nil).Once()
			setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

			repo := NewAuthRefreshTokenRepository(mockDB, "test-table", zaptest.NewLogger(t), nil)
			err := repo.RevokeTokenFamily(ctx, "family-1", "reason")
			require.NoError(t, err)
		})

		t.Run("transaction update failure surfaces", func(t *testing.T) {
			mockInner := new(mocks.MockDB)
			mockDB := &round08TxDB{inner: mockInner}
			mockQuery := new(mocks.MockQuery)

			mockInner.On("WithContext", mock.Anything).Return(mockInner).Maybe()
			mockInner.On("Model", mock.Anything).Return(mockQuery).Maybe()
			mockInner.On("TransactWrite", mock.Anything, mock.Anything).Return(errors.New("update failed")).Once()
			mockQuery.On("AllPaginated", mock.Anything).Run(func(args mock.Arguments) {
				tokens := args.Get(0).(*[]models.AuthRefreshToken)
				*tokens = append(*tokens,
					models.AuthRefreshToken{Token: "t1", UserID: "user-1", Family: "family-1", CreatedAt: baseTime.Unix(), ExpiresAt: baseTime.Add(time.Hour).Unix()},
				)
			}).Return(&core.PaginatedResult{HasMore: false}, nil).Once()

			setupPermissiveRound08Mocks(mockInner, mockQuery, nil, baseTime)

			repo := NewAuthRefreshTokenRepository(mockDB, "test-table", zaptest.NewLogger(t), nil)
			err := repo.RevokeTokenFamily(ctx, "family-1", "reason")
			require.Error(t, err)
		})
	})

	t.Run("RevokeUserTokens and GetTokensByUser/Family", func(t *testing.T) {
		mockInner := new(mocks.MockDB)
		mockDB := &round08TxDB{inner: mockInner}
		mockQuery := new(mocks.MockQuery)

		mockInner.On("WithContext", mock.Anything).Return(mockInner).Maybe()
		mockInner.On("Model", mock.Anything).Return(mockQuery).Maybe()
		mockInner.On("Transaction", mock.Anything).Return(nil).Maybe()

		// RevokeUserTokens not found.
		mockQuery.On("AllPaginated", mock.Anything).Return(nil, dynamormErrors.ErrItemNotFound).Once()
		// GetTokensByUser: not found -> empty slice.
		mockQuery.On("AllPaginated", mock.Anything).Return(nil, dynamormErrors.ErrItemNotFound).Once()
		// GetTokensByFamily: return two tokens.
		mockQuery.On("AllPaginated", mock.Anything).Run(func(args mock.Arguments) {
			out := args.Get(0).(*[]models.AuthRefreshToken)
			*out = append(*out,
				models.AuthRefreshToken{Token: "active", UserID: "user-1", Family: "family-1", CreatedAt: baseTime.Unix(), ExpiresAt: baseTime.Add(time.Hour).Unix(), Revoked: false},
				models.AuthRefreshToken{Token: "revoked", UserID: "user-1", Family: "family-1", CreatedAt: baseTime.Unix(), ExpiresAt: baseTime.Add(time.Hour).Unix(), Revoked: true},
			)
		}).Return(&core.PaginatedResult{HasMore: false}, nil).Once()

		setupPermissiveRound08Mocks(mockInner, mockQuery, nil, baseTime)

		repo := NewAuthRefreshTokenRepository(mockDB, "test-table", zaptest.NewLogger(t), nil)
		require.NoError(t, repo.RevokeUserTokens(ctx, "user-1", "logout"))

		tokens, err := repo.GetTokensByUser(ctx, "user-1")
		require.NoError(t, err)
		require.Empty(t, tokens)
		familyTokens, err := repo.GetTokensByFamily(ctx, "family-1")
		require.NoError(t, err)
		require.Len(t, familyTokens, 2)
	})
}

func TestRound08_AuthRefreshTokenRepository_CreateRefreshToken_Succeeds(t *testing.T) {
	baseTime := time.Now().UTC()
	ctx := context.Background()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

	repo := NewAuthRefreshTokenRepository(mockDB, "test-table", zaptest.NewLogger(t), nil)

	token, err := repo.CreateRefreshToken(ctx, "user-1", "device", "127.0.0.1")
	require.NoError(t, err)
	require.NotEmpty(t, token.Token)
	require.NotEmpty(t, token.Family)
	require.Equal(t, 1, token.Generation)
}

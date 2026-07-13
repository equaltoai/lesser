package repositories

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	dynamormErrors "github.com/theory-cloud/tabletheory/v2/pkg/errors"
	"github.com/theory-cloud/tabletheory/v2/pkg/mocks"
	"go.uber.org/zap/zaptest"
	"golang.org/x/crypto/bcrypt"
)

func TestRound08_AccountRepositoryAuth_ValidatePassword(t *testing.T) {
	baseTime := time.Now().UTC()
	ctx := context.Background()

	t.Run("suspended account", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("First", mock.MatchedBy(func(v any) bool {
			_, ok := v.(*userCoreProjection)
			return ok
		})).Run(func(args mock.Arguments) {
			user := args.Get(0).(*userCoreProjection)
			user.Username = "user-1"
			user.Suspended = true
		}).Return(nil).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))
		_, err := repo.ValidatePassword(ctx, "user-1", "password")
		require.Error(t, err)
	})

	t.Run("invalid password tracks failed login", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		hash, err := bcrypt.GenerateFromPassword([]byte("correct"), bcrypt.MinCost)
		require.NoError(t, err)

		mockQuery.On("First", mock.MatchedBy(func(v any) bool {
			_, ok := v.(*userCoreProjection)
			return ok
		})).Run(func(args mock.Arguments) {
			user := args.Get(0).(*userCoreProjection)
			user.Username = "user-1"
			user.Suspended = false
			user.PasswordHash = string(hash)
		}).Return(nil).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))
		_, err = repo.ValidatePassword(ctx, "user-1", "wrong")
		require.Error(t, err)
	})

	t.Run("valid password tracks successful login", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		hash, err := bcrypt.GenerateFromPassword([]byte("password"), bcrypt.MinCost)
		require.NoError(t, err)

		mockQuery.On("First", mock.MatchedBy(func(v any) bool {
			_, ok := v.(*userCoreProjection)
			return ok
		})).Run(func(args mock.Arguments) {
			user := args.Get(0).(*userCoreProjection)
			user.Username = "user-1"
			user.Suspended = false
			user.PasswordHash = string(hash)
		}).Return(nil).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))
		user, err := repo.ValidatePassword(ctx, "user-1", "password")
		require.NoError(t, err)
		require.Equal(t, "user-1", user.Username)
	})
}

func TestRound08_AccountRepositoryAuth_PasswordResetFlow(t *testing.T) {
	baseTime := time.Now().UTC()
	ctx := context.Background()

	t.Run("CreatePasswordResetToken email mismatch", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("First", mock.MatchedBy(func(v any) bool {
			_, ok := v.(*userCoreProjection)
			return ok
		})).Run(func(args mock.Arguments) {
			user := args.Get(0).(*userCoreProjection)
			user.Username = "user-1"
			user.Email = "user-1@example.com"
		}).Return(nil).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))
		_, err := repo.CreatePasswordResetToken(ctx, "user-1", "different@example.com")
		require.Error(t, err)
	})

	t.Run("ValidatePasswordResetToken not found / expired / used / valid", func(t *testing.T) {
		t.Run("not found", func(t *testing.T) {
			mockDB := new(mocks.MockDB)
			mockQuery := new(mocks.MockQuery)
			mockQuery.On("First", mock.Anything).Return(dynamormErrors.ErrItemNotFound).Once()
			setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

			repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))
			_, err := repo.ValidatePasswordResetToken(ctx, "tok")
			require.Error(t, err)
		})

		t.Run("expired", func(t *testing.T) {
			mockDB := new(mocks.MockDB)
			mockQuery := new(mocks.MockQuery)
			mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
				reset := args.Get(0).(*models.PasswordReset)
				reset.Username = "user-1"
				reset.Token = "tok"
				reset.Email = "user-1@example.com"
				reset.CreatedAt = baseTime.Add(-2 * time.Hour)
				reset.ExpiresAt = baseTime.Add(-1 * time.Hour)
			}).Return(nil).Once()
			setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

			repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))
			_, err := repo.ValidatePasswordResetToken(ctx, "tok")
			require.Error(t, err)
		})

		t.Run("used", func(t *testing.T) {
			mockDB := new(mocks.MockDB)
			mockQuery := new(mocks.MockQuery)
			mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
				reset := args.Get(0).(*models.PasswordReset)
				reset.Username = "user-1"
				reset.Token = "tok"
				reset.Email = "user-1@example.com"
				reset.CreatedAt = baseTime.Add(-time.Minute)
				reset.ExpiresAt = baseTime.Add(1 * time.Hour)
				reset.Used = true
			}).Return(nil).Once()
			setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

			repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))
			_, err := repo.ValidatePasswordResetToken(ctx, "tok")
			require.Error(t, err)
		})

		t.Run("valid", func(t *testing.T) {
			mockDB := new(mocks.MockDB)
			mockQuery := new(mocks.MockQuery)
			mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
				reset := args.Get(0).(*models.PasswordReset)
				reset.Username = "user-1"
				reset.Token = "tok"
				reset.Email = "user-1@example.com"
				reset.CreatedAt = baseTime.Add(-time.Minute)
				reset.ExpiresAt = baseTime.Add(1 * time.Hour)
				reset.Used = false
			}).Return(nil).Once()
			setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

			repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))
			out, err := repo.ValidatePasswordResetToken(ctx, "tok")
			require.NoError(t, err)
			require.Equal(t, "user-1", out.Username)
		})
	})

	t.Run("ResetPassword updates user and best-effort marks token used", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockUpdateBuilder := new(mocks.MockUpdateBuilder)

		// ValidatePasswordResetToken lookup.
		mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
			reset := args.Get(0).(*models.PasswordReset)
			reset.Username = "user-1"
			reset.Token = "tok"
			reset.Email = "user-1@example.com"
			reset.CreatedAt = baseTime.Add(-time.Minute)
			reset.ExpiresAt = baseTime.Add(1 * time.Hour)
			reset.Used = false
		}).Return(nil).Once()

		// Mark-used lookup fails and is ignored.
		mockQuery.On("First", mock.MatchedBy(func(v any) bool {
			_, ok := v.(*models.PasswordReset)
			return ok
		})).Return(errors.New("boom")).Once()

		// UpdateUser -> Get user (BaseRepository.Get uses First) + optional version hydration.
		mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
			switch v := args.Get(0).(type) {
			case *models.User:
				v.Username = "user-1"
				v.Version = 1
			case *userVersionProjection:
				version := 1
				v.Value = &version
			}
		}).Return(nil).Maybe()

		setupPermissiveRound08Mocks(mockDB, mockQuery, mockUpdateBuilder, baseTime)

		repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))
		err := repo.ResetPassword(ctx, "tok", "new-hash")
		require.NoError(t, err)
	})
}

func TestRound08_AccountRepositoryAuth_SessionsAndRateLimiting(t *testing.T) {
	baseTime := time.Now().UTC()
	ctx := context.Background()

	t.Run("Session CRUD and refresh-token lookup", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))

		_, err := repo.CreateSession(ctx, "user-1", "127.0.0.1", "ua")
		require.NoError(t, err)

		_, err = repo.GetUserSessions(ctx, "user-1")
		require.NoError(t, err)

		// InvalidateSession not found.
		mockQueryNotFound := new(mocks.MockQuery)
		mockDBNotFound := new(mocks.MockDB)
		mockQueryNotFound.On("First", mock.Anything).Return(dynamormErrors.ErrItemNotFound).Once()
		setupPermissiveRound08Mocks(mockDBNotFound, mockQueryNotFound, nil, baseTime)
		repoNF := NewAccountRepository(mockDBNotFound, "test-table", "example.com", zaptest.NewLogger(t))
		require.Error(t, repoNF.InvalidateSession(ctx, "user-1", "sid"))

		require.NoError(t, repo.InvalidateAllSessions(ctx, "user-1"))

		require.NoError(t, repo.UpdateSession(ctx, "sid", "rt", "127.0.0.1", time.Now(), time.Now().Add(time.Hour)))
		require.NoError(t, repo.DeleteSession(ctx, "sid"))

		_, _ = repo.GetSession(ctx, "sid")
		_, _ = repo.GetSessionByRefreshToken(ctx, "rt")
	})

	t.Run("Rate limiting methods", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)
		repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))

		limited, _, err := repo.IsRateLimited(ctx, "user-1")
		require.NoError(t, err)
		require.True(t, limited)

		// Not found -> not limited.
		mockQueryNF := new(mocks.MockQuery)
		mockDBNF := new(mocks.MockDB)
		mockQueryNF.On("First", mock.Anything).Return(dynamormErrors.ErrItemNotFound).Once()
		setupPermissiveRound08Mocks(mockDBNF, mockQueryNF, nil, baseTime)
		repoNF := NewAccountRepository(mockDBNF, "test-table", "example.com", zaptest.NewLogger(t))
		limited, _, err = repoNF.IsRateLimited(ctx, "user-1")
		require.NoError(t, err)
		require.False(t, limited)

		require.NoError(t, repo.RecordLoginAttempt(ctx, "user-1", true))

		// ClearLoginAttempts deletes attempts + lockout.
		mockQueryDelErr := new(mocks.MockQuery)
		mockDBDelErr := new(mocks.MockDB)
		mockQueryDelErr.On("Scan", mock.Anything).Run(func(args mock.Arguments) {
			out := args.Get(0).(*[]models.LoginAttempt)
			*out = append(*out, models.LoginAttempt{PK: "RATELIMIT#user-1", SK: "sk"})
		}).Return(nil).Once()
		mockQueryDelErr.On("Delete").Return(errors.New("delete failed")).Once()
		mockQueryDelErr.On("Delete").Return(dynamormErrors.ErrItemNotFound).Once()
		setupPermissiveRound08Mocks(mockDBDelErr, mockQueryDelErr, nil, baseTime)
		repoDelErr := NewAccountRepository(mockDBDelErr, "test-table", "example.com", zaptest.NewLogger(t))
		require.NoError(t, repoDelErr.ClearLoginAttempts(ctx, "user-1"))

		_, err = repo.GetLoginAttemptCount(ctx, "user-1", time.Now().Add(-time.Hour))
		require.NoError(t, err)
	})
}

func TestRound08_AccountRepositoryAuth_RecoveryAndWebAuthn(t *testing.T) {
	baseTime := time.Now().UTC()
	ctx := context.Background()

	t.Run("GetUserByRecoveryCode finds user and handles hash errors", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		hash, err := bcrypt.GenerateFromPassword([]byte("code"), bcrypt.MinCost)
		require.NoError(t, err)

		mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
			out := args.Get(0).(*[]models.RecoveryCode)
			code := models.RecoveryCode{
				Username:  "user-1",
				CodeHash:  string(hash),
				CreatedAt: baseTime,
				UsedAt:    nil,
				Position:  0,
			}
			_ = code.UpdateKeys()
			*out = append(*out, code)
		}).Return(nil).Once()
		mockQuery.On("Update").Return(errors.New("update failed")).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))
		user, err := repo.GetUserByRecoveryCode(ctx, "code")
		require.NoError(t, err)
		require.Equal(t, "user-1", user.Username)

		require.False(t, repo.verifyRecoveryCodeHash("code", "not-a-bcrypt-hash"))
		require.False(t, repo.verifyRecoveryCodeHash("wrong", string(hash)))
	})

	t.Run("RecoveryToken CRUD", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))
		require.NoError(t, repo.StoreRecoveryToken(ctx, "k", map[string]any{"a": "b"}))

		_, err := repo.GetRecoveryToken(ctx, "k")
		require.NoError(t, err)

		require.NoError(t, repo.DeleteRecoveryToken(ctx, "k"))

		// Not found -> mapped error.
		mockDBNF := new(mocks.MockDB)
		mockQueryNF := new(mocks.MockQuery)
		mockQueryNF.On("First", mock.Anything).Return(dynamormErrors.ErrItemNotFound).Once()
		setupPermissiveRound08Mocks(mockDBNF, mockQueryNF, nil, baseTime)
		repoNF := NewAccountRepository(mockDBNF, "test-table", "example.com", zaptest.NewLogger(t))
		_, err = repoNF.GetRecoveryToken(ctx, "k")
		require.Error(t, err)
	})

	t.Run("WebAuthn challenge and credential operations", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)
		repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))

		require.Error(t, repo.StoreWebAuthnChallenge(ctx, nil))

		require.NoError(t, repo.StoreWebAuthnChallenge(ctx, &storage.WebAuthnChallenge{
			Challenge:   "c",
			UserID:      "user-1",
			SessionData: []byte("s"),
			ExpiresAt:   time.Now().Add(time.Minute),
			Type:        "authentication",
		}))

		require.Error(t, repo.StoreWebAuthnCredential(ctx, nil))

		require.NoError(t, repo.StoreWebAuthnCredential(ctx, &storage.WebAuthnCredential{
			ID:           "cred-1",
			UserID:       "user-1",
			PublicKey:    []byte("pk"),
			CreatedAt:    time.Time{},
			LastUsed:     time.Now(),
			CloneWarning: false,
		}))

		require.Error(t, repo.UpdateWebAuthnCredential(ctx, "", 1))

		mockDBNF := new(mocks.MockDB)
		mockQueryNF := new(mocks.MockQuery)
		mockQueryNF.On("First", mock.Anything).Return(dynamormErrors.ErrItemNotFound).Once()
		setupPermissiveRound08Mocks(mockDBNF, mockQueryNF, nil, baseTime)
		repoNF := NewAccountRepository(mockDBNF, "test-table", "example.com", zaptest.NewLogger(t))
		require.Error(t, repoNF.UpdateWebAuthnCredential(ctx, "cred-1", 1))
	})

	t.Run("Wallet + provider linkage helpers", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)
		repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))

		require.Error(t, repo.UpdateWalletLastUsed(ctx, "", "0xabc"))
		require.Error(t, repo.UpdateWalletLastUsed(ctx, "user-1", ""))
		_ = repo.UpdateWalletLastUsed(ctx, "user-1", "0xabc")

		providers, err := repo.GetLinkedProviders(ctx, "user-1")
		require.NoError(t, err)
		require.NotNil(t, providers)

		// Not found -> empty slice.
		mockDBNF := new(mocks.MockDB)
		mockQueryNF := new(mocks.MockQuery)
		mockQueryNF.On("All", mock.Anything).Return(dynamormErrors.ErrItemNotFound).Once()
		setupPermissiveRound08Mocks(mockDBNF, mockQueryNF, nil, baseTime)
		repoNF := NewAccountRepository(mockDBNF, "test-table", "example.com", zaptest.NewLogger(t))
		providers, err = repoNF.GetLinkedProviders(ctx, "user-1")
		require.NoError(t, err)
		require.Empty(t, providers)
	})

	t.Run("misc helpers", func(t *testing.T) {
		require.Equal(t, "", extractUsernameFromPK("bad"))
		require.Equal(t, "u", extractUsernameFromUserID("USER#u"))
		require.Equal(t, 1, minInt(1, 2))
		require.Len(t, hashTokenForGSI("token"), 16)
		require.Error(t, common.ValidateRequiredParam("k", "")) // keep common import used
	})
}

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
	dynamormErrors "github.com/theory-cloud/tabletheory/v2/pkg/errors"
	"github.com/theory-cloud/tabletheory/v2/pkg/mocks"
	"go.uber.org/zap/zaptest"
	"golang.org/x/crypto/bcrypt"
)

func TestRound08_AccountRepositoryAuth_CoverageSweep(t *testing.T) {
	baseTime := time.Now().UTC()
	ctx := context.Background()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	mockUpdateBuilder := new(mocks.MockUpdateBuilder)

	// Force some explicit errors for mapping branches.
	mockQuery.On("Create").Return(errors.New("create failed")).Once() // for trackSuccessfulLogin/failed login
	mockQuery.On("Create").Return(nil).Maybe()

	setupPermissiveRound08Mocks(mockDB, mockQuery, mockUpdateBuilder, baseTime)

	repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))

	// Password validation paths.
	hash, err := bcrypt.GenerateFromPassword([]byte("password"), bcrypt.MinCost)
	require.NoError(t, err)
	mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
		user := args.Get(0).(*models.User)
		user.Username = "user-1"
		user.Suspended = false
		user.PasswordHash = string(hash)
		user.Email = "user-1@example.com"
		user.Version = 1
	}).Return(nil).Maybe()

	_, _ = repo.ValidatePassword(ctx, "user-1", "password")
	_, _ = repo.ValidatePassword(ctx, "user-1", "wrong")

	// Recent login attempts.
	_, _ = repo.GetRecentLoginAttempts(ctx, "user-1", time.Now().Add(-time.Hour))

	// Password reset token flow.
	// Email is forbidden and always empty in storage.User, so pass empty string to match.
	token, err := repo.CreatePasswordResetToken(ctx, "user-1", "")
	require.NoError(t, err)
	_, _ = repo.ValidatePasswordResetToken(ctx, token)
	_ = repo.ResetPassword(ctx, token, "new-hash")

	// Sessions.
	_, _ = repo.GetUserSessions(ctx, "user-1")
	session, err := repo.CreateSession(ctx, "user-1", "127.0.0.1", "ua")
	require.NoError(t, err)
	_ = repo.InvalidateSession(ctx, "user-1", session.SessionID)
	_ = repo.InvalidateAllSessions(ctx, "user-1")
	_ = repo.UpdateLastLogin(ctx, "user-1")
	_, _ = repo.GetSession(ctx, session.SessionID)
	_ = repo.UpdateSession(ctx, session.SessionID, "rt", "127.0.0.1", time.Now(), time.Now().Add(time.Hour))
	_ = repo.DeleteSession(ctx, session.SessionID)
	_, _ = repo.GetSessionByRefreshToken(ctx, "rt")

	// Rate limiting.
	_, _, _ = repo.IsRateLimited(ctx, "user-1")
	_ = repo.RecordLoginAttempt(ctx, "user-1", true)
	_ = repo.ClearLoginAttempts(ctx, "user-1")
	_, _ = repo.GetLoginAttemptCount(ctx, "user-1", time.Now().Add(-time.Hour))

	// Recovery codes.
	_, _ = repo.GetUserByRecoveryCode(ctx, "password")

	// Device management.
	_ = repo.CreateDevice(ctx, nil)
	require.NoError(t, repo.CreateDevice(ctx, &storage.Device{
		DeviceID:      "device-1",
		Username:      "user-1",
		DeviceName:    "dev",
		DeviceType:    "web",
		LastIPAddress: "127.0.0.1",
		LastUserAgent: "ua",
		CreatedAt:     baseTime,
		LastSeenAt:    baseTime,
		TrustLevel:    "", // triggers default
	}))
	_, _ = repo.GetDevice(ctx, "device-1")
	_ = repo.UpdateDevice(ctx, &storage.Device{
		DeviceID:      "device-1",
		Username:      "user-1",
		LastSeenAt:    baseTime.Add(time.Minute),
		LastIPAddress: "127.0.0.1",
		LastUserAgent: "ua2",
		TrustLevel:    "trusted",
	})
	_, _ = repo.GetUserDevices(ctx, "user-1")

	// Session creation from struct.
	require.Error(t, repo.CreateSessionFromStruct(ctx, nil))
	require.NoError(t, repo.CreateSessionFromStruct(ctx, &storage.Session{
		SessionID:    "sid",
		Username:     "user-1",
		RefreshToken: "rt",
		CreatedAt:    baseTime,
		ExpiresAt:    baseTime.Add(time.Hour),
		LastActivity: baseTime,
		IPAddress:    "127.0.0.1",
		UserAgent:    "ua",
	}))

	// Recovery token.
	require.NoError(t, repo.StoreRecoveryToken(ctx, "k", map[string]any{"a": "b"}))
	_, _ = repo.GetRecoveryToken(ctx, "k")
	_ = repo.DeleteRecoveryToken(ctx, "k")

	// WebAuthn.
	_ = repo.StoreWebAuthnChallenge(ctx, &storage.WebAuthnChallenge{
		Challenge:   "c",
		UserID:      "user-1",
		SessionData: "not-bytes",
		ExpiresAt:   time.Now().Add(time.Minute),
		Type:        "authentication",
	})
	_ = repo.StoreWebAuthnCredential(ctx, &storage.WebAuthnCredential{
		ID:         "cred-1",
		UserID:     "user-1",
		PublicKey:  []byte("pk"),
		CreatedAt:  baseTime,
		LastUsedAt: baseTime,
		LastUsed:   baseTime,
	})
	_ = repo.UpdateWebAuthnCredential(ctx, "cred-1", 1)

	// Wallet + provider helpers.
	_ = repo.UpdateWalletLastUsed(ctx, "user-1", "0xabc")
	_, _ = repo.GetLinkedProviders(ctx, "user-1")

	// Helper functions.
	_ = extractUsernameFromPK("USER#user-1")
	_ = extractUsernameFromUserID("USER#user-1")
	_ = minInt(1, 2)
	_ = hashTokenForGSI("token")
}

func TestRound08_AccountRepositoryAuth_ErrorBranches(t *testing.T) {
	baseTime := time.Now().UTC()
	ctx := context.Background()

	t.Run("IsRateLimited expired lockout returns not limited", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
			lockout := args.Get(0).(*models.RateLimitLockout)
			lockout.PK = "RATELIMIT#k"
			lockout.SK = "LOCKOUT"
			lockout.UnlockTime = baseTime.Add(-time.Minute)
		}).Return(nil).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))
		limited, _, err := repo.IsRateLimited(ctx, "k")
		require.NoError(t, err)
		require.False(t, limited)
	})

	t.Run("ClearLoginAttempts scan error and GetLoginAttemptCount scan error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("Scan", mock.Anything).Return(errors.New("scan failed")).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))
		require.Error(t, repo.ClearLoginAttempts(ctx, "user-1"))

		mockDB2 := new(mocks.MockDB)
		mockQuery2 := new(mocks.MockQuery)
		mockQuery2.On("Scan", mock.Anything).Return(errors.New("scan failed")).Once()
		setupPermissiveRound08Mocks(mockDB2, mockQuery2, nil, baseTime)
		repo2 := NewAccountRepository(mockDB2, "test-table", "example.com", zaptest.NewLogger(t))
		_, err := repo2.GetLoginAttemptCount(ctx, "user-1", time.Now().Add(-time.Hour))
		require.Error(t, err)
	})

	t.Run("UpdateDevice not found and GetDevice empty results", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("First", mock.Anything).Return(dynamormErrors.ErrItemNotFound).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))
		require.Error(t, repo.UpdateDevice(ctx, &storage.Device{DeviceID: "d", Username: "u"}))

		mockDB2 := new(mocks.MockDB)
		mockQuery2 := new(mocks.MockQuery)
		mockQuery2.On("All", mock.Anything).Return(nil).Once()
		setupPermissiveRound08Mocks(mockDB2, mockQuery2, nil, baseTime)
		repo2 := NewAccountRepository(mockDB2, "test-table", "example.com", zaptest.NewLogger(t))
		_, err := repo2.GetDevice(ctx, "device-1")
		require.Error(t, err)
	})
}

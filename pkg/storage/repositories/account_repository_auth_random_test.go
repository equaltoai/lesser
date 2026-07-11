package repositories

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/v2/pkg/mocks"
	"go.uber.org/zap/zaptest"
	"golang.org/x/crypto/bcrypt"
)

func TestAccountRepositoryAuthRandomIdentifiers(t *testing.T) {
	t.Run("session IDs use cryptographic random bytes", func(t *testing.T) {
		first, err := generateSessionID()
		require.NoError(t, err)
		second, err := generateSessionID()
		require.NoError(t, err)

		require.Regexp(t, `^session_[0-9a-f]{64}$`, first)
		require.Regexp(t, `^session_[0-9a-f]{64}$`, second)
		require.NotEqual(t, first, second)
	})

	t.Run("secure tokens use cryptographic random bytes", func(t *testing.T) {
		first, err := generateSecureToken()
		require.NoError(t, err)
		second, err := generateSecureToken()
		require.NoError(t, err)

		require.Regexp(t, `^[0-9a-f]{64}$`, first)
		require.Regexp(t, `^[0-9a-f]{64}$`, second)
		require.NotEqual(t, first, second)
	})
}

func TestAccountRepositoryAuthRandomIdentifiersFailClosed(t *testing.T) {
	originalReader := accountAuthRandRead
	accountAuthRandRead = func([]byte) (int, error) {
		return 0, errors.New("random unavailable")
	}
	defer func() { accountAuthRandRead = originalReader }()

	sessionID, err := generateSessionID()
	require.Error(t, err)
	require.Empty(t, sessionID)

	token, err := generateSecureToken()
	require.Error(t, err)
	require.Empty(t, token)
}

func TestAccountRepositoryCreateSessionFailsClosedOnRandomErrors(t *testing.T) {
	originalReader := accountAuthRandRead
	defer func() { accountAuthRandRead = originalReader }()

	repo := NewAccountRepository(nil, "test-table", "example.com", zaptest.NewLogger(t))

	t.Run("session ID generation failure", func(t *testing.T) {
		accountAuthRandRead = func([]byte) (int, error) {
			return 0, errors.New("random unavailable")
		}

		session, err := repo.CreateSession(context.Background(), "alice", "192.0.2.10", "curl/8.0")
		require.Error(t, err)
		require.Nil(t, session)
	})

	t.Run("session token generation failure", func(t *testing.T) {
		readCount := 0
		accountAuthRandRead = func(b []byte) (int, error) {
			readCount++
			if readCount == 1 {
				for i := range b {
					b[i] = byte(i + 1)
				}
				return len(b), nil
			}
			return 0, errors.New("random unavailable")
		}

		session, err := repo.CreateSession(context.Background(), "alice", "192.0.2.10", "curl/8.0")
		require.Error(t, err)
		require.Nil(t, session)
		require.Equal(t, 2, readCount)
	})
}

func TestAccountRepositoryPasswordResetTokenFailsClosedOnRandomError(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	mockUpdateBuilder := new(mocks.MockUpdateBuilder)
	setupPermissiveRound08Mocks(mockDB, mockQuery, mockUpdateBuilder, time.Now().UTC())

	originalReader := accountAuthRandRead
	accountAuthRandRead = func([]byte) (int, error) {
		return 0, errors.New("random unavailable")
	}
	defer func() { accountAuthRandRead = originalReader }()

	repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))
	token, err := repo.CreatePasswordResetToken(context.Background(), "user-1", "")
	require.Error(t, err)
	require.Empty(t, token)
}

func TestAccountRepositoryAuthSmallHelpers(t *testing.T) {
	now := time.Now().UTC()
	require.True(t, derefTime(nil).IsZero())
	require.Equal(t, now, derefTime(&now))

	require.Equal(t, 1, minInt(1, 2))
	require.Equal(t, 2, minInt(3, 2))

	hashed := hashTokenForGSI("refresh-token")
	require.Len(t, hashed, 16)
	require.NotContains(t, hashed, "refresh-token")

	require.Equal(t, "alice", extractUsernameFromPK("USER#alice"))
	require.Empty(t, extractUsernameFromPK("alice"))
	require.Equal(t, "bob", extractUsernameFromUserID("USER#bob"))

	passwordHash, err := bcrypt.GenerateFromPassword([]byte("recovery-code"), bcrypt.MinCost)
	require.NoError(t, err)
	repo := NewAccountRepository(nil, "test-table", "example.com", zaptest.NewLogger(t))
	require.True(t, repo.verifyRecoveryCodeHash("recovery-code", string(passwordHash)))
	require.False(t, repo.verifyRecoveryCodeHash("wrong-code", string(passwordHash)))
}

func TestAccountRepositoryProjectionTableNames(t *testing.T) {
	require.Equal(t, "custom-table", userCoreProjection{Table: "custom-table"}.TableName())
	require.Equal(t, "custom-table", userMetadataProjection{Table: "custom-table"}.TableName())
	require.Equal(t, "custom-table", userVersionProjection{Table: "custom-table"}.TableName())

	require.NotEmpty(t, userCoreProjection{}.TableName())
	require.NotEmpty(t, userMetadataProjection{}.TableName())
	require.NotEmpty(t, userVersionProjection{}.TableName())
}

package auth

import (
	"crypto/rand"
	"crypto/rsa"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	"go.uber.org/zap"
)

type fakeAuthRepos struct {
	account *repositories.AccountRepository
}

func (f fakeAuthRepos) Account() *repositories.AccountRepository        { return f.account }
func (f fakeAuthRepos) Actor() interfaces.ActorRepository               { return nil }
func (f fakeAuthRepos) Activity() interfaces.ActivityRepository         { return nil }
func (f fakeAuthRepos) Notification() interfaces.NotificationRepository { return nil }
func (f fakeAuthRepos) Recovery() *repositories.RecoveryRepository      { return nil }
func (f fakeAuthRepos) Audit() *repositories.AuditRepository            { return nil }

func TestValidateJWTSecretStrength(t *testing.T) {
	assert.Error(t, validateJWTSecretStrength("short"))
	assert.Error(t, validateJWTSecretStrength("this-is-a-long-but-default-secret-should-fail-12345"))
	assert.NoError(t, validateJWTSecretStrength("a-very-strong-jwt-key-without-weak-patterns-9876543210"))
}

func TestAuthService_generateShortLivedAccessToken_AndValidateAccessToken(t *testing.T) {
	secret := "a-very-strong-jwt-key-without-weak-patterns-9876543210"

	// Mock dynamorm DB for AccountRepository.GetSession.
	db := new(mocks.MockDB)
	q := new(mocks.MockQuery)
	db.On("WithContext", mock.Anything).Return(db)
	db.On("Model", mock.Anything).Return(q)
	q.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(q)

	sessionID := "sid-123"
	q.On("First", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*models.Session)
		*dest = models.Session{
			SessionID:    sessionID,
			UserID:       "USER#alice",
			RefreshToken: "rt",
			CreatedAt:    time.Now(),
			LastUsedAt:   time.Now(),
			UpdatedAt:    time.Now(),
			ExpiresAt:    time.Now().Add(1 * time.Hour).Unix(),
			UserAgent:    "ua",
			IPAddress:    "ip",
			DeviceID:     "dev-1",
		}
	})

	accountRepo := repositories.NewAccountRepository(db, "test-table", "example.com", zap.NewNop())
	as := &AuthService{
		repos:     fakeAuthRepos{account: accountRepo},
		jwtSecret: []byte(secret),
	}

	token, err := as.generateShortLivedAccessToken("alice", sessionID, "dev-1", DefaultScopes())
	assert.NoError(t, err)
	assert.NotEmpty(t, token)

	claims, err := as.ValidateAccessToken(token)
	assert.NoError(t, err)
	assert.Equal(t, "alice", claims.Username)
	assert.Equal(t, sessionID, claims.SessionID)
	assert.Equal(t, "dev-1", claims.DeviceID)

	db.AssertExpectations(t)
	q.AssertExpectations(t)
}

func TestAuthService_ValidateAccessTokenRejectsRevokedSession(t *testing.T) {
	secret := "a-very-strong-jwt-key-without-weak-patterns-9876543210"

	db := new(mocks.MockDB)
	q := new(mocks.MockQuery)
	db.On("WithContext", mock.Anything).Return(db)
	db.On("Model", mock.Anything).Return(q)
	q.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(q)

	sessionID := "sid-revoked"
	q.On("First", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*models.Session)
		*dest = models.Session{
			SessionID:  sessionID,
			UserID:     "USER#alice",
			CreatedAt:  time.Now(),
			LastUsedAt: time.Now(),
			UpdatedAt:  time.Now(),
			ExpiresAt:  time.Now().Add(1 * time.Hour).Unix(),
			IsRevoked:  true,
		}
	})

	accountRepo := repositories.NewAccountRepository(db, "test-table", "example.com", zap.NewNop())
	as := &AuthService{
		repos:     fakeAuthRepos{account: accountRepo},
		jwtSecret: []byte(secret),
	}

	token, err := as.generateShortLivedAccessToken("alice", sessionID, "dev-1", DefaultScopes())
	assert.NoError(t, err)

	_, err = as.ValidateAccessToken(token)
	assert.ErrorIs(t, err, ErrInvalidToken)
}

func TestAuthService_ValidateAccessToken_RejectsNonHMAC(t *testing.T) {
	as := &AuthService{jwtSecret: []byte("a-very-strong-jwt-key-without-weak-patterns-9876543210")}

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate RSA key: %v", err)
	}

	now := time.Now()
	token := jwtTokenForTest(Claims{
		RegisteredClaims: RegisteredClaimsForTest(now, now.Add(1*time.Hour)),
		Username:         "alice",
		SessionID:        "sid",
		DeviceID:         "dev",
		Scopes:           []string{"read"},
	}, privateKey)

	_, err = as.ValidateAccessToken(token)
	assert.Error(t, err)
}

// Helpers kept local to avoid coupling tests to unrelated auth code.
func RegisteredClaimsForTest(now, expires time.Time) jwt.RegisteredClaims {
	return jwt.RegisteredClaims{
		Subject:   "alice",
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(expires),
		NotBefore: jwt.NewNumericDate(now),
	}
}

func jwtTokenForTest(claims Claims, privateKey *rsa.PrivateKey) string {
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	s, err := token.SignedString(privateKey)
	if err != nil {
		panic(err)
	}
	return s
}

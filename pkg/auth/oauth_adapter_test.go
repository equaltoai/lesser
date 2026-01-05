package auth

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/pay-theory/dynamorm/pkg/mocks"
	"github.com/pay-theory/lift/pkg/lift"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
)

type oauthAdapterRepos struct {
	account *repositories.AccountRepository
}

func (r oauthAdapterRepos) Account() *repositories.AccountRepository      { return r.account }
func (oauthAdapterRepos) Actor() interfaces.ActorRepository               { return nil }
func (oauthAdapterRepos) Activity() interfaces.ActivityRepository         { return nil }
func (oauthAdapterRepos) Notification() interfaces.NotificationRepository { return nil }
func (oauthAdapterRepos) Recovery() *repositories.RecoveryRepository      { return nil }
func (oauthAdapterRepos) Audit() *repositories.AuditRepository            { return nil }

func TestOAuthServiceAdapter_ValidateAccessToken(t *testing.T) {
	adapter := NewOAuthServiceAdapter(&AuthService{})
	_, err := adapter.ValidateAccessToken("not-a-token")
	require.Error(t, err)

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

	logger := zaptest.NewLogger(t)
	accountRepo := repositories.NewAccountRepository(db, "test-table", "example.com", logger)

	as := &AuthService{
		repos:     oauthAdapterRepos{account: accountRepo},
		jwtSecret: []byte(secret),
	}

	token, err := as.generateShortLivedAccessToken("alice", sessionID, "dev-1", DefaultScopes())
	require.NoError(t, err)

	adapter = NewOAuthServiceAdapter(as)
	claims, err := adapter.ValidateAccessToken(token)
	require.NoError(t, err)
	require.NotNil(t, claims)
	assert.Equal(t, "alice", claims.GetUsername())

	_ = CreateAPIAuthMiddlewareFromAuthService(as, zap.NewNop())
	_ = CreateGraphQLAuthMiddlewareFromAuthService(as, zap.NewNop())
	_ = CreateFederationAuthMiddlewareFromAuthService(as, zap.NewNop())
}

func TestOAuthServiceAdapter_MiddlewareIntegration_Smoke(t *testing.T) {
	// Create a service with no JWT secret, but we won't validate tokens.
	as := &AuthService{jwtSecret: []byte("dummy")}
	mw := CreateAPIAuthMiddlewareFromAuthService(as, zap.NewNop())
	require.NotNil(t, mw)

	req := lift.NewRequest(nil)
	ctx := lift.NewContext(context.Background(), req)

	called := false
	handler := mw(lift.HandlerFunc(func(*lift.Context) error {
		called = true
		return nil
	}))

	require.NoError(t, handler.Handle(ctx))
	assert.True(t, called)
}

package auth

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	dynamormCore "github.com/theory-cloud/tabletheory/pkg/core"
	dynamormmocks "github.com/theory-cloud/tabletheory/pkg/mocks"
	"go.uber.org/zap"
)

type reposStub struct{}

func (reposStub) Account() *repositories.AccountRepository        { return nil }
func (reposStub) Actor() interfaces.ActorRepository               { return nil }
func (reposStub) Activity() interfaces.ActivityRepository         { return nil }
func (reposStub) Notification() interfaces.NotificationRepository { return nil }
func (reposStub) Recovery() *repositories.RecoveryRepository      { return nil }
func (reposStub) Audit() *repositories.AuditRepository            { return nil }

type reposWithAccount struct {
	account *repositories.AccountRepository
}

func (r reposWithAccount) Account() *repositories.AccountRepository      { return r.account }
func (reposWithAccount) Actor() interfaces.ActorRepository               { return nil }
func (reposWithAccount) Activity() interfaces.ActivityRepository         { return nil }
func (reposWithAccount) Notification() interfaces.NotificationRepository { return nil }
func (reposWithAccount) Recovery() *repositories.RecoveryRepository      { return nil }
func (reposWithAccount) Audit() *repositories.AuditRepository            { return nil }

type leaseReposStub struct {
	db dynamormCore.DB
}

func (leaseReposStub) Account() *repositories.AccountRepository        { return nil }
func (leaseReposStub) Actor() interfaces.ActorRepository               { return nil }
func (leaseReposStub) Activity() interfaces.ActivityRepository         { return nil }
func (leaseReposStub) Notification() interfaces.NotificationRepository { return nil }
func (leaseReposStub) Recovery() *repositories.RecoveryRepository      { return nil }
func (leaseReposStub) Audit() *repositories.AuditRepository            { return nil }
func (r leaseReposStub) GetDB() dynamormCore.DB                        { return r.db }
func (leaseReposStub) GetTableName() string                            { return "test-table" }

func TestOAuthService_GenerateTokensWithAccessTokenTTLAndClientContext(t *testing.T) {
	t.Parallel()

	svc := &OAuthService{jwtSecret: []byte("test-secret")}

	access, refresh, err := svc.GenerateTokensWithAccessTokenTTLAndClientContext(
		context.Background(),
		"alice",
		"client-1",
		"192.0.2.10",
		[]string{ScopeRead},
		-1*time.Second,
		ClientClassCLI,
		" sid-1 ",
	)
	require.NoError(t, err)
	require.NotEmpty(t, access)
	require.NotEmpty(t, refresh)

	claims, err := svc.ValidateAccessToken(access)
	require.NoError(t, err)
	require.Equal(t, "alice", claims.Username)
	require.Equal(t, "client-1", claims.ClientID)
	require.Equal(t, []string{ScopeRead}, claims.Scopes)
	require.Equal(t, "192.0.2.10", claims.IPAddress)
	require.Equal(t, ClientClassCLI, claims.ClientClass)
	require.Equal(t, "sid-1", claims.SessionID)
	require.NotNil(t, claims.ExpiresAt)
	require.Greater(t, time.Until(claims.ExpiresAt.Time), 30*time.Minute)
}

func TestOAuthService_ValidateAccessToken_AllowsLongLivedLeaseTokens(t *testing.T) {
	t.Parallel()

	mockDB := new(dynamormmocks.MockDB)
	mockQuery := new(dynamormmocks.MockQuery)
	now := time.Now().UTC()

	mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
	mockDB.On("Model", mock.AnythingOfType("*models.AgentAccessLease")).Return(mockQuery).Once()
	mockQuery.On("Where", "PK", "=", "AGENT_ACCESS_LEASE#agent1").Return(mockQuery).Once()
	mockQuery.On("Where", "SK", "=", "LEASE#lease-1").Return(mockQuery).Once()
	mockQuery.On("First", mock.AnythingOfType("*models.AgentAccessLease")).
		Run(func(args mock.Arguments) {
			dest := args.Get(0).(*models.AgentAccessLease)
			*dest = models.AgentAccessLease{
				ID:                "lease-1",
				Username:          "agent1",
				Status:            "active",
				IdleExpiresAt:     now.Add(7 * 24 * time.Hour),
				AbsoluteExpiresAt: now.Add(30 * 24 * time.Hour),
			}
		}).
		Return(nil).Once()

	svc := &OAuthService{
		jwtSecret: []byte("test-secret"),
		repos:     leaseReposStub{db: mockDB},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "agent1",
			IssuedAt:  jwt.NewNumericDate(now.Add(-48 * time.Hour)),
			ExpiresAt: jwt.NewNumericDate(now.Add(48 * time.Hour)),
			NotBefore: jwt.NewNumericDate(now.Add(-48 * time.Hour)),
		},
		Username:    "agent1",
		ClientID:    agentLeaseOAuthClientID,
		ClientClass: ClientClassAgent,
		SessionID:   "lease-1",
		Scopes:      []string{ScopeRead},
	})
	tokenString, err := token.SignedString(svc.jwtSecret)
	require.NoError(t, err)

	claims, err := svc.ValidateAccessToken(tokenString)
	require.NoError(t, err)
	require.Equal(t, "agent1", claims.Username)
	require.Equal(t, "lease-1", claims.SessionID)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestOAuthService_ValidateAccessToken_RejectsRevokedLeaseTokens(t *testing.T) {
	t.Parallel()

	mockDB := new(dynamormmocks.MockDB)
	mockQuery := new(dynamormmocks.MockQuery)
	now := time.Now().UTC()

	mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
	mockDB.On("Model", mock.AnythingOfType("*models.AgentAccessLease")).Return(mockQuery).Once()
	mockQuery.On("Where", "PK", "=", "AGENT_ACCESS_LEASE#agent1").Return(mockQuery).Once()
	mockQuery.On("Where", "SK", "=", "LEASE#lease-1").Return(mockQuery).Once()
	mockQuery.On("First", mock.AnythingOfType("*models.AgentAccessLease")).
		Run(func(args mock.Arguments) {
			dest := args.Get(0).(*models.AgentAccessLease)
			*dest = models.AgentAccessLease{
				ID:                "lease-1",
				Username:          "agent1",
				Status:            "revoked",
				IdleExpiresAt:     now.Add(7 * 24 * time.Hour),
				AbsoluteExpiresAt: now.Add(30 * 24 * time.Hour),
			}
		}).
		Return(nil).Once()

	svc := &OAuthService{
		jwtSecret: []byte("test-secret"),
		repos:     leaseReposStub{db: mockDB},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "agent1",
			IssuedAt:  jwt.NewNumericDate(now.Add(-48 * time.Hour)),
			ExpiresAt: jwt.NewNumericDate(now.Add(48 * time.Hour)),
			NotBefore: jwt.NewNumericDate(now.Add(-48 * time.Hour)),
		},
		Username:    "agent1",
		ClientID:    agentLeaseOAuthClientID,
		ClientClass: ClientClassAgent,
		SessionID:   "lease-1",
		Scopes:      []string{ScopeRead},
	})
	tokenString, err := token.SignedString(svc.jwtSecret)
	require.NoError(t, err)

	_, err = svc.ValidateAccessToken(tokenString)
	require.ErrorIs(t, err, ErrInvalidToken)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestResolveAgentClaims_Branches(t *testing.T) {
	t.Parallel()

	{
		svc := &OAuthService{jwtSecret: []byte("test-secret")}
		isAgent, agentType, delegatedBy := svc.resolveAgentClaims(context.Background(), "alice")
		require.False(t, isAgent)
		require.Empty(t, agentType)
		require.Empty(t, delegatedBy)
	}

	{
		svc := &OAuthService{jwtSecret: []byte("test-secret"), repos: reposStub{}}
		isAgent, agentType, delegatedBy := svc.resolveAgentClaims(context.Background(), "alice")
		require.False(t, isAgent)
		require.Empty(t, agentType)
		require.Empty(t, delegatedBy)
	}

	{
		accountRepo := repositories.NewAccountRepository(nil, "test-table", "example.com", zap.NewNop())
		svc := &OAuthService{jwtSecret: []byte("test-secret"), repos: reposWithAccount{account: accountRepo}}
		isAgent, agentType, delegatedBy := svc.resolveAgentClaims(context.Background(), "alice")
		require.False(t, isAgent)
		require.Empty(t, agentType)
		require.Empty(t, delegatedBy)
	}
}

func TestNormalizeDelegatedBy(t *testing.T) {
	t.Parallel()

	require.Equal(t, "", normalizeDelegatedBy(""))
	require.Equal(t, "", normalizeDelegatedBy(" "))
	require.Equal(t, "@owner", normalizeDelegatedBy("owner"))
	require.Equal(t, "@owner", normalizeDelegatedBy("@owner"))
	require.Equal(t, "@owner", normalizeDelegatedBy(" @owner "))
}

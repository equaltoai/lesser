package auth

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/stretchr/testify/require"
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

func TestOAuthService_GenerateTokensWithAccessTokenTTLAndClientContextAndAudience(t *testing.T) {
	t.Parallel()

	svc := &OAuthService{jwtSecret: []byte("test-secret")}

	access, refresh, err := svc.GenerateTokensWithAccessTokenTTLAndClientContextAndAudience(
		context.Background(),
		"alice",
		"client-1",
		"192.0.2.10",
		[]string{ScopeRead},
		time.Hour,
		ClientClassCLI,
		"sid-1",
		"https://mcp.example/resource",
	)
	require.NoError(t, err)
	require.NotEmpty(t, access)
	require.NotEmpty(t, refresh)

	claims, err := svc.ValidateAccessToken(access)
	require.NoError(t, err)
	require.Equal(t, []string{"https://mcp.example/resource"}, []string(claims.Audience))
}

func TestOAuthService_GenerateOperatorTokensCarryOperatorClaims(t *testing.T) {
	t.Parallel()

	svc := &OAuthService{jwtSecret: []byte("test-secret")}

	access, refresh, err := svc.GenerateTokensWithAccessTokenTTLAndClientContext(
		context.Background(),
		"admin",
		"operator-client",
		"192.0.2.10",
		[]string{ScopeRead, ScopeWrite, ScopeAdmin},
		time.Hour,
		ClientClassOperator,
		"",
	)
	require.NoError(t, err)
	require.NotEmpty(t, access)
	require.NotEmpty(t, refresh)

	claims, err := svc.ValidateAccessToken(access)
	require.NoError(t, err)
	require.Equal(t, "admin", claims.Username)
	require.Equal(t, "operator-client", claims.ClientID)
	require.Equal(t, ClientClassOperator, claims.ClientClass)
	require.Equal(t, []string{ScopeRead, ScopeWrite, ScopeAdmin}, claims.Scopes)
}

func TestResolveAgentClaims_Branches(t *testing.T) {
	t.Parallel()

	{
		svc := &OAuthService{jwtSecret: []byte("test-secret")}
		isAgent, agentType, delegatedBy := svc.resolveAgentClaims(context.Background(), "alice", "")
		require.False(t, isAgent)
		require.Empty(t, agentType)
		require.Empty(t, delegatedBy)
	}

	{
		svc := &OAuthService{jwtSecret: []byte("test-secret"), repos: reposStub{}}
		isAgent, agentType, delegatedBy := svc.resolveAgentClaims(context.Background(), "alice", "")
		require.False(t, isAgent)
		require.Empty(t, agentType)
		require.Empty(t, delegatedBy)
	}

	{
		accountRepo := repositories.NewAccountRepository(nil, "test-table", "example.com", zap.NewNop())
		svc := &OAuthService{jwtSecret: []byte("test-secret"), repos: reposWithAccount{account: accountRepo}}
		isAgent, agentType, delegatedBy := svc.resolveAgentClaims(context.Background(), "alice", "")
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

func TestAgentAccessTokenTTL(t *testing.T) {
	t.Parallel()

	require.Equal(t, AccessTokenDuration, AgentAccessTokenTTL(nil))
	require.Equal(t, AccessTokenDuration, AgentAccessTokenTTL(&config.Config{}))
	require.Equal(t, 6*time.Hour, AgentAccessTokenTTL(&config.Config{AgentAccessTokenDuration: 6 * time.Hour}))
}

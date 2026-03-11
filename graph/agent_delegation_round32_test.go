package graph

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	apperrors "github.com/equaltoai/lesser/pkg/errors"
	storageModels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	pkgtesting "github.com/equaltoai/lesser/pkg/testing"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	dynamormmocks "github.com/theory-cloud/tabletheory/pkg/mocks"
	"go.uber.org/zap"
)

type agentDelegationGraphStorage struct {
	*pkgtesting.MockRepositoryStorage
	accountRepo *repositories.AccountRepository
}

func (s *agentDelegationGraphStorage) Account() *repositories.AccountRepository {
	return s.accountRepo
}

func delegatedAgentAuthContext(username string, scopes ...string) context.Context {
	return context.WithValue(context.Background(), common.ContextKeyClaims, &auth.Claims{
		Username: username,
		Scopes:   scopes,
	})
}

func newDelegationResolver(t *testing.T, storedScopes []string, allowAgentRegistration bool) *Resolver {
	t.Helper()

	mockDB := new(dynamormmocks.MockDB)
	mockQuery := new(dynamormmocks.MockQuery)

	mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
	mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()

	mockQuery.On("WithContext", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Index", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("ConsistentRead").Return(mockQuery).Maybe()
	mockQuery.On("Create").Return(nil).Maybe()
	mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
		switch v := args.Get(0).(type) {
		case *storageModels.User:
			v.Username = "agent1"
			v.DisplayName = "Agent One"
			v.IsAgent = true
			v.AgentType = "CUSTOM"
			v.AgentVersion = "v1"
			v.AgentOwner = "@owner"
			v.Metadata = map[string]any{
				"agent_delegated_scopes": append([]string(nil), storedScopes...),
			}
			v.CreatedAt = time.Now().Add(-time.Hour)
			v.UpdatedAt = time.Now()
			_ = v.UpdateKeys()
		case *storageModels.Actor:
			v.Username = "agent1"
			v.NumericID = common.GenerateNumericID("agent1")
			v.CreatedAt = time.Now().Add(-time.Hour)
			v.UpdatedAt = time.Now()
			v.Actor = &activitypub.Actor{
				BaseObject: activitypub.BaseObject{
					ID:   "https://localhost/users/agent1",
					Type: activitypub.ServiceType,
				},
				PreferredUsername: "agent1",
			}
			_ = v.UpdateKeys()
		}
	}).Return(nil).Maybe()

	accountRepo := repositories.NewAccountRepository(mockDB, "test-table", "localhost", zap.NewNop())
	storage := &agentDelegationGraphStorage{
		MockRepositoryStorage: pkgtesting.NewMockRepositoryStorage(),
		accountRepo:           accountRepo,
	}

	cfg := &config.Config{
		Domain:                 "localhost",
		AllowAgents:            true,
		AllowAgentRegistration: allowAgentRegistration,
		JWTSecret:              strings.Repeat("x", 32),
	}

	return &Resolver{
		Config:  cfg,
		Storage: storage,
		Logger:  zap.NewNop(),
	}
}

func TestDelegateToAgent_AllowsExistingAgentWhenRegistrationDisabled(t *testing.T) {
	resolver := newDelegationResolver(t, []string{"read", "write:statuses"}, false)
	ctx := delegatedAgentAuthContext("owner", auth.ScopeRead, auth.ScopeWrite)

	result, err := resolver.Mutation().DelegateToAgent(ctx, model.DelegateToAgentInput{
		AgentUsername: "agent1",
		DisplayName:   "",
		Scopes:        []string{"write:statuses"},
		AgentType:     model.AgentTypeCustom,
		Version:       "v1",
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotEmpty(t, result.AccessToken)
	require.NotEmpty(t, result.RefreshToken)
	require.Equal(t, "Bearer", result.TokenType)
	require.Equal(t, "write:statuses", result.Scope)
	require.NotNil(t, result.Agent)
	require.Equal(t, "agent1", result.Agent.Username)
}

func TestDelegateToAgent_EnforcesStoredAgentScopeEnvelope(t *testing.T) {
	resolver := newDelegationResolver(t, []string{"read"}, true)
	ctx := delegatedAgentAuthContext("owner", auth.ScopeRead, auth.ScopeWrite, "follow")

	_, err := resolver.Mutation().DelegateToAgent(ctx, model.DelegateToAgentInput{
		AgentUsername: "agent1",
		DisplayName:   "",
		Scopes:        []string{"follow"},
		AgentType:     model.AgentTypeCustom,
		Version:       "v1",
	})
	require.Error(t, err)
	require.True(t, apperrors.HasCode(err, apperrors.CodeForbidden))
}

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
	"github.com/equaltoai/lesser/pkg/storage/theorydb/marshalers"
	pkgtesting "github.com/equaltoai/lesser/pkg/testing"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	dynamormerrors "github.com/theory-cloud/tabletheory/pkg/errors"
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

type passthroughEncryptor struct{}

func (passthroughEncryptor) Encrypt(plaintext []byte) ([]byte, error) {
	return append([]byte(nil), plaintext...), nil
}
func (passthroughEncryptor) Decrypt(ciphertext []byte) ([]byte, error) {
	return append([]byte(nil), ciphertext...), nil
}

type delegationGraphState struct {
	users              map[string]*storageModels.User
	actors             map[string]*storageModels.Actor
	refreshTokens      map[string]*storageModels.RefreshToken
	createConflictOnce bool
}

func newDelegationGraphState(username string, storedScopes []string) *delegationGraphState {
	now := time.Now().UTC()
	user := &storageModels.User{
		Username:     username,
		DisplayName:  "Agent One",
		IsAgent:      true,
		AgentType:    "CUSTOM",
		AgentVersion: "v1",
		AgentOwner:   "@owner",
		Metadata: map[string]any{
			"agent_delegated_scopes": append([]string(nil), storedScopes...),
		},
		CreatedAt: now.Add(-time.Hour),
		UpdatedAt: now,
	}
	_ = user.UpdateKeys()

	actor := &storageModels.Actor{
		Username:  username,
		NumericID: common.GenerateNumericID(username),
		CreatedAt: now.Add(-time.Hour),
		UpdatedAt: now,
		Actor: &activitypub.Actor{
			BaseObject: activitypub.BaseObject{
				ID:   "https://localhost/users/" + username,
				Type: activitypub.ServiceType,
			},
			PreferredUsername: username,
		},
	}
	_ = actor.UpdateKeys()

	return &delegationGraphState{
		users:  map[string]*storageModels.User{username: user},
		actors: map[string]*storageModels.Actor{username: actor},
	}
}

func (s *delegationGraphState) user(username string) *storageModels.User {
	if s == nil || s.users == nil {
		return nil
	}
	return s.users[username]
}

func (s *delegationGraphState) actor(username string) *storageModels.Actor {
	if s == nil || s.actors == nil {
		return nil
	}
	return s.actors[username]
}

func (s *delegationGraphState) refreshTokensForUserClient(username, clientID string) []storageModels.RefreshToken {
	if s == nil || s.refreshTokens == nil {
		return nil
	}
	items := make([]storageModels.RefreshToken, 0)
	targetPK := "RUNTIME_USER#" + username + "#" + clientID
	for _, token := range s.refreshTokens {
		if token != nil && token.GSI1PK == targetPK {
			items = append(items, *token)
		}
	}
	return items
}

func (s *delegationGraphState) refreshTokensForFamily(familyID string) []storageModels.RefreshToken {
	if s == nil || s.refreshTokens == nil {
		return nil
	}
	items := make([]storageModels.RefreshToken, 0)
	targetPK := "RUNTIME_FAMILY#" + familyID
	for _, token := range s.refreshTokens {
		if token != nil && token.GSI2PK == targetPK {
			items = append(items, *token)
		}
	}
	return items
}

func (s *delegationGraphState) storeCreate(model any) {
	switch v := model.(type) {
	case *storageModels.User:
		if s.users == nil {
			s.users = map[string]*storageModels.User{}
		}
		copyUser := *v
		s.users[v.Username] = &copyUser
	case *storageModels.Actor:
		if s.actors == nil {
			s.actors = map[string]*storageModels.Actor{}
		}
		copyActor := *v
		s.actors[v.Username] = &copyActor
	case *storageModels.RefreshToken:
		if s.refreshTokens == nil {
			s.refreshTokens = map[string]*storageModels.RefreshToken{}
		}
		copyToken := *v
		s.refreshTokens[v.Token] = &copyToken
	}
}

func newDelegationResolver(t *testing.T, state *delegationGraphState, allowAgentRegistration bool) *Resolver {
	t.Helper()

	if state == nil {
		state = &delegationGraphState{}
	}

	mockDB := new(dynamormmocks.MockDB)
	mockQuery := new(dynamormmocks.MockQuery)
	mockUpdate := new(dynamormmocks.MockUpdateBuilder)
	var currentModel any
	var lastPK string
	var lastGSI1PK string
	var lastGSI2PK string

	mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
	mockDB.On("Model", mock.Anything).Run(func(args mock.Arguments) {
		currentModel = args.Get(0)
		lastPK = ""
		lastGSI1PK = ""
		lastGSI2PK = ""
	}).Return(mockQuery).Maybe()

	mockQuery.On("WithContext", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		field, _ := args.Get(0).(string)
		value, _ := args.Get(2).(string)
		if field == "PK" {
			lastPK = value
		}
		if field == "gsi1PK" {
			lastGSI1PK = value
		}
		if field == "gsi2PK" {
			lastGSI2PK = value
		}
	}).Return(mockQuery).Maybe()
	mockQuery.On("Index", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("ConsistentRead").Return(mockQuery).Maybe()
	mockQuery.On("UpdateBuilder").Return(mockUpdate).Maybe()
	mockUpdate.On("SetIfNotExists", mock.Anything, mock.Anything, mock.Anything).Return(mockUpdate).Maybe()
	mockUpdate.On("Set", mock.Anything, mock.Anything).Return(mockUpdate).Maybe()
	mockUpdate.On("ConditionVersion", mock.Anything).Return(mockUpdate).Maybe()
	mockUpdate.On("Execute").Return(nil).Maybe()
	if state.createConflictOnce {
		mockQuery.On("Create").Return(dynamormerrors.ErrConditionFailed).Once()
	}
	mockQuery.On("Create").Run(func(mock.Arguments) {
		state.storeCreate(currentModel)
	}).Return(nil).Maybe()
	mockQuery.On("Update", mock.Anything).Run(func(mock.Arguments) {
		state.storeCreate(currentModel)
	}).Return(nil).Maybe()
	mockQuery.On("All", mock.MatchedBy(func(dest any) bool {
		_, ok := dest.(*[]storageModels.RefreshToken)
		return ok
	})).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]storageModels.RefreshToken)
		switch {
		case strings.HasPrefix(lastGSI1PK, "RUNTIME_USER#"):
			parts := strings.SplitN(lastGSI1PK, "#", 3)
			if len(parts) == 3 {
				*dest = state.refreshTokensForUserClient(parts[1], parts[2])
				return
			}
			*dest = nil
		case strings.HasPrefix(lastGSI2PK, "RUNTIME_FAMILY#"):
			*dest = state.refreshTokensForFamily(strings.TrimPrefix(lastGSI2PK, "RUNTIME_FAMILY#"))
		default:
			*dest = nil
		}
	}).Return(nil).Maybe()

	mockQuery.On("First", mock.MatchedBy(func(dest any) bool {
		_, ok := dest.(*storageModels.User)
		username := strings.TrimPrefix(lastPK, "USER#")
		return ok && state.user(username) == nil
	})).Return(dynamormerrors.ErrItemNotFound).Maybe()
	mockQuery.On("First", mock.MatchedBy(func(dest any) bool {
		_, ok := dest.(*storageModels.User)
		username := strings.TrimPrefix(lastPK, "USER#")
		return ok && state.user(username) != nil
	})).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*storageModels.User)
		*dest = *state.user(strings.TrimPrefix(lastPK, "USER#"))
	}).Return(nil).Maybe()
	mockQuery.On("First", mock.MatchedBy(func(dest any) bool {
		_, ok := dest.(*storageModels.Actor)
		username := strings.TrimPrefix(lastPK, "ACTOR#")
		return ok && state.actor(username) == nil
	})).Return(dynamormerrors.ErrItemNotFound).Maybe()
	mockQuery.On("First", mock.MatchedBy(func(dest any) bool {
		_, ok := dest.(*storageModels.Actor)
		username := strings.TrimPrefix(lastPK, "ACTOR#")
		return ok && state.actor(username) != nil
	})).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*storageModels.Actor)
		*dest = *state.actor(strings.TrimPrefix(lastPK, "ACTOR#"))
	}).Return(nil).Maybe()
	mockQuery.On("First", mock.Anything).Return(nil).Maybe()

	accountRepo := repositories.NewAccountRepository(mockDB, "test-table", "localhost", zap.NewNop())
	accountRepo.SetEncryptor(marshalers.Encryptor(passthroughEncryptor{}))
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
	resolver := newDelegationResolver(t, newDelegationGraphState("agent1", []string{"read", "write:statuses"}), false)
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

func TestDelegateToAgent_CreatesMissingAgentWhenRegistrationEnabled(t *testing.T) {
	resolver := newDelegationResolver(t, &delegationGraphState{}, true)
	ctx := delegatedAgentAuthContext("owner", auth.ScopeRead, auth.ScopeWrite, "follow")
	bio := "Market intelligence advisor"

	result, err := resolver.Mutation().DelegateToAgent(ctx, model.DelegateToAgentInput{
		AgentUsername: "scout",
		DisplayName:   "Scout",
		Bio:           &bio,
		Scopes:        []string{"read", "write", "follow"},
		AgentType:     model.AgentTypeResearcher,
		Version:       "1.0.0",
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "scout", result.Agent.Username)
	require.Equal(t, "Scout", result.Agent.DisplayName)
	require.NotNil(t, result.Agent.Bio)
	require.Equal(t, bio, *result.Agent.Bio)
	require.Equal(t, model.AgentTypeResearcher, result.Agent.AgentType)
	require.Equal(t, "1.0.0", result.Agent.AgentVersion)
	require.Equal(t, "@owner", *result.Agent.AgentOwner)
	require.ElementsMatch(t, []string{"read", "write", "follow"}, result.Agent.DelegatedScopes)
	require.True(t, result.Agent.AgentCapabilities.CanPost)
	require.True(t, result.Agent.AgentCapabilities.CanFollow)
}

func TestDelegateToAgent_CreateMissingAgentRespectsRegistrationPolicy(t *testing.T) {
	resolver := newDelegationResolver(t, &delegationGraphState{}, false)
	ctx := delegatedAgentAuthContext("owner", auth.ScopeRead, auth.ScopeWrite)

	_, err := resolver.Mutation().DelegateToAgent(ctx, model.DelegateToAgentInput{
		AgentUsername: "scout",
		DisplayName:   "Scout",
		Scopes:        []string{"read"},
		AgentType:     model.AgentTypeResearcher,
		Version:       "1.0.0",
	})
	require.Error(t, err)
	require.True(t, apperrors.HasCode(err, apperrors.CodeForbidden))
}

func TestDelegateToAgent_CreateConflictFallsBackToExistingAgent(t *testing.T) {
	state := &delegationGraphState{
		users: map[string]*storageModels.User{
			"scout": {
				Username:     "scout",
				DisplayName:  "Scout",
				IsAgent:      true,
				AgentType:    "RESEARCHER",
				AgentVersion: "1.0.0",
				AgentOwner:   "@owner",
				Metadata: map[string]any{
					"agent_delegated_scopes": []string{"read", "write"},
				},
				CreatedAt: time.Now().Add(-time.Hour),
				UpdatedAt: time.Now(),
			},
		},
		actors: map[string]*storageModels.Actor{
			"scout": {
				Username:  "scout",
				NumericID: common.GenerateNumericID("scout"),
				CreatedAt: time.Now().Add(-time.Hour),
				UpdatedAt: time.Now(),
				Actor: &activitypub.Actor{
					BaseObject: activitypub.BaseObject{
						ID:   "https://localhost/users/scout",
						Type: activitypub.ServiceType,
					},
					PreferredUsername: "scout",
				},
			},
		},
	}

	resolver := newDelegationResolver(t, state, true)
	ctx := delegatedAgentAuthContext("owner", auth.ScopeRead, auth.ScopeWrite)

	result, err := resolver.Mutation().DelegateToAgent(ctx, model.DelegateToAgentInput{
		AgentUsername: "scout",
		DisplayName:   "Scout",
		Scopes:        []string{"read"},
		AgentType:     model.AgentTypeResearcher,
		Version:       "1.0.0",
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "scout", result.Agent.Username)
}

func TestDelegateToAgent_EnforcesStoredAgentScopeEnvelope(t *testing.T) {
	resolver := newDelegationResolver(t, newDelegationGraphState("agent1", []string{"read"}), true)
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

func TestRevokeAgentToken_UsesRuntimeSessionRevocationSemantics(t *testing.T) {
	now := time.Now().UTC()
	state := newDelegationGraphState("agent1", []string{"read"})

	parent := &storageModels.RefreshToken{
		Token:             "rt-parent",
		ClientID:          delegatedAgentClientID,
		Username:          "agent1",
		Scopes:            []string{auth.ScopeRead},
		CreatedAt:         now.Add(-2 * time.Hour),
		ExpiresAt:         now.Add(24 * time.Hour),
		ClientClass:       auth.ClientClassAgent,
		SessionID:         "sid-runtime-1",
		FamilyID:          "family-runtime-1",
		Generation:        1,
		Current:           false,
		DeviceLabel:       "sim-runtime",
		LastUsedAt:        now.Add(-time.Hour),
		IdleExpiresAt:     now.Add(24 * time.Hour),
		AbsoluteExpiresAt: now.Add(7 * 24 * time.Hour),
		SessionCreatedAt:  now.Add(-48 * time.Hour),
		AccessTTLSeconds:  1800,
	}
	require.NoError(t, parent.BeforeCreate())

	current := &storageModels.RefreshToken{
		Token:             "rt-current",
		ClientID:          delegatedAgentClientID,
		Username:          "agent1",
		Scopes:            []string{auth.ScopeRead},
		CreatedAt:         now.Add(-30 * time.Minute),
		ExpiresAt:         now.Add(24 * time.Hour),
		ClientClass:       auth.ClientClassAgent,
		SessionID:         "sid-runtime-1",
		FamilyID:          "family-runtime-1",
		Generation:        2,
		Current:           true,
		DeviceLabel:       "sim-runtime",
		LastUsedAt:        now.Add(-5 * time.Minute),
		IdleExpiresAt:     now.Add(24 * time.Hour),
		AbsoluteExpiresAt: now.Add(7 * 24 * time.Hour),
		SessionCreatedAt:  now.Add(-48 * time.Hour),
		AccessTTLSeconds:  1800,
	}
	require.NoError(t, current.BeforeCreate())

	state.refreshTokens = map[string]*storageModels.RefreshToken{
		parent.Token:  parent,
		current.Token: current,
	}

	resolver := newDelegationResolver(t, state, true)
	ctx := delegatedAgentAuthContext("owner", "write:accounts")

	ok, err := resolver.Mutation().RevokeAgentToken(ctx, "agent1")
	require.NoError(t, err)
	require.True(t, ok)

	require.Contains(t, state.refreshTokens, "rt-parent")
	require.Contains(t, state.refreshTokens, "rt-current")
	require.True(t, state.refreshTokens["rt-parent"].Revoked)
	require.True(t, state.refreshTokens["rt-current"].Revoked)
	require.Equal(t, "manual_runtime_session_revocation", state.refreshTokens["rt-parent"].RevokedReason)
	require.Equal(t, "manual_runtime_session_revocation", state.refreshTokens["rt-current"].RevokedReason)
}

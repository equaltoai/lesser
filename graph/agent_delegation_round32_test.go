package graph

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	apperrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/storage"
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
	governance         map[string]*storageModels.AgentGovernanceState
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
		CreatedAt:    now.Add(-time.Hour),
		UpdatedAt:    now,
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

	governance := &storageModels.AgentGovernanceState{
		Username:        username,
		DelegatedScopes: append([]string(nil), storedScopes...),
		CreatedAt:       now.Add(-time.Hour),
		UpdatedAt:       now,
	}
	_ = governance.BeforeCreate()

	return &delegationGraphState{
		users:      map[string]*storageModels.User{username: user},
		actors:     map[string]*storageModels.Actor{username: actor},
		governance: map[string]*storageModels.AgentGovernanceState{username: governance},
	}
}

func (s *delegationGraphState) user(username string) *storageModels.User {
	if s == nil || s.users == nil {
		return nil
	}
	if user, ok := s.users[username]; ok {
		return user
	}
	for candidate, user := range s.users {
		if strings.EqualFold(candidate, username) || strings.EqualFold(user.Username, username) {
			return user
		}
	}
	return nil
}

func (s *delegationGraphState) actor(username string) *storageModels.Actor {
	if s == nil || s.actors == nil {
		return nil
	}
	if actor, ok := s.actors[username]; ok {
		return actor
	}
	for candidate, actor := range s.actors {
		if strings.EqualFold(candidate, username) || strings.EqualFold(actor.Username, username) {
			return actor
		}
	}
	return nil
}

func (s *delegationGraphState) governanceState(username string) *storageModels.AgentGovernanceState {
	if s == nil || s.governance == nil {
		return nil
	}
	if governance, ok := s.governance[username]; ok {
		return governance
	}
	for candidate, governance := range s.governance {
		if strings.EqualFold(candidate, username) || strings.EqualFold(governance.Username, username) {
			return governance
		}
	}
	return nil
}

func delegationPopulateUserProjection(dest any, user *storageModels.User) bool {
	if dest == nil || user == nil {
		return false
	}
	typeName := reflect.TypeOf(dest).String()
	if typeName != "*repositories.userCoreProjection" && typeName != "*repositories.userMetadataProjection" {
		return false
	}

	value := reflect.ValueOf(dest).Elem()
	delegationSetField(value, "Table", "test-table")
	delegationSetField(value, "PK", "USER#"+user.Username)
	delegationSetField(value, "SK", storageModels.SKMetadata)

	if typeName == "*repositories.userMetadataProjection" {
		delegationSetField(value, "Metadata", user.Metadata)
		return true
	}

	delegationSetField(value, "Username", user.Username)
	delegationSetField(value, "Email", user.Email)
	delegationSetField(value, "PasswordHash", user.PasswordHash)
	delegationSetField(value, "DisplayName", user.DisplayName)
	delegationSetField(value, "Note", user.Note)
	delegationSetField(value, "Avatar", user.Avatar)
	delegationSetField(value, "Header", user.Header)
	delegationSetField(value, "URL", user.URL)
	delegationSetField(value, "Locked", user.Locked)
	delegationSetField(value, "Discoverable", user.Discoverable)
	delegationSetField(value, "Fields", user.Fields)
	delegationSetField(value, "CreatedAt", user.CreatedAt)
	delegationSetField(value, "UpdatedAt", user.UpdatedAt)
	delegationSetField(value, "Approved", user.Approved)
	delegationSetField(value, "Suspended", user.Suspended)
	delegationSetField(value, "Silenced", user.Silenced)
	delegationSetField(value, "Role", user.Role)
	delegationSetField(value, "Locale", user.Locale)
	delegationSetField(value, "RecoveryMethods", user.RecoveryMethods)
	delegationSetField(value, "AllowNSFW", user.AllowNSFW)
	delegationSetField(value, "RequireNSFWWarning", user.RequireNSFWWarning)
	delegationSetField(value, "IsAgent", user.IsAgent)
	delegationSetField(value, "AgentType", user.AgentType)
	delegationSetField(value, "AgentCapabilities", user.AgentCapabilities)
	delegationSetField(value, "AgentVersion", user.AgentVersion)
	delegationSetField(value, "AgentOwner", user.AgentOwner)
	delegationSetField(value, "AgentCreatedBy", user.AgentCreatedBy)
	delegationSetField(value, "AgentPublicKey", user.AgentPublicKey)
	delegationSetField(value, "AgentKeyType", user.AgentKeyType)
	delegationSetField(value, "Version", user.Version)
	return true
}

func delegationSetField(target reflect.Value, name string, value any) {
	field := target.FieldByName(name)
	if !field.IsValid() || !field.CanSet() {
		return
	}
	if value == nil {
		field.Set(reflect.Zero(field.Type()))
		return
	}
	incoming := reflect.ValueOf(value)
	if incoming.Type().AssignableTo(field.Type()) {
		field.Set(incoming)
		return
	}
	if incoming.Type().ConvertibleTo(field.Type()) {
		field.Set(incoming.Convert(field.Type()))
	}
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
	case *storageModels.AgentGovernanceState:
		if s.governance == nil {
			s.governance = map[string]*storageModels.AgentGovernanceState{}
		}
		copyGovernance := *v
		s.governance[v.Username] = &copyGovernance
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
	mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Limit", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("ConsistentRead").Return(mockQuery).Maybe()
	mockQuery.On("IfNotExists").Return(mockQuery).Maybe()
	mockQuery.On("UpdateBuilder").Return(mockUpdate).Maybe()
	mockUpdate.On("SetIfNotExists", mock.Anything, mock.Anything, mock.Anything).Return(mockUpdate).Maybe()
	mockUpdate.On("Set", mock.Anything, mock.Anything).Return(mockUpdate).Maybe()
	mockUpdate.On("Remove", mock.Anything).Return(mockUpdate).Maybe()
	mockUpdate.On("ConditionNotExists", mock.Anything).Return(mockUpdate).Maybe()
	mockUpdate.On("ConditionVersion", mock.Anything).Return(mockUpdate).Maybe()
	mockUpdate.On("Execute").Run(func(mock.Arguments) {
		state.storeCreate(currentModel)
	}).Return(nil).Maybe()
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
	mockQuery.On("All", mock.MatchedBy(func(dest any) bool {
		_, ok := dest.(*[]*storageModels.OAuthClient)
		return ok
	})).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]*storageModels.OAuthClient)
		*dest = nil
	}).Return(nil).Maybe()

	mockQuery.On("First", mock.MatchedBy(func(dest any) bool {
		if dest == nil {
			return false
		}
		typeName := reflect.TypeOf(dest).String()
		if typeName != "*repositories.userCoreProjection" && typeName != "*repositories.userMetadataProjection" {
			return false
		}
		username := strings.TrimPrefix(lastPK, "USER#")
		return state.user(username) == nil
	})).Return(dynamormerrors.ErrItemNotFound).Maybe()
	mockQuery.On("First", mock.MatchedBy(func(dest any) bool {
		if dest == nil {
			return false
		}
		typeName := reflect.TypeOf(dest).String()
		if typeName != "*repositories.userCoreProjection" && typeName != "*repositories.userMetadataProjection" {
			return false
		}
		username := strings.TrimPrefix(lastPK, "USER#")
		return state.user(username) != nil
	})).Run(func(args mock.Arguments) {
		_ = delegationPopulateUserProjection(args.Get(0), state.user(strings.TrimPrefix(lastPK, "USER#")))
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
	mockQuery.On("First", mock.MatchedBy(func(dest any) bool {
		_, ok := dest.(*storageModels.AgentGovernanceState)
		username := strings.TrimPrefix(lastPK, "USER#")
		return ok && state.governanceState(username) == nil
	})).Return(dynamormerrors.ErrItemNotFound).Maybe()
	mockQuery.On("First", mock.MatchedBy(func(dest any) bool {
		_, ok := dest.(*storageModels.AgentGovernanceState)
		username := strings.TrimPrefix(lastPK, "USER#")
		return ok && state.governanceState(username) != nil
	})).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*storageModels.AgentGovernanceState)
		*dest = *state.governanceState(strings.TrimPrefix(lastPK, "USER#"))
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
				CreatedAt:    time.Now().Add(-time.Hour),
				UpdatedAt:    time.Now(),
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
		governance: map[string]*storageModels.AgentGovernanceState{
			"scout": {
				PK:              "USER#scout",
				SK:              storageModels.SKAgentGovernance,
				Username:        "scout",
				DelegatedScopes: []string{"read", "write"},
				CreatedAt:       time.Now().Add(-time.Hour),
				UpdatedAt:       time.Now(),
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

func TestDelegateToAgent_MissingGovernanceFailsClosed(t *testing.T) {
	state := newDelegationGraphState("agent1", []string{"read"})
	delete(state.governance, "agent1")

	resolver := newDelegationResolver(t, state, true)
	ctx := delegatedAgentAuthContext("owner", auth.ScopeRead, auth.ScopeWrite)

	_, err := resolver.Mutation().DelegateToAgent(ctx, model.DelegateToAgentInput{
		AgentUsername: "agent1",
		DisplayName:   "",
		Scopes:        []string{"read"},
		AgentType:     model.AgentTypeCustom,
		Version:       "v1",
	})
	require.ErrorIs(t, err, ErrAgentGovernanceUnavailable)
}

func TestAdminUnverifyAgent_ClearsStaleVerificationFields(t *testing.T) {
	state := newDelegationGraphState("agent1", []string{"read"})
	verifiedAt := time.Now().UTC().Add(-time.Hour)
	governance := state.governanceState("agent1")
	governance.Verified = true
	governance.VerifiedAt = &verifiedAt
	governance.VerifiedBy = "reviewer"
	governance.VerifiedReason = "approved"

	resolver := newDelegationResolver(t, state, true)
	ctx := delegatedAgentAuthContext("admin", auth.ScopeAdmin)

	result, err := resolver.Mutation().AdminUnverifyAgent(ctx, "agent1", &model.AdminVerifyAgentInput{
		Reason: ptrString("revoked"),
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.False(t, result.Verified)
	require.Nil(t, result.VerifiedAt)

	updated := state.governanceState("agent1")
	require.False(t, updated.Verified)
	require.Nil(t, updated.VerifiedAt)
	require.Empty(t, updated.VerifiedBy)
	require.Empty(t, updated.VerifiedReason)
	require.Equal(t, "admin", updated.UnverifiedBy)
	require.Equal(t, "revoked", updated.UnverifiedReason)
	require.NotNil(t, updated.UnverifiedAt)
}

func TestAdminVerifyAgent_ReturnsUpdatedQuarantineState(t *testing.T) {
	state := newDelegationGraphState("agent1", []string{"read"})
	start := time.Now().UTC().Add(-time.Hour)
	end := time.Now().UTC().Add(24 * time.Hour)
	governance := state.governanceState("agent1")
	governance.QuarantineStatus = storage.AgentQuarantineStatusQuarantined
	governance.QuarantineStart = &start
	governance.QuarantineEnd = &end

	resolver := newDelegationResolver(t, state, true)
	ctx := delegatedAgentAuthContext("admin", auth.ScopeAdmin)

	exit := true
	result, err := resolver.Mutation().AdminVerifyAgent(ctx, "agent1", &model.AdminVerifyAgentInput{
		Reason:         ptrString("ok"),
		ExitQuarantine: &exit,
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.Verified)
	require.NotNil(t, result.QuarantineStatus)
	require.Equal(t, storage.AgentQuarantineStatusApproved, *result.QuarantineStatus)
	require.False(t, result.QuarantineActive)
	require.NotNil(t, result.QuarantineApprovedBy)
	require.Equal(t, "admin", *result.QuarantineApprovedBy)
	require.NotNil(t, result.QuarantineApprovedAt)

	updated := state.governanceState("agent1")
	require.Equal(t, storage.AgentQuarantineStatusApproved, updated.QuarantineStatus)
	require.Equal(t, "admin", updated.QuarantineApprovedBy)
	require.NotNil(t, updated.QuarantineApprovedAt)
}

func TestAdminUnverifyAgent_MissingGovernanceFailsClosed(t *testing.T) {
	state := newDelegationGraphState("agent1", []string{"read"})
	delete(state.governance, "agent1")

	resolver := newDelegationResolver(t, state, true)
	ctx := delegatedAgentAuthContext("admin", auth.ScopeAdmin)

	_, err := resolver.Mutation().AdminUnverifyAgent(ctx, "agent1", &model.AdminVerifyAgentInput{
		Reason: ptrString("revoked"),
	})
	require.ErrorIs(t, err, ErrAgentGovernanceUnavailable)
}

func TestRevokeAgentToken_UsesRuntimeSessionRevocationSemantics(t *testing.T) {
	now := time.Now().UTC()
	state := newDelegationGraphState("agent1", []string{"read"})

	parent := &storageModels.RefreshToken{
		Token:               "rt-parent",
		ClientID:            delegatedAgentClientID,
		Username:            "agent1",
		Scopes:              []string{auth.ScopeRead},
		CreatedAt:           now.Add(-2 * time.Hour),
		ExpiresAt:           now.Add(24 * time.Hour),
		ClientClass:         auth.ClientClassAgent,
		SessionID:           "sid-runtime-1",
		FamilyID:            "family-runtime-1",
		Generation:          1,
		Current:             false,
		DeviceLabel:         "sim-runtime",
		LastUsedAt:          now.Add(-time.Hour),
		IdleExpiresAt:       now.Add(24 * time.Hour),
		AbsoluteExpiresAt:   now.Add(7 * 24 * time.Hour),
		SessionCreatedAt:    now.Add(-48 * time.Hour),
		AccessTTLSeconds:    1800,
		LastAuthSuccessAt:   now.Add(-45 * time.Minute),
		LastAuthFailureCode: "invalid_client",
		LastAuthFailureMsg:  "Invalid client credentials",
		LastAuthFailureAt:   now.Add(-10 * time.Minute),
	}
	require.NoError(t, parent.BeforeCreate())

	current := &storageModels.RefreshToken{
		Token:               "rt-current",
		ClientID:            delegatedAgentClientID,
		Username:            "agent1",
		Scopes:              []string{auth.ScopeRead},
		CreatedAt:           now.Add(-30 * time.Minute),
		ExpiresAt:           now.Add(24 * time.Hour),
		ClientClass:         auth.ClientClassAgent,
		SessionID:           "sid-runtime-1",
		FamilyID:            "family-runtime-1",
		Generation:          2,
		Current:             true,
		DeviceLabel:         "sim-runtime",
		LastUsedAt:          now.Add(-5 * time.Minute),
		IdleExpiresAt:       now.Add(24 * time.Hour),
		AbsoluteExpiresAt:   now.Add(7 * 24 * time.Hour),
		SessionCreatedAt:    now.Add(-48 * time.Hour),
		AccessTTLSeconds:    1800,
		LastAuthSuccessAt:   now.Add(-45 * time.Minute),
		LastAuthFailureCode: "invalid_client",
		LastAuthFailureMsg:  "Invalid client credentials",
		LastAuthFailureAt:   now.Add(-10 * time.Minute),
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

func TestAgentRuntimeSessions_ListAndRevoke(t *testing.T) {
	now := time.Now().UTC()
	state := newDelegationGraphState("agent1", []string{"read"})

	current := &storageModels.RefreshToken{
		Token:               "rt-current",
		ClientID:            delegatedAgentClientID,
		Username:            "agent1",
		Scopes:              []string{auth.ScopeRead, "write:statuses"},
		CreatedAt:           now.Add(-30 * time.Minute),
		ExpiresAt:           now.Add(24 * time.Hour),
		ClientClass:         auth.ClientClassAgent,
		SessionID:           "sid-runtime-1",
		FamilyID:            "family-runtime-1",
		Generation:          2,
		Current:             true,
		DeviceLabel:         "sim-runtime",
		LastUsedAt:          now.Add(-5 * time.Minute),
		IdleExpiresAt:       now.Add(24 * time.Hour),
		AbsoluteExpiresAt:   now.Add(7 * 24 * time.Hour),
		SessionCreatedAt:    now.Add(-48 * time.Hour),
		AccessTTLSeconds:    1800,
		LastAuthSuccessAt:   now.Add(-45 * time.Minute),
		LastAuthFailureCode: "invalid_client",
		LastAuthFailureMsg:  "Invalid client credentials",
		LastAuthFailureAt:   now.Add(-10 * time.Minute),
	}
	require.NoError(t, current.BeforeCreate())

	state.refreshTokens = map[string]*storageModels.RefreshToken{
		current.Token: current,
	}

	resolver := newDelegationResolver(t, state, true)
	ctx := delegatedAgentAuthContext("owner", "write:accounts")

	sessions, err := resolver.Query().AgentRuntimeSessions(ctx, "agent1")
	require.NoError(t, err)
	require.Len(t, sessions, 1)
	require.Equal(t, "sid-runtime-1", sessions[0].SessionID)
	require.Equal(t, "sim-runtime", sessions[0].DeviceLabel)
	require.Equal(t, "read write:statuses", sessions[0].Scope)
	require.NotNil(t, sessions[0].AuthDiagnostic)
	require.Equal(t, model.AgentRuntimeSessionAuthStatusFailed, sessions[0].AuthDiagnostic.Status)
	require.Equal(t, "invalid_client", derefString(sessions[0].AuthDiagnostic.FailureCode))
	require.Equal(t, "Invalid client credentials", derefString(sessions[0].AuthDiagnostic.FailureMessage))
	require.NotNil(t, sessions[0].AuthDiagnostic.LastSuccessAt)

	reason := "operator_retired_runtime"
	revoked, err := resolver.Mutation().RevokeAgentRuntimeSession(ctx, "agent1", "sid-runtime-1", &reason)
	require.NoError(t, err)
	require.NotNil(t, revoked)
	require.True(t, revoked.Revoked)
	require.Equal(t, "operator_retired_runtime", derefString(revoked.RevokedReason))
	require.NotNil(t, revoked.AuthDiagnostic)
	require.Equal(t, model.AgentRuntimeSessionAuthStatusRevoked, revoked.AuthDiagnostic.Status)

	require.True(t, state.refreshTokens["rt-current"].Revoked)
	require.Equal(t, "operator_retired_runtime", state.refreshTokens["rt-current"].RevokedReason)
}

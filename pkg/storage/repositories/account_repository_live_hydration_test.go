package repositories

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/agents"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/pkg/mocks"
	"go.uber.org/zap/zaptest"
)

type liveAccountHydrationFixture struct {
	User  liveAccountHydrationUserFixture  `json:"user"`
	Actor liveAccountHydrationActorFixture `json:"actor"`
}

type liveAccountHydrationUserFixture struct {
	Username           string               `json:"username"`
	DisplayName        string               `json:"displayName"`
	Note               string               `json:"note"`
	URL                string               `json:"url"`
	Locked             bool                 `json:"locked"`
	Discoverable       bool                 `json:"discoverable"`
	Fields             []map[string]string  `json:"fields"`
	CreatedAt          time.Time            `json:"createdAt"`
	UpdatedAt          time.Time            `json:"updatedAt"`
	Approved           bool                 `json:"approved"`
	Suspended          bool                 `json:"suspended"`
	Silenced           bool                 `json:"silenced"`
	Role               string               `json:"role"`
	Locale             string               `json:"locale"`
	RecoveryMethods    []string             `json:"recoveryMethods"`
	AllowNSFW          bool                 `json:"allowNSFW"`
	RequireNSFWWarning bool                 `json:"requireNSFWWarning"`
	IsAgent            bool                 `json:"isAgent"`
	AgentType          string               `json:"agentType"`
	AgentVersion       string               `json:"agentVersion"`
	AgentOwner         string               `json:"agentOwner"`
	AgentCreatedBy     string               `json:"agentCreatedBy"`
	AgentPublicKey     string               `json:"agentPublicKey"`
	AgentKeyType       string               `json:"agentKeyType"`
	AgentCapabilities  *agents.Capabilities `json:"agentCapabilities"`
	Version            int                  `json:"version"`
}

type liveAccountHydrationActorFixture struct {
	ID                string `json:"id"`
	PreferredUsername string `json:"preferredUsername"`
	Name              string `json:"name"`
	URL               string `json:"url"`
	Type              string `json:"type"`
}

func TestAccountRepository_GetAccount_LiveAgentFixtures(t *testing.T) {
	ctx := context.Background()
	fixtures := loadLiveAccountHydrationFixtures(t)

	for username, fixture := range fixtures {
		t.Run(username, func(t *testing.T) {
			mockDB := new(mocks.MockDB)
			mockQuery := new(mocks.MockQuery)

			mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
			mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()

			mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
			mockQuery.On("Index", mock.Anything).Return(mockQuery).Maybe()
			mockQuery.On("Limit", mock.Anything).Return(mockQuery).Maybe()
			mockQuery.On("First", mock.MatchedBy(func(dest any) bool {
				_, ok := dest.(*userCoreProjection)
				return ok
			})).Run(func(args mock.Arguments) {
				dest := args.Get(0).(*userCoreProjection)
				*dest = fixture.User.toCoreProjection()
				dest.Table = "test-table"
			}).Return(nil).Once()
			mockQuery.On("First", mock.MatchedBy(func(dest any) bool {
				_, ok := dest.(*userMetadataProjection)
				return ok
			})).Run(func(args mock.Arguments) {
				dest := args.Get(0).(*userMetadataProjection)
				dest.Table = "test-table"
			}).Return(nil).Once()
			mockQuery.On("First", mock.MatchedBy(func(dest any) bool {
				_, ok := dest.(*models.Actor)
				return ok
			})).Run(func(args mock.Arguments) {
				dest := args.Get(0).(*models.Actor)
				dest.Username = fixture.User.Username
				dest.Actor = fixture.Actor.toActivityPubActor()
				dest.CreatedAt = fixture.User.CreatedAt
				dest.UpdatedAt = fixture.User.UpdatedAt
				dest.Version = 1
				_ = dest.UpdateKeys()
			}).Return(nil).Once()

			repo := NewAccountRepository(mockDB, "test-table", "dev.simulacrum.greater.website", zaptest.NewLogger(t))

			account, err := repo.GetAccount(ctx, username)
			require.NoError(t, err)
			require.NotNil(t, account)
			require.NotNil(t, account.User)
			require.NotNil(t, account.Actor)
			require.Equal(t, fixture.User.Username, account.User.Username)
			require.Equal(t, fixture.User.DisplayName, account.User.DisplayName)
			require.Equal(t, fixture.User.AgentType, account.User.AgentType)
			require.Equal(t, fixture.User.Version, account.User.Version)
			require.Equal(t, fixture.Actor.PreferredUsername, account.Actor.PreferredUsername)
			require.Equal(t, fixture.Actor.Name, account.Actor.Name)

			mockDB.AssertExpectations(t)
			mockQuery.AssertExpectations(t)
		})
	}
}

func TestAccountRepository_GetUser_IgnoresOptionalMetadataDecodeFailure(t *testing.T) {
	ctx := context.Background()
	fixture := loadLiveAccountHydrationFixtures(t)["medic"]

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
	mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()

	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("First", mock.MatchedBy(func(dest any) bool {
		_, ok := dest.(*userCoreProjection)
		return ok
	})).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*userCoreProjection)
		*dest = fixture.User.toCoreProjection()
		dest.Table = "test-table"
	}).Return(nil).Once()
	mockQuery.On("First", mock.MatchedBy(func(dest any) bool {
		_, ok := dest.(*userMetadataProjection)
		return ok
	})).Return(errors.New("decode metadata: unexpected nested shape")).Once()

	repo := NewAccountRepository(mockDB, "test-table", "dev.simulacrum.greater.website", zaptest.NewLogger(t))

	user, err := repo.GetUser(ctx, "medic")
	require.NoError(t, err)
	require.NotNil(t, user)
	require.Equal(t, fixture.User.Username, user.Username)
	require.Equal(t, fixture.User.DisplayName, user.DisplayName)
	require.Equal(t, fixture.User.AgentType, user.AgentType)
	require.Nil(t, user.Metadata)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func loadLiveAccountHydrationFixtures(t *testing.T) map[string]liveAccountHydrationFixture {
	t.Helper()

	path := filepath.Join("..", "..", "..", "testdata", "account_hydration", "live_agents.json")
	raw, err := os.ReadFile(path)
	require.NoError(t, err)

	var fixtures map[string]liveAccountHydrationFixture
	require.NoError(t, json.Unmarshal(raw, &fixtures))
	require.NotEmpty(t, fixtures)
	return fixtures
}

func (f liveAccountHydrationUserFixture) toCoreProjection() userCoreProjection {
	return userCoreProjection{
		PK:                 "USER#" + f.Username,
		SK:                 models.SKMetadata,
		Username:           f.Username,
		DisplayName:        f.DisplayName,
		Note:               f.Note,
		URL:                f.URL,
		Locked:             f.Locked,
		Discoverable:       f.Discoverable,
		Fields:             cloneFields(f.Fields),
		CreatedAt:          f.CreatedAt,
		UpdatedAt:          f.UpdatedAt,
		Approved:           f.Approved,
		Suspended:          f.Suspended,
		Silenced:           f.Silenced,
		Role:               f.Role,
		Locale:             f.Locale,
		RecoveryMethods:    append([]string(nil), f.RecoveryMethods...),
		AllowNSFW:          f.AllowNSFW,
		RequireNSFWWarning: f.RequireNSFWWarning,
		IsAgent:            f.IsAgent,
		AgentType:          f.AgentType,
		AgentVersion:       f.AgentVersion,
		AgentOwner:         f.AgentOwner,
		AgentCreatedBy:     f.AgentCreatedBy,
		AgentPublicKey:     f.AgentPublicKey,
		AgentKeyType:       f.AgentKeyType,
		AgentCapabilities:  f.AgentCapabilities,
		Version:            f.Version,
	}
}

func (f liveAccountHydrationActorFixture) toActivityPubActor() *activitypub.Actor {
	return &activitypub.Actor{
		BaseObject: activitypub.BaseObject{
			ID:   f.ID,
			Type: f.Type,
		},
		PreferredUsername: f.PreferredUsername,
		Name:              f.Name,
		URL:               f.URL,
	}
}

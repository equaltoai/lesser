package repositories

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	dynamormErrors "github.com/theory-cloud/tabletheory/pkg/errors"
	"github.com/theory-cloud/tabletheory/pkg/mocks"
	"go.uber.org/zap/zaptest"
)

func TestAccountRepository_CanonicalUsername_LowercasesNormalizedHandles(t *testing.T) {
	repo := &AccountRepository{domain: "example.com"}

	require.Equal(t, "arch", repo.canonicalUsername("acct:@Arch@example.com"))
	require.Equal(t, "arch", repo.canonicalUsername("https://example.com/users/Arch"))
	require.Equal(t, "arch@remote.example", repo.canonicalUsername("@Arch@Remote.Example"))
	require.Equal(t, "arch@remote.example", repo.canonicalUsername("https://Remote.Example/users/Arch"))
}

func TestAccountRepository_GetUser_FallsBackToLegacyMixedCaseKeys(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, time.March, 23, 9, 0, 0, 0, time.UTC)

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	mockDB.On("WithContext", ctx).Return(mockDB).Maybe()
	mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Limit", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("First", mock.AnythingOfType("*repositories.userCoreProjection")).
		Return(dynamormErrors.ErrItemNotFound).
		Once()
	mockQuery.On("First", mock.AnythingOfType("*repositories.userCoreProjection")).
		Run(func(args mock.Arguments) {
			user := args.Get(0).(*userCoreProjection)
			user.Username = "Arch"
			user.Role = "user"
			user.CreatedAt = now
			user.UpdatedAt = now
		}).
		Return(nil).
		Once()
	mockQuery.On("First", mock.AnythingOfType("*repositories.userMetadataProjection")).
		Return(nil).
		Once()

	repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))

	user, err := repo.GetUser(ctx, "Arch")
	require.NoError(t, err)
	require.NotNil(t, user)
	require.Equal(t, "Arch", user.Username)
	mockQuery.AssertNumberOfCalls(t, "First", 3)
}

func TestAccountRepository_GetUser_ResolvesLegacyMixedCaseKeysViaCanonicalHandleIndex(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, time.March, 24, 11, 30, 0, 0, time.UTC)

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	mockDB.On("WithContext", ctx).Return(mockDB).Maybe()
	mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Limit", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Index", "gsi5").Return(mockQuery).Once()
	mockQuery.On("First", mock.AnythingOfType("*repositories.userCoreProjection")).
		Return(dynamormErrors.ErrItemNotFound).
		Once()
	mockQuery.On("First", mock.AnythingOfType("*repositories.userCoreProjection")).
		Run(func(args mock.Arguments) {
			user := args.Get(0).(*userCoreProjection)
			user.Username = "Medic"
			user.Role = "user"
			user.CreatedAt = now
			user.UpdatedAt = now
		}).
		Return(nil).
		Once()
	mockQuery.On("First", mock.AnythingOfType("*repositories.userMetadataProjection")).
		Return(nil).
		Once()

	repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))

	user, err := repo.GetUser(ctx, "medic")
	require.NoError(t, err)
	require.NotNil(t, user)
	require.Equal(t, "Medic", user.Username)
	mockQuery.AssertNumberOfCalls(t, "First", 3)
	mockQuery.AssertCalled(t, "Index", "gsi5")
}

func TestAccountRepository_GetActor_NormalizesLegacyMixedCaseIdentityOnRead(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, time.March, 23, 9, 30, 0, 0, time.UTC)

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	mockDB.On("WithContext", ctx).Return(mockDB).Maybe()
	mockDB.On("Model", mock.AnythingOfType("*models.Actor")).Return(mockQuery).Maybe()
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("First", mock.AnythingOfType("*models.Actor")).
		Return(dynamormErrors.ErrItemNotFound).
		Once()
	mockQuery.On("First", mock.AnythingOfType("*models.Actor")).
		Run(func(args mock.Arguments) {
			actorModel := args.Get(0).(*models.Actor)
			actorModel.Username = "Arch"
			actorModel.CreatedAt = now
			actorModel.UpdatedAt = now
			actorModel.Actor = &activitypub.Actor{
				BaseObject: activitypub.BaseObject{
					ID:   "https://example.com/users/Arch",
					Type: activitypub.PersonType,
				},
				PreferredUsername: "Arch",
				URL:               "https://example.com/@Arch",
			}
			_ = actorModel.UpdateKeys()
		}).
		Return(nil).
		Once()

	repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))

	actor, err := repo.GetActor(ctx, "Arch")
	require.NoError(t, err)
	require.NotNil(t, actor)
	require.Equal(t, "arch", actor.PreferredUsername)
	require.Equal(t, "https://example.com/users/arch", actor.ID)
	require.Equal(t, "https://example.com/@arch", actor.URL)
	mockQuery.AssertNumberOfCalls(t, "First", 2)
}

func TestAccountRepository_CreateActor_NormalizesMixedCaseIdentityOnWrite(t *testing.T) {
	ctx := context.Background()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	var currentModel any
	var createdActor *models.Actor

	mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
	mockDB.On("Model", mock.Anything).Run(func(args mock.Arguments) {
		currentModel = args.Get(0)
	}).Return(mockQuery).Maybe()
	mockQuery.On("Create").Run(func(mock.Arguments) {
		if actorModel, ok := currentModel.(*models.Actor); ok {
			copyActor := *actorModel
			createdActor = &copyActor
		}
	}).Return(nil).Maybe()

	repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))
	repo.SetEncryptor(testEncryptor{})

	err := repo.createActor(ctx, &activitypub.Actor{
		BaseObject: activitypub.BaseObject{
			ID:   "https://example.com/users/Arch",
			Type: activitypub.PersonType,
		},
		PreferredUsername: "Arch",
		URL:               "https://example.com/@Arch",
	}, "private-key")
	require.NoError(t, err)
	require.NotNil(t, createdActor)
	require.Equal(t, "arch", createdActor.Username)
	require.Equal(t, "arch", createdActor.Actor.PreferredUsername)
	require.Equal(t, "https://example.com/users/arch", createdActor.Actor.ID)
	require.Equal(t, "https://example.com/@arch", createdActor.Actor.URL)
}

package repositories

import (
	"context"
	"testing"
	"time"

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
}

func TestAccountRepository_GetUser_FallsBackToLegacyMixedCaseKeys(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, time.March, 23, 9, 0, 0, 0, time.UTC)

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	mockDB.On("WithContext", ctx).Return(mockDB).Maybe()
	mockDB.On("Model", mock.AnythingOfType("*models.User")).Return(mockQuery).Maybe()
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("First", mock.AnythingOfType("*models.User")).
		Return(dynamormErrors.ErrItemNotFound).
		Once()
	mockQuery.On("First", mock.AnythingOfType("*models.User")).
		Run(func(args mock.Arguments) {
			user := args.Get(0).(*models.User)
			user.Username = "Arch"
			user.Role = "user"
			user.CreatedAt = now
			user.UpdatedAt = now
			_ = user.UpdateKeys()
		}).
		Return(nil).
		Once()

	repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))

	user, err := repo.GetUser(ctx, "Arch")
	require.NoError(t, err)
	require.NotNil(t, user)
	require.Equal(t, "Arch", user.Username)
	mockQuery.AssertNumberOfCalls(t, "First", 2)
}

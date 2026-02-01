package auth

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/pkg/mocks"
	"go.uber.org/zap"
)

func TestAuthService_GetWalletByAddress_CallsAccountRepository(t *testing.T) {
	t.Parallel()

	db := new(mocks.MockDB)
	q := new(mocks.MockQuery)

	db.On("WithContext", mock.Anything).Return(db)
	db.On("Model", mock.Anything).Return(q)
	q.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(q)
	q.On("Limit", mock.Anything).Return(q)

	q.On("All", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]models.WalletIndex)
		*dest = []models.WalletIndex{{Username: "alice"}}
	})

	q.On("First", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*models.WalletCredential)
		*dest = models.WalletCredential{
			Username: "alice",
			Address:  "0xabc",
			ChainID:  1,
			Type:     "ethereum",
			LinkedAt: time.Now(),
			LastUsed: time.Now(),
		}
	})

	accountRepo := repositories.NewAccountRepository(db, "test-table", "example.com", zap.NewNop())

	as := &AuthService{repos: fakeAuthRepos{account: accountRepo}}
	cred, err := as.GetWalletByAddress(context.Background(), "0xAbC")
	require.NoError(t, err)
	require.Equal(t, "alice", cred.Username)
	require.Equal(t, "0xabc", cred.Address)
}

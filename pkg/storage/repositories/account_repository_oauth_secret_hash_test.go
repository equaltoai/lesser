package repositories

import (
	"context"
	"strings"
	"testing"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/v2/pkg/mocks"
	"go.uber.org/zap"
)

func TestAccountRepository_CreateOAuthClient_StoresHashedSecret(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	var createdModel *models.OAuthClient
	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.OAuthClient")).Return(mockQuery).Run(func(args mock.Arguments) {
		createdModel = args.Get(0).(*models.OAuthClient)
	})
	mockQuery.On("Create").Return(nil)

	repo := NewAccountRepository(mockDB, "test-table", "example.com", zap.NewNop())
	repo.SetValidationService(nil)
	repo.SetPermissionService(nil)
	repo.SetEventService(nil)
	repo.SetCachingService(nil)

	client := &storage.OAuthClient{
		Name:         "Test App",
		RedirectURIs: []string{"https://example.com/callback"},
	}

	require.NoError(t, repo.CreateOAuthClient(ctx, client))
	require.NotEmpty(t, client.ClientSecret)
	require.NotEmpty(t, client.ClientSecretHash)
	require.NotNil(t, createdModel)

	require.Equal(t, client.ClientSecretHash, createdModel.ClientSecret)
	require.True(t, strings.HasPrefix(createdModel.ClientSecret, common.OAuthClientSecretHashPrefix))
	require.NotEqual(t, client.ClientSecret, createdModel.ClientSecret)
}

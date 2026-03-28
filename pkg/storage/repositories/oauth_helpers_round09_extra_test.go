package repositories

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	dynamormerrors "github.com/theory-cloud/tabletheory/pkg/errors"
	"github.com/theory-cloud/tabletheory/pkg/mocks"
	"go.uber.org/zap"

	"github.com/equaltoai/lesser/pkg/storage"
)

func TestRound09_OAuthHelpers_DeleteStateAndClientBranches(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	mockDBStateNotFound := new(mocks.MockDB)
	mockQueryStateNotFound := new(mocks.MockQuery)
	mockQueryStateNotFound.On("Delete", mock.Anything).Return(dynamormerrors.ErrItemNotFound).Once()
	setupPermissiveRound08Mocks(mockDBStateNotFound, mockQueryStateNotFound, nil, time.Now().UTC())
	helperStateNotFound := NewOAuthHelper(mockDBStateNotFound, zap.NewNop())
	require.NoError(t, helperStateNotFound.DeleteOAuthStateGeneric(ctx, "missing"))

	mockDBStateErr := new(mocks.MockDB)
	mockQueryStateErr := new(mocks.MockQuery)
	mockQueryStateErr.On("Delete", mock.Anything).Return(errors.New("boom")).Once()
	setupPermissiveRound08Mocks(mockDBStateErr, mockQueryStateErr, nil, time.Now().UTC())
	helperStateErr := NewOAuthHelper(mockDBStateErr, zap.NewNop())
	require.Error(t, helperStateErr.DeleteOAuthStateGeneric(ctx, "state-err"))

	mockDBClientNotFound := new(mocks.MockDB)
	mockQueryClientNotFound := new(mocks.MockQuery)
	mockQueryClientNotFound.On("Delete", mock.Anything).Return(dynamormerrors.ErrItemNotFound).Once()
	setupPermissiveRound08Mocks(mockDBClientNotFound, mockQueryClientNotFound, nil, time.Now().UTC())
	helperClientNotFound := NewOAuthHelper(mockDBClientNotFound, zap.NewNop())
	require.NoError(t, helperClientNotFound.DeleteOAuthClientGeneric(ctx, "client-missing"))

	mockDBClientErr := new(mocks.MockDB)
	mockQueryClientErr := new(mocks.MockQuery)
	mockQueryClientErr.On("Delete", mock.Anything).Return(errors.New("boom")).Once()
	setupPermissiveRound08Mocks(mockDBClientErr, mockQueryClientErr, nil, time.Now().UTC())
	helperClientErr := NewOAuthHelper(mockDBClientErr, zap.NewNop())
	require.Error(t, helperClientErr.DeleteOAuthClientGeneric(ctx, "client-err"))
}

func TestRound09_OAuthHelpers_UserAppConsentBranches(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	consent := &storage.UserAppConsent{
		UserID:    "user-1",
		AppID:     "client-1",
		Scopes:    []string{"read"},
		CreatedAt: time.Now(),
	}

	mockDBHappy := new(mocks.MockDB)
	mockQueryHappy := new(mocks.MockQuery)
	setupPermissiveRound08Mocks(mockDBHappy, mockQueryHappy, nil, time.Now().UTC())
	helperHappy := NewOAuthHelper(mockDBHappy, zap.NewNop())
	require.NoError(t, helperHappy.SaveUserAppConsentGeneric(ctx, consent))

	got, err := helperHappy.GetUserAppConsentGeneric(ctx, consent.UserID, consent.AppID, "")
	require.NoError(t, err)
	require.Equal(t, consent.UserID, got.UserID)

	mockDBUpsertCreate := new(mocks.MockDB)
	mockQueryUpsertCreate := new(mocks.MockQuery)
	mockQueryUpsertCreate.On("Update", mock.Anything).Return(dynamormerrors.ErrItemNotFound).Once()
	mockQueryUpsertCreate.On("Create").Return(nil).Once()
	setupPermissiveRound08Mocks(mockDBUpsertCreate, mockQueryUpsertCreate, nil, time.Now().UTC())
	helperUpsertCreate := NewOAuthHelper(mockDBUpsertCreate, zap.NewNop())
	require.NoError(t, helperUpsertCreate.SaveUserAppConsentGeneric(ctx, &storage.UserAppConsent{
		UserID: "user-2",
		AppID:  "client-1",
		Scopes: []string{"read"},
	}))

	mockDBUpsertCreateErr := new(mocks.MockDB)
	mockQueryUpsertCreateErr := new(mocks.MockQuery)
	mockQueryUpsertCreateErr.On("Update", mock.Anything).Return(dynamormerrors.ErrItemNotFound).Once()
	mockQueryUpsertCreateErr.On("Create").Return(errors.New("boom")).Once()
	setupPermissiveRound08Mocks(mockDBUpsertCreateErr, mockQueryUpsertCreateErr, nil, time.Now().UTC())
	helperUpsertCreateErr := NewOAuthHelper(mockDBUpsertCreateErr, zap.NewNop())
	require.Error(t, helperUpsertCreateErr.SaveUserAppConsentGeneric(ctx, &storage.UserAppConsent{
		UserID: "user-3",
		AppID:  "client-1",
		Scopes: []string{"read"},
	}))

	mockDBGetNotFound := new(mocks.MockDB)
	mockQueryGetNotFound := new(mocks.MockQuery)
	mockQueryGetNotFound.On("First", mock.Anything).Return(dynamormerrors.ErrItemNotFound).Once()
	setupPermissiveRound08Mocks(mockDBGetNotFound, mockQueryGetNotFound, nil, time.Now().UTC())
	helperGetNotFound := NewOAuthHelper(mockDBGetNotFound, zap.NewNop())
	_, err = helperGetNotFound.GetUserAppConsentGeneric(ctx, "missing", "client-1", "")
	require.Error(t, err)

	mockDBUpdateErr := new(mocks.MockDB)
	mockQueryUpdateErr := new(mocks.MockQuery)
	mockQueryUpdateErr.On("Update", mock.Anything).Return(errors.New("boom")).Once()
	setupPermissiveRound08Mocks(mockDBUpdateErr, mockQueryUpdateErr, nil, time.Now().UTC())
	helperUpdateErr := NewOAuthHelper(mockDBUpdateErr, zap.NewNop())
	require.Error(t, helperUpdateErr.SaveUserAppConsentGeneric(ctx, &storage.UserAppConsent{
		UserID: "user-4",
		AppID:  "client-1",
		Scopes: []string{"read"},
	}))
}

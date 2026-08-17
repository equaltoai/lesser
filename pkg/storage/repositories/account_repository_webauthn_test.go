package repositories

import (
	"context"
	stdErrors "errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	dynamormerrors "github.com/theory-cloud/tabletheory/v3/pkg/errors"
	"github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	"go.uber.org/zap"
)

func TestAccountRepository_WebAuthnCanonicalKeyPredicates_GetByCredentialID(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", ctx).Return(mockDB).Once()
	mockDB.On("Model", mock.AnythingOfType("*models.WebAuthnCredential")).Return(mockQuery).Once()

	mockQuery.On("Index", "gsi1").Return(mockQuery).Once()
	mockQuery.On("Where", "gsi1PK", "=", "WEBAUTHN_CREDENTIAL#cred-123").Return(mockQuery).Once()
	mockQuery.On("Limit", 1).Return(mockQuery).Once()
	mockQuery.On("First", mock.AnythingOfType("*models.WebAuthnCredential")).Run(func(args mock.Arguments) {
		model := args.Get(0).(*models.WebAuthnCredential)
		model.ID = "cred-123"
		model.UserID = "alice"
		model.PK = "USER#alice"
		model.SK = "WEBAUTHN_CRED#cred-123"
		model.GSI1PK = "WEBAUTHN_CREDENTIAL#cred-123"
		model.GSI1SK = "USER#alice"
		model.PublicKey = []byte("pk")
		model.CreatedAt = time.Unix(100, 0).UTC()
		model.LastUsedAt = time.Unix(200, 0).UTC()
	}).Return(nil).Once()

	repo := NewAccountRepository(mockDB, "test-table", "example.com", zap.NewNop())
	repo.SetValidationService(nil)
	repo.SetPermissionService(nil)
	repo.SetEventService(nil)
	repo.SetCachingService(nil)

	cred, err := repo.GetWebAuthnCredential(ctx, "cred-123")
	require.NoError(t, err)
	require.Equal(t, &storage.WebAuthnCredential{
		ID:         "cred-123",
		UserID:     "alice",
		PublicKey:  []byte("pk"),
		CreatedAt:  time.Unix(100, 0).UTC(),
		LastUsedAt: time.Unix(200, 0).UTC(),
	}, cred)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestAccountRepository_WebAuthnCanonicalKeyPredicates_ListByUser(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", ctx).Return(mockDB).Once()
	mockDB.On("Model", mock.AnythingOfType("*models.WebAuthnCredential")).Return(mockQuery).Once()

	mockQuery.On("Where", "PK", "=", "USER#alice").Return(mockQuery).Once()
	mockQuery.On("Where", "SK", "BEGINS_WITH", "WEBAUTHN_CRED#").Return(mockQuery).Once()
	mockQuery.On("All", mock.AnythingOfType("*[]models.WebAuthnCredential")).Run(func(args mock.Arguments) {
		items := args.Get(0).(*[]models.WebAuthnCredential)
		*items = []models.WebAuthnCredential{
			{
				ID:     "cred-123",
				UserID: "alice",
				PK:     "USER#alice",
				SK:     "WEBAUTHN_CRED#cred-123",
				GSI1PK: "WEBAUTHN_CREDENTIAL#cred-123",
				GSI1SK: "USER#alice",
				Name:   "Laptop",
			},
			{
				ID:     "cred-456",
				UserID: "alice",
				PK:     "USER#alice",
				SK:     "WEBAUTHN_CRED#cred-456",
				GSI1PK: "WEBAUTHN_CREDENTIAL#cred-456",
				GSI1SK: "USER#alice",
				Name:   "Phone",
			},
		}
	}).Return(nil).Once()

	repo := NewAccountRepository(mockDB, "test-table", "example.com", zap.NewNop())
	repo.SetValidationService(nil)
	repo.SetPermissionService(nil)
	repo.SetEventService(nil)
	repo.SetCachingService(nil)

	creds, err := repo.GetUserWebAuthnCredentials(ctx, "alice")
	require.NoError(t, err)
	require.Len(t, creds, 2)
	require.Equal(t, "cred-123", creds[0].ID)
	require.Equal(t, "alice", creds[0].UserID)
	require.Equal(t, "cred-456", creds[1].ID)
	require.Equal(t, "alice", creds[1].UserID)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestAccountRepository_WebAuthnCanonicalKeyPredicates_DeleteByCredentialID(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	lookupQuery := new(mocks.MockQuery)
	deleteQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", ctx).Return(mockDB).Twice()
	mockDB.On("Model", mock.AnythingOfType("*models.WebAuthnCredential")).Return(lookupQuery).Once()
	mockDB.On("Model", mock.AnythingOfType("*models.WebAuthnCredential")).Return(deleteQuery).Once()

	lookupQuery.On("Index", "gsi1").Return(lookupQuery).Once()
	lookupQuery.On("Where", "gsi1PK", "=", "WEBAUTHN_CREDENTIAL#cred-123").Return(lookupQuery).Once()
	lookupQuery.On("Limit", 1).Return(lookupQuery).Once()
	lookupQuery.On("First", mock.AnythingOfType("*models.WebAuthnCredential")).Run(func(args mock.Arguments) {
		model := args.Get(0).(*models.WebAuthnCredential)
		model.ID = "cred-123"
		model.UserID = "alice"
		model.PK = "USER#alice"
		model.SK = "WEBAUTHN_CRED#cred-123"
		model.GSI1PK = "WEBAUTHN_CREDENTIAL#cred-123"
		model.GSI1SK = "USER#alice"
	}).Return(nil).Once()

	deleteQuery.On("Where", "PK", "=", "USER#alice").Return(deleteQuery).Once()
	deleteQuery.On("Where", "SK", "=", "WEBAUTHN_CRED#cred-123").Return(deleteQuery).Once()
	deleteQuery.On("Delete").Return(nil).Once()

	repo := NewAccountRepository(mockDB, "test-table", "example.com", zap.NewNop())
	repo.SetValidationService(nil)
	repo.SetPermissionService(nil)
	repo.SetEventService(nil)
	repo.SetCachingService(nil)

	require.NoError(t, repo.DeleteWebAuthnCredential(ctx, "cred-123"))

	mockDB.AssertExpectations(t)
	lookupQuery.AssertExpectations(t)
	deleteQuery.AssertExpectations(t)
}

func TestAccountRepository_WebAuthnCanonicalKeyPredicates_UpdateLastUsedByCredentialID(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	lookupQuery := new(mocks.MockQuery)
	updateQuery := new(mocks.MockQuery)
	updateBuilder := new(mocks.MockUpdateBuilder)

	mockDB.On("WithContext", ctx).Return(mockDB).Twice()
	mockDB.On("Model", mock.AnythingOfType("*models.WebAuthnCredential")).Return(lookupQuery).Once()
	mockDB.On("Model", mock.MatchedBy(func(v any) bool {
		model, ok := v.(*models.WebAuthnCredential)
		if !ok {
			return false
		}
		return model.PK == "USER#alice" &&
			model.SK == "WEBAUTHN_CRED#cred-123" &&
			model.GSI1PK == "WEBAUTHN_CREDENTIAL#cred-123" &&
			model.GSI1SK == "USER#alice" &&
			model.ID == "cred-123"
	})).Return(updateQuery).Once()

	lookupQuery.On("Index", "gsi1").Return(lookupQuery).Once()
	lookupQuery.On("Where", "gsi1PK", "=", "WEBAUTHN_CREDENTIAL#cred-123").Return(lookupQuery).Once()
	lookupQuery.On("Limit", 1).Return(lookupQuery).Once()
	lookupQuery.On("First", mock.AnythingOfType("*models.WebAuthnCredential")).Run(func(args mock.Arguments) {
		model := args.Get(0).(*models.WebAuthnCredential)
		model.ID = "cred-123"
		model.UserID = "alice"
		model.PK = "USER#alice"
		model.SK = "WEBAUTHN_CRED#cred-123"
		model.GSI1PK = "WEBAUTHN_CREDENTIAL#cred-123"
		model.GSI1SK = "USER#alice"
		model.PublicKey = []byte("pk")
		model.CloneWarning = true
		model.BackupState = true
		model.CreatedAt = time.Unix(100, 0).UTC()
		model.LastUsedAt = time.Unix(200, 0).UTC()
	}).Return(nil).Once()

	updateQuery.On("Where", "PK", "=", "USER#alice").Return(updateQuery).Once()
	updateQuery.On("Where", "SK", "=", "WEBAUTHN_CRED#cred-123").Return(updateQuery).Once()
	updateQuery.On("UpdateBuilder").Return(updateBuilder).Once()
	updateBuilder.On("Set", "SignCount", uint32(9)).Return(updateBuilder).Once()
	updateBuilder.On("Set", "LastUsedAt", mock.AnythingOfType("time.Time")).Return(updateBuilder).Once()
	updateBuilder.On("Execute").Return(nil).Once()

	repo := NewAccountRepository(mockDB, "test-table", "example.com", zap.NewNop())
	repo.SetValidationService(nil)
	repo.SetPermissionService(nil)
	repo.SetEventService(nil)
	repo.SetCachingService(nil)

	require.NoError(t, repo.UpdateWebAuthnLastUsed(ctx, "cred-123", 9))

	mockDB.AssertExpectations(t)
	lookupQuery.AssertExpectations(t)
	updateQuery.AssertExpectations(t)
	updateBuilder.AssertExpectations(t)
}

func TestAccountRepository_WebAuthnCanonicalKeyPredicates_UpdateNameByCredentialID(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	lookupQuery := new(mocks.MockQuery)
	updateQuery := new(mocks.MockQuery)
	updateBuilder := new(mocks.MockUpdateBuilder)

	mockDB.On("WithContext", ctx).Return(mockDB).Twice()
	mockDB.On("Model", mock.AnythingOfType("*models.WebAuthnCredential")).Return(lookupQuery).Once()
	mockDB.On("Model", mock.MatchedBy(func(v any) bool {
		model, ok := v.(*models.WebAuthnCredential)
		if !ok {
			return false
		}
		return model.PK == "USER#alice" &&
			model.SK == "WEBAUTHN_CRED#cred-123" &&
			model.GSI1PK == "WEBAUTHN_CREDENTIAL#cred-123" &&
			model.GSI1SK == "USER#alice" &&
			model.ID == "cred-123"
	})).Return(updateQuery).Once()

	lookupQuery.On("Index", "gsi1").Return(lookupQuery).Once()
	lookupQuery.On("Where", "gsi1PK", "=", "WEBAUTHN_CREDENTIAL#cred-123").Return(lookupQuery).Once()
	lookupQuery.On("Limit", 1).Return(lookupQuery).Once()
	lookupQuery.On("First", mock.AnythingOfType("*models.WebAuthnCredential")).Run(func(args mock.Arguments) {
		model := args.Get(0).(*models.WebAuthnCredential)
		model.ID = "cred-123"
		model.UserID = "alice"
		model.PK = "USER#alice"
		model.SK = "WEBAUTHN_CRED#cred-123"
		model.GSI1PK = "WEBAUTHN_CREDENTIAL#cred-123"
		model.GSI1SK = "USER#alice"
		model.Name = "old"
		model.LastUsedAt = time.Unix(200, 0).UTC()
	}).Return(nil).Once()

	updateQuery.On("Where", "PK", "=", "USER#alice").Return(updateQuery).Once()
	updateQuery.On("Where", "SK", "=", "WEBAUTHN_CRED#cred-123").Return(updateQuery).Once()
	updateQuery.On("UpdateBuilder").Return(updateBuilder).Once()
	updateBuilder.On("Set", "Name", "Renamed Key").Return(updateBuilder).Once()
	updateBuilder.On("Execute").Return(nil).Once()

	repo := NewAccountRepository(mockDB, "test-table", "example.com", zap.NewNop())
	repo.SetValidationService(nil)
	repo.SetPermissionService(nil)
	repo.SetEventService(nil)
	repo.SetCachingService(nil)

	require.NoError(t, repo.UpdateWebAuthnCredentialName(ctx, "cred-123", "Renamed Key"))

	mockDB.AssertExpectations(t)
	lookupQuery.AssertExpectations(t)
	updateQuery.AssertExpectations(t)
	updateBuilder.AssertExpectations(t)
}

func TestAccountRepository_WebAuthnCanonicalKeyPredicates_UpdateAuthenticationStateByCredentialID(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	lookupQuery := new(mocks.MockQuery)
	updateQuery := new(mocks.MockQuery)
	updateBuilder := new(mocks.MockUpdateBuilder)
	lastUsedAt := time.Unix(300, 0).UTC()

	mockDB.On("WithContext", ctx).Return(mockDB).Twice()
	mockDB.On("Model", mock.AnythingOfType("*models.WebAuthnCredential")).Return(lookupQuery).Once()
	mockDB.On("Model", mock.MatchedBy(func(v any) bool {
		model, ok := v.(*models.WebAuthnCredential)
		if !ok {
			return false
		}
		return model.PK == "USER#alice" &&
			model.SK == "WEBAUTHN_CRED#cred-123" &&
			model.GSI1PK == "WEBAUTHN_CREDENTIAL#cred-123" &&
			model.GSI1SK == "USER#alice" &&
			model.ID == "cred-123"
	})).Return(updateQuery).Once()

	lookupQuery.On("Index", "gsi1").Return(lookupQuery).Once()
	lookupQuery.On("Where", "gsi1PK", "=", "WEBAUTHN_CREDENTIAL#cred-123").Return(lookupQuery).Once()
	lookupQuery.On("Limit", 1).Return(lookupQuery).Once()
	lookupQuery.On("First", mock.AnythingOfType("*models.WebAuthnCredential")).Run(func(args mock.Arguments) {
		model := args.Get(0).(*models.WebAuthnCredential)
		model.ID = "cred-123"
		model.UserID = "alice"
		model.PK = "USER#alice"
		model.SK = "WEBAUTHN_CRED#cred-123"
		model.GSI1PK = "WEBAUTHN_CREDENTIAL#cred-123"
		model.GSI1SK = "USER#alice"
		model.SignCount = 1
	}).Return(nil).Once()

	updateQuery.On("Where", "PK", "=", "USER#alice").Return(updateQuery).Once()
	updateQuery.On("Where", "SK", "=", "WEBAUTHN_CRED#cred-123").Return(updateQuery).Once()
	updateQuery.On("UpdateBuilder").Return(updateBuilder).Once()
	updateBuilder.On("Set", "SignCount", uint32(7)).Return(updateBuilder).Once()
	updateBuilder.On("Set", "CloneWarning", true).Return(updateBuilder).Once()
	updateBuilder.On("Set", "BackupState", true).Return(updateBuilder).Once()
	updateBuilder.On("Set", "LastUsedAt", lastUsedAt).Return(updateBuilder).Once()
	updateBuilder.On("Execute").Return(nil).Once()

	repo := NewAccountRepository(mockDB, "test-table", "example.com", zap.NewNop())
	repo.SetValidationService(nil)
	repo.SetPermissionService(nil)
	repo.SetEventService(nil)
	repo.SetCachingService(nil)

	require.NoError(t, repo.UpdateWebAuthnAuthenticationState(ctx, "cred-123", 7, true, true, lastUsedAt))

	mockDB.AssertExpectations(t)
	lookupQuery.AssertExpectations(t)
	updateQuery.AssertExpectations(t)
	updateBuilder.AssertExpectations(t)
}

func TestAccountRepository_WebAuthnCanonicalKeyPredicates_UpdateNameByCredentialID_ErrorPaths(t *testing.T) {
	t.Parallel()

	t.Run("lookup not found", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		mockDB := new(mocks.MockDB)
		lookupQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", ctx).Return(mockDB).Once()
		mockDB.On("Model", mock.AnythingOfType("*models.WebAuthnCredential")).Return(lookupQuery).Once()

		lookupQuery.On("Index", "gsi1").Return(lookupQuery).Once()
		lookupQuery.On("Where", "gsi1PK", "=", "WEBAUTHN_CREDENTIAL#missing").Return(lookupQuery).Once()
		lookupQuery.On("Limit", 1).Return(lookupQuery).Once()
		lookupQuery.On("First", mock.AnythingOfType("*models.WebAuthnCredential")).Return(dynamormerrors.ErrItemNotFound).Once()

		repo := NewAccountRepository(mockDB, "test-table", "example.com", zap.NewNop())
		repo.SetValidationService(nil)
		repo.SetPermissionService(nil)
		repo.SetEventService(nil)
		repo.SetCachingService(nil)

		err := repo.UpdateWebAuthnCredentialName(ctx, "missing", "Renamed Key")
		require.Error(t, err)
		require.True(t, dynamormerrors.IsNotFound(err))

		mockDB.AssertExpectations(t)
		lookupQuery.AssertExpectations(t)
	})

	t.Run("update execute error", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		mockDB := new(mocks.MockDB)
		lookupQuery := new(mocks.MockQuery)
		updateQuery := new(mocks.MockQuery)
		updateBuilder := new(mocks.MockUpdateBuilder)

		mockDB.On("WithContext", ctx).Return(mockDB).Twice()
		mockDB.On("Model", mock.AnythingOfType("*models.WebAuthnCredential")).Return(lookupQuery).Once()
		mockDB.On("Model", mock.MatchedBy(func(v any) bool {
			model, ok := v.(*models.WebAuthnCredential)
			if !ok {
				return false
			}
			return model.PK == "USER#alice" &&
				model.SK == "WEBAUTHN_CRED#cred-123" &&
				model.GSI1PK == "WEBAUTHN_CREDENTIAL#cred-123" &&
				model.GSI1SK == "USER#alice" &&
				model.ID == "cred-123"
		})).Return(updateQuery).Once()

		lookupQuery.On("Index", "gsi1").Return(lookupQuery).Once()
		lookupQuery.On("Where", "gsi1PK", "=", "WEBAUTHN_CREDENTIAL#cred-123").Return(lookupQuery).Once()
		lookupQuery.On("Limit", 1).Return(lookupQuery).Once()
		lookupQuery.On("First", mock.AnythingOfType("*models.WebAuthnCredential")).Run(func(args mock.Arguments) {
			model := args.Get(0).(*models.WebAuthnCredential)
			model.ID = "cred-123"
			model.UserID = "alice"
		}).Return(nil).Once()

		updateQuery.On("Where", "PK", "=", "USER#alice").Return(updateQuery).Once()
		updateQuery.On("Where", "SK", "=", "WEBAUTHN_CRED#cred-123").Return(updateQuery).Once()
		updateQuery.On("UpdateBuilder").Return(updateBuilder).Once()
		updateBuilder.On("Set", "Name", "Renamed Key").Return(updateBuilder).Once()
		updateBuilder.On("Execute").Return(stdErrors.New("boom")).Once()

		repo := NewAccountRepository(mockDB, "test-table", "example.com", zap.NewNop())
		repo.SetValidationService(nil)
		repo.SetPermissionService(nil)
		repo.SetEventService(nil)
		repo.SetCachingService(nil)

		err := repo.UpdateWebAuthnCredentialName(ctx, "cred-123", "Renamed Key")
		require.Error(t, err)
		require.False(t, dynamormerrors.IsNotFound(err))

		mockDB.AssertExpectations(t)
		lookupQuery.AssertExpectations(t)
		updateQuery.AssertExpectations(t)
		updateBuilder.AssertExpectations(t)
	})
}

func TestAccountRepository_WebAuthnCanonicalKeyPredicates_UpdateAuthenticationStateByCredentialID_ErrorPaths(t *testing.T) {
	t.Parallel()

	t.Run("lookup not found", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		mockDB := new(mocks.MockDB)
		lookupQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", ctx).Return(mockDB).Once()
		mockDB.On("Model", mock.AnythingOfType("*models.WebAuthnCredential")).Return(lookupQuery).Once()

		lookupQuery.On("Index", "gsi1").Return(lookupQuery).Once()
		lookupQuery.On("Where", "gsi1PK", "=", "WEBAUTHN_CREDENTIAL#missing").Return(lookupQuery).Once()
		lookupQuery.On("Limit", 1).Return(lookupQuery).Once()
		lookupQuery.On("First", mock.AnythingOfType("*models.WebAuthnCredential")).Return(dynamormerrors.ErrItemNotFound).Once()

		repo := NewAccountRepository(mockDB, "test-table", "example.com", zap.NewNop())
		repo.SetValidationService(nil)
		repo.SetPermissionService(nil)
		repo.SetEventService(nil)
		repo.SetCachingService(nil)

		err := repo.UpdateWebAuthnAuthenticationState(ctx, "missing", 7, true, true, time.Unix(300, 0).UTC())
		require.Error(t, err)
		require.True(t, dynamormerrors.IsNotFound(err))

		mockDB.AssertExpectations(t)
		lookupQuery.AssertExpectations(t)
	})

	t.Run("update execute error", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		mockDB := new(mocks.MockDB)
		lookupQuery := new(mocks.MockQuery)
		updateQuery := new(mocks.MockQuery)
		updateBuilder := new(mocks.MockUpdateBuilder)
		lastUsedAt := time.Unix(300, 0).UTC()

		mockDB.On("WithContext", ctx).Return(mockDB).Twice()
		mockDB.On("Model", mock.AnythingOfType("*models.WebAuthnCredential")).Return(lookupQuery).Once()
		mockDB.On("Model", mock.MatchedBy(func(v any) bool {
			model, ok := v.(*models.WebAuthnCredential)
			if !ok {
				return false
			}
			return model.PK == "USER#alice" &&
				model.SK == "WEBAUTHN_CRED#cred-123" &&
				model.GSI1PK == "WEBAUTHN_CREDENTIAL#cred-123" &&
				model.GSI1SK == "USER#alice" &&
				model.ID == "cred-123"
		})).Return(updateQuery).Once()

		lookupQuery.On("Index", "gsi1").Return(lookupQuery).Once()
		lookupQuery.On("Where", "gsi1PK", "=", "WEBAUTHN_CREDENTIAL#cred-123").Return(lookupQuery).Once()
		lookupQuery.On("Limit", 1).Return(lookupQuery).Once()
		lookupQuery.On("First", mock.AnythingOfType("*models.WebAuthnCredential")).Run(func(args mock.Arguments) {
			model := args.Get(0).(*models.WebAuthnCredential)
			model.ID = "cred-123"
			model.UserID = "alice"
		}).Return(nil).Once()

		updateQuery.On("Where", "PK", "=", "USER#alice").Return(updateQuery).Once()
		updateQuery.On("Where", "SK", "=", "WEBAUTHN_CRED#cred-123").Return(updateQuery).Once()
		updateQuery.On("UpdateBuilder").Return(updateBuilder).Once()
		updateBuilder.On("Set", "SignCount", uint32(7)).Return(updateBuilder).Once()
		updateBuilder.On("Set", "CloneWarning", true).Return(updateBuilder).Once()
		updateBuilder.On("Set", "BackupState", true).Return(updateBuilder).Once()
		updateBuilder.On("Set", "LastUsedAt", lastUsedAt).Return(updateBuilder).Once()
		updateBuilder.On("Execute").Return(stdErrors.New("boom")).Once()

		repo := NewAccountRepository(mockDB, "test-table", "example.com", zap.NewNop())
		repo.SetValidationService(nil)
		repo.SetPermissionService(nil)
		repo.SetEventService(nil)
		repo.SetCachingService(nil)

		err := repo.UpdateWebAuthnAuthenticationState(ctx, "cred-123", 7, true, true, lastUsedAt)
		require.Error(t, err)
		require.False(t, dynamormerrors.IsNotFound(err))

		mockDB.AssertExpectations(t)
		lookupQuery.AssertExpectations(t)
		updateQuery.AssertExpectations(t)
		updateBuilder.AssertExpectations(t)
	})
}

func TestAccountRepository_WebAuthnHelperBranches(t *testing.T) {
	t.Parallel()

	t.Run("nil helpers return safely", func(t *testing.T) {
		t.Parallel()

		require.Nil(t, webAuthnCredentialFromModel(nil))
		require.NotPanics(t, func() {
			ensureWebAuthnCredentialCanonicalKeys(nil)
		})
		require.ErrorIs(
			t,
			persistWebAuthnCredentialAuthenticationState(context.Background(), nil, nil, 1, true, false, time.Time{}),
			storage.ErrInvalidInput,
		)
	})

	t.Run("zero last used at is normalized and canonical keys are applied", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		mockDB := new(mocks.MockDB)
		updateQuery := new(mocks.MockQuery)
		updateBuilder := new(mocks.MockUpdateBuilder)

		mockDB.On("WithContext", ctx).Return(mockDB).Once()
		mockDB.On("Model", mock.MatchedBy(func(v any) bool {
			model, ok := v.(*models.WebAuthnCredential)
			if !ok {
				return false
			}
			return model.ID == "cred-123" &&
				model.UserID == "alice" &&
				model.PK == "USER#alice" &&
				model.SK == "WEBAUTHN_CRED#cred-123" &&
				model.GSI1PK == "WEBAUTHN_CREDENTIAL#cred-123" &&
				model.GSI1SK == "USER#alice"
		})).Return(updateQuery).Once()

		updateQuery.On("Where", "PK", "=", "USER#alice").Return(updateQuery).Once()
		updateQuery.On("Where", "SK", "=", "WEBAUTHN_CRED#cred-123").Return(updateQuery).Once()
		updateQuery.On("UpdateBuilder").Return(updateBuilder).Once()
		updateBuilder.On("Set", "SignCount", uint32(7)).Return(updateBuilder).Once()
		updateBuilder.On("Set", "CloneWarning", true).Return(updateBuilder).Once()
		updateBuilder.On("Set", "BackupState", false).Return(updateBuilder).Once()
		updateBuilder.On("Set", "LastUsedAt", mock.MatchedBy(func(ts time.Time) bool {
			return !ts.IsZero() && ts.Equal(ts.UTC())
		})).Return(updateBuilder).Once()
		updateBuilder.On("Execute").Return(nil).Once()

		credential := &models.WebAuthnCredential{ID: "cred-123", UserID: "alice"}
		require.NoError(t, persistWebAuthnCredentialAuthenticationState(ctx, mockDB, credential, 7, true, false, time.Time{}))
		require.Equal(t, "USER#alice", credential.PK)
		require.Equal(t, "WEBAUTHN_CRED#cred-123", credential.SK)
		require.Equal(t, "WEBAUTHN_CREDENTIAL#cred-123", credential.GSI1PK)
		require.Equal(t, "USER#alice", credential.GSI1SK)

		mockDB.AssertExpectations(t)
		updateQuery.AssertExpectations(t)
		updateBuilder.AssertExpectations(t)
	})
}

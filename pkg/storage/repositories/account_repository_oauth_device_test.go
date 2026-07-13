package repositories

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	dynamormerrors "github.com/theory-cloud/tabletheory/v2/pkg/errors"
	dynamock "github.com/theory-cloud/tabletheory/v2/pkg/mocks"
	"go.uber.org/zap/zaptest"
)

func TestAccountRepository_OAuthDeviceSession_Create_Validation(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	mockDB := new(dynamock.MockDB)
	repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))

	require.ErrorIs(t, repo.CreateOAuthDeviceSession(ctx, nil), storage.ErrInvalidInput)

	require.ErrorIs(t, repo.CreateOAuthDeviceSession(ctx, &storage.OAuthDeviceSession{
		DeviceCodeHash: "",
		UserCode:       "UC",
		ClientID:       "client-1",
		Status:         "pending",
		ExpiresAt:      time.Now().Add(time.Minute),
	}), storage.ErrInvalidInput)

	require.ErrorIs(t, repo.CreateOAuthDeviceSession(ctx, &storage.OAuthDeviceSession{
		DeviceCodeHash: "hash-1",
		UserCode:       "UC",
		ClientID:       "client-1",
		Status:         "pending",
	}), storage.ErrInvalidInput)
}

func TestAccountRepository_OAuthDeviceSession_Create_SuccessAndDuplicate(t *testing.T) {
	t.Parallel()

	baseTime := time.Now().UTC()
	ctx := context.Background()

	{
		mockDB := new(dynamock.MockDB)
		mockQuery := new(dynamock.MockQuery)
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))
		session := &storage.OAuthDeviceSession{
			DeviceCodeHash:  " hash-1 ",
			UserCode:        " UC-1 ",
			ClientID:        " client-1 ",
			Scopes:          []string{"read"},
			Status:          " pending ",
			IntervalSeconds: 10,
			ExpiresAt:       baseTime.Add(10 * time.Minute),
		}

		require.NoError(t, repo.CreateOAuthDeviceSession(ctx, session))
		require.Equal(t, "hash-1", session.DeviceCodeHash)
		require.Equal(t, "UC-1", session.UserCode)
		require.Equal(t, "client-1", session.ClientID)
		require.Equal(t, "pending", session.Status)
		require.False(t, session.CreatedAt.IsZero())
		require.False(t, session.UpdatedAt.IsZero())
	}

	{
		mockDB := new(dynamock.MockDB)
		mockQuery := new(dynamock.MockQuery)
		mockQuery.On("Create").Return(errors.New("ConditionalCheckFailed")).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))
		err := repo.CreateOAuthDeviceSession(ctx, &storage.OAuthDeviceSession{
			DeviceCodeHash: "dup-hash",
			UserCode:       "UC-2",
			ClientID:       "client-1",
			Status:         "pending",
			ExpiresAt:      baseTime.Add(time.Minute),
		})
		require.ErrorIs(t, err, storage.ErrAlreadyExists)
	}
}

func TestAccountRepository_OAuthDeviceSession_Get_ByDeviceCodeHash(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	baseTime := time.Now().UTC()
	mockDB := new(dynamock.MockDB)
	mockQuery := new(dynamock.MockQuery)
	repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))

	_, err := repo.GetOAuthDeviceSession(ctx, "")
	require.ErrorIs(t, err, storage.ErrInvalidInput)

	{
		mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Where", "PK", "=", "OAUTH_DEVICE#hash-missing").Return(mockQuery).Once()
		mockQuery.On("Where", "SK", "=", oauthDeviceSessionSK).Return(mockQuery).Once()
		mockQuery.On("First", mock.Anything).Return(dynamormerrors.ErrItemNotFound).Once()

		_, err := repo.GetOAuthDeviceSession(ctx, "hash-missing")
		require.ErrorIs(t, err, storage.ErrNotFound)
	}

	{
		lastPolled := baseTimePtr(baseTime)
		approvedAt := baseTimePtr(baseTime.Add(time.Minute))
		deniedAt := baseTimePtr(baseTime.Add(2 * time.Minute))
		consumedAt := baseTimePtr(baseTime.Add(3 * time.Minute))

		mockDB2 := new(dynamock.MockDB)
		mockQuery2 := new(dynamock.MockQuery)
		repo2 := NewAccountRepository(mockDB2, "test-table", "example.com", zaptest.NewLogger(t))

		mockDB2.On("WithContext", mock.Anything).Return(mockDB2).Once()
		mockDB2.On("Model", mock.Anything).Return(mockQuery2).Once()
		mockQuery2.On("Where", "PK", "=", "OAUTH_DEVICE#hash-1").Return(mockQuery2).Once()
		mockQuery2.On("Where", "SK", "=", oauthDeviceSessionSK).Return(mockQuery2).Once()
		mockQuery2.On("First", mock.Anything).Run(func(args mock.Arguments) {
			target := args.Get(0).(*models.OAuthDeviceSession)
			target.DeviceCodeHash = "hash-1"
			target.UserCode = "UC-1"
			target.ClientID = "client-1"
			target.Scopes = []string{"read", "write"}
			target.Status = "approved"
			target.IntervalSeconds = 10
			target.PollCount = 3
			target.LastPolledAt = lastPolled
			target.ApprovedUsername = "alice"
			target.ApprovedAt = approvedAt
			target.DeniedAt = deniedAt
			target.ConsumedAt = consumedAt
			target.CreatedAt = baseTime.Add(-time.Minute)
			target.UpdatedAt = baseTime
			target.ExpiresAt = baseTime.Add(10 * time.Minute)
		}).Return(nil).Once()

		session, err := repo2.GetOAuthDeviceSession(ctx, "hash-1")
		require.NoError(t, err)
		require.Equal(t, "hash-1", session.DeviceCodeHash)
		require.Equal(t, "UC-1", session.UserCode)
		require.Equal(t, "client-1", session.ClientID)
		require.Equal(t, "approved", session.Status)
		require.Equal(t, "alice", session.ApprovedUsername)
		require.Equal(t, 3, session.PollCount)
		require.False(t, session.LastPolledAt.IsZero())
		require.False(t, session.ApprovedAt.IsZero())
		require.False(t, session.DeniedAt.IsZero())
		require.False(t, session.ConsumedAt.IsZero())
	}
}

func TestAccountRepository_OAuthDeviceSession_Get_ByUserCode(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	baseTime := time.Now().UTC()

	{
		mockDB := new(dynamock.MockDB)
		repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))
		_, err := repo.GetOAuthDeviceSessionByUserCode(ctx, "")
		require.ErrorIs(t, err, storage.ErrInvalidInput)
	}

	{
		mockDB := new(dynamock.MockDB)
		mockQuery := new(dynamock.MockQuery)
		repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Index", models.IndexGSI1).Return(mockQuery).Once()
		mockQuery.On("Where", "gsi1PK", "=", "OAUTH_DEVICE_USER_CODE#UC-1").Return(mockQuery).Once()
		mockQuery.On("Limit", 2).Return(mockQuery).Once()
		mockQuery.On("All", mock.Anything).Return(dynamormerrors.ErrItemNotFound).Once()

		_, err := repo.GetOAuthDeviceSessionByUserCode(ctx, "UC-1")
		require.ErrorIs(t, err, storage.ErrNotFound)
	}

	{
		mockDB := new(dynamock.MockDB)
		mockQuery := new(dynamock.MockQuery)
		repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Index", models.IndexGSI1).Return(mockQuery).Once()
		mockQuery.On("Where", "gsi1PK", "=", "OAUTH_DEVICE_USER_CODE#UC-2").Return(mockQuery).Once()
		mockQuery.On("Limit", 2).Return(mockQuery).Once()
		mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]models.OAuthDeviceSession)
			*dest = []models.OAuthDeviceSession{
				{
					DeviceCodeHash: "hash-2",
					UserCode:       "UC-2",
					ClientID:       "client-1",
					Status:         "pending",
					CreatedAt:      baseTime,
					UpdatedAt:      baseTime,
					ExpiresAt:      baseTime.Add(10 * time.Minute),
				},
				{
					DeviceCodeHash: "hash-2b",
					UserCode:       "UC-2",
					ClientID:       "client-1",
					Status:         "pending",
					CreatedAt:      baseTime,
					UpdatedAt:      baseTime,
					ExpiresAt:      baseTime.Add(10 * time.Minute),
				},
			}
		}).Return(nil).Once()

		session, err := repo.GetOAuthDeviceSessionByUserCode(ctx, "UC-2")
		require.NoError(t, err)
		require.Equal(t, "hash-2", session.DeviceCodeHash)
		require.Equal(t, "UC-2", session.UserCode)
	}
}

func TestAccountRepository_OAuthDeviceSession_Update(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	baseTime := time.Now().UTC()

	mockDB := new(dynamock.MockDB)
	repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))

	require.ErrorIs(t, repo.UpdateOAuthDeviceSession(ctx, nil), storage.ErrInvalidInput)
	require.ErrorIs(t, repo.UpdateOAuthDeviceSession(ctx, &storage.OAuthDeviceSession{}), storage.ErrInvalidInput)

	{
		mockDB2 := new(dynamock.MockDB)
		mockQuery2 := new(dynamock.MockQuery)
		setupPermissiveRound08Mocks(mockDB2, mockQuery2, nil, baseTime)
		repo2 := NewAccountRepository(mockDB2, "test-table", "example.com", zaptest.NewLogger(t))

		session := &storage.OAuthDeviceSession{
			DeviceCodeHash: "hash-3",
			UserCode:       "UC-3",
			ClientID:       "client-1",
			Status:         "pending",
			ExpiresAt:      baseTime.Add(10 * time.Minute),
		}
		require.NoError(t, repo2.UpdateOAuthDeviceSession(ctx, session))
		require.False(t, session.UpdatedAt.IsZero())
	}

	{
		mockDB3 := new(dynamock.MockDB)
		mockQuery3 := new(dynamock.MockQuery)
		mockDB3.On("WithContext", mock.Anything).Return(mockDB3).Once()
		mockDB3.On("Model", mock.Anything).Return(mockQuery3).Once()
		mockQuery3.On("Update", mock.Anything).Return(errors.New("boom")).Once()
		repo3 := NewAccountRepository(mockDB3, "test-table", "example.com", zaptest.NewLogger(t))

		err := repo3.UpdateOAuthDeviceSession(ctx, &storage.OAuthDeviceSession{
			DeviceCodeHash: "hash-4",
			UserCode:       "UC-4",
			ClientID:       "client-1",
			Status:         "pending",
			ExpiresAt:      baseTime.Add(10 * time.Minute),
		})
		require.Error(t, err)
	}
}

func TestOAuthDeviceSessionStorageModelConversions(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()

	require.Nil(t, oauthDeviceSessionModelFromStorage(nil))
	require.Nil(t, oauthDeviceSessionStorageFromModel(nil))

	st := &storage.OAuthDeviceSession{
		DeviceCodeHash:   "hash",
		UserCode:         "UC",
		ClientID:         "client",
		Scopes:           []string{"read"},
		Status:           "approved",
		IntervalSeconds:  10,
		PollCount:        2,
		LastPolledAt:     now,
		ApprovedUsername: "alice",
		ApprovedAt:       now,
		DeniedAt:         now,
		ConsumedAt:       now,
		CreatedAt:        now,
		UpdatedAt:        now,
		ExpiresAt:        now.Add(time.Minute),
	}

	model := oauthDeviceSessionModelFromStorage(st)
	require.NotNil(t, model)
	require.NotNil(t, model.LastPolledAt)
	require.NotNil(t, model.ApprovedAt)
	require.NotNil(t, model.DeniedAt)
	require.NotNil(t, model.ConsumedAt)

	roundTrip := oauthDeviceSessionStorageFromModel(model)
	require.NotNil(t, roundTrip)
	require.Equal(t, st.DeviceCodeHash, roundTrip.DeviceCodeHash)
	require.Equal(t, st.UserCode, roundTrip.UserCode)
	require.Equal(t, st.ClientID, roundTrip.ClientID)
	require.Equal(t, st.PollCount, roundTrip.PollCount)
	require.False(t, roundTrip.LastPolledAt.IsZero())
	require.False(t, roundTrip.ApprovedAt.IsZero())
	require.False(t, roundTrip.DeniedAt.IsZero())
	require.False(t, roundTrip.ConsumedAt.IsZero())
}

func baseTimePtr(t time.Time) *time.Time {
	return &t
}

func TestAccountRepository_OAuthDeviceSession_Create_DBError(t *testing.T) {
	t.Parallel()

	baseTime := time.Now().UTC()
	ctx := context.Background()

	mockDB := new(dynamock.MockDB)
	mockQuery := new(dynamock.MockQuery)
	mockQuery.On("Create").Return(errors.New("boom")).Once()
	setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

	repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))
	err := repo.CreateOAuthDeviceSession(ctx, &storage.OAuthDeviceSession{
		DeviceCodeHash: "hash-err",
		UserCode:       "UC-ERR",
		ClientID:       "client-1",
		Status:         "pending",
		ExpiresAt:      baseTime.Add(time.Minute),
	})
	require.Error(t, err)
}

func TestAccountRepository_OAuthDeviceSession_Get_DBError(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	mockDB := new(dynamock.MockDB)
	mockQuery := new(dynamock.MockQuery)
	repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))

	mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
	mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
	mockQuery.On("Where", "PK", "=", "OAUTH_DEVICE#hash-err").Return(mockQuery).Once()
	mockQuery.On("Where", "SK", "=", oauthDeviceSessionSK).Return(mockQuery).Once()
	mockQuery.On("First", mock.Anything).Return(errors.New("boom")).Once()

	_, err := repo.GetOAuthDeviceSession(ctx, "hash-err")
	require.Error(t, err)
}

func TestAccountRepository_OAuthDeviceSession_GetByUserCode_EmptyResult(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	mockDB := new(dynamock.MockDB)
	mockQuery := new(dynamock.MockQuery)
	repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))

	mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
	mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
	mockQuery.On("Index", models.IndexGSI1).Return(mockQuery).Once()
	mockQuery.On("Where", "gsi1PK", "=", "OAUTH_DEVICE_USER_CODE#UC-EMPTY").Return(mockQuery).Once()
	mockQuery.On("Limit", 2).Return(mockQuery).Once()
	mockQuery.On("All", mock.Anything).Return(nil).Once()

	_, err := repo.GetOAuthDeviceSessionByUserCode(ctx, "UC-EMPTY")
	require.ErrorIs(t, err, storage.ErrNotFound)
}

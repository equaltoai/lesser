package repositories

import (
	"context"
	"encoding/json"
	stdErrors "errors"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	ddbErrors "github.com/theory-cloud/tabletheory/v3/pkg/errors"
	"github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	"go.uber.org/zap"
)

type fakeSecretsClient struct {
	get func(ctx context.Context, params *secretsmanager.GetSecretValueInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error)
	put func(ctx context.Context, params *secretsmanager.PutSecretValueInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.PutSecretValueOutput, error)
}

func (f fakeSecretsClient) GetSecretValue(ctx context.Context, params *secretsmanager.GetSecretValueInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error) {
	if f.get == nil {
		return nil, stdErrors.New("GetSecretValue not implemented")
	}
	return f.get(ctx, params, optFns...)
}

func (f fakeSecretsClient) PutSecretValue(ctx context.Context, params *secretsmanager.PutSecretValueInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.PutSecretValueOutput, error) {
	if f.put == nil {
		return nil, stdErrors.New("PutSecretValue not implemented")
	}
	return f.put(ctx, params, optFns...)
}

func TestRound07_PushSubscriptionRepository_CRUDAndConversions(t *testing.T) {
	baseTime := time.Unix(1, 0).UTC()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	setupPermissiveRound07Mocks(mockDB, mockQuery, nil, baseTime)

	repo := NewPushSubscriptionRepository(mockDB, "test-table", zap.NewNop(), nil, nil, "", "mailto:default@example.com")

	sub := &storage.PushSubscription{
		Endpoint: "https://example.com/push/1",
		P256dh:   "p256dh",
		Auth:     "auth",
		Alerts: storage.PushSubscriptionAlerts{
			Follow: true,
		},
		Policy: "all",
	}
	require.NoError(t, repo.CreatePushSubscription(context.Background(), "user-1", sub))
	require.NotEmpty(t, sub.ID)

	got, err := repo.GetPushSubscription(context.Background(), "user-1", sub.ID)
	require.NoError(t, err)
	require.NotNil(t, got)

	subs, err := repo.GetUserPushSubscriptions(context.Background(), "user-1")
	require.NoError(t, err)
	require.Len(t, subs, 2)

	require.NoError(t, repo.UpdatePushSubscription(context.Background(), "user-1", sub.ID, storage.PushSubscriptionAlerts{Mention: true}))
	require.NoError(t, repo.DeletePushSubscription(context.Background(), "user-1", sub.ID))
	require.NoError(t, repo.DeleteAllPushSubscriptions(context.Background(), "user-1"))

	modelAlerts := convertStorageAlerts(storage.PushSubscriptionAlerts{AdminReport: true, Mention: true})
	storageAlerts := convertModelAlerts(modelAlerts)
	require.True(t, storageAlerts.AdminReport)
	require.True(t, storageAlerts.Mention)
}

func TestRound07_PushSubscriptionRepository_GetUserPushSubscriptions_QueryError(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("Limit", mock.Anything).Return(mockQuery)
	mockQuery.On("All", mock.Anything).Return(stdErrors.New("query-failed"))

	repo := NewPushSubscriptionRepository(mockDB, "test-table", zap.NewNop(), nil, nil, "", "mailto:default@example.com")
	_, err := repo.GetUserPushSubscriptions(context.Background(), "user-1")
	require.Error(t, err)
}

func TestRound07_PushSubscriptionRepository_GetPushSubscription_NotFound(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("First", mock.Anything).Return(ddbErrors.ErrItemNotFound)

	repo := NewPushSubscriptionRepository(mockDB, "test-table", zap.NewNop(), nil, nil, "", "mailto:default@example.com")
	_, err := repo.GetPushSubscription(context.Background(), "user-1", "missing")
	require.Error(t, err)
}

func TestRound07_PushSubscriptionRepository_DeletePushSubscription_NotFoundAndOtherError(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("Delete").Return(ddbErrors.ErrItemNotFound).Once()
	mockQuery.On("Delete").Return(stdErrors.New("delete-failed")).Once()

	repo := NewPushSubscriptionRepository(mockDB, "test-table", zap.NewNop(), nil, nil, "", "mailto:default@example.com")
	require.NoError(t, repo.DeletePushSubscription(context.Background(), "user-1", "missing"))
	require.Error(t, repo.DeletePushSubscription(context.Background(), "user-1", "boom"))
}

func TestRound07_PushSubscriptionRepository_DeleteAllPushSubscriptions_ContinuesOnDeleteError(t *testing.T) {
	baseTime := time.Unix(1, 0).UTC()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	setupPermissiveRound07Mocks(mockDB, mockQuery, nil, baseTime)

	mockQuery.On("Delete").Return(stdErrors.New("delete-failed")).Maybe()

	repo := NewPushSubscriptionRepository(mockDB, "test-table", zap.NewNop(), nil, nil, "", "mailto:default@example.com")
	require.NoError(t, repo.DeleteAllPushSubscriptions(context.Background(), "user-1"))
}

func TestRound07_PushSubscriptionRepository_VAPIDSecretHelpers_Errors(t *testing.T) {
	repoNil := NewPushSubscriptionRepository(new(mocks.MockDB), "test-table", zap.NewNop(), nil, nil, "", "mailto:default@example.com")
	keys, err := repoNil.getVAPIDKeysFromSecret(context.Background())
	require.NoError(t, err)
	require.Nil(t, keys)

	secretARN := "arn:aws:secretsmanager:us-east-1:000000000000:secret:test"
	repoErr := NewPushSubscriptionRepository(
		new(mocks.MockDB),
		"test-table",
		zap.NewNop(),
		nil,
		fakeSecretsClient{
			get: func(_ context.Context, _ *secretsmanager.GetSecretValueInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error) {
				return nil, stdErrors.New("get-failed")
			},
		},
		secretARN,
		"mailto:default@example.com",
	)
	_, err = repoErr.getVAPIDKeysFromSecret(context.Background())
	require.Error(t, err)

	repoNoString := NewPushSubscriptionRepository(
		new(mocks.MockDB),
		"test-table",
		zap.NewNop(),
		nil,
		fakeSecretsClient{
			get: func(_ context.Context, _ *secretsmanager.GetSecretValueInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error) {
				return &secretsmanager.GetSecretValueOutput{SecretString: nil}, nil
			},
		},
		secretARN,
		"mailto:default@example.com",
	)
	_, err = repoNoString.getVAPIDKeysFromSecret(context.Background())
	require.Error(t, err)

	repoBadJSON := NewPushSubscriptionRepository(
		new(mocks.MockDB),
		"test-table",
		zap.NewNop(),
		nil,
		fakeSecretsClient{
			get: func(_ context.Context, _ *secretsmanager.GetSecretValueInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error) {
				value := "{"
				return &secretsmanager.GetSecretValueOutput{SecretString: aws.String(value)}, nil
			},
		},
		secretARN,
		"mailto:default@example.com",
	)
	_, err = repoBadJSON.getVAPIDKeysFromSecret(context.Background())
	require.Error(t, err)

	repoMissingKeys := NewPushSubscriptionRepository(
		new(mocks.MockDB),
		"test-table",
		zap.NewNop(),
		nil,
		fakeSecretsClient{
			get: func(_ context.Context, _ *secretsmanager.GetSecretValueInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error) {
				payload := vapidSecretPayload{PublicKey: "", PrivateKey: "private"}
				bytes, marshalErr := json.Marshal(payload)
				require.NoError(t, marshalErr)
				str := string(bytes)
				return &secretsmanager.GetSecretValueOutput{SecretString: aws.String(str)}, nil
			},
		},
		secretARN,
		"mailto:default@example.com",
	)
	_, err = repoMissingKeys.getVAPIDKeysFromSecret(context.Background())
	require.Error(t, err)

	repoPutErr := NewPushSubscriptionRepository(
		new(mocks.MockDB),
		"test-table",
		zap.NewNop(),
		nil,
		fakeSecretsClient{
			put: func(_ context.Context, _ *secretsmanager.PutSecretValueInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.PutSecretValueOutput, error) {
				return nil, stdErrors.New("put-failed")
			},
		},
		secretARN,
		"mailto:default@example.com",
	)
	require.Error(t, repoPutErr.setVAPIDKeysInSecret(context.Background(), &storage.VAPIDKeys{
		PublicKey:  "public",
		PrivateKey: "private",
		Subject:    "mailto:test@example.com",
		CreatedAt:  time.Unix(1, 0).UTC(),
		UpdatedAt:  time.Unix(2, 0).UTC(),
	}))
}

func TestRound07_PushSubscriptionRepository_VAPIDSecretAndFallbackBranches(t *testing.T) {
	baseTime := time.Unix(1, 0).UTC()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	setupPermissiveRound07Mocks(mockDB, mockQuery, nil, baseTime)

	secretARN := "arn:aws:secretsmanager:us-east-1:000000000000:secret:test"
	defaultSubject := "mailto:default@example.com"

	repo := NewPushSubscriptionRepository(
		mockDB,
		"test-table",
		zap.NewNop(),
		nil,
		fakeSecretsClient{
			get: func(_ context.Context, _ *secretsmanager.GetSecretValueInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error) {
				payload := vapidSecretPayload{
					PublicKey:  "public",
					PrivateKey: "private",
					Subject:    "",
					CreatedAt:  baseTime.UTC().Format(time.RFC3339),
					UpdatedAt:  "not-a-time",
				}
				bytes, err := json.Marshal(payload)
				require.NoError(t, err)
				str := string(bytes)
				return &secretsmanager.GetSecretValueOutput{SecretString: aws.String(str)}, nil
			},
			put: func(_ context.Context, _ *secretsmanager.PutSecretValueInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.PutSecretValueOutput, error) {
				return nil, stdErrors.New("put-failed")
			},
		},
		secretARN,
		defaultSubject,
	)

	keys, err := repo.GetVAPIDKeys(context.Background())
	require.NoError(t, err)
	require.Equal(t, "public", keys.PublicKey)
	require.Equal(t, "private", keys.PrivateKey)
	require.Equal(t, defaultSubject, keys.Subject)
	require.False(t, keys.CreatedAt.IsZero())
	require.True(t, keys.UpdatedAt.IsZero())

	parsed := parseVAPIDTimestamp(baseTime.UTC().Format(time.RFC3339))
	require.False(t, parsed.IsZero())
	require.True(t, parseVAPIDTimestamp("").IsZero())
	require.True(t, parseVAPIDTimestamp("not-a-time").IsZero())

	require.NoError(t, repo.SetVAPIDKeys(context.Background(), &storage.VAPIDKeys{PublicKey: "p", PrivateKey: "s"}))
}

func TestRound07_PushSubscriptionRepository_VAPIDFallbackAndTypeAssertionError(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
		record := args.Get(0).(*models.VAPIDKeyRecord)
		record.PK = "INSTANCE#CONFIG"
		record.SK = "VAPID_KEYS"
		record.Data = map[string]any{"not": "vapid"}
	}).Return(nil)

	repo := NewPushSubscriptionRepository(mockDB, "test-table", zap.NewNop(), nil, nil, "", "mailto:default@example.com")
	_, err := repo.GetVAPIDKeys(context.Background())
	require.Error(t, err)
}

func TestRound07_PushSubscriptionRepository_SetVAPIDKeys_NilAndUpdateFallback(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Update", mock.Anything).Return(ddbErrors.ErrItemNotFound).Once()
	mockQuery.On("Create").Return(nil).Once()

	repo := NewPushSubscriptionRepository(mockDB, "test-table", zap.NewNop(), nil, nil, "", "mailto:default@example.com")
	require.Error(t, repo.SetVAPIDKeys(context.Background(), nil))
	require.NoError(t, repo.SetVAPIDKeys(context.Background(), &storage.VAPIDKeys{PublicKey: "p", PrivateKey: "s"}))
}

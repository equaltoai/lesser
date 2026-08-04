package handlers

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/equaltoai/lesser/pkg/config"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	dynamormerrors "github.com/theory-cloud/tabletheory/v3/pkg/errors"
	dynamormmocks "github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	"go.uber.org/zap"
)

func setupMockInstanceRepoDB(db *dynamormmocks.MockDB, q *dynamormmocks.MockQuery) {
	db.On("WithContext", mock.Anything).Return(db).Maybe()
	db.On("Model", mock.Anything).Return(q).Maybe()
	q.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(q).Maybe()
}

func TestHandler_effectiveLesserHostTrustBaseURL_and_Attestations(t *testing.T) {
	ctx := context.Background()

	t.Run("repos_nil_uses_legacy_config", func(t *testing.T) {
		h := &Handler{
			cfg:    &config.Config{LesserHostURL: " https://legacy.example/ "},
			repos:  nil,
			logger: zap.NewNop(),
		}
		require.Equal(t, "https://legacy.example", h.effectiveLesserHostTrustBaseURL(ctx))
	})

	t.Run("trust_config_missing_uses_legacy_config", func(t *testing.T) {
		db := new(dynamormmocks.MockDB)
		q := new(dynamormmocks.MockQuery)
		setupMockInstanceRepoDB(db, q)
		q.On("First", mock.AnythingOfType("*models.InstanceTrustConfig")).Return(dynamormerrors.ErrItemNotFound).Maybe()

		instanceRepo := repositories.NewInstanceRepository(db, "test-table", zap.NewNop())
		repos := &MockRepositoryStorage{}
		repos.On("Instance").Return(instanceRepo).Maybe()

		h := &Handler{
			cfg:    &config.Config{LesserHostURL: "https://legacy.example", LesserHostAttestationsURL: ""},
			repos:  repos,
			logger: zap.NewNop(),
		}
		require.Equal(t, "https://legacy.example", h.effectiveLesserHostTrustBaseURL(ctx))
		require.Equal(t, "https://legacy.example", h.effectiveLesserHostAttestationsBaseURL(ctx))
	})

	t.Run("trust_config_error_disables_proxy_base_urls", func(t *testing.T) {
		db := new(dynamormmocks.MockDB)
		q := new(dynamormmocks.MockQuery)
		setupMockInstanceRepoDB(db, q)
		q.On("First", mock.AnythingOfType("*models.InstanceTrustConfig")).Return(errors.New("boom")).Maybe()

		instanceRepo := repositories.NewInstanceRepository(db, "test-table", zap.NewNop())
		repos := &MockRepositoryStorage{}
		repos.On("Instance").Return(instanceRepo).Maybe()

		h := &Handler{
			cfg:    &config.Config{LesserHostURL: "https://legacy.example"},
			repos:  repos,
			logger: zap.NewNop(),
		}
		require.Equal(t, "", h.effectiveLesserHostTrustBaseURL(ctx))
		require.Equal(t, "", h.effectiveLesserHostAttestationsBaseURL(ctx))
	})

	t.Run("trust_config_present_uses_effective_config", func(t *testing.T) {
		db := new(dynamormmocks.MockDB)
		q := new(dynamormmocks.MockQuery)
		setupMockInstanceRepoDB(db, q)

		q.On("First", mock.AnythingOfType("*models.InstanceTrustConfig")).Run(func(args mock.Arguments) {
			out := args.Get(0).(*storagemodels.InstanceTrustConfig)
			out.PK = "INSTANCE#CONFIG"
			out.SK = storagemodels.SKTrustConfig
			out.Managed = &storagemodels.InstanceTrustConfigManaged{
				BaseURL:         " https://persisted.example/ ",
				AttestationsURL: " https://attest.example/ ",
			}
			out.Override = nil
		}).Return(nil).Maybe()

		instanceRepo := repositories.NewInstanceRepository(db, "test-table", zap.NewNop())
		repos := &MockRepositoryStorage{}
		repos.On("Instance").Return(instanceRepo).Maybe()

		h := &Handler{
			cfg:    &config.Config{LesserHostURL: "https://legacy.example", LesserHostAttestationsURL: "https://legacy-attest.example"},
			repos:  repos,
			logger: zap.NewNop(),
		}
		require.Equal(t, "https://persisted.example", h.effectiveLesserHostTrustBaseURL(ctx))
		require.Equal(t, "https://attest.example", h.effectiveLesserHostAttestationsBaseURL(ctx))
	})
}

func TestHandler_effectiveLesserHostInstanceKey(t *testing.T) {
	ctx := context.Background()

	t.Run("repos_nil_uses_legacy_key", func(t *testing.T) {
		h := &Handler{cfg: &config.Config{LesserHostInstanceKey: " legacy "}}
		key, err := h.effectiveLesserHostInstanceKey(ctx)
		require.NoError(t, err)
		require.Equal(t, "legacy", key)
	})

	t.Run("trust_config_missing_uses_legacy_key", func(t *testing.T) {
		db := new(dynamormmocks.MockDB)
		q := new(dynamormmocks.MockQuery)
		setupMockInstanceRepoDB(db, q)
		q.On("First", mock.AnythingOfType("*models.InstanceTrustConfig")).Return(dynamormerrors.ErrItemNotFound).Maybe()

		instanceRepo := repositories.NewInstanceRepository(db, "test-table", zap.NewNop())
		repos := &MockRepositoryStorage{}
		repos.On("Instance").Return(instanceRepo).Maybe()

		h := &Handler{
			cfg:    &config.Config{LesserHostInstanceKey: "legacy"},
			repos:  repos,
			logger: zap.NewNop(),
		}
		key, err := h.effectiveLesserHostInstanceKey(ctx)
		require.NoError(t, err)
		require.Equal(t, "legacy", key)
	})

	t.Run("trust_config_present_without_secret_arn_returns_empty", func(t *testing.T) {
		db := new(dynamormmocks.MockDB)
		q := new(dynamormmocks.MockQuery)
		setupMockInstanceRepoDB(db, q)

		q.On("First", mock.AnythingOfType("*models.InstanceTrustConfig")).Run(func(args mock.Arguments) {
			out := args.Get(0).(*storagemodels.InstanceTrustConfig)
			out.PK = "INSTANCE#CONFIG"
			out.SK = storagemodels.SKTrustConfig
			out.Managed = &storagemodels.InstanceTrustConfigManaged{
				BaseURL:              "https://persisted.example",
				InstanceKeySecretARN: "",
			}
		}).Return(nil).Maybe()

		instanceRepo := repositories.NewInstanceRepository(db, "test-table", zap.NewNop())
		repos := &MockRepositoryStorage{}
		repos.On("Instance").Return(instanceRepo).Maybe()

		h := &Handler{
			cfg:    &config.Config{LesserHostInstanceKey: ""},
			repos:  repos,
			logger: zap.NewNop(),
		}
		key, err := h.effectiveLesserHostInstanceKey(ctx)
		require.NoError(t, err)
		require.Empty(t, key)
	})

	t.Run("trust_config_present_with_secret_arn_resolves_secret", func(t *testing.T) {
		resetTrustSecretCache()

		origLoad := loadAWSConfigForTrustSecrets
		origNewClient := newSecretsManagerClientForTrustSecret
		t.Cleanup(func() {
			loadAWSConfigForTrustSecrets = origLoad
			newSecretsManagerClientForTrustSecret = origNewClient
			resetTrustSecretCache()
		})

		loadAWSConfigForTrustSecrets = func(context.Context, ...func(*awsconfig.LoadOptions) error) (aws.Config, error) {
			return aws.Config{}, nil
		}

		secret := "resolved-key"
		client := &stubTrustSecretsManagerClient{
			out: &secretsmanager.GetSecretValueOutput{
				SecretString: &secret,
			},
		}
		newSecretsManagerClientForTrustSecret = func(aws.Config) trustSecretsManagerClient {
			return client
		}

		db := new(dynamormmocks.MockDB)
		q := new(dynamormmocks.MockQuery)
		setupMockInstanceRepoDB(db, q)
		q.On("First", mock.AnythingOfType("*models.InstanceTrustConfig")).Run(func(args mock.Arguments) {
			out := args.Get(0).(*storagemodels.InstanceTrustConfig)
			out.PK = "INSTANCE#CONFIG"
			out.SK = storagemodels.SKTrustConfig
			out.Managed = &storagemodels.InstanceTrustConfigManaged{
				BaseURL:              "https://persisted.example",
				InstanceKeySecretARN: "secret-id",
			}
		}).Return(nil).Maybe()

		instanceRepo := repositories.NewInstanceRepository(db, "test-table", zap.NewNop())
		repos := &MockRepositoryStorage{}
		repos.On("Instance").Return(instanceRepo).Maybe()

		h := &Handler{
			cfg:    &config.Config{LesserHostInstanceKey: ""},
			repos:  repos,
			logger: zap.NewNop(),
		}

		key, err := h.effectiveLesserHostInstanceKey(ctx)
		require.NoError(t, err)
		require.Equal(t, "resolved-key", key)
		require.Equal(t, 1, client.callCount)
	})
}

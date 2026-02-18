package handlers

import (
	"context"
	"errors"
	"testing"

	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

type stubInstanceFeatureConfigRepo struct {
	trustExists    bool
	trustExistsErr error
	trustEnsures   int
	trustEnsureErr error
	trustSets      int
	trustSetErr    error
	trustPatch     storagemodels.InstanceTrustConfigPatch

	translationExists    bool
	translationExistsErr error
	translationEnsures   int
	translationEnsureErr error
	translationSets      int
	translationSetErr    error
	translationPatch     storagemodels.InstanceTranslationConfigPatch

	tipsExists    bool
	tipsExistsErr error
	tipsEnsures   int
	tipsEnsureErr error
	tipsSets      int
	tipsSetErr    error
	tipsPatch     storagemodels.InstanceTipsConfigPatch
}

func (s *stubInstanceFeatureConfigRepo) TrustConfigExists(context.Context) (bool, error) {
	return s.trustExists, s.trustExistsErr
}

func (s *stubInstanceFeatureConfigRepo) EnsureTrustConfig(context.Context) (*storagemodels.InstanceTrustConfig, error) {
	s.trustEnsures++
	if s.trustEnsureErr != nil {
		return nil, s.trustEnsureErr
	}
	return &storagemodels.InstanceTrustConfig{}, nil
}

func (s *stubInstanceFeatureConfigRepo) SetTrustManagedDefaults(_ context.Context, patch storagemodels.InstanceTrustConfigPatch) error {
	s.trustSets++
	s.trustPatch = patch
	return s.trustSetErr
}

func (s *stubInstanceFeatureConfigRepo) TranslationConfigExists(context.Context) (bool, error) {
	return s.translationExists, s.translationExistsErr
}

func (s *stubInstanceFeatureConfigRepo) EnsureTranslationConfig(context.Context) (*storagemodels.InstanceTranslationConfig, error) {
	s.translationEnsures++
	if s.translationEnsureErr != nil {
		return nil, s.translationEnsureErr
	}
	return &storagemodels.InstanceTranslationConfig{}, nil
}

func (s *stubInstanceFeatureConfigRepo) SetTranslationManagedDefaults(_ context.Context, patch storagemodels.InstanceTranslationConfigPatch) error {
	s.translationSets++
	s.translationPatch = patch
	return s.translationSetErr
}

func (s *stubInstanceFeatureConfigRepo) TipsConfigExists(context.Context) (bool, error) {
	return s.tipsExists, s.tipsExistsErr
}

func (s *stubInstanceFeatureConfigRepo) EnsureTipsConfig(context.Context) (*storagemodels.InstanceTipsConfig, error) {
	s.tipsEnsures++
	if s.tipsEnsureErr != nil {
		return nil, s.tipsEnsureErr
	}
	return &storagemodels.InstanceTipsConfig{}, nil
}

func (s *stubInstanceFeatureConfigRepo) SetTipsManagedDefaults(_ context.Context, patch storagemodels.InstanceTipsConfigPatch) error {
	s.tipsSets++
	s.tipsPatch = patch
	return s.tipsSetErr
}

func TestHandler_migrateTrustConfigFromEnv_SkipsWhenExists(t *testing.T) {
	t.Setenv("LESSER_HOST_URL", "https://stage.lesser.host/")
	t.Setenv("LESSER_HOST_ATTESTATIONS_URL", "")
	t.Setenv("LESSER_HOST_INSTANCE_KEY_ARN", "arn:aws:secretsmanager:us-east-1:123:secret:abc")

	h := &Handler{logger: zap.NewNop()}
	repo := &stubInstanceFeatureConfigRepo{trustExists: true}

	h.migrateTrustConfigFromEnv(context.Background(), repo)
	assert.Equal(t, 0, repo.trustEnsures)
	assert.Equal(t, 0, repo.trustSets)
}

func TestHandler_migrateTrustConfigFromEnv_ReturnsOnExistsError(t *testing.T) {
	h := &Handler{logger: zap.NewNop()}
	repo := &stubInstanceFeatureConfigRepo{trustExistsErr: errors.New("boom")}

	h.migrateTrustConfigFromEnv(context.Background(), repo)
	assert.Equal(t, 0, repo.trustEnsures)
	assert.Equal(t, 0, repo.trustSets)
}

func TestHandler_migrateTrustConfigFromEnv_SkipsWhenEnvEmpty(t *testing.T) {
	t.Setenv("LESSER_HOST_URL", "")
	t.Setenv("LESSER_HOST_ATTESTATIONS_URL", "")
	t.Setenv("LESSER_HOST_INSTANCE_KEY_ARN", "")

	h := &Handler{logger: zap.NewNop()}
	repo := &stubInstanceFeatureConfigRepo{trustExists: false}

	h.migrateTrustConfigFromEnv(context.Background(), repo)
	assert.Equal(t, 0, repo.trustEnsures)
	assert.Equal(t, 0, repo.trustSets)
}

func TestHandler_migrateTrustConfigFromEnv_SkipsWhenMissingSecretARN(t *testing.T) {
	t.Setenv("LESSER_HOST_URL", "https://stage.lesser.host/")
	t.Setenv("LESSER_HOST_ATTESTATIONS_URL", "")
	t.Setenv("LESSER_HOST_INSTANCE_KEY_ARN", "")

	h := &Handler{logger: zap.NewNop()}
	repo := &stubInstanceFeatureConfigRepo{trustExists: false}

	h.migrateTrustConfigFromEnv(context.Background(), repo)
	assert.Equal(t, 0, repo.trustEnsures)
	assert.Equal(t, 0, repo.trustSets)
}

func TestHandler_migrateTrustConfigFromEnv_SkipsWhenMissingBaseURLEvenWithSecretARN(t *testing.T) {
	t.Setenv("LESSER_HOST_URL", "")
	t.Setenv("LESSER_HOST_ATTESTATIONS_URL", "")
	t.Setenv("LESSER_HOST_INSTANCE_KEY_ARN", "arn:aws:secretsmanager:us-east-1:123:secret:abc")

	h := &Handler{logger: zap.NewNop()}
	repo := &stubInstanceFeatureConfigRepo{trustExists: false}

	h.migrateTrustConfigFromEnv(context.Background(), repo)
	assert.Equal(t, 0, repo.trustEnsures)
	assert.Equal(t, 0, repo.trustSets)
}

func TestHandler_migrateTrustConfigFromEnv_PersistsWithAttestationsFallback(t *testing.T) {
	t.Setenv("LESSER_HOST_URL", "")
	t.Setenv("LESSER_HOST_ATTESTATIONS_URL", " https://attest.example/ ")
	t.Setenv("LESSER_HOST_INSTANCE_KEY_ARN", " arn:aws:secretsmanager:us-east-1:123:secret:abc ")

	h := &Handler{logger: zap.NewNop()}
	repo := &stubInstanceFeatureConfigRepo{trustExists: false}

	h.migrateTrustConfigFromEnv(context.Background(), repo)
	assert.Equal(t, 1, repo.trustEnsures)
	assert.Equal(t, 1, repo.trustSets)
	if assert.NotNil(t, repo.trustPatch.BaseURL) {
		assert.Equal(t, "https://attest.example", *repo.trustPatch.BaseURL)
	}
	if assert.NotNil(t, repo.trustPatch.AttestationsURL) {
		assert.Equal(t, "https://attest.example", *repo.trustPatch.AttestationsURL)
	}
	if assert.NotNil(t, repo.trustPatch.InstanceKeySecretARN) {
		assert.Equal(t, "arn:aws:secretsmanager:us-east-1:123:secret:abc", *repo.trustPatch.InstanceKeySecretARN)
	}
}

func TestHandler_migrateTrustConfigFromEnv_PersistsWithoutAttestationsURL(t *testing.T) {
	t.Setenv("LESSER_HOST_URL", " https://trust.example/ ")
	t.Setenv("LESSER_HOST_ATTESTATIONS_URL", "")
	t.Setenv("LESSER_HOST_INSTANCE_KEY_ARN", " arn:aws:secretsmanager:us-east-1:123:secret:abc ")

	h := &Handler{logger: zap.NewNop()}
	repo := &stubInstanceFeatureConfigRepo{trustExists: false}

	h.migrateTrustConfigFromEnv(context.Background(), repo)
	assert.Equal(t, 1, repo.trustEnsures)
	assert.Equal(t, 1, repo.trustSets)
	if assert.NotNil(t, repo.trustPatch.BaseURL) {
		assert.Equal(t, "https://trust.example", *repo.trustPatch.BaseURL)
	}
	assert.Nil(t, repo.trustPatch.AttestationsURL)
}

func TestHandler_migrateTrustConfigFromEnv_ReturnsWhenEnsureFails(t *testing.T) {
	t.Setenv("LESSER_HOST_URL", "https://trust.example/")
	t.Setenv("LESSER_HOST_ATTESTATIONS_URL", "")
	t.Setenv("LESSER_HOST_INSTANCE_KEY_ARN", "arn:aws:secretsmanager:us-east-1:123:secret:abc")

	h := &Handler{logger: zap.NewNop()}
	repo := &stubInstanceFeatureConfigRepo{trustEnsureErr: errors.New("boom")}

	h.migrateTrustConfigFromEnv(context.Background(), repo)
	assert.Equal(t, 1, repo.trustEnsures)
	assert.Equal(t, 0, repo.trustSets)
}

func TestHandler_migrateTrustConfigFromEnv_ReturnsWhenSetFails(t *testing.T) {
	t.Setenv("LESSER_HOST_URL", "https://trust.example/")
	t.Setenv("LESSER_HOST_ATTESTATIONS_URL", "")
	t.Setenv("LESSER_HOST_INSTANCE_KEY_ARN", "arn:aws:secretsmanager:us-east-1:123:secret:abc")

	h := &Handler{logger: zap.NewNop()}
	repo := &stubInstanceFeatureConfigRepo{trustSetErr: errors.New("boom")}

	h.migrateTrustConfigFromEnv(context.Background(), repo)
	assert.Equal(t, 1, repo.trustEnsures)
	assert.Equal(t, 1, repo.trustSets)
}

func TestHandler_migrateTranslationConfigFromEnv_SkipsWhenEnvEmpty(t *testing.T) {
	t.Setenv("TRANSLATION_ENABLED", "")

	h := &Handler{logger: zap.NewNop()}
	repo := &stubInstanceFeatureConfigRepo{translationExists: false}

	h.migrateTranslationConfigFromEnv(context.Background(), repo)
	assert.Equal(t, 0, repo.translationEnsures)
	assert.Equal(t, 0, repo.translationSets)
}

func TestHandler_migrateTranslationConfigFromEnv_SkipsWhenExists(t *testing.T) {
	t.Setenv("TRANSLATION_ENABLED", "true")

	h := &Handler{logger: zap.NewNop()}
	repo := &stubInstanceFeatureConfigRepo{translationExists: true}

	h.migrateTranslationConfigFromEnv(context.Background(), repo)
	assert.Equal(t, 0, repo.translationEnsures)
	assert.Equal(t, 0, repo.translationSets)
}

func TestHandler_migrateTranslationConfigFromEnv_ReturnsOnExistsError(t *testing.T) {
	t.Setenv("TRANSLATION_ENABLED", "true")

	h := &Handler{logger: zap.NewNop()}
	repo := &stubInstanceFeatureConfigRepo{translationExistsErr: errors.New("boom")}

	h.migrateTranslationConfigFromEnv(context.Background(), repo)
	assert.Equal(t, 0, repo.translationEnsures)
	assert.Equal(t, 0, repo.translationSets)
}

func TestHandler_migrateTranslationConfigFromEnv_PersistsEnabled(t *testing.T) {
	t.Setenv("TRANSLATION_ENABLED", "1")

	h := &Handler{logger: zap.NewNop()}
	repo := &stubInstanceFeatureConfigRepo{translationExists: false}

	h.migrateTranslationConfigFromEnv(context.Background(), repo)
	assert.Equal(t, 1, repo.translationEnsures)
	assert.Equal(t, 1, repo.translationSets)
	if assert.NotNil(t, repo.translationPatch.Enabled) {
		assert.True(t, *repo.translationPatch.Enabled)
	}
}

func TestHandler_migrateTranslationConfigFromEnv_ReturnsWhenEnsureFails(t *testing.T) {
	t.Setenv("TRANSLATION_ENABLED", "true")

	h := &Handler{logger: zap.NewNop()}
	repo := &stubInstanceFeatureConfigRepo{translationEnsureErr: errors.New("boom")}

	h.migrateTranslationConfigFromEnv(context.Background(), repo)
	assert.Equal(t, 1, repo.translationEnsures)
	assert.Equal(t, 0, repo.translationSets)
}

func TestHandler_migrateTranslationConfigFromEnv_ReturnsWhenSetFails(t *testing.T) {
	t.Setenv("TRANSLATION_ENABLED", "true")

	h := &Handler{logger: zap.NewNop()}
	repo := &stubInstanceFeatureConfigRepo{translationSetErr: errors.New("boom")}

	h.migrateTranslationConfigFromEnv(context.Background(), repo)
	assert.Equal(t, 1, repo.translationEnsures)
	assert.Equal(t, 1, repo.translationSets)
}

func TestHandler_migrateTipsConfigFromEnv_SkipsWhenEnvEmpty(t *testing.T) {
	t.Setenv("TIP_ENABLED", "")
	t.Setenv("TIP_CHAIN_ID", "")
	t.Setenv("TIP_CONTRACT_ADDRESS", "")

	h := &Handler{logger: zap.NewNop()}
	repo := &stubInstanceFeatureConfigRepo{tipsExists: false}

	h.migrateTipsConfigFromEnv(context.Background(), repo)
	assert.Equal(t, 0, repo.tipsEnsures)
	assert.Equal(t, 0, repo.tipsSets)
}

func TestHandler_migrateTipsConfigFromEnv_SkipsWhenExists(t *testing.T) {
	t.Setenv("TIP_ENABLED", "false")
	t.Setenv("TIP_CHAIN_ID", "10")
	t.Setenv("TIP_CONTRACT_ADDRESS", "0xabc")

	h := &Handler{logger: zap.NewNop()}
	repo := &stubInstanceFeatureConfigRepo{tipsExists: true}

	h.migrateTipsConfigFromEnv(context.Background(), repo)
	assert.Equal(t, 0, repo.tipsEnsures)
	assert.Equal(t, 0, repo.tipsSets)
}

func TestHandler_migrateTipsConfigFromEnv_ReturnsOnExistsError(t *testing.T) {
	t.Setenv("TIP_ENABLED", "false")
	t.Setenv("TIP_CHAIN_ID", "10")
	t.Setenv("TIP_CONTRACT_ADDRESS", "0xabc")

	h := &Handler{logger: zap.NewNop()}
	repo := &stubInstanceFeatureConfigRepo{tipsExistsErr: errors.New("boom")}

	h.migrateTipsConfigFromEnv(context.Background(), repo)
	assert.Equal(t, 0, repo.tipsEnsures)
	assert.Equal(t, 0, repo.tipsSets)
}

func TestHandler_migrateTipsConfigFromEnv_DisablesWhenEnabledMissingChainOrContract(t *testing.T) {
	t.Setenv("TIP_ENABLED", "true")
	t.Setenv("TIP_CHAIN_ID", "")
	t.Setenv("TIP_CONTRACT_ADDRESS", "")

	h := &Handler{logger: zap.NewNop()}
	repo := &stubInstanceFeatureConfigRepo{tipsExists: false}

	h.migrateTipsConfigFromEnv(context.Background(), repo)
	assert.Equal(t, 1, repo.tipsEnsures)
	assert.Equal(t, 1, repo.tipsSets)
	if assert.NotNil(t, repo.tipsPatch.Enabled) {
		assert.False(t, *repo.tipsPatch.Enabled)
	}
	assert.Nil(t, repo.tipsPatch.ChainID)
	assert.Nil(t, repo.tipsPatch.ContractAddress)
}

func TestHandler_migrateTipsConfigFromEnv_PersistsEnabledChainAndContract(t *testing.T) {
	t.Setenv("TIP_ENABLED", "true")
	t.Setenv("TIP_CHAIN_ID", "10")
	t.Setenv("TIP_CONTRACT_ADDRESS", " 0xabc ")

	h := &Handler{logger: zap.NewNop()}
	repo := &stubInstanceFeatureConfigRepo{tipsExists: false}

	h.migrateTipsConfigFromEnv(context.Background(), repo)
	assert.Equal(t, 1, repo.tipsEnsures)
	assert.Equal(t, 1, repo.tipsSets)
	if assert.NotNil(t, repo.tipsPatch.Enabled) {
		assert.True(t, *repo.tipsPatch.Enabled)
	}
	if assert.NotNil(t, repo.tipsPatch.ChainID) {
		assert.Equal(t, 10, *repo.tipsPatch.ChainID)
	}
	if assert.NotNil(t, repo.tipsPatch.ContractAddress) {
		assert.Equal(t, "0xabc", *repo.tipsPatch.ContractAddress)
	}
}

func TestHandler_migrateTipsConfigFromEnv_IgnoresBadChainIDAndPersistsOtherFields(t *testing.T) {
	t.Setenv("TIP_ENABLED", "false")
	t.Setenv("TIP_CHAIN_ID", "bad")
	t.Setenv("TIP_CONTRACT_ADDRESS", " 0xabc ")

	h := &Handler{logger: zap.NewNop()}
	repo := &stubInstanceFeatureConfigRepo{tipsExists: false}

	h.migrateTipsConfigFromEnv(context.Background(), repo)
	assert.Equal(t, 1, repo.tipsEnsures)
	assert.Equal(t, 1, repo.tipsSets)
	assert.NotNil(t, repo.tipsPatch.Enabled)
	assert.False(t, *repo.tipsPatch.Enabled)
	assert.Nil(t, repo.tipsPatch.ChainID)
	if assert.NotNil(t, repo.tipsPatch.ContractAddress) {
		assert.Equal(t, "0xabc", *repo.tipsPatch.ContractAddress)
	}
}

func TestHandler_migrateTipsConfigFromEnv_ReturnsWhenEnsureFails(t *testing.T) {
	t.Setenv("TIP_ENABLED", "true")
	t.Setenv("TIP_CHAIN_ID", "10")
	t.Setenv("TIP_CONTRACT_ADDRESS", "0xabc")

	h := &Handler{logger: zap.NewNop()}
	repo := &stubInstanceFeatureConfigRepo{tipsEnsureErr: errors.New("boom")}

	h.migrateTipsConfigFromEnv(context.Background(), repo)
	assert.Equal(t, 1, repo.tipsEnsures)
	assert.Equal(t, 0, repo.tipsSets)
}

func TestHandler_migrateTipsConfigFromEnv_ReturnsWhenSetFails(t *testing.T) {
	t.Setenv("TIP_ENABLED", "true")
	t.Setenv("TIP_CHAIN_ID", "10")
	t.Setenv("TIP_CONTRACT_ADDRESS", "0xabc")

	h := &Handler{logger: zap.NewNop()}
	repo := &stubInstanceFeatureConfigRepo{tipsSetErr: errors.New("boom")}

	h.migrateTipsConfigFromEnv(context.Background(), repo)
	assert.Equal(t, 1, repo.tipsEnsures)
	assert.Equal(t, 1, repo.tipsSets)
}

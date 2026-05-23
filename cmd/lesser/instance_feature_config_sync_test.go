package main

import (
	"context"
	"errors"
	"testing"

	"github.com/equaltoai/lesser/pkg/deploy/naming"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	theorydb "github.com/theory-cloud/tabletheory/pkg/core"
	dynamormerrors "github.com/theory-cloud/tabletheory/pkg/errors"
	dynamormmocks "github.com/theory-cloud/tabletheory/pkg/mocks"
	"go.uber.org/zap"
)

func setupMockInstanceRepoDB(db *dynamormmocks.MockDB, q *dynamormmocks.MockQuery) {
	db.On("WithContext", mock.Anything).Return(db).Maybe()
	db.On("Model", mock.Anything).Return(q).Maybe()
	q.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(q).Maybe()
	q.On("Create").Return(nil).Maybe()
	q.On("Update", mock.Anything).Return(nil).Maybe()
}

func setupMockFirstForFeatureConfigs(q *dynamormmocks.MockQuery) {
	q.On("First", mock.AnythingOfType("*models.InstanceTrustConfig")).Run(func(args mock.Arguments) {
		out := args.Get(0).(*models.InstanceTrustConfig)
		out.PK = "INSTANCE#CONFIG"
		out.SK = models.SKTrustConfig
		out.Managed = nil
		out.Override = nil
	}).Return(nil).Maybe()

	q.On("First", mock.AnythingOfType("*models.InstanceTranslationConfig")).Run(func(args mock.Arguments) {
		out := args.Get(0).(*models.InstanceTranslationConfig)
		out.PK = "INSTANCE#CONFIG"
		out.SK = models.SKTranslationConfig
		out.Managed = nil
		out.Override = nil
	}).Return(nil).Maybe()

	q.On("First", mock.AnythingOfType("*models.InstanceTipsConfig")).Run(func(args mock.Arguments) {
		out := args.Get(0).(*models.InstanceTipsConfig)
		out.PK = "INSTANCE#CONFIG"
		out.SK = models.SKTipsConfig
		out.Managed = nil
		out.Override = nil
	}).Return(nil).Maybe()

	q.On("First", mock.AnythingOfType("*models.AIInstanceConfig")).Run(func(args mock.Arguments) {
		out := args.Get(0).(*models.AIInstanceConfig)
		out.PK = "INSTANCE#CONFIG"
		out.SK = models.SKAIConfig
		out.Managed = nil
		out.Override = nil
	}).Return(nil).Maybe()

	q.On("First", mock.AnythingOfType("*models.AgentInstanceConfig")).Run(func(args mock.Arguments) {
		out := args.Get(0).(*models.AgentInstanceConfig)
		out.PK = "INSTANCE#CONFIG"
		out.SK = "AGENT_CONFIG"
	}).Return(nil).Maybe()
}

func TestEnvFeatureConfigSeedAvailable_FalseWhenNoEnvSet(t *testing.T) {
	t.Setenv("LESSER_HOST_URL", "")
	t.Setenv("LESSER_HOST_ATTESTATIONS_URL", "")
	t.Setenv("LESSER_HOST_INSTANCE_KEY_ARN", "")
	t.Setenv("TRANSLATION_ENABLED", "")
	t.Setenv("TIP_ENABLED", "")
	t.Setenv("TIP_CHAIN_ID", "")
	t.Setenv("TIP_CONTRACT_ADDRESS", "")

	assert.False(t, envFeatureConfigSeedAvailable())
}

func TestEnvFeatureConfigSeedAvailable_TrueWhenAnyEnvSet(t *testing.T) {
	t.Setenv("LESSER_HOST_URL", "")
	t.Setenv("LESSER_HOST_ATTESTATIONS_URL", "")
	t.Setenv("LESSER_HOST_INSTANCE_KEY_ARN", "")
	t.Setenv("TRANSLATION_ENABLED", "")
	t.Setenv("TIP_ENABLED", "")
	t.Setenv("TIP_CHAIN_ID", "")
	t.Setenv("TIP_CONTRACT_ADDRESS", "0xabc")

	assert.True(t, envFeatureConfigSeedAvailable())
}

func TestEnvBoolPtr(t *testing.T) {
	t.Setenv("TEST_BOOL", "")
	assert.Nil(t, envBoolPtr("TEST_BOOL"))

	t.Setenv("TEST_BOOL", "TRUE")
	require.NotNil(t, envBoolPtr("TEST_BOOL"))
	assert.True(t, *envBoolPtr("TEST_BOOL"))

	t.Setenv("TEST_BOOL", "yes")
	require.NotNil(t, envBoolPtr("TEST_BOOL"))
	assert.True(t, *envBoolPtr("TEST_BOOL"))

	t.Setenv("TEST_BOOL", "0")
	require.NotNil(t, envBoolPtr("TEST_BOOL"))
	assert.False(t, *envBoolPtr("TEST_BOOL"))
}

func TestEnvIntPtr(t *testing.T) {
	t.Setenv("TEST_INT", "")
	assert.Nil(t, envIntPtr("TEST_INT"))

	t.Setenv("TEST_INT", "bad")
	assert.Nil(t, envIntPtr("TEST_INT"))

	t.Setenv("TEST_INT", "10")
	require.NotNil(t, envIntPtr("TEST_INT"))
	assert.Equal(t, 10, *envIntPtr("TEST_INT"))
}

func TestStringPtrOrNil(t *testing.T) {
	assert.Nil(t, stringPtrOrNil(""))
	assert.Nil(t, stringPtrOrNil("   "))
	require.NotNil(t, stringPtrOrNil("  x  "))
	assert.Equal(t, "x", *stringPtrOrNil("  x  "))
}

func TestEnvTrustPatch_TrimsAndDropsEmptyFields(t *testing.T) {
	t.Setenv("LESSER_HOST_URL", " https://trust.example/ ")
	t.Setenv("LESSER_HOST_ATTESTATIONS_URL", "")
	t.Setenv("LESSER_HOST_INSTANCE_KEY_ARN", " arn:aws:secretsmanager:us-east-1:123:secret:abc ")

	patch := envTrustPatch()
	if assert.NotNil(t, patch.BaseURL) {
		assert.Equal(t, "https://trust.example", *patch.BaseURL)
	}
	assert.Nil(t, patch.AttestationsURL)
	if assert.NotNil(t, patch.InstanceKeySecretARN) {
		assert.Equal(t, "arn:aws:secretsmanager:us-east-1:123:secret:abc", *patch.InstanceKeySecretARN)
	}
}

func TestEnvTipsPatch_ParsesEnv(t *testing.T) {
	t.Setenv("TIP_ENABLED", "true")
	t.Setenv("TIP_CHAIN_ID", "10")
	t.Setenv("TIP_CONTRACT_ADDRESS", " 0xabc ")

	patch := envTipsPatch()
	if assert.NotNil(t, patch.Enabled) {
		assert.True(t, *patch.Enabled)
	}
	if assert.NotNil(t, patch.ChainID) {
		assert.Equal(t, 10, *patch.ChainID)
	}
	if assert.NotNil(t, patch.ContractAddress) {
		assert.Equal(t, "0xabc", *patch.ContractAddress)
	}
}

func TestEnsureAndApplyManagedProvisioningConfig_NoOpWhenRepoNil(t *testing.T) {
	require.NoError(t, ensureAndApplyManagedProvisioningConfig(context.Background(), nil, upArgs{}))
}

func TestEnsureAndApplyManagedProvisioningConfig_AppliesPatches(t *testing.T) {
	ctx := context.Background()
	db := new(dynamormmocks.MockDB)
	q := new(dynamormmocks.MockQuery)

	setupMockInstanceRepoDB(db, q)
	setupMockFirstForFeatureConfigs(q)

	instanceRepo := repositories.NewInstanceRepository(db, "test-table", zap.NewNop())

	translationEnabled := true
	tipEnabled := true
	tipChainID := 10

	aiEnabled := true
	moderationEnabled := false
	nsfwEnabled := true
	spamEnabled := false
	piiEnabled := true
	contentEnabled := true

	args := upArgs{
		LesserHostURL:             " https://trust.example/ ",
		LesserHostAttestationsURL: " https://attest.example/ ",
		LesserHostInstanceKeyARN:  " arn:aws:secretsmanager:us-east-1:123:secret:abc ",
		TranslationEnabled:        &translationEnabled,
		TipEnabled:                &tipEnabled,
		TipChainID:                &tipChainID,
		TipContractAddress:        " 0xabc ",
		AIEnabled:                 &aiEnabled,
		AIModerationEnabled:       &moderationEnabled,
		AINsfwDetectionEnabled:    &nsfwEnabled,
		AISpamDetectionEnabled:    &spamEnabled,
		AIPiiDetectionEnabled:     &piiEnabled,
		AIContentDetectionEnabled: &contentEnabled,
	}

	require.NoError(t, ensureAndApplyManagedProvisioningConfig(ctx, instanceRepo, args))

	trustCfg, err := instanceRepo.GetTrustConfig(ctx)
	require.NoError(t, err)
	require.NotNil(t, trustCfg.Managed)
	assert.Equal(t, "https://trust.example", trustCfg.Managed.BaseURL)
	assert.Equal(t, "https://attest.example", trustCfg.Managed.AttestationsURL)
	assert.Equal(t, "arn:aws:secretsmanager:us-east-1:123:secret:abc", trustCfg.Managed.InstanceKeySecretARN)

	translationCfg, err := instanceRepo.GetTranslationConfig(ctx)
	require.NoError(t, err)
	require.NotNil(t, translationCfg.Managed)
	assert.True(t, translationCfg.Managed.Enabled)

	tipsCfg, err := instanceRepo.GetTipsConfig(ctx)
	require.NoError(t, err)
	require.NotNil(t, tipsCfg.Managed)
	assert.True(t, tipsCfg.Managed.Enabled)
	assert.Equal(t, 10, tipsCfg.Managed.ChainID)
	assert.Equal(t, "0xabc", tipsCfg.Managed.ContractAddress)

	aiCfg, err := instanceRepo.GetAIInstanceConfig(ctx)
	require.NoError(t, err)
	require.NotNil(t, aiCfg.Managed)
	assert.True(t, aiCfg.Managed.AIEnabled)
	assert.False(t, aiCfg.Managed.ModerationEnabled)
	assert.True(t, aiCfg.Managed.NSFWDetectionEnabled)
	assert.False(t, aiCfg.Managed.SpamDetectionEnabled)
	assert.True(t, aiCfg.Managed.PIIDetectionEnabled)
	assert.True(t, aiCfg.Managed.AIContentDetection)
}

func TestSeedTrustManagedDefaultsFromEnvOnce_NoOpWhenConfigExists(t *testing.T) {
	ctx := context.Background()
	db := new(dynamormmocks.MockDB)
	q := new(dynamormmocks.MockQuery)

	setupMockInstanceRepoDB(db, q)
	setupMockFirstForFeatureConfigs(q)

	t.Setenv("LESSER_HOST_URL", "https://trust.example/")
	t.Setenv("LESSER_HOST_INSTANCE_KEY_ARN", "arn:aws:secretsmanager:us-east-1:123:secret:abc")

	instanceRepo := repositories.NewInstanceRepository(db, "test-table", zap.NewNop())
	require.NoError(t, seedTrustManagedDefaultsFromEnvOnce(ctx, instanceRepo, "dev", "test-table"))
}

func TestSeedTrustManagedDefaultsFromEnvOnce_NoOpWhenNoConfigAndNoEnv(t *testing.T) {
	ctx := context.Background()
	db := new(dynamormmocks.MockDB)
	q := new(dynamormmocks.MockQuery)

	setupMockInstanceRepoDB(db, q)
	q.On("First", mock.AnythingOfType("*models.InstanceTrustConfig")).Return(dynamormerrors.ErrItemNotFound).Once()

	t.Setenv("LESSER_HOST_URL", "")
	t.Setenv("LESSER_HOST_ATTESTATIONS_URL", "")
	t.Setenv("LESSER_HOST_INSTANCE_KEY_ARN", "")

	instanceRepo := repositories.NewInstanceRepository(db, "test-table", zap.NewNop())
	require.NoError(t, seedTrustManagedDefaultsFromEnvOnce(ctx, instanceRepo, "dev", "test-table"))
}

func TestSeedTrustManagedDefaultsFromEnvOnce_SeedsFromEnv(t *testing.T) {
	ctx := context.Background()
	db := new(dynamormmocks.MockDB)
	q := new(dynamormmocks.MockQuery)

	setupMockInstanceRepoDB(db, q)
	q.On("First", mock.AnythingOfType("*models.InstanceTrustConfig")).Return(dynamormerrors.ErrItemNotFound).Once()
	q.On("First", mock.AnythingOfType("*models.InstanceTrustConfig")).Run(func(args mock.Arguments) {
		out := args.Get(0).(*models.InstanceTrustConfig)
		out.PK = "INSTANCE#CONFIG"
		out.SK = models.SKTrustConfig
		out.Managed = nil
		out.Override = nil
	}).Return(nil).Maybe()

	t.Setenv("LESSER_HOST_URL", " https://trust.example/ ")
	t.Setenv("LESSER_HOST_ATTESTATIONS_URL", "")
	t.Setenv("LESSER_HOST_INSTANCE_KEY_ARN", " arn:aws:secretsmanager:us-east-1:123:secret:abc ")

	instanceRepo := repositories.NewInstanceRepository(db, "test-table", zap.NewNop())
	require.NoError(t, seedTrustManagedDefaultsFromEnvOnce(ctx, instanceRepo, "dev", "test-table"))

	trustCfg, err := instanceRepo.GetTrustConfig(ctx)
	require.NoError(t, err)
	require.NotNil(t, trustCfg.Managed)
	assert.Equal(t, "https://trust.example", trustCfg.Managed.BaseURL)
	assert.Equal(t, "arn:aws:secretsmanager:us-east-1:123:secret:abc", trustCfg.Managed.InstanceKeySecretARN)
}

func TestSeedManagedConfigFromEnvOnce_NoOpWhenRepoNil(t *testing.T) {
	require.NoError(t, seedManagedConfigFromEnvOnce(context.Background(), nil, "dev", "test-table"))
}

func TestSeedTranslationManagedDefaultsFromEnvOnce_NoOpWhenConfigExists(t *testing.T) {
	ctx := context.Background()
	db := new(dynamormmocks.MockDB)
	q := new(dynamormmocks.MockQuery)

	setupMockInstanceRepoDB(db, q)
	setupMockFirstForFeatureConfigs(q)

	t.Setenv("TRANSLATION_ENABLED", "true")

	instanceRepo := repositories.NewInstanceRepository(db, "test-table", zap.NewNop())
	require.NoError(t, seedTranslationManagedDefaultsFromEnvOnce(ctx, instanceRepo, "dev", "test-table"))
}

func TestSeedTranslationManagedDefaultsFromEnvOnce_NoOpWhenEnvUnset(t *testing.T) {
	ctx := context.Background()
	db := new(dynamormmocks.MockDB)
	q := new(dynamormmocks.MockQuery)

	setupMockInstanceRepoDB(db, q)
	q.On("First", mock.AnythingOfType("*models.InstanceTranslationConfig")).Return(dynamormerrors.ErrItemNotFound).Once()

	t.Setenv("TRANSLATION_ENABLED", "")

	instanceRepo := repositories.NewInstanceRepository(db, "test-table", zap.NewNop())
	require.NoError(t, seedTranslationManagedDefaultsFromEnvOnce(ctx, instanceRepo, "dev", "test-table"))
}

func TestSeedTranslationManagedDefaultsFromEnvOnce_SeedsFromEnv(t *testing.T) {
	ctx := context.Background()
	db := new(dynamormmocks.MockDB)
	q := new(dynamormmocks.MockQuery)

	setupMockInstanceRepoDB(db, q)
	q.On("First", mock.AnythingOfType("*models.InstanceTranslationConfig")).Return(dynamormerrors.ErrItemNotFound).Once()
	q.On("First", mock.AnythingOfType("*models.InstanceTranslationConfig")).Run(func(args mock.Arguments) {
		out := args.Get(0).(*models.InstanceTranslationConfig)
		out.PK = "INSTANCE#CONFIG"
		out.SK = models.SKTranslationConfig
		out.Managed = nil
		out.Override = nil
	}).Return(nil).Maybe()

	t.Setenv("TRANSLATION_ENABLED", "yes")

	instanceRepo := repositories.NewInstanceRepository(db, "test-table", zap.NewNop())
	require.NoError(t, seedTranslationManagedDefaultsFromEnvOnce(ctx, instanceRepo, "dev", "test-table"))

	cfg, err := instanceRepo.GetTranslationConfig(ctx)
	require.NoError(t, err)
	require.NotNil(t, cfg.Managed)
	assert.True(t, cfg.Managed.Enabled)
}

func TestSeedTipsManagedDefaultsFromEnvOnce_NoOpWhenConfigExists(t *testing.T) {
	ctx := context.Background()
	db := new(dynamormmocks.MockDB)
	q := new(dynamormmocks.MockQuery)

	setupMockInstanceRepoDB(db, q)
	setupMockFirstForFeatureConfigs(q)

	t.Setenv("TIP_ENABLED", "true")
	t.Setenv("TIP_CHAIN_ID", "10")
	t.Setenv("TIP_CONTRACT_ADDRESS", "0xabc")

	instanceRepo := repositories.NewInstanceRepository(db, "test-table", zap.NewNop())
	require.NoError(t, seedTipsManagedDefaultsFromEnvOnce(ctx, instanceRepo, "dev", "test-table"))
}

func TestSeedTipsManagedDefaultsFromEnvOnce_NoOpWhenNoEnv(t *testing.T) {
	ctx := context.Background()
	db := new(dynamormmocks.MockDB)
	q := new(dynamormmocks.MockQuery)

	setupMockInstanceRepoDB(db, q)
	q.On("First", mock.AnythingOfType("*models.InstanceTipsConfig")).Return(dynamormerrors.ErrItemNotFound).Once()

	t.Setenv("TIP_ENABLED", "")
	t.Setenv("TIP_CHAIN_ID", "")
	t.Setenv("TIP_CONTRACT_ADDRESS", "")

	instanceRepo := repositories.NewInstanceRepository(db, "test-table", zap.NewNop())
	require.NoError(t, seedTipsManagedDefaultsFromEnvOnce(ctx, instanceRepo, "dev", "test-table"))
}

func TestSeedTipsManagedDefaultsFromEnvOnce_SeedsFromEnv(t *testing.T) {
	ctx := context.Background()
	db := new(dynamormmocks.MockDB)
	q := new(dynamormmocks.MockQuery)

	setupMockInstanceRepoDB(db, q)
	q.On("First", mock.AnythingOfType("*models.InstanceTipsConfig")).Return(dynamormerrors.ErrItemNotFound).Once()
	q.On("First", mock.AnythingOfType("*models.InstanceTipsConfig")).Run(func(args mock.Arguments) {
		out := args.Get(0).(*models.InstanceTipsConfig)
		out.PK = "INSTANCE#CONFIG"
		out.SK = models.SKTipsConfig
		out.Managed = nil
		out.Override = nil
	}).Return(nil).Maybe()

	t.Setenv("TIP_ENABLED", "true")
	t.Setenv("TIP_CHAIN_ID", "10")
	t.Setenv("TIP_CONTRACT_ADDRESS", " 0xabc ")

	instanceRepo := repositories.NewInstanceRepository(db, "test-table", zap.NewNop())
	require.NoError(t, seedTipsManagedDefaultsFromEnvOnce(ctx, instanceRepo, "dev", "test-table"))

	cfg, err := instanceRepo.GetTipsConfig(ctx)
	require.NoError(t, err)
	require.NotNil(t, cfg.Managed)
	assert.True(t, cfg.Managed.Enabled)
	assert.Equal(t, 10, cfg.Managed.ChainID)
	assert.Equal(t, "0xabc", cfg.Managed.ContractAddress)
}

func TestUpEnv_syncInstanceFeatureConfig_NoOpsWhenNoProvisioningAndNoEnv(t *testing.T) {
	t.Setenv("LESSER_HOST_URL", "")
	t.Setenv("LESSER_HOST_ATTESTATIONS_URL", "")
	t.Setenv("LESSER_HOST_INSTANCE_KEY_ARN", "")
	t.Setenv("TRANSLATION_ENABLED", "")
	t.Setenv("TIP_ENABLED", "")
	t.Setenv("TIP_CHAIN_ID", "")
	t.Setenv("TIP_CONTRACT_ADDRESS", "")

	env := &upEnv{
		args: upArgs{},
		stages: []naming.Stage{
			naming.StageDev,
		},
		newDB: func() (theorydb.DB, error) {
			return nil, errors.New("unexpected db call")
		},
	}
	receipt := &upReceipt{
		Stages: map[string]*stageReceipt{
			string(naming.StageDev): {TableName: "test-table"},
		},
	}

	require.NoError(t, env.syncInstanceFeatureConfig(context.Background(), receipt))
}

func TestUpEnv_syncInstanceFeatureConfig_NoOpsWhenEnvOrReceiptNil(t *testing.T) {
	var env *upEnv
	require.NoError(t, env.syncInstanceFeatureConfig(context.Background(), &upReceipt{}))

	nonNil := &upEnv{}
	require.NoError(t, nonNil.syncInstanceFeatureConfig(context.Background(), nil))
}

func TestUpEnv_syncInstanceFeatureConfig_ReturnsErrorWhenDBFactoryFails(t *testing.T) {
	env := &upEnv{
		args: upArgs{ProvisioningInputPath: "provisioning.json"},
		stages: []naming.Stage{
			naming.StageDev,
		},
		newDB: func() (theorydb.DB, error) {
			return nil, errors.New("boom")
		},
	}
	receipt := &upReceipt{
		Stages: map[string]*stageReceipt{
			string(naming.StageDev): {TableName: "test-table"},
		},
	}

	err := env.syncInstanceFeatureConfig(context.Background(), receipt)
	require.Error(t, err)
}

func TestUpEnv_syncInstanceFeatureConfig_AppliesProvisioningConfig(t *testing.T) {
	ctx := context.Background()

	db := new(dynamormmocks.MockDB)
	q := new(dynamormmocks.MockQuery)

	setupMockInstanceRepoDB(db, q)
	setupMockFirstForFeatureConfigs(q)
	db.On("Close").Return(nil).Once()

	env := &upEnv{
		args: upArgs{
			ProvisioningInputPath:     "provisioning.json",
			LesserHostURL:             "https://trust.example",
			LesserHostAttestationsURL: "https://attest.example",
			LesserHostInstanceKeyARN:  "arn:aws:secretsmanager:us-east-1:123:secret:abc",
		},
		stages: []naming.Stage{
			naming.StageDev,
		},
		newDB: func() (theorydb.DB, error) {
			return db, nil
		},
	}

	receipt := &upReceipt{
		Stages: map[string]*stageReceipt{
			string(naming.StageDev): {TableName: "test-table"},
		},
	}

	require.NoError(t, env.syncInstanceFeatureConfig(ctx, receipt))
}

func TestUpEnv_syncInstanceFeatureConfig_SeedsFromEnvWhenProvisioningMissing(t *testing.T) {
	ctx := context.Background()

	t.Setenv("TIP_ENABLED", "true")

	db := new(dynamormmocks.MockDB)
	q := new(dynamormmocks.MockQuery)

	setupMockInstanceRepoDB(db, q)
	setupMockFirstForFeatureConfigs(q)
	db.On("Close").Return(nil).Once()

	env := &upEnv{
		args: upArgs{},
		stages: []naming.Stage{
			naming.StageDev,
		},
		newDB: func() (theorydb.DB, error) {
			return db, nil
		},
	}
	receipt := &upReceipt{
		Stages: map[string]*stageReceipt{
			string(naming.StageDev): {TableName: "test-table"},
		},
	}

	require.NoError(t, env.syncInstanceFeatureConfig(ctx, receipt))
}

func TestUpEnv_syncInstanceFeatureConfig_SkipsStagesMissingReceiptOrTableName(t *testing.T) {
	env := &upEnv{
		args: upArgs{ProvisioningInputPath: "provisioning.json"},
		stages: []naming.Stage{
			naming.StageDev,
			naming.StageStaging,
		},
		newDB: func() (theorydb.DB, error) {
			return nil, errors.New("unexpected db call")
		},
	}
	receipt := &upReceipt{
		Stages: map[string]*stageReceipt{
			string(naming.StageDev):     nil,             // missing stage receipt should be skipped
			string(naming.StageStaging): {TableName: ""}, // empty table name should be skipped
		},
	}

	require.NoError(t, env.syncInstanceFeatureConfig(context.Background(), receipt))
}

func TestUpEnv_syncInstanceFeatureConfig_ProvisioningErrorRestoresMainTableName(t *testing.T) {
	ctx := context.Background()

	db := new(dynamormmocks.MockDB)
	q := new(dynamormmocks.MockQuery)
	setupMockInstanceRepoDB(db, q)
	q.On("First", mock.AnythingOfType("*models.InstanceTrustConfig")).Return(errors.New("boom")).Maybe()
	db.On("Close").Return(nil).Once()

	prev := models.MainTableName
	models.MainTableName = "original"
	t.Cleanup(func() { models.MainTableName = prev })

	env := &upEnv{
		args: upArgs{ProvisioningInputPath: "provisioning.json"},
		stages: []naming.Stage{
			naming.StageDev,
		},
		newDB: func() (theorydb.DB, error) {
			return db, nil
		},
	}
	receipt := &upReceipt{
		Stages: map[string]*stageReceipt{
			string(naming.StageDev): {TableName: "test-table"},
		},
	}

	err := env.syncInstanceFeatureConfig(ctx, receipt)
	require.Error(t, err)
	require.Equal(t, "original", models.MainTableName)
}

func TestUpEnv_syncInstanceFeatureConfig_SeedErrorRestoresMainTableName(t *testing.T) {
	ctx := context.Background()

	t.Setenv("TIP_ENABLED", "true")

	db := new(dynamormmocks.MockDB)
	q := new(dynamormmocks.MockQuery)
	setupMockInstanceRepoDB(db, q)
	q.On("First", mock.AnythingOfType("*models.InstanceTrustConfig")).Return(errors.New("boom")).Maybe()
	db.On("Close").Return(nil).Once()

	prev := models.MainTableName
	models.MainTableName = "original"
	t.Cleanup(func() { models.MainTableName = prev })

	env := &upEnv{
		args: upArgs{},
		stages: []naming.Stage{
			naming.StageDev,
		},
		newDB: func() (theorydb.DB, error) {
			return db, nil
		},
	}
	receipt := &upReceipt{
		Stages: map[string]*stageReceipt{
			string(naming.StageDev): {TableName: "test-table"},
		},
	}

	err := env.syncInstanceFeatureConfig(ctx, receipt)
	require.Error(t, err)
	require.Equal(t, "original", models.MainTableName)
}

func TestSeedManagedConfigFromEnvOnce_ReturnsErrorWhenTrustExistsCheckFails(t *testing.T) {
	ctx := context.Background()

	db := new(dynamormmocks.MockDB)
	q := new(dynamormmocks.MockQuery)
	setupMockInstanceRepoDB(db, q)
	q.On("First", mock.AnythingOfType("*models.InstanceTrustConfig")).Return(errors.New("boom")).Maybe()

	instanceRepo := repositories.NewInstanceRepository(db, "test-table", zap.NewNop())
	require.Error(t, seedManagedConfigFromEnvOnce(ctx, instanceRepo, "dev", "test-table"))
}

func TestEnsureAndApplyManagedProvisioningConfig_ReturnsErrorWhenEnsureFails(t *testing.T) {
	ctx := context.Background()

	db := new(dynamormmocks.MockDB)
	q := new(dynamormmocks.MockQuery)

	setupMockInstanceRepoDB(db, q)
	q.On("First", mock.AnythingOfType("*models.InstanceTrustConfig")).Return(errors.New("boom")).Maybe()

	instanceRepo := repositories.NewInstanceRepository(db, "test-table", zap.NewNop())
	require.Error(t, ensureAndApplyManagedProvisioningConfig(ctx, instanceRepo, upArgs{LesserHostURL: "https://trust.example"}))
}

func TestEnsureAndApplyManagedProvisioningConfig_ReturnsErrorWhenSetFails(t *testing.T) {
	ctx := context.Background()

	db := new(dynamormmocks.MockDB)
	q := new(dynamormmocks.MockQuery)

	q.On("Update", mock.Anything).Return(errors.New("boom")).Once()
	setupMockInstanceRepoDB(db, q)
	setupMockFirstForFeatureConfigs(q)

	instanceRepo := repositories.NewInstanceRepository(db, "test-table", zap.NewNop())
	require.Error(t, ensureAndApplyManagedProvisioningConfig(ctx, instanceRepo, upArgs{LesserHostURL: "https://trust.example"}))
}

func TestEnsureAndApplyManagedProvisioningConfig_ReturnsErrorWhenEnsureTranslationFails(t *testing.T) {
	ctx := context.Background()

	db := new(dynamormmocks.MockDB)
	q := new(dynamormmocks.MockQuery)

	q.On("First", mock.AnythingOfType("*models.InstanceTranslationConfig")).Return(errors.New("boom")).Once()
	setupMockInstanceRepoDB(db, q)
	setupMockFirstForFeatureConfigs(q)

	instanceRepo := repositories.NewInstanceRepository(db, "test-table", zap.NewNop())
	require.Error(t, ensureAndApplyManagedProvisioningConfig(ctx, instanceRepo, upArgs{LesserHostURL: "https://trust.example"}))
}

func TestSeedManagedConfigFromEnvOnce_ReturnsErrorWhenTranslationExistsCheckFails(t *testing.T) {
	ctx := context.Background()

	db := new(dynamormmocks.MockDB)
	q := new(dynamormmocks.MockQuery)

	setupMockInstanceRepoDB(db, q)
	q.On("First", mock.AnythingOfType("*models.InstanceTrustConfig")).Run(func(args mock.Arguments) {
		out := args.Get(0).(*models.InstanceTrustConfig)
		out.PK = "INSTANCE#CONFIG"
		out.SK = models.SKTrustConfig
		out.Managed = nil
		out.Override = nil
	}).Return(nil).Maybe()
	q.On("First", mock.AnythingOfType("*models.InstanceTranslationConfig")).Return(errors.New("boom")).Maybe()

	instanceRepo := repositories.NewInstanceRepository(db, "test-table", zap.NewNop())
	require.Error(t, seedManagedConfigFromEnvOnce(ctx, instanceRepo, "dev", "test-table"))
}

func TestSeedTipsManagedDefaultsFromEnvOnce_ReturnsErrorWhenExistsCheckFails(t *testing.T) {
	ctx := context.Background()

	db := new(dynamormmocks.MockDB)
	q := new(dynamormmocks.MockQuery)

	setupMockInstanceRepoDB(db, q)
	q.On("First", mock.AnythingOfType("*models.InstanceTipsConfig")).Return(errors.New("boom")).Maybe()

	instanceRepo := repositories.NewInstanceRepository(db, "test-table", zap.NewNop())
	require.Error(t, seedTipsManagedDefaultsFromEnvOnce(ctx, instanceRepo, "dev", "test-table"))
}

func TestSeedTrustManagedDefaultsFromEnvOnce_ReturnsErrorWhenEnsureFails(t *testing.T) {
	ctx := context.Background()

	db := new(dynamormmocks.MockDB)
	q := new(dynamormmocks.MockQuery)

	q.On("Create").Return(errors.New("boom")).Once()
	setupMockInstanceRepoDB(db, q)
	q.On("First", mock.AnythingOfType("*models.InstanceTrustConfig")).Return(dynamormerrors.ErrItemNotFound).Maybe()

	t.Setenv("LESSER_HOST_URL", "https://trust.example")
	t.Setenv("LESSER_HOST_INSTANCE_KEY_ARN", "arn:aws:secretsmanager:us-east-1:123:secret:abc")

	instanceRepo := repositories.NewInstanceRepository(db, "test-table", zap.NewNop())
	require.Error(t, seedTrustManagedDefaultsFromEnvOnce(ctx, instanceRepo, "dev", "test-table"))
}

func TestSeedTranslationManagedDefaultsFromEnvOnce_ReturnsErrorWhenSetFails(t *testing.T) {
	ctx := context.Background()

	db := new(dynamormmocks.MockDB)
	q := new(dynamormmocks.MockQuery)

	q.On("Update", mock.Anything).Return(errors.New("boom")).Once()
	setupMockInstanceRepoDB(db, q)
	q.On("First", mock.AnythingOfType("*models.InstanceTranslationConfig")).Return(dynamormerrors.ErrItemNotFound).Maybe()

	t.Setenv("TRANSLATION_ENABLED", "true")

	instanceRepo := repositories.NewInstanceRepository(db, "test-table", zap.NewNop())
	require.Error(t, seedTranslationManagedDefaultsFromEnvOnce(ctx, instanceRepo, "dev", "test-table"))
}

func TestSeedTipsManagedDefaultsFromEnvOnce_ReturnsErrorWhenSetFails(t *testing.T) {
	ctx := context.Background()

	db := new(dynamormmocks.MockDB)
	q := new(dynamormmocks.MockQuery)

	q.On("Update", mock.Anything).Return(errors.New("boom")).Once()
	setupMockInstanceRepoDB(db, q)
	q.On("First", mock.AnythingOfType("*models.InstanceTipsConfig")).Return(dynamormerrors.ErrItemNotFound).Maybe()

	t.Setenv("TIP_ENABLED", "true")

	instanceRepo := repositories.NewInstanceRepository(db, "test-table", zap.NewNop())
	require.Error(t, seedTipsManagedDefaultsFromEnvOnce(ctx, instanceRepo, "dev", "test-table"))
}

func TestSeedTranslationManagedDefaultsFromEnvOnce_ReturnsErrorWhenEnsureFails(t *testing.T) {
	ctx := context.Background()

	db := new(dynamormmocks.MockDB)
	q := new(dynamormmocks.MockQuery)

	q.On("Create").Return(errors.New("boom")).Once()
	setupMockInstanceRepoDB(db, q)
	q.On("First", mock.AnythingOfType("*models.InstanceTranslationConfig")).Return(dynamormerrors.ErrItemNotFound).Maybe()

	t.Setenv("TRANSLATION_ENABLED", "true")

	instanceRepo := repositories.NewInstanceRepository(db, "test-table", zap.NewNop())
	require.Error(t, seedTranslationManagedDefaultsFromEnvOnce(ctx, instanceRepo, "dev", "test-table"))
}

func TestSeedTipsManagedDefaultsFromEnvOnce_ReturnsErrorWhenEnsureFails(t *testing.T) {
	ctx := context.Background()

	db := new(dynamormmocks.MockDB)
	q := new(dynamormmocks.MockQuery)

	q.On("Create").Return(errors.New("boom")).Once()
	setupMockInstanceRepoDB(db, q)
	q.On("First", mock.AnythingOfType("*models.InstanceTipsConfig")).Return(dynamormerrors.ErrItemNotFound).Maybe()

	t.Setenv("TIP_ENABLED", "true")

	instanceRepo := repositories.NewInstanceRepository(db, "test-table", zap.NewNop())
	require.Error(t, seedTipsManagedDefaultsFromEnvOnce(ctx, instanceRepo, "dev", "test-table"))
}

func TestSeedManagedConfigFromEnvOnce_ReturnsErrorWhenTipsSeedFails(t *testing.T) {
	ctx := context.Background()

	db := new(dynamormmocks.MockDB)
	q := new(dynamormmocks.MockQuery)

	setupMockInstanceRepoDB(db, q)
	q.On("First", mock.AnythingOfType("*models.InstanceTrustConfig")).Run(func(args mock.Arguments) {
		out := args.Get(0).(*models.InstanceTrustConfig)
		out.PK = "INSTANCE#CONFIG"
		out.SK = models.SKTrustConfig
	}).Return(nil).Maybe()
	q.On("First", mock.AnythingOfType("*models.InstanceTranslationConfig")).Run(func(args mock.Arguments) {
		out := args.Get(0).(*models.InstanceTranslationConfig)
		out.PK = "INSTANCE#CONFIG"
		out.SK = models.SKTranslationConfig
	}).Return(nil).Maybe()
	q.On("First", mock.AnythingOfType("*models.InstanceTipsConfig")).Return(errors.New("boom")).Maybe()

	instanceRepo := repositories.NewInstanceRepository(db, "test-table", zap.NewNop())
	require.Error(t, seedManagedConfigFromEnvOnce(ctx, instanceRepo, "dev", "test-table"))
}

func TestEnsureAndApplyManagedProvisioningConfig_ReturnsErrorWhenEnsureAIFails(t *testing.T) {
	ctx := context.Background()

	db := new(dynamormmocks.MockDB)
	q := new(dynamormmocks.MockQuery)

	q.On("First", mock.AnythingOfType("*models.AIInstanceConfig")).Return(errors.New("boom")).Once()
	setupMockInstanceRepoDB(db, q)
	setupMockFirstForFeatureConfigs(q)

	instanceRepo := repositories.NewInstanceRepository(db, "test-table", zap.NewNop())
	require.Error(t, ensureAndApplyManagedProvisioningConfig(ctx, instanceRepo, upArgs{LesserHostURL: "https://trust.example"}))
}

func TestEnsureAndApplyManagedProvisioningConfig_ReturnsErrorWhenEnsureTipsFails(t *testing.T) {
	ctx := context.Background()

	db := new(dynamormmocks.MockDB)
	q := new(dynamormmocks.MockQuery)

	q.On("First", mock.AnythingOfType("*models.InstanceTipsConfig")).Return(errors.New("boom")).Once()
	setupMockInstanceRepoDB(db, q)
	setupMockFirstForFeatureConfigs(q)

	instanceRepo := repositories.NewInstanceRepository(db, "test-table", zap.NewNop())
	require.Error(t, ensureAndApplyManagedProvisioningConfig(ctx, instanceRepo, upArgs{LesserHostURL: "https://trust.example"}))
}

func TestEnsureAndApplyManagedProvisioningConfig_ReturnsErrorWhenSetTranslationFails(t *testing.T) {
	ctx := context.Background()

	db := new(dynamormmocks.MockDB)
	q := new(dynamormmocks.MockQuery)

	translationEnabled := true

	q.On("Update", mock.Anything).Return(nil).Once()
	q.On("Update", mock.Anything).Return(errors.New("boom")).Once()
	setupMockInstanceRepoDB(db, q)
	setupMockFirstForFeatureConfigs(q)

	instanceRepo := repositories.NewInstanceRepository(db, "test-table", zap.NewNop())
	require.Error(t, ensureAndApplyManagedProvisioningConfig(ctx, instanceRepo, upArgs{
		LesserHostURL:      "https://trust.example",
		TranslationEnabled: &translationEnabled,
	}))
}

func TestEnsureAndApplyManagedProvisioningConfig_ReturnsErrorWhenSetTipsFails(t *testing.T) {
	ctx := context.Background()

	db := new(dynamormmocks.MockDB)
	q := new(dynamormmocks.MockQuery)

	translationEnabled := true
	tipEnabled := true

	q.On("Update", mock.Anything).Return(nil).Once()
	q.On("Update", mock.Anything).Return(nil).Once()
	q.On("Update", mock.Anything).Return(errors.New("boom")).Once()
	setupMockInstanceRepoDB(db, q)
	setupMockFirstForFeatureConfigs(q)

	instanceRepo := repositories.NewInstanceRepository(db, "test-table", zap.NewNop())
	require.Error(t, ensureAndApplyManagedProvisioningConfig(ctx, instanceRepo, upArgs{
		LesserHostURL:      "https://trust.example",
		TranslationEnabled: &translationEnabled,
		TipEnabled:         &tipEnabled,
	}))
}

func TestEnsureAndApplyManagedProvisioningConfig_ReturnsErrorWhenSetAIFails(t *testing.T) {
	ctx := context.Background()

	db := new(dynamormmocks.MockDB)
	q := new(dynamormmocks.MockQuery)

	translationEnabled := true
	tipEnabled := true
	aiEnabled := true

	q.On("Update", mock.Anything).Return(nil).Once()
	q.On("Update", mock.Anything).Return(nil).Once()
	q.On("Update", mock.Anything).Return(nil).Once()
	q.On("Update", mock.Anything).Return(errors.New("boom")).Once()
	setupMockInstanceRepoDB(db, q)
	setupMockFirstForFeatureConfigs(q)

	instanceRepo := repositories.NewInstanceRepository(db, "test-table", zap.NewNop())
	require.Error(t, ensureAndApplyManagedProvisioningConfig(ctx, instanceRepo, upArgs{
		LesserHostURL:      "https://trust.example",
		TranslationEnabled: &translationEnabled,
		TipEnabled:         &tipEnabled,
		AIEnabled:          &aiEnabled,
	}))
}

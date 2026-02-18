package repositories

import (
	"context"
	"testing"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/pkg/core"
	dynamormerrors "github.com/theory-cloud/tabletheory/pkg/errors"
	dynamormmocks "github.com/theory-cloud/tabletheory/pkg/mocks"
	"go.uber.org/zap"
)

type testUpdateBuilder struct{}

func (b *testUpdateBuilder) Set(string, any) core.UpdateBuilder                 { return b }
func (b *testUpdateBuilder) SetIfNotExists(string, any, any) core.UpdateBuilder { return b }
func (b *testUpdateBuilder) Add(string, any) core.UpdateBuilder                 { return b }
func (b *testUpdateBuilder) AddAll(string, any) core.UpdateBuilder              { return b }
func (b *testUpdateBuilder) Increment(string) core.UpdateBuilder                { return b }
func (b *testUpdateBuilder) Decrement(string) core.UpdateBuilder                { return b }
func (b *testUpdateBuilder) Remove(string) core.UpdateBuilder                   { return b }
func (b *testUpdateBuilder) Delete(string, any) core.UpdateBuilder              { return b }
func (b *testUpdateBuilder) AppendToList(string, any) core.UpdateBuilder        { return b }
func (b *testUpdateBuilder) PrependToList(string, any) core.UpdateBuilder       { return b }
func (b *testUpdateBuilder) RemoveFromListAt(string, int) core.UpdateBuilder    { return b }
func (b *testUpdateBuilder) SetListElement(string, int, any) core.UpdateBuilder { return b }
func (b *testUpdateBuilder) Condition(string, string, any) core.UpdateBuilder   { return b }
func (b *testUpdateBuilder) OrCondition(string, string, any) core.UpdateBuilder { return b }
func (b *testUpdateBuilder) ConditionExists(string) core.UpdateBuilder          { return b }
func (b *testUpdateBuilder) ConditionNotExists(string) core.UpdateBuilder       { return b }
func (b *testUpdateBuilder) ConditionVersion(int64) core.UpdateBuilder          { return b }
func (b *testUpdateBuilder) ReturnValues(string) core.UpdateBuilder             { return b }
func (b *testUpdateBuilder) Execute() error                                     { return nil }
func (b *testUpdateBuilder) ExecuteWithResult(any) error                        { return nil }

type testUpdateBuilderError struct {
	testUpdateBuilder
	err error
}

func (b *testUpdateBuilderError) Execute() error              { return b.err }
func (b *testUpdateBuilderError) ExecuteWithResult(any) error { return b.err }

func setupMockDB(db *dynamormmocks.MockDB, q *dynamormmocks.MockQuery) {
	db.On("WithContext", mock.Anything).Return(db).Maybe()
	db.On("Model", mock.Anything).Return(q).Maybe()
	q.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(q).Maybe()
}

func TestInstanceRepository_GetTrustConfig_DefaultWhenMissingAndCaches(t *testing.T) {
	ctx := context.Background()
	db := new(dynamormmocks.MockDB)
	q := new(dynamormmocks.MockQuery)

	setupMockDB(db, q)
	q.On("First", mock.AnythingOfType("*models.InstanceTrustConfig")).Return(dynamormerrors.ErrItemNotFound).Once()

	repo := NewInstanceRepository(db, "test-table", zap.NewNop())
	cfg, err := repo.GetTrustConfig(ctx)
	require.NoError(t, err)
	require.NotNil(t, cfg)
	assert.Equal(t, models.SKTrustConfig, cfg.SK)
	assert.NotNil(t, cfg.Managed)

	// Cached path should not hit DB again.
	cfg2, err := repo.GetTrustConfig(ctx)
	require.NoError(t, err)
	assert.Same(t, cfg, cfg2)
}

func TestInstanceRepository_EnsureTrustConfig_CreateWhenMissing(t *testing.T) {
	ctx := context.Background()
	db := new(dynamormmocks.MockDB)
	q := new(dynamormmocks.MockQuery)

	setupMockDB(db, q)

	q.On("First", mock.AnythingOfType("*models.InstanceTrustConfig")).Return(dynamormerrors.ErrItemNotFound).Once()
	q.On("Create").Return(nil).Once()

	repo := NewInstanceRepository(db, "test-table", zap.NewNop())
	cfg, err := repo.EnsureTrustConfig(ctx)
	require.NoError(t, err)
	require.NotNil(t, cfg)
	assert.Equal(t, models.SKTrustConfig, cfg.SK)
}

func TestInstanceRepository_EffectiveTrustConfig_PrefersOverrideAndDefaultsAttestationsToBase(t *testing.T) {
	ctx := context.Background()
	db := new(dynamormmocks.MockDB)
	q := new(dynamormmocks.MockQuery)

	setupMockDB(db, q)
	q.On("First", mock.AnythingOfType("*models.InstanceTrustConfig")).Run(func(args mock.Arguments) {
		out := args.Get(0).(*models.InstanceTrustConfig)
		out.PK = "INSTANCE#CONFIG"
		out.SK = models.SKTrustConfig
		out.Managed = &models.InstanceTrustConfigManaged{
			BaseURL:              "https://managed.example/",
			AttestationsURL:      "",
			InstanceKeySecretARN: " arn:aws:secretsmanager:us-east-1:123:secret:abc ",
		}
		overrideBase := "https://override.example/"
		out.Override = &models.InstanceTrustConfigOverride{
			BaseURL: &overrideBase,
		}
	}).Return(nil).Once()

	repo := NewInstanceRepository(db, "test-table", zap.NewNop())
	effective, err := repo.EffectiveTrustConfig(ctx)
	require.NoError(t, err)
	require.NotNil(t, effective)
	assert.Equal(t, "https://override.example", effective.TrustBaseURL)
	assert.Equal(t, "https://override.example", effective.AttestationsBaseURL)
	assert.Equal(t, "arn:aws:secretsmanager:us-east-1:123:secret:abc", effective.InstanceKeySecretARN)
	assert.True(t, effective.PublicAttestationsEnabled)
	assert.True(t, effective.TrustProxyEnabled)
}

func TestInstanceRepository_EffectiveTranslationEnabled_OverrideWins(t *testing.T) {
	ctx := context.Background()
	db := new(dynamormmocks.MockDB)
	q := new(dynamormmocks.MockQuery)

	setupMockDB(db, q)
	q.On("First", mock.AnythingOfType("*models.InstanceTranslationConfig")).Run(func(args mock.Arguments) {
		out := args.Get(0).(*models.InstanceTranslationConfig)
		out.PK = "INSTANCE#CONFIG"
		out.SK = models.SKTranslationConfig
		out.Managed = &models.InstanceTranslationConfigManaged{Enabled: false}
		override := true
		out.Override = &models.InstanceTranslationConfigOverride{Enabled: &override}
	}).Return(nil).Once()

	repo := NewInstanceRepository(db, "test-table", zap.NewNop())
	enabled, err := repo.EffectiveTranslationEnabled(ctx)
	require.NoError(t, err)
	assert.True(t, enabled)
}

func TestInstanceRepository_EffectiveTipsConfig_OverrideWinsPerField(t *testing.T) {
	ctx := context.Background()
	db := new(dynamormmocks.MockDB)
	q := new(dynamormmocks.MockQuery)

	setupMockDB(db, q)
	q.On("First", mock.AnythingOfType("*models.InstanceTipsConfig")).Run(func(args mock.Arguments) {
		out := args.Get(0).(*models.InstanceTipsConfig)
		out.PK = "INSTANCE#CONFIG"
		out.SK = models.SKTipsConfig
		out.Managed = &models.InstanceTipsConfigManaged{
			Enabled:         false,
			ChainID:         1,
			ContractAddress: "0xmanaged",
		}
		overrideEnabled := true
		overrideChain := 10
		overrideContract := " 0xoverride "
		out.Override = &models.InstanceTipsConfigOverride{
			Enabled:         &overrideEnabled,
			ChainID:         &overrideChain,
			ContractAddress: &overrideContract,
		}
	}).Return(nil).Once()

	repo := NewInstanceRepository(db, "test-table", zap.NewNop())
	effective, err := repo.EffectiveTipsConfig(ctx)
	require.NoError(t, err)
	require.NotNil(t, effective)
	assert.True(t, effective.Enabled)
	assert.Equal(t, 10, effective.ChainID)
	assert.Equal(t, "0xoverride", effective.ContractAddress)
}

func TestInstanceRepository_EffectiveAIConfig_UsesLegacyWhenManagedMissing(t *testing.T) {
	ctx := context.Background()
	db := new(dynamormmocks.MockDB)
	q := new(dynamormmocks.MockQuery)

	setupMockDB(db, q)
	q.On("First", mock.AnythingOfType("*models.AIInstanceConfig")).Run(func(args mock.Arguments) {
		out := args.Get(0).(*models.AIInstanceConfig)
		out.PK = "INSTANCE#CONFIG"
		out.SK = models.SKAIConfig
		out.Managed = nil
		out.Override = nil
		out.LegacyAIEnabled = true
		out.LegacyModerationEnabled = true
		out.LegacyNSFWDetectionEnabled = false
		out.LegacySpamDetectionEnabled = true
		out.LegacyPIIDetectionEnabled = false
		out.LegacyAIContentDetection = false
	}).Return(nil).Once()

	repo := NewInstanceRepository(db, "test-table", zap.NewNop())
	effective, err := repo.EffectiveAIConfig(ctx)
	require.NoError(t, err)
	require.NotNil(t, effective)
	assert.True(t, effective.AIEnabled)
	assert.True(t, effective.ModerationEnabled)
	assert.False(t, effective.NSFWDetectionEnabled)
	assert.True(t, effective.SpamDetectionEnabled)
}

func TestInstanceRepository_EffectiveAIConfig_ReturnsErrorWhenConfigGetFails(t *testing.T) {
	ctx := context.Background()
	db := new(dynamormmocks.MockDB)
	q := new(dynamormmocks.MockQuery)

	setupMockDB(db, q)
	q.On("First", mock.AnythingOfType("*models.AIInstanceConfig")).Return(assert.AnError).Once()

	repo := NewInstanceRepository(db, "test-table", zap.NewNop())
	effective, err := repo.EffectiveAIConfig(ctx)
	require.Error(t, err)
	assert.Nil(t, effective)
}

func TestInstanceRepository_ClearTrustOverride_RemovesOverrideAttribute(t *testing.T) {
	ctx := context.Background()
	db := new(dynamormmocks.MockDB)
	q := new(dynamormmocks.MockQuery)
	builder := &testUpdateBuilder{}

	setupMockDB(db, q)
	q.On("First", mock.AnythingOfType("*models.InstanceTrustConfig")).Run(func(args mock.Arguments) {
		out := args.Get(0).(*models.InstanceTrustConfig)
		*out = *models.NewInstanceTrustConfig()
		overrideBase := "https://override.example"
		out.Override = &models.InstanceTrustConfigOverride{BaseURL: &overrideBase}
	}).Return(nil).Once()
	q.On("UpdateBuilder").Return(builder).Once()

	repo := NewInstanceRepository(db, "test-table", zap.NewNop())
	require.NoError(t, repo.ClearTrustOverride(ctx))

	cached, ok := repo.getCachedTrustConfig()
	require.True(t, ok)
	assert.Nil(t, cached.Override)
}

func TestInstanceRepository_ClearTrustOverride_ReturnsErrorWhenUpdateFails(t *testing.T) {
	ctx := context.Background()
	db := new(dynamormmocks.MockDB)
	q := new(dynamormmocks.MockQuery)
	builder := &testUpdateBuilderError{err: assert.AnError}

	setupMockDB(db, q)
	q.On("First", mock.AnythingOfType("*models.InstanceTrustConfig")).Run(func(args mock.Arguments) {
		out := args.Get(0).(*models.InstanceTrustConfig)
		out.PK = "INSTANCE#CONFIG"
		out.SK = models.SKTrustConfig
	}).Return(nil).Once()
	q.On("UpdateBuilder").Return(builder).Once()

	repo := NewInstanceRepository(db, "test-table", zap.NewNop())
	require.Error(t, repo.ClearTrustOverride(ctx))
}

func TestInstanceRepository_ConfigExists_ReturnsFalseOnNotFound(t *testing.T) {
	ctx := context.Background()
	db := new(dynamormmocks.MockDB)
	q := new(dynamormmocks.MockQuery)

	setupMockDB(db, q)
	q.On("First", mock.AnythingOfType("*models.InstanceTrustConfig")).Return(dynamormerrors.ErrItemNotFound).Once()
	q.On("First", mock.AnythingOfType("*models.InstanceTranslationConfig")).Return(dynamormerrors.ErrItemNotFound).Once()
	q.On("First", mock.AnythingOfType("*models.InstanceTipsConfig")).Return(dynamormerrors.ErrItemNotFound).Once()
	q.On("First", mock.AnythingOfType("*models.AIInstanceConfig")).Return(dynamormerrors.ErrItemNotFound).Once()

	repo := NewInstanceRepository(db, "test-table", zap.NewNop())

	ok, err := repo.TrustConfigExists(ctx)
	require.NoError(t, err)
	assert.False(t, ok)

	ok, err = repo.TranslationConfigExists(ctx)
	require.NoError(t, err)
	assert.False(t, ok)

	ok, err = repo.TipsConfigExists(ctx)
	require.NoError(t, err)
	assert.False(t, ok)

	ok, err = repo.AIConfigExists(ctx)
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestInstanceRepository_ConfigExists_ReturnsTrueAndCaches(t *testing.T) {
	ctx := context.Background()
	db := new(dynamormmocks.MockDB)
	q := new(dynamormmocks.MockQuery)

	setupMockDB(db, q)
	q.On("First", mock.AnythingOfType("*models.InstanceTrustConfig")).Run(func(args mock.Arguments) {
		out := args.Get(0).(*models.InstanceTrustConfig)
		out.PK = "INSTANCE#CONFIG"
		out.SK = models.SKTrustConfig
	}).Return(nil).Once()

	repo := NewInstanceRepository(db, "test-table", zap.NewNop())

	ok, err := repo.TrustConfigExists(ctx)
	require.NoError(t, err)
	assert.True(t, ok)

	cached, cacheOK := repo.getCachedTrustConfig()
	require.True(t, cacheOK)
	assert.Equal(t, models.SKTrustConfig, cached.SK)
}

func TestInstanceRepository_TranslationConfigExists_ReturnsTrueAndCaches(t *testing.T) {
	ctx := context.Background()
	db := new(dynamormmocks.MockDB)
	q := new(dynamormmocks.MockQuery)

	setupMockDB(db, q)
	q.On("First", mock.AnythingOfType("*models.InstanceTranslationConfig")).Run(func(args mock.Arguments) {
		out := args.Get(0).(*models.InstanceTranslationConfig)
		out.PK = "INSTANCE#CONFIG"
		out.SK = models.SKTranslationConfig
	}).Return(nil).Once()

	repo := NewInstanceRepository(db, "test-table", zap.NewNop())

	ok, err := repo.TranslationConfigExists(ctx)
	require.NoError(t, err)
	assert.True(t, ok)

	cached, cacheOK := repo.getCachedTranslationConfig()
	require.True(t, cacheOK)
	assert.Equal(t, models.SKTranslationConfig, cached.SK)
}

func TestInstanceRepository_TipsConfigExists_ReturnsTrueAndCaches(t *testing.T) {
	ctx := context.Background()
	db := new(dynamormmocks.MockDB)
	q := new(dynamormmocks.MockQuery)

	setupMockDB(db, q)
	q.On("First", mock.AnythingOfType("*models.InstanceTipsConfig")).Run(func(args mock.Arguments) {
		out := args.Get(0).(*models.InstanceTipsConfig)
		out.PK = "INSTANCE#CONFIG"
		out.SK = models.SKTipsConfig
	}).Return(nil).Once()

	repo := NewInstanceRepository(db, "test-table", zap.NewNop())

	ok, err := repo.TipsConfigExists(ctx)
	require.NoError(t, err)
	assert.True(t, ok)

	cached, cacheOK := repo.getCachedTipsConfig()
	require.True(t, cacheOK)
	assert.Equal(t, models.SKTipsConfig, cached.SK)
}

func TestInstanceRepository_AIConfigExists_ReturnsTrueAndCaches(t *testing.T) {
	ctx := context.Background()
	db := new(dynamormmocks.MockDB)
	q := new(dynamormmocks.MockQuery)

	setupMockDB(db, q)
	q.On("First", mock.AnythingOfType("*models.AIInstanceConfig")).Run(func(args mock.Arguments) {
		out := args.Get(0).(*models.AIInstanceConfig)
		out.PK = "INSTANCE#CONFIG"
		out.SK = models.SKAIConfig
	}).Return(nil).Once()

	repo := NewInstanceRepository(db, "test-table", zap.NewNop())

	ok, err := repo.AIConfigExists(ctx)
	require.NoError(t, err)
	assert.True(t, ok)

	cached, cacheOK := repo.getCachedAIConfig()
	require.True(t, cacheOK)
	assert.Equal(t, models.SKAIConfig, cached.SK)
}

func TestInstanceRepository_ConfigExists_ReturnsFalseWhenKeysBlank(t *testing.T) {
	ctx := context.Background()
	db := new(dynamormmocks.MockDB)
	q := new(dynamormmocks.MockQuery)

	setupMockDB(db, q)
	q.On("First", mock.AnythingOfType("*models.InstanceTrustConfig")).Return(nil).Once()
	q.On("First", mock.AnythingOfType("*models.InstanceTranslationConfig")).Return(nil).Once()
	q.On("First", mock.AnythingOfType("*models.InstanceTipsConfig")).Return(nil).Once()
	q.On("First", mock.AnythingOfType("*models.AIInstanceConfig")).Return(nil).Once()

	repo := NewInstanceRepository(db, "test-table", zap.NewNop())

	ok, err := repo.TrustConfigExists(ctx)
	require.NoError(t, err)
	assert.False(t, ok)
	_, cacheOK := repo.getCachedTrustConfig()
	assert.False(t, cacheOK)

	ok, err = repo.TranslationConfigExists(ctx)
	require.NoError(t, err)
	assert.False(t, ok)
	_, cacheOK = repo.getCachedTranslationConfig()
	assert.False(t, cacheOK)

	ok, err = repo.TipsConfigExists(ctx)
	require.NoError(t, err)
	assert.False(t, ok)
	_, cacheOK = repo.getCachedTipsConfig()
	assert.False(t, cacheOK)

	ok, err = repo.AIConfigExists(ctx)
	require.NoError(t, err)
	assert.False(t, ok)
	_, cacheOK = repo.getCachedAIConfig()
	assert.False(t, cacheOK)
}

func TestInstanceRepository_EnsureTranslationConfig_CreateConflictFetchesExisting(t *testing.T) {
	ctx := context.Background()
	db := new(dynamormmocks.MockDB)
	q := new(dynamormmocks.MockQuery)

	setupMockDB(db, q)
	q.On("First", mock.AnythingOfType("*models.InstanceTranslationConfig")).Return(dynamormerrors.ErrItemNotFound).Once()
	q.On("Create").Return(dynamormerrors.ErrConditionFailed).Once()
	q.On("First", mock.AnythingOfType("*models.InstanceTranslationConfig")).Run(func(args mock.Arguments) {
		out := args.Get(0).(*models.InstanceTranslationConfig)
		out.PK = "INSTANCE#CONFIG"
		out.SK = models.SKTranslationConfig
		out.Managed = &models.InstanceTranslationConfigManaged{Enabled: true}
	}).Return(nil).Once()

	repo := NewInstanceRepository(db, "test-table", zap.NewNop())
	cfg, err := repo.EnsureTranslationConfig(ctx)
	require.NoError(t, err)
	require.NotNil(t, cfg)
	assert.Equal(t, models.SKTranslationConfig, cfg.SK)

	cached, ok := repo.getCachedTranslationConfig()
	require.True(t, ok)
	assert.Same(t, cfg, cached)
}

func TestInstanceRepository_EnsureTipsConfig_GetErrorPropagates(t *testing.T) {
	ctx := context.Background()
	db := new(dynamormmocks.MockDB)
	q := new(dynamormmocks.MockQuery)

	setupMockDB(db, q)
	q.On("First", mock.AnythingOfType("*models.InstanceTipsConfig")).Return(assert.AnError).Once()

	repo := NewInstanceRepository(db, "test-table", zap.NewNop())
	_, err := repo.EnsureTipsConfig(ctx)
	require.Error(t, err)
}

func TestInstanceRepository_EnsureTipsConfig_CreateErrorPropagates(t *testing.T) {
	ctx := context.Background()
	db := new(dynamormmocks.MockDB)
	q := new(dynamormmocks.MockQuery)

	setupMockDB(db, q)
	q.On("First", mock.AnythingOfType("*models.InstanceTipsConfig")).Return(dynamormerrors.ErrItemNotFound).Once()
	q.On("Create").Return(assert.AnError).Once()

	repo := NewInstanceRepository(db, "test-table", zap.NewNop())
	_, err := repo.EnsureTipsConfig(ctx)
	require.Error(t, err)
}

func TestInstanceRepository_EnsureTipsConfig_CreateConflictThenGetErrorPropagates(t *testing.T) {
	ctx := context.Background()
	db := new(dynamormmocks.MockDB)
	q := new(dynamormmocks.MockQuery)

	setupMockDB(db, q)
	q.On("First", mock.AnythingOfType("*models.InstanceTipsConfig")).Return(dynamormerrors.ErrItemNotFound).Once()
	q.On("Create").Return(dynamormerrors.ErrConditionFailed).Once()
	q.On("First", mock.AnythingOfType("*models.InstanceTipsConfig")).Return(assert.AnError).Once()

	repo := NewInstanceRepository(db, "test-table", zap.NewNop())
	_, err := repo.EnsureTipsConfig(ctx)
	require.Error(t, err)
}

func TestInstanceRepository_GetTranslationConfig_DefaultWhenMissingAndCaches(t *testing.T) {
	ctx := context.Background()
	db := new(dynamormmocks.MockDB)
	q := new(dynamormmocks.MockQuery)

	setupMockDB(db, q)
	q.On("First", mock.AnythingOfType("*models.InstanceTranslationConfig")).Return(dynamormerrors.ErrItemNotFound).Once()

	repo := NewInstanceRepository(db, "test-table", zap.NewNop())
	cfg, err := repo.GetTranslationConfig(ctx)
	require.NoError(t, err)
	require.NotNil(t, cfg)
	assert.Equal(t, models.SKTranslationConfig, cfg.SK)
	require.NotNil(t, cfg.Managed)

	cfg2, err := repo.GetTranslationConfig(ctx)
	require.NoError(t, err)
	assert.Same(t, cfg, cfg2)
}

func TestInstanceRepository_SetTrustManagedDefaults_NoOpWhenPatchEmpty(t *testing.T) {
	ctx := context.Background()
	db := new(dynamormmocks.MockDB)
	q := new(dynamormmocks.MockQuery)

	setupMockDB(db, q)
	q.On("First", mock.AnythingOfType("*models.InstanceTrustConfig")).Run(func(args mock.Arguments) {
		out := args.Get(0).(*models.InstanceTrustConfig)
		out.PK = "INSTANCE#CONFIG"
		out.SK = models.SKTrustConfig
		out.Managed = nil
	}).Return(nil).Once()

	repo := NewInstanceRepository(db, "test-table", zap.NewNop())
	require.NoError(t, repo.SetTrustManagedDefaults(ctx, models.InstanceTrustConfigPatch{}))
}

func TestInstanceRepository_SetTrustOverride_NoOpWhenPatchEmpty(t *testing.T) {
	ctx := context.Background()
	db := new(dynamormmocks.MockDB)
	q := new(dynamormmocks.MockQuery)

	setupMockDB(db, q)
	q.On("First", mock.AnythingOfType("*models.InstanceTrustConfig")).Run(func(args mock.Arguments) {
		out := args.Get(0).(*models.InstanceTrustConfig)
		out.PK = "INSTANCE#CONFIG"
		out.SK = models.SKTrustConfig
		out.Override = nil
	}).Return(nil).Once()

	repo := NewInstanceRepository(db, "test-table", zap.NewNop())
	require.NoError(t, repo.SetTrustOverride(ctx, models.InstanceTrustConfigPatch{}))
}

func TestInstanceRepository_SetTrustManagedDefaults_UpdatesAndCaches(t *testing.T) {
	ctx := context.Background()
	db := new(dynamormmocks.MockDB)
	q := new(dynamormmocks.MockQuery)

	setupMockDB(db, q)
	q.On("First", mock.AnythingOfType("*models.InstanceTrustConfig")).Run(func(args mock.Arguments) {
		out := args.Get(0).(*models.InstanceTrustConfig)
		out.PK = "INSTANCE#CONFIG"
		out.SK = models.SKTrustConfig
		out.Managed = nil
	}).Return(nil).Once()
	q.On("Update", mock.Anything).Return(nil).Once()

	repo := NewInstanceRepository(db, "test-table", zap.NewNop())
	baseURL := " https://managed.example/ "
	attURL := " https://attest.example/ "
	arn := " arn:aws:secretsmanager:us-east-1:123:secret:abc "

	require.NoError(t, repo.SetTrustManagedDefaults(ctx, models.InstanceTrustConfigPatch{
		BaseURL:              &baseURL,
		AttestationsURL:      &attURL,
		InstanceKeySecretARN: &arn,
	}))

	cached, ok := repo.getCachedTrustConfig()
	require.True(t, ok)
	require.NotNil(t, cached.Managed)
	assert.Equal(t, "https://managed.example", cached.Managed.BaseURL)
	assert.Equal(t, "https://attest.example", cached.Managed.AttestationsURL)
	assert.Equal(t, "arn:aws:secretsmanager:us-east-1:123:secret:abc", cached.Managed.InstanceKeySecretARN)
}

func TestInstanceRepository_SetTrustOverride_UpdatesAndCaches(t *testing.T) {
	ctx := context.Background()
	db := new(dynamormmocks.MockDB)
	q := new(dynamormmocks.MockQuery)

	setupMockDB(db, q)
	q.On("First", mock.AnythingOfType("*models.InstanceTrustConfig")).Run(func(args mock.Arguments) {
		out := args.Get(0).(*models.InstanceTrustConfig)
		out.PK = "INSTANCE#CONFIG"
		out.SK = models.SKTrustConfig
		out.Override = nil
	}).Return(nil).Once()
	q.On("Update", mock.Anything).Return(nil).Once()

	repo := NewInstanceRepository(db, "test-table", zap.NewNop())
	baseURL := " https://override.example/ "
	attURL := " https://attest.example/ "
	arn := " arn:aws:secretsmanager:us-east-1:123:secret:abc "

	require.NoError(t, repo.SetTrustOverride(ctx, models.InstanceTrustConfigPatch{
		BaseURL:              &baseURL,
		AttestationsURL:      &attURL,
		InstanceKeySecretARN: &arn,
	}))

	cached, ok := repo.getCachedTrustConfig()
	require.True(t, ok)
	require.NotNil(t, cached.Override)
	require.NotNil(t, cached.Override.BaseURL)
	require.NotNil(t, cached.Override.AttestationsURL)
	require.NotNil(t, cached.Override.InstanceKeySecretARN)
	assert.Equal(t, "https://override.example", *cached.Override.BaseURL)
	assert.Equal(t, "https://attest.example", *cached.Override.AttestationsURL)
	assert.Equal(t, "arn:aws:secretsmanager:us-east-1:123:secret:abc", *cached.Override.InstanceKeySecretARN)
}

func TestInstanceRepository_SetTranslationManagedDefaults_UpdatesAndCaches(t *testing.T) {
	ctx := context.Background()
	db := new(dynamormmocks.MockDB)
	q := new(dynamormmocks.MockQuery)

	setupMockDB(db, q)
	q.On("First", mock.AnythingOfType("*models.InstanceTranslationConfig")).Run(func(args mock.Arguments) {
		out := args.Get(0).(*models.InstanceTranslationConfig)
		out.PK = "INSTANCE#CONFIG"
		out.SK = models.SKTranslationConfig
		out.Managed = nil
	}).Return(nil).Once()
	q.On("Update", mock.Anything).Return(nil).Once()

	repo := NewInstanceRepository(db, "test-table", zap.NewNop())
	enabled := true
	require.NoError(t, repo.SetTranslationManagedDefaults(ctx, models.InstanceTranslationConfigPatch{Enabled: &enabled}))

	cached, ok := repo.getCachedTranslationConfig()
	require.True(t, ok)
	require.NotNil(t, cached.Managed)
	assert.True(t, cached.Managed.Enabled)
}

func TestInstanceRepository_SetTranslationOverride_UpdatesAndCaches(t *testing.T) {
	ctx := context.Background()
	db := new(dynamormmocks.MockDB)
	q := new(dynamormmocks.MockQuery)

	setupMockDB(db, q)
	q.On("First", mock.AnythingOfType("*models.InstanceTranslationConfig")).Run(func(args mock.Arguments) {
		out := args.Get(0).(*models.InstanceTranslationConfig)
		out.PK = "INSTANCE#CONFIG"
		out.SK = models.SKTranslationConfig
		out.Override = nil
	}).Return(nil).Once()
	q.On("Update", mock.Anything).Return(nil).Once()

	repo := NewInstanceRepository(db, "test-table", zap.NewNop())
	enabled := true
	require.NoError(t, repo.SetTranslationOverride(ctx, models.InstanceTranslationConfigPatch{Enabled: &enabled}))

	cached, ok := repo.getCachedTranslationConfig()
	require.True(t, ok)
	require.NotNil(t, cached.Override)
	require.NotNil(t, cached.Override.Enabled)
	assert.True(t, *cached.Override.Enabled)
}

func TestInstanceRepository_ClearTranslationOverride_RemovesOverrideAttribute(t *testing.T) {
	ctx := context.Background()
	db := new(dynamormmocks.MockDB)
	q := new(dynamormmocks.MockQuery)
	builder := &testUpdateBuilder{}

	setupMockDB(db, q)
	q.On("First", mock.AnythingOfType("*models.InstanceTranslationConfig")).Run(func(args mock.Arguments) {
		out := args.Get(0).(*models.InstanceTranslationConfig)
		*out = *models.NewInstanceTranslationConfig()
		override := true
		out.Override = &models.InstanceTranslationConfigOverride{Enabled: &override}
	}).Return(nil).Once()
	q.On("UpdateBuilder").Return(builder).Once()

	repo := NewInstanceRepository(db, "test-table", zap.NewNop())
	require.NoError(t, repo.ClearTranslationOverride(ctx))

	cached, ok := repo.getCachedTranslationConfig()
	require.True(t, ok)
	assert.Nil(t, cached.Override)
}

func TestInstanceRepository_ClearTranslationOverride_ReturnsErrorWhenUpdateFails(t *testing.T) {
	ctx := context.Background()
	db := new(dynamormmocks.MockDB)
	q := new(dynamormmocks.MockQuery)
	builder := &testUpdateBuilderError{err: assert.AnError}

	setupMockDB(db, q)
	q.On("First", mock.AnythingOfType("*models.InstanceTranslationConfig")).Run(func(args mock.Arguments) {
		out := args.Get(0).(*models.InstanceTranslationConfig)
		out.PK = "INSTANCE#CONFIG"
		out.SK = models.SKTranslationConfig
	}).Return(nil).Once()
	q.On("UpdateBuilder").Return(builder).Once()

	repo := NewInstanceRepository(db, "test-table", zap.NewNop())
	require.Error(t, repo.ClearTranslationOverride(ctx))
}

func TestInstanceRepository_SetTranslationManagedDefaults_NoOpWhenPatchNil(t *testing.T) {
	repo := NewInstanceRepository(new(dynamormmocks.MockDB), "test-table", zap.NewNop())
	require.NoError(t, repo.SetTranslationManagedDefaults(context.Background(), models.InstanceTranslationConfigPatch{}))
}

func TestInstanceRepository_SetTranslationOverride_NoOpWhenPatchNil(t *testing.T) {
	repo := NewInstanceRepository(new(dynamormmocks.MockDB), "test-table", zap.NewNop())
	require.NoError(t, repo.SetTranslationOverride(context.Background(), models.InstanceTranslationConfigPatch{}))
}

func TestInstanceRepository_SetTranslationManagedDefaults_CreatesWhenUpdateNotFound(t *testing.T) {
	ctx := context.Background()
	db := new(dynamormmocks.MockDB)
	q := new(dynamormmocks.MockQuery)

	setupMockDB(db, q)
	q.On("First", mock.AnythingOfType("*models.InstanceTranslationConfig")).Run(func(args mock.Arguments) {
		out := args.Get(0).(*models.InstanceTranslationConfig)
		out.PK = "INSTANCE#CONFIG"
		out.SK = models.SKTranslationConfig
		out.Managed = &models.InstanceTranslationConfigManaged{Enabled: false}
	}).Return(nil).Once()
	q.On("Update", mock.Anything).Return(dynamormerrors.ErrItemNotFound).Once()
	q.On("Create").Return(nil).Once()

	repo := NewInstanceRepository(db, "test-table", zap.NewNop())
	enabled := true
	require.NoError(t, repo.SetTranslationManagedDefaults(ctx, models.InstanceTranslationConfigPatch{Enabled: &enabled}))

	cached, ok := repo.getCachedTranslationConfig()
	require.True(t, ok)
	require.NotNil(t, cached.Managed)
	assert.True(t, cached.Managed.Enabled)
}

func TestInstanceRepository_SetTranslationOverride_CreatesWhenUpdateNotFound(t *testing.T) {
	ctx := context.Background()
	db := new(dynamormmocks.MockDB)
	q := new(dynamormmocks.MockQuery)

	setupMockDB(db, q)
	q.On("First", mock.AnythingOfType("*models.InstanceTranslationConfig")).Run(func(args mock.Arguments) {
		out := args.Get(0).(*models.InstanceTranslationConfig)
		out.PK = "INSTANCE#CONFIG"
		out.SK = models.SKTranslationConfig
		out.Override = nil
	}).Return(nil).Once()
	q.On("Update", mock.Anything).Return(dynamormerrors.ErrItemNotFound).Once()
	q.On("Create").Return(nil).Once()

	repo := NewInstanceRepository(db, "test-table", zap.NewNop())
	enabled := true
	require.NoError(t, repo.SetTranslationOverride(ctx, models.InstanceTranslationConfigPatch{Enabled: &enabled}))

	cached, ok := repo.getCachedTranslationConfig()
	require.True(t, ok)
	require.NotNil(t, cached.Override)
	require.NotNil(t, cached.Override.Enabled)
	assert.True(t, *cached.Override.Enabled)
}

func TestInstanceRepository_GetTipsConfig_DefaultWhenMissingAndCaches(t *testing.T) {
	ctx := context.Background()
	db := new(dynamormmocks.MockDB)
	q := new(dynamormmocks.MockQuery)

	setupMockDB(db, q)
	q.On("First", mock.AnythingOfType("*models.InstanceTipsConfig")).Return(dynamormerrors.ErrItemNotFound).Once()

	repo := NewInstanceRepository(db, "test-table", zap.NewNop())
	cfg, err := repo.GetTipsConfig(ctx)
	require.NoError(t, err)
	require.NotNil(t, cfg)
	assert.Equal(t, models.SKTipsConfig, cfg.SK)
	require.NotNil(t, cfg.Managed)

	cfg2, err := repo.GetTipsConfig(ctx)
	require.NoError(t, err)
	assert.Same(t, cfg, cfg2)
}

func TestInstanceRepository_SetTipsManagedDefaults_UpdatesAndCaches(t *testing.T) {
	ctx := context.Background()
	db := new(dynamormmocks.MockDB)
	q := new(dynamormmocks.MockQuery)

	setupMockDB(db, q)
	q.On("First", mock.AnythingOfType("*models.InstanceTipsConfig")).Run(func(args mock.Arguments) {
		out := args.Get(0).(*models.InstanceTipsConfig)
		out.PK = "INSTANCE#CONFIG"
		out.SK = models.SKTipsConfig
		out.Managed = nil
	}).Return(nil).Once()
	q.On("Update", mock.Anything).Return(nil).Once()

	repo := NewInstanceRepository(db, "test-table", zap.NewNop())
	enabled := true
	chainID := 10
	contract := " 0xmanaged "
	require.NoError(t, repo.SetTipsManagedDefaults(ctx, models.InstanceTipsConfigPatch{
		Enabled:         &enabled,
		ChainID:         &chainID,
		ContractAddress: &contract,
	}))

	cached, ok := repo.getCachedTipsConfig()
	require.True(t, ok)
	require.NotNil(t, cached.Managed)
	assert.True(t, cached.Managed.Enabled)
	assert.Equal(t, 10, cached.Managed.ChainID)
	assert.Equal(t, "0xmanaged", cached.Managed.ContractAddress)
}

func TestInstanceRepository_SetTipsOverride_UpdatesAndCaches(t *testing.T) {
	ctx := context.Background()
	db := new(dynamormmocks.MockDB)
	q := new(dynamormmocks.MockQuery)

	setupMockDB(db, q)
	q.On("First", mock.AnythingOfType("*models.InstanceTipsConfig")).Run(func(args mock.Arguments) {
		out := args.Get(0).(*models.InstanceTipsConfig)
		out.PK = "INSTANCE#CONFIG"
		out.SK = models.SKTipsConfig
		out.Override = nil
	}).Return(nil).Once()
	q.On("Update", mock.Anything).Return(nil).Once()

	repo := NewInstanceRepository(db, "test-table", zap.NewNop())
	enabled := true
	chainID := 10
	contract := " 0xoverride "
	require.NoError(t, repo.SetTipsOverride(ctx, models.InstanceTipsConfigPatch{
		Enabled:         &enabled,
		ChainID:         &chainID,
		ContractAddress: &contract,
	}))

	cached, ok := repo.getCachedTipsConfig()
	require.True(t, ok)
	require.NotNil(t, cached.Override)
	require.NotNil(t, cached.Override.Enabled)
	require.NotNil(t, cached.Override.ChainID)
	require.NotNil(t, cached.Override.ContractAddress)
	assert.True(t, *cached.Override.Enabled)
	assert.Equal(t, 10, *cached.Override.ChainID)
	assert.Equal(t, "0xoverride", *cached.Override.ContractAddress)
}

func TestInstanceRepository_ClearTipsOverride_RemovesOverrideAttribute(t *testing.T) {
	ctx := context.Background()
	db := new(dynamormmocks.MockDB)
	q := new(dynamormmocks.MockQuery)
	builder := &testUpdateBuilder{}

	setupMockDB(db, q)
	q.On("First", mock.AnythingOfType("*models.InstanceTipsConfig")).Run(func(args mock.Arguments) {
		out := args.Get(0).(*models.InstanceTipsConfig)
		*out = *models.NewInstanceTipsConfig()
		overrideEnabled := true
		out.Override = &models.InstanceTipsConfigOverride{Enabled: &overrideEnabled}
	}).Return(nil).Once()
	q.On("UpdateBuilder").Return(builder).Once()

	repo := NewInstanceRepository(db, "test-table", zap.NewNop())
	require.NoError(t, repo.ClearTipsOverride(ctx))

	cached, ok := repo.getCachedTipsConfig()
	require.True(t, ok)
	assert.Nil(t, cached.Override)
}

func TestInstanceRepository_ClearTipsOverride_ReturnsErrorWhenUpdateFails(t *testing.T) {
	ctx := context.Background()
	db := new(dynamormmocks.MockDB)
	q := new(dynamormmocks.MockQuery)
	builder := &testUpdateBuilderError{err: assert.AnError}

	setupMockDB(db, q)
	q.On("First", mock.AnythingOfType("*models.InstanceTipsConfig")).Run(func(args mock.Arguments) {
		out := args.Get(0).(*models.InstanceTipsConfig)
		out.PK = "INSTANCE#CONFIG"
		out.SK = models.SKTipsConfig
	}).Return(nil).Once()
	q.On("UpdateBuilder").Return(builder).Once()

	repo := NewInstanceRepository(db, "test-table", zap.NewNop())
	require.Error(t, repo.ClearTipsOverride(ctx))
}

func TestInstanceRepository_GetAIInstanceConfig_DefaultWhenMissingAndCaches(t *testing.T) {
	ctx := context.Background()
	db := new(dynamormmocks.MockDB)
	q := new(dynamormmocks.MockQuery)

	setupMockDB(db, q)
	q.On("First", mock.AnythingOfType("*models.AIInstanceConfig")).Return(dynamormerrors.ErrItemNotFound).Once()

	repo := NewInstanceRepository(db, "test-table", zap.NewNop())
	cfg, err := repo.GetAIInstanceConfig(ctx)
	require.NoError(t, err)
	require.NotNil(t, cfg)
	assert.Equal(t, models.SKAIConfig, cfg.SK)
	require.NotNil(t, cfg.Managed)

	cfg2, err := repo.GetAIInstanceConfig(ctx)
	require.NoError(t, err)
	assert.Same(t, cfg, cfg2)
}

func TestInstanceRepository_EffectiveAIConfig_PrefersOverride(t *testing.T) {
	ctx := context.Background()
	db := new(dynamormmocks.MockDB)
	q := new(dynamormmocks.MockQuery)

	setupMockDB(db, q)
	q.On("First", mock.AnythingOfType("*models.AIInstanceConfig")).Run(func(args mock.Arguments) {
		out := args.Get(0).(*models.AIInstanceConfig)
		out.PK = "INSTANCE#CONFIG"
		out.SK = models.SKAIConfig
		out.Managed = &models.AIInstanceConfigManaged{
			AIEnabled:            false,
			ModerationEnabled:    false,
			NSFWDetectionEnabled: false,
			SpamDetectionEnabled: false,
			PIIDetectionEnabled:  false,
			AIContentDetection:   false,
		}
		aiEnabled := true
		moderationEnabled := true
		nsfwEnabled := true
		spamEnabled := true
		piiEnabled := true
		contentEnabled := true
		out.Override = &models.AIInstanceConfigOverride{
			AIEnabled:            &aiEnabled,
			ModerationEnabled:    &moderationEnabled,
			NSFWDetectionEnabled: &nsfwEnabled,
			SpamDetectionEnabled: &spamEnabled,
			PIIDetectionEnabled:  &piiEnabled,
			AIContentDetection:   &contentEnabled,
		}
	}).Return(nil).Once()

	repo := NewInstanceRepository(db, "test-table", zap.NewNop())
	effective, err := repo.EffectiveAIConfig(ctx)
	require.NoError(t, err)
	require.NotNil(t, effective)
	assert.True(t, effective.AIEnabled)
	assert.True(t, effective.ModerationEnabled)
	assert.True(t, effective.NSFWDetectionEnabled)
	assert.True(t, effective.SpamDetectionEnabled)
	assert.True(t, effective.PIIDetectionEnabled)
	assert.True(t, effective.AIContentDetection)
}

func TestInstanceRepository_SetAIManagedDefaults_UpdatesAndCaches(t *testing.T) {
	ctx := context.Background()
	db := new(dynamormmocks.MockDB)
	q := new(dynamormmocks.MockQuery)

	setupMockDB(db, q)
	q.On("First", mock.AnythingOfType("*models.AIInstanceConfig")).Run(func(args mock.Arguments) {
		out := args.Get(0).(*models.AIInstanceConfig)
		out.PK = "INSTANCE#CONFIG"
		out.SK = models.SKAIConfig
		out.Managed = nil
	}).Return(nil).Once()
	q.On("Update", mock.Anything).Return(nil).Once()

	repo := NewInstanceRepository(db, "test-table", zap.NewNop())
	aiEnabled := true
	moderationEnabled := true
	nsfwEnabled := true
	spamEnabled := true
	piiEnabled := true
	contentEnabled := true

	require.NoError(t, repo.SetAIManagedDefaults(ctx, models.AIInstanceConfigPatch{
		AIEnabled:            &aiEnabled,
		ModerationEnabled:    &moderationEnabled,
		NSFWDetectionEnabled: &nsfwEnabled,
		SpamDetectionEnabled: &spamEnabled,
		PIIDetectionEnabled:  &piiEnabled,
		AIContentDetection:   &contentEnabled,
	}))

	cached, ok := repo.getCachedAIConfig()
	require.True(t, ok)
	require.NotNil(t, cached.Managed)
	assert.True(t, cached.Managed.AIEnabled)
	assert.True(t, cached.Managed.ModerationEnabled)
	assert.True(t, cached.Managed.NSFWDetectionEnabled)
	assert.True(t, cached.Managed.SpamDetectionEnabled)
	assert.True(t, cached.Managed.PIIDetectionEnabled)
	assert.True(t, cached.Managed.AIContentDetection)
}

func TestInstanceRepository_SetAIOverride_UpdatesAndCaches(t *testing.T) {
	ctx := context.Background()
	db := new(dynamormmocks.MockDB)
	q := new(dynamormmocks.MockQuery)

	setupMockDB(db, q)
	q.On("First", mock.AnythingOfType("*models.AIInstanceConfig")).Run(func(args mock.Arguments) {
		out := args.Get(0).(*models.AIInstanceConfig)
		out.PK = "INSTANCE#CONFIG"
		out.SK = models.SKAIConfig
		out.Override = nil
	}).Return(nil).Once()
	q.On("Update", mock.Anything).Return(nil).Once()

	repo := NewInstanceRepository(db, "test-table", zap.NewNop())
	aiEnabled := true
	moderationEnabled := true
	nsfwEnabled := true
	spamEnabled := true
	piiEnabled := true
	contentEnabled := true

	require.NoError(t, repo.SetAIOverride(ctx, models.AIInstanceConfigPatch{
		AIEnabled:            &aiEnabled,
		ModerationEnabled:    &moderationEnabled,
		NSFWDetectionEnabled: &nsfwEnabled,
		SpamDetectionEnabled: &spamEnabled,
		PIIDetectionEnabled:  &piiEnabled,
		AIContentDetection:   &contentEnabled,
	}))

	cached, ok := repo.getCachedAIConfig()
	require.True(t, ok)
	require.NotNil(t, cached.Override)
	require.NotNil(t, cached.Override.AIEnabled)
	require.NotNil(t, cached.Override.ModerationEnabled)
	require.NotNil(t, cached.Override.NSFWDetectionEnabled)
	require.NotNil(t, cached.Override.SpamDetectionEnabled)
	require.NotNil(t, cached.Override.PIIDetectionEnabled)
	require.NotNil(t, cached.Override.AIContentDetection)
	assert.True(t, *cached.Override.AIEnabled)
	assert.True(t, *cached.Override.ModerationEnabled)
	assert.True(t, *cached.Override.NSFWDetectionEnabled)
	assert.True(t, *cached.Override.SpamDetectionEnabled)
	assert.True(t, *cached.Override.PIIDetectionEnabled)
	assert.True(t, *cached.Override.AIContentDetection)
}

func TestInstanceRepository_ClearAIOverride_RemovesOverrideAttribute(t *testing.T) {
	ctx := context.Background()
	db := new(dynamormmocks.MockDB)
	q := new(dynamormmocks.MockQuery)
	builder := &testUpdateBuilder{}

	setupMockDB(db, q)
	q.On("First", mock.AnythingOfType("*models.AIInstanceConfig")).Run(func(args mock.Arguments) {
		out := args.Get(0).(*models.AIInstanceConfig)
		*out = *models.NewAIInstanceConfig()
		overrideEnabled := true
		out.Override = &models.AIInstanceConfigOverride{AIEnabled: &overrideEnabled}
	}).Return(nil).Once()
	q.On("UpdateBuilder").Return(builder).Once()

	repo := NewInstanceRepository(db, "test-table", zap.NewNop())
	require.NoError(t, repo.ClearAIOverride(ctx))

	cached, ok := repo.getCachedAIConfig()
	require.True(t, ok)
	assert.Nil(t, cached.Override)
}

func TestInstanceRepository_ClearAIOverride_ReturnsErrorWhenUpdateFails(t *testing.T) {
	ctx := context.Background()
	db := new(dynamormmocks.MockDB)
	q := new(dynamormmocks.MockQuery)
	builder := &testUpdateBuilderError{err: assert.AnError}

	setupMockDB(db, q)
	q.On("First", mock.AnythingOfType("*models.AIInstanceConfig")).Run(func(args mock.Arguments) {
		out := args.Get(0).(*models.AIInstanceConfig)
		out.PK = "INSTANCE#CONFIG"
		out.SK = models.SKAIConfig
	}).Return(nil).Once()
	q.On("UpdateBuilder").Return(builder).Once()

	repo := NewInstanceRepository(db, "test-table", zap.NewNop())
	require.Error(t, repo.ClearAIOverride(ctx))
}

func TestInstanceRepository_InvalidateFeatureConfigCaches(t *testing.T) {
	repo := NewInstanceRepository(new(dynamormmocks.MockDB), "test-table", zap.NewNop())
	repo.setCachedTrustConfig(models.NewInstanceTrustConfig())
	repo.setCachedTranslationConfig(models.NewInstanceTranslationConfig())
	repo.setCachedTipsConfig(models.NewInstanceTipsConfig())
	repo.setCachedAIConfig(models.NewAIInstanceConfig())

	repo.invalidateTrustConfigCache()
	repo.invalidateTranslationConfigCache()
	repo.invalidateTipsConfigCache()
	repo.invalidateAIConfigCache()

	_, ok := repo.getCachedTrustConfig()
	assert.False(t, ok)
	_, ok = repo.getCachedTranslationConfig()
	assert.False(t, ok)
	_, ok = repo.getCachedTipsConfig()
	assert.False(t, ok)
	_, ok = repo.getCachedAIConfig()
	assert.False(t, ok)

	repo.warnInvalidEffectiveConfig("test warning", zap.String("key", "value"))
}

func TestInstanceRepository_warnInvalidEffectiveConfig_ReturnsWhenNilReceiver(t *testing.T) {
	require.NotPanics(t, func() {
		var repo *InstanceRepository
		repo.warnInvalidEffectiveConfig("ignored")
	})
}

package repositories

import (
	"context"
	"testing"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	dynamormerrors "github.com/theory-cloud/tabletheory/pkg/errors"
	dynamormmocks "github.com/theory-cloud/tabletheory/pkg/mocks"
	"github.com/theory-cloud/tabletheory/pkg/core"
	"go.uber.org/zap"
)

type testUpdateBuilder struct{}

func (b *testUpdateBuilder) Set(string, any) core.UpdateBuilder                          { return b }
func (b *testUpdateBuilder) SetIfNotExists(string, any, any) core.UpdateBuilder         { return b }
func (b *testUpdateBuilder) Add(string, any) core.UpdateBuilder                         { return b }
func (b *testUpdateBuilder) AddAll(string, any) core.UpdateBuilder                      { return b }
func (b *testUpdateBuilder) Increment(string) core.UpdateBuilder                        { return b }
func (b *testUpdateBuilder) Decrement(string) core.UpdateBuilder                        { return b }
func (b *testUpdateBuilder) Remove(string) core.UpdateBuilder                           { return b }
func (b *testUpdateBuilder) Delete(string, any) core.UpdateBuilder                      { return b }
func (b *testUpdateBuilder) AppendToList(string, any) core.UpdateBuilder                { return b }
func (b *testUpdateBuilder) PrependToList(string, any) core.UpdateBuilder               { return b }
func (b *testUpdateBuilder) RemoveFromListAt(string, int) core.UpdateBuilder            { return b }
func (b *testUpdateBuilder) SetListElement(string, int, any) core.UpdateBuilder         { return b }
func (b *testUpdateBuilder) Condition(string, string, any) core.UpdateBuilder           { return b }
func (b *testUpdateBuilder) OrCondition(string, string, any) core.UpdateBuilder         { return b }
func (b *testUpdateBuilder) ConditionExists(string) core.UpdateBuilder                  { return b }
func (b *testUpdateBuilder) ConditionNotExists(string) core.UpdateBuilder               { return b }
func (b *testUpdateBuilder) ConditionVersion(int64) core.UpdateBuilder                  { return b }
func (b *testUpdateBuilder) ReturnValues(string) core.UpdateBuilder                     { return b }
func (b *testUpdateBuilder) Execute() error                                             { return nil }
func (b *testUpdateBuilder) ExecuteWithResult(any) error                                { return nil }

func TestInstanceRepository_GetTrustConfig_DefaultWhenMissingAndCaches(t *testing.T) {
	ctx := context.Background()
	db := new(dynamormmocks.MockDB)
	q := new(dynamormmocks.MockQuery)

	db.On("WithContext", mock.Anything).Return(db)
	db.On("Model", mock.Anything).Return(q)
	q.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(q)
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

	db.On("WithContext", mock.Anything).Return(db).Maybe()
	db.On("Model", mock.Anything).Return(q).Maybe()
	q.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(q).Maybe()

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

	db.On("WithContext", mock.Anything).Return(db)
	db.On("Model", mock.Anything).Return(q)
	q.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(q)
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

	db.On("WithContext", mock.Anything).Return(db)
	db.On("Model", mock.Anything).Return(q)
	q.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(q)
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

	db.On("WithContext", mock.Anything).Return(db)
	db.On("Model", mock.Anything).Return(q)
	q.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(q)
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

	db.On("WithContext", mock.Anything).Return(db)
	db.On("Model", mock.Anything).Return(q)
	q.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(q)
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

func TestInstanceRepository_ClearTrustOverride_RemovesOverrideAttribute(t *testing.T) {
	ctx := context.Background()
	db := new(dynamormmocks.MockDB)
	q := new(dynamormmocks.MockQuery)
	builder := &testUpdateBuilder{}

	db.On("WithContext", mock.Anything).Return(db).Maybe()
	db.On("Model", mock.Anything).Return(q).Maybe()
	q.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(q).Maybe()
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


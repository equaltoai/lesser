package repositories

import (
	"context"
	"strings"
	"time"

	appErrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"go.uber.org/zap"
)

func (r *InstanceRepository) getCachedTrustConfig() (*models.InstanceTrustConfig, bool) {
	r.trustCache.mu.RLock()
	cfg := r.trustCache.cfg
	expiresAt := r.trustCache.expiresAt
	r.trustCache.mu.RUnlock()

	if cfg == nil || time.Now().After(expiresAt) {
		return nil, false
	}
	return cfg, true
}

func (r *InstanceRepository) setCachedTrustConfig(cfg *models.InstanceTrustConfig) {
	r.trustCache.mu.Lock()
	r.trustCache.cfg = cfg
	r.trustCache.expiresAt = time.Now().Add(instanceFeatureConfigCacheTTL)
	r.trustCache.mu.Unlock()
}

func (r *InstanceRepository) invalidateTrustConfigCache() {
	r.trustCache.mu.Lock()
	r.trustCache.cfg = nil
	r.trustCache.expiresAt = time.Time{}
	r.trustCache.mu.Unlock()
}

func (r *InstanceRepository) getCachedTranslationConfig() (*models.InstanceTranslationConfig, bool) {
	r.translationCache.mu.RLock()
	cfg := r.translationCache.cfg
	expiresAt := r.translationCache.expiresAt
	r.translationCache.mu.RUnlock()

	if cfg == nil || time.Now().After(expiresAt) {
		return nil, false
	}
	return cfg, true
}

func (r *InstanceRepository) setCachedTranslationConfig(cfg *models.InstanceTranslationConfig) {
	r.translationCache.mu.Lock()
	r.translationCache.cfg = cfg
	r.translationCache.expiresAt = time.Now().Add(instanceFeatureConfigCacheTTL)
	r.translationCache.mu.Unlock()
}

func (r *InstanceRepository) invalidateTranslationConfigCache() {
	r.translationCache.mu.Lock()
	r.translationCache.cfg = nil
	r.translationCache.expiresAt = time.Time{}
	r.translationCache.mu.Unlock()
}

func (r *InstanceRepository) getCachedTipsConfig() (*models.InstanceTipsConfig, bool) {
	r.tipsCache.mu.RLock()
	cfg := r.tipsCache.cfg
	expiresAt := r.tipsCache.expiresAt
	r.tipsCache.mu.RUnlock()

	if cfg == nil || time.Now().After(expiresAt) {
		return nil, false
	}
	return cfg, true
}

func (r *InstanceRepository) setCachedTipsConfig(cfg *models.InstanceTipsConfig) {
	r.tipsCache.mu.Lock()
	r.tipsCache.cfg = cfg
	r.tipsCache.expiresAt = time.Now().Add(instanceFeatureConfigCacheTTL)
	r.tipsCache.mu.Unlock()
}

func (r *InstanceRepository) invalidateTipsConfigCache() {
	r.tipsCache.mu.Lock()
	r.tipsCache.cfg = nil
	r.tipsCache.expiresAt = time.Time{}
	r.tipsCache.mu.Unlock()
}

func (r *InstanceRepository) getCachedAIConfig() (*models.AIInstanceConfig, bool) {
	r.aiConfigCache.mu.RLock()
	cfg := r.aiConfigCache.cfg
	expiresAt := r.aiConfigCache.expiresAt
	r.aiConfigCache.mu.RUnlock()

	if cfg == nil || time.Now().After(expiresAt) {
		return nil, false
	}
	return cfg, true
}

func (r *InstanceRepository) setCachedAIConfig(cfg *models.AIInstanceConfig) {
	r.aiConfigCache.mu.Lock()
	r.aiConfigCache.cfg = cfg
	r.aiConfigCache.expiresAt = time.Now().Add(instanceFeatureConfigCacheTTL)
	r.aiConfigCache.mu.Unlock()
}

func (r *InstanceRepository) invalidateAIConfigCache() {
	r.aiConfigCache.mu.Lock()
	r.aiConfigCache.cfg = nil
	r.aiConfigCache.expiresAt = time.Time{}
	r.aiConfigCache.mu.Unlock()
}

// TrustConfigExists reports whether the trust config record exists in storage.
// This is used for migration-safe fallback behavior (legacy env/receipt wiring only applies when no record exists).
func (r *InstanceRepository) TrustConfigExists(ctx context.Context) (bool, error) {
	cfg := &models.InstanceTrustConfig{}
	err := r.trustRepo.Get(ctx, storage.InstanceConfigKey, models.SKTrustConfig, cfg)
	if err != nil {
		if appErrors.HasCode(err, appErrors.CodeNotFound) {
			return false, nil
		}
		return false, err
	}
	if strings.TrimSpace(cfg.PK) == "" || strings.TrimSpace(cfg.SK) == "" {
		return false, nil
	}
	r.setCachedTrustConfig(cfg)
	return true, nil
}

// TranslationConfigExists reports whether the translation config record exists in storage.
func (r *InstanceRepository) TranslationConfigExists(ctx context.Context) (bool, error) {
	cfg := &models.InstanceTranslationConfig{}
	err := r.translationRepo.Get(ctx, storage.InstanceConfigKey, models.SKTranslationConfig, cfg)
	if err != nil {
		if appErrors.HasCode(err, appErrors.CodeNotFound) {
			return false, nil
		}
		return false, err
	}
	if strings.TrimSpace(cfg.PK) == "" || strings.TrimSpace(cfg.SK) == "" {
		return false, nil
	}
	r.setCachedTranslationConfig(cfg)
	return true, nil
}

// TipsConfigExists reports whether the tips config record exists in storage.
func (r *InstanceRepository) TipsConfigExists(ctx context.Context) (bool, error) {
	cfg := &models.InstanceTipsConfig{}
	err := r.tipsRepo.Get(ctx, storage.InstanceConfigKey, models.SKTipsConfig, cfg)
	if err != nil {
		if appErrors.HasCode(err, appErrors.CodeNotFound) {
			return false, nil
		}
		return false, err
	}
	if strings.TrimSpace(cfg.PK) == "" || strings.TrimSpace(cfg.SK) == "" {
		return false, nil
	}
	r.setCachedTipsConfig(cfg)
	return true, nil
}

// AIConfigExists reports whether the AI config record exists in storage.
func (r *InstanceRepository) AIConfigExists(ctx context.Context) (bool, error) {
	cfg := &models.AIInstanceConfig{}
	err := r.aiConfigRepo.Get(ctx, storage.InstanceConfigKey, models.SKAIConfig, cfg)
	if err != nil {
		if appErrors.HasCode(err, appErrors.CodeNotFound) {
			return false, nil
		}
		return false, err
	}
	if strings.TrimSpace(cfg.PK) == "" || strings.TrimSpace(cfg.SK) == "" {
		return false, nil
	}
	r.setCachedAIConfig(cfg)
	return true, nil
}

// GetTrustConfig returns the current instance trust configuration.
// If no record exists yet, it returns built-in defaults without persisting.
func (r *InstanceRepository) GetTrustConfig(ctx context.Context) (*models.InstanceTrustConfig, error) {
	if cached, ok := r.getCachedTrustConfig(); ok {
		return cached, nil
	}

	cfg := &models.InstanceTrustConfig{}
	err := r.trustRepo.Get(ctx, storage.InstanceConfigKey, models.SKTrustConfig, cfg)
	if err != nil {
		if appErrors.HasCode(err, appErrors.CodeNotFound) {
			defaultCfg := models.NewInstanceTrustConfig()
			r.setCachedTrustConfig(defaultCfg)
			return defaultCfg, nil
		}
		return nil, err
	}

	r.setCachedTrustConfig(cfg)
	return cfg, nil
}

// EnsureTrustConfig ensures the instance trust config record exists and returns it.
func (r *InstanceRepository) EnsureTrustConfig(ctx context.Context) (*models.InstanceTrustConfig, error) {
	cfg := &models.InstanceTrustConfig{}
	err := r.trustRepo.Get(ctx, storage.InstanceConfigKey, models.SKTrustConfig, cfg)
	if err == nil {
		r.setCachedTrustConfig(cfg)
		return cfg, nil
	}

	if !appErrors.HasCode(err, appErrors.CodeNotFound) {
		return nil, err
	}

	cfg = models.NewInstanceTrustConfig()
	if createErr := r.trustRepo.Create(ctx, cfg); createErr != nil {
		if appErrors.HasCode(createErr, appErrors.CodeAlreadyExists) || appErrors.HasCode(createErr, appErrors.CodeConflict) {
			cfg = &models.InstanceTrustConfig{}
			if err := r.trustRepo.Get(ctx, storage.InstanceConfigKey, models.SKTrustConfig, cfg); err != nil {
				return nil, err
			}
			r.setCachedTrustConfig(cfg)
			return cfg, nil
		}
		return nil, createErr
	}

	r.setCachedTrustConfig(cfg)
	return cfg, nil
}

// SetTrustManagedDefaults merges managed trust defaults into instance config.
// Nil patch fields are treated as "no change" (merge-safe).
func (r *InstanceRepository) SetTrustManagedDefaults(ctx context.Context, patch models.InstanceTrustConfigPatch) error {
	cfg, err := r.EnsureTrustConfig(ctx)
	if err != nil {
		return err
	}
	if cfg.Managed == nil {
		cfg.Managed = &models.InstanceTrustConfigManaged{}
	}

	changed := false
	if patch.BaseURL != nil {
		cfg.Managed.BaseURL = strings.TrimRight(strings.TrimSpace(*patch.BaseURL), "/")
		changed = true
	}
	if patch.AttestationsURL != nil {
		cfg.Managed.AttestationsURL = strings.TrimRight(strings.TrimSpace(*patch.AttestationsURL), "/")
		changed = true
	}
	if patch.InstanceKeySecretARN != nil {
		cfg.Managed.InstanceKeySecretARN = strings.TrimSpace(*patch.InstanceKeySecretARN)
		changed = true
	}
	if !changed {
		return nil
	}

	cfg.UpdatedAt = time.Now()
	if err := r.trustRepo.Update(ctx, cfg); err != nil {
		if appErrors.HasCode(err, appErrors.CodeNotFound) {
			if err := r.trustRepo.Create(ctx, cfg); err != nil {
				return err
			}
		} else {
			return err
		}
	}

	r.setCachedTrustConfig(cfg)
	return nil
}

// SetTrustOverride merges operator trust overrides into instance config.
// Nil patch fields are treated as "no change".
func (r *InstanceRepository) SetTrustOverride(ctx context.Context, patch models.InstanceTrustConfigPatch) error {
	cfg, err := r.EnsureTrustConfig(ctx)
	if err != nil {
		return err
	}
	if cfg.Override == nil {
		cfg.Override = &models.InstanceTrustConfigOverride{}
	}

	changed := false
	if patch.BaseURL != nil {
		v := strings.TrimRight(strings.TrimSpace(*patch.BaseURL), "/")
		cfg.Override.BaseURL = &v
		changed = true
	}
	if patch.AttestationsURL != nil {
		v := strings.TrimRight(strings.TrimSpace(*patch.AttestationsURL), "/")
		cfg.Override.AttestationsURL = &v
		changed = true
	}
	if patch.InstanceKeySecretARN != nil {
		v := strings.TrimSpace(*patch.InstanceKeySecretARN)
		cfg.Override.InstanceKeySecretARN = &v
		changed = true
	}
	if !changed {
		return nil
	}

	cfg.UpdatedAt = time.Now()
	if err := r.trustRepo.Update(ctx, cfg); err != nil {
		if appErrors.HasCode(err, appErrors.CodeNotFound) {
			if err := r.trustRepo.Create(ctx, cfg); err != nil {
				return err
			}
		} else {
			return err
		}
	}

	r.setCachedTrustConfig(cfg)
	return nil
}

// ClearTrustOverride removes the operator override layer from the instance trust config.
func (r *InstanceRepository) ClearTrustOverride(ctx context.Context) error {
	cfg, err := r.EnsureTrustConfig(ctx)
	if err != nil {
		return err
	}

	now := time.Now()
	builder := r.trustRepo.db.WithContext(ctx).
		Model(&models.InstanceTrustConfig{}).
		Where("PK", "=", storage.InstanceConfigKey).
		Where("SK", "=", models.SKTrustConfig).
		UpdateBuilder()
	builder.Remove("override")
	builder.Set("UpdatedAt", now.UTC())

	if err := builder.Execute(); err != nil {
		return ErrorHandler.HandleUpdateError(err, "trust config", "override")
	}

	cfg.Override = nil
	cfg.UpdatedAt = now
	r.setCachedTrustConfig(cfg)
	return nil
}

// EffectiveTrustConfig resolves effective trust config values for runtime usage.
func (r *InstanceRepository) EffectiveTrustConfig(ctx context.Context) (*models.EffectiveTrustConfig, error) {
	cfg, err := r.GetTrustConfig(ctx)
	if err != nil {
		return nil, err
	}

	managed := cfg.Managed
	if managed == nil {
		managed = &models.InstanceTrustConfigManaged{}
	}

	trustBase := strings.TrimRight(strings.TrimSpace(managed.BaseURL), "/")
	attestBase := strings.TrimRight(strings.TrimSpace(managed.AttestationsURL), "/")
	keyARN := strings.TrimSpace(managed.InstanceKeySecretARN)

	if cfg.Override != nil {
		if cfg.Override.BaseURL != nil {
			trustBase = strings.TrimRight(strings.TrimSpace(*cfg.Override.BaseURL), "/")
		}
		if cfg.Override.AttestationsURL != nil {
			attestBase = strings.TrimRight(strings.TrimSpace(*cfg.Override.AttestationsURL), "/")
		}
		if cfg.Override.InstanceKeySecretARN != nil {
			keyARN = strings.TrimSpace(*cfg.Override.InstanceKeySecretARN)
		}
	}

	if attestBase == "" {
		attestBase = trustBase
	}

	out := &models.EffectiveTrustConfig{
		TrustBaseURL:              trustBase,
		AttestationsBaseURL:       attestBase,
		InstanceKeySecretARN:      keyARN,
		PublicAttestationsEnabled: attestBase != "",
		TrustProxyEnabled:         trustBase != "" && keyARN != "",
	}
	return out, nil
}

// GetTranslationConfig returns the current instance translation configuration.
// If no record exists yet, it returns built-in defaults without persisting.
func (r *InstanceRepository) GetTranslationConfig(ctx context.Context) (*models.InstanceTranslationConfig, error) {
	if cached, ok := r.getCachedTranslationConfig(); ok {
		return cached, nil
	}

	cfg := &models.InstanceTranslationConfig{}
	err := r.translationRepo.Get(ctx, storage.InstanceConfigKey, models.SKTranslationConfig, cfg)
	if err != nil {
		if appErrors.HasCode(err, appErrors.CodeNotFound) {
			defaultCfg := models.NewInstanceTranslationConfig()
			r.setCachedTranslationConfig(defaultCfg)
			return defaultCfg, nil
		}
		return nil, err
	}

	r.setCachedTranslationConfig(cfg)
	return cfg, nil
}

// EnsureTranslationConfig ensures the instance translation config record exists and returns it.
func (r *InstanceRepository) EnsureTranslationConfig(ctx context.Context) (*models.InstanceTranslationConfig, error) {
	cfg := &models.InstanceTranslationConfig{}
	err := r.translationRepo.Get(ctx, storage.InstanceConfigKey, models.SKTranslationConfig, cfg)
	if err == nil {
		r.setCachedTranslationConfig(cfg)
		return cfg, nil
	}

	if !appErrors.HasCode(err, appErrors.CodeNotFound) {
		return nil, err
	}

	cfg = models.NewInstanceTranslationConfig()
	if createErr := r.translationRepo.Create(ctx, cfg); createErr != nil {
		if appErrors.HasCode(createErr, appErrors.CodeAlreadyExists) || appErrors.HasCode(createErr, appErrors.CodeConflict) {
			cfg = &models.InstanceTranslationConfig{}
			if err := r.translationRepo.Get(ctx, storage.InstanceConfigKey, models.SKTranslationConfig, cfg); err != nil {
				return nil, err
			}
			r.setCachedTranslationConfig(cfg)
			return cfg, nil
		}
		return nil, createErr
	}

	r.setCachedTranslationConfig(cfg)
	return cfg, nil
}

// SetTranslationManagedDefaults merges managed translation defaults into instance config.
func (r *InstanceRepository) SetTranslationManagedDefaults(ctx context.Context, patch models.InstanceTranslationConfigPatch) error {
	if patch.Enabled == nil {
		return nil
	}

	cfg, err := r.EnsureTranslationConfig(ctx)
	if err != nil {
		return err
	}
	if cfg.Managed == nil {
		cfg.Managed = &models.InstanceTranslationConfigManaged{}
	}

	cfg.Managed.Enabled = *patch.Enabled
	cfg.UpdatedAt = time.Now()
	if err := r.translationRepo.Update(ctx, cfg); err != nil {
		if appErrors.HasCode(err, appErrors.CodeNotFound) {
			if err := r.translationRepo.Create(ctx, cfg); err != nil {
				return err
			}
		} else {
			return err
		}
	}

	r.setCachedTranslationConfig(cfg)
	return nil
}

// SetTranslationOverride merges operator translation overrides into instance config.
func (r *InstanceRepository) SetTranslationOverride(ctx context.Context, patch models.InstanceTranslationConfigPatch) error {
	if patch.Enabled == nil {
		return nil
	}

	cfg, err := r.EnsureTranslationConfig(ctx)
	if err != nil {
		return err
	}
	if cfg.Override == nil {
		cfg.Override = &models.InstanceTranslationConfigOverride{}
	}

	cfg.Override.Enabled = patch.Enabled
	cfg.UpdatedAt = time.Now()
	if err := r.translationRepo.Update(ctx, cfg); err != nil {
		if appErrors.HasCode(err, appErrors.CodeNotFound) {
			if err := r.translationRepo.Create(ctx, cfg); err != nil {
				return err
			}
		} else {
			return err
		}
	}

	r.setCachedTranslationConfig(cfg)
	return nil
}

// ClearTranslationOverride removes the operator override layer from the instance translation config.
func (r *InstanceRepository) ClearTranslationOverride(ctx context.Context) error {
	cfg, err := r.EnsureTranslationConfig(ctx)
	if err != nil {
		return err
	}

	now := time.Now()
	builder := r.translationRepo.db.WithContext(ctx).
		Model(&models.InstanceTranslationConfig{}).
		Where("PK", "=", storage.InstanceConfigKey).
		Where("SK", "=", models.SKTranslationConfig).
		UpdateBuilder()
	builder.Remove("override")
	builder.Set("UpdatedAt", now.UTC())

	if err := builder.Execute(); err != nil {
		return ErrorHandler.HandleUpdateError(err, "translation config", "override")
	}

	cfg.Override = nil
	cfg.UpdatedAt = now
	r.setCachedTranslationConfig(cfg)
	return nil
}

// EffectiveTranslationEnabled resolves whether translation is enabled for runtime usage.
func (r *InstanceRepository) EffectiveTranslationEnabled(ctx context.Context) (bool, error) {
	cfg, err := r.GetTranslationConfig(ctx)
	if err != nil {
		return false, err
	}

	managed := cfg.Managed
	if managed == nil {
		managed = &models.InstanceTranslationConfigManaged{Enabled: false}
	}
	enabled := managed.Enabled
	if cfg.Override != nil && cfg.Override.Enabled != nil {
		enabled = *cfg.Override.Enabled
	}
	return enabled, nil
}

// GetTipsConfig returns the current instance tips configuration.
// If no record exists yet, it returns built-in defaults without persisting.
func (r *InstanceRepository) GetTipsConfig(ctx context.Context) (*models.InstanceTipsConfig, error) {
	if cached, ok := r.getCachedTipsConfig(); ok {
		return cached, nil
	}

	cfg := &models.InstanceTipsConfig{}
	err := r.tipsRepo.Get(ctx, storage.InstanceConfigKey, models.SKTipsConfig, cfg)
	if err != nil {
		if appErrors.HasCode(err, appErrors.CodeNotFound) {
			defaultCfg := models.NewInstanceTipsConfig()
			r.setCachedTipsConfig(defaultCfg)
			return defaultCfg, nil
		}
		return nil, err
	}

	r.setCachedTipsConfig(cfg)
	return cfg, nil
}

// EnsureTipsConfig ensures the instance tips config record exists and returns it.
func (r *InstanceRepository) EnsureTipsConfig(ctx context.Context) (*models.InstanceTipsConfig, error) {
	cfg := &models.InstanceTipsConfig{}
	err := r.tipsRepo.Get(ctx, storage.InstanceConfigKey, models.SKTipsConfig, cfg)
	if err == nil {
		r.setCachedTipsConfig(cfg)
		return cfg, nil
	}

	if !appErrors.HasCode(err, appErrors.CodeNotFound) {
		return nil, err
	}

	cfg = models.NewInstanceTipsConfig()
	if createErr := r.tipsRepo.Create(ctx, cfg); createErr != nil {
		if appErrors.HasCode(createErr, appErrors.CodeAlreadyExists) || appErrors.HasCode(createErr, appErrors.CodeConflict) {
			cfg = &models.InstanceTipsConfig{}
			if err := r.tipsRepo.Get(ctx, storage.InstanceConfigKey, models.SKTipsConfig, cfg); err != nil {
				return nil, err
			}
			r.setCachedTipsConfig(cfg)
			return cfg, nil
		}
		return nil, createErr
	}

	r.setCachedTipsConfig(cfg)
	return cfg, nil
}

// SetTipsManagedDefaults merges managed tips defaults into instance config.
func (r *InstanceRepository) SetTipsManagedDefaults(ctx context.Context, patch models.InstanceTipsConfigPatch) error {
	cfg, err := r.EnsureTipsConfig(ctx)
	if err != nil {
		return err
	}
	if cfg.Managed == nil {
		cfg.Managed = &models.InstanceTipsConfigManaged{}
	}

	changed := false
	if patch.Enabled != nil {
		cfg.Managed.Enabled = *patch.Enabled
		changed = true
	}
	if patch.ChainID != nil {
		cfg.Managed.ChainID = *patch.ChainID
		changed = true
	}
	if patch.ContractAddress != nil {
		cfg.Managed.ContractAddress = strings.TrimSpace(*patch.ContractAddress)
		changed = true
	}
	if !changed {
		return nil
	}

	cfg.UpdatedAt = time.Now()
	if err := r.tipsRepo.Update(ctx, cfg); err != nil {
		if appErrors.HasCode(err, appErrors.CodeNotFound) {
			if err := r.tipsRepo.Create(ctx, cfg); err != nil {
				return err
			}
		} else {
			return err
		}
	}

	r.setCachedTipsConfig(cfg)
	return nil
}

// SetTipsOverride merges operator tips overrides into instance config.
func (r *InstanceRepository) SetTipsOverride(ctx context.Context, patch models.InstanceTipsConfigPatch) error {
	cfg, err := r.EnsureTipsConfig(ctx)
	if err != nil {
		return err
	}
	if cfg.Override == nil {
		cfg.Override = &models.InstanceTipsConfigOverride{}
	}

	changed := false
	if patch.Enabled != nil {
		cfg.Override.Enabled = patch.Enabled
		changed = true
	}
	if patch.ChainID != nil {
		cfg.Override.ChainID = patch.ChainID
		changed = true
	}
	if patch.ContractAddress != nil {
		v := strings.TrimSpace(*patch.ContractAddress)
		cfg.Override.ContractAddress = &v
		changed = true
	}
	if !changed {
		return nil
	}

	cfg.UpdatedAt = time.Now()
	if err := r.tipsRepo.Update(ctx, cfg); err != nil {
		if appErrors.HasCode(err, appErrors.CodeNotFound) {
			if err := r.tipsRepo.Create(ctx, cfg); err != nil {
				return err
			}
		} else {
			return err
		}
	}

	r.setCachedTipsConfig(cfg)
	return nil
}

// ClearTipsOverride removes the operator override layer from the instance tips config.
func (r *InstanceRepository) ClearTipsOverride(ctx context.Context) error {
	cfg, err := r.EnsureTipsConfig(ctx)
	if err != nil {
		return err
	}

	now := time.Now()
	builder := r.tipsRepo.db.WithContext(ctx).
		Model(&models.InstanceTipsConfig{}).
		Where("PK", "=", storage.InstanceConfigKey).
		Where("SK", "=", models.SKTipsConfig).
		UpdateBuilder()
	builder.Remove("override")
	builder.Set("UpdatedAt", now.UTC())

	if err := builder.Execute(); err != nil {
		return ErrorHandler.HandleUpdateError(err, "tips config", "override")
	}

	cfg.Override = nil
	cfg.UpdatedAt = now
	r.setCachedTipsConfig(cfg)
	return nil
}

// EffectiveTipsConfig resolves effective tips config values for runtime usage.
func (r *InstanceRepository) EffectiveTipsConfig(ctx context.Context) (*models.EffectiveTipsConfig, error) {
	cfg, err := r.GetTipsConfig(ctx)
	if err != nil {
		return nil, err
	}

	managed := cfg.Managed
	if managed == nil {
		managed = &models.InstanceTipsConfigManaged{}
	}

	enabled := managed.Enabled
	chainID := managed.ChainID
	contract := strings.TrimSpace(managed.ContractAddress)

	if cfg.Override != nil {
		if cfg.Override.Enabled != nil {
			enabled = *cfg.Override.Enabled
		}
		if cfg.Override.ChainID != nil {
			chainID = *cfg.Override.ChainID
		}
		if cfg.Override.ContractAddress != nil {
			contract = strings.TrimSpace(*cfg.Override.ContractAddress)
		}
	}

	out := &models.EffectiveTipsConfig{
		Enabled:         enabled,
		ChainID:         chainID,
		ContractAddress: contract,
	}
	return out, nil
}

// GetAIInstanceConfig returns the current instance AI configuration.
// If no record exists yet, it returns built-in defaults without persisting.
func (r *InstanceRepository) GetAIInstanceConfig(ctx context.Context) (*models.AIInstanceConfig, error) {
	if cached, ok := r.getCachedAIConfig(); ok {
		return cached, nil
	}

	cfg := &models.AIInstanceConfig{}
	err := r.aiConfigRepo.Get(ctx, storage.InstanceConfigKey, models.SKAIConfig, cfg)
	if err != nil {
		if appErrors.HasCode(err, appErrors.CodeNotFound) {
			defaultCfg := models.NewAIInstanceConfig()
			r.setCachedAIConfig(defaultCfg)
			return defaultCfg, nil
		}
		return nil, err
	}

	r.setCachedAIConfig(cfg)
	return cfg, nil
}

// EnsureAIInstanceConfig ensures the instance AI config record exists and returns it.
func (r *InstanceRepository) EnsureAIInstanceConfig(ctx context.Context) (*models.AIInstanceConfig, error) {
	cfg := &models.AIInstanceConfig{}
	err := r.aiConfigRepo.Get(ctx, storage.InstanceConfigKey, models.SKAIConfig, cfg)
	if err == nil {
		r.setCachedAIConfig(cfg)
		return cfg, nil
	}

	if !appErrors.HasCode(err, appErrors.CodeNotFound) {
		return nil, err
	}

	cfg = models.NewAIInstanceConfig()
	if createErr := r.aiConfigRepo.Create(ctx, cfg); createErr != nil {
		if appErrors.HasCode(createErr, appErrors.CodeAlreadyExists) || appErrors.HasCode(createErr, appErrors.CodeConflict) {
			cfg = &models.AIInstanceConfig{}
			if err := r.aiConfigRepo.Get(ctx, storage.InstanceConfigKey, models.SKAIConfig, cfg); err != nil {
				return nil, err
			}
			r.setCachedAIConfig(cfg)
			return cfg, nil
		}
		return nil, createErr
	}

	r.setCachedAIConfig(cfg)
	return cfg, nil
}

// SetAIManagedDefaults merges managed AI defaults into instance config.
func (r *InstanceRepository) SetAIManagedDefaults(ctx context.Context, patch models.AIInstanceConfigPatch) error {
	cfg, err := r.EnsureAIInstanceConfig(ctx)
	if err != nil {
		return err
	}
	if cfg.Managed == nil {
		cfg.Managed = &models.AIInstanceConfigManaged{}
	}

	changed := false
	if patch.AIEnabled != nil {
		cfg.Managed.AIEnabled = *patch.AIEnabled
		changed = true
	}
	if patch.ModerationEnabled != nil {
		cfg.Managed.ModerationEnabled = *patch.ModerationEnabled
		changed = true
	}
	if patch.NSFWDetectionEnabled != nil {
		cfg.Managed.NSFWDetectionEnabled = *patch.NSFWDetectionEnabled
		changed = true
	}
	if patch.SpamDetectionEnabled != nil {
		cfg.Managed.SpamDetectionEnabled = *patch.SpamDetectionEnabled
		changed = true
	}
	if patch.PIIDetectionEnabled != nil {
		cfg.Managed.PIIDetectionEnabled = *patch.PIIDetectionEnabled
		changed = true
	}
	if patch.AIContentDetection != nil {
		cfg.Managed.AIContentDetection = *patch.AIContentDetection
		changed = true
	}

	if !changed {
		return nil
	}

	cfg.UpdatedAt = time.Now()
	if err := r.aiConfigRepo.Update(ctx, cfg); err != nil {
		if appErrors.HasCode(err, appErrors.CodeNotFound) {
			if err := r.aiConfigRepo.Create(ctx, cfg); err != nil {
				return err
			}
		} else {
			return err
		}
	}

	r.setCachedAIConfig(cfg)
	return nil
}

// SetAIOverride merges operator AI overrides into instance config.
func (r *InstanceRepository) SetAIOverride(ctx context.Context, patch models.AIInstanceConfigPatch) error {
	cfg, err := r.EnsureAIInstanceConfig(ctx)
	if err != nil {
		return err
	}
	if cfg.Override == nil {
		cfg.Override = &models.AIInstanceConfigOverride{}
	}

	changed := false
	if patch.AIEnabled != nil {
		cfg.Override.AIEnabled = patch.AIEnabled
		changed = true
	}
	if patch.ModerationEnabled != nil {
		cfg.Override.ModerationEnabled = patch.ModerationEnabled
		changed = true
	}
	if patch.NSFWDetectionEnabled != nil {
		cfg.Override.NSFWDetectionEnabled = patch.NSFWDetectionEnabled
		changed = true
	}
	if patch.SpamDetectionEnabled != nil {
		cfg.Override.SpamDetectionEnabled = patch.SpamDetectionEnabled
		changed = true
	}
	if patch.PIIDetectionEnabled != nil {
		cfg.Override.PIIDetectionEnabled = patch.PIIDetectionEnabled
		changed = true
	}
	if patch.AIContentDetection != nil {
		cfg.Override.AIContentDetection = patch.AIContentDetection
		changed = true
	}
	if !changed {
		return nil
	}

	cfg.UpdatedAt = time.Now()
	if err := r.aiConfigRepo.Update(ctx, cfg); err != nil {
		if appErrors.HasCode(err, appErrors.CodeNotFound) {
			if err := r.aiConfigRepo.Create(ctx, cfg); err != nil {
				return err
			}
		} else {
			return err
		}
	}

	r.setCachedAIConfig(cfg)
	return nil
}

// ClearAIOverride removes the operator override layer from the instance AI config.
func (r *InstanceRepository) ClearAIOverride(ctx context.Context) error {
	cfg, err := r.EnsureAIInstanceConfig(ctx)
	if err != nil {
		return err
	}

	now := time.Now()
	builder := r.aiConfigRepo.db.WithContext(ctx).
		Model(&models.AIInstanceConfig{}).
		Where("PK", "=", storage.InstanceConfigKey).
		Where("SK", "=", models.SKAIConfig).
		UpdateBuilder()
	builder.Remove("override")
	builder.Set("UpdatedAt", now.UTC())

	if err := builder.Execute(); err != nil {
		return ErrorHandler.HandleUpdateError(err, "ai config", "override")
	}

	cfg.Override = nil
	cfg.UpdatedAt = now
	r.setCachedAIConfig(cfg)
	return nil
}

// EffectiveAIConfig resolves effective AI config values for runtime usage.
func (r *InstanceRepository) EffectiveAIConfig(ctx context.Context) (*models.EffectiveAIInstanceConfig, error) {
	cfg, err := r.GetAIInstanceConfig(ctx)
	if err != nil {
		return nil, err
	}

	managed := cfg.Managed
	if managed == nil {
		managed = &models.AIInstanceConfigManaged{
			AIEnabled:            cfg.LegacyAIEnabled,
			ModerationEnabled:    cfg.LegacyModerationEnabled,
			NSFWDetectionEnabled: cfg.LegacyNSFWDetectionEnabled,
			SpamDetectionEnabled: cfg.LegacySpamDetectionEnabled,
			PIIDetectionEnabled:  cfg.LegacyPIIDetectionEnabled,
			AIContentDetection:   cfg.LegacyAIContentDetection,
		}
	}

	out := &models.EffectiveAIInstanceConfig{
		AIEnabled:            managed.AIEnabled,
		ModerationEnabled:    managed.ModerationEnabled,
		NSFWDetectionEnabled: managed.NSFWDetectionEnabled,
		SpamDetectionEnabled: managed.SpamDetectionEnabled,
		PIIDetectionEnabled:  managed.PIIDetectionEnabled,
		AIContentDetection:   managed.AIContentDetection,
	}

	if cfg.Override != nil {
		if cfg.Override.AIEnabled != nil {
			out.AIEnabled = *cfg.Override.AIEnabled
		}
		if cfg.Override.ModerationEnabled != nil {
			out.ModerationEnabled = *cfg.Override.ModerationEnabled
		}
		if cfg.Override.NSFWDetectionEnabled != nil {
			out.NSFWDetectionEnabled = *cfg.Override.NSFWDetectionEnabled
		}
		if cfg.Override.SpamDetectionEnabled != nil {
			out.SpamDetectionEnabled = *cfg.Override.SpamDetectionEnabled
		}
		if cfg.Override.PIIDetectionEnabled != nil {
			out.PIIDetectionEnabled = *cfg.Override.PIIDetectionEnabled
		}
		if cfg.Override.AIContentDetection != nil {
			out.AIContentDetection = *cfg.Override.AIContentDetection
		}
	}

	return out, nil
}

func (r *InstanceRepository) warnInvalidEffectiveConfig(message string, fields ...zap.Field) {
	if r == nil || r.logger == nil {
		return
	}
	r.logger.Warn(message, fields...)
}

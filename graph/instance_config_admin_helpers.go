package graph

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"time"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/storage/models"
)

func (r *Resolver) resolveAdminInstanceConfig(ctx context.Context) (*model.AdminInstanceConfig, error) {
	if r == nil || r.Storage == nil || r.Storage.Instance() == nil {
		return nil, ErrStorageUnavailable
	}

	instanceRepo := r.Storage.Instance()

	trustCfg, trustEffective, trustExists, err := resolveAdminTrustConfig(ctx, instanceRepo)
	if err != nil {
		return nil, err
	}

	translationCfg, translationEffective, translationExists, err := resolveAdminTranslationConfig(ctx, instanceRepo)
	if err != nil {
		return nil, err
	}

	tipsCfg, tipsEffective, tipsExists, err := resolveAdminTipsConfig(ctx, instanceRepo)
	if err != nil {
		return nil, err
	}

	aiCfg, aiEffective, aiExists, err := resolveAdminAIConfig(ctx, instanceRepo)
	if err != nil {
		return nil, err
	}

	return &model.AdminInstanceConfig{
		Trust: buildAdminTrustConfig(trustCfg, trustEffective, trustExists),
		Translation: &model.AdminTranslationConfig{
			RecordExists:     translationExists,
			UpdatedAt:        model.Time(translationCfg.UpdatedAt),
			ManagedEnabled:   translationCfg.Managed.Enabled,
			OverrideEnabled:  translationOverrideEnabled(translationCfg),
			EffectiveEnabled: translationEffective,
		},
		Tips: buildAdminTipsConfig(tipsCfg, tipsEffective, tipsExists),
		Ai:   buildAdminAIConfig(aiCfg, aiEffective, aiExists),
	}, nil
}

func resolveAdminTrustConfig(ctx context.Context, instanceRepo interface {
	TrustConfigExists(context.Context) (bool, error)
	GetTrustConfig(context.Context) (*models.InstanceTrustConfig, error)
	EffectiveTrustConfig(context.Context) (*models.EffectiveTrustConfig, error)
}) (*models.InstanceTrustConfig, *models.EffectiveTrustConfig, bool, error) {
	return resolveAdminConfigRecord(ctx, instanceRepo.TrustConfigExists, instanceRepo.GetTrustConfig, models.NewInstanceTrustConfig, hasTrustConfigKeys, ensureTrustConfigManaged, instanceRepo.EffectiveTrustConfig)
}

func hasTrustConfigKeys(cfg *models.InstanceTrustConfig) bool {
	return cfg != nil && strings.TrimSpace(cfg.PK) != "" && strings.TrimSpace(cfg.SK) != ""
}

func ensureTrustConfigManaged(cfg *models.InstanceTrustConfig) {
	if cfg != nil && cfg.Managed == nil {
		cfg.Managed = &models.InstanceTrustConfigManaged{}
	}
}

func resolveAdminConfigRecord[T any, E any](
	ctx context.Context,
	existsFn func(context.Context) (bool, error),
	getFn func(context.Context) (T, error),
	newDefault func() T,
	hasKeys func(T) bool,
	ensureManaged func(T),
	effectiveFn func(context.Context) (E, error),
) (T, E, bool, error) {
	var zeroT T
	var zeroE E

	exists, err := existsFn(ctx)
	if err != nil {
		return zeroT, zeroE, false, err
	}

	cfg, err := getFn(ctx)
	if err != nil {
		return zeroT, zeroE, false, err
	}
	if hasKeys != nil && !hasKeys(cfg) {
		cfg = newDefault()
	}
	if ensureManaged != nil {
		ensureManaged(cfg)
	}

	effective, err := effectiveFn(ctx)
	if err != nil {
		return zeroT, zeroE, false, err
	}

	return cfg, effective, exists, nil
}

func resolveAdminTranslationConfig(ctx context.Context, instanceRepo interface {
	TranslationConfigExists(context.Context) (bool, error)
	GetTranslationConfig(context.Context) (*models.InstanceTranslationConfig, error)
	EffectiveTranslationEnabled(context.Context) (bool, error)
}) (*models.InstanceTranslationConfig, bool, bool, error) {
	exists, err := instanceRepo.TranslationConfigExists(ctx)
	if err != nil {
		return nil, false, false, err
	}

	cfg, err := instanceRepo.GetTranslationConfig(ctx)
	if err != nil {
		return nil, false, false, err
	}
	if cfg == nil || strings.TrimSpace(cfg.PK) == "" || strings.TrimSpace(cfg.SK) == "" {
		cfg = models.NewInstanceTranslationConfig()
	}
	if cfg.Managed == nil {
		cfg.Managed = &models.InstanceTranslationConfigManaged{}
	}

	effective, err := instanceRepo.EffectiveTranslationEnabled(ctx)
	if err != nil {
		return nil, false, false, err
	}

	return cfg, effective, exists, nil
}

func resolveAdminTipsConfig(ctx context.Context, instanceRepo interface {
	TipsConfigExists(context.Context) (bool, error)
	GetTipsConfig(context.Context) (*models.InstanceTipsConfig, error)
	EffectiveTipsConfig(context.Context) (*models.EffectiveTipsConfig, error)
}) (*models.InstanceTipsConfig, *models.EffectiveTipsConfig, bool, error) {
	return resolveAdminConfigRecord(ctx, instanceRepo.TipsConfigExists, instanceRepo.GetTipsConfig, models.NewInstanceTipsConfig, hasTipsConfigKeys, ensureTipsConfigManaged, instanceRepo.EffectiveTipsConfig)
}

func hasTipsConfigKeys(cfg *models.InstanceTipsConfig) bool {
	return cfg != nil && strings.TrimSpace(cfg.PK) != "" && strings.TrimSpace(cfg.SK) != ""
}

func ensureTipsConfigManaged(cfg *models.InstanceTipsConfig) {
	if cfg != nil && cfg.Managed == nil {
		cfg.Managed = &models.InstanceTipsConfigManaged{}
	}
}

func resolveAdminAIConfig(ctx context.Context, instanceRepo interface {
	AIConfigExists(context.Context) (bool, error)
	GetAIInstanceConfig(context.Context) (*models.AIInstanceConfig, error)
	EffectiveAIConfig(context.Context) (*models.EffectiveAIInstanceConfig, error)
}) (*models.AIInstanceConfig, *models.EffectiveAIInstanceConfig, bool, error) {
	exists, err := instanceRepo.AIConfigExists(ctx)
	if err != nil {
		return nil, nil, false, err
	}

	cfg, err := instanceRepo.GetAIInstanceConfig(ctx)
	if err != nil {
		return nil, nil, false, err
	}
	if cfg == nil || strings.TrimSpace(cfg.PK) == "" || strings.TrimSpace(cfg.SK) == "" {
		cfg = models.NewAIInstanceConfig()
	}
	if cfg.Managed == nil {
		cfg.Managed = &models.AIInstanceConfigManaged{
			AIEnabled:            cfg.LegacyAIEnabled,
			ModerationEnabled:    cfg.LegacyModerationEnabled,
			NSFWDetectionEnabled: cfg.LegacyNSFWDetectionEnabled,
			SpamDetectionEnabled: cfg.LegacySpamDetectionEnabled,
			PIIDetectionEnabled:  cfg.LegacyPIIDetectionEnabled,
			AIContentDetection:   cfg.LegacyAIContentDetection,
		}
	}

	effective, err := instanceRepo.EffectiveAIConfig(ctx)
	if err != nil {
		return nil, nil, false, err
	}

	return cfg, effective, exists, nil
}

func buildAdminTrustConfig(cfg *models.InstanceTrustConfig, effective *models.EffectiveTrustConfig, exists bool) *model.AdminTrustConfig {
	updatedAt := cfg.UpdatedAt
	if updatedAt.IsZero() {
		updatedAt = time.Time{}
	}

	var override *model.AdminTrustConfigLayerOverride
	if cfg.Override != nil {
		override = &model.AdminTrustConfigLayerOverride{
			BaseURL:              cfg.Override.BaseURL,
			AttestationsURL:      cfg.Override.AttestationsURL,
			InstanceKeySecretArn: cfg.Override.InstanceKeySecretARN,
		}
	}

	return &model.AdminTrustConfig{
		RecordExists: exists,
		UpdatedAt:    model.Time(updatedAt),
		Managed: &model.AdminTrustConfigLayer{
			BaseURL:              strings.TrimSpace(cfg.Managed.BaseURL),
			AttestationsURL:      strings.TrimSpace(cfg.Managed.AttestationsURL),
			InstanceKeySecretArn: strings.TrimSpace(cfg.Managed.InstanceKeySecretARN),
		},
		Override: override,
		Effective: &model.AdminEffectiveTrustConfig{
			BaseURL:                   strings.TrimSpace(effective.TrustBaseURL),
			AttestationsURL:           strings.TrimSpace(effective.AttestationsBaseURL),
			InstanceKeySecretArn:      strings.TrimSpace(effective.InstanceKeySecretARN),
			TrustProxyEnabled:         effective.TrustProxyEnabled,
			PublicAttestationsEnabled: effective.PublicAttestationsEnabled,
		},
	}
}

func buildAdminTipsConfig(cfg *models.InstanceTipsConfig, effective *models.EffectiveTipsConfig, exists bool) *model.AdminTipsConfig {
	var override *model.AdminTipsConfigLayerOverride
	if cfg.Override != nil {
		override = &model.AdminTipsConfigLayerOverride{
			Enabled:         cfg.Override.Enabled,
			ChainID:         cfg.Override.ChainID,
			ContractAddress: cfg.Override.ContractAddress,
		}
	}

	var chainIDPtr *int
	var contractPtr *string
	if effective.Enabled {
		chainID := effective.ChainID
		chainIDPtr = &chainID
		contractAddress := strings.TrimSpace(effective.ContractAddress)
		contractPtr = &contractAddress
	}

	return &model.AdminTipsConfig{
		RecordExists: exists,
		UpdatedAt:    model.Time(cfg.UpdatedAt),
		Managed: &model.AdminTipsConfigLayer{
			Enabled:         cfg.Managed.Enabled,
			ChainID:         cfg.Managed.ChainID,
			ContractAddress: strings.TrimSpace(cfg.Managed.ContractAddress),
		},
		Override: override,
		Effective: &model.TipsConfig{
			Enabled:         effective.Enabled,
			ChainID:         chainIDPtr,
			ContractAddress: contractPtr,
		},
	}
}

func buildAdminAIConfig(cfg *models.AIInstanceConfig, effective *models.EffectiveAIInstanceConfig, exists bool) *model.AdminAIConfig {
	var override *model.AdminAIConfigLayerOverride
	if cfg.Override != nil {
		override = &model.AdminAIConfigLayerOverride{
			AiEnabled:            cfg.Override.AIEnabled,
			ModerationEnabled:    cfg.Override.ModerationEnabled,
			NsfwDetectionEnabled: cfg.Override.NSFWDetectionEnabled,
			SpamDetectionEnabled: cfg.Override.SpamDetectionEnabled,
			PiiDetectionEnabled:  cfg.Override.PIIDetectionEnabled,
			AiContentDetection:   cfg.Override.AIContentDetection,
		}
	}

	return &model.AdminAIConfig{
		RecordExists: exists,
		UpdatedAt:    model.Time(cfg.UpdatedAt),
		Managed: &model.AdminAIConfigLayer{
			AiEnabled:            cfg.Managed.AIEnabled,
			ModerationEnabled:    cfg.Managed.ModerationEnabled,
			NsfwDetectionEnabled: cfg.Managed.NSFWDetectionEnabled,
			SpamDetectionEnabled: cfg.Managed.SpamDetectionEnabled,
			PiiDetectionEnabled:  cfg.Managed.PIIDetectionEnabled,
			AiContentDetection:   cfg.Managed.AIContentDetection,
		},
		Override: override,
		Effective: &model.AdminAIConfigLayer{
			AiEnabled:            effective.AIEnabled,
			ModerationEnabled:    effective.ModerationEnabled,
			NsfwDetectionEnabled: effective.NSFWDetectionEnabled,
			SpamDetectionEnabled: effective.SpamDetectionEnabled,
			PiiDetectionEnabled:  effective.PIIDetectionEnabled,
			AiContentDetection:   effective.AIContentDetection,
		},
	}
}

func translationOverrideEnabled(cfg *models.InstanceTranslationConfig) *bool {
	if cfg == nil || cfg.Override == nil {
		return nil
	}
	return cfg.Override.Enabled
}

func validateAdminTrustBaseURL(raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}

	u, err := url.Parse(raw)
	if err != nil {
		return err
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return errors.New("base URL must include http(s) scheme")
	}

	host := strings.ToLower(strings.TrimSpace(u.Hostname()))
	if host == "" {
		return errors.New("base URL missing hostname")
	}
	if strings.Contains(host, ".lambda-url.") && strings.HasSuffix(host, ".on.aws") {
		return errors.New("lambda function URL hosts are not supported")
	}

	return nil
}

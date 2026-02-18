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

	trustExists, err := instanceRepo.TrustConfigExists(ctx)
	if err != nil {
		return nil, err
	}
	trustCfg, err := instanceRepo.GetTrustConfig(ctx)
	if err != nil {
		return nil, err
	}
	if trustCfg == nil || strings.TrimSpace(trustCfg.PK) == "" || strings.TrimSpace(trustCfg.SK) == "" {
		trustCfg = models.NewInstanceTrustConfig()
	}
	if trustCfg.Managed == nil {
		trustCfg.Managed = &models.InstanceTrustConfigManaged{}
	}
	trustEffective, err := instanceRepo.EffectiveTrustConfig(ctx)
	if err != nil {
		return nil, err
	}

	translationExists, err := instanceRepo.TranslationConfigExists(ctx)
	if err != nil {
		return nil, err
	}
	translationCfg, err := instanceRepo.GetTranslationConfig(ctx)
	if err != nil {
		return nil, err
	}
	if translationCfg == nil || strings.TrimSpace(translationCfg.PK) == "" || strings.TrimSpace(translationCfg.SK) == "" {
		translationCfg = models.NewInstanceTranslationConfig()
	}
	if translationCfg.Managed == nil {
		translationCfg.Managed = &models.InstanceTranslationConfigManaged{}
	}
	translationEffective, err := instanceRepo.EffectiveTranslationEnabled(ctx)
	if err != nil {
		return nil, err
	}

	tipsExists, err := instanceRepo.TipsConfigExists(ctx)
	if err != nil {
		return nil, err
	}
	tipsCfg, err := instanceRepo.GetTipsConfig(ctx)
	if err != nil {
		return nil, err
	}
	if tipsCfg == nil || strings.TrimSpace(tipsCfg.PK) == "" || strings.TrimSpace(tipsCfg.SK) == "" {
		tipsCfg = models.NewInstanceTipsConfig()
	}
	if tipsCfg.Managed == nil {
		tipsCfg.Managed = &models.InstanceTipsConfigManaged{}
	}
	tipsEffective, err := instanceRepo.EffectiveTipsConfig(ctx)
	if err != nil {
		return nil, err
	}

	aiExists, err := instanceRepo.AIConfigExists(ctx)
	if err != nil {
		return nil, err
	}
	aiCfg, err := instanceRepo.GetAIInstanceConfig(ctx)
	if err != nil {
		return nil, err
	}
	if aiCfg == nil || strings.TrimSpace(aiCfg.PK) == "" || strings.TrimSpace(aiCfg.SK) == "" {
		aiCfg = models.NewAIInstanceConfig()
	}
	if aiCfg.Managed == nil {
		aiCfg.Managed = &models.AIInstanceConfigManaged{
			AIEnabled:            aiCfg.LegacyAIEnabled,
			ModerationEnabled:    aiCfg.LegacyModerationEnabled,
			NSFWDetectionEnabled: aiCfg.LegacyNSFWDetectionEnabled,
			SpamDetectionEnabled: aiCfg.LegacySpamDetectionEnabled,
			PIIDetectionEnabled:  aiCfg.LegacyPIIDetectionEnabled,
			AIContentDetection:   aiCfg.LegacyAIContentDetection,
		}
	}
	aiEffective, err := instanceRepo.EffectiveAIConfig(ctx)
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

package handlers

import (
	"context"
	"os"
	"strconv"
	"strings"

	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"go.uber.org/zap"
)

// MigrateInstanceFeatureConfigFromEnv persists legacy env-var configuration into instance-owned DynamoDB records.
//
// This is a one-time, best-effort migration path intended to eliminate env-var drift across redeploys.
// If a config record already exists, it is left untouched.
func (h *Handler) MigrateInstanceFeatureConfigFromEnv(ctx context.Context) {
	if h == nil || h.repos == nil || h.repos.Instance() == nil || h.logger == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}

	instanceRepo := h.repos.Instance()

	h.migrateTrustConfigFromEnv(ctx, instanceRepo)
	h.migrateTranslationConfigFromEnv(ctx, instanceRepo)
	h.migrateTipsConfigFromEnv(ctx, instanceRepo)
}

func (h *Handler) migrateTrustConfigFromEnv(ctx context.Context, instanceRepo interface {
	TrustConfigExists(context.Context) (bool, error)
	EnsureTrustConfig(context.Context) (*storagemodels.InstanceTrustConfig, error)
	SetTrustManagedDefaults(context.Context, storagemodels.InstanceTrustConfigPatch) error
}) {
	exists, err := instanceRepo.TrustConfigExists(ctx)
	if err != nil {
		h.logger.Warn("instance feature config migration: failed to check trust config", zap.Error(err))
		return
	}
	if exists {
		return
	}

	baseURL := strings.TrimRight(strings.TrimSpace(os.Getenv("LESSER_HOST_URL")), "/")
	attestationsURL := strings.TrimRight(strings.TrimSpace(os.Getenv("LESSER_HOST_ATTESTATIONS_URL")), "/")
	secretARN := strings.TrimSpace(os.Getenv("LESSER_HOST_INSTANCE_KEY_ARN"))

	if baseURL == "" && attestationsURL == "" && secretARN == "" {
		return
	}

	if secretARN == "" {
		h.warnTrustMigrationSkippedMissingSecretARN()
		return
	}

	if baseURL == "" {
		baseURL = attestationsURL
	}
	if baseURL == "" {
		h.logger.Warn("instance feature config migration: trust env vars set but missing base URL; skipping",
			zap.String("missing_env", "LESSER_HOST_URL/LESSER_HOST_ATTESTATIONS_URL"),
		)
		return
	}

	if _, err := instanceRepo.EnsureTrustConfig(ctx); err != nil {
		h.logger.Warn("instance feature config migration: failed to ensure trust config record", zap.Error(err))
		return
	}

	patch := storagemodels.InstanceTrustConfigPatch{
		BaseURL:              stringPtr(baseURL),
		AttestationsURL:      stringPtrOrNil(attestationsURL),
		InstanceKeySecretARN: stringPtr(secretARN),
	}
	if err := instanceRepo.SetTrustManagedDefaults(ctx, patch); err != nil {
		h.logger.Warn("instance feature config migration: failed to persist trust config managed defaults", zap.Error(err))
		return
	}

	h.logger.Info("migrated TRUST_CONFIG managed defaults from env",
		zap.String("lesser_host_url", baseURL),
		zap.Bool("attestations_url_set", attestationsURL != ""),
	)
}

func (h *Handler) migrateTranslationConfigFromEnv(ctx context.Context, instanceRepo interface {
	TranslationConfigExists(context.Context) (bool, error)
	EnsureTranslationConfig(context.Context) (*storagemodels.InstanceTranslationConfig, error)
	SetTranslationManagedDefaults(context.Context, storagemodels.InstanceTranslationConfigPatch) error
}) {
	exists, err := instanceRepo.TranslationConfigExists(ctx)
	if err != nil {
		h.logger.Warn("instance feature config migration: failed to check translation config", zap.Error(err))
		return
	}
	if exists {
		return
	}

	raw := strings.TrimSpace(os.Getenv("TRANSLATION_ENABLED"))
	if raw == "" {
		return
	}
	enabled := parseEnvBool(raw)

	if _, err := instanceRepo.EnsureTranslationConfig(ctx); err != nil {
		h.logger.Warn("instance feature config migration: failed to ensure translation config record", zap.Error(err))
		return
	}
	if err := instanceRepo.SetTranslationManagedDefaults(ctx, storagemodels.InstanceTranslationConfigPatch{Enabled: &enabled}); err != nil {
		h.logger.Warn("instance feature config migration: failed to persist translation config managed defaults", zap.Error(err))
		return
	}

	h.logger.Info("migrated TRANSLATION_CONFIG managed defaults from env", zap.Bool("enabled", enabled))
}

func (h *Handler) migrateTipsConfigFromEnv(ctx context.Context, instanceRepo interface {
	TipsConfigExists(context.Context) (bool, error)
	EnsureTipsConfig(context.Context) (*storagemodels.InstanceTipsConfig, error)
	SetTipsManagedDefaults(context.Context, storagemodels.InstanceTipsConfigPatch) error
}) {
	exists, err := instanceRepo.TipsConfigExists(ctx)
	if err != nil {
		h.logger.Warn("instance feature config migration: failed to check tips config", zap.Error(err))
		return
	}
	if exists {
		return
	}

	rawEnabled := strings.TrimSpace(os.Getenv("TIP_ENABLED"))
	rawChainID := strings.TrimSpace(os.Getenv("TIP_CHAIN_ID"))
	contract := strings.TrimSpace(os.Getenv("TIP_CONTRACT_ADDRESS"))

	if rawEnabled == "" && rawChainID == "" && contract == "" {
		return
	}

	patch := storagemodels.InstanceTipsConfigPatch{}
	if rawEnabled != "" {
		enabled := parseEnvBool(rawEnabled)
		patch.Enabled = &enabled
	}
	if rawChainID != "" {
		if v, err := strconv.Atoi(rawChainID); err == nil {
			patch.ChainID = &v
		}
	}
	if strings.TrimSpace(contract) != "" {
		patch.ContractAddress = stringPtr(contract)
	}

	if patch.Enabled != nil && *patch.Enabled {
		if patch.ChainID == nil || patch.ContractAddress == nil || strings.TrimSpace(*patch.ContractAddress) == "" {
			h.logger.Warn("instance feature config migration: TIP_ENABLED=true but missing TIP_CHAIN_ID or TIP_CONTRACT_ADDRESS; disabling tips during migration")
			disabled := false
			patch.Enabled = &disabled
		}
	}

	if _, err := instanceRepo.EnsureTipsConfig(ctx); err != nil {
		h.logger.Warn("instance feature config migration: failed to ensure tips config record", zap.Error(err))
		return
	}
	if err := instanceRepo.SetTipsManagedDefaults(ctx, patch); err != nil {
		h.logger.Warn("instance feature config migration: failed to persist tips config managed defaults", zap.Error(err))
		return
	}

	h.logger.Info("migrated TIPS_CONFIG managed defaults from env",
		zap.Bool("enabled_set", patch.Enabled != nil),
		zap.Bool("chain_id_set", patch.ChainID != nil),
		zap.Bool("contract_address_set", patch.ContractAddress != nil && strings.TrimSpace(*patch.ContractAddress) != ""),
	)
}

func parseEnvBool(raw string) bool {
	raw = strings.ToLower(strings.TrimSpace(raw))
	return raw == boolTrue || raw == "1" || raw == "yes"
}

func stringPtr(raw string) *string {
	v := strings.TrimSpace(raw)
	return &v
}

func stringPtrOrNil(raw string) *string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	v := raw
	return &v
}

package main

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"go.uber.org/zap"
)

func (e *upEnv) syncInstanceFeatureConfig(ctx context.Context, receipt *upReceipt) error {
	if e == nil || receipt == nil {
		return nil
	}

	provisioningProvided := strings.TrimSpace(e.args.ProvisioningInputPath) != ""
	if !provisioningProvided && !envFeatureConfigSeedAvailable() {
		return nil
	}

	for _, stage := range e.stages {
		stageReceipt := receipt.Stages[string(stage)]
		if stageReceipt == nil {
			continue
		}
		tableName := strings.TrimSpace(stageReceipt.TableName)
		if tableName == "" {
			continue
		}

		db, err := e.newDB()
		if err != nil {
			return err
		}

		prevTableName := models.MainTableName
		models.MainTableName = tableName

		instanceRepo := repositories.NewInstanceRepository(db, tableName, zap.NewNop())

		if provisioningProvided {
			if err := ensureAndApplyManagedProvisioningConfig(ctx, instanceRepo, e.args); err != nil {
				_ = db.Close()
				models.MainTableName = prevTableName
				return fmt.Errorf("apply managed instance config for stage %s: %w", stage, err)
			}
		} else {
			if err := seedManagedConfigFromEnvOnce(ctx, instanceRepo, string(stage), tableName); err != nil {
				_ = db.Close()
				models.MainTableName = prevTableName
				return fmt.Errorf("seed env instance config for stage %s: %w", stage, err)
			}
		}

		models.MainTableName = prevTableName
		_ = db.Close()
	}

	return nil
}

func envFeatureConfigSeedAvailable() bool {
	keys := []string{
		"LESSER_HOST_URL",
		"LESSER_HOST_ATTESTATIONS_URL",
		"LESSER_HOST_INSTANCE_KEY_ARN",
		"TRANSLATION_ENABLED",
		"TIP_ENABLED",
		"TIP_CHAIN_ID",
		"TIP_CONTRACT_ADDRESS",
	}
	for _, key := range keys {
		if strings.TrimSpace(os.Getenv(key)) != "" {
			return true
		}
	}
	return false
}

func ensureAndApplyManagedProvisioningConfig(ctx context.Context, instanceRepo *repositories.InstanceRepository, args upArgs) error {
	if instanceRepo == nil {
		return nil
	}

	if _, err := instanceRepo.EnsureTrustConfig(ctx); err != nil {
		return err
	}
	if _, err := instanceRepo.EnsureTranslationConfig(ctx); err != nil {
		return err
	}
	if _, err := instanceRepo.EnsureTipsConfig(ctx); err != nil {
		return err
	}
	if _, err := instanceRepo.EnsureAIInstanceConfig(ctx); err != nil {
		return err
	}

	trustPatch := models.InstanceTrustConfigPatch{
		BaseURL:              stringPtrOrNil(strings.TrimSpace(args.LesserHostURL)),
		AttestationsURL:      stringPtrOrNil(strings.TrimSpace(args.LesserHostAttestationsURL)),
		InstanceKeySecretARN: stringPtrOrNil(strings.TrimSpace(args.LesserHostInstanceKeyARN)),
	}
	if err := instanceRepo.SetTrustManagedDefaults(ctx, trustPatch); err != nil {
		return err
	}

	if err := instanceRepo.SetTranslationManagedDefaults(ctx, models.InstanceTranslationConfigPatch{Enabled: args.TranslationEnabled}); err != nil {
		return err
	}

	tipsPatch := models.InstanceTipsConfigPatch{
		Enabled:         args.TipEnabled,
		ChainID:         args.TipChainID,
		ContractAddress: stringPtrOrNil(strings.TrimSpace(args.TipContractAddress)),
	}
	if err := instanceRepo.SetTipsManagedDefaults(ctx, tipsPatch); err != nil {
		return err
	}

	aiPatch := models.AIInstanceConfigPatch{
		AIEnabled:            args.AIEnabled,
		ModerationEnabled:    args.AIModerationEnabled,
		NSFWDetectionEnabled: args.AINsfwDetectionEnabled,
		SpamDetectionEnabled: args.AISpamDetectionEnabled,
		PIIDetectionEnabled:  args.AIPiiDetectionEnabled,
		AIContentDetection:   args.AIContentDetectionEnabled,
	}
	if err := instanceRepo.SetAIManagedDefaults(ctx, aiPatch); err != nil {
		return err
	}

	return nil
}

func seedManagedConfigFromEnvOnce(ctx context.Context, instanceRepo *repositories.InstanceRepository, stageName, tableName string) error {
	if instanceRepo == nil {
		return nil
	}

	if err := seedTrustManagedDefaultsFromEnvOnce(ctx, instanceRepo, stageName, tableName); err != nil {
		return err
	}
	if err := seedTranslationManagedDefaultsFromEnvOnce(ctx, instanceRepo, stageName, tableName); err != nil {
		return err
	}
	if err := seedTipsManagedDefaultsFromEnvOnce(ctx, instanceRepo, stageName, tableName); err != nil {
		return err
	}

	return nil
}

func seedTrustManagedDefaultsFromEnvOnce(ctx context.Context, instanceRepo *repositories.InstanceRepository, stageName, tableName string) error {
	trustExists, err := instanceRepo.TrustConfigExists(ctx)
	if err != nil {
		return err
	}
	if trustExists {
		return nil
	}

	trustPatch := envTrustPatch()
	if trustPatch.BaseURL == nil && trustPatch.AttestationsURL == nil && trustPatch.InstanceKeySecretARN == nil {
		return nil
	}

	if _, err := instanceRepo.EnsureTrustConfig(ctx); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "WARNING: seeding TRUST_CONFIG managed defaults from env for %s (%s)\n", stageName, tableName)
	return instanceRepo.SetTrustManagedDefaults(ctx, trustPatch)
}

func seedTranslationManagedDefaultsFromEnvOnce(ctx context.Context, instanceRepo *repositories.InstanceRepository, stageName, tableName string) error {
	translationExists, err := instanceRepo.TranslationConfigExists(ctx)
	if err != nil {
		return err
	}
	if translationExists {
		return nil
	}

	enabled := envBoolPtr("TRANSLATION_ENABLED")
	if enabled == nil {
		return nil
	}

	if _, err := instanceRepo.EnsureTranslationConfig(ctx); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "WARNING: seeding TRANSLATION_CONFIG managed defaults from env for %s (%s)\n", stageName, tableName)
	return instanceRepo.SetTranslationManagedDefaults(ctx, models.InstanceTranslationConfigPatch{Enabled: enabled})
}

func seedTipsManagedDefaultsFromEnvOnce(ctx context.Context, instanceRepo *repositories.InstanceRepository, stageName, tableName string) error {
	tipsExists, err := instanceRepo.TipsConfigExists(ctx)
	if err != nil {
		return err
	}
	if tipsExists {
		return nil
	}

	tipsPatch := envTipsPatch()
	if tipsPatch.Enabled == nil && tipsPatch.ChainID == nil && tipsPatch.ContractAddress == nil {
		return nil
	}

	if _, err := instanceRepo.EnsureTipsConfig(ctx); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "WARNING: seeding TIPS_CONFIG managed defaults from env for %s (%s)\n", stageName, tableName)
	return instanceRepo.SetTipsManagedDefaults(ctx, tipsPatch)
}

func envTrustPatch() models.InstanceTrustConfigPatch {
	base := strings.TrimRight(strings.TrimSpace(os.Getenv("LESSER_HOST_URL")), "/")
	att := strings.TrimRight(strings.TrimSpace(os.Getenv("LESSER_HOST_ATTESTATIONS_URL")), "/")
	secretARN := strings.TrimSpace(os.Getenv("LESSER_HOST_INSTANCE_KEY_ARN"))

	return models.InstanceTrustConfigPatch{
		BaseURL:              stringPtrOrNil(base),
		AttestationsURL:      stringPtrOrNil(att),
		InstanceKeySecretARN: stringPtrOrNil(secretARN),
	}
}

func envTipsPatch() models.InstanceTipsConfigPatch {
	enabled := envBoolPtr("TIP_ENABLED")
	chainID := envIntPtr("TIP_CHAIN_ID")
	contract := strings.TrimSpace(os.Getenv("TIP_CONTRACT_ADDRESS"))

	return models.InstanceTipsConfigPatch{
		Enabled:         enabled,
		ChainID:         chainID,
		ContractAddress: stringPtrOrNil(contract),
	}
}

func envBoolPtr(envKey string) *bool {
	raw := strings.TrimSpace(os.Getenv(envKey))
	if raw == "" {
		return nil
	}

	raw = strings.ToLower(raw)
	v := raw == flagTrue || raw == "1" || raw == flagYes
	return &v
}

func envIntPtr(envKey string) *int {
	raw := strings.TrimSpace(os.Getenv(envKey))
	if raw == "" {
		return nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return nil
	}
	return &n
}

func stringPtrOrNil(raw string) *string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	v := raw
	return &v
}

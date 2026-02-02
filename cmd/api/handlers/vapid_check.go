package handlers

import (
	"context"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/storage/core"
	"go.uber.org/zap"
)

// IsProductionEnvironment checks if the current environment is production
func IsProductionEnvironment(cfg *config.Config) bool {
	env := cfg.Stage
	return env == "production" || env == "prod"
}

// ValidateVAPIDKeysForProduction validates that VAPID keys are available in production
func ValidateVAPIDKeysForProduction(ctx context.Context, cfg *config.Config, repos core.RepositoryStorage, logger *zap.Logger) error {
	// Only check in production environment
	if !IsProductionEnvironment(cfg) {
		logger.Info("Non-production environment detected, skipping VAPID key validation")
		return nil
	}

	logger.Info("Production environment detected, validating VAPID keys")

	// Check if VAPID keys exist in storage
	_, err := repos.PushSubscription().GetVAPIDKeys(ctx)
	if err != nil {
		logger.Error("VAPID keys are required in production but not found", zap.Error(err))
		return err
	}

	// Check if VAPID public key configuration is set (optional but recommended)
	vapidPublicKey := cfg.VAPIDPublicKey
	if err := common.ValidateRequiredParam("vapidPublicKey", vapidPublicKey); err != nil {
		logger.Warn("VAPID_PUBLIC_KEY configuration not set - using keys from storage only")
	} else {
		logger.Info("VAPID configuration validated successfully")
	}

	return nil
}

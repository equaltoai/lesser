package lift

import (
	"context"
	"os"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage/core"
	"go.uber.org/zap"
)

// IsProductionEnvironment checks if the current environment is production
func IsProductionEnvironment() bool {
	env := os.Getenv("ENV")
	if err := common.ValidateRequiredParam("env", env); err != nil {
		env = os.Getenv("ENVIRONMENT")
	}
	return env == "production" || env == "prod"
}

// ValidateVAPIDKeysForProduction validates that VAPID keys are available in production
func ValidateVAPIDKeysForProduction(ctx context.Context, repos core.RepositoryStorage, logger *zap.Logger) error {
	// Only check in production environment
	if !IsProductionEnvironment() {
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

	// Check if VAPID public key environment variable is set (optional but recommended)
	vapidPublicKey := os.Getenv("VAPID_PUBLIC_KEY")
	if err := common.ValidateRequiredParam("vapidPublicKey", vapidPublicKey); err != nil {
		logger.Warn("VAPID_PUBLIC_KEY environment variable not set - using keys from storage only")
	} else {
		logger.Info("VAPID configuration validated successfully")
	}

	return nil
}

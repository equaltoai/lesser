package handlers

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/deploy/naming"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/core"
	"go.uber.org/zap"
)

// IsProductionEnvironment checks if the current environment is production
func IsProductionEnvironment(cfg *config.Config) bool {
	return naming.IsLiveEnvironment(cfg.Stage)
}

// ValidateVAPIDKeysForProduction validates that VAPID keys are available in production
func ValidateVAPIDKeysForProduction(ctx context.Context, cfg *config.Config, repos core.RepositoryStorage, logger *zap.Logger) error {
	if strings.TrimSpace(cfg.VAPIDSecretARN) == "" {
		if IsProductionEnvironment(cfg) {
			err := errors.New("VAPID_SECRET_ARN is required in production for durable VAPID key persistence")
			logger.Error("VAPID Secrets Manager configuration is missing", zap.Error(err))
			return err
		}

		logger.Warn("VAPID_SECRET_ARN is not configured; skipping non-production VAPID bootstrap because generated keys could not be persisted durably")
		return nil
	}

	pushRepo := repos.PushSubscription()
	if pushRepo == nil {
		logger.Warn("push subscription repository unavailable; skipping VAPID key validation")
		if IsProductionEnvironment(cfg) {
			return errors.New("push subscription repository unavailable")
		}
		return nil
	}

	// Check if VAPID keys exist in storage
	_, err := pushRepo.GetVAPIDKeys(ctx)
	if err == nil {
		// Keys exist, check if config is set
		vapidPublicKey := cfg.VAPIDPublicKey
		if err := common.ValidateRequiredParam("vapidPublicKey", vapidPublicKey); err != nil {
			logger.Warn("VAPID_PUBLIC_KEY configuration not set - using keys from storage only")
		} else {
			logger.Info("VAPID configuration validated successfully")
		}
		return nil
	}

	// Keys not found - auto-generate them
	logger.Info("VAPID keys not found, auto-generating")
	publicKey, privateKey, genErr := generateVAPIDKeyPair()
	if genErr != nil {
		logger.Error("failed to auto-generate VAPID keys", zap.Error(genErr))
		if IsProductionEnvironment(cfg) {
			return genErr
		}
		return nil
	}

	keys := &storage.VAPIDKeys{
		PublicKey:  publicKey,
		PrivateKey: privateKey,
		Subject:    "mailto:admin@" + cfg.Domain,
		CreatedAt:  time.Now().UTC(),
		UpdatedAt:  time.Now().UTC(),
	}

	if setErr := pushRepo.SetVAPIDKeys(ctx, keys); setErr != nil {
		logger.Error("failed to store auto-generated VAPID keys", zap.Error(setErr))
		if IsProductionEnvironment(cfg) {
			return setErr
		}
		return nil
	}

	logger.Info("VAPID keys auto-generated and stored successfully",
		zap.String("public_key", publicKey))
	return nil
}

// generateVAPIDKeyPair generates a new ECDSA P-256 key pair for VAPID.
//
// The push-delivery processor expects the private key as the raw P-256 scalar
// encoded with base64url, matching the format produced by the API's runtime
// /api/v1/instance VAPID bootstrap path.
func generateVAPIDKeyPair() (publicKeyB64 string, privateKeyB64 string, err error) {
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return "", "", err
	}

	privateKeyBytes, err := privateKey.Bytes()
	if err != nil {
		return "", "", err
	}

	ecdhKey, err := privateKey.ECDH()
	if err != nil {
		return "", "", err
	}
	publicKeyBytes := ecdhKey.PublicKey().Bytes()
	publicKeyB64 = base64.RawURLEncoding.EncodeToString(publicKeyBytes)
	privateKeyB64 = base64.RawURLEncoding.EncodeToString(privateKeyBytes)

	return publicKeyB64, privateKeyB64, nil
}

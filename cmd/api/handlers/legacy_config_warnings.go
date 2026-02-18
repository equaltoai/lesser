package handlers

import (
	"os"
	"strings"

	"go.uber.org/zap"
)

func envAnySet(keys ...string) bool {
	for _, key := range keys {
		if strings.TrimSpace(os.Getenv(key)) != "" {
			return true
		}
	}
	return false
}

func (h *Handler) warnLegacyTrustConfig() {
	if h == nil || h.logger == nil {
		return
	}
	if !envAnySet("LESSER_HOST_URL", "LESSER_HOST_ATTESTATIONS_URL", "LESSER_HOST_INSTANCE_KEY", "LESSER_HOST_INSTANCE_KEY_ARN") {
		return
	}
	h.legacyTrustConfigWarnOnce.Do(func() {
		h.logger.Warn("TRUST_CONFIG missing; falling back to legacy LESSER_HOST_* env vars (deprecated). Re-run `lesser up` (or set instance config) to persist trust settings and avoid redeploy drift.")
	})
}

func (h *Handler) warnLegacyTranslationConfig() {
	if h == nil || h.logger == nil {
		return
	}
	if !envAnySet("TRANSLATION_ENABLED") {
		return
	}
	h.legacyTranslationConfigWarnOnce.Do(func() {
		h.logger.Warn("TRANSLATION_CONFIG missing; falling back to legacy TRANSLATION_ENABLED env var (deprecated). Re-run `lesser up` (or set instance config) to persist translation settings.")
	})
}

func (h *Handler) warnLegacyTipsConfig() {
	if h == nil || h.logger == nil {
		return
	}
	if !envAnySet("TIP_ENABLED", "TIP_CHAIN_ID", "TIP_CONTRACT_ADDRESS") {
		return
	}
	h.legacyTipsConfigWarnOnce.Do(func() {
		h.logger.Warn("TIPS_CONFIG missing; falling back to legacy TIP_* env vars (deprecated). Re-run `lesser up` (or set instance config) to persist tips settings and avoid redeploy drift.")
	})
}

func (h *Handler) warnTrustMigrationSkippedMissingSecretARN() {
	if h == nil || h.logger == nil {
		return
	}
	if !envAnySet("LESSER_HOST_URL", "LESSER_HOST_ATTESTATIONS_URL", "LESSER_HOST_INSTANCE_KEY") {
		return
	}
	if strings.TrimSpace(os.Getenv("LESSER_HOST_INSTANCE_KEY_ARN")) != "" {
		return
	}
	h.legacyTrustConfigWarnOnce.Do(func() {
		h.logger.Warn("TRUST_CONFIG migration skipped: LESSER_HOST_INSTANCE_KEY_ARN not set; cannot persist a plaintext LESSER_HOST_INSTANCE_KEY into DynamoDB. Configure the secret ARN to make trust survive redeploys.",
			zap.String("missing_env", "LESSER_HOST_INSTANCE_KEY_ARN"),
		)
	})
}

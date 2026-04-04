package handlers

import (
	"context"
	"errors"
	"net/url"
	"os"
	"strings"

	"go.uber.org/zap"
)

func (h *Handler) instanceTrustConfig(ctx context.Context) map[string]any {
	trust := map[string]any{
		"enabled": false,
	}

	if h == nil || h.cfg == nil {
		return trust
	}

	exists, err := h.repos.Instance().TrustConfigExists(ctx)
	if err != nil {
		h.logger.Warn("failed to check persisted trust config; disabling trust in instance config", zap.Error(err))
		return trust
	}

	var (
		baseURL           string
		trustProxyEnabled bool
	)

	if exists {
		effective, err := h.repos.Instance().EffectiveTrustConfig(ctx)
		if err != nil {
			h.logger.Warn("failed to resolve effective trust config; disabling trust in instance config", zap.Error(err))
			return trust
		}

		baseURL = effective.AttestationsBaseURL
		trustProxyEnabled = effective.TrustProxyEnabled
	} else {
		h.warnLegacyTrustConfig()

		var ok bool
		baseURL, ok = resolveExplicitLesserHostTrustBaseURL()
		if !ok {
			return trust
		}

		key, err := h.cfg.ResolveLesserHostInstanceKey()
		if err != nil {
			h.logger.Warn("failed to resolve trust instance key from config; disabling trust in instance config",
				zap.String("lesser_host_url", baseURL),
				zap.Error(err),
			)
			return trust
		}
		if strings.TrimSpace(key) == "" {
			h.logger.Warn("trust config missing instance key; disabling trust in instance config",
				zap.String("lesser_host_url", baseURL),
			)
			return trust
		}
		trustProxyEnabled = true
	}

	if err := validateTrustBaseURL(baseURL); err != nil {
		h.logger.Warn("trust config invalid; disabling trust in instance config",
			zap.String("lesser_host_url", baseURL),
			zap.Error(err),
		)
		return trust
	}

	if !trustProxyEnabled {
		h.logger.Warn("trust config missing instance key secret ARN; disabling trust in instance config",
			zap.String("lesser_host_url", baseURL),
		)
		return trust
	}

	trust["enabled"] = true
	trust["base_url"] = baseURL
	trust["jwks_url"] = baseURL + "/.well-known/jwks.json"
	trust["attestations_url"] = baseURL + "/attestations"
	trust["proxy"] = map[string]any{
		"jwks_path":                 "/api/v1/trust/jwks.json",
		"attestations_path":         "/api/v1/trust/attestations",
		"attestation_path_template": "/api/v1/trust/attestations/{id}",
	}

	return trust
}

func resolveExplicitLesserHostTrustBaseURL() (string, bool) {
	if base := strings.TrimRight(strings.TrimSpace(os.Getenv("LESSER_HOST_ATTESTATIONS_URL")), "/"); base != "" {
		return base, true
	}
	if base := strings.TrimRight(strings.TrimSpace(os.Getenv("LESSER_HOST_URL")), "/"); base != "" {
		return base, true
	}
	return "", false
}

func validateTrustBaseURL(raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return errors.New("missing base URL")
	}

	parsed, err := url.Parse(raw)
	if err != nil {
		return err
	}
	if parsed.Scheme != schemeHTTPS && parsed.Scheme != schemeHTTP {
		return errors.New("base URL must include http(s) scheme")
	}

	host := strings.ToLower(strings.TrimSpace(parsed.Hostname()))
	if host == "" {
		return errors.New("base URL missing hostname")
	}
	if isLambdaFunctionURLHost(host) {
		return errors.New("lambda function URL hosts are not supported")
	}

	return nil
}

func isLambdaFunctionURLHost(host string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	return strings.Contains(host, ".lambda-url.") && strings.HasSuffix(host, ".on.aws")
}

// Package common provides common utilities and helpers for the Lesser application.
package common

import (
	"context"
	"fmt"

	pkgconfig "github.com/equaltoai/lesser/pkg/config"
)

// ErrFeatureDisabled creates an error indicating a feature is disabled
func ErrFeatureDisabled(feature string) error {
	return fmt.Errorf("feature disabled: %s", feature)
}

// IsModerationMLEnabled checks if ML moderation is enabled for a request.
// It checks both the global flag and optional tenant allow-list.
func IsModerationMLEnabled(_ context.Context, tenantID string) bool {
	cfg := pkgconfig.Get()
	if cfg == nil {
		return false
	}

	// Check if feature is globally disabled
	if !cfg.ModerationMLEnabled {
		return false
	}

	// If no tenant restrictions, allow all
	if len(cfg.ModerationMLTenants) == 0 {
		return true
	}

	// Check if tenant is in allow-list
	for _, allowedTenant := range cfg.ModerationMLTenants {
		if allowedTenant == tenantID {
			return true
		}
	}

	return false
}

// MustCheckModerationMLAccess returns an error if ML moderation is not available.
// Useful for resolver-level validation.
func MustCheckModerationMLAccess(ctx context.Context, tenantID string) error {
	if !IsModerationMLEnabled(ctx, tenantID) {
		return ErrFeatureDisabled("moderation ML is not enabled for this tenant")
	}
	return nil
}

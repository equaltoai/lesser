package common

import (
	"context"
	"testing"

	pkgconfig "github.com/equaltoai/lesser/pkg/config"
	"github.com/stretchr/testify/assert"
)

func TestErrFeatureDisabled(t *testing.T) {
	err := ErrFeatureDisabled("test-feature")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "feature disabled")
	assert.Contains(t, err.Error(), "test-feature")
}

func TestIsModerationMLEnabled_AndAccessCheck(t *testing.T) {
	cfg := pkgconfig.Get()

	previousEnabled := cfg.ModerationMLEnabled
	previousTenants := append([]string(nil), cfg.ModerationMLTenants...)
	t.Cleanup(func() {
		cfg.ModerationMLEnabled = previousEnabled
		cfg.ModerationMLTenants = previousTenants
	})

	// Disabled globally: always false.
	cfg.ModerationMLEnabled = false
	cfg.ModerationMLTenants = nil
	assert.False(t, IsModerationMLEnabled(context.Background(), "tenant-1"))

	// Enabled globally with no tenant allow-list: true for all tenants.
	cfg.ModerationMLEnabled = true
	cfg.ModerationMLTenants = nil
	assert.True(t, IsModerationMLEnabled(context.Background(), "tenant-1"))

	// Tenant allow-list: only allowed tenants get access.
	cfg.ModerationMLTenants = []string{"tenant-2", "tenant-3"}
	assert.False(t, IsModerationMLEnabled(context.Background(), "tenant-1"))
	assert.True(t, IsModerationMLEnabled(context.Background(), "tenant-2"))

	// MustCheckModerationMLAccess follows IsModerationMLEnabled.
	assert.Error(t, MustCheckModerationMLAccess(context.Background(), "tenant-1"))
	assert.NoError(t, MustCheckModerationMLAccess(context.Background(), "tenant-2"))
}

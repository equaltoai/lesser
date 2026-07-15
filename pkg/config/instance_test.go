package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetInstanceConfig_UsesCentralizedConfig(t *testing.T) {
	SetupTestEnvironment(t)
	t.Setenv("VERSION", "v1.5.20")
	ResetForTests()
	t.Cleanup(ResetForTests)

	base := Get()
	cfg := GetInstanceConfig()
	assert.NotNil(t, cfg)
	assert.Equal(t, base.InstanceTitle, cfg.Title)
	assert.Equal(t, "lesser", cfg.Software)
	assert.Equal(t, "v1.5.20", cfg.SoftwareVersion)
	assert.Equal(t, "4.0.0 (compatible; Lesser v1.5.20)", cfg.Version)
	assert.NotContains(t, cfg.Version, "Lesser 0.1.0")
	assert.Greater(t, cfg.MaxStatusChars, 0)
	assert.True(t, cfg.FederationEnabled)
}

func TestMastodonCompatibleVersion_DefaultsToDevNotStaleRelease(t *testing.T) {
	got := MastodonCompatibleVersion("")
	assert.Equal(t, "4.0.0 (compatible; Lesser dev)", got)
	assert.NotContains(t, got, "Lesser 0.1.0")
}

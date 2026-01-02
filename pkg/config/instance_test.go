package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetInstanceConfig_UsesCentralizedConfig(t *testing.T) {
	SetupTestEnvironment(t)

	base := Get()
	cfg := GetInstanceConfig()
	assert.NotNil(t, cfg)
	assert.Equal(t, base.InstanceTitle, cfg.Title)
	assert.Equal(t, "lesser", cfg.Software)
	assert.Greater(t, cfg.MaxStatusChars, 0)
	assert.True(t, cfg.FederationEnabled)
}

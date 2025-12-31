package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFederationInstanceConfigTracking_Defaults_Keys_AndLimits(t *testing.T) {
	t.Run("UpdateKeys sets GSIs and handles optional custom budget", func(t *testing.T) {
		cfg := &FederationInstanceConfigTracking{
			Domain: "example.com",
			Tier:   FederationTierBasic,
		}
		cfg.UpdateKeys()
		assert.Equal(t, "INSTANCE#example.com", cfg.PK)
		assert.Equal(t, SKConfig, cfg.SK)
		assert.Equal(t, "InstanceConfig", cfg.Type)
		assert.Equal(t, "TIER#basic", cfg.GSI1PK)
		assert.Equal(t, "DOMAIN#example.com", cfg.GSI1SK)
		assert.Empty(t, cfg.GSI2PK)
		assert.Empty(t, cfg.GSI2SK)
		assert.Equal(t, MainTableName, cfg.TableName())
		assert.Equal(t, MainTableName, (RetryPolicy{}).TableName())

		custom := 12.34
		cfg.CustomBudgetUSD = &custom
		cfg.UpdateKeys()
		assert.Equal(t, "BUDGET_OVERRIDE", cfg.GSI2PK)
		assert.Contains(t, cfg.GSI2SK, "BUDGET#000012.34#example.com")
	})

	t.Run("BeforeCreate applies secure defaults and performance defaults", func(t *testing.T) {
		cfg := &FederationInstanceConfigTracking{
			Domain: "example.com",
		}

		require.NoError(t, cfg.BeforeCreate())

		assert.Equal(t, FederationTierFree, cfg.Tier)
		assert.InDelta(t, 0.5, cfg.TrustScore, 0.000001)
		assert.InDelta(t, 1.0, cfg.ReputationMultiplier, 0.000001)

		assert.True(t, cfg.EnableSignatureValidation)
		assert.True(t, cfg.EnableRateLimiting)
		assert.True(t, cfg.EnableBudgetEnforcement)

		assert.Equal(t, 3600, cfg.CacheTTLSeconds)
		assert.Equal(t, 1000, cfg.MaxCacheSize)
		assert.Equal(t, 10, cfg.MaxConcurrentRequests)
		assert.Equal(t, 30, cfg.RequestTimeoutSeconds)
		assert.Equal(t, 20, cfg.ConnectionPoolSize)
		assert.Equal(t, 1024, cfg.CompressionThreshold)

		assert.Equal(t, "INSTANCE#example.com", cfg.PK)
		assert.Equal(t, SKConfig, cfg.SK)
	})

	t.Run("BeforeUpdate bumps LastModified", func(t *testing.T) {
		cfg := &FederationInstanceConfigTracking{
			Domain:        "example.com",
			Tier:          FederationTierFree,
			LastModified:  time.Unix(1700000000, 0).UTC(),
			CacheTTLSeconds: 1,
		}

		require.NoError(t, cfg.BeforeUpdate())
		assert.True(t, cfg.LastModified.After(time.Unix(1700000000, 0).UTC()))
		assert.Equal(t, "INSTANCE#example.com", cfg.PK)
	})

	t.Run("GetBudgetLimit respects overrides and tiers", func(t *testing.T) {
		custom := 5.0
		cfg := &FederationInstanceConfigTracking{CustomBudgetUSD: &custom}
		assert.InDelta(t, 5.0, cfg.GetBudgetLimit(), 0.000001)

		assert.InDelta(t, 1.0, (&FederationInstanceConfigTracking{Tier: FederationTierFree}).GetBudgetLimit(), 0.000001)
		assert.InDelta(t, 10.0, (&FederationInstanceConfigTracking{Tier: FederationTierBasic}).GetBudgetLimit(), 0.000001)
		assert.InDelta(t, 100.0, (&FederationInstanceConfigTracking{Tier: FederationTierPremium}).GetBudgetLimit(), 0.000001)
		assert.InDelta(t, 1000.0, (&FederationInstanceConfigTracking{Tier: FederationTierEnterprise}).GetBudgetLimit(), 0.000001)
		assert.InDelta(t, 0.0, (&FederationInstanceConfigTracking{Tier: FederationTierBlocked}).GetBudgetLimit(), 0.000001)
		assert.InDelta(t, 1.0, (&FederationInstanceConfigTracking{Tier: FederationTier("unknown")}).GetBudgetLimit(), 0.000001)
	})

	t.Run("GetRateLimit respects overrides and tiers", func(t *testing.T) {
		override := 42
		cfg := &FederationInstanceConfigTracking{RateLimitOverride: &override}
		assert.Equal(t, 42, cfg.GetRateLimit())

		assert.Equal(t, 100, (&FederationInstanceConfigTracking{Tier: FederationTierFree}).GetRateLimit())
		assert.Equal(t, 1000, (&FederationInstanceConfigTracking{Tier: FederationTierBasic}).GetRateLimit())
		assert.Equal(t, 10000, (&FederationInstanceConfigTracking{Tier: FederationTierPremium}).GetRateLimit())
		assert.Equal(t, 100000, (&FederationInstanceConfigTracking{Tier: FederationTierEnterprise}).GetRateLimit())
		assert.Equal(t, 0, (&FederationInstanceConfigTracking{Tier: FederationTierBlocked}).GetRateLimit())
		assert.Equal(t, 100, (&FederationInstanceConfigTracking{Tier: FederationTier("unknown")}).GetRateLimit())
	})
}


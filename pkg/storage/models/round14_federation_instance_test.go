package models

import (
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/federation/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFederationInstanceRegistry_KeysAndConversion(t *testing.T) {
	t.Run("UpdateKeys sets PK/SK and GSIs", func(t *testing.T) {
		r := &FederationInstanceRegistry{
			Domain:       "example.com",
			Status:       "active",
			TierLevel:    "standard",
			CurrentUsage: 12,
		}
		before := time.Now()
		require.NoError(t, r.UpdateKeys())
		assert.Equal(t, "INSTANCE#example.com", r.PK)
		assert.Equal(t, SKMetadata, r.SK)
		assert.Equal(t, "STATUS#active", r.GSI1PK)
		assert.Equal(t, "DOMAIN#example.com", r.GSI1SK)
		assert.Equal(t, "TIER#standard", r.GSI2PK)
		assert.Equal(t, "USAGE#0000000012", r.GSI2SK)
		assert.True(t, time.Unix(r.TTL, 0).After(before.Add(364*24*time.Hour)))
		assert.Equal(t, MainTableName, r.TableName())
		assert.Equal(t, r.PK, r.GetPK())
		assert.Equal(t, r.SK, r.GetSK())
	})

	t.Run("ToInstance converts supported types and rate limits", func(t *testing.T) {
		ts := time.Unix(1700000000, 0).UTC()
		r := &FederationInstanceRegistry{
			ID:              "id-1",
			Domain:          "example.com",
			InboxURL:        "https://example.com/inbox",
			SharedInboxURL:  "https://example.com/shared",
			PublicKeyPEM:    "pem",
			Status:          string(types.InstanceStatusActive),
			LastSeen:        ts,
			RegisteredAt:    ts,
			AvgResponseTime: 123,
			SuccessRate:     0.9,
			ErrorRate:       0.1,
			TierLevel:       string(types.TierStandard),
			MonthlyQuota:    100,
			CurrentUsage:    10,
			MaxMessageSize:  1234,
			SupportedTypes:  []string{string(types.MessageTypeFollow), string(types.MessageTypeCreate)},
			RateLimits: map[string]interface{}{
				"MessagesPerMinute": float64(1),
				"MessagesPerHour":   float64(2),
				"BytesPerMinute":    float64(3),
				"BytesPerHour":      float64(4),
				"BurstSize":         float64(5),
			},
		}

		inst := r.ToInstance()
		require.NotNil(t, inst)
		assert.Equal(t, "example.com", inst.Domain)
		assert.Equal(t, 123*time.Millisecond, inst.AvgResponseTime)
		assert.Equal(t, []types.MessageType{types.MessageTypeFollow, types.MessageTypeCreate}, inst.SupportedTypes)
		assert.Equal(t, 1, inst.RateLimits.MessagesPerMinute)
		assert.Equal(t, 2, inst.RateLimits.MessagesPerHour)
		assert.Equal(t, int64(3), inst.RateLimits.BytesPerMinute)
		assert.Equal(t, int64(4), inst.RateLimits.BytesPerHour)
		assert.Equal(t, 5, inst.RateLimits.BurstSize)
	})
}

func TestFederationInstanceRegistryHealthHistory_Keys(t *testing.T) {
	ts := time.Unix(1700000000, 0).UTC()
	h := &FederationInstanceRegistryHealthHistory{
		PK:        "INSTANCE#example.com",
		Timestamp: ts,
	}
	before := time.Now()
	require.NoError(t, h.UpdateKeys())
	assert.Equal(t, "INSTANCE#example.com", h.PK)
	assert.Contains(t, h.SK, "HEALTH#")
	assert.True(t, time.Unix(h.TTL, 0).After(before.Add(6*24*time.Hour)))
	assert.Equal(t, MainTableName, h.TableName())
	assert.Equal(t, h.PK, h.GetPK())
	assert.Equal(t, h.SK, h.GetSK())
	assert.Equal(t, "example.com", h.extractInstanceIDFromPK())

	h = &FederationInstanceRegistryHealthHistory{}
	assert.Equal(t, "", h.extractInstanceIDFromPK())
}


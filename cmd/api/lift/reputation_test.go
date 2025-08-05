package lift

import (
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/reputation"
	"github.com/stretchr/testify/assert"
)

func TestConvertReputationToAPI(t *testing.T) {
	now := time.Now()
	rep := &reputation.Reputation{
		ActorID:         "https://example.com/users/testuser",
		InstanceURL:     "https://example.com",
		TotalScore:      800,
		TrustScore:      700,
		ActivityScore:   900,
		ModerationScore: 1000,
		CommunityScore:  600,
		CalculatedAt:    now,
		Version:         "1.0",
		TotalPosts:      100,
		TotalFollowers:  50,
		AccountAge:      30,
		VouchCount:      5,
	}

	result := convertReputationToAPI(rep)

	assert.Equal(t, "https://example.com/users/testuser", result["id"])
	assert.Equal(t, "https://example.com", result["instance"])
	assert.Equal(t, 800, result["total_score"])
	assert.Equal(t, 700, result["trust_score"])
	assert.Equal(t, 900, result["activity_score"])
	assert.Equal(t, 1000, result["moderation_score"])
	assert.Equal(t, 600, result["community_score"])
	assert.Equal(t, now, result["calculated_at"])
	assert.Equal(t, "1.0", result["version"])

	evidence := result["evidence"].(map[string]any)
	assert.Equal(t, 100, evidence["total_posts"])
	assert.Equal(t, 50, evidence["total_followers"])
	assert.Equal(t, 30, evidence["account_age"])
	assert.Equal(t, 5, evidence["vouch_count"])
}

func TestConvertVouchToAPI(t *testing.T) {
	now := time.Now()
	revokedAt := now.Add(time.Hour)
	
	vouch := &reputation.Vouch{
		ID:                 "vouch123",
		From:               "https://example.com/users/testuser",
		To:                 "https://example.com/users/targetuser",
		Confidence:         0.8,
		Context:            "good user",
		CreatedAt:          now,
		ExpiresAt:          now.Add(30 * 24 * time.Hour),
		VoucherReputation:  900,
		Active:             false,
		Revoked:            true,
		RevokedAt:          &revokedAt,
	}

	result := convertVouchToAPI(vouch)

	assert.Equal(t, "vouch123", result["id"])
	assert.Equal(t, "https://example.com/users/testuser", result["from"])
	assert.Equal(t, "https://example.com/users/targetuser", result["to"])
	assert.Equal(t, 0.8, result["confidence"])
	assert.Equal(t, "good user", result["context"])
	assert.Equal(t, now, result["created_at"])
	assert.Equal(t, now.Add(30 * 24 * time.Hour), result["expires_at"])
	assert.Equal(t, 900, result["voucher_reputation"])
	assert.Equal(t, false, result["active"])
	assert.Equal(t, true, result["revoked"])
	assert.Equal(t, &revokedAt, result["revoked_at"])
}

func TestConvertVouchToAPIWithoutRevokedAt(t *testing.T) {
	now := time.Now()
	
	vouch := &reputation.Vouch{
		ID:                 "vouch123",
		From:               "https://example.com/users/testuser",
		To:                 "https://example.com/users/targetuser",
		Confidence:         0.8,
		Context:            "good user",
		CreatedAt:          now,
		ExpiresAt:          now.Add(30 * 24 * time.Hour),
		VoucherReputation:  900,
		Active:             true,
		Revoked:            false,
		RevokedAt:          nil,
	}

	result := convertVouchToAPI(vouch)

	assert.Equal(t, "vouch123", result["id"])
	assert.Equal(t, "https://example.com/users/testuser", result["from"])
	assert.Equal(t, "https://example.com/users/targetuser", result["to"])
	assert.Equal(t, 0.8, result["confidence"])
	assert.Equal(t, "good user", result["context"])
	assert.Equal(t, now, result["created_at"])
	assert.Equal(t, now.Add(30 * 24 * time.Hour), result["expires_at"])
	assert.Equal(t, 900, result["voucher_reputation"])
	assert.Equal(t, true, result["active"])
	assert.Equal(t, false, result["revoked"])
	
	// Should not contain revoked_at when it's nil
	_, hasRevokedAt := result["revoked_at"]
	assert.False(t, hasRevokedAt)
}

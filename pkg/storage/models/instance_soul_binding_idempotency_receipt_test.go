package models

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestInstanceSoulBindingIdempotencyReceipt_ConstructsHashedTTLKeys(t *testing.T) {
	t.Parallel()

	ttl := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	receipt := NewInstanceSoulBindingIdempotencyReceipt(
		" Lesser-Body ",
		" Bind-Key ",
		" sha256:payload ",
		" 0XAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA ",
		" Drone-Ada ",
		ttl,
	)

	require.Equal(t, "lesser-body", receipt.CallerID)
	require.Equal(t, testSoulBindingReceiptHash("Bind-Key"), receipt.IdempotencyKeyHash)
	require.Equal(t, "sha256:payload", receipt.PayloadHash)
	require.Equal(t, "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", receipt.AgentID)
	require.Equal(t, "Drone-Ada", receipt.ActorUsername)
	require.Equal(t, "received", receipt.Status)
	require.Equal(t, ttl.Unix(), receipt.TTL)
	require.False(t, receipt.CreatedAt.IsZero())
	require.False(t, receipt.UpdatedAt.IsZero())
	require.Equal(t, SoulBindingIdempotencyPartitionKey("lesser-body"), receipt.GetPK())
	require.Equal(t, SoulBindingIdempotencySortKey("Bind-Key"), receipt.GetSK())
	require.Equal(t, MainTableName, receipt.TableName())
}

func TestSoulBindingIdempotencyKeyHelpersNormalizeAndHash(t *testing.T) {
	t.Parallel()

	keyHash := testSoulBindingReceiptHash("bind-key")
	callerHash := testSoulBindingReceiptHash("lesser-body")

	require.Equal(t, PKSoulBindingIdempotencyPrefix+callerHash, SoulBindingIdempotencyPartitionKey(" Lesser-Body "))
	require.Equal(t, keyHash, SoulBindingIdempotencyKeyHash(" bind-key "))
	require.Equal(t, SKSoulBindingIdempotencyKeyPrefix+keyHash, SoulBindingIdempotencySortKey(" bind-key "))
	require.Equal(t, SKSoulBindingIdempotencyKeyPrefix+keyHash, SoulBindingIdempotencySortKeyFromHash(" "+strings.ToUpper(keyHash)+" "))
	require.Equal(t, "lesser-body", normalizeSoulBindingReceiptCaller(" Lesser-Body "))
	require.Equal(t, keyHash, normalizeSoulBindingReceiptHash(" "+strings.ToUpper(keyHash)+" "))
	require.Equal(t, testSoulBindingReceiptHash("payload"), hashSoulBindingReceiptValue(" payload "))

	withoutTTL := NewInstanceSoulBindingIdempotencyReceipt(
		"lesser-body",
		"bind-key",
		"sha256:payload",
		"0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"drone-ada",
		time.Time{},
	)
	require.Zero(t, withoutTTL.TTL)
}

func TestInstanceSoulBindingIdempotencyReceipt_UpdateKeysDefaultsAndValidates(t *testing.T) {
	t.Parallel()

	var nilReceipt *InstanceSoulBindingIdempotencyReceipt
	require.Error(t, nilReceipt.UpdateKeys())

	receipt := validSoulBindingIdempotencyReceipt()
	receipt.BodyActorID = " body://ptah/drone-ada "
	receipt.Status = " "
	receipt.BindingState = " bound "
	require.NoError(t, receipt.UpdateKeys())
	require.Equal(t, "received", receipt.Status)
	require.Equal(t, "bound", receipt.BindingState)
	require.Equal(t, "body://ptah/drone-ada", receipt.BodyActorID)
	require.False(t, receipt.CreatedAt.IsZero())
	require.Equal(t, receipt.CreatedAt, receipt.UpdatedAt)
	require.NotEmpty(t, receipt.PK)
	require.NotEmpty(t, receipt.SK)

	testCases := []struct {
		name   string
		mutate func(*InstanceSoulBindingIdempotencyReceipt)
	}{
		{
			name: "caller id is required",
			mutate: func(receipt *InstanceSoulBindingIdempotencyReceipt) {
				receipt.CallerID = " "
			},
		},
		{
			name: "idempotency key hash is required",
			mutate: func(receipt *InstanceSoulBindingIdempotencyReceipt) {
				receipt.IdempotencyKeyHash = " "
			},
		},
		{
			name: "payload hash is required",
			mutate: func(receipt *InstanceSoulBindingIdempotencyReceipt) {
				receipt.PayloadHash = " "
			},
		},
		{
			name: "agent id is required",
			mutate: func(receipt *InstanceSoulBindingIdempotencyReceipt) {
				receipt.AgentID = " "
			},
		},
		{
			name: "actor username is required",
			mutate: func(receipt *InstanceSoulBindingIdempotencyReceipt) {
				receipt.ActorUsername = " "
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			receipt := validSoulBindingIdempotencyReceipt()
			tc.mutate(receipt)
			require.ErrorContains(t, receipt.UpdateKeys(), tc.name)
		})
	}
}

func validSoulBindingIdempotencyReceipt() *InstanceSoulBindingIdempotencyReceipt {
	return &InstanceSoulBindingIdempotencyReceipt{
		CallerID:           "lesser-body",
		IdempotencyKeyHash: SoulBindingIdempotencyKeyHash("bind-key"),
		PayloadHash:        "sha256:payload",
		AgentID:            "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		ActorUsername:      "drone-ada",
	}
}

func testSoulBindingReceiptHash(value string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(value)))
	return hex.EncodeToString(sum[:])
}

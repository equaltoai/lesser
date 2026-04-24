package models

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestInboxProcessingReceipt_UpdateKeys(t *testing.T) {
	now := time.Date(2026, 4, 24, 16, 0, 0, 0, time.UTC)
	receipt := NewInboxProcessingReceipt(
		"https://remote.example/activities/create-1",
		"https://example.com/users/alice",
		"Create",
		now,
	)

	require.NoError(t, receipt.UpdateKeys())
	require.True(t, strings.HasPrefix(receipt.PK, "INBOX_ACTIVITY#"))
	require.True(t, strings.HasPrefix(receipt.SK, "TARGET#"))
	require.NotContains(t, receipt.PK, "https://")
	require.NotContains(t, receipt.SK, "https://")
	require.Equal(t, "https://remote.example/activities/create-1", receipt.ActivityID)
	require.Equal(t, "https://example.com/users/alice", receipt.TargetActorID)
	require.Equal(t, "Create", receipt.ActivityType)
	require.Equal(t, now.Add(30*24*time.Hour).Unix(), receipt.TTL)
}

func TestInboxProcessingReceipt_UpdateKeysRequiresActivityAndTarget(t *testing.T) {
	require.Error(t, (&InboxProcessingReceipt{TargetActorID: "https://example.com/users/alice"}).UpdateKeys())
	require.Error(t, (&InboxProcessingReceipt{ActivityID: "https://remote.example/activities/create-1"}).UpdateKeys())
}

func TestInboxProcessingReceipt_BeforeHooksAndNilKeyAccessors(t *testing.T) {
	var nilReceipt *InboxProcessingReceipt
	require.Empty(t, nilReceipt.GetPK())
	require.Empty(t, nilReceipt.GetSK())

	receipt := &InboxProcessingReceipt{
		ActivityID:    "https://remote.example/activities/create-2",
		TargetActorID: "https://example.com/users/alice",
		ActivityType:  "Undo",
	}
	require.NoError(t, receipt.BeforeCreate())
	require.NotZero(t, receipt.CreatedAt)
	require.NotZero(t, receipt.TTL)
	require.NotEmpty(t, receipt.GetPK())
	require.NotEmpty(t, receipt.GetSK())

	receipt.PK = ""
	receipt.SK = ""
	require.NoError(t, receipt.BeforeUpdate())
	require.NotEmpty(t, receipt.GetPK())
	require.NotEmpty(t, receipt.GetSK())
	require.Equal(t, MainTableName, receipt.TableName())
}

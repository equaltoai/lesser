package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTrusteeConfig_KeysPermissionsAndHelpers(t *testing.T) {
	t.Run("NewTrusteeConfig sets defaults and keys", func(t *testing.T) {
		tc := NewTrusteeConfig("alice", "@bob@remote", "recovery")
		require.NotNil(t, tc)
		assert.Equal(t, "alice", tc.Username)
		assert.Equal(t, "@bob@remote", tc.ActorID)
		assert.Equal(t, "recovery", tc.Category)
		assert.False(t, tc.Confirmed)
		assert.Equal(t, "limited", tc.TrustLevel)
		assert.Equal(t, 999, tc.RecoveryPriority)
		assert.Empty(t, tc.Permissions)
		assert.Equal(t, TrusteeConfigPK, tc.PK)
		assert.Equal(t, "recovery#alice", tc.SK)
		assert.Equal(t, MainTableName, tc.TableName())
	})

	t.Run("Key helpers", func(t *testing.T) {
		pk, sk := GetTrusteeConfigKey("moderation", "alice")
		assert.Equal(t, TrusteeConfigPK, pk)
		assert.Equal(t, "moderation#alice", sk)

		pk, prefix := GetTrusteeConfigsByCategoryKeys("recovery")
		assert.Equal(t, TrusteeConfigPK, pk)
		assert.Equal(t, "recovery#", prefix)
	})

	t.Run("Confirm and RecordUsage update timestamps", func(t *testing.T) {
		tc := &TrusteeConfig{UpdatedAt: time.Unix(0, 0).UTC()}
		tc.Confirm()
		assert.True(t, tc.Confirmed)
		require.NotNil(t, tc.ConfirmedAt)
		assert.True(t, tc.UpdatedAt.After(time.Unix(0, 0).UTC()))

		prev := tc.UpdatedAt
		tc.RecordUsage()
		require.NotNil(t, tc.LastUsed)
		assert.Equal(t, 1, tc.UsageCount)
		assert.True(t, tc.UpdatedAt.After(prev))
	})

	t.Run("CanPerformAction respects trust level and permissions", func(t *testing.T) {
		tc := &TrusteeConfig{TrustLevel: "full"}
		assert.True(t, tc.CanPerformAction("anything"))

		tc = &TrusteeConfig{TrustLevel: "emergency_only"}
		assert.False(t, tc.CanPerformAction("emergency_recovery"))
		assert.False(t, tc.CanPerformAction("normal_recovery"))
		tc.Permissions = []string{"emergency_recovery"}
		assert.True(t, tc.CanPerformAction("emergency_recovery"))

		tc = &TrusteeConfig{TrustLevel: "limited", Permissions: []string{"recover"}}
		assert.True(t, tc.CanPerformAction("recover"))
		assert.False(t, tc.CanPerformAction("other"))

		tc.Permissions = []string{"*"}
		assert.True(t, tc.CanPerformAction("anything"))
	})

	t.Run("Permission list helpers", func(t *testing.T) {
		tc := &TrusteeConfig{Permissions: []string{"a"}}
		assert.False(t, tc.AddPermission("a"))
		assert.True(t, tc.AddPermission("b"))
		assert.ElementsMatch(t, []string{"a", "b"}, tc.Permissions)

		assert.True(t, tc.RemovePermission("a"))
		assert.False(t, tc.RemovePermission("missing"))
		assert.Equal(t, []string{"b"}, tc.Permissions)

		tc.SetPermissions([]string{"x"})
		assert.Equal(t, []string{"x"}, tc.Permissions)
	})

	t.Run("Trust level priority and category helpers", func(t *testing.T) {
		tc := &TrusteeConfig{TrustLevel: "full", Category: "recovery", ActorID: "@a", UpdatedAt: time.Now()}
		assert.Equal(t, 1, tc.GetTrustLevelPriority())
		assert.True(t, tc.IsRecoveryTrustee())
		assert.False(t, tc.IsModerationTrustee())
		assert.Contains(t, tc.FormatDisplayName(), "recovery")

		tc.TrustLevel = "emergency_only"
		assert.Equal(t, 3, tc.GetTrustLevelPriority())

		tc.TrustLevel = "unknown"
		assert.Equal(t, 999, tc.GetTrustLevelPriority())
	})
}

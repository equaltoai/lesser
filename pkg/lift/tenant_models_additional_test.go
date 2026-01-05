package lift

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestTenantAwareModel_Basics(t *testing.T) {
	model := NewTenantAwareModel("t1", "user", "1")
	require.Equal(t, "tenant#t1", model.GetPartitionKey())
	require.Equal(t, "user#1", model.GetSortKey())
	require.True(t, model.ValidateTenant("t1"))
	require.False(t, model.ValidateTenant("t2"))

	before := model.UpdatedAt
	model.UpdateTimestamp()
	require.True(t, model.UpdatedAt.After(before) || model.UpdatedAt.Equal(before))
}

func TestTenantModelFactories(t *testing.T) {
	user := NewTenantUser("t1", "u1")
	require.Equal(t, "u1", user.UserID)
	require.Equal(t, "active", user.Status)

	project := NewTenantProject("t1", "p1", "u1")
	require.Equal(t, "p1", project.ProjectID)
	require.Equal(t, "u1", project.OwnerID)

	cfg := NewTenantConfig("t1")
	require.Equal(t, "free", cfg.Plan)
	require.True(t, cfg.IsActive)
	require.NotNil(t, cfg.Settings)

	// Ensure timestamps are initialized.
	require.WithinDuration(t, time.Now(), cfg.UpdatedAt, time.Second)
}

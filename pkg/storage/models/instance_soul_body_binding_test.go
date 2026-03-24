package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSoulBodyBindingKeyHelpers(t *testing.T) {
	t.Parallel()

	require.Equal(t,
		"SOUL_BODY_BINDING#0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		SoulBodyBindingSortKey(" 0XAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA "),
	)
	require.Equal(t, "", SoulBodyBindingSortKey(" "))
	require.Equal(t, "SOUL_BODY_BINDING_USERNAME#alice", SoulBodyBindingUsernamePartitionKey(" alice "))
	require.Equal(t, "SOUL_BODY_BINDING_USERNAME#medic", SoulBodyBindingUsernamePartitionKey(" Medic "))
	require.Equal(t, "", SoulBodyBindingUsernamePartitionKey(" "))
}

func TestInstanceSoulBodyBinding_UpdateKeysNormalizesFields(t *testing.T) {
	t.Parallel()

	when := time.Date(2026, 3, 8, 12, 0, 0, 0, time.UTC)
	binding := &InstanceSoulBodyBinding{
		AgentID:          " 0XAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA ",
		Username:         " alice ",
		PrincipalAddress: " 0X1111111111111111111111111111111111111111 ",
		BoundAt:          when,
		UpdatedAt:        when,
	}

	require.NoError(t, binding.UpdateKeys())
	require.Equal(t, instanceConfigPK, binding.PK)
	require.Equal(t, "SOUL_BODY_BINDING#0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", binding.SK)
	require.Equal(t, "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", binding.AgentID)
	require.Equal(t, "alice", binding.Username)
	require.Equal(t, "0x1111111111111111111111111111111111111111", binding.PrincipalAddress)
	require.Equal(t, when, binding.BoundAt)
	require.Equal(t, when, binding.UpdatedAt)
	require.Equal(t, instanceConfigPK, binding.GetPK())
	require.Equal(t, binding.SK, binding.GetSK())
	require.Equal(t, MainTableName, binding.TableName())
}

func TestInstanceSoulBodyBindingUsername_UpdateKeysNormalizesFields(t *testing.T) {
	t.Parallel()

	when := time.Date(2026, 3, 8, 12, 0, 0, 0, time.UTC)
	index := &InstanceSoulBodyBindingUsername{
		Username:  " alice ",
		AgentID:   " 0XBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB ",
		UpdatedAt: when,
	}

	require.NoError(t, index.UpdateKeys())
	require.Equal(t, "SOUL_BODY_BINDING_USERNAME#alice", index.PK)
	require.Equal(t, SKSoulBodyBindingUsername, index.SK)
	require.Equal(t, "alice", index.Username)
	require.Equal(t, "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", index.AgentID)
	require.Equal(t, when, index.UpdatedAt)
	require.Equal(t, index.PK, index.GetPK())
	require.Equal(t, index.SK, index.GetSK())
	require.Equal(t, MainTableName, index.TableName())
}

func TestInstanceSoulBodyBinding_ConstructorsPopulateDefaults(t *testing.T) {
	t.Parallel()

	binding := NewInstanceSoulBodyBinding(
		" 0XAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA ",
		" alice ",
		" 0X1111111111111111111111111111111111111111 ",
	)
	require.Equal(t, "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", binding.AgentID)
	require.Equal(t, "alice", binding.Username)
	require.Equal(t, "0x1111111111111111111111111111111111111111", binding.PrincipalAddress)
	require.False(t, binding.BoundAt.IsZero())
	require.False(t, binding.UpdatedAt.IsZero())
	require.Equal(t, instanceConfigPK, binding.GetPK())
	require.NotEmpty(t, binding.GetSK())

	index := NewInstanceSoulBodyBindingUsername(" alice ", " 0XBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB ")
	require.Equal(t, "alice", index.Username)
	require.Equal(t, "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", index.AgentID)
	require.False(t, index.UpdatedAt.IsZero())
	require.Equal(t, "SOUL_BODY_BINDING_USERNAME#alice", index.GetPK())
	require.Equal(t, SKSoulBodyBindingUsername, index.GetSK())
}

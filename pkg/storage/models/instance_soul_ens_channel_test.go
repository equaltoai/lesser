package models

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSoulENSChannelSortKey(t *testing.T) {
	t.Parallel()

	require.Equal(t, "", SoulENSChannelSortKey(""))
	require.Equal(
		t,
		"SOUL_ENS_CHANNEL#0xabc",
		SoulENSChannelSortKey(" 0xAbC "),
	)
}

func TestInstanceSoulENSChannelUpdateKeys(t *testing.T) {
	t.Parallel()

	var nilCfg *InstanceSoulENSChannel
	require.ErrorContains(t, nilCfg.UpdateKeys(), "nil")

	cfg := &InstanceSoulENSChannel{
		AgentID:         " 0XABC ",
		Name:            " Agent.Alice.ETH ",
		ResolverAddress: " 0x000000000000000000000000000000000000cAFe ",
		Chain:           " Sepolia ",
	}
	require.NoError(t, cfg.UpdateKeys())
	require.Equal(t, instanceConfigPK, cfg.GetPK())
	require.Equal(t, "SOUL_ENS_CHANNEL#0xabc", cfg.GetSK())
	require.Equal(t, "0xabc", cfg.AgentID)
	require.Equal(t, "agent.alice.eth", cfg.Name)
	require.Equal(t, "0x000000000000000000000000000000000000cAFe", cfg.ResolverAddress)
	require.Equal(t, "sepolia", cfg.Chain)
}

func TestNewInstanceSoulENSChannel(t *testing.T) {
	t.Parallel()

	cfg := NewInstanceSoulENSChannel(" 0xAbC ")
	require.Equal(t, "0xabc", cfg.AgentID)
	require.Equal(t, SoulENSChainSepolia, cfg.Chain)
	require.Equal(t, instanceConfigPK, cfg.GetPK())
	require.Equal(t, "SOUL_ENS_CHANNEL#0xabc", cfg.GetSK())
	require.False(t, cfg.UpdatedAt.IsZero())
}

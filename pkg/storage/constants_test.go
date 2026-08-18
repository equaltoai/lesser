package storage

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestKeyPrefixes_Format(t *testing.T) {
	require.Equal(t, "USER#alice", fmt.Sprintf(UserKeyPrefix, "alice"))
	require.Equal(t, "ACTOR#alice", fmt.Sprintf(ActorKeyPrefix, "alice"))
	require.Equal(t, "DELIVERY#123", fmt.Sprintf(DeliveryKeyPrefix, "123"))
	require.Equal(t, "WS_COST#abc", fmt.Sprintf(WSCostKey, "abc"))
	require.Equal(t, "WS_BUDGET#alice#day", fmt.Sprintf(WSBudgetKey, "alice", "day"))
}

package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBootstrapRound14_TableNameAndDetermineWalletBranches(t *testing.T) {
	bootstrapTableName = "tbl"
	require.Equal(t, "tbl", (bootstrapInstanceStateRecord{}).TableName())

	wallet, err := determineBootstrapWallet("  ")
	require.NoError(t, err)
	require.NotEmpty(t, wallet.Address)
	require.NotEmpty(t, wallet.Mnemonic)
	require.Equal(t, defaultBootstrapDerivationPath, wallet.DerivationPath)
	require.Equal(t, 1, wallet.ChainID)
}


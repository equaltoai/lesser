package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRunInitAdmin_RejectsReservedWallet(t *testing.T) {
	err := runInitAdmin([]string{
		"--app", "app",
		"--base-domain", "example.com",
		"--aws-profile", "profile",
		"--stage", "dev",
		"--username", "app",
		"--wallet-address", "0x80189edb676d51b2fb2257b2ad38e018b20ca46e",
		"--signature", "0xdeadbeef",
		"--message", "consent",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "reserved")
}


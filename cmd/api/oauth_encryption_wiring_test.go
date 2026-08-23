package main

import (
	"os"
	"testing"

	"github.com/equaltoai/lesser/pkg/config"
	"github.com/stretchr/testify/require"
)

func TestConfigureTableTheoryEncryption(t *testing.T) {
	t.Run("bridges Lesser KMS key id", func(t *testing.T) {
		t.Setenv("KMS_KEY_ARN", "")
		require.NoError(t, configureTableTheoryEncryption(&config.Config{KMSKeyID: "alias/lesser-test"}))
		require.Equal(t, "alias/lesser-test", os.Getenv("KMS_KEY_ARN"))
	})

	t.Run("preserves explicit TableTheory configuration", func(t *testing.T) {
		t.Setenv("KMS_KEY_ARN", "arn:aws:kms:us-east-1:123456789012:key/example")
		require.NoError(t, configureTableTheoryEncryption(&config.Config{KMSKeyID: "alias/ignored"}))
		require.Equal(t, "arn:aws:kms:us-east-1:123456789012:key/example", os.Getenv("KMS_KEY_ARN"))
	})
}

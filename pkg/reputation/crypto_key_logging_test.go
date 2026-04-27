package reputation

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestNewSigner_DoesNotLogGeneratedPrivateKey(t *testing.T) {
	core, observed := observer.New(zap.DebugLevel)
	_, err := NewSigner("", "https://example.com", zap.New(core))
	require.NoError(t, err)

	var sawFingerprint bool
	for _, entry := range observed.All() {
		fields := fmt.Sprint(entry.Context)
		require.NotContains(t, fields, "private_key_pem")
		require.NotContains(t, fields, "PRIVATE KEY")
		if entry.Message == "generated new Ed25519 key pair" && strings.Contains(fields, "public_key_fingerprint") {
			sawFingerprint = true
		}
	}
	require.True(t, sawFingerprint, "expected generated-key log to include a public fingerprint")
}

package handlers

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLegacyConfigWarnings_NilHandlerDoesNotPanic(t *testing.T) {
	t.Parallel()

	require.NotPanics(t, func() { (*Handler)(nil).warnLegacyTrustConfig() })
	require.NotPanics(t, func() { (*Handler)(nil).warnLegacyTranslationConfig() })
	require.NotPanics(t, func() { (*Handler)(nil).warnLegacyTipsConfig() })
	require.NotPanics(t, func() { (*Handler)(nil).warnTrustMigrationSkippedMissingSecretARN() })
}

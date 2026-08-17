package auth

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAuditRequestMetadataHelpers_HandleNilAndEmptyContexts(t *testing.T) {
	t.Parallel()

	ctx := WithAuditRequestMetadata(nil, "", "")
	require.NotNil(t, ctx)

	ipAddress, userAgent := auditRequestMetadataFromContext(ctx)
	require.Empty(t, ipAddress)
	require.Empty(t, userAgent)

	ipAddress, userAgent = auditRequestMetadataFromContext(nil)
	require.Empty(t, ipAddress)
	require.Empty(t, userAgent)
}

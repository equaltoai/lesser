package graph

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGraphAgentGovernanceReaders_DoNotUseRawUserMetadata(t *testing.T) {
	files := []string{
		"agent_governance_helpers.go",
		"agent_resolvers_stubs.go",
		"schema.resolvers.go",
	}

	disallowed := []string{
		`Metadata["quarantine_status"]`,
		`Metadata["delegated_scopes"]`,
		`Metadata["self_scopes"]`,
		`Metadata["self_sovereign"]`,
		`Metadata["verified"]`,
		`Metadata["verified_at"]`,
		`Metadata["verified_by"]`,
		`Metadata["unverified_at"]`,
		`Metadata["unverified_by"]`,
	}

	for _, path := range files {
		data, err := os.ReadFile(path)
		require.NoError(t, err)
		for _, snippet := range disallowed {
			require.NotContainsf(t, string(data), snippet, "%s should use typed governance accessors", path)
		}
	}
}

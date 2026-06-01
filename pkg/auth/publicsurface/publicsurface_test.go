package publicsurface

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestContractAuthOverrides(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		route string
		want  ContractAuthClass
	}{
		{
			name:  "setup admin uses setup bearer",
			route: "/setup/admin",
			want:  ContractAuthSetupBearer,
		},
		{
			name:  "setup finalize preserves oauth bearer contract",
			route: "/setup/finalize",
			want:  ContractAuthBearerRequired,
		},
		{
			name:  "notification delivery is internal only",
			route: "/api/v1/notifications/deliver",
			want:  ContractAuthInternalOnly,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, ok := ContractAuth(http.MethodPost, tc.route)
			require.True(t, ok)
			require.Equal(t, tc.want, got)
			require.True(t, IsPublic(http.MethodPost, tc.route), "contract metadata must not change gate reachability")
		})
	}
}

func TestContractAuthDoesNotClassifyNormalPublicRoutes(t *testing.T) {
	t.Parallel()

	_, ok := ContractAuth(http.MethodGet, "/api/v1/custom_emojis")
	require.False(t, ok)
	require.True(t, IsPublic(http.MethodGet, "/api/v1/custom_emojis"))
}

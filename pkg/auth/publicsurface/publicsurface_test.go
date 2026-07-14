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

func TestAgentRegistrationAndAuthProofRoutesArePublic(t *testing.T) {
	t.Parallel()

	tests := []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/api/v1/agents/register/challenge"},
		{http.MethodPost, "/api/v1/agents/register"},
		{http.MethodPost, "/api/v1/agents/auth/challenge"},
		{http.MethodPost, "/api/v1/agents/auth/token"},
		{http.MethodPost, "/api/v1/agents/della-marlowe/access-leases/lease-1/session-key/challenge"},
		{http.MethodPost, "/api/v1/agents/della-marlowe/access-leases/lease-1/session-key"},
		{http.MethodPost, "/api/v1/agents/della-marlowe/access-leases/lease-1/renew/challenge"},
		{http.MethodPost, "/api/v1/agents/della-marlowe/access-leases/lease-1/token"},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.path, func(t *testing.T) {
			t.Parallel()

			require.True(t, IsPublic(tc.method, tc.path))
			require.Equal(t, ClassificationAnonymous, Classify(tc.method, tc.path).Kind)
		})
	}
}

func TestPublicProfileRoutesDoNotOpenPrivateSiblings(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		path string
		want bool
	}{
		{"account profile", "/api/v1/accounts/silas-vane", true},
		{"account lookup", "/api/v1/accounts/lookup", true},
		{"account relationships", "/api/v1/accounts/relationships", false},
		{"account verify credentials", "/api/v1/accounts/verify_credentials", false},
		{"account pinned statuses", "/api/v1/accounts/silas-vane/pinned_statuses", false},
		{"agent directory", "/api/v1/agents", true},
		{"agent profile", "/api/v1/agents/della-marlowe", true},
		{"agent activity", "/api/v1/agents/della-marlowe/activity", false},
		{"agent memory", "/api/v1/agents/memory/search", false},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tc.want, IsPublic(http.MethodGet, tc.path))
		})
	}
}

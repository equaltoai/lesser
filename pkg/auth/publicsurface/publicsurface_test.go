package publicsurface

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestContractAuthOverrides(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		method string
		route  string
		want   ContractAuthClass
	}{
		{
			name:   "setup admin uses setup bearer",
			method: http.MethodPost,
			route:  "/setup/admin",
			want:   ContractAuthSetupBearer,
		},
		{
			name:   "setup finalize preserves oauth bearer contract",
			method: http.MethodPost,
			route:  "/setup/finalize",
			want:   ContractAuthBearerRequired,
		},
		{
			name:   "notification delivery is internal only",
			method: http.MethodPost,
			route:  "/api/v1/notifications/deliver",
			want:   ContractAuthInternalOnly,
		},
		{
			name:   "soul binding write uses dedicated integration bearer",
			method: http.MethodPost,
			route:  "/api/v1/souls/bindings",
			want:   ContractAuthSoulBindingIntegration,
		},
		{
			name:   "soul binding read uses dedicated integration bearer",
			method: http.MethodGet,
			route:  "/api/v1/souls/bindings/{agentId}",
			want:   ContractAuthSoulBindingIntegration,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, ok := ContractAuth(tc.method, tc.route)
			require.True(t, ok)
			require.Equal(t, tc.want, got)
			require.True(t, IsPublic(tc.method, tc.route), "contract metadata must not change gate reachability")
		})
	}
}

func TestContractAuthDoesNotClassifyNormalPublicRoutes(t *testing.T) {
	t.Parallel()

	_, ok := ContractAuth(http.MethodGet, "/api/v1/custom_emojis")
	require.False(t, ok)
	require.True(t, IsPublic(http.MethodGet, "/api/v1/custom_emojis"))
}

func TestSoulBindingRoutesAreGateReachableButNotOrdinaryPublicSiblings(t *testing.T) {
	t.Parallel()

	require.True(t, IsPublic(http.MethodPost, "/api/v1/souls/bindings"))
	require.True(t, IsPublic(http.MethodGet, "/api/v1/souls/bindings/0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"))

	require.False(t, IsPublic(http.MethodPost, "/api/v1/souls/bindings/0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"))
	require.False(t, IsPublic(http.MethodGet, "/api/v1/souls/bindings/0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa/audit"))
	require.False(t, IsPublic(http.MethodGet, "/api/v1/souls/private"))
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

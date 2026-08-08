package limits

import (
	"context"
	"testing"

	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/stretchr/testify/require"
)

func TestRequestDepthLimit(t *testing.T) {
	tests := []struct {
		name       string
		configured int
		claims     *auth.Claims
		want       int
	}{
		{name: "anonymous uses configured limit", configured: 12, want: 12},
		{name: "human uses configured limit", configured: 12, claims: &auth.Claims{}, want: 12},
		{name: "agent receives bounded profile", configured: 12, claims: &auth.Claims{IsAgent: true}, want: AutomationMaxDepth},
		{name: "cli receives bounded profile", configured: 12, claims: &auth.Claims{ClientClass: "CLI"}, want: AutomationMaxDepth},
		{name: "agent remains bounded when general limit disabled", claims: &auth.Claims{IsAgent: true}, want: AutomationMaxDepth},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			if tt.claims != nil {
				ctx = context.WithValue(ctx, common.ContextKeyClaims, tt.claims)
			}
			require.Equal(t, tt.want, RequestDepthLimit(ctx, tt.configured))
		})
	}
}

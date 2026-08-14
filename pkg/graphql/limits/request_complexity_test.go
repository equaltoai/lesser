package limits

import (
	"context"
	"testing"

	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/stretchr/testify/require"
)

func TestRequestComplexityLimit(t *testing.T) {
	tests := []struct {
		name       string
		configured int
		automation int
		claims     *auth.Claims
		want       int
	}{
		{name: "anonymous uses configured limit", configured: 500, automation: 4000, want: 500},
		{name: "human uses configured limit", configured: 500, automation: 4000, claims: &auth.Claims{}, want: 500},
		{name: "agent receives bounded profile", configured: 500, automation: 4000, claims: &auth.Claims{IsAgent: true}, want: 4000},
		{name: "cli receives bounded profile", configured: 500, automation: 4000, claims: &auth.Claims{ClientClass: "CLI"}, want: 4000},
		{name: "agent remains bounded when general limit disabled", configured: 0, automation: 4000, claims: &auth.Claims{IsAgent: true}, want: 4000},
		{name: "agent profile is independent of human limit", configured: 8000, automation: 4000, claims: &auth.Claims{IsAgent: true}, want: 4000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			if tt.claims != nil {
				ctx = context.WithValue(ctx, common.ContextKeyClaims, tt.claims)
			}
			require.Equal(t, tt.want, RequestComplexityLimit(ctx, tt.configured, tt.automation))
		})
	}
}

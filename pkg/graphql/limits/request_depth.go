package limits

import (
	"context"
	"strings"

	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
)

// RequestDepthLimit resolves the depth limit for the authenticated caller.
// Human and anonymous callers retain the operator-configured limit; automation
// callers retain a bounded, independently-configurable limit even when the
// general limit is disabled. Depth is a coarse structural bound: query
// complexity, parser-token, and pagination limits remain the primary breadth
// and resource-consumption controls.
func RequestDepthLimit(ctx context.Context, configuredDepth int, automationDepth int) int {
	if isAutomationCaller(ctx) {
		return automationDepth
	}
	return configuredDepth
}

// isAutomationCaller reports whether the authenticated caller is an agent or a
// CLI automation token.
func isAutomationCaller(ctx context.Context) bool {
	claims, ok := ctx.Value(common.ContextKeyClaims).(*auth.Claims)
	return ok && claims != nil && (claims.IsAgent || strings.EqualFold(claims.ClientClass, auth.ClientClassCLI))
}

package limits

import (
	"context"
	"strings"

	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
)

// AutomationMaxDepth is the GraphQL selection-depth ceiling for agent and CLI
// automation tokens. Query complexity, parser-token, and pagination limits
// remain the primary breadth and resource-consumption controls.
const AutomationMaxDepth = 4

// RequestDepthLimit resolves the depth limit for the authenticated caller.
// Human and anonymous callers retain the operator-configured limit; automation
// callers retain a bounded limit even when the general limit is disabled.
func RequestDepthLimit(ctx context.Context, configuredDepth int) int {
	claims, ok := ctx.Value(common.ContextKeyClaims).(*auth.Claims)
	if ok && claims != nil && (claims.IsAgent || strings.EqualFold(claims.ClientClass, auth.ClientClassCLI)) {
		return AutomationMaxDepth
	}
	return configuredDepth
}

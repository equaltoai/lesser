package limits

import "context"

// RequestComplexityLimit resolves the complexity limit for the authenticated
// caller. Human and anonymous callers retain the operator-configured limit;
// automation callers retain a bounded, independently-configurable limit even
// when the general limit is disabled. Complexity remains the primary
// resource-consumption control and is intentionally per-profile.
func RequestComplexityLimit(ctx context.Context, configuredComplexity int, automationComplexity int) int {
	if isAutomationCaller(ctx) {
		return automationComplexity
	}
	return configuredComplexity
}

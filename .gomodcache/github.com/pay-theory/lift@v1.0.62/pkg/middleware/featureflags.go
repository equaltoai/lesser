package middleware

import (
	"github.com/pay-theory/lift/pkg/features"
	"github.com/pay-theory/lift/pkg/lift"
)

// FeatureFlagMiddleware injects feature flags into the request context
func FeatureFlagMiddleware(ff *features.FeatureFlags) lift.Middleware {
	return func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			// Add feature flags instance to context
			ctx.Set("feature_flags", ff)

			// Add helper method for checking flags
			ctx.Set("is_feature_enabled", func(flag string) bool {
				return ff.IsEnabled(flag)
			})

			// Add all current flags for debugging (only in dev mode)
			if ff.IsEnabled(features.DebugLoggingEnabled) {
				ctx.Set("all_feature_flags", ff.GetAllFlags())
			}

			return next.Handle(ctx)
		})
	}
}

// GetFeatureFlags retrieves the feature flags from context
func GetFeatureFlags(ctx *lift.Context) *features.FeatureFlags {
	if ff, ok := ctx.Get("feature_flags").(*features.FeatureFlags); ok {
		return ff
	}
	return nil
}

// IsFeatureEnabled checks if a feature is enabled from context
func IsFeatureEnabled(ctx *lift.Context, flag string) bool {
	ff := GetFeatureFlags(ctx)
	if ff == nil {
		// Return safe default if feature flags not in context
		return features.IsEnabled(flag)
	}
	return ff.IsEnabled(flag)
}

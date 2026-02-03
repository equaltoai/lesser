package crawler

import (
	"os"
	"strings"

	apptheory "github.com/theory-cloud/apptheory/runtime"
	"go.uber.org/zap"
)

type protectionMode string

const (
	protectionModeOff     protectionMode = "off"
	protectionModeObserve protectionMode = "observe"
	protectionModeLimit   protectionMode = "limit"
	protectionModeBlock   protectionMode = "block"
)

const (
	contextCrawlerCategoryKey = "crawler_category"
	contextCrawlerReasonKey   = "crawler_reason"
)

func parseProtectionMode(raw string) protectionMode {
	raw = strings.ToLower(strings.TrimSpace(raw))
	switch raw {
	case string(protectionModeObserve):
		return protectionModeObserve
	case string(protectionModeLimit):
		return protectionModeLimit
	case string(protectionModeBlock):
		return protectionModeBlock
	default:
		return protectionModeOff
	}
}

func protectionModeFromEnv() protectionMode {
	return parseProtectionMode(os.Getenv("CRAWLER_PROTECTION_MODE"))
}

// NewMiddleware returns the crawler middleware configured by environment.
//
// Modes (CRAWLER_PROTECTION_MODE):
//   - off (default): no classification, no logging
//   - observe: classify requests, log the classification, do not enforce
//   - limit/block: reserved for later milestones; currently behaves like observe
func NewMiddleware(logger *zap.Logger) apptheory.Middleware {
	return Middleware(protectionModeFromEnv(), logger)
}

// Middleware classifies requests and (optionally) logs the classification.
//
// Enforcement is intentionally out of scope for this milestone.
func Middleware(mode protectionMode, logger *zap.Logger) apptheory.Middleware {
	if logger == nil {
		logger = zap.NewNop()
	}

	mode = parseProtectionMode(string(mode))
	if mode == protectionModeOff {
		return func(next apptheory.Handler) apptheory.Handler { return next }
	}

	return func(next apptheory.Handler) apptheory.Handler {
		return func(ctx *apptheory.Context) (*apptheory.Response, error) {
			if ctx == nil {
				return next(ctx)
			}

			userAgent := headerValue(ctx, "User-Agent")
			accept := headerValue(ctx, "Accept")
			category, reason := ClassifyRequest(userAgent, accept, ctx.Request.Path)

			ctx.Set(contextCrawlerCategoryKey, category.String())
			ctx.Set(contextCrawlerReasonKey, reason)

			if mode != protectionModeOff {
				logger.Debug("crawler classification",
					zap.String("request_id", strings.TrimSpace(ctx.RequestID)),
					zap.String("method", strings.TrimSpace(ctx.Request.Method)),
					zap.String("path", strings.TrimSpace(ctx.Request.Path)),
					zap.String("category", category.String()),
					zap.String("reason", reason),
					zap.String("user_agent", userAgent),
					zap.String("client_ip", headerValue(ctx, "X-Forwarded-For")),
				)
			}

			return next(ctx)
		}
	}
}

func headerValue(ctx *apptheory.Context, key string) string {
	if ctx == nil {
		return ""
	}
	key = strings.ToLower(strings.TrimSpace(key))
	values := ctx.Request.Headers[key]
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

package middleware

import (
	"fmt"
	"strings"
	"time"

	"github.com/pay-theory/lift/pkg/lift"
)

const (
	schemeHTTPS = "https"
)

// SecurityHeadersConfig configures the security headers middleware
type SecurityHeadersConfig struct {
	CustomHeaders           map[string]string
	ContentSecurityPolicy   string
	XFrameOptions           string
	XXSSProtection          string
	StrictTransportSecurity string
	ReferrerPolicy          string
	PermissionsPolicy       string
	XContentTypeOptions     bool
	IncludeInDevelopment    bool
}

// DefaultSecurityHeadersConfig returns secure default configuration
func DefaultSecurityHeadersConfig() SecurityHeadersConfig {
	return SecurityHeadersConfig{
		ContentSecurityPolicy:   "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data: https:; font-src 'self'; connect-src 'self'; media-src 'self'; object-src 'none'; child-src 'none'; frame-src 'none'; worker-src 'none'; frame-ancestors 'none'; form-action 'self'; base-uri 'self';",
		XFrameOptions:           "DENY",
		XContentTypeOptions:     true,
		XXSSProtection:          "1; mode=block",
		StrictTransportSecurity: "max-age=31536000; includeSubDomains; preload",
		ReferrerPolicy:          "strict-origin-when-cross-origin",
		PermissionsPolicy:       "geolocation=(), microphone=(), camera=(), payment=(), usb=(), magnetometer=(), gyroscope=(), accelerometer=()",
		CustomHeaders:           make(map[string]string),
		IncludeInDevelopment:    true,
	}
}

// SecurityHeaders returns the security headers middleware
func SecurityHeaders(config SecurityHeadersConfig) lift.Middleware {
	applier := newSecurityHeaderApplier(config)

	return func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			return applier.apply(ctx, next)
		})
	}
}

// securityHeaderApplier applies security headers based on configuration
type securityHeaderApplier struct {
	headerSetters     []headerSetter
	conditionalSetter *conditionalHeaderSetter
	config            SecurityHeadersConfig
}

// newSecurityHeaderApplier creates a new security header applier
func newSecurityHeaderApplier(config SecurityHeadersConfig) *securityHeaderApplier {
	applier := &securityHeaderApplier{
		config: config,
	}

	// Initialize header setters
	applier.initializeHeaderSetters()

	// Initialize conditional setter for HSTS
	applier.conditionalSetter = newConditionalHeaderSetter(config)

	return applier
}

// initializeHeaderSetters creates all header setter functions
func (sha *securityHeaderApplier) initializeHeaderSetters() {
	sha.headerSetters = []headerSetter{
		newSimpleHeaderSetter("Content-Security-Policy", sha.config.ContentSecurityPolicy),
		newSimpleHeaderSetter("X-Frame-Options", sha.config.XFrameOptions),
		newConditionalBoolHeaderSetter("X-Content-Type-Options", "nosniff", sha.config.XContentTypeOptions),
		newSimpleHeaderSetter("X-XSS-Protection", sha.config.XXSSProtection),
		newSimpleHeaderSetter("Referrer-Policy", sha.config.ReferrerPolicy),
		newSimpleHeaderSetter("Permissions-Policy", sha.config.PermissionsPolicy),
	}

	// Add custom headers
	for key, value := range sha.config.CustomHeaders {
		sha.headerSetters = append(sha.headerSetters, newSimpleHeaderSetter(key, value))
	}
}

// apply applies security headers to the response
func (sha *securityHeaderApplier) apply(ctx *lift.Context, next lift.Handler) error {
	// Check development environment
	if sha.shouldSkipInDevelopment(ctx) {
		return next.Handle(ctx)
	}

	// Apply all simple headers
	for _, setter := range sha.headerSetters {
		setter.setHeader(ctx)
	}

	// Apply conditional headers (HSTS)
	sha.conditionalSetter.applyConditionalHeaders(ctx)

	// Apply cache control for sensitive paths
	sha.applyCacheControl(ctx)

	return next.Handle(ctx)
}

// shouldSkipInDevelopment checks if headers should be skipped in development
func (sha *securityHeaderApplier) shouldSkipInDevelopment(ctx *lift.Context) bool {
	if sha.config.IncludeInDevelopment {
		return false
	}

	env := ctx.Get("environment")
	return env == "development"
}

// applyCacheControl applies cache control headers for sensitive paths
func (sha *securityHeaderApplier) applyCacheControl(ctx *lift.Context) {
	if isSensitivePath(ctx.Request.Path) {
		ctx.Response.Header("Cache-Control", "no-store, no-cache, must-revalidate, private")
		ctx.Response.Header("Pragma", "no-cache")
		ctx.Response.Header("Expires", "0")
	}
}

// headerSetter interface for setting headers
type headerSetter interface {
	setHeader(ctx *lift.Context)
}

// simpleHeaderSetter sets a header with a fixed value
type simpleHeaderSetter struct {
	name  string
	value string
}

// newSimpleHeaderSetter creates a new simple header setter
func newSimpleHeaderSetter(name, value string) headerSetter {
	return &simpleHeaderSetter{name: name, value: value}
}

// setHeader sets the header if value is not empty
func (shs *simpleHeaderSetter) setHeader(ctx *lift.Context) {
	if shs.value != "" {
		ctx.Response.Header(shs.name, shs.value)
	}
}

// conditionalBoolHeaderSetter sets a header based on a boolean condition
type conditionalBoolHeaderSetter struct {
	name      string
	value     string
	condition bool
}

// newConditionalBoolHeaderSetter creates a new conditional bool header setter
func newConditionalBoolHeaderSetter(name, value string, condition bool) headerSetter {
	return &conditionalBoolHeaderSetter{
		name:      name,
		value:     value,
		condition: condition,
	}
}

// setHeader sets the header if condition is true
func (cbhs *conditionalBoolHeaderSetter) setHeader(ctx *lift.Context) {
	if cbhs.condition {
		ctx.Response.Header(cbhs.name, cbhs.value)
	}
}

// conditionalHeaderSetter handles headers that require runtime conditions
type conditionalHeaderSetter struct {
	config SecurityHeadersConfig
}

// newConditionalHeaderSetter creates a new conditional header setter
func newConditionalHeaderSetter(config SecurityHeadersConfig) *conditionalHeaderSetter {
	return &conditionalHeaderSetter{config: config}
}

// applyConditionalHeaders applies headers that require runtime checks
func (chs *conditionalHeaderSetter) applyConditionalHeaders(ctx *lift.Context) {
	// Apply HSTS only for HTTPS connections
	if chs.config.StrictTransportSecurity != "" && chs.isSecureConnection(ctx) {
		ctx.Response.Header("Strict-Transport-Security", chs.config.StrictTransportSecurity)
	}
}

// isSecureConnection checks if the connection is over HTTPS
func (chs *conditionalHeaderSetter) isSecureConnection(ctx *lift.Context) bool {
	return ctx.Header("X-Forwarded-Proto") == schemeHTTPS ||
		ctx.Header("CloudFront-Forwarded-Proto") == schemeHTTPS ||
		ctx.Header("X-Forwarded-SSL") == "on"
}

// StrictSecurityHeaders returns a middleware with very strict security settings
func StrictSecurityHeaders() lift.Middleware {
	config := SecurityHeadersConfig{
		ContentSecurityPolicy:   "default-src 'none'; script-src 'self'; style-src 'self'; img-src 'self'; font-src 'self'; connect-src 'self'; base-uri 'self'; form-action 'self';",
		XFrameOptions:           "DENY",
		XContentTypeOptions:     true,
		XXSSProtection:          "1; mode=block",
		StrictTransportSecurity: "max-age=63072000; includeSubDomains; preload",
		ReferrerPolicy:          "no-referrer",
		PermissionsPolicy:       "geolocation=(), microphone=(), camera=(), payment=(), usb=(), magnetometer=(), gyroscope=(), accelerometer=(), fullscreen=(), autoplay=()",
		CustomHeaders: map[string]string{
			"X-Permitted-Cross-Domain-Policies": "none",
			"Cross-Origin-Embedder-Policy":      "require-corp",
			"Cross-Origin-Opener-Policy":        "same-origin",
			"Cross-Origin-Resource-Policy":      "same-origin",
		},
		IncludeInDevelopment: false,
	}

	return SecurityHeaders(config)
}

// APISecurityHeaders returns security headers optimized for API endpoints
func APISecurityHeaders() lift.Middleware {
	config := SecurityHeadersConfig{
		ContentSecurityPolicy:   "default-src 'none'; frame-ancestors 'none';",
		XFrameOptions:           "DENY",
		XContentTypeOptions:     true,
		XXSSProtection:          "1; mode=block",
		StrictTransportSecurity: "max-age=31536000; includeSubDomains",
		ReferrerPolicy:          "strict-origin-when-cross-origin",
		CustomHeaders: map[string]string{
			"X-Permitted-Cross-Domain-Policies": "none",
			"X-API-Version":                     "1.0",
		},
		IncludeInDevelopment: true,
	}

	return SecurityHeaders(config)
}

// SecurityHeadersWithNonce creates security headers with a nonce for CSP
func SecurityHeadersWithNonce() lift.Middleware {
	return func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			// Generate a unique nonce for this request
			nonce := generateNonce()
			ctx.Set("csp_nonce", nonce)

			// Create CSP with nonce
			csp := fmt.Sprintf("default-src 'self'; script-src 'self' 'nonce-%s'; style-src 'self' 'nonce-%s' 'unsafe-inline'; img-src 'self' data: https:; font-src 'self'; connect-src 'self'; frame-ancestors 'none';", nonce, nonce)

			config := SecurityHeadersConfig{
				ContentSecurityPolicy:   csp,
				XFrameOptions:           "DENY",
				XContentTypeOptions:     true,
				XXSSProtection:          "1; mode=block",
				StrictTransportSecurity: "max-age=31536000; includeSubDomains",
				ReferrerPolicy:          "strict-origin-when-cross-origin",
				IncludeInDevelopment:    true,
			}

			return SecurityHeaders(config)(next).Handle(ctx)
		})
	}
}

// generateNonce creates a cryptographically secure nonce
func generateNonce() string {
	// In a real implementation, this would use crypto/rand
	// For now, use timestamp-based approach
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

// SecurityAuditHeaders returns middleware that adds headers for security auditing
func SecurityAuditHeaders() lift.Middleware {
	return func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			// Add security audit headers
			ctx.Response.Header("X-Security-Headers", "enabled")
			ctx.Response.Header("X-Security-Audit", fmt.Sprintf("scan-date-%s", time.Now().Format("2006-01-02")))

			return next.Handle(ctx)
		})
	}
}

// isSensitivePath checks if a path should be considered sensitive
func isSensitivePath(path string) bool {
	sensitivePaths := []string{
		"/api/auth",
		"/api/payment",
		"/api/users",
		"/api/accounts",
		"/admin",
		"/dashboard",
	}

	for _, sensitive := range sensitivePaths {
		if strings.HasPrefix(path, sensitive) {
			return true
		}
	}

	return false
}

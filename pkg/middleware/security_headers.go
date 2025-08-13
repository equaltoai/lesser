// Package middleware provides HTTP middleware for the Lesser application
package middleware

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"
)

// SecurityHeadersConfig defines configuration for security headers
type SecurityHeadersConfig struct {
	// Content Security Policy
	EnableCSP          bool
	CSPDirectives      map[string][]string
	CSPReportOnly      bool
	CSPReportURI       string
	
	// Strict Transport Security
	EnableHSTS         bool
	HSTSMaxAge         int // in seconds
	HSTSIncludeSubDomains bool
	HSTSPreload        bool
	
	// Frame Options
	XFrameOptions      string // DENY, SAMEORIGIN, or ALLOW-FROM uri
	
	// Content Type Options
	XContentTypeOptions string // nosniff
	
	// XSS Protection (for older browsers)
	XXSSProtection     string // 1; mode=block
	
	// Referrer Policy
	ReferrerPolicy     string // no-referrer, origin, strict-origin-when-cross-origin, etc.
	
	// Permissions Policy (formerly Feature Policy)
	PermissionsPolicy  map[string][]string
	
	// Cross-Origin Policies
	CrossOriginOpenerPolicy   string // same-origin, same-origin-allow-popups, unsafe-none
	CrossOriginEmbedderPolicy string // require-corp, credentialless, unsafe-none
	CrossOriginResourcePolicy string // same-origin, same-site, cross-origin
	
	// Custom headers
	CustomHeaders      map[string]string
	
	// Nonce generation for inline scripts/styles
	GenerateNonce      bool
	
	// Development mode (less strict policies)
	DevelopmentMode    bool
}

// EnhancedSecurityHeaders provides security headers middleware
type EnhancedSecurityHeaders struct {
	config *SecurityHeadersConfig
	logger *zap.Logger
}

// NewEnhancedSecurityHeaders creates a new security headers middleware
func NewEnhancedSecurityHeaders(config *SecurityHeadersConfig, logger *zap.Logger) *EnhancedSecurityHeaders {
	if config == nil {
		config = DefaultSecurityHeadersConfig()
	}
	
	return &EnhancedSecurityHeaders{
		config: config,
		logger: logger,
	}
}

// DefaultSecurityHeadersConfig returns default security headers configuration
func DefaultSecurityHeadersConfig() *SecurityHeadersConfig {
	return &SecurityHeadersConfig{
		EnableCSP:     true,
		CSPDirectives: map[string][]string{
			"default-src": {"'self'"},
			"script-src":  {"'self'", "'unsafe-inline'", "https://cdn.jsdelivr.net"},
			"style-src":   {"'self'", "'unsafe-inline'", "https://fonts.googleapis.com"},
			"img-src":     {"'self'", "data:", "https:", "blob:"},
			"font-src":    {"'self'", "data:", "https://fonts.gstatic.com"},
			"connect-src": {"'self'", "wss:", "https:"},
			"media-src":   {"'self'", "https:", "blob:"},
			"object-src":  {"'none'"},
			"frame-src":   {"'self'"},
			"base-uri":    {"'self'"},
			"form-action": {"'self'"},
			"frame-ancestors": {"'none'"},
			"upgrade-insecure-requests": {},
		},
		CSPReportOnly:         false,
		CSPReportURI:          "/api/v1/csp-report",
		EnableHSTS:            true,
		HSTSMaxAge:            31536000, // 1 year
		HSTSIncludeSubDomains: true,
		HSTSPreload:           false,
		XFrameOptions:         "DENY",
		XContentTypeOptions:   "nosniff",
		XXSSProtection:        "1; mode=block",
		ReferrerPolicy:        "strict-origin-when-cross-origin",
		PermissionsPolicy: map[string][]string{
			"accelerometer":     {"()"},
			"camera":            {"()"},
			"geolocation":       {"()"},
			"gyroscope":         {"()"},
			"magnetometer":      {"()"},
			"microphone":        {"()"},
			"payment":           {"()"},
			"usb":               {"()"},
			"interest-cohort":   {"()"}, // Disable FLoC
		},
		CrossOriginOpenerPolicy:   "same-origin",
		CrossOriginEmbedderPolicy: "require-corp",
		CrossOriginResourcePolicy: "same-origin",
		GenerateNonce:             true,
		DevelopmentMode:           false,
	}
}

// DevelopmentSecurityHeadersConfig returns a more permissive configuration for development
func DevelopmentSecurityHeadersConfig() *SecurityHeadersConfig {
	config := DefaultSecurityHeadersConfig()
	config.DevelopmentMode = true
	
	// Relax CSP for development
	config.CSPDirectives["script-src"] = append(config.CSPDirectives["script-src"], "'unsafe-eval'")
	config.CSPDirectives["connect-src"] = append(config.CSPDirectives["connect-src"], "ws://localhost:*", "http://localhost:*")
	
	// Disable HSTS in development
	config.EnableHSTS = false
	
	// Allow framing from same origin in development
	config.XFrameOptions = "SAMEORIGIN"
	
	// Less strict CORS policies
	config.CrossOriginEmbedderPolicy = "unsafe-none"
	config.CrossOriginResourcePolicy = "cross-origin"
	
	return config
}

// Middleware returns the security headers middleware for Lift
func (sh *EnhancedSecurityHeaders) Middleware() func(lift.HandlerFunc) lift.HandlerFunc {
	return func(next lift.HandlerFunc) lift.HandlerFunc {
		return func(ctx *lift.Context) error {
			// Generate nonce if configured
			var nonce string
			if sh.config.GenerateNonce {
				nonce = sh.generateNonce()
				ctx.Set("csp-nonce", nonce)
			}
			
			// Set security headers
			sh.setSecurityHeaders(ctx, nonce)
			
			// Continue to next handler
			return next(ctx)
		}
	}
}

// setSecurityHeaders sets all configured security headers
func (sh *EnhancedSecurityHeaders) setSecurityHeaders(ctx *lift.Context, nonce string) {
	// Content Security Policy
	if sh.config.EnableCSP {
		csp := sh.buildCSP(nonce)
		if sh.config.CSPReportOnly {
			ctx.Set("Content-Security-Policy-Report-Only", csp)
		} else {
			ctx.Set("Content-Security-Policy", csp)
		}
	}
	
	// Strict Transport Security
	if sh.config.EnableHSTS {
		hsts := fmt.Sprintf("max-age=%d", sh.config.HSTSMaxAge)
		if sh.config.HSTSIncludeSubDomains {
			hsts += "; includeSubDomains"
		}
		if sh.config.HSTSPreload {
			hsts += "; preload"
		}
		ctx.Set("Strict-Transport-Security", hsts)
	}
	
	// X-Frame-Options
	if sh.config.XFrameOptions != "" {
		ctx.Set("X-Frame-Options", sh.config.XFrameOptions)
	}
	
	// X-Content-Type-Options
	if sh.config.XContentTypeOptions != "" {
		ctx.Set("X-Content-Type-Options", sh.config.XContentTypeOptions)
	}
	
	// X-XSS-Protection (for older browsers)
	if sh.config.XXSSProtection != "" {
		ctx.Set("X-XSS-Protection", sh.config.XXSSProtection)
	}
	
	// Referrer Policy
	if sh.config.ReferrerPolicy != "" {
		ctx.Set("Referrer-Policy", sh.config.ReferrerPolicy)
	}
	
	// Permissions Policy
	if len(sh.config.PermissionsPolicy) > 0 {
		pp := sh.buildPermissionsPolicy()
		ctx.Set("Permissions-Policy", pp)
	}
	
	// Cross-Origin Policies
	if sh.config.CrossOriginOpenerPolicy != "" {
		ctx.Set("Cross-Origin-Opener-Policy", sh.config.CrossOriginOpenerPolicy)
	}
	
	if sh.config.CrossOriginEmbedderPolicy != "" {
		ctx.Set("Cross-Origin-Embedder-Policy", sh.config.CrossOriginEmbedderPolicy)
	}
	
	if sh.config.CrossOriginResourcePolicy != "" {
		ctx.Set("Cross-Origin-Resource-Policy", sh.config.CrossOriginResourcePolicy)
	}
	
	// Custom headers
	for key, value := range sh.config.CustomHeaders {
		ctx.Set(key, value)
	}
	
	// Remove potentially dangerous headers
	sh.removeUnsafeHeaders(ctx)
}

// buildCSP builds the Content Security Policy string
func (sh *EnhancedSecurityHeaders) buildCSP(nonce string) string {
	var directives []string
	
	for directive, sources := range sh.config.CSPDirectives {
		// Add nonce to script-src and style-src if generated
		if nonce != "" && (directive == "script-src" || directive == "style-src") {
			sources = append(sources, fmt.Sprintf("'nonce-%s'", nonce))
		}
		
		if len(sources) == 0 {
			directives = append(directives, directive)
		} else {
			directives = append(directives, fmt.Sprintf("%s %s", directive, strings.Join(sources, " ")))
		}
	}
	
	// Add report-uri if configured
	if sh.config.CSPReportURI != "" {
		directives = append(directives, fmt.Sprintf("report-uri %s", sh.config.CSPReportURI))
	}
	
	return strings.Join(directives, "; ")
}

// buildPermissionsPolicy builds the Permissions Policy string
func (sh *EnhancedSecurityHeaders) buildPermissionsPolicy() string {
	var policies []string
	
	for feature, allowList := range sh.config.PermissionsPolicy {
		if len(allowList) == 0 {
			policies = append(policies, fmt.Sprintf("%s=()", feature))
		} else {
			policies = append(policies, fmt.Sprintf("%s=(%s)", feature, strings.Join(allowList, " ")))
		}
	}
	
	return strings.Join(policies, ", ")
}

// generateNonce generates a random nonce for CSP
func (sh *EnhancedSecurityHeaders) generateNonce() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		sh.logger.Error("failed to generate nonce", zap.Error(err))
		return ""
	}
	return base64.StdEncoding.EncodeToString(b)
}

// removeUnsafeHeaders removes potentially dangerous headers
func (sh *EnhancedSecurityHeaders) removeUnsafeHeaders(ctx *lift.Context) {
	// Remove headers that might leak information
	ctx.Set("X-Powered-By", "")
	ctx.Set("Server", "")
	ctx.Set("X-AspNet-Version", "")
	ctx.Set("X-AspNetMvc-Version", "")
}

// CSPReportHandler handles CSP violation reports
func (sh *EnhancedSecurityHeaders) CSPReportHandler() lift.HandlerFunc {
	return func(ctx *lift.Context) error {
		// Parse CSP report
		var report CSPReport
		if err := ctx.JSON(&report); err != nil {
			sh.logger.Warn("failed to parse CSP report", zap.Error(err))
			return ctx.Status(400).JSON(map[string]string{"error": "invalid report"})
		}
		
		// Log the violation
		sh.logger.Warn("CSP violation reported",
			zap.String("document_uri", report.CSPReport.DocumentURI),
			zap.String("violated_directive", report.CSPReport.ViolatedDirective),
			zap.String("blocked_uri", report.CSPReport.BlockedURI),
			zap.String("source_file", report.CSPReport.SourceFile),
			zap.Int("line_number", report.CSPReport.LineNumber),
			zap.Int("column_number", report.CSPReport.ColumnNumber))
		
		// You could store these reports in a database for analysis
		
		ctx.Status(204)
		return nil // No Content
	}
}

// CSPReport represents a CSP violation report
type CSPReport struct {
	CSPReport struct {
		DocumentURI        string `json:"document-uri"`
		Referrer          string `json:"referrer"`
		ViolatedDirective string `json:"violated-directive"`
		EffectiveDirective string `json:"effective-directive"`
		OriginalPolicy    string `json:"original-policy"`
		BlockedURI        string `json:"blocked-uri"`
		SourceFile        string `json:"source-file"`
		LineNumber        int    `json:"line-number"`
		ColumnNumber      int    `json:"column-number"`
		StatusCode        int    `json:"status-code"`
	} `json:"csp-report"`
}

// APISecurityHeaders returns security headers specifically for API endpoints
func APISecurityHeaders() *SecurityHeadersConfig {
	return &SecurityHeadersConfig{
		EnableCSP: true,
		CSPDirectives: map[string][]string{
			"default-src": {"'none'"},
			"frame-ancestors": {"'none'"},
		},
		EnableHSTS:            true,
		HSTSMaxAge:            31536000,
		HSTSIncludeSubDomains: true,
		XFrameOptions:         "DENY",
		XContentTypeOptions:   "nosniff",
		ReferrerPolicy:        "no-referrer",
		CrossOriginResourcePolicy: "same-origin",
		CustomHeaders: map[string]string{
			"X-API-Version": "1.0",
			"Cache-Control": "no-store, no-cache, must-revalidate, private",
		},
	}
}

// MediaSecurityHeaders returns security headers for media/asset endpoints
func MediaSecurityHeaders() *SecurityHeadersConfig {
	return &SecurityHeadersConfig{
		EnableCSP: true,
		CSPDirectives: map[string][]string{
			"default-src": {"'none'"},
			"img-src":     {"'self'"},
			"media-src":   {"'self'"},
			"style-src":   {"'none'"},
			"script-src":  {"'none'"},
		},
		XContentTypeOptions: "nosniff",
		CustomHeaders: map[string]string{
			"Cache-Control": "public, max-age=31536000, immutable",
			"Accept-Ranges": "bytes",
		},
	}
}

// WebSocketSecurityHeaders returns security headers for WebSocket endpoints
func WebSocketSecurityHeaders() *SecurityHeadersConfig {
	return &SecurityHeadersConfig{
		EnableCSP: true,
		CSPDirectives: map[string][]string{
			"default-src":  {"'self'"},
			"connect-src":  {"'self'", "wss:"},
			"frame-ancestors": {"'none'"},
		},
		XFrameOptions:       "DENY",
		XContentTypeOptions: "nosniff",
		CustomHeaders: map[string]string{
			"X-WebSocket-Protocol": "v1",
		},
	}
}

// ActivityPubSecurityHeaders returns security headers for ActivityPub endpoints
func ActivityPubSecurityHeaders() *SecurityHeadersConfig {
	return &SecurityHeadersConfig{
		EnableCSP: false, // CSP can interfere with federation
		XContentTypeOptions: "nosniff",
		ReferrerPolicy: "strict-origin-when-cross-origin",
		CrossOriginResourcePolicy: "cross-origin", // Allow federation
		CustomHeaders: map[string]string{
			"X-Robots-Tag": "noindex, nofollow", // Don't index federation endpoints
		},
	}
}
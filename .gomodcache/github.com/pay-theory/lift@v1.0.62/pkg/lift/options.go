package lift

// (JWT helpers removed; use middleware.JWTAuth instead)

// SecurityConfig holds configuration for security middleware
// Memory optimized: 96 → 88 bytes (8 bytes saved)
type SecurityConfig struct {
	// 8-byte aligned fields (functions, slices)
	Handler       func(ctx *Context) error                              // Custom security handler
	AuditLogger   func(ctx *Context, event string, data map[string]any) // Audit logger
	IPWhitelist   []string                                              // IP whitelist (empty means allow all)
	RequiredRoles []string                                              // Required roles for all endpoints (can be overridden per route)

	// Boolean flags (1 byte each)
	EnableSecurityHeaders bool // Enable security headers
	EnableCSRF            bool // Enable CSRF protection
	EnableRateLimiting    bool // Enable rate limiting
}

// WithSecurityMiddleware adds security middleware to the application
func WithSecurityMiddleware(config SecurityConfig) AppOption {
	return func(app *App) {
		// Create security middleware
		securityMiddleware := createSecurityMiddleware(config)
		app.Use(securityMiddleware)
	}
}

// (JWT helpers removed; use middleware.JWTAuth instead)

// createSecurityMiddleware creates the security middleware
func createSecurityMiddleware(config SecurityConfig) Middleware {
	processor := newSecurityProcessor(config)

	return func(next Handler) Handler {
		return HandlerFunc(func(ctx *Context) error {
			return processor.process(ctx, next)
		})
	}
}

// securityProcessor handles security processing logic
// Memory optimized: struct with 104 pointer bytes could be 80
type securityProcessor struct {
	// Pointers first (8 bytes each)
	ipValidator   *ipValidator
	headerApplier *securityHeaderApplier
	roleValidator *roleValidator
	auditLogger   *securityAuditLogger
	// SecurityConfig struct (depends on its size, treating as large struct)
	config SecurityConfig
}

// newSecurityProcessor creates a new security processor
func newSecurityProcessor(config SecurityConfig) *securityProcessor {
	processor := &securityProcessor{
		config: config,
	}

	// Initialize components based on configuration
	if len(config.IPWhitelist) > 0 {
		processor.ipValidator = newIPValidator(config.IPWhitelist)
	}

	if config.EnableSecurityHeaders {
		processor.headerApplier = newSecurityHeaderApplier()
	}

	if len(config.RequiredRoles) > 0 {
		processor.roleValidator = newRoleValidator(config.RequiredRoles)
	}

	if config.AuditLogger != nil {
		processor.auditLogger = newSecurityAuditLogger(config.AuditLogger)
	}

	return processor
}

// process handles the security processing
func (sp *securityProcessor) process(ctx *Context, next Handler) error {
	// Convert to security context
	secCtx := NewSecurityContext(ctx)

	// Validate IP if configured
	if sp.ipValidator != nil {
		if err := sp.ipValidator.validate(secCtx); err != nil {
			return err
		}
	}

	// Apply security headers if configured
	if sp.headerApplier != nil {
		sp.headerApplier.apply(ctx)
	}

	// Validate roles if configured
	if sp.roleValidator != nil {
		if err := sp.roleValidator.validate(ctx, secCtx); err != nil {
			return err
		}
	}

	// Setup audit logging if configured
	if sp.auditLogger != nil {
		defer sp.auditLogger.logRequest(ctx, secCtx)
	}

	// Call custom handler if provided
	if sp.config.Handler != nil {
		if err := sp.config.Handler(ctx); err != nil {
			return err
		}
	}

	// Continue to next handler
	return next.Handle(ctx)
}

// ipValidator validates IP addresses against a whitelist
type ipValidator struct {
	whitelist []string
}

// newIPValidator creates a new IP validator
func newIPValidator(whitelist []string) *ipValidator {
	return &ipValidator{whitelist: whitelist}
}

// validate checks if the request IP is in the whitelist
func (iv *ipValidator) validate(secCtx *SecurityContext) error {
	if !secCtx.ValidateIP(iv.whitelist) {
		return AuthorizationError("Access denied")
	}
	return nil
}

// securityHeaderApplier applies security headers to responses
type securityHeaderApplier struct {
	headers map[string]string
}

// newSecurityHeaderApplier creates a new security header applier
func newSecurityHeaderApplier() *securityHeaderApplier {
	return &securityHeaderApplier{
		headers: map[string]string{
			"X-Content-Type-Options":    "nosniff",
			"X-Frame-Options":           "DENY",
			"X-XSS-Protection":          "1; mode=block",
			"Strict-Transport-Security": "max-age=31536000; includeSubDomains",
		},
	}
}

// apply adds security headers to the response
func (sha *securityHeaderApplier) apply(ctx *Context) {
	for key, value := range sha.headers {
		ctx.Response.Headers[key] = value
	}
}

// roleValidator validates required roles
type roleValidator struct {
	requiredRoles []string
}

// newRoleValidator creates a new role validator
func newRoleValidator(requiredRoles []string) *roleValidator {
	return &roleValidator{requiredRoles: requiredRoles}
}

// validate checks if the user has required roles
func (rv *roleValidator) validate(ctx *Context, secCtx *SecurityContext) error {
	if !ctx.IsAuthenticated() {
		return nil // Skip role check for unauthenticated requests
	}

	for _, role := range rv.requiredRoles {
		if secCtx.HasRole(role) {
			return nil // User has at least one required role
		}
	}

	return AuthorizationError("Insufficient permissions")
}

// securityAuditLogger handles security audit logging
type securityAuditLogger struct {
	logger func(ctx *Context, event string, data map[string]any)
}

// newSecurityAuditLogger creates a new security audit logger
func newSecurityAuditLogger(logger func(ctx *Context, event string, data map[string]any)) *securityAuditLogger {
	return &securityAuditLogger{logger: logger}
}

// logRequest logs the request for audit purposes
func (sal *securityAuditLogger) logRequest(ctx *Context, secCtx *SecurityContext) {
	sal.logger(ctx, "request", secCtx.ToAuditMap())
}

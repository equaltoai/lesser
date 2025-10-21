package context

import (
	"context"

	"github.com/pay-theory/lift/pkg/lift"
	"github.com/pay-theory/lift/pkg/observability"
)

// Context wraps lift.Context with additional convenience methods
type Context struct {
	*lift.Context
}

// NewContext creates a new context wrapper
func NewContext(ctx *lift.Context) *Context {
	return &Context{Context: ctx}
}

// GoContext returns the underlying Go context
func (c *Context) GoContext() context.Context {
	return c.Context.Context
}

// TenantID returns the tenant ID from the context
func (c *Context) TenantID() string {
	// Delegate to the embedded lift.Context
	return c.Context.TenantID()
}

// UserID returns the user ID from the context
func (c *Context) UserID() string {
	// Delegate to the embedded lift.Context
	return c.Context.UserID()
}

// Logger returns a structured logger
func (c *Context) Logger() observability.StructuredLogger {
	// Return a default logger if none is set
	// This would typically be injected via middleware
	if logger, ok := c.Get("logger").(observability.StructuredLogger); ok {
		return logger
	}
	// Return a no-op logger as fallback
	return &noOpLogger{}
}

// PathParam returns a path parameter value
func (c *Context) PathParam(key string) string {
	return c.Param(key)
}

// QueryParam returns a query parameter value with optional default
func (c *Context) QueryParam(key string, defaultValue ...string) string {
	value := c.Query(key)
	if value == "" && len(defaultValue) > 0 {
		return defaultValue[0]
	}
	return value
}

// ParseJSON parses the request body as JSON
func (c *Context) ParseJSON(target any) error {
	return c.ParseRequest(target)
}

// noOpLogger is a fallback logger that does nothing
type noOpLogger struct{}

func (l *noOpLogger) Debug(_ string, _ ...map[string]any) {}
func (l *noOpLogger) Info(_ string, _ ...map[string]any)  {}
func (l *noOpLogger) Warn(_ string, _ ...map[string]any)  {}
func (l *noOpLogger) Error(_ string, _ ...map[string]any) {}
func (l *noOpLogger) Fatal(_ string, _ ...map[string]any) {}

// lift.Logger interface methods
func (l *noOpLogger) WithField(_ string, _ any) lift.Logger   { return l }
func (l *noOpLogger) WithFields(_ map[string]any) lift.Logger { return l }

// StructuredLogger interface methods
func (l *noOpLogger) WithRequestID(_ string) observability.StructuredLogger { return l }
func (l *noOpLogger) WithTenantID(_ string) observability.StructuredLogger  { return l }
func (l *noOpLogger) WithUserID(_ string) observability.StructuredLogger    { return l }
func (l *noOpLogger) WithTraceID(_ string) observability.StructuredLogger   { return l }
func (l *noOpLogger) WithSpanID(_ string) observability.StructuredLogger    { return l }

func (l *noOpLogger) Flush(_ context.Context) error       { return nil }
func (l *noOpLogger) Close() error                        { return nil }
func (l *noOpLogger) IsHealthy() bool                     { return true }
func (l *noOpLogger) GetStats() observability.LoggerStats { return observability.LoggerStats{} }

package auth

import "context"

type auditRequestContextKey struct{}

type auditRequestMetadata struct {
	ipAddress string
	userAgent string
}

// WithAuditRequestMetadata stores request-derived audit metadata on the context so
// service-layer audit emitters can reuse handler-collected device information
// without widening every method signature.
func WithAuditRequestMetadata(ctx context.Context, ipAddress, userAgent string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if ipAddress == "" && userAgent == "" {
		return ctx
	}
	return context.WithValue(ctx, auditRequestContextKey{}, auditRequestMetadata{
		ipAddress: ipAddress,
		userAgent: userAgent,
	})
}

func auditRequestMetadataFromContext(ctx context.Context) (ipAddress, userAgent string) {
	if ctx == nil {
		return "", ""
	}
	metadata, ok := ctx.Value(auditRequestContextKey{}).(auditRequestMetadata)
	if !ok {
		return "", ""
	}
	return metadata.ipAddress, metadata.userAgent
}

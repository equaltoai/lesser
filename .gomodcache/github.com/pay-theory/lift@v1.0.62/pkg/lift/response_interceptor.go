package lift

// ResponseInterceptor is implemented by middleware that needs to access response data
// after handler execution. When middleware implements this interface and returns true
// from NeedsResponseInterception(), the framework will automatically enable response
// buffering for that request.
type ResponseInterceptor interface {
	NeedsResponseInterception() bool
}

// InterceptingMiddleware wraps a middleware function to indicate it needs response interception
type InterceptingMiddleware struct {
	middleware Middleware
}

// NewInterceptingMiddleware creates a new InterceptingMiddleware
func NewInterceptingMiddleware(middleware Middleware) *InterceptingMiddleware {
	return &InterceptingMiddleware{
		middleware: middleware,
	}
}

// NeedsResponseInterception returns true to indicate this middleware needs response buffering
func (im *InterceptingMiddleware) NeedsResponseInterception() bool {
	return true
}

// Apply returns the wrapped middleware function
func (im *InterceptingMiddleware) Apply() Middleware {
	return im.middleware
}

package lift

// Handler represents a request handler
type Handler interface {
	Handle(ctx *Context) error
}

// HandlerFunc is an adapter to allow the use of ordinary functions as Handlers
type HandlerFunc func(ctx *Context) error

// Handle calls f(ctx)
func (f HandlerFunc) Handle(ctx *Context) error {
	return f(ctx)
}

// Middleware represents a middleware function that wraps a Handler
type Middleware func(Handler) Handler

// TypedHandler is a generic handler that provides type safety for request/response
type TypedHandler[Req, Resp any] interface {
	Handle(ctx *Context, req Req) (Resp, error)
}

// TypedHandlerFunc is an adapter for typed handler functions
type TypedHandlerFunc[Req, Resp any] func(ctx *Context, req Req) (Resp, error)

// Handle implements TypedHandler
func (f TypedHandlerFunc[Req, Resp]) Handle(ctx *Context, req Req) (Resp, error) {
	return f(ctx, req)
}

// SimpleHandler creates a Handler from a typed handler function
// This is the main convenience function for creating type-safe handlers
func SimpleHandler[Req, Resp any](handler func(ctx *Context, req Req) (Resp, error)) Handler {
	return &typedHandlerAdapter[Req, Resp]{
		handler: TypedHandlerFunc[Req, Resp](handler),
	}
}

// typedHandlerAdapter adapts a TypedHandler to the Handler interface
type typedHandlerAdapter[Req, Resp any] struct {
	handler TypedHandler[Req, Resp]
}

// Handle adapts the typed handler to the generic Handler interface
func (adapter *typedHandlerAdapter[Req, Resp]) Handle(ctx *Context) error {
	// Parse the request into the expected type
	var req Req
	if err := ctx.ParseRequest(&req); err != nil {
		return err
	}

	// Call the typed handler
	resp, err := adapter.handler.Handle(ctx, req)
	if err != nil {
		return err
	}

	// Set the response as JSON
	return ctx.JSON(resp)
}

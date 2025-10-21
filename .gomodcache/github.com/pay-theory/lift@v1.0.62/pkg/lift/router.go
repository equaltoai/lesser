package lift

import (
	"fmt"
	"strings"
)

// Router handles route matching and middleware execution
type Router struct {
	// Exact route matches
	routes map[string]map[string]Handler // method -> path -> handler

	// Parameter routes (e.g., /users/:id)
	paramRoutes map[string][]*paramRoute // method -> []paramRoute

	// Global middleware
	middleware []Middleware
}

// paramRoute represents a route with parameters
type paramRoute struct {
	pattern string // e.g., "/users/:id/posts/:postId"
	handler Handler
	params  []string // e.g., ["id", "postId"]
}

// NewRouter creates a new router instance
func NewRouter() *Router {
	return &Router{
		routes:      make(map[string]map[string]Handler),
		paramRoutes: make(map[string][]*paramRoute),
		middleware:  make([]Middleware, 0),
	}
}

// AddRoute adds a route to the router
func (r *Router) AddRoute(method, path string, handler Handler) {
	// Check if path contains parameters
	if strings.Contains(path, ":") {
		r.addParamRoute(method, path, handler)
	} else {
		r.addExactRoute(method, path, handler)
	}
}

// addExactRoute adds a route without parameters
func (r *Router) addExactRoute(method, path string, handler Handler) {
	if r.routes[method] == nil {
		r.routes[method] = make(map[string]Handler)
	}
	if _, exists := r.routes[method][path]; exists {
		panic(fmt.Sprintf("duplicate route registration: %s %s", method, path))
	}
	r.routes[method][path] = handler
}

// addParamRoute adds a route with parameters
func (r *Router) addParamRoute(method, path string, handler Handler) {
	params := extractParams(path)
	route := &paramRoute{
		pattern: path,
		handler: handler,
		params:  params,
	}
	// Prevent duplicate parameter route patterns
	for _, existing := range r.paramRoutes[method] {
		if existing.pattern == path {
			panic(fmt.Sprintf("duplicate parameter route registration: %s %s", method, path))
		}
	}
	r.paramRoutes[method] = append(r.paramRoutes[method], route)
}

// SetMiddleware sets the global middleware stack
func (r *Router) SetMiddleware(middleware []Middleware) {
	r.middleware = middleware
}

// Handle processes a request through the router
func (r *Router) Handle(ctx *Context) error {
	method := ctx.Request.Method
	path := ctx.Request.Path

	// Find the handler
	handler, params := r.findHandler(method, path)
	if handler == nil {
		// Return a structured 404 error for unmatched routes
		return NotFound(fmt.Sprintf("route not found: %s %s", method, path))
	}

	// Set path parameters in context
	for key, value := range params {
		ctx.SetParam(key, value)
	}

	// Apply middleware chain
	finalHandler := handler
	for i := len(r.middleware) - 1; i >= 0; i-- {
		finalHandler = r.middleware[i](finalHandler)
	}

	// Execute the final handler
	return finalHandler.Handle(ctx)
}

// findHandler finds a handler for the given method and path
func (r *Router) findHandler(method, path string) (Handler, map[string]string) {
	// Try exact match first
	if methodRoutes, exists := r.routes[method]; exists {
		if handler, exists := methodRoutes[path]; exists {
			return handler, nil
		}
	}

	// Try parameter matching
	if paramRoutes, exists := r.paramRoutes[method]; exists {
		for _, route := range paramRoutes {
			if params := matchPattern(route.pattern, path); params != nil {
				return route.handler, params
			}
		}
	}

	return nil, nil
}

// extractParams extracts parameter names from a route pattern. Parameters are
// denoted by a leading ':' (e.g., /users/:id/posts/:postId yields [id, postId]).
func extractParams(pattern string) []string {
	parts := strings.Split(pattern, "/")
	var params []string

	for _, part := range parts {
		if strings.HasPrefix(part, ":") {
			params = append(params, part[1:]) // Remove the ":"
		}
	}

	return params
}

// matchPattern checks whether a concrete path matches a route pattern and, if
// so, returns a map of parameter values keyed by parameter name.
func matchPattern(pattern, path string) map[string]string {
	patternParts := strings.Split(pattern, "/")
	pathParts := strings.Split(path, "/")

	// Must have same number of parts
	if len(patternParts) != len(pathParts) {
		return nil
	}

	params := make(map[string]string)

	for i, patternPart := range patternParts {
		pathPart := pathParts[i]

		if strings.HasPrefix(patternPart, ":") {
			// This is a parameter
			paramName := patternPart[1:] // Remove the ":"
			params[paramName] = pathPart
		} else if patternPart != pathPart {
			// This must be an exact match
			return nil
		}
	}

	return params
}

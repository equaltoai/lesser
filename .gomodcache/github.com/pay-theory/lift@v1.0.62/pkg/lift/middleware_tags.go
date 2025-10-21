package lift

import (
	"reflect"
	"sync"
)

var globalMiddlewareRegistry sync.Map

// MarkGlobalMiddleware marks the provided middleware as applicable to all trigger types.
// It returns the same middleware for fluent usage.
func MarkGlobalMiddleware(m Middleware) Middleware {
	if m == nil {
		return m
	}
	ptr := reflect.ValueOf(m).Pointer()
	globalMiddlewareRegistry.Store(ptr, true)
	return m
}

func middlewareAppliesToEvents(m Middleware) bool {
	if m == nil {
		return false
	}
	if val, ok := globalMiddlewareRegistry.Load(reflect.ValueOf(m).Pointer()); ok {
		if applies, ok := val.(bool); ok {
			return applies
		}
	}
	return false
}

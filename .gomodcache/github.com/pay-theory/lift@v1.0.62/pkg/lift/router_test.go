package lift

import (
	"context"
	"testing"

	"github.com/pay-theory/lift/pkg/lift/adapters"
)

func TestNewRouter(t *testing.T) {
	router := NewRouter()
	if router == nil {
		t.Fatal("NewRouter() returned nil")
	}

	if router.routes == nil {
		t.Error("Router routes map is nil")
	}

	if router.paramRoutes == nil {
		t.Error("Router paramRoutes map is nil")
	}
}

func TestRouterAddRoute(t *testing.T) {
	router := NewRouter()
	handler := HandlerFunc(func(_ *Context) error {
		return nil
	})

	// Test exact route
	router.AddRoute("GET", "/users", handler)

	// Test parameter route
	router.AddRoute("GET", "/users/:id", handler)
	router.AddRoute("GET", "/users/:id/posts/:postId", handler)

	// Verify exact routes
	if len(router.routes["GET"]) != 1 {
		t.Errorf("Expected 1 exact route, got %d", len(router.routes["GET"]))
	}

	// Verify parameter routes
	if len(router.paramRoutes["GET"]) != 2 {
		t.Errorf("Expected 2 parameter routes, got %d", len(router.paramRoutes["GET"]))
	}
}

func TestExtractParams(t *testing.T) {
	tests := []struct {
		pattern  string
		expected []string
	}{
		{"/users/:id", []string{"id"}},
		{"/users/:id/posts/:postId", []string{"id", "postId"}},
		{"/static/path", []string{}},
		{"/:category/:subcategory/:item", []string{"category", "subcategory", "item"}},
	}

	for _, test := range tests {
		result := extractParams(test.pattern)
		if len(result) != len(test.expected) {
			t.Errorf("Pattern %s: expected %d params, got %d", test.pattern, len(test.expected), len(result))
			continue
		}

		for i, expected := range test.expected {
			if result[i] != expected {
				t.Errorf("Pattern %s: expected param %s, got %s", test.pattern, expected, result[i])
			}
		}
	}
}

func TestMatchPattern(t *testing.T) {
	tests := []struct {
		expected    map[string]string
		pattern     string
		path        string
		shouldMatch bool
	}{
		{
			pattern:     "/users/:id",
			path:        "/users/123",
			expected:    map[string]string{"id": "123"},
			shouldMatch: true,
		},
		{
			pattern:     "/users/:id/posts/:postId",
			path:        "/users/123/posts/456",
			expected:    map[string]string{"id": "123", "postId": "456"},
			shouldMatch: true,
		},
		{
			pattern:     "/users/:id",
			path:        "/users/123/extra",
			expected:    nil,
			shouldMatch: false,
		},
		{
			pattern:     "/static/path",
			path:        "/static/path",
			expected:    map[string]string{},
			shouldMatch: true,
		},
		{
			pattern:     "/static/path",
			path:        "/different/path",
			expected:    nil,
			shouldMatch: false,
		},
	}

	for _, test := range tests {
		result := matchPattern(test.pattern, test.path)

		if test.shouldMatch {
			if result == nil {
				t.Errorf("Pattern %s should match path %s", test.pattern, test.path)
				continue
			}

			if len(result) != len(test.expected) {
				t.Errorf("Pattern %s with path %s: expected %d params, got %d",
					test.pattern, test.path, len(test.expected), len(result))
				continue
			}

			for key, expectedValue := range test.expected {
				if actualValue, exists := result[key]; !exists || actualValue != expectedValue {
					t.Errorf("Pattern %s with path %s: expected param %s=%s, got %s=%s",
						test.pattern, test.path, key, expectedValue, key, actualValue)
				}
			}
		} else if result != nil {
			t.Errorf("Pattern %s should not match path %s", test.pattern, test.path)
		}
	}
}

func TestRouterFindHandler(t *testing.T) {
	router := NewRouter()

	exactHandler := HandlerFunc(func(ctx *Context) error {
		return ctx.JSON(map[string]string{"type": "exact"})
	})

	paramHandler := HandlerFunc(func(ctx *Context) error {
		return ctx.JSON(map[string]string{"type": "param"})
	})

	// Add routes
	router.AddRoute("GET", "/exact", exactHandler)
	router.AddRoute("GET", "/users/:id", paramHandler)

	// Test exact match
	handler, params := router.findHandler("GET", "/exact")
	if handler == nil {
		t.Error("Should find exact handler")
	}
	if len(params) > 0 {
		t.Error("Exact match should not have parameters")
	}

	// Test parameter match
	handler, params = router.findHandler("GET", "/users/123")
	if handler == nil {
		t.Error("Should find parameter handler")
	}
	if params == nil || params["id"] != "123" {
		t.Error("Parameter match should extract id=123")
	}

	// Test no match
	handler, _ = router.findHandler("GET", "/nonexistent")
	if handler != nil {
		t.Error("Should not find handler for nonexistent route")
	}
}

func TestRouterHandle(t *testing.T) {
	router := NewRouter()

	// Add test handler
	router.AddRoute("GET", "/test/:id", HandlerFunc(func(ctx *Context) error {
		id := ctx.Param("id")
		return ctx.JSON(map[string]string{"id": id})
	}))

	// Create test context
	req := NewRequest(&adapters.Request{
		Method:      "GET",
		Path:        "/test/123",
		Headers:     make(map[string]string),
		QueryParams: make(map[string]string),
		PathParams:  make(map[string]string),
	})

	ctx := NewContext(context.Background(), req)

	// Test handle
	err := router.Handle(ctx)
	if err != nil {
		t.Fatalf("Router.Handle() failed: %v", err)
	}

	// Verify parameter was set
	if ctx.Param("id") != "123" {
		t.Errorf("Expected param id=123, got %s", ctx.Param("id"))
	}
}

package middleware

import (
	"context"
	"strings"
	"testing"

	"github.com/pay-theory/lift/pkg/lift"
	"github.com/pay-theory/lift/pkg/lift/adapters"
	"github.com/stretchr/testify/assert"
)

func TestInputValidationMiddleware(t *testing.T) {
	config := DefaultValidationConfig()
	middleware := InputValidation(config)

	// Create a simple handler for testing
	handler := middleware(lift.HandlerFunc(func(ctx *lift.Context) error {
		return ctx.OK(map[string]string{"status": "ok"})
	}))

	t.Run("Valid request passes validation", func(t *testing.T) {
		req := &adapters.Request{
			Method:      "POST",
			Path:        "/api/test",
			Headers:     map[string]string{"Content-Type": "application/json"},
			QueryParams: map[string]string{"page": "1"},
			PathParams:  map[string]string{"id": "123"},
			Body:        []byte(`{"name": "test"}`),
		}

		ctx := createValidationTestContext(req)
		err := handler.Handle(ctx)
		assert.NoError(t, err)
		assert.Equal(t, 200, ctx.Response.StatusCode)
	})

	t.Run("Request body too large", func(t *testing.T) {
		largeBody := strings.Repeat("a", int(config.MaxBodySize)+1)
		req := &adapters.Request{
			Method: "POST",
			Path:   "/api/test",
			Body:   []byte(largeBody),
		}

		ctx := createValidationTestContext(req)
		err := handler.Handle(ctx)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "Request body too large")
	})

	t.Run("Invalid content type", func(t *testing.T) {
		req := &adapters.Request{
			Method:  "POST",
			Path:    "/api/test",
			Headers: map[string]string{"Content-Type": "application/xml"},
			Body:    []byte(`<test>data</test>`),
		}

		ctx := createValidationTestContext(req)
		err := handler.Handle(ctx)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "Content type not allowed")
	})

	t.Run("Blocked user agent", func(t *testing.T) {
		req := &adapters.Request{
			Method:  "GET",
			Path:    "/api/test",
			Headers: map[string]string{"User-Agent": "sqlmap/1.0"},
		}

		ctx := createValidationTestContext(req)
		err := handler.Handle(ctx)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "User agent blocked")
	})
}

func TestSQLInjectionDetection(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{"Normal text", "hello world", false},
		{"Union select attack", "1' UNION SELECT * FROM users--", true},
		{"Or attack", "1' OR '1'='1", true},
		{"Drop table attack", "'; DROP TABLE users;--", true},
		{"SQL comment", "test-- comment", true},
		{"Information schema", "SELECT * FROM information_schema.tables", true},
		{"Case insensitive", "1' uNiOn SeLeCt * from users", true},
		{"SQL function", "exec('malicious code')", true},
		{"Valid number", "123", false},
		{"Valid email", "user@example.com", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := checkSQLInjection(tt.input)
			if tt.expected {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "SQL injection")
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestXSSDetection(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{"Normal text", "hello world", false},
		{"Script tag", "<script>alert('xss')</script>", true},
		{"JavaScript URL", "javascript:alert('xss')", true},
		{"Event handler", "onload=alert('xss')", true},
		{"Iframe tag", "<iframe src='malicious'></iframe>", true},
		{"Document.cookie", "document.cookie", true},
		{"Window.location", "window.location = 'evil.com'", true},
		{"Eval function", "eval('malicious code')", true},
		{"Case insensitive", "<ScRiPt>alert('xss')</ScRiPt>", true},
		{"Valid HTML", "<p>Normal paragraph</p>", false},
		{"Valid URL", "https://example.com", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := checkXSS(tt.input)
			if tt.expected {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "XSS")
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestPathTraversalDetection(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{"Normal path", "/api/users", false},
		{"Simple traversal", "../../../etc/passwd", true},
		{"Windows traversal", "..\\..\\..\\windows\\system32", true},
		{"URL encoded traversal", "..%2f..%2fetc%2fpasswd", true},
		{"Double encoded", "%2e%2e%2f%2e%2e%2f", true},
		{"Double slash traversal", "....//....//", true},
		{"Valid relative path", "./files/document.txt", false},
		{"Valid absolute path", "/home/user/file.txt", false},
		{"Mixed case", "../ETC/passwd", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := checkPathTraversal(tt.input)
			if tt.expected {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "path traversal")
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidationHelpers(t *testing.T) {
	t.Run("ValidateEmail", func(t *testing.T) {
		validEmails := []string{
			"user@example.com",
			"test.email+tag@domain.co.uk",
			"user123@test-domain.org",
		}

		invalidEmails := []string{
			"invalid-email",
			"@domain.com",
			"user@",
			"user@domain",
			"user.domain.com",
		}

		for _, email := range validEmails {
			assert.NoError(t, ValidateEmail(email), "Should be valid: %s", email)
		}

		for _, email := range invalidEmails {
			assert.Error(t, ValidateEmail(email), "Should be invalid: %s", email)
		}
	})

	t.Run("ValidateUUID", func(t *testing.T) {
		validUUIDs := []string{
			"123e4567-e89b-12d3-a456-426614174000",
			"550e8400-e29b-41d4-a716-446655440000",
			"00000000-0000-0000-0000-000000000000",
		}

		invalidUUIDs := []string{
			"invalid-uuid",
			"123e4567-e89b-12d3-a456",
			"123e4567-e89b-12d3-a456-42661417400g",
			"123e4567e89b12d3a456426614174000",
		}

		for _, uuid := range validUUIDs {
			assert.NoError(t, ValidateUUID(uuid), "Should be valid: %s", uuid)
		}

		for _, uuid := range invalidUUIDs {
			assert.Error(t, ValidateUUID(uuid), "Should be invalid: %s", uuid)
		}
	})

	t.Run("ValidateNumeric", func(t *testing.T) {
		validNumbers := []string{"123", "0", "-456", "3.14", "1.23e-4"}
		invalidNumbers := []string{"abc", "12a3", "1.2.3", ""}

		for _, num := range validNumbers {
			assert.NoError(t, ValidateNumeric(num), "Should be valid: %s", num)
		}

		for _, num := range invalidNumbers {
			assert.Error(t, ValidateNumeric(num), "Should be invalid: %s", num)
		}
	})

	t.Run("ValidateAlphaNumeric", func(t *testing.T) {
		validValues := []string{"abc123", "ABC", "123", "aBc123"}
		invalidValues := []string{"abc-123", "abc 123", "abc@123", ""}

		for _, val := range validValues {
			assert.NoError(t, ValidateAlphaNumeric(val), "Should be valid: %s", val)
		}

		for _, val := range invalidValues {
			assert.Error(t, ValidateAlphaNumeric(val), "Should be invalid: %s", val)
		}
	})

	t.Run("ValidateLength", func(t *testing.T) {
		validator := ValidateLength(3, 10)

		validValues := []string{"abc", "hello", "1234567890"}
		invalidValues := []string{"ab", "12345678901", ""}

		for _, val := range validValues {
			assert.NoError(t, validator(val), "Should be valid: %s", val)
		}

		for _, val := range invalidValues {
			assert.Error(t, validator(val), "Should be invalid: %s", val)
		}
	})
}

func TestCustomValidators(t *testing.T) {
	config := DefaultValidationConfig()
	config.CustomValidators["id"] = ValidateNumeric
	config.CustomValidators["email"] = ValidateEmail

	middleware := InputValidation(config)
	handler := middleware(lift.HandlerFunc(func(ctx *lift.Context) error {
		return ctx.OK(map[string]string{"status": "ok"})
	}))

	t.Run("Valid custom validation", func(t *testing.T) {
		req := &adapters.Request{
			Method:      "GET",
			Path:        "/api/test",
			QueryParams: map[string]string{"id": "123", "email": "user@example.com"},
		}

		ctx := createValidationTestContext(req)
		err := handler.Handle(ctx)
		assert.NoError(t, err)
	})

	t.Run("Invalid custom validation", func(t *testing.T) {
		req := &adapters.Request{
			Method:      "GET",
			Path:        "/api/test",
			QueryParams: map[string]string{"id": "not-a-number"},
		}

		ctx := createValidationTestContext(req)
		err := handler.Handle(ctx)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "must be numeric")
	})
}

func TestHeaderValidation(t *testing.T) {
	config := DefaultValidationConfig()
	config.MaxHeaderSize = 100 // Small limit for testing

	middleware := InputValidation(config)
	handler := middleware(lift.HandlerFunc(func(ctx *lift.Context) error {
		return ctx.OK(map[string]string{"status": "ok"})
	}))

	t.Run("Header too large", func(t *testing.T) {
		largeHeader := strings.Repeat("a", 101)
		req := &adapters.Request{
			Method:  "GET",
			Path:    "/api/test",
			Headers: map[string]string{"X-Large-Header": largeHeader},
		}

		ctx := createValidationTestContext(req)
		err := handler.Handle(ctx)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "too large")
	})

	t.Run("Header with XSS", func(t *testing.T) {
		req := &adapters.Request{
			Method:  "GET",
			Path:    "/api/test",
			Headers: map[string]string{"X-Custom": "<script>alert('xss')</script>"},
		}

		ctx := createValidationTestContext(req)
		err := handler.Handle(ctx)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "XSS")
	})

	t.Run("Header with invalid UTF-8", func(t *testing.T) {
		req := &adapters.Request{
			Method:  "GET",
			Path:    "/api/test",
			Headers: map[string]string{"X-Invalid": string([]byte{0xff, 0xfe, 0xfd})},
		}

		ctx := createValidationTestContext(req)
		err := handler.Handle(ctx)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid UTF-8")
	})
}

func TestJSONValidation(t *testing.T) {
	config := DefaultValidationConfig()
	middleware := InputValidation(config)
	handler := middleware(lift.HandlerFunc(func(ctx *lift.Context) error {
		return ctx.OK(map[string]string{"status": "ok"})
	}))

	t.Run("Valid JSON", func(t *testing.T) {
		req := &adapters.Request{
			Method:  "POST",
			Path:    "/api/test",
			Headers: map[string]string{"Content-Type": "application/json"},
			Body:    []byte(`{"name": "test", "value": 123}`),
		}

		ctx := createValidationTestContext(req)
		err := handler.Handle(ctx)
		assert.NoError(t, err)
	})

	t.Run("Invalid JSON", func(t *testing.T) {
		req := &adapters.Request{
			Method:  "POST",
			Path:    "/api/test",
			Headers: map[string]string{"Content-Type": "application/json"},
			Body:    []byte(`{"name": "test", "value": 123`), // Missing closing brace
		}

		ctx := createValidationTestContext(req)
		err := handler.Handle(ctx)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "Invalid JSON")
	})

	t.Run("JSON with XSS", func(t *testing.T) {
		req := &adapters.Request{
			Method:  "POST",
			Path:    "/api/test",
			Headers: map[string]string{"Content-Type": "application/json"},
			Body:    []byte(`{"script": "<script>alert('xss')</script>"}`),
		}

		ctx := createValidationTestContext(req)
		err := handler.Handle(ctx)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "XSS")
	})
}

// Helper function to create test context
func createValidationTestContext(req *adapters.Request) *lift.Context {
	if req.Headers == nil {
		req.Headers = make(map[string]string)
	}
	if req.QueryParams == nil {
		req.QueryParams = make(map[string]string)
	}
	if req.PathParams == nil {
		req.PathParams = make(map[string]string)
	}

	liftReq := lift.NewRequest(req)
	return lift.NewContext(context.Background(), liftReq)
}

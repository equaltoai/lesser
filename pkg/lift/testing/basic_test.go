package testing

import (
	"testing"

	"github.com/pay-theory/lift/pkg/lift"
	"github.com/stretchr/testify/assert"
)

// TestBasicTestAppCreation tests that we can create a TestApp
func TestBasicTestAppCreation(t *testing.T) {
	app := NewTestApp()
	assert.NotNil(t, app)
	assert.NotNil(t, app.App())
}

// TestBasicHTTPMethods tests basic HTTP method support
func TestBasicHTTPMethods(t *testing.T) {
	app := NewTestApp()
	
	// Setup a simple endpoint
	app.App().GET("/test", func(ctx *lift.Context) error {
		return ctx.JSON(map[string]string{"message": "success"})
	})
	
	// Test GET
	response := app.GET("/test")
	assert.Equal(t, 200, response.StatusCode)
	assert.Contains(t, response.Body, "success")
}

// TestHeadersWork tests that headers are properly applied
func TestHeadersWork(t *testing.T) {
	app := NewTestApp()
	
	// Setup an endpoint that echoes a header
	app.App().GET("/echo", func(ctx *lift.Context) error {
		auth := ctx.Header("Authorization")
		return ctx.JSON(map[string]string{"auth": auth})
	})
	
	// Test with header
	response := app.WithHeader("Authorization", "Bearer test").GET("/echo")
	assert.Equal(t, 200, response.StatusCode)
	assert.Contains(t, response.Body, "Bearer test")
}

// TestOPTIONSMethod tests OPTIONS method support
func TestOPTIONSMethod(t *testing.T) {
	app := NewTestApp()
	
	response := app.OPTIONS("/test")
	assert.Equal(t, 200, response.StatusCode)
	assert.Contains(t, response.Body, "allowed")
}

// TestHEADMethod tests HEAD method support  
func TestHEADMethod(t *testing.T) {
	app := NewTestApp()
	
	response := app.HEAD("/test")
	assert.Equal(t, 200, response.StatusCode)
	assert.Equal(t, "", response.Body) // HEAD should have no body
}
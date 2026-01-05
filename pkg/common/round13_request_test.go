package common

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	liftTesting "github.com/equaltoai/lesser/pkg/testing/lift"
	"github.com/pay-theory/lift/pkg/lift"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadRequestBody_SizeLimits(t *testing.T) {
	ok, err := ReadRequestBody(strings.NewReader("hi"), 2)
	require.NoError(t, err)
	assert.Equal(t, []byte("hi"), ok)

	_, err = ReadRequestBody(strings.NewReader("toolong"), 3)
	assert.Error(t, err)

	str, err := ReadRequestBodyString(strings.NewReader("x"), 1)
	require.NoError(t, err)
	assert.Equal(t, "x", str)
}

func TestParseRequestWithFallback_UsesFallbackBodiesWhenParseRequestFails(t *testing.T) {
	type payload struct {
		Name string `json:"name"`
	}

	t.Run("ctx.Request.Body", func(t *testing.T) {
		ctx := liftTesting.MockLiftContext("POST", "/test", liftTesting.WithBody([]byte(`{"name":"alice"}`)))
		// Force ctx.ParseRequest to fail by removing the underlying adapter request.
		ctx.Request.Request = nil

		var out payload
		err := ParseRequestWithFallback(ctx, &out)
		require.NoError(t, err)
		assert.Equal(t, "alice", out.Name)
	})

	t.Run("ctx.Request.Request.Body", func(t *testing.T) {
		ctx := liftTesting.MockLiftContext("POST", "/test", liftTesting.WithBody([]byte(`{"name":"bob"}`)))
		ctx.Request.Body = nil

		var out payload
		err := ParseRequestWithFallback(ctx, &out)
		require.NoError(t, err)
		assert.Equal(t, "bob", out.Name)
	})

	t.Run("failure", func(t *testing.T) {
		ctx := &lift.Context{Request: &lift.Request{}}
		var out payload
		err := ParseRequestWithFallback(ctx, &out)
		assert.Error(t, err)
	})
}

func TestParseRequestHelpers_ResponseWrapping(t *testing.T) {
	type payload struct {
		Name string `json:"name"`
	}

	// ParseRequestWithValidation returns a validation response on failure.
	ctx := liftTesting.MockLiftContext("POST", "/test")
	var out payload
	err := ParseRequestWithValidation(ctx, &out)
	assert.NoError(t, err)
	assert.Equal(t, 400, ctx.Response.StatusCode)

	// ParseRequestWithCustomError returns a bad request response on failure.
	ctx2 := liftTesting.MockLiftContext("POST", "/test")
	err = ParseRequestWithCustomError(ctx2, &out, "bad")
	assert.NoError(t, err)
	assert.Equal(t, 400, ctx2.Response.StatusCode)

	// ParseRequestBodyWithValidation returns missing parameter response.
	ctx3 := liftTesting.MockLiftContext("POST", "/test")
	err = ParseRequestBodyWithValidation(ctx3, &out, "name")
	assert.NoError(t, err)
	assert.Equal(t, 400, ctx3.Response.StatusCode)
}

func TestParseRequestWithComplexFallback_UsesAlternateBodySource(t *testing.T) {
	type payload struct {
		Name string `json:"name"`
	}

	ctx := liftTesting.MockLiftContext("POST", "/test")
	ctx.Request.Body = nil
	ctx.Request.Request.Body = []byte(`{"name":"ok"}`)

	var out payload
	err := ParseRequestWithComplexFallback(ctx, &out)
	require.NoError(t, err)
	assert.Equal(t, "ok", out.Name)
}

func TestReadRequestBody_ZeroOrNegativeUsesDefault(t *testing.T) {
	// MaxRequestSize default is large enough; this should not fail.
	data := bytes.Repeat([]byte("a"), 10)
	out, err := ReadRequestBody(bytes.NewReader(data), 0)
	require.NoError(t, err)
	assert.Equal(t, data, out)
}

func TestParseRequestWithFallback_WhenParseRequestSucceeds(t *testing.T) {
	type payload struct {
		Name string `json:"name"`
	}

	body, _ := json.Marshal(payload{Name: "ok"})
	ctx := liftTesting.MockLiftContext("POST", "/test", liftTesting.WithBody(body), liftTesting.WithHeaders(map[string]string{"Content-Type": "application/json"}))

	var out payload
	err := ParseRequestWithFallback(ctx, &out)
	require.NoError(t, err)
	assert.Equal(t, "ok", out.Name)
}

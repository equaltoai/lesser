package common

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apptheory "github.com/theory-cloud/apptheory/v2/runtime"
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

	t.Run("parses request body", func(t *testing.T) {
		ctx := &apptheory.Context{Request: apptheory.Request{Method: "POST", Path: "/test", Body: []byte(`{"name":"alice"}`)}}
		var out payload
		err := ParseRequestWithFallback(ctx, &out)
		require.NoError(t, err)
		assert.Equal(t, "alice", out.Name)
	})

	t.Run("failure", func(t *testing.T) {
		ctx := &apptheory.Context{Request: apptheory.Request{Method: "POST", Path: "/test"}}
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
	ctx := &apptheory.Context{Request: apptheory.Request{Method: "POST", Path: "/test"}}
	var out payload
	resp, err := ParseRequestWithValidation(ctx, &out)
	assert.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, 400, resp.Status)

	// ParseRequestWithCustomError returns a bad request response on failure.
	ctx2 := &apptheory.Context{Request: apptheory.Request{Method: "POST", Path: "/test"}}
	resp, err = ParseRequestWithCustomError(ctx2, &out, "bad")
	assert.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, 400, resp.Status)

	// ParseRequestBodyWithValidation returns missing parameter response.
	ctx3 := &apptheory.Context{Request: apptheory.Request{Method: "POST", Path: "/test"}}
	resp, err = ParseRequestBodyWithValidation(ctx3, &out, "name")
	assert.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, 400, resp.Status)
}

func TestParseRequestWithComplexFallback_UsesAlternateBodySource(t *testing.T) {
	type payload struct {
		Name string `json:"name"`
	}

	ctx := &apptheory.Context{Request: apptheory.Request{Method: "POST", Path: "/test", Body: []byte(`{"name":"ok"}`)}}

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
	ctx := &apptheory.Context{Request: apptheory.Request{Method: "POST", Path: "/test", Body: body, Headers: map[string][]string{"content-type": {"application/json"}}}}

	var out payload
	err := ParseRequestWithFallback(ctx, &out)
	require.NoError(t, err)
	assert.Equal(t, "ok", out.Name)
}

package main

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
	apptheory "github.com/theory-cloud/apptheory/v2/runtime"
)

func TestActorActivityJSON_Round24_MarshalError(t *testing.T) {
	resp, err := actorActivityJSON(http.StatusOK, make(chan int))
	require.Nil(t, resp)
	require.Error(t, err)
}

func TestConvertAppTheoryRequest_Round24_NilContext(t *testing.T) {
	h := &Handler{}
	req, err := h.convertAppTheoryRequest(nil)
	require.Nil(t, req)
	require.Error(t, err)
}

func TestConvertAppTheoryRequest_Round24_SetsHostAndQuery(t *testing.T) {
	h := &Handler{}
	ctx := &apptheory.Context{
		Request: apptheory.Request{
			Method: http.MethodGet,
			Path:   "/users/alice",
			Query: map[string][]string{
				"foo": {"bar"},
			},
			Headers: map[string][]string{
				"host":                     {"internal.execute-api.us-east-1.amazonaws.com"},
				"x-lesser-forwarded-host":  {"example.com"},
				"x-lesser-forwarded-proto": {"https"},
				"x-extra-header":           {"value"},
				"x-empty":                  {},
			},
		},
	}

	req, err := h.convertAppTheoryRequest(ctx)
	require.NoError(t, err)
	require.Equal(t, "example.com", req.Host)
	require.Equal(t, "bar", req.URL.Query().Get("foo"))
	require.Equal(t, "value", req.Header.Get("x-extra-header"))
	require.Empty(t, req.Header.Values("x-empty"))
}

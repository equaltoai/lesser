package handlers

import (
	"testing"

	"github.com/stretchr/testify/require"
	apptheory "github.com/theory-cloud/apptheory/runtime"
)

func requireStatus(t *testing.T, expected int) func(*apptheory.Response, error) *apptheory.Response {
	t.Helper()

	return func(resp *apptheory.Response, err error) *apptheory.Response {
		t.Helper()

		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, expected, resp.Status, "response body: %s", string(resp.Body))
		return resp
	}
}

package main

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRedirectResponse_Round15(t *testing.T) {
	resp := redirectResponse("https://example.com", false)
	require.Equal(t, http.StatusFound, resp.Status)
	require.Equal(t, []string{"https://example.com"}, resp.Headers["location"])

	resp = redirectResponse("https://example.com", true)
	require.Equal(t, http.StatusMovedPermanently, resp.Status)
	require.Equal(t, []string{"https://example.com"}, resp.Headers["location"])
}


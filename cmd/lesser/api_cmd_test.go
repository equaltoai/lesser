package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestAPICmd_Request_SuccessAndValidation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/oauth/token":
			require.NoError(t, r.ParseForm())
			require.Equal(t, "refresh_token", r.FormValue("grant_type"))
			require.Equal(t, "client-1", r.FormValue("client_id"))
			require.NotEmpty(t, r.FormValue("refresh_token"))
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"access-1","token_type":"Bearer","expires_in":3600,"refresh_token":"refresh-1","scope":"read","created_at":1}`))
		case "/api/test":
			require.Equal(t, "Bearer access-1", r.Header.Get("Authorization"))
			_, _ = w.Write([]byte("ok"))
		case "/api/post":
			require.Equal(t, http.MethodPost, r.Method)
			require.Equal(t, "application/json", r.Header.Get("Content-Type"))
			_, _ = w.Write([]byte("created"))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	t.Setenv("HOME", t.TempDir())
	t.Setenv("LESSER_AUTH_SECRET", "test-secret")

	baseURL := srv.URL
	key := deriveAuthKey(baseURL, "test-secret")

	require.NoError(t, writeAuthSession(baseURL, key, &cliAuthSession{
		Version:      cliAuthSessionVersion,
		BaseURL:      baseURL,
		ClientID:     "client-1",
		RefreshToken: "refresh-1",
		Username:     "alice",
		CreatedAt:    time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
	}))

	require.NoError(t, runAPI(nil))
	require.NoError(t, runAPI([]string{helpFlagShort}))
	require.NoError(t, runAPI([]string{"-x"}))
	require.Error(t, runAPI([]string{"nope"}))

	require.Error(t, runAPIRequest([]string{"--base-url", baseURL, "--method", "GET"}))
	require.Error(t, runAPIRequest([]string{"--base-url", baseURL, "--method", "GET", "--path", "api/test"}))
	require.Error(t, runAPIRequest([]string{"--base-url", baseURL, "--method", "GET", "--path", "/api/test", "--header", "bad"}))

	dataFile := filepath.Join(t.TempDir(), "data.json")
	require.NoError(t, os.WriteFile(dataFile, []byte(`{"hello":"world"}`), 0o600))
	require.Error(t, runAPIRequest([]string{"--base-url", baseURL, "--method", "POST", "--path", "/api/post", "--data", "x", "--data-file", dataFile}))

	require.NoError(t, runAPIRequest([]string{"--base-url", baseURL, "--method", "GET", "--path", "/api/test", "--rps", "0", "--max-concurrency", "1", "--retries", "0", "--timeout", "2"}))
	require.NoError(t, runAPIRequest([]string{"--base-url", baseURL, "--method", "POST", "--path", "/api/post", "--data-file", dataFile, "--rps", "0", "--max-concurrency", "1", "--retries", "0", "--timeout", "2"}))
}

func TestAPICmd_HelperFlagsAndParsers(t *testing.T) {
	t.Setenv("LESSER_CLI_MAX_CONCURRENCY", "")
	require.Equal(t, 2, envOrDefaultInt("LESSER_CLI_MAX_CONCURRENCY", 2))

	t.Setenv("LESSER_CLI_MAX_CONCURRENCY", "bad")
	require.Equal(t, 2, envOrDefaultInt("LESSER_CLI_MAX_CONCURRENCY", 2))

	t.Setenv("LESSER_CLI_MAX_CONCURRENCY", "3")
	require.Equal(t, 3, envOrDefaultInt("LESSER_CLI_MAX_CONCURRENCY", 2))

	t.Setenv("LESSER_CLI_RPS", "")
	require.Equal(t, 2.0, envOrDefaultFloat("LESSER_CLI_RPS", 2.0))

	t.Setenv("LESSER_CLI_RPS", "bad")
	require.Equal(t, 2.0, envOrDefaultFloat("LESSER_CLI_RPS", 2.0))

	t.Setenv("LESSER_CLI_RPS", "3.5")
	require.Equal(t, 3.5, envOrDefaultFloat("LESSER_CLI_RPS", 2.0))

	require.Error(t, requireNonEmpty("name", ""))
	require.NoError(t, requireNonEmpty("name", "x"))

	_, _, err := splitHeader("")
	require.Error(t, err)
	_, _, err = splitHeader("bad")
	require.Error(t, err)
	_, _, err = splitHeader(": x")
	require.Error(t, err)

	name, value, err := splitHeader("X-Test: ok")
	require.NoError(t, err)
	require.Equal(t, "X-Test", name)
	require.Equal(t, "ok", value)

	body, err := readBodyArg("", "")
	require.NoError(t, err)
	require.Nil(t, body)

	_, err = readBodyArg("x", "y")
	require.Error(t, err)

	file := filepath.Join(t.TempDir(), "body.txt")
	require.NoError(t, os.WriteFile(file, []byte("hello"), 0o600))
	body, err = readBodyArg("", file)
	require.NoError(t, err)
	require.Equal(t, []byte("hello"), body)

	var mv multiValueFlag
	require.Equal(t, "", mv.String())
	require.NoError(t, mv.Set("a"))
	require.NoError(t, mv.Set("b"))
	require.Equal(t, []string{"a", "b"}, mv.Values())

	var nilMV *multiValueFlag
	require.Equal(t, "", nilMV.String())
	require.Nil(t, nilMV.Values())

	_, err = readBodyArg("", filepath.Join(t.TempDir(), "missing"))
	require.Error(t, err)
}

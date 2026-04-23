package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRunVerify_DispatchesUnresolvedRemoteParent(t *testing.T) {
	previous := runVerifyUnresolvedRemoteParentFn
	t.Cleanup(func() { runVerifyUnresolvedRemoteParentFn = previous })

	var called bool
	runVerifyUnresolvedRemoteParentFn = func(argv []string) error {
		called = true
		require.Equal(t, []string{"--base-url", "https://dev.example.com", "--token", "tok", "--parent-url", "https://remote.example/users/alice/statuses/1"}, argv)
		return nil
	}

	require.NoError(t, runVerify([]string{"unresolved-remote-parent", "--base-url", "https://dev.example.com", "--token", "tok", "--parent-url", "https://remote.example/users/alice/statuses/1"}))
	require.True(t, called)
}

func TestValidateVerifyUnresolvedRemoteParentConfig(t *testing.T) {
	t.Run("requires core flags", func(t *testing.T) {
		require.ErrorContains(t, validateVerifyUnresolvedRemoteParentConfig(verifyUnresolvedRemoteParentConfig{}), "--base-url is required")
		require.ErrorContains(t, validateVerifyUnresolvedRemoteParentConfig(verifyUnresolvedRemoteParentConfig{BaseURL: "https://dev.example.com"}), "--token is required")
		require.ErrorContains(t, validateVerifyUnresolvedRemoteParentConfig(verifyUnresolvedRemoteParentConfig{BaseURL: "https://dev.example.com", Token: "tok"}), "--parent-url is required")
	})

	t.Run("blocks live and wrong stage hosts", func(t *testing.T) {
		require.ErrorContains(t, validateVerifyUnresolvedRemoteParentConfig(verifyUnresolvedRemoteParentConfig{
			BaseURL:        "https://example.com",
			Stage:          "live",
			Token:          "tok",
			ParentURL:      "https://remote.example/users/alice/statuses/1",
			Visibility:     "public",
			Expected:       "success",
			TimeoutSeconds: 15,
		}), "must not run against live")
		require.ErrorContains(t, validateVerifyUnresolvedRemoteParentConfig(verifyUnresolvedRemoteParentConfig{
			BaseURL:        "https://staging.example.com",
			Stage:          valueDev,
			Token:          "tok",
			ParentURL:      "https://remote.example/users/alice/statuses/1",
			Visibility:     "public",
			Expected:       "success",
			TimeoutSeconds: 15,
		}), "dev subdomain")
	})

	t.Run("rejects invalid visibility and bad url for success probes", func(t *testing.T) {
		require.ErrorContains(t, validateVerifyUnresolvedRemoteParentConfig(verifyUnresolvedRemoteParentConfig{
			BaseURL:        "https://dev.example.com",
			Stage:          valueDev,
			Token:          "tok",
			ParentURL:      "https://remote.example/users/alice/statuses/1",
			Visibility:     "direct",
			Expected:       "success",
			TimeoutSeconds: 15,
		}), "conversations-owned")
		require.ErrorContains(t, validateVerifyUnresolvedRemoteParentConfig(verifyUnresolvedRemoteParentConfig{
			BaseURL:        "https://dev.example.com",
			Stage:          valueDev,
			Token:          "tok",
			ParentURL:      "not-a-url",
			Visibility:     "public",
			Expected:       "success",
			TimeoutSeconds: 15,
		}), "parent_url")
	})

	t.Run("bad request probes may use intentionally invalid parent references", func(t *testing.T) {
		require.NoError(t, validateVerifyUnresolvedRemoteParentConfig(verifyUnresolvedRemoteParentConfig{
			BaseURL:        "https://dev.example.com",
			Stage:          valueDev,
			Token:          "tok",
			ParentURL:      "not-a-url",
			Visibility:     "public",
			Expected:       "bad-request",
			TimeoutSeconds: 15,
		}))
	})
}

func TestRunVerifyUnresolvedRemoteParent_Success(t *testing.T) {
	previousClient := newVerifyUnresolvedRemoteParentClientFn
	previousExecute := executeVerifyUnresolvedRemoteParentFn
	previousNow := verifyUnresolvedRemoteParentNowFn
	t.Cleanup(func() {
		newVerifyUnresolvedRemoteParentClientFn = previousClient
		executeVerifyUnresolvedRemoteParentFn = previousExecute
		verifyUnresolvedRemoteParentNowFn = previousNow
	})

	verifyUnresolvedRemoteParentNowFn = func() time.Time {
		return time.Date(2026, time.April, 22, 23, 15, 0, 0, time.UTC)
	}
	newVerifyUnresolvedRemoteParentClientFn = func(int) *http.Client { return http.DefaultClient }
	executeVerifyUnresolvedRemoteParentFn = func(_ context.Context, _ *http.Client, cfg verifyUnresolvedRemoteParentConfig) (verifyUnresolvedRemoteParentSummary, error) {
		require.Equal(t, "verify unresolved remote parent 2026-04-22T23:15:00Z", cfg.Content)
		return verifyUnresolvedRemoteParentSummary{
			BaseURL:         "https://dev.example.com",
			Stage:           valueDev,
			ParentURL:       cfg.ParentURL,
			Visibility:      cfg.Visibility,
			Expected:        cfg.Expected,
			Classification:  "success",
			HTTPStatus:      http.StatusCreated,
			CreatedStatusID: "123",
		}, nil
	}

	stdout := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = w
	t.Cleanup(func() { os.Stdout = stdout })

	err = runVerifyUnresolvedRemoteParent([]string{
		"--base-url", "https://dev.example.com",
		"--token", "tok",
		"--parent-url", "https://remote.example/users/alice/statuses/1",
	})
	require.NoError(t, err)
	require.NoError(t, w.Close())
	var out bytes.Buffer
	_, err = io.Copy(&out, r)
	require.NoError(t, err)
	require.Contains(t, out.String(), "verify unresolved-remote-parent complete")
	require.Contains(t, out.String(), "created_status_id: 123")
}

func TestExecuteVerifyUnresolvedRemoteParent(t *testing.T) {
	t.Run("success captures created status id", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			require.Equal(t, http.MethodPost, r.Method)
			require.Equal(t, "/api/v1/statuses", r.URL.Path)
			require.Equal(t, "Bearer tok", r.Header.Get("Authorization"))
			var payload map[string]any
			require.NoError(t, jsonNewDecoder(r.Body).Decode(&payload))
			require.Equal(t, "hello", payload["status"])
			require.Equal(t, "public", payload["visibility"])
			require.Equal(t, "https://remote.example/users/alice/statuses/1", payload["in_reply_to_id"])
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":"status-1"}`))
		}))
		defer server.Close()

		summary, err := executeVerifyUnresolvedRemoteParent(context.Background(), server.Client(), verifyUnresolvedRemoteParentConfig{
			BaseURL:    server.URL,
			Stage:      valueDev,
			Token:      "tok",
			ParentURL:  "https://remote.example/users/alice/statuses/1",
			Visibility: "public",
			Expected:   "success",
			Content:    "hello",
		})
		require.NoError(t, err)
		require.Equal(t, "success", summary.Classification)
		require.Equal(t, "status-1", summary.CreatedStatusID)
	})

	t.Run("unusable response preserves error code", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnprocessableEntity)
			_, _ = w.Write([]byte(`{"error":"remote reply parent is not usable","code":"unprocessable_entity"}`))
		}))
		defer server.Close()

		summary, err := executeVerifyUnresolvedRemoteParent(context.Background(), server.Client(), verifyUnresolvedRemoteParentConfig{
			BaseURL:    server.URL,
			Stage:      valueDev,
			Token:      "tok",
			ParentURL:  "https://remote.example/users/alice/statuses/1",
			Visibility: "private",
			Expected:   "unusable",
			Content:    "hello",
		})
		require.NoError(t, err)
		require.Equal(t, "unusable", summary.Classification)
		require.Equal(t, "unprocessable_entity", summary.ErrorCode)
	})

	t.Run("mismatch returns descriptive error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "upstream timeout", http.StatusRequestTimeout)
		}))
		defer server.Close()

		_, err := executeVerifyUnresolvedRemoteParent(context.Background(), server.Client(), verifyUnresolvedRemoteParentConfig{
			BaseURL:    server.URL,
			Stage:      valueDev,
			Token:      "tok",
			ParentURL:  "https://remote.example/users/alice/statuses/1",
			Visibility: "public",
			Expected:   "success",
			Content:    "hello",
		})
		require.ErrorContains(t, err, "expected success but got timeout")
	})
}

func TestClassifyVerifyUnresolvedRemoteParentStatus(t *testing.T) {
	require.Equal(t, "success", classifyVerifyUnresolvedRemoteParentStatus(http.StatusCreated))
	require.Equal(t, "bad-request", classifyVerifyUnresolvedRemoteParentStatus(http.StatusBadRequest))
	require.Equal(t, "timeout", classifyVerifyUnresolvedRemoteParentStatus(http.StatusRequestTimeout))
	require.Equal(t, "unusable", classifyVerifyUnresolvedRemoteParentStatus(http.StatusUnprocessableEntity))
	require.Equal(t, "unavailable", classifyVerifyUnresolvedRemoteParentStatus(http.StatusServiceUnavailable))
	require.Equal(t, "unexpected-500", classifyVerifyUnresolvedRemoteParentStatus(http.StatusInternalServerError))
}

func TestVerifyUnresolvedRemoteParentBodyMessage(t *testing.T) {
	require.Equal(t, "remote fetch timed out", verifyUnresolvedRemoteParentBodyMessage([]byte(`{"error_description":"remote fetch timed out"}`)))
	require.Equal(t, "plain text", verifyUnresolvedRemoteParentBodyMessage([]byte("plain text")))
}

func jsonNewDecoder(r io.Reader) *json.Decoder {
	return json.NewDecoder(r)
}

func TestValidateVerifyUnresolvedRemoteParentTarget(t *testing.T) {
	require.ErrorContains(t, validateVerifyUnresolvedRemoteParentTarget(valueDev, "://bad"), "absolute URL")
	require.NoError(t, validateVerifyUnresolvedRemoteParentTarget(valueStaging, "https://staging.example.com"))
	require.ErrorContains(t, validateVerifyUnresolvedRemoteParentTarget("qa", "https://dev.example.com"), "--stage must be dev or staging")
}

func TestNewVerifyUnresolvedRemoteParentHTTPClient_DefaultTimeout(t *testing.T) {
	client := newVerifyUnresolvedRemoteParentHTTPClient(0)
	require.Equal(t, defaultHTTPTimeout, client.Timeout)
}

func TestExecuteVerifyUnresolvedRemoteParent_EdgeCases(t *testing.T) {
	t.Run("invalid endpoint bubbles up", func(t *testing.T) {
		_, err := executeVerifyUnresolvedRemoteParent(context.Background(), http.DefaultClient, verifyUnresolvedRemoteParentConfig{
			BaseURL:    "://bad",
			Stage:      valueDev,
			Token:      "tok",
			ParentURL:  "https://remote.example/users/alice/statuses/1",
			Visibility: "public",
			Expected:   "success",
			Content:    "hello",
		})
		require.Error(t, err)
	})

	t.Run("success without id fails", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"url":"https://dev.example.com/users/alice/statuses/1"}`))
		}))
		defer server.Close()

		_, err := executeVerifyUnresolvedRemoteParent(context.Background(), server.Client(), verifyUnresolvedRemoteParentConfig{
			BaseURL:    server.URL,
			Stage:      valueDev,
			Token:      "tok",
			ParentURL:  "https://remote.example/users/alice/statuses/1",
			Visibility: "public",
			Expected:   "success",
			Content:    "hello",
		})
		require.ErrorContains(t, err, "missing id")
	})

	t.Run("success with malformed json fails", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`not-json`))
		}))
		defer server.Close()

		_, err := executeVerifyUnresolvedRemoteParent(context.Background(), server.Client(), verifyUnresolvedRemoteParentConfig{
			BaseURL:    server.URL,
			Stage:      valueDev,
			Token:      "tok",
			ParentURL:  "https://remote.example/users/alice/statuses/1",
			Visibility: "public",
			Expected:   "success",
			Content:    "hello",
		})
		require.ErrorContains(t, err, "decode create-status response")
	})
}

func TestVerifyUnresolvedRemoteParentDoRequest_WithoutAuthHeader(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Empty(t, r.Header.Get("Authorization"))
		_, _ = w.Write([]byte(`ok`))
	}))
	defer server.Close()

	status, body, err := verifyUnresolvedRemoteParentDoRequest(context.Background(), server.Client(), http.MethodGet, server.URL, "", nil)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, status)
	require.Equal(t, []byte("ok"), body)
}

func TestDecodeVerifyUnresolvedRemoteParentErrorAndContentHelpers(t *testing.T) {
	_, ok := decodeVerifyUnresolvedRemoteParentError(nil)
	require.False(t, ok)
	_, ok = decodeVerifyUnresolvedRemoteParentError([]byte(`not-json`))
	require.False(t, ok)
	require.Equal(t, "empty response body", verifyUnresolvedRemoteParentBodyMessage(nil))
	require.Equal(t, "custom", resolveVerifyUnresolvedRemoteParentContent("custom", "ignored"))
	require.Equal(t, "verify unresolved remote parent suffix", resolveVerifyUnresolvedRemoteParentContent("", "suffix"))
}

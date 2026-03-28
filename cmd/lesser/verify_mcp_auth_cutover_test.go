package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/stretchr/testify/require"
)

func TestParseVerifyMCPAuthCutoverActors(t *testing.T) {
	actors, err := parseVerifyMCPAuthCutoverActors(" agent-a,agent-b,agent-a ")
	require.NoError(t, err)
	require.Equal(t, []string{"agent-a", "agent-b"}, actors)

	_, err = parseVerifyMCPAuthCutoverActors("agent-a")
	require.ErrorContains(t, err, "at least two unique actor usernames")

	_, err = parseVerifyMCPAuthCutoverActors("agent-a,bad actor")
	require.ErrorContains(t, err, "invalid actor username")
}

func TestExecuteVerifyMCPAuthCutover_ReadOnly(t *testing.T) {
	var protectedHits []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/oauth-authorization-server":
			require.Equal(t, http.MethodGet, r.Method)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"issuer":                serverURLFromRequest(r),
				"registration_endpoint": serverURLFromRequest(r) + "/oauth/register",
				"grant_types_supported": []string{"authorization_code", "refresh_token"},
			})
		case "/.well-known/oauth-protected-resource/mcp/agent-a":
			protectedHits = append(protectedHits, r.URL.Path)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"resource":              serverURLFromRequest(r) + "/mcp/agent-a",
				"authorization_servers": []string{serverURLFromRequest(r)},
			})
		case "/.well-known/oauth-protected-resource/mcp/agent-b":
			protectedHits = append(protectedHits, r.URL.Path)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"resource":              serverURLFromRequest(r) + "/mcp/agent-b",
				"authorization_servers": []string{serverURLFromRequest(r) + "/.well-known/oauth-authorization-server"},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	summary, err := executeVerifyMCPAuthCutover(context.Background(), server.Client(), server.URL, []string{"agent-a", "agent-b"}, false)
	require.NoError(t, err)
	require.Equal(t, server.URL, summary.BaseURL)
	require.Equal(t, server.URL, summary.AuthorizationServer)
	require.Equal(t, server.URL+"/oauth/register", summary.RegistrationEndpoint)
	require.Equal(t, []string{server.URL + "/mcp/agent-a", server.URL + "/mcp/agent-b"}, summary.ProtectedResources)
	require.False(t, summary.WriteChecks)
	require.Len(t, protectedHits, 2)
}

func TestExecuteVerifyMCPAuthCutover_WriteChecks(t *testing.T) {
	var registerCalls int
	var appsCalls int

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/oauth-authorization-server":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"issuer":                serverURLFromRequest(r),
				"registration_endpoint": serverURLFromRequest(r) + "/oauth/register",
				"grant_types_supported": []string{"authorization_code", "refresh_token"},
			})
		case "/.well-known/oauth-protected-resource/mcp/agent-a":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"resource":              serverURLFromRequest(r) + "/mcp/agent-a",
				"authorization_servers": []string{serverURLFromRequest(r)},
			})
		case "/.well-known/oauth-protected-resource/mcp/agent-b":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"resource":              serverURLFromRequest(r) + "/mcp/agent-b",
				"authorization_servers": []string{serverURLFromRequest(r)},
			})
		case "/oauth/register":
			registerCalls++
			require.Equal(t, http.MethodPost, r.Method)
			var req map[string]any
			require.NoError(t, json.NewDecoder(r.Body).Decode(&req))

			var grantTypes []string
			if raw, ok := req["grant_types"].([]any); ok {
				for _, item := range raw {
					grantTypes = append(grantTypes, strings.TrimSpace(item.(string)))
				}
			}

			if len(grantTypes) == 1 && grantTypes[0] == auth.GrantTypeClientCredentials {
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"error":             "invalid_client_metadata",
					"error_description": "client_credentials is not supported for public registration",
				})
				return
			}

			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"client_id":                  "client-public",
				"client_name":                req["client_name"],
				"client_class":               "cli",
				"grant_types":                []string{"authorization_code", "refresh_token"},
				"token_endpoint_auth_method": "none",
			})
		case "/api/v1/apps":
			appsCalls++
			require.Equal(t, http.MethodPost, r.Method)
			require.Contains(t, r.Header.Get("Content-Type"), "application/x-www-form-urlencoded")
			body, err := io.ReadAll(r.Body)
			require.NoError(t, err)
			values, err := url.ParseQuery(string(body))
			require.NoError(t, err)
			require.Equal(t, "agent-a", values.Get("agent_username"))
			w.WriteHeader(http.StatusUnprocessableEntity)
			_, _ = w.Write([]byte(`{"error":"agent_username is not supported for public registration"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	summary, err := executeVerifyMCPAuthCutover(context.Background(), server.Client(), server.URL, []string{"agent-a", "agent-b"}, true)
	require.NoError(t, err)
	require.True(t, summary.WriteChecks)
	require.Equal(t, 2, registerCalls)
	require.Equal(t, 1, appsCalls)
}

func TestExecuteVerifyMCPAuthCutover_FailsWhenClientCredentialsAdvertised(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/oauth-authorization-server":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"issuer":                serverURLFromRequest(r),
				"registration_endpoint": serverURLFromRequest(r) + "/oauth/register",
				"grant_types_supported": []string{"authorization_code", "refresh_token", auth.GrantTypeClientCredentials},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	_, err := executeVerifyMCPAuthCutover(context.Background(), server.Client(), server.URL, []string{"agent-a", "agent-b"}, false)
	require.ErrorContains(t, err, "still advertises client_credentials")
}

func TestRunVerifyMCPAuthCutover_PrintsSummary(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/oauth-authorization-server":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"issuer":                serverURLFromRequest(r),
				"registration_endpoint": serverURLFromRequest(r) + "/oauth/register",
				"grant_types_supported": []string{"authorization_code", "refresh_token"},
			})
		case "/.well-known/oauth-protected-resource/mcp/agent-a":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"resource":              serverURLFromRequest(r) + "/mcp/agent-a",
				"authorization_servers": []string{serverURLFromRequest(r)},
			})
		case "/.well-known/oauth-protected-resource/mcp/agent-b":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"resource":              serverURLFromRequest(r) + "/mcp/agent-b",
				"authorization_servers": []string{serverURLFromRequest(r)},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	output := captureStdout(t, func() {
		require.NoError(t, runVerify([]string{
			"mcp-auth-cutover",
			"--base-url", server.URL,
			"--actors", "agent-a,agent-b",
		}))
	})

	require.Contains(t, output, "verify mcp-auth-cutover complete")
	require.Contains(t, output, "actors_checked: 2")
}

func TestRunVerifyMCPAuthCutover_RequiresActors(t *testing.T) {
	err := runVerifyMCPAuthCutover([]string{
		"--base-url", "https://example.com",
	})
	require.ErrorContains(t, err, "at least two unique actor usernames")
}

func TestVerifyMCPAuthCutoverHelpers(t *testing.T) {
	t.Run("http client defaults timeout", func(t *testing.T) {
		client := newVerifyMCPAuthCutoverHTTPClient(0)
		require.NotNil(t, client)
		require.Equal(t, defaultHTTPTimeout, client.Timeout)
	})

	t.Run("resolve endpoint accepts relative and absolute values", func(t *testing.T) {
		endpoint, err := resolveVerifyMCPAuthCutoverEndpoint("https://example.com/", "/oauth/register")
		require.NoError(t, err)
		require.Equal(t, "https://example.com/oauth/register", endpoint)

		endpoint, err = resolveVerifyMCPAuthCutoverEndpoint("https://example.com/", "https://auth.example.com/register")
		require.NoError(t, err)
		require.Equal(t, "https://auth.example.com/register", endpoint)

		_, err = resolveVerifyMCPAuthCutoverEndpoint("https://example.com/", "not a url")
		require.ErrorContains(t, err, "not an absolute URL")
	})

	t.Run("protected resource auth server matching accepts issuer, base url, and metadata url", func(t *testing.T) {
		require.True(t, protectedResourcePointsAtAuthServer(
			[]string{"https://example.com"},
			"https://example.com",
			"https://example.com",
		))
		require.True(t, protectedResourcePointsAtAuthServer(
			[]string{"https://example.com/.well-known/oauth-authorization-server"},
			"https://example.com",
			"https://example.com",
		))
		require.False(t, protectedResourcePointsAtAuthServer(
			[]string{"https://other.example.com"},
			"https://example.com",
			"https://example.com",
		))
	})

	t.Run("normalize url trims trailing slash and query", func(t *testing.T) {
		require.Equal(t, "https://example.com/path", normalizeVerifyMCPAuthCutoverURL("https://example.com/path/?a=1"))
		require.Equal(t, "https://example.com/path", normalizeVerifyMCPAuthCutoverURL("https://example.com/path/"))
		require.True(t, containsFold([]string{"Read", "Write"}, "write"))
		require.False(t, containsFold([]string{"Read", "Write"}, "follow"))
	})
}

func TestExecuteVerifyMCPAuthCutover_Errors(t *testing.T) {
	_, err := executeVerifyMCPAuthCutover(context.Background(), nil, "https://example.com", []string{"agent-a", "agent-b"}, false)
	require.ErrorContains(t, err, "http client is required")

	client := newVerifyMCPAuthCutoverHTTPClient(1)
	_, err = executeVerifyMCPAuthCutover(context.Background(), client, "", []string{"agent-a", "agent-b"}, false)
	require.ErrorContains(t, err, "--base-url is required")

	_, err = executeVerifyMCPAuthCutover(context.Background(), client, "https://example.com", []string{"agent-a"}, false)
	require.ErrorContains(t, err, "at least two actors are required")

	t.Run("missing registration endpoint errors", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/.well-known/oauth-authorization-server":
				_ = json.NewEncoder(w).Encode(map[string]any{
					"issuer":                serverURLFromRequest(r),
					"grant_types_supported": []string{"authorization_code", "refresh_token"},
				})
			default:
				http.NotFound(w, r)
			}
		}))
		defer server.Close()

		_, err := executeVerifyMCPAuthCutover(context.Background(), server.Client(), server.URL, []string{"agent-a", "agent-b"}, false)
		require.ErrorContains(t, err, "missing registration_endpoint")
	})

	t.Run("protected resource mismatch errors", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/.well-known/oauth-authorization-server":
				_ = json.NewEncoder(w).Encode(map[string]any{
					"issuer":                serverURLFromRequest(r),
					"registration_endpoint": serverURLFromRequest(r) + "/oauth/register",
					"grant_types_supported": []string{"authorization_code", "refresh_token"},
				})
			case "/.well-known/oauth-protected-resource/mcp/agent-a":
				_ = json.NewEncoder(w).Encode(map[string]any{
					"resource":              serverURLFromRequest(r) + "/mcp/not-agent-a",
					"authorization_servers": []string{serverURLFromRequest(r)},
				})
			case "/.well-known/oauth-protected-resource/mcp/agent-b":
				_ = json.NewEncoder(w).Encode(map[string]any{
					"resource":              serverURLFromRequest(r) + "/mcp/agent-b",
					"authorization_servers": []string{serverURLFromRequest(r)},
				})
			default:
				http.NotFound(w, r)
			}
		}))
		defer server.Close()

		_, err := executeVerifyMCPAuthCutover(context.Background(), server.Client(), server.URL, []string{"agent-a", "agent-b"}, false)
		require.ErrorContains(t, err, "expected")
	})
}

func TestExecuteVerifyMCPAuthCutoverWriteChecks_Errors(t *testing.T) {
	t.Run("public registration returning a client secret errors", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/oauth/register":
				w.WriteHeader(http.StatusCreated)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"client_id":                  "client-public",
					"client_secret":              "unexpected",
					"client_class":               "cli",
					"grant_types":                []string{"authorization_code", "refresh_token"},
					"token_endpoint_auth_method": "none",
				})
			case "/api/v1/apps":
				w.WriteHeader(http.StatusUnprocessableEntity)
				_, _ = w.Write([]byte(`{"error":"agent_username is not supported for public registration"}`))
			default:
				http.NotFound(w, r)
			}
		}))
		defer server.Close()

		err := executeVerifyMCPAuthCutoverWriteChecks(context.Background(), server.Client(), server.URL, server.URL+"/oauth/register", "agent-a")
		require.ErrorContains(t, err, "unexpectedly returned client_secret")
	})

	t.Run("compat endpoint still accepting agent username errors", func(t *testing.T) {
		registerCalls := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/oauth/register":
				registerCalls++
				if registerCalls == 1 {
					w.WriteHeader(http.StatusCreated)
					_ = json.NewEncoder(w).Encode(map[string]any{
						"client_id":                  "client-public",
						"client_class":               "cli",
						"grant_types":                []string{"authorization_code", "refresh_token"},
						"token_endpoint_auth_method": "none",
					})
					return
				}
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"error":             "invalid_client_metadata",
					"error_description": "invalid grant_types",
				})
			case "/api/v1/apps":
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"client_id":"accepted"}`))
			default:
				http.NotFound(w, r)
			}
		}))
		defer server.Close()

		err := executeVerifyMCPAuthCutoverWriteChecks(context.Background(), server.Client(), server.URL, server.URL+"/oauth/register", "agent-a")
		require.ErrorContains(t, err, "still accepted removed agent_username input")
	})

	t.Run("client credentials rejection requires expected oauth error", func(t *testing.T) {
		registerCalls := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/oauth/register":
				registerCalls++
				if registerCalls == 1 {
					w.WriteHeader(http.StatusCreated)
					_ = json.NewEncoder(w).Encode(map[string]any{
						"client_id":                  "client-public",
						"client_class":               "cli",
						"grant_types":                []string{"authorization_code", "refresh_token"},
						"token_endpoint_auth_method": "none",
					})
					return
				}
				http.Error(w, "upstream boom", http.StatusTooManyRequests)
			case "/api/v1/apps":
				w.WriteHeader(http.StatusUnprocessableEntity)
				_, _ = w.Write([]byte(`{"error":"agent_username is not supported for public registration","code":"unprocessable_entity"}`))
			default:
				http.NotFound(w, r)
			}
		}))
		defer server.Close()

		err := executeVerifyMCPAuthCutoverWriteChecks(context.Background(), server.Client(), server.URL, server.URL+"/oauth/register", "agent-a")
		require.ErrorContains(t, err, "expected 400 invalid_client_metadata")
	})

	t.Run("agent username rejection requires expected validation error", func(t *testing.T) {
		registerCalls := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/oauth/register":
				registerCalls++
				if registerCalls == 1 {
					w.WriteHeader(http.StatusCreated)
					_ = json.NewEncoder(w).Encode(map[string]any{
						"client_id":                  "client-public",
						"client_class":               "cli",
						"grant_types":                []string{"authorization_code", "refresh_token"},
						"token_endpoint_auth_method": "none",
					})
					return
				}
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"error":             "invalid_client_metadata",
					"error_description": "client_credentials is not supported for public registration",
				})
			case "/api/v1/apps":
				http.Error(w, "upstream boom", http.StatusInternalServerError)
			default:
				http.NotFound(w, r)
			}
		}))
		defer server.Close()

		err := executeVerifyMCPAuthCutoverWriteChecks(context.Background(), server.Client(), server.URL, server.URL+"/oauth/register", "agent-a")
		require.ErrorContains(t, err, "expected 422 validation error")
	})
}

func TestVerifyMCPAuthCutoverHTTPHelpers_Errors(t *testing.T) {
	client := newVerifyMCPAuthCutoverHTTPClient(1)

	_, _, err := verifyMCPAuthCutoverDoRequest(context.Background(), client, http.MethodGet, "://bad-url", "application/json", nil)
	require.Error(t, err)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer server.Close()

	var out map[string]any
	err = verifyMCPAuthCutoverGetJSON(context.Background(), server.Client(), server.URL, &out)
	require.ErrorContains(t, err, "returned 500")
}

func TestVerifyMCPAuthCutoverRejectionHelpers(t *testing.T) {
	t.Run("client credentials rejection accepts expected oauth error", func(t *testing.T) {
		err := verifyMCPAuthCutoverExpectClientCredentialsRejection(http.StatusBadRequest, []byte(`{"error":"invalid_client_metadata","error_description":"client_credentials is not supported for public registration"}`))
		require.NoError(t, err)
	})

	t.Run("client credentials rejection fails for malformed oauth error", func(t *testing.T) {
		err := verifyMCPAuthCutoverExpectClientCredentialsRejection(http.StatusBadRequest, []byte(`{"error":"invalid_client_metadata","error_description":"client_name is required"}`))
		require.ErrorContains(t, err, "mention grant_types or client_credentials")
	})

	t.Run("agent username rejection accepts expected validation error", func(t *testing.T) {
		err := verifyMCPAuthCutoverExpectRemovedAgentUsernameRejection(http.StatusUnprocessableEntity, []byte(`{"error":"agent_username is not supported for public registration","code":"unprocessable_entity"}`))
		require.NoError(t, err)
	})

	t.Run("agent username rejection fails for non-json body", func(t *testing.T) {
		err := verifyMCPAuthCutoverExpectRemovedAgentUsernameRejection(http.StatusUnprocessableEntity, []byte(`upstream boom`))
		require.ErrorContains(t, err, "expected JSON validation error")
	})
}

func serverURLFromRequest(r *http.Request) string {
	return "http://" + r.Host
}

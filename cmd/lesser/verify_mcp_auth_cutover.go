package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
)

type verifyMCPAuthCutoverAuthServerMetadata struct {
	Issuer               string   `json:"issuer"`
	RegistrationEndpoint string   `json:"registration_endpoint"`
	GrantTypesSupported  []string `json:"grant_types_supported"`
}

type verifyMCPAuthCutoverProtectedResourceMetadata struct {
	Resource             string   `json:"resource"`
	AuthorizationServers []string `json:"authorization_servers"`
}

type verifyMCPAuthCutoverSummary struct {
	BaseURL              string
	Actors               []string
	AuthorizationServer  string
	RegistrationEndpoint string
	ProtectedResources   []string
	WriteChecks          bool
}

type verifyMCPAuthCutoverErrorResponse struct {
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
	Code             string `json:"code"`
}

func runVerifyMCPAuthCutover(argv []string) error {
	fs := flag.NewFlagSet("lesser verify mcp-auth-cutover", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var baseURL string
	var actorsCSV string
	var timeoutSeconds int
	var allowWrite bool

	fs.StringVar(&baseURL, "base-url", "", "public Lesser base url (required)")
	fs.StringVar(&actorsCSV, "actors", "", "comma-separated actor usernames to verify (required; at least two)")
	fs.IntVar(&timeoutSeconds, "timeout-seconds", 15, "per-request timeout seconds")
	fs.BoolVar(&allowWrite, "allow-write", false, "run write-path registration checks against a disposable environment")

	if err := fs.Parse(argv); err != nil {
		return err
	}

	actors, err := parseVerifyMCPAuthCutoverActors(actorsCSV)
	if err != nil {
		return err
	}

	client := newVerifyMCPAuthCutoverHTTPClient(timeoutSeconds)
	summary, err := executeVerifyMCPAuthCutover(context.Background(), client, baseURL, actors, allowWrite)
	if err != nil {
		return err
	}

	fmt.Println("verify mcp-auth-cutover complete")
	fmt.Printf("base_url: %s\n", summary.BaseURL)
	fmt.Printf("actors_checked: %d\n", len(summary.Actors))
	fmt.Printf("authorization_server: %s\n", summary.AuthorizationServer)
	fmt.Printf("registration_endpoint: %s\n", summary.RegistrationEndpoint)
	fmt.Printf("protected_resources_checked: %d\n", len(summary.ProtectedResources))
	fmt.Printf("write_checks: %t\n", summary.WriteChecks)
	return nil
}

func parseVerifyMCPAuthCutoverActors(raw string) ([]string, error) {
	seen := map[string]struct{}{}
	actors := make([]string, 0, 2)
	for _, item := range strings.Split(raw, ",") {
		actor := strings.TrimSpace(item)
		if actor == "" {
			continue
		}
		if err := common.ValidateUsernameParamID(actor); err != nil {
			return nil, fmt.Errorf("invalid actor username %q", actor)
		}
		if _, ok := seen[actor]; ok {
			continue
		}
		seen[actor] = struct{}{}
		actors = append(actors, actor)
	}
	if len(actors) < 2 {
		return nil, errors.New("--actors must contain at least two unique actor usernames")
	}
	return actors, nil
}

func newVerifyMCPAuthCutoverHTTPClient(timeoutSeconds int) *http.Client {
	timeout := time.Duration(timeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = defaultHTTPTimeout
	}

	return &http.Client{
		Timeout: timeout,
	}
}

func executeVerifyMCPAuthCutover(ctx context.Context, client *http.Client, baseURL string, actors []string, allowWrite bool) (verifyMCPAuthCutoverSummary, error) {
	summary := verifyMCPAuthCutoverSummary{
		BaseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		Actors:  append([]string(nil), actors...),
	}
	if client == nil {
		return summary, errors.New("http client is required")
	}
	if summary.BaseURL == "" {
		return summary, errors.New("--base-url is required")
	}
	if len(actors) < 2 {
		return summary, errors.New("at least two actors are required")
	}

	var authMeta verifyMCPAuthCutoverAuthServerMetadata
	if err := verifyMCPAuthCutoverGetJSON(ctx, client, summary.BaseURL+"/.well-known/oauth-authorization-server", &authMeta); err != nil {
		return summary, fmt.Errorf("load authorization server metadata: %w", err)
	}
	if strings.TrimSpace(authMeta.Issuer) == "" {
		return summary, errors.New("authorization server metadata missing issuer")
	}
	if strings.TrimSpace(authMeta.RegistrationEndpoint) == "" {
		return summary, errors.New("authorization server metadata missing registration_endpoint")
	}
	if containsFold(authMeta.GrantTypesSupported, auth.GrantTypeClientCredentials) {
		return summary, errors.New("authorization server metadata still advertises client_credentials")
	}

	registrationEndpoint, err := resolveVerifyMCPAuthCutoverEndpoint(summary.BaseURL, authMeta.RegistrationEndpoint)
	if err != nil {
		return summary, fmt.Errorf("resolve registration endpoint: %w", err)
	}

	summary.AuthorizationServer = strings.TrimRight(strings.TrimSpace(authMeta.Issuer), "/")
	summary.RegistrationEndpoint = registrationEndpoint

	seenResources := map[string]string{}
	for _, actor := range actors {
		var protectedMeta verifyMCPAuthCutoverProtectedResourceMetadata
		endpoint := summary.BaseURL + "/.well-known/oauth-protected-resource/mcp/" + url.PathEscape(actor)
		if err := verifyMCPAuthCutoverGetJSON(ctx, client, endpoint, &protectedMeta); err != nil {
			return summary, fmt.Errorf("load protected-resource metadata for %q: %w", actor, err)
		}

		expectedResource := summary.BaseURL + "/mcp/" + actor
		resource := strings.TrimRight(strings.TrimSpace(protectedMeta.Resource), "/")
		if resource != expectedResource {
			return summary, fmt.Errorf("protected-resource metadata for %q returned %q (expected %q)", actor, resource, expectedResource)
		}
		if len(protectedMeta.AuthorizationServers) == 0 {
			return summary, fmt.Errorf("protected-resource metadata for %q is missing authorization_servers", actor)
		}
		if !protectedResourcePointsAtAuthServer(protectedMeta.AuthorizationServers, summary.AuthorizationServer, summary.BaseURL) {
			return summary, fmt.Errorf("protected-resource metadata for %q does not point at the published authorization server", actor)
		}
		if previousActor, exists := seenResources[resource]; exists && previousActor != actor {
			return summary, fmt.Errorf("protected-resource metadata reused %q for both %q and %q", resource, previousActor, actor)
		}
		seenResources[resource] = actor
		summary.ProtectedResources = append(summary.ProtectedResources, resource)
	}

	if allowWrite {
		if err := executeVerifyMCPAuthCutoverWriteChecks(ctx, client, summary.BaseURL, registrationEndpoint, actors[0]); err != nil {
			return summary, err
		}
		summary.WriteChecks = true
	}

	return summary, nil
}

func executeVerifyMCPAuthCutoverWriteChecks(ctx context.Context, client *http.Client, baseURL, registrationEndpoint, actor string) error {
	suffix := fmt.Sprintf("%d", time.Now().UTC().UnixNano())

	status, body, err := verifyMCPAuthCutoverJSONRequest(ctx, client, http.MethodPost, registrationEndpoint, apimodels.OAuthDynamicClientRegistrationRequest{
		ClientName:              "mcp-auth-cutover-verifier-" + suffix,
		RedirectURIs:            []string{"http://127.0.0.1:8787/callback"},
		Scope:                   "read write",
		TokenEndpointAuthMethod: "none",
	})
	if err != nil {
		return fmt.Errorf("register generic public client: %w", err)
	}
	if status != http.StatusCreated {
		return fmt.Errorf("register generic public client: expected 201, got %d (%s)", status, strings.TrimSpace(string(body)))
	}

	var registration apimodels.OAuthDynamicClientRegistrationResponse
	if err := json.Unmarshal(body, &registration); err != nil {
		return fmt.Errorf("decode dynamic registration response: %w", err)
	}
	var rawRegistration map[string]any
	if err := json.Unmarshal(body, &rawRegistration); err != nil {
		return fmt.Errorf("decode dynamic registration response map: %w", err)
	}
	if strings.TrimSpace(registration.ClientID) == "" {
		return errors.New("dynamic registration response missing client_id")
	}
	if strings.TrimSpace(registration.ClientSecret) != "" {
		return errors.New("public dynamic registration unexpectedly returned client_secret")
	}
	if strings.EqualFold(strings.TrimSpace(registration.ClientClass), auth.ClientClassAgent) {
		return errors.New("public dynamic registration unexpectedly returned client_class=agent")
	}
	if containsFold(registration.GrantTypes, auth.GrantTypeClientCredentials) {
		return errors.New("public dynamic registration unexpectedly returned client_credentials grant")
	}
	if _, ok := rawRegistration["agent_username"]; ok {
		return errors.New("dynamic registration response unexpectedly included agent_username")
	}

	status, body, err = verifyMCPAuthCutoverJSONRequest(ctx, client, http.MethodPost, registrationEndpoint, apimodels.OAuthDynamicClientRegistrationRequest{
		ClientName:              "mcp-auth-cutover-reject-client-credentials-" + suffix,
		RedirectURIs:            []string{"http://127.0.0.1:8787/callback"},
		GrantTypes:              []string{auth.GrantTypeClientCredentials},
		TokenEndpointAuthMethod: "none",
	})
	if err != nil {
		return fmt.Errorf("reject public client_credentials registration: %w", err)
	}
	if err := verifyMCPAuthCutoverExpectClientCredentialsRejection(status, body); err != nil {
		return fmt.Errorf("reject public client_credentials registration: %w", err)
	}

	form := url.Values{}
	form.Set("client_name", "mcp-auth-cutover-reject-agent-username-"+suffix)
	form.Set("redirect_uris", "https://example.com/callback")
	form.Set("scopes", "read write")
	form.Set("agent_username", actor)

	status, body, err = verifyMCPAuthCutoverFormRequest(ctx, client, http.MethodPost, baseURL+"/api/v1/apps", form)
	if err != nil {
		return fmt.Errorf("reject /api/v1/apps agent_username input: %w", err)
	}
	if err := verifyMCPAuthCutoverExpectRemovedAgentUsernameRejection(status, body); err != nil {
		return fmt.Errorf("reject /api/v1/apps agent_username input: %w", err)
	}

	return nil
}

func verifyMCPAuthCutoverExpectClientCredentialsRejection(status int, body []byte) error {
	trimmedBody := strings.TrimSpace(string(body))
	if status == http.StatusCreated {
		return fmt.Errorf("still accepted client_credentials (%d): %s", status, trimmedBody)
	}
	if status != http.StatusBadRequest {
		return fmt.Errorf("expected 400 invalid_client_metadata, got %d (%s)", status, trimmedBody)
	}

	var resp verifyMCPAuthCutoverErrorResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return fmt.Errorf("expected JSON invalid_client_metadata error, got %q", trimmedBody)
	}
	if !strings.EqualFold(strings.TrimSpace(resp.Error), "invalid_client_metadata") {
		return fmt.Errorf("expected error=invalid_client_metadata, got %q", strings.TrimSpace(resp.Error))
	}

	description := strings.ToLower(strings.TrimSpace(resp.ErrorDescription))
	if !strings.Contains(description, "grant_types") && !strings.Contains(description, auth.GrantTypeClientCredentials) {
		return fmt.Errorf("expected invalid_client_metadata description to mention grant_types or client_credentials, got %q", strings.TrimSpace(resp.ErrorDescription))
	}

	return nil
}

func verifyMCPAuthCutoverExpectRemovedAgentUsernameRejection(status int, body []byte) error {
	trimmedBody := strings.TrimSpace(string(body))
	if status == http.StatusOK || status == http.StatusCreated {
		return fmt.Errorf("still accepted removed agent_username input (%d): %s", status, trimmedBody)
	}
	if status != http.StatusUnprocessableEntity {
		return fmt.Errorf("expected 422 validation error, got %d (%s)", status, trimmedBody)
	}

	var resp verifyMCPAuthCutoverErrorResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return fmt.Errorf("expected JSON validation error, got %q", trimmedBody)
	}

	message := strings.ToLower(strings.TrimSpace(resp.Error))
	if !strings.Contains(message, "agent_username") || !strings.Contains(message, "not supported for public registration") {
		return fmt.Errorf("expected validation error mentioning removed agent_username input, got %q", strings.TrimSpace(resp.Error))
	}

	return nil
}

func verifyMCPAuthCutoverGetJSON(ctx context.Context, client *http.Client, endpoint string, out any) error {
	status, body, err := verifyMCPAuthCutoverDoRequest(ctx, client, http.MethodGet, endpoint, "application/json", nil)
	if err != nil {
		return err
	}
	if status >= http.StatusBadRequest {
		msg := strings.TrimSpace(string(body))
		if msg == "" {
			msg = http.StatusText(status)
		}
		return fmt.Errorf("%s returned %d: %s", endpoint, status, msg)
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("decode %s response: %w", endpoint, err)
	}
	return nil
}

func verifyMCPAuthCutoverJSONRequest(ctx context.Context, client *http.Client, method, endpoint string, payload any) (int, []byte, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return 0, nil, err
	}
	return verifyMCPAuthCutoverDoRequest(ctx, client, method, endpoint, "application/json", body)
}

func verifyMCPAuthCutoverFormRequest(ctx context.Context, client *http.Client, method, endpoint string, form url.Values) (int, []byte, error) {
	return verifyMCPAuthCutoverDoRequest(ctx, client, method, endpoint, "application/x-www-form-urlencoded", []byte(form.Encode()))
}

func verifyMCPAuthCutoverDoRequest(ctx context.Context, client *http.Client, method, endpoint, contentType string, body []byte) (int, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, method, endpoint, strings.NewReader(string(body)))
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Accept", "application/json")
	if strings.TrimSpace(contentType) != "" {
		req.Header.Set("Content-Type", contentType)
	}

	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
	if err != nil {
		return 0, nil, err
	}
	return resp.StatusCode, raw, nil
}

func resolveVerifyMCPAuthCutoverEndpoint(baseURL, endpoint string) (string, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return "", errors.New("endpoint is empty")
	}
	if strings.HasPrefix(endpoint, "/") {
		return baseURL + endpoint, nil
	}
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed == nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("endpoint %q is not an absolute URL", endpoint)
	}
	return endpoint, nil
}

func protectedResourcePointsAtAuthServer(authorizationServers []string, issuer, baseURL string) bool {
	issuer = normalizeVerifyMCPAuthCutoverURL(issuer)
	baseURL = normalizeVerifyMCPAuthCutoverURL(baseURL)
	metadataURL := normalizeVerifyMCPAuthCutoverURL(baseURL + "/.well-known/oauth-authorization-server")

	for _, item := range authorizationServers {
		normalized := normalizeVerifyMCPAuthCutoverURL(item)
		if normalized == "" {
			continue
		}
		if normalized == issuer || normalized == baseURL || normalized == metadataURL {
			return true
		}
	}
	return false
}

func normalizeVerifyMCPAuthCutoverURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed == nil {
		return strings.TrimRight(raw, "/")
	}
	parsed.Fragment = ""
	if parsed.RawPath == "" {
		parsed.RawPath = parsed.EscapedPath()
	}
	if parsed.RawQuery == "" {
		return strings.TrimRight(parsed.String(), "/")
	}
	return strings.TrimRight(parsed.Scheme+"://"+parsed.Host+parsed.Path, "/")
}

func containsFold(values []string, target string) bool {
	target = strings.TrimSpace(target)
	for _, item := range values {
		if strings.EqualFold(strings.TrimSpace(item), target) {
			return true
		}
	}
	return false
}

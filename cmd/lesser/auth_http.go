package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
)

const (
	defaultAuthClientName    = "lesser cli"
	defaultAuthRedirectURIs  = "urn:ietf:wg:oauth:2.0:oob"
	defaultHTTPTimeout       = 15 * time.Second
	defaultDevicePollBackoff = 5 * time.Second
)

func getOrCreateOAuthClientID(ctx context.Context, baseURL, scopes string, key []byte, flags *authFlags) (string, error) {
	if flags == nil {
		flags = &authFlags{}
	}

	// Prefer existing cached session's client_id (avoids registering an app each login).
	if existing, err := readAuthSession(baseURL, key); err == nil && existing != nil {
		if clientID := strings.TrimSpace(existing.ClientID); clientID != "" {
			flags.debugf("reusing existing client_id")
			return clientID, nil
		}
	}

	flags.debugf("registering oauth app via /api/v1/apps")
	clientID, err := registerOAuthApp(ctx, baseURL, scopes)
	if err != nil {
		return "", err
	}
	return clientID, nil
}

func registerOAuthApp(ctx context.Context, baseURL, scopes string) (string, error) {
	form := url.Values{}
	form.Set("client_name", defaultAuthClientName)
	form.Set("redirect_uris", defaultAuthRedirectURIs)
	form.Set("scopes", strings.TrimSpace(scopes))

	var resp apimodels.AppRegistrationResponse
	if err := doFormPOST(ctx, baseURL, "/api/v1/apps", form, &resp); err != nil {
		return "", err
	}
	clientID := strings.TrimSpace(resp.ClientID)
	if clientID == "" {
		return "", fmt.Errorf("app registration response is missing client_id")
	}
	return clientID, nil
}

func startDeviceAuthorization(ctx context.Context, baseURL, clientID, scopes string) (*apimodels.OAuthDeviceCodeResponse, error) {
	form := url.Values{}
	form.Set("client_id", strings.TrimSpace(clientID))
	if scope := strings.TrimSpace(scopes); scope != "" {
		form.Set("scope", scope)
	}

	var resp apimodels.OAuthDeviceCodeResponse
	if err := doFormPOST(ctx, baseURL, "/oauth/device/code", form, &resp); err != nil {
		return nil, err
	}
	if strings.TrimSpace(resp.DeviceCode) == "" || strings.TrimSpace(resp.UserCode) == "" {
		return nil, fmt.Errorf("device code response missing required fields")
	}
	return &resp, nil
}

func pollDeviceToken(ctx context.Context, baseURL, clientID, deviceCode string, interval time.Duration, ttl time.Duration, flags *authFlags) (*apimodels.OAuthTokenResponse, error) {
	if ttl <= 0 {
		ttl = 10 * time.Minute
	}

	deadline := time.Now().Add(ttl)
	nextInterval := interval
	if nextInterval <= 0 {
		nextInterval = 10 * time.Second
	}

	for {
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("device authorization timed out")
		}

		tokenResp, oauthErr, err := exchangeDeviceCodeForToken(ctx, baseURL, clientID, deviceCode)
		if err != nil {
			return nil, err
		}
		if tokenResp != nil {
			return tokenResp, nil
		}

		code := strings.ToLower(strings.TrimSpace(oauthErr.Error))
		if flags != nil {
			flags.debugf("device poll error=%s", code)
		}

		switch code {
		case "authorization_pending":
			// Keep waiting.
		case "slow_down":
			nextInterval += defaultDevicePollBackoff
		case "access_denied":
			return nil, fmt.Errorf("device authorization denied")
		case "expired_token":
			return nil, fmt.Errorf("device code expired")
		case "invalid_grant":
			return nil, fmt.Errorf("device authorization invalid")
		default:
			msg := strings.TrimSpace(oauthErr.ErrorDescription)
			if msg == "" {
				msg = "oauth error"
			}
			return nil, fmt.Errorf("%s (%s)", msg, code)
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(nextInterval):
		}
	}
}

func exchangeDeviceCodeForToken(ctx context.Context, baseURL, clientID, deviceCode string) (*apimodels.OAuthTokenResponse, *apimodels.OAuthErrorResponse, error) {
	form := url.Values{}
	form.Set("grant_type", "urn:ietf:params:oauth:grant-type:device_code")
	form.Set("device_code", strings.TrimSpace(deviceCode))
	form.Set("client_id", strings.TrimSpace(clientID))

	var resp apimodels.OAuthTokenResponse
	err := doFormPOST(ctx, baseURL, "/oauth/token", form, &resp)
	if err == nil {
		return &resp, nil, nil
	}

	var oauthErr *oauthHTTPError
	if errors.As(err, &oauthErr) {
		return nil, &oauthErr.OAuth, nil
	}

	return nil, nil, err
}

func refreshAccessToken(ctx context.Context, baseURL, clientID, refreshToken string) (*apimodels.OAuthTokenResponse, error) {
	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("client_id", strings.TrimSpace(clientID))
	form.Set("refresh_token", strings.TrimSpace(refreshToken))

	var resp apimodels.OAuthTokenResponse
	if err := doFormPOST(ctx, baseURL, "/oauth/token", form, &resp); err != nil {
		var oauthErr *oauthHTTPError
		if errors.As(err, &oauthErr) {
			code := strings.ToLower(strings.TrimSpace(oauthErr.OAuth.Error))
			if code == "invalid_grant" {
				return nil, fmt.Errorf("refresh token invalid; re-auth required")
			}
		}
		return nil, err
	}
	return &resp, nil
}

type verifyCredentialsResponse struct {
	Username string `json:"username"`
}

func resolveViewerAndScopes(ctx context.Context, baseURL, accessToken, scope string) (string, []string, error) {
	var out verifyCredentialsResponse
	if err := doGETJSON(ctx, baseURL, "/api/v1/accounts/verify_credentials", accessToken, &out); err != nil {
		return "", nil, err
	}

	username := strings.TrimSpace(out.Username)
	if username == "" {
		return "", nil, fmt.Errorf("verify_credentials response missing username")
	}

	scopes := strings.Fields(strings.TrimSpace(scope))
	if len(scopes) == 0 {
		scopes = nil
	}
	return username, scopes, nil
}

type oauthHTTPError struct {
	Status int
	OAuth  apimodels.OAuthErrorResponse
}

func (e *oauthHTTPError) Error() string {
	code := strings.TrimSpace(e.OAuth.Error)
	desc := strings.TrimSpace(e.OAuth.ErrorDescription)
	if desc == "" {
		desc = "oauth error"
	}
	if code == "" {
		return desc
	}
	return fmt.Sprintf("%s (%s)", desc, code)
}

func doFormPOST(ctx context.Context, baseURL, path string, form url.Values, out any) error {
	client := &http.Client{Timeout: defaultHTTPTimeout}
	endpoint := strings.TrimRight(baseURL, "/") + path

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
	if err != nil {
		return err
	}

	if resp.StatusCode >= 400 {
		var oauthErr apimodels.OAuthErrorResponse
		if jsonErr := json.Unmarshal(body, &oauthErr); jsonErr == nil && strings.TrimSpace(oauthErr.Error) != "" {
			return &oauthHTTPError{Status: resp.StatusCode, OAuth: oauthErr}
		}

		msg := strings.TrimSpace(string(body))
		if msg == "" {
			msg = http.StatusText(resp.StatusCode)
		}
		return fmt.Errorf("%s: %s", endpoint, msg)
	}

	if out == nil {
		return nil
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("decode %s response: %w", path, err)
	}
	return nil
}

func doGETJSON(ctx context.Context, baseURL, path, accessToken string, out any) error {
	client := &http.Client{Timeout: defaultHTTPTimeout}
	endpoint := strings.TrimRight(baseURL, "/") + path

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if token := strings.TrimSpace(accessToken); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
	if err != nil {
		return err
	}
	if resp.StatusCode >= 400 {
		msg := strings.TrimSpace(string(body))
		if msg == "" {
			msg = http.StatusText(resp.StatusCode)
		}
		return fmt.Errorf("%s: %s", endpoint, msg)
	}

	if out == nil {
		return nil
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("decode %s response: %w", path, err)
	}
	return nil
}

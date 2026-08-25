package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
)

const (
	loopbackCallbackPath      = "/oauth/callback"
	defaultLoopbackAuthTTL    = 10 * time.Minute
	loopbackHTMLResponseTitle = "lesser cli"
)

type openBrowserFunc func(targetURL string) error

var openBrowserFn openBrowserFunc = openBrowser

func runAuthLoopbackLogin(baseURL string, key []byte, flags *authFlags) error {
	if flags == nil {
		flags = &authFlags{}
	}

	ctx, cancel := context.WithTimeout(context.Background(), defaultLoopbackAuthTTL)
	defer cancel()

	state, err := randomURLSafeString(32)
	if err != nil {
		return err
	}
	codeVerifier, codeChallenge, err := generatePKCE()
	if err != nil {
		return err
	}

	callback, err := startLoopbackCallbackServer(state)
	if err != nil {
		return err
	}
	defer func() {
		_ = callback.Shutdown(context.Background())
	}()

	redirectURIs := strings.TrimSpace(defaultAuthRedirectURIs + "\n" + callback.RedirectURI)
	clientID, err := registerOAuthApp(ctx, baseURL, flags.Scopes, redirectURIs, defaultAuthClientClass)
	if err != nil {
		return err
	}

	authorizeURL, err := buildAuthorizationCodeAuthorizeURL(baseURL, clientID, callback.RedirectURI, flags.Scopes, state, codeChallenge)
	if err != nil {
		return err
	}

	fmt.Println("Complete login in your browser:")
	fmt.Println("  ", authorizeURL)
	if !flags.NoBrowser && openBrowserFn != nil {
		if err := openBrowserFn(authorizeURL); err != nil {
			flags.debugf("failed to open browser: %v", err)
		}
	}

	code, err := callback.WaitForCode(ctx)
	if err != nil {
		return err
	}

	tokenResp, err := exchangeAuthorizationCodeForToken(ctx, baseURL, clientID, code, callback.RedirectURI, codeVerifier)
	if err != nil {
		return err
	}

	username, scopes, err := resolveViewerAndScopes(ctx, baseURL, tokenResp.AccessToken, tokenResp.Scope)
	if err != nil {
		return err
	}

	session := &cliAuthSession{
		Version:      cliAuthSessionVersion,
		BaseURL:      baseURL,
		ClientID:     clientID,
		RefreshToken: tokenResp.RefreshToken,
		Username:     username,
		Scopes:       scopes,
		CreatedAt:    time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
	}
	if err := writeAuthSession(baseURL, key, session); err != nil {
		return err
	}

	fmt.Printf("Authenticated as @%s on %s\n", username, baseURL)
	return nil
}

type loopbackCallbackServer struct {
	RedirectURI string

	srv     *http.Server
	once    sync.Once
	resultC chan loopbackCallbackResult
}

type loopbackCallbackResult struct {
	Code             string
	OAuthError       string
	OAuthDescription string
}

func startLoopbackCallbackServer(expectedState string) (*loopbackCallbackServer, error) {
	expectedState = strings.TrimSpace(expectedState)
	if expectedState == "" {
		return nil, errors.New("missing expected state")
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}

	redirectURI := fmt.Sprintf("http://%s%s", ln.Addr().String(), loopbackCallbackPath)
	s := &loopbackCallbackServer{
		RedirectURI: redirectURI,
		resultC:     make(chan loopbackCallbackResult, 1),
	}

	mux := http.NewServeMux()
	mux.HandleFunc(loopbackCallbackPath, func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if strings.TrimSpace(q.Get("state")) != expectedState {
			http.Error(w, "invalid state", http.StatusBadRequest)
			return
		}

		if errCode := strings.TrimSpace(q.Get("error")); errCode != "" {
			s.once.Do(func() {
				s.resultC <- loopbackCallbackResult{
					OAuthError:       errCode,
					OAuthDescription: strings.TrimSpace(q.Get("error_description")),
				}
			})
			writeLoopbackHTML(w, loopbackHTMLResponseTitle, "Login cancelled", "You can close this window.")
			return
		}

		code := strings.TrimSpace(q.Get("code"))
		if code == "" {
			http.Error(w, "missing code", http.StatusBadRequest)
			return
		}

		s.once.Do(func() { s.resultC <- loopbackCallbackResult{Code: code} })
		writeLoopbackHTML(w, loopbackHTMLResponseTitle, "Login complete", "You can close this window and return to your terminal.")
	})

	s.srv = &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		_ = s.srv.Serve(ln)
	}()

	return s, nil
}

func (s *loopbackCallbackServer) WaitForCode(ctx context.Context) (string, error) {
	if s == nil {
		return "", errors.New("loopback server is nil")
	}

	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case res := <-s.resultC:
		if res.OAuthError != "" {
			msg := strings.TrimSpace(res.OAuthDescription)
			if msg == "" {
				msg = "authorization failed"
			}
			return "", fmt.Errorf("%s (%s)", msg, res.OAuthError)
		}
		return res.Code, nil
	}
}

func (s *loopbackCallbackServer) Shutdown(ctx context.Context) error {
	if s == nil || s.srv == nil {
		return nil
	}
	return s.srv.Shutdown(ctx)
}

func buildAuthorizationCodeAuthorizeURL(baseURL, clientID, redirectURI, scopes, state, codeChallenge string) (string, error) {
	base, err := url.Parse(strings.TrimRight(baseURL, "/") + "/oauth/authorize")
	if err != nil {
		return "", err
	}

	q := base.Query()
	q.Set("response_type", "code")
	q.Set("client_id", strings.TrimSpace(clientID))
	q.Set("redirect_uri", strings.TrimSpace(redirectURI))
	if strings.TrimSpace(scopes) != "" {
		q.Set("scope", strings.TrimSpace(scopes))
	}
	q.Set("state", strings.TrimSpace(state))
	q.Set("code_challenge", strings.TrimSpace(codeChallenge))
	q.Set("code_challenge_method", "S256")
	base.RawQuery = q.Encode()

	return base.String(), nil
}

func exchangeAuthorizationCodeForToken(ctx context.Context, baseURL, clientID, code, redirectURI, codeVerifier string) (*apimodels.OAuthTokenResponse, error) {
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", strings.TrimSpace(code))
	form.Set("client_id", strings.TrimSpace(clientID))
	form.Set("redirect_uri", strings.TrimSpace(redirectURI))
	form.Set("code_verifier", strings.TrimSpace(codeVerifier))

	var resp apimodels.OAuthTokenResponse
	if err := doFormPOST(ctx, baseURL, "/oauth/token", form, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func generatePKCE() (verifier string, challenge string, err error) {
	verifier, err = randomURLSafeString(64)
	if err != nil {
		return "", "", err
	}

	sum := sha256.Sum256([]byte(verifier))
	challenge = base64.RawURLEncoding.EncodeToString(sum[:])
	return verifier, challenge, nil
}

func randomURLSafeString(n int) (string, error) {
	if n <= 0 {
		return "", errors.New("invalid random string length")
	}

	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}

	return base64.RawURLEncoding.EncodeToString(b), nil
}

func openBrowser(targetURL string) error {
	targetURL = strings.TrimSpace(targetURL)
	if targetURL == "" {
		return errors.New("missing url")
	}

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", targetURL) //nolint:gosec // G204: fixed OS browser launcher receives the operator-approved URL as a separate argument; no shell is involved
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", targetURL) //nolint:gosec // G204: fixed OS browser launcher receives the operator-approved URL as a separate argument; no shell is involved
	default:
		cmd = exec.Command("xdg-open", targetURL) //nolint:gosec // G204: fixed OS browser launcher receives the operator-approved URL as a separate argument; no shell is involved
	}

	return cmd.Start()
}

func writeLoopbackHTML(w http.ResponseWriter, title, heading, body string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = fmt.Fprintf(w, "<!doctype html><html><head><title>%s</title></head><body><h2>%s</h2><p>%s</p></body></html>", htmlEscape(title), htmlEscape(heading), htmlEscape(body))
}

func htmlEscape(value string) string {
	value = strings.ReplaceAll(value, "&", "&amp;")
	value = strings.ReplaceAll(value, "<", "&lt;")
	value = strings.ReplaceAll(value, ">", "&gt;")
	value = strings.ReplaceAll(value, "\"", "&quot;")
	value = strings.ReplaceAll(value, "'", "&#39;")
	return value
}

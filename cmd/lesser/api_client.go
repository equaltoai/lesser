package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
)

type cliAPIClientOptions struct {
	MaxConcurrency int
	RPS            float64
	Burst          int

	Retries int
	Timeout time.Duration
}

type cliAPIClient struct {
	baseURL string
	key     []byte

	sessionMu sync.Mutex
	session   *cliAuthSession

	accessTokenMu        sync.Mutex
	accessToken          string
	accessTokenExpiresAt time.Time

	httpClient *http.Client
	limiter    *clientLimiter

	retries int

	nowFn   func() time.Time
	sleepFn func(time.Duration)
}

func newCLIAPIClient(baseURL string, key []byte, opts cliAPIClientOptions) (*cliAPIClient, error) {
	if baseURL == "" {
		return nil, fmt.Errorf("baseURL is empty")
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("auth key must be 32 bytes (got %d)", len(key))
	}

	if opts.MaxConcurrency <= 0 {
		opts.MaxConcurrency = 1
	}
	if opts.Burst <= 0 {
		opts.Burst = 1
	}
	if opts.Timeout <= 0 {
		opts.Timeout = 30 * time.Second
	}
	if opts.Retries < 0 {
		opts.Retries = 0
	}

	session, err := readAuthSession(baseURL, key)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, err
		}
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("not logged in to %s (run: lesser auth login --base-url %s)", baseURL, baseURL)
		}
		return nil, err
	}

	limiter, err := newClientLimiter(opts.MaxConcurrency, opts.RPS, opts.Burst)
	if err != nil {
		return nil, err
	}

	return &cliAPIClient{
		baseURL: baseURL,
		key:     key,
		session: session,
		httpClient: &http.Client{
			Timeout: opts.Timeout,
		},
		limiter: limiter,
		retries: opts.Retries,
		nowFn:   time.Now,
		sleepFn: time.Sleep,
	}, nil
}

func (c *cliAPIClient) Close() {
	if c == nil || c.limiter == nil {
		return
	}
	c.limiter.Close()
}

func (c *cliAPIClient) Request(ctx context.Context, method, path string, headers http.Header, body []byte) (int, http.Header, []byte, error) {
	if c == nil {
		return 0, nil, nil, fmt.Errorf("client is nil")
	}

	accessToken, err := c.getAccessToken(ctx)
	if err != nil {
		return 0, nil, nil, err
	}

	headers = cloneHeader(headers)
	if headers.Get("Authorization") == "" {
		headers.Set("Authorization", "Bearer "+accessToken)
	}

	status, respHeaders, respBody, err := c.doRequestWithRetries(ctx, method, path, headers, body)
	return status, respHeaders, respBody, err
}

func (c *cliAPIClient) getAccessToken(ctx context.Context) (string, error) {
	c.accessTokenMu.Lock()
	token := strings.TrimSpace(c.accessToken)
	expiresAt := c.accessTokenExpiresAt
	c.accessTokenMu.Unlock()

	now := c.nowFn().UTC()
	if token != "" && !expiresAt.IsZero() && now.Before(expiresAt.Add(-30*time.Second)) {
		return token, nil
	}

	c.sessionMu.Lock()
	session := c.session
	c.sessionMu.Unlock()
	if session == nil {
		return "", fmt.Errorf("missing auth session")
	}

	tokenResp, err := c.refreshTokens(ctx, session.ClientID, session.RefreshToken)
	if err != nil {
		return "", err
	}

	accessToken := strings.TrimSpace(tokenResp.AccessToken)
	if accessToken == "" {
		return "", fmt.Errorf("token refresh response missing access_token")
	}

	expiresIn := tokenResp.ExpiresIn
	if expiresIn <= 0 {
		expiresIn = 3600
	}

	c.accessTokenMu.Lock()
	c.accessToken = accessToken
	c.accessTokenExpiresAt = now.Add(time.Duration(expiresIn) * time.Second)
	c.accessTokenMu.Unlock()

	newRefresh := strings.TrimSpace(tokenResp.RefreshToken)
	if newRefresh != "" && newRefresh != session.RefreshToken {
		updated := *session
		updated.RefreshToken = newRefresh
		updated.UpdatedAt = now
		if err := writeAuthSession(c.baseURL, c.key, &updated); err != nil {
			return "", err
		}
		c.sessionMu.Lock()
		c.session = &updated
		c.sessionMu.Unlock()
	}

	return accessToken, nil
}

func (c *cliAPIClient) refreshTokens(ctx context.Context, clientID, refreshToken string) (*apimodels.OAuthTokenResponse, error) {
	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("client_id", strings.TrimSpace(clientID))
	form.Set("refresh_token", strings.TrimSpace(refreshToken))

	headers := http.Header{}
	headers.Set("Content-Type", "application/x-www-form-urlencoded")
	headers.Set("Accept", "application/json")

	status, _, body, err := c.doRequestWithRetries(ctx, http.MethodPost, "/oauth/token", headers, []byte(form.Encode()))
	if err != nil {
		return nil, err
	}

	if status >= 400 {
		var oauthErr apimodels.OAuthErrorResponse
		if jsonErr := json.Unmarshal(body, &oauthErr); jsonErr == nil && strings.TrimSpace(oauthErr.Error) != "" {
			code := strings.ToLower(strings.TrimSpace(oauthErr.Error))
			if code == "invalid_grant" {
				return nil, fmt.Errorf("refresh token invalid; re-auth required")
			}
			desc := strings.TrimSpace(oauthErr.ErrorDescription)
			if desc == "" {
				desc = "oauth error"
			}
			return nil, fmt.Errorf("%s (%s)", desc, code)
		}
		msg := strings.TrimSpace(string(body))
		if msg == "" {
			msg = http.StatusText(status)
		}
		return nil, fmt.Errorf("token refresh failed (%d): %s", status, msg)
	}

	var tokenResp apimodels.OAuthTokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("decode token refresh response: %w", err)
	}

	return &tokenResp, nil
}

func (c *cliAPIClient) doRequestWithRetries(ctx context.Context, method, path string, headers http.Header, body []byte) (int, http.Header, []byte, error) {
	if c == nil {
		return 0, nil, nil, fmt.Errorf("client is nil")
	}

	method = strings.ToUpper(strings.TrimSpace(method))
	if method == "" {
		return 0, nil, nil, fmt.Errorf("method is empty")
	}
	path = strings.TrimSpace(path)
	if path == "" {
		return 0, nil, nil, fmt.Errorf("path is empty")
	}
	if !strings.HasPrefix(path, "/") {
		return 0, nil, nil, fmt.Errorf("path must start with '/' (got %q)", path)
	}

	attempts := c.retries + 1
	cost := requestCost(method, path)

	var lastErr error
	for attempt := 0; attempt < attempts; attempt++ {
		if err := c.limiter.Acquire(ctx, cost); err != nil {
			return 0, nil, nil, err
		}

		status, respHeaders, respBody, err := c.doSingleRequest(ctx, method, path, headers, body)
		c.limiter.Release()

		if err == nil && status < 429 && status < 500 {
			return status, respHeaders, respBody, nil
		}

		if attempt == attempts-1 {
			if err != nil {
				return status, respHeaders, respBody, err
			}
			return status, respHeaders, respBody, nil
		}

		if err != nil {
			lastErr = err
			c.sleepFn(backoffDuration(attempt))
			continue
		}

		if status == http.StatusTooManyRequests {
			wait := retryAfterDuration(respHeaders.Get("Retry-After"), c.nowFn())
			if wait <= 0 {
				wait = 5 * time.Second
			}
			c.sleepFn(wait)
			continue
		}

		if status >= 500 {
			c.sleepFn(backoffDuration(attempt))
			continue
		}

		return status, respHeaders, respBody, nil
	}

	if lastErr != nil {
		return 0, nil, nil, lastErr
	}
	return 0, nil, nil, fmt.Errorf("request failed")
}

func (c *cliAPIClient) doSingleRequest(ctx context.Context, method, path string, headers http.Header, body []byte) (int, http.Header, []byte, error) {
	endpoint := strings.TrimRight(c.baseURL, "/") + path

	var bodyReader io.Reader
	if len(body) > 0 {
		bodyReader = bytes.NewReader(body)
	}

	req, err := http.NewRequestWithContext(ctx, method, endpoint, bodyReader)
	if err != nil {
		return 0, nil, nil, err
	}

	for k, values := range headers {
		for _, v := range values {
			req.Header.Add(k, v)
		}
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, nil, nil, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024))
	if err != nil {
		return resp.StatusCode, resp.Header, nil, err
	}

	return resp.StatusCode, resp.Header, data, nil
}

func cloneHeader(h http.Header) http.Header {
	if h == nil {
		return http.Header{}
	}
	out := make(http.Header, len(h))
	for k, values := range h {
		out[k] = append([]string(nil), values...)
	}
	return out
}

func requestCost(method, path string) int {
	cost := 1
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		cost++
	}
	pathLower := strings.ToLower(path)
	if strings.Contains(pathLower, "/search") || strings.Contains(pathLower, "/graphql") {
		cost++
	}
	if cost < 1 {
		return 1
	}
	if cost > 5 {
		return 5
	}
	return cost
}

func backoffDuration(attempt int) time.Duration {
	if attempt < 0 {
		return 0
	}
	base := 250 * time.Millisecond
	d := time.Duration(float64(base) * math.Pow(2, float64(attempt)))
	if d > 5*time.Second {
		d = 5 * time.Second
	}
	return d
}

func retryAfterDuration(header string, now time.Time) time.Duration {
	header = strings.TrimSpace(header)
	if header == "" {
		return 0
	}

	if seconds, err := strconv.Atoi(header); err == nil {
		if seconds <= 0 {
			return 0
		}
		return time.Duration(seconds) * time.Second
	}

	if t, err := http.ParseTime(header); err == nil {
		d := t.Sub(now)
		if d < 0 {
			return 0
		}
		return d
	}

	return 0
}

type clientLimiter struct {
	sem *semaphore
	tb  *tokenBucket
}

func newClientLimiter(maxConcurrency int, rps float64, burst int) (*clientLimiter, error) {
	sem := newSemaphore(maxConcurrency)

	var tb *tokenBucket
	if rps > 0 {
		b, err := newTokenBucket(rps, burst)
		if err != nil {
			return nil, err
		}
		tb = b
	}

	return &clientLimiter{
		sem: sem,
		tb:  tb,
	}, nil
}

func (l *clientLimiter) Acquire(ctx context.Context, cost int) error {
	if l == nil {
		return nil
	}
	if err := l.sem.Acquire(ctx); err != nil {
		return err
	}
	if l.tb == nil {
		return nil
	}
	if cost < 1 {
		cost = 1
	}
	if err := l.tb.Wait(ctx, cost); err != nil {
		l.sem.Release()
		return err
	}
	return nil
}

func (l *clientLimiter) Release() {
	if l == nil || l.sem == nil {
		return
	}
	l.sem.Release()
}

func (l *clientLimiter) Close() {
	if l == nil || l.tb == nil {
		return
	}
	l.tb.Stop()
}

type semaphore struct {
	ch chan struct{}
}

func newSemaphore(n int) *semaphore {
	if n <= 0 {
		n = 1
	}
	return &semaphore{ch: make(chan struct{}, n)}
}

func (s *semaphore) Acquire(ctx context.Context) error {
	select {
	case s.ch <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *semaphore) Release() {
	select {
	case <-s.ch:
	default:
	}
}

type tokenBucket struct {
	tokens chan struct{}
	stop   chan struct{}
}

func newTokenBucket(rps float64, burst int) (*tokenBucket, error) {
	if rps <= 0 {
		return nil, fmt.Errorf("rps must be > 0")
	}
	if burst <= 0 {
		burst = 1
	}

	interval := time.Duration(float64(time.Second) / rps)
	if interval <= 0 {
		interval = time.Millisecond
	}

	tb := &tokenBucket{
		tokens: make(chan struct{}, burst),
		stop:   make(chan struct{}),
	}

	for i := 0; i < burst; i++ {
		tb.tokens <- struct{}{}
	}

	ticker := time.NewTicker(interval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-tb.stop:
				return
			case <-ticker.C:
				select {
				case tb.tokens <- struct{}{}:
				default:
				}
			}
		}
	}()

	return tb, nil
}

func (t *tokenBucket) Stop() {
	if t == nil {
		return
	}
	select {
	case <-t.stop:
	default:
		close(t.stop)
	}
}

func (t *tokenBucket) Wait(ctx context.Context, cost int) error {
	if t == nil {
		return nil
	}
	if cost < 1 {
		cost = 1
	}

	for i := 0; i < cost; i++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.tokens:
		}
	}
	return nil
}

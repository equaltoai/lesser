package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/deploy/naming"
)

type verifyUnresolvedRemoteParentConfig struct {
	BaseURL        string
	Stage          string
	Token          string
	ParentURL      string
	Visibility     string
	Expected       string
	Content        string
	TimeoutSeconds int
}

type verifyUnresolvedRemoteParentSummary struct {
	BaseURL                  string
	Stage                    string
	ParentURL                string
	Visibility               string
	Expected                 string
	Classification           string
	HTTPStatus               int
	CreatedStatusID          string
	CanonicalObjectURL       string
	CanonicalFetchHTTPStatus int
	ErrorCode                string
}

type verifyUnresolvedRemoteParentErrorResponse struct {
	Error            string `json:"error"`
	Code             string `json:"code"`
	ErrorDescription string `json:"error_description"`
}

type verifyUnresolvedRemoteParentCreateResponse struct {
	ID  string `json:"id"`
	URI string `json:"uri"`
}

var (
	newVerifyUnresolvedRemoteParentClientFn = newVerifyUnresolvedRemoteParentHTTPClient
	executeVerifyUnresolvedRemoteParentFn   = executeVerifyUnresolvedRemoteParent
	verifyUnresolvedRemoteParentNowFn       = time.Now
)

func runVerifyUnresolvedRemoteParent(argv []string) error {
	fs := flag.NewFlagSet("lesser verify unresolved-remote-parent", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var cfg verifyUnresolvedRemoteParentConfig
	fs.StringVar(&cfg.BaseURL, "base-url", "", "target Lesser base url (required; dev/staging only)")
	fs.StringVar(&cfg.Stage, "stage", valueDev, "target stage (dev|staging; live is blocked)")
	fs.StringVar(&cfg.Token, "token", "", "bearer token used to call POST /api/v1/statuses (required)")
	fs.StringVar(&cfg.ParentURL, "parent-url", "", "reply parent reference to probe (required)")
	fs.StringVar(&cfg.Visibility, "visibility", "public", "status visibility to exercise (public|unlisted|private)")
	fs.StringVar(&cfg.Expected, "expect", "success", "expected outcome (success|bad-request|timeout|unavailable|unusable)")
	fs.StringVar(&cfg.Content, "content", "", "explicit status text override")
	fs.IntVar(&cfg.TimeoutSeconds, "timeout-seconds", 15, "per-request timeout seconds")

	if err := fs.Parse(argv); err != nil {
		return err
	}
	if err := validateVerifyUnresolvedRemoteParentConfig(cfg); err != nil {
		return err
	}
	cfg.Content = resolveVerifyUnresolvedRemoteParentContent(cfg.Content, verifyUnresolvedRemoteParentNowFn().UTC().Format(time.RFC3339))

	client := newVerifyUnresolvedRemoteParentClientFn(cfg.TimeoutSeconds)
	summary, err := executeVerifyUnresolvedRemoteParentFn(context.Background(), client, cfg)
	if err != nil {
		return err
	}

	fmt.Println("verify unresolved-remote-parent complete")
	fmt.Printf("base_url: %s\n", summary.BaseURL)
	fmt.Printf("stage: %s\n", summary.Stage)
	fmt.Printf("parent_url: %s\n", summary.ParentURL)
	fmt.Printf("visibility: %s\n", summary.Visibility)
	fmt.Printf("expected: %s\n", summary.Expected)
	fmt.Printf("classification: %s\n", summary.Classification)
	fmt.Printf("http_status: %d\n", summary.HTTPStatus)
	if summary.CreatedStatusID != "" {
		fmt.Printf("created_status_id: %s\n", summary.CreatedStatusID)
	}
	if summary.CanonicalObjectURL != "" {
		fmt.Printf("canonical_object_url: %s\n", summary.CanonicalObjectURL)
	}
	if summary.CanonicalFetchHTTPStatus != 0 {
		fmt.Printf("canonical_fetch_http_status: %d\n", summary.CanonicalFetchHTTPStatus)
	}
	if summary.ErrorCode != "" {
		fmt.Printf("error_code: %s\n", summary.ErrorCode)
	}
	return nil
}

func validateVerifyUnresolvedRemoteParentConfig(cfg verifyUnresolvedRemoteParentConfig) error {
	cfg.BaseURL = strings.TrimSpace(cfg.BaseURL)
	cfg.Token = strings.TrimSpace(cfg.Token)
	cfg.ParentURL = strings.TrimSpace(cfg.ParentURL)
	cfg.Visibility = strings.ToLower(strings.TrimSpace(cfg.Visibility))
	cfg.Expected = strings.ToLower(strings.TrimSpace(cfg.Expected))

	if cfg.BaseURL == "" {
		return fmt.Errorf("--base-url is required")
	}
	if cfg.Token == "" {
		return fmt.Errorf("--token is required")
	}
	if cfg.ParentURL == "" {
		return fmt.Errorf("--parent-url is required")
	}
	if err := validateVerifyUnresolvedRemoteParentTarget(cfg.Stage, cfg.BaseURL); err != nil {
		return err
	}
	if cfg.Visibility == "direct" {
		return fmt.Errorf("--visibility=direct is not supported; direct replies remain conversations-owned")
	}
	if !containsFold([]string{"public", "unlisted", "private"}, cfg.Visibility) {
		return fmt.Errorf("--visibility must be one of public, unlisted, or private")
	}
	if !containsFold([]string{"success", "bad-request", "timeout", "unavailable", "unusable"}, cfg.Expected) {
		return fmt.Errorf("--expect must be one of success, bad-request, timeout, unavailable, or unusable")
	}
	if cfg.TimeoutSeconds <= 0 {
		return fmt.Errorf("--timeout-seconds must be greater than zero")
	}
	if !strings.EqualFold(cfg.Expected, "bad-request") {
		if err := common.ValidateURL(cfg.ParentURL, "parent_url"); err != nil {
			return err
		}
	}
	return nil
}

func validateVerifyUnresolvedRemoteParentTarget(stage, baseURL string) error {
	if naming.IsLiveEnvironment(stage) {
		return fmt.Errorf("verify unresolved-remote-parent writes synthetic data and must not run against live")
	}
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || parsed == nil || parsed.Scheme == "" || parsed.Host == "" {
		return fmt.Errorf("--base-url must be an absolute URL")
	}

	host := strings.ToLower(strings.TrimSpace(parsed.Hostname()))
	switch strings.ToLower(strings.TrimSpace(stage)) {
	case valueDev:
		if !strings.HasPrefix(host, "dev.") {
			return fmt.Errorf("verify unresolved-remote-parent dev targets must use a dev subdomain base url")
		}
	case valueStaging:
		if !strings.HasPrefix(host, "staging.") {
			return fmt.Errorf("verify unresolved-remote-parent staging targets must use a staging subdomain base url")
		}
	default:
		return fmt.Errorf("--stage must be dev or staging")
	}
	return nil
}

func newVerifyUnresolvedRemoteParentHTTPClient(timeoutSeconds int) *http.Client {
	timeout := time.Duration(timeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = defaultHTTPTimeout
	}
	return &http.Client{Timeout: timeout}
}

func executeVerifyUnresolvedRemoteParent(ctx context.Context, client *http.Client, cfg verifyUnresolvedRemoteParentConfig) (verifyUnresolvedRemoteParentSummary, error) {
	endpoint, err := resolveVerifyMCPAuthCutoverEndpoint(cfg.BaseURL, "/api/v1/statuses")
	if err != nil {
		return verifyUnresolvedRemoteParentSummary{}, err
	}

	payload := map[string]any{
		"status":         cfg.Content,
		"visibility":     cfg.Visibility,
		"sensitive":      false,
		"in_reply_to_id": cfg.ParentURL,
	}
	status, body, err := verifyUnresolvedRemoteParentJSONRequest(ctx, client, http.MethodPost, endpoint, strings.TrimSpace(cfg.Token), payload)
	if err != nil {
		return verifyUnresolvedRemoteParentSummary{}, err
	}

	summary := verifyUnresolvedRemoteParentSummary{
		BaseURL:        strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/"),
		Stage:          strings.ToLower(strings.TrimSpace(cfg.Stage)),
		ParentURL:      strings.TrimSpace(cfg.ParentURL),
		Visibility:     strings.ToLower(strings.TrimSpace(cfg.Visibility)),
		Expected:       strings.ToLower(strings.TrimSpace(cfg.Expected)),
		Classification: classifyVerifyUnresolvedRemoteParentStatus(status),
		HTTPStatus:     status,
	}
	if errResp, ok := decodeVerifyUnresolvedRemoteParentError(body); ok {
		summary.ErrorCode = firstNonEmptyString(errResp.Code, errResp.Error)
	}

	if summary.Classification != summary.Expected {
		return summary, fmt.Errorf("expected %s but got %s (%d): %s", summary.Expected, summary.Classification, status, verifyUnresolvedRemoteParentBodyMessage(body))
	}

	if summary.Classification != "success" {
		return summary, nil
	}

	var created verifyUnresolvedRemoteParentCreateResponse
	if err := json.Unmarshal(body, &created); err != nil {
		return summary, fmt.Errorf("decode create-status response: %w", err)
	}
	if strings.TrimSpace(created.ID) == "" {
		return summary, fmt.Errorf("create-status response missing id")
	}
	summary.CreatedStatusID = strings.TrimSpace(created.ID)

	canonicalObjectURL := strings.TrimSpace(created.URI)
	if canonicalObjectURL == "" {
		return summary, fmt.Errorf("create-status response missing canonical uri")
	}
	summary.CanonicalObjectURL = canonicalObjectURL

	fetchStatus, fetchBody, err := verifyUnresolvedRemoteParentFetchCanonicalObject(ctx, client, canonicalObjectURL)
	if err != nil {
		return summary, err
	}
	summary.CanonicalFetchHTTPStatus = fetchStatus
	if fetchStatus != http.StatusOK {
		return summary, fmt.Errorf("canonical object fetch returned %d: %s", fetchStatus, verifyUnresolvedRemoteParentBodyMessage(fetchBody))
	}

	var fetched struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(fetchBody, &fetched); err != nil {
		return summary, fmt.Errorf("decode canonical object response: %w", err)
	}
	if strings.TrimSpace(fetched.ID) != canonicalObjectURL {
		return summary, fmt.Errorf("canonical object fetch returned mismatched id %q", strings.TrimSpace(fetched.ID))
	}

	return summary, nil
}

func verifyUnresolvedRemoteParentJSONRequest(ctx context.Context, client *http.Client, method, endpoint, token string, payload any) (int, []byte, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return 0, nil, err
	}
	return verifyUnresolvedRemoteParentDoRequest(ctx, client, method, endpoint, token, body)
}

func verifyUnresolvedRemoteParentDoRequest(ctx context.Context, client *http.Client, method, endpoint, token string, body []byte) (int, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, method, endpoint, bytes.NewReader(body))
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	if strings.TrimSpace(token) != "" {
		req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(token))
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

func verifyUnresolvedRemoteParentFetchCanonicalObject(ctx context.Context, client *http.Client, endpoint string) (int, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Accept", "application/activity+json")

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

func classifyVerifyUnresolvedRemoteParentStatus(status int) string {
	switch {
	case status >= 200 && status < 300:
		return "success"
	case status == http.StatusBadRequest:
		return "bad-request"
	case status == http.StatusRequestTimeout:
		return "timeout"
	case status == http.StatusServiceUnavailable:
		return "unavailable"
	case status == http.StatusUnprocessableEntity:
		return "unusable"
	default:
		return fmt.Sprintf("unexpected-%d", status)
	}
}

func decodeVerifyUnresolvedRemoteParentError(body []byte) (verifyUnresolvedRemoteParentErrorResponse, bool) {
	if len(body) == 0 {
		return verifyUnresolvedRemoteParentErrorResponse{}, false
	}
	var out verifyUnresolvedRemoteParentErrorResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return verifyUnresolvedRemoteParentErrorResponse{}, false
	}
	if strings.TrimSpace(out.Error) == "" && strings.TrimSpace(out.Code) == "" && strings.TrimSpace(out.ErrorDescription) == "" {
		return verifyUnresolvedRemoteParentErrorResponse{}, false
	}
	return out, true
}

func verifyUnresolvedRemoteParentBodyMessage(body []byte) string {
	if errResp, ok := decodeVerifyUnresolvedRemoteParentError(body); ok {
		return firstNonEmptyString(errResp.ErrorDescription, errResp.Error, errResp.Code)
	}
	message := strings.TrimSpace(string(body))
	if message == "" {
		return "empty response body"
	}
	return message
}

func resolveVerifyUnresolvedRemoteParentContent(content, suffix string) string {
	content = strings.TrimSpace(content)
	if content != "" {
		return content
	}
	return fmt.Sprintf("verify unresolved remote parent %s", strings.TrimSpace(suffix))
}

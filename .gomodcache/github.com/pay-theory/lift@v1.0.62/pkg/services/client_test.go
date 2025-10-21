package services

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"sync"
	"testing"
	"time"
)

func TestContainsMatchesMidString(t *testing.T) {
	input := "temporary failure: connection timeout at proxy"
	if !contains(input, "timeout") {
		t.Fatalf("expected substring match for \"timeout\" in %q", input)
	}
}

type retryRecordingClient struct {
	mu       sync.Mutex
	attempts int
	bodies   [][]byte
}

func (c *retryRecordingClient) Do(req *http.Request) (*http.Response, error) {
	var bodyCopy []byte
	if req.Body != nil {
		data, err := io.ReadAll(req.Body)
		if err != nil {
			return nil, err
		}
		bodyCopy = append(bodyCopy, data...)
		_ = req.Body.Close()
	}

	c.mu.Lock()
	c.bodies = append(c.bodies, bodyCopy)
	attempt := c.attempts
	c.attempts++
	c.mu.Unlock()

	if attempt == 0 {
		return nil, fmt.Errorf("transient timeout")
	}

	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewReader([]byte(`{"ok":true}`))),
		Header:     make(http.Header),
	}, nil
}

func TestExecuteRequestRebuildsBodyAcrossRetries(t *testing.T) {
	httpClient := &retryRecordingClient{}

	client := &ServiceClient{
		httpClient: httpClient,
		retryPolicy: &RetryPolicy{
			MaxRetries:           1,
			InitialBackoff:       time.Millisecond,
			MaxBackoff:           time.Millisecond,
			BackoffMultiplier:    1,
			RetryableStatusCodes: nil,
			RetryableErrors:      []string{"timeout"},
		},
		config: ServiceClientConfig{
			UserAgent: "lift-test",
		},
	}

	instance := &ServiceInstance{
		ID:          "svc-instance",
		ServiceName: "test-service",
		Endpoint: ServiceEndpoint{
			Protocol: "http",
			Host:     "example.com",
			Port:     80,
		},
	}

	request := &ServiceRequest{
		ServiceName: "test-service",
		Method:      http.MethodPost,
		Path:        "/retry",
		Body:        map[string]string{"hello": "world"},
		Headers:     map[string]string{"X-Test": "value"},
	}

	resp, err := client.executeRequest(context.Background(), instance, request)
	if err != nil {
		t.Fatalf("executeRequest returned error: %v", err)
	}

	if resp == nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("expected successful response, got %+v", resp)
	}

	httpClient.mu.Lock()
	defer httpClient.mu.Unlock()

	if len(httpClient.bodies) != 2 {
		t.Fatalf("expected 2 attempts, got %d", len(httpClient.bodies))
	}

	first := httpClient.bodies[0]
	second := httpClient.bodies[1]
	if !bytes.Equal(first, second) {
		t.Fatalf("expected identical request bodies across retries: %q vs %q", first, second)
	}
	if len(first) == 0 {
		t.Fatal("expected non-empty request body")
	}
}

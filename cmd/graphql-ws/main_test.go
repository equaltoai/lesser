package main

import (
	"os"
	"testing"

	"github.com/aws/aws-lambda-go/events"
)

func init() {
	_ = os.Setenv("ENVIRONMENT", "test")
}

func TestExtractAuthTokenFromQuery(t *testing.T) {
	token := "abc123"
	event := events.APIGatewayWebsocketProxyRequest{
		QueryStringParameters: map[string]string{
			"access_token": token,
		},
		RequestContext: events.APIGatewayWebsocketProxyRequestContext{
			RequestID: "test-request",
		},
	}

	got := extractAuthToken(event)
	if got != token {
		t.Fatalf("expected %q, got %q", token, got)
	}
}

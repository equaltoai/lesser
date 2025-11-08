package main

import (
	"os"
	"testing"

	"github.com/aws/aws-lambda-go/events"
)

// TestExtractAuthTokenInput verifies a full JWT string passes through on the query param path.
func TestExtractAuthTokenInput(t *testing.T) {
	os.Setenv("ENVIRONMENT", "test")
	os.Setenv("JWT_SECRET", "testing")

	token := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiJsZXNzZXIiLCJleHAiOjE3NjIwNDYyOTksIm5iZiI6MTc2MjA0MjY5OSwiaWF0IjoxNzYyMDQyNjk5LCJqdGkiOiJmZTdid01ucmU2M3dqOVVYUDZWNl9nPT0iLCJ1c2VybmFtZSI6Imxlc3NlciIsInNjb3BlcyI6WyJyZWFkIiwid3JpdGUiLCJmb2xsb3ciLCJwdXNoIl0sImNsaWVudF9pZCI6IktXVGp4c0FsMHB2UTBIeXlnZmdLTHc9PSJ9.TY3sP-3ow4kSRImUUdbTeWbJDtd6qMHJbBk6dRJCLA8"
	event := events.APIGatewayWebsocketProxyRequest{
		QueryStringParameters: map[string]string{
			"access_token": token,
		},
		RequestContext: events.APIGatewayWebsocketProxyRequestContext{RequestID: "test"},
	}

	got := extractAuthToken(event)
	if got != token {
		t.Fatalf("expected %q, got %q", token, got)
	}
}

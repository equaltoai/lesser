package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/aws/aws-lambda-go/events"
	"github.com/equaltoai/lesser/internal/testutil/mocks"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestHandler(t *testing.T) {
	// Test actor
	testActor := &activitypub.Actor{
		BaseObject: activitypub.BaseObject{
			Context: []any{"https://www.w3.org/ns/activitystreams"},
			ID:      "https://example.com/users/alice",
			Type:    "Person",
			Summary: "Test user",
		},
		PreferredUsername: "alice",
		Name:              "Alice Smith",
		Inbox:             "https://example.com/users/alice/inbox",
		Outbox:            "https://example.com/users/alice/outbox",
		Followers:         "https://example.com/users/alice/followers",
		Following:         "https://example.com/users/alice/following",
		PublicKey: &activitypub.PublicKey{
			ID:           "https://example.com/users/alice#main-key",
			Owner:        "https://example.com/users/alice",
			PublicKeyPem: "-----BEGIN PUBLIC KEY-----\ntest\n-----END PUBLIC KEY-----",
		},
		Icon: &activitypub.Image{
			BaseObject: activitypub.BaseObject{
				Type: "Image",
			},
			URL: "https://example.com/avatars/alice.jpg",
		},
	}

	tests := []struct {
		name           string
		request        events.APIGatewayV2HTTPRequest
		setupMock      func(*mocks.MockStorage)
		wantStatusCode int
		wantHeaders    map[string]string
		checkBody      func(*testing.T, string)
	}{
		{
			name: "JSON response for ActivityPub client",
			request: events.APIGatewayV2HTTPRequest{
				PathParameters: map[string]string{"username": "alice"},
				Headers:        map[string]string{"Accept": "application/activity+json"},
			},
			setupMock: func(m *mocks.MockStorage) {
				m.On("GetActor", mock.Anything, "alice").Return(testActor, nil)
			},
			wantStatusCode: 200,
			wantHeaders: map[string]string{
				"Content-Type": "application/activity+json",
			},
			checkBody: func(t *testing.T, body string) {
				assert.Contains(t, body, `"preferredUsername":"alice"`)
				assert.Contains(t, body, `"type":"Person"`)
				assert.Contains(t, body, `"publicKey"`)
			},
		},
		{
			name: "JSON response for LD+JSON accept header",
			request: events.APIGatewayV2HTTPRequest{
				PathParameters: map[string]string{"username": "alice"},
				Headers:        map[string]string{"Accept": "application/ld+json; profile=\"https://www.w3.org/ns/activitystreams\""},
			},
			setupMock: func(m *mocks.MockStorage) {
				m.On("GetActor", mock.Anything, "alice").Return(testActor, nil)
			},
			wantStatusCode: 200,
			wantHeaders: map[string]string{
				"Content-Type": "application/activity+json",
			},
		},
		{
			name: "HTML response for browser",
			request: events.APIGatewayV2HTTPRequest{
				PathParameters: map[string]string{"username": "alice"},
				Headers:        map[string]string{"Accept": "text/html"},
			},
			setupMock: func(m *mocks.MockStorage) {
				m.On("GetActor", mock.Anything, "alice").Return(testActor, nil)
			},
			wantStatusCode: 200,
			wantHeaders: map[string]string{
				"Content-Type": "text/html; charset=utf-8",
			},
			checkBody: func(t *testing.T, body string) {
				assert.Contains(t, body, "Alice Smith")
				assert.Contains(t, body, "@alice@")
				assert.Contains(t, body, "Test user")
				assert.Contains(t, body, `<link rel="alternate" type="application/activity+json"`)
				assert.Contains(t, body, `<img src="https://example.com/avatars/alice.jpg"`)
			},
		},
		{
			name: "HTML response when no Accept header",
			request: events.APIGatewayV2HTTPRequest{
				PathParameters: map[string]string{"username": "alice"},
			},
			setupMock: func(m *mocks.MockStorage) {
				m.On("GetActor", mock.Anything, "alice").Return(testActor, nil)
			},
			wantStatusCode: 200,
			wantHeaders: map[string]string{
				"Content-Type": "text/html; charset=utf-8",
			},
		},
		{
			name: "actor not found",
			request: events.APIGatewayV2HTTPRequest{
				PathParameters: map[string]string{"username": "nonexistent"},
				Headers:        map[string]string{"Accept": "application/activity+json"},
			},
			setupMock: func(m *mocks.MockStorage) {
				m.On("GetActor", mock.Anything, "nonexistent").Return(nil, common.ActorNotFoundError{Username: "nonexistent"})
			},
			wantStatusCode: 404,
			checkBody: func(t *testing.T, body string) {
				assert.Contains(t, body, "not found")
			},
		},
		{
			name: "missing username parameter",
			request: events.APIGatewayV2HTTPRequest{
				PathParameters: map[string]string{},
				Headers:        map[string]string{"Accept": "application/activity+json"},
			},
			setupMock:      func(m *mocks.MockStorage) {},
			wantStatusCode: 400,
			checkBody: func(t *testing.T, body string) {
				assert.Contains(t, body, "missing username")
			},
		},
		{
			name: "storage error",
			request: events.APIGatewayV2HTTPRequest{
				PathParameters: map[string]string{"username": "alice"},
				Headers:        map[string]string{"Accept": "application/activity+json"},
			},
			setupMock: func(m *mocks.MockStorage) {
				m.On("GetActor", mock.Anything, "alice").Return(nil, errors.New("database error"))
			},
			wantStatusCode: 500,
			checkBody: func(t *testing.T, body string) {
				assert.Contains(t, body, "Internal Server Error")
			},
		},
		{
			name: "HTML for actor without avatar",
			request: events.APIGatewayV2HTTPRequest{
				PathParameters: map[string]string{"username": "bob"},
				Headers:        map[string]string{"Accept": "text/html"},
			},
			setupMock: func(m *mocks.MockStorage) {
				actorNoAvatar := &activitypub.Actor{
					BaseObject: activitypub.BaseObject{
						Context: []any{"https://www.w3.org/ns/activitystreams"},
						ID:      "https://example.com/users/bob",
						Type:    "Person",
					},
					PreferredUsername: "bob",
					Name:              "Bob",
					Inbox:             "https://example.com/users/bob/inbox",
					Outbox:            "https://example.com/users/bob/outbox",
				}
				m.On("GetActor", mock.Anything, "bob").Return(actorNoAvatar, nil)
			},
			wantStatusCode: 200,
			wantHeaders: map[string]string{
				"Content-Type": "text/html; charset=utf-8",
			},
			checkBody: func(t *testing.T, body string) {
				assert.NotContains(t, body, `<img src=`)
			},
		},
		{
			name: "lowercase accept header",
			request: events.APIGatewayV2HTTPRequest{
				PathParameters: map[string]string{"username": "alice"},
				Headers:        map[string]string{"accept": "application/activity+json"},
			},
			setupMock: func(m *mocks.MockStorage) {
				m.On("GetActor", mock.Anything, "alice").Return(testActor, nil)
			},
			wantStatusCode: 200,
			wantHeaders: map[string]string{
				"Content-Type": "application/activity+json",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock storage
			mockStore := new(mocks.MockStorage)
			tt.setupMock(mockStore)

			// Replace global store with mock
			oldStore := store
			store = mockStore
			defer func() { store = oldStore }()

			// Call handler
			resp, err := handler(context.Background(), tt.request)
			require.NoError(t, err)

			// Check status code
			assert.Equal(t, tt.wantStatusCode, resp.StatusCode)

			// Check headers
			for key, value := range tt.wantHeaders {
				assert.Equal(t, value, resp.Headers[key])
			}

			// Check body if provided
			if tt.checkBody != nil {
				tt.checkBody(t, resp.Body)
			}

			// Verify mock expectations
			mockStore.AssertExpectations(t)
		})
	}
}

func TestGenerateHTMLProfile(t *testing.T) {
	tests := []struct {
		name      string
		actor     *activitypub.Actor
		checkHTML func(*testing.T, string)
	}{
		{
			name: "full profile",
			actor: &activitypub.Actor{
				BaseObject: activitypub.BaseObject{
					ID:      "https://example.com/users/alice",
					Summary: "Test bio with <b>HTML</b>",
				},
				PreferredUsername: "alice",
				Name:              "Alice Smith",
				Followers:         "https://example.com/users/alice/followers",
				Following:         "https://example.com/users/alice/following",
				Icon: &activitypub.Image{
					URL: "https://example.com/avatar.jpg",
				},
			},
			checkHTML: func(t *testing.T, html string) {
				assert.Contains(t, html, "Alice Smith")
				assert.Contains(t, html, "@alice@")
				assert.Contains(t, html, "Test bio with <b>HTML</b>")
				assert.Contains(t, html, `<img src="https://example.com/avatar.jpg"`)
				assert.Contains(t, html, `<a href="https://example.com/users/alice/followers">Followers</a>`)
			},
		},
		{
			name: "minimal profile",
			actor: &activitypub.Actor{
				BaseObject: activitypub.BaseObject{
					ID: "https://example.com/users/bob",
				},
				PreferredUsername: "bob",
			},
			checkHTML: func(t *testing.T, html string) {
				assert.Contains(t, html, "bob") // Name falls back to username
				assert.Contains(t, html, "@bob@")
				assert.NotContains(t, html, `<div class="bio">`)
				assert.NotContains(t, html, `<div class="stats">`)
				assert.NotContains(t, html, `<img src=`)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			html := generateHTMLProfile(tt.actor)
			tt.checkHTML(t, html)

			// Common checks
			assert.Contains(t, html, "<!DOCTYPE html>")
			assert.Contains(t, html, `<meta property="og:type" content="profile">`)
			assert.Contains(t, html, `<link rel="alternate" type="application/activity+json"`)
		})
	}
}

// Test that Accept header parsing works correctly
func TestAcceptHeaderParsing(t *testing.T) {
	acceptHeaders := []struct {
		header    string
		wantJSON  bool
		wantHTML  bool
		defaultTo string
	}{
		{"application/activity+json", true, false, "json"},
		{"application/ld+json", true, false, "json"},
		{"application/json", true, false, "json"},
		{"text/html", false, true, "html"},
		{"text/html, application/xhtml+xml", false, true, "html"},
		{"*/*", false, true, "html"}, // Default to HTML
		{"", false, true, "html"},    // Default to HTML
		{"application/activity+json, text/html;q=0.9", true, false, "json"},
	}

	for _, tt := range acceptHeaders {
		t.Run(fmt.Sprintf("Accept: %s", tt.header), func(t *testing.T) {
			isJSON := strings.Contains(tt.header, "application/activity+json") ||
				strings.Contains(tt.header, "application/ld+json") ||
				strings.Contains(tt.header, "application/json")

			if tt.wantJSON {
				assert.True(t, isJSON, "Should recognize as JSON request")
			} else {
				assert.False(t, isJSON, "Should not recognize as JSON request")
			}
		})
	}
}

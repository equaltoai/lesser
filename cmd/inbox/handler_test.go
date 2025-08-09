package main

import (
	"testing"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/stretchr/testify/assert"
)

func TestIsAddressedTo(t *testing.T) {
	// Create test handler
	handler := &InboxHandler{}

	actor := &activitypub.Actor{
		BaseObject: activitypub.BaseObject{
			ID: "https://example.com/users/alice",
		},
		Inbox: "https://example.com/users/alice/inbox",
	}

	tests := []struct {
		name     string
		activity *activitypub.Activity
		expected bool
	}{
		{
			name: "addressed in to field by actor ID",
			activity: &activitypub.Activity{
				BaseObject: activitypub.BaseObject{
					To: []string{actor.ID},
				},
			},
			expected: true,
		},
		{
			name: "addressed in to field by inbox URL",
			activity: &activitypub.Activity{
				BaseObject: activitypub.BaseObject{
					To: []string{actor.Inbox},
				},
			},
			expected: true,
		},
		{
			name: "addressed to public",
			activity: &activitypub.Activity{
				BaseObject: activitypub.BaseObject{
					To: []string{activitypub.PublicAddress},
				},
			},
			expected: true,
		},
		{
			name: "not addressed to actor",
			activity: &activitypub.Activity{
				BaseObject: activitypub.BaseObject{
					To: []string{"https://example.com/users/bob"},
				},
			},
			expected: false,
		},
		{
			name: "addressed in cc field",
			activity: &activitypub.Activity{
				BaseObject: activitypub.BaseObject{
					CC: []string{actor.ID},
				},
			},
			expected: true,
		},
		{
			name: "addressed in bto field",
			activity: &activitypub.Activity{
				BaseObject: activitypub.BaseObject{
					BTo: []string{actor.ID},
				},
			},
			expected: true,
		},
		{
			name: "addressed in bcc field",
			activity: &activitypub.Activity{
				BaseObject: activitypub.BaseObject{
					BCC: []string{actor.ID},
				},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := handler.isAddressedTo(tt.activity, actor)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExtractHandleFromActorID(t *testing.T) {
	handler := &InboxHandler{}

	tests := []struct {
		name     string
		actorID  string
		expected string
	}{
		{
			name:     "standard ActivityPub actor ID",
			actorID:  "https://example.com/users/alice",
			expected: "@alice@example.com",
		},
		{
			name:     "actor ID with port",
			actorID:  "https://example.com:8080/users/bob",
			expected: "@bob@example.com:8080",
		},
		{
			name:     "actor ID with subdomain",
			actorID:  "https://social.example.com/users/charlie",
			expected: "@charlie@social.example.com",
		},
		{
			name:     "invalid actor ID format",
			actorID:  "not-a-url",
			expected: "not-a-url", // Falls back to returning as-is
		},
		{
			name:     "short actor ID",
			actorID:  "https://example.com/u/dave",
			expected: "https://example.com/u/dave", // Falls back due to unexpected format
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := handler.extractHandleFromActorID(tt.actorID)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExtractDomainFromURL(t *testing.T) {
	handler := &InboxHandler{}

	tests := []struct {
		name     string
		url      string
		expected string
	}{
		{
			name:     "standard HTTPS URL",
			url:      "https://example.com/inbox",
			expected: "example.com",
		},
		{
			name:     "URL with port",
			url:      "https://example.com:8080/inbox",
			expected: "example.com:8080",
		},
		{
			name:     "HTTP URL",
			url:      "http://example.com/path",
			expected: "example.com",
		},
		{
			name:     "URL with subdomain",
			url:      "https://social.example.com/users/alice",
			expected: "social.example.com",
		},
		{
			name:     "invalid URL",
			url:      "not a url",
			expected: "",
		},
		{
			name:     "empty URL",
			url:      "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := handler.extractDomainFromURL(tt.url)
			assert.Equal(t, tt.expected, result)
		})
	}
}

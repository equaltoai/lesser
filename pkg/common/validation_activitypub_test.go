package common

import (
	"strings"
	"testing"
)

// TestValidateActivityPubActor tests the ValidateActivityPubActor function
// for various actor object configurations
func TestValidateActivityPubActor(t *testing.T) {
	tests := []struct {
		name      string
		actor     map[string]interface{}
		expectErr bool
		errField  string
	}{
		{
			name: "valid minimal actor",
			actor: map[string]interface{}{
				"@context":          "https://www.w3.org/ns/activitystreams",
				"id":                "https://example.com/users/alice",
				"type":              "Person",
				"preferredUsername": "alice",
				"inbox":             "https://example.com/users/alice/inbox",
			},
			expectErr: false,
		},
		{
			name: "valid actor with all optional fields",
			actor: map[string]interface{}{
				"@context":          "https://www.w3.org/ns/activitystreams",
				"id":                "https://example.com/users/bob",
				"type":              "Person",
				"preferredUsername": "bob",
				"inbox":             "https://example.com/users/bob/inbox",
				"outbox":            "https://example.com/users/bob/outbox",
				"name":              "Bob Smith",
				"summary":           "A friendly user",
				"followers":         "https://example.com/users/bob/followers",
				"following":         "https://example.com/users/bob/following",
			},
			expectErr: false,
		},
		{
			name: "valid actor with context array",
			actor: map[string]interface{}{
				"@context": []interface{}{
					"https://www.w3.org/ns/activitystreams",
					"https://w3id.org/security/v1",
				},
				"id":                "https://example.com/users/charlie",
				"type":              "Service",
				"preferredUsername": "charlie",
				"inbox":             "https://example.com/users/charlie/inbox",
			},
			expectErr: false,
		},
		{
			name: "valid actor with public key",
			actor: map[string]interface{}{
				"@context":          "https://www.w3.org/ns/activitystreams",
				"id":                "https://example.com/users/dan",
				"type":              "Person",
				"preferredUsername": "dan",
				"inbox":             "https://example.com/users/dan/inbox",
				"publicKey": map[string]interface{}{
					"id":           "https://example.com/users/dan#main-key",
					"owner":        "https://example.com/users/dan",
					"publicKeyPem": "-----BEGIN PUBLIC KEY-----\nMIIBIjANBgkq...\n-----END PUBLIC KEY-----",
				},
			},
			expectErr: false,
		},
		{
			name: "missing @context",
			actor: map[string]interface{}{
				"id":                "https://example.com/users/alice",
				"type":              "Person",
				"preferredUsername": "alice",
				"inbox":             "https://example.com/users/alice/inbox",
			},
			expectErr: true,
			errField:  "@context",
		},
		{
			name: "invalid @context value",
			actor: map[string]interface{}{
				"@context":          "https://invalid.context.com",
				"id":                "https://example.com/users/alice",
				"type":              "Person",
				"preferredUsername": "alice",
				"inbox":             "https://example.com/users/alice/inbox",
			},
			expectErr: true,
			errField:  "@context",
		},
		{
			name: "missing id",
			actor: map[string]interface{}{
				"@context":          "https://www.w3.org/ns/activitystreams",
				"type":              "Person",
				"preferredUsername": "alice",
				"inbox":             "https://example.com/users/alice/inbox",
			},
			expectErr: true,
			errField:  "id",
		},
		{
			name: "empty id",
			actor: map[string]interface{}{
				"@context":          "https://www.w3.org/ns/activitystreams",
				"id":                "",
				"type":              "Person",
				"preferredUsername": "alice",
				"inbox":             "https://example.com/users/alice/inbox",
			},
			expectErr: true,
			errField:  "id",
		},
		{
			name: "invalid actor type",
			actor: map[string]interface{}{
				"@context":          "https://www.w3.org/ns/activitystreams",
				"id":                "https://example.com/users/alice",
				"type":              "InvalidType",
				"preferredUsername": "alice",
				"inbox":             "https://example.com/users/alice/inbox",
			},
			expectErr: true,
			errField:  "type",
		},
		{
			name: "missing preferredUsername",
			actor: map[string]interface{}{
				"@context": "https://www.w3.org/ns/activitystreams",
				"id":       "https://example.com/users/alice",
				"type":     "Person",
				"inbox":    "https://example.com/users/alice/inbox",
			},
			expectErr: true,
			errField:  "preferredUsername",
		},
		{
			name: "missing inbox",
			actor: map[string]interface{}{
				"@context":          "https://www.w3.org/ns/activitystreams",
				"id":                "https://example.com/users/alice",
				"type":              "Person",
				"preferredUsername": "alice",
			},
			expectErr: true,
			errField:  "inbox",
		},
		{
			name: "invalid inbox URL",
			actor: map[string]interface{}{
				"@context":          "https://www.w3.org/ns/activitystreams",
				"id":                "https://example.com/users/alice",
				"type":              "Person",
				"preferredUsername": "alice",
				"inbox":             "ftp://example.com/inbox",
			},
			expectErr: true,
			errField:  "inbox",
		},
		{
			name: "username with special chars at start",
			actor: map[string]interface{}{
				"@context":          "https://www.w3.org/ns/activitystreams",
				"id":                "https://example.com/users/alice",
				"type":              "Person",
				"preferredUsername": ".alice",
				"inbox":             "https://example.com/users/alice/inbox",
			},
			expectErr: true,
			errField:  "preferredUsername",
		},
		{
			name: "username too long",
			actor: map[string]interface{}{
				"@context":          "https://www.w3.org/ns/activitystreams",
				"id":                "https://example.com/users/alice",
				"type":              "Person",
				"preferredUsername": "thisisaverylongusernamethatexceeds30characters",
				"inbox":             "https://example.com/users/alice/inbox",
			},
			expectErr: true,
			errField:  "preferredUsername",
		},
		{
			name: "Group actor type",
			actor: map[string]interface{}{
				"@context":          "https://www.w3.org/ns/activitystreams",
				"id":                "https://example.com/groups/devs",
				"type":              "Group",
				"preferredUsername": "devs",
				"inbox":             "https://example.com/groups/devs/inbox",
			},
			expectErr: false,
		},
		{
			name: "Application actor type",
			actor: map[string]interface{}{
				"@context":          "https://www.w3.org/ns/activitystreams",
				"id":                "https://example.com/apps/bot",
				"type":              "Application",
				"preferredUsername": "bot",
				"inbox":             "https://example.com/apps/bot/inbox",
			},
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateActivityPubActor(tt.actor)
			if tt.expectErr {
				if err == nil {
					t.Errorf("ValidateActivityPubActor() expected error, got nil")
					return
				}
				if tt.errField != "" {
					verr, ok := err.(ValidationError)
					if ok && verr.Field != tt.errField {
						t.Errorf("ValidateActivityPubActor() error field = %v, want %v", verr.Field, tt.errField)
					}
				}
			} else {
				if err != nil {
					t.Errorf("ValidateActivityPubActor() unexpected error = %v", err)
				}
			}
		})
	}
}

// TestValidateActivityPubActivity tests the ValidateActivityPubActivity function
func TestValidateActivityPubActivity(t *testing.T) {
	tests := []struct {
		name      string
		activity  map[string]interface{}
		expectErr bool
		errField  string
	}{
		{
			name: "valid Create activity",
			activity: map[string]interface{}{
				"@context": "https://www.w3.org/ns/activitystreams",
				"id":       "https://example.com/activities/1",
				"type":     "Create",
				"actor":    "https://example.com/users/alice",
				"object": map[string]interface{}{
					"type":    "Note",
					"content": "Hello World",
				},
			},
			expectErr: false,
		},
		{
			name: "valid Follow activity without object",
			activity: map[string]interface{}{
				"@context": "https://www.w3.org/ns/activitystreams",
				"id":       "https://example.com/activities/2",
				"type":     "Follow",
				"actor":    "https://example.com/users/alice",
				"object":   "https://example.com/users/bob",
			},
			expectErr: false,
		},
		{
			name: "valid activity with addressing",
			activity: map[string]interface{}{
				"@context": "https://www.w3.org/ns/activitystreams",
				"id":       "https://example.com/activities/3",
				"type":     "Create",
				"actor":    "https://example.com/users/alice",
				"to":       []interface{}{"https://www.w3.org/ns/activitystreams#Public"},
				"cc":       []interface{}{"https://example.com/users/bob"},
				"object": map[string]interface{}{
					"type":    "Note",
					"content": "Hello Everyone",
				},
			},
			expectErr: false,
		},
		{
			name: "valid activity with published timestamp",
			activity: map[string]interface{}{
				"@context":  "https://www.w3.org/ns/activitystreams",
				"id":        "https://example.com/activities/4",
				"type":      "Create",
				"actor":     "https://example.com/users/alice",
				"published": "2024-01-15T10:00:00Z",
				"object": map[string]interface{}{
					"type":    "Note",
					"content": "Time-stamped note",
				},
			},
			expectErr: false,
		},
		{
			name: "missing @context",
			activity: map[string]interface{}{
				"id":    "https://example.com/activities/1",
				"type":  "Create",
				"actor": "https://example.com/users/alice",
				"object": map[string]interface{}{
					"type":    "Note",
					"content": "Hello World",
				},
			},
			expectErr: true,
			errField:  "@context",
		},
		{
			name: "missing id",
			activity: map[string]interface{}{
				"@context": "https://www.w3.org/ns/activitystreams",
				"type":     "Create",
				"actor":    "https://example.com/users/alice",
				"object": map[string]interface{}{
					"type":    "Note",
					"content": "Hello World",
				},
			},
			expectErr: true,
			errField:  "id",
		},
		{
			name: "missing type",
			activity: map[string]interface{}{
				"@context": "https://www.w3.org/ns/activitystreams",
				"id":       "https://example.com/activities/1",
				"actor":    "https://example.com/users/alice",
				"object": map[string]interface{}{
					"type":    "Note",
					"content": "Hello World",
				},
			},
			expectErr: true,
			errField:  "type",
		},
		{
			name: "invalid activity type",
			activity: map[string]interface{}{
				"@context": "https://www.w3.org/ns/activitystreams",
				"id":       "https://example.com/activities/1",
				"type":     "InvalidActivity",
				"actor":    "https://example.com/users/alice",
			},
			expectErr: true,
			errField:  "type",
		},
		{
			name: "missing actor",
			activity: map[string]interface{}{
				"@context": "https://www.w3.org/ns/activitystreams",
				"id":       "https://example.com/activities/1",
				"type":     "Create",
				"object": map[string]interface{}{
					"type":    "Note",
					"content": "Hello World",
				},
			},
			expectErr: true,
			errField:  "actor",
		},
		{
			name: "Create without object",
			activity: map[string]interface{}{
				"@context": "https://www.w3.org/ns/activitystreams",
				"id":       "https://example.com/activities/1",
				"type":     "Create",
				"actor":    "https://example.com/users/alice",
			},
			expectErr: true,
			errField:  "object",
		},
		{
			name: "Like without object",
			activity: map[string]interface{}{
				"@context": "https://www.w3.org/ns/activitystreams",
				"id":       "https://example.com/activities/1",
				"type":     "Like",
				"actor":    "https://example.com/users/alice",
			},
			expectErr: true,
			errField:  "object",
		},
		{
			name: "invalid published timestamp",
			activity: map[string]interface{}{
				"@context":  "https://www.w3.org/ns/activitystreams",
				"id":        "https://example.com/activities/1",
				"type":      "Create",
				"actor":     "https://example.com/users/alice",
				"published": "not-a-timestamp",
				"object": map[string]interface{}{
					"type":    "Note",
					"content": "Hello World",
				},
			},
			expectErr: true,
			errField:  "published",
		},
		{
			name: "Accept activity type",
			activity: map[string]interface{}{
				"@context": "https://www.w3.org/ns/activitystreams",
				"id":       "https://example.com/activities/5",
				"type":     "Accept",
				"actor":    "https://example.com/users/bob",
				"object":   "https://example.com/activities/2",
			},
			expectErr: false,
		},
		{
			name: "Announce activity type",
			activity: map[string]interface{}{
				"@context": "https://www.w3.org/ns/activitystreams",
				"id":       "https://example.com/activities/6",
				"type":     "Announce",
				"actor":    "https://example.com/users/alice",
				"object":   "https://example.com/notes/1",
			},
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateActivityPubActivity(tt.activity)
			if tt.expectErr {
				if err == nil {
					t.Errorf("ValidateActivityPubActivity() expected error, got nil")
					return
				}
				if tt.errField != "" {
					verr, ok := err.(ValidationError)
					if ok && verr.Field != tt.errField {
						t.Errorf("ValidateActivityPubActivity() error field = %v, want %v", verr.Field, tt.errField)
					}
				}
			} else {
				if err != nil {
					t.Errorf("ValidateActivityPubActivity() unexpected error = %v", err)
				}
			}
		})
	}
}

// TestValidateActivityPubNote tests the ValidateActivityPubNote function
func TestValidateActivityPubNote(t *testing.T) {
	tests := []struct {
		name      string
		note      map[string]interface{}
		expectErr bool
		errField  string
	}{
		{
			name: "valid minimal note",
			note: map[string]interface{}{
				"@context":     "https://www.w3.org/ns/activitystreams",
				"type":         "Note",
				"content":      "Hello World",
				"attributedTo": "https://example.com/users/alice",
			},
			expectErr: false,
		},
		{
			name: "valid note with all optional fields",
			note: map[string]interface{}{
				"@context":     "https://www.w3.org/ns/activitystreams",
				"id":           "https://example.com/notes/1",
				"type":         "Note",
				"content":      "Hello World",
				"attributedTo": "https://example.com/users/alice",
				"inReplyTo":    "https://example.com/notes/0",
				"summary":      "Content warning",
				"published":    "2024-01-15T10:00:00Z",
				"to":           []interface{}{"https://www.w3.org/ns/activitystreams#Public"},
				"cc":           []interface{}{"https://example.com/users/bob"},
			},
			expectErr: false,
		},
		{
			name: "valid note with attachments",
			note: map[string]interface{}{
				"@context":     "https://www.w3.org/ns/activitystreams",
				"type":         "Note",
				"content":      "Check out this image",
				"attributedTo": "https://example.com/users/alice",
				"attachment": []interface{}{
					map[string]interface{}{
						"type": "Image",
						"url":  "https://example.com/media/image.png",
					},
				},
			},
			expectErr: false,
		},
		{
			name: "valid note with tags",
			note: map[string]interface{}{
				"@context":     "https://www.w3.org/ns/activitystreams",
				"type":         "Note",
				"content":      "Hello @bob #greeting",
				"attributedTo": "https://example.com/users/alice",
				"tag": []interface{}{
					map[string]interface{}{
						"type": "Mention",
						"href": "https://example.com/users/bob",
						"name": "@bob",
					},
					map[string]interface{}{
						"type": "Hashtag",
						"href": "https://example.com/tags/greeting",
						"name": "#greeting",
					},
				},
			},
			expectErr: false,
		},
		{
			name: "missing @context",
			note: map[string]interface{}{
				"type":         "Note",
				"content":      "Hello World",
				"attributedTo": "https://example.com/users/alice",
			},
			expectErr: true,
			errField:  "@context",
		},
		{
			name: "wrong type",
			note: map[string]interface{}{
				"@context":     "https://www.w3.org/ns/activitystreams",
				"type":         "Article",
				"content":      "Hello World",
				"attributedTo": "https://example.com/users/alice",
			},
			expectErr: true,
			errField:  "type",
		},
		{
			name: "missing content",
			note: map[string]interface{}{
				"@context":     "https://www.w3.org/ns/activitystreams",
				"type":         "Note",
				"attributedTo": "https://example.com/users/alice",
			},
			expectErr: true,
			errField:  "content",
		},
		{
			name: "empty content",
			note: map[string]interface{}{
				"@context":     "https://www.w3.org/ns/activitystreams",
				"type":         "Note",
				"content":      "   ",
				"attributedTo": "https://example.com/users/alice",
			},
			expectErr: true,
			errField:  "content",
		},
		{
			name: "missing attributedTo",
			note: map[string]interface{}{
				"@context": "https://www.w3.org/ns/activitystreams",
				"type":     "Note",
				"content":  "Hello World",
			},
			expectErr: true,
			errField:  "attributedTo",
		},
		{
			name: "invalid attributedTo URL",
			note: map[string]interface{}{
				"@context":     "https://www.w3.org/ns/activitystreams",
				"type":         "Note",
				"content":      "Hello World",
				"attributedTo": "ftp://example.com/users/alice",
			},
			expectErr: true,
			errField:  "attributedTo",
		},
		{
			name: "invalid inReplyTo URL",
			note: map[string]interface{}{
				"@context":     "https://www.w3.org/ns/activitystreams",
				"type":         "Note",
				"content":      "Reply content",
				"attributedTo": "https://example.com/users/alice",
				"inReplyTo":    "ftp://example.com/notes/0",
			},
			expectErr: true,
			errField:  "inReplyTo",
		},
		{
			name: "summary too long",
			note: map[string]interface{}{
				"@context":     "https://www.w3.org/ns/activitystreams",
				"type":         "Note",
				"content":      "Hello World",
				"attributedTo": "https://example.com/users/alice",
				"summary":      strings.Repeat("x", 501),
			},
			expectErr: true,
			errField:  "summary",
		},
		{
			name: "too many attachments",
			note: map[string]interface{}{
				"@context":     "https://www.w3.org/ns/activitystreams",
				"type":         "Note",
				"content":      "Hello World",
				"attributedTo": "https://example.com/users/alice",
				"attachment": []interface{}{
					map[string]interface{}{"type": "Image", "url": "https://example.com/1.png"},
					map[string]interface{}{"type": "Image", "url": "https://example.com/2.png"},
					map[string]interface{}{"type": "Image", "url": "https://example.com/3.png"},
					map[string]interface{}{"type": "Image", "url": "https://example.com/4.png"},
					map[string]interface{}{"type": "Image", "url": "https://example.com/5.png"},
					map[string]interface{}{"type": "Image", "url": "https://example.com/6.png"},
					map[string]interface{}{"type": "Image", "url": "https://example.com/7.png"},
					map[string]interface{}{"type": "Image", "url": "https://example.com/8.png"},
					map[string]interface{}{"type": "Image", "url": "https://example.com/9.png"},
					map[string]interface{}{"type": "Image", "url": "https://example.com/10.png"},
					map[string]interface{}{"type": "Image", "url": "https://example.com/11.png"},
				},
			},
			expectErr: true,
			errField:  "attachment",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateActivityPubNote(tt.note)
			if tt.expectErr {
				if err == nil {
					t.Errorf("ValidateActivityPubNote() expected error, got nil")
					return
				}
				if tt.errField != "" {
					verr, ok := err.(ValidationError)
					if ok && verr.Field != tt.errField {
						t.Errorf("ValidateActivityPubNote() error field = %v, want %v", verr.Field, tt.errField)
					}
				}
			} else {
				if err != nil {
					t.Errorf("ValidateActivityPubNote() unexpected error = %v", err)
				}
			}
		})
	}
}

// TestValidateActivityPubURL tests the ValidateActivityPubURL function
func TestValidateActivityPubURL(t *testing.T) {
	tests := []struct {
		name      string
		url       string
		fieldName string
		expectErr bool
	}{
		{
			name:      "valid HTTPS URL",
			url:       "https://example.com/users/alice",
			fieldName: "id",
			expectErr: false,
		},
		{
			name:      "valid HTTP URL",
			url:       "http://example.com/users/alice",
			fieldName: "id",
			expectErr: false,
		},
		{
			name:      "valid URL with port",
			url:       "https://example.com:8080/users/alice",
			fieldName: "id",
			expectErr: false,
		},
		{
			name:      "valid URL with query params",
			url:       "https://example.com/users/alice?page=1",
			fieldName: "id",
			expectErr: false,
		},
		{
			name:      "empty URL",
			url:       "",
			fieldName: "id",
			expectErr: true,
		},
		{
			name:      "FTP scheme not allowed",
			url:       "ftp://example.com/users/alice",
			fieldName: "id",
			expectErr: true,
		},
		{
			name:      "mailto scheme not allowed",
			url:       "mailto:alice@example.com",
			fieldName: "id",
			expectErr: true,
		},
		{
			name:      "URL without host",
			url:       "https:///users/alice",
			fieldName: "id",
			expectErr: true,
		},
		{
			name:      "URL too long",
			url:       "https://example.com/" + strings.Repeat("x", 2001),
			fieldName: "id",
			expectErr: true,
		},
		{
			name:      "relative URL not allowed",
			url:       "/users/alice",
			fieldName: "id",
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateActivityPubURL(tt.url, tt.fieldName)
			if (err != nil) != tt.expectErr {
				t.Errorf("ValidateActivityPubURL() error = %v, expectErr %v", err, tt.expectErr)
			}
		})
	}
}

// TestValidateActivityPubUsername tests the ValidateActivityPubUsername function
func TestValidateActivityPubUsername(t *testing.T) {
	tests := []struct {
		name      string
		username  string
		expectErr bool
	}{
		{
			name:      "valid simple username",
			username:  "alice",
			expectErr: false,
		},
		{
			name:      "valid username with numbers",
			username:  "alice123",
			expectErr: false,
		},
		{
			name:      "valid username with underscore",
			username:  "alice_smith",
			expectErr: false,
		},
		{
			name:      "valid username with hyphen",
			username:  "alice-smith",
			expectErr: false,
		},
		{
			name:      "valid username with dot in middle",
			username:  "alice.smith",
			expectErr: false,
		},
		{
			name:      "empty username",
			username:  "",
			expectErr: true,
		},
		{
			name:      "username too long",
			username:  "abcdefghijklmnopqrstuvwxyz12345",
			expectErr: true,
		},
		{
			name:      "username with spaces",
			username:  "alice smith",
			expectErr: true,
		},
		{
			name:      "username with special characters",
			username:  "alice@example",
			expectErr: true,
		},
		{
			name:      "username starting with dot",
			username:  ".alice",
			expectErr: true,
		},
		{
			name:      "username ending with dot",
			username:  "alice.",
			expectErr: true,
		},
		{
			name:      "username starting with hyphen",
			username:  "-alice",
			expectErr: true,
		},
		{
			name:      "username ending with hyphen",
			username:  "alice-",
			expectErr: true,
		},
		{
			name:      "username starting with underscore",
			username:  "_alice",
			expectErr: true,
		},
		{
			name:      "username ending with underscore",
			username:  "alice_",
			expectErr: true,
		},
		{
			name:      "username exactly 30 chars",
			username:  "abcdefghijklmnopqrstuvwxyz1234",
			expectErr: false,
		},
		{
			name:      "uppercase letters allowed",
			username:  "Alice",
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateActivityPubUsername(tt.username)
			if (err != nil) != tt.expectErr {
				t.Errorf("ValidateActivityPubUsername() error = %v, expectErr %v", err, tt.expectErr)
			}
		})
	}
}

// TestValidateActivityPubAddressing tests the ValidateActivityPubAddressing function
func TestValidateActivityPubAddressing(t *testing.T) {
	tests := []struct {
		name       string
		addressing interface{}
		fieldName  string
		expectErr  bool
	}{
		{
			name:       "single public address",
			addressing: PublicAddress,
			fieldName:  "to",
			expectErr:  false,
		},
		{
			name:       "single valid URL",
			addressing: "https://example.com/users/alice",
			fieldName:  "to",
			expectErr:  false,
		},
		{
			name:       "array with public address",
			addressing: []interface{}{PublicAddress},
			fieldName:  "to",
			expectErr:  false,
		},
		{
			name: "array with multiple addresses",
			addressing: []interface{}{
				"https://www.w3.org/ns/activitystreams#Public",
				"https://example.com/users/alice",
				"https://example.com/users/bob",
			},
			fieldName: "cc",
			expectErr: false,
		},
		{
			name:       "empty array",
			addressing: []interface{}{},
			fieldName:  "to",
			expectErr:  false,
		},
		{
			name:       "invalid URL in single address",
			addressing: "ftp://example.com/users/alice",
			fieldName:  "to",
			expectErr:  true,
		},
		{
			name: "invalid URL in array",
			addressing: []interface{}{
				"https://example.com/users/alice",
				"ftp://example.com/users/bob",
			},
			fieldName: "cc",
			expectErr: true,
		},
		{
			name: "non-string in array",
			addressing: []interface{}{
				"https://example.com/users/alice",
				123,
			},
			fieldName: "to",
			expectErr: true,
		},
		{
			name:       "invalid type (number)",
			addressing: 12345,
			fieldName:  "to",
			expectErr:  true,
		},
		{
			name:       "invalid type (map)",
			addressing: map[string]string{"url": "https://example.com"},
			fieldName:  "to",
			expectErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateActivityPubAddressing(tt.addressing, tt.fieldName)
			if (err != nil) != tt.expectErr {
				t.Errorf("ValidateActivityPubAddressing() error = %v, expectErr %v", err, tt.expectErr)
			}
		})
	}
}

// TestValidateWebfingerResource tests the ValidateWebfingerResource function
func TestValidateWebfingerResource(t *testing.T) {
	tests := []struct {
		name      string
		resource  string
		expectErr bool
	}{
		{
			name:      "valid acct URI",
			resource:  "acct:alice@example.com",
			expectErr: false,
		},
		{
			name:      "valid acct URI with subdomain",
			resource:  "acct:alice@social.example.com",
			expectErr: false,
		},
		{
			name:      "valid HTTPS URL",
			resource:  "https://example.com/users/alice",
			expectErr: false,
		},
		{
			name:      "valid HTTP URL",
			resource:  "http://example.com/users/alice",
			expectErr: false,
		},
		{
			name:      "empty resource",
			resource:  "",
			expectErr: true,
		},
		{
			name:      "invalid acct format - missing @",
			resource:  "acct:alice",
			expectErr: true,
		},
		{
			name:      "invalid acct format - multiple @",
			resource:  "acct:alice@foo@bar.com",
			expectErr: true,
		},
		{
			name:      "invalid acct format - empty username",
			resource:  "acct:@example.com",
			expectErr: true,
		},
		{
			name:      "invalid acct format - invalid domain",
			resource:  "acct:alice@-invalid.com",
			expectErr: true,
		},
		{
			name:      "acct with username starting with dot",
			resource:  "acct:.alice@example.com",
			expectErr: true,
		},
		{
			name:      "acct with username ending with hyphen",
			resource:  "acct:alice-@example.com",
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateWebfingerResource(tt.resource)
			if (err != nil) != tt.expectErr {
				t.Errorf("ValidateWebfingerResource() error = %v, expectErr %v", err, tt.expectErr)
			}
		})
	}
}

// TestValidateActivityPubContext tests the ValidateActivityPubContext function
func TestValidateActivityPubContext(t *testing.T) {
	tests := []struct {
		name      string
		obj       map[string]interface{}
		expectErr bool
	}{
		{
			name: "valid string context",
			obj: map[string]interface{}{
				"@context": "https://www.w3.org/ns/activitystreams",
			},
			expectErr: false,
		},
		{
			name: "valid array context",
			obj: map[string]interface{}{
				"@context": []interface{}{
					"https://www.w3.org/ns/activitystreams",
					"https://w3id.org/security/v1",
				},
			},
			expectErr: false,
		},
		{
			name: "valid array context with ActivityStreams not first",
			obj: map[string]interface{}{
				"@context": []interface{}{
					"https://w3id.org/security/v1",
					"https://www.w3.org/ns/activitystreams",
				},
			},
			expectErr: false,
		},
		{
			name: "valid array context with objects",
			obj: map[string]interface{}{
				"@context": []interface{}{
					"https://www.w3.org/ns/activitystreams",
					map[string]string{"sensitive": "as:sensitive"},
				},
			},
			expectErr: false,
		},
		{
			name:      "missing @context",
			obj:       map[string]interface{}{},
			expectErr: true,
		},
		{
			name: "wrong context string",
			obj: map[string]interface{}{
				"@context": "https://example.com/custom-context",
			},
			expectErr: true,
		},
		{
			name: "array context without ActivityStreams",
			obj: map[string]interface{}{
				"@context": []interface{}{
					"https://w3id.org/security/v1",
					"https://example.com/custom",
				},
			},
			expectErr: true,
		},
		{
			name: "invalid context type (number)",
			obj: map[string]interface{}{
				"@context": 12345,
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateActivityPubContext(tt.obj)
			if (err != nil) != tt.expectErr {
				t.Errorf("ValidateActivityPubContext() error = %v, expectErr %v", err, tt.expectErr)
			}
		})
	}
}

// TestValidateActivityPubTimestamp tests the ValidateActivityPubTimestamp function
func TestValidateActivityPubTimestamp(t *testing.T) {
	tests := []struct {
		name      string
		timestamp string
		expectErr bool
	}{
		{
			name:      "valid RFC3339 timestamp",
			timestamp: "2024-01-15T10:30:00Z",
			expectErr: false,
		},
		{
			name:      "valid RFC3339 with timezone offset",
			timestamp: "2024-01-15T10:30:00+05:00",
			expectErr: false,
		},
		{
			name:      "valid RFC3339 with negative offset",
			timestamp: "2024-01-15T10:30:00-05:00",
			expectErr: false,
		},
		{
			name:      "empty timestamp allowed",
			timestamp: "",
			expectErr: false,
		},
		{
			name:      "invalid format - date only",
			timestamp: "2024-01-15",
			expectErr: true,
		},
		{
			name:      "invalid format - not ISO",
			timestamp: "Jan 15, 2024",
			expectErr: true,
		},
		{
			name:      "invalid format - Unix timestamp",
			timestamp: "1705315800",
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateActivityPubTimestamp(tt.timestamp, "published")
			if (err != nil) != tt.expectErr {
				t.Errorf("ValidateActivityPubTimestamp() error = %v, expectErr %v", err, tt.expectErr)
			}
		})
	}
}

// TestValidateDomainName tests the ValidateDomainName function
func TestValidateDomainName(t *testing.T) {
	tests := []struct {
		name      string
		domain    string
		expectErr bool
	}{
		{
			name:      "valid simple domain",
			domain:    "example.com",
			expectErr: false,
		},
		{
			name:      "valid subdomain",
			domain:    "social.example.com",
			expectErr: false,
		},
		{
			name:      "valid domain with hyphen",
			domain:    "my-example.com",
			expectErr: false,
		},
		{
			name:      "single label domain",
			domain:    "localhost",
			expectErr: false,
		},
		{
			name:      "empty domain",
			domain:    "",
			expectErr: true,
		},
		{
			name:      "domain too long",
			domain:    strings.Repeat("x", 254),
			expectErr: true,
		},
		{
			name:      "domain starting with hyphen",
			domain:    "-example.com",
			expectErr: true,
		},
		{
			name:      "domain with spaces",
			domain:    "example .com",
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateDomainName(tt.domain)
			if (err != nil) != tt.expectErr {
				t.Errorf("ValidateDomainName() error = %v, expectErr %v", err, tt.expectErr)
			}
		})
	}
}

// TestValidateActivityPubSignature tests the ValidateActivityPubSignature function
func TestValidateActivityPubSignature(t *testing.T) {
	tests := []struct {
		name      string
		signature string
		expectErr bool
	}{
		{
			name:      "valid signature",
			signature: `keyId="https://example.com/users/alice#main-key",algorithm="rsa-sha256",headers="(request-target) host date",signature="abc123=="`,
			expectErr: false,
		},
		{
			name:      "valid signature without algorithm",
			signature: `keyId="https://example.com/users/alice#main-key",signature="abc123=="`,
			expectErr: false,
		},
		{
			name:      "empty signature",
			signature: "",
			expectErr: true,
		},
		{
			name:      "missing keyId",
			signature: `algorithm="rsa-sha256",signature="abc123=="`,
			expectErr: true,
		},
		{
			name:      "missing signature value",
			signature: `keyId="https://example.com/users/alice#main-key",algorithm="rsa-sha256"`,
			expectErr: true,
		},
		{
			name:      "invalid keyId URL",
			signature: `keyId="not-a-url",signature="abc123=="`,
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateActivityPubSignature(tt.signature)
			if (err != nil) != tt.expectErr {
				t.Errorf("ValidateActivityPubSignature() error = %v, expectErr %v", err, tt.expectErr)
			}
		})
	}
}

// TestValidateActivityPubContentType tests the ValidateActivityPubContentType function
func TestValidateActivityPubContentType(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		expectErr   bool
	}{
		{
			name:        "activity+json",
			contentType: "application/activity+json",
			expectErr:   false,
		},
		{
			name:        "ld+json",
			contentType: "application/ld+json",
			expectErr:   false,
		},
		{
			name:        "application/json",
			contentType: "application/json",
			expectErr:   false,
		},
		{
			name:        "with charset",
			contentType: "application/activity+json; charset=utf-8",
			expectErr:   false,
		},
		{
			name:        "uppercase should work",
			contentType: "APPLICATION/ACTIVITY+JSON",
			expectErr:   false,
		},
		{
			name:        "empty content type",
			contentType: "",
			expectErr:   true,
		},
		{
			name:        "text/html not allowed",
			contentType: "text/html",
			expectErr:   true,
		},
		{
			name:        "text/plain not allowed",
			contentType: "text/plain",
			expectErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateActivityPubContentType(tt.contentType)
			if (err != nil) != tt.expectErr {
				t.Errorf("ValidateActivityPubContentType() error = %v, expectErr %v", err, tt.expectErr)
			}
		})
	}
}

// TestValidateActivityPubJSON tests the ValidateActivityPubJSON function
func TestValidateActivityPubJSON(t *testing.T) {
	tests := []struct {
		name      string
		jsonStr   string
		expectErr bool
	}{
		{
			name:      "valid ActivityPub JSON",
			jsonStr:   `{"@context":"https://www.w3.org/ns/activitystreams","type":"Note","content":"Hello"}`,
			expectErr: false,
		},
		{
			name:      "valid with array context",
			jsonStr:   `{"@context":["https://www.w3.org/ns/activitystreams"],"type":"Create"}`,
			expectErr: false,
		},
		{
			name:      "empty string",
			jsonStr:   "",
			expectErr: true,
		},
		{
			name:      "invalid JSON",
			jsonStr:   `{"broken": json}`,
			expectErr: true,
		},
		{
			name:      "missing @context",
			jsonStr:   `{"type":"Note"}`,
			expectErr: true,
		},
		{
			name:      "missing type",
			jsonStr:   `{"@context":"https://www.w3.org/ns/activitystreams"}`,
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateActivityPubJSON(tt.jsonStr, "body")
			if (err != nil) != tt.expectErr {
				t.Errorf("ValidateActivityPubJSON() error = %v, expectErr %v", err, tt.expectErr)
			}
		})
	}
}

// TestValidateActivityPubPublicKey tests the ValidateActivityPubPublicKey function
func TestValidateActivityPubPublicKey(t *testing.T) {
	tests := []struct {
		name      string
		publicKey map[string]interface{}
		expectErr bool
	}{
		{
			name: "valid public key",
			publicKey: map[string]interface{}{
				"id":           "https://example.com/users/alice#main-key",
				"owner":        "https://example.com/users/alice",
				"publicKeyPem": "-----BEGIN PUBLIC KEY-----\nMIIBIjAN...\n-----END PUBLIC KEY-----",
			},
			expectErr: false,
		},
		{
			name: "missing id",
			publicKey: map[string]interface{}{
				"owner":        "https://example.com/users/alice",
				"publicKeyPem": "-----BEGIN PUBLIC KEY-----\nMIIBIjAN...\n-----END PUBLIC KEY-----",
			},
			expectErr: true,
		},
		{
			name: "missing owner",
			publicKey: map[string]interface{}{
				"id":           "https://example.com/users/alice#main-key",
				"publicKeyPem": "-----BEGIN PUBLIC KEY-----\nMIIBIjAN...\n-----END PUBLIC KEY-----",
			},
			expectErr: true,
		},
		{
			name: "missing publicKeyPem",
			publicKey: map[string]interface{}{
				"id":    "https://example.com/users/alice#main-key",
				"owner": "https://example.com/users/alice",
			},
			expectErr: true,
		},
		{
			name: "invalid PEM format",
			publicKey: map[string]interface{}{
				"id":           "https://example.com/users/alice#main-key",
				"owner":        "https://example.com/users/alice",
				"publicKeyPem": "not-a-valid-pem-key",
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateActivityPubPublicKey(tt.publicKey)
			if (err != nil) != tt.expectErr {
				t.Errorf("ValidateActivityPubPublicKey() error = %v, expectErr %v", err, tt.expectErr)
			}
		})
	}
}

// TestValidateActivityPubAttachments tests the ValidateActivityPubAttachments function
func TestValidateActivityPubAttachments(t *testing.T) {
	tests := []struct {
		name        string
		attachments interface{}
		expectErr   bool
	}{
		{
			name: "valid attachments",
			attachments: []interface{}{
				map[string]interface{}{
					"type": "Image",
					"url":  "https://example.com/image.png",
				},
			},
			expectErr: false,
		},
		{
			name:        "empty array",
			attachments: []interface{}{},
			expectErr:   false,
		},
		{
			name: "multiple attachments",
			attachments: []interface{}{
				map[string]interface{}{"type": "Image", "url": "https://example.com/1.png"},
				map[string]interface{}{"type": "Video", "url": "https://example.com/video.mp4"},
			},
			expectErr: false,
		},
		{
			name:        "not an array",
			attachments: "not-an-array",
			expectErr:   true,
		},
		{
			name: "attachment without type",
			attachments: []interface{}{
				map[string]interface{}{
					"url": "https://example.com/image.png",
				},
			},
			expectErr: true,
		},
		{
			name: "attachment without url",
			attachments: []interface{}{
				map[string]interface{}{
					"type": "Image",
				},
			},
			expectErr: true,
		},
		{
			name: "attachment with invalid url",
			attachments: []interface{}{
				map[string]interface{}{
					"type": "Image",
					"url":  "ftp://example.com/image.png",
				},
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateActivityPubAttachments(tt.attachments, "attachment")
			if (err != nil) != tt.expectErr {
				t.Errorf("ValidateActivityPubAttachments() error = %v, expectErr %v", err, tt.expectErr)
			}
		})
	}
}

// TestValidateActivityPubTags tests the ValidateActivityPubTags function
func TestValidateActivityPubTags(t *testing.T) {
	tests := []struct {
		name      string
		tags      interface{}
		expectErr bool
	}{
		{
			name: "valid mention tag",
			tags: []interface{}{
				map[string]interface{}{
					"type": "Mention",
					"href": "https://example.com/users/bob",
					"name": "@bob",
				},
			},
			expectErr: false,
		},
		{
			name: "valid hashtag",
			tags: []interface{}{
				map[string]interface{}{
					"type": "Hashtag",
					"href": "https://example.com/tags/hello",
					"name": "#hello",
				},
			},
			expectErr: false,
		},
		{
			name:      "empty array",
			tags:      []interface{}{},
			expectErr: false,
		},
		{
			name:      "not an array",
			tags:      "not-an-array",
			expectErr: true,
		},
		{
			name: "tag without type",
			tags: []interface{}{
				map[string]interface{}{
					"href": "https://example.com/users/bob",
				},
			},
			expectErr: true,
		},
		{
			name: "mention with invalid href",
			tags: []interface{}{
				map[string]interface{}{
					"type": "Mention",
					"href": "ftp://example.com/users/bob",
				},
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateActivityPubTags(tt.tags, "tag")
			if (err != nil) != tt.expectErr {
				t.Errorf("ValidateActivityPubTags() error = %v, expectErr %v", err, tt.expectErr)
			}
		})
	}
}

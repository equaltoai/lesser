package common

import (
	"strings"
	"testing"
)

// TestValidateStatusParams tests the ValidateStatusParams function
// for various status creation/update scenarios
func TestValidateStatusParams(t *testing.T) {
	tests := []struct {
		name      string
		params    map[string]interface{}
		expectErr bool
		errField  string
	}{
		{
			name: "valid status with content only",
			params: map[string]interface{}{
				"status": "Hello World!",
			},
			expectErr: false,
		},
		{
			name: "valid status with media only",
			params: map[string]interface{}{
				"media_ids": []interface{}{"media123"},
			},
			expectErr: false,
		},
		{
			name: "valid status with content and media",
			params: map[string]interface{}{
				"status":    "Check out this image!",
				"media_ids": []interface{}{"media123"},
			},
			expectErr: false,
		},
		{
			name: "valid status with all optional fields",
			params: map[string]interface{}{
				"status":       "Hello World!",
				"sensitive":    true,
				"spoiler_text": "Content warning",
				"visibility":   "public",
				"language":     "en",
			},
			expectErr: false,
		},
		{
			name: "valid status with poll",
			params: map[string]interface{}{
				"status": "What's your favorite color?",
				"poll": map[string]interface{}{
					"options":    []interface{}{"Red", "Blue", "Green"},
					"expires_in": float64(3600),
				},
			},
			expectErr: false,
		},
		{
			name: "valid status with reply",
			params: map[string]interface{}{
				"status":         "Great post!",
				"in_reply_to_id": "status123",
			},
			expectErr: false,
		},
		{
			name: "valid status with canonical remote reply parent url",
			params: map[string]interface{}{
				"status":         "Great post!",
				"in_reply_to_id": "https://remote.example/users/steward/statuses/seed-1",
			},
			expectErr: false,
		},
		{
			name:      "missing content and media",
			params:    map[string]interface{}{},
			expectErr: true,
			errField:  "status",
		},
		{
			name: "empty status and no media",
			params: map[string]interface{}{
				"status": "",
			},
			expectErr: true,
			errField:  "status",
		},
		{
			name: "whitespace-only status and no media",
			params: map[string]interface{}{
				"status": "   ",
			},
			expectErr: true,
			errField:  "status",
		},
		{
			name: "status too long",
			params: map[string]interface{}{
				"status": strings.Repeat("x", MaxStatusLength+1),
			},
			expectErr: true,
			errField:  "status",
		},
		{
			name: "exactly max length status",
			params: map[string]interface{}{
				"status": strings.Repeat("x", MaxStatusLength),
			},
			expectErr: false,
		},
		{
			name: "spoiler text too long",
			params: map[string]interface{}{
				"status":       "Hello",
				"spoiler_text": strings.Repeat("x", MaxStatusSpoiler+1),
			},
			expectErr: true,
			errField:  "spoiler_text",
		},
		{
			name: "invalid visibility",
			params: map[string]interface{}{
				"status":     "Hello",
				"visibility": "invalid",
			},
			expectErr: true,
			errField:  "visibility",
		},
		{
			name: "invalid reply parent url scheme",
			params: map[string]interface{}{
				"status":         "Hello",
				"in_reply_to_id": "ftp://remote.example/statuses/seed-1",
			},
			expectErr: true,
			errField:  "in_reply_to_id",
		},
		{
			name: "valid private visibility",
			params: map[string]interface{}{
				"status":     "Hello",
				"visibility": "private",
			},
			expectErr: false,
		},
		{
			name: "valid direct visibility",
			params: map[string]interface{}{
				"status":     "Hello",
				"visibility": "direct",
			},
			expectErr: false,
		},
		{
			name: "invalid language code",
			params: map[string]interface{}{
				"status":   "Hello",
				"language": "invalid",
			},
			expectErr: true,
			errField:  "language",
		},
		{
			name: "valid language code",
			params: map[string]interface{}{
				"status":   "Hello",
				"language": "en",
			},
			expectErr: false,
		},
		{
			name: "too many media attachments",
			params: map[string]interface{}{
				"media_ids": []interface{}{"1", "2", "3", "4", "5"},
			},
			expectErr: true,
			errField:  "media_ids",
		},
		{
			name: "exactly max media attachments",
			params: map[string]interface{}{
				"media_ids": []interface{}{"1", "2", "3", "4"},
			},
			expectErr: false,
		},
		{
			name: "sensitive must be boolean",
			params: map[string]interface{}{
				"status":    "Hello",
				"sensitive": "true",
			},
			expectErr: true,
			errField:  "sensitive",
		},
		{
			name: "empty media_ids array with status",
			params: map[string]interface{}{
				"status":    "Hello",
				"media_ids": []interface{}{},
			},
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateStatusParams(tt.params)
			if tt.expectErr {
				if err == nil {
					t.Errorf("ValidateStatusParams() expected error, got nil")
					return
				}
				if tt.errField != "" {
					verr, ok := err.(ValidationError)
					if ok && verr.Field != tt.errField {
						t.Errorf("ValidateStatusParams() error field = %v, want %v", verr.Field, tt.errField)
					}
				}
			} else {
				if err != nil {
					t.Errorf("ValidateStatusParams() unexpected error = %v", err)
				}
			}
		})
	}
}

// TestValidatePollParams tests the ValidatePollParams function
func TestValidatePollParams(t *testing.T) {
	tests := []struct {
		name      string
		poll      interface{}
		expectErr bool
		errField  string
	}{
		{
			name: "valid poll with 2 options",
			poll: map[string]interface{}{
				"options":    []interface{}{"Yes", "No"},
				"expires_in": float64(3600),
			},
			expectErr: false,
		},
		{
			name: "valid poll with 4 options",
			poll: map[string]interface{}{
				"options":    []interface{}{"A", "B", "C", "D"},
				"expires_in": float64(86400),
			},
			expectErr: false,
		},
		{
			name: "valid poll with optional fields",
			poll: map[string]interface{}{
				"options":     []interface{}{"Yes", "No"},
				"expires_in":  float64(3600),
				"multiple":    true,
				"hide_totals": false,
			},
			expectErr: false,
		},
		{
			name:      "poll not an object",
			poll:      "not-an-object",
			expectErr: true,
			errField:  "poll",
		},
		{
			name: "missing options",
			poll: map[string]interface{}{
				"expires_in": float64(3600),
			},
			expectErr: true,
			errField:  "poll.options",
		},
		{
			name: "missing expires_in",
			poll: map[string]interface{}{
				"options": []interface{}{"Yes", "No"},
			},
			expectErr: true,
			errField:  "poll.expires_in",
		},
		{
			name: "only 1 option",
			poll: map[string]interface{}{
				"options":    []interface{}{"Yes"},
				"expires_in": float64(3600),
			},
			expectErr: true,
			errField:  "poll.options",
		},
		{
			name: "too many options",
			poll: map[string]interface{}{
				"options":    []interface{}{"A", "B", "C", "D", "E"},
				"expires_in": float64(3600),
			},
			expectErr: true,
			errField:  "poll.options",
		},
		{
			name: "empty option string",
			poll: map[string]interface{}{
				"options":    []interface{}{"Yes", ""},
				"expires_in": float64(3600),
			},
			expectErr: true,
			errField:  "poll.options[1]",
		},
		{
			name: "option too long",
			poll: map[string]interface{}{
				"options":    []interface{}{"Yes", strings.Repeat("x", MaxPollOptionLength+1)},
				"expires_in": float64(3600),
			},
			expectErr: true,
			errField:  "poll.options[1]",
		},
		{
			name: "option not a string",
			poll: map[string]interface{}{
				"options":    []interface{}{"Yes", 123},
				"expires_in": float64(3600),
			},
			expectErr: true,
			errField:  "poll.options[1]",
		},
		{
			name: "expires_in too short",
			poll: map[string]interface{}{
				"options":    []interface{}{"Yes", "No"},
				"expires_in": float64(60), // 1 minute, min is 5 minutes
			},
			expectErr: true,
			errField:  "poll.expires_in",
		},
		{
			name: "expires_in too long",
			poll: map[string]interface{}{
				"options":    []interface{}{"Yes", "No"},
				"expires_in": float64(MaxPollDuration + 1),
			},
			expectErr: true,
			errField:  "poll.expires_in",
		},
		{
			name: "expires_in at minimum",
			poll: map[string]interface{}{
				"options":    []interface{}{"Yes", "No"},
				"expires_in": float64(MinPollDuration),
			},
			expectErr: false,
		},
		{
			name: "expires_in at maximum",
			poll: map[string]interface{}{
				"options":    []interface{}{"Yes", "No"},
				"expires_in": float64(MaxPollDuration),
			},
			expectErr: false,
		},
		{
			name: "multiple must be boolean",
			poll: map[string]interface{}{
				"options":    []interface{}{"Yes", "No"},
				"expires_in": float64(3600),
				"multiple":   "true",
			},
			expectErr: true,
			errField:  "poll.multiple",
		},
		{
			name: "hide_totals must be boolean",
			poll: map[string]interface{}{
				"options":     []interface{}{"Yes", "No"},
				"expires_in":  float64(3600),
				"hide_totals": "false",
			},
			expectErr: true,
			errField:  "poll.hide_totals",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePollParams(tt.poll)
			if tt.expectErr {
				if err == nil {
					t.Errorf("ValidatePollParams() expected error, got nil")
					return
				}
				if tt.errField != "" {
					verr, ok := err.(ValidationError)
					if ok && verr.Field != tt.errField {
						t.Errorf("ValidatePollParams() error field = %v, want %v", verr.Field, tt.errField)
					}
				}
			} else {
				if err != nil {
					t.Errorf("ValidatePollParams() unexpected error = %v", err)
				}
			}
		})
	}
}

// TestValidateMediaParams tests the ValidateMediaParams function
func TestValidateMediaParams(t *testing.T) {
	tests := []struct {
		name      string
		params    map[string]interface{}
		expectErr bool
		errField  string
	}{
		{
			name: "valid media with file only",
			params: map[string]interface{}{
				"file": "base64-encoded-data",
			},
			expectErr: false,
		},
		{
			name: "valid media with description",
			params: map[string]interface{}{
				"file":        "base64-encoded-data",
				"description": "A beautiful sunset",
			},
			expectErr: false,
		},
		{
			name: "valid media with focus",
			params: map[string]interface{}{
				"file":  "base64-encoded-data",
				"focus": "0.5,-0.5",
			},
			expectErr: false,
		},
		{
			name:      "missing file",
			params:    map[string]interface{}{},
			expectErr: true,
			errField:  "file",
		},
		{
			name: "file not a string",
			params: map[string]interface{}{
				"file": 123,
			},
			expectErr: true,
			errField:  "file",
		},
		{
			name: "empty file",
			params: map[string]interface{}{
				"file": "",
			},
			expectErr: true,
			errField:  "file",
		},
		{
			name: "description too long",
			params: map[string]interface{}{
				"file":        "base64-encoded-data",
				"description": strings.Repeat("x", MaxMediaDescLength+1),
			},
			expectErr: true,
			errField:  "description",
		},
		{
			name: "max length description",
			params: map[string]interface{}{
				"file":        "base64-encoded-data",
				"description": strings.Repeat("x", MaxMediaDescLength),
			},
			expectErr: false,
		},
		{
			name: "invalid focus format",
			params: map[string]interface{}{
				"file":  "base64-encoded-data",
				"focus": "invalid",
			},
			expectErr: true,
			errField:  "focus",
		},
		{
			name: "focus out of range",
			params: map[string]interface{}{
				"file":  "base64-encoded-data",
				"focus": "1.5,0",
			},
			expectErr: true,
			errField:  "focus",
		},
		{
			name: "focus with valid coordinates",
			params: map[string]interface{}{
				"file":  "base64-encoded-data",
				"focus": "-1.0,1.0",
			},
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateMediaParams(tt.params)
			if tt.expectErr {
				if err == nil {
					t.Errorf("ValidateMediaParams() expected error, got nil")
					return
				}
				if tt.errField != "" {
					verr, ok := err.(ValidationError)
					if ok && verr.Field != tt.errField {
						t.Errorf("ValidateMediaParams() error field = %v, want %v", verr.Field, tt.errField)
					}
				}
			} else {
				if err != nil {
					t.Errorf("ValidateMediaParams() unexpected error = %v", err)
				}
			}
		})
	}
}

// TestValidateFilterParams tests the ValidateFilterParams function
func TestValidateFilterParams(t *testing.T) {
	tests := []struct {
		name      string
		params    map[string]interface{}
		expectErr bool
		errField  string
	}{
		{
			name: "valid filter with required fields",
			params: map[string]interface{}{
				"title":   "Spam filter",
				"context": []interface{}{"home", "public"},
			},
			expectErr: false,
		},
		{
			name: "valid filter with all fields",
			params: map[string]interface{}{
				"title":         "Spam filter",
				"context":       []interface{}{"home"},
				"filter_action": "hide",
				"expires_in":    float64(86400),
				"keywords_attributes": []interface{}{
					map[string]interface{}{
						"keyword":    "spam",
						"whole_word": true,
					},
				},
			},
			expectErr: false,
		},
		{
			name: "missing title",
			params: map[string]interface{}{
				"context": []interface{}{"home"},
			},
			expectErr: true,
			errField:  "title",
		},
		{
			name: "empty title",
			params: map[string]interface{}{
				"title":   "",
				"context": []interface{}{"home"},
			},
			expectErr: true,
			errField:  "title",
		},
		{
			name: "title too long",
			params: map[string]interface{}{
				"title":   strings.Repeat("x", MaxFilterTitleLength+1),
				"context": []interface{}{"home"},
			},
			expectErr: true,
			errField:  "title",
		},
		{
			name: "missing context",
			params: map[string]interface{}{
				"title": "Filter",
			},
			expectErr: true,
			errField:  "context",
		},
		{
			name: "empty context array",
			params: map[string]interface{}{
				"title":   "Filter",
				"context": []interface{}{},
			},
			expectErr: true,
			errField:  "context",
		},
		{
			name: "invalid context value",
			params: map[string]interface{}{
				"title":   "Filter",
				"context": []interface{}{"invalid"},
			},
			expectErr: true,
			errField:  "context[0]",
		},
		{
			name: "context not an array",
			params: map[string]interface{}{
				"title":   "Filter",
				"context": "home",
			},
			expectErr: true,
			errField:  "context",
		},
		{
			name: "valid all contexts",
			params: map[string]interface{}{
				"title":   "Filter",
				"context": []interface{}{"home", "notifications", "public", "thread", "account"},
			},
			expectErr: false,
		},
		{
			name: "expires_in negative",
			params: map[string]interface{}{
				"title":      "Filter",
				"context":    []interface{}{"home"},
				"expires_in": float64(-1),
			},
			expectErr: true,
			errField:  "expires_in",
		},
		{
			name: "too many keywords",
			params: map[string]interface{}{
				"title":   "Filter",
				"context": []interface{}{"home"},
				"keywords_attributes": func() []interface{} {
					keywords := make([]interface{}, MaxFilterKeywords+1)
					for i := range keywords {
						keywords[i] = map[string]interface{}{"keyword": "spam"}
					}
					return keywords
				}(),
			},
			expectErr: true,
			errField:  "keywords_attributes",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateFilterParams(tt.params)
			if tt.expectErr {
				if err == nil {
					t.Errorf("ValidateFilterParams() expected error, got nil")
					return
				}
				if tt.errField != "" {
					verr, ok := err.(ValidationError)
					if ok && verr.Field != tt.errField {
						t.Errorf("ValidateFilterParams() error field = %v, want %v", verr.Field, tt.errField)
					}
				}
			} else {
				if err != nil {
					t.Errorf("ValidateFilterParams() unexpected error = %v", err)
				}
			}
		})
	}
}

// TestValidateListParams tests the ValidateListParams function
func TestValidateListParams(t *testing.T) {
	tests := []struct {
		name      string
		params    map[string]interface{}
		expectErr bool
		errField  string
	}{
		{
			name: "valid list with title only",
			params: map[string]interface{}{
				"title": "My List",
			},
			expectErr: false,
		},
		{
			name: "valid list with replies_policy",
			params: map[string]interface{}{
				"title":          "My List",
				"replies_policy": "followed",
			},
			expectErr: false,
		},
		{
			name: "valid replies_policy list",
			params: map[string]interface{}{
				"title":          "My List",
				"replies_policy": "list",
			},
			expectErr: false,
		},
		{
			name: "valid replies_policy none",
			params: map[string]interface{}{
				"title":          "My List",
				"replies_policy": "none",
			},
			expectErr: false,
		},
		{
			name:      "missing title",
			params:    map[string]interface{}{},
			expectErr: true,
			errField:  "title",
		},
		{
			name: "empty title",
			params: map[string]interface{}{
				"title": "",
			},
			expectErr: true,
			errField:  "title",
		},
		{
			name: "title too long",
			params: map[string]interface{}{
				"title": strings.Repeat("x", MaxListTitleLength+1),
			},
			expectErr: true,
			errField:  "title",
		},
		{
			name: "title not a string",
			params: map[string]interface{}{
				"title": 123,
			},
			expectErr: true,
			errField:  "title",
		},
		{
			name: "invalid replies_policy",
			params: map[string]interface{}{
				"title":          "My List",
				"replies_policy": "invalid",
			},
			expectErr: true,
			errField:  "replies_policy",
		},
		{
			name: "max length title",
			params: map[string]interface{}{
				"title": strings.Repeat("x", MaxListTitleLength),
			},
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateListParams(tt.params)
			if tt.expectErr {
				if err == nil {
					t.Errorf("ValidateListParams() expected error, got nil")
					return
				}
				if tt.errField != "" {
					verr, ok := err.(ValidationError)
					if ok && verr.Field != tt.errField {
						t.Errorf("ValidateListParams() error field = %v, want %v", verr.Field, tt.errField)
					}
				}
			} else {
				if err != nil {
					t.Errorf("ValidateListParams() unexpected error = %v", err)
				}
			}
		})
	}
}

// TestValidateApplicationParams tests the ValidateApplicationParams function
func TestValidateApplicationParams(t *testing.T) {
	tests := []struct {
		name      string
		params    map[string]interface{}
		expectErr bool
		errField  string
	}{
		{
			name: "valid app with required fields",
			params: map[string]interface{}{
				"client_name":   "My App",
				"redirect_uris": "urn:ietf:wg:oauth:2.0:oob",
			},
			expectErr: false,
		},
		{
			name: "valid app with all fields",
			params: map[string]interface{}{
				"client_name":   "My App",
				"redirect_uris": "https://example.com/callback",
				"scopes":        "read write",
				"website":       "https://example.com",
			},
			expectErr: false,
		},
		{
			name: "valid app with multiple redirect URIs",
			params: map[string]interface{}{
				"client_name":   "My App",
				"redirect_uris": "https://example.com/callback https://example.com/auth",
			},
			expectErr: false,
		},
		{
			name: "missing client_name",
			params: map[string]interface{}{
				"redirect_uris": "https://example.com/callback",
			},
			expectErr: true,
			errField:  "client_name",
		},
		{
			name: "empty client_name",
			params: map[string]interface{}{
				"client_name":   "",
				"redirect_uris": "https://example.com/callback",
			},
			expectErr: true,
			errField:  "client_name",
		},
		{
			name: "client_name too long",
			params: map[string]interface{}{
				"client_name":   strings.Repeat("x", MaxAppNameLength+1),
				"redirect_uris": "https://example.com/callback",
			},
			expectErr: true,
			errField:  "client_name",
		},
		{
			name: "missing redirect_uris",
			params: map[string]interface{}{
				"client_name": "My App",
			},
			expectErr: true,
			errField:  "redirect_uris",
		},
		{
			name: "empty redirect_uris",
			params: map[string]interface{}{
				"client_name":   "My App",
				"redirect_uris": "",
			},
			expectErr: true,
			errField:  "redirect_uris",
		},
		{
			name: "invalid redirect_uri",
			params: map[string]interface{}{
				"client_name":   "My App",
				"redirect_uris": "not-a-valid-uri",
			},
			expectErr: true,
			errField:  "redirect_uris",
		},
		{
			name: "invalid scope",
			params: map[string]interface{}{
				"client_name":   "My App",
				"redirect_uris": "urn:ietf:wg:oauth:2.0:oob",
				"scopes":        "invalid_scope",
			},
			expectErr: true,
			errField:  "scopes",
		},
		{
			name: "valid hierarchical scopes",
			params: map[string]interface{}{
				"client_name":   "My App",
				"redirect_uris": "urn:ietf:wg:oauth:2.0:oob",
				"scopes":        "read:accounts write:statuses",
			},
			expectErr: false,
		},
		{
			name: "scopes too long",
			params: map[string]interface{}{
				"client_name":   "My App",
				"redirect_uris": "urn:ietf:wg:oauth:2.0:oob",
				"scopes":        strings.Repeat("read ", MaxAppScopesLength/5+1),
			},
			expectErr: true,
			errField:  "scopes",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateApplicationParams(tt.params)
			if tt.expectErr {
				if err == nil {
					t.Errorf("ValidateApplicationParams() expected error, got nil")
					return
				}
				if tt.errField != "" {
					verr, ok := err.(ValidationError)
					if ok && verr.Field != tt.errField {
						t.Errorf("ValidateApplicationParams() error field = %v, want %v", verr.Field, tt.errField)
					}
				}
			} else {
				if err != nil {
					t.Errorf("ValidateApplicationParams() unexpected error = %v", err)
				}
			}
		})
	}
}

// TestValidateMastodonStatusID tests the ValidateMastodonStatusID function
func TestValidateMastodonStatusID(t *testing.T) {
	tests := []struct {
		name      string
		statusID  string
		expectErr bool
	}{
		{
			name:      "valid numeric ID",
			statusID:  "12345",
			expectErr: false,
		},
		{
			name:      "valid alphanumeric ID",
			statusID:  "abc123",
			expectErr: false,
		},
		{
			name:      "valid ID with underscore",
			statusID:  "status_123",
			expectErr: false,
		},
		{
			name:      "valid ID with hyphen",
			statusID:  "status-123",
			expectErr: false,
		},
		{
			name:      "empty ID",
			statusID:  "",
			expectErr: true,
		},
		{
			name:      "ID too long",
			statusID:  strings.Repeat("x", 101),
			expectErr: true,
		},
		{
			name:      "ID with invalid characters",
			statusID:  "status@123",
			expectErr: true,
		},
		{
			name:      "ID with spaces",
			statusID:  "status 123",
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateMastodonStatusID(tt.statusID)
			if (err != nil) != tt.expectErr {
				t.Errorf("ValidateMastodonStatusID() error = %v, expectErr %v", err, tt.expectErr)
			}
		})
	}
}

// TestValidateMastodonAccountID tests the ValidateMastodonAccountID function
func TestValidateMastodonAccountID(t *testing.T) {
	tests := []struct {
		name      string
		accountID string
		expectErr bool
	}{
		{
			name:      "valid numeric ID",
			accountID: "12345",
			expectErr: false,
		},
		{
			name:      "valid alphanumeric ID",
			accountID: "abc123",
			expectErr: false,
		},
		{
			name:      "valid ID with dots and hyphens",
			accountID: "user.name-123",
			expectErr: false,
		},
		{
			name:      "valid ID with @ for federation",
			accountID: "user@example.com",
			expectErr: false,
		},
		{
			name:      "valid ID with path",
			accountID: "https://example.com/users/alice",
			expectErr: false,
		},
		{
			name:      "empty ID",
			accountID: "",
			expectErr: true,
		},
		{
			name:      "ID too long",
			accountID: strings.Repeat("x", 501),
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateMastodonAccountID(tt.accountID)
			if (err != nil) != tt.expectErr {
				t.Errorf("ValidateMastodonAccountID() error = %v, expectErr %v", err, tt.expectErr)
			}
		})
	}
}

// TestValidateHashtag tests the ValidateHashtag function
func TestValidateHashtag(t *testing.T) {
	tests := []struct {
		name      string
		hashtag   string
		expectErr bool
	}{
		{
			name:      "valid simple hashtag",
			hashtag:   "hello",
			expectErr: false,
		},
		{
			name:      "valid hashtag with hash prefix",
			hashtag:   "#hello",
			expectErr: false,
		},
		{
			name:      "valid hashtag with numbers",
			hashtag:   "hello123",
			expectErr: false,
		},
		{
			name:      "valid hashtag with underscore",
			hashtag:   "hello_world",
			expectErr: false,
		},
		{
			name:      "empty hashtag",
			hashtag:   "",
			expectErr: true,
		},
		{
			name:      "hashtag too long",
			hashtag:   strings.Repeat("x", 101),
			expectErr: true,
		},
		{
			name:      "hashtag with spaces",
			hashtag:   "hello world",
			expectErr: true,
		},
		{
			name:      "hashtag with special chars",
			hashtag:   "hello@world",
			expectErr: true,
		},
		{
			name:      "max length hashtag",
			hashtag:   strings.Repeat("x", 100),
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateHashtag(tt.hashtag)
			if (err != nil) != tt.expectErr {
				t.Errorf("ValidateHashtag() error = %v, expectErr %v", err, tt.expectErr)
			}
		})
	}
}

// TestValidateMastodonMimeType tests the ValidateMastodonMimeType function
func TestValidateMastodonMimeType(t *testing.T) {
	tests := []struct {
		name      string
		mimeType  string
		expectErr bool
	}{
		{
			name:      "valid image/jpeg",
			mimeType:  "image/jpeg",
			expectErr: false,
		},
		{
			name:      "valid image/png",
			mimeType:  "image/png",
			expectErr: false,
		},
		{
			name:      "valid image/gif",
			mimeType:  "image/gif",
			expectErr: false,
		},
		{
			name:      "valid image/webp",
			mimeType:  "image/webp",
			expectErr: false,
		},
		{
			name:      "valid video/mp4",
			mimeType:  "video/mp4",
			expectErr: false,
		},
		{
			name:      "valid video/webm",
			mimeType:  "video/webm",
			expectErr: false,
		},
		{
			name:      "valid audio/mpeg",
			mimeType:  "audio/mpeg",
			expectErr: false,
		},
		{
			name:      "valid audio/ogg",
			mimeType:  "audio/ogg",
			expectErr: false,
		},
		{
			name:      "empty mime type",
			mimeType:  "",
			expectErr: true,
		},
		{
			name:      "unsupported mime type",
			mimeType:  "application/pdf",
			expectErr: true,
		},
		{
			name:      "invalid mime type format",
			mimeType:  "not-a-mime-type",
			expectErr: true,
		},
		{
			name:      "text/plain not allowed",
			mimeType:  "text/plain",
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateMastodonMimeType(tt.mimeType)
			if (err != nil) != tt.expectErr {
				t.Errorf("ValidateMastodonMimeType() error = %v, expectErr %v", err, tt.expectErr)
			}
		})
	}
}

// TestValidateAccountFields tests the ValidateAccountFields function
func TestValidateAccountFields(t *testing.T) {
	tests := []struct {
		name      string
		fields    interface{}
		expectErr bool
		errField  string
	}{
		{
			name:      "empty fields array",
			fields:    []interface{}{},
			expectErr: false,
		},
		{
			name: "valid single field",
			fields: []interface{}{
				map[string]interface{}{
					"name":  "Website",
					"value": "https://example.com",
				},
			},
			expectErr: false,
		},
		{
			name: "valid multiple fields",
			fields: []interface{}{
				map[string]interface{}{"name": "Website", "value": "https://example.com"},
				map[string]interface{}{"name": "GitHub", "value": "@user"},
				map[string]interface{}{"name": "Pronouns", "value": "they/them"},
				map[string]interface{}{"name": "Location", "value": "Earth"},
			},
			expectErr: false,
		},
		{
			name:      "not an array",
			fields:    "not-an-array",
			expectErr: true,
			errField:  "fields_attributes",
		},
		{
			name: "too many fields",
			fields: []interface{}{
				map[string]interface{}{"name": "1", "value": "v"},
				map[string]interface{}{"name": "2", "value": "v"},
				map[string]interface{}{"name": "3", "value": "v"},
				map[string]interface{}{"name": "4", "value": "v"},
				map[string]interface{}{"name": "5", "value": "v"},
			},
			expectErr: true,
			errField:  "fields_attributes",
		},
		{
			name: "field not an object",
			fields: []interface{}{
				"not-an-object",
			},
			expectErr: true,
			errField:  "fields_attributes[0]",
		},
		{
			name: "field name too long",
			fields: []interface{}{
				map[string]interface{}{
					"name":  strings.Repeat("x", MaxFieldNameLength+1),
					"value": "test",
				},
			},
			expectErr: true,
			errField:  "fields_attributes[0].name",
		},
		{
			name: "field value too long",
			fields: []interface{}{
				map[string]interface{}{
					"name":  "Test",
					"value": strings.Repeat("x", MaxFieldValueLength+1),
				},
			},
			expectErr: true,
			errField:  "fields_attributes[0].value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateAccountFields(tt.fields)
			if tt.expectErr {
				if err == nil {
					t.Errorf("ValidateAccountFields() expected error, got nil")
					return
				}
				if tt.errField != "" {
					verr, ok := err.(ValidationError)
					if ok && verr.Field != tt.errField {
						t.Errorf("ValidateAccountFields() error field = %v, want %v", verr.Field, tt.errField)
					}
				}
			} else {
				if err != nil {
					t.Errorf("ValidateAccountFields() unexpected error = %v", err)
				}
			}
		})
	}
}

// TestValidateMediaFocus tests the ValidateMediaFocus function
func TestValidateMediaFocus(t *testing.T) {
	tests := []struct {
		name      string
		focus     string
		expectErr bool
	}{
		{
			name:      "valid center focus",
			focus:     "0,0",
			expectErr: false,
		},
		{
			name:      "valid top-left focus",
			focus:     "-1,-1",
			expectErr: false,
		},
		{
			name:      "valid bottom-right focus",
			focus:     "1,1",
			expectErr: false,
		},
		{
			name:      "valid fractional focus",
			focus:     "0.5,-0.5",
			expectErr: false,
		},
		{
			name:      "empty focus allowed",
			focus:     "",
			expectErr: false,
		},
		{
			name:      "invalid format - missing comma",
			focus:     "0.5",
			expectErr: true,
		},
		{
			name:      "invalid format - too many parts",
			focus:     "0,0,0",
			expectErr: true,
		},
		{
			name:      "out of range - x too high",
			focus:     "1.1,0",
			expectErr: true,
		},
		{
			name:      "out of range - y too low",
			focus:     "0,-1.1",
			expectErr: true,
		},
		{
			name:      "invalid number",
			focus:     "abc,def",
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateMediaFocus(tt.focus)
			if (err != nil) != tt.expectErr {
				t.Errorf("ValidateMediaFocus() error = %v, expectErr %v", err, tt.expectErr)
			}
		})
	}
}

// TestValidateReportParams tests the ValidateReportParams function
func TestValidateReportParams(t *testing.T) {
	tests := []struct {
		name      string
		params    map[string]interface{}
		expectErr bool
		errField  string
	}{
		{
			name: "valid report with account_id only",
			params: map[string]interface{}{
				"account_id": "user123",
			},
			expectErr: false,
		},
		{
			name: "valid report with all fields",
			params: map[string]interface{}{
				"account_id": "user123",
				"status_ids": []interface{}{"status1", "status2"},
				"comment":    "This user is posting spam",
				"category":   "spam",
				"forward":    true,
			},
			expectErr: false,
		},
		{
			name:      "missing account_id",
			params:    map[string]interface{}{},
			expectErr: true,
			errField:  "account_id",
		},
		{
			name: "empty account_id",
			params: map[string]interface{}{
				"account_id": "",
			},
			expectErr: true,
			errField:  "account_id",
		},
		{
			name: "account_id not a string",
			params: map[string]interface{}{
				"account_id": 123,
			},
			expectErr: true,
			errField:  "account_id",
		},
		{
			name: "too many status_ids",
			params: map[string]interface{}{
				"account_id": "user123",
				"status_ids": func() []interface{} {
					ids := make([]interface{}, 21)
					for i := range ids {
						ids[i] = "status"
					}
					return ids
				}(),
			},
			expectErr: true,
			errField:  "status_ids",
		},
		{
			name: "comment too long",
			params: map[string]interface{}{
				"account_id": "user123",
				"comment":    strings.Repeat("x", 1001),
			},
			expectErr: true,
			errField:  "comment",
		},
		{
			name: "invalid category",
			params: map[string]interface{}{
				"account_id": "user123",
				"category":   "invalid",
			},
			expectErr: true,
			errField:  "category",
		},
		{
			name: "valid spam category",
			params: map[string]interface{}{
				"account_id": "user123",
				"category":   "spam",
			},
			expectErr: false,
		},
		{
			name: "valid violation category",
			params: map[string]interface{}{
				"account_id": "user123",
				"category":   "violation",
			},
			expectErr: false,
		},
		{
			name: "valid other category",
			params: map[string]interface{}{
				"account_id": "user123",
				"category":   "other",
			},
			expectErr: false,
		},
		{
			name: "forward must be boolean",
			params: map[string]interface{}{
				"account_id": "user123",
				"forward":    "true",
			},
			expectErr: true,
			errField:  "forward",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateReportParams(tt.params)
			if tt.expectErr {
				if err == nil {
					t.Errorf("ValidateReportParams() expected error, got nil")
					return
				}
				if tt.errField != "" {
					verr, ok := err.(ValidationError)
					if ok && verr.Field != tt.errField {
						t.Errorf("ValidateReportParams() error field = %v, want %v", verr.Field, tt.errField)
					}
				}
			} else {
				if err != nil {
					t.Errorf("ValidateReportParams() unexpected error = %v", err)
				}
			}
		})
	}
}

// TestValidateRedirectURIs tests the ValidateRedirectURIs function
func TestValidateRedirectURIs(t *testing.T) {
	tests := []struct {
		name      string
		uris      string
		expectErr bool
	}{
		{
			name:      "valid OOB URI",
			uris:      "urn:ietf:wg:oauth:2.0:oob",
			expectErr: false,
		},
		{
			name:      "valid HTTPS URI",
			uris:      "https://example.com/callback",
			expectErr: false,
		},
		{
			name:      "valid HTTP URI",
			uris:      "http://localhost:3000/callback",
			expectErr: false,
		},
		{
			name:      "multiple valid URIs",
			uris:      "https://example.com/callback https://example.com/auth",
			expectErr: false,
		},
		{
			name:      "OOB with HTTPS",
			uris:      "urn:ietf:wg:oauth:2.0:oob https://example.com/callback",
			expectErr: false,
		},
		{
			name:      "empty URIs",
			uris:      "",
			expectErr: true,
		},
		{
			name:      "invalid URI",
			uris:      "not-a-valid-uri",
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateRedirectURIs(tt.uris)
			if (err != nil) != tt.expectErr {
				t.Errorf("ValidateRedirectURIs() error = %v, expectErr %v", err, tt.expectErr)
			}
		})
	}
}

// TestValidateApplicationScopes tests the ValidateApplicationScopes function
func TestValidateApplicationScopes(t *testing.T) {
	tests := []struct {
		name      string
		scopes    string
		expectErr bool
	}{
		{
			name:      "valid single scope",
			scopes:    "read",
			expectErr: false,
		},
		{
			name:      "valid multiple scopes",
			scopes:    "read write follow",
			expectErr: false,
		},
		{
			name:      "valid hierarchical scope",
			scopes:    "read:accounts",
			expectErr: false,
		},
		{
			name:      "valid multiple hierarchical scopes",
			scopes:    "read:accounts write:statuses push",
			expectErr: false,
		},
		{
			name:      "invalid base scope",
			scopes:    "invalid",
			expectErr: true,
		},
		{
			name:      "invalid hierarchical scope base",
			scopes:    "invalid:accounts",
			expectErr: true,
		},
		{
			name:      "scopes too long",
			scopes:    strings.Repeat("read ", MaxAppScopesLength/5+1),
			expectErr: true,
		},
		{
			name:      "admin scope",
			scopes:    "admin",
			expectErr: false,
		},
		{
			name:      "push scope",
			scopes:    "push",
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateApplicationScopes(tt.scopes)
			if (err != nil) != tt.expectErr {
				t.Errorf("ValidateApplicationScopes() error = %v, expectErr %v", err, tt.expectErr)
			}
		})
	}
}

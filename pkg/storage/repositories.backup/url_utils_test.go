package repositories

import (
	"context"
	"testing"

	"go.uber.org/zap"
)

func TestURLValidator_ExtractAndValidateURL(t *testing.T) {
	logger := zap.NewNop()
	validator := NewURLValidator(logger)
	ctx := context.Background()

	testCases := []struct {
		name           string
		url            string
		expectValid    bool
		expectUsername string
		expectPlatform string
		expectSecure   bool
		expectSocial   bool
	}{
		{
			name:           "Twitter URL",
			url:            "https://twitter.com/username",
			expectValid:    true,
			expectUsername: "username",
			expectPlatform: "twitter",
			expectSecure:   true,
			expectSocial:   true,
		},
		{
			name:           "X.com URL",
			url:            "https://x.com/username",
			expectValid:    true,
			expectUsername: "username",
			expectPlatform: "twitter",
			expectSecure:   true,
			expectSocial:   true,
		},
		{
			name:           "Mastodon URL",
			url:            "https://mastodon.social/@alice",
			expectValid:    true,
			expectUsername: "alice",
			expectPlatform: "mastodon",
			expectSecure:   true,
			expectSocial:   true,
		},
		{
			name:           "GitHub URL",
			url:            "https://github.com/user",
			expectValid:    true,
			expectUsername: "user",
			expectPlatform: "github",
			expectSecure:   true,
			expectSocial:   true,
		},
		{
			name:           "ActivityPub URL",
			url:            "https://example.com/users/alice",
			expectValid:    true,
			expectUsername: "alice",
			expectPlatform: "activitypub",
			expectSecure:   true,
			expectSocial:   false,
		},
		{
			name:           "URL without protocol",
			url:            "github.com/user",
			expectValid:    true,
			expectUsername: "user",
			expectPlatform: "github",
			expectSecure:   true,
			expectSocial:   true,
		},
		{
			name:         "HTTP URL (insecure)",
			url:          "http://example.com",
			expectValid:  true,
			expectSecure: false,
		},
		{
			name:        "Invalid URL",
			url:         "not-a-url",
			expectValid: false,
		},
		{
			name:        "Empty URL",
			url:         "",
			expectValid: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := validator.ExtractAndValidateURL(ctx, tc.url)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if result.IsValid != tc.expectValid {
				t.Errorf("expected IsValid=%v, got %v", tc.expectValid, result.IsValid)
			}

			if tc.expectValid {
				if result.Username != tc.expectUsername {
					t.Errorf("expected Username=%q, got %q", tc.expectUsername, result.Username)
				}

				if result.ProfileType != tc.expectPlatform {
					t.Errorf("expected ProfileType=%q, got %q", tc.expectPlatform, result.ProfileType)
				}

				if result.IsSecure != tc.expectSecure {
					t.Errorf("expected IsSecure=%v, got %v", tc.expectSecure, result.IsSecure)
				}

				if result.IsSocial != tc.expectSocial {
					t.Errorf("expected IsSocial=%v, got %v", tc.expectSocial, result.IsSocial)
				}
			}
		})
	}
}

func TestURLValidator_EnhancedExtractAccountFromReply(t *testing.T) {
	logger := zap.NewNop()
	validator := NewURLValidator(logger)
	ctx := context.Background()

	testCases := []struct {
		name           string
		inReplyTo      string
		expectUsername string
	}{
		{
			name:           "POST format",
			inReplyTo:      "POST#alice#1234567890",
			expectUsername: "alice",
		},
		{
			name:           "Mastodon user URL",
			inReplyTo:      "https://mastodon.social/users/bob",
			expectUsername: "bob",
		},
		{
			name:           "Mastodon @ URL",
			inReplyTo:      "https://mastodon.social/@charlie",
			expectUsername: "charlie",
		},
		{
			name:           "ActivityPub actor URL",
			inReplyTo:      "https://example.com/actors/dave",
			expectUsername: "dave",
		},
		{
			name:           "Profile URL",
			inReplyTo:      "https://social.example.com/profile/eve",
			expectUsername: "eve",
		},
		{
			name:           "Twitter status URL with username path",
			inReplyTo:      "https://twitter.com/frank/status/1234567890",
			expectUsername: "frank",
		},
		{
			name:           "Invalid URL",
			inReplyTo:      "not-a-valid-url",
			expectUsername: "",
		},
		{
			name:           "Empty string",
			inReplyTo:      "",
			expectUsername: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			username, err := validator.EnhancedExtractAccountFromReply(ctx, tc.inReplyTo)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if username != tc.expectUsername {
				t.Errorf("expected username=%q, got %q", tc.expectUsername, username)
			}
		})
	}
}

func TestURLValidator_ExtractProfileURLs(t *testing.T) {
	logger := zap.NewNop()
	validator := NewURLValidator(logger)
	ctx := context.Background()

	fields := []map[string]string{
		{
			"name":  "Website",
			"value": "https://example.com",
		},
		{
			"name":  "Twitter",
			"value": "Follow me at https://twitter.com/username!",
		},
		{
			"name":  "GitHub",
			"value": "github.com/user",
		},
		{
			"name":  "No URL",
			"value": "Just text",
		},
	}

	results, err := validator.ExtractProfileURLs(ctx, fields)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(results) != 3 {
		t.Errorf("expected 3 URL results, got %d", len(results))
	}

	// Check that Twitter URL was extracted with username
	twitterFound := false
	for _, result := range results {
		if result.ProfileType == "twitter" && result.Username == "username" {
			twitterFound = true
			break
		}
	}
	if !twitterFound {
		t.Error("expected to find Twitter URL with username extracted")
	}

	// Check that GitHub URL was extracted
	githubFound := false
	for _, result := range results {
		if result.ProfileType == "github" && result.Username == "user" {
			githubFound = true
			break
		}
	}
	if !githubFound {
		t.Error("expected to find GitHub URL with username extracted")
	}
}

func TestURLValidator_ValidateAndNormalizeProfileURLs(t *testing.T) {
	logger := zap.NewNop()
	validator := NewURLValidator(logger)
	ctx := context.Background()

	fields := []map[string]string{
		{
			"name":  "Website",
			"value": "example.com", // Should be normalized to https://example.com
		},
		{
			"name":  "Insecure",
			"value": "http://insecure.com", // Should generate warning
		},
		{
			"name":  "Valid",
			"value": "https://secure.com",
		},
	}

	normalizedFields, warnings, err := validator.ValidateAndNormalizeProfileURLs(ctx, fields)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(normalizedFields) != 3 {
		t.Errorf("expected 3 normalized fields, got %d", len(normalizedFields))
	}

	// Check normalization
	if normalizedFields[0]["value"] != "https://example.com" {
		t.Errorf("expected normalized URL to be 'https://example.com', got %q", normalizedFields[0]["value"])
	}

	// Check warnings
	if len(warnings) == 0 {
		t.Error("expected warnings for insecure HTTP URL")
	}
}

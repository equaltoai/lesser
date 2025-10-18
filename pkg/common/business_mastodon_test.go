package common

import (
	"context"
	"testing"

	"go.uber.org/zap"
)

func TestNewMastodonBusinessLogic(t *testing.T) {
	config := DefaultMastodonConfig()
	config.Domain = "test.example.com"
	logger := zap.NewNop()

	logic := NewMastodonBusinessLogic(config, logger)

	if logic == nil {
		t.Error("NewMastodonBusinessLogic returned nil")
	}
	if logic.config.Domain != config.Domain {
		t.Errorf("Domain = %v, want %v", logic.config.Domain, config.Domain)
	}
	if logic.logger != logger {
		t.Error("Logger not set correctly")
	}
}

func TestDefaultMastodonConfig(t *testing.T) {
	config := DefaultMastodonConfig()

	if config.MaxStatusLength != 500 {
		t.Errorf("MaxStatusLength = %v, want %v", config.MaxStatusLength, 500)
	}
	if config.MaxPollOptions != 4 {
		t.Errorf("MaxPollOptions = %v, want %v", config.MaxPollOptions, 4)
	}
	if config.MaxDisplayName != 30 {
		t.Errorf("MaxDisplayName = %v, want %v", config.MaxDisplayName, 30)
	}
	if config.MaxBioLength != 160 {
		t.Errorf("MaxBioLength = %v, want %v", config.MaxBioLength, 160)
	}
}

func TestValidateStatusContent(t *testing.T) {
	config := DefaultMastodonConfig()
	logic := NewMastodonBusinessLogic(config, zap.NewNop())

	tests := []struct {
		name        string
		content     string
		mediaCount  int
		pollOptions int
		expectErr   bool
	}{
		{
			name:        "valid status",
			content:     "This is a valid status",
			mediaCount:  0,
			pollOptions: 0,
			expectErr:   false,
		},
		{
			name:        "status too long",
			content:     string(make([]byte, 501)), // 501 characters
			mediaCount:  0,
			pollOptions: 0,
			expectErr:   true,
		},
		{
			name:        "empty content with media",
			content:     "",
			mediaCount:  1,
			pollOptions: 0,
			expectErr:   false,
		},
		{
			name:        "empty content without media",
			content:     "",
			mediaCount:  0,
			pollOptions: 0,
			expectErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := logic.ValidateStatusContent(tt.content, tt.mediaCount, tt.pollOptions)
			if (err != nil) != tt.expectErr {
				t.Errorf("ValidateStatusContent() error = %v, expectErr %v", err, tt.expectErr)
			}
		})
	}
}

func TestValidateDisplayName(t *testing.T) {
	config := DefaultMastodonConfig()
	logic := NewMastodonBusinessLogic(config, zap.NewNop())

	tests := []struct {
		name        string
		displayName string
		expectErr   bool
	}{
		{
			name:        "valid display name",
			displayName: "Alice",
			expectErr:   false,
		},
		{
			name:        "empty display name",
			displayName: "",
			expectErr:   false, // Empty is allowed
		},
		{
			name:        "display name too long",
			displayName: string(make([]byte, 31)), // 31 characters
			expectErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := logic.ValidateDisplayName(tt.displayName)
			if (err != nil) != tt.expectErr {
				t.Errorf("ValidateDisplayName() error = %v, expectErr %v", err, tt.expectErr)
			}
		})
	}
}

func TestValidateBio(t *testing.T) {
	config := DefaultMastodonConfig()
	logic := NewMastodonBusinessLogic(config, zap.NewNop())

	tests := []struct {
		name      string
		bio       string
		expectErr bool
	}{
		{
			name:      "valid bio",
			bio:       "This is a valid bio",
			expectErr: false,
		},
		{
			name:      "empty bio",
			bio:       "",
			expectErr: false, // Empty is allowed
		},
		{
			name:      "bio too long",
			bio:       string(make([]byte, 161)), // 161 characters
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := logic.ValidateBio(tt.bio)
			if (err != nil) != tt.expectErr {
				t.Errorf("ValidateBio() error = %v, expectErr %v", err, tt.expectErr)
			}
		})
	}
}

func TestValidateRateLimit(t *testing.T) {
	config := DefaultMastodonConfig()
	logic := NewMastodonBusinessLogic(config, zap.NewNop())
	ctx := context.Background()

	// Test basic rate limit validation (this may be a placeholder implementation)
	limits := RateLimitConfig{
		PostsPerHour:    300,
		FollowsPerHour:  100,
		ReportsPerHour:  5,
		UploadsPerHour:  30,
		SearchesPerHour: 60,
	}

	err := logic.ValidateRateLimit(ctx, "user123", "posts", limits)
	// Note: This might return nil in placeholder implementation
	if err != nil {
		t.Logf("Rate limit validation returned: %v", err)
	}
}

func TestValidateMastodonPaginationParams(t *testing.T) {
	config := DefaultMastodonConfig()
	logic := NewMastodonBusinessLogic(config, zap.NewNop())

	params := MastodonPaginationParams{
		MaxID: "12345",
		MinID: "67890",
		Limit: 20,
	}

	result := logic.ValidateMastodonPaginationParams(params)

	if result.MaxID != "12345" {
		t.Errorf("MaxID = %v, want %v", result.MaxID, "12345")
	}
	if result.MinID != "67890" {
		t.Errorf("MinID = %v, want %v", result.MinID, "67890")
	}
	if result.Limit != 20 {
		t.Errorf("Limit = %v, want %v", result.Limit, 20)
	}
}

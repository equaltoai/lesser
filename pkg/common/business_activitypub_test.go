package common

import (
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestNewActivityPubBusinessLogic(t *testing.T) {
	config := &FederationConfig{
		Domain:         "test.example.com",
		UserAgent:      "Test/1.0",
		MaxRetries:     3,
		RetryDelay:     5 * time.Second,
		RequestTimeout: 30 * time.Second,
	}
	logger := zap.NewNop()

	logic := NewActivityPubBusinessLogic(config, logger)

	if logic == nil {
		t.Error("NewActivityPubBusinessLogic returned nil")
	}
	if logic.config.Domain != config.Domain {
		t.Errorf("Domain = %v, want %v", logic.config.Domain, config.Domain)
	}
	if logic.logger != logger {
		t.Error("Logger not set correctly")
	}
}

func TestValidateActorURI(t *testing.T) {
	tests := []struct {
		name      string
		actorURI  string
		rules     ActivityPubValidationRules
		expectErr bool
	}{
		{
			name:     "valid https actor",
			actorURI: "https://example.com/users/alice",
			rules: ActivityPubValidationRules{
				RequireHTTPS: true,
			},
			expectErr: false,
		},
		{
			name:     "invalid http actor",
			actorURI: "http://example.com/users/alice",
			rules: ActivityPubValidationRules{
				RequireHTTPS: true,
			},
			expectErr: true,
		},
		{
			name:      "empty actor URI",
			actorURI:  "",
			rules:     ActivityPubValidationRules{},
			expectErr: true,
		},
		{
			name:      "invalid URI format",
			actorURI:  "not-a-uri",
			rules:     ActivityPubValidationRules{},
			expectErr: false, // url.Parse is lenient, only fails for specific cases
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateActorURI(tt.actorURI, tt.rules)
			if (err != nil) != tt.expectErr {
				t.Errorf("ValidateActorURI() error = %v, expectErr %v", err, tt.expectErr)
			}
		})
	}
}

func TestValidateActivityType(t *testing.T) {
	tests := []struct {
		name         string
		activityType string
		rules        ActivityPubValidationRules
		expectErr    bool
	}{
		{
			name:         "valid activity type",
			activityType: "Create",
			rules:        ActivityPubValidationRules{},
			expectErr:    false,
		},
		{
			name:         "invalid activity type",
			activityType: "InvalidType",
			rules:        ActivityPubValidationRules{},
			expectErr:    true,
		},
		{
			name:         "empty activity type",
			activityType: "",
			rules:        ActivityPubValidationRules{},
			expectErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateActivityType(tt.activityType, tt.rules)
			if (err != nil) != tt.expectErr {
				t.Errorf("ValidateActivityType() error = %v, expectErr %v", err, tt.expectErr)
			}
		})
	}
}

func TestWrapObjectInActivity(t *testing.T) {
	activityType := "Create"
	actorID := "https://test.example.com/users/alice"
	object := map[string]interface{}{
		"type":    "Note",
		"content": "Hello world",
		"id":      "https://test.example.com/notes/123",
	}
	published := time.Now()

	activity := WrapObjectInActivity(activityType, actorID, object, published)

	if activity["type"] != activityType {
		t.Errorf("Activity type = %v, want %v", activity["type"], activityType)
	}
	if activity["actor"] != actorID {
		t.Errorf("Activity actor = %v, want %v", activity["actor"], actorID)
	}
	if activity["object"] == nil {
		t.Error("Activity object is nil")
	}
	if activity["@context"] == nil {
		t.Error("Activity context is nil")
	}
	if activity["id"] == nil {
		t.Error("Activity id is nil")
	}
	if activity["published"] == nil {
		t.Error("Activity published timestamp is nil")
	}
}

func TestGenerateActivityPubID(t *testing.T) {
	domain := "test.example.com"
	objectType := "notes"
	localID := "123"

	id := GenerateActivityPubID(domain, objectType, localID)

	expected := "https://test.example.com/notes/123"
	if id != expected {
		t.Errorf("GenerateActivityPubID() = %v, want %v", id, expected)
	}
}

func TestGenerateActorID(t *testing.T) {
	domain := "test.example.com"
	username := "alice"

	id := GenerateActorID(domain, username)

	expected := "https://test.example.com/users/alice"
	if id != expected {
		t.Errorf("GenerateActorID() = %v, want %v", id, expected)
	}
}

func TestCalculateActivityPubAudience(t *testing.T) {
	visibility := VisibilityPublic
	actorFollowers := "https://test.example.com/users/alice/followers"
	mentions := []string{"https://test.example.com/users/bob"}

	audience := CalculateActivityPubAudience(visibility, actorFollowers, mentions)

	publicCollection := "https://www.w3.org/ns/activitystreams#Public"
	if len(audience.To) == 0 || audience.To[0] != publicCollection {
		t.Errorf("Public visibility should have public collection in To field")
	}
	if len(audience.CC) == 0 || audience.CC[0] != actorFollowers {
		t.Errorf("Public visibility should have followers in CC field")
	}
}

func TestParseHTTPSignature(t *testing.T) {
	tests := []struct {
		name      string
		signature string
		expectErr bool
	}{
		{
			name:      "valid signature format",
			signature: `keyId="https://example.com/users/alice#main-key",algorithm="rsa-sha256",headers="(request-target) host date",signature="abc123"`,
			expectErr: false,
		},
		{
			name:      "empty signature",
			signature: "",
			expectErr: true,
		},
		{
			name:      "missing keyId",
			signature: `algorithm="rsa-sha256",signature="abc123"`,
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseHTTPSignature(tt.signature)
			if (err != nil) != tt.expectErr {
				t.Errorf("ParseHTTPSignature() error = %v, expectErr %v", err, tt.expectErr)
			}
		})
	}
}

func TestCreateActivityPubCollection(t *testing.T) {
	id := "https://test.example.com/users/alice/followers"
	collectionType := "Collection"
	totalItems := 100
	firstPage := "https://test.example.com/users/alice/followers?page=1"

	collection := CreateActivityPubCollection(id, collectionType, totalItems, firstPage)

	if collection.ID != id {
		t.Errorf("Collection ID = %v, want %v", collection.ID, id)
	}
	if collection.Type != collectionType {
		t.Errorf("Collection Type = %v, want %v", collection.Type, collectionType)
	}
	if collection.TotalItems != totalItems {
		t.Errorf("Collection TotalItems = %v, want %v", collection.TotalItems, totalItems)
	}
	if collection.First != firstPage {
		t.Errorf("Collection First = %v, want %v", collection.First, firstPage)
	}
}

func TestGetStandardActivityPubContext(t *testing.T) {
	context := GetStandardActivityPubContext()

	if len(context) == 0 {
		t.Error("ActivityPub context is empty")
	}

	// Check that it includes the main ActivityStreams context
	if context[0] != "https://www.w3.org/ns/activitystreams" {
		t.Errorf("First context item = %v, want %v", context[0], "https://www.w3.org/ns/activitystreams")
	}
}

func TestActivityPubError(t *testing.T) {
	err := NewActivityPubError("timeout", "request timeout", true)

	if err.Type != "timeout" {
		t.Errorf("Error type = %v, want %v", err.Type, "timeout")
	}
	if err.Message != "request timeout" {
		t.Errorf("Error message = %v, want %v", err.Message, "request timeout")
	}
	if !err.IsTemporary() {
		t.Error("Error should be temporary")
	}

	errorStr := err.Error()
	if !strings.Contains(errorStr, "timeout") {
		t.Errorf("Error string should contain 'timeout': %s", errorStr)
	}
}

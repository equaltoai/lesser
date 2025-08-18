package transformers

import (
	"fmt"
	"testing"
	"time"

	"github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage"
	storageModels "github.com/equaltoai/lesser/pkg/storage/models"
)

func TestMastodonTransformer_StorageStatusToMastodon(t *testing.T) {
	transformer := NewMastodonTransformer("https://example.com")

	// Create a test storage status
	now := time.Now()
	status := &storageModels.Status{
		StatusID:       "test-status-123",
		Content:        "Hello, world!",
		AuthorUsername: "testuser",
		AuthorID:       "https://example.com/users/testuser",
		Visibility:     "public",
		Language:       "en",
		CreatedAt:      now,
		Hashtags:       []string{"test", "hello"},
		Mentions:       []string{"otheruser"},
	}

	// Transform to Mastodon format
	mastodonStatus, err := transformer.StorageStatusToMastodon(status, "viewer")
	if err != nil {
		t.Fatalf("Failed to transform status: %v", err)
	}

	// Verify basic fields
	if mastodonStatus.ID != "test-status-123" {
		t.Errorf("Expected ID 'test-status-123', got '%s'", mastodonStatus.ID)
	}
	if mastodonStatus.Content != "Hello, world!" {
		t.Errorf("Expected content 'Hello, world!', got '%s'", mastodonStatus.Content)
	}
	if mastodonStatus.Visibility != "public" {
		t.Errorf("Expected visibility 'public', got '%s'", mastodonStatus.Visibility)
	}
	if mastodonStatus.Language != "en" {
		t.Errorf("Expected language 'en', got '%s'", mastodonStatus.Language)
	}

	// Verify account fields
	if mastodonStatus.Account.Username != "testuser" {
		t.Errorf("Expected account username 'testuser', got '%s'", mastodonStatus.Account.Username)
	}

	// Verify hashtags transformation
	if len(mastodonStatus.Tags) != 2 {
		t.Errorf("Expected 2 tags, got %d", len(mastodonStatus.Tags))
	}

	// Verify mentions transformation
	if len(mastodonStatus.Mentions) != 1 {
		t.Errorf("Expected 1 mention, got %d", len(mastodonStatus.Mentions))
	}

	// Verify URL generation
	expectedURI := "https://example.com/users/testuser/statuses/test-status-123"
	if mastodonStatus.URI != expectedURI {
		t.Errorf("Expected URI '%s', got '%s'", expectedURI, mastodonStatus.URI)
	}

	expectedURL := "https://example.com/@testuser/test-status-123"
	if mastodonStatus.URL != expectedURL {
		t.Errorf("Expected URL '%s', got '%s'", expectedURL, mastodonStatus.URL)
	}
}

func TestMastodonTransformer_StorageAccountToMastodon(t *testing.T) {
	transformer := NewMastodonTransformer("https://example.com")

	// Create a test storage account
	now := time.Now()
	account := &storage.Account{
		User: &storage.User{
			Username:    "testuser",
			DisplayName: "Test User",
			CreatedAt:   now,
		},
		Actor: &activitypub.Actor{
			BaseObject: activitypub.BaseObject{
				ID: "https://example.com/users/testuser",
			},
			PreferredUsername: "testuser",
			Name:              "Test User",
			Summary:           "A test user bio",
		},
	}

	// Transform to Mastodon format
	mastodonAccount, err := transformer.StorageAccountToMastodon(account)
	if err != nil {
		t.Fatalf("Failed to transform account: %v", err)
	}

	// Verify basic fields
	if mastodonAccount.Username != "testuser" {
		t.Errorf("Expected username 'testuser', got '%s'", mastodonAccount.Username)
	}
	if mastodonAccount.DisplayName != "Test User" {
		t.Errorf("Expected display name 'Test User', got '%s'", mastodonAccount.DisplayName)
	}

	// Verify URL generation
	expectedURL := "https://example.com/@testuser"
	if mastodonAccount.URL != expectedURL {
		t.Errorf("Expected URL '%s', got '%s'", expectedURL, mastodonAccount.URL)
	}

	// Verify default values are set
	if len(mastodonAccount.Fields) != 0 {
		t.Errorf("Expected empty fields, got %d", len(mastodonAccount.Fields))
	}
	if len(mastodonAccount.Emojis) != 0 {
		t.Errorf("Expected empty emojis, got %d", len(mastodonAccount.Emojis))
	}
}

func TestMastodonTransformer_MastodonStatusParamsToStorage(t *testing.T) {
	transformer := NewMastodonTransformer("https://example.com")

	// Create Mastodon API request
	params := &models.CreateStatusRequest{
		Status:      "Hello, world!",
		Visibility:  "public",
		Sensitive:   true,
		Language:    "en",
		InReplyToID: "reply-to-123",
		MediaIDs:    []string{"media-1", "media-2"},
	}

	// Transform to storage format
	storageReq, err := transformer.MastodonStatusParamsToStorage(params, "testuser")
	if err != nil {
		t.Fatalf("Failed to transform params: %v", err)
	}

	// Verify transformation
	if storageReq.AuthorUsername != "testuser" {
		t.Errorf("Expected author username 'testuser', got '%s'", storageReq.AuthorUsername)
	}
	if storageReq.Content != "Hello, world!" {
		t.Errorf("Expected content 'Hello, world!', got '%s'", storageReq.Content)
	}
	if storageReq.Visibility != "public" {
		t.Errorf("Expected visibility 'public', got '%s'", storageReq.Visibility)
	}
	if !storageReq.Sensitive {
		t.Error("Expected sensitive to be true")
	}
	if storageReq.Language != "en" {
		t.Errorf("Expected language 'en', got '%s'", storageReq.Language)
	}
	if storageReq.InReplyToID != "reply-to-123" {
		t.Errorf("Expected in_reply_to_id 'reply-to-123', got '%s'", storageReq.InReplyToID)
	}
	if len(storageReq.MediaIDs) != 2 {
		t.Errorf("Expected 2 media IDs, got %d", len(storageReq.MediaIDs))
	}
}

func TestMastodonTransformer_DefaultVisibility(t *testing.T) {
	transformer := NewMastodonTransformer("https://example.com")

	// Create Mastodon API request without visibility
	params := &models.CreateStatusRequest{
		Status: "Hello, world!",
		// Visibility not set
	}

	// Transform to storage format
	storageReq, err := transformer.MastodonStatusParamsToStorage(params, "testuser")
	if err != nil {
		t.Fatalf("Failed to transform params: %v", err)
	}

	// Verify default visibility is set
	if storageReq.Visibility != "public" {
		t.Errorf("Expected default visibility 'public', got '%s'", storageReq.Visibility)
	}
}

func TestMastodonTransformer_FormatMastodonError(t *testing.T) {
	transformer := NewMastodonTransformer("https://example.com")

	// Test with actual error
	testErr := fmt.Errorf("test error message")
	errorResponse := transformer.FormatMastodonError(testErr)

	if errorResponse["error"] != "test error message" {
		t.Errorf("Expected error message 'test error message', got '%v'", errorResponse["error"])
	}

	if errorResponse["error_type"] != "*errors.errorString" {
		t.Errorf("Expected error type '*errors.errorString', got '%v'", errorResponse["error_type"])
	}

	// Test with nil error
	nilErrorResponse := transformer.FormatMastodonError(nil)
	if nilErrorResponse["error"] != "unknown error" {
		t.Errorf("Expected 'unknown error' for nil error, got '%v'", nilErrorResponse["error"])
	}
}

func TestMastodonTransformer_BuildLinkHeader(t *testing.T) {
	transformer := NewMastodonTransformer("https://example.com")

	// Test with pagination info
	pagination := &PaginationInfo{
		NextCursor: "next-123",
		MinID:      "min-456",
		Limit:      20,
	}

	linkHeader := transformer.BuildLinkHeader("https://example.com/api/v1/statuses", pagination)
	
	expectedNext := `<https://example.com/api/v1/statuses?max_id=next-123&limit=20>; rel="next"`
	expectedPrev := `<https://example.com/api/v1/statuses?min_id=min-456&limit=20>; rel="prev"`
	expected := expectedNext + ", " + expectedPrev

	if linkHeader != expected {
		t.Errorf("Expected link header '%s', got '%s'", expected, linkHeader)
	}

	// Test with empty pagination
	emptyHeader := transformer.BuildLinkHeader("https://example.com/api/v1/statuses", nil)
	if emptyHeader != "" {
		t.Errorf("Expected empty header for nil pagination, got '%s'", emptyHeader)
	}
}

func TestMastodonTransformer_TransformHashtags(t *testing.T) {
	transformer := NewMastodonTransformer("https://example.com")

	hashtags := []string{"test", "hello", "world"}
	transformed := transformer.transformHashtags(hashtags)

	if len(transformed) != 3 {
		t.Errorf("Expected 3 transformed hashtags, got %d", len(transformed))
	}

	// Check first hashtag
	if tagMap, ok := transformed[0].(map[string]interface{}); ok {
		if tagMap["name"] != "test" {
			t.Errorf("Expected hashtag name 'test', got '%v'", tagMap["name"])
		}
		expectedURL := "https://example.com/tags/test"
		if tagMap["url"] != expectedURL {
			t.Errorf("Expected hashtag URL '%s', got '%v'", expectedURL, tagMap["url"])
		}
	} else {
		t.Error("Expected hashtag to be a map")
	}
}

func TestMastodonTransformer_TransformMentions(t *testing.T) {
	transformer := NewMastodonTransformer("https://example.com")

	mentions := []string{"user1", "user2"}
	transformed := transformer.transformMentions(mentions)

	if len(transformed) != 2 {
		t.Errorf("Expected 2 transformed mentions, got %d", len(transformed))
	}

	// Check first mention
	if mentionMap, ok := transformed[0].(map[string]interface{}); ok {
		if mentionMap["username"] != "user1" {
			t.Errorf("Expected mention username 'user1', got '%v'", mentionMap["username"])
		}
		expectedURL := "https://example.com/@user1"
		if mentionMap["url"] != expectedURL {
			t.Errorf("Expected mention URL '%s', got '%v'", expectedURL, mentionMap["url"])
		}
	} else {
		t.Error("Expected mention to be a map")
	}
}

func TestBatchProcessor_ProcessStatusBatch(t *testing.T) {
	processor := NewBatchProcessor("https://example.com")

	// Create test statuses
	now := time.Now()
	statuses := []*storageModels.Status{
		{
			StatusID:       "status-1",
			Content:        "First status",
			AuthorUsername: "user1",
			AuthorID:       "https://example.com/users/user1",
			CreatedAt:      now,
		},
		{
			StatusID:       "status-2", 
			Content:        "Second status",
			AuthorUsername: "user2",
			AuthorID:       "https://example.com/users/user2",
			CreatedAt:      now,
		},
	}

	// Process batch
	results, err := processor.ProcessStatusBatch(statuses, "viewer")
	if err != nil {
		t.Fatalf("Failed to process status batch: %v", err)
	}

	// Verify results
	if len(results) != 2 {
		t.Errorf("Expected 2 processed statuses, got %d", len(results))
	}

	if results[0].ID != "status-1" {
		t.Errorf("Expected first status ID 'status-1', got '%s'", results[0].ID)
	}
	if results[1].ID != "status-2" {
		t.Errorf("Expected second status ID 'status-2', got '%s'", results[1].ID)
	}
}

func TestCachedTransformer(t *testing.T) {
	cachedTransformer := NewCachedTransformer("https://example.com")

	// Verify it has the base transformer
	if cachedTransformer.MastodonTransformer == nil {
		t.Error("Expected cached transformer to have base transformer")
	}

	// Verify cache is initialized
	if cachedTransformer.cache == nil {
		t.Error("Expected cache to be initialized")
	}

	// Test cache clearing
	cachedTransformer.cache["test"] = "value"
	cachedTransformer.ClearCache()
	if len(cachedTransformer.cache) != 0 {
		t.Error("Expected cache to be cleared")
	}
}
package transformers

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage"
	storageModels "github.com/equaltoai/lesser/pkg/storage/models"
)

func BenchmarkStorageStatusToMastodon(b *testing.B) {
	transformer := NewMastodonTransformer("https://example.com")

	now := time.Now()
	status := &storageModels.Status{
		StatusID:       "test-status-123",
		Content:        "Hello, world! This is a test status with some content.",
		AuthorUsername: "testuser",
		AuthorID:       "https://example.com/users/testuser",
		Visibility:     "public",
		Language:       "en",
		CreatedAt:      now,
		Hashtags:       []string{"test", "hello", "benchmark", "performance"},
		Mentions:       []string{"user1", "user2", "user3"},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := transformer.StorageStatusToMastodon(status, "viewer")
		if err != nil {
			b.Fatalf("Transformation failed: %v", err)
		}
	}
}

func BenchmarkStorageAccountToMastodon(b *testing.B) {
	transformer := NewMastodonTransformer("https://example.com")

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
			Summary:           "A test user bio with some content",
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := transformer.StorageAccountToMastodon(account)
		if err != nil {
			b.Fatalf("Transformation failed: %v", err)
		}
	}
}

func BenchmarkMastodonStatusParamsToStorage(b *testing.B) {
	transformer := NewMastodonTransformer("https://example.com")

	params := &models.CreateStatusRequest{
		Status:      "Hello, world! This is a test status creation request.",
		Visibility:  "public",
		Sensitive:   false,
		Language:    "en",
		InReplyToID: "reply-to-123",
		MediaIDs:    []string{"media-1", "media-2", "media-3"},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := transformer.MastodonStatusParamsToStorage(params, "testuser")
		if err != nil {
			b.Fatalf("Transformation failed: %v", err)
		}
	}
}

func BenchmarkBatchProcessStatusBatch(b *testing.B) {
	processor := NewBatchProcessor("https://example.com")

	// Create a batch of test statuses
	now := time.Now()
	statuses := make([]*storageModels.Status, 100)
	for i := 0; i < 100; i++ {
		statuses[i] = &storageModels.Status{
			StatusID:       fmt.Sprintf("status-%d", i),
			Content:        fmt.Sprintf("This is test status number %d", i),
			AuthorUsername: fmt.Sprintf("user%d", i%10), // 10 different users
			AuthorID:       fmt.Sprintf("https://example.com/users/user%d", i%10),
			Visibility:     "public",
			CreatedAt:      now,
			Hashtags:       []string{"test", "benchmark"},
			Mentions:       []string{"mention1", "mention2"},
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := processor.ProcessStatusBatch(statuses, "viewer")
		if err != nil {
			b.Fatalf("Batch processing failed: %v", err)
		}
	}
}

func BenchmarkTransformHashtags(b *testing.B) {
	transformer := NewMastodonTransformer("https://example.com")

	hashtags := []string{"test", "benchmark", "performance", "golang", "mastodon", "api", "transformation", "optimization"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = transformer.transformHashtags(hashtags)
	}
}

func BenchmarkTransformMentions(b *testing.B) {
	transformer := NewMastodonTransformer("https://example.com")

	mentions := []string{"user1", "user2", "user3", "user4", "user5", "user6", "user7", "user8"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = transformer.transformMentions(mentions)
	}
}

func BenchmarkFormatMastodonError(b *testing.B) {
	transformer := NewMastodonTransformer("https://example.com")

	testErr := fmt.Errorf("test error message for benchmarking")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = transformer.FormatMastodonError(testErr)
	}
}

func BenchmarkBuildLinkHeader(b *testing.B) {
	transformer := NewMastodonTransformer("https://example.com")

	pagination := &PaginationInfo{
		NextCursor: "next-123456",
		MinID:      "min-789012",
		Limit:      20,
	}

	baseURL := "https://example.com/api/v1/statuses"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = transformer.BuildLinkHeader(baseURL, pagination)
	}
}

// Benchmark the transformation framework bridge
func BenchmarkTransformationFrameworkBridge(b *testing.B) {
	transformer := NewMastodonTransformer("https://example.com")
	bridge := transformer.WithTransformationFramework()

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

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := bridge.Transform(context.Background(), account)
		if err != nil {
			b.Fatalf("Bridge transformation failed: %v", err)
		}
	}
}

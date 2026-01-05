package services

import (
	"os"
	"path/filepath"
	"testing"

	pkgconfig "github.com/equaltoai/lesser/pkg/config"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

func TestRegistry_ConfigHelpersAndBuckets(t *testing.T) {
	storage := newMockStorage()

	cfg := &ServiceConfig{
		BaseURL:   "https://example.com",
		JWTSecret: "test-secret",
		Config: &pkgconfig.Config{
			S3BucketName:           "main-bucket",
			MediaSourceBucketName:  "source-bucket",
			MediaStreamingBucketName: "streaming-bucket",
			CloudFrontDomain:       "cdn.example.com",
			ManifestTTLHours:       12,
		},
	}

	registry, err := NewRegistry(
		WithStorage(storage),
		WithLogger(zap.NewNop()),
		WithConfig(cfg),
	)
	if err != nil {
		t.Fatalf("failed to create registry: %v", err)
	}

	assert.Equal(t, "source-bucket", registry.getMediaSourceBucket())
	assert.Equal(t, "streaming-bucket", registry.getMediaStreamingBucket())
	assert.Equal(t, "cdn.example.com", registry.getCloudFrontDomain())
	assert.Equal(t, 12, registry.getConfigInt(cfg.Config, "ManifestTTLHours"))

	// Fallbacks
	registry.config.Config.MediaSourceBucketName = ""
	assert.Equal(t, "main-bucket", registry.getMediaSourceBucket())
	registry.config.Config.S3BucketName = ""
	assert.Equal(t, "lesser-media-bucket", registry.getMediaSourceBucket())
}

func TestRegistry_readCloudFrontPrivateKey_FileAndCachedSecret(t *testing.T) {
	storage := newMockStorage()
	registry, err := NewRegistry(WithStorage(storage))
	if err != nil {
		t.Fatalf("failed to create registry: %v", err)
	}

	t.Run("reads from file path", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "cloudfront_key.pem")
		want := "test-private-key"
		if err := os.WriteFile(path, []byte(want), 0o600); err != nil {
			t.Fatalf("write temp key: %v", err)
		}

		got, err := registry.readCloudFrontPrivateKey(path)
		assert.NoError(t, err)
		assert.Equal(t, want, got)
	})

	t.Run("reads cached secret without AWS calls", func(t *testing.T) {
		secretID := "arn:aws:secretsmanager:us-east-1:123456789012:secret:lesser/test"
		registry.secretsCacheMu.Lock()
		registry.secretsCache[secretID] = "cached"
		registry.secretsCacheMu.Unlock()

		got, err := registry.readCloudFrontPrivateKey(secretID)
		assert.NoError(t, err)
		assert.Equal(t, "cached", got)
	})
}

func TestRegistry_DomainServices_WithMissingRepos(t *testing.T) {
	storage := newMockStorage() // all repos nil
	registry, err := NewRegistry(WithStorage(storage))
	if err != nil {
		t.Fatalf("failed to create registry: %v", err)
	}

	// Services that should initialize even if backing repos are nil.
	assert.NotNil(t, registry.Accounts())
	assert.NotNil(t, registry.Relationships())
	assert.NotNil(t, registry.QueryTracker())

	// Services that should refuse to initialize without required repos.
	assert.Nil(t, registry.Notes())
	assert.Nil(t, registry.StreamingAnalytics())
	assert.Nil(t, registry.ModerationML())

	initialized := registry.GetInitializedServices()
	assert.Contains(t, initialized, "Accounts")
	assert.Contains(t, initialized, "Relationships")
	assert.Contains(t, initialized, "QueryTracker")
	assert.NotContains(t, initialized, "Notes")
	assert.NotContains(t, initialized, "StreamingAnalytics")
	assert.NotContains(t, initialized, "ModerationML")
}


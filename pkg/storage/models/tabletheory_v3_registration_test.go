package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	tablemodel "github.com/theory-cloud/tabletheory/v3/pkg/model"
)

func TestArticleRegistersWithSingleCreatedAtMapping(t *testing.T) {
	// TableTheory v2 persisted embedded Object.CreatedAt under the createdAt attribute.
	registry := tablemodel.NewRegistry()
	require.NoError(t, registry.Register(&Article{}))

	metadata, err := registry.GetMetadata(&Article{})
	require.NoError(t, err)

	createdAt, ok := metadata.FieldsByDBName["createdAt"]
	require.True(t, ok)
	require.Equal(t, "CreatedAt", createdAt.Name)
	require.Equal(t, []int{0, createdAt.Index}, createdAt.IndexPath)

	want := time.Now().UTC()
	article := Article{Object: Object{CreatedAt: want}}
	require.Equal(t, want, article.Object.CreatedAt)
	require.Equal(t, want, article.CreatedAt)
}

func TestDNSCacheRegistersWithV2TTLPrecedence(t *testing.T) {
	// TableTheory v2 persisted ExpiresAt under the ttl attribute; ExpiresAt won over the Go TTL field.
	registry := tablemodel.NewRegistry()
	require.NoError(t, registry.Register(&DNSCache{}))

	metadata, err := registry.GetMetadata(&DNSCache{})
	require.NoError(t, err)

	ttl, ok := metadata.FieldsByDBName["ttl"]
	require.True(t, ok)
	require.Equal(t, "ExpiresAt", ttl.Name)
	require.True(t, ttl.IsTTL)
	_, persistedDuration := metadata.Fields["TTL"]
	require.False(t, persistedDuration)
}

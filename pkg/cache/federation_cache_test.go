package cache

import (
	"crypto"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestFederationCache_PublicKeyLifecycle(t *testing.T) {
	fc := NewFederationCache(DefaultCacheConfig(), nil, zap.NewNop())
	t.Cleanup(fc.Close)

	fc.SetPublicKey("kid1", []byte("pubkey"), "https://example.com/actor", "rsa")

	entry, found := fc.GetPublicKey("kid1")
	require.True(t, found)
	require.Equal(t, "kid1", entry.KeyID)
	require.Equal(t, "rsa", entry.Algorithm)
	require.Equal(t, "https://example.com/actor", entry.Owner)
	require.Equal(t, []byte("pubkey"), entry.Key)

	fc.InvalidatePublicKey("kid1")
	_, found = fc.GetPublicKey("kid1")
	require.False(t, found)
}

func TestFederationCache_GetOrFetchPublicKey_Caches(t *testing.T) {
	fc := NewFederationCache(DefaultCacheConfig(), nil, zap.NewNop())
	t.Cleanup(fc.Close)

	calls := 0
	fetchFn := func() (crypto.PublicKey, string, string, error) {
		calls++
		return []byte("pubkey"), "owner", "rsa", nil
	}

	key, err := fc.GetOrFetchPublicKey("kid1", fetchFn)
	require.NoError(t, err)
	require.Equal(t, []byte("pubkey"), key)
	require.Equal(t, 1, calls)

	key, err = fc.GetOrFetchPublicKey("kid1", fetchFn)
	require.NoError(t, err)
	require.Equal(t, []byte("pubkey"), key)
	require.Equal(t, 1, calls)
}

func TestFederationCache_GetOrFetchInstance_NegativeCaching(t *testing.T) {
	fc := NewFederationCache(DefaultCacheConfig(), nil, zap.NewNop())
	t.Cleanup(fc.Close)

	_, err := fc.GetOrFetchInstance("bad.example", func() (map[string]interface{}, bool, error) {
		return nil, false, assertErr("fetch failed")
	})
	require.Error(t, err)

	entry, found := fc.GetInstance("bad.example")
	require.True(t, found)
	require.False(t, entry.Available)
}

type assertErr string

func (e assertErr) Error() string { return string(e) }

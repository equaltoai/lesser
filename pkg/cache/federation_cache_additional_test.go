package cache

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestFederationCache_DefaultsAndManagement(t *testing.T) {
	fc := NewFederationCache(Config{Size: 0}, struct{}{}, nil)
	t.Cleanup(fc.Close)

	require.Equal(t, 10000, fc.maxEntries)
	require.NotNil(t, fc.logger)

	fc.SetPublicKey("kid", []byte("k"), "owner", "rsa")
	fc.SetInstance("example.com", map[string]interface{}{"v": 1}, true)
	fc.SetActor("https://example.com/users/alice", "alice", "example.com", "kid", map[string]interface{}{"k": "v"})

	stats := fc.GetStats()
	require.Equal(t, 3, stats.Size)

	fc.Clear()
	require.Equal(t, 0, fc.GetStats().Size)
}

func TestFederationCache_ExpiredEntriesAreInvalidatedOnGet(t *testing.T) {
	fc := NewFederationCache(DefaultCacheConfig(), nil, zap.NewNop())
	t.Cleanup(fc.Close)

	now := time.Now()
	fc.publicKeys["kid"] = &PublicKeyEntry{KeyID: "kid", ExpiresAt: now.Add(-time.Second)}
	fc.instances["example.com"] = &InstanceEntry{Domain: "example.com", ExpiresAt: now.Add(-time.Second)}
	fc.actors["actor"] = &ActorEntry{ActorID: "actor", ExpiresAt: now.Add(-time.Second)}

	_, found := fc.GetPublicKey("kid")
	require.False(t, found)
	_, found = fc.GetInstance("example.com")
	require.False(t, found)
	_, found = fc.GetActor("actor")
	require.False(t, found)
}

func TestFederationCache_InstanceAndActorInvalidation(t *testing.T) {
	fc := NewFederationCache(DefaultCacheConfig(), nil, zap.NewNop())
	t.Cleanup(fc.Close)

	fc.SetInstance("example.com", map[string]interface{}{"k": "v"}, true)
	entry, found := fc.GetInstance("example.com")
	require.True(t, found)
	require.True(t, entry.Available)

	fc.InvalidateInstance("example.com")
	_, found = fc.GetInstance("example.com")
	require.False(t, found)

	fc.SetActor("actor", "alice", "example.com", "kid", map[string]interface{}{"k": "v"})
	actor, found := fc.GetActor("actor")
	require.True(t, found)
	require.Equal(t, "alice", actor.Username)

	fc.InvalidateActor("actor")
	_, found = fc.GetActor("actor")
	require.False(t, found)
}

func TestFederationCache_Cleanup_RemovesExpiredEntries(t *testing.T) {
	fc := NewFederationCache(DefaultCacheConfig(), nil, zap.NewNop())
	t.Cleanup(fc.Close)

	now := time.Now()
	fc.publicKeys["expired"] = &PublicKeyEntry{KeyID: "expired", ExpiresAt: now.Add(-time.Second)}
	fc.publicKeys["ok"] = &PublicKeyEntry{KeyID: "ok", ExpiresAt: now.Add(time.Hour)}

	fc.instances["expired.example"] = &InstanceEntry{Domain: "expired.example", ExpiresAt: now.Add(-time.Second)}
	fc.instances["ok.example"] = &InstanceEntry{Domain: "ok.example", ExpiresAt: now.Add(time.Hour)}

	fc.actors["expired-actor"] = &ActorEntry{ActorID: "expired-actor", ExpiresAt: now.Add(-time.Second)}
	fc.actors["ok-actor"] = &ActorEntry{ActorID: "ok-actor", ExpiresAt: now.Add(time.Hour)}

	fc.cleanup()

	require.NotContains(t, fc.publicKeys, "expired")
	require.Contains(t, fc.publicKeys, "ok")
	require.NotContains(t, fc.instances, "expired.example")
	require.Contains(t, fc.instances, "ok.example")
	require.NotContains(t, fc.actors, "expired-actor")
	require.Contains(t, fc.actors, "ok-actor")
}

func TestFederationCache_PersistenceMethods_HandleMarshalErrors(t *testing.T) {
	fc := NewFederationCache(DefaultCacheConfig(), struct{}{}, zap.NewNop())
	t.Cleanup(fc.Close)

	// Marshal error: channels are not JSON-marshalable.
	fc.persistPublicKey("kid", &PublicKeyEntry{Key: make(chan int), KeyID: "kid", ExpiresAt: time.Now().Add(time.Hour)})
	fc.persistInstance("example.com", &InstanceEntry{Domain: "example.com", Metadata: map[string]interface{}{"bad": make(chan int)}, ExpiresAt: time.Now().Add(time.Hour)})
	fc.persistActor("actor", &ActorEntry{ActorID: "actor", Profile: map[string]interface{}{"bad": make(chan int)}, ExpiresAt: time.Now().Add(time.Hour)})

	// Success cases.
	fc.persistPublicKey("kid2", &PublicKeyEntry{Key: []byte("k"), KeyID: "kid2", ExpiresAt: time.Now().Add(time.Hour)})
	fc.persistInstance("ok.example", &InstanceEntry{Domain: "ok.example", Metadata: map[string]interface{}{"k": "v"}, ExpiresAt: time.Now().Add(time.Hour)})
	fc.persistActor("ok-actor", &ActorEntry{ActorID: "ok-actor", Profile: map[string]interface{}{"k": "v"}, ExpiresAt: time.Now().Add(time.Hour)})

	fc.removePersistentPublicKey("kid")
	fc.removePersistentInstance("example.com")
	fc.removePersistentActor("actor")
}

func TestFederationCache_GetOrFetchInstance_CachesOnSuccessAndHitSkipsFetch(t *testing.T) {
	fc := NewFederationCache(DefaultCacheConfig(), nil, zap.NewNop())
	t.Cleanup(fc.Close)

	calls := 0
	fetch := func() (map[string]interface{}, bool, error) {
		calls++
		return map[string]interface{}{"k": "v"}, true, nil
	}

	entry, err := fc.GetOrFetchInstance("example.com", fetch)
	require.NoError(t, err)
	require.NotNil(t, entry)
	require.True(t, entry.Available)
	require.Equal(t, 1, calls)

	entry, err = fc.GetOrFetchInstance("example.com", fetch)
	require.NoError(t, err)
	require.NotNil(t, entry)
	require.Equal(t, 1, calls)
}


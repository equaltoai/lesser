package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNormalizeRemoteActorHandle_SamePackageCoverage(t *testing.T) {
	assert.Equal(t, "alice@example.com", NormalizeRemoteActorHandle("https://Example.com/users/Alice"))
	assert.Equal(t, "bob@example.com", NormalizeRemoteActorHandle("https://example.com/actors/Bob"))
	assert.Equal(t, "carol@example.com", NormalizeRemoteActorHandle("https://example.com/@Carol"))
	assert.Equal(t, "dan@example.com", NormalizeRemoteActorHandle("@Dan@Example.com:8443/path"))
	assert.Equal(t, "eve", NormalizeRemoteActorHandle(" EVE "))
	assert.Equal(t, "", NormalizeRemoteActorHandle("example.com/users/alice"))
	assert.Equal(t, "", NormalizeRemoteActorHandle("https://example.com/users/"))
}

func TestRemoteActor_UpdateKeysAndHelpers(t *testing.T) {
	expiresAt := time.Date(2025, 2, 3, 4, 5, 6, 0, time.UTC)
	actor := &RemoteActor{
		Handle:    "https://Example.com/users/Alice",
		ExpiresAt: expiresAt,
	}

	actor.UpdateKeys()

	assert.Equal(t, "alice@example.com", actor.Handle)
	assert.Equal(t, "REMOTE_ACTOR#alice@example.com", actor.PK)
	assert.Equal(t, SKProfile, actor.SK)
	assert.Equal(t, "example.com", actor.Domain)
	assert.Equal(t, expiresAt.Unix(), actor.TTL)

	assert.Equal(t, "remote.example", extractDomainFromHandle("bob@remote.example"))
	assert.Equal(t, "", extractDomainFromHandle("bob"))
	assert.Equal(t, "remote.example", normalizeRemoteActorDomain("https://Remote.Example:443/users/bob"))
	assert.Equal(t, MainTableName, (RemoteActor{}).TableName())
}

package handlers

import (
	"context"
	"testing"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/stretchr/testify/require"
)

func TestAccountsRound20_PublicAccountFallbackHelpers(t *testing.T) {
	cfg := round10TestConfig()
	h := &Handler{cfg: cfg}

	t.Run("public account helpers handle nil inputs", func(t *testing.T) {
		require.Empty(t, h.publicAccountFromStorageAccount(nil))
		require.Empty(t, h.publicAccountFromActor(context.Background(), nil))
	})

	t.Run("publicAccountFromStorageAccount falls back to actor conversion when user is missing", func(t *testing.T) {
		actor := &activitypub.Actor{
			BaseObject:        activitypub.BaseObject{ID: "https://remote.example/users/alice", Type: "Person"},
			PreferredUsername: "alice@example.net",
			Name:              "Alice",
			URL:               "https://remote.example/@alice",
		}

		account := h.publicAccountFromStorageAccount(&storage.Account{Actor: actor})
		require.Equal(t, actor.PreferredUsername, account.Username)
		require.Equal(t, actor.Name, account.DisplayName)
		require.Equal(t, actor.URL, account.URL)
		require.NotNil(t, account.Emojis)
		require.NotNil(t, account.Fields)
	})

	t.Run("publicAccountFromActor synthesizes local storage account data", func(t *testing.T) {
		actor := &activitypub.Actor{
			BaseObject:        activitypub.BaseObject{ID: cfg.BaseURL() + "/users/alice", Type: "Person"},
			PreferredUsername: "alice",
			Name:              "Alice",
		}

		account := h.publicAccountFromActor(context.Background(), actor)
		require.Equal(t, "alice", account.Username)
		require.Equal(t, "Alice", account.DisplayName)
		require.Equal(t, cfg.BaseURL()+"/@alice", account.URL)
		require.NotEmpty(t, account.ID)
		require.NotNil(t, account.Emojis)
		require.NotNil(t, account.Fields)
	})

	t.Run("publicAccountFromActor preserves remote actor fallback", func(t *testing.T) {
		actor := &activitypub.Actor{
			BaseObject:        activitypub.BaseObject{ID: "https://remote.example/users/bob", Type: "Person"},
			PreferredUsername: "bob@example.net",
			Name:              "Bob",
			URL:               "https://remote.example/@bob",
		}

		account := h.publicAccountFromActor(context.Background(), actor)
		require.Equal(t, actor.PreferredUsername, account.Username)
		require.Equal(t, actor.Name, account.DisplayName)
		require.Equal(t, actor.URL, account.URL)
		require.NotNil(t, account.Emojis)
		require.NotNil(t, account.Fields)
	})
}

func TestAccountsRound20_AccountResolutionHelpers(t *testing.T) {
	t.Run("storageAccountFromActor handles nil empty and valid actors", func(t *testing.T) {
		require.Nil(t, storageAccountFromActor(nil))
		require.Nil(t, storageAccountFromActor(&activitypub.Actor{PreferredUsername: "   "}))

		actor := &activitypub.Actor{
			BaseObject:        activitypub.BaseObject{ID: "https://example.com/users/alice", Type: "Person"},
			PreferredUsername: "alice",
			Name:              "Alice",
		}

		account := storageAccountFromActor(actor)
		require.NotNil(t, account)
		require.Equal(t, "alice", account.User.Username)
		require.Equal(t, "Alice", account.User.DisplayName)
		require.Same(t, actor, account.Actor)
	})

	t.Run("shouldFallbackAccountResolution recognizes public id shapes", func(t *testing.T) {
		require.False(t, shouldFallbackAccountResolution(""))
		require.False(t, shouldFallbackAccountResolution("alice"))
		require.True(t, shouldFallbackAccountResolution("1234567890"))
		require.True(t, shouldFallbackAccountResolution("https://example.com/users/alice"))
		require.True(t, shouldFallbackAccountResolution("alice@example.com"))
	})

	t.Run("actorAppearsLocal honors base url and username style", func(t *testing.T) {
		cfg := round10TestConfig()
		h := &Handler{cfg: cfg}

		require.False(t, h.actorAppearsLocal(nil))
		require.True(t, h.actorAppearsLocal(&activitypub.Actor{
			BaseObject:        activitypub.BaseObject{ID: cfg.BaseURL() + "/users/alice", Type: "Person"},
			PreferredUsername: "alice@example.com",
		}))
		require.True(t, h.actorAppearsLocal(&activitypub.Actor{
			BaseObject:        activitypub.BaseObject{ID: "https://remote.example/users/alice", Type: "Person"},
			PreferredUsername: "alice",
		}))
		require.False(t, h.actorAppearsLocal(&activitypub.Actor{
			BaseObject:        activitypub.BaseObject{ID: "https://remote.example/users/alice", Type: "Person"},
			PreferredUsername: "alice@example.com",
		}))
	})
}

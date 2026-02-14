package handlers

import (
	"testing"
	"time"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/stretchr/testify/require"
)

func TestAccounts_MastodonTransformHelpers(t *testing.T) {
	t.Parallel()

	t.Run("handlerBaseURL trims trailing slash and handles nil", func(t *testing.T) {
		require.Equal(t, "", handlerBaseURL(nil))
		require.Equal(t, "", handlerBaseURL(&Handler{}))

		cfg := round10TestConfig()
		cfg.Domain = "example.com/"
		h := &Handler{cfg: cfg}
		require.Equal(t, "https://example.com", handlerBaseURL(h))
	})

	t.Run("applyMastodonIdentity sets username/acct and id fallback", func(t *testing.T) {
		out := apimodels.Account{}
		username := "alice"

		applyMastodonIdentity(nil, nil, username)

		applyMastodonIdentity(&out, &storage.User{ID: "", Username: username}, username)
		require.Equal(t, username, out.Username)
		require.Equal(t, username, out.Acct)
		require.Equal(t, common.GenerateNumericID(username), out.ID)

		applyMastodonIdentity(&out, &storage.User{ID: "user-123", Username: username}, username)
		require.Equal(t, "user-123", out.ID)
	})

	t.Run("ensureMastodonAccountCollections normalizes nil slices", func(t *testing.T) {
		out := apimodels.Account{}
		require.Nil(t, out.Emojis)
		require.Nil(t, out.Fields)

		ensureMastodonAccountCollections(&out)
		require.NotNil(t, out.Emojis)
		require.NotNil(t, out.Fields)
	})

	t.Run("applyMastodonProfileFields builds field maps and ignores nil field", func(t *testing.T) {
		out := apimodels.Account{}

		applyMastodonProfileFields(&out, []map[string]string{
			nil,
			{"name": "foo", "value": "bar"},
			{"name": "verified", "value": "baz", "verified_at": "2026-01-01T00:00:00Z"},
		})

		require.Len(t, out.Fields, 2)
	})

	t.Run("applyMastodonProfile fills defaults and sets bot for agents", func(t *testing.T) {
		out := apimodels.Account{}
		user := &storage.User{
			Username:     "bob",
			DisplayName:  "",
			CreatedAt:    time.Time{},
			URL:          "",
			Avatar:       "",
			Header:       "",
			Locked:       true,
			Discoverable: true,
			IsAgent:      true,
		}

		applyMastodonProfile(nil, nil, "https://example.com", "bob")

		applyMastodonProfile(&out, user, "https://example.com", "bob")
		require.Equal(t, "bob", out.DisplayName)
		require.Equal(t, true, out.Locked)
		require.Equal(t, true, out.Discoverable)
		require.Equal(t, true, out.Bot)
		require.NotEmpty(t, out.CreatedAt)
		require.Equal(t, "https://example.com/@bob", out.URL)
		require.Equal(t, "https://example.com/avatars/original/missing.png", out.Avatar)
		require.Equal(t, out.Avatar, out.AvatarStatic)
		require.Equal(t, "https://example.com/headers/original/missing.png", out.Header)
		require.Equal(t, out.Header, out.HeaderStatic)
	})

	t.Run("applyMastodonProfile preserves explicit user fields", func(t *testing.T) {
		out := apimodels.Account{}
		createdAt := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
		user := &storage.User{
			Username:     "bob",
			DisplayName:  "Bobby",
			Note:         "hello",
			CreatedAt:    createdAt,
			URL:          "https://profiles.example/bob",
			Avatar:       "https://cdn.example/avatar.png",
			Header:       "https://cdn.example/header.png",
			Locked:       false,
			Discoverable: false,
			IsAgent:      false,
		}

		applyMastodonProfile(&out, user, "https://example.com", "bob")
		require.Equal(t, "Bobby", out.DisplayName)
		require.Equal(t, "hello", out.Note)
		require.Equal(t, createdAt.UTC().Format(time.RFC3339), out.CreatedAt)
		require.Equal(t, "https://profiles.example/bob", out.URL)
		require.Equal(t, "https://cdn.example/avatar.png", out.Avatar)
		require.Equal(t, out.Avatar, out.AvatarStatic)
		require.Equal(t, "https://cdn.example/header.png", out.Header)
		require.Equal(t, out.Header, out.HeaderStatic)
		require.False(t, out.Bot)
	})
}

package repositories

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"

	"github.com/equaltoai/lesser/pkg/activitypub"
)

func TestMergeActorDataForUpdate_PreservesIdentifiersAndAppliesUpdates(t *testing.T) {
	repo := NewAccountRepository(nil, "test-table", "dev.lesser.host", zaptest.NewLogger(t))

	existing := &activitypub.Actor{
		BaseObject: activitypub.BaseObject{
			ID: "https://dev.lesser.host/users/admin",
		},
		PreferredUsername: "admin",
		URL:               "https://dev.lesser.host/@admin",
		Inbox:             "https://dev.lesser.host/users/admin/inbox",
		Outbox:            "https://dev.lesser.host/users/admin/outbox",
		Followers:         "https://dev.lesser.host/users/admin/followers",
		Following:         "https://dev.lesser.host/users/admin/following",
		Liked:             "https://dev.lesser.host/users/admin/liked",
		Icon: &activitypub.Image{
			URL: "https://cdn.dev.lesser.host/avatars/admin.png",
		},
	}

	incoming := &activitypub.Actor{
		Name:    "Administrator QA",
		Summary: "Updated summary",
		Icon: &activitypub.Image{
			URL: "https://cdn.dev.lesser.host/avatars/admin-updated.png",
		},
		Discoverable:              true,
		ManuallyApprovesFollowers: true,
	}

	merged := repo.mergeActorDataForUpdate("admin", existing, incoming)

	require.Equal(t, "https://dev.lesser.host/users/admin", merged.ID)
	require.Equal(t, "Administrator QA", merged.Name)
	require.Equal(t, "Updated summary", merged.Summary)
	require.True(t, merged.Discoverable)
	require.True(t, merged.ManuallyApprovesFollowers)
	require.NotNil(t, merged.Icon)
	require.Equal(t, "https://cdn.dev.lesser.host/avatars/admin-updated.png", merged.Icon.URL)
	require.Equal(t, "admin", merged.PreferredUsername)
	require.Equal(t, "https://dev.lesser.host/@admin", merged.URL)
}

func TestMergeActorDataForUpdate_DerivesIdentifiersWhenMissing(t *testing.T) {
	repo := NewAccountRepository(nil, "test-table", "dev.lesser.host", zaptest.NewLogger(t))

	incoming := &activitypub.Actor{
		Name: "Test User",
	}

	merged := repo.mergeActorDataForUpdate("tester", nil, incoming)

	require.Equal(t, "https://dev.lesser.host/users/tester", merged.ID)
	require.Equal(t, "https://dev.lesser.host/@tester", merged.URL)
	require.Equal(t, "https://dev.lesser.host/users/tester/inbox", merged.Inbox)
	require.Equal(t, "https://dev.lesser.host/users/tester/outbox", merged.Outbox)
	require.Equal(t, "https://dev.lesser.host/users/tester/followers", merged.Followers)
	require.Equal(t, "https://dev.lesser.host/users/tester/following", merged.Following)
	require.Equal(t, "https://dev.lesser.host/users/tester/liked", merged.Liked)
	require.Equal(t, "tester", merged.PreferredUsername)
	require.Equal(t, "Test User", merged.Name)
}

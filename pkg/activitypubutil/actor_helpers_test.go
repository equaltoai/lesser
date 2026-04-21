package activitypubutil

import (
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/stretchr/testify/require"
)

func TestDerivePreferredUsername(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		actor    *activitypub.Actor
		fallback string
		expected string
	}{
		{
			name: "uses preferred username",
			actor: &activitypub.Actor{
				PreferredUsername: "alice",
			},
			expected: "alice",
		},
		{
			name: "falls back to actor ID",
			actor: &activitypub.Actor{
				BaseObject: activitypub.BaseObject{ID: "https://example.org/users/bob"},
			},
			expected: "bob",
		},
		{
			name: "falls back to actor URL",
			actor: &activitypub.Actor{
				URL: "https://example.org/@carol",
			},
			expected: "carol",
		},
		{
			name:     "uses fallback when no actor",
			fallback: "@dave@example.org",
			expected: "dave",
		},
		{
			name: "strips suffix json",
			actor: &activitypub.Actor{
				BaseObject: activitypub.BaseObject{ID: "https://remote/users/erin.json"},
			},
			expected: "erin",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.expected, DerivePreferredUsername(tc.actor, tc.fallback))
		})
	}
}

func TestBuildLocalActorFromUser(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	user := &storage.User{
		Username:     "frank",
		DisplayName:  "Frank Lambda",
		Note:         "hello world",
		Avatar:       "https://cdn.example/avatar.png",
		Header:       "https://cdn.example/header.png",
		URL:          "https://app.example/@frank",
		Locked:       true,
		Discoverable: true,
		CreatedAt:    now.Add(-24 * time.Hour),
		UpdatedAt:    now,
		Fields: []map[string]string{
			{"name": "Location", "value": "Earth"},
		},
	}

	actor := BuildLocalActor("frank", "https://app.example", user, nil)
	require.NotNil(t, actor)
	require.Equal(t, activitypub.PersonType, actor.Type)
	require.Equal(t, "frank", actor.PreferredUsername)
	require.Equal(t, "https://app.example/users/frank", actor.ID)
	require.Equal(t, "https://app.example/@frank", actor.URL)
	require.Equal(t, "https://app.example/users/frank/inbox", actor.Inbox)
	require.Equal(t, "https://app.example/users/frank/outbox", actor.Outbox)
	require.Equal(t, "https://app.example/users/frank/followers", actor.Followers)
	require.Equal(t, "https://app.example/users/frank/following", actor.Following)
	require.NotNil(t, actor.Endpoints)
	require.Equal(t, "https://app.example/inbox", actor.Endpoints.SharedInbox)
	require.Equal(t, "Frank Lambda", actor.Name)
	require.Equal(t, "hello world", actor.Summary)
	require.NotNil(t, actor.Icon)
	require.Equal(t, "https://cdn.example/avatar.png", actor.Icon.URL)
	require.NotNil(t, actor.Image)
	require.Equal(t, "https://cdn.example/header.png", actor.Image.URL)
	require.Len(t, actor.Attachment, 1)
	require.Equal(t, "PropertyValue", actor.Attachment[0].Type)
	require.True(t, actor.ManuallyApprovesFollowers)
	require.True(t, actor.Discoverable)
	require.NotNil(t, actor.Published)
	require.NotNil(t, actor.Updated)
}

func TestBuildLocalActorPreservesExistingFields(t *testing.T) {
	t.Parallel()

	pub := time.Now().Add(-48 * time.Hour)
	existing := &activitypub.Actor{
		BaseObject: activitypub.BaseObject{
			ID:        "https://remote/users/grace",
			Type:      activitypub.ServiceType,
			Published: &pub,
		},
		PreferredUsername: "grace",
		Summary:           "remote summary",
		Icon: &activitypub.Image{
			BaseObject: activitypub.BaseObject{Type: activitypub.ImageType},
			URL:        "https://remote/avatar.png",
		},
	}

	user := &storage.User{
		Username:    "grace",
		DisplayName: "Grace Hopper",
		Note:        "local note",
	}

	actor := BuildLocalActor("grace", "https://app.example", user, existing)
	require.NotNil(t, actor)
	require.Equal(t, activitypub.ServiceType, actor.Type)
	require.Equal(t, "remote summary", actor.Summary)
	require.Equal(t, "https://remote/avatar.png", actor.Icon.URL)
	require.NotNil(t, actor.Published)
	require.Equal(t, pub.UTC(), actor.Published.UTC())
}

func TestApplyLocalActorIdentifiers_UsesManifestSharedInbox(t *testing.T) {
	t.Parallel()

	actor := &activitypub.Actor{PublicKey: &activitypub.PublicKey{PublicKeyPem: "pem"}}
	ApplyLocalActorIdentifiers(actor, "https://example.com/", "alice")

	require.Equal(t, "https://example.com/users/alice", actor.ID)
	require.Equal(t, "https://example.com/@alice", actor.URL)
	require.Equal(t, "https://example.com/users/alice/inbox", actor.Inbox)
	require.Equal(t, "https://example.com/users/alice/outbox", actor.Outbox)
	require.Equal(t, "https://example.com/users/alice/followers", actor.Followers)
	require.Equal(t, "https://example.com/users/alice/following", actor.Following)
	require.Equal(t, "https://example.com/users/alice/liked", actor.Liked)
	require.NotNil(t, actor.Endpoints)
	require.Equal(t, "https://example.com/inbox", actor.Endpoints.SharedInbox)
	require.Equal(t, "https://example.com/users/alice", actor.PublicKey.Owner)
	require.Equal(t, "https://example.com/users/alice#main-key", actor.PublicKey.ID)
}

func TestMergeActorMetadata(t *testing.T) {
	t.Parallel()

	pub := time.Now()
	src := &activitypub.Actor{
		BaseObject: activitypub.BaseObject{
			ID:        "https://example/users/henry",
			Type:      activitypub.PersonType,
			Published: &pub,
		},
		PreferredUsername: "henry",
		Name:              "Henry",
		URL:               "https://example/@henry",
		Icon: &activitypub.Image{
			BaseObject: activitypub.BaseObject{Type: activitypub.ImageType},
			URL:        "https://cdn/henry.png",
		},
		Endpoints: &activitypub.Endpoints{
			SharedInbox: "https://example/inbox",
		},
		ManuallyApprovesFollowers: true,
		Discoverable:              true,
	}

	dst := &activitypub.Actor{}
	MergeActorMetadata(dst, src)

	require.Equal(t, "https://example/users/henry", dst.ID)
	require.Equal(t, "henry", dst.PreferredUsername)
	require.Equal(t, "Henry", dst.Name)
	require.NotNil(t, dst.Published)
	require.NotSame(t, src.Icon, dst.Icon)
	require.Equal(t, src.Icon.URL, dst.Icon.URL)
	require.True(t, dst.ManuallyApprovesFollowers)
	require.True(t, dst.Discoverable)
	require.NotNil(t, dst.Endpoints)
	require.Equal(t, "https://example/inbox", dst.Endpoints.SharedInbox)
}

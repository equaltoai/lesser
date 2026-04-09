package activitypubutil

import (
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/stretchr/testify/require"
)

func TestDerivePreferredUsername_AdditionalBranches(t *testing.T) {
	t.Parallel()

	t.Run("returns identifier when id is not a URL", func(t *testing.T) {
		t.Parallel()
		actor := &activitypub.Actor{BaseObject: activitypub.BaseObject{ID: "simple-id"}}
		require.Equal(t, "simple-id", DerivePreferredUsername(actor, ""))
	})

	t.Run("returns raw value when id URL cannot parse", func(t *testing.T) {
		t.Parallel()
		actor := &activitypub.Actor{BaseObject: activitypub.BaseObject{ID: "https://%zz"}}
		require.Equal(t, "https://%zz", DerivePreferredUsername(actor, ""))
	})

	t.Run("fallback empty returns empty", func(t *testing.T) {
		t.Parallel()
		require.Equal(t, "", DerivePreferredUsername(nil, " "))
	})
}

func TestNormalizeBaseURL(t *testing.T) {
	t.Parallel()

	require.Equal(t, "", normalizeBaseURL(""))
	require.Equal(t, "https://example.com", normalizeBaseURL("example.com/"))
	require.Equal(t, "http://example.com", normalizeBaseURL("http://example.com/"))
	require.Equal(t, "https://example.com", normalizeBaseURL("https://example.com/"))
}

func TestBuildLocalActor_EmptyBaseAndUsernameBranches(t *testing.T) {
	t.Parallel()

	actor := BuildLocalActor("", "", nil, nil)
	require.NotNil(t, actor)
	require.Equal(t, activitypub.PersonType, actor.Type)
	require.Empty(t, actor.ID)

	user := &storage.User{Username: "sam", URL: "https://app.example/@sam"}
	actor2 := BuildLocalActor("", "", user, nil)
	require.Equal(t, "sam", actor2.PreferredUsername)
}

func TestBuildLocalActor_CanonicalizesLocalPublicKeyIdentifiers(t *testing.T) {
	t.Parallel()

	existing := &activitypub.Actor{
		BaseObject: activitypub.BaseObject{
			ID: "https://app.example/users/Sam",
		},
		PreferredUsername: "Sam",
		PublicKey: &activitypub.PublicKey{
			ID:           "https://app.example/users/Sam#main-key",
			Owner:        "https://app.example/users/Sam",
			PublicKeyPem: "pem",
		},
	}

	actor := BuildLocalActor("sam", "https://app.example", nil, existing)
	require.NotNil(t, actor)
	require.Equal(t, "https://app.example/users/sam", actor.ID)
	require.Equal(t, "sam", actor.PreferredUsername)
	require.NotNil(t, actor.PublicKey)
	require.Equal(t, "https://app.example/users/sam", actor.PublicKey.Owner)
	require.Equal(t, "https://app.example/users/sam#main-key", actor.PublicKey.ID)
	require.Equal(t, "pem", actor.PublicKey.PublicKeyPem)
}

func TestBuildLocalActor_PreservesExistingLocalOriginWhenHostMatches(t *testing.T) {
	t.Parallel()

	existing := &activitypub.Actor{
		BaseObject: activitypub.BaseObject{
			ID: "https://localhost/users/Alice",
		},
		PreferredUsername: "Alice",
	}

	actor := BuildLocalActor("alice", "http://localhost", nil, existing)
	require.NotNil(t, actor)
	require.Equal(t, "https://localhost/users/alice", actor.ID)
	require.Equal(t, "https://localhost/@alice", actor.URL)
	require.Equal(t, "https://localhost/users/alice/inbox", actor.Inbox)
}

func TestMergeUserProfile_AttachmentsAndImages(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	user := &storage.User{
		Username:     "jane",
		DisplayName:  "",
		Note:         "hi",
		Avatar:       "",
		Header:       "https://cdn.example/header.png",
		Locked:       true,
		Discoverable: true,
		CreatedAt:    now.Add(-time.Hour),
		UpdatedAt:    now,
		Fields: []map[string]string{
			{"name": "", "value": ""},
			{"name": "Website", "value": "https://example.com"},
		},
	}

	actor := &activitypub.Actor{
		BaseObject: activitypub.BaseObject{Type: activitypub.PersonType},
		// Ensure these are already true so mergeUserProfile won't override with false.
		ManuallyApprovesFollowers: true,
		Discoverable:              true,
	}

	out := BuildLocalActor("jane", "https://app.example", user, actor)
	require.Equal(t, "jane", out.PreferredUsername)
	require.Equal(t, "hi", out.Summary)
	require.NotNil(t, out.Image)
	require.Equal(t, "https://cdn.example/header.png", out.Image.URL)
	require.Nil(t, out.Icon) // Avatar is empty, buildImage returns nil
	require.Len(t, out.Attachment, 1)
	require.Equal(t, "Website", out.Attachment[0].Name)
	require.NotNil(t, out.Published)
	require.NotNil(t, out.Updated)
}

func TestMergeActorMetadata_ClonesCollections(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	src := &activitypub.Actor{
		BaseObject: activitypub.BaseObject{
			Type:      activitypub.PersonType,
			ID:        "https://example/users/alice",
			Published: &now,
			To:        []string{"https://www.w3.org/ns/activitystreams#Public"},
			CC:        []string{"https://example/users/bob"},
		},
		AlsoKnownAs: []string{"https://alt.example/users/alice"},
		Endpoints:   &activitypub.Endpoints{SharedInbox: "https://example/inbox"},
		PublicKey:   &activitypub.PublicKey{ID: "key-1", Owner: "alice"},
		Icon:        &activitypub.Image{BaseObject: activitypub.BaseObject{Type: activitypub.ImageType}, URL: "https://cdn/icon.png"},
	}

	dst := &activitypub.Actor{}
	MergeActorMetadata(dst, src)

	require.NotNil(t, dst.To)
	require.Equal(t, src.To, dst.To)
	dst.To[0] = "changed"
	require.NotEqual(t, src.To[0], dst.To[0])
	require.NotNil(t, dst.CC)
	require.Equal(t, src.CC, dst.CC)
	dst.CC[0] = "changed"
	require.NotEqual(t, src.CC[0], dst.CC[0])
	require.NotNil(t, dst.AlsoKnownAs)
	require.Equal(t, src.AlsoKnownAs, dst.AlsoKnownAs)
	dst.AlsoKnownAs[0] = "changed"
	require.NotEqual(t, src.AlsoKnownAs[0], dst.AlsoKnownAs[0])
	require.NotNil(t, dst.PublicKey)
	require.NotSame(t, src.PublicKey, dst.PublicKey)
	require.NotNil(t, dst.Endpoints)
	require.NotSame(t, src.Endpoints, dst.Endpoints)
	require.NotNil(t, dst.Icon)
	require.NotSame(t, src.Icon, dst.Icon)
	require.NotNil(t, dst.Published)
	require.NotSame(t, src.Published, dst.Published)
}

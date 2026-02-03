package activitypubutil

import (
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/agents"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/stretchr/testify/require"
)

func TestBuildLocalActor_AgentManifestApplied(t *testing.T) {
	t.Parallel()

	user := &storage.User{
		Username:     "agent-1",
		URL:          "https://app.example/@agent-1",
		IsAgent:      true,
		AgentVersion: "v1.2.3",
		AgentType:    "assistant",
		AgentOwner:   "operator",
		AgentCapabilities: &agents.Capabilities{
			CanPost:           true,
			CanReply:          true,
			CanBoost:          false,
			CanFollow:         true,
			CanDM:             false,
			RestrictedDomains: []string{"example.com"},
			MaxPostsPerHour:   5,
			RequiresApproval:  true,
		},
	}

	actor := BuildLocalActor("", "", user, nil)
	require.Equal(t, activitypub.ServiceType, actor.Type)
	require.NotNil(t, actor.AgentManifest)
	require.Equal(t, "Agent", actor.AgentManifest.Type)
	require.Equal(t, "v1.2.3", actor.AgentManifest.Version)
	require.Equal(t, "assistant", actor.AgentManifest.Purpose)
	require.Equal(t, "@operator", actor.AgentManifest.OperatedBy)

	require.NotNil(t, actor.AgentManifest.Capabilities)
	require.True(t, actor.AgentManifest.Capabilities.CanPost)
	require.True(t, actor.AgentManifest.Capabilities.CanReply)
	require.True(t, actor.AgentManifest.Capabilities.CanFollow)
	require.False(t, actor.AgentManifest.Capabilities.CanBoost)
	require.False(t, actor.AgentManifest.Capabilities.CanDM)
	require.Equal(t, 5, actor.AgentManifest.Capabilities.MaxPostsPerHour)
	require.True(t, actor.AgentManifest.Capabilities.RequiresApproval)
	require.Equal(t, []string{"example.com"}, actor.AgentManifest.Capabilities.RestrictedDomains)

	user.AgentCapabilities.RestrictedDomains[0] = "mutated.example"
	require.Equal(t, []string{"example.com"}, actor.AgentManifest.Capabilities.RestrictedDomains)
}

func TestNormalizeOperatedBy(t *testing.T) {
	t.Parallel()

	require.Equal(t, "", normalizeOperatedBy(" "))
	require.Equal(t, "@alice", normalizeOperatedBy("alice"))
	require.Equal(t, "@alice", normalizeOperatedBy("@alice"))
}

func TestApplyAgentManifest_ExistingManifestAndNilInputs(t *testing.T) {
	t.Parallel()

	applyAgentManifest(nil, nil)
	ensureActorType(nil, nil)

	user := &storage.User{
		IsAgent:      true,
		AgentVersion: "v9",
		AgentType:    "assistant",
		AgentOwner:   "@owner",
	}

	actor := &activitypub.Actor{
		BaseObject: activitypub.BaseObject{Type: activitypub.PersonType},
		AgentManifest: &activitypub.AgentManifest{
			Type: "",
		},
	}

	ensureActorType(actor, user)
	require.Equal(t, activitypub.ServiceType, actor.Type)
	require.NotNil(t, actor.AgentManifest)
	require.Equal(t, "Agent", actor.AgentManifest.Type)
	require.Equal(t, "v9", actor.AgentManifest.Version)
	require.Equal(t, "assistant", actor.AgentManifest.Purpose)
	require.Equal(t, "@owner", actor.AgentManifest.OperatedBy)
}

func TestCloneAndMergeHelpers_AdditionalBranches(t *testing.T) {
	t.Parallel()

	require.Nil(t, cloneAttachments(nil))
	require.Nil(t, cloneAttachments([]activitypub.Attachment{}))

	now := time.Now().UTC()
	updated := now.Add(time.Minute)
	img := &activitypub.Image{
		BaseObject: activitypub.BaseObject{Type: activitypub.ImageType, Published: &now, Updated: &updated},
		URL:        "https://cdn.example/img.png",
	}
	clone := cloneImage(img)
	require.NotNil(t, clone)
	require.NotSame(t, img, clone)
	require.NotNil(t, clone.Published)
	require.Equal(t, now.UTC(), clone.Published.UTC())
	require.NotNil(t, clone.Updated)
	require.Equal(t, updated.UTC(), clone.Updated.UTC())

	dst := &activitypub.Actor{
		Endpoints: &activitypub.Endpoints{},
		PublicKey: &activitypub.PublicKey{},
		Icon:      &activitypub.Image{},
		Image:     &activitypub.Image{},
	}
	src := &activitypub.Actor{
		BaseObject: activitypub.BaseObject{
			Published: &now,
			Updated:   &updated,
		},
		Endpoints: &activitypub.Endpoints{SharedInbox: "https://example/inbox"},
		PublicKey: &activitypub.PublicKey{ID: "key", Owner: "owner"},
		Icon:      &activitypub.Image{MediaType: "image/png"},
		Image:     &activitypub.Image{MediaType: "image/jpeg"},
	}

	copyEndpointFields(dst, src)
	require.Equal(t, "https://example/inbox", dst.Endpoints.SharedInbox)

	copyKeyFields(dst, src)
	require.Equal(t, "key", dst.PublicKey.ID)
	require.Equal(t, "owner", dst.PublicKey.Owner)

	copyMediaFields(dst, src)
	require.Equal(t, "image/png", dst.Icon.MediaType)
	require.Equal(t, "image/jpeg", dst.Image.MediaType)

	copyTemporalFields(dst, src)
	require.NotNil(t, dst.Published)
	require.NotNil(t, dst.Updated)
}

func TestPreferFirst_AllEmptyReturnsEmpty(t *testing.T) {
	t.Parallel()
	require.Equal(t, "", preferFirst(" ", "\t"))
}

func TestActorHelperBranches(t *testing.T) {
	t.Parallel()

	MergeActorMetadata(nil, &activitypub.Actor{})
	MergeActorMetadata(&activitypub.Actor{}, nil)

	require.Equal(t, "", deriveFromURL(""))
	require.Equal(t, "https://%zz", deriveFromURL("https://%zz"))

	require.Equal(t, "", lastPathSegment("/"))
	require.Equal(t, "alice", lastPathSegment("/users/alice/"))

	originalAttachments := []activitypub.Attachment{{Type: "PropertyValue", Name: "n", Value: "v"}}
	attachments := cloneAttachments(originalAttachments)
	require.Len(t, attachments, 1)
	require.NotSame(t, &originalAttachments[0], &attachments[0])

	require.Nil(t, cloneEndpoints(nil))
	require.Nil(t, clonePublicKey(nil))

	src := &activitypub.Actor{
		BaseObject: activitypub.BaseObject{
			Context: activitypub.Context.Clone(),
			To:      []string{"https://www.w3.org/ns/activitystreams#Public"},
		},
		Endpoints: &activitypub.Endpoints{SharedInbox: "https://example/inbox"},
	}
	dst := &activitypub.Actor{}

	copyScalarFields(dst, src)
	require.NotNil(t, dst.Context)
	src.Context = append(src.Context, "mutate")
	require.NotEqual(t, len(src.Context), len(dst.Context))

	copySliceFields(dst, src)
	require.Equal(t, src.To, dst.To)
	dst.To[0] = "changed"
	require.NotEqual(t, src.To[0], dst.To[0])

	copyEndpointFields(dst, src)
	require.NotNil(t, dst.Endpoints)
	require.Equal(t, "https://example/inbox", dst.Endpoints.SharedInbox)
	require.NotSame(t, src.Endpoints, dst.Endpoints)
}

func TestCloneActor_DeepCopiesOptionalFields(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	updated := now.Add(time.Hour)

	actor := &activitypub.Actor{
		BaseObject: activitypub.BaseObject{
			Context:   activitypub.Context.With("extra"),
			ID:        "https://example/users/alice",
			Type:      activitypub.PersonType,
			Published: &now,
			Updated:   &updated,
			To:        []string{"to"},
			CC:        []string{"cc"},
			BTo:       []string{"bto"},
			BCC:       []string{"bcc"},
		},
		AlsoKnownAs: []string{"https://alt/users/alice"},
		Attachment:  []activitypub.Attachment{{Type: "PropertyValue", Name: "n", Value: "v"}},
		PublicKey:   &activitypub.PublicKey{ID: "key", Owner: "owner"},
		Endpoints:   &activitypub.Endpoints{SharedInbox: "https://example/inbox"},
		Icon: &activitypub.Image{
			BaseObject: activitypub.BaseObject{Type: activitypub.ImageType, Published: &now},
			URL:        "https://cdn.example/icon.png",
		},
		Image: &activitypub.Image{
			BaseObject: activitypub.BaseObject{Type: activitypub.ImageType, Updated: &updated},
			URL:        "https://cdn.example/img.png",
		},
	}

	cloned := cloneActor(actor)
	require.NotNil(t, cloned)
	require.NotSame(t, actor, cloned)
	require.NotSame(t, actor.PublicKey, cloned.PublicKey)
	require.NotSame(t, actor.Endpoints, cloned.Endpoints)
	require.NotSame(t, actor.Icon, cloned.Icon)
	require.NotSame(t, actor.Image, cloned.Image)
	require.NotSame(t, actor.Published, cloned.Published)
	require.NotSame(t, actor.Updated, cloned.Updated)

	require.Equal(t, actor.To, cloned.To)
	cloned.To[0] = "changed"
	require.NotEqual(t, actor.To[0], cloned.To[0])

	require.Equal(t, actor.Attachment, cloned.Attachment)
	cloned.Attachment[0].Name = "changed"
	require.NotEqual(t, actor.Attachment[0].Name, cloned.Attachment[0].Name)

	require.Nil(t, cloneImage(nil))
}

func TestCopySliceFields_CopiesAllCollections(t *testing.T) {
	t.Parallel()

	src := &activitypub.Actor{
		BaseObject: activitypub.BaseObject{
			To:  []string{"to"},
			CC:  []string{"cc"},
			BTo: []string{"bto"},
			BCC: []string{"bcc"},
		},
		AlsoKnownAs: []string{"aka"},
		Attachment:  []activitypub.Attachment{{Type: "PropertyValue", Name: "n", Value: "v"}},
	}
	dst := &activitypub.Actor{}

	copySliceFields(dst, src)
	require.Equal(t, src.To, dst.To)
	require.Equal(t, src.CC, dst.CC)
	require.Equal(t, src.BTo, dst.BTo)
	require.Equal(t, src.BCC, dst.BCC)
	require.Equal(t, src.AlsoKnownAs, dst.AlsoKnownAs)
	require.Equal(t, src.Attachment, dst.Attachment)
}

func TestCopyMediaFields_ClonesImagesWhenNil(t *testing.T) {
	t.Parallel()

	src := &activitypub.Actor{
		Icon:  &activitypub.Image{BaseObject: activitypub.BaseObject{Type: activitypub.ImageType}, URL: "https://cdn/icon.png"},
		Image: &activitypub.Image{BaseObject: activitypub.BaseObject{Type: activitypub.ImageType}, URL: "https://cdn/img.png"},
	}
	dst := &activitypub.Actor{}

	copyMediaFields(dst, src)
	require.NotNil(t, dst.Icon)
	require.NotNil(t, dst.Image)
	require.NotSame(t, src.Icon, dst.Icon)
	require.NotSame(t, src.Image, dst.Image)
}

func TestAttachmentsFromFields_EmptyReturnsNil(t *testing.T) {
	t.Parallel()

	require.Nil(t, attachmentsFromFields(nil))
	require.Nil(t, attachmentsFromFields([]map[string]string{{"name": "", "value": ""}}))
}

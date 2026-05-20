package transformations

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestActivityPubObjectToStorage_ValidationErrors(t *testing.T) {
	_, err := ActivityPubObjectToStorage(nil)
	require.Error(t, err)

	_, err = ActivityPubObjectToStorage(&activitypub.Note{})
	require.Error(t, err)
}

func TestActivityPubObjectToStorage_PopulatesFieldsAndJSON(t *testing.T) {
	updated := time.Now().UTC()
	obj := &activitypub.Note{
		BaseObject: activitypub.BaseObject{
			ID:        "https://example.com/objects/1",
			Type:      activitypub.NoteType,
			To:        []string{activitypub.PublicAddress},
			CC:        []string{"https://example.com/users/alice/followers"},
			BTo:       []string{"bto"},
			BCC:       []string{"bcc"},
			InReplyTo: "https://example.com/objects/parent",
			Summary:   "spoiler",
			Sensitive: true,
			Updated:   &updated,
		},
		Content:      "hi",
		AttributedTo: "https://example.com/users/alice",
		Attachment: []activitypub.Attachment{
			{Type: "Image", URL: "https://cdn/img.png"},
		},
		Tag: []activitypub.Tag{
			{Type: "Hashtag", Name: "#golang", Href: "https://example.com/tags/golang"},
		},
	}

	storageObj, err := ActivityPubObjectToStorage(obj)
	require.NoError(t, err)
	require.Equal(t, obj.ID, storageObj.ID)
	require.Equal(t, obj.Type, storageObj.Type)
	require.NotZero(t, storageObj.Published)
	require.Equal(t, updated, storageObj.Updated)
	require.NotNil(t, storageObj.InReplyTo)
	require.Equal(t, obj.InReplyTo, *storageObj.InReplyTo)
	require.NotEmpty(t, storageObj.AttachmentJSON)
	require.NotEmpty(t, storageObj.TagJSON)
}

func TestStorageObjectToActivityPub_ParsesJSONFields(t *testing.T) {
	reply := "https://example.com/objects/parent"
	now := time.Now().UTC()
	obj := &storagemodels.Object{
		ID:             "https://example.com/objects/1",
		Type:           activitypub.NoteType,
		Content:        "hi",
		Summary:        "spoiler",
		AttributedTo:   "https://example.com/users/alice",
		Sensitive:      true,
		To:             []string{activitypub.PublicAddress},
		InReplyTo:      &reply,
		Published:      now,
		Updated:        now,
		AttachmentJSON: "attachments:1",
		TagJSON:        "tags:1",
	}

	note, err := StorageObjectToActivityPub(obj)
	require.NoError(t, err)
	require.Equal(t, obj.ID, note.ID)
	require.Equal(t, obj.Content, note.Content)
	require.Equal(t, reply, note.InReplyTo)
	require.Len(t, note.Attachment, 0)
	require.Len(t, note.Tag, 0)
}

func TestStorageArticleToActivityPub(t *testing.T) {
	article := &storagemodels.Article{
		Object: storagemodels.Object{
			ID:      "https://example.com/articles/1",
			Type:    activitypub.ArticleType,
			Content: "# Title\n\nBody<script>alert(1)</script>",
			Name:    "Title",
		},
		ContentFormat: "markdown",
	}

	apArticle, err := StorageArticleToActivityPub(article)
	require.NoError(t, err)
	require.Equal(t, activitypub.ArticleType, apArticle.Type)
	require.Equal(t, "Title", apArticle.Name)
	require.Contains(t, apArticle.Content, `<h1 id="title">Title</h1>`)
	require.Contains(t, apArticle.Content, "<p>Body</p>")
	require.NotContains(t, apArticle.Content, "<script")
}

func TestActivityPubObjectToMastodon_VisibilityAndReply(t *testing.T) {
	published := time.Now().UTC()
	actor := &activitypub.Actor{
		BaseObject: activitypub.BaseObject{
			ID:   "https://example.com/users/alice",
			Type: activitypub.PersonType,
		},
		PreferredUsername: "alice",
		Inbox:             "https://example.com/users/alice/inbox",
		Outbox:            "https://example.com/users/alice/outbox",
	}

	obj := &activitypub.Note{
		BaseObject: activitypub.BaseObject{
			ID:        "https://example.com/objects/1",
			Type:      activitypub.NoteType,
			To:        []string{activitypub.PublicAddress},
			InReplyTo: "https://example.com/objects/parent",
			Published: &published,
		},
		Content:      "hi",
		AttributedTo: actor.ID,
		Tag: []activitypub.Tag{
			{Type: "Mention", Name: "@bob", Href: "https://remote.example/users/bob"},
			{Type: "Hashtag", Name: "#golang", Href: "https://example.com/tags/golang"},
		},
	}

	status, err := ActivityPubObjectToMastodon(obj, actor, "https://example.com")
	require.NoError(t, err)
	require.Equal(t, "public", status.Visibility)
	require.NotNil(t, status.InReplyToID)
	require.Len(t, status.Mentions, 1)
	require.Len(t, status.Tags, 1)
	require.Equal(t, "en", status.Language)
}

func TestDetermineVisibility(t *testing.T) {
	baseURL := "https://example.com"
	public := determineVisibility([]string{activitypub.PublicAddress}, nil, baseURL)
	require.Equal(t, "public", public)

	unlisted := determineVisibility(nil, []string{activitypub.PublicAddress}, baseURL)
	require.Equal(t, "unlisted", unlisted)

	private := determineVisibility([]string{baseURL + "/followers"}, nil, baseURL)
	require.Equal(t, "private", private)

	require.Equal(t, "direct", determineVisibility(nil, nil, baseURL))
}

func TestActivityPubActivityToStorage_AndBack(t *testing.T) {
	activity := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			ID:   "https://example.com/activities/1",
			Type: activitypub.CreateType,
		},
		Actor: "https://example.com/users/alice",
	}

	storageActivity, err := ActivityPubActivityToStorage(activity)
	require.NoError(t, err)
	require.Equal(t, "ACTOR#alice", storageActivity.PK)
	require.True(t, strings.HasPrefix(storageActivity.SK, "ACTIVITY#"))

	roundTrip, err := StorageActivityToActivityPub(storageActivity)
	require.NoError(t, err)
	require.Equal(t, activity, roundTrip)

	_, err = StorageActivityToActivityPub(&storagemodels.Activity{})
	require.Error(t, err)
}

func TestObjectToMastodonTransformer_ErrorsWithoutActorInContext(t *testing.T) {
	tr := NewObjectToMastodonTransformer()
	_, err := tr.Transform(context.Background(), &activitypub.Note{BaseObject: activitypub.BaseObject{ID: "1"}})
	require.Error(t, err)
}

func TestObjectBatchToMastodonTransformer_ErrorsWithoutActorInContext(t *testing.T) {
	tr := NewObjectBatchToMastodonTransformer()

	_, err := tr.TransformBatch(context.Background(), []*activitypub.Note{{BaseObject: activitypub.BaseObject{ID: "1"}}})
	require.Error(t, err)
}

func TestActorBatchToMastodonTransformer_TransformBatch(t *testing.T) {
	tr := NewActorBatchToMastodonTransformer()

	ctx := context.WithValue(context.Background(), "baseURL", "https://example.com")
	accounts, err := tr.TransformBatch(ctx, []*activitypub.Actor{
		{PreferredUsername: "alice"},
		{PreferredUsername: "bob"},
	})
	require.NoError(t, err)
	require.Len(t, accounts, 2)
}

func TestNewStorageToActorTransformer(t *testing.T) {
	tr := NewStorageToActorTransformer()

	actor, err := tr.Transform(context.Background(), &storagemodels.Actor{
		Username: "alice",
	})
	require.NoError(t, err)
	require.Equal(t, "alice", actor.PreferredUsername)
}

func TestGenerateNumericIDFromURLAndUsernameHelpers(t *testing.T) {
	require.Equal(t, "0", generateNumericIDFromURL(""))
	require.NotEmpty(t, generateNumericIDFromURL("https://example.com/objects/1"))
	require.NotEmpty(t, generateNumericIDFromUsername("alice"))
}

func TestExtractUsernameFromActorIDHelpers(t *testing.T) {
	require.Equal(t, "", extractUsernameFromActorID(""))
	require.Equal(t, "alice", extractUsernameFromActorID("https://example.com/users/alice"))
	require.Equal(t, "alice", extractUsernameFromActorID("https://example.com/actors/alice"))
}

func TestExtractLanguageFromContent(t *testing.T) {
	require.Equal(t, "", extractLanguageFromContent(""))
	require.Equal(t, "en", extractLanguageFromContent("hello"))
}

func TestTagAndAttachmentJSONHelpers(t *testing.T) {
	out, err := convertAttachmentsToJSON(nil)
	require.NoError(t, err)
	require.Equal(t, "", out)

	out, err = convertAttachmentsToJSON([]activitypub.Attachment{{Type: "Image"}})
	require.NoError(t, err)
	require.NotEmpty(t, out)

	parsedAttachments, err := parseAttachmentsFromJSON("x")
	require.NoError(t, err)
	require.Len(t, parsedAttachments, 0)

	out, err = convertTagsToJSON([]activitypub.Tag{{Type: "Hashtag"}})
	require.NoError(t, err)
	require.NotEmpty(t, out)

	parsedTags, err := parseTagsFromJSON("x")
	require.NoError(t, err)
	require.Len(t, parsedTags, 0)
}

func TestNewObjectToStorageTransformer(t *testing.T) {
	tr := NewObjectToStorageTransformer()
	_, err := tr.Transform(context.Background(), &activitypub.Note{BaseObject: activitypub.BaseObject{ID: "1"}})
	require.NoError(t, err)
}

func TestNewObjectToMastodonTransformer_SuccessPath(t *testing.T) {
	tr := NewObjectToMastodonTransformer()

	ctx := context.WithValue(context.Background(), "actor", &activitypub.Actor{PreferredUsername: "alice"})
	ctx = context.WithValue(ctx, "baseURL", "https://example.com")

	status, err := tr.Transform(ctx, &activitypub.Note{
		BaseObject: activitypub.BaseObject{
			ID:   "https://example.com/objects/1",
			Type: activitypub.NoteType,
			To:   []string{activitypub.PublicAddress},
		},
		Content: "hi",
	})
	require.NoError(t, err)
	require.IsType(t, &models.Status{}, status)
}

func TestNewActivityToStorageTransformer(t *testing.T) {
	tr := NewActivityToStorageTransformer()
	_, err := tr.Transform(context.Background(), &activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			ID:   "https://example.com/activities/1",
			Type: activitypub.CreateType,
		},
		Actor: "https://example.com/users/alice",
	})
	require.NoError(t, err)
}

func TestNewStorageToActivityTransformer(t *testing.T) {
	tr := NewStorageToActivityTransformer()
	_, err := tr.Transform(context.Background(), &storagemodels.Activity{})
	require.Error(t, err)
}

func TestConvertAttachmentsToMastodonMedia_SkipsProfileFields(t *testing.T) {
	media := convertAttachmentsToMastodonMedia([]activitypub.Attachment{
		{Type: propertyValueType, URL: "https://example.com/skip", Name: "field"},
		{Type: "Image", URL: "https://cdn/img.png", Name: "desc"},
	})
	require.Len(t, media, 1)
}

func TestConvertTagsToMastodonMentionsAndTags(t *testing.T) {
	tags := []activitypub.Tag{
		{Type: "Mention", Name: "@bob", Href: "https://remote.example/users/bob"},
		{Type: "Hashtag", Name: "#golang", Href: "https://example.com/tags/golang"},
	}

	mentions := convertTagsToMastodonMentions(tags, "https://example.com")
	require.Len(t, mentions, 1)

	hashtags := convertTagsToMastodonTags(tags, "https://example.com")
	require.Len(t, hashtags, 1)
}

func TestConvertActorFieldsToAttachments_RoundTrip(t *testing.T) {
	fields := convertAttachmentsToActorFields([]activitypub.Attachment{
		{Type: propertyValueType, Name: "n", Value: "v"},
	})

	attachments := convertActorFieldsToAttachments(fields)
	require.Len(t, attachments, 1)
	require.Equal(t, propertyValueType, attachments[0].Type)
}

func TestTransformers_AreRegistered(t *testing.T) {
	require.NotNil(t, ActivityPubRegistry)

	_, exists := ActivityPubRegistry.Get("object_to_storage")
	require.True(t, exists)

	transformers := ActivityPubRegistry.List()
	require.NotEmpty(t, transformers)
}

func TestNewGraphQLTransformer_Compiles(t *testing.T) {
	tr := NewGraphQLTransformer(func(_ context.Context, in string) (map[string]interface{}, error) {
		return map[string]interface{}{"in": in}, nil
	})
	out, err := tr.Transform(context.Background(), "x")
	require.NoError(t, err)
	require.Equal(t, "x", out["in"])
}

func TestNewStorageModelTransformer_Compiles(t *testing.T) {
	tr := NewStorageModelTransformer(func(_ context.Context, in int) (int, error) { return in + 1, nil })
	out, err := tr.Transform(context.Background(), 1)
	require.NoError(t, err)
	require.Equal(t, 2, out)
}

func TestNewAPIResponseTransformer_Compiles(t *testing.T) {
	tr := NewAPIResponseTransformer("https://example.com", func(_ context.Context, in string) (models.Account, error) {
		return models.Account{Username: in}, nil
	})
	out, err := tr.Transform(context.Background(), "alice")
	require.NoError(t, err)
	require.Equal(t, "alice", out.Username)
}

func TestNewStatusResponseTransformer_Compiles(t *testing.T) {
	tr := NewStatusResponseTransformer("https://example.com", func(_ context.Context, in string) (models.Status, error) {
		return models.Status{Content: in}, nil
	})
	out, err := tr.Transform(context.Background(), "hi")
	require.NoError(t, err)
	require.Equal(t, "hi", out.Content)
}

func TestNewActivityPubTransformer_Compiles(t *testing.T) {
	tr := NewActivityPubTransformer(func(_ context.Context, in string) (*activitypub.Actor, error) {
		return &activitypub.Actor{PreferredUsername: in}, nil
	})
	out, err := tr.Transform(context.Background(), "alice")
	require.NoError(t, err)
	require.Equal(t, "alice", out.PreferredUsername)
}

func TestNewTransformerFactories_AreUsable(t *testing.T) {
	ctx := context.WithValue(context.Background(), "baseURL", "https://example.com")
	actor := &activitypub.Actor{PreferredUsername: "alice"}

	accountTransformer := NewActorToMastodonTransformer()
	account, err := accountTransformer.Transform(ctx, actor)
	require.NoError(t, err)
	require.Equal(t, "alice", account.Username)

	objTransformer := NewObjectToStorageTransformer()
	_, err = objTransformer.Transform(context.Background(), &activitypub.Note{BaseObject: activitypub.BaseObject{ID: "1"}})
	require.NoError(t, err)

	activityTransformer := NewActivityToStorageTransformer()
	_, err = activityTransformer.Transform(context.Background(), &activitypub.Activity{BaseObject: activitypub.BaseObject{ID: "1"}, Actor: ""})
	require.Error(t, err)
}

func TestActivityTransformer_FromRegistry(t *testing.T) {
	trIface, exists := ActivityPubRegistry.Get("activity_to_storage")
	require.True(t, exists)

	tr, ok := trIface.(common.Transformer[*activitypub.Activity, *storagemodels.Activity])
	require.True(t, ok)

	_, err := tr.Transform(context.Background(), &activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			ID:   "https://example.com/activities/1",
			Type: activitypub.CreateType,
		},
		Actor: "https://example.com/users/alice",
	})
	require.NoError(t, err)
}

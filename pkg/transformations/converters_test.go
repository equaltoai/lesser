package transformations

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/stretchr/testify/require"
)

func TestGenerateNumericIDFromUsername_IsDeterministic(t *testing.T) {
	first := GenerateNumericIDFromUsername("alice")
	second := GenerateNumericIDFromUsername("alice")
	third := GenerateNumericIDFromUsername("bob")

	require.NotEmpty(t, first)
	require.Equal(t, first, second)
	require.NotEqual(t, first, third)
}

func TestGetAvatarURLAndHeaderURL(t *testing.T) {
	base := "https://base"

	require.Equal(t, base+"/avatars/original/missing.png", getAvatarURL(nil, base))
	require.Equal(t, "https://cdn/avatar.png", getAvatarURL(map[string]interface{}{"url": "https://cdn/avatar.png"}, base))
	require.Equal(t, base+"/avatars/original/missing.png", getAvatarURL(map[string]interface{}{"url": 123}, base))

	require.Equal(t, base+"/headers/original/missing.png", getHeaderURL(nil, base))
	require.Equal(t, "https://cdn/header.png", getHeaderURL(map[string]interface{}{"url": "https://cdn/header.png"}, base))
	require.Equal(t, base+"/headers/original/missing.png", getHeaderURL(map[string]interface{}{"url": 123}, base))
}

func TestIsBot(t *testing.T) {
	require.True(t, isBot("Service"))
	require.True(t, isBot("Application"))
	require.False(t, isBot("Person"))
}

func TestTransformAttachments(t *testing.T) {
	attachments := []interface{}{
		map[string]interface{}{"name": "n", "value": "v", "verified_at": "t"},
		map[string]interface{}{"name": "", "value": ""},
		"not-a-map",
	}

	fields := transformAttachments(attachments)
	require.Len(t, fields, 1)

	field, ok := fields[0].(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, "n", field["name"])
	require.Equal(t, "v", field["value"])
	require.Equal(t, "t", field["verified_at"])
}

func TestTransformMediaAttachments_ClassifiesTypeAndMeta(t *testing.T) {
	obj := map[string]interface{}{
		"attachment": []interface{}{
			map[string]interface{}{
				"type":      "Image",
				"mediaType": "image/png",
				"url":       "https://cdn/img.png",
				"name":      "desc",
				"width":     float64(100),
				"height":    float64(200),
			},
			map[string]interface{}{
				"type": "PropertyValue",
				"url":  "https://example.com/skip",
			},
		},
	}

	media := transformMediaAttachments(obj)
	require.Len(t, media, 1)

	item, ok := media[0].(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, "image", item["type"])
	require.Equal(t, "https://cdn/img.png", item["url"])
	require.Equal(t, "desc", item["description"])

	meta, ok := item["meta"].(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, 100, meta["width"])
	require.Equal(t, 200, meta["height"])
}

func TestTransformMentionsAndTags(t *testing.T) {
	obj := map[string]interface{}{
		"tag": []interface{}{
			map[string]interface{}{
				"type": "Mention",
				"href": "https://remote.example/users/bob",
				"name": "@bob",
			},
			map[string]interface{}{
				"type": "Hashtag",
				"href": "https://example.com/tags/golang",
				"name": "#golang",
			},
			map[string]interface{}{
				"type": "Note",
			},
		},
	}

	mentions := transformMentions(obj)
	require.Len(t, mentions, 1)
	mention, ok := mentions[0].(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, "bob", mention["username"])
	require.Equal(t, "bob@remote.example", mention["acct"])

	tags := transformTags(obj)
	require.Len(t, tags, 1)
	tag, ok := tags[0].(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, "golang", tag["name"])
	_, ok = tag["history"].([]interface{})
	require.True(t, ok)
}

func TestConvertObjectToMap_Note(t *testing.T) {
	now := time.Now().UTC()
	note := &activitypub.Note{
		BaseObject: activitypub.BaseObject{
			ID:        "https://example.com/objects/1",
			Published: &now,
			InReplyTo: "https://example.com/objects/parent",
			Summary:   "spoiler",
			Sensitive: true,
		},
		Content:      "<p>hello</p>",
		AttributedTo: "https://example.com/users/alice",
	}

	out := convertObjectToMap(note)
	require.Equal(t, note.ID, out["id"])
	require.Equal(t, note.Content, out["content"])
	require.Equal(t, note.Summary, out["summary"])
	require.Equal(t, true, out["sensitive"])
	require.Equal(t, note.InReplyTo, out["inReplyTo"])
	require.Equal(t, note.AttributedTo, out["attributedTo"])
	require.NotEmpty(t, out["published"])
}

func TestConvertNoteToMap_FallbackOnMarshalFailure(t *testing.T) {
	out := convertNoteToMap(map[string]interface{}{"id": "1"})
	require.Equal(t, "1", out["id"])

	type good struct {
		ID      string `json:"id"`
		Content string `json:"content"`
	}
	out = convertNoteToMap(good{ID: "2", Content: "hi"})
	require.Equal(t, "2", out["id"])
	require.Equal(t, "hi", out["content"])

	type bad struct {
		C chan int `json:"c"`
	}
	out = convertNoteToMap(bad{C: make(chan int)})
	require.Equal(t, "unknown", out["id"])
	require.Equal(t, "", out["content"])
}

func TestObjectToStatusWithContext(t *testing.T) {
	obj := map[string]interface{}{
		"id":        "https://example.com/objects/1",
		"content":   "hi",
		"published": "2024-01-02T03:04:05Z",
	}
	actor := &activitypub.Actor{PreferredUsername: "alice"}

	ctx := context.WithValue(context.Background(), "baseURL", "https://base")
	_, err := ObjectToStatusWithContext(ctx, obj)
	require.NoError(t, err)

	ctx = context.WithValue(ctx, "actor", actor)
	status, err := ObjectToStatusWithContext(ctx, obj)
	require.NoError(t, err)
	require.Equal(t, "hi", status.Content)
	require.Equal(t, "alice", status.Account.Username)
}

func TestObjectToStatusWithContextAndCounts(t *testing.T) {
	actor := &activitypub.Actor{PreferredUsername: "alice"}
	status := ObjectToStatusWithContextAndCounts(
		context.Background(),
		map[string]interface{}{"id": "https://example.com/objects/1"},
		actor,
		2,
		3,
		true,
		false,
		true,
		"https://base",
	)

	require.Equal(t, 2, status.FavouritesCount)
	require.Equal(t, 3, status.ReblogsCount)
	require.True(t, status.Favourited)
	require.False(t, status.Reblogged)
	require.True(t, status.Bookmarked)
}

func TestNotesToStatusAny_ExtractsActorFromAuthorID(t *testing.T) {
	note := map[string]interface{}{
		"id":       "https://example.com/objects/1",
		"content":  "hi",
		"authorID": "https://example.com/users/alice",
	}

	status := NotesToStatusAny(note, "https://base")
	require.Equal(t, "alice", status.Account.Username)
}

func TestExtractUsernameFromActorID(t *testing.T) {
	require.Equal(t, "", ExtractUsernameFromActorID(""))
	require.Equal(t, "alice", ExtractUsernameFromActorID("https://example.com/users/alice"))
	require.Equal(t, "alice", ExtractUsernameFromActorID("alice"))
}

func TestConvertObjectToMap_Fallbacks(t *testing.T) {
	out := convertObjectToMap(map[string]interface{}{"id": "1"})
	require.Equal(t, "1", out["id"])

	type good struct {
		ID string `json:"id"`
	}
	out = convertObjectToMap(good{ID: "2"})
	require.Equal(t, "2", out["id"])

	type bad struct {
		F func() `json:"f"`
	}
	out = convertObjectToMap(bad{})
	require.Equal(t, "unknown", out["id"])

	// Cover JSON marshal/unmarshal success path for non-map types.
	bytes, err := json.Marshal(map[string]interface{}{"id": "ok"})
	require.NoError(t, err)
	var decoded map[string]interface{}
	require.NoError(t, json.Unmarshal(bytes, &decoded))
	out = convertObjectToMap(decoded)
	require.Equal(t, "ok", out["id"])
}

func TestExtractLanguage(t *testing.T) {
	obj := map[string]interface{}{
		"contentMap": map[string]interface{}{
			"en": "hi",
		},
	}
	require.Equal(t, "en", *extractLanguage(obj))
	require.Nil(t, extractLanguage(map[string]interface{}{}))
}

func TestExtractReplyToID(t *testing.T) {
	obj := map[string]interface{}{
		"inReplyTo": "https://example.com/objects/parent",
	}
	replyTo := extractReplyToID(obj)
	require.NotNil(t, replyTo)
	require.NotEmpty(t, *replyTo)
	require.Nil(t, extractReplyToID(map[string]interface{}{}))
}

func TestNilInputs_ReturnZeroValues(t *testing.T) {
	account := ActorToAccountBase(nil, "https://base")
	require.Empty(t, account.ID)

	_, err := ActorToAccountWithContext(context.Background(), nil)
	require.NoError(t, err)

	_, err = ObjectToStatusWithContext(context.Background(), nil)
	require.NoError(t, err)

	status := ObjectToStatusBase(nil, &activitypub.Actor{PreferredUsername: "alice"}, "https://base")
	require.Empty(t, status.ID)

	status = ObjectToStatusAny(nil, &activitypub.Actor{PreferredUsername: "alice"}, "https://base")
	require.Empty(t, status.ID)

	status = ObjectToStatusWithContextAndCounts(context.Background(), nil, &activitypub.Actor{PreferredUsername: "alice"}, 0, 0, false, false, false, "https://base")
	require.Empty(t, status.ID)

	status = NotesToStatusAny(nil, "https://base")
	require.Empty(t, status.ID)
}

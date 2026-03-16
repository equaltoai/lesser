package transformations

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/equaltoai/lesser/cmd/api/models"
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

func TestProcessMentionTag_HTTPDomainParsing(t *testing.T) {
	mention := processMentionTag(map[string]interface{}{
		"type": "Mention",
		"href": "http://remote.example/users/bob",
		"name": "@bob",
	})

	mentionMap, ok := mention.(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, "bob@remote.example", mentionMap["acct"])
}

func TestTransformMentionsAndTags_InvalidInputs(t *testing.T) {
	require.Equal(t, []any{}, transformMentions(map[string]interface{}{}))
	require.Equal(t, []any{}, transformMentions(map[string]interface{}{"tag": "not-a-slice"}))

	require.Nil(t, processMentionTag("not-a-map"))
	require.Nil(t, processMentionTag(map[string]interface{}{"type": "Note"}))
	require.Nil(t, processMentionTag(map[string]interface{}{"type": "Mention", "href": "", "name": "@bob"}))
	require.Nil(t, processMentionTag(map[string]interface{}{"type": "Mention", "href": "users/bob", "name": ""}))

	require.Equal(t, []any{}, transformTags(map[string]interface{}{}))
	require.Equal(t, []any{}, transformTags(map[string]interface{}{"tag": "not-a-slice"}))

	require.Nil(t, processHashtagTag("not-a-map"))
	require.Nil(t, processHashtagTag(map[string]interface{}{"type": "Note"}))
	require.Nil(t, processHashtagTag(map[string]interface{}{"type": "Hashtag", "href": "", "name": "#go"}))
	require.Nil(t, processHashtagTag(map[string]interface{}{"type": "Hashtag", "href": "https://example/tags/go", "name": ""}))
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

func TestActorToAccountWithCounts_SetsCounts(t *testing.T) {
	account := ActorToAccountWithCounts(&activitypub.Actor{PreferredUsername: "alice"}, "https://base", 1, 2, 3)
	require.Equal(t, "alice", account.Username)
	require.Equal(t, 1, account.FollowersCount)
	require.Equal(t, 2, account.FollowingCount)
	require.Equal(t, 3, account.StatusesCount)

	require.Equal(t, models.Account{}, ActorToAccountWithCounts(nil, "https://base", 1, 2, 3))
}

func TestBuildAgentConstraints(t *testing.T) {
	require.Nil(t, buildAgentConstraints(nil))

	constraints := buildAgentConstraints(&activitypub.AgentCapabilities{
		MaxPostsPerHour:   10,
		RequiresApproval:  true,
		RestrictedDomains: []string{"example.com", "remote.example"},
	})

	require.Contains(t, constraints, "max_posts_per_hour:10")
	require.Contains(t, constraints, "requires_approval")
	require.Contains(t, constraints, "restricted_domains:example.com,remote.example")
}

func TestBuildAgentPostAttribution_Branches(t *testing.T) {
	require.Nil(t, buildAgentPostAttribution(nil))

	require.Nil(t, buildAgentPostAttribution(&activitypub.Actor{
		BaseObject: activitypub.BaseObject{Type: activitypub.PersonType},
	}))

	attribution := buildAgentPostAttribution(&activitypub.Actor{
		BaseObject: activitypub.BaseObject{Type: activitypub.ServiceType},
	})
	require.NotNil(t, attribution)
	require.Equal(t, activitypub.AgentAttributionSchemaVersion, attribution.SchemaVersion)
	require.Equal(t, "unknown", attribution.ModelID)
	require.Nil(t, attribution.Constraints)

	attribution = buildAgentPostAttribution(&activitypub.Actor{
		BaseObject: activitypub.BaseObject{Type: activitypub.PersonType},
		AgentManifest: &activitypub.AgentManifest{
			Version:    "m1",
			OperatedBy: "@delegator",
			Capabilities: &activitypub.AgentCapabilities{
				RequiresApproval: true,
			},
		},
	})
	require.NotNil(t, attribution)
	require.Equal(t, activitypub.AgentAttributionSchemaVersion, attribution.SchemaVersion)
	require.Equal(t, "m1", attribution.ModelID)
	require.Equal(t, "@delegator", attribution.DelegatedBy)
	require.Contains(t, attribution.Constraints, "requires_approval")
}

func TestObjectToStatusAny_SetsAgentAttribution(t *testing.T) {
	obj := map[string]interface{}{
		"id":        "https://example.com/objects/1",
		"content":   "hi",
		"published": "2024-01-02T03:04:05Z",
	}

	status := ObjectToStatusAny(obj, &activitypub.Actor{BaseObject: activitypub.BaseObject{Type: activitypub.ServiceType}}, "https://base")
	require.NotNil(t, status.AgentAttribution)

	require.Equal(t, models.Status{}, ObjectToStatusAny(nil, &activitypub.Actor{}, "https://base"))
	require.Equal(t, models.Status{}, ObjectToStatusAny(obj, nil, "https://base"))
}

func TestConverters_AdditionalBranches(t *testing.T) {
	now := time.Now().UTC()
	account := ActorToAccountBase(&activitypub.Actor{
		BaseObject:        activitypub.BaseObject{Published: &now},
		PreferredUsername: "alice",
	}, "https://base")
	require.NotEmpty(t, account.CreatedAt)

	require.Len(t, transformAttachments(map[string]interface{}{"name": "n", "value": "v"}), 1)
	require.Equal(t, []any{}, transformAttachments(nil))

	require.True(t, extractSensitive(map[string]interface{}{"sensitive": true}))
	require.False(t, extractSensitive(map[string]interface{}{"sensitive": "no"}))
	require.False(t, extractSensitive(map[string]interface{}{}))

	require.Equal(t, "spoiler", extractSpoilerText(map[string]interface{}{"summary": "spoiler"}))
	require.Equal(t, "", extractSpoilerText(map[string]interface{}{"summary": 123}))
	require.Equal(t, "", extractSpoilerText(map[string]interface{}{}))

	require.Nil(t, processMediaAttachmentItem(map[string]interface{}{"type": "PropertyValue"}))
	require.Nil(t, processMediaAttachmentItem(map[string]interface{}{"type": "Image", "url": ""}))

	require.Nil(t, processHashtagTag(map[string]interface{}{"type": "Hashtag", "href": "https://example/tags/x", "name": "#"}))

	require.NotEmpty(t, GenerateNumericIDFromURL("https://example.com/users/alice/"))
	require.Equal(t, "0", GenerateNumericIDFromURL(""))

	account, err := ActorToAccountWithContext(context.WithValue(context.Background(), "baseURL", "https://ctx"), &activitypub.Actor{PreferredUsername: "bob"})
	require.NoError(t, err)
	require.Equal(t, "bob", account.Username)

	require.Equal(t, "", ExtractUsernameFromActorID(""))

	status := ObjectToStatusBase(map[string]interface{}{
		"id":         "https://example.com/objects/1",
		"content":    "hi",
		"published":  "2024-01-02T03:04:05Z",
		"contentMap": map[string]interface{}{"en": "hi"},
	}, &activitypub.Actor{PreferredUsername: "alice"}, "https://base")
	require.Equal(t, "en", status.Language)

	mention := processMentionTag(map[string]interface{}{
		"type": "Mention",
		"href": "users/bob",
		"name": "@bob",
	})
	mentionMap, ok := mention.(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, "bob", mentionMap["acct"])
}

func TestGenerateNumericID_NormalizesNegativeHashes(t *testing.T) {
	var hash int64
	length := 0
	for i := 0; i < 100; i++ {
		hash = hash*31 + int64('z')
		length = i + 1
		if hash < 0 {
			break
		}
	}
	require.Less(t, hash, int64(0))

	id := generateNumericID(strings.Repeat("z", length))
	require.NotEmpty(t, id)
	require.NotEqual(t, "-", id[:1])
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

func TestTransformMediaAttachments_InvalidInputs(t *testing.T) {
	require.Equal(t, []any{}, transformMediaAttachments(map[string]interface{}{}))
	require.Equal(t, []any{}, transformMediaAttachments(map[string]interface{}{"attachment": "not-a-slice"}))

	require.Nil(t, processMediaAttachmentItem("not-a-map"))

	video := processMediaAttachmentItem(map[string]interface{}{
		"type":      "Video",
		"mediaType": "video/mp4",
		"url":       "https://cdn.example/video.mp4",
	})
	videoMap, ok := video.(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, "video", videoMap["type"])

	audio := processMediaAttachmentItem(map[string]interface{}{
		"type":      "Audio",
		"mediaType": "audio/mpeg",
		"url":       "https://cdn.example/audio.mp3",
	})
	audioMap, ok := audio.(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, "audio", audioMap["type"])

	unknown := processMediaAttachmentItem(map[string]interface{}{
		"type":      "Document",
		"mediaType": "application/pdf",
		"url":       "https://cdn.example/file.pdf",
	})
	unknownMap, ok := unknown.(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, "unknown", unknownMap["type"])
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

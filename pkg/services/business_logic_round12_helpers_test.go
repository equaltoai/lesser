package services

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/mastodon"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

func TestBusinessLogicService_Helpers(t *testing.T) {
	svc := &businessLogicService{
		deps: &ServiceDependencies{
			Config: &ServiceConfig{BaseURL: "https://example.com"},
		},
		logger: zap.NewNop(),
	}

	t.Run("createHashtagTags_builds_urls_and_preserves_case_in_name", func(t *testing.T) {
		hashtags := []string{"GoLang", "hello-world"}
		tags := svc.createHashtagTags(hashtags)
		if assert.Len(t, tags, 2) {
			assert.Equal(t, "Hashtag", tags[0].Type)
			assert.Equal(t, "#GoLang", tags[0].Name)
			assert.Equal(t, "https://example.com/tags/"+mastodon.NormalizeHashtag("GoLang"), tags[0].Href)
		}
	})

	t.Run("setNoteAddressing_handles_visibility_modes", func(t *testing.T) {
		actor := &activitypub.Actor{
			BaseObject: activitypub.BaseObject{ID: "https://example.com/users/alice"},
			Followers:  "https://example.com/users/alice/followers",
		}

		publicNote := &activitypub.Note{}
		svc.setNoteAddressing(publicNote, VisibilityPublic, actor)
		assert.Equal(t, []string{activitypub.PublicAddress}, publicNote.To)
		assert.Equal(t, []string{actor.Followers}, publicNote.CC)

		unlisted := &activitypub.Note{}
		svc.setNoteAddressing(unlisted, VisibilityUnlisted, actor)
		assert.Equal(t, []string{actor.Followers}, unlisted.To)
		assert.Equal(t, []string{activitypub.PublicAddress}, unlisted.CC)

		privateNote := &activitypub.Note{}
		svc.setNoteAddressing(privateNote, VisibilityPrivate, actor)
		assert.Equal(t, []string{actor.Followers}, privateNote.To)
		assert.Empty(t, privateNote.CC)

		direct := &activitypub.Note{Content: "@bob hi @carol,"}
		svc.setNoteAddressing(direct, VisibilityDirect, actor)
		assert.Equal(t, []string{
			"https://example.com/users/bob",
			"https://example.com/users/carol",
		}, direct.To)
	})

	t.Run("extractMentions_strips_punctuation_and_skips_empty", func(t *testing.T) {
		mentions := svc.extractMentions("@bob, hi @carol! @")
		assert.Equal(t, []string{
			"https://example.com/users/bob",
			"https://example.com/users/carol",
		}, mentions)
	})

	t.Run("normalizeObjectID_prefixes_baseurl_for_non_urls", func(t *testing.T) {
		assert.Equal(t, "https://example.com/objects/123", svc.normalizeObjectID("123"))
		assert.Equal(t, "https://remote.example/objects/xyz", svc.normalizeObjectID("https://remote.example/objects/xyz"))
	})

	t.Run("ownership_helpers_support_note_and_map", func(t *testing.T) {
		actorID := "https://example.com/users/alice"
		note := &activitypub.Note{AttributedTo: actorID}
		assert.True(t, svc.verifyObjectOwnership(note, actorID))
		assert.False(t, svc.verifyObjectOwnership(note, "https://example.com/users/bob"))

		objMap := map[string]interface{}{"attributedTo": actorID}
		assert.True(t, svc.verifyObjectOwnership(objMap, actorID))
		assert.False(t, svc.verifyObjectOwnership(map[string]interface{}{}, actorID))
	})

	t.Run("extractLinksFromContent_finds_http_urls", func(t *testing.T) {
		links := svc.extractLinksFromContent("hello http://a.example one https://b.example two ftp://nope")
		assert.Equal(t, []string{"http://a.example", "https://b.example"}, links)
	})

	t.Run("createNoteFromInput_defaults_visibility_and_sets_reply_and_tags", func(t *testing.T) {
		now := time.Unix(1700000000, 0).UTC()
		actor := &activitypub.Actor{
			BaseObject: activitypub.BaseObject{ID: "https://example.com/users/alice"},
			Followers:  "https://example.com/users/alice/followers",
		}
		input := &CreatePostInput{
			Content:     "Hello #Test",
			Visibility:  "",
			InReplyToID: "https://remote.example/objects/abc",
		}

		note, hashtags := svc.createNoteFromInput(input, actor, now)
		assert.Equal(t, VisibilityPublic, note.Visibility)
		assert.Equal(t, input.Content, note.Content)
		assert.Equal(t, actor.ID, note.AttributedTo)
		assert.Equal(t, input.InReplyToID, note.InReplyTo)
		assert.NotNil(t, note.Published)
		assert.Equal(t, now, *note.Published)
		assert.NotEmpty(t, note.ID)
		assert.Equal(t, []string{activitypub.PublicAddress}, note.To)
		assert.Equal(t, []string{actor.Followers}, note.CC)
		assert.NotEmpty(t, hashtags)
		assert.NotEmpty(t, note.Tag)
	})

	t.Run("processContentAndEmojis_errors_when_repo_storage_unavailable", func(t *testing.T) {
		svc2 := &businessLogicService{
			deps: &ServiceDependencies{
				Config: &ServiceConfig{BaseURL: "https://example.com"},
				Repos:  struct{}{},
			},
			logger: zap.NewNop(),
		}

		_, err := svc2.processContentAndEmojis(context.Background(), &activitypub.Note{Content: "hi :smile:"})
		assert.Error(t, err)
		se, ok := err.(*ServiceError)
		if assert.True(t, ok) {
			assert.Equal(t, "INTERNAL_ERROR", se.Code)
			assert.Equal(t, ErrEmojiRepositoryNotAvailable, se.Cause)
		}
	})
}

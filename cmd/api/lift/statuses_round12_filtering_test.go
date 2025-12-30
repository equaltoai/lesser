package lift

import (
	"context"
	"net/http"
	"testing"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/config"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestStatusFilteringHelpers_Round12(t *testing.T) {
	cfg := round11TestConfig()
	handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, makeRegistry(&NotesServiceStub{}, &AccountsServiceStub{}))

	actor := &activitypub.Actor{
		BaseObject:         activitypub.BaseObject{ID: cfg.ActorURL("alice"), Type: "Person"},
		PreferredUsername:  "alice",
		Name:               "alice",
		ManuallyApprovesFollowers: false,
	}

	noteMedia := &activitypub.Note{
		BaseObject: activitypub.BaseObject{
			ID:   cfg.ObjectURL("objects", "m1"),
			Type: activitypub.NoteType,
		},
		Attachment: []activitypub.Attachment{{URL: cfg.BaseURL() + "/media/1"}},
	}
	noteReply := &activitypub.Note{
		BaseObject: activitypub.BaseObject{
			ID:        cfg.ObjectURL("objects", "r1"),
			Type:      activitypub.NoteType,
			InReplyTo: cfg.ObjectURL("objects", "parent"),
		},
	}
	noteHashtag := &activitypub.Note{
		BaseObject: activitypub.BaseObject{
			ID:   cfg.ObjectURL("objects", "t1"),
			Type: activitypub.NoteType,
		},
		Tag: []activitypub.Tag{{Type: "Hashtag", Name: "#Go"}},
	}

	announce := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			ID:   cfg.BaseURL() + "/activities/a1",
			Type: activitypub.AnnounceType,
		},
		Actor:  cfg.ActorURL("alice"),
		Object: cfg.ObjectURL("objects", "orig"),
	}

	apiReblog := &apimodels.Status{Reblog: &apimodels.Status{ID: "orig"}}
	apiHashtag := &apimodels.Status{Tags: []any{map[string]any{"name": "#Go"}}}
	storageReblog := &storagemodels.Status{ReblogOfID: "orig"}

	params := accountStatusesParams{}
	require.False(t, handler.shouldFilterObject(noteMedia, params))

	params.onlyMedia = true
	require.False(t, handler.shouldFilterObject(noteMedia, params))
	require.True(t, handler.shouldFilterObject(noteReply, params))

	params = accountStatusesParams{excludeReplies: true}
	require.True(t, handler.shouldFilterObject(noteReply, params))
	require.False(t, handler.shouldFilterObject(noteMedia, params))

	params = accountStatusesParams{excludeReblogs: true}
	require.True(t, handler.shouldFilterObject(announce, params))
	require.True(t, handler.shouldFilterObject(apiReblog, params))
	require.True(t, handler.shouldFilterObject(storageReblog, params))

	params = accountStatusesParams{tagged: "go"}
	require.True(t, handler.shouldFilterObject(noteMedia, params))
	require.False(t, handler.shouldFilterObject(noteHashtag, params))

	// Exercise extractFromAPIStatus and extractFromActivityPubNote paths.
	require.True(t, handler.objectHasHashtags(apiHashtag, "go"))
	require.True(t, handler.objectHasHashtags(noteHashtag, "go"))
	require.False(t, handler.objectHasHashtags(noteHashtag, "rust"))

	// Ensure conversion path reaches logStatusConversion.
	ctx, err := round10NewLiftContext(http.MethodGet, "/statuses", nil, nil, nil)
	require.NoError(t, err)
	_ = handler.convertAndFilterObjects(ctx, []any{noteMedia, noteHashtag}, actor, accountStatusesParams{})
}

func TestDetermineUpdateDeliveryRecipients_Round12(t *testing.T) {
	cfg := &config.Config{
		Domain:          "example.com",
		JWTSecret:       round11StrongJWTSecret,
		DynamoTableName: "test-table",
		Stage:           "development",
	}

	state := &round10QueryState{
		relationshipRecords: []storagemodels.RelationshipRecord{
			{GSI1SK: "FOLLOWER#bob"},
			{GSI1SK: "FOLLOWER#bob@example.org"},
			{GSI1SK: "FOLLOWER#https://remote.example/users/bob"},
		},
	}

	handler, _, _ := round11NewHandler(t, cfg, state, makeRegistry(&NotesServiceStub{}, &AccountsServiceStub{}))

	actor := &activitypub.Actor{
		BaseObject:        activitypub.BaseObject{ID: cfg.ActorURL("alice"), Type: "Person"},
		PreferredUsername: "alice",
	}
	note := &activitypub.Note{
		BaseObject: activitypub.BaseObject{
			ID:   cfg.ObjectURL("objects", "s1"),
			Type: activitypub.NoteType,
			To:   []string{activitypub.PublicAddress, cfg.ActorURL("carol")},
			CC:   []string{cfg.ActorURL("dave")},
		},
		Tag: []activitypub.Tag{{Type: "Mention", Href: cfg.ActorURL("erin")}},
	}

	recipients, err := handler.determineUpdateDeliveryRecipients(context.Background(), actor, note)
	require.NoError(t, err)
	require.Contains(t, recipients, cfg.ActorURL("bob"))
	require.Contains(t, recipients, "https://example.org/users/bob")
	require.Contains(t, recipients, "https://remote.example/users/bob")
	require.Contains(t, recipients, cfg.ActorURL("carol"))
	require.Contains(t, recipients, cfg.ActorURL("dave"))
	require.Contains(t, recipients, cfg.ActorURL("erin"))
}

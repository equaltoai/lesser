package notecontract

import (
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
)

var (
	publicFixturePublished = time.Date(2026, time.March, 27, 13, 15, 0, 0, time.UTC)
	directFixturePublished = time.Date(2026, time.March, 27, 13, 16, 0, 0, time.UTC)
)

// PublicFixtureNote returns a live-shaped public note fixture with context,
// audiences, mentions, hashtags, attachments, quote metadata, and agent
// attribution.
func PublicFixtureNote() *activitypub.Note {
	return &activitypub.Note{
		BaseObject: activitypub.BaseObject{
			Context:   activitypub.Context.Clone(),
			ID:        "https://lesser.example/users/alice/statuses/public-fixture",
			Type:      activitypub.NoteType,
			Published: &publicFixturePublished,
			To:        []string{"https://www.w3.org/ns/activitystreams#Public"},
			CC:        []string{"https://lesser.example/users/alice/followers"},
			Summary:   "fixture spoiler",
			Sensitive: true,
		},
		Content:      "Fixture public note for @bob@example.net with #StatusContract",
		AttributedTo: "https://lesser.example/users/alice",
		Attachment: []activitypub.Attachment{
			{
				Type:      "Document",
				MediaType: "image/png",
				URL:       "https://cdn.lesser.example/media/public-fixture.png",
				Name:      "fixture-image",
				Width:     640,
				Height:    480,
			},
		},
		Tag: []activitypub.Tag{
			{Type: "Hashtag", Name: "#StatusContract", Href: "https://lesser.example/tags/statuscontract"},
			{Type: "Mention", Name: "@bob@example.net", Href: "https://remote.example/users/bob"},
		},
		Visibility:         "public",
		ConversationID:     "public-fixture-thread",
		QuoteURL:           "https://lesser.example/users/quote-target/statuses/1",
		Quoteable:          true,
		QuoteNotifications: true,
		QuoteContext: &activitypub.QuoteContext{
			OriginalNoteID:         "quoted-status-1",
			OriginalAuthor:         "https://lesser.example/users/quoted",
			OriginalAuthorUsername: "quoted",
			QuoteCount:             4,
			AllowWithdrawal:        true,
			QuoteAllowed:           true,
			Withdrawn:              false,
		},
		AgentAttribution: &activitypub.AgentPostAttribution{
			TriggerType:     "assistant",
			TriggerDetails:  "status-contract fixture",
			MemoryCitations: []string{"memory-1"},
			DelegatedBy:     "https://lesser.example/users/operator",
			DelegatedByDID:  "did:example:operator",
			Scopes:          []string{"post:public"},
			Constraints:     []string{"no_external_side_effects"},
			SchemaVersion:   activitypub.AgentAttributionSchemaVersion,
			ModelID:         "gpt-5.4",
		},
	}
}

// DirectFixtureNote returns a live-shaped direct-message note fixture with
// ActivityPub context, DM audiences, mention tags, conversation metadata, and
// agent attribution.
func DirectFixtureNote() *activitypub.Note {
	return &activitypub.Note{
		BaseObject: activitypub.BaseObject{
			Context:   activitypub.Context.Clone(),
			ID:        "https://lesser.example/users/alice/statuses/direct-fixture",
			Type:      activitypub.NoteType,
			Published: &directFixturePublished,
			To: []string{
				"https://remote.example/users/bob",
				"https://lesser.example/users/carol",
			},
			Summary: "fixture direct message",
		},
		Content:      "Fixture DM note for @bob@remote.example and @carol",
		AttributedTo: "https://lesser.example/users/alice",
		Tag: []activitypub.Tag{
			{Type: "Mention", Name: "@bob@remote.example", Href: "https://remote.example/users/bob"},
			{Type: "Mention", Name: "@carol", Href: "https://lesser.example/users/carol"},
		},
		Visibility:     "direct",
		ConversationID: "dm-fixture-thread",
		AgentAttribution: &activitypub.AgentPostAttribution{
			TriggerType:    "assistant",
			TriggerDetails: "direct-status-contract fixture",
			Scopes:         []string{"post:dm"},
			SchemaVersion:  activitypub.AgentAttributionSchemaVersion,
			ModelID:        "gpt-5.4",
		},
	}
}

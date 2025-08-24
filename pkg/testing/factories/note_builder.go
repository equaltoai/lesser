package factories

import (
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
)

// NoteBuilder builds ActivityPub Note objects using the builder pattern
type NoteBuilder struct {
	*BaseBuilder
	note *activitypub.Note
}

// NewNoteBuilder creates a new note builder
func NewNoteBuilder(domain string) *NoteBuilder {
	return &NoteBuilder{
		BaseBuilder: NewBaseBuilder(domain),
		note:        &activitypub.Note{},
	}
}

// Reset resets the builder to create a new note
func (b *NoteBuilder) Reset() *NoteBuilder {
	b.note = &activitypub.Note{
		BaseObject: activitypub.BaseObject{
			Context: []string{"https://www.w3.org/ns/activitystreams"},
			Type:    "Note",
		},
	}
	return b
}

// WithContent sets the note content
func (b *NoteBuilder) WithContent(content string) *NoteBuilder {
	b.note.Content = content
	return b
}

// WithAttributedTo sets the author
func (b *NoteBuilder) WithAttributedTo(actor string) *NoteBuilder {
	b.note.AttributedTo = actor
	return b
}

// WithInReplyTo sets the reply target
func (b *NoteBuilder) WithInReplyTo(replyTo string) *NoteBuilder {
	b.note.InReplyTo = replyTo
	return b
}

// WithSummary sets the content warning/summary
func (b *NoteBuilder) WithSummary(summary string) *NoteBuilder {
	b.note.Summary = summary
	return b
}

// WithSensitive sets the sensitive flag
func (b *NoteBuilder) WithSensitive(sensitive bool) *NoteBuilder {
	b.note.Sensitive = sensitive
	return b
}

// WithPublished sets the published timestamp
func (b *NoteBuilder) WithPublished(published time.Time) *NoteBuilder {
	b.note.Published = &published
	return b
}

// WithTo adds recipients to the "to" field
func (b *NoteBuilder) WithTo(recipients ...string) *NoteBuilder {
	b.note.To = append(b.note.To, recipients...)
	return b
}

// WithCC adds recipients to the "cc" field
func (b *NoteBuilder) WithCC(recipients ...string) *NoteBuilder {
	b.note.CC = append(b.note.CC, recipients...)
	return b
}

// WithTag adds a tag (mention, hashtag, etc.)
func (b *NoteBuilder) WithTag(tag activitypub.Tag) *NoteBuilder {
	b.note.Tag = append(b.note.Tag, tag)
	return b
}

// WithMention adds a mention tag
func (b *NoteBuilder) WithMention(name, href string) *NoteBuilder {
	mention := activitypub.Tag{
		Type: "Mention",
		Name: name,
		Href: href,
	}
	return b.WithTag(mention)
}

// WithHashtag adds a hashtag
func (b *NoteBuilder) WithHashtag(name string) *NoteBuilder {
	hashtag := activitypub.Tag{
		Type: "Hashtag",
		Name: "#" + name,
		Href: fmt.Sprintf("https://%s/tags/%s", b.domain, name),
	}
	return b.WithTag(hashtag)
}

// WithAttachment adds an attachment
func (b *NoteBuilder) WithAttachment(attachment activitypub.Attachment) *NoteBuilder {
	b.note.Attachment = append(b.note.Attachment, attachment)
	return b
}

// WithImageAttachment adds an image attachment
func (b *NoteBuilder) WithImageAttachment(url, mediaType string) *NoteBuilder {
	attachment := activitypub.Attachment{
		Type:      "Document",
		MediaType: mediaType,
		URL:       url,
	}
	return b.WithAttachment(attachment)
}

// WithID sets a custom ID
func (b *NoteBuilder) WithID(id string) *NoteBuilder {
	b.note.ID = id
	return b
}

// AsPublic makes the note public
func (b *NoteBuilder) AsPublic() *NoteBuilder {
	return b.WithTo("https://www.w3.org/ns/activitystreams#Public")
}

// AsFollowersOnly makes the note followers-only
func (b *NoteBuilder) AsFollowersOnly(actor string) *NoteBuilder {
	followersURL := fmt.Sprintf("%s/followers", actor)
	return b.WithTo(followersURL)
}

// AsDirect makes the note a direct message
func (b *NoteBuilder) AsDirect(recipients ...string) *NoteBuilder {
	return b.WithTo(recipients...)
}

// Build creates the note with defaults for any unset values
func (b *NoteBuilder) Build() *activitypub.Note {
	// Apply defaults
	if err := common.ValidateRequiredParam("ID", b.note.ID); err != nil {
		b.note.ID = b.GenerateID("statuses")
	}

	if err := common.ValidateRequiredParam("AttributedTo", b.note.AttributedTo); err != nil {
		b.note.AttributedTo = fmt.Sprintf("https://%s/users/testuser", b.domain)
	}

	if b.note.Published == nil {
		timestamp := b.GenerateTimestamp()
		b.note.Published = &timestamp
	}

	if len(b.note.To) == 0 {
		// Default to public
		b.note.To = []string{"https://www.w3.org/ns/activitystreams#Public"}
	}

	if err := common.ValidateRequiredParam("Content", b.note.Content); err != nil {
		b.note.Content = fmt.Sprintf("Test note %d", b.sequence)
	}

	// Create a copy to return
	result := *b.note

	// Reset for next build
	b.Reset()

	return &result
}

// BuildReply creates a reply note
func (b *NoteBuilder) BuildReply(content, inReplyTo, author string) *activitypub.Note {
	return b.Reset().
		WithContent(content).
		WithInReplyTo(inReplyTo).
		WithAttributedTo(author).
		AsPublic().
		Build()
}

// BuildDirectMessage creates a direct message note
func (b *NoteBuilder) BuildDirectMessage(content, from string, to ...string) *activitypub.Note {
	return b.Reset().
		WithContent(content).
		WithAttributedTo(from).
		AsDirect(to...).
		Build()
}

// BuildSensitiveNote creates a note with content warning
func (b *NoteBuilder) BuildSensitiveNote(content, summary, author string) *activitypub.Note {
	return b.Reset().
		WithContent(content).
		WithSummary(summary).
		WithSensitive(true).
		WithAttributedTo(author).
		AsPublic().
		Build()
}

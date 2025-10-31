// Package factories provides test data factories for consistent test data generation
package factories

import (
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
)

// ActivityFactory creates ActivityPub activities for testing
type ActivityFactory struct {
	domain   string
	sequence int
	baseTime time.Time
}

// NewActivityFactory creates a new activity factory
func NewActivityFactory(domain string) *ActivityFactory {
	return &ActivityFactory{
		domain:   domain,
		sequence: 1,
		baseTime: time.Now().Truncate(time.Hour),
	}
}

// ActivityOptions configures activity creation
type ActivityOptions struct {
	Type      string
	Actor     string
	Object    interface{}
	Target    string
	Published *time.Time
	ID        string
}

// CreateActivity creates a basic activity with default values
func (f *ActivityFactory) CreateActivity(opts ActivityOptions) *activitypub.Activity {
	if err := common.ValidateRequiredParam("type", opts.Type); err != nil {
		opts.Type = "Create"
	}

	if err := common.ValidateRequiredParam("actor", opts.Actor); err != nil {
		opts.Actor = f.generateActorID("testuser")
	}

	published := f.baseTime.Add(time.Duration(f.sequence) * time.Minute)
	if opts.Published != nil {
		published = *opts.Published
	}

	id := opts.ID
	if err := common.ValidateRequiredParam("id", id); err != nil {
		id = f.generateActivityID()
	}

	f.sequence++

	activity := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			Context:   activitypub.Context,
			ID:        id,
			Type:      opts.Type,
			Published: &published,
		},
		Actor: opts.Actor,
	}

	if opts.Object != nil {
		activity.Object = opts.Object
	}

	if err := common.ValidateRequiredParam("target", opts.Target); err == nil {
		activity.Target = opts.Target
	}

	return activity
}

// CreateNote creates a note object for activities
func (f *ActivityFactory) CreateNote(content string, actorID string, opts ...NoteOptions) *activitypub.Note {
	options := NoteOptions{}
	if len(opts) > 0 {
		options = opts[0]
	}

	id := options.ID
	if err := common.ValidateRequiredParam("id", id); err != nil {
		id = f.generateObjectID()
	}

	published := f.baseTime.Add(time.Duration(f.sequence) * time.Minute)
	if options.Published != nil {
		published = *options.Published
	}

	note := &activitypub.Note{
		BaseObject: activitypub.BaseObject{
			Context:   activitypub.Context,
			ID:        id,
			Type:      "Note",
			Published: &published,
			To:        []string{"https://www.w3.org/ns/activitystreams#Public"},
		},
		Content:      content,
		AttributedTo: actorID,
	}

	if err := common.ValidateRequiredParam("inReplyTo", options.InReplyTo); err == nil {
		note.InReplyTo = options.InReplyTo
	}

	if len(options.Tags) > 0 {
		note.Tag = options.Tags
	}

	if len(options.Attachments) > 0 {
		note.Attachment = options.Attachments
	}

	f.sequence++
	return note
}

// NoteOptions configures note creation
type NoteOptions struct {
	ID          string
	InReplyTo   string
	Tags        []activitypub.Tag
	Attachments []activitypub.Attachment
	Published   *time.Time
}

// CreateFollow creates a Follow activity
func (f *ActivityFactory) CreateFollow(follower, target string, opts ...ActivityOptions) *activitypub.Activity {
	options := ActivityOptions{
		Type:   "Follow",
		Actor:  follower,
		Object: target,
	}

	if len(opts) > 0 {
		if err := common.ValidateRequiredParam("id", opts[0].ID); err == nil {
			options.ID = opts[0].ID
		}
		if opts[0].Published != nil {
			options.Published = opts[0].Published
		}
	}

	return f.CreateActivity(options)
}

// CreateLike creates a Like activity
func (f *ActivityFactory) CreateLike(actor string, object interface{}, opts ...ActivityOptions) *activitypub.Activity {
	options := ActivityOptions{
		Type:   "Like",
		Actor:  actor,
		Object: object,
	}

	if len(opts) > 0 {
		if err := common.ValidateRequiredParam("id", opts[0].ID); err == nil {
			options.ID = opts[0].ID
		}
		if opts[0].Published != nil {
			options.Published = opts[0].Published
		}
	}

	return f.CreateActivity(options)
}

// CreateAnnounce creates an Announce (boost/reblog) activity
func (f *ActivityFactory) CreateAnnounce(actor string, object interface{}, opts ...ActivityOptions) *activitypub.Activity {
	options := ActivityOptions{
		Type:   "Announce",
		Actor:  actor,
		Object: object,
	}

	if len(opts) > 0 {
		if err := common.ValidateRequiredParam("id", opts[0].ID); err == nil {
			options.ID = opts[0].ID
		}
		if opts[0].Published != nil {
			options.Published = opts[0].Published
		}
	}

	return f.CreateActivity(options)
}

// CreateDelete creates a Delete activity
func (f *ActivityFactory) CreateDelete(actor string, object interface{}, opts ...ActivityOptions) *activitypub.Activity {
	options := ActivityOptions{
		Type:   "Delete",
		Actor:  actor,
		Object: object,
	}

	if len(opts) > 0 {
		if err := common.ValidateRequiredParam("id", opts[0].ID); err == nil {
			options.ID = opts[0].ID
		}
		if opts[0].Published != nil {
			options.Published = opts[0].Published
		}
	}

	return f.CreateActivity(options)
}

// CreateUpdate creates an Update activity
func (f *ActivityFactory) CreateUpdate(actor string, object interface{}, opts ...ActivityOptions) *activitypub.Activity {
	options := ActivityOptions{
		Type:   "Update",
		Actor:  actor,
		Object: object,
	}

	if len(opts) > 0 {
		if err := common.ValidateRequiredParam("id", opts[0].ID); err == nil {
			options.ID = opts[0].ID
		}
		if opts[0].Published != nil {
			options.Published = opts[0].Published
		}
	}

	return f.CreateActivity(options)
}

// CreateThread creates a thread of activities (original post + replies)
func (f *ActivityFactory) CreateThread(originalActor string, originalContent string, replies []ThreadReply) []*activitypub.Activity {
	activities := make([]*activitypub.Activity, 0, len(replies)+1)

	// Create original post
	originalNote := f.CreateNote(originalContent, f.generateActorID(originalActor))
	originalActivity := f.CreateActivity(ActivityOptions{
		Type:   "Create",
		Actor:  f.generateActorID(originalActor),
		Object: originalNote,
	})
	activities = append(activities, originalActivity)

	// Create replies
	for _, reply := range replies {
		replyNote := f.CreateNote(reply.Content, f.generateActorID(reply.Actor), NoteOptions{
			InReplyTo: originalNote.ID,
		})
		replyActivity := f.CreateActivity(ActivityOptions{
			Type:   "Create",
			Actor:  f.generateActorID(reply.Actor),
			Object: replyNote,
		})
		activities = append(activities, replyActivity)
	}

	return activities
}

// ThreadReply represents a reply in a thread
type ThreadReply struct {
	Actor   string
	Content string
}

// CreateConversation creates a back-and-forth conversation
func (f *ActivityFactory) CreateConversation(participants []string, messages []string) []*activitypub.Activity {
	activities := make([]*activitypub.Activity, 0, len(messages))
	var lastNoteID string

	for i, message := range messages {
		actor := participants[i%len(participants)]
		noteOpts := NoteOptions{}

		if err := common.ValidateRequiredParam("lastNoteID", lastNoteID); err == nil {
			noteOpts.InReplyTo = lastNoteID
		}

		note := f.CreateNote(message, f.generateActorID(actor), noteOpts)
		activity := f.CreateActivity(ActivityOptions{
			Type:   "Create",
			Actor:  f.generateActorID(actor),
			Object: note,
		})

		activities = append(activities, activity)
		lastNoteID = note.ID
	}

	return activities
}

// CreateBatch creates multiple activities of the same type
func (f *ActivityFactory) CreateBatch(activityType string, count int, actor string) []*activitypub.Activity {
	activities := make([]*activitypub.Activity, count)

	for i := 0; i < count; i++ {
		switch activityType {
		case "Create":
			note := f.CreateNote(fmt.Sprintf("Test post #%d", i+1), f.generateActorID(actor))
			activities[i] = f.CreateActivity(ActivityOptions{
				Type:   "Create",
				Actor:  f.generateActorID(actor),
				Object: note,
			})
		case "Follow":
			target := fmt.Sprintf("user%d", i+1)
			activities[i] = f.CreateFollow(f.generateActorID(actor), f.generateActorID(target))
		case "Like":
			objectID := f.generateObjectID()
			activities[i] = f.CreateLike(f.generateActorID(actor), objectID)
		case "Announce":
			objectID := f.generateObjectID()
			activities[i] = f.CreateAnnounce(f.generateActorID(actor), objectID)
		}
	}

	return activities
}

// Helper methods for generating IDs
func (f *ActivityFactory) generateActivityID() string {
	id := fmt.Sprintf("https://%s/activities/%d_%d", f.domain, time.Now().UnixNano(), f.sequence)
	return id
}

func (f *ActivityFactory) generateObjectID() string {
	id := fmt.Sprintf("https://%s/objects/%d_%d", f.domain, time.Now().UnixNano(), f.sequence)
	return id
}

func (f *ActivityFactory) generateActorID(username string) string {
	return fmt.Sprintf("https://%s/users/%s", f.domain, username)
}

// Reset resets the factory sequence counter
func (f *ActivityFactory) Reset() {
	f.sequence = 1
	f.baseTime = time.Now().Truncate(time.Hour)
}

// SetBaseTime sets the base time for generated activities
func (f *ActivityFactory) SetBaseTime(t time.Time) {
	f.baseTime = t
}

// GetSequence returns the current sequence number
func (f *ActivityFactory) GetSequence() int {
	return f.sequence
}

// CreateWithTags creates a note with hashtags
func (f *ActivityFactory) CreateWithTags(content string, actor string, hashtags []string) *activitypub.Activity {
	tags := make([]activitypub.Tag, len(hashtags))
	for i, tag := range hashtags {
		tags[i] = activitypub.Tag{
			Type: "Hashtag",
			Name: "#" + tag,
			Href: fmt.Sprintf("https://%s/tags/%s", f.domain, tag),
		}
	}

	note := f.CreateNote(content, f.generateActorID(actor), NoteOptions{
		Tags: tags,
	})

	return f.CreateActivity(ActivityOptions{
		Type:   "Create",
		Actor:  f.generateActorID(actor),
		Object: note,
	})
}

// CreateWithMention creates a note with mentions
func (f *ActivityFactory) CreateWithMention(content string, actor string, mentions []string) *activitypub.Activity {
	tags := make([]activitypub.Tag, len(mentions))
	for i, mention := range mentions {
		tags[i] = activitypub.Tag{
			Type: "Mention",
			Name: "@" + mention,
			Href: f.generateActorID(mention),
		}
	}

	note := f.CreateNote(content, f.generateActorID(actor), NoteOptions{
		Tags: tags,
	})

	return f.CreateActivity(ActivityOptions{
		Type:   "Create",
		Actor:  f.generateActorID(actor),
		Object: note,
	})
}

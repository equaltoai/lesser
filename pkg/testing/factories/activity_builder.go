package factories

import (
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
)

const (
	// Activity type constants
	activityTypeCreate = "Create"
)

// ActivityBuilder builds ActivityPub activities using the builder pattern
type ActivityBuilder struct {
	*BaseBuilder
	activity *activitypub.Activity
}

// NewActivityBuilder creates a new activity builder
func NewActivityBuilder(domain string) *ActivityBuilder {
	return &ActivityBuilder{
		BaseBuilder: NewBaseBuilder(domain),
		activity:    &activitypub.Activity{},
	}
}

// Reset resets the builder to create a new activity
func (b *ActivityBuilder) Reset() *ActivityBuilder {
	b.activity = &activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			Context: []string{"https://www.w3.org/ns/activitystreams"},
		},
	}
	return b
}

// WithType sets the activity type
func (b *ActivityBuilder) WithType(activityType string) *ActivityBuilder {
	b.activity.Type = activityType
	return b
}

// WithActor sets the actor
func (b *ActivityBuilder) WithActor(actor string) *ActivityBuilder {
	b.activity.Actor = actor
	return b
}

// WithObject sets the object
func (b *ActivityBuilder) WithObject(object interface{}) *ActivityBuilder {
	b.activity.Object = object
	return b
}

// WithTarget sets the target
func (b *ActivityBuilder) WithTarget(target string) *ActivityBuilder {
	b.activity.Target = target
	return b
}

// WithID sets a custom ID
func (b *ActivityBuilder) WithID(id string) *ActivityBuilder {
	b.activity.ID = id
	return b
}

// WithPublished sets the published timestamp
func (b *ActivityBuilder) WithPublished(published time.Time) *ActivityBuilder {
	b.activity.Published = &published
	return b
}

// WithTo adds recipients to the "to" field
func (b *ActivityBuilder) WithTo(recipients ...string) *ActivityBuilder {
	b.activity.To = append(b.activity.To, recipients...)
	return b
}

// WithCC adds recipients to the "cc" field
func (b *ActivityBuilder) WithCC(recipients ...string) *ActivityBuilder {
	b.activity.CC = append(b.activity.CC, recipients...)
	return b
}

// Build creates the activity with defaults for any unset values
func (b *ActivityBuilder) Build() *activitypub.Activity {
	// Apply defaults
	if err := common.ValidateRequiredParam("Type", b.activity.Type); err != nil {
		b.activity.Type = activityTypeCreate
	}

	if err := common.ValidateRequiredParam("ID", b.activity.ID); err != nil {
		b.activity.ID = b.GenerateID("activities")
	}

	if err := common.ValidateRequiredParam("Actor", b.activity.Actor); err != nil {
		b.activity.Actor = fmt.Sprintf("https://%s/users/testuser", b.domain)
	}

	if b.activity.Published == nil {
		timestamp := b.GenerateTimestamp()
		b.activity.Published = &timestamp
	}

	if b.activity.Context == nil {
		b.activity.Context = []string{"https://www.w3.org/ns/activitystreams"}
	}

	// Create a copy to return
	result := *b.activity

	// Reset for next build
	b.Reset()

	return &result
}

// BuildFollow creates a Follow activity with fluent interface
func (b *ActivityBuilder) BuildFollow(follower, target string) *activitypub.Activity {
	return b.Reset().
		WithType("Follow").
		WithActor(follower).
		WithObject(target).
		Build()
}

// BuildLike creates a Like activity
func (b *ActivityBuilder) BuildLike(actor, object string) *activitypub.Activity {
	return b.Reset().
		WithType("Like").
		WithActor(actor).
		WithObject(object).
		Build()
}

// BuildAnnounce creates an Announce (reblog) activity
func (b *ActivityBuilder) BuildAnnounce(actor, object string) *activitypub.Activity {
	return b.Reset().
		WithType("Announce").
		WithActor(actor).
		WithObject(object).
		WithTo("https://www.w3.org/ns/activitystreams#Public").
		Build()
}

// BuildCreate creates a Create activity with a Note object
func (b *ActivityBuilder) BuildCreate(actor string, note *activitypub.Note) *activitypub.Activity {
	return b.Reset().
		WithType(activityTypeCreate).
		WithActor(actor).
		WithObject(note).
		WithTo(note.To...).
		WithCC(note.CC...).
		Build()
}

// BuildUndo creates an Undo activity
func (b *ActivityBuilder) BuildUndo(actor string, activity *activitypub.Activity) *activitypub.Activity {
	return b.Reset().
		WithType("Undo").
		WithActor(actor).
		WithObject(activity).
		Build()
}

// BuildDelete creates a Delete activity
func (b *ActivityBuilder) BuildDelete(actor, object string) *activitypub.Activity {
	return b.Reset().
		WithType("Delete").
		WithActor(actor).
		WithObject(object).
		Build()
}

// BuildUpdate creates an Update activity
func (b *ActivityBuilder) BuildUpdate(actor string, object interface{}) *activitypub.Activity {
	return b.Reset().
		WithType("Update").
		WithActor(actor).
		WithObject(object).
		Build()
}

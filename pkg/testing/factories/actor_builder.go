package factories

import (
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
)

// ActorBuilder builds ActivityPub Actor objects using the builder pattern
type ActorBuilder struct {
	*BaseBuilder
	actor *activitypub.Actor
}

// NewActorBuilder creates a new actor builder
func NewActorBuilder(domain string) *ActorBuilder {
	return &ActorBuilder{
		BaseBuilder: NewBaseBuilder(domain),
		actor:       &activitypub.Actor{},
	}
}

// Reset resets the builder to create a new actor
func (b *ActorBuilder) Reset() *ActorBuilder {
	b.actor = &activitypub.Actor{
		BaseObject: activitypub.BaseObject{
			Context: []string{"https://www.w3.org/ns/activitystreams"},
			Type:    "Person",
		},
	}
	return b
}

// WithUsername sets the actor's username
func (b *ActorBuilder) WithUsername(username string) *ActorBuilder {
	b.actor.PreferredUsername = username
	if err := common.ValidateRequiredParam("b.actor.ID", b.actor.ID); err != nil {
		b.actor.ID = fmt.Sprintf("https://%s/users/%s", b.domain, username)
	}
	if err := common.ValidateRequiredParam("b.actor.Inbox", b.actor.Inbox); err != nil {
		b.actor.Inbox = fmt.Sprintf("https://%s/users/%s/inbox", b.domain, username)
	}
	if err := common.ValidateRequiredParam("b.actor.Outbox", b.actor.Outbox); err != nil {
		b.actor.Outbox = fmt.Sprintf("https://%s/users/%s/outbox", b.domain, username)
	}
	if err := common.ValidateRequiredParam("b.actor.URL", b.actor.URL); err != nil {
		b.actor.URL = fmt.Sprintf("https://%s/@%s", b.domain, username)
	}
	return b
}

// WithDisplayName sets the actor's display name
func (b *ActorBuilder) WithDisplayName(name string) *ActorBuilder {
	b.actor.Name = name
	return b
}

// WithSummary sets the actor's bio/summary
func (b *ActorBuilder) WithSummary(summary string) *ActorBuilder {
	b.actor.Summary = summary
	return b
}

// WithID sets a custom ID
func (b *ActorBuilder) WithID(id string) *ActorBuilder {
	b.actor.ID = id
	return b
}

// WithInbox sets the inbox URL
func (b *ActorBuilder) WithInbox(inbox string) *ActorBuilder {
	b.actor.Inbox = inbox
	return b
}

// WithOutbox sets the outbox URL
func (b *ActorBuilder) WithOutbox(outbox string) *ActorBuilder {
	b.actor.Outbox = outbox
	return b
}

// WithFollowers sets the followers URL
func (b *ActorBuilder) WithFollowers(followers string) *ActorBuilder {
	b.actor.Followers = followers
	return b
}

// WithFollowing sets the following URL
func (b *ActorBuilder) WithFollowing(following string) *ActorBuilder {
	b.actor.Following = following
	return b
}

// WithPublicKey sets the actor's public key
func (b *ActorBuilder) WithPublicKey(keyPEM string) *ActorBuilder {
	if b.actor.PublicKey == nil {
		b.actor.PublicKey = &activitypub.PublicKey{}
	}
	b.actor.PublicKey.ID = fmt.Sprintf("%s#main-key", b.actor.ID)
	b.actor.PublicKey.Owner = b.actor.ID
	b.actor.PublicKey.PublicKeyPem = keyPEM
	return b
}

// WithIcon sets the actor's avatar
func (b *ActorBuilder) WithIcon(url, mediaType string) *ActorBuilder {
	b.actor.Icon = &activitypub.Image{
		BaseObject: activitypub.BaseObject{
			Type: "Image",
		},
		MediaType: mediaType,
		URL:       url,
	}
	return b
}

// WithImage sets the actor's header image
func (b *ActorBuilder) WithImage(url, mediaType string) *ActorBuilder {
	b.actor.Image = &activitypub.Image{
		BaseObject: activitypub.BaseObject{
			Type: "Image",
		},
		MediaType: mediaType,
		URL:       url,
	}
	return b
}

// AsBot marks the actor as a bot
func (b *ActorBuilder) AsBot() *ActorBuilder {
	b.actor.Type = "Service"
	return b
}

// AsGroup marks the actor as a group
func (b *ActorBuilder) AsGroup() *ActorBuilder {
	b.actor.Type = "Group"
	return b
}

// AsApplication marks the actor as an application
func (b *ActorBuilder) AsApplication() *ActorBuilder {
	b.actor.Type = "Application"
	return b
}

// WithManuallyApprovesFollowers sets whether the actor manually approves followers
func (b *ActorBuilder) WithManuallyApprovesFollowers(approves bool) *ActorBuilder {
	b.actor.ManuallyApprovesFollowers = approves
	return b
}

// WithDiscoverable sets whether the actor is discoverable
func (b *ActorBuilder) WithDiscoverable(discoverable bool) *ActorBuilder {
	b.actor.Discoverable = discoverable
	return b
}

// WithPublished sets when the actor was created
func (b *ActorBuilder) WithPublished(published time.Time) *ActorBuilder {
	b.actor.Published = &published
	return b
}

// WithEndpoints sets the actor's endpoints
func (b *ActorBuilder) WithEndpoints(sharedInbox string) *ActorBuilder {
	b.actor.Endpoints = &activitypub.Endpoints{
		SharedInbox: sharedInbox,
	}
	return b
}

// WithAttachment adds a profile field attachment
func (b *ActorBuilder) WithAttachment(name, value string) *ActorBuilder {
	attachment := activitypub.Attachment{
		Type:  "PropertyValue",
		Name:  name,
		Value: value,
	}
	b.actor.Attachment = append(b.actor.Attachment, attachment)
	return b
}

// Build creates the actor with defaults for any unset values
func (b *ActorBuilder) Build() *activitypub.Actor {
	// Apply defaults
	if err := common.ValidateRequiredParam("b.actor.ID", b.actor.ID); err != nil {
		b.actor.ID = b.GenerateID("users")
	}
	
	if err := common.ValidateRequiredParam("b.actor.PreferredUsername", b.actor.PreferredUsername); err != nil {
		b.actor.PreferredUsername = fmt.Sprintf("user%d", b.sequence)
	}
	
	if err := common.ValidateRequiredParam("b.actor.Name", b.actor.Name); err != nil {
		b.actor.Name = fmt.Sprintf("Test User %d", b.sequence)
	}
	
	if err := common.ValidateRequiredParam("b.actor.Inbox", b.actor.Inbox); err != nil {
		b.actor.Inbox = fmt.Sprintf("%s/inbox", b.actor.ID)
	}
	
	if err := common.ValidateRequiredParam("b.actor.Outbox", b.actor.Outbox); err != nil {
		b.actor.Outbox = fmt.Sprintf("%s/outbox", b.actor.ID)
	}
	
	if err := common.ValidateRequiredParam("b.actor.Followers", b.actor.Followers); err != nil {
		b.actor.Followers = fmt.Sprintf("%s/followers", b.actor.ID)
	}
	
	if err := common.ValidateRequiredParam("b.actor.Following", b.actor.Following); err != nil {
		b.actor.Following = fmt.Sprintf("%s/following", b.actor.ID)
	}
	
	if err := common.ValidateRequiredParam("b.actor.URL", b.actor.URL); err != nil {
		b.actor.URL = fmt.Sprintf("https://%s/@%s", b.domain, b.actor.PreferredUsername)
	}
	
	if b.actor.Published == nil {
		timestamp := b.GenerateTimestamp()
		b.actor.Published = &timestamp
	}
	
	if b.actor.PublicKey == nil {
		// Generate a dummy public key
		b.WithPublicKey("-----BEGIN PUBLIC KEY-----\nMIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEA...\n-----END PUBLIC KEY-----")
	}
	
	// Create a copy to return
	result := *b.actor
	
	// Reset for next build
	b.Reset()
	
	return &result
}

// BuildLocal creates a local actor
func (b *ActorBuilder) BuildLocal(username string) *activitypub.Actor {
	return b.Reset().
		WithUsername(username).
		WithDisplayName(fmt.Sprintf("%s User", username)).
		WithSummary(fmt.Sprintf("This is %s's bio", username)).
		Build()
}

// BuildRemote creates a remote actor
func (b *ActorBuilder) BuildRemote(username, remoteDomain string) *activitypub.Actor {
	remoteID := fmt.Sprintf("https://%s/users/%s", remoteDomain, username)
	return b.Reset().
		WithID(remoteID).
		WithUsername(fmt.Sprintf("%s@%s", username, remoteDomain)).
		WithDisplayName(fmt.Sprintf("%s from %s", username, remoteDomain)).
		WithInbox(fmt.Sprintf("%s/inbox", remoteID)).
		WithOutbox(fmt.Sprintf("%s/outbox", remoteID)).
		Build()
}

// BuildBot creates a bot actor
func (b *ActorBuilder) BuildBot(username string) *activitypub.Actor {
	return b.Reset().
		WithUsername(username).
		WithDisplayName(fmt.Sprintf("%s Bot", username)).
		WithSummary("I am a bot account").
		AsBot().
		Build()
}

// BuildLocked creates an actor with locked account (manual follower approval)
func (b *ActorBuilder) BuildLocked(username string) *activitypub.Actor {
	return b.Reset().
		WithUsername(username).
		WithDisplayName(fmt.Sprintf("%s (Locked)", username)).
		WithManuallyApprovesFollowers(true).
		Build()
}
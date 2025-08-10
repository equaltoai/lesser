// Package factories provides actor factory for test data generation
package factories

import (
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
)

// ActorFactory creates actors for testing
type ActorFactory struct {
	domain   string
	sequence int
}

// NewActorFactory creates a new actor factory
func NewActorFactory(domain string) *ActorFactory {
	return &ActorFactory{
		domain:   domain,
		sequence: 1,
	}
}

// ActorOptions configures actor creation
type ActorOptions struct {
	Username    string
	DisplayName string
	Summary     string
	PublicKey   string
	PrivateKey  string
	Locked      bool
	Bot         bool
	Discoverable bool
	Avatar      *activitypub.Image
	Header      *activitypub.Image
	Fields      []activitypub.Attachment
}

// CreateActor creates a basic actor with default values
func (f *ActorFactory) CreateActor(opts ActorOptions) *activitypub.Actor {
	username := opts.Username
	if username == "" {
		username = fmt.Sprintf("testuser%d", f.sequence)
	}

	displayName := opts.DisplayName
	if displayName == "" {
		displayName = fmt.Sprintf("Test User %d", f.sequence)
	}

	publicKey := opts.PublicKey
	if publicKey == "" {
		publicKey = f.generateTestPublicKey()
	}

	actor := &activitypub.Actor{
		BaseObject: activitypub.BaseObject{
			Context: []string{
				"https://www.w3.org/ns/activitystreams",
				"https://w3id.org/security/v1",
			},
			ID:   fmt.Sprintf("https://%s/users/%s", f.domain, username),
			Type: "Person",
		},
		PreferredUsername: username,
		Name:              displayName,
		Summary:           opts.Summary,
		URL:               fmt.Sprintf("https://%s/@%s", f.domain, username),
		Inbox:             fmt.Sprintf("https://%s/users/%s/inbox", f.domain, username),
		Outbox:            fmt.Sprintf("https://%s/users/%s/outbox", f.domain, username),
		Following:         fmt.Sprintf("https://%s/users/%s/following", f.domain, username),
		Followers:         fmt.Sprintf("https://%s/users/%s/followers", f.domain, username),
		Liked:             fmt.Sprintf("https://%s/users/%s/liked", f.domain, username),
		PublicKey: &activitypub.PublicKey{
			ID:           fmt.Sprintf("https://%s/users/%s#main-key", f.domain, username),
			Owner:        fmt.Sprintf("https://%s/users/%s", f.domain, username),
			PublicKeyPem: publicKey,
		},
		ManuallyApprovesFollowers: opts.Locked,
		Discoverable:              opts.Discoverable,
	}

	if opts.Bot {
		actor.Type = "Service"
	}

	if opts.Avatar != nil {
		actor.Icon = opts.Avatar
	}

	if opts.Header != nil {
		actor.Image = opts.Header
	}

	if len(opts.Fields) > 0 {
		actor.Attachment = opts.Fields
	}

	f.sequence++
	return actor
}

// CreateUserData creates user data for testing (returns a map to avoid type issues)
func (f *ActorFactory) CreateUserData(opts ActorOptions) map[string]interface{} {
	username := opts.Username
	if username == "" {
		username = fmt.Sprintf("testuser%d", f.sequence)
	}

	displayName := opts.DisplayName
	if displayName == "" {
		displayName = fmt.Sprintf("Test User %d", f.sequence)
	}

	userData := map[string]interface{}{
		"id":           fmt.Sprintf("user_%d", f.sequence),
		"username":     username,
		"email":        fmt.Sprintf("%s@test.example.com", username),
		"display_name": displayName,
		"created_at":   time.Now(),
		"updated_at":   time.Now(),
	}

	f.sequence++
	return userData
}

// CreateActorWithFollowers creates an actor with a specified number of followers
func (f *ActorFactory) CreateActorWithFollowers(username string, followerCount int) (*activitypub.Actor, []*activitypub.Actor) {
	actor := f.CreateActor(ActorOptions{Username: username})
	
	followers := make([]*activitypub.Actor, followerCount)
	for i := 0; i < followerCount; i++ {
		follower := f.CreateActor(ActorOptions{
			Username: fmt.Sprintf("%s_follower_%d", username, i+1),
		})
		followers[i] = follower
	}

	return actor, followers
}

// CreateActorWithFollowing creates an actor following a specified number of other actors
func (f *ActorFactory) CreateActorWithFollowing(username string, followingCount int) (*activitypub.Actor, []*activitypub.Actor) {
	actor := f.CreateActor(ActorOptions{Username: username})
	
	following := make([]*activitypub.Actor, followingCount)
	for i := 0; i < followingCount; i++ {
		followed := f.CreateActor(ActorOptions{
			Username: fmt.Sprintf("%s_following_%d", username, i+1),
		})
		following[i] = followed
	}

	return actor, following
}

// CreateBotActor creates a bot actor
func (f *ActorFactory) CreateBotActor(username string) *activitypub.Actor {
	return f.CreateActor(ActorOptions{
		Username:    username,
		DisplayName: fmt.Sprintf("Bot %s", username),
		Summary:     "I am a test bot account",
		Bot:         true,
		Locked:      false,
		Discoverable: true,
	})
}

// CreateLockedActor creates a locked (private) actor
func (f *ActorFactory) CreateLockedActor(username string) *activitypub.Actor {
	return f.CreateActor(ActorOptions{
		Username:    username,
		DisplayName: fmt.Sprintf("Private %s", username),
		Summary:     "This is a private account",
		Bot:         false,
		Locked:      true,
		Discoverable: false,
	})
}

// CreateActorWithProfile creates an actor with complete profile information
func (f *ActorFactory) CreateActorWithProfile(username string) *activitypub.Actor {
	avatar := &activitypub.Image{
		BaseObject: activitypub.BaseObject{
			Type: "Image",
		},
		MediaType: "image/png",
		URL:       fmt.Sprintf("https://%s/system/accounts/avatars/000/000/001/original/%s.png", f.domain, username),
	}

	header := &activitypub.Image{
		BaseObject: activitypub.BaseObject{
			Type: "Image",
		},
		MediaType: "image/png",
		URL:       fmt.Sprintf("https://%s/system/accounts/headers/000/000/001/original/%s.png", f.domain, username),
	}

	fields := []activitypub.Attachment{
		{
			Type:  "PropertyValue",
			Name:  "Website",
			Value: fmt.Sprintf("https://%s.example.com", username),
		},
		{
			Type:  "PropertyValue",
			Name:  "Location",  
			Value: "Test City, TC",
		},
	}

	return f.CreateActor(ActorOptions{
		Username:    username,
		DisplayName: fmt.Sprintf("Test User %s", username),
		Summary:     fmt.Sprintf("This is the bio for test user %s. I like testing things!", username),
		Avatar:      avatar,
		Header:      header,
		Fields:      fields,
		Discoverable: true,
	})
}

// CreateRemoteActor creates an actor from a remote instance
func (f *ActorFactory) CreateRemoteActor(username, remoteDomain string) *activitypub.Actor {
	remoteFactory := NewActorFactory(remoteDomain)
	return remoteFactory.CreateActor(ActorOptions{
		Username: username,
		DisplayName: fmt.Sprintf("Remote User %s", username),
		Summary:     fmt.Sprintf("I'm a user from %s", remoteDomain),
		Discoverable: true,
	})
}

// CreateActorBatch creates multiple actors for testing
func (f *ActorFactory) CreateActorBatch(count int, prefix string) []*activitypub.Actor {
	actors := make([]*activitypub.Actor, count)
	
	for i := 0; i < count; i++ {
		actors[i] = f.CreateActor(ActorOptions{
			Username:    fmt.Sprintf("%s%d", prefix, i+1),
			DisplayName: fmt.Sprintf("Test User %s%d", prefix, i+1),
			Discoverable: true,
		})
	}

	return actors
}

// CreateUserDataBatch creates multiple users for testing (returns maps to avoid type issues)
func (f *ActorFactory) CreateUserDataBatch(count int, prefix string) []map[string]interface{} {
	users := make([]map[string]interface{}, count)
	
	for i := 0; i < count; i++ {
		users[i] = f.CreateUserData(ActorOptions{
			Username:    fmt.Sprintf("%s%d", prefix, i+1),
			DisplayName: fmt.Sprintf("Test User %s%d", prefix, i+1),
			Discoverable: true,
		})
	}

	return users
}

// CreateActorWithCustomFields creates an actor with custom profile fields
func (f *ActorFactory) CreateActorWithCustomFields(username string, fields map[string]string) *activitypub.Actor {
	propertyValues := make([]activitypub.Attachment, 0, len(fields))
	
	for name, value := range fields {
		propertyValues = append(propertyValues, activitypub.Attachment{
			Type:  "PropertyValue",
			Name:  name,
			Value: value,
		})
	}

	return f.CreateActor(ActorOptions{
		Username:    username,
		DisplayName: fmt.Sprintf("Test User %s", username),
		Fields:      propertyValues,
		Discoverable: true,
	})
}

// Helper methods for generating test keys
func (f *ActorFactory) generateTestPublicKey() string {
	return `-----BEGIN PUBLIC KEY-----
MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEA1234567890abcdefghij
klmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ1234567890abcdefghijkl
mnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ1234567890abcdefghijklmn
opqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ1234567890abcdefghijklmnop
qrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ1234567890abcdefghijklmnopqr
stuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ1234567890abcdefghijklmnopqrst
uvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ1234567890abcdefghijklmnopqrstuv
wxyzABCDEFGHIJKLMNOPQRSTUVWXYZ
-----END PUBLIC KEY-----`
}

// Reset resets the factory sequence counter
func (f *ActorFactory) Reset() {
	f.sequence = 1
}

// GetSequence returns the current sequence number
func (f *ActorFactory) GetSequence() int {
	return f.sequence
}

// SetDomain changes the domain used for actor IDs
func (f *ActorFactory) SetDomain(domain string) {
	f.domain = domain
}
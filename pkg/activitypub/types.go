package activitypub

import (
	"encoding/json"
	"time"
)

// Context represents the JSON-LD context for ActivityStreams
var Context = []any{
	"https://www.w3.org/ns/activitystreams",
	map[string]any{
		"manuallyApprovesFollowers": "as:manuallyApprovesFollowers",
		"sensitive":                 "as:sensitive",
		"Hashtag":                   "as:Hashtag",
		"quoteUrl":                  "as:quoteUrl",
		"toot":                      "http://joinmastodon.org/ns#",
		"Emoji":                     "toot:Emoji",
		"featured":                  "toot:featured",
		"discoverable":              "toot:discoverable",
		"schema":                    "http://schema.org#",
		"PropertyValue":             "schema:PropertyValue",
		"value":                     "schema:value",
	},
}

// BaseObject represents the base ActivityStreams object
type BaseObject struct {
	Context   any        `json:"@context,omitempty"`
	ID        string     `json:"id"`
	Type      string     `json:"type"`
	Published *time.Time `json:"published,omitempty"`
	Updated   *time.Time `json:"updated,omitempty"`
	To        []string   `json:"to,omitempty"`
	CC        []string   `json:"cc,omitempty"`
	BTo       []string   `json:"bto,omitempty"`
	BCC       []string   `json:"bcc,omitempty"`
	InReplyTo string     `json:"inReplyTo,omitempty"`
	Summary   string     `json:"summary,omitempty"`
	Sensitive bool       `json:"sensitive,omitempty"`
}

// Actor represents an ActivityPub actor (Person, Service, etc.)
type Actor struct {
	BaseObject
	PreferredUsername string     `json:"preferredUsername"`
	Name              string     `json:"name,omitempty"`
	Summary           string     `json:"summary,omitempty"`
	URL               string     `json:"url,omitempty"`
	Icon              *Image     `json:"icon,omitempty"`
	Image             *Image     `json:"image,omitempty"`
	Inbox             string     `json:"inbox"`
	Outbox            string     `json:"outbox"`
	Following         string     `json:"following,omitempty"`
	Followers         string     `json:"followers,omitempty"`
	Liked             string     `json:"liked,omitempty"`
	PublicKey         *PublicKey `json:"publicKey,omitempty"`
	Endpoints         *Endpoints `json:"endpoints,omitempty"`

	// Mastodon-specific fields
	ManuallyApprovesFollowers bool         `json:"manuallyApprovesFollowers,omitempty"`
	Discoverable              bool         `json:"discoverable,omitempty"`
	Featured                  string       `json:"featured,omitempty"`
	LastStatusAt              *time.Time   `json:"lastStatusAt,omitempty"`
	CreatedAt                 *time.Time   `json:"createdAt,omitempty"`
	Attachment                []Attachment `json:"attachment,omitempty"`
}

// PublicKey represents an actor's public key for HTTP signatures
type PublicKey struct {
	ID           string `json:"id"`
	Owner        string `json:"owner"`
	PublicKeyPem string `json:"publicKeyPem"`
}

// Endpoints represents an actor's endpoints
type Endpoints struct {
	SharedInbox string `json:"sharedInbox,omitempty"`
}

// Activity represents an ActivityPub activity
type Activity struct {
	BaseObject
	Actor      string `json:"actor"`
	Object     any    `json:"object"`
	Target     string `json:"target,omitempty"`     // For Add/Remove activities
	Origin     string `json:"origin,omitempty"`     // For Move activities
	Instrument string `json:"instrument,omitempty"` // For various activities
}

// Note represents a basic text post
type Note struct {
	BaseObject
	Content        string       `json:"content"`
	AttributedTo   string       `json:"attributedTo"`
	Attachment     []Attachment `json:"attachment,omitempty"`
	Tag            []Tag        `json:"tag,omitempty"`
	ConversationID string       `json:"conversationId,omitempty"` // For tracking conversation threads
	Visibility     string       `json:"_:visibility,omitempty"`   // Lesser extension for preserving visibility
}

// QuoteNote represents a note that quotes another note
type QuoteNote struct {
	Note
	QuoteURL           string        `json:"quoteUrl"`
	Quoteable          bool          `json:"_:quoteable"`
	QuoteNotifications bool          `json:"_:quoteNotifications"`
	QuoteContext       *QuoteContext `json:"_:quoteContext,omitempty"`
}

// QuoteContext provides metadata about a quoted note
type QuoteContext struct {
	OriginalAuthor  string `json:"originalAuthor"`
	QuoteCount      int    `json:"quoteCount"`
	AllowWithdrawal bool   `json:"allowWithdrawal"`
}

// Article represents long-form content
type Article struct {
	Note
	Name string `json:"name"` // Title
}

// Image represents an image object
type Image struct {
	BaseObject
	URL       string `json:"url"`
	MediaType string `json:"mediaType,omitempty"`
	Width     int    `json:"width,omitempty"`
	Height    int    `json:"height,omitempty"`
}

// Attachment represents media attachments
type Attachment struct {
	Type      string `json:"type"`
	MediaType string `json:"mediaType"`
	URL       string `json:"url"`
	Name      string `json:"name,omitempty"`
	Value     string `json:"value,omitempty"` // For profile metadata
	Width     int    `json:"width,omitempty"`
	Height    int    `json:"height,omitempty"`
}

// Tag represents hashtags or mentions
type Tag struct {
	Type string `json:"type"`
	Href string `json:"href,omitempty"`
	Name string `json:"name"`
}

// Collection represents an ActivityStreams collection
type Collection struct {
	BaseObject
	TotalItems   int    `json:"totalItems"`
	Current      string `json:"current,omitempty"`
	First        string `json:"first,omitempty"`
	Last         string `json:"last,omitempty"`
	Items        any    `json:"items,omitempty"`
	OrderedItems any    `json:"orderedItems,omitempty"`
}

// OrderedCollection represents an ordered ActivityStreams collection
type OrderedCollection struct {
	Collection
}

// CollectionPage represents a page of a collection
type CollectionPage struct {
	Collection
	PartOf string `json:"partOf"`
	Next   string `json:"next,omitempty"`
	Prev   string `json:"prev,omitempty"`
}

// OrderedCollectionPage represents a page of an ordered collection
type OrderedCollectionPage struct {
	CollectionPage
}

// Follow represents a follow activity
type Follow struct {
	Activity
}

// Accept represents an accept activity
type Accept struct {
	Activity
}

// Reject represents a reject activity
type Reject struct {
	Activity
}

// Create represents a create activity
type Create struct {
	Activity
}

// Update represents an update activity
type Update struct {
	Activity
}

// Delete represents a delete activity
type Delete struct {
	Activity
}

// Like represents a like activity
type Like struct {
	Activity
}

// Announce represents an announce (boost/share) activity
type Announce struct {
	Activity
}

// Undo represents an undo activity
type Undo struct {
	Activity
}

// Flag represents a flag activity for content moderation
type Flag struct {
	Activity
}

// Move represents a move activity for account migration
type Move struct {
	Activity
	Target string `json:"target"` // The new account location
}

// Add represents an add activity for adding items to collections
type Add struct {
	Activity
	Target string `json:"target"` // The collection to add to
}

// Remove represents a remove activity for removing items from collections
type Remove struct {
	Activity
	Target string `json:"target"` // The collection to remove from
}

// WebFingerResource represents a WebFinger response
type WebFingerResource struct {
	Subject string          `json:"subject"`
	Aliases []string        `json:"aliases,omitempty"`
	Links   []WebFingerLink `json:"links"`
}

// WebFingerLink represents a link in a WebFinger response
type WebFingerLink struct {
	Rel      string `json:"rel"`
	Type     string `json:"type,omitempty"`
	Href     string `json:"href,omitempty"`
	Template string `json:"template,omitempty"`
}

// Helper functions

// NewActor creates a new actor with required fields
func NewActor(actorType, id, username string) *Actor {
	return &Actor{
		BaseObject: BaseObject{
			Context: Context,
			ID:      id,
			Type:    actorType,
		},
		PreferredUsername: username,
		Inbox:             id + "/inbox",
		Outbox:            id + "/outbox",
		Following:         id + "/following",
		Followers:         id + "/followers",
		Liked:             id + "/liked",
	}
}

// NewActivity creates a new activity
func NewActivity(activityType, id, actorID string, object any) *Activity {
	now := time.Now()
	return &Activity{
		BaseObject: BaseObject{
			Context:   Context,
			ID:        id,
			Type:      activityType,
			Published: &now,
		},
		Actor:  actorID,
		Object: object,
	}
}

// MarshalJSON provides custom JSON marshaling to handle the context
func (a *Actor) MarshalJSON() ([]byte, error) {
	type Alias Actor
	return json.Marshal(&struct {
		*Alias
	}{
		Alias: (*Alias)(a),
	})
}

// Constants for common ActivityPub types
const (
	// Actor types
	PersonType       = "Person"
	ServiceType      = "Service"
	GroupType        = "Group"
	OrganizationType = "Organization"
	ApplicationType  = "Application"

	// Activity types
	CreateType   = "Create"
	UpdateType   = "Update"
	DeleteType   = "Delete"
	FollowType   = "Follow"
	AcceptType   = "Accept"
	RejectType   = "Reject"
	LikeType     = "Like"
	AnnounceType = "Announce"
	UndoType     = "Undo"
	BlockType    = "Block"
	FlagType     = "Flag"
	MoveType     = "Move"
	AddType      = "Add"
	RemoveType   = "Remove"

	// Object types
	NoteType              = "Note"
	ArticleType           = "Article"
	ImageType             = "Image"
	VideoType             = "Video"
	DocumentType          = "Document"
	CollectionType        = "Collection"
	OrderedCollectionType = "OrderedCollection"

	// Collection types
	InboxCollection     = "inbox"
	OutboxCollection    = "outbox"
	FollowersCollection = "followers"
	FollowingCollection = "following"
	LikedCollection     = "liked"

	// Public addressing
	PublicAddress = "https://www.w3.org/ns/activitystreams#Public"
)

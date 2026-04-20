package activitypub

import (
	"bytes"
	"encoding/json"
	"time"
)

// ContextValue wraps the ActivityPub @context field to support both legacy string
// and modern array representations.
type ContextValue []any

// MarshalJSON persists the context as a string when a single string value is present,
// otherwise it writes an array to match ActivityPub expectations.
func (c ContextValue) MarshalJSON() ([]byte, error) {
	if len(c) == 1 {
		if s, ok := c[0].(string); ok {
			return json.Marshal(s)
		}
	}
	return json.Marshal([]any(c))
}

// UnmarshalJSON accepts either a single string or an array of values.
func (c *ContextValue) UnmarshalJSON(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		*c = nil
		return nil
	}

	switch trimmed[0] {
	case '[':
		var arr []any
		if err := json.Unmarshal(trimmed, &arr); err != nil {
			return err
		}
		*c = ContextValue(arr)
		return nil
	case '"':
		var s string
		if err := json.Unmarshal(trimmed, &s); err != nil {
			return err
		}
		*c = ContextValue{s}
		return nil
	default:
		var v any
		if err := json.Unmarshal(trimmed, &v); err != nil {
			return err
		}
		switch vv := v.(type) {
		case []any:
			*c = ContextValue(vv)
		case string:
			*c = ContextValue{vv}
		default:
			*c = ContextValue{vv}
		}
		return nil
	}
}

// Clone returns a shallow copy of the context slice so callers can safely append
// without mutating the shared base context.
func (c ContextValue) Clone() ContextValue {
	if c == nil {
		return nil
	}
	clone := make(ContextValue, len(c))
	copy(clone, c)
	return clone
}

// With returns a new context that includes the receiver's entries plus the provided ones.
func (c ContextValue) With(values ...any) ContextValue {
	if len(values) == 0 {
		return c.Clone()
	}
	clone := c.Clone()
	clone = append(clone, values...)
	return clone
}

// DefaultContext represents the bare ActivityStreams context.
var DefaultContext = ContextValue{"https://www.w3.org/ns/activitystreams"}

// Context represents the richer JSON-LD context for ActivityStreams with Mastodon extensions.
var Context = ContextValue{
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
		"lessersoul":                "https://spec.lessersoul.ai/ns/agent-attribution/v1#",
		"agentAttribution": map[string]any{
			"@id":   "lessersoul:agentAttribution",
			"@type": "@json",
		},
		"agentManifest": "https://lesser.social/ns/agentManifest",
	},
}

// BaseObject represents the base ActivityStreams object
type BaseObject struct {
	Context   ContextValue `json:"@context,omitempty"`
	ID        string       `json:"id"`
	Type      string       `json:"type"`
	Published *time.Time   `json:"published,omitempty"`
	Updated   *time.Time   `json:"updated,omitempty"`
	To        []string     `json:"to,omitempty"`
	CC        []string     `json:"cc,omitempty"`
	BTo       []string     `json:"bto,omitempty"`
	BCC       []string     `json:"bcc,omitempty"`
	InReplyTo string       `json:"inReplyTo,omitempty"`
	Summary   string       `json:"summary,omitempty"`
	Sensitive bool         `json:"sensitive,omitempty"`
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

	// Account migration support
	AlsoKnownAs []string `json:"alsoKnownAs,omitempty"`
	MovedTo     string   `json:"movedTo,omitempty"`

	// Mastodon-specific fields
	ManuallyApprovesFollowers bool         `json:"manuallyApprovesFollowers,omitempty"`
	Discoverable              bool         `json:"discoverable,omitempty"`
	Featured                  string       `json:"featured,omitempty"`
	LastStatusAt              *time.Time   `json:"lastStatusAt,omitempty"`
	CreatedAt                 *time.Time   `json:"createdAt,omitempty"`
	Attachment                []Attachment `json:"attachment,omitempty"`

	// Lesser extension: optional agent metadata for Service actors.
	AgentManifest *AgentManifest `json:"agentManifest,omitempty"`
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
	Content            string        `json:"content"`
	AttributedTo       string        `json:"attributedTo"`
	Attachment         []Attachment  `json:"attachment,omitempty"`
	Tag                []Tag         `json:"tag,omitempty"`
	ConversationID     string        `json:"conversationId,omitempty"` // For tracking conversation threads
	Visibility         string        `json:"_:visibility,omitempty"`   // Lesser extension for preserving visibility
	QuoteURL           string        `json:"quoteUrl,omitempty"`
	Quoteable          bool          `json:"_:quoteable,omitempty"`
	QuoteNotifications bool          `json:"_:quoteNotifications,omitempty"`
	QuoteContext       *QuoteContext `json:"_:quoteContext,omitempty"`

	// Lesser extension: per-status attribution for agent-authored content.
	AgentAttribution *AgentPostAttribution `json:"agentAttribution,omitempty"`
}

type noteAlias Note

type legacyNote struct {
	noteAlias
	LegacyAgentAttribution *AgentPostAttribution `json:"_:agentAttribution,omitempty"`
}

// MarshalJSON normalizes agent attribution before emitting a Note.
func (n Note) MarshalJSON() ([]byte, error) {
	out := noteAlias(n)
	out.AgentAttribution = normalizeAgentPostAttributionForActor(n.AgentAttribution, n.AttributedTo)

	return json.Marshal(out)
}

// UnmarshalJSON accepts both the new namespaced agentAttribution key and the legacy
// _:agentAttribution key stored in older records.
func (n *Note) UnmarshalJSON(data []byte) error {
	var legacy legacyNote
	if err := json.Unmarshal(data, &legacy); err != nil {
		return err
	}

	*n = Note(legacy.noteAlias)
	if n.AgentAttribution == nil && legacy.LegacyAgentAttribution != nil {
		n.AgentAttribution = legacy.LegacyAgentAttribution
	}

	return nil
}

// QuoteNote is retained for backwards compatibility; it now simply aliases Note.
type QuoteNote struct {
	Note
}

// MarshalJSON preserves QuoteNote compatibility while normalizing agent attribution.
func (q QuoteNote) MarshalJSON() ([]byte, error) {
	out := noteAlias(q.Note)
	out.AgentAttribution = normalizeAgentPostAttributionForActor(q.AgentAttribution, q.AttributedTo)

	return json.Marshal(out)
}

// QuoteContext provides metadata about a quoted note
type QuoteContext struct {
	OriginalNoteID         string `json:"originalNoteId,omitempty"`
	OriginalAuthor         string `json:"originalAuthor"`
	OriginalAuthorUsername string `json:"originalAuthorUsername,omitempty"`
	QuoteCount             int    `json:"quoteCount"`
	AllowWithdrawal        bool   `json:"allowWithdrawal"`
	QuoteAllowed           bool   `json:"quoteAllowed"`
	Withdrawn              bool   `json:"withdrawn"`
	OriginalStatus         any    `json:"-"`
}

// Article represents long-form content
type Article struct {
	Note
	Name string `json:"name"` // Title
}

// MarshalJSON preserves Article fields while normalizing embedded agent attribution.
func (a Article) MarshalJSON() ([]byte, error) {
	type articleAlias struct {
		noteAlias
		Name string `json:"name"`
	}

	out := articleAlias{
		noteAlias: noteAlias(a.Note),
		Name:      a.Name,
	}
	out.AgentAttribution = normalizeAgentPostAttributionForActor(a.AgentAttribution, a.AttributedTo)

	return json.Marshal(out)
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

// Tombstone represents a deleted object placeholder per ActivityPub spec
type Tombstone struct {
	BaseObject
	FormerType string `json:"formerType,omitempty"` // The original object type
	Deleted    string `json:"deleted,omitempty"`    // ISO 8601 timestamp of deletion
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
	NoteType                  = "Note"
	ArticleType               = "Article"
	ImageType                 = "Image"
	VideoType                 = "Video"
	DocumentType              = "Document"
	CollectionType            = "Collection"
	OrderedCollectionType     = "OrderedCollection"
	OrderedCollectionPageType = "OrderedCollectionPage"
	TombstoneType             = "Tombstone"

	// Collection types
	InboxCollection     = "inbox"
	OutboxCollection    = "outbox"
	FollowersCollection = "followers"
	FollowingCollection = "following"
	LikedCollection     = "liked"

	// Public addressing
	PublicAddress = "https://www.w3.org/ns/activitystreams#Public"
)

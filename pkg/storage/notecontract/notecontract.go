package notecontract

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
)

// Marshal renders a Status.Note payload into the canonical Dynamo-safe map shape
// owned by the note contract. The stored field names intentionally preserve the
// existing nested note layout so readers do not require a migration to hydrate
// current and future rows.
func Marshal(note *activitypub.Note) (map[string]any, error) {
	if note == nil {
		return nil, nil
	}

	payload := persistedNoteFromActivityPub(note)
	blob, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal persisted note contract: %w", err)
	}

	var out map[string]any
	if err := json.Unmarshal(blob, &out); err != nil {
		return nil, fmt.Errorf("decode persisted note contract: %w", err)
	}

	return out, nil
}

// Unmarshal hydrates the canonical persisted note map back into an ActivityPub
// Note. It accepts both the canonical stored field names and ActivityPub JSON key
// aliases so legacy reads and fixture helpers can share one entry point.
func Unmarshal(raw map[string]any) (*activitypub.Note, error) {
	if raw == nil {
		return nil, nil
	}

	normalized := normalizePersistedNoteMap(raw)
	blob, err := json.Marshal(normalized)
	if err != nil {
		return nil, fmt.Errorf("marshal persisted note payload: %w", err)
	}

	var payload persistedNote
	if err := json.Unmarshal(blob, &payload); err != nil {
		return nil, fmt.Errorf("unmarshal persisted note payload: %w", err)
	}

	return payload.toActivityPub(), nil
}

// Normalize deep-copies a note through the canonical persisted representation so
// every write path sees the same contract-owned shape before persistence.
func Normalize(note *activitypub.Note) (*activitypub.Note, error) {
	raw, err := Marshal(note)
	if err != nil {
		return nil, err
	}
	return Unmarshal(raw)
}

type persistedNote struct {
	BaseObject         persistedBaseObject            `json:"BaseObject,omitempty"`
	Content            string                         `json:"Content,omitempty"`
	AttributedTo       string                         `json:"AttributedTo,omitempty"`
	Attachment         []persistedAttachment          `json:"Attachment,omitempty"`
	Tag                []persistedTag                 `json:"Tag,omitempty"`
	ConversationID     string                         `json:"ConversationID,omitempty"`
	Visibility         string                         `json:"Visibility,omitempty"`
	QuoteURL           string                         `json:"QuoteURL,omitempty"`
	Quoteable          bool                           `json:"Quoteable,omitempty"`
	QuoteNotifications bool                           `json:"QuoteNotifications,omitempty"`
	QuoteContext       *persistedQuoteContext         `json:"QuoteContext,omitempty"`
	AgentAttribution   *persistedAgentPostAttribution `json:"AgentAttribution,omitempty"`
}

type persistedBaseObject struct {
	Context   activitypub.ContextValue `json:"Context,omitempty"`
	ID        string                   `json:"ID,omitempty"`
	Type      string                   `json:"Type,omitempty"`
	Published *time.Time               `json:"Published,omitempty"`
	Updated   *time.Time               `json:"Updated,omitempty"`
	To        []string                 `json:"To,omitempty"`
	CC        []string                 `json:"CC,omitempty"`
	BTo       []string                 `json:"BTo,omitempty"`
	BCC       []string                 `json:"BCC,omitempty"`
	InReplyTo string                   `json:"InReplyTo,omitempty"`
	Summary   string                   `json:"Summary,omitempty"`
	Sensitive bool                     `json:"Sensitive,omitempty"`
}

type persistedAttachment struct {
	Type      string `json:"Type,omitempty"`
	MediaType string `json:"MediaType,omitempty"`
	URL       string `json:"URL,omitempty"`
	Name      string `json:"Name,omitempty"`
	Value     string `json:"Value,omitempty"`
	Width     int    `json:"Width,omitempty"`
	Height    int    `json:"Height,omitempty"`
}

type persistedTag struct {
	Type string `json:"Type,omitempty"`
	Href string `json:"Href,omitempty"`
	Name string `json:"Name,omitempty"`
}

type persistedQuoteContext struct {
	OriginalNoteID         string `json:"OriginalNoteID,omitempty"`
	OriginalAuthor         string `json:"OriginalAuthor,omitempty"`
	OriginalAuthorUsername string `json:"OriginalAuthorUsername,omitempty"`
	QuoteCount             int    `json:"QuoteCount,omitempty"`
	AllowWithdrawal        bool   `json:"AllowWithdrawal,omitempty"`
	QuoteAllowed           bool   `json:"QuoteAllowed,omitempty"`
	Withdrawn              bool   `json:"Withdrawn,omitempty"`
}

type persistedAgentPostAttribution struct {
	TriggerType     string   `json:"TriggerType,omitempty"`
	TriggerDetails  string   `json:"TriggerDetails,omitempty"`
	MemoryCitations []string `json:"MemoryCitations,omitempty"`
	DelegatedBy     string   `json:"DelegatedBy,omitempty"`
	DelegatedByDID  string   `json:"DelegatedByDID,omitempty"`
	Scopes          []string `json:"Scopes,omitempty"`
	Constraints     []string `json:"Constraints,omitempty"`
	SchemaVersion   string   `json:"SchemaVersion,omitempty"`
	ModelID         string   `json:"ModelID,omitempty"`
	ModelVersion    string   `json:"ModelVersion,omitempty"`
}

func persistedNoteFromActivityPub(note *activitypub.Note) persistedNote {
	if note == nil {
		return persistedNote{}
	}

	out := persistedNote{
		BaseObject: persistedBaseObject{
			Context:   note.Context.Clone(),
			ID:        note.ID,
			Type:      note.Type,
			Published: cloneTimePtr(note.Published),
			Updated:   cloneTimePtr(note.Updated),
			To:        append([]string(nil), note.To...),
			CC:        append([]string(nil), note.CC...),
			BTo:       append([]string(nil), note.BTo...),
			BCC:       append([]string(nil), note.BCC...),
			InReplyTo: note.InReplyTo,
			Summary:   note.Summary,
			Sensitive: note.Sensitive,
		},
		Content:            note.Content,
		AttributedTo:       note.AttributedTo,
		ConversationID:     note.ConversationID,
		Visibility:         note.Visibility,
		QuoteURL:           note.QuoteURL,
		Quoteable:          note.Quoteable,
		QuoteNotifications: note.QuoteNotifications,
	}

	if len(note.Attachment) > 0 {
		out.Attachment = make([]persistedAttachment, 0, len(note.Attachment))
		for _, attachment := range note.Attachment {
			out.Attachment = append(out.Attachment, persistedAttachment{
				Type:      attachment.Type,
				MediaType: attachment.MediaType,
				URL:       attachment.URL,
				Name:      attachment.Name,
				Value:     attachment.Value,
				Width:     attachment.Width,
				Height:    attachment.Height,
			})
		}
	}

	if len(note.Tag) > 0 {
		out.Tag = make([]persistedTag, 0, len(note.Tag))
		for _, tag := range note.Tag {
			out.Tag = append(out.Tag, persistedTag{
				Type: tag.Type,
				Href: tag.Href,
				Name: tag.Name,
			})
		}
	}

	if note.QuoteContext != nil {
		out.QuoteContext = &persistedQuoteContext{
			OriginalNoteID:         note.QuoteContext.OriginalNoteID,
			OriginalAuthor:         note.QuoteContext.OriginalAuthor,
			OriginalAuthorUsername: note.QuoteContext.OriginalAuthorUsername,
			QuoteCount:             note.QuoteContext.QuoteCount,
			AllowWithdrawal:        note.QuoteContext.AllowWithdrawal,
			QuoteAllowed:           note.QuoteContext.QuoteAllowed,
			Withdrawn:              note.QuoteContext.Withdrawn,
		}
	}

	if note.AgentAttribution != nil {
		out.AgentAttribution = &persistedAgentPostAttribution{
			TriggerType:     note.AgentAttribution.TriggerType,
			TriggerDetails:  note.AgentAttribution.TriggerDetails,
			MemoryCitations: append([]string(nil), note.AgentAttribution.MemoryCitations...),
			DelegatedBy:     note.AgentAttribution.DelegatedBy,
			DelegatedByDID:  note.AgentAttribution.DelegatedByDID,
			Scopes:          append([]string(nil), note.AgentAttribution.Scopes...),
			Constraints:     append([]string(nil), note.AgentAttribution.Constraints...),
			SchemaVersion:   note.AgentAttribution.SchemaVersion,
			ModelID:         note.AgentAttribution.ModelID,
		}
	}

	return out
}

func (p persistedNote) toActivityPub() *activitypub.Note {
	note := &activitypub.Note{
		BaseObject: activitypub.BaseObject{
			Context:   p.BaseObject.Context.Clone(),
			ID:        p.BaseObject.ID,
			Type:      p.BaseObject.Type,
			Published: cloneTimePtr(p.BaseObject.Published),
			Updated:   cloneTimePtr(p.BaseObject.Updated),
			To:        append([]string(nil), p.BaseObject.To...),
			CC:        append([]string(nil), p.BaseObject.CC...),
			BTo:       append([]string(nil), p.BaseObject.BTo...),
			BCC:       append([]string(nil), p.BaseObject.BCC...),
			InReplyTo: p.BaseObject.InReplyTo,
			Summary:   p.BaseObject.Summary,
			Sensitive: p.BaseObject.Sensitive,
		},
		Content:            p.Content,
		AttributedTo:       p.AttributedTo,
		ConversationID:     p.ConversationID,
		Visibility:         p.Visibility,
		QuoteURL:           p.QuoteURL,
		Quoteable:          p.Quoteable,
		QuoteNotifications: p.QuoteNotifications,
	}

	if len(p.Attachment) > 0 {
		note.Attachment = make([]activitypub.Attachment, 0, len(p.Attachment))
		for _, attachment := range p.Attachment {
			note.Attachment = append(note.Attachment, activitypub.Attachment{
				Type:      attachment.Type,
				MediaType: attachment.MediaType,
				URL:       attachment.URL,
				Name:      attachment.Name,
				Value:     attachment.Value,
				Width:     attachment.Width,
				Height:    attachment.Height,
			})
		}
	}

	if len(p.Tag) > 0 {
		note.Tag = make([]activitypub.Tag, 0, len(p.Tag))
		for _, tag := range p.Tag {
			note.Tag = append(note.Tag, activitypub.Tag{
				Type: tag.Type,
				Href: tag.Href,
				Name: tag.Name,
			})
		}
	}

	if p.QuoteContext != nil {
		note.QuoteContext = &activitypub.QuoteContext{
			OriginalNoteID:         p.QuoteContext.OriginalNoteID,
			OriginalAuthor:         p.QuoteContext.OriginalAuthor,
			OriginalAuthorUsername: p.QuoteContext.OriginalAuthorUsername,
			QuoteCount:             p.QuoteContext.QuoteCount,
			AllowWithdrawal:        p.QuoteContext.AllowWithdrawal,
			QuoteAllowed:           p.QuoteContext.QuoteAllowed,
			Withdrawn:              p.QuoteContext.Withdrawn,
		}
	}

	if p.AgentAttribution != nil {
		note.AgentAttribution = &activitypub.AgentPostAttribution{
			TriggerType:     p.AgentAttribution.TriggerType,
			TriggerDetails:  p.AgentAttribution.TriggerDetails,
			MemoryCitations: append([]string(nil), p.AgentAttribution.MemoryCitations...),
			DelegatedBy:     p.AgentAttribution.DelegatedBy,
			DelegatedByDID:  p.AgentAttribution.DelegatedByDID,
			Scopes:          append([]string(nil), p.AgentAttribution.Scopes...),
			Constraints:     append([]string(nil), p.AgentAttribution.Constraints...),
			SchemaVersion:   p.AgentAttribution.SchemaVersion,
			ModelID:         p.AgentAttribution.ModelID,
		}
		if note.AgentAttribution.ModelID == "" && p.AgentAttribution.ModelVersion != "" {
			note.AgentAttribution.ModelID = p.AgentAttribution.ModelVersion
		}
	}

	return note
}

func normalizePersistedNoteMap(raw map[string]any) map[string]any {
	if len(raw) == 0 {
		return nil
	}

	if _, ok := raw["BaseObject"]; ok {
		return cloneAnyMap(raw)
	}

	out := make(map[string]any, len(raw))
	base := make(map[string]any)

	copyAnyAlias(base, raw, "Context", "@context")
	copyAnyAlias(base, raw, "ID", "id")
	copyAnyAlias(base, raw, "Type", "type")
	copyAnyAlias(base, raw, "Published", "published")
	copyAnyAlias(base, raw, "Updated", "updated")
	copyAnyAlias(base, raw, "To", "to")
	copyAnyAlias(base, raw, "CC", "cc")
	copyAnyAlias(base, raw, "BTo", "bto")
	copyAnyAlias(base, raw, "BCC", "bcc")
	copyAnyAlias(base, raw, "InReplyTo", "inReplyTo")
	copyAnyAlias(base, raw, "Summary", "summary")
	copyAnyAlias(base, raw, "Sensitive", "sensitive")
	if len(base) > 0 {
		out["BaseObject"] = base
	}

	copyAnyAlias(out, raw, "Content", "content")
	copyAnyAlias(out, raw, "AttributedTo", "attributedTo")
	copyAnyAlias(out, raw, "ConversationID", "conversationId")
	copyAnyAlias(out, raw, "Visibility", "_:visibility", "visibility")
	copyAnyAlias(out, raw, "QuoteURL", "quoteUrl")
	copyAnyAlias(out, raw, "Quoteable", "_:quoteable", "quoteable")
	copyAnyAlias(out, raw, "QuoteNotifications", "_:quoteNotifications", "quoteNotifications")

	if attachments, ok := firstAny(raw, "Attachment", "attachment"); ok {
		out["Attachment"] = normalizeAnySliceMaps(attachments, normalizeAttachmentMap)
	}
	if tags, ok := firstAny(raw, "Tag", "tag"); ok {
		out["Tag"] = normalizeAnySliceMaps(tags, normalizeTagMap)
	}
	if quoteContext, ok := firstAny(raw, "QuoteContext", "_:quoteContext", "quoteContext"); ok {
		if typed, ok := quoteContext.(map[string]any); ok {
			out["QuoteContext"] = normalizeQuoteContextMap(typed)
		}
	}
	if attr, ok := firstAny(raw, "AgentAttribution", "agentAttribution", "_:agentAttribution"); ok {
		if typed, ok := attr.(map[string]any); ok {
			out["AgentAttribution"] = normalizeAgentAttributionMap(typed)
		}
	}

	return out
}

func normalizeAttachmentMap(raw map[string]any) map[string]any {
	out := make(map[string]any, len(raw))
	copyAnyAlias(out, raw, "Type", "type")
	copyAnyAlias(out, raw, "MediaType", "mediaType")
	copyAnyAlias(out, raw, "URL", "url")
	copyAnyAlias(out, raw, "Name", "name")
	copyAnyAlias(out, raw, "Value", "value")
	copyAnyAlias(out, raw, "Width", "width")
	copyAnyAlias(out, raw, "Height", "height")
	return out
}

func normalizeTagMap(raw map[string]any) map[string]any {
	out := make(map[string]any, len(raw))
	copyAnyAlias(out, raw, "Type", "type")
	copyAnyAlias(out, raw, "Href", "href")
	copyAnyAlias(out, raw, "Name", "name")
	return out
}

func normalizeQuoteContextMap(raw map[string]any) map[string]any {
	out := make(map[string]any, len(raw))
	copyAnyAlias(out, raw, "OriginalNoteID", "originalNoteId")
	copyAnyAlias(out, raw, "OriginalAuthor", "originalAuthor")
	copyAnyAlias(out, raw, "OriginalAuthorUsername", "originalAuthorUsername")
	copyAnyAlias(out, raw, "QuoteCount", "quoteCount")
	copyAnyAlias(out, raw, "AllowWithdrawal", "allowWithdrawal")
	copyAnyAlias(out, raw, "QuoteAllowed", "quoteAllowed")
	copyAnyAlias(out, raw, "Withdrawn", "withdrawn")
	return out
}

func normalizeAgentAttributionMap(raw map[string]any) map[string]any {
	out := make(map[string]any, len(raw))
	copyAnyAlias(out, raw, "TriggerType", "trigger_type")
	copyAnyAlias(out, raw, "TriggerDetails", "trigger_details")
	copyAnyAlias(out, raw, "MemoryCitations", "memory_citations")
	copyAnyAlias(out, raw, "DelegatedBy", "delegated_by")
	copyAnyAlias(out, raw, "DelegatedByDID", "delegated_by_did")
	copyAnyAlias(out, raw, "Scopes", "scopes")
	copyAnyAlias(out, raw, "Constraints", "constraints")
	copyAnyAlias(out, raw, "SchemaVersion", "schema_version")
	copyAnyAlias(out, raw, "ModelID", "model_id")
	copyAnyAlias(out, raw, "ModelVersion", "model_version")
	return out
}

func normalizeAnySliceMaps(value any, normalize func(map[string]any) map[string]any) any {
	rawSlice, ok := value.([]any)
	if !ok {
		return value
	}

	out := make([]any, 0, len(rawSlice))
	for _, entry := range rawSlice {
		typed, ok := entry.(map[string]any)
		if !ok {
			out = append(out, entry)
			continue
		}
		out = append(out, normalize(typed))
	}
	return out
}

func copyAnyAlias(dest map[string]any, src map[string]any, target string, aliases ...string) {
	if dest == nil || src == nil {
		return
	}

	for _, key := range append([]string{target}, aliases...) {
		if value, ok := src[key]; ok {
			dest[target] = cloneAnyValue(value)
			return
		}
	}
}

func firstAny(src map[string]any, keys ...string) (any, bool) {
	for _, key := range keys {
		if value, ok := src[key]; ok {
			return value, true
		}
	}
	return nil, false
}

func cloneAnyMap(src map[string]any) map[string]any {
	if src == nil {
		return nil
	}

	out := make(map[string]any, len(src))
	for key, value := range src {
		out[key] = cloneAnyValue(value)
	}
	return out
}

func cloneAnyValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneAnyMap(typed)
	case []any:
		out := make([]any, 0, len(typed))
		for _, entry := range typed {
			out = append(out, cloneAnyValue(entry))
		}
		return out
	default:
		return typed
	}
}

func cloneTimePtr(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}

	copied := value.UTC()
	return &copied
}

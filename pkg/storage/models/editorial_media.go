package models

import (
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/cmsrender"
)

// EditorialLifecycle is the editorial lifecycle of an internal asset, distinct
// from the processing-pipeline Status enum ("pending/processing/ready/failed").
// The lifecycle is an inspectable editorial decision surface: withdrawal,
// supersession, and service unavailability are explicit states that block
// publication until the draft is re-reviewed against the surviving asset.
type EditorialLifecycle string

const (
	// EditorialLifecycleAvailable is the default servable editorial state.
	EditorialLifecycleAvailable EditorialLifecycle = "available"
	// EditorialLifecycleWithdrawn removes an asset from editorial circulation.
	EditorialLifecycleWithdrawn EditorialLifecycle = "withdrawn"
	// EditorialLifecycleSuperseded marks an asset replaced by a named successor.
	EditorialLifecycleSuperseded EditorialLifecycle = "superseded"
	// EditorialLifecycleUnavailable marks an asset whose bytes are not servable.
	EditorialLifecycleUnavailable EditorialLifecycle = "unavailable"
)

var validEditorialLifecycles = map[EditorialLifecycle]struct{}{
	EditorialLifecycleAvailable:   {},
	EditorialLifecycleWithdrawn:   {},
	EditorialLifecycleSuperseded:  {},
	EditorialLifecycleUnavailable: {},
}

// MediaVisibility describes whether media bytes may use the public social-media
// delivery posture or must remain inside the authenticated editorial workflow.
type MediaVisibility string

const (
	// MediaVisibilityPublic preserves the existing Mastodon/social-media posture.
	MediaVisibilityPublic MediaVisibility = "public"
	// MediaVisibilityInternal requires an owner or exact draft-review grant before access.
	MediaVisibilityInternal MediaVisibility = "internal"
)

// EditorialMediaRole describes how one media asset is used by an article draft.
type EditorialMediaRole string

const (
	// EditorialMediaRoleHero is the draft-side source for Article.featuredImage.
	EditorialMediaRoleHero EditorialMediaRole = "hero"
	// EditorialMediaRoleInline places an asset at a modeled position in article content.
	EditorialMediaRoleInline EditorialMediaRole = "inline"
	// EditorialMediaRoleSocialCard is the promotional/social-card presentation.
	EditorialMediaRoleSocialCard EditorialMediaRole = "social_card"
)

// EditorialMediaOrigin records the editorial origin of an asset.
type EditorialMediaOrigin string

const (
	// EditorialMediaOriginAIGenerated identifies wholly generated media.
	EditorialMediaOriginAIGenerated EditorialMediaOrigin = "ai_generated"
	// EditorialMediaOriginAIEdited identifies AI-modified source media.
	EditorialMediaOriginAIEdited EditorialMediaOrigin = "ai_edited"
	// EditorialMediaOriginPhotographed identifies photographic media.
	EditorialMediaOriginPhotographed EditorialMediaOrigin = "photographed"
	// EditorialMediaOriginIllustrated identifies illustrated media.
	EditorialMediaOriginIllustrated EditorialMediaOrigin = "illustrated"
	// EditorialMediaOriginSupplied identifies externally supplied media.
	EditorialMediaOriginSupplied EditorialMediaOrigin = "supplied"
)

var validEditorialMediaOrigins = map[EditorialMediaOrigin]struct{}{
	EditorialMediaOriginAIGenerated:  {},
	EditorialMediaOriginAIEdited:     {},
	EditorialMediaOriginPhotographed: {},
	EditorialMediaOriginIllustrated:  {},
	EditorialMediaOriginSupplied:     {},
}

const (
	maxDraftMediaUsages      = 100
	maxEditorialCaptionBytes = 1500
	maxEditorialCreditBytes  = 500
	maxEditorialAltTextBytes = 1500
	maxEditorialFocusBytes   = 100
	maxProvenanceToolBytes   = 500
	maxProvenanceRightsBytes = 4000
	maxProvenanceSources     = 50
	maxProvenanceSourceBytes = 2048
)

// MediaProvenance is internal editorial evidence about an asset. CreditLine on
// DraftMediaUsage is the distinct reader-facing attribution surface.
type MediaProvenance struct {
	Origin             EditorialMediaOrigin `json:"origin"`
	Tool               string               `json:"tool,omitempty"`
	ResponsibleActor   string               `json:"responsible_actor"`
	SourceReferences   []string             `json:"source_references,omitempty"`
	RightsLicenseNotes string               `json:"rights_license_notes,omitempty"`
	CreatedAt          *time.Time           `json:"created_at,omitempty"`
	UpdatedAt          *time.Time           `json:"updated_at,omitempty"`
	RecordedAt         time.Time            `json:"recorded_at"`
	ContentIntegrity   string               `json:"content_integrity"`
}

// DraftMediaUsage is per-draft editorial presentation metadata for an asset.
// InlinePosition is zero-based and required only for inline assets.
type DraftMediaUsage struct {
	MediaID        string             `json:"media_id"`
	Role           EditorialMediaRole `json:"role"`
	InlinePosition *int               `json:"inline_position,omitempty"`
	Caption        string             `json:"caption,omitempty"`
	CreditLine     string             `json:"credit_line,omitempty"`
	AltText        string             `json:"alt_text,omitempty"`
	Focus          string             `json:"focus,omitempty"`
}

// ArticleEditorialMedia is the durable published form of one editorial binding:
// the modeled usage plus the minted public serving composed into article HTML.
// The publish transition writes it onto the Article from the digest-verified
// minted bytes; a missing or unpublished asset never composes.
type ArticleEditorialMedia struct {
	MediaID        string             `json:"media_id"`
	Role           EditorialMediaRole `json:"role"`
	InlinePosition *int               `json:"inline_position,omitempty"`
	Caption        string             `json:"caption,omitempty"`
	CreditLine     string             `json:"credit_line,omitempty"`
	AltText        string             `json:"alt_text,omitempty"`
	Focus          string             `json:"focus,omitempty"`
	// URL is the durable published serving minted at the publish transition.
	URL         string `json:"url"`
	ContentType string `json:"content_type,omitempty"`
	ContentHash string `json:"content_hash,omitempty"`
	Width       int    `json:"width,omitempty"`
	Height      int    `json:"height,omitempty"`
}

// RenderMedia maps the published binding onto the canonical renderer's media
// descriptor so every article read path composes the minted serving.
func (m ArticleEditorialMedia) RenderMedia() cmsrender.ArticleMedia {
	position := 0
	if m.InlinePosition != nil && *m.InlinePosition > 0 {
		position = *m.InlinePosition
	}
	return cmsrender.ArticleMedia{
		Role:           cmsrender.ArticleMediaRole(strings.ToLower(strings.TrimSpace(string(m.Role)))),
		InlinePosition: position,
		URL:            strings.TrimSpace(m.URL),
		AltText:        strings.TrimSpace(m.AltText),
		Caption:        strings.TrimSpace(m.Caption),
		CreditLine:     strings.TrimSpace(m.CreditLine),
		Width:          m.Width,
		Height:         m.Height,
		ContentType:    strings.TrimSpace(m.ContentType),
	}
}

// Normalize validates and canonicalizes a provenance record while binding it
// to the media byte digest established by the M0 upload pipeline.
func (p *MediaProvenance) Normalize(owner, contentIntegrity string, now time.Time) error {
	if p == nil {
		return errors.New("editorial media provenance is required")
	}
	p.Origin = EditorialMediaOrigin(strings.ToLower(strings.TrimSpace(string(p.Origin))))
	if _, ok := validEditorialMediaOrigins[p.Origin]; !ok {
		return fmt.Errorf("invalid editorial media origin %q", p.Origin)
	}
	p.Tool = strings.TrimSpace(p.Tool)
	if (p.Origin == EditorialMediaOriginAIGenerated || p.Origin == EditorialMediaOriginAIEdited) && p.Tool == "" {
		return errors.New("generating or editing tool is required for AI-origin media")
	}
	if len(p.Tool) > maxProvenanceToolBytes {
		return errors.New("editorial media tool exceeds maximum length")
	}
	p.ResponsibleActor = strings.TrimSpace(p.ResponsibleActor)
	if p.ResponsibleActor == "" {
		p.ResponsibleActor = strings.TrimSpace(owner)
	}
	if p.ResponsibleActor == "" {
		return errors.New("responsible actor is required")
	}
	p.RightsLicenseNotes = strings.TrimSpace(p.RightsLicenseNotes)
	if len(p.RightsLicenseNotes) > maxProvenanceRightsBytes {
		return errors.New("editorial media rights/license notes exceed maximum length")
	}
	p.SourceReferences = normalizeEditorialStrings(p.SourceReferences)
	if len(p.SourceReferences) > maxProvenanceSources {
		return errors.New("editorial media source reference count exceeds maximum")
	}
	for _, reference := range p.SourceReferences {
		if len(reference) > maxProvenanceSourceBytes {
			return errors.New("editorial media source reference exceeds maximum length")
		}
	}
	expectedIntegrity := strings.TrimSpace(contentIntegrity)
	p.ContentIntegrity = strings.TrimSpace(p.ContentIntegrity)
	if p.ContentIntegrity == "" {
		p.ContentIntegrity = expectedIntegrity
	} else if p.ContentIntegrity != expectedIntegrity {
		return errors.New("editorial media provenance does not match the stored content integrity identifier")
	}
	if !strings.HasPrefix(p.ContentIntegrity, "sha256:") || len(p.ContentIntegrity) != len("sha256:")+64 {
		return errors.New("canonical sha256 content integrity identifier is required")
	}
	if _, err := hex.DecodeString(strings.TrimPrefix(p.ContentIntegrity, "sha256:")); err != nil {
		return errors.New("canonical sha256 content integrity identifier is required")
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if p.RecordedAt.IsZero() {
		p.RecordedAt = now.UTC()
	} else {
		p.RecordedAt = p.RecordedAt.UTC()
	}
	if p.CreatedAt != nil {
		created := p.CreatedAt.UTC()
		p.CreatedAt = &created
	}
	if p.UpdatedAt != nil {
		updated := p.UpdatedAt.UTC()
		p.UpdatedAt = &updated
	}
	if p.CreatedAt != nil && p.UpdatedAt != nil && p.UpdatedAt.Before(*p.CreatedAt) {
		return errors.New("editorial media updated timestamp precedes created timestamp")
	}
	return nil
}

// NormalizeDraftMediaUsages validates draft associations and returns a stable
// copy. A draft can have at most one hero and one social card; media IDs cannot
// be reused in multiple roles because an exact binding must be unambiguous.
func NormalizeDraftMediaUsages(usages []DraftMediaUsage) ([]DraftMediaUsage, error) {
	if len(usages) > maxDraftMediaUsages {
		return nil, fmt.Errorf("draft editorial media count exceeds %d", maxDraftMediaUsages)
	}
	out := make([]DraftMediaUsage, 0, len(usages))
	seenMedia := make(map[string]struct{}, len(usages))
	seenSingleton := make(map[EditorialMediaRole]struct{}, 2)
	seenInlinePositions := make(map[int]struct{}, len(usages))
	for _, source := range usages {
		u := source
		u.MediaID = strings.TrimSpace(u.MediaID)
		u.Role = EditorialMediaRole(strings.ToLower(strings.TrimSpace(string(u.Role))))
		u.Caption = strings.TrimSpace(u.Caption)
		u.CreditLine = strings.TrimSpace(u.CreditLine)
		u.AltText = strings.TrimSpace(u.AltText)
		u.Focus = strings.TrimSpace(u.Focus)
		if len(u.Caption) > maxEditorialCaptionBytes {
			return nil, errors.New("editorial media caption exceeds maximum length")
		}
		if len(u.CreditLine) > maxEditorialCreditBytes {
			return nil, errors.New("editorial media credit line exceeds maximum length")
		}
		if len(u.AltText) > maxEditorialAltTextBytes {
			return nil, errors.New("editorial media alt text exceeds maximum length")
		}
		if len(u.Focus) > maxEditorialFocusBytes {
			return nil, errors.New("editorial media focus exceeds maximum length")
		}
		if u.MediaID == "" {
			return nil, errors.New("editorial media ID is required")
		}
		if _, exists := seenMedia[u.MediaID]; exists {
			return nil, fmt.Errorf("editorial media %q is bound more than once", u.MediaID)
		}
		seenMedia[u.MediaID] = struct{}{}
		switch u.Role {
		case EditorialMediaRoleHero, EditorialMediaRoleSocialCard:
			if u.InlinePosition != nil {
				return nil, fmt.Errorf("%s media cannot have an inline position", u.Role)
			}
			if _, exists := seenSingleton[u.Role]; exists {
				return nil, fmt.Errorf("draft can have only one %s asset", u.Role)
			}
			seenSingleton[u.Role] = struct{}{}
		case EditorialMediaRoleInline:
			if u.InlinePosition == nil || *u.InlinePosition < 0 {
				return nil, errors.New("inline media requires a non-negative inline position")
			}
			if _, exists := seenInlinePositions[*u.InlinePosition]; exists {
				return nil, fmt.Errorf("inline position %d is already occupied", *u.InlinePosition)
			}
			seenInlinePositions[*u.InlinePosition] = struct{}{}
		default:
			return nil, fmt.Errorf("invalid editorial media role %q", u.Role)
		}
		out = append(out, u)
	}
	return out, nil
}

func normalizeEditorialStrings(values []string) []string {
	out := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

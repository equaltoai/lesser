package notes

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
	svcErrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"go.uber.org/zap"
)

// Promo release sentinel errors. They are joined with storage-layer errors so
// callers can both classify the failure and surface the storage root cause.
var (
	// ErrPromoVisibilityRestricted means a promo release attempted a visibility
	// other than public or unlisted (issue #1446 scope). It is a structural
	// rejection, not a policy hint.
	ErrPromoVisibilityRestricted = errors.New("promo release visibility must be public or unlisted")
	// ErrPromoAssetNotPublished means a bound asset is not in the M2 PUBLISHED
	// durable state (IsPublished). Attaching internal/unpublished bytes to an
	// outbound post is structurally prevented here, not merely discouraged.
	ErrPromoAssetNotPublished = errors.New("promo release asset is not in the PUBLISHED durable state")
	// ErrPromoAssetDigestMismatch means the asset's live canonical digest no
	// longer equals the digest bound into the reviewed package — the exact
	// reviewed bytes can no longer be attached (no substitution).
	ErrPromoAssetDigestMismatch = errors.New("promo release asset bytes changed after review")
)

// PromoPublishedMediaRef identifies one approved, PUBLISHED asset the promo
// release attaches to the outbound post. It carries the media identity and the
// canonical sha256 digest bound into the reviewed package; the notes service
// re-verifies the live media record against both before attaching, so the exact
// reviewed bytes are what attach — a stale or substituted reference fails
// closed. The attachment URL always comes from the media record's M2 durable
// published serving (PublishedURL), never from a caller-supplied URL.
type PromoPublishedMediaRef struct {
	MediaID     string
	ContentHash string
}

// CreatePromoNote creates the outbound post for an approved promo package. It
// is the release seam for the M4 gate: visibility is structurally restricted to
// public/unlisted, and the published media set is validated against the
// PUBLISHED durable state and the digests bound at review time before any
// attachment is built. AI-authorship disclosure travels in cmd.AgentAttribution
// (set by the promo gate when any bound asset is AI-origin per provenance) and
// is preserved on the underlying Note exactly as the ordinary agent-post path
// does. The release creates the post and nothing else — no boosts, likes, or
// synthetic engagement side effects.
func (s *Service) CreatePromoNote(ctx context.Context, cmd *CreateNoteCommand, published []PromoPublishedMediaRef) (*NoteResult, error) {
	if cmd == nil {
		return nil, fmt.Errorf("create note command is required")
	}
	visibility := strings.ToLower(strings.TrimSpace(cmd.Visibility))
	if visibility != models.VisibilityPublic && visibility != models.VisibilityUnlisted {
		return nil, ErrPromoVisibilityRestricted
	}
	cmd.Visibility = visibility
	if len(published) == 0 {
		return nil, fmt.Errorf("promo release requires at least one published asset")
	}
	return s.createNote(ctx, cmd, published)
}

// preparePromoPublishedAttachments validates the approved PUBLISHED asset set
// and converts it to ActivityPub attachments, structurally preventing any
// internal/unpublished bytes from reaching the outbound post. Each asset must
// still exist, belong to the posting author, be in the PUBLISHED durable state
// (IsPublished), and carry the exact canonical digest bound into the reviewed
// package; the attachment URL is the record's M2 publishedUrl (world-served by
// design). The set is capped at the outbound Status attachment limit.
func (s *Service) preparePromoPublishedAttachments(ctx context.Context, author *storage.Account, published []PromoPublishedMediaRef) ([]activitypub.Attachment, []string, error) {
	if err := common.ValidateSliceLength("promo_media", published, 4); err != nil {
		s.logger.Warn("too many promo media attachments for status",
			zap.Int("limit", 4),
			zap.Int("requested", len(published)))
		return nil, nil, errors.Join(svcErrors.ErrValidationFailed, err)
	}

	if s.mediaRepo == nil {
		s.logger.Error("media repository unavailable for promo attachments")
		return nil, nil, svcErrors.ErrRetrieveMediaAttachment
	}

	attachments := make([]activitypub.Attachment, 0, len(published))
	markIDs := make([]string, 0, len(published))
	seen := make(map[string]struct{}, len(published))

	for _, ref := range published {
		mediaID := strings.TrimSpace(ref.MediaID)
		contentHash := strings.TrimSpace(ref.ContentHash)
		if mediaID == "" || contentHash == "" {
			return nil, nil, fmt.Errorf("%w: media ID and bound digest are required", svcErrors.ErrMediaAttachmentNotFound)
		}
		if _, dup := seen[mediaID]; dup {
			return nil, nil, fmt.Errorf("%w: media %s is bound more than once", svcErrors.ErrMediaAttachmentNotFound, mediaID)
		}
		seen[mediaID] = struct{}{}

		media, err := s.mediaRepo.GetMedia(ctx, mediaID)
		if err != nil {
			s.logger.Error("failed to get promo media attachment",
				zap.String("media_id", mediaID),
				zap.Error(err))
			return nil, nil, errors.Join(svcErrors.ErrMediaAttachmentNotFound, err)
		}
		if !s.mediaBelongsToAuthor(media, author) {
			s.logger.Warn("promo media attachment does not belong to author",
				zap.String("media_id", mediaID),
				zap.String("media_owner", media.UserID))
			return nil, nil, svcErrors.ErrMediaAttachmentNotFound
		}
		// The load-bearing PUBLISHED-only guard: only assets whose bytes are
		// already durably world-served may attach to an outbound post. Any
		// internal/unpublished lifecycle state is rejected here regardless of
		// what the caller requested.
		if !media.IsPublished() {
			s.logger.Warn("promo media attachment not in PUBLISHED state",
				zap.String("media_id", mediaID),
				zap.String("editorial_state", string(media.EditorialState)))
			return nil, nil, fmt.Errorf("%w: media %s", ErrPromoAssetNotPublished, mediaID)
		}
		// The exact reviewed bytes must still be the media's bytes: the digest
		// bound into the reviewed package must equal the record's live digest.
		if strings.TrimSpace(media.ContentHash) != contentHash {
			s.logger.Warn("promo media digest changed after review",
				zap.String("media_id", mediaID))
			return nil, nil, fmt.Errorf("%w: media %s", ErrPromoAssetDigestMismatch, mediaID)
		}

		attachments = append(attachments, promoAttachmentFromPublishedMedia(media))
		markIDs = append(markIDs, mediaID)
	}

	return attachments, markIDs, nil
}

// promoAttachmentFromPublishedMedia builds the ActivityPub attachment from the
// M2 durable published serving of the approved bytes. Unlike the ordinary
// social-media path it never falls back to an unguessable media route: the
// attachment URL is the publishedUrl minted at the publish transition, which is
// world-served by design for PUBLISHED assets.
func promoAttachmentFromPublishedMedia(media *models.Media) activitypub.Attachment {
	attachment := activitypub.Attachment{
		Type:      mapMediaCategoryToAttachmentType(media.MediaCategory),
		MediaType: media.ContentType,
		URL:       strings.TrimSpace(media.PublishedURL),
		Width:     media.Width,
		Height:    media.Height,
	}

	if media.Description != "" {
		attachment.Name = media.Description
	} else if media.FileName != "" {
		attachment.Name = media.FileName
	}

	if media.FileName != "" {
		attachment.Value = media.FileName
	}

	return attachment
}

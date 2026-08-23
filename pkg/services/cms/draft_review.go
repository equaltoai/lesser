package cms

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"sort"
	"strings"
	"time"

	apperrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
)

// Draft review verdict values.
const (
	DraftReviewApproved         = "APPROVED"
	DraftReviewChangesRequested = "CHANGES_REQUESTED"
	maxDraftReviewReadGrants    = 200

	// DraftReviewGrantLifetime bounds every review grant. Grants are cheap,
	// ephemeral assignments refreshed on re-share; the bound prevents a stale
	// grant from authorizing reviewer reads, URL minting, or approval forever.
	DraftReviewGrantLifetime = 7 * 24 * time.Hour

	// Bound-media blocking reasons exposed through the review state surface.
	DraftReviewMediaReasonMissing     = "BOUND_MEDIA_MISSING"
	DraftReviewMediaReasonNotReady    = "BOUND_MEDIA_NOT_READY"
	DraftReviewMediaReasonWithdrawn   = "BOUND_MEDIA_WITHDRAWN"
	DraftReviewMediaReasonSuperseded  = "BOUND_MEDIA_SUPERSEDED"
	DraftReviewMediaReasonUnavailable = "BOUND_MEDIA_UNAVAILABLE"
)

// draftReviewContentHash binds every field that changes the published article's
// reviewed content or permalink, including the ordered bound media set. Each
// field is length-prefixed so field boundaries remain unambiguous even when
// values contain control characters. Media usages hash their canonical content
// digest (Media.ContentHash / provenance ContentIntegrity), not just the
// MediaID, so the review hash is bound to bytes even if a future path mutates
// records. Metadata, autosave state, and timestamps do not reach the published
// article and are intentionally excluded. GeneratedBy is also excluded because
// cmsApplyDraftRequestAttribution in graph/mutation_resolvers_cms.go only sets
// and never clears it; that invariant is load-bearing because GeneratedBy
// enables the principal-approval gate.
func draftReviewContentHash(d *models.Draft, mediaDigests map[string]string) string {
	h := sha256.New()
	var length [8]byte
	write := func(value string) {
		binary.BigEndian.PutUint64(length[:], uint64(len(value)))
		_, _ = h.Write(length[:])
		_, _ = h.Write([]byte(value))
	}
	for _, field := range []string{d.ContentFormat, d.Slug, d.Title, d.Content} {
		write(field)
	}
	// Canonical media order: hero, then inline by InlinePosition, then social
	// card. A usage whose digest cannot be resolved hashes as an empty digest;
	// the binding still contributes role, position, caption, credit, alt, and
	// focus, so replace/remove/reorder/recaption all change the revision hash.
	for _, usage := range canonicalDraftMediaOrder(d.EditorialMedia) {
		write(mediaDigests[usage.MediaID])
		write(string(usage.Role))
		position := int64(-1)
		if usage.InlinePosition != nil {
			position = int64(*usage.InlinePosition)
		}
		var positionBytes [8]byte
		binary.BigEndian.PutUint64(positionBytes[:], uint64(position))
		_, _ = h.Write(positionBytes[:])
		write(usage.Caption)
		write(usage.CreditLine)
		write(usage.AltText)
		write(usage.Focus)
	}
	return hex.EncodeToString(h.Sum(nil))
}

// canonicalDraftMediaOrder returns the ordered media set used by the revision
// hash: hero first, inline by ascending InlinePosition, then social card. The
// role partition, not the caller's list order, decides, so [social, hero] and
// [hero, social] bindings hash identically and reordering the list cannot
// stale a prior approval.
func canonicalDraftMediaOrder(usages []models.DraftMediaUsage) []models.DraftMediaUsage {
	out := make([]models.DraftMediaUsage, 0, len(usages))
	heroes := make([]models.DraftMediaUsage, 0, len(usages))
	inline := make([]models.DraftMediaUsage, 0, len(usages))
	social := make([]models.DraftMediaUsage, 0, len(usages))
	for _, usage := range usages {
		switch usage.Role {
		case models.EditorialMediaRoleHero:
			heroes = append(heroes, usage)
		case models.EditorialMediaRoleInline:
			inline = append(inline, usage)
		case models.EditorialMediaRoleSocialCard:
			social = append(social, usage)
		}
	}
	sort.SliceStable(inline, func(i, j int) bool {
		left, right := -1, -1
		if inline[i].InlinePosition != nil {
			left = *inline[i].InlinePosition
		}
		if inline[j].InlinePosition != nil {
			right = *inline[j].InlinePosition
		}
		return left < right
	})
	out = append(out, heroes...)
	out = append(out, inline...)
	out = append(out, social...)
	return out
}

// DraftReviewContentHash returns the canonical text hash used to bind review
// verdicts to draft content when no media digest resolution is available
// (equivalent to a draft with no bound media).
func DraftReviewContentHash(d *models.Draft) string {
	if d == nil {
		return ""
	}
	return draftReviewContentHash(d, nil)
}

// DraftReviewContentHashWithMedia binds review verdicts to the exact bound
// media bytes. mediaDigests maps each bound MediaID to its canonical
// sha256:<hex> digest; callers resolve digests through the editorial media
// repository (bounded at 100 usages per draft).
func DraftReviewContentHashWithMedia(d *models.Draft, mediaDigests map[string]string) string {
	if d == nil {
		return ""
	}
	return draftReviewContentHash(d, mediaDigests)
}

var (
	// ErrDraftReviewApprovalRequired means an active reviewer is missing a current approval.
	ErrDraftReviewApprovalRequired = errors.New("draft requires approval from every active reviewer")
	// ErrDraftReviewPrincipalApprovalRequired means the designated principal is missing a current approval.
	ErrDraftReviewPrincipalApprovalRequired = errors.New("generated draft requires an active approval from the instance principal")
	// ErrDraftReviewMediaRequired means a required bound asset cannot serve the
	// exact approved bytes (missing, not ready, withdrawn, superseded, or
	// unavailable). The wrapped message names the blocking reasons.
	ErrDraftReviewMediaRequired = errors.New("draft requires its bound media to be ready and available")
	// ErrInstancePrincipalUnavailable means the principal provider could not be used.
	ErrInstancePrincipalUnavailable = errors.New("instance principal is unavailable")
	// ErrInstancePrincipalNotConfigured means the provider returned no principal username.
	ErrInstancePrincipalNotConfigured = errors.New("instance principal is not configured")

	errDraftReviewStorageUnavailable = errors.New("draft review storage is not available")
)

type draftReviewRepository interface {
	CreateDraftReviewGrant(context.Context, *models.DraftReviewGrant) error
	RegrantDraftReviewGrant(context.Context, *models.DraftReviewGrant) error
	RevokeDraftReviewGrant(context.Context, *models.DraftReviewGrant) error
	GetDraftReviewGrant(context.Context, string, string, string) (*models.DraftReviewGrant, error)
	ListActiveDraftReviewGrants(context.Context, string, int, string) ([]*models.DraftReviewGrant, string, error)
	CountActiveDraftReviewGrants(context.Context, string) (int, error)
	ListDraftReviewGrants(context.Context, string, string) ([]*models.DraftReviewGrant, error)
	CreateDraftReviewVerdict(context.Context, *models.DraftReviewVerdict) error
	ListDraftReviewVerdicts(context.Context, string, string) ([]*models.DraftReviewVerdict, error)
}

type draftReviewFieldUpdater interface {
	UpdateDraftReviewFields(context.Context, string, *models.Draft) error
}

type ownedDraftReviewRepository interface {
	ListDraftReviewGrantsByOwner(context.Context, string) ([]*models.DraftReviewGrant, error)
}

// resolveDraftMediaBindings resolves every bound usage to its canonical content
// digest (bounded at 100 usages per draft) and reports, in canonical order,
// which assets cannot serve the exact approved bytes at the review or publish
// boundary. A resolution failure (including an unwired media repository on a
// draft that has bound media) is returned as an error so the review and publish
// gates fail closed instead of approving against unresolvable bytes.
func (s *DraftService) resolveDraftMediaBindings(ctx context.Context, draft *models.Draft) (map[string]string, []string, error) {
	digests := make(map[string]string, len(draft.EditorialMedia))
	if len(draft.EditorialMedia) == 0 {
		return digests, nil, nil
	}
	if s.mediaRepo == nil {
		return nil, nil, errors.New("editorial media repository is unavailable")
	}
	var reasons []string
	for _, usage := range draft.EditorialMedia {
		media, getErr := s.mediaRepo.GetMedia(ctx, usage.MediaID)
		if getErr != nil && (errors.Is(getErr, storage.ErrNotFound) || apperrors.HasCode(getErr, apperrors.CodeNotFound)) {
			reasons = append(reasons, DraftReviewMediaReasonMissing)
			continue
		}
		if getErr != nil {
			return nil, nil, getErr
		}
		if media == nil || !strings.EqualFold(strings.TrimSpace(media.UserID), strings.TrimSpace(draft.AuthorID)) {
			reasons = append(reasons, DraftReviewMediaReasonMissing)
			continue
		}
		if reason := draftMediaLifecycleReason(media); reason != "" {
			reasons = append(reasons, reason)
			continue
		}
		if !media.IsInternalEditorial() || media.Provenance == nil || media.Provenance.ContentIntegrity != media.ContentHash {
			reasons = append(reasons, DraftReviewMediaReasonUnavailable)
			continue
		}
		if !media.IsReady() {
			reasons = append(reasons, DraftReviewMediaReasonNotReady)
			continue
		}
		digests[usage.MediaID] = media.ContentHash
	}
	return digests, reasons, nil
}

// draftMediaLifecycleReason maps the explicit editorial lifecycle onto the
// blocking-reason vocabulary. The empty/available lifecycle is servable.
func draftMediaLifecycleReason(media *models.Media) string {
	if media == nil {
		return DraftReviewMediaReasonMissing
	}
	switch models.EditorialLifecycle(strings.ToLower(strings.TrimSpace(string(media.EditorialState)))) {
	case "", models.EditorialLifecycleAvailable:
		return ""
	case models.EditorialLifecycleWithdrawn:
		return DraftReviewMediaReasonWithdrawn
	case models.EditorialLifecycleSuperseded:
		return DraftReviewMediaReasonSuperseded
	default:
		return DraftReviewMediaReasonUnavailable
	}
}

// draftContentHash resolves the current text-and-media digest for a draft.
// Review verdicts, the read state, and the publish/schedule gates all use this
// exact digest so media binding changes stale prior approvals end-to-end.
func (s *DraftService) draftContentHash(ctx context.Context, draft *models.Draft) (string, error) {
	digests, _, err := s.resolveDraftMediaBindings(ctx, draft)
	if err != nil {
		return "", err
	}
	return draftReviewContentHash(draft, digests), nil
}

// DraftEditorialMediaBinding resolves one modeled usage without hiding a
// missing asset. Preview clients need the nil Media value to render a
// conspicuous missing placeholder instead of silently dropping the binding.
type DraftEditorialMediaBinding struct {
	Usage models.DraftMediaUsage
	Media *models.Media
}

func (s *DraftService) reviewRepository() (draftReviewRepository, error) {
	repo, ok := s.draftRepo.(draftReviewRepository)
	if !ok || repo == nil {
		return nil, errDraftReviewStorageUnavailable
	}
	return repo, nil
}

// ShareDraftForReview creates or refreshes an owner-authorized reviewer grant.
func (s *DraftService) ShareDraftForReview(ctx context.Context, owner, draftID, reviewer string) (*models.DraftReviewGrant, error) {
	owner = strings.TrimSpace(owner)
	draftID = strings.TrimSpace(draftID)
	reviewer = strings.TrimSpace(reviewer)
	if owner == "" || draftID == "" || reviewer == "" {
		return nil, errors.New("owner, draft, and reviewer are required")
	}
	if owner == reviewer {
		principal, err := s.instancePrincipal(ctx)
		if err != nil || principal != owner {
			return nil, errors.New("draft owner cannot review their own draft")
		}
	}
	if _, err := s.draftRepo.GetDraft(ctx, owner, draftID); err != nil {
		return nil, err
	}
	repo, err := s.reviewRepository()
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	expiresAt := now.Add(DraftReviewGrantLifetime)
	g := &models.DraftReviewGrant{OwnerID: owner, DraftID: draftID, Reviewer: reviewer, GrantedAt: now, ExpiresAt: &expiresAt}
	existing, getErr := repo.GetDraftReviewGrant(ctx, owner, draftID, reviewer)
	if getErr == nil && existing != nil {
		g.Version = existing.Version
		if err := repo.RegrantDraftReviewGrant(ctx, g); err != nil {
			return nil, err
		}
		return g, nil
	}
	if getErr != nil && !errors.Is(getErr, storage.ErrNotFound) {
		return nil, getErr
	}
	if err := repo.CreateDraftReviewGrant(ctx, g); err != nil {
		return nil, err
	}
	return g, nil
}

// RevokeDraftReview immediately disables a reviewer grant.
func (s *DraftService) RevokeDraftReview(ctx context.Context, owner, draftID, reviewer string) error {
	repo, err := s.reviewRepository()
	if err != nil {
		return err
	}
	g, err := repo.GetDraftReviewGrant(ctx, strings.TrimSpace(owner), strings.TrimSpace(draftID), strings.TrimSpace(reviewer))
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	g.RevokedAt = &now
	return repo.RevokeDraftReviewGrant(ctx, g)
}

// ActiveDraftReviewGrant returns a non-revoked, non-expired grant for one reviewer.
// Expired grants fail closed: they authorize neither reviewer reads/URL minting
// nor the approval gate.
func (s *DraftService) ActiveDraftReviewGrant(ctx context.Context, owner, draftID, reviewer string) (*models.DraftReviewGrant, error) {
	repo, err := s.reviewRepository()
	if err != nil {
		return nil, err
	}
	g, err := repo.GetDraftReviewGrant(ctx, owner, draftID, reviewer)
	if err != nil {
		return nil, err
	}
	if !g.IsActive(time.Now().UTC()) {
		return nil, errors.New("draft review grant is not active")
	}
	return g, nil
}

// SharedDraftReviews lists one cursor page of active review queue grants.
// Expired grants are excluded so a stale assignment cannot appear actionable.
func (s *DraftService) SharedDraftReviews(ctx context.Context, reviewer string, limit int, cursor string) ([]*models.DraftReviewGrant, string, error) {
	repo, err := s.reviewRepository()
	if err != nil {
		return nil, "", err
	}
	grants, nextCursor, err := repo.ListActiveDraftReviewGrants(ctx, strings.TrimSpace(reviewer), limit, strings.TrimSpace(cursor))
	if err != nil {
		return nil, "", err
	}
	now := time.Now().UTC()
	active := make([]*models.DraftReviewGrant, 0, len(grants))
	for _, grant := range grants {
		if grant != nil && grant.IsActive(now) {
			active = append(active, grant)
		}
	}
	return active, nextCursor, nil
}

// CountSharedDraftReviews returns the full active queue size, applying the same
// active-grant predicate as SharedDraftReviews so the reported count can never
// exceed the edges the list would return for the same reviewer.
func (s *DraftService) CountSharedDraftReviews(ctx context.Context, reviewer string) (int, error) {
	repo, err := s.reviewRepository()
	if err != nil {
		return 0, err
	}
	now := time.Now().UTC()
	total := 0
	cursor := ""
	for {
		grants, nextCursor, listErr := repo.ListActiveDraftReviewGrants(ctx, strings.TrimSpace(reviewer), maxDraftReviewReadGrants, cursor)
		if listErr != nil {
			return 0, listErr
		}
		for _, grant := range grants {
			if grant != nil && grant.IsActive(now) {
				total++
			}
		}
		if nextCursor == "" {
			break
		}
		if nextCursor == cursor {
			return 0, errors.New("draft review pagination did not advance")
		}
		cursor = nextCursor
	}
	return total, nil
}

// OwnedDraftReviews returns active review assignments created by one draft owner.
// The complete active set is returned so GraphQL can filter before paginating and report an exact count.
func (s *DraftService) OwnedDraftReviews(ctx context.Context, owner string) ([]*models.DraftReviewGrant, error) {
	repo, ok := s.draftRepo.(ownedDraftReviewRepository)
	if !ok || repo == nil {
		return nil, errDraftReviewStorageUnavailable
	}
	grants, err := repo.ListDraftReviewGrantsByOwner(ctx, strings.TrimSpace(owner))
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	active := make([]*models.DraftReviewGrant, 0, len(grants))
	for _, grant := range grants {
		if grant != nil && grant.IsActive(now) {
			active = append(active, grant)
		}
	}
	sort.Slice(active, func(i, j int) bool { return active[i].SK < active[j].SK })
	return active, nil
}

// DraftReviewForCaller resolves a draft only for its owner or active reviewer.
func (s *DraftService) DraftReviewForCaller(ctx context.Context, caller, draftID string) (*models.Draft, *models.DraftReviewGrant, error) {
	caller = strings.TrimSpace(caller)
	draftID = strings.TrimSpace(draftID)
	if d, e := s.draftRepo.GetDraft(ctx, caller, draftID); e == nil {
		// Owners ordinarily have no grant, except the explicit principal-owner
		// approval flow for generated drafts.
		if g, grantErr := s.ActiveDraftReviewGrant(ctx, caller, draftID, caller); grantErr == nil {
			return d, g, nil
		}
		return d, nil, nil
	}
	cursor := ""
	for {
		grants, nextCursor, e := s.SharedDraftReviews(ctx, caller, 200, cursor)
		if e != nil {
			return nil, nil, e
		}
		for _, g := range grants {
			if g != nil && g.DraftID == draftID {
				d, getErr := s.draftRepo.GetDraft(ctx, g.OwnerID, draftID)
				return d, g, getErr
			}
		}
		if nextCursor == "" {
			break
		}
		if nextCursor == cursor {
			return nil, nil, errors.New("draft review pagination did not advance")
		}
		cursor = nextCursor
	}
	return nil, nil, errors.New("draft review not found")
}

// DraftEditorialMediaForCaller returns only assets bound to the authorized
// draft. Review grants never turn into a general media-library capability.
func (s *DraftService) DraftEditorialMediaForCaller(ctx context.Context, caller, draftID string) (*models.Draft, []DraftEditorialMediaBinding, error) {
	draft, _, err := s.DraftReviewForCaller(ctx, caller, draftID)
	if err != nil {
		return nil, nil, err
	}
	if s.mediaRepo == nil {
		return nil, nil, errors.New("editorial media repository is unavailable")
	}
	bindings := make([]DraftEditorialMediaBinding, 0, len(draft.EditorialMedia))
	for _, usage := range draft.EditorialMedia {
		media, getErr := s.mediaRepo.GetMedia(ctx, usage.MediaID)
		if getErr != nil && (errors.Is(getErr, storage.ErrNotFound) || apperrors.HasCode(getErr, apperrors.CodeNotFound)) {
			bindings = append(bindings, DraftEditorialMediaBinding{Usage: usage})
			continue
		}
		if getErr != nil {
			return nil, nil, getErr
		}
		if media == nil || !strings.EqualFold(strings.TrimSpace(media.UserID), strings.TrimSpace(draft.AuthorID)) {
			bindings = append(bindings, DraftEditorialMediaBinding{Usage: usage})
			continue
		}
		bindings = append(bindings, DraftEditorialMediaBinding{Usage: usage, Media: media})
	}
	return draft, bindings, nil
}

// BoundEditorialMediaForCaller authorizes one exact asset against an owner or
// active reviewer grant. It intentionally cannot authorize unbound media.
func (s *DraftService) BoundEditorialMediaForCaller(ctx context.Context, caller, draftID, mediaID string) (*models.Media, error) {
	_, bindings, err := s.DraftEditorialMediaForCaller(ctx, caller, draftID)
	if err != nil {
		return nil, err
	}
	mediaID = strings.TrimSpace(mediaID)
	for _, binding := range bindings {
		if binding.Usage.MediaID != mediaID {
			continue
		}
		if binding.Media == nil || !binding.Media.IsInternalEditorial() {
			return nil, errors.New("bound editorial media is unavailable")
		}
		return binding.Media, nil
	}
	return nil, errors.New("editorial media is not bound to this draft")
}

// SubmitDraftReview records an immutable reviewer verdict.
func (s *DraftService) SubmitDraftReview(ctx context.Context, caller, owner, draftID, verdict, notes string) (*models.DraftReviewVerdict, error) {
	caller = strings.TrimSpace(caller)
	owner = strings.TrimSpace(owner)
	if caller == owner {
		principal, err := s.instancePrincipal(ctx)
		if err != nil || principal != owner {
			return nil, errors.New("draft owner cannot review their own draft")
		}
	}
	if _, err := s.ActiveDraftReviewGrant(ctx, owner, draftID, caller); err != nil {
		return nil, err
	}
	verdict = strings.ToUpper(strings.TrimSpace(verdict))
	if verdict != DraftReviewApproved && verdict != DraftReviewChangesRequested {
		return nil, errors.New("invalid draft review verdict")
	}
	repo, err := s.reviewRepository()
	if err != nil {
		return nil, err
	}
	fieldUpdater, ok := s.draftRepo.(draftReviewFieldUpdater)
	if !ok || fieldUpdater == nil {
		return nil, errDraftReviewStorageUnavailable
	}
	d, err := s.draftRepo.GetDraft(ctx, owner, draftID)
	if err != nil {
		return nil, err
	}
	contentHash, err := s.draftContentHash(ctx, d)
	if err != nil {
		return nil, err
	}
	v := &models.DraftReviewVerdict{
		OwnerID:     owner,
		DraftID:     strings.TrimSpace(draftID),
		Reviewer:    caller,
		Verdict:     verdict,
		Notes:       strings.TrimSpace(notes),
		ContentHash: contentHash,
		RecordedAt:  time.Now().UTC(),
	}
	if err := repo.CreateDraftReviewVerdict(ctx, v); err != nil {
		return nil, err
	}
	d.ReviewedBy = caller
	d.ReviewStatus = verdict
	d.EditorNotes = v.Notes
	if err := fieldUpdater.UpdateDraftReviewFields(ctx, owner, d); err != nil {
		return nil, err
	}
	return v, nil
}

// DraftReviewVerdicts lists ordered verdict history for a draft.
func (s *DraftService) DraftReviewVerdicts(ctx context.Context, owner, draftID string) ([]*models.DraftReviewVerdict, error) {
	repo, err := s.reviewRepository()
	if err != nil {
		return nil, err
	}
	return repo.ListDraftReviewVerdicts(ctx, owner, draftID)
}
func (s *DraftService) instancePrincipal(ctx context.Context) (string, error) {
	if s.principalUsername == nil {
		return "", ErrInstancePrincipalUnavailable
	}
	principal, err := s.principalUsername(ctx)
	if err != nil {
		return "", errors.Join(ErrInstancePrincipalUnavailable, err)
	}
	principal = strings.TrimSpace(principal)
	if principal == "" {
		return "", ErrInstancePrincipalNotConfigured
	}
	return principal, nil
}

type draftReviewApprovalState struct {
	active       map[string]*models.DraftReviewGrant
	latest       map[string]*models.DraftReviewVerdict
	contentHash  string
	mediaDigests map[string]string
	mediaReasons []string
}

// DraftReviewReadState is the complete, revision-bound review state exposed to
// authorized clients. CurrentVerdicts contains only verdicts that apply to the
// present draft digest and were recorded after the active grant.
type DraftReviewReadState struct {
	ContentHash               string
	Grants                    []*models.DraftReviewGrant
	GrantCount                int
	GrantsTruncated           bool
	CurrentVerdicts           map[string]*models.DraftReviewVerdict
	ReviewersApproved         bool
	PrincipalApprovalRequired bool
	PrincipalApproved         bool
	PublishEligible           bool
	BlockingReasons           []string
}

// DraftReviewState returns review grants, current approvals, and the same
// eligibility decision enforced by publish and schedule operations.
func (s *DraftService) DraftReviewState(ctx context.Context, owner, draftID string, draft *models.Draft) (*DraftReviewReadState, error) {
	if draft == nil {
		var err error
		draft, err = s.draftRepo.GetDraft(ctx, owner, draftID)
		if err != nil {
			return nil, err
		}
	}
	repo, err := s.reviewRepository()
	if err != nil {
		return nil, err
	}
	grants, err := repo.ListDraftReviewGrants(ctx, owner, draftID)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	sort.SliceStable(grants, func(i, j int) bool {
		leftActive := grants[i] != nil && grants[i].IsActive(now)
		rightActive := grants[j] != nil && grants[j].IsActive(now)
		if leftActive != rightActive {
			return leftActive
		}
		if grants[i] == nil || grants[j] == nil {
			return grants[i] != nil
		}
		return grants[i].GrantedAt.After(grants[j].GrantedAt)
	})
	grantCount := len(grants)
	grantsTruncated := grantCount > maxDraftReviewReadGrants
	if grantsTruncated {
		grants = grants[:maxDraftReviewReadGrants]
	}
	approval, err := s.draftReviewApprovalState(ctx, owner, draftID, draft)
	if err != nil {
		return nil, err
	}

	reviewersApproved := allActiveReviewersApproved(approval)
	principalRequired := strings.TrimSpace(draft.GeneratedBy) != ""
	principalApproved := false
	principalUnavailable := false
	if principalRequired {
		principal, principalErr := s.instancePrincipal(ctx)
		if principalErr != nil {
			principalUnavailable = true
		} else {
			verdict := approval.latest[principal]
			principalApproved = approval.active[principal] != nil && verdict != nil && verdict.Verdict == DraftReviewApproved
		}
	}

	blockingReasons := make([]string, 0, 4)
	if !reviewersApproved {
		blockingReasons = append(blockingReasons, "REVIEW_APPROVAL_REQUIRED")
	}
	if principalUnavailable {
		blockingReasons = append(blockingReasons, "PRINCIPAL_APPROVAL_UNAVAILABLE")
	} else if principalRequired && !principalApproved {
		blockingReasons = append(blockingReasons, "PRINCIPAL_APPROVAL_REQUIRED")
	}
	blockingReasons = append(blockingReasons, approval.mediaReasons...)
	return &DraftReviewReadState{
		ContentHash:               approval.contentHash,
		Grants:                    grants,
		GrantCount:                grantCount,
		GrantsTruncated:           grantsTruncated,
		CurrentVerdicts:           approval.latest,
		ReviewersApproved:         reviewersApproved,
		PrincipalApprovalRequired: principalRequired,
		PrincipalApproved:         principalApproved,
		PublishEligible:           len(blockingReasons) == 0,
		BlockingReasons:           blockingReasons,
	}, nil
}

func (s *DraftService) draftReviewApprovalState(ctx context.Context, owner, draftID string, draft *models.Draft) (*draftReviewApprovalState, error) {
	repo, err := s.reviewRepository()
	if err != nil {
		return nil, err
	}
	grants, err := repo.ListDraftReviewGrants(ctx, owner, draftID)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	active := make(map[string]*models.DraftReviewGrant, len(grants))
	for _, grant := range grants {
		if grant != nil && grant.IsActive(now) {
			active[grant.Reviewer] = grant
		}
	}
	if len(active) == 0 {
		// Public approval helpers may not already have the draft snapshot. Fetch
		// only when the caller supplied one: with no grants the approval answer
		// is vacuously true and the hash is only relevant to read-state callers.
		if draft == nil {
			return &draftReviewApprovalState{active: active, latest: map[string]*models.DraftReviewVerdict{}}, nil
		}
		digests, reasons, resolveErr := s.resolveDraftMediaBindings(ctx, draft)
		if resolveErr != nil {
			return nil, resolveErr
		}
		return &draftReviewApprovalState{
			active:       active,
			latest:       map[string]*models.DraftReviewVerdict{},
			contentHash:  draftReviewContentHash(draft, digests),
			mediaDigests: digests,
			mediaReasons: reasons,
		}, nil
	}
	verdicts, err := repo.ListDraftReviewVerdicts(ctx, owner, draftID)
	if err != nil {
		return nil, err
	}
	if draft == nil {
		draft, err = s.draftRepo.GetDraft(ctx, owner, draftID)
		if err != nil {
			return nil, err
		}
	}
	digests, reasons, resolveErr := s.resolveDraftMediaBindings(ctx, draft)
	if resolveErr != nil {
		return nil, resolveErr
	}
	currentHash := draftReviewContentHash(draft, digests)
	latest := make(map[string]*models.DraftReviewVerdict, len(active))
	for _, verdict := range verdicts {
		if verdict == nil {
			continue
		}
		grant := active[verdict.Reviewer]
		// A re-grant deliberately requires a verdict recorded after the new grant.
		// Pre-deploy approvals have no hash and deliberately revert to unapproved;
		// the fail-closed/no-backfill rollout requires re-review.
		if grant != nil && verdict.RecordedAt.After(grant.GrantedAt) &&
			verdict.ContentHash == currentHash {
			if current := latest[verdict.Reviewer]; current == nil || verdict.RecordedAt.After(current.RecordedAt) {
				latest[verdict.Reviewer] = verdict
			}
		}
	}
	return &draftReviewApprovalState{active: active, latest: latest, contentHash: currentHash, mediaDigests: digests, mediaReasons: reasons}, nil
}

func allActiveReviewersApproved(state *draftReviewApprovalState) bool {
	if state == nil {
		return false
	}
	for reviewer := range state.active {
		if verdict := state.latest[reviewer]; verdict == nil || verdict.Verdict != DraftReviewApproved {
			return false
		}
	}
	return true
}

// draftReviewGateApprovals derives the approval snapshot once for the publish
// and schedule gates, then answers both the unanimous-reviewer and conditional
// principal requirements from that same snapshot. The derived state carries the
// exact text-and-media hash the approvals were matched against, so the publish
// path can mint durable serving for exactly those bytes.
func (s *DraftService) draftReviewGateApprovals(ctx context.Context, owner, draftID string, draft *models.Draft) (bool, bool, *draftReviewApprovalState, error) {
	if draft == nil {
		// Callers without a snapshot fetch it exactly once before deriving both
		// approval answers from that same content.
		var err error
		draft, err = s.draftRepo.GetDraft(ctx, owner, draftID)
		if err != nil {
			return false, false, nil, err
		}
	}
	state, err := s.draftReviewApprovalState(ctx, owner, draftID, draft)
	if err != nil {
		if !errors.Is(err, errDraftReviewStorageUnavailable) {
			return false, false, nil, err
		}
		if strings.TrimSpace(draft.GeneratedBy) == "" {
			// A repository without review support cannot contain active grants;
			// preserve the pre-review behavior for human-authored drafts while
			// still deriving the media surface for the publish gate.
			digests, reasons, mediaErr := s.resolveDraftMediaBindings(ctx, draft)
			if mediaErr != nil {
				return false, false, nil, mediaErr
			}
			emptyState := &draftReviewApprovalState{
				active:       map[string]*models.DraftReviewGrant{},
				latest:       map[string]*models.DraftReviewVerdict{},
				contentHash:  draftReviewContentHash(draft, digests),
				mediaDigests: digests,
				mediaReasons: reasons,
			}
			return true, true, emptyState, nil
		}
		// Preserve the generated-draft error ordering: principal resolution ran
		// before its independent approval-state derivation on the former path.
		if _, principalErr := s.instancePrincipal(ctx); principalErr != nil {
			return false, false, nil, principalErr
		}
		return false, false, nil, err
	}
	if !allActiveReviewersApproved(state) {
		return false, false, state, nil
	}
	if strings.TrimSpace(draft.GeneratedBy) == "" {
		return true, true, state, nil
	}
	principal, err := s.instancePrincipal(ctx)
	if err != nil {
		return false, false, state, err
	}
	grant := state.active[principal]
	verdict := state.latest[principal]
	return true, grant != nil && verdict != nil && verdict.Verdict == DraftReviewApproved, state, nil
}

// HasUnanimousActiveApproval applies the all-invited-reviewers rule. With no
// active grants the result is vacuously true, preserving human draft behavior.
func (s *DraftService) HasUnanimousActiveApproval(ctx context.Context, owner, draftID string) (bool, error) {
	return s.hasUnanimousActiveApproval(ctx, owner, draftID, nil)
}

func (s *DraftService) hasUnanimousActiveApproval(ctx context.Context, owner, draftID string, draft *models.Draft) (bool, error) {
	state, err := s.draftReviewApprovalState(ctx, owner, draftID, draft)
	if err != nil {
		if errors.Is(err, errDraftReviewStorageUnavailable) {
			// A repository without review support cannot contain active grants;
			// preserve the pre-review behavior for human-authored drafts.
			return true, nil
		}
		return false, err
	}
	return allActiveReviewersApproved(state), nil
}

// HasPrincipalApproval applies the additional generated-content principal rule.
func (s *DraftService) HasPrincipalApproval(ctx context.Context, owner, draftID string) (bool, error) {
	return s.hasPrincipalApproval(ctx, owner, draftID, nil)
}

func (s *DraftService) hasPrincipalApproval(ctx context.Context, owner, draftID string, draft *models.Draft) (bool, error) {
	principal, err := s.instancePrincipal(ctx)
	if err != nil {
		return false, err
	}
	state, err := s.draftReviewApprovalState(ctx, owner, draftID, draft)
	if err != nil {
		return false, err
	}
	grant := state.active[principal]
	verdict := state.latest[principal]
	return grant != nil && verdict != nil && verdict.Verdict == DraftReviewApproved, nil
}

// HasActiveApproval preserves the combined generated-draft gate for callers
// that need both unanimous active-reviewer and principal approval.
func (s *DraftService) HasActiveApproval(ctx context.Context, owner, draftID string) (bool, error) {
	state, err := s.draftReviewApprovalState(ctx, owner, draftID, nil)
	if err != nil {
		return false, err
	}
	if !allActiveReviewersApproved(state) {
		return false, nil
	}
	principal, err := s.instancePrincipal(ctx)
	if err != nil {
		return false, err
	}
	grant := state.active[principal]
	verdict := state.latest[principal]
	return grant != nil && verdict != nil && verdict.Verdict == DraftReviewApproved, nil
}

package cms

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"math"
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
// and never clears it; it describes authorship, not reviewed content, so it
// must never stale a verdict.
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
		// InlinePosition is schema-validated non-negative; hero and social-card
		// usages carry no position and encode as the MaxUint64 sentinel that the
		// legacy int64(-1) conversion produced, so existing revision hashes stay
		// byte-identical. A negative position (only reachable from corrupt data)
		// fails closed to the same sentinel instead of a wrapping conversion.
		position := uint64(math.MaxUint64)
		if usage.InlinePosition != nil && *usage.InlinePosition >= 0 {
			position = uint64(*usage.InlinePosition)
		}
		var positionBytes [8]byte
		binary.BigEndian.PutUint64(positionBytes[:], position)
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
	// ErrDraftReviewApprovalRequired means a required reviewer is missing a
	// current approval for the exact reviewed content. Required reviewers are
	// holders of an active grant plus every reviewer who ever recorded a verdict
	// (operator doctrine, "requested = required").
	ErrDraftReviewApprovalRequired = errors.New("draft requires approval from every required reviewer")
	// ErrDraftReviewPrincipalApprovalRequired means the operator doctrine
	// principal floor is unmet: the releasing actor is not the instance
	// principal, and the principal does not hold a current approving verdict
	// (regardless of draft provenance).
	ErrDraftReviewPrincipalApprovalRequired = errors.New("draft release requires an active approval from the instance principal")
	// ErrDraftReviewMediaRequired means a required bound asset cannot serve the
	// exact approved bytes (missing, not ready, withdrawn, superseded, or
	// unavailable). The wrapped message names the blocking reasons.
	ErrDraftReviewMediaRequired = errors.New("draft requires its bound media to be ready and available")
	// ErrDraftReviewConflict is the additive conflict signal surfaced when a
	// review submit carries an expected content hash that no longer matches the
	// stored draft; the caller re-reads and retries.
	ErrDraftReviewConflict = errors.New("draft changed concurrently")
	// ErrDraftReviewReviewContentChanged is the additive conflict signal surfaced
	// when a review submit carries an expected content hash that no longer
	// matches the stored draft: the owner edited after the reviewer inspected
	// the draft, so the verdict must not bless unseen content.
	ErrDraftReviewReviewContentChanged = errors.New("draft content changed since the reviewer inspected it")
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
		if err != nil || !sameAccount(principal, owner) {
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

// SubmitDraftReview records an immutable reviewer verdict bound to the exact
// draft content hash. The caller MAY carry the expectedContentHash it actually
// inspected: an empty value is the legacy no-constraint path (the draft surface
// has deployed consumers that submit without a hash, so the argument defaults
// to empty and the advisory binding is closed at the client contract rather
// than the server). When a non-empty expected hash no longer matches the stored
// draft (the owner edited between the reviewer's read and this submit), the
// submit is rejected with a conflict signal instead of silently blessing unseen
// content.
func (s *DraftService) SubmitDraftReview(ctx context.Context, caller, owner, draftID, verdict, notes string, expectedContentHashes ...string) (*models.DraftReviewVerdict, error) {
	caller = strings.TrimSpace(caller)
	owner = strings.TrimSpace(owner)
	if caller == owner {
		principal, err := s.instancePrincipal(ctx)
		if err != nil || !sameAccount(principal, owner) {
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
	expectedContentHash := ""
	if len(expectedContentHashes) > 0 {
		expectedContentHash = strings.TrimSpace(expectedContentHashes[0])
	}
	if expectedContentHash != "" && expectedContentHash != contentHash {
		return nil, errors.Join(ErrDraftReviewConflict, ErrDraftReviewReviewContentChanged)
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

// instancePrincipal resolves the operator doctrine's designated principal
// account in its canonical lowercase form. PrimaryAdminUsername is persisted
// TrimSpace-only, so the configured principal string can carry casing that the
// reviewer strings recorded at share/submit time do not — and the approval
// state maps are keyed byte-wise by those recorded strings. Canonicalizing here
// is the single choke point (both the draft and promo surfaces resolve through
// this function): every downstream principal lookup — the byte-keyed
// state.active/state.latest floor access and the sameAccount comparisons — sees
// the same lowercase form, so a real principal approval recorded under the
// canonical identity resolves regardless of the casing the operator configured.
func (s *DraftService) instancePrincipal(ctx context.Context) (string, error) {
	if s.principalUsername == nil {
		return "", ErrInstancePrincipalUnavailable
	}
	principal, err := s.principalUsername(ctx)
	if err != nil {
		return "", errors.Join(ErrInstancePrincipalUnavailable, err)
	}
	principal = strings.ToLower(strings.TrimSpace(principal))
	if principal == "" {
		return "", ErrInstancePrincipalNotConfigured
	}
	return principal, nil
}

type draftReviewApprovalState struct {
	// active maps every currently active grant (not revoked, not expired) to its
	// grant record.
	active map[string]*models.DraftReviewGrant
	// required maps every reviewer whose approval the publish/schedule gate
	// demands: holders of an active grant plus every reviewer who EVER recorded a
	// verdict — revocation or expiry cannot delete a required approval
	// (operator doctrine, "requested = required").
	required     map[string]*models.DraftReviewGrant
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

	// The owner is the presumed releaser when rendering the read state; the
	// doctrine answers are derived for that actor (a principal owner releases
	// with implicit approval, a non-principal owner triggers the principal
	// floor).
	principal, principalErr := s.instancePrincipal(ctx)
	principalUnavailable := principalErr != nil
	principalApproved := false
	principalRequired := principalUnavailable || !sameAccount(owner, principal)
	var reviewersApproved bool
	if principalUnavailable {
		reviewersApproved = allRequiredReviewersApproved(approval)
	} else {
		reviewersApproved, principalApproved = draftApprovalAnswers(approval, owner, principal)
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
	verdicts, listErr := repo.ListDraftReviewVerdicts(ctx, owner, draftID)
	if listErr != nil {
		return nil, listErr
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
	active, required := draftRequiredReviewers(grants, verdicts, time.Now().UTC())
	latest := draftCurrentVerdicts(verdicts, required, currentHash)
	return &draftReviewApprovalState{
		active:       active,
		required:     required,
		latest:       latest,
		contentHash:  currentHash,
		mediaDigests: digests,
		mediaReasons: reasons,
	}, nil
}

// draftRequiredReviewers derives the doctrine "requested = required" reviewer
// set: every reviewer with an active grant, plus every reviewer who ever
// recorded a verdict — revocation or expiry cannot delete a required approval.
// Each required reviewer is mapped to their grant record so the current verdict
// can be dated against it (a re-grant requires a verdict recorded after the new
// grant).
//
//nolint:dupl // the required-reviewer derivation mirrors the promo review surface (M4 issue #1446); the record types differ
func draftRequiredReviewers(grants []*models.DraftReviewGrant, verdicts []*models.DraftReviewVerdict, now time.Time) (active, required map[string]*models.DraftReviewGrant) {
	active = make(map[string]*models.DraftReviewGrant, len(grants))
	grantByReviewer := make(map[string]*models.DraftReviewGrant, len(grants))
	for _, grant := range grants {
		if grant == nil || strings.TrimSpace(grant.Reviewer) == "" {
			continue
		}
		grantByReviewer[grant.Reviewer] = grant
		if grant.IsActive(now) {
			active[grant.Reviewer] = grant
		}
	}
	required = make(map[string]*models.DraftReviewGrant, len(grantByReviewer))
	for reviewer, grant := range grantByReviewer {
		if _, ok := active[reviewer]; ok {
			required[reviewer] = grant
		}
	}
	for _, verdict := range verdicts {
		if verdict == nil || strings.TrimSpace(verdict.Reviewer) == "" {
			continue
		}
		if _, ok := required[verdict.Reviewer]; !ok {
			if grant := grantByReviewer[verdict.Reviewer]; grant != nil {
				required[verdict.Reviewer] = grant
			}
		}
	}
	return active, required
}

// draftCurrentVerdicts derives, per required reviewer, the latest verdict that
// is current for the exact reviewed content: hash-matching and recorded after
// the reviewer's latest grant. Pre-deploy approvals have no hash and
// deliberately revert to unapproved; the fail-closed/no-backfill rollout
// requires re-review.
func draftCurrentVerdicts(verdicts []*models.DraftReviewVerdict, required map[string]*models.DraftReviewGrant, currentHash string) map[string]*models.DraftReviewVerdict {
	latest := make(map[string]*models.DraftReviewVerdict, len(required))
	for _, verdict := range verdicts {
		if verdict == nil {
			continue
		}
		grant := required[verdict.Reviewer]
		// A re-grant deliberately requires a verdict recorded after the new grant.
		if grant != nil && verdict.RecordedAt.After(grant.GrantedAt) &&
			verdict.ContentHash == currentHash {
			if current := latest[verdict.Reviewer]; current == nil || verdict.RecordedAt.After(current.RecordedAt) {
				latest[verdict.Reviewer] = verdict
			}
		}
	}
	return latest
}

// allRequiredReviewersApproved reports whether every required reviewer (active
// grants plus ever-recorded-verdict reviewers) holds a current approving
// verdict for the exact reviewed content.
func allRequiredReviewersApproved(state *draftReviewApprovalState) bool {
	if state == nil {
		return false
	}
	for reviewer := range state.required {
		if verdict := state.latest[reviewer]; verdict == nil || verdict.Verdict != DraftReviewApproved {
			return false
		}
	}
	return true
}

// draftApprovalAnswers applies the operator doctrine matrix to an already
// derived approval snapshot for one releasing actor. When the releaser IS the
// principal, their own requirement is implicit (their action is the approval)
// and only other required reviewers must approve; for a non-principal releaser
// the principal floor demands an active grant with a current approving verdict,
// regardless of draft provenance.
func draftApprovalAnswers(state *draftReviewApprovalState, releaser, principal string) (reviewersApproved, principalApproved bool) {
	required := state.required
	if sameAccount(releaser, principal) {
		required = draftRequiredWithoutPrincipal(state.required, principal)
	}
	for reviewer := range required {
		if verdict := state.latest[reviewer]; verdict == nil || verdict.Verdict != DraftReviewApproved {
			return false, false
		}
	}
	if sameAccount(releaser, principal) {
		return true, true
	}
	grant := state.active[principal]
	verdict := state.latest[principal]
	return true, grant != nil && verdict != nil && verdict.Verdict == DraftReviewApproved
}

// draftRequiredWithoutPrincipal returns the required set with the principal's
// own requirement dropped (their release action is the implicit approval).
func draftRequiredWithoutPrincipal(required map[string]*models.DraftReviewGrant, principal string) map[string]*models.DraftReviewGrant {
	if required == nil {
		return nil
	}
	out := make(map[string]*models.DraftReviewGrant, len(required))
	for reviewer, grant := range required {
		if sameAccount(reviewer, principal) {
			continue
		}
		out[reviewer] = grant
	}
	return out
}

// draftReviewGateApprovals derives the approval snapshot once for the publish
// and schedule gates, then answers the reviewer-approval and principal-approval
// requirements of the operator doctrine from that same snapshot: every required
// reviewer (active grants plus ever-recorded-verdict reviewers — revocation
// cannot delete a required approval) must hold a current approving verdict, and
// the principal is required for any non-principal release regardless of draft
// provenance. The derived state carries the exact text-and-media hash the
// approvals were matched against, so the publish path can mint durable serving
// for exactly those bytes.
func (s *DraftService) draftReviewGateApprovals(ctx context.Context, releaser, owner, draftID string, draft *models.Draft) (bool, bool, *draftReviewApprovalState, error) {
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
		// A repository without review support cannot contain grants or verdicts,
		// so the required-reviewer set is empty. The principal floor still
		// applies: only the instance principal may release without a current
		// principal approval (a non-principal releaser has no way to record one
		// here and fails closed).
		digests, reasons, mediaErr := s.resolveDraftMediaBindings(ctx, draft)
		if mediaErr != nil {
			return false, false, nil, mediaErr
		}
		emptyState := &draftReviewApprovalState{
			active:       map[string]*models.DraftReviewGrant{},
			required:     map[string]*models.DraftReviewGrant{},
			latest:       map[string]*models.DraftReviewVerdict{},
			contentHash:  draftReviewContentHash(draft, digests),
			mediaDigests: digests,
			mediaReasons: reasons,
		}
		principal, principalErr := s.instancePrincipal(ctx)
		if principalErr != nil {
			return false, false, emptyState, principalErr
		}
		return true, sameAccount(releaser, principal), emptyState, nil
	}
	principal, err := s.instancePrincipal(ctx)
	if err != nil {
		return false, false, state, err
	}
	reviewersApproved, principalApproved := draftApprovalAnswers(state, strings.TrimSpace(releaser), principal)
	return reviewersApproved, principalApproved, state, nil
}

// HasUnanimousActiveApproval applies the requested = required rule: every
// reviewer with an active grant plus every reviewer who ever recorded a verdict
// must hold a current approving verdict. With no grants and no verdicts the
// result is vacuously true, preserving human draft behavior.
func (s *DraftService) HasUnanimousActiveApproval(ctx context.Context, owner, draftID string) (bool, error) {
	return s.hasUnanimousActiveApproval(ctx, owner, draftID, nil)
}

func (s *DraftService) hasUnanimousActiveApproval(ctx context.Context, owner, draftID string, draft *models.Draft) (bool, error) {
	state, err := s.draftReviewApprovalState(ctx, owner, draftID, draft)
	if err != nil {
		if errors.Is(err, errDraftReviewStorageUnavailable) {
			// A repository without review support cannot contain grants or
			// verdicts; the required set is empty and the answer is vacuously
			// true.
			return true, nil
		}
		return false, err
	}
	return allRequiredReviewersApproved(state), nil
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

// HasActiveApproval preserves the combined non-principal-releaser gate for
// callers that need both required-reviewer and principal approval.
func (s *DraftService) HasActiveApproval(ctx context.Context, owner, draftID string) (bool, error) {
	state, err := s.draftReviewApprovalState(ctx, owner, draftID, nil)
	if err != nil {
		return false, err
	}
	if !allRequiredReviewersApproved(state) {
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

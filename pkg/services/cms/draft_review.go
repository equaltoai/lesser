package cms

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
)

// Draft review verdict values.
const (
	DraftReviewApproved         = "APPROVED"
	DraftReviewChangesRequested = "CHANGES_REQUESTED"
)

type draftReviewRepository interface {
	PutDraftReviewGrant(context.Context, *models.DraftReviewGrant) error
	GetDraftReviewGrant(context.Context, string, string, string) (*models.DraftReviewGrant, error)
	ListActiveDraftReviewGrants(context.Context, string, int) ([]*models.DraftReviewGrant, error)
	ListDraftReviewGrants(context.Context, string, string) ([]*models.DraftReviewGrant, error)
	CreateDraftReviewVerdict(context.Context, *models.DraftReviewVerdict) error
	ListDraftReviewVerdicts(context.Context, string, string) ([]*models.DraftReviewVerdict, error)
}

func (s *DraftService) reviewRepository() (draftReviewRepository, error) {
	repo, ok := s.draftRepo.(draftReviewRepository)
	if !ok || repo == nil {
		return nil, errors.New("draft review storage is not available")
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
	g := &models.DraftReviewGrant{OwnerID: owner, DraftID: draftID, Reviewer: reviewer, GrantedAt: now}
	if existing, e := repo.GetDraftReviewGrant(ctx, owner, draftID, reviewer); e == nil && existing != nil {
		g.Version = existing.Version
		g.RevokedAt = nil
	}
	if err := repo.PutDraftReviewGrant(ctx, g); err != nil {
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
	return repo.PutDraftReviewGrant(ctx, g)
}

// ActiveDraftReviewGrant returns a non-revoked grant for one reviewer.
func (s *DraftService) ActiveDraftReviewGrant(ctx context.Context, owner, draftID, reviewer string) (*models.DraftReviewGrant, error) {
	repo, err := s.reviewRepository()
	if err != nil {
		return nil, err
	}
	g, err := repo.GetDraftReviewGrant(ctx, owner, draftID, reviewer)
	if err != nil {
		return nil, err
	}
	if g.RevokedAt != nil {
		return nil, errors.New("draft review grant is not active")
	}
	return g, nil
}

// SharedDraftReviews lists active review queue grants for a reviewer.
func (s *DraftService) SharedDraftReviews(ctx context.Context, reviewer string, limit int) ([]*models.DraftReviewGrant, error) {
	repo, err := s.reviewRepository()
	if err != nil {
		return nil, err
	}
	return repo.ListActiveDraftReviewGrants(ctx, strings.TrimSpace(reviewer), limit)
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
	grants, e := s.SharedDraftReviews(ctx, caller, 200)
	if e != nil {
		return nil, nil, e
	}
	for _, g := range grants {
		if g != nil && g.DraftID == draftID {
			d, e := s.draftRepo.GetDraft(ctx, g.OwnerID, draftID)
			return d, g, e
		}
	}
	return nil, nil, errors.New("draft review not found")
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
	v := &models.DraftReviewVerdict{OwnerID: owner, DraftID: strings.TrimSpace(draftID), Reviewer: caller, Verdict: verdict, Notes: strings.TrimSpace(notes), RecordedAt: time.Now().UTC()}
	if err := repo.CreateDraftReviewVerdict(ctx, v); err != nil {
		return nil, err
	}
	d, err := s.draftRepo.GetDraft(ctx, owner, draftID)
	if err != nil {
		return nil, err
	}
	d.ReviewedBy = caller
	d.ReviewStatus = verdict
	d.EditorNotes = v.Notes
	if err := s.draftRepo.UpdateDraft(ctx, owner, d); err != nil {
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
		return "", errors.New("instance principal is unavailable")
	}
	principal, err := s.principalUsername(ctx)
	if err != nil {
		return "", err
	}
	principal = strings.TrimSpace(principal)
	if principal == "" {
		return "", errors.New("instance principal is not configured")
	}
	return principal, nil
}

// HasActiveApproval enforces the generated-draft publication gate. Every active
// grant needs a current approval, and the instance principal must be one of them.
func (s *DraftService) HasActiveApproval(ctx context.Context, owner, draftID string) (bool, error) {
	principal, err := s.instancePrincipal(ctx)
	if err != nil {
		return false, err
	}
	repo, err := s.reviewRepository()
	if err != nil {
		return false, err
	}
	grants, err := repo.ListDraftReviewGrants(ctx, owner, draftID)
	if err != nil {
		return false, err
	}
	active := make(map[string]*models.DraftReviewGrant, len(grants))
	for _, grant := range grants {
		if grant != nil && grant.RevokedAt == nil {
			active[grant.Reviewer] = grant
		}
	}
	if len(active) == 0 || active[principal] == nil {
		return false, nil
	}
	verdicts, err := repo.ListDraftReviewVerdicts(ctx, owner, draftID)
	if err != nil {
		return false, err
	}
	latest := make(map[string]*models.DraftReviewVerdict, len(active))
	for _, verdict := range verdicts {
		grant := active[verdict.Reviewer]
		// A re-grant deliberately requires a verdict recorded after the new grant.
		if grant != nil && verdict.RecordedAt.After(grant.GrantedAt) {
			if current := latest[verdict.Reviewer]; current == nil || verdict.RecordedAt.After(current.RecordedAt) {
				latest[verdict.Reviewer] = verdict
			}
		}
	}
	for reviewer := range active {
		if verdict := latest[reviewer]; verdict == nil || verdict.Verdict != DraftReviewApproved {
			return false, nil
		}
	}
	return true, nil
}

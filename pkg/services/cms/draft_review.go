package cms

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
)

// Draft review verdict values.
const (
	DraftReviewApproved         = "APPROVED"
	DraftReviewChangesRequested = "CHANGES_REQUESTED"
)

var (
	// ErrDraftReviewApprovalRequired means an active reviewer is missing a current approval.
	ErrDraftReviewApprovalRequired = errors.New("draft requires approval from every active reviewer")
	// ErrDraftReviewPrincipalApprovalRequired means the designated principal is missing a current approval.
	ErrDraftReviewPrincipalApprovalRequired = errors.New("generated draft requires an active approval from the instance principal")
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
	g := &models.DraftReviewGrant{OwnerID: owner, DraftID: draftID, Reviewer: reviewer, GrantedAt: now}
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

// SharedDraftReviews lists one cursor page of active review queue grants.
func (s *DraftService) SharedDraftReviews(ctx context.Context, reviewer string, limit int, cursor string) ([]*models.DraftReviewGrant, string, error) {
	repo, err := s.reviewRepository()
	if err != nil {
		return nil, "", err
	}
	grants, nextCursor, err := repo.ListActiveDraftReviewGrants(ctx, strings.TrimSpace(reviewer), limit, strings.TrimSpace(cursor))
	if err != nil {
		return nil, "", err
	}
	active := make([]*models.DraftReviewGrant, 0, len(grants))
	for _, grant := range grants {
		if grant != nil && grant.RevokedAt == nil {
			active = append(active, grant)
		}
	}
	return active, nextCursor, nil
}

// CountSharedDraftReviews returns the full active queue size.
func (s *DraftService) CountSharedDraftReviews(ctx context.Context, reviewer string) (int, error) {
	repo, err := s.reviewRepository()
	if err != nil {
		return 0, err
	}
	return repo.CountActiveDraftReviewGrants(ctx, strings.TrimSpace(reviewer))
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
	active map[string]*models.DraftReviewGrant
	latest map[string]*models.DraftReviewVerdict
}

func (s *DraftService) draftReviewApprovalState(ctx context.Context, owner, draftID string) (*draftReviewApprovalState, error) {
	repo, err := s.reviewRepository()
	if err != nil {
		return nil, err
	}
	grants, err := repo.ListDraftReviewGrants(ctx, owner, draftID)
	if err != nil {
		return nil, err
	}
	active := make(map[string]*models.DraftReviewGrant, len(grants))
	for _, grant := range grants {
		if grant != nil && grant.RevokedAt == nil {
			active[grant.Reviewer] = grant
		}
	}
	verdicts, err := repo.ListDraftReviewVerdicts(ctx, owner, draftID)
	if err != nil {
		return nil, err
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
	return &draftReviewApprovalState{active: active, latest: latest}, nil
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

// HasUnanimousActiveApproval applies the all-invited-reviewers rule. With no
// active grants the result is vacuously true, preserving human draft behavior.
func (s *DraftService) HasUnanimousActiveApproval(ctx context.Context, owner, draftID string) (bool, error) {
	state, err := s.draftReviewApprovalState(ctx, owner, draftID)
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
	principal, err := s.instancePrincipal(ctx)
	if err != nil {
		return false, err
	}
	state, err := s.draftReviewApprovalState(ctx, owner, draftID)
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
	approved, err := s.HasUnanimousActiveApproval(ctx, owner, draftID)
	if err != nil || !approved {
		return approved, err
	}
	return s.HasPrincipalApproval(ctx, owner, draftID)
}

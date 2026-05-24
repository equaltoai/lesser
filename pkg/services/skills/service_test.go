package skills

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

type fakeSkillRepo struct {
	skills      map[string]*models.Skill
	revisions   map[string]*models.SkillRevision
	assignments map[string]*models.SkillAssignment
	proposals   map[string]*models.SkillProposal

	listSkillsErr             error
	listRevisionsErr          error
	listProposalsErr          error
	listAssignmentsErr        error
	listSubjectAssignmentsErr error
	updateSkillErr            error
	updateRevisionErr         error
	createAssignmentErr       error

	// listRevisionsByStatusCallCount tracks how many times ListSkillRevisionsByStatus
	// was called. Used by regression tests to prove bounded scan behavior.
	listRevisionsByStatusCallCount int
}

func newFakeSkillRepo() *fakeSkillRepo {
	return &fakeSkillRepo{
		skills:      map[string]*models.Skill{},
		revisions:   map[string]*models.SkillRevision{},
		assignments: map[string]*models.SkillAssignment{},
		proposals:   map[string]*models.SkillProposal{},
	}
}

func seedApprovedRevision(t *testing.T, ctx context.Context, repo *fakeSkillRepo, revision *models.SkillRevision) {
	t.Helper()

	revision.SkillID = strings.ToLower(strings.TrimSpace(revision.SkillID))
	if revision.ID == "" {
		revision.ID = fmt.Sprintf("%s-r%d", revision.SkillID, revision.RevisionNumber)
	}
	if revision.Status == "" {
		revision.Status = models.SkillRevisionStatusApproved
	}
	if strings.TrimSpace(revision.DefaultExposure) == "" {
		revision.DefaultExposure = models.SkillExposurePrivate
	}
	if revision.ApprovalID == "" {
		revision.ApprovalID = "approval-" + revision.ID
	}
	if revision.ApprovalAuthorityType == "" {
		revision.ApprovalAuthorityType = models.SkillApprovalAuthorityAdmin
	}
	if revision.ApprovalAuthorityID == "" {
		revision.ApprovalAuthorityID = "ops"
	}
	if revision.ApprovedBy == "" {
		revision.ApprovedBy = "ops"
	}
	if revision.PrincipalID == "" {
		revision.PrincipalID = "principal-1"
	}
	if revision.ApprovedAt == nil {
		approvedAt := time.Date(2026, 5, 13, 12, 0, 0, 0, time.UTC)
		revision.ApprovedAt = &approvedAt
	}
	digest, err := models.SkillRevisionApprovalDigest(revision, revision.PrincipalID, revision.ApprovalAuthorityType, revision.ApprovalAuthorityID)
	require.NoError(t, err)
	revision.ApprovalDigest = digest
	require.NoError(t, repo.CreateSkillRevision(ctx, revision))
}

func acceptedPromotionProposal(t *testing.T, id, skillID string) *models.SkillProposal {
	t.Helper()
	manifestJSON, manifestDigest, err := canonicalizeSkillManifest(`{"capabilities":["social.post","memory.read"],"name":"Skill A","version":1}`)
	require.NoError(t, err)
	reviewedAt := time.Date(2026, 5, 13, 12, 15, 0, 0, time.UTC)
	return &models.SkillProposal{
		ID:                     id,
		SkillID:                skillID,
		Status:                 models.SkillProposalStatusAccepted,
		RequestedExposure:      models.SkillExposurePrivate,
		ProposedRevisionNumber: 1,
		ProposedManifestJSON:   manifestJSON,
		ProposedManifestDigest: manifestDigest,
		SourceType:             models.SkillSourceTypeHostConversation,
		SourceURI:              "lesser-host://conversations/conv-1/messages/msg-1",
		SourceDigest:           "sha256:source",
		ConversationID:         "conv-1",
		ConversationMessageID:  "msg-1",
		PrincipalID:            "principal-1",
		PrincipalApprovalID:    "principal-approval-1",
		ReviewedBy:             "ops",
		ReviewedAt:             &reviewedAt,
		ReviewReason:           "reviewed source output",
	}
}

func (f *fakeSkillRepo) CreateSkill(_ context.Context, skill *models.Skill) error {
	if err := skill.UpdateKeys(); err != nil {
		return err
	}
	f.skills[skill.ID] = skill
	return nil
}

func (f *fakeSkillRepo) GetSkill(_ context.Context, skillID string) (*models.Skill, error) {
	skill, ok := f.skills[strings.ToLower(strings.TrimSpace(skillID))]
	if !ok {
		return nil, errors.New("not found")
	}
	return skill, nil
}

func (f *fakeSkillRepo) UpdateSkill(_ context.Context, skill *models.Skill) error {
	if f.updateSkillErr != nil {
		return f.updateSkillErr
	}
	if err := skill.UpdateKeys(); err != nil {
		return err
	}
	f.skills[skill.ID] = skill
	return nil
}

func (f *fakeSkillRepo) ListSkillsByStatus(_ context.Context, status string, _ int, _ string) ([]*models.Skill, string, error) {
	if f.listSkillsErr != nil {
		return nil, "", f.listSkillsErr
	}
	if status == "" {
		status = models.SkillStatusActive
	}
	out := []*models.Skill{}
	for _, skill := range f.skills {
		if skill.Status == status {
			out = append(out, skill)
		}
	}
	return out, "", nil
}

func (f *fakeSkillRepo) ListSkillsByExposure(_ context.Context, exposure string, _ int, _ string) ([]*models.Skill, string, error) {
	if f.listSkillsErr != nil {
		return nil, "", f.listSkillsErr
	}
	out := []*models.Skill{}
	for _, skill := range f.skills {
		if skill.DefaultExposure == exposure {
			out = append(out, skill)
		}
	}
	return out, "", nil
}

func (f *fakeSkillRepo) CreateSkillRevision(_ context.Context, revision *models.SkillRevision) error {
	if err := revision.UpdateKeys(); err != nil {
		return err
	}
	f.revisions[revisionKey(revision.SkillID, revision.RevisionNumber)] = revision
	return nil
}

func (f *fakeSkillRepo) GetSkillRevision(_ context.Context, skillID string, revisionNumber int) (*models.SkillRevision, error) {
	revision, ok := f.revisions[revisionKey(skillID, revisionNumber)]
	if !ok {
		return nil, errors.New("not found")
	}
	return revision, nil
}

func (f *fakeSkillRepo) UpdateSkillRevision(_ context.Context, revision *models.SkillRevision) error {
	if f.updateRevisionErr != nil {
		return f.updateRevisionErr
	}
	if err := revision.UpdateKeys(); err != nil {
		return err
	}
	f.revisions[revisionKey(revision.SkillID, revision.RevisionNumber)] = revision
	return nil
}

func (f *fakeSkillRepo) ListSkillRevisions(_ context.Context, skillID string, _ int, _ string) ([]*models.SkillRevision, string, error) {
	if f.listRevisionsErr != nil {
		return nil, "", f.listRevisionsErr
	}
	out := []*models.SkillRevision{}
	for _, revision := range f.revisions {
		if revision.SkillID == skillID {
			out = append(out, revision)
		}
	}
	return out, "", nil
}

func (f *fakeSkillRepo) ListSkillRevisionsByStatus(_ context.Context, status string, limit int, cursor string) ([]*models.SkillRevision, string, error) {
	f.listRevisionsByStatusCallCount++
	if f.listRevisionsErr != nil {
		return nil, "", f.listRevisionsErr
	}
	out := []*models.SkillRevision{}
	for _, revision := range f.revisions {
		if status == "" || revision.Status == status {
			out = append(out, revision)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return fakeSkillRevisionCursor(out[i]) < fakeSkillRevisionCursor(out[j])
	})

	cursor = strings.TrimSpace(cursor)
	if cursor != "" {
		filtered := out[:0]
		for _, revision := range out {
			if fakeSkillRevisionCursor(revision) > cursor {
				filtered = append(filtered, revision)
			}
		}
		out = filtered
	}

	if limit <= 0 {
		return out, "", nil
	}
	if len(out) > limit {
		nextCursor := fakeSkillRevisionCursor(out[limit-1])
		return out[:limit], nextCursor, nil
	}
	return out, "", nil
}

func fakeSkillRevisionCursor(revision *models.SkillRevision) string {
	if revision == nil {
		return ""
	}
	if revision.GSI1SK != "" {
		return revision.GSI1SK
	}
	return revision.GetSK()
}

func (f *fakeSkillRepo) GetSkillRevisionByDigest(_ context.Context, manifestDigest string) (*models.SkillRevision, error) {
	for _, revision := range f.revisions {
		if revision.ManifestDigest == manifestDigest {
			return revision, nil
		}
	}
	return nil, errors.New("not found")
}

func (f *fakeSkillRepo) CreateSkillProposal(_ context.Context, proposal *models.SkillProposal) error {
	if err := proposal.UpdateKeys(); err != nil {
		return err
	}
	f.proposals[proposal.ID] = proposal
	return nil
}

func (f *fakeSkillRepo) GetSkillProposal(_ context.Context, proposalID string) (*models.SkillProposal, error) {
	proposal, ok := f.proposals[strings.ToLower(strings.TrimSpace(proposalID))]
	if !ok {
		return nil, errors.New("not found")
	}
	return proposal, nil
}

func (f *fakeSkillRepo) UpdateSkillProposal(_ context.Context, proposal *models.SkillProposal) error {
	if err := proposal.UpdateKeys(); err != nil {
		return err
	}
	f.proposals[proposal.ID] = proposal
	return nil
}

func (f *fakeSkillRepo) ListSkillProposalsForSkill(_ context.Context, skillID string, _ int, _ string) ([]*models.SkillProposal, string, error) {
	if f.listProposalsErr != nil {
		return nil, "", f.listProposalsErr
	}
	out := []*models.SkillProposal{}
	for _, proposal := range f.proposals {
		if proposal.SkillID == skillID {
			out = append(out, proposal)
		}
	}
	return out, "", nil
}

func (f *fakeSkillRepo) ListSkillProposalsByStatus(_ context.Context, status string, _ int, _ string) ([]*models.SkillProposal, string, error) {
	if f.listProposalsErr != nil {
		return nil, "", f.listProposalsErr
	}
	out := []*models.SkillProposal{}
	for _, proposal := range f.proposals {
		if status == "" || proposal.Status == status {
			out = append(out, proposal)
		}
	}
	return out, "", nil
}

func (f *fakeSkillRepo) CreateSkillAssignment(_ context.Context, assignment *models.SkillAssignment) error {
	if f.createAssignmentErr != nil {
		return f.createAssignmentErr
	}
	if err := assignment.UpdateKeys(); err != nil {
		return err
	}
	f.assignments[assignmentKey(assignment.SubjectType, assignment.SubjectID, assignment.SkillID, assignment.ID)] = assignment
	return nil
}

func (f *fakeSkillRepo) GetSkillAssignment(_ context.Context, subjectType, subjectID, skillID, assignmentID string) (*models.SkillAssignment, error) {
	assignment, ok := f.assignments[assignmentKey(subjectType, subjectID, skillID, assignmentID)]
	if !ok {
		return nil, errors.New("not found")
	}
	return assignment, nil
}

func (f *fakeSkillRepo) UpdateSkillAssignment(_ context.Context, assignment *models.SkillAssignment) error {
	if err := assignment.UpdateKeys(); err != nil {
		return err
	}
	f.assignments[assignmentKey(assignment.SubjectType, assignment.SubjectID, assignment.SkillID, assignment.ID)] = assignment
	return nil
}

func (f *fakeSkillRepo) ListSkillAssignmentsForSubject(_ context.Context, subjectType, subjectID string, _ int, _ string) ([]*models.SkillAssignment, string, error) {
	if f.listSubjectAssignmentsErr != nil {
		return nil, "", f.listSubjectAssignmentsErr
	}
	out := []*models.SkillAssignment{}
	for _, assignment := range f.assignments {
		if assignment.SubjectType == subjectType && assignment.SubjectID == subjectID {
			out = append(out, assignment)
		}
	}
	return out, "", nil
}

func (f *fakeSkillRepo) ListSkillAssignmentsForSkill(_ context.Context, skillID string, _ int, _ string) ([]*models.SkillAssignment, string, error) {
	if f.listAssignmentsErr != nil {
		return nil, "", f.listAssignmentsErr
	}
	out := []*models.SkillAssignment{}
	for _, assignment := range f.assignments {
		if assignment.SkillID == skillID {
			out = append(out, assignment)
		}
	}
	return out, "", nil
}

func TestServiceApproveRevisionRecordsPrincipalApproval(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 5, 13, 12, 30, 0, 0, time.UTC)
	repo := newFakeSkillRepo()
	svc := NewService(repo).WithNow(func() time.Time { return now })

	require.NoError(t, repo.CreateSkill(ctx, &models.Skill{ID: "skill-a", Status: models.SkillStatusDraft}))
	require.NoError(t, repo.CreateSkillRevision(ctx, &models.SkillRevision{
		SkillID:         "skill-a",
		RevisionNumber:  1,
		Status:          models.SkillRevisionStatusProposed,
		ManifestDigest:  "sha256:manifest",
		DefaultExposure: models.SkillExposureInstance,
		ProposalID:      "proposal-1",
	}))

	revision, err := svc.ApproveRevision(ctx, "skill-a", 1, ApprovalCommand{
		ActorUsername:       "ops",
		PrincipalID:         "principal-1",
		PrincipalApprovalID: "principal-approval-1",
		ApprovalRef:         "lesser://approvals/principal-approval-1",
		ApprovalReason:      "reviewed",
	})
	require.NoError(t, err)
	require.Equal(t, models.SkillRevisionStatusApproved, revision.Status)
	require.Equal(t, "ops", revision.ApprovedBy)
	require.Equal(t, "principal-1", revision.PrincipalID)
	require.Equal(t, models.SkillApprovalAuthorityAdmin, revision.ApprovalAuthorityType)
	require.NotEmpty(t, revision.ApprovalDigest)
	require.Len(t, revision.Provenance, 1)
	require.Equal(t, models.SkillSourceTypeApproval, revision.Provenance[0].SourceType)
	require.Equal(t, revision.ApprovalDigest, revision.Provenance[0].Digest)

	skill, err := repo.GetSkill(ctx, "skill-a")
	require.NoError(t, err)
	require.Equal(t, models.SkillStatusActive, skill.Status)
	require.Equal(t, 1, skill.CurrentRevisionNumber)
	require.Equal(t, models.SkillExposureInstance, skill.DefaultExposure)
}

func TestServiceApproveRevisionRejectsDigestMismatch(t *testing.T) {
	ctx := context.Background()
	repo := newFakeSkillRepo()
	svc := NewService(repo)
	require.NoError(t, repo.CreateSkill(ctx, &models.Skill{ID: "skill-a"}))
	require.NoError(t, repo.CreateSkillRevision(ctx, &models.SkillRevision{SkillID: "skill-a", RevisionNumber: 1}))

	_, err := svc.ApproveRevision(ctx, "skill-a", 1, ApprovalCommand{
		ActorUsername:  "ops",
		PrincipalID:    "principal-1",
		ApprovalDigest: "sha256:not-the-server-digest",
	})
	require.ErrorIs(t, err, ErrApprovalDigestMismatch)
}

func TestServicePromoteProposalCreatesApprovedRevision(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 5, 13, 17, 0, 0, 0, time.UTC)
	repo := newFakeSkillRepo()
	svc := NewService(repo).WithNow(func() time.Time { return now })
	require.NoError(t, repo.CreateSkill(ctx, &models.Skill{ID: "skill-a", Status: models.SkillStatusDraft, DefaultExposure: models.SkillExposurePrivate}))
	proposal := acceptedPromotionProposal(t, "proposal-1", "skill-a")
	require.NoError(t, repo.CreateSkillProposal(ctx, proposal))

	result, err := svc.PromoteProposal(ctx, "skill-a", PromotionCommand{
		ActorUsername:          "ops",
		ProposalID:             "proposal-1",
		ExpectedManifestDigest: proposal.ProposedManifestDigest,
		ExpectedSourceDigest:   proposal.SourceDigest,
		ApprovalRef:            "lesser://approvals/principal-approval-1",
	})
	require.NoError(t, err)
	require.True(t, result.Created)
	require.Equal(t, models.SkillRevisionStatusApproved, result.Revision.Status)
	require.Equal(t, "skill-a-r1", result.Revision.ID)
	require.Equal(t, "proposal-1", result.Revision.ProposalID)
	require.Equal(t, proposal.ProposedManifestDigest, result.Revision.ManifestDigest)
	require.Equal(t, "sha256:source", result.Revision.ContentDigest)
	require.Equal(t, []string{"memory.read", "social.post"}, result.Revision.Capabilities)
	require.Equal(t, models.SkillExposurePrivate, result.Revision.DefaultExposure)
	require.Equal(t, "principal-1", result.Revision.PrincipalID)
	require.Equal(t, "principal-approval-1", result.Revision.PrincipalApprovalID)
	require.Equal(t, "ops", result.Revision.ApprovedBy)
	require.NotEmpty(t, result.Revision.ApprovalDigest)
	require.Len(t, result.Revision.Provenance, 3)
	require.Equal(t, models.SkillSourceTypeHostConversation, result.Revision.Provenance[0].SourceType)
	require.Equal(t, "msg-1", result.Revision.Provenance[0].Ref)
	require.Equal(t, models.SkillSourceTypeProposal, result.Revision.Provenance[1].SourceType)
	require.Equal(t, models.SkillSourceTypeApproval, result.Revision.Provenance[2].SourceType)

	skill, err := repo.GetSkill(ctx, "skill-a")
	require.NoError(t, err)
	require.Equal(t, models.SkillStatusActive, skill.Status)
	require.Equal(t, 1, skill.CurrentRevisionNumber)
	require.Equal(t, models.SkillExposurePrivate, skill.DefaultExposure)

	promoted, err := repo.GetSkillProposal(ctx, "proposal-1")
	require.NoError(t, err)
	require.Equal(t, "skill-a-r1", promoted.PromotedRevisionID)
	require.Equal(t, 1, promoted.PromotedRevisionNumber)
	require.NotEmpty(t, promoted.PromotionDigest)
	require.Equal(t, "ops", promoted.PromotedBy)
	require.NotNil(t, promoted.PromotedAt)
}

func TestServicePromoteProposalFailsClosedForSourceAndApprovalState(t *testing.T) {
	ctx := context.Background()
	repo := newFakeSkillRepo()
	svc := NewService(repo)
	require.NoError(t, repo.CreateSkill(ctx, &models.Skill{ID: "skill-a"}))

	proposed := acceptedPromotionProposal(t, "proposal-proposed", "skill-a")
	proposed.Status = models.SkillProposalStatusProposed
	require.NoError(t, repo.CreateSkillProposal(ctx, proposed))
	_, err := svc.PromoteProposal(ctx, "skill-a", PromotionCommand{ActorUsername: "ops", ProposalID: "proposal-proposed"})
	require.ErrorIs(t, err, ErrInvalidState)

	missing := acceptedPromotionProposal(t, "proposal-missing", "skill-a")
	missing.ProposedManifestJSON = ""
	require.NoError(t, repo.CreateSkillProposal(ctx, missing))
	_, err = svc.PromoteProposal(ctx, "skill-a", PromotionCommand{ActorUsername: "ops", ProposalID: "proposal-missing"})
	require.ErrorIs(t, err, ErrInvalidInput)

	malformed := acceptedPromotionProposal(t, "proposal-malformed", "skill-a")
	malformed.ProposedManifestJSON = "{"
	require.NoError(t, repo.CreateSkillProposal(ctx, malformed))
	_, err = svc.PromoteProposal(ctx, "skill-a", PromotionCommand{ActorUsername: "ops", ProposalID: "proposal-malformed"})
	require.ErrorIs(t, err, ErrInvalidInput)

	mismatch := acceptedPromotionProposal(t, "proposal-mismatch", "skill-a")
	mismatch.ProposedManifestDigest = "sha256:not-the-manifest"
	require.NoError(t, repo.CreateSkillProposal(ctx, mismatch))
	_, err = svc.PromoteProposal(ctx, "skill-a", PromotionCommand{ActorUsername: "ops", ProposalID: "proposal-mismatch"})
	require.ErrorIs(t, err, ErrPromotionDigestMismatch)

	proposal := acceptedPromotionProposal(t, "proposal-unauthorized", "skill-a")
	require.NoError(t, repo.CreateSkillProposal(ctx, proposal))
	_, err = svc.PromoteProposal(ctx, "skill-a", PromotionCommand{ProposalID: "proposal-unauthorized"})
	require.ErrorIs(t, err, ErrInvalidInput)
}

func TestServicePromoteProposalIdempotencyAndConflict(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 5, 13, 18, 0, 0, 0, time.UTC)
	repo := newFakeSkillRepo()
	svc := NewService(repo).WithNow(func() time.Time { return now })
	require.NoError(t, repo.CreateSkill(ctx, &models.Skill{ID: "skill-a"}))
	proposal := acceptedPromotionProposal(t, "proposal-1", "skill-a")
	require.NoError(t, repo.CreateSkillProposal(ctx, proposal))

	first, err := svc.PromoteProposal(ctx, "skill-a", PromotionCommand{ActorUsername: "ops", ProposalID: "proposal-1"})
	require.NoError(t, err)
	require.True(t, first.Created)
	second, err := svc.PromoteProposal(ctx, "skill-a", PromotionCommand{ActorUsername: "ops", ProposalID: "proposal-1"})
	require.NoError(t, err)
	require.False(t, second.Created)
	require.Equal(t, first.Revision.ID, second.Revision.ID)

	conflictRepo := newFakeSkillRepo()
	conflictSvc := NewService(conflictRepo).WithNow(func() time.Time { return now })
	require.NoError(t, conflictRepo.CreateSkill(ctx, &models.Skill{ID: "skill-b"}))
	conflictProposal := acceptedPromotionProposal(t, "proposal-2", "skill-b")
	require.NoError(t, conflictRepo.CreateSkillProposal(ctx, conflictProposal))
	seedApprovedRevision(t, ctx, conflictRepo, &models.SkillRevision{
		SkillID:         "skill-b",
		RevisionNumber:  1,
		ProposalID:      "other-proposal",
		ManifestDigest:  conflictProposal.ProposedManifestDigest,
		DefaultExposure: models.SkillExposurePrivate,
		PrincipalID:     "principal-1",
	})
	_, err = conflictSvc.PromoteProposal(ctx, "skill-b", PromotionCommand{ActorUsername: "ops", ProposalID: "proposal-2"})
	require.ErrorIs(t, err, ErrPromotionConflict)

	digestConflictRepo := newFakeSkillRepo()
	digestConflictSvc := NewService(digestConflictRepo).WithNow(func() time.Time { return now })
	require.NoError(t, digestConflictRepo.CreateSkill(ctx, &models.Skill{ID: "skill-c"}))
	digestConflictProposal := acceptedPromotionProposal(t, "proposal-3", "skill-c")
	digestConflictProposal.ProposedRevisionNumber = 2
	require.NoError(t, digestConflictRepo.CreateSkillProposal(ctx, digestConflictProposal))
	seedApprovedRevision(t, ctx, digestConflictRepo, &models.SkillRevision{
		SkillID:         "skill-c",
		RevisionNumber:  1,
		ProposalID:      "other-proposal",
		ManifestDigest:  digestConflictProposal.ProposedManifestDigest,
		DefaultExposure: models.SkillExposurePrivate,
		PrincipalID:     "principal-1",
	})
	_, err = digestConflictSvc.PromoteProposal(ctx, "skill-c", PromotionCommand{ActorUsername: "ops", ProposalID: "proposal-3"})
	require.ErrorIs(t, err, ErrPromotionConflict)
}

func TestServicePromoteProposalReconcilesExistingRevision(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 5, 13, 18, 30, 0, 0, time.UTC)
	repo := newFakeSkillRepo()
	svc := NewService(repo).WithNow(func() time.Time { return now })
	require.NoError(t, repo.CreateSkill(ctx, &models.Skill{
		ID:                    "skill-a",
		Status:                models.SkillStatusActive,
		DefaultExposure:       models.SkillExposurePrivate,
		CurrentRevisionNumber: 1,
		CurrentRevisionID:     "skill-a-r1",
	}))
	proposal := acceptedPromotionProposal(t, "proposal-1", "skill-a")
	proposal.ProposedRevisionNumber = 2
	require.NoError(t, repo.CreateSkillProposal(ctx, proposal))

	manifestJSON, manifestDigest, err := canonicalizeSkillManifest(proposal.ProposedManifestJSON)
	require.NoError(t, err)
	expected := buildPromotionRevision(&models.Skill{ID: "skill-a", CurrentRevisionNumber: 1}, proposal, manifestJSON, manifestDigest, PromotionCommand{ActorUsername: "ops"}, now)
	require.NoError(t, applyPromotionApproval(expected, proposal, PromotionCommand{ActorUsername: "ops"}, now))
	require.NoError(t, repo.CreateSkillRevision(ctx, expected))

	result, err := svc.PromoteProposal(ctx, "skill-a", PromotionCommand{
		ActorUsername:          "ops",
		ProposalID:             "proposal-1",
		ExpectedManifestDigest: manifestDigest,
	})
	require.NoError(t, err)
	require.False(t, result.Created)
	require.Equal(t, expected.ID, result.Revision.ID)

	promoted, err := repo.GetSkillProposal(ctx, "proposal-1")
	require.NoError(t, err)
	require.Equal(t, expected.ID, promoted.PromotedRevisionID)
	require.NotEmpty(t, promoted.PromotionDigest)

	skill, err := repo.GetSkill(ctx, "skill-a")
	require.NoError(t, err)
	require.Equal(t, 2, skill.CurrentRevisionNumber)
}

func TestServicePromotionHelperBranches(t *testing.T) {
	_, _, err := canonicalizeSkillManifest(`{"name":"Skill"} trailing`)
	require.Error(t, err)

	require.Nil(t, capabilitiesFromManifest("{"))

	fallbackProposal := &models.SkillProposal{
		ID:                "proposal-1",
		SkillID:           "skill-a",
		SourceType:        models.SkillSourceTypeManual,
		RequestedExposure: models.SkillExposurePrivate,
	}
	provenance := promotionProvenance(fallbackProposal, "sha256:manifest")
	require.Len(t, provenance, 2)
	require.Equal(t, "sha256:manifest", provenance[0].Digest)

	require.False(t, samePromotedRevision(nil, fallbackProposal, &models.SkillRevision{}))
	require.False(t, isSkillNotFound(errors.New("boom")))

	ctx := context.Background()
	repo := newFakeSkillRepo()
	svc := NewService(repo)
	skill := &models.Skill{ID: "skill-a", CurrentRevisionNumber: 3, CurrentRevisionID: "skill-a-r3"}
	revision := &models.SkillRevision{ID: "skill-a-r2", SkillID: "skill-a", RevisionNumber: 2}
	require.NoError(t, svc.promoteSkillPointer(ctx, skill, revision, "ops"))
	require.Equal(t, 3, skill.CurrentRevisionNumber)
}

func TestServiceAssignAndResolveEffectiveSkills(t *testing.T) {
	ctx := context.Background()
	repo := newFakeSkillRepo()
	svc := NewService(repo).WithNow(func() time.Time {
		return time.Date(2026, 5, 13, 13, 0, 0, 0, time.UTC)
	})
	require.NoError(t, repo.CreateSkill(ctx, &models.Skill{
		ID:                    "skill-a",
		Status:                models.SkillStatusActive,
		DefaultExposure:       models.SkillExposurePrivate,
		CurrentRevisionID:     "skill-a-r1",
		CurrentRevisionNumber: 1,
	}))
	approved := time.Date(2026, 5, 13, 12, 0, 0, 0, time.UTC)
	seedApprovedRevision(t, ctx, repo, &models.SkillRevision{
		SkillID:               "skill-a",
		RevisionNumber:        1,
		Status:                models.SkillRevisionStatusApproved,
		DefaultExposure:       models.SkillExposurePrivate,
		ApprovalID:            "approval-1",
		ApprovalAuthorityType: models.SkillApprovalAuthorityAdmin,
		ApprovalAuthorityID:   "ops",
		ApprovedBy:            "ops",
		ApprovedAt:            &approved,
		PrincipalID:           "principal-1",
	})

	_, err := svc.AssignSkill(ctx, AssignmentCommand{
		ActorUsername:  "ops",
		SkillID:        "skill-a",
		SubjectType:    models.SkillAssignmentSubjectActor,
		SubjectID:      "alice",
		Exposure:       models.SkillExposurePublic,
		RevisionNumber: 1,
	})
	require.ErrorIs(t, err, ErrExposureViolation)

	assignment, err := svc.AssignSkill(ctx, AssignmentCommand{
		ActorUsername:  "ops",
		AssignmentID:   "assign-1",
		SkillID:        "skill-a",
		SubjectType:    models.SkillAssignmentSubjectActor,
		SubjectID:      "alice",
		Exposure:       models.SkillExposurePrivate,
		RevisionNumber: 1,
	})
	require.NoError(t, err)
	require.Equal(t, models.SkillAssignmentStatusActive, assignment.Status)

	result, err := svc.ResolveEffectiveSkills(ctx, Viewer{Username: "alice", Authenticated: true}, ResolveCommand{
		SubjectType: models.SkillAssignmentSubjectActor,
		SubjectID:   "alice",
	})
	require.NoError(t, err)
	require.Len(t, result.Items, 1)
	require.Equal(t, "skill-a", result.Items[0].Skill.ID)

	_, err = svc.ResolveEffectiveSkills(ctx, Viewer{Username: "bob", Authenticated: true}, ResolveCommand{
		SubjectType: models.SkillAssignmentSubjectActor,
		SubjectID:   "alice",
	})
	require.ErrorIs(t, err, ErrForbidden)
}

func TestServiceListSkillsAppliesExposure(t *testing.T) {
	ctx := context.Background()
	repo := newFakeSkillRepo()
	svc := NewService(repo)
	require.NoError(t, repo.CreateSkill(ctx, &models.Skill{ID: "public", Status: models.SkillStatusActive, DefaultExposure: models.SkillExposurePublic}))
	require.NoError(t, repo.CreateSkill(ctx, &models.Skill{ID: "instance", Status: models.SkillStatusActive, DefaultExposure: models.SkillExposureInstance}))
	require.NoError(t, repo.CreateSkill(ctx, &models.Skill{ID: "private", Status: models.SkillStatusActive, DefaultExposure: models.SkillExposurePrivate}))

	items, _, err := svc.ListSkills(ctx, Viewer{}, ListFilter{Status: models.SkillStatusActive})
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, "public", items[0].ID)

	items, _, err = svc.ListSkills(ctx, Viewer{Username: "alice", Authenticated: true}, ListFilter{Status: models.SkillStatusActive})
	require.NoError(t, err)
	require.Len(t, items, 2)

	items, _, err = svc.ListSkills(ctx, Viewer{Username: "ops", Authenticated: true, Admin: true}, ListFilter{Status: models.SkillStatusActive})
	require.NoError(t, err)
	require.Len(t, items, 3)
}

func TestServiceListCatalogPublishesApprovedVisibleBundles(t *testing.T) {
	ctx := context.Background()
	repo := newFakeSkillRepo()
	svc := NewService(repo)
	manifestJSON, manifestDigest, err := canonicalizeSkillManifest(`{
		"capabilities":["social.post"],
		"runtime_targets":["codex"],
		"install_hints":{"layout":"skill-directory-v1","directory_name":"skill-a","entrypoint":"SKILL.md"},
		"files":[{"path":"SKILL.md","role":"entrypoint","content_type":"text/markdown","content":"# Skill A\n"}]
	}`)
	require.NoError(t, err)
	require.NoError(t, repo.CreateSkill(ctx, &models.Skill{
		ID: "skill-a", Slug: "skill-a", Name: "Skill A", Status: models.SkillStatusActive,
		DefaultExposure: models.SkillExposurePublic, CurrentRevisionID: "skill-a-r1", CurrentRevisionNumber: 1,
	}))
	seedApprovedRevision(t, ctx, repo, &models.SkillRevision{
		SkillID:         "skill-a",
		RevisionNumber:  1,
		ManifestJSON:    manifestJSON,
		ManifestDigest:  manifestDigest,
		ContentDigest:   "sha256:content",
		DefaultExposure: models.SkillExposurePublic,
		Provenance: []models.SkillProvenanceRef{{
			SourceType: models.SkillSourceTypeProposal,
			Digest:     manifestDigest,
			Ref:        "proposal-1",
		}},
	})
	require.NoError(t, repo.CreateSkill(ctx, &models.Skill{ID: "private", Status: models.SkillStatusActive, DefaultExposure: models.SkillExposurePrivate}))
	seedApprovedRevision(t, ctx, repo, &models.SkillRevision{
		SkillID:         "private",
		RevisionNumber:  1,
		DefaultExposure: models.SkillExposurePrivate,
	})
	require.NoError(t, repo.CreateSkill(ctx, &models.Skill{ID: "draft", Status: models.SkillStatusActive, DefaultExposure: models.SkillExposurePublic}))
	require.NoError(t, repo.CreateSkillRevision(ctx, &models.SkillRevision{SkillID: "draft", RevisionNumber: 1, Status: models.SkillRevisionStatusDraft, DefaultExposure: models.SkillExposurePublic}))

	catalog, _, err := svc.ListCatalog(ctx, Viewer{}, CatalogFilter{})
	require.NoError(t, err)
	require.Len(t, catalog, 1)
	require.Equal(t, "skill-a", catalog[0].Skill.ID)
	require.Equal(t, SkillBundleSchemaVersion, catalog[0].Bundle.SchemaVersion)
	require.Equal(t, "skill:skill-a:revision:00000001", catalog[0].Bundle.BundleID)
	require.NotEmpty(t, catalog[0].Bundle.BundleDigest)
	require.NotEmpty(t, catalog[0].Bundle.PublicationDigest)
	require.Equal(t, []string{"codex"}, catalog[0].Bundle.InstallHints.RuntimeTargets)
	require.Equal(t, "SKILL.md", catalog[0].Bundle.InstallHints.EntryPoint)
	require.Len(t, catalog[0].Bundle.Files, 1)
	require.Equal(t, "skill-a/SKILL.md", catalog[0].Bundle.Files[0].InstallPath)
	require.False(t, catalog[0].Bundle.Files[0].ContentIncluded)

	catalog, _, err = svc.ListCatalog(ctx, Viewer{Username: "ops", Authenticated: true, Admin: true}, CatalogFilter{Exposure: models.SkillExposurePrivate})
	require.NoError(t, err)
	require.Len(t, catalog, 1)
	require.Equal(t, "private", catalog[0].Skill.ID)
}

func TestServiceListCatalogCursorDoesNotRevealHiddenRevisions(t *testing.T) {
	ctx := context.Background()
	repo := newFakeSkillRepo()
	svc := NewService(repo)

	seedCatalogSkill := func(skillID, exposure string, updatedAt time.Time) {
		t.Helper()
		require.NoError(t, repo.CreateSkill(ctx, &models.Skill{
			ID:                    skillID,
			Slug:                  skillID,
			Name:                  skillID,
			Status:                models.SkillStatusActive,
			DefaultExposure:       exposure,
			CurrentRevisionID:     skillID + "-r1",
			CurrentRevisionNumber: 1,
			UpdatedAt:             updatedAt,
		}))
		seedApprovedRevision(t, ctx, repo, &models.SkillRevision{
			SkillID:         skillID,
			RevisionNumber:  1,
			DefaultExposure: exposure,
			UpdatedAt:       updatedAt,
		})
	}

	base := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
	seedCatalogSkill("private-a", models.SkillExposurePrivate, base)
	seedCatalogSkill("private-b", models.SkillExposurePrivate, base.Add(time.Minute))
	seedCatalogSkill("public-a", models.SkillExposurePublic, base.Add(2*time.Minute))
	seedCatalogSkill("public-b", models.SkillExposurePublic, base.Add(3*time.Minute))

	catalog, cursor, err := svc.ListCatalog(ctx, Viewer{}, CatalogFilter{Limit: 1})
	require.NoError(t, err)
	require.Len(t, catalog, 1)
	require.Equal(t, "public-a", catalog[0].Skill.ID)
	require.NotEmpty(t, cursor)
	require.Contains(t, cursor, "SKILL#public-a")
	require.NotContains(t, cursor, "private")

	catalog, cursor, err = svc.ListCatalog(ctx, Viewer{}, CatalogFilter{Limit: 1, Cursor: cursor})
	require.NoError(t, err)
	require.Len(t, catalog, 1)
	require.Equal(t, "public-b", catalog[0].Skill.ID)
	require.Empty(t, cursor)
}

// TestServiceListCatalogScansAreBounded proves that ListCatalog cannot make
// unbounded ListSkillRevisionsByStatus calls when many approved revisions are
// hidden from the public viewer (CSR-039 regression). The service must cap raw
// revision inspection and return a cursor for continuation rather than scanning
// indefinitely.
func TestServiceListCatalogScansAreBounded(t *testing.T) {
	ctx := context.Background()
	repo := newFakeSkillRepo()
	svc := NewService(repo)

	// Seed many hidden (private) approved revisions that will appear before any
	// public revisions in the repo's cursor-ordered listing.
	const hiddenCount = 301
	for i := 0; i < hiddenCount; i++ {
		skillID := fmt.Sprintf("hidden-%03d", i)
		require.NoError(t, repo.CreateSkill(ctx, &models.Skill{
			ID:                    skillID,
			Slug:                  skillID,
			Name:                  skillID,
			Status:                models.SkillStatusActive,
			DefaultExposure:       models.SkillExposurePrivate,
			CurrentRevisionID:     skillID + "-r1",
			CurrentRevisionNumber: 1,
		}))
		seedApprovedRevision(t, ctx, repo, &models.SkillRevision{
			SkillID:               skillID,
			RevisionNumber:        1,
			DefaultExposure:       models.SkillExposurePrivate,
			ApprovalID:            "approval-" + skillID,
			ApprovalAuthorityType: models.SkillApprovalAuthorityAdmin,
			ApprovalAuthorityID:   "ops",
			ApprovedBy:            "ops",
			PrincipalID:           "principal-1",
		})
	}

	// Add a few public revisions that would be visible — but they sort after
	// "hidden-*" so they land beyond the scan cap.
	const publicCount = 5
	for i := 0; i < publicCount; i++ {
		skillID := fmt.Sprintf("public-%03d", i)
		require.NoError(t, repo.CreateSkill(ctx, &models.Skill{
			ID:                    skillID,
			Slug:                  skillID,
			Name:                  skillID,
			Status:                models.SkillStatusActive,
			DefaultExposure:       models.SkillExposurePublic,
			CurrentRevisionID:     skillID + "-r1",
			CurrentRevisionNumber: 1,
		}))
		seedApprovedRevision(t, ctx, repo, &models.SkillRevision{
			SkillID:               skillID,
			RevisionNumber:        1,
			DefaultExposure:       models.SkillExposurePublic,
			ApprovalID:            "approval-" + skillID,
			ApprovalAuthorityType: models.SkillApprovalAuthorityAdmin,
			ApprovalAuthorityID:   "ops",
			ApprovedBy:            "ops",
			PrincipalID:           "principal-1",
		})
	}

	// Reset call counter after seeding.
	repo.listRevisionsByStatusCallCount = 0

	// Public viewer, default limit (25 per page).
	catalog, cursor, err := svc.ListCatalog(ctx, Viewer{}, CatalogFilter{Limit: 25})
	require.NoError(t, err)

	// The fake repo returns revisions in cursor-sorted order: "hidden-*" < "public-*",
	// so all hidden revisions precede visible ones. With 25 per page and a cap of
	// maxCatalogScanRevisions=300, the service must stop after at most 12 calls.
	maxExpectedCalls := (maxCatalogScanRevisions + 24) / 25 // ceil(300/25)
	require.LessOrEqual(t, repo.listRevisionsByStatusCallCount, maxExpectedCalls,
		"ListCatalog made %d ListSkillRevisionsByStatus calls; expected at most %d (scan cap not enforced)",
		repo.listRevisionsByStatusCallCount, maxExpectedCalls)

	// Because all visible revisions are past the scan cap, the public viewer
	// should see zero entries and receive a continuation cursor.
	require.Empty(t, catalog, "expected no visible entries when all public revisions are beyond scan cap")
	require.NotEmpty(t, cursor, "expected a continuation cursor when scan cap is hit with no visible entries")

	t.Logf("hidden=%d public=%d calls=%d cap=%d maxCalls=%d cursor=%q",
		hiddenCount, publicCount, repo.listRevisionsByStatusCallCount,
		maxCatalogScanRevisions, maxExpectedCalls, cursor)
}

func TestServiceListCatalogScansAreBoundedWithLimit100(t *testing.T) {
	ctx := context.Background()
	repo := newFakeSkillRepo()
	svc := NewService(repo)

	// Seed enough hidden revisions to exceed the cap at limit 100.
	for i := 0; i < 350; i++ {
		skillID := fmt.Sprintf("hidden-%03d", i)
		require.NoError(t, repo.CreateSkill(ctx, &models.Skill{
			ID:                    skillID,
			Slug:                  skillID,
			Name:                  skillID,
			Status:                models.SkillStatusActive,
			DefaultExposure:       models.SkillExposurePrivate,
			CurrentRevisionID:     skillID + "-r1",
			CurrentRevisionNumber: 1,
		}))
		seedApprovedRevision(t, ctx, repo, &models.SkillRevision{
			SkillID:               skillID,
			RevisionNumber:        1,
			DefaultExposure:       models.SkillExposurePrivate,
			ApprovalID:            "approval-" + skillID,
			ApprovalAuthorityType: models.SkillApprovalAuthorityAdmin,
			ApprovalAuthorityID:   "ops",
			ApprovedBy:            "ops",
			PrincipalID:           "principal-1",
		})
	}

	repo.listRevisionsByStatusCallCount = 0

	catalog, cursor, err := svc.ListCatalog(ctx, Viewer{}, CatalogFilter{Limit: 100})
	require.NoError(t, err)

	// With limit 100 and cap 300, at most 3 calls.
	maxExpectedCalls := (maxCatalogScanRevisions + 99) / 100 // ceil(300/100)
	require.LessOrEqual(t, repo.listRevisionsByStatusCallCount, maxExpectedCalls,
		"ListCatalog made %d calls at limit 100; expected at most %d",
		repo.listRevisionsByStatusCallCount, maxExpectedCalls)
	require.Empty(t, catalog)
	require.NotEmpty(t, cursor)

	t.Logf("limit=100 calls=%d maxCalls=%d", repo.listRevisionsByStatusCallCount, maxExpectedCalls)
}

func TestServiceGetBundleIncludesContentAndRejectsUnpublished(t *testing.T) {
	ctx := context.Background()
	repo := newFakeSkillRepo()
	svc := NewService(repo)
	manifestJSON, manifestDigest, err := canonicalizeSkillManifest(`{
		"runtime_targets":["codex","generic"],
		"files":[{"path":"SKILL.md","role":"entrypoint","content":"# Skill A\n"}]
	}`)
	require.NoError(t, err)
	require.NoError(t, repo.CreateSkill(ctx, &models.Skill{ID: "skill-a", Slug: "skill-a", Status: models.SkillStatusActive, DefaultExposure: models.SkillExposurePublic, CurrentRevisionNumber: 1}))
	seedApprovedRevision(t, ctx, repo, &models.SkillRevision{
		SkillID:         "skill-a",
		RevisionNumber:  1,
		ManifestJSON:    manifestJSON,
		ManifestDigest:  manifestDigest,
		DefaultExposure: models.SkillExposurePublic,
	})
	require.NoError(t, repo.CreateSkillRevision(ctx, &models.SkillRevision{SkillID: "skill-a", RevisionNumber: 2, Status: models.SkillRevisionStatusProposed, DefaultExposure: models.SkillExposurePublic}))

	entry, err := svc.GetBundle(ctx, Viewer{}, "skill-a", 1, true)
	require.NoError(t, err)
	require.NotNil(t, entry)
	require.Len(t, entry.Bundle.Files, 1)
	require.True(t, entry.Bundle.Files[0].ContentIncluded)
	require.Equal(t, "# Skill A\n", entry.Bundle.Files[0].Content)
	require.Equal(t, "utf-8", entry.Bundle.Files[0].Encoding)
	require.Equal(t, int64(len("# Skill A\n")), entry.Bundle.Files[0].SizeBytes)
	require.NotEmpty(t, entry.Bundle.Files[0].Digest)

	_, err = svc.GetBundle(ctx, Viewer{}, "skill-a", 2, true)
	require.ErrorIs(t, err, ErrSkillRevisionNotFound)
}

func TestServiceGetBundleFailsClosedOnUnsafeFilePath(t *testing.T) {
	ctx := context.Background()
	repo := newFakeSkillRepo()
	svc := NewService(repo)
	manifestJSON, manifestDigest, err := canonicalizeSkillManifest(`{"files":[{"path":"../SKILL.md","content":"unsafe"}]}`)
	require.NoError(t, err)
	require.NoError(t, repo.CreateSkill(ctx, &models.Skill{ID: "skill-a", Status: models.SkillStatusActive, DefaultExposure: models.SkillExposurePublic, CurrentRevisionNumber: 1}))
	seedApprovedRevision(t, ctx, repo, &models.SkillRevision{
		SkillID:         "skill-a",
		RevisionNumber:  1,
		ManifestJSON:    manifestJSON,
		ManifestDigest:  manifestDigest,
		DefaultExposure: models.SkillExposurePublic,
	})

	_, err = svc.GetBundle(ctx, Viewer{}, "skill-a", 1, false)
	require.ErrorIs(t, err, ErrInvalidState)
}

func TestServiceGetBundleFailsClosedOnInlineDigestMismatch(t *testing.T) {
	ctx := context.Background()
	repo := newFakeSkillRepo()
	svc := NewService(repo)
	manifestJSON, manifestDigest, err := canonicalizeSkillManifest(`{
		"files":[{
			"path":"SKILL.md",
			"digest":"sha256:0000000000000000000000000000000000000000000000000000000000000000",
			"content":"# Skill A\n"
		}]
	}`)
	require.NoError(t, err)
	require.NoError(t, repo.CreateSkill(ctx, &models.Skill{ID: "skill-a", Status: models.SkillStatusActive, DefaultExposure: models.SkillExposurePublic, CurrentRevisionNumber: 1}))
	seedApprovedRevision(t, ctx, repo, &models.SkillRevision{
		SkillID:         "skill-a",
		RevisionNumber:  1,
		ManifestJSON:    manifestJSON,
		ManifestDigest:  manifestDigest,
		DefaultExposure: models.SkillExposurePublic,
	})

	_, err = svc.GetBundle(ctx, Viewer{}, "skill-a", 1, true)
	require.ErrorIs(t, err, ErrInvalidState)
}

func TestServiceGetBundleUsesResolvedInstallDirectory(t *testing.T) {
	ctx := context.Background()
	repo := newFakeSkillRepo()
	svc := NewService(repo)
	manifestJSON, manifestDigest, err := canonicalizeSkillManifest(`{
		"install_hints":{"directory_name":"manifest-dir","entrypoint":"SKILL.md"},
		"files":[{"path":"SKILL.md","content":"# Skill A\n"}]
	}`)
	require.NoError(t, err)
	require.NoError(t, repo.CreateSkill(ctx, &models.Skill{ID: "skill-a", Slug: "skill-slug", Status: models.SkillStatusActive, DefaultExposure: models.SkillExposurePublic, CurrentRevisionNumber: 1}))
	seedApprovedRevision(t, ctx, repo, &models.SkillRevision{
		SkillID:         "skill-a",
		RevisionNumber:  1,
		ManifestJSON:    manifestJSON,
		ManifestDigest:  manifestDigest,
		DefaultExposure: models.SkillExposurePublic,
	})

	entry, err := svc.GetBundle(ctx, Viewer{}, "skill-a", 1, false)
	require.NoError(t, err)
	require.Equal(t, "manifest-dir", entry.Bundle.InstallHints.DirectoryName)
	require.Len(t, entry.Bundle.Files, 1)
	require.Equal(t, "manifest-dir/SKILL.md", entry.Bundle.Files[0].InstallPath)
}

func TestSkillBundleHelpersCoverContractEdges(t *testing.T) {
	encoded := "IyBTa2lsbCBCCg=="
	manifestJSON := `{
		"runtime_targets":["generic","codex","codex"],
		"install_hints":{"runtime_targets":["codex"],"required_files":["SKILL.md","../blocked"]},
		"files":[
			{"path":"docs/README.md","role":"support","digest":"sha256:readme"},
			{"path":"SKILL.md","role":"skill","content":"` + encoded + `","encoding":"base64"}
		]
	}`
	skill := &models.Skill{ID: "skill-a", Slug: "skill-a"}
	revision := &models.SkillRevision{
		ID:              "skill-a-r1",
		SkillID:         "skill-a",
		RevisionNumber:  1,
		Status:          models.SkillRevisionStatusApproved,
		ManifestJSON:    manifestJSON,
		ManifestDigest:  "sha256:manifest",
		BundleDigest:    "sha256:stored-bundle",
		ContentDigest:   "sha256:content",
		ApprovalDigest:  "sha256:approval",
		DefaultExposure: models.SkillExposurePublic,
	}

	entry, err := buildCatalogEntry(skill, revision, true)
	require.NoError(t, err)
	require.Equal(t, "sha256:stored-bundle", entry.Bundle.BundleDigest)
	require.Equal(t, "skill:skill-a:revision:00000001", entry.Bundle.BundleID)
	require.Equal(t, []string{"codex"}, entry.Bundle.InstallHints.RuntimeTargets)
	require.Equal(t, []string{"SKILL.md"}, entry.Bundle.InstallHints.RequiredFiles)
	require.Len(t, entry.Bundle.Files, 2)
	require.Equal(t, "docs/README.md", entry.Bundle.Files[0].Path)
	require.Equal(t, "SKILL.md", entry.Bundle.Files[1].Path)
	require.Equal(t, encoded, entry.Bundle.Files[1].Content)
	require.Equal(t, skillBundleBase64, entry.Bundle.Files[1].Encoding)
	require.True(t, entry.Bundle.Files[1].ContentIncluded)

	files := revisionFilesFromManifest(manifestJSON)
	require.Len(t, files, 2)
	require.Equal(t, "SKILL.md", files[1].Path)
	require.NotEmpty(t, files[1].Digest)

	_, err = buildCatalogEntry(nil, revision, false)
	require.ErrorIs(t, err, ErrInvalidInput)
	_, err = buildCatalogEntry(skill, &models.SkillRevision{Status: models.SkillRevisionStatusDraft}, false)
	require.ErrorIs(t, err, ErrSkillRevisionNotFound)
	_, err = parseSkillBundleManifest(`{"files":[]}{}`)
	require.Error(t, err)
	_, err = buildSkillBundle(skill, &models.SkillRevision{ManifestJSON: `{"files":[{"path":"SKILL.md"}]}`}, false)
	require.Error(t, err)
	_, _, _, _, err = manifestFileContent(skillManifestBundleFile{InlineBase64: "not-base64"})
	require.Error(t, err)
	var ok bool
	_, _, _, ok, err = manifestFileContent(skillManifestBundleFile{InlineText: "plain"})
	require.NoError(t, err)
	require.True(t, ok)
	require.Empty(t, skillBundleID(nil))
	require.Equal(t, []string{"safe/file.md"}, safeBundlePaths([]string{"safe/file.md", "../blocked", "/also-blocked"}))
	require.Equal(t, "skill/SKILL.md", skillInstallPath("", "SKILL.md"))
	require.ErrorIs(t, validateCatalogFilter(CatalogFilter{Exposure: "world"}), ErrInvalidInput)
	require.NotEmpty(t, effectiveBundleDigest(&models.SkillRevision{SkillID: "skill-a", ID: "skill-a-r2", RevisionNumber: 2}, entry.Bundle.Files, entry.Bundle.InstallHints))
	require.Len(t, sortedBundleFiles([]SkillBundleFile{{Path: "z"}, {Path: "a"}}), 2)

	revisionWithStoredFiles := *revision
	revisionWithStoredFiles.BundleDigest = ""
	revisionWithStoredFiles.Files = []models.SkillRevisionFile{{
		Path:        "SKILL.md",
		Digest:      sha256Digest([]byte("# Skill B\n")),
		ContentType: "text/markdown",
		Role:        "entrypoint",
		SizeBytes:   99,
	}}
	bundle, err := buildSkillBundle(skill, &revisionWithStoredFiles, false)
	require.NoError(t, err)
	require.NotEqual(t, "sha256:stored-bundle", bundle.BundleDigest)
	require.Len(t, bundle.Files, 2)
	require.False(t, bundle.Files[0].ContentIncluded)

	repo := newFakeSkillRepo()
	repo.listRevisionsErr = errors.New("list failed")
	_, _, err = NewService(repo).ListCatalog(context.Background(), Viewer{}, CatalogFilter{})
	require.Error(t, err)
}

func TestServiceInspectAndAdminReadPaths(t *testing.T) {
	ctx := context.Background()
	repo := newFakeSkillRepo()
	svc := NewService(repo)
	publicSkill := &models.Skill{ID: "public", Status: models.SkillStatusActive, DefaultExposure: models.SkillExposurePublic}
	privateSkill := &models.Skill{ID: "private", Status: models.SkillStatusActive, DefaultExposure: models.SkillExposurePrivate}
	require.NoError(t, repo.CreateSkill(ctx, publicSkill))
	require.NoError(t, repo.CreateSkill(ctx, privateSkill))
	approvedAt := time.Date(2026, 5, 13, 12, 0, 0, 0, time.UTC)
	seedApprovedRevision(t, ctx, repo, &models.SkillRevision{
		SkillID:               "public",
		RevisionNumber:        1,
		Status:                models.SkillRevisionStatusApproved,
		DefaultExposure:       models.SkillExposurePublic,
		ApprovalID:            "approval-public",
		ApprovalAuthorityType: models.SkillApprovalAuthorityAdmin,
		ApprovalAuthorityID:   "ops",
		ApprovedBy:            "ops",
		ApprovedAt:            &approvedAt,
		PrincipalID:           "principal-1",
	})
	require.NoError(t, repo.CreateSkillRevision(ctx, &models.SkillRevision{SkillID: "public", RevisionNumber: 2, Status: models.SkillRevisionStatusDraft}))
	seedApprovedRevision(t, ctx, repo, &models.SkillRevision{
		SkillID:               "public",
		RevisionNumber:        3,
		Status:                models.SkillRevisionStatusApproved,
		DefaultExposure:       models.SkillExposurePrivate,
		ApprovalID:            "approval-private",
		ApprovalAuthorityType: models.SkillApprovalAuthorityAdmin,
		ApprovalAuthorityID:   "ops",
		ApprovedBy:            "ops",
		ApprovedAt:            &approvedAt,
		PrincipalID:           "principal-1",
	})
	seedApprovedRevision(t, ctx, repo, &models.SkillRevision{
		SkillID:               "public",
		RevisionNumber:        4,
		Status:                models.SkillRevisionStatusApproved,
		DefaultExposure:       models.SkillExposureInstance,
		ApprovalID:            "approval-instance",
		ApprovalAuthorityType: models.SkillApprovalAuthorityAdmin,
		ApprovalAuthorityID:   "ops",
		ApprovedBy:            "ops",
		ApprovedAt:            &approvedAt,
		PrincipalID:           "principal-1",
	})
	require.NoError(t, repo.CreateSkillProposal(ctx, &models.SkillProposal{ID: "proposal-1", SkillID: "public", Status: models.SkillProposalStatusProposed}))
	require.NoError(t, repo.CreateSkillAssignment(ctx, &models.SkillAssignment{
		ID:             "assign-1",
		SkillID:        "public",
		RevisionNumber: 1,
		SubjectType:    models.SkillAssignmentSubjectActor,
		SubjectID:      "alice",
		Exposure:       models.SkillExposurePublic,
		Status:         models.SkillAssignmentStatusActive,
	}))

	got, err := svc.GetSkill(ctx, Viewer{}, "public")
	require.NoError(t, err)
	require.Equal(t, "public", got.ID)

	_, err = svc.GetSkill(ctx, Viewer{Username: "alice", Authenticated: true}, "private")
	require.ErrorIs(t, err, ErrSkillNotFound)

	got, err = svc.GetSkill(ctx, Viewer{Username: "ops", Authenticated: true, Admin: true}, "private")
	require.NoError(t, err)
	require.Equal(t, "private", got.ID)

	revisions, _, err := svc.ListRevisions(ctx, Viewer{}, "public", 10, "")
	require.NoError(t, err)
	require.Len(t, revisions, 1)
	require.Equal(t, 1, revisions[0].RevisionNumber)

	revisions, _, err = svc.ListRevisions(ctx, Viewer{Username: "alice", Authenticated: true}, "public", 10, "")
	require.NoError(t, err)
	require.Len(t, revisions, 2)

	revisions, _, err = svc.ListRevisions(ctx, Viewer{Username: "ops", Authenticated: true, Admin: true}, "public", 10, "")
	require.NoError(t, err)
	require.Len(t, revisions, 4)

	revision, err := svc.GetRevision(ctx, Viewer{}, "public", 1)
	require.NoError(t, err)
	require.Equal(t, 1, revision.RevisionNumber)

	_, err = svc.GetRevision(ctx, Viewer{}, "public", 2)
	require.ErrorIs(t, err, ErrSkillRevisionNotFound)
	_, err = svc.GetRevision(ctx, Viewer{}, "public", 3)
	require.ErrorIs(t, err, ErrSkillRevisionNotFound)
	_, err = svc.GetRevision(ctx, Viewer{Username: "alice", Authenticated: true}, "public", 4)
	require.NoError(t, err)

	_, err = svc.GetRevision(ctx, Viewer{}, "public", 2)
	require.ErrorIs(t, err, ErrSkillRevisionNotFound)

	proposals, _, err := svc.ListProposals(ctx, "", models.SkillProposalStatusProposed, 10, "")
	require.NoError(t, err)
	require.Len(t, proposals, 1)

	proposals, _, err = svc.ListProposals(ctx, "public", "", 10, "")
	require.NoError(t, err)
	require.Len(t, proposals, 1)

	proposal, err := svc.GetProposal(ctx, "proposal-1")
	require.NoError(t, err)
	require.Equal(t, "proposal-1", proposal.ID)

	_, err = svc.GetProposal(ctx, "missing")
	require.ErrorIs(t, err, ErrSkillProposalNotFound)

	assignments, _, err := svc.ListAssignmentsForSkill(ctx, "public", 10, "")
	require.NoError(t, err)
	require.Len(t, assignments, 1)
}

func TestServiceApproveRevisionSupersedesCurrent(t *testing.T) {
	ctx := context.Background()
	repo := newFakeSkillRepo()
	svc := NewService(repo).WithNow(func() time.Time {
		return time.Date(2026, 5, 13, 14, 0, 0, 0, time.UTC)
	})
	approvedAt := time.Date(2026, 5, 13, 13, 0, 0, 0, time.UTC)
	require.NoError(t, repo.CreateSkill(ctx, &models.Skill{
		ID:                    "skill-a",
		Status:                models.SkillStatusActive,
		DefaultExposure:       models.SkillExposurePrivate,
		CurrentRevisionID:     "skill-a-r1",
		CurrentRevisionNumber: 1,
	}))
	seedApprovedRevision(t, ctx, repo, &models.SkillRevision{
		SkillID:               "skill-a",
		RevisionNumber:        1,
		Status:                models.SkillRevisionStatusApproved,
		DefaultExposure:       models.SkillExposurePrivate,
		ApprovalID:            "approval-1",
		ApprovalAuthorityType: models.SkillApprovalAuthorityAdmin,
		ApprovalAuthorityID:   "ops",
		ApprovedBy:            "ops",
		ApprovedAt:            &approvedAt,
		PrincipalID:           "principal-1",
	})
	require.NoError(t, repo.CreateSkillRevision(ctx, &models.SkillRevision{
		SkillID:         "skill-a",
		RevisionNumber:  2,
		Status:          models.SkillRevisionStatusProposed,
		DefaultExposure: models.SkillExposureInstance,
		ManifestDigest:  "sha256:manifest-2",
	}))

	revision, err := svc.ApproveRevision(ctx, "skill-a", 2, ApprovalCommand{ActorUsername: "ops"})
	require.NoError(t, err)
	require.Equal(t, models.SkillRevisionStatusApproved, revision.Status)

	oldRevision, err := repo.GetSkillRevision(ctx, "skill-a", 1)
	require.NoError(t, err)
	require.Equal(t, models.SkillRevisionStatusSuperseded, oldRevision.Status)

	skill, err := repo.GetSkill(ctx, "skill-a")
	require.NoError(t, err)
	require.Equal(t, 2, skill.CurrentRevisionNumber)
	require.Equal(t, models.SkillExposureInstance, skill.DefaultExposure)

	_, err = svc.ApproveRevision(ctx, "skill-a", 2, ApprovalCommand{ActorUsername: "ops"})
	require.ErrorIs(t, err, ErrInvalidState)
}

func TestServiceRevokeRevisionClearsCurrent(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 5, 13, 15, 0, 0, 0, time.UTC)
	repo := newFakeSkillRepo()
	svc := NewService(repo).WithNow(func() time.Time { return now })
	approvedAt := now.Add(-time.Hour)
	require.NoError(t, repo.CreateSkill(ctx, &models.Skill{
		ID:                    "skill-a",
		Status:                models.SkillStatusActive,
		DefaultExposure:       models.SkillExposurePrivate,
		CurrentRevisionID:     "skill-a-r1",
		CurrentRevisionNumber: 1,
	}))
	seedApprovedRevision(t, ctx, repo, &models.SkillRevision{
		SkillID:               "skill-a",
		RevisionNumber:        1,
		Status:                models.SkillRevisionStatusApproved,
		DefaultExposure:       models.SkillExposurePrivate,
		ApprovalID:            "approval-1",
		ApprovalAuthorityType: models.SkillApprovalAuthorityAdmin,
		ApprovalAuthorityID:   "ops",
		ApprovedBy:            "ops",
		ApprovedAt:            &approvedAt,
		PrincipalID:           "principal-1",
	})

	revision, err := svc.RevokeRevision(ctx, "skill-a", 1, RevocationCommand{ActorUsername: "ops", Reason: "unsafe"})
	require.NoError(t, err)
	require.Equal(t, models.SkillRevisionStatusRevoked, revision.Status)
	require.Equal(t, "ops", revision.RevokedBy)
	require.Equal(t, "unsafe", revision.RevokedReason)
	require.NotNil(t, revision.RevokedAt)

	skill, err := repo.GetSkill(ctx, "skill-a")
	require.NoError(t, err)
	require.Equal(t, models.SkillStatusDraft, skill.Status)
	require.Zero(t, skill.CurrentRevisionNumber)

	_, err = svc.RevokeRevision(ctx, "skill-a", 1, RevocationCommand{ActorUsername: "ops"})
	require.ErrorIs(t, err, ErrInvalidState)
}

func TestServiceRevokeAssignment(t *testing.T) {
	ctx := context.Background()
	repo := newFakeSkillRepo()
	svc := NewService(repo).WithNow(func() time.Time {
		return time.Date(2026, 5, 13, 16, 0, 0, 0, time.UTC)
	})
	require.NoError(t, repo.CreateSkill(ctx, &models.Skill{
		ID:                    "skill-a",
		Status:                models.SkillStatusActive,
		DefaultExposure:       models.SkillExposureInstance,
		CurrentRevisionID:     "skill-a-r1",
		CurrentRevisionNumber: 1,
	}))
	approvedAt := time.Date(2026, 5, 13, 12, 0, 0, 0, time.UTC)
	seedApprovedRevision(t, ctx, repo, &models.SkillRevision{
		SkillID:               "skill-a",
		RevisionNumber:        1,
		Status:                models.SkillRevisionStatusApproved,
		DefaultExposure:       models.SkillExposureInstance,
		ApprovalID:            "approval-1",
		ApprovalAuthorityType: models.SkillApprovalAuthorityAdmin,
		ApprovalAuthorityID:   "ops",
		ApprovedBy:            "ops",
		ApprovedAt:            &approvedAt,
		PrincipalID:           "principal-1",
	})
	assignment, err := svc.AssignSkill(ctx, AssignmentCommand{
		ActorUsername:  "ops",
		AssignmentID:   "assign-1",
		SkillID:        "skill-a",
		SubjectType:    models.SkillAssignmentSubjectActor,
		SubjectID:      "alice",
		Exposure:       models.SkillExposureInstance,
		RevisionNumber: 1,
	})
	require.NoError(t, err)
	require.Equal(t, models.SkillAssignmentStatusActive, assignment.Status)

	revoked, err := svc.RevokeAssignment(ctx, AssignmentRevocationCommand{
		SkillID:      "skill-a",
		AssignmentID: "assign-1",
		SubjectType:  models.SkillAssignmentSubjectActor,
		SubjectID:    "alice",
		RevocationCommand: RevocationCommand{
			ActorUsername: "ops",
			Reason:        "rotated",
		},
	})
	require.NoError(t, err)
	require.Equal(t, models.SkillAssignmentStatusRevoked, revoked.Status)
	require.Equal(t, "rotated", revoked.RevokedReason)

	_, err = svc.RevokeAssignment(ctx, AssignmentRevocationCommand{
		SkillID:           "skill-a",
		AssignmentID:      "assign-1",
		SubjectType:       models.SkillAssignmentSubjectActor,
		SubjectID:         "alice",
		RevocationCommand: RevocationCommand{ActorUsername: "ops"},
	})
	require.ErrorIs(t, err, ErrInvalidState)

	_, err = svc.RevokeAssignment(ctx, AssignmentRevocationCommand{
		SkillID:           "skill-a",
		AssignmentID:      "missing",
		SubjectType:       models.SkillAssignmentSubjectActor,
		SubjectID:         "alice",
		RevocationCommand: RevocationCommand{ActorUsername: "ops"},
	})
	require.ErrorIs(t, err, ErrSkillAssignmentNotFound)
}

func TestServiceValidationAndVisibilityHelpers(t *testing.T) {
	ctx := context.Background()
	svc := NewService(newFakeSkillRepo())
	_, _, err := svc.ListSkills(ctx, Viewer{}, ListFilter{Exposure: "friends"})
	require.ErrorIs(t, err, ErrInvalidInput)
	_, _, err = svc.ListSkills(ctx, Viewer{}, ListFilter{Status: "deleted"})
	require.ErrorIs(t, err, ErrInvalidInput)
	_, _, err = (*Service)(nil).ListSkills(ctx, Viewer{}, ListFilter{})
	require.ErrorIs(t, err, ErrRepositoryUnavailable)

	require.False(t, CanInspectSkill(Viewer{Admin: true}, nil))
	require.False(t, CanInspectSkill(Viewer{Authenticated: true}, &models.Skill{DefaultExposure: "unknown"}))
	require.False(t, CanInspectRevision(Viewer{}, nil))
	require.False(t, CanInspectRevision(Viewer{}, &models.SkillRevision{Status: models.SkillRevisionStatusDraft, DefaultExposure: models.SkillExposurePublic}))
	require.True(t, CanInspectRevision(Viewer{}, &models.SkillRevision{Status: models.SkillRevisionStatusApproved, DefaultExposure: models.SkillExposurePublic}))
	require.True(t, CanInspectRevision(Viewer{Authenticated: true}, &models.SkillRevision{Status: models.SkillRevisionStatusApproved, DefaultExposure: models.SkillExposureInstance}))
	require.False(t, CanInspectRevision(Viewer{}, &models.SkillRevision{Status: models.SkillRevisionStatusApproved, DefaultExposure: models.SkillExposureInstance}))
	require.False(t, CanInspectRevision(Viewer{Authenticated: true}, &models.SkillRevision{Status: models.SkillRevisionStatusApproved, DefaultExposure: models.SkillExposurePrivate}))
	require.True(t, CanInspectRevision(Viewer{Admin: true}, &models.SkillRevision{Status: models.SkillRevisionStatusDraft, DefaultExposure: models.SkillExposurePrivate}))
	require.True(t, CanResolveSubject(Viewer{Admin: true}, models.SkillAssignmentSubjectInstance, "site"))
	require.False(t, CanResolveSubject(Viewer{}, models.SkillAssignmentSubjectActor, "alice"))
	require.False(t, ExposureWithin("unknown", models.SkillExposurePublic))

	require.True(t, canExposeAssignment(Viewer{}, &models.SkillAssignment{Exposure: models.SkillExposurePublic}))
	require.True(t, canExposeAssignment(Viewer{Authenticated: true}, &models.SkillAssignment{Exposure: models.SkillExposureInstance}))
	require.True(t, canExposeAssignment(Viewer{Username: "alice", Authenticated: true}, &models.SkillAssignment{
		SubjectType: models.SkillAssignmentSubjectActor,
		SubjectID:   "alice",
		Exposure:    models.SkillExposurePrivate,
	}))
	require.False(t, canExposeAssignment(Viewer{Username: "bob", Authenticated: true}, &models.SkillAssignment{
		SubjectType: models.SkillAssignmentSubjectActor,
		SubjectID:   "alice",
		Exposure:    models.SkillExposurePrivate,
	}))
	require.False(t, canExposeAssignment(Viewer{Authenticated: true}, &models.SkillAssignment{Exposure: "unknown"}))
	require.False(t, canExposeAssignment(Viewer{}, nil))
}

func TestServiceAssignSkillInvalidTargets(t *testing.T) {
	ctx := context.Background()
	repo := newFakeSkillRepo()
	svc := NewService(repo)

	_, err := svc.AssignSkill(ctx, AssignmentCommand{SkillID: "missing"})
	require.ErrorIs(t, err, ErrSkillNotFound)

	require.NoError(t, repo.CreateSkill(ctx, &models.Skill{ID: "skill-a", Status: models.SkillStatusDraft}))
	_, err = svc.AssignSkill(ctx, AssignmentCommand{SkillID: "skill-a"})
	require.ErrorIs(t, err, ErrInvalidState)

	_, err = svc.AssignSkill(ctx, AssignmentCommand{SkillID: "skill-a", RevisionNumber: 9})
	require.ErrorIs(t, err, ErrSkillRevisionNotFound)

	require.NoError(t, repo.CreateSkillRevision(ctx, &models.SkillRevision{SkillID: "skill-a", RevisionNumber: 1, Status: models.SkillRevisionStatusDraft}))
	_, err = svc.AssignSkill(ctx, AssignmentCommand{SkillID: "skill-a", RevisionNumber: 1})
	require.ErrorIs(t, err, ErrInvalidState)
}

func TestServiceResolveFiltersInvalidAssignments(t *testing.T) {
	ctx := context.Background()
	repo := newFakeSkillRepo()
	svc := NewService(repo)
	approvedAt := time.Date(2026, 5, 13, 12, 0, 0, 0, time.UTC)
	require.NoError(t, repo.CreateSkill(ctx, &models.Skill{ID: "valid", Status: models.SkillStatusActive, CurrentRevisionNumber: 1, DefaultExposure: models.SkillExposurePublic}))
	seedApprovedRevision(t, ctx, repo, &models.SkillRevision{
		SkillID:               "valid",
		RevisionNumber:        1,
		Status:                models.SkillRevisionStatusApproved,
		DefaultExposure:       models.SkillExposurePublic,
		ApprovalID:            "approval-valid",
		ApprovalAuthorityType: models.SkillApprovalAuthorityAdmin,
		ApprovalAuthorityID:   "ops",
		ApprovedBy:            "ops",
		ApprovedAt:            &approvedAt,
		PrincipalID:           "principal-1",
	})
	require.NoError(t, repo.CreateSkill(ctx, &models.Skill{ID: "draft", Status: models.SkillStatusDraft, CurrentRevisionNumber: 1, DefaultExposure: models.SkillExposurePublic}))
	require.NoError(t, repo.CreateSkillRevision(ctx, &models.SkillRevision{SkillID: "draft", RevisionNumber: 1, Status: models.SkillRevisionStatusDraft, DefaultExposure: models.SkillExposurePublic}))

	assignments := []*models.SkillAssignment{
		{ID: "valid", SkillID: "valid", RevisionNumber: 1, SubjectType: models.SkillAssignmentSubjectActor, SubjectID: "alice", Exposure: models.SkillExposurePublic, Status: models.SkillAssignmentStatusActive},
		{ID: "revoked", SkillID: "valid", RevisionNumber: 1, SubjectType: models.SkillAssignmentSubjectActor, SubjectID: "alice", Exposure: models.SkillExposurePublic, Status: models.SkillAssignmentStatusRevoked},
		{ID: "missing-skill", SkillID: "missing", RevisionNumber: 1, SubjectType: models.SkillAssignmentSubjectActor, SubjectID: "alice", Exposure: models.SkillExposurePublic, Status: models.SkillAssignmentStatusActive},
		{ID: "draft-revision", SkillID: "draft", RevisionNumber: 1, SubjectType: models.SkillAssignmentSubjectActor, SubjectID: "alice", Exposure: models.SkillExposurePublic, Status: models.SkillAssignmentStatusActive},
	}
	for _, assignment := range assignments {
		require.NoError(t, repo.CreateSkillAssignment(ctx, assignment))
	}

	result, err := svc.ResolveEffectiveSkills(ctx, Viewer{Username: "alice", Authenticated: true}, ResolveCommand{SubjectType: models.SkillAssignmentSubjectActor, SubjectID: "alice"})
	require.NoError(t, err)
	require.Len(t, result.Items, 1)
	require.Equal(t, "valid", result.Items[0].Skill.ID)

	_, ok := svc.resolveAssignment(ctx, Viewer{}, &models.SkillAssignment{SkillID: "valid", SubjectType: models.SkillAssignmentSubjectActor, SubjectID: "alice", Exposure: models.SkillExposurePrivate, Status: models.SkillAssignmentStatusActive})
	require.False(t, ok)
}

func TestServiceMutationValidationBranches(t *testing.T) {
	ctx := context.Background()
	repo := newFakeSkillRepo()
	svc := NewService(repo)
	approvedAt := time.Date(2026, 5, 13, 12, 0, 0, 0, time.UTC)
	require.NoError(t, repo.CreateSkill(ctx, &models.Skill{ID: "skill-a", Status: models.SkillStatusActive, CurrentRevisionNumber: 1, DefaultExposure: models.SkillExposurePrivate}))
	seedApprovedRevision(t, ctx, repo, &models.SkillRevision{
		SkillID:               "skill-a",
		RevisionNumber:        1,
		Status:                models.SkillRevisionStatusApproved,
		DefaultExposure:       models.SkillExposurePrivate,
		ApprovalID:            "approval-1",
		ApprovalAuthorityType: models.SkillApprovalAuthorityAdmin,
		ApprovalAuthorityID:   "ops",
		ApprovedBy:            "ops",
		ApprovedAt:            &approvedAt,
		PrincipalID:           "principal-1",
	})

	_, err := svc.RevokeRevision(ctx, "skill-a", 1, RevocationCommand{})
	require.ErrorIs(t, err, ErrInvalidInput)

	require.NoError(t, repo.CreateSkillAssignment(ctx, &models.SkillAssignment{
		ID:             "assign-1",
		SkillID:        "skill-a",
		RevisionNumber: 1,
		SubjectType:    models.SkillAssignmentSubjectActor,
		SubjectID:      "alice",
		Exposure:       models.SkillExposurePrivate,
		Status:         models.SkillAssignmentStatusActive,
	}))
	_, err = svc.RevokeAssignment(ctx, AssignmentRevocationCommand{
		SkillID:      "skill-a",
		AssignmentID: "assign-1",
		SubjectType:  models.SkillAssignmentSubjectActor,
		SubjectID:    "alice",
	})
	require.ErrorIs(t, err, ErrInvalidInput)

	_, err = (*Service)(nil).ApproveRevision(ctx, "skill-a", 1, ApprovalCommand{})
	require.ErrorIs(t, err, ErrRepositoryUnavailable)
	_, err = (*Service)(nil).AssignSkill(ctx, AssignmentCommand{})
	require.ErrorIs(t, err, ErrRepositoryUnavailable)
}

func TestServiceRepositoryErrorBranches(t *testing.T) {
	ctx := context.Background()
	boom := errors.New("boom")
	approvedAt := time.Date(2026, 5, 13, 12, 0, 0, 0, time.UTC)
	seed := func(repo *fakeSkillRepo) {
		require.NoError(t, repo.CreateSkill(ctx, &models.Skill{ID: "skill-a", Status: models.SkillStatusActive, CurrentRevisionNumber: 1, DefaultExposure: models.SkillExposurePublic}))
		seedApprovedRevision(t, ctx, repo, &models.SkillRevision{
			SkillID:               "skill-a",
			RevisionNumber:        1,
			Status:                models.SkillRevisionStatusApproved,
			DefaultExposure:       models.SkillExposurePublic,
			ApprovalID:            "approval-1",
			ApprovalAuthorityType: models.SkillApprovalAuthorityAdmin,
			ApprovalAuthorityID:   "ops",
			ApprovedBy:            "ops",
			ApprovedAt:            &approvedAt,
			PrincipalID:           "principal-1",
		})
		require.NoError(t, repo.CreateSkillProposal(ctx, &models.SkillProposal{ID: "proposal-1", SkillID: "skill-a", Status: models.SkillProposalStatusProposed}))
	}

	repo := newFakeSkillRepo()
	seed(repo)
	repo.listRevisionsErr = boom
	_, _, err := NewService(repo).ListRevisions(ctx, Viewer{}, "skill-a", 10, "")
	require.ErrorIs(t, err, boom)

	repo = newFakeSkillRepo()
	seed(repo)
	repo.listProposalsErr = boom
	_, _, err = NewService(repo).ListProposals(ctx, "skill-a", "", 10, "")
	require.ErrorIs(t, err, boom)

	repo = newFakeSkillRepo()
	seed(repo)
	repo.listAssignmentsErr = boom
	_, _, err = NewService(repo).ListAssignmentsForSkill(ctx, "skill-a", 10, "")
	require.ErrorIs(t, err, boom)

	repo = newFakeSkillRepo()
	seed(repo)
	repo.listSubjectAssignmentsErr = boom
	_, err = NewService(repo).ResolveEffectiveSkills(ctx, Viewer{Username: "alice", Authenticated: true}, ResolveCommand{SubjectType: models.SkillAssignmentSubjectActor, SubjectID: "alice"})
	require.ErrorIs(t, err, boom)

	repo = newFakeSkillRepo()
	seed(repo)
	repo.createAssignmentErr = boom
	_, err = NewService(repo).AssignSkill(ctx, AssignmentCommand{ActorUsername: "ops", SkillID: "skill-a", SubjectType: models.SkillAssignmentSubjectActor, SubjectID: "alice", RevisionNumber: 1})
	require.ErrorIs(t, err, boom)

	repo = newFakeSkillRepo()
	seed(repo)
	repo.updateRevisionErr = boom
	_, err = NewService(repo).RevokeRevision(ctx, "skill-a", 1, RevocationCommand{ActorUsername: "ops"})
	require.ErrorIs(t, err, boom)

	repo = newFakeSkillRepo()
	require.NoError(t, repo.CreateSkill(ctx, &models.Skill{ID: "skill-a", Status: models.SkillStatusDraft, DefaultExposure: models.SkillExposurePrivate}))
	require.NoError(t, repo.CreateSkillRevision(ctx, &models.SkillRevision{SkillID: "skill-a", RevisionNumber: 1, Status: models.SkillRevisionStatusProposed}))
	repo.updateSkillErr = boom
	_, err = NewService(repo).ApproveRevision(ctx, "skill-a", 1, ApprovalCommand{ActorUsername: "ops"})
	require.ErrorIs(t, err, boom)
}

func TestServiceSmallBranchCoverage(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 5, 13, 17, 0, 0, 0, time.UTC)
	require.NotEmpty(t, generatedApprovalID(nil, now))

	var nilSvc *Service
	require.False(t, nilSvc.currentTime().IsZero())
	require.False(t, (&Service{}).currentTime().IsZero())

	revision := &models.SkillRevision{}
	appendApprovalProvenance(revision, ApprovalCommand{})
	require.Empty(t, revision.Provenance)

	repo := newFakeSkillRepo()
	svc := NewService(repo)
	_, err := svc.GetSkill(ctx, Viewer{}, "missing")
	require.ErrorIs(t, err, ErrSkillNotFound)
	_, _, err = svc.loadSkillRevision(ctx, "missing", 1)
	require.ErrorIs(t, err, ErrSkillNotFound)
	require.NoError(t, repo.CreateSkill(ctx, &models.Skill{ID: "skill-a", Status: models.SkillStatusActive, DefaultExposure: models.SkillExposurePublic}))
	_, _, err = svc.loadSkillRevision(ctx, "skill-a", 9)
	require.ErrorIs(t, err, ErrSkillRevisionNotFound)

	_, _, err = (*Service)(nil).ListAssignmentsForSkill(ctx, "skill-a", 10, "")
	require.ErrorIs(t, err, ErrRepositoryUnavailable)
	_, err = (*Service)(nil).GetProposal(ctx, "proposal-1")
	require.ErrorIs(t, err, ErrRepositoryUnavailable)
	_, err = (*Service)(nil).RevokeAssignment(ctx, AssignmentRevocationCommand{})
	require.ErrorIs(t, err, ErrRepositoryUnavailable)
}

func TestServiceReadAndMutationLoadErrors(t *testing.T) {
	ctx := context.Background()
	boom := errors.New("boom")
	repo := newFakeSkillRepo()
	svc := NewService(repo)
	repo.listSkillsErr = boom
	_, _, err := svc.ListSkills(ctx, Viewer{}, ListFilter{})
	require.ErrorIs(t, err, boom)
	_, _, err = svc.ListSkills(ctx, Viewer{}, ListFilter{Exposure: models.SkillExposurePublic})
	require.ErrorIs(t, err, boom)

	repo = newFakeSkillRepo()
	svc = NewService(repo)
	require.NoError(t, repo.CreateSkill(ctx, &models.Skill{ID: "private", Status: models.SkillStatusActive, DefaultExposure: models.SkillExposurePrivate}))
	_, _, err = svc.ListRevisions(ctx, Viewer{}, "private", 10, "")
	require.ErrorIs(t, err, ErrSkillNotFound)
	_, err = svc.GetRevision(ctx, Viewer{}, "private", 1)
	require.ErrorIs(t, err, ErrSkillNotFound)

	_, err = svc.ApproveRevision(ctx, "missing", 1, ApprovalCommand{ActorUsername: "ops"})
	require.ErrorIs(t, err, ErrSkillNotFound)
	_, err = svc.RevokeRevision(ctx, "missing", 1, RevocationCommand{ActorUsername: "ops"})
	require.ErrorIs(t, err, ErrSkillNotFound)
	_, err = svc.ApproveRevision(ctx, "private", 9, ApprovalCommand{ActorUsername: "ops"})
	require.ErrorIs(t, err, ErrSkillRevisionNotFound)
	_, err = svc.RevokeRevision(ctx, "private", 9, RevocationCommand{ActorUsername: "ops"})
	require.ErrorIs(t, err, ErrSkillRevisionNotFound)

	require.NoError(t, svc.supersedeCurrentRevision(ctx, nil, nil))
	require.NoError(t, svc.supersedeCurrentRevision(ctx, &models.Skill{ID: "private"}, &models.SkillRevision{RevisionNumber: 1}))
}

func revisionKey(skillID string, revisionNumber int) string {
	return skillID + ":" + models.SkillRevisionSortKey(revisionNumber)
}

func assignmentKey(subjectType, subjectID, skillID, assignmentID string) string {
	return subjectType + ":" + subjectID + ":" + skillID + ":" + assignmentID
}

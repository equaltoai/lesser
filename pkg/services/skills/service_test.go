package skills

import (
	"context"
	"errors"
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
}

func newFakeSkillRepo() *fakeSkillRepo {
	return &fakeSkillRepo{
		skills:      map[string]*models.Skill{},
		revisions:   map[string]*models.SkillRevision{},
		assignments: map[string]*models.SkillAssignment{},
		proposals:   map[string]*models.SkillProposal{},
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
	require.NoError(t, repo.CreateSkillRevision(ctx, &models.SkillRevision{
		SkillID:               "skill-a",
		RevisionNumber:        1,
		Status:                models.SkillRevisionStatusApproved,
		DefaultExposure:       models.SkillExposurePrivate,
		ApprovalID:            "approval-1",
		ApprovalAuthorityType: models.SkillApprovalAuthorityAdmin,
		ApprovalAuthorityID:   "ops",
		ApprovalDigest:        "sha256:approval",
		ApprovedBy:            "ops",
		ApprovedAt:            &approved,
		PrincipalID:           "principal-1",
	}))

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

func TestServiceInspectAndAdminReadPaths(t *testing.T) {
	ctx := context.Background()
	repo := newFakeSkillRepo()
	svc := NewService(repo)
	publicSkill := &models.Skill{ID: "public", Status: models.SkillStatusActive, DefaultExposure: models.SkillExposurePublic}
	privateSkill := &models.Skill{ID: "private", Status: models.SkillStatusActive, DefaultExposure: models.SkillExposurePrivate}
	require.NoError(t, repo.CreateSkill(ctx, publicSkill))
	require.NoError(t, repo.CreateSkill(ctx, privateSkill))
	require.NoError(t, repo.CreateSkillRevision(ctx, &models.SkillRevision{SkillID: "public", RevisionNumber: 1, Status: models.SkillRevisionStatusDraft}))
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

	revision, err := svc.GetRevision(ctx, Viewer{}, "public", 1)
	require.NoError(t, err)
	require.Equal(t, 1, revision.RevisionNumber)

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
	require.NoError(t, repo.CreateSkillRevision(ctx, &models.SkillRevision{
		SkillID:               "skill-a",
		RevisionNumber:        1,
		Status:                models.SkillRevisionStatusApproved,
		DefaultExposure:       models.SkillExposurePrivate,
		ApprovalID:            "approval-1",
		ApprovalAuthorityType: models.SkillApprovalAuthorityAdmin,
		ApprovalAuthorityID:   "ops",
		ApprovalDigest:        "sha256:approval-1",
		ApprovedBy:            "ops",
		ApprovedAt:            &approvedAt,
		PrincipalID:           "principal-1",
	}))
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
	require.NoError(t, repo.CreateSkillRevision(ctx, &models.SkillRevision{
		SkillID:               "skill-a",
		RevisionNumber:        1,
		Status:                models.SkillRevisionStatusApproved,
		DefaultExposure:       models.SkillExposurePrivate,
		ApprovalID:            "approval-1",
		ApprovalAuthorityType: models.SkillApprovalAuthorityAdmin,
		ApprovalAuthorityID:   "ops",
		ApprovalDigest:        "sha256:approval-1",
		ApprovedBy:            "ops",
		ApprovedAt:            &approvedAt,
		PrincipalID:           "principal-1",
	}))

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
	require.NoError(t, repo.CreateSkillRevision(ctx, &models.SkillRevision{
		SkillID:               "skill-a",
		RevisionNumber:        1,
		Status:                models.SkillRevisionStatusApproved,
		DefaultExposure:       models.SkillExposureInstance,
		ApprovalID:            "approval-1",
		ApprovalAuthorityType: models.SkillApprovalAuthorityAdmin,
		ApprovalAuthorityID:   "ops",
		ApprovalDigest:        "sha256:approval",
		ApprovedBy:            "ops",
		ApprovedAt:            &approvedAt,
		PrincipalID:           "principal-1",
	}))
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
	require.NoError(t, repo.CreateSkillRevision(ctx, &models.SkillRevision{
		SkillID:               "valid",
		RevisionNumber:        1,
		Status:                models.SkillRevisionStatusApproved,
		DefaultExposure:       models.SkillExposurePublic,
		ApprovalID:            "approval-valid",
		ApprovalAuthorityType: models.SkillApprovalAuthorityAdmin,
		ApprovalAuthorityID:   "ops",
		ApprovalDigest:        "sha256:valid",
		ApprovedBy:            "ops",
		ApprovedAt:            &approvedAt,
		PrincipalID:           "principal-1",
	}))
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
	require.NoError(t, repo.CreateSkillRevision(ctx, &models.SkillRevision{
		SkillID:               "skill-a",
		RevisionNumber:        1,
		Status:                models.SkillRevisionStatusApproved,
		DefaultExposure:       models.SkillExposurePrivate,
		ApprovalID:            "approval-1",
		ApprovalAuthorityType: models.SkillApprovalAuthorityAdmin,
		ApprovalAuthorityID:   "ops",
		ApprovalDigest:        "sha256:approval-1",
		ApprovedBy:            "ops",
		ApprovedAt:            &approvedAt,
		PrincipalID:           "principal-1",
	}))

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
		require.NoError(t, repo.CreateSkillRevision(ctx, &models.SkillRevision{
			SkillID:               "skill-a",
			RevisionNumber:        1,
			Status:                models.SkillRevisionStatusApproved,
			DefaultExposure:       models.SkillExposurePublic,
			ApprovalID:            "approval-1",
			ApprovalAuthorityType: models.SkillApprovalAuthorityAdmin,
			ApprovalAuthorityID:   "ops",
			ApprovalDigest:        "sha256:approval-1",
			ApprovedBy:            "ops",
			ApprovedAt:            &approvedAt,
			PrincipalID:           "principal-1",
		}))
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

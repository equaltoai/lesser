// Package skills implements Lesser's canonical skill authority service layer.
package skills

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	tableerrors "github.com/theory-cloud/tabletheory/pkg/errors"
)

var (
	// ErrRepositoryUnavailable indicates the skill repository was not wired.
	ErrRepositoryUnavailable = errors.New("skill repository unavailable")
	// ErrSkillNotFound indicates the requested skill was not found or not visible.
	ErrSkillNotFound = errors.New("skill not found")
	// ErrSkillRevisionNotFound indicates the requested skill revision was not found.
	ErrSkillRevisionNotFound = errors.New("skill revision not found")
	// ErrSkillAssignmentNotFound indicates the requested skill assignment was not found.
	ErrSkillAssignmentNotFound = errors.New("skill assignment not found")
	// ErrSkillProposalNotFound indicates the requested skill proposal was not found.
	ErrSkillProposalNotFound = errors.New("skill proposal not found")
	// ErrForbidden indicates the caller cannot inspect or mutate the requested skill state.
	ErrForbidden = errors.New("skill access forbidden")
	// ErrInvalidInput indicates malformed request input.
	ErrInvalidInput = errors.New("invalid skill request")
	// ErrInvalidState indicates a requested transition is not valid for current state.
	ErrInvalidState = errors.New("invalid skill state")
	// ErrApprovalDigestMismatch indicates a caller-provided approval digest did not match Lesser's digest.
	ErrApprovalDigestMismatch = errors.New("approval digest mismatch")
	// ErrExposureViolation indicates the requested exposure exceeds the canonical revision exposure.
	ErrExposureViolation = errors.New("skill exposure violation")
	// ErrPromotionDigestMismatch indicates promotion source/output digest validation failed.
	ErrPromotionDigestMismatch = errors.New("skill promotion digest mismatch")
	// ErrPromotionConflict indicates a proposal was already promoted into a conflicting canonical revision.
	ErrPromotionConflict = errors.New("skill promotion conflict")
)

// Viewer identifies the caller for exposure and self-resolution checks.
type Viewer struct {
	Username      string
	Authenticated bool
	Admin         bool
}

// ListFilter constrains skill listing.
type ListFilter struct {
	Status   string
	Exposure string
	Limit    int
	Cursor   string
}

// ApprovalCommand approves a canonical skill revision.
type ApprovalCommand struct {
	ActorUsername         string
	ApprovalID            string
	PrincipalID           string
	PrincipalApprovalID   string
	ApprovalAuthorityType string
	ApprovalAuthorityID   string
	ApprovalDigest        string
	ApprovalSignature     string
	ApprovalRef           string
	ApprovalReason        string
	ApprovedAt            time.Time
}

// RevocationCommand revokes a canonical skill revision or assignment.
type RevocationCommand struct {
	ActorUsername string
	Reason        string
	RevokedAt     time.Time
}

// AssignmentCommand creates an effective-resolution assignment.
type AssignmentCommand struct {
	ActorUsername       string
	AssignmentID        string
	SkillID             string
	RevisionNumber      int
	SubjectType         string
	SubjectID           string
	Exposure            string
	ApprovalID          string
	PrincipalID         string
	PrincipalApprovalID string
	AssignedAt          time.Time
}

// AssignmentRevocationCommand revokes an assignment at its subject boundary.
type AssignmentRevocationCommand struct {
	SkillID      string
	AssignmentID string
	SubjectType  string
	SubjectID    string
	RevocationCommand
}

// PromotionCommand promotes accepted proposal/conversation output into a canonical revision.
type PromotionCommand struct {
	ActorUsername          string
	ProposalID             string
	ExpectedManifestDigest string
	ExpectedSourceDigest   string
	ApprovalID             string
	PrincipalID            string
	PrincipalApprovalID    string
	ApprovalAuthorityType  string
	ApprovalAuthorityID    string
	ApprovalDigest         string
	ApprovalSignature      string
	ApprovalRef            string
	ApprovalReason         string
	ApprovedAt             time.Time
}

// ResolveCommand resolves active assignments for a subject.
type ResolveCommand struct {
	SubjectType string
	SubjectID   string
	Limit       int
	Cursor      string
}

// EffectiveSkill is one resolved canonical skill revision and assignment.
type EffectiveSkill struct {
	Skill      *models.Skill
	Revision   *models.SkillRevision
	Assignment *models.SkillAssignment
}

// ResolveResult contains effective skills for a subject boundary.
type ResolveResult struct {
	SubjectType string
	SubjectID   string
	Items       []EffectiveSkill
	NextCursor  string
}

// PromotionResult describes the canonical revision produced by a proposal promotion.
type PromotionResult struct {
	Revision *models.SkillRevision
	Proposal *models.SkillProposal
	Created  bool
}

// Service owns canonical skill approval, assignment, and resolution semantics.
type Service struct {
	repo interfaces.SkillRepository
	now  func() time.Time
}

// NewService constructs a canonical skill service.
func NewService(repo interfaces.SkillRepository) *Service {
	return &Service{repo: repo, now: func() time.Time { return time.Now().UTC() }}
}

// WithNow overrides the clock for tests.
func (s *Service) WithNow(now func() time.Time) *Service {
	if s != nil && now != nil {
		s.now = now
	}
	return s
}

// ListSkills returns inspectable skills for the viewer.
func (s *Service) ListSkills(ctx context.Context, viewer Viewer, filter ListFilter) ([]*models.Skill, string, error) {
	if err := s.ensureRepo(); err != nil {
		return nil, "", err
	}
	if err := validateListFilter(filter); err != nil {
		return nil, "", err
	}
	items, cursor, err := s.listSkills(ctx, filter)
	if err != nil {
		return nil, "", err
	}
	out := make([]*models.Skill, 0, len(items))
	for _, item := range items {
		if CanInspectSkill(viewer, item) {
			out = append(out, item)
		}
	}
	return out, cursor, nil
}

// GetSkill returns a skill when the viewer may inspect it.
func (s *Service) GetSkill(ctx context.Context, viewer Viewer, skillID string) (*models.Skill, error) {
	if err := s.ensureRepo(); err != nil {
		return nil, err
	}
	skill, err := s.repo.GetSkill(ctx, skillID)
	if err != nil {
		return nil, ErrSkillNotFound
	}
	if !CanInspectSkill(viewer, skill) {
		return nil, ErrSkillNotFound
	}
	return skill, nil
}

// ListRevisions returns revision records visible to the viewer.
func (s *Service) ListRevisions(ctx context.Context, viewer Viewer, skillID string, limit int, cursor string) ([]*models.SkillRevision, string, error) {
	if _, err := s.GetSkill(ctx, viewer, skillID); err != nil {
		return nil, "", err
	}
	items, next, err := s.repo.ListSkillRevisions(ctx, skillID, limit, cursor)
	if err != nil {
		return nil, "", err
	}
	out := make([]*models.SkillRevision, 0, len(items))
	for _, item := range items {
		if CanInspectRevision(viewer, item) {
			out = append(out, item)
		}
	}
	return out, next, nil
}

// GetRevision returns one revision when both the skill and revision are visible to the viewer.
func (s *Service) GetRevision(ctx context.Context, viewer Viewer, skillID string, revisionNumber int) (*models.SkillRevision, error) {
	if _, err := s.GetSkill(ctx, viewer, skillID); err != nil {
		return nil, err
	}
	revision, err := s.repo.GetSkillRevision(ctx, skillID, revisionNumber)
	if err != nil {
		return nil, ErrSkillRevisionNotFound
	}
	if !CanInspectRevision(viewer, revision) {
		return nil, ErrSkillRevisionNotFound
	}
	return revision, nil
}

// ListProposals returns proposal records for admin inspection.
func (s *Service) ListProposals(ctx context.Context, skillID string, status string, limit int, cursor string) ([]*models.SkillProposal, string, error) {
	if err := s.ensureRepo(); err != nil {
		return nil, "", err
	}
	skillID = strings.ToLower(strings.TrimSpace(skillID))
	if skillID != "" {
		return s.repo.ListSkillProposalsForSkill(ctx, skillID, limit, cursor)
	}
	return s.repo.ListSkillProposalsByStatus(ctx, status, limit, cursor)
}

// GetProposal returns one proposal for admin inspection.
func (s *Service) GetProposal(ctx context.Context, proposalID string) (*models.SkillProposal, error) {
	if err := s.ensureRepo(); err != nil {
		return nil, err
	}
	proposal, err := s.repo.GetSkillProposal(ctx, proposalID)
	if err != nil {
		return nil, ErrSkillProposalNotFound
	}
	return proposal, nil
}

// ListAssignmentsForSkill returns assignment records for admin inspection.
func (s *Service) ListAssignmentsForSkill(ctx context.Context, skillID string, limit int, cursor string) ([]*models.SkillAssignment, string, error) {
	if err := s.ensureRepo(); err != nil {
		return nil, "", err
	}
	return s.repo.ListSkillAssignmentsForSkill(ctx, skillID, limit, cursor)
}

// ApproveRevision approves a canonical revision and updates the skill current pointer.
func (s *Service) ApproveRevision(ctx context.Context, skillID string, revisionNumber int, cmd ApprovalCommand) (*models.SkillRevision, error) {
	if err := s.ensureRepo(); err != nil {
		return nil, err
	}
	skill, revision, err := s.loadSkillRevision(ctx, skillID, revisionNumber)
	if err != nil {
		return nil, err
	}
	if !canApproveRevision(revision.Status) {
		return nil, ErrInvalidState
	}
	applyApprovalDefaults(&cmd, revision, s.currentTime())
	if err := applyApprovalCommand(revision, cmd); err != nil {
		return nil, err
	}
	if err := s.repo.UpdateSkillRevision(ctx, revision); err != nil {
		return nil, err
	}
	if err := s.supersedeCurrentRevision(ctx, skill, revision); err != nil {
		return nil, err
	}
	updateSkillCurrentRevision(skill, revision, cmd.ActorUsername)
	if err := s.repo.UpdateSkill(ctx, skill); err != nil {
		return nil, err
	}
	return revision, nil
}

// RevokeRevision revokes a canonical revision and clears the skill pointer when needed.
func (s *Service) RevokeRevision(ctx context.Context, skillID string, revisionNumber int, cmd RevocationCommand) (*models.SkillRevision, error) {
	if err := s.ensureRepo(); err != nil {
		return nil, err
	}
	skill, revision, err := s.loadSkillRevision(ctx, skillID, revisionNumber)
	if err != nil {
		return nil, err
	}
	if revision.Status == models.SkillRevisionStatusRevoked {
		return nil, ErrInvalidState
	}
	if strings.TrimSpace(cmd.ActorUsername) == "" {
		return nil, errors.Join(ErrInvalidInput, errors.New("revocation actor is required"))
	}
	applyRevisionRevocation(revision, cmd, s.currentTime())
	if err := s.repo.UpdateSkillRevision(ctx, revision); err != nil {
		return nil, err
	}
	if skill.CurrentRevisionNumber == revision.RevisionNumber {
		clearSkillCurrentRevision(skill, cmd.ActorUsername)
		if err := s.repo.UpdateSkill(ctx, skill); err != nil {
			return nil, err
		}
	}
	return revision, nil
}

// AssignSkill assigns an approved revision to a subject for effective resolution.
func (s *Service) AssignSkill(ctx context.Context, cmd AssignmentCommand) (*models.SkillAssignment, error) {
	if err := s.ensureRepo(); err != nil {
		return nil, err
	}
	skill, revision, err := s.assignmentTarget(ctx, cmd)
	if err != nil {
		return nil, err
	}
	cmd = applyAssignmentDefaults(cmd, skill, revision, s.currentTime())
	assignment := assignmentFromCommand(cmd, revision)
	if !ExposureWithin(assignment.Exposure, revision.DefaultExposure) {
		return nil, ErrExposureViolation
	}
	if err := assignment.UpdateKeys(); err != nil {
		return nil, errors.Join(ErrInvalidInput, err)
	}
	if err := s.repo.CreateSkillAssignment(ctx, assignment); err != nil {
		return nil, err
	}
	return assignment, nil
}

// RevokeAssignment revokes an existing subject assignment.
func (s *Service) RevokeAssignment(ctx context.Context, cmd AssignmentRevocationCommand) (*models.SkillAssignment, error) {
	if err := s.ensureRepo(); err != nil {
		return nil, err
	}
	assignment, err := s.repo.GetSkillAssignment(ctx, cmd.SubjectType, cmd.SubjectID, cmd.SkillID, cmd.AssignmentID)
	if err != nil {
		return nil, ErrSkillAssignmentNotFound
	}
	if assignment.Status == models.SkillAssignmentStatusRevoked {
		return nil, ErrInvalidState
	}
	if strings.TrimSpace(cmd.ActorUsername) == "" {
		return nil, errors.Join(ErrInvalidInput, errors.New("revocation actor is required"))
	}
	applyAssignmentRevocation(assignment, cmd.RevocationCommand, s.currentTime())
	if err := s.repo.UpdateSkillAssignment(ctx, assignment); err != nil {
		return nil, err
	}
	return assignment, nil
}

// PromoteProposal promotes accepted proposal output into an approved canonical revision.
func (s *Service) PromoteProposal(ctx context.Context, skillID string, cmd PromotionCommand) (*PromotionResult, error) {
	if err := s.ensureRepo(); err != nil {
		return nil, err
	}
	cmd = normalizePromotionCommand(cmd)
	if cmd.ActorUsername == "" {
		return nil, errors.Join(ErrInvalidInput, errors.New("promotion actor is required"))
	}
	if cmd.ProposalID == "" {
		return nil, errors.Join(ErrInvalidInput, errors.New("proposal id is required"))
	}
	skill, err := s.repo.GetSkill(ctx, skillID)
	if err != nil {
		return nil, ErrSkillNotFound
	}
	proposal, err := s.repo.GetSkillProposal(ctx, cmd.ProposalID)
	if err != nil {
		return nil, ErrSkillProposalNotFound
	}
	if !strings.EqualFold(strings.TrimSpace(proposal.SkillID), strings.TrimSpace(skill.ID)) {
		return nil, ErrPromotionConflict
	}
	if proposal.Status != models.SkillProposalStatusAccepted {
		return nil, ErrInvalidState
	}

	manifestJSON, manifestDigest, err := canonicalizeSkillManifest(proposal.ProposedManifestJSON)
	if err != nil {
		return nil, errors.Join(ErrInvalidInput, err)
	}
	if proposal.ProposedManifestDigest == "" {
		return nil, errors.Join(ErrInvalidInput, errors.New("proposal manifest digest is required"))
	}
	if !sameDigest(proposal.ProposedManifestDigest, manifestDigest) {
		return nil, ErrPromotionDigestMismatch
	}
	if cmd.ExpectedManifestDigest != "" && !sameDigest(cmd.ExpectedManifestDigest, manifestDigest) {
		return nil, ErrPromotionDigestMismatch
	}
	if cmd.ExpectedSourceDigest != "" && !sameDigest(cmd.ExpectedSourceDigest, proposal.SourceDigest) {
		return nil, ErrPromotionDigestMismatch
	}

	revision := buildPromotionRevision(skill, proposal, manifestJSON, manifestDigest, cmd, s.currentTime())
	if err := applyPromotionApproval(revision, proposal, cmd, s.currentTime()); err != nil {
		return nil, err
	}
	promotionDigest, err := models.SkillPromotionDigest(proposal, revision)
	if err != nil {
		return nil, errors.Join(ErrInvalidInput, err)
	}

	if result, ok, err := s.resolveExistingPromotion(ctx, skill, proposal, revision, promotionDigest, cmd.ActorUsername); ok || err != nil {
		return result, err
	}

	if err := s.repo.CreateSkillRevision(ctx, revision); err != nil {
		return nil, err
	}
	if err := s.promoteSkillPointer(ctx, skill, revision, cmd.ActorUsername); err != nil {
		return nil, err
	}
	applyProposalPromotion(proposal, revision, promotionDigest, cmd.ActorUsername, s.currentTime())
	if err := s.repo.UpdateSkillProposal(ctx, proposal); err != nil {
		return nil, err
	}
	return &PromotionResult{Revision: revision, Proposal: proposal, Created: true}, nil
}

// ResolveEffectiveSkills returns active approved skill revisions for a subject.
func (s *Service) ResolveEffectiveSkills(ctx context.Context, viewer Viewer, cmd ResolveCommand) (*ResolveResult, error) {
	if err := s.ensureRepo(); err != nil {
		return nil, err
	}
	cmd.SubjectType = strings.ToLower(strings.TrimSpace(cmd.SubjectType))
	cmd.SubjectID = strings.ToLower(strings.TrimSpace(cmd.SubjectID))
	if !CanResolveSubject(viewer, cmd.SubjectType, cmd.SubjectID) {
		return nil, ErrForbidden
	}
	assignments, cursor, err := s.repo.ListSkillAssignmentsForSubject(ctx, cmd.SubjectType, cmd.SubjectID, cmd.Limit, cmd.Cursor)
	if err != nil {
		return nil, err
	}
	items := make([]EffectiveSkill, 0, len(assignments))
	for _, assignment := range assignments {
		item, ok := s.resolveAssignment(ctx, viewer, assignment)
		if ok {
			items = append(items, item)
		}
	}
	return &ResolveResult{SubjectType: cmd.SubjectType, SubjectID: cmd.SubjectID, Items: items, NextCursor: cursor}, nil
}

// CanInspectSkill reports whether a viewer can inspect a skill record.
func CanInspectSkill(viewer Viewer, skill *models.Skill) bool {
	if skill == nil {
		return false
	}
	switch strings.TrimSpace(skill.DefaultExposure) {
	case models.SkillExposurePublic:
		return true
	case models.SkillExposureInstance:
		return viewer.Authenticated || viewer.Admin
	case models.SkillExposurePrivate:
		return viewer.Admin
	default:
		return false
	}
}

// CanInspectRevision reports whether a viewer can inspect a skill revision record.
func CanInspectRevision(viewer Viewer, revision *models.SkillRevision) bool {
	if revision == nil {
		return false
	}
	if viewer.Admin {
		return true
	}
	if revision.Status != models.SkillRevisionStatusApproved {
		return false
	}
	switch strings.TrimSpace(revision.DefaultExposure) {
	case models.SkillExposurePublic:
		return true
	case models.SkillExposureInstance:
		return viewer.Authenticated
	case models.SkillExposurePrivate:
		return false
	default:
		return false
	}
}

// CanResolveSubject reports whether the viewer can resolve a subject's effective skills.
func CanResolveSubject(viewer Viewer, subjectType, subjectID string) bool {
	if viewer.Admin {
		return true
	}
	if !viewer.Authenticated {
		return false
	}
	return strings.TrimSpace(subjectType) == models.SkillAssignmentSubjectActor &&
		strings.EqualFold(strings.TrimSpace(subjectID), strings.TrimSpace(viewer.Username))
}

// ExposureWithin reports whether requested exposure is no broader than allowed exposure.
func ExposureWithin(requested, allowed string) bool {
	return exposureRank(requested) <= exposureRank(allowed) && exposureRank(requested) >= 0
}

func (s *Service) listSkills(ctx context.Context, filter ListFilter) ([]*models.Skill, string, error) {
	exposure := strings.TrimSpace(strings.ToLower(filter.Exposure))
	if exposure != "" {
		return s.repo.ListSkillsByExposure(ctx, exposure, filter.Limit, filter.Cursor)
	}
	status := strings.TrimSpace(strings.ToLower(filter.Status))
	return s.repo.ListSkillsByStatus(ctx, status, filter.Limit, filter.Cursor)
}

func (s *Service) loadSkillRevision(ctx context.Context, skillID string, revisionNumber int) (*models.Skill, *models.SkillRevision, error) {
	skill, err := s.repo.GetSkill(ctx, skillID)
	if err != nil {
		return nil, nil, ErrSkillNotFound
	}
	revision, err := s.repo.GetSkillRevision(ctx, skillID, revisionNumber)
	if err != nil {
		return nil, nil, ErrSkillRevisionNotFound
	}
	return skill, revision, nil
}

func (s *Service) supersedeCurrentRevision(ctx context.Context, skill *models.Skill, revision *models.SkillRevision) error {
	if skill == nil || revision == nil || skill.CurrentRevisionNumber <= 0 || skill.CurrentRevisionNumber == revision.RevisionNumber {
		return nil
	}
	current, err := s.repo.GetSkillRevision(ctx, skill.ID, skill.CurrentRevisionNumber)
	if err != nil || current == nil || current.Status != models.SkillRevisionStatusApproved {
		return nil
	}
	current.Status = models.SkillRevisionStatusSuperseded
	current.UpdatedBy = revision.ApprovedBy
	current.UpdatedAt = s.currentTime()
	return s.repo.UpdateSkillRevision(ctx, current)
}

func (s *Service) assignmentTarget(ctx context.Context, cmd AssignmentCommand) (*models.Skill, *models.SkillRevision, error) {
	skill, err := s.repo.GetSkill(ctx, cmd.SkillID)
	if err != nil {
		return nil, nil, ErrSkillNotFound
	}
	revisionNumber := cmd.RevisionNumber
	if revisionNumber <= 0 {
		revisionNumber = skill.CurrentRevisionNumber
	}
	if revisionNumber <= 0 {
		return nil, nil, ErrInvalidState
	}
	revision, err := s.repo.GetSkillRevision(ctx, cmd.SkillID, revisionNumber)
	if err != nil {
		return nil, nil, ErrSkillRevisionNotFound
	}
	if revision.Status != models.SkillRevisionStatusApproved {
		return nil, nil, ErrInvalidState
	}
	return skill, revision, nil
}

func (s *Service) resolveAssignment(ctx context.Context, viewer Viewer, assignment *models.SkillAssignment) (EffectiveSkill, bool) {
	if assignment == nil || assignment.Status != models.SkillAssignmentStatusActive {
		return EffectiveSkill{}, false
	}
	if !canExposeAssignment(viewer, assignment) {
		return EffectiveSkill{}, false
	}
	skill, err := s.repo.GetSkill(ctx, assignment.SkillID)
	if err != nil {
		return EffectiveSkill{}, false
	}
	revisionNumber := assignment.RevisionNumber
	if revisionNumber <= 0 {
		revisionNumber = skill.CurrentRevisionNumber
	}
	revision, err := s.repo.GetSkillRevision(ctx, assignment.SkillID, revisionNumber)
	if err != nil || revision.Status != models.SkillRevisionStatusApproved {
		return EffectiveSkill{}, false
	}
	return EffectiveSkill{Skill: skill, Revision: revision, Assignment: assignment}, true
}

func (s *Service) ensureRepo() error {
	if s == nil || s.repo == nil {
		return ErrRepositoryUnavailable
	}
	return nil
}

func (s *Service) currentTime() time.Time {
	if s == nil || s.now == nil {
		return time.Now().UTC()
	}
	return s.now().UTC()
}

func canApproveRevision(status string) bool {
	switch status {
	case models.SkillRevisionStatusDraft, models.SkillRevisionStatusProposed:
		return true
	default:
		return false
	}
}

func applyApprovalDefaults(cmd *ApprovalCommand, revision *models.SkillRevision, now time.Time) {
	cmd.ActorUsername = strings.TrimSpace(cmd.ActorUsername)
	cmd.PrincipalID = defaultTrimmed(cmd.PrincipalID, cmd.ActorUsername)
	cmd.ApprovalAuthorityType = defaultTrimmed(cmd.ApprovalAuthorityType, models.SkillApprovalAuthorityAdmin)
	cmd.ApprovalAuthorityID = defaultTrimmed(cmd.ApprovalAuthorityID, cmd.ActorUsername)
	cmd.ApprovalID = defaultTrimmed(cmd.ApprovalID, generatedApprovalID(revision, now))
	if cmd.ApprovedAt.IsZero() {
		cmd.ApprovedAt = now
	}
}

func applyApprovalCommand(revision *models.SkillRevision, cmd ApprovalCommand) error {
	if strings.TrimSpace(cmd.ActorUsername) == "" {
		return errors.Join(ErrInvalidInput, errors.New("approval actor is required"))
	}
	digest, err := models.SkillRevisionApprovalDigest(revision, cmd.PrincipalID, cmd.ApprovalAuthorityType, cmd.ApprovalAuthorityID)
	if err != nil {
		return errors.Join(ErrInvalidInput, err)
	}
	if cmd.ApprovalDigest != "" && !strings.EqualFold(strings.TrimSpace(cmd.ApprovalDigest), digest) {
		return ErrApprovalDigestMismatch
	}
	revision.Status = models.SkillRevisionStatusApproved
	revision.ApprovalID = strings.TrimSpace(cmd.ApprovalID)
	revision.ApprovalAuthorityType = strings.ToLower(strings.TrimSpace(cmd.ApprovalAuthorityType))
	revision.ApprovalAuthorityID = strings.TrimSpace(cmd.ApprovalAuthorityID)
	revision.ApprovalDigest = digest
	revision.ApprovalSignature = strings.TrimSpace(cmd.ApprovalSignature)
	revision.ApprovalRef = strings.TrimSpace(cmd.ApprovalRef)
	revision.ApprovalReason = strings.TrimSpace(cmd.ApprovalReason)
	revision.ApprovedBy = strings.TrimSpace(cmd.ActorUsername)
	revision.ApprovedAt = ptrTime(cmd.ApprovedAt)
	revision.PrincipalID = strings.TrimSpace(cmd.PrincipalID)
	revision.PrincipalApprovalID = strings.TrimSpace(cmd.PrincipalApprovalID)
	revision.UpdatedBy = strings.TrimSpace(cmd.ActorUsername)
	appendApprovalProvenance(revision, cmd)
	return revision.UpdateKeys()
}

func appendApprovalProvenance(revision *models.SkillRevision, cmd ApprovalCommand) {
	ref := strings.TrimSpace(cmd.PrincipalApprovalID)
	if ref == "" {
		ref = strings.TrimSpace(cmd.ApprovalID)
	}
	uri := strings.TrimSpace(cmd.ApprovalRef)
	if ref == "" && uri == "" && revision.ApprovalDigest == "" {
		return
	}
	revision.Provenance = append(revision.Provenance, models.SkillProvenanceRef{
		SourceType: models.SkillSourceTypeApproval,
		SourceURI:  uri,
		Digest:     revision.ApprovalDigest,
		Ref:        ref,
	})
}

func applyRevisionRevocation(revision *models.SkillRevision, cmd RevocationCommand, now time.Time) {
	actor := strings.TrimSpace(cmd.ActorUsername)
	when := cmd.RevokedAt
	if when.IsZero() {
		when = now
	}
	revision.Status = models.SkillRevisionStatusRevoked
	revision.RevokedBy = actor
	revision.RevokedAt = ptrTime(when)
	revision.RevokedReason = strings.TrimSpace(cmd.Reason)
	revision.UpdatedBy = actor
	_ = revision.UpdateKeys()
}

func updateSkillCurrentRevision(skill *models.Skill, revision *models.SkillRevision, actor string) {
	skill.Status = models.SkillStatusActive
	skill.CurrentRevisionID = revision.ID
	skill.CurrentRevisionNumber = revision.RevisionNumber
	skill.DefaultExposure = revision.DefaultExposure
	skill.UpdatedBy = strings.TrimSpace(actor)
	_ = skill.UpdateKeys()
}

func clearSkillCurrentRevision(skill *models.Skill, actor string) {
	skill.Status = models.SkillStatusDraft
	skill.CurrentRevisionID = ""
	skill.CurrentRevisionNumber = 0
	skill.UpdatedBy = strings.TrimSpace(actor)
	_ = skill.UpdateKeys()
}

func applyAssignmentDefaults(cmd AssignmentCommand, skill *models.Skill, revision *models.SkillRevision, now time.Time) AssignmentCommand {
	cmd.ActorUsername = strings.TrimSpace(cmd.ActorUsername)
	cmd.SkillID = defaultTrimmed(cmd.SkillID, skill.ID)
	cmd.RevisionNumber = revision.RevisionNumber
	cmd.SubjectType = strings.ToLower(strings.TrimSpace(cmd.SubjectType))
	cmd.SubjectID = strings.ToLower(strings.TrimSpace(cmd.SubjectID))
	cmd.Exposure = defaultTrimmed(cmd.Exposure, models.SkillExposurePrivate)
	cmd.AssignmentID = defaultTrimmed(cmd.AssignmentID, generatedAssignmentID(cmd, revision, now))
	cmd.PrincipalID = defaultTrimmed(cmd.PrincipalID, cmd.ActorUsername)
	cmd.ApprovalID = defaultTrimmed(cmd.ApprovalID, "assignment-"+cmd.AssignmentID)
	if cmd.AssignedAt.IsZero() {
		cmd.AssignedAt = now
	}
	return cmd
}

func assignmentFromCommand(cmd AssignmentCommand, revision *models.SkillRevision) *models.SkillAssignment {
	return &models.SkillAssignment{
		ID:                  cmd.AssignmentID,
		SkillID:             cmd.SkillID,
		RevisionID:          revision.ID,
		RevisionNumber:      revision.RevisionNumber,
		SubjectType:         cmd.SubjectType,
		SubjectID:           cmd.SubjectID,
		Exposure:            cmd.Exposure,
		Status:              models.SkillAssignmentStatusActive,
		ApprovalID:          cmd.ApprovalID,
		PrincipalID:         cmd.PrincipalID,
		PrincipalApprovalID: strings.TrimSpace(cmd.PrincipalApprovalID),
		AssignedBy:          cmd.ActorUsername,
		AssignedAt:          cmd.AssignedAt,
	}
}

func applyAssignmentRevocation(assignment *models.SkillAssignment, cmd RevocationCommand, now time.Time) {
	actor := strings.TrimSpace(cmd.ActorUsername)
	when := cmd.RevokedAt
	if when.IsZero() {
		when = now
	}
	assignment.Status = models.SkillAssignmentStatusRevoked
	assignment.RevokedBy = actor
	assignment.RevokedAt = ptrTime(when)
	assignment.RevokedReason = strings.TrimSpace(cmd.Reason)
	_ = assignment.UpdateKeys()
}

func normalizePromotionCommand(cmd PromotionCommand) PromotionCommand {
	cmd.ActorUsername = strings.TrimSpace(cmd.ActorUsername)
	cmd.ProposalID = strings.ToLower(strings.TrimSpace(cmd.ProposalID))
	cmd.ExpectedManifestDigest = strings.ToLower(strings.TrimSpace(cmd.ExpectedManifestDigest))
	cmd.ExpectedSourceDigest = strings.ToLower(strings.TrimSpace(cmd.ExpectedSourceDigest))
	cmd.ApprovalID = strings.TrimSpace(cmd.ApprovalID)
	cmd.PrincipalID = strings.TrimSpace(cmd.PrincipalID)
	cmd.PrincipalApprovalID = strings.TrimSpace(cmd.PrincipalApprovalID)
	cmd.ApprovalAuthorityType = strings.ToLower(strings.TrimSpace(cmd.ApprovalAuthorityType))
	cmd.ApprovalAuthorityID = strings.TrimSpace(cmd.ApprovalAuthorityID)
	cmd.ApprovalDigest = strings.ToLower(strings.TrimSpace(cmd.ApprovalDigest))
	cmd.ApprovalSignature = strings.TrimSpace(cmd.ApprovalSignature)
	cmd.ApprovalRef = strings.TrimSpace(cmd.ApprovalRef)
	cmd.ApprovalReason = strings.TrimSpace(cmd.ApprovalReason)
	return cmd
}

func canonicalizeSkillManifest(raw string) (string, string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", "", errors.New("proposal manifest json is required")
	}
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.UseNumber()
	var manifest map[string]any
	if err := decoder.Decode(&manifest); err != nil {
		return "", "", fmt.Errorf("proposal manifest json is malformed: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return "", "", errors.New("proposal manifest json has trailing data")
	}
	if len(manifest) == 0 {
		return "", "", errors.New("proposal manifest json must be a non-empty object")
	}
	canonical, err := json.Marshal(manifest)
	if err != nil {
		return "", "", fmt.Errorf("canonicalize proposal manifest: %w", err)
	}
	sum := sha256.Sum256(canonical)
	return string(canonical), "sha256:" + hex.EncodeToString(sum[:]), nil
}

func buildPromotionRevision(skill *models.Skill, proposal *models.SkillProposal, manifestJSON, manifestDigest string, cmd PromotionCommand, now time.Time) *models.SkillRevision {
	revisionNumber := proposal.ProposedRevisionNumber
	if revisionNumber <= 0 && skill != nil {
		revisionNumber = skill.CurrentRevisionNumber + 1
	}
	if revisionNumber <= 0 {
		revisionNumber = 1
	}
	exposure := proposal.RequestedExposure
	if exposure == "" {
		exposure = models.SkillExposurePrivate
	}
	actor := strings.TrimSpace(cmd.ActorUsername)
	revision := &models.SkillRevision{
		SkillID:         skill.ID,
		RevisionNumber:  revisionNumber,
		Status:          models.SkillRevisionStatusProposed,
		ProposalID:      proposal.ID,
		ManifestJSON:    manifestJSON,
		ManifestDigest:  manifestDigest,
		ContentDigest:   proposal.SourceDigest,
		Capabilities:    capabilitiesFromManifest(manifestJSON),
		DefaultExposure: exposure,
		CreatedBy:       actor,
		UpdatedBy:       actor,
		CreatedAt:       now,
		UpdatedAt:       now,
		Provenance:      promotionProvenance(proposal, manifestDigest),
	}
	if revision.ID == "" {
		revision.ID = fmt.Sprintf("%s-r%d", skill.ID, revisionNumber)
	}
	return revision
}

func capabilitiesFromManifest(manifestJSON string) []string {
	var manifest struct {
		Capabilities []string `json:"capabilities"`
	}
	if err := json.Unmarshal([]byte(manifestJSON), &manifest); err != nil {
		return nil
	}
	return manifest.Capabilities
}

func promotionProvenance(proposal *models.SkillProposal, manifestDigest string) []models.SkillProvenanceRef {
	out := append([]models.SkillProvenanceRef(nil), proposal.Provenance...)
	sourceType := proposal.SourceType
	if sourceType == "" {
		sourceType = models.SkillSourceTypeManual
	}
	sourceRef := strings.TrimSpace(proposal.ConversationMessageID)
	if sourceRef == "" {
		sourceRef = strings.TrimSpace(proposal.ConversationID)
	}
	sourceDigest := strings.TrimSpace(proposal.SourceDigest)
	if sourceDigest == "" {
		sourceDigest = manifestDigest
	}
	out = append(out, models.SkillProvenanceRef{
		SourceType: sourceType,
		SourceURI:  strings.TrimSpace(proposal.SourceURI),
		Digest:     sourceDigest,
		Ref:        sourceRef,
	})
	out = append(out, models.SkillProvenanceRef{
		SourceType: models.SkillSourceTypeProposal,
		Digest:     manifestDigest,
		Ref:        proposal.ID,
	})
	return out
}

func applyPromotionApproval(revision *models.SkillRevision, proposal *models.SkillProposal, cmd PromotionCommand, now time.Time) error {
	approval := ApprovalCommand{
		ActorUsername:         cmd.ActorUsername,
		ApprovalID:            cmd.ApprovalID,
		PrincipalID:           defaultTrimmed(cmd.PrincipalID, proposal.PrincipalID),
		PrincipalApprovalID:   defaultTrimmed(cmd.PrincipalApprovalID, proposal.PrincipalApprovalID),
		ApprovalAuthorityType: cmd.ApprovalAuthorityType,
		ApprovalAuthorityID:   cmd.ApprovalAuthorityID,
		ApprovalDigest:        cmd.ApprovalDigest,
		ApprovalSignature:     cmd.ApprovalSignature,
		ApprovalRef:           cmd.ApprovalRef,
		ApprovalReason:        defaultTrimmed(cmd.ApprovalReason, proposal.ReviewReason),
		ApprovedAt:            cmd.ApprovedAt,
	}
	applyApprovalDefaults(&approval, revision, now)
	return applyApprovalCommand(revision, approval)
}

func (s *Service) resolveExistingPromotion(ctx context.Context, skill *models.Skill, proposal *models.SkillProposal, revision *models.SkillRevision, promotionDigest, actor string) (*PromotionResult, bool, error) {
	if proposal.PromotedRevisionID != "" || proposal.PromotedRevisionNumber > 0 || proposal.PromotionDigest != "" {
		if !samePromotionMetadata(proposal, revision, promotionDigest) {
			return nil, true, ErrPromotionConflict
		}
		existing, err := s.repo.GetSkillRevision(ctx, skill.ID, proposal.PromotedRevisionNumber)
		if err != nil {
			return nil, true, ErrPromotionConflict
		}
		if !samePromotedRevision(existing, proposal, revision) {
			return nil, true, ErrPromotionConflict
		}
		if err := s.promoteSkillPointer(ctx, skill, existing, actor); err != nil {
			return nil, true, err
		}
		return &PromotionResult{Revision: existing, Proposal: proposal, Created: false}, true, nil
	}

	existing, err := s.repo.GetSkillRevision(ctx, skill.ID, revision.RevisionNumber)
	if err == nil {
		return s.reconcileExistingPromotion(ctx, skill, proposal, existing, revision, promotionDigest, actor)
	}
	if !isSkillNotFound(err) {
		return nil, true, err
	}

	existing, err = s.repo.GetSkillRevisionByDigest(ctx, revision.ManifestDigest)
	if err == nil {
		return s.reconcileExistingPromotion(ctx, skill, proposal, existing, revision, promotionDigest, actor)
	}
	if !isSkillNotFound(err) {
		return nil, true, err
	}
	return nil, false, nil
}

func (s *Service) reconcileExistingPromotion(ctx context.Context, skill *models.Skill, proposal *models.SkillProposal, existing, expected *models.SkillRevision, promotionDigest, actor string) (*PromotionResult, bool, error) {
	if !samePromotedRevision(existing, proposal, expected) {
		return nil, true, ErrPromotionConflict
	}
	if err := s.promoteSkillPointer(ctx, skill, existing, actor); err != nil {
		return nil, true, err
	}
	applyProposalPromotion(proposal, existing, promotionDigest, actor, s.currentTime())
	if err := s.repo.UpdateSkillProposal(ctx, proposal); err != nil {
		return nil, true, err
	}
	return &PromotionResult{Revision: existing, Proposal: proposal, Created: false}, true, nil
}

func (s *Service) promoteSkillPointer(ctx context.Context, skill *models.Skill, revision *models.SkillRevision, actor string) error {
	if skill == nil || revision == nil {
		return ErrInvalidInput
	}
	if skill.CurrentRevisionNumber > revision.RevisionNumber {
		return nil
	}
	if skill.CurrentRevisionNumber != revision.RevisionNumber {
		if err := s.supersedeCurrentRevision(ctx, skill, revision); err != nil {
			return err
		}
	}
	updateSkillCurrentRevision(skill, revision, actor)
	skill.UpdatedAt = s.currentTime()
	return s.repo.UpdateSkill(ctx, skill)
}

func applyProposalPromotion(proposal *models.SkillProposal, revision *models.SkillRevision, promotionDigest, actor string, now time.Time) {
	proposal.Status = models.SkillProposalStatusAccepted
	proposal.ProposedRevisionNumber = revision.RevisionNumber
	proposal.ProposedManifestDigest = revision.ManifestDigest
	proposal.PromotedRevisionID = revision.ID
	proposal.PromotedRevisionNumber = revision.RevisionNumber
	proposal.PromotionDigest = promotionDigest
	proposal.PromotedBy = strings.TrimSpace(actor)
	proposal.PromotedAt = ptrTime(now)
	proposal.UpdatedAt = now.UTC()
	_ = proposal.UpdateKeys()
}

func samePromotionMetadata(proposal *models.SkillProposal, revision *models.SkillRevision, promotionDigest string) bool {
	return proposal.PromotedRevisionNumber == revision.RevisionNumber &&
		strings.EqualFold(proposal.PromotedRevisionID, revision.ID) &&
		sameDigest(proposal.PromotionDigest, promotionDigest)
}

func samePromotedRevision(existing *models.SkillRevision, proposal *models.SkillProposal, expected *models.SkillRevision) bool {
	if existing == nil || proposal == nil || expected == nil {
		return false
	}
	return existing.Status == models.SkillRevisionStatusApproved &&
		strings.EqualFold(existing.SkillID, expected.SkillID) &&
		existing.RevisionNumber == expected.RevisionNumber &&
		strings.EqualFold(existing.ProposalID, proposal.ID) &&
		sameDigest(existing.ManifestDigest, expected.ManifestDigest) &&
		sameDigest(existing.ApprovalDigest, expected.ApprovalDigest)
}

func sameDigest(left, right string) bool {
	left = strings.ToLower(strings.TrimSpace(left))
	right = strings.ToLower(strings.TrimSpace(right))
	return left != "" && left == right
}

func isSkillNotFound(err error) bool {
	if err == nil {
		return false
	}
	return tableerrors.IsNotFound(err) || strings.Contains(strings.ToLower(err.Error()), "not found")
}

func canExposeAssignment(viewer Viewer, assignment *models.SkillAssignment) bool {
	if assignment == nil {
		return false
	}
	switch assignment.Exposure {
	case models.SkillExposurePublic:
		return true
	case models.SkillExposureInstance:
		return viewer.Authenticated || viewer.Admin
	case models.SkillExposurePrivate:
		return viewer.Admin || CanResolveSubject(viewer, assignment.SubjectType, assignment.SubjectID)
	default:
		return false
	}
}

func exposureRank(value string) int {
	switch strings.TrimSpace(value) {
	case models.SkillExposurePrivate:
		return 0
	case models.SkillExposureInstance:
		return 1
	case models.SkillExposurePublic:
		return 2
	default:
		return -1
	}
}

func generatedApprovalID(revision *models.SkillRevision, now time.Time) string {
	if revision == nil {
		return fmt.Sprintf("approval-%d", now.UnixNano())
	}
	return fmt.Sprintf("approval-%s-r%d-%d", revision.SkillID, revision.RevisionNumber, now.UnixNano())
}

func generatedAssignmentID(cmd AssignmentCommand, revision *models.SkillRevision, now time.Time) string {
	return fmt.Sprintf("assign-%s-%s-r%d-%d", cmd.SubjectType, cmd.SkillID, revision.RevisionNumber, now.UnixNano())
}

func defaultTrimmed(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return strings.TrimSpace(fallback)
	}
	return value
}

func validateListFilter(filter ListFilter) error {
	if filter.Exposure != "" && exposureRank(strings.ToLower(strings.TrimSpace(filter.Exposure))) < 0 {
		return errors.Join(ErrInvalidInput, errors.New("unsupported exposure"))
	}
	if filter.Status == "" {
		return nil
	}
	switch strings.ToLower(strings.TrimSpace(filter.Status)) {
	case models.SkillStatusDraft, models.SkillStatusActive, models.SkillStatusArchived:
		return nil
	default:
		return errors.Join(ErrInvalidInput, errors.New("unsupported status"))
	}
}

func ptrTime(value time.Time) *time.Time {
	value = value.UTC()
	return &value
}

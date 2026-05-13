package handlers

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/auth"
	skillservice "github.com/equaltoai/lesser/pkg/services/skills"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
	apptheory "github.com/theory-cloud/apptheory/runtime"
)

func TestSkillHandlers_ServiceErrorMapping(t *testing.T) {
	t.Parallel()

	h := &Handler{cfg: round10TestConfig(), logger: round10TestLogger(t)}
	ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/skills/missing", nil, nil, nil)
	require.NoError(t, err)

	cases := []struct {
		name       string
		err        error
		wantStatus int
	}{
		{name: "not_found", err: skillservice.ErrSkillNotFound, wantStatus: http.StatusNotFound},
		{name: "forbidden", err: skillservice.ErrForbidden, wantStatus: http.StatusForbidden},
		{name: "invalid_input", err: skillservice.ErrInvalidInput, wantStatus: http.StatusBadRequest},
		{name: "invalid_state", err: skillservice.ErrInvalidState, wantStatus: http.StatusUnprocessableEntity},
		{name: "digest_mismatch", err: skillservice.ErrApprovalDigestMismatch, wantStatus: http.StatusUnprocessableEntity},
		{name: "exposure_violation", err: skillservice.ErrExposureViolation, wantStatus: http.StatusForbidden},
		{name: "unknown", err: errors.New("boom"), wantStatus: http.StatusInternalServerError},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			resp := requireStatus(t, tc.wantStatus)(h.respondSkillServiceError(ctx, tc.err))
			require.Contains(t, string(resp.Body), "error")
		})
	}
}

func TestSkillHandlers_ToAPIRevisionIncludesApprovalSemantics(t *testing.T) {
	t.Parallel()

	revision := &storagemodels.SkillRevision{
		ID:                    "skill-a-r1",
		SkillID:               "skill-a",
		RevisionNumber:        1,
		Status:                storagemodels.SkillRevisionStatusApproved,
		DefaultExposure:       storagemodels.SkillExposureInstance,
		ApprovalID:            "approval-1",
		ApprovalAuthorityType: storagemodels.SkillApprovalAuthorityAdmin,
		ApprovalAuthorityID:   "ops",
		ApprovalDigest:        "sha256:approval",
		ApprovedBy:            "ops",
		PrincipalID:           "principal-1",
		Provenance: []storagemodels.SkillProvenanceRef{{
			SourceType: storagemodels.SkillSourceTypeApproval,
			Digest:     "sha256:approval",
			Ref:        "approval-1",
		}},
	}

	out := toAPISkillRevision(revision)
	require.Equal(t, apimodels.SkillRevisionResource{
		ID:                    "skill-a-r1",
		SkillID:               "skill-a",
		RevisionNumber:        1,
		Status:                storagemodels.SkillRevisionStatusApproved,
		DefaultExposure:       storagemodels.SkillExposureInstance,
		ApprovalID:            "approval-1",
		ApprovalAuthorityType: storagemodels.SkillApprovalAuthorityAdmin,
		ApprovalAuthorityID:   "ops",
		ApprovalDigest:        "sha256:approval",
		ApprovedBy:            "ops",
		PrincipalID:           "principal-1",
		Provenance: []apimodels.SkillProvenanceRef{{
			SourceType: storagemodels.SkillSourceTypeApproval,
			Digest:     "sha256:approval",
			Ref:        "approval-1",
		}},
	}, out)
}

type skillHandlerStub struct {
	nilSkillService
	skill       *storagemodels.Skill
	revisions   []*storagemodels.SkillRevision
	proposal    *storagemodels.SkillProposal
	assignment  *storagemodels.SkillAssignment
	resolveItem skillservice.EffectiveSkill
}

func (s *skillHandlerStub) ListSkills(context.Context, skillservice.Viewer, skillservice.ListFilter) ([]*storagemodels.Skill, string, error) {
	return []*storagemodels.Skill{s.skill}, "next", nil
}

func (s *skillHandlerStub) GetSkill(context.Context, skillservice.Viewer, string) (*storagemodels.Skill, error) {
	return s.skill, nil
}

func (s *skillHandlerStub) ListRevisions(context.Context, skillservice.Viewer, string, int, string) ([]*storagemodels.SkillRevision, string, error) {
	return s.revisions, "next-revision", nil
}

func (s *skillHandlerStub) GetRevision(context.Context, skillservice.Viewer, string, int) (*storagemodels.SkillRevision, error) {
	return s.revisions[0], nil
}

func TestSkillHandlers_PublicReadHandlers(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 5, 13, 12, 0, 0, 0, time.UTC)
	stub := &skillHandlerStub{
		skill: &storagemodels.Skill{
			ID:                    "skill-a",
			Slug:                  "skill-a",
			Name:                  "Skill A",
			Status:                storagemodels.SkillStatusActive,
			DefaultExposure:       storagemodels.SkillExposurePublic,
			CurrentRevisionID:     "skill-a-r1",
			CurrentRevisionNumber: 1,
			CreatedAt:             now,
			UpdatedAt:             now,
		},
		revisions: []*storagemodels.SkillRevision{{
			ID:              "skill-a-r1",
			SkillID:         "skill-a",
			RevisionNumber:  1,
			Status:          storagemodels.SkillRevisionStatusApproved,
			DefaultExposure: storagemodels.SkillExposurePublic,
			CreatedAt:       now,
			UpdatedAt:       now,
		}},
	}
	h := &Handler{cfg: round10TestConfig(), logger: round10TestLogger(t), skillsService: stub}

	ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/skills", nil, map[string]string{"limit": "1"}, nil)
	require.NoError(t, err)
	resp := requireStatus(t, http.StatusOK)(h.HandleListSkillsLift(ctx))
	require.Contains(t, string(resp.Body), "skill-a")
	require.Contains(t, string(resp.Body), "next")

	ctx, err = round10NewLiftContext(http.MethodGet, "/api/v1/skills/skill-a", nil, nil, nil)
	require.NoError(t, err)
	ctx.Params["skillId"] = "skill-a"
	resp = requireStatus(t, http.StatusOK)(h.HandleGetSkillLift(ctx))
	require.Contains(t, string(resp.Body), "Skill A")

	ctx, err = round10NewLiftContext(http.MethodGet, "/api/v1/skills/skill-a/revisions", nil, nil, nil)
	require.NoError(t, err)
	ctx.Params["skillId"] = "skill-a"
	resp = requireStatus(t, http.StatusOK)(h.HandleListSkillRevisionsLift(ctx))
	require.Contains(t, string(resp.Body), "next-revision")

	ctx, err = round10NewLiftContext(http.MethodGet, "/api/v1/skills/skill-a/revisions/1", nil, nil, nil)
	require.NoError(t, err)
	ctx.Params["skillId"] = "skill-a"
	ctx.Params["revisionNumber"] = "1"
	resp = requireStatus(t, http.StatusOK)(h.HandleGetSkillRevisionLift(ctx))
	require.Contains(t, string(resp.Body), "skill-a-r1")

	ctx.Params["revisionNumber"] = "bad"
	resp = requireStatus(t, http.StatusBadRequest)(h.HandleGetSkillRevisionLift(ctx))
	require.Contains(t, string(resp.Body), "revision_number")
}

func TestSkillHandlers_HelperCoverage(t *testing.T) {
	t.Parallel()

	require.Equal(t, 25, parseSkillLimit(""))
	require.Equal(t, 25, parseSkillLimit("-1"))
	require.Equal(t, 100, parseSkillLimit("500"))
	require.Equal(t, 10, parseSkillLimit("10"))
	_, err := parseSkillRevisionNumber("0")
	require.Error(t, err)
	gotRevision, err := parseSkillRevisionNumber("7")
	require.NoError(t, err)
	require.Equal(t, 7, gotRevision)

	require.IsType(t, nilSkillService{}, (&Handler{}).getSkillService())
	require.IsType(t, nilSkillService{}, ((*Handler)(nil)).getSkillService())
	stub := &skillHandlerStub{}
	require.Same(t, stub, (&Handler{skillsService: stub}).getSkillService())

	ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/skills/resolve", nil, nil, nil)
	require.NoError(t, err)
	resp := requireStatus(t, http.StatusUnauthorized)((&Handler{cfg: round10TestConfig(), logger: round10TestLogger(t)}).respondSkillAuthError(ctx, skillservice.ErrForbidden))
	require.Contains(t, string(resp.Body), "error")
}

func TestSkillHandlers_MappingHelpers(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 5, 13, 12, 0, 0, 0, time.UTC)
	revokedAt := now.Add(time.Hour)
	provenance := []storagemodels.SkillProvenanceRef{{SourceType: storagemodels.SkillSourceTypeManual, SourceURI: "file://skill", Digest: "sha256:source", Ref: "ref", Notes: "note"}}

	require.Empty(t, toAPISkill(nil).ID)
	require.Empty(t, toAPISkillRevision(nil).ID)
	require.Empty(t, toAPISkillProposal(nil).ID)
	require.Empty(t, toAPISkillAssignment(nil).ID)
	require.Nil(t, toAPISkillProvenance(nil))
	require.Nil(t, toAPISkillRevisionFiles(nil))

	skill := toAPISkill(&storagemodels.Skill{
		ID: "skill-a", Slug: "skill-a", Name: "Skill A", Description: "desc", Status: storagemodels.SkillStatusActive,
		DefaultExposure: storagemodels.SkillExposurePublic, CurrentRevisionID: "skill-a-r1", CurrentRevisionNumber: 1,
		Capabilities: []string{"cap"}, Tags: []string{"tag"}, Provenance: provenance, CreatedBy: "ops", UpdatedBy: "ops", CreatedAt: now, UpdatedAt: now, Version: 2,
	})
	require.Equal(t, "skill-a", skill.ID)
	require.Equal(t, []string{"cap"}, skill.Capabilities)
	require.Equal(t, "sha256:source", skill.Provenance[0].Digest)

	revision := toAPISkillRevision(&storagemodels.SkillRevision{
		ID: "skill-a-r1", SkillID: "skill-a", RevisionNumber: 1, Status: storagemodels.SkillRevisionStatusApproved,
		ProposalID: "proposal-1", ManifestDigest: "sha256:manifest", BundleDigest: "sha256:bundle", ContentDigest: "sha256:content",
		Files:        []storagemodels.SkillRevisionFile{{Path: "SKILL.md", Digest: "sha256:file", ContentType: "text/markdown", Role: "manifest", SizeBytes: 42}},
		Capabilities: []string{"cap"}, DefaultExposure: storagemodels.SkillExposurePublic, ApprovalID: "approval-1",
		ApprovalAuthorityType: storagemodels.SkillApprovalAuthorityAdmin, ApprovalAuthorityID: "ops", ApprovalDigest: "sha256:approval",
		ApprovalSignature: "sig", ApprovalRef: "ref", ApprovalReason: "reason", ApprovedBy: "ops", ApprovedAt: &now,
		PrincipalID: "principal-1", PrincipalApprovalID: "principal-approval-1", Provenance: provenance,
		RevokedBy: "ops", RevokedAt: &revokedAt, RevokedReason: "rotated", CreatedBy: "ops", UpdatedBy: "ops", CreatedAt: now, UpdatedAt: now, Version: 3,
	})
	require.Equal(t, "SKILL.md", revision.Files[0].Path)
	require.Equal(t, "principal-approval-1", revision.PrincipalApprovalID)

	proposal := toAPISkillProposal(&storagemodels.SkillProposal{
		ID: "proposal-1", SkillID: "skill-a", Title: "Title", Summary: "Summary", Status: storagemodels.SkillProposalStatusProposed,
		RequestedExposure: storagemodels.SkillExposurePublic, ProposedRevisionNumber: 2, ProposedManifestDigest: "sha256:manifest",
		SourceType: storagemodels.SkillSourceTypeLocalFile, SourceURI: "file://skill", SourceDigest: "sha256:source",
		ConversationID: "conv", ConversationMessageID: "msg", PrincipalID: "principal-1", PrincipalApprovalID: "principal-approval-1",
		Provenance: provenance, CreatedBy: "ops", ReviewedBy: "ops", ReviewedAt: &now, ReviewReason: "ok", CreatedAt: now, UpdatedAt: now, Version: 4,
	})
	require.Equal(t, "proposal-1", proposal.ID)
	require.Equal(t, "principal-approval-1", proposal.PrincipalApprovalID)

	assignment := toAPISkillAssignment(&storagemodels.SkillAssignment{
		ID: "assign-1", SkillID: "skill-a", RevisionID: "skill-a-r1", RevisionNumber: 1,
		SubjectType: storagemodels.SkillAssignmentSubjectActor, SubjectID: "alice", Exposure: storagemodels.SkillExposurePrivate,
		Status: storagemodels.SkillAssignmentStatusRevoked, ApprovalID: "approval-1", PrincipalID: "principal-1", PrincipalApprovalID: "principal-approval-1",
		Provenance: provenance, AssignedBy: "ops", AssignedAt: now, RevokedBy: "ops", RevokedAt: &revokedAt, RevokedReason: "rotated", CreatedAt: now, UpdatedAt: now, Version: 5,
	})
	require.Equal(t, "assign-1", assignment.ID)
	require.Equal(t, "rotated", assignment.RevokedReason)
}

func (s *skillHandlerStub) ListProposals(context.Context, string, string, int, string) ([]*storagemodels.SkillProposal, string, error) {
	return []*storagemodels.SkillProposal{s.proposal}, "next-proposal", nil
}

func (s *skillHandlerStub) GetProposal(context.Context, string) (*storagemodels.SkillProposal, error) {
	return s.proposal, nil
}

func (s *skillHandlerStub) ListAssignmentsForSkill(context.Context, string, int, string) ([]*storagemodels.SkillAssignment, string, error) {
	return []*storagemodels.SkillAssignment{s.assignment}, "next-assignment", nil
}

func (s *skillHandlerStub) ApproveRevision(context.Context, string, int, skillservice.ApprovalCommand) (*storagemodels.SkillRevision, error) {
	return s.revisions[0], nil
}

func (s *skillHandlerStub) RevokeRevision(context.Context, string, int, skillservice.RevocationCommand) (*storagemodels.SkillRevision, error) {
	return s.revisions[0], nil
}

func (s *skillHandlerStub) AssignSkill(context.Context, skillservice.AssignmentCommand) (*storagemodels.SkillAssignment, error) {
	return s.assignment, nil
}

func (s *skillHandlerStub) RevokeAssignment(context.Context, skillservice.AssignmentRevocationCommand) (*storagemodels.SkillAssignment, error) {
	return s.assignment, nil
}

func (s *skillHandlerStub) ResolveEffectiveSkills(context.Context, skillservice.Viewer, skillservice.ResolveCommand) (*skillservice.ResolveResult, error) {
	return &skillservice.ResolveResult{SubjectType: storagemodels.SkillAssignmentSubjectActor, SubjectID: "alice", Items: []skillservice.EffectiveSkill{s.resolveItem}, NextCursor: "next-effective"}, nil
}

type skillHandlerErrorStub struct {
	nilSkillService
	err error
}

func (s skillHandlerErrorStub) ListSkills(context.Context, skillservice.Viewer, skillservice.ListFilter) ([]*storagemodels.Skill, string, error) {
	return nil, "", s.err
}

func (s skillHandlerErrorStub) GetSkill(context.Context, skillservice.Viewer, string) (*storagemodels.Skill, error) {
	return nil, s.err
}

func (s skillHandlerErrorStub) ListRevisions(context.Context, skillservice.Viewer, string, int, string) ([]*storagemodels.SkillRevision, string, error) {
	return nil, "", s.err
}

func (s skillHandlerErrorStub) GetRevision(context.Context, skillservice.Viewer, string, int) (*storagemodels.SkillRevision, error) {
	return nil, s.err
}

func (s skillHandlerErrorStub) ListProposals(context.Context, string, string, int, string) ([]*storagemodels.SkillProposal, string, error) {
	return nil, "", s.err
}

func (s skillHandlerErrorStub) GetProposal(context.Context, string) (*storagemodels.SkillProposal, error) {
	return nil, s.err
}

func (s skillHandlerErrorStub) ListAssignmentsForSkill(context.Context, string, int, string) ([]*storagemodels.SkillAssignment, string, error) {
	return nil, "", s.err
}

func (s skillHandlerErrorStub) ApproveRevision(context.Context, string, int, skillservice.ApprovalCommand) (*storagemodels.SkillRevision, error) {
	return nil, s.err
}

func (s skillHandlerErrorStub) RevokeRevision(context.Context, string, int, skillservice.RevocationCommand) (*storagemodels.SkillRevision, error) {
	return nil, s.err
}

func (s skillHandlerErrorStub) AssignSkill(context.Context, skillservice.AssignmentCommand) (*storagemodels.SkillAssignment, error) {
	return nil, s.err
}

func (s skillHandlerErrorStub) RevokeAssignment(context.Context, skillservice.AssignmentRevocationCommand) (*storagemodels.SkillAssignment, error) {
	return nil, s.err
}

func (s skillHandlerErrorStub) ResolveEffectiveSkills(context.Context, skillservice.Viewer, skillservice.ResolveCommand) (*skillservice.ResolveResult, error) {
	return nil, s.err
}

func TestSkillHandlers_ServiceErrorsFromEndpoints(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 5, 13, 12, 0, 0, 0, time.UTC)
	state := &round10QueryState{usersByUsername: map[string]storagemodels.User{
		"admin": {PK: "USER#admin", SK: storagemodels.SKMetadata, Username: "admin", Role: "admin", Approved: true, Version: 1, CreatedAt: now, UpdatedAt: now},
	}}
	cfg := round11TestConfig()
	h, _, _ := round11NewHandler(t, cfg, state)
	h.skillsService = skillHandlerErrorStub{err: skillservice.ErrInvalidInput}
	readHeaders := map[string]string{"Authorization": "Bearer " + round11SignAccessToken(t, cfg.JWTSecret, "admin", []string{auth.ScopeRead})}
	adminReadHeaders := map[string]string{"Authorization": "Bearer " + round11SignAccessToken(t, cfg.JWTSecret, "admin", []string{"admin:read"})}
	adminWriteHeaders := map[string]string{"Authorization": "Bearer " + round11SignAccessToken(t, cfg.JWTSecret, "admin", []string{"admin:write"})}

	cases := []struct {
		name    string
		context func() *apptheory.Context
		handle  func(*apptheory.Context) (*apptheory.Response, error)
	}{
		{
			name: "list skills",
			context: func() *apptheory.Context {
				ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/skills", nil, nil, nil)
				require.NoError(t, err)
				return ctx
			},
			handle: h.HandleListSkillsLift,
		},
		{
			name: "get skill",
			context: func() *apptheory.Context {
				ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/skills/skill-a", nil, nil, nil)
				require.NoError(t, err)
				ctx.Params["skillId"] = "skill-a"
				return ctx
			},
			handle: h.HandleGetSkillLift,
		},
		{
			name: "list revisions",
			context: func() *apptheory.Context {
				ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/skills/skill-a/revisions", nil, nil, nil)
				require.NoError(t, err)
				ctx.Params["skillId"] = "skill-a"
				return ctx
			},
			handle: h.HandleListSkillRevisionsLift,
		},
		{
			name: "get revision",
			context: func() *apptheory.Context {
				ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/skills/skill-a/revisions/1", nil, nil, nil)
				require.NoError(t, err)
				ctx.Params["skillId"] = "skill-a"
				ctx.Params["revisionNumber"] = "1"
				return ctx
			},
			handle: h.HandleGetSkillRevisionLift,
		},
		{
			name: "resolve",
			context: func() *apptheory.Context {
				ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/skills/resolve", readHeaders, map[string]string{"subject_type": "actor", "subject_id": "alice"}, nil)
				require.NoError(t, err)
				return ctx
			},
			handle: h.HandleResolveEffectiveSkillsLift,
		},
		{
			name: "list proposals",
			context: func() *apptheory.Context {
				ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/admin/skills/proposals", adminReadHeaders, nil, nil)
				require.NoError(t, err)
				return ctx
			},
			handle: h.HandleAdminListSkillProposalsLift,
		},
		{
			name: "get proposal",
			context: func() *apptheory.Context {
				ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/admin/skills/proposals/proposal-1", adminReadHeaders, nil, nil)
				require.NoError(t, err)
				ctx.Params["proposalId"] = "proposal-1"
				return ctx
			},
			handle: h.HandleAdminGetSkillProposalLift,
		},
		{
			name: "list assignments",
			context: func() *apptheory.Context {
				ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/admin/skills/skill-a/assignments", adminReadHeaders, nil, nil)
				require.NoError(t, err)
				ctx.Params["skillId"] = "skill-a"
				return ctx
			},
			handle: h.HandleAdminListSkillAssignmentsLift,
		},
		{
			name: "approve",
			context: func() *apptheory.Context {
				ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/skills/skill-a/revisions/1/approve", adminWriteHeaders, nil, apimodels.ApproveSkillRevisionRequest{PrincipalID: "principal-1"})
				require.NoError(t, err)
				ctx.Params["skillId"] = "skill-a"
				ctx.Params["revisionNumber"] = "1"
				return ctx
			},
			handle: h.HandleAdminApproveSkillRevisionLift,
		},
		{
			name: "revoke revision",
			context: func() *apptheory.Context {
				ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/skills/skill-a/revisions/1/revoke", adminWriteHeaders, nil, apimodels.RevokeSkillRevisionRequest{Reason: "rotated"})
				require.NoError(t, err)
				ctx.Params["skillId"] = "skill-a"
				ctx.Params["revisionNumber"] = "1"
				return ctx
			},
			handle: h.HandleAdminRevokeSkillRevisionLift,
		},
		{
			name: "create assignment",
			context: func() *apptheory.Context {
				ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/skills/skill-a/assignments", adminWriteHeaders, nil, apimodels.CreateSkillAssignmentRequest{SubjectType: "actor", SubjectID: "alice", RevisionNumber: 1})
				require.NoError(t, err)
				ctx.Params["skillId"] = "skill-a"
				return ctx
			},
			handle: h.HandleAdminCreateSkillAssignmentLift,
		},
		{
			name: "revoke assignment",
			context: func() *apptheory.Context {
				ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/skills/skill-a/assignments/assign-1/revoke", adminWriteHeaders, nil, apimodels.RevokeSkillAssignmentRequest{SubjectType: "actor", SubjectID: "alice", Reason: "rotated"})
				require.NoError(t, err)
				ctx.Params["skillId"] = "skill-a"
				ctx.Params["assignmentId"] = "assign-1"
				return ctx
			},
			handle: h.HandleAdminRevokeSkillAssignmentLift,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			resp := requireStatus(t, http.StatusBadRequest)(tc.handle(tc.context()))
			require.Contains(t, string(resp.Body), "error")
		})
	}
}

func TestSkillHandlers_AuthenticatedAndAdminHandlers(t *testing.T) {
	now := time.Date(2026, 5, 13, 12, 0, 0, 0, time.UTC)
	state := &round10QueryState{usersByUsername: map[string]storagemodels.User{
		"admin": {PK: "USER#admin", SK: storagemodels.SKMetadata, Username: "admin", Role: "admin", Approved: true, Version: 1, CreatedAt: now, UpdatedAt: now},
	}}
	cfg := round11TestConfig()
	h, _, _ := round11NewHandler(t, cfg, state)
	stub := &skillHandlerStub{
		skill:      &storagemodels.Skill{ID: "skill-a", Slug: "skill-a", Name: "Skill A", Status: storagemodels.SkillStatusActive, DefaultExposure: storagemodels.SkillExposurePublic, CreatedAt: now, UpdatedAt: now},
		revisions:  []*storagemodels.SkillRevision{{ID: "skill-a-r1", SkillID: "skill-a", RevisionNumber: 1, Status: storagemodels.SkillRevisionStatusApproved, DefaultExposure: storagemodels.SkillExposurePublic, CreatedAt: now, UpdatedAt: now}},
		proposal:   &storagemodels.SkillProposal{ID: "proposal-1", SkillID: "skill-a", Status: storagemodels.SkillProposalStatusProposed, RequestedExposure: storagemodels.SkillExposurePublic, CreatedAt: now, UpdatedAt: now},
		assignment: &storagemodels.SkillAssignment{ID: "assign-1", SkillID: "skill-a", RevisionNumber: 1, SubjectType: storagemodels.SkillAssignmentSubjectActor, SubjectID: "alice", Exposure: storagemodels.SkillExposurePublic, Status: storagemodels.SkillAssignmentStatusActive, AssignedAt: now, CreatedAt: now, UpdatedAt: now},
	}
	stub.resolveItem = skillservice.EffectiveSkill{Skill: stub.skill, Revision: stub.revisions[0], Assignment: stub.assignment}
	h.skillsService = stub
	readHeaders := map[string]string{"Authorization": "Bearer " + round11SignAccessToken(t, cfg.JWTSecret, "admin", []string{auth.ScopeRead})}
	adminReadHeaders := map[string]string{"Authorization": "Bearer " + round11SignAccessToken(t, cfg.JWTSecret, "admin", []string{"admin:read"})}
	adminWriteHeaders := map[string]string{"Authorization": "Bearer " + round11SignAccessToken(t, cfg.JWTSecret, "admin", []string{"admin:write"})}

	ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/skills/resolve", readHeaders, map[string]string{"subject_type": "actor", "subject_id": "alice"}, nil)
	require.NoError(t, err)
	resp := requireStatus(t, http.StatusOK)(h.HandleResolveEffectiveSkillsLift(ctx))
	require.Contains(t, string(resp.Body), "next-effective")

	ctx, err = round10NewLiftContext(http.MethodGet, "/api/v1/admin/skills/proposals", adminReadHeaders, nil, nil)
	require.NoError(t, err)
	resp = requireStatus(t, http.StatusOK)(h.HandleAdminListSkillProposalsLift(ctx))
	require.Contains(t, string(resp.Body), "next-proposal")

	ctx, err = round10NewLiftContext(http.MethodGet, "/api/v1/admin/skills/proposals/proposal-1", adminReadHeaders, nil, nil)
	require.NoError(t, err)
	ctx.Params["proposalId"] = "proposal-1"
	resp = requireStatus(t, http.StatusOK)(h.HandleAdminGetSkillProposalLift(ctx))
	require.Contains(t, string(resp.Body), "proposal-1")

	ctx, err = round10NewLiftContext(http.MethodGet, "/api/v1/admin/skills/skill-a/assignments", adminReadHeaders, nil, nil)
	require.NoError(t, err)
	ctx.Params["skillId"] = "skill-a"
	resp = requireStatus(t, http.StatusOK)(h.HandleAdminListSkillAssignmentsLift(ctx))
	require.Contains(t, string(resp.Body), "next-assignment")

	ctx, err = round10NewLiftContext(http.MethodPost, "/api/v1/admin/skills/skill-a/revisions/1/approve", adminWriteHeaders, nil, apimodels.ApproveSkillRevisionRequest{PrincipalID: "principal-1"})
	require.NoError(t, err)
	ctx.Params["skillId"] = "skill-a"
	ctx.Params["revisionNumber"] = "1"
	resp = requireStatus(t, http.StatusOK)(h.HandleAdminApproveSkillRevisionLift(ctx))
	require.Contains(t, string(resp.Body), "skill-a-r1")

	ctx, err = round10NewLiftContext(http.MethodPost, "/api/v1/admin/skills/skill-a/revisions/1/revoke", adminWriteHeaders, nil, apimodels.RevokeSkillRevisionRequest{Reason: "rotated"})
	require.NoError(t, err)
	ctx.Params["skillId"] = "skill-a"
	ctx.Params["revisionNumber"] = "1"
	resp = requireStatus(t, http.StatusOK)(h.HandleAdminRevokeSkillRevisionLift(ctx))
	require.Contains(t, string(resp.Body), "skill-a-r1")

	ctx, err = round10NewLiftContext(http.MethodPost, "/api/v1/admin/skills/skill-a/assignments", adminWriteHeaders, nil, apimodels.CreateSkillAssignmentRequest{SubjectType: "actor", SubjectID: "alice", RevisionNumber: 1})
	require.NoError(t, err)
	ctx.Params["skillId"] = "skill-a"
	resp = requireStatus(t, http.StatusCreated)(h.HandleAdminCreateSkillAssignmentLift(ctx))
	require.Contains(t, string(resp.Body), "assign-1")

	ctx, err = round10NewLiftContext(http.MethodPost, "/api/v1/admin/skills/skill-a/assignments/assign-1/revoke", adminWriteHeaders, nil, apimodels.RevokeSkillAssignmentRequest{SubjectType: "actor", SubjectID: "alice", Reason: "rotated"})
	require.NoError(t, err)
	ctx.Params["skillId"] = "skill-a"
	ctx.Params["assignmentId"] = "assign-1"
	resp = requireStatus(t, http.StatusOK)(h.HandleAdminRevokeSkillAssignmentLift(ctx))
	require.Contains(t, string(resp.Body), "assign-1")
}

func TestSkillHandlers_AdminValidationBranches(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 5, 13, 12, 0, 0, 0, time.UTC)
	state := &round10QueryState{usersByUsername: map[string]storagemodels.User{
		"admin": {PK: "USER#admin", SK: storagemodels.SKMetadata, Username: "admin", Role: "admin", Approved: true, Version: 1, CreatedAt: now, UpdatedAt: now},
	}}
	cfg := round11TestConfig()
	h, _, _ := round11NewHandler(t, cfg, state)
	h.skillsService = &skillHandlerStub{}
	adminWriteHeaders := map[string]string{"Authorization": "Bearer " + round11SignAccessToken(t, cfg.JWTSecret, "admin", []string{"admin:write"})}

	ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v1/admin/skills/skill-a/revisions/1/approve", adminWriteHeaders, nil, []byte("{"))
	ctx.Params["skillId"] = "skill-a"
	ctx.Params["revisionNumber"] = "1"
	resp := requireStatus(t, http.StatusBadRequest)(h.HandleAdminApproveSkillRevisionLift(ctx))
	require.Contains(t, string(resp.Body), "invalid request body")

	ctxWithReq, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/skills/skill-a/revisions/bad/revoke", adminWriteHeaders, nil, apimodels.RevokeSkillRevisionRequest{Reason: "rotated"})
	require.NoError(t, err)
	ctxWithReq.Params["skillId"] = "skill-a"
	ctxWithReq.Params["revisionNumber"] = "bad"
	resp = requireStatus(t, http.StatusBadRequest)(h.HandleAdminRevokeSkillRevisionLift(ctxWithReq))
	require.Contains(t, string(resp.Body), "revision_number")

	ctx = round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v1/admin/skills/skill-a/assignments", adminWriteHeaders, nil, []byte("{"))
	ctx.Params["skillId"] = "skill-a"
	resp = requireStatus(t, http.StatusBadRequest)(h.HandleAdminCreateSkillAssignmentLift(ctx))
	require.Contains(t, string(resp.Body), "invalid request body")

	ctx = round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v1/admin/skills/skill-a/assignments/assign-1/revoke", adminWriteHeaders, nil, []byte("{"))
	ctx.Params["skillId"] = "skill-a"
	ctx.Params["assignmentId"] = "assign-1"
	resp = requireStatus(t, http.StatusBadRequest)(h.HandleAdminRevokeSkillAssignmentLift(ctx))
	require.Contains(t, string(resp.Body), "invalid request body")
}

func TestSkillHandlers_AdminAuthFailuresAndNilService(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 5, 13, 12, 0, 0, 0, time.UTC)
	state := &round10QueryState{usersByUsername: map[string]storagemodels.User{
		"admin": {PK: "USER#admin", SK: storagemodels.SKMetadata, Username: "admin", Role: "admin", Approved: true, Version: 1, CreatedAt: now, UpdatedAt: now},
	}}
	cfg := round11TestConfig()
	h, _, _ := round11NewHandler(t, cfg, state)
	h.skillsService = &skillHandlerStub{}
	readWriteHeaders := map[string]string{"Authorization": "Bearer " + round10SignAccessToken(t, cfg.JWTSecret, "admin")}
	ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/admin/skills/proposals", readWriteHeaders, nil, nil)
	require.NoError(t, err)
	resp := requireStatus(t, http.StatusForbidden)(h.HandleAdminListSkillProposalsLift(ctx))
	require.Contains(t, string(resp.Body), "insufficient")

	adminHeaders := map[string]string{"Authorization": "Bearer " + round11SignAccessToken(t, cfg.JWTSecret, "admin", []string{auth.ScopeAdmin})}
	ctx, err = round10NewLiftContext(http.MethodGet, "/api/v1/admin/skills/proposals", adminHeaders, nil, nil)
	require.NoError(t, err)
	resp = requireStatus(t, http.StatusOK)(h.HandleAdminListSkillProposalsLift(ctx))
	require.Contains(t, string(resp.Body), "proposals")

	state.usersByUsername["operator"] = storagemodels.User{PK: "USER#operator", SK: storagemodels.SKMetadata, Username: "operator", Role: "user", Approved: true, Version: 1, CreatedAt: now, UpdatedAt: now}
	operatorHeaders := map[string]string{"Authorization": "Bearer " + round11SignAccessToken(t, cfg.JWTSecret, "operator", []string{auth.ScopeAdmin})}
	ctx, err = round10NewLiftContext(http.MethodGet, "/api/v1/admin/skills/proposals", operatorHeaders, nil, nil)
	require.NoError(t, err)
	resp = requireStatus(t, http.StatusForbidden)(h.HandleAdminListSkillProposalsLift(ctx))
	require.Contains(t, string(resp.Body), "admin access")

	nilSvc := nilSkillService{}
	_, _, err = nilSvc.ListSkills(context.Background(), skillservice.Viewer{}, skillservice.ListFilter{})
	require.ErrorIs(t, err, skillservice.ErrRepositoryUnavailable)
	_, err = nilSvc.GetSkill(context.Background(), skillservice.Viewer{}, "skill-a")
	require.ErrorIs(t, err, skillservice.ErrRepositoryUnavailable)
	_, _, err = nilSvc.ListRevisions(context.Background(), skillservice.Viewer{}, "skill-a", 10, "")
	require.ErrorIs(t, err, skillservice.ErrRepositoryUnavailable)
	_, err = nilSvc.GetRevision(context.Background(), skillservice.Viewer{}, "skill-a", 1)
	require.ErrorIs(t, err, skillservice.ErrRepositoryUnavailable)
	_, _, err = nilSvc.ListProposals(context.Background(), "", "", 10, "")
	require.ErrorIs(t, err, skillservice.ErrRepositoryUnavailable)
	_, err = nilSvc.GetProposal(context.Background(), "proposal-1")
	require.ErrorIs(t, err, skillservice.ErrRepositoryUnavailable)
	_, _, err = nilSvc.ListAssignmentsForSkill(context.Background(), "skill-a", 10, "")
	require.ErrorIs(t, err, skillservice.ErrRepositoryUnavailable)
	_, err = nilSvc.ApproveRevision(context.Background(), "skill-a", 1, skillservice.ApprovalCommand{})
	require.ErrorIs(t, err, skillservice.ErrRepositoryUnavailable)
	_, err = nilSvc.RevokeRevision(context.Background(), "skill-a", 1, skillservice.RevocationCommand{})
	require.ErrorIs(t, err, skillservice.ErrRepositoryUnavailable)
	_, err = nilSvc.AssignSkill(context.Background(), skillservice.AssignmentCommand{})
	require.ErrorIs(t, err, skillservice.ErrRepositoryUnavailable)
	_, err = nilSvc.RevokeAssignment(context.Background(), skillservice.AssignmentRevocationCommand{})
	require.ErrorIs(t, err, skillservice.ErrRepositoryUnavailable)
	_, err = nilSvc.ResolveEffectiveSkills(context.Background(), skillservice.Viewer{}, skillservice.ResolveCommand{})
	require.ErrorIs(t, err, skillservice.ErrRepositoryUnavailable)
}

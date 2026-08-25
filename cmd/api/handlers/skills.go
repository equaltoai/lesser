package handlers

import (
	"context"
	"errors"
	"strconv"
	"strings"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	skillservice "github.com/equaltoai/lesser/pkg/services/skills"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	apptheory "github.com/theory-cloud/apptheory/v4/runtime"
)

type skillHandlerService interface {
	ListSkills(context.Context, skillservice.Viewer, skillservice.ListFilter) ([]*storagemodels.Skill, string, error)
	GetSkill(context.Context, skillservice.Viewer, string) (*storagemodels.Skill, error)
	ListRevisions(context.Context, skillservice.Viewer, string, int, string) ([]*storagemodels.SkillRevision, string, error)
	GetRevision(context.Context, skillservice.Viewer, string, int) (*storagemodels.SkillRevision, error)
	ListCatalog(context.Context, skillservice.Viewer, skillservice.CatalogFilter) ([]skillservice.CatalogEntry, string, error)
	GetBundle(context.Context, skillservice.Viewer, string, int, bool) (*skillservice.CatalogEntry, error)
	ListProposals(context.Context, string, string, int, string) ([]*storagemodels.SkillProposal, string, error)
	GetProposal(context.Context, string) (*storagemodels.SkillProposal, error)
	ListAssignmentsForSkill(context.Context, string, int, string) ([]*storagemodels.SkillAssignment, string, error)
	ApproveRevision(context.Context, string, int, skillservice.ApprovalCommand) (*storagemodels.SkillRevision, error)
	RevokeRevision(context.Context, string, int, skillservice.RevocationCommand) (*storagemodels.SkillRevision, error)
	PromoteProposal(context.Context, string, skillservice.PromotionCommand) (*skillservice.PromotionResult, error)
	AssignSkill(context.Context, skillservice.AssignmentCommand) (*storagemodels.SkillAssignment, error)
	RevokeAssignment(context.Context, skillservice.AssignmentRevocationCommand) (*storagemodels.SkillAssignment, error)
	ResolveEffectiveSkills(context.Context, skillservice.Viewer, skillservice.ResolveCommand) (*skillservice.ResolveResult, error)
}

// HandleListSkillsLift handles GET /api/v1/skills.
func (h *Handler) HandleListSkillsLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	claims := h.optionalAuthenticatedClaimsLift(ctx)
	viewer := h.skillViewer(ctx.Context(), claims)
	items, cursor, err := h.getSkillService().ListSkills(ctx.Context(), viewer, skillservice.ListFilter{
		Status:   queryValue(ctx, "status"),
		Exposure: queryValue(ctx, "exposure"),
		Limit:    parseSkillLimit(queryValue(ctx, "limit")),
		Cursor:   queryValue(ctx, "cursor"),
	})
	if err != nil {
		return h.respondSkillServiceError(ctx, err)
	}
	out := make([]apimodels.SkillResource, 0, len(items))
	for _, item := range items {
		out = append(out, toAPISkill(item))
	}
	return okJSON(apimodels.SkillListResponse{Skills: out, Count: len(out), NextCursor: cursor})
}

// HandleGetSkillLift handles GET /api/v1/skills/{skillId}.
func (h *Handler) HandleGetSkillLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	claims := h.optionalAuthenticatedClaimsLift(ctx)
	skill, err := h.getSkillService().GetSkill(ctx.Context(), h.skillViewer(ctx.Context(), claims), ctx.Param("skillId"))
	if err != nil {
		return h.respondSkillServiceError(ctx, err)
	}
	return okJSON(apimodels.SkillResponse{Skill: toAPISkill(skill)})
}

// HandleListSkillRevisionsLift handles GET /api/v1/skills/{skillId}/revisions.
func (h *Handler) HandleListSkillRevisionsLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	claims := h.optionalAuthenticatedClaimsLift(ctx)
	items, cursor, err := h.getSkillService().ListRevisions(
		ctx.Context(),
		h.skillViewer(ctx.Context(), claims),
		ctx.Param("skillId"),
		parseSkillLimit(queryValue(ctx, "limit")),
		queryValue(ctx, "cursor"),
	)
	if err != nil {
		return h.respondSkillServiceError(ctx, err)
	}
	out := make([]apimodels.SkillRevisionResource, 0, len(items))
	for _, item := range items {
		out = append(out, toAPISkillRevision(item))
	}
	return okJSON(apimodels.SkillRevisionsResponse{Revisions: out, Count: len(out), NextCursor: cursor})
}

// HandleGetSkillRevisionLift handles GET /api/v1/skills/{skillId}/revisions/{revisionNumber}.
func (h *Handler) HandleGetSkillRevisionLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	revisionNumber, err := parseSkillRevisionNumber(ctx.Param("revisionNumber"))
	if err != nil {
		return common.RespondBadRequest(ctx, "revision_number is invalid")
	}
	claims := h.optionalAuthenticatedClaimsLift(ctx)
	revision, err := h.getSkillService().GetRevision(ctx.Context(), h.skillViewer(ctx.Context(), claims), ctx.Param("skillId"), revisionNumber)
	if err != nil {
		return h.respondSkillServiceError(ctx, err)
	}
	return okJSON(apimodels.SkillRevisionResponse{Revision: toAPISkillRevision(revision)})
}

// HandleListSkillCatalogLift handles GET /api/v1/skills/catalog.
func (h *Handler) HandleListSkillCatalogLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	claims := h.optionalAuthenticatedClaimsLift(ctx)
	items, cursor, err := h.getSkillService().ListCatalog(ctx.Context(), h.skillViewer(ctx.Context(), claims), skillservice.CatalogFilter{
		Exposure: queryValue(ctx, "exposure"),
		Limit:    parseSkillLimit(queryValue(ctx, "limit")),
		Cursor:   queryValue(ctx, "cursor"),
	})
	if err != nil {
		return h.respondSkillServiceError(ctx, err)
	}
	out := make([]apimodels.SkillCatalogEntryResource, 0, len(items))
	for _, item := range items {
		out = append(out, toAPISkillCatalogEntry(item))
	}
	return okJSON(apimodels.SkillCatalogResponse{Entries: out, Count: len(out), NextCursor: cursor})
}

// HandleGetSkillBundleLift handles GET /api/v1/skills/{skillId}/revisions/{revisionNumber}/bundle.
func (h *Handler) HandleGetSkillBundleLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	revisionNumber, err := parseSkillRevisionNumber(ctx.Param("revisionNumber"))
	if err != nil {
		return common.RespondBadRequest(ctx, "revision_number is invalid")
	}
	claims := h.optionalAuthenticatedClaimsLift(ctx)
	entry, err := h.getSkillService().GetBundle(
		ctx.Context(),
		h.skillViewer(ctx.Context(), claims),
		ctx.Param("skillId"),
		revisionNumber,
		parseSkillBool(queryValue(ctx, "include_content")),
	)
	if err != nil {
		return h.respondSkillServiceError(ctx, err)
	}
	return okJSON(apimodels.SkillBundleResponse{Bundle: toAPISkillBundle(entry)})
}

// HandleResolveEffectiveSkillsLift handles GET /api/v1/skills/resolve.
func (h *Handler) HandleResolveEffectiveSkillsLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	claims, err := h.authenticateWithScope(ctx, auth.ScopeRead)
	if err != nil {
		return h.respondSkillAuthError(ctx, err)
	}
	result, err := h.getSkillService().ResolveEffectiveSkills(ctx.Context(), h.skillViewer(ctx.Context(), claims), skillservice.ResolveCommand{
		SubjectType: queryValue(ctx, "subject_type"),
		SubjectID:   queryValue(ctx, "subject_id"),
		Limit:       parseSkillLimit(queryValue(ctx, "limit")),
		Cursor:      queryValue(ctx, "cursor"),
	})
	if err != nil {
		return h.respondSkillServiceError(ctx, err)
	}
	out := make([]apimodels.EffectiveSkillResource, 0, len(result.Items))
	for _, item := range result.Items {
		out = append(out, apimodels.EffectiveSkillResource{
			Skill:      toAPISkill(item.Skill),
			Revision:   toAPISkillRevision(item.Revision),
			Assignment: toAPISkillAssignment(item.Assignment),
		})
	}
	return okJSON(apimodels.EffectiveSkillsResponse{
		SubjectType: result.SubjectType,
		SubjectID:   result.SubjectID,
		Skills:      out,
		Count:       len(out),
		NextCursor:  result.NextCursor,
	})
}

// HandleAdminListSkillProposalsLift handles GET /api/v1/admin/skills/proposals.
func (h *Handler) HandleAdminListSkillProposalsLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	if _, resp, err := h.requireSkillAdmin(ctx, "admin:read"); resp != nil || err != nil {
		return resp, err
	}
	items, cursor, err := h.getSkillService().ListProposals(
		ctx.Context(),
		queryValue(ctx, "skill_id"),
		queryValue(ctx, "status"),
		parseSkillLimit(queryValue(ctx, "limit")),
		queryValue(ctx, "cursor"),
	)
	if err != nil {
		return h.respondSkillServiceError(ctx, err)
	}
	out := make([]apimodels.SkillProposalResource, 0, len(items))
	for _, item := range items {
		out = append(out, toAPISkillProposal(item))
	}
	return okJSON(apimodels.SkillProposalsResponse{Proposals: out, Count: len(out), NextCursor: cursor})
}

// HandleAdminGetSkillProposalLift handles GET /api/v1/admin/skills/proposals/{proposalId}.
func (h *Handler) HandleAdminGetSkillProposalLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	if _, resp, err := h.requireSkillAdmin(ctx, "admin:read"); resp != nil || err != nil {
		return resp, err
	}
	proposal, err := h.getSkillService().GetProposal(ctx.Context(), ctx.Param("proposalId"))
	if err != nil {
		return h.respondSkillServiceError(ctx, err)
	}
	return okJSON(apimodels.SkillProposalResponse{Proposal: toAPISkillProposal(proposal)})
}

// HandleAdminListSkillAssignmentsLift handles GET /api/v1/admin/skills/{skillId}/assignments.
func (h *Handler) HandleAdminListSkillAssignmentsLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	if _, resp, err := h.requireSkillAdmin(ctx, "admin:read"); resp != nil || err != nil {
		return resp, err
	}
	items, cursor, err := h.getSkillService().ListAssignmentsForSkill(
		ctx.Context(),
		ctx.Param("skillId"),
		parseSkillLimit(queryValue(ctx, "limit")),
		queryValue(ctx, "cursor"),
	)
	if err != nil {
		return h.respondSkillServiceError(ctx, err)
	}
	out := make([]apimodels.SkillAssignmentResource, 0, len(items))
	for _, item := range items {
		out = append(out, toAPISkillAssignment(item))
	}
	return okJSON(apimodels.SkillAssignmentsResponse{Assignments: out, Count: len(out), NextCursor: cursor})
}

// HandleAdminApproveSkillRevisionLift handles POST /api/v1/admin/skills/{skillId}/revisions/{revisionNumber}/approve.
func (h *Handler) HandleAdminApproveSkillRevisionLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	claims, resp, err := h.requireSkillAdmin(ctx, "admin:write")
	if resp != nil || err != nil {
		return resp, err
	}
	revisionNumber, err := parseSkillRevisionNumber(ctx.Param("revisionNumber"))
	if err != nil {
		return common.RespondBadRequest(ctx, "revision_number is invalid")
	}
	var req apimodels.ApproveSkillRevisionRequest
	if err := h.parseRequestBody(ctx, &req); err != nil {
		return common.RespondBadRequest(ctx, "invalid request body")
	}
	revision, err := h.getSkillService().ApproveRevision(ctx.Context(), ctx.Param("skillId"), revisionNumber, skillservice.ApprovalCommand{
		ActorUsername:         claims.Username,
		ApprovalID:            req.ApprovalID,
		PrincipalID:           req.PrincipalID,
		PrincipalApprovalID:   req.PrincipalApprovalID,
		ApprovalAuthorityType: req.ApprovalAuthorityType,
		ApprovalAuthorityID:   req.ApprovalAuthorityID,
		ApprovalDigest:        req.ApprovalDigest,
		ApprovalSignature:     req.ApprovalSignature,
		ApprovalRef:           req.ApprovalRef,
		ApprovalReason:        req.ApprovalReason,
	})
	if err != nil {
		return h.respondSkillServiceError(ctx, err)
	}
	return okJSON(apimodels.SkillRevisionResponse{Revision: toAPISkillRevision(revision)})
}

// HandleAdminRevokeSkillRevisionLift handles POST /api/v1/admin/skills/{skillId}/revisions/{revisionNumber}/revoke.
func (h *Handler) HandleAdminRevokeSkillRevisionLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	claims, resp, err := h.requireSkillAdmin(ctx, "admin:write")
	if resp != nil || err != nil {
		return resp, err
	}
	revisionNumber, err := parseSkillRevisionNumber(ctx.Param("revisionNumber"))
	if err != nil {
		return common.RespondBadRequest(ctx, "revision_number is invalid")
	}
	var req apimodels.RevokeSkillRevisionRequest
	if err := h.parseRequestBody(ctx, &req); err != nil {
		return common.RespondBadRequest(ctx, "invalid request body")
	}
	revision, err := h.getSkillService().RevokeRevision(ctx.Context(), ctx.Param("skillId"), revisionNumber, skillservice.RevocationCommand{
		ActorUsername: claims.Username,
		Reason:        req.Reason,
	})
	if err != nil {
		return h.respondSkillServiceError(ctx, err)
	}
	return okJSON(apimodels.SkillRevisionResponse{Revision: toAPISkillRevision(revision)})
}

// HandleAdminPromoteSkillProposalLift handles POST /api/v1/admin/skills/{skillId}/proposals/{proposalId}/promote.
func (h *Handler) HandleAdminPromoteSkillProposalLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	claims, resp, err := h.requireSkillAdmin(ctx, "admin:write")
	if resp != nil || err != nil {
		return resp, err
	}
	var req apimodels.PromoteSkillProposalRequest
	if err := h.parseRequestBody(ctx, &req); err != nil {
		return common.RespondBadRequest(ctx, "invalid request body")
	}
	result, err := h.getSkillService().PromoteProposal(ctx.Context(), ctx.Param("skillId"), skillservice.PromotionCommand{
		ActorUsername:          claims.Username,
		ProposalID:             ctx.Param("proposalId"),
		ExpectedManifestDigest: req.ExpectedManifestDigest,
		ExpectedSourceDigest:   req.ExpectedSourceDigest,
		ApprovalID:             req.ApprovalID,
		PrincipalID:            req.PrincipalID,
		PrincipalApprovalID:    req.PrincipalApprovalID,
		ApprovalAuthorityType:  req.ApprovalAuthorityType,
		ApprovalAuthorityID:    req.ApprovalAuthorityID,
		ApprovalDigest:         req.ApprovalDigest,
		ApprovalSignature:      req.ApprovalSignature,
		ApprovalRef:            req.ApprovalRef,
		ApprovalReason:         req.ApprovalReason,
	})
	if err != nil {
		return h.respondSkillServiceError(ctx, err)
	}
	out := apimodels.PromoteSkillProposalResponse{
		Revision: toAPISkillRevision(result.Revision),
		Proposal: toAPISkillProposal(result.Proposal),
		Created:  result.Created,
	}
	return createdJSON(out)
}

// HandleAdminCreateSkillAssignmentLift handles POST /api/v1/admin/skills/{skillId}/assignments.
func (h *Handler) HandleAdminCreateSkillAssignmentLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	claims, resp, err := h.requireSkillAdmin(ctx, "admin:write")
	if resp != nil || err != nil {
		return resp, err
	}
	var req apimodels.CreateSkillAssignmentRequest
	if err := h.parseRequestBody(ctx, &req); err != nil {
		return common.RespondBadRequest(ctx, "invalid request body")
	}
	assignment, err := h.getSkillService().AssignSkill(ctx.Context(), skillservice.AssignmentCommand{
		ActorUsername:       claims.Username,
		AssignmentID:        req.AssignmentID,
		SkillID:             ctx.Param("skillId"),
		RevisionNumber:      req.RevisionNumber,
		SubjectType:         req.SubjectType,
		SubjectID:           req.SubjectID,
		Exposure:            req.Exposure,
		ApprovalID:          req.ApprovalID,
		PrincipalID:         req.PrincipalID,
		PrincipalApprovalID: req.PrincipalApprovalID,
	})
	if err != nil {
		return h.respondSkillServiceError(ctx, err)
	}
	return createdJSON(apimodels.SkillAssignmentResponse{Assignment: toAPISkillAssignment(assignment)})
}

// HandleAdminRevokeSkillAssignmentLift handles POST /api/v1/admin/skills/{skillId}/assignments/{assignmentId}/revoke.
func (h *Handler) HandleAdminRevokeSkillAssignmentLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	claims, resp, err := h.requireSkillAdmin(ctx, "admin:write")
	if resp != nil || err != nil {
		return resp, err
	}
	var req apimodels.RevokeSkillAssignmentRequest
	if err := h.parseRequestBody(ctx, &req); err != nil {
		return common.RespondBadRequest(ctx, "invalid request body")
	}
	assignment, err := h.getSkillService().RevokeAssignment(ctx.Context(), skillservice.AssignmentRevocationCommand{
		SkillID:      ctx.Param("skillId"),
		AssignmentID: ctx.Param("assignmentId"),
		SubjectType:  req.SubjectType,
		SubjectID:    req.SubjectID,
		RevocationCommand: skillservice.RevocationCommand{
			ActorUsername: claims.Username,
			Reason:        req.Reason,
		},
	})
	if err != nil {
		return h.respondSkillServiceError(ctx, err)
	}
	return okJSON(apimodels.SkillAssignmentResponse{Assignment: toAPISkillAssignment(assignment)})
}

func (h *Handler) getSkillService() skillHandlerService {
	if h == nil {
		return nilSkillService{}
	}
	if h.skillsService != nil {
		return h.skillsService
	}
	if h.repos == nil {
		return nilSkillService{}
	}
	return skillservice.NewService(h.repos.Skill())
}

func (h *Handler) skillViewer(ctx context.Context, claims *auth.Claims) skillservice.Viewer {
	viewer := skillservice.Viewer{}
	if claims == nil {
		return viewer
	}
	viewer.Username = claims.Username
	viewer.Authenticated = strings.TrimSpace(claims.Username) != ""
	admin, err := h.claimsUserHasAdminRole(ctx, claims)
	viewer.Admin = err == nil && admin
	return viewer
}

func (h *Handler) requireSkillAdmin(ctx *apptheory.Context, operationScope string) (*auth.Claims, *apptheory.Response, error) {
	claims, err := h.authenticateWithAnyScope(ctx, auth.ScopeAdmin, operationScope)
	if err != nil {
		resp, respErr := h.respondSkillAuthError(ctx, err)
		return nil, resp, respErr
	}
	user, err := h.repos.Account().GetUser(ctx.Context(), claims.Username)
	if err != nil || user.Role != roleAdmin {
		resp, respErr := common.RespondForbidden(ctx, common.ErrorAdminAccessRequired)
		return nil, resp, respErr
	}
	return claims, nil, nil
}

func (h *Handler) respondSkillAuthError(ctx *apptheory.Context, err error) (*apptheory.Response, error) {
	if isInsufficientScopeError(err) {
		return common.RespondForbidden(ctx, err.Error())
	}
	return common.RespondUnauthorized(ctx)
}

func (h *Handler) respondSkillServiceError(ctx *apptheory.Context, err error) (*apptheory.Response, error) {
	switch {
	case errors.Is(err, skillservice.ErrSkillNotFound),
		errors.Is(err, skillservice.ErrSkillRevisionNotFound),
		errors.Is(err, skillservice.ErrSkillProposalNotFound),
		errors.Is(err, skillservice.ErrSkillAssignmentNotFound):
		return common.RespondNotFound(ctx, "skill")
	case errors.Is(err, skillservice.ErrForbidden):
		return common.RespondForbidden(ctx, "not authorized to access skill")
	case errors.Is(err, skillservice.ErrInvalidInput):
		return common.RespondBadRequest(ctx, "invalid skill request")
	case errors.Is(err, skillservice.ErrInvalidState):
		return common.RespondUnprocessableEntity(ctx, "invalid skill state")
	case errors.Is(err, skillservice.ErrApprovalDigestMismatch):
		return common.RespondUnprocessableEntity(ctx, "approval digest mismatch")
	case errors.Is(err, skillservice.ErrPromotionDigestMismatch):
		return common.RespondUnprocessableEntity(ctx, "promotion digest mismatch")
	case errors.Is(err, skillservice.ErrPromotionConflict):
		return common.RespondConflict(ctx, "skill promotion conflict")
	case errors.Is(err, skillservice.ErrExposureViolation):
		return common.RespondForbidden(ctx, "skill exposure violation")
	default:
		return common.RespondInternalServerError(ctx)
	}
}

func parseSkillLimit(value string) int {
	limit, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || limit <= 0 {
		return 25
	}
	if limit > 100 {
		return 100
	}
	return limit
}

func parseSkillBool(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", boolTrue, "yes", "y", "on":
		return true
	default:
		return false
	}
}

func parseSkillRevisionNumber(value string) (int, error) {
	n, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || n <= 0 {
		return 0, errors.New("invalid revision number")
	}
	return n, nil
}

func toAPISkill(skill *storagemodels.Skill) apimodels.SkillResource {
	if skill == nil {
		return apimodels.SkillResource{}
	}
	return apimodels.SkillResource{
		ID:                    skill.ID,
		Slug:                  skill.Slug,
		Name:                  skill.Name,
		Description:           skill.Description,
		Status:                skill.Status,
		DefaultExposure:       skill.DefaultExposure,
		CurrentRevisionID:     skill.CurrentRevisionID,
		CurrentRevisionNumber: skill.CurrentRevisionNumber,
		Capabilities:          append([]string(nil), skill.Capabilities...),
		Tags:                  append([]string(nil), skill.Tags...),
		Provenance:            toAPISkillProvenance(skill.Provenance),
		CreatedBy:             skill.CreatedBy,
		UpdatedBy:             skill.UpdatedBy,
		CreatedAt:             skill.CreatedAt,
		UpdatedAt:             skill.UpdatedAt,
		Version:               skill.Version,
	}
}

func toAPISkillRevision(revision *storagemodels.SkillRevision) apimodels.SkillRevisionResource {
	if revision == nil {
		return apimodels.SkillRevisionResource{}
	}
	return apimodels.SkillRevisionResource{
		ID:                    revision.ID,
		SkillID:               revision.SkillID,
		RevisionNumber:        revision.RevisionNumber,
		Status:                revision.Status,
		ProposalID:            revision.ProposalID,
		ManifestDigest:        revision.ManifestDigest,
		BundleDigest:          revision.BundleDigest,
		ContentDigest:         revision.ContentDigest,
		Files:                 toAPISkillRevisionFiles(revision.Files),
		Capabilities:          append([]string(nil), revision.Capabilities...),
		DefaultExposure:       revision.DefaultExposure,
		ApprovalID:            revision.ApprovalID,
		ApprovalAuthorityType: revision.ApprovalAuthorityType,
		ApprovalAuthorityID:   revision.ApprovalAuthorityID,
		ApprovalDigest:        revision.ApprovalDigest,
		ApprovalSignature:     revision.ApprovalSignature,
		ApprovalRef:           revision.ApprovalRef,
		ApprovalReason:        revision.ApprovalReason,
		ApprovedBy:            revision.ApprovedBy,
		ApprovedAt:            revision.ApprovedAt,
		PrincipalID:           revision.PrincipalID,
		PrincipalApprovalID:   revision.PrincipalApprovalID,
		Provenance:            toAPISkillProvenance(revision.Provenance),
		RevokedBy:             revision.RevokedBy,
		RevokedAt:             revision.RevokedAt,
		RevokedReason:         revision.RevokedReason,
		CreatedBy:             revision.CreatedBy,
		UpdatedBy:             revision.UpdatedBy,
		CreatedAt:             revision.CreatedAt,
		UpdatedAt:             revision.UpdatedAt,
		Version:               revision.Version,
	}
}

func toAPISkillProposal(proposal *storagemodels.SkillProposal) apimodels.SkillProposalResource {
	if proposal == nil {
		return apimodels.SkillProposalResource{}
	}
	return apimodels.SkillProposalResource{
		ID:                     proposal.ID,
		SkillID:                proposal.SkillID,
		Title:                  proposal.Title,
		Summary:                proposal.Summary,
		Status:                 proposal.Status,
		RequestedExposure:      proposal.RequestedExposure,
		ProposedRevisionNumber: proposal.ProposedRevisionNumber,
		ProposedManifestDigest: proposal.ProposedManifestDigest,
		SourceType:             proposal.SourceType,
		SourceURI:              proposal.SourceURI,
		SourceDigest:           proposal.SourceDigest,
		ConversationID:         proposal.ConversationID,
		ConversationMessageID:  proposal.ConversationMessageID,
		PrincipalID:            proposal.PrincipalID,
		PrincipalApprovalID:    proposal.PrincipalApprovalID,
		PromotedRevisionID:     proposal.PromotedRevisionID,
		PromotedRevisionNumber: proposal.PromotedRevisionNumber,
		PromotionDigest:        proposal.PromotionDigest,
		PromotedBy:             proposal.PromotedBy,
		PromotedAt:             proposal.PromotedAt,
		Provenance:             toAPISkillProvenance(proposal.Provenance),
		CreatedBy:              proposal.CreatedBy,
		ReviewedBy:             proposal.ReviewedBy,
		ReviewedAt:             proposal.ReviewedAt,
		ReviewReason:           proposal.ReviewReason,
		CreatedAt:              proposal.CreatedAt,
		UpdatedAt:              proposal.UpdatedAt,
		Version:                proposal.Version,
	}
}

func toAPISkillAssignment(assignment *storagemodels.SkillAssignment) apimodels.SkillAssignmentResource {
	if assignment == nil {
		return apimodels.SkillAssignmentResource{}
	}
	return apimodels.SkillAssignmentResource{
		ID:                  assignment.ID,
		SkillID:             assignment.SkillID,
		RevisionID:          assignment.RevisionID,
		RevisionNumber:      assignment.RevisionNumber,
		SubjectType:         assignment.SubjectType,
		SubjectID:           assignment.SubjectID,
		Exposure:            assignment.Exposure,
		Status:              assignment.Status,
		ApprovalID:          assignment.ApprovalID,
		PrincipalID:         assignment.PrincipalID,
		PrincipalApprovalID: assignment.PrincipalApprovalID,
		Provenance:          toAPISkillProvenance(assignment.Provenance),
		AssignedBy:          assignment.AssignedBy,
		AssignedAt:          assignment.AssignedAt,
		RevokedBy:           assignment.RevokedBy,
		RevokedAt:           assignment.RevokedAt,
		RevokedReason:       assignment.RevokedReason,
		CreatedAt:           assignment.CreatedAt,
		UpdatedAt:           assignment.UpdatedAt,
		Version:             assignment.Version,
	}
}

func toAPISkillCatalogEntry(entry skillservice.CatalogEntry) apimodels.SkillCatalogEntryResource {
	return apimodels.SkillCatalogEntryResource{
		Skill:    toAPISkill(entry.Skill),
		Revision: toAPISkillRevision(entry.Revision),
		Bundle:   toAPISkillBundle(&entry),
	}
}

func toAPISkillBundle(entry *skillservice.CatalogEntry) apimodels.SkillBundleResource {
	if entry == nil || entry.Revision == nil {
		return apimodels.SkillBundleResource{}
	}
	revision := entry.Revision
	bundle := entry.Bundle
	return apimodels.SkillBundleResource{
		SchemaVersion:   bundle.SchemaVersion,
		BundleID:        bundle.BundleID,
		Source:          "canonical_skill_revision",
		Published:       revision.Status == storagemodels.SkillRevisionStatusApproved,
		SkillID:         revision.SkillID,
		RevisionID:      revision.ID,
		RevisionNumber:  revision.RevisionNumber,
		DefaultExposure: revision.DefaultExposure,
		Digests: apimodels.SkillBundleDigestsResource{
			ManifestDigest:    bundle.ManifestDigest,
			BundleDigest:      bundle.BundleDigest,
			PublicationDigest: bundle.PublicationDigest,
			ContentDigest:     bundle.ContentDigest,
			ApprovalDigest:    bundle.ApprovalDigest,
		},
		Files:                 toAPISkillBundleFiles(bundle.Files),
		InstallHints:          toAPISkillInstallHints(bundle.InstallHints),
		Provenance:            toAPISkillProvenance(bundle.Provenance),
		ApprovalID:            revision.ApprovalID,
		ApprovalAuthorityType: revision.ApprovalAuthorityType,
		ApprovalAuthorityID:   revision.ApprovalAuthorityID,
		ApprovalSignature:     revision.ApprovalSignature,
		ApprovalRef:           revision.ApprovalRef,
		ApprovedBy:            revision.ApprovedBy,
		ApprovedAt:            revision.ApprovedAt,
		PrincipalID:           revision.PrincipalID,
		PrincipalApprovalID:   revision.PrincipalApprovalID,
	}
}

func toAPISkillBundleFiles(values []skillservice.SkillBundleFile) []apimodels.SkillBundleFileResource {
	if len(values) == 0 {
		return nil
	}
	out := make([]apimodels.SkillBundleFileResource, 0, len(values))
	for _, value := range values {
		out = append(out, apimodels.SkillBundleFileResource{
			Path:            value.Path,
			Digest:          value.Digest,
			ContentType:     value.ContentType,
			Role:            value.Role,
			SizeBytes:       value.SizeBytes,
			InstallPath:     value.InstallPath,
			Content:         value.Content,
			Encoding:        value.Encoding,
			ContentIncluded: value.ContentIncluded,
		})
	}
	return out
}

func toAPISkillInstallHints(value skillservice.SkillInstallHints) apimodels.SkillInstallHintsResource {
	return apimodels.SkillInstallHintsResource{
		Layout:         value.Layout,
		RuntimeTargets: append([]string(nil), value.RuntimeTargets...),
		DirectoryName:  value.DirectoryName,
		EntryPoint:     value.EntryPoint,
		RequiredFiles:  append([]string(nil), value.RequiredFiles...),
	}
}

func toAPISkillProvenance(values []storagemodels.SkillProvenanceRef) []apimodels.SkillProvenanceRef {
	if len(values) == 0 {
		return nil
	}
	out := make([]apimodels.SkillProvenanceRef, 0, len(values))
	for _, value := range values {
		out = append(out, apimodels.SkillProvenanceRef{
			SourceType: value.SourceType,
			SourceURI:  value.SourceURI,
			Digest:     value.Digest,
			Ref:        value.Ref,
			Notes:      value.Notes,
		})
	}
	return out
}

func toAPISkillRevisionFiles(values []storagemodels.SkillRevisionFile) []apimodels.SkillRevisionFile {
	if len(values) == 0 {
		return nil
	}
	out := make([]apimodels.SkillRevisionFile, 0, len(values))
	for _, value := range values {
		out = append(out, apimodels.SkillRevisionFile{
			Path:        value.Path,
			Digest:      value.Digest,
			ContentType: value.ContentType,
			Role:        value.Role,
			SizeBytes:   value.SizeBytes,
		})
	}
	return out
}

type nilSkillService struct{}

func (nilSkillService) ListSkills(context.Context, skillservice.Viewer, skillservice.ListFilter) ([]*storagemodels.Skill, string, error) {
	return nil, "", skillservice.ErrRepositoryUnavailable
}
func (nilSkillService) GetSkill(context.Context, skillservice.Viewer, string) (*storagemodels.Skill, error) {
	return nil, skillservice.ErrRepositoryUnavailable
}
func (nilSkillService) ListRevisions(context.Context, skillservice.Viewer, string, int, string) ([]*storagemodels.SkillRevision, string, error) {
	return nil, "", skillservice.ErrRepositoryUnavailable
}
func (nilSkillService) GetRevision(context.Context, skillservice.Viewer, string, int) (*storagemodels.SkillRevision, error) {
	return nil, skillservice.ErrRepositoryUnavailable
}
func (nilSkillService) ListCatalog(context.Context, skillservice.Viewer, skillservice.CatalogFilter) ([]skillservice.CatalogEntry, string, error) {
	return nil, "", skillservice.ErrRepositoryUnavailable
}
func (nilSkillService) GetBundle(context.Context, skillservice.Viewer, string, int, bool) (*skillservice.CatalogEntry, error) {
	return nil, skillservice.ErrRepositoryUnavailable
}
func (nilSkillService) ListProposals(context.Context, string, string, int, string) ([]*storagemodels.SkillProposal, string, error) {
	return nil, "", skillservice.ErrRepositoryUnavailable
}
func (nilSkillService) GetProposal(context.Context, string) (*storagemodels.SkillProposal, error) {
	return nil, skillservice.ErrRepositoryUnavailable
}
func (nilSkillService) ListAssignmentsForSkill(context.Context, string, int, string) ([]*storagemodels.SkillAssignment, string, error) {
	return nil, "", skillservice.ErrRepositoryUnavailable
}
func (nilSkillService) ApproveRevision(context.Context, string, int, skillservice.ApprovalCommand) (*storagemodels.SkillRevision, error) {
	return nil, skillservice.ErrRepositoryUnavailable
}
func (nilSkillService) RevokeRevision(context.Context, string, int, skillservice.RevocationCommand) (*storagemodels.SkillRevision, error) {
	return nil, skillservice.ErrRepositoryUnavailable
}
func (nilSkillService) PromoteProposal(context.Context, string, skillservice.PromotionCommand) (*skillservice.PromotionResult, error) {
	return nil, skillservice.ErrRepositoryUnavailable
}
func (nilSkillService) AssignSkill(context.Context, skillservice.AssignmentCommand) (*storagemodels.SkillAssignment, error) {
	return nil, skillservice.ErrRepositoryUnavailable
}
func (nilSkillService) RevokeAssignment(context.Context, skillservice.AssignmentRevocationCommand) (*storagemodels.SkillAssignment, error) {
	return nil, skillservice.ErrRepositoryUnavailable
}
func (nilSkillService) ResolveEffectiveSkills(context.Context, skillservice.Viewer, skillservice.ResolveCommand) (*skillservice.ResolveResult, error) {
	return nil, skillservice.ErrRepositoryUnavailable
}

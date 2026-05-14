package models

import "time"

// SkillProvenanceRef represents one source/provenance reference for canonical skill state.
type SkillProvenanceRef struct {
	SourceType string `json:"source_type"`
	SourceURI  string `json:"source_uri,omitempty"`
	Digest     string `json:"digest,omitempty"`
	Ref        string `json:"ref,omitempty"`
	Notes      string `json:"notes,omitempty"`
}

// SkillRevisionFile represents one file in a canonical skill revision manifest.
type SkillRevisionFile struct {
	Path        string `json:"path"`
	Digest      string `json:"digest"`
	ContentType string `json:"content_type,omitempty"`
	Role        string `json:"role,omitempty"`
	SizeBytes   int64  `json:"size_bytes,omitempty"`
}

// SkillResource represents a canonical skill root.
type SkillResource struct {
	ID                    string               `json:"id"`
	Slug                  string               `json:"slug"`
	Name                  string               `json:"name"`
	Description           string               `json:"description,omitempty"`
	Status                string               `json:"status"`
	DefaultExposure       string               `json:"default_exposure"`
	CurrentRevisionID     string               `json:"current_revision_id,omitempty"`
	CurrentRevisionNumber int                  `json:"current_revision_number,omitempty"`
	Capabilities          []string             `json:"capabilities,omitempty"`
	Tags                  []string             `json:"tags,omitempty"`
	Provenance            []SkillProvenanceRef `json:"provenance,omitempty"`
	CreatedBy             string               `json:"created_by,omitempty"`
	UpdatedBy             string               `json:"updated_by,omitempty"`
	CreatedAt             time.Time            `json:"created_at"`
	UpdatedAt             time.Time            `json:"updated_at"`
	Version               int                  `json:"version"`
}

// SkillRevisionResource represents a canonical skill revision.
type SkillRevisionResource struct {
	ID                    string               `json:"id"`
	SkillID               string               `json:"skill_id"`
	RevisionNumber        int                  `json:"revision_number"`
	Status                string               `json:"status"`
	ProposalID            string               `json:"proposal_id,omitempty"`
	ManifestDigest        string               `json:"manifest_digest,omitempty"`
	BundleDigest          string               `json:"bundle_digest,omitempty"`
	ContentDigest         string               `json:"content_digest,omitempty"`
	Files                 []SkillRevisionFile  `json:"files,omitempty"`
	Capabilities          []string             `json:"capabilities,omitempty"`
	DefaultExposure       string               `json:"default_exposure"`
	ApprovalID            string               `json:"approval_id,omitempty"`
	ApprovalAuthorityType string               `json:"approval_authority_type,omitempty"`
	ApprovalAuthorityID   string               `json:"approval_authority_id,omitempty"`
	ApprovalDigest        string               `json:"approval_digest,omitempty"`
	ApprovalSignature     string               `json:"approval_signature,omitempty"`
	ApprovalRef           string               `json:"approval_ref,omitempty"`
	ApprovalReason        string               `json:"approval_reason,omitempty"`
	ApprovedBy            string               `json:"approved_by,omitempty"`
	ApprovedAt            *time.Time           `json:"approved_at,omitempty"`
	PrincipalID           string               `json:"principal_id,omitempty"`
	PrincipalApprovalID   string               `json:"principal_approval_id,omitempty"`
	Provenance            []SkillProvenanceRef `json:"provenance,omitempty"`
	RevokedBy             string               `json:"revoked_by,omitempty"`
	RevokedAt             *time.Time           `json:"revoked_at,omitempty"`
	RevokedReason         string               `json:"revoked_reason,omitempty"`
	CreatedBy             string               `json:"created_by,omitempty"`
	UpdatedBy             string               `json:"updated_by,omitempty"`
	CreatedAt             time.Time            `json:"created_at"`
	UpdatedAt             time.Time            `json:"updated_at"`
	Version               int                  `json:"version"`
}

// SkillProposalResource represents non-authoritative material proposed for canonicalization.
type SkillProposalResource struct {
	ID                     string               `json:"id"`
	SkillID                string               `json:"skill_id"`
	Title                  string               `json:"title,omitempty"`
	Summary                string               `json:"summary,omitempty"`
	Status                 string               `json:"status"`
	RequestedExposure      string               `json:"requested_exposure"`
	ProposedRevisionNumber int                  `json:"proposed_revision_number,omitempty"`
	ProposedManifestDigest string               `json:"proposed_manifest_digest,omitempty"`
	SourceType             string               `json:"source_type,omitempty"`
	SourceURI              string               `json:"source_uri,omitempty"`
	SourceDigest           string               `json:"source_digest,omitempty"`
	ConversationID         string               `json:"conversation_id,omitempty"`
	ConversationMessageID  string               `json:"conversation_message_id,omitempty"`
	PrincipalID            string               `json:"principal_id,omitempty"`
	PrincipalApprovalID    string               `json:"principal_approval_id,omitempty"`
	PromotedRevisionID     string               `json:"promoted_revision_id,omitempty"`
	PromotedRevisionNumber int                  `json:"promoted_revision_number,omitempty"`
	PromotionDigest        string               `json:"promotion_digest,omitempty"`
	PromotedBy             string               `json:"promoted_by,omitempty"`
	PromotedAt             *time.Time           `json:"promoted_at,omitempty"`
	Provenance             []SkillProvenanceRef `json:"provenance,omitempty"`
	CreatedBy              string               `json:"created_by,omitempty"`
	ReviewedBy             string               `json:"reviewed_by,omitempty"`
	ReviewedAt             *time.Time           `json:"reviewed_at,omitempty"`
	ReviewReason           string               `json:"review_reason,omitempty"`
	CreatedAt              time.Time            `json:"created_at"`
	UpdatedAt              time.Time            `json:"updated_at"`
	Version                int                  `json:"version"`
}

// SkillAssignmentResource represents an explicit skill assignment.
type SkillAssignmentResource struct {
	ID                  string               `json:"id"`
	SkillID             string               `json:"skill_id"`
	RevisionID          string               `json:"revision_id,omitempty"`
	RevisionNumber      int                  `json:"revision_number,omitempty"`
	SubjectType         string               `json:"subject_type"`
	SubjectID           string               `json:"subject_id"`
	Exposure            string               `json:"exposure"`
	Status              string               `json:"status"`
	ApprovalID          string               `json:"approval_id,omitempty"`
	PrincipalID         string               `json:"principal_id,omitempty"`
	PrincipalApprovalID string               `json:"principal_approval_id,omitempty"`
	Provenance          []SkillProvenanceRef `json:"provenance,omitempty"`
	AssignedBy          string               `json:"assigned_by,omitempty"`
	AssignedAt          time.Time            `json:"assigned_at"`
	RevokedBy           string               `json:"revoked_by,omitempty"`
	RevokedAt           *time.Time           `json:"revoked_at,omitempty"`
	RevokedReason       string               `json:"revoked_reason,omitempty"`
	CreatedAt           time.Time            `json:"created_at"`
	UpdatedAt           time.Time            `json:"updated_at"`
	Version             int                  `json:"version"`
}

// SkillBundleFileResource represents one file in a published approved skill bundle.
type SkillBundleFileResource struct {
	Path            string `json:"path"`
	Digest          string `json:"digest"`
	ContentType     string `json:"content_type,omitempty"`
	Role            string `json:"role,omitempty"`
	SizeBytes       int64  `json:"size_bytes,omitempty"`
	InstallPath     string `json:"install_path"`
	Content         string `json:"content,omitempty"`
	Encoding        string `json:"encoding,omitempty"`
	ContentIncluded bool   `json:"content_included"`
}

// SkillInstallHintsResource gives clients advisory placement hints without mutating their workspace.
type SkillInstallHintsResource struct {
	Layout         string   `json:"layout"`
	RuntimeTargets []string `json:"runtime_targets,omitempty"`
	DirectoryName  string   `json:"directory_name"`
	EntryPoint     string   `json:"entrypoint"`
	RequiredFiles  []string `json:"required_files,omitempty"`
}

// SkillBundleDigestsResource groups stable digests that downstream clients verify.
type SkillBundleDigestsResource struct {
	ManifestDigest    string `json:"manifest_digest,omitempty"`
	BundleDigest      string `json:"bundle_digest"`
	PublicationDigest string `json:"publication_digest"`
	ContentDigest     string `json:"content_digest,omitempty"`
	ApprovalDigest    string `json:"approval_digest,omitempty"`
}

// SkillBundleResource represents the approved revision bundle publication contract.
type SkillBundleResource struct {
	SchemaVersion         string                     `json:"schema_version"`
	BundleID              string                     `json:"bundle_id"`
	Source                string                     `json:"source"`
	Published             bool                       `json:"published"`
	SkillID               string                     `json:"skill_id"`
	RevisionID            string                     `json:"revision_id"`
	RevisionNumber        int                        `json:"revision_number"`
	DefaultExposure       string                     `json:"default_exposure"`
	Digests               SkillBundleDigestsResource `json:"digests"`
	Files                 []SkillBundleFileResource  `json:"files,omitempty"`
	InstallHints          SkillInstallHintsResource  `json:"install_hints"`
	Provenance            []SkillProvenanceRef       `json:"provenance,omitempty"`
	ApprovalID            string                     `json:"approval_id,omitempty"`
	ApprovalAuthorityType string                     `json:"approval_authority_type,omitempty"`
	ApprovalAuthorityID   string                     `json:"approval_authority_id,omitempty"`
	ApprovalSignature     string                     `json:"approval_signature,omitempty"`
	ApprovalRef           string                     `json:"approval_ref,omitempty"`
	ApprovedBy            string                     `json:"approved_by,omitempty"`
	ApprovedAt            *time.Time                 `json:"approved_at,omitempty"`
	PrincipalID           string                     `json:"principal_id,omitempty"`
	PrincipalApprovalID   string                     `json:"principal_approval_id,omitempty"`
}

// SkillCatalogEntryResource represents one approved catalog entry.
type SkillCatalogEntryResource struct {
	Skill    SkillResource         `json:"skill"`
	Revision SkillRevisionResource `json:"revision"`
	Bundle   SkillBundleResource   `json:"bundle"`
}

// SkillListResponse represents GET /api/v1/skills.
type SkillListResponse struct {
	Skills     []SkillResource `json:"skills"`
	Count      int             `json:"count"`
	NextCursor string          `json:"next_cursor,omitempty"`
}

// SkillResponse represents GET /api/v1/skills/{skillId}.
type SkillResponse struct {
	Skill SkillResource `json:"skill"`
}

// SkillRevisionsResponse represents GET /api/v1/skills/{skillId}/revisions.
type SkillRevisionsResponse struct {
	Revisions  []SkillRevisionResource `json:"revisions"`
	Count      int                     `json:"count"`
	NextCursor string                  `json:"next_cursor,omitempty"`
}

// SkillRevisionResponse represents one skill revision response.
type SkillRevisionResponse struct {
	Revision SkillRevisionResource `json:"revision"`
}

// SkillCatalogResponse represents GET /api/v1/skills/catalog.
type SkillCatalogResponse struct {
	Entries    []SkillCatalogEntryResource `json:"entries"`
	Count      int                         `json:"count"`
	NextCursor string                      `json:"next_cursor,omitempty"`
}

// SkillBundleResponse represents GET /api/v1/skills/{skillId}/revisions/{revisionNumber}/bundle.
type SkillBundleResponse struct {
	Bundle SkillBundleResource `json:"bundle"`
}

// SkillProposalsResponse represents admin proposal listings.
type SkillProposalsResponse struct {
	Proposals  []SkillProposalResource `json:"proposals"`
	Count      int                     `json:"count"`
	NextCursor string                  `json:"next_cursor,omitempty"`
}

// SkillProposalResponse represents one proposal response.
type SkillProposalResponse struct {
	Proposal SkillProposalResource `json:"proposal"`
}

// SkillAssignmentsResponse represents assignment listings.
type SkillAssignmentsResponse struct {
	Assignments []SkillAssignmentResource `json:"assignments"`
	Count       int                       `json:"count"`
	NextCursor  string                    `json:"next_cursor,omitempty"`
}

// SkillAssignmentResponse represents one assignment response.
type SkillAssignmentResponse struct {
	Assignment SkillAssignmentResource `json:"assignment"`
}

// EffectiveSkillResource represents one effective resolved skill.
type EffectiveSkillResource struct {
	Skill      SkillResource           `json:"skill"`
	Revision   SkillRevisionResource   `json:"revision"`
	Assignment SkillAssignmentResource `json:"assignment"`
}

// EffectiveSkillsResponse represents GET /api/v1/skills/resolve.
type EffectiveSkillsResponse struct {
	SubjectType string                   `json:"subject_type"`
	SubjectID   string                   `json:"subject_id"`
	Skills      []EffectiveSkillResource `json:"skills"`
	Count       int                      `json:"count"`
	NextCursor  string                   `json:"next_cursor,omitempty"`
}

// ApproveSkillRevisionRequest represents an admin revision approval request.
type ApproveSkillRevisionRequest struct {
	ApprovalID            string `json:"approval_id,omitempty"`
	PrincipalID           string `json:"principal_id,omitempty"`
	PrincipalApprovalID   string `json:"principal_approval_id,omitempty"`
	ApprovalAuthorityType string `json:"approval_authority_type,omitempty"`
	ApprovalAuthorityID   string `json:"approval_authority_id,omitempty"`
	ApprovalDigest        string `json:"approval_digest,omitempty"`
	ApprovalSignature     string `json:"approval_signature,omitempty"`
	ApprovalRef           string `json:"approval_ref,omitempty"`
	ApprovalReason        string `json:"approval_reason,omitempty"`
}

// RevokeSkillRevisionRequest represents an admin revision revocation request.
type RevokeSkillRevisionRequest struct {
	Reason string `json:"reason,omitempty"`
}

// PromoteSkillProposalRequest represents an admin proposal promotion request.
type PromoteSkillProposalRequest struct {
	ExpectedManifestDigest string `json:"expected_manifest_digest,omitempty"`
	ExpectedSourceDigest   string `json:"expected_source_digest,omitempty"`
	ApprovalID             string `json:"approval_id,omitempty"`
	PrincipalID            string `json:"principal_id,omitempty"`
	PrincipalApprovalID    string `json:"principal_approval_id,omitempty"`
	ApprovalAuthorityType  string `json:"approval_authority_type,omitempty"`
	ApprovalAuthorityID    string `json:"approval_authority_id,omitempty"`
	ApprovalDigest         string `json:"approval_digest,omitempty"`
	ApprovalSignature      string `json:"approval_signature,omitempty"`
	ApprovalRef            string `json:"approval_ref,omitempty"`
	ApprovalReason         string `json:"approval_reason,omitempty"`
}

// PromoteSkillProposalResponse represents the canonical revision produced by promotion.
type PromoteSkillProposalResponse struct {
	Revision SkillRevisionResource `json:"revision"`
	Proposal SkillProposalResource `json:"proposal"`
	Created  bool                  `json:"created"`
}

// CreateSkillAssignmentRequest represents an admin assignment request.
type CreateSkillAssignmentRequest struct {
	AssignmentID        string `json:"assignment_id,omitempty"`
	RevisionNumber      int    `json:"revision_number,omitempty"`
	SubjectType         string `json:"subject_type"`
	SubjectID           string `json:"subject_id"`
	Exposure            string `json:"exposure,omitempty"`
	ApprovalID          string `json:"approval_id,omitempty"`
	PrincipalID         string `json:"principal_id,omitempty"`
	PrincipalApprovalID string `json:"principal_approval_id,omitempty"`
}

// RevokeSkillAssignmentRequest represents an admin assignment revocation request.
type RevokeSkillAssignmentRequest struct {
	SubjectType string `json:"subject_type"`
	SubjectID   string `json:"subject_id"`
	Reason      string `json:"reason,omitempty"`
}

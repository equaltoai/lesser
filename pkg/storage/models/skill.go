package models

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
)

const (
	// SKSkill is the root sort key for a canonical skill authority record.
	SKSkill = "SKILL"

	// SKSkillRevisionPrefix is the sort-key prefix for canonical skill revisions.
	SKSkillRevisionPrefix = "REVISION#"

	// SKSkillProposal is the sort key for proposal records.
	SKSkillProposal = "PROPOSAL"

	// SKSkillAssignmentPrefix is the sort-key prefix for skill assignments under a subject partition.
	SKSkillAssignmentPrefix = "SKILL#"
)

const (
	// SkillStatusDraft indicates a canonical skill record exists but has no exposed approved revision yet.
	SkillStatusDraft = "draft"
	// SkillStatusActive indicates a canonical skill has at least one active revision or assignment path.
	SkillStatusActive = "active"
	// SkillStatusArchived indicates a canonical skill is retained but no longer assignable by default.
	SkillStatusArchived = "archived"
)

const (
	// SkillRevisionStatusDraft indicates a revision is still being formed inside Lesser.
	SkillRevisionStatusDraft = "draft"
	// SkillRevisionStatusProposed indicates a revision is ready for a later approval flow.
	SkillRevisionStatusProposed = "proposed"
	// SkillRevisionStatusApproved indicates a revision has passed a later approval flow.
	SkillRevisionStatusApproved = "approved"
	// SkillRevisionStatusSuperseded indicates a revision was replaced by a newer approved revision.
	SkillRevisionStatusSuperseded = "superseded"
	// SkillRevisionStatusRevoked indicates a revision should no longer be used.
	SkillRevisionStatusRevoked = "revoked"
)

const (
	// SkillProposalStatusProposed indicates a proposal is awaiting review.
	SkillProposalStatusProposed = "proposed"
	// SkillProposalStatusAccepted indicates a proposal produced or will produce a canonical revision.
	SkillProposalStatusAccepted = "accepted"
	// SkillProposalStatusRejected indicates a proposal was rejected.
	SkillProposalStatusRejected = "rejected"
	// SkillProposalStatusWithdrawn indicates a proposal was withdrawn before approval.
	SkillProposalStatusWithdrawn = "withdrawn"
)

const (
	// SkillAssignmentStatusActive indicates an assignment currently participates in effective resolution.
	SkillAssignmentStatusActive = "active"
	// SkillAssignmentStatusPending indicates an assignment is staged but not effective yet.
	SkillAssignmentStatusPending = "pending"
	// SkillAssignmentStatusRevoked indicates an assignment was explicitly revoked.
	SkillAssignmentStatusRevoked = "revoked"
)

const (
	// SkillExposurePublic means the skill can be exposed publicly once approved.
	SkillExposurePublic = "public"
	// SkillExposureInstance means the skill is visible within the local Lesser instance.
	SkillExposureInstance = "instance"
	// SkillExposurePrivate means the skill is only visible to explicitly assigned/private principals.
	SkillExposurePrivate = "private"
)

const (
	// SkillSourceTypeLocalFile records SKILL.md seed material imported from a repo/workspace.
	SkillSourceTypeLocalFile = "local_file"
	// SkillSourceTypeHostConversation records lesser-host mint conversation provenance without making host canonical.
	SkillSourceTypeHostConversation = "host_conversation"
	// SkillSourceTypeManual records operator or maintainer-created skill material.
	SkillSourceTypeManual = "manual"
	// SkillSourceTypeProposal records provenance that points at a Lesser-owned SkillProposal.
	SkillSourceTypeProposal = "proposal"
	// SkillSourceTypeApproval records provenance that points at a principal approval artifact.
	SkillSourceTypeApproval = "approval"
)

const (
	// SkillAssignmentSubjectInstance assigns a skill at instance scope.
	SkillAssignmentSubjectInstance = "instance"
	// SkillAssignmentSubjectActor assigns a skill to a local actor/body.
	SkillAssignmentSubjectActor = "actor"
	// SkillAssignmentSubjectPrincipal assigns a skill to an approving or owning principal.
	SkillAssignmentSubjectPrincipal = "principal"
)

const (
	// SkillApprovalAuthorityAdmin means a local Lesser admin approved the revision.
	SkillApprovalAuthorityAdmin = "admin"
	// SkillApprovalAuthorityPrincipal means the named principal approved the revision.
	SkillApprovalAuthorityPrincipal = "principal"
	// SkillApprovalAuthorityInstance means instance-level governance approved the revision.
	SkillApprovalAuthorityInstance = "instance"
)

// SkillProvenanceRef captures source/provenance for canonical skills, proposals, and revisions.
type SkillProvenanceRef struct {
	SourceType string `theorydb:"attr:sourceType" json:"source_type"`
	SourceURI  string `theorydb:"attr:sourceURI,omitempty" json:"source_uri,omitempty"`
	Digest     string `theorydb:"attr:digest,omitempty" json:"digest,omitempty"`
	Ref        string `theorydb:"attr:ref,omitempty" json:"ref,omitempty"`
	Notes      string `theorydb:"attr:notes,omitempty" json:"notes,omitempty"`
}

// SkillRevisionFile records a publishable file included in a canonical skill revision manifest.
type SkillRevisionFile struct {
	Path        string `theorydb:"attr:path" json:"path"`
	Digest      string `theorydb:"attr:digest" json:"digest"`
	ContentType string `theorydb:"attr:contentType,omitempty" json:"content_type,omitempty"`
	Role        string `theorydb:"attr:role,omitempty" json:"role,omitempty"`
	SizeBytes   int64  `theorydb:"attr:sizeBytes,omitempty" json:"size_bytes,omitempty"`
}

// Skill stores canonical instance-owned skill authority metadata.
type Skill struct {
	_ struct{} `theorydb:"naming:camelCase"`

	PK string `theorydb:"pk,attr:PK" json:"-"`
	SK string `theorydb:"sk,attr:SK" json:"-"`

	// GSI1 lists canonical skills by lifecycle status for later approval/catalog APIs.
	GSI1PK string `theorydb:"index:gsi1,pk,attr:gsi1PK,omitempty" json:"-"`
	GSI1SK string `theorydb:"index:gsi1,sk,attr:gsi1SK,omitempty" json:"-"`

	// GSI2 lists canonical skills by default exposure class without exposing private content.
	GSI2PK string `theorydb:"index:gsi2,pk,attr:gsi2PK,omitempty" json:"-"`
	GSI2SK string `theorydb:"index:gsi2,sk,attr:gsi2SK,omitempty" json:"-"`

	ID                    string               `theorydb:"attr:id" json:"id"`
	Slug                  string               `theorydb:"attr:slug" json:"slug"`
	Name                  string               `theorydb:"attr:name" json:"name"`
	Description           string               `theorydb:"attr:description,omitempty" json:"description,omitempty"`
	Status                string               `theorydb:"attr:status" json:"status"`
	DefaultExposure       string               `theorydb:"attr:defaultExposure" json:"default_exposure"`
	CurrentRevisionID     string               `theorydb:"attr:currentRevisionID,omitempty" json:"current_revision_id,omitempty"`
	CurrentRevisionNumber int                  `theorydb:"attr:currentRevisionNumber,omitempty" json:"current_revision_number,omitempty"`
	Capabilities          []string             `theorydb:"attr:capabilities" json:"capabilities,omitempty"`
	Tags                  []string             `theorydb:"attr:tags" json:"tags,omitempty"`
	Provenance            []SkillProvenanceRef `theorydb:"attr:provenance" json:"provenance,omitempty"`
	CreatedBy             string               `theorydb:"attr:createdBy,omitempty" json:"created_by,omitempty"`
	UpdatedBy             string               `theorydb:"attr:updatedBy,omitempty" json:"updated_by,omitempty"`
	CreatedAt             time.Time            `theorydb:"attr:createdAt" json:"created_at"`
	UpdatedAt             time.Time            `theorydb:"attr:updatedAt" json:"updated_at"`
	Version               int                  `theorydb:"version,attr:version" json:"version"`
}

// SkillRevision stores a canonical revision under its skill partition.
type SkillRevision struct {
	_ struct{} `theorydb:"naming:camelCase"`

	PK string `theorydb:"pk,attr:PK" json:"-"`
	SK string `theorydb:"sk,attr:SK" json:"-"`

	// GSI1 supports later review/effective-resolution queues by revision status.
	GSI1PK string `theorydb:"index:gsi1,pk,attr:gsi1PK,omitempty" json:"-"`
	GSI1SK string `theorydb:"index:gsi1,sk,attr:gsi1SK,omitempty" json:"-"`

	// GSI2 is sparse and supports digest/provenance de-duplication for imported manifests.
	GSI2PK string `theorydb:"index:gsi2,pk,attr:gsi2PK,omitempty" json:"-"`
	GSI2SK string `theorydb:"index:gsi2,sk,attr:gsi2SK,omitempty" json:"-"`

	ID                    string               `theorydb:"attr:id" json:"id"`
	SkillID               string               `theorydb:"attr:skillID" json:"skill_id"`
	RevisionNumber        int                  `theorydb:"attr:revisionNumber" json:"revision_number"`
	Status                string               `theorydb:"attr:status" json:"status"`
	ProposalID            string               `theorydb:"attr:proposalID,omitempty" json:"proposal_id,omitempty"`
	ManifestJSON          string               `theorydb:"attr:manifestJSON,omitempty" json:"manifest_json,omitempty"`
	ManifestDigest        string               `theorydb:"attr:manifestDigest,omitempty" json:"manifest_digest,omitempty"`
	BundleDigest          string               `theorydb:"attr:bundleDigest,omitempty" json:"bundle_digest,omitempty"`
	ContentDigest         string               `theorydb:"attr:contentDigest,omitempty" json:"content_digest,omitempty"`
	Files                 []SkillRevisionFile  `theorydb:"attr:files" json:"files,omitempty"`
	Capabilities          []string             `theorydb:"attr:capabilities" json:"capabilities,omitempty"`
	DefaultExposure       string               `theorydb:"attr:defaultExposure" json:"default_exposure"`
	ApprovalID            string               `theorydb:"attr:approvalID,omitempty" json:"approval_id,omitempty"`
	ApprovalAuthorityType string               `theorydb:"attr:approvalAuthorityType,omitempty" json:"approval_authority_type,omitempty"`
	ApprovalAuthorityID   string               `theorydb:"attr:approvalAuthorityID,omitempty" json:"approval_authority_id,omitempty"`
	ApprovalDigest        string               `theorydb:"attr:approvalDigest,omitempty" json:"approval_digest,omitempty"`
	ApprovalSignature     string               `theorydb:"attr:approvalSignature,omitempty" json:"approval_signature,omitempty"`
	ApprovalRef           string               `theorydb:"attr:approvalRef,omitempty" json:"approval_ref,omitempty"`
	ApprovalReason        string               `theorydb:"attr:approvalReason,omitempty" json:"approval_reason,omitempty"`
	ApprovedBy            string               `theorydb:"attr:approvedBy,omitempty" json:"approved_by,omitempty"`
	ApprovedAt            *time.Time           `theorydb:"attr:approvedAt" json:"approved_at,omitempty"`
	PrincipalID           string               `theorydb:"attr:principalID,omitempty" json:"principal_id,omitempty"`
	PrincipalApprovalID   string               `theorydb:"attr:principalApprovalID,omitempty" json:"principal_approval_id,omitempty"`
	Provenance            []SkillProvenanceRef `theorydb:"attr:provenance" json:"provenance,omitempty"`
	RevokedBy             string               `theorydb:"attr:revokedBy,omitempty" json:"revoked_by,omitempty"`
	RevokedAt             *time.Time           `theorydb:"attr:revokedAt" json:"revoked_at,omitempty"`
	RevokedReason         string               `theorydb:"attr:revokedReason,omitempty" json:"revoked_reason,omitempty"`
	CreatedBy             string               `theorydb:"attr:createdBy,omitempty" json:"created_by,omitempty"`
	UpdatedBy             string               `theorydb:"attr:updatedBy,omitempty" json:"updated_by,omitempty"`
	CreatedAt             time.Time            `theorydb:"attr:createdAt" json:"created_at"`
	UpdatedAt             time.Time            `theorydb:"attr:updatedAt" json:"updated_at"`
	Version               int                  `theorydb:"version,attr:version" json:"version"`
}

// SkillProposal stores non-authoritative source material proposed for canonicalization.
type SkillProposal struct {
	_ struct{} `theorydb:"naming:camelCase"`

	PK string `theorydb:"pk,attr:PK" json:"-"`
	SK string `theorydb:"sk,attr:SK" json:"-"`

	// GSI1 lists proposals for a skill.
	GSI1PK string `theorydb:"index:gsi1,pk,attr:gsi1PK,omitempty" json:"-"`
	GSI1SK string `theorydb:"index:gsi1,sk,attr:gsi1SK,omitempty" json:"-"`

	// GSI2 supports later proposal review queues by status.
	GSI2PK string `theorydb:"index:gsi2,pk,attr:gsi2PK,omitempty" json:"-"`
	GSI2SK string `theorydb:"index:gsi2,sk,attr:gsi2SK,omitempty" json:"-"`

	ID                     string               `theorydb:"attr:id" json:"id"`
	SkillID                string               `theorydb:"attr:skillID" json:"skill_id"`
	Title                  string               `theorydb:"attr:title,omitempty" json:"title,omitempty"`
	Summary                string               `theorydb:"attr:summary,omitempty" json:"summary,omitempty"`
	Status                 string               `theorydb:"attr:status" json:"status"`
	RequestedExposure      string               `theorydb:"attr:requestedExposure" json:"requested_exposure"`
	ProposedRevisionNumber int                  `theorydb:"attr:proposedRevisionNumber,omitempty" json:"proposed_revision_number,omitempty"`
	ProposedManifestJSON   string               `theorydb:"attr:proposedManifestJSON,omitempty" json:"proposed_manifest_json,omitempty"`
	ProposedManifestDigest string               `theorydb:"attr:proposedManifestDigest,omitempty" json:"proposed_manifest_digest,omitempty"`
	SourceType             string               `theorydb:"attr:sourceType,omitempty" json:"source_type,omitempty"`
	SourceURI              string               `theorydb:"attr:sourceURI,omitempty" json:"source_uri,omitempty"`
	SourceDigest           string               `theorydb:"attr:sourceDigest,omitempty" json:"source_digest,omitempty"`
	ConversationID         string               `theorydb:"attr:conversationID,omitempty" json:"conversation_id,omitempty"`
	ConversationMessageID  string               `theorydb:"attr:conversationMessageID,omitempty" json:"conversation_message_id,omitempty"`
	PrincipalID            string               `theorydb:"attr:principalID,omitempty" json:"principal_id,omitempty"`
	PrincipalApprovalID    string               `theorydb:"attr:principalApprovalID,omitempty" json:"principal_approval_id,omitempty"`
	Provenance             []SkillProvenanceRef `theorydb:"attr:provenance" json:"provenance,omitempty"`
	CreatedBy              string               `theorydb:"attr:createdBy,omitempty" json:"created_by,omitempty"`
	ReviewedBy             string               `theorydb:"attr:reviewedBy,omitempty" json:"reviewed_by,omitempty"`
	ReviewedAt             *time.Time           `theorydb:"attr:reviewedAt" json:"reviewed_at,omitempty"`
	ReviewReason           string               `theorydb:"attr:reviewReason,omitempty" json:"review_reason,omitempty"`
	CreatedAt              time.Time            `theorydb:"attr:createdAt" json:"created_at"`
	UpdatedAt              time.Time            `theorydb:"attr:updatedAt" json:"updated_at"`
	Version                int                  `theorydb:"version,attr:version" json:"version"`
}

// SkillAssignment stores an explicit skill assignment for effective-resolution APIs added later.
type SkillAssignment struct {
	_ struct{} `theorydb:"naming:camelCase"`

	PK string `theorydb:"pk,attr:PK" json:"-"`
	SK string `theorydb:"sk,attr:SK" json:"-"`

	// GSI1 lists assignments by skill for audit and revocation walks.
	GSI1PK string `theorydb:"index:gsi1,pk,attr:gsi1PK,omitempty" json:"-"`
	GSI1SK string `theorydb:"index:gsi1,sk,attr:gsi1SK,omitempty" json:"-"`

	// GSI2 supports sparse status queues for pending/revoked assignment processing.
	GSI2PK string `theorydb:"index:gsi2,pk,attr:gsi2PK,omitempty" json:"-"`
	GSI2SK string `theorydb:"index:gsi2,sk,attr:gsi2SK,omitempty" json:"-"`

	ID                  string               `theorydb:"attr:id" json:"id"`
	SkillID             string               `theorydb:"attr:skillID" json:"skill_id"`
	RevisionID          string               `theorydb:"attr:revisionID,omitempty" json:"revision_id,omitempty"`
	RevisionNumber      int                  `theorydb:"attr:revisionNumber,omitempty" json:"revision_number,omitempty"`
	SubjectType         string               `theorydb:"attr:subjectType" json:"subject_type"`
	SubjectID           string               `theorydb:"attr:subjectID" json:"subject_id"`
	Exposure            string               `theorydb:"attr:exposure" json:"exposure"`
	Status              string               `theorydb:"attr:status" json:"status"`
	ApprovalID          string               `theorydb:"attr:approvalID,omitempty" json:"approval_id,omitempty"`
	PrincipalID         string               `theorydb:"attr:principalID,omitempty" json:"principal_id,omitempty"`
	PrincipalApprovalID string               `theorydb:"attr:principalApprovalID,omitempty" json:"principal_approval_id,omitempty"`
	Provenance          []SkillProvenanceRef `theorydb:"attr:provenance" json:"provenance,omitempty"`
	AssignedBy          string               `theorydb:"attr:assignedBy,omitempty" json:"assigned_by,omitempty"`
	AssignedAt          time.Time            `theorydb:"attr:assignedAt" json:"assigned_at"`
	RevokedBy           string               `theorydb:"attr:revokedBy,omitempty" json:"revoked_by,omitempty"`
	RevokedAt           *time.Time           `theorydb:"attr:revokedAt" json:"revoked_at,omitempty"`
	RevokedReason       string               `theorydb:"attr:revokedReason,omitempty" json:"revoked_reason,omitempty"`
	CreatedAt           time.Time            `theorydb:"attr:createdAt" json:"created_at"`
	UpdatedAt           time.Time            `theorydb:"attr:updatedAt" json:"updated_at"`
	Version             int                  `theorydb:"version,attr:version" json:"version"`
}

// TableName returns the DynamoDB table backing Skill.
func (Skill) TableName() string { return MainTableName }

// TableName returns the DynamoDB table backing SkillRevision.
func (SkillRevision) TableName() string { return MainTableName }

// TableName returns the DynamoDB table backing SkillProposal.
func (SkillProposal) TableName() string { return MainTableName }

// TableName returns the DynamoDB table backing SkillAssignment.
func (SkillAssignment) TableName() string { return MainTableName }

// SkillPartitionKey returns the canonical partition for a skill and its revisions.
func SkillPartitionKey(skillID string) string {
	skillID = normalizeSkillID(skillID)
	if skillID == "" {
		return ""
	}
	return "SKILL#" + skillID
}

// SkillRevisionSortKey returns the zero-padded revision sort key.
func SkillRevisionSortKey(revisionNumber int) string {
	if revisionNumber <= 0 {
		return ""
	}
	return fmt.Sprintf("%s%08d", SKSkillRevisionPrefix, revisionNumber)
}

// SkillProposalPartitionKey returns the primary partition for a proposal.
func SkillProposalPartitionKey(proposalID string) string {
	proposalID = normalizeSkillID(proposalID)
	if proposalID == "" {
		return ""
	}
	return "SKILL_PROPOSAL#" + proposalID
}

// SkillAssignmentPartitionKey returns the primary partition for a subject assignment set.
func SkillAssignmentPartitionKey(subjectType, subjectID string) string {
	subjectType = normalizeSkillToken(subjectType)
	subjectID = normalizeSkillID(subjectID)
	if subjectType == "" || subjectID == "" {
		return ""
	}
	return fmt.Sprintf("SKILL_ASSIGNMENT#%s#%s", subjectType, subjectID)
}

// SkillAssignmentSortKey returns the sort key for a subject assignment.
func SkillAssignmentSortKey(skillID, assignmentID string) string {
	skillID = normalizeSkillID(skillID)
	assignmentID = normalizeSkillID(assignmentID)
	if skillID == "" || assignmentID == "" {
		return ""
	}
	return fmt.Sprintf("%s%s#ASSIGNMENT#%s", SKSkillAssignmentPrefix, skillID, assignmentID)
}

// SkillRevisionApprovalDigest returns the canonical digest an approval signs for a revision.
func SkillRevisionApprovalDigest(revision *SkillRevision, principalID, authorityType, authorityID string) (string, error) {
	if revision == nil {
		return "", fmt.Errorf("skill revision is required")
	}
	skillID := normalizeSkillID(revision.SkillID)
	if err := common.ValidateRequiredParam("SkillID", skillID); err != nil {
		return "", err
	}
	if revision.RevisionNumber <= 0 {
		return "", fmt.Errorf("revision number is required")
	}
	principalID = strings.TrimSpace(principalID)
	if err := common.ValidateRequiredParam("principalID", principalID); err != nil {
		return "", err
	}
	authorityType = normalizeSkillApprovalAuthority(authorityType)
	if err := validateSkillApprovalAuthority("approvalAuthorityType", authorityType); err != nil {
		return "", err
	}
	authorityID = strings.TrimSpace(authorityID)
	if err := common.ValidateRequiredParam("approvalAuthorityID", authorityID); err != nil {
		return "", err
	}

	material := strings.Join([]string{
		"lesser-skill-revision-approval-v1",
		skillID,
		fmt.Sprintf("%08d", revision.RevisionNumber),
		normalizeSkillID(revision.ID),
		normalizeSkillID(revision.ProposalID),
		normalizeSkillDigest(revision.ManifestDigest),
		normalizeSkillDigest(revision.ContentDigest),
		normalizeSkillDigest(revision.BundleDigest),
		principalID,
		authorityType,
		authorityID,
	}, "\x1f")
	sum := sha256.Sum256([]byte(material))
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

// BeforeCreate normalizes a Skill before insert.
func (s *Skill) BeforeCreate() error { return s.prepareForWrite(true) }

// BeforeUpdate normalizes a Skill before update.
func (s *Skill) BeforeUpdate() error { return s.prepareForWrite(false) }

// UpdateKeys refreshes Skill primary and secondary keys.
func (s *Skill) UpdateKeys() error { return s.prepareForWrite(false) }

// GetPK returns the partition key.
func (s *Skill) GetPK() string {
	if s == nil {
		return ""
	}
	return s.PK
}

// GetSK returns the sort key.
func (s *Skill) GetSK() string {
	if s == nil {
		return ""
	}
	return s.SK
}

// BeforeCreate normalizes a SkillRevision before insert.
func (r *SkillRevision) BeforeCreate() error { return r.prepareForWrite(true) }

// BeforeUpdate normalizes a SkillRevision before update.
func (r *SkillRevision) BeforeUpdate() error { return r.prepareForWrite(false) }

// UpdateKeys refreshes SkillRevision primary and secondary keys.
func (r *SkillRevision) UpdateKeys() error { return r.prepareForWrite(false) }

// GetPK returns the partition key.
func (r *SkillRevision) GetPK() string {
	if r == nil {
		return ""
	}
	return r.PK
}

// GetSK returns the sort key.
func (r *SkillRevision) GetSK() string {
	if r == nil {
		return ""
	}
	return r.SK
}

// BeforeCreate normalizes a SkillProposal before insert.
func (p *SkillProposal) BeforeCreate() error { return p.prepareForWrite(true) }

// BeforeUpdate normalizes a SkillProposal before update.
func (p *SkillProposal) BeforeUpdate() error { return p.prepareForWrite(false) }

// UpdateKeys refreshes SkillProposal primary and secondary keys.
func (p *SkillProposal) UpdateKeys() error { return p.prepareForWrite(false) }

// GetPK returns the partition key.
func (p *SkillProposal) GetPK() string {
	if p == nil {
		return ""
	}
	return p.PK
}

// GetSK returns the sort key.
func (p *SkillProposal) GetSK() string {
	if p == nil {
		return ""
	}
	return p.SK
}

// BeforeCreate normalizes a SkillAssignment before insert.
func (a *SkillAssignment) BeforeCreate() error { return a.prepareForWrite(true) }

// BeforeUpdate normalizes a SkillAssignment before update.
func (a *SkillAssignment) BeforeUpdate() error { return a.prepareForWrite(false) }

// UpdateKeys refreshes SkillAssignment primary and secondary keys.
func (a *SkillAssignment) UpdateKeys() error { return a.prepareForWrite(false) }

// GetPK returns the partition key.
func (a *SkillAssignment) GetPK() string {
	if a == nil {
		return ""
	}
	return a.PK
}

// GetSK returns the sort key.
func (a *SkillAssignment) GetSK() string {
	if a == nil {
		return ""
	}
	return a.SK
}

func (s *Skill) prepareForWrite(isCreate bool) error {
	if s == nil {
		return fmt.Errorf("skill is required")
	}
	s.ID = normalizeSkillID(s.ID)
	if err := common.ValidateRequiredParam("ID", s.ID); err != nil {
		return err
	}
	s.Slug = normalizeSkillSlug(defaultString(s.Slug, s.ID))
	s.Name = strings.TrimSpace(s.Name)
	if s.Name == "" {
		s.Name = s.Slug
	}
	s.Status = normalizeSkillStatus(s.Status, SkillStatusDraft)
	if err := validateSkillStatus("Status", s.Status, SkillStatusDraft, SkillStatusActive, SkillStatusArchived); err != nil {
		return err
	}
	s.DefaultExposure = normalizeSkillExposure(s.DefaultExposure, SkillExposurePrivate)
	if err := validateSkillExposure("DefaultExposure", s.DefaultExposure); err != nil {
		return err
	}
	s.CurrentRevisionID = normalizeSkillID(s.CurrentRevisionID)
	s.Capabilities = normalizeSkillStringSet(s.Capabilities, false)
	s.Tags = normalizeSkillStringSet(s.Tags, true)
	s.Provenance = normalizeSkillProvenance(s.Provenance)
	if err := validateSkillProvenance(s.Provenance); err != nil {
		return err
	}
	s.CreatedBy = strings.TrimSpace(s.CreatedBy)
	s.UpdatedBy = strings.TrimSpace(s.UpdatedBy)
	s.applyTimestamps(isCreate)
	s.PK = SkillPartitionKey(s.ID)
	s.SK = SKSkill
	s.GSI1PK = "SKILL#STATUS#" + s.Status
	s.GSI1SK = fmt.Sprintf("UPDATED#%s#SKILL#%s", formatSkillTime(s.UpdatedAt), s.ID)
	s.GSI2PK = "SKILL#EXPOSURE#" + s.DefaultExposure
	s.GSI2SK = fmt.Sprintf("NAME#%s#SKILL#%s", s.Slug, s.ID)
	return nil
}

func (r *SkillRevision) prepareForWrite(isCreate bool) error {
	if r == nil {
		return fmt.Errorf("skill revision is required")
	}
	r.SkillID = normalizeSkillID(r.SkillID)
	if err := common.ValidateRequiredParam("SkillID", r.SkillID); err != nil {
		return err
	}
	if r.RevisionNumber <= 0 {
		return fmt.Errorf("revision number is required")
	}
	r.ID = normalizeSkillID(r.ID)
	if r.ID == "" {
		r.ID = fmt.Sprintf("%s-r%d", r.SkillID, r.RevisionNumber)
	}
	r.Status = normalizeSkillStatus(r.Status, SkillRevisionStatusDraft)
	if err := validateSkillStatus("Status", r.Status, SkillRevisionStatusDraft, SkillRevisionStatusProposed, SkillRevisionStatusApproved, SkillRevisionStatusSuperseded, SkillRevisionStatusRevoked); err != nil {
		return err
	}
	r.ProposalID = normalizeSkillID(r.ProposalID)
	r.ManifestDigest = normalizeSkillDigest(r.ManifestDigest)
	r.BundleDigest = normalizeSkillDigest(r.BundleDigest)
	r.ContentDigest = normalizeSkillDigest(r.ContentDigest)
	r.Files = normalizeSkillRevisionFiles(r.Files)
	r.Capabilities = normalizeSkillStringSet(r.Capabilities, false)
	r.DefaultExposure = normalizeSkillExposure(r.DefaultExposure, SkillExposurePrivate)
	if err := validateSkillExposure("DefaultExposure", r.DefaultExposure); err != nil {
		return err
	}
	r.ApprovalID = strings.TrimSpace(r.ApprovalID)
	r.ApprovalAuthorityType = normalizeSkillApprovalAuthority(r.ApprovalAuthorityType)
	r.ApprovalAuthorityID = strings.TrimSpace(r.ApprovalAuthorityID)
	r.ApprovalDigest = normalizeSkillDigest(r.ApprovalDigest)
	r.ApprovalSignature = strings.TrimSpace(r.ApprovalSignature)
	r.ApprovalRef = strings.TrimSpace(r.ApprovalRef)
	r.ApprovalReason = strings.TrimSpace(r.ApprovalReason)
	r.ApprovedBy = strings.TrimSpace(r.ApprovedBy)
	r.ApprovedAt = normalizeSkillTimePtr(r.ApprovedAt)
	r.PrincipalID = strings.TrimSpace(r.PrincipalID)
	r.PrincipalApprovalID = strings.TrimSpace(r.PrincipalApprovalID)
	r.Provenance = normalizeSkillProvenance(r.Provenance)
	if err := validateSkillProvenance(r.Provenance); err != nil {
		return err
	}
	r.RevokedBy = strings.TrimSpace(r.RevokedBy)
	r.RevokedAt = normalizeSkillTimePtr(r.RevokedAt)
	r.RevokedReason = strings.TrimSpace(r.RevokedReason)
	r.CreatedBy = strings.TrimSpace(r.CreatedBy)
	r.UpdatedBy = strings.TrimSpace(r.UpdatedBy)
	if err := r.validateApprovalState(); err != nil {
		return err
	}
	r.applyTimestamps(isCreate)
	r.PK = SkillPartitionKey(r.SkillID)
	r.SK = SkillRevisionSortKey(r.RevisionNumber)
	r.GSI1PK = "SKILL_REVISION#STATUS#" + r.Status
	r.GSI1SK = fmt.Sprintf("UPDATED#%s#SKILL#%s#REVISION#%08d", formatSkillTime(r.UpdatedAt), r.SkillID, r.RevisionNumber)
	if r.ManifestDigest != "" {
		r.GSI2PK = "SKILL_REVISION_DIGEST#" + r.ManifestDigest
		r.GSI2SK = fmt.Sprintf("SKILL#%s#REVISION#%08d", r.SkillID, r.RevisionNumber)
	} else {
		r.GSI2PK = ""
		r.GSI2SK = ""
	}
	return nil
}

func (p *SkillProposal) prepareForWrite(isCreate bool) error {
	if p == nil {
		return fmt.Errorf("skill proposal is required")
	}
	p.ID = normalizeSkillID(p.ID)
	if err := common.ValidateRequiredParam("ID", p.ID); err != nil {
		return err
	}
	p.SkillID = normalizeSkillID(p.SkillID)
	if err := common.ValidateRequiredParam("SkillID", p.SkillID); err != nil {
		return err
	}
	p.Title = strings.TrimSpace(p.Title)
	p.Summary = strings.TrimSpace(p.Summary)
	p.Status = normalizeSkillStatus(p.Status, SkillProposalStatusProposed)
	if err := validateSkillStatus("Status", p.Status, SkillProposalStatusProposed, SkillProposalStatusAccepted, SkillProposalStatusRejected, SkillProposalStatusWithdrawn); err != nil {
		return err
	}
	p.RequestedExposure = normalizeSkillExposure(p.RequestedExposure, SkillExposurePrivate)
	if err := validateSkillExposure("RequestedExposure", p.RequestedExposure); err != nil {
		return err
	}
	p.ProposedManifestDigest = normalizeSkillDigest(p.ProposedManifestDigest)
	p.SourceType = normalizeSkillSourceType(p.SourceType)
	if p.SourceType == "" {
		p.SourceType = SkillSourceTypeManual
	}
	if err := validateSkillSourceType("SourceType", p.SourceType, false); err != nil {
		return err
	}
	p.SourceURI = strings.TrimSpace(p.SourceURI)
	p.SourceDigest = normalizeSkillDigest(p.SourceDigest)
	p.ConversationID = strings.TrimSpace(p.ConversationID)
	p.ConversationMessageID = strings.TrimSpace(p.ConversationMessageID)
	p.PrincipalID = strings.TrimSpace(p.PrincipalID)
	p.PrincipalApprovalID = strings.TrimSpace(p.PrincipalApprovalID)
	p.Provenance = normalizeSkillProvenance(p.Provenance)
	if err := validateSkillProvenance(p.Provenance); err != nil {
		return err
	}
	p.CreatedBy = strings.TrimSpace(p.CreatedBy)
	p.ReviewedBy = strings.TrimSpace(p.ReviewedBy)
	p.ReviewedAt = normalizeSkillTimePtr(p.ReviewedAt)
	p.ReviewReason = strings.TrimSpace(p.ReviewReason)
	p.applyTimestamps(isCreate)
	p.PK = SkillProposalPartitionKey(p.ID)
	p.SK = SKSkillProposal
	p.GSI1PK = "SKILL#" + p.SkillID + "#PROPOSAL"
	p.GSI1SK = fmt.Sprintf("STATUS#%s#CREATED#%s#PROPOSAL#%s", p.Status, formatSkillTime(p.CreatedAt), p.ID)
	p.GSI2PK = "SKILL_PROPOSAL#STATUS#" + p.Status
	p.GSI2SK = fmt.Sprintf("CREATED#%s#SKILL#%s#PROPOSAL#%s", formatSkillTime(p.CreatedAt), p.SkillID, p.ID)
	return nil
}

func (a *SkillAssignment) prepareForWrite(isCreate bool) error {
	if a == nil {
		return fmt.Errorf("skill assignment is required")
	}
	a.ID = normalizeSkillID(a.ID)
	if err := common.ValidateRequiredParam("ID", a.ID); err != nil {
		return err
	}
	a.SkillID = normalizeSkillID(a.SkillID)
	if err := common.ValidateRequiredParam("SkillID", a.SkillID); err != nil {
		return err
	}
	a.RevisionID = normalizeSkillID(a.RevisionID)
	a.SubjectType = normalizeSkillSubjectType(a.SubjectType)
	if err := common.ValidateRequiredParam("SubjectType", a.SubjectType); err != nil {
		return err
	}
	if err := validateSkillSubjectType("SubjectType", a.SubjectType); err != nil {
		return err
	}
	a.SubjectID = normalizeSkillID(a.SubjectID)
	if err := common.ValidateRequiredParam("SubjectID", a.SubjectID); err != nil {
		return err
	}
	a.Exposure = normalizeSkillExposure(a.Exposure, SkillExposurePrivate)
	if err := validateSkillExposure("Exposure", a.Exposure); err != nil {
		return err
	}
	a.Status = normalizeSkillStatus(a.Status, SkillAssignmentStatusActive)
	if err := validateSkillStatus("Status", a.Status, SkillAssignmentStatusActive, SkillAssignmentStatusPending, SkillAssignmentStatusRevoked); err != nil {
		return err
	}
	a.ApprovalID = strings.TrimSpace(a.ApprovalID)
	a.PrincipalID = strings.TrimSpace(a.PrincipalID)
	a.PrincipalApprovalID = strings.TrimSpace(a.PrincipalApprovalID)
	a.Provenance = normalizeSkillProvenance(a.Provenance)
	if err := validateSkillProvenance(a.Provenance); err != nil {
		return err
	}
	a.AssignedBy = strings.TrimSpace(a.AssignedBy)
	a.RevokedBy = strings.TrimSpace(a.RevokedBy)
	a.RevokedAt = normalizeSkillTimePtr(a.RevokedAt)
	a.RevokedReason = strings.TrimSpace(a.RevokedReason)
	a.applyTimestamps(isCreate)
	if a.AssignedAt.IsZero() {
		a.AssignedAt = a.CreatedAt
	}
	a.AssignedAt = a.AssignedAt.UTC()
	a.PK = SkillAssignmentPartitionKey(a.SubjectType, a.SubjectID)
	a.SK = SkillAssignmentSortKey(a.SkillID, a.ID)
	a.GSI1PK = "SKILL#" + a.SkillID + "#ASSIGNMENT"
	a.GSI1SK = fmt.Sprintf("SUBJECT#%s#%s#ASSIGNMENT#%s", a.SubjectType, a.SubjectID, a.ID)
	a.GSI2PK = "SKILL_ASSIGNMENT#STATUS#" + a.Status
	a.GSI2SK = fmt.Sprintf("UPDATED#%s#SUBJECT#%s#%s#SKILL#%s#REVISION#%08d#ASSIGNMENT#%s", formatSkillTime(a.UpdatedAt), a.SubjectType, a.SubjectID, a.SkillID, a.RevisionNumber, a.ID)
	return nil
}

func (r *SkillRevision) validateApprovalState() error {
	if r == nil {
		return fmt.Errorf("skill revision is required")
	}
	if r.ApprovalSignature != "" && r.ApprovalDigest == "" {
		return fmt.Errorf("approval digest is required when approval signature is present")
	}
	if r.ApprovalAuthorityType != "" {
		if err := validateSkillApprovalAuthority("ApprovalAuthorityType", r.ApprovalAuthorityType); err != nil {
			return err
		}
	}

	switch r.Status {
	case SkillRevisionStatusApproved:
		return r.validateApprovedState()
	case SkillRevisionStatusRevoked:
		if r.RevokedBy == "" {
			return fmt.Errorf("revoked by is required for revoked skill revision")
		}
		if r.RevokedAt == nil {
			return fmt.Errorf("revoked at is required for revoked skill revision")
		}
	}
	return nil
}

func (r *SkillRevision) validateApprovedState() error {
	required := []struct {
		name  string
		value string
	}{
		{"approvalID", r.ApprovalID},
		{"approvalAuthorityType", r.ApprovalAuthorityType},
		{"approvalAuthorityID", r.ApprovalAuthorityID},
		{"approvalDigest", r.ApprovalDigest},
		{"approvedBy", r.ApprovedBy},
		{"principalID", r.PrincipalID},
	}
	for _, field := range required {
		if strings.TrimSpace(field.value) == "" {
			return fmt.Errorf("%s is required for approved skill revision", field.name)
		}
	}
	if r.ApprovedAt == nil {
		return fmt.Errorf("approved at is required for approved skill revision")
	}
	return validateSkillApprovalAuthority("approvalAuthorityType", r.ApprovalAuthorityType)
}

func (s *Skill) applyTimestamps(isCreate bool) {
	applySkillTimestamps(&s.CreatedAt, &s.UpdatedAt, isCreate)
}

func (r *SkillRevision) applyTimestamps(isCreate bool) {
	applySkillTimestamps(&r.CreatedAt, &r.UpdatedAt, isCreate)
}

func (p *SkillProposal) applyTimestamps(isCreate bool) {
	applySkillTimestamps(&p.CreatedAt, &p.UpdatedAt, isCreate)
}

func (a *SkillAssignment) applyTimestamps(isCreate bool) {
	applySkillTimestamps(&a.CreatedAt, &a.UpdatedAt, isCreate)
}

func applySkillTimestamps(createdAt, updatedAt *time.Time, isCreate bool) {
	now := time.Now().UTC()
	if isCreate && createdAt.IsZero() {
		*createdAt = now
	}
	if createdAt.IsZero() {
		*createdAt = now
	}
	if !isCreate {
		if updatedAt.IsZero() {
			*updatedAt = now
		}
	} else if updatedAt.IsZero() {
		*updatedAt = *createdAt
	}
	*createdAt = createdAt.UTC()
	*updatedAt = updatedAt.UTC()
}

func normalizeSkillID(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func normalizeSkillSlug(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func normalizeSkillToken(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func normalizeSkillDigest(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func normalizeSkillStatus(value, fallback string) string {
	value = normalizeSkillToken(value)
	if value == "" {
		return fallback
	}
	return value
}

func normalizeSkillExposure(value, fallback string) string {
	value = normalizeSkillToken(value)
	switch value {
	case SkillExposurePublic, SkillExposureInstance, SkillExposurePrivate:
		return value
	case "":
		return fallback
	default:
		return value
	}
}

func normalizeSkillSourceType(value string) string {
	return normalizeSkillToken(value)
}

func normalizeSkillSubjectType(value string) string {
	return normalizeSkillToken(value)
}

func normalizeSkillApprovalAuthority(value string) string {
	return normalizeSkillToken(value)
}

func validateSkillStatus(field, value string, allowed ...string) error {
	return validateSkillEnum(field, value, allowed...)
}

func validateSkillExposure(field, value string) error {
	return validateSkillEnum(field, value, SkillExposurePublic, SkillExposureInstance, SkillExposurePrivate)
}

func validateSkillSourceType(field, value string, allowEmpty bool) error {
	if allowEmpty && strings.TrimSpace(value) == "" {
		return nil
	}
	return validateSkillEnum(field, value, SkillSourceTypeLocalFile, SkillSourceTypeHostConversation, SkillSourceTypeManual, SkillSourceTypeProposal, SkillSourceTypeApproval)
}

func validateSkillSubjectType(field, value string) error {
	return validateSkillEnum(field, value, SkillAssignmentSubjectInstance, SkillAssignmentSubjectActor, SkillAssignmentSubjectPrincipal)
}

func validateSkillApprovalAuthority(field, value string) error {
	return validateSkillEnum(field, value, SkillApprovalAuthorityAdmin, SkillApprovalAuthorityPrincipal, SkillApprovalAuthorityInstance)
}

func validateSkillEnum(field, value string, allowed ...string) error {
	value = strings.TrimSpace(value)
	for _, allowedValue := range allowed {
		if value == allowedValue {
			return nil
		}
	}
	return fmt.Errorf("%s has unsupported value %q", field, value)
}

func normalizeSkillStringSet(values []string, lower bool) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if lower {
			trimmed = strings.ToLower(trimmed)
		}
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	if len(out) == 0 {
		return nil
	}
	slices.Sort(out)
	return out
}

func normalizeSkillProvenance(values []SkillProvenanceRef) []SkillProvenanceRef {
	if len(values) == 0 {
		return nil
	}
	out := make([]SkillProvenanceRef, 0, len(values))
	for _, value := range values {
		value.SourceType = normalizeSkillSourceType(value.SourceType)
		value.SourceURI = strings.TrimSpace(value.SourceURI)
		value.Digest = normalizeSkillDigest(value.Digest)
		value.Ref = strings.TrimSpace(value.Ref)
		value.Notes = strings.TrimSpace(value.Notes)
		if value.SourceType == "" && value.SourceURI == "" && value.Digest == "" && value.Ref == "" && value.Notes == "" {
			continue
		}
		out = append(out, value)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func validateSkillProvenance(values []SkillProvenanceRef) error {
	for _, value := range values {
		if value.SourceType == "" {
			return fmt.Errorf("provenance source type is required")
		}
		if err := validateSkillSourceType("provenance.sourceType", value.SourceType, false); err != nil {
			return err
		}
	}
	return nil
}

func normalizeSkillRevisionFiles(values []SkillRevisionFile) []SkillRevisionFile {
	if len(values) == 0 {
		return nil
	}
	out := make([]SkillRevisionFile, 0, len(values))
	for _, value := range values {
		value.Path = strings.TrimSpace(value.Path)
		value.Digest = normalizeSkillDigest(value.Digest)
		value.ContentType = strings.TrimSpace(value.ContentType)
		value.Role = normalizeSkillToken(value.Role)
		if value.Path == "" && value.Digest == "" {
			continue
		}
		out = append(out, value)
	}
	if len(out) == 0 {
		return nil
	}
	slices.SortFunc(out, func(a, b SkillRevisionFile) int {
		return strings.Compare(a.Path, b.Path)
	})
	return out
}

func normalizeSkillTimePtr(value *time.Time) *time.Time {
	if value == nil || value.IsZero() {
		return nil
	}
	t := value.UTC()
	return &t
}

func formatSkillTime(value time.Time) string {
	if value.IsZero() {
		value = time.Now().UTC()
	}
	return value.UTC().Format(time.RFC3339Nano)
}

func defaultString(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSkillAuthorityKeyHelpers(t *testing.T) {
	t.Parallel()

	require.Equal(t, "SKILL#skill-a", SkillPartitionKey(" Skill-A "))
	require.Equal(t, "", SkillPartitionKey(" "))
	require.Equal(t, "REVISION#00000042", SkillRevisionSortKey(42))
	require.Equal(t, "", SkillRevisionSortKey(0))
	require.Equal(t, "SKILL_PROPOSAL#proposal-1", SkillProposalPartitionKey(" Proposal-1 "))
	require.Equal(t, "", SkillProposalPartitionKey(" "))
	require.Equal(t, "SKILL_ASSIGNMENT#actor#alice", SkillAssignmentPartitionKey(" Actor ", " Alice "))
	require.Equal(t, "", SkillAssignmentPartitionKey("actor", " "))
	require.Equal(t, "SKILL#skill-a#ASSIGNMENT#assign-1", SkillAssignmentSortKey(" Skill-A ", " Assign-1 "))
	require.Equal(t, "", SkillAssignmentSortKey("skill-a", " "))
}

func TestSkill_UpdateKeysNormalizesAuthorityMetadata(t *testing.T) {
	t.Parallel()

	created := time.Date(2026, 5, 13, 3, 30, 0, 0, time.UTC)
	skill := &Skill{
		ID:              " Skill-A ",
		Slug:            " Skill-Alias ",
		Name:            "  Skill Alpha  ",
		Status:          " Active ",
		DefaultExposure: " Instance ",
		Capabilities:    []string{"write", "read", "write", "  "},
		Tags:            []string{"Governance", "governance", " skills "},
		Provenance: []SkillProvenanceRef{{
			SourceType: " Local_File ",
			SourceURI:  " .codex/skills/example/SKILL.md ",
			Digest:     " SHA256:ABC ",
		}},
		CreatedAt: created,
		UpdatedAt: created,
	}

	require.NoError(t, skill.UpdateKeys())
	require.Equal(t, "SKILL#skill-a", skill.PK)
	require.Equal(t, SKSkill, skill.SK)
	require.Equal(t, "skill-a", skill.ID)
	require.Equal(t, "skill-alias", skill.Slug)
	require.Equal(t, "Skill Alpha", skill.Name)
	require.Equal(t, SkillStatusActive, skill.Status)
	require.Equal(t, SkillExposureInstance, skill.DefaultExposure)
	require.Equal(t, []string{"read", "write"}, skill.Capabilities)
	require.Equal(t, []string{"governance", "skills"}, skill.Tags)
	require.Equal(t, "SKILL#STATUS#active", skill.GSI1PK)
	require.Contains(t, skill.GSI1SK, "#SKILL#skill-a")
	require.Equal(t, "SKILL#EXPOSURE#instance", skill.GSI2PK)
	require.Equal(t, "NAME#skill-alias#SKILL#skill-a", skill.GSI2SK)
	require.Equal(t, SkillSourceTypeLocalFile, skill.Provenance[0].SourceType)
	require.Equal(t, "sha256:abc", skill.Provenance[0].Digest)
	require.Equal(t, MainTableName, skill.TableName())
	require.Equal(t, skill.PK, skill.GetPK())
	require.Equal(t, skill.SK, skill.GetSK())
}

func TestSkillRevision_UpdateKeysCapturesDigestAndRevisionBoundary(t *testing.T) {
	t.Parallel()

	approved := time.Date(2026, 5, 13, 4, 0, 0, 0, time.UTC)
	revision := &SkillRevision{
		ID:                    "skill-a-r7",
		SkillID:               " Skill-A ",
		RevisionNumber:        7,
		Status:                " Approved ",
		ManifestDigest:        " SHA256:ABCDEF ",
		BundleDigest:          " SHA256:BUNDLE ",
		ApprovalID:            " approval-1 ",
		ApprovalAuthorityType: " Admin ",
		ApprovalAuthorityID:   " ops ",
		ApprovalDigest:        " SHA256:APPROVAL ",
		ApprovedBy:            " ops ",
		PrincipalID:           " principal-1 ",
		Files: []SkillRevisionFile{
			{Path: " skill.json ", Digest: " SHA256:2 ", Role: " Manifest "},
			{Path: " SKILL.md ", Digest: " SHA256:1 ", Role: " Primary "},
		},
		Capabilities:    []string{"memory.read", "memory.read", "social.post"},
		DefaultExposure: " Public ",
		ApprovedAt:      &approved,
	}
	approvalDigest, err := SkillRevisionApprovalDigest(revision, revision.PrincipalID, revision.ApprovalAuthorityType, revision.ApprovalAuthorityID)
	require.NoError(t, err)
	revision.ApprovalDigest = approvalDigest

	require.NoError(t, revision.UpdateKeys())
	require.Equal(t, "SKILL#skill-a", revision.PK)
	require.Equal(t, "REVISION#00000007", revision.SK)
	require.Equal(t, "skill-a-r7", revision.ID)
	require.Equal(t, SkillRevisionStatusApproved, revision.Status)
	require.Equal(t, "sha256:abcdef", revision.ManifestDigest)
	require.Equal(t, SkillExposurePublic, revision.DefaultExposure)
	require.Equal(t, SkillApprovalAuthorityAdmin, revision.ApprovalAuthorityType)
	require.Equal(t, approvalDigest, revision.ApprovalDigest)
	require.Equal(t, "SKILL_REVISION#STATUS#approved", revision.GSI1PK)
	require.Contains(t, revision.GSI1SK, "#SKILL#skill-a#REVISION#00000007")
	require.Equal(t, "SKILL_REVISION_DIGEST#sha256:abcdef", revision.GSI2PK)
	require.Equal(t, "SKILL#skill-a#REVISION#00000007", revision.GSI2SK)
	require.Equal(t, "SKILL.md", revision.Files[0].Path)
	require.Equal(t, "skill.json", revision.Files[1].Path)
	require.Equal(t, []string{"memory.read", "social.post"}, revision.Capabilities)
	require.NotNil(t, revision.ApprovedAt)
	require.Equal(t, approved, *revision.ApprovedAt)
	require.Equal(t, MainTableName, revision.TableName())
}

func TestSkillProposal_UpdateKeysKeepsSeedSourceNonCanonical(t *testing.T) {
	t.Parallel()

	reviewed := time.Date(2026, 5, 13, 5, 0, 0, 0, time.UTC)
	proposal := &SkillProposal{
		ID:                     " Proposal-1 ",
		SkillID:                " Skill-A ",
		Status:                 " Proposed ",
		RequestedExposure:      " Private ",
		ProposedRevisionNumber: 2,
		SourceType:             " Local_File ",
		SourceURI:              " .codex/skills/example/SKILL.md ",
		SourceDigest:           " SHA256:ABC ",
		ConversationID:         " conv-1 ",
		PrincipalApprovalID:    " approval-1 ",
		ReviewedAt:             &reviewed,
	}

	require.NoError(t, proposal.UpdateKeys())
	require.Equal(t, "SKILL_PROPOSAL#proposal-1", proposal.PK)
	require.Equal(t, SKSkillProposal, proposal.SK)
	require.Equal(t, SkillProposalStatusProposed, proposal.Status)
	require.Equal(t, SkillSourceTypeLocalFile, proposal.SourceType)
	require.Equal(t, "sha256:abc", proposal.SourceDigest)
	require.Equal(t, "SKILL#skill-a#PROPOSAL", proposal.GSI1PK)
	require.Contains(t, proposal.GSI1SK, "STATUS#proposed#")
	require.Contains(t, proposal.GSI1SK, "#PROPOSAL#proposal-1")
	require.Equal(t, "SKILL_PROPOSAL#STATUS#proposed", proposal.GSI2PK)
	require.Contains(t, proposal.GSI2SK, "#SKILL#skill-a#PROPOSAL#proposal-1")
	require.NotNil(t, proposal.ReviewedAt)
	require.Equal(t, reviewed, *proposal.ReviewedAt)
	require.Equal(t, MainTableName, proposal.TableName())
}

func TestSkillProposal_UpdateKeysValidatesPromotionMetadata(t *testing.T) {
	t.Parallel()

	promotedAt := time.Date(2026, 5, 13, 8, 0, 0, 0, time.UTC)
	proposal := &SkillProposal{
		ID:                     " Proposal-1 ",
		SkillID:                " Skill-A ",
		Status:                 SkillProposalStatusAccepted,
		RequestedExposure:      SkillExposurePrivate,
		ProposedRevisionNumber: 2,
		PromotedRevisionID:     " Skill-A-R2 ",
		PromotedRevisionNumber: 2,
		PromotionDigest:        " SHA256:PROMOTION ",
		PromotedBy:             " ops ",
		PromotedAt:             &promotedAt,
	}

	require.NoError(t, proposal.UpdateKeys())
	require.Equal(t, "skill-a-r2", proposal.PromotedRevisionID)
	require.Equal(t, "sha256:promotion", proposal.PromotionDigest)
	require.Equal(t, "ops", proposal.PromotedBy)
	require.NotNil(t, proposal.PromotedAt)

	proposal.Status = SkillProposalStatusProposed
	require.ErrorContains(t, proposal.UpdateKeys(), "promoted skill proposal must be accepted")
}

func TestSkillAssignment_UpdateKeysDefinesSubjectBoundary(t *testing.T) {
	t.Parallel()

	revoked := time.Date(2026, 5, 13, 6, 0, 0, 0, time.UTC)
	assignment := &SkillAssignment{
		ID:                  " Assignment-1 ",
		SkillID:             " Skill-A ",
		RevisionNumber:      3,
		SubjectType:         " Actor ",
		SubjectID:           " Alice ",
		Exposure:            " Instance ",
		Status:              " Pending ",
		PrincipalApprovalID: " approval-1 ",
		RevokedAt:           &revoked,
	}

	require.NoError(t, assignment.UpdateKeys())
	require.Equal(t, "SKILL_ASSIGNMENT#actor#alice", assignment.PK)
	require.Equal(t, "SKILL#skill-a#ASSIGNMENT#assignment-1", assignment.SK)
	require.Equal(t, SkillAssignmentSubjectActor, assignment.SubjectType)
	require.Equal(t, "alice", assignment.SubjectID)
	require.Equal(t, SkillExposureInstance, assignment.Exposure)
	require.Equal(t, SkillAssignmentStatusPending, assignment.Status)
	require.Equal(t, "SKILL#skill-a#ASSIGNMENT", assignment.GSI1PK)
	require.Equal(t, "SUBJECT#actor#alice#ASSIGNMENT#assignment-1", assignment.GSI1SK)
	require.Equal(t, "SKILL_ASSIGNMENT#STATUS#pending", assignment.GSI2PK)
	require.Contains(t, assignment.GSI2SK, "#SUBJECT#actor#alice#SKILL#skill-a#REVISION#00000003#ASSIGNMENT#assignment-1")
	require.False(t, assignment.AssignedAt.IsZero())
	require.NotNil(t, assignment.RevokedAt)
	require.Equal(t, revoked, *assignment.RevokedAt)
	require.Equal(t, MainTableName, assignment.TableName())
}

func TestSkillAuthorityModels_RequireIdentifiers(t *testing.T) {
	t.Parallel()

	require.Error(t, (&Skill{}).UpdateKeys())
	require.Error(t, (&SkillRevision{SkillID: "skill-a"}).UpdateKeys())
	require.Error(t, (&SkillProposal{ID: "proposal-1"}).UpdateKeys())
	require.Error(t, (&SkillAssignment{ID: "assignment-1", SkillID: "skill-a", SubjectType: "actor"}).UpdateKeys())
}

func TestSkillAuthorityModels_FailClosedVocabularies(t *testing.T) {
	t.Parallel()

	require.Error(t, (&Skill{ID: "skill-a", Status: "mystery"}).UpdateKeys())
	require.Error(t, (&Skill{ID: "skill-a", DefaultExposure: "friends"}).UpdateKeys())
	require.Error(t, (&SkillRevision{SkillID: "skill-a", RevisionNumber: 1, Status: "reviewing"}).UpdateKeys())
	require.Error(t, (&SkillProposal{ID: "proposal-1", SkillID: "skill-a", SourceType: "host"}).UpdateKeys())
	require.Error(t, (&SkillAssignment{ID: "assignment-1", SkillID: "skill-a", SubjectType: "group", SubjectID: "alice"}).UpdateKeys())
	require.Error(t, (&Skill{
		ID: "skill-a",
		Provenance: []SkillProvenanceRef{{
			SourceURI: ".codex/skills/example/SKILL.md",
		}},
	}).UpdateKeys())
}

func TestSkillRevisionApprovalDigestIsStableAndAuthorityScoped(t *testing.T) {
	t.Parallel()

	revision := &SkillRevision{
		ID:              "skill-a-r1",
		SkillID:         "skill-a",
		RevisionNumber:  1,
		ManifestDigest:  "sha256:manifest",
		ContentDigest:   "sha256:content",
		BundleDigest:    "sha256:bundle",
		DefaultExposure: SkillExposurePrivate,
	}

	first, err := SkillRevisionApprovalDigest(revision, "principal-1", SkillApprovalAuthorityAdmin, "ops")
	require.NoError(t, err)
	second, err := SkillRevisionApprovalDigest(revision, "principal-1", SkillApprovalAuthorityAdmin, "ops")
	require.NoError(t, err)
	require.Equal(t, first, second)
	require.Contains(t, first, "sha256:")

	other, err := SkillRevisionApprovalDigest(revision, "principal-1", SkillApprovalAuthorityPrincipal, "principal-1")
	require.NoError(t, err)
	require.NotEqual(t, first, other)

	revision.DefaultExposure = SkillExposurePublic
	publicDigest, err := SkillRevisionApprovalDigest(revision, "principal-1", SkillApprovalAuthorityAdmin, "ops")
	require.NoError(t, err)
	require.NotEqual(t, first, publicDigest)

	_, err = SkillRevisionApprovalDigest(revision, "", SkillApprovalAuthorityAdmin, "ops")
	require.Error(t, err)
	_, err = SkillRevisionApprovalDigest(revision, "principal-1", "unknown", "ops")
	require.Error(t, err)
}

func TestSkillPromotionDigestBindsProposalAndRevision(t *testing.T) {
	t.Parallel()

	proposal := &SkillProposal{
		ID:                     "proposal-1",
		SkillID:                "skill-a",
		ProposedManifestDigest: "sha256:manifest",
		SourceDigest:           "sha256:source",
		ConversationID:         "conv-1",
		ConversationMessageID:  "msg-1",
	}
	revision := &SkillRevision{
		ID:              "skill-a-r1",
		SkillID:         "skill-a",
		RevisionNumber:  1,
		ManifestDigest:  "sha256:manifest",
		ApprovalDigest:  "sha256:approval",
		DefaultExposure: SkillExposurePrivate,
	}

	first, err := SkillPromotionDigest(proposal, revision)
	require.NoError(t, err)
	second, err := SkillPromotionDigest(proposal, revision)
	require.NoError(t, err)
	require.Equal(t, first, second)
	require.Contains(t, first, "sha256:")

	revision.ApprovalDigest = "sha256:other"
	other, err := SkillPromotionDigest(proposal, revision)
	require.NoError(t, err)
	require.NotEqual(t, first, other)

	revision.SkillID = "skill-b"
	_, err = SkillPromotionDigest(proposal, revision)
	require.Error(t, err)
}

func TestSkillRevisionApprovedStateRequiresCurrentApprovalDigest(t *testing.T) {
	t.Parallel()

	approved := time.Date(2026, 5, 13, 7, 0, 0, 0, time.UTC)
	revision := &SkillRevision{
		ID:                    "skill-a-r1",
		SkillID:               "skill-a",
		RevisionNumber:        1,
		Status:                SkillRevisionStatusApproved,
		DefaultExposure:       SkillExposurePrivate,
		ApprovalID:            "approval-1",
		ApprovalAuthorityType: SkillApprovalAuthorityAdmin,
		ApprovalAuthorityID:   "ops",
		ApprovedBy:            "ops",
		ApprovedAt:            &approved,
		PrincipalID:           "principal-1",
	}
	digest, err := SkillRevisionApprovalDigest(revision, revision.PrincipalID, revision.ApprovalAuthorityType, revision.ApprovalAuthorityID)
	require.NoError(t, err)
	revision.ApprovalDigest = digest
	require.NoError(t, revision.UpdateKeys())

	revision.DefaultExposure = SkillExposurePublic
	require.ErrorContains(t, revision.UpdateKeys(), "approval digest does not match")
}

func TestSkillAuthorityLifecycleHooksAndNilAccessors(t *testing.T) {
	t.Parallel()

	skill := &Skill{ID: " Skill-A "}
	require.NoError(t, skill.BeforeCreate())
	require.Equal(t, "SKILL#skill-a", skill.GetPK())
	require.Equal(t, SKSkill, skill.GetSK())
	require.False(t, skill.CreatedAt.IsZero())
	require.False(t, skill.UpdatedAt.IsZero())
	require.NoError(t, skill.BeforeUpdate())

	revision := &SkillRevision{SkillID: " Skill-A ", RevisionNumber: 1}
	require.NoError(t, revision.BeforeCreate())
	require.Equal(t, "SKILL#skill-a", revision.GetPK())
	require.Equal(t, "REVISION#00000001", revision.GetSK())
	require.NoError(t, revision.BeforeUpdate())

	proposal := &SkillProposal{ID: " Proposal-1 ", SkillID: " Skill-A "}
	require.NoError(t, proposal.BeforeCreate())
	require.Equal(t, "SKILL_PROPOSAL#proposal-1", proposal.GetPK())
	require.Equal(t, SKSkillProposal, proposal.GetSK())
	require.NoError(t, proposal.BeforeUpdate())

	assignment := &SkillAssignment{
		ID:          " Assignment-1 ",
		SkillID:     " Skill-A ",
		SubjectType: SkillAssignmentSubjectInstance,
		SubjectID:   " Local ",
	}
	require.NoError(t, assignment.BeforeCreate())
	require.Equal(t, "SKILL_ASSIGNMENT#instance#local", assignment.GetPK())
	require.Equal(t, "SKILL#skill-a#ASSIGNMENT#assignment-1", assignment.GetSK())
	require.False(t, assignment.AssignedAt.IsZero())
	require.NoError(t, assignment.BeforeUpdate())

	var nilSkill *Skill
	require.Empty(t, nilSkill.GetPK())
	require.Empty(t, nilSkill.GetSK())
	var nilRevision *SkillRevision
	require.Empty(t, nilRevision.GetPK())
	require.Empty(t, nilRevision.GetSK())
	var nilProposal *SkillProposal
	require.Empty(t, nilProposal.GetPK())
	require.Empty(t, nilProposal.GetSK())
	var nilAssignment *SkillAssignment
	require.Empty(t, nilAssignment.GetPK())
	require.Empty(t, nilAssignment.GetSK())
}

func TestSkillAuthorityDigestValidationErrors(t *testing.T) {
	t.Parallel()

	validRevision := &SkillRevision{
		ID:              "skill-a-r1",
		SkillID:         "skill-a",
		RevisionNumber:  1,
		ManifestDigest:  "sha256:manifest",
		ContentDigest:   "sha256:content",
		BundleDigest:    "sha256:bundle",
		DefaultExposure: SkillExposurePrivate,
	}

	_, err := SkillRevisionApprovalDigest(nil, "principal-1", SkillApprovalAuthorityAdmin, "ops")
	require.ErrorContains(t, err, "skill revision is required")
	_, err = SkillRevisionApprovalDigest(&SkillRevision{RevisionNumber: 1}, "principal-1", SkillApprovalAuthorityAdmin, "ops")
	require.Error(t, err)
	_, err = SkillRevisionApprovalDigest(&SkillRevision{SkillID: "skill-a"}, "principal-1", SkillApprovalAuthorityAdmin, "ops")
	require.ErrorContains(t, err, "revision number is required")
	_, err = SkillRevisionApprovalDigest(validRevision, "principal-1", SkillApprovalAuthorityAdmin, " ")
	require.Error(t, err)
	badExposure := *validRevision
	badExposure.DefaultExposure = "friends"
	_, err = SkillRevisionApprovalDigest(&badExposure, "principal-1", SkillApprovalAuthorityAdmin, "ops")
	require.Error(t, err)

	proposal := &SkillProposal{ID: "proposal-1", SkillID: "skill-a", ProposedManifestDigest: "sha256:manifest"}
	revision := &SkillRevision{ID: "skill-a-r1", SkillID: "skill-a", RevisionNumber: 1, ManifestDigest: "sha256:manifest", ApprovalDigest: "sha256:approval"}
	_, err = SkillPromotionDigest(nil, revision)
	require.ErrorContains(t, err, "skill proposal is required")
	_, err = SkillPromotionDigest(proposal, nil)
	require.ErrorContains(t, err, "skill revision is required")
	_, err = SkillPromotionDigest(&SkillProposal{SkillID: "skill-a"}, revision)
	require.Error(t, err)
	_, err = SkillPromotionDigest(&SkillProposal{ID: "proposal-1"}, revision)
	require.Error(t, err)
	_, err = SkillPromotionDigest(proposal, &SkillRevision{ID: "skill-a-r1", RevisionNumber: 1})
	require.Error(t, err)
	_, err = SkillPromotionDigest(proposal, &SkillRevision{ID: "skill-a-r1", SkillID: "skill-a"})
	require.ErrorContains(t, err, "revision number is required")
	_, err = SkillPromotionDigest(proposal, &SkillRevision{SkillID: "skill-a", RevisionNumber: 1, ManifestDigest: "sha256:manifest", ApprovalDigest: "sha256:approval"})
	require.ErrorContains(t, err, "revision id is required")
	_, err = SkillPromotionDigest(proposal, &SkillRevision{ID: "skill-a-r1", SkillID: "skill-a", RevisionNumber: 1, ApprovalDigest: "sha256:approval"})
	require.ErrorContains(t, err, "manifest digest is required")
	_, err = SkillPromotionDigest(proposal, &SkillRevision{ID: "skill-a-r1", SkillID: "skill-a", RevisionNumber: 1, ManifestDigest: "sha256:manifest"})
	require.ErrorContains(t, err, "approval digest is required")
}

func TestSkillProposalPromotionStateRequiresCompleteAcceptedPromotion(t *testing.T) {
	t.Parallel()

	promotedAt := time.Date(2026, 7, 12, 12, 0, 0, 0, time.UTC)
	base := SkillProposal{
		ID:                "proposal-1",
		SkillID:           "skill-a",
		Status:            SkillProposalStatusAccepted,
		RequestedExposure: SkillExposurePrivate,
	}

	proposal := base
	proposal.PromotedRevisionNumber = 1
	require.ErrorContains(t, proposal.UpdateKeys(), "promotedRevisionID is required")

	proposal = base
	proposal.PromotedRevisionID = "skill-a-r1"
	proposal.PromotedBy = "ops"
	proposal.PromotedAt = &promotedAt
	require.ErrorContains(t, proposal.UpdateKeys(), "promotionDigest is required")

	proposal = base
	proposal.PromotedRevisionID = "skill-a-r1"
	proposal.PromotionDigest = "sha256:promotion"
	proposal.PromotedAt = &promotedAt
	require.ErrorContains(t, proposal.UpdateKeys(), "promotedBy is required")

	proposal = base
	proposal.PromotedRevisionID = "skill-a-r1"
	proposal.PromotionDigest = "sha256:promotion"
	proposal.PromotedBy = "ops"
	proposal.PromotedAt = &promotedAt
	require.ErrorContains(t, proposal.UpdateKeys(), "promoted revision number is required")

	proposal = base
	proposal.PromotedRevisionID = "skill-a-r1"
	proposal.PromotedRevisionNumber = 1
	proposal.PromotionDigest = "sha256:promotion"
	proposal.PromotedBy = "ops"
	require.ErrorContains(t, proposal.UpdateKeys(), "promoted at is required")
}

func TestSkillRevisionApprovalStateRejectsIncompleteApprovedAndRevoked(t *testing.T) {
	t.Parallel()

	revision := &SkillRevision{
		SkillID:           "skill-a",
		RevisionNumber:    1,
		ApprovalSignature: "sig",
	}
	require.ErrorContains(t, revision.UpdateKeys(), "approval digest is required when approval signature is present")

	revision = &SkillRevision{
		SkillID:               "skill-a",
		RevisionNumber:        1,
		ApprovalAuthorityType: "unknown",
	}
	require.Error(t, revision.UpdateKeys())

	revision = &SkillRevision{SkillID: "skill-a", RevisionNumber: 1, Status: SkillRevisionStatusRevoked}
	require.ErrorContains(t, revision.UpdateKeys(), "revoked by is required")

	revision.RevokedBy = "ops"
	require.ErrorContains(t, revision.UpdateKeys(), "revoked at is required")

	revision = &SkillRevision{SkillID: "skill-a", RevisionNumber: 1, Status: SkillRevisionStatusApproved}
	require.ErrorContains(t, revision.UpdateKeys(), "approvalID is required")

	approvedAt := time.Date(2026, 7, 12, 13, 0, 0, 0, time.UTC)
	revision = &SkillRevision{
		ID:                    "skill-a-r1",
		SkillID:               "skill-a",
		RevisionNumber:        1,
		Status:                SkillRevisionStatusApproved,
		DefaultExposure:       SkillExposurePrivate,
		ApprovalID:            "approval-1",
		ApprovalAuthorityType: SkillApprovalAuthorityAdmin,
		ApprovalAuthorityID:   "ops",
		ApprovedBy:            "ops",
		PrincipalID:           "principal-1",
		ApprovedAt:            &approvedAt,
	}
	digest, err := SkillRevisionApprovalDigest(revision, revision.PrincipalID, revision.ApprovalAuthorityType, revision.ApprovalAuthorityID)
	require.NoError(t, err)
	revision.ApprovalDigest = digest
	revision.ApprovedAt = nil
	require.ErrorContains(t, revision.UpdateKeys(), "approved at is required")
}

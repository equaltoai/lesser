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

	require.NoError(t, revision.UpdateKeys())
	require.Equal(t, "SKILL#skill-a", revision.PK)
	require.Equal(t, "REVISION#00000007", revision.SK)
	require.Equal(t, "skill-a-r7", revision.ID)
	require.Equal(t, SkillRevisionStatusApproved, revision.Status)
	require.Equal(t, "sha256:abcdef", revision.ManifestDigest)
	require.Equal(t, SkillExposurePublic, revision.DefaultExposure)
	require.Equal(t, SkillApprovalAuthorityAdmin, revision.ApprovalAuthorityType)
	require.Equal(t, "sha256:approval", revision.ApprovalDigest)
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
		ID:             "skill-a-r1",
		SkillID:        "skill-a",
		RevisionNumber: 1,
		ManifestDigest: "sha256:manifest",
		ContentDigest:  "sha256:content",
		BundleDigest:   "sha256:bundle",
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

	_, err = SkillRevisionApprovalDigest(revision, "", SkillApprovalAuthorityAdmin, "ops")
	require.Error(t, err)
	_, err = SkillRevisionApprovalDigest(revision, "principal-1", "unknown", "ops")
	require.Error(t, err)
}

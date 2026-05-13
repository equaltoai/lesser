// Package interfaces defines the repository interfaces for the Lesser application.
package interfaces //nolint:revive // Standard interfaces package name

import (
	"context"

	"github.com/equaltoai/lesser/pkg/storage/models"
)

// SkillRepository defines canonical skill authority storage operations.
// Local SKILL.md files and lesser-host conversations may feed proposals or provenance,
// but these records are the Lesser-owned system of record.
type SkillRepository interface {
	// Skill root operations.
	CreateSkill(ctx context.Context, skill *models.Skill) error
	GetSkill(ctx context.Context, skillID string) (*models.Skill, error)
	UpdateSkill(ctx context.Context, skill *models.Skill) error
	ListSkillsByStatus(ctx context.Context, status string, limit int, cursor string) ([]*models.Skill, string, error)
	ListSkillsByExposure(ctx context.Context, exposure string, limit int, cursor string) ([]*models.Skill, string, error)

	// Revision operations.
	CreateSkillRevision(ctx context.Context, revision *models.SkillRevision) error
	GetSkillRevision(ctx context.Context, skillID string, revisionNumber int) (*models.SkillRevision, error)
	UpdateSkillRevision(ctx context.Context, revision *models.SkillRevision) error
	ListSkillRevisions(ctx context.Context, skillID string, limit int, cursor string) ([]*models.SkillRevision, string, error)
	GetSkillRevisionByDigest(ctx context.Context, manifestDigest string) (*models.SkillRevision, error)

	// Proposal operations.
	CreateSkillProposal(ctx context.Context, proposal *models.SkillProposal) error
	GetSkillProposal(ctx context.Context, proposalID string) (*models.SkillProposal, error)
	UpdateSkillProposal(ctx context.Context, proposal *models.SkillProposal) error
	ListSkillProposalsForSkill(ctx context.Context, skillID string, limit int, cursor string) ([]*models.SkillProposal, string, error)
	ListSkillProposalsByStatus(ctx context.Context, status string, limit int, cursor string) ([]*models.SkillProposal, string, error)

	// Assignment operations.
	CreateSkillAssignment(ctx context.Context, assignment *models.SkillAssignment) error
	GetSkillAssignment(ctx context.Context, subjectType, subjectID, skillID, assignmentID string) (*models.SkillAssignment, error)
	UpdateSkillAssignment(ctx context.Context, assignment *models.SkillAssignment) error
	ListSkillAssignmentsForSubject(ctx context.Context, subjectType, subjectID string, limit int, cursor string) ([]*models.SkillAssignment, string, error)
	ListSkillAssignmentsForSkill(ctx context.Context, skillID string, limit int, cursor string) ([]*models.SkillAssignment, string, error)
}

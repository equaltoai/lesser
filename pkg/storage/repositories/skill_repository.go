package repositories

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/cost"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/theory-cloud/tabletheory/v2/pkg/core"
	"go.uber.org/zap"
)

const (
	defaultSkillQueryLimit = 25
	maxSkillQueryLimit     = 100
)

// SkillRepository implements canonical skill authority storage operations.
type SkillRepository struct {
	db             core.DB
	skillRepo      *EnhancedBaseRepository[*models.Skill]
	revisionRepo   *EnhancedBaseRepository[*models.SkillRevision]
	proposalRepo   *EnhancedBaseRepository[*models.SkillProposal]
	assignmentRepo *EnhancedBaseRepository[*models.SkillAssignment]
}

// NewSkillRepository creates a repository for canonical skill authority records.
func NewSkillRepository(db core.DB, tableName string, logger *zap.Logger, costService *cost.TrackingService) *SkillRepository {
	return &SkillRepository{
		db:             db,
		skillRepo:      newSkillEntityRepository[*models.Skill](db, tableName, logger, costService, "SkillRepository", "skill"),
		revisionRepo:   newSkillEntityRepository[*models.SkillRevision](db, tableName, logger, costService, "SkillRevisionRepository", "skill_revision"),
		proposalRepo:   newSkillEntityRepository[*models.SkillProposal](db, tableName, logger, costService, "SkillProposalRepository", "skill_proposal"),
		assignmentRepo: newSkillEntityRepository[*models.SkillAssignment](db, tableName, logger, costService, "SkillAssignmentRepository", "skill_assignment"),
	}
}

func newSkillEntityRepository[T BaseModel](db core.DB, tableName string, logger *zap.Logger, costService *cost.TrackingService, repoName, entityName string) *EnhancedBaseRepository[T] {
	repo := NewEnhancedBaseRepository[T](db, tableName, logger, costService, repoName, entityName)
	repo.SetValidationService(NewDefaultValidationService())
	repo.SetPermissionService(NewDefaultPermissionService())
	repo.SetCachingService(NewInMemoryCachingService())
	repo.SetEventService(NewDefaultEventService())
	return repo
}

// CreateSkill creates a canonical skill root.
func (r *SkillRepository) CreateSkill(ctx context.Context, skill *models.Skill) error {
	return r.skillRepo.ValidateAndCreate(ctx, skill)
}

// GetSkill retrieves a canonical skill root.
func (r *SkillRepository) GetSkill(ctx context.Context, skillID string) (*models.Skill, error) {
	var skill models.Skill
	if err := r.skillRepo.Get(ctx, models.SkillPartitionKey(skillID), models.SKSkill, &skill); err != nil {
		return nil, err
	}
	return &skill, nil
}

// UpdateSkill updates a canonical skill root.
func (r *SkillRepository) UpdateSkill(ctx context.Context, skill *models.Skill) error {
	return r.skillRepo.ValidateAndUpdate(ctx, skill)
}

// ListSkillsByStatus lists canonical skills by lifecycle status.
func (r *SkillRepository) ListSkillsByStatus(ctx context.Context, status string, limit int, cursor string) ([]*models.Skill, string, error) {
	status = strings.ToLower(strings.TrimSpace(status))
	if status == "" {
		status = models.SkillStatusActive
	}
	return querySkillGSIPage[*models.Skill](ctx, r.db, &models.Skill{}, models.IndexGSI1, "gsi1PK", gsi1SKField, "SKILL#STATUS#"+status, limit, cursor)
}

// ListSkillsByExposure lists canonical skills by their default exposure class.
func (r *SkillRepository) ListSkillsByExposure(ctx context.Context, exposure string, limit int, cursor string) ([]*models.Skill, string, error) {
	exposure = strings.ToLower(strings.TrimSpace(exposure))
	if exposure == "" {
		exposure = models.SkillExposurePublic
	}
	return querySkillGSIPage[*models.Skill](ctx, r.db, &models.Skill{}, models.IndexGSI2, "gsi2PK", gsi2SKField, "SKILL#EXPOSURE#"+exposure, limit, cursor)
}

// CreateSkillRevision creates a canonical skill revision.
func (r *SkillRepository) CreateSkillRevision(ctx context.Context, revision *models.SkillRevision) error {
	return r.revisionRepo.ValidateAndCreate(ctx, revision)
}

// GetSkillRevision retrieves a canonical skill revision by skill and revision number.
func (r *SkillRepository) GetSkillRevision(ctx context.Context, skillID string, revisionNumber int) (*models.SkillRevision, error) {
	var revision models.SkillRevision
	if err := r.revisionRepo.Get(ctx, models.SkillPartitionKey(skillID), models.SkillRevisionSortKey(revisionNumber), &revision); err != nil {
		return nil, err
	}
	return &revision, nil
}

// UpdateSkillRevision updates a canonical skill revision.
func (r *SkillRepository) UpdateSkillRevision(ctx context.Context, revision *models.SkillRevision) error {
	return r.revisionRepo.ValidateAndUpdate(ctx, revision)
}

// ListSkillRevisions lists revisions for a canonical skill.
func (r *SkillRepository) ListSkillRevisions(ctx context.Context, skillID string, limit int, cursor string) ([]*models.SkillRevision, string, error) {
	skillID = strings.ToLower(strings.TrimSpace(skillID))
	if err := common.ValidateRequiredParam("skillID", skillID); err != nil {
		return nil, "", err
	}
	return listByPKSKPrefixPaginated[*models.SkillRevision](ctx, r.db, &models.SkillRevision{}, models.SkillPartitionKey(skillID), models.SKSkillRevisionPrefix, sanitizeSkillLimit(limit), cursor)
}

// ListSkillRevisionsByStatus lists canonical revisions by lifecycle status.
func (r *SkillRepository) ListSkillRevisionsByStatus(ctx context.Context, status string, limit int, cursor string) ([]*models.SkillRevision, string, error) {
	status = strings.ToLower(strings.TrimSpace(status))
	if status == "" {
		status = models.SkillRevisionStatusApproved
	}
	return querySkillGSIPage[*models.SkillRevision](ctx, r.db, &models.SkillRevision{}, models.IndexGSI1, "gsi1PK", gsi1SKField, "SKILL_REVISION#STATUS#"+status, limit, cursor)
}

// GetSkillRevisionByDigest resolves a revision by manifest digest when the digest index is populated.
func (r *SkillRepository) GetSkillRevisionByDigest(ctx context.Context, manifestDigest string) (*models.SkillRevision, error) {
	manifestDigest = strings.ToLower(strings.TrimSpace(manifestDigest))
	if err := common.ValidateRequiredParam("manifestDigest", manifestDigest); err != nil {
		return nil, err
	}
	items, _, err := querySkillGSIPage[*models.SkillRevision](ctx, r.db, &models.SkillRevision{}, models.IndexGSI2, "gsi2PK", gsi2SKField, "SKILL_REVISION_DIGEST#"+manifestDigest, 1, "")
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("skill revision digest not found: %s", manifestDigest)
	}
	return items[0], nil
}

// CreateSkillProposal creates a non-authoritative proposal for later canonicalization.
func (r *SkillRepository) CreateSkillProposal(ctx context.Context, proposal *models.SkillProposal) error {
	return r.proposalRepo.ValidateAndCreate(ctx, proposal)
}

// GetSkillProposal retrieves a skill proposal by ID.
func (r *SkillRepository) GetSkillProposal(ctx context.Context, proposalID string) (*models.SkillProposal, error) {
	var proposal models.SkillProposal
	if err := r.proposalRepo.Get(ctx, models.SkillProposalPartitionKey(proposalID), models.SKSkillProposal, &proposal); err != nil {
		return nil, err
	}
	return &proposal, nil
}

// UpdateSkillProposal updates a skill proposal.
func (r *SkillRepository) UpdateSkillProposal(ctx context.Context, proposal *models.SkillProposal) error {
	return r.proposalRepo.ValidateAndUpdate(ctx, proposal)
}

// ListSkillProposalsForSkill lists proposals associated with a canonical skill.
func (r *SkillRepository) ListSkillProposalsForSkill(ctx context.Context, skillID string, limit int, cursor string) ([]*models.SkillProposal, string, error) {
	skillID = strings.ToLower(strings.TrimSpace(skillID))
	if err := common.ValidateRequiredParam("skillID", skillID); err != nil {
		return nil, "", err
	}
	return querySkillGSIPage[*models.SkillProposal](ctx, r.db, &models.SkillProposal{}, models.IndexGSI1, "gsi1PK", gsi1SKField, "SKILL#"+skillID+"#PROPOSAL", limit, cursor)
}

// ListSkillProposalsByStatus lists proposals by lifecycle status for later review APIs.
func (r *SkillRepository) ListSkillProposalsByStatus(ctx context.Context, status string, limit int, cursor string) ([]*models.SkillProposal, string, error) {
	status = strings.ToLower(strings.TrimSpace(status))
	if status == "" {
		status = models.SkillProposalStatusProposed
	}
	return querySkillGSIPage[*models.SkillProposal](ctx, r.db, &models.SkillProposal{}, models.IndexGSI2, "gsi2PK", gsi2SKField, "SKILL_PROPOSAL#STATUS#"+status, limit, cursor)
}

// CreateSkillAssignment creates an effective-resolution assignment placeholder.
func (r *SkillRepository) CreateSkillAssignment(ctx context.Context, assignment *models.SkillAssignment) error {
	return r.assignmentRepo.ValidateAndCreate(ctx, assignment)
}

// GetSkillAssignment retrieves an assignment by subject, skill, and assignment ID.
func (r *SkillRepository) GetSkillAssignment(ctx context.Context, subjectType, subjectID, skillID, assignmentID string) (*models.SkillAssignment, error) {
	var assignment models.SkillAssignment
	if err := r.assignmentRepo.Get(ctx, models.SkillAssignmentPartitionKey(subjectType, subjectID), models.SkillAssignmentSortKey(skillID, assignmentID), &assignment); err != nil {
		return nil, err
	}
	return &assignment, nil
}

// UpdateSkillAssignment updates a skill assignment.
func (r *SkillRepository) UpdateSkillAssignment(ctx context.Context, assignment *models.SkillAssignment) error {
	return r.assignmentRepo.ValidateAndUpdate(ctx, assignment)
}

// ListSkillAssignmentsForSubject lists assignments for a subject boundary.
func (r *SkillRepository) ListSkillAssignmentsForSubject(ctx context.Context, subjectType, subjectID string, limit int, cursor string) ([]*models.SkillAssignment, string, error) {
	pk := models.SkillAssignmentPartitionKey(subjectType, subjectID)
	if err := common.ValidateRequiredParam("assignmentSubject", pk); err != nil {
		return nil, "", err
	}
	return listByPKSKPrefixPaginated[*models.SkillAssignment](ctx, r.db, &models.SkillAssignment{}, pk, models.SKSkillAssignmentPrefix, sanitizeSkillLimit(limit), cursor)
}

// ListSkillAssignmentsForSkill lists assignment references for a skill.
func (r *SkillRepository) ListSkillAssignmentsForSkill(ctx context.Context, skillID string, limit int, cursor string) ([]*models.SkillAssignment, string, error) {
	skillID = strings.ToLower(strings.TrimSpace(skillID))
	if err := common.ValidateRequiredParam("skillID", skillID); err != nil {
		return nil, "", err
	}
	return querySkillGSIPage[*models.SkillAssignment](ctx, r.db, &models.SkillAssignment{}, models.IndexGSI1, "gsi1PK", gsi1SKField, "SKILL#"+skillID+"#ASSIGNMENT", limit, cursor)
}

type skillGSIItem interface {
	GetSK() string
}

func querySkillGSIPage[T skillGSIItem](ctx context.Context, db core.DB, model any, indexName, pkAttr, skAttr, pk string, limit int, cursor string) ([]T, string, error) {
	if db == nil {
		return nil, "", fmt.Errorf("database client is nil")
	}
	limit = sanitizeSkillLimit(limit)
	query := db.WithContext(ctx).Model(model).
		Index(indexName).
		Where(pkAttr, "=", pk).
		OrderBy(skAttr, "ASC")

	cursor = strings.TrimSpace(cursor)
	if cursor != "" {
		query = query.Where(skAttr, ">", cursor)
	}

	query = query.Limit(limit + 1)

	var items []T
	if err := query.All(&items); err != nil {
		return nil, "", err
	}

	nextCursor := ""
	if len(items) > limit {
		nextCursor = skillGSIItemCursor(items[limit-1], skAttr)
		if nextCursor == "" {
			nextCursor = items[limit-1].GetSK()
		}
		items = items[:limit]
	}
	return items, nextCursor, nil
}

func skillGSIItemCursor(item any, attr string) string {
	value := reflect.ValueOf(item)
	if !value.IsValid() {
		return ""
	}
	if value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return ""
		}
		value = value.Elem()
	}
	if value.Kind() != reflect.Struct {
		return ""
	}
	fieldName := skillGSIFieldName(attr)
	if fieldName == "" {
		return ""
	}
	field := value.FieldByName(fieldName)
	if !field.IsValid() || field.Kind() != reflect.String {
		return ""
	}
	return field.String()
}

func skillGSIFieldName(attr string) string {
	switch attr {
	case gsi1SKField:
		return "GSI1SK"
	case gsi2SKField:
		return "GSI2SK"
	default:
		return ""
	}
}

func sanitizeSkillLimit(limit int) int {
	switch {
	case limit <= 0:
		return defaultSkillQueryLimit
	case limit > maxSkillQueryLimit:
		return maxSkillQueryLimit
	default:
		return limit
	}
}

var _ interfaces.SkillRepository = (*SkillRepository)(nil)

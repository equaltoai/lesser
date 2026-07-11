package repositories

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/v2/pkg/mocks"
	"go.uber.org/zap"
)

func TestSkillGSIItemCursorUsesCanonicalGSIFieldNames(t *testing.T) {
	t.Parallel()

	skill := &models.Skill{
		GSI1SK: "UPDATED#2026-05-13T00:00:00Z#SKILL#example",
		GSI2SK: "NAME#example#SKILL#example",
	}

	require.Equal(t, skill.GSI1SK, skillGSIItemCursor(skill, gsi1SKField))
	require.Equal(t, skill.GSI2SK, skillGSIItemCursor(skill, gsi2SKField))
	require.Empty(t, skillGSIItemCursor(skill, "gsi3SK"))
	require.Empty(t, skillGSIItemCursor((*models.Skill)(nil), gsi1SKField))
}

func TestSkillRepositoryNilDatabasePaths(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repo := NewSkillRepository(nil, "test-table", zap.NewNop(), nil)

	now := time.Date(2026, 5, 13, 8, 0, 0, 0, time.UTC)
	skill := &models.Skill{
		ID:              "skill-a",
		Name:            "Skill A",
		Status:          models.SkillStatusDraft,
		DefaultExposure: models.SkillExposurePrivate,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	require.Error(t, repo.CreateSkill(ctx, skill))
	require.Error(t, repo.UpdateSkill(ctx, skill))
	_, err := repo.GetSkill(ctx, "skill-a")
	require.Error(t, err)
	_, _, err = repo.ListSkillsByStatus(ctx, "", 0, "")
	require.Error(t, err)
	_, _, err = repo.ListSkillsByExposure(ctx, "", 0, "")
	require.Error(t, err)

	revision := &models.SkillRevision{
		ID:              "skill-a-r1",
		SkillID:         "skill-a",
		RevisionNumber:  1,
		Status:          models.SkillRevisionStatusDraft,
		DefaultExposure: models.SkillExposurePrivate,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	require.Error(t, repo.CreateSkillRevision(ctx, revision))
	require.Error(t, repo.UpdateSkillRevision(ctx, revision))
	_, err = repo.GetSkillRevision(ctx, "skill-a", 1)
	require.Error(t, err)
	_, err = repo.GetSkillRevisionByDigest(ctx, "sha256:abc")
	require.Error(t, err)
	_, err = repo.GetSkillRevisionByDigest(ctx, "")
	require.Error(t, err)
	_, _, err = repo.ListSkillRevisions(ctx, "", 0, "")
	require.Error(t, err)

	proposal := &models.SkillProposal{
		ID:                "proposal-1",
		SkillID:           "skill-a",
		Status:            models.SkillProposalStatusProposed,
		RequestedExposure: models.SkillExposurePrivate,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	require.Error(t, repo.CreateSkillProposal(ctx, proposal))
	require.Error(t, repo.UpdateSkillProposal(ctx, proposal))
	_, err = repo.GetSkillProposal(ctx, "proposal-1")
	require.Error(t, err)
	_, _, err = repo.ListSkillProposalsForSkill(ctx, "skill-a", 0, "")
	require.Error(t, err)
	_, _, err = repo.ListSkillProposalsForSkill(ctx, "", 0, "")
	require.Error(t, err)
	_, _, err = repo.ListSkillProposalsByStatus(ctx, "", 0, "")
	require.Error(t, err)

	assignment := &models.SkillAssignment{
		ID:                  "assignment-1",
		SkillID:             "skill-a",
		SubjectType:         models.SkillAssignmentSubjectActor,
		SubjectID:           "alice",
		Exposure:            models.SkillExposurePrivate,
		Status:              models.SkillAssignmentStatusActive,
		AssignedAt:          now,
		CreatedAt:           now,
		UpdatedAt:           now,
		RevisionNumber:      1,
		PrincipalID:         "principal-1",
		ApprovalID:          "approval-1",
		RevisionID:          "skill-a-r1",
		AssignedBy:          "operator",
		RevokedReason:       "",
		RevokedBy:           "",
		RevokedAt:           nil,
		PrincipalApprovalID: "principal-approval-1",
	}
	require.Error(t, repo.CreateSkillAssignment(ctx, assignment))
	require.Error(t, repo.UpdateSkillAssignment(ctx, assignment))
	_, err = repo.GetSkillAssignment(ctx, "actor", "alice", "skill-a", "assignment-1")
	require.Error(t, err)
	_, _, err = repo.ListSkillAssignmentsForSkill(ctx, "skill-a", 0, "")
	require.Error(t, err)
	_, _, err = repo.ListSkillAssignmentsForSkill(ctx, "", 0, "")
	require.Error(t, err)
	_, _, err = repo.ListSkillAssignmentsForSubject(ctx, "", "", 0, "")
	require.Error(t, err)
}

func TestQuerySkillGSIPagePagination(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
	mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
	mockQuery.On("Index", models.IndexGSI1).Return(mockQuery).Once()
	mockQuery.On("Where", "gsi1PK", "=", "SKILL#STATUS#active").Return(mockQuery).Once()
	mockQuery.On("OrderBy", gsi1SKField, "ASC").Return(mockQuery).Once()
	mockQuery.On("Where", gsi1SKField, ">", "cursor").Return(mockQuery).Once()
	mockQuery.On("Limit", 3).Return(mockQuery).Once()
	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		out := args.Get(0).(*[]*models.Skill)
		*out = []*models.Skill{
			{ID: "a", GSI1SK: "a"},
			{ID: "b", GSI1SK: "b"},
			{ID: "c", GSI1SK: "c"},
		}
	}).Return(nil).Once()

	items, cursor, err := querySkillGSIPage[*models.Skill](
		ctx,
		mockDB,
		&models.Skill{},
		models.IndexGSI1,
		"gsi1PK",
		gsi1SKField,
		"SKILL#STATUS#active",
		2,
		"cursor",
	)
	require.NoError(t, err)
	require.Len(t, items, 2)
	require.Equal(t, "b", cursor)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestQuerySkillGSIPageErrorsAndLimits(t *testing.T) {
	t.Parallel()

	_, _, err := querySkillGSIPage[*models.Skill](context.Background(), nil, &models.Skill{}, models.IndexGSI1, "gsi1PK", gsi1SKField, "pk", 1, "")
	require.Error(t, err)
	require.Equal(t, defaultSkillQueryLimit, sanitizeSkillLimit(0))
	require.Equal(t, defaultSkillQueryLimit, sanitizeSkillLimit(-1))
	require.Equal(t, maxSkillQueryLimit, sanitizeSkillLimit(maxSkillQueryLimit+1))
	require.Equal(t, 2, sanitizeSkillLimit(2))

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
	mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
	mockQuery.On("Index", models.IndexGSI1).Return(mockQuery).Once()
	mockQuery.On("Where", "gsi1PK", "=", "pk").Return(mockQuery).Once()
	mockQuery.On("OrderBy", gsi1SKField, "ASC").Return(mockQuery).Once()
	mockQuery.On("Limit", 2).Return(mockQuery).Once()
	mockQuery.On("All", mock.Anything).Return(errors.New("query failed")).Once()

	_, _, err = querySkillGSIPage[*models.Skill](context.Background(), mockDB, &models.Skill{}, models.IndexGSI1, "gsi1PK", gsi1SKField, "pk", 1, "")
	require.Error(t, err)
}

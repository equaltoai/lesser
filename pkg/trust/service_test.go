package trust

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type stubTrustRepo struct {
	getRelationshipFn func(ctx context.Context, trusterID, trusteeID, category string) (*storage.TrustRelationship, error)
	getScoreFn        func(ctx context.Context, actorID, category string) (*storage.TrustScore, error)
	getRelsFn         func(ctx context.Context, trusterID string, limit int, cursor string) ([]*storage.TrustRelationship, string, error)
	getTrustedByFn    func(ctx context.Context, trusteeID string, limit int, cursor string) ([]*storage.TrustRelationship, string, error)

	createdRel  *storage.TrustRelationship
	updatedRel  *storage.TrustRelationship
	deletedArgs struct {
		trusterID string
		trusteeID string
		category  string
	}
	recordedUpdate *storage.TrustUpdate

	userTrustScore    float64
	userTrustScoreErr error
}

func (s *stubTrustRepo) CreateTrustRelationship(_ context.Context, relationship *storage.TrustRelationship) error {
	s.createdRel = relationship
	return nil
}

func (s *stubTrustRepo) GetTrustRelationship(ctx context.Context, trusterID, trusteeID, category string) (*storage.TrustRelationship, error) {
	if s.getRelationshipFn != nil {
		return s.getRelationshipFn(ctx, trusterID, trusteeID, category)
	}
	return nil, storage.ErrNotFound
}

func (s *stubTrustRepo) UpdateTrustRelationship(_ context.Context, relationship *storage.TrustRelationship) error {
	s.updatedRel = relationship
	return nil
}

func (s *stubTrustRepo) DeleteTrustRelationship(_ context.Context, trusterID, trusteeID, category string) error {
	s.deletedArgs.trusterID = trusterID
	s.deletedArgs.trusteeID = trusteeID
	s.deletedArgs.category = category
	return nil
}

func (s *stubTrustRepo) GetTrustRelationships(ctx context.Context, trusterID string, limit int, cursor string) ([]*storage.TrustRelationship, string, error) {
	if s.getRelsFn != nil {
		return s.getRelsFn(ctx, trusterID, limit, cursor)
	}
	return []*storage.TrustRelationship{}, "", nil
}

func (s *stubTrustRepo) GetTrustedByRelationships(ctx context.Context, trusteeID string, limit int, cursor string) ([]*storage.TrustRelationship, string, error) {
	if s.getTrustedByFn != nil {
		return s.getTrustedByFn(ctx, trusteeID, limit, cursor)
	}
	return []*storage.TrustRelationship{}, "", nil
}

func (s *stubTrustRepo) GetTrustScore(ctx context.Context, actorID, category string) (*storage.TrustScore, error) {
	if s.getScoreFn != nil {
		return s.getScoreFn(ctx, actorID, category)
	}
	return nil, storage.ErrNotFound
}

func (s *stubTrustRepo) UpdateTrustScore(_ context.Context, _ *storage.TrustScore) error { return nil }

func (s *stubTrustRepo) RecordTrustUpdate(_ context.Context, update *storage.TrustUpdate) error {
	s.recordedUpdate = update
	return nil
}

func (s *stubTrustRepo) GetAllTrustRelationships(_ context.Context, _ int) ([]*storage.TrustRelationship, error) {
	return []*storage.TrustRelationship{}, nil
}

func (s *stubTrustRepo) GetUserTrustScore(_ context.Context, _ string) (float64, error) {
	return s.userTrustScore, s.userTrustScoreErr
}

func TestService_GetTrustScore_DirectRelationship(t *testing.T) {
	repo := &stubTrustRepo{
		getRelationshipFn: func(_ context.Context, trusterID, trusteeID, category string) (*storage.TrustRelationship, error) {
			require.Equal(t, "alice", trusterID)
			require.Equal(t, "bob", trusteeID)
			require.Equal(t, string(TrustCategoryGeneral), category)
			return &storage.TrustRelationship{TrusteeID: trusteeID, Score: 0.9, Confidence: 0.7}, nil
		},
	}
	svc := NewService(repo, zap.NewNop())

	score, err := svc.GetTrustScore(context.Background(), "alice", "bob")
	require.NoError(t, err)
	require.NotNil(t, score)
	assert.Equal(t, 0.9, score.Score)
	assert.Equal(t, 0.9, score.DirectScore)
	assert.Equal(t, 1, score.TrusterCount)
}

func TestService_GetTrustScore_DefaultWhenNoData(t *testing.T) {
	repo := &stubTrustRepo{
		getRelationshipFn: func(_ context.Context, _, _, _ string) (*storage.TrustRelationship, error) {
			return nil, storage.ErrNotFound
		},
		getScoreFn: func(_ context.Context, _, _ string) (*storage.TrustScore, error) {
			return nil, storage.ErrNotFound
		},
	}
	svc := NewService(repo, zap.NewNop())

	score, err := svc.GetTrustScore(context.Background(), "alice", "bob")
	require.NoError(t, err)
	require.NotNil(t, score)
	assert.Equal(t, 0.5, score.Score)
	assert.Equal(t, 0, score.TrusterCount)
}

func TestService_GetTrustScore_UsesCalculatedScore(t *testing.T) {
	now := time.Now()
	repo := &stubTrustRepo{
		getRelationshipFn: func(_ context.Context, _, _, _ string) (*storage.TrustRelationship, error) {
			return nil, storage.ErrNotFound
		},
		getScoreFn: func(_ context.Context, actorID, category string) (*storage.TrustScore, error) {
			return &storage.TrustScore{
				ActorID:         actorID,
				Category:        TrustCategory(category),
				Score:           0.8,
				DirectScore:     0.6,
				PropagatedScore: 0.2,
				Confidence:      0.7,
				TrusterCount:    3,
				LastCalculated:  now,
				CacheTTL:        now.Add(time.Hour),
				CategoryScores:  map[string]float64{category: 0.8},
			}, nil
		},
	}
	svc := NewService(repo, zap.NewNop())

	score, err := svc.GetTrustScore(context.Background(), "alice", "bob")
	require.NoError(t, err)
	require.NotNil(t, score)
	assert.Equal(t, 0.8, score.Score)
	assert.Equal(t, 3, score.TrusterCount)
	assert.Equal(t, map[string]float64{string(TrustCategoryGeneral): 0.8}, score.CategoryScores)
}

func TestService_CreateUpdateDeleteAndRecord(t *testing.T) {
	repo := &stubTrustRepo{}
	svc := NewService(repo, zap.NewNop())

	require.Error(t, svc.CreateTrustRelationship(context.Background(), nil))
	require.Error(t, svc.UpdateTrustRelationship(context.Background(), nil))
	require.Error(t, svc.RecordTrustUpdate(context.Background(), nil))

	err := svc.CreateTrustRelationship(context.Background(), &TrustRelationship{
		ID:         "r1",
		TrusterID:  "alice",
		TrusteeID:  "bob",
		Category:   TrustCategoryGeneral,
		Score:      0.9,
		Confidence: 0.7,
		Evidence:   []TrustEvidence{{Type: "direct_interaction", Score: 0.1, Description: "x"}},
	})
	require.NoError(t, err)
	require.NotNil(t, repo.createdRel)
	assert.Equal(t, "alice", repo.createdRel.TrusterID)
	assert.Equal(t, []storage.TrustEvidence{{Type: "direct_interaction", Score: 0.1, Description: "x"}}, repo.createdRel.Evidence)

	err = svc.UpdateTrustRelationship(context.Background(), &TrustRelationship{
		ID:         "r2",
		TrusterID:  "alice",
		TrusteeID:  "bob",
		Category:   TrustCategoryGeneral,
		Score:      0.1,
		Confidence: 0.2,
	})
	require.NoError(t, err)
	require.NotNil(t, repo.updatedRel)
	assert.Equal(t, "r2", repo.updatedRel.ID)

	require.NoError(t, svc.DeleteTrustRelationship(context.Background(), "alice", "bob", TrustCategoryGeneral))
	assert.Equal(t, "alice", repo.deletedArgs.trusterID)
	assert.Equal(t, string(TrustCategoryGeneral), repo.deletedArgs.category)

	err = svc.RecordTrustUpdate(context.Background(), &TrustUpdate{
		ActorID:   "bob",
		Category:  TrustCategoryGeneral,
		Delta:     0.1,
		Reason:    "test",
		EventID:   "e1",
		Timestamp: time.Now(),
	})
	require.NoError(t, err)
	require.NotNil(t, repo.recordedUpdate)
	assert.Equal(t, "bob", repo.recordedUpdate.ActorID)
}

func TestService_GetTrustSummary_ReputationLevels(t *testing.T) {
	tests := []struct {
		name            string
		scoresByCat     map[string]*storage.TrustScore
		expectLevel     string
		expectOverallGE float64
	}{
		{
			name: "high",
			scoresByCat: map[string]*storage.TrustScore{
				string(TrustCategoryGeneral):   {Score: 0.9, TrusterCount: 10},
				string(TrustCategoryContent):   {Score: 0.9},
				string(TrustCategoryBehavior):  {Score: 0.9},
				string(TrustCategoryTechnical): {Score: 0.9},
			},
			expectLevel:     "high",
			expectOverallGE: 0.8,
		},
		{
			name: "medium",
			scoresByCat: map[string]*storage.TrustScore{
				string(TrustCategoryGeneral):   {Score: 0.7, TrusterCount: 2},
				string(TrustCategoryContent):   {Score: 0.7},
				string(TrustCategoryBehavior):  {Score: 0.7},
				string(TrustCategoryTechnical): {Score: 0.7},
			},
			expectLevel:     "medium",
			expectOverallGE: 0.6,
		},
		{
			name: "low",
			scoresByCat: map[string]*storage.TrustScore{
				string(TrustCategoryGeneral):   {Score: 0.5, TrusterCount: 1},
				string(TrustCategoryContent):   {Score: 0.5},
				string(TrustCategoryBehavior):  {Score: 0.5},
				string(TrustCategoryTechnical): {Score: 0.5},
			},
			expectLevel:     "low",
			expectOverallGE: 0.4,
		},
		{
			name: "new",
			scoresByCat: map[string]*storage.TrustScore{
				string(TrustCategoryGeneral):   {Score: 0.1, TrusterCount: 0},
				string(TrustCategoryContent):   {Score: 0.1},
				string(TrustCategoryBehavior):  {Score: 0.1},
				string(TrustCategoryTechnical): {Score: 0.1},
			},
			expectLevel: "new",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := &stubTrustRepo{
				getScoreFn: func(_ context.Context, _ string, category string) (*storage.TrustScore, error) {
					if score, ok := tc.scoresByCat[category]; ok {
						return score, nil
					}
					return nil, storage.ErrNotFound
				},
				getRelsFn: func(_ context.Context, _ string, _ int, _ string) ([]*storage.TrustRelationship, string, error) {
					return []*storage.TrustRelationship{{ID: "r1"}, {ID: "r2"}}, "", nil
				},
			}
			svc := NewService(repo, zap.NewNop())

			summary, err := svc.GetTrustSummary(context.Background(), "alice")
			require.NoError(t, err)
			require.NotNil(t, summary)
			assert.Equal(t, tc.expectLevel, summary.ReputationLevel)
			if tc.expectOverallGE > 0 {
				assert.GreaterOrEqual(t, summary.OverallScore, tc.expectOverallGE)
			}
			assert.Equal(t, 2, summary.TrustsCount)
		})
	}

	t.Run("non-notfound errors skip categories", func(t *testing.T) {
		repo := &stubTrustRepo{
			getScoreFn: func(_ context.Context, _ string, category string) (*storage.TrustScore, error) {
				if category == string(TrustCategoryBehavior) {
					return nil, errors.New("boom")
				}
				return &storage.TrustScore{Score: 0.9}, nil
			},
		}
		svc := NewService(repo, zap.NewNop())

		summary, err := svc.GetTrustSummary(context.Background(), "alice")
		require.NoError(t, err)
		assert.NotEmpty(t, summary.ReputationLevel)
	})
}

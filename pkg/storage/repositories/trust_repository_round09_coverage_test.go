package repositories

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/trust"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	dmerrors "github.com/theory-cloud/tabletheory/v3/pkg/errors"
	"github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	"go.uber.org/zap"
)

func TestTrustRepository_CRUDAndQueries(t *testing.T) {
	mockDB, mockQuery := newMockDBQuery()
	updateBuilder := new(mocks.MockUpdateBuilder)
	mockQuery.On("UpdateBuilder").Return(updateBuilder).Maybe()
	updateBuilder.On("Set", mock.Anything, mock.Anything).Return(updateBuilder).Maybe()
	updateBuilder.On("Execute").Return(nil).Maybe()

	repo := &TrustRepository{
		EnhancedBaseRepository: NewEnhancedBaseRepository[*models.TrustRelationship](mockDB, "tbl", zap.NewNop(), nil, "TrustRepository", "trust"),
		scoreRepo:              NewEnhancedBaseRepository[*models.TrustScore](mockDB, "tbl", zap.NewNop(), nil, "TrustRepository.Score", "trust_score"),
		updateRepo:             NewEnhancedBaseRepository[*models.TrustUpdate](mockDB, "tbl", zap.NewNop(), nil, "TrustRepository.Update", "trust_update"),
		logger:                 zap.NewNop(),
	}

	err := repo.CreateTrustRelationship(context.Background(), nil)
	require.Error(t, err)

	mockQuery.On("Create").Return(nil).Once()
	mockQuery.On("Delete").Return(nil).Maybe() // cache invalidation and deletes
	err = repo.CreateTrustRelationship(context.Background(), &storage.TrustRelationship{
		ID:         "rel-1",
		TrusterID:  "alice",
		TrusteeID:  "bob",
		Category:   trust.TrustCategoryGeneral,
		Score:      0.8,
		Confidence: 0.9,
		Created:    time.Now(),
		Updated:    time.Now(),
		TTL:        123,
	})
	require.NoError(t, err)

	// GetTrustRelationship not found by string pattern
	mockQuery.On("First", mock.Anything).Return(errors.New("item not found: pk=TRUST#alice#general, sk=TRUSTEE#bob")).Once()
	_, err = repo.GetTrustRelationship(context.Background(), "alice", "bob", "general")
	require.Error(t, err)

	// UpdateTrustRelationship create when not found
	mockQuery.On("First", mock.Anything).Return(dmerrors.ErrItemNotFound).Once()
	mockQuery.On("Create").Return(nil).Once()
	mockQuery.On("Delete").Return(nil).Maybe()
	err = repo.UpdateTrustRelationship(context.Background(), &storage.TrustRelationship{
		ID:         "rel-2",
		TrusterID:  "alice",
		TrusteeID:  "carol",
		Category:   trust.TrustCategoryGeneral,
		Score:      0.6,
		Confidence: 1.0,
		TTL:        0,
	})
	require.NoError(t, err)

	// UpdateTrustRelationship update existing
	mockQuery.On("First", mock.Anything).Return(nil).Once()
	err = repo.UpdateTrustRelationship(context.Background(), &storage.TrustRelationship{
		ID:         "rel-3",
		TrusterID:  "alice",
		TrusteeID:  "dave",
		Category:   trust.TrustCategoryGeneral,
		Score:      0.2,
		Confidence: 0.7,
		TTL:        999,
	})
	require.NoError(t, err)

	// DeleteTrustRelationship
	err = repo.DeleteTrustRelationship(context.Background(), "alice", "bob", "general")
	require.NoError(t, err)

	// GetTrustRelationships pagination
	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		if rels, ok := args.Get(0).(*[]*models.TrustRelationship); ok {
			*rels = []*models.TrustRelationship{
				{ID: "rel-a", TrusterID: "alice", TrusteeID: "x", SK: "SK#1"},
				{ID: "rel-b", TrusterID: "alice", TrusteeID: "y", SK: "SK#2"},
			}
		}
	}).Return(nil).Once()
	relationships, next, err := repo.GetTrustRelationships(context.Background(), "alice", 1, "")
	require.NoError(t, err)
	require.Len(t, relationships, 1)
	require.NotEmpty(t, next)

	// GetTrustedByRelationships pagination
	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		if rels, ok := args.Get(0).(*[]*models.TrustRelationship); ok {
			*rels = []*models.TrustRelationship{
				{TrusterID: "t1", TrusteeID: "bob", Category: trust.TrustCategoryGeneral, GSI1SK: "s1"},
				{TrusterID: "t2", TrusteeID: "bob", Category: trust.TrustCategoryGeneral, GSI1SK: "s2"},
			}
		}
	}).Return(nil).Once()
	relationships, next, err = repo.GetTrustedByRelationships(context.Background(), "bob", 1, "")
	require.NoError(t, err)
	require.Len(t, relationships, 1)
	require.NotEmpty(t, next)

	requireNoMockExpectations(t, mockDB, mockQuery)
	updateBuilder.AssertExpectations(t)
}

func TestTrustRepository_ScoreCachingAndPropagationHelpers(t *testing.T) {
	mockDB, mockQuery := newMockDBQuery()
	repo := &TrustRepository{
		EnhancedBaseRepository: NewEnhancedBaseRepository[*models.TrustRelationship](mockDB, "tbl", zap.NewNop(), nil, "TrustRepository", "trust"),
		scoreRepo:              NewEnhancedBaseRepository[*models.TrustScore](mockDB, "tbl", zap.NewNop(), nil, "TrustRepository.Score", "trust_score"),
		updateRepo:             NewEnhancedBaseRepository[*models.TrustUpdate](mockDB, "tbl", zap.NewNop(), nil, "TrustRepository.Update", "trust_update"),
		logger:                 zap.NewNop(),
	}

	// Cache hit
	mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
		dst := args.Get(0)
		if m, ok := dst.(*models.TrustScore); ok {
			m.ActorID = "bob"
			m.Category = trust.TrustCategoryGeneral
			m.Score = 0.9
			m.CacheTTL = time.Now().Add(time.Hour)
		}
	}).Return(nil).Once()
	score, err := repo.GetTrustScore(context.Background(), "bob", string(trust.TrustCategoryGeneral))
	require.NoError(t, err)
	require.Equal(t, 0.9, score.Score)

	// UpdateTrustScore nil validation
	err = repo.UpdateTrustScore(context.Background(), nil)
	require.Error(t, err)

	// RecordTrustUpdate nil validation
	err = repo.RecordTrustUpdate(context.Background(), nil)
	require.Error(t, err)

	// Pure helpers
	init := repo.initializeTrustScore("a", "general")
	require.Equal(t, "a", init.ActorID)

	relWeight := repo.calculateRelationshipWeight(&storage.TrustRelationship{Category: trust.TrustCategoryGeneral, Confidence: 0.5}, "general")
	require.Equal(t, 0.5, relWeight)
	relWeight = repo.calculateRelationshipWeight(&storage.TrustRelationship{Category: storage.TrustCategory("other"), Confidence: 0.5}, "general")
	require.Equal(t, 0.25, relWeight)

	ts := &storage.TrustScore{DirectScore: 2.0, PropagatedScore: 2.0, TrusterCount: 0}
	repo.finalizeTrustScore(ts)
	require.Equal(t, 1.0, ts.Score)

	ts = &storage.TrustScore{DirectScore: -1.0, PropagatedScore: -1.0, TrusterCount: 1}
	repo.finalizeTrustScore(ts)
	require.Equal(t, 0.0, ts.Score)

	w := repo.processNodeTrust(trustNode{actorID: "t1", pathTrust: 0.4}, map[string]float64{"t1": 0.2}, 0.1, 0.0)
	require.Equal(t, 0.4, w)
	w = repo.processNodeTrust(trustNode{actorID: "t2", pathTrust: 0.4}, map[string]float64{"t2": 0.05}, 0.1, 0.0)
	require.Equal(t, 0.0, w)

	visited := map[string]bool{"t1": true}
	require.False(t, repo.shouldAddToQueue(&storage.TrustRelationship{TrusterID: "t1", Category: trust.TrustCategoryGeneral}, "general", visited))
	require.True(t, repo.shouldAddToQueue(&storage.TrustRelationship{TrusterID: "t2", Category: trust.TrustCategoryGeneral}, "general", visited))

	requireNoMockExpectations(t, mockDB, mockQuery)
}

func TestTrustRepository_GetTrustScore_RecalculateAndCache(t *testing.T) {
	mockDB, mockQuery := newMockDBQuery()
	repo := &TrustRepository{
		EnhancedBaseRepository: NewEnhancedBaseRepository[*models.TrustRelationship](mockDB, "tbl", zap.NewNop(), nil, "TrustRepository", "trust"),
		scoreRepo:              NewEnhancedBaseRepository[*models.TrustScore](mockDB, "tbl", zap.NewNop(), nil, "TrustRepository.Score", "trust_score"),
		updateRepo:             NewEnhancedBaseRepository[*models.TrustUpdate](mockDB, "tbl", zap.NewNop(), nil, "TrustRepository.Update", "trust_update"),
		logger:                 zap.NewNop(),
	}

	// 1) scoreRepo.Get cache miss
	mockQuery.On("First", mock.Anything).Return(errors.New("miss")).Once()

	allCall := 0
	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		allCall++
		rels, ok := args.Get(0).(*[]*models.TrustRelationship)
		if !ok {
			return
		}

		switch allCall {
		case 1, 2, 3:
			// GetTrustedByRelationships (direct): content/behavior/technical have no rows.
			*rels = nil
		case 4:
			// GetTrustedByRelationships (direct): general returns direct relationships.
			*rels = []*models.TrustRelationship{
				{TrusterID: "t1", TrusteeID: "actor", Category: trust.TrustCategoryGeneral, Score: 0.8, Confidence: 1.0, GSI1SK: "x"},
			}
		case 5, 6, 7:
			// GetTrustedByRelationships (propagation expansion): content/behavior/technical have no rows.
			*rels = nil
		case 8:
			// GetTrustedByRelationships (propagation expansion): general returns additional relationships.
			*rels = []*models.TrustRelationship{
				{TrusterID: "t2", TrusteeID: "actor", Category: trust.TrustCategoryGeneral, Score: 0.6, Confidence: 1.0, GSI1SK: "y"},
			}
		default:
			*rels = nil
		}
	}).Return(nil)

	// 3) getCachedTrustScore (scoreRepo.Get) returns a valid cached node score to allow propagation
	mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
		if s, ok := args.Get(0).(*models.TrustScore); ok {
			s.ActorID = "actor"
			s.Category = trust.TrustCategoryGeneral
			s.Score = 0.9
			s.CacheTTL = time.Now().Add(time.Hour)
		}
	}).Return(nil).Once()

	// 5) UpdateTrustScore caches calculated score (Create)
	mockQuery.On("Create").Return(nil).Once()

	score, err := repo.GetTrustScore(context.Background(), "actor", string(trust.TrustCategoryGeneral))
	require.NoError(t, err)
	require.GreaterOrEqual(t, score.Score, 0.0)

	requireNoMockExpectations(t, mockDB, mockQuery)
}

func TestTrustRepository_GetAllTrustRelationshipsAndUpdates(t *testing.T) {
	mockDB, mockQuery := newMockDBQuery()
	repo := &TrustRepository{
		EnhancedBaseRepository: NewEnhancedBaseRepository[*models.TrustRelationship](mockDB, "tbl", zap.NewNop(), nil, "TrustRepository", "trust"),
		scoreRepo:              NewEnhancedBaseRepository[*models.TrustScore](mockDB, "tbl", zap.NewNop(), nil, "TrustRepository.Score", "trust_score"),
		updateRepo:             NewEnhancedBaseRepository[*models.TrustUpdate](mockDB, "tbl", zap.NewNop(), nil, "TrustRepository.Update", "trust_update"),
		logger:                 zap.NewNop(),
	}

	// GetAllTrustRelationships scan success
	mockQuery.On("Scan", mock.Anything).Run(func(args mock.Arguments) {
		if rels, ok := args.Get(0).(*[]*models.TrustRelationship); ok {
			*rels = []*models.TrustRelationship{{ID: "rel-1", TrusterID: "a", TrusteeID: "b", Category: trust.TrustCategoryGeneral}}
		}
	}).Return(nil).Once()
	all, err := repo.GetAllTrustRelationships(context.Background(), 10)
	require.NoError(t, err)
	require.Len(t, all, 1)

	// RecordTrustUpdate success
	mockQuery.On("Create").Return(nil).Once()
	err = repo.RecordTrustUpdate(context.Background(), &storage.TrustUpdate{
		ActorID:   "a",
		EventID:   "evt",
		Category:  trust.TrustCategoryGeneral,
		Delta:     0.1,
		Reason:    "test",
		Timestamp: time.Now(),
	})
	require.NoError(t, err)

	// getCachedTrustScore expired path
	mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
		if s, ok := args.Get(0).(*models.TrustScore); ok {
			s.ActorID = "a"
			s.Category = trust.TrustCategoryGeneral
			s.Score = 0.9
			s.CacheTTL = time.Now().Add(-time.Hour)
		}
	}).Return(nil).Once()
	cached, err := repo.getCachedTrustScore(context.Background(), "a", "general")
	require.NoError(t, err)
	require.Nil(t, cached)

	requireNoMockExpectations(t, mockDB, mockQuery)
}

func TestTrustRepository_GetUserTrustScore_AndConstructor(t *testing.T) {
	mockDB, mockQuery := newMockDBQuery()
	_ = NewTrustRepository(mockDB, "tbl", zap.NewNop(), nil)

	repo := &TrustRepository{
		EnhancedBaseRepository: NewEnhancedBaseRepository[*models.TrustRelationship](mockDB, "tbl", zap.NewNop(), nil, "TrustRepository", "trust"),
		scoreRepo:              NewEnhancedBaseRepository[*models.TrustScore](mockDB, "tbl", zap.NewNop(), nil, "TrustRepository.Score", "trust_score"),
		updateRepo:             NewEnhancedBaseRepository[*models.TrustUpdate](mockDB, "tbl", zap.NewNop(), nil, "TrustRepository.Update", "trust_update"),
		logger:                 zap.NewNop(),
	}

	// GetTrustScore cache hit used by GetUserTrustScore
	mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
		if s, ok := args.Get(0).(*models.TrustScore); ok {
			s.ActorID = "u"
			s.Category = trust.TrustCategoryGeneral
			s.Score = 0.77
			s.CacheTTL = time.Now().Add(time.Hour)
		}
	}).Return(nil).Once()

	score, err := repo.GetUserTrustScore(context.Background(), "u")
	require.NoError(t, err)
	require.Equal(t, 0.77, score)

	requireNoMockExpectations(t, mockDB, mockQuery)
}

func TestTrustRepository_MoreErrorBranches(t *testing.T) {
	mockDB, mockQuery := newMockDBQuery()
	updateBuilder := new(mocks.MockUpdateBuilder)
	mockQuery.On("UpdateBuilder").Return(updateBuilder).Maybe()
	updateBuilder.On("Set", mock.Anything, mock.Anything).Return(updateBuilder).Maybe()
	updateBuilder.On("Execute").Return(errors.New("boom")).Once()

	repo := &TrustRepository{
		EnhancedBaseRepository: NewEnhancedBaseRepository[*models.TrustRelationship](mockDB, "tbl", zap.NewNop(), nil, "TrustRepository", "trust"),
		scoreRepo:              NewEnhancedBaseRepository[*models.TrustScore](mockDB, "tbl", zap.NewNop(), nil, "TrustRepository.Score", "trust_score"),
		updateRepo:             NewEnhancedBaseRepository[*models.TrustUpdate](mockDB, "tbl", zap.NewNop(), nil, "TrustRepository.Update", "trust_update"),
		logger:                 zap.NewNop(),
	}

	// CreateTrustRelationship create error
	mockQuery.On("Create").Return(errors.New("boom")).Once()
	err := repo.CreateTrustRelationship(context.Background(), &storage.TrustRelationship{
		ID:         "rel-x",
		TrusterID:  "a",
		TrusteeID:  "b",
		Category:   trust.TrustCategoryGeneral,
		Score:      0.1,
		Confidence: 0.2,
	})
	require.Error(t, err)

	// UpdateTrustRelationship execute error
	mockQuery.On("First", mock.Anything).Return(nil).Once()
	err = repo.UpdateTrustRelationship(context.Background(), &storage.TrustRelationship{
		ID:         "rel-y",
		TrusterID:  "a",
		TrusteeID:  "b",
		Category:   trust.TrustCategoryGeneral,
		Score:      0.2,
		Confidence: 0.3,
	})
	require.Error(t, err)

	// DeleteTrustRelationship delete error
	mockQuery.On("Delete").Return(errors.New("boom")).Once()
	err = repo.DeleteTrustRelationship(context.Background(), "a", "b", "general")
	require.Error(t, err)

	// GetTrustRelationships cursor branch + scan error
	mockQuery.On("All", mock.Anything).Return(errors.New("boom")).Once()
	cursor := encodeTrustRelationshipCursor(trustRelationshipCursor{Category: "general", LastSK: "RELATIONSHIP#sk"})
	_, _, err = repo.GetTrustRelationships(context.Background(), "a", 10, cursor)
	require.Error(t, err)

	// getCachedTrustScore error path
	mockQuery.On("First", mock.Anything).Return(errors.New("boom")).Once()
	_, err = repo.getCachedTrustScore(context.Background(), "a", "general")
	require.Error(t, err)

	// UpdateTrustScore create error
	mockQuery.On("Create").Return(errors.New("boom")).Once()
	err = repo.UpdateTrustScore(context.Background(), &storage.TrustScore{
		ActorID:        "a",
		Category:       trust.TrustCategoryGeneral,
		Score:          0.1,
		LastCalculated: time.Now(),
		CacheTTL:       time.Now().Add(time.Hour),
		CategoryScores: map[string]float64{},
	})
	require.Error(t, err)

	// invalidateTrustScoreCache error branch (delete fails, ignored)
	mockQuery.On("Delete").Return(errors.New("boom")).Once()
	repo.invalidateTrustScoreCache(context.Background(), "a", "general")

	requireNoMockExpectations(t, mockDB, mockQuery)
	updateBuilder.AssertExpectations(t)
}

func TestTrustRepository_GetUserTrustScore_ErrorPath(t *testing.T) {
	mockDB, mockQuery := newMockDBQuery()
	repo := &TrustRepository{
		EnhancedBaseRepository: NewEnhancedBaseRepository[*models.TrustRelationship](mockDB, "tbl", zap.NewNop(), nil, "TrustRepository", "trust"),
		scoreRepo:              NewEnhancedBaseRepository[*models.TrustScore](mockDB, "tbl", zap.NewNop(), nil, "TrustRepository.Score", "trust_score"),
		updateRepo:             NewEnhancedBaseRepository[*models.TrustUpdate](mockDB, "tbl", zap.NewNop(), nil, "TrustRepository.Update", "trust_update"),
		logger:                 zap.NewNop(),
	}

	// cache miss then calculateTrustScore fails due to GetTrustedByRelationships scan error
	mockQuery.On("First", mock.Anything).Return(errors.New("miss")).Once()
	mockQuery.On("All", mock.Anything).Return(errors.New("boom")).Once()

	_, err := repo.GetUserTrustScore(context.Background(), "u")
	require.Error(t, err)

	requireNoMockExpectations(t, mockDB, mockQuery)
}

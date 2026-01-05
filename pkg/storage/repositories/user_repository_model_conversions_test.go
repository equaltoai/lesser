package repositories

import (
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/trust"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

// ============================================
// Test shouldAddToPropagation
// ============================================

func TestUserRepository_shouldAddToPropagation(t *testing.T) {
	repo := newMinimalUserRepo()

	tests := []struct {
		name     string
		rel      *storage.TrustRelationship
		category string
		visited  map[string]bool
		expected bool
	}{
		{
			name: "not visited and exact category match",
			rel: &storage.TrustRelationship{
				TrusterID: "actor1",
				TrusteeID: "actor2",
				Category:  trust.TrustCategoryContent,
			},
			category: "content",
			visited:  map[string]bool{},
			expected: true,
		},
		{
			name: "not visited and general category",
			rel: &storage.TrustRelationship{
				TrusterID: "actor1",
				TrusteeID: "actor2",
				Category:  trust.TrustCategoryGeneral,
			},
			category: "content", // general is relevant for any category
			visited:  map[string]bool{},
			expected: true,
		},
		{
			name: "already visited",
			rel: &storage.TrustRelationship{
				TrusterID: "actor1",
				TrusteeID: "actor2",
				Category:  trust.TrustCategoryContent,
			},
			category: "content",
			visited:  map[string]bool{"actor1": true},
			expected: false,
		},
		{
			name: "wrong category and not general",
			rel: &storage.TrustRelationship{
				TrusterID: "actor1",
				TrusteeID: "actor2",
				Category:  trust.TrustCategoryBehavior,
			},
			category: "content",
			visited:  map[string]bool{},
			expected: false,
		},
		{
			name: "visited but different actor",
			rel: &storage.TrustRelationship{
				TrusterID: "actor1",
				TrusteeID: "actor2",
				Category:  trust.TrustCategoryContent,
			},
			category: "content",
			visited:  map[string]bool{"actor3": true, "actor4": true},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := repo.shouldAddToPropagation(tt.rel, tt.category, tt.visited)
			assert.Equal(t, tt.expected, got)
		})
	}
}

// ============================================
// Test modelToTrustRelationship
// ============================================

func TestUserRepository_modelToTrustRelationship(t *testing.T) {
	repo := newMinimalUserRepo()

	now := time.Now()
	model := &models.TrustRelationship{
		ID:         "test-id",
		TrusterID:  "truster1",
		TrusteeID:  "trustee1",
		Category:   models.TrustCategoryContent,
		Score:      0.85,
		Confidence: 0.9,
		Evidence: []models.TrustEvidence{
			{
				Type:        "test",
				Score:       0.5,
				Description: "test evidence",
				Timestamp:   now,
			},
		},
		TTL:     12345,
		Created: now,
		Updated: now,
	}

	result := repo.modelToTrustRelationship(model)

	assert.Equal(t, "test-id", result.ID)
	assert.Equal(t, "truster1", result.TrusterID)
	assert.Equal(t, "trustee1", result.TrusteeID)
	assert.Equal(t, models.TrustCategoryContent, result.Category)
	assert.InDelta(t, 0.85, result.Score, 0.0001)
	assert.InDelta(t, 0.9, result.Confidence, 0.0001)
	assert.Equal(t, int64(12345), result.TTL)
	assert.Equal(t, now, result.Created)
	assert.Equal(t, now, result.Updated)
}

func TestUserRepository_modelToTrustRelationship_EmptyEvidence(t *testing.T) {
	repo := newMinimalUserRepo()

	model := &models.TrustRelationship{
		ID:        "test-id",
		TrusterID: "truster1",
		TrusteeID: "trustee1",
		Category:  models.TrustCategoryGeneral,
		Score:     0.5,
		Evidence:  nil, // No evidence
	}

	result := repo.modelToTrustRelationship(model)

	assert.Equal(t, "test-id", result.ID)
	assert.Equal(t, "truster1", result.TrusterID)
	assert.Nil(t, result.Evidence)
}

// ============================================
// Test modelToTrustScore
// ============================================

func TestUserRepository_modelToTrustScore(t *testing.T) {
	repo := newMinimalUserRepo()

	now := time.Now()
	later := now.Add(time.Hour)
	model := &models.TrustScore{
		ActorID:         "actor1",
		Category:        models.TrustCategoryGeneral,
		Score:           0.75,
		DirectScore:     0.8,
		PropagatedScore: 0.6,
		Confidence:      0.9,
		TrusterCount:    5,
		CategoryScores: map[string]float64{
			"content":   0.8,
			"behavior":  0.7,
			"technical": 0.6,
		},
		LastCalculated: now,
		CacheTTL:       later,
	}

	result := repo.modelToTrustScore(model)

	assert.Equal(t, "actor1", result.ActorID)
	assert.Equal(t, models.TrustCategoryGeneral, result.Category)
	assert.InDelta(t, 0.75, result.Score, 0.0001)
	assert.InDelta(t, 0.8, result.DirectScore, 0.0001)
	assert.InDelta(t, 0.6, result.PropagatedScore, 0.0001)
	assert.InDelta(t, 0.9, result.Confidence, 0.0001)
	assert.Equal(t, 5, result.TrusterCount)
	assert.Equal(t, 0.8, result.CategoryScores["content"])
	assert.Equal(t, 0.7, result.CategoryScores["behavior"])
	assert.Equal(t, 0.6, result.CategoryScores["technical"])
	assert.Equal(t, now, result.LastCalculated)
	assert.Equal(t, later, result.CacheTTL)
}

func TestUserRepository_modelToTrustScore_EmptyCategoryScores(t *testing.T) {
	repo := newMinimalUserRepo()

	model := &models.TrustScore{
		ActorID:        "actor1",
		Score:          0.5,
		CategoryScores: nil,
	}

	result := repo.modelToTrustScore(model)

	assert.Equal(t, "actor1", result.ActorID)
	assert.InDelta(t, 0.5, result.Score, 0.0001)
	assert.Nil(t, result.CategoryScores)
}

// ============================================
// Test initTrustScore
// ============================================

func TestUserRepository_initTrustScore(t *testing.T) {
	repo := newMinimalUserRepo()

	tests := []struct {
		name          string
		actorID       string
		category      string
		expectedActor string
		expectedCat   models.TrustCategory
	}{
		{
			name:          "general category",
			actorID:       "actor1",
			category:      "general",
			expectedActor: "actor1",
			expectedCat:   models.TrustCategoryGeneral,
		},
		{
			name:          "content category",
			actorID:       "https://example.com/users/alice",
			category:      "content",
			expectedActor: "https://example.com/users/alice",
			expectedCat:   models.TrustCategoryContent,
		},
		{
			name:          "empty category",
			actorID:       "actor2",
			category:      "",
			expectedActor: "actor2",
			expectedCat:   models.TrustCategory(""),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := repo.initTrustScore(tt.actorID, tt.category)

			assert.NotNil(t, score)
			assert.Equal(t, tt.expectedActor, score.ActorID)
			assert.Equal(t, tt.expectedCat, score.Category)
			assert.Equal(t, 0.0, score.Score)
			assert.Equal(t, 0.0, score.DirectScore)
			assert.Equal(t, 0.0, score.PropagatedScore)
			assert.True(t, score.LastCalculated.IsZero()) // Not set by initTrustScore
			assert.NotNil(t, score.CategoryScores)
		})
	}
}

// ============================================
// Additional edge case tests
// ============================================

func TestUserRepository_applyUpdates_RecoveryMethodsInterface(t *testing.T) {
	repo := newMinimalUserRepo()

	// Test with []interface{} which could come from JSON unmarshaling
	user := &models.User{
		Username:        "testuser",
		RecoveryMethods: []string{"email"},
	}

	// Test wrong type - []interface{} instead of []string
	updates := map[string]any{
		"recovery_methods": []interface{}{"passkey", "wallet"},
	}

	repo.applyUpdates(user, updates)

	// Should not change because []interface{} is not []string
	assert.Equal(t, []string{"email"}, user.RecoveryMethods)
}

func TestUserRepository_calculatePropagationFactor_HighDepth(t *testing.T) {
	repo := newMinimalUserRepo()

	// Test with very high depth
	factor := repo.calculatePropagationFactor(10, 0.5)

	// Should be 0.5^9 = 0.001953125
	assert.InDelta(t, 0.001953125, factor, 0.0001)
}

func TestUserRepository_combineTrustScores_EdgeCases(t *testing.T) {
	repo := newMinimalUserRepo()

	tests := []struct {
		name          string
		directScore   float64
		propagated    float64
		expectedScore float64
	}{
		{
			name:          "very small positive direct",
			directScore:   0.001,
			propagated:    0.0,
			expectedScore: 0.001, // Uses direct only
		},
		{
			name:          "very small positive propagated",
			directScore:   0.0,
			propagated:    0.001,
			expectedScore: 0.001, // Uses propagated only
		},
		{
			name:          "equal scores",
			directScore:   0.5,
			propagated:    0.5,
			expectedScore: 0.5, // (0.5 * 0.7) + (0.5 * 0.3) = 0.5
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := &storage.TrustScore{
				DirectScore:     tt.directScore,
				PropagatedScore: tt.propagated,
			}
			repo.combineTrustScores(score)
			assert.InDelta(t, tt.expectedScore, score.Score, 0.0001)
		})
	}
}

// ============================================
// Test generateRandomID (package-level function)
// ============================================

func TestGenerateRandomID(t *testing.T) {
	tests := []struct {
		name   string
		length int
	}{
		{"length 8", 8},
		{"length 16", 16},
		{"length 1", 1},
		{"length 0", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id := generateRandomID(tt.length)
			assert.Len(t, id, tt.length)

			// All characters should be from the charset
			for _, c := range id {
				assert.Contains(t, "abcdefghijklmnopqrstuvwxyz0123456789", string(c))
			}
		})
	}
}

// ============================================
// Minimal logger helper test
// ============================================

func newMinimalUserRepoWithLogger() *UserRepository {
	return &UserRepository{
		logger: zap.NewNop(),
	}
}

func TestUserRepository_logTrustCalculation(t *testing.T) {
	repo := newMinimalUserRepoWithLogger()

	// This should not panic even with nop logger
	score := &storage.TrustScore{
		DirectScore:     0.8,
		PropagatedScore: 0.6,
		Score:           0.74,
	}

	// Should not panic
	repo.logTrustCalculation("actor1", "general", score)
}

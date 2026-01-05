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

// newMinimalUserRepo creates a minimal repository for testing pure functions
func newMinimalUserRepo() *UserRepository {
	return &UserRepository{
		logger: zap.NewNop(),
	}
}

// ============================================
// Test modelToStorage
// ============================================

func TestUserRepository_modelToStorage(t *testing.T) {
	repo := newMinimalUserRepo()

	tests := []struct {
		name      string
		userModel *models.User
		want      *storage.User
	}{
		{
			name: "complete user model",
			userModel: &models.User{
				Username:        "alice",
				Email:           "alice@example.com",
				PasswordHash:    "hashed",
				DisplayName:     "Alice Wonder",
				CreatedAt:       time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				UpdatedAt:       time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
				Approved:        true,
				Suspended:       false,
				Silenced:        true,
				Role:            "admin",
				Locale:          "fr",
				RecoveryMethods: []string{"email", "passkey"},
			},
			want: &storage.User{
				Username:        "alice",
				Email:           "alice@example.com",
				PasswordHash:    "hashed",
				DisplayName:     "Alice Wonder",
				CreatedAt:       time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				UpdatedAt:       time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
				Approved:        true,
				Suspended:       false,
				Silenced:        true,
				Role:            "admin",
				Locale:          "fr",
				RecoveryMethods: []string{"email", "passkey"},
			},
		},
		{
			name: "minimal user model",
			userModel: &models.User{
				Username: "bob",
				Role:     "user",
			},
			want: &storage.User{
				Username: "bob",
				Role:     "user",
			},
		},
		{
			name:      "nil recovery methods stays nil",
			userModel: &models.User{Username: "charlie", RecoveryMethods: nil},
			want:      &storage.User{Username: "charlie", RecoveryMethods: nil},
		},
		{
			name:      "empty recovery methods stays empty",
			userModel: &models.User{Username: "diana", RecoveryMethods: []string{}},
			want:      &storage.User{Username: "diana", RecoveryMethods: []string{}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := repo.modelToStorage(tt.userModel)
			assert.Equal(t, tt.want.Username, got.Username)
			assert.Equal(t, tt.want.Email, got.Email)
			assert.Equal(t, tt.want.PasswordHash, got.PasswordHash)
			assert.Equal(t, tt.want.DisplayName, got.DisplayName)
			assert.Equal(t, tt.want.CreatedAt, got.CreatedAt)
			assert.Equal(t, tt.want.UpdatedAt, got.UpdatedAt)
			assert.Equal(t, tt.want.Approved, got.Approved)
			assert.Equal(t, tt.want.Suspended, got.Suspended)
			assert.Equal(t, tt.want.Silenced, got.Silenced)
			assert.Equal(t, tt.want.Role, got.Role)
			assert.Equal(t, tt.want.Locale, got.Locale)
			assert.Equal(t, tt.want.RecoveryMethods, got.RecoveryMethods)
		})
	}
}

// ============================================
// Test applyUpdates
// ============================================

func TestUserRepository_applyUpdates(t *testing.T) {
	repo := newMinimalUserRepo()

	tests := []struct {
		name       string
		initial    *models.User
		updates    map[string]any
		assertions func(t *testing.T, user *models.User)
	}{
		{
			name: "apply all supported string fields",
			initial: &models.User{
				Username:     "testuser",
				Email:        "old@example.com",
				PasswordHash: "oldhash",
				DisplayName:  "Old Name",
				Role:         "user",
				Locale:       "en",
			},
			updates: map[string]any{
				"email":         "new@example.com",
				"password_hash": "newhash",
				"display_name":  "New Name",
				"role":          "admin",
				"locale":        "es",
			},
			assertions: func(t *testing.T, user *models.User) {
				assert.Equal(t, "new@example.com", user.Email)
				assert.Equal(t, "newhash", user.PasswordHash)
				assert.Equal(t, "New Name", user.DisplayName)
				assert.Equal(t, "admin", user.Role)
				assert.Equal(t, "es", user.Locale)
			},
		},
		{
			name: "apply all supported bool fields",
			initial: &models.User{
				Username:  "testuser",
				Approved:  false,
				Suspended: false,
				Silenced:  false,
			},
			updates: map[string]any{
				"approved":  true,
				"suspended": true,
				"silenced":  true,
			},
			assertions: func(t *testing.T, user *models.User) {
				assert.True(t, user.Approved)
				assert.True(t, user.Suspended)
				assert.True(t, user.Silenced)
			},
		},
		{
			name: "apply recovery_methods slice",
			initial: &models.User{
				Username:        "testuser",
				RecoveryMethods: []string{"email"},
			},
			updates: map[string]any{
				"recovery_methods": []string{"passkey", "wallet", "email"},
			},
			assertions: func(t *testing.T, user *models.User) {
				assert.Equal(t, []string{"passkey", "wallet", "email"}, user.RecoveryMethods)
			},
		},
		{
			name: "ignore unknown fields",
			initial: &models.User{
				Username: "testuser",
				Email:    "test@example.com",
			},
			updates: map[string]any{
				"unknown_field":          "should be ignored",
				"another_unknown":        123,
				"definitely_not_a_field": true,
			},
			assertions: func(t *testing.T, user *models.User) {
				// Verify nothing changed
				assert.Equal(t, "testuser", user.Username)
				assert.Equal(t, "test@example.com", user.Email)
			},
		},
		{
			name: "wrong type for string field is ignored",
			initial: &models.User{
				Email: "old@example.com",
			},
			updates: map[string]any{
				"email": 12345, // wrong type - should be string
			},
			assertions: func(t *testing.T, user *models.User) {
				assert.Equal(t, "old@example.com", user.Email)
			},
		},
		{
			name: "wrong type for bool field is ignored",
			initial: &models.User{
				Approved: false,
			},
			updates: map[string]any{
				"approved": "true", // wrong type - should be bool
			},
			assertions: func(t *testing.T, user *models.User) {
				assert.False(t, user.Approved)
			},
		},
		{
			name: "wrong type for slice field is ignored",
			initial: &models.User{
				RecoveryMethods: []string{"email"},
			},
			updates: map[string]any{
				"recovery_methods": "passkey,email", // wrong type - should be []string
			},
			assertions: func(t *testing.T, user *models.User) {
				assert.Equal(t, []string{"email"}, user.RecoveryMethods)
			},
		},
		{
			name:    "empty updates does nothing",
			initial: &models.User{Username: "testuser", Email: "test@example.com"},
			updates: map[string]any{},
			assertions: func(t *testing.T, user *models.User) {
				assert.Equal(t, "testuser", user.Username)
				assert.Equal(t, "test@example.com", user.Email)
			},
		},
		{
			name:    "nil updates does nothing",
			initial: &models.User{Username: "testuser"},
			updates: nil,
			assertions: func(t *testing.T, user *models.User) {
				assert.Equal(t, "testuser", user.Username)
			},
		},
		{
			name: "set bool fields to false",
			initial: &models.User{
				Approved:  true,
				Suspended: true,
				Silenced:  true,
			},
			updates: map[string]any{
				"approved":  false,
				"suspended": false,
				"silenced":  false,
			},
			assertions: func(t *testing.T, user *models.User) {
				assert.False(t, user.Approved)
				assert.False(t, user.Suspended)
				assert.False(t, user.Silenced)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Make a copy of initial to avoid mutation between tests
			user := *tt.initial
			repo.applyUpdates(&user, tt.updates)
			tt.assertions(t, &user)
		})
	}
}

// ============================================
// Test extractUsernameFromActorID
// ============================================

func TestExtractUsernameFromActorID(t *testing.T) {
	tests := []struct {
		name     string
		actorID  string
		expected string
	}{
		{
			name:     "URL format",
			actorID:  "https://example.com/users/alice",
			expected: "alice",
		},
		{
			name:     "URL format with trailing slash",
			actorID:  "https://example.com/users/alice/",
			expected: "", // Empty trailing part
		},
		{
			name:     "simple path",
			actorID:  "users/bob",
			expected: "bob",
		},
		{
			name:     "single element",
			actorID:  "a",
			expected: "", // Not enough parts
		},
		{
			name:     "empty string",
			actorID:  "",
			expected: "",
		},
		{
			name:     "URL with subdirectory",
			actorID:  "https://social.example.com/api/v1/accounts/charlie",
			expected: "charlie",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractUsernameFromActorID(tt.actorID)
			assert.Equal(t, tt.expected, got)
		})
	}
}

// ============================================
// Test isRelevantCategory
// ============================================

func TestUserRepository_isRelevantCategory(t *testing.T) {
	repo := newMinimalUserRepo()

	tests := []struct {
		name           string
		relCategory    trust.TrustCategory
		targetCategory string
		expected       bool
	}{
		{
			name:           "exact match content",
			relCategory:    trust.TrustCategoryContent,
			targetCategory: "content",
			expected:       true,
		},
		{
			name:           "exact match behavior",
			relCategory:    trust.TrustCategoryBehavior,
			targetCategory: "behavior",
			expected:       true,
		},
		{
			name:           "exact match technical",
			relCategory:    trust.TrustCategoryTechnical,
			targetCategory: "technical",
			expected:       true,
		},
		{
			name:           "exact match general",
			relCategory:    trust.TrustCategoryGeneral,
			targetCategory: "general",
			expected:       true,
		},
		{
			name:           "general is always relevant",
			relCategory:    trust.TrustCategoryGeneral,
			targetCategory: "content",
			expected:       true,
		},
		{
			name:           "general is relevant for behavior",
			relCategory:    trust.TrustCategoryGeneral,
			targetCategory: "behavior",
			expected:       true,
		},
		{
			name:           "content is not relevant for behavior",
			relCategory:    trust.TrustCategoryContent,
			targetCategory: "behavior",
			expected:       false,
		},
		{
			name:           "behavior is not relevant for technical",
			relCategory:    trust.TrustCategoryBehavior,
			targetCategory: "technical",
			expected:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := repo.isRelevantCategory(tt.relCategory, tt.targetCategory)
			assert.Equal(t, tt.expected, got)
		})
	}
}

// ============================================
// Test calculatePropagationFactor
// ============================================

func TestUserRepository_calculatePropagationFactor(t *testing.T) {
	repo := newMinimalUserRepo()

	tests := []struct {
		name            string
		depth           int
		propagationRate float64
		expected        float64
	}{
		{
			name:            "depth 1 returns 1.0",
			depth:           1,
			propagationRate: 0.5,
			expected:        1.0,
		},
		{
			name:            "depth 2 returns propagation rate",
			depth:           2,
			propagationRate: 0.5,
			expected:        0.5,
		},
		{
			name:            "depth 3 returns rate squared",
			depth:           3,
			propagationRate: 0.5,
			expected:        0.25,
		},
		{
			name:            "depth 4 returns rate cubed",
			depth:           4,
			propagationRate: 0.5,
			expected:        0.125,
		},
		{
			name:            "depth 1 with rate 0.8",
			depth:           1,
			propagationRate: 0.8,
			expected:        1.0,
		},
		{
			name:            "depth 2 with rate 0.8",
			depth:           2,
			propagationRate: 0.8,
			expected:        0.8,
		},
		{
			name:            "depth 3 with rate 0.8",
			depth:           3,
			propagationRate: 0.8,
			expected:        0.64,
		},
		{
			name:            "depth 0 returns 1.0",
			depth:           0,
			propagationRate: 0.5,
			expected:        1.0,
		},
		{
			name:            "depth -1 returns 1.0 (edge case)",
			depth:           -1,
			propagationRate: 0.5,
			expected:        1.0,
		},
		{
			name:            "propagation rate 1.0 always returns 1.0",
			depth:           5,
			propagationRate: 1.0,
			expected:        1.0,
		},
		{
			name:            "propagation rate 0.0 returns 0 for depth > 1",
			depth:           3,
			propagationRate: 0.0,
			expected:        0.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := repo.calculatePropagationFactor(tt.depth, tt.propagationRate)
			assert.InDelta(t, tt.expected, got, 0.0001)
		})
	}
}

// ============================================
// Test calculateNodeWeight
// ============================================

func TestUserRepository_calculateNodeWeight(t *testing.T) {
	repo := newMinimalUserRepo()

	tests := []struct {
		name            string
		node            propagationNode
		propagationRate float64
		expected        float64
	}{
		{
			name: "basic node weight at depth 1",
			node: propagationNode{
				actorID:   "actor1",
				trustPath: 0.8,
				depth:     1,
			},
			propagationRate: 0.5,
			expected:        0.8, // trustPath * 1.0 (propagation factor for depth 1)
		},
		{
			name: "node weight at depth 2",
			node: propagationNode{
				actorID:   "actor2",
				trustPath: 0.8,
				depth:     2,
			},
			propagationRate: 0.5,
			expected:        0.4, // 0.8 * 0.5
		},
		{
			name: "node weight at depth 3",
			node: propagationNode{
				actorID:   "actor3",
				trustPath: 1.0,
				depth:     3,
			},
			propagationRate: 0.5,
			expected:        0.25, // 1.0 * 0.25
		},
		{
			name: "zero trust path",
			node: propagationNode{
				actorID:   "actor",
				trustPath: 0.0,
				depth:     1,
			},
			propagationRate: 0.5,
			expected:        0.0,
		},
		{
			name: "high trust path",
			node: propagationNode{
				actorID:   "actor",
				trustPath: 1.0,
				depth:     1,
			},
			propagationRate: 0.5,
			expected:        1.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := repo.calculateNodeWeight(tt.node, tt.propagationRate)
			assert.InDelta(t, tt.expected, got, 0.0001)
		})
	}
}

// ============================================
// Test calculateContribution
// ============================================

func TestUserRepository_calculateContribution(t *testing.T) {
	repo := newMinimalUserRepo()

	config := userTrustPropagationConfig{
		dampingFactor:   0.85,
		maxDepth:        3,
		minTrustScore:   0.1,
		propagationRate: 0.5,
		maxVisited:      100,
	}

	tests := []struct {
		name      string
		node      propagationNode
		nodeScore float64
		expected  float64
	}{
		{
			name: "basic contribution at depth 1",
			node: propagationNode{
				actorID:   "actor",
				trustPath: 0.8,
				depth:     1,
			},
			nodeScore: 0.9,
			// contribution = trustPath * nodeScore * propagationFactor * dampingFactor
			// = 0.8 * 0.9 * 1.0 * 0.85 = 0.612
			expected: 0.612,
		},
		{
			name: "contribution at depth 2",
			node: propagationNode{
				actorID:   "actor",
				trustPath: 0.8,
				depth:     2,
			},
			nodeScore: 0.9,
			// = 0.8 * 0.9 * 0.5 * 0.85 = 0.306
			expected: 0.306,
		},
		{
			name: "contribution at depth 3",
			node: propagationNode{
				actorID:   "actor",
				trustPath: 0.8,
				depth:     3,
			},
			nodeScore: 0.9,
			// = 0.8 * 0.9 * 0.25 * 0.85 = 0.153
			expected: 0.153,
		},
		{
			name: "zero node score",
			node: propagationNode{
				actorID:   "actor",
				trustPath: 0.8,
				depth:     1,
			},
			nodeScore: 0.0,
			expected:  0.0,
		},
		{
			name: "zero trust path",
			node: propagationNode{
				actorID:   "actor",
				trustPath: 0.0,
				depth:     1,
			},
			nodeScore: 0.9,
			expected:  0.0,
		},
		{
			name: "full trust path and score",
			node: propagationNode{
				actorID:   "actor",
				trustPath: 1.0,
				depth:     1,
			},
			nodeScore: 1.0,
			// = 1.0 * 1.0 * 1.0 * 0.85 = 0.85
			expected: 0.85,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := repo.calculateContribution(tt.node, tt.nodeScore, config)
			assert.InDelta(t, tt.expected, got, 0.0001)
		})
	}
}

// ============================================
// Test combineTrustScores
// ============================================

func TestUserRepository_combineTrustScores(t *testing.T) {
	repo := newMinimalUserRepo()

	tests := []struct {
		name          string
		inputScore    *storage.TrustScore
		expectedScore float64
	}{
		{
			name: "both direct and propagated scores",
			inputScore: &storage.TrustScore{
				DirectScore:     0.8,
				PropagatedScore: 0.6,
			},
			// = (0.8 * 0.7) + (0.6 * 0.3) = 0.56 + 0.18 = 0.74
			expectedScore: 0.74,
		},
		{
			name: "only direct score",
			inputScore: &storage.TrustScore{
				DirectScore:     0.9,
				PropagatedScore: 0.0,
			},
			expectedScore: 0.9,
		},
		{
			name: "only propagated score",
			inputScore: &storage.TrustScore{
				DirectScore:     0.0,
				PropagatedScore: 0.7,
			},
			expectedScore: 0.7,
		},
		{
			name: "both zero",
			inputScore: &storage.TrustScore{
				DirectScore:     0.0,
				PropagatedScore: 0.0,
			},
			expectedScore: 0.0,
		},
		{
			name: "clamping above 1.0",
			inputScore: &storage.TrustScore{
				DirectScore:     1.5,
				PropagatedScore: 1.5,
			},
			// Would calculate > 1.0, but should be clamped
			expectedScore: 1.0,
		},
		{
			name: "clamping below 0.0",
			inputScore: &storage.TrustScore{
				DirectScore:     -0.5,
				PropagatedScore: 0.0,
			},
			// Would be negative, but should be clamped to 0
			expectedScore: 0.0,
		},
		{
			name: "small but positive values",
			inputScore: &storage.TrustScore{
				DirectScore:     0.1,
				PropagatedScore: 0.1,
			},
			// = (0.1 * 0.7) + (0.1 * 0.3) = 0.07 + 0.03 = 0.10
			expectedScore: 0.1,
		},
		{
			name: "max values",
			inputScore: &storage.TrustScore{
				DirectScore:     1.0,
				PropagatedScore: 1.0,
			},
			// = (1.0 * 0.7) + (1.0 * 0.3) = 1.0
			expectedScore: 1.0,
		},
		{
			name: "high direct, low propagated",
			inputScore: &storage.TrustScore{
				DirectScore:     1.0,
				PropagatedScore: 0.1,
			},
			// = (1.0 * 0.7) + (0.1 * 0.3) = 0.7 + 0.03 = 0.73
			expectedScore: 0.73,
		},
		{
			name: "low direct, high propagated",
			inputScore: &storage.TrustScore{
				DirectScore:     0.1,
				PropagatedScore: 1.0,
			},
			// = (0.1 * 0.7) + (1.0 * 0.3) = 0.07 + 0.30 = 0.37
			expectedScore: 0.37,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Make a copy to avoid mutating test data
			score := *tt.inputScore
			repo.combineTrustScores(&score)
			assert.InDelta(t, tt.expectedScore, score.Score, 0.0001)

			// Verify bounds are always respected
			assert.GreaterOrEqual(t, score.Score, 0.0)
			assert.LessOrEqual(t, score.Score, 1.0)
		})
	}
}

// ============================================
// Test shouldProcessNode
// ============================================

func TestUserRepository_shouldProcessNode(t *testing.T) {
	repo := newMinimalUserRepo()

	tests := []struct {
		name     string
		node     propagationNode
		visited  map[string]bool
		maxDepth int
		expected bool
	}{
		{
			name:     "unvisited node within depth",
			node:     propagationNode{actorID: "actor1", depth: 2},
			visited:  map[string]bool{},
			maxDepth: 3,
			expected: true,
		},
		{
			name:     "visited node",
			node:     propagationNode{actorID: "actor1", depth: 2},
			visited:  map[string]bool{"actor1": true},
			maxDepth: 3,
			expected: false,
		},
		{
			name:     "node exceeds max depth",
			node:     propagationNode{actorID: "actor1", depth: 4},
			visited:  map[string]bool{},
			maxDepth: 3,
			expected: false,
		},
		{
			name:     "node at max depth",
			node:     propagationNode{actorID: "actor1", depth: 3},
			visited:  map[string]bool{},
			maxDepth: 3,
			expected: true,
		},
		{
			name:     "other actors visited but not this one",
			node:     propagationNode{actorID: "actor2", depth: 1},
			visited:  map[string]bool{"actor1": true, "actor3": true},
			maxDepth: 3,
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := repo.shouldProcessNode(tt.node, tt.visited, tt.maxDepth)
			assert.Equal(t, tt.expected, got)
		})
	}
}

// ============================================
// Test initializePropagationQueue
// ============================================

func TestUserRepository_initializePropagationQueue(t *testing.T) {
	repo := newMinimalUserRepo()

	tests := []struct {
		name          string
		trusterScores map[string]float64
		minTrustScore float64
		expectedLen   int
		checkContent  func(t *testing.T, queue []propagationNode)
	}{
		{
			name: "all scores above minimum",
			trusterScores: map[string]float64{
				"actor1": 0.8,
				"actor2": 0.9,
				"actor3": 0.5,
			},
			minTrustScore: 0.1,
			expectedLen:   3,
		},
		{
			name: "some scores below minimum",
			trusterScores: map[string]float64{
				"actor1": 0.8,
				"actor2": 0.05, // below min
				"actor3": 0.5,
			},
			minTrustScore: 0.1,
			expectedLen:   2,
		},
		{
			name: "all scores below minimum",
			trusterScores: map[string]float64{
				"actor1": 0.05,
				"actor2": 0.08,
			},
			minTrustScore: 0.1,
			expectedLen:   0,
		},
		{
			name:          "empty truster scores",
			trusterScores: map[string]float64{},
			minTrustScore: 0.1,
			expectedLen:   0,
		},
		{
			name:          "nil truster scores",
			trusterScores: nil,
			minTrustScore: 0.1,
			expectedLen:   0,
		},
		{
			name: "score exactly at minimum",
			trusterScores: map[string]float64{
				"actor1": 0.1,
			},
			minTrustScore: 0.1,
			expectedLen:   1,
		},
		{
			name: "verify node properties",
			trusterScores: map[string]float64{
				"actor1": 0.8,
			},
			minTrustScore: 0.1,
			expectedLen:   1,
			checkContent: func(t *testing.T, queue []propagationNode) {
				assert.Equal(t, "actor1", queue[0].actorID)
				assert.InDelta(t, 0.8, queue[0].trustPath, 0.0001)
				assert.Equal(t, 1, queue[0].depth)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			queue := repo.initializePropagationQueue(tt.trusterScores, tt.minTrustScore)
			assert.Len(t, queue, tt.expectedLen)
			if tt.checkContent != nil {
				tt.checkContent(t, queue)
			}
		})
	}
}

// ============================================
// Test defaultPropagationConfig
// ============================================

func TestUserRepository_defaultPropagationConfig(t *testing.T) {
	repo := newMinimalUserRepo()
	config := repo.defaultPropagationConfig()

	assert.InDelta(t, 0.85, config.dampingFactor, 0.001)
	assert.Equal(t, 3, config.maxDepth)
	assert.InDelta(t, 0.1, config.minTrustScore, 0.001)
	assert.InDelta(t, 0.5, config.propagationRate, 0.001)
	assert.Equal(t, 100, config.maxVisited)
}

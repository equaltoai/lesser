package repositories

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	"go.uber.org/zap"
)

// ============================================
// Test ListUsers
// ============================================

func TestUserRepository_ListUsers_Success(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewUserRepository(mockDB, "test-table", logger)

	ctx := context.Background()

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", &models.User{}).Return(mockQuery)
	mockQuery.On("Index", "gsi1").Return(mockQuery)
	mockQuery.On("Where", "gsi1PK", "=", "USERS").Return(mockQuery)
	mockQuery.On("Limit", 21).Return(mockQuery) // limit+1 for pagination detection
	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		out := args.Get(0).(*[]models.User)
		*out = []models.User{
			{Username: "alice", Email: "alice@example.com", Role: "user", SK: "KEY#alice"},
			{Username: "bob", Email: "bob@example.com", Role: "user", SK: "KEY#bob"},
			{Username: "charlie", Email: "charlie@example.com", Role: "admin", SK: "KEY#charlie"},
		}
	}).Return(nil)

	users, nextCursor, err := repo.ListUsers(ctx, 20, "")

	assert.NoError(t, err)
	assert.Len(t, users, 3)
	assert.Empty(t, nextCursor) // No more pages
	assert.Equal(t, "alice", users[0].Username)
	assert.Equal(t, "bob", users[1].Username)
	assert.Equal(t, "charlie", users[2].Username)
}

func TestUserRepository_ListUsers_WithPagination(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewUserRepository(mockDB, "test-table", logger)

	ctx := context.Background()

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", &models.User{}).Return(mockQuery)
	mockQuery.On("Index", "gsi1").Return(mockQuery)
	mockQuery.On("Where", "gsi1PK", "=", "USERS").Return(mockQuery)
	mockQuery.On("Limit", 3).Return(mockQuery) // limit+1 = 3
	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		// Return limit+1 items to indicate more pages
		out := args.Get(0).(*[]models.User)
		*out = []models.User{
			{Username: "alice", Email: "alice@example.com", SK: "KEY#A"},
			{Username: "bob", Email: "bob@example.com", SK: "KEY#B"},
			{Username: "charlie", Email: "charlie@example.com", SK: "KEY#C"}, // Extra item
		}
	}).Return(nil)

	users, nextCursor, err := repo.ListUsers(ctx, 2, "")

	assert.NoError(t, err)
	assert.Len(t, users, 3) // Results are truncated at the query level
	assert.NotEmpty(t, nextCursor)
}

func TestUserRepository_ListUsers_WithCursor(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewUserRepository(mockDB, "test-table", logger)

	ctx := context.Background()
	inputCursor := "KEY#bob"

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", &models.User{}).Return(mockQuery)
	mockQuery.On("Index", "gsi1").Return(mockQuery)
	mockQuery.On("Where", "gsi1PK", "=", "USERS").Return(mockQuery)
	mockQuery.On("Limit", 21).Return(mockQuery)
	mockQuery.On("Where", "gsi1SK", ">", inputCursor).Return(mockQuery) // Cursor condition
	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		out := args.Get(0).(*[]models.User)
		*out = []models.User{
			{Username: "charlie", Email: "charlie@example.com", SK: "KEY#C"},
			{Username: "diana", Email: "diana@example.com", SK: "KEY#D"},
		}
	}).Return(nil)

	users, _, err := repo.ListUsers(ctx, 20, inputCursor)

	assert.NoError(t, err)
	assert.Len(t, users, 2)
	assert.Equal(t, "charlie", users[0].Username)
	assert.Equal(t, "diana", users[1].Username)
}

func TestUserRepository_ListUsers_Empty(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewUserRepository(mockDB, "test-table", logger)

	ctx := context.Background()

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", &models.User{}).Return(mockQuery)
	mockQuery.On("Index", "gsi1").Return(mockQuery)
	mockQuery.On("Where", "gsi1PK", "=", "USERS").Return(mockQuery)
	mockQuery.On("Limit", 21).Return(mockQuery)
	mockQuery.On("All", mock.Anything).Return(nil) // Empty result

	users, nextCursor, err := repo.ListUsers(ctx, 20, "")

	assert.NoError(t, err)
	assert.Empty(t, users)
	assert.Empty(t, nextCursor)
}

func TestUserRepository_ListUsers_QueryError(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewUserRepository(mockDB, "test-table", logger)

	ctx := context.Background()

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", &models.User{}).Return(mockQuery)
	mockQuery.On("Index", "gsi1").Return(mockQuery)
	mockQuery.On("Where", "gsi1PK", "=", "USERS").Return(mockQuery)
	mockQuery.On("Limit", 21).Return(mockQuery)
	mockQuery.On("All", mock.Anything).Return(ErrTestMockError)

	users, nextCursor, err := repo.ListUsers(ctx, 20, "")

	assert.Error(t, err)
	assert.Nil(t, users)
	assert.Empty(t, nextCursor)
}

func TestUserRepository_ListUsers_InvalidLimitFallback(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewUserRepository(mockDB, "test-table", logger)

	ctx := context.Background()

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", &models.User{}).Return(mockQuery)
	mockQuery.On("Index", "gsi1").Return(mockQuery)
	mockQuery.On("Where", "gsi1PK", "=", "USERS").Return(mockQuery)
	// Limit should fallback to 20+1=21 when invalid limit provided
	mockQuery.On("Limit", 21).Return(mockQuery)
	mockQuery.On("All", mock.Anything).Return(nil)

	_, _, err := repo.ListUsers(ctx, -1, "") // Invalid negative limit

	assert.NoError(t, err)
}

func TestUserRepository_ListUsers_ExceedsMaxLimit(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewUserRepository(mockDB, "test-table", logger)

	ctx := context.Background()

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", &models.User{}).Return(mockQuery)
	mockQuery.On("Index", "gsi1").Return(mockQuery)
	mockQuery.On("Where", "gsi1PK", "=", "USERS").Return(mockQuery)
	// Limit should fallback to 20+1=21 when exceeds max (100)
	mockQuery.On("Limit", 21).Return(mockQuery)
	mockQuery.On("All", mock.Anything).Return(nil)

	_, _, err := repo.ListUsers(ctx, 500, "") // Exceeds max

	assert.NoError(t, err)
}

// ============================================
// Test GetTrustRelationships
// ============================================

func TestUserRepository_GetTrustRelationships_Success(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewUserRepository(mockDB, "test-table", logger)

	ctx := context.Background()

	mockDB.On("Model", mock.AnythingOfType("*models.TrustRelationship")).Return(mockQuery)
	mockQuery.On("Filter", "Type", "=", "RELATIONSHIP").Return(mockQuery)
	mockQuery.On("Limit", 20).Return(mockQuery) // limit * 2
	mockQuery.On("Scan", mock.Anything).Run(func(args mock.Arguments) {
		out := args.Get(0).(*[]*models.TrustRelationship)
		*out = []*models.TrustRelationship{
			{
				PK:        "TRUST#actor1#content",
				SK:        "TRUSTEE#actor2",
				TrusterID: "actor1",
				TrusteeID: "actor2",
				Category:  models.TrustCategoryContent,
				Score:     0.8,
				Created:   time.Now(),
			},
			{
				PK:        "TRUST#actor1#general",
				SK:        "TRUSTEE#actor3",
				TrusterID: "actor1",
				TrusteeID: "actor3",
				Category:  models.TrustCategoryGeneral,
				Score:     0.9,
				Created:   time.Now(),
			},
		}
	}).Return(nil)

	relationships, nextCursor, err := repo.GetTrustRelationships(ctx, "actor1", 10, "")

	assert.NoError(t, err)
	assert.Len(t, relationships, 2)
	assert.Empty(t, nextCursor)
	assert.Equal(t, "actor1", relationships[0].TrusterID)
	assert.Equal(t, "actor2", relationships[0].TrusteeID)
}

func TestUserRepository_GetTrustRelationships_FiltersByTruster(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewUserRepository(mockDB, "test-table", logger)

	ctx := context.Background()

	mockDB.On("Model", mock.AnythingOfType("*models.TrustRelationship")).Return(mockQuery)
	mockQuery.On("Filter", "Type", "=", "RELATIONSHIP").Return(mockQuery)
	mockQuery.On("Limit", 20).Return(mockQuery)
	mockQuery.On("Scan", mock.Anything).Run(func(args mock.Arguments) {
		out := args.Get(0).(*[]*models.TrustRelationship)
		*out = []*models.TrustRelationship{
			// This one matches actor1
			{
				PK:        "TRUST#actor1#content",
				SK:        "TRUSTEE#actor2",
				TrusterID: "actor1",
				TrusteeID: "actor2",
				Category:  models.TrustCategoryContent,
				Score:     0.8,
			},
			// This one does NOT match actor1 - different truster
			{
				PK:        "TRUST#otherActor#content",
				SK:        "TRUSTEE#actor3",
				TrusterID: "otherActor",
				TrusteeID: "actor3",
				Category:  models.TrustCategoryContent,
				Score:     0.7,
			},
		}
	}).Return(nil)

	relationships, _, err := repo.GetTrustRelationships(ctx, "actor1", 10, "")

	assert.NoError(t, err)
	assert.Len(t, relationships, 1)
	assert.Equal(t, "actor1", relationships[0].TrusterID)
}

func TestUserRepository_GetTrustRelationships_WithCursor(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewUserRepository(mockDB, "test-table", logger)

	ctx := context.Background()
	inputCursor := "someCursor"

	mockDB.On("Model", mock.AnythingOfType("*models.TrustRelationship")).Return(mockQuery)
	mockQuery.On("Filter", "Type", "=", "RELATIONSHIP").Return(mockQuery)
	mockQuery.On("Limit", 20).Return(mockQuery)
	mockQuery.On("Cursor", inputCursor).Return(mockQuery)
	mockQuery.On("Scan", mock.Anything).Return(nil)

	_, _, err := repo.GetTrustRelationships(ctx, "actor1", 10, inputCursor)

	assert.NoError(t, err)
	mockQuery.AssertCalled(t, "Cursor", inputCursor)
}

func TestUserRepository_GetTrustRelationships_ScanError(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewUserRepository(mockDB, "test-table", logger)

	ctx := context.Background()

	mockDB.On("Model", mock.AnythingOfType("*models.TrustRelationship")).Return(mockQuery)
	mockQuery.On("Filter", "Type", "=", "RELATIONSHIP").Return(mockQuery)
	mockQuery.On("Limit", 20).Return(mockQuery)
	mockQuery.On("Scan", mock.Anything).Return(ErrTestMockError)

	relationships, nextCursor, err := repo.GetTrustRelationships(ctx, "actor1", 10, "")

	assert.Error(t, err)
	assert.Nil(t, relationships)
	assert.Empty(t, nextCursor)
}

func TestUserRepository_GetTrustRelationships_LimitEnforced(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewUserRepository(mockDB, "test-table", logger)

	ctx := context.Background()

	mockDB.On("Model", mock.AnythingOfType("*models.TrustRelationship")).Return(mockQuery)
	mockQuery.On("Filter", "Type", "=", "RELATIONSHIP").Return(mockQuery)
	mockQuery.On("Limit", 10).Return(mockQuery) // limit * 2 = 10
	mockQuery.On("Scan", mock.Anything).Run(func(args mock.Arguments) {
		out := args.Get(0).(*[]*models.TrustRelationship)
		// Return more than limit to test enforcement
		rels := make([]*models.TrustRelationship, 8)
		for i := 0; i < 8; i++ {
			rels[i] = &models.TrustRelationship{
				PK:        "TRUST#actor1#content",
				TrusterID: "actor1",
				TrusteeID: "actor" + string(rune('a'+i)),
			}
		}
		*out = rels
	}).Return(nil)

	// Limit is 5, should only return up to 5
	relationships, _, err := repo.GetTrustRelationships(ctx, "actor1", 5, "")

	assert.NoError(t, err)
	assert.LessOrEqual(t, len(relationships), 5)
}

// ============================================
// Test GetTrustedByRelationships
// ============================================

func TestUserRepository_GetTrustedByRelationships_Success(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewUserRepository(mockDB, "test-table", logger)

	ctx := context.Background()

	mockDB.On("Model", mock.AnythingOfType("*models.TrustRelationship")).Return(mockQuery)
	mockQuery.On("Index", "gsi1").Return(mockQuery)
	mockQuery.On("Filter", "Type", "=", "RELATIONSHIP").Return(mockQuery)
	mockQuery.On("Limit", 20).Return(mockQuery)
	mockQuery.On("Scan", mock.Anything).Run(func(args mock.Arguments) {
		out := args.Get(0).(*[]*models.TrustRelationship)
		*out = []*models.TrustRelationship{
			{
				PK:        "TRUST#actor2#content",
				GSI1PK:    "TRUSTED#actor1#content",
				GSI1SK:    "TRUSTER#actor2",
				TrusterID: "actor2",
				TrusteeID: "actor1",
				Category:  models.TrustCategoryContent,
				Score:     0.8,
			},
		}
	}).Return(nil)

	relationships, nextCursor, err := repo.GetTrustedByRelationships(ctx, "actor1", 10, "")

	assert.NoError(t, err)
	assert.Len(t, relationships, 1)
	assert.Empty(t, nextCursor)
	assert.Equal(t, "actor2", relationships[0].TrusterID)
	assert.Equal(t, "actor1", relationships[0].TrusteeID)
}

func TestUserRepository_GetTrustedByRelationships_FiltersByTrustee(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewUserRepository(mockDB, "test-table", logger)

	ctx := context.Background()

	mockDB.On("Model", mock.AnythingOfType("*models.TrustRelationship")).Return(mockQuery)
	mockQuery.On("Index", "gsi1").Return(mockQuery)
	mockQuery.On("Filter", "Type", "=", "RELATIONSHIP").Return(mockQuery)
	mockQuery.On("Limit", 20).Return(mockQuery)
	mockQuery.On("Scan", mock.Anything).Run(func(args mock.Arguments) {
		out := args.Get(0).(*[]*models.TrustRelationship)
		*out = []*models.TrustRelationship{
			// This one matches actor1 as trustee
			{
				GSI1PK:    "TRUSTED#actor1#content",
				TrusterID: "actor2",
				TrusteeID: "actor1",
			},
			// This one does NOT match - different trustee
			{
				GSI1PK:    "TRUSTED#otherActor#content",
				TrusterID: "actor3",
				TrusteeID: "otherActor",
			},
		}
	}).Return(nil)

	relationships, _, err := repo.GetTrustedByRelationships(ctx, "actor1", 10, "")

	assert.NoError(t, err)
	assert.Len(t, relationships, 1)
	assert.Equal(t, "actor1", relationships[0].TrusteeID)
}

func TestUserRepository_GetTrustedByRelationships_ScanError(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewUserRepository(mockDB, "test-table", logger)

	ctx := context.Background()

	mockDB.On("Model", mock.AnythingOfType("*models.TrustRelationship")).Return(mockQuery)
	mockQuery.On("Index", "gsi1").Return(mockQuery)
	mockQuery.On("Filter", "Type", "=", "RELATIONSHIP").Return(mockQuery)
	mockQuery.On("Limit", 20).Return(mockQuery)
	mockQuery.On("Scan", mock.Anything).Return(ErrTestMockError)

	relationships, nextCursor, err := repo.GetTrustedByRelationships(ctx, "actor1", 10, "")

	assert.Error(t, err)
	assert.Nil(t, relationships)
	assert.Empty(t, nextCursor)
}

func TestUserRepository_GetTrustedByRelationships_WithCursor(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewUserRepository(mockDB, "test-table", logger)

	ctx := context.Background()
	inputCursor := "someCursor"

	mockDB.On("Model", mock.AnythingOfType("*models.TrustRelationship")).Return(mockQuery)
	mockQuery.On("Index", "gsi1").Return(mockQuery)
	mockQuery.On("Filter", "Type", "=", "RELATIONSHIP").Return(mockQuery)
	mockQuery.On("Limit", 20).Return(mockQuery)
	mockQuery.On("Cursor", inputCursor).Return(mockQuery)
	mockQuery.On("Scan", mock.Anything).Return(nil)

	_, _, err := repo.GetTrustedByRelationships(ctx, "actor1", 10, inputCursor)

	assert.NoError(t, err)
	mockQuery.AssertCalled(t, "Cursor", inputCursor)
}

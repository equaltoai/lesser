package testing

import (
	"context"
	"fmt"
	"testing"

	"github.com/aron23/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestNewTestDB(t *testing.T) {
	tableName := "test-table"
	testDB := NewTestDB(t, tableName)

	assert.NotNil(t, testDB)
	assert.Equal(t, tableName, testDB.GetTableName())
	assert.NotNil(t, testDB.GetClient())
	assert.Empty(t, testDB.createdData)
}

func TestTestDB_TrackCreatedItem(t *testing.T) {
	testDB := NewTestDB(t, "test")
	
	item1 := &models.User{Username: "test1"}
	item2 := &models.User{Username: "test2"}
	
	testDB.TrackCreatedItem(item1)
	testDB.TrackCreatedItem(item2)
	
	assert.Len(t, testDB.createdData, 2)
}

func TestTestDB_Cleanup(t *testing.T) {
	testDB := NewTestDB(t, "test")
	
	// Add some test data
	testDB.TrackCreatedItem(&models.User{Username: "test1"})
	testDB.TrackCreatedItem(&models.User{Username: "test2"})
	
	assert.Len(t, testDB.createdData, 2)
	
	// Cleanup
	testDB.Cleanup(t)
	
	// Data should be cleared (though not actually deleted from DB in mock)
	assert.Empty(t, testDB.createdData)
}

func TestNewFixtureBuilder(t *testing.T) {
	builder := NewFixtureBuilder()
	
	assert.NotNil(t, builder)
	assert.Empty(t, builder.users)
	assert.Empty(t, builder.statuses)
	assert.Empty(t, builder.follows)
	assert.Empty(t, builder.notifications)
	assert.Empty(t, builder.media)
	assert.Empty(t, builder.sessions)
	assert.Empty(t, builder.providers)
}

func TestFixtureBuilder_WithUser(t *testing.T) {
	builder := NewFixtureBuilder()
	
	result := builder.WithUser("testuser", "test@example.com")
	
	assert.Equal(t, builder, result) // Should return same builder for chaining
	assert.Len(t, builder.users, 1)
	
	user := builder.users[0]
	assert.Equal(t, "testuser", user.Username)
	assert.Equal(t, "test@example.com", user.Email)
	assert.Equal(t, "Display testuser", user.DisplayName)
	assert.True(t, user.Approved)
	assert.False(t, user.Suspended)
	assert.False(t, user.Silenced)
	assert.Equal(t, "user", user.Role)
	assert.Equal(t, "en", user.Locale)
}

func TestFixtureBuilder_WithStatus(t *testing.T) {
	builder := NewFixtureBuilder()
	
	result := builder.WithStatus("user123", "Hello, world!")
	
	assert.Equal(t, builder, result)
	assert.Len(t, builder.statuses, 1)
	
	status := builder.statuses[0]
	assert.Equal(t, "user123", status.AuthorID)
	assert.Equal(t, "Hello, world!", status.Content)
	assert.Equal(t, "public", status.Visibility)
}

func TestFixtureBuilder_WithFollow(t *testing.T) {
	builder := NewFixtureBuilder()
	
	result := builder.WithFollow("follower1", "followee1")
	
	assert.Equal(t, builder, result)
	assert.Len(t, builder.follows, 1)
	
	follow := builder.follows[0]
	assert.Equal(t, "follower1", follow["FollowerID"])
	assert.Equal(t, "followee1", follow["FolloweeID"])
	assert.NotNil(t, follow["CreatedAt"])
}

func TestFixtureBuilder_WithNotification(t *testing.T) {
	builder := NewFixtureBuilder()
	
	result := builder.WithNotification("user1", "actor1", "mention")
	
	assert.Equal(t, builder, result)
	assert.Len(t, builder.notifications, 1)
	
	notification := builder.notifications[0]
	assert.Equal(t, "user1", notification.UserID)
	assert.Equal(t, "actor1", notification.ActorID)
	assert.Equal(t, "mention", notification.Type)
	assert.False(t, notification.IsRead)
}

func TestFixtureBuilder_WithMedia(t *testing.T) {
	builder := NewFixtureBuilder()
	
	result := builder.WithMedia("user1", "test.jpg")
	
	assert.Equal(t, builder, result)
	assert.Len(t, builder.media, 1)
	
	media := builder.media[0]
	assert.Equal(t, "user1", media.UserID)
	assert.Equal(t, "test.jpg", media.FileName)
	assert.Equal(t, "image/jpeg", media.ContentType)
	assert.Equal(t, int64(1024*1024), media.FileSize)
	assert.Equal(t, "ready", media.Status)
}

func TestFixtureBuilder_WithSession(t *testing.T) {
	builder := NewFixtureBuilder()
	
	result := builder.WithSession("user1", "192.168.1.1")
	
	assert.Equal(t, builder, result)
	assert.Len(t, builder.sessions, 1)
	
	session := builder.sessions[0]
	assert.Equal(t, "user1", session.UserID)
	assert.Equal(t, "192.168.1.1", session.IPAddress)
	assert.Equal(t, "Test User Agent", session.UserAgent)
	assert.Equal(t, []string{"read", "write"}, session.Scopes)
	assert.False(t, session.IsRevoked)
}

func TestFixtureBuilder_WithProviderAccount(t *testing.T) {
	builder := NewFixtureBuilder()
	
	result := builder.WithProviderAccount("user1", "google", "google123")
	
	assert.Equal(t, builder, result)
	assert.Len(t, builder.providers, 1)
	
	provider := builder.providers[0]
	assert.Equal(t, "user1", provider.UserID)
	assert.Equal(t, "google", provider.Provider)
	assert.Equal(t, "google123", provider.ProviderID)
	assert.Equal(t, "google123@google.com", provider.Email)
	assert.True(t, provider.IsActive)
	assert.False(t, provider.IsPrimary)
}

func TestFixtureBuilder_Chaining(t *testing.T) {
	fixtures := NewFixtureBuilder().
		WithUser("user1", "user1@example.com").
		WithUser("user2", "user2@example.com").
		WithStatus("user1", "First status").
		WithStatus("user1", "Second status").
		WithFollow("user2", "user1").
		WithNotification("user1", "user2", "follow").
		Build()
	
	assert.Len(t, fixtures.Users, 2)
	assert.Len(t, fixtures.Statuses, 2)
	assert.Len(t, fixtures.Follows, 1)
	assert.Len(t, fixtures.Notifications, 1)
	assert.Empty(t, fixtures.Media)
	assert.Empty(t, fixtures.Sessions)
	assert.Empty(t, fixtures.ProviderAccounts)
}

func TestFixtures_Count(t *testing.T) {
	fixtures := NewFixtureBuilder().
		WithUser("user1", "user1@example.com").
		WithStatus("user1", "status1").
		WithFollow("user2", "user1").
		Build()
	
	assert.Equal(t, 3, fixtures.Count())
}

func TestFixtures_LoadIntoTestDB(t *testing.T) {
	testDB := NewTestDB(t, "test")
	fixtures := NewFixtureBuilder().
		WithUser("user1", "user1@example.com").
		WithStatus("user1", "status1").
		Build()
	
	fixtures.LoadIntoTestDB(t, testDB)
	
	// Should track all fixtures
	assert.Len(t, testDB.createdData, 2)
}

func TestNewMockRepository(t *testing.T) {
	repo := NewMockRepository()
	
	assert.NotNil(t, repo)
	assert.Empty(t, repo.data)
}

func TestMockRepository_Create(t *testing.T) {
	repo := NewMockRepository()
	ctx := context.Background()
	user := &models.User{Username: "testuser"}
	
	repo.On("Create", ctx, user).Return(nil)
	
	err := repo.Create(ctx, user)
	
	assert.NoError(t, err)
	data := repo.GetData()
	assert.Contains(t, data, "user:testuser")
	repo.AssertExpectations(t)
}

func TestMockRepository_Get(t *testing.T) {
	repo := NewMockRepository()
	ctx := context.Background()
	key := "testkey"
	var dest interface{}
	
	repo.On("Get", ctx, key, mock.AnythingOfType("*interface {}")).Return(nil)
	
	err := repo.Get(ctx, key, &dest)
	
	assert.NoError(t, err)
	repo.AssertExpectations(t)
}

func TestMockRepository_Update(t *testing.T) {
	repo := NewMockRepository()
	ctx := context.Background()
	user := &models.User{Username: "testuser"}
	
	repo.On("Update", ctx, user).Return(nil)
	
	err := repo.Update(ctx, user)
	
	assert.NoError(t, err)
	data := repo.GetData()
	assert.Contains(t, data, "user:testuser")
	repo.AssertExpectations(t)
}

func TestMockRepository_Delete(t *testing.T) {
	repo := NewMockRepository()
	ctx := context.Background()
	key := "testkey"
	
	// First store an item
	repo.storeItem(&models.User{Username: "testuser"})
	assert.Len(t, repo.GetData(), 1)
	
	repo.On("Delete", ctx, key).Return(nil)
	
	err := repo.Delete(ctx, key)
	
	assert.NoError(t, err)
	repo.AssertExpectations(t)
}

func TestMockRepository_extractKey(t *testing.T) {
	repo := NewMockRepository()
	
	user := &models.User{Username: "testuser"}
	key := repo.extractKey(user)
	assert.Equal(t, "user:testuser", key)
	
	status := &models.Status{StatusID: "status123"}
	key = repo.extractKey(status)
	assert.Equal(t, "status:status123", key)
	
	// Test unknown type
	other := "unknown"
	key = repo.extractKey(other)
	assert.Contains(t, key, "item:")
}

func TestMockRepository_Clear(t *testing.T) {
	repo := NewMockRepository()
	
	// Add some data
	repo.storeItem(&models.User{Username: "test1"})
	repo.storeItem(&models.User{Username: "test2"})
	assert.Len(t, repo.GetData(), 2)
	
	// Clear
	repo.Clear()
	assert.Empty(t, repo.GetData())
}

func TestAssertItemExists(t *testing.T) {
	repo := NewMockRepository()
	user := &models.User{Username: "testuser"}
	repo.storeItem(user)
	
	// This should pass
	AssertItemExists(t, repo, "user", "testuser")
}

func TestAssertItemNotExists(t *testing.T) {
	repo := NewMockRepository()
	
	// This should pass - item doesn't exist
	AssertItemNotExists(t, repo, "user", "nonexistent")
}

func TestAssertItemCount(t *testing.T) {
	repo := NewMockRepository()
	
	// Empty repo
	AssertItemCount(t, repo, 0)
	
	// Add items
	repo.storeItem(&models.User{Username: "user1"})
	repo.storeItem(&models.User{Username: "user2"})
	
	AssertItemCount(t, repo, 2)
}

func TestRepositoryTestSuite(t *testing.T) {
	suite := &RepositoryTestSuite{
		Name: "User",
		CreateItem: func() any {
			return &models.User{
				Username: "testuser",
				Email:    "test@example.com",
			}
		},
		UpdateItem: func(item any) any {
			user := item.(*models.User)
			user.DisplayName = "Updated Name"
			return user
		},
		GetKey: func(item any) string {
			user := item.(*models.User)
			return fmt.Sprintf("user:%s", user.Username)
		},
		ValidateItem: func(t *testing.T, item any) {
			user := item.(*models.User)
			assert.Equal(t, "testuser", user.Username)
		},
	}
	
	repo := NewMockRepository()
	suite.RunCRUDTests(t, repo)
}

func TestNewIntegrationTestContext(t *testing.T) {
	ctx := NewIntegrationTestContext(t, "test-table")
	
	assert.NotNil(t, ctx)
	assert.NotNil(t, ctx.TestDB)
	assert.NotNil(t, ctx.Logger)
	assert.NotNil(t, ctx.CostTracker)
	assert.Equal(t, "test-table", ctx.TestDB.GetTableName())
}

func TestIntegrationTestContext_SetupFixtures(t *testing.T) {
	ctx := NewIntegrationTestContext(t, "test")
	
	ctx.SetupFixtures(t, func(builder *FixtureBuilder) *FixtureBuilder {
		return builder.
			WithUser("user1", "user1@example.com").
			WithUser("user2", "user2@example.com").
			WithStatus("user1", "Hello world")
	})
	
	assert.NotNil(t, ctx.Fixtures)
	assert.Len(t, ctx.Fixtures.Users, 2)
	assert.Len(t, ctx.Fixtures.Statuses, 1)
	assert.Equal(t, 3, ctx.Fixtures.Count())
}

func TestIntegrationTestContext_Cleanup(t *testing.T) {
	ctx := NewIntegrationTestContext(t, "test")
	
	// Add some test data
	ctx.TestDB.TrackCreatedItem(&models.User{Username: "test"})
	assert.Len(t, ctx.TestDB.createdData, 1)
	
	// Cleanup should not panic and should clean up data
	ctx.Cleanup(t)
	assert.Empty(t, ctx.TestDB.createdData)
}

// Benchmark tests

func BenchmarkFixtureBuilder_WithUser(b *testing.B) {
	builder := NewFixtureBuilder()
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		builder.WithUser(fmt.Sprintf("user%d", i), fmt.Sprintf("user%d@example.com", i))
	}
}

func BenchmarkMockRepository_Create(b *testing.B) {
	repo := NewMockRepository()
	ctx := context.Background()
	
	// Set up mock to always succeed
	repo.On("Create", mock.Anything, mock.Anything).Return(nil)
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		user := &models.User{Username: fmt.Sprintf("user%d", i)}
		repo.Create(ctx, user)
	}
}

func BenchmarkMockRepository_GetData(b *testing.B) {
	repo := NewMockRepository()
	
	// Pre-populate with data
	for i := 0; i < 1000; i++ {
		repo.storeItem(&models.User{Username: fmt.Sprintf("user%d", i)})
	}
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		repo.GetData()
	}
}


// Package inmemory provides thread-safe in-memory implementations of repository interfaces.
package inmemory

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/core"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestUserRepositoryInterfaceCompleteness verifies that UserRepository
// implements all methods of the UserRepository interface.
// Feature: repository-interface-extraction, Property 1: Interface Method Completeness
func TestUserRepositoryInterfaceCompleteness(t *testing.T) {
	// Compile-time check that UserRepository implements UserRepository
	var _ interfaces.UserRepository = (*UserRepository)(nil)

	// Use reflection to verify all interface methods are implemented
	interfaceType := reflect.TypeOf((*interfaces.UserRepository)(nil)).Elem()
	implType := reflect.TypeOf(&UserRepository{})

	// Get all methods from the interface
	interfaceMethods := make(map[string]reflect.Method)
	for i := 0; i < interfaceType.NumMethod(); i++ {
		method := interfaceType.Method(i)
		interfaceMethods[method.Name] = method
	}

	// Verify each interface method exists in the implementation
	for name, interfaceMethod := range interfaceMethods {
		implMethod, found := implType.MethodByName(name)
		assert.True(t, found, "Method %s not found in UserRepository", name)

		if found {
			// Verify method signatures match (accounting for receiver)
			// Interface method type doesn't include receiver, impl does
			interfaceMethodType := interfaceMethod.Type
			implMethodType := implMethod.Type

			// Check number of inputs (impl has receiver as first param)
			assert.Equal(t, interfaceMethodType.NumIn(), implMethodType.NumIn()-1,
				"Method %s has wrong number of inputs", name)

			// Check number of outputs
			assert.Equal(t, interfaceMethodType.NumOut(), implMethodType.NumOut(),
				"Method %s has wrong number of outputs", name)
		}
	}

	t.Logf("Verified %d interface methods are implemented", len(interfaceMethods))
}

// TestUserRepositoryRoundTrip verifies that data stored in UserRepository
// can be retrieved with equivalent values.
// Feature: repository-interface-extraction, Property 5: In-Memory Round-Trip Consistency
func TestUserRepositoryRoundTrip(t *testing.T) {
	ctx := context.Background()
	repo := NewUserRepository()

	testCases := []struct {
		name string
		user *storage.User
	}{
		{
			name: "basic user",
			user: &storage.User{
				Username:    "testuser1",
				Email:       "test1@example.com",
				DisplayName: "Test User 1",
				Role:        "user",
				Approved:    true,
			},
		},
		{
			name: "user with all fields",
			user: &storage.User{
				Username:    "testuser2",
				Email:       "test2@example.com",
				DisplayName: "Test User 2",
				Role:        "admin",
				Approved:    true,
				Suspended:   false,
				Silenced:    false,
				Locale:      "en-US",
			},
		},
		{
			name: "user with empty optional fields",
			user: &storage.User{
				Username: "testuser3",
				Role:     "user",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Store the user
			err := repo.CreateUser(ctx, tc.user)
			require.NoError(t, err, "CreateUser should succeed")

			// Retrieve the user
			retrieved, err := repo.GetUser(ctx, tc.user.Username)
			require.NoError(t, err, "GetUser should succeed")

			// Verify key fields match
			assert.Equal(t, tc.user.Username, retrieved.Username)
			assert.Equal(t, tc.user.Email, retrieved.Email)
			assert.Equal(t, tc.user.DisplayName, retrieved.DisplayName)
			assert.Equal(t, tc.user.Role, retrieved.Role)
			assert.Equal(t, tc.user.Approved, retrieved.Approved)
			assert.Equal(t, tc.user.Suspended, retrieved.Suspended)
			assert.Equal(t, tc.user.Silenced, retrieved.Silenced)
			assert.Equal(t, tc.user.Locale, retrieved.Locale)

			// Verify timestamps are set
			assert.False(t, retrieved.CreatedAt.IsZero(), "CreatedAt should be set")
			assert.False(t, retrieved.UpdatedAt.IsZero(), "UpdatedAt should be set")
		})
	}
}

// TestUserRepositoryRoundTripByEmail verifies email-based retrieval
func TestUserRepositoryRoundTripByEmail(t *testing.T) {
	ctx := context.Background()
	repo := NewUserRepository()

	user := &storage.User{
		Username:    "emailuser",
		Email:       "unique@example.com",
		DisplayName: "Email User",
		Role:        "user",
	}

	err := repo.CreateUser(ctx, user)
	require.NoError(t, err)

	retrieved, err := repo.GetUserByEmail(ctx, user.Email)
	require.NoError(t, err)
	assert.Equal(t, user.Username, retrieved.Username)
	assert.Equal(t, user.Email, retrieved.Email)
}

// TestUserRepositoryVouchRoundTrip verifies vouch operations
func TestUserRepositoryVouchRoundTrip(t *testing.T) {
	ctx := context.Background()
	repo := NewUserRepository()

	vouch := &storage.Vouch{
		From:      "voucher",
		To:        "vouchee",
		Category:  "trust",
		Strength:  1.0,
		Active:    true,
		CreatedAt: time.Now(),
	}

	err := repo.CreateVouch(ctx, vouch)
	require.NoError(t, err)
	require.NotEmpty(t, vouch.ID, "Vouch ID should be generated")

	retrieved, err := repo.GetVouch(ctx, vouch.ID)
	require.NoError(t, err)
	assert.Equal(t, vouch.From, retrieved.From)
	assert.Equal(t, vouch.To, retrieved.To)
	assert.Equal(t, vouch.Category, retrieved.Category)
	assert.Equal(t, vouch.Active, retrieved.Active)
}

// TestUserRepositoryBookmarkRoundTrip verifies bookmark operations
func TestUserRepositoryBookmarkRoundTrip(t *testing.T) {
	ctx := context.Background()
	repo := NewUserRepository()

	username := "bookmarkuser"
	objectID := "object123"

	err := repo.CreateBookmark(ctx, username, objectID)
	require.NoError(t, err)

	isBookmarked, err := repo.IsBookmarked(ctx, username, objectID)
	require.NoError(t, err)
	assert.True(t, isBookmarked)

	bookmarks, _, err := repo.GetBookmarks(ctx, username, 10, "")
	require.NoError(t, err)
	assert.Contains(t, bookmarks, objectID)
}

// TestUserRepositoryPreferencesRoundTrip verifies preferences operations
func TestUserRepositoryPreferencesRoundTrip(t *testing.T) {
	ctx := context.Background()
	repo := NewUserRepository()

	username := "prefuser"

	// Set language preference
	err := repo.SetUserLanguagePreference(ctx, username, "fr")
	require.NoError(t, err)

	lang, err := repo.GetUserLanguagePreference(ctx, username)
	require.NoError(t, err)
	assert.Equal(t, "fr", lang)

	// Set individual preference
	err = repo.SetPreference(ctx, username, "theme", "dark")
	require.NoError(t, err)

	value, err := repo.GetPreference(ctx, username, "theme")
	require.NoError(t, err)
	assert.Equal(t, "dark", value)
}

// TestUserRepositoryThreadSafety verifies concurrent operations don't cause races.
// Feature: repository-interface-extraction, Property 6: In-Memory Thread Safety
func TestUserRepositoryThreadSafety(t *testing.T) {
	ctx := context.Background()
	repo := NewUserRepository()

	const numGoroutines = 50
	const numOperations = 100

	var wg sync.WaitGroup
	errChan := make(chan error, numGoroutines*numOperations)

	// Concurrent writes
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				user := &storage.User{
					Username:    generateUsername(id, j),
					Email:       generateEmail(id, j),
					DisplayName: "Test User",
					Role:        "user",
				}
				if err := repo.CreateUser(ctx, user); err != nil && !errors.Is(err, storage.ErrAlreadyExists) {
					errChan <- err
				}
			}
		}(i)
	}

	// Concurrent reads
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				username := generateUsername(id%10, j%10)
				_, _ = repo.GetUser(ctx, username)
			}
		}(i)
	}

	// Concurrent count operations
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				_, _ = repo.GetTotalUserCount(ctx)
				_, _ = repo.GetActiveUserCount(ctx, 30)
			}
		}()
	}

	wg.Wait()
	close(errChan)

	// Check for errors
	var errCount int
	for err := range errChan {
		t.Logf("Error during concurrent operation: %v", err)
		errCount++
	}

	assert.Zero(t, errCount, "Should have no errors during concurrent operations")
}

// TestUserRepositoryThreadSafetyVouches verifies concurrent vouch operations
func TestUserRepositoryThreadSafetyVouches(t *testing.T) {
	ctx := context.Background()
	repo := NewUserRepository()

	const numGoroutines = 20
	const numOperations = 50

	var wg sync.WaitGroup

	// Concurrent vouch creation
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				vouch := &storage.Vouch{
					From:      generateUsername(id, 0),
					To:        generateUsername(j%10, 0),
					Category:  "trust",
					Strength:  1.0,
					Active:    true,
					CreatedAt: time.Now(),
				}
				_ = repo.CreateVouch(ctx, vouch)
			}
		}(i)
	}

	// Concurrent vouch reads
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				actorID := generateUsername(id%10, 0)
				_, _ = repo.GetVouchesByActor(ctx, actorID, true)
				_, _ = repo.GetVouchesForActor(ctx, actorID, true)
			}
		}(i)
	}

	wg.Wait()
}

// Helper functions for generating test data
func generateUsername(id, seq int) string {
	return "user_" + string(rune('a'+id%26)) + "_" + string(rune('0'+seq%10))
}

func generateEmail(id, seq int) string {
	return generateUsername(id, seq) + "@example.com"
}

// TestUserRepositoryErrorCases verifies proper error handling
func TestUserRepositoryErrorCases(t *testing.T) {
	ctx := context.Background()
	repo := NewUserRepository()

	t.Run("GetUser not found", func(t *testing.T) {
		_, err := repo.GetUser(ctx, "nonexistent")
		assert.ErrorIs(t, err, storage.ErrNotFound)
	})

	t.Run("GetUserByEmail not found", func(t *testing.T) {
		_, err := repo.GetUserByEmail(ctx, "nonexistent@example.com")
		assert.ErrorIs(t, err, storage.ErrNotFound)
	})

	t.Run("CreateUser duplicate", func(t *testing.T) {
		user := &storage.User{Username: "duplicate", Email: "dup@example.com"}
		err := repo.CreateUser(ctx, user)
		require.NoError(t, err)

		err = repo.CreateUser(ctx, user)
		assert.ErrorIs(t, err, storage.ErrAlreadyExists)
	})

	t.Run("CreateUser duplicate email", func(t *testing.T) {
		user1 := &storage.User{Username: "user1", Email: "shared@example.com"}
		err := repo.CreateUser(ctx, user1)
		require.NoError(t, err)

		user2 := &storage.User{Username: "user2", Email: "shared@example.com"}
		err = repo.CreateUser(ctx, user2)
		assert.ErrorIs(t, err, storage.ErrAlreadyExists)
	})

	t.Run("UpdateUser not found", func(t *testing.T) {
		err := repo.UpdateUser(ctx, "nonexistent", map[string]any{"role": "admin"})
		assert.ErrorIs(t, err, storage.ErrNotFound)
	})

	t.Run("DeleteUser not found", func(t *testing.T) {
		err := repo.DeleteUser(ctx, "nonexistent")
		assert.ErrorIs(t, err, storage.ErrNotFound)
	})

	t.Run("GetVouch not found", func(t *testing.T) {
		_, err := repo.GetVouch(ctx, "nonexistent-vouch")
		assert.ErrorIs(t, err, storage.ErrNotFound)
	})

	t.Run("RemoveBookmark not found", func(t *testing.T) {
		err := repo.RemoveBookmark(ctx, "user", "nonexistent-object")
		assert.ErrorIs(t, err, storage.ErrNotFound)
	})
}

// TestRepositoryStorageUserReturnType verifies that RepositoryStorage.User()
// returns an interface type (interfaces.UserRepository), not a concrete pointer type.
// Feature: repository-interface-extraction, Property 2: Return Type Correctness
func TestRepositoryStorageUserReturnType(t *testing.T) {
	// Get the RepositoryStorage interface type from core package
	repoStorageType := reflect.TypeOf((*core.RepositoryStorage)(nil)).Elem()

	// Find the User() method
	userMethod, found := repoStorageType.MethodByName("User")
	require.True(t, found, "RepositoryStorage should have a User() method")

	// Get the return type of User()
	require.Equal(t, 1, userMethod.Type.NumOut(), "User() should return exactly one value")
	returnType := userMethod.Type.Out(0)

	// Verify the return type is an interface, not a pointer to a concrete type
	assert.Equal(t, reflect.Interface, returnType.Kind(),
		"User() should return an interface type, got %s", returnType.Kind())

	// Verify the return type is specifically interfaces.UserRepository
	expectedType := reflect.TypeOf((*interfaces.UserRepository)(nil)).Elem()
	assert.Equal(t, expectedType, returnType,
		"User() should return interfaces.UserRepository, got %s", returnType)

	t.Logf("Verified User() returns interface type: %s", returnType)
}

// TestConcreteUserRepositorySatisfiesInterface verifies that the concrete
// *repositories.UserRepository type satisfies the interfaces.UserRepository interface.
// This ensures backward compatibility - existing code using concrete types still works.
// Feature: repository-interface-extraction, Property 2: Return Type Correctness
func TestConcreteUserRepositorySatisfiesInterface(t *testing.T) {
	// This is a compile-time check - if it compiles, the concrete type satisfies the interface
	// We use a nil pointer since we don't need an actual instance for the type check
	var _ interfaces.UserRepository = (*UserRepository)(nil)

	// Use reflection to verify the concrete type implements all interface methods
	interfaceType := reflect.TypeOf((*interfaces.UserRepository)(nil)).Elem()
	concreteType := reflect.TypeOf(&UserRepository{})

	// Verify the concrete type implements the interface
	assert.True(t, concreteType.Implements(interfaceType),
		"*UserRepository should implement interfaces.UserRepository")

	t.Logf("Verified *UserRepository implements interfaces.UserRepository")
}

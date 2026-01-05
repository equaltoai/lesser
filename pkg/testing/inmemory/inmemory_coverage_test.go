// Package inmemory provides thread-safe in-memory implementations of repository interfaces.
package inmemory

import (
	"reflect"
	"testing"

	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestInMemoryImplementationCoverage verifies that all 10 Phase 2 repository interfaces
// have corresponding in-memory implementations.
// Feature: repository-interface-extraction, Property 4: In-Memory Implementation Coverage
// Validates: Requirements 3.1
func TestInMemoryImplementationCoverage(t *testing.T) {
	// Define the 10 Phase 2 repository interfaces and their corresponding in-memory types
	// Each entry maps an interface type to its in-memory implementation type
	// Phase 2 repositories (tasks 8.1-8.10):
	// 8.1 AccountRepository, 8.2 ActorRepository, 8.3 StatusRepository, 8.4 TimelineRepository,
	// 8.5 NotificationRepository, 8.6 RelationshipRepository, 8.7 ObjectRepository,
	// 8.8 ActivityRepository, 8.9 TrustRepository, 8.10 ModerationRepository
	phase2Repositories := []struct {
		name           string
		interfaceType  reflect.Type
		inMemoryType   reflect.Type
	}{
		{
			name:          "AccountRepository",
			interfaceType: reflect.TypeOf((*interfaces.AccountRepository)(nil)).Elem(),
			inMemoryType:  reflect.TypeOf(&AccountRepository{}),
		},
		{
			name:          "ActorRepository",
			interfaceType: reflect.TypeOf((*interfaces.ActorRepository)(nil)).Elem(),
			inMemoryType:  reflect.TypeOf(&ActorRepository{}),
		},
		{
			name:          "StatusRepository",
			interfaceType: reflect.TypeOf((*interfaces.StatusRepository)(nil)).Elem(),
			inMemoryType:  reflect.TypeOf(&StatusRepository{}),
		},
		{
			name:          "TimelineRepository",
			interfaceType: reflect.TypeOf((*interfaces.TimelineRepository)(nil)).Elem(),
			inMemoryType:  reflect.TypeOf(&TimelineRepository{}),
		},
		{
			name:          "NotificationRepository",
			interfaceType: reflect.TypeOf((*interfaces.NotificationRepository)(nil)).Elem(),
			inMemoryType:  reflect.TypeOf(&NotificationRepository{}),
		},
		{
			name:          "RelationshipRepository",
			interfaceType: reflect.TypeOf((*interfaces.ConcreteRelationshipRepository)(nil)).Elem(),
			inMemoryType:  reflect.TypeOf(&RelationshipRepository{}),
		},
		{
			name:          "ObjectRepository",
			interfaceType: reflect.TypeOf((*interfaces.ObjectRepository)(nil)).Elem(),
			inMemoryType:  reflect.TypeOf(&ObjectRepository{}),
		},
		{
			name:          "ActivityRepository",
			interfaceType: reflect.TypeOf((*interfaces.ActivityRepository)(nil)).Elem(),
			inMemoryType:  reflect.TypeOf(&ActivityRepository{}),
		},
		{
			name:          "TrustRepository",
			interfaceType: reflect.TypeOf((*interfaces.TrustRepository)(nil)).Elem(),
			inMemoryType:  reflect.TypeOf(&TrustRepository{}),
		},
		{
			name:          "ModerationRepository",
			interfaceType: reflect.TypeOf((*interfaces.ModerationRepository)(nil)).Elem(),
			inMemoryType:  reflect.TypeOf(&ModerationRepository{}),
		},
	}

	// Track coverage statistics
	var coveredCount int
	var totalMethods int

	for _, repo := range phase2Repositories {
		t.Run(repo.name, func(t *testing.T) {
			// Verify the in-memory type implements the interface
			assert.True(t, repo.inMemoryType.Implements(repo.interfaceType),
				"In-memory %s should implement %s interface", repo.inMemoryType.Name(), repo.name)

			// Count interface methods
			numMethods := repo.interfaceType.NumMethod()
			totalMethods += numMethods

			// Verify all interface methods are present in the in-memory implementation
			for i := 0; i < numMethods; i++ {
				method := repo.interfaceType.Method(i)
				inMemoryMethod, found := repo.inMemoryType.MethodByName(method.Name)

				if assert.True(t, found, "In-memory %s should have method %s", repo.inMemoryType.Name(), method.Name) {
					// Verify method signature matches (accounting for receiver)
					interfaceMethodType := method.Type
					inMemoryMethodType := inMemoryMethod.Type

					// Check number of inputs (in-memory has receiver as first param)
					assert.Equal(t, interfaceMethodType.NumIn(), inMemoryMethodType.NumIn()-1,
						"Method %s.%s has wrong number of inputs", repo.name, method.Name)

					// Check number of outputs
					assert.Equal(t, interfaceMethodType.NumOut(), inMemoryMethodType.NumOut(),
						"Method %s.%s has wrong number of outputs", repo.name, method.Name)
				}
			}

			coveredCount++
			t.Logf("Verified in-memory for %s implements %d methods", repo.name, numMethods)
		})
	}

	// Summary assertion
	require.Equal(t, 10, coveredCount, "All 10 Phase 2 repository interfaces should have in-memory implementations")
	t.Logf("In-memory coverage complete: %d interfaces, %d total methods verified", coveredCount, totalMethods)
}

// TestInMemoryImplementationCompileTimeChecks provides compile-time verification
// that all in-memory types implement their corresponding interfaces.
// Feature: repository-interface-extraction, Property 4: In-Memory Implementation Coverage
// Validates: Requirements 3.1
func TestInMemoryImplementationCompileTimeChecks(t *testing.T) {
	// These are compile-time checks - if the code compiles, the in-memory implementations implement the interfaces
	// Using nil pointers since we don't need actual instances for type checking

	// Phase 2 repositories (10 total - tasks 8.1-8.10)
	var _ interfaces.AccountRepository = (*AccountRepository)(nil)
	var _ interfaces.ActorRepository = (*ActorRepository)(nil)
	var _ interfaces.StatusRepository = (*StatusRepository)(nil)
	var _ interfaces.TimelineRepository = (*TimelineRepository)(nil)
	var _ interfaces.NotificationRepository = (*NotificationRepository)(nil)
	var _ interfaces.ConcreteRelationshipRepository = (*RelationshipRepository)(nil)
	var _ interfaces.ObjectRepository = (*ObjectRepository)(nil)
	var _ interfaces.ActivityRepository = (*ActivityRepository)(nil)
	var _ interfaces.TrustRepository = (*TrustRepository)(nil)
	var _ interfaces.ModerationRepository = (*ModerationRepository)(nil)

	t.Log("All 10 Phase 2 in-memory implementations pass compile-time interface checks")
}

// TestInMemoryThreadSafetyStructure verifies that all in-memory implementations
// have the required sync.RWMutex field for thread safety.
// Feature: repository-interface-extraction, Property 4: In-Memory Implementation Coverage
// Validates: Requirements 3.1, 3.4
func TestInMemoryThreadSafetyStructure(t *testing.T) {
	inMemoryTypes := []struct {
		name         string
		inMemoryType reflect.Type
	}{
		{"AccountRepository", reflect.TypeOf(AccountRepository{})},
		{"ActorRepository", reflect.TypeOf(ActorRepository{})},
		{"StatusRepository", reflect.TypeOf(StatusRepository{})},
		{"TimelineRepository", reflect.TypeOf(TimelineRepository{})},
		{"NotificationRepository", reflect.TypeOf(NotificationRepository{})},
		{"RelationshipRepository", reflect.TypeOf(RelationshipRepository{})},
		{"ObjectRepository", reflect.TypeOf(ObjectRepository{})},
		{"ActivityRepository", reflect.TypeOf(ActivityRepository{})},
		{"TrustRepository", reflect.TypeOf(TrustRepository{})},
		{"ModerationRepository", reflect.TypeOf(ModerationRepository{})},
	}

	for _, mt := range inMemoryTypes {
		t.Run(mt.name, func(t *testing.T) {
			// Verify the in-memory type has a mu field of type sync.RWMutex
			muField, found := mt.inMemoryType.FieldByName("mu")
			require.True(t, found, "%s should have mu field for thread safety", mt.name)
			assert.Equal(t, "sync.RWMutex", muField.Type.String(), "%s.mu should be sync.RWMutex", mt.name)
		})
	}

	t.Log("All 10 Phase 2 in-memory implementations have sync.RWMutex for thread safety")
}

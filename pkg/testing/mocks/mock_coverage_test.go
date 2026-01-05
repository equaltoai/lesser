// Package mocks provides mock implementations for testing.
package mocks

import (
	"reflect"
	"testing"

	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMockImplementationCoverage verifies that all 10 Phase 2 repository interfaces
// have corresponding mock implementations.
// Feature: repository-interface-extraction, Property 3: Mock Implementation Coverage
// Validates: Requirements 2.1
func TestMockImplementationCoverage(t *testing.T) {
	// Define the 10 Phase 2 repository interfaces and their corresponding mock types
	// Each entry maps an interface type to its mock implementation type
	// Note: Some mocks use "Interface" suffix to distinguish from older legacy mocks
	// Phase 2 repositories (tasks 8.1-8.10):
	// 8.1 AccountRepository, 8.2 ActorRepository, 8.3 StatusRepository, 8.4 TimelineRepository,
	// 8.5 NotificationRepository, 8.6 RelationshipRepository, 8.7 ObjectRepository,
	// 8.8 ActivityRepository, 8.9 TrustRepository, 8.10 ModerationRepository
	phase2Repositories := []struct {
		name          string
		interfaceType reflect.Type
		mockType      reflect.Type
	}{
		{
			name:          "AccountRepository",
			interfaceType: reflect.TypeOf((*interfaces.AccountRepository)(nil)).Elem(),
			mockType:      reflect.TypeOf(&MockAccountRepository{}),
		},
		{
			name:          "ActorRepository",
			interfaceType: reflect.TypeOf((*interfaces.ActorRepository)(nil)).Elem(),
			mockType:      reflect.TypeOf(&MockActorRepository{}),
		},
		{
			name:          "StatusRepository",
			interfaceType: reflect.TypeOf((*interfaces.StatusRepository)(nil)).Elem(),
			mockType:      reflect.TypeOf(&MockStatusRepositoryInterface{}),
		},
		{
			name:          "TimelineRepository",
			interfaceType: reflect.TypeOf((*interfaces.TimelineRepository)(nil)).Elem(),
			mockType:      reflect.TypeOf(&MockTimelineRepositoryInterface{}),
		},
		{
			name:          "NotificationRepository",
			interfaceType: reflect.TypeOf((*interfaces.NotificationRepository)(nil)).Elem(),
			mockType:      reflect.TypeOf(&MockNotificationRepository{}),
		},
		{
			name:          "RelationshipRepository",
			interfaceType: reflect.TypeOf((*interfaces.ConcreteRelationshipRepository)(nil)).Elem(),
			mockType:      reflect.TypeOf(&MockRelationshipRepository{}),
		},
		{
			name:          "ObjectRepository",
			interfaceType: reflect.TypeOf((*interfaces.ObjectRepository)(nil)).Elem(),
			mockType:      reflect.TypeOf(&MockObjectRepository{}),
		},
		{
			name:          "ActivityRepository",
			interfaceType: reflect.TypeOf((*interfaces.ActivityRepository)(nil)).Elem(),
			mockType:      reflect.TypeOf(&MockActivityRepository{}),
		},
		{
			name:          "TrustRepository",
			interfaceType: reflect.TypeOf((*interfaces.TrustRepository)(nil)).Elem(),
			mockType:      reflect.TypeOf(&MockTrustRepository{}),
		},
		{
			name:          "ModerationRepository",
			interfaceType: reflect.TypeOf((*interfaces.ModerationRepository)(nil)).Elem(),
			mockType:      reflect.TypeOf(&MockModerationRepository{}),
		},
	}

	// Track coverage statistics
	var coveredCount int
	var totalMethods int

	for _, repo := range phase2Repositories {
		t.Run(repo.name, func(t *testing.T) {
			// Verify the mock type implements the interface
			assert.True(t, repo.mockType.Implements(repo.interfaceType),
				"Mock %s should implement %s interface", repo.mockType.Name(), repo.name)

			// Count interface methods
			numMethods := repo.interfaceType.NumMethod()
			totalMethods += numMethods

			// Verify all interface methods are present in the mock
			for i := 0; i < numMethods; i++ {
				method := repo.interfaceType.Method(i)
				mockMethod, found := repo.mockType.MethodByName(method.Name)
				
				if assert.True(t, found, "Mock %s should have method %s", repo.mockType.Name(), method.Name) {
					// Verify method signature matches (accounting for receiver)
					interfaceMethodType := method.Type
					mockMethodType := mockMethod.Type

					// Check number of inputs (mock has receiver as first param)
					assert.Equal(t, interfaceMethodType.NumIn(), mockMethodType.NumIn()-1,
						"Method %s.%s has wrong number of inputs", repo.name, method.Name)

					// Check number of outputs
					assert.Equal(t, interfaceMethodType.NumOut(), mockMethodType.NumOut(),
						"Method %s.%s has wrong number of outputs", repo.name, method.Name)
				}
			}

			coveredCount++
			t.Logf("Verified mock for %s implements %d methods", repo.name, numMethods)
		})
	}

	// Summary assertion
	require.Equal(t, 10, coveredCount, "All 10 Phase 2 repository interfaces should have mocks")
	t.Logf("Mock coverage complete: %d interfaces, %d total methods verified", coveredCount, totalMethods)
}

// TestMockImplementationCompileTimeChecks provides compile-time verification
// that all mock types implement their corresponding interfaces.
// Feature: repository-interface-extraction, Property 3: Mock Implementation Coverage
// Validates: Requirements 2.1
func TestMockImplementationCompileTimeChecks(t *testing.T) {
	// These are compile-time checks - if the code compiles, the mocks implement the interfaces
	// Using nil pointers since we don't need actual instances for type checking

	// Phase 2 repositories (10 total - tasks 8.1-8.10)
	var _ interfaces.AccountRepository = (*MockAccountRepository)(nil)
	var _ interfaces.ActorRepository = (*MockActorRepository)(nil)
	var _ interfaces.StatusRepository = (*MockStatusRepositoryInterface)(nil)
	var _ interfaces.TimelineRepository = (*MockTimelineRepositoryInterface)(nil)
	var _ interfaces.NotificationRepository = (*MockNotificationRepository)(nil)
	var _ interfaces.ConcreteRelationshipRepository = (*MockRelationshipRepository)(nil)
	var _ interfaces.ObjectRepository = (*MockObjectRepository)(nil)
	var _ interfaces.ActivityRepository = (*MockActivityRepository)(nil)
	var _ interfaces.TrustRepository = (*MockTrustRepository)(nil)
	var _ interfaces.ModerationRepository = (*MockModerationRepository)(nil)

	t.Log("All 10 Phase 2 mock implementations pass compile-time interface checks")
}

// TestMockTestifyIntegration verifies that all mocks properly embed testify/mock.Mock
// and can be used for expectation-based testing.
// Feature: repository-interface-extraction, Property 3: Mock Implementation Coverage
// Validates: Requirements 2.2, 2.3, 2.4
func TestMockTestifyIntegration(t *testing.T) {
	mockTypes := []struct {
		name     string
		mockType reflect.Type
	}{
		{"MockAccountRepository", reflect.TypeOf(MockAccountRepository{})},
		{"MockActorRepository", reflect.TypeOf(MockActorRepository{})},
		{"MockStatusRepositoryInterface", reflect.TypeOf(MockStatusRepositoryInterface{})},
		{"MockTimelineRepositoryInterface", reflect.TypeOf(MockTimelineRepositoryInterface{})},
		{"MockNotificationRepository", reflect.TypeOf(MockNotificationRepository{})},
		{"MockRelationshipRepository", reflect.TypeOf(MockRelationshipRepository{})},
		{"MockObjectRepository", reflect.TypeOf(MockObjectRepository{})},
		{"MockActivityRepository", reflect.TypeOf(MockActivityRepository{})},
		{"MockTrustRepository", reflect.TypeOf(MockTrustRepository{})},
		{"MockModerationRepository", reflect.TypeOf(MockModerationRepository{})},
	}

	for _, mt := range mockTypes {
		t.Run(mt.name, func(t *testing.T) {
			// Verify the mock type has an embedded mock.Mock field
			mockField, found := mt.mockType.FieldByName("Mock")
			require.True(t, found, "%s should embed mock.Mock", mt.name)
			assert.Equal(t, "Mock", mockField.Name, "%s should have Mock field", mt.name)
			assert.True(t, mockField.Anonymous, "%s.Mock should be an embedded field", mt.name)
		})
	}

	t.Log("All 10 Phase 2 mocks properly embed testify/mock.Mock")
}

// Package interfaces defines repository interfaces for the Lesser application.
//
// This package provides interface definitions for all repositories, enabling:
//   - Mock implementations for unit testing
//   - In-memory implementations for integration testing
//   - Decoupling of business logic from storage implementation details
//
// The interfaces follow the pattern where each repository interface mirrors
// the public methods of its concrete DynamoDB-backed implementation in
// pkg/storage/repositories/, allowing the concrete types to satisfy these
// interfaces through Go's implicit interface implementation.
//
// Usage:
//
//	// In production code, use the concrete implementation
//	storage := adapters.NewDynamORMStorage(db, tableName, logger)
//	userRepo := storage.User() // Returns interfaces.UserRepository
//
//	// In tests, use mock or in-memory implementations
//	mockRepo := mocks.NewMockUserRepository()
//	mockRepo.On("GetUser", mock.Anything, "testuser").Return(user, nil)
package interfaces //nolint:revive // Standard interfaces package name

// Package inmemory provides thread-safe in-memory implementations of repository interfaces.
//
// These implementations are designed for integration-style testing where you need
// realistic storage behavior without the overhead of DynamoDB. Unlike mocks,
// in-memory repositories maintain state across operations, making them suitable for:
//   - Testing complex workflows that span multiple repository operations
//   - Integration tests that verify data persistence and retrieval
//   - Performance testing without network latency
//
// All implementations are thread-safe using sync.RWMutex, allowing concurrent
// read/write operations from multiple goroutines without data races.
//
// Usage:
//
//	// Create an in-memory repository
//	userRepo := inmemory.NewUserRepository()
//
//	// Use it like a real repository
//	err := userRepo.CreateUser(ctx, user)
//	retrieved, err := userRepo.GetUser(ctx, user.Username)
//
//	// Use with MockRepositoryStorage for full storage mocking
//	storage := testing.NewMockRepositoryStorage(
//	    testing.WithUserRepository(userRepo),
//	)
package inmemory

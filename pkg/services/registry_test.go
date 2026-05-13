package services

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/equaltoai/lesser/pkg/streaming"
	dynamormCore "github.com/theory-cloud/tabletheory/pkg/core"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
)

// mockStorage implements RepositoryStorage for testing
type mockStorage struct {
	closed bool
	mu     sync.RWMutex
}

func newMockStorage() *mockStorage {
	return &mockStorage{}
}

func (m *mockStorage) Account() *repositories.AccountRepository                         { return nil }
func (m *mockStorage) Bookmark() *repositories.BookmarkRepository                       { return nil }
func (m *mockStorage) Actor() interfaces.ActorRepository                                { return nil }
func (m *mockStorage) Object() interfaces.ObjectRepository                              { return nil }
func (m *mockStorage) Activity() interfaces.ActivityRepository                          { return nil }
func (m *mockStorage) Timeline() interfaces.TimelineRepository                          { return nil }
func (m *mockStorage) Notification() interfaces.NotificationRepository                  { return nil }
func (m *mockStorage) Like() *repositories.LikeRepository                               { return nil }
func (m *mockStorage) Moderation() interfaces.ModerationRepository                      { return nil }
func (m *mockStorage) List() *repositories.ListRepository                               { return nil }
func (m *mockStorage) Media() *repositories.MediaRepository                             { return nil }
func (m *mockStorage) Poll() *repositories.PollRepository                               { return nil }
func (m *mockStorage) PushSubscription() *repositories.PushSubscriptionRepository       { return nil }
func (m *mockStorage) Hashtag() *repositories.HashtagRepository                         { return nil }
func (m *mockStorage) ScheduledStatus() *repositories.ScheduledStatusRepository         { return nil }
func (m *mockStorage) Announcement() *repositories.AnnouncementRepository               { return nil }
func (m *mockStorage) DomainBlock() *repositories.DomainBlockRepository                 { return nil }
func (m *mockStorage) Relationship() interfaces.ConcreteRelationshipRepository          { return nil }
func (m *mockStorage) Instance() *repositories.InstanceRepository                       { return nil }
func (m *mockStorage) Federation() *repositories.FederationRepository                   { return nil }
func (m *mockStorage) Recovery() *repositories.RecoveryRepository                       { return nil }
func (m *mockStorage) Analytics() *repositories.TrendingRepository                      { return nil }
func (m *mockStorage) Social() *repositories.SocialRepository                           { return nil }
func (m *mockStorage) User() interfaces.UserRepository                                  { return nil }
func (m *mockStorage) Status() interfaces.StatusRepository                              { return nil }
func (m *mockStorage) Cost() *repositories.TrackingRepository                           { return nil }
func (m *mockStorage) WebSocketCost() *repositories.WebSocketCostRepository             { return nil }
func (m *mockStorage) Trust() interfaces.TrustRepository                                { return nil }
func (m *mockStorage) Search() *repositories.SearchRepository                           { return nil }
func (m *mockStorage) Relay() *repositories.RelayRepository                             { return nil }
func (m *mockStorage) CommunityNote() *repositories.CommunityNoteRepository             { return nil }
func (m *mockStorage) Emoji() *repositories.EmojiRepository                             { return nil }
func (m *mockStorage) RateLimit() *repositories.RateLimitRepository                     { return nil }
func (m *mockStorage) Conversation() *repositories.ConversationRepository               { return nil }
func (m *mockStorage) Marker() *repositories.MarkerRepository                           { return nil }
func (m *mockStorage) FeaturedTag() *repositories.FeaturedTagRepository                 { return nil }
func (m *mockStorage) AI() *repositories.AIRepository                                   { return nil }
func (m *mockStorage) Export() *repositories.ExportRepository                           { return nil }
func (m *mockStorage) Import() *repositories.ImportRepository                           { return nil }
func (m *mockStorage) DLQ() *repositories.DLQRepository                                 { return nil }
func (m *mockStorage) MetricRecord() *repositories.MetricRecordRepository               { return nil }
func (m *mockStorage) CloudWatchMetrics() *repositories.CloudWatchMetricsRepository     { return nil }
func (m *mockStorage) Audit() *repositories.AuditRepository                             { return nil }
func (m *mockStorage) MediaMetadata() *repositories.MediaMetadataRepository             { return nil }
func (m *mockStorage) OAuth() *repositories.OAuthRepository                             { return nil }
func (m *mockStorage) Skill() interfaces.SkillRepository                                { return nil }
func (m *mockStorage) StreamingCloudWatch() *repositories.StreamingCloudWatchRepository { return nil }
func (m *mockStorage) DNSCache() *repositories.DNSCacheRepository                       { return nil }
func (m *mockStorage) Filter() *repositories.FilterRepository                           { return nil }
func (m *mockStorage) Thread() *repositories.ThreadRepository                           { return nil }
func (m *mockStorage) Severance() *repositories.SeveranceRepository                     { return nil }
func (m *mockStorage) ModerationML() *repositories.ModerationMLRepository               { return nil }
func (m *mockStorage) Quote() *repositories.QuoteRepository                             { return nil }
func (m *mockStorage) MediaAnalytics() interfaces.MediaAnalyticsRepository              { return nil }
func (m *mockStorage) MediaPopularity() interfaces.MediaPopularityRepository            { return nil }
func (m *mockStorage) MediaSession() interfaces.MediaSessionRepository                  { return nil }
func (m *mockStorage) StreamingConnection() interfaces.StreamingConnectionRepository    { return nil }
func (m *mockStorage) Article() interfaces.ArticleRepository                            { return nil }
func (m *mockStorage) Draft() interfaces.DraftRepository                                { return nil }
func (m *mockStorage) Revision() interfaces.RevisionRepository                          { return nil }
func (m *mockStorage) Series() interfaces.SeriesRepository                              { return nil }
func (m *mockStorage) Category() interfaces.CategoryRepository                          { return nil }
func (m *mockStorage) Publication() interfaces.PublicationRepository                    { return nil }
func (m *mockStorage) PublicationMember() interfaces.PublicationMemberRepository        { return nil }
func (m *mockStorage) GetDB() dynamormCore.DB                                           { return nil }
func (m *mockStorage) GetTableName() string                                             { return "test-table" }
func (m *mockStorage) GetLogger() *zap.Logger                                           { return zap.NewNop() }

func (m *mockStorage) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	return nil
}

func (m *mockStorage) IsClosed() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.closed
}

// mockPublisher implements streaming.Publisher for testing
type mockPublisher struct {
	events      []streaming.Event
	closed      bool
	shouldError bool
	errorMsg    string
	closeErr    error
	mu          sync.RWMutex
}

func newMockPublisher() *mockPublisher {
	return &mockPublisher{
		events: make([]streaming.Event, 0),
	}
}

func (m *mockPublisher) PublishToUser(ctx context.Context, userID string, event *streaming.Event) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return errors.New("publisher is closed")
	}
	if m.shouldError {
		return errors.New(m.errorMsg)
	}

	m.events = append(m.events, *event)
	return nil
}

func (m *mockPublisher) PublishToStream(ctx context.Context, streamName string, event *streaming.Event) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return errors.New("publisher is closed")
	}
	if m.shouldError {
		return errors.New(m.errorMsg)
	}

	m.events = append(m.events, *event)
	return nil
}

func (m *mockPublisher) PublishToConversation(ctx context.Context, conversationID string, event *streaming.Event) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return errors.New("publisher is closed")
	}
	if m.shouldError {
		return errors.New(m.errorMsg)
	}

	m.events = append(m.events, *event)
	return nil
}

func (m *mockPublisher) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	return m.closeErr
}

func (m *mockPublisher) SetError(shouldError bool, msg string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.shouldError = shouldError
	m.errorMsg = msg
}

func (m *mockPublisher) SetCloseError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closeErr = err
}

func (m *mockPublisher) GetEvents() []streaming.Event {
	m.mu.RLock()
	defer m.mu.RUnlock()
	events := make([]streaming.Event, len(m.events))
	copy(events, m.events)
	return events
}

func (m *mockPublisher) IsClosed() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.closed
}

// mockConfig returns a test configuration
func mockConfig() *ServiceConfig {
	return &ServiceConfig{
		BaseURL:   "https://test.example.com",
		JWTSecret: "test-jwt-secret-key",
	}
}

func TestNewRegistry_Success(t *testing.T) {
	storage := newMockStorage()
	publisher := newMockPublisher()
	logger := zaptest.NewLogger(t)
	config := mockConfig()

	registry, err := NewRegistry(
		WithStorage(storage),
		WithPublisher(publisher),
		WithLogger(logger),
		WithConfig(config),
	)

	if err != nil {
		t.Fatalf("Failed to create registry: %v", err)
	}

	if registry == nil {
		t.Fatal("Registry should not be nil")
	}

	// Verify dependencies are set (can't directly compare interfaces, check if not nil)
	if registry.GetStorage() == nil {
		t.Error("Storage not set correctly")
	}
	if registry.GetPublisher() != publisher {
		t.Error("Publisher not set correctly")
	}
	if registry.GetLogger() != logger {
		t.Error("Logger not set correctly")
	}
	if !reflect.DeepEqual(registry.GetConfig(), config) {
		t.Error("Config not set correctly")
	}
}

func TestNewRegistry_MinimalConfiguration(t *testing.T) {
	storage := newMockStorage()

	registry, err := NewRegistry(WithStorage(storage))
	if err != nil {
		t.Fatalf("Failed to create registry with minimal config: %v", err)
	}

	// Should have defaults set
	if registry.GetLogger() == nil {
		t.Error("Logger should have default value")
	}
	if registry.GetConfig() == nil {
		t.Error("Config should have default value")
	}
	if registry.GetPublisher() != nil {
		t.Error("Publisher should be nil when not provided")
	}
}

func TestNewRegistry_MissingStorage(t *testing.T) {
	logger := zaptest.NewLogger(t)

	_, err := NewRegistry(WithLogger(logger))
	if err == nil {
		t.Fatal("Expected error when storage is missing")
	}

	if !containsString(strings.ToLower(err.Error()), "registry validation failed") {
		t.Errorf("Expected registry validation error, got: %v", err)
	}
}

func TestNewRegistry_NilDependencies(t *testing.T) {
	tests := []struct {
		name    string
		option  RegistryOption
		wantErr string
	}{
		{
			name:    "nil storage",
			option:  WithStorage(nil),
			wantErr: "Failed to apply registry option",
		},
		{
			name:    "nil publisher",
			option:  WithPublisher(nil),
			wantErr: "Failed to apply registry option",
		},
		{
			name:    "nil logger",
			option:  WithLogger(nil),
			wantErr: "Failed to apply registry option",
		},
		{
			name:    "nil config",
			option:  WithConfig(nil),
			wantErr: "Failed to apply registry option",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewRegistry(tt.option)
			if err == nil {
				t.Fatalf("Expected error for %s", tt.name)
			}
			if !containsString(strings.ToLower(err.Error()), strings.ToLower(tt.wantErr)) {
				t.Errorf("Expected error to contain '%s', got: %v", tt.wantErr, err)
			}
		})
	}
}

func TestRegistry_ServiceInitialization(t *testing.T) {
	storage := newMockStorage()
	registry, err := NewRegistry(WithStorage(storage))
	if err != nil {
		t.Fatalf("Failed to create registry: %v", err)
	}

	// Initially no services should be initialized
	initialized := registry.GetInitializedServices()
	if len(initialized) != 0 {
		t.Errorf("Expected no initialized services, got: %v", initialized)
	}

	// Access a service to initialize it
	validation := registry.Validation()
	if validation == nil {
		t.Fatal("Validation service should not be nil")
	}

	// Check that it's tracked as initialized
	initialized = registry.GetInitializedServices()
	if len(initialized) != 1 || initialized[0] != "Validation" {
		t.Errorf("Expected Validation to be initialized, got: %v", initialized)
	}

	// Access the same service again - should return the same instance
	validation2 := registry.Validation()
	if validation != validation2 {
		t.Error("Should return same instance on subsequent calls")
	}
}

func TestRegistry_AllServices(t *testing.T) {
	storage := newMockStorage()
	registry, err := NewRegistry(WithStorage(storage))
	if err != nil {
		t.Fatalf("Failed to create registry: %v", err)
	}

	// Test all service accessors
	services := map[string]interface{}{
		"BusinessLogic":  registry.BusinessLogic(),
		"Validation":     registry.Validation(),
		"Authentication": registry.Authentication(),
		"Federation":     registry.Federation(),
		"Timeline":       registry.Timeline(),
		"Analytics":      registry.Analytics(),
		"Notification":   registry.Notification(),
	}

	for name, service := range services {
		if service == nil {
			t.Errorf("Service %s should not be nil", name)
		}
	}

	// All services should now be initialized
	initialized := registry.GetInitializedServices()
	if len(initialized) != len(services) {
		t.Errorf("Expected %d initialized services, got %d", len(services), len(initialized))
	}
}

func TestRegistry_ConcurrentAccess(t *testing.T) {
	storage := newMockStorage()
	registry, err := NewRegistry(WithStorage(storage))
	if err != nil {
		t.Fatalf("Failed to create registry: %v", err)
	}

	// Test concurrent access to services
	var wg sync.WaitGroup
	const numGoroutines = 10

	results := make([]ValidationService, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			results[index] = registry.Validation()
		}(i)
	}

	wg.Wait()

	// All results should be the same instance
	first := results[0]
	for i := 1; i < numGoroutines; i++ {
		if results[i] != first {
			t.Error("Concurrent access should return same instance")
		}
	}
}

func TestRegistry_Close(t *testing.T) {
	storage := newMockStorage()
	publisher := newMockPublisher()
	registry, err := NewRegistry(
		WithStorage(storage),
		WithPublisher(publisher),
	)
	if err != nil {
		t.Fatalf("Failed to create registry: %v", err)
	}

	// Close the registry
	err = registry.Close()
	if err != nil {
		t.Errorf("Failed to close registry: %v", err)
	}

	// Publisher should be closed
	if !publisher.IsClosed() {
		t.Error("Publisher should be closed")
	}
}

func TestRegistry_CloseWithError(t *testing.T) {
	storage := newMockStorage()
	publisher := newMockPublisher()

	// Make publisher return error on close
	publisher.SetCloseError(errors.New("close error"))

	registry, err := NewRegistry(
		WithStorage(storage),
		WithPublisher(publisher),
	)
	if err != nil {
		t.Fatalf("Failed to create registry: %v", err)
	}

	// Close should return error when publisher fails
	err = registry.Close()
	if err == nil {
		t.Error("Expected error from close")
	}
}

func TestRegistry_Health(t *testing.T) {
	storage := newMockStorage()
	publisher := newMockPublisher()
	logger := zaptest.NewLogger(t)
	config := mockConfig()

	registry, err := NewRegistry(
		WithStorage(storage),
		WithPublisher(publisher),
		WithLogger(logger),
		WithConfig(config),
	)
	if err != nil {
		t.Fatalf("Failed to create registry: %v", err)
	}

	// Initialize a service
	registry.Validation()

	health := registry.Health()

	// Check health structure
	status, ok := health["status"].(string)
	if !ok || status != "healthy" {
		t.Errorf("Expected status to be 'healthy', got: %v", status)
	}

	initialized, ok := health["initialized_services"].([]string)
	if !ok || len(initialized) != 1 || initialized[0] != "Validation" {
		t.Errorf("Expected initialized services to include Validation, got: %v", initialized)
	}

	deps, ok := health["dependencies"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected dependencies in health")
	}

	expectedDeps := map[string]bool{
		"storage":   true,
		"publisher": true,
		"logger":    true,
		"config":    true,
	}

	for key, expected := range expectedDeps {
		actual, ok := deps[key].(bool)
		if !ok || actual != expected {
			t.Errorf("Expected %s to be %v, got: %v", key, expected, actual)
		}
	}
}

func TestRegistry_HealthMinimalConfig(t *testing.T) {
	storage := newMockStorage()
	registry, err := NewRegistry(WithStorage(storage))
	if err != nil {
		t.Fatalf("Failed to create registry: %v", err)
	}

	health := registry.Health()
	deps := health["dependencies"].(map[string]interface{})

	// Publisher should be false (not provided)
	if deps["publisher"].(bool) != false {
		t.Error("Publisher should be false when not provided")
	}

	// Others should be true (have defaults)
	if deps["storage"].(bool) != true {
		t.Error("Storage should be true")
	}
	if deps["logger"].(bool) != true {
		t.Error("Logger should be true")
	}
	if deps["config"].(bool) != true {
		t.Error("Config should be true")
	}
}

// Benchmark tests
func BenchmarkRegistry_ServiceAccess(b *testing.B) {
	storage := newMockStorage()
	registry, err := NewRegistry(WithStorage(storage))
	if err != nil {
		b.Fatalf("Failed to create registry: %v", err)
	}

	b.ResetTimer()

	b.Run("first_access", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			// Create new registry for each iteration to test initialization
			storage := newMockStorage()
			registry, _ := NewRegistry(WithStorage(storage))
			_ = registry.Validation()
		}
	})

	b.Run("subsequent_access", func(b *testing.B) {
		// Initialize service once
		_ = registry.Validation()
		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			_ = registry.Validation()
		}
	})
}

func BenchmarkRegistry_ConcurrentAccess(b *testing.B) {
	storage := newMockStorage()
	registry, err := NewRegistry(WithStorage(storage))
	if err != nil {
		b.Fatalf("Failed to create registry: %v", err)
	}

	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = registry.Validation()
		}
	})
}

// Helper functions
func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > len(substr) && findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// Package testing provides test utilities and helpers for DynamORM repository testing with mock support.
package testing

import (
	"context"
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/cost"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/theory-cloud/tabletheory/pkg/core"
	"go.uber.org/zap"
)

// TestDB provides a test database wrapper with cleanup capabilities
type TestDB struct {
	client      core.DB
	tableName   string
	createdData []any
	logger      *zap.Logger
	mu          sync.Mutex
}

// NewTestDB creates a new test database wrapper
func NewTestDB(t *testing.T, tableName string) *TestDB {
	// Connect to a test DynamoDB instance
	// For now, we'll create a mock-based test DB

	logger := zap.NewNop()

	return &TestDB{
		client:      newMockDB(t),
		tableName:   tableName,
		createdData: make([]any, 0),
		logger:      logger,
	}
}

// GetClient returns the underlying database client
func (tdb *TestDB) GetClient() core.DB {
	return tdb.client
}

// GetTableName returns the test table name
func (tdb *TestDB) GetTableName() string {
	return tdb.tableName
}

// TrackCreatedItem tracks an item for cleanup
func (tdb *TestDB) TrackCreatedItem(item any) {
	tdb.mu.Lock()
	defer tdb.mu.Unlock()
	tdb.createdData = append(tdb.createdData, item)
}

// Cleanup removes all tracked test data
func (tdb *TestDB) Cleanup(t *testing.T) {
	tdb.mu.Lock()
	defer tdb.mu.Unlock()

	// Delete all tracked items
	// For testing, we'll just log the cleanup
	if len(tdb.createdData) > 0 {
		t.Logf("Cleaning up %d test items", len(tdb.createdData))
		tdb.createdData = tdb.createdData[:0]
	}
}

// Repository test fixture builder

// FixtureBuilder helps create test data fixtures
type FixtureBuilder struct {
	users         []*models.User
	statuses      []*models.Status
	follows       []map[string]any
	notifications []*models.Notification
	media         []*models.Media
	sessions      []*models.Session
	providers     []*models.ProviderAccount
	mu            sync.Mutex
}

// NewFixtureBuilder creates a new fixture builder
func NewFixtureBuilder() *FixtureBuilder {
	return &FixtureBuilder{
		users:         make([]*models.User, 0),
		statuses:      make([]*models.Status, 0),
		follows:       make([]map[string]any, 0),
		notifications: make([]*models.Notification, 0),
		media:         make([]*models.Media, 0),
		sessions:      make([]*models.Session, 0),
		providers:     make([]*models.ProviderAccount, 0),
	}
}

// WithUser adds a user fixture
func (fb *FixtureBuilder) WithUser(username, email string) *FixtureBuilder {
	fb.mu.Lock()
	defer fb.mu.Unlock()

	user := &models.User{
		Username:    username,
		Email:       email,
		DisplayName: fmt.Sprintf("Display %s", username),
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
		Approved:    true,
		Suspended:   false,
		Silenced:    false,
		Role:        "user",
		Locale:      "en",
	}
	fb.users = append(fb.users, user)
	return fb
}

// WithStatus adds a status fixture
func (fb *FixtureBuilder) WithStatus(userID, content string) *FixtureBuilder {
	fb.mu.Lock()
	defer fb.mu.Unlock()

	status := &models.Status{
		AuthorID:   userID,
		Content:    content,
		Visibility: "public",
	}
	fb.statuses = append(fb.statuses, status)
	return fb
}

// WithFollow adds a follow relationship fixture
func (fb *FixtureBuilder) WithFollow(followerID, followeeID string) *FixtureBuilder {
	fb.mu.Lock()
	defer fb.mu.Unlock()

	follow := map[string]any{
		"FollowerID": followerID,
		"FolloweeID": followeeID,
		"CreatedAt":  time.Now(),
	}
	fb.follows = append(fb.follows, follow)
	return fb
}

// WithNotification adds a notification fixture
func (fb *FixtureBuilder) WithNotification(userID, actorID, notifType string) *FixtureBuilder {
	fb.mu.Lock()
	defer fb.mu.Unlock()

	notification := &models.Notification{
		UserID:    userID,
		Type:      notifType,
		ActorID:   actorID,
		CreatedAt: time.Now(),
		IsRead:    false,
		ExpiresAt: time.Now().Add(30 * 24 * time.Hour).Unix(),
	}
	fb.notifications = append(fb.notifications, notification)
	return fb
}

// WithMedia adds a media fixture
func (fb *FixtureBuilder) WithMedia(userID, fileName string) *FixtureBuilder {
	fb.mu.Lock()
	defer fb.mu.Unlock()

	media := &models.Media{
		UserID:      userID,
		FileName:    fileName,
		ContentType: "image/jpeg",
		FileSize:    1024 * 1024, // 1MB
		Status:      "ready",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	fb.media = append(fb.media, media)
	return fb
}

// WithSession adds a session fixture
func (fb *FixtureBuilder) WithSession(userID, ipAddress string) *FixtureBuilder {
	fb.mu.Lock()
	defer fb.mu.Unlock()

	session := &models.Session{
		UserID:    userID,
		IPAddress: ipAddress,
		UserAgent: "Test User Agent",
		Scopes:    []string{"read", "write"},
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(24 * time.Hour).Unix(),
		IsRevoked: false,
	}
	fb.sessions = append(fb.sessions, session)
	return fb
}

// WithProviderAccount adds a provider account fixture
func (fb *FixtureBuilder) WithProviderAccount(userID, provider, providerID string) *FixtureBuilder {
	fb.mu.Lock()
	defer fb.mu.Unlock()

	providerAccount := &models.ProviderAccount{
		UserID:     userID,
		Provider:   provider,
		ProviderID: providerID,
		Email:      fmt.Sprintf("%s@%s.com", providerID, provider),
		IsActive:   true,
		IsPrimary:  false,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	fb.providers = append(fb.providers, providerAccount)
	return fb
}

// Build creates and returns the fixture data
func (fb *FixtureBuilder) Build() *Fixtures {
	fb.mu.Lock()
	defer fb.mu.Unlock()

	return &Fixtures{
		Users:            fb.users,
		Statuses:         fb.statuses,
		Follows:          fb.follows,
		Notifications:    fb.notifications,
		Media:            fb.media,
		Sessions:         fb.sessions,
		ProviderAccounts: fb.providers,
	}
}

// Fixtures contains all test fixture data
type Fixtures struct {
	Users            []*models.User
	Statuses         []*models.Status
	Follows          []map[string]any
	Notifications    []*models.Notification
	Media            []*models.Media
	Sessions         []*models.Session
	ProviderAccounts []*models.ProviderAccount
}

// LoadIntoTestDB loads all fixtures into the test database
func (f *Fixtures) LoadIntoTestDB(t *testing.T, testDB *TestDB) {
	// Load users
	for _, user := range f.Users {
		testDB.TrackCreatedItem(user)
	}

	// Load statuses
	for _, status := range f.Statuses {
		testDB.TrackCreatedItem(status)
	}

	// Load follows
	for _, follow := range f.Follows {
		testDB.TrackCreatedItem(follow)
	}

	// Load notifications
	for _, notification := range f.Notifications {
		testDB.TrackCreatedItem(notification)
	}

	// Load media
	for _, media := range f.Media {
		testDB.TrackCreatedItem(media)
	}

	// Load sessions
	for _, session := range f.Sessions {
		testDB.TrackCreatedItem(session)
	}

	// Load provider accounts
	for _, provider := range f.ProviderAccounts {
		testDB.TrackCreatedItem(provider)
	}

	t.Logf("Loaded %d fixtures into test database", f.Count())
}

// Count returns the total number of fixtures
func (f *Fixtures) Count() int {
	return len(f.Users) + len(f.Statuses) + len(f.Follows) +
		len(f.Notifications) + len(f.Media) + len(f.Sessions) +
		len(f.ProviderAccounts)
}

// Repository mocks

// MockRepository provides a mock implementation for testing
type MockRepository struct {
	mock.Mock
	data map[string]any
	mu   sync.RWMutex
}

// NewMockRepository creates a new mock repository
func NewMockRepository() *MockRepository {
	return &MockRepository{
		data: make(map[string]any),
	}
}

// Create mocks the create operation
func (m *MockRepository) Create(ctx context.Context, item any) error {
	args := m.Called(ctx, item)
	if args.Error(0) == nil {
		m.storeItem(item)
	}
	return args.Error(0)
}

// Get mocks the get operation
func (m *MockRepository) Get(ctx context.Context, key string, dest any) error {
	args := m.Called(ctx, key, dest)
	if args.Error(0) == nil {
		if item, exists := m.getItem(key); exists {
			m.copyItem(item, dest)
		}
	}
	return args.Error(0)
}

// Update mocks the update operation
func (m *MockRepository) Update(ctx context.Context, item any) error {
	args := m.Called(ctx, item)
	if args.Error(0) == nil {
		m.storeItem(item)
	}
	return args.Error(0)
}

// Delete mocks the delete operation
func (m *MockRepository) Delete(ctx context.Context, key string) error {
	args := m.Called(ctx, key)
	if args.Error(0) == nil {
		m.deleteItem(key)
	}
	return args.Error(0)
}

// Query mocks the query operation
func (m *MockRepository) Query(ctx context.Context, query any, dest any) error {
	args := m.Called(ctx, query, dest)
	return args.Error(0)
}

// Helper methods for mock data management

func (m *MockRepository) storeItem(item any) {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := m.extractKey(item)
	m.data[key] = item
}

func (m *MockRepository) getItem(key string) (any, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	item, exists := m.data[key]
	return item, exists
}

func (m *MockRepository) deleteItem(key string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.data, key)
}

func (m *MockRepository) extractKey(item any) string {
	// Simple key extraction based on type
	switch v := item.(type) {
	case *models.User:
		return fmt.Sprintf("user:%s", v.Username)
	case *models.Status:
		return fmt.Sprintf("status:%s", v.StatusID)
	default:
		return fmt.Sprintf("item:%p", item)
	}
}

func (m *MockRepository) copyItem(src, dest any) {
	srcValue := reflect.ValueOf(src)
	destValue := reflect.ValueOf(dest)

	if destValue.Kind() == reflect.Ptr && srcValue.Type() == destValue.Elem().Type() {
		destValue.Elem().Set(srcValue)
	}
}

// GetData returns all stored data (for testing assertions)
func (m *MockRepository) GetData() map[string]any {
	m.mu.RLock()
	defer m.mu.RUnlock()

	data := make(map[string]any)
	for k, v := range m.data {
		data[k] = v
	}
	return data
}

// Clear removes all stored data
func (m *MockRepository) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data = make(map[string]any)
}

// Test assertion helpers

// AssertItemExists checks if an item exists in the repository
func AssertItemExists(t *testing.T, repo *MockRepository, itemType, key string) {
	fullKey := fmt.Sprintf("%s:%s", itemType, key)
	data := repo.GetData()

	_, exists := data[fullKey]
	assert.True(t, exists, "Expected item %s to exist in repository", fullKey)
}

// AssertItemNotExists checks if an item does not exist in the repository
func AssertItemNotExists(t *testing.T, repo *MockRepository, itemType, key string) {
	fullKey := fmt.Sprintf("%s:%s", itemType, key)
	data := repo.GetData()

	_, exists := data[fullKey]
	assert.False(t, exists, "Expected item %s to not exist in repository", fullKey)
}

// AssertItemCount checks the total number of items in the repository
func AssertItemCount(t *testing.T, repo *MockRepository, expectedCount int) {
	data := repo.GetData()
	actualCount := len(data)

	assert.Equal(t, expectedCount, actualCount, "Expected %d items in repository, got %d", expectedCount, actualCount)
}

// Repository test suite

// RepositoryTestSuite provides a standardized test suite for repositories
type RepositoryTestSuite struct {
	Name         string
	CreateItem   func() any
	UpdateItem   func(any) any
	GetKey       func(any) string
	ValidateItem func(*testing.T, any)
}

// RunCRUDTests runs standard CRUD tests for a repository
func (rts *RepositoryTestSuite) RunCRUDTests(t *testing.T, repo *MockRepository) {
	t.Run(fmt.Sprintf("%s_CRUD", rts.Name), func(t *testing.T) {
		ctx := context.Background()

		// Test Create
		t.Run("Create", func(t *testing.T) {
			item := rts.CreateItem()
			repo.On("Create", ctx, item).Return(nil)

			err := repo.Create(ctx, item)
			assert.NoError(t, err)

			key := rts.GetKey(item)
			data := repo.GetData()
			assert.Contains(t, data, key)

			repo.AssertExpectations(t)
		})

		// Test Get
		t.Run("Get", func(t *testing.T) {
			item := rts.CreateItem()
			key := rts.GetKey(item)

			var dest any
			repo.On("Get", ctx, key, mock.AnythingOfType("*interface {}")).Return(nil).Run(func(args mock.Arguments) {
				dest = args.Get(2)
			})

			err := repo.Get(ctx, key, &dest)
			assert.NoError(t, err)

			repo.AssertExpectations(t)
		})

		// Test Update
		t.Run("Update", func(t *testing.T) {
			item := rts.CreateItem()
			updatedItem := rts.UpdateItem(item)

			repo.On("Update", ctx, updatedItem).Return(nil)

			err := repo.Update(ctx, updatedItem)
			assert.NoError(t, err)

			repo.AssertExpectations(t)
		})

		// Test Delete
		t.Run("Delete", func(t *testing.T) {
			item := rts.CreateItem()
			key := rts.GetKey(item)

			repo.On("Delete", ctx, key).Return(nil)

			err := repo.Delete(ctx, key)
			assert.NoError(t, err)

			repo.AssertExpectations(t)
		})
	})
}

// Performance test helpers

// BenchmarkConfig holds configuration for benchmark tests
type BenchmarkConfig struct {
	ItemCount     int
	ConcurrentOps int
	OperationType string
	SetupFunc     func() any
	BenchmarkFunc func(any) error
}

// RunBenchmark runs a performance benchmark
func RunBenchmark(b *testing.B, config BenchmarkConfig) {
	// Setup
	items := make([]any, config.ItemCount)
	for i := 0; i < config.ItemCount; i++ {
		items[i] = config.SetupFunc()
	}

	b.ResetTimer()
	b.ReportAllocs()

	if config.ConcurrentOps > 1 {
		// Concurrent benchmark
		b.RunParallel(func(pb *testing.PB) {
			i := 0
			for pb.Next() {
				item := items[i%len(items)]
				_ = config.BenchmarkFunc(item)
				i++
			}
		})
	} else {
		// Sequential benchmark
		for i := 0; i < b.N; i++ {
			item := items[i%len(items)]
			_ = config.BenchmarkFunc(item)
		}
	}
}

// Integration test helpers

// IntegrationTestContext provides context for integration tests
type IntegrationTestContext struct {
	TestDB      *TestDB
	Repository  any
	Fixtures    *Fixtures
	Logger      *zap.Logger
	CostTracker *cost.Tracker
}

// NewIntegrationTestContext creates a new integration test context
func NewIntegrationTestContext(t *testing.T, tableName string) *IntegrationTestContext {
	testDB := NewTestDB(t, tableName)
	logger := zap.NewNop()
	costTracker := cost.New()

	return &IntegrationTestContext{
		TestDB:      testDB,
		Logger:      logger,
		CostTracker: costTracker,
	}
}

// SetupFixtures creates and loads test fixtures
func (itc *IntegrationTestContext) SetupFixtures(t *testing.T, fixtureFunc func(*FixtureBuilder) *FixtureBuilder) {
	builder := NewFixtureBuilder()
	builder = fixtureFunc(builder)
	itc.Fixtures = builder.Build()
	itc.Fixtures.LoadIntoTestDB(t, itc.TestDB)
}

// Cleanup cleans up test resources
func (itc *IntegrationTestContext) Cleanup(t *testing.T) {
	if itc.TestDB != nil {
		itc.TestDB.Cleanup(t)
	}
}

// Utility functions

// createMockDB creates a mock database for testing
func newMockDB(_ *testing.T) core.DB {
	mockDB := &MockDB{}

	// Set up default mock behaviors
	mockQuery := &MockQuery{}
	mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Create").Return(nil).Maybe()
	mockQuery.On("First", mock.Anything).Return(nil).Maybe()
	mockQuery.On("All", mock.Anything).Return(nil).Maybe()
	mockQuery.On("Update", mock.Anything).Return(nil).Maybe()
	mockQuery.On("Delete").Return(nil).Maybe()

	return mockDB
}

// SetupDefaultMockDB sets up a MockDB with default expectations
func SetupDefaultMockDB() (*MockDB, *MockQuery) {
	mockDB := &MockDB{}
	mockQuery := &MockQuery{}

	// Set up default behaviors that work for most tests
	mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Create").Return(nil).Maybe()
	mockQuery.On("First", mock.Anything).Return(nil).Maybe()
	mockQuery.On("All", mock.Anything).Return(nil).Maybe()
	mockQuery.On("Delete").Return(nil).Maybe()

	// For update operations
	mockUpdateBuilder := &MockUpdateBuilder{}
	mockQuery.On("Update", mock.Anything).Return(mockUpdateBuilder).Maybe()
	mockUpdateBuilder.On("Set", mock.Anything, mock.Anything).Return(mockUpdateBuilder).Maybe()
	mockUpdateBuilder.On("Execute").Return(nil).Maybe()

	return mockDB, mockQuery
}

// MockDB implements core.DB for testing
type MockDB struct {
	mock.Mock
}

// Model mocks the Model method of the core.DB interface
func (m *MockDB) Model(model any) core.Query {
	args := m.Called(model)
	return args.Get(0).(core.Query)
}

// Transaction mocks the Transaction method of the core.DB interface
func (m *MockDB) Transaction(fn func(*core.Tx) error) error {
	args := m.Called(fn)
	return args.Error(0)
}

// WithContext mocks the WithContext method of the core.DB interface
func (m *MockDB) WithContext(ctx context.Context) core.DB {
	args := m.Called(ctx)
	return args.Get(0).(core.DB)
}

// AutoMigrate mocks the AutoMigrate method of the core.DB interface
func (m *MockDB) AutoMigrate(models ...any) error {
	args := m.Called(models)
	return args.Error(0)
}

// Close mocks the Close method of the core.DB interface
func (m *MockDB) Close() error {
	args := m.Called()
	return args.Error(0)
}

// Migrate mocks the Migrate method of the core.DB interface
func (m *MockDB) Migrate() error {
	args := m.Called()
	return args.Error(0)
}

// MockQuery implements core.Query for testing
type MockQuery struct {
	mock.Mock
}

// Where mocks the Where method of the core.Query interface
func (m *MockQuery) Where(field, op string, value any) core.Query {
	args := m.Called(field, op, value)
	return args.Get(0).(core.Query)
}

// Index mocks the Index method of the core.Query interface
func (m *MockQuery) Index(indexName string) core.Query {
	args := m.Called(indexName)
	return args.Get(0).(core.Query)
}

// Limit mocks the Limit method of the core.Query interface
func (m *MockQuery) Limit(limit int) core.Query {
	args := m.Called(limit)
	return args.Get(0).(core.Query)
}

// First mocks the First method of the core.Query interface
func (m *MockQuery) First(dest any) error {
	args := m.Called(dest)
	return args.Error(0)
}

// All mocks the All method of the core.Query interface
func (m *MockQuery) All(dest any) error {
	args := m.Called(dest)
	return args.Error(0)
}

// Count mocks the Count method of the core.Query interface
func (m *MockQuery) Count() (int64, error) {
	args := m.Called()
	return args.Get(0).(int64), args.Error(1)
}

// Create mocks the Create method of the core.Query interface
func (m *MockQuery) Create() error {
	args := m.Called()
	return args.Error(0)
}

// Update mocks the Update method of the core.Query interface
func (m *MockQuery) Update(fields ...string) error {
	args := m.Called(fields)
	return args.Error(0)
}

// Delete mocks the Delete method of the core.Query interface
func (m *MockQuery) Delete() error {
	args := m.Called()
	return args.Error(0)
}

// BatchCreate mocks the BatchCreate method of the core.Query interface
func (m *MockQuery) BatchCreate(items any) error {
	args := m.Called(items)
	return args.Error(0)
}

// AllPaginated mocks the AllPaginated method of the core.Query interface
func (m *MockQuery) AllPaginated(dest any) (*core.PaginatedResult, error) {
	args := m.Called(dest)
	return args.Get(0).(*core.PaginatedResult), args.Error(1)
}

// BatchDelete mocks the BatchDelete method of the core.Query interface
func (m *MockQuery) BatchDelete(items []any) error {
	args := m.Called(items)
	return args.Error(0)
}

// BatchGet mocks the BatchGet method of the core.Query interface
func (m *MockQuery) BatchGet(keys []any, dest any) error {
	args := m.Called(keys, dest)
	return args.Error(0)
}

// BatchGetWithOptions mocks the BatchGetWithOptions method of the core.Query interface
func (m *MockQuery) BatchGetWithOptions(keys []any, dest any, opts *core.BatchGetOptions) error {
	args := m.Called(keys, dest, opts)
	return args.Error(0)
}

// BatchGetBuilder mocks the BatchGetBuilder method of the core.Query interface
func (m *MockQuery) BatchGetBuilder() core.BatchGetBuilder {
	if len(m.ExpectedCalls) == 0 {
		return nil
	}
	args := m.Called()
	if len(args) > 0 {
		if builder, ok := args.Get(0).(core.BatchGetBuilder); ok {
			return builder
		}
	}
	return nil
}

// BatchUpdateWithOptions mocks the BatchUpdateWithOptions method of the core.Query interface
func (m *MockQuery) BatchUpdateWithOptions(items []any, fields []string, options ...any) error {
	args := m.Called(items, fields, options)
	return args.Error(0)
}

// BatchWrite mocks the BatchWrite method of the core.Query interface
func (m *MockQuery) BatchWrite(putItems []any, deleteKeys []any) error {
	args := m.Called(putItems, deleteKeys)
	return args.Error(0)
}

// Filter mocks the Filter method of the core.Query interface
func (m *MockQuery) Filter(field string, op string, value any) core.Query {
	args := m.Called(field, op, value)
	return args.Get(0).(core.Query)
}

// OrFilter mocks the OrFilter method of the core.Query interface
func (m *MockQuery) OrFilter(field string, op string, value any) core.Query {
	args := m.Called(field, op, value)
	return args.Get(0).(core.Query)
}

// FilterGroup mocks the FilterGroup method of the core.Query interface
func (m *MockQuery) FilterGroup(fn func(core.Query)) core.Query {
	args := m.Called(fn)
	return args.Get(0).(core.Query)
}

// OrFilterGroup mocks the OrFilterGroup method of the core.Query interface
func (m *MockQuery) OrFilterGroup(fn func(core.Query)) core.Query {
	args := m.Called(fn)
	return args.Get(0).(core.Query)
}

// WithCondition mocks the WithCondition method of the core.Query interface
func (m *MockQuery) WithCondition(field, operator string, value any) core.Query {
	args := m.Called(field, operator, value)
	return args.Get(0).(core.Query)
}

// WithConditionExpression mocks the WithConditionExpression method of the core.Query interface
func (m *MockQuery) WithConditionExpression(expr string, values map[string]any) core.Query {
	args := m.Called(expr, values)
	return args.Get(0).(core.Query)
}

// Offset mocks the Offset method of the core.Query interface
func (m *MockQuery) Offset(offset int) core.Query {
	args := m.Called(offset)
	return args.Get(0).(core.Query)
}

// Select mocks the Select method of the core.Query interface
func (m *MockQuery) Select(fields ...string) core.Query {
	args := m.Called(fields)
	return args.Get(0).(core.Query)
}

// ConsistentRead mocks the ConsistentRead method of the core.Query interface
func (m *MockQuery) ConsistentRead() core.Query {
	args := m.Called()
	return args.Get(0).(core.Query)
}

// WithRetry mocks the WithRetry method of the core.Query interface
func (m *MockQuery) WithRetry(maxRetries int, initialDelay time.Duration) core.Query {
	args := m.Called(maxRetries, initialDelay)
	return args.Get(0).(core.Query)
}

// CreateOrUpdate mocks the CreateOrUpdate method of the core.Query interface
func (m *MockQuery) CreateOrUpdate() error {
	args := m.Called()
	return args.Error(0)
}

// Cursor mocks the Cursor method of the core.Query interface
func (m *MockQuery) Cursor(cursor string) core.Query {
	args := m.Called(cursor)
	return args.Get(0).(core.Query)
}

// IfExists mocks the IfExists method of the core.Query interface
func (m *MockQuery) IfExists() core.Query {
	args := m.Called()
	return args.Get(0).(core.Query)
}

// IfNotExists mocks the IfNotExists method of the core.Query interface
func (m *MockQuery) IfNotExists() core.Query {
	args := m.Called()
	return args.Get(0).(core.Query)
}

// OrderBy mocks the OrderBy method of the core.Query interface
func (m *MockQuery) OrderBy(field string, order string) core.Query {
	args := m.Called(field, order)
	return args.Get(0).(core.Query)
}

// Scan mocks the Scan method of the core.Query interface
func (m *MockQuery) Scan(dest any) error {
	args := m.Called(dest)
	return args.Error(0)
}

// ParallelScan mocks the ParallelScan method of the core.Query interface
func (m *MockQuery) ParallelScan(segment int32, totalSegments int32) core.Query {
	args := m.Called(segment, totalSegments)
	return args.Get(0).(core.Query)
}

// ScanAllSegments mocks the ScanAllSegments method of the core.Query interface
func (m *MockQuery) ScanAllSegments(dest any, totalSegments int32) error {
	args := m.Called(dest, totalSegments)
	return args.Error(0)
}

// SetCursor mocks the SetCursor method of the core.Query interface
func (m *MockQuery) SetCursor(cursor string) error {
	args := m.Called(cursor)
	return args.Error(0)
}

// UpdateBuilder mocks the UpdateBuilder method of the core.Query interface
func (m *MockQuery) UpdateBuilder() core.UpdateBuilder {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(core.UpdateBuilder)
}

// WithContext mocks the WithContext method of the core.Query interface
func (m *MockQuery) WithContext(ctx context.Context) core.Query {
	args := m.Called(ctx)
	return args.Get(0).(core.Query)
}

// MockUpdateBuilder mocks the UpdateBuilder interface
type MockUpdateBuilder struct {
	mock.Mock
}

// Set mocks the Set method of the core.UpdateBuilder interface
func (m *MockUpdateBuilder) Set(field string, value any) core.UpdateBuilder {
	args := m.Called(field, value)
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(core.UpdateBuilder)
}

// Execute mocks the Execute method of the core.UpdateBuilder interface
func (m *MockUpdateBuilder) Execute() error {
	args := m.Called()
	return args.Error(0)
}

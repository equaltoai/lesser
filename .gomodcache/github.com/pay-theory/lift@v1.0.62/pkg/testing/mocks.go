package testing

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/pay-theory/lift/pkg/dynamorm"
)

// MockDynamORM provides a mock implementation of DynamORM for testing
// Memory optimized for better alignment
type MockDynamORM struct {
	data            map[string]map[string]any
	transactions    map[string]*MockTransaction
	FailOnOperation map[string]error
	Delays          map[string]time.Duration
	config          *dynamorm.DynamORMConfig
	mu              sync.RWMutex
}

// NewMockDynamORM creates a new mock DynamORM instance
func NewMockDynamORM() *MockDynamORM {
	return &MockDynamORM{
		data:            make(map[string]map[string]any),
		transactions:    make(map[string]*MockTransaction),
		config:          dynamorm.DefaultConfig(),
		FailOnOperation: make(map[string]error),
		Delays:          make(map[string]time.Duration),
	}
}

// WithFailure configures the mock to fail on specific operations
func (m *MockDynamORM) WithFailure(operation string, err error) *MockDynamORM {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.FailOnOperation[operation] = err
	return m
}

// WithDelay configures the mock to add delays to operations
func (m *MockDynamORM) WithDelay(operation string, delay time.Duration) *MockDynamORM {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Delays[operation] = delay
	return m
}

// WithData pre-populates the mock with test data
func (m *MockDynamORM) WithData(table, key string, item any) *MockDynamORM {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.data[table] == nil {
		m.data[table] = make(map[string]any)
	}
	m.data[table][key] = item
	return m
}

// Get retrieves an item by key
func (m *MockDynamORM) Get(_ context.Context, table, key string, result any) error {
	if err := m.simulateOperation("get"); err != nil {
		return err
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	tableData, exists := m.data[table]
	if !exists {
		return fmt.Errorf("item not found")
	}

	item, exists := tableData[key]
	if !exists {
		return fmt.Errorf("item not found")
	}

	// Marshal and unmarshal to simulate DynamoDB behavior
	data, err := json.Marshal(item)
	if err != nil {
		return err
	}

	return json.Unmarshal(data, result)
}

// Put saves an item
func (m *MockDynamORM) Put(_ context.Context, table, key string, item any) error {
	if err := m.simulateOperation("put"); err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if m.data[table] == nil {
		m.data[table] = make(map[string]any)
	}

	m.data[table][key] = item
	return nil
}

// Delete removes an item
func (m *MockDynamORM) Delete(_ context.Context, table, key string) error {
	if err := m.simulateOperation("delete"); err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if tableData, exists := m.data[table]; exists {
		delete(tableData, key)
	}

	return nil
}

// Query performs a query operation
func (m *MockDynamORM) Query(_ context.Context, table string, _ *dynamorm.Query) (*dynamorm.QueryResult, error) {
	if err := m.simulateOperation("query"); err != nil {
		return nil, err
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	tableData, exists := m.data[table]
	if !exists {
		return &dynamorm.QueryResult{
			Items: []any{},
			Count: 0,
		}, nil
	}

	// Simple implementation - return all items (for now)
	items := make([]any, 0, len(tableData))
	for _, item := range tableData {
		items = append(items, item)
	}

	return &dynamorm.QueryResult{
		Items: items,
		Count: len(items),
	}, nil
}

// BeginTransaction starts a mock transaction
func (m *MockDynamORM) BeginTransaction() (*MockTransaction, error) {
	if err := m.simulateOperation("begin_transaction"); err != nil {
		return nil, err
	}

	tx := &MockTransaction{
		id:         fmt.Sprintf("tx_%d", time.Now().UnixNano()),
		mock:       m,
		committed:  false,
		rolledBack: false,
		operations: make([]TransactionOperation, 0),
	}

	m.mu.Lock()
	m.transactions[tx.id] = tx
	m.mu.Unlock()

	return tx, nil
}

// simulateOperation handles failure simulation and delays
func (m *MockDynamORM) simulateOperation(operation string) error {
	// Check for configured delays
	if delay, exists := m.Delays[operation]; exists {
		time.Sleep(delay)
	}

	// Check for configured failures
	if err, exists := m.FailOnOperation[operation]; exists {
		return err
	}

	return nil
}

// GetAllData returns all data for testing inspection
func (m *MockDynamORM) GetAllData() map[string]map[string]any {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Deep copy to prevent external modification
	result := make(map[string]map[string]any)
	for table, tableData := range m.data {
		result[table] = make(map[string]any)
		for key, item := range tableData {
			result[table][key] = item
		}
	}

	return result
}

// Reset clears all data and resets the mock state
func (m *MockDynamORM) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.data = make(map[string]map[string]any)
	m.transactions = make(map[string]*MockTransaction)
	m.FailOnOperation = make(map[string]error)
	m.Delays = make(map[string]time.Duration)
}

// MockTransaction represents a mock DynamORM transaction
// Memory optimized for better alignment
type MockTransaction struct {
	mock       *MockDynamORM
	id         string
	operations []TransactionOperation
	mu         sync.RWMutex
	committed  bool
	rolledBack bool
}

// TransactionOperation represents an operation within a transaction
// Memory optimized for better alignment
type TransactionOperation struct {
	// Interface first (24 bytes)
	Item any
	// Strings (16 bytes each)
	Type  string
	Table string
	Key   string
}

// Put adds a put operation to the transaction
func (tx *MockTransaction) Put(_ context.Context, table, key string, item any) error {
	tx.mu.Lock()
	defer tx.mu.Unlock()

	if tx.committed || tx.rolledBack {
		return fmt.Errorf("transaction already completed")
	}

	tx.operations = append(tx.operations, TransactionOperation{
		Type:  "put",
		Table: table,
		Key:   key,
		Item:  item,
	})

	return nil
}

// Delete adds a delete operation to the transaction
func (tx *MockTransaction) Delete(_ context.Context, table, key string) error {
	tx.mu.Lock()
	defer tx.mu.Unlock()

	if tx.committed || tx.rolledBack {
		return fmt.Errorf("transaction already completed")
	}

	tx.operations = append(tx.operations, TransactionOperation{
		Type:  "delete",
		Table: table,
		Key:   key,
	})

	return nil
}

// Commit commits the transaction
func (tx *MockTransaction) Commit() error {
	tx.mu.Lock()
	defer tx.mu.Unlock()

	if tx.committed || tx.rolledBack {
		return fmt.Errorf("transaction already completed")
	}

	// Apply all operations atomically
	for _, op := range tx.operations {
		switch op.Type {
		case "put":
			tx.mock.mu.Lock()
			if tx.mock.data[op.Table] == nil {
				tx.mock.data[op.Table] = make(map[string]any)
			}
			tx.mock.data[op.Table][op.Key] = op.Item
			tx.mock.mu.Unlock()
		case "delete":
			tx.mock.mu.Lock()
			if tableData, exists := tx.mock.data[op.Table]; exists {
				delete(tableData, op.Key)
			}
			tx.mock.mu.Unlock()
		}
	}

	tx.committed = true
	return nil
}

// Rollback rolls back the transaction
func (tx *MockTransaction) Rollback() error {
	tx.mu.Lock()
	defer tx.mu.Unlock()

	if tx.committed || tx.rolledBack {
		return fmt.Errorf("transaction already completed")
	}

	// Simply mark as rolled back - no operations to undo since they weren't applied
	tx.rolledBack = true
	return nil
}

// MockAWSService provides a generic mock for AWS services
// Memory optimized for better alignment
type MockAWSService struct {
	responses map[string]any
	errors    map[string]error
	callCount map[string]int
	mu        sync.RWMutex
}

// NewMockAWSService creates a new mock AWS service
func NewMockAWSService() *MockAWSService {
	return &MockAWSService{
		responses: make(map[string]any),
		errors:    make(map[string]error),
		callCount: make(map[string]int),
	}
}

// WithResponse configures a response for an operation
func (m *MockAWSService) WithResponse(operation string, response any) *MockAWSService {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.responses[operation] = response
	return m
}

// WithError configures an error for an operation
func (m *MockAWSService) WithError(operation string, err error) *MockAWSService {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.errors[operation] = err
	return m
}

// Call simulates calling an AWS service operation
func (m *MockAWSService) Call(operation string, _ any) (any, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Track call count
	m.callCount[operation]++

	// Return configured error if exists
	if err, exists := m.errors[operation]; exists {
		return nil, err
	}

	// Return configured response if exists
	if response, exists := m.responses[operation]; exists {
		return response, nil
	}

	// Default empty response
	return nil, nil
}

// GetCallCount returns the number of times an operation was called
func (m *MockAWSService) GetCallCount(operation string) int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.callCount[operation]
}

// Reset clears all mock state
func (m *MockAWSService) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.responses = make(map[string]any)
	m.errors = make(map[string]error)
	m.callCount = make(map[string]int)
}

// MockHTTPClient provides a mock HTTP client for external API testing
// Memory optimized for better alignment
type MockHTTPClient struct {
	responses map[string]*MockHTTPResponse
	callCount map[string]int
	mu        sync.RWMutex
}

// MockHTTPResponse represents a mock HTTP response
// Memory optimized for better alignment
type MockHTTPResponse struct {
	Headers    map[string]string
	Body       string
	Delay      time.Duration
	StatusCode int
}

// NewMockHTTPClient creates a new mock HTTP client
func NewMockHTTPClient() *MockHTTPClient {
	return &MockHTTPClient{
		responses: make(map[string]*MockHTTPResponse),
		callCount: make(map[string]int),
	}
}

// WithResponse configures a response for a URL
func (m *MockHTTPClient) WithResponse(url string, response *MockHTTPResponse) *MockHTTPClient {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.responses[url] = response
	return m
}

// Get simulates an HTTP GET request
func (m *MockHTTPClient) Get(url string) (*MockHTTPResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.callCount[url]++

	response, exists := m.responses[url]
	if !exists {
		return &MockHTTPResponse{
			StatusCode: 404,
			Body:       "Not Found",
			Headers:    make(map[string]string),
		}, nil
	}

	// Simulate delay if configured
	if response.Delay > 0 {
		time.Sleep(response.Delay)
	}

	return response, nil
}

// Reset clears all mock state
func (m *MockHTTPClient) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.responses = make(map[string]*MockHTTPResponse)
	m.callCount = make(map[string]int)
}

// MockDynamORMWrapper wraps MockDynamORM to match the DynamORMWrapper interface
type MockDynamORMWrapper struct {
	mock      *MockDynamORM
	tableName string
	tenantID  string
}

// NewMockDynamORMWrapper creates a wrapper that matches DynamORMWrapper interface
func NewMockDynamORMWrapper(tableName string) *MockDynamORMWrapper {
	return &MockDynamORMWrapper{
		mock:      NewMockDynamORM(),
		tableName: tableName,
	}
}

// WithTenant creates a tenant-scoped wrapper
func (w *MockDynamORMWrapper) WithTenant(tenantID string) *MockDynamORMWrapper {
	return &MockDynamORMWrapper{
		mock:      w.mock,
		tableName: w.tableName,
		tenantID:  tenantID,
	}
}

// Get retrieves an item by key (matches DynamORMWrapper interface)
func (w *MockDynamORMWrapper) Get(ctx context.Context, key any, result any) error {
	keyStr := fmt.Sprintf("%v", key)
	return w.mock.Get(ctx, w.tableName, keyStr, result)
}

// Put saves an item (matches DynamORMWrapper interface)
func (w *MockDynamORMWrapper) Put(ctx context.Context, item any) error {
	// Extract ID from item for the key
	keyStr := w.extractID(item)
	return w.mock.Put(ctx, w.tableName, keyStr, item)
}

// Delete removes an item (matches DynamORMWrapper interface)
func (w *MockDynamORMWrapper) Delete(ctx context.Context, key any) error {
	keyStr := fmt.Sprintf("%v", key)
	return w.mock.Delete(ctx, w.tableName, keyStr)
}

// Query performs a query operation (matches DynamORMWrapper interface)
func (w *MockDynamORMWrapper) Query(ctx context.Context, _ any) (any, error) {
	// Simple implementation for testing
	return w.mock.Query(ctx, w.tableName, nil)
}

// BeginTransaction starts a mock transaction
func (w *MockDynamORMWrapper) BeginTransaction() (any, error) {
	return w.mock.BeginTransaction()
}

// extractID extracts the ID field from an item using reflection or type assertion
func (w *MockDynamORMWrapper) extractID(item any) string {
	// Try to extract ID field using type assertion for common types
	if v, ok := item.(map[string]any); ok {
		if id, ok := v["id"].(string); ok {
			return id
		}
		if id, ok := v["ID"].(string); ok {
			return id
		}
	}

	// For struct types, try to access ID field via interface
	if idGetter, ok := item.(interface{ GetID() string }); ok {
		return idGetter.GetID()
	}

	// Fallback to string representation
	return fmt.Sprintf("%v", item)
}

// WithData pre-populates the mock with test data
func (w *MockDynamORMWrapper) WithData(key string, item any) *MockDynamORMWrapper {
	w.mock.WithData(w.tableName, key, item)
	return w
}

// WithFailure configures the mock to fail on specific operations
func (w *MockDynamORMWrapper) WithFailure(operation string, err error) *MockDynamORMWrapper {
	w.mock.WithFailure(operation, err)
	return w
}

// GetAllData returns all data for testing inspection
func (w *MockDynamORMWrapper) GetAllData() map[string]map[string]any {
	return w.mock.GetAllData()
}

// =============================================================================
// API Gateway Management API Mocks
// =============================================================================

// ConnectionState represents the state of a WebSocket connection
type ConnectionState string

const (
	ConnectionStateActive       ConnectionState = "ACTIVE"
	ConnectionStateDisconnected ConnectionState = "DISCONNECTED"
	ConnectionStateStale        ConnectionState = "STALE"
)

// MockConnection represents a WebSocket connection in the mock
type MockConnection struct {
	CreatedAt    time.Time       `json:"created_at"`
	LastActiveAt time.Time       `json:"last_active_at"`
	Metadata     map[string]any  `json:"metadata,omitempty"`
	ID           string          `json:"id"`
	State        ConnectionState `json:"state"`
	SourceIP     string          `json:"source_ip"`
	UserAgent    string          `json:"user_agent"`
}

// MockAPIGatewayConfig configures the behavior of the API Gateway mock
type MockAPIGatewayConfig struct {
	ErrorRates     map[string]float64
	ConnectionTTL  int64
	MaxMessageSize int64
	NetworkDelay   time.Duration
}

// DefaultMockAPIGatewayConfig returns default configuration
func DefaultMockAPIGatewayConfig() *MockAPIGatewayConfig {
	return &MockAPIGatewayConfig{
		ConnectionTTL:  7200,   // 2 hours
		MaxMessageSize: 131072, // 128KB
		NetworkDelay:   0,
		ErrorRates:     make(map[string]float64),
	}
}

// MockAPIGatewayManagementClient provides a mock implementation of API Gateway Management API
type MockAPIGatewayManagementClient struct {
	connections map[string]*MockConnection
	messages    map[string][][]byte
	errors      map[string]error
	callCount   map[string]int
	config      *MockAPIGatewayConfig
	mu          sync.RWMutex
}

// NewMockAPIGatewayManagementClient creates a new mock API Gateway Management client
func NewMockAPIGatewayManagementClient() *MockAPIGatewayManagementClient {
	return &MockAPIGatewayManagementClient{
		connections: make(map[string]*MockConnection),
		messages:    make(map[string][][]byte),
		errors:      make(map[string]error),
		callCount:   make(map[string]int),
		config:      DefaultMockAPIGatewayConfig(),
	}
}

// WithConfig sets the mock configuration
func (m *MockAPIGatewayManagementClient) WithConfig(config *MockAPIGatewayConfig) *MockAPIGatewayManagementClient {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.config = config
	return m
}

// WithConnection adds a connection to the mock
func (m *MockAPIGatewayManagementClient) WithConnection(connectionID string, conn *MockConnection) *MockAPIGatewayManagementClient {
	m.mu.Lock()
	defer m.mu.Unlock()

	if conn == nil {
		conn = &MockConnection{
			ID:           connectionID,
			State:        ConnectionStateActive,
			CreatedAt:    time.Now(),
			LastActiveAt: time.Now(),
			SourceIP:     "127.0.0.1",
			UserAgent:    "MockClient/1.0",
			Metadata:     make(map[string]any),
		}
	}

	m.connections[connectionID] = conn
	return m
}

// WithError configures an error for a specific connection
func (m *MockAPIGatewayManagementClient) WithError(connectionID string, err error) *MockAPIGatewayManagementClient {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.errors[connectionID] = err
	return m
}

// PostToConnection sends data to a WebSocket connection
func (m *MockAPIGatewayManagementClient) PostToConnection(_ context.Context, connectionID string, data []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Track call count
	m.callCount["PostToConnection"]++

	// Simulate network delay
	if m.config.NetworkDelay > 0 {
		time.Sleep(m.config.NetworkDelay)
	}

	// Check for configured error
	if err, exists := m.errors[connectionID]; exists {
		return err
	}

	// Check if connection exists
	conn, exists := m.connections[connectionID]
	if !exists {
		return &MockAPIGatewayError{
			Code:       "GoneException",
			Message:    fmt.Sprintf("Connection %s not found", connectionID),
			StatusCode: 410,
			Retryable:  false,
		}
	}

	// Check connection state
	if conn.State != ConnectionStateActive {
		return &MockAPIGatewayError{
			Code:       "GoneException",
			Message:    fmt.Sprintf("Connection %s is not active", connectionID),
			StatusCode: 410,
			Retryable:  false,
		}
	}

	// Check if connection is stale (TTL expired)
	if time.Since(conn.CreatedAt).Seconds() > float64(m.config.ConnectionTTL) {
		conn.State = ConnectionStateStale
		return &MockAPIGatewayError{
			Code:       "GoneException",
			Message:    fmt.Sprintf("Connection %s has expired", connectionID),
			StatusCode: 410,
			Retryable:  false,
		}
	}

	// Check message size
	if int64(len(data)) > m.config.MaxMessageSize {
		return &MockAPIGatewayError{
			Code:       "PayloadTooLargeException",
			Message:    fmt.Sprintf("Message size %d exceeds maximum %d", len(data), m.config.MaxMessageSize),
			StatusCode: 413,
			Retryable:  false,
		}
	}

	// Store the message
	m.messages[connectionID] = append(m.messages[connectionID], data)

	// Update last active time
	conn.LastActiveAt = time.Now()

	return nil
}

// DeleteConnection terminates a WebSocket connection
func (m *MockAPIGatewayManagementClient) DeleteConnection(_ context.Context, connectionID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Track call count
	m.callCount["DeleteConnection"]++

	// Simulate network delay
	if m.config.NetworkDelay > 0 {
		time.Sleep(m.config.NetworkDelay)
	}

	// Check for configured error
	if err, exists := m.errors[connectionID]; exists {
		return err
	}

	// Check if connection exists
	conn, exists := m.connections[connectionID]
	if !exists {
		return &MockAPIGatewayError{
			Code:       "GoneException",
			Message:    fmt.Sprintf("Connection %s not found", connectionID),
			StatusCode: 410,
			Retryable:  false,
		}
	}

	// Mark connection as disconnected
	conn.State = ConnectionStateDisconnected
	conn.LastActiveAt = time.Now()

	return nil
}

// GetConnection retrieves connection information
func (m *MockAPIGatewayManagementClient) GetConnection(_ context.Context, connectionID string) (*MockConnectionInfo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Track call count
	m.callCount["GetConnection"]++

	// Simulate network delay
	if m.config.NetworkDelay > 0 {
		time.Sleep(m.config.NetworkDelay)
	}

	// Check for configured error
	if err, exists := m.errors[connectionID]; exists {
		return nil, err
	}

	// Check if connection exists
	conn, exists := m.connections[connectionID]
	if !exists {
		return nil, &MockAPIGatewayError{
			Code:       "GoneException",
			Message:    fmt.Sprintf("Connection %s not found", connectionID),
			StatusCode: 410,
			Retryable:  false,
		}
	}

	return &MockConnectionInfo{
		ConnectionID: conn.ID,
		ConnectedAt:  conn.CreatedAt.Format(time.RFC3339),
		LastActiveAt: conn.LastActiveAt.Format(time.RFC3339),
		SourceIP:     conn.SourceIP,
		UserAgent:    conn.UserAgent,
	}, nil
}

// MockConnectionInfo represents connection information returned by GetConnection
type MockConnectionInfo struct {
	ConnectionID string `json:"connectionId"`
	ConnectedAt  string `json:"connectedAt"`
	LastActiveAt string `json:"lastActiveAt"`
	SourceIP     string `json:"sourceIp"`
	UserAgent    string `json:"userAgent"`
}

// MockAPIGatewayError implements the APIError interface for testing
type MockAPIGatewayError struct {
	Code       string
	Message    string
	StatusCode int
	Retryable  bool
}

func (e *MockAPIGatewayError) Error() string {
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func (e *MockAPIGatewayError) HTTPStatusCode() int {
	return e.StatusCode
}

func (e *MockAPIGatewayError) ErrorCode() string {
	return e.Code
}

func (e *MockAPIGatewayError) IsRetryable() bool {
	return e.Retryable
}

// Helper methods for testing

// GetMessages returns all messages sent to a connection
func (m *MockAPIGatewayManagementClient) GetMessages(connectionID string) [][]byte {
	m.mu.RLock()
	defer m.mu.RUnlock()

	messages, exists := m.messages[connectionID]
	if !exists {
		return nil
	}

	// Return a copy to prevent external modification
	result := make([][]byte, len(messages))
	for i, msg := range messages {
		result[i] = make([]byte, len(msg))
		copy(result[i], msg)
	}

	return result
}

// GetMessageCount returns the number of messages sent to a connection
func (m *MockAPIGatewayManagementClient) GetMessageCount(connectionID string) int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.messages[connectionID])
}

// GetCallCount returns the number of times an operation was called
func (m *MockAPIGatewayManagementClient) GetCallCount(operation string) int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.callCount[operation]
}

// GetConnectionState returns a copy of the connection state
func (m *MockAPIGatewayManagementClient) GetConnectionState(connectionID string) *MockConnection {
	m.mu.RLock()
	defer m.mu.RUnlock()

	conn, exists := m.connections[connectionID]
	if !exists {
		return nil
	}

	// Return a copy to prevent external modification
	connCopy := *conn
	if conn.Metadata != nil {
		connCopy.Metadata = make(map[string]any)
		for k, v := range conn.Metadata {
			connCopy.Metadata[k] = v
		}
	}

	return &connCopy
}

// GetActiveConnections returns all active connections
func (m *MockAPIGatewayManagementClient) GetActiveConnections() map[string]*MockConnection {
	m.mu.RLock()
	defer m.mu.RUnlock()

	active := make(map[string]*MockConnection)
	for id, conn := range m.connections {
		if conn.State == ConnectionStateActive {
			// Return a copy
			connCopy := *conn
			if conn.Metadata != nil {
				connCopy.Metadata = make(map[string]any)
				for k, v := range conn.Metadata {
					connCopy.Metadata[k] = v
				}
			}
			active[id] = &connCopy
		}
	}

	return active
}

// Reset clears all mock state
func (m *MockAPIGatewayManagementClient) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.connections = make(map[string]*MockConnection)
	m.messages = make(map[string][][]byte)
	m.errors = make(map[string]error)
	m.callCount = make(map[string]int)
	m.config = DefaultMockAPIGatewayConfig()
}

// SimulateConnectionExpiry marks connections as stale based on TTL
func (m *MockAPIGatewayManagementClient) SimulateConnectionExpiry() {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	for _, conn := range m.connections {
		if conn.State == ConnectionStateActive &&
			now.Sub(conn.CreatedAt).Seconds() > float64(m.config.ConnectionTTL) {
			conn.State = ConnectionStateStale
		}
	}
}

// =============================================================================
// End of API Gateway Management API Mocks
// =============================================================================

// =============================================================================
// CloudWatch Metrics & Alarms Mocks
// =============================================================================

// MetricUnit represents CloudWatch metric units
type MetricUnit string

const (
	MetricUnitNone           MetricUnit = "None"
	MetricUnitSeconds        MetricUnit = "Seconds"
	MetricUnitMicroseconds   MetricUnit = "Microseconds"
	MetricUnitMilliseconds   MetricUnit = "Milliseconds"
	MetricUnitBytes          MetricUnit = "Bytes"
	MetricUnitKilobytes      MetricUnit = "Kilobytes"
	MetricUnitMegabytes      MetricUnit = "Megabytes"
	MetricUnitGigabytes      MetricUnit = "Gigabytes"
	MetricUnitTerabytes      MetricUnit = "Terabytes"
	MetricUnitBits           MetricUnit = "Bits"
	MetricUnitKilobits       MetricUnit = "Kilobits"
	MetricUnitMegabits       MetricUnit = "Megabits"
	MetricUnitGigabits       MetricUnit = "Gigabits"
	MetricUnitTerabits       MetricUnit = "Terabits"
	MetricUnitPercent        MetricUnit = "Percent"
	MetricUnitCount          MetricUnit = "Count"
	MetricUnitCountPerSecond MetricUnit = "Count/Second"
)

// MockMetricDatum represents a single metric data point
type MockMetricDatum struct {
	Timestamp  time.Time         `json:"timestamp"`
	Dimensions map[string]string `json:"dimensions,omitempty"`
	Metadata   map[string]any    `json:"metadata,omitempty"`
	MetricName string            `json:"metric_name"`
	Unit       MetricUnit        `json:"unit"`
	Value      float64           `json:"value"`
}

// AlarmState represents the state of a CloudWatch alarm
type AlarmState string

const (
	AlarmStateOK               AlarmState = "OK"
	AlarmStateAlarm            AlarmState = "ALARM"
	AlarmStateInsufficientData AlarmState = "INSUFFICIENT_DATA"
)

// ComparisonOperator represents alarm comparison operators
type ComparisonOperator string

const (
	ComparisonGreaterThanThreshold                     ComparisonOperator = "GreaterThanThreshold"
	ComparisonGreaterThanOrEqualToThreshold            ComparisonOperator = "GreaterThanOrEqualToThreshold"
	ComparisonLessThanThreshold                        ComparisonOperator = "LessThanThreshold"
	ComparisonLessThanOrEqualToThreshold               ComparisonOperator = "LessThanOrEqualToThreshold"
	ComparisonLessThanLowerOrGreaterThanUpperThreshold ComparisonOperator = "LessThanLowerOrGreaterThanUpperThreshold"
	ComparisonLessThanLowerThreshold                   ComparisonOperator = "LessThanLowerThreshold"
	ComparisonGreaterThanUpperThreshold                ComparisonOperator = "GreaterThanUpperThreshold"
)

// Statistic represents CloudWatch statistics
type Statistic string

const (
	StatisticSampleCount Statistic = "SampleCount"
	StatisticAverage     Statistic = "Average"
	StatisticSum         Statistic = "Sum"
	StatisticMinimum     Statistic = "Minimum"
	StatisticMaximum     Statistic = "Maximum"
)

// MockAlarmDefinition represents a CloudWatch alarm
type MockAlarmDefinition struct {
	UpdatedAt          time.Time          `json:"updated_at"`
	CreatedAt          time.Time          `json:"created_at"`
	StateUpdatedAt     time.Time          `json:"state_updated_at"`
	Dimensions         map[string]string  `json:"dimensions,omitempty"`
	StateReason        string             `json:"state_reason"`
	Statistic          Statistic          `json:"statistic"`
	ComparisonOperator ComparisonOperator `json:"comparison_operator"`
	TreatMissingData   string             `json:"treat_missing_data"`
	State              AlarmState         `json:"state"`
	AlarmName          string             `json:"alarm_name"`
	Namespace          string             `json:"namespace"`
	MetricName         string             `json:"metric_name"`
	AlarmDescription   string             `json:"alarm_description"`
	Threshold          float64            `json:"threshold"`
	Period             int32              `json:"period"`
	EvaluationPeriods  int32              `json:"evaluation_periods"`
}

// MockCloudWatchConfig configures the behavior of CloudWatch mocks
type MockCloudWatchConfig struct {
	// Maximum number of metrics per PutMetricData call
	MaxMetricsPerCall int
	// Simulate network delays
	NetworkDelay time.Duration
	// Auto-evaluate alarms when metrics are published
	AutoEvaluateAlarms bool
	// Metric retention period in hours
	MetricRetentionHours int
}

// DefaultMockCloudWatchConfig returns default configuration
func DefaultMockCloudWatchConfig() *MockCloudWatchConfig {
	return &MockCloudWatchConfig{
		MaxMetricsPerCall:    20,
		NetworkDelay:         0,
		AutoEvaluateAlarms:   true,
		MetricRetentionHours: 24 * 15, // 15 days
	}
}

// MockCloudWatchMetricsClient provides a mock implementation of CloudWatch Metrics
type MockCloudWatchMetricsClient struct {
	metrics   map[string][]*MockMetricDatum
	callCount map[string]int
	errors    map[string]error
	config    *MockCloudWatchConfig
	mu        sync.RWMutex
}

// NewMockCloudWatchMetricsClient creates a new mock CloudWatch Metrics client
func NewMockCloudWatchMetricsClient() *MockCloudWatchMetricsClient {
	return &MockCloudWatchMetricsClient{
		metrics:   make(map[string][]*MockMetricDatum),
		callCount: make(map[string]int),
		errors:    make(map[string]error),
		config:    DefaultMockCloudWatchConfig(),
	}
}

// WithConfig sets the mock configuration
func (m *MockCloudWatchMetricsClient) WithConfig(config *MockCloudWatchConfig) *MockCloudWatchMetricsClient {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.config = config
	return m
}

// WithError configures an error for a specific operation
func (m *MockCloudWatchMetricsClient) WithError(operation string, err error) *MockCloudWatchMetricsClient {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.errors[operation] = err
	return m
}

// PutMetricData publishes metric data to CloudWatch
func (m *MockCloudWatchMetricsClient) PutMetricData(_ context.Context, namespace string, metricData []*MockMetricDatum) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Track call count
	m.callCount["PutMetricData"]++

	// Simulate network delay
	if m.config.NetworkDelay > 0 {
		time.Sleep(m.config.NetworkDelay)
	}

	// Check for configured error
	if err, exists := m.errors["PutMetricData"]; exists {
		return err
	}

	// Validate input
	if namespace == "" {
		return fmt.Errorf("namespace is required")
	}

	if len(metricData) == 0 {
		return fmt.Errorf("at least one metric datum is required")
	}

	if len(metricData) > m.config.MaxMetricsPerCall {
		return fmt.Errorf("too many metrics: %d, maximum allowed: %d", len(metricData), m.config.MaxMetricsPerCall)
	}

	// Store metrics
	if m.metrics[namespace] == nil {
		m.metrics[namespace] = make([]*MockMetricDatum, 0)
	}

	// Add timestamps if not provided
	now := time.Now()
	for _, datum := range metricData {
		if datum.Timestamp.IsZero() {
			datum.Timestamp = now
		}

		// Create a copy to prevent external modification
		datumCopy := *datum
		if datum.Dimensions != nil {
			datumCopy.Dimensions = make(map[string]string)
			for k, v := range datum.Dimensions {
				datumCopy.Dimensions[k] = v
			}
		}
		if datum.Metadata != nil {
			datumCopy.Metadata = make(map[string]any)
			for k, v := range datum.Metadata {
				datumCopy.Metadata[k] = v
			}
		}

		m.metrics[namespace] = append(m.metrics[namespace], &datumCopy)
	}

	return nil
}

// GetMetricStatistics retrieves statistics for a metric
func (m *MockCloudWatchMetricsClient) GetMetricStatistics(_ context.Context, namespace, metricName string, dimensions map[string]string, startTime, endTime time.Time, _ int32, statistics []Statistic) (map[Statistic]float64, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Track call count
	m.callCount["GetMetricStatistics"]++

	// Simulate network delay
	if m.config.NetworkDelay > 0 {
		time.Sleep(m.config.NetworkDelay)
	}

	// Check for configured error
	if err, exists := m.errors["GetMetricStatistics"]; exists {
		return nil, err
	}

	// Create metric query and execute
	query := &metricQuery{
		namespace:  namespace,
		metricName: metricName,
		dimensions: dimensions,
		startTime:  startTime,
		endTime:    endTime,
	}

	matcher := newMetricMatcher(m.metrics)
	values := matcher.findMatchingValues(query)

	if len(values) == 0 {
		return make(map[Statistic]float64), nil
	}

	// Calculate requested statistics
	calculator := newStatisticsCalculator(values)
	return calculator.calculate(statistics), nil
}

// metricQuery represents a query for metrics
type metricQuery struct {
	startTime  time.Time
	endTime    time.Time
	dimensions map[string]string
	namespace  string
	metricName string
}

// metricMatcher handles metric filtering and matching
type metricMatcher struct {
	metrics map[string][]*MockMetricDatum
}

// newMetricMatcher creates a new metric matcher
func newMetricMatcher(metrics map[string][]*MockMetricDatum) *metricMatcher {
	return &metricMatcher{metrics: metrics}
}

// findMatchingValues finds all metric values matching the query
func (mm *metricMatcher) findMatchingValues(query *metricQuery) []float64 {
	namespaceMetrics, exists := mm.metrics[query.namespace]
	if !exists {
		return nil
	}

	var values []float64
	for _, metric := range namespaceMetrics {
		if mm.matches(metric, query) {
			values = append(values, metric.Value)
		}
	}

	return values
}

// matches checks if a metric matches the query criteria
func (mm *metricMatcher) matches(metric *MockMetricDatum, query *metricQuery) bool {
	// Check metric name
	if metric.MetricName != query.metricName {
		return false
	}

	// Check time range
	if metric.Timestamp.Before(query.startTime) || metric.Timestamp.After(query.endTime) {
		return false
	}

	// Check dimensions
	return mm.dimensionsMatch(metric.Dimensions, query.dimensions)
}

// dimensionsMatch checks if metric dimensions match the query dimensions
func (mm *metricMatcher) dimensionsMatch(metricDims, queryDims map[string]string) bool {
	if queryDims == nil {
		return true
	}

	for k, v := range queryDims {
		if metricDims[k] != v {
			return false
		}
	}

	return true
}

// statisticsCalculator handles statistics calculations
type statisticsCalculator struct {
	values []float64
}

// newStatisticsCalculator creates a new statistics calculator
func newStatisticsCalculator(values []float64) *statisticsCalculator {
	return &statisticsCalculator{values: values}
}

// calculate computes the requested statistics
func (sc *statisticsCalculator) calculate(statistics []Statistic) map[Statistic]float64 {
	result := make(map[Statistic]float64)

	for _, stat := range statistics {
		result[stat] = sc.computeStatistic(stat)
	}

	return result
}

// computeStatistic computes a single statistic
func (sc *statisticsCalculator) computeStatistic(stat Statistic) float64 {
	switch stat {
	case StatisticSampleCount:
		return float64(len(sc.values))
	case StatisticSum:
		return sc.sum()
	case StatisticAverage:
		return sc.average()
	case StatisticMinimum:
		return sc.minimum()
	case StatisticMaximum:
		return sc.maximum()
	default:
		return 0
	}
}

// sum calculates the sum of all values
func (sc *statisticsCalculator) sum() float64 {
	total := 0.0
	for _, v := range sc.values {
		total += v
	}
	return total
}

// average calculates the average of all values
func (sc *statisticsCalculator) average() float64 {
	if len(sc.values) == 0 {
		return 0
	}
	return sc.sum() / float64(len(sc.values))
}

// minimum finds the minimum value
func (sc *statisticsCalculator) minimum() float64 {
	if len(sc.values) == 0 {
		return 0
	}

	minVal := sc.values[0]
	for _, v := range sc.values[1:] {
		if v < minVal {
			minVal = v
		}
	}
	return minVal
}

// maximum finds the maximum value
func (sc *statisticsCalculator) maximum() float64 {
	if len(sc.values) == 0 {
		return 0
	}

	maxVal := sc.values[0]
	for _, v := range sc.values[1:] {
		if v > maxVal {
			maxVal = v
		}
	}
	return maxVal
}

// MockCloudWatchAlarmsClient provides a mock implementation of CloudWatch Alarms
type MockCloudWatchAlarmsClient struct {
	alarms        map[string]*MockAlarmDefinition
	callCount     map[string]int
	errors        map[string]error
	config        *MockCloudWatchConfig
	metricsClient *MockCloudWatchMetricsClient
	mu            sync.RWMutex
}

// NewMockCloudWatchAlarmsClient creates a new mock CloudWatch Alarms client
func NewMockCloudWatchAlarmsClient() *MockCloudWatchAlarmsClient {
	return &MockCloudWatchAlarmsClient{
		alarms:    make(map[string]*MockAlarmDefinition),
		callCount: make(map[string]int),
		errors:    make(map[string]error),
		config:    DefaultMockCloudWatchConfig(),
	}
}

// WithConfig sets the mock configuration
func (m *MockCloudWatchAlarmsClient) WithConfig(config *MockCloudWatchConfig) *MockCloudWatchAlarmsClient {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.config = config
	return m
}

// WithMetricsClient sets the metrics client for alarm evaluation
func (m *MockCloudWatchAlarmsClient) WithMetricsClient(client *MockCloudWatchMetricsClient) *MockCloudWatchAlarmsClient {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.metricsClient = client
	return m
}

// WithError configures an error for a specific operation
func (m *MockCloudWatchAlarmsClient) WithError(operation string, err error) *MockCloudWatchAlarmsClient {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.errors[operation] = err
	return m
}

// PutMetricAlarm creates or updates an alarm
func (m *MockCloudWatchAlarmsClient) PutMetricAlarm(_ context.Context, alarm *MockAlarmDefinition) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Track call count
	m.callCount["PutMetricAlarm"]++

	// Simulate network delay
	if m.config.NetworkDelay > 0 {
		time.Sleep(m.config.NetworkDelay)
	}

	// Check for configured error
	if err, exists := m.errors["PutMetricAlarm"]; exists {
		return err
	}

	// Validate input
	if alarm.AlarmName == "" {
		return fmt.Errorf("alarm name is required")
	}
	if alarm.MetricName == "" {
		return fmt.Errorf("metric name is required")
	}
	if alarm.Namespace == "" {
		return fmt.Errorf("namespace is required")
	}

	// Create a copy to prevent external modification
	alarmCopy := *alarm
	if alarm.Dimensions != nil {
		alarmCopy.Dimensions = make(map[string]string)
		for k, v := range alarm.Dimensions {
			alarmCopy.Dimensions[k] = v
		}
	}

	// Set timestamps
	now := time.Now()
	if alarmCopy.CreatedAt.IsZero() {
		alarmCopy.CreatedAt = now
	}
	alarmCopy.UpdatedAt = now

	// Set initial state if not provided
	if alarmCopy.State == "" {
		alarmCopy.State = AlarmStateInsufficientData
		alarmCopy.StateReason = "Insufficient Data"
		alarmCopy.StateUpdatedAt = now
	}

	m.alarms[alarm.AlarmName] = &alarmCopy

	return nil
}

// DescribeAlarms retrieves alarm information
func (m *MockCloudWatchAlarmsClient) DescribeAlarms(_ context.Context, alarmNames []string) ([]*MockAlarmDefinition, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Track call count
	m.callCount["DescribeAlarms"]++

	// Simulate network delay
	if m.config.NetworkDelay > 0 {
		time.Sleep(m.config.NetworkDelay)
	}

	// Check for configured error
	if err, exists := m.errors["DescribeAlarms"]; exists {
		return nil, err
	}

	var result []*MockAlarmDefinition

	if len(alarmNames) == 0 {
		// Return all alarms
		for _, alarm := range m.alarms {
			alarmCopy := *alarm
			if alarm.Dimensions != nil {
				alarmCopy.Dimensions = make(map[string]string)
				for k, v := range alarm.Dimensions {
					alarmCopy.Dimensions[k] = v
				}
			}
			result = append(result, &alarmCopy)
		}
	} else {
		// Return specific alarms
		for _, name := range alarmNames {
			if alarm, exists := m.alarms[name]; exists {
				alarmCopy := *alarm
				if alarm.Dimensions != nil {
					alarmCopy.Dimensions = make(map[string]string)
					for k, v := range alarm.Dimensions {
						alarmCopy.Dimensions[k] = v
					}
				}
				result = append(result, &alarmCopy)
			}
		}
	}

	return result, nil
}

// DeleteAlarms deletes one or more alarms
func (m *MockCloudWatchAlarmsClient) DeleteAlarms(_ context.Context, alarmNames []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Track call count
	m.callCount["DeleteAlarms"]++

	// Simulate network delay
	if m.config.NetworkDelay > 0 {
		time.Sleep(m.config.NetworkDelay)
	}

	// Check for configured error
	if err, exists := m.errors["DeleteAlarms"]; exists {
		return err
	}

	for _, name := range alarmNames {
		delete(m.alarms, name)
	}

	return nil
}

// EvaluateAlarms evaluates all alarms against current metrics
func (m *MockCloudWatchAlarmsClient) EvaluateAlarms(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.metricsClient == nil {
		return fmt.Errorf("metrics client not configured")
	}

	now := time.Now()

	for _, alarm := range m.alarms {
		// Calculate evaluation period
		evaluationDuration := time.Duration(alarm.Period*alarm.EvaluationPeriods) * time.Second
		startTime := now.Add(-evaluationDuration)

		// Get metric statistics
		stats, err := m.metricsClient.GetMetricStatistics(
			ctx,
			alarm.Namespace,
			alarm.MetricName,
			alarm.Dimensions,
			startTime,
			now,
			alarm.Period,
			[]Statistic{alarm.Statistic},
		)
		if err != nil {
			continue
		}

		value, exists := stats[alarm.Statistic]
		if !exists {
			// Insufficient data
			if alarm.State != AlarmStateInsufficientData {
				alarm.State = AlarmStateInsufficientData
				alarm.StateReason = "Insufficient Data"
				alarm.StateUpdatedAt = now
			}
			continue
		}

		// Evaluate threshold
		var inAlarmState bool
		switch alarm.ComparisonOperator {
		case ComparisonGreaterThanThreshold:
			inAlarmState = value > alarm.Threshold
		case ComparisonGreaterThanOrEqualToThreshold:
			inAlarmState = value >= alarm.Threshold
		case ComparisonLessThanThreshold:
			inAlarmState = value < alarm.Threshold
		case ComparisonLessThanOrEqualToThreshold:
			inAlarmState = value <= alarm.Threshold
		}

		// Update alarm state
		newState := AlarmStateOK
		newReason := fmt.Sprintf("Threshold Crossed: %f %s %f", value, alarm.ComparisonOperator, alarm.Threshold)

		if inAlarmState {
			newState = AlarmStateAlarm
		}

		if alarm.State != newState {
			alarm.State = newState
			alarm.StateReason = newReason
			alarm.StateUpdatedAt = now
		}
	}

	return nil
}

// Helper methods for testing

// GetCallCount returns the number of times an operation was called
func (m *MockCloudWatchMetricsClient) GetCallCount(operation string) int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.callCount[operation]
}

// GetMetrics returns all metrics for a namespace
func (m *MockCloudWatchMetricsClient) GetMetrics(namespace string) []*MockMetricDatum {
	m.mu.RLock()
	defer m.mu.RUnlock()

	metrics, exists := m.metrics[namespace]
	if !exists {
		return nil
	}

	// Return copies to prevent external modification
	result := make([]*MockMetricDatum, len(metrics))
	for i, metric := range metrics {
		metricCopy := *metric
		if metric.Dimensions != nil {
			metricCopy.Dimensions = make(map[string]string)
			for k, v := range metric.Dimensions {
				metricCopy.Dimensions[k] = v
			}
		}
		if metric.Metadata != nil {
			metricCopy.Metadata = make(map[string]any)
			for k, v := range metric.Metadata {
				metricCopy.Metadata[k] = v
			}
		}
		result[i] = &metricCopy
	}

	return result
}

// GetAllMetrics returns all metrics across all namespaces
func (m *MockCloudWatchMetricsClient) GetAllMetrics() map[string][]*MockMetricDatum {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string][]*MockMetricDatum)
	for namespace, metrics := range m.metrics {
		result[namespace] = make([]*MockMetricDatum, len(metrics))
		for i, metric := range metrics {
			metricCopy := *metric
			if metric.Dimensions != nil {
				metricCopy.Dimensions = make(map[string]string)
				for k, v := range metric.Dimensions {
					metricCopy.Dimensions[k] = v
				}
			}
			if metric.Metadata != nil {
				metricCopy.Metadata = make(map[string]any)
				for k, v := range metric.Metadata {
					metricCopy.Metadata[k] = v
				}
			}
			result[namespace][i] = &metricCopy
		}
	}

	return result
}

// Reset clears all mock state
func (m *MockCloudWatchMetricsClient) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.metrics = make(map[string][]*MockMetricDatum)
	m.callCount = make(map[string]int)
	m.errors = make(map[string]error)
	m.config = DefaultMockCloudWatchConfig()
}

// GetCallCount returns the number of times an operation was called
func (m *MockCloudWatchAlarmsClient) GetCallCount(operation string) int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.callCount[operation]
}

// GetAlarm returns a copy of an alarm definition
func (m *MockCloudWatchAlarmsClient) GetAlarm(alarmName string) *MockAlarmDefinition {
	m.mu.RLock()
	defer m.mu.RUnlock()

	alarm, exists := m.alarms[alarmName]
	if !exists {
		return nil
	}

	// Return a copy to prevent external modification
	alarmCopy := *alarm
	if alarm.Dimensions != nil {
		alarmCopy.Dimensions = make(map[string]string)
		for k, v := range alarm.Dimensions {
			alarmCopy.Dimensions[k] = v
		}
	}

	return &alarmCopy
}

// GetAllAlarms returns all alarm definitions
func (m *MockCloudWatchAlarmsClient) GetAllAlarms() map[string]*MockAlarmDefinition {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string]*MockAlarmDefinition)
	for name, alarm := range m.alarms {
		alarmCopy := *alarm
		if alarm.Dimensions != nil {
			alarmCopy.Dimensions = make(map[string]string)
			for k, v := range alarm.Dimensions {
				alarmCopy.Dimensions[k] = v
			}
		}
		result[name] = &alarmCopy
	}

	return result
}

// Reset clears all mock state
func (m *MockCloudWatchAlarmsClient) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.alarms = make(map[string]*MockAlarmDefinition)
	m.callCount = make(map[string]int)
	m.errors = make(map[string]error)
	m.config = DefaultMockCloudWatchConfig()
}

// =============================================================================
// End of CloudWatch Metrics & Alarms Mocks
// =============================================================================

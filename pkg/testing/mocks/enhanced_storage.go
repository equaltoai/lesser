// Package mocks provides enhanced mock implementations for testing
package mocks

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"
)

// EnhancedMockStorage provides a more sophisticated mock storage with state management
type EnhancedMockStorage struct {
	mock.Mock
	mu sync.RWMutex

	// In-memory state
	actors        map[string]*activitypub.Actor
	activities    map[string]*activitypub.Activity
	objects       map[string]interface{}
	followers     map[string][]string
	following     map[string][]string
	timeline      map[string][]*activitypub.Activity
	notifications map[string][]*storage.Notification
	sessions      map[string]*storage.Session
	users         map[string]*storage.User

	// Behavioral controls
	latencySimulation time.Duration
	errorRate         float64
	operationCounts   map[string]int
}

// NewEnhancedMockStorage creates a new enhanced mock storage instance
func NewEnhancedMockStorage() *EnhancedMockStorage {
	return &EnhancedMockStorage{
		actors:          make(map[string]*activitypub.Actor),
		activities:      make(map[string]*activitypub.Activity),
		objects:         make(map[string]interface{}),
		followers:       make(map[string][]string),
		following:       make(map[string][]string),
		timeline:        make(map[string][]*activitypub.Activity),
		notifications:   make(map[string][]*storage.Notification),
		sessions:        make(map[string]*storage.Session),
		users:           make(map[string]*storage.User),
		operationCounts: make(map[string]int),
	}
}

// SetLatencySimulation adds artificial latency to operations
func (m *EnhancedMockStorage) SetLatencySimulation(latency time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.latencySimulation = latency
}

// SetErrorRate sets the probability (0.0-1.0) that operations will fail
func (m *EnhancedMockStorage) SetErrorRate(rate float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.errorRate = rate
}

// GetOperationCount returns how many times an operation has been called
func (m *EnhancedMockStorage) GetOperationCount(operation string) int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.operationCounts[operation]
}

// ResetCounts resets all operation counts
func (m *EnhancedMockStorage) ResetCounts() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.operationCounts = make(map[string]int)
}

// simulateOperation adds latency and potential errors
func (m *EnhancedMockStorage) simulateOperation(operation string) error {
	m.mu.Lock()
	m.operationCounts[operation]++
	latency := m.latencySimulation
	errorRate := m.errorRate
	m.mu.Unlock()

	// Simulate latency
	if latency > 0 {
		time.Sleep(latency)
	}

	// Simulate errors
	if errorRate > 0 && float64(time.Now().UnixNano()%1000)/1000.0 < errorRate {
		return fmt.Errorf("simulated error for operation %s", operation)
	}

	return nil
}

// CreateActor creates an actor with state tracking
// Note: privateKey parameter is required by the Storage interface but not used in mock implementation
func (m *EnhancedMockStorage) CreateActor(_ context.Context, actor *activitypub.Actor, _ string) error { //nolint:revive // privateKey required by interface
	if err := m.simulateOperation("CreateActor"); err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if actor already exists
	if _, exists := m.actors[actor.PreferredUsername]; exists {
		return fmt.Errorf("actor %s already exists", actor.PreferredUsername)
	}

	// Store actor
	m.actors[actor.PreferredUsername] = actor
	m.followers[actor.PreferredUsername] = make([]string, 0)
	m.following[actor.PreferredUsername] = make([]string, 0)
	m.timeline[actor.PreferredUsername] = make([]*activitypub.Activity, 0)

	return nil
}

// GetActor retrieves an actor with state management
func (m *EnhancedMockStorage) GetActor(_ context.Context, username string) (*activitypub.Actor, error) {
	if err := m.simulateOperation("GetActor"); err != nil {
		return nil, err
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	actor, exists := m.actors[username]
	if !exists {
		return nil, fmt.Errorf("actor %s not found", username)
	}

	// Return a copy to prevent external modifications
	actorCopy := *actor
	return &actorCopy, nil
}

// StoreActivity stores an activity with timeline updates
func (m *EnhancedMockStorage) StoreActivity(_ context.Context, activity *activitypub.Activity) error {
	if err := m.simulateOperation("StoreActivity"); err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.activities[activity.ID] = activity

	// Update relevant timelines based on activity type
	switch activity.Type {
	case "Create", "Announce", "Like":
		if actorUsername := m.getActorUsernameFromID(activity.Actor); actorUsername != "" {
			// Add to actor's timeline
			m.timeline[actorUsername] = append([]*activitypub.Activity{activity}, m.timeline[actorUsername]...)

			// Add to followers' timelines
			for _, follower := range m.followers[actorUsername] {
				m.timeline[follower] = append([]*activitypub.Activity{activity}, m.timeline[follower]...)
			}
		}
	}

	return nil
}

// GetActivity retrieves an activity by ID
func (m *EnhancedMockStorage) GetActivity(_ context.Context, activityID string) (*activitypub.Activity, error) {
	if err := m.simulateOperation("GetActivity"); err != nil {
		return nil, err
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	activity, exists := m.activities[activityID]
	if !exists {
		return nil, fmt.Errorf("activity %s not found", activityID)
	}

	activityCopy := *activity
	return &activityCopy, nil
}

// DeleteActivity deletes an activity
func (m *EnhancedMockStorage) DeleteActivity(_ context.Context, activityID string) error {
	if err := m.simulateOperation("DeleteActivity"); err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.activities, activityID)
	return nil
}

// DeleteActor deletes an actor and all related data
func (m *EnhancedMockStorage) DeleteActor(_ context.Context, username string) error {
	if err := m.simulateOperation("DeleteActor"); err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.actors, username)
	delete(m.followers, username)
	delete(m.following, username)
	delete(m.timeline, username)

	return nil
}

// FollowActor establishes a following relationship
func (m *EnhancedMockStorage) FollowActor(_ context.Context, followerUsername, targetUsername string) error {
	if err := m.simulateOperation("FollowActor"); err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Add to follower's following list
	if following, exists := m.following[followerUsername]; exists {
		// Check if already following
		for _, f := range following {
			if f == targetUsername {
				return nil // Already following
			}
		}
		m.following[followerUsername] = append(following, targetUsername)
	} else {
		m.following[followerUsername] = []string{targetUsername}
	}

	// Add to target's followers list
	if followers, exists := m.followers[targetUsername]; exists {
		m.followers[targetUsername] = append(followers, followerUsername)
	} else {
		m.followers[targetUsername] = []string{followerUsername}
	}

	return nil
}

// UnfollowActor removes a following relationship
func (m *EnhancedMockStorage) UnfollowActor(_ context.Context, followerUsername, targetUsername string) error {
	if err := m.simulateOperation("UnfollowActor"); err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Remove from follower's following list
	if following := m.following[followerUsername]; following != nil {
		for i, f := range following {
			if f == targetUsername {
				m.following[followerUsername] = append(following[:i], following[i+1:]...)
				break
			}
		}
	}

	// Remove from target's followers list
	if followers := m.followers[targetUsername]; followers != nil {
		for i, f := range followers {
			if f == followerUsername {
				m.followers[targetUsername] = append(followers[:i], followers[i+1:]...)
				break
			}
		}
	}

	return nil
}

// GetTimeline retrieves a user's timeline
func (m *EnhancedMockStorage) GetTimeline(_ context.Context, username string, limit int, maxID string) ([]*activitypub.Activity, error) {
	if err := m.simulateOperation("GetTimeline"); err != nil {
		return nil, err
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	timeline, exists := m.timeline[username]
	if !exists {
		return []*activitypub.Activity{}, nil
	}

	// Apply pagination
	start := 0
	if maxID != "" {
		// Find the position of maxID
		for i, activity := range timeline {
			if activity.ID == maxID {
				start = i + 1
				break
			}
		}
	}

	end := start + limit
	if end > len(timeline) {
		end = len(timeline)
	}

	if start >= len(timeline) {
		return []*activitypub.Activity{}, nil
	}

	result := make([]*activitypub.Activity, end-start)
	for i, activity := range timeline[start:end] {
		activityCopy := *activity
		result[i] = &activityCopy
	}

	return result, nil
}

// Helper method to extract username from actor ID
func (m *EnhancedMockStorage) getActorUsernameFromID(actorID string) string {
	for username, actor := range m.actors {
		if actor.ID == actorID {
			return username
		}
	}
	return ""
}

// GetState returns a snapshot of the current storage state for debugging
func (m *EnhancedMockStorage) GetState() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return map[string]interface{}{
		"actors_count":     len(m.actors),
		"activities_count": len(m.activities),
		"objects_count":    len(m.objects),
		"operation_counts": m.operationCounts,
	}
}

// MockExternalService provides a mock for external service dependencies
type MockExternalService struct {
	mock.Mock
	responses  map[string]interface{}
	requestLog []MockRequest
	mu         sync.RWMutex
}

// MockRequest represents a logged request to the mock service
type MockRequest struct {
	Method    string
	URL       string
	Body      interface{}
	Headers   map[string]string
	Timestamp time.Time
}

// NewMockExternalService creates a new mock external service
func NewMockExternalService() *MockExternalService {
	return &MockExternalService{
		responses:  make(map[string]interface{}),
		requestLog: make([]MockRequest, 0),
	}
}

// SetResponse sets a mock response for a specific endpoint
func (m *MockExternalService) SetResponse(endpoint string, response interface{}) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.responses[endpoint] = response
}

// GetRequestLog returns all logged requests
func (m *MockExternalService) GetRequestLog() []MockRequest {
	m.mu.RLock()
	defer m.mu.RUnlock()
	log := make([]MockRequest, len(m.requestLog))
	copy(log, m.requestLog)
	return log
}

// ClearRequestLog clears the request log
func (m *MockExternalService) ClearRequestLog() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.requestLog = make([]MockRequest, 0)
}

// LogRequest logs a request to the mock service
func (m *MockExternalService) LogRequest(method, url string, body interface{}, headers map[string]string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.requestLog = append(m.requestLog, MockRequest{
		Method:    method,
		URL:       url,
		Body:      body,
		Headers:   headers,
		Timestamp: time.Now(),
	})
}

// MockTestLogger provides a testing logger that captures log entries
type MockTestLogger struct {
	entries []LogEntry
	mu      sync.RWMutex
}

// LogEntry represents a captured log entry
type LogEntry struct {
	Level   string
	Message string
	Fields  map[string]interface{}
	Time    time.Time
}

// NewMockTestLogger creates a new mock test logger
func NewMockTestLogger() *MockTestLogger {
	return &MockTestLogger{
		entries: make([]LogEntry, 0),
	}
}

// Info implements the info logging level
func (m *MockTestLogger) Info(msg string, fields ...zap.Field) {
	m.addEntry("info", msg, fields)
}

// Error implements the error logging level
func (m *MockTestLogger) Error(msg string, fields ...zap.Field) {
	m.addEntry("error", msg, fields)
}

// Warn implements the warn logging level
func (m *MockTestLogger) Warn(msg string, fields ...zap.Field) {
	m.addEntry("warn", msg, fields)
}

// Debug implements the debug logging level
func (m *MockTestLogger) Debug(msg string, fields ...zap.Field) {
	m.addEntry("debug", msg, fields)
}

func (m *MockTestLogger) addEntry(level, msg string, fields []zap.Field) {
	m.mu.Lock()
	defer m.mu.Unlock()

	fieldMap := make(map[string]interface{})
	for _, field := range fields {
		// This is a simplified field extraction
		fieldMap[field.Key] = field.Interface
	}

	m.entries = append(m.entries, LogEntry{
		Level:   level,
		Message: msg,
		Fields:  fieldMap,
		Time:    time.Now(),
	})
}

// GetEntries returns all captured log entries
func (m *MockTestLogger) GetEntries() []LogEntry {
	m.mu.RLock()
	defer m.mu.RUnlock()
	entries := make([]LogEntry, len(m.entries))
	copy(entries, m.entries)
	return entries
}

// GetEntriesByLevel returns entries filtered by log level
func (m *MockTestLogger) GetEntriesByLevel(level string) []LogEntry {
	m.mu.RLock()
	defer m.mu.RUnlock()

	filtered := make([]LogEntry, 0)
	for _, entry := range m.entries {
		if entry.Level == level {
			filtered = append(filtered, entry)
		}
	}
	return filtered
}

// Clear clears all captured log entries
func (m *MockTestLogger) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.entries = make([]LogEntry, 0)
}

// Package mocks provides mock implementations for testing.
package mocks

import (
	"context"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
)

// MockConversationRepository is a mock implementation of interfaces.ConversationRepository
// using testify/mock for expectation-based testing.
type MockConversationRepository struct {
	mock.Mock
}

// NewMockConversationRepository creates a new mock conversation repository
func NewMockConversationRepository() *MockConversationRepository {
	return &MockConversationRepository{}
}

// ===== Core Conversation Operations =====

// CreateConversation mocks the CreateConversation method
func (m *MockConversationRepository) CreateConversation(ctx context.Context, conversation *models.Conversation, participants []string) error {
	args := m.Called(ctx, conversation, participants)
	return args.Error(0)
}

// CreateConversationWithParticipantStates mocks the CreateConversationWithParticipantStates method
func (m *MockConversationRepository) CreateConversationWithParticipantStates(ctx context.Context, conversation *models.Conversation, participants []string, participantStates []*models.UserConversationState) error {
	args := m.Called(ctx, conversation, participants, participantStates)
	return args.Error(0)
}

// GetConversation mocks the GetConversation method
func (m *MockConversationRepository) GetConversation(ctx context.Context, id string) (*models.Conversation, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Conversation), args.Error(1)
}

// UpdateConversation mocks the UpdateConversation method
func (m *MockConversationRepository) UpdateConversation(ctx context.Context, conversation *models.Conversation) error {
	args := m.Called(ctx, conversation)
	return args.Error(0)
}

// DeleteConversation mocks the DeleteConversation method
func (m *MockConversationRepository) DeleteConversation(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

// ===== User Conversation Operations =====

// GetUserConversations mocks the GetUserConversations method
func (m *MockConversationRepository) GetUserConversations(ctx context.Context, userID string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Conversation], error) {
	args := m.Called(ctx, userID, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*interfaces.PaginatedResult[*models.Conversation]), args.Error(1)
}

// GetUserConversationsByFolder mocks the GetUserConversationsByFolder method.
func (m *MockConversationRepository) GetUserConversationsByFolder(ctx context.Context, userID string, folder models.UserConversationFolder, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Conversation], error) {
	args := m.Called(ctx, userID, folder, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*interfaces.PaginatedResult[*models.Conversation]), args.Error(1)
}

// GetUserConversationsByRequestState mocks the GetUserConversationsByRequestState method
func (m *MockConversationRepository) GetUserConversationsByRequestState(ctx context.Context, userID string, requestState models.DmRequestState, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Conversation], error) {
	args := m.Called(ctx, userID, requestState, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*interfaces.PaginatedResult[*models.Conversation]), args.Error(1)
}

// GetConversationByParticipants mocks the GetConversationByParticipants method
func (m *MockConversationRepository) GetConversationByParticipants(ctx context.Context, participants []string) (*models.Conversation, error) {
	args := m.Called(ctx, participants)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Conversation), args.Error(1)
}

// GetUnreadConversations mocks the GetUnreadConversations method
func (m *MockConversationRepository) GetUnreadConversations(ctx context.Context, userID string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Conversation], error) {
	args := m.Called(ctx, userID, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*interfaces.PaginatedResult[*models.Conversation]), args.Error(1)
}

// SearchConversations mocks the SearchConversations method
func (m *MockConversationRepository) SearchConversations(ctx context.Context, userID, query string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Conversation], error) {
	args := m.Called(ctx, userID, query, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*interfaces.PaginatedResult[*models.Conversation]), args.Error(1)
}

// ===== Read Status Operations =====

// MarkConversationRead mocks the MarkConversationRead method
func (m *MockConversationRepository) MarkConversationRead(ctx context.Context, conversationID, username string) error {
	args := m.Called(ctx, conversationID, username)
	return args.Error(0)
}

// MarkConversationUnread mocks the MarkConversationUnread method
func (m *MockConversationRepository) MarkConversationUnread(ctx context.Context, conversationID, userID string) error {
	args := m.Called(ctx, conversationID, userID)
	return args.Error(0)
}

// GetUnreadConversationCount mocks the GetUnreadConversationCount method
func (m *MockConversationRepository) GetUnreadConversationCount(ctx context.Context, username string) (int, error) {
	args := m.Called(ctx, username)
	return args.Int(0), args.Error(1)
}

// ===== Status/Message Operations =====

// AddStatusToConversation mocks the AddStatusToConversation method
func (m *MockConversationRepository) AddStatusToConversation(ctx context.Context, conversationID, statusID, senderUsername string) error {
	args := m.Called(ctx, conversationID, statusID, senderUsername)
	return args.Error(0)
}

// GetConversationStatuses mocks the GetConversationStatuses method
func (m *MockConversationRepository) GetConversationStatuses(ctx context.Context, conversationID string, limit int, cursor string) ([]*storage.ConversationStatus, string, error) {
	args := m.Called(ctx, conversationID, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*storage.ConversationStatus), args.String(1), args.Error(2)
}

// RemoveStatusFromConversation mocks the RemoveStatusFromConversation method
func (m *MockConversationRepository) RemoveStatusFromConversation(ctx context.Context, conversationID, statusID string) error {
	args := m.Called(ctx, conversationID, statusID)
	return args.Error(0)
}

// MarkStatusRead mocks the MarkStatusRead method
func (m *MockConversationRepository) MarkStatusRead(ctx context.Context, conversationID, statusID, username string) error {
	args := m.Called(ctx, conversationID, statusID, username)
	return args.Error(0)
}

// GetUnreadStatusCount mocks the GetUnreadStatusCount method
func (m *MockConversationRepository) GetUnreadStatusCount(ctx context.Context, conversationID, username string) (int, error) {
	args := m.Called(ctx, conversationID, username)
	return args.Int(0), args.Error(1)
}

// UpdateConversationLastStatus mocks the UpdateConversationLastStatus method
func (m *MockConversationRepository) UpdateConversationLastStatus(ctx context.Context, id, lastStatusID string) error {
	args := m.Called(ctx, id, lastStatusID)
	return args.Error(0)
}

// ApplyDirectMessageSend mocks the ApplyDirectMessageSend method.
func (m *MockConversationRepository) ApplyDirectMessageSend(ctx context.Context, transition *models.DirectMessageSendTransition) error {
	args := m.Called(ctx, transition)
	return args.Error(0)
}

// ===== Participant Operations =====

// AddParticipant mocks the AddParticipant method
func (m *MockConversationRepository) AddParticipant(ctx context.Context, conversationID, participantID string) error {
	args := m.Called(ctx, conversationID, participantID)
	return args.Error(0)
}

// RemoveParticipant mocks the RemoveParticipant method
func (m *MockConversationRepository) RemoveParticipant(ctx context.Context, conversationID, participantID string) error {
	args := m.Called(ctx, conversationID, participantID)
	return args.Error(0)
}

// GetConversationParticipants mocks the GetConversationParticipants method
func (m *MockConversationRepository) GetConversationParticipants(ctx context.Context, conversationID string) ([]string, error) {
	args := m.Called(ctx, conversationID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]string), args.Error(1)
}

// GetConversationParticipantRecord mocks the GetConversationParticipantRecord method
func (m *MockConversationRepository) GetConversationParticipantRecord(ctx context.Context, conversationID, participantID string) (*models.ConversationParticipantRecord, error) {
	args := m.Called(ctx, conversationID, participantID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.ConversationParticipantRecord), args.Error(1)
}

// UpdateConversationParticipantRecord mocks the UpdateConversationParticipantRecord method
func (m *MockConversationRepository) UpdateConversationParticipantRecord(ctx context.Context, record *models.ConversationParticipantRecord) error {
	args := m.Called(ctx, record)
	return args.Error(0)
}

// LeaveConversation mocks the LeaveConversation method
func (m *MockConversationRepository) LeaveConversation(ctx context.Context, conversationID, username string) error {
	args := m.Called(ctx, conversationID, username)
	return args.Error(0)
}

// ===== Mute Operations =====

// CreateConversationMute mocks the CreateConversationMute method
func (m *MockConversationRepository) CreateConversationMute(ctx context.Context, mute *storage.ConversationMute) error {
	args := m.Called(ctx, mute)
	return args.Error(0)
}

// DeleteConversationMute mocks the DeleteConversationMute method
func (m *MockConversationRepository) DeleteConversationMute(ctx context.Context, username, conversationID string) error {
	args := m.Called(ctx, username, conversationID)
	return args.Error(0)
}

// IsConversationMuted mocks the IsConversationMuted method
func (m *MockConversationRepository) IsConversationMuted(ctx context.Context, username, conversationID string) (bool, error) {
	args := m.Called(ctx, username, conversationID)
	return args.Bool(0), args.Error(1)
}

// GetMutedConversations mocks the GetMutedConversations method
func (m *MockConversationRepository) GetMutedConversations(ctx context.Context, username string) ([]string, error) {
	args := m.Called(ctx, username)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]string), args.Error(1)
}

// Ensure MockConversationRepository implements interfaces.ConversationRepository
var _ interfaces.ConversationRepository = (*MockConversationRepository)(nil)

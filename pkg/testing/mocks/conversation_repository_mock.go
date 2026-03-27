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

// GetUserConversationState mocks the GetUserConversationState method.
func (m *MockConversationRepository) GetUserConversationState(ctx context.Context, viewerID, conversationID string) (*interfaces.UserConversationStateContract, error) {
	args := m.Called(ctx, viewerID, conversationID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*interfaces.UserConversationStateContract), args.Error(1)
}

// ListUserConversationStatesByFolder mocks the ListUserConversationStatesByFolder method.
func (m *MockConversationRepository) ListUserConversationStatesByFolder(ctx context.Context, viewerID string, folder interfaces.UserConversationFolder, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*interfaces.UserConversationStateContract], error) {
	args := m.Called(ctx, viewerID, folder, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*interfaces.PaginatedResult[*interfaces.UserConversationStateContract]), args.Error(1)
}

// ListUnreadUserConversationStates mocks the ListUnreadUserConversationStates method.
func (m *MockConversationRepository) ListUnreadUserConversationStates(ctx context.Context, viewerID string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*interfaces.UserConversationStateContract], error) {
	args := m.Called(ctx, viewerID, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*interfaces.PaginatedResult[*interfaces.UserConversationStateContract]), args.Error(1)
}

// ListConversationParticipantStates mocks the ListConversationParticipantStates method.
func (m *MockConversationRepository) ListConversationParticipantStates(ctx context.Context, conversationID string) ([]*interfaces.UserConversationStateContract, error) {
	args := m.Called(ctx, conversationID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*interfaces.UserConversationStateContract), args.Error(1)
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

// ApplyDirectMessageSend mocks the ApplyDirectMessageSend method.
func (m *MockConversationRepository) ApplyDirectMessageSend(ctx context.Context, transition *models.DirectMessageSendTransition, stageStatusCreate interfaces.DirectMessageStatusStageFn) error {
	args := m.Called(ctx, transition, stageStatusCreate)
	return args.Error(0)
}

// ===== Participant Operations =====

// GetConversationParticipants mocks the GetConversationParticipants method
func (m *MockConversationRepository) GetConversationParticipants(ctx context.Context, conversationID string) ([]string, error) {
	args := m.Called(ctx, conversationID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]string), args.Error(1)
}

// PutUserConversationState mocks the PutUserConversationState method.
func (m *MockConversationRepository) PutUserConversationState(ctx context.Context, state *models.UserConversationState) error {
	args := m.Called(ctx, state)
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

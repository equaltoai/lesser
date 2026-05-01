// Package mocks provides mock implementations for testing.
package mocks

import (
	"context"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
)

// MockDraftRepository is a mock implementation of interfaces.DraftRepository
// using testify/mock for expectation-based testing.
type MockDraftRepository struct {
	mock.Mock
}

// NewMockDraftRepository creates a new mock draft repository
func NewMockDraftRepository() *MockDraftRepository {
	return &MockDraftRepository{}
}

// ===== Core CRUD Operations =====

// CreateDraft mocks the CreateDraft method
func (m *MockDraftRepository) CreateDraft(ctx context.Context, draft *models.Draft) error {
	args := m.Called(ctx, draft)
	return args.Error(0)
}

// GetDraft mocks the GetDraft method
func (m *MockDraftRepository) GetDraft(ctx context.Context, authorID, draftID string) (*models.Draft, error) {
	args := m.Called(ctx, authorID, draftID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Draft), args.Error(1)
}

// UpdateDraft mocks the UpdateDraft method
func (m *MockDraftRepository) UpdateDraft(ctx context.Context, authorID string, draft *models.Draft) error {
	args := m.Called(ctx, authorID, draft)
	return args.Error(0)
}

// DeleteDraft mocks the DeleteDraft method
func (m *MockDraftRepository) DeleteDraft(ctx context.Context, authorID, draftID string) error {
	args := m.Called(ctx, authorID, draftID)
	return args.Error(0)
}

// ===== List Operations =====

// ListDraftsByAuthor mocks the ListDraftsByAuthor method
func (m *MockDraftRepository) ListDraftsByAuthor(ctx context.Context, authorID string, limit int) ([]*models.Draft, error) {
	args := m.Called(ctx, authorID, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.Draft), args.Error(1)
}

// ListDraftsByAuthorPaginated mocks the ListDraftsByAuthorPaginated method
func (m *MockDraftRepository) ListDraftsByAuthorPaginated(ctx context.Context, authorID string, limit int, cursor string) ([]*models.Draft, string, error) {
	args := m.Called(ctx, authorID, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*models.Draft), args.String(1), args.Error(2)
}

// ===== Scheduled Operations =====

// ListScheduledDraftsDuePaginated mocks the ListScheduledDraftsDuePaginated method
func (m *MockDraftRepository) ListScheduledDraftsDuePaginated(ctx context.Context, dueBefore time.Time, limit int, cursor string) ([]*models.Draft, string, error) {
	args := m.Called(ctx, dueBefore, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*models.Draft), args.String(1), args.Error(2)
}

// Ensure MockDraftRepository implements interfaces.DraftRepository
var _ interfaces.DraftRepository = (*MockDraftRepository)(nil)

// Package mocks provides mock implementations for testing.
package mocks

import (
	"context"

	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	dynamormcore "github.com/theory-cloud/tabletheory/v2/pkg/core"
)

// MockArticleRepository is a mock implementation of interfaces.ArticleRepository
// using testify/mock for expectation-based testing.
type MockArticleRepository struct {
	mock.Mock
}

// NewMockArticleRepository creates a new mock article repository
func NewMockArticleRepository() *MockArticleRepository {
	return &MockArticleRepository{}
}

// ===== Database Access =====

// GetDB mocks the GetDB method
func (m *MockArticleRepository) GetDB() dynamormcore.DB {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(dynamormcore.DB)
}

// ===== Core CRUD Operations =====

// CreateArticle mocks the CreateArticle method
func (m *MockArticleRepository) CreateArticle(ctx context.Context, article *models.Article) error {
	args := m.Called(ctx, article)
	return args.Error(0)
}

// GetArticle mocks the GetArticle method
func (m *MockArticleRepository) GetArticle(ctx context.Context, id string) (*models.Article, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Article), args.Error(1)
}

// UpdateArticle mocks the UpdateArticle method
func (m *MockArticleRepository) UpdateArticle(ctx context.Context, article *models.Article) error {
	args := m.Called(ctx, article)
	return args.Error(0)
}

// DeleteArticle mocks the DeleteArticle method
func (m *MockArticleRepository) DeleteArticle(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

// ===== List Operations =====

// ListArticles mocks the ListArticles method
func (m *MockArticleRepository) ListArticles(ctx context.Context, limit int) ([]*models.Article, error) {
	args := m.Called(ctx, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.Article), args.Error(1)
}

// ListArticlesPaginated mocks the ListArticlesPaginated method
func (m *MockArticleRepository) ListArticlesPaginated(ctx context.Context, limit int, cursor string) ([]*models.Article, string, error) {
	args := m.Called(ctx, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*models.Article), args.String(1), args.Error(2)
}

// ===== Filtered List Operations =====

// ListArticlesByAuthorPaginated mocks the ListArticlesByAuthorPaginated method
func (m *MockArticleRepository) ListArticlesByAuthorPaginated(ctx context.Context, authorActorID string, limit int, cursor string) ([]*models.Article, string, error) {
	args := m.Called(ctx, authorActorID, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*models.Article), args.String(1), args.Error(2)
}

// ListArticlesBySeriesPaginated mocks the ListArticlesBySeriesPaginated method
func (m *MockArticleRepository) ListArticlesBySeriesPaginated(ctx context.Context, seriesID string, limit int, cursor string) ([]*models.Article, string, error) {
	args := m.Called(ctx, seriesID, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*models.Article), args.String(1), args.Error(2)
}

// ListArticlesByCategoryPaginated mocks the ListArticlesByCategoryPaginated method
func (m *MockArticleRepository) ListArticlesByCategoryPaginated(ctx context.Context, categoryID string, limit int, cursor string) ([]*models.Article, string, error) {
	args := m.Called(ctx, categoryID, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*models.Article), args.String(1), args.Error(2)
}

// Ensure MockArticleRepository implements interfaces.ArticleRepository
var _ interfaces.ArticleRepository = (*MockArticleRepository)(nil)

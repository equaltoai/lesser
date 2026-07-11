// Package inmemory provides thread-safe in-memory implementations of repository interfaces.
package inmemory

import (
	"context"
	"sort"
	"strings"
	"sync"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	dynamormcore "github.com/theory-cloud/tabletheory/v2/pkg/core"
)

// ArticleRepository is a thread-safe in-memory implementation of interfaces.ArticleRepository.
type ArticleRepository struct {
	mu sync.RWMutex

	// Articles by ID
	articlesByID map[string]*models.Article

	// Articles by author: authorActorID -> []articleID
	articlesByAuthor map[string][]string

	// Articles by series: seriesID -> []articleID
	articlesBySeries map[string][]string

	// Articles by category: categoryID -> []articleID
	articlesByCategory map[string][]string
}

// NewArticleRepository creates a new in-memory article repository
func NewArticleRepository() *ArticleRepository {
	return &ArticleRepository{
		articlesByID:       make(map[string]*models.Article),
		articlesByAuthor:   make(map[string][]string),
		articlesBySeries:   make(map[string][]string),
		articlesByCategory: make(map[string][]string),
	}
}

// CreateArticle creates a new article
func (r *ArticleRepository) CreateArticle(_ context.Context, article *models.Article) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if article == nil || article.ID == "" {
		return storage.ErrInvalidInput
	}

	if _, exists := r.articlesByID[article.ID]; exists {
		return storage.ErrAlreadyExists
	}

	// Store article
	r.articlesByID[article.ID] = article

	// Index by author
	if article.AttributedTo != "" {
		r.articlesByAuthor[article.AttributedTo] = append(r.articlesByAuthor[article.AttributedTo], article.ID)
	}

	// Index by series
	if article.SeriesID != nil && *article.SeriesID != "" {
		r.articlesBySeries[*article.SeriesID] = append(r.articlesBySeries[*article.SeriesID], article.ID)
	}

	// Index by categories
	for _, catID := range article.CategoryIDs {
		r.articlesByCategory[catID] = append(r.articlesByCategory[catID], article.ID)
	}

	return nil
}

// GetArticle retrieves an article by ID
func (r *ArticleRepository) GetArticle(_ context.Context, id string) (*models.Article, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	article, exists := r.articlesByID[id]
	if !exists {
		return nil, storage.ErrNotFound
	}

	return article, nil
}

// UpdateArticle updates an existing article
func (r *ArticleRepository) UpdateArticle(_ context.Context, article *models.Article) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if article == nil || article.ID == "" {
		return storage.ErrInvalidInput
	}

	if _, exists := r.articlesByID[article.ID]; !exists {
		return storage.ErrNotFound
	}

	r.articlesByID[article.ID] = article
	return nil
}

// DeleteArticle deletes an article
func (r *ArticleRepository) DeleteArticle(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	article, exists := r.articlesByID[id]
	if !exists {
		return storage.ErrNotFound
	}

	// Remove from author index
	if article.AttributedTo != "" {
		r.articlesByAuthor[article.AttributedTo] = articleRemoveFromSlice(r.articlesByAuthor[article.AttributedTo], id)
	}

	// Remove from series index
	if article.SeriesID != nil && *article.SeriesID != "" {
		r.articlesBySeries[*article.SeriesID] = articleRemoveFromSlice(r.articlesBySeries[*article.SeriesID], id)
	}

	// Remove from category indexes
	for _, catID := range article.CategoryIDs {
		r.articlesByCategory[catID] = articleRemoveFromSlice(r.articlesByCategory[catID], id)
	}

	delete(r.articlesByID, id)
	return nil
}

// ListArticles lists articles with a limit
func (r *ArticleRepository) ListArticles(ctx context.Context, limit int) ([]*models.Article, error) {
	articles, _, err := r.ListArticlesPaginated(ctx, limit, "")
	return articles, err
}

// ListArticlesPaginated lists articles with cursor pagination
func (r *ArticleRepository) ListArticlesPaginated(_ context.Context, limit int, cursor string) ([]*models.Article, string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if limit <= 0 {
		limit = 25
	}

	// Get all articles and sort by GSI2SK (published time) descending
	articles := make([]*models.Article, 0, len(r.articlesByID))
	for _, article := range r.articlesByID {
		articles = append(articles, article)
	}

	sort.Slice(articles, func(i, j int) bool {
		return articles[i].GSI2SK > articles[j].GSI2SK
	})

	// Apply cursor
	startIdx := 0
	cursor = strings.TrimSpace(cursor)
	if cursor != "" {
		for i, article := range articles {
			if article.GSI2SK < cursor {
				startIdx = i
				break
			}
		}
	}

	// Apply limit
	endIdx := startIdx + limit
	if endIdx > len(articles) {
		endIdx = len(articles)
	}

	result := articles[startIdx:endIdx]
	nextCursor := ""
	if endIdx < len(articles) && len(result) > 0 {
		nextCursor = result[len(result)-1].GSI2SK
	}

	return result, nextCursor, nil
}

// ListArticlesByAuthorPaginated lists articles for a specific author with pagination
func (r *ArticleRepository) ListArticlesByAuthorPaginated(_ context.Context, authorActorID string, limit int, cursor string) ([]*models.Article, string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if limit <= 0 {
		limit = 25
	}

	authorActorID = strings.TrimSpace(authorActorID)
	if authorActorID == "" {
		return []*models.Article{}, "", nil
	}

	articleIDs := r.articlesByAuthor[authorActorID]
	return r.paginateArticlesByIDs(articleIDs, limit, cursor)
}

// ListArticlesBySeriesPaginated lists articles for a specific series with pagination
func (r *ArticleRepository) ListArticlesBySeriesPaginated(_ context.Context, seriesID string, limit int, cursor string) ([]*models.Article, string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if limit <= 0 {
		limit = 25
	}

	seriesID = strings.TrimSpace(seriesID)
	if seriesID == "" {
		return []*models.Article{}, "", nil
	}

	articleIDs := r.articlesBySeries[seriesID]
	return r.paginateArticlesByIDs(articleIDs, limit, cursor)
}

// ListArticlesByCategoryPaginated lists articles for a specific category with pagination
func (r *ArticleRepository) ListArticlesByCategoryPaginated(_ context.Context, categoryID string, limit int, cursor string) ([]*models.Article, string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if limit <= 0 {
		limit = 25
	}

	categoryID = strings.TrimSpace(categoryID)
	if categoryID == "" {
		return []*models.Article{}, "", nil
	}

	articleIDs := r.articlesByCategory[categoryID]
	return r.paginateArticlesByIDs(articleIDs, limit, cursor)
}

// paginateArticlesByIDs is a helper to paginate articles by their IDs
func (r *ArticleRepository) paginateArticlesByIDs(articleIDs []string, limit int, cursor string) ([]*models.Article, string, error) {
	// Get articles and sort by published time descending
	articles := make([]*models.Article, 0, len(articleIDs))
	for _, id := range articleIDs {
		if article, exists := r.articlesByID[id]; exists {
			articles = append(articles, article)
		}
	}

	sort.Slice(articles, func(i, j int) bool {
		return articles[i].GSI2SK > articles[j].GSI2SK
	})

	// Apply cursor
	startIdx := 0
	cursor = strings.TrimSpace(cursor)
	if cursor != "" {
		for i, article := range articles {
			if article.GSI2SK < cursor {
				startIdx = i
				break
			}
		}
	}

	// Apply limit
	endIdx := startIdx + limit
	if endIdx > len(articles) {
		endIdx = len(articles)
	}

	result := articles[startIdx:endIdx]
	nextCursor := ""
	if endIdx < len(articles) && len(result) > 0 {
		nextCursor = result[len(result)-1].GSI2SK
	}

	return result, nextCursor, nil
}

// Clear clears all data (test helper)
func (r *ArticleRepository) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.articlesByID = make(map[string]*models.Article)
	r.articlesByAuthor = make(map[string][]string)
	r.articlesBySeries = make(map[string][]string)
	r.articlesByCategory = make(map[string][]string)
}

// GetDB returns the underlying DynamoDB connection.
// For in-memory implementation, this returns nil.
func (r *ArticleRepository) GetDB() dynamormcore.DB {
	return nil
}

// articleRemoveFromSlice removes an element from a slice
func articleRemoveFromSlice(slice []string, element string) []string {
	result := make([]string, 0, len(slice))
	for _, s := range slice {
		if s != element {
			result = append(result, s)
		}
	}
	return result
}

// Ensure ArticleRepository implements interfaces.ArticleRepository
var _ interfaces.ArticleRepository = (*ArticleRepository)(nil)

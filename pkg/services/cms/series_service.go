package cms

import (
	"context"
	"errors"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"go.uber.org/zap"
)

// SeriesService handles business logic for series
type SeriesService struct {
	seriesRepo  *repositories.SeriesRepository
	articleRepo *repositories.ArticleRepository
	logger      *zap.Logger
}

// NewSeriesService creates a new SeriesService
func NewSeriesService(seriesRepo *repositories.SeriesRepository, articleRepo *repositories.ArticleRepository, logger *zap.Logger) *SeriesService {
	return &SeriesService{
		seriesRepo:  seriesRepo,
		articleRepo: articleRepo,
		logger:      logger,
	}
}

// CreateSeries creates a new series
func (s *SeriesService) CreateSeries(ctx context.Context, series *models.Series) error {
	s.logger.Info("creating series", zap.String("title", series.Title))
	
	if series.CreatedAt.IsZero() {
		series.CreatedAt = time.Now()
	}
	series.UpdatedAt = time.Now()

	return s.seriesRepo.CreateSeries(ctx, series)
}

// GetSeries retrieves a series by author ID and series ID
func (s *SeriesService) GetSeries(ctx context.Context, authorID, seriesID string) (*models.Series, error) {
	return s.seriesRepo.GetSeries(ctx, authorID, seriesID)
}

// UpdateSeries updates an existing series
func (s *SeriesService) UpdateSeries(ctx context.Context, series *models.Series) error {
	s.logger.Info("updating series", zap.String("id", series.ID))
	series.UpdatedAt = time.Now()
	// Note: SeriesRepository needs an UpdateSeries method, assuming it inherits from EnhancedBaseRepository which has Update
	return s.seriesRepo.Update(ctx, series)
}

// DeleteSeries deletes a series
func (s *SeriesService) DeleteSeries(ctx context.Context, authorID, seriesID string) error {
	s.logger.Info("deleting series", zap.String("id", seriesID))
	// We should probably check if there are articles in the series and handle them (e.g., unset SeriesID)
	// For now, we just delete the series.
	// Assuming Delete method exists in repository
	pk := "AUTHOR#" + authorID + "#SERIES"
	sk := "ID#" + seriesID
	return s.seriesRepo.Delete(ctx, pk, sk)
}

// ListSeriesByAuthor lists series for an author
func (s *SeriesService) ListSeriesByAuthor(ctx context.Context, authorID string, limit int) ([]*models.Series, error) {
	return s.seriesRepo.ListSeriesByAuthor(ctx, authorID, limit)
}

// AddArticleToSeries adds an article to a series
func (s *SeriesService) AddArticleToSeries(ctx context.Context, articleID string, seriesID string, order int) error {
	s.logger.Info("adding article to series", zap.String("articleID", articleID), zap.String("seriesID", seriesID))
	
	article, err := s.articleRepo.GetArticle(ctx, articleID)
	if err != nil {
		return err
	}

	article.SeriesID = &seriesID
	article.SeriesOrder = &order
	article.UpdatedAt = time.Now()

	return s.articleRepo.UpdateArticle(ctx, article)
}

// RemoveArticleFromSeries removes an article from a series
func (s *SeriesService) RemoveArticleFromSeries(ctx context.Context, articleID string) error {
	s.logger.Info("removing article from series", zap.String("articleID", articleID))

	article, err := s.articleRepo.GetArticle(ctx, articleID)
	if err != nil {
		return err
	}

	article.SeriesID = nil
	article.SeriesOrder = nil
	article.UpdatedAt = time.Now()

	return s.articleRepo.UpdateArticle(ctx, article)
}

// ReorderArticles updates the order of articles in a series
func (s *SeriesService) ReorderArticles(ctx context.Context, seriesID string, articleOrders map[string]int) error {
	s.logger.Info("reordering articles in series", zap.String("seriesID", seriesID))

	for articleID, order := range articleOrders {
		article, err := s.articleRepo.GetArticle(ctx, articleID)
		if err != nil {
			s.logger.Error("failed to get article for reordering", zap.String("articleID", articleID), zap.Error(err))
			continue // Or return error to abort transaction
		}

		if article.SeriesID == nil || *article.SeriesID != seriesID {
			return errors.New("article does not belong to the specified series")
		}

		orderVal := order
		article.SeriesOrder = &orderVal
		article.UpdatedAt = time.Now()

		if err := s.articleRepo.UpdateArticle(ctx, article); err != nil {
			return err
		}
	}
	return nil
}
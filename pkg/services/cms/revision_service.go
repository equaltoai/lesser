package cms

import (
	"context"
	"encoding/json"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"go.uber.org/zap"
)

// RevisionService handles business logic for content revisions
type RevisionService struct {
	revisionRepo *repositories.RevisionRepository
	articleRepo  *repositories.ArticleRepository
	logger       *zap.Logger
}

// NewRevisionService creates a new RevisionService
func NewRevisionService(revisionRepo *repositories.RevisionRepository, articleRepo *repositories.ArticleRepository, logger *zap.Logger) *RevisionService {
	return &RevisionService{
		revisionRepo: revisionRepo,
		articleRepo:  articleRepo,
		logger:       logger,
	}
}

// CreateRevision creates a new revision from an article state
func (s *RevisionService) CreateRevision(ctx context.Context, article *models.Article) (*models.Revision, error) {
	s.logger.Info("creating revision", zap.String("article_id", article.ID))

	// Determine next version number
	// Ideally this should be transactional or based on a counter, but for now we count existing revisions
	revisions, err := s.revisionRepo.ListRevisions(ctx, article.ID, 1)
	nextVersion := 1
	if err == nil && len(revisions) > 0 {
		nextVersion = revisions[0].Version + 1
	}

	// Snapshot metadata as JSON for later restoration
	metadataSnapshot := map[string]interface{}{
		"name":         article.Name,
		"summary":      article.Summary,
		"attributedTo": article.AttributedTo,
	}
	metadataJSON, _ := json.Marshal(metadataSnapshot)

	revision := &models.Revision{
		ObjectID:     article.ID,
		Version:      nextVersion,
		Content:      article.Content,
		MetadataJSON: string(metadataJSON),
		ChangedBy:    article.AttributedTo,
		ChangeType:   "update",
		CreatedAt:    time.Now(),
	}

	if err := s.revisionRepo.CreateRevision(ctx, revision); err != nil {
		s.logger.Error("failed to create revision", zap.Error(err))
		return nil, err
	}

	return revision, nil
}

// ListRevisions lists revisions for a given article
func (s *RevisionService) ListRevisions(ctx context.Context, objectID string, limit int) ([]*models.Revision, error) {
	return s.revisionRepo.ListRevisions(ctx, objectID, limit)
}

// GetRevision retrieves a specific revision
func (s *RevisionService) GetRevision(ctx context.Context, objectID string, version int) (*models.Revision, error) {
	return s.revisionRepo.GetRevision(ctx, objectID, version)
}

// RestoreRevision reverts an article to a specific revision
func (s *RevisionService) RestoreRevision(ctx context.Context, objectID string, version int) (*models.Article, error) {
	s.logger.Info("restoring revision", zap.String("object_id", objectID), zap.Int("version", version))

	// Get the revision
	revision, err := s.revisionRepo.GetRevision(ctx, objectID, version)
	if err != nil {
		return nil, err
	}

	// Get the current article
	article, err := s.articleRepo.GetArticle(ctx, objectID)
	if err != nil {
		return nil, err
	}

	// Create a revision of the CURRENT state before overwriting (safety net)
	_, err = s.CreateRevision(ctx, article)
	if err != nil {
		s.logger.Warn("failed to create safety revision before restore", zap.Error(err))
		// Proceed anyway? Or fail? Let's proceed but log warning.
	}

	// Update article with revision content
	article.Content = revision.Content

	// Restore metadata from JSON if available
	if revision.MetadataJSON != "" {
		var metadata map[string]interface{}
		if err := json.Unmarshal([]byte(revision.MetadataJSON), &metadata); err == nil {
			if name, ok := metadata["name"].(string); ok {
				article.Name = name
			}
			if summary, ok := metadata["summary"].(string); ok {
				article.Summary = summary
			}
		}
	}
	article.Updated = time.Now()
	article.UpdatedAt = time.Now()

	if err := s.articleRepo.UpdateArticle(ctx, article); err != nil {
		s.logger.Error("failed to update article for restore", zap.Error(err))
		return nil, err
	}

	return article, nil
}

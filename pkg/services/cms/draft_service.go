package cms

import (
	"context"
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"go.uber.org/zap"
)

// DraftService handles business logic for drafts
type DraftService struct {
	draftRepo      *repositories.DraftRepository
	articleService *ArticleService
	logger         *zap.Logger
}

// NewDraftService creates a new DraftService
func NewDraftService(draftRepo *repositories.DraftRepository, articleService *ArticleService, logger *zap.Logger) *DraftService {
	return &DraftService{
		draftRepo:      draftRepo,
		articleService: articleService,
		logger:         logger,
	}
}

// CreateDraft creates a new draft
func (s *DraftService) CreateDraft(ctx context.Context, draft *models.Draft) error {
	s.logger.Info("creating draft", zap.String("title", draft.Title))

	if draft.CreatedAt.IsZero() {
		draft.CreatedAt = time.Now()
	}
	draft.UpdatedAt = time.Now()
	draft.LastSavedAt = time.Now()

	return s.draftRepo.CreateDraft(ctx, draft)
}

// UpdateDraft updates an existing draft
func (s *DraftService) UpdateDraft(ctx context.Context, draft *models.Draft) error {
	draft.UpdatedAt = time.Now()
	return s.draftRepo.UpdateDraft(ctx, draft)
}

// Autosave updates the draft content without changing its primary status
func (s *DraftService) Autosave(ctx context.Context, draft *models.Draft) error {
	s.logger.Debug("autosaving draft", zap.String("id", draft.ID))
	draft.LastSavedAt = time.Now()
	draft.AutosaveVersion++
	return s.draftRepo.UpdateDraft(ctx, draft)
}

// GetDraft retrieves a draft
func (s *DraftService) GetDraft(ctx context.Context, authorID, draftID string) (*models.Draft, error) {
	return s.draftRepo.GetDraft(ctx, authorID, draftID)
}

// DeleteDraft deletes a draft
func (s *DraftService) DeleteDraft(ctx context.Context, authorID, draftID string) error {
	return s.draftRepo.DeleteDraft(ctx, authorID, draftID)
}

// ScheduleDraft schedules a draft for publishing
func (s *DraftService) ScheduleDraft(ctx context.Context, authorID, draftID string, scheduledAt time.Time) error {
	draft, err := s.draftRepo.GetDraft(ctx, authorID, draftID)
	if err != nil {
		return err
	}

	draft.ScheduledAt = &scheduledAt
	return s.draftRepo.UpdateDraft(ctx, draft)
}

// PublishDraft converts a draft into an article
func (s *DraftService) PublishDraft(ctx context.Context, authorID, draftID string) (*models.Article, error) {
	s.logger.Info("publishing draft", zap.String("draft_id", draftID))

	draft, err := s.draftRepo.GetDraft(ctx, authorID, draftID)
	if err != nil {
		return nil, err
	}

	// Create Article from Draft
	// Note: ID generation strategy should be consistent.
	// Here we assume a new ID is generated or we use the draft ID if appropriate.
	// Usually articles have their own ID space.
	article := &models.Article{
		Object: models.Object{
			ID:           fmt.Sprintf("article_%d", time.Now().UnixNano()), // Simple ID generation
			Type:         "Article",
			Name:         draft.Title,
			Content:      draft.Content,
			AttributedTo: draft.AuthorID,
			Published:    time.Now(),
			Updated:      time.Now(),
			CreatedAt:    time.Now(),
		},
		ContentFormat: draft.ContentFormat,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}

	if err := s.articleService.CreateArticle(ctx, article); err != nil {
		return nil, err
	}

	// Delete the draft after successful publish
	if err := s.draftRepo.DeleteDraft(ctx, authorID, draftID); err != nil {
		s.logger.Warn("failed to delete draft after publish", zap.Error(err))
		// Don't fail the operation, as the article is created
	}

	return article, nil
}

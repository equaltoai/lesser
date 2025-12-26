package cms

import (
	"context"
	stdErrors "errors"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
	apperrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"go.uber.org/zap"
)

const (
	draftStatusDraft      = "draft"
	draftStatusScheduled  = "scheduled"
	draftStatusPublishing = "publishing"
	draftStatusPublished  = "published"
	draftStatusFailed     = "failed"
)

// DraftService handles business logic for drafts
type DraftService struct {
	draftRepo      *repositories.DraftRepository
	articleService *ArticleService
	domain         string
	scheduling     bool
	logger         *zap.Logger
}

// NewDraftService creates a new DraftService
func NewDraftService(draftRepo *repositories.DraftRepository, articleService *ArticleService, domain string, schedulingEnabled bool, logger *zap.Logger) *DraftService {
	return &DraftService{
		draftRepo:      draftRepo,
		articleService: articleService,
		domain:         strings.TrimSpace(domain),
		scheduling:     schedulingEnabled,
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
	now := time.Now()
	draft.UpdatedAt = now
	draft.LastSavedAt = now
	return s.draftRepo.UpdateDraft(ctx, draft)
}

// Autosave updates the draft content without changing its primary status
func (s *DraftService) Autosave(ctx context.Context, draft *models.Draft) error {
	s.logger.Debug("autosaving draft", zap.String("id", draft.ID))
	now := time.Now()
	draft.LastSavedAt = now
	draft.UpdatedAt = now
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
	if !s.scheduling {
		return stdErrors.New("scheduled publishing is disabled")
	}

	draft, err := s.draftRepo.GetDraft(ctx, authorID, draftID)
	if err != nil {
		return err
	}

	draft.ScheduledAt = &scheduledAt
	draft.Status = draftStatusScheduled
	draft.UpdatedAt = time.Now()
	return s.draftRepo.UpdateDraft(ctx, draft)
}

// PublishDraft converts a draft into an article
func (s *DraftService) PublishDraft(ctx context.Context, authorID, draftID string) (*models.Article, error) {
	s.logger.Info("publishing draft", zap.String("draft_id", draftID))

	domain := s.domain
	if domain == "" {
		return nil, stdErrors.New("domain is required to publish drafts")
	}

	draft, err := s.draftRepo.GetDraft(ctx, authorID, draftID)
	if err != nil {
		return nil, err
	}

	if !strings.EqualFold(strings.TrimSpace(draft.ContentType), activitypub.ArticleType) {
		return nil, stdErrors.New("only article drafts can be published")
	}

	if s.isPublishedDraftCleanup(draft) {
		return s.cleanupPublishedDraft(ctx, authorID, draftID, domain, draft)
	}

	objectID, err := s.resolveArticleDraftTarget(domain, draft)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	if err := s.transitionDraftToPublishing(ctx, draft, now); err != nil {
		return nil, err
	}

	if draft.ObjectID != nil && strings.TrimSpace(*draft.ObjectID) != "" {
		return s.publishDraftUpdateExistingArticle(ctx, authorID, draftID, domain, objectID, draft, now)
	}

	return s.publishDraftCreateNewArticle(ctx, authorID, draftID, domain, objectID, draft, now)
}

func (s *DraftService) isPublishedDraftCleanup(draft *models.Draft) bool {
	if draft == nil {
		return false
	}
	if !strings.EqualFold(strings.TrimSpace(draft.Status), draftStatusPublished) {
		return false
	}
	return draft.ObjectID != nil && strings.TrimSpace(*draft.ObjectID) != ""
}

func (s *DraftService) resolveArticleDraftTarget(domain string, draft *models.Draft) (string, error) {
	objectID := ""
	if draft.ObjectID != nil {
		objectID = strings.TrimSpace(*draft.ObjectID)
	}

	slug := common.Slugify(draft.Slug)
	if slug == "" {
		slug = common.Slugify(draft.Title)
	}

	if objectID == "" {
		if slug == "" {
			return "", stdErrors.New("draft slug or title is required to publish")
		}
		objectID = common.GenerateObjectID(domain, "articles", slug)
	} else if slug != "" {
		expected := common.GenerateObjectID(domain, "articles", slug)
		if !strings.EqualFold(strings.TrimSpace(expected), objectID) {
			return "", stdErrors.New("draft slug does not match objectId")
		}
	}

	if !strings.HasPrefix(objectID, "https://"+domain+"/articles/") {
		return "", stdErrors.New("draft objectId must be a local article id")
	}

	return objectID, nil
}

func (s *DraftService) cleanupPublishedDraft(ctx context.Context, authorID, draftID, domain string, draft *models.Draft) (*models.Article, error) {
	objectID := strings.TrimSpace(*draft.ObjectID)
	article, err := s.articleService.GetArticle(ctx, objectID)
	if err != nil {
		return nil, err
	}

	if strings.TrimSpace(article.AttributedTo) != common.GenerateActorID(domain, authorID) {
		return nil, stdErrors.New("draft does not have permission to access this article")
	}

	if err := s.draftRepo.DeleteDraft(ctx, authorID, draftID); err != nil {
		s.logger.Warn("failed to delete published draft during cleanup", zap.Error(err))
	}

	return article, nil
}

func (s *DraftService) transitionDraftToPublishing(ctx context.Context, draft *models.Draft, now time.Time) error {
	draft.Status = draftStatusPublishing
	draft.ScheduledAt = nil
	draft.UpdatedAt = now
	return s.draftRepo.UpdateDraft(ctx, draft)
}

func (s *DraftService) publishDraftUpdateExistingArticle(ctx context.Context, authorID, draftID, domain, objectID string, draft *models.Draft, now time.Time) (*models.Article, error) {
	article, err := s.articleService.GetArticle(ctx, objectID)
	if err != nil {
		s.markDraftFailed(ctx, draft, draftID, err)
		return nil, err
	}

	if strings.TrimSpace(article.AttributedTo) != common.GenerateActorID(domain, authorID) {
		err := stdErrors.New("draft does not have permission to update this article")
		s.markDraftFailed(ctx, draft, draftID, err)
		return nil, err
	}

	if title := strings.TrimSpace(draft.Title); title != "" {
		article.Name = title
	}
	article.Content = draft.Content
	if format := strings.TrimSpace(draft.ContentFormat); format != "" {
		article.ContentFormat = format
	}
	article.UpdatedAt = now
	article.Updated = now

	if err := s.articleService.UpdateArticle(ctx, article); err != nil {
		s.markDraftFailed(ctx, draft, draftID, err)
		return nil, err
	}

	s.deleteDraftAfterPublish(ctx, draft, authorID, draftID, objectID)
	return article, nil
}

func (s *DraftService) publishDraftCreateNewArticle(ctx context.Context, authorID, draftID, domain, objectID string, draft *models.Draft, now time.Time) (*models.Article, error) {
	article := &models.Article{
		Object: models.Object{
			ID:           objectID,
			Type:         activitypub.ArticleType,
			Name:         draft.Title,
			Content:      draft.Content,
			AttributedTo: common.GenerateActorID(domain, authorID),
			Published:    now,
			Updated:      now,
			CreatedAt:    now,
		},
		ContentFormat: draft.ContentFormat,
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	if err := s.articleService.CreateArticle(ctx, article); err != nil {
		if apperrors.HasCode(err, apperrors.CodeAlreadyExists) {
			existing, getErr := s.articleService.GetArticle(ctx, objectID)
			if getErr != nil {
				s.markDraftFailed(ctx, draft, draftID, err)
				return nil, err
			}
			if strings.TrimSpace(existing.AttributedTo) != common.GenerateActorID(domain, authorID) {
				s.markDraftFailed(ctx, draft, draftID, err)
				return nil, err
			}

			s.deleteDraftAfterPublish(ctx, draft, authorID, draftID, objectID)
			return existing, nil
		}

		s.markDraftFailed(ctx, draft, draftID, err)
		return nil, err
	}

	s.deleteDraftAfterPublish(ctx, draft, authorID, draftID, objectID)
	return article, nil
}

func (s *DraftService) deleteDraftAfterPublish(ctx context.Context, draft *models.Draft, authorID, draftID, objectID string) {
	if err := s.draftRepo.DeleteDraft(ctx, authorID, draftID); err != nil {
		s.logger.Warn("failed to delete draft after publish", zap.Error(err))
		draft.Status = draftStatusPublished
		draft.ObjectID = &objectID
		draft.UpdatedAt = time.Now()
		_ = s.draftRepo.UpdateDraft(ctx, draft)
	}
}

func (s *DraftService) markDraftFailed(ctx context.Context, draft *models.Draft, draftID string, err error) {
	s.logger.Warn("draft publish failed", zap.String("draft_id", draftID), zap.Error(err))
	draft.Status = draftStatusFailed
	draft.UpdatedAt = time.Now()
	_ = s.draftRepo.UpdateDraft(ctx, draft)
}

// CancelScheduledDraft cancels a scheduled draft publish.
func (s *DraftService) CancelScheduledDraft(ctx context.Context, authorID, draftID string) error {
	draft, err := s.draftRepo.GetDraft(ctx, authorID, draftID)
	if err != nil {
		return err
	}

	draft.ScheduledAt = nil
	draft.Status = draftStatusDraft
	draft.UpdatedAt = time.Now()

	return s.draftRepo.UpdateDraft(ctx, draft)
}

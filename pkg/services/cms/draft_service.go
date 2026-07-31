package cms

import (
	"context"
	stdErrors "errors"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/cmsrender"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"go.uber.org/zap"
)

const (
	draftStatusDraft      = "draft"
	draftStatusScheduled  = "scheduled"
	draftStatusPublishing = "publishing"
	draftStatusPublished  = "published"
	draftStatusFailed     = "failed"
)

type draftRepository interface {
	CreateDraft(ctx context.Context, draft *models.Draft) error
	UpdateDraft(ctx context.Context, authorID string, draft *models.Draft) error
	GetDraft(ctx context.Context, authorID, draftID string) (*models.Draft, error)
	DeleteDraft(ctx context.Context, authorID, draftID string) error
}

type articleDraftPublisher interface {
	GetArticle(ctx context.Context, articleID string) (*models.Article, error)
	GetArticleBySlug(ctx context.Context, slug string) (*models.Article, error)
	CreateArticle(ctx context.Context, article *models.Article) error
	UpdateArticle(ctx context.Context, article *models.Article) error
}

// DraftService handles business logic for drafts
type DraftService struct {
	draftRepo         draftRepository
	articleService    articleDraftPublisher
	domain            string
	scheduling        bool
	logger            *zap.Logger
	principalUsername func(context.Context) (string, error)
}

// NewDraftService creates a new DraftService
func NewDraftService(draftRepo draftRepository, articleService *ArticleService, domain string, schedulingEnabled bool, logger *zap.Logger) *DraftService {
	return &DraftService{
		draftRepo:      draftRepo,
		articleService: articleService,
		domain:         strings.TrimSpace(domain),
		scheduling:     schedulingEnabled,
		logger:         logger,
	}
}

// SetPrincipalUsernameProvider supplies the instance's designated operator account.
// Generated drafts fail closed when this provider is absent or cannot identify one.
func (s *DraftService) SetPrincipalUsernameProvider(provider func(context.Context) (string, error)) {
	s.principalUsername = provider
}

// CreateDraft creates a new draft
func (s *DraftService) CreateDraft(ctx context.Context, draft *models.Draft) error {
	if err := validateArticleDraftRenderable(draft); err != nil {
		logCMSDraftRenderFailure(s.logger, "create_draft", draft, err)
		return err
	}
	cmsNormalizeDraftAttribution(draft)

	s.logger.Info("creating draft", zap.String("title", draft.Title))

	if draft.CreatedAt.IsZero() {
		draft.CreatedAt = time.Now()
	}
	draft.UpdatedAt = time.Now()
	draft.LastSavedAt = time.Now()

	return s.draftRepo.CreateDraft(ctx, draft)
}

func validateDraftWriteAuthor(authorID string, draft *models.Draft) error {
	authorID = strings.TrimSpace(authorID)
	if authorID == "" {
		return stdErrors.New("authorID is required")
	}
	if draft == nil {
		return stdErrors.New("draft is required")
	}
	draftAuthor := strings.TrimSpace(draft.AuthorID)
	if draftAuthor == "" {
		return stdErrors.New("draft author is required")
	}
	if draftAuthor != authorID {
		return stdErrors.New("draft does not belong to author")
	}
	draft.AuthorID = draftAuthor
	return nil
}

func validateArticleDraftRenderable(draft *models.Draft) error {
	if draft == nil {
		return stdErrors.New("draft is required")
	}
	if !strings.EqualFold(strings.TrimSpace(draft.ContentType), activitypub.ArticleType) {
		return nil
	}
	rendered, err := cmsrender.RenderArticleContent(draft.Content, draft.ContentFormat)
	if err != nil {
		return err
	}
	draft.ContentFormat = rendered.SourceFormat
	return nil
}

// UpdateDraft updates an existing draft
func (s *DraftService) UpdateDraft(ctx context.Context, authorID string, draft *models.Draft) error {
	if err := validateDraftWriteAuthor(authorID, draft); err != nil {
		return err
	}
	if err := validateArticleDraftRenderable(draft); err != nil {
		logCMSDraftRenderFailure(s.logger, "update_draft", draft, err)
		return err
	}
	cmsNormalizeDraftAttribution(draft)
	now := time.Now()
	draft.UpdatedAt = now
	draft.LastSavedAt = now
	return s.draftRepo.UpdateDraft(ctx, authorID, draft)
}

// Autosave updates the draft content without changing its primary status
func (s *DraftService) Autosave(ctx context.Context, authorID string, draft *models.Draft) error {
	if err := validateDraftWriteAuthor(authorID, draft); err != nil {
		return err
	}
	if err := validateArticleDraftRenderable(draft); err != nil {
		logCMSDraftRenderFailure(s.logger, "autosave_draft", draft, err)
		return err
	}
	cmsNormalizeDraftAttribution(draft)
	s.logger.Debug("autosaving draft", zap.String("id", draft.ID))
	now := time.Now()
	draft.LastSavedAt = now
	draft.UpdatedAt = now
	draft.AutosaveVersion++
	return s.draftRepo.UpdateDraft(ctx, authorID, draft)
}

// GetDraft retrieves a draft
func (s *DraftService) GetDraft(ctx context.Context, authorID, draftID string) (*models.Draft, error) {
	return s.draftRepo.GetDraft(ctx, authorID, draftID)
}

// PreviewDraft renders a draft through the same Article publication renderer used for ActivityPub and public HTML.
func (s *DraftService) PreviewDraft(ctx context.Context, authorID, draftID string) (cmsrender.RenderedArticleContent, error) {
	draft, err := s.draftRepo.GetDraft(ctx, authorID, draftID)
	if err != nil {
		return cmsrender.RenderedArticleContent{}, err
	}
	rendered, err := RenderDraftPreview(draft)
	if err != nil {
		logCMSDraftRenderFailure(s.logger, "preview_draft", draft, err)
		return cmsrender.RenderedArticleContent{}, err
	}
	return rendered, nil
}

// RenderDraftPreview renders draft source through the canonical Article renderer.
func RenderDraftPreview(draft *models.Draft) (cmsrender.RenderedArticleContent, error) {
	if draft == nil {
		return cmsrender.RenderedArticleContent{}, stdErrors.New("draft is required")
	}
	if !strings.EqualFold(strings.TrimSpace(draft.ContentType), activitypub.ArticleType) {
		return cmsrender.RenderedArticleContent{}, stdErrors.New("only article drafts can be previewed")
	}
	rendered, err := cmsrender.RenderArticleContent(draft.Content, draft.ContentFormat)
	if err != nil {
		return cmsrender.RenderedArticleContent{}, err
	}
	draft.ContentFormat = rendered.SourceFormat
	return rendered, nil
}

// DeleteDraft deletes a draft
func (s *DraftService) DeleteDraft(ctx context.Context, authorID, draftID string) error {
	authorID = strings.TrimSpace(authorID)
	draftID = strings.TrimSpace(draftID)
	if authorID == "" {
		return stdErrors.New("authorID is required")
	}
	if draftID == "" {
		return stdErrors.New("draftID is required")
	}
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

	approved, approvalErr := s.HasUnanimousActiveApproval(ctx, authorID, draftID)
	if approvalErr != nil {
		return approvalErr
	}
	if !approved {
		return ErrDraftReviewApprovalRequired
	}
	if strings.TrimSpace(draft.GeneratedBy) != "" {
		principalApproved, principalErr := s.HasPrincipalApproval(ctx, authorID, draftID)
		if principalErr != nil {
			return principalErr
		}
		if !principalApproved {
			return ErrDraftReviewPrincipalApprovalRequired
		}
	}
	draft.ScheduledAt = &scheduledAt
	draft.Status = draftStatusScheduled
	draft.PublishFailureReason = ""
	draft.UpdatedAt = time.Now()
	return s.draftRepo.UpdateDraft(ctx, authorID, draft)
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
	approved, approvalErr := s.HasUnanimousActiveApproval(ctx, authorID, draftID)
	if approvalErr != nil {
		return nil, approvalErr
	}
	if !approved {
		return nil, ErrDraftReviewApprovalRequired
	}
	if strings.TrimSpace(draft.GeneratedBy) != "" {
		principalApproved, principalErr := s.HasPrincipalApproval(ctx, authorID, draftID)
		if principalErr != nil {
			return nil, principalErr
		}
		if !principalApproved {
			return nil, ErrDraftReviewPrincipalApprovalRequired
		}
	}

	if s.isPublishedDraftCleanup(draft) {
		return s.cleanupPublishedDraft(ctx, authorID, draftID, domain, draft)
	}

	objectID, slug, err := s.resolveArticleDraftTarget(domain, draft)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	if err := s.transitionDraftToPublishing(ctx, authorID, draft, now); err != nil {
		return nil, err
	}

	if draft.ObjectID != nil && strings.TrimSpace(*draft.ObjectID) != "" {
		return s.publishDraftUpdateExistingArticle(ctx, authorID, draftID, domain, objectID, slug, draft, now)
	}

	return s.publishDraftCreateNewArticle(ctx, authorID, draftID, domain, objectID, slug, draft, now)
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

func (s *DraftService) resolveArticleDraftTarget(domain string, draft *models.Draft) (objectID string, slug string, err error) {
	objectID = ""
	if draft.ObjectID != nil {
		objectID = strings.TrimSpace(*draft.ObjectID)
	}

	slug = common.Slugify(draft.Slug)
	if slug == "" {
		slug = common.Slugify(draft.Title)
	}

	if slug == "" {
		return "", "", stdErrors.New("draft slug or title is required to publish")
	}

	if objectID == "" {
		objectID = common.GenerateObjectID(domain, "articles", slug)
	}

	if !strings.HasPrefix(objectID, "https://"+domain+"/") && !strings.HasPrefix(objectID, "http://"+domain+"/") {
		return "", "", stdErrors.New("draft objectId must be a local id")
	}

	if !strings.HasPrefix(objectID, "https://"+domain+"/articles/") &&
		!strings.HasPrefix(objectID, "http://"+domain+"/articles/") &&
		!strings.HasPrefix(objectID, "https://"+domain+"/objects/") &&
		!strings.HasPrefix(objectID, "http://"+domain+"/objects/") {
		return "", "", stdErrors.New("draft objectId must be a local article id")
	}

	return objectID, slug, nil
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

func (s *DraftService) transitionDraftToPublishing(ctx context.Context, authorID string, draft *models.Draft, now time.Time) error {
	if err := validateDraftWriteAuthor(authorID, draft); err != nil {
		return err
	}
	draft.Status = draftStatusPublishing
	draft.ScheduledAt = nil
	draft.UpdatedAt = now
	return s.draftRepo.UpdateDraft(ctx, authorID, draft)
}

func (s *DraftService) publishDraftUpdateExistingArticle(ctx context.Context, authorID, draftID, domain, objectID, slug string, draft *models.Draft, now time.Time) (*models.Article, error) {
	article, err := s.articleService.GetArticle(ctx, objectID)
	if err != nil {
		s.markDraftFailed(ctx, authorID, draft, draftID, err)
		return nil, err
	}

	if strings.TrimSpace(article.AttributedTo) != common.GenerateActorID(domain, authorID) {
		err := stdErrors.New("draft does not have permission to update this article")
		s.markDraftFailed(ctx, authorID, draft, draftID, err)
		return nil, err
	}

	article = cmsCloneArticleForDraftMutation(article)
	if title := strings.TrimSpace(draft.Title); title != "" {
		article.Name = title
	}
	if slug != "" {
		article.Slug = slug
	}
	article.Content = draft.Content
	if format := strings.TrimSpace(draft.ContentFormat); format != "" {
		article.ContentFormat = format
	}
	cmsApplyDraftAttributionToArticle(article, draft, domain, authorID, true)
	article.UpdatedAt = now
	article.Updated = now

	if err := s.articleService.UpdateArticle(ctx, article); err != nil {
		s.markDraftFailed(ctx, authorID, draft, draftID, err)
		return nil, err
	}

	s.deleteDraftAfterPublish(ctx, draft, authorID, draftID, objectID)
	return article, nil
}

func cmsCloneArticleForDraftMutation(article *models.Article) *models.Article {
	if article == nil {
		return nil
	}
	cp := *article
	cp.To = append([]string{}, article.To...)
	cp.CC = append([]string{}, article.CC...)
	cp.BTo = append([]string{}, article.BTo...)
	cp.BCC = append([]string{}, article.BCC...)
	cp.CategoryIDs = append([]string{}, article.CategoryIDs...)
	cp.TableOfContents = append([]models.TOCEntry{}, article.TableOfContents...)
	if article.InReplyTo != nil {
		v := *article.InReplyTo
		cp.InReplyTo = &v
	}
	if article.SeriesID != nil {
		v := *article.SeriesID
		cp.SeriesID = &v
	}
	if article.SeriesOrder != nil {
		v := *article.SeriesOrder
		cp.SeriesOrder = &v
	}
	if article.FeaturedImage != nil {
		v := *article.FeaturedImage
		cp.FeaturedImage = &v
	}
	return &cp
}

func (s *DraftService) publishDraftCreateNewArticle(ctx context.Context, authorID, draftID, domain, objectID, slug string, draft *models.Draft, now time.Time) (*models.Article, error) {
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
		Slug:          slug,
		ContentFormat: draft.ContentFormat,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	cmsApplyDraftAttributionToArticle(article, draft, domain, authorID, false)

	if err := s.articleService.CreateArticle(ctx, article); err != nil {
		s.markDraftFailed(ctx, authorID, draft, draftID, err)
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
		_ = s.draftRepo.UpdateDraft(ctx, authorID, draft)
	}
}

func (s *DraftService) markDraftFailed(ctx context.Context, authorID string, draft *models.Draft, draftID string, err error) {
	s.logger.Warn("draft publish failed", zap.String("draft_id", draftID), zap.Error(err))
	draft.Status = draftStatusFailed
	draft.UpdatedAt = time.Now()
	_ = s.draftRepo.UpdateDraft(ctx, authorID, draft)
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

	return s.draftRepo.UpdateDraft(ctx, authorID, draft)
}

// Package cms provides services for Content Management System functionality
package cms

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
	apperrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/equaltoai/lesser/pkg/transformations"
	"github.com/google/uuid"
	dynamormerrors "github.com/pay-theory/dynamorm/pkg/errors"
	"go.uber.org/zap"
)

// FederationService interface to avoid circular imports with pkg/services
type FederationService interface {
	DeliverToFollowers(ctx context.Context, activity *activitypub.Activity, actor *activitypub.Actor) error
}

// ArticleService handles business logic for articles
type ArticleService struct {
	articleRepo     *repositories.ArticleRepository
	actorRepo       *repositories.ActorRepository
	seriesRepo      *repositories.SeriesRepository
	categoryRepo    *repositories.CategoryRepository
	revisionService *RevisionService
	federation      FederationService
	logger          *zap.Logger
}

// NewArticleService creates a new ArticleService
func NewArticleService(
	articleRepo *repositories.ArticleRepository,
	actorRepo *repositories.ActorRepository,
	seriesRepo *repositories.SeriesRepository,
	categoryRepo *repositories.CategoryRepository,
	revisionService *RevisionService,
	federation FederationService,
	logger *zap.Logger,
) *ArticleService {
	return &ArticleService{
		articleRepo:     articleRepo,
		actorRepo:       actorRepo,
		seriesRepo:      seriesRepo,
		categoryRepo:    categoryRepo,
		revisionService: revisionService,
		federation:      federation,
		logger:          logger,
	}
}

// CreateArticle creates a new article
func (s *ArticleService) CreateArticle(ctx context.Context, article *models.Article) error {
	if article == nil {
		return errors.New("article is required")
	}

	slug := strings.TrimSpace(article.Slug)
	if slug == "" {
		return apperrors.ValidationFailedWithField("slug")
	}
	article.Slug = slug

	if strings.TrimSpace(article.ID) == "" {
		return apperrors.ValidationFailedWithField("id")
	}

	s.logger.Info("creating article", zap.String("title", article.Name))

	if article.CreatedAt.IsZero() {
		article.CreatedAt = time.Now()
	}
	article.UpdatedAt = time.Now()

	enrichArticleContent(article)

	if err := s.ensureLegacyArticleSlugAvailable(ctx, slug, article.ID); err != nil {
		return err
	}

	slugCreated, err := cmsEnsureArticleSlugIndex(ctx, s.articleRepo.GetDB(), slug, article.ID)
	if err != nil {
		return err
	}

	if err := s.articleRepo.CreateArticle(ctx, article); err != nil {
		if slugCreated {
			cmsDeleteArticleSlugIndex(ctx, s.articleRepo.GetDB(), slug)
		}
		return err
	}

	if err := s.upsertCMSArticleIndexes(ctx, article); err != nil {
		s.logger.Error("failed to upsert CMS article indexes on create", zap.Error(err), zap.String("article_id", article.ID))
		// Best-effort rollback to avoid creating unreachable content.
		s.deleteCMSArticleIndexes(ctx, article)
		if slugCreated {
			cmsDeleteArticleSlugIndex(ctx, s.articleRepo.GetDB(), slug)
		}
		_ = s.articleRepo.DeleteArticle(ctx, article.ID)
		return err
	}

	s.updateCMSArticleCountsBestEffort(ctx, nil, article)

	// Federate the article creation asynchronously
	go s.federateArticleCreation(context.Background(), article)

	return nil
}

// GetArticleBySlug retrieves an article by its slug index (no legacy fallback).
func (s *ArticleService) GetArticleBySlug(ctx context.Context, slug string) (*models.Article, error) {
	slug = strings.TrimSpace(slug)
	if slug == "" {
		return nil, apperrors.ValidationFailedWithField("slug")
	}

	var idx models.CMSSlugIndex
	err := s.articleRepo.GetDB().WithContext(ctx).Model(&models.CMSSlugIndex{}).
		Where("PK", "=", models.CMSArticleSlugIndexPK(slug)).
		Where("SK", "=", models.CMSSlugIndexSK()).
		First(&idx)
	if err != nil {
		if dynamormerrors.IsNotFound(err) {
			return nil, apperrors.ItemNotFoundWithID("article slug", slug)
		}
		return nil, err
	}

	articleID := strings.TrimSpace(idx.TargetID)
	if articleID == "" {
		return nil, apperrors.ItemNotFoundWithID("article slug", slug)
	}

	return s.articleRepo.GetArticle(ctx, articleID)
}

// GetArticle retrieves an article by ID.
func (s *ArticleService) GetArticle(ctx context.Context, articleID string) (*models.Article, error) {
	articleID = strings.TrimSpace(articleID)
	if articleID == "" {
		return nil, errors.New("article id is required")
	}
	return s.articleRepo.GetArticle(ctx, articleID)
}

// UpdateArticle updates an existing article and records a revision if configured.
func (s *ArticleService) UpdateArticle(ctx context.Context, article *models.Article) error {
	if article == nil {
		return errors.New("article is required")
	}
	if strings.TrimSpace(article.ID) == "" {
		return errors.New("article id is required")
	}

	existing, _ := s.articleRepo.GetArticle(ctx, strings.TrimSpace(article.ID))

	slug := strings.TrimSpace(article.Slug)
	article.Slug = slug

	slugCreated := false
	if slug != "" {
		if err := s.ensureLegacyArticleSlugAvailable(ctx, slug, article.ID); err != nil {
			return err
		}

		created, err := cmsEnsureArticleSlugIndex(ctx, s.articleRepo.GetDB(), slug, article.ID)
		if err != nil {
			return err
		}
		slugCreated = created
	}

	// Snapshot existing state before applying the update.
	if s.revisionService != nil && existing != nil {
		_, _ = s.revisionService.CreateRevision(ctx, existing)
	}

	if article.UpdatedAt.IsZero() {
		article.UpdatedAt = time.Now()
	}
	if article.Updated.IsZero() {
		article.Updated = article.UpdatedAt
	}

	enrichArticleContent(article)

	if err := s.articleRepo.UpdateArticle(ctx, article); err != nil {
		if slugCreated {
			cmsDeleteArticleSlugIndex(ctx, s.articleRepo.GetDB(), slug)
		}
		return err
	}

	if err := s.upsertCMSArticleIndexes(ctx, article); err != nil {
		s.logger.Error("failed to upsert CMS article indexes on update", zap.Error(err), zap.String("article_id", article.ID))
		return err
	}
	if existing != nil {
		s.deleteCMSArticleIndexesForRemovedGroups(ctx, existing, article)
	}
	s.updateCMSArticleCountsBestEffort(ctx, existing, article)

	go s.federateArticleUpdate(context.Background(), article)

	return nil
}

// DeleteArticle deletes an article and federates a Delete activity best-effort.
func (s *ArticleService) DeleteArticle(ctx context.Context, article *models.Article) error {
	if article == nil {
		return errors.New("article is required")
	}
	if strings.TrimSpace(article.ID) == "" {
		return errors.New("article id is required")
	}

	if err := s.articleRepo.DeleteArticle(ctx, strings.TrimSpace(article.ID)); err != nil {
		return err
	}

	s.deleteCMSArticleIndexes(ctx, article)
	s.updateCMSArticleCountsBestEffort(ctx, article, nil)

	go s.federateArticleDeletion(context.Background(), article)

	return nil
}

func (s *ArticleService) ensureLegacyArticleSlugAvailable(ctx context.Context, slug string, articleID string) error {
	host := cmsHostFromURL(articleID)
	if host == "" {
		return nil
	}

	legacyID := common.GenerateObjectID(host, "articles", slug)
	if legacyID == "" || strings.EqualFold(legacyID, articleID) {
		return nil
	}

	_, err := s.articleRepo.GetArticle(ctx, legacyID)
	if err == nil {
		return apperrors.ItemAlreadyExistsWithID("article slug", slug)
	}
	if apperrors.HasCode(err, apperrors.CodeNotFound) {
		return nil
	}
	return err
}

func (s *ArticleService) upsertCMSArticleIndexes(ctx context.Context, article *models.Article) error {
	if article == nil {
		return nil
	}
	db := s.articleRepo.GetDB()

	entries := cmsArticleIndexEntries(article)
	for _, entry := range entries {
		if entry == nil {
			continue
		}
		if err := db.WithContext(ctx).Model(entry).Create(); err != nil {
			return err
		}
	}
	return nil
}

func (s *ArticleService) deleteCMSArticleIndexes(ctx context.Context, article *models.Article) {
	if article == nil {
		return
	}
	db := s.articleRepo.GetDB()

	entries := cmsArticleIndexEntries(article)
	for _, entry := range entries {
		if entry == nil {
			continue
		}
		if err := db.WithContext(ctx).Model(entry).Delete(); err != nil && !dynamormerrors.IsNotFound(err) {
			s.logger.Warn("failed to delete CMS article index entry", zap.Error(err), zap.String("pk", entry.PK), zap.String("sk", entry.SK))
		}
	}
}

func (s *ArticleService) deleteCMSArticleIndexesForRemovedGroups(ctx context.Context, before *models.Article, after *models.Article) {
	if before == nil {
		return
	}
	db := s.articleRepo.GetDB()

	removed := cmsArticleIndexEntriesForRemovedGroups(before, after)
	for _, entry := range removed {
		if entry == nil {
			continue
		}
		if err := db.WithContext(ctx).Model(entry).Delete(); err != nil && !dynamormerrors.IsNotFound(err) {
			s.logger.Warn("failed to delete removed CMS article index entry", zap.Error(err), zap.String("pk", entry.PK), zap.String("sk", entry.SK))
		}
	}
}

func (s *ArticleService) updateCMSArticleCountsBestEffort(ctx context.Context, before *models.Article, after *models.Article) {
	cmsUpdateArticleCountsBestEffort(ctx, s.seriesRepo, s.categoryRepo, before, after, s.logger)
}

func (s *ArticleService) federateArticleCreation(ctx context.Context, article *models.Article) {
	s.federateArticleWriteActivity(ctx, article, activitypub.CreateType, "create")
}

func (s *ArticleService) federateArticleUpdate(ctx context.Context, article *models.Article) {
	s.federateArticleWriteActivity(ctx, article, activitypub.UpdateType, "update")
}

func (s *ArticleService) federateArticleWriteActivity(ctx context.Context, article *models.Article, activityType string, label string) {
	apArticle, err := transformations.StorageArticleToActivityPub(article)
	if err != nil {
		s.logger.Error("failed to convert article to AP for federation",
			zap.String("label", label),
			zap.String("article_id", article.ID),
			zap.Error(err))
		return
	}

	username := extractUsernameFromActorID(article.AttributedTo)
	apActor, err := s.actorRepo.GetActor(ctx, username)
	if err != nil {
		s.logger.Error("failed to get actor for article federation",
			zap.String("label", label),
			zap.String("actor_id", article.AttributedTo),
			zap.Error(err))
		return
	}

	now := time.Now()
	activityID := fmt.Sprintf("%s/activities/%s-%d-%s", apActor.ID, label, now.Unix(), uuid.NewString())
	activity := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			Context:   activitypub.Context,
			Type:      activityType,
			ID:        activityID,
			To:        apArticle.To,
			CC:        apArticle.CC,
			Published: &now,
		},
		Actor:  apActor.ID,
		Object: apArticle,
	}

	if err := s.federation.DeliverToFollowers(ctx, activity, apActor); err != nil {
		s.logger.Error("failed to deliver article activity",
			zap.String("label", label),
			zap.String("activity_id", activityID),
			zap.Error(err))
	} else {
		s.logger.Info("successfully federated article activity",
			zap.String("label", label),
			zap.String("article_id", article.ID),
			zap.String("activity_id", activityID))
	}
}
func (s *ArticleService) federateArticleDeletion(ctx context.Context, article *models.Article) {
	username := extractUsernameFromActorID(article.AttributedTo)
	apActor, err := s.actorRepo.GetActor(ctx, username)
	if err != nil {
		s.logger.Error("failed to get actor for article delete federation",
			zap.String("actor_id", article.AttributedTo),
			zap.Error(err))
		return
	}

	to := []string{activitypub.PublicAddress}
	cc := []string{}
	if apArticle, err := transformations.StorageArticleToActivityPub(article); err == nil {
		if len(apArticle.To) > 0 {
			to = apArticle.To
		}
		cc = apArticle.CC
	}

	now := time.Now()
	activityID := fmt.Sprintf("%s/activities/delete-%d-%s", apActor.ID, now.Unix(), uuid.NewString())
	activity := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			Context:   activitypub.Context,
			Type:      activitypub.DeleteType,
			ID:        activityID,
			To:        to,
			CC:        cc,
			Published: &now,
		},
		Actor:  apActor.ID,
		Object: article.ID,
	}

	if err := s.federation.DeliverToFollowers(ctx, activity, apActor); err != nil {
		s.logger.Error("failed to deliver article delete activity",
			zap.String("activity_id", activityID),
			zap.Error(err))
	} else {
		s.logger.Info("successfully federated article delete",
			zap.String("article_id", article.ID),
			zap.String("activity_id", activityID))
	}
}

// extractUsernameFromActorID extracts username from an actor ID URL
// e.g., https://example.com/users/alice -> alice
func extractUsernameFromActorID(actorID string) string {
	// Simple extraction - take the last path segment
	parts := strings.Split(actorID, "/")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return actorID
}

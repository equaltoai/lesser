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
	"github.com/equaltoai/lesser/pkg/transformations"
	"github.com/google/uuid"
	dynamormcore "github.com/theory-cloud/tabletheory/pkg/core"
	dynamormerrors "github.com/theory-cloud/tabletheory/pkg/errors"
	"go.uber.org/zap"
)

// FederationService interface to avoid circular imports with pkg/services
type FederationService interface {
	DeliverToFollowers(ctx context.Context, activity *activitypub.Activity, actor *activitypub.Actor) error
	DeliverToRecipients(ctx context.Context, activity *activitypub.Activity, actor *activitypub.Actor) error
}

type articleServiceRepository interface {
	GetDB() dynamormcore.DB
	CreateArticle(ctx context.Context, article *models.Article) error
	GetArticle(ctx context.Context, articleID string) (*models.Article, error)
	UpdateArticle(ctx context.Context, article *models.Article) error
	DeleteArticle(ctx context.Context, articleID string) error
}

type actorRepository interface {
	GetActor(ctx context.Context, username string) (*activitypub.Actor, error)
}

type articleRevisionCreator interface {
	CreateRevision(ctx context.Context, article *models.Article) (*models.Revision, error)
}

// ArticleService handles business logic for articles
type ArticleService struct {
	articleRepo     articleServiceRepository
	actorRepo       actorRepository
	seriesRepo      cmsSeriesArticleCountUpdater
	categoryRepo    cmsCategoryArticleCountUpdater
	revisionService articleRevisionCreator
	federation      FederationService
	logger          *zap.Logger
}

// NewArticleService creates a new ArticleService
func NewArticleService(
	articleRepo articleServiceRepository,
	actorRepo actorRepository,
	seriesRepo cmsSeriesArticleCountUpdater,
	categoryRepo cmsCategoryArticleCountUpdater,
	revisionService articleRevisionCreator,
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

	tenant := cmsTenantFromID(article.ID)
	slugCreated, err := cmsEnsureArticleSlugIndexForTenant(ctx, s.articleRepo.GetDB(), tenant, slug, article.ID)
	if err != nil {
		return err
	}

	if err := s.articleRepo.CreateArticle(ctx, article); err != nil {
		if slugCreated {
			cmsDeleteArticleSlugIndexForTenant(ctx, s.articleRepo.GetDB(), tenant, slug)
		}
		return err
	}

	if err := s.upsertCMSArticleIndexes(ctx, article); err != nil {
		s.logger.Error("failed to upsert CMS article indexes on create", zap.Error(err), zap.String("article_id", article.ID))
		// Best-effort rollback to avoid creating unreachable content.
		s.deleteCMSArticleIndexes(ctx, article)
		if slugCreated {
			cmsDeleteArticleSlugIndexForTenant(ctx, s.articleRepo.GetDB(), tenant, slug)
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
	return s.getArticleBySlugIndex(ctx, slug, models.CMSArticleSlugIndexPK(slug))
}

// GetArticleByTenantSlug retrieves an article by a tenant-scoped slug index with
// legacy global-index compatibility. Legacy matches are only returned after the
// resolved article ID is verified to belong to the requested tenant.
func (s *ArticleService) GetArticleByTenantSlug(ctx context.Context, tenant string, slug string) (*models.Article, error) {
	slug = strings.TrimSpace(slug)
	if slug == "" {
		return nil, apperrors.ValidationFailedWithField("slug")
	}
	tenant = cmsNormalizeTenant(tenant)
	if tenant == "" {
		return s.GetArticleBySlug(ctx, slug)
	}

	article, err := s.getArticleBySlugIndex(ctx, slug, models.CMSTenantArticleSlugIndexPK(tenant, slug))
	if err == nil {
		if articleBelongsToTenant(article, tenant) {
			return article, nil
		}
		return nil, apperrors.ItemNotFoundWithID("article slug", slug)
	}
	if err != nil && !apperrors.HasCode(err, apperrors.CodeNotFound) {
		return nil, err
	}

	article, err = s.getArticleBySlugIndex(ctx, slug, models.CMSArticleSlugIndexPK(slug))
	if err != nil {
		return nil, err
	}
	if !articleBelongsToTenant(article, tenant) {
		return nil, apperrors.ItemNotFoundWithID("article slug", slug)
	}

	_, _ = cmsEnsureArticleSlugIndexForTenant(ctx, s.articleRepo.GetDB(), tenant, slug, article.ID)
	return article, nil
}

func (s *ArticleService) getArticleBySlugIndex(ctx context.Context, slug string, pk string) (*models.Article, error) {
	if strings.TrimSpace(pk) == "" {
		return nil, apperrors.ItemNotFoundWithID("article slug", slug)
	}

	var idx models.CMSSlugIndex
	err := s.articleRepo.GetDB().WithContext(ctx).Model(&models.CMSSlugIndex{}).
		Where("PK", "=", pk).
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

func articleBelongsToTenant(article *models.Article, tenant string) bool {
	if article == nil {
		return false
	}
	return strings.EqualFold(cmsTenantFromID(article.ID), tenant)
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

		tenant := cmsTenantFromID(article.ID)
		created, err := cmsEnsureArticleSlugIndexForTenant(ctx, s.articleRepo.GetDB(), tenant, slug, article.ID)
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
			cmsDeleteArticleSlugIndexForTenant(ctx, s.articleRepo.GetDB(), cmsTenantFromID(article.ID), slug)
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
	var seriesUpdater cmsSeriesArticleCountUpdater
	if s.seriesRepo != nil {
		seriesUpdater = s.seriesRepo
	}

	var categoryUpdater cmsCategoryArticleCountUpdater
	if s.categoryRepo != nil {
		categoryUpdater = s.categoryRepo
	}

	cmsUpdateArticleCountsBestEffort(ctx, seriesUpdater, categoryUpdater, before, after, s.logger)
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

	if err := s.deliverFederatedArticleActivity(ctx, activity, apActor); err != nil {
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

	if err := s.deliverFederatedArticleActivity(ctx, activity, apActor); err != nil {
		s.logger.Error("failed to deliver article delete activity",
			zap.String("activity_id", activityID),
			zap.Error(err))
	} else {
		s.logger.Info("successfully federated article delete",
			zap.String("article_id", article.ID),
			zap.String("activity_id", activityID))
	}
}

func (s *ArticleService) deliverFederatedArticleActivity(ctx context.Context, activity *activitypub.Activity, actor *activitypub.Actor) error {
	if articleActivityHasPublicAddressing(activity) {
		if err := s.federation.DeliverToFollowers(ctx, activity, actor); err != nil {
			s.logger.Error("failed to deliver article activity to followers",
				zap.String("activity_id", activity.ID),
				zap.Error(err))
		}
	}

	return s.federation.DeliverToRecipients(ctx, activity, actor)
}

func articleActivityHasPublicAddressing(activity *activitypub.Activity) bool {
	if activity == nil {
		return false
	}

	for _, recipient := range append(activity.To, activity.CC...) {
		if recipient == activitypub.PublicAddress {
			return true
		}
	}

	return false
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

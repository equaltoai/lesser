// Package cms provides services for Content Management System functionality
package cms

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/cmsrender"
	"github.com/equaltoai/lesser/pkg/common"
	apperrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/transformations"
	"github.com/google/uuid"
	dynamormcore "github.com/theory-cloud/tabletheory/v3/pkg/core"
	dynamormerrors "github.com/theory-cloud/tabletheory/v3/pkg/errors"
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

type transactWriteDB interface {
	TransactWrite(ctx context.Context, fn func(dynamormcore.TransactionBuilder) error) error
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
	cmsNormalizeArticleAttribution(article, nil)

	if err := validateArticleRenderable(article); err != nil {
		logCMSArticleRenderFailure(s.logger, "create_article", article, err)
		return err
	}

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

	// Federate an immutable snapshot so repository ownership of article cannot
	// race later CMS writes while this best-effort handoff is still running.
	go s.federateArticleCreation(context.Background(), cloneArticleForFederation(article))

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

func validateArticleRenderable(article *models.Article) error {
	if article == nil {
		return errors.New("article is required")
	}

	rendered, err := cmsrender.RenderArticleContent(article.Content, article.ContentFormat)
	if err != nil {
		return err
	}
	article.ContentFormat = rendered.SourceFormat
	return nil
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
	cmsNormalizeArticleAttribution(article, existing)

	slug := strings.TrimSpace(article.Slug)
	article.Slug = slug
	if existing != nil {
		if canonicalSlug, ok := cmsArticleSlugFromCanonicalID(existing.ID); ok {
			requestedSlug := slug
			if requestedSlug == "" {
				requestedSlug = strings.TrimSpace(existing.Slug)
			}
			if requestedSlug == "" {
				requestedSlug = canonicalSlug
			}
			if requestedSlug != canonicalSlug {
				return apperrors.ValidationFailed("slug", "published article slug is immutable")
			}
			article.Slug = requestedSlug
			slug = requestedSlug
		}
	}

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

	// Roll back the slug index if it was created during this call and a
	// subsequent step fails. Extracted to keep UpdateArticle under the
	// gocognit threshold (CSR-029).
	cleanupSlugIfCreated := func() {
		if slugCreated {
			cmsDeleteArticleSlugIndexForTenant(ctx, s.articleRepo.GetDB(), cmsTenantFromID(article.ID), slug)
		}
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

	if err := validateArticleRenderable(article); err != nil {
		logCMSArticleRenderFailure(s.logger, "update_article", article, err)
		cleanupSlugIfCreated()
		return err
	}
	enrichArticleContent(article)

	if err := s.articleRepo.UpdateArticle(ctx, article); err != nil {
		cleanupSlugIfCreated()
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

	go s.federateArticleUpdate(context.Background(), cloneArticleForFederation(article))

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

	if err := s.deleteArticleAndCreateTombstone(ctx, article); err != nil {
		return err
	}

	s.deleteCMSArticleIndexes(ctx, article)
	s.updateCMSArticleCountsBestEffort(ctx, article, nil)

	go s.federateArticleDeletion(context.Background(), cloneArticleForFederation(article))

	return nil
}

func (s *ArticleService) deleteArticleAndCreateTombstone(ctx context.Context, article *models.Article) error {
	tombstone, err := s.buildArticleTombstone(article)
	if err != nil {
		return err
	}

	db := s.articleRepo.GetDB()
	if db == nil {
		return errors.New("article repository db is required for tombstone")
	}

	if txDB, ok := db.(transactWriteDB); ok {
		deleteModel := *article
		if err := deleteModel.UpdateKeys(); err != nil {
			return fmt.Errorf("prepare article delete keys: %w", err)
		}

		if err := txDB.TransactWrite(ctx, func(tx dynamormcore.TransactionBuilder) error {
			tx.Create(tombstone).Delete(&deleteModel)
			return nil
		}); err != nil {
			s.logger.Error("failed to transactionally delete article with tombstone",
				zap.String("article_id", tombstone.ID),
				zap.String("former_type", tombstone.FormerType),
				zap.Error(err))
			return err
		}
		return nil
	}

	// Production TableTheory DBs expose TransactWrite above. This fallback keeps
	// minimal core.DB test doubles functional while preserving the previous write
	// order for implementations that cannot express a multi-item transaction.
	if err := s.articleRepo.DeleteArticle(ctx, tombstone.ID); err != nil {
		return err
	}
	return s.persistArticleTombstone(ctx, tombstone)
}

func (s *ArticleService) buildArticleTombstone(article *models.Article) (*models.Tombstone, error) {
	objectID := strings.TrimSpace(article.ID)
	if objectID == "" {
		return nil, errors.New("article id is required")
	}

	formerType := strings.TrimSpace(article.Type)
	if formerType == "" {
		formerType = activitypub.ArticleType
	}

	deletedBy := strings.TrimSpace(article.AttributedTo)
	summary := "Article was deleted"
	if deletedBy != "" {
		summary = fmt.Sprintf("Object deleted by %s", deletedBy)
	}

	tombstone := &models.Tombstone{
		ID:           objectID,
		FormerType:   formerType,
		DeletedBy:    deletedBy,
		AttributedTo: deletedBy,
		// No CMS writer populates Article.To/CC today. Keep the historical public
		// default for unaddressed articles while deriving fail-closed once an
		// audience-writing surface is introduced.
		IsPublic: articleIsPubliclyAddressed(article),
		Summary:  summary,
		Deleted:  time.Now(),
	}
	if err := tombstone.BeforeCreate(); err != nil {
		return nil, fmt.Errorf("prepare article tombstone: %w", err)
	}

	return tombstone, nil
}

// articleIsPubliclyAddressed derives tombstone visibility from the stored
// ActivityPub addressing. No CMS writer populates Article.To/CC today, so
// unaddressed articles retain their historical public default. The explicit
// predicate is forward-looking defense-in-depth for a future audience field.
func articleIsPubliclyAddressed(article *models.Article) bool {
	if article == nil {
		return false
	}
	if len(article.To) == 0 && len(article.CC) == 0 {
		return true
	}
	for _, recipient := range article.To {
		if isActivityStreamsPublicAddress(recipient) {
			return true
		}
	}
	for _, recipient := range article.CC {
		if isActivityStreamsPublicAddress(recipient) {
			return true
		}
	}
	return false
}

func isActivityStreamsPublicAddress(recipient string) bool {
	switch strings.TrimSpace(recipient) {
	case activitypub.PublicAddress, "as:Public", "Public":
		return true
	default:
		return false
	}
}

func (s *ArticleService) persistArticleTombstone(ctx context.Context, tombstone *models.Tombstone) error {
	if tombstone == nil {
		return errors.New("article tombstone is required")
	}

	db := s.articleRepo.GetDB()
	if db == nil {
		return errors.New("article repository db is required for tombstone")
	}
	if err := db.WithContext(ctx).Model(tombstone).Create(); err != nil {
		s.logger.Error("failed to create article tombstone",
			zap.String("article_id", tombstone.ID),
			zap.String("former_type", tombstone.FormerType),
			zap.Error(err))
		return err
	}

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
		if err := db.WithContext(ctx).Model(entry).CreateOrUpdate(); err != nil {
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

func cloneArticleForFederation(article *models.Article) *models.Article {
	if article == nil {
		return nil
	}

	clone := *article
	clone.To = append([]string(nil), article.To...)
	clone.CC = append([]string(nil), article.CC...)
	clone.BTo = append([]string(nil), article.BTo...)
	clone.BCC = append([]string(nil), article.BCC...)
	clone.TableOfContents = append([]models.TOCEntry(nil), article.TableOfContents...)
	clone.CategoryIDs = append([]string(nil), article.CategoryIDs...)

	if article.InReplyTo != nil {
		inReplyTo := *article.InReplyTo
		clone.InReplyTo = &inReplyTo
	}
	if article.SeriesID != nil {
		seriesID := *article.SeriesID
		clone.SeriesID = &seriesID
	}
	if article.SeriesOrder != nil {
		seriesOrder := *article.SeriesOrder
		clone.SeriesOrder = &seriesOrder
	}
	if article.FeaturedImage != nil {
		featuredImage := *article.FeaturedImage
		clone.FeaturedImage = &featuredImage
	}

	return &clone
}

func (s *ArticleService) federateArticleUpdate(ctx context.Context, article *models.Article) {
	s.federateArticleWriteActivity(ctx, article, activitypub.UpdateType, "update")
}

func (s *ArticleService) federateArticleWriteActivity(ctx context.Context, article *models.Article, activityType string, label string) {
	logCMSArticleFederationAttempt(s.logger, label, activityType, article)

	apArticle, err := transformations.StorageArticleToActivityPub(article)
	if err != nil {
		s.logger.Error("failed to convert article to AP for federation",
			zap.String("label", label),
			zap.String("article_id", cmsArticleID(article)),
			zap.Error(err))
		logCMSArticleFederationFailure(s.logger, label, cmsFederationFailureStageTransform, activityType, "", article, err)
		return
	}

	username := extractUsernameFromActorID(article.AttributedTo)
	apActor, err := s.actorRepo.GetActor(ctx, username)
	if err != nil {
		s.logger.Error("failed to get actor for article federation",
			zap.String("label", label),
			zap.String("actor_id", article.AttributedTo),
			zap.Error(err))
		logCMSArticleFederationFailure(s.logger, label, cmsFederationFailureStageActor, activityType, "", article, err)
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
		logCMSArticleFederationFailure(s.logger, label, cmsFederationFailureStageDelivery, activityType, activityID, article, err)
	} else {
		s.logger.Info("successfully federated article activity",
			zap.String("label", label),
			zap.String("article_id", article.ID),
			zap.String("activity_id", activityID))
		logCMSArticleFederationSuccess(s.logger, label, activityType, activityID, article)
	}
}
func (s *ArticleService) federateArticleDeletion(ctx context.Context, article *models.Article) {
	logCMSArticleFederationAttempt(s.logger, "delete", activitypub.DeleteType, article)

	username := extractUsernameFromActorID(article.AttributedTo)
	apActor, err := s.actorRepo.GetActor(ctx, username)
	if err != nil {
		s.logger.Error("failed to get actor for article delete federation",
			zap.String("actor_id", article.AttributedTo),
			zap.Error(err))
		logCMSArticleFederationFailure(s.logger, "delete", cmsFederationFailureStageActor, activitypub.DeleteType, "", article, err)
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
		logCMSArticleFederationFailure(s.logger, "delete", cmsFederationFailureStageDelivery, activitypub.DeleteType, activityID, article, err)
	} else {
		s.logger.Info("successfully federated article delete",
			zap.String("article_id", article.ID),
			zap.String("activity_id", activityID))
		logCMSArticleFederationSuccess(s.logger, "delete", activitypub.DeleteType, activityID, article)
	}
}

func (s *ArticleService) deliverFederatedArticleActivity(ctx context.Context, activity *activitypub.Activity, actor *activitypub.Actor) error {
	var followerErr error
	if articleActivityHasPublicAddressing(activity) {
		if err := s.federation.DeliverToFollowers(ctx, activity, actor); err != nil {
			followerErr = err
			s.logger.Error("failed to deliver article activity to followers",
				zap.String("activity_id", activity.ID),
				zap.Error(err))
		}
	}

	if err := s.federation.DeliverToRecipients(ctx, activity, actor); err != nil {
		return err
	}
	return followerErr
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

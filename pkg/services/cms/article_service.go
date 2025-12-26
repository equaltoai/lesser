// Package cms provides services for Content Management System functionality
package cms

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/equaltoai/lesser/pkg/transformations"
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
	revisionService *RevisionService
	federation      FederationService
	logger          *zap.Logger
}

// NewArticleService creates a new ArticleService
func NewArticleService(
	articleRepo *repositories.ArticleRepository,
	actorRepo *repositories.ActorRepository,
	revisionService *RevisionService,
	federation FederationService,
	logger *zap.Logger,
) *ArticleService {
	return &ArticleService{
		articleRepo:     articleRepo,
		actorRepo:       actorRepo,
		revisionService: revisionService,
		federation:      federation,
		logger:          logger,
	}
}

// CreateArticle creates a new article
func (s *ArticleService) CreateArticle(ctx context.Context, article *models.Article) error {
	s.logger.Info("creating article", zap.String("title", article.Name))

	if article.CreatedAt.IsZero() {
		article.CreatedAt = time.Now()
	}
	article.UpdatedAt = time.Now()

	if err := s.articleRepo.CreateArticle(ctx, article); err != nil {
		return err
	}

	// Federate the article creation asynchronously
	go s.federateArticleCreation(context.Background(), article)

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

	// Snapshot existing state before applying the update.
	if s.revisionService != nil {
		existing, err := s.articleRepo.GetArticle(ctx, strings.TrimSpace(article.ID))
		if err == nil && existing != nil {
			_, _ = s.revisionService.CreateRevision(ctx, existing)
		}
	}

	if article.UpdatedAt.IsZero() {
		article.UpdatedAt = time.Now()
	}
	if article.Updated.IsZero() {
		article.Updated = article.UpdatedAt
	}

	if err := s.articleRepo.UpdateArticle(ctx, article); err != nil {
		return err
	}

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

	go s.federateArticleDeletion(context.Background(), article)

	return nil
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
	activityID := fmt.Sprintf("%s/activities/%s-%d-%s", apActor.ID, label, now.Unix(), article.ID)
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
	activityID := fmt.Sprintf("%s/activities/delete-%d-%s", apActor.ID, now.Unix(), article.ID)
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

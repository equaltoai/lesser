package cms

import (
	"context"
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

func (s *ArticleService) federateArticleCreation(ctx context.Context, article *models.Article) {
	// 1. Convert to ActivityPub Article
	apArticle, err := transformations.StorageArticleToActivityPub(article)
	if err != nil {
		s.logger.Error("failed to convert article to AP for federation",
			zap.String("article_id", article.ID),
			zap.Error(err))
		return
	}

	// 2. Get the actor (author) - GetActor returns *activitypub.Actor directly
	username := extractUsernameFromActorID(article.AttributedTo)
	apActor, err := s.actorRepo.GetActor(ctx, username)
	if err != nil {
		s.logger.Error("failed to get actor for article federation",
			zap.String("actor_id", article.AttributedTo),
			zap.Error(err))
		return
	}

	// 3. Create Create Activity
	now := time.Now()
	activityID := fmt.Sprintf("%s/activities/create-%d-%s", apActor.ID, now.Unix(), article.ID)

	activity := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			Context:   activitypub.Context,
			Type:      activitypub.CreateType,
			ID:        activityID,
			To:        apArticle.To,
			CC:        apArticle.CC,
			Published: &now,
		},
		Actor:  apActor.ID,
		Object: apArticle,
	}

	// 4. Deliver
	if err := s.federation.DeliverToFollowers(ctx, activity, apActor); err != nil {
		s.logger.Error("failed to deliver article creation activity",
			zap.String("activity_id", activityID),
			zap.Error(err))
	} else {
		s.logger.Info("successfully federated article creation",
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

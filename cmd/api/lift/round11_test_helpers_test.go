package lift

import (
	"context"
	"testing"

	"github.com/equaltoai/lesser/graph"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/mastodon"
	"github.com/equaltoai/lesser/pkg/storage"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
)

const round11StrongJWTSecret = "k9mG8xPq1Vv3nQw4rTz5Yb7u8i0o6Hj2"

type round11UserRepoDeps struct{}

func (round11UserRepoDeps) GetFollowers(ctx context.Context, username string, limit int, cursor string) ([]string, string, error) {
	return nil, "", nil
}

func (round11UserRepoDeps) GetListsContainingAccount(ctx context.Context, accountID, username string) ([]*storage.List, error) {
	return nil, nil
}

func (round11UserRepoDeps) CreateTimelineEntries(ctx context.Context, entries []*storagemodels.Timeline) error {
	return nil
}

func (round11UserRepoDeps) GetPendingFollowRequests(ctx context.Context, username string, limit int, cursor string) ([]string, string, error) {
	return nil, "", nil
}

func (round11UserRepoDeps) RemoveFollow(ctx context.Context, followerUsername, username string) error {
	return nil
}

func round11TestConfig() *config.Config {
	return &config.Config{
		Domain:          "example.com",
		JWTSecret:       round11StrongJWTSecret,
		DynamoTableName: "test-table",
		Stage:           "development",
	}
}

func round11NewHandler(t *testing.T, args ...any) (*Handler, *MockRepositoryStorage, *round10DynamoHarness) {
	t.Helper()

	var cfg *config.Config
	var state *round10QueryState
	var reg ServiceRegistry

	for _, arg := range args {
		switch v := arg.(type) {
		case *config.Config:
			cfg = v
		case *round10QueryState:
			state = v
		case ServiceRegistry:
			reg = v
		}
	}

	if cfg == nil {
		cfg = round11TestConfig()
	}
	if state == nil {
		state = &round10QueryState{}
	}

	logger := round10TestLogger(t)
	harness := round10NewDynamoHarness(t, state)

	accountRepo := repositories.NewAccountRepository(harness.db, cfg.DynamoTableName, cfg.Domain, logger)
	actorRepo := repositories.NewActorRepository(harness.db, cfg.DynamoTableName, logger)
	objectRepo := repositories.NewObjectRepository(harness.db, cfg.DynamoTableName, cfg.Domain, logger)
	activityRepo := repositories.NewActivityRepository(harness.db, cfg.DynamoTableName, logger, nil)
	statusRepo := repositories.NewStatusRepository(harness.db, cfg.DynamoTableName, logger, nil)
	userRepo := repositories.NewUserRepository(harness.db, cfg.DynamoTableName, logger)
	userRepo.SetDependencies(round11UserRepoDeps{})
	likeRepo := repositories.NewLikeRepository(harness.db, cfg.DynamoTableName, logger)
	moderationRepo := repositories.NewModerationRepository(harness.db, cfg.DynamoTableName, logger)
	socialRepo := repositories.NewSocialRepository(harness.db, cfg.DynamoTableName, logger, nil)
	trustRepo := repositories.NewTrustRepository(harness.db, cfg.DynamoTableName, logger, nil)
	bookmarkRepo := repositories.NewBookmarkRepository(harness.db, cfg.DynamoTableName, logger)
	conversationRepo := repositories.NewConversationRepository(harness.db, cfg.DynamoTableName, logger, nil)
	pollRepo := repositories.NewPollRepository(harness.db, cfg.DynamoTableName, logger, nil)
	hashtagRepo := repositories.NewHashtagRepository(harness.db, cfg.DynamoTableName, logger, cfg.Domain)
	featuredTagRepo := repositories.NewFeaturedTagRepository(harness.db, cfg.DynamoTableName, logger, nil)
	announcementRepo := repositories.NewAnnouncementRepository(harness.db, cfg.DynamoTableName, logger)
	domainBlockRepo := repositories.NewDomainBlockRepository(harness.db, cfg.DynamoTableName, logger)
	relationshipRepo := repositories.NewRelationshipRepository(harness.db, cfg.DynamoTableName, logger)
	instanceRepo := repositories.NewInstanceRepository(harness.db, cfg.DynamoTableName, logger)
	recoveryRepo := repositories.NewRecoveryRepository(harness.db, cfg.DynamoTableName, logger, nil)
	quoteRepo := repositories.NewQuoteRepository(harness.db, cfg.DynamoTableName, logger, nil)
	emojiRepo := repositories.NewEmojiRepository(harness.db, cfg.DynamoTableName, logger, nil)
	notificationRepo := repositories.NewNotificationRepository(harness.db, cfg.DynamoTableName, logger, nil)
	trendingRepo := repositories.NewTrendingRepository(harness.db, logger, nil)
	auditRepo := repositories.NewAuditRepository(harness.db, cfg.DynamoTableName, logger, nil)
	pushSubscriptionRepo := repositories.NewPushSubscriptionRepository(harness.db, cfg.DynamoTableName, logger, nil, nil, "", "mailto:push@example.com")
	searchRepo := repositories.NewSearchRepository(harness.db, cfg.DynamoTableName, logger, nil)
	importRepo := repositories.NewImportRepository(harness.db, cfg.DynamoTableName, logger)
	costRepo := repositories.NewTrackingRepository(harness.db, cfg.DynamoTableName, logger, nil)
	oauthRepo := repositories.NewOAuthRepository(harness.db, logger)
	trendingRepo.SetStatusRepository(statusRepo)
	exportRepo := repositories.NewExportRepository(harness.db, cfg.DynamoTableName, logger)
	repos := &MockRepositoryStorage{}
	repos.On("Account").Return(accountRepo).Maybe()
	repos.On("Actor").Return(actorRepo).Maybe()
	repos.On("Object").Return(objectRepo).Maybe()
	repos.On("Activity").Return(activityRepo).Maybe()
	repos.On("Status").Return(statusRepo).Maybe()
	repos.On("User").Return(userRepo).Maybe()
	repos.On("Like").Return(likeRepo).Maybe()
	repos.On("Moderation").Return(moderationRepo).Maybe()
	repos.On("Social").Return(socialRepo).Maybe()
	repos.On("Trust").Return(trustRepo).Maybe()
	repos.On("Bookmark").Return(bookmarkRepo).Maybe()
	repos.On("Conversation").Return(conversationRepo).Maybe()
	repos.On("Poll").Return(pollRepo).Maybe()
	repos.On("Hashtag").Return(hashtagRepo).Maybe()
	repos.On("FeaturedTag").Return(featuredTagRepo).Maybe()
	repos.On("Announcement").Return(announcementRepo).Maybe()
	repos.On("DomainBlock").Return(domainBlockRepo).Maybe()
	repos.On("Relationship").Return(relationshipRepo).Maybe()
	repos.On("Instance").Return(instanceRepo).Maybe()
	repos.On("Recovery").Return(recoveryRepo).Maybe()
	repos.On("Quote").Return(quoteRepo).Maybe()
	repos.On("Emoji").Return(emojiRepo).Maybe()
	repos.On("Notification").Return(notificationRepo).Maybe()
	repos.On("Analytics").Return(trendingRepo).Maybe()
	repos.On("Audit").Return(auditRepo).Maybe()
	repos.On("PushSubscription").Return(pushSubscriptionRepo).Maybe()
	repos.On("Search").Return(searchRepo).Maybe()
	repos.On("Import").Return(importRepo).Maybe()
	repos.On("Export").Return(exportRepo).Maybe()
	repos.On("Cost").Return(costRepo).Maybe()
	repos.On("OAuth").Return(oauthRepo).Maybe()
	repos.On("OAuth").Return(oauthRepo).Maybe()
	repos.On("GetDB").Return(harness.db).Maybe()
	repos.On("GetTableName").Return(cfg.DynamoTableName).Maybe()
	repos.On("GetLogger").Return(logger).Maybe()

	handler := &Handler{
		cfg:       cfg,
		repos:     repos,
		logger:    logger,
		registry:  reg,
		converter: mastodon.NewConverterWithEmojis(cfg.BaseURL(), emojiRepo),
		loaders:   graph.NewLoaders(repos, logger),
	}

	return handler, repos, harness
}

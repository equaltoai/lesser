package services

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"io"
	"net/http"
	"os"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/activitypub"
	pkgconfig "github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/services/media"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	dynamormcore "github.com/theory-cloud/tabletheory/pkg/core"
	dynamormMocks "github.com/theory-cloud/tabletheory/pkg/mocks"
	"go.uber.org/zap"
)

type permissiveRegistryStorage struct {
	*mockStorage

	db        dynamormcore.DB
	tableName string
	logger    *zap.Logger

	account             *repositories.AccountRepository
	bookmark            *repositories.BookmarkRepository
	actor               interfaces.ActorRepository
	object              interfaces.ObjectRepository
	activity            interfaces.ActivityRepository
	timeline            interfaces.TimelineRepository
	notification        interfaces.NotificationRepository
	like                *repositories.LikeRepository
	list                *repositories.ListRepository
	media               *repositories.MediaRepository
	poll                *repositories.PollRepository
	hashtag             *repositories.HashtagRepository
	scheduledStatus     *repositories.ScheduledStatusRepository
	relationship        interfaces.ConcreteRelationshipRepository
	federation          *repositories.FederationRepository
	social              *repositories.SocialRepository
	user                *repositories.UserRepository
	status              *repositories.StatusRepository
	search              *repositories.SearchRepository
	communityNote       *repositories.CommunityNoteRepository
	emoji               *repositories.EmojiRepository
	conversation        *repositories.ConversationRepository
	exportRepo          *repositories.ExportRepository
	importRepo          *repositories.ImportRepository
	threadRepo          *repositories.ThreadRepository
	severanceRepo       *repositories.SeveranceRepository
	moderationMLRepo    *repositories.ModerationMLRepository
	mediaAnalyticsRepo  interfaces.MediaAnalyticsRepository
	mediaPopularityRepo interfaces.MediaPopularityRepository
	mediaSessionRepo    interfaces.MediaSessionRepository
	streamingConnRepo   interfaces.StreamingConnectionRepository

	// CMS repos
	articleRepo           interfaces.ArticleRepository
	draftRepo             interfaces.DraftRepository
	revisionRepo          interfaces.RevisionRepository
	seriesRepo            interfaces.SeriesRepository
	categoryRepo          interfaces.CategoryRepository
	publicationRepo       interfaces.PublicationRepository
	publicationMemberRepo interfaces.PublicationMemberRepository
}

func (s *permissiveRegistryStorage) Account() *repositories.AccountRepository   { return s.account }
func (s *permissiveRegistryStorage) Bookmark() *repositories.BookmarkRepository { return s.bookmark }
func (s *permissiveRegistryStorage) Actor() interfaces.ActorRepository          { return s.actor }
func (s *permissiveRegistryStorage) Object() interfaces.ObjectRepository        { return s.object }
func (s *permissiveRegistryStorage) Activity() interfaces.ActivityRepository    { return s.activity }
func (s *permissiveRegistryStorage) Timeline() interfaces.TimelineRepository    { return s.timeline }
func (s *permissiveRegistryStorage) Notification() interfaces.NotificationRepository {
	return s.notification
}
func (s *permissiveRegistryStorage) Like() *repositories.LikeRepository { return s.like }
func (s *permissiveRegistryStorage) List() *repositories.ListRepository { return s.list }
func (s *permissiveRegistryStorage) Media() *repositories.MediaRepository {
	return s.media
}
func (s *permissiveRegistryStorage) Poll() *repositories.PollRepository { return s.poll }
func (s *permissiveRegistryStorage) Hashtag() *repositories.HashtagRepository {
	return s.hashtag
}
func (s *permissiveRegistryStorage) ScheduledStatus() *repositories.ScheduledStatusRepository {
	return s.scheduledStatus
}
func (s *permissiveRegistryStorage) Relationship() interfaces.ConcreteRelationshipRepository {
	return s.relationship
}
func (s *permissiveRegistryStorage) Federation() *repositories.FederationRepository {
	return s.federation
}
func (s *permissiveRegistryStorage) Social() *repositories.SocialRepository { return s.social }
func (s *permissiveRegistryStorage) User() interfaces.UserRepository        { return s.user }
func (s *permissiveRegistryStorage) Status() interfaces.StatusRepository    { return s.status }
func (s *permissiveRegistryStorage) Search() *repositories.SearchRepository { return s.search }
func (s *permissiveRegistryStorage) CommunityNote() *repositories.CommunityNoteRepository {
	return s.communityNote
}
func (s *permissiveRegistryStorage) Emoji() *repositories.EmojiRepository { return s.emoji }
func (s *permissiveRegistryStorage) Conversation() *repositories.ConversationRepository {
	return s.conversation
}
func (s *permissiveRegistryStorage) Export() *repositories.ExportRepository { return s.exportRepo }
func (s *permissiveRegistryStorage) Import() *repositories.ImportRepository { return s.importRepo }
func (s *permissiveRegistryStorage) Thread() *repositories.ThreadRepository { return s.threadRepo }
func (s *permissiveRegistryStorage) Severance() *repositories.SeveranceRepository {
	return s.severanceRepo
}
func (s *permissiveRegistryStorage) ModerationML() *repositories.ModerationMLRepository {
	return s.moderationMLRepo
}
func (s *permissiveRegistryStorage) MediaAnalytics() interfaces.MediaAnalyticsRepository {
	return s.mediaAnalyticsRepo
}
func (s *permissiveRegistryStorage) MediaPopularity() interfaces.MediaPopularityRepository {
	return s.mediaPopularityRepo
}
func (s *permissiveRegistryStorage) MediaSession() interfaces.MediaSessionRepository {
	return s.mediaSessionRepo
}
func (s *permissiveRegistryStorage) StreamingConnection() interfaces.StreamingConnectionRepository {
	return s.streamingConnRepo
}
func (s *permissiveRegistryStorage) Article() interfaces.ArticleRepository { return s.articleRepo }
func (s *permissiveRegistryStorage) Draft() interfaces.DraftRepository     { return s.draftRepo }
func (s *permissiveRegistryStorage) Revision() interfaces.RevisionRepository {
	return s.revisionRepo
}
func (s *permissiveRegistryStorage) Series() interfaces.SeriesRepository { return s.seriesRepo }
func (s *permissiveRegistryStorage) Category() interfaces.CategoryRepository {
	return s.categoryRepo
}
func (s *permissiveRegistryStorage) Publication() interfaces.PublicationRepository {
	return s.publicationRepo
}
func (s *permissiveRegistryStorage) PublicationMember() interfaces.PublicationMemberRepository {
	return s.publicationMemberRepo
}
func (s *permissiveRegistryStorage) GetDB() dynamormcore.DB { return s.db }
func (s *permissiveRegistryStorage) GetTableName() string   { return s.tableName }
func (s *permissiveRegistryStorage) GetLogger() *zap.Logger { return s.logger }

func newPermissiveRegistryStorage(t *testing.T, domain string, logger *zap.Logger) *permissiveRegistryStorage {
	t.Helper()

	if logger == nil {
		logger = zap.NewNop()
	}

	db := newPermissiveDynamormDB(t)
	tableName := "test-table"

	return &permissiveRegistryStorage{
		mockStorage: &mockStorage{},
		db:          db,
		tableName:   tableName,
		logger:      logger,

		account:             repositories.NewAccountRepository(db, tableName, domain, logger),
		bookmark:            repositories.NewBookmarkRepository(db, tableName, logger),
		actor:               repositories.NewActorRepository(db, tableName, logger),
		object:              repositories.NewObjectRepository(db, tableName, domain, logger),
		activity:            repositories.NewActivityRepository(db, tableName, logger, nil),
		timeline:            repositories.NewTimelineRepository(db, tableName, logger, nil),
		notification:        repositories.NewNotificationRepository(db, tableName, logger, nil),
		like:                repositories.NewLikeRepository(db, tableName, logger),
		list:                repositories.NewListRepository(db, tableName, logger, nil),
		media:               repositories.NewMediaRepository(db, tableName, logger, nil),
		poll:                repositories.NewPollRepository(db, tableName, logger, nil),
		hashtag:             repositories.NewHashtagRepository(db, tableName, logger, domain),
		scheduledStatus:     repositories.NewScheduledStatusRepository(db, tableName, logger, nil),
		relationship:        repositories.NewRelationshipRepository(db, tableName, logger),
		federation:          repositories.NewFederationRepository(db, tableName, logger, nil, nil),
		social:              repositories.NewSocialRepository(db, tableName, logger, nil),
		user:                repositories.NewUserRepository(db, tableName, logger),
		status:              repositories.NewStatusRepository(db, tableName, logger, nil),
		search:              repositories.NewSearchRepository(db, tableName, logger, nil),
		communityNote:       repositories.NewCommunityNoteRepository(db, tableName, logger, nil),
		emoji:               repositories.NewEmojiRepository(db, tableName, logger, nil),
		conversation:        repositories.NewConversationRepository(db, tableName, logger, nil),
		exportRepo:          repositories.NewExportRepository(db, tableName, logger),
		importRepo:          repositories.NewImportRepository(db, tableName, logger),
		threadRepo:          repositories.NewThreadRepository(db, logger),
		severanceRepo:       repositories.NewSeveranceRepository(db, tableName, logger),
		moderationMLRepo:    repositories.NewModerationMLRepository(db, tableName, logger),
		mediaAnalyticsRepo:  repositories.NewMediaAnalyticsRepository(db, tableName, logger, nil),
		mediaPopularityRepo: repositories.NewMediaPopularityRepository(db, tableName, logger, nil),
		mediaSessionRepo:    repositories.NewMediaSessionRepository(db, logger, nil),
		streamingConnRepo:   repositories.NewStreamingConnectionRepository(db, tableName, db, tableName, logger, nil),

		articleRepo:           repositories.NewArticleRepository(db, tableName, logger, nil),
		draftRepo:             repositories.NewDraftRepository(db, tableName, logger, nil),
		revisionRepo:          repositories.NewRevisionRepository(db, tableName, logger, nil),
		seriesRepo:            repositories.NewSeriesRepository(db, tableName, logger, nil),
		categoryRepo:          repositories.NewCategoryRepository(db, tableName, logger, nil),
		publicationRepo:       repositories.NewPublicationRepository(db, tableName, logger, nil),
		publicationMemberRepo: repositories.NewPublicationMemberRepository(db, tableName, logger, nil),
	}
}

func setTestAWSEnv(t *testing.T) {
	t.Helper()
	t.Setenv("AWS_REGION", "us-east-1")
	t.Setenv("AWS_DEFAULT_REGION", "us-east-1")
	t.Setenv("AWS_ACCESS_KEY_ID", "test")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "test")
	t.Setenv("AWS_SESSION_TOKEN", "test")
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
}

func TestRegistry_DomainAndCMSHelpers(t *testing.T) {
	storage := newMockStorage()
	cfg := &ServiceConfig{
		BaseURL:   "https://example.com",
		JWTSecret: strings.Repeat("x", 32),
		Config:    &pkgconfig.Config{Domain: "cms.example.com"},
	}
	reg, err := NewRegistry(WithStorage(storage), WithConfig(cfg), WithLogger(zap.NewNop()))
	require.NoError(t, err)

	assert.Equal(t, "example.com", reg.getDomainName())
	reg.config.BaseURL = "http://example.com"
	assert.Equal(t, "example.com", reg.getDomainName())
	reg.config.BaseURL = "example.com"
	assert.Equal(t, DefaultLocalhost, reg.getDomainName())

	assert.Equal(t, "cms.example.com", reg.getCMSDomainName())
	reg.config.Config.Domain = ""
	reg.config.BaseURL = "https://example.com"
	assert.Equal(t, "example.com", reg.getCMSDomainName())

	reg.config.Config.CMSMaxRevisionsPerObject = 12
	assert.Equal(t, 12, reg.getCMSMaxRevisionsPerObject())
	reg.config.Config.CMSMaxRevisionsPerObject = 0
	assert.Equal(t, 0, reg.getCMSMaxRevisionsPerObject())

	// When config is nil, CMS defaults to enabled in the registry.
	regNilCfg, err := NewRegistry(WithStorage(storage), WithConfig(&ServiceConfig{
		BaseURL:   "https://example.com",
		JWTSecret: strings.Repeat("x", 32),
	}))
	require.NoError(t, err)
	assert.True(t, regNilCfg.cmsLongFormEnabled())
	assert.True(t, regNilCfg.cmsDraftsEnabled())
	assert.True(t, regNilCfg.cmsRevisionsEnabled())
	assert.True(t, regNilCfg.cmsSchedulingEnabled())
	assert.True(t, regNilCfg.cmsSeriesEnabled())
	assert.True(t, regNilCfg.cmsCategoriesEnabled())

	// When Config is present but instance mode disables CMS, helpers should return false.
	regCMSOff, err := NewRegistry(WithStorage(storage), WithConfig(&ServiceConfig{
		BaseURL:   "https://example.com",
		JWTSecret: strings.Repeat("x", 32),
		Config: &pkgconfig.Config{
			InstanceMode: pkgconfig.InstanceModeSocial,
		},
	}))
	require.NoError(t, err)
	assert.False(t, regCMSOff.cmsLongFormEnabled())
	assert.False(t, regCMSOff.cmsDraftsEnabled())
	assert.False(t, regCMSOff.cmsRevisionsEnabled())
	assert.False(t, regCMSOff.cmsSchedulingEnabled())
	assert.False(t, regCMSOff.cmsSeriesEnabled())
	assert.False(t, regCMSOff.cmsCategoriesEnabled())
}

func TestRegistry_CMSServices_EnabledAndDisabled(t *testing.T) {
	setTestAWSEnv(t)

	logger := zap.NewNop()
	storage := newPermissiveRegistryStorage(t, "example.com", logger)
	cfg := &ServiceConfig{
		BaseURL:   "https://example.com",
		JWTSecret: strings.Repeat("x", 32),
		Config: &pkgconfig.Config{
			Domain:                        "cms.example.com",
			InstanceMode:                  pkgconfig.InstanceModeHybrid,
			CMSLongFormPublishingEnabled:  true,
			CMSDraftSystemEnabled:         true,
			CMSRevisionHistoryEnabled:     true,
			CMSScheduledPublishingEnabled: true,
			CMSSeriesEnabled:              true,
			CMSCategoriesEnabled:          true,
			CMSMaxRevisionsPerObject:      5,
		},
	}

	reg, err := NewRegistry(WithStorage(storage), WithLogger(logger), WithConfig(cfg))
	require.NoError(t, err)

	// Call Articles first to exercise the inline revision initialization path.
	require.NotNil(t, reg.Articles())
	require.NotNil(t, reg.Revisions())
	require.NotNil(t, reg.Drafts())
	require.NotNil(t, reg.Series())
	require.NotNil(t, reg.Categories())
	require.NotNil(t, reg.Publications())

	// Call Drafts first on a fresh registry to exercise ensureCMSArticleServiceLocked.
	regDraftFirst, err := NewRegistry(WithStorage(storage), WithLogger(logger), WithConfig(cfg))
	require.NoError(t, err)
	require.NotNil(t, regDraftFirst.Drafts())

	// Disabled CMS should return nil without touching repos.
	regDisabled, err := NewRegistry(WithStorage(storage), WithLogger(logger), WithConfig(&ServiceConfig{
		BaseURL:   "https://example.com",
		JWTSecret: strings.Repeat("x", 32),
		Config: &pkgconfig.Config{
			InstanceMode: pkgconfig.InstanceModeSocial,
		},
	}))
	require.NoError(t, err)
	assert.Nil(t, regDisabled.Revisions())
	assert.Nil(t, regDisabled.Articles())
	assert.Nil(t, regDisabled.Drafts())
	assert.Nil(t, regDisabled.Series())
	assert.Nil(t, regDisabled.Categories())
	assert.Nil(t, regDisabled.Publications())
}

func TestRegistry_ServiceAccessors_AndAdapters(t *testing.T) {
	setTestAWSEnv(t)

	logger := zap.NewNop()
	storage := newPermissiveRegistryStorage(t, "example.com", logger)
	publisher := newMockPublisher()

	dir := t.TempDir()
	privateKeyPath := dir + "/cloudfront.pem"
	privateKeyPEM := mustGenerateRSAPrivateKeyPEM(t)
	require.NoError(t, os.WriteFile(privateKeyPath, []byte(privateKeyPEM), 0o600))

	cfg := &ServiceConfig{
		BaseURL:   "https://example.com",
		JWTSecret: strings.Repeat("x", 32),
		Config: &pkgconfig.Config{
			Domain:                        "example.com",
			InstanceMode:                  pkgconfig.InstanceModeHybrid,
			CMSLongFormPublishingEnabled:  true,
			CMSDraftSystemEnabled:         true,
			CMSRevisionHistoryEnabled:     true,
			CMSScheduledPublishingEnabled: true,
			CMSSeriesEnabled:              true,
			CMSCategoriesEnabled:          true,
			CMSMaxRevisionsPerObject:      5,

			MediaSourceBucketName:    "source-bucket",
			MediaStreamingBucketName: "streaming-bucket",
			CloudFrontDomain:         "cdn.example.com",
			ManifestTTLHours:         12,
			MediaConvertEndpoint:     "http://mediaconvert.local",
			MediaConvertRoleArn:      "arn:aws:iam::123456789012:role/mediaconvert",
			CloudFrontKeyPairID:      "K1234567890",
			CloudFrontPrivateKeyPath: privateKeyPath,

			ModerationTrainingBucketName: "train-bucket",
			BedrockTrainingRegion:        "us-east-1",
			BedrockInferenceModelID:      "bedrock-model",
			BedrockGuardrailID:           "guardrail-id",
			BedrockGuardrailVersion:      "",
			BedrockCustomizationRoleARN:  "",
		},
	}

	reg, err := NewRegistry(
		WithStorage(storage),
		WithPublisher(publisher),
		WithLogger(logger),
		WithConfig(cfg),
	)
	require.NoError(t, err)

	// Core services
	require.NotNil(t, reg.BusinessLogic())
	require.NotNil(t, reg.Validation())
	require.NotNil(t, reg.Authentication())
	require.NotNil(t, reg.Federation())
	require.NotNil(t, reg.Timeline())
	require.NotNil(t, reg.Analytics())
	require.NotNil(t, reg.Notification())
	require.NotNil(t, reg.QueryTracker())

	// Domain services
	require.NotNil(t, reg.Threads())
	require.NotNil(t, reg.Severance())
	require.NotNil(t, reg.FederationGraph())
	require.NotNil(t, reg.StreamingAnalytics())
	require.NotNil(t, reg.ModerationML())
	require.NotNil(t, reg.Performance())
	require.NotNil(t, reg.Notes())
	require.NotNil(t, reg.Accounts())
	require.NotNil(t, reg.Relationships())
	require.NotNil(t, reg.Conversations())
	require.NotNil(t, reg.Media())
	require.NotNil(t, reg.Lists())
	require.NotNil(t, reg.Notifications())
	require.NotNil(t, reg.AI())
	require.NotNil(t, reg.Emoji())
	require.NotNil(t, reg.Hashtags())
	require.NotNil(t, reg.Scheduled())
	require.NotNil(t, reg.Search())
	require.NotNil(t, reg.Bulk())
	require.NotNil(t, reg.Quotes())

	require.NotNil(t, reg.StreamingConnectionRepository())
	require.Equal(t, publisher, reg.Publisher())

	// Adapters and helpers
	require.NotNil(t, reg.createNotesFederationAdapterUnlocked())
	require.NotNil(t, reg.createThreadsFederationAdapterUnlocked())
	require.NotNil(t, reg.createSeveranceFederationAdapter())
	require.NotNil(t, reg.createSeveranceNotificationAdapter())

	adapter := reg.createSeveranceEventPublisherAdapter()
	require.NotNil(t, adapter)
	require.NoError(t, adapter.PublishEvent(context.Background(), &models.StreamingEvent{EventID: "e1"}))

	awsCfg1, err := reg.getAWSConfig()
	require.NoError(t, err)
	awsCfg2, err := reg.getAWSConfig()
	require.NoError(t, err)
	require.Same(t, awsCfg1, awsCfg2)

	// Config helpers
	assert.Equal(t, cfg.Config.MediaConvertEndpoint, reg.getConfigString(cfg.Config, "MediaConvertEndpoint"))
	assert.Equal(t, cfg.Config.MediaConvertRoleArn, reg.getConfigString(cfg.Config, "MediaConvertRoleArn"))
	assert.Equal(t, cfg.Config.CloudFrontKeyPairID, reg.getConfigString(cfg.Config, "CloudFrontKeyPairID"))
	assert.Equal(t, cfg.Config.CloudFrontPrivateKeyPath, reg.getConfigString(cfg.Config, "CloudFrontPrivateKeyPath"))
	assert.Equal(t, cfg.Config.MediaSourceBucketName, reg.getConfigString(cfg.Config, "MediaSourceBucketName"))
	assert.Equal(t, cfg.Config.MediaStreamingBucketName, reg.getConfigString(cfg.Config, "MediaStreamingBucketName"))
	assert.Equal(t, cfg.Config.CloudFrontDomain, reg.getConfigString(cfg.Config, "CloudFrontDomain"))
	assert.Equal(t, cfg.Config.S3BucketName, reg.getConfigString(cfg.Config, "S3BucketName"))
	assert.Equal(t, cfg.Config.ModerationTrainingBucketName, reg.getConfigString(cfg.Config, "ModerationTrainingBucketName"))
	assert.Equal(t, cfg.Config.BedrockTrainingRegion, reg.getConfigString(cfg.Config, "BedrockTrainingRegion"))
	assert.Equal(t, cfg.Config.BedrockInferenceModelID, reg.getConfigString(cfg.Config, "BedrockInferenceModelID"))
	assert.Equal(t, cfg.Config.BedrockGuardrailID, reg.getConfigString(cfg.Config, "BedrockGuardrailID"))
	assert.Equal(t, cfg.Config.BedrockGuardrailVersion, reg.getConfigString(cfg.Config, "BedrockGuardrailVersion"))
	assert.Equal(t, cfg.Config.BedrockCustomizationRoleARN, reg.getConfigString(cfg.Config, "BedrockCustomizationRoleARN"))
	assert.Empty(t, reg.getConfigString(struct{}{}, "MediaConvertEndpoint"))
	assert.Empty(t, reg.getConfigString(cfg.Config, "does_not_exist"))
	assert.Equal(t, 12, reg.getConfigInt(cfg.Config, "ManifestTTLHours"))
	assert.Equal(t, 0, reg.getConfigInt(cfg.Config, "does_not_exist"))
	assert.Equal(t, "source-bucket", reg.getMediaSourceBucket())
	assert.Equal(t, "streaming-bucket", reg.getMediaStreamingBucket())
	assert.Equal(t, "cdn.example.com", reg.getCloudFrontDomain())

	// Domain extraction helper used by ImportExport / others.
	assert.Equal(t, "example.com", reg.extractDomainName())

	// Close is covered elsewhere; make sure it doesn't panic with initialized services.
	require.NoError(t, reg.Close())
}

func TestRegistry_ImportExport_ValidationShortCircuit_AvoidsAWSStorageClient(t *testing.T) {
	setTestAWSEnv(t)

	storage := newMockStorage() // missing repos => validateImportExportRepositories returns false
	reg, err := NewRegistry(WithStorage(storage), WithLogger(zap.NewNop()), WithConfig(&ServiceConfig{
		BaseURL:   "https://example.com",
		JWTSecret: strings.Repeat("x", 32),
		Config: &pkgconfig.Config{
			Region: "us-east-1",
		},
	}))
	require.NoError(t, err)

	assert.Nil(t, reg.ImportExport())
	assert.False(t, reg.initializeImportExportService())
}

func TestRegistry_getSecretFromSecretsManager_StubbedEndpointAndCaching(t *testing.T) {
	setTestAWSEnv(t)

	t.Run("extracts_privateKey_from_JSON_and_caches", func(t *testing.T) {
		var calls atomic.Int32
		awsCfg, err := awsconfig.LoadDefaultConfig(context.Background())
		require.NoError(t, err)
		awsCfg.HTTPClient = &http.Client{Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			calls.Add(1)
			if req.Body != nil {
				_, _ = io.Copy(io.Discard, req.Body)
				_ = req.Body.Close()
			}
			body := `{"SecretString":"{\"privateKey\":\"pem-from-json\"}"}`
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/x-amz-json-1.1"}},
				Body:       io.NopCloser(strings.NewReader(body)),
				Request:    req,
			}, nil
		})}

		reg, err := NewRegistry(WithStorage(newMockStorage()), WithLogger(zap.NewNop()), WithConfig(&ServiceConfig{
			BaseURL:   "https://example.com",
			JWTSecret: strings.Repeat("x", 32),
			Config: &pkgconfig.Config{
				Region: "us-east-1",
			},
		}))
		require.NoError(t, err)
		reg.awsConfigCached = &awsCfg

		secret1, err := reg.getSecretFromSecretsManager("lesser/test")
		require.NoError(t, err)
		assert.Equal(t, "pem-from-json", secret1)

		secret2, err := reg.getSecretFromSecretsManager("lesser/test")
		require.NoError(t, err)
		assert.Equal(t, "pem-from-json", secret2)

		assert.Equal(t, int32(1), calls.Load())
	})

	t.Run("returns_plain_secret_string", func(t *testing.T) {
		awsCfg, err := awsconfig.LoadDefaultConfig(context.Background())
		require.NoError(t, err)
		awsCfg.HTTPClient = &http.Client{Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			if req.Body != nil {
				_, _ = io.Copy(io.Discard, req.Body)
				_ = req.Body.Close()
			}
			body := `{"SecretString":"plain-secret"}`
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/x-amz-json-1.1"}},
				Body:       io.NopCloser(strings.NewReader(body)),
				Request:    req,
			}, nil
		})}

		reg, err := NewRegistry(WithStorage(newMockStorage()), WithLogger(zap.NewNop()), WithConfig(&ServiceConfig{
			BaseURL:   "https://example.com",
			JWTSecret: strings.Repeat("x", 32),
			Config: &pkgconfig.Config{
				Region: "us-east-1",
			},
		}))
		require.NoError(t, err)
		reg.awsConfigCached = &awsCfg

		secret, err := reg.getSecretFromSecretsManager("lesser/plain")
		require.NoError(t, err)
		assert.Equal(t, "plain-secret", secret)
	})

	t.Run("errors_when_secret_string_missing", func(t *testing.T) {
		awsCfg, err := awsconfig.LoadDefaultConfig(context.Background())
		require.NoError(t, err)
		awsCfg.HTTPClient = &http.Client{Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			if req.Body != nil {
				_, _ = io.Copy(io.Discard, req.Body)
				_ = req.Body.Close()
			}
			body := `{"ARN":"x"}`
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/x-amz-json-1.1"}},
				Body:       io.NopCloser(strings.NewReader(body)),
				Request:    req,
			}, nil
		})}

		reg, err := NewRegistry(WithStorage(newMockStorage()), WithLogger(zap.NewNop()), WithConfig(&ServiceConfig{
			BaseURL:   "https://example.com",
			JWTSecret: strings.Repeat("x", 32),
			Config: &pkgconfig.Config{
				Region: "us-east-1",
			},
		}))
		require.NoError(t, err)
		reg.awsConfigCached = &awsCfg

		_, err = reg.getSecretFromSecretsManager("lesser/missing")
		assert.Error(t, err)
	})
}

func TestRegistry_ImportExport_Success_WithStubbedS3Endpoint(t *testing.T) {
	setTestAWSEnv(t)

	reg, err := NewRegistry(
		WithStorage(newPermissiveRegistryStorage(t, "example.com", zap.NewNop())),
		WithLogger(zap.NewNop()),
		WithConfig(&ServiceConfig{
			BaseURL:   "https://example.com",
			JWTSecret: strings.Repeat("x", 32),
			Config: &pkgconfig.Config{
				IntegrationTestMode: true,
				Region:              "us-east-1",
			},
		}),
	)
	require.NoError(t, err)

	assert.NotNil(t, reg.ImportExport())
	assert.NotNil(t, reg.ImportExport()) // cached branch
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestRegistry_NilStorageBranches(t *testing.T) {
	r := &Registry{logger: zap.NewNop()}
	assert.Nil(t, r.StreamingConnectionRepository())

	assert.NoError(t, (&severanceEventPublisherAdapter{storage: nil}).PublishEvent(context.Background(), &models.StreamingEvent{EventID: "e1"}))
	assert.NoError(t, (&severanceEventPublisherAdapter{storage: newMockStorage()}).PublishEvent(context.Background(), &models.StreamingEvent{EventID: "e1"}))
}

func TestRegistry_AdditionalBranches(t *testing.T) {
	setTestAWSEnv(t)

	logger := zap.NewNop()

	t.Run("Revisions_warns_when_repos_missing", func(t *testing.T) {
		storage := newPermissiveRegistryStorage(t, "example.com", logger)
		storage.revisionRepo = nil
		reg, err := NewRegistry(WithStorage(storage), WithLogger(logger), WithConfig(&ServiceConfig{
			BaseURL:   "https://example.com",
			JWTSecret: strings.Repeat("x", 32),
			Config: &pkgconfig.Config{
				InstanceMode:                 pkgconfig.InstanceModeHybrid,
				CMSLongFormPublishingEnabled: true,
				CMSRevisionHistoryEnabled:    true,
			},
		}))
		require.NoError(t, err)
		assert.Nil(t, reg.Revisions())
	})

	t.Run("Drafts_warns_when_draft_repo_missing", func(t *testing.T) {
		storage := newPermissiveRegistryStorage(t, "example.com", logger)
		storage.draftRepo = nil
		reg, err := NewRegistry(WithStorage(storage), WithLogger(logger), WithConfig(&ServiceConfig{
			BaseURL:   "https://example.com",
			JWTSecret: strings.Repeat("x", 32),
			Config: &pkgconfig.Config{
				InstanceMode:                 pkgconfig.InstanceModeHybrid,
				CMSLongFormPublishingEnabled: true,
				CMSDraftSystemEnabled:        true,
			},
		}))
		require.NoError(t, err)
		assert.Nil(t, reg.Drafts())
	})

	t.Run("Drafts_warns_when_article_service_unavailable", func(t *testing.T) {
		storage := newPermissiveRegistryStorage(t, "example.com", logger)
		storage.articleRepo = nil
		reg, err := NewRegistry(WithStorage(storage), WithLogger(logger), WithConfig(&ServiceConfig{
			BaseURL:   "https://example.com",
			JWTSecret: strings.Repeat("x", 32),
			Config: &pkgconfig.Config{
				InstanceMode:                 pkgconfig.InstanceModeHybrid,
				CMSLongFormPublishingEnabled: true,
				CMSDraftSystemEnabled:        true,
				CMSRevisionHistoryEnabled:    true,
			},
		}))
		require.NoError(t, err)
		assert.Nil(t, reg.Drafts())
	})

	t.Run("extractDomainName_cases", func(t *testing.T) {
		reg := &Registry{config: &ServiceConfig{BaseURL: ""}}
		assert.Equal(t, DefaultLocalhost, reg.extractDomainName())
		reg.config.BaseURL = "https://example.com"
		assert.Equal(t, "example.com", reg.extractDomainName())
		reg.config.BaseURL = "http://example.com"
		assert.Equal(t, "example.com", reg.extractDomainName())
		reg.config.BaseURL = "example.com"
		assert.Equal(t, DefaultLocalhost, reg.extractDomainName())
	})

	t.Run("Bulk_warns_when_repos_missing_and_returns_cached_instance", func(t *testing.T) {
		regMissing, err := NewRegistry(WithStorage(newMockStorage()), WithLogger(logger), WithConfig(&ServiceConfig{
			BaseURL:   "https://example.com",
			JWTSecret: strings.Repeat("x", 32),
			Config:    &pkgconfig.Config{InstanceMode: pkgconfig.InstanceModeHybrid},
		}))
		require.NoError(t, err)
		assert.Nil(t, regMissing.Bulk())

		reg, err := NewRegistry(WithStorage(newPermissiveRegistryStorage(t, "example.com", logger)), WithLogger(logger), WithConfig(&ServiceConfig{
			BaseURL:   "https://example.com",
			JWTSecret: strings.Repeat("x", 32),
			Config:    &pkgconfig.Config{InstanceMode: pkgconfig.InstanceModeHybrid},
		}))
		require.NoError(t, err)
		require.NotNil(t, reg.Bulk())
		require.NotNil(t, reg.Bulk())
	})

	t.Run("getJobQueue_falls_back_without_app_config", func(t *testing.T) {
		reg := &Registry{logger: logger, config: &ServiceConfig{JWTSecret: strings.Repeat("x", 32)}}
		jobQueue := reg.getJobQueue()
		_, ok := jobQueue.(*simpleJobQueue)
		assert.True(t, ok)
	})

	t.Run("service accessors warn on missing repos", func(t *testing.T) {
		reg, err := NewRegistry(WithStorage(newMockStorage()), WithLogger(logger), WithConfig(&ServiceConfig{
			BaseURL:   "https://example.com",
			JWTSecret: strings.Repeat("x", 32),
			Config:    &pkgconfig.Config{InstanceMode: pkgconfig.InstanceModeHybrid},
		}))
		require.NoError(t, err)

		assert.Nil(t, reg.Threads())
		assert.Nil(t, reg.Severance())
		assert.Nil(t, reg.FederationGraph())
		assert.Nil(t, reg.Conversations())
		assert.Nil(t, reg.Media())
		assert.Nil(t, reg.Lists())
		assert.Nil(t, reg.Notifications())
		assert.Nil(t, reg.Emoji())
	})

	t.Run("media streaming initializers no-op when config incomplete", func(t *testing.T) {
		reg, err := NewRegistry(WithStorage(newMockStorage()), WithLogger(logger), WithConfig(&ServiceConfig{
			BaseURL:   "https://example.com",
			JWTSecret: strings.Repeat("x", 32),
			Config:    &pkgconfig.Config{},
		}))
		require.NoError(t, err)

		assert.Nil(t, reg.initializeTranscodingService(reg.config.Config))
		assert.Nil(t, reg.initializeManifestService(reg.config.Config))
		assert.Nil(t, reg.initializeCloudFrontService(reg.config.Config))
	})

	t.Run("readCloudFrontPrivateKey_errors_on_missing_file", func(t *testing.T) {
		reg, err := NewRegistry(WithStorage(newMockStorage()), WithLogger(logger), WithConfig(&ServiceConfig{
			BaseURL:   "https://example.com",
			JWTSecret: strings.Repeat("x", 32),
		}))
		require.NoError(t, err)

		_, err = reg.readCloudFrontPrivateKey("/does/not/exist.pem")
		assert.Error(t, err)
	})
}

func TestRegistry_Adapters_And_JobQueueImplementations(t *testing.T) {
	logger := zap.NewNop()

	t.Run("validateJWTSecret_and_isLowEntropy", func(t *testing.T) {
		assert.Error(t, validateJWTSecret("short"))
		assert.Error(t, validateJWTSecret(strings.Repeat("a", 32)))
		assert.Error(t, validateJWTSecret("default-please-change-"+strings.Repeat("x", 32)))
		assert.NoError(t, validateJWTSecret("g5hJkL9pQ2rS7tV3wX6yZ8aB1cD4eF0nM6pR3tV9"))

		assert.True(t, isLowEntropy(""))
		assert.True(t, isLowEntropy("aaaa"))
		assert.True(t, isLowEntropy("abcd"))
		assert.False(t, isLowEntropy("abca"))
	})

	t.Run("simpleJobQueue_methods_do_not_error", func(t *testing.T) {
		q := &simpleJobQueue{logger: logger}
		assert.NoError(t, q.QueueImportJob(context.Background(), ImportJobMessage{ImportID: "i1", Username: "alice", Type: "mastodon"}))
		assert.NoError(t, q.QueueExportJob(context.Background(), ExportJobMessage{ExportID: "e1", Username: "alice", Type: "mastodon"}))
		assert.NoError(t, q.QueueMediaJob(context.Background(), MediaJobMessage{JobID: "j1", MediaID: "m1", Username: "alice"}))
		assert.NoError(t, q.QueueScheduledJob(context.Background(), ScheduledJobMessage{ScheduledStatusID: "s1", Username: "alice", ScheduledAt: time.Unix(1, 0)}))
		assert.NoError(t, q.QueueActivityJob(context.Background(), ActivityJobMessage{ActivityID: "a1", ActorID: "actor", Priority: federationPriorityNormal}))
		assert.NoError(t, q.QueueDelayedJob(context.Background(), "queue", map[string]any{"x": true}, 3))
	})

	t.Run("mediaJobQueueAdapter_converts_messages", func(t *testing.T) {
		captured := make([]MediaJobMessage, 0, 1)
		jobQueue := &captureJobQueue{onMedia: func(msg MediaJobMessage) { captured = append(captured, msg) }}
		adapter := &mediaJobQueueAdapter{jobQueue: jobQueue}

		require.NoError(t, adapter.QueueMediaJob(context.Background(), media.JobMessage{JobID: "j1", MediaID: "m1", Username: "alice", Timestamp: 123}))
		require.Len(t, captured, 1)
		assert.Equal(t, MediaJobMessage{JobID: "j1", MediaID: "m1", Username: "alice", Timestamp: 123}, captured[0])
	})

	t.Run("federationServiceAdapter_falls_back_when_job_queue_nil", func(t *testing.T) {
		fed := &stubFederationService{}
		adapter := &federationServiceAdapter{federation: fed, jobQueue: nil}
		require.NoError(t, adapter.QueueActivity(context.Background(), &activitypub.Activity{
			BaseObject: activitypub.BaseObject{Type: "Create"},
			Actor:      "https://example.com/users/alice",
		}))
		assert.Equal(t, 1, fed.deliverFollowersCalls)
	})

	t.Run("federationServiceAdapter_queues_activity_job_when_job_queue_present", func(t *testing.T) {
		fed := &stubFederationService{}
		captured := make([]ActivityJobMessage, 0, 1)
		jobQueue := &captureJobQueue{onActivity: func(msg ActivityJobMessage) { captured = append(captured, msg) }}
		adapter := &federationServiceAdapter{federation: fed, jobQueue: jobQueue}

		activity := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{
				ID:   "https://example.com/activities/1",
				Type: "Delete",
				To:   []string{activitypub.PublicAddress, "https://example.com/users/bob"},
				CC:   []string{"https://example.com/users/alice/followers"},
			},
			Actor: "https://example.com/users/alice",
		}

		require.NoError(t, adapter.QueueActivity(context.Background(), activity))
		require.Len(t, captured, 1)
		assert.Equal(t, federationPriorityHigh, captured[0].Priority)
		assert.Contains(t, captured[0].Recipients, "https://example.com/users/bob")
	})

	t.Run("federationServiceAdapter_determineRecipients_and_priority", func(t *testing.T) {
		adapter := &federationServiceAdapter{federation: &stubFederationService{}}

		recipients, err := adapter.determineRecipients(context.Background(), &activitypub.Activity{
			BaseObject: activitypub.BaseObject{
				To: []string{activitypub.PublicAddress},
				CC: []string{"https://example.com/users/alice/followers"},
			},
		})
		require.NoError(t, err)
		assert.Empty(t, recipients)

		assert.Equal(t, federationPriorityHigh, adapter.determinePriority(&activitypub.Activity{BaseObject: activitypub.BaseObject{Type: "Undo"}}))
		assert.Equal(t, federationPriorityHigh, adapter.determinePriority(&activitypub.Activity{BaseObject: activitypub.BaseObject{Type: "Follow"}}))
		assert.Equal(t, federationPriorityNormal, adapter.determinePriority(&activitypub.Activity{BaseObject: activitypub.BaseObject{Type: "Like"}}))
		assert.Equal(t, federationPriorityNormal, adapter.determinePriority(&activitypub.Activity{BaseObject: activitypub.BaseObject{Type: "Create"}}))
		assert.Equal(t, federationPriorityHigh, adapter.determinePriority(&activitypub.Activity{BaseObject: activitypub.BaseObject{Type: "Flag"}}))
		assert.Equal(t, federationPriorityNormal, adapter.determinePriority(&activitypub.Activity{BaseObject: activitypub.BaseObject{Type: "Other"}}))
	})

	t.Run("severance adapters handle empty and nil", func(t *testing.T) {
		fedAdapter := &severanceFederationAdapter{}
		_, err := fedAdapter.CheckInstanceReachability(context.Background(), "")
		assert.Error(t, err)

		ok, err := fedAdapter.CheckInstanceReachability(context.Background(), "remote.example")
		assert.NoError(t, err)
		assert.True(t, ok)

		notifAdapter := &severanceNotificationAdapter{}
		assert.Error(t, notifAdapter.SendSeveranceNotification(context.Background(), "", "s1", models.SeveranceReasonOther))
		assert.NoError(t, notifAdapter.SendSeveranceNotification(context.Background(), "alice", "s1", models.SeveranceReasonOther))
		assert.NoError(t, notifAdapter.NotifySeverance(context.Background(), "alice", "s1"))

		notifAdapter2 := &severanceNotificationAdapter{notification: noopNotificationService{}}
		assert.NoError(t, notifAdapter2.SendSeveranceNotification(context.Background(), "alice", "s1", models.SeveranceReasonOther))
	})

	t.Run("queueFederationAdapter stubs", func(t *testing.T) {
		adapter := &queueFederationAdapter{logger: logger}
		_, err := adapter.FetchObject(context.Background(), "https://remote.example/object/1", nil)
		assert.Error(t, err)

		assert.NoError(t, adapter.QueueActivity(context.Background(), &activitypub.Activity{Actor: "https://example.com/users/alice"}))
		assert.Equal(t, "alice", adapter.extractUsernameFromActorURI("https://example.com/users/alice"))
		assert.Equal(t, "alice", adapter.extractUsernameFromActorURI("https://example.com/@alice"))
		assert.Equal(t, "alice", adapter.extractUsernameFromActorURI("https://example.com/actor/alice"))
		assert.Equal(t, "", adapter.extractUsernameFromActorURI("not-a-url"))
		assert.Equal(t, "", adapter.extractUsernameFromActorURI("https://example.com/users/"+strings.Repeat("a", 101)))

		assert.NotNil(t, adapter.convertStorageActorToActivityPub(struct{}{}))
	})

	t.Run("queueFederationAdapter_delivers_with_fallback_actor", func(t *testing.T) {
		fed := &stubFederationService{}
		storage := newPermissiveRegistryStorage(t, "example.com", logger)
		adapter := &queueFederationAdapter{
			federation: fed,
			storage:    storage,
			logger:     logger,
		}

		activity := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{Type: "Create"},
			Actor:      "https://example.com/users/alice",
		}
		require.NoError(t, adapter.QueueActivity(context.Background(), activity))

		assert.Equal(t, 1, fed.deliverFollowersCalls)
		require.NotNil(t, fed.lastActor)
		assert.Equal(t, activity.Actor, fed.lastActor.ID)

		require.NoError(t, adapter.QueueActivity(context.Background(), &activitypub.Activity{BaseObject: activitypub.BaseObject{Type: "Create"}}))
		assert.Equal(t, 2, fed.deliverFollowersCalls)
	})

	t.Run("simpleFederationService_queue_activity_noop", func(t *testing.T) {
		svc := &simpleFederationService{logger: logger}
		assert.NoError(t, svc.QueueActivity(context.Background(), &activitypub.Activity{
			BaseObject: activitypub.BaseObject{Type: "Create"},
			Actor:      "actor",
		}))
	})
}

type captureJobQueue struct {
	onActivity func(ActivityJobMessage)
	onMedia    func(MediaJobMessage)
}

func (q *captureJobQueue) QueueImportJob(context.Context, ImportJobMessage) error { return nil }
func (q *captureJobQueue) QueueExportJob(context.Context, ExportJobMessage) error { return nil }
func (q *captureJobQueue) QueueScheduledJob(context.Context, ScheduledJobMessage) error {
	return nil
}
func (q *captureJobQueue) QueueDelayedJob(context.Context, string, interface{}, int32) error {
	return nil
}
func (q *captureJobQueue) QueueMediaJob(_ context.Context, msg MediaJobMessage) error {
	if q.onMedia != nil {
		q.onMedia(msg)
	}
	return nil
}
func (q *captureJobQueue) QueueActivityJob(_ context.Context, msg ActivityJobMessage) error {
	if q.onActivity != nil {
		q.onActivity(msg)
	}
	return nil
}

type stubFederationService struct {
	deliverFollowersCalls int
	lastActivity          *activitypub.Activity
	lastActor             *activitypub.Actor
}

func (s *stubFederationService) DeliverToFollowers(_ context.Context, activity *activitypub.Activity, actor *activitypub.Actor) error {
	s.deliverFollowersCalls++
	s.lastActivity = activity
	s.lastActor = actor
	return nil
}
func (s *stubFederationService) DeliverToRecipients(context.Context, *activitypub.Activity, *activitypub.Actor) error {
	return nil
}
func (s *stubFederationService) DetermineRecipients(context.Context, *activitypub.Activity, string) ([]string, error) {
	return nil, nil
}
func (s *stubFederationService) GetInstanceRelationships(context.Context, string) (*model.InstanceRelations, error) {
	return nil, nil
}

type noopNotificationService struct{}

func (noopNotificationService) CreateFollowNotification(context.Context, *activitypub.Activity) error {
	return nil
}
func (noopNotificationService) CreateLikeNotification(context.Context, *activitypub.Activity) error {
	return nil
}
func (noopNotificationService) CreateReblogNotification(context.Context, *activitypub.Activity) error {
	return nil
}
func (noopNotificationService) CreateReplyNotification(context.Context, *activitypub.Activity) error {
	return nil
}
func (noopNotificationService) CreateMentionNotification(context.Context, []string, *activitypub.Activity) error {
	return nil
}

func mustGenerateRSAPrivateKeyPEM(t *testing.T) string {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	der := x509.MarshalPKCS1PrivateKey(key)
	return string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: der}))
}

func newPermissiveDynamormDB(t *testing.T) dynamormcore.DB {
	t.Helper()

	db := new(dynamormMocks.MockDB)
	q := new(dynamormMocks.MockQuery)

	db.On("WithContext", mock.Anything).Return(db).Maybe()
	db.On("Model", mock.Anything).Return(q).Maybe()
	db.On("Transaction", mock.Anything).Return(nil).Maybe()
	db.On("Migrate").Return(nil).Maybe()
	db.On("AutoMigrate", mock.Anything).Return(nil).Maybe()
	db.On("Close").Return(nil).Maybe()

	queryType := reflect.TypeOf((*dynamormcore.Query)(nil)).Elem()
	updateBuilderType := reflect.TypeOf((*dynamormcore.UpdateBuilder)(nil)).Elem()
	batchGetBuilderType := reflect.TypeOf((*dynamormcore.BatchGetBuilder)(nil)).Elem()

	for i := 0; i < queryType.NumMethod(); i++ {
		method := queryType.Method(i)

		// Some query methods accept a destination pointer and fill it.
		switch method.Name {
		case "All", "Scan", "BatchGet", "ScanAllSegments":
			args := make([]any, method.Type.NumIn())
			for j := 0; j < len(args); j++ {
				args[j] = mock.Anything
			}
			q.On(method.Name, args...).Run(func(arguments mock.Arguments) {
				for _, arg := range arguments {
					fillSlicePointer(arg)
				}
			}).Return(nil).Maybe()
			continue
		case "First":
			args := make([]any, method.Type.NumIn())
			for j := 0; j < len(args); j++ {
				args[j] = mock.Anything
			}
			q.On("First", args...).Return(nil).Maybe()
			continue
		}

		args := make([]any, method.Type.NumIn())
		for j := 0; j < len(args); j++ {
			args[j] = mock.Anything
		}

		switch method.Type.NumOut() {
		case 0:
			q.On(method.Name, args...).Return().Maybe()
		case 1:
			out0 := method.Type.Out(0)
			switch {
			case out0.Implements(queryType) || out0.AssignableTo(queryType):
				q.On(method.Name, args...).Return(q).Maybe()
			case out0.Implements(updateBuilderType) || out0.AssignableTo(updateBuilderType):
				q.On(method.Name, args...).Return(new(dynamormMocks.MockUpdateBuilder)).Maybe()
			case out0.Implements(batchGetBuilderType) || out0.AssignableTo(batchGetBuilderType):
				q.On(method.Name, args...).Return(new(dynamormMocks.MockBatchGetBuilder)).Maybe()
			case out0.Kind() == reflect.Int || out0.Kind() == reflect.Int64:
				q.On(method.Name, args...).Return(0).Maybe()
			case out0.Kind() == reflect.Bool:
				q.On(method.Name, args...).Return(false).Maybe()
			default:
				q.On(method.Name, args...).Return(reflect.Zero(out0).Interface()).Maybe()
			}
		case 2:
			q.On(method.Name, args...).Return(
				reflect.Zero(method.Type.Out(0)).Interface(),
				reflect.Zero(method.Type.Out(1)).Interface(),
			).Maybe()
		default:
			zero := make([]any, method.Type.NumOut())
			for j := range zero {
				zero[j] = reflect.Zero(method.Type.Out(j)).Interface()
			}
			q.On(method.Name, args...).Return(zero...).Maybe()
		}
	}

	return db
}

func fillSlicePointer(dest any) {
	ptr := reflect.ValueOf(dest)
	if ptr.Kind() != reflect.Ptr || ptr.IsNil() {
		return
	}
	elem := ptr.Elem()
	if elem.Kind() != reflect.Slice {
		return
	}
	if elem.Len() == 0 {
		elem.Set(reflect.MakeSlice(elem.Type(), 1, 1))
	}
}

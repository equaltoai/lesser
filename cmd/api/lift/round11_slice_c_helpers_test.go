package lift

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/graph"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/mastodon"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/golang-jwt/jwt/v5"
)

type round11TestRepos struct {
	*MockRepositoryStorage
	account      *repositories.AccountRepository
	actor        interfaces.ActorRepository
	moderation   *repositories.ModerationRepository
	push         *repositories.PushSubscriptionRepository
	importRepo   *repositories.ImportRepository
	cost         *repositories.TrackingRepository
	analytics    *repositories.TrendingRepository
	instance     *repositories.InstanceRepository
	metricRecord *repositories.MetricRecordRepository
	status       *repositories.StatusRepository
	like         *repositories.LikeRepository
	social       *repositories.SocialRepository
	bookmark     *repositories.BookmarkRepository
	conversation *repositories.ConversationRepository
	trust        interfaces.TrustRepository
}

func (r *round11TestRepos) Account() *repositories.AccountRepository { return r.account }
func (r *round11TestRepos) Actor() interfaces.ActorRepository        { return r.actor }
func (r *round11TestRepos) Moderation() *repositories.ModerationRepository {
	return r.moderation
}
func (r *round11TestRepos) PushSubscription() *repositories.PushSubscriptionRepository {
	return r.push
}
func (r *round11TestRepos) Import() *repositories.ImportRepository { return r.importRepo }
func (r *round11TestRepos) Cost() *repositories.TrackingRepository { return r.cost }
func (r *round11TestRepos) Analytics() *repositories.TrendingRepository {
	return r.analytics
}
func (r *round11TestRepos) Instance() *repositories.InstanceRepository { return r.instance }
func (r *round11TestRepos) MetricRecord() *repositories.MetricRecordRepository {
	return r.metricRecord
}
func (r *round11TestRepos) Status() interfaces.StatusRepository { return r.status }
func (r *round11TestRepos) Like() *repositories.LikeRepository     { return r.like }
func (r *round11TestRepos) Social() *repositories.SocialRepository { return r.social }
func (r *round11TestRepos) Bookmark() *repositories.BookmarkRepository {
	return r.bookmark
}
func (r *round11TestRepos) Conversation() *repositories.ConversationRepository {
	return r.conversation
}
func (r *round11TestRepos) Trust() interfaces.TrustRepository { return r.trust }

func round11NewHandlerSliceC(t *testing.T, state *round10QueryState) (*Handler, *round10DynamoHarness, *round11TestRepos) {
	t.Helper()

	if state == nil {
		state = &round10QueryState{}
	}

	logger := round10TestLogger(t)
	cfg := round10TestConfig()
	cfg.Region = "us-east-1"
	cfg.Domain = "example.com"
	cfg.DynamoTableName = "test-table"
	cfg.JWTSecret = round11StrongJWTSecret

	harness := round10NewDynamoHarness(t, state)

	repos := &round11TestRepos{
		MockRepositoryStorage: &MockRepositoryStorage{},
		account:               repositories.NewAccountRepository(harness.db, cfg.DynamoTableName, cfg.Domain, logger),
		actor:                 repositories.NewActorRepository(harness.db, cfg.DynamoTableName, logger),
		moderation:            repositories.NewModerationRepository(harness.db, cfg.DynamoTableName, logger),
		push:                  repositories.NewPushSubscriptionRepository(harness.db, cfg.DynamoTableName, logger, nil, nil, "", "mailto:push@example.com"),
		importRepo:            repositories.NewImportRepository(harness.db, cfg.DynamoTableName, logger),
		cost:                  repositories.NewTrackingRepository(harness.db, cfg.DynamoTableName, logger, nil),
		analytics:             repositories.NewTrendingRepository(harness.db, logger, nil),
		instance:              repositories.NewInstanceRepository(harness.db, cfg.DynamoTableName, logger),
		metricRecord:          repositories.NewMetricRecordRepository(harness.db, cfg.DynamoTableName, logger, nil),
		status:                repositories.NewStatusRepository(harness.db, cfg.DynamoTableName, logger, nil),
		like:                  repositories.NewLikeRepository(harness.db, cfg.DynamoTableName, logger),
		social:                repositories.NewSocialRepository(harness.db, cfg.DynamoTableName, logger, nil),
		bookmark:              repositories.NewBookmarkRepository(harness.db, cfg.DynamoTableName, logger),
		conversation:          repositories.NewConversationRepository(harness.db, cfg.DynamoTableName, logger, nil),
		trust:                 repositories.NewTrustRepository(harness.db, cfg.DynamoTableName, logger, nil),
	}

	h := &Handler{
		cfg:       cfg,
		repos:     repos,
		logger:    logger,
		converter: stubConverter{},
		registry:  &RegistryStub{},
		loaders:   graph.NewLoaders(repos, logger),
	}

	return h, harness, repos
}

type stubConverter struct{}

func (stubConverter) ActorToAccount(actor *activitypub.Actor) apimodels.Account {
	return apimodels.Account{ID: actor.ID}
}

func (stubConverter) ActorToAccountWithCounts(actor *activitypub.Actor, followers, following, statuses int) apimodels.Account {
	return apimodels.Account{ID: actor.ID}
}

func (stubConverter) ActorToAccountWithMetadata(actor *activitypub.Actor, _ *storage.ActorMetadata, followers, following, statuses int) apimodels.Account {
	return apimodels.Account{ID: actor.ID}
}

func (stubConverter) ObjectToStatus(_ any, _ *activitypub.Actor) apimodels.Status {
	return apimodels.Status{}
}

func (stubConverter) ObjectToStatusWithContext(_ context.Context, _ any, _ *activitypub.Actor, likeCount, reblogCount int, favorited, reblogged, bookmarked bool) apimodels.Status {
	return apimodels.Status{}
}

func (stubConverter) ConversationToAPI(_ *storagemodels.Conversation, _ []*activitypub.Actor, _ any, _ bool) apimodels.Conversation {
	return apimodels.Conversation{}
}

func (stubConverter) ConvertFilterToMastodon(filter *storage.Filter, keywords []*storage.FilterKeyword, statuses []*storage.FilterStatus) *mastodon.Filter {
	result := &mastodon.Filter{
		ID:           filter.ID,
		Title:        filter.Title,
		Context:      filter.Context,
		FilterAction: filter.FilterAction,
	}
	for _, keyword := range keywords {
		result.Keywords = append(result.Keywords, mastodon.FilterKeyword{
			ID:        keyword.ID,
			Keyword:   keyword.Keyword,
			WholeWord: keyword.WholeWord,
		})
	}
	for _, status := range statuses {
		result.Statuses = append(result.Statuses, mastodon.FilterStatus{
			ID:       status.ID,
			StatusID: status.StatusID,
		})
	}
	return result
}

func (stubConverter) ConvertFilterKeywordToV1(keyword *storage.FilterKeyword, filter *storage.Filter) *mastodon.V1Filter {
	return &mastodon.V1Filter{ID: filter.ID, Phrase: keyword.Keyword, Context: filter.Context}
}

func (stubConverter) ConvertMuteToRelationship(_ *apimodels.Relationship, _ *storage.Mute) {}

func (stubConverter) NotesToStatus(_ any) apimodels.Status {
	return apimodels.Status{}
}

func (stubConverter) PollToAPI(_ *storage.Poll, _ []int) apimodels.Poll {
	return apimodels.Poll{}
}

func (stubConverter) ExtractUsernameFromActorID(actorID string) string { return actorID }

func (stubConverter) ExtractIDFromURL(url string) string { return url }

func round11SignToken(t *testing.T, secret, username string, scopes []string, sessionID string) string {
	return round11SignTokenWithClientID(t, secret, username, "test-client", scopes, sessionID)
}

func round11SignTokenWithClientID(t *testing.T, secret, username, clientID string, scopes []string, sessionID string) string {
	t.Helper()

	now := time.Now()
	claims := auth.Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   username,
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(1 * time.Hour)),
		},
		Username:  username,
		ClientID:  clientID,
		Scopes:    scopes,
		SessionID: sessionID,
		DeviceID:  "device-1",
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("failed to sign token: %v", err)
	}
	return signed
}

func round11JSONBody(t *testing.T, body any) []byte {
	t.Helper()

	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("failed to marshal body: %v", err)
	}
	return payload
}

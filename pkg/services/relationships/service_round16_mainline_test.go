package relationships

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/core"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/equaltoai/lesser/pkg/streaming"
	"github.com/equaltoai/lesser/pkg/testing/inmemory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	dynamormCore "github.com/theory-cloud/tabletheory/v3/pkg/core"
	dynamormerrors "github.com/theory-cloud/tabletheory/v3/pkg/errors"
	"github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	"go.uber.org/zap"
)

type transactionalMockDB struct {
	*mocks.MockDB
}

func (db transactionalMockDB) TransactWrite(ctx context.Context, fn func(dynamormCore.TransactionBuilder) error) error {
	if fn == nil {
		return nil
	}
	_ = fn(noopTransactionBuilder{})
	return nil
}

type noopTransactionBuilder struct{}

func (noopTransactionBuilder) Put(_ any, _ ...dynamormCore.TransactCondition) dynamormCore.TransactionBuilder {
	return noopTransactionBuilder{}
}
func (noopTransactionBuilder) Create(_ any, _ ...dynamormCore.TransactCondition) dynamormCore.TransactionBuilder {
	return noopTransactionBuilder{}
}
func (noopTransactionBuilder) Update(_ any, _ []string, _ ...dynamormCore.TransactCondition) dynamormCore.TransactionBuilder {
	return noopTransactionBuilder{}
}
func (noopTransactionBuilder) UpdateWithBuilder(_ any, _ func(dynamormCore.UpdateBuilder) error, _ ...dynamormCore.TransactCondition) dynamormCore.TransactionBuilder {
	return noopTransactionBuilder{}
}
func (noopTransactionBuilder) Delete(_ any, _ ...dynamormCore.TransactCondition) dynamormCore.TransactionBuilder {
	return noopTransactionBuilder{}
}
func (noopTransactionBuilder) ConditionCheck(_ any, _ ...dynamormCore.TransactCondition) dynamormCore.TransactionBuilder {
	return noopTransactionBuilder{}
}
func (noopTransactionBuilder) WithContext(_ context.Context) dynamormCore.TransactionBuilder {
	return noopTransactionBuilder{}
}
func (noopTransactionBuilder) Execute() error                             { return nil }
func (noopTransactionBuilder) ExecuteWithContext(_ context.Context) error { return nil }

type permissiveQueryState struct {
	lastPK string
	lastSK string
}

func setupPermissiveDynamormMocks(mockDB *mocks.MockDB, mockQuery *mocks.MockQuery, mockUpdateBuilder *mocks.MockUpdateBuilder, state *permissiveQueryState) {
	mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
	mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()

	mockQuery.On("Index", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		field, _ := args.Get(0).(string)
		value, _ := args.Get(2).(string)
		fieldLower := strings.ToLower(field)
		if strings.HasSuffix(fieldLower, "pk") {
			state.lastPK = value
		}
		if strings.HasSuffix(fieldLower, "sk") {
			state.lastSK = value
		}
	}).Return(mockQuery).Maybe()
	mockQuery.On("Filter", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Limit", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Cursor", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("ConsistentRead").Return(mockQuery).Maybe()
	mockQuery.On("Select", mock.Anything).Return(mockQuery).Maybe()

	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		populateSlice(args.Get(0), state)
	}).Return(nil).Maybe()
	mockQuery.On("Scan", mock.Anything).Run(func(args mock.Arguments) {
		populateSlice(args.Get(0), state)
	}).Return(nil).Maybe()
	mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
		populateStruct(args.Get(0), state)
	}).Return(nil).Maybe()

	mockQuery.On("Create").Return(nil).Maybe()
	mockQuery.On("Update").Return(nil).Maybe()
	mockQuery.On("Update", mock.Anything).Return(nil).Maybe()
	mockQuery.On("Delete").Return(nil).Maybe()
	mockQuery.On("Delete", mock.Anything).Return(nil).Maybe()
	mockQuery.On("Count").Return(int64(2), nil).Maybe()
	mockQuery.On("BatchGet", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		populateSlice(args.Get(1), state)
	}).Return(nil).Maybe()

	mockQuery.On("UpdateBuilder").Return(mockUpdateBuilder).Maybe()
	mockUpdateBuilder.On("Set", mock.Anything, mock.Anything).Return(mockUpdateBuilder).Maybe()
	mockUpdateBuilder.On("SetIfNotExists", mock.Anything, mock.Anything, mock.Anything).Return(mockUpdateBuilder).Maybe()
	mockUpdateBuilder.On("Remove", mock.Anything).Return(mockUpdateBuilder).Maybe()
	mockUpdateBuilder.On("Add", mock.Anything, mock.Anything).Return(mockUpdateBuilder).Maybe()
	mockUpdateBuilder.On("Condition", mock.Anything, mock.Anything, mock.Anything).Return(mockUpdateBuilder).Maybe()
	mockUpdateBuilder.On("Execute").Return(nil).Maybe()
}

func populateSlice(target any, state *permissiveQueryState) {
	v := reflect.ValueOf(target)
	if v.Kind() != reflect.Ptr || v.Elem().Kind() != reflect.Slice {
		return
	}

	sliceValue := v.Elem()
	elemType := sliceValue.Type().Elem()
	baseElemType := elemType
	if baseElemType.Kind() == reflect.Ptr {
		baseElemType = baseElemType.Elem()
	}
	if baseElemType.Kind() == reflect.Interface {
		return
	}

	for i := range 2 {
		at := time.Date(2025, 1, i+1, 0, 0, 0, 0, time.UTC)

		var element reflect.Value
		if elemType.Kind() == reflect.Ptr {
			element = reflect.New(baseElemType)
			populateStructWithTime(element.Interface(), state, at)
		} else {
			ptr := reflect.New(baseElemType)
			populateStructWithTime(ptr.Interface(), state, at)
			element = ptr.Elem()
		}

		sliceValue = reflect.Append(sliceValue, element)
	}

	v.Elem().Set(sliceValue)
}

func populateStruct(target any, state *permissiveQueryState) {
	populateStructWithTime(target, state, time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
}

func populateStructWithTime(target any, state *permissiveQueryState, at time.Time) {
	switch model := target.(type) {
	case *models.RelationshipRecord:
		pk := state.lastPK
		if !strings.HasPrefix(pk, "FOLLOW#") {
			pk = "FOLLOW#alice"
		}
		sk := state.lastSK
		if !strings.HasPrefix(sk, "FOLLOWING#") {
			if at.Day()%2 == 0 {
				sk = "FOLLOWING#erin"
			} else {
				sk = "FOLLOWING#carol"
			}
		}

		model.PK = pk
		model.SK = sk

		model.GSI1PK = "FOLLOW#alice"
		if at.Day()%2 == 0 {
			model.GSI1SK = "FOLLOWER#dave"
		} else {
			model.GSI1SK = "FOLLOWER#bob"
		}
		model.State = models.RelationshipAccepted
		model.Notifying = true
		model.ShowingReblogs = true
		model.Languages = []string{"en"}
		model.Note = "note"
		model.CreatedAt = at
		model.UpdatedAt = at
	case *models.Actor:
		username := strings.TrimPrefix(state.lastPK, "ACTOR#")
		if username == "" {
			username = "bob"
		}
		model.Username = username
		model.Actor = &activitypub.Actor{
			BaseObject: activitypub.BaseObject{
				ID:   fmt.Sprintf("https://remote.social/users/%s", username),
				Type: "Person",
			},
			PreferredUsername: username,
			URL:               fmt.Sprintf("https://remote.social/@%s", username),
		}
		model.CreatedAt = at
		model.UpdatedAt = at
	case *models.User:
		username := strings.TrimPrefix(state.lastPK, "USER#")
		if username == "" {
			username = "bob"
		}
		model.Username = username
		model.DisplayName = "Stored User"
		model.Note = "stored note"
		model.Avatar = "https://example.com/avatar.png"
		model.Header = "https://example.com/header.png"
		model.CreatedAt = at
		model.UpdatedAt = at
	case *models.UserDomainBlock:
		model.Username = strings.TrimPrefix(state.lastPK, "USER#")
		if model.Username == "" {
			model.Username = "alice"
		}
		model.Domain = fmt.Sprintf("blocked-%d.example", at.Day())
		_ = model.UpdateKeys()
	case *models.Mute:
		model.Actor = "alice"
		model.Object = "bob"
		model.ID = fmt.Sprintf("mute-%d", at.Day())
		model.Published = at
		model.CreatedAt = at
		_ = model.BeforeCreate()
	case *models.Block:
		model.Actor = "alice"
		model.Object = "https://example.com/users/bob"
		model.ID = fmt.Sprintf("block-%d", at.Day())
		model.Published = at
		model.CreatedAt = at
		_ = model.BeforeCreate()
	}
}

type testRepositoryStorage struct {
	db        dynamormCore.DB
	tableName string
	logger    *zap.Logger

	actorRepo        interfaces.ActorRepository
	activityRepo     interfaces.ActivityRepository
	domainBlockRepo  *repositories.DomainBlockRepository
	relationshipRepo interfaces.ConcreteRelationshipRepository
	socialRepo       *repositories.SocialRepository
	userRepo         interfaces.UserRepository
}

func (s *testRepositoryStorage) GetDB() dynamormCore.DB { return s.db }
func (s *testRepositoryStorage) GetTableName() string   { return s.tableName }
func (s *testRepositoryStorage) GetLogger() *zap.Logger { return s.logger }

func (s *testRepositoryStorage) Actor() interfaces.ActorRepository { return s.actorRepo }
func (s *testRepositoryStorage) DomainBlock() *repositories.DomainBlockRepository {
	return s.domainBlockRepo
}
func (s *testRepositoryStorage) Relationship() interfaces.ConcreteRelationshipRepository {
	return s.relationshipRepo
}
func (s *testRepositoryStorage) Social() *repositories.SocialRepository { return s.socialRepo }
func (s *testRepositoryStorage) User() interfaces.UserRepository        { return s.userRepo }

// Repository access methods - return nil repositories unless a test needs them.
func (s *testRepositoryStorage) Account() *repositories.AccountRepository             { return nil }
func (s *testRepositoryStorage) Bookmark() *repositories.BookmarkRepository           { return nil }
func (s *testRepositoryStorage) Object() interfaces.ObjectRepository                  { return nil }
func (s *testRepositoryStorage) Activity() interfaces.ActivityRepository              { return s.activityRepo }
func (s *testRepositoryStorage) Timeline() interfaces.TimelineRepository              { return nil }
func (s *testRepositoryStorage) Notification() interfaces.NotificationRepository      { return nil }
func (s *testRepositoryStorage) Like() *repositories.LikeRepository                   { return nil }
func (s *testRepositoryStorage) Moderation() interfaces.ModerationRepository          { return nil }
func (s *testRepositoryStorage) List() *repositories.ListRepository                   { return nil }
func (s *testRepositoryStorage) Media() *repositories.MediaRepository                 { return nil }
func (s *testRepositoryStorage) MediaMetadata() *repositories.MediaMetadataRepository { return nil }
func (s *testRepositoryStorage) Poll() *repositories.PollRepository                   { return nil }
func (s *testRepositoryStorage) PushSubscription() *repositories.PushSubscriptionRepository {
	return nil
}
func (s *testRepositoryStorage) Hashtag() *repositories.HashtagRepository { return nil }
func (s *testRepositoryStorage) ScheduledStatus() *repositories.ScheduledStatusRepository {
	return nil
}
func (s *testRepositoryStorage) Announcement() *repositories.AnnouncementRepository { return nil }
func (s *testRepositoryStorage) Instance() *repositories.InstanceRepository         { return nil }
func (s *testRepositoryStorage) Federation() *repositories.FederationRepository     { return nil }
func (s *testRepositoryStorage) Recovery() *repositories.RecoveryRepository         { return nil }
func (s *testRepositoryStorage) Analytics() *repositories.TrendingRepository        { return nil }
func (s *testRepositoryStorage) Status() interfaces.StatusRepository                { return nil }
func (s *testRepositoryStorage) Cost() *repositories.TrackingRepository             { return nil }
func (s *testRepositoryStorage) WebSocketCost() *repositories.WebSocketCostRepository {
	return nil
}
func (s *testRepositoryStorage) Trust() interfaces.TrustRepository { return nil }
func (s *testRepositoryStorage) Search() *repositories.SearchRepository {
	return nil
}
func (s *testRepositoryStorage) Relay() *repositories.RelayRepository { return nil }
func (s *testRepositoryStorage) CommunityNote() *repositories.CommunityNoteRepository {
	return nil
}
func (s *testRepositoryStorage) Emoji() *repositories.EmojiRepository { return nil }
func (s *testRepositoryStorage) RateLimit() *repositories.RateLimitRepository {
	return nil
}
func (s *testRepositoryStorage) Conversation() *repositories.ConversationRepository { return nil }
func (s *testRepositoryStorage) Marker() *repositories.MarkerRepository             { return nil }
func (s *testRepositoryStorage) FeaturedTag() *repositories.FeaturedTagRepository   { return nil }
func (s *testRepositoryStorage) AI() *repositories.AIRepository                     { return nil }
func (s *testRepositoryStorage) Export() *repositories.ExportRepository             { return nil }
func (s *testRepositoryStorage) Import() *repositories.ImportRepository             { return nil }
func (s *testRepositoryStorage) DLQ() *repositories.DLQRepository                   { return nil }
func (s *testRepositoryStorage) MetricRecord() *repositories.MetricRecordRepository { return nil }
func (s *testRepositoryStorage) CloudWatchMetrics() *repositories.CloudWatchMetricsRepository {
	return nil
}
func (s *testRepositoryStorage) StreamingCloudWatch() *repositories.StreamingCloudWatchRepository {
	return nil
}
func (s *testRepositoryStorage) Audit() *repositories.AuditRepository       { return nil }
func (s *testRepositoryStorage) OAuth() *repositories.OAuthRepository       { return nil }
func (s *testRepositoryStorage) Skill() interfaces.SkillRepository          { return nil }
func (s *testRepositoryStorage) DNSCache() *repositories.DNSCacheRepository { return nil }
func (s *testRepositoryStorage) Filter() *repositories.FilterRepository     { return nil }
func (s *testRepositoryStorage) Thread() *repositories.ThreadRepository     { return nil }
func (s *testRepositoryStorage) Severance() *repositories.SeveranceRepository {
	return nil
}
func (s *testRepositoryStorage) ModerationML() *repositories.ModerationMLRepository {
	return nil
}
func (s *testRepositoryStorage) Quote() *repositories.QuoteRepository { return nil }
func (s *testRepositoryStorage) MediaAnalytics() interfaces.MediaAnalyticsRepository {
	return nil
}
func (s *testRepositoryStorage) MediaPopularity() interfaces.MediaPopularityRepository {
	return nil
}
func (s *testRepositoryStorage) MediaSession() interfaces.MediaSessionRepository {
	return nil
}
func (s *testRepositoryStorage) StreamingConnection() interfaces.StreamingConnectionRepository {
	return nil
}
func (s *testRepositoryStorage) Article() interfaces.ArticleRepository           { return nil }
func (s *testRepositoryStorage) Draft() interfaces.DraftRepository               { return nil }
func (s *testRepositoryStorage) UploadGrant() interfaces.UploadGrantRepository   { return nil }
func (s *testRepositoryStorage) PromoPackage() interfaces.PromoPackageRepository { return nil }
func (s *testRepositoryStorage) Revision() interfaces.RevisionRepository {
	return nil
}
func (s *testRepositoryStorage) Series() interfaces.SeriesRepository { return nil }
func (s *testRepositoryStorage) Category() interfaces.CategoryRepository {
	return nil
}
func (s *testRepositoryStorage) Publication() interfaces.PublicationRepository {
	return nil
}
func (s *testRepositoryStorage) PublicationMember() interfaces.PublicationMemberRepository {
	return nil
}

func newServiceWithStorageHarness(t *testing.T) (*Service, *testRepositoryStorage) {
	t.Helper()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	mockUpdateBuilder := new(mocks.MockUpdateBuilder)
	state := &permissiveQueryState{}
	setupPermissiveDynamormMocks(mockDB, mockQuery, mockUpdateBuilder, state)

	db := transactionalMockDB{MockDB: mockDB}
	logger := zap.NewNop()
	tableName := "test-table"

	storageHarness := &testRepositoryStorage{
		db:               db,
		tableName:        tableName,
		logger:           logger,
		actorRepo:        repositories.NewActorRepository(db, tableName, logger),
		activityRepo:     inmemory.NewActivityRepository(),
		domainBlockRepo:  repositories.NewDomainBlockRepository(db, tableName, logger),
		relationshipRepo: repositories.NewRelationshipRepository(db, tableName, logger),
		socialRepo:       repositories.NewSocialRepository(db, tableName, logger, nil),
		userRepo:         repositories.NewUserRepository(db, tableName, logger),
	}

	service := NewServiceWithStorage(storageHarness, streaming.NewMockPublisher(), nil, nil, "example.com")
	return service, storageHarness
}

func newServiceWithStorageHarnessConfigured(
	t *testing.T,
	configure func(db *mocks.MockDB, q *mocks.MockQuery, ub *mocks.MockUpdateBuilder, state *permissiveQueryState),
) (*Service, *testRepositoryStorage) {
	t.Helper()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	mockUpdateBuilder := new(mocks.MockUpdateBuilder)
	state := &permissiveQueryState{}

	if configure != nil {
		configure(mockDB, mockQuery, mockUpdateBuilder, state)
	}
	// Defaults go last so any overrides match first.
	setupPermissiveDynamormMocks(mockDB, mockQuery, mockUpdateBuilder, state)

	db := transactionalMockDB{MockDB: mockDB}
	logger := zap.NewNop()
	tableName := "test-table"

	storageHarness := &testRepositoryStorage{
		db:               db,
		tableName:        tableName,
		logger:           logger,
		actorRepo:        repositories.NewActorRepository(db, tableName, logger),
		activityRepo:     inmemory.NewActivityRepository(),
		domainBlockRepo:  repositories.NewDomainBlockRepository(db, tableName, logger),
		relationshipRepo: repositories.NewRelationshipRepository(db, tableName, logger),
		socialRepo:       repositories.NewSocialRepository(db, tableName, logger, nil),
		userRepo:         repositories.NewUserRepository(db, tableName, logger),
	}

	service := NewServiceWithStorage(storageHarness, streaming.NewMockPublisher(), nil, nil, "example.com")
	return service, storageHarness
}

func TestStreamingEventEmitter_EmitEvents(t *testing.T) {
	ctx := context.Background()
	publisher := streaming.NewMockPublisher()

	emitter := &streamingEventEmitter{publisher: publisher}
	events := []*common.StreamingEvent{
		{
			Type:      "test.one",
			Timestamp: time.Now(),
			Metadata:  map[string]interface{}{"a": "b"},
		},
		{
			Type:      "test.two",
			Timestamp: time.Now(),
			Metadata:  map[string]interface{}{"c": "d"},
		},
	}

	require.NoError(t, emitter.EmitEvents(ctx, events))

	published := publisher.(interface {
		GetPublishedEventsForStream(streamName string) []streaming.MockPublishedEvent
	})
	assert.Len(t, published.GetPublishedEventsForStream("user"), 2)

	if configurable, ok := publisher.(interface {
		SetError(shouldError bool, message string)
	}); ok {
		configurable.SetError(true, "boom")
	}
	assert.Error(t, emitter.EmitEvents(ctx, events))
}

func TestService_NewServiceWithStorage_GetFollowAccounts_StoragePath(t *testing.T) {
	ctx := context.Background()
	service, storageHarness := newServiceWithStorageHarness(t)

	actorRepo := inmemory.NewActorRepository()
	storageHarness.actorRepo = actorRepo

	require.NoError(t, actorRepo.CreateActor(ctx, &activitypub.Actor{
		BaseObject: activitypub.BaseObject{
			ID:   "https://example.com/users/alice",
			Type: activitypub.PersonType,
		},
		PreferredUsername: "alice",
	}, ""))
	require.NoError(t, actorRepo.CreateActor(ctx, &activitypub.Actor{
		BaseObject: activitypub.BaseObject{
			ID:   "https://example.com/users/bob",
			Type: activitypub.PersonType,
		},
		PreferredUsername: "bob",
	}, ""))

	follower, following, err := service.getFollowAccounts(ctx, "alice", "bob")
	require.NoError(t, err)
	require.NotNil(t, follower)
	require.NotNil(t, following)
	require.NotNil(t, follower.Actor)
	require.NotNil(t, following.Actor)
	assert.Equal(t, follower.User.Username, follower.Actor.PreferredUsername)
	assert.Equal(t, following.User.Username, following.Actor.PreferredUsername)
}

func TestService_MuteUnmute_UsesStorageForCreateAndEvents(t *testing.T) {
	ctx := context.Background()
	service, _ := newServiceWithStorageHarness(t)

	// Use legacy repo for IsMuted/UnmuteUser checks so we can deterministically drive branches.
	statefulRepo := newStatefulRelationshipRepo()
	service.relationshipRepo = statefulRepo

	accountRepo := &MockAccountRepository{}
	accountRepo.On("GetAccount", ctx, "alice").Return(createTestAccount("alice", false), nil)
	accountRepo.On("GetAccount", ctx, "bob").Return(createTestAccount("bob", false), nil)
	service.accountRepo = accountRepo

	// Self-mute rejected.
	_, err := service.Mute(ctx, &MuteCommand{MuterID: "alice", MutedID: "alice"})
	assert.Error(t, err)

	duration := 15 * time.Minute
	res, err := service.Mute(ctx, &MuteCommand{MuterID: "alice", MutedID: "bob", Duration: &duration})
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.NotEmpty(t, res.Events)

	// Drive "already muted" branch.
	statefulRepo.setMuted("alice", "bob", true)
	res, err = service.Mute(ctx, &MuteCommand{MuterID: "alice", MutedID: "bob"})
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Empty(t, res.Events)

	// Not muted -> idempotent unmute.
	statefulRepo.setMuted("alice", "carol", false)
	res, err = service.Unmute(ctx, &UnmuteCommand{MuterID: "alice", MutedID: "carol"})
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Empty(t, res.Events)

	// Muted -> unmute events.
	statefulRepo.setMuted("alice", "bob", true)
	res, err = service.Unmute(ctx, &UnmuteCommand{MuterID: "alice", MutedID: "bob"})
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.NotEmpty(t, res.Events)
}

func TestService_UpdateRelationship_StorageBacked(t *testing.T) {
	ctx := context.Background()
	service, _ := newServiceWithStorageHarness(t)

	notify := true
	showReblogs := false
	languages := []string{"en", "es"}
	note := "hello"

	rel, err := service.UpdateRelationship(ctx, &UpdateRelationshipCommand{
		FollowerID:  "alice",
		FollowingID: "bob",
		Notify:      &notify,
		ShowReblogs: &showReblogs,
		Languages:   &languages,
		Note:        &note,
	})
	require.NoError(t, err)
	require.NotNil(t, rel)

	// No-op updates still returns a relationship (skips repository update).
	rel, err = service.UpdateRelationship(ctx, &UpdateRelationshipCommand{
		FollowerID:  "alice",
		FollowingID: "bob",
	})
	require.NoError(t, err)
	require.NotNil(t, rel)
}

func TestService_DomainBlocks_ReadWrite(t *testing.T) {
	ctx := context.Background()
	service, _ := newServiceWithStorageHarness(t)

	assert.Error(t, service.AddDomainBlock(ctx, &AddDomainBlockCommand{UserID: "alice"}))

	require.NoError(t, service.AddDomainBlock(ctx, &AddDomainBlockCommand{
		UserID: "alice",
		Domain: "blocked.example",
	}))
	require.NoError(t, service.RemoveDomainBlock(ctx, &RemoveDomainBlockCommand{
		UserID: "alice",
		Domain: "blocked.example",
	}))

	query := &GetDomainBlocksQuery{UserID: "alice"}
	result, err := service.GetDomainBlocks(ctx, query)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.GreaterOrEqual(t, query.Limit, 1)
}

func TestService_ReadOperations_WithStorage(t *testing.T) {
	ctx := context.Background()
	service, _ := newServiceWithStorageHarness(t)

	// GetMutedUsers uses Social + Actor repos.
	muted, err := service.GetMutedUsers(ctx, &GetMutedUsersQuery{UserID: "alice"})
	require.NoError(t, err)
	require.NotNil(t, muted)

	// GetBlockedUsers uses Relationship + Actor + User repos and exercises buildAccountFromActor.
	blocked, err := service.GetBlockedUsers(ctx, &GetBlockedUsersQuery{UserID: "alice"})
	require.NoError(t, err)
	require.NotNil(t, blocked)

	// Followers/following use Relationship + Actor repos.
	followers, _, err := service.GetFollowers(ctx, "alice", 10, "")
	require.NoError(t, err)
	following, _, err := service.GetFollowing(ctx, "alice", 10, "")
	require.NoError(t, err)
	assert.NotNil(t, followers)
	assert.NotNil(t, following)

	// Count helpers.
	_, err = service.CountFollowers(ctx, "alice")
	require.NoError(t, err)
	_, err = service.CountFollowing(ctx, "alice")
	require.NoError(t, err)

	// Relationship flags.
	_, err = service.IsBlocked(ctx, "alice", "bob")
	require.NoError(t, err)
	_, err = service.IsMuted(ctx, "alice", "bob")
	require.NoError(t, err)
}

func TestService_FollowRequests_AcceptReject(t *testing.T) {
	ctx := context.Background()
	service, _ := newServiceWithStorageHarness(t)

	// Wire federation service to hit queue paths.
	federation := &MockFederationService{}
	federation.On("QueueActivity", mock.Anything, mock.Anything).Return(nil).Maybe()
	service.federation = federation

	// Pending requests list.
	result, err := service.GetPendingFollowRequests(ctx, &GetFollowRequestsQuery{UserID: "alice"})
	require.NoError(t, err)
	require.NotNil(t, result)

	// Validation errors.
	_, err = service.AcceptFollowRequest(ctx, &AcceptFollowRequestCommand{RequesterID: "alice", FollowerID: "alice"})
	assert.Error(t, err)

	// Accept and reject.
	accepted, err := service.AcceptFollowRequest(ctx, &AcceptFollowRequestCommand{RequesterID: "alice", FollowerID: "bob"})
	require.NoError(t, err)
	require.NotNil(t, accepted)

	rejected, err := service.RejectFollowRequest(ctx, &RejectFollowRequestCommand{RequesterID: "alice", FollowerID: "bob"})
	require.NoError(t, err)
	require.NotNil(t, rejected)
}

type panicFederationService struct{}

func (panicFederationService) QueueActivity(context.Context, *activitypub.Activity) error {
	panic("boom")
}

func TestFederationQueueHelpers_HandleNilAndPanics(t *testing.T) {
	ctx := context.Background()
	service, _ := newServiceWithStorageHarness(t)
	service.domainName = "example.com"

	local := &storage.Account{
		User:  &storage.User{Username: "alice"},
		Actor: &activitypub.Actor{BaseObject: activitypub.BaseObject{ID: "https://remote.social/users/alice"}},
	}
	remote := &storage.Account{
		User:  &storage.User{Username: "bob"},
		Actor: &activitypub.Actor{BaseObject: activitypub.BaseObject{ID: "https://remote.social/users/bob"}},
	}

	// Nil service paths.
	service.federation = nil
	service.queueFederationBlock(ctx, local, remote)
	service.queueFederationUndo(ctx, local, remote, "Block")
	service.queueFederationReject(ctx, remote, local, "")

	// Typed nil should also be treated as nil.
	var typedNil *MockFederationService
	service.federation = typedNil
	service.queueFederationFollowDirectly(ctx, &activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			Context: activitypub.Context,
			Type:    activitypub.FollowType,
			ID:      "https://example.com/activities/test-follow",
			To:      []string{remote.Actor.ID},
		},
		Actor:  local.Actor.ID,
		Object: remote.Actor.ID,
	}, local, remote)

	// Panic in queue should be suppressed.
	service.federation = panicFederationService{}
	service.queueFederationBlock(ctx, local, remote)
	service.queueFederationUndo(ctx, local, remote, "Block")
	service.queueFederationReject(ctx, remote, local, "")
}

func TestService_RemoveRelationshipGeneric_StorageBranch(t *testing.T) {
	ctx := context.Background()
	service, _ := newServiceWithStorageHarness(t)

	// Ensure storage repo is used for existence/removal.
	service.relationshipRepo = nil

	// Cover follower path (emitUnfollowEvents + queueFederationUndo).
	service.federation = &MockFederationService{}
	service.federation.(*MockFederationService).On("QueueActivity", mock.Anything, mock.Anything).Return(nil).Maybe()

	_, err := service.Unfollow(ctx, &UnfollowCommand{FollowerID: "alice", FollowingID: "bob"})
	require.NoError(t, err)

	// Cover unblock path (emitUnblockEvents + queueFederationUndo).
	_, err = service.Unblock(ctx, &UnblockCommand{BlockerID: "alice", BlockedID: "bob"})
	require.NoError(t, err)

	// Unsupported relation type.
	_, _, err = service.getRelatedAccounts(ctx, "alice", 10, "", "bogus")
	assert.Error(t, err)
}

func TestService_Severance_AcknowledgeAndList(t *testing.T) {
	ctx := context.Background()
	service, _ := newServiceWithStorageHarness(t)

	_, err := service.AcknowledgeSeverance(ctx, &AcknowledgeSeveranceCommand{})
	assert.Error(t, err)

	result, err := service.AcknowledgeSeverance(ctx, &AcknowledgeSeveranceCommand{
		UserID:      "alice",
		SeveranceID: "sev-1",
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Success)

	_, err = service.GetAffectedRelationships(ctx, &GetAffectedRelationshipsQuery{})
	assert.Error(t, err)

	affected, err := service.GetAffectedRelationships(ctx, &GetAffectedRelationshipsQuery{
		UserID:                "alice",
		SeveredRelationshipID: "rel-1",
	})
	require.NoError(t, err)
	require.NotNil(t, affected)
}

func TestService_GetRelationships_Success(t *testing.T) {
	ctx := context.Background()
	service, _ := newServiceWithStorageHarness(t)

	result, err := service.GetRelationships(ctx, &GetRelationshipsQuery{
		RequesterID: "alice",
		TargetIDs:   []string{"bob", "carol"},
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Len(t, result.Relationships, 2)
}

func TestService_CreateRelationship_Branches(t *testing.T) {
	ctx := context.Background()

	// Storage-backed success.
	service, storageHarness := newServiceWithStorageHarness(t)
	require.NoError(t, service.createRelationship(ctx, "alice", "bob", "activity"))

	// Storage present but relationship repo missing.
	storageHarness.relationshipRepo = nil
	assert.Error(t, service.createRelationship(ctx, "alice", "bob", "activity"))

	// Legacy interface branch.
	legacyRepo := newStatefulRelationshipRepo()
	legacySvc := NewService(legacyRepo, nil, streaming.NewMockPublisher(), nil, zap.NewNop(), "example.com")
	require.NoError(t, legacySvc.createRelationship(ctx, "alice", "bob", "activity"))
	assert.Equal(t, 1, legacyRepo.createFollowRequestCalls)

	// No repositories available.
	noRepoSvc := NewService(nil, nil, streaming.NewMockPublisher(), nil, zap.NewNop(), "example.com")
	assert.Error(t, noRepoSvc.createRelationship(ctx, "alice", "bob", "activity"))
}

func TestService_AcceptFollowRequest_HelperBranches(t *testing.T) {
	ctx := context.Background()

	// Storage-backed branch.
	service, _ := newServiceWithStorageHarness(t)
	require.NoError(t, service.acceptFollowRequest(ctx, "alice", "bob"))

	// Legacy interface branch.
	repo := newStatefulRelationshipRepo()
	repo.setFollowStatus("alice", "bob", models.RelationshipPending)

	legacySvc := NewService(repo, nil, streaming.NewMockPublisher(), nil, zap.NewNop(), "example.com")
	require.NoError(t, legacySvc.acceptFollowRequest(ctx, "alice", "bob"))
	assert.Equal(t, 1, repo.acceptFollowRequestCalls)

	// No repository branch.
	noRepoSvc := NewService(nil, nil, streaming.NewMockPublisher(), nil, zap.NewNop(), "example.com")
	assert.Error(t, noRepoSvc.acceptFollowRequest(ctx, "alice", "bob"))
}

func TestService_EnsureAccountForActivity_Branches(t *testing.T) {
	ctx := context.Background()

	// No storage/account repo: creates minimal account.
	service := NewService(nil, nil, streaming.NewMockPublisher(), nil, zap.NewNop(), "example.com")
	account := service.ensureAccountForActivity(ctx, nil, " alice ")
	require.NotNil(t, account)
	require.NotNil(t, account.User)
	assert.Equal(t, "alice", account.User.Username)

	// Ensures User struct is present when account is non-nil.
	account = service.ensureAccountForActivity(ctx, &storage.Account{}, "bob")
	require.NotNil(t, account.User)
	assert.Equal(t, "bob", account.User.Username)

	// Storage-backed actor hydration.
	storageSvc, _ := newServiceWithStorageHarness(t)
	account = storageSvc.ensureAccountForActivity(ctx, nil, "carol")
	require.NotNil(t, account)
	require.NotNil(t, account.Actor)
}

type flakyRelationshipRepo struct {
	*statefulRelationshipRepo
	isBlockedErr   error
	isFollowingErr error
}

func (r *flakyRelationshipRepo) IsBlocked(ctx context.Context, blockerID, blockedID string) (bool, error) {
	if r.isBlockedErr != nil {
		return false, r.isBlockedErr
	}
	return r.statefulRelationshipRepo.IsBlocked(ctx, blockerID, blockedID)
}

func (r *flakyRelationshipRepo) IsFollowing(ctx context.Context, followerID, followingID string) (bool, error) {
	if r.isFollowingErr != nil {
		return false, r.isFollowingErr
	}
	return r.statefulRelationshipRepo.IsFollowing(ctx, followerID, followingID)
}

func TestService_Block_AdditionalBranches(t *testing.T) {
	ctx := context.Background()
	service, _ := newServiceWithStorageHarness(t)

	repo := &flakyRelationshipRepo{statefulRelationshipRepo: newStatefulRelationshipRepo()}
	service.relationshipRepo = repo

	// Cover self-block.
	_, err := service.Block(ctx, &BlockCommand{BlockerID: "alice", BlockedID: "alice"})
	assert.Error(t, err)

	// Cover early error from IsBlocked.
	repo.isBlockedErr = fmt.Errorf("blocked lookup failed")
	_, err = service.Block(ctx, &BlockCommand{BlockerID: "alice", BlockedID: "bob"})
	assert.Error(t, err)
	repo.isBlockedErr = nil

	// Cover already-blocked idempotent path.
	repo.setBlocked("alice", "bob", true)
	result, err := service.Block(ctx, &BlockCommand{BlockerID: "alice", BlockedID: "bob"})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Empty(t, result.Events)

	// Cover main path with storage-based account hydration and unfollow warnings.
	repo.setBlocked("alice", "bob", false)
	repo.setFollowStatus("alice", "bob", models.RelationshipAccepted)
	repo.setFollowStatus("bob", "alice", models.RelationshipAccepted)
	repo.isFollowingErr = fmt.Errorf("follow lookup failed")

	result, err = service.Block(ctx, &BlockCommand{BlockerID: "alice", BlockedID: "bob"})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.NotEmpty(t, result.Events)
}

func TestService_MuteUnmute_StorageHydrationAndNoStorage(t *testing.T) {
	ctx := context.Background()

	// Storage-backed account hydration (accountRepo nil).
	service, _ := newServiceWithStorageHarness(t)
	repo := newStatefulRelationshipRepo()
	service.relationshipRepo = repo
	service.accountRepo = nil

	// Create mute without duration.
	res, err := service.Mute(ctx, &MuteCommand{MuterID: "alice", MutedID: "bob"})
	require.NoError(t, err)
	require.NotNil(t, res)

	// Unmute uses storage-based account hydration.
	repo.setMuted("alice", "bob", true)
	res, err = service.Unmute(ctx, &UnmuteCommand{MuterID: "alice", MutedID: "bob"})
	require.NoError(t, err)
	require.NotNil(t, res)

	// No storage available should fail for create mute path.
	noStorageSvc := NewService(repo, nil, streaming.NewMockPublisher(), nil, zap.NewNop(), "example.com")
	_, err = noStorageSvc.Mute(ctx, &MuteCommand{MuterID: "alice", MutedID: "bob"})
	assert.Error(t, err)
}

func TestService_BaseURL_Branches(t *testing.T) {
	service := NewService(nil, nil, streaming.NewMockPublisher(), nil, zap.NewNop(), "")
	assert.Equal(t, "", service.baseURL())

	service.domainName = "http://example.com/"
	assert.Equal(t, "http://example.com", service.baseURL())

	service.domainName = "example.com/"
	assert.Equal(t, "https://example.com", service.baseURL())
}

func TestIsLocalActor(t *testing.T) {
	assert.False(t, isLocalActor(nil, "example.com"))
	assert.True(t, isLocalActor(&activitypub.Actor{BaseObject: activitypub.BaseObject{ID: "https://example.com/users/alice"}}, "example.com"))
	assert.True(t, isLocalActor(&activitypub.Actor{BaseObject: activitypub.BaseObject{ID: "HTTP://EXAMPLE.COM/users/alice"}}, "example.com"))
}

func TestService_StorageErrorBranches(t *testing.T) {
	ctx := context.Background()
	service := NewService(nil, nil, streaming.NewMockPublisher(), nil, zap.NewNop(), "example.com")

	_, err := service.IsBlocked(ctx, "alice", "bob")
	assert.Error(t, err)

	_, err = service.CountFollowing(ctx, "alice")
	assert.Error(t, err)

	_, _, err = service.getRelatedAccounts(ctx, "alice", 10, "", "followers")
	assert.Error(t, err)

	err = service.AddDomainBlock(ctx, &AddDomainBlockCommand{UserID: "alice", Domain: "blocked.example"})
	assert.Error(t, err)

	_, err = service.GetPendingFollowRequests(ctx, &GetFollowRequestsQuery{UserID: "alice"})
	assert.Error(t, err)
}

func TestService_GetRelationship_ValidationErrors(t *testing.T) {
	ctx := context.Background()
	service := NewService(nil, nil, streaming.NewMockPublisher(), nil, zap.NewNop(), "example.com")

	_, err := service.GetRelationship(ctx, "", "bob")
	assert.Error(t, err)
	_, err = service.GetRelationship(ctx, "alice", "")
	assert.Error(t, err)
}

func TestService_UnfollowUnblock_ValidationAndRepoNil(t *testing.T) {
	ctx := context.Background()
	service := NewService(nil, nil, streaming.NewMockPublisher(), nil, zap.NewNop(), "example.com")

	_, err := service.Unfollow(ctx, &UnfollowCommand{FollowerID: "", FollowingID: "bob"})
	assert.Error(t, err)
	_, err = service.Unfollow(ctx, &UnfollowCommand{FollowerID: "alice", FollowingID: "bob"})
	assert.Error(t, err)

	_, err = service.Unblock(ctx, &UnblockCommand{BlockerID: "", BlockedID: "bob"})
	assert.Error(t, err)
	_, err = service.Unblock(ctx, &UnblockCommand{BlockerID: "alice", BlockedID: "bob"})
	assert.Error(t, err)
}

func TestService_GetFollowAccounts_ErrorBranches(t *testing.T) {
	ctx := context.Background()

	service := NewService(nil, nil, streaming.NewMockPublisher(), nil, zap.NewNop(), "example.com")
	_, _, err := service.getFollowAccounts(ctx, "alice", "bob")
	assert.Error(t, err)

	service, _ = newServiceWithStorageHarness(t)
	accountRepo := &MockAccountRepository{}
	accountRepo.On("GetAccount", ctx, "alice").Return((*storage.Account)(nil), fmt.Errorf("account lookup failed")).Once()
	service.accountRepo = accountRepo

	_, _, err = service.getFollowAccounts(ctx, "alice", "bob")
	assert.Error(t, err)

	service, _ = newServiceWithStorageHarnessConfigured(t, func(_ *mocks.MockDB, q *mocks.MockQuery, _ *mocks.MockUpdateBuilder, _ *permissiveQueryState) {
		q.On("First", mock.Anything).Return(fmt.Errorf("actor lookup failed")).Once()
	})
	service.accountRepo = nil

	_, _, err = service.getFollowAccounts(ctx, "alice", "bob")
	assert.Error(t, err)
}

func TestService_CheckFollowPrerequisites_Branches(t *testing.T) {
	ctx := context.Background()

	service := NewService(nil, nil, streaming.NewMockPublisher(), nil, zap.NewNop(), "example.com")
	_, _, err := service.checkFollowPrerequisites(ctx, "alice", "bob")
	assert.Error(t, err)

	repo := &MockRelationshipRepository{}
	service.relationshipRepo = repo

	repo.On("IsFollowing", ctx, "alice", "bob").Return(false, fmt.Errorf("follow check failed")).Once()
	_, _, err = service.checkFollowPrerequisites(ctx, "alice", "bob")
	assert.Error(t, err)

	repo.On("IsFollowing", ctx, "alice", "bob").Return(false, nil).Once()
	repo.On("IsBlocked", ctx, "bob", "alice").Return(false, fmt.Errorf("block check failed")).Once()
	_, _, err = service.checkFollowPrerequisites(ctx, "alice", "bob")
	assert.Error(t, err)

	repo.On("IsFollowing", ctx, "alice", "bob").Return(false, nil).Once()
	repo.On("IsBlocked", ctx, "bob", "alice").Return(true, nil).Once()
	_, _, err = service.checkFollowPrerequisites(ctx, "alice", "bob")
	assert.Error(t, err)
}

func TestService_MuteUnmute_ErrorBranches(t *testing.T) {
	ctx := context.Background()

	noRepoSvc := NewService(nil, nil, streaming.NewMockPublisher(), nil, zap.NewNop(), "example.com")
	_, err := noRepoSvc.Mute(ctx, &MuteCommand{MuterID: "alice", MutedID: "bob"})
	assert.Error(t, err)
	_, err = noRepoSvc.Unmute(ctx, &UnmuteCommand{MuterID: "alice", MutedID: "bob"})
	assert.Error(t, err)

	// IsMuted error
	svc, _ := newServiceWithStorageHarness(t)
	relRepo := &MockRelationshipRepository{}
	relRepo.On("IsMuted", ctx, "alice", "bob").Return(false, fmt.Errorf("mute lookup failed")).Once()
	svc.relationshipRepo = relRepo
	_, err = svc.Mute(ctx, &MuteCommand{MuterID: "alice", MutedID: "bob"})
	assert.Error(t, err)

	// accountRepo error when loading accounts
	svc, _ = newServiceWithStorageHarness(t)
	svc.relationshipRepo = newStatefulRelationshipRepo()
	accRepo := &MockAccountRepository{}
	accRepo.On("GetAccount", ctx, "alice").Return(createTestAccount("alice", false), nil).Once()
	accRepo.On("GetAccount", ctx, "bob").Return((*storage.Account)(nil), fmt.Errorf("account lookup failed")).Once()
	svc.accountRepo = accRepo
	_, err = svc.Mute(ctx, &MuteCommand{MuterID: "alice", MutedID: "bob"})
	assert.Error(t, err)

	// CreateMute error via query.Create failure.
	svc, _ = newServiceWithStorageHarnessConfigured(t, func(_ *mocks.MockDB, q *mocks.MockQuery, _ *mocks.MockUpdateBuilder, _ *permissiveQueryState) {
		q.On("Create").Return(fmt.Errorf("create failed")).Once()
	})
	svc.relationshipRepo = newStatefulRelationshipRepo()
	svc.accountRepo = nil
	_, err = svc.Mute(ctx, &MuteCommand{MuterID: "alice", MutedID: "bob"})
	assert.Error(t, err)

	// IsMuted error for Unmute.
	svc, _ = newServiceWithStorageHarness(t)
	relRepo = &MockRelationshipRepository{}
	relRepo.On("IsMuted", ctx, "alice", "bob").Return(false, fmt.Errorf("mute lookup failed")).Once()
	svc.relationshipRepo = relRepo
	_, err = svc.Unmute(ctx, &UnmuteCommand{MuterID: "alice", MutedID: "bob"})
	assert.Error(t, err)

	// UnmuteUser error after successful actor hydration.
	svc, _ = newServiceWithStorageHarness(t)
	relRepo = &MockRelationshipRepository{}
	relRepo.On("IsMuted", ctx, "alice", "bob").Return(true, nil).Once()
	relRepo.On("UnmuteUser", ctx, "alice", "bob").Return(fmt.Errorf("unmute failed")).Once()
	svc.relationshipRepo = relRepo
	_, err = svc.Unmute(ctx, &UnmuteCommand{MuterID: "alice", MutedID: "bob"})
	assert.Error(t, err)
}

func TestService_AcceptRejectFollowRequest_ErrorBranches(t *testing.T) {
	ctx := context.Background()

	noStorageSvc := NewService(nil, nil, streaming.NewMockPublisher(), nil, zap.NewNop(), "example.com")
	_, err := noStorageSvc.AcceptFollowRequest(ctx, &AcceptFollowRequestCommand{RequesterID: "alice", FollowerID: "bob"})
	assert.Error(t, err)
	_, err = noStorageSvc.RejectFollowRequest(ctx, &RejectFollowRequestCommand{RequesterID: "alice", FollowerID: "bob"})
	assert.Error(t, err)

	svc, storageHarness := newServiceWithStorageHarness(t)
	storageHarness.relationshipRepo = nil
	_, err = svc.AcceptFollowRequest(ctx, &AcceptFollowRequestCommand{RequesterID: "alice", FollowerID: "bob"})
	assert.Error(t, err)

	// Actor hydration warnings skip event emission when one side is missing.
	svc, _ = newServiceWithStorageHarnessConfigured(t, func(_ *mocks.MockDB, q *mocks.MockQuery, _ *mocks.MockUpdateBuilder, _ *permissiveQueryState) {
		q.On("First", mock.Anything).Return(nil).Twice()
		q.On("First", mock.Anything).Return(fmt.Errorf("actor fetch failed")).Once()
	})
	res, err := svc.AcceptFollowRequest(ctx, &AcceptFollowRequestCommand{RequesterID: "alice", FollowerID: "bob"})
	require.NoError(t, err)
	require.NotNil(t, res)

	svc, _ = newServiceWithStorageHarnessConfigured(t, func(_ *mocks.MockDB, q *mocks.MockQuery, _ *mocks.MockUpdateBuilder, _ *permissiveQueryState) {
		q.On("First", mock.Anything).Return(nil).Twice()
		q.On("First", mock.Anything).Return(fmt.Errorf("actor fetch failed")).Once()
	})
	res, err = svc.RejectFollowRequest(ctx, &RejectFollowRequestCommand{RequesterID: "alice", FollowerID: "bob"})
	require.NoError(t, err)
	require.NotNil(t, res)
}

func TestService_QueueFederationBlockUndo_Branches(t *testing.T) {
	ctx := context.Background()
	publisher := streaming.NewMockPublisher()

	fed := &MockFederationService{}
	fed.On("QueueActivity", mock.Anything, mock.Anything).Return(fmt.Errorf("queue failed")).Maybe()

	service := NewService(nil, nil, publisher, fed, zap.NewNop(), "example.com")

	actor := &storage.Account{
		User:  &storage.User{Username: "alice"},
		Actor: &activitypub.Actor{BaseObject: activitypub.BaseObject{ID: "https://example.com/users/alice"}},
	}
	targetRemote := &storage.Account{
		User:  &storage.User{Username: "bob"},
		Actor: &activitypub.Actor{BaseObject: activitypub.BaseObject{ID: "https://remote.social/users/bob"}},
	}

	// Missing domain name.
	service.domainName = ""
	service.queueFederationBlock(ctx, actor, targetRemote)
	service.queueFederationUndo(ctx, actor, targetRemote, "Block")

	// Local actor is skipped.
	service.domainName = "example.com"
	targetLocal := &storage.Account{
		User:  &storage.User{Username: "bob"},
		Actor: &activitypub.Actor{BaseObject: activitypub.BaseObject{ID: "https://example.com/users/bob"}},
	}
	service.queueFederationBlock(ctx, actor, targetLocal)
	service.queueFederationUndo(ctx, actor, targetLocal, "Block")

	// Nil actor is skipped.
	service.queueFederationBlock(ctx, actor, &storage.Account{User: &storage.User{Username: "bob"}})
	service.queueFederationUndo(ctx, actor, &storage.Account{User: &storage.User{Username: "bob"}}, "Block")

	// Remote actor queues and handles QueueActivity errors.
	service.queueFederationBlock(ctx, actor, targetRemote)
	service.queueFederationUndo(ctx, actor, targetRemote, "Block")
}

func TestService_EventEmission_PublisherErrors(t *testing.T) {
	ctx := context.Background()

	publisher := streaming.NewMockPublisher()
	if configurable, ok := publisher.(interface {
		SetError(shouldError bool, message string)
	}); ok {
		configurable.SetError(true, "boom")
	}

	service := NewService(nil, nil, publisher, nil, zap.NewNop(), "example.com")

	alice := &storage.Account{
		User:  &storage.User{Username: "alice"},
		Actor: &activitypub.Actor{BaseObject: activitypub.BaseObject{ID: "https://example.com/users/alice"}},
	}
	bob := &storage.Account{
		User:  &storage.User{Username: "bob"},
		Actor: &activitypub.Actor{BaseObject: activitypub.BaseObject{ID: "https://example.com/users/bob"}},
	}

	assert.Empty(t, service.emitRelationshipEvents(ctx, alice, bob, "act", streaming.RelationshipFollowAccepted, "follow"))
	assert.Empty(t, service.emitBlockEvents(ctx, alice, bob))
	assert.Empty(t, service.emitUnblockEvents(ctx, alice, bob))
	assert.Empty(t, service.emitMuteEvents(ctx, alice, bob, nil))
	assert.Empty(t, service.emitUnmuteEvents(ctx, alice, bob))
	assert.Empty(t, service.emitSeveranceAcknowledgedEvents(ctx, "alice", "sev-1"))
}

func TestService_BuildFollowActivity_CornerCases(t *testing.T) {
	ctx := context.Background()
	service := NewService(nil, nil, streaming.NewMockPublisher(), nil, zap.NewNop(), "example.com")

	// Empty actor ID returns nil.
	assert.Nil(t, service.buildFollowActivity(ctx, nil, nil, "", "bob", "", nil))

	createdAt := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)
	relationship := &RelationshipData{CreatedAt: createdAt}

	// Prefer PreferredUsername when Actor.ID is empty; following nil forces fallback object assignment.
	activity := service.buildFollowActivity(ctx,
		&storage.Account{User: &storage.User{Username: "alice"}, Actor: &activitypub.Actor{PreferredUsername: "alice"}},
		nil,
		"alice",
		"",
		"",
		relationship,
	)
	require.NotNil(t, activity)
	require.NotNil(t, activity.Published)
	assert.Equal(t, createdAt, *activity.Published)
	assert.Equal(t, "https://example.com/users/alice", activity.Actor)
	assert.IsType(t, "", activity.Object)

	// Actor.ID trimming fallback when baseActor becomes empty.
	activity = service.buildFollowActivity(ctx,
		&storage.Account{User: &storage.User{Username: "alice"}, Actor: &activitypub.Actor{BaseObject: activitypub.BaseObject{ID: "/"}}},
		&storage.Account{User: &storage.User{Username: "bob"}, Actor: &activitypub.Actor{URL: "https://remote.social/users/bob"}},
		"alice",
		"bob",
		"",
		nil,
	)
	require.NotNil(t, activity)
}

func TestService_Block_UnfollowWarningBranches(t *testing.T) {
	ctx := context.Background()
	service, _ := newServiceWithStorageHarness(t)
	service.accountRepo = nil

	repo := &MockRelationshipRepository{}
	repo.On("IsBlocked", ctx, "alice", "bob").Return(false, nil).Once()
	repo.On("IsFollowing", ctx, "alice", "bob").Return(true, nil).Once()
	repo.On("Unfollow", ctx, "alice", "bob").Return(fmt.Errorf("unfollow failed")).Once()
	repo.On("IsFollowing", ctx, "bob", "alice").Return(true, nil).Once()
	repo.On("Unfollow", ctx, "bob", "alice").Return(fmt.Errorf("unfollow failed")).Once()
	repo.On("BlockUser", ctx, "alice", "bob").Return(nil).Once()

	service.relationshipRepo = repo
	service.federation = nil

	result, err := service.Block(ctx, &BlockCommand{BlockerID: "alice", BlockedID: "bob"})
	require.NoError(t, err)
	require.NotNil(t, result)
}

func TestService_Unmute_AccountAndActorLookupErrors(t *testing.T) {
	ctx := context.Background()

	// accountRepo error path.
	service, _ := newServiceWithStorageHarness(t)
	stateful := newStatefulRelationshipRepo()
	stateful.setMuted("alice", "bob", true)
	service.relationshipRepo = stateful

	accountRepo := &MockAccountRepository{}
	accountRepo.On("GetAccount", ctx, "alice").Return((*storage.Account)(nil), fmt.Errorf("account lookup failed")).Once()
	service.accountRepo = accountRepo

	_, err := service.Unmute(ctx, &UnmuteCommand{MuterID: "alice", MutedID: "bob"})
	assert.Error(t, err)

	// Storage actor lookup error path.
	service, _ = newServiceWithStorageHarnessConfigured(t, func(_ *mocks.MockDB, q *mocks.MockQuery, _ *mocks.MockUpdateBuilder, _ *permissiveQueryState) {
		q.On("First", mock.Anything).Return(fmt.Errorf("actor lookup failed")).Once()
	})
	stateful = newStatefulRelationshipRepo()
	stateful.setMuted("alice", "bob", true)
	service.relationshipRepo = stateful
	service.accountRepo = nil

	_, err = service.Unmute(ctx, &UnmuteCommand{MuterID: "alice", MutedID: "bob"})
	assert.Error(t, err)
}

func TestService_IsBlockedAndCountFollowing_RepositoryNil(t *testing.T) {
	ctx := context.Background()
	service, storageHarness := newServiceWithStorageHarness(t)
	storageHarness.relationshipRepo = nil

	_, err := service.IsBlocked(ctx, "alice", "bob")
	assert.Error(t, err)
	_, err = service.CountFollowing(ctx, "alice")
	assert.Error(t, err)
}

func TestService_QueueFederationReject_ErrorPath(t *testing.T) {
	ctx := context.Background()

	fed := &MockFederationService{}
	fed.On("QueueActivity", mock.Anything, mock.Anything).Return(fmt.Errorf("queue failed")).Once()

	service := NewService(nil, nil, streaming.NewMockPublisher(), fed, zap.NewNop(), "example.com")

	follower := &storage.Account{
		User:  &storage.User{Username: "alice"},
		Actor: &activitypub.Actor{BaseObject: activitypub.BaseObject{ID: "https://remote.social/users/alice"}},
	}
	following := &storage.Account{
		User:  &storage.User{Username: "bob"},
		Actor: &activitypub.Actor{BaseObject: activitypub.BaseObject{ID: "https://example.com/users/bob"}},
	}

	service.queueFederationReject(ctx, follower, following, "")
}

func TestService_RemoveRelationshipGeneric_ErrorBranches(t *testing.T) {
	ctx := context.Background()

	service, _ := newServiceWithStorageHarness(t)

	// Existence check error short-circuits.
	_, err := service.removeRelationshipGeneric(ctx, removeRelationshipParams{
		actorID:    "alice",
		targetID:   "bob",
		actorName:  "actor_id",
		targetName: "target_id",
		checkExistsFn: func(context.Context, string, string) (bool, error) {
			return false, fmt.Errorf("exists check failed")
		},
		removeFn:       func(context.Context, string, string) error { return nil },
		emitEventsFn:   func(context.Context, *storage.Account, *storage.Account) []*streaming.Event { return nil },
		federationType: "Follow",
	})
	assert.Error(t, err)

	// accountRepo lookup failures.
	accountRepo := &MockAccountRepository{}
	accountRepo.On("GetAccount", ctx, "alice").Return((*storage.Account)(nil), fmt.Errorf("actor lookup failed")).Once()
	service.accountRepo = accountRepo
	service.storage = nil

	_, err = service.removeRelationshipGeneric(ctx, removeRelationshipParams{
		actorID:        "alice",
		targetID:       "bob",
		actorName:      "actor_id",
		targetName:     "target_id",
		checkExistsFn:  func(context.Context, string, string) (bool, error) { return true, nil },
		removeFn:       func(context.Context, string, string) error { return nil },
		emitEventsFn:   func(context.Context, *storage.Account, *storage.Account) []*streaming.Event { return nil },
		federationType: "Follow",
	})
	assert.Error(t, err)

	service, _ = newServiceWithStorageHarness(t)
	accountRepo = &MockAccountRepository{}
	accountRepo.On("GetAccount", ctx, "alice").Return(&storage.Account{User: &storage.User{Username: "alice"}}, nil).Once()
	accountRepo.On("GetAccount", ctx, "bob").Return((*storage.Account)(nil), fmt.Errorf("target lookup failed")).Once()
	service.accountRepo = accountRepo
	service.storage = nil

	_, err = service.removeRelationshipGeneric(ctx, removeRelationshipParams{
		actorID:        "alice",
		targetID:       "bob",
		actorName:      "actor_id",
		targetName:     "target_id",
		checkExistsFn:  func(context.Context, string, string) (bool, error) { return true, nil },
		removeFn:       func(context.Context, string, string) error { return nil },
		emitEventsFn:   func(context.Context, *storage.Account, *storage.Account) []*streaming.Event { return nil },
		federationType: "Follow",
	})
	assert.Error(t, err)

	// Storage actor lookup failure in fallback branch.
	failingSvc, _ := newServiceWithStorageHarnessConfigured(t, func(_ *mocks.MockDB, q *mocks.MockQuery, _ *mocks.MockUpdateBuilder, _ *permissiveQueryState) {
		q.On("First", mock.Anything).Return(fmt.Errorf("db down")).Once()
	})
	_, err = failingSvc.removeRelationshipGeneric(ctx, removeRelationshipParams{
		actorID:        "alice",
		targetID:       "bob",
		actorName:      "actor_id",
		targetName:     "target_id",
		checkExistsFn:  func(context.Context, string, string) (bool, error) { return true, nil },
		removeFn:       func(context.Context, string, string) error { return nil },
		emitEventsFn:   func(context.Context, *storage.Account, *storage.Account) []*streaming.Event { return nil },
		federationType: "Follow",
	})
	assert.Error(t, err)

	// removeFn error after minimal account construction (no accountRepo/storage).
	noStorageSvc := NewService(nil, nil, streaming.NewMockPublisher(), nil, zap.NewNop(), "example.com")
	_, err = noStorageSvc.removeRelationshipGeneric(ctx, removeRelationshipParams{
		actorID:        "alice",
		targetID:       "bob",
		actorName:      "actor_id",
		targetName:     "target_id",
		checkExistsFn:  func(context.Context, string, string) (bool, error) { return true, nil },
		removeFn:       func(context.Context, string, string) error { return fmt.Errorf("remove failed") },
		emitEventsFn:   func(context.Context, *storage.Account, *storage.Account) []*streaming.Event { return nil },
		federationType: "Follow",
	})
	assert.Error(t, err)
}

func TestService_UpdateRelationship_ErrorBranches(t *testing.T) {
	ctx := context.Background()

	noStorageSvc := NewService(nil, nil, streaming.NewMockPublisher(), nil, zap.NewNop(), "example.com")
	_, err := noStorageSvc.UpdateRelationship(ctx, &UpdateRelationshipCommand{FollowerID: "alice", FollowingID: "bob"})
	assert.Error(t, err)

	svc, storageHarness := newServiceWithStorageHarness(t)
	storageHarness.relationshipRepo = nil
	_, err = svc.UpdateRelationship(ctx, &UpdateRelationshipCommand{FollowerID: "alice", FollowingID: "bob"})
	assert.Error(t, err)

	// isFollowing error propagates as a service error.
	svc, _ = newServiceWithStorageHarnessConfigured(t, func(_ *mocks.MockDB, q *mocks.MockQuery, _ *mocks.MockUpdateBuilder, _ *permissiveQueryState) {
		q.On("First", mock.Anything).Return(fmt.Errorf("db down")).Once()
	})
	_, err = svc.UpdateRelationship(ctx, &UpdateRelationshipCommand{FollowerID: "alice", FollowingID: "bob"})
	assert.Error(t, err)

	// Not following -> consistent error.
	svc, _ = newServiceWithStorageHarnessConfigured(t, func(_ *mocks.MockDB, q *mocks.MockQuery, _ *mocks.MockUpdateBuilder, _ *permissiveQueryState) {
		q.On("First", mock.Anything).Return(dynamormerrors.ErrItemNotFound).Once()
	})
	_, err = svc.UpdateRelationship(ctx, &UpdateRelationshipCommand{FollowerID: "alice", FollowingID: "bob"})
	assert.Error(t, err)

	// Update failure.
	svc, _ = newServiceWithStorageHarnessConfigured(t, func(_ *mocks.MockDB, q *mocks.MockQuery, _ *mocks.MockUpdateBuilder, _ *permissiveQueryState) {
		q.On("Update").Return(fmt.Errorf("update failed")).Once()
		q.On("Update", mock.Anything).Return(fmt.Errorf("update failed")).Maybe()
	})
	notify := false
	_, err = svc.UpdateRelationship(ctx, &UpdateRelationshipCommand{
		FollowerID:  "alice",
		FollowingID: "bob",
		Notify:      &notify,
	})
	assert.Error(t, err)
}

func TestService_DomainBlocks_ErrorBranches(t *testing.T) {
	ctx := context.Background()

	noStorageSvc := NewService(nil, nil, streaming.NewMockPublisher(), nil, zap.NewNop(), "example.com")
	assert.Error(t, noStorageSvc.RemoveDomainBlock(ctx, &RemoveDomainBlockCommand{UserID: "alice", Domain: "blocked.example"}))

	svc, storageHarness := newServiceWithStorageHarness(t)
	storageHarness.domainBlockRepo = nil
	assert.Error(t, svc.RemoveDomainBlock(ctx, &RemoveDomainBlockCommand{UserID: "alice", Domain: "blocked.example"}))

	// Repository query error.
	svc, _ = newServiceWithStorageHarnessConfigured(t, func(_ *mocks.MockDB, q *mocks.MockQuery, _ *mocks.MockUpdateBuilder, _ *permissiveQueryState) {
		q.On("All", mock.Anything).Return(fmt.Errorf("query failed")).Once()
	})
	_, err := svc.GetDomainBlocks(ctx, &GetDomainBlocksQuery{UserID: "alice", Limit: 1})
	assert.Error(t, err)
}

func TestService_GetMutedUsers_WarnsOnActorLookupError(t *testing.T) {
	ctx := context.Background()
	svc, _ := newServiceWithStorageHarnessConfigured(t, func(_ *mocks.MockDB, q *mocks.MockQuery, _ *mocks.MockUpdateBuilder, _ *permissiveQueryState) {
		q.On("First", mock.Anything).Return(fmt.Errorf("actor fetch failed")).Once()
	})

	res, err := svc.GetMutedUsers(ctx, &GetMutedUsersQuery{UserID: "alice"})
	require.NoError(t, err)
	require.NotNil(t, res)
}

func TestService_GetBlockedUsers_WarnsOnActorLookupError(t *testing.T) {
	ctx := context.Background()
	svc, _ := newServiceWithStorageHarnessConfigured(t, func(_ *mocks.MockDB, q *mocks.MockQuery, _ *mocks.MockUpdateBuilder, _ *permissiveQueryState) {
		q.On("First", mock.Anything).Return(fmt.Errorf("actor fetch failed")).Once()
	})

	res, err := svc.GetBlockedUsers(ctx, &GetBlockedUsersQuery{UserID: "alice"})
	require.NoError(t, err)
	require.NotNil(t, res)
}

func TestService_BuildAccountFromActor_UserRepoErrorFallsBack(t *testing.T) {
	ctx := context.Background()
	svc, _ := newServiceWithStorageHarnessConfigured(t, func(_ *mocks.MockDB, q *mocks.MockQuery, _ *mocks.MockUpdateBuilder, _ *permissiveQueryState) {
		q.On("First", mock.Anything).Return(fmt.Errorf("db down")).Once()
	})

	now := time.Now().UTC()
	actor := &activitypub.Actor{
		BaseObject: activitypub.BaseObject{
			ID:        "https://remote.social/users/bob",
			Type:      "Person",
			Published: &now,
			Updated:   &now,
		},
		PreferredUsername: "bob",
	}

	account := svc.buildAccountFromActor(ctx, actor, "bob")
	require.NotNil(t, account)
	require.NotNil(t, account.User)
}

func TestService_QueueFederationFollow_Branches(t *testing.T) {
	ctx := context.Background()
	publisher := streaming.NewMockPublisher()

	followingRemote := &storage.Account{
		User:  &storage.User{Username: "bob"},
		Actor: &activitypub.Actor{BaseObject: activitypub.BaseObject{ID: "https://remote.social/users/bob"}},
	}
	follower := &storage.Account{
		User:  &storage.User{Username: "alice"},
		Actor: &activitypub.Actor{BaseObject: activitypub.BaseObject{ID: "https://example.com/users/alice"}},
	}

	fed := &MockFederationService{}
	var queued []*activitypub.Activity
	fed.On("QueueActivity", mock.Anything, mock.AnythingOfType("*activitypub.Activity")).
		Run(func(args mock.Arguments) {
			if activity, ok := args.Get(1).(*activitypub.Activity); ok {
				queued = append(queued, activity)
			}
		}).
		Return(fmt.Errorf("queue failed")).
		Maybe()

	service := NewService(nil, nil, publisher, fed, zap.NewNop(), "example.com")
	followActivity := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			Context: activitypub.Context,
			Type:    activitypub.FollowType,
			ID:      "https://example.com/activities/follow-act",
			To:      []string{followingRemote.Actor.ID},
		},
		Actor:  follower.Actor.ID,
		Object: followingRemote.Actor.ID,
	}

	// Missing domain name.
	service.domainName = ""
	service.queueFederationFollow(ctx, followActivity, follower, followingRemote, "follow")

	// Local actor is skipped.
	service.domainName = "example.com"
	followingLocal := &storage.Account{
		User:  &storage.User{Username: "bob"},
		Actor: &activitypub.Actor{BaseObject: activitypub.BaseObject{ID: "https://example.com/users/bob"}},
	}
	service.queueFederationFollow(ctx, followActivity, follower, followingLocal, "follow")

	// Nil actor is skipped.
	service.queueFederationFollow(ctx, followActivity, follower, &storage.Account{User: &storage.User{Username: "bob"}}, "follow")

	// Remote actor queues and handles QueueActivity errors.
	service.queueFederationFollow(ctx, followActivity, follower, followingRemote, "follow")
	require.Len(t, queued, 1)
	require.Equal(t, followingRemote.Actor.ID, queued[0].Object)
	require.Equal(t, []string{followingRemote.Actor.ID}, queued[0].To)
}

func TestService_GetRelatedAccounts_RemoteAwareReadback(t *testing.T) {
	ctx := context.Background()
	service, storageHarness := newServiceWithStorageHarness(t)

	actorRepo := inmemory.NewActorRepository()
	userRepo := inmemory.NewUserRepository()
	relationshipRepo := inmemory.NewRelationshipRepository()
	storageHarness.actorRepo = actorRepo
	storageHarness.userRepo = userRepo
	storageHarness.relationshipRepo = relationshipRepo

	createLocalActor := func(username string) *activitypub.Actor {
		return &activitypub.Actor{
			BaseObject: activitypub.BaseObject{
				ID:   fmt.Sprintf("https://example.com/users/%s", username),
				Type: activitypub.PersonType,
			},
			PreferredUsername: username,
			Inbox:             fmt.Sprintf("https://example.com/users/%s/inbox", username),
			Outbox:            fmt.Sprintf("https://example.com/users/%s/outbox", username),
		}
	}

	for _, username := range []string{"alice", "bob", "carol"} {
		require.NoError(t, actorRepo.CreateActor(ctx, createLocalActor(username), ""))
	}

	remoteFollower := &activitypub.Actor{
		BaseObject: activitypub.BaseObject{
			ID:   "https://remote.social/users/dave",
			Type: activitypub.PersonType,
		},
		PreferredUsername: "dave",
		URL:               "https://remote.social/@dave",
		Inbox:             "https://remote.social/users/dave/inbox",
		Outbox:            "https://remote.social/users/dave/outbox",
	}
	remoteFollowing := &activitypub.Actor{
		BaseObject: activitypub.BaseObject{
			ID:   "https://remote.social/users/erin",
			Type: activitypub.PersonType,
		},
		PreferredUsername: "erin",
		URL:               "https://remote.social/@erin",
		Inbox:             "https://remote.social/users/erin/inbox",
		Outbox:            "https://remote.social/users/erin/outbox",
	}
	actorRepo.SetCachedRemoteActor("dave@remote.social", remoteFollower, time.Hour)
	actorRepo.SetCachedRemoteActor("erin@remote.social", remoteFollowing, time.Hour)

	require.NoError(t, relationshipRepo.CreateRelationship(ctx, "bob", "alice", "act-bob"))
	require.NoError(t, relationshipRepo.AcceptFollowRequest(ctx, "bob", "alice"))
	require.NoError(t, relationshipRepo.CreateRelationship(ctx, "dave@remote.social", "alice", "act-dave"))
	require.NoError(t, relationshipRepo.AcceptFollowRequest(ctx, "dave@remote.social", "alice"))
	require.NoError(t, relationshipRepo.CreateRelationship(ctx, "alice", "carol", "act-carol"))
	require.NoError(t, relationshipRepo.AcceptFollowRequest(ctx, "alice", "carol"))
	require.NoError(t, relationshipRepo.CreateRelationship(ctx, "alice", "erin@remote.social", "act-erin"))
	require.NoError(t, relationshipRepo.AcceptFollowRequest(ctx, "alice", "erin@remote.social"))

	followers, _, err := service.GetFollowers(ctx, "alice", 10, "")
	require.NoError(t, err)
	require.Len(t, followers, 2)

	followerAccounts := make(map[string]string, len(followers))
	for _, account := range followers {
		require.NotNil(t, account)
		require.NotNil(t, account.User)
		require.NotNil(t, account.Actor)
		followerAccounts[account.User.Username] = account.Actor.ID
	}
	require.Equal(t, "https://example.com/users/bob", followerAccounts["bob"])
	require.Equal(t, remoteFollower.ID, followerAccounts["dave@remote.social"])

	following, _, err := service.GetFollowing(ctx, "alice", 10, "")
	require.NoError(t, err)
	require.Len(t, following, 2)

	followingAccounts := make(map[string]string, len(following))
	for _, account := range following {
		require.NotNil(t, account)
		require.NotNil(t, account.User)
		require.NotNil(t, account.Actor)
		followingAccounts[account.User.Username] = account.Actor.ID
	}
	require.Equal(t, "https://example.com/users/carol", followingAccounts["carol"])
	require.Equal(t, remoteFollowing.ID, followingAccounts["erin@remote.social"])
}

func TestIsNilFederationService(t *testing.T) {
	assert.True(t, isNilFederationService(nil))

	var typedNil *panicFederationService
	assert.True(t, isNilFederationService(typedNil))

	assert.False(t, isNilFederationService(panicFederationService{}))
}

func TestNewServiceWithStorage_ImplementsInterface(t *testing.T) {
	service, storageHarness := newServiceWithStorageHarness(t)
	assert.NotNil(t, service)
	assert.NotNil(t, storageHarness)
	var _ core.RepositoryStorage = storageHarness
}

func TestService_ValidationHelpers_AdditionalBranches(t *testing.T) {
	ctx := context.Background()
	service := NewService(nil, nil, streaming.NewMockPublisher(), nil, zap.NewNop(), "example.com")

	assert.Error(t, service.validateAddDomainBlockCommand(&AddDomainBlockCommand{UserID: "", Domain: "blocked.example"}))
	assert.Error(t, service.validateRemoveDomainBlockCommand(&RemoveDomainBlockCommand{UserID: "", Domain: "blocked.example"}))
	assert.Error(t, service.validateRemoveDomainBlockCommand(&RemoveDomainBlockCommand{UserID: "alice", Domain: ""}))

	blocks := &GetDomainBlocksQuery{UserID: "alice"}
	require.NoError(t, service.validateGetDomainBlocksQuery(blocks))
	assert.Equal(t, 100, blocks.Limit)
	assert.Error(t, service.validateGetDomainBlocksQuery(&GetDomainBlocksQuery{}))

	muted := &GetMutedUsersQuery{UserID: "alice"}
	require.NoError(t, service.validateGetMutedUsersQuery(muted))
	assert.Equal(t, 40, muted.Limit)
	assert.Error(t, service.validateGetMutedUsersQuery(&GetMutedUsersQuery{}))

	blocked := &GetBlockedUsersQuery{UserID: "alice"}
	require.NoError(t, service.validateGetBlockedUsersQuery(blocked))
	assert.Equal(t, 40, blocked.Limit)
	assert.Error(t, service.validateGetBlockedUsersQuery(&GetBlockedUsersQuery{}))

	requests := &GetFollowRequestsQuery{UserID: "alice"}
	require.NoError(t, service.validateGetFollowRequestsQuery(requests))
	assert.Equal(t, 100, requests.Limit)
	assert.Error(t, service.validateGetFollowRequestsQuery(&GetFollowRequestsQuery{}))

	assert.Error(t, service.validateAcceptFollowRequestCommand(&AcceptFollowRequestCommand{RequesterID: "alice", FollowerID: "alice"}))
	assert.Error(t, service.validateAcceptFollowRequestCommand(&AcceptFollowRequestCommand{RequesterID: "", FollowerID: "bob"}))
	assert.Error(t, service.validateAcceptFollowRequestCommand(&AcceptFollowRequestCommand{RequesterID: "alice", FollowerID: ""}))
	assert.Error(t, service.validateRejectFollowRequestCommand(&RejectFollowRequestCommand{RequesterID: "alice", FollowerID: "alice"}))
	assert.Error(t, service.validateRejectFollowRequestCommand(&RejectFollowRequestCommand{RequesterID: "", FollowerID: "bob"}))
	assert.Error(t, service.validateRejectFollowRequestCommand(&RejectFollowRequestCommand{RequesterID: "alice", FollowerID: ""}))

	assert.Error(t, service.validateUnfollowCommand(ctx, &UnfollowCommand{FollowerID: "alice", FollowingID: ""}))
	assert.Error(t, service.validateBlockCommand(ctx, &BlockCommand{BlockerID: "", BlockedID: "bob"}))
	assert.Error(t, service.validateBlockCommand(ctx, &BlockCommand{BlockerID: "alice", BlockedID: ""}))
	assert.Error(t, service.validateMuteCommand(ctx, &MuteCommand{MuterID: "", MutedID: "bob"}))
	assert.Error(t, service.validateMuteCommand(ctx, &MuteCommand{MuterID: "alice", MutedID: ""}))
	assert.Error(t, service.validateUnmuteCommand(ctx, &UnmuteCommand{MuterID: "", MutedID: "bob"}))
	assert.Error(t, service.validateUnmuteCommand(ctx, &UnmuteCommand{MuterID: "alice", MutedID: ""}))

	assert.Error(t, service.validateGetRelationshipsQuery(&GetRelationshipsQuery{RequesterID: "alice"}))

	var tooMany []string
	for i := range 41 {
		tooMany = append(tooMany, fmt.Sprintf("user-%d", i))
	}
	assert.Error(t, service.validateGetRelationshipsQuery(&GetRelationshipsQuery{
		RequesterID: "alice",
		TargetIDs:   tooMany,
	}))

	assert.Error(t, service.validateAcknowledgeSeveranceCommand(&AcknowledgeSeveranceCommand{UserID: "alice"}))
	assert.Error(t, service.validateGetAffectedRelationshipsQuery(&GetAffectedRelationshipsQuery{UserID: "alice"}))
}

func TestService_NormalizeActorIdentifier_Branches(t *testing.T) {
	service := NewService(nil, nil, streaming.NewMockPublisher(), nil, zap.NewNop(), "example.com")

	assert.Equal(t, "", service.normalizeActorIdentifier("  "))
	assert.Equal(t, "alice", service.normalizeActorIdentifier(" alice "))

	// Unparseable URL -> return original.
	assert.Equal(t, "https://[::1/users/alice", service.normalizeActorIdentifier("https://[::1/users/alice"))

	// Empty path -> return original.
	assert.Equal(t, "https://remote.social/", service.normalizeActorIdentifier("https://remote.social/"))

	// Username collapses to empty after trimming ".json".
	assert.Equal(t, "https://remote.social/.json", service.normalizeActorIdentifier("https://remote.social/.json"))

	// Hostless URL returns username.
	assert.Equal(t, "alice", service.normalizeActorIdentifier("https:///users/alice"))

	// Local domain returns username (strip port when comparing).
	service.domainName = "example.com:443"
	assert.Equal(t, "bob", service.normalizeActorIdentifier("https://example.com/users/bob"))

	// Remote domain retains host.
	assert.Equal(t, "carol@remote.social", service.normalizeActorIdentifier("https://remote.social/users/carol"))
}

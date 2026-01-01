package federation

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	appConfig "github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/core"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	dynamormcore "github.com/pay-theory/dynamorm/pkg/core"
	dynamormerrors "github.com/pay-theory/dynamorm/pkg/errors"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type memDB struct {
	mu sync.Mutex

	relationships map[string]models.FederationRelationship
	aggregates    map[string]models.FederationRelationshipAggregate
	indexes       map[string]models.FederationRelationshipIndex
	fedEdges      map[string]models.FederationEdge

	instanceConnectionsByGSI2PK map[string][]models.InstanceConnection

	hookFirst       func(q *memQuery, dest any) error
	hookScan        func(q *memQuery, dest any) error
	hookCreate      func(q *memQuery) error
	hookDelete      func(q *memQuery) error
	hookBatchCreate func(q *memQuery, items any) error
}

func newMemDB() *memDB {
	return &memDB{
		relationships:               make(map[string]models.FederationRelationship),
		aggregates:                  make(map[string]models.FederationRelationshipAggregate),
		indexes:                     make(map[string]models.FederationRelationshipIndex),
		fedEdges:                    make(map[string]models.FederationEdge),
		instanceConnectionsByGSI2PK: make(map[string][]models.InstanceConnection),
	}
}

func (db *memDB) Model(model any) dynamormcore.Query { return newMemQuery(db, model) }
func (db *memDB) Transaction(_ func(tx *dynamormcore.Tx) error) error {
	return nil
}
func (db *memDB) Migrate() error                                { return nil }
func (db *memDB) AutoMigrate(_ ...any) error                    { return nil }
func (db *memDB) Close() error                                  { return nil }
func (db *memDB) WithContext(_ context.Context) dynamormcore.DB { return db }

type memCondition struct {
	op    string
	value any
}

type memQuery struct {
	db    *memDB
	model any

	index string
	where map[string]memCondition
	limit int
}

func newMemQuery(db *memDB, model any) *memQuery {
	return &memQuery{
		db:    db,
		model: model,
		where: make(map[string]memCondition),
		limit: -1,
	}
}

func (q *memQuery) Where(field string, op string, value any) dynamormcore.Query {
	q.where[field] = memCondition{op: op, value: value}
	return q
}
func (q *memQuery) Index(indexName string) dynamormcore.Query                           { q.index = indexName; return q }
func (q *memQuery) Filter(string, string, any) dynamormcore.Query                       { return q }
func (q *memQuery) OrFilter(string, string, any) dynamormcore.Query                     { return q }
func (q *memQuery) FilterGroup(func(dynamormcore.Query)) dynamormcore.Query             { return q }
func (q *memQuery) OrFilterGroup(func(dynamormcore.Query)) dynamormcore.Query           { return q }
func (q *memQuery) IfNotExists() dynamormcore.Query                                     { return q }
func (q *memQuery) IfExists() dynamormcore.Query                                        { return q }
func (q *memQuery) WithCondition(string, string, any) dynamormcore.Query                { return q }
func (q *memQuery) WithConditionExpression(string, map[string]any) dynamormcore.Query   { return q }
func (q *memQuery) OrderBy(string, string) dynamormcore.Query                           { return q }
func (q *memQuery) Limit(limit int) dynamormcore.Query                                  { q.limit = limit; return q }
func (q *memQuery) Offset(int) dynamormcore.Query                                       { return q }
func (q *memQuery) Select(...string) dynamormcore.Query                                 { return q }
func (q *memQuery) ConsistentRead() dynamormcore.Query                                  { return q }
func (q *memQuery) WithRetry(int, time.Duration) dynamormcore.Query                     { return q }
func (q *memQuery) All(any) error                                                       { return nil }
func (q *memQuery) AllPaginated(any) (*dynamormcore.PaginatedResult, error)             { return nil, nil }
func (q *memQuery) Count() (int64, error)                                               { return 0, nil }
func (q *memQuery) CreateOrUpdate() error                                               { return q.Create() }
func (q *memQuery) Update(...string) error                                              { return nil }
func (q *memQuery) UpdateBuilder() dynamormcore.UpdateBuilder                           { return nil }
func (q *memQuery) ParallelScan(int32, int32) dynamormcore.Query                        { return q }
func (q *memQuery) ScanAllSegments(any, int32) error                                    { return nil }
func (q *memQuery) BatchGet([]any, any) error                                           { return nil }
func (q *memQuery) BatchGetWithOptions([]any, any, *dynamormcore.BatchGetOptions) error { return nil }
func (q *memQuery) BatchGetBuilder() dynamormcore.BatchGetBuilder                       { return nil }
func (q *memQuery) BatchDelete([]any) error                                             { return nil }
func (q *memQuery) BatchWrite([]any, []any) error                                       { return nil }
func (q *memQuery) BatchUpdateWithOptions([]any, []string, ...any) error                { return nil }
func (q *memQuery) Cursor(string) dynamormcore.Query                                    { return q }
func (q *memQuery) SetCursor(string) error                                              { return nil }
func (q *memQuery) WithContext(context.Context) dynamormcore.Query                      { return q }

func (q *memQuery) First(dest any) error {
	if q.db.hookFirst != nil {
		if err := q.db.hookFirst(q, dest); err != nil {
			return err
		}
	}

	q.db.mu.Lock()
	defer q.db.mu.Unlock()

	switch typedDest := dest.(type) {
	case *models.FederationRelationship:
		key, ok := q.primaryKey()
		if !ok {
			return fmt.Errorf("missing PK/SK where clauses")
		}
		found, exists := q.db.relationships[key]
		if !exists {
			return dynamormerrors.ErrItemNotFound
		}
		*typedDest = found
		return nil
	case *models.FederationRelationshipAggregate:
		key, ok := q.primaryKey()
		if !ok {
			return fmt.Errorf("missing PK/SK where clauses")
		}
		found, exists := q.db.aggregates[key]
		if !exists {
			return dynamormerrors.ErrItemNotFound
		}
		*typedDest = found
		return nil
	case *models.FederationRelationshipIndex:
		key, ok := q.primaryKey()
		if !ok {
			return fmt.Errorf("missing PK/SK where clauses")
		}
		found, exists := q.db.indexes[key]
		if !exists {
			return dynamormerrors.ErrItemNotFound
		}
		*typedDest = found
		return nil
	case *models.FederationEdge:
		key, ok := q.primaryKey()
		if !ok {
			return fmt.Errorf("missing PK/SK where clauses")
		}
		found, exists := q.db.fedEdges[key]
		if !exists {
			return dynamormerrors.ErrItemNotFound
		}
		*typedDest = found
		return nil
	default:
		return dynamormerrors.ErrItemNotFound
	}
}

func (q *memQuery) Create() error {
	if q.db.hookCreate != nil {
		if err := q.db.hookCreate(q); err != nil {
			return err
		}
	}

	q.db.mu.Lock()
	defer q.db.mu.Unlock()

	switch m := q.model.(type) {
	case *models.FederationRelationship:
		q.db.relationships[m.PK+"|"+m.SK] = *m
	case *models.FederationRelationshipAggregate:
		q.db.aggregates[m.PK+"|"+m.SK] = *m
	case *models.FederationRelationshipIndex:
		q.db.indexes[m.PK+"|"+m.SK] = *m
	case *models.FederationEdge:
		q.db.fedEdges[m.PK+"|"+m.SK] = *m
	default:
		// Treat unknown models as successful no-ops for this in-memory harness.
	}

	return nil
}

func (q *memQuery) Delete() error {
	if q.db.hookDelete != nil {
		if err := q.db.hookDelete(q); err != nil {
			return err
		}
	}

	q.db.mu.Lock()
	defer q.db.mu.Unlock()

	switch m := q.model.(type) {
	case *models.FederationRelationship:
		delete(q.db.relationships, m.PK+"|"+m.SK)
	case *models.FederationRelationshipIndex:
		delete(q.db.indexes, m.PK+"|"+m.SK)
	default:
	}
	return nil
}

func (q *memQuery) Scan(dest any) error {
	if q.db.hookScan != nil {
		if err := q.db.hookScan(q, dest); err != nil {
			return err
		}
	}

	q.db.mu.Lock()
	defer q.db.mu.Unlock()

	switch typedDest := dest.(type) {
	case *[]models.FederationRelationship:
		var out []models.FederationRelationship
		for _, rel := range q.db.relationships {
			if !q.matchesRelationship(rel) {
				continue
			}
			out = append(out, rel)
			if q.limit > 0 && len(out) >= q.limit {
				break
			}
		}
		*typedDest = out
		return nil
	case *[]models.FederationEdge:
		var out []models.FederationEdge
		for _, edge := range q.db.fedEdges {
			if !q.matchesFederationEdge(edge) {
				continue
			}
			out = append(out, edge)
			if q.limit > 0 && len(out) >= q.limit {
				break
			}
		}
		*typedDest = out
		return nil
	case *[]models.InstanceConnection:
		pkValue, _ := q.stringWhere("gsi2PK")
		conns := q.db.instanceConnectionsByGSI2PK[pkValue]
		out := append([]models.InstanceConnection(nil), conns...)
		if q.limit > 0 && len(out) > q.limit {
			out = out[:q.limit]
		}
		*typedDest = out
		return nil
	default:
		return nil
	}
}

func (q *memQuery) BatchCreate(items any) error {
	if q.db.hookBatchCreate != nil {
		if err := q.db.hookBatchCreate(q, items); err != nil {
			return err
		}
	}

	slice, ok := items.([]any)
	if !ok {
		return fmt.Errorf("batch create expects []any, got %T", items)
	}

	q.db.mu.Lock()
	defer q.db.mu.Unlock()

	for _, item := range slice {
		switch rel := item.(type) {
		case models.FederationRelationship:
			q.db.relationships[rel.PK+"|"+rel.SK] = rel
		case *models.FederationRelationship:
			q.db.relationships[rel.PK+"|"+rel.SK] = *rel
		default:
		}
	}

	return nil
}

func (q *memQuery) primaryKey() (string, bool) {
	pk, ok1 := q.stringWhere("PK")
	sk, ok2 := q.stringWhere("SK")
	if !ok1 || !ok2 {
		return "", false
	}
	return pk + "|" + sk, true
}

func (q *memQuery) stringWhere(field string) (string, bool) {
	cond, ok := q.where[field]
	if !ok {
		return "", false
	}
	if cond.op != "=" {
		return "", false
	}
	s, ok := cond.value.(string)
	return s, ok
}

func (q *memQuery) matchesRelationship(rel models.FederationRelationship) bool {
	if pk, ok := q.stringWhere("PK"); ok && rel.PK != pk {
		return false
	}
	if q.index == models.IndexGSI1 {
		if cond, ok := q.where["gsi1PK"]; ok && cond.op == "=" {
			want, _ := cond.value.(string)
			if rel.GSI1PK != want {
				return false
			}
		}
		if cond, ok := q.where["gsi1SK"]; ok && cond.op == "<" {
			thresholdStr, _ := cond.value.(string)
			threshold, err := parseUnix(thresholdStr)
			if err == nil && rel.LastActivity.Unix() >= threshold {
				return false
			}
		}
	}
	return true
}

func (q *memQuery) matchesFederationEdge(edge models.FederationEdge) bool {
	if q.index == "gsi2" {
		if cond, ok := q.where["gsi2PK"]; ok && cond.op == "begins_with" {
			prefix, _ := cond.value.(string)
			if prefix != "" && len(edge.GSI2PK) >= len(prefix) && edge.GSI2PK[:len(prefix)] != prefix {
				return false
			}
		}
	}
	return true
}

func parseUnix(s string) (int64, error) {
	var out int64
	_, err := fmt.Sscanf(s, "%d", &out)
	return out, err
}

type fakeS3 struct {
	mu sync.Mutex

	putErr    error
	getErr    error
	deleteErr error

	objects map[string][]byte
}

func newFakeS3() *fakeS3 {
	return &fakeS3{objects: make(map[string][]byte)}
}

func (s *fakeS3) PutObject(_ context.Context, input *s3.PutObjectInput, _ ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
	if s.putErr != nil {
		return nil, s.putErr
	}
	if input == nil || input.Key == nil {
		return nil, errors.New("missing key")
	}

	data, err := io.ReadAll(input.Body)
	if err != nil {
		return nil, err
	}

	s.mu.Lock()
	s.objects[*input.Key] = data
	s.mu.Unlock()

	return &s3.PutObjectOutput{}, nil
}

func (s *fakeS3) GetObject(_ context.Context, input *s3.GetObjectInput, _ ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	if s.getErr != nil {
		return nil, s.getErr
	}
	if input == nil || input.Key == nil {
		return nil, errors.New("missing key")
	}

	s.mu.Lock()
	data, ok := s.objects[*input.Key]
	s.mu.Unlock()
	if !ok {
		return nil, errors.New("not found")
	}

	size := int64(len(data))
	return &s3.GetObjectOutput{
		Body:          io.NopCloser(bytes.NewReader(data)),
		ContentLength: &size,
	}, nil
}

func (s *fakeS3) DeleteObject(_ context.Context, input *s3.DeleteObjectInput, _ ...func(*s3.Options)) (*s3.DeleteObjectOutput, error) {
	if s.deleteErr != nil {
		return nil, s.deleteErr
	}
	if input == nil || input.Key == nil {
		return nil, errors.New("missing key")
	}
	s.mu.Lock()
	delete(s.objects, *input.Key)
	s.mu.Unlock()
	return &s3.DeleteObjectOutput{}, nil
}

type repoStorageStub struct {
	federation *repositories.FederationRepository
	db         dynamormcore.DB
	tableName  string
	logger     *zap.Logger
}

var _ core.RepositoryStorage = (*repoStorageStub)(nil)

func (s *repoStorageStub) Federation() *repositories.FederationRepository { return s.federation }
func (s *repoStorageStub) GetDB() dynamormcore.DB                         { return s.db }
func (s *repoStorageStub) GetTableName() string                           { return s.tableName }
func (s *repoStorageStub) GetLogger() *zap.Logger                         { return s.logger }

func (s *repoStorageStub) Account() *repositories.AccountRepository                     { return nil }
func (s *repoStorageStub) Bookmark() *repositories.BookmarkRepository                   { return nil }
func (s *repoStorageStub) Actor() interfaces.ActorRepository                            { return nil }
func (s *repoStorageStub) Object() *repositories.ObjectRepository                       { return nil }
func (s *repoStorageStub) Activity() *repositories.ActivityRepository                   { return nil }
func (s *repoStorageStub) Timeline() interfaces.TimelineRepository                      { return nil }
func (s *repoStorageStub) Notification() *repositories.NotificationRepository           { return nil }
func (s *repoStorageStub) Like() *repositories.LikeRepository                           { return nil }
func (s *repoStorageStub) Moderation() *repositories.ModerationRepository               { return nil }
func (s *repoStorageStub) List() *repositories.ListRepository                           { return nil }
func (s *repoStorageStub) Media() *repositories.MediaRepository                         { return nil }
func (s *repoStorageStub) MediaMetadata() *repositories.MediaMetadataRepository         { return nil }
func (s *repoStorageStub) Poll() *repositories.PollRepository                           { return nil }
func (s *repoStorageStub) PushSubscription() *repositories.PushSubscriptionRepository   { return nil }
func (s *repoStorageStub) Hashtag() *repositories.HashtagRepository                     { return nil }
func (s *repoStorageStub) ScheduledStatus() *repositories.ScheduledStatusRepository     { return nil }
func (s *repoStorageStub) Announcement() *repositories.AnnouncementRepository           { return nil }
func (s *repoStorageStub) DomainBlock() *repositories.DomainBlockRepository             { return nil }
func (s *repoStorageStub) Relationship() *repositories.RelationshipRepository           { return nil }
func (s *repoStorageStub) Instance() *repositories.InstanceRepository                   { return nil }
func (s *repoStorageStub) Recovery() *repositories.RecoveryRepository                   { return nil }
func (s *repoStorageStub) Analytics() *repositories.TrendingRepository                  { return nil }
func (s *repoStorageStub) Social() *repositories.SocialRepository                       { return nil }
func (s *repoStorageStub) User() interfaces.UserRepository                              { return nil }
func (s *repoStorageStub) Status() interfaces.StatusRepository                       { return nil }
func (s *repoStorageStub) Cost() *repositories.TrackingRepository                       { return nil }
func (s *repoStorageStub) WebSocketCost() *repositories.WebSocketCostRepository         { return nil }
func (s *repoStorageStub) Trust() *repositories.TrustRepository                         { return nil }
func (s *repoStorageStub) Search() *repositories.SearchRepository                       { return nil }
func (s *repoStorageStub) Relay() *repositories.RelayRepository                         { return nil }
func (s *repoStorageStub) CommunityNote() *repositories.CommunityNoteRepository         { return nil }
func (s *repoStorageStub) Emoji() *repositories.EmojiRepository                         { return nil }
func (s *repoStorageStub) RateLimit() *repositories.RateLimitRepository                 { return nil }
func (s *repoStorageStub) Conversation() *repositories.ConversationRepository           { return nil }
func (s *repoStorageStub) Marker() *repositories.MarkerRepository                       { return nil }
func (s *repoStorageStub) FeaturedTag() *repositories.FeaturedTagRepository             { return nil }
func (s *repoStorageStub) AI() *repositories.AIRepository                               { return nil }
func (s *repoStorageStub) Export() *repositories.ExportRepository                       { return nil }
func (s *repoStorageStub) Import() *repositories.ImportRepository                       { return nil }
func (s *repoStorageStub) DLQ() *repositories.DLQRepository                             { return nil }
func (s *repoStorageStub) MetricRecord() *repositories.MetricRecordRepository           { return nil }
func (s *repoStorageStub) CloudWatchMetrics() *repositories.CloudWatchMetricsRepository { return nil }
func (s *repoStorageStub) StreamingCloudWatch() *repositories.StreamingCloudWatchRepository {
	return nil
}
func (s *repoStorageStub) Audit() *repositories.AuditRepository                     { return nil }
func (s *repoStorageStub) OAuth() *repositories.OAuthRepository                     { return nil }
func (s *repoStorageStub) DNSCache() *repositories.DNSCacheRepository               { return nil }
func (s *repoStorageStub) Filter() *repositories.FilterRepository                   { return nil }
func (s *repoStorageStub) Thread() *repositories.ThreadRepository                   { return nil }
func (s *repoStorageStub) Severance() *repositories.SeveranceRepository             { return nil }
func (s *repoStorageStub) ModerationML() *repositories.ModerationMLRepository       { return nil }
func (s *repoStorageStub) Quote() *repositories.QuoteRepository                     { return nil }
func (s *repoStorageStub) MediaAnalytics() *repositories.MediaAnalyticsRepository   { return nil }
func (s *repoStorageStub) MediaPopularity() *repositories.MediaPopularityRepository { return nil }
func (s *repoStorageStub) MediaSession() *repositories.MediaSessionRepository       { return nil }
func (s *repoStorageStub) StreamingConnection() *repositories.StreamingConnectionRepository {
	return nil
}
func (s *repoStorageStub) Article() *repositories.ArticleRepository                     { return nil }
func (s *repoStorageStub) Draft() *repositories.DraftRepository                         { return nil }
func (s *repoStorageStub) Revision() *repositories.RevisionRepository                   { return nil }
func (s *repoStorageStub) Series() *repositories.SeriesRepository                       { return nil }
func (s *repoStorageStub) Category() *repositories.CategoryRepository                   { return nil }
func (s *repoStorageStub) Publication() *repositories.PublicationRepository             { return nil }
func (s *repoStorageStub) PublicationMember() *repositories.PublicationMemberRepository { return nil }

func newTrackerHarness(t *testing.T, db *memDB) (*RelationshipTracker, *repoStorageStub) {
	t.Helper()

	logger := zap.NewNop()
	federationRepo := repositories.NewFederationRepository(db, "test-table", logger, nil, appConfig.Get())
	store := &repoStorageStub{
		federation: federationRepo,
		db:         db,
		tableName:  "test-table",
		logger:     logger,
	}

	return NewRelationshipTracker(store, db, logger), store
}

func TestRelationshipTracker_CoverageHappyPaths(t *testing.T) {
	t.Parallel()

	db := newMemDB()
	tracker, _ := newTrackerHarness(t, db)
	ctx := context.Background()

	// Track a delivery attempt end-to-end.
	err := tracker.TrackDeliveryAttempt(ctx, &DeliveryAttempt{
		SourceDomain:   "source.example",
		TargetDomain:   "target.example",
		ActivityType:   "follow",
		Success:        true,
		ResponseTimeMs: 125,
		Timestamp:      time.Now(),
		UserID:         "user-1",
	})
	require.NoError(t, err)

	// Missing user ID should skip user-level tracking without failing.
	err = tracker.TrackDeliveryAttempt(ctx, &DeliveryAttempt{
		SourceDomain:   "source.example",
		TargetDomain:   "target.example",
		ActivityType:   "follow",
		Success:        false,
		ResponseTimeMs: 0,
		Timestamp:      time.Now(),
		UserID:         "",
	})
	require.NoError(t, err)

	// Existing IDLE relationship should transition to ACTIVE after new activity.
	idleRel := models.FederationRelationship{
		ID:               tracker.generateRelationshipID("idle-user", "idle-target.example", "follow"),
		UserID:           "idle-user",
		TargetInstance:   "idle-target.example",
		RelationshipType: "follow",
		State:            models.StateIdle,
		LastActivity:     time.Now().Add(-60 * 24 * time.Hour),
		FirstSeen:        time.Now().Add(-60 * 24 * time.Hour),
		StateChangedAt:   time.Now().Add(-60 * 24 * time.Hour),
		WindowStart15m:   time.Now(),
		CreatedAt:        time.Now().Add(-60 * 24 * time.Hour),
		UpdatedAt:        time.Now().Add(-60 * 24 * time.Hour),
	}
	idleRel.UpdateKeys()
	db.relationships[idleRel.PK+"|"+idleRel.SK] = idleRel

	err = tracker.TrackDeliveryAttempt(ctx, &DeliveryAttempt{
		SourceDomain:   "source.example",
		TargetDomain:   "idle-target.example",
		ActivityType:   "follow",
		Success:        true,
		ResponseTimeMs: 25,
		Timestamp:      time.Now(),
		UserID:         "idle-user",
	})
	require.NoError(t, err)

	// Track an inbound activity end-to-end.
	err = tracker.TrackInboundActivity(ctx, &InboundActivity{
		SourceDomain: "remote.example",
		TargetDomain: "local.example",
		ActivityType: "mention",
		Timestamp:    time.Now(),
		UserID:       "user-2",
	})
	require.NoError(t, err)

	// Analyze relationship strength with and without edges.
	analysis, err := tracker.AnalyzeRelationshipStrength(ctx, "missing-a", "missing-b")
	require.NoError(t, err)
	require.Equal(t, "dormant", analysis.RelationshipType)

	// Seed edges for analysis.
	now := time.Now()
	for _, edge := range []*storage.FederationEdge{
		{
			SourceDomain:   "a.example",
			TargetDomain:   "b.example",
			ConnectionType: "all",
			VolumeOut:      110,
			Strength:       0.9,
			LastActivity:   now,
		},
		{
			SourceDomain:   "b.example",
			TargetDomain:   "a.example",
			ConnectionType: "all",
			VolumeOut:      90,
			Strength:       0.4,
			LastActivity:   now,
		},
	} {
		require.NoError(t, tracker.storage.Federation().UpdateFederationEdge(ctx, edge))
	}

	analysis, err = tracker.AnalyzeRelationshipStrength(ctx, "a.example", "b.example")
	require.NoError(t, err)
	require.Equal(t, int64(200), analysis.TotalVolume)
	require.Equal(t, "mutual", analysis.RelationshipType)

	// Populate instance connections for recommendations (repository uses gsi2PK=CONNECTION#domain).
	var modelConnections []models.InstanceConnection
	for i := 0; i < 11; i++ {
		modelConnections = append(modelConnections, models.InstanceConnection{
			Domain:         "local.example",
			TargetDomain:   fmt.Sprintf("t%d.example", i),
			ConnectionType: "all",
			Direction:      "outbound",
			VolumeIn:       0,
			VolumeOut:      1, // low volume
			LastActivity:   now,
			Success:        i%2 == 0,
		})
	}
	db.instanceConnectionsByGSI2PK["CONNECTION#local.example"] = modelConnections

	// Add a strong federation edge for an underutilized domain.
	edgeModel := models.FederationEdge{
		SourceDomain:   "other.example",
		TargetDomain:   "popular.example",
		ConnectionType: "all",
		Strength:       0.9,
		LastActivity:   now,
	}
	edgeModel.UpdateKeys()
	db.fedEdges[edgeModel.PK+"|"+edgeModel.SK] = edgeModel

	recs, err := tracker.GenerateRecommendations(ctx, "local.example")
	require.NoError(t, err)
	require.NotEmpty(t, recs)

	types := make(map[string]bool)
	for _, rec := range recs {
		types[rec.Type] = true
	}
	require.True(t, types["performance"])
	require.True(t, types["opportunity"])
	require.True(t, types["cost"])
}

func TestRelationshipTracker_StateTransitionsAndArchival(t *testing.T) {
	t.Parallel()

	db := newMemDB()
	tracker, _ := newTrackerHarness(t, db)
	ctx := context.Background()

	// Seed relationships in ACTIVE/IDLE/DORMANT that need transitions.
	active := models.FederationRelationship{
		ID:               "rel1",
		UserID:           "u1",
		TargetInstance:   "x.example",
		RelationshipType: "follow",
		State:            models.StateActive,
		LastActivity:     time.Now().Add(-8 * 24 * time.Hour),
		FirstSeen:        time.Now().Add(-9 * 24 * time.Hour),
		WindowStart15m:   time.Now().Add(-16 * time.Minute),
		CreatedAt:        time.Now().Add(-9 * 24 * time.Hour),
		UpdatedAt:        time.Now().Add(-8 * 24 * time.Hour),
	}
	active.UpdateKeys()

	idle := models.FederationRelationship{
		ID:               "rel2",
		UserID:           "u2",
		TargetInstance:   "y.example",
		RelationshipType: "follow",
		State:            models.StateIdle,
		LastActivity:     time.Now().Add(-24 * time.Hour),
		FirstSeen:        time.Now().Add(-40 * 24 * time.Hour),
		WindowStart15m:   time.Now(),
		CreatedAt:        time.Now().Add(-40 * 24 * time.Hour),
		UpdatedAt:        time.Now().Add(-24 * time.Hour),
	}
	idle.UpdateKeys()

	dormant := models.FederationRelationship{
		ID:               "rel3",
		UserID:           "u3",
		TargetInstance:   "z.example",
		RelationshipType: "follow",
		State:            models.StateDormant,
		LastActivity:     time.Now().Add(-100 * 24 * time.Hour),
		FirstSeen:        time.Now().Add(-200 * 24 * time.Hour),
		WindowStart15m:   time.Now(),
		CreatedAt:        time.Now().Add(-200 * 24 * time.Hour),
		UpdatedAt:        time.Now().Add(-100 * 24 * time.Hour),
	}
	dormant.UpdateKeys()

	db.relationships[active.PK+"|"+active.SK] = active
	db.relationships[idle.PK+"|"+idle.SK] = idle
	db.relationships[dormant.PK+"|"+dormant.SK] = dormant

	require.NoError(t, tracker.processStateTransitions(ctx))

	// Archival path: configure S3 and add a dormant relationship older than archiveAfter.
	s3Client := newFakeS3()
	tracker.s3Client = s3Client
	tracker.archiveBucket = "archive-bucket"

	toArchive := models.FederationRelationship{
		ID:               "arch1",
		UserID:           "ua",
		TargetInstance:   "archive-target.example",
		RelationshipType: "follow",
		State:            models.StateDormant,
		LastActivity:     time.Now().Add(-100 * 24 * time.Hour),
		FirstSeen:        time.Now().Add(-200 * 24 * time.Hour),
		WindowStart15m:   time.Now(),
		CreatedAt:        time.Now().Add(-200 * 24 * time.Hour),
		UpdatedAt:        time.Now().Add(-100 * 24 * time.Hour),
	}
	toArchive.UpdateKeys()
	db.relationships[toArchive.PK+"|"+toArchive.SK] = toArchive

	require.NoError(t, tracker.archiveDormantRelationships(ctx))

	// Cache cleanup.
	oldRel := &models.FederationRelationship{UpdatedAt: time.Now().Add(-2 * time.Hour)}
	newRel := &models.FederationRelationship{UpdatedAt: time.Now()}
	tracker.cacheMutex.Lock()
	tracker.relationshipCache["old"] = oldRel
	tracker.relationshipCache["new"] = newRel
	tracker.cacheMutex.Unlock()

	tracker.cleanCache()
	tracker.cacheMutex.RLock()
	_, hasOld := tracker.relationshipCache["old"]
	_, hasNew := tracker.relationshipCache["new"]
	tracker.cacheMutex.RUnlock()
	require.False(t, hasOld)
	require.True(t, hasNew)
}

func TestRelationshipTracker_PublicAPIAndS3RestorePaths(t *testing.T) {
	t.Parallel()

	db := newMemDB()
	tracker, _ := newTrackerHarness(t, db)
	ctx := context.Background()

	// GetSuccessRate: uses aggregate (not found -> optimistic default on create path sets OverallSuccessRate to 0).
	rate, err := tracker.GetSuccessRate(ctx, "rate.example")
	require.NoError(t, err)
	require.GreaterOrEqual(t, rate, 0.0)

	// GetRelationshipByID: creates on miss, and caches.
	rel, err := tracker.GetRelationshipByID(ctx, "u1", "t.example", "follow")
	require.NoError(t, err)
	require.Equal(t, "u1", rel.UserID)

	// ForceStateTransition: transitions and saves.
	require.NoError(t, tracker.ForceStateTransition(ctx, "u1", "t.example", "follow", models.StateDormant))

	// GetUserRelationships / GetRelationshipsByState.
	userRels, err := tracker.GetUserRelationships(ctx, "u1", 10)
	require.NoError(t, err)
	require.NotEmpty(t, userRels)

	// GetInstanceAggregate: empty period should default to 15min.
	aggResult, err := tracker.GetInstanceAggregate(ctx, "agg.example", "")
	require.NoError(t, err)
	require.Equal(t, "15min", aggResult.Period)

	stateRels, err := tracker.GetRelationshipsByState(ctx, models.StateDormant, 10)
	require.NoError(t, err)
	require.NotEmpty(t, stateRels)

	// GetHealthScore: seed aggregate response time to apply penalty.
	agg := models.FederationRelationshipAggregate{
		InstanceDomain:     "health.example",
		Period:             "15min",
		Timestamp:          time.Now().Truncate(15 * time.Minute),
		OverallSuccessRate: 0.9,
		AvgResponseTime:    5000,
		StateTransitions:   map[string]int64{},
		CreatedAt:          time.Now(),
		TotalSuccesses15m:  9,
		TotalFailures15m:   1,
	}
	agg.UpdateKeys()
	db.aggregates[agg.PK+"|"+agg.SK] = agg

	score, err := tracker.GetHealthScore(ctx, "health.example")
	require.NoError(t, err)
	require.Less(t, score, 100.0)

	// GetUnhealthyRelationships.
	unhealthyRel := models.FederationRelationship{
		ID:               "unhealthy-1",
		UserID:           "u-unhealthy",
		TargetInstance:   "bad.example",
		RelationshipType: "follow",
		State:            models.StateActive,
		LastActivity:     time.Now(),
		FirstSeen:        time.Now().Add(-24 * time.Hour),
		WindowStart15m:   time.Now(),
		SuccessRate:      0.2,
		CreatedAt:        time.Now().Add(-24 * time.Hour),
		UpdatedAt:        time.Now(),
	}
	unhealthyRel.UpdateKeys()
	db.relationships[unhealthyRel.PK+"|"+unhealthyRel.SK] = unhealthyRel

	unhealthy, err := tracker.GetUnhealthyRelationships(ctx, 0.8, 5)
	require.NoError(t, err)
	require.NotEmpty(t, unhealthy)

	// ReactivateRelationship:
	// 1) Not found index -> ForceStateTransition path.
	require.NoError(t, tracker.ReactivateRelationship(ctx, "noindex-user", "noindex-target", "follow"))

	// 2) Index found but restore fails -> fallback baseline path (no S3 configured).
	indexID := tracker.generateRelationshipID("fallback-user", "fallback-target", "follow")
	index := models.FederationRelationshipIndex{
		RelationshipID:  indexID,
		UserID:          "fallback-user",
		TargetInstance:  "fallback-target",
		State:           models.StateArchived,
		LastActivity:    time.Now().Add(-200 * 24 * time.Hour),
		ArchiveLocation: "missing-object.gz",
		CreatedAt:       time.Now().Add(-200 * 24 * time.Hour),
	}
	index.UpdateKeys()
	db.indexes[index.PK+"|"+index.SK] = index
	require.NoError(t, tracker.ReactivateRelationship(ctx, "fallback-user", "fallback-target", "follow"))

	// 3) Index found and restore succeeds.
	s3Client := newFakeS3()
	tracker.s3Client = s3Client
	tracker.archiveBucket = "archive-bucket"

	archivedRel := &models.FederationRelationship{
		ID:               tracker.generateRelationshipID("restore-user", "restore-target", "follow"),
		UserID:           "restore-user",
		TargetInstance:   "restore-target",
		RelationshipType: "follow",
		State:            models.StateArchived,
		LastActivity:     time.Now().Add(-120 * 24 * time.Hour),
		FirstSeen:        time.Now().Add(-200 * 24 * time.Hour),
		WindowStart15m:   time.Now().Add(-20 * time.Minute),
		CreatedAt:        time.Now().Add(-200 * 24 * time.Hour),
		UpdatedAt:        time.Now().Add(-120 * 24 * time.Hour),
	}
	archivedRel.UpdateKeys()
	require.NoError(t, tracker.archiveToS3(ctx, archivedRel))

	restoreIndex := models.FederationRelationshipIndex{
		RelationshipID:  archivedRel.ID,
		UserID:          archivedRel.UserID,
		TargetInstance:  archivedRel.TargetInstance,
		State:           models.StateArchived,
		LastActivity:    archivedRel.LastActivity,
		ArchiveLocation: archivedRel.ArchiveLocation,
		CreatedAt:       archivedRel.CreatedAt,
	}
	restoreIndex.UpdateKeys()
	db.indexes[restoreIndex.PK+"|"+restoreIndex.SK] = restoreIndex

	require.NoError(t, tracker.ReactivateRelationship(ctx, "restore-user", "restore-target", "follow"))
}

func TestRelationshipTracker_S3RetriesRespectContextCancellation(t *testing.T) {
	t.Parallel()

	db := newMemDB()
	tracker, _ := newTrackerHarness(t, db)

	s3Client := newFakeS3()
	s3Client.putErr = errors.New("put failed")
	s3Client.getErr = errors.New("get failed")
	tracker.s3Client = s3Client
	tracker.archiveBucket = "archive-bucket"

	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel()

	rel := &models.FederationRelationship{
		ID:               "retry1",
		UserID:           "u",
		TargetInstance:   "t",
		RelationshipType: "follow",
		State:            models.StateDormant,
		LastActivity:     time.Now().Add(-100 * 24 * time.Hour),
		FirstSeen:        time.Now().Add(-200 * 24 * time.Hour),
		WindowStart15m:   time.Now(),
		CreatedAt:        time.Now().Add(-200 * 24 * time.Hour),
		UpdatedAt:        time.Now().Add(-100 * 24 * time.Hour),
	}
	rel.UpdateKeys()

	// First attempt fails, second attempt should observe context cancellation without sleeping.
	_ = tracker.archiveToS3(canceledCtx, rel)
	_, _ = tracker.restoreFromS3(canceledCtx, "missing-key")
}

func TestNewRelationshipTrackerWithS3_SetsBucketAndClient(t *testing.T) {
	t.Parallel()

	db := newMemDB()
	logger := zap.NewNop()
	federationRepo := repositories.NewFederationRepository(db, "test-table", logger, nil, appConfig.Get())
	store := &repoStorageStub{
		federation: federationRepo,
		db:         db,
		tableName:  "test-table",
		logger:     logger,
	}

	rt := NewRelationshipTrackerWithS3(store, db, logger, s3.New(s3.Options{}), "archive-bucket")
	require.Equal(t, "archive-bucket", rt.archiveBucket)
	require.NotNil(t, rt.s3Client)
}

func TestRelationshipTracker_BatchArchiveAndBatchRestore(t *testing.T) {
	t.Parallel()

	db := newMemDB()
	tracker, _ := newTrackerHarness(t, db)
	ctx := context.Background()

	s3Client := newFakeS3()
	tracker.s3Client = s3Client
	tracker.archiveBucket = "archive-bucket"

	rels := []models.FederationRelationship{
		{
			ID:               "b1",
			UserID:           "u1",
			TargetInstance:   "one.example",
			RelationshipType: "follow",
			State:            models.StateDormant,
			LastActivity:     time.Now().Add(-100 * 24 * time.Hour),
			FirstSeen:        time.Now().Add(-200 * 24 * time.Hour),
			WindowStart15m:   time.Now(),
			CreatedAt:        time.Now().Add(-200 * 24 * time.Hour),
			UpdatedAt:        time.Now().Add(-100 * 24 * time.Hour),
		},
		{
			ID:               "b2",
			UserID:           "u2",
			TargetInstance:   "one.example",
			RelationshipType: "follow",
			State:            models.StateDormant,
			LastActivity:     time.Now().Add(-101 * 24 * time.Hour),
			FirstSeen:        time.Now().Add(-201 * 24 * time.Hour),
			WindowStart15m:   time.Now(),
			CreatedAt:        time.Now().Add(-201 * 24 * time.Hour),
			UpdatedAt:        time.Now().Add(-101 * 24 * time.Hour),
		},
		{
			ID:               "b3",
			UserID:           "u3",
			TargetInstance:   "two.example",
			RelationshipType: "follow",
			State:            models.StateDormant,
			LastActivity:     time.Now().Add(-99 * 24 * time.Hour),
			FirstSeen:        time.Now().Add(-199 * 24 * time.Hour),
			WindowStart15m:   time.Now(),
			CreatedAt:        time.Now().Add(-199 * 24 * time.Hour),
			UpdatedAt:        time.Now().Add(-99 * 24 * time.Hour),
		},
	}

	// Batch archive exercises grouping + archiveInstanceGroup.
	require.NoError(t, tracker.BatchArchiveToS3(ctx, rels))

	var archiveKeys []string
	s3Client.mu.Lock()
	for key := range s3Client.objects {
		archiveKeys = append(archiveKeys, key)
	}
	s3Client.mu.Unlock()
	require.NotEmpty(t, archiveKeys)

	// Restore multiple from one of the batch archives.
	restored, err := tracker.restoreMultipleFromS3(ctx, archiveKeys[0])
	require.NoError(t, err)
	require.NotEmpty(t, restored)

	// Exported wrapper also hits archiveInstanceGroup.
	require.NoError(t, tracker.ArchiveInstanceGroup(ctx, "wrapper.example", rels))

	// Batch restore exercises BatchRestoreRelationships + BatchCreate + cleanup warning path.
	s3Client.deleteErr = errors.New("delete failed")
	require.NoError(t, tracker.BatchRestoreRelationships(ctx, archiveKeys[:1]))
	require.NotEmpty(t, db.relationships)

	// cleanupS3Archive success path.
	s3Client.deleteErr = nil
	require.NoError(t, tracker.cleanupS3Archive(ctx, archiveKeys[0]))
}

func TestRelationshipTracker_ClassifyRelationship_AllBranches(t *testing.T) {
	t.Parallel()

	rt := &RelationshipTracker{}

	cases := []struct {
		name string
		in   *RelationshipAnalysis
		want string
	}{
		{
			name: "dormant",
			in:   &RelationshipAnalysis{TotalVolume: 0},
			want: "dormant",
		},
		{
			name: "mutual",
			in:   &RelationshipAnalysis{TotalVolume: 200, Reciprocity: 0.9},
			want: "mutual",
		},
		{
			name: "outbound_focused",
			in:   &RelationshipAnalysis{TotalVolume: 120, InboundVolume: 10, OutboundVolume: 100, Reciprocity: 0.2},
			want: "outbound_focused",
		},
		{
			name: "inbound_focused",
			in:   &RelationshipAnalysis{TotalVolume: 120, InboundVolume: 100, OutboundVolume: 10, Reciprocity: 0.2},
			want: "inbound_focused",
		},
		{
			name: "active",
			in:   &RelationshipAnalysis{TotalVolume: 600, InboundVolume: 300, OutboundVolume: 300, Reciprocity: 0.5},
			want: "active",
		},
		{
			name: "casual",
			in:   &RelationshipAnalysis{TotalVolume: 200, InboundVolume: 120, OutboundVolume: 80, Reciprocity: 0.2},
			want: "casual",
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, rt.classifyRelationship(tt.in))
		})
	}

	// mathMin branch coverage.
	require.Equal(t, int64(1), mathMin(1, 2))
	require.Equal(t, int64(1), mathMin(2, 1))
}

func TestRelationshipTracker_ErrorBranches(t *testing.T) {
	t.Parallel()

	t.Run("calculateSuccessRate falls back on aggregate error", func(t *testing.T) {
		db := newMemDB()
		db.hookFirst = func(_ *memQuery, dest any) error {
			if _, ok := dest.(*models.FederationRelationshipAggregate); ok {
				return errors.New("boom")
			}
			return nil
		}
		tracker, _ := newTrackerHarness(t, db)

		rate, err := tracker.GetSuccessRate(context.Background(), "x.example")
		require.NoError(t, err)
		require.Equal(t, 0.5, rate)
	})

	t.Run("GetUserRelationships returns joined error on scan failure", func(t *testing.T) {
		db := newMemDB()
		db.hookScan = func(_ *memQuery, dest any) error {
			if _, ok := dest.(*[]models.FederationRelationship); ok {
				return errors.New("scan failed")
			}
			return nil
		}
		tracker, _ := newTrackerHarness(t, db)

		_, err := tracker.GetUserRelationships(context.Background(), "u", 10)
		require.Error(t, err)
	})

	t.Run("GetRelationshipsByState returns joined error on scan failure", func(t *testing.T) {
		db := newMemDB()
		db.hookScan = func(_ *memQuery, dest any) error {
			if _, ok := dest.(*[]models.FederationRelationship); ok {
				return errors.New("scan failed")
			}
			return nil
		}
		tracker, _ := newTrackerHarness(t, db)

		_, err := tracker.GetRelationshipsByState(context.Background(), models.StateActive, 10)
		require.Error(t, err)
	})

	t.Run("ForceStateTransition surfaces get and save errors", func(t *testing.T) {
		ctx := context.Background()

		db := newMemDB()
		db.hookFirst = func(_ *memQuery, dest any) error {
			if _, ok := dest.(*models.FederationRelationship); ok {
				return errors.New("first failed")
			}
			return nil
		}
		tracker, _ := newTrackerHarness(t, db)
		require.Error(t, tracker.ForceStateTransition(ctx, "u", "t", "follow", models.StateDormant))

		db2 := newMemDB()
		db2.hookCreate = func(q *memQuery) error {
			if _, ok := q.model.(*models.FederationRelationship); ok {
				return errors.New("create failed")
			}
			return nil
		}
		tracker2, _ := newTrackerHarness(t, db2)
		require.Error(t, tracker2.ForceStateTransition(ctx, "u", "t", "follow", models.StateDormant))
	})

	t.Run("ReactivateRelationship surfaces index lookup error", func(t *testing.T) {
		db := newMemDB()
		db.hookFirst = func(_ *memQuery, dest any) error {
			if _, ok := dest.(*models.FederationRelationshipIndex); ok {
				return errors.New("index failed")
			}
			return nil
		}
		tracker, _ := newTrackerHarness(t, db)
		require.Error(t, tracker.ReactivateRelationship(context.Background(), "u", "t", "follow"))
	})

	t.Run("processStateTransitions continues on scan error", func(t *testing.T) {
		db := newMemDB()
		db.hookScan = func(q *memQuery, dest any) error {
			cond, ok := q.where["gsi1PK"]
			if ok && cond.op == "=" && cond.value == fmt.Sprintf("FEDERATION_STATE#%s", models.StateActive) {
				return errors.New("scan failed")
			}
			return nil
		}
		tracker, _ := newTrackerHarness(t, db)
		require.NoError(t, tracker.processStateTransitions(context.Background()))
	})

	t.Run("archiveDormantRelationships returns joined error on scan failure", func(t *testing.T) {
		db := newMemDB()
		db.hookScan = func(_ *memQuery, dest any) error {
			if _, ok := dest.(*[]models.FederationRelationship); ok {
				return errors.New("scan failed")
			}
			return nil
		}
		tracker, _ := newTrackerHarness(t, db)
		require.Error(t, tracker.archiveDormantRelationships(context.Background()))
	})

	t.Run("archiveDormantRelationships continues on S3/index/delete errors", func(t *testing.T) {
		db := newMemDB()
		tracker, _ := newTrackerHarness(t, db)

		s3Client := newFakeS3()
		s3Client.putErr = errors.New("put failed")
		tracker.s3Client = s3Client
		tracker.archiveBucket = "archive-bucket"

		rel := models.FederationRelationship{
			ID:               "err-archive-1",
			UserID:           "u",
			TargetInstance:   "t",
			RelationshipType: "follow",
			State:            models.StateDormant,
			LastActivity:     time.Now().Add(-200 * 24 * time.Hour),
			FirstSeen:        time.Now().Add(-200 * 24 * time.Hour),
			WindowStart15m:   time.Now(),
			CreatedAt:        time.Now().Add(-200 * 24 * time.Hour),
			UpdatedAt:        time.Now().Add(-200 * 24 * time.Hour),
		}
		rel.UpdateKeys()
		db.relationships[rel.PK+"|"+rel.SK] = rel

		canceledCtx, cancel := context.WithCancel(context.Background())
		cancel()
		require.NoError(t, tracker.archiveDormantRelationships(canceledCtx))

		// Index create failure should be logged and skipped.
		db2 := newMemDB()
		tracker2, _ := newTrackerHarness(t, db2)
		tracker2.s3Client = newFakeS3()
		tracker2.archiveBucket = "archive-bucket"
		rel2 := rel
		rel2.ID = "err-archive-2"
		rel2.UpdateKeys()
		db2.relationships[rel2.PK+"|"+rel2.SK] = rel2
		db2.hookCreate = func(q *memQuery) error {
			if _, ok := q.model.(*models.FederationRelationshipIndex); ok {
				return errors.New("index create failed")
			}
			return nil
		}
		require.NoError(t, tracker2.archiveDormantRelationships(context.Background()))

		// Delete failure should be logged.
		db3 := newMemDB()
		tracker3, _ := newTrackerHarness(t, db3)
		tracker3.s3Client = newFakeS3()
		tracker3.archiveBucket = "archive-bucket"
		rel3 := rel
		rel3.ID = "err-archive-3"
		rel3.UpdateKeys()
		db3.relationships[rel3.PK+"|"+rel3.SK] = rel3
		db3.hookDelete = func(q *memQuery) error {
			if _, ok := q.model.(*models.FederationRelationship); ok {
				return errors.New("delete failed")
			}
			return nil
		}
		require.NoError(t, tracker3.archiveDormantRelationships(context.Background()))
	})

	t.Run("S3 restore failures exercise error branches without retry sleeps", func(t *testing.T) {
		db := newMemDB()
		tracker, _ := newTrackerHarness(t, db)

		s3Client := newFakeS3()
		tracker.s3Client = s3Client
		tracker.archiveBucket = "archive-bucket"

		// Empty relationship list should return ErrArchiveContainsNoRelationships without retries.
		emptyKey := "empty.json.gz"
		s3Client.objects[emptyKey] = gzipBytes(`{"relationships":[],"metadata":{}}`)
		_, err := tracker.restoreFromS3(context.Background(), emptyKey)
		require.ErrorIs(t, err, ErrArchiveContainsNoRelationships)

		// Invalid gzip and invalid JSON branches: cancel context to avoid backoff sleeps.
		canceledCtx, cancel := context.WithCancel(context.Background())
		cancel()
		s3Client.objects["not-gzip"] = []byte("not-gzip")
		_, _ = tracker.restoreFromS3(canceledCtx, "not-gzip")

		s3Client.objects["bad-json.json.gz"] = gzipBytes("not-json")
		_, _ = tracker.restoreFromS3(canceledCtx, "bad-json.json.gz")
	})

	t.Run("restoreMultipleFromS3 covers error branches", func(t *testing.T) {
		db := newMemDB()
		tracker, _ := newTrackerHarness(t, db)

		// Not configured.
		_, err := tracker.restoreMultipleFromS3(context.Background(), "x")
		require.ErrorIs(t, err, ErrS3ClientNotConfigured)

		s3Client := newFakeS3()
		tracker.s3Client = s3Client

		// Missing bucket is treated as not configured.
		_, err = tracker.restoreMultipleFromS3(context.Background(), "x")
		require.ErrorIs(t, err, ErrS3ClientNotConfigured)

		tracker.archiveBucket = "archive-bucket"
		_, err = tracker.restoreMultipleFromS3(context.Background(), "missing")
		require.Error(t, err)

		s3Client.objects["bad-gzip"] = []byte("not-gzip")
		_, err = tracker.restoreMultipleFromS3(context.Background(), "bad-gzip")
		require.Error(t, err)

		s3Client.objects["bad-json.json.gz"] = gzipBytes("not-json")
		_, err = tracker.restoreMultipleFromS3(context.Background(), "bad-json.json.gz")
		require.Error(t, err)

		s3Client.objects["empty.json.gz"] = gzipBytes(`{"relationships":[],"metadata":{}}`)
		_, err = tracker.restoreMultipleFromS3(context.Background(), "empty.json.gz")
		require.ErrorIs(t, err, ErrArchiveContainsNoRelationships)
	})

	t.Run("archiveInstanceGroup put error is surfaced", func(t *testing.T) {
		db := newMemDB()
		tracker, _ := newTrackerHarness(t, db)

		s3Client := newFakeS3()
		s3Client.putErr = errors.New("put failed")
		tracker.s3Client = s3Client
		tracker.archiveBucket = "archive-bucket"

		require.Error(t, tracker.archiveInstanceGroup(context.Background(), "t", []models.FederationRelationship{
			{TargetInstance: "t", LastActivity: time.Now()},
		}))
	})

	t.Run("BatchRestoreRelationships returns error when context is canceled during batch write", func(t *testing.T) {
		db := newMemDB()
		tracker, _ := newTrackerHarness(t, db)

		s3Client := newFakeS3()
		tracker.s3Client = s3Client
		tracker.archiveBucket = "archive-bucket"

		rel := models.FederationRelationship{
			ID:               "r1",
			UserID:           "u",
			TargetInstance:   "t",
			RelationshipType: "follow",
			State:            models.StateDormant,
			LastActivity:     time.Now().Add(-200 * 24 * time.Hour),
			FirstSeen:        time.Now().Add(-200 * 24 * time.Hour),
			WindowStart15m:   time.Now(),
			CreatedAt:        time.Now().Add(-200 * 24 * time.Hour),
			UpdatedAt:        time.Now().Add(-200 * 24 * time.Hour),
		}
		key := "restore.json.gz"
		s3Client.objects[key] = gzipBytes(fmt.Sprintf(`{"relationships":[%s],"metadata":{}}`, mustJSON(t, rel)))

		canceledCtx, cancel := context.WithCancel(context.Background())
		cancel()
		require.Error(t, tracker.BatchRestoreRelationships(canceledCtx, []string{key}))
	})

	t.Run("GetUnhealthyRelationships returns joined error when active scan fails", func(t *testing.T) {
		db := newMemDB()
		db.hookScan = func(_ *memQuery, dest any) error {
			if _, ok := dest.(*[]models.FederationRelationship); ok {
				return errors.New("scan failed")
			}
			return nil
		}
		tracker, _ := newTrackerHarness(t, db)

		_, err := tracker.GetUnhealthyRelationships(context.Background(), 0.9, 5)
		require.Error(t, err)
	})

	t.Run("GetHealthScore clamps bounds", func(t *testing.T) {
		db := newMemDB()
		tracker, _ := newTrackerHarness(t, db)
		ctx := context.Background()

		// Clamp to 100.
		aggHigh := models.FederationRelationshipAggregate{
			InstanceDomain:     "high.example",
			Period:             "15min",
			Timestamp:          time.Now().Truncate(15 * time.Minute),
			OverallSuccessRate: 2.0,
			AvgResponseTime:    0,
			StateTransitions:   map[string]int64{},
			CreatedAt:          time.Now(),
		}
		aggHigh.UpdateKeys()
		db.aggregates[aggHigh.PK+"|"+aggHigh.SK] = aggHigh
		score, err := tracker.GetHealthScore(ctx, "high.example")
		require.NoError(t, err)
		require.Equal(t, 100.0, score)

		// Clamp to 0.
		aggLow := models.FederationRelationshipAggregate{
			InstanceDomain:     "low.example",
			Period:             "15min",
			Timestamp:          time.Now().Truncate(15 * time.Minute),
			OverallSuccessRate: 0.0,
			AvgResponseTime:    999999,
			StateTransitions:   map[string]int64{},
			CreatedAt:          time.Now(),
		}
		aggLow.UpdateKeys()
		db.aggregates[aggLow.PK+"|"+aggLow.SK] = aggLow
		score, err = tracker.GetHealthScore(ctx, "low.example")
		require.NoError(t, err)
		require.Equal(t, 0.0, score)
	})

	t.Run("calculateSuccessRateForConnection covers success branch", func(t *testing.T) {
		rt := &RelationshipTracker{}
		require.Equal(t, 1.0, rt.calculateSuccessRateForConnection(&storage.InstanceConnection{Success: true}))
	})
}

func TestRelationshipTracker_CoverageGaps(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	// TrackDeliveryAttempt / TrackInboundActivity: exercise error logging/warn branches.
	{
		db := newMemDB()
		db.hookCreate = func(_ *memQuery) error { return errors.New("create failed") }
		tracker, _ := newTrackerHarness(t, db)

		require.NoError(t, tracker.TrackDeliveryAttempt(ctx, &DeliveryAttempt{
			SourceDomain:   "source.example",
			TargetDomain:   "target.example",
			ActivityType:   "follow",
			Success:        true,
			ResponseTimeMs: 123,
			Timestamp:      time.Now(),
			UserID:         "u",
		}))

		require.NoError(t, tracker.TrackInboundActivity(ctx, &InboundActivity{
			SourceDomain: "remote.example",
			TargetDomain: "local.example",
			ActivityType: "mention",
			Timestamp:    time.Now(),
			UserID:       "u",
		}))
	}

	// saveAggregate error branch.
	{
		db := newMemDB()
		db.hookCreate = func(q *memQuery) error {
			if _, ok := q.model.(*models.FederationRelationshipAggregate); ok {
				return errors.New("create failed")
			}
			return nil
		}
		tracker, _ := newTrackerHarness(t, db)
		agg := &models.FederationRelationshipAggregate{
			InstanceDomain:   "agg.example",
			Period:           "15min",
			Timestamp:        time.Now().Truncate(15 * time.Minute),
			StateTransitions: map[string]int64{},
			CreatedAt:        time.Now(),
		}
		require.Error(t, tracker.saveAggregate(ctx, agg))
	}

	// trackInstanceRelationship weighted average branch.
	{
		db := newMemDB()
		tracker, _ := newTrackerHarness(t, db)
		require.NoError(t, tracker.TrackDeliveryAttempt(ctx, &DeliveryAttempt{
			SourceDomain:   "s.example",
			TargetDomain:   "t.example",
			ActivityType:   "follow",
			Success:        true,
			ResponseTimeMs: 100,
			Timestamp:      time.Now(),
			UserID:         "u",
		}))
		require.NoError(t, tracker.TrackDeliveryAttempt(ctx, &DeliveryAttempt{
			SourceDomain:   "s.example",
			TargetDomain:   "t.example",
			ActivityType:   "follow",
			Success:        true,
			ResponseTimeMs: 200,
			Timestamp:      time.Now(),
			UserID:         "u",
		}))
	}

	// calculateReciprocity / calculateOverallStrength additional branches.
	{
		rt := &RelationshipTracker{}

		require.Equal(t, 0.0, rt.calculateReciprocity(0, 0))
		require.Equal(t, 0.0, rt.calculateReciprocity(0, 1))
		require.Equal(t, 0.0, rt.calculateReciprocity(1, 0))
		require.Equal(t, 1.0, rt.calculateReciprocity(2, 2))

		now := time.Now()
		_ = rt.calculateOverallStrength(&RelationshipAnalysis{
			TotalVolume:          2000,
			Reciprocity:          0.5,
			LastInboundActivity:  now,
			LastOutboundActivity: now.Add(-1 * time.Hour),
		})
		_ = rt.calculateOverallStrength(&RelationshipAnalysis{
			TotalVolume:          2000,
			Reciprocity:          0.5,
			LastInboundActivity:  now.Add(-1 * time.Hour),
			LastOutboundActivity: now,
		})
	}

	// ReactivateRelationship: ArchiveLocation empty branch.
	{
		db := newMemDB()
		tracker, _ := newTrackerHarness(t, db)

		indexID := tracker.generateRelationshipID("plain-user", "plain-target", "follow")
		index := models.FederationRelationshipIndex{
			RelationshipID: indexID,
			UserID:         "plain-user",
			TargetInstance: "plain-target",
			State:          models.StateArchived,
			LastActivity:   time.Now().Add(-200 * 24 * time.Hour),
			CreatedAt:      time.Now().Add(-200 * 24 * time.Hour),
		}
		index.UpdateKeys()
		db.indexes[index.PK+"|"+index.SK] = index

		require.NoError(t, tracker.ReactivateRelationship(ctx, "plain-user", "plain-target", "follow"))
	}

	// archiveToS3: early returns for nil client / missing bucket.
	{
		db := newMemDB()
		tracker, _ := newTrackerHarness(t, db)

		rel := &models.FederationRelationship{
			ID:               "a",
			UserID:           "u",
			TargetInstance:   "t",
			RelationshipType: "follow",
			State:            models.StateDormant,
			LastActivity:     time.Now().Add(-200 * 24 * time.Hour),
			FirstSeen:        time.Now().Add(-200 * 24 * time.Hour),
			WindowStart15m:   time.Now(),
			CreatedAt:        time.Now().Add(-200 * 24 * time.Hour),
			UpdatedAt:        time.Now().Add(-200 * 24 * time.Hour),
		}
		rel.UpdateKeys()

		require.NoError(t, tracker.archiveToS3(ctx, rel))

		tracker.s3Client = newFakeS3()
		tracker.archiveBucket = ""
		require.NoError(t, tracker.archiveToS3(ctx, rel))
	}
}

func TestRelationshipTracker_SmallBranchCoverage(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	// restoreFromS3 early returns.
	{
		rt := &RelationshipTracker{logger: zap.NewNop()}
		_, err := rt.restoreFromS3(ctx, "x")
		require.ErrorIs(t, err, ErrS3ClientNotConfigured)

		rt.s3Client = newFakeS3()
		rt.archiveBucket = ""
		_, err = rt.restoreFromS3(ctx, "x")
		require.ErrorIs(t, err, ErrS3ClientNotConfigured)
	}

	// cleanupS3Archive early returns.
	{
		rt := &RelationshipTracker{logger: zap.NewNop()}
		require.NoError(t, rt.cleanupS3Archive(ctx, "x"))

		rt.s3Client = newFakeS3()
		rt.archiveBucket = ""
		require.NoError(t, rt.cleanupS3Archive(ctx, "x"))
	}

	// BatchArchiveToS3 / BatchRestoreRelationships early returns.
	{
		rt := &RelationshipTracker{logger: zap.NewNop()}
		require.NoError(t, rt.BatchArchiveToS3(ctx, nil))
		require.NoError(t, rt.BatchRestoreRelationships(ctx, nil))
	}

	// generateCostRecommendations nil branch.
	{
		rt := &RelationshipTracker{}
		require.Nil(t, rt.generateCostRecommendations([]*storage.InstanceConnection{
			{VolumeIn: 100, VolumeOut: 100},
		}))
	}

	// GetHealthScore no-penalty branch.
	{
		db := newMemDB()
		tracker, _ := newTrackerHarness(t, db)

		agg := models.FederationRelationshipAggregate{
			InstanceDomain:     "fast.example",
			Period:             "15min",
			Timestamp:          time.Now().Truncate(15 * time.Minute),
			OverallSuccessRate: 1.0,
			AvgResponseTime:    500,
			StateTransitions:   map[string]int64{},
			CreatedAt:          time.Now(),
		}
		agg.UpdateKeys()
		db.aggregates[agg.PK+"|"+agg.SK] = agg

		score, err := tracker.GetHealthScore(ctx, "fast.example")
		require.NoError(t, err)
		require.Equal(t, 100.0, score)
	}
}

func TestRelationshipTracker_LastStatements(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("trackUserRelationship returns joined error on DB failure", func(t *testing.T) {
		db := newMemDB()
		db.hookFirst = func(_ *memQuery, dest any) error {
			if _, ok := dest.(*models.FederationRelationship); ok {
				return errors.New("db down")
			}
			return nil
		}
		tracker, _ := newTrackerHarness(t, db)
		err := tracker.trackUserRelationship(ctx, &DeliveryAttempt{
			UserID:       "u",
			TargetDomain: "t.example",
			ActivityType: "follow",
			Success:      true,
		})
		require.Error(t, err)
	})

	t.Run("BatchArchiveToS3 logs and continues on per-group failure", func(t *testing.T) {
		db := newMemDB()
		tracker, _ := newTrackerHarness(t, db)

		s3Client := newFakeS3()
		s3Client.putErr = errors.New("put failed")
		tracker.s3Client = s3Client
		tracker.archiveBucket = "archive-bucket"

		require.NoError(t, tracker.BatchArchiveToS3(ctx, []models.FederationRelationship{
			{TargetInstance: "bad.example", LastActivity: time.Now()},
		}))
	})
}

func TestRelationshipTracker_ReactivateRelationship_WarnsOnIndexDeleteFailure(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	db := newMemDB()
	db.hookDelete = func(q *memQuery) error {
		if _, ok := q.model.(*models.FederationRelationshipIndex); ok {
			return errors.New("delete failed")
		}
		return nil
	}

	tracker, _ := newTrackerHarness(t, db)
	tracker.s3Client = newFakeS3()
	tracker.archiveBucket = "archive-bucket"

	archivedRel := &models.FederationRelationship{
		ID:               tracker.generateRelationshipID("warn-user", "warn-target", "follow"),
		UserID:           "warn-user",
		TargetInstance:   "warn-target",
		RelationshipType: "follow",
		State:            models.StateArchived,
		LastActivity:     time.Now().Add(-120 * 24 * time.Hour),
		FirstSeen:        time.Now().Add(-200 * 24 * time.Hour),
		WindowStart15m:   time.Now().Add(-20 * time.Minute),
		CreatedAt:        time.Now().Add(-200 * 24 * time.Hour),
		UpdatedAt:        time.Now().Add(-120 * 24 * time.Hour),
	}
	archivedRel.UpdateKeys()
	require.NoError(t, tracker.archiveToS3(ctx, archivedRel))

	restoreIndex := models.FederationRelationshipIndex{
		RelationshipID:  archivedRel.ID,
		UserID:          archivedRel.UserID,
		TargetInstance:  archivedRel.TargetInstance,
		State:           models.StateArchived,
		LastActivity:    archivedRel.LastActivity,
		ArchiveLocation: archivedRel.ArchiveLocation,
		CreatedAt:       archivedRel.CreatedAt,
	}
	restoreIndex.UpdateKeys()
	db.indexes[restoreIndex.PK+"|"+restoreIndex.SK] = restoreIndex

	require.NoError(t, tracker.ReactivateRelationship(ctx, "warn-user", "warn-target", "follow"))
}

func gzipBytes(s string) []byte {
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	_, _ = w.Write([]byte(s))
	_ = w.Close()
	return buf.Bytes()
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	data, err := json.Marshal(v)
	require.NoError(t, err)
	return string(data)
}

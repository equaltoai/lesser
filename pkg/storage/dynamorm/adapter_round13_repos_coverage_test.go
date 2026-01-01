package dynamorm

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	dynamormCore "github.com/pay-theory/dynamorm/pkg/core"
	dynamormMocks "github.com/pay-theory/dynamorm/pkg/mocks"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap/zaptest"
)

func TestStorageAdapter_ExportedMethods_DoNotPanic_WithPermissiveRepos(t *testing.T) {
	db := newPermissiveDynamormDB(t)
	logger := zaptest.NewLogger(t)

	repoStorage := &permissiveRepositoryStorage{
		SimpleRepositoryStorage: &SimpleRepositoryStorage{
			db:        db,
			tableName: "test-table",
			logger:    logger,
		},
		actor:        repositories.NewActorRepository(db, "test-table", logger),
		activity:     repositories.NewActivityRepository(db, "test-table", logger, nil),
		user:         repositories.NewUserRepository(db, "test-table", logger),
		account:      repositories.NewAccountRepository(db, "test-table", "example.com", logger),
		object:       repositories.NewObjectRepository(db, "test-table", "example.com", logger),
		status:       repositories.NewStatusRepository(db, "test-table", logger, nil),
		timeline:     repositories.NewTimelineRepository(db, "test-table", logger, nil),
		relationship: repositories.NewRelationshipRepository(db, "test-table", logger),
		like:         repositories.NewLikeRepository(db, "test-table", logger),
		notification: repositories.NewNotificationRepository(db, "test-table", logger, nil),
		media:        repositories.NewMediaRepository(db, "test-table", logger, nil),
		mediaMeta:    repositories.NewMediaMetadataRepository(db, "test-table", logger, nil),
		dnsCache:     repositories.NewDNSCacheRepository(db, "test-table", logger, nil),
		analytics:    repositories.NewTrendingRepository(db, logger, nil),
		hashtag:      repositories.NewHashtagRepository(db, "test-table", logger, "example.com"),
		list:         repositories.NewListRepository(db, "test-table", logger, nil),
		scheduled:    repositories.NewScheduledStatusRepository(db, "test-table", logger, nil),
		cost:         repositories.NewTrackingRepository(db, "test-table", logger, nil),
	}

	adapter := NewStorageAdapter(repoStorage)

	adapterValue := reflect.ValueOf(adapter)
	contextType := reflect.TypeOf((*context.Context)(nil)).Elem()

	manualCalls := map[string]func(){
		"GetNotifications": func() { _, _, _ = adapter.GetNotifications(context.Background(), "x", 1, "x") },
		"SearchAll":        func() { _, _, _ = adapter.SearchAll(context.Background(), "x", 1, "x") },
		"SearchStatuses":   func() { _, _, _ = adapter.SearchStatuses(context.Background(), "x", 1, "x") },
		"SearchUsers":      func() { _, _, _ = adapter.SearchUsers(context.Background(), "x", 1, "x") },
		"CreateMediaAttachment": func() {
			_ = adapter.CreateMediaAttachment(context.Background(), &models.Media{MediaID: "m", FileName: "file.txt", FileSize: 1})
		},
		"UpdateMediaAttachment": func() {
			_ = adapter.UpdateMediaAttachment(context.Background(), &models.Media{MediaID: "m", FileName: "file.txt", FileSize: 1})
		},
		"UpdateMediaProcessingStatus": func() {
			_ = adapter.UpdateMediaProcessingStatus(context.Background(), "m", "ready", nil)
			_ = adapter.UpdateMediaProcessingStatus(context.Background(), "m", "failed", map[string]interface{}{"error": "boom"})
		},
		"CreateList": func() {
			_ = adapter.CreateList(context.Background(), &models.List{ID: "list-1", Username: "alice", Title: "t"})
		},
		"UpdateList": func() {
			_ = adapter.UpdateList(context.Background(), &models.List{ID: "list-1", Username: "alice", Title: "t"})
		},
		"CreateScheduledStatus": func() {
			_ = adapter.CreateScheduledStatus(context.Background(), &storage.ScheduledStatus{
				ID:          "sched-1",
				Username:    "alice",
				Content:     "x",
				Visibility:  "public",
				ScheduledAt: time.Now().Add(time.Hour),
			})
		},
		"UpdateScheduledStatus": func() {
			_ = adapter.UpdateScheduledStatus(context.Background(), &storage.ScheduledStatus{ID: "sched-1", Username: "alice"})
		},
		"SetCache": func() {
			_ = adapter.SetCache(context.Background(), "DNS:example.com", &storage.DNSCacheEntry{
				Hostname: "example.com",
				IPs:      []string{"127.0.0.1"},
				TTL:      60,
			}, time.Second)
		},
		"GetCache": func() {
			var dest any
			_ = adapter.GetCache(context.Background(), "DNS:example.com", &dest)
		},
		"DeleteCache": func() { _ = adapter.DeleteCache(context.Background(), "DNS:example.com") },
		"ClearCache":  func() { _ = adapter.ClearCache(context.Background(), "DNS") },
	}

	for i := 0; i < adapterValue.NumMethod(); i++ {
		method := adapterValue.Method(i)
		methodType := method.Type()
		methodName := adapterValue.Type().Method(i).Name
		methodTypeStr := methodType.String()
		methodNumIn := methodType.NumIn()

		if manual, ok := manualCalls[methodName]; ok {
			t.Run(methodName, func(t *testing.T) {
				defer func() {
					if r := recover(); r != nil {
						t.Fatalf("panicked: %v", r)
					}
				}()
				manual()
			})
			continue
		}

		args := make([]reflect.Value, methodType.NumIn())
		for j := 0; j < methodType.NumIn(); j++ {
			args[j] = testValueForType(methodType.In(j), contextType)
		}

		t.Run(methodName, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("panicked: %v (type=%s numIn=%d args=%d)", r, methodTypeStr, methodNumIn, len(args))
				}
			}()

			_ = method.Call(args)
		})
	}
}

type permissiveRepositoryStorage struct {
	*SimpleRepositoryStorage

	actor        interfaces.ActorRepository
	activity     *repositories.ActivityRepository
	user         *repositories.UserRepository
	account      *repositories.AccountRepository
	object       *repositories.ObjectRepository
	status       *repositories.StatusRepository
	timeline     interfaces.TimelineRepository
	relationship *repositories.RelationshipRepository
	like         *repositories.LikeRepository
	notification interfaces.NotificationRepository
	media        *repositories.MediaRepository
	mediaMeta    *repositories.MediaMetadataRepository
	dnsCache     *repositories.DNSCacheRepository
	analytics    *repositories.TrendingRepository
	hashtag      *repositories.HashtagRepository
	list         *repositories.ListRepository
	scheduled    *repositories.ScheduledStatusRepository
	cost         *repositories.TrackingRepository
}

func (s *permissiveRepositoryStorage) Actor() interfaces.ActorRepository          { return s.actor }
func (s *permissiveRepositoryStorage) Activity() *repositories.ActivityRepository { return s.activity }
func (s *permissiveRepositoryStorage) User() interfaces.UserRepository            { return s.user }
func (s *permissiveRepositoryStorage) Account() *repositories.AccountRepository   { return s.account }
func (s *permissiveRepositoryStorage) Object() *repositories.ObjectRepository     { return s.object }
func (s *permissiveRepositoryStorage) Status() interfaces.StatusRepository        { return s.status }
func (s *permissiveRepositoryStorage) Timeline() interfaces.TimelineRepository    { return s.timeline }
func (s *permissiveRepositoryStorage) Relationship() *repositories.RelationshipRepository {
	return s.relationship
}
func (s *permissiveRepositoryStorage) Like() *repositories.LikeRepository { return s.like }
func (s *permissiveRepositoryStorage) Notification() interfaces.NotificationRepository {
	return s.notification
}
func (s *permissiveRepositoryStorage) Media() *repositories.MediaRepository { return s.media }
func (s *permissiveRepositoryStorage) MediaMetadata() *repositories.MediaMetadataRepository {
	return s.mediaMeta
}
func (s *permissiveRepositoryStorage) DNSCache() *repositories.DNSCacheRepository { return s.dnsCache }
func (s *permissiveRepositoryStorage) Analytics() *repositories.TrendingRepository {
	return s.analytics
}
func (s *permissiveRepositoryStorage) Hashtag() *repositories.HashtagRepository { return s.hashtag }
func (s *permissiveRepositoryStorage) List() *repositories.ListRepository       { return s.list }
func (s *permissiveRepositoryStorage) ScheduledStatus() *repositories.ScheduledStatusRepository {
	return s.scheduled
}
func (s *permissiveRepositoryStorage) Cost() *repositories.TrackingRepository { return s.cost }

func newPermissiveDynamormDB(t *testing.T) dynamormCore.DB {
	t.Helper()

	db := new(dynamormMocks.MockDB)
	q := new(dynamormMocks.MockQuery)

	// DB interface
	db.On("WithContext", mock.Anything).Return(db).Maybe()
	db.On("Model", mock.Anything).Return(q).Maybe()
	db.On("Transaction", mock.Anything).Return(nil).Maybe()
	db.On("Migrate").Return(nil).Maybe()
	db.On("AutoMigrate", mock.Anything).Return(nil).Maybe()
	db.On("Close").Return(nil).Maybe()

	// Query interface: allow any chain calls.
	queryType := reflect.TypeOf((*dynamormCore.Query)(nil)).Elem()
	updateBuilderType := reflect.TypeOf((*dynamormCore.UpdateBuilder)(nil)).Elem()
	batchGetBuilderType := reflect.TypeOf((*dynamormCore.BatchGetBuilder)(nil)).Elem()

	for i := 0; i < queryType.NumMethod(); i++ {
		method := queryType.Method(i)

		// Specialized fill behavior for slice-based fetches.
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
			case out0 == queryType:
				q.On(method.Name, args...).Return(q).Maybe()
			case out0 == updateBuilderType:
				q.On(method.Name, args...).Return(noopUpdateBuilder{}).Maybe()
			case out0 == batchGetBuilderType:
				q.On(method.Name, args...).Return(noopBatchGetBuilder{}).Maybe()
			default:
				q.On(method.Name, args...).Return(reflect.Zero(out0).Interface()).Maybe()
			}
		case 2:
			out0 := method.Type.Out(0)
			out1 := method.Type.Out(1)

			// Most 2-value returns end in error.
			if out1 == reflect.TypeOf((*error)(nil)).Elem() {
				zero0 := reflect.Zero(out0).Interface()
				switch {
				case out0 == queryType:
					zero0 = q
				case out0 == updateBuilderType:
					zero0 = noopUpdateBuilder{}
				case out0 == batchGetBuilderType:
					zero0 = noopBatchGetBuilder{}
				}
				q.On(method.Name, args...).Return(zero0, nil).Maybe()
				continue
			}

			// Fallback: return zeros.
			q.On(method.Name, args...).Return(reflect.Zero(out0).Interface(), reflect.Zero(out1).Interface()).Maybe()
		default:
			// Not expected for core.Query, but keep safe.
			returns := make([]any, method.Type.NumOut())
			for i := 0; i < method.Type.NumOut(); i++ {
				returns[i] = reflect.Zero(method.Type.Out(i)).Interface()
			}
			q.On(method.Name, args...).Return(returns...).Maybe()
		}
	}

	return db
}

func fillSlicePointer(dest any) {
	destValue := reflect.ValueOf(dest)
	if !destValue.IsValid() || destValue.Kind() != reflect.Pointer {
		return
	}
	destElem := destValue.Elem()
	if !destElem.IsValid() || destElem.Kind() != reflect.Slice {
		return
	}
	elemType := destElem.Type().Elem()
	sliceValue := reflect.MakeSlice(destElem.Type(), 1, 1)
	if elemType.Kind() == reflect.Pointer {
		sliceValue.Index(0).Set(reflect.New(elemType.Elem()))
	} else {
		sliceValue.Index(0).Set(reflect.Zero(elemType))
	}
	destElem.Set(sliceValue)
}

type noopUpdateBuilder struct{}

func (noopUpdateBuilder) Set(string, any) dynamormCore.UpdateBuilder { return noopUpdateBuilder{} }
func (noopUpdateBuilder) SetIfNotExists(string, any, any) dynamormCore.UpdateBuilder {
	return noopUpdateBuilder{}
}
func (noopUpdateBuilder) Add(string, any) dynamormCore.UpdateBuilder    { return noopUpdateBuilder{} }
func (noopUpdateBuilder) Increment(string) dynamormCore.UpdateBuilder   { return noopUpdateBuilder{} }
func (noopUpdateBuilder) Decrement(string) dynamormCore.UpdateBuilder   { return noopUpdateBuilder{} }
func (noopUpdateBuilder) Remove(string) dynamormCore.UpdateBuilder      { return noopUpdateBuilder{} }
func (noopUpdateBuilder) Delete(string, any) dynamormCore.UpdateBuilder { return noopUpdateBuilder{} }
func (noopUpdateBuilder) AppendToList(string, any) dynamormCore.UpdateBuilder {
	return noopUpdateBuilder{}
}
func (noopUpdateBuilder) PrependToList(string, any) dynamormCore.UpdateBuilder {
	return noopUpdateBuilder{}
}
func (noopUpdateBuilder) RemoveFromListAt(string, int) dynamormCore.UpdateBuilder {
	return noopUpdateBuilder{}
}
func (noopUpdateBuilder) SetListElement(string, int, any) dynamormCore.UpdateBuilder {
	return noopUpdateBuilder{}
}
func (noopUpdateBuilder) Condition(string, string, any) dynamormCore.UpdateBuilder {
	return noopUpdateBuilder{}
}
func (noopUpdateBuilder) OrCondition(string, string, any) dynamormCore.UpdateBuilder {
	return noopUpdateBuilder{}
}
func (noopUpdateBuilder) ConditionExists(string) dynamormCore.UpdateBuilder {
	return noopUpdateBuilder{}
}
func (noopUpdateBuilder) ConditionNotExists(string) dynamormCore.UpdateBuilder {
	return noopUpdateBuilder{}
}
func (noopUpdateBuilder) ConditionVersion(int64) dynamormCore.UpdateBuilder {
	return noopUpdateBuilder{}
}
func (noopUpdateBuilder) ReturnValues(string) dynamormCore.UpdateBuilder { return noopUpdateBuilder{} }
func (noopUpdateBuilder) Execute() error                                 { return nil }
func (noopUpdateBuilder) ExecuteWithResult(any) error                    { return nil }

type noopBatchGetBuilder struct{}

func (noopBatchGetBuilder) Keys([]any) dynamormCore.BatchGetBuilder    { return noopBatchGetBuilder{} }
func (noopBatchGetBuilder) ChunkSize(int) dynamormCore.BatchGetBuilder { return noopBatchGetBuilder{} }
func (noopBatchGetBuilder) ConsistentRead() dynamormCore.BatchGetBuilder {
	return noopBatchGetBuilder{}
}
func (noopBatchGetBuilder) Parallel(int) dynamormCore.BatchGetBuilder { return noopBatchGetBuilder{} }
func (noopBatchGetBuilder) WithRetry(*dynamormCore.RetryPolicy) dynamormCore.BatchGetBuilder {
	return noopBatchGetBuilder{}
}
func (noopBatchGetBuilder) Select(...string) dynamormCore.BatchGetBuilder {
	return noopBatchGetBuilder{}
}
func (noopBatchGetBuilder) OnProgress(dynamormCore.BatchProgressCallback) dynamormCore.BatchGetBuilder {
	return noopBatchGetBuilder{}
}
func (noopBatchGetBuilder) OnError(dynamormCore.BatchChunkErrorHandler) dynamormCore.BatchGetBuilder {
	return noopBatchGetBuilder{}
}
func (noopBatchGetBuilder) Execute(any) error { return nil }

var _ dynamormCore.UpdateBuilder = noopUpdateBuilder{}
var _ dynamormCore.BatchGetBuilder = noopBatchGetBuilder{}

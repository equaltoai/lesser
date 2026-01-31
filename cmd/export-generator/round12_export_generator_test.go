package main

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/factory"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	dynamormCore "github.com/pay-theory/dynamorm/pkg/core"
	"github.com/stretchr/testify/require"
	apptheory "github.com/theory-cloud/apptheory/runtime"
	"go.uber.org/zap"
)

type exportRepoRecorder struct {
	statusCalls []string
	dataByID    map[string]map[string]any
	errByStatus map[string]error
}

func (r *exportRepoRecorder) UpdateExportStatus(_ context.Context, exportID, status string, completionData map[string]any, _ string) error {
	r.statusCalls = append(r.statusCalls, status)
	if completionData != nil {
		if r.dataByID == nil {
			r.dataByID = make(map[string]map[string]any)
		}
		r.dataByID[exportID] = completionData
	}
	if err, ok := r.errByStatus[status]; ok {
		return err
	}
	return nil
}

type costTrackingRepoRecorder struct {
	createCalls int
	createErr   error
}

func (r *costTrackingRepoRecorder) Create(_ context.Context, _ *models.DynamoDBCostRecord) error {
	r.createCalls++
	return r.createErr
}

type budgetUpdaterRecorder struct {
	calls int
	err   error
}

func (r *budgetUpdaterRecorder) UpdateBudgetUsage(_ context.Context, _ string, _ string, _ int64, _ int64) error {
	r.calls++
	return r.err
}

type exportStorageStub struct {
	account      accountRepo
	relationship relationshipRepo
	social       socialRepo
	list         listRepo
	user         userRepo
	object       objectRepo
	activity     activityRepo
	like         likeRepo
	domainBlock  domainBlockRepo
	media        mediaRepo
}

func (s exportStorageStub) Account() accountRepo           { return s.account }
func (s exportStorageStub) Relationship() relationshipRepo { return s.relationship }
func (s exportStorageStub) Social() socialRepo             { return s.social }
func (s exportStorageStub) List() listRepo                 { return s.list }
func (s exportStorageStub) User() userRepo                 { return s.user }
func (s exportStorageStub) Object() objectRepo             { return s.object }
func (s exportStorageStub) Activity() activityRepo         { return s.activity }
func (s exportStorageStub) Like() likeRepo                 { return s.like }
func (s exportStorageStub) DomainBlock() domainBlockRepo   { return s.domainBlock }
func (s exportStorageStub) Media() mediaRepo               { return s.media }

type accountRepoFunc func(ctx context.Context, username string) (*activitypub.Actor, error)

func (f accountRepoFunc) GetActor(ctx context.Context, username string) (*activitypub.Actor, error) {
	return f(ctx, username)
}

type relationshipRepoStub struct{}

func (relationshipRepoStub) GetFollowers(_ context.Context, _ string, _ int, cursor string) ([]string, string, error) {
	if cursor == "" {
		return []string{"https://remote.example/users/bob"}, "next", nil
	}
	return []string{"https://remote.example/users/carol"}, "", nil
}

func (relationshipRepoStub) GetFollowing(_ context.Context, _ string, _ int, cursor string) ([]string, string, error) {
	if cursor == "" {
		return []string{"https://remote.example/users/dana"}, "next", nil
	}
	return []string{"https://remote.example/users/erin"}, "", nil
}

type socialRepoStub struct{}

func (socialRepoStub) GetBlockedUsers(_ context.Context, _ string, _ int, _ string) ([]*storage.Block, string, error) {
	return []*storage.Block{{Object: "https://remote.example/users/blocked"}}, "", nil
}

func (socialRepoStub) GetMutedUsers(_ context.Context, _ string, _ int, _ string) ([]*storage.Mute, string, error) {
	return []*storage.Mute{{Object: "https://remote.example/users/muted", HideNotifications: true}}, "", nil
}

type listRepoStub struct{}

func (listRepoStub) GetListsForUser(_ context.Context, _ string) ([]*storage.List, error) {
	return []*storage.List{
		{ID: "list-1", Title: "friends"},
		{ID: "bad-list", Title: "broken"},
	}, nil
}

func (listRepoStub) GetListAccounts(_ context.Context, listID string) ([]string, error) {
	if listID == "bad-list" {
		return nil, errors.New("list accounts query failed")
	}
	return []string{"https://remote.example/users/alice"}, nil
}

type userRepoStub struct{}

func (userRepoStub) GetBookmarks(_ context.Context, _ string, _ int, cursor string) ([]string, string, error) {
	if cursor == "" {
		return []string{"bookmark-1", "bookmark-2"}, "next", nil
	}
	return []string{"bookmark-3", "bookmark-4", "bookmark-5"}, "", nil
}

type objectRepoStub struct {
	now time.Time
}

func (s objectRepoStub) GetObject(_ context.Context, id string) (any, error) {
	switch id {
	case "bookmark-1":
		return map[string]any{
			"url":       "https://example.com/statuses/1",
			"published": "2024-01-02T03:04:05Z",
		}, nil
	case "bookmark-2":
		return &activitypub.Note{BaseObject: activitypub.BaseObject{ID: "https://example.com/statuses/2", Published: &s.now}}, nil
	case "bookmark-3":
		return &activitypub.Article{Note: activitypub.Note{BaseObject: activitypub.BaseObject{ID: "https://example.com/statuses/3", Published: &s.now}}}, nil
	case "bookmark-4":
		return &activitypub.BaseObject{ID: "https://example.com/statuses/4", Published: &s.now}, nil
	case "bookmark-5":
		return nil, errors.New("get object failed")
	default:
		return "unsupported", nil
	}
}

type activityRepoStub struct {
	now time.Time
}

func (s activityRepoStub) GetOutboxActivities(_ context.Context, _ string, _ int, _ string) ([]*activitypub.Activity, string, error) {
	inRange := s.now.Add(-10 * time.Minute)
	outOfRange := s.now.Add(-48 * time.Hour)
	return []*activitypub.Activity{
		{BaseObject: activitypub.BaseObject{ID: "https://example.com/activities/1", Published: &inRange}, Actor: "https://example.com/users/alice", Object: map[string]any{"id": "https://example.com/objects/1"}},
		{BaseObject: activitypub.BaseObject{ID: "https://example.com/activities/2", Published: &outOfRange}, Actor: "https://example.com/users/alice", Object: map[string]any{"id": "https://example.com/objects/2"}},
	}, "", nil
}

type likeRepoStub struct{}

func (likeRepoStub) GetActorLikes(_ context.Context, actorID string, _ int, _ string) ([]*models.Like, string, error) {
	return []*models.Like{
		{
			Actor:     actorID,
			Object:    "https://example.com/objects/liked",
			ID:        "like-1",
			Published: time.Now(),
		},
	}, "", nil
}

type domainBlockRepoStub struct{}

func (domainBlockRepoStub) GetUserDomainBlocks(_ context.Context, _ string, _ int, cursor string) ([]string, string, error) {
	if cursor == "" {
		return []string{"blocked.example"}, "next", nil
	}
	return []string{"muted.example"}, "", nil
}

type mediaRepoStub struct{}

func (mediaRepoStub) GetUserMedia(_ context.Context, userID string, _ interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Media], error) {
	now := time.Now()
	return &interfaces.PaginatedResult[*models.Media]{
		Items: []*models.Media{
			{
				MediaID:     "m1",
				UserID:      userID,
				FileName:    "one.jpg",
				ContentType: "image/jpeg",
				FileSize:    12,
				S3Key:       "media/one.jpg",
				CDNUrl:      "https://cdn.example/media/one.jpg",
				Status:      "ready",
				CreatedAt:   now,
				UpdatedAt:   now,
			},
			nil,
		},
	}, nil
}

type failingReader struct{}

func (failingReader) Read(_ []byte) (int, error) { return 0, errors.New("read failed") }

type failingWriter struct{}

func (failingWriter) Write(_ []byte) (int, error) { return 0, errors.New("write failed") }

type failingRelationshipRepo struct{}

func (failingRelationshipRepo) GetFollowers(context.Context, string, int, string) ([]string, string, error) {
	return nil, "", errors.New("followers failed")
}

func (failingRelationshipRepo) GetFollowing(context.Context, string, int, string) ([]string, string, error) {
	return nil, "", errors.New("following failed")
}

type failingSocialRepo struct{}

func (failingSocialRepo) GetBlockedUsers(context.Context, string, int, string) ([]*storage.Block, string, error) {
	return nil, "", errors.New("blocks failed")
}

func (failingSocialRepo) GetMutedUsers(context.Context, string, int, string) ([]*storage.Mute, string, error) {
	return nil, "", errors.New("mutes failed")
}

type failingListRepo struct{}

func (failingListRepo) GetListsForUser(context.Context, string) ([]*storage.List, error) {
	return nil, errors.New("lists failed")
}

func (failingListRepo) GetListAccounts(context.Context, string) ([]string, error) {
	return nil, errors.New("list accounts failed")
}

type failingUserRepo struct{}

func (failingUserRepo) GetBookmarks(context.Context, string, int, string) ([]string, string, error) {
	return nil, "", errors.New("bookmarks failed")
}

type failingObjectRepo struct{}

func (failingObjectRepo) GetObject(context.Context, string) (any, error) {
	return nil, errors.New("object failed")
}

type failingActivityRepo struct{}

func (failingActivityRepo) GetOutboxActivities(context.Context, string, int, string) ([]*activitypub.Activity, string, error) {
	return nil, "", errors.New("outbox failed")
}

type failingLikeRepo struct{}

func (failingLikeRepo) GetActorLikes(context.Context, string, int, string) ([]*models.Like, string, error) {
	return nil, "", errors.New("likes failed")
}

type failingDomainBlockRepo struct{}

func (failingDomainBlockRepo) GetUserDomainBlocks(context.Context, string, int, string) ([]string, string, error) {
	return nil, "", errors.New("domain blocks failed")
}

type failingMediaRepo struct{}

func (failingMediaRepo) GetUserMedia(context.Context, string, interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Media], error) {
	return nil, errors.New("media failed")
}

func setAWSEnvForS3Test(t *testing.T, endpoint string) {
	t.Setenv("AWS_REGION", "us-east-1")
	t.Setenv("AWS_ACCESS_KEY_ID", "test")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "test")
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
	t.Setenv("AWS_ENDPOINT_URL_S3", endpoint)
	t.Setenv("AWS_MAX_ATTEMPTS", "1")
	t.Setenv("AWS_S3_USE_PATH_STYLE", "true")
}

func TestExportProcessor_ProcessExportJob_And_HandleSQSWithContext_Round12(t *testing.T) {
	now := time.Now()
	setAWSEnvForS3Test(t, "https://example.com")

	repo := &exportRepoRecorder{errByStatus: map[string]error{"failed": errors.New("status update failed")}}
	costRepo := &costTrackingRepoRecorder{createErr: errors.New("tracking create failed")}
	budgetRepo := &budgetUpdaterRecorder{err: errors.New("budget update failed")}

	repos := exportStorageStub{
		account: accountRepoFunc(func(_ context.Context, username string) (*activitypub.Actor, error) {
			return &activitypub.Actor{BaseObject: activitypub.BaseObject{ID: "https://example.com/users/" + username, Type: "Person"}}, nil
		}),
		relationship: relationshipRepoStub{},
		social:       socialRepoStub{},
		list:         listRepoStub{},
		user:         userRepoStub{},
		object:       objectRepoStub{now: now},
		activity:     activityRepoStub{now: now},
		like:         likeRepoStub{},
		domainBlock:  domainBlockRepoStub{},
		media:        mediaRepoStub{},
	}

	ep := &ExportProcessor{
		repos:            repos,
		exportRepo:       repo,
		costTrackingRepo: costRepo,
		budgetUpdater:    budgetRepo,
		logger:           zap.NewNop(),
		tableName:        "table",
		bucketName:       "bucket",
		baseURL:          "https://example.com",
	}
	require.NoError(t, ep.initializeAWSClients(context.Background()))
	ep.s3Client = &s3ClientStub{
		getObjectFn: func(_ *s3.GetObjectInput) (*s3.GetObjectOutput, error) {
			return &s3.GetObjectOutput{Body: io.NopCloser(bytes.NewReader([]byte("media-bytes")))}, nil
		},
	}

	dateRange := &DateRange{Start: now.Add(-24 * time.Hour), End: now.Add(24 * time.Hour)}

	t.Run("csv types", func(t *testing.T) {
		for _, exportType := range []string{"followers", "following", "blocks", "mutes", "lists", "bookmarks", "domain_blocks"} {
			err := ep.processExportJob(context.Background(), ExportGeneratorEvent{
				ExportID:  "exp-csv-" + exportType,
				Username:  "alice",
				Type:      exportType,
				Format:    "csv",
				DateRange: dateRange,
			})
			require.NoError(t, err)
		}

		require.Error(t, ep.processExportJob(context.Background(), ExportGeneratorEvent{
			ExportID: "exp-csv-bad",
			Username: "alice",
			Type:     "unknown",
			Format:   "csv",
		}))
	})

	t.Run("activitypub archive with media", func(t *testing.T) {
		err := ep.processExportJob(context.Background(), ExportGeneratorEvent{
			ExportID:     "exp-ap-archive",
			Username:     "alice",
			Type:         "archive",
			Format:       "activitypub",
			IncludeMedia: true,
			DateRange:    dateRange,
		})
		require.NoError(t, err)
	})

	t.Run("mastodon archive", func(t *testing.T) {
		err := ep.processExportJob(context.Background(), ExportGeneratorEvent{
			ExportID:  "exp-mastodon-archive",
			Username:  "alice",
			Type:      "archive",
			Format:    "mastodon",
			DateRange: dateRange,
		})
		require.NoError(t, err)
	})

	t.Run("mastodon archive with media", func(t *testing.T) {
		err := ep.processExportJob(context.Background(), ExportGeneratorEvent{
			ExportID:     "exp-mastodon-archive-media",
			Username:     "alice",
			Type:         "archive",
			Format:       "mastodon",
			IncludeMedia: true,
			DateRange:    dateRange,
		})
		require.NoError(t, err)
	})

	t.Run("unsupported format", func(t *testing.T) {
		require.Error(t, ep.processExportJob(context.Background(), ExportGeneratorEvent{
			ExportID: "exp-bad-format",
			Username: "alice",
			Type:     "archive",
			Format:   "nope",
		}))
	})

	t.Run("completed status update failure returns wrapped error", func(t *testing.T) {
		repo.errByStatus["completed"] = errors.New("completed update failed")
		t.Cleanup(func() { delete(repo.errByStatus, "completed") })

		err := ep.processExportJob(context.Background(), ExportGeneratorEvent{
			ExportID:  "exp-completed-fail",
			Username:  "alice",
			Type:      "followers",
			Format:    "csv",
			DateRange: dateRange,
		})
		require.Error(t, err)
	})

	t.Run("HandleSQSMessage parses new and legacy formats", func(t *testing.T) {
		msgCtx := &apptheory.EventContext{RequestID: "req-1"}
		event := events.SQSEvent{
			Records: []events.SQSMessage{
				{MessageId: "1", Body: `{"export_id":"sqs-1","username":"alice","type":"followers","format":"csv","include_media":false,"timestamp":123}`},
				{MessageId: "2", Body: `{"export_id":"sqs-2","username":"alice","type":"followers","format":"csv","include_media":false,"timestamp":"bad"}`},
				{MessageId: "3", Body: `{bad json`},
				{MessageId: "4", Body: `{"export_id":"sqs-4","username":"alice","type":"archive","format":"nope","include_media":false,"timestamp":123}`},
			},
		}

		for _, record := range event.Records {
			require.NoError(t, ep.HandleSQSMessage(msgCtx, record))
		}
		require.NotEmpty(t, repo.statusCalls)
	})

	require.NotEmpty(t, repo.dataByID)
	require.GreaterOrEqual(t, costRepo.createCalls, 1)
	require.GreaterOrEqual(t, budgetRepo.calls, 1)
}

func TestExportStorageAdapter_NilStorage_Round12(t *testing.T) {
	adapter := exportStorageAdapter{storage: nil}
	require.Nil(t, adapter.Account())
	require.Nil(t, adapter.Relationship())
	require.Nil(t, adapter.Social())
	require.Nil(t, adapter.List())
	require.Nil(t, adapter.User())
	require.Nil(t, adapter.Object())
	require.Nil(t, adapter.Activity())
	require.Nil(t, adapter.Like())
	require.Nil(t, adapter.DomainBlock())
	require.Nil(t, adapter.Media())
}

type exportStorageCoreStub struct{}

func (exportStorageCoreStub) Account() *repositories.AccountRepository                { return nil }
func (exportStorageCoreStub) Relationship() interfaces.ConcreteRelationshipRepository { return nil }
func (exportStorageCoreStub) Social() *repositories.SocialRepository                  { return nil }
func (exportStorageCoreStub) List() *repositories.ListRepository                      { return nil }
func (exportStorageCoreStub) User() interfaces.UserRepository                         { return nil }
func (exportStorageCoreStub) Object() interfaces.ObjectRepository                     { return nil }
func (exportStorageCoreStub) Activity() interfaces.ActivityRepository                 { return nil }
func (exportStorageCoreStub) Like() *repositories.LikeRepository                      { return nil }
func (exportStorageCoreStub) DomainBlock() *repositories.DomainBlockRepository        { return nil }
func (exportStorageCoreStub) Media() *repositories.MediaRepository                    { return nil }

func TestExportStorageAdapter_NonNilStorage_Round12(t *testing.T) {
	adapter := exportStorageAdapter{storage: exportStorageCoreStub{}}
	_ = adapter.Account()
	_ = adapter.Relationship()
	_ = adapter.Social()
	_ = adapter.List()
	_ = adapter.User()
	_ = adapter.Object()
	_ = adapter.Activity()
	_ = adapter.Like()
	_ = adapter.DomainBlock()
	_ = adapter.Media()
}

func TestExportProcessor_HelperBranches_Round12(t *testing.T) {
	ep := &ExportProcessor{logger: zap.NewNop()}

	require.Equal(t, "alice", ep.convertActorIDToHandle("alice"))
	require.Equal(t, "@bob@example.com", ep.convertActorIDToHandle("https://example.com/users/bob"))
	require.Equal(t, "@@bob@example.com", ep.convertActorIDToHandle("https://example.com/@bob"))
	require.Equal(t, "https://example.com/", ep.convertActorIDToHandle("https://example.com/"))

	now := time.Now()
	dateRange := &DateRange{Start: now.Add(-1 * time.Hour), End: now.Add(1 * time.Hour)}
	require.True(t, ep.isMediaInDateRange(map[string]any{}, nil))
	require.True(t, ep.isMediaInDateRange(map[string]any{}, dateRange))
	require.True(t, ep.isMediaInDateRange(map[string]any{"CreatedAt": 123}, dateRange))
	require.True(t, ep.isMediaInDateRange(map[string]any{"CreatedAt": "not-a-time"}, dateRange))
	require.False(t, ep.isMediaInDateRange(map[string]any{"CreatedAt": now.Add(-2 * time.Hour).Format(time.RFC3339)}, dateRange))
	require.True(t, ep.isMediaInDateRange(map[string]any{"CreatedAt": now.Format(time.RFC3339)}, dateRange))

	require.Equal(t, "", ep.extractS3Key(map[string]any{}))
	require.Equal(t, "", ep.extractS3Key(map[string]any{"S3Key": 123}))
	require.Equal(t, "media/one.jpg", ep.extractS3Key(map[string]any{"S3Key": "media/one.jpg"}))

	keys := ep.extractVariantKeys(map[string]any{})
	require.Empty(t, keys)
	keys = ep.extractVariantKeys(map[string]any{"Variants": 123})
	require.Empty(t, keys)
	keys = ep.extractVariantKeys(map[string]any{
		"Variants": map[string]any{
			"bad": "not-a-map",
			"no_s3": map[string]any{
				"foo": "bar",
			},
			"wrong_type": map[string]any{
				"S3Key": 123,
			},
			"empty": map[string]any{
				"S3Key": "",
			},
			"good": map[string]any{
				"S3Key": "media/variant.jpg",
			},
		},
	})
	require.Equal(t, []string{"media/variant.jpg"}, keys)

	url, _ := ep.extractBookmarkFromMap(map[string]any{"id": "https://example.com/statuses/1"})
	require.Equal(t, "https://example.com/statuses/1", url)
}

func TestExportProcessor_ZipHelpers_ErrorBranches_Round12(t *testing.T) {
	ep := &ExportProcessor{logger: zap.NewNop()}

	t.Run("addFileToZip returns ErrZipCopy when reader errors", func(t *testing.T) {
		var buf bytes.Buffer
		zw := zip.NewWriter(&buf)
		t.Cleanup(func() { _ = zw.Close() })

		require.Error(t, ep.addFileToZip(zw, "x.txt", failingReader{}))
	})
}

func TestExportProcessor_downloadSingleMediaFile_ErrorBranches_Round12(t *testing.T) {
	t.Run("download error returns false", func(t *testing.T) {
		setAWSEnvForS3Test(t, "https://example.com")

		ep := &ExportProcessor{
			logger:     zap.NewNop(),
			bucketName: "bucket",
		}
		require.NoError(t, ep.initializeAWSClients(context.Background()))
		ep.s3Client = &s3ClientStub{
			getObjectFn: func(_ *s3.GetObjectInput) (*s3.GetObjectOutput, error) {
				return nil, errors.New("download failed")
			},
		}

		var buf bytes.Buffer
		zw := zip.NewWriter(&buf)
		t.Cleanup(func() { _ = zw.Close() })

		require.False(t, ep.downloadSingleMediaFile(context.Background(), zw, "media/missing.jpg"))
	})

	t.Run("zip add error returns false", func(t *testing.T) {
		setAWSEnvForS3Test(t, "https://example.com")

		ep := &ExportProcessor{
			logger:     zap.NewNop(),
			bucketName: "bucket",
		}
		require.NoError(t, ep.initializeAWSClients(context.Background()))
		ep.s3Client = &s3ClientStub{
			getObjectFn: func(_ *s3.GetObjectInput) (*s3.GetObjectOutput, error) {
				return &s3.GetObjectOutput{Body: io.NopCloser(failingReader{})}, nil
			},
		}

		var buf bytes.Buffer
		zw := zip.NewWriter(&buf)
		t.Cleanup(func() { _ = zw.Close() })

		require.False(t, ep.downloadSingleMediaFile(context.Background(), zw, "media/ok.jpg"))
	})
}

func TestExportProcessor_DataRetrieval_ErrorPaths_Round12(t *testing.T) {
	ep := &ExportProcessor{
		logger: zap.NewNop(),
		repos: exportStorageStub{
			account: accountRepoFunc(func(_ context.Context, _ string) (*activitypub.Actor, error) {
				return nil, errors.New("get actor failed")
			}),
		},
	}
	require.Error(t, func() error { _, err := ep.getActor(context.Background(), "alice"); return err }())

	ep.repos = exportStorageStub{
		account:      accountRepoFunc(func(_ context.Context, _ string) (*activitypub.Actor, error) { return &activitypub.Actor{}, nil }),
		relationship: failingRelationshipRepo{},
		social:       failingSocialRepo{},
		list:         failingListRepo{},
		user:         failingUserRepo{},
		object:       failingObjectRepo{},
		activity:     failingActivityRepo{},
		like:         failingLikeRepo{},
		domainBlock:  failingDomainBlockRepo{},
		media:        failingMediaRepo{},
	}

	require.Error(t, func() error { _, err := ep.getFollowers(context.Background(), "alice"); return err }())
	require.Error(t, func() error { _, err := ep.getFollowing(context.Background(), "alice"); return err }())
	require.Error(t, func() error { _, err := ep.getBlocks(context.Background(), "alice"); return err }())
	require.Error(t, func() error { _, err := ep.getMutes(context.Background(), "alice"); return err }())
	require.Error(t, func() error { _, err := ep.getListsWithMembers(context.Background(), "alice"); return err }())
	require.Error(t, func() error { _, _, err := ep.fetchBookmarkBatch(context.Background(), "alice", ""); return err }())
	require.Error(t, func() error { _, _, err := ep.getOutbox(context.Background(), "alice", nil); return err }())
	require.Error(t, func() error { _, err := ep.getFollowingActors(context.Background(), "alice"); return err }())
	require.Error(t, func() error { _, err := ep.getFollowersActors(context.Background(), "alice"); return err }())
	require.Error(t, func() error { _, err := ep.getLikes(context.Background(), "alice"); return err }())
	require.Error(t, func() error { _, err := ep.getDomainBlocks(context.Background(), "alice"); return err }())
	require.Error(t, func() error { _, err := ep.fetchUserMedia(context.Background(), "alice"); return err }())
}

func TestExportProcessor_getLikes_ActorError_Round12(t *testing.T) {
	ep := &ExportProcessor{
		logger: zap.NewNop(),
		repos: exportStorageStub{
			account: accountRepoFunc(func(_ context.Context, _ string) (*activitypub.Actor, error) {
				return nil, errors.New("actor lookup failed")
			}),
		},
	}

	_, err := ep.getLikes(context.Background(), "alice")
	require.Error(t, err)
}

func TestExportProcessor_generateArchiveExports_NonArchiveType_Round12(t *testing.T) {
	now := time.Now()
	repos := exportStorageStub{
		account: accountRepoFunc(func(_ context.Context, username string) (*activitypub.Actor, error) {
			return &activitypub.Actor{BaseObject: activitypub.BaseObject{ID: "https://example.com/users/" + username, Type: "Person"}}, nil
		}),
	}

	ep := &ExportProcessor{
		logger:  zap.NewNop(),
		repos:   repos,
		baseURL: "https://example.com",
	}

	data, _, err := ep.generateActivityPubExport(context.Background(), ExportGeneratorEvent{
		Username:  "alice",
		Type:      "followers",
		Format:    "activitypub",
		DateRange: &DateRange{Start: now.Add(-time.Hour), End: now.Add(time.Hour)},
	}, &models.ExportCostTracking{})
	require.NoError(t, err)
	require.NotEmpty(t, data)

	data, _, err = ep.generateMastodonExport(context.Background(), ExportGeneratorEvent{
		Username:  "alice",
		Type:      "followers",
		Format:    "mastodon",
		DateRange: &DateRange{Start: now.Add(-time.Hour), End: now.Add(time.Hour)},
	}, &models.ExportCostTracking{})
	require.NoError(t, err)
	require.NotEmpty(t, data)
}

func TestExportProcessor_generateArchiveExports_ActorError_Round12(t *testing.T) {
	ep := &ExportProcessor{
		logger: zap.NewNop(),
		repos: exportStorageStub{
			account: accountRepoFunc(func(_ context.Context, _ string) (*activitypub.Actor, error) {
				return nil, errors.New("get actor failed")
			}),
		},
	}

	_, _, err := ep.generateActivityPubExport(context.Background(), ExportGeneratorEvent{Username: "alice", Type: "archive", Format: "activitypub"}, &models.ExportCostTracking{})
	require.Error(t, err)

	_, _, err = ep.generateMastodonExport(context.Background(), ExportGeneratorEvent{Username: "alice", Type: "archive", Format: "mastodon"}, &models.ExportCostTracking{})
	require.Error(t, err)
}

func TestExportProcessor_ErrorConstructors_Round12(t *testing.T) {
	require.Error(t, ErrAWSConfigLoad(errors.New("x")))
	require.Error(t, ErrS3Upload(errors.New("x")))
	require.Error(t, ErrS3PresignedURL(errors.New("x")))
	require.Error(t, ErrGenerateExport(errors.New("x")))
	require.Error(t, ErrCSVWriter(errors.New("x")))
	require.Error(t, ErrZipWriter(errors.New("x")))
	require.Error(t, ErrUpdateStatus(errors.New("x")))
	require.Error(t, ErrGetActor(errors.New("x")))
	require.Error(t, ErrGetFollowers(errors.New("x")))
	require.Error(t, ErrGetFollowing(errors.New("x")))
	require.Error(t, ErrGetBlocks(errors.New("x")))
	require.Error(t, ErrGetMutes(errors.New("x")))
	require.Error(t, ErrGetLists(errors.New("x")))
	require.Error(t, ErrGetBookmarks(errors.New("x")))
	require.Error(t, ErrGetOutbox(errors.New("x")))
	require.Error(t, ErrGetFollowingActors(errors.New("x")))
	require.Error(t, ErrGetFollowerActors(errors.New("x")))
	require.Error(t, ErrGetActorLikes(errors.New("x")))
	require.Error(t, ErrGetDomainBlocks(errors.New("x")))
	require.Error(t, ErrGetUserMedia(errors.New("x")))
	require.Error(t, ErrZipEntryCreate(errors.New("x")))
	require.Error(t, ErrZipCopy(errors.New("x")))
}

func TestExportGenerator_Main_InitializesAndStartsLambda_Round12(t *testing.T) {
	origMustInit := mustInitializeLambda
	origGetClient := getDynamormClient
	origNewFactory := newRepoFactory
	origStart := startLambda
	t.Cleanup(func() {
		mustInitializeLambda = origMustInit
		getDynamormClient = origGetClient
		newRepoFactory = origNewFactory
		startLambda = origStart
	})

	mustInitializeLambda = func(_ common.LambdaConfig) *common.LambdaContext {
		return &common.LambdaContext{
			Config: &config.Config{
				Domain:          "example.com",
				DynamoTableName: "table",
				S3BucketName:    "bucket",
			},
			Logger: zap.NewNop(),
		}
	}

	getDynamormClient = func(context.Context) (dynamormCore.DB, error) { return nil, nil }
	newRepoFactory = func(dynamormCore.DB, string, *zap.Logger) (*factory.RepositoryFactory, error) { return nil, nil }

	startCalls := 0
	startLambda = func(handler interface{}) {
		startCalls++

		h, ok := handler.(func(context.Context, json.RawMessage) (any, error))
		require.True(t, ok)

		event := events.SQSEvent{
			Records: []events.SQSMessage{
				{
					MessageId:      "1",
					Body:           "{bad json",
					EventSourceARN: "arn:aws:sqs:us-east-1:123456789012:lesser-dev-export-processor-queue",
					EventSource:    "aws:sqs",
				},
			},
		}
		raw, err := json.Marshal(event)
		require.NoError(t, err)
		respAny, err := h(context.Background(), raw)
		require.NoError(t, err)

		resp, ok := respAny.(events.SQSEventResponse)
		require.True(t, ok)
		require.Empty(t, resp.BatchItemFailures)
	}

	t.Setenv("APP_NAME", "lesser")
	t.Setenv("STAGE", "dev")
	t.Setenv("ENVIRONMENT", "dev")

	setAWSEnvForS3Test(t, "http://localhost")
	main()

	require.Equal(t, 1, startCalls)
	require.NotNil(t, processor)
	require.Equal(t, "bucket", processor.bucketName)
	require.Equal(t, "https://example.com", processor.baseURL)
}

type s3ClientStub struct {
	putObjectFn func(input *s3.PutObjectInput) (*s3.PutObjectOutput, error)
	getObjectFn func(input *s3.GetObjectInput) (*s3.GetObjectOutput, error)
}

func (s *s3ClientStub) PutObject(_ context.Context, input *s3.PutObjectInput, _ ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
	if s.putObjectFn != nil {
		return s.putObjectFn(input)
	}
	return &s3.PutObjectOutput{}, nil
}

func (s *s3ClientStub) GetObject(_ context.Context, input *s3.GetObjectInput, _ ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	if s.getObjectFn != nil {
		return s.getObjectFn(input)
	}
	return &s3.GetObjectOutput{Body: io.NopCloser(bytes.NewReader(nil))}, nil
}

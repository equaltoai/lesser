package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	pkgErrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/services"
	"github.com/equaltoai/lesser/pkg/services/cms"
	storageCore "github.com/equaltoai/lesser/pkg/storage/core"
	"github.com/equaltoai/lesser/pkg/storage/models"
	lessertesting "github.com/equaltoai/lesser/pkg/testing"
	"github.com/stretchr/testify/require"
	apptheory "github.com/theory-cloud/apptheory/runtime"
	dynamormCore "github.com/theory-cloud/tabletheory/pkg/core"
	dynamormmocks "github.com/theory-cloud/tabletheory/pkg/mocks"
	"go.uber.org/zap"
)

type fakeSchedulerRegistry struct {
	storage storageCore.RepositoryStorage
	drafts  *cms.DraftService
}

func (f *fakeSchedulerRegistry) GetStorage() storageCore.RepositoryStorage { return f.storage }
func (f *fakeSchedulerRegistry) Drafts() *cms.DraftService                 { return f.drafts }

type fakeInstanceStateRepo struct {
	state *models.InstanceState
	err   error
}

func (f *fakeInstanceStateRepo) GetInstanceState(context.Context) (*models.InstanceState, error) {
	return f.state, f.err
}

type fakeDraftRepo struct {
	listFn   func(context.Context, time.Time, int, string) ([]*models.Draft, string, error)
	getFn    func(context.Context, string, string) (*models.Draft, error)
	updateFn func(context.Context, *models.Draft) error

	listCalls   int
	getCalls    int
	updateCalls int
	lastUpdated *models.Draft
}

func (f *fakeDraftRepo) CreateDraft(context.Context, *models.Draft) error { return nil }

func (f *fakeDraftRepo) GetDraft(ctx context.Context, authorID, draftID string) (*models.Draft, error) {
	f.getCalls++
	if f.getFn != nil {
		return f.getFn(ctx, authorID, draftID)
	}
	return nil, errors.New("not implemented")
}

func (f *fakeDraftRepo) UpdateDraft(ctx context.Context, draft *models.Draft) error {
	f.updateCalls++
	f.lastUpdated = draft
	if f.updateFn != nil {
		return f.updateFn(ctx, draft)
	}
	return nil
}

func (f *fakeDraftRepo) DeleteDraft(context.Context, string, string) error { return nil }

func (f *fakeDraftRepo) ListDraftsByAuthor(context.Context, string, int) ([]*models.Draft, error) {
	return nil, nil
}

func (f *fakeDraftRepo) ListDraftsByAuthorPaginated(context.Context, string, int, string) ([]*models.Draft, string, error) {
	return nil, "", nil
}

func (f *fakeDraftRepo) ListScheduledDraftsDuePaginated(ctx context.Context, dueBefore time.Time, limit int, cursor string) ([]*models.Draft, string, error) {
	f.listCalls++
	if f.listFn != nil {
		return f.listFn(ctx, dueBefore, limit, cursor)
	}
	return nil, "", nil
}

type fakeDraftPublisher struct {
	publishFn func(context.Context, string, string) (*models.Article, error)
	calls     int
}

func (f *fakeDraftPublisher) PublishDraft(ctx context.Context, authorID, draftID string) (*models.Article, error) {
	f.calls++
	if f.publishFn != nil {
		return f.publishFn(ctx, authorID, draftID)
	}
	return &models.Article{}, nil
}

func TestEnvInt_Round12(t *testing.T) {
	t.Setenv(envPageSize, "")
	require.Equal(t, defaultPageSize, envInt(envPageSize, defaultPageSize))

	t.Setenv(envPageSize, "10")
	require.Equal(t, 10, envInt(envPageSize, defaultPageSize))

	t.Setenv(envPageSize, "nope")
	require.Equal(t, defaultPageSize, envInt(envPageSize, defaultPageSize))

	t.Setenv(envPageSize, "-1")
	require.Equal(t, defaultPageSize, envInt(envPageSize, defaultPageSize))
}

func TestNewCMSSchedulerProcessor_UsesEnvDefaults_Round12(t *testing.T) {
	origPageSize := os.Getenv(envPageSize)
	origMax := os.Getenv(envMaxDraftsPerRun)
	t.Cleanup(func() {
		_ = os.Setenv(envPageSize, origPageSize)
		_ = os.Setenv(envMaxDraftsPerRun, origMax)
	})

	_ = os.Setenv(envPageSize, "11")
	_ = os.Setenv(envMaxDraftsPerRun, "22")

	p := NewCMSSchedulerProcessor(&fakeSchedulerRegistry{}, &config.Config{}, zap.NewNop())
	require.Equal(t, 11, p.pageSize)
	require.Equal(t, 22, p.maxDraftsPerRun)
}

func TestCMSSchedulerProcessor_publishScheduledDraft_Round12(t *testing.T) {
	origSleep := sleepFn
	t.Cleanup(func() { sleepFn = origSleep })

	p := &CMSSchedulerProcessor{logger: zap.NewNop()}
	draft := &models.Draft{ID: "draft-1", AuthorID: "author-1"}

	t.Run("success", func(t *testing.T) {
		pub := &fakeDraftPublisher{}
		repo := &fakeDraftRepo{}
		require.NoError(t, p.publishScheduledDraft(context.Background(), pub, repo, draft))
		require.Equal(t, 1, pub.calls)
	})

	t.Run("not found is treated as noop", func(t *testing.T) {
		pub := &fakeDraftPublisher{
			publishFn: func(context.Context, string, string) (*models.Article, error) {
				return nil, pkgErrors.NewAppError(pkgErrors.CodeNotFound, pkgErrors.CategoryBusiness, "missing")
			},
		}
		repo := &fakeDraftRepo{}
		require.NoError(t, p.publishScheduledDraft(context.Background(), pub, repo, draft))
		require.Equal(t, 1, pub.calls)
	})

	t.Run("retryable retries with backoff", func(t *testing.T) {
		var sleeps []time.Duration
		sleepFn = func(d time.Duration) { sleeps = append(sleeps, d) }

		attempt := 0
		pub := &fakeDraftPublisher{
			publishFn: func(context.Context, string, string) (*models.Article, error) {
				attempt++
				if attempt < 3 {
					return nil, pkgErrors.NewAppError(pkgErrors.CodeInternal, pkgErrors.CategoryLambda, "retry").AsRetryable()
				}
				return &models.Article{}, nil
			},
		}
		repo := &fakeDraftRepo{}
		require.NoError(t, p.publishScheduledDraft(context.Background(), pub, repo, draft))
		require.Equal(t, 3, pub.calls)
		require.Equal(t, []time.Duration{250 * time.Millisecond, 1000 * time.Millisecond}, sleeps)
	})

	t.Run("non retryable marks failed", func(t *testing.T) {
		pub := &fakeDraftPublisher{
			publishFn: func(context.Context, string, string) (*models.Article, error) {
				return nil, pkgErrors.NewAppError(pkgErrors.CodeInternal, pkgErrors.CategoryLambda, "boom")
			},
		}

		scheduledAt := time.Now().Add(1 * time.Hour).UTC()
		repo := &fakeDraftRepo{
			getFn: func(context.Context, string, string) (*models.Draft, error) {
				return &models.Draft{
					ID:              "draft-1",
					AuthorID:        "author-1",
					Status:          "scheduled",
					ScheduledAt:     &scheduledAt,
					UpdatedAt:       time.Now().Add(-time.Hour).UTC(),
					LastSavedAt:     time.Now().Add(-2 * time.Hour).UTC(),
					AutosaveVersion: 0,
				}, nil
			},
		}

		err := p.publishScheduledDraft(context.Background(), pub, repo, draft)
		require.Error(t, err)
		require.Equal(t, 1, repo.getCalls)
		require.Equal(t, 1, repo.updateCalls)
		require.NotNil(t, repo.lastUpdated)
		require.Equal(t, "failed", repo.lastUpdated.Status)
		require.Nil(t, repo.lastUpdated.ScheduledAt)
	})
}

func TestCMSSchedulerProcessor_markScheduledDraftFailed_Round12(t *testing.T) {
	p := &CMSSchedulerProcessor{logger: zap.NewNop()}

	t.Run("get error is ignored", func(t *testing.T) {
		repo := &fakeDraftRepo{
			getFn: func(context.Context, string, string) (*models.Draft, error) {
				return nil, errors.New("boom")
			},
		}
		p.markScheduledDraftFailed(context.Background(), repo, "author", "draft", errors.New("cause"))
		require.Equal(t, 1, repo.getCalls)
		require.Equal(t, 0, repo.updateCalls)
	})

	t.Run("published drafts are not marked failed", func(t *testing.T) {
		repo := &fakeDraftRepo{
			getFn: func(context.Context, string, string) (*models.Draft, error) {
				return &models.Draft{ID: "draft", AuthorID: "author", Status: "published"}, nil
			},
		}
		p.markScheduledDraftFailed(context.Background(), repo, "author", "draft", errors.New("cause"))
		require.Equal(t, 1, repo.getCalls)
		require.Equal(t, 0, repo.updateCalls)
	})

	t.Run("update error is logged but not returned", func(t *testing.T) {
		repo := &fakeDraftRepo{
			getFn: func(context.Context, string, string) (*models.Draft, error) {
				return &models.Draft{ID: "draft", AuthorID: "author", Status: "scheduled"}, nil
			},
			updateFn: func(context.Context, *models.Draft) error { return errors.New("boom") },
		}
		p.markScheduledDraftFailed(context.Background(), repo, "author", "draft", errors.New("cause"))
		require.Equal(t, 1, repo.updateCalls)
	})
}

func TestCMSSchedulerProcessor_HandleScheduledEvent_Round12(t *testing.T) {
	ctx := &apptheory.EventContext{RequestID: "req"}

	enabledCfg := &config.Config{
		InstanceMode:                  config.InstanceModeCMS,
		CMSLongFormPublishingEnabled:  true,
		CMSDraftSystemEnabled:         true,
		CMSScheduledPublishingEnabled: true,
	}

	t.Run("disabled config returns early", func(t *testing.T) {
		p := &CMSSchedulerProcessor{
			registry: &fakeSchedulerRegistry{},
			cfg:      &config.Config{InstanceMode: config.InstanceModeSocial},
			logger:   zap.NewNop(),
		}
		_, err := p.HandleScheduledEvent(ctx, events.EventBridgeEvent{Source: "aws.events"})
		require.NoError(t, err)
	})

	t.Run("nil registry returns early", func(t *testing.T) {
		p := &CMSSchedulerProcessor{
			registry: nil,
			cfg:      enabledCfg,
			logger:   zap.NewNop(),
		}
		_, err := p.HandleScheduledEvent(ctx, events.EventBridgeEvent{Source: "aws.events"})
		require.NoError(t, err)
	})

	t.Run("nil storage returns early", func(t *testing.T) {
		p := &CMSSchedulerProcessor{
			registry: &fakeSchedulerRegistry{storage: nil},
			cfg:      enabledCfg,
			logger:   zap.NewNop(),
		}
		_, err := p.HandleScheduledEvent(ctx, events.EventBridgeEvent{Source: "aws.events"})
		require.NoError(t, err)
	})

	t.Run("missing draft repository returns early", func(t *testing.T) {
		storage := lessertesting.NewMockRepositoryStorage(lessertesting.WithDraftRepository(nil))
		p := &CMSSchedulerProcessor{
			registry: &fakeSchedulerRegistry{storage: storage},
			cfg:      enabledCfg,
			logger:   zap.NewNop(),
		}
		p.instanceRepo = &fakeInstanceStateRepo{state: &models.InstanceState{Locked: false}}
		p.draftSvc = &fakeDraftPublisher{}

		_, err := p.HandleScheduledEvent(ctx, events.EventBridgeEvent{Source: "aws.events"})
		require.NoError(t, err)
	})

	t.Run("missing instance repository returns early", func(t *testing.T) {
		draftRepo := &fakeDraftRepo{}
		storage := lessertesting.NewMockRepositoryStorage(lessertesting.WithDraftRepository(draftRepo))

		p := &CMSSchedulerProcessor{
			registry:        &fakeSchedulerRegistry{storage: storage},
			cfg:             enabledCfg,
			logger:          zap.NewNop(),
			pageSize:        1,
			maxDraftsPerRun: 1,
		}
		_, err := p.HandleScheduledEvent(ctx, events.EventBridgeEvent{Source: "aws.events"})
		require.NoError(t, err)
		require.Equal(t, 0, draftRepo.listCalls)
	})

	t.Run("instance state error returns early", func(t *testing.T) {
		draftRepo := &fakeDraftRepo{}
		storage := lessertesting.NewMockRepositoryStorage(lessertesting.WithDraftRepository(draftRepo))

		p := &CMSSchedulerProcessor{
			registry:        &fakeSchedulerRegistry{storage: storage},
			cfg:             enabledCfg,
			logger:          zap.NewNop(),
			pageSize:        1,
			maxDraftsPerRun: 1,
		}
		p.instanceRepo = &fakeInstanceStateRepo{err: errors.New("boom")}
		_, err := p.HandleScheduledEvent(ctx, events.EventBridgeEvent{Source: "aws.events"})
		require.NoError(t, err)
		require.Equal(t, 0, draftRepo.listCalls)
	})

	t.Run("locked instance skips run", func(t *testing.T) {
		draftRepo := &fakeDraftRepo{}
		storage := lessertesting.NewMockRepositoryStorage(lessertesting.WithDraftRepository(draftRepo))
		p := &CMSSchedulerProcessor{
			registry:        &fakeSchedulerRegistry{storage: storage},
			cfg:             enabledCfg,
			logger:          zap.NewNop(),
			pageSize:        1,
			maxDraftsPerRun: 1,
		}
		p.instanceRepo = &fakeInstanceStateRepo{state: &models.InstanceState{Locked: true}}
		p.draftSvc = &fakeDraftPublisher{}

		_, err := p.HandleScheduledEvent(ctx, events.EventBridgeEvent{Source: "aws.events"})
		require.NoError(t, err)
		require.Equal(t, 0, draftRepo.listCalls)
	})

	t.Run("missing draft service returns early", func(t *testing.T) {
		draftRepo := &fakeDraftRepo{}
		storage := lessertesting.NewMockRepositoryStorage(lessertesting.WithDraftRepository(draftRepo))

		p := &CMSSchedulerProcessor{
			registry:        &fakeSchedulerRegistry{storage: storage, drafts: nil},
			cfg:             enabledCfg,
			logger:          zap.NewNop(),
			pageSize:        1,
			maxDraftsPerRun: 1,
		}
		p.instanceRepo = &fakeInstanceStateRepo{state: &models.InstanceState{Locked: false}}

		_, err := p.HandleScheduledEvent(ctx, events.EventBridgeEvent{Source: "aws.events"})
		require.NoError(t, err)
		require.Equal(t, 0, draftRepo.listCalls)
	})

	t.Run("list error surfaces", func(t *testing.T) {
		draftRepo := &fakeDraftRepo{
			listFn: func(context.Context, time.Time, int, string) ([]*models.Draft, string, error) {
				return nil, "", errors.New("boom")
			},
		}
		storage := lessertesting.NewMockRepositoryStorage(lessertesting.WithDraftRepository(draftRepo))

		p := &CMSSchedulerProcessor{
			registry:        &fakeSchedulerRegistry{storage: storage},
			cfg:             enabledCfg,
			logger:          zap.NewNop(),
			pageSize:        1,
			maxDraftsPerRun: 1,
		}
		p.instanceRepo = &fakeInstanceStateRepo{state: &models.InstanceState{Locked: false}}
		p.draftSvc = &fakeDraftPublisher{}

		_, err := p.HandleScheduledEvent(ctx, events.EventBridgeEvent{Source: "aws.events"})
		require.Error(t, err)
		require.Equal(t, 1, draftRepo.listCalls)
	})

	t.Run("empty scheduled page stops without publish", func(t *testing.T) {
		draftRepo := &fakeDraftRepo{
			listFn: func(context.Context, time.Time, int, string) ([]*models.Draft, string, error) {
				return []*models.Draft{}, "", nil
			},
		}
		storage := lessertesting.NewMockRepositoryStorage(lessertesting.WithDraftRepository(draftRepo))

		pub := &fakeDraftPublisher{}
		p := &CMSSchedulerProcessor{
			registry:        &fakeSchedulerRegistry{storage: storage},
			cfg:             enabledCfg,
			logger:          zap.NewNop(),
			pageSize:        1,
			maxDraftsPerRun: 1,
		}
		p.instanceRepo = &fakeInstanceStateRepo{state: &models.InstanceState{Locked: false}}
		p.draftSvc = pub

		_, err := p.HandleScheduledEvent(ctx, events.EventBridgeEvent{Source: "aws.events"})
		require.NoError(t, err)
		require.Equal(t, 0, pub.calls)
		require.Equal(t, 1, draftRepo.listCalls)
	})

	t.Run("publishes drafts until exhausted", func(t *testing.T) {
		draftRepo := &fakeDraftRepo{
			listFn: func(context.Context, time.Time, int, string) ([]*models.Draft, string, error) {
				return []*models.Draft{
					{ID: "draft-1", AuthorID: "author-1", Status: "scheduled"},
				}, "", nil
			},
		}
		storage := lessertesting.NewMockRepositoryStorage(lessertesting.WithDraftRepository(draftRepo))

		pub := &fakeDraftPublisher{}
		p := &CMSSchedulerProcessor{
			registry:        &fakeSchedulerRegistry{storage: storage},
			cfg:             enabledCfg,
			logger:          zap.NewNop(),
			pageSize:        1,
			maxDraftsPerRun: 1,
		}
		p.instanceRepo = &fakeInstanceStateRepo{state: &models.InstanceState{Locked: false}}
		p.draftSvc = pub

		_, err := p.HandleScheduledEvent(ctx, events.EventBridgeEvent{Source: "aws.events"})
		require.NoError(t, err)
		require.Equal(t, 1, pub.calls)
		require.Equal(t, 1, draftRepo.listCalls)
	})

	t.Run("paginates with cursor and respects max drafts", func(t *testing.T) {
		draftRepo := &fakeDraftRepo{
			listFn: func(_ context.Context, _ time.Time, _ int, cursor string) ([]*models.Draft, string, error) {
				switch cursor {
				case "":
					return []*models.Draft{
						{ID: "draft-1", AuthorID: "author-1", Status: "scheduled"},
						{ID: "draft-2", AuthorID: "author-2", Status: "scheduled"},
					}, "next", nil
				default:
					return []*models.Draft{}, "", nil
				}
			},
		}
		storage := lessertesting.NewMockRepositoryStorage(lessertesting.WithDraftRepository(draftRepo))

		pub := &fakeDraftPublisher{}
		p := &CMSSchedulerProcessor{
			registry:        &fakeSchedulerRegistry{storage: storage},
			cfg:             enabledCfg,
			logger:          zap.NewNop(),
			pageSize:        2,
			maxDraftsPerRun: 1,
		}
		p.instanceRepo = &fakeInstanceStateRepo{state: &models.InstanceState{Locked: false}}
		p.draftSvc = pub

		_, err := p.HandleScheduledEvent(ctx, events.EventBridgeEvent{Source: "aws.events"})
		require.NoError(t, err)
		require.Equal(t, 1, pub.calls)
		require.Equal(t, 1, draftRepo.listCalls)
	})
}

func TestMain_RoutesEventBridge_Round12(t *testing.T) {
	origStart := lambdaStartFn
	origProcessor := processor
	t.Cleanup(func() {
		lambdaStartFn = origStart
		processor = origProcessor
	})

	t.Setenv("APP_NAME", "lesser")
	t.Setenv("STAGE", "dev")
	t.Setenv("ENVIRONMENT", "dev")

	called := false
	lambdaStartFn = func(handler any) {
		called = true

		fn, ok := handler.(func(context.Context, json.RawMessage) (any, error))
		require.True(t, ok)

		event := map[string]any{
			"id":          "evt",
			"source":      "aws.events",
			"detail-type": "Scheduled Event",
			"detail":      map[string]any{},
			"time":        time.Now().UTC().Format(time.RFC3339),
			"resources": []any{
				"arn:aws:events:us-east-1:123456789012:rule/lesser-dev-cms-scheduler-schedule-0",
			},
		}
		raw, err := json.Marshal(event)
		require.NoError(t, err)

		processor = nil
		_, err = fn(context.Background(), raw)
		require.Error(t, err)

		processor = &CMSSchedulerProcessor{cfg: &config.Config{}, logger: zap.NewNop()}
		_, err = fn(context.Background(), raw)
		require.NoError(t, err)
	}

	main()
	require.True(t, called)
}

func TestInitializeCMSScheduler_Round12(t *testing.T) {
	origMust := mustInitializeLambdaFn
	origInitDefaults := initializeWithDefaultsFn
	origNewClient := newLambdaOptimizedClientFn
	origNewStorage := newRepositoryStorageFn
	origNewRegistry := newRegistryFn
	origNewProcessor := newProcessorFn
	t.Cleanup(func() {
		mustInitializeLambdaFn = origMust
		initializeWithDefaultsFn = origInitDefaults
		newLambdaOptimizedClientFn = origNewClient
		newRepositoryStorageFn = origNewStorage
		newRegistryFn = origNewRegistry
		newProcessorFn = origNewProcessor
	})

	mustInitializeLambdaFn = func(common.LambdaConfig) *common.LambdaContext {
		return &common.LambdaContext{
			Config: &config.Config{
				Domain:                        "example.com",
				Region:                        "us-east-1",
				DynamoTableName:               "test-table",
				JWTSecret:                     "secret",
				InstanceMode:                  config.InstanceModeCMS,
				CMSLongFormPublishingEnabled:  true,
				CMSDraftSystemEnabled:         true,
				CMSScheduledPublishingEnabled: true,
			},
			Logger: zap.NewNop(),
		}
	}
	initializeWithDefaultsFn = func(*common.LambdaContext) error { return errors.New("boom") }
	newLambdaOptimizedClientFn = func(context.Context, string) (dynamormCore.DB, error) {
		return new(dynamormmocks.MockDB), nil
	}
	newRepositoryStorageFn = func(dynamormCore.DB, string, *zap.Logger) (storageCore.RepositoryStorage, error) {
		return lessertesting.NewMockRepositoryStorage(), nil
	}
	newRegistryFn = func(opts ...services.RegistryOption) (*services.Registry, error) {
		return services.NewRegistry(opts...)
	}
	newProcessorFn = func(registry cmsSchedulerRegistry, cfg *config.Config, logger *zap.Logger) *CMSSchedulerProcessor {
		return &CMSSchedulerProcessor{registry: registry, cfg: cfg, logger: logger}
	}

	initializeCMSScheduler()

	require.NotNil(t, lambdaCtx)
	require.NotNil(t, cfg)
	require.NotNil(t, logger)
	require.NotNil(t, processor)
}

func TestMain_WiresLambdaStart_Round12(t *testing.T) {
	origStart := lambdaStartFn
	origLogger := logger
	t.Cleanup(func() {
		lambdaStartFn = origStart
		logger = origLogger
	})

	logger = zap.NewNop()
	called := false
	lambdaStartFn = func(any) { called = true }
	main()
	require.True(t, called)
}

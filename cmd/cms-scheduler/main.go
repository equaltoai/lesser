// Package main implements the cms-scheduler Lambda function for publishing scheduled CMS drafts.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/deploy/naming"
	pkgErrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/services"
	"github.com/equaltoai/lesser/pkg/services/cms"
	storageCore "github.com/equaltoai/lesser/pkg/storage/core"
	"github.com/equaltoai/lesser/pkg/storage/factory"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/theorydb"
	apptheory "github.com/theory-cloud/apptheory/runtime"
	dynamormCore "github.com/theory-cloud/tabletheory/pkg/core"
	"go.uber.org/zap"
)

const (
	defaultPageSize        = 25
	defaultMaxDraftsPerRun = 200

	envPageSize        = "CMS_SCHEDULER_PAGE_SIZE"
	envMaxDraftsPerRun = "CMS_SCHEDULER_MAX_DRAFTS"
)

type CMSSchedulerProcessor struct {
	registry cmsSchedulerRegistry
	cfg      *config.Config
	logger   *zap.Logger

	pageSize        int
	maxDraftsPerRun int

	instanceRepo instanceStateGetter
	draftSvc     draftPublisher
}

type cmsSchedulerRegistry interface {
	GetStorage() storageCore.RepositoryStorage
	Drafts() *cms.DraftService
}

type instanceStateGetter interface {
	GetInstanceState(ctx context.Context) (*models.InstanceState, error)
}

type draftPublisher interface {
	PublishDraft(ctx context.Context, authorID, draftID string) (*models.Article, error)
}

var sleepFn = time.Sleep

func NewCMSSchedulerProcessor(registry cmsSchedulerRegistry, cfg *config.Config, logger *zap.Logger) *CMSSchedulerProcessor {
	return &CMSSchedulerProcessor{
		registry:        registry,
		cfg:             cfg,
		logger:          logger,
		pageSize:        envInt(envPageSize, defaultPageSize),
		maxDraftsPerRun: envInt(envMaxDraftsPerRun, defaultMaxDraftsPerRun),
	}
}

func (p *CMSSchedulerProcessor) HandleScheduledEvent(ctx *apptheory.EventContext, event events.EventBridgeEvent) (any, error) {
	requestID := ""
	runCtx := context.Background()
	if ctx != nil {
		requestID = ctx.RequestID
		runCtx = ctx.Context()
	}
	now := time.Now().UTC()

	if !p.isSchedulerEnabled(requestID) {
		return nil, nil
	}

	p.logger.Info("cms scheduler run starting",
		zap.String("request_id", requestID),
		zap.String("source", event.Source),
		zap.String("detail_type", event.DetailType),
		zap.Time("now", now),
		zap.Int("page_size", p.pageSize),
		zap.Int("max_drafts", p.maxDraftsPerRun),
	)

	draftRepo, draftSvc, err := p.resolveDependencies(runCtx, requestID)
	if err != nil {
		return nil, nil // logged inside resolveDependencies
	}

	attempted, published, err := p.processDrafts(runCtx, draftRepo, draftSvc, now)
	if err != nil {
		return nil, err
	}

	p.logger.Info("cms scheduler run complete",
		zap.String("request_id", requestID),
		zap.Int("attempted", attempted),
		zap.Int("published", published),
	)

	return nil, nil
}

func (p *CMSSchedulerProcessor) isSchedulerEnabled(requestID string) bool {
	if p.cfg == nil || !p.cfg.CMSLongFormEnabled() || !p.cfg.CMSDraftsEnabled() || !p.cfg.CMSSchedulingEnabled() {
		instanceMode := ""
		if p.cfg != nil {
			instanceMode = string(p.cfg.EffectiveInstanceMode())
		}
		p.logger.Info("cms scheduler disabled by configuration",
			zap.String("request_id", requestID),
			zap.String("instance_mode", instanceMode),
		)
		return false
	}
	return true
}

func (p *CMSSchedulerProcessor) resolveDependencies(runCtx context.Context, requestID string) (interfaces.DraftRepository, draftPublisher, error) {
	if p.registry == nil {
		p.logger.Warn("cms scheduler registry not available; skipping run",
			zap.String("request_id", requestID),
		)
		return nil, nil, fmt.Errorf("registry missing")
	}

	storage := p.registry.GetStorage()
	draftRepo := interfaces.DraftRepository(nil)
	if storage != nil {
		draftRepo = storage.Draft()
	}
	if storage == nil || draftRepo == nil {
		p.logger.Warn("cms scheduler storage not available; skipping run",
			zap.String("request_id", requestID),
		)
		return nil, nil, fmt.Errorf("storage missing")
	}

	stateRepo := p.instanceRepo
	if stateRepo == nil {
		if repo := storage.Instance(); repo != nil {
			stateRepo = repo
		}
	}
	if stateRepo == nil {
		p.logger.Warn("cms scheduler instance repository not available; skipping run",
			zap.String("request_id", requestID),
		)
		return nil, nil, fmt.Errorf("instance repo missing")
	}

	state, err := stateRepo.GetInstanceState(runCtx)
	if err != nil {
		p.logger.Warn("failed to get instance state; defaulting to locked and skipping cms scheduler run",
			zap.String("request_id", requestID),
			zap.Error(err),
		)
		return nil, nil, err
	}
	if state.Locked {
		p.logger.Info("instance is locked; skipping cms scheduler run",
			zap.String("request_id", requestID),
		)
		return nil, nil, fmt.Errorf("instance locked")
	}

	draftSvc := p.draftSvc
	if draftSvc == nil {
		if svc := p.registry.Drafts(); svc != nil {
			draftSvc = svc
		}
	}
	if draftSvc == nil {
		p.logger.Warn("draft service not available; skipping cms scheduler run",
			zap.String("request_id", requestID),
		)
		return nil, nil, fmt.Errorf("draft service missing")
	}

	return draftRepo, draftSvc, nil
}

func (p *CMSSchedulerProcessor) processDrafts(ctx context.Context, draftRepo interfaces.DraftRepository, draftSvc draftPublisher, now time.Time) (int, int, error) {
	attempted := 0
	published := 0
	cursor := ""

	for attempted < p.maxDraftsPerRun {
		remaining := p.maxDraftsPerRun - attempted
		limit := p.pageSize
		if remaining < limit {
			limit = remaining
		}

		drafts, nextCursor, err := draftRepo.ListScheduledDraftsDuePaginated(ctx, now, limit, cursor)
		if err != nil {
			return attempted, published, pkgErrors.WrapError(err, pkgErrors.CodeInternal, pkgErrors.CategoryLambda, "failed to list scheduled drafts")
		}
		if len(drafts) == 0 {
			break
		}

		for _, draft := range drafts {
			if attempted >= p.maxDraftsPerRun {
				break
			}
			if err := p.publishScheduledDraft(ctx, draftSvc, draftRepo, draft); err != nil {
				p.logger.Error("failed to publish scheduled draft",
					zap.String("draft_id", draft.ID),
					zap.String("author_id", draft.AuthorID),
					zap.String("status", draft.Status),
					zap.Timep("scheduled_at", draft.ScheduledAt),
					zap.Error(err),
				)
			} else {
				published++
			}
			attempted++
		}

		if nextCursor == "" {
			break
		}
		cursor = nextCursor
	}

	return attempted, published, nil
}

func (p *CMSSchedulerProcessor) publishScheduledDraft(ctx context.Context, draftSvc draftPublisher, draftRepo interfaces.DraftRepository, draft *models.Draft) error {
	// NOTE: We accept drafts in any state; the scheduler only queries scheduled drafts, but publish can be retried safely.
	const maxAttempts = 3

	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		_, err := draftSvc.PublishDraft(ctx, draft.AuthorID, draft.ID)
		if err == nil {
			return nil
		}

		if pkgErrors.HasCode(err, pkgErrors.CodeNotFound) {
			// Draft or article may have been deleted between query and publish; treat as a no-op.
			p.logger.Info("scheduled draft no longer exists",
				zap.String("draft_id", draft.ID),
				zap.String("author_id", draft.AuthorID),
			)
			return nil
		}

		lastErr = err
		if pkgErrors.IsRetryable(err) && attempt < maxAttempts {
			backoff := time.Duration(attempt*attempt) * 250 * time.Millisecond
			p.logger.Warn("retrying scheduled draft publish",
				zap.String("draft_id", draft.ID),
				zap.String("author_id", draft.AuthorID),
				zap.Int("attempt", attempt),
				zap.Duration("backoff", backoff),
				zap.Error(err),
			)
			sleepFn(backoff)
			continue
		}

		// If publish failed with a non-retryable error (or retries exhausted), mark as FAILED and clear schedule.
		p.markScheduledDraftFailed(ctx, draftRepo, draft.AuthorID, draft.ID, err)
		return err
	}

	return lastErr
}

func (p *CMSSchedulerProcessor) markScheduledDraftFailed(ctx context.Context, draftRepo interfaces.DraftRepository, authorID, draftID string, err error) {
	draft, getErr := draftRepo.GetDraft(ctx, authorID, draftID)
	if getErr != nil {
		return
	}

	status := strings.ToLower(strings.TrimSpace(draft.Status))
	if status == "published" {
		return
	}

	now := time.Now().UTC()
	draft.Status = "failed"
	draft.ScheduledAt = nil
	draft.UpdatedAt = now
	if updateErr := draftRepo.UpdateDraft(ctx, authorID, draft); updateErr != nil {
		p.logger.Warn("failed to mark scheduled draft as failed",
			zap.String("draft_id", draftID),
			zap.String("author_id", authorID),
			zap.Error(err),
			zap.Error(updateErr),
		)
	}
}

func envInt(key string, defaultValue int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return defaultValue
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return defaultValue
	}
	return parsed
}

// --- Lambda wiring ---

var (
	lambdaCtx *common.LambdaContext
	cfg       *config.Config
	logger    *zap.Logger
	processor *CMSSchedulerProcessor
)

func init() {
	if common.RunningUnitTests() {
		return
	}
	initializeCMSScheduler()
}

var (
	mustInitializeLambdaFn     = common.MustInitializeLambda
	initializeWithDefaultsFn   = func(ctx *common.LambdaContext) error { return ctx.InitializeWithDefaults() }
	newLambdaOptimizedClientFn = theorydb.NewLambdaOptimizedClient
	newRepositoryStorageFn     = func(db dynamormCore.DB, tableName string, logger *zap.Logger) (storageCore.RepositoryStorage, error) {
		repos, err := factory.NewRepositoryFactory(db, tableName, logger)
		if err != nil {
			return nil, err
		}
		return repos, nil
	}
	newRegistryFn  = services.NewRegistry
	newProcessorFn = NewCMSSchedulerProcessor
	lambdaStartFn  = lambda.Start
)

func initializeCMSScheduler() {
	lambdaCtx = mustInitializeLambdaFn(common.LambdaConfig{
		ServiceName: "cms-scheduler",
		LambdaType:  common.LambdaTypeProcessor,
	})

	cfg = lambdaCtx.Config
	logger = lambdaCtx.Logger

	if err := initializeWithDefaultsFn(lambdaCtx); err != nil {
		logger.Warn("failed to initialize with defaults", zap.Error(err))
	}

	db, err := newLambdaOptimizedClientFn(context.Background(), cfg.Region)
	if err != nil {
		logger.Fatal("failed to initialize DynamORM", zap.Error(err))
	}

	repos, err := newRepositoryStorageFn(db, cfg.DynamoTableName, logger)
	if err != nil {
		logger.Fatal("failed to initialize repositories", zap.Error(err))
	}

	serviceCfg := &services.ServiceConfig{
		BaseURL:   fmt.Sprintf("https://%s", cfg.Domain),
		JWTSecret: cfg.JWTSecret,
		Config:    cfg,
	}

	registry, err := newRegistryFn(
		services.WithStorage(repos),
		services.WithLogger(logger),
		services.WithConfig(serviceCfg),
	)
	if err != nil {
		logger.Fatal("failed to initialize services registry", zap.Error(err))
	}

	processor = newProcessorFn(registry, cfg, logger)
}

func main() {
	app := apptheory.New()

	appName := strings.TrimSpace(os.Getenv("APP_NAME"))
	stage := strings.TrimSpace(os.Getenv("STAGE"))
	ruleName := naming.ResourceNameWithApp(appName, "cms-scheduler-schedule-0", stage)

	app.EventBridge(apptheory.EventBridgeRule(ruleName), func(ctx *apptheory.EventContext, event events.EventBridgeEvent) (any, error) {
		if processor == nil {
			return nil, fmt.Errorf("cms scheduler processor not initialized")
		}
		return processor.HandleScheduledEvent(ctx, event)
	})

	lambdaStartFn(func(ctx context.Context, event json.RawMessage) (any, error) {
		return app.HandleLambda(ctx, event)
	})
}

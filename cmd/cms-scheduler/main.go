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
	pkgErrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/lift/patterns"
	"github.com/equaltoai/lesser/pkg/middleware"
	"github.com/equaltoai/lesser/pkg/services"
	"github.com/equaltoai/lesser/pkg/services/cms"
	"github.com/equaltoai/lesser/pkg/storage/dynamorm"
	"github.com/equaltoai/lesser/pkg/storage/factory"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"
)

const (
	defaultPageSize        = 25
	defaultMaxDraftsPerRun = 200

	envPageSize        = "CMS_SCHEDULER_PAGE_SIZE"
	envMaxDraftsPerRun = "CMS_SCHEDULER_MAX_DRAFTS"
)

type CMSSchedulerProcessor struct {
	registry *services.Registry
	cfg      *config.Config
	logger   *zap.Logger

	pageSize        int
	maxDraftsPerRun int
}

func NewCMSSchedulerProcessor(registry *services.Registry, cfg *config.Config, logger *zap.Logger) *CMSSchedulerProcessor {
	return &CMSSchedulerProcessor{
		registry:        registry,
		cfg:             cfg,
		logger:          logger,
		pageSize:        envInt(envPageSize, defaultPageSize),
		maxDraftsPerRun: envInt(envMaxDraftsPerRun, defaultMaxDraftsPerRun),
	}
}

func (p *CMSSchedulerProcessor) HandleEvent(ctx *lift.Context, event events.CloudWatchEvent) error {
	runCtx := ctx.Request.Context()
	now := time.Now().UTC()

	if p.cfg == nil || !p.cfg.CMSLongFormEnabled() || !p.cfg.CMSDraftsEnabled() || !p.cfg.CMSSchedulingEnabled() {
		instanceMode := ""
		if p.cfg != nil {
			instanceMode = string(p.cfg.EffectiveInstanceMode())
		}
		p.logger.Info("cms scheduler disabled by configuration",
			zap.String("request_id", ctx.GetRequestID()),
			zap.String("instance_mode", instanceMode),
		)
		return nil
	}

	p.logger.Info("cms scheduler run starting",
		zap.String("request_id", ctx.GetRequestID()),
		zap.String("source", event.Source),
		zap.String("detail_type", event.DetailType),
		zap.Time("now", now),
		zap.Int("page_size", p.pageSize),
		zap.Int("max_drafts", p.maxDraftsPerRun),
	)

	storage := p.registry.GetStorage()
	if storage == nil || storage.Draft() == nil {
		p.logger.Warn("cms scheduler storage not available; skipping run",
			zap.String("request_id", ctx.GetRequestID()),
		)
		return nil
	}

	state, err := storage.Instance().GetInstanceState(runCtx)
	if err != nil {
		p.logger.Warn("failed to get instance state; defaulting to locked and skipping cms scheduler run",
			zap.String("request_id", ctx.GetRequestID()),
			zap.Error(err),
		)
		return nil
	}
	if state.Locked {
		p.logger.Info("instance is locked; skipping cms scheduler run",
			zap.String("request_id", ctx.GetRequestID()),
		)
		return nil
	}

	draftRepo := storage.Draft()
	draftSvc := p.registry.Drafts()
	if draftSvc == nil {
		p.logger.Warn("draft service not available; skipping cms scheduler run",
			zap.String("request_id", ctx.GetRequestID()),
		)
		return nil
	}

	attempted := 0
	published := 0
	cursor := ""
	for attempted < p.maxDraftsPerRun {
		remaining := p.maxDraftsPerRun - attempted
		limit := p.pageSize
		if remaining < limit {
			limit = remaining
		}

		drafts, nextCursor, err := draftRepo.ListScheduledDraftsDuePaginated(runCtx, now, limit, cursor)
		if err != nil {
			return pkgErrors.WrapError(err, pkgErrors.CodeInternal, pkgErrors.CategoryLambda, "failed to list scheduled drafts")
		}
		if len(drafts) == 0 {
			break
		}

		for _, draft := range drafts {
			if attempted >= p.maxDraftsPerRun {
				break
			}
			if err := p.publishScheduledDraft(runCtx, draftSvc, draftRepo, draft); err != nil {
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

	p.logger.Info("cms scheduler run complete",
		zap.String("request_id", ctx.GetRequestID()),
		zap.Int("attempted", attempted),
		zap.Int("published", published),
	)

	return nil
}

func (p *CMSSchedulerProcessor) publishScheduledDraft(ctx context.Context, draftSvc *cms.DraftService, draftRepo *repositories.DraftRepository, draft *models.Draft) error {
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
			time.Sleep(backoff)
			continue
		}

		// If publish failed with a non-retryable error (or retries exhausted), mark as FAILED and clear schedule.
		p.markScheduledDraftFailed(ctx, draftRepo, draft.AuthorID, draft.ID, err)
		return err
	}

	return lastErr
}

func (p *CMSSchedulerProcessor) markScheduledDraftFailed(ctx context.Context, draftRepo *repositories.DraftRepository, authorID, draftID string, err error) {
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
	if updateErr := draftRepo.UpdateDraft(ctx, draft); updateErr != nil {
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

	lambdaCtx = common.MustInitializeLambda(common.LambdaConfig{
		ServiceName: "cms-scheduler",
		LambdaType:  common.LambdaTypeProcessor,
	})

	cfg = lambdaCtx.Config
	logger = lambdaCtx.Logger

	if err := lambdaCtx.InitializeWithDefaults(); err != nil {
		logger.Warn("failed to initialize with defaults", zap.Error(err))
	}

	db, err := dynamorm.NewLambdaOptimizedClient(context.Background(), cfg.Region)
	if err != nil {
		logger.Fatal("failed to initialize DynamORM", zap.Error(err))
	}

	repos, err := factory.NewRepositoryFactory(db, cfg.DynamoTableName, logger)
	if err != nil {
		logger.Fatal("failed to initialize repositories", zap.Error(err))
	}

	serviceCfg := &services.ServiceConfig{
		BaseURL:   fmt.Sprintf("https://%s", cfg.Domain),
		JWTSecret: cfg.JWTSecret,
		Config:    cfg,
	}

	registry, err := services.NewRegistry(
		services.WithStorage(repos),
		services.WithLogger(logger),
		services.WithConfig(serviceCfg),
	)
	if err != nil {
		logger.Fatal("failed to initialize services registry", zap.Error(err))
	}

	processor = NewCMSSchedulerProcessor(registry, cfg, logger)
}

func main() {
	app := lift.New()

	app.Use(middleware.PanicRecovery(logger))
	app.Use(patterns.RequestIDMiddleware("cms-scheduler"))
	app.Use(patterns.LoggingMiddleware(logger))
	app.Use(patterns.RecoveryMiddleware(logger))

	_ = app.EventBridge("*-cms-scheduler-schedule-0", func(ctx *lift.Context) error {
		if ctx.Request.RawEvent == nil {
			return lift.NewLiftError("MISSING_EVENT", "no EventBridge event in request", 400)
		}

		eventBytes, err := json.Marshal(ctx.Request.RawEvent)
		if err != nil {
			return lift.NewLiftError("EVENT_MARSHAL_ERROR", "failed to marshal raw event", 500).WithCause(err)
		}

		var event events.CloudWatchEvent
		if err := json.Unmarshal(eventBytes, &event); err != nil {
			return lift.NewLiftError("EVENT_PARSE_ERROR", "failed to parse EventBridge event", 500).WithCause(err)
		}

		return processor.HandleEvent(ctx, event)
	})

	lambda.Start(app.HandleRequest)
}

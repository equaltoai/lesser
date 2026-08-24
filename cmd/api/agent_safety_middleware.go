// Package main contains the Lesser API Lambda entrypoint and shared middleware.
package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/storage/core"
	storageModels "github.com/equaltoai/lesser/pkg/storage/models"
	storageRepos "github.com/equaltoai/lesser/pkg/storage/repositories"
	apptheory "github.com/theory-cloud/apptheory/v4/runtime"
	dynamormCore "github.com/theory-cloud/tabletheory/v3/pkg/core"
	dynamormErrors "github.com/theory-cloud/tabletheory/v3/pkg/errors"
	"go.uber.org/zap"
)

const (
	agentConcurrencyPKPrefix       = "AGENT_CONCURRENCY"
	cliAutomationConcurrencyPrefix = "CLI_CONCURRENCY"

	agentConcurrencyLimit         = 2
	agentConcurrencyLeaseDuration = 30 * time.Second

	agentErrorRateWindow      = 5 * time.Minute
	agentErrorRateThreshold   = 0.60
	agentErrorRateMinRequests = 25
	agentErrorRateCounterCap  = 100000

	agentErrorRateLockoutDuration = 10 * time.Minute

	cliAutomationDefaultConcurrencyLimit = 2
	cliAutomationDefaultBurstLimit       = 20
	cliAutomationDefaultBurstWindow      = 10 * time.Second
	cliAutomationDefaultSustainedLimit   = 60
	cliAutomationDefaultSustainedWindow  = time.Minute
	cliAutomationDefaultErrorRateWindow  = time.Minute
	cliAutomationDefaultErrorRateMin     = 10
	cliAutomationDefaultErrorRateThresh  = 0.10
	cliAutomationDefaultLockoutDuration  = time.Hour
)

var errAgentConcurrencyExceeded = errors.New("agent concurrency exceeded")

func createAgentSafetyRailsMiddleware(cfg *config.Config, repos core.RepositoryStorage, logger *zap.Logger) apptheory.Middleware {
	if cfg == nil || repos == nil {
		return func(next apptheory.Handler) apptheory.Handler { return next }
	}
	if logger == nil {
		logger = zap.NewNop()
	}

	rateRepo := safeRateLimitRepo(repos)
	if rateRepo == nil {
		return func(next apptheory.Handler) apptheory.Handler { return next }
	}

	return func(next apptheory.Handler) apptheory.Handler {
		return func(ctx *apptheory.Context) (*apptheory.Response, error) {
			claims := agentClaimsFromContext(ctx)
			if claims == nil {
				return next(ctx)
			}

			if claims.IsAgent {
				if resp, err := rejectLockedAgent(ctx, rateRepo, claims); resp != nil || err != nil {
					return resp, err
				}

				if lease, err := enforceAgentConcurrency(ctx, repos, claims); err != nil {
					if errors.Is(err, errAgentConcurrencyExceeded) {
						return apptheory.JSON(http.StatusTooManyRequests, map[string]any{
							"error":             "too_many_requests",
							"error_description": "agent concurrency limit exceeded",
						})
					}
					logger.Debug("agent concurrency check failed - allowing request", zap.Error(err))
				} else if lease != nil {
					defer lease.release(ctx.Context(), repos.GetDB())
				}

				resp, handlerErr := next(ctx)

				maybeApplyAgentErrorRateLockout(ctx, rateRepo, claims, resp, handlerErr)

				return resp, handlerErr
			}

			if isCLIAutomationClaims(claims) {
				resp, handlerErr := handleCLIAutomationSafetyRails(ctx, cfg, repos, rateRepo, logger, next, claims)
				return resp, handlerErr
			}

			return next(ctx)
		}
	}
}

func rejectLockedAgent(ctx *apptheory.Context, rateRepo *storageRepos.RateLimitRepository, claims *auth.Claims) (*apptheory.Response, error) {
	if ctx == nil || rateRepo == nil || claims == nil || strings.TrimSpace(claims.Username) == "" {
		return nil, nil
	}

	locked, unlockAt, err := rateRepo.IsRateLimited(ctx.Context(), agentLockoutIdentifier(claims.Username))
	if err != nil || !locked {
		return nil, nil
	}

	return apptheory.JSON(http.StatusTooManyRequests, map[string]any{
		"error":             "agent_locked",
		"error_description": "agent account is temporarily locked",
		"unlock_time":       unlockAt.Format(time.RFC3339),
	})
}

func enforceAgentConcurrency(ctx *apptheory.Context, repos core.RepositoryStorage, claims *auth.Claims) (*agentConcurrencyLease, error) {
	if ctx == nil || repos == nil || claims == nil {
		return nil, nil
	}

	sessionID := strings.TrimSpace(claims.AgentSessionID)
	if sessionID == "" {
		sessionID = strings.TrimSpace(claims.SessionID)
	}
	if sessionID == "" {
		return nil, nil
	}

	return acquireAgentConcurrencyLease(ctx.Context(), repos.GetDB(), sessionID, agentConcurrencyLimit, agentConcurrencyLeaseDuration)
}

func maybeApplyAgentErrorRateLockout(ctx *apptheory.Context, rateRepo *storageRepos.RateLimitRepository, claims *auth.Claims, resp *apptheory.Response, handlerErr error) {
	if ctx == nil || rateRepo == nil || claims == nil || strings.TrimSpace(claims.Username) == "" {
		return
	}

	_ = rateRepo.CheckAPIRateLimit(ctx.Context(), agentRateLimitUserID(claims.Username), "agent_request_total", agentErrorRateCounterCap, agentErrorRateWindow)
	if !agentResponseCountsTowardLockout(ctx, resp, handlerErr) {
		return
	}

	_ = rateRepo.CheckAPIRateLimit(ctx.Context(), agentRateLimitUserID(claims.Username), "agent_request_error", agentErrorRateCounterCap, agentErrorRateWindow)

	totalRemaining, _, _ := rateRepo.GetAPIRateLimitInfo(ctx.Context(), agentRateLimitUserID(claims.Username), "agent_request_total", agentErrorRateCounterCap, agentErrorRateWindow)
	errorRemaining, _, _ := rateRepo.GetAPIRateLimitInfo(ctx.Context(), agentRateLimitUserID(claims.Username), "agent_request_error", agentErrorRateCounterCap, agentErrorRateWindow)

	totalCount := agentErrorRateCounterCap - totalRemaining
	errorCount := agentErrorRateCounterCap - errorRemaining

	if totalCount < agentErrorRateMinRequests || totalCount <= 0 {
		return
	}

	errorRate := float64(errorCount) / float64(totalCount)
	if errorRate > agentErrorRateThreshold {
		_ = rateRepo.ImposeLockout(ctx.Context(), agentLockoutIdentifier(claims.Username), agentErrorRateLockoutDuration)
	}
}

func agentResponseCountsTowardLockout(ctx *apptheory.Context, resp *apptheory.Response, handlerErr error) bool {
	if handlerErr != nil {
		return true
	}
	if resp == nil {
		return false
	}

	method := strings.ToUpper(strings.TrimSpace(ctx.Request.Method))
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return false
	}

	return resp.Status >= http.StatusInternalServerError
}

type agentConcurrencyLease struct {
	pkPrefix  string
	sessionID string
	slot      int
	leaseID   string
}

func acquireAgentConcurrencyLease(ctx context.Context, db dynamormCore.DB, sessionID string, limit int, leaseDuration time.Duration) (*agentConcurrencyLease, error) {
	return acquireConcurrencyLease(ctx, db, agentConcurrencyPKPrefix, sessionID, limit, leaseDuration)
}

func acquireCLIAutomationConcurrencyLease(ctx context.Context, db dynamormCore.DB, sessionID string, limit int, leaseDuration time.Duration) (*agentConcurrencyLease, error) {
	return acquireConcurrencyLease(ctx, db, cliAutomationConcurrencyPrefix, sessionID, limit, leaseDuration)
}

func acquireConcurrencyLease(ctx context.Context, db dynamormCore.DB, pkPrefix string, sessionID string, limit int, leaseDuration time.Duration) (*agentConcurrencyLease, error) {
	if db == nil {
		return nil, nil
	}

	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" || limit <= 0 {
		return nil, nil
	}
	pkPrefix = strings.TrimSpace(pkPrefix)
	if pkPrefix == "" {
		pkPrefix = agentConcurrencyPKPrefix
	}
	if leaseDuration <= 0 {
		leaseDuration = agentConcurrencyLeaseDuration
	}

	now := time.Now().UTC()
	leaseID := common.GenerateOperationIDULID()
	pk := fmt.Sprintf("%s#%s", pkPrefix, sessionID)

	for slot := 0; slot < limit; slot++ {
		sk := fmt.Sprintf("SLOT#%d", slot)
		expiresAt := now.Add(leaseDuration)

		slotModel := &storageModels.AgentConcurrencySlot{
			PK:        pk,
			SK:        sk,
			LeaseID:   leaseID,
			ExpiresAt: expiresAt,
		}

		err := db.Model(slotModel).WithContext(ctx).IfNotExists().Create()
		if err == nil {
			return &agentConcurrencyLease{pkPrefix: pkPrefix, sessionID: sessionID, slot: slot, leaseID: leaseID}, nil
		}

		if !dynamormErrors.IsConditionFailed(err) {
			return nil, err
		}

		// Slot exists - attempt takeover if the lease is expired.
		existing := &storageModels.AgentConcurrencySlot{
			PK: pk,
			SK: sk,
		}

		claimErr := db.Model(existing).WithContext(ctx).UpdateBuilder().
			Set("LeaseID", leaseID).
			Set("ExpiresAt", expiresAt).
			Set("TTL", expiresAt.Unix()).
			Condition("TTL", "<", now.Unix()).
			Execute()

		if claimErr == nil {
			return &agentConcurrencyLease{pkPrefix: pkPrefix, sessionID: sessionID, slot: slot, leaseID: leaseID}, nil
		}
		if dynamormErrors.IsConditionFailed(claimErr) {
			continue
		}
		if dynamormErrors.IsNotFound(claimErr) {
			// Lease disappeared between create + takeover attempt; retry create once.
			if retryErr := db.Model(slotModel).WithContext(ctx).IfNotExists().Create(); retryErr == nil {
				return &agentConcurrencyLease{pkPrefix: pkPrefix, sessionID: sessionID, slot: slot, leaseID: leaseID}, nil
			}
		} else {
			return nil, claimErr
		}
	}

	return nil, errAgentConcurrencyExceeded
}

func (l *agentConcurrencyLease) release(ctx context.Context, db dynamormCore.DB) {
	if l == nil || db == nil {
		return
	}

	prefix := strings.TrimSpace(l.pkPrefix)
	if prefix == "" {
		prefix = agentConcurrencyPKPrefix
	}

	pk := fmt.Sprintf("%s#%s", prefix, l.sessionID)
	sk := fmt.Sprintf("SLOT#%d", l.slot)

	model := &storageModels.AgentConcurrencySlot{
		PK: pk,
		SK: sk,
	}

	err := db.Model(model).WithContext(ctx).WithCondition("LeaseID", "=", l.leaseID).Delete()
	_ = err
}

func agentClaimsFromContext(ctx *apptheory.Context) *auth.Claims {
	return auth.ClaimsFromAppTheoryContext(ctx)
}

func agentLockoutIdentifier(username string) string {
	username = strings.TrimSpace(username)
	if username == "" {
		return "agent:unknown"
	}
	return agentRateLimitUserID(username)
}

func agentRateLimitUserID(username string) string {
	username = strings.TrimSpace(username)
	if username == "" {
		return "agent:unknown"
	}
	return "agent:" + strings.ToLower(username)
}

func isCLIAutomationClaims(claims *auth.Claims) bool {
	if claims == nil {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(claims.ClientClass), auth.ClientClassCLI)
}

func cliAutomationSessionID(claims *auth.Claims) string {
	if claims == nil {
		return ""
	}

	if sessionID := strings.TrimSpace(claims.SessionID); sessionID != "" {
		return sessionID
	}
	if clientID := strings.TrimSpace(claims.ClientID); clientID != "" {
		return clientID
	}
	if subject := strings.TrimSpace(claims.Subject); subject != "" {
		return subject
	}
	if username := strings.TrimSpace(claims.Username); username != "" {
		return username
	}

	return ""
}

func cliAutomationLockoutIdentifier(sessionID string) string {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return "cli:unknown"
	}
	return "cli:" + strings.ToLower(sessionID)
}

func handleCLIAutomationSafetyRails(ctx *apptheory.Context, cfg *config.Config, repos core.RepositoryStorage, rateRepo *storageRepos.RateLimitRepository, logger *zap.Logger, next apptheory.Handler, claims *auth.Claims) (*apptheory.Response, error) {
	if ctx == nil || repos == nil || rateRepo == nil || next == nil || claims == nil {
		return next(ctx)
	}

	sessionID := cliAutomationSessionID(claims)
	if sessionID == "" {
		sessionID = "unknown"
	}

	if resp, err := rejectLockedCLIAutomation(ctx, rateRepo, sessionID); resp != nil || err != nil {
		return resp, err
	}

	limits := resolveCLIAutomationLimits(cfg)

	// Throttles (per session ID; stable cardinality).
	identifier := "cli:" + strings.ToLower(sessionID)
	if ok, remaining, resetAt, err := rateRepo.CheckFixedWindowRateLimit(ctx.Context(), identifier, "api_all_sustained", limits.sustainedLimit, limits.sustainedWindow); err != nil {
		logger.Debug("cli sustained throttle check failed - allowing request", zap.Error(err))
	} else if !ok {
		return cliRateLimitedResponse("cli sustained rate limit exceeded", limits.sustainedLimit, remaining, resetAt)
	}

	if ok, remaining, resetAt, err := rateRepo.CheckFixedWindowRateLimit(ctx.Context(), identifier, "api_all_burst", limits.burstLimit, limits.burstWindow); err != nil {
		logger.Debug("cli burst throttle check failed - allowing request", zap.Error(err))
	} else if !ok {
		return cliRateLimitedResponse("cli burst rate limit exceeded", limits.burstLimit, remaining, resetAt)
	}

	if lease, err := acquireCLIAutomationConcurrencyLease(ctx.Context(), repos.GetDB(), sessionID, limits.concurrencyLimit, agentConcurrencyLeaseDuration); err != nil {
		if errors.Is(err, errAgentConcurrencyExceeded) {
			return cliTooManyRequestsResponse("cli concurrency limit exceeded", 1)
		}
		logger.Debug("cli concurrency check failed - allowing request", zap.Error(err))
	} else if lease != nil {
		defer lease.release(ctx.Context(), repos.GetDB())
	}

	resp, handlerErr := next(ctx)

	maybeApplyCLIAutomationErrorRateLockout(ctx, rateRepo, sessionID, resp, handlerErr, limits.errorRateThreshold, limits.errorRateMin, limits.errorRateWindow, limits.lockoutDuration)

	return resp, handlerErr
}

type cliAutomationLimits struct {
	sustainedLimit     int
	sustainedWindow    time.Duration
	burstLimit         int
	burstWindow        time.Duration
	concurrencyLimit   int
	errorRateThreshold float64
	errorRateMin       int
	errorRateWindow    time.Duration
	lockoutDuration    time.Duration
}

func resolveCLIAutomationLimits(cfg *config.Config) cliAutomationLimits {
	limits := cliAutomationLimits{
		sustainedLimit:     cliAutomationDefaultSustainedLimit,
		sustainedWindow:    cliAutomationDefaultSustainedWindow,
		burstLimit:         cliAutomationDefaultBurstLimit,
		burstWindow:        cliAutomationDefaultBurstWindow,
		concurrencyLimit:   cliAutomationDefaultConcurrencyLimit,
		errorRateThreshold: cliAutomationDefaultErrorRateThresh,
		errorRateMin:       cliAutomationDefaultErrorRateMin,
		errorRateWindow:    cliAutomationDefaultErrorRateWindow,
		lockoutDuration:    cliAutomationDefaultLockoutDuration,
	}

	if cfg == nil {
		return limits
	}

	if cfg.CLIAutomationSustainedLimit > 0 {
		limits.sustainedLimit = cfg.CLIAutomationSustainedLimit
	}
	if cfg.CLIAutomationSustainedWindow > 0 {
		limits.sustainedWindow = cfg.CLIAutomationSustainedWindow
	}
	if cfg.CLIAutomationBurstLimit > 0 {
		limits.burstLimit = cfg.CLIAutomationBurstLimit
	}
	if cfg.CLIAutomationBurstWindow > 0 {
		limits.burstWindow = cfg.CLIAutomationBurstWindow
	}
	if cfg.CLIAutomationConcurrencyLimit > 0 {
		limits.concurrencyLimit = cfg.CLIAutomationConcurrencyLimit
	}
	if cfg.CLIAutomationErrorRateThreshold > 0 {
		limits.errorRateThreshold = cfg.CLIAutomationErrorRateThreshold
	}
	if cfg.CLIAutomationErrorRateMin > 0 {
		limits.errorRateMin = cfg.CLIAutomationErrorRateMin
	}
	if cfg.CLIAutomationErrorRateWindow > 0 {
		limits.errorRateWindow = cfg.CLIAutomationErrorRateWindow
	}
	if cfg.CLIAutomationLockoutDuration > 0 {
		limits.lockoutDuration = cfg.CLIAutomationLockoutDuration
	}

	return limits
}

func rejectLockedCLIAutomation(ctx *apptheory.Context, rateRepo *storageRepos.RateLimitRepository, sessionID string) (*apptheory.Response, error) {
	if ctx == nil || rateRepo == nil {
		return nil, nil
	}

	locked, unlockAt, err := rateRepo.IsRateLimited(ctx.Context(), cliAutomationLockoutIdentifier(sessionID))
	if err != nil || !locked {
		return nil, nil
	}

	retryAfter := ceilSeconds(time.Until(unlockAt))
	resp, respErr := apptheory.JSON(http.StatusTooManyRequests, map[string]any{
		"error":             "cli_locked",
		"error_description": "cli session is temporarily locked",
		"unlock_time":       unlockAt.Format(time.RFC3339),
	})
	if resp != nil {
		attachRateLimitHeaders(resp, rateLimitHeaders{
			retryAfterSeconds: retryAfter,
		})
	}
	return resp, respErr
}

func maybeApplyCLIAutomationErrorRateLockout(ctx *apptheory.Context, rateRepo *storageRepos.RateLimitRepository, sessionID string, resp *apptheory.Response, handlerErr error, threshold float64, minRequests int, window time.Duration, lockoutDuration time.Duration) {
	if ctx == nil || rateRepo == nil || strings.TrimSpace(sessionID) == "" {
		return
	}

	if threshold <= 0 {
		threshold = cliAutomationDefaultErrorRateThresh
	}
	if minRequests <= 0 {
		minRequests = cliAutomationDefaultErrorRateMin
	}
	if window <= 0 {
		window = cliAutomationDefaultErrorRateWindow
	}
	if lockoutDuration <= 0 {
		lockoutDuration = cliAutomationDefaultLockoutDuration
	}

	isError := handlerErr != nil || (resp != nil && resp.Status >= 400)
	_ = rateRepo.CheckAPIRateLimit(ctx.Context(), "cli:"+sessionID, "cli_request_total", agentErrorRateCounterCap, window)
	if !isError {
		return
	}

	_ = rateRepo.CheckAPIRateLimit(ctx.Context(), "cli:"+sessionID, "cli_request_error", agentErrorRateCounterCap, window)

	totalRemaining, _, _ := rateRepo.GetAPIRateLimitInfo(ctx.Context(), "cli:"+sessionID, "cli_request_total", agentErrorRateCounterCap, window)
	errorRemaining, _, _ := rateRepo.GetAPIRateLimitInfo(ctx.Context(), "cli:"+sessionID, "cli_request_error", agentErrorRateCounterCap, window)

	totalCount := agentErrorRateCounterCap - totalRemaining
	errorCount := agentErrorRateCounterCap - errorRemaining

	if totalCount < minRequests || totalCount <= 0 {
		return
	}

	errorRate := float64(errorCount) / float64(totalCount)
	if errorRate > threshold {
		_ = rateRepo.ImposeLockout(ctx.Context(), cliAutomationLockoutIdentifier(sessionID), lockoutDuration)
	}
}

type rateLimitHeaders struct {
	limit             int
	remaining         int
	resetTimeUnix     int64
	retryAfterSeconds int
}

func attachRateLimitHeaders(resp *apptheory.Response, hdr rateLimitHeaders) {
	if resp == nil {
		return
	}
	if resp.Headers == nil {
		resp.Headers = map[string][]string{}
	}

	if hdr.limit > 0 {
		resp.Headers["x-ratelimit-limit"] = []string{strconv.Itoa(hdr.limit)}
	}
	resp.Headers["x-ratelimit-remaining"] = []string{strconv.Itoa(maxInt(0, hdr.remaining))}
	if hdr.resetTimeUnix > 0 {
		resp.Headers["x-ratelimit-reset"] = []string{strconv.FormatInt(hdr.resetTimeUnix, 10)}
	}
	if hdr.retryAfterSeconds > 0 {
		resp.Headers["retry-after"] = []string{strconv.Itoa(hdr.retryAfterSeconds)}
	}
}

func cliRateLimitedResponse(description string, limit int, remaining int, resetAt time.Time) (*apptheory.Response, error) {
	retryAfter := ceilSeconds(time.Until(resetAt))
	resp, err := apptheory.JSON(http.StatusTooManyRequests, map[string]any{
		"error":             "too_many_requests",
		"error_description": description,
		"limit":             limit,
		"remaining":         maxInt(0, remaining),
		"reset_at":          resetAt.Unix(),
		"retry_after":       retryAfter,
	})
	if resp != nil {
		attachRateLimitHeaders(resp, rateLimitHeaders{
			limit:             limit,
			remaining:         remaining,
			resetTimeUnix:     resetAt.Unix(),
			retryAfterSeconds: retryAfter,
		})
	}
	return resp, err
}

func cliTooManyRequestsResponse(description string, retryAfterSeconds int) (*apptheory.Response, error) {
	if retryAfterSeconds <= 0 {
		retryAfterSeconds = 1
	}
	resp, err := apptheory.JSON(http.StatusTooManyRequests, map[string]any{
		"error":             "too_many_requests",
		"error_description": description,
		"retry_after":       retryAfterSeconds,
	})
	if resp != nil {
		attachRateLimitHeaders(resp, rateLimitHeaders{
			retryAfterSeconds: retryAfterSeconds,
		})
	}
	return resp, err
}

func ceilSeconds(d time.Duration) int {
	if d <= 0 {
		return 1
	}
	seconds := int(d / time.Second)
	if d%time.Second != 0 {
		seconds++
	}
	if seconds < 1 {
		return 1
	}
	return seconds
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func safeRateLimitRepo(repos core.RepositoryStorage) (repo *storageRepos.RateLimitRepository) {
	if repos == nil {
		return nil
	}

	defer func() {
		if recover() != nil {
			repo = nil
		}
	}()

	return repos.RateLimit()
}

// Package main contains the Lesser API Lambda entrypoint and shared middleware.
package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/storage/core"
	storageModels "github.com/equaltoai/lesser/pkg/storage/models"
	storageRepos "github.com/equaltoai/lesser/pkg/storage/repositories"
	apptheory "github.com/theory-cloud/apptheory/runtime"
	dynamormCore "github.com/theory-cloud/tabletheory/pkg/core"
	dynamormErrors "github.com/theory-cloud/tabletheory/pkg/errors"
	"go.uber.org/zap"
)

const (
	agentConcurrencyLimit         = 2
	agentConcurrencyLeaseDuration = 30 * time.Second

	agentErrorRateWindow      = time.Minute
	agentErrorRateThreshold   = 0.10
	agentErrorRateMinRequests = 10
	agentErrorRateCounterCap  = 100000

	agentErrorRateLockoutDuration = time.Hour
)

var errAgentConcurrencyExceeded = errors.New("agent concurrency exceeded")

func createOptionalOAuthAuthMiddleware(cfg *config.Config, repos core.RepositoryStorage, _ *zap.Logger) apptheory.Middleware {
	if cfg == nil || strings.TrimSpace(cfg.JWTSecret) == "" || repos == nil {
		return func(next apptheory.Handler) apptheory.Handler { return next }
	}

	oauthSvc := auth.NewOAuthService(cfg.JWTSecret, cfg, repos, nil)

	return func(next apptheory.Handler) apptheory.Handler {
		return func(ctx *apptheory.Context) (*apptheory.Response, error) {
			if ctx == nil {
				return next(ctx)
			}

			// If the primary auth middleware already populated context, don't override it.
			if ctx.Get("claims") != nil && ctx.Get("username") != nil {
				return next(ctx)
			}

			authHeader := common.ExtractAuthHeader(ctx)
			token, err := auth.ExtractBearerToken(authHeader)
			if err != nil || strings.TrimSpace(token) == "" {
				return next(ctx)
			}

			claims, err := oauthSvc.ValidateAccessToken(token)
			if err != nil || claims == nil {
				return next(ctx)
			}

			ctx.Set("claims", claims)
			ctx.Set("username", claims.Username)
			ctx.Set("is_authenticated", true)

			return next(ctx)
		}
	}
}

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
			if claims == nil || !claims.IsAgent {
				return next(ctx)
			}

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

	isError := handlerErr != nil || (resp != nil && resp.Status >= 400)
	_ = rateRepo.CheckAPIRateLimit(ctx.Context(), "agent:"+claims.Username, "agent_request_total", agentErrorRateCounterCap, agentErrorRateWindow)
	if !isError {
		return
	}

	_ = rateRepo.CheckAPIRateLimit(ctx.Context(), "agent:"+claims.Username, "agent_request_error", agentErrorRateCounterCap, agentErrorRateWindow)

	totalRemaining, _, _ := rateRepo.GetAPIRateLimitInfo(ctx.Context(), "agent:"+claims.Username, "agent_request_total", agentErrorRateCounterCap, agentErrorRateWindow)
	errorRemaining, _, _ := rateRepo.GetAPIRateLimitInfo(ctx.Context(), "agent:"+claims.Username, "agent_request_error", agentErrorRateCounterCap, agentErrorRateWindow)

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

type agentConcurrencyLease struct {
	sessionID string
	slot      int
	leaseID   string
}

func acquireAgentConcurrencyLease(ctx context.Context, db dynamormCore.DB, sessionID string, limit int, leaseDuration time.Duration) (*agentConcurrencyLease, error) {
	if db == nil {
		return nil, nil
	}

	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" || limit <= 0 {
		return nil, nil
	}
	if leaseDuration <= 0 {
		leaseDuration = agentConcurrencyLeaseDuration
	}

	now := time.Now().UTC()
	leaseID := common.GenerateOperationIDULID()
	pk := fmt.Sprintf("AGENT_CONCURRENCY#%s", sessionID)

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
			return &agentConcurrencyLease{sessionID: sessionID, slot: slot, leaseID: leaseID}, nil
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
			return &agentConcurrencyLease{sessionID: sessionID, slot: slot, leaseID: leaseID}, nil
		}
		if dynamormErrors.IsConditionFailed(claimErr) {
			continue
		}
		if dynamormErrors.IsNotFound(claimErr) {
			// Lease disappeared between create + takeover attempt; retry create once.
			if retryErr := db.Model(slotModel).WithContext(ctx).IfNotExists().Create(); retryErr == nil {
				return &agentConcurrencyLease{sessionID: sessionID, slot: slot, leaseID: leaseID}, nil
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

	pk := fmt.Sprintf("AGENT_CONCURRENCY#%s", l.sessionID)
	sk := fmt.Sprintf("SLOT#%d", l.slot)

	model := &storageModels.AgentConcurrencySlot{
		PK: pk,
		SK: sk,
	}

	err := db.Model(model).WithContext(ctx).WithCondition("LeaseID", "=", l.leaseID).Delete()
	_ = err
}

func agentClaimsFromContext(ctx *apptheory.Context) *auth.Claims {
	if ctx == nil {
		return nil
	}

	if claims, ok := ctx.Get("claims").(*auth.Claims); ok {
		return claims
	}

	return nil
}

func agentLockoutIdentifier(username string) string {
	username = strings.TrimSpace(username)
	if username == "" {
		return "agent:unknown"
	}
	return "agent:" + strings.ToLower(username)
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

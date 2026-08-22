package handlers

import (
	"context"
	cryptorand "crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"go.uber.org/zap"
)

const (
	oauthRefreshCASAttempts     = 3
	oauthRefreshCASBaseDelay    = 25 * time.Millisecond
	oauthRefreshCASDelayCap     = 200 * time.Millisecond
)

func sleepWithContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func fullJitterDuration(delayCap time.Duration) time.Duration {
	if delayCap <= 0 {
		return 0
	}
	// Duration.Nanoseconds returns the underlying int64 without a narrowing
	// conversion, and the configured caps are far below MaxInt64.
	n, err := cryptorand.Int(cryptorand.Reader, big.NewInt(delayCap.Nanoseconds()+1))
	if err != nil {
		return delayCap
	}
	return time.Duration(n.Int64())
}

func (h *Handler) refreshCASBackoff(ctx context.Context, attempt int) error {
	delayCap := oauthRefreshCASBaseDelay << attempt
	if delayCap > oauthRefreshCASDelayCap {
		delayCap = oauthRefreshCASDelayCap
	}
	jitter := h.oauthRefreshJitter
	if jitter == nil {
		jitter = fullJitterDuration
	}
	sleep := h.oauthRefreshSleep
	if sleep == nil {
		sleep = sleepWithContext
	}
	return sleep(ctx, jitter(delayCap))
}

func (h *Handler) retryRefreshCAS(ctx context.Context, attempt int) (bool, error) {
	if attempt+1 >= oauthRefreshCASAttempts {
		return false, nil
	}
	if err := h.refreshCASBackoff(ctx, attempt); err != nil {
		return false, err
	}
	return true, nil
}

var errOAuthRefreshCASContention = errors.New("refresh CAS contention")

type standardRefreshRotation struct {
	fresh            *storage.RefreshToken
	currentAuthority *storagemodels.OAuthRefreshAuthority
	now              time.Time
	refreshExpiry    time.Time
	accessTTL        time.Duration
	familyID         string
	clientClass      string
	delegatedBy      string
	generation       int
}

func (h *Handler) prepareStandardRefreshRotation(
	ctx context.Context,
	client *storage.OAuthClient,
	rawToken, clientID, requestedResource string,
	telemetry *oauthGrantTelemetry,
) (*standardRefreshRotation, error) {
	fresh, err := h.repos.Account().GetRefreshTokenConsistent(ctx, rawToken)
	if err != nil {
		return nil, fmt.Errorf("load refresh head for CAS: %w", err)
	}
	freshHasLineage := strings.TrimSpace(fresh.FamilyID) != "" && fresh.Generation > 0
	if fresh.Revoked || !fresh.RevokedAt.IsZero() || (freshHasLineage && !fresh.Current) {
		return nil, errOAuthRefreshCASContention
	}

	now := time.Now().UTC()
	reason, err := h.standardRefreshAuthorityRevocationReason(ctx, client, fresh, requestedResource, now)
	if err != nil {
		return nil, err
	}
	if reason != "" {
		telemetry.setReason(reason)
		return nil, auth.ErrInvalidToken
	}
	accessTTL, refreshExpiry, clientClass, err := standardRefreshGrantLifetimes(now, h.cfg, client, fresh)
	if err != nil {
		telemetry.setReason(oauthGrantReasonRefreshTokenExpired)
		return nil, auth.ErrInvalidToken
	}
	delegatedBy, err := h.validateRefreshDelegation(ctx, fresh, telemetry)
	if err != nil {
		return nil, err
	}

	familyID := strings.TrimSpace(fresh.FamilyID)
	generation := fresh.Generation + 1
	if familyID == "" || fresh.Generation < 1 {
		familyID = common.GenerateSessionIDULID()
		generation = 2
	}
	currentAuthority, err := h.repos.Account().GetOAuthRefreshAuthority(ctx, fresh.Username, clientID, fresh.Resource)
	if err != nil && !errors.Is(err, storage.ErrNotFound) {
		return nil, err
	}
	if slot, ok := refreshAuthoritySlot(currentAuthority, familyID); ok &&
		(slot.HeadTokenHash != storage.RefreshTokenReplacementHash(fresh.Token) || slot.Generation != fresh.Generation) {
		return nil, errors.Join(errOAuthRefreshCASContention, errors.New("refresh authority head changed"))
	}

	return &standardRefreshRotation{
		fresh: fresh, currentAuthority: currentAuthority, now: now, refreshExpiry: refreshExpiry,
		accessTTL: accessTTL, familyID: familyID, clientClass: clientClass,
		delegatedBy: delegatedBy, generation: generation,
	}, nil
}

func (h *Handler) mintStandardRefreshSuccessor(
	ctx context.Context,
	oauthSvc *auth.OAuthService,
	rotation *standardRefreshRotation,
	ipAddress string,
	telemetry *oauthGrantTelemetry,
) (string, string, *storage.RefreshToken, error) {
	fresh := rotation.fresh
	accessToken, nextRaw, err := oauthSvc.GenerateRefreshTokensWithAuthoritativeSubject(
		ctx, fresh.Username, fresh.ClientID, ipAddress, fresh.Scopes, rotation.accessTTL, rotation.clientClass,
		fresh.SessionID, fresh.Resource, rotation.delegatedBy,
	)
	if err != nil {
		if errors.Is(err, auth.ErrInvalidToken) {
			telemetry.setReason(oauthGrantReasonRefreshAuthorityAbsent)
			return "", "", nil, auth.ErrInvalidToken
		}
		telemetry.setReason(oauthGrantReasonTokenGenerationFailed)
		return "", "", nil, errors.Join(auth.ErrOAuthTemporarilyUnavailable, err)
	}
	next := refreshSuccessorFromFresh(
		fresh, nextRaw, rotation.delegatedBy, rotation.familyID, rotation.generation,
		rotation.clientClass, rotation.refreshExpiry, rotation.now,
	)
	return accessToken, nextRaw, next, nil
}

type standardRefreshStartResult struct {
	accessToken  string
	refreshToken string
	scopes       []string
	err          error
}

func (h *Handler) resolveStandardRefreshStart(
	ctx context.Context,
	oauthSvc *auth.OAuthService,
	client *storage.OAuthClient,
	rawToken, clientID, requestedResource, ipAddress, userAgent string,
	telemetry *oauthGrantTelemetry,
) *standardRefreshStartResult {
	initial, err := h.repos.Account().GetRefreshTokenConsistent(ctx, rawToken)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			telemetry.setReason(oauthGrantReasonRefreshTokenAbsent)
			return &standardRefreshStartResult{err: auth.ErrInvalidToken}
		}
		_, _, _, unavailable := h.refreshUnavailable(telemetry, err)
		return &standardRefreshStartResult{err: unavailable}
	}
	telemetry.FamilyID = strings.TrimSpace(initial.FamilyID)
	telemetry.setResourceIfEmpty(initial.Resource)
	if initial.ClientID != clientID {
		telemetry.setReason("refresh_cross_client_replay")
		// The client binding is authoritative before any replay decision. Keep
		// the existing containment sweep for a credential presented by another
		// client; GSI2 is used only as best-effort reconciliation evidence here,
		// never to discover or authorize the live refresh head.
		h.revokeStandardRefreshFamily(ctx, initial, "refresh_cross_client_replay", ipAddress, userAgent)
		return &standardRefreshStartResult{err: auth.ErrInvalidToken}
	}
	if initial.Revoked || !initial.RevokedAt.IsZero() {
		telemetry.setReason(oauthGrantReasonRefreshStaleGeneration)
		return &standardRefreshStartResult{err: auth.ErrInvalidToken}
	}
	if strings.TrimSpace(initial.FamilyID) != "" && initial.Generation > 0 && !initial.Current {
		telemetry.setReason(oauthGrantReasonRefreshStaleGeneration)
		return &standardRefreshStartResult{err: auth.ErrInvalidToken}
	}
	return nil
}

func (h *Handler) exchangeStandardRefreshTokenCAS(
	ctx context.Context,
	oauthSvc *auth.OAuthService,
	client *storage.OAuthClient,
	rawToken, clientID, requestedResource, ipAddress, userAgent string,
	telemetry *oauthGrantTelemetry,
) (string, string, []string, error) {
	resolved := h.resolveStandardRefreshStart(
		ctx, oauthSvc, client, rawToken, clientID, requestedResource, ipAddress, userAgent, telemetry,
	)
	if resolved != nil {
		return resolved.accessToken, resolved.refreshToken, resolved.scopes, resolved.err
	}

	for attempt := 0; attempt < oauthRefreshCASAttempts; attempt++ {
		rotation, prepareErr := h.prepareStandardRefreshRotation(ctx, client, rawToken, clientID, requestedResource, telemetry)
		if prepareErr != nil {
			if errors.Is(prepareErr, errOAuthRefreshCASContention) {
				retry, backoffErr := h.retryRefreshCAS(ctx, attempt)
				if backoffErr != nil {
					return h.refreshUnavailable(telemetry, backoffErr)
				}
				if retry {
					continue
				}
				return h.refreshUnavailable(telemetry, prepareErr)
			}
			if errors.Is(prepareErr, auth.ErrInvalidToken) {
				return "", "", nil, auth.ErrInvalidToken
			}
			return h.refreshUnavailable(telemetry, prepareErr)
		}

		accessToken, nextRaw, next, mintErr := h.mintStandardRefreshSuccessor(ctx, oauthSvc, rotation, ipAddress, telemetry)
		if mintErr != nil {
			return "", "", nil, mintErr
		}
		nextAuthority := repositories.OAuthRefreshAuthorityWithHead(
			rotation.currentAuthority, rotation.fresh.Username, clientID, rotation.fresh.Resource, rotation.familyID,
			storage.RefreshTokenReplacementHash(nextRaw), rotation.generation, rotation.refreshExpiry, rotation.now,
		)
		if h.oauthRefreshBeforeCAS != nil {
			h.oauthRefreshBeforeCAS()
		}
		rotateErr := h.repos.Account().RotateRefreshTokenWithAuthority(
			ctx, rotation.fresh, next, rotation.currentAuthority, nextAuthority, rotation.now,
		)
		if rotateErr == nil {
			telemetry.FamilyID = rotation.familyID
			return accessToken, nextRaw, rotation.fresh.Scopes, nil
		}
		if !repositories.IsOAuthRefreshCASConflict(rotateErr) {
			h.logger.Warn("refresh authority transaction failed", zap.Error(rotateErr))
			return h.refreshUnavailable(telemetry, rotateErr)
		}
		if _, backoffErr := h.retryRefreshCAS(ctx, attempt); backoffErr != nil {
			return h.refreshUnavailable(telemetry, backoffErr)
		}
	}
	return h.refreshUnavailable(telemetry, errors.New("refresh CAS retries exhausted"))
}

func (h *Handler) validateRefreshDelegation(ctx context.Context, token *storage.RefreshToken, telemetry *oauthGrantTelemetry) (string, error) {
	delegatedBy := strings.TrimSpace(token.PrincipalUsername)
	if !oauthResourceTargetsActorMCP(token.Resource) {
		return delegatedBy, nil
	}
	if delegatedBy == "" {
		telemetry.setReason(oauthGrantReasonRefreshAuthorityAbsent)
		return "", auth.ErrInvalidToken
	}
	authorized, err := h.agentRefreshGrantAuthorized(ctx, token.Username, delegatedBy)
	if err != nil {
		return "", errors.Join(auth.ErrOAuthTemporarilyUnavailable, err)
	}
	if !authorized {
		telemetry.setReason("refresh_share_grant_revoked")
		return "", auth.ErrInvalidToken
	}
	return delegatedBy, nil
}

func refreshSuccessorFromFresh(fresh *storage.RefreshToken, raw, delegatedBy, familyID string, generation int, clientClass string, expiry, now time.Time) *storage.RefreshToken {
	next := &storage.RefreshToken{
		Token: raw, Username: fresh.Username, PrincipalUsername: delegatedBy,
		ClientID: fresh.ClientID, Resource: fresh.Resource, Scopes: fresh.Scopes,
		CreatedAt: now, ExpiresAt: expiry, ClientClass: clientClass,
		SessionID: fresh.SessionID, FamilyID: familyID, Generation: generation, Current: true,
		AccessTTLSeconds: fresh.AccessTTLSeconds, DeviceLabel: fresh.DeviceLabel,
		IdleExpiresAt: fresh.IdleExpiresAt, AbsoluteExpiresAt: fresh.AbsoluteExpiresAt,
		SessionCreatedAt: fresh.SessionCreatedAt, LastAuthFailureCode: fresh.LastAuthFailureCode,
		LastAuthFailureAt: fresh.LastAuthFailureAt, LastAuthFailureMsg: fresh.LastAuthFailureMsg,
		LastAuthSuccessAt: now,
	}
	if !fresh.LastUsedAt.IsZero() {
		next.LastUsedAt = now
	}
	return next
}

func refreshAuthoritySlot(authority *storagemodels.OAuthRefreshAuthority, familyID string) (storagemodels.OAuthRefreshFamilySlot, bool) {
	if authority == nil {
		return storagemodels.OAuthRefreshFamilySlot{}, false
	}
	for i := len(authority.Slots) - 1; i >= 0; i-- {
		if authority.Slots[i].FamilyID == familyID {
			return authority.Slots[i], true
		}
	}
	return storagemodels.OAuthRefreshFamilySlot{}, false
}


func (h *Handler) refreshUnavailable(telemetry *oauthGrantTelemetry, err error) (string, string, []string, error) {
	telemetry.setReason(oauthGrantReasonTemporarilyUnavailable)
	return "", "", nil, errors.Join(auth.ErrOAuthTemporarilyUnavailable, fmt.Errorf("refresh reliability path: %w", err))
}

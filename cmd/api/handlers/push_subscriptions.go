package handlers

import (
	"net/http"

	"github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage"
	apptheory "github.com/theory-cloud/apptheory/runtime"
	"go.uber.org/zap"
)

// HandleGetPushSubscriptionLift handles GET /api/v1/push/subscription
func (h *Handler) HandleGetPushSubscriptionLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	token := h.getBearerTokenLift(ctx)
	if err := common.ValidateRequiredParam("token", token); err != nil {
		return common.RespondMissingAuth(ctx)
	}

	// Validate token
	oauthSvc := createOAuthService(h.cfg.JWTSecret, h.cfg, h.repos, h.logger)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return common.RespondUnauthorized(ctx, "invalid token")
	}

	// Check push scope
	if !claims.HasScope("push") && !claims.HasScope(auth.ScopeRead) {
		return common.RespondForbidden(ctx, "insufficient scope")
	}

	// Get user's push subscriptions
	subscriptions, err := h.repos.PushSubscription().GetUserPushSubscriptions(ctx.Context(), claims.Username)
	if err != nil {
		h.logger.Error("failed to get push subscriptions",
			zap.String("username", claims.Username),
			zap.Error(err))
		return common.RespondInternalServerError(ctx, "failed to get push subscription")
	}

	// If no subscriptions, return empty response
	if err := common.ValidateSliceNotEmpty("subscriptions", subscriptions); err != nil {
		return okJSON(map[string]any{
			"id":       "",
			"endpoint": "",
			"alerts": map[string]bool{
				"follow":         false,
				"favourite":      false,
				"reblog":         false,
				"mention":        false,
				"poll":           false,
				"follow_request": false,
				"status":         false,
				"update":         false,
			},
			"server_key": "",
		})
	}

	// Return the first subscription (Mastodon only supports one per user)
	sub := subscriptions[0]

	// Get VAPID public key
	var serverKey string
	vapidKeys, err := h.repos.PushSubscription().GetVAPIDKeys(ctx.Context())
	if err != nil {
		h.logger.Warn("failed to get VAPID keys", zap.Error(err))
		serverKey = ""
	} else {
		serverKey = vapidKeys.PublicKey
	}

	// Convert to API format
	resp := models.PushSubscription{
		ID:       sub.ID,
		Endpoint: sub.Endpoint,
		Keys: models.PushSubscriptionKeys{
			P256dh: sub.P256dh,
			Auth:   sub.Auth,
		},
		Alerts: models.PushSubscriptionAlerts{
			Follow:        sub.Alerts.Follow,
			Favourite:     sub.Alerts.Favourite,
			Reblog:        sub.Alerts.Reblog,
			Mention:       sub.Alerts.Mention,
			Poll:          sub.Alerts.Poll,
			FollowRequest: sub.Alerts.FollowRequest,
			Status:        sub.Alerts.Status,
			Update:        sub.Alerts.Update,
			AdminSignUp:   sub.Alerts.AdminSignUp,
			AdminReport:   sub.Alerts.AdminReport,
		},
		Policy:    sub.Policy,
		ServerKey: serverKey,
	}

	return okJSON(resp)
}

// HandleCreatePushSubscriptionLift handles POST /api/v1/push/subscription
func (h *Handler) HandleCreatePushSubscriptionLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	token := h.getBearerTokenLift(ctx)
	if err := common.ValidateRequiredParam("token", token); err != nil {
		return common.RespondMissingAuth(ctx)
	}

	// Validate token
	oauthSvc := createOAuthService(h.cfg.JWTSecret, h.cfg, h.repos, h.logger)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return common.RespondUnauthorized(ctx, "invalid token")
	}

	// Check push scope
	if !claims.HasScope("push") && !claims.HasScope(auth.ScopeWrite) {
		return common.RespondForbidden(ctx, "insufficient scope")
	}

	// Parse request
	var req models.PushSubscriptionRequest
	if err := common.ParseRequestWithFallback(ctx, &req); err != nil {
		return common.RespondBadRequest(ctx, "invalid request body")
	}

	// Validate request using centralized validation
	if err := common.ValidateRequiredParam("endpoint", req.Subscription.Endpoint); err != nil {
		return apptheory.JSON(http.StatusUnprocessableEntity, map[string]string{"error": "endpoint is required"})
	}
	if err := common.ValidateRequiredParam("p256dh", req.Subscription.Keys.P256dh); err != nil {
		return apptheory.JSON(http.StatusUnprocessableEntity, map[string]string{"error": "keys.p256dh is required"})
	}
	if err := common.ValidateRequiredParam("auth", req.Subscription.Keys.Auth); err != nil {
		return apptheory.JSON(http.StatusUnprocessableEntity, map[string]string{"error": "keys.auth is required"})
	}

	// Delete any existing subscriptions for this user
	if err := h.repos.PushSubscription().DeleteAllPushSubscriptions(ctx.Context(), claims.Username); err != nil {
		h.logger.Warn("failed to delete existing push subscriptions",
			zap.String("username", claims.Username),
			zap.Error(err))
	}

	// Create new subscription
	subscription := &storage.PushSubscription{
		Username: claims.Username,
		Endpoint: req.Subscription.Endpoint,
		P256dh:   req.Subscription.Keys.P256dh,
		Auth:     req.Subscription.Keys.Auth,
		Alerts: storage.PushSubscriptionAlerts{
			Follow:        req.Data.Follow,
			Favourite:     req.Data.Favourite,
			Reblog:        req.Data.Reblog,
			Mention:       req.Data.Mention,
			Poll:          req.Data.Poll,
			FollowRequest: req.Data.FollowRequest,
			Status:        req.Data.Status,
			Update:        req.Data.Update,
			AdminSignUp:   req.Data.AdminSignUp,
			AdminReport:   req.Data.AdminReport,
		},
		Policy: "all", // Default policy
	}

	if err := h.repos.PushSubscription().CreatePushSubscription(ctx.Context(), claims.Username, subscription); err != nil {
		h.logger.Error("failed to create push subscription",
			zap.String("username", claims.Username),
			zap.Error(err))
		return common.RespondInternalServerError(ctx, "failed to create push subscription")
	}

	// Get VAPID public key
	var serverKey string
	vapidKeys, err := h.repos.PushSubscription().GetVAPIDKeys(ctx.Context())
	if err != nil {
		h.logger.Warn("failed to get VAPID keys", zap.Error(err))
		serverKey = ""
	} else {
		serverKey = vapidKeys.PublicKey
	}

	// Return response
	resp := models.PushSubscription{
		ID:       subscription.ID,
		Endpoint: subscription.Endpoint,
		Keys: models.PushSubscriptionKeys{
			P256dh: subscription.P256dh,
			Auth:   subscription.Auth,
		},
		Alerts:    req.Data,
		Policy:    subscription.Policy,
		ServerKey: serverKey,
	}

	return okJSON(resp)
}

// HandleUpdatePushSubscriptionLift handles PUT /api/v1/push/subscription
func (h *Handler) HandleUpdatePushSubscriptionLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	token := h.getBearerTokenLift(ctx)
	if err := common.ValidateRequiredParam("token", token); err != nil {
		return common.RespondMissingAuth(ctx)
	}

	// Validate token
	oauthSvc := createOAuthService(h.cfg.JWTSecret, h.cfg, h.repos, h.logger)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return common.RespondUnauthorized(ctx, "invalid token")
	}

	// Check push scope
	if !claims.HasScope("push") && !claims.HasScope(auth.ScopeWrite) {
		return common.RespondForbidden(ctx, "insufficient scope")
	}

	// Parse request (only data field for updates)
	var req models.PushSubscriptionRequest
	if err := common.ParseRequestWithFallback(ctx, &req); err != nil {
		return common.RespondBadRequest(ctx, "invalid request body")
	}

	// Get existing subscription
	subscriptions, err := h.repos.PushSubscription().GetUserPushSubscriptions(ctx.Context(), claims.Username)
	if err != nil || len(subscriptions) == 0 {
		return apptheory.JSON(http.StatusNotFound, map[string]string{"error": "push subscription not found"})
	}

	sub := subscriptions[0]

	// Update alerts
	alerts := storage.PushSubscriptionAlerts{
		Follow:        req.Data.Follow,
		Favourite:     req.Data.Favourite,
		Reblog:        req.Data.Reblog,
		Mention:       req.Data.Mention,
		Poll:          req.Data.Poll,
		FollowRequest: req.Data.FollowRequest,
		Status:        req.Data.Status,
		Update:        req.Data.Update,
		AdminSignUp:   req.Data.AdminSignUp,
		AdminReport:   req.Data.AdminReport,
	}

	if err := h.repos.PushSubscription().UpdatePushSubscription(ctx.Context(), claims.Username, sub.ID, alerts); err != nil {
		h.logger.Error("failed to update push subscription",
			zap.String("username", claims.Username),
			zap.String("subscription_id", sub.ID),
			zap.Error(err))
		return common.RespondInternalServerError(ctx, "failed to update push subscription")
	}

	// Get VAPID public key
	var serverKey string
	vapidKeys, err := h.repos.PushSubscription().GetVAPIDKeys(ctx.Context())
	if err != nil {
		h.logger.Warn("failed to get VAPID keys", zap.Error(err))
		serverKey = ""
	} else {
		serverKey = vapidKeys.PublicKey
	}

	// Return updated subscription
	resp := models.PushSubscription{
		ID:       sub.ID,
		Endpoint: sub.Endpoint,
		Keys: models.PushSubscriptionKeys{
			P256dh: sub.P256dh,
			Auth:   sub.Auth,
		},
		Alerts:    req.Data,
		Policy:    sub.Policy,
		ServerKey: serverKey,
	}

	return okJSON(resp)
}

// HandleDeletePushSubscriptionLift handles DELETE /api/v1/push/subscription
func (h *Handler) HandleDeletePushSubscriptionLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	token := h.getBearerTokenLift(ctx)
	if err := common.ValidateRequiredParam("token", token); err != nil {
		return common.RespondMissingAuth(ctx)
	}

	// Validate token
	oauthSvc := createOAuthService(h.cfg.JWTSecret, h.cfg, h.repos, h.logger)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return common.RespondUnauthorized(ctx, "invalid token")
	}

	// Check push scope
	if !claims.HasScope("push") && !claims.HasScope(auth.ScopeWrite) {
		return common.RespondForbidden(ctx, "insufficient scope")
	}

	// Delete all push subscriptions for this user
	if err := h.repos.PushSubscription().DeleteAllPushSubscriptions(ctx.Context(), claims.Username); err != nil {
		h.logger.Error("failed to delete push subscriptions",
			zap.String("username", claims.Username),
			zap.Error(err))
		return common.RespondInternalServerError(ctx, "failed to delete push subscription")
	}

	return noContent(), nil
}

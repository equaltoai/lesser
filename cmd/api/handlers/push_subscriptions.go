package handlers

import (
	"context"
	"errors"
	"fmt"

	"github.com/aron23/lesser/cmd/api/models"
	"github.com/aron23/lesser/pkg/auth"
	"github.com/aron23/lesser/pkg/common"
	"github.com/aron23/lesser/pkg/storage"
	"github.com/aws/aws-lambda-go/events"
	"go.uber.org/zap"
)

// HandleGetPushSubscription handles GET /api/v1/push/subscription
func (h *Handler) HandleGetPushSubscription(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	// Extract token
	token, err := auth.ExtractBearerToken(request.Headers["Authorization"])
	if err != nil {
		token, err = auth.ExtractBearerToken(request.Headers["authorization"])
		if err != nil {
			return common.Unauthorized(err), nil
		}
	}

	// Validate token
	oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.store)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	// Check push scope
	if !claims.HasScope("push") && !claims.HasScope(auth.ScopeRead) {
		return common.Forbidden(errors.New("insufficient scope")), nil
	}

	// Get user's push subscriptions
	subscriptions, err := h.store.GetUserPushSubscriptions(ctx, claims.Username)
	if err != nil {
		h.logger.Error("failed to get push subscriptions",
			zap.String("username", claims.Username),
			zap.Error(err))
		return common.InternalServerError(fmt.Errorf("failed to get push subscription")), nil
	}

	// If no subscriptions, return empty response
	if len(subscriptions) == 0 {
		return common.OK(map[string]any{
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
		}), nil
	}

	// Return the first subscription (Mastodon only supports one per user)
	sub := subscriptions[0]

	// Get VAPID public key
	var serverKey string
	vapidKeys, err := h.store.GetVAPIDKeys(ctx)
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

	return common.OK(resp), nil
}

// HandleCreatePushSubscription handles POST /api/v1/push/subscription
func (h *Handler) HandleCreatePushSubscription(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	// Extract token
	token, err := auth.ExtractBearerToken(request.Headers["Authorization"])
	if err != nil {
		token, err = auth.ExtractBearerToken(request.Headers["authorization"])
		if err != nil {
			return common.Unauthorized(err), nil
		}
	}

	// Validate token
	oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.store)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	// Check push scope
	if !claims.HasScope("push") && !claims.HasScope(auth.ScopeWrite) {
		return common.Forbidden(errors.New("insufficient scope")), nil
	}

	// Parse request
	var req models.PushSubscriptionRequest
	if err := common.ParseRequestBody([]byte(request.Body), &req); err != nil {
		return common.BadRequest(err), nil
	}

	// Validate request
	if req.Subscription.Endpoint == "" {
		return common.UnprocessableEntity(errors.New("endpoint is required")), nil
	}
	if req.Subscription.Keys.P256dh == "" {
		return common.UnprocessableEntity(errors.New("keys.p256dh is required")), nil
	}
	if req.Subscription.Keys.Auth == "" {
		return common.UnprocessableEntity(errors.New("keys.auth is required")), nil
	}

	// Delete any existing subscriptions for this user
	if err := h.store.DeleteAllPushSubscriptions(ctx, claims.Username); err != nil {
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

	if err := h.store.CreatePushSubscription(ctx, claims.Username, subscription); err != nil {
		h.logger.Error("failed to create push subscription",
			zap.String("username", claims.Username),
			zap.Error(err))
		return common.InternalServerError(fmt.Errorf("failed to create push subscription")), nil
	}

	// Get VAPID public key
	var serverKey string
	vapidKeys, err := h.store.GetVAPIDKeys(ctx)
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

	return common.OK(resp), nil
}

// HandleUpdatePushSubscription handles PUT /api/v1/push/subscription
func (h *Handler) HandleUpdatePushSubscription(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	// Extract token
	token, err := auth.ExtractBearerToken(request.Headers["Authorization"])
	if err != nil {
		token, err = auth.ExtractBearerToken(request.Headers["authorization"])
		if err != nil {
			return common.Unauthorized(err), nil
		}
	}

	// Validate token
	oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.store)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	// Check push scope
	if !claims.HasScope("push") && !claims.HasScope(auth.ScopeWrite) {
		return common.Forbidden(errors.New("insufficient scope")), nil
	}

	// Parse request (only data field for updates)
	var req struct {
		Data models.PushSubscriptionAlerts `json:"data"`
	}
	if err := common.ParseRequestBody([]byte(request.Body), &req); err != nil {
		return common.BadRequest(err), nil
	}

	// Get existing subscription
	subscriptions, err := h.store.GetUserPushSubscriptions(ctx, claims.Username)
	if err != nil || len(subscriptions) == 0 {
		return common.NotFound(fmt.Errorf("push subscription not found")), nil
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

	if err := h.store.UpdatePushSubscription(ctx, claims.Username, sub.ID, alerts); err != nil {
		h.logger.Error("failed to update push subscription",
			zap.String("username", claims.Username),
			zap.String("subscription_id", sub.ID),
			zap.Error(err))
		return common.InternalServerError(fmt.Errorf("failed to update push subscription")), nil
	}

	// Get VAPID public key
	var serverKey string
	vapidKeys, err := h.store.GetVAPIDKeys(ctx)
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

	return common.OK(resp), nil
}

// HandleDeletePushSubscription handles DELETE /api/v1/push/subscription
func (h *Handler) HandleDeletePushSubscription(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	// Extract token
	token, err := auth.ExtractBearerToken(request.Headers["Authorization"])
	if err != nil {
		token, err = auth.ExtractBearerToken(request.Headers["authorization"])
		if err != nil {
			return common.Unauthorized(err), nil
		}
	}

	// Validate token
	oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.store)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	// Check push scope
	if !claims.HasScope("push") && !claims.HasScope(auth.ScopeWrite) {
		return common.Forbidden(errors.New("insufficient scope")), nil
	}

	// Delete all push subscriptions for this user
	if err := h.store.DeleteAllPushSubscriptions(ctx, claims.Username); err != nil {
		h.logger.Error("failed to delete push subscriptions",
			zap.String("username", claims.Username),
			zap.Error(err))
		return common.InternalServerError(fmt.Errorf("failed to delete push subscription")), nil
	}

	return common.NoContent(), nil
}

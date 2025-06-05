package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/mail"
	"regexp"
	"strings"
	"time"

	"github.com/aron23/lesser/cmd/api/models"
	"github.com/aron23/lesser/pkg/activitypub"
	"github.com/aron23/lesser/pkg/auth"
	"github.com/aron23/lesser/pkg/common"
	"github.com/aron23/lesser/pkg/federation"
	"github.com/aron23/lesser/pkg/storage"
	"github.com/aws/aws-lambda-go/events"
	"go.uber.org/zap"
)

// HandleRegistration handles user registration requests
func (h *Handler) HandleRegistration(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	// Parse request
	var req models.AccountRegistrationRequest
	if err := json.Unmarshal([]byte(request.Body), &req); err != nil {
		return common.BadRequest(err), nil
	}

	// Validate request
	if err := h.validateRegistrationRequest(req); err != nil {
		errResp := map[string]interface{}{
			"error": err.Error(),
		}
		body, _ := json.Marshal(errResp)
		return &events.APIGatewayV2HTTPResponse{
			StatusCode: http.StatusUnprocessableEntity,
			Headers:    common.Headers(),
			Body:       string(body),
		}, nil
	}

	// Hash password
	passwordHash, err := auth.HashPassword(req.Password)
	if err != nil {
		errResp := map[string]interface{}{
			"error": err.Error(),
		}
		body, _ := json.Marshal(errResp)
		return &events.APIGatewayV2HTTPResponse{
			StatusCode: http.StatusUnprocessableEntity,
			Headers:    common.Headers(),
			Body:       string(body),
		}, nil
	}

	// Create user
	user := &storage.User{
		Username:     req.Username,
		Email:        req.Email,
		PasswordHash: passwordHash,
		Approved:     true, // Auto-approve for now, can be changed to require admin approval
		Suspended:    false,
		Role:         "user",
		Locale:       req.Locale,
	}

	if err := h.store.CreateUser(ctx, user); err != nil {
		if strings.Contains(err.Error(), "already exists") {
			errResp := map[string]interface{}{
				"error": "Username is already taken",
			}
			body, _ := json.Marshal(errResp)
			return &events.APIGatewayV2HTTPResponse{
				StatusCode: http.StatusUnprocessableEntity,
				Headers:    common.Headers(),
				Body:       string(body),
			}, nil
		}
		h.logger.Error("failed to create user", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	// Generate RSA keypair for the actor
	privateKey, err := federation.GenerateRSAKeyPair(2048)
	if err != nil {
		h.logger.Error("failed to generate RSA keypair", zap.Error(err))
		// Clean up user since we couldn't create the actor
		_ = h.store.DeleteUser(ctx, user.Username)
		return common.InternalServerError(err), nil
	}

	// Encode public key to PEM format
	publicKeyPEM, err := federation.EncodePublicKeyPEM(&privateKey.PublicKey)
	if err != nil {
		h.logger.Error("failed to encode public key", zap.Error(err))
		// Clean up user since we couldn't create the actor
		_ = h.store.DeleteUser(ctx, user.Username)
		return common.InternalServerError(err), nil
	}

	// Encode private key to PEM format
	privateKeyPEM, err := federation.EncodePrivateKeyPEM(privateKey)
	if err != nil {
		h.logger.Error("failed to encode private key", zap.Error(err))
		// Clean up user since we couldn't create the actor
		_ = h.store.DeleteUser(ctx, user.Username)
		return common.InternalServerError(err), nil
	}

	// Create corresponding actor
	actorID := fmt.Sprintf("%s/users/%s", h.cfg.BaseURL(), user.Username)
	actor := activitypub.NewActor(activitypub.PersonType, actorID, user.Username)
	actor.Name = user.Username
	actor.URL = fmt.Sprintf("%s/@%s", h.cfg.BaseURL(), user.Username)
	actor.PublicKey = &activitypub.PublicKey{
		ID:           fmt.Sprintf("%s#main-key", actorID),
		Owner:        actorID,
		PublicKeyPem: string(publicKeyPEM),
	}

	// Create the actor
	if err := h.store.CreateActor(ctx, actor, string(privateKeyPEM)); err != nil {
		h.logger.Error("failed to create actor", zap.Error(err))
		// Clean up user since we couldn't create the actor
		_ = h.store.DeleteUser(ctx, user.Username)
		return common.InternalServerError(err), nil
	}

	// Return response
	resp := models.AccountRegistrationResponse{
		ID:       actor.ID,
		Username: user.Username,
		Email:    user.Email,
		Created:  true,
	}

	body, _ := json.Marshal(resp)
	return &events.APIGatewayV2HTTPResponse{
		StatusCode: http.StatusCreated,
		Headers: map[string]string{
			"Content-Type": "application/json",
		},
		Body: string(body),
	}, nil
}

// HandleVerifyCredentials returns the current user's information
func (h *Handler) HandleVerifyCredentials(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	// Extract token from Authorization header
	authHeader := request.Headers["Authorization"]
	if authHeader == "" {
		authHeader = request.Headers["authorization"]
	}

	token, err := auth.ExtractBearerToken(authHeader)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	// Validate token and get claims
	oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.store)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	// Get user
	user, err := h.store.GetUser(ctx, claims.Username)
	if err != nil {
		h.logger.Error("failed to get user", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	// Get actor
	actor, err := h.store.GetActor(ctx, user.Username)
	if err != nil {
		h.logger.Error("failed to get actor", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	// TODO: Get real counts
	resp := models.VerifyCredentialsResponse{
		ID:             actor.ID,
		Username:       user.Username,
		DisplayName:    actor.Name,
		Email:          user.Email,
		EmailVerified:  true, // TODO: Implement email verification
		Note:           actor.Summary,
		URL:            actor.URL,
		Avatar:         "", // TODO: Implement avatars
		AvatarStatic:   "",
		Header:         "",
		HeaderStatic:   "",
		FollowersCount: 0,
		FollowingCount: 0,
		StatusesCount:  0,
		CreatedAt:      user.CreatedAt.Format("2006-01-02T15:04:05.000Z"),
		Role:           user.Role,
		Source: map[string]interface{}{
			"privacy":   "public",
			"sensitive": false,
			"language":  user.Locale,
			"fields":    []interface{}{},
		},
	}

	body, _ := json.Marshal(resp)
	return &events.APIGatewayV2HTTPResponse{
		StatusCode: http.StatusOK,
		Headers: map[string]string{
			"Content-Type": "application/json",
		},
		Body: string(body),
	}, nil
}

// HandleUpdateCredentials updates the current user's profile
func (h *Handler) HandleUpdateCredentials(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	// Extract token from Authorization header
	authHeader := request.Headers["Authorization"]
	if authHeader == "" {
		authHeader = request.Headers["authorization"]
	}

	token, err := auth.ExtractBearerToken(authHeader)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	// Validate token and get claims
	oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.store)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	// Check write scope
	if err := h.authMiddleware.RequireScope(claims, auth.ScopeWrite); err != nil {
		return common.Forbidden(err), nil
	}

	// Get the actor
	actor, err := h.store.GetActor(ctx, claims.Username)
	if err != nil {
		return common.NotFound(err), nil
	}

	// Parse request body
	var updateReq models.UpdateCredentialsRequest
	if err := json.Unmarshal([]byte(request.Body), &updateReq); err != nil {
		return common.BadRequest(err), nil
	}

	// Update actor information (only fields that exist)
	if updateReq.DisplayName != "" {
		actor.Name = updateReq.DisplayName
	}
	if updateReq.Note != "" {
		actor.Summary = updateReq.Note
	}
	if updateReq.Avatar != "" && actor.Icon != nil {
		actor.Icon.URL = updateReq.Avatar
	}
	// Update actor flags
	actor.ManuallyApprovesFollowers = updateReq.Locked
	actor.Discoverable = updateReq.Discoverable
	// Bot status would need to be tracked separately

	// Save changes
	if err := h.store.UpdateActor(ctx, actor); err != nil {
		return common.InternalServerError(err), nil
	}

	// Get user for email
	user, err := h.store.GetUser(ctx, claims.Username)
	if err != nil {
		h.logger.Error("failed to get user", zap.Error(err))
	}

	// Return updated credentials in Mastodon format
	resp := models.VerifyCredentialsResponse{
		ID:             actor.ID,
		Username:       actor.PreferredUsername,
		DisplayName:    actor.Name,
		Email:          user.Email,
		EmailVerified:  true,
		Note:           actor.Summary,
		URL:            actor.URL,
		Avatar:         "",
		AvatarStatic:   "",
		Header:         "",
		HeaderStatic:   "",
		FollowersCount: 0,
		FollowingCount: 0,
		StatusesCount:  0,
		CreatedAt:      time.Now().Format("2006-01-02T15:04:05.000Z"),
		Role:           user.Role,
		Source: map[string]interface{}{
			"privacy":   "public",
			"sensitive": false,
			"language":  "en",
			"fields":    []interface{}{},
		},
	}

	if actor.Icon != nil {
		resp.Avatar = actor.Icon.URL
		resp.AvatarStatic = actor.Icon.URL
	}

	body, _ := json.Marshal(resp)
	return &events.APIGatewayV2HTTPResponse{
		StatusCode: http.StatusOK,
		Headers: map[string]string{
			"Content-Type": "application/json",
		},
		Body: string(body),
	}, nil
}

// HandleGetAccount retrieves account information by ID (username)
func (h *Handler) HandleGetAccount(ctx context.Context, request events.APIGatewayV2HTTPRequest, accountID string) (*events.APIGatewayV2HTTPResponse, error) {
	// In our implementation, account ID is the username
	username := accountID

	// Get the actor
	actor, err := h.store.GetActor(ctx, username)
	if err != nil {
		return common.NotFound(fmt.Errorf("account not found")), nil
	}

	// Get follower and following counts
	followerCount := 0
	followingCount := 0
	statusesCount := 0

	// Get followers count (approximate - just get first page)
	followers, _, err := h.store.GetFollowers(ctx, username, 1, "")
	if err == nil && len(followers) > 0 {
		// This is approximate - in production you'd want actual counts
		followerCount = len(followers)
	}

	// Get following count
	following, _, err := h.store.GetFollowing(ctx, username, 1, "")
	if err == nil && len(following) > 0 {
		followingCount = len(following)
	}

	// Get statuses count
	objects, _, err := h.store.GetObjectsByActor(ctx, actor.ID, "", 1)
	if err == nil && len(objects) > 0 {
		statusesCount = len(objects)
	}

	// Convert to Mastodon account format
	account := models.Account{
		ID:             username,
		Username:       username,
		Acct:           username,
		DisplayName:    actor.Name,
		Locked:         false, // TODO: Check actor.ManuallyApprovesFollowers
		Bot:            false, // TODO: Check actor.Type
		Discoverable:   true,  // TODO: Check actor.Discoverable
		Group:          actor.Type == "Group",
		CreatedAt:      time.Now().Format(time.RFC3339), // TODO: Store actor creation time
		Note:           actor.Summary,
		URL:            actor.URL,
		Avatar:         actor.Icon.URL,
		AvatarStatic:   actor.Icon.URL,
		Header:         "", // TODO: Add header support
		HeaderStatic:   "",
		FollowersCount: followerCount,
		FollowingCount: followingCount,
		StatusesCount:  statusesCount,
		LastStatusAt:   "", // TODO: Track last status time
		Emojis:         []interface{}{},
		Fields:         []interface{}{}, // TODO: Parse actor.Attachment
	}

	// Set default avatar if not present
	if account.Avatar == "" {
		account.Avatar = fmt.Sprintf("%s/avatars/default.png", h.cfg.BaseURL())
		account.AvatarStatic = account.Avatar
	}

	body, err := json.Marshal(account)
	if err != nil {
		return common.InternalServerError(err), nil
	}

	return &events.APIGatewayV2HTTPResponse{
		StatusCode: http.StatusOK,
		Headers: map[string]string{
			"Content-Type": "application/json",
		},
		Body: string(body),
	}, nil
}

// validateRegistrationRequest validates a registration request
func (h *Handler) validateRegistrationRequest(req models.AccountRegistrationRequest) error {
	// Validate username
	if req.Username == "" {
		return errors.New("username is required")
	}
	if len(req.Username) < 3 || len(req.Username) > 30 {
		return errors.New("username must be between 3 and 30 characters")
	}
	if !isValidUsername(req.Username) {
		return errors.New("username can only contain letters, numbers, and underscores")
	}

	// Validate email
	if req.Email == "" {
		return errors.New("email is required")
	}
	if _, err := mail.ParseAddress(req.Email); err != nil {
		return errors.New("invalid email address")
	}

	// Validate password
	if req.Password == "" {
		return errors.New("password is required")
	}

	// Validate agreement
	if !req.Agreement {
		return errors.New("you must agree to the terms of service")
	}

	return nil
}

// isValidUsername checks if a username is valid
func isValidUsername(username string) bool {
	// Username regex: alphanumeric and underscores only
	match, _ := regexp.MatchString("^[a-zA-Z0-9_]+$", username)
	return match
}

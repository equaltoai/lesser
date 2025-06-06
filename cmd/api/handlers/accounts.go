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

// HandleAccountLookup looks up an account by username@domain
func (h *Handler) HandleAccountLookup(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	// Get acct parameter
	acct := request.QueryStringParameters["acct"]
	if acct == "" {
		return common.BadRequest(errors.New("acct parameter is required")), nil
	}

	// Remove @ prefix if present
	acct = strings.TrimPrefix(acct, "@")

	// For local accounts, just use the username part
	username := acct
	if parts := strings.Split(acct, "@"); len(parts) > 0 {
		username = parts[0]
	}

	// Get the actor
	actor, err := h.store.GetActor(ctx, username)
	if err != nil {
		return common.NotFound(fmt.Errorf("account not found")), nil
	}

	// Convert to Mastodon account format
	account := h.converter.ActorToAccount(actor)

	return common.OK(account), nil
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

// HandleGetAccountFollowers retrieves the list of accounts following the given account
func (h *Handler) HandleGetAccountFollowers(ctx context.Context, request events.APIGatewayV2HTTPRequest, accountID string) (*events.APIGatewayV2HTTPResponse, error) {
	// Extract username from accountID
	username := accountID

	// Get the actor to verify it exists
	_, err := h.store.GetActor(ctx, username)
	if err != nil {
		return common.NotFound(fmt.Errorf("account not found")), nil
	}

	// Parse pagination parameters
	limit := 40
	maxID := request.QueryStringParameters["max_id"]
	minID := request.QueryStringParameters["min_id"]
	cursor := maxID

	// Use minID as cursor if provided and maxID is not
	if minID != "" && maxID == "" {
		cursor = minID
	}

	// Get followers
	followers, nextCursor, err := h.store.GetFollowers(ctx, username, limit, cursor)
	if err != nil {
		h.logger.Error("failed to get followers", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	// Convert followers to Mastodon account format
	accounts := make([]models.Account, 0, len(followers))
	for _, follower := range followers {
		// Extract username from follower ID
		followerUsername := h.converter.ExtractUsernameFromActorID(follower)
		if followerUsername == "" {
			// Try to parse as a remote actor
			h.logger.Warn("could not extract username from follower", zap.String("follower_id", follower))
			continue
		}

		// Get the follower actor
		followerActor, err := h.store.GetActor(ctx, followerUsername)
		if err != nil {
			h.logger.Warn("could not get follower actor",
				zap.String("username", followerUsername),
				zap.Error(err))
			continue
		}

		// Convert to account
		account := h.converter.ActorToAccount(followerActor)
		accounts = append(accounts, account)
	}

	// Set Link header for pagination if there's a cursor
	headers := map[string]string{
		"Content-Type": "application/json",
	}
	if nextCursor != "" {
		nextURL := fmt.Sprintf("%s/api/v1/accounts/%s/followers?max_id=%s",
			h.cfg.BaseURL(), accountID, nextCursor)
		headers["Link"] = fmt.Sprintf(`<%s>; rel="next"`, nextURL)
	}

	body, _ := json.Marshal(accounts)
	return &events.APIGatewayV2HTTPResponse{
		StatusCode: http.StatusOK,
		Headers:    headers,
		Body:       string(body),
	}, nil
}

// HandleGetAccountFollowing retrieves the list of accounts the given account is following
func (h *Handler) HandleGetAccountFollowing(ctx context.Context, request events.APIGatewayV2HTTPRequest, accountID string) (*events.APIGatewayV2HTTPResponse, error) {
	// Extract username from accountID
	username := accountID

	// Get the actor to verify it exists
	_, err := h.store.GetActor(ctx, username)
	if err != nil {
		return common.NotFound(fmt.Errorf("account not found")), nil
	}

	// Parse pagination parameters
	limit := 40
	maxID := request.QueryStringParameters["max_id"]
	minID := request.QueryStringParameters["min_id"]
	cursor := maxID

	// Use minID as cursor if provided and maxID is not
	if minID != "" && maxID == "" {
		cursor = minID
	}

	// Get following
	following, nextCursor, err := h.store.GetFollowing(ctx, username, limit, cursor)
	if err != nil {
		h.logger.Error("failed to get following", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	// Convert following to Mastodon account format
	accounts := make([]models.Account, 0, len(following))
	for _, followed := range following {
		// Extract username from followed ID
		followedUsername := h.converter.ExtractUsernameFromActorID(followed)
		if followedUsername == "" {
			// Try to parse as a remote actor
			h.logger.Warn("could not extract username from followed", zap.String("followed_id", followed))
			continue
		}

		// Get the followed actor
		followedActor, err := h.store.GetActor(ctx, followedUsername)
		if err != nil {
			h.logger.Warn("could not get followed actor",
				zap.String("username", followedUsername),
				zap.Error(err))
			continue
		}

		// Convert to account
		account := h.converter.ActorToAccount(followedActor)
		accounts = append(accounts, account)
	}

	// Set Link header for pagination if there's a cursor
	headers := map[string]string{
		"Content-Type": "application/json",
	}
	if nextCursor != "" {
		nextURL := fmt.Sprintf("%s/api/v1/accounts/%s/following?max_id=%s",
			h.cfg.BaseURL(), accountID, nextCursor)
		headers["Link"] = fmt.Sprintf(`<%s>; rel="next"`, nextURL)
	}

	body, _ := json.Marshal(accounts)
	return &events.APIGatewayV2HTTPResponse{
		StatusCode: http.StatusOK,
		Headers:    headers,
		Body:       string(body),
	}, nil
}

// HandleGetFamiliarFollowers returns accounts that the requesting user follows and who also follow the given account
func (h *Handler) HandleGetFamiliarFollowers(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
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

	// Get account IDs from query parameter
	accountIDs := request.QueryStringParameters["id[]"]
	if accountIDs == "" {
		return common.BadRequest(errors.New("id[] parameter is required")), nil
	}

	// Split account IDs
	ids := strings.Split(accountIDs, ",")
	if len(ids) == 0 {
		return common.BadRequest(errors.New("at least one account ID is required")), nil
	}

	// Get current user's following list
	following, _, err := h.store.GetFollowing(ctx, claims.Username, 1000, "") // Get a reasonable number
	if err != nil {
		h.logger.Error("failed to get following", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	// Create a map of who the current user follows
	userFollowsMap := make(map[string]bool)
	for _, followedActorID := range following {
		userFollowsMap[followedActorID] = true
	}

	// Build response for each requested account
	type FamiliarFollowersResponse struct {
		ID       string           `json:"id"`
		Accounts []models.Account `json:"accounts"`
	}

	results := make([]FamiliarFollowersResponse, 0, len(ids))

	for _, accountID := range ids {
		// Get followers of this account
		followers, _, err := h.store.GetFollowers(ctx, accountID, 100, "")
		if err != nil {
			// Skip if account not found
			continue
		}

		// Find mutual connections (accounts that follow this user AND are followed by the current user)
		mutualAccounts := make([]models.Account, 0)
		for _, followerActorID := range followers {
			if userFollowsMap[followerActorID] {
				// Get actor details
				username := h.converter.ExtractUsernameFromActorID(followerActorID)
				if username != "" {
					actor, err := h.store.GetActor(ctx, username)
					if err == nil {
						account := h.converter.ActorToAccount(actor)
						mutualAccounts = append(mutualAccounts, account)
					}
				}
			}
		}

		results = append(results, FamiliarFollowersResponse{
			ID:       accountID,
			Accounts: mutualAccounts,
		})
	}

	return common.OK(results), nil
}

// HandlePinAccount pins an account to the user's profile
func (h *Handler) HandlePinAccount(ctx context.Context, request events.APIGatewayV2HTTPRequest, accountID string) (*events.APIGatewayV2HTTPResponse, error) {
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

	// Check write:accounts scope
	if !claims.HasScope(auth.ScopeWrite) {
		return common.Forbidden(errors.New("insufficient scope")), nil
	}

	// Get target actor to verify it exists
	targetActor, err := h.store.GetActor(ctx, accountID)
	if err != nil {
		return common.NotFound(errors.New("account not found")), nil
	}

	// Create pin relationship
	pin := &storage.AccountPin{
		Username:       claims.Username,
		PinnedActorID:  targetActor.ID,
		PinnedUsername: accountID,
		CreatedAt:      time.Now(),
	}

	// Store the pin
	if err := h.store.CreateAccountPin(ctx, pin); err != nil {
		if strings.Contains(err.Error(), "already pinned") {
			return common.UnprocessableEntity(errors.New("account already pinned")), nil
		}
		h.logger.Error("failed to pin account", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	// Get relationship status to return
	relationship, err := h.getRelationshipMap(ctx, claims.Username, accountID)
	if err != nil {
		h.logger.Error("failed to get relationship", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	return common.OK(relationship), nil
}

// HandleUnpinAccount unpins an account from the user's profile
func (h *Handler) HandleUnpinAccount(ctx context.Context, request events.APIGatewayV2HTTPRequest, accountID string) (*events.APIGatewayV2HTTPResponse, error) {
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

	// Check write:accounts scope
	if !claims.HasScope(auth.ScopeWrite) {
		return common.Forbidden(errors.New("insufficient scope")), nil
	}

	// Get target actor to verify it exists
	targetActor, err := h.store.GetActor(ctx, accountID)
	if err != nil {
		return common.NotFound(errors.New("account not found")), nil
	}

	// Delete the pin
	if err := h.store.DeleteAccountPin(ctx, claims.Username, targetActor.ID); err != nil {
		h.logger.Error("failed to unpin account", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	// Get relationship status to return
	relationship, err := h.getRelationshipMap(ctx, claims.Username, accountID)
	if err != nil {
		h.logger.Error("failed to get relationship", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	return common.OK(relationship), nil
}

// HandleSetAccountNote sets a private note on an account
func (h *Handler) HandleSetAccountNote(ctx context.Context, request events.APIGatewayV2HTTPRequest, accountID string) (*events.APIGatewayV2HTTPResponse, error) {
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

	// Check write:accounts scope
	if !claims.HasScope(auth.ScopeWrite) {
		return common.Forbidden(errors.New("insufficient scope")), nil
	}

	// Parse request body
	var req struct {
		Comment string `json:"comment"`
	}
	if err := json.Unmarshal([]byte(request.Body), &req); err != nil {
		return common.BadRequest(err), nil
	}

	// Get target actor to verify it exists
	targetActor, err := h.store.GetActor(ctx, accountID)
	if err != nil {
		return common.NotFound(errors.New("account not found")), nil
	}

	// Create or update account note
	note := &storage.AccountNote{
		Username:       claims.Username,
		TargetActorID:  targetActor.ID,
		TargetUsername: accountID,
		Note:           req.Comment,
		UpdatedAt:      time.Now(),
	}

	// Store the note
	if err := h.store.SetAccountNote(ctx, note); err != nil {
		h.logger.Error("failed to set account note", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	// Get relationship status to return
	relationship, err := h.getRelationshipMap(ctx, claims.Username, accountID)
	if err != nil {
		h.logger.Error("failed to get relationship", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	// Add the note to the relationship
	relationship["note"] = req.Comment

	return common.OK(relationship), nil
}

// HandleRemoveFromFollowers removes a follower from the current user's followers list
func (h *Handler) HandleRemoveFromFollowers(ctx context.Context, request events.APIGatewayV2HTTPRequest, accountID string) (*events.APIGatewayV2HTTPResponse, error) {
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

	// Check write:follows scope
	if !claims.HasScope(auth.ScopeWrite) {
		return common.Forbidden(errors.New("insufficient scope")), nil
	}

	// Get target actor to verify it exists
	_, err = h.store.GetActor(ctx, accountID)
	if err != nil {
		return common.NotFound(errors.New("account not found")), nil
	}

	// Check if target follows current user using IsFollowing
	follows, err := h.store.IsFollowing(ctx, accountID, claims.Username)
	if err != nil {
		h.logger.Error("failed to check follow status", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	if !follows {
		return common.NotFound(errors.New("account is not following you")), nil
	}

	// Remove the follow relationship
	if err := h.store.RemoveFollow(ctx, accountID, claims.Username); err != nil {
		h.logger.Error("failed to remove follower", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	// Get relationship status to return
	relationship, err := h.getRelationshipMap(ctx, claims.Username, accountID)
	if err != nil {
		h.logger.Error("failed to get relationship", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	return common.OK(relationship), nil
}

// Helper function to get relationship status (used by pin/unpin/note endpoints)
func (h *Handler) getRelationshipMap(ctx context.Context, currentUsername, targetUsername string) (map[string]interface{}, error) {
	// Get actors
	currentActor, err := h.store.GetActor(ctx, currentUsername)
	if err != nil {
		return nil, err
	}

	targetActor, err := h.store.GetActor(ctx, targetUsername)
	if err != nil {
		return nil, err
	}

	// Check various relationship statuses using IsFollowing
	following, _ := h.store.IsFollowing(ctx, currentUsername, targetUsername)
	followedBy, _ := h.store.IsFollowing(ctx, targetUsername, currentUsername)

	// Check if pinned
	endorsed, _ := h.store.IsAccountPinned(ctx, currentUsername, targetActor.ID)

	// Get note if exists
	note, _ := h.store.GetAccountNote(ctx, currentUsername, targetActor.ID)
	noteText := ""
	if note != nil {
		noteText = note.Note
	}

	// Check if muted
	muted, _ := h.store.IsMuted(ctx, currentActor.ID, targetActor.ID)

	// Check if blocked
	blocking, _ := h.store.IsBlocked(ctx, currentActor.ID, targetActor.ID)
	blockedBy, _ := h.store.IsBlocked(ctx, targetActor.ID, currentActor.ID)

	// Build relationship response
	relationship := map[string]interface{}{
		"id":                   targetUsername,
		"following":            following,
		"showing_reblogs":      true,  // TODO: Implement reblog filtering
		"notifying":            false, // TODO: Implement notification settings
		"followed_by":          followedBy,
		"blocking":             blocking,
		"blocked_by":           blockedBy,
		"muting":               muted,
		"muting_notifications": false, // TODO: Implement notification muting separately
		"requested":            false, // TODO: Check follow requests
		"domain_blocking":      false, // TODO: Check domain blocks
		"endorsed":             endorsed,
		"note":                 noteText,
	}

	return relationship, nil
}

package handlers

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"mime/multipart"
	"net/http"
	"net/mail"
	"os"
	"strings"
	"time"

	"github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/federation"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"go.uber.org/zap"
)

// HandleRegistration handles user registration requests
func (h *Handler) HandleRegistration(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	// Parse request
	var req models.AccountRegistrationRequest
	if err := common.ParseRequestBody([]byte(request.Body), &req); err != nil {
		return common.BadRequest(err), nil
	}

	// Validate request
	if err := h.validateRegistrationRequest(req); err != nil {
		errResp := map[string]any{
			"error": err.Error(),
		}
		body, _ := json.Marshal(errResp)
		return &events.APIGatewayV2HTTPResponse{
			StatusCode: http.StatusUnprocessableEntity,
			Headers:    common.Headers(),
			Body:       string(body),
		}, nil
	}

	// Handle password if provided (optional for WebAuthn registration)
	var passwordHash string
	if req.Password != "" {
		// Validate password strength
		if err := auth.ValidatePassword(req.Password, req.Username); err != nil {
			errResp := map[string]any{
				"error": err.Error(),
			}
			body, _ := json.Marshal(errResp)
			return &events.APIGatewayV2HTTPResponse{
				StatusCode: http.StatusUnprocessableEntity,
				Headers:    common.Headers(),
				Body:       string(body),
			}, nil
		}

		// Check password strength
		strength := auth.PasswordStrength(req.Password)
		if strength < 3 {
			hints := auth.GeneratePasswordHint(req.Password)
			errResp := map[string]any{
				"error": fmt.Sprintf("Password is too weak (%s). Suggestions: %s",
					auth.PasswordStrengthLabel(strength),
					strings.Join(hints, ", ")),
			}
			body, _ := json.Marshal(errResp)
			return &events.APIGatewayV2HTTPResponse{
				StatusCode: http.StatusUnprocessableEntity,
				Headers:    common.Headers(),
				Body:       string(body),
			}, nil
		}

		// Hash password
		hash, err := auth.HashPassword(req.Password)
		if err != nil {
			errResp := map[string]any{
				"error": err.Error(),
			}
			body, _ := json.Marshal(errResp)
			return &events.APIGatewayV2HTTPResponse{
				StatusCode: http.StatusUnprocessableEntity,
				Headers:    common.Headers(),
				Body:       string(body),
			}, nil
		}
		passwordHash = hash
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
			errResp := map[string]any{
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
	actor.CreatedAt = &user.CreatedAt // Set actual creation time
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

	// Record registration activity for metrics
	if err := h.store.RecordActivity(ctx, "registration", actor.ID, time.Now()); err != nil {
		// Log the error but don't fail the request
		h.logger.Warn("failed to record registration activity", zap.Error(err))
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

	// Get real counts
	followerCount, _ := h.store.GetFollowersCount(ctx, actor.ID)
	followingCount, _ := h.store.GetFollowingCount(ctx, actor.ID)
	statusesCount, _ := h.store.GetStatusCount(ctx, actor.ID)

	resp := models.VerifyCredentialsResponse{
		ID:             actor.ID,
		Username:       user.Username,
		Acct:           user.Username, // For local accounts, same as username
		DisplayName:    actor.Name,
		Locked:         actor.ManuallyApprovesFollowers,
		Bot:            actor.Type == "Service",
		Discoverable:   actor.Discoverable,
		Group:          actor.Type == "Group",
		Email:          user.Email,
		EmailVerified:  true, // No email verification needed - system doesn't use email
		Note:           actor.Summary,
		URL:            actor.URL,
		Avatar:         "",
		AvatarStatic:   "",
		Header:         "",
		HeaderStatic:   "",
		FollowersCount: followerCount,
		FollowingCount: followingCount,
		StatusesCount:  statusesCount,
		LastStatusAt:   h.formatLastStatusTime(actor.LastStatusAt),
		Emojis:         []any{},
		Fields:         []any{},
		CreatedAt:      user.CreatedAt.Format("2006-01-02T15:04:05.000Z"),
		Role:           user.Role,
		Source: map[string]any{
			"privacy":   "public",
			"sensitive": false,
			"language":  user.Locale,
			"fields":    []any{},
		},
	}

	// Populate avatar from actor Icon
	if actor.Icon != nil && actor.Icon.URL != "" {
		resp.Avatar = actor.Icon.URL
		resp.AvatarStatic = actor.Icon.URL
	}

	// Populate header from actor Image
	if actor.Image != nil && actor.Image.URL != "" {
		resp.Header = actor.Image.URL
		resp.HeaderStatic = actor.Image.URL
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

	// Check content type
	contentType := request.Headers["Content-Type"]
	if contentType == "" {
		contentType = request.Headers["content-type"]
	}

	var updateReq models.UpdateCredentialsRequest
	var avatarData []byte
	var avatarContentType string
	var headerData []byte
	var headerContentType string

	// Handle multipart/form-data
	if strings.HasPrefix(contentType, "multipart/form-data") {
		// Check for empty body
		if len(request.Body) == 0 {
			return common.BadRequest(errors.New("empty request body")), nil
		}

		// Log request info for debugging
		h.logger.Info("handling multipart request",
			zap.Bool("is_base64", request.IsBase64Encoded),
			zap.Int("body_length", len(request.Body)),
			zap.String("content_type", contentType))

		// Parse multipart form data
		var bodyBytes []byte

		// Try to decode as base64 first (API Gateway typically encodes binary data)
		decoded, err := base64.StdEncoding.DecodeString(request.Body)
		if err == nil {
			// Successfully decoded - use the decoded bytes
			bodyBytes = decoded
			h.logger.Debug("successfully decoded base64 body", zap.Int("decoded_length", len(bodyBytes)))
		} else {
			// Failed to decode - check if it's raw multipart data
			if strings.Contains(request.Body[:min(200, len(request.Body))], "------WebKitFormBoundary") {
				// Looks like raw multipart data
				bodyBytes = []byte(request.Body)
				h.logger.Debug("using raw body as multipart data", zap.Int("body_length", len(bodyBytes)))
			} else {
				// Neither base64 nor raw multipart - return error
				h.logger.Error("unable to parse request body",
					zap.Error(err),
					zap.String("body_preview", request.Body[:min(50, len(request.Body))]))
				return common.BadRequest(fmt.Errorf("unable to parse request body: not valid base64 or multipart data")), nil
			}
		}

		// Parse boundary from content type
		_, params, err := mime.ParseMediaType(contentType)
		if err != nil {
			return common.BadRequest(fmt.Errorf("invalid content type: %w", err)), nil
		}

		boundary := params["boundary"]
		if boundary == "" {
			return common.BadRequest(errors.New("missing boundary in content type")), nil
		}

		// Create multipart reader
		reader := multipart.NewReader(bytes.NewReader(bodyBytes), boundary)

		// Read form fields and files
		for {
			part, err := reader.NextPart()
			if err != nil {
				break
			}
			defer func() {
				if err := part.Close(); err != nil {
					// Log error but don't fail the request
				}
			}()

			switch part.FormName() {
			case "display_name":
				buf := new(bytes.Buffer)
				if _, err := buf.ReadFrom(part); err != nil {
					continue
				}
				updateReq.DisplayName = buf.String()
			case "note":
				buf := new(bytes.Buffer)
				if _, err := buf.ReadFrom(part); err != nil {
					continue
				}
				updateReq.Note = buf.String()
			case "locked":
				buf := new(bytes.Buffer)
				if _, err := buf.ReadFrom(part); err != nil {
					continue
				}
				updateReq.Locked = buf.String() == "true"
			case "bot":
				buf := new(bytes.Buffer)
				if _, err := buf.ReadFrom(part); err != nil {
					continue
				}
				updateReq.Bot = buf.String() == "true"
			case "discoverable":
				buf := new(bytes.Buffer)
				if _, err := buf.ReadFrom(part); err != nil {
					continue
				}
				updateReq.Discoverable = buf.String() == "true"
			case "avatar":
				if part.FileName() != "" {
					buf := new(bytes.Buffer)
					if _, err := buf.ReadFrom(part); err != nil {
						continue
					}
					avatarData = buf.Bytes()
					avatarContentType = part.Header.Get("Content-Type")
				}
			case "header":
				if part.FileName() != "" {
					buf := new(bytes.Buffer)
					if _, err := buf.ReadFrom(part); err != nil {
						continue
					}
					headerData = buf.Bytes()
					headerContentType = part.Header.Get("Content-Type")
				}
			}
		}
	} else {
		// Parse JSON request body
		if err := common.ParseRequestBody([]byte(request.Body), &updateReq); err != nil {
			return common.BadRequest(err), nil
		}
	}

	// Handle avatar upload if provided
	if len(avatarData) > 0 {
		// Validate file size (10MB limit)
		maxSize := int64(10 * 1024 * 1024)
		if int64(len(avatarData)) > maxSize {
			return common.UnprocessableEntity(fmt.Errorf("avatar file size exceeds %dMB limit", maxSize/1024/1024)), nil
		}

		// Validate MIME type
		if !isAllowedImageMimeType(avatarContentType) {
			return common.UnprocessableEntity(fmt.Errorf("unsupported avatar file type: %s", avatarContentType)), nil
		}

		// Upload avatar to S3
		avatarURL, err := h.uploadProfileImage(ctx, claims.Username, "avatar", avatarData, avatarContentType)
		if err != nil {
			h.logger.Error("failed to upload avatar", zap.Error(err))
			return common.InternalServerError(errors.New("failed to upload avatar")), nil
		}

		// Initialize Icon if nil
		if actor.Icon == nil {
			actor.Icon = &activitypub.Image{}
		}
		actor.Icon.URL = avatarURL
	}

	// Handle header upload if provided
	if len(headerData) > 0 {
		// Validate file size (10MB limit)
		maxSize := int64(10 * 1024 * 1024)
		if int64(len(headerData)) > maxSize {
			return common.UnprocessableEntity(fmt.Errorf("header file size exceeds %dMB limit", maxSize/1024/1024)), nil
		}

		// Validate MIME type
		if !isAllowedImageMimeType(headerContentType) {
			return common.UnprocessableEntity(fmt.Errorf("unsupported header file type: %s", headerContentType)), nil
		}

		// Upload header to S3
		headerURL, err := h.uploadProfileImage(ctx, claims.Username, "header", headerData, headerContentType)
		if err != nil {
			h.logger.Error("failed to upload header", zap.Error(err))
			return common.InternalServerError(errors.New("failed to upload header")), nil
		}

		// Initialize Image if nil
		if actor.Image == nil {
			actor.Image = &activitypub.Image{}
		}
		actor.Image.URL = headerURL
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
	if updateReq.Header != "" && actor.Image != nil {
		actor.Image.URL = updateReq.Header
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

	// Get real counts
	followerCount, _ := h.store.GetFollowersCount(ctx, actor.ID)
	followingCount, _ := h.store.GetFollowingCount(ctx, actor.ID)
	statusesCount, _ := h.store.GetStatusCount(ctx, actor.ID)

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
		FollowersCount: followerCount,
		FollowingCount: followingCount,
		StatusesCount:  statusesCount,
		CreatedAt:      time.Now().Format("2006-01-02T15:04:05.000Z"),
		Role:           user.Role,
		Source: map[string]any{
			"privacy":   "public",
			"sensitive": false,
			"language":  "en",
			"fields":    []any{},
		},
	}

	if actor.Icon != nil {
		resp.Avatar = actor.Icon.URL
		resp.AvatarStatic = actor.Icon.URL
	}

	if actor.Image != nil {
		resp.Header = actor.Image.URL
		resp.HeaderStatic = actor.Image.URL
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

// HandleGetAccount retrieves account information by ID (username or URL)
func (h *Handler) HandleGetAccount(ctx context.Context, request events.APIGatewayV2HTTPRequest, accountID string) (*events.APIGatewayV2HTTPResponse, error) {
	// Resolve account ID to actor
	actor, err := h.resolveAccountID(ctx, accountID)
	if err != nil {
		return common.NotFound(fmt.Errorf("account not found")), nil
	}

	// Get real counts
	followerCount, _ := h.store.GetFollowersCount(ctx, actor.ID)
	followingCount, _ := h.store.GetFollowingCount(ctx, actor.ID)
	statusesCount, _ := h.store.GetStatusCount(ctx, actor.ID)

	// Convert to Mastodon account format
	account := models.Account{
		ID:             actor.PreferredUsername,
		Username:       actor.PreferredUsername,
		Acct:           actor.PreferredUsername,
		DisplayName:    actor.Name,
		Locked:         actor.ManuallyApprovesFollowers,
		Bot:            actor.Type == "Service",
		Discoverable:   actor.Discoverable,
		Group:          actor.Type == "Group",
		CreatedAt:      h.formatActorCreatedTime(actor.CreatedAt),
		Note:           actor.Summary,
		URL:            actor.URL,
		Avatar:         "", // Default empty avatar
		AvatarStatic:   "", // Default empty avatar
		Header:         h.getHeaderURL(actor),
		HeaderStatic:   "",
		FollowersCount: followerCount,
		FollowingCount: followingCount,
		StatusesCount:  statusesCount,
		LastStatusAt:   h.formatLastStatusTime(actor.LastStatusAt),
		Emojis:         []any{},
		Fields:         h.parseActorFields(ctx, actor, actor.Attachment),
	}

	// Set avatar if icon is present
	if actor.Icon != nil && actor.Icon.URL != "" {
		account.Avatar = actor.Icon.URL
		account.AvatarStatic = actor.Icon.URL
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

	// Get real counts
	followerCount, _ := h.store.GetFollowersCount(ctx, actor.ID)
	followingCount, _ := h.store.GetFollowingCount(ctx, actor.ID)
	statusesCount, _ := h.store.GetStatusCount(ctx, actor.ID)

	// Convert to Mastodon account format with real counts
	account := h.converter.ActorToAccountWithCounts(actor, followerCount, followingCount, statusesCount)

	return common.OK(account), nil
}

// validateRegistrationRequest validates a registration request
func (h *Handler) validateRegistrationRequest(req models.AccountRegistrationRequest) error {
	// Validate username using comprehensive validation
	if err := activitypub.ValidateUsername(req.Username); err != nil {
		return err
	}

	// Validate email (optional for email-free auth)
	if req.Email != "" {
		if _, err := mail.ParseAddress(req.Email); err != nil {
			return errors.New("invalid email address")
		}
	}

	// Validate password (optional for WebAuthn/passkey auth)
	// Password is only required if not using alternative auth methods

	// Validate agreement
	if !req.Agreement {
		return errors.New("you must agree to the terms of service")
	}

	return nil
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
	if err := common.ParseRequestBody([]byte(request.Body), &req); err != nil {
		return common.BadRequest(err), nil
	}

	// Get target actor to verify it exists
	targetActor, err := h.store.GetActor(ctx, accountID)
	if err != nil {
		return common.NotFound(errors.New("account not found")), nil
	}

	// Create or update account note
	note := &storage.AccountNote{
		Username:      claims.Username,
		TargetActorID: targetActor.ID,
		Note:          req.Comment,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}

	// Store the note
	if err := h.store.CreateAccountNote(ctx, note); err != nil {
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
func (h *Handler) getRelationshipMap(ctx context.Context, currentUsername, targetUsername string) (map[string]any, error) {
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
	relationship := map[string]any{
		"id":                   targetUsername,
		"following":            following,
		"showing_reblogs":      !h.isReblogFiltered(ctx, currentActor.ID, targetActor.ID),
		"notifying":            h.isNotifyingEnabled(ctx, currentActor.ID, targetActor.ID),
		"followed_by":          followedBy,
		"blocking":             blocking,
		"blocked_by":           blockedBy,
		"muting":               muted,
		"muting_notifications": h.isNotificationsMuted(ctx, currentActor.ID, targetActor.ID),
		"requested":            h.hasFollowRequest(ctx, currentActor.ID, targetActor.ID),
		"domain_blocking":      h.isDomainBlocked(ctx, currentActor.ID, targetActor.ID),
		"endorsed":             endorsed,
		"note":                 noteText,
	}

	return relationship, nil
}

// uploadProfileImage uploads a profile image (avatar or header) to S3
func (h *Handler) uploadProfileImage(ctx context.Context, username, imageType string, data []byte, contentType string) (string, error) {
	// Generate unique ID based on timestamp
	imageID := fmt.Sprintf("%d", time.Now().UnixNano())

	// Get file extension
	ext := getExtensionFromImageMimeType(contentType)

	// Generate S3 key
	s3Key := fmt.Sprintf("media/%s/%s/%s%s", username, imageType, imageID, ext)

	// Initialize S3 client
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to load AWS config: %w", err)
	}
	s3Client := s3.NewFromConfig(awsCfg)

	// Upload to S3
	bucketName := h.cfg.S3BucketName
	if bucketName == "" {
		return "", errors.New("S3 bucket not configured")
	}

	putInput := &s3.PutObjectInput{
		Bucket:       aws.String(bucketName),
		Key:          aws.String(s3Key),
		Body:         bytes.NewReader(data),
		ContentType:  aws.String(contentType),
		ACL:          types.ObjectCannedACLPrivate, // Use CloudFront for access
		CacheControl: aws.String("public, max-age=31536000, immutable"),
	}

	_, err = s3Client.PutObject(ctx, putInput)
	if err != nil {
		return "", fmt.Errorf("failed to upload to S3: %w", err)
	}

	// Build URL (using CDN if configured)
	cdnDomain := os.Getenv("CDN_DOMAIN")
	var imageURL string
	if cdnDomain != "" {
		imageURL = fmt.Sprintf("https://%s/%s", cdnDomain, s3Key)
	} else {
		// Fallback to S3 URL if no CDN
		imageURL = fmt.Sprintf("https://%s.s3.amazonaws.com/%s", bucketName, s3Key)
	}

	return imageURL, nil
}

// isAllowedImageMimeType checks if the mime type is allowed for images
func isAllowedImageMimeType(mimeType string) bool {
	allowed := []string{
		"image/jpeg",
		"image/png",
		"image/gif",
		"image/webp",
	}

	for _, t := range allowed {
		if t == mimeType {
			return true
		}
	}
	return false
}

// getExtensionFromImageMimeType returns the file extension for an image mime type
func getExtensionFromImageMimeType(mimeType string) string {
	extensions := map[string]string{
		"image/jpeg": ".jpg",
		"image/png":  ".png",
		"image/gif":  ".gif",
		"image/webp": ".webp",
	}

	if ext, ok := extensions[mimeType]; ok {
		return ext
	}
	return ".jpg" // Default to jpg
}

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// formatLastStatusTime formats the last status time for API response
func (h *Handler) formatLastStatusTime(lastStatusAt *time.Time) string {
	if lastStatusAt == nil {
		return ""
	}
	return lastStatusAt.Format("2006-01-02T15:04:05.000Z")
}

// formatActorCreatedTime formats the actor creation time for API response
func (h *Handler) formatActorCreatedTime(createdAt *time.Time) string {
	if createdAt == nil {
		return time.Now().Format(time.RFC3339)
	}
	return createdAt.Format(time.RFC3339)
}

// getHeaderURL returns the header image URL for an actor
func (h *Handler) getHeaderURL(actor *activitypub.Actor) string {
	if actor.Image != nil && actor.Image.URL != "" {
		return actor.Image.URL
	}
	return ""
}

// parseActorFields parses actor attachment fields into Mastodon format
func (h *Handler) parseActorFields(ctx context.Context, actor *activitypub.Actor, attachments []activitypub.Attachment) []any {
	var fields []any
	for _, attachment := range attachments {
		if attachment.Type == "PropertyValue" {
			field := map[string]any{
				"name":        attachment.Name,
				"value":       attachment.Value,
				"verified_at": h.getFieldVerificationTime(ctx, actor.PreferredUsername, attachment.Name),
			}
			fields = append(fields, field)
		}
	}
	return fields
}

// isReblogFiltered checks if reblogs are filtered for a relationship
func (h *Handler) isReblogFiltered(ctx context.Context, followerID, followeeID string) bool {
	// Check user preference for showing reblogs from this user
	// Use extended preferences to check if reblogs are filtered
	showReblogs, err := h.store.GetPreference(ctx, followerID, fmt.Sprintf("show_reblogs:%s", followeeID))
	if err != nil {
		h.logger.Debug("failed to get reblog preference", zap.Error(err))
		return false // Default to showing reblogs if error
	}

	// If preference exists and is false, reblogs are filtered
	if showReblogs != nil {
		if show, ok := showReblogs.(bool); ok {
			return !show // Filtered if show is false
		}
	}

	return false // Default to not filtered
}

// isNotifyingEnabled checks if notifications are enabled for a relationship
func (h *Handler) isNotifyingEnabled(ctx context.Context, followerID, followeeID string) bool {
	// Check user preference for notifications from this user
	notifyEnabled, err := h.store.GetPreference(ctx, followerID, fmt.Sprintf("notify:%s", followeeID))
	if err != nil {
		h.logger.Debug("failed to check notification preference", zap.Error(err))
		return false // Default to not notifying if error
	}

	// If preference exists and is true, notifications are enabled
	if notifyEnabled != nil {
		if enabled, ok := notifyEnabled.(bool); ok {
			return enabled
		}
	}

	return false // Default to not notifying
}

// isNotificationsMuted checks if notifications are muted for a relationship
func (h *Handler) isNotificationsMuted(ctx context.Context, muterID, muteeID string) bool {
	// Check user preference for muting notifications from this user
	notifMuted, err := h.store.GetPreference(ctx, muterID, fmt.Sprintf("mute_notifications:%s", muteeID))
	if err != nil {
		h.logger.Debug("failed to check notification mute preference", zap.Error(err))
		return false // Default to not muted if error
	}

	// If preference exists and is true, notifications are muted
	if notifMuted != nil {
		if muted, ok := notifMuted.(bool); ok {
			return muted
		}
	}

	return false // Default to not muted
}

// hasFollowRequest checks if there's a pending follow request
func (h *Handler) hasFollowRequest(ctx context.Context, requesterID, targetID string) bool {
	// Check follow request state - returns "pending", "accepted", "rejected", or ""
	state, err := h.store.GetFollowRequestState(ctx, requesterID, targetID)
	if err != nil {
		h.logger.Debug("failed to check follow request state", zap.Error(err))
		return false // Default to no pending request if error
	}

	return state == "pending"
}

// isDomainBlocked checks if a domain is blocked
func (h *Handler) isDomainBlocked(ctx context.Context, userID, targetID string) bool {
	// Extract domain from targetID if it's a full actor URL
	targetDomain := h.extractDomainFromActorID(targetID)
	if targetDomain == "" {
		return false // Local user, no domain blocking
	}

	// Check if user has blocked this domain
	blocked, err := h.store.IsBlockedDomain(ctx, userID, targetDomain)
	if err != nil {
		h.logger.Debug("failed to check domain block status", zap.Error(err))
		return false // Default to not blocked if error
	}

	return blocked
}

// extractDomainFromActorID extracts domain from an actor ID URL
func (h *Handler) extractDomainFromActorID(actorID string) string {
	// If it's a local username, return empty
	if !strings.Contains(actorID, "://") {
		return ""
	}

	// Parse as URL to extract domain
	if strings.HasPrefix(actorID, "https://") || strings.HasPrefix(actorID, "http://") {
		parts := strings.Split(actorID, "/")
		if len(parts) >= 3 {
			return parts[2] // domain part
		}
	}

	return ""
}

// getFieldVerificationTime gets the verification time for a profile field
func (h *Handler) getFieldVerificationTime(ctx context.Context, username, fieldName string) any {
	field, err := h.store.GetFieldVerification(ctx, username, fieldName)
	if err != nil {
		h.logger.Warn("failed to get field verification",
			zap.String("username", username),
			zap.String("field_name", fieldName),
			zap.Error(err))
		return nil
	}

	if field.VerifiedAt != nil {
		return field.VerifiedAt.Format("2006-01-02T15:04:05.000Z")
	}

	return nil
}

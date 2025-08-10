package lift

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"mime"
	"mime/multipart"
	"net/mail"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/federation"
	"github.com/equaltoai/lesser/pkg/mastodon"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"
)

// HandleRegistrationLift handles user registration requests
func (h *Handler) HandleRegistrationLift(ctx *lift.Context) error {
	// Parse request body
	var req models.AccountRegistrationRequest
	if err := ctx.ParseRequest(&req); err != nil {
		return ctx.Status(400).JSON(map[string]string{"error": err.Error()})
	}

	// Validate request
	if err := h.validateRegistrationRequestLift(req); err != nil {
		return ctx.Status(422).JSON(map[string]string{"error": err.Error()})
	}

	// Handle password if provided (optional for WebAuthn registration)
	var passwordHash string
	if req.Password != "" {
		// Validate password strength
		if err := auth.ValidatePassword(req.Password, req.Username); err != nil {
			return ctx.Status(422).JSON(map[string]string{"error": err.Error()})
		}

		// Check password strength
		strength := auth.PasswordStrength(req.Password)
		if strength < 3 {
			hints := auth.GeneratePasswordHint(req.Password)
			return ctx.Status(422).JSON(map[string]string{
				"error": fmt.Sprintf("Password is too weak (%s). Suggestions: %s",
					auth.PasswordStrengthLabel(strength),
					strings.Join(hints, ", ")),
			})
		}

		// Hash password
		hash, err := auth.HashPassword(req.Password)
		if err != nil {
			return ctx.Status(422).JSON(map[string]string{"error": err.Error()})
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

	// Don't create account yet - wait until we have the actor

	// Generate RSA keypair for the actor
	privateKey, err := federation.GenerateRSAKeyPair(2048)
	if err != nil {
		h.logger.Error("failed to generate RSA keypair", zap.Error(err))
		// Clean up user since we couldn't create the actor
		_ = h.repos.Account().DeleteAccount(ctx.Context, user.Username)
		return ctx.Status(500).JSON(map[string]string{"error": "Internal server error"})
	}

	// Encode public key to PEM format
	publicKeyPEM, err := federation.EncodePublicKeyPEM(&privateKey.PublicKey)
	if err != nil {
		h.logger.Error("failed to encode public key", zap.Error(err))
		// Clean up user since we couldn't create the actor
		_ = h.repos.Account().DeleteAccount(ctx.Context, user.Username)
		return ctx.Status(500).JSON(map[string]string{"error": "Internal server error"})
	}

	// Encode private key to PEM format
	privateKeyPEM, err := federation.EncodePrivateKeyPEM(privateKey)
	if err != nil {
		h.logger.Error("failed to encode private key", zap.Error(err))
		// Clean up user since we couldn't create the actor
		_ = h.repos.Account().DeleteAccount(ctx.Context, user.Username)
		return ctx.Status(500).JSON(map[string]string{"error": "Internal server error"})
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

	// Create account with actor together
	if err := h.repos.Account().CreateAccount(ctx.Context, user.Username, user.Email, user.PasswordHash, user.Approved, actor, string(privateKeyPEM)); err != nil {
		if strings.Contains(err.Error(), "already exists") {
			return ctx.Status(422).JSON(map[string]string{"error": "Username is already taken"})
		}
		h.logger.Error("failed to create account with actor", zap.Error(err))
		return ctx.Status(500).JSON(map[string]string{"error": "Internal server error"})
	}

	// Record registration activity for metrics
	if err := h.repos.Activity().RecordActivity(ctx.Context, "registration", actor.ID, time.Now()); err != nil {
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

	return ctx.Status(201).JSON(resp)
}

// HandleVerifyCredentialsLift returns the current user's information
func (h *Handler) HandleVerifyCredentialsLift(ctx *lift.Context) error {
	// Extract token from Authorization header
	authHeader := ctx.Header("Authorization")
	token, err := auth.ExtractBearerToken(authHeader)
	if err != nil {
		return ctx.Status(401).JSON(map[string]string{"error": "Unauthorized"})
	}

	// Validate token and get claims
	oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.repos)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return ctx.Status(401).JSON(map[string]string{"error": "Unauthorized"})
	}

	// Get user
	user, err := h.repos.Account().GetUser(ctx.Context, claims.Username)
	if err != nil {
		h.logger.Error("failed to get user", zap.Error(err))
		return ctx.Status(500).JSON(map[string]string{"error": "Internal server error"})
	}

	// Get actor
	actor, err := h.repos.Account().GetActor(ctx.Context, user.Username)
	if err != nil {
		h.logger.Error("failed to get actor", zap.Error(err))
		return ctx.Status(500).JSON(map[string]string{"error": "Internal server error"})
	}

	// Get real counts - these are not stored on the ActivityPub actor
	// For now, we'll default to 0 since we just fetched the actor
	followerCount := 0
	followingCount := 0
	statusesCount := 0

	resp := models.VerifyCredentialsResponse{
		ID:             actor.ID,
		Username:       user.Username,
		Acct:           user.Username, // For local accounts, same as username
		DisplayName:    actor.Name,
		Locked:         actor.ManuallyApprovesFollowers,
		Bot:            actor.Type == actorTypeService,
		Discoverable:   actor.Discoverable,
		Group:          actor.Type == actorTypeGroup,
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
		LastStatusAt:   h.formatLastStatusTimeLift(actor.LastStatusAt),
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

	return ctx.JSON(resp)
}

// HandleUpdateCredentialsLift updates the current user's profile
func (h *Handler) HandleUpdateCredentialsLift(ctx *lift.Context) error {
	// Authenticate and get actor
	claims, actor, err := h.authenticateAndGetActor(ctx)
	if err != nil {
		return err
	}

	// Parse request data
	updateReq, fileData, err := h.parseUpdateCredentialsRequest(ctx)
	if err != nil {
		return err
	}

	// Handle file uploads
	if err := h.handleFileUploads(ctx, actor, claims, fileData); err != nil {
		return err
	}

	// Update actor with new data
	h.updateActorFromRequest(actor, updateReq)

	// Save changes
	if err := h.repos.Actor().UpdateActor(ctx.Context, actor); err != nil {
		return ctx.Status(500).JSON(map[string]string{"error": "Internal server error"})
	}

	// Build and return response
	resp := h.buildCredentialsResponse(ctx, actor, claims)

	return ctx.JSON(resp)
}

// UpdateCredentialsFileData holds file upload data from multipart form
type UpdateCredentialsFileData struct {
	AvatarData        []byte
	AvatarContentType string
	HeaderData        []byte
	HeaderContentType string
}

// authenticateAndGetActor handles authentication and retrieves the actor
func (h *Handler) authenticateAndGetActor(ctx *lift.Context) (*auth.Claims, *activitypub.Actor, error) {
	// Extract token from Authorization header
	authHeader := ctx.Header("Authorization")
	token, err := auth.ExtractBearerToken(authHeader)
	if err != nil {
		return nil, nil, ctx.Status(401).JSON(map[string]string{"error": "Unauthorized"})
	}

	// Validate token and get claims
	oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.repos)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return nil, nil, ctx.Status(401).JSON(map[string]string{"error": "Unauthorized"})
	}

	// Check write scope
	if err := h.authMiddleware.RequireScope(claims, auth.ScopeWrite); err != nil {
		return nil, nil, ctx.Status(403).JSON(map[string]string{"error": "Forbidden"})
	}

	// Get the actor
	actor, err := h.repos.Account().GetActor(ctx.Context, claims.Username)
	if err != nil {
		return nil, nil, ctx.Status(404).JSON(map[string]string{"error": "Not found"})
	}

	return claims, actor, nil
}

// parseUpdateCredentialsRequest parses both JSON and multipart form requests
func (h *Handler) parseUpdateCredentialsRequest(ctx *lift.Context) (models.UpdateCredentialsRequest, UpdateCredentialsFileData, error) {
	var updateReq models.UpdateCredentialsRequest
	var fileData UpdateCredentialsFileData

	contentType := ctx.Header("Content-Type")

	if strings.HasPrefix(contentType, "multipart/form-data") {
		req, files, err := h.parseMultipartRequest(ctx, contentType)
		if err != nil {
			return updateReq, fileData, err
		}
		updateReq = req
		fileData = files
	} else {
		// Parse JSON request body
		if err := ctx.ParseRequest(&updateReq); err != nil {
			return updateReq, fileData, ctx.Status(400).JSON(map[string]string{"error": err.Error()})
		}
	}

	return updateReq, fileData, nil
}

// parseMultipartRequest handles multipart form parsing with base64 decoding
func (h *Handler) parseMultipartRequest(ctx *lift.Context, contentType string) (models.UpdateCredentialsRequest, UpdateCredentialsFileData, error) {
	var updateReq models.UpdateCredentialsRequest
	var fileData UpdateCredentialsFileData

	// Get raw body from request
	bodyBytes := ctx.Request.Body
	if len(bodyBytes) == 0 {
		return updateReq, fileData, ctx.Status(400).JSON(map[string]string{"error": "Empty request body"})
	}

	// Handle base64 decoding for binary data
	bodyBytes, err := h.handleBase64Decoding(bodyBytes)
	if err != nil {
		return updateReq, fileData, ctx.Status(400).JSON(map[string]string{"error": "Unable to parse request body"})
	}

	// Parse multipart form
	boundary, err := h.extractBoundary(contentType)
	if err != nil {
		return updateReq, fileData, ctx.Status(400).JSON(map[string]string{"error": err.Error()})
	}

	reader := multipart.NewReader(bytes.NewReader(bodyBytes), boundary)

	// Process each part
	for {
		part, err := reader.NextPart()
		if err != nil {
			break
		}

		if err := h.processMultipartPart(part, &updateReq, &fileData); err != nil {
			h.logger.Warn("failed to process multipart part", zap.Error(err))
		}

		if err := part.Close(); err != nil {
			h.logger.Warn("failed to close multipart reader", zap.Error(err))
		}
	}

	return updateReq, fileData, nil
}

// handleBase64Decoding handles base64 decoding for API Gateway
func (h *Handler) handleBase64Decoding(bodyBytes []byte) ([]byte, error) {
	h.logger.Info("handling multipart request", zap.Int("body_length", len(bodyBytes)))

	// Try to decode as base64 first (API Gateway typically encodes binary data)
	if decoded, err := base64.StdEncoding.DecodeString(string(bodyBytes)); err == nil {
		h.logger.Debug("successfully decoded base64 body", zap.Int("decoded_length", len(decoded)))
		return decoded, nil
	}

	// Check if it's raw multipart data
	preview := string(bodyBytes[:mathMin(200, len(bodyBytes))])
	if strings.Contains(preview, "------WebKitFormBoundary") {
		h.logger.Debug("using raw body as multipart data", zap.Int("body_length", len(bodyBytes)))
		return bodyBytes, nil
	}

	// Neither base64 nor raw multipart
	h.logger.Error("unable to parse request body", zap.String("body_preview", preview[:mathMin(50, len(preview))]))
	return nil, fmt.Errorf("unable to parse request body")
}

// extractBoundary extracts the boundary from content type header
func (h *Handler) extractBoundary(contentType string) (string, error) {
	_, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		return "", fmt.Errorf("invalid content type: %w", err)
	}

	boundary := params["boundary"]
	if boundary == "" {
		return "", fmt.Errorf("missing boundary in content type")
	}

	return boundary, nil
}

// processMultipartPart processes a single multipart form part
func (h *Handler) processMultipartPart(part *multipart.Part, updateReq *models.UpdateCredentialsRequest, fileData *UpdateCredentialsFileData) error {
	buf := new(bytes.Buffer)
	if _, err := buf.ReadFrom(part); err != nil {
		return err
	}

	switch part.FormName() {
	case "display_name":
		updateReq.DisplayName = buf.String()
	case "note":
		updateReq.Note = buf.String()
	case "locked":
		updateReq.Locked = buf.String() == boolTrue
	case "bot":
		updateReq.Bot = buf.String() == boolTrue
	case "discoverable":
		updateReq.Discoverable = buf.String() == boolTrue
	case "avatar":
		if part.FileName() != "" {
			fileData.AvatarData = buf.Bytes()
			fileData.AvatarContentType = part.Header.Get("Content-Type")
		}
	case "header":
		if part.FileName() != "" {
			fileData.HeaderData = buf.Bytes()
			fileData.HeaderContentType = part.Header.Get("Content-Type")
		}
	}

	return nil
}

// handleFileUploads handles avatar and header file uploads
func (h *Handler) handleFileUploads(ctx *lift.Context, actor *activitypub.Actor, claims *auth.Claims, fileData UpdateCredentialsFileData) error {
	// Handle avatar upload
	if len(fileData.AvatarData) > 0 {
		if err := h.uploadAndSetAvatar(ctx, actor, claims, fileData.AvatarData, fileData.AvatarContentType); err != nil {
			return err
		}
	}

	// Handle header upload
	if len(fileData.HeaderData) > 0 {
		if err := h.uploadAndSetHeader(ctx, actor, claims, fileData.HeaderData, fileData.HeaderContentType); err != nil {
			return err
		}
	}

	return nil
}

// uploadAndSetAvatar validates and uploads avatar image
func (h *Handler) uploadAndSetAvatar(ctx *lift.Context, actor *activitypub.Actor, claims *auth.Claims, data []byte, contentType string) error {
	if err := h.validateImageUpload(data, contentType, "avatar"); err != nil {
		return ctx.Status(422).JSON(map[string]string{"error": err.Error()})
	}

	avatarURL, err := h.uploadProfileImageLift(ctx.Context, claims.Username, "avatar", data, contentType)
	if err != nil {
		h.logger.Error("failed to upload avatar", zap.Error(err))
		return ctx.Status(500).JSON(map[string]string{"error": "Failed to upload avatar"})
	}

	if actor.Icon == nil {
		actor.Icon = &activitypub.Image{}
	}
	actor.Icon.URL = avatarURL

	return nil
}

// uploadAndSetHeader validates and uploads header image
func (h *Handler) uploadAndSetHeader(ctx *lift.Context, actor *activitypub.Actor, claims *auth.Claims, data []byte, contentType string) error {
	if err := h.validateImageUpload(data, contentType, "header"); err != nil {
		return ctx.Status(422).JSON(map[string]string{"error": err.Error()})
	}

	headerURL, err := h.uploadProfileImageLift(ctx.Context, claims.Username, "header", data, contentType)
	if err != nil {
		h.logger.Error("failed to upload header", zap.Error(err))
		return ctx.Status(500).JSON(map[string]string{"error": "Failed to upload header"})
	}

	if actor.Image == nil {
		actor.Image = &activitypub.Image{}
	}
	actor.Image.URL = headerURL

	return nil
}

// validateImageUpload validates image file size and type
func (h *Handler) validateImageUpload(data []byte, contentType, imageType string) error {
	// Validate file size (10MB limit)
	maxSize := int64(10 * 1024 * 1024)
	if int64(len(data)) > maxSize {
		return fmt.Errorf("%s file size exceeds 10MB limit", imageType)
	}

	// Validate MIME type
	if !h.isAllowedImageMimeTypeLift(contentType) {
		return fmt.Errorf("unsupported %s file type: %s", imageType, contentType)
	}

	return nil
}

// updateActorFromRequest updates actor fields from the request
func (h *Handler) updateActorFromRequest(actor *activitypub.Actor, req models.UpdateCredentialsRequest) {
	// Update text fields
	if req.DisplayName != "" {
		actor.Name = req.DisplayName
	}
	if req.Note != "" {
		actor.Summary = req.Note
	}

	// Update URL fields if they exist and actor has the corresponding image
	if req.Avatar != "" && actor.Icon != nil {
		actor.Icon.URL = req.Avatar
	}
	if req.Header != "" && actor.Image != nil {
		actor.Image.URL = req.Header
	}

	// Update boolean flags
	actor.ManuallyApprovesFollowers = req.Locked
	actor.Discoverable = req.Discoverable
	// Note: Bot status would need to be tracked separately in a different field
}

// buildCredentialsResponse builds the credentials response
func (h *Handler) buildCredentialsResponse(ctx *lift.Context, actor *activitypub.Actor, claims *auth.Claims) models.VerifyCredentialsResponse {
	// Get user for email
	user, err := h.repos.Account().GetUser(ctx.Context, claims.Username)
	if err != nil {
		h.logger.Error("failed to get user", zap.Error(err))
		user = &storage.User{Email: "", Role: "user"} // Default values
	}

	// Get real counts - these are not stored on the ActivityPub actor
	// For now, we'll default to 0 since we just fetched the actor
	followerCount := 0
	followingCount := 0
	statusesCount := 0

	// Build response
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

	// Set image URLs
	if actor.Icon != nil {
		resp.Avatar = actor.Icon.URL
		resp.AvatarStatic = actor.Icon.URL
	}
	if actor.Image != nil {
		resp.Header = actor.Image.URL
		resp.HeaderStatic = actor.Image.URL
	}

	return resp
}

// HandleGetAccountLift retrieves account information by ID (username or URL)
func (h *Handler) HandleGetAccountLift(ctx *lift.Context) error {
	accountID := ctx.Param("id")
	if accountID == "" {
		return ctx.Status(400).JSON(map[string]string{"error": "Account ID is required"})
	}

	// Resolve account ID to actor
	actor, err := h.resolveAccountIDLift(ctx.Context, accountID)
	if err != nil {
		return ctx.Status(404).JSON(map[string]string{"error": "Account not found"})
	}

	// Get real counts - these are not stored on the ActivityPub actor
	// For now, we'll default to 0 since we just fetched the actor
	followerCount := 0
	followingCount := 0
	statusesCount := 0

	// Convert to Mastodon account format
	account := models.Account{
		ID:             actor.PreferredUsername,
		Username:       actor.PreferredUsername,
		Acct:           actor.PreferredUsername,
		DisplayName:    actor.Name,
		Locked:         actor.ManuallyApprovesFollowers,
		Bot:            actor.Type == actorTypeService,
		Discoverable:   actor.Discoverable,
		Group:          actor.Type == actorTypeGroup,
		CreatedAt:      h.formatActorCreatedTimeLift(actor.CreatedAt),
		Note:           actor.Summary,
		URL:            actor.URL,
		Avatar:         "", // Default empty avatar
		AvatarStatic:   "", // Default empty avatar
		Header:         h.getHeaderURLLift(actor),
		HeaderStatic:   "",
		FollowersCount: followerCount,
		FollowingCount: followingCount,
		StatusesCount:  statusesCount,
		LastStatusAt:   h.formatLastStatusTimeLift(actor.LastStatusAt),
		Emojis:         []any{},
		Fields:         h.parseActorFieldsLift(ctx.Context, actor, actor.Attachment),
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

	return ctx.JSON(account)
}

// HandleAccountLookupLift looks up an account by username@domain
func (h *Handler) HandleAccountLookupLift(ctx *lift.Context) error {
	// Get acct parameter
	acct := ctx.Query("acct")
	if acct == "" {
		return ctx.Status(400).JSON(map[string]string{"error": "acct parameter is required"})
	}

	// Remove @ prefix if present
	acct = strings.TrimPrefix(acct, "@")

	// For local accounts, just use the username part
	username := acct
	if parts := strings.Split(acct, "@"); len(parts) > 0 {
		username = parts[0]
	}

	// Get the actor
	actor, err := h.repos.Actor().GetActor(ctx.Context, username)
	if err != nil {
		return ctx.Status(404).JSON(map[string]string{"error": "Account not found"})
	}

	// Get real counts - these are not stored on the ActivityPub actor
	// For now, we'll default to 0 since we just fetched the actor
	followerCount := 0
	followingCount := 0
	statusesCount := 0

	// Convert to Mastodon account format with real counts
	converter := mastodon.NewConverter(h.cfg.BaseURL())
	account := converter.ActorToAccountWithCounts(actor, followerCount, followingCount, statusesCount)

	return ctx.JSON(account)
}

// relationshipType represents the type of relationship being queried
type relationshipType string

const (
	relationshipFollowers relationshipType = "followers"
	relationshipFollowing relationshipType = "following"
)

// handleAccountRelationshipsList is a generic handler for followers and following lists
func (h *Handler) handleAccountRelationshipsList(ctx *lift.Context, relType relationshipType) error {
	accountID := ctx.Param("id")
	if accountID == "" {
		return ctx.Status(400).JSON(map[string]string{"error": "Account ID is required"})
	}

	// Extract username from accountID
	username := accountID

	// Get the actor to verify it exists
	_, err := h.repos.Actor().GetActor(ctx.Context, username)
	if err != nil {
		return ctx.Status(404).JSON(map[string]string{"error": "Account not found"})
	}

	// Parse pagination parameters
	limit := 40
	maxID := ctx.Query("max_id")
	minID := ctx.Query("min_id")
	cursor := maxID

	// Use minID as cursor if provided and maxID is not
	if minID != "" && maxID == "" {
		cursor = minID
	}

	// Get relationships based on type
	var relationships []string
	var nextCursor string

	switch relType {
	case relationshipFollowers:
		relationships, nextCursor, err = h.repos.Relationship().GetFollowers(ctx.Context, username, limit, cursor)
		if err != nil {
			h.logger.Error("failed to get followers", zap.Error(err))
			return ctx.Status(500).JSON(map[string]string{"error": "Internal server error"})
		}
	case relationshipFollowing:
		relationships, nextCursor, err = h.repos.Relationship().GetFollowing(ctx.Context, username, limit, cursor)
		if err != nil {
			h.logger.Error("failed to get following", zap.Error(err))
			return ctx.Status(500).JSON(map[string]string{"error": "Internal server error"})
		}
	}

	// Convert relationships to Mastodon account format
	converter := mastodon.NewConverter(h.cfg.BaseURL())
	accounts := make([]models.Account, 0, len(relationships))
	for _, relationshipID := range relationships {
		// Extract username from relationship ID
		relatedUsername := converter.ExtractUsernameFromActorID(relationshipID)
		if relatedUsername == "" {
			h.logger.Warn("could not extract username from relationship",
				zap.String("relationship_id", relationshipID),
				zap.String("type", string(relType)))
			continue
		}

		// Get the related actor
		relatedActor, err := h.repos.Actor().GetActor(ctx.Context, relatedUsername)
		if err != nil {
			h.logger.Warn("could not get related actor",
				zap.String("username", relatedUsername),
				zap.String("type", string(relType)),
				zap.Error(err))
			continue
		}

		// Convert to account
		account := converter.ActorToAccount(relatedActor)
		accounts = append(accounts, account)
	}

	// Set Link header for pagination if there's a cursor
	if nextCursor != "" {
		nextURL := fmt.Sprintf("%s/api/v1/accounts/%s/%s?max_id=%s",
			h.cfg.BaseURL(), accountID, string(relType), nextCursor)
		ctx.Response.Header("Link", fmt.Sprintf(`<%s>; rel="next"`, nextURL))
	}

	return ctx.JSON(accounts)
}

// HandleGetAccountFollowersLift retrieves the list of accounts following the given account
func (h *Handler) HandleGetAccountFollowersLift(ctx *lift.Context) error {
	return h.handleAccountRelationshipsList(ctx, relationshipFollowers)
}

// HandleGetAccountFollowingLift retrieves the list of accounts the given account is following
func (h *Handler) HandleGetAccountFollowingLift(ctx *lift.Context) error {
	return h.handleAccountRelationshipsList(ctx, relationshipFollowing)
}

// HandleGetFamiliarFollowersLift returns accounts that the requesting user follows and who also follow the given account
func (h *Handler) HandleGetFamiliarFollowersLift(ctx *lift.Context) error {
	// Extract token from Authorization header
	authHeader := ctx.Header("Authorization")
	token, err := auth.ExtractBearerToken(authHeader)
	if err != nil {
		return ctx.Status(401).JSON(map[string]string{"error": "Unauthorized"})
	}

	// Validate token and get claims
	oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.repos)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return ctx.Status(401).JSON(map[string]string{"error": "Unauthorized"})
	}

	// Get account IDs from query parameter
	accountIDs := ctx.Query("id[]")
	if accountIDs == "" {
		return ctx.Status(400).JSON(map[string]string{"error": "id[] parameter is required"})
	}

	// Split account IDs
	ids := strings.Split(accountIDs, ",")
	if len(ids) == 0 {
		return ctx.Status(400).JSON(map[string]string{"error": "At least one account ID is required"})
	}

	// Get current user's following list
	following, _, err := h.repos.Relationship().GetFollowing(ctx.Context, claims.Username, 1000, "") // Get a reasonable number
	if err != nil {
		h.logger.Error("failed to get following", zap.Error(err))
		return ctx.Status(500).JSON(map[string]string{"error": "Internal server error"})
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
	converter := mastodon.NewConverter(h.cfg.BaseURL())

	for _, accountID := range ids {
		// Get followers of this account
		followers, _, err := h.repos.Relationship().GetFollowers(ctx.Context, accountID, 100, "")
		if err != nil {
			// Skip if account not found
			continue
		}

		// Find mutual connections (accounts that follow this user AND are followed by the current user)
		mutualAccounts := make([]models.Account, 0)
		for _, followerActorID := range followers {
			if userFollowsMap[followerActorID] {
				// Get actor details
				username := converter.ExtractUsernameFromActorID(followerActorID)
				if username != "" {
					actor, err := h.repos.Actor().GetActor(ctx.Context, username)
					if err == nil {
						account := converter.ActorToAccount(actor)
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

	return ctx.JSON(results)
}

// HandlePinAccountLift pins an account to the user's profile
func (h *Handler) HandlePinAccountLift(ctx *lift.Context) error {
	accountID := ctx.Param("id")
	if accountID == "" {
		return ctx.Status(400).JSON(map[string]string{"error": "Account ID is required"})
	}

	// Extract token from Authorization header
	authHeader := ctx.Header("Authorization")
	token, err := auth.ExtractBearerToken(authHeader)
	if err != nil {
		return ctx.Status(401).JSON(map[string]string{"error": "Unauthorized"})
	}

	// Validate token and get claims
	oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.repos)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return ctx.Status(401).JSON(map[string]string{"error": "Unauthorized"})
	}

	// Check write:accounts scope
	if !claims.HasScope(auth.ScopeWrite) {
		return ctx.Status(403).JSON(map[string]string{"error": "Insufficient scope"})
	}

	// Get target actor to verify it exists
	targetActor, err := h.repos.Actor().GetActor(ctx.Context, accountID)
	if err != nil {
		return ctx.Status(404).JSON(map[string]string{"error": "Account not found"})
	}

	// Create pin relationship
	pin := &storage.AccountPin{
		Username:       claims.Username,
		PinnedActorID:  targetActor.ID,
		PinnedUsername: accountID,
		CreatedAt:      time.Now(),
	}

	// Store the pin
	if err := h.repos.Account().CreateAccountPin(ctx.Context, pin); err != nil {
		if strings.Contains(err.Error(), "already pinned") {
			return ctx.Status(422).JSON(map[string]string{"error": "Account already pinned"})
		}
		h.logger.Error("failed to pin account", zap.Error(err))
		return ctx.Status(500).JSON(map[string]string{"error": "Internal server error"})
	}

	// Get relationship status to return
	relationship, err := h.getRelationshipMapLift(ctx.Context, claims.Username, accountID)
	if err != nil {
		h.logger.Error("failed to get relationship", zap.Error(err))
		return ctx.Status(500).JSON(map[string]string{"error": "Internal server error"})
	}

	return ctx.JSON(relationship)
}

// HandleUnpinAccountLift unpins an account from the user's profile
func (h *Handler) HandleUnpinAccountLift(ctx *lift.Context) error {
	accountID := ctx.Param("id")
	if accountID == "" {
		return ctx.Status(400).JSON(map[string]string{"error": "Account ID is required"})
	}

	// Extract token from Authorization header
	authHeader := ctx.Header("Authorization")
	token, err := auth.ExtractBearerToken(authHeader)
	if err != nil {
		return ctx.Status(401).JSON(map[string]string{"error": "Unauthorized"})
	}

	// Validate token and get claims
	oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.repos)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return ctx.Status(401).JSON(map[string]string{"error": "Unauthorized"})
	}

	// Check write:accounts scope
	if !claims.HasScope(auth.ScopeWrite) {
		return ctx.Status(403).JSON(map[string]string{"error": "Insufficient scope"})
	}

	// Get target actor to verify it exists
	targetActor, err := h.repos.Actor().GetActor(ctx.Context, accountID)
	if err != nil {
		return ctx.Status(404).JSON(map[string]string{"error": "Account not found"})
	}

	// Delete the pin
	if err := h.repos.Account().DeleteAccountPin(ctx.Context, claims.Username, targetActor.ID); err != nil {
		h.logger.Error("failed to unpin account", zap.Error(err))
		return ctx.Status(500).JSON(map[string]string{"error": "Internal server error"})
	}

	// Get relationship status to return
	relationship, err := h.getRelationshipMapLift(ctx.Context, claims.Username, accountID)
	if err != nil {
		h.logger.Error("failed to get relationship", zap.Error(err))
		return ctx.Status(500).JSON(map[string]string{"error": "Internal server error"})
	}

	return ctx.JSON(relationship)
}

// HandleSetAccountNoteLift sets a private note on an account
func (h *Handler) HandleSetAccountNoteLift(ctx *lift.Context) error {
	accountID := ctx.Param("id")
	if accountID == "" {
		return ctx.Status(400).JSON(map[string]string{"error": "Account ID is required"})
	}

	// Extract token from Authorization header
	authHeader := ctx.Header("Authorization")
	token, err := auth.ExtractBearerToken(authHeader)
	if err != nil {
		return ctx.Status(401).JSON(map[string]string{"error": "Unauthorized"})
	}

	// Validate token and get claims
	oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.repos)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return ctx.Status(401).JSON(map[string]string{"error": "Unauthorized"})
	}

	// Check write:accounts scope
	if !claims.HasScope(auth.ScopeWrite) {
		return ctx.Status(403).JSON(map[string]string{"error": "Insufficient scope"})
	}

	// Parse request body
	var req struct {
		Comment string `json:"comment"`
	}
	if err := ctx.ParseRequest(&req); err != nil {
		return ctx.Status(400).JSON(map[string]string{"error": err.Error()})
	}

	// Get target actor to verify it exists
	targetActor, err := h.repos.Actor().GetActor(ctx.Context, accountID)
	if err != nil {
		return ctx.Status(404).JSON(map[string]string{"error": "Account not found"})
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
	if err := h.repos.Account().CreateAccountNote(ctx.Context, note); err != nil {
		h.logger.Error("failed to set account note", zap.Error(err))
		return ctx.Status(500).JSON(map[string]string{"error": "Internal server error"})
	}

	// Get relationship status to return
	relationship, err := h.getRelationshipMapLift(ctx.Context, claims.Username, accountID)
	if err != nil {
		h.logger.Error("failed to get relationship", zap.Error(err))
		return ctx.Status(500).JSON(map[string]string{"error": "Internal server error"})
	}

	// Add the note to the relationship
	relationship["note"] = req.Comment

	return ctx.JSON(relationship)
}

// HandleRemoveFromFollowersLift removes a follower from the current user's followers list
func (h *Handler) HandleRemoveFromFollowersLift(ctx *lift.Context) error {
	accountID := ctx.Param("id")
	if accountID == "" {
		return ctx.Status(400).JSON(map[string]string{"error": "Account ID is required"})
	}

	// Extract token from Authorization header
	authHeader := ctx.Header("Authorization")
	token, err := auth.ExtractBearerToken(authHeader)
	if err != nil {
		return ctx.Status(401).JSON(map[string]string{"error": "Unauthorized"})
	}

	// Validate token and get claims
	oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.repos)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return ctx.Status(401).JSON(map[string]string{"error": "Unauthorized"})
	}

	// Check write:follows scope
	if !claims.HasScope(auth.ScopeWrite) {
		return ctx.Status(403).JSON(map[string]string{"error": "Insufficient scope"})
	}

	// Get target actor to verify it exists
	_, err = h.repos.Actor().GetActor(ctx.Context, accountID)
	if err != nil {
		return ctx.Status(404).JSON(map[string]string{"error": "Account not found"})
	}

	// Check if target follows current user using GetRelationship
	relationship, err := h.repos.Relationship().GetRelationship(ctx.Context, accountID, claims.Username)
	if err != nil || relationship == nil {
		return ctx.Status(404).JSON(map[string]string{"error": "Account is not following you"})
	}

	// Remove the follow relationship
	if err := h.repos.Relationship().DeleteRelationship(ctx.Context, accountID, claims.Username); err != nil {
		h.logger.Error("failed to remove follower", zap.Error(err))
		return ctx.Status(500).JSON(map[string]string{"error": "Internal server error"})
	}

	// Get relationship status to return
	relationshipMap, err := h.getRelationshipMapLift(ctx.Context, claims.Username, accountID)
	if err != nil {
		h.logger.Error("failed to get relationship", zap.Error(err))
		return ctx.Status(500).JSON(map[string]string{"error": "Internal server error"})
	}

	return ctx.JSON(relationshipMap)
}

// Helper methods for Lift implementation

// validateRegistrationRequestLift validates a registration request
func (h *Handler) validateRegistrationRequestLift(req models.AccountRegistrationRequest) error {
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

	// Validate agreement
	if !req.Agreement {
		return errors.New("you must agree to the terms of service")
	}

	return nil
}

// resolveAccountIDLift resolves an account ID (which can be a username, numeric ID, or URL) to an actor
func (h *Handler) resolveAccountIDLift(ctx context.Context, accountID string) (*activitypub.Actor, error) {
	return h.resolveAccountID(ctx, accountID)
}

// getRelationshipMapLift gets relationship status for Lift implementation
func (h *Handler) getRelationshipMapLift(ctx context.Context, currentUsername, targetUsername string) (map[string]any, error) {
	// Get actors
	currentActor, err := h.repos.Actor().GetActor(ctx, currentUsername)
	if err != nil {
		return nil, err
	}

	targetActor, err := h.repos.Actor().GetActor(ctx, targetUsername)
	if err != nil {
		return nil, err
	}

	// Check various relationship statuses using GetRelationship
	followingRel, _ := h.repos.Relationship().GetRelationship(ctx, currentUsername, targetUsername)
	following := followingRel != nil
	followedByRel, _ := h.repos.Relationship().GetRelationship(ctx, targetUsername, currentUsername)
	followedBy := followedByRel != nil

	// Check if pinned
	endorsed, _ := h.repos.Account().IsAccountPinned(ctx, currentUsername, targetActor.ID)

	// Get note if exists
	note, _ := h.repos.User().GetAccountNote(ctx, currentUsername, targetActor.ID)
	noteText := ""
	if note != nil {
		noteText = note.Note
	}

	// Check if muted
	muted, _ := h.repos.Social().IsMuted(ctx, currentActor.ID, targetActor.ID)

	// Check if blocked
	blocking, _ := h.repos.Relationship().IsBlocked(ctx, currentActor.ID, targetActor.ID)
	blockedBy, _ := h.repos.Relationship().IsBlocked(ctx, targetActor.ID, currentActor.ID)

	// Build relationship response
	relationship := map[string]any{
		"id":                   targetUsername,
		"following":            following,
		"showing_reblogs":      !h.isReblogFilteredLift(ctx, currentActor.ID, targetActor.ID),
		"notifying":            h.isNotifyingEnabledLift(ctx, currentActor.ID, targetActor.ID),
		"followed_by":          followedBy,
		"blocking":             blocking,
		"blocked_by":           blockedBy,
		"muting":               muted,
		"muting_notifications": h.isNotificationsMutedLift(ctx, currentActor.ID, targetActor.ID),
		"requested":            h.hasFollowRequestLift(ctx, currentActor.ID, targetActor.ID),
		"domain_blocking":      h.isDomainBlockedLift(ctx, currentActor.ID, targetActor.ID),
		"endorsed":             endorsed,
		"note":                 noteText,
	}

	return relationship, nil
}

// uploadProfileImageLift uploads a profile image (avatar or header) to S3
func (h *Handler) uploadProfileImageLift(ctx context.Context, username, imageType string, data []byte, contentType string) (string, error) {
	// Generate unique ID based on timestamp
	imageID := fmt.Sprintf("%d", time.Now().UnixNano())

	// Get file extension
	ext := h.getExtensionFromImageMimeTypeLift(contentType)

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

// isAllowedImageMimeTypeLift checks if the mime type is allowed for images
func (h *Handler) isAllowedImageMimeTypeLift(mimeType string) bool {
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

// getExtensionFromImageMimeTypeLift returns the file extension for an image mime type
func (h *Handler) getExtensionFromImageMimeTypeLift(mimeType string) string {
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
func mathMin(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// formatLastStatusTimeLift formats the last status time for API response
func (h *Handler) formatLastStatusTimeLift(lastStatusAt *time.Time) string {
	if lastStatusAt == nil {
		return ""
	}
	return lastStatusAt.Format("2006-01-02T15:04:05.000Z")
}

// formatActorCreatedTimeLift formats the actor creation time for API response
func (h *Handler) formatActorCreatedTimeLift(createdAt *time.Time) string {
	if createdAt == nil {
		return time.Now().Format(time.RFC3339)
	}
	return createdAt.Format(time.RFC3339)
}

// getHeaderURLLift returns the header image URL for an actor
func (h *Handler) getHeaderURLLift(actor *activitypub.Actor) string {
	if actor.Image != nil && actor.Image.URL != "" {
		return actor.Image.URL
	}
	return ""
}

// parseActorFieldsLift parses actor attachment fields into Mastodon format
func (h *Handler) parseActorFieldsLift(ctx context.Context, actor *activitypub.Actor, attachments []activitypub.Attachment) []any {
	var fields []any
	for _, attachment := range attachments {
		if attachment.Type == "PropertyValue" {
			field := map[string]any{
				"name":        attachment.Name,
				"value":       attachment.Value,
				"verified_at": h.getFieldVerificationTimeLift(ctx, actor.PreferredUsername, attachment.Name),
			}
			fields = append(fields, field)
		}
	}
	return fields
}

// isReblogFilteredLift checks if reblogs are filtered for a relationship
func (h *Handler) isReblogFilteredLift(ctx context.Context, followerID, followeeID string) bool {
	// Check user preference for showing reblogs from this user
	// Use extended preferences to check if reblogs are filtered
	showReblogs, err := h.repos.Account().GetPreference(ctx, followerID, fmt.Sprintf("show_reblogs:%s", followeeID))
	if err != nil {
		h.logger.Debug("failed to get reblog preference", zap.Error(err))
		return false // Default to showing reblogs if error
	}

	// If preference exists and is "false", reblogs are filtered
	if showReblogs != "" {
		if showReblogs == "false" {
			return true // Filtered if show is false
		}
	}

	return false // Default to not filtered
}

// isNotifyingEnabledLift checks if notifications are enabled for a relationship
func (h *Handler) isNotifyingEnabledLift(ctx context.Context, followerID, followeeID string) bool {
	// Check user preference for notifications from this user
	notifyEnabled, err := h.repos.Account().GetPreference(ctx, followerID, fmt.Sprintf("notify:%s", followeeID))
	if err != nil {
		h.logger.Debug("failed to check notification preference", zap.Error(err))
		return false // Default to not notifying if error
	}

	// If preference exists and is "true", notifications are enabled
	if notifyEnabled != "" {
		return notifyEnabled == boolTrue
	}

	return false // Default to not notifying
}

// isNotificationsMutedLift checks if notifications are muted for a relationship
func (h *Handler) isNotificationsMutedLift(ctx context.Context, muterID, muteeID string) bool {
	// Check user preference for muting notifications from this user
	notifMuted, err := h.repos.Account().GetPreference(ctx, muterID, fmt.Sprintf("mute_notifications:%s", muteeID))
	if err != nil {
		h.logger.Debug("failed to check notification mute preference", zap.Error(err))
		return false // Default to not muted if error
	}

	// If preference exists and is "true", notifications are muted
	if notifMuted != "" {
		return notifMuted == boolTrue
	}

	return false // Default to not muted
}

// hasFollowRequestLift checks if there's a pending follow request
func (h *Handler) hasFollowRequestLift(ctx context.Context, requesterID, targetID string) bool {
	// Check follow request state - returns "pending", "accepted", "rejected", or ""
	state, err := h.repos.Account().GetFollowRequestState(ctx, requesterID, targetID)
	if err != nil {
		h.logger.Debug("failed to check follow request state", zap.Error(err))
		return false // Default to no pending request if error
	}

	return state == "pending"
}

// isDomainBlockedLift checks if a domain is blocked
func (h *Handler) isDomainBlockedLift(ctx context.Context, userID, targetID string) bool {
	// Extract domain from targetID if it's a full actor URL
	targetDomain := h.extractDomainFromActorIDLift(targetID)
	if targetDomain == "" {
		return false // Local user, no domain blocking
	}

	// Check if user has blocked this domain
	blocked, err := h.repos.Account().IsBlockedDomain(ctx, userID, targetDomain)
	if err != nil {
		h.logger.Debug("failed to check domain block status", zap.Error(err))
		return false // Default to not blocked if error
	}

	return blocked
}

// extractDomainFromActorIDLift extracts domain from an actor ID URL
func (h *Handler) extractDomainFromActorIDLift(actorID string) string {
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

// getFieldVerificationTimeLift gets the verification time for a profile field
func (h *Handler) getFieldVerificationTimeLift(ctx context.Context, username, fieldName string) any {
	field, err := h.repos.Account().GetFieldVerification(ctx, username, fieldName)
	if err != nil {
		h.logger.Warn("failed to get field verification",
			zap.String("username", username),
			zap.String("field_name", fieldName),
			zap.Error(err))
		return nil
	}

	if !field.VerifiedAt.IsZero() {
		return field.VerifiedAt.Format("2006-01-02T15:04:05.000Z")
	}

	return nil
}

// HandleActivityPubFollowersLift handles ActivityPub followers collection endpoint
func (h *Handler) HandleActivityPubFollowersLift(ctx *lift.Context) error {
	return h.handleActivityPubCollection(ctx, "followers")
}

// HandleActivityPubFollowingLift handles ActivityPub following collection endpoint  
func (h *Handler) HandleActivityPubFollowingLift(ctx *lift.Context) error {
	return h.handleActivityPubCollection(ctx, "following")
}

// handleActivityPubCollection handles ActivityPub collection requests with proper format
func (h *Handler) handleActivityPubCollection(ctx *lift.Context, collectionType string) error {
	// Extract username from path parameters
	username := ctx.Param("username")
	if username == "" {
		return ctx.Status(400).JSON(map[string]string{"error": "missing username"})
	}

	// Check if actor exists
	actor, err := h.repos.Actor().GetActor(ctx.Context, username)
	if err != nil {
		return ctx.Status(404).JSON(map[string]string{"error": "user not found"})
	}

	// Check Accept header for content negotiation
	acceptHeader := ctx.Header("Accept")
	isActivityPub := strings.Contains(acceptHeader, "application/activity+json") ||
		strings.Contains(acceptHeader, "application/ld+json") ||
		strings.Contains(acceptHeader, "application/json")

	// If not requesting ActivityPub format, redirect to API endpoint
	if !isActivityPub {
		// Return Mastodon API format instead
		switch collectionType {
		case string(relationshipFollowers):
			return h.HandleGetAccountFollowersLift(ctx)
		case string(relationshipFollowing):
			return h.HandleGetAccountFollowingLift(ctx)
		}
	}

	// Parse query parameters for pagination
	isPage := ctx.Query("page") != ""
	cursor := ctx.Query("cursor") 
	limit := 20 // default limit
	if l := ctx.Query("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed >= 1 && parsed <= 100 {
			limit = parsed
		}
	}

	// If not requesting a page, return the collection metadata
	if !isPage {
		return h.returnActivityPubCollection(ctx, actor, collectionType)
	}

	// Get collection data with pagination
	return h.returnActivityPubCollectionPage(ctx, actor, collectionType, cursor, limit)
}

// returnActivityPubCollection returns ActivityPub OrderedCollection metadata
func (h *Handler) returnActivityPubCollection(ctx *lift.Context, actor *activitypub.Actor, collectionType string) error {
	// Check privacy settings for followers collection
	if collectionType == string(relationshipFollowers) && actor.ManuallyApprovesFollowers {
		// Check if the requester is authorized to view followers
		authHeader := ctx.Header("Authorization")
		if authHeader != "" {
			token, err := auth.ExtractBearerToken(authHeader)
			if err == nil {
				oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.repos)
				claims, err := oauthSvc.ValidateAccessToken(token)
				if err != nil || claims.Username != actor.PreferredUsername {
					// Other users cannot see private followers
					return h.returnEmptyCollection(ctx, actor, collectionType)
				}
			} else {
				// No valid authentication, return empty collection
				return h.returnEmptyCollection(ctx, actor, collectionType)
			}
		} else {
			// No authentication, return empty collection for private accounts
			return h.returnEmptyCollection(ctx, actor, collectionType)
		}
	}

	// Get total count
	var totalItems int
	var err error

	switch collectionType {
	case "followers":
		totalItems, err = h.repos.Relationship().CountFollowers(ctx.Context, actor.PreferredUsername)
	case "following":
		totalItems, err = h.repos.Relationship().CountFollowing(ctx.Context, actor.PreferredUsername)
	}

	if err != nil {
		h.logger.Error("failed to get collection count", zap.String("type", collectionType), zap.Error(err))
		return ctx.Status(500).JSON(map[string]string{"error": "internal server error"})
	}

	// Build collection URL
	collectionID := fmt.Sprintf("%s/%s", actor.ID, collectionType)

	// Build the collection
	collection := &activitypub.OrderedCollection{
		Collection: activitypub.Collection{
			BaseObject: activitypub.BaseObject{
				Context: activitypub.Context,
				ID:      collectionID,
				Type:    activitypub.OrderedCollectionType,
			},
			TotalItems: totalItems,
		},
	}

	// Only add first page link if there are items
	if totalItems > 0 {
		collection.First = fmt.Sprintf("%s?page=1", collectionID)
	}

	// Set ActivityPub content type and caching headers
	ctx.Response.Header("Content-Type", "application/activity+json")
	ctx.Response.Header("Cache-Control", "max-age=300")
	return ctx.JSON(collection)
}

// returnActivityPubCollectionPage returns ActivityPub OrderedCollectionPage with items
func (h *Handler) returnActivityPubCollectionPage(ctx *lift.Context, actor *activitypub.Actor, collectionType, cursor string, limit int) error {
	// Check privacy settings for followers collection
	if collectionType == string(relationshipFollowers) && actor.ManuallyApprovesFollowers {
		// Check if the requester is authorized to view followers
		authHeader := ctx.Header("Authorization")
		if authHeader != "" {
			token, err := auth.ExtractBearerToken(authHeader)
			if err == nil {
				oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.repos)
				claims, err := oauthSvc.ValidateAccessToken(token)
				if err != nil || claims.Username != actor.PreferredUsername {
					// Other users cannot see private followers
					return h.returnEmptyCollection(ctx, actor, collectionType)
				}
			} else {
				// No valid authentication, return empty collection
				return h.returnEmptyCollection(ctx, actor, collectionType)
			}
		} else {
			// No authentication, return empty collection for private accounts
			return h.returnEmptyCollection(ctx, actor, collectionType)
		}
	}

	// Get relationships based on type
	var usernames []string
	var nextCursor string
	var err error

	switch collectionType {
	case "followers":
		usernames, nextCursor, err = h.repos.Relationship().GetFollowers(ctx.Context, actor.PreferredUsername, limit, cursor)
	case "following":
		usernames, nextCursor, err = h.repos.Relationship().GetFollowing(ctx.Context, actor.PreferredUsername, limit, cursor)
	}

	if err != nil {
		h.logger.Error("failed to get collection data", zap.String("type", collectionType), zap.Error(err))
		return ctx.Status(500).JSON(map[string]string{"error": "internal server error"})
	}

	// Convert usernames to actor URLs
	orderedItems := make([]any, len(usernames))
	for i, username := range usernames {
		orderedItems[i] = fmt.Sprintf("%s/users/%s", h.cfg.BaseURL(), username)
	}

	// Build collection and page URLs
	collectionID := fmt.Sprintf("%s/%s", actor.ID, collectionType)
	pageID := fmt.Sprintf("%s?page=1", collectionID)
	if cursor != "" {
		pageID = fmt.Sprintf("%s&cursor=%s", pageID, cursor)
	}

	// Build the page
	page := &activitypub.OrderedCollectionPage{
		CollectionPage: activitypub.CollectionPage{
			Collection: activitypub.Collection{
				BaseObject: activitypub.BaseObject{
					Context: activitypub.Context,
					ID:      pageID,
					Type:    activitypub.OrderedCollectionPageType,
				},
				OrderedItems: orderedItems,
			},
			PartOf: collectionID,
		},
	}

	// Add next link if there are more items
	if nextCursor != "" {
		page.Next = fmt.Sprintf("%s?page=1&cursor=%s&limit=%d", collectionID, nextCursor, limit)
	}

	// Add prev link if this is not the first page
	if cursor != "" {
		// For simplicity, just link back to the first page
		// In a full implementation, you'd calculate the previous cursor
		page.Prev = fmt.Sprintf("%s?page=1", collectionID)
	}

	// Set ActivityPub content type and caching headers
	ctx.Response.Header("Content-Type", "application/activity+json")
	ctx.Response.Header("Cache-Control", "max-age=300")
	return ctx.JSON(page)
}

// returnEmptyCollection returns an empty ActivityPub collection for privacy protection
func (h *Handler) returnEmptyCollection(ctx *lift.Context, actor *activitypub.Actor, collectionType string) error {
	// Build collection URL
	collectionID := fmt.Sprintf("%s/%s", actor.ID, collectionType)

	// Return empty collection
	collection := &activitypub.OrderedCollection{
		Collection: activitypub.Collection{
			BaseObject: activitypub.BaseObject{
				Context: activitypub.Context,
				ID:      collectionID,
				Type:    activitypub.OrderedCollectionType,
			},
			TotalItems: 0,
		},
	}

	ctx.Response.Header("Content-Type", "application/activity+json")
	ctx.Response.Header("Cache-Control", "max-age=300")
	return ctx.JSON(collection)
}

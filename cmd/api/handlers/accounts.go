package handlers

import (
	"encoding/base64"
	"errors"
	"fmt"
	"mime"
	"strings"
	"time"

	"github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/services/accounts"
	"github.com/equaltoai/lesser/pkg/storage"
	storageModels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/transformations"
	apptheory "github.com/theory-cloud/apptheory/runtime"
	"go.uber.org/zap"
)

const (
	boolTrue = "true"
)

// HandleRegistrationLift handles user registration requests
func (h *Handler) HandleRegistrationLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	// Parse request body
	var req models.AccountRegistrationRequest
	if resp, err := common.ParseRequestWithValidation(ctx, &req); resp != nil {
		return resp, err
	}

	// Validate request
	if err := h.validateRegistrationRequestLift(req); err != nil {
		return common.RespondUnprocessableEntity(ctx, err.Error())
	}

	// Require wallet verification as the registration proof (passwordless signup).
	// The auth-ui flow calls POST /auth/wallet/verify first, which marks the challenge as used.
	authService, authResp, err := h.requireAuthService(ctx)
	if authResp != nil || err != nil {
		return authResp, err
	}

	challengeID := strings.TrimSpace(req.WalletChallengeID)
	challenge, err := authService.GetWalletChallenge(ctx.Context(), challengeID)
	if err != nil || challenge == nil {
		return common.RespondUnprocessableEntity(ctx, "wallet_challenge_id is invalid or expired")
	}
	if strings.TrimSpace(challenge.Username) != strings.TrimSpace(req.Username) {
		return common.RespondUnprocessableEntity(ctx, "wallet challenge was created for a different username")
	}
	if !challenge.Used {
		return common.RespondUnprocessableEntity(ctx, "wallet challenge has not been verified")
	}
	if challenge.Spent {
		return common.RespondUnprocessableEntity(ctx, "wallet challenge was already spent")
	}

	// NOTE: Password-based authentication is disabled. This system uses WebAuthn/crypto wallet authentication only.
	// If a password is provided in the request, it will be ignored to prevent password-based access.
	if req.Password != "" {
		h.logger.Warn("password provided in registration request but passwords are disabled",
			zap.String("username", req.Username))
		req.Password = "" // Clear password to enforce passwordless authentication
	}

	// Use Accounts service to register the account
	accountsService := h.registry.Accounts()
	result, err := accountsService.RegisterAccount(ctx.Context(), &accounts.RegisterAccountCommand{
		Username:                 req.Username,
		Email:                    "", // Email is forbidden - always empty
		Password:                 req.Password,
		Locale:                   req.Locale,
		Agreement:                req.Agreement,
		Reason:                   req.Reason,
		DefaultPostingVisibility: req.DefaultPostingVisibility,
		RegistrationChallengeID:  challengeID,
	})
	if err != nil {
		if errors.Is(err, accounts.ErrUsernameAlreadyTaken) {
			return common.RespondAlreadyExists(ctx, "Username")
		}
		if errors.Is(err, accounts.ErrValidationFailed) {
			return common.RespondUnprocessableEntity(ctx, err.Error())
		}
		// Log the full error for debugging
		h.logger.Error("failed to register account",
			zap.String("username", req.Username),
			zap.Error(err))
		// Return error message instead of generic internal server error
		return common.RespondUnprocessableEntity(ctx, fmt.Sprintf("registration failed: %v", err))
	}

	// Return response using common helper
	resp := models.AccountRegistrationResponse{
		ID:       result.Actor.ID,
		Username: result.Account.User.Username,
		Created:  true,
	}

	return h.respondCreated(ctx, resp)
}

// HandleVerifyCredentialsLift returns the current user's information
func (h *Handler) HandleVerifyCredentialsLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	h.logger.Info("handle verify_credentials: start")

	// Authenticate user
	claims, err := h.authenticateWithScope(ctx, auth.ScopeRead)
	if err != nil {
		h.logger.Warn("handle verify_credentials: authentication failed", zap.Error(err))
		return nil, err
	}

	h.logger.Info("handle verify_credentials: authentication succeeded",
		zap.String("username", claims.Username))

	// Check if registry is available
	if h.registry == nil {
		h.logger.Error("handle verify_credentials: service registry not initialized")
		return common.RespondInternalServerError(ctx)
	}

	// Call Accounts service
	account, err := h.registry.Accounts().GetAccount(ctx.Context(), claims.Username)
	if err != nil {
		h.logger.Error("handle verify_credentials: failed to get account",
			zap.String("username", claims.Username),
			zap.Error(err))
		return common.RespondInternalServerError(ctx)
	}

	h.logger.Info("handle verify_credentials: account retrieved",
		zap.String("username", claims.Username))

	mastodonAccount, err := h.mastodonAccountFromStorageAccount(account)
	if err != nil {
		h.logger.Error("handle verify_credentials: failed to transform account response",
			zap.String("username", claims.Username),
			zap.Error(err))
		return common.RespondInternalServerError(ctx)
	}

	return h.respondOK(ctx, mastodonAccount)
}

func (h *Handler) mastodonAccountFromStorageAccount(account *storage.Account) (models.Account, error) {
	if account == nil || account.User == nil {
		return models.Account{}, fmt.Errorf("account is missing user")
	}

	username := strings.TrimSpace(account.User.Username)
	if username == "" {
		return models.Account{}, fmt.Errorf("account is missing username")
	}

	baseURL := handlerBaseURL(h)

	out := mastodonAccountFromActor(account.Actor, baseURL)
	applyMastodonIdentity(&out, account.User, username)
	applyMastodonProfile(&out, account.User, baseURL, username)
	ensureMastodonAccountCollections(&out)
	applyMastodonProfileFields(&out, account.User.Fields)

	return out, nil
}

func handlerBaseURL(h *Handler) string {
	if h == nil || h.cfg == nil {
		return ""
	}
	return strings.TrimSuffix(strings.TrimSpace(h.cfg.BaseURL()), "/")
}

func mastodonAccountFromActor(actor *activitypub.Actor, baseURL string) models.Account {
	if actor == nil {
		return models.Account{}
	}
	return transformations.ActorToAccountWithCounts(actor, baseURL, 0, 0, 0)
}

func applyMastodonIdentity(out *models.Account, user *storage.User, username string) {
	if out == nil || user == nil {
		return
	}

	out.Username = username
	out.Acct = username
	if id := strings.TrimSpace(user.ID); id != "" {
		out.ID = id
	} else {
		out.ID = common.GenerateNumericID(username)
	}
}

func applyMastodonProfile(out *models.Account, user *storage.User, baseURL, username string) {
	if out == nil || user == nil {
		return
	}

	if displayName := strings.TrimSpace(user.DisplayName); displayName != "" {
		out.DisplayName = displayName
	}
	if out.DisplayName == "" {
		out.DisplayName = username
	}
	if note := strings.TrimSpace(user.Note); note != "" {
		out.Note = note
	}

	out.Locked = user.Locked
	out.Discoverable = user.Discoverable
	if user.IsAgent {
		out.Bot = true
	}

	if !user.CreatedAt.IsZero() {
		out.CreatedAt = user.CreatedAt.UTC().Format(time.RFC3339)
	} else if out.CreatedAt == "" {
		out.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	}

	if profileURL := strings.TrimSpace(user.URL); profileURL != "" {
		out.URL = profileURL
	}
	if out.URL == "" && baseURL != "" {
		out.URL = fmt.Sprintf("%s/@%s", baseURL, username)
	}

	if avatar := strings.TrimSpace(user.Avatar); avatar != "" {
		out.Avatar = avatar
		out.AvatarStatic = avatar
	}
	if header := strings.TrimSpace(user.Header); header != "" {
		out.Header = header
		out.HeaderStatic = header
	}

	if out.Avatar == "" && baseURL != "" {
		out.Avatar = baseURL + "/avatars/original/missing.png"
	}
	if out.AvatarStatic == "" {
		out.AvatarStatic = out.Avatar
	}
	if out.Header == "" && baseURL != "" {
		out.Header = baseURL + "/headers/original/missing.png"
	}
	if out.HeaderStatic == "" {
		out.HeaderStatic = out.Header
	}
}

func ensureMastodonAccountCollections(out *models.Account) {
	if out == nil {
		return
	}

	// Ensure JSON arrays are serialized as [] not null (OpenAPI).
	if out.Emojis == nil {
		out.Emojis = []any{}
	}
	if out.Fields == nil {
		out.Fields = []any{}
	}
}

func applyMastodonProfileFields(out *models.Account, storedFields []map[string]string) {
	if out == nil || len(storedFields) == 0 {
		return
	}

	fields := make([]any, 0, len(storedFields))
	for _, field := range storedFields {
		if field == nil {
			continue
		}
		m := map[string]any{
			"name":        field["name"],
			"value":       field["value"],
			"verified_at": nil,
		}
		if verifiedAt := strings.TrimSpace(field["verified_at"]); verifiedAt != "" {
			m["verified_at"] = verifiedAt
		}
		fields = append(fields, m)
	}
	out.Fields = fields
}

// HandleUpdateCredentialsLift updates the current user's profile
func (h *Handler) HandleUpdateCredentialsLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	// Parse request
	var req models.UpdateCredentialsRequest
	if err := common.ParseRequestWithFallback(ctx, &req); err != nil {
		return common.RespondInvalidRequest(ctx)
	}

	// Validate account parameters using comprehensive validation
	accountParams := map[string]interface{}{
		"display_name": req.DisplayName,
		"note":         req.Note,
		"locked":       req.Locked,
		"bot":          req.Bot,
		"discoverable": req.Discoverable,
	}
	if err := common.ValidateAccountParams(accountParams); err != nil {
		return common.RespondBadRequest(ctx, err.Error())
	}

	// Authenticate user
	claims, err := h.authenticateWithScope(ctx, auth.ScopeWrite)
	if err != nil {
		return nil, err
	}

	// Call Accounts service
	result, err := h.registry.Accounts().UpdateProfile(ctx.Context(), &accounts.UpdateProfileCommand{
		Username:     claims.Username,
		DisplayName:  req.DisplayName,
		Bio:          req.Note,
		Locked:       req.Locked,
		Bot:          req.Bot,
		Discoverable: req.Discoverable,
		UpdaterID:    claims.Username,
	})
	if err != nil {
		h.logger.Error("failed to update profile", zap.Error(err))
		return common.RespondFailedToUpdate(ctx, "profile")
	}

	return h.respondOK(ctx, result.Account)
}

// UpdateCredentialsFileData holds file upload data from multipart form
type UpdateCredentialsFileData struct {
	AvatarData        []byte
	AvatarContentType string
	HeaderData        []byte
	HeaderContentType string
}

// authenticateAndGetActor handles authentication and retrieves the actor

// parseUpdateCredentialsRequest parses both JSON and multipart form requests

// parseMultipartRequest handles multipart form parsing with base64 decoding

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
	return nil, unableToParseRequestBody()
}

// extractBoundary extracts the boundary from content type header
func (h *Handler) extractBoundary(contentType string) (string, error) {
	_, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		h.logger.Error("failed to parse content type", zap.String("content_type", contentType), zap.Error(err))
		return "", errors.Join(invalidContentType(), err)
	}

	boundary := params["boundary"]
	if err := common.ValidateRequiredParam("boundary", boundary); err != nil {
		return "", missingBoundaryInContentType()
	}

	return boundary, nil
}

// processMultipartPart processes a single multipart form part

// handleFileUploads handles avatar and header file uploads

// uploadAndSetAvatar validates and uploads avatar image

// uploadAndSetHeader validates and uploads header image

// validateImageUpload validates image file size and type

// updateActorFromRequest updates actor fields from the request

// buildCredentialsResponse builds the credentials response

// HandleGetAccountLift retrieves account information by ID (username or URL)
func (h *Handler) HandleGetAccountLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	accountID := ctx.Param("id")
	if err := common.ValidateAccountParamID(accountID); err != nil {
		return common.RespondValidationError(ctx, err)
	}

	// Call Accounts service
	account, err := h.registry.Accounts().GetAccount(ctx.Context(), accountID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return common.RespondAccountNotFound(ctx)
		}
		h.logger.Error("failed to get account", zap.Error(err))
		return common.RespondInternalServerError(ctx)
	}

	return okJSON(account)
}

// HandleAccountLookupLift looks up an account by username@domain
func (h *Handler) HandleAccountLookupLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	// Get acct parameter
	acct := queryValue(ctx, "acct")
	if err := common.ValidateAcctParameter(acct); err != nil {
		return common.RespondValidationError(ctx, err)
	}

	// Call Accounts service
	account, err := h.registry.Accounts().LookupAccount(ctx.Context(), &accounts.LookupAccountQuery{
		Acct:     acct,
		ViewerID: "", // Anonymous lookup for now
	})
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return common.RespondAccountNotFound(ctx)
		}
		h.logger.Error("failed to lookup account", zap.Error(err))
		return common.RespondInternalServerError(ctx)
	}

	// Convert to Mastodon account format
	mastodonAccount := transformations.ActorToAccountWithCounts(account.Actor, h.cfg.BaseURL(), 0, 0, 0)

	return okJSON(mastodonAccount)
}

// relationshipType represents the type of relationship being queried
type relationshipType string

const (
	relationshipFollowers relationshipType = "followers"
	relationshipFollowing relationshipType = "following"
)

// handleAccountRelationshipsList is a generic handler for followers and following lists
func (h *Handler) handleAccountRelationshipsList(ctx *apptheory.Context, relType relationshipType) (*apptheory.Response, error) {
	accountID := ctx.Param("id")
	if err := common.ValidateAccountParamID(accountID); err != nil {
		return common.RespondValidationError(ctx, err)
	}

	// Extract username from accountID
	username := accountID

	// Get the actor to verify it exists
	_, err := h.registry.Accounts().GetAccount(ctx.Context(), username)
	if err != nil {
		return common.RespondAccountNotFound(ctx)
	}

	// Parse pagination parameters
	limit := 40
	maxID := queryValue(ctx, "max_id")
	minID := queryValue(ctx, "min_id")
	cursor := maxID

	// Use minID as cursor if provided and maxID is not
	if minID != "" && maxID == "" {
		cursor = minID
	}

	// Get relationships based on type
	var relationshipAccounts []*storage.Account
	var nextCursor string

	switch relType {
	case relationshipFollowers:
		relationshipAccounts, nextCursor, err = h.registry.Relationships().GetFollowers(ctx.Context(), username, limit, cursor)
		if err != nil {
			h.logger.Error("failed to get followers", zap.Error(err))
			return common.RespondInternalServerError(ctx)
		}
	case relationshipFollowing:
		relationshipAccounts, nextCursor, err = h.registry.Relationships().GetFollowing(ctx.Context(), username, limit, cursor)
		if err != nil {
			h.logger.Error("failed to get following", zap.Error(err))
			return common.RespondInternalServerError(ctx)
		}
	}

	// Convert relationships to Mastodon account format
	accounts := make([]models.Account, 0, len(relationshipAccounts))
	for _, relatedAccount := range relationshipAccounts {
		if relatedAccount == nil || relatedAccount.Actor == nil {
			continue
		}

		// Convert to account using the Actor from the account we already have
		account := transformations.ActorToAccountBase(relatedAccount.Actor, h.cfg.BaseURL())
		accounts = append(accounts, account)
	}

	resp, err := okJSON(accounts)
	if err != nil {
		return nil, err
	}

	if nextCursor != "" {
		nextURL := fmt.Sprintf("%s/api/v1/accounts/%s/%s?max_id=%s",
			h.cfg.BaseURL(), accountID, string(relType), nextCursor)
		setHeader(resp, "Link", fmt.Sprintf(`<%s>; rel="next"`, nextURL))
	}

	return resp, nil
}

// HandleGetAccountFollowersLift retrieves the list of accounts following the given account
func (h *Handler) HandleGetAccountFollowersLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	return h.handleAccountRelationshipsList(ctx, relationshipFollowers)
}

// HandleGetAccountFollowingLift retrieves the list of accounts the given account is following
func (h *Handler) HandleGetAccountFollowingLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	return h.handleAccountRelationshipsList(ctx, relationshipFollowing)
}

// HandleGetFamiliarFollowersLift returns accounts that the requesting user follows and who also follow the given account
func (h *Handler) HandleGetFamiliarFollowersLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	// Extract token from Authorization header
	authHeader := headerValue(ctx, "Authorization")
	token, err := auth.ExtractBearerToken(authHeader)
	if err != nil {
		return common.RespondUnauthorized(ctx)
	}

	// Validate token and get claims
	oauthSvc := createOAuthService(h.cfg.JWTSecret, h.cfg, h.repos, h.logger)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return common.RespondUnauthorized(ctx)
	}

	// Get account IDs from query parameter
	accountIDs := queryValue(ctx, "id[]")
	ids, err := common.ValidateAccountIDsParameter(accountIDs)
	if err != nil {
		return common.RespondValidationError(ctx, err)
	}

	// Use Accounts service to get familiar followers
	result, err := h.registry.Accounts().GetFamiliarFollowers(ctx.Context(), &accounts.GetFamiliarFollowersQuery{
		AccountIDs: ids,
		ViewerID:   claims.Username,
	})
	if err != nil {
		h.logger.Error("failed to get familiar followers", zap.Error(err))
		return common.RespondInternalServerError(ctx)
	}

	// Build response for each requested account
	type FamiliarFollowersResponse struct {
		ID       string           `json:"id"`
		Accounts []models.Account `json:"accounts"`
	}

	results := make([]FamiliarFollowersResponse, 0, len(result.Results))

	for _, familiarResult := range result.Results {
		apiAccounts := make([]models.Account, 0, len(familiarResult.Accounts))
		for _, storageAccount := range familiarResult.Accounts {
			if storageAccount.Actor != nil {
				apiAccount := transformations.ActorToAccountBase(storageAccount.Actor, h.cfg.BaseURL())
				apiAccounts = append(apiAccounts, apiAccount)
			}
		}

		results = append(results, FamiliarFollowersResponse{
			ID:       familiarResult.ID,
			Accounts: apiAccounts,
		})
	}

	return okJSON(results)
}

// HandlePinAccountLift pins an account to the user's profile
func (h *Handler) HandlePinAccountLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	accountID := ctx.Param("id")
	if err := common.ValidateAccountParamID(accountID); err != nil {
		return common.RespondValidationError(ctx, err)
	}

	// Authenticate user
	claims, err := h.authenticateWithScope(ctx, auth.ScopeWrite)
	if err != nil {
		return nil, err
	}

	// Call Accounts service
	result, err := h.registry.Accounts().PinAccount(ctx.Context(), &accounts.PinAccountCommand{
		Username:      claims.Username,
		TargetAccount: accountID,
		PinnerID:      claims.Username,
	})
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return common.RespondAccountNotFound(ctx)
		}
		if strings.Contains(err.Error(), "already pinned") {
			return common.RespondUnprocessableEntity(ctx, "Account already pinned")
		}
		if strings.Contains(err.Error(), "unauthorized") {
			return common.RespondForbidden(ctx)
		}
		h.logger.Error("failed to pin account", zap.Error(err))
		return common.RespondInternalServerError(ctx)
	}

	return okJSON(result.Relationship)
}

// HandleUnpinAccountLift unpins an account from the user's profile
func (h *Handler) HandleUnpinAccountLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	accountID := ctx.Param("id")
	if err := common.ValidateAccountParamID(accountID); err != nil {
		return common.RespondValidationError(ctx, err)
	}

	// Authenticate user
	claims, err := h.authenticateWithScope(ctx, auth.ScopeWrite)
	if err != nil {
		return nil, err
	}

	// Call Accounts service
	result, err := h.registry.Accounts().UnpinAccount(ctx.Context(), &accounts.UnpinAccountCommand{
		Username:      claims.Username,
		TargetAccount: accountID,
		PinnerID:      claims.Username,
	})
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return common.RespondAccountNotFound(ctx)
		}
		if strings.Contains(err.Error(), "unauthorized") {
			return common.RespondForbidden(ctx)
		}
		h.logger.Error("failed to unpin account", zap.Error(err))
		return common.RespondInternalServerError(ctx)
	}

	return okJSON(result.Relationship)
}

// HandleSetAccountNoteLift sets a private note on an account
func (h *Handler) HandleSetAccountNoteLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	accountID := ctx.Param("id")
	if err := common.ValidateAccountParamID(accountID); err != nil {
		return common.RespondValidationError(ctx, err)
	}

	// Authenticate user
	claims, err := h.authenticateWithScope(ctx, auth.ScopeWrite)
	if err != nil {
		return nil, err
	}

	// Parse request body
	var req struct {
		Comment string `json:"comment"`
	}
	if err := common.ParseRequestWithFallback(ctx, &req); err != nil {
		return common.RespondValidationError(ctx, err)
	}

	// Call Accounts service
	result, err := h.registry.Accounts().SetAccountNote(ctx.Context(), &accounts.SetAccountNoteCommand{
		Username:      claims.Username,
		TargetAccount: accountID,
		Note:          req.Comment,
		SetterID:      claims.Username,
	})
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return common.RespondAccountNotFound(ctx)
		}
		if strings.Contains(err.Error(), "unauthorized") {
			return common.RespondForbidden(ctx)
		}
		h.logger.Error("failed to set account note", zap.Error(err))
		return common.RespondInternalServerError(ctx)
	}

	return okJSON(result.Relationship)
}

// HandleRemoveFromFollowersLift removes a follower from the current user's followers list
func (h *Handler) HandleRemoveFromFollowersLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	accountID := ctx.Param("id")
	if err := common.ValidateAccountParamID(accountID); err != nil {
		return common.RespondValidationError(ctx, err)
	}

	// Authenticate user
	claims, err := h.authenticateWithScope(ctx, auth.ScopeWrite)
	if err != nil {
		return nil, err
	}

	// Call Accounts service
	result, err := h.registry.Accounts().RemoveFollower(ctx.Context(), &accounts.RemoveFollowerCommand{
		Username:   claims.Username,
		FollowerID: accountID,
		RemoverID:  claims.Username,
	})
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return common.RespondNotFound(ctx, "Account not found or is not following you")
		}
		if strings.Contains(err.Error(), "unauthorized") {
			return common.RespondForbidden(ctx)
		}
		h.logger.Error("failed to remove follower", zap.Error(err))
		return common.RespondInternalServerError(ctx)
	}

	return okJSON(result.Relationship)
}

// Helper methods for Lift implementation

// validateRegistrationRequestLift validates a registration request
func (h *Handler) validateRegistrationRequestLift(req models.AccountRegistrationRequest) error {
	// Validate username using comprehensive validation
	if err := activitypub.ValidateUsername(req.Username); err != nil {
		return err
	}

	// Email validation removed - email is forbidden

	// Validate agreement
	if !req.Agreement {
		return errors.New("you must agree to the terms of service")
	}

	if req.DefaultPostingVisibility != "" {
		switch strings.ToLower(req.DefaultPostingVisibility) {
		case storageModels.VisibilityPublic, storageModels.VisibilityUnlisted,
			storageModels.VisibilityPrivate, storageModels.VisibilityDirect:
		default:
			return fmt.Errorf("default_posting_visibility must be one of public, unlisted, private, or direct")
		}
	}

	if strings.TrimSpace(req.WalletChallengeID) == "" {
		return errors.New("wallet_challenge_id is required")
	}

	return nil
}

// resolveAccountIDLift resolves an account ID (which can be a username, numeric ID, or URL) to an actor

// getRelationshipMapLift gets relationship status for Lift implementation

// uploadProfileImageLift uploads a profile image (avatar or header) to S3

// isAllowedImageMimeTypeLift checks if the mime type is allowed for images

// getExtensionFromImageMimeTypeLift returns the file extension for an image mime type

// min returns the minimum of two integers
func mathMin(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// formatLastStatusTimeLift formats the last status time for API response

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

// isReblogFilteredLift checks if reblogs are filtered for a relationship

// isNotifyingEnabledLift checks if notifications are enabled for a relationship

// isNotificationsMutedLift checks if notifications are muted for a relationship

// hasFollowRequestLift checks if there's a pending follow request

// isDomainBlockedLift checks if a domain is blocked

// extractDomainFromActorIDLift extracts domain from an actor ID URL

// getFieldVerificationTimeLift gets the verification time for a profile field

// HandleActivityPubFollowersLift handles ActivityPub followers collection endpoint
func (h *Handler) HandleActivityPubFollowersLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	return h.handleActivityPubCollection(ctx, "followers")
}

// HandleActivityPubFollowingLift handles ActivityPub following collection endpoint
func (h *Handler) HandleActivityPubFollowingLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	return h.handleActivityPubCollection(ctx, "following")
}

// handleActivityPubCollection handles ActivityPub collection requests with proper format
func (h *Handler) handleActivityPubCollection(ctx *apptheory.Context, collectionType string) (*apptheory.Response, error) {
	// Extract username from path parameters
	username := ctx.Param("username")
	if err := common.ValidateUsernameParamID(username); err != nil {
		return common.RespondValidationError(ctx, err)
	}

	// Check if actor exists
	actor, err := h.registry.Accounts().GetAccount(ctx.Context(), username)
	if err != nil {
		return common.RespondUserNotFound(ctx)
	}

	// Check Accept header for content negotiation
	acceptHeader := headerValue(ctx, "Accept")
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
	isPage := queryValue(ctx, "page") != ""
	cursor := queryValue(ctx, "cursor")
	limit, err := common.ParseAndValidateActivityPubLimit(queryValue(ctx, "limit"))
	if err != nil {
		return common.RespondValidationError(ctx, err)
	}

	// If not requesting a page, return the collection metadata
	if !isPage {
		return h.returnActivityPubCollection(ctx, actor.Actor, collectionType)
	}

	// Get collection data with pagination
	return h.returnActivityPubCollectionPage(ctx, actor.Actor, collectionType, cursor, limit)
}

// returnActivityPubCollection returns ActivityPub OrderedCollection metadata
func (h *Handler) returnActivityPubCollection(ctx *apptheory.Context, actor *activitypub.Actor, collectionType string) (*apptheory.Response, error) {
	// Check privacy settings for followers collection
	if collectionType == string(relationshipFollowers) && actor.ManuallyApprovesFollowers {
		// Check if the requester is authorized to view followers
		authHeader := headerValue(ctx, "Authorization")
		if authHeader != "" {
			token, err := auth.ExtractBearerToken(authHeader)
			if err == nil {
				oauthSvc := createOAuthService(h.cfg.JWTSecret, h.cfg, h.repos, h.logger)
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
		count, countErr := h.registry.Relationships().CountFollowers(ctx.Context(), actor.PreferredUsername)
		totalItems, err = int(count), countErr
	case "following":
		count, countErr := h.registry.Relationships().CountFollowing(ctx.Context(), actor.PreferredUsername)
		totalItems, err = int(count), countErr
	}

	if err != nil {
		h.logger.Error("failed to get collection count", zap.String("type", collectionType), zap.Error(err))
		return common.RespondInternalServerError(ctx)
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
	resp, err := okJSON(collection)
	if err != nil {
		return nil, err
	}
	setHeader(resp, "Content-Type", "application/activity+json")
	setHeader(resp, "Cache-Control", "max-age=300")
	return resp, nil
}

// returnActivityPubCollectionPage returns ActivityPub OrderedCollectionPage with items
func (h *Handler) returnActivityPubCollectionPage(ctx *apptheory.Context, actor *activitypub.Actor, collectionType, cursor string, limit int) (*apptheory.Response, error) {
	// Check privacy settings and authorization.
	if resp, err := h.checkCollectionAccess(ctx, actor, collectionType); resp != nil {
		return resp, err
	}

	// Get collection data
	usernames, nextCursor, err := h.getCollectionData(ctx, actor, collectionType, cursor, limit)
	if err != nil {
		h.logger.Error("failed to get collection data", zap.String("type", collectionType), zap.Error(err))
		return common.RespondInternalServerError(ctx)
	}

	// Build and return the ActivityPub collection page
	page := h.buildCollectionPage(actor, collectionType, cursor, nextCursor, limit, usernames)

	// Set ActivityPub content type and caching headers
	resp, err := okJSON(page)
	if err != nil {
		return nil, err
	}
	setHeader(resp, "Content-Type", "application/activity+json")
	setHeader(resp, "Cache-Control", "max-age=300")
	return resp, nil
}

// checkCollectionAccess checks if the requester is authorized to view the collection
func (h *Handler) checkCollectionAccess(ctx *apptheory.Context, actor *activitypub.Actor, collectionType string) (*apptheory.Response, error) {
	// Only check privacy for followers collection of private accounts
	if collectionType != string(relationshipFollowers) || !actor.ManuallyApprovesFollowers {
		return nil, nil
	}

	authHeader := headerValue(ctx, "Authorization")
	if err := common.ValidateRequiredParam("authorization", authHeader); err != nil {
		return h.returnEmptyCollection(ctx, actor, collectionType)
	}

	token, err := auth.ExtractBearerToken(authHeader)
	if err != nil {
		return h.returnEmptyCollection(ctx, actor, collectionType)
	}

	oauthSvc := createOAuthService(h.cfg.JWTSecret, h.cfg, h.repos, h.logger)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil || claims.Username != actor.PreferredUsername {
		return h.returnEmptyCollection(ctx, actor, collectionType)
	}

	return nil, nil
}

// getCollectionData retrieves the collection data based on type
func (h *Handler) getCollectionData(ctx *apptheory.Context, actor *activitypub.Actor, collectionType, cursor string, limit int) ([]string, string, error) {
	switch collectionType {
	case "followers":
		return h.getFollowersData(ctx, actor.PreferredUsername, cursor, limit)
	case "following":
		return h.getFollowingData(ctx, actor.PreferredUsername, cursor, limit)
	default:
		h.logger.Error("unsupported collection type requested", zap.String("collection_type", collectionType))
		return nil, "", unsupportedCollectionType()
	}
}

// getFollowersData gets followers data and converts to usernames
func (h *Handler) getFollowersData(ctx *apptheory.Context, username, cursor string, limit int) ([]string, string, error) {
	accounts, nextCursor, err := h.registry.Relationships().GetFollowers(ctx.Context(), username, limit, cursor)
	if err != nil {
		return nil, "", err
	}

	usernames := make([]string, len(accounts))
	for i, acc := range accounts {
		if acc != nil && acc.User != nil {
			usernames[i] = acc.User.Username
		}
	}

	return usernames, nextCursor, nil
}

// getFollowingData gets following data and converts to usernames
func (h *Handler) getFollowingData(ctx *apptheory.Context, username, cursor string, limit int) ([]string, string, error) {
	accounts, nextCursor, err := h.registry.Relationships().GetFollowing(ctx.Context(), username, limit, cursor)
	if err != nil {
		return nil, "", err
	}

	usernames := make([]string, len(accounts))
	for i, acc := range accounts {
		if acc != nil && acc.User != nil {
			usernames[i] = acc.User.Username
		}
	}

	return usernames, nextCursor, nil
}

// buildCollectionPage builds the ActivityPub OrderedCollectionPage
func (h *Handler) buildCollectionPage(actor *activitypub.Actor, collectionType, cursor, nextCursor string, limit int, usernames []string) *activitypub.OrderedCollectionPage {
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

	// Add pagination links
	if nextCursor != "" {
		page.Next = fmt.Sprintf("%s?page=1&cursor=%s&limit=%d", collectionID, nextCursor, limit)
	}

	if cursor != "" {
		// For simplicity, just link back to the first page
		// In a full implementation, you'd calculate the previous cursor
		page.Prev = fmt.Sprintf("%s?page=1", collectionID)
	}

	return page
}

// returnEmptyCollection returns an empty ActivityPub collection for privacy protection.
func (h *Handler) returnEmptyCollection(_ *apptheory.Context, actor *activitypub.Actor, collectionType string) (*apptheory.Response, error) {
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

	resp, err := okJSON(collection)
	if err != nil {
		return nil, err
	}
	setHeader(resp, "Content-Type", "application/activity+json")
	setHeader(resp, "Cache-Control", "max-age=300")
	return resp, nil
}

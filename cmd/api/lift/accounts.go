package lift

import (
	"encoding/base64"
	"errors"
	"fmt"
	"mime"
	"net/mail"
	"strings"
	"time"

	"github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/services/accounts"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/transformations"
	"github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"
)

const (
	boolTrue = "true"
)

// HandleRegistrationLift handles user registration requests
func (h *Handler) HandleRegistrationLift(ctx *lift.Context) error {
	// Parse request body
	var req models.AccountRegistrationRequest
	if err := common.ParseRequestWithValidation(ctx, &req); err != nil {
		return err
	}

	// Validate request
	if err := h.validateRegistrationRequestLift(req); err != nil {
		return common.RespondUnprocessableEntity(ctx, err.Error())
	}

	// Validate password strength if provided
	if req.Password != "" {
		// Validate password strength
		if err := auth.ValidatePassword(req.Password, req.Username); err != nil {
			return common.RespondUnprocessableEntity(ctx, err.Error())
		}

		// Check password strength
		strength := auth.PasswordStrength(req.Password)
		if strength < 3 {
			hints := auth.GeneratePasswordHint(req.Password)
			return common.RespondUnprocessableEntity(ctx, fmt.Sprintf("Password is too weak (%s). Suggestions: %s",
				auth.PasswordStrengthLabel(strength),
				strings.Join(hints, ", ")))
		}
	}

	// Use Accounts service to register the account
	accountsService := h.registry.Accounts()
	result, err := accountsService.RegisterAccount(ctx.Context, &accounts.RegisterAccountCommand{
		Username:  req.Username,
		Email:     req.Email,
		Password:  req.Password,
		Locale:    req.Locale,
		Agreement: req.Agreement,
		Reason:    req.Reason,
	})
	if err != nil {
		if strings.Contains(err.Error(), "username already taken") || strings.Contains(err.Error(), "Username is already taken") {
			return common.RespondAlreadyExists(ctx, "Username")
		}
		if strings.Contains(err.Error(), "validation failed") {
			return common.RespondUnprocessableEntity(ctx, err.Error())
		}
		h.logger.Error("failed to register account", zap.Error(err))
		return common.RespondInternalServerError(ctx)
	}

	// Return response using common helper
	resp := models.AccountRegistrationResponse{
		ID:       result.Actor.ID,
		Username: result.Account.User.Username,
		Email:    result.Account.User.Email,
		Created:  true,
	}

	return h.respondCreated(ctx, resp)
}

// HandleVerifyCredentialsLift returns the current user's information
func (h *Handler) HandleVerifyCredentialsLift(ctx *lift.Context) error {
	// Authenticate user
	claims, err := h.authenticateWithScope(ctx, auth.ScopeRead)
	if err != nil {
		return err
	}

	// Call Accounts service
	account, err := h.registry.Accounts().GetAccount(ctx.Context, claims.Username)
	if err != nil {
		h.logger.Error("failed to get account", zap.Error(err))
		return common.RespondInternalServerError(ctx)
	}

	return h.respondOK(ctx, account)
}

// HandleUpdateCredentialsLift updates the current user's profile
func (h *Handler) HandleUpdateCredentialsLift(ctx *lift.Context) error {
	// Parse request
	var req models.UpdateCredentialsRequest
	if err := ctx.ParseRequest(&req); err != nil {
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
		return err
	}

	// Call Accounts service
	result, err := h.registry.Accounts().UpdateProfile(ctx.Context, &accounts.UpdateProfileCommand{
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
func (h *Handler) HandleGetAccountLift(ctx *lift.Context) error {
	accountID := ctx.Param("id")
	if err := common.ValidateAccountParamID(accountID); err != nil {
		return common.RespondValidationError(ctx, err)
	}

	// Call Accounts service
	account, err := h.registry.Accounts().GetAccount(ctx.Context, accountID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return common.RespondAccountNotFound(ctx)
		}
		h.logger.Error("failed to get account", zap.Error(err))
		return common.RespondInternalServerError(ctx)
	}

	return ctx.JSON(account)
}

// HandleAccountLookupLift looks up an account by username@domain
func (h *Handler) HandleAccountLookupLift(ctx *lift.Context) error {
	// Get acct parameter
	acct := ctx.Query("acct")
	if err := common.ValidateAcctParameter(acct); err != nil {
		return common.RespondValidationError(ctx, err)
	}

	// Call Accounts service
	account, err := h.registry.Accounts().LookupAccount(ctx.Context, &accounts.LookupAccountQuery{
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

	return ctx.JSON(mastodonAccount)
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
	if err := common.ValidateAccountParamID(accountID); err != nil {
		return common.RespondValidationError(ctx, err)
	}

	// Extract username from accountID
	username := accountID

	// Get the actor to verify it exists
	_, err := h.registry.Accounts().GetAccount(ctx.Context, username)
	if err != nil {
		return common.RespondAccountNotFound(ctx)
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
	var relationshipAccounts []*storage.Account
	var nextCursor string

	switch relType {
	case relationshipFollowers:
		relationshipAccounts, nextCursor, err = h.registry.Relationships().GetFollowers(ctx.Context, username, limit, cursor)
		if err != nil {
			h.logger.Error("failed to get followers", zap.Error(err))
			return common.RespondInternalServerError(ctx)
		}
	case relationshipFollowing:
		relationshipAccounts, nextCursor, err = h.registry.Relationships().GetFollowing(ctx.Context, username, limit, cursor)
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
		return common.RespondUnauthorized(ctx)
	}

	// Validate token and get claims
	oauthSvc := createOAuthService(h.cfg.JWTSecret, h.cfg, h.repos, h.logger)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return common.RespondUnauthorized(ctx)
	}

	// Get account IDs from query parameter
	accountIDs := ctx.Query("id[]")
	ids, err := common.ValidateAccountIDsParameter(accountIDs)
	if err != nil {
		return common.RespondValidationError(ctx, err)
	}

	// Use Accounts service to get familiar followers
	result, err := h.registry.Accounts().GetFamiliarFollowers(ctx.Context, &accounts.GetFamiliarFollowersQuery{
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

	return ctx.JSON(results)
}

// HandlePinAccountLift pins an account to the user's profile
func (h *Handler) HandlePinAccountLift(ctx *lift.Context) error {
	accountID := ctx.Param("id")
	if err := common.ValidateAccountParamID(accountID); err != nil {
		return common.RespondValidationError(ctx, err)
	}

	// Authenticate user
	claims, err := h.authenticateWithScope(ctx, auth.ScopeWrite)
	if err != nil {
		return err
	}

	// Call Accounts service
	result, err := h.registry.Accounts().PinAccount(ctx.Context, &accounts.PinAccountCommand{
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

	return ctx.JSON(result.Relationship)
}

// HandleUnpinAccountLift unpins an account from the user's profile
func (h *Handler) HandleUnpinAccountLift(ctx *lift.Context) error {
	accountID := ctx.Param("id")
	if err := common.ValidateAccountParamID(accountID); err != nil {
		return common.RespondValidationError(ctx, err)
	}

	// Authenticate user
	claims, err := h.authenticateWithScope(ctx, auth.ScopeWrite)
	if err != nil {
		return err
	}

	// Call Accounts service
	result, err := h.registry.Accounts().UnpinAccount(ctx.Context, &accounts.UnpinAccountCommand{
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

	return ctx.JSON(result.Relationship)
}

// HandleSetAccountNoteLift sets a private note on an account
func (h *Handler) HandleSetAccountNoteLift(ctx *lift.Context) error {
	accountID := ctx.Param("id")
	if err := common.ValidateAccountParamID(accountID); err != nil {
		return common.RespondValidationError(ctx, err)
	}

	// Authenticate user
	claims, err := h.authenticateWithScope(ctx, auth.ScopeWrite)
	if err != nil {
		return err
	}

	// Parse request body
	var req struct {
		Comment string `json:"comment"`
	}
	if err := ctx.ParseRequest(&req); err != nil {
		return common.RespondValidationError(ctx, err)
	}

	// Call Accounts service
	result, err := h.registry.Accounts().SetAccountNote(ctx.Context, &accounts.SetAccountNoteCommand{
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

	return ctx.JSON(result.Relationship)
}

// HandleRemoveFromFollowersLift removes a follower from the current user's followers list
func (h *Handler) HandleRemoveFromFollowersLift(ctx *lift.Context) error {
	accountID := ctx.Param("id")
	if err := common.ValidateAccountParamID(accountID); err != nil {
		return common.RespondValidationError(ctx, err)
	}

	// Authenticate user
	claims, err := h.authenticateWithScope(ctx, auth.ScopeWrite)
	if err != nil {
		return err
	}

	// Call Accounts service
	result, err := h.registry.Accounts().RemoveFollower(ctx.Context, &accounts.RemoveFollowerCommand{
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

	return ctx.JSON(result.Relationship)
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
	if err := common.ValidateUsernameParamID(username); err != nil {
		return common.RespondValidationError(ctx, err)
	}

	// Check if actor exists
	actor, err := h.registry.Accounts().GetAccount(ctx.Context, username)
	if err != nil {
		return common.RespondUserNotFound(ctx)
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
	limit, err := common.ParseAndValidateActivityPubLimit(ctx.Query("limit"))
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
func (h *Handler) returnActivityPubCollection(ctx *lift.Context, actor *activitypub.Actor, collectionType string) error {
	// Check privacy settings for followers collection
	if collectionType == string(relationshipFollowers) && actor.ManuallyApprovesFollowers {
		// Check if the requester is authorized to view followers
		authHeader := ctx.Header("Authorization")
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
		count, countErr := h.registry.Relationships().CountFollowers(ctx.Context, actor.PreferredUsername)
		totalItems, err = int(count), countErr
	case "following":
		count, countErr := h.registry.Relationships().CountFollowing(ctx.Context, actor.PreferredUsername)
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
	ctx.Response.Header("Content-Type", "application/activity+json")
	ctx.Response.Header("Cache-Control", "max-age=300")
	return ctx.JSON(collection)
}

// returnActivityPubCollectionPage returns ActivityPub OrderedCollectionPage with items
func (h *Handler) returnActivityPubCollectionPage(ctx *lift.Context, actor *activitypub.Actor, collectionType, cursor string, limit int) error {
	// Check privacy settings and authorization
	if err := h.checkCollectionAccess(ctx, actor, collectionType); err != nil {
		return err
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
	ctx.Response.Header("Content-Type", "application/activity+json")
	ctx.Response.Header("Cache-Control", "max-age=300")
	return ctx.JSON(page)
}

// checkCollectionAccess checks if the requester is authorized to view the collection
func (h *Handler) checkCollectionAccess(ctx *lift.Context, actor *activitypub.Actor, collectionType string) error {
	// Only check privacy for followers collection of private accounts
	if collectionType != string(relationshipFollowers) || !actor.ManuallyApprovesFollowers {
		return nil
	}

	authHeader := ctx.Header("Authorization")
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

	return nil
}

// getCollectionData retrieves the collection data based on type
func (h *Handler) getCollectionData(ctx *lift.Context, actor *activitypub.Actor, collectionType, cursor string, limit int) ([]string, string, error) {
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
func (h *Handler) getFollowersData(ctx *lift.Context, username, cursor string, limit int) ([]string, string, error) {
	accounts, nextCursor, err := h.registry.Relationships().GetFollowers(ctx.Context, username, limit, cursor)
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
func (h *Handler) getFollowingData(ctx *lift.Context, username, cursor string, limit int) ([]string, string, error) {
	accounts, nextCursor, err := h.registry.Relationships().GetFollowing(ctx.Context, username, limit, cursor)
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

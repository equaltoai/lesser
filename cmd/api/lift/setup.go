package lift

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	appErrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/services/accounts"
	storageModels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"
)

const (
	setupSessionPurposeBootstrap = "bootstrap"
)

func instanceStateString(locked bool) string {
	if locked {
		return "locked"
	}
	return "active"
}

func (h *Handler) stageURLs() map[string]any {
	domain := strings.TrimSpace(h.cfg.Domain)

	httpScheme := "https"
	wsScheme := "wss"
	if domain == "localhost" || domain == "127.0.0.1" {
		httpScheme = "http"
		wsScheme = "ws"
	}

	baseURL := h.cfg.BaseURL()
	setupURL := fmt.Sprintf("%s/auth/setup", baseURL)

	return map[string]any{
		// Single-domain path routing (preferred).
		"client":       fmt.Sprintf("%s/l", baseURL),
		"api":          baseURL,
		"auth":         fmt.Sprintf("%s/auth", baseURL),
		"setup":        setupURL,
		"setup_status": fmt.Sprintf("%s/setup/status", baseURL),
		"ws":           fmt.Sprintf("%s://ws.%s", wsScheme, domain),
		"media":        fmt.Sprintf("%s://media.%s", httpScheme, domain),

		// Backwards-compatible field name.
		"auth_setup": setupURL,
	}
}

// HandleSetupStatusLift handles GET /setup/status.
func (h *Handler) HandleSetupStatusLift(ctx *lift.Context) error {
	state, err := h.repos.Instance().GetInstanceState(ctx.Context)
	if err != nil {
		h.logger.Error("failed to get instance state", zap.Error(err))
		return h.respondInternalError(ctx, "failed to get instance state")
	}

	bootstrapUsername := strings.TrimSpace(state.BootstrapUsername)
	if bootstrapUsername == "" {
		bootstrapUsername = storageModels.DefaultBootstrapUsername
	}

	return h.respondOK(ctx, map[string]any{
		"instance_state":   instanceStateString(state.Locked),
		"locked":           state.Locked,
		"finalize_allowed": state.Locked && strings.TrimSpace(state.PrimaryAdminUsername) != "",
		"bootstrap_actor": map[string]any{
			"username": bootstrapUsername,
			"acct":     fmt.Sprintf("%s@%s", bootstrapUsername, strings.TrimSpace(h.cfg.Domain)),
			"actor":    h.cfg.ActorURL(bootstrapUsername),
		},
		"urls": h.stageURLs(),
		"bootstrap": map[string]any{
			"username":             bootstrapUsername,
			"wallet_address_set":   strings.TrimSpace(state.BootstrapWalletAddress) != "",
			"wallet_address":       state.BootstrapWalletAddress,
			"primary_admin_set":    strings.TrimSpace(state.PrimaryAdminUsername) != "",
			"primary_admin":        state.PrimaryAdminUsername,
			"setup_session_scheme": "bearer",
		},
		"activated_at": state.ActivatedAt,
	})
}

// HandleSetupBootstrapChallengeLift handles POST /setup/bootstrap/challenge.
func (h *Handler) HandleSetupBootstrapChallengeLift(ctx *lift.Context) error {
	state, err := h.repos.Instance().GetInstanceState(ctx.Context)
	if err != nil {
		h.logger.Error("failed to get instance state", zap.Error(err))
		return h.respondInternalError(ctx, "failed to get instance state")
	}
	if !state.Locked {
		return h.respondConflict(ctx, "instance is already activated")
	}

	var req struct {
		Address string `json:"address"`
		ChainID int    `json:"chainId"`
	}
	if err := h.parseRequestBody(ctx, &req); err != nil {
		return err
	}

	if err := common.ValidateRequiredParam("address", req.Address); err != nil {
		return h.respondBadRequest(ctx, "address is required")
	}
	if req.ChainID == 0 {
		req.ChainID = 1
	}

	bootstrapAddr := strings.ToLower(strings.TrimSpace(state.BootstrapWalletAddress))
	if bootstrapAddr == "" {
		return h.respondConflict(ctx, "bootstrap wallet is not configured for this instance")
	}
	if strings.ToLower(strings.TrimSpace(req.Address)) != bootstrapAddr {
		return h.respondForbidden(ctx, "wallet does not match bootstrap credential")
	}

	bootstrapUsername := strings.TrimSpace(state.BootstrapUsername)
	if bootstrapUsername == "" {
		bootstrapUsername = storageModels.DefaultBootstrapUsername
	}

	authService, err := h.requireAuthService(ctx)
	if err != nil {
		return err
	}

	challenge, err := authService.CreateWalletChallenge(ctx.Context, bootstrapAddr, req.ChainID, bootstrapUsername)
	if err != nil {
		return h.handleAuthServiceError(ctx, err, "create bootstrap challenge")
	}

	return h.respondOK(ctx, map[string]any{
		"challenge_id": challenge.ID,
		"challenge":    challenge.Message,
		"issued_at":    challenge.IssuedAt,
		"expires_at":   challenge.ExpiresAt,

		// Backwards-compatible fields.
		"id":        challenge.ID,
		"username":  challenge.Username,
		"address":   challenge.Address,
		"chainId":   challenge.ChainID,
		"nonce":     challenge.Nonce,
		"message":   challenge.Message,
		"issuedAt":  challenge.IssuedAt,
		"expiresAt": challenge.ExpiresAt,
	})
}

// HandleSetupBootstrapVerifyLift handles POST /setup/bootstrap/verify.
func (h *Handler) HandleSetupBootstrapVerifyLift(ctx *lift.Context) error {
	state, err := h.repos.Instance().GetInstanceState(ctx.Context)
	if err != nil {
		h.logger.Error("failed to get instance state", zap.Error(err))
		return h.respondInternalError(ctx, "failed to get instance state")
	}
	if !state.Locked {
		return h.respondConflict(ctx, "instance is already activated")
	}

	var raw struct {
		ChallengeID      string `json:"challengeId"`
		ChallengeIDSnake string `json:"challenge_id"`
		Address          string `json:"address"`
		Signature        string `json:"signature"`
		Message          string `json:"message"`
		Challenge        string `json:"challenge"`
	}
	if err := h.parseRequestBody(ctx, &raw); err != nil {
		return err
	}

	challengeID := strings.TrimSpace(raw.ChallengeID)
	if challengeID == "" {
		challengeID = strings.TrimSpace(raw.ChallengeIDSnake)
	}

	message := strings.TrimSpace(raw.Message)
	if message == "" {
		message = strings.TrimSpace(raw.Challenge)
	}

	req := auth.WalletVerifyRequest{
		ChallengeID: challengeID,
		Address:     raw.Address,
		Signature:   raw.Signature,
		Message:     message,
	}

	if err := common.ValidateRequiredParam("challengeId", req.ChallengeID); err != nil {
		return h.respondBadRequest(ctx, "challengeId is required")
	}
	if err := common.ValidateRequiredParam("address", req.Address); err != nil {
		return h.respondBadRequest(ctx, "address is required")
	}
	if err := common.ValidateRequiredParam("signature", req.Signature); err != nil {
		return h.respondBadRequest(ctx, "signature is required")
	}
	if err := common.ValidateRequiredParam("message", req.Message); err != nil {
		return h.respondBadRequest(ctx, "message is required")
	}

	bootstrapAddr := strings.ToLower(strings.TrimSpace(state.BootstrapWalletAddress))
	if bootstrapAddr == "" {
		return h.respondConflict(ctx, "bootstrap wallet is not configured for this instance")
	}
	if strings.ToLower(strings.TrimSpace(req.Address)) != bootstrapAddr {
		return h.respondForbidden(ctx, "wallet does not match bootstrap credential")
	}

	bootstrapUsername := strings.TrimSpace(state.BootstrapUsername)
	if bootstrapUsername == "" {
		bootstrapUsername = storageModels.DefaultBootstrapUsername
	}

	authService, err := h.requireAuthService(ctx)
	if err != nil {
		return err
	}

	challenge, err := authService.GetWalletChallenge(ctx.Context, req.ChallengeID)
	if err != nil {
		return h.respondUnauthorized(ctx)
	}
	if challenge.Username != bootstrapUsername {
		return h.respondForbidden(ctx, "challenge is not bound to bootstrap identity")
	}

	if err := authService.VerifyWalletSignature(ctx.Context, &req); err != nil {
		return h.handleAuthServiceError(ctx, err, "verify bootstrap signature")
	}
	_ = authService.MarkWalletChallengeSpent(ctx.Context, req.ChallengeID)

	token, err := generateSetupToken()
	if err != nil {
		h.logger.Error("failed to generate setup token", zap.Error(err))
		return h.respondInternalError(ctx, "failed to create setup session")
	}

	now := time.Now().UTC()
	expiresAt := now.Add(1 * time.Hour)

	setupSession := &storageModels.SetupSession{
		ID:           token,
		Purpose:      setupSessionPurposeBootstrap,
		WalletType:   "ethereum",
		WalletAddr:   bootstrapAddr,
		IssuedAt:     now,
		ExpiresAt:    expiresAt,
		InstanceLock: true,
	}

	repo := repositories.NewBaseRepository[*storageModels.SetupSession](h.repos.GetDB(), h.repos.GetTableName(), h.logger)
	if err := repo.Create(ctx.Context, setupSession); err != nil {
		h.logger.Error("failed to store setup session", zap.Error(err))
		return h.respondInternalError(ctx, "failed to create setup session")
	}

	return h.respondOK(ctx, map[string]any{
		"token_type":  "Bearer",
		"token":       token,
		"setup_token": token,
		"expires_at":  expiresAt,
	})
}

// HandleSetupCreateAdminLift handles POST /setup/admin.
func (h *Handler) HandleSetupCreateAdminLift(ctx *lift.Context) error {
	state, err := h.repos.Instance().GetInstanceState(ctx.Context)
	if err != nil {
		h.logger.Error("failed to get instance state", zap.Error(err))
		return h.respondInternalError(ctx, "failed to get instance state")
	}
	if !state.Locked {
		return h.respondConflict(ctx, "instance is already activated")
	}
	if strings.TrimSpace(state.PrimaryAdminUsername) != "" {
		return h.respondConflict(ctx, "primary admin already created")
	}

	bootstrapUsername := strings.TrimSpace(state.BootstrapUsername)
	if bootstrapUsername == "" {
		bootstrapUsername = storageModels.DefaultBootstrapUsername
	}
	if _, err := h.requireSetupSession(ctx, bootstrapUsername, state); err != nil {
		return err
	}

	req, err := h.parseSetupCreateAdminRequest(ctx, bootstrapUsername)
	if err != nil {
		return err
	}

	authService, err := h.requireAuthService(ctx)
	if err != nil {
		return err
	}

	walletChainID, adminWalletAddr, err := h.verifySetupCreateAdminWallet(ctx, authService, req.Username, req.Wallet)
	if err != nil {
		return err
	}

	actorID, err := h.ensureSetupAdminAccount(ctx, req.Username)
	if err != nil {
		return err
	}

	updates := map[string]any{
		"role": roleAdmin,
	}
	if strings.TrimSpace(req.DisplayName) != "" {
		updates["display_name"] = strings.TrimSpace(req.DisplayName)
	}
	if err := h.repos.User().UpdateUser(ctx.Context, req.Username, updates); err != nil {
		h.logger.Error("failed to promote admin user", zap.String("username", req.Username), zap.Error(err))
		return h.respondInternalError(ctx, "failed to configure admin account")
	}

	if err := authService.LinkWallet(ctx.Context, req.Username, adminWalletAddr, walletChainID, "ethereum"); err != nil {
		h.logger.Error("failed to link admin wallet", zap.String("username", req.Username), zap.Error(err))
		return h.respondInternalError(ctx, "failed to link wallet")
	}

	if err := h.repos.Instance().SetPrimaryAdminUsername(ctx.Context, req.Username); err != nil {
		h.logger.Error("failed to record primary admin", zap.String("username", req.Username), zap.Error(err))
		return h.respondInternalError(ctx, "failed to update setup state")
	}

	return h.respondCreated(ctx, map[string]any{
		"username": req.Username,
		"actor":    actorID,
	})
}

type setupCreateAdminRequest struct {
	Username    string                   `json:"username"`
	DisplayName string                   `json:"displayName,omitempty"`
	Wallet      auth.WalletVerifyRequest `json:"wallet"`
}

func (h *Handler) parseSetupCreateAdminRequest(ctx *lift.Context, bootstrapUsername string) (setupCreateAdminRequest, error) {
	var req setupCreateAdminRequest
	if err := h.parseRequestBody(ctx, &req); err != nil {
		return setupCreateAdminRequest{}, err
	}

	req.Username = strings.TrimSpace(req.Username)
	if err := common.ValidateRequiredParam("username", req.Username); err != nil {
		return setupCreateAdminRequest{}, h.respondBadRequest(ctx, "username is required")
	}
	if strings.EqualFold(req.Username, bootstrapUsername) {
		return setupCreateAdminRequest{}, h.respondBadRequest(ctx, "username is reserved")
	}
	return req, nil
}

func (h *Handler) verifySetupCreateAdminWallet(
	ctx *lift.Context,
	authService *auth.AuthService,
	username string,
	wallet auth.WalletVerifyRequest,
) (int, string, error) {
	// Verify the new admin wallet signature (binds wallet to chosen username).
	if err := common.ValidateRequiredParam("challengeId", wallet.ChallengeID); err != nil {
		return 0, "", h.respondBadRequest(ctx, "wallet.challengeId is required")
	}
	if err := common.ValidateRequiredParam("address", wallet.Address); err != nil {
		return 0, "", h.respondBadRequest(ctx, "wallet.address is required")
	}
	if err := common.ValidateRequiredParam("signature", wallet.Signature); err != nil {
		return 0, "", h.respondBadRequest(ctx, "wallet.signature is required")
	}
	if err := common.ValidateRequiredParam("message", wallet.Message); err != nil {
		return 0, "", h.respondBadRequest(ctx, "wallet.message is required")
	}

	challenge, err := authService.GetWalletChallenge(ctx.Context, wallet.ChallengeID)
	if err != nil {
		return 0, "", h.respondUnauthorized(ctx)
	}
	if challenge.Username != username {
		return 0, "", h.respondForbidden(ctx, "wallet challenge username mismatch")
	}

	adminWalletAddr := strings.ToLower(strings.TrimSpace(wallet.Address))
	existingUsers, err := h.repos.Account().GetAllUsersForWallet(ctx.Context, "ethereum", adminWalletAddr)
	if err != nil {
		h.logger.Error("failed to check wallet index", zap.Error(err))
		return 0, "", h.respondInternalError(ctx, "failed to validate wallet")
	}
	if len(existingUsers) > 0 {
		return 0, "", h.respondConflict(ctx, "wallet is already linked to a user")
	}

	if err := authService.VerifyWalletSignature(ctx.Context, &wallet); err != nil {
		return 0, "", h.handleAuthServiceError(ctx, err, "verify admin wallet signature")
	}
	_ = authService.MarkWalletChallengeSpent(ctx.Context, wallet.ChallengeID)

	return challenge.ChainID, adminWalletAddr, nil
}

func (h *Handler) ensureSetupAdminAccount(ctx *lift.Context, username string) (string, error) {
	if h.registry == nil {
		h.logger.Error("service registry not initialized")
		return "", h.respondInternalError(ctx, "internal server error")
	}

	result, err := h.registry.Accounts().RegisterAccount(ctx.Context, &accounts.RegisterAccountCommand{
		Username:                 username,
		Email:                    "",
		Password:                 "",
		Locale:                   "",
		Agreement:                true,
		Reason:                   "",
		InviteCode:               "",
		DefaultPostingVisibility: "",
	})
	if err != nil {
		if !errors.Is(err, accounts.ErrUsernameAlreadyTaken) {
			h.logger.Error("failed to create admin account", zap.String("username", username), zap.Error(err))
			return "", h.respondUnprocessableEntity(ctx, "failed to create admin account")
		}

		existingActor, actorErr := h.repos.Actor().GetActor(ctx.Context, username)
		if actorErr != nil {
			h.logger.Error("admin username exists but actor lookup failed", zap.String("username", username), zap.Error(actorErr))
			return "", h.respondUnprocessableEntity(ctx, "failed to create admin account")
		}
		return existingActor.ID, nil
	}

	if result != nil && result.Actor != nil && strings.TrimSpace(result.Actor.ID) != "" {
		return result.Actor.ID, nil
	}
	return h.cfg.ActorURL(username), nil
}

// HandleSetupFinalizeLift handles POST /setup/finalize.
func (h *Handler) HandleSetupFinalizeLift(ctx *lift.Context) error {
	state, err := h.repos.Instance().GetInstanceState(ctx.Context)
	if err != nil {
		h.logger.Error("failed to get instance state", zap.Error(err))
		return h.respondInternalError(ctx, "failed to get instance state")
	}
	if !state.Locked {
		return h.respondConflict(ctx, "instance is already activated")
	}

	claims, err := h.authenticateWithScope(ctx, auth.ScopeWrite)
	if err != nil {
		return err
	}

	user, err := h.repos.User().GetUser(ctx.Context, claims.Username)
	if err != nil {
		h.logger.Error("failed to get user for finalize", zap.String("username", claims.Username), zap.Error(err))
		return h.respondForbidden(ctx, "admin required")
	}
	if user.Role != roleAdmin {
		return h.respondForbidden(ctx, "admin required")
	}

	if strings.TrimSpace(state.PrimaryAdminUsername) != "" && state.PrimaryAdminUsername != claims.Username {
		return h.respondForbidden(ctx, "only the primary admin can finalize activation")
	}

	if err := h.repos.Instance().SetInstanceLocked(ctx.Context, false); err != nil {
		h.logger.Error("failed to unlock instance", zap.Error(err))
		return h.respondInternalError(ctx, "failed to activate instance")
	}

	bootstrapUsername := strings.TrimSpace(state.BootstrapUsername)
	if bootstrapUsername == "" {
		bootstrapUsername = storageModels.DefaultBootstrapUsername
	}

	if err := h.repos.Actor().DeleteActor(ctx.Context, bootstrapUsername); err != nil && !common.IsNotFound(err) {
		h.logger.Warn("failed to delete bootstrap actor", zap.String("username", bootstrapUsername), zap.Error(err))
	}
	if err := h.repos.User().DeleteUser(ctx.Context, bootstrapUsername); err != nil && !common.IsNotFound(err) {
		h.logger.Warn("failed to delete bootstrap user", zap.String("username", bootstrapUsername), zap.Error(err))
	}

	_ = h.repos.Instance().SetBootstrapWalletAddress(ctx.Context, "")

	updatedState, updatedErr := h.repos.Instance().GetInstanceState(ctx.Context)
	var activatedAt *time.Time
	if updatedErr == nil {
		activatedAt = updatedState.ActivatedAt
	}

	return h.respondOK(ctx, map[string]any{
		"instance_state": instanceStateString(false),
		"locked":         false,
		"activated_at":   activatedAt,
		"urls":           h.stageURLs(),
	})
}

func (h *Handler) requireSetupSession(ctx *lift.Context, bootstrapUsername string, state *storageModels.InstanceState) (*storageModels.SetupSession, error) {
	authHeader := ctx.Header("Authorization")
	if err := common.ValidateRequiredParam("authHeader", authHeader); err != nil {
		authHeader = ctx.Header("authorization")
	}

	token, err := auth.ExtractBearerToken(authHeader)
	if err != nil || strings.TrimSpace(token) == "" {
		return nil, h.respondUnauthorized(ctx)
	}

	repo := repositories.NewBaseRepository[*storageModels.SetupSession](h.repos.GetDB(), h.repos.GetTableName(), h.logger)
	session := &storageModels.SetupSession{}
	err = repo.Get(ctx.Context, "SETUP_SESSION#"+token, "SESSION", session)
	if err != nil {
		if appErrors.HasCode(err, appErrors.CodeNotFound) {
			return nil, h.respondUnauthorized(ctx)
		}
		h.logger.Error("failed to get setup session", zap.Error(err))
		return nil, h.respondInternalError(ctx, "failed to validate setup session")
	}

	if time.Now().After(session.ExpiresAt) {
		_ = repo.Delete(ctx.Context, session.PK, session.SK)
		return nil, h.respondUnauthorized(ctx)
	}
	if session.Purpose != setupSessionPurposeBootstrap {
		return nil, h.respondUnauthorized(ctx)
	}

	bootstrapAddr := strings.ToLower(strings.TrimSpace(state.BootstrapWalletAddress))
	if bootstrapAddr == "" || strings.ToLower(strings.TrimSpace(session.WalletAddr)) != bootstrapAddr {
		return nil, h.respondUnauthorized(ctx)
	}

	if !strings.EqualFold(bootstrapUsername, state.BootstrapUsername) {
		return nil, h.respondUnauthorized(ctx)
	}

	return session, nil
}

func generateSetupToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

package lift

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
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

func (h *Handler) stageURLs() apimodels.SetupStageURLs {
	domain := strings.TrimSpace(h.cfg.Domain)

	httpScheme := "https"
	wsScheme := "wss"
	if domain == "localhost" || domain == "127.0.0.1" {
		httpScheme = "http"
		wsScheme = "ws"
	}

	baseURL := h.cfg.BaseURL()
	setupURL := fmt.Sprintf("%s/auth/setup", baseURL)

	return apimodels.SetupStageURLs{
		Client:      fmt.Sprintf("%s/l", baseURL),
		API:         baseURL,
		Auth:        fmt.Sprintf("%s/auth", baseURL),
		Setup:       setupURL,
		SetupStatus: fmt.Sprintf("%s/setup/status", baseURL),
		WS:          fmt.Sprintf("%s://ws.%s", wsScheme, domain),
		Media:       fmt.Sprintf("%s://media.%s", httpScheme, domain),
		AuthSetup:   setupURL,
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

	return h.respondOK(ctx, apimodels.SetupStatusResponse{
		InstanceState:   instanceStateString(state.Locked),
		Locked:          state.Locked,
		FinalizeAllowed: state.Locked && strings.TrimSpace(state.PrimaryAdminUsername) != "",
		BootstrapActor: apimodels.SetupBootstrapActor{
			Username: bootstrapUsername,
			Acct:     fmt.Sprintf("%s@%s", bootstrapUsername, strings.TrimSpace(h.cfg.Domain)),
			Actor:    h.cfg.ActorURL(bootstrapUsername),
		},
		URLs: h.stageURLs(),
		Bootstrap: apimodels.SetupBootstrapState{
			Username:           bootstrapUsername,
			WalletAddressSet:   strings.TrimSpace(state.BootstrapWalletAddress) != "",
			WalletAddress:      state.BootstrapWalletAddress,
			PrimaryAdminSet:    strings.TrimSpace(state.PrimaryAdminUsername) != "",
			PrimaryAdmin:       state.PrimaryAdminUsername,
			SetupSessionScheme: "bearer",
		},
		ActivatedAt: state.ActivatedAt,
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

	var req apimodels.SetupBootstrapChallengeRequest
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

	return h.respondOK(ctx, apimodels.SetupBootstrapChallengeResponse{
		ChallengeID: challenge.ID,
		Challenge:   challenge.Message,
		IssuedAt:    challenge.IssuedAt,
		ExpiresAt:   challenge.ExpiresAt,

		ID:              challenge.ID,
		Username:        challenge.Username,
		Address:         challenge.Address,
		ChainID:         challenge.ChainID,
		Nonce:           challenge.Nonce,
		Message:         challenge.Message,
		IssuedAtLegacy:  challenge.IssuedAt,
		ExpiresAtLegacy: challenge.ExpiresAt,
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

	var raw apimodels.SetupBootstrapVerifyRequest
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

	return h.respondOK(ctx, apimodels.SetupBootstrapVerifyResponse{
		TokenType:  "Bearer",
		Token:      token,
		SetupToken: token,
		ExpiresAt:  expiresAt,
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
	if _, handled, err := h.requireSetupSession(ctx, bootstrapUsername, state); err != nil {
		return err
	} else if handled {
		return nil
	}

	req, handled, err := h.parseSetupCreateAdminRequest(ctx, bootstrapUsername)
	if err != nil {
		return err
	} else if handled {
		return nil
	}

	authService, err := h.requireAuthService(ctx)
	if err != nil {
		return err
	}

	walletChainID, adminWalletAddr, handled, err := h.verifySetupCreateAdminWallet(ctx, authService, req.Username, req.Wallet)
	if err != nil {
		return err
	} else if handled {
		return nil
	}

	actorID, handled, err := h.ensureSetupAdminAccount(ctx, req.Username)
	if err != nil {
		return err
	} else if handled {
		return nil
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

	return h.respondCreated(ctx, apimodels.SetupCreateAdminResponse{
		Username: req.Username,
		Actor:    actorID,
	})
}

func (h *Handler) parseSetupCreateAdminRequest(ctx *lift.Context, bootstrapUsername string) (apimodels.SetupCreateAdminRequest, bool, error) {
	var req apimodels.SetupCreateAdminRequest
	if err := h.parseRequestBody(ctx, &req); err != nil {
		return apimodels.SetupCreateAdminRequest{}, false, err
	}

	req.Username = strings.TrimSpace(req.Username)
	if err := common.ValidateRequiredParam("username", req.Username); err != nil {
		_ = h.respondBadRequest(ctx, "username is required")
		return apimodels.SetupCreateAdminRequest{}, true, nil
	}
	if strings.EqualFold(req.Username, bootstrapUsername) {
		_ = h.respondBadRequest(ctx, "username is reserved")
		return apimodels.SetupCreateAdminRequest{}, true, nil
	}
	return req, false, nil
}

func (h *Handler) verifySetupCreateAdminWallet(
	ctx *lift.Context,
	authService *auth.AuthService,
	username string,
	wallet auth.WalletVerifyRequest,
) (int, string, bool, error) {
	// Verify the new admin wallet signature (binds wallet to chosen username).
	if err := common.ValidateRequiredParam("challengeId", wallet.ChallengeID); err != nil {
		_ = h.respondBadRequest(ctx, "wallet.challengeId is required")
		return 0, "", true, nil
	}
	if err := common.ValidateRequiredParam("address", wallet.Address); err != nil {
		_ = h.respondBadRequest(ctx, "wallet.address is required")
		return 0, "", true, nil
	}
	if err := common.ValidateRequiredParam("signature", wallet.Signature); err != nil {
		_ = h.respondBadRequest(ctx, "wallet.signature is required")
		return 0, "", true, nil
	}
	if err := common.ValidateRequiredParam("message", wallet.Message); err != nil {
		_ = h.respondBadRequest(ctx, "wallet.message is required")
		return 0, "", true, nil
	}

	challenge, err := authService.GetWalletChallenge(ctx.Context, wallet.ChallengeID)
	if err != nil {
		_ = h.respondUnauthorized(ctx)
		return 0, "", true, nil
	}
	if challenge.Username != username {
		_ = h.respondForbidden(ctx, "wallet challenge username mismatch")
		return 0, "", true, nil
	}

	adminWalletAddr := strings.ToLower(strings.TrimSpace(wallet.Address))
	existingUsers, err := h.repos.Account().GetAllUsersForWallet(ctx.Context, "ethereum", adminWalletAddr)
	if err != nil {
		h.logger.Error("failed to check wallet index", zap.Error(err))
		_ = h.respondInternalError(ctx, "failed to validate wallet")
		return 0, "", true, nil
	}
	if len(existingUsers) > 0 {
		_ = h.respondConflict(ctx, "wallet is already linked to a user")
		return 0, "", true, nil
	}

	if err := authService.VerifyWalletSignature(ctx.Context, &wallet); err != nil {
		if respErr := h.handleAuthServiceError(ctx, err, "verify admin wallet signature"); respErr != nil {
			return 0, "", false, respErr
		}
		return 0, "", true, nil
	}
	_ = authService.MarkWalletChallengeSpent(ctx.Context, wallet.ChallengeID)

	return challenge.ChainID, adminWalletAddr, false, nil
}

func (h *Handler) ensureSetupAdminAccount(ctx *lift.Context, username string) (string, bool, error) {
	if h.registry == nil {
		h.logger.Error("service registry not initialized")
		_ = h.respondInternalError(ctx, "internal server error")
		return "", true, nil
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
			_ = h.respondUnprocessableEntity(ctx, "failed to create admin account")
			return "", true, nil
		}

		existingActor, actorErr := h.repos.Actor().GetActor(ctx.Context, username)
		if actorErr != nil {
			h.logger.Error("admin username exists but actor lookup failed", zap.String("username", username), zap.Error(actorErr))
			_ = h.respondUnprocessableEntity(ctx, "failed to create admin account")
			return "", true, nil
		}
		return existingActor.ID, false, nil
	}

	if result != nil && result.Actor != nil && strings.TrimSpace(result.Actor.ID) != "" {
		return result.Actor.ID, false, nil
	}
	return h.cfg.ActorURL(username), false, nil
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

	return h.respondOK(ctx, apimodels.SetupFinalizeResponse{
		InstanceState: instanceStateString(false),
		Locked:        false,
		ActivatedAt:   activatedAt,
		URLs:          h.stageURLs(),
	})
}

func (h *Handler) requireSetupSession(ctx *lift.Context, bootstrapUsername string, state *storageModels.InstanceState) (*storageModels.SetupSession, bool, error) {
	authHeader := ctx.Header("Authorization")
	if err := common.ValidateRequiredParam("authHeader", authHeader); err != nil {
		authHeader = ctx.Header("authorization")
	}

	token, err := auth.ExtractBearerToken(authHeader)
	if err != nil || strings.TrimSpace(token) == "" {
		_ = h.respondUnauthorized(ctx)
		return nil, true, nil
	}

	repo := repositories.NewBaseRepository[*storageModels.SetupSession](h.repos.GetDB(), h.repos.GetTableName(), h.logger)
	session := &storageModels.SetupSession{}
	err = repo.Get(ctx.Context, "SETUP_SESSION#"+token, "SESSION", session)
	if err != nil {
		if appErrors.HasCode(err, appErrors.CodeNotFound) {
			_ = h.respondUnauthorized(ctx)
			return nil, true, nil
		}
		h.logger.Error("failed to get setup session", zap.Error(err))
		_ = h.respondInternalError(ctx, "failed to validate setup session")
		return nil, true, nil
	}

	if time.Now().After(session.ExpiresAt) {
		_ = repo.Delete(ctx.Context, session.PK, session.SK)
		_ = h.respondUnauthorized(ctx)
		return nil, true, nil
	}
	if session.Purpose != setupSessionPurposeBootstrap {
		_ = h.respondUnauthorized(ctx)
		return nil, true, nil
	}

	bootstrapAddr := strings.ToLower(strings.TrimSpace(state.BootstrapWalletAddress))
	if bootstrapAddr == "" || strings.ToLower(strings.TrimSpace(session.WalletAddr)) != bootstrapAddr {
		_ = h.respondUnauthorized(ctx)
		return nil, true, nil
	}

	if !strings.EqualFold(bootstrapUsername, state.BootstrapUsername) {
		_ = h.respondUnauthorized(ctx)
		return nil, true, nil
	}

	return session, false, nil
}

func generateSetupToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

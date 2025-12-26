package lift

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"
)

// requireAdminLift validates admin authentication for Lift handlers
func (h *Handler) requireAdminLift(ctx *lift.Context) (*auth.Claims, error) {
	token := h.getBearerTokenLift(ctx)
	if err := common.ValidateRequiredParam("token", token); err != nil {
		return nil, errors.New("missing authentication")
	}

	oauthSvc := createOAuthService(h.cfg.JWTSecret, h.cfg, h.repos, h.logger)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return nil, err
	}

	// Check admin role
	user, err := h.repos.Account().GetUser(ctx.Context, claims.Username)
	if err != nil || user.Role != roleAdmin {
		return nil, errors.New(common.ErrorAdminAccessRequired)
	}

	return claims, nil
}

// HandleGetAdminDomainBlocksLift handles GET /api/v1/admin/domain_blocks
func (h *Handler) HandleGetAdminDomainBlocksLift(ctx *lift.Context) error {
	// Check admin authentication
	_, err := h.requireAdminLift(ctx)
	if err != nil {
		ctx.Status(http.StatusForbidden)
		return ctx.JSON(map[string]string{"error": err.Error()})
	}

	// Parse query parameters
	limitStr := ctx.Query("limit")
	limit, err := common.ParseFederationLimit(limitStr)
	if err != nil {
		limit = 100
	}

	cursor := ctx.Query("max_id")

	// Get domain blocks from storage
	blocks, nextCursor, err := h.repos.DomainBlock().GetDomainBlocks(ctx.Context, limit, cursor)
	if err != nil {
		h.logger.Error("failed to get domain blocks", zap.Error(err))
		ctx.Status(http.StatusInternalServerError)
		return ctx.JSON(map[string]string{"error": "failed to get domain blocks"})
	}

	// Convert to response format
	responses := make([]apimodels.AdminDomainBlockResponse, 0, len(blocks))
	for _, block := range blocks {
		resp := apimodels.AdminDomainBlockResponse{
			ID:             block.ID,
			Domain:         block.Domain,
			Severity:       block.Severity,
			RejectMedia:    block.RejectMedia,
			RejectReports:  block.RejectReports,
			PrivateComment: block.PrivateComment,
			PublicComment:  block.PublicComment,
			Obfuscate:      block.Obfuscate,
			CreatedAt:      block.CreatedAt,
			UpdatedAt:      block.UpdatedAt,
		}
		responses = append(responses, resp)
	}

	// Add Link header for pagination
	if nextCursor != "" && len(responses) > 0 {
		linkHeader := fmt.Sprintf(`<%s/api/v1/admin/domain_blocks?max_id=%s&limit=%d>; rel="next"`,
			h.cfg.BaseURL(), nextCursor, limit)
		ctx.Response.Header("Link", linkHeader)
	}

	ctx.Status(http.StatusOK)
	return ctx.JSON(responses)
}

// HandleGetAdminDomainBlockLift handles GET /api/v1/admin/domain_blocks/:id
func (h *Handler) HandleGetAdminDomainBlockLift(ctx *lift.Context) error {
	// Check admin authentication
	_, err := h.requireAdminLift(ctx)
	if err != nil {
		ctx.Status(http.StatusForbidden)
		return ctx.JSON(map[string]string{"error": err.Error()})
	}

	// Get domain block ID from path
	blockID := ctx.Param("id")
	if err := common.ValidateRequiredParam("blockID", blockID); err != nil {
		ctx.Status(http.StatusBadRequest)
		return ctx.JSON(map[string]string{"error": "block ID required"})
	}

	// Get domain block from storage
	block, err := h.repos.DomainBlock().GetDomainBlock(ctx.Context, blockID)
	if err != nil {
		if err == storage.ErrNotFound {
			ctx.Status(http.StatusNotFound)
			return ctx.JSON(map[string]string{"error": "domain block not found"})
		}
		h.logger.Error("failed to get domain block", zap.String("id", blockID), zap.Error(err))
		ctx.Status(http.StatusInternalServerError)
		return ctx.JSON(map[string]string{"error": "failed to get domain block"})
	}

	// Convert to response format
	resp := apimodels.AdminDomainBlockResponse{
		ID:             block.ID,
		Domain:         block.Domain,
		Severity:       block.Severity,
		RejectMedia:    block.RejectMedia,
		RejectReports:  block.RejectReports,
		PrivateComment: block.PrivateComment,
		PublicComment:  block.PublicComment,
		Obfuscate:      block.Obfuscate,
		CreatedAt:      block.CreatedAt,
		UpdatedAt:      block.UpdatedAt,
	}

	ctx.Status(http.StatusOK)
	return ctx.JSON(resp)
}

// HandleCreateAdminDomainBlockLift handles POST /api/v1/admin/domain_blocks
func (h *Handler) HandleCreateAdminDomainBlockLift(ctx *lift.Context) error {
	// Check admin authentication
	adminClaims, err := h.requireAdminLift(ctx)
	if err != nil {
		ctx.Status(http.StatusForbidden)
		return ctx.JSON(map[string]string{"error": err.Error()})
	}

	// Parse request body
	var req apimodels.AdminDomainBlockRequest
	if err := ctx.ParseRequest(&req); err != nil {
		h.logger.Debug("invalid domain block request", zap.Error(err))
		ctx.Status(http.StatusBadRequest)
		return ctx.JSON(map[string]string{"error": "invalid request"})
	}

	// Validate domain using centralized validation
	if err := common.ValidateRequiredParam("domain", req.Domain); err != nil {
		ctx.Status(http.StatusBadRequest)
		return ctx.JSON(map[string]string{"error": "domain is required"})
	}

	// Clean domain (remove protocol, path, etc.)
	req.Domain = cleanDomain(req.Domain)

	// Validate domain format
	if _, err := url.Parse("https://" + req.Domain); err != nil {
		ctx.Status(http.StatusBadRequest)
		return ctx.JSON(map[string]string{"error": "invalid domain format"})
	}

	// Validate severity
	if err := common.ValidateRequiredParam("severity", req.Severity); err != nil {
		req.Severity = actionSuspend // Default to suspend
	}
	if req.Severity != actionSilence && req.Severity != actionSuspend {
		ctx.Status(http.StatusBadRequest)
		return ctx.JSON(map[string]string{"error": "severity must be 'silence' or 'suspend'"})
	}

	// Create domain block
	block := &storage.InstanceDomainBlock{
		Domain:         req.Domain,
		Severity:       req.Severity,
		RejectMedia:    req.RejectMedia,
		RejectReports:  req.RejectReports,
		PrivateComment: req.PrivateComment,
		PublicComment:  req.PublicComment,
		Obfuscate:      req.Obfuscate,
		CreatedBy:      adminClaims.Username,
	}

	if err := h.repos.DomainBlock().CreateDomainBlock(ctx.Context, block); err != nil {
		if strings.Contains(err.Error(), "already exists") {
			ctx.Status(http.StatusUnprocessableEntity)
			return ctx.JSON(map[string]string{"error": "domain block already exists"})
		}
		h.logger.Error("failed to create domain block",
			zap.String("domain", req.Domain),
			zap.String("admin", adminClaims.Username),
			zap.Error(err))
		ctx.Status(http.StatusInternalServerError)
		return ctx.JSON(map[string]string{"error": "failed to create domain block"})
	}

	// Log admin action
	h.logger.Info("admin created domain block",
		zap.String("domain", req.Domain),
		zap.String("severity", req.Severity),
		zap.String("admin", adminClaims.Username))

	// Convert to response format
	resp := apimodels.AdminDomainBlockResponse{
		ID:             block.ID,
		Domain:         block.Domain,
		Severity:       block.Severity,
		RejectMedia:    block.RejectMedia,
		RejectReports:  block.RejectReports,
		PrivateComment: block.PrivateComment,
		PublicComment:  block.PublicComment,
		Obfuscate:      block.Obfuscate,
		CreatedAt:      block.CreatedAt,
		UpdatedAt:      block.UpdatedAt,
	}

	ctx.Status(http.StatusOK)
	return ctx.JSON(resp)
}

// HandleUpdateAdminDomainBlockLift handles PUT /api/v1/admin/domain_blocks/:id
func (h *Handler) HandleUpdateAdminDomainBlockLift(ctx *lift.Context) error {
	// Check admin authentication
	adminClaims, err := h.requireAdminLift(ctx)
	if err != nil {
		ctx.Status(http.StatusForbidden)
		return ctx.JSON(map[string]string{"error": err.Error()})
	}

	// Get domain block ID from path
	blockID := ctx.Param("id")
	if err := common.ValidateRequiredParam("blockID", blockID); err != nil {
		ctx.Status(http.StatusBadRequest)
		return ctx.JSON(map[string]string{"error": "block ID required"})
	}

	// Parse request body
	var req apimodels.AdminDomainBlockRequest
	if err := ctx.ParseRequest(&req); err != nil {
		h.logger.Debug("invalid domain block update request", zap.Error(err))
		ctx.Status(http.StatusBadRequest)
		return ctx.JSON(map[string]string{"error": "invalid request"})
	}

	// Build updates map
	updates := make(map[string]any)
	if err := common.ValidateRequiredParam("severity", req.Severity); err == nil {
		if req.Severity != actionSilence && req.Severity != actionSuspend {
			ctx.Status(http.StatusBadRequest)
			return ctx.JSON(map[string]string{"error": "severity must be 'silence' or 'suspend'"})
		}
		updates["severity"] = req.Severity
	}
	updates["reject_media"] = req.RejectMedia
	updates["reject_reports"] = req.RejectReports
	updates["private_comment"] = req.PrivateComment
	updates["public_comment"] = req.PublicComment
	updates["obfuscate"] = req.Obfuscate

	// Update domain block
	if err := h.repos.DomainBlock().UpdateDomainBlock(ctx.Context, blockID, updates); err != nil {
		if err == storage.ErrNotFound {
			ctx.Status(http.StatusNotFound)
			return ctx.JSON(map[string]string{"error": "domain block not found"})
		}
		h.logger.Error("failed to update domain block",
			zap.String("id", blockID),
			zap.String("admin", adminClaims.Username),
			zap.Error(err))
		ctx.Status(http.StatusInternalServerError)
		return ctx.JSON(map[string]string{"error": "failed to update domain block"})
	}

	// Get updated block
	block, err := h.repos.DomainBlock().GetDomainBlock(ctx.Context, blockID)
	if err != nil {
		h.logger.Error("failed to get updated domain block", zap.String("id", blockID), zap.Error(err))
		ctx.Status(http.StatusInternalServerError)
		return ctx.JSON(map[string]string{"error": "failed to get updated domain block"})
	}

	// Log admin action
	h.logger.Info("admin updated domain block",
		zap.String("domain", block.Domain),
		zap.String("admin", adminClaims.Username))

	// Convert to response format
	resp := apimodels.AdminDomainBlockResponse{
		ID:             block.ID,
		Domain:         block.Domain,
		Severity:       block.Severity,
		RejectMedia:    block.RejectMedia,
		RejectReports:  block.RejectReports,
		PrivateComment: block.PrivateComment,
		PublicComment:  block.PublicComment,
		Obfuscate:      block.Obfuscate,
		CreatedAt:      block.CreatedAt,
		UpdatedAt:      block.UpdatedAt,
	}

	ctx.Status(http.StatusOK)
	return ctx.JSON(resp)
}

// HandleDeleteAdminDomainBlockLift handles DELETE /api/v1/admin/domain_blocks/:id
func (h *Handler) HandleDeleteAdminDomainBlockLift(ctx *lift.Context) error {
	// Check admin authentication
	adminClaims, err := h.requireAdminLift(ctx)
	if err != nil {
		ctx.Status(http.StatusForbidden)
		return ctx.JSON(map[string]string{"error": err.Error()})
	}

	// Get domain block ID from path
	blockID := ctx.Param("id")
	if err := common.ValidateRequiredParam("blockID", blockID); err != nil {
		ctx.Status(http.StatusBadRequest)
		return ctx.JSON(map[string]string{"error": "block ID required"})
	}

	// Get block before deletion for logging
	block, err := h.repos.DomainBlock().GetDomainBlock(ctx.Context, blockID)
	if err != nil {
		if err == storage.ErrNotFound {
			ctx.Status(http.StatusNotFound)
			return ctx.JSON(map[string]string{"error": "domain block not found"})
		}
		h.logger.Error("failed to get domain block", zap.String("id", blockID), zap.Error(err))
		ctx.Status(http.StatusInternalServerError)
		return ctx.JSON(map[string]string{"error": "failed to get domain block"})
	}

	// Delete domain block
	if err := h.repos.DomainBlock().DeleteDomainBlock(ctx.Context, blockID); err != nil {
		h.logger.Error("failed to delete domain block",
			zap.String("id", blockID),
			zap.String("admin", adminClaims.Username),
			zap.Error(err))
		ctx.Status(http.StatusInternalServerError)
		return ctx.JSON(map[string]string{"error": "failed to delete domain block"})
	}

	// Log admin action
	h.logger.Info("admin deleted domain block",
		zap.String("domain", block.Domain),
		zap.String("admin", adminClaims.Username))

	// Return empty object (Mastodon compatibility)
	ctx.Status(http.StatusOK)
	return ctx.JSON(apimodels.EmptyObject{})
}

// HandleGetAdminDomainAllowsLift handles GET /api/v1/admin/domain_allows
func (h *Handler) HandleGetAdminDomainAllowsLift(ctx *lift.Context) error {
	// Check admin authentication
	_, err := h.requireAdminLift(ctx)
	if err != nil {
		ctx.Status(http.StatusForbidden)
		return ctx.JSON(map[string]string{"error": err.Error()})
	}

	// Parse query parameters
	limitStr := ctx.Query("limit")
	limit, err := common.ParseFederationLimit(limitStr)
	if err != nil {
		limit = 100
	}

	cursor := ctx.Query("max_id")

	// Get domain allows from storage
	allows, nextCursor, err := h.repos.DomainBlock().GetDomainAllows(ctx.Context, limit, cursor)
	if err != nil {
		h.logger.Error("failed to get domain allows", zap.Error(err))
		ctx.Status(http.StatusInternalServerError)
		return ctx.JSON(map[string]string{"error": "failed to get domain allows"})
	}

	// Convert to response format
	responses := make([]apimodels.AdminDomainAllowResponse, 0, len(allows))
	for _, allow := range allows {
		resp := apimodels.AdminDomainAllowResponse{
			ID:        allow.ID,
			Domain:    allow.Domain,
			CreatedAt: allow.CreatedAt,
		}
		responses = append(responses, resp)
	}

	// Add Link header for pagination
	if nextCursor != "" && len(responses) > 0 {
		linkHeader := fmt.Sprintf(`<%s/api/v1/admin/domain_allows?max_id=%s&limit=%d>; rel="next"`,
			h.cfg.BaseURL(), nextCursor, limit)
		ctx.Response.Header("Link", linkHeader)
	}

	ctx.Status(http.StatusOK)
	return ctx.JSON(responses)
}

// HandleCreateAdminDomainAllowLift handles POST /api/v1/admin/domain_allows
func (h *Handler) HandleCreateAdminDomainAllowLift(ctx *lift.Context) error {
	// Check admin authentication
	adminClaims, err := h.requireAdminLift(ctx)
	if err != nil {
		ctx.Status(http.StatusForbidden)
		return ctx.JSON(map[string]string{"error": err.Error()})
	}

	// Parse request body
	var req apimodels.AdminDomainAllowRequest
	if err := ctx.ParseRequest(&req); err != nil {
		h.logger.Debug("invalid domain allow request", zap.Error(err))
		ctx.Status(http.StatusBadRequest)
		return ctx.JSON(map[string]string{"error": "invalid request"})
	}

	// Validate domain using centralized validation
	if err := common.ValidateRequiredParam("domain", req.Domain); err != nil {
		ctx.Status(http.StatusBadRequest)
		return ctx.JSON(map[string]string{"error": "domain is required"})
	}

	// Clean domain
	req.Domain = cleanDomain(req.Domain)

	// Validate domain format
	if _, err := url.Parse("https://" + req.Domain); err != nil {
		ctx.Status(http.StatusBadRequest)
		return ctx.JSON(map[string]string{"error": "invalid domain format"})
	}

	// Create domain allow
	allow := &storage.DomainAllow{
		Domain:    req.Domain,
		CreatedBy: adminClaims.Username,
	}

	if err := h.repos.DomainBlock().CreateDomainAllow(ctx.Context, allow); err != nil {
		if strings.Contains(err.Error(), "already exists") {
			ctx.Status(http.StatusUnprocessableEntity)
			return ctx.JSON(map[string]string{"error": "domain allow already exists"})
		}
		h.logger.Error("failed to create domain allow",
			zap.String("domain", req.Domain),
			zap.String("admin", adminClaims.Username),
			zap.Error(err))
		ctx.Status(http.StatusInternalServerError)
		return ctx.JSON(map[string]string{"error": "failed to create domain allow"})
	}

	// Log admin action
	h.logger.Info("admin created domain allow",
		zap.String("domain", req.Domain),
		zap.String("admin", adminClaims.Username))

	// Convert to response format
	resp := apimodels.AdminDomainAllowResponse{
		ID:        allow.ID,
		Domain:    allow.Domain,
		CreatedAt: allow.CreatedAt,
	}

	ctx.Status(http.StatusOK)
	return ctx.JSON(resp)
}

// HandleDeleteAdminDomainAllowLift handles DELETE /api/v1/admin/domain_allows/:id
func (h *Handler) HandleDeleteAdminDomainAllowLift(ctx *lift.Context) error {
	return h.adminDomainDeleteAction(ctx, "allow", "allow ID required", "domain allow not found",
		func(itemID string) error {
			return h.repos.DomainBlock().DeleteDomainAllow(ctx.Context, itemID)
		})
}

// HandleGetFederationInstancesLift handles GET /api/v1/admin/federation/instances
func (h *Handler) HandleGetFederationInstancesLift(ctx *lift.Context) error {
	// Check admin authentication
	_, err := h.requireAdminLift(ctx)
	if err != nil {
		ctx.Status(http.StatusForbidden)
		return ctx.JSON(map[string]string{"error": err.Error()})
	}

	// Parse query parameters
	limitStr := ctx.Query("limit")
	limit, err := common.ParseFederationLimit(limitStr)
	if err != nil {
		limit = 100
	}

	cursor := ctx.Query("cursor")

	// Get known instances from storage
	instances, nextCursor, err := h.repos.Federation().GetKnownInstances(ctx.Context, limit, cursor)
	if err != nil {
		h.logger.Error("failed to get known instances", zap.Error(err))
		ctx.Status(http.StatusInternalServerError)
		return ctx.JSON(map[string]string{"error": "failed to get instances"})
	}

	// Convert to response format
	responses := make([]apimodels.InstanceInfoResponse, 0, len(instances))
	for _, instance := range instances {
		// Check if domain is blocked
		isBlocked, block, err := h.repos.DomainBlock().IsDomainBlocked(ctx.Context, instance.Domain)
		if err != nil {
			h.logger.Warn("failed to check domain block status",
				zap.String("domain", instance.Domain),
				zap.Error(err))
		}

		resp := apimodels.InstanceInfoResponse{
			Domain:        instance.Domain,
			Software:      instance.Software,
			Version:       instance.Version,
			ActiveUsers:   int(instance.ActiveUsers),
			TotalMessages: instance.TotalMessages,
			TrustScore:    instance.TrustScore,
			FirstSeen:     instance.FirstSeen,
			LastSeen:      instance.LastSeen,
		}

		if isBlocked && block != nil {
			resp.IsSilenced = block.Severity == "silence"
			resp.IsSuspended = block.Severity == "suspend"
		}

		responses = append(responses, resp)
	}

	// Create response with pagination cursor
	result := apimodels.FederationInstancesResponse{
		Instances: responses,
	}
	if nextCursor != "" {
		result.NextCursor = &nextCursor
	}

	ctx.Status(http.StatusOK)
	return ctx.JSON(result)
}

// HandleGetFederationInstanceLift handles GET /api/v1/admin/federation/instance/:domain
func (h *Handler) HandleGetFederationInstanceLift(ctx *lift.Context) error {
	// Check admin authentication
	_, err := h.requireAdminLift(ctx)
	if err != nil {
		ctx.Status(http.StatusForbidden)
		return ctx.JSON(map[string]string{"error": err.Error()})
	}

	// Get domain from path
	domain := ctx.Param("domain")
	if err := common.ValidateRequiredParam("domain", domain); err != nil {
		ctx.Status(http.StatusBadRequest)
		return ctx.JSON(map[string]string{"error": "domain required"})
	}

	// Clean domain
	domain = cleanDomain(domain)

	// Get instance info from storage
	instance, err := h.repos.Federation().GetInstanceInfo(ctx.Context, domain)
	if err != nil {
		if err == storage.ErrNotFound {
			ctx.Status(http.StatusNotFound)
			return ctx.JSON(map[string]string{"error": "instance not found"})
		}
		h.logger.Error("failed to get instance info", zap.String("domain", domain), zap.Error(err))
		ctx.Status(http.StatusInternalServerError)
		return ctx.JSON(map[string]string{"error": "failed to get instance info"})
	}

	// Check if domain is blocked
	isBlocked, block, err := h.repos.DomainBlock().IsDomainBlocked(ctx.Context, instance.Domain)
	if err != nil {
		h.logger.Warn("failed to check domain block status",
			zap.String("domain", instance.Domain),
			zap.Error(err))
	}

	resp := apimodels.InstanceInfoResponse{
		Domain:        instance.Domain,
		Software:      instance.Software,
		Version:       instance.Version,
		ActiveUsers:   int(instance.ActiveUsers),
		TotalMessages: instance.TotalMessages,
		TrustScore:    instance.TrustScore,
		FirstSeen:     instance.FirstSeen,
		LastSeen:      instance.LastSeen,
	}

	if isBlocked && block != nil {
		resp.IsSilenced = block.Severity == "silence"
		resp.IsSuspended = block.Severity == "suspend"
	}

	// Add detailed federation information
	details := h.getFederationDetails(ctx.Context, domain)

	// Create response with instance info and details
	responseData := apimodels.FederationInstanceResponse{
		Instance: resp,
		Details:  details,
	}

	ctx.Status(http.StatusOK)
	return ctx.JSON(responseData)
}

// HandleGetFederationStatisticsLift handles GET /api/v1/admin/federation/statistics
func (h *Handler) HandleGetFederationStatisticsLift(ctx *lift.Context) error {
	// Check admin authentication
	_, err := h.requireAdminLift(ctx)
	if err != nil {
		ctx.Status(http.StatusForbidden)
		return ctx.JSON(map[string]string{"error": err.Error()})
	}

	// Get time range from query parameters (default to last 7 days)
	endTime := time.Now()
	startTime := endTime.AddDate(0, 0, -7)

	if startStr := ctx.Query("start"); startStr != "" {
		if t, err := time.Parse(time.RFC3339, startStr); err == nil {
			startTime = t
		}
	}
	if endStr := ctx.Query("end"); endStr != "" {
		if t, err := time.Parse(time.RFC3339, endStr); err == nil {
			endTime = t
		}
	}

	// Get federation statistics from storage
	stats, err := h.repos.Federation().GetFederationStatistics(ctx.Context, startTime, endTime)
	if err != nil {
		h.logger.Error("failed to get federation statistics", zap.Error(err))
		ctx.Status(http.StatusInternalServerError)
		return ctx.JSON(map[string]string{"error": "failed to get statistics"})
	}

	// Build response
	resp := apimodels.FederationStatisticsResponse{
		ActiveInstances: stats.ActiveInstances,
		TotalMessages:   stats.TotalMessages,
		TotalUsers:      stats.TotalUsers,
		TimeRange: apimodels.FederationStatisticsTimeRange{
			Start: startTime.Format(time.RFC3339),
			End:   endTime.Format(time.RFC3339),
		},
	}

	ctx.Status(http.StatusOK)
	return ctx.JSON(resp)
}

// HandleGetEmailDomainBlocksLift handles GET /api/v1/admin/email_domain_blocks
func (h *Handler) HandleGetEmailDomainBlocksLift(ctx *lift.Context) error {
	// Check admin authentication
	_, err := h.requireAdminLift(ctx)
	if err != nil {
		ctx.Status(http.StatusForbidden)
		return ctx.JSON(map[string]string{"error": err.Error()})
	}

	// Parse query parameters
	limitStr := ctx.Query("limit")
	limit, err := common.ParseFederationLimit(limitStr)
	if err != nil {
		limit = 100
	}

	cursor := ctx.Query("cursor")

	// Get email domain blocks from storage
	blocks, nextCursor, err := h.repos.DomainBlock().GetEmailDomainBlocks(ctx.Context, limit, cursor)
	if err != nil {
		h.logger.Error("failed to get email domain blocks", zap.Error(err))
		ctx.Status(http.StatusInternalServerError)
		return ctx.JSON(map[string]string{"error": "failed to get email domain blocks"})
	}

	// Convert to response format
	responses := make([]apimodels.EmailDomainBlockResponse, 0, len(blocks))
	for _, block := range blocks {
		resp := apimodels.EmailDomainBlockResponse{
			ID:        block.ID,
			Domain:    block.Domain,
			CreatedAt: block.CreatedAt,
		}
		responses = append(responses, resp)
	}

	// Create response with cursor for pagination
	result := apimodels.EmailDomainBlocksResponse{
		Blocks: responses,
	}
	if nextCursor != "" {
		result.NextCursor = &nextCursor
	}

	ctx.Status(http.StatusOK)
	return ctx.JSON(result)
}

// HandleCreateEmailDomainBlockLift handles POST /api/v1/admin/email_domain_blocks
func (h *Handler) HandleCreateEmailDomainBlockLift(ctx *lift.Context) error {
	// Check admin authentication
	adminClaims, err := h.requireAdminLift(ctx)
	if err != nil {
		ctx.Status(http.StatusForbidden)
		return ctx.JSON(map[string]string{"error": err.Error()})
	}

	// Parse request body
	var req apimodels.EmailDomainBlockRequest
	if err := ctx.ParseRequest(&req); err != nil {
		h.logger.Debug("invalid email domain block request", zap.Error(err))
		ctx.Status(http.StatusBadRequest)
		return ctx.JSON(map[string]string{"error": "invalid request"})
	}

	// Validate domain using centralized validation
	if err := common.ValidateRequiredParam("domain", req.Domain); err != nil {
		ctx.Status(http.StatusBadRequest)
		return ctx.JSON(map[string]string{"error": "domain is required"})
	}

	// Clean domain (remove @ if present)
	req.Domain = strings.TrimPrefix(req.Domain, "@")
	req.Domain = strings.ToLower(req.Domain)

	// Create email domain block
	block := &storage.EmailDomainBlock{
		Domain:    req.Domain,
		CreatedBy: adminClaims.Username,
	}

	if err := h.repos.DomainBlock().CreateEmailDomainBlock(ctx.Context, block); err != nil {
		if strings.Contains(err.Error(), "already exists") {
			ctx.Status(http.StatusUnprocessableEntity)
			return ctx.JSON(map[string]string{"error": "email domain block already exists"})
		}
		h.logger.Error("failed to create email domain block",
			zap.String("domain", req.Domain),
			zap.String("admin", adminClaims.Username),
			zap.Error(err))
		ctx.Status(http.StatusInternalServerError)
		return ctx.JSON(map[string]string{"error": "failed to create email domain block"})
	}

	// Log admin action
	h.logger.Info("admin created email domain block",
		zap.String("domain", req.Domain),
		zap.String("admin", adminClaims.Username))

	// Convert to response format
	resp := apimodels.EmailDomainBlockResponse{
		ID:        block.ID,
		Domain:    block.Domain,
		CreatedAt: block.CreatedAt,
	}

	ctx.Status(http.StatusOK)
	return ctx.JSON(resp)
}

// HandleDeleteEmailDomainBlockLift handles DELETE /api/v1/admin/email_domain_blocks/:id
func (h *Handler) HandleDeleteEmailDomainBlockLift(ctx *lift.Context) error {
	return h.adminDomainDeleteAction(ctx, "email domain block", "block ID required", "email domain block not found",
		func(itemID string) error {
			return h.repos.DomainBlock().DeleteEmailDomainBlock(ctx.Context, itemID)
		})
}

// adminDomainDeleteAction consolidates domain deletion operations
func (h *Handler) adminDomainDeleteAction(ctx *lift.Context, itemType, validationError, notFoundError string, deleteFn func(string) error) error {
	// Check admin authentication
	adminClaims, err := h.requireAdminLift(ctx)
	if err != nil {
		ctx.Status(http.StatusForbidden)
		return ctx.JSON(map[string]string{"error": err.Error()})
	}

	// Get item ID from path
	itemID := ctx.Param("id")
	if err := common.ValidateRequiredParam("itemID", itemID); err != nil {
		ctx.Status(http.StatusBadRequest)
		return ctx.JSON(map[string]string{"error": validationError})
	}

	// Delete the item
	if err := deleteFn(itemID); err != nil {
		if err == storage.ErrNotFound {
			ctx.Status(http.StatusNotFound)
			return ctx.JSON(map[string]string{"error": notFoundError})
		}
		h.logger.Error("failed to delete "+itemType,
			zap.String("id", itemID),
			zap.String("admin", adminClaims.Username),
			zap.Error(err))
		ctx.Status(http.StatusInternalServerError)
		return ctx.JSON(map[string]string{"error": "failed to delete " + itemType})
	}

	// Log admin action
	h.logger.Info("admin deleted "+itemType,
		zap.String("id", itemID),
		zap.String("admin", adminClaims.Username))

	// Return empty object (Mastodon compatibility)
	ctx.Status(http.StatusOK)
	return ctx.JSON(apimodels.EmptyObject{})
}

// cleanDomain removes protocol, path, and trailing slashes from a domain
func cleanDomain(domain string) string {
	// Remove protocol
	domain = strings.TrimPrefix(domain, "https://")
	domain = strings.TrimPrefix(domain, "http://")

	// Remove path
	if idx := strings.Index(domain, "/"); idx != -1 {
		domain = domain[:idx]
	}

	// Remove port if it's the default
	domain = strings.TrimSuffix(domain, ":443")
	domain = strings.TrimSuffix(domain, ":80")

	// Lowercase
	domain = strings.ToLower(domain)

	return domain
}

// Helper methods for federation details
func (h *Handler) getFederationDetails(ctx context.Context, domain string) map[string]any {
	stats, err := h.repos.Instance().GetDomainStats(ctx, domain)
	if err != nil {
		h.logger.Warn("failed to get domain stats", zap.Error(err))
		return map[string]any{}
	}

	// Handle stats as a map since GetDomainStats returns any
	if statsMap, ok := stats.(map[string]any); ok {
		return map[string]any{
			"total_users":    getFieldOrDefault(statsMap, "total_users", 0),
			"active_users":   getFieldOrDefault(statsMap, "active_users", 0),
			"total_statuses": getFieldOrDefault(statsMap, "total_statuses", 0),
			"last_activity":  getFieldOrDefault(statsMap, "last_activity", nil),
			"software":       getFieldOrDefault(statsMap, "software", ""),
			"version":        getFieldOrDefault(statsMap, "version", ""),
		}
	}

	// Return empty map if stats is not in expected format
	return map[string]any{}
}

// Helper function to safely get field from map with default value
func getFieldOrDefault(m map[string]any, key string, defaultValue any) any {
	if value, exists := m[key]; exists {
		return value
	}
	return defaultValue
}

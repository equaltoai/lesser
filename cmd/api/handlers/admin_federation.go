package handlers

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/aron23/lesser/pkg/common"
	"github.com/aron23/lesser/pkg/storage"
	"github.com/aws/aws-lambda-go/events"
	"go.uber.org/zap"
)

// AdminDomainBlockRequest represents a request to block a domain at the instance level
type AdminDomainBlockRequest struct {
	Domain         string `json:"domain"`
	Severity       string `json:"severity"`        // "silence" or "suspend"
	RejectMedia    bool   `json:"reject_media"`    // Reject media files from this domain
	RejectReports  bool   `json:"reject_reports"`  // Reject reports from this domain
	PrivateComment string `json:"private_comment"` // Admin-only notes
	PublicComment  string `json:"public_comment"`  // Public reason
	Obfuscate      bool   `json:"obfuscate"`       // Whether to obfuscate domain in public lists
}

// AdminDomainBlockResponse represents a domain block in API responses
type AdminDomainBlockResponse struct {
	ID             string    `json:"id"`
	Domain         string    `json:"domain"`
	Severity       string    `json:"severity"`
	RejectMedia    bool      `json:"reject_media"`
	RejectReports  bool      `json:"reject_reports"`
	PrivateComment string    `json:"private_comment,omitempty"`
	PublicComment  string    `json:"public_comment,omitempty"`
	Obfuscate      bool      `json:"obfuscate"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// HandleGetAdminDomainBlocks handles GET /api/v1/admin/domain_blocks
func (h *Handler) HandleGetAdminDomainBlocks(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	// Check admin authentication
	_, err := h.requireAdmin(ctx, request)
	if err != nil {
		return common.Forbidden(err), nil
	}

	// Parse query parameters
	limit := 100
	if limitStr := request.QueryStringParameters["limit"]; limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 200 {
			limit = l
		}
	}

	cursor := request.QueryStringParameters["max_id"]

	// Get domain blocks from storage
	blocks, nextCursor, err := h.store.GetDomainBlocks(ctx, limit, cursor)
	if err != nil {
		h.logger.Error("failed to get domain blocks", zap.Error(err))
		return common.InternalServerError(fmt.Errorf("failed to get domain blocks")), nil
	}

	// Convert to response format
	responses := make([]AdminDomainBlockResponse, 0, len(blocks))
	for _, block := range blocks {
		resp := AdminDomainBlockResponse{
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

	// Create response with Link header for pagination
	response := common.OK(responses)
	if nextCursor != "" && len(responses) > 0 {
		linkHeader := fmt.Sprintf(`<%s/api/v1/admin/domain_blocks?max_id=%s&limit=%d>; rel="next"`,
			h.cfg.BaseURL(), nextCursor, limit)
		response.Headers["Link"] = linkHeader
	}

	return response, nil
}

// HandleGetAdminDomainBlock handles GET /api/v1/admin/domain_blocks/:id
func (h *Handler) HandleGetAdminDomainBlock(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	// Check admin authentication
	_, err := h.requireAdmin(ctx, request)
	if err != nil {
		return common.Forbidden(err), nil
	}

	// Get domain block ID from path
	blockID := request.PathParameters["id"]
	if blockID == "" {
		return common.BadRequest(errors.New("block ID required")), nil
	}

	// Get domain block from storage
	block, err := h.store.GetDomainBlock(ctx, blockID)
	if err != nil {
		if err == storage.ErrNotFound {
			return common.NotFound(errors.New("domain block not found")), nil
		}
		h.logger.Error("failed to get domain block", zap.String("id", blockID), zap.Error(err))
		return common.InternalServerError(fmt.Errorf("failed to get domain block")), nil
	}

	// Convert to response format
	resp := AdminDomainBlockResponse{
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

	return common.OK(resp), nil
}

// HandleCreateAdminDomainBlock handles POST /api/v1/admin/domain_blocks
func (h *Handler) HandleCreateAdminDomainBlock(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	// Check admin authentication
	adminClaims, err := h.requireAdmin(ctx, request)
	if err != nil {
		return common.Forbidden(err), nil
	}

	// Parse request body
	var req AdminDomainBlockRequest
	if err := common.ParseRequestBody([]byte(request.Body), &req); err != nil {
		h.logger.Debug("invalid domain block request", zap.Error(err))
		return common.BadRequest(errors.New("invalid request")), nil
	}

	// Validate domain
	if req.Domain == "" {
		return common.BadRequest(errors.New("domain is required")), nil
	}

	// Clean domain (remove protocol, path, etc.)
	req.Domain = cleanDomain(req.Domain)

	// Validate domain format
	if _, err := url.Parse("https://" + req.Domain); err != nil {
		return common.BadRequest(errors.New("invalid domain format")), nil
	}

	// Validate severity
	if req.Severity == "" {
		req.Severity = "suspend" // Default to suspend
	}
	if req.Severity != "silence" && req.Severity != "suspend" {
		return common.BadRequest(errors.New("severity must be 'silence' or 'suspend'")), nil
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

	if err := h.store.CreateDomainBlock(ctx, block); err != nil {
		if strings.Contains(err.Error(), "already exists") {
			return common.UnprocessableEntity(errors.New("domain block already exists")), nil
		}
		h.logger.Error("failed to create domain block",
			zap.String("domain", req.Domain),
			zap.String("admin", adminClaims.Username),
			zap.Error(err))
		return common.InternalServerError(fmt.Errorf("failed to create domain block")), nil
	}

	// Log admin action
	h.logger.Info("admin created domain block",
		zap.String("domain", req.Domain),
		zap.String("severity", req.Severity),
		zap.String("admin", adminClaims.Username))

	// Convert to response format
	resp := AdminDomainBlockResponse{
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

	return common.OK(resp), nil
}

// HandleUpdateAdminDomainBlock handles PUT /api/v1/admin/domain_blocks/:id
func (h *Handler) HandleUpdateAdminDomainBlock(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	// Check admin authentication
	adminClaims, err := h.requireAdmin(ctx, request)
	if err != nil {
		return common.Forbidden(err), nil
	}

	// Get domain block ID from path
	blockID := request.PathParameters["id"]
	if blockID == "" {
		return common.BadRequest(errors.New("block ID required")), nil
	}

	// Parse request body
	var req AdminDomainBlockRequest
	if err := common.ParseRequestBody([]byte(request.Body), &req); err != nil {
		h.logger.Debug("invalid domain block update request", zap.Error(err))
		return common.BadRequest(errors.New("invalid request")), nil
	}

	// Build updates map
	updates := make(map[string]interface{})
	if req.Severity != "" {
		if req.Severity != "silence" && req.Severity != "suspend" {
			return common.BadRequest(errors.New("severity must be 'silence' or 'suspend'")), nil
		}
		updates["severity"] = req.Severity
	}
	updates["reject_media"] = req.RejectMedia
	updates["reject_reports"] = req.RejectReports
	updates["private_comment"] = req.PrivateComment
	updates["public_comment"] = req.PublicComment
	updates["obfuscate"] = req.Obfuscate

	// Update domain block
	if err := h.store.UpdateDomainBlock(ctx, blockID, updates); err != nil {
		if err == storage.ErrNotFound {
			return common.NotFound(errors.New("domain block not found")), nil
		}
		h.logger.Error("failed to update domain block",
			zap.String("id", blockID),
			zap.String("admin", adminClaims.Username),
			zap.Error(err))
		return common.InternalServerError(fmt.Errorf("failed to update domain block")), nil
	}

	// Get updated block
	block, err := h.store.GetDomainBlock(ctx, blockID)
	if err != nil {
		h.logger.Error("failed to get updated domain block", zap.String("id", blockID), zap.Error(err))
		return common.InternalServerError(fmt.Errorf("failed to get updated domain block")), nil
	}

	// Log admin action
	h.logger.Info("admin updated domain block",
		zap.String("domain", block.Domain),
		zap.String("admin", adminClaims.Username))

	// Convert to response format
	resp := AdminDomainBlockResponse{
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

	return common.OK(resp), nil
}

// HandleDeleteAdminDomainBlock handles DELETE /api/v1/admin/domain_blocks/:id
func (h *Handler) HandleDeleteAdminDomainBlock(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	// Check admin authentication
	adminClaims, err := h.requireAdmin(ctx, request)
	if err != nil {
		return common.Forbidden(err), nil
	}

	// Get domain block ID from path
	blockID := request.PathParameters["id"]
	if blockID == "" {
		return common.BadRequest(errors.New("block ID required")), nil
	}

	// Get block before deletion for logging
	block, err := h.store.GetDomainBlock(ctx, blockID)
	if err != nil {
		if err == storage.ErrNotFound {
			return common.NotFound(errors.New("domain block not found")), nil
		}
		h.logger.Error("failed to get domain block", zap.String("id", blockID), zap.Error(err))
		return common.InternalServerError(fmt.Errorf("failed to get domain block")), nil
	}

	// Delete domain block
	if err := h.store.DeleteDomainBlock(ctx, blockID); err != nil {
		h.logger.Error("failed to delete domain block",
			zap.String("id", blockID),
			zap.String("admin", adminClaims.Username),
			zap.Error(err))
		return common.InternalServerError(fmt.Errorf("failed to delete domain block")), nil
	}

	// Log admin action
	h.logger.Info("admin deleted domain block",
		zap.String("domain", block.Domain),
		zap.String("admin", adminClaims.Username))

	// Return empty object (Mastodon compatibility)
	return common.OK(map[string]interface{}{}), nil
}

// AdminDomainAllowResponse represents a domain allow in API responses
type AdminDomainAllowResponse struct {
	ID        string    `json:"id"`
	Domain    string    `json:"domain"`
	CreatedAt time.Time `json:"created_at"`
}

// HandleGetAdminDomainAllows handles GET /api/v1/admin/domain_allows
func (h *Handler) HandleGetAdminDomainAllows(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	// Check admin authentication
	_, err := h.requireAdmin(ctx, request)
	if err != nil {
		return common.Forbidden(err), nil
	}

	// Parse query parameters
	limit := 100
	if limitStr := request.QueryStringParameters["limit"]; limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 200 {
			limit = l
		}
	}

	cursor := request.QueryStringParameters["max_id"]

	// Get domain allows from storage
	allows, nextCursor, err := h.store.GetDomainAllows(ctx, limit, cursor)
	if err != nil {
		h.logger.Error("failed to get domain allows", zap.Error(err))
		return common.InternalServerError(fmt.Errorf("failed to get domain allows")), nil
	}

	// Convert to response format
	responses := make([]AdminDomainAllowResponse, 0, len(allows))
	for _, allow := range allows {
		resp := AdminDomainAllowResponse{
			ID:        allow.ID,
			Domain:    allow.Domain,
			CreatedAt: allow.CreatedAt,
		}
		responses = append(responses, resp)
	}

	// Create response with Link header for pagination
	response := common.OK(responses)
	if nextCursor != "" && len(responses) > 0 {
		linkHeader := fmt.Sprintf(`<%s/api/v1/admin/domain_allows?max_id=%s&limit=%d>; rel="next"`,
			h.cfg.BaseURL(), nextCursor, limit)
		response.Headers["Link"] = linkHeader
	}

	return response, nil
}

// HandleCreateAdminDomainAllow handles POST /api/v1/admin/domain_allows
func (h *Handler) HandleCreateAdminDomainAllow(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	// Check admin authentication
	adminClaims, err := h.requireAdmin(ctx, request)
	if err != nil {
		return common.Forbidden(err), nil
	}

	// Parse request body
	var req struct {
		Domain string `json:"domain"`
	}
	if err := common.ParseRequestBody([]byte(request.Body), &req); err != nil {
		h.logger.Debug("invalid domain allow request", zap.Error(err))
		return common.BadRequest(errors.New("invalid request")), nil
	}

	// Validate domain
	if req.Domain == "" {
		return common.BadRequest(errors.New("domain is required")), nil
	}

	// Clean domain
	req.Domain = cleanDomain(req.Domain)

	// Validate domain format
	if _, err := url.Parse("https://" + req.Domain); err != nil {
		return common.BadRequest(errors.New("invalid domain format")), nil
	}

	// Create domain allow
	allow := &storage.DomainAllow{
		Domain:    req.Domain,
		CreatedBy: adminClaims.Username,
	}

	if err := h.store.CreateDomainAllow(ctx, allow); err != nil {
		if strings.Contains(err.Error(), "already exists") {
			return common.UnprocessableEntity(errors.New("domain allow already exists")), nil
		}
		h.logger.Error("failed to create domain allow",
			zap.String("domain", req.Domain),
			zap.String("admin", adminClaims.Username),
			zap.Error(err))
		return common.InternalServerError(fmt.Errorf("failed to create domain allow")), nil
	}

	// Log admin action
	h.logger.Info("admin created domain allow",
		zap.String("domain", req.Domain),
		zap.String("admin", adminClaims.Username))

	// Convert to response format
	resp := AdminDomainAllowResponse{
		ID:        allow.ID,
		Domain:    allow.Domain,
		CreatedAt: allow.CreatedAt,
	}

	return common.OK(resp), nil
}

// HandleDeleteAdminDomainAllow handles DELETE /api/v1/admin/domain_allows/:id
func (h *Handler) HandleDeleteAdminDomainAllow(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	// Check admin authentication
	adminClaims, err := h.requireAdmin(ctx, request)
	if err != nil {
		return common.Forbidden(err), nil
	}

	// Get domain allow ID from path
	allowID := request.PathParameters["id"]
	if allowID == "" {
		return common.BadRequest(errors.New("allow ID required")), nil
	}

	// Delete domain allow
	if err := h.store.DeleteDomainAllow(ctx, allowID); err != nil {
		if err == storage.ErrNotFound {
			return common.NotFound(errors.New("domain allow not found")), nil
		}
		h.logger.Error("failed to delete domain allow",
			zap.String("id", allowID),
			zap.String("admin", adminClaims.Username),
			zap.Error(err))
		return common.InternalServerError(fmt.Errorf("failed to delete domain allow")), nil
	}

	// Log admin action
	h.logger.Info("admin deleted domain allow",
		zap.String("id", allowID),
		zap.String("admin", adminClaims.Username))

	// Return empty object (Mastodon compatibility)
	return common.OK(map[string]interface{}{}), nil
}

// InstanceInfoResponse represents instance information in API responses
type InstanceInfoResponse struct {
	Domain        string    `json:"domain"`
	Software      string    `json:"software,omitempty"`
	Version       string    `json:"version,omitempty"`
	ActiveUsers   int       `json:"active_users"`
	TotalMessages int64     `json:"total_messages"`
	TrustScore    float64   `json:"trust_score"`
	FirstSeen     time.Time `json:"first_seen"`
	LastSeen      time.Time `json:"last_seen"`
	IsSilenced    bool      `json:"is_silenced"`
	IsSuspended   bool      `json:"is_suspended"`
}

// HandleGetFederationInstances handles GET /api/v1/admin/federation/instances
func (h *Handler) HandleGetFederationInstances(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	// Check admin authentication
	_, err := h.requireAdmin(ctx, request)
	if err != nil {
		return common.Forbidden(err), nil
	}

	// Parse query parameters
	limit := 100
	if limitStr := request.QueryStringParameters["limit"]; limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 200 {
			limit = l
		}
	}

	cursor := request.QueryStringParameters["cursor"]

	// Get known instances from storage
	instances, nextCursor, err := h.store.GetKnownInstances(ctx, limit, cursor)
	if err != nil {
		h.logger.Error("failed to get known instances", zap.Error(err))
		return common.InternalServerError(fmt.Errorf("failed to get instances")), nil
	}

	// Convert to response format
	responses := make([]InstanceInfoResponse, 0, len(instances))
	for _, instance := range instances {
		// Check if domain is blocked
		isBlocked, block, err := h.store.IsDomainBlocked(ctx, instance.Domain)
		if err != nil {
			h.logger.Warn("failed to check domain block status",
				zap.String("domain", instance.Domain),
				zap.Error(err))
		}

		resp := InstanceInfoResponse{
			Domain:        instance.Domain,
			Software:      instance.Software,
			Version:       instance.Version,
			ActiveUsers:   instance.ActiveUsers,
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
	result := map[string]interface{}{
		"instances": responses,
	}
	if nextCursor != "" {
		result["next_cursor"] = nextCursor
	}

	return common.OK(result), nil
}

// HandleGetFederationInstance handles GET /api/v1/admin/federation/instance/:domain
func (h *Handler) HandleGetFederationInstance(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	// Check admin authentication
	_, err := h.requireAdmin(ctx, request)
	if err != nil {
		return common.Forbidden(err), nil
	}

	// Get domain from path
	domain := request.PathParameters["domain"]
	if domain == "" {
		return common.BadRequest(errors.New("domain required")), nil
	}

	// Clean domain
	domain = cleanDomain(domain)

	// Get instance info from storage
	instance, err := h.store.GetInstanceInfo(ctx, domain)
	if err != nil {
		if err == storage.ErrNotFound {
			return common.NotFound(errors.New("instance not found")), nil
		}
		h.logger.Error("failed to get instance info", zap.String("domain", domain), zap.Error(err))
		return common.InternalServerError(fmt.Errorf("failed to get instance info")), nil
	}

	// Check if domain is blocked
	isBlocked, block, err := h.store.IsDomainBlocked(ctx, instance.Domain)
	if err != nil {
		h.logger.Warn("failed to check domain block status",
			zap.String("domain", instance.Domain),
			zap.Error(err))
	}

	resp := InstanceInfoResponse{
		Domain:        instance.Domain,
		Software:      instance.Software,
		Version:       instance.Version,
		ActiveUsers:   instance.ActiveUsers,
		TotalMessages: instance.TotalMessages,
		TrustScore:    instance.TrustScore,
		FirstSeen:     instance.FirstSeen,
		LastSeen:      instance.LastSeen,
	}

	if isBlocked && block != nil {
		resp.IsSilenced = block.Severity == "silence"
		resp.IsSuspended = block.Severity == "suspend"
	}

	// TODO: Add more detailed information like:
	// - Recent activity from this instance
	// - Users from this instance
	// - Moderation history

	return common.OK(resp), nil
}

// HandleGetFederationStatistics handles GET /api/v1/admin/federation/statistics
func (h *Handler) HandleGetFederationStatistics(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	// Check admin authentication
	_, err := h.requireAdmin(ctx, request)
	if err != nil {
		return common.Forbidden(err), nil
	}

	// Get time range from query parameters (default to last 7 days)
	endTime := time.Now()
	startTime := endTime.AddDate(0, 0, -7)

	if startStr := request.QueryStringParameters["start"]; startStr != "" {
		if t, err := time.Parse(time.RFC3339, startStr); err == nil {
			startTime = t
		}
	}
	if endStr := request.QueryStringParameters["end"]; endStr != "" {
		if t, err := time.Parse(time.RFC3339, endStr); err == nil {
			endTime = t
		}
	}

	// Get federation statistics from storage
	stats, err := h.store.GetFederationStatistics(ctx, startTime, endTime)
	if err != nil {
		h.logger.Error("failed to get federation statistics", zap.Error(err))
		return common.InternalServerError(fmt.Errorf("failed to get statistics")), nil
	}

	// Build response
	resp := map[string]interface{}{
		"active_instances": stats.ActiveInstances,
		"total_messages":   stats.TotalMessages,
		"total_users":      stats.TotalUsers,
		"time_range": map[string]interface{}{
			"start": startTime.Format(time.RFC3339),
			"end":   endTime.Format(time.RFC3339),
		},
	}

	return common.OK(resp), nil
}

// EmailDomainBlockResponse represents an email domain block in API responses
type EmailDomainBlockResponse struct {
	ID        string    `json:"id"`
	Domain    string    `json:"domain"`
	CreatedAt time.Time `json:"created_at"`
}

// HandleGetEmailDomainBlocks handles GET /api/v1/admin/email_domain_blocks
func (h *Handler) HandleGetEmailDomainBlocks(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	// Check admin authentication
	_, err := h.requireAdmin(ctx, request)
	if err != nil {
		return common.Forbidden(err), nil
	}

	// Parse query parameters
	limit := 100
	if limitStr := request.QueryStringParameters["limit"]; limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 200 {
			limit = l
		}
	}

	cursor := request.QueryStringParameters["cursor"]

	// Get email domain blocks from storage
	blocks, nextCursor, err := h.store.GetEmailDomainBlocks(ctx, limit, cursor)
	if err != nil {
		h.logger.Error("failed to get email domain blocks", zap.Error(err))
		return common.InternalServerError(fmt.Errorf("failed to get email domain blocks")), nil
	}

	// Convert to response format
	responses := make([]EmailDomainBlockResponse, 0, len(blocks))
	for _, block := range blocks {
		resp := EmailDomainBlockResponse{
			ID:        block.ID,
			Domain:    block.Domain,
			CreatedAt: block.CreatedAt,
		}
		responses = append(responses, resp)
	}

	// Create response with cursor for pagination
	result := map[string]interface{}{
		"blocks": responses,
	}
	if nextCursor != "" {
		result["next_cursor"] = nextCursor
	}

	return common.OK(result), nil
}

// HandleCreateEmailDomainBlock handles POST /api/v1/admin/email_domain_blocks
func (h *Handler) HandleCreateEmailDomainBlock(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	// Check admin authentication
	adminClaims, err := h.requireAdmin(ctx, request)
	if err != nil {
		return common.Forbidden(err), nil
	}

	// Parse request body
	var req struct {
		Domain string `json:"domain"`
	}
	if err := common.ParseRequestBody([]byte(request.Body), &req); err != nil {
		h.logger.Debug("invalid email domain block request", zap.Error(err))
		return common.BadRequest(errors.New("invalid request")), nil
	}

	// Validate domain
	if req.Domain == "" {
		return common.BadRequest(errors.New("domain is required")), nil
	}

	// Clean domain (remove @ if present)
	req.Domain = strings.TrimPrefix(req.Domain, "@")
	req.Domain = strings.ToLower(req.Domain)

	// Create email domain block
	block := &storage.EmailDomainBlock{
		Domain:    req.Domain,
		CreatedBy: adminClaims.Username,
	}

	if err := h.store.CreateEmailDomainBlock(ctx, block); err != nil {
		if strings.Contains(err.Error(), "already exists") {
			return common.UnprocessableEntity(errors.New("email domain block already exists")), nil
		}
		h.logger.Error("failed to create email domain block",
			zap.String("domain", req.Domain),
			zap.String("admin", adminClaims.Username),
			zap.Error(err))
		return common.InternalServerError(fmt.Errorf("failed to create email domain block")), nil
	}

	// Log admin action
	h.logger.Info("admin created email domain block",
		zap.String("domain", req.Domain),
		zap.String("admin", adminClaims.Username))

	// Convert to response format
	resp := EmailDomainBlockResponse{
		ID:        block.ID,
		Domain:    block.Domain,
		CreatedAt: block.CreatedAt,
	}

	return common.OK(resp), nil
}

// HandleDeleteEmailDomainBlock handles DELETE /api/v1/admin/email_domain_blocks/:id
func (h *Handler) HandleDeleteEmailDomainBlock(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	// Check admin authentication
	adminClaims, err := h.requireAdmin(ctx, request)
	if err != nil {
		return common.Forbidden(err), nil
	}

	// Get email domain block ID from path
	blockID := request.PathParameters["id"]
	if blockID == "" {
		return common.BadRequest(errors.New("block ID required")), nil
	}

	// Delete email domain block
	if err := h.store.DeleteEmailDomainBlock(ctx, blockID); err != nil {
		if err == storage.ErrNotFound {
			return common.NotFound(errors.New("email domain block not found")), nil
		}
		h.logger.Error("failed to delete email domain block",
			zap.String("id", blockID),
			zap.String("admin", adminClaims.Username),
			zap.Error(err))
		return common.InternalServerError(fmt.Errorf("failed to delete email domain block")), nil
	}

	// Log admin action
	h.logger.Info("admin deleted email domain block",
		zap.String("id", blockID),
		zap.String("admin", adminClaims.Username))

	// Return empty object (Mastodon compatibility)
	return common.OK(map[string]interface{}{}), nil
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

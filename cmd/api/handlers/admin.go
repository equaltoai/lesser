package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/aron23/lesser/cmd/api/models"
	"github.com/aron23/lesser/pkg/activitypub"
	"github.com/aron23/lesser/pkg/auth"
	"github.com/aron23/lesser/pkg/common"
	"github.com/aron23/lesser/pkg/storage"
	"github.com/aws/aws-lambda-go/events"
	"go.uber.org/zap"
)

// Helper function to check if user is admin
func (h *Handler) requireAdmin(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*auth.Claims, error) {
	// Extract token from Authorization header
	authHeader := request.Headers["Authorization"]
	if authHeader == "" {
		authHeader = request.Headers["authorization"]
	}

	token, err := auth.ExtractBearerToken(authHeader)
	if err != nil {
		return nil, err
	}

	// Validate token and get claims
	oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.store)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return nil, err
	}

	// Check if user is admin
	user, err := h.store.GetUser(ctx, claims.Username)
	if err != nil {
		return nil, err
	}

	if user.Role != "admin" {
		return nil, errors.New("admin access required")
	}

	return claims, nil
}

// Helper to convert activitypub.Actor to models.Account
func (h *Handler) convertActorToAccountWithCounts(ctx context.Context, actor *activitypub.Actor) models.Account {
	// Default avatar and header
	avatar := fmt.Sprintf("https://%s/avatars/default.png", h.cfg.Domain)
	header := fmt.Sprintf("https://%s/headers/default.png", h.cfg.Domain)

	if actor.Icon != nil && actor.Icon.URL != "" {
		avatar = actor.Icon.URL
	}
	if actor.Image != nil && actor.Image.URL != "" {
		header = actor.Image.URL
	}

	// Get metadata
	createdAt := time.Now() // Default fallback
	lastStatusAt := ""

	// Get actor with metadata
	_, metadata, err := h.store.GetActorWithMetadata(ctx, actor.PreferredUsername)
	if err == nil && metadata != nil {
		createdAt = metadata.CreatedAt
		if metadata.LastStatusAt != nil {
			lastStatusAt = metadata.LastStatusAt.Format(time.RFC3339)
		}
	}

	// Get counts
	statusesCount, _ := h.store.GetStatusCount(ctx, actor.ID)
	followersCount, _ := h.store.GetFollowerCount(ctx, actor.ID)

	// Get following count by checking first page
	following, _, _ := h.store.GetFollowing(ctx, actor.PreferredUsername, 1, "")
	followingCount := len(following)

	return models.Account{
		ID:             actor.PreferredUsername,
		Username:       actor.PreferredUsername,
		Acct:           actor.PreferredUsername,
		URL:            actor.URL,
		DisplayName:    actor.Name,
		Note:           actor.Summary,
		Avatar:         avatar,
		AvatarStatic:   avatar,
		Header:         header,
		HeaderStatic:   header,
		Locked:         actor.ManuallyApprovesFollowers,
		Bot:            actor.Type == "Service",
		Discoverable:   actor.Discoverable,
		CreatedAt:      createdAt.Format(time.RFC3339),
		LastStatusAt:   lastStatusAt,
		StatusesCount:  statusesCount,
		FollowersCount: followersCount,
		FollowingCount: followingCount,
	}
}

// HandleAdminGetAccounts handles GET /api/v1/admin/accounts
func (h *Handler) HandleAdminGetAccounts(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	// Check admin access
	_, err := h.requireAdmin(ctx, request)
	if err != nil {
		return common.Forbidden(err), nil
	}

	// Parse query parameters
	params := request.QueryStringParameters

	limit := 20
	if l := params["limit"]; l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 100 {
			limit = parsed
		}
	}

	cursor := params["cursor"]

	// Get users from storage
	users, nextCursor, err := h.store.ListUsers(ctx, int32(limit), cursor)
	if err != nil {
		h.logger.Error("failed to list users", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	// Convert to admin account format
	accounts := make([]models.AdminAccount, 0, len(users))
	for _, user := range users {
		// Get actor info
		actor, err := h.store.GetActor(ctx, user.Username)
		if err != nil {
			h.logger.Warn("failed to get actor for user",
				zap.String("username", user.Username),
				zap.Error(err))
			continue
		}

		// Get IP history from sessions
		var lastIP *string
		var ipHistory []models.AdminIP

		sessions, err := h.store.GetUserSessions(ctx, user.Username)
		if err == nil && len(sessions) > 0 {
			// Sort sessions by last activity (most recent first)
			sort.Slice(sessions, func(i, j int) bool {
				return sessions[i].LastActivity.After(sessions[j].LastActivity)
			})

			// Get the most recent IP
			if sessions[0].IPAddress != "" {
				lastIP = &sessions[0].IPAddress
			}

			// Build IP history (unique IPs with their usage info)
			ipMap := make(map[string]*models.AdminIP)
			for _, sess := range sessions {
				if sess.IPAddress != "" {
					if existing, ok := ipMap[sess.IPAddress]; ok {
						// Update if this session is more recent
						if sess.LastActivity.After(existing.UsedAt) {
							existing.UsedAt = sess.LastActivity
						}
					} else {
						ipMap[sess.IPAddress] = &models.AdminIP{
							IP:     sess.IPAddress,
							UsedAt: sess.LastActivity,
						}
					}
				}
			}

			// Convert map to slice
			for _, ip := range ipMap {
				ipHistory = append(ipHistory, *ip)
			}

			// Sort by most recent use
			sort.Slice(ipHistory, func(i, j int) bool {
				return ipHistory[i].UsedAt.After(ipHistory[j].UsedAt)
			})
		}

		account := models.AdminAccount{
			ID:        fmt.Sprintf("user-%s", user.Username),
			Username:  user.Username,
			Domain:    nil, // Local user
			CreatedAt: user.CreatedAt,
			Email:     user.Email,
			IP:        lastIP,
			IPs:       ipHistory,
			Locale:    user.Locale,
			Role: models.Role{
				ID:          getAdminRoleID(user.Role),
				Name:        user.Role,
				Permissions: getAdminRolePermissions(user.Role),
			},
			Confirmed: user.Approved,
			Suspended: user.Suspended,
			Silenced:  false, // TODO: Implement silencing
			Disabled:  !user.Approved,
			Approved:  user.Approved,
			Account:   h.convertActorToAccountWithCounts(ctx, actor),
		}

		accounts = append(accounts, account)
	}

	// Add Link header for pagination if there's more data
	headers := common.Headers()
	if nextCursor != "" {
		// Build next page URL
		nextURL := url.URL{
			Path: "/api/v1/admin/accounts",
			RawQuery: url.Values{
				"limit":  []string{strconv.Itoa(limit)},
				"cursor": []string{nextCursor},
			}.Encode(),
		}
		headers["Link"] = fmt.Sprintf(`<%s>; rel="next"`, nextURL.String())
	}

	// Return response with custom headers
	response := common.OK(accounts)
	for k, v := range headers {
		response.Headers[k] = v
	}
	return response, nil
}

// HandleAdminGetAccount handles GET /api/v1/admin/accounts/:id
func (h *Handler) HandleAdminGetAccount(ctx context.Context, request events.APIGatewayV2HTTPRequest, accountID string) (*events.APIGatewayV2HTTPResponse, error) {
	// Check admin access
	_, err := h.requireAdmin(ctx, request)
	if err != nil {
		return common.Forbidden(err), nil
	}

	// Extract username from account ID
	username := strings.TrimPrefix(accountID, "user-")

	// Get user from storage
	user, err := h.store.GetUser(ctx, username)
	if err != nil {
		if err == storage.ErrNotFound {
			return common.NotFound(fmt.Errorf("account not found")), nil
		}
		h.logger.Error("failed to get user", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	// Get actor info
	actor, err := h.store.GetActor(ctx, username)
	if err != nil {
		h.logger.Error("failed to get actor", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	// Get report stats
	reportStats, err := h.store.GetReportStats(ctx, username)
	if err != nil {
		h.logger.Warn("failed to get report stats", zap.Error(err))
		reportStats = &storage.ReportStats{} // Use empty stats
	}

	// Get IP history from sessions
	var lastIP *string
	var ipHistory []models.AdminIP

	sessions, err := h.store.GetUserSessions(ctx, username)
	if err == nil && len(sessions) > 0 {
		// Sort sessions by last activity (most recent first)
		sort.Slice(sessions, func(i, j int) bool {
			return sessions[i].LastActivity.After(sessions[j].LastActivity)
		})

		// Get the most recent IP
		if sessions[0].IPAddress != "" {
			lastIP = &sessions[0].IPAddress
		}

		// Build IP history (unique IPs with their usage info)
		ipMap := make(map[string]*models.AdminIP)
		for _, sess := range sessions {
			if sess.IPAddress != "" {
				if existing, ok := ipMap[sess.IPAddress]; ok {
					// Update if this session is more recent
					if sess.LastActivity.After(existing.UsedAt) {
						existing.UsedAt = sess.LastActivity
					}
				} else {
					ipMap[sess.IPAddress] = &models.AdminIP{
						IP:     sess.IPAddress,
						UsedAt: sess.LastActivity,
					}
				}
			}
		}

		// Convert map to slice
		for _, ip := range ipMap {
			ipHistory = append(ipHistory, *ip)
		}

		// Sort by most recent use
		sort.Slice(ipHistory, func(i, j int) bool {
			return ipHistory[i].UsedAt.After(ipHistory[j].UsedAt)
		})
	}

	account := models.AdminAccount{
		ID:        accountID,
		Username:  user.Username,
		Domain:    nil, // Local user
		CreatedAt: user.CreatedAt,
		Email:     user.Email,
		IP:        lastIP,
		IPs:       ipHistory,
		Locale:    user.Locale,
		Role: models.Role{
			ID:          getAdminRoleID(user.Role),
			Name:        user.Role,
			Permissions: getAdminRolePermissions(user.Role),
		},
		Confirmed: user.Approved,
		Suspended: user.Suspended,
		Silenced:  false, // TODO: Implement silencing
		Disabled:  !user.Approved,
		Approved:  user.Approved,
		Account:   h.convertActorToAccountWithCounts(ctx, actor),
		// Add report stats to account info
		ReportsCount:         reportStats.TotalReports,
		ResolvedReportsCount: reportStats.ResolvedReports,
	}

	return common.OK(account), nil
}

// HandleAdminAccountAction handles POST /api/v1/admin/accounts/:id/action
func (h *Handler) HandleAdminAccountAction(ctx context.Context, request events.APIGatewayV2HTTPRequest, accountID string) (*events.APIGatewayV2HTTPResponse, error) {
	// Check admin access
	adminClaims, err := h.requireAdmin(ctx, request)
	if err != nil {
		return common.Forbidden(err), nil
	}

	// Parse request body
	var req models.AdminAccountActionRequest
	if err := common.ParseRequestBody([]byte(request.Body), &req); err != nil {
		return common.BadRequest(err), nil
	}

	// Extract username from account ID
	username := strings.TrimPrefix(accountID, "user-")

	// Validate action type
	validActions := map[string]bool{
		"suspend":     true,
		"unsuspend":   true,
		"silence":     true,
		"unsilence":   true,
		"sensitive":   true,
		"unsensitive": true,
		"disable":     true,
		"enable":      true,
		"approve":     true,
	}

	if !validActions[req.Type] {
		return common.BadRequest(fmt.Errorf("invalid action type: %s", req.Type)), nil
	}

	// Log admin action for audit trail
	h.logger.Info("admin account action",
		zap.String("admin", adminClaims.Username),
		zap.String("target", username),
		zap.String("action", req.Type),
		zap.String("reason", req.Text))

	// Handle the action
	updates := make(map[string]interface{})

	switch req.Type {
	case "suspend":
		updates["suspended"] = true
		// TODO: Cancel all follow relationships
		// TODO: Hide all content
	case "unsuspend":
		updates["suspended"] = false
	case "disable":
		updates["approved"] = false
	case "enable", "approve":
		updates["approved"] = true
	case "silence":
		// TODO: Implement silencing
		h.logger.Warn("silence action not yet implemented")
	case "unsilence":
		// TODO: Implement unsilencing
		h.logger.Warn("unsilence action not yet implemented")
	case "sensitive":
		// TODO: Mark all media as sensitive
		h.logger.Warn("sensitive action not yet implemented")
	case "unsensitive":
		// TODO: Unmark media as sensitive
		h.logger.Warn("unsensitive action not yet implemented")
	}

	// Update user in storage
	if len(updates) > 0 {
		updates["updated_at"] = time.Now()
		if err := h.store.UpdateUser(ctx, username, updates); err != nil {
			h.logger.Error("failed to update user", zap.Error(err))
			return common.InternalServerError(err), nil
		}
	}

	// Send notification if requested
	if req.SendEmailNotification {
		// TODO: Send email notification
		h.logger.Info("would send email notification", zap.String("username", username))
	}

	// Return empty response
	return common.NoContent(), nil
}

// HandleAdminApproveAccount handles POST /api/v1/admin/accounts/:id/approve
func (h *Handler) HandleAdminApproveAccount(ctx context.Context, request events.APIGatewayV2HTTPRequest, accountID string) (*events.APIGatewayV2HTTPResponse, error) {
	// Check admin access
	adminClaims, err := h.requireAdmin(ctx, request)
	if err != nil {
		return common.Forbidden(err), nil
	}

	// Extract username from account ID
	username := strings.TrimPrefix(accountID, "user-")

	// Log admin action
	h.logger.Info("admin approve account",
		zap.String("admin", adminClaims.Username),
		zap.String("target", username))

	// Update user
	updates := map[string]interface{}{
		"approved":   true,
		"updated_at": time.Now(),
	}

	if err := h.store.UpdateUser(ctx, username, updates); err != nil {
		h.logger.Error("failed to approve user", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	// TODO: Send welcome email

	return common.NoContent(), nil
}

// HandleAdminRejectAccount handles POST /api/v1/admin/accounts/:id/reject
func (h *Handler) HandleAdminRejectAccount(ctx context.Context, request events.APIGatewayV2HTTPRequest, accountID string) (*events.APIGatewayV2HTTPResponse, error) {
	// Check admin access
	adminClaims, err := h.requireAdmin(ctx, request)
	if err != nil {
		return common.Forbidden(err), nil
	}

	// Extract username from account ID
	username := strings.TrimPrefix(accountID, "user-")

	// Log admin action
	h.logger.Info("admin reject account",
		zap.String("admin", adminClaims.Username),
		zap.String("target", username))

	// Delete the user and associated data
	if err := h.store.DeleteUser(ctx, username); err != nil {
		h.logger.Error("failed to delete user", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	// TODO: Send rejection email

	return common.NoContent(), nil
}

// HandleAdminEnableAccount handles POST /api/v1/admin/accounts/:id/enable
func (h *Handler) HandleAdminEnableAccount(ctx context.Context, request events.APIGatewayV2HTTPRequest, accountID string) (*events.APIGatewayV2HTTPResponse, error) {
	// Check admin access
	adminClaims, err := h.requireAdmin(ctx, request)
	if err != nil {
		return common.Forbidden(err), nil
	}

	// Extract username from account ID
	username := strings.TrimPrefix(accountID, "user-")

	// Log admin action
	h.logger.Info("admin enable account",
		zap.String("admin", adminClaims.Username),
		zap.String("target", username))

	// Update user
	updates := map[string]interface{}{
		"approved":   true,
		"suspended":  false,
		"updated_at": time.Now(),
	}

	if err := h.store.UpdateUser(ctx, username, updates); err != nil {
		h.logger.Error("failed to enable user", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	return common.NoContent(), nil
}

// HandleAdminUnsilenceAccount handles POST /api/v1/admin/accounts/:id/unsilence
func (h *Handler) HandleAdminUnsilenceAccount(ctx context.Context, request events.APIGatewayV2HTTPRequest, accountID string) (*events.APIGatewayV2HTTPResponse, error) {
	// Check admin access
	adminClaims, err := h.requireAdmin(ctx, request)
	if err != nil {
		return common.Forbidden(err), nil
	}

	// Extract username from account ID
	username := strings.TrimPrefix(accountID, "user-")

	// Log admin action
	h.logger.Info("admin unsilence account",
		zap.String("admin", adminClaims.Username),
		zap.String("target", username))

	// TODO: Implement silencing/unsilencing in storage
	h.logger.Warn("unsilence not yet implemented in storage layer")

	return common.NoContent(), nil
}

// HandleAdminUnsuspendAccount handles POST /api/v1/admin/accounts/:id/unsuspend
func (h *Handler) HandleAdminUnsuspendAccount(ctx context.Context, request events.APIGatewayV2HTTPRequest, accountID string) (*events.APIGatewayV2HTTPResponse, error) {
	// Check admin access
	adminClaims, err := h.requireAdmin(ctx, request)
	if err != nil {
		return common.Forbidden(err), nil
	}

	// Extract username from account ID
	username := strings.TrimPrefix(accountID, "user-")

	// Log admin action
	h.logger.Info("admin unsuspend account",
		zap.String("admin", adminClaims.Username),
		zap.String("target", username))

	// Update user
	updates := map[string]interface{}{
		"suspended":  false,
		"updated_at": time.Now(),
	}

	if err := h.store.UpdateUser(ctx, username, updates); err != nil {
		h.logger.Error("failed to unsuspend user", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	return common.NoContent(), nil
}

// HandleAdminUnsensitiveAccount handles POST /api/v1/admin/accounts/:id/unsensitive
func (h *Handler) HandleAdminUnsensitiveAccount(ctx context.Context, request events.APIGatewayV2HTTPRequest, accountID string) (*events.APIGatewayV2HTTPResponse, error) {
	// Check admin access
	adminClaims, err := h.requireAdmin(ctx, request)
	if err != nil {
		return common.Forbidden(err), nil
	}

	// Extract username from account ID
	username := strings.TrimPrefix(accountID, "user-")

	// Log admin action
	h.logger.Info("admin unsensitive account",
		zap.String("admin", adminClaims.Username),
		zap.String("target", username))

	// TODO: Implement sensitive flag in storage and remove from all media
	h.logger.Warn("unsensitive not yet implemented - needs media sensitive flag support")

	return common.NoContent(), nil
}

// Helper functions
func getAdminRoleID(role string) string {
	switch role {
	case "admin":
		return "3"
	case "moderator":
		return "2"
	default:
		return "1" // user
	}
}

func getAdminRolePermissions(role string) int {
	switch role {
	case "admin":
		return 0xFFFFFFFF // All permissions
	case "moderator":
		return 0x0000FFFF // Moderation permissions
	default:
		return 0x00000001 // Basic user permissions
	}
}

// HandleAdminGetReports handles GET /api/v1/admin/reports
func (h *Handler) HandleAdminGetReports(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	// Check admin access
	_, err := h.requireAdmin(ctx, request)
	if err != nil {
		return common.Forbidden(err), nil
	}

	// Parse query parameters
	params := request.QueryStringParameters

	limit := 20
	if l := params["limit"]; l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 100 {
			limit = parsed
		}
	}

	cursor := params["cursor"]
	status := storage.ReportStatusOpen // Default to open reports

	if s := params["status"]; s != "" {
		switch s {
		case "resolved":
			status = storage.ReportStatusResolved
		case "rejected":
			status = storage.ReportStatusRejected
		}
	}

	// Get reports from storage
	reports, nextCursor, err := h.store.GetReportsByStatus(ctx, status, limit, cursor)
	if err != nil {
		h.logger.Error("failed to get reports", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	// Convert to API format
	apiReports := make([]models.AdminReport, 0, len(reports))
	for _, report := range reports {
		// Get reporter account info
		reporterActor, err := h.store.GetActor(ctx, report.ReporterID)
		if err != nil {
			h.logger.Warn("failed to get reporter actor",
				zap.String("reporter", report.ReporterID),
				zap.Error(err))
			continue
		}

		// Get target account info
		targetActor, err := h.store.GetActor(ctx, report.TargetAccountID)
		if err != nil {
			h.logger.Warn("failed to get target actor",
				zap.String("target", report.TargetAccountID),
				zap.Error(err))
			continue
		}

		// Get assigned account if any
		var assignedAccount *models.Account
		if report.AssignedTo != "" {
			assignedActor, err := h.store.GetActor(ctx, report.AssignedTo)
			if err == nil {
				acc := h.convertActorToAccountWithCounts(ctx, assignedActor)
				assignedAccount = &acc
			}
		}

		// Get moderator account if action was taken
		var actionTakenByAccount *models.Account
		if report.ModeratorID != "" {
			moderatorActor, err := h.store.GetActor(ctx, report.ModeratorID)
			if err == nil {
				acc := h.convertActorToAccountWithCounts(ctx, moderatorActor)
				actionTakenByAccount = &acc
			}
		}

		apiReport := models.AdminReport{
			ID:                   report.ID,
			ActionTaken:          report.ActionTaken != "",
			ActionTakenAt:        report.ActionTakenAt,
			Category:             report.Category,
			Comment:              report.Comment,
			Forwarded:            report.Forwarded,
			CreatedAt:            report.CreatedAt,
			UpdatedAt:            report.UpdatedAt,
			Account:              h.convertActorToAccountWithCounts(ctx, reporterActor),
			TargetAccount:        h.convertActorToAccountWithCounts(ctx, targetActor),
			AssignedAccount:      assignedAccount,
			ActionTakenByAccount: actionTakenByAccount,
			Statuses:             []models.Status{}, // TODO: Load reported statuses
			Rules:                []models.Rule{},   // TODO: Implement rules
		}

		apiReports = append(apiReports, apiReport)
	}

	// Add Link header for pagination if there's more data
	headers := common.Headers()
	if nextCursor != "" {
		// Build next page URL
		nextURL := url.URL{
			Path: "/api/v1/admin/reports",
			RawQuery: url.Values{
				"limit":  []string{strconv.Itoa(limit)},
				"cursor": []string{nextCursor},
				"status": []string{string(status)},
			}.Encode(),
		}
		headers["Link"] = fmt.Sprintf(`<%s>; rel="next"`, nextURL.String())
	}

	body, _ := json.Marshal(apiReports)
	return &events.APIGatewayV2HTTPResponse{
		StatusCode: 200,
		Headers:    headers,
		Body:       string(body),
	}, nil
}

// HandleAdminGetReport handles GET /api/v1/admin/reports/:id
func (h *Handler) HandleAdminGetReport(ctx context.Context, request events.APIGatewayV2HTTPRequest, reportID string) (*events.APIGatewayV2HTTPResponse, error) {
	// Check admin access
	_, err := h.requireAdmin(ctx, request)
	if err != nil {
		return common.Forbidden(err), nil
	}

	// Get report from storage
	report, err := h.store.GetReport(ctx, reportID)
	if err != nil {
		if err == storage.ErrNotFound {
			return common.NotFound(fmt.Errorf("report not found")), nil
		}
		h.logger.Error("failed to get report", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	// Get reporter account info
	reporterActor, err := h.store.GetActor(ctx, report.ReporterID)
	if err != nil {
		h.logger.Error("failed to get reporter actor", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	// Get target account info
	targetActor, err := h.store.GetActor(ctx, report.TargetAccountID)
	if err != nil {
		h.logger.Error("failed to get target actor", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	// Get assigned account if any
	var assignedAccount *models.Account
	if report.AssignedTo != "" {
		assignedActor, err := h.store.GetActor(ctx, report.AssignedTo)
		if err == nil {
			acc := h.convertActorToAccountWithCounts(ctx, assignedActor)
			assignedAccount = &acc
		} else {
			h.logger.Warn("failed to get assigned actor",
				zap.String("username", report.AssignedTo),
				zap.Error(err))
		}
	}

	// Get moderator account if action was taken
	var actionTakenByAccount *models.Account
	if report.ModeratorID != "" {
		moderatorActor, err := h.store.GetActor(ctx, report.ModeratorID)
		if err == nil {
			acc := h.convertActorToAccountWithCounts(ctx, moderatorActor)
			actionTakenByAccount = &acc
		} else {
			h.logger.Warn("failed to get moderator actor",
				zap.String("username", report.ModeratorID),
				zap.Error(err))
		}
	}

	apiReport := models.AdminReport{
		ID:                   report.ID,
		ActionTaken:          report.ActionTaken != "",
		ActionTakenAt:        report.ActionTakenAt,
		Category:             report.Category,
		Comment:              report.Comment,
		Forwarded:            report.Forwarded,
		CreatedAt:            report.CreatedAt,
		UpdatedAt:            report.UpdatedAt,
		Account:              h.convertActorToAccountWithCounts(ctx, reporterActor),
		TargetAccount:        h.convertActorToAccountWithCounts(ctx, targetActor),
		AssignedAccount:      assignedAccount,
		ActionTakenByAccount: actionTakenByAccount,
		Statuses:             []models.Status{}, // TODO: Load reported statuses
		Rules:                []models.Rule{},   // TODO: Implement rules
	}

	return common.OK(apiReport), nil
}

// HandleAdminResolveReport handles POST /api/v1/admin/reports/:id/resolve
func (h *Handler) HandleAdminResolveReport(ctx context.Context, request events.APIGatewayV2HTTPRequest, reportID string) (*events.APIGatewayV2HTTPResponse, error) {
	// Check admin access
	adminClaims, err := h.requireAdmin(ctx, request)
	if err != nil {
		return common.Forbidden(err), nil
	}

	// Update report status
	err = h.store.UpdateReportStatus(ctx, reportID, storage.ReportStatusResolved, "Resolved by admin", adminClaims.Username)
	if err != nil {
		if err == storage.ErrNotFound {
			return common.NotFound(fmt.Errorf("report not found")), nil
		}
		h.logger.Error("failed to resolve report", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	// Get updated report
	report, err := h.store.GetReport(ctx, reportID)
	if err != nil {
		h.logger.Error("failed to get updated report", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	// Convert to API format (simplified for now)
	apiReport := models.AdminReport{
		ID:            report.ID,
		ActionTaken:   true,
		ActionTakenAt: report.ActionTakenAt,
		UpdatedAt:     report.UpdatedAt,
	}

	return common.OK(apiReport), nil
}

// HandleAdminReopenReport handles POST /api/v1/admin/reports/:id/reopen
func (h *Handler) HandleAdminReopenReport(ctx context.Context, request events.APIGatewayV2HTTPRequest, reportID string) (*events.APIGatewayV2HTTPResponse, error) {
	// Check admin access
	adminClaims, err := h.requireAdmin(ctx, request)
	if err != nil {
		return common.Forbidden(err), nil
	}

	// Update report status
	err = h.store.UpdateReportStatus(ctx, reportID, storage.ReportStatusOpen, "Reopened by admin", adminClaims.Username)
	if err != nil {
		if err == storage.ErrNotFound {
			return common.NotFound(fmt.Errorf("report not found")), nil
		}
		h.logger.Error("failed to reopen report", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	// Log admin action
	h.logger.Info("admin reopened report",
		zap.String("admin", adminClaims.Username),
		zap.String("report_id", reportID))

	// Return updated report
	return h.HandleAdminGetReport(ctx, request, reportID)
}

// HandleAdminAssignReport handles POST /api/v1/admin/reports/:id/assign_to_self
func (h *Handler) HandleAdminAssignReport(ctx context.Context, request events.APIGatewayV2HTTPRequest, reportID string) (*events.APIGatewayV2HTTPResponse, error) {
	// Check admin access
	adminClaims, err := h.requireAdmin(ctx, request)
	if err != nil {
		return common.Forbidden(err), nil
	}

	// Verify report exists
	_, err = h.store.GetReport(ctx, reportID)
	if err != nil {
		if err == storage.ErrNotFound {
			return common.NotFound(fmt.Errorf("report not found")), nil
		}
		h.logger.Error("failed to get report", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	// Assign report to the admin
	if err := h.store.AssignReport(ctx, reportID, adminClaims.Username); err != nil {
		h.logger.Error("failed to assign report", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	// Log admin action
	h.logger.Info("admin assigned report to self",
		zap.String("admin", adminClaims.Username),
		zap.String("report_id", reportID))

	// Return updated report
	return h.HandleAdminGetReport(ctx, request, reportID)
}

// HandleAdminUnassignReport handles POST /api/v1/admin/reports/:id/unassign
func (h *Handler) HandleAdminUnassignReport(ctx context.Context, request events.APIGatewayV2HTTPRequest, reportID string) (*events.APIGatewayV2HTTPResponse, error) {
	// Check admin access
	adminClaims, err := h.requireAdmin(ctx, request)
	if err != nil {
		return common.Forbidden(err), nil
	}

	// Verify report exists
	_, err = h.store.GetReport(ctx, reportID)
	if err != nil {
		if err == storage.ErrNotFound {
			return common.NotFound(fmt.Errorf("report not found")), nil
		}
		h.logger.Error("failed to get report", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	// Unassign the report
	if err := h.store.UnassignReport(ctx, reportID); err != nil {
		h.logger.Error("failed to unassign report", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	// Log admin action
	h.logger.Info("admin unassigned report",
		zap.String("admin", adminClaims.Username),
		zap.String("report_id", reportID))

	// Return updated report
	return h.HandleAdminGetReport(ctx, request, reportID)
}

// HandleAdminModerationOverview handles GET /api/v1/admin/moderation/overview
func (h *Handler) HandleAdminModerationOverview(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	// Check admin access
	_, err := h.requireAdmin(ctx, request)
	if err != nil {
		return common.Forbidden(err), nil
	}

	// Get moderation queue count
	queueItems, err := h.store.GetModerationQueue(ctx, &storage.ModerationFilter{Limit: 1})
	queueCount := 0
	if err == nil && len(queueItems) > 0 {
		// TODO: Get actual total count
		queueCount = 10 // Placeholder
	}

	// Count open reports
	openReports, _, err := h.store.GetReportsByStatus(ctx, storage.ReportStatusOpen, 1, "")
	openReportCount := 0
	if err == nil && len(openReports) > 0 {
		// TODO: Get actual total count
		openReportCount = 5 // Placeholder
	}

	overview := map[string]interface{}{
		"pending_reviews":   queueCount,
		"open_reports":      openReportCount,
		"active_moderators": 3,               // TODO: Count active moderators
		"recent_decisions":  []interface{}{}, // TODO: Get recent consensus decisions
		"trust_graph_health": map[string]interface{}{
			"total_relationships": 150,  // TODO: Count trust relationships
			"average_trust_score": 0.75, // TODO: Calculate average trust
			"isolated_users":      12,   // TODO: Count users with no trust relationships
		},
	}

	return common.OK(overview), nil
}

// HandleAdminGetModerationEvents handles GET /api/v1/admin/moderation/events
func (h *Handler) HandleAdminGetModerationEvents(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	// Check admin access
	_, err := h.requireAdmin(ctx, request)
	if err != nil {
		return common.Forbidden(err), nil
	}

	// Parse query parameters
	params := request.QueryStringParameters

	limit := 50
	if l := params["limit"]; l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 100 {
			limit = parsed
		}
	}

	cursor := params["cursor"]

	// Build filter from query parameters
	filter := &storage.ModerationEventFilter{}

	if eventType := params["event_type"]; eventType != "" {
		et := storage.EventType(eventType)
		filter.EventType = &et
	}

	if category := params["category"]; category != "" {
		cat := storage.Category(category)
		filter.Category = &cat
	}

	if severity := params["min_severity"]; severity != "" {
		if sev, err := strconv.Atoi(severity); err == nil {
			s := storage.Severity(sev)
			filter.MinSeverity = &s
		}
	}

	filter.ActorID = params["actor_id"]
	filter.ObjectID = params["object_id"]

	// Get moderation events
	events, nextCursor, err := h.store.GetModerationEvents(ctx, filter, limit, cursor)
	if err != nil {
		h.logger.Error("failed to get moderation events", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	// Convert to response format
	response := make([]map[string]interface{}, 0, len(events))
	for _, event := range events {
		response = append(response, map[string]interface{}{
			"id":               event.ID,
			"event_type":       event.EventType,
			"actor_id":         event.ActorID,
			"object_id":        event.ObjectID,
			"object_type":      event.ObjectType,
			"category":         event.Category,
			"severity":         event.Severity,
			"reason":           event.Reason,
			"evidence":         event.Evidence,
			"confidence_score": event.ConfidenceScore,
			"created_at":       event.Created,
		})
	}

	// Add Link header for pagination if there's more data
	headers := common.Headers()
	if nextCursor != "" {
		nextURL := fmt.Sprintf("/api/v1/admin/moderation/events?limit=%d&cursor=%s", limit, nextCursor)
		headers["Link"] = fmt.Sprintf(`<%s>; rel="next"`, nextURL)
	}

	// Return response with custom headers
	result := common.OK(response)
	for k, v := range headers {
		result.Headers[k] = v
	}
	return result, nil
}

// HandleAdminOverrideModerationEvent handles POST /api/v1/admin/moderation/events/:id/override
func (h *Handler) HandleAdminOverrideModerationEvent(ctx context.Context, request events.APIGatewayV2HTTPRequest, eventID string) (*events.APIGatewayV2HTTPResponse, error) {
	// Check admin access
	adminClaims, err := h.requireAdmin(ctx, request)
	if err != nil {
		return common.Forbidden(err), nil
	}

	// Parse request body
	var req struct {
		Decision string `json:"decision"` // "approve" or "reject"
		Reason   string `json:"reason"`
	}
	if err := common.ParseRequestBody([]byte(request.Body), &req); err != nil {
		return common.BadRequest(err), nil
	}

	// Validate decision
	if req.Decision != "approve" && req.Decision != "reject" {
		return common.BadRequest(fmt.Errorf("invalid decision: must be 'approve' or 'reject'")), nil
	}

	// Get the moderation event
	event, err := h.store.GetModerationEvent(ctx, eventID)
	if err != nil {
		if err == storage.ErrNotFound {
			return common.NotFound(fmt.Errorf("moderation event not found")), nil
		}
		h.logger.Error("failed to get moderation event", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	// Map decision to action type
	var action storage.ActionType
	switch req.Decision {
	case "approve":
		action = storage.ActionTypeNone // Approve means no action needed
	case "reject":
		// Choose action based on event severity
		switch event.Severity {
		case storage.SeverityCritical:
			action = storage.ActionTypeSuspend
		case storage.SeverityHigh:
			action = storage.ActionTypeSilence
		default:
			action = storage.ActionTypeWarning
		}
	}

	// Create admin review with override
	err = h.store.CreateAdminReview(ctx, eventID, adminClaims.Username, action, req.Reason)
	if err != nil {
		h.logger.Error("failed to create admin review", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	// Log admin action
	h.logger.Info("admin override moderation event",
		zap.String("admin", adminClaims.Username),
		zap.String("event_id", eventID),
		zap.String("decision", req.Decision),
		zap.String("action", string(action)),
		zap.String("reason", req.Reason))

	// Return success
	return common.OK(map[string]interface{}{
		"event_id": eventID,
		"decision": req.Decision,
		"action":   string(action),
		"override": true,
		"admin":    adminClaims.Username,
		"reason":   req.Reason,
	}), nil
}

// HandleAdminGetTrustGraph handles GET /api/v1/admin/moderation/trust/graph
func (h *Handler) HandleAdminGetTrustGraph(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	// Check admin access
	_, err := h.requireAdmin(ctx, request)
	if err != nil {
		return common.Forbidden(err), nil
	}

	// Get query parameters
	params := request.QueryStringParameters
	limit := 100
	if l := params["limit"]; l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 1000 {
			limit = parsed
		}
	}

	// Get all trust relationships
	trustRelationships, err := h.store.GetAllTrustRelationships(ctx, limit)
	if err != nil {
		h.logger.Error("failed to get trust relationships", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	// Build graph structure
	nodes := make(map[string]interface{})
	edges := make([]map[string]interface{}, 0)

	for _, rel := range trustRelationships {
		// Add nodes
		if _, exists := nodes[rel.TrusterID]; !exists {
			nodes[rel.TrusterID] = map[string]interface{}{
				"id":   rel.TrusterID,
				"type": "actor",
			}
		}
		if _, exists := nodes[rel.TrusteeID]; !exists {
			nodes[rel.TrusteeID] = map[string]interface{}{
				"id":   rel.TrusteeID,
				"type": "actor",
			}
		}

		// Add edge
		edges = append(edges, map[string]interface{}{
			"from":       rel.TrusterID,
			"to":         rel.TrusteeID,
			"trust":      rel.Score,
			"created_at": rel.Created,
			"updated_at": rel.Updated,
		})
	}

	// Convert nodes map to array
	nodeArray := make([]interface{}, 0, len(nodes))
	for _, node := range nodes {
		nodeArray = append(nodeArray, node)
	}

	// Return graph data
	return common.OK(map[string]interface{}{
		"nodes": nodeArray,
		"edges": edges,
		"stats": map[string]interface{}{
			"total_nodes": len(nodes),
			"total_edges": len(edges),
		},
	}), nil
}

// HandleAdminUpdateTrust handles PUT /api/v1/admin/moderation/trust/:from/:to
func (h *Handler) HandleAdminUpdateTrust(ctx context.Context, request events.APIGatewayV2HTTPRequest, fromActorID string, toActorID string) (*events.APIGatewayV2HTTPResponse, error) {
	// Check admin access
	adminClaims, err := h.requireAdmin(ctx, request)
	if err != nil {
		return common.Forbidden(err), nil
	}

	// Parse request body
	var req struct {
		Trust    float64 `json:"trust"`
		Category string  `json:"category,omitempty"` // Optional category, defaults to "general"
		Reason   string  `json:"reason"`
	}
	if err := common.ParseRequestBody([]byte(request.Body), &req); err != nil {
		return common.BadRequest(err), nil
	}

	// Validate trust value
	if req.Trust < -1.0 || req.Trust > 1.0 {
		return common.BadRequest(fmt.Errorf("trust must be between -1.0 and 1.0")), nil
	}

	// Default category if not specified
	category := "general"
	if req.Category != "" {
		category = req.Category
	}

	// Update trust relationship - using correct field names from trust.TrustRelationship
	trustRel := &storage.TrustRelationship{
		TrusterID:  fromActorID,
		TrusteeID:  toActorID,
		Category:   storage.TrustCategory(category),
		Score:      req.Trust,
		Confidence: 1.0, // Admin updates have full confidence
		Updated:    time.Now(),
	}

	if err := h.store.UpdateTrustRelationship(ctx, trustRel); err != nil {
		h.logger.Error("failed to update trust relationship", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	// Log admin action
	h.logger.Info("admin updated trust relationship",
		zap.String("admin", adminClaims.Username),
		zap.String("from", fromActorID),
		zap.String("to", toActorID),
		zap.Float64("trust", req.Trust),
		zap.String("category", category),
		zap.String("reason", req.Reason))

	// Return updated relationship
	return common.OK(map[string]interface{}{
		"from_actor_id": fromActorID,
		"to_actor_id":   toActorID,
		"trust":         req.Trust,
		"category":      category,
		"updated_by":    adminClaims.Username,
		"reason":        req.Reason,
		"updated_at":    time.Now(),
	}), nil
}

// HandleAdminGetReviewers handles GET /api/v1/admin/moderation/reviewers
func (h *Handler) HandleAdminGetReviewers(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	// Check admin access
	_, err := h.requireAdmin(ctx, request)
	if err != nil {
		return common.Forbidden(err), nil
	}

	// Get all users with moderator or admin role
	// TODO: This needs a more efficient query in the storage layer
	users, _, err := h.store.ListUsers(ctx, 1000, "") // Get all users for now
	if err != nil {
		h.logger.Error("failed to list users", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	reviewers := make([]map[string]interface{}, 0)
	for _, user := range users {
		if user.Role == "moderator" || user.Role == "admin" {
			// Get review stats for this user
			stats, err := h.store.GetReviewerStats(ctx, user.Username)
			if err != nil {
				h.logger.Warn("failed to get reviewer stats",
					zap.String("username", user.Username),
					zap.Error(err))
				stats = &storage.ReviewerStats{} // Use empty stats
			}

			reviewers = append(reviewers, map[string]interface{}{
				"id":               fmt.Sprintf("user-%s", user.Username),
				"username":         user.Username,
				"role":             user.Role,
				"total_reviews":    stats.TotalReviews,
				"accurate_reviews": stats.AccurateReviews,
				"accuracy_rate":    stats.AccuracyRate,
				"last_review_at":   stats.LastReviewAt,
			})
		}
	}

	return common.OK(map[string]interface{}{
		"reviewers": reviewers,
		"total":     len(reviewers),
	}), nil
}

// HandleAdminPromoteModerator handles POST /api/v1/admin/moderation/reviewers/:id/promote
func (h *Handler) HandleAdminPromoteModerator(ctx context.Context, request events.APIGatewayV2HTTPRequest, userID string) (*events.APIGatewayV2HTTPResponse, error) {
	// Check admin access
	adminClaims, err := h.requireAdmin(ctx, request)
	if err != nil {
		return common.Forbidden(err), nil
	}

	// Extract username from user ID
	username := strings.TrimPrefix(userID, "user-")

	// Update user role to moderator
	updates := map[string]interface{}{
		"role":       "moderator",
		"updated_at": time.Now(),
	}

	if err := h.store.UpdateUser(ctx, username, updates); err != nil {
		h.logger.Error("failed to promote user to moderator", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	// Log admin action
	h.logger.Info("admin promoted user to moderator",
		zap.String("admin", adminClaims.Username),
		zap.String("target", username))

	return common.OK(map[string]interface{}{
		"user_id":     userID,
		"username":    username,
		"new_role":    "moderator",
		"promoted_by": adminClaims.Username,
	}), nil
}

// HandleAdminDemoteModerator handles POST /api/v1/admin/moderation/reviewers/:id/demote
func (h *Handler) HandleAdminDemoteModerator(ctx context.Context, request events.APIGatewayV2HTTPRequest, userID string) (*events.APIGatewayV2HTTPResponse, error) {
	// Check admin access
	adminClaims, err := h.requireAdmin(ctx, request)
	if err != nil {
		return common.Forbidden(err), nil
	}

	// Extract username from user ID
	username := strings.TrimPrefix(userID, "user-")

	// Don't allow demoting admins
	user, err := h.store.GetUser(ctx, username)
	if err != nil {
		if err == storage.ErrNotFound {
			return common.NotFound(fmt.Errorf("user not found")), nil
		}
		h.logger.Error("failed to get user", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	if user.Role == "admin" {
		return common.BadRequest(fmt.Errorf("cannot demote admin users")), nil
	}

	// Update user role to regular user
	updates := map[string]interface{}{
		"role":       "user",
		"updated_at": time.Now(),
	}

	if err := h.store.UpdateUser(ctx, username, updates); err != nil {
		h.logger.Error("failed to demote moderator", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	// Log admin action
	h.logger.Info("admin demoted moderator to user",
		zap.String("admin", adminClaims.Username),
		zap.String("target", username))

	return common.OK(map[string]interface{}{
		"user_id":    userID,
		"username":   username,
		"new_role":   "user",
		"demoted_by": adminClaims.Username,
	}), nil
}

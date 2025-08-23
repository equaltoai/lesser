package lift

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	storageModels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/transformations"
	"github.com/google/uuid"
	"github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"
)

// contextKey is the type for context keys to avoid untyped string context keys
type contextKey string

const (
	baseURLContextKey contextKey = "baseURL"
)

// Admin action constants
const (
	actionSuspend = "suspend"
	actionApprove = "approve"
	actionSilence = "silence"
	actionEnable  = "enable"
	actionReject  = "reject"

	roleAdmin     = "admin"
	roleModerator = "moderator"

	// ActivityPub actor types
	actorTypeService = "Service"
	actorTypeGroup   = "Group"
)

// requireAdminLift is already defined in admin_federation.go

// processUserSessions processes user sessions to extract IP history and most recent IP
func processUserSessions(sessions []*storage.Session) (lastIP *string, ipHistory []models.AdminIP) {
	if err := common.ValidateSliceNotEmpty("sessions", sessions); err != nil {
		return nil, nil
	}

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

	return lastIP, ipHistory
}

// adminAccountAction performs a generic admin account action with logging and validation
func (h *Handler) adminAccountAction(ctx *lift.Context, action string, actionFn func(username string) error) error {
	// Check admin access
	adminClaims, err := h.requireAdminLift(ctx)
	if err != nil {
		ctx.Status(http.StatusForbidden)
		return ctx.JSON(map[string]string{"error": err.Error()})
	}

	accountID := ctx.Param("id")
	// Extract username from account ID
	username := strings.TrimPrefix(accountID, "user-")

	// Log admin action
	h.logger.Info("admin "+action+" account",
		zap.String(roleAdmin, adminClaims.Username),
		zap.String("target", username))

	// Execute the action
	if err := actionFn(username); err != nil {
		h.logger.Error("failed to "+action+" account", zap.Error(err))
		ctx.Status(http.StatusInternalServerError)
		return ctx.JSON(map[string]string{"error": err.Error()})
	}

	ctx.Status(http.StatusNoContent)
	return nil
}

// adminAction performs a generic admin action with logging and validation
func (h *Handler) adminAction(ctx *lift.Context, action string, updatesFn func(username string) (map[string]any, error)) error {
	// Check admin access
	adminClaims, err := h.requireAdminLift(ctx)
	if err != nil {
		ctx.Status(http.StatusForbidden)
		return ctx.JSON(map[string]string{"error": err.Error()})
	}

	accountID := ctx.Param("id")
	// Extract username from account ID
	username := strings.TrimPrefix(accountID, "user-")

	// Log admin action
	h.logger.Info("admin "+action+" account",
		zap.String(roleAdmin, adminClaims.Username),
		zap.String("target", username))

	// Get updates from callback
	updates, err := updatesFn(username)
	if err != nil {
		h.logger.Error("failed to execute "+action, zap.Error(err))
		ctx.Status(http.StatusInternalServerError)
		return ctx.JSON(map[string]string{"error": err.Error()})
	}

	// Update user if we have updates  
	if len(updates) > 0 {
		updates["updated_at"] = time.Now()
		if err := h.repos.Account().UpdateUser(ctx.Context, username, updates); err != nil {
			h.logger.Error("failed to "+action+" user", zap.Error(err))
			ctx.Status(http.StatusInternalServerError)
			return ctx.JSON(map[string]string{"error": err.Error()})
		}
	}

	ctx.Status(http.StatusNoContent)
	return nil
}

// HandleAdminGetAccountsLift handles GET /api/v1/admin/accounts
func (h *Handler) HandleAdminGetAccountsLift(ctx *lift.Context) error {
	// Check admin access
	_, err := h.requireAdminLift(ctx)
	if err != nil {
		ctx.Status(http.StatusForbidden)
		return ctx.JSON(map[string]string{"error": err.Error()})
	}

	// Parse query parameters
	limit, err := common.ParseAdminLimit(ctx.Query("limit"))
	if err != nil {
		ctx.Status(http.StatusBadRequest)
		return ctx.JSON(map[string]string{"error": err.Error()})
	}

	cursor := ctx.Query("cursor")

	// Get users from storage
	// Safely convert limit to int32, capping at max int32 if needed
	var safeLimit32 int32
	if limit > int(^int32(0)) {
		safeLimit32 = ^int32(0) // Max int32 value
	} else if limit < 0 {
		safeLimit32 = 0
	} else {
		safeLimit32 = int32(limit)
	}
	users, nextCursor, err := h.repos.User().ListUsers(ctx.Context, safeLimit32, cursor)
	if err != nil {
		h.logger.Error("failed to list users", zap.Error(err))
		ctx.Status(http.StatusInternalServerError)
		return ctx.JSON(map[string]string{"error": err.Error()})
	}

	// Convert to admin account format
	accounts := make([]models.AdminAccount, 0, len(users))
	for _, user := range users {
		// Get actor info
		actor, err := h.repos.Actor().GetActor(ctx.Context, user.Username)
		if err != nil {
			h.logger.Warn("failed to get actor for user",
				zap.String("username", user.Username),
				zap.Error(err))
			continue
		}

		// Get IP history from sessions
		sessions, err := h.repos.Account().GetUserSessions(ctx.Context, user.Username)
		var lastIP *string
		var ipHistory []models.AdminIP
		if err == nil {
			lastIP, ipHistory = processUserSessions(sessions)
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
			Silenced:  user.Silenced,
			Disabled:  !user.Approved,
			Approved:  user.Approved,
			Account:   h.convertActorToAccountWithCounts(ctx.Context, actor),
		}

		accounts = append(accounts, account)
	}

	// Add Link header for pagination if there's more data
	if nextCursor != "" {
		// Build next page URL
		nextURL := url.URL{
			Path: "/api/v1/admin/accounts",
			RawQuery: url.Values{
				"limit":  []string{strconv.Itoa(limit)},
				"cursor": []string{nextCursor},
			}.Encode(),
		}
		ctx.Response.Headers["Link"] = fmt.Sprintf(`<%s>; rel="next"`, nextURL.String())
	}

	ctx.Status(http.StatusOK)
	return ctx.JSON(accounts)
}

// HandleAdminGetAccountLift handles GET /api/v1/admin/accounts/:id
func (h *Handler) HandleAdminGetAccountLift(ctx *lift.Context) error {
	// Check admin access
	_, err := h.requireAdminLift(ctx)
	if err != nil {
		ctx.Status(http.StatusForbidden)
		return ctx.JSON(map[string]string{"error": err.Error()})
	}

	accountID := ctx.Param("id")

	// Extract username from account ID
	username := strings.TrimPrefix(accountID, "user-")

	// Get user from storage
	user, err := h.repos.Account().GetUser(ctx.Context, username)
	if err != nil {
		if err == storage.ErrNotFound {
			ctx.Status(http.StatusNotFound)
			return ctx.JSON(map[string]string{"error": "account not found"})
		}
		h.logger.Error("failed to get user", zap.Error(err))
		ctx.Status(http.StatusInternalServerError)
		return ctx.JSON(map[string]string{"error": err.Error()})
	}

	// Get actor info
	actor, err := h.repos.Actor().GetActor(ctx.Context, username)
	if err != nil {
		h.logger.Error("failed to get actor", zap.Error(err))
		ctx.Status(http.StatusInternalServerError)
		return ctx.JSON(map[string]string{"error": err.Error()})
	}

	// Get report stats
	reportStats, err := h.repos.Moderation().GetReportStats(ctx.Context, username)
	if err != nil {
		h.logger.Warn("failed to get report stats", zap.Error(err))
		reportStats = &storage.ReportStats{} // Use empty stats
	}

	// Get IP history from sessions
	sessions, err := h.repos.Account().GetUserSessions(ctx.Context, username)
	var lastIP *string
	var ipHistory []models.AdminIP
	if err == nil {
		lastIP, ipHistory = processUserSessions(sessions)
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
		Silenced:  user.Silenced,
		Disabled:  !user.Approved,
		Approved:  user.Approved,
		Account:   h.convertActorToAccountWithCounts(ctx.Context, actor),
		// Add report stats to account info
		ReportsCount:         reportStats.TotalReports,
		ResolvedReportsCount: reportStats.ResolvedReports,
	}

	ctx.Status(http.StatusOK)
	return ctx.JSON(account)
}

// HandleAdminAccountActionLift handles POST /api/v1/admin/accounts/:id/action
func (h *Handler) HandleAdminAccountActionLift(ctx *lift.Context) error {
	// Check admin access
	adminClaims, err := h.requireAdminLift(ctx)
	if err != nil {
		ctx.Status(http.StatusForbidden)
		return ctx.JSON(map[string]string{"error": err.Error()})
	}

	accountID := ctx.Param("id")

	// Parse request body
	var req models.AdminAccountActionRequest
	if err := ctx.ParseRequest(&req); err != nil {
		ctx.Status(http.StatusBadRequest)
		return ctx.JSON(map[string]string{"error": err.Error()})
	}

	// Extract username from account ID
	username := strings.TrimPrefix(accountID, "user-")

	// Validate action type
	validActions := map[string]bool{
		actionSuspend: true,
		"unsuspend":   true,
		actionSilence: true,
		"unsilence":   true,
		"sensitive":   true,
		"unsensitive": true,
		"disable":     true,
		actionEnable:  true,
		actionApprove: true,
	}

	if !validActions[req.Type] {
		ctx.Status(http.StatusBadRequest)
		return ctx.JSON(map[string]string{"error": fmt.Sprintf("invalid action type: %s", req.Type)})
	}

	// Log admin action for audit trail
	h.logger.Info("admin account action",
		zap.String(roleAdmin, adminClaims.Username),
		zap.String("target", username),
		zap.String("action", req.Type),
		zap.String("reason", req.Text))

	// Handle the action
	updates := make(map[string]any)

	switch req.Type {
	case actionSuspend:
		updates["suspended"] = true
		// Cancel all follow relationships when suspending
		if err := h.cancelUserFollowRelationships(ctx.Context, username); err != nil {
			h.logger.Error("failed to cancel follow relationships", zap.Error(err))
		}
	case "unsuspend":
		updates["suspended"] = false
	case "disable":
		updates["approved"] = false
	case actionEnable, actionApprove:
		updates["approved"] = true
	case actionSilence:
		updates["silenced"] = true
	case "unsilence":
		updates["silenced"] = false
	case "sensitive":
		// Mark all media as sensitive
		if err := h.markAllUserMediaAsSensitive(ctx.Context, username); err != nil {
			h.logger.Error("failed to mark media as sensitive", zap.Error(err))
		}
		updates["sensitive_media"] = true
	case "unsensitive":
		// Unmark media as sensitive
		if err := h.repos.Media().UnmarkAllMediaAsSensitive(ctx.Context, username); err != nil {
			h.logger.Error("failed to unmark media as sensitive", zap.Error(err))
		}
		updates["sensitive_media"] = false
	}

	// Update user in storage  
	if len(updates) > 0 {
		updates["updated_at"] = time.Now()
		if err := h.repos.Account().UpdateUser(ctx.Context, username, updates); err != nil {
			h.logger.Error("failed to update user", zap.Error(err))
			ctx.Status(http.StatusInternalServerError)
			return ctx.JSON(map[string]string{"error": err.Error()})
		}
	}

	// Lesser does not support email functionality

	// Return empty response
	ctx.Status(http.StatusNoContent)
	return nil
}

// HandleAdminApproveAccountLift handles POST /api/v1/admin/accounts/:id/approve
func (h *Handler) HandleAdminApproveAccountLift(ctx *lift.Context) error {
	return h.adminAction(ctx, actionApprove, func(_ string) (map[string]any, error) {
		return map[string]any{"approved": true}, nil
	})
}

// HandleAdminRejectAccountLift handles POST /api/v1/admin/accounts/:id/reject
func (h *Handler) HandleAdminRejectAccountLift(ctx *lift.Context) error {
	return h.adminAccountAction(ctx, "reject", func(username string) error {
		// Delete the user and associated data
		return h.repos.Account().DeleteAccount(ctx.Context, username)
	})
}

// HandleAdminEnableAccountLift handles POST /api/v1/admin/accounts/:id/enable
func (h *Handler) HandleAdminEnableAccountLift(ctx *lift.Context) error {
	// Check admin access
	adminClaims, err := h.requireAdminLift(ctx)
	if err != nil {
		ctx.Status(http.StatusForbidden)
		return ctx.JSON(map[string]string{"error": err.Error()})
	}

	accountID := ctx.Param("id")

	// Extract username from account ID
	username := strings.TrimPrefix(accountID, "user-")

	// Log admin action
	h.logger.Info("admin enable account",
		zap.String(roleAdmin, adminClaims.Username),
		zap.String("target", username))

	// Update user
	updates := map[string]any{
		"approved":   true,
		"suspended":  false,
		"updated_at": time.Now(),
	}

	if err := h.repos.Account().UpdateUser(ctx.Context, username, updates); err != nil {
		h.logger.Error("failed to enable user", zap.Error(err))
		ctx.Status(http.StatusInternalServerError)
		return ctx.JSON(map[string]string{"error": err.Error()})
	}

	ctx.Status(http.StatusNoContent)
	return nil
}

// HandleAdminUnsilenceAccountLift handles POST /api/v1/admin/accounts/:id/unsilence
func (h *Handler) HandleAdminUnsilenceAccountLift(ctx *lift.Context) error {
	return h.adminAction(ctx, "unsilence", func(_ string) (map[string]any, error) {
		return map[string]any{"silenced": false}, nil
	})
}

// HandleAdminUnsuspendAccountLift handles POST /api/v1/admin/accounts/:id/unsuspend
func (h *Handler) HandleAdminUnsuspendAccountLift(ctx *lift.Context) error {
	return h.adminAction(ctx, "unsuspend", func(_ string) (map[string]any, error) {
		return map[string]any{"suspended": false}, nil
	})
}

// HandleAdminUnsensitiveAccountLift handles POST /api/v1/admin/accounts/:id/unsensitive
func (h *Handler) HandleAdminUnsensitiveAccountLift(ctx *lift.Context) error {
	return h.adminAccountAction(ctx, "unsensitive", func(username string) error {
		// Remove sensitive flag from all media
		return h.repos.Media().UnmarkAllMediaAsSensitive(ctx.Context, username)
	})
}

// HandleAdminGetReportsLift handles GET /api/v1/admin/reports
func (h *Handler) HandleAdminGetReportsLift(ctx *lift.Context) error {
	// Check admin access
	_, err := h.requireAdminLift(ctx)
	if err != nil {
		ctx.Status(http.StatusForbidden)
		return ctx.JSON(map[string]string{"error": err.Error()})
	}

	// Parse query parameters
	limit, err := common.ParseAdminLimit(ctx.Query("limit"))
	if err != nil {
		ctx.Status(http.StatusBadRequest)
		return ctx.JSON(map[string]string{"error": err.Error()})
	}

	cursor := ctx.Query("cursor")
	status := storage.ReportStatusOpen // Default to open reports

	if s := ctx.Query("status"); s != "" {
		switch s {
		case "resolved":
			status = storage.ReportStatusResolved
		case "rejected":
			status = storage.ReportStatusRejected
		}
	}

	// Get reports from storage
	reports, nextCursor, err := h.repos.Moderation().GetReportsByStatus(ctx.Context, status, limit, cursor)
	if err != nil {
		h.logger.Error("failed to get reports", zap.Error(err))
		ctx.Status(http.StatusInternalServerError)
		return ctx.JSON(map[string]string{"error": err.Error()})
	}

	// Convert to API format
	apiReports := make([]models.AdminReport, 0, len(reports))
	for _, report := range reports {
		// Get reporter account info
		reporterActor, err := h.repos.Actor().GetActor(ctx.Context, report.ReporterID)
		if err != nil {
			h.logger.Warn("failed to get reporter actor",
				zap.String("reporter", report.ReporterID),
				zap.Error(err))
			continue
		}

		// Get target account info
		targetActor, err := h.repos.Actor().GetActor(ctx.Context, report.TargetAccountID)
		if err != nil {
			h.logger.Warn("failed to get target actor",
				zap.String("target", report.TargetAccountID),
				zap.Error(err))
			continue
		}

		// Get assigned account if any
		var assignedAccount *models.Account
		if report.AssignedTo != "" {
			assignedActor, err := h.repos.Actor().GetActor(ctx.Context, report.AssignedTo)
			if err == nil {
				acc := h.convertActorToAccountWithCounts(ctx.Context, assignedActor)
				assignedAccount = &acc
			}
		}

		// Get moderator account if action was taken
		var actionTakenByAccount *models.Account
		if report.ModeratorID != "" {
			moderatorActor, err := h.repos.Actor().GetActor(ctx.Context, report.ModeratorID)
			if err == nil {
				acc := h.convertActorToAccountWithCounts(ctx.Context, moderatorActor)
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
			Account:              h.convertActorToAccountWithCounts(ctx.Context, reporterActor),
			TargetAccount:        h.convertActorToAccountWithCounts(ctx.Context, targetActor),
			AssignedAccount:      assignedAccount,
			ActionTakenByAccount: actionTakenByAccount,
			Statuses:             h.loadReportedStatuses(ctx.Context, report.ID),
			Rules:                h.loadViolatedRules(ctx.Context, report.Category),
		}

		apiReports = append(apiReports, apiReport)
	}

	// Add Link header for pagination if there's more data
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
		ctx.Response.Headers["Link"] = fmt.Sprintf(`<%s>; rel="next"`, nextURL.String())
	}

	ctx.Status(http.StatusOK)
	return ctx.JSON(apiReports)
}

// HandleAdminGetReportLift handles GET /api/v1/admin/reports/:id
func (h *Handler) HandleAdminGetReportLift(ctx *lift.Context) error {
	// Check admin access
	_, err := h.requireAdminLift(ctx)
	if err != nil {
		ctx.Status(http.StatusForbidden)
		return ctx.JSON(map[string]string{"error": err.Error()})
	}

	reportID := ctx.Param("id")

	// Get report from storage
	report, err := h.repos.Moderation().GetReport(ctx.Context, reportID)
	if err != nil {
		if err == storage.ErrNotFound {
			ctx.Status(http.StatusNotFound)
			return ctx.JSON(map[string]string{"error": "report not found"})
		}
		h.logger.Error("failed to get report", zap.Error(err))
		ctx.Status(http.StatusInternalServerError)
		return ctx.JSON(map[string]string{"error": err.Error()})
	}

	// Get reporter account info
	reporterActor, err := h.repos.Actor().GetActor(ctx.Context, report.ReporterID)
	if err != nil {
		h.logger.Error("failed to get reporter actor", zap.Error(err))
		ctx.Status(http.StatusInternalServerError)
		return ctx.JSON(map[string]string{"error": err.Error()})
	}

	// Get target account info
	targetActor, err := h.repos.Actor().GetActor(ctx.Context, report.TargetAccountID)
	if err != nil {
		h.logger.Error("failed to get target actor", zap.Error(err))
		ctx.Status(http.StatusInternalServerError)
		return ctx.JSON(map[string]string{"error": err.Error()})
	}

	// Get assigned account if any
	var assignedAccount *models.Account
	if report.AssignedTo != "" {
		assignedActor, err := h.repos.Actor().GetActor(ctx.Context, report.AssignedTo)
		if err == nil {
			acc := h.convertActorToAccountWithCounts(ctx.Context, assignedActor)
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
		moderatorActor, err := h.repos.Actor().GetActor(ctx.Context, report.ModeratorID)
		if err == nil {
			acc := h.convertActorToAccountWithCounts(ctx.Context, moderatorActor)
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
		Account:              h.convertActorToAccountWithCounts(ctx.Context, reporterActor),
		TargetAccount:        h.convertActorToAccountWithCounts(ctx.Context, targetActor),
		AssignedAccount:      assignedAccount,
		ActionTakenByAccount: actionTakenByAccount,
		Statuses:             h.loadReportedStatuses(ctx.Context, report.ID),
		Rules:                h.loadViolatedRules(ctx.Context, report.Category),
	}

	ctx.Status(http.StatusOK)
	return ctx.JSON(apiReport)
}

// HandleAdminResolveReportLift handles POST /api/v1/admin/reports/:id/resolve
func (h *Handler) HandleAdminResolveReportLift(ctx *lift.Context) error {
	// Check admin access
	adminClaims, err := h.requireAdminLift(ctx)
	if err != nil {
		ctx.Status(http.StatusForbidden)
		return ctx.JSON(map[string]string{"error": err.Error()})
	}

	reportID := ctx.Param("id")

	// Update report status
	err = h.repos.Moderation().UpdateReportStatus(ctx.Context, reportID, storage.ReportStatusResolved, "Resolved by admin", adminClaims.Username)
	if err != nil {
		if err == storage.ErrNotFound {
			ctx.Status(http.StatusNotFound)
			return ctx.JSON(map[string]string{"error": "report not found"})
		}
		h.logger.Error("failed to resolve report", zap.Error(err))
		ctx.Status(http.StatusInternalServerError)
		return ctx.JSON(map[string]string{"error": err.Error()})
	}

	// Get updated report
	report, err := h.repos.Moderation().GetReport(ctx.Context, reportID)
	if err != nil {
		h.logger.Error("failed to get updated report", zap.Error(err))
		ctx.Status(http.StatusInternalServerError)
		return ctx.JSON(map[string]string{"error": err.Error()})
	}

	// Convert to API format (simplified for now)
	apiReport := models.AdminReport{
		ID:            report.ID,
		ActionTaken:   true,
		ActionTakenAt: report.ActionTakenAt,
		UpdatedAt:     report.UpdatedAt,
	}

	ctx.Status(http.StatusOK)
	return ctx.JSON(apiReport)
}

// HandleAdminReopenReportLift handles POST /api/v1/admin/reports/:id/reopen
func (h *Handler) HandleAdminReopenReportLift(ctx *lift.Context) error {
	// Check admin access
	adminClaims, err := h.requireAdminLift(ctx)
	if err != nil {
		ctx.Status(http.StatusForbidden)
		return ctx.JSON(map[string]string{"error": err.Error()})
	}

	reportID := ctx.Param("id")

	// Update report status
	err = h.repos.Moderation().UpdateReportStatus(ctx.Context, reportID, storage.ReportStatusOpen, "Reopened by admin", adminClaims.Username)
	if err != nil {
		if err == storage.ErrNotFound {
			ctx.Status(http.StatusNotFound)
			return ctx.JSON(map[string]string{"error": "report not found"})
		}
		h.logger.Error("failed to reopen report", zap.Error(err))
		ctx.Status(http.StatusInternalServerError)
		return ctx.JSON(map[string]string{"error": err.Error()})
	}

	// Log admin action
	h.logger.Info("admin reopened report",
		zap.String(roleAdmin, adminClaims.Username),
		zap.String("report_id", reportID))

	// Return updated report
	return h.HandleAdminGetReportLift(ctx)
}

// HandleAdminAssignReportLift handles POST /api/v1/admin/reports/:id/assign_to_self
func (h *Handler) HandleAdminAssignReportLift(ctx *lift.Context) error {
	// Check admin access
	adminClaims, err := h.requireAdminLift(ctx)
	if err != nil {
		ctx.Status(http.StatusForbidden)
		return ctx.JSON(map[string]string{"error": err.Error()})
	}

	reportID := ctx.Param("id")

	// Verify report exists
	_, err = h.repos.Moderation().GetReport(ctx.Context, reportID)
	if err != nil {
		if err == storage.ErrNotFound {
			ctx.Status(http.StatusNotFound)
			return ctx.JSON(map[string]string{"error": "report not found"})
		}
		h.logger.Error("failed to get report", zap.Error(err))
		ctx.Status(http.StatusInternalServerError)
		return ctx.JSON(map[string]string{"error": err.Error()})
	}

	// Assign report to the admin
	if err := h.repos.Moderation().AssignReport(ctx.Context, reportID, adminClaims.Username); err != nil {
		h.logger.Error("failed to assign report", zap.Error(err))
		ctx.Status(http.StatusInternalServerError)
		return ctx.JSON(map[string]string{"error": err.Error()})
	}

	// Log admin action
	h.logger.Info("admin assigned report to self",
		zap.String(roleAdmin, adminClaims.Username),
		zap.String("report_id", reportID))

	// Return updated report
	return h.HandleAdminGetReportLift(ctx)
}

// HandleAdminUnassignReportLift handles POST /api/v1/admin/reports/:id/unassign
func (h *Handler) HandleAdminUnassignReportLift(ctx *lift.Context) error {
	// Check admin access
	adminClaims, err := h.requireAdminLift(ctx)
	if err != nil {
		ctx.Status(http.StatusForbidden)
		return ctx.JSON(map[string]string{"error": err.Error()})
	}

	reportID := ctx.Param("id")

	// Verify report exists
	_, err = h.repos.Moderation().GetReport(ctx.Context, reportID)
	if err != nil {
		if err == storage.ErrNotFound {
			ctx.Status(http.StatusNotFound)
			return ctx.JSON(map[string]string{"error": "report not found"})
		}
		h.logger.Error("failed to get report", zap.Error(err))
		ctx.Status(http.StatusInternalServerError)
		return ctx.JSON(map[string]string{"error": err.Error()})
	}

	// Unassign the report
	if err := h.repos.Moderation().UnassignReport(ctx.Context, reportID); err != nil {
		h.logger.Error("failed to unassign report", zap.Error(err))
		ctx.Status(http.StatusInternalServerError)
		return ctx.JSON(map[string]string{"error": err.Error()})
	}

	// Log admin action
	h.logger.Info("admin unassigned report",
		zap.String(roleAdmin, adminClaims.Username),
		zap.String("report_id", reportID))

	// Return updated report
	return h.HandleAdminGetReportLift(ctx)
}

// HandleAdminModerationOverviewLift handles GET /api/v1/admin/moderation/overview
func (h *Handler) HandleAdminModerationOverviewLift(ctx *lift.Context) error {
	// Check admin access
	_, err := h.requireAdminLift(ctx)
	if err != nil {
		ctx.Status(http.StatusForbidden)
		return ctx.JSON(map[string]string{"error": err.Error()})
	}

	// Get moderation queue count
	queueItems, err := h.repos.Moderation().GetModerationQueue(ctx.Context, &storage.ModerationFilter{Limit: 1})
	queueCount := 0
	if err == nil && len(queueItems) > 0 {
		// Get actual total count from moderation queue
		queueCount, err = h.repos.Moderation().GetModerationQueueCount(ctx.Context)
		if err != nil {
			h.logger.Warn("failed to get queue count", zap.Error(err))
			queueCount = 0
		}
	}

	// Count open reports
	openReports, _, err := h.repos.Moderation().GetReportsByStatus(ctx.Context, storage.ReportStatusOpen, 1, "")
	openReportCount := 0
	if err == nil && len(openReports) > 0 {
		// Get actual total count of open reports
		openReportCount, err = h.repos.Moderation().GetOpenReportsCount(ctx.Context)
		if err != nil {
			h.logger.Warn("failed to get open reports count", zap.Error(err))
			openReportCount = 0
		}
	}

	overview := map[string]any{
		"pending_reviews":    queueCount,
		"open_reports":       openReportCount,
		"active_moderators":  h.getActiveModeratorsCount(ctx.Context),
		"recent_decisions":   h.getRecentConsensusDecisions(ctx.Context),
		"trust_graph_health": h.getTrustGraphHealth(ctx.Context),
	}

	ctx.Status(http.StatusOK)
	return ctx.JSON(overview)
}

// HandleAdminGetModerationEventsLift handles GET /api/v1/admin/moderation/events
func (h *Handler) HandleAdminGetModerationEventsLift(ctx *lift.Context) error {
	// Check admin access
	_, err := h.requireAdminLift(ctx)
	if err != nil {
		ctx.Status(http.StatusForbidden)
		return ctx.JSON(map[string]string{"error": err.Error()})
	}

	// Parse query parameters
	limit, err := common.ParseAdminLimit(ctx.Query("limit"))
	if err != nil {
		ctx.Status(http.StatusBadRequest)
		return ctx.JSON(map[string]string{"error": err.Error()})
	}
	if limit == 20 { // Default from ParseAdminLimit, override for this endpoint
		limit = 50
	}

	cursor := ctx.Query("cursor")

	// Build filter from query parameters
	filter := &storage.ModerationEventFilter{}

	if eventType := ctx.Query("event_type"); eventType != "" {
		filter.EventType = eventType
	}

	if category := ctx.Query("category"); category != "" {
		filter.Category = category
	}

	if severity := ctx.Query("min_severity"); severity != "" {
		if sev, err := common.ParseAndValidateIntWithBounds("min_severity", severity, 0, 5, 0); err == nil {
			filter.MinSeverity = &sev
		}
	}

	filter.ActorID = ctx.Query("actor_id")
	filter.ObjectID = ctx.Query("object_id")

	// Get moderation events
	events, nextCursor, err := h.repos.Moderation().GetModerationEvents(ctx.Context, filter, limit, cursor)
	if err != nil {
		h.logger.Error("failed to get moderation events", zap.Error(err))
		ctx.Status(http.StatusInternalServerError)
		return ctx.JSON(map[string]string{"error": err.Error()})
	}

	// Convert to response format
	response := make([]map[string]any, 0, len(events))
	for _, event := range events {
		response = append(response, map[string]any{
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
	if nextCursor != "" {
		nextURL := fmt.Sprintf("/api/v1/admin/moderation/events?limit=%d&cursor=%s", limit, nextCursor)
		ctx.Response.Headers["Link"] = fmt.Sprintf(`<%s>; rel="next"`, nextURL)
	}

	ctx.Status(http.StatusOK)
	return ctx.JSON(response)
}

// HandleAdminOverrideModerationEventLift handles POST /api/v1/admin/moderation/events/:id/override
func (h *Handler) HandleAdminOverrideModerationEventLift(ctx *lift.Context) error {
	// Check admin access
	adminClaims, err := h.requireAdminLift(ctx)
	if err != nil {
		ctx.Status(http.StatusForbidden)
		return ctx.JSON(map[string]string{"error": err.Error()})
	}

	eventID := ctx.Param("id")

	// Parse request body
	var req struct {
		Decision string `json:"decision"` // actionApprove or actionReject
		Reason   string `json:"reason"`
	}
	if err := ctx.ParseRequest(&req); err != nil {
		ctx.Status(http.StatusBadRequest)
		return ctx.JSON(map[string]string{"error": err.Error()})
	}

	// Validate decision
	if req.Decision != actionApprove && req.Decision != actionReject {
		ctx.Status(http.StatusBadRequest)
		return ctx.JSON(map[string]string{"error": "invalid decision: must be 'approve' or 'reject'"})
	}

	// Get the moderation event
	event, err := h.repos.Moderation().GetModerationEvent(ctx.Context, eventID)
	if err != nil {
		if err == storage.ErrNotFound {
			ctx.Status(http.StatusNotFound)
			return ctx.JSON(map[string]string{"error": "moderation event not found"})
		}
		h.logger.Error("failed to get moderation event", zap.Error(err))
		ctx.Status(http.StatusInternalServerError)
		return ctx.JSON(map[string]string{"error": err.Error()})
	}

	// Map decision to action type
	var action storage.ActionType
	switch req.Decision {
	case actionApprove:
		action = storage.ActionTypeNone // Approve means no action needed
	case actionReject:
		// Choose action based on event severity
		switch event.Severity {
		case "4": // Critical
			action = storage.ActionTypeSuspend
		case "3": // High
			action = storage.ActionTypeSilence
		default:
			action = storage.ActionTypeWarning
		}
	}

	// Create admin review with override
	err = h.repos.Moderation().CreateAdminReview(ctx.Context, eventID, adminClaims.Username, action, req.Reason)
	if err != nil {
		h.logger.Error("failed to create admin review", zap.Error(err))
		ctx.Status(http.StatusInternalServerError)
		return ctx.JSON(map[string]string{"error": err.Error()})
	}

	// Log admin action
	h.logger.Info("admin override moderation event",
		zap.String(roleAdmin, adminClaims.Username),
		zap.String("event_id", eventID),
		zap.String("decision", req.Decision),
		zap.String("action", string(action)),
		zap.String("reason", req.Reason))

	// Return success
	ctx.Status(http.StatusOK)
	return ctx.JSON(map[string]any{
		"event_id": eventID,
		"decision": req.Decision,
		"action":   string(action),
		"override": true,
		"admin":    adminClaims.Username,
		"reason":   req.Reason,
	})
}

// HandleAdminGetTrustGraphLift handles GET /api/v1/admin/moderation/trust/graph
func (h *Handler) HandleAdminGetTrustGraphLift(ctx *lift.Context) error {
	// Check admin access
	_, err := h.requireAdminLift(ctx)
	if err != nil {
		ctx.Status(http.StatusForbidden)
		return ctx.JSON(map[string]string{"error": err.Error()})
	}

	// Get query parameters
	limit, err := common.ParseFederationLimit(ctx.Query("limit"))
	if err != nil {
		ctx.Status(http.StatusBadRequest)
		return ctx.JSON(map[string]string{"error": err.Error()})
	}
	if limit == 20 { // Default from ParseFederationLimit, override for this endpoint
		limit = 100
	}

	// Get all trust relationships
	trustRelationships, err := h.repos.Trust().GetAllTrustRelationships(ctx.Context, limit)
	if err != nil {
		h.logger.Error("failed to get trust relationships", zap.Error(err))
		ctx.Status(http.StatusInternalServerError)
		return ctx.JSON(map[string]string{"error": err.Error()})
	}

	// Build graph structure
	nodes := make(map[string]any)
	edges := make([]map[string]any, 0)

	for _, rel := range trustRelationships {
		// Add nodes
		if _, exists := nodes[rel.TrusterID]; !exists {
			nodes[rel.TrusterID] = map[string]any{
				"id":   rel.TrusterID,
				"type": "actor",
			}
		}
		if _, exists := nodes[rel.TrusteeID]; !exists {
			nodes[rel.TrusteeID] = map[string]any{
				"id":   rel.TrusteeID,
				"type": "actor",
			}
		}

		// Add edge
		edges = append(edges, map[string]any{
			"from":       rel.TrusterID,
			"to":         rel.TrusteeID,
			"trust":      rel.Score,
			"created_at": rel.Created,
			"updated_at": rel.Updated,
		})
	}

	// Convert nodes map to array
	nodeArray := make([]any, 0, len(nodes))
	for _, node := range nodes {
		nodeArray = append(nodeArray, node)
	}

	// Return graph data
	ctx.Status(http.StatusOK)
	return ctx.JSON(map[string]any{
		"nodes": nodeArray,
		"edges": edges,
		"stats": map[string]any{
			"total_nodes": len(nodes),
			"total_edges": len(edges),
		},
	})
}

// HandleAdminUpdateTrustLift handles PUT /api/v1/admin/moderation/trust/:from/:to
func (h *Handler) HandleAdminUpdateTrustLift(ctx *lift.Context) error {
	// Check admin access
	adminClaims, err := h.requireAdminLift(ctx)
	if err != nil {
		ctx.Status(http.StatusForbidden)
		return ctx.JSON(map[string]string{"error": err.Error()})
	}

	fromActorID := ctx.Param("from")
	toActorID := ctx.Param("to")

	// Parse request body
	var req struct {
		Trust    float64 `json:"trust"`
		Category string  `json:"category,omitempty"` // Optional category, defaults to "general"
		Reason   string  `json:"reason"`
	}
	if err := ctx.ParseRequest(&req); err != nil {
		ctx.Status(http.StatusBadRequest)
		return ctx.JSON(map[string]string{"error": err.Error()})
	}

	// Validate trust value
	if err := common.ValidateFloatRange("trust", req.Trust, -1.0, 1.0); err != nil {
		ctx.Status(http.StatusBadRequest)
		return ctx.JSON(map[string]string{"error": err.Error()})
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

	if err := h.repos.Trust().UpdateTrustRelationship(ctx.Context, trustRel); err != nil {
		h.logger.Error("failed to update trust relationship", zap.Error(err))
		ctx.Status(http.StatusInternalServerError)
		return ctx.JSON(map[string]string{"error": err.Error()})
	}

	// Log admin action
	h.logger.Info("admin updated trust relationship",
		zap.String(roleAdmin, adminClaims.Username),
		zap.String("from", fromActorID),
		zap.String("to", toActorID),
		zap.Float64("trust", req.Trust),
		zap.String("category", category),
		zap.String("reason", req.Reason))

	// Return updated relationship
	ctx.Status(http.StatusOK)
	return ctx.JSON(map[string]any{
		"from_actor_id": fromActorID,
		"to_actor_id":   toActorID,
		"trust":         req.Trust,
		"category":      category,
		"updated_by":    adminClaims.Username,
		"reason":        req.Reason,
		"updated_at":    time.Now(),
	})
}

// HandleAdminGetReviewersLift handles GET /api/v1/admin/moderation/reviewers
func (h *Handler) HandleAdminGetReviewersLift(ctx *lift.Context) error {
	// Check admin access
	_, err := h.requireAdminLift(ctx)
	if err != nil {
		ctx.Status(http.StatusForbidden)
		return ctx.JSON(map[string]string{"error": err.Error()})
	}

	// Get users with moderator role
	moderatorUsers, err := h.repos.User().ListUsersByRole(ctx.Context, roleModerator)
	if err != nil {
		h.logger.Error("failed to list moderator users", zap.Error(err))
		ctx.Status(http.StatusInternalServerError)
		return ctx.JSON(map[string]string{"error": err.Error()})
	}

	// Get users with admin role
	adminUsers, err := h.repos.User().ListUsersByRole(ctx.Context, roleAdmin)
	if err != nil {
		h.logger.Error("failed to list admin users", zap.Error(err))
		ctx.Status(http.StatusInternalServerError)
		return ctx.JSON(map[string]string{"error": err.Error()})
	}

	// Combine both lists
	users := append(moderatorUsers, adminUsers...)

	reviewers := make([]map[string]any, 0)
	for _, user := range users {
		if user.Role == roleModerator || user.Role == roleAdmin {
			// Get review stats for this user
			stats, err := h.repos.Moderation().GetReviewerStats(ctx.Context, user.Username)
			if err != nil {
				h.logger.Warn("failed to get reviewer stats",
					zap.String("username", user.Username),
					zap.Error(err))
				stats = &storage.ReviewerStats{} // Use empty stats
			}

			reviewers = append(reviewers, map[string]any{
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

	ctx.Status(http.StatusOK)
	return ctx.JSON(map[string]any{
		"reviewers": reviewers,
		"total":     len(reviewers),
	})
}

// HandleAdminPromoteModeratorLift handles POST /api/v1/admin/moderation/reviewers/:id/promote
func (h *Handler) HandleAdminPromoteModeratorLift(ctx *lift.Context) error {
	// Check admin access
	adminClaims, err := h.requireAdminLift(ctx)
	if err != nil {
		ctx.Status(http.StatusForbidden)
		return ctx.JSON(map[string]string{"error": err.Error()})
	}

	userID := ctx.Param("id")

	// Extract username from user ID
	username := strings.TrimPrefix(userID, "user-")

	// Update user role to moderator
	updates := map[string]any{
		"role":       roleModerator,
		"updated_at": time.Now(),
	}

	if err := h.repos.Account().UpdateUser(ctx.Context, username, updates); err != nil {
		h.logger.Error("failed to promote user to moderator", zap.Error(err))
		ctx.Status(http.StatusInternalServerError)
		return ctx.JSON(map[string]string{"error": err.Error()})
	}

	// Log admin action
	h.logger.Info("admin promoted user to moderator",
		zap.String(roleAdmin, adminClaims.Username),
		zap.String("target", username))

	ctx.Status(http.StatusOK)
	return ctx.JSON(map[string]any{
		"user_id":     userID,
		"username":    username,
		"new_role":    roleModerator,
		"promoted_by": adminClaims.Username,
	})
}

// HandleAdminDemoteModeratorLift handles POST /api/v1/admin/moderation/reviewers/:id/demote
func (h *Handler) HandleAdminDemoteModeratorLift(ctx *lift.Context) error {
	// Check admin access
	adminClaims, err := h.requireAdminLift(ctx)
	if err != nil {
		ctx.Status(http.StatusForbidden)
		return ctx.JSON(map[string]string{"error": err.Error()})
	}

	userID := ctx.Param("id")

	// Extract username from user ID
	username := strings.TrimPrefix(userID, "user-")

	// Don't allow demoting admins
	user, err := h.repos.Account().GetUser(ctx.Context, username)
	if err != nil {
		if err == storage.ErrNotFound {
			ctx.Status(http.StatusNotFound)
			return ctx.JSON(map[string]string{"error": "user not found"})
		}
		h.logger.Error("failed to get user", zap.Error(err))
		ctx.Status(http.StatusInternalServerError)
		return ctx.JSON(map[string]string{"error": err.Error()})
	}

	if user.Role == roleAdmin {
		ctx.Status(http.StatusBadRequest)
		return ctx.JSON(map[string]string{"error": "cannot demote admin users"})
	}

	// Update user role to regular user
	updates := map[string]any{
		"role":       "user",
		"updated_at": time.Now(),
	}

	if err := h.repos.Account().UpdateUser(ctx.Context, username, updates); err != nil {
		h.logger.Error("failed to demote moderator", zap.Error(err))
		ctx.Status(http.StatusInternalServerError)
		return ctx.JSON(map[string]string{"error": err.Error()})
	}

	// Log admin action
	h.logger.Info("admin demoted moderator to user",
		zap.String(roleAdmin, adminClaims.Username),
		zap.String("target", username))

	ctx.Status(http.StatusOK)
	return ctx.JSON(map[string]any{
		"user_id":    userID,
		"username":   username,
		"new_role":   "user",
		"demoted_by": adminClaims.Username,
	})
}

// Helper functions for admin functionality

// Helper to convert activitypub.Actor to models.Account using transformation framework
func (h *Handler) convertActorToAccountWithCounts(ctx context.Context, actor *activitypub.Actor) models.Account {
	// Use centralized transformation framework for base conversion - ELIMINATES 40+ LINES OF DUPLICATE CODE
	account := transformations.ActorToAccountBase(actor, h.cfg.BaseURL())
	
	// Override ID to use PreferredUsername for admin interface consistency
	account.ID = actor.PreferredUsername
	
	// Get metadata for accurate timestamps
	_, metadata, err := h.repos.Actor().GetActorWithMetadata(ctx, actor.PreferredUsername)
	if err == nil && metadata != nil {
		account.CreatedAt = metadata.CreatedAt.Format(time.RFC3339)
		if metadata.LastStatusAt != nil {
			account.LastStatusAt = metadata.LastStatusAt.Format(time.RFC3339)
		}
	}

	// Get real counts for admin interface
	account.StatusesCount, _ = h.repos.Status().CountStatusesByAuthor(ctx, actor.ID)
	account.FollowersCount, _ = h.repos.Relationship().CountFollowers(ctx, actor.ID)

	// Get following count by checking first page
	following, _, _ := h.repos.Relationship().GetFollowing(ctx, actor.PreferredUsername, 1, "")
	account.FollowingCount = len(following)

	return account
}

// Helper functions
func getAdminRoleID(role string) string {
	switch role {
	case "admin":
		return "3"
	case roleModerator:
		return "2"
	default:
		return "1" // user
	}
}

func getAdminRolePermissions(role string) int {
	switch role {
	case "admin":
		return 0xFFFFFFFF // All permissions
	case roleModerator:
		return 0x0000FFFF // Moderation permissions
	default:
		return 0x00000001 // Basic user permissions
	}
}

// getActiveModeratorsCount returns the number of active moderators
func (h *Handler) getActiveModeratorsCount(ctx context.Context) int {
	moderatorUsers, err := h.repos.User().ListUsersByRole(ctx, roleModerator)
	if err != nil {
		h.logger.Warn("failed to get moderator users", zap.Error(err))
		return 0
	}

	adminUsers, err := h.repos.User().ListUsersByRole(ctx, "admin")
	if err != nil {
		h.logger.Warn("failed to get admin users", zap.Error(err))
		return 0
	}

	users := append(moderatorUsers, adminUsers...)

	// Count users who have been active in the last 30 days
	activeCount := 0
	thirtyDaysAgo := time.Now().AddDate(0, 0, -30)

	for _, user := range users {
		// Check if user has any recent activity
		sessions, err := h.repos.Account().GetUserSessions(ctx, user.Username)
		if err != nil {
			continue
		}

		for _, session := range sessions {
			if session.LastActivity.After(thirtyDaysAgo) {
				activeCount++
				break
			}
		}
	}

	return activeCount
}

// getRecentConsensusDecisions returns recent consensus-based moderation decisions
func (h *Handler) getRecentConsensusDecisions(ctx context.Context) []any {
	// Get recent moderation events that were resolved through consensus
	events, _, err := h.repos.Moderation().GetModerationEvents(ctx, &storage.ModerationEventFilter{
		MinSeverity: func() *int { s := 2; return &s }(), // 2 = medium severity
	}, 10, "")
	if err != nil {
		h.logger.Warn("failed to get recent consensus decisions", zap.Error(err))
		return []any{}
	}

	decisions := make([]any, 0, len(events))
	for _, event := range events {
		decisions = append(decisions, map[string]any{
			"id":         event.ID,
			"event_type": event.EventType,
			"actor_id":   event.ActorID,
			"severity":   event.Severity,
			"confidence": event.ConfidenceScore,
			"created_at": event.Created,
		})
	}

	return decisions
}

// getTrustGraphHealth returns trust graph health metrics
func (h *Handler) getTrustGraphHealth(ctx context.Context) map[string]any {
	// Get all trust relationships
	relationships, err := h.repos.Trust().GetAllTrustRelationships(ctx, 10000)
	if err != nil {
		h.logger.Warn("failed to get trust relationships", zap.Error(err))
		return map[string]any{
			"total_relationships": 0,
			"average_trust_score": 0.0,
			"isolated_users":      0,
		}
	}

	// Calculate metrics
	totalRelationships := len(relationships)
	var totalTrust float64
	userConnections := make(map[string]int)

	for _, rel := range relationships {
		totalTrust += rel.Score
		userConnections[rel.TrusterID]++
		userConnections[rel.TrusteeID]++
	}

	var averageTrust float64
	if totalRelationships > 0 {
		averageTrust = totalTrust / float64(totalRelationships)
	}

	// Count isolated users (users with no trust relationships)
	isolatedUsers := 0
	users, _, err := h.repos.User().ListUsers(ctx, 10000, "")
	if err == nil {
		for _, user := range users {
			if userConnections[user.Username] == 0 {
				isolatedUsers++
			}
		}
	}

	return map[string]any{
		"total_relationships": totalRelationships,
		"average_trust_score": averageTrust,
		"isolated_users":      isolatedUsers,
	}
}

// loadReportedStatuses loads the statuses that were reported
func (h *Handler) loadReportedStatuses(ctx context.Context, reportID string) []models.Status {
	statuses, err := h.repos.Moderation().GetReportedStatuses(ctx, reportID)
	if err != nil {
		h.logger.Warn("failed to load reported statuses", zap.String("report_id", reportID), zap.Error(err))
		return []models.Status{}
	}

	// Convert storage statuses to API models
	result := make([]models.Status, 0, len(statuses))
	for _, statusInterface := range statuses {
		// Handle any type - in practice these would be map[string]any or struct types
		statusMap, ok := statusInterface.(map[string]any)
		if !ok {
			continue
		}

		// Convert status using transformation framework - ELIMINATES 6+ LINES OF DUPLICATE CODE
		transformer := transformations.NewStatusResponseTransformer(h.cfg.BaseURL(), transformations.ObjectToStatusWithContext)
		transformCtx := context.WithValue(ctx, baseURLContextKey, h.cfg.BaseURL())
		
		apiStatus, err := transformer.Transform(transformCtx, statusMap)
		if err != nil {
			// Fallback for failed transformations
			apiStatus = models.Status{
				ID:        getStringFromMap(statusMap, "ID", ""),
				Content:   getStringFromMap(statusMap, "Content", ""),
				CreatedAt: getStringFromMap(statusMap, "CreatedAt", ""),
			}
		}
		result = append(result, apiStatus)
	}

	return result
}

// loadViolatedRules loads the rules that were violated in a report
func (h *Handler) loadViolatedRules(ctx context.Context, category string) []models.Rule {
	rules, err := h.repos.Instance().GetRulesByCategory(ctx, category)
	if err != nil {
		h.logger.Warn("failed to load violated rules", zap.String("category", category), zap.Error(err))
		return []models.Rule{}
	}

	// Convert storage rules to API models
	result := make([]models.Rule, 0, len(rules))
	for _, rule := range rules {
		apiRule := models.Rule{
			ID:   rule.ID,
			Text: rule.Text,
		}
		result = append(result, apiRule)
	}

	return result
}

// cancelUserFollowRelationships cancels all follow relationships for a user
func (h *Handler) cancelUserFollowRelationships(ctx context.Context, username string) error {
	// Get user's actor
	actor, err := h.repos.Actor().GetActor(ctx, username)
	if err != nil {
		return err
	}

	// Get all users this user is following
	following, _, err := h.repos.Relationship().GetFollowing(ctx, username, 1000, "")
	if err != nil {
		h.logger.Warn("failed to get following list for user", zap.String("username", username), zap.Error(err))
	} else {
		// Remove each follow relationship
		for _, followedActor := range following {
			if err := h.repos.Relationship().DeleteRelationship(ctx, actor.ID, followedActor); err != nil {
				h.logger.Warn("failed to remove follow relationship",
					zap.String("follower", username),
					zap.String("followed", followedActor),
					zap.Error(err))
			}
		}
	}

	// Get all users following this user
	followers, _, err := h.repos.Relationship().GetFollowers(ctx, username, 1000, "")
	if err != nil {
		h.logger.Warn("failed to get followers list for user", zap.String("username", username), zap.Error(err))
	} else {
		// Remove each follower relationship
		for _, followerActor := range followers {
			if err := h.repos.Relationship().DeleteRelationship(ctx, followerActor, actor.ID); err != nil {
				h.logger.Warn("failed to remove follower relationship",
					zap.String("follower", followerActor),
					zap.String("followed", username),
					zap.Error(err))
			}
		}
	}

	return nil
}

// markAllUserMediaAsSensitive marks all media uploaded by a user as sensitive
func (h *Handler) markAllUserMediaAsSensitive(ctx context.Context, username string) error {
	// Get all media attachments for this user
	mediaResult, err := h.repos.Media().GetUserMedia(ctx, username, interfaces.PaginationOptions{Limit: 100})
	if err != nil {
		return err
	}

	// Mark each media item as sensitive
	for _, mediaItem := range mediaResult.Items {
		if mediaItem == nil {
			continue
		}

		mediaID := mediaItem.MediaID
		if err := common.ValidateEntityID(mediaID, "media"); err != nil {
			continue
		}

		updates := map[string]any{
			"sensitive": true,
		}
		if err := h.repos.Media().UpdateMediaAttachment(ctx, mediaID, updates); err != nil {
			h.logger.Warn("failed to mark media as sensitive",
				zap.String("media_id", mediaID),
				zap.Error(err))
		}
	}

	return nil
}

// getStringFromMap is already defined in statuses.go

// Admin Status Moderation Endpoints

// HandleAdminGetStatusesLift handles GET /api/v1/admin/statuses
func (h *Handler) HandleAdminGetStatusesLift(ctx *lift.Context) error {
	// Check admin access
	_, err := h.requireAdminLift(ctx)
	if err != nil {
		ctx.Status(http.StatusForbidden)
		return ctx.JSON(map[string]string{"error": err.Error()})
	}

	pagination := h.parseAdminStatusPagination(ctx)
	filter := h.parseAdminStatusFilter(ctx)

	// Get statuses using the filtering method
	storageStatuses, nextCursor, err := h.repos.Status().ListStatusesForAdmin(ctx.Context, filter, pagination.Limit, pagination.Cursor)
	if err != nil {
		h.logger.Error("failed to get admin statuses", zap.Error(err))
		ctx.Status(http.StatusInternalServerError)
		return ctx.JSON(map[string]string{"error": "failed to get statuses"})
	}

	// Convert to []any for JSON response
	statusesAny := h.convertStatusesToAny(storageStatuses)

	// Add optional count header
	h.addCountHeaderIfRequested(ctx, filter)

	// Add pagination header
	h.addPaginationHeader(ctx, nextCursor, pagination)

	ctx.Status(http.StatusOK)
	return ctx.JSON(statusesAny)
}

// AdminStatusPagination holds pagination parameters for admin status queries
type AdminStatusPagination struct {
	Limit  int
	Cursor string
}

// parseAdminStatusPagination parses pagination parameters from request
func (h *Handler) parseAdminStatusPagination(ctx *lift.Context) AdminStatusPagination {
	limit, err := common.ParseAdminLimit(ctx.Query("limit"))
	if err != nil {
		// Log warning but use default instead of failing
		limit = 20
	}

	return AdminStatusPagination{
		Limit:  limit,
		Cursor: ctx.Query("cursor"),
	}
}

// parseAdminStatusFilter parses filter parameters from request
func (h *Handler) parseAdminStatusFilter(ctx *lift.Context) *repositories.StatusFilter {
	local := ctx.Query("local") == boolTrue
	remote := ctx.Query("remote") == boolTrue
	flagged := ctx.Query("flagged") == boolTrue
	reported := ctx.Query("reported") == boolTrue
	withMedia := ctx.Query("media") == boolTrue
	sensitive := ctx.Query("sensitive") == boolTrue

	filter := &repositories.StatusFilter{
		ByDomain:   ctx.Query("by_domain"),
		Visibility: ctx.Query("visibility"),
		MinDate:    h.parseDate(ctx.Query("min_date")),
		MaxDate:    h.parseDate(ctx.Query("max_date")),
	}

	// Set boolean filters only if explicitly requested
	if local {
		filter.Local = &local
	}
	if remote {
		filter.Remote = &remote
	}
	if ctx.Query("flagged") != "" {
		filter.Flagged = &flagged
	}
	if ctx.Query("reported") != "" {
		filter.Reported = &reported
	}
	if ctx.Query("media") != "" {
		filter.WithMedia = &withMedia
	}
	if ctx.Query("sensitive") != "" {
		filter.Sensitive = &sensitive
	}

	return filter
}

// parseDate parses RFC3339 date string, returns nil if empty or invalid
func (h *Handler) parseDate(dateStr string) *time.Time {
	if err := common.ValidateRequiredParam("dateStr", dateStr); err != nil {
		return nil
	}
	if parsed, err := time.Parse(time.RFC3339, dateStr); err == nil {
		return &parsed
	}
	return nil
}

// convertStatusesToAny converts status slice to []any for JSON response
func (h *Handler) convertStatusesToAny(storageStatuses []*storageModels.Status) []any {
	statusesAny := make([]any, len(storageStatuses))
	for i, status := range storageStatuses {
		statusesAny[i] = status
	}
	return statusesAny
}

// addCountHeaderIfRequested adds total count header if requested
func (h *Handler) addCountHeaderIfRequested(ctx *lift.Context, filter *repositories.StatusFilter) {
	if ctx.Query("include_count") == boolTrue {
		if totalCount, countErr := h.repos.Status().CountStatusesForAdmin(ctx.Context, filter); countErr == nil {
			ctx.Response.Headers["X-Total-Count"] = strconv.FormatInt(totalCount, 10)
		}
	}
}

// addPaginationHeader adds Link header for pagination if there's more data
func (h *Handler) addPaginationHeader(ctx *lift.Context, nextCursor string, pagination AdminStatusPagination) {
	if err := common.ValidateRequiredParam("nextCursor", nextCursor); err != nil {
		return
	}

	params := h.buildPaginationParams(ctx, pagination.Limit, nextCursor)
	nextURL := url.URL{
		Path:     "/api/v1/admin/statuses",
		RawQuery: params.Encode(),
	}
	ctx.Response.Headers["Link"] = fmt.Sprintf(`<%s>; rel="next"`, nextURL.String())
}

// buildPaginationParams builds URL parameters for pagination maintaining filter state
func (h *Handler) buildPaginationParams(ctx *lift.Context, limit int, nextCursor string) url.Values {
	params := url.Values{
		"limit":  []string{strconv.Itoa(limit)},
		"cursor": []string{nextCursor},
	}

	// Add filter parameters to maintain filter state across pagination
	filterParams := []string{
		"local", "remote", "by_domain", "visibility", "flagged", 
		"reported", "media", "sensitive", "min_date", "max_date",
	}

	for _, param := range filterParams {
		if value := ctx.Query(param); value != "" {
			params.Add(param, value)
		}
	}

	return params
}

// HandleAdminGetStatusLift handles GET /api/v1/admin/statuses/:id
func (h *Handler) HandleAdminGetStatusLift(ctx *lift.Context) error {
	// Check admin access
	_, err := h.requireAdminLift(ctx)
	if err != nil {
		ctx.Status(http.StatusForbidden)
		return ctx.JSON(map[string]string{"error": err.Error()})
	}

	statusID := ctx.Param("id")

	// Get status
	status, err := h.repos.Status().GetStatus(ctx.Context, statusID)
	if err != nil {
		if err == storage.ErrNotFound {
			ctx.Status(http.StatusNotFound)
			return ctx.JSON(map[string]string{"error": "status not found"})
		}
		h.logger.Error("failed to get status", zap.Error(err))
		ctx.Status(http.StatusInternalServerError)
		return ctx.JSON(map[string]string{"error": "failed to get status"})
	}

	ctx.Status(http.StatusOK)
	return ctx.JSON(status)
}

// HandleAdminDeleteStatusLift handles DELETE /api/v1/admin/statuses/:id
func (h *Handler) HandleAdminDeleteStatusLift(ctx *lift.Context) error {
	// Check admin access
	adminClaims, err := h.requireAdminLift(ctx)
	if err != nil {
		ctx.Status(http.StatusForbidden)
		return ctx.JSON(map[string]string{"error": err.Error()})
	}

	statusID := ctx.Param("id")

	// Get the status first to validate it exists
	status, err := h.repos.Status().GetStatus(ctx.Context, statusID)
	if err != nil {
		if err == storage.ErrNotFound {
			ctx.Status(http.StatusNotFound)
			return ctx.JSON(map[string]string{"error": "status not found"})
		}
		h.logger.Error("failed to get status for deletion", zap.Error(err))
		ctx.Status(http.StatusInternalServerError)
		return ctx.JSON(map[string]string{"error": "failed to get status"})
	}

	// Log admin action
	h.logger.Info("admin delete status",
		zap.String("admin", adminClaims.Username),
		zap.String("status_id", statusID),
		zap.Any("status", status))

	// Delete the status
	err = h.repos.Status().DeleteStatus(ctx.Context, statusID)
	if err != nil {
		h.logger.Error("failed to delete status", zap.Error(err))
		ctx.Status(http.StatusInternalServerError)
		return ctx.JSON(map[string]string{"error": "failed to delete status"})
	}

	// Create audit log entry
	if err := h.createAuditLogEntry(ctx.Context, adminClaims.Username, "delete_status", "status", statusID, "Admin deleted status", nil); err != nil {
		h.logger.Warn("failed to create audit log entry", zap.Error(err))
		// Don't fail the request due to audit log issues
	}

	ctx.Status(http.StatusNoContent)
	return nil
}

// HandleAdminMarkStatusSensitiveLift handles POST /api/v1/admin/statuses/:id/sensitive
func (h *Handler) HandleAdminMarkStatusSensitiveLift(ctx *lift.Context) error {
	return h.adminStatusSensitiveAction(ctx, true, "mark_sensitive", "Admin marked status as sensitive")
}

// HandleAdminUnmarkStatusSensitiveLift handles POST /api/v1/admin/statuses/:id/unsensitive
func (h *Handler) HandleAdminUnmarkStatusSensitiveLift(ctx *lift.Context) error {
	return h.adminStatusSensitiveAction(ctx, false, "unmark_sensitive", "Admin unmarked status as sensitive")
}

// adminStatusSensitiveAction consolidates status sensitive/unsensitive operations
func (h *Handler) adminStatusSensitiveAction(ctx *lift.Context, sensitive bool, action, reason string) error {
	// Check admin access
	adminClaims, err := h.requireAdminLift(ctx)
	if err != nil {
		ctx.Status(http.StatusForbidden)
		return ctx.JSON(map[string]string{"error": err.Error()})
	}

	statusID := ctx.Param("id")

	// Get the existing status first
	status, err := h.repos.Status().GetStatus(ctx.Context, statusID)
	if err != nil {
		if err == storage.ErrNotFound {
			ctx.Status(http.StatusNotFound)
			return ctx.JSON(map[string]string{"error": "status not found"})
		}
		h.logger.Error("failed to get status for update", zap.Error(err))
		ctx.Status(http.StatusInternalServerError)
		return ctx.JSON(map[string]string{"error": "failed to get status"})
	}

	// Update the status fields
	status.Sensitive = sensitive
	status.UpdatedAt = time.Now()

	err = h.repos.Status().UpdateStatus(ctx.Context, status)
	if err != nil {
		if err == storage.ErrNotFound {
			ctx.Status(http.StatusNotFound)
			return ctx.JSON(map[string]string{"error": "status not found"})
		}
		logMessage := "failed to mark status as sensitive"
		if !sensitive {
			logMessage = "failed to unmark status as sensitive"
		}
		h.logger.Error(logMessage, zap.Error(err))
		ctx.Status(http.StatusInternalServerError)
		return ctx.JSON(map[string]string{"error": "failed to update status"})
	}

	// Log admin action
	logMessage := "admin mark status sensitive"
	if !sensitive {
		logMessage = "admin unmark status sensitive"
	}
	h.logger.Info(logMessage,
		zap.String("admin", adminClaims.Username),
		zap.String("status_id", statusID))

	// Create audit log entry
	if err := h.createAuditLogEntry(ctx.Context, adminClaims.Username, action, "status", statusID, reason, nil); err != nil {
		h.logger.Warn("failed to create audit log entry", zap.Error(err))
	}

	// Get updated status
	updatedStatus, err := h.repos.Status().GetStatus(ctx.Context, statusID)
	if err != nil {
		h.logger.Error("failed to get updated status", zap.Error(err))
		ctx.Status(http.StatusInternalServerError)
		return ctx.JSON(map[string]string{"error": "failed to get updated status"})
	}

	ctx.Status(http.StatusOK)
	return ctx.JSON(updatedStatus)
}

// Helper function to create audit log entries
func (h *Handler) createAuditLogEntry(ctx context.Context, adminID, action, targetType, targetID, reason string, details any) error {
	auditEntry := &storage.AuditLog{
		ID:         uuid.New().String(),
		AdminID:    adminID,
		AdminRole:  roleAdmin, // Could check actual role
		Action:     action,
		TargetType: targetType,
		TargetID:   targetID,
		Reason:     reason,
		Details:    details,
		Timestamp:  time.Now(),
		CreatedAt:  time.Now(),
	}

	return h.repos.Moderation().CreateAuditLog(ctx, auditEntry)
}

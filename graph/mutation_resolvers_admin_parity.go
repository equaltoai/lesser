package graph

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage"
	"go.uber.org/zap"
)

// AdminCreateUser covers POST /api/v1/admin/accounts.
func (r *mutationResolver) AdminCreateUser(ctx context.Context, input model.AdminCreateUserInput) (*model.AdminAccount, error) {
	_, err := r.requireAdmin(ctx)
	if err != nil {
		return nil, err
	}

	if r.Storage == nil || r.Storage.User() == nil {
		return nil, ErrStorageUnavailable
	}

	username := strings.TrimSpace(input.Username)
	if err := common.ValidateRequiredParam("username", username); err != nil {
		return nil, err
	}

	//nolint:staticcheck // Legacy password field support
	if input.Password != nil && strings.TrimSpace(*input.Password) != "" {
		r.Logger.Warn("password provided in admin user creation but passwords are disabled",
			zap.String("username", username))
	}

	role := adminRoleUser
	if input.Role != nil && strings.TrimSpace(*input.Role) != "" {
		role = strings.TrimSpace(*input.Role)
	}

	user := &storage.User{
		Username:     username,
		Email:        ptrOrEmpty(input.Email),
		PasswordHash: "",
		DisplayName:  ptrOrEmpty(input.DisplayName),
		Role:         role,
		Approved:     true,
		Locale:       "en",
	}

	if err := r.Storage.User().CreateUser(ctx, user); err != nil {
		return nil, errors.Join(errors.New("failed to create user"), err)
	}

	accountID, _ := normalizeAdminAccountID(username)
	return &model.AdminAccount{
		ID:                   accountID,
		Username:             username,
		CreatedAt:            model.Time(user.CreatedAt),
		Locale:               user.Locale,
		IP:                   nil,
		Ips:                  []*model.AdminIP{},
		Role:                 adminRoleFromName(role),
		Confirmed:            true,
		Approved:             true,
		Disabled:             false,
		Silenced:             false,
		Suspended:            false,
		Actor:                nil,
		ReportsCount:         0,
		ResolvedReportsCount: 0,
	}, nil
}

// AdminAccountAction covers POST /api/v1/admin/accounts/{id}/action and related action endpoints.
func (r *mutationResolver) AdminAccountAction(ctx context.Context, input model.AdminAccountActionInput) (bool, error) {
	adminUsername, err := r.requireAdmin(ctx)
	if err != nil {
		return false, err
	}

	_, username := normalizeAdminAccountID(input.ID)
	if err := common.ValidateRequiredParam("username", username); err != nil {
		return false, err
	}

	if r.Storage == nil || r.Storage.Account() == nil {
		return false, ErrStorageUnavailable
	}

	actionType := strings.ToLower(strings.TrimSpace(input.Type))
	if err := common.ValidateRequiredParam("type", actionType); err != nil {
		return false, err
	}

	reason := strings.TrimSpace(ptrOrEmpty(input.Text))

	validActions := map[string]bool{
		AdminAccountActionSuspend:     true,
		"unsuspend":                   true,
		AdminAccountActionSilence:     true,
		"unsilence":                   true,
		AdminAccountActionSensitive:   true,
		AdminAccountActionUnsensitive: true,
		"disable":                     true,
		"enable":                      true,
		ModerationActionApprove:       true,
		ModerationActionReject:        true,
	}

	if !validActions[actionType] {
		return false, errors.New("invalid action type")
	}

	r.Logger.Info("admin account action",
		zap.String("admin", adminUsername),
		zap.String("target", username),
		zap.String("action", actionType),
		zap.String("reason", reason))

	switch actionType {
	case ModerationActionReject:
		if err := r.Storage.Account().DeleteAccount(ctx, username); err != nil {
			return false, errors.Join(errors.New("failed to reject account"), err)
		}
		return true, nil
	case AdminAccountActionUnsensitive:
		if r.Storage.Media() != nil {
			if err := r.Storage.Media().UnmarkAllMediaAsSensitive(ctx, username); err != nil {
				r.Logger.Warn("failed to unmark media as sensitive",
					zap.String("username", username),
					zap.Error(err))
			}
		}
	}

	updates := make(map[string]any)

	switch actionType {
	case AdminAccountActionSuspend:
		updates[common.AccountStatusSuspended] = true
		r.cancelUserFollowRelationships(ctx, username)
	case "unsuspend":
		updates[common.AccountStatusSuspended] = false
	case "disable":
		updates["approved"] = false
	case "enable", ModerationActionApprove:
		updates["approved"] = true
	case AdminAccountActionSilence:
		updates["silenced"] = true
	case "unsilence":
		updates["silenced"] = false
	case AdminAccountActionSensitive:
		r.markAllUserMediaAsSensitive(ctx, username)
		updates["sensitive_media"] = true
	case AdminAccountActionUnsensitive:
		updates["sensitive_media"] = false
	}

	if len(updates) == 0 {
		return true, nil
	}

	updates["updated_at"] = time.Now()
	if err := r.Storage.Account().UpdateUser(ctx, username, updates); err != nil {
		return false, errors.Join(errors.New("failed to update user"), err)
	}

	return true, nil
}

// AdminCreateAnnouncement covers POST /api/v1/admin/announcements.
func (r *mutationResolver) AdminCreateAnnouncement(ctx context.Context, input model.AdminCreateAnnouncementInput) (*model.Announcement, error) {
	_, err := r.requireAdmin(ctx)
	if err != nil {
		return nil, err
	}

	if r.Storage == nil || r.Storage.Announcement() == nil {
		return nil, ErrStorageUnavailable
	}

	text := strings.TrimSpace(input.Text)
	if err := common.ValidateRequiredParam("text", text); err != nil {
		return nil, err
	}

	allDay := false
	if input.AllDay != nil {
		allDay = *input.AllDay
	}

	now := time.Now()
	announcement := &storage.Announcement{
		ID:          now.Format("20060102150405"),
		Content:     text,
		Text:        text,
		AllDay:      allDay,
		PublishedAt: now,
		UpdatedAt:   now,
	}

	if input.StartsAt != nil {
		start := time.Time(*input.StartsAt)
		announcement.StartsAt = &start
	}
	if input.EndsAt != nil {
		end := time.Time(*input.EndsAt)
		announcement.EndsAt = &end
	}

	if err := r.Storage.Announcement().CreateAnnouncement(ctx, announcement); err != nil {
		return nil, errors.Join(errors.New("failed to create announcement"), err)
	}

	startsAt := (*model.Time)(nil)
	if announcement.StartsAt != nil {
		t := model.Time(*announcement.StartsAt)
		startsAt = &t
	}
	endsAt := (*model.Time)(nil)
	if announcement.EndsAt != nil {
		t := model.Time(*announcement.EndsAt)
		endsAt = &t
	}

	return &model.Announcement{
		ID:          announcement.ID,
		Content:     announcement.Content,
		Text:        announcement.Text,
		PublishedAt: model.Time(announcement.PublishedAt),
		UpdatedAt:   model.Time(announcement.UpdatedAt),
		AllDay:      announcement.AllDay,
		StartsAt:    startsAt,
		EndsAt:      endsAt,
		Read:        false,
		Reactions:   []*model.AnnouncementReaction{},
	}, nil
}

// AdminCreateDomainAllow covers POST /api/v1/admin/domain_allows.
func (r *mutationResolver) AdminCreateDomainAllow(ctx context.Context, domain string) (*model.AdminDomainAllow, error) {
	adminUsername, err := r.requireAdmin(ctx)
	if err != nil {
		return nil, err
	}
	if r.Storage == nil || r.Storage.DomainBlock() == nil {
		return nil, ErrStorageUnavailable
	}

	domain = cleanDomainValue(domain)
	if err := common.ValidateRequiredParam("domain", domain); err != nil {
		return nil, err
	}

	allow := &storage.DomainAllow{
		Domain:    domain,
		CreatedBy: adminUsername,
	}

	if err := r.Storage.DomainBlock().CreateDomainAllow(ctx, allow); err != nil {
		return nil, errors.Join(errors.New("failed to create domain allow"), err)
	}

	return &model.AdminDomainAllow{
		ID:        allow.ID,
		Domain:    allow.Domain,
		CreatedAt: model.Time(allow.CreatedAt),
	}, nil
}

// AdminDeleteDomainAllow covers DELETE /api/v1/admin/domain_allows/{id}.
func (r *mutationResolver) AdminDeleteDomainAllow(ctx context.Context, id string) (bool, error) {
	_, err := r.requireAdmin(ctx)
	if err != nil {
		return false, err
	}
	if r.Storage == nil || r.Storage.DomainBlock() == nil {
		return false, ErrStorageUnavailable
	}

	id = strings.TrimSpace(id)
	if err := common.ValidateRequiredParam("id", id); err != nil {
		return false, err
	}

	if err := r.Storage.DomainBlock().DeleteDomainAllow(ctx, id); err != nil {
		return false, errors.Join(errors.New("failed to delete domain allow"), err)
	}
	return true, nil
}

// AdminCreateDomainBlock covers POST /api/v1/admin/domain_blocks.
func (r *mutationResolver) AdminCreateDomainBlock(ctx context.Context, input model.AdminDomainBlockCreateInput) (*model.AdminDomainBlock, error) {
	adminUsername, err := r.requireAdmin(ctx)
	if err != nil {
		return nil, err
	}

	if r.Storage == nil || r.Storage.DomainBlock() == nil {
		return nil, ErrStorageUnavailable
	}

	domain := cleanDomainValue(input.Domain)
	if err := common.ValidateRequiredParam("domain", domain); err != nil {
		return nil, err
	}

	severity := "suspend"
	if input.Severity != nil && strings.TrimSpace(*input.Severity) != "" {
		severity = strings.TrimSpace(*input.Severity)
	}
	if severity != "silence" && severity != "suspend" {
		return nil, errors.New("severity must be 'silence' or 'suspend'")
	}

	block := &storage.InstanceDomainBlock{
		Domain:         domain,
		Severity:       severity,
		RejectMedia:    boolOrFalse(input.RejectMedia),
		RejectReports:  boolOrFalse(input.RejectReports),
		PrivateComment: ptrOrEmpty(input.PrivateComment),
		PublicComment:  ptrOrEmpty(input.PublicComment),
		Obfuscate:      boolOrFalse(input.Obfuscate),
		CreatedBy:      adminUsername,
	}

	if err := r.Storage.DomainBlock().CreateDomainBlock(ctx, block); err != nil {
		return nil, errors.Join(errors.New("failed to create domain block"), err)
	}

	return convertInstanceDomainBlockToAdminBlock(block), nil
}

// AdminUpdateDomainBlock covers PUT /api/v1/admin/domain_blocks/{id}.
func (r *mutationResolver) AdminUpdateDomainBlock(ctx context.Context, id string, input model.AdminDomainBlockUpdateInput) (*model.AdminDomainBlock, error) {
	_, err := r.requireAdmin(ctx)
	if err != nil {
		return nil, err
	}

	blockID := strings.TrimSpace(id)
	if err := common.ValidateRequiredParam("id", blockID); err != nil {
		return nil, err
	}

	if r.Storage == nil || r.Storage.DomainBlock() == nil {
		return nil, ErrStorageUnavailable
	}

	updates := make(map[string]any)
	if input.Severity != nil && strings.TrimSpace(*input.Severity) != "" {
		updates["severity"] = strings.TrimSpace(*input.Severity)
	}
	if input.RejectMedia != nil {
		updates["reject_media"] = *input.RejectMedia
	}
	if input.RejectReports != nil {
		updates["reject_reports"] = *input.RejectReports
	}
	if input.PrivateComment != nil {
		updates["private_comment"] = strings.TrimSpace(*input.PrivateComment)
	}
	if input.PublicComment != nil {
		updates["public_comment"] = strings.TrimSpace(*input.PublicComment)
	}
	if input.Obfuscate != nil {
		updates["obfuscate"] = *input.Obfuscate
	}

	if err := r.Storage.DomainBlock().UpdateDomainBlock(ctx, blockID, updates); err != nil {
		return nil, errors.Join(errors.New("failed to update domain block"), err)
	}

	updated, err := r.Storage.DomainBlock().GetDomainBlock(ctx, blockID)
	if err != nil {
		return nil, errors.Join(errors.New("failed to load updated domain block"), err)
	}

	return convertInstanceDomainBlockToAdminBlock(updated), nil
}

// AdminDeleteDomainBlock covers DELETE /api/v1/admin/domain_blocks/{id}.
func (r *mutationResolver) AdminDeleteDomainBlock(ctx context.Context, id string) (bool, error) {
	_, err := r.requireAdmin(ctx)
	if err != nil {
		return false, err
	}
	if r.Storage == nil || r.Storage.DomainBlock() == nil {
		return false, ErrStorageUnavailable
	}

	id = strings.TrimSpace(id)
	if err := common.ValidateRequiredParam("id", id); err != nil {
		return false, err
	}

	if err := r.Storage.DomainBlock().DeleteDomainBlock(ctx, id); err != nil {
		return false, errors.Join(errors.New("failed to delete domain block"), err)
	}
	return true, nil
}

// AdminCreateEmailDomainBlock covers POST /api/v1/admin/email_domain_blocks.
func (r *mutationResolver) AdminCreateEmailDomainBlock(ctx context.Context, domain string) (*model.AdminEmailDomainBlock, error) {
	adminUsername, err := r.requireAdmin(ctx)
	if err != nil {
		return nil, err
	}
	if r.Storage == nil || r.Storage.DomainBlock() == nil {
		return nil, ErrStorageUnavailable
	}

	domain = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(domain)), "@")
	if err := common.ValidateRequiredParam("domain", domain); err != nil {
		return nil, err
	}

	block := &storage.EmailDomainBlock{
		Domain:    domain,
		CreatedBy: adminUsername,
	}

	if err := r.Storage.DomainBlock().CreateEmailDomainBlock(ctx, block); err != nil {
		return nil, errors.Join(errors.New("failed to create email domain block"), err)
	}

	return &model.AdminEmailDomainBlock{
		ID:        block.ID,
		Domain:    block.Domain,
		CreatedAt: model.Time(block.CreatedAt),
	}, nil
}

// AdminDeleteEmailDomainBlock covers DELETE /api/v1/admin/email_domain_blocks/{id}.
func (r *mutationResolver) AdminDeleteEmailDomainBlock(ctx context.Context, id string) (bool, error) {
	_, err := r.requireAdmin(ctx)
	if err != nil {
		return false, err
	}
	if r.Storage == nil || r.Storage.DomainBlock() == nil {
		return false, ErrStorageUnavailable
	}

	id = strings.TrimSpace(id)
	if err := common.ValidateRequiredParam("id", id); err != nil {
		return false, err
	}

	if err := r.Storage.DomainBlock().DeleteEmailDomainBlock(ctx, id); err != nil {
		return false, errors.Join(errors.New("failed to delete email domain block"), err)
	}
	return true, nil
}

// AdminOverrideModerationEvent covers POST /api/v1/admin/moderation/events/{id}/override.
func (r *mutationResolver) AdminOverrideModerationEvent(ctx context.Context, input model.AdminModerationEventOverrideInput) (*model.AdminModerationEventOverrideResult, error) {
	adminUsername, err := r.requireAdmin(ctx)
	if err != nil {
		return nil, err
	}
	if r.Storage == nil || r.Storage.Moderation() == nil {
		return nil, ErrModerationRepositoryUnavailable
	}

	eventID := strings.TrimSpace(input.EventID)
	if err := common.ValidateRequiredParam("eventId", eventID); err != nil {
		return nil, err
	}

	decision := strings.ToLower(strings.TrimSpace(input.Decision))
	if decision != "approve" && decision != "reject" {
		return nil, errors.New("decision must be 'approve' or 'reject'")
	}

	event, err := r.Storage.Moderation().GetModerationEvent(ctx, eventID)
	if err != nil {
		return nil, errors.Join(errors.New("failed to load moderation event"), err)
	}

	action := storage.ActionTypeNone
	if decision == "reject" {
		switch strings.TrimSpace(event.Severity) {
		case "4":
			action = storage.ActionTypeSuspend
		case "3":
			action = storage.ActionTypeSilence
		default:
			action = storage.ActionTypeWarning
		}
	}

	reason := strings.TrimSpace(ptrOrEmpty(input.Reason))
	if err := r.Storage.Moderation().CreateAdminReview(ctx, eventID, adminUsername, action, reason); err != nil {
		return nil, errors.Join(errors.New("failed to create admin review"), err)
	}

	r.Logger.Info("admin override moderation event",
		zap.String("admin", adminUsername),
		zap.String("event_id", eventID),
		zap.String("decision", decision),
		zap.String("action", string(action)),
		zap.String("reason", reason))

	return &model.AdminModerationEventOverrideResult{
		EventID:  eventID,
		Decision: decision,
		Action:   string(action),
		Override: true,
		Admin:    adminUsername,
		Reason:   optionalString(reason),
	}, nil
}

// AdminUpdateTrust covers PUT /api/v1/admin/moderation/trust/{from}/{to}.
func (r *mutationResolver) AdminUpdateTrust(ctx context.Context, input model.AdminUpdateTrustInput) (*model.AdminUpdateTrustResult, error) {
	adminUsername, err := r.requireAdmin(ctx)
	if err != nil {
		return nil, err
	}
	if r.Storage == nil || r.Storage.Trust() == nil {
		return nil, ErrTrustRepositoryUnavailable
	}

	if err := common.ValidateRequiredParam("fromActorId", input.FromActorID); err != nil {
		return nil, err
	}
	if err := common.ValidateRequiredParam("toActorId", input.ToActorID); err != nil {
		return nil, err
	}
	if err := common.ValidateFloatRange("trust", input.Trust, -1.0, 1.0); err != nil {
		return nil, err
	}

	category := "general"
	if input.Category != nil && strings.TrimSpace(*input.Category) != "" {
		category = strings.TrimSpace(*input.Category)
	}

	now := time.Now()
	relationship := &storage.TrustRelationship{
		TrusterID:  input.FromActorID,
		TrusteeID:  input.ToActorID,
		Category:   storage.TrustCategory(category),
		Score:      input.Trust,
		Confidence: 1.0,
		Created:    now,
		Updated:    now,
	}

	if err := r.Storage.Trust().UpdateTrustRelationship(ctx, relationship); err != nil {
		return nil, errors.Join(errors.New("failed to update trust relationship"), err)
	}

	return &model.AdminUpdateTrustResult{
		FromActorID: input.FromActorID,
		ToActorID:   input.ToActorID,
		Trust:       input.Trust,
		Category:    category,
		UpdatedBy:   adminUsername,
		Reason:      optionalString(ptrOrEmpty(input.Reason)),
		UpdatedAt:   model.Time(now),
	}, nil
}

// AdminPromoteReviewer covers POST /api/v1/admin/moderation/reviewers/{id}/promote.
func (r *mutationResolver) AdminPromoteReviewer(ctx context.Context, id string) (*model.AdminReviewerRoleResult, error) {
	adminUsername, err := r.requireAdmin(ctx)
	if err != nil {
		return nil, err
	}
	if r.Storage == nil || r.Storage.Account() == nil {
		return nil, ErrStorageUnavailable
	}

	userID, username := normalizeAdminAccountID(id)
	if err := common.ValidateRequiredParam("id", userID); err != nil {
		return nil, err
	}

	updates := map[string]any{
		"role":       adminRoleModerator,
		"updated_at": time.Now(),
	}

	if err := r.Storage.Account().UpdateUser(ctx, username, updates); err != nil {
		return nil, errors.Join(errors.New("failed to promote reviewer"), err)
	}

	return &model.AdminReviewerRoleResult{
		UserID:    userID,
		Username:  username,
		NewRole:   adminRoleModerator,
		UpdatedBy: adminUsername,
	}, nil
}

// AdminDemoteReviewer covers POST /api/v1/admin/moderation/reviewers/{id}/demote.
func (r *mutationResolver) AdminDemoteReviewer(ctx context.Context, id string) (*model.AdminReviewerRoleResult, error) {
	adminUsername, err := r.requireAdmin(ctx)
	if err != nil {
		return nil, err
	}
	if r.Storage == nil || r.Storage.Account() == nil {
		return nil, ErrStorageUnavailable
	}

	userID, username := normalizeAdminAccountID(id)
	if err := common.ValidateRequiredParam("id", userID); err != nil {
		return nil, err
	}

	user, err := r.Storage.Account().GetUser(ctx, username)
	if err != nil {
		return nil, errors.Join(errors.New("failed to load user"), err)
	}

	if strings.EqualFold(strings.TrimSpace(user.Role), adminRoleAdmin) {
		return nil, errors.New("cannot demote admin users")
	}

	updates := map[string]any{
		"role":       adminRoleUser,
		"updated_at": time.Now(),
	}

	if err := r.Storage.Account().UpdateUser(ctx, username, updates); err != nil {
		return nil, errors.Join(errors.New("failed to demote reviewer"), err)
	}

	return &model.AdminReviewerRoleResult{
		UserID:    userID,
		Username:  username,
		NewRole:   adminRoleUser,
		UpdatedBy: adminUsername,
	}, nil
}

// AdminReportAction covers /api/v1/admin/reports/{id}/* action routes.
func (r *mutationResolver) AdminReportAction(ctx context.Context, id string, action model.AdminReportAction) (*model.AdminReport, error) {
	adminUsername, err := r.requireAdmin(ctx)
	if err != nil {
		return nil, err
	}
	if r.Storage == nil || r.Storage.Moderation() == nil {
		return nil, ErrModerationRepositoryUnavailable
	}

	reportID := strings.TrimSpace(id)
	if err := common.ValidateRequiredParam("id", reportID); err != nil {
		return nil, err
	}

	switch action {
	case model.AdminReportActionAssignToSelf:
		if err := r.Storage.Moderation().AssignReport(ctx, reportID, adminUsername); err != nil {
			return nil, errors.Join(errors.New("failed to assign report"), err)
		}
	case model.AdminReportActionUnassign:
		if err := r.Storage.Moderation().UnassignReport(ctx, reportID); err != nil {
			return nil, errors.Join(errors.New("failed to unassign report"), err)
		}
	case model.AdminReportActionResolve:
		if err := r.Storage.Moderation().UpdateReportStatus(ctx, reportID, storage.ReportStatusResolved, "Resolved by admin", adminUsername); err != nil {
			return nil, errors.Join(errors.New("failed to resolve report"), err)
		}
	case model.AdminReportActionReopen:
		if err := r.Storage.Moderation().UpdateReportStatus(ctx, reportID, storage.ReportStatusOpen, "Reopened by admin", adminUsername); err != nil {
			return nil, errors.Join(errors.New("failed to reopen report"), err)
		}
	default:
		return nil, errors.New("unsupported report action")
	}

	report, err := r.Storage.Moderation().GetReport(ctx, reportID)
	if err != nil {
		return nil, errors.Join(errors.New("failed to load report"), err)
	}

	qr := &queryResolver{r.Resolver}
	return qr.convertReportToAdminReport(ctx, report), nil
}

// AdminDeleteStatus covers DELETE /api/v1/admin/statuses/{id}.
func (r *mutationResolver) AdminDeleteStatus(ctx context.Context, id string) (bool, error) {
	_, err := r.requireAdmin(ctx)
	if err != nil {
		return false, err
	}
	if r.Storage == nil || r.Storage.Status() == nil {
		return false, ErrStatusRepositoryUnavailable
	}

	statusID := strings.TrimSpace(id)
	if err := common.ValidateRequiredParam("id", statusID); err != nil {
		return false, err
	}

	if err := r.Storage.Status().DeleteStatus(ctx, statusID); err != nil {
		return false, errors.Join(errors.New("failed to delete status"), err)
	}

	return true, nil
}

// AdminSetStatusSensitive covers POST /api/v1/admin/statuses/{id}/{sensitive|unsensitive}.
func (r *mutationResolver) AdminSetStatusSensitive(ctx context.Context, id string, sensitive bool) (*model.Object, error) {
	_, err := r.requireAdmin(ctx)
	if err != nil {
		return nil, err
	}
	if r.Storage == nil || r.Storage.Status() == nil {
		return nil, ErrStatusRepositoryUnavailable
	}

	statusID := strings.TrimSpace(id)
	if err := common.ValidateRequiredParam("id", statusID); err != nil {
		return nil, err
	}

	status, err := r.Storage.Status().GetStatus(ctx, statusID)
	if err != nil {
		return nil, errors.Join(errors.New("failed to load status"), err)
	}

	status.Sensitive = sensitive
	status.UpdatedAt = time.Now()

	if err := r.Storage.Status().UpdateStatus(ctx, status); err != nil {
		return nil, errors.Join(errors.New("failed to update status"), err)
	}

	return r.convertStatusToObject(ctx, status), nil
}

func (r *mutationResolver) markAllUserMediaAsSensitive(ctx context.Context, username string) {
	if r.Storage == nil || r.Storage.Media() == nil {
		return
	}

	mediaList, err := r.Storage.Media().GetMediaByUser(ctx, username, 0)
	if err != nil {
		r.Logger.Warn("failed to list media for user",
			zap.String("username", username),
			zap.Error(err))
		return
	}

	for _, media := range mediaList {
		if media == nil {
			continue
		}
		media.IsNSFW = true
		if err := r.Storage.Media().UpdateMedia(ctx, media); err != nil {
			r.Logger.Warn("failed to mark media as sensitive",
				zap.String("media_id", media.MediaID),
				zap.Error(err))
		}
	}
}

func (r *mutationResolver) cancelUserFollowRelationships(ctx context.Context, username string) {
	if r.Storage == nil || r.Storage.Relationship() == nil {
		return
	}

	identifiers := []string{username}
	if r.Storage.Actor() != nil {
		actor, err := r.Storage.Actor().GetActor(ctx, username)
		if err == nil && actor != nil && strings.TrimSpace(actor.ID) != "" {
			identifiers = append(identifiers, actor.ID)
		}
	}

	for _, id := range identifiers {
		following, _, err := r.Storage.Relationship().GetFollowing(ctx, id, 1000, "")
		if err == nil {
			for _, target := range following {
				_ = r.Storage.Relationship().DeleteRelationship(ctx, id, target)
			}
		}

		followers, _, err := r.Storage.Relationship().GetFollowers(ctx, id, 1000, "")
		if err == nil {
			for _, follower := range followers {
				_ = r.Storage.Relationship().DeleteRelationship(ctx, follower, id)
			}
		}
	}
}

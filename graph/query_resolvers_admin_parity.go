package graph

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"go.uber.org/zap"
)

// AdminAccounts covers GET /api/v1/admin/accounts.
func (r *queryResolver) AdminAccounts(ctx context.Context, first *int, after *model.Cursor) (*model.AdminAccountConnection, error) {
	_, err := r.requireAdmin(ctx)
	if err != nil {
		return nil, err
	}

	if r.Storage == nil || r.Storage.User() == nil {
		return nil, ErrStorageUnavailable
	}

	limit := 50
	if first != nil {
		limit = clampInt(*first, 1, 100)
	}

	const maxInt32 = int32(^uint32(0) >> 1)
	var safeLimit32 int32
	switch {
	case limit <= 0:
		safeLimit32 = 0
	case limit > int(maxInt32):
		safeLimit32 = maxInt32
	default:
		safeLimit32 = int32(limit)
	}

	users, nextCursor, err := r.Storage.User().ListUsers(ctx, safeLimit32, cursorToString(after))
	if err != nil {
		return nil, errors.Join(errors.New("failed to list users"), err)
	}

	accounts := make([]*model.AdminAccount, 0, len(users))
	for _, user := range users {
		if user == nil {
			continue
		}

		account, err := r.buildAdminAccount(ctx, user, false)
		if err != nil {
			r.Logger.Warn("failed to build admin account",
				zap.String("username", user.Username),
				zap.Error(err))
			continue
		}
		accounts = append(accounts, account)
	}

	return &model.AdminAccountConnection{
		Accounts:   accounts,
		NextCursor: stringToCursor(nextCursor),
	}, nil
}

// AdminAccount covers GET /api/v1/admin/accounts/{id}.
func (r *queryResolver) AdminAccount(ctx context.Context, id string) (*model.AdminAccount, error) {
	_, err := r.requireAdmin(ctx)
	if err != nil {
		return nil, err
	}

	accountID, username := normalizeAdminAccountID(id)
	if err := common.ValidateRequiredParam("id", accountID); err != nil {
		return nil, err
	}
	if err := common.ValidateRequiredParam("username", username); err != nil {
		return nil, err
	}

	if r.Storage == nil || r.Storage.Account() == nil {
		return nil, ErrStorageUnavailable
	}

	user, err := r.Storage.Account().GetUser(ctx, username)
	if err != nil {
		return nil, errors.Join(errors.New("failed to load user"), err)
	}

	return r.buildAdminAccount(ctx, user, true)
}

// AdminDomainAllows covers GET /api/v1/admin/domain_allows.
func (r *queryResolver) AdminDomainAllows(ctx context.Context, first *int, after *model.Cursor) (*model.AdminDomainAllowConnection, error) {
	return adminListDomainBlockConnection(
		ctx,
		r,
		first,
		after,
		100,
		200,
		func(ctx context.Context, limit int, cursor string) ([]*storage.DomainAllow, string, error) {
			return r.Storage.DomainBlock().GetDomainAllows(ctx, limit, cursor)
		},
		convertDomainAllowToAdminModel,
		buildAdminDomainAllowConnection,
		"failed to list domain allows",
	)
}

// AdminDomainBlocks covers GET /api/v1/admin/domain_blocks.
func (r *queryResolver) AdminDomainBlocks(ctx context.Context, first *int, after *model.Cursor) (*model.AdminDomainBlockConnection, error) {
	_, err := r.requireAdmin(ctx)
	if err != nil {
		return nil, err
	}
	if r.Storage == nil || r.Storage.DomainBlock() == nil {
		return nil, ErrStorageUnavailable
	}

	limit := 100
	if first != nil {
		limit = clampInt(*first, 1, 200)
	}

	blocks, nextCursor, err := r.Storage.DomainBlock().GetDomainBlocks(ctx, limit, cursorToString(after))
	if err != nil {
		return nil, errors.Join(errors.New("failed to list domain blocks"), err)
	}

	out := make([]*model.AdminDomainBlock, 0, len(blocks))
	for _, block := range blocks {
		if block == nil {
			continue
		}
		out = append(out, convertInstanceDomainBlockToAdminBlock(block))
	}

	return &model.AdminDomainBlockConnection{
		Blocks:     out,
		NextCursor: stringToCursor(nextCursor),
	}, nil
}

// AdminDomainBlock covers GET /api/v1/admin/domain_blocks/{id}.
func (r *queryResolver) AdminDomainBlock(ctx context.Context, id string) (*model.AdminDomainBlock, error) {
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

	block, err := r.Storage.DomainBlock().GetDomainBlock(ctx, blockID)
	if err != nil {
		return nil, errors.Join(errors.New("failed to load domain block"), err)
	}

	return convertInstanceDomainBlockToAdminBlock(block), nil
}

// AdminEmailDomainBlocks covers GET /api/v1/admin/email_domain_blocks.
func (r *queryResolver) AdminEmailDomainBlocks(ctx context.Context, first *int, after *model.Cursor) (*model.AdminEmailDomainBlockConnection, error) {
	return adminListDomainBlockConnection(
		ctx,
		r,
		first,
		after,
		100,
		200,
		func(ctx context.Context, limit int, cursor string) ([]*storage.EmailDomainBlock, string, error) {
			return r.Storage.DomainBlock().GetEmailDomainBlocks(ctx, limit, cursor)
		},
		convertEmailDomainBlockToAdminModel,
		buildAdminEmailDomainBlockConnection,
		"failed to list email domain blocks",
	)
}

func adminListDomainBlockConnection[T any, R any, C any](
	ctx context.Context,
	r *queryResolver,
	first *int,
	after *model.Cursor,
	defaultLimit int,
	maxLimit int,
	listFn func(ctx context.Context, limit int, cursor string) ([]T, string, error),
	convertFn func(item T) (R, bool),
	buildConn func(items []R, nextCursor *model.Cursor) C,
	errMessage string,
) (C, error) {
	var zero C

	_, err := r.requireAdmin(ctx)
	if err != nil {
		return zero, err
	}
	if r.Storage == nil || r.Storage.DomainBlock() == nil {
		return zero, ErrStorageUnavailable
	}

	items, nextCursor, err := listAdminCursorConnection(
		ctx,
		first,
		after,
		defaultLimit,
		maxLimit,
		listFn,
		convertFn,
	)
	if err != nil {
		return zero, errors.Join(errors.New(errMessage), err)
	}

	return buildConn(items, stringToCursor(nextCursor)), nil
}

func convertDomainAllowToAdminModel(allow *storage.DomainAllow) (*model.AdminDomainAllow, bool) {
	if allow == nil {
		return nil, false
	}
	return &model.AdminDomainAllow{
		ID:        allow.ID,
		Domain:    allow.Domain,
		CreatedAt: model.Time(allow.CreatedAt),
	}, true
}

func buildAdminDomainAllowConnection(allows []*model.AdminDomainAllow, nextCursor *model.Cursor) *model.AdminDomainAllowConnection {
	return &model.AdminDomainAllowConnection{
		Allows:     allows,
		NextCursor: nextCursor,
	}
}

func convertEmailDomainBlockToAdminModel(block *storage.EmailDomainBlock) (*model.AdminEmailDomainBlock, bool) {
	if block == nil {
		return nil, false
	}
	return &model.AdminEmailDomainBlock{
		ID:        block.ID,
		Domain:    block.Domain,
		CreatedAt: model.Time(block.CreatedAt),
	}, true
}

func buildAdminEmailDomainBlockConnection(blocks []*model.AdminEmailDomainBlock, nextCursor *model.Cursor) *model.AdminEmailDomainBlockConnection {
	return &model.AdminEmailDomainBlockConnection{
		Blocks:     blocks,
		NextCursor: nextCursor,
	}
}

func listAdminCursorConnection[T any, R any](
	ctx context.Context,
	first *int,
	after *model.Cursor,
	defaultLimit int,
	maxLimit int,
	listFn func(ctx context.Context, limit int, cursor string) ([]T, string, error),
	convertFn func(item T) (R, bool),
) ([]R, string, error) {
	limit := defaultLimit
	if first != nil {
		limit = clampInt(*first, 1, maxLimit)
	}

	items, nextCursor, err := listFn(ctx, limit, cursorToString(after))
	if err != nil {
		var empty []R
		return empty, "", err
	}

	out := make([]R, 0, len(items))
	for _, item := range items {
		converted, ok := convertFn(item)
		if !ok {
			continue
		}
		out = append(out, converted)
	}

	return out, nextCursor, nil
}

// AdminFederationInstances covers GET /api/v1/admin/federation/instances.
func (r *queryResolver) AdminFederationInstances(ctx context.Context, first *int, after *model.Cursor) (*model.AdminFederationInstanceConnection, error) {
	_, err := r.requireAdmin(ctx)
	if err != nil {
		return nil, err
	}
	if r.Storage == nil || r.Storage.Federation() == nil || r.Storage.DomainBlock() == nil {
		return nil, ErrStorageUnavailable
	}

	limit := 100
	if first != nil {
		limit = clampInt(*first, 1, 200)
	}

	instances, nextCursor, err := r.Storage.Federation().GetKnownInstances(ctx, limit, cursorToString(after))
	if err != nil {
		return nil, errors.Join(errors.New("failed to list federation instances"), err)
	}

	out := make([]*model.AdminFederationInstanceInfo, 0, len(instances))
	for _, instance := range instances {
		if instance == nil {
			continue
		}
		out = append(out, r.convertFederationInstanceInfo(ctx, instance))
	}

	return &model.AdminFederationInstanceConnection{
		Instances:  out,
		NextCursor: stringToCursor(nextCursor),
	}, nil
}

// AdminFederationInstance covers GET /api/v1/admin/federation/instance/{domain}.
func (r *queryResolver) AdminFederationInstance(ctx context.Context, domain string) (*model.AdminFederationInstance, error) {
	_, err := r.requireAdmin(ctx)
	if err != nil {
		return nil, err
	}

	domain = cleanDomainValue(domain)
	if err := common.ValidateRequiredParam("domain", domain); err != nil {
		return nil, err
	}

	if r.Storage == nil || r.Storage.Federation() == nil {
		return nil, ErrStorageUnavailable
	}

	instance, err := r.Storage.Federation().GetInstanceInfo(ctx, domain)
	if err != nil {
		return nil, errors.Join(errors.New("failed to load federation instance"), err)
	}

	details := (*string)(nil)
	if r.Storage.Instance() != nil {
		stats, err := r.Storage.Instance().GetDomainStats(ctx, domain)
		if err == nil {
			details = toJSONPointer(stats)
		}
	}

	return &model.AdminFederationInstance{
		Instance:    r.convertFederationInstanceInfo(ctx, instance),
		DetailsJSON: details,
	}, nil
}

// AdminFederationStatistics covers GET /api/v1/admin/federation/statistics.
func (r *queryResolver) AdminFederationStatistics(ctx context.Context, start *model.Time, end *model.Time) (*model.AdminFederationStatistics, error) {
	_, err := r.requireAdmin(ctx)
	if err != nil {
		return nil, err
	}

	if r.Storage == nil || r.Storage.Federation() == nil {
		return nil, ErrStorageUnavailable
	}

	endTime := time.Now()
	startTime := endTime.AddDate(0, 0, -7)
	if start != nil {
		startTime = time.Time(*start)
	}
	if end != nil {
		endTime = time.Time(*end)
	}

	stats, err := r.Storage.Federation().GetFederationStatistics(ctx, startTime, endTime)
	if err != nil {
		return nil, errors.Join(errors.New("failed to load federation statistics"), err)
	}

	return &model.AdminFederationStatistics{
		ActiveInstances: int(stats.ActiveInstances),
		TotalMessages:   int(stats.TotalMessages),
		TotalUsers:      int(stats.TotalUsers),
		TimeRange: &model.AdminFederationStatisticsTimeRange{
			Start: model.Time(startTime),
			End:   model.Time(endTime),
		},
	}, nil
}

// AdminModerationEvents covers GET /api/v1/admin/moderation/events.
func (r *queryResolver) AdminModerationEvents(ctx context.Context, filter *model.AdminModerationEventFilter, first *int, after *model.Cursor) (*model.AdminModerationEventConnection, error) {
	_, err := r.requireAdmin(ctx)
	if err != nil {
		return nil, err
	}
	if r.Storage == nil || r.Storage.Moderation() == nil {
		return nil, ErrModerationRepositoryUnavailable
	}

	limit := 50
	if first != nil {
		limit = clampInt(*first, 1, 100)
	}

	storageFilter := &storage.ModerationEventFilter{}
	if filter != nil {
		if filter.EventType != nil {
			storageFilter.EventType = strings.TrimSpace(*filter.EventType)
		}
		if filter.Category != nil {
			storageFilter.Category = strings.TrimSpace(*filter.Category)
		}
		if filter.MinSeverity != nil {
			storageFilter.MinSeverity = filter.MinSeverity
		}
		if filter.ActorID != nil {
			storageFilter.ActorID = strings.TrimSpace(*filter.ActorID)
		}
		if filter.ObjectID != nil {
			storageFilter.ObjectID = strings.TrimSpace(*filter.ObjectID)
		}
	}

	events, nextCursor, err := r.Storage.Moderation().GetModerationEvents(ctx, storageFilter, limit, cursorToString(after))
	if err != nil {
		return nil, errors.Join(errors.New("failed to list moderation events"), err)
	}

	out := make([]*model.AdminModerationEvent, 0, len(events))
	for _, event := range events {
		if event == nil {
			continue
		}
		out = append(out, convertModerationEventToAdminEvent(event))
	}

	return &model.AdminModerationEventConnection{
		Events:     out,
		NextCursor: stringToCursor(nextCursor),
	}, nil
}

// AdminModerationReviewers covers GET /api/v1/admin/moderation/reviewers.
func (r *queryResolver) AdminModerationReviewers(ctx context.Context) ([]*model.AdminReviewer, error) {
	_, err := r.requireAdmin(ctx)
	if err != nil {
		return nil, err
	}

	if r.Storage == nil || r.Storage.User() == nil || r.Storage.Moderation() == nil {
		return nil, ErrStorageUnavailable
	}

	moderators, err := r.Storage.User().ListUsersByRole(ctx, adminRoleModerator)
	if err != nil {
		return nil, errors.Join(errors.New("failed to list moderators"), err)
	}

	admins, err := r.Storage.User().ListUsersByRole(ctx, adminRoleAdmin)
	if err != nil {
		return nil, errors.Join(errors.New("failed to list admins"), err)
	}

	users := append(moderators, admins...)
	out := make([]*model.AdminReviewer, 0, len(users))
	for _, user := range users {
		if user == nil {
			continue
		}
		reviewer, err := r.buildAdminReviewer(ctx, user.Username, user.Role)
		if err != nil {
			r.Logger.Warn("failed to build reviewer",
				zap.String("username", user.Username),
				zap.Error(err))
			continue
		}
		out = append(out, reviewer)
	}

	return out, nil
}

// AdminTrustGraph covers GET /api/v1/admin/moderation/trust/graph.
func (r *queryResolver) AdminTrustGraph(ctx context.Context, limit *int) (*model.AdminTrustGraph, error) {
	_, err := r.requireAdmin(ctx)
	if err != nil {
		return nil, err
	}

	if r.Storage == nil || r.Storage.Trust() == nil {
		return nil, ErrTrustRepositoryUnavailable
	}

	safeLimit := 100
	if limit != nil {
		safeLimit = clampInt(*limit, 1, 500)
	}

	relationships, err := r.Storage.Trust().GetAllTrustRelationships(ctx, safeLimit)
	if err != nil {
		return nil, errors.Join(errors.New("failed to load trust relationships"), err)
	}

	return buildAdminTrustGraph(relationships), nil
}

// AdminReports covers GET /api/v1/admin/reports.
func (r *queryResolver) AdminReports(ctx context.Context, status *model.AdminReportStatus, first *int, after *model.Cursor) (*model.AdminReportConnection, error) {
	_, err := r.requireAdmin(ctx)
	if err != nil {
		return nil, err
	}
	if r.Storage == nil || r.Storage.Moderation() == nil {
		return nil, ErrModerationRepositoryUnavailable
	}

	limit := 50
	if first != nil {
		limit = clampInt(*first, 1, 100)
	}

	storageStatus := storage.ReportStatusOpen
	if status != nil {
		storageStatus = mapAdminReportStatus(*status)
	}

	reports, nextCursor, err := r.Storage.Moderation().GetReportsByStatus(ctx, storageStatus, limit, cursorToString(after))
	if err != nil {
		return nil, errors.Join(errors.New("failed to list reports"), err)
	}

	out := make([]*model.AdminReport, 0, len(reports))
	for _, report := range reports {
		if report == nil {
			continue
		}
		converted := r.convertReportToAdminReport(ctx, report)
		if converted != nil {
			out = append(out, converted)
		}
	}

	return &model.AdminReportConnection{
		Reports:    out,
		NextCursor: stringToCursor(nextCursor),
	}, nil
}

// AdminReport covers GET /api/v1/admin/reports/{id}.
func (r *queryResolver) AdminReport(ctx context.Context, id string) (*model.AdminReport, error) {
	_, err := r.requireAdmin(ctx)
	if err != nil {
		return nil, err
	}

	reportID := strings.TrimSpace(id)
	if err := common.ValidateRequiredParam("id", reportID); err != nil {
		return nil, err
	}

	if r.Storage == nil || r.Storage.Moderation() == nil {
		return nil, ErrModerationRepositoryUnavailable
	}

	report, err := r.Storage.Moderation().GetReport(ctx, reportID)
	if err != nil {
		return nil, errors.Join(errors.New("failed to load report"), err)
	}

	return r.convertReportToAdminReport(ctx, report), nil
}

// AdminStatuses covers GET /api/v1/admin/statuses.
func (r *queryResolver) AdminStatuses(ctx context.Context, filter *model.AdminStatusFilter, first *int, after *model.Cursor) (*model.AdminStatusConnection, error) {
	_, err := r.requireAdmin(ctx)
	if err != nil {
		return nil, err
	}

	if r.Storage == nil || r.Storage.Status() == nil {
		return nil, ErrStatusRepositoryUnavailable
	}

	limit := 20
	if first != nil {
		limit = clampInt(*first, 1, 100)
	}

	storageFilter := buildAdminStatusFilter(filter)

	statuses, nextCursor, err := r.Storage.Status().ListStatusesForAdmin(ctx, storageFilter, limit, cursorToString(after))
	if err != nil {
		return nil, errors.Join(errors.New("failed to list statuses"), err)
	}

	out := make([]*model.Object, 0, len(statuses))
	for _, status := range statuses {
		if status == nil {
			continue
		}
		out = append(out, r.convertStatusToObject(ctx, status))
	}

	return &model.AdminStatusConnection{
		Statuses:   out,
		NextCursor: stringToCursor(nextCursor),
	}, nil
}

// AdminStatus covers GET /api/v1/admin/statuses/{id}.
func (r *queryResolver) AdminStatus(ctx context.Context, id string) (*model.Object, error) {
	_, err := r.requireAdmin(ctx)
	if err != nil {
		return nil, err
	}

	statusID := strings.TrimSpace(id)
	if err := common.ValidateRequiredParam("id", statusID); err != nil {
		return nil, err
	}

	if r.Storage == nil || r.Storage.Status() == nil {
		return nil, ErrStatusRepositoryUnavailable
	}

	status, err := r.Storage.Status().GetStatus(ctx, statusID)
	if err != nil {
		return nil, errors.Join(errors.New("failed to load status"), err)
	}

	return r.convertStatusToObject(ctx, status), nil
}

func (r *queryResolver) buildAdminAccount(ctx context.Context, user *storage.User, includeReportStats bool) (*model.AdminAccount, error) {
	if user == nil {
		return nil, errors.New("user is nil")
	}

	accountID, username := normalizeAdminAccountID(user.Username)

	var actor *activitypub.Actor
	if r.Storage != nil && r.Storage.Actor() != nil {
		var err error
		actor, err = r.Storage.Actor().GetActor(ctx, username)
		if err != nil {
			actor = nil
		}
	}

	var sessions []*storage.Session
	if r.Storage != nil && r.Storage.Account() != nil {
		var err error
		sessions, err = r.Storage.Account().GetUserSessions(ctx, username)
		if err != nil {
			sessions = nil
		}
	}
	lastIP, ipHistory := deriveAdminIPInfo(sessions)

	reportsCount := 0
	resolvedReportsCount := 0
	if includeReportStats && r.Storage != nil && r.Storage.Moderation() != nil {
		stats, err := r.Storage.Moderation().GetReportStats(ctx, username)
		if err == nil && stats != nil {
			reportsCount = stats.TotalReports
			resolvedReportsCount = stats.ResolvedReports
		}
	}

	locale := strings.TrimSpace(user.Locale)
	if locale == "" {
		locale = "en"
	}

	return &model.AdminAccount{
		ID:                   accountID,
		Username:             username,
		CreatedAt:            model.Time(user.CreatedAt),
		Locale:               locale,
		IP:                   lastIP,
		Ips:                  ipHistory,
		Role:                 adminRoleFromName(user.Role),
		Confirmed:            user.Approved,
		Approved:             user.Approved,
		Disabled:             !user.Approved,
		Silenced:             user.Silenced,
		Suspended:            user.Suspended,
		Actor:                actor,
		ReportsCount:         reportsCount,
		ResolvedReportsCount: resolvedReportsCount,
	}, nil
}

func convertInstanceDomainBlockToAdminBlock(block *storage.InstanceDomainBlock) *model.AdminDomainBlock {
	if block == nil {
		return nil
	}

	return &model.AdminDomainBlock{
		ID:             block.ID,
		Domain:         block.Domain,
		Severity:       block.Severity,
		RejectMedia:    block.RejectMedia,
		RejectReports:  block.RejectReports,
		PrivateComment: optionalString(block.PrivateComment),
		PublicComment:  optionalString(block.PublicComment),
		Obfuscate:      block.Obfuscate,
		CreatedAt:      model.Time(block.CreatedAt),
		UpdatedAt:      model.Time(block.UpdatedAt),
	}
}

func (r *queryResolver) convertFederationInstanceInfo(ctx context.Context, instance *storage.InstanceInfo) *model.AdminFederationInstanceInfo {
	if instance == nil {
		return nil
	}

	isSilenced := false
	isSuspended := false
	if r.Storage != nil && r.Storage.DomainBlock() != nil {
		blocked, block, err := r.Storage.DomainBlock().IsDomainBlocked(ctx, instance.Domain)
		if err == nil && blocked && block != nil {
			isSilenced = block.Severity == DomainBlockSeveritySilence
			isSuspended = block.Severity == DomainBlockSeveritySuspend
		}
	}

	return &model.AdminFederationInstanceInfo{
		Domain:        instance.Domain,
		Software:      optionalString(instance.Software),
		Version:       optionalString(instance.Version),
		ActiveUsers:   int(instance.ActiveUsers),
		TotalMessages: int(instance.TotalMessages),
		TrustScore:    instance.TrustScore,
		FirstSeen:     model.Time(instance.FirstSeen),
		LastSeen:      model.Time(instance.LastSeen),
		IsSilenced:    isSilenced,
		IsSuspended:   isSuspended,
	}
}

func convertModerationEventToAdminEvent(event *storage.ModerationEvent) *model.AdminModerationEvent {
	if event == nil {
		return nil
	}

	return &model.AdminModerationEvent{
		ID:              event.ID,
		EventType:       event.EventType,
		ActorID:         event.ActorID,
		ObjectID:        event.ObjectID,
		ObjectType:      event.ObjectType,
		Category:        event.Category,
		Severity:        event.Severity,
		Reason:          optionalString(event.Reason),
		EvidenceJSON:    toJSONPointer(event.Evidence),
		ConfidenceScore: event.ConfidenceScore,
		CreatedAt:       model.Time(event.Created),
	}
}

func (r *queryResolver) buildAdminReviewer(ctx context.Context, username string, role string) (*model.AdminReviewer, error) {
	if r.Storage == nil || r.Storage.Moderation() == nil {
		return nil, ErrModerationRepositoryUnavailable
	}

	stats, err := r.Storage.Moderation().GetReviewerStats(ctx, username)
	if err != nil {
		return nil, err
	}

	var lastReviewAt *model.Time
	if stats != nil && !stats.LastReviewAt.IsZero() {
		t := model.Time(stats.LastReviewAt)
		lastReviewAt = &t
	}

	userID, _ := normalizeAdminAccountID(username)

	return &model.AdminReviewer{
		ID:              userID,
		Username:        username,
		Role:            role,
		TotalReviews:    stats.TotalReviews,
		AccurateReviews: stats.AccurateReviews,
		AccuracyRate:    stats.AccuracyRate,
		LastReviewAt:    lastReviewAt,
	}, nil
}

func buildAdminTrustGraph(relationships []*storage.TrustRelationship) *model.AdminTrustGraph {
	nodes := make(map[string]*model.AdminTrustGraphNode)
	edges := make([]*model.AdminTrustGraphEdge, 0, len(relationships))

	for _, rel := range relationships {
		if rel == nil {
			continue
		}

		if _, ok := nodes[rel.TrusterID]; !ok {
			nodes[rel.TrusterID] = &model.AdminTrustGraphNode{ID: rel.TrusterID, Type: "actor"}
		}
		if _, ok := nodes[rel.TrusteeID]; !ok {
			nodes[rel.TrusteeID] = &model.AdminTrustGraphNode{ID: rel.TrusteeID, Type: "actor"}
		}

		edges = append(edges, &model.AdminTrustGraphEdge{
			From:      rel.TrusterID,
			To:        rel.TrusteeID,
			Trust:     rel.Score,
			CreatedAt: model.Time(rel.Created),
			UpdatedAt: model.Time(rel.Updated),
		})
	}

	nodeList := make([]*model.AdminTrustGraphNode, 0, len(nodes))
	for _, node := range nodes {
		nodeList = append(nodeList, node)
	}

	return &model.AdminTrustGraph{
		Nodes: nodeList,
		Edges: edges,
		Stats: &model.AdminTrustGraphStats{
			TotalNodes: len(nodes),
			TotalEdges: len(edges),
		},
	}
}

func mapAdminReportStatus(status model.AdminReportStatus) storage.ReportStatus {
	switch status {
	case model.AdminReportStatusResolved:
		return storage.ReportStatusResolved
	case model.AdminReportStatusRejected:
		return storage.ReportStatusRejected
	default:
		return storage.ReportStatusOpen
	}
}

func buildAdminStatusFilter(filter *model.AdminStatusFilter) *interfaces.StatusFilter {
	if filter == nil {
		return &interfaces.StatusFilter{}
	}

	out := &interfaces.StatusFilter{}

	if filter.Local != nil && *filter.Local {
		local := true
		out.Local = &local
	}
	if filter.Remote != nil && *filter.Remote {
		remote := true
		out.Remote = &remote
	}
	if filter.ByDomain != nil {
		out.ByDomain = strings.TrimSpace(*filter.ByDomain)
	}
	if filter.Visibility != nil {
		out.Visibility = strings.TrimSpace(*filter.Visibility)
	}

	out.Flagged = filter.Flagged
	out.Reported = filter.Reported

	if filter.Media != nil {
		out.WithMedia = filter.Media
	}
	out.Sensitive = filter.Sensitive

	if filter.MinDate != nil {
		t := time.Time(*filter.MinDate)
		out.MinDate = &t
	}
	if filter.MaxDate != nil {
		t := time.Time(*filter.MaxDate)
		out.MaxDate = &t
	}

	return out
}

func (r *queryResolver) convertReportToAdminReport(ctx context.Context, report *storage.Report) *model.AdminReport {
	if report == nil {
		return nil
	}

	reporter := r.resolveActorByID(ctx, report.ReporterID)
	target := r.resolveActorByID(ctx, report.TargetAccountID)

	var assigned *activitypub.Actor
	if strings.TrimSpace(report.AssignedTo) != "" {
		assigned = r.resolveActorByID(ctx, report.AssignedTo)
	}

	var actionTakenBy *activitypub.Actor
	if strings.TrimSpace(report.ModeratorID) != "" {
		actionTakenBy = r.resolveActorByID(ctx, report.ModeratorID)
	}

	statuses := r.loadReportStatuses(ctx, report.StatusIDs)

	actionTakenAt := (*model.Time)(nil)
	if report.ActionTakenAt != nil {
		t := model.Time(*report.ActionTakenAt)
		actionTakenAt = &t
	}

	comment := optionalString(report.Comment)

	return &model.AdminReport{
		ID:              report.ID,
		ActionTaken:     strings.TrimSpace(report.ActionTaken) != "",
		ActionTakenAt:   actionTakenAt,
		Category:        report.Category,
		Comment:         comment,
		Forwarded:       report.Forwarded,
		CreatedAt:       model.Time(report.CreatedAt),
		UpdatedAt:       model.Time(report.UpdatedAt),
		Reporter:        reporter,
		Target:          target,
		AssignedAccount: assigned,
		ActionTakenBy:   actionTakenBy,
		Statuses:        statuses,
	}
}

func (r *queryResolver) loadReportStatuses(ctx context.Context, statusIDs []string) []*model.Object {
	if len(statusIDs) == 0 || r.Storage == nil || r.Storage.Status() == nil {
		return []*model.Object{}
	}

	out := make([]*model.Object, 0, len(statusIDs))
	for _, statusID := range statusIDs {
		statusID = strings.TrimSpace(statusID)
		if statusID == "" {
			continue
		}
		status, err := r.Storage.Status().GetStatus(ctx, statusID)
		if err != nil || status == nil {
			continue
		}
		out = append(out, r.convertStatusToObject(ctx, status))
	}
	return out
}

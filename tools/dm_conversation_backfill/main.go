// Package main backfills conversation records for orphaned direct-message statuses.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/deploy/naming"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/google/uuid"
	"github.com/theory-cloud/tabletheory"
	ttErrors "github.com/theory-cloud/tabletheory/pkg/errors"
	"github.com/theory-cloud/tabletheory/pkg/session"
	"go.uber.org/zap"
)

type runConfig struct {
	App        string
	Stage      naming.Stage
	AWSProfile string
	TableName  string
	Apply      bool
}

type backfillStats struct {
	ScannedStatuses          int
	StatusesAlreadyLinked    int
	StatusesSkipped          int
	StatusesNormalized       int
	ConversationGroups       int
	ConversationsCreated     int
	ConversationsUpdated     int
	ParticipantRecordsSynced int
	ConversationStatusesUp   int
}

type repairGroup struct {
	ParticipantKey        string
	Participants          []string
	Statuses              []*models.Status
	ExistingConversation  *models.Conversation
	LatestPublishedStatus *models.Status
}

type backfiller struct {
	statusRepo       *repositories.StatusRepository
	conversationRepo *repositories.ConversationRepository
	logger           *zap.Logger
	apply            bool
	now              func() time.Time
}

func main() {
	if err := run(context.Background(), os.Args[1:], os.Stderr); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, argv []string, stderr *os.File) error {
	cfg, err := parseFlags(argv)
	if err != nil {
		return err
	}

	awsCfg, err := loadAWSConfig(ctx, cfg.AWSProfile)
	if err != nil {
		return err
	}

	db, err := tabletheory.New(session.Config{
		Region:              awsCfg.Region,
		CredentialsProvider: awsCfg.Credentials,
	})
	if err != nil {
		return fmt.Errorf("open tabletheory session: %w", err)
	}
	defer func() { _ = db.Close() }()

	prevTableName := models.MainTableName
	models.MainTableName = cfg.TableName
	defer func() {
		models.MainTableName = prevTableName
	}()

	logger := zap.NewExample().With(
		zap.String("tool", "dm_conversation_backfill"),
		zap.String("table", cfg.TableName),
		zap.String("app", cfg.App),
		zap.String("stage", string(cfg.Stage)),
	)

	b := &backfiller{
		statusRepo:       repositories.NewStatusRepository(db, cfg.TableName, logger, nil),
		conversationRepo: repositories.NewConversationRepository(db, cfg.TableName, logger, nil),
		logger:           logger,
		apply:            cfg.Apply,
		now:              func() time.Time { return time.Now().UTC() },
	}

	stats, err := b.run(ctx)
	if err != nil {
		return err
	}

	mode := "dry-run"
	if cfg.Apply {
		mode = "apply"
	}

	fmt.Fprintf(stderr, "dm conversation backfill (%s)\n", mode)
	fmt.Fprintf(stderr, "  table: %s\n", cfg.TableName)
	fmt.Fprintf(stderr, "  statuses scanned: %d\n", stats.ScannedStatuses)
	fmt.Fprintf(stderr, "  statuses already linked: %d\n", stats.StatusesAlreadyLinked)
	fmt.Fprintf(stderr, "  statuses skipped: %d\n", stats.StatusesSkipped)
	fmt.Fprintf(stderr, "  statuses normalized: %d\n", stats.StatusesNormalized)
	fmt.Fprintf(stderr, "  conversation groups: %d\n", stats.ConversationGroups)
	fmt.Fprintf(stderr, "  conversations created: %d\n", stats.ConversationsCreated)
	fmt.Fprintf(stderr, "  conversations updated: %d\n", stats.ConversationsUpdated)
	fmt.Fprintf(stderr, "  participant records synced: %d\n", stats.ParticipantRecordsSynced)
	fmt.Fprintf(stderr, "  conversation statuses upserted: %d\n", stats.ConversationStatusesUp)

	return nil
}

func parseFlags(argv []string) (runConfig, error) {
	fs := flag.NewFlagSet("dm_conversation_backfill", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var app string
	var stage string
	var awsProfile string
	var tableName string
	var apply bool

	fs.StringVar(&app, "app", envOrDefault("LESSER_APP", ""), "app slug (default: lesser)")
	fs.StringVar(&stage, "stage", envOrDefault("LESSER_STAGE", "dev"), "deployment stage (dev|staging|live)")
	fs.StringVar(&awsProfile, "aws-profile", os.Getenv("AWS_PROFILE"), "AWS shared config profile")
	fs.StringVar(&tableName, "table", "", "explicit DynamoDB table name override")
	fs.BoolVar(&apply, "apply", false, "persist the backfill instead of reporting what would change")

	if err := fs.Parse(argv); err != nil {
		return runConfig{}, err
	}

	appName := strings.TrimSpace(app)
	if appName == "" {
		appName = naming.DefaultAppName
	}
	normalizedApp, err := naming.NormalizeAppName(appName)
	if err != nil {
		return runConfig{}, err
	}

	normalizedStage := naming.StageForEnvironment(stage)
	switch normalizedStage {
	case naming.StageDev, naming.StageStaging, naming.StageLive:
	default:
		return runConfig{}, fmt.Errorf("invalid --stage %q (expected dev|staging|live)", stage)
	}

	resolvedTable := strings.TrimSpace(tableName)
	if resolvedTable == "" {
		resolvedTable = naming.ResourceNameWithApp(normalizedApp, "main-table", string(normalizedStage))
	}

	return runConfig{
		App:        normalizedApp,
		Stage:      normalizedStage,
		AWSProfile: strings.TrimSpace(awsProfile),
		TableName:  resolvedTable,
		Apply:      apply,
	}, nil
}

func loadAWSConfig(ctx context.Context, profile string) (aws.Config, error) {
	opts := []func(*awsconfig.LoadOptions) error{}
	if strings.TrimSpace(profile) != "" {
		opts = append(opts, awsconfig.WithSharedConfigProfile(profile))
	}

	cfg, err := awsconfig.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return aws.Config{}, fmt.Errorf("load AWS config: %w", err)
	}
	if strings.TrimSpace(cfg.Region) == "" {
		return aws.Config{}, errors.New("AWS region is required")
	}

	return cfg, nil
}

func (b *backfiller) run(ctx context.Context) (backfillStats, error) {
	statuses, err := b.scanDirectStatuses(ctx)
	if err != nil {
		return backfillStats{}, err
	}

	stats := backfillStats{ScannedStatuses: len(statuses)}

	conversationExists := make(map[string]bool)
	participantConversation := make(map[string]*models.Conversation)
	groups := make(map[string]*repairGroup)

	for i := range statuses {
		status := statuses[i]
		if status == nil {
			continue
		}

		linked, err := b.statusConversationExists(ctx, status, conversationExists)
		if err != nil {
			return stats, err
		}
		if linked {
			stats.StatusesAlreadyLinked++
			continue
		}

		participants, ok := participantsForStatus(status)
		if !ok {
			stats.StatusesSkipped++
			b.logger.Warn("skipping orphan DM without a recoverable participant set",
				zap.String("status_id", status.StatusID),
				zap.String("author", status.AuthorUsername),
				zap.String("content", status.Content))
			continue
		}

		participantKey := strings.Join(participants, ",")
		group := groups[participantKey]
		if group == nil {
			group = &repairGroup{
				ParticipantKey: participantKey,
				Participants:   participants,
			}
			if cached, ok := participantConversation[participantKey]; ok {
				group.ExistingConversation = cached
			} else {
				conversation, err := b.lookupConversationByParticipants(ctx, participants)
				if err != nil {
					return stats, err
				}
				group.ExistingConversation = conversation
				participantConversation[participantKey] = conversation
			}
			groups[participantKey] = group
		}

		group.Statuses = append(group.Statuses, status)
		if group.LatestPublishedStatus == nil || statusPublishedAt(status).After(statusPublishedAt(group.LatestPublishedStatus)) {
			group.LatestPublishedStatus = status
		}
	}

	stats.ConversationGroups = len(groups)

	keys := make([]string, 0, len(groups))
	for key := range groups {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, key := range keys {
		group := groups[key]
		groupStats, err := b.repairGroup(ctx, group)
		if err != nil {
			return stats, err
		}
		stats.StatusesNormalized += groupStats.StatusesNormalized
		stats.ConversationsCreated += groupStats.ConversationsCreated
		stats.ConversationsUpdated += groupStats.ConversationsUpdated
		stats.ParticipantRecordsSynced += groupStats.ParticipantRecordsSynced
		stats.ConversationStatusesUp += groupStats.ConversationStatusesUp
	}

	return stats, nil
}

func (b *backfiller) scanDirectStatuses(ctx context.Context) ([]*models.Status, error) {
	var statuses []models.Status
	err := b.statusRepo.GetDB().WithContext(ctx).
		Model(&models.Status{}).
		Filter("Visibility", "=", models.VisibilityDirect).
		Filter("Deleted", "=", false).
		Scan(&statuses)
	if err != nil {
		return nil, fmt.Errorf("scan direct statuses: %w", err)
	}

	result := make([]*models.Status, 0, len(statuses))
	for i := range statuses {
		status := statuses[i]
		result = append(result, &status)
	}

	return result, nil
}

func (b *backfiller) statusConversationExists(ctx context.Context, status *models.Status, cache map[string]bool) (bool, error) {
	if status == nil || strings.TrimSpace(status.ConversationID) == "" {
		return false, nil
	}

	if exists, ok := cache[status.ConversationID]; ok {
		return exists, nil
	}

	_, err := b.conversationRepo.GetConversation(ctx, status.ConversationID)
	if err == nil {
		cache[status.ConversationID] = true
		return true, nil
	}
	if isNotFound(err) {
		cache[status.ConversationID] = false
		return false, nil
	}

	return false, fmt.Errorf("lookup conversation %s for status %s: %w", status.ConversationID, status.StatusID, err)
}

func (b *backfiller) lookupConversationByParticipants(ctx context.Context, participants []string) (*models.Conversation, error) {
	conversation, err := b.conversationRepo.GetConversationByParticipants(ctx, participants)
	if err == nil {
		return conversation, nil
	}
	if isNotFound(err) {
		return nil, nil
	}
	return nil, fmt.Errorf("lookup conversation by participants %q: %w", strings.Join(participants, ","), err)
}

func (b *backfiller) repairGroup(ctx context.Context, group *repairGroup) (backfillStats, error) {
	if group == nil || len(group.Statuses) == 0 {
		return backfillStats{}, nil
	}

	stats := backfillStats{}
	sortRepairGroupStatuses(group)
	conversationID := conversationIDForGroup(group)

	normalizedCount, err := b.normalizeGroupStatuses(ctx, group.Statuses, conversationID)
	if err != nil {
		return stats, err
	}
	stats.StatusesNormalized = normalizedCount

	thread, err := b.loadConversationThread(ctx, conversationID, group)
	if err != nil {
		return stats, err
	}

	conv := buildConversationRecord(conversationID, group.Participants, thread, group.ExistingConversation)
	if !b.apply {
		recordGroupConversationStats(&stats, group.ExistingConversation == nil, len(group.Participants))
		return stats, nil
	}

	if err := b.persistGroupConversation(ctx, group, conv, thread, &stats); err != nil {
		return stats, err
	}

	return stats, nil
}

func sortRepairGroupStatuses(group *repairGroup) {
	sort.Slice(group.Statuses, func(i, j int) bool {
		left := statusPublishedAt(group.Statuses[i])
		right := statusPublishedAt(group.Statuses[j])
		if left.Equal(right) {
			return group.Statuses[i].StatusID < group.Statuses[j].StatusID
		}
		return left.Before(right)
	})
}

func conversationIDForGroup(group *repairGroup) string {
	if group.ExistingConversation != nil {
		return group.ExistingConversation.ID
	}
	return uuid.NewString()
}

func (b *backfiller) normalizeGroupStatuses(ctx context.Context, statuses []*models.Status, conversationID string) (int, error) {
	normalizedCount := 0
	for _, status := range statuses {
		if !statusNeedsConversationNormalization(status, conversationID) {
			continue
		}
		if !b.apply {
			normalizedCount++
			continue
		}
		if err := b.normalizeStatusConversation(ctx, status, conversationID); err != nil {
			return normalizedCount, err
		}
		normalizedCount++
	}
	return normalizedCount, nil
}

func recordGroupConversationStats(stats *backfillStats, creating bool, participantCount int) {
	if creating {
		stats.ConversationsCreated++
		stats.ParticipantRecordsSynced += participantCount
		stats.ConversationStatusesUp += participantCount
		return
	}
	stats.ConversationsUpdated++
}

func (b *backfiller) persistGroupConversation(ctx context.Context, group *repairGroup, conv *models.Conversation, thread []*models.Status, stats *backfillStats) error {
	if group.ExistingConversation == nil {
		return b.createBackfilledConversation(ctx, conv, thread, stats)
	}
	if err := b.conversationRepo.UpdateConversation(ctx, conv); err != nil {
		return fmt.Errorf("update conversation %s: %w", conv.ID, err)
	}
	stats.ConversationsUpdated++
	return nil
}

func (b *backfiller) createBackfilledConversation(ctx context.Context, conv *models.Conversation, thread []*models.Status, stats *backfillStats) error {
	if err := b.conversationRepo.CreateConversation(ctx, conv, conv.Participants); err != nil {
		return fmt.Errorf("create conversation %s: %w", conv.ID, err)
	}
	stats.ConversationsCreated++

	latest := thread[0]
	firstPublished := statusPublishedAt(thread[len(thread)-1])
	if err := b.syncNewConversationParticipantRecords(ctx, conv.ID, conv.Participants, latest.AuthorUsername, latest.PublishedAt, firstPublished); err != nil {
		return err
	}
	stats.ParticipantRecordsSynced += len(conv.Participants)

	return b.persistConversationUnreadState(ctx, conv.ID, conv.Participants, latest.AuthorUsername, latest.PublishedAt, stats)
}

func (b *backfiller) persistConversationUnreadState(ctx context.Context, conversationID string, participants []string, lastSender string, latestPublishedAt time.Time, stats *backfillStats) error {
	for _, participant := range participants {
		unread := participant != lastSender
		lastReadAt := latestPublishedAt
		if unread {
			lastReadAt = time.Unix(0, 0).UTC()
		}
		if err := b.upsertConversationStatus(ctx, conversationID, participant, unread, lastReadAt); err != nil {
			return err
		}
		stats.ConversationStatusesUp++
	}
	return nil
}

func (b *backfiller) normalizeStatusConversation(ctx context.Context, status *models.Status, conversationID string) error {
	if status == nil {
		return nil
	}

	publishedAt := statusPublishedAt(status)
	gsi3PK := "CONVERSATION#" + conversationID
	gsi3SK := fmt.Sprintf("%d#%s", publishedAt.Unix(), status.StatusID)

	builder := b.statusRepo.GetDB().WithContext(ctx).
		Model(&models.Status{}).
		Where("PK", "=", statusPrimaryKey(status.StatusID)).
		Where("SK", "=", statusPrimaryKey(status.StatusID)).
		UpdateBuilder().
		Set("ConversationID", conversationID).
		Set("gsi3PK", gsi3PK).
		Set("gsi3SK", gsi3SK).
		Set("UpdatedAt", b.now())

	if status.Note != nil {
		noteCopy := *status.Note
		noteCopy.ConversationID = conversationID
		builder.Set("Note", &noteCopy)
		status.Note = &noteCopy
	}

	if err := builder.Execute(); err != nil {
		return fmt.Errorf("normalize status %s conversation to %s: %w", status.StatusID, conversationID, err)
	}

	status.ConversationID = conversationID
	status.GSI3PK = gsi3PK
	status.GSI3SK = gsi3SK
	return nil
}

func (b *backfiller) loadConversationThread(ctx context.Context, conversationID string, group *repairGroup) ([]*models.Status, error) {
	if !b.apply {
		thread := make([]*models.Status, len(group.Statuses))
		copy(thread, group.Statuses)
		sort.Slice(thread, func(i, j int) bool {
			left := statusPublishedAt(thread[i])
			right := statusPublishedAt(thread[j])
			if left.Equal(right) {
				return thread[i].StatusID > thread[j].StatusID
			}
			return left.After(right)
		})
		return thread, nil
	}

	var thread []*models.Status
	cursor := ""
	for {
		page, err := b.statusRepo.GetConversationThreadReverse(ctx, conversationID, interfaces.PaginationOptions{
			Limit:  100,
			Cursor: cursor,
		})
		if err != nil {
			if isNotFound(err) && len(group.Statuses) > 0 {
				break
			}
			return nil, fmt.Errorf("load conversation thread %s: %w", conversationID, err)
		}

		thread = append(thread, page.Items...)
		if !page.HasMore || strings.TrimSpace(page.NextCursor) == "" {
			break
		}
		cursor = page.NextCursor
	}

	if len(thread) == 0 {
		thread = make([]*models.Status, len(group.Statuses))
		copy(thread, group.Statuses)
		sort.Slice(thread, func(i, j int) bool {
			left := statusPublishedAt(thread[i])
			right := statusPublishedAt(thread[j])
			if left.Equal(right) {
				return thread[i].StatusID > thread[j].StatusID
			}
			return left.After(right)
		})
	}

	return thread, nil
}

func buildConversationRecord(conversationID string, participants []string, thread []*models.Status, existing *models.Conversation) *models.Conversation {
	latest := thread[0]
	earliest := thread[len(thread)-1]
	createdAt := statusPublishedAt(earliest)
	updatedAt := statusPublishedAt(latest)

	conv := &models.Conversation{
		ID:                conversationID,
		Participants:      append([]string(nil), participants...),
		LastStatusID:      latest.StatusID,
		TotalMessageCount: int64(len(thread)),
		LastMessageTime:   updatedAt,
		CreatedAt:         createdAt,
		UpdatedAt:         updatedAt,
	}

	if existing != nil {
		if !existing.CreatedAt.IsZero() && existing.CreatedAt.Before(conv.CreatedAt) {
			conv.CreatedAt = existing.CreatedAt
		}
		if !existing.UpdatedAt.IsZero() && existing.UpdatedAt.After(conv.UpdatedAt) {
			conv.UpdatedAt = existing.UpdatedAt
		}
		if existing.TotalMessageCount > conv.TotalMessageCount {
			conv.TotalMessageCount = existing.TotalMessageCount
		}
	}

	return conv
}

func (b *backfiller) syncNewConversationParticipantRecords(ctx context.Context, conversationID string, participants []string, lastSender string, latestPublishedAt, acceptedAt time.Time) error {
	for _, participant := range participants {
		stateContract, err := b.conversationRepo.GetUserConversationState(ctx, participant, conversationID)
		if err != nil {
			return fmt.Errorf("load user conversation state %s/%s: %w", conversationID, participant, err)
		}

		state := userConversationStateFromContract(stateContract)
		state.RequestState = models.DmRequestStateAccepted
		state.RequestedAt = nil
		state.AcceptedAt = timePtr(acceptedAt)
		state.DeclinedAt = nil
		state.DeletedAt = nil
		state.Unread = participant != lastSender
		if state.Unread {
			state.LastReadAt = nil
		} else {
			state.LastReadAt = timePtr(latestPublishedAt)
		}

		if err := b.conversationRepo.PutUserConversationState(ctx, state); err != nil {
			return fmt.Errorf("update user conversation state %s/%s: %w", conversationID, participant, err)
		}
	}

	return nil
}

func userConversationStateFromContract(state *interfaces.UserConversationStateContract) *models.UserConversationState {
	if state == nil {
		return nil
	}
	return &models.UserConversationState{
		ViewerID:                 state.ViewerID,
		ConversationID:           state.ConversationID,
		CounterpartID:            state.CounterpartID,
		Folder:                   state.Folder,
		RequestState:             state.RequestState,
		PreviewStatusID:          state.PreviewStatusID,
		PreviewStatusPublishedAt: state.PreviewStatusPublishedAt,
		SortAt:                   state.SortAt,
		Unread:                   state.Unread,
		LastReadAt:               state.LastReadAt,
		DeletedAt:                state.DeletedAt,
		RequestedAt:              state.RequestedAt,
		AcceptedAt:               state.AcceptedAt,
		DeclinedAt:               state.DeclinedAt,
		CreatedAt:                state.CreatedAt,
		UpdatedAt:                state.UpdatedAt,
	}
}

func (b *backfiller) upsertConversationStatus(ctx context.Context, conversationID, participant string, unread bool, lastReadAt time.Time) error {
	pk := fmt.Sprintf("CONVERSATION_STATUS#%s", conversationID)
	sk := fmt.Sprintf("USER#%s", participant)

	err := b.conversationRepo.GetDB().WithContext(ctx).
		Model(&models.ConversationStatus{}).
		Where("PK", "=", pk).
		Where("SK", "=", sk).
		UpdateBuilder().
		Set("ConversationID", conversationID).
		Set("UserID", participant).
		Set("Unread", unread).
		Set("LastReadAt", lastReadAt).
		Execute()
	if err != nil {
		return fmt.Errorf("upsert conversation status %s/%s: %w", conversationID, participant, err)
	}

	return nil
}

func participantsForStatus(status *models.Status) ([]string, bool) {
	if status == nil {
		return nil, false
	}

	sender := strings.TrimSpace(status.AuthorUsername)
	if sender == "" {
		sender = extractUsernameFromActorID(status.AuthorID)
	}
	if sender == "" {
		return nil, false
	}

	participants := []string{sender}
	participants = append(participants, recipientParticipants(status, sender)...)
	participants = canonicalizeParticipants(participants)
	if len(participants) < 2 {
		return nil, false
	}
	return participants, true
}

func recipientParticipants(status *models.Status, sender string) []string {
	seen := map[string]struct{}{
		sender: {},
	}

	recipients := make([]string, 0)
	appendRecipient := func(candidate string) {
		normalized := participantIDForRecipient(status, candidate)
		if normalized == "" {
			return
		}
		if _, exists := seen[normalized]; exists {
			return
		}
		seen[normalized] = struct{}{}
		recipients = append(recipients, normalized)
	}

	for _, recipient := range authoritativeRecipients(status) {
		appendRecipient(recipient)
	}

	return recipients
}

func authoritativeRecipients(status *models.Status) []string {
	if status == nil {
		return nil
	}

	recipients := make([]string, 0, len(status.ToRecipients)+len(status.CcRecipients)+len(status.BtoRecipients)+len(status.BccRecipients))
	recipients = append(recipients, status.ToRecipients...)
	recipients = append(recipients, status.CcRecipients...)
	recipients = append(recipients, status.BtoRecipients...)
	recipients = append(recipients, status.BccRecipients...)
	if len(recipients) > 0 || status.Note == nil {
		return recipients
	}

	recipients = append(recipients, status.Note.To...)
	recipients = append(recipients, status.Note.CC...)
	recipients = append(recipients, status.Note.BTo...)
	recipients = append(recipients, status.Note.BCC...)
	return recipients
}

func participantIDForRecipient(status *models.Status, recipient string) string {
	recipient = strings.TrimSpace(recipient)
	if !isSpecificActorRecipient(recipient) {
		return ""
	}

	recipientHost := actorHost(recipient)
	authorID := ""
	if status != nil {
		authorID = status.AuthorID
	}
	authorHost := actorHost(authorID)
	if recipientHost != "" {
		if authorHost == "" || !strings.EqualFold(recipientHost, authorHost) {
			return recipient
		}
	}

	return extractUsernameFromActorID(recipient)
}

func isSpecificActorRecipient(recipient string) bool {
	if recipient == "" || strings.EqualFold(recipient, activitypub.PublicAddress) {
		return false
	}

	lowerRecipient := strings.ToLower(recipient)
	if strings.Contains(lowerRecipient, "/followers") || strings.Contains(lowerRecipient, "/following") {
		return false
	}

	return true
}

func canonicalizeParticipants(participants []string) []string {
	seen := make(map[string]struct{}, len(participants))
	canonical := make([]string, 0, len(participants))
	for _, participant := range participants {
		participant = strings.TrimSpace(participant)
		if participant == "" {
			continue
		}
		if _, exists := seen[participant]; exists {
			continue
		}
		seen[participant] = struct{}{}
		canonical = append(canonical, participant)
	}
	sort.Strings(canonical)
	return canonical
}

func statusNeedsConversationNormalization(status *models.Status, conversationID string) bool {
	if status == nil {
		return false
	}
	if strings.TrimSpace(status.ConversationID) != conversationID {
		return true
	}
	if strings.TrimSpace(status.GSI3PK) != "CONVERSATION#"+conversationID {
		return true
	}
	if status.Note != nil && strings.TrimSpace(status.Note.ConversationID) != conversationID {
		return true
	}
	return false
}

func statusPublishedAt(status *models.Status) time.Time {
	if status == nil {
		return time.Time{}
	}
	switch {
	case !status.PublishedAt.IsZero():
		return status.PublishedAt.UTC()
	case !status.CreatedAt.IsZero():
		return status.CreatedAt.UTC()
	case !status.UpdatedAt.IsZero():
		return status.UpdatedAt.UTC()
	default:
		return time.Unix(0, 0).UTC()
	}
}

func statusPrimaryKey(statusID string) string {
	return fmt.Sprintf("status#%s", statusID)
}

func extractUsernameFromActorID(actorID string) string {
	value := strings.TrimSpace(actorID)
	if value == "" {
		return ""
	}

	value = strings.TrimSuffix(value, "/")
	parts := strings.Split(value, "/")
	if len(parts) == 0 {
		return ""
	}

	last := strings.TrimSpace(parts[len(parts)-1])
	if last == "" {
		return ""
	}
	return strings.TrimPrefix(last, "@")
}

func actorHost(actorID string) string {
	value := strings.TrimSpace(actorID)
	if value == "" {
		return ""
	}
	if strings.Contains(value, "://") {
		parsed, err := url.Parse(value)
		if err != nil {
			return ""
		}
		return strings.ToLower(strings.TrimSpace(parsed.Hostname()))
	}

	handle := strings.TrimPrefix(value, "@")
	if parts := strings.SplitN(handle, "@", 2); len(parts) == 2 {
		return strings.ToLower(strings.TrimSpace(parts[1]))
	}

	return ""
}

func timePtr(ts time.Time) *time.Time {
	if ts.IsZero() {
		return nil
	}
	tsCopy := ts.UTC()
	return &tsCopy
}

func isNotFound(err error) bool {
	if err == nil {
		return false
	}
	return ttErrors.IsNotFound(err)
}

func envOrDefault(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

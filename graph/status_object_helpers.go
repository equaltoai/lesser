package graph

import (
	"context"
	neturl "net/url"
	"strings"
	"time"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/services/notes"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"go.uber.org/zap"
)

func mapStatusVisibility(status *models.Status) model.Visibility {
	if status == nil {
		return model.VisibilityPublic
	}

	switch status.Visibility {
	case VisibilityUnlisted:
		return model.VisibilityUnlisted
	case VisibilityPrivate, EventTypeFollowers:
		return model.VisibilityFollowers
	case TimelineTypeDirect:
		return model.VisibilityDirect
	default:
		return model.VisibilityPublic
	}
}

func cloneNoteAttachments(status *models.Status) []*activitypub.Attachment {
	if status == nil || status.Note == nil {
		return []*activitypub.Attachment{}
	}

	attachments := make([]*activitypub.Attachment, 0, len(status.Note.Attachment))
	for _, att := range status.Note.Attachment {
		attachment := att
		attachments = append(attachments, &attachment)
	}

	return attachments
}

func cloneNoteTags(status *models.Status) []*activitypub.Tag {
	if status == nil || status.Note == nil {
		return []*activitypub.Tag{}
	}

	tags := make([]*activitypub.Tag, 0, len(status.Note.Tag))
	for _, tag := range status.Note.Tag {
		tagCopy := tag
		tags = append(tags, &tagCopy)
	}

	return tags
}

func (r *Resolver) buildMentions(status *models.Status) []*model.Mention {
	if status == nil || status.Mentions == nil {
		return []*model.Mention{}
	}

	mentions := make([]*model.Mention, 0, len(status.Mentions))
	for _, mentionURL := range status.Mentions {
		username, domain := r.parseMentionURL(mentionURL)
		if username == "" {
			continue
		}
		mentions = append(mentions, &model.Mention{
			ID:       mentionURL,
			Username: username,
			URL:      mentionURL,
			Domain:   domain,
		})
	}

	return mentions
}

func (r *Resolver) determineQuoteable(ctx context.Context, status *models.Status) (bool, model.QuotePermission) {
	if r == nil || status == nil || strings.TrimSpace(status.StatusID) == "" {
		return false, model.QuotePermissionNone
	}
	authorUsername := resolveStatusAuthorUsername(status)
	if authorUsername == "" {
		return false, model.QuotePermissionNone
	}

	store := r.Storage
	if r.Registry != nil && r.Registry.GetStorage() != nil {
		store = r.Registry.GetStorage()
	}
	if store == nil || store.Object() == nil {
		return false, model.QuotePermissionNone
	}

	quoteType := ""
	var accountPermissions *models.QuotePermissions
	var err error
	if GetLoaders(ctx) != nil {
		loaded, loadErr := loadQuoteControl(ctx, status.StatusID)
		err = loadErr
		if loaded != nil {
			quoteType = loaded.quoteType
			r.trackQuoteControlBatch(ctx, loaded.batch)
		}
		account, accountErr := loadQuoteAccount(ctx, authorUsername)
		if err == nil {
			err = accountErr
		}
		if account != nil {
			accountPermissions = account.permissions
			r.trackQuoteControlBatch(ctx, account.batch)
		}
	} else {
		// Non-GraphQL callers and focused unit tests have no request loader. The
		// production GraphQL middleware always installs a fresh loader set.
		quoteType, err = store.Object().GetQuoteType(ctx, status.StatusID)
		r.trackDynamoOperation(ctx, "read", 1)
		if err == nil && r.Registry != nil && r.Registry.Quotes() != nil {
			accountPermissions, err = r.Registry.Quotes().GetQuotePermissions(ctx, authorUsername)
			r.trackDynamoOperation(ctx, "read", 1)
		}
	}
	if err != nil || accountPermissions == nil {
		if r.Logger != nil {
			r.Logger.Warn("effective quote control projection failed closed",
				zap.String("status_id", status.StatusID),
				zap.Error(err))
		}
		return false, model.QuotePermissionNone
	}
	return effectiveGraphQLQuoteControl(accountPermissions, quoteType)
}

func effectiveGraphQLQuoteControl(account *models.QuotePermissions, storedQuoteType string) (bool, model.QuotePermission) {
	if account == nil {
		return false, model.QuotePermissionNone
	}
	perNote := strings.ToLower(strings.TrimSpace(storedQuoteType))
	publicAllowed := account.AllowPublic && perNote == models.VisibilityPublic
	followersAllowed := (account.AllowPublic || account.AllowFollowers) &&
		(perNote == models.VisibilityPublic || perNote == EventTypeFollowers)
	mentionedAllowed := (account.AllowPublic || account.AllowMentioned) &&
		(perNote == models.VisibilityPublic || perNote == "mentioned")
	switch {
	case publicAllowed:
		return true, model.QuotePermissionEveryone
	case followersAllowed:
		return true, model.QuotePermissionFollowers
	case mentionedAllowed:
		return true, model.QuotePermissionMentioned
	default:
		return false, model.QuotePermissionNone
	}
}

func (r *Resolver) trackQuoteControlBatch(ctx context.Context, batch *quoteControlBatch) {
	if r == nil || batch == nil || batch.readUnits <= 0 || !batch.tracked.CompareAndSwap(false, true) {
		return
	}
	r.trackDynamoOperation(ctx, "read", batch.readUnits)
}

func quoteControlPrefetchIDs(statuses []*models.Status) []string {
	seen := make(map[string]struct{}, len(statuses)*3)
	ids := make([]string, 0, len(statuses)*3)
	add := func(raw string) {
		id := strings.TrimSpace(raw)
		if id == "" {
			return
		}
		if _, ok := seen[id]; ok {
			return
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	for _, status := range statuses {
		if status == nil {
			continue
		}
		add(status.StatusID)
		add(status.InReplyToID)
		add(boostTargetStatusID(status))
	}
	return ids
}

func quoteAccountPrefetchUsernames(statuses []*models.Status) []string {
	seen := make(map[string]struct{}, len(statuses))
	usernames := make([]string, 0, len(statuses))
	for _, status := range statuses {
		username := strings.TrimSpace(resolveStatusAuthorUsername(status))
		if username == "" {
			continue
		}
		if _, ok := seen[username]; ok {
			continue
		}
		seen[username] = struct{}{}
		usernames = append(usernames, username)
	}
	return usernames
}

func (r *Resolver) prefetchQuoteControls(ctx context.Context, statuses []*models.Status) {
	if GetLoaders(ctx) == nil {
		return
	}
	ids := quoteControlPrefetchIDs(statuses)
	if len(ids) == 0 {
		return
	}
	loaded, _ := loadQuoteControls(ctx, ids)
	for _, result := range loaded {
		if result != nil {
			r.trackQuoteControlBatch(ctx, result.batch)
			break
		}
	}

	usernames := quoteAccountPrefetchUsernames(statuses)
	if len(usernames) == 0 {
		return
	}
	accounts, _ := loadQuoteAccounts(ctx, usernames)
	for _, result := range accounts {
		if result != nil {
			r.trackQuoteControlBatch(ctx, result.batch)
			return
		}
	}
}

func extractStatusSummary(status *models.Status) *string {
	if status == nil || status.Note == nil {
		return nil
	}
	if status.Note.Summary == "" {
		return nil
	}
	summary := status.Note.Summary
	return &summary
}

func (r *Resolver) viewerBoostState(ctx context.Context, status *models.Status, convertLogger *zap.Logger) bool {
	if status == nil || status.StatusID == "" {
		return false
	}

	viewerUsername := getUsernameFromContext(ctx)
	if viewerUsername == "" {
		return false
	}

	boosted, err := viewerBoostStateResolverFunc(ctx, r, viewerUsername, status.StatusID)
	if err != nil && convertLogger != nil {
		convertLogger.Warn("failed to resolve viewer boost state",
			zap.String("status_id", status.StatusID),
			zap.String("viewer", viewerUsername),
			zap.Error(err))
	}

	return err == nil && boosted
}

func (r *Resolver) resolveActorForStatus(ctx context.Context, status *models.Status, convertLogger *zap.Logger) *activitypub.Actor {
	if status == nil {
		return nil
	}

	username := resolveStatusAuthorUsername(status)
	lookupCandidates := statusActorLookupCandidates(status)
	actorURLLookupConstraint := statusActorURLLookupConstraint(status)
	authorIsRemote := statusAuthorIsRemote(status, r.localActorDomain())
	if len(lookupCandidates) == 0 {
		if convertLogger != nil {
			convertLogger.Warn("status missing author username",
				zap.String("status_id", status.StatusID),
				zap.String("author_id", status.AuthorID))
		}
		return nil
	}

	if !authorIsRemote {
		if prefetched := prefetchedConversationAccount(ctx, username); prefetched != nil {
			return r.convertAccountToActor(prefetched)
		}
	}

	for _, candidate := range lookupCandidates {
		lookupStart := time.Now()
		resolution, err := r.resolveStoredActorLookup(ctx, candidate)
		if convertLogger != nil {
			convertLogger.Info("convertStatusToObject stored actor lookup",
				zap.String("status_id", status.StatusID),
				zap.String("actor_lookup", candidate),
				zap.Duration("duration", time.Since(lookupStart)),
				zap.Bool("found", err == nil && resolution != nil && resolution.Actor != nil),
				zap.Error(err))
		}
		if err != nil || resolution == nil {
			continue
		}
		if resolution.Actor != nil && !common.CachedRemoteActorIDMatchesLookup(resolution.Actor.ID, actorURLLookupConstraint) {
			continue
		}

		if actor := r.materializeActorResolution(ctx, resolution); actor != nil {
			return actor
		}
	}

	if authorIsRemote {
		placeholderID := statusActorPlaceholderIdentifier(status)
		if placeholderID == "" {
			return nil
		}

		placeholder := r.buildPlaceholderActor(placeholderID, "")
		if convertLogger != nil {
			convertLogger.Info("convertStatusToObject remote placeholder actor",
				zap.String("status_id", status.StatusID),
				zap.String("actor_lookup", placeholderID),
				zap.Bool("found", placeholder != nil))
		}
		return placeholder
	}

	if r.Registry == nil || r.Registry.Accounts() == nil || username == "" {
		return nil
	}

	accountStart := time.Now()
	result, err := r.Registry.Accounts().GetAccount(ctx, username)
	if convertLogger != nil {
		convertLogger.Info("convertStatusToObject account lookup fallback",
			zap.String("status_id", status.StatusID),
			zap.String("author_username", username),
			zap.Duration("duration", time.Since(accountStart)),
			zap.Bool("found", err == nil && result != nil),
			zap.Error(err))
	}
	if err != nil || result == nil {
		return nil
	}
	return r.convertAccountToActor(result)
}

func (r *Resolver) resolveInReplyToObject(ctx context.Context, status *models.Status, convertLogger *zap.Logger) *model.Object {
	if status == nil || status.InReplyToID == "" {
		return nil
	}

	depth := r.getConversionDepth(ctx)
	if depth >= 3 {
		return nil
	}

	newCtx := r.setConversionDepth(ctx, depth+1)
	viewerUsername := r.optionalAuth(newCtx)
	parentLookupStart := time.Now()
	parentStatus, err := r.Registry.Notes().GetNoteWithViewer(newCtx, &notes.GetNoteQuery{
		StatusID: status.InReplyToID,
		ViewerID: viewerUsername,
	})
	if convertLogger != nil {
		convertLogger.Info("convertStatusToObject parent lookup",
			zap.String("status_id", status.StatusID),
			zap.String("in_reply_to", status.InReplyToID),
			zap.Duration("duration", time.Since(parentLookupStart)),
			zap.Error(err))
	}
	if err != nil || parentStatus == nil {
		return nil
	}

	return r.convertStatusToObject(newCtx, parentStatus)
}

func boostTargetStatusID(status *models.Status) string {
	if status == nil {
		return ""
	}
	if trimmed := strings.TrimSpace(status.ReblogOfID); trimmed != "" {
		return trimmed
	}
	return strings.TrimSpace(status.BoostOfStatusID)
}

func (r *Resolver) resolveBoostedObject(ctx context.Context, status *models.Status, convertLogger *zap.Logger) *model.Object {
	targetStatusID := boostTargetStatusID(status)
	if targetStatusID == "" {
		return nil
	}

	depth := r.getConversionDepth(ctx)
	if depth >= 3 {
		return nil
	}

	newCtx := r.setConversionDepth(ctx, depth+1)
	viewerUsername := r.optionalAuth(newCtx)
	notesSvc := r.notesService()
	if notesSvc == nil {
		if convertLogger != nil {
			convertLogger.Warn("notes service unavailable while resolving boost target",
				zap.String("status_id", status.StatusID),
				zap.String("target_status_id", targetStatusID))
		}
		return nil
	}

	originalStatus, err := notesSvc.GetNoteWithViewer(newCtx, &notes.GetNoteQuery{
		StatusID: targetStatusID,
		ViewerID: viewerUsername,
	})
	if err != nil {
		if convertLogger != nil {
			convertLogger.Warn("failed to resolve boost target",
				zap.String("status_id", status.StatusID),
				zap.String("target_status_id", targetStatusID),
				zap.Error(err))
		}
		return nil
	}

	return r.convertStatusToObject(newCtx, originalStatus)
}

func resolveStatusAuthorUsername(status *models.Status) string {
	if status == nil {
		return ""
	}

	if trimmed := strings.TrimSpace(status.AuthorUsername); trimmed != "" {
		return trimmed
	}

	return extractUsernameFromActorIdentifier(status.AuthorID)
}

func statusActorLookupCandidates(status *models.Status) []string {
	if status == nil {
		return nil
	}

	candidates := make([]string, 0, 3)
	appendStatusActorLookupCandidate(&candidates, status.AuthorID)
	if status.Note != nil {
		appendStatusActorLookupCandidate(&candidates, status.Note.AttributedTo)
	}
	appendStatusActorLookupCandidate(&candidates, status.AuthorUsername)

	return candidates
}

func statusActorURLLookupConstraint(status *models.Status) string {
	if status == nil {
		return ""
	}
	for _, candidate := range []string{
		strings.TrimSpace(status.AuthorID),
		func() string {
			if status.Note == nil {
				return ""
			}
			return strings.TrimSpace(status.Note.AttributedTo)
		}(),
	} {
		lowerCandidate := strings.ToLower(strings.TrimSpace(candidate))
		if strings.HasPrefix(lowerCandidate, "http://") || strings.HasPrefix(lowerCandidate, "https://") {
			return candidate
		}
	}
	return ""
}

func appendStatusActorLookupCandidate(dst *[]string, candidate string) {
	trimmed := strings.TrimSpace(candidate)
	if trimmed == "" {
		return
	}

	for _, existing := range *dst {
		if strings.EqualFold(existing, trimmed) {
			return
		}
	}

	*dst = append(*dst, trimmed)
}

func statusActorPlaceholderIdentifier(status *models.Status) string {
	for _, candidate := range statusActorLookupCandidates(status) {
		if strings.TrimSpace(candidate) != "" {
			return strings.TrimSpace(candidate)
		}
	}

	return resolveStatusAuthorUsername(status)
}

func statusAuthorIsRemote(status *models.Status, localDomain string) bool {
	for _, candidate := range statusActorLookupCandidates(status) {
		if actorIdentifierLooksRemote(candidate, localDomain) {
			return true
		}
	}

	return false
}

func actorIdentifierLooksRemote(identifier, localDomain string) bool {
	identifier = strings.TrimSpace(identifier)
	if identifier == "" {
		return false
	}

	localDomain = strings.TrimSpace(strings.ToLower(localDomain))
	lowerIdentifier := strings.ToLower(identifier)
	if strings.HasPrefix(lowerIdentifier, "http://") || strings.HasPrefix(lowerIdentifier, "https://") {
		parsed, err := neturl.Parse(identifier)
		if err != nil {
			return false
		}
		host := strings.TrimSpace(strings.ToLower(parsed.Hostname()))
		if host == "" {
			return false
		}
		if localDomain == "" {
			return true
		}
		return host != localDomain
	}

	handle := strings.TrimPrefix(identifier, "@")
	if strings.Count(handle, "@") != 1 {
		return false
	}

	parts := strings.Split(handle, "@")
	if len(parts) != 2 {
		return false
	}

	domain := strings.TrimSpace(strings.ToLower(parts[1]))
	if domain == "" {
		return false
	}
	if localDomain == "" {
		return true
	}
	return domain != localDomain
}

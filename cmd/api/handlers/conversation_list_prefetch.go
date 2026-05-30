package handlers

import (
	"context"
	"strings"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage"
	storageModels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/transformations"
)

type prefetchedConversationAccountsContextKey struct{}

type conversationAPIPrefetch struct {
	accountsByKey map[string]*storage.Account
	statusesByID  map[string]*storageModels.Status
}

func withPrefetchedConversationAccounts(ctx context.Context, accountsByKey map[string]*storage.Account) context.Context {
	if len(accountsByKey) == 0 {
		return ctx
	}
	return context.WithValue(ctx, prefetchedConversationAccountsContextKey{}, accountsByKey)
}

func prefetchedConversationAccount(ctx context.Context, username string) *storage.Account {
	if ctx == nil || strings.TrimSpace(username) == "" {
		return nil
	}
	accountsByKey, _ := ctx.Value(prefetchedConversationAccountsContextKey{}).(map[string]*storage.Account)
	if len(accountsByKey) == 0 {
		return nil
	}
	return accountsByKey[strings.ToLower(strings.TrimSpace(username))]
}

func conversationPreviewStatusID(conv *storageModels.Conversation) string {
	if conv == nil {
		return ""
	}
	if conv.ViewerState != nil && strings.TrimSpace(conv.ViewerState.PreviewStatusID) != "" {
		return strings.TrimSpace(conv.ViewerState.PreviewStatusID)
	}
	return strings.TrimSpace(conv.LastStatusID)
}

func conversationParticipantUsernames(conv *storageModels.Conversation, viewerUsername string) []string {
	if conv == nil {
		return nil
	}

	viewerUsername = strings.ToLower(strings.TrimSpace(viewerUsername))
	refs := conversationParticipantRefsForProjection(conv)
	seen := make(map[string]struct{}, len(refs))
	usernames := make([]string, 0, len(refs))
	for _, ref := range refs {
		candidate := strings.TrimSpace(ref.ParticipantID)
		if ref.ParticipantType != storageModels.ConversationParticipantTypeRemoteActor {
			candidate = strings.ToLower(candidate)
		}
		if candidate == "" || candidate == viewerUsername {
			continue
		}
		key := strings.ToLower(candidate)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		usernames = append(usernames, candidate)
	}

	if len(usernames) == 0 && conv.ViewerState != nil {
		if counterpart := strings.ToLower(strings.TrimSpace(conv.ViewerState.CounterpartID)); counterpart != "" && counterpart != viewerUsername {
			usernames = append(usernames, counterpart)
		}
	}

	return usernames
}

func conversationParticipantRefsForProjection(conv *storageModels.Conversation) []storageModels.ConversationParticipantRef {
	if conv == nil {
		return nil
	}
	if len(conv.ParticipantRefs) > 0 {
		return storageModels.NormalizeConversationParticipantRefs(conv.ParticipantRefs)
	}
	refs := make([]storageModels.ConversationParticipantRef, 0, len(conv.Participants))
	for _, participant := range conv.Participants {
		participant = strings.TrimSpace(participant)
		if participant == "" {
			continue
		}
		participantType := storageModels.ConversationParticipantTypeLocalUser
		if strings.Contains(participant, "://") {
			participantType = storageModels.ConversationParticipantTypeRemoteActor
		}
		refs = append(refs, storageModels.ConversationParticipantRef{
			ParticipantType: participantType,
			ParticipantID:   participant,
		})
	}
	if conv.ViewerState != nil && conv.ViewerState.CounterpartID != "" {
		refs = append(refs, storageModels.ConversationParticipantRef{
			ParticipantType: conv.ViewerState.CounterpartType,
			ParticipantID:   conv.ViewerState.CounterpartID,
			Acct:            conv.ViewerState.CounterpartAcct,
			Domain:          conv.ViewerState.CounterpartDomain,
			ResolvedAt:      conv.ViewerState.CounterpartResolvedAt,
		})
	}
	return storageModels.NormalizeConversationParticipantRefs(refs)
}

func conversationAPIAccountPrefetchKeys(account *storage.Account) []string {
	if account == nil {
		return nil
	}

	keys := make([]string, 0, 5)
	add := func(raw string) {
		candidate := strings.ToLower(strings.TrimSpace(raw))
		if candidate == "" {
			return
		}
		for _, existing := range keys {
			if existing == candidate {
				return
			}
		}
		keys = append(keys, candidate)
	}

	if account.User != nil {
		add(account.User.Username)
		add(account.User.ID)
	}
	if account.Actor != nil {
		add(account.Actor.ID)
		add(account.Actor.URL)
		add(account.Actor.PreferredUsername)
	}

	return keys
}

func buildConversationAPIAccountMap(accounts []*storage.Account) map[string]*storage.Account {
	accountsByKey := make(map[string]*storage.Account, len(accounts)*2)
	for _, account := range accounts {
		for _, key := range conversationAPIAccountPrefetchKeys(account) {
			accountsByKey[key] = account
		}
	}
	return accountsByKey
}

func buildConversationAPIStatusMap(statuses []*storageModels.Status) map[string]*storageModels.Status {
	statusesByID := make(map[string]*storageModels.Status, len(statuses))
	for _, status := range statuses {
		if status == nil || strings.TrimSpace(status.StatusID) == "" {
			continue
		}
		statusesByID[status.StatusID] = status
	}
	return statusesByID
}

func (h *Handler) loadConversationAPIPrefetch(ctx context.Context, conversations []*storageModels.Conversation, viewerUsername string) *conversationAPIPrefetch {
	prefetch := &conversationAPIPrefetch{
		accountsByKey: map[string]*storage.Account{},
		statusesByID:  map[string]*storageModels.Status{},
	}

	if h == nil || len(conversations) == 0 || h.repos == nil {
		return prefetch
	}

	participantSet := make(map[string]struct{})
	statusIDs := make([]string, 0, len(conversations))
	statusSet := make(map[string]struct{})

	for _, conv := range conversations {
		for _, participant := range conversationParticipantUsernames(conv, viewerUsername) {
			participantSet[participant] = struct{}{}
		}
		if previewStatusID := conversationPreviewStatusID(conv); previewStatusID != "" {
			if _, ok := statusSet[previewStatusID]; !ok {
				statusSet[previewStatusID] = struct{}{}
				statusIDs = append(statusIDs, previewStatusID)
			}
		}
	}

	if len(participantSet) > 0 && h.repos.Account() != nil {
		participants := make([]string, 0, len(participantSet))
		for participant := range participantSet {
			participants = append(participants, participant)
		}
		if accounts, err := h.repos.Account().GetAccountsByUsernames(ctx, participants); err == nil {
			prefetch.accountsByKey = buildConversationAPIAccountMap(accounts)
		}
	}

	h.addRemoteConversationAPIAccountsToPrefetch(ctx, conversations, prefetch)

	if len(statusIDs) > 0 && h.repos.Status() != nil {
		if statuses, err := h.repos.Status().GetStatusesByIDs(ctx, statusIDs); err == nil {
			prefetch.statusesByID = buildConversationAPIStatusMap(statuses)
		}
	}

	return prefetch
}

func (h *Handler) addRemoteConversationAPIAccountsToPrefetch(ctx context.Context, conversations []*storageModels.Conversation, prefetch *conversationAPIPrefetch) {
	if h == nil || h.repos == nil || h.repos.Actor() == nil || prefetch == nil {
		return
	}
	for _, conv := range conversations {
		for _, ref := range conversationParticipantRefsForProjection(conv) {
			if ref.ParticipantType != storageModels.ConversationParticipantTypeRemoteActor {
				continue
			}
			account := h.conversationAPIRemoteAccountForParticipantRef(ctx, ref)
			if account == nil {
				continue
			}
			for _, key := range conversationAPIAccountPrefetchKeys(account) {
				prefetch.accountsByKey[key] = account
			}
		}
	}
}

func (h *Handler) conversationAPIAccountForParticipant(ctx context.Context, participant string, prefetch *conversationAPIPrefetch) *storage.Account {
	participant = strings.ToLower(strings.TrimSpace(participant))
	if participant == "" || h == nil || h.repos == nil {
		return nil
	}

	if prefetch != nil {
		if account := prefetch.accountsByKey[participant]; account != nil {
			return account
		}
		if derivedUsername := strings.ToLower(strings.TrimSpace(extractUsernameFromActorID(participant))); derivedUsername != "" && derivedUsername != participant {
			if account := prefetch.accountsByKey[derivedUsername]; account != nil {
				return account
			}
		}
		if account := prefetch.accountsByKey[strings.ToLower(strings.TrimSpace(participant))]; account != nil {
			return account
		}
	}

	actor, err := h.repos.Actor().GetActor(ctx, participant)
	if err != nil || actor == nil {
		return nil
	}

	return &storage.Account{
		User:  &storage.User{Username: participant, DisplayName: participant},
		Actor: actor,
	}
}

func (h *Handler) conversationAPIAccountForParticipantRef(ctx context.Context, ref storageModels.ConversationParticipantRef, prefetch *conversationAPIPrefetch) *storage.Account {
	ref = storageModels.NormalizeConversationParticipantRef(ref)
	if ref.ParticipantID == "" {
		return nil
	}
	if prefetch != nil {
		if account := prefetch.accountsByKey[strings.ToLower(ref.ParticipantID)]; account != nil {
			return account
		}
		if ref.Acct != "" {
			if account := prefetch.accountsByKey[strings.ToLower(ref.Acct)]; account != nil {
				return account
			}
		}
	}
	if ref.ParticipantType == storageModels.ConversationParticipantTypeRemoteActor {
		return h.conversationAPIRemoteAccountForParticipantRef(ctx, ref)
	}
	return h.conversationAPIAccountForParticipant(ctx, ref.ParticipantID, prefetch)
}

func (h *Handler) conversationAPIRemoteAccountForParticipantRef(ctx context.Context, ref storageModels.ConversationParticipantRef) *storage.Account {
	ref = storageModels.NormalizeConversationParticipantRef(ref)
	if ref.ParticipantID == "" || h == nil {
		return nil
	}
	localDomain := ""
	if h.cfg != nil {
		localDomain = h.cfg.Domain
	}

	if h.repos != nil && h.repos.Actor() != nil {
		candidates := []string{ref.ParticipantID, ref.Acct}
		for _, candidate := range candidates {
			candidate = strings.TrimSpace(candidate)
			if candidate == "" {
				continue
			}
			actor, err := h.repos.Actor().GetCachedRemoteActor(ctx, candidate)
			if err == nil && actor != nil {
				if !common.CachedRemoteActorIDMatchesLookup(actor.ID, ref.ParticipantID) {
					continue
				}
				return storageAccountFromActor(actor, localDomain)
			}
		}
	}

	return storageAccountFromActor(conversationAPISyntheticRemoteActor(ref), localDomain)
}

func conversationAPISyntheticRemoteActor(ref storageModels.ConversationParticipantRef) *activitypub.Actor {
	ref = storageModels.NormalizeConversationParticipantRef(ref)
	username := strings.TrimSpace(ref.Acct)
	if username != "" {
		if handleUser, _, ok := strings.Cut(username, "@"); ok {
			username = handleUser
		}
	}
	if username == "" {
		username = strings.TrimSpace(extractUsernameFromActorID(ref.ParticipantID))
	}
	if username == "" {
		username = strings.TrimSpace(ref.ParticipantID)
	}
	return &activitypub.Actor{
		BaseObject: activitypub.BaseObject{
			ID:   ref.ParticipantID,
			Type: activitypub.PersonType,
		},
		PreferredUsername: username,
		Name:              username,
		URL:               ref.ParticipantID,
	}
}

func (h *Handler) conversationAPIAccounts(ctx context.Context, conv *storageModels.Conversation, viewerUsername string, prefetch *conversationAPIPrefetch) []apimodels.Account {
	accounts := make([]apimodels.Account, 0, len(conv.Participants))
	for _, ref := range conversationParticipantRefsForProjection(conv) {
		participant := ref.ParticipantID
		if ref.ParticipantType != storageModels.ConversationParticipantTypeRemoteActor {
			participant = strings.ToLower(strings.TrimSpace(participant))
		}
		if participant == "" || participant == strings.ToLower(strings.TrimSpace(viewerUsername)) {
			continue
		}
		account := h.conversationAPIAccountForParticipantRef(ctx, ref, prefetch)
		if account == nil || account.Actor == nil {
			continue
		}
		accounts = append(accounts, transformations.ActorToAccountBase(account.Actor, h.cfg.BaseURL()))
	}
	return accounts
}

func (h *Handler) conversationAPIStatus(ctx context.Context, conv *storageModels.Conversation, prefetch *conversationAPIPrefetch) *storageModels.Status {
	previewStatusID := conversationPreviewStatusID(conv)
	if previewStatusID == "" || h == nil || h.repos == nil || h.repos.Status() == nil {
		return nil
	}

	if prefetch != nil {
		if status := prefetch.statusesByID[previewStatusID]; status != nil {
			return status
		}
	}

	status, err := h.repos.Status().GetStatus(ctx, previewStatusID)
	if err != nil {
		return nil
	}
	return status
}

func conversationAPIStatusContext(h *Handler, prefetch *conversationAPIPrefetch) context.Context {
	if h == nil {
		return context.Background()
	}

	statusCtx := h.statusConversionContext()
	if prefetch != nil {
		statusCtx = withPrefetchedConversationAccounts(statusCtx, prefetch.accountsByKey)
	}
	return statusCtx
}

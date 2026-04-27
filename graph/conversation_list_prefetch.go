package graph

import (
	"context"
	"strings"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
)

type prefetchedConversationAccountsContextKey struct{}

type conversationListPrefetch struct {
	accountsByKey map[string]*storage.Account
	statusesByID  map[string]*storagemodels.Status
	statusesReady bool
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

func conversationListViewerMetadata(state *storagemodels.UserConversationState) *model.ConversationViewerMetadata {
	viewerMetadata := &model.ConversationViewerMetadata{
		RequestState: model.DmRequestStateAccepted,
	}
	if state == nil {
		return viewerMetadata
	}

	if state.RequestState != "" {
		viewerMetadata.RequestState = model.DmRequestState(state.RequestState)
	}
	viewerMetadata.RequestedAt = modelTimePtr(state.RequestedAt)
	viewerMetadata.AcceptedAt = modelTimePtr(state.AcceptedAt)
	viewerMetadata.DeclinedAt = modelTimePtr(state.DeclinedAt)
	return viewerMetadata
}

func conversationListPreviewStatusID(conv *storagemodels.Conversation) string {
	if conv == nil {
		return ""
	}
	if conv.ViewerState != nil && strings.TrimSpace(conv.ViewerState.PreviewStatusID) != "" {
		return strings.TrimSpace(conv.ViewerState.PreviewStatusID)
	}
	return strings.TrimSpace(conv.LastStatusID)
}

func conversationListParticipantUsernames(conv *storagemodels.Conversation, viewerUsername string) []string {
	if conv == nil {
		return nil
	}

	viewerUsername = strings.ToLower(strings.TrimSpace(viewerUsername))
	refs := conversationListParticipantRefsForProjection(conv)
	seen := make(map[string]struct{}, len(refs))
	usernames := make([]string, 0, len(refs))
	for _, ref := range refs {
		candidate := strings.TrimSpace(ref.ParticipantID)
		if ref.ParticipantType != storagemodels.ConversationParticipantTypeRemoteActor {
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

func conversationListParticipantRefsForProjection(conv *storagemodels.Conversation) []storagemodels.ConversationParticipantRef {
	if conv == nil {
		return nil
	}
	if len(conv.ParticipantRefs) > 0 {
		return storagemodels.NormalizeConversationParticipantRefs(conv.ParticipantRefs)
	}
	refs := make([]storagemodels.ConversationParticipantRef, 0, len(conv.Participants))
	for _, participant := range conv.Participants {
		participant = strings.TrimSpace(participant)
		if participant == "" {
			continue
		}
		participantType := storagemodels.ConversationParticipantTypeLocalUser
		if strings.Contains(participant, "://") {
			participantType = storagemodels.ConversationParticipantTypeRemoteActor
		}
		refs = append(refs, storagemodels.ConversationParticipantRef{
			ParticipantType: participantType,
			ParticipantID:   participant,
		})
	}
	if conv.ViewerState != nil && conv.ViewerState.CounterpartID != "" {
		refs = append(refs, storagemodels.ConversationParticipantRef{
			ParticipantType: conv.ViewerState.CounterpartType,
			ParticipantID:   conv.ViewerState.CounterpartID,
			Acct:            conv.ViewerState.CounterpartAcct,
			Domain:          conv.ViewerState.CounterpartDomain,
			ResolvedAt:      conv.ViewerState.CounterpartResolvedAt,
		})
	}
	return storagemodels.NormalizeConversationParticipantRefs(refs)
}

func conversationAccountPrefetchKeys(account *storage.Account) []string {
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

func buildConversationAccountMap(accounts []*storage.Account) map[string]*storage.Account {
	accountsByKey := make(map[string]*storage.Account, len(accounts)*2)
	for _, account := range accounts {
		for _, key := range conversationAccountPrefetchKeys(account) {
			accountsByKey[key] = account
		}
	}
	return accountsByKey
}

func buildConversationStatusMap(statuses []*storagemodels.Status) map[string]*storagemodels.Status {
	statusesByID := make(map[string]*storagemodels.Status, len(statuses))
	for _, status := range statuses {
		if status == nil || strings.TrimSpace(status.StatusID) == "" {
			continue
		}
		statusesByID[status.StatusID] = status
	}
	return statusesByID
}

func (r *Resolver) loadConversationListPrefetch(ctx context.Context, viewerUsername string, conversations []*storagemodels.Conversation) *conversationListPrefetch {
	prefetch := &conversationListPrefetch{
		accountsByKey: map[string]*storage.Account{},
		statusesByID:  map[string]*storagemodels.Status{},
	}

	if len(conversations) == 0 || r == nil || r.Registry == nil {
		return prefetch
	}

	storageRepo := r.Registry.GetStorage()
	if storageRepo == nil {
		return prefetch
	}

	participantSet := make(map[string]struct{})
	statusIDs := make([]string, 0, len(conversations))
	statusSet := make(map[string]struct{})

	for _, conv := range conversations {
		for _, participant := range conversationListParticipantUsernames(conv, viewerUsername) {
			participantSet[participant] = struct{}{}
		}

		if previewStatusID := conversationListPreviewStatusID(conv); previewStatusID != "" {
			if _, ok := statusSet[previewStatusID]; !ok {
				statusSet[previewStatusID] = struct{}{}
				statusIDs = append(statusIDs, previewStatusID)
			}
		}
	}

	if len(participantSet) > 0 && storageRepo.Account() != nil {
		participants := make([]string, 0, len(participantSet))
		for participant := range participantSet {
			participants = append(participants, participant)
		}
		if accounts, err := storageRepo.Account().GetAccountsByUsernames(ctx, participants); err == nil {
			prefetch.accountsByKey = buildConversationAccountMap(accounts)
		}
	}

	r.addRemoteConversationAccountsToPrefetch(ctx, conversations, prefetch)

	if len(statusIDs) > 0 && storageRepo.Status() != nil {
		if statuses, err := storageRepo.Status().GetStatusesByIDs(ctx, statusIDs); err == nil {
			prefetch.statusesByID = buildConversationStatusMap(statuses)
			prefetch.statusesReady = true
		}
	} else {
		prefetch.statusesReady = true
	}

	return prefetch
}

func (r *Resolver) addRemoteConversationAccountsToPrefetch(ctx context.Context, conversations []*storagemodels.Conversation, prefetch *conversationListPrefetch) {
	if r == nil || r.Registry == nil || prefetch == nil {
		return
	}
	storageRepo := r.Registry.GetStorage()
	if storageRepo == nil || storageRepo.Actor() == nil {
		return
	}
	for _, conv := range conversations {
		for _, ref := range conversationListParticipantRefsForProjection(conv) {
			if ref.ParticipantType != storagemodels.ConversationParticipantTypeRemoteActor {
				continue
			}
			account := r.conversationRemoteAccountForParticipantRef(ctx, ref)
			if account == nil {
				continue
			}
			for _, key := range conversationAccountPrefetchKeys(account) {
				prefetch.accountsByKey[key] = account
			}
		}
	}
}

func (r *Resolver) conversationAccountByIdentifier(ctx context.Context, participantID string) *storage.Account {
	if r == nil {
		return nil
	}
	if r.Registry == nil || r.Registry.Accounts() == nil {
		return nil
	}
	result, err := r.Registry.Accounts().GetAccount(ctx, participantID)
	if err != nil || result == nil {
		return nil
	}
	return result
}

func (r *Resolver) prefetchedConversationAccount(prefetch *conversationListPrefetch, participantID string) *storage.Account {
	if prefetch == nil {
		return nil
	}

	candidate := strings.ToLower(strings.TrimSpace(participantID))
	if candidate == "" {
		return nil
	}
	if account := prefetch.accountsByKey[candidate]; account != nil {
		return account
	}

	derivedUsername := strings.ToLower(strings.TrimSpace(extractUsernameFromActorIdentifier(candidate)))
	if derivedUsername != "" && derivedUsername != candidate {
		return prefetch.accountsByKey[derivedUsername]
	}
	return nil
}

func (r *Resolver) prefetchedConversationAccountForRef(prefetch *conversationListPrefetch, ref storagemodels.ConversationParticipantRef) *storage.Account {
	ref = storagemodels.NormalizeConversationParticipantRef(ref)
	if prefetch == nil || ref.ParticipantID == "" {
		return nil
	}
	if account := prefetch.accountsByKey[strings.ToLower(ref.ParticipantID)]; account != nil {
		return account
	}
	if ref.Acct != "" {
		if account := prefetch.accountsByKey[strings.ToLower(ref.Acct)]; account != nil {
			return account
		}
	}
	return r.prefetchedConversationAccount(prefetch, ref.ParticipantID)
}

func (r *Resolver) conversationRemoteAccountForParticipantRef(ctx context.Context, ref storagemodels.ConversationParticipantRef) *storage.Account {
	ref = storagemodels.NormalizeConversationParticipantRef(ref)
	if ref.ParticipantID == "" || r == nil || r.Registry == nil {
		return nil
	}
	store := r.Registry.GetStorage()
	if store != nil && store.Actor() != nil {
		for _, candidate := range []string{ref.ParticipantID, ref.Acct} {
			candidate = strings.TrimSpace(candidate)
			if candidate == "" {
				continue
			}
			actor, err := store.Actor().GetCachedRemoteActor(ctx, candidate)
			if err == nil && actor != nil {
				return &storage.Account{Actor: actor}
			}
		}
	}
	return &storage.Account{Actor: conversationGraphSyntheticRemoteActor(ref)}
}

func conversationGraphSyntheticRemoteActor(ref storagemodels.ConversationParticipantRef) *activitypub.Actor {
	ref = storagemodels.NormalizeConversationParticipantRef(ref)
	username := strings.TrimSpace(ref.Acct)
	if username != "" {
		if handleUser, _, ok := strings.Cut(username, "@"); ok {
			username = handleUser
		}
	}
	if username == "" {
		username = strings.TrimSpace(extractUsernameFromActorIdentifier(ref.ParticipantID))
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

func (r *Resolver) conversationAccountsFromParticipantRefs(ctx context.Context, refs []storagemodels.ConversationParticipantRef, viewerUsername string, prefetch *conversationListPrefetch) []*activitypub.Actor {
	if r == nil {
		return nil
	}

	refs = storagemodels.NormalizeConversationParticipantRefs(refs)
	actors := make([]*activitypub.Actor, 0, len(refs))
	seen := make(map[string]struct{}, len(refs))
	viewerUsername = strings.ToLower(strings.TrimSpace(viewerUsername))
	for _, ref := range refs {
		candidate := strings.TrimSpace(ref.ParticipantID)
		if ref.ParticipantType != storagemodels.ConversationParticipantTypeRemoteActor {
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

		account := r.prefetchedConversationAccountForRef(prefetch, ref)
		if account == nil {
			if ref.ParticipantType == storagemodels.ConversationParticipantTypeRemoteActor {
				account = r.conversationRemoteAccountForParticipantRef(ctx, ref)
			} else {
				account = r.conversationAccountByIdentifier(ctx, ref.ParticipantID)
			}
		}
		if account == nil {
			continue
		}
		if actor := r.convertAccountToActor(account); actor != nil {
			actors = append(actors, actor)
		}
	}
	return actors
}

func (r *Resolver) conversationListLastStatus(ctx context.Context, conv *storagemodels.Conversation, viewerUsername string, prefetch *conversationListPrefetch) *model.Object {
	if conv == nil {
		return nil
	}

	previewStatusID := conversationListPreviewStatusID(conv)
	if previewStatusID == "" {
		return nil
	}

	statusCtx := ctx
	if prefetch != nil {
		statusCtx = withPrefetchedConversationAccounts(statusCtx, prefetch.accountsByKey)
		if prefetch.statusesReady {
			if previewStatus := prefetch.statusesByID[previewStatusID]; previewStatus != nil {
				if converted := r.convertStatusToObject(statusCtx, previewStatus); converted != nil {
					return converted
				}
			}
		}
	}

	if noteObject := r.noteObject(statusCtx, previewStatusID, viewerUsername); noteObject != nil {
		return noteObject
	}

	return r.conversationLastStatusFallback(ctx, conv.ID, viewerUsername)
}

func (r *Resolver) convertConversationListToGraphQL(ctx context.Context, conv *storagemodels.Conversation, prefetch *conversationListPrefetch) *model.Conversation {
	if conv == nil {
		return nil
	}

	viewerUsername := getUsernameFromContext(ctx)
	viewerMetadata := conversationListViewerMetadata(conv.ViewerState)
	accounts := r.conversationAccountsFromParticipantRefs(ctx, conversationListParticipantRefsForProjection(conv), viewerUsername, prefetch)
	lastStatus := r.conversationListLastStatus(ctx, conv, viewerUsername, prefetch)

	return &model.Conversation{
		ID:             conv.ID,
		LastStatus:     lastStatus,
		Unread:         conv.Unread,
		Accounts:       accounts,
		ViewerMetadata: viewerMetadata,
		CreatedAt:      model.Time(conv.CreatedAt),
		UpdatedAt:      model.Time(conv.UpdatedAt),
	}
}

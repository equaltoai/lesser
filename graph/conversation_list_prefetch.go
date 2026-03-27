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
	seen := make(map[string]struct{}, len(conv.Participants))
	usernames := make([]string, 0, len(conv.Participants))
	for _, participant := range conv.Participants {
		candidate := strings.ToLower(strings.TrimSpace(participant))
		if candidate == "" || candidate == viewerUsername {
			continue
		}
		if _, ok := seen[candidate]; ok {
			continue
		}
		seen[candidate] = struct{}{}
		usernames = append(usernames, candidate)
	}

	if len(usernames) == 0 && conv.ViewerState != nil {
		if counterpart := strings.ToLower(strings.TrimSpace(conv.ViewerState.CounterpartID)); counterpart != "" && counterpart != viewerUsername {
			usernames = append(usernames, counterpart)
		}
	}

	return usernames
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

func (r *Resolver) conversationAccountsFromPrefetch(ctx context.Context, participantIDs []string, viewerUsername string, prefetch *conversationListPrefetch) []*activitypub.Actor {
	if r == nil {
		return nil
	}

	actors := make([]*activitypub.Actor, 0, len(participantIDs))
	seen := make(map[string]struct{}, len(participantIDs))
	viewerUsername = strings.ToLower(strings.TrimSpace(viewerUsername))
	for _, participant := range participantIDs {
		candidate := strings.ToLower(strings.TrimSpace(participant))
		if candidate == "" || candidate == viewerUsername {
			continue
		}
		if _, ok := seen[candidate]; ok {
			continue
		}
		seen[candidate] = struct{}{}

		account := r.prefetchedConversationAccount(prefetch, participant)
		if account == nil {
			account = r.conversationAccountByIdentifier(ctx, participant)
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
	accounts := r.conversationAccountsFromPrefetch(ctx, conv.Participants, viewerUsername, prefetch)
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

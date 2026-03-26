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
	accountsByUsername map[string]*storage.Account
	statusesByID       map[string]*storagemodels.Status
}

func withPrefetchedConversationAccounts(ctx context.Context, accountsByUsername map[string]*storage.Account) context.Context {
	if len(accountsByUsername) == 0 {
		return ctx
	}
	return context.WithValue(ctx, prefetchedConversationAccountsContextKey{}, accountsByUsername)
}

func prefetchedConversationAccount(ctx context.Context, username string) *storage.Account {
	if ctx == nil || strings.TrimSpace(username) == "" {
		return nil
	}
	accountsByUsername, _ := ctx.Value(prefetchedConversationAccountsContextKey{}).(map[string]*storage.Account)
	if len(accountsByUsername) == 0 {
		return nil
	}
	return accountsByUsername[strings.ToLower(strings.TrimSpace(username))]
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

func buildConversationAccountMap(accounts []*storage.Account) map[string]*storage.Account {
	accountsByUsername := make(map[string]*storage.Account, len(accounts))
	for _, account := range accounts {
		if account == nil || account.User == nil {
			continue
		}
		username := strings.ToLower(strings.TrimSpace(account.User.Username))
		if username == "" {
			continue
		}
		accountsByUsername[username] = account
	}
	return accountsByUsername
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
		accountsByUsername: map[string]*storage.Account{},
		statusesByID:       map[string]*storagemodels.Status{},
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
			prefetch.accountsByUsername = buildConversationAccountMap(accounts)
		}
	}

	if len(statusIDs) > 0 && storageRepo.Status() != nil {
		if statuses, err := storageRepo.Status().GetStatusesByIDs(ctx, statusIDs); err == nil {
			prefetch.statusesByID = buildConversationStatusMap(statuses)
		}
	}

	return prefetch
}

func (r *Resolver) conversationAccountsFromPrefetch(ctx context.Context, participantIDs []string, viewerUsername string, prefetch *conversationListPrefetch) []*activitypub.Actor {
	if r == nil {
		return nil
	}
	if prefetch == nil {
		return r.conversationAccounts(ctx, participantIDs)
	}
	if len(prefetch.accountsByUsername) == 0 {
		return []*activitypub.Actor{}
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

		account := prefetch.accountsByUsername[candidate]
		if account == nil {
			continue
		}
		if actor := r.convertAccountToActor(account); actor != nil {
			actors = append(actors, actor)
		}
	}
	return actors
}

func (r *Resolver) convertConversationListToGraphQL(ctx context.Context, conv *storagemodels.Conversation, prefetch *conversationListPrefetch) *model.Conversation {
	if conv == nil {
		return nil
	}

	viewerUsername := getUsernameFromContext(ctx)
	viewerMetadata := conversationListViewerMetadata(conv.ViewerState)
	accounts := r.conversationAccountsFromPrefetch(ctx, conv.Participants, viewerUsername, prefetch)

	var lastStatus *model.Object
	if prefetch != nil {
		if previewStatus := prefetch.statusesByID[conversationListPreviewStatusID(conv)]; previewStatus != nil {
			lastStatus = r.convertStatusToObject(withPrefetchedConversationAccounts(ctx, prefetch.accountsByUsername), previewStatus)
		}
	} else {
		lastStatus = r.conversationLastStatus(ctx, conv, viewerUsername)
	}

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

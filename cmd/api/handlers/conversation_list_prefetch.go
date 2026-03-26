package handlers

import (
	"context"
	"strings"

	"github.com/equaltoai/lesser/pkg/storage"
	storageModels "github.com/equaltoai/lesser/pkg/storage/models"
)

type prefetchedConversationAccountsContextKey struct{}

type conversationAPIPrefetch struct {
	accountsByUsername map[string]*storage.Account
	statusesByID       map[string]*storageModels.Status
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

func buildConversationAPIAccountMap(accounts []*storage.Account) map[string]*storage.Account {
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
		accountsByUsername: map[string]*storage.Account{},
		statusesByID:       map[string]*storageModels.Status{},
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
			prefetch.accountsByUsername = buildConversationAPIAccountMap(accounts)
		}
	}

	if len(statusIDs) > 0 && h.repos.Status() != nil {
		if statuses, err := h.repos.Status().GetStatusesByIDs(ctx, statusIDs); err == nil {
			prefetch.statusesByID = buildConversationAPIStatusMap(statuses)
		}
	}

	return prefetch
}

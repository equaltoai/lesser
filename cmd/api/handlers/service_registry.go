package handlers

import (
	"context"

	"github.com/equaltoai/lesser/pkg/services"
	"github.com/equaltoai/lesser/pkg/services/accounts"
	"github.com/equaltoai/lesser/pkg/services/ai"
	"github.com/equaltoai/lesser/pkg/services/conversations"
	"github.com/equaltoai/lesser/pkg/services/emoji"
	"github.com/equaltoai/lesser/pkg/services/lists"
	"github.com/equaltoai/lesser/pkg/services/media"
	"github.com/equaltoai/lesser/pkg/services/notes"
	"github.com/equaltoai/lesser/pkg/services/notifications"
	"github.com/equaltoai/lesser/pkg/services/relationships"
	"github.com/equaltoai/lesser/pkg/services/scheduled"
	"github.com/equaltoai/lesser/pkg/services/search"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
)

// ServiceRegistry is a thin abstraction over pkg/services.Registry to make cmd/api/handlers unit-testable
// without requiring DynamoDB/AWS/network side-effects.
type ServiceRegistry interface {
	Accounts() AccountsService
	AI() AIService
	Conversations() ConversationsService
	Emoji() EmojiService
	Lists() ListsService
	Media() MediaService
	Notes() NotesService
	Notifications() NotificationsService
	Relationships() RelationshipsService
	Scheduled() ScheduledService
	Search() SearchService
}

// AccountsService defines the subset of account-related operations used by the Lift API
type AccountsService interface {
	CreateAuthorizationCode(ctx context.Context, cmd *accounts.CreateAuthorizationCodeCommand) (*accounts.CreateAuthorizationCodeResult, error)
	GetAccount(ctx context.Context, username string) (*storage.Account, error)
	GetAccountPins(ctx context.Context, query *accounts.GetAccountPinsQuery) (*accounts.AccountPinsResult, error)
	GetFamiliarFollowers(ctx context.Context, query *accounts.GetFamiliarFollowersQuery) (*accounts.FamiliarFollowersResult, error)
	GetInstanceStats(ctx context.Context, query *accounts.GetInstanceStatsQuery) (*accounts.GetInstanceStatsResult, error)
	GetMarkers(ctx context.Context, query *accounts.GetMarkersQuery) (*accounts.GetMarkersResult, error)
	GetOAuthApp(ctx context.Context, query *accounts.GetOAuthAppQuery) (*accounts.GetOAuthAppResult, error)
	GetPreferences(ctx context.Context, query *accounts.GetPreferencesQuery) (*accounts.PreferencesResult, error)
	GetUserAppConsent(ctx context.Context, query *accounts.GetUserAppConsentQuery) (*accounts.GetUserAppConsentResult, error)
	IsAccountPinned(ctx context.Context, userID, pinnedActorID string) (bool, error)
	LookupAccount(ctx context.Context, query *accounts.LookupAccountQuery) (*storage.Account, error)
	PinAccount(ctx context.Context, cmd *accounts.PinAccountCommand) (*accounts.RelationshipResult, error)
	RegisterAccount(ctx context.Context, cmd *accounts.RegisterAccountCommand) (*accounts.RegisterAccountResult, error)
	RemoveFollower(ctx context.Context, cmd *accounts.RemoveFollowerCommand) (*accounts.RelationshipResult, error)
	SaveMarker(ctx context.Context, cmd *accounts.SaveMarkerCommand) (*accounts.SaveMarkerResult, error)
	SetAccountNote(ctx context.Context, cmd *accounts.SetAccountNoteCommand) (*accounts.RelationshipResult, error)
	StoreOAuthState(ctx context.Context, cmd *accounts.StoreOAuthStateCommand) (*accounts.StoreOAuthStateResult, error)
	UnpinAccount(ctx context.Context, cmd *accounts.UnpinAccountCommand) (*accounts.RelationshipResult, error)
	UpdatePreferences(ctx context.Context, cmd *accounts.UpdatePreferencesCommand) (*accounts.PreferencesResult, error)
	UpdateProfile(ctx context.Context, cmd *accounts.UpdateProfileCommand) (*accounts.AccountResult, error)
}

// AIService defines the subset of AI-related operations used by the Lift API
type AIService interface {
	GetAnalysis(ctx context.Context, query *ai.GetAnalysisQuery) (*ai.GetAnalysisResult, error)
	GetStats(ctx context.Context, query *ai.GetStatsQuery) (*ai.GetStatsResult, error)
	QueueForAnalysis(ctx context.Context, cmd *ai.QueueAnalysisCommand) (*ai.QueueAnalysisResult, error)
}

// ConversationsService defines the subset of conversation-related operations used by the Lift API
type ConversationsService interface {
	DeleteConversation(ctx context.Context, cmd *conversations.DeleteConversationCommand) (*conversations.ConversationResult, error)
	ListConversations(ctx context.Context, query *conversations.ListConversationsQuery) (*conversations.Result, error)
	MarkConversationRead(ctx context.Context, cmd *conversations.MarkConversationReadCommand) (*conversations.ConversationResult, error)
	SendDirectMessage(ctx context.Context, cmd *conversations.SendDirectMessageCommand) (*conversations.MessageResult, error)
}

// EmojiService defines the subset of emoji-related operations used by the Lift API
type EmojiService interface {
	CreateEmoji(ctx context.Context, cmd *emoji.CreateEmojiCommand) (*emoji.Result, error)
	DeleteEmoji(ctx context.Context, cmd *emoji.DeleteEmojiCommand) error
	ListEmojis(ctx context.Context, query *emoji.ListEmojisQuery) (*emoji.ListResult, error)
	UpdateEmoji(ctx context.Context, cmd *emoji.UpdateEmojiCommand) (*emoji.Result, error)
}

// ListsService defines the subset of list-related operations used by the Lift API
type ListsService interface {
	AddToList(ctx context.Context, cmd *lists.AddToListCommand) (*lists.MembershipResult, error)
	CreateList(ctx context.Context, cmd *lists.CreateListCommand) (*lists.ListResult, error)
	DeleteList(ctx context.Context, cmd *lists.DeleteListCommand) error
	GetList(ctx context.Context, query *lists.GetListQuery) (*storagemodels.List, error)
	GetListMembers(ctx context.Context, query *lists.GetListMembersQuery) (*lists.MembersResult, error)
	GetListTimeline(ctx context.Context, query *lists.GetListTimelineQuery) (*lists.TimelineResult, error)
	ListUserLists(ctx context.Context, query *lists.ListUserListsQuery) (*lists.Result, error)
	RemoveFromList(ctx context.Context, cmd *lists.RemoveFromListCommand) (*lists.MembershipResult, error)
	UpdateList(ctx context.Context, cmd *lists.UpdateListCommand) (*lists.ListResult, error)
}

// MediaService defines the subset of media-related operations used by the Lift API
type MediaService interface {
	GetMedia(ctx context.Context, query *media.GetMediaQuery) (*storagemodels.Media, error)
	UpdateMedia(ctx context.Context, cmd *media.UpdateMediaCommand) (*media.UpdateResult, error)
	UploadMedia(ctx context.Context, cmd *media.UploadMediaCommand) (*media.Result, error)
}

// NotesService defines the subset of note-related operations used by the Lift API
type NotesService interface {
	BookmarkNote(ctx context.Context, cmd *notes.BookmarkNoteCommand) (*notes.BookmarkResult, error)
	CountNotesByAuthor(ctx context.Context, authorID string) (int64, error)
	CountReplies(ctx context.Context, statusID string) (int, error)
	CreateNote(ctx context.Context, cmd *notes.CreateNoteCommand) (*notes.NoteResult, error)
	CreateCommunityNote(ctx context.Context, cmd *notes.CreateCommunityNoteCommand) (*notes.CreateCommunityNoteResult, error)
	CreateCommunityNoteVote(ctx context.Context, cmd *notes.CreateCommunityNoteVoteCommand) (*notes.CreateCommunityNoteVoteResult, error)
	DeleteNote(ctx context.Context, cmd *notes.DeleteNoteCommand) error
	GetBookmarks(ctx context.Context, query *notes.GetBookmarksQuery) (*notes.Result, error)
	GetBoostCount(ctx context.Context, statusID string) (int64, error)
	GetCommunityNote(ctx context.Context, query *notes.GetCommunityNoteQuery) (*notes.GetCommunityNoteResult, error)
	GetCommunityNotesByAuthor(ctx context.Context, query *notes.GetCommunityNotesByAuthorQuery) (*notes.GetCommunityNotesByAuthorResult, error)
	GetFavoritedNotes(ctx context.Context, query *notes.ListNotesQuery) (*notes.Result, error)
	GetLikeCount(ctx context.Context, statusID string) (int64, error)
	GetLikers(ctx context.Context, query *notes.GetLikersQuery) (*notes.UsersResult, error)
	GetNote(ctx context.Context, statusID string) (*storagemodels.Status, error)
	GetNoteWithViewer(ctx context.Context, query *notes.GetNoteQuery) (*storagemodels.Status, error)
	GetRebloggers(ctx context.Context, query *notes.GetRebloggersQuery) (*notes.UsersResult, error)
	GetSearchSuggestions(ctx context.Context, query *notes.GetSearchSuggestionsQuery) (*notes.GetSearchSuggestionsResult, error)
	GetUpdateHistory(ctx context.Context, query *notes.GetUpdateHistoryQuery) (*notes.GetUpdateHistoryResult, error)
	GetUserTimeline(ctx context.Context, actorID string, opts interfaces.PaginationOptions) (*notes.GetUserTimelineResult, error)
	GetVisibleCommunityNotes(ctx context.Context, query *notes.GetVisibleCommunityNotesQuery) (*notes.GetVisibleCommunityNotesResult, error)
	HasLiked(ctx context.Context, userID, statusID string) (bool, error)
	HasReblogged(ctx context.Context, userID, statusID string) (bool, error)
	IsBookmarked(ctx context.Context, userID, statusID string) (bool, error)
	LikeNote(ctx context.Context, cmd *notes.LikeNoteCommand) (*notes.LikeResult, error)
	ListNotes(ctx context.Context, query *notes.ListNotesQuery) (*notes.Result, error)
	ReblogNote(ctx context.Context, cmd *notes.ReblogNoteCommand) (*notes.LikeResult, error)
	UnlikeNote(ctx context.Context, cmd *notes.UnlikeNoteCommand) (*notes.LikeResult, error)
	UnbookmarkNote(ctx context.Context, cmd *notes.UnbookmarkNoteCommand) (*notes.BookmarkResult, error)
	UnreblogNote(ctx context.Context, cmd *notes.UnreblogNoteCommand) (*notes.LikeResult, error)
}

// NotificationsService defines the subset of notification-related operations used by the Lift API
type NotificationsService interface {
	ClearNotifications(ctx context.Context, cmd *notifications.ClearCommand) (*notifications.ClearResult, error)
	CreateNotification(ctx context.Context, cmd *notifications.CreateNotificationCommand) (*notifications.NotificationResult, error)
	ListNotifications(ctx context.Context, query *notifications.ListNotificationsQuery) (*notifications.NotificationListResult, error)
	MarkAsRead(ctx context.Context, cmd *notifications.MarkAsReadCommand) (*notifications.NotificationResult, error)
}

// RelationshipsService defines the subset of relationship-related operations used by the Lift API
type RelationshipsService interface {
	AcceptFollowRequest(ctx context.Context, cmd *relationships.AcceptFollowRequestCommand) (*relationships.RelationshipResult, error)
	AddDomainBlock(ctx context.Context, cmd *relationships.AddDomainBlockCommand) error
	Block(ctx context.Context, cmd *relationships.BlockCommand) (*relationships.RelationshipResult, error)
	CountFollowers(ctx context.Context, username string) (int64, error)
	CountFollowing(ctx context.Context, username string) (int64, error)
	Follow(ctx context.Context, cmd *relationships.FollowCommand) (*relationships.FollowResult, error)
	GetBlockedUsers(ctx context.Context, query *relationships.GetBlockedUsersQuery) (*relationships.BlockedUsersResult, error)
	GetDomainBlocks(ctx context.Context, query *relationships.GetDomainBlocksQuery) (*relationships.DomainBlocksResult, error)
	GetFollowers(ctx context.Context, username string, limit int, cursor string) ([]*storage.Account, string, error)
	GetFollowing(ctx context.Context, username string, limit int, cursor string) ([]*storage.Account, string, error)
	GetMutedUsers(ctx context.Context, query *relationships.GetMutedUsersQuery) (*relationships.MutedUsersResult, error)
	GetPendingFollowRequests(ctx context.Context, query *relationships.GetFollowRequestsQuery) (*relationships.FollowRequestsResult, error)
	GetRelationship(ctx context.Context, requesterID, targetID string) (*relationships.RelationshipData, error)
	GetRelationships(ctx context.Context, query *relationships.GetRelationshipsQuery) (*relationships.Result, error)
	IsMuted(ctx context.Context, muterID, mutedID string) (bool, error)
	Mute(ctx context.Context, cmd *relationships.MuteCommand) (*relationships.RelationshipResult, error)
	RejectFollowRequest(ctx context.Context, cmd *relationships.RejectFollowRequestCommand) (*relationships.RelationshipResult, error)
	RemoveDomainBlock(ctx context.Context, cmd *relationships.RemoveDomainBlockCommand) error
	Unblock(ctx context.Context, cmd *relationships.UnblockCommand) (*relationships.RelationshipResult, error)
	Unfollow(ctx context.Context, cmd *relationships.UnfollowCommand) (*relationships.RelationshipResult, error)
	Unmute(ctx context.Context, cmd *relationships.UnmuteCommand) (*relationships.RelationshipResult, error)
}

// ScheduledService defines the subset of scheduled status operations used by the Lift API
type ScheduledService interface {
	CreateScheduledStatus(ctx context.Context, cmd *scheduled.CreateScheduledStatusCommand) (*scheduled.StatusResult, error)
	DeleteScheduledStatus(ctx context.Context, cmd *scheduled.DeleteScheduledStatusCommand) error
	GetScheduledMediaAttachments(ctx context.Context, scheduledStatusID string, username string) ([]*storagemodels.Media, error)
	GetScheduledStatus(ctx context.Context, query *scheduled.GetScheduledStatusQuery) (*scheduled.StatusResult, error)
	ListScheduledStatuses(ctx context.Context, query *scheduled.ListScheduledStatusesQuery) (*scheduled.StatusListResult, error)
	UpdateScheduledStatus(ctx context.Context, cmd *scheduled.UpdateScheduledStatusCommand) (*scheduled.StatusResult, error)
}

// SearchService defines the subset of search-related operations used by the Lift API
type SearchService interface {
	GetDirectory(ctx context.Context, query *search.DirectoryQuery) (*search.DirectoryResult, error)
	GetSuggestions(ctx context.Context, query *search.SuggestionsQuery) (*search.SuggestionsResult, error)
	RemoveSuggestion(ctx context.Context, cmd *search.RemoveSuggestionCommand) error
}

type servicesRegistryAdapter struct {
	registry *services.Registry
}

func newServiceRegistry(registry *services.Registry) ServiceRegistry {
	if registry == nil {
		return nil
	}
	return &servicesRegistryAdapter{registry: registry}
}

func (a *servicesRegistryAdapter) Accounts() AccountsService {
	if a == nil || a.registry == nil {
		return nil
	}
	svc := a.registry.Accounts()
	if svc == nil {
		return nil
	}
	return svc
}

func (a *servicesRegistryAdapter) AI() AIService {
	if a == nil || a.registry == nil {
		return nil
	}
	svc := a.registry.AI()
	if svc == nil {
		return nil
	}
	return svc
}

func (a *servicesRegistryAdapter) Conversations() ConversationsService {
	if a == nil || a.registry == nil {
		return nil
	}
	svc := a.registry.Conversations()
	if svc == nil {
		return nil
	}
	return svc
}

func (a *servicesRegistryAdapter) Emoji() EmojiService {
	if a == nil || a.registry == nil {
		return nil
	}
	svc := a.registry.Emoji()
	if svc == nil {
		return nil
	}
	return svc
}

func (a *servicesRegistryAdapter) Lists() ListsService {
	if a == nil || a.registry == nil {
		return nil
	}
	svc := a.registry.Lists()
	if svc == nil {
		return nil
	}
	return svc
}

func (a *servicesRegistryAdapter) Media() MediaService {
	if a == nil || a.registry == nil {
		return nil
	}
	svc := a.registry.Media()
	if svc == nil {
		return nil
	}
	return svc
}

func (a *servicesRegistryAdapter) Notes() NotesService {
	if a == nil || a.registry == nil {
		return nil
	}
	svc := a.registry.Notes()
	if svc == nil {
		return nil
	}
	return svc
}

func (a *servicesRegistryAdapter) Notifications() NotificationsService {
	if a == nil || a.registry == nil {
		return nil
	}
	svc := a.registry.Notifications()
	if svc == nil {
		return nil
	}
	return svc
}

func (a *servicesRegistryAdapter) Relationships() RelationshipsService {
	if a == nil || a.registry == nil {
		return nil
	}
	svc := a.registry.Relationships()
	if svc == nil {
		return nil
	}
	return svc
}

func (a *servicesRegistryAdapter) Scheduled() ScheduledService {
	if a == nil || a.registry == nil {
		return nil
	}
	svc := a.registry.Scheduled()
	if svc == nil {
		return nil
	}
	return svc
}

func (a *servicesRegistryAdapter) Search() SearchService {
	if a == nil || a.registry == nil {
		return nil
	}
	svc := a.registry.Search()
	if svc == nil {
		return nil
	}
	return svc
}

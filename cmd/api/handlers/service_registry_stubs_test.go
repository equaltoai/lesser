package handlers

import (
	"context"
	"fmt"

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

func missingStub(method string) error {
	return fmt.Errorf("lift test stub missing: %s", method)
}

type RegistryStub struct {
	AccountsSvc      AccountsService
	AISvc            AIService
	ConversationsSvc ConversationsService
	EmojiSvc         EmojiService
	ListsSvc         ListsService
	MediaSvc         MediaService
	NotesSvc         NotesService
	NotificationsSvc NotificationsService
	RelationshipsSvc RelationshipsService
	ScheduledSvc     ScheduledService
	SearchSvc        SearchService
}

var _ ServiceRegistry = (*RegistryStub)(nil)

func (r *RegistryStub) Accounts() AccountsService           { return r.AccountsSvc }
func (r *RegistryStub) AI() AIService                       { return r.AISvc }
func (r *RegistryStub) Conversations() ConversationsService { return r.ConversationsSvc }
func (r *RegistryStub) Emoji() EmojiService                 { return r.EmojiSvc }
func (r *RegistryStub) Lists() ListsService                 { return r.ListsSvc }
func (r *RegistryStub) Media() MediaService                 { return r.MediaSvc }
func (r *RegistryStub) Notes() NotesService                 { return r.NotesSvc }
func (r *RegistryStub) Notifications() NotificationsService { return r.NotificationsSvc }
func (r *RegistryStub) Relationships() RelationshipsService { return r.RelationshipsSvc }
func (r *RegistryStub) Scheduled() ScheduledService         { return r.ScheduledSvc }
func (r *RegistryStub) Search() SearchService               { return r.SearchSvc }

type AccountsServiceStub struct {
	CreateAuthorizationCodeFunc func(ctx context.Context, cmd *accounts.CreateAuthorizationCodeCommand) (*accounts.CreateAuthorizationCodeResult, error)
	GetAccountFunc              func(ctx context.Context, username string) (*storage.Account, error)
	GetAccountPinsFunc          func(ctx context.Context, query *accounts.GetAccountPinsQuery) (*accounts.AccountPinsResult, error)
	GetFamiliarFollowersFunc    func(ctx context.Context, query *accounts.GetFamiliarFollowersQuery) (*accounts.FamiliarFollowersResult, error)
	GetInstanceStatsFunc        func(ctx context.Context, query *accounts.GetInstanceStatsQuery) (*accounts.GetInstanceStatsResult, error)
	GetMarkersFunc              func(ctx context.Context, query *accounts.GetMarkersQuery) (*accounts.GetMarkersResult, error)
	GetOAuthAppFunc             func(ctx context.Context, query *accounts.GetOAuthAppQuery) (*accounts.GetOAuthAppResult, error)
	GetPreferencesFunc          func(ctx context.Context, query *accounts.GetPreferencesQuery) (*accounts.PreferencesResult, error)
	GetUserAppConsentFunc       func(ctx context.Context, query *accounts.GetUserAppConsentQuery) (*accounts.GetUserAppConsentResult, error)
	IsAccountPinnedFunc         func(ctx context.Context, userID, pinnedActorID string) (bool, error)
	LookupAccountFunc           func(ctx context.Context, query *accounts.LookupAccountQuery) (*storage.Account, error)
	PinAccountFunc              func(ctx context.Context, cmd *accounts.PinAccountCommand) (*accounts.RelationshipResult, error)
	RegisterAccountFunc         func(ctx context.Context, cmd *accounts.RegisterAccountCommand) (*accounts.RegisterAccountResult, error)
	RemoveFollowerFunc          func(ctx context.Context, cmd *accounts.RemoveFollowerCommand) (*accounts.RelationshipResult, error)
	SaveMarkerFunc              func(ctx context.Context, cmd *accounts.SaveMarkerCommand) (*accounts.SaveMarkerResult, error)
	SetAccountNoteFunc          func(ctx context.Context, cmd *accounts.SetAccountNoteCommand) (*accounts.RelationshipResult, error)
	StoreOAuthStateFunc         func(ctx context.Context, cmd *accounts.StoreOAuthStateCommand) (*accounts.StoreOAuthStateResult, error)
	UnpinAccountFunc            func(ctx context.Context, cmd *accounts.UnpinAccountCommand) (*accounts.RelationshipResult, error)
	UpdatePreferencesFunc       func(ctx context.Context, cmd *accounts.UpdatePreferencesCommand) (*accounts.PreferencesResult, error)
	UpdateProfileFunc           func(ctx context.Context, cmd *accounts.UpdateProfileCommand) (*accounts.AccountResult, error)
}

var _ AccountsService = (*AccountsServiceStub)(nil)

func (s *AccountsServiceStub) CreateAuthorizationCode(ctx context.Context, cmd *accounts.CreateAuthorizationCodeCommand) (*accounts.CreateAuthorizationCodeResult, error) {
	if s != nil && s.CreateAuthorizationCodeFunc != nil {
		return s.CreateAuthorizationCodeFunc(ctx, cmd)
	}
	return nil, missingStub("AccountsService.CreateAuthorizationCode")
}

func (s *AccountsServiceStub) GetAccount(ctx context.Context, username string) (*storage.Account, error) {
	if s != nil && s.GetAccountFunc != nil {
		return s.GetAccountFunc(ctx, username)
	}
	return nil, missingStub("AccountsService.GetAccount")
}

func (s *AccountsServiceStub) GetAccountPins(ctx context.Context, query *accounts.GetAccountPinsQuery) (*accounts.AccountPinsResult, error) {
	if s != nil && s.GetAccountPinsFunc != nil {
		return s.GetAccountPinsFunc(ctx, query)
	}
	return nil, missingStub("AccountsService.GetAccountPins")
}

func (s *AccountsServiceStub) GetFamiliarFollowers(ctx context.Context, query *accounts.GetFamiliarFollowersQuery) (*accounts.FamiliarFollowersResult, error) {
	if s != nil && s.GetFamiliarFollowersFunc != nil {
		return s.GetFamiliarFollowersFunc(ctx, query)
	}
	return nil, missingStub("AccountsService.GetFamiliarFollowers")
}

func (s *AccountsServiceStub) GetInstanceStats(ctx context.Context, query *accounts.GetInstanceStatsQuery) (*accounts.GetInstanceStatsResult, error) {
	if s != nil && s.GetInstanceStatsFunc != nil {
		return s.GetInstanceStatsFunc(ctx, query)
	}
	return nil, missingStub("AccountsService.GetInstanceStats")
}

func (s *AccountsServiceStub) GetMarkers(ctx context.Context, query *accounts.GetMarkersQuery) (*accounts.GetMarkersResult, error) {
	if s != nil && s.GetMarkersFunc != nil {
		return s.GetMarkersFunc(ctx, query)
	}
	return nil, missingStub("AccountsService.GetMarkers")
}

func (s *AccountsServiceStub) GetOAuthApp(ctx context.Context, query *accounts.GetOAuthAppQuery) (*accounts.GetOAuthAppResult, error) {
	if s != nil && s.GetOAuthAppFunc != nil {
		return s.GetOAuthAppFunc(ctx, query)
	}
	return nil, missingStub("AccountsService.GetOAuthApp")
}

func (s *AccountsServiceStub) GetPreferences(ctx context.Context, query *accounts.GetPreferencesQuery) (*accounts.PreferencesResult, error) {
	if s != nil && s.GetPreferencesFunc != nil {
		return s.GetPreferencesFunc(ctx, query)
	}
	return nil, missingStub("AccountsService.GetPreferences")
}

func (s *AccountsServiceStub) GetUserAppConsent(ctx context.Context, query *accounts.GetUserAppConsentQuery) (*accounts.GetUserAppConsentResult, error) {
	if s != nil && s.GetUserAppConsentFunc != nil {
		return s.GetUserAppConsentFunc(ctx, query)
	}
	return nil, missingStub("AccountsService.GetUserAppConsent")
}

func (s *AccountsServiceStub) IsAccountPinned(ctx context.Context, userID, pinnedActorID string) (bool, error) {
	if s != nil && s.IsAccountPinnedFunc != nil {
		return s.IsAccountPinnedFunc(ctx, userID, pinnedActorID)
	}
	return false, missingStub("AccountsService.IsAccountPinned")
}

func (s *AccountsServiceStub) LookupAccount(ctx context.Context, query *accounts.LookupAccountQuery) (*storage.Account, error) {
	if s != nil && s.LookupAccountFunc != nil {
		return s.LookupAccountFunc(ctx, query)
	}
	return nil, missingStub("AccountsService.LookupAccount")
}

func (s *AccountsServiceStub) PinAccount(ctx context.Context, cmd *accounts.PinAccountCommand) (*accounts.RelationshipResult, error) {
	if s != nil && s.PinAccountFunc != nil {
		return s.PinAccountFunc(ctx, cmd)
	}
	return nil, missingStub("AccountsService.PinAccount")
}

func (s *AccountsServiceStub) RegisterAccount(ctx context.Context, cmd *accounts.RegisterAccountCommand) (*accounts.RegisterAccountResult, error) {
	if s != nil && s.RegisterAccountFunc != nil {
		return s.RegisterAccountFunc(ctx, cmd)
	}
	return nil, missingStub("AccountsService.RegisterAccount")
}

func (s *AccountsServiceStub) RemoveFollower(ctx context.Context, cmd *accounts.RemoveFollowerCommand) (*accounts.RelationshipResult, error) {
	if s != nil && s.RemoveFollowerFunc != nil {
		return s.RemoveFollowerFunc(ctx, cmd)
	}
	return nil, missingStub("AccountsService.RemoveFollower")
}

func (s *AccountsServiceStub) SaveMarker(ctx context.Context, cmd *accounts.SaveMarkerCommand) (*accounts.SaveMarkerResult, error) {
	if s != nil && s.SaveMarkerFunc != nil {
		return s.SaveMarkerFunc(ctx, cmd)
	}
	return nil, missingStub("AccountsService.SaveMarker")
}

func (s *AccountsServiceStub) SetAccountNote(ctx context.Context, cmd *accounts.SetAccountNoteCommand) (*accounts.RelationshipResult, error) {
	if s != nil && s.SetAccountNoteFunc != nil {
		return s.SetAccountNoteFunc(ctx, cmd)
	}
	return nil, missingStub("AccountsService.SetAccountNote")
}

func (s *AccountsServiceStub) StoreOAuthState(ctx context.Context, cmd *accounts.StoreOAuthStateCommand) (*accounts.StoreOAuthStateResult, error) {
	if s != nil && s.StoreOAuthStateFunc != nil {
		return s.StoreOAuthStateFunc(ctx, cmd)
	}
	return nil, missingStub("AccountsService.StoreOAuthState")
}

func (s *AccountsServiceStub) UnpinAccount(ctx context.Context, cmd *accounts.UnpinAccountCommand) (*accounts.RelationshipResult, error) {
	if s != nil && s.UnpinAccountFunc != nil {
		return s.UnpinAccountFunc(ctx, cmd)
	}
	return nil, missingStub("AccountsService.UnpinAccount")
}

func (s *AccountsServiceStub) UpdatePreferences(ctx context.Context, cmd *accounts.UpdatePreferencesCommand) (*accounts.PreferencesResult, error) {
	if s != nil && s.UpdatePreferencesFunc != nil {
		return s.UpdatePreferencesFunc(ctx, cmd)
	}
	return nil, missingStub("AccountsService.UpdatePreferences")
}

func (s *AccountsServiceStub) UpdateProfile(ctx context.Context, cmd *accounts.UpdateProfileCommand) (*accounts.AccountResult, error) {
	if s != nil && s.UpdateProfileFunc != nil {
		return s.UpdateProfileFunc(ctx, cmd)
	}
	return nil, missingStub("AccountsService.UpdateProfile")
}

type AIServiceStub struct {
	GetAnalysisFunc      func(ctx context.Context, query *ai.GetAnalysisQuery) (*ai.GetAnalysisResult, error)
	GetStatsFunc         func(ctx context.Context, query *ai.GetStatsQuery) (*ai.GetStatsResult, error)
	QueueForAnalysisFunc func(ctx context.Context, cmd *ai.QueueAnalysisCommand) (*ai.QueueAnalysisResult, error)
}

var _ AIService = (*AIServiceStub)(nil)

func (s *AIServiceStub) GetAnalysis(ctx context.Context, query *ai.GetAnalysisQuery) (*ai.GetAnalysisResult, error) {
	if s != nil && s.GetAnalysisFunc != nil {
		return s.GetAnalysisFunc(ctx, query)
	}
	return nil, missingStub("AIService.GetAnalysis")
}

func (s *AIServiceStub) GetStats(ctx context.Context, query *ai.GetStatsQuery) (*ai.GetStatsResult, error) {
	if s != nil && s.GetStatsFunc != nil {
		return s.GetStatsFunc(ctx, query)
	}
	return nil, missingStub("AIService.GetStats")
}

func (s *AIServiceStub) QueueForAnalysis(ctx context.Context, cmd *ai.QueueAnalysisCommand) (*ai.QueueAnalysisResult, error) {
	if s != nil && s.QueueForAnalysisFunc != nil {
		return s.QueueForAnalysisFunc(ctx, cmd)
	}
	return nil, missingStub("AIService.QueueForAnalysis")
}

type ConversationsServiceStub struct {
	DeleteConversationFunc   func(ctx context.Context, cmd *conversations.DeleteConversationCommand) (*conversations.ConversationResult, error)
	ListConversationsFunc    func(ctx context.Context, query *conversations.ListConversationsQuery) (*conversations.Result, error)
	MarkConversationReadFunc func(ctx context.Context, cmd *conversations.MarkConversationReadCommand) (*conversations.ConversationResult, error)
}

var _ ConversationsService = (*ConversationsServiceStub)(nil)

func (s *ConversationsServiceStub) DeleteConversation(ctx context.Context, cmd *conversations.DeleteConversationCommand) (*conversations.ConversationResult, error) {
	if s != nil && s.DeleteConversationFunc != nil {
		return s.DeleteConversationFunc(ctx, cmd)
	}
	return nil, missingStub("ConversationsService.DeleteConversation")
}

func (s *ConversationsServiceStub) ListConversations(ctx context.Context, query *conversations.ListConversationsQuery) (*conversations.Result, error) {
	if s != nil && s.ListConversationsFunc != nil {
		return s.ListConversationsFunc(ctx, query)
	}
	return nil, missingStub("ConversationsService.ListConversations")
}

func (s *ConversationsServiceStub) MarkConversationRead(ctx context.Context, cmd *conversations.MarkConversationReadCommand) (*conversations.ConversationResult, error) {
	if s != nil && s.MarkConversationReadFunc != nil {
		return s.MarkConversationReadFunc(ctx, cmd)
	}
	return nil, missingStub("ConversationsService.MarkConversationRead")
}

type EmojiServiceStub struct {
	CreateEmojiFunc func(ctx context.Context, cmd *emoji.CreateEmojiCommand) (*emoji.Result, error)
	DeleteEmojiFunc func(ctx context.Context, cmd *emoji.DeleteEmojiCommand) error
	ListEmojisFunc  func(ctx context.Context, query *emoji.ListEmojisQuery) (*emoji.ListResult, error)
	UpdateEmojiFunc func(ctx context.Context, cmd *emoji.UpdateEmojiCommand) (*emoji.Result, error)
}

var _ EmojiService = (*EmojiServiceStub)(nil)

func (s *EmojiServiceStub) CreateEmoji(ctx context.Context, cmd *emoji.CreateEmojiCommand) (*emoji.Result, error) {
	if s != nil && s.CreateEmojiFunc != nil {
		return s.CreateEmojiFunc(ctx, cmd)
	}
	return nil, missingStub("EmojiService.CreateEmoji")
}

func (s *EmojiServiceStub) DeleteEmoji(ctx context.Context, cmd *emoji.DeleteEmojiCommand) error {
	if s != nil && s.DeleteEmojiFunc != nil {
		return s.DeleteEmojiFunc(ctx, cmd)
	}
	return missingStub("EmojiService.DeleteEmoji")
}

func (s *EmojiServiceStub) ListEmojis(ctx context.Context, query *emoji.ListEmojisQuery) (*emoji.ListResult, error) {
	if s != nil && s.ListEmojisFunc != nil {
		return s.ListEmojisFunc(ctx, query)
	}
	return nil, missingStub("EmojiService.ListEmojis")
}

func (s *EmojiServiceStub) UpdateEmoji(ctx context.Context, cmd *emoji.UpdateEmojiCommand) (*emoji.Result, error) {
	if s != nil && s.UpdateEmojiFunc != nil {
		return s.UpdateEmojiFunc(ctx, cmd)
	}
	return nil, missingStub("EmojiService.UpdateEmoji")
}

type ListsServiceStub struct {
	AddToListFunc       func(ctx context.Context, cmd *lists.AddToListCommand) (*lists.MembershipResult, error)
	CreateListFunc      func(ctx context.Context, cmd *lists.CreateListCommand) (*lists.ListResult, error)
	DeleteListFunc      func(ctx context.Context, cmd *lists.DeleteListCommand) error
	GetListFunc         func(ctx context.Context, query *lists.GetListQuery) (*storagemodels.List, error)
	GetListMembersFunc  func(ctx context.Context, query *lists.GetListMembersQuery) (*lists.MembersResult, error)
	GetListTimelineFunc func(ctx context.Context, query *lists.GetListTimelineQuery) (*lists.TimelineResult, error)
	ListUserListsFunc   func(ctx context.Context, query *lists.ListUserListsQuery) (*lists.Result, error)
	RemoveFromListFunc  func(ctx context.Context, cmd *lists.RemoveFromListCommand) (*lists.MembershipResult, error)
	UpdateListFunc      func(ctx context.Context, cmd *lists.UpdateListCommand) (*lists.ListResult, error)
}

var _ ListsService = (*ListsServiceStub)(nil)

func (s *ListsServiceStub) AddToList(ctx context.Context, cmd *lists.AddToListCommand) (*lists.MembershipResult, error) {
	if s != nil && s.AddToListFunc != nil {
		return s.AddToListFunc(ctx, cmd)
	}
	return nil, missingStub("ListsService.AddToList")
}

func (s *ListsServiceStub) CreateList(ctx context.Context, cmd *lists.CreateListCommand) (*lists.ListResult, error) {
	if s != nil && s.CreateListFunc != nil {
		return s.CreateListFunc(ctx, cmd)
	}
	return nil, missingStub("ListsService.CreateList")
}

func (s *ListsServiceStub) DeleteList(ctx context.Context, cmd *lists.DeleteListCommand) error {
	if s != nil && s.DeleteListFunc != nil {
		return s.DeleteListFunc(ctx, cmd)
	}
	return missingStub("ListsService.DeleteList")
}

func (s *ListsServiceStub) GetList(ctx context.Context, query *lists.GetListQuery) (*storagemodels.List, error) {
	if s != nil && s.GetListFunc != nil {
		return s.GetListFunc(ctx, query)
	}
	return nil, missingStub("ListsService.GetList")
}

func (s *ListsServiceStub) GetListMembers(ctx context.Context, query *lists.GetListMembersQuery) (*lists.MembersResult, error) {
	if s != nil && s.GetListMembersFunc != nil {
		return s.GetListMembersFunc(ctx, query)
	}
	return nil, missingStub("ListsService.GetListMembers")
}

func (s *ListsServiceStub) GetListTimeline(ctx context.Context, query *lists.GetListTimelineQuery) (*lists.TimelineResult, error) {
	if s != nil && s.GetListTimelineFunc != nil {
		return s.GetListTimelineFunc(ctx, query)
	}
	return nil, missingStub("ListsService.GetListTimeline")
}

func (s *ListsServiceStub) ListUserLists(ctx context.Context, query *lists.ListUserListsQuery) (*lists.Result, error) {
	if s != nil && s.ListUserListsFunc != nil {
		return s.ListUserListsFunc(ctx, query)
	}
	return nil, missingStub("ListsService.ListUserLists")
}

func (s *ListsServiceStub) RemoveFromList(ctx context.Context, cmd *lists.RemoveFromListCommand) (*lists.MembershipResult, error) {
	if s != nil && s.RemoveFromListFunc != nil {
		return s.RemoveFromListFunc(ctx, cmd)
	}
	return nil, missingStub("ListsService.RemoveFromList")
}

func (s *ListsServiceStub) UpdateList(ctx context.Context, cmd *lists.UpdateListCommand) (*lists.ListResult, error) {
	if s != nil && s.UpdateListFunc != nil {
		return s.UpdateListFunc(ctx, cmd)
	}
	return nil, missingStub("ListsService.UpdateList")
}

type MediaServiceStub struct {
	GetMediaFunc    func(ctx context.Context, query *media.GetMediaQuery) (*storagemodels.Media, error)
	UpdateMediaFunc func(ctx context.Context, cmd *media.UpdateMediaCommand) (*media.UpdateResult, error)
	UploadMediaFunc func(ctx context.Context, cmd *media.UploadMediaCommand) (*media.Result, error)
}

var _ MediaService = (*MediaServiceStub)(nil)

func (s *MediaServiceStub) GetMedia(ctx context.Context, query *media.GetMediaQuery) (*storagemodels.Media, error) {
	if s != nil && s.GetMediaFunc != nil {
		return s.GetMediaFunc(ctx, query)
	}
	return nil, missingStub("MediaService.GetMedia")
}

func (s *MediaServiceStub) UpdateMedia(ctx context.Context, cmd *media.UpdateMediaCommand) (*media.UpdateResult, error) {
	if s != nil && s.UpdateMediaFunc != nil {
		return s.UpdateMediaFunc(ctx, cmd)
	}
	return nil, missingStub("MediaService.UpdateMedia")
}

func (s *MediaServiceStub) UploadMedia(ctx context.Context, cmd *media.UploadMediaCommand) (*media.Result, error) {
	if s != nil && s.UploadMediaFunc != nil {
		return s.UploadMediaFunc(ctx, cmd)
	}
	return nil, missingStub("MediaService.UploadMedia")
}

type NotesServiceStub struct {
	BookmarkNoteFunc              func(ctx context.Context, cmd *notes.BookmarkNoteCommand) (*notes.BookmarkResult, error)
	CountNotesByAuthorFunc        func(ctx context.Context, authorID string) (int64, error)
	CountRepliesFunc              func(ctx context.Context, statusID string) (int, error)
	CreateNoteFunc                func(ctx context.Context, cmd *notes.CreateNoteCommand) (*notes.NoteResult, error)
	CreateCommunityNoteFunc       func(ctx context.Context, cmd *notes.CreateCommunityNoteCommand) (*notes.CreateCommunityNoteResult, error)
	CreateCommunityNoteVoteFunc   func(ctx context.Context, cmd *notes.CreateCommunityNoteVoteCommand) (*notes.CreateCommunityNoteVoteResult, error)
	DeleteNoteFunc                func(ctx context.Context, cmd *notes.DeleteNoteCommand) error
	GetBookmarksFunc              func(ctx context.Context, query *notes.GetBookmarksQuery) (*notes.Result, error)
	GetBoostCountFunc             func(ctx context.Context, statusID string) (int64, error)
	GetCommunityNoteFunc          func(ctx context.Context, query *notes.GetCommunityNoteQuery) (*notes.GetCommunityNoteResult, error)
	GetCommunityNotesByAuthorFunc func(ctx context.Context, query *notes.GetCommunityNotesByAuthorQuery) (*notes.GetCommunityNotesByAuthorResult, error)
	GetFavoritedNotesFunc         func(ctx context.Context, query *notes.ListNotesQuery) (*notes.Result, error)
	GetLikeCountFunc              func(ctx context.Context, statusID string) (int64, error)
	GetLikersFunc                 func(ctx context.Context, query *notes.GetLikersQuery) (*notes.UsersResult, error)
	GetNoteFunc                   func(ctx context.Context, statusID string) (*storagemodels.Status, error)
	GetRebloggersFunc             func(ctx context.Context, query *notes.GetRebloggersQuery) (*notes.UsersResult, error)
	GetSearchSuggestionsFunc      func(ctx context.Context, query *notes.GetSearchSuggestionsQuery) (*notes.GetSearchSuggestionsResult, error)
	GetUpdateHistoryFunc          func(ctx context.Context, query *notes.GetUpdateHistoryQuery) (*notes.GetUpdateHistoryResult, error)
	GetUserTimelineFunc           func(ctx context.Context, actorID string, opts interfaces.PaginationOptions) (*notes.GetUserTimelineResult, error)
	GetVisibleCommunityNotesFunc  func(ctx context.Context, query *notes.GetVisibleCommunityNotesQuery) (*notes.GetVisibleCommunityNotesResult, error)
	HasLikedFunc                  func(ctx context.Context, userID, statusID string) (bool, error)
	HasRebloggedFunc              func(ctx context.Context, userID, statusID string) (bool, error)
	IsBookmarkedFunc              func(ctx context.Context, userID, statusID string) (bool, error)
	LikeNoteFunc                  func(ctx context.Context, cmd *notes.LikeNoteCommand) (*notes.LikeResult, error)
	ListNotesFunc                 func(ctx context.Context, query *notes.ListNotesQuery) (*notes.Result, error)
	ReblogNoteFunc                func(ctx context.Context, cmd *notes.ReblogNoteCommand) (*notes.LikeResult, error)
	UnlikeNoteFunc                func(ctx context.Context, cmd *notes.UnlikeNoteCommand) (*notes.LikeResult, error)
	UnbookmarkNoteFunc            func(ctx context.Context, cmd *notes.UnbookmarkNoteCommand) (*notes.BookmarkResult, error)
	UnreblogNoteFunc              func(ctx context.Context, cmd *notes.UnreblogNoteCommand) (*notes.LikeResult, error)
}

var _ NotesService = (*NotesServiceStub)(nil)

func (s *NotesServiceStub) BookmarkNote(ctx context.Context, cmd *notes.BookmarkNoteCommand) (*notes.BookmarkResult, error) {
	if s != nil && s.BookmarkNoteFunc != nil {
		return s.BookmarkNoteFunc(ctx, cmd)
	}
	return nil, missingStub("NotesService.BookmarkNote")
}

func (s *NotesServiceStub) CountNotesByAuthor(ctx context.Context, authorID string) (int64, error) {
	if s != nil && s.CountNotesByAuthorFunc != nil {
		return s.CountNotesByAuthorFunc(ctx, authorID)
	}
	return 0, missingStub("NotesService.CountNotesByAuthor")
}

func (s *NotesServiceStub) CountReplies(ctx context.Context, statusID string) (int, error) {
	if s != nil && s.CountRepliesFunc != nil {
		return s.CountRepliesFunc(ctx, statusID)
	}
	return 0, missingStub("NotesService.CountReplies")
}

func (s *NotesServiceStub) CreateNote(ctx context.Context, cmd *notes.CreateNoteCommand) (*notes.NoteResult, error) {
	if s != nil && s.CreateNoteFunc != nil {
		return s.CreateNoteFunc(ctx, cmd)
	}
	return nil, missingStub("NotesService.CreateNote")
}

func (s *NotesServiceStub) CreateCommunityNote(ctx context.Context, cmd *notes.CreateCommunityNoteCommand) (*notes.CreateCommunityNoteResult, error) {
	if s != nil && s.CreateCommunityNoteFunc != nil {
		return s.CreateCommunityNoteFunc(ctx, cmd)
	}
	return nil, missingStub("NotesService.CreateCommunityNote")
}

func (s *NotesServiceStub) CreateCommunityNoteVote(ctx context.Context, cmd *notes.CreateCommunityNoteVoteCommand) (*notes.CreateCommunityNoteVoteResult, error) {
	if s != nil && s.CreateCommunityNoteVoteFunc != nil {
		return s.CreateCommunityNoteVoteFunc(ctx, cmd)
	}
	return nil, missingStub("NotesService.CreateCommunityNoteVote")
}

func (s *NotesServiceStub) DeleteNote(ctx context.Context, cmd *notes.DeleteNoteCommand) error {
	if s != nil && s.DeleteNoteFunc != nil {
		return s.DeleteNoteFunc(ctx, cmd)
	}
	return missingStub("NotesService.DeleteNote")
}

func (s *NotesServiceStub) GetBookmarks(ctx context.Context, query *notes.GetBookmarksQuery) (*notes.Result, error) {
	if s != nil && s.GetBookmarksFunc != nil {
		return s.GetBookmarksFunc(ctx, query)
	}
	return nil, missingStub("NotesService.GetBookmarks")
}

func (s *NotesServiceStub) GetBoostCount(ctx context.Context, statusID string) (int64, error) {
	if s != nil && s.GetBoostCountFunc != nil {
		return s.GetBoostCountFunc(ctx, statusID)
	}
	return 0, missingStub("NotesService.GetBoostCount")
}

func (s *NotesServiceStub) GetCommunityNote(ctx context.Context, query *notes.GetCommunityNoteQuery) (*notes.GetCommunityNoteResult, error) {
	if s != nil && s.GetCommunityNoteFunc != nil {
		return s.GetCommunityNoteFunc(ctx, query)
	}
	return nil, missingStub("NotesService.GetCommunityNote")
}

func (s *NotesServiceStub) GetCommunityNotesByAuthor(ctx context.Context, query *notes.GetCommunityNotesByAuthorQuery) (*notes.GetCommunityNotesByAuthorResult, error) {
	if s != nil && s.GetCommunityNotesByAuthorFunc != nil {
		return s.GetCommunityNotesByAuthorFunc(ctx, query)
	}
	return nil, missingStub("NotesService.GetCommunityNotesByAuthor")
}

func (s *NotesServiceStub) GetFavoritedNotes(ctx context.Context, query *notes.ListNotesQuery) (*notes.Result, error) {
	if s != nil && s.GetFavoritedNotesFunc != nil {
		return s.GetFavoritedNotesFunc(ctx, query)
	}
	return nil, missingStub("NotesService.GetFavoritedNotes")
}

func (s *NotesServiceStub) GetLikeCount(ctx context.Context, statusID string) (int64, error) {
	if s != nil && s.GetLikeCountFunc != nil {
		return s.GetLikeCountFunc(ctx, statusID)
	}
	return 0, missingStub("NotesService.GetLikeCount")
}

func (s *NotesServiceStub) GetLikers(ctx context.Context, query *notes.GetLikersQuery) (*notes.UsersResult, error) {
	if s != nil && s.GetLikersFunc != nil {
		return s.GetLikersFunc(ctx, query)
	}
	return nil, missingStub("NotesService.GetLikers")
}

func (s *NotesServiceStub) GetNote(ctx context.Context, statusID string) (*storagemodels.Status, error) {
	if s != nil && s.GetNoteFunc != nil {
		return s.GetNoteFunc(ctx, statusID)
	}
	return nil, missingStub("NotesService.GetNote")
}

func (s *NotesServiceStub) GetRebloggers(ctx context.Context, query *notes.GetRebloggersQuery) (*notes.UsersResult, error) {
	if s != nil && s.GetRebloggersFunc != nil {
		return s.GetRebloggersFunc(ctx, query)
	}
	return nil, missingStub("NotesService.GetRebloggers")
}

func (s *NotesServiceStub) GetSearchSuggestions(ctx context.Context, query *notes.GetSearchSuggestionsQuery) (*notes.GetSearchSuggestionsResult, error) {
	if s != nil && s.GetSearchSuggestionsFunc != nil {
		return s.GetSearchSuggestionsFunc(ctx, query)
	}
	return nil, missingStub("NotesService.GetSearchSuggestions")
}

func (s *NotesServiceStub) GetUpdateHistory(ctx context.Context, query *notes.GetUpdateHistoryQuery) (*notes.GetUpdateHistoryResult, error) {
	if s != nil && s.GetUpdateHistoryFunc != nil {
		return s.GetUpdateHistoryFunc(ctx, query)
	}
	return nil, missingStub("NotesService.GetUpdateHistory")
}

func (s *NotesServiceStub) GetUserTimeline(ctx context.Context, actorID string, opts interfaces.PaginationOptions) (*notes.GetUserTimelineResult, error) {
	if s != nil && s.GetUserTimelineFunc != nil {
		return s.GetUserTimelineFunc(ctx, actorID, opts)
	}
	return nil, missingStub("NotesService.GetUserTimeline")
}

func (s *NotesServiceStub) GetVisibleCommunityNotes(ctx context.Context, query *notes.GetVisibleCommunityNotesQuery) (*notes.GetVisibleCommunityNotesResult, error) {
	if s != nil && s.GetVisibleCommunityNotesFunc != nil {
		return s.GetVisibleCommunityNotesFunc(ctx, query)
	}
	return nil, missingStub("NotesService.GetVisibleCommunityNotes")
}

func (s *NotesServiceStub) HasLiked(ctx context.Context, userID, statusID string) (bool, error) {
	if s != nil && s.HasLikedFunc != nil {
		return s.HasLikedFunc(ctx, userID, statusID)
	}
	return false, missingStub("NotesService.HasLiked")
}

func (s *NotesServiceStub) HasReblogged(ctx context.Context, userID, statusID string) (bool, error) {
	if s != nil && s.HasRebloggedFunc != nil {
		return s.HasRebloggedFunc(ctx, userID, statusID)
	}
	return false, missingStub("NotesService.HasReblogged")
}

func (s *NotesServiceStub) IsBookmarked(ctx context.Context, userID, statusID string) (bool, error) {
	if s != nil && s.IsBookmarkedFunc != nil {
		return s.IsBookmarkedFunc(ctx, userID, statusID)
	}
	return false, missingStub("NotesService.IsBookmarked")
}

func (s *NotesServiceStub) LikeNote(ctx context.Context, cmd *notes.LikeNoteCommand) (*notes.LikeResult, error) {
	if s != nil && s.LikeNoteFunc != nil {
		return s.LikeNoteFunc(ctx, cmd)
	}
	return nil, missingStub("NotesService.LikeNote")
}

func (s *NotesServiceStub) ListNotes(ctx context.Context, query *notes.ListNotesQuery) (*notes.Result, error) {
	if s != nil && s.ListNotesFunc != nil {
		return s.ListNotesFunc(ctx, query)
	}
	return nil, missingStub("NotesService.ListNotes")
}

func (s *NotesServiceStub) ReblogNote(ctx context.Context, cmd *notes.ReblogNoteCommand) (*notes.LikeResult, error) {
	if s != nil && s.ReblogNoteFunc != nil {
		return s.ReblogNoteFunc(ctx, cmd)
	}
	return nil, missingStub("NotesService.ReblogNote")
}

func (s *NotesServiceStub) UnlikeNote(ctx context.Context, cmd *notes.UnlikeNoteCommand) (*notes.LikeResult, error) {
	if s != nil && s.UnlikeNoteFunc != nil {
		return s.UnlikeNoteFunc(ctx, cmd)
	}
	return nil, missingStub("NotesService.UnlikeNote")
}

func (s *NotesServiceStub) UnbookmarkNote(ctx context.Context, cmd *notes.UnbookmarkNoteCommand) (*notes.BookmarkResult, error) {
	if s != nil && s.UnbookmarkNoteFunc != nil {
		return s.UnbookmarkNoteFunc(ctx, cmd)
	}
	return nil, missingStub("NotesService.UnbookmarkNote")
}

func (s *NotesServiceStub) UnreblogNote(ctx context.Context, cmd *notes.UnreblogNoteCommand) (*notes.LikeResult, error) {
	if s != nil && s.UnreblogNoteFunc != nil {
		return s.UnreblogNoteFunc(ctx, cmd)
	}
	return nil, missingStub("NotesService.UnreblogNote")
}

type NotificationsServiceStub struct {
	ClearNotificationsFunc func(ctx context.Context, cmd *notifications.ClearCommand) (*notifications.ClearResult, error)
	CreateNotificationFunc func(ctx context.Context, cmd *notifications.CreateNotificationCommand) (*notifications.NotificationResult, error)
	ListNotificationsFunc  func(ctx context.Context, query *notifications.ListNotificationsQuery) (*notifications.NotificationListResult, error)
	MarkAsReadFunc         func(ctx context.Context, cmd *notifications.MarkAsReadCommand) (*notifications.NotificationResult, error)
}

var _ NotificationsService = (*NotificationsServiceStub)(nil)

func (s *NotificationsServiceStub) ClearNotifications(ctx context.Context, cmd *notifications.ClearCommand) (*notifications.ClearResult, error) {
	if s != nil && s.ClearNotificationsFunc != nil {
		return s.ClearNotificationsFunc(ctx, cmd)
	}
	return nil, missingStub("NotificationsService.ClearNotifications")
}

func (s *NotificationsServiceStub) CreateNotification(ctx context.Context, cmd *notifications.CreateNotificationCommand) (*notifications.NotificationResult, error) {
	if s != nil && s.CreateNotificationFunc != nil {
		return s.CreateNotificationFunc(ctx, cmd)
	}
	return nil, missingStub("NotificationsService.CreateNotification")
}

func (s *NotificationsServiceStub) ListNotifications(ctx context.Context, query *notifications.ListNotificationsQuery) (*notifications.NotificationListResult, error) {
	if s != nil && s.ListNotificationsFunc != nil {
		return s.ListNotificationsFunc(ctx, query)
	}
	return nil, missingStub("NotificationsService.ListNotifications")
}

func (s *NotificationsServiceStub) MarkAsRead(ctx context.Context, cmd *notifications.MarkAsReadCommand) (*notifications.NotificationResult, error) {
	if s != nil && s.MarkAsReadFunc != nil {
		return s.MarkAsReadFunc(ctx, cmd)
	}
	return nil, missingStub("NotificationsService.MarkAsRead")
}

type RelationshipsServiceStub struct {
	AcceptFollowRequestFunc      func(ctx context.Context, cmd *relationships.AcceptFollowRequestCommand) (*relationships.RelationshipResult, error)
	AddDomainBlockFunc           func(ctx context.Context, cmd *relationships.AddDomainBlockCommand) error
	BlockFunc                    func(ctx context.Context, cmd *relationships.BlockCommand) (*relationships.RelationshipResult, error)
	CountFollowersFunc           func(ctx context.Context, username string) (int64, error)
	CountFollowingFunc           func(ctx context.Context, username string) (int64, error)
	FollowFunc                   func(ctx context.Context, cmd *relationships.FollowCommand) (*relationships.FollowResult, error)
	GetBlockedUsersFunc          func(ctx context.Context, query *relationships.GetBlockedUsersQuery) (*relationships.BlockedUsersResult, error)
	GetDomainBlocksFunc          func(ctx context.Context, query *relationships.GetDomainBlocksQuery) (*relationships.DomainBlocksResult, error)
	GetFollowersFunc             func(ctx context.Context, username string, limit int, cursor string) ([]*storage.Account, string, error)
	GetFollowingFunc             func(ctx context.Context, username string, limit int, cursor string) ([]*storage.Account, string, error)
	GetMutedUsersFunc            func(ctx context.Context, query *relationships.GetMutedUsersQuery) (*relationships.MutedUsersResult, error)
	GetPendingFollowRequestsFunc func(ctx context.Context, query *relationships.GetFollowRequestsQuery) (*relationships.FollowRequestsResult, error)
	GetRelationshipFunc          func(ctx context.Context, requesterID, targetID string) (*relationships.RelationshipData, error)
	GetRelationshipsFunc         func(ctx context.Context, query *relationships.GetRelationshipsQuery) (*relationships.Result, error)
	IsMutedFunc                  func(ctx context.Context, muterID, mutedID string) (bool, error)
	MuteFunc                     func(ctx context.Context, cmd *relationships.MuteCommand) (*relationships.RelationshipResult, error)
	RejectFollowRequestFunc      func(ctx context.Context, cmd *relationships.RejectFollowRequestCommand) (*relationships.RelationshipResult, error)
	RemoveDomainBlockFunc        func(ctx context.Context, cmd *relationships.RemoveDomainBlockCommand) error
	UnblockFunc                  func(ctx context.Context, cmd *relationships.UnblockCommand) (*relationships.RelationshipResult, error)
	UnfollowFunc                 func(ctx context.Context, cmd *relationships.UnfollowCommand) (*relationships.RelationshipResult, error)
	UnmuteFunc                   func(ctx context.Context, cmd *relationships.UnmuteCommand) (*relationships.RelationshipResult, error)
}

var _ RelationshipsService = (*RelationshipsServiceStub)(nil)

func (s *RelationshipsServiceStub) AcceptFollowRequest(ctx context.Context, cmd *relationships.AcceptFollowRequestCommand) (*relationships.RelationshipResult, error) {
	if s != nil && s.AcceptFollowRequestFunc != nil {
		return s.AcceptFollowRequestFunc(ctx, cmd)
	}
	return nil, missingStub("RelationshipsService.AcceptFollowRequest")
}

func (s *RelationshipsServiceStub) AddDomainBlock(ctx context.Context, cmd *relationships.AddDomainBlockCommand) error {
	if s != nil && s.AddDomainBlockFunc != nil {
		return s.AddDomainBlockFunc(ctx, cmd)
	}
	return missingStub("RelationshipsService.AddDomainBlock")
}

func (s *RelationshipsServiceStub) Block(ctx context.Context, cmd *relationships.BlockCommand) (*relationships.RelationshipResult, error) {
	if s != nil && s.BlockFunc != nil {
		return s.BlockFunc(ctx, cmd)
	}
	return nil, missingStub("RelationshipsService.Block")
}

func (s *RelationshipsServiceStub) CountFollowers(ctx context.Context, username string) (int64, error) {
	if s != nil && s.CountFollowersFunc != nil {
		return s.CountFollowersFunc(ctx, username)
	}
	return 0, missingStub("RelationshipsService.CountFollowers")
}

func (s *RelationshipsServiceStub) CountFollowing(ctx context.Context, username string) (int64, error) {
	if s != nil && s.CountFollowingFunc != nil {
		return s.CountFollowingFunc(ctx, username)
	}
	return 0, missingStub("RelationshipsService.CountFollowing")
}

func (s *RelationshipsServiceStub) Follow(ctx context.Context, cmd *relationships.FollowCommand) (*relationships.FollowResult, error) {
	if s != nil && s.FollowFunc != nil {
		return s.FollowFunc(ctx, cmd)
	}
	return nil, missingStub("RelationshipsService.Follow")
}

func (s *RelationshipsServiceStub) GetBlockedUsers(ctx context.Context, query *relationships.GetBlockedUsersQuery) (*relationships.BlockedUsersResult, error) {
	if s != nil && s.GetBlockedUsersFunc != nil {
		return s.GetBlockedUsersFunc(ctx, query)
	}
	return nil, missingStub("RelationshipsService.GetBlockedUsers")
}

func (s *RelationshipsServiceStub) GetDomainBlocks(ctx context.Context, query *relationships.GetDomainBlocksQuery) (*relationships.DomainBlocksResult, error) {
	if s != nil && s.GetDomainBlocksFunc != nil {
		return s.GetDomainBlocksFunc(ctx, query)
	}
	return nil, missingStub("RelationshipsService.GetDomainBlocks")
}

func (s *RelationshipsServiceStub) GetFollowers(ctx context.Context, username string, limit int, cursor string) ([]*storage.Account, string, error) {
	if s != nil && s.GetFollowersFunc != nil {
		return s.GetFollowersFunc(ctx, username, limit, cursor)
	}
	return nil, "", missingStub("RelationshipsService.GetFollowers")
}

func (s *RelationshipsServiceStub) GetFollowing(ctx context.Context, username string, limit int, cursor string) ([]*storage.Account, string, error) {
	if s != nil && s.GetFollowingFunc != nil {
		return s.GetFollowingFunc(ctx, username, limit, cursor)
	}
	return nil, "", missingStub("RelationshipsService.GetFollowing")
}

func (s *RelationshipsServiceStub) GetMutedUsers(ctx context.Context, query *relationships.GetMutedUsersQuery) (*relationships.MutedUsersResult, error) {
	if s != nil && s.GetMutedUsersFunc != nil {
		return s.GetMutedUsersFunc(ctx, query)
	}
	return nil, missingStub("RelationshipsService.GetMutedUsers")
}

func (s *RelationshipsServiceStub) GetPendingFollowRequests(ctx context.Context, query *relationships.GetFollowRequestsQuery) (*relationships.FollowRequestsResult, error) {
	if s != nil && s.GetPendingFollowRequestsFunc != nil {
		return s.GetPendingFollowRequestsFunc(ctx, query)
	}
	return nil, missingStub("RelationshipsService.GetPendingFollowRequests")
}

func (s *RelationshipsServiceStub) GetRelationship(ctx context.Context, requesterID, targetID string) (*relationships.RelationshipData, error) {
	if s != nil && s.GetRelationshipFunc != nil {
		return s.GetRelationshipFunc(ctx, requesterID, targetID)
	}
	return nil, missingStub("RelationshipsService.GetRelationship")
}

func (s *RelationshipsServiceStub) GetRelationships(ctx context.Context, query *relationships.GetRelationshipsQuery) (*relationships.Result, error) {
	if s != nil && s.GetRelationshipsFunc != nil {
		return s.GetRelationshipsFunc(ctx, query)
	}
	return nil, missingStub("RelationshipsService.GetRelationships")
}

func (s *RelationshipsServiceStub) IsMuted(ctx context.Context, muterID, mutedID string) (bool, error) {
	if s != nil && s.IsMutedFunc != nil {
		return s.IsMutedFunc(ctx, muterID, mutedID)
	}
	return false, missingStub("RelationshipsService.IsMuted")
}

func (s *RelationshipsServiceStub) Mute(ctx context.Context, cmd *relationships.MuteCommand) (*relationships.RelationshipResult, error) {
	if s != nil && s.MuteFunc != nil {
		return s.MuteFunc(ctx, cmd)
	}
	return nil, missingStub("RelationshipsService.Mute")
}

func (s *RelationshipsServiceStub) RejectFollowRequest(ctx context.Context, cmd *relationships.RejectFollowRequestCommand) (*relationships.RelationshipResult, error) {
	if s != nil && s.RejectFollowRequestFunc != nil {
		return s.RejectFollowRequestFunc(ctx, cmd)
	}
	return nil, missingStub("RelationshipsService.RejectFollowRequest")
}

func (s *RelationshipsServiceStub) RemoveDomainBlock(ctx context.Context, cmd *relationships.RemoveDomainBlockCommand) error {
	if s != nil && s.RemoveDomainBlockFunc != nil {
		return s.RemoveDomainBlockFunc(ctx, cmd)
	}
	return missingStub("RelationshipsService.RemoveDomainBlock")
}

func (s *RelationshipsServiceStub) Unblock(ctx context.Context, cmd *relationships.UnblockCommand) (*relationships.RelationshipResult, error) {
	if s != nil && s.UnblockFunc != nil {
		return s.UnblockFunc(ctx, cmd)
	}
	return nil, missingStub("RelationshipsService.Unblock")
}

func (s *RelationshipsServiceStub) Unfollow(ctx context.Context, cmd *relationships.UnfollowCommand) (*relationships.RelationshipResult, error) {
	if s != nil && s.UnfollowFunc != nil {
		return s.UnfollowFunc(ctx, cmd)
	}
	return nil, missingStub("RelationshipsService.Unfollow")
}

func (s *RelationshipsServiceStub) Unmute(ctx context.Context, cmd *relationships.UnmuteCommand) (*relationships.RelationshipResult, error) {
	if s != nil && s.UnmuteFunc != nil {
		return s.UnmuteFunc(ctx, cmd)
	}
	return nil, missingStub("RelationshipsService.Unmute")
}

type ScheduledServiceStub struct {
	CreateScheduledStatusFunc        func(ctx context.Context, cmd *scheduled.CreateScheduledStatusCommand) (*scheduled.StatusResult, error)
	DeleteScheduledStatusFunc        func(ctx context.Context, cmd *scheduled.DeleteScheduledStatusCommand) error
	GetScheduledMediaAttachmentsFunc func(ctx context.Context, scheduledStatusID string) ([]*storagemodels.Media, error)
	GetScheduledStatusFunc           func(ctx context.Context, query *scheduled.GetScheduledStatusQuery) (*scheduled.StatusResult, error)
	ListScheduledStatusesFunc        func(ctx context.Context, query *scheduled.ListScheduledStatusesQuery) (*scheduled.StatusListResult, error)
	UpdateScheduledStatusFunc        func(ctx context.Context, cmd *scheduled.UpdateScheduledStatusCommand) (*scheduled.StatusResult, error)
}

var _ ScheduledService = (*ScheduledServiceStub)(nil)

func (s *ScheduledServiceStub) CreateScheduledStatus(ctx context.Context, cmd *scheduled.CreateScheduledStatusCommand) (*scheduled.StatusResult, error) {
	if s != nil && s.CreateScheduledStatusFunc != nil {
		return s.CreateScheduledStatusFunc(ctx, cmd)
	}
	return nil, missingStub("ScheduledService.CreateScheduledStatus")
}

func (s *ScheduledServiceStub) DeleteScheduledStatus(ctx context.Context, cmd *scheduled.DeleteScheduledStatusCommand) error {
	if s != nil && s.DeleteScheduledStatusFunc != nil {
		return s.DeleteScheduledStatusFunc(ctx, cmd)
	}
	return missingStub("ScheduledService.DeleteScheduledStatus")
}

func (s *ScheduledServiceStub) GetScheduledMediaAttachments(ctx context.Context, scheduledStatusID string) ([]*storagemodels.Media, error) {
	if s != nil && s.GetScheduledMediaAttachmentsFunc != nil {
		return s.GetScheduledMediaAttachmentsFunc(ctx, scheduledStatusID)
	}
	return nil, missingStub("ScheduledService.GetScheduledMediaAttachments")
}

func (s *ScheduledServiceStub) GetScheduledStatus(ctx context.Context, query *scheduled.GetScheduledStatusQuery) (*scheduled.StatusResult, error) {
	if s != nil && s.GetScheduledStatusFunc != nil {
		return s.GetScheduledStatusFunc(ctx, query)
	}
	return nil, missingStub("ScheduledService.GetScheduledStatus")
}

func (s *ScheduledServiceStub) ListScheduledStatuses(ctx context.Context, query *scheduled.ListScheduledStatusesQuery) (*scheduled.StatusListResult, error) {
	if s != nil && s.ListScheduledStatusesFunc != nil {
		return s.ListScheduledStatusesFunc(ctx, query)
	}
	return nil, missingStub("ScheduledService.ListScheduledStatuses")
}

func (s *ScheduledServiceStub) UpdateScheduledStatus(ctx context.Context, cmd *scheduled.UpdateScheduledStatusCommand) (*scheduled.StatusResult, error) {
	if s != nil && s.UpdateScheduledStatusFunc != nil {
		return s.UpdateScheduledStatusFunc(ctx, cmd)
	}
	return nil, missingStub("ScheduledService.UpdateScheduledStatus")
}

type SearchServiceStub struct {
	GetDirectoryFunc     func(ctx context.Context, query *search.DirectoryQuery) (*search.DirectoryResult, error)
	GetSuggestionsFunc   func(ctx context.Context, query *search.SuggestionsQuery) (*search.SuggestionsResult, error)
	RemoveSuggestionFunc func(ctx context.Context, cmd *search.RemoveSuggestionCommand) error
}

var _ SearchService = (*SearchServiceStub)(nil)

func (s *SearchServiceStub) GetDirectory(ctx context.Context, query *search.DirectoryQuery) (*search.DirectoryResult, error) {
	if s != nil && s.GetDirectoryFunc != nil {
		return s.GetDirectoryFunc(ctx, query)
	}
	return nil, missingStub("SearchService.GetDirectory")
}

func (s *SearchServiceStub) GetSuggestions(ctx context.Context, query *search.SuggestionsQuery) (*search.SuggestionsResult, error) {
	if s != nil && s.GetSuggestionsFunc != nil {
		return s.GetSuggestionsFunc(ctx, query)
	}
	return nil, missingStub("SearchService.GetSuggestions")
}

func (s *SearchServiceStub) RemoveSuggestion(ctx context.Context, cmd *search.RemoveSuggestionCommand) error {
	if s != nil && s.RemoveSuggestionFunc != nil {
		return s.RemoveSuggestionFunc(ctx, cmd)
	}
	return missingStub("SearchService.RemoveSuggestion")
}

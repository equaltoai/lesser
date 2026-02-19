package conversations

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/streaming"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

// Mock implementations for testing

type mockConversationRepository struct {
	mock.Mock
}

func (m *mockConversationRepository) CreateConversation(ctx context.Context, conversation *models.Conversation, participants []string) error {
	args := m.Called(ctx, conversation, participants)
	return args.Error(0)
}

func (m *mockConversationRepository) GetConversation(ctx context.Context, conversationID string) (*models.Conversation, error) {
	args := m.Called(ctx, conversationID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Conversation), args.Error(1)
}

func (m *mockConversationRepository) UpdateConversation(ctx context.Context, conversation *models.Conversation) error {
	args := m.Called(ctx, conversation)
	return args.Error(0)
}

func (m *mockConversationRepository) DeleteConversation(ctx context.Context, conversationID string) error {
	args := m.Called(ctx, conversationID)
	return args.Error(0)
}

func (m *mockConversationRepository) GetUserConversations(ctx context.Context, userID string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Conversation], error) {
	args := m.Called(ctx, userID, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*interfaces.PaginatedResult[*models.Conversation]), args.Error(1)
}

func (m *mockConversationRepository) GetUserConversationsByRequestState(ctx context.Context, userID string, requestState models.DmRequestState, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Conversation], error) {
	args := m.Called(ctx, userID, requestState, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*interfaces.PaginatedResult[*models.Conversation]), args.Error(1)
}

func (m *mockConversationRepository) GetConversationByParticipants(ctx context.Context, participants []string) (*models.Conversation, error) {
	args := m.Called(ctx, participants)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Conversation), args.Error(1)
}

func (m *mockConversationRepository) AddParticipant(ctx context.Context, conversationID, participantID string) error {
	args := m.Called(ctx, conversationID, participantID)
	return args.Error(0)
}

func (m *mockConversationRepository) RemoveParticipant(ctx context.Context, conversationID, participantID string) error {
	args := m.Called(ctx, conversationID, participantID)
	return args.Error(0)
}

func (m *mockConversationRepository) GetConversationParticipants(ctx context.Context, conversationID string) ([]string, error) {
	args := m.Called(ctx, conversationID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]string), args.Error(1)
}

func (m *mockConversationRepository) GetConversationParticipantRecord(ctx context.Context, conversationID, participantID string) (*models.ConversationParticipantRecord, error) {
	args := m.Called(ctx, conversationID, participantID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.ConversationParticipantRecord), args.Error(1)
}

func (m *mockConversationRepository) UpdateConversationParticipantRecord(ctx context.Context, record *models.ConversationParticipantRecord) error {
	args := m.Called(ctx, record)
	return args.Error(0)
}

func (m *mockConversationRepository) MarkConversationRead(ctx context.Context, conversationID, userID string) error {
	args := m.Called(ctx, conversationID, userID)
	return args.Error(0)
}

func (m *mockConversationRepository) MarkConversationUnread(ctx context.Context, conversationID, userID string) error {
	args := m.Called(ctx, conversationID, userID)
	return args.Error(0)
}

func (m *mockConversationRepository) GetUnreadConversations(ctx context.Context, userID string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Conversation], error) {
	args := m.Called(ctx, userID, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*interfaces.PaginatedResult[*models.Conversation]), args.Error(1)
}

func (m *mockConversationRepository) SearchConversations(ctx context.Context, userID, query string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Conversation], error) {
	args := m.Called(ctx, userID, query, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*interfaces.PaginatedResult[*models.Conversation]), args.Error(1)
}

func (m *mockConversationRepository) CreateConversationMute(ctx context.Context, mute *storage.ConversationMute) error {
	args := m.Called(ctx, mute)
	return args.Error(0)
}

func (m *mockConversationRepository) DeleteConversationMute(ctx context.Context, username, conversationID string) error {
	args := m.Called(ctx, username, conversationID)
	return args.Error(0)
}

func (m *mockConversationRepository) GetUnreadConversationCount(ctx context.Context, username string) (int, error) {
	args := m.Called(ctx, username)
	return args.Int(0), args.Error(1)
}

func (m *mockConversationRepository) AddStatusToConversation(ctx context.Context, conversationID, statusID, senderUsername string) error {
	args := m.Called(ctx, conversationID, statusID, senderUsername)
	return args.Error(0)
}

func (m *mockConversationRepository) GetConversationStatuses(ctx context.Context, conversationID string, limit int, cursor string) ([]*storage.ConversationStatus, string, error) {
	args := m.Called(ctx, conversationID, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*storage.ConversationStatus), args.String(1), args.Error(2)
}

func (m *mockConversationRepository) RemoveStatusFromConversation(ctx context.Context, conversationID, statusID string) error {
	args := m.Called(ctx, conversationID, statusID)
	return args.Error(0)
}

func (m *mockConversationRepository) MarkStatusRead(ctx context.Context, conversationID, statusID, username string) error {
	args := m.Called(ctx, conversationID, statusID, username)
	return args.Error(0)
}

func (m *mockConversationRepository) GetUnreadStatusCount(ctx context.Context, conversationID, username string) (int, error) {
	args := m.Called(ctx, conversationID, username)
	return args.Int(0), args.Error(1)
}

func (m *mockConversationRepository) UpdateConversationLastStatus(ctx context.Context, id, lastStatusID string) error {
	args := m.Called(ctx, id, lastStatusID)
	return args.Error(0)
}

func (m *mockConversationRepository) LeaveConversation(ctx context.Context, conversationID, username string) error {
	args := m.Called(ctx, conversationID, username)
	return args.Error(0)
}

func (m *mockConversationRepository) IsConversationMuted(ctx context.Context, username, conversationID string) (bool, error) {
	args := m.Called(ctx, username, conversationID)
	return args.Bool(0), args.Error(1)
}

func (m *mockConversationRepository) GetMutedConversations(ctx context.Context, username string) ([]string, error) {
	args := m.Called(ctx, username)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]string), args.Error(1)
}

type mockNoteRepository struct {
	mock.Mock
}

type mockDirectMessageTombstoneRepository struct {
	mock.Mock
}

func (m *mockDirectMessageTombstoneRepository) CreateTombstone(ctx context.Context, viewerUsername, statusID string) error {
	args := m.Called(ctx, viewerUsername, statusID)
	return args.Error(0)
}

func (m *mockDirectMessageTombstoneRepository) TombstonesByStatusID(ctx context.Context, viewerUsername string, statusIDs []string) (map[string]bool, error) {
	args := m.Called(ctx, viewerUsername, statusIDs)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(map[string]bool), args.Error(1)
}

func (m *mockNoteRepository) CreateStatus(ctx context.Context, status *models.Status) error {
	args := m.Called(ctx, status)
	return args.Error(0)
}

func (m *mockNoteRepository) CreateBoostStatus(ctx context.Context, status *models.Status) error {
	// Reuse CreateStatus expectations when needed; boosts aren't exercised in these tests.
	return m.CreateStatus(ctx, status)
}

func (m *mockNoteRepository) GetStatus(ctx context.Context, statusID string) (*models.Status, error) {
	args := m.Called(ctx, statusID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Status), args.Error(1)
}

func (m *mockNoteRepository) GetStatusByURL(ctx context.Context, url string) (*models.Status, error) {
	args := m.Called(ctx, url)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Status), args.Error(1)
}

func (m *mockNoteRepository) UpdateStatus(ctx context.Context, status *models.Status) error {
	args := m.Called(ctx, status)
	return args.Error(0)
}

func (m *mockNoteRepository) DeleteStatus(ctx context.Context, statusID string) error {
	args := m.Called(ctx, statusID)
	return args.Error(0)
}

func (m *mockNoteRepository) GetPublicTimeline(ctx context.Context, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Status], error) {
	args := m.Called(ctx, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*interfaces.PaginatedResult[*models.Status]), args.Error(1)
}

func (m *mockNoteRepository) GetHomeTimeline(ctx context.Context, userID string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Status], error) {
	args := m.Called(ctx, userID, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*interfaces.PaginatedResult[*models.Status]), args.Error(1)
}

func (m *mockNoteRepository) GetUserTimeline(ctx context.Context, userID string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Status], error) {
	args := m.Called(ctx, userID, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*interfaces.PaginatedResult[*models.Status]), args.Error(1)
}

func (m *mockNoteRepository) GetConversationThread(ctx context.Context, conversationID string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Status], error) {
	args := m.Called(ctx, conversationID, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*interfaces.PaginatedResult[*models.Status]), args.Error(1)
}

func (m *mockNoteRepository) GetConversationThreadReverse(ctx context.Context, conversationID string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Status], error) {
	args := m.Called(ctx, conversationID, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*interfaces.PaginatedResult[*models.Status]), args.Error(1)
}

func (m *mockNoteRepository) GetReplies(ctx context.Context, parentStatusID string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Status], error) {
	args := m.Called(ctx, parentStatusID, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*interfaces.PaginatedResult[*models.Status]), args.Error(1)
}

func (m *mockNoteRepository) SearchStatuses(ctx context.Context, query string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Status], error) {
	args := m.Called(ctx, query, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*interfaces.PaginatedResult[*models.Status]), args.Error(1)
}

func (m *mockNoteRepository) GetStatusesByHashtag(ctx context.Context, hashtag string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Status], error) {
	args := m.Called(ctx, hashtag, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*interfaces.PaginatedResult[*models.Status]), args.Error(1)
}

func (m *mockNoteRepository) GetTrendingStatuses(ctx context.Context, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Status], error) {
	args := m.Called(ctx, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*interfaces.PaginatedResult[*models.Status]), args.Error(1)
}

func (m *mockNoteRepository) LikeStatus(ctx context.Context, userID, statusID string) error {
	args := m.Called(ctx, userID, statusID)
	return args.Error(0)
}

func (m *mockNoteRepository) UnlikeStatus(ctx context.Context, userID, statusID string) error {
	args := m.Called(ctx, userID, statusID)
	return args.Error(0)
}

func (m *mockNoteRepository) ReblogStatus(ctx context.Context, userID, statusID string, reblogStatusID string) error {
	args := m.Called(ctx, userID, statusID, reblogStatusID)
	return args.Error(0)
}

func (m *mockNoteRepository) UnreblogStatus(ctx context.Context, userID, statusID string) error {
	args := m.Called(ctx, userID, statusID)
	return args.Error(0)
}

func (m *mockNoteRepository) BookmarkStatus(ctx context.Context, userID, statusID string) error {
	args := m.Called(ctx, userID, statusID)
	return args.Error(0)
}

func (m *mockNoteRepository) UnbookmarkStatus(ctx context.Context, userID, statusID string) error {
	args := m.Called(ctx, userID, statusID)
	return args.Error(0)
}

func (m *mockNoteRepository) FlagStatus(ctx context.Context, statusID, reason string, reportedBy string) error {
	args := m.Called(ctx, statusID, reason, reportedBy)
	return args.Error(0)
}

func (m *mockNoteRepository) UnflagStatus(ctx context.Context, statusID string) error {
	args := m.Called(ctx, statusID)
	return args.Error(0)
}

func (m *mockNoteRepository) GetFlaggedStatuses(ctx context.Context, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Status], error) {
	args := m.Called(ctx, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*interfaces.PaginatedResult[*models.Status]), args.Error(1)
}

func (m *mockNoteRepository) GetStatusesByIDs(ctx context.Context, statusIDs []string) ([]*models.Status, error) {
	args := m.Called(ctx, statusIDs)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.Status), args.Error(1)
}

func (m *mockNoteRepository) GetStatusCounts(ctx context.Context, statusID string) (likes, reblogs, replies int, err error) {
	args := m.Called(ctx, statusID)
	return args.Int(0), args.Int(1), args.Int(2), args.Error(3)
}

func (m *mockNoteRepository) GetStatusContext(ctx context.Context, statusID string) (ancestors, descendants []*models.Status, err error) {
	args := m.Called(ctx, statusID)
	var anc, desc []*models.Status
	if args.Get(0) != nil {
		anc = args.Get(0).([]*models.Status)
	}
	if args.Get(1) != nil {
		desc = args.Get(1).([]*models.Status)
	}
	return anc, desc, args.Error(2)
}

func (m *mockNoteRepository) GetStatusEngagement(ctx context.Context, statusID, userID string) (liked, reblogged, bookmarked bool, err error) {
	args := m.Called(ctx, statusID, userID)
	return args.Bool(0), args.Bool(1), args.Bool(2), args.Error(3)
}

func (m *mockNoteRepository) DeleteBoostStatus(context.Context, string, string) (*models.Status, error) {
	// Direct message tests never create boost statuses; return nil to satisfy interface.
	return nil, nil
}

func (m *mockNoteRepository) CountStatusesByAuthor(ctx context.Context, authorID string) (int, error) {
	args := m.Called(ctx, authorID)
	return args.Int(0), args.Error(1)
}

func (m *mockNoteRepository) CountReplies(ctx context.Context, statusID string) (int, error) {
	args := m.Called(ctx, statusID)
	return args.Int(0), args.Error(1)
}

func (m *mockNoteRepository) ListStatusesForAdmin(ctx context.Context, filter *interfaces.StatusFilter, limit int, cursor string) ([]*models.Status, string, error) {
	args := m.Called(ctx, filter, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*models.Status), args.String(1), args.Error(2)
}

type mockAccountRepository struct {
	mock.Mock
}

func (m *mockAccountRepository) GetAccount(ctx context.Context, username string) (*storage.Account, error) {
	args := m.Called(ctx, username)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.Account), args.Error(1)
}

func (m *mockAccountRepository) CreateAccount(ctx context.Context, account *storage.Account) error {
	args := m.Called(ctx, account)
	return args.Error(0)
}

func (m *mockAccountRepository) GetAccountByURL(ctx context.Context, actorURL string) (*storage.Account, error) {
	args := m.Called(ctx, actorURL)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.Account), args.Error(1)
}

func (m *mockAccountRepository) GetAccountByEmail(ctx context.Context, email string) (*storage.Account, error) {
	args := m.Called(ctx, email)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.Account), args.Error(1)
}

func (m *mockAccountRepository) UpdateAccount(ctx context.Context, account *storage.Account) error {
	args := m.Called(ctx, account)
	return args.Error(0)
}

func (m *mockAccountRepository) DeleteAccount(ctx context.Context, username string) error {
	args := m.Called(ctx, username)
	return args.Error(0)
}

func (m *mockAccountRepository) SearchAccounts(ctx context.Context, query string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*storage.Account], error) {
	args := m.Called(ctx, query, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*interfaces.PaginatedResult[*storage.Account]), args.Error(1)
}

func (m *mockAccountRepository) GetSuggestedAccounts(ctx context.Context, forUserID string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*storage.AccountSuggestion], error) {
	args := m.Called(ctx, forUserID, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*interfaces.PaginatedResult[*storage.AccountSuggestion]), args.Error(1)
}

func (m *mockAccountRepository) GetFeaturedAccounts(ctx context.Context, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*storage.Account], error) {
	args := m.Called(ctx, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*interfaces.PaginatedResult[*storage.Account]), args.Error(1)
}

func (m *mockAccountRepository) ApproveAccount(ctx context.Context, username string) error {
	args := m.Called(ctx, username)
	return args.Error(0)
}

func (m *mockAccountRepository) SuspendAccount(ctx context.Context, username string, reason string) error {
	args := m.Called(ctx, username, reason)
	return args.Error(0)
}

func (m *mockAccountRepository) UnsuspendAccount(ctx context.Context, username string) error {
	args := m.Called(ctx, username)
	return args.Error(0)
}

func (m *mockAccountRepository) SilenceAccount(ctx context.Context, username string, reason string) error {
	args := m.Called(ctx, username, reason)
	return args.Error(0)
}

func (m *mockAccountRepository) UnsilenceAccount(ctx context.Context, username string) error {
	args := m.Called(ctx, username)
	return args.Error(0)
}

func (m *mockAccountRepository) UpdateAccountPreferences(ctx context.Context, username string, preferences map[string]interface{}) error {
	args := m.Called(ctx, username, preferences)
	return args.Error(0)
}

func (m *mockAccountRepository) GetAccountPreferences(ctx context.Context, username string) (map[string]interface{}, error) {
	args := m.Called(ctx, username)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(map[string]interface{}), args.Error(1)
}

func (m *mockAccountRepository) UpdateAccountFeatures(ctx context.Context, username string, features map[string]bool) error {
	args := m.Called(ctx, username, features)
	return args.Error(0)
}

func (m *mockAccountRepository) GetAccountFeatures(ctx context.Context, username string) (map[string]bool, error) {
	args := m.Called(ctx, username)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(map[string]bool), args.Error(1)
}

func (m *mockAccountRepository) ValidateCredentials(ctx context.Context, username, password string) (*storage.Account, error) {
	args := m.Called(ctx, username, password)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.Account), args.Error(1)
}

func (m *mockAccountRepository) UpdatePassword(ctx context.Context, username, newPasswordHash string) error {
	args := m.Called(ctx, username, newPasswordHash)
	return args.Error(0)
}

func (m *mockAccountRepository) CreatePasswordReset(ctx context.Context, reset *storage.PasswordReset) error {
	args := m.Called(ctx, reset)
	return args.Error(0)
}

func (m *mockAccountRepository) GetPasswordReset(ctx context.Context, token string) (*storage.PasswordReset, error) {
	args := m.Called(ctx, token)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.PasswordReset), args.Error(1)
}

func (m *mockAccountRepository) UsePasswordReset(ctx context.Context, token string) error {
	args := m.Called(ctx, token)
	return args.Error(0)
}

func (m *mockAccountRepository) RecordLogin(ctx context.Context, attempt *storage.LoginAttempt) error {
	args := m.Called(ctx, attempt)
	return args.Error(0)
}

func (m *mockAccountRepository) GetLoginHistory(ctx context.Context, username string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*storage.LoginAttempt], error) {
	args := m.Called(ctx, username, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*interfaces.PaginatedResult[*storage.LoginAttempt]), args.Error(1)
}

func (m *mockAccountRepository) UpdateLastActivity(ctx context.Context, username string, activity time.Time) error {
	args := m.Called(ctx, username, activity)
	return args.Error(0)
}

func (m *mockAccountRepository) GetAccountsByUsernames(ctx context.Context, usernames []string) ([]*storage.Account, error) {
	args := m.Called(ctx, usernames)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.Account), args.Error(1)
}

func (m *mockAccountRepository) GetAccountsCount(ctx context.Context) (int64, error) {
	args := m.Called(ctx)
	return args.Get(0).(int64), args.Error(1)
}

func (m *mockAccountRepository) AddBookmark(ctx context.Context, username, objectID string) error {
	args := m.Called(ctx, username, objectID)
	return args.Error(0)
}

func (m *mockAccountRepository) RemoveBookmark(ctx context.Context, username, objectID string) error {
	args := m.Called(ctx, username, objectID)
	return args.Error(0)
}

func (m *mockAccountRepository) GetBookmarks(ctx context.Context, username string, limit int, cursor string) ([]*storage.Bookmark, string, error) {
	args := m.Called(ctx, username, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*storage.Bookmark), args.String(1), args.Error(2)
}

func (m *mockAccountRepository) GetBookmarkedStatuses(ctx context.Context, username string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Status], error) {
	args := m.Called(ctx, username, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*interfaces.PaginatedResult[*models.Status]), args.Error(1)
}

type mockPublisher struct {
	events []streaming.Event
}

func (m *mockPublisher) PublishToUser(ctx context.Context, userID string, event *streaming.Event) error {
	// Preserve the stream name set by the service - don't override it
	if event.Stream == "" {
		event.Stream = fmt.Sprintf("user:%s", userID)
	}
	m.events = append(m.events, *event)
	return nil
}

func (m *mockPublisher) PublishToStream(ctx context.Context, streamName string, event *streaming.Event) error {
	event.Stream = streamName
	m.events = append(m.events, *event)
	return nil
}

func (m *mockPublisher) PublishToConversation(ctx context.Context, conversationID string, event *streaming.Event) error {
	event.Stream = fmt.Sprintf("conversation:%s", conversationID)
	m.events = append(m.events, *event)
	return nil
}

func (m *mockPublisher) Close() error {
	return nil
}

func (m *mockPublisher) GetEvents() []streaming.Event {
	return m.events
}

type mockFederationService struct {
	activities []*activitypub.Activity
}

func (m *mockFederationService) QueueActivity(ctx context.Context, activity *activitypub.Activity) error {
	m.activities = append(m.activities, activity)
	return nil
}

func (m *mockFederationService) GetQueuedActivities() []*activitypub.Activity {
	return m.activities
}

// Test helper functions

func createTestService() (*Service, *mockConversationRepository, *mockNoteRepository, *mockAccountRepository, *mockPublisher, *mockFederationService) {
	conversationRepo := &mockConversationRepository{}
	noteRepo := &mockNoteRepository{}
	accountRepo := &mockAccountRepository{}
	publisher := &mockPublisher{}
	federation := &mockFederationService{}
	logger := zaptest.NewLogger(&testing.T{})

	service := NewService(
		conversationRepo,
		noteRepo,
		nil,
		accountRepo,
		nil,
		nil,
		publisher,
		federation,
		logger,
		"example.com",
	)

	return service, conversationRepo, noteRepo, accountRepo, publisher, federation
}

func createTestAccount(userID, username string) *storage.Account {
	return &storage.Account{
		User: &storage.User{
			Username: username,
			Email:    fmt.Sprintf("%s@example.com", username),
		},
	}
}

func createTestConversation(id string, participants []string) *models.Conversation {
	return &models.Conversation{
		ID:           id,
		Participants: participants,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
}

func createTestMessage(id, authorID, conversationID, content string) *models.Status {
	status := &models.Status{
		StatusID:       id,
		AuthorID:       authorID,
		Content:        content,
		Visibility:     VisibilityDirect,
		ConversationID: conversationID,
		PublishedAt:    time.Now(),
	}

	// For testing, we'll assume these methods exist and work correctly
	// In a real implementation, these would be part of the Status model
	return status
}

// Test cases

func TestService_SendDirectMessage_NewConversation(t *testing.T) {
	service, conversationRepo, noteRepo, accountRepo, publisher, federation := createTestService()
	ctx := context.Background()

	// Test data
	senderAccount := createTestAccount("sender123", "alice")
	recipientAccount := createTestAccount("recipient456", "bob")

	cmd := &SendDirectMessageCommand{
		SenderID:   "sender123",
		Recipients: []string{"recipient456"},
		Content:    "Hello, this is a direct message!",
		Language:   "en",
	}

	// Mock expectations - called once for sender and once for recipient (accounts cached after that)
	accountRepo.On("GetAccount", ctx, "sender123").Return(senderAccount, nil)
	accountRepo.On("GetAccount", ctx, "recipient456").Return(recipientAccount, nil)

	// No existing conversation
	conversationRepo.On("GetConversationByParticipants", ctx, []string{"recipient456", "sender123"}).Return(nil, fmt.Errorf("not found"))

	// Create new conversation
	conversationRepo.On("CreateConversation", ctx, mock.AnythingOfType("*models.Conversation"), []string{"recipient456", "sender123"}).Return(nil)

	// Create message
	noteRepo.On("CreateStatus", ctx, mock.AnythingOfType("*models.Status")).Return(nil)

	// Update conversation
	conversationRepo.On("UpdateConversation", ctx, mock.AnythingOfType("*models.Conversation")).Return(nil)

	// DM request lifecycle + unread tracking
	conversationRepo.On("GetConversationParticipantRecord", ctx, mock.Anything, "sender123").Return(&models.ConversationParticipantRecord{}, nil)
	conversationRepo.On("GetConversationParticipantRecord", ctx, mock.Anything, "recipient456").Return(&models.ConversationParticipantRecord{}, nil)
	conversationRepo.On("UpdateConversationParticipantRecord", ctx, mock.AnythingOfType("*models.ConversationParticipantRecord")).Return(nil)
	conversationRepo.On("MarkConversationRead", ctx, mock.Anything, "sender123").Return(nil)
	conversationRepo.On("MarkConversationUnread", ctx, mock.Anything, "recipient456").Return(nil)

	// Execute
	result, err := service.SendDirectMessage(ctx, cmd)

	// Assertions
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.NotNil(t, result.Message)
	assert.NotNil(t, result.Conversation)
	assert.Equal(t, "Hello, this is a direct message!", result.Message.Content)
	assert.Equal(t, VisibilityDirect, result.Message.Visibility)
	assert.Equal(t, "sender123", result.Message.AuthorID)
	assert.Contains(t, result.Conversation.Participants, "sender123")
	assert.Contains(t, result.Conversation.Participants, "recipient456")
	assert.Len(t, result.Events, 3) // conversation + 2 user direct streams

	// Verify federation was NOT queued (local recipients only)
	activities := federation.GetQueuedActivities()
	assert.Len(t, activities, 0)

	// Verify events were published
	events := publisher.GetEvents()
	assert.Len(t, events, 3)

	conversationRepo.AssertExpectations(t)
	noteRepo.AssertExpectations(t)
	accountRepo.AssertExpectations(t)
}

func TestService_SendDirectMessage_WithRemoteRecipient(t *testing.T) {
	service, conversationRepo, noteRepo, accountRepo, _, federation := createTestService()
	ctx := context.Background()

	// Test data - create a remote recipient (different domain)
	senderAccount := createTestAccount("sender123", "alice")
	remoteRecipientAccount := &storage.Account{
		User: &storage.User{
			Username: "bob",
			Email:    "bob@remote.com",
		},
	}

	cmd := &SendDirectMessageCommand{
		SenderID:   "sender123",
		Recipients: []string{"bob@remote.com"},
		Content:    "Hello remote user!",
	}

	// Mock expectations
	accountRepo.On("GetAccount", ctx, "sender123").Return(senderAccount, nil)
	accountRepo.On("GetAccount", ctx, "bob@remote.com").Return(remoteRecipientAccount, nil)

	// No existing conversation
	conversationRepo.On("GetConversationByParticipants", ctx, []string{"bob@remote.com", "sender123"}).Return(nil, fmt.Errorf("not found"))

	// Create new conversation
	conversationRepo.On("CreateConversation", ctx, mock.AnythingOfType("*models.Conversation"), []string{"bob@remote.com", "sender123"}).Return(nil)

	// Create message
	noteRepo.On("CreateStatus", ctx, mock.AnythingOfType("*models.Status")).Return(nil)

	// Update conversation
	conversationRepo.On("UpdateConversation", ctx, mock.AnythingOfType("*models.Conversation")).Return(nil)

	// DM request lifecycle + unread tracking
	conversationRepo.On("GetConversationParticipantRecord", ctx, mock.Anything, "sender123").Return(&models.ConversationParticipantRecord{}, nil)
	conversationRepo.On("GetConversationParticipantRecord", ctx, mock.Anything, "bob@remote.com").Return(&models.ConversationParticipantRecord{}, nil)
	conversationRepo.On("UpdateConversationParticipantRecord", ctx, mock.AnythingOfType("*models.ConversationParticipantRecord")).Return(nil)
	conversationRepo.On("MarkConversationRead", ctx, mock.Anything, "sender123").Return(nil)
	conversationRepo.On("MarkConversationUnread", ctx, mock.Anything, "bob@remote.com").Return(nil)

	// Execute
	result, err := service.SendDirectMessage(ctx, cmd)

	// Assertions
	require.NoError(t, err)
	assert.NotNil(t, result)

	// Verify federation was NOT queued - even for "remote" accounts, if we store them locally
	// they get processed as local users since we construct URLs with our domain
	activities := federation.GetQueuedActivities()
	assert.Len(t, activities, 0)

	conversationRepo.AssertExpectations(t)
	noteRepo.AssertExpectations(t)
	accountRepo.AssertExpectations(t)
}

func TestService_SendDirectMessage_ExistingConversation(t *testing.T) {
	service, conversationRepo, noteRepo, accountRepo, _, _ := createTestService()
	ctx := context.Background()

	// Test data
	senderAccount := createTestAccount("sender123", "alice")
	recipientAccount := createTestAccount("recipient456", "bob")
	existingConversation := createTestConversation("conv123", []string{"recipient456", "sender123"})

	cmd := &SendDirectMessageCommand{
		SenderID:   "sender123",
		Recipients: []string{"recipient456"},
		Content:    "Another message in existing conversation",
	}

	// Mock expectations - called once for sender and once for recipient (accounts cached after that)
	accountRepo.On("GetAccount", ctx, "sender123").Return(senderAccount, nil)
	accountRepo.On("GetAccount", ctx, "recipient456").Return(recipientAccount, nil)

	// Existing conversation found
	conversationRepo.On("GetConversationByParticipants", ctx, []string{"recipient456", "sender123"}).Return(existingConversation, nil)

	// Create message
	noteRepo.On("CreateStatus", ctx, mock.AnythingOfType("*models.Status")).Return(nil)

	// Update conversation
	conversationRepo.On("UpdateConversation", ctx, mock.AnythingOfType("*models.Conversation")).Return(nil)

	// DM request lifecycle + unread tracking
	conversationRepo.On("GetConversationParticipantRecord", ctx, mock.Anything, "sender123").Return(&models.ConversationParticipantRecord{}, nil)
	conversationRepo.On("GetConversationParticipantRecord", ctx, mock.Anything, "recipient456").Return(&models.ConversationParticipantRecord{}, nil)
	conversationRepo.On("UpdateConversationParticipantRecord", ctx, mock.AnythingOfType("*models.ConversationParticipantRecord")).Return(nil)
	conversationRepo.On("MarkConversationRead", ctx, "conv123", "sender123").Return(nil)
	conversationRepo.On("MarkConversationUnread", ctx, "conv123", "recipient456").Return(nil)

	// Execute
	result, err := service.SendDirectMessage(ctx, cmd)

	// Assertions
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "conv123", result.Conversation.ID)
	assert.Equal(t, "Another message in existing conversation", result.Message.Content)

	conversationRepo.AssertExpectations(t)
	noteRepo.AssertExpectations(t)
	accountRepo.AssertExpectations(t)
}

func TestService_SendDirectMessage_ValidationError(t *testing.T) {
	service, _, _, _, _, _ := createTestService()
	ctx := context.Background()

	// Test empty content
	cmd := &SendDirectMessageCommand{
		SenderID:   "sender123",
		Recipients: []string{"recipient456"},
		Content:    "",
	}

	result, err := service.SendDirectMessage(ctx, cmd)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "is required")
}

func TestService_SendDirectMessage_InvalidSender(t *testing.T) {
	service, _, _, accountRepo, _, _ := createTestService()
	ctx := context.Background()

	cmd := &SendDirectMessageCommand{
		SenderID:   "invalid_sender",
		Recipients: []string{"recipient456"},
		Content:    "Test message",
	}

	// Mock sender not found
	accountRepo.On("GetAccount", ctx, "invalid_sender").Return(nil, fmt.Errorf("account not found"))

	result, err := service.SendDirectMessage(ctx, cmd)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "Failed to retrieve sender account")

	accountRepo.AssertExpectations(t)
}

func TestService_MarkConversationRead_Success(t *testing.T) {
	service, conversationRepo, _, _, publisher, _ := createTestService()
	ctx := context.Background()

	conversation := createTestConversation("conv123", []string{"user123", "user456"})

	cmd := &MarkConversationReadCommand{
		ConversationID: "conv123",
		UserID:         "user123",
	}

	// Mock expectations
	conversationRepo.On("GetConversation", ctx, "conv123").Return(conversation, nil)
	conversationRepo.On("MarkConversationRead", ctx, "conv123", "user123").Return(nil)

	// Execute
	result, err := service.MarkConversationRead(ctx, cmd)

	// Assertions
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "conv123", result.Conversation.ID)
	assert.Len(t, result.Events, 1)
	assert.Equal(t, ConversationReadEvent, result.Events[0].Type)

	// Verify event was published only to the user who read it
	events := publisher.GetEvents()
	assert.Len(t, events, 1)
	assert.Equal(t, "user:user123:direct", events[0].Stream)

	conversationRepo.AssertExpectations(t)
}

func TestService_MarkConversationRead_NotParticipant(t *testing.T) {
	service, conversationRepo, _, _, _, _ := createTestService()
	ctx := context.Background()

	conversation := createTestConversation("conv123", []string{"user123", "user456"})

	cmd := &MarkConversationReadCommand{
		ConversationID: "conv123",
		UserID:         "user789", // Not a participant
	}

	// Mock expectations
	conversationRepo.On("GetConversation", ctx, "conv123").Return(conversation, nil)

	// Execute
	result, err := service.MarkConversationRead(ctx, cmd)

	// Assertions
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "Access denied")

	conversationRepo.AssertExpectations(t)
}

func TestService_ListConversations_Success(t *testing.T) {
	service, conversationRepo, _, _, _, _ := createTestService()
	ctx := context.Background()

	conversations := []*models.Conversation{
		createTestConversation("conv1", []string{"user123", "user456"}),
		createTestConversation("conv2", []string{"user123", "user789"}),
	}

	paginatedResult := &interfaces.PaginatedResult[*models.Conversation]{
		Items:   conversations,
		HasMore: false,
		Total:   2,
	}

	query := &ListConversationsQuery{
		UserID: "user123",
		Pagination: interfaces.PaginationOptions{
			Limit: 10,
		},
	}

	// Mock expectations
	conversationRepo.On("GetUserConversations", ctx, "user123", query.Pagination).Return(paginatedResult, nil)

	// Execute
	result, err := service.ListConversations(ctx, query)

	// Assertions
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Len(t, result.Conversations.Items, 2)
	assert.Equal(t, "conv1", result.Conversations.Items[0].ID)
	assert.Equal(t, "conv2", result.Conversations.Items[1].ID)
	assert.Len(t, result.Events, 0) // No events for read operations

	conversationRepo.AssertExpectations(t)
}

func TestService_ListConversations_UnreadOnly(t *testing.T) {
	service, conversationRepo, _, _, _, _ := createTestService()
	ctx := context.Background()

	conversations := []*models.Conversation{
		createTestConversation("conv1", []string{"user123", "user456"}),
	}

	paginatedResult := &interfaces.PaginatedResult[*models.Conversation]{
		Items:   conversations,
		HasMore: false,
		Total:   1,
	}

	query := &ListConversationsQuery{
		UserID:     "user123",
		OnlyUnread: true,
		Pagination: interfaces.PaginationOptions{
			Limit: 10,
		},
	}

	// Mock expectations
	conversationRepo.On("GetUnreadConversations", ctx, "user123", query.Pagination).Return(paginatedResult, nil)

	// Execute
	result, err := service.ListConversations(ctx, query)

	// Assertions
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Len(t, result.Conversations.Items, 1)

	conversationRepo.AssertExpectations(t)
}

func TestService_GetConversation_Success(t *testing.T) {
	service, conversationRepo, noteRepo, _, _, _ := createTestService()
	ctx := context.Background()

	conversation := createTestConversation("conv123", []string{"user123", "user456"})

	// Create messages with proper recipients so they pass visibility checks
	msg1 := createTestMessage("msg1", "user123", "conv123", "Hello")
	msg1.AuthorUsername = "user123"
	msg1.ToRecipients = []string{"https://example.com/users/user456"} // Make user456 a recipient

	msg2 := createTestMessage("msg2", "user456", "conv123", "Hi there!")
	msg2.AuthorUsername = "user456"
	msg2.ToRecipients = []string{"https://example.com/users/user123"} // Make user123 a recipient

	messages := []*models.Status{msg1, msg2}

	paginatedMessages := &interfaces.PaginatedResult[*models.Status]{
		Items:   messages,
		HasMore: false,
		Total:   2,
	}

	query := &GetConversationQuery{
		ConversationID: "conv123",
		ViewerID:       "user123",
		Pagination: interfaces.PaginationOptions{
			Limit: 20,
		},
	}

	// Mock expectations
	conversationRepo.On("GetConversation", ctx, "conv123").Return(conversation, nil)
	conversationRepo.On("GetConversationParticipantRecord", ctx, "conv123", "user123").Return(&models.ConversationParticipantRecord{Unread: false}, nil)
	noteRepo.On("GetConversationThread", ctx, "conv123", query.Pagination).Return(paginatedMessages, nil)

	// Execute
	result, err := service.GetConversation(ctx, query)

	// Assertions
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "conv123", result.Conversation.ID)
	assert.Len(t, result.Messages.Items, 2)
	assert.Equal(t, "Hello", result.Messages.Items[0].Content)
	assert.Equal(t, "Hi there!", result.Messages.Items[1].Content)
	assert.Len(t, result.Events, 0) // No events for read operations

	conversationRepo.AssertExpectations(t)
	noteRepo.AssertExpectations(t)
}

func TestService_GetConversation_NotParticipant(t *testing.T) {
	service, conversationRepo, _, _, _, _ := createTestService()
	ctx := context.Background()

	conversation := createTestConversation("conv123", []string{"user123", "user456"})

	query := &GetConversationQuery{
		ConversationID: "conv123",
		ViewerID:       "user789", // Not a participant
	}

	// Mock expectations
	conversationRepo.On("GetConversation", ctx, "conv123").Return(conversation, nil)

	// Execute
	result, err := service.GetConversation(ctx, query)

	// Assertions
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "Access denied")

	conversationRepo.AssertExpectations(t)
}

func TestService_ValidateSendMessageCommandBasic_Success(t *testing.T) {
	service, _, _, _, _, _ := createTestService()
	ctx := context.Background()

	cmd := &SendDirectMessageCommand{
		SenderID:   "sender123",
		Recipients: []string{"recipient456"},
		Content:    "Valid message content",
	}

	// Execute
	err := service.validateSendMessageCommandBasic(ctx, cmd)

	// Assertions
	assert.NoError(t, err)
}

func TestService_ValidateSendMessageCommandBasic_ContentTooLong(t *testing.T) {
	service, _, _, _, _, _ := createTestService()
	ctx := context.Background()

	longContent := strings.Repeat("a", 5001) // Exceeds 5000 character limit
	cmd := &SendDirectMessageCommand{
		SenderID:   "sender123",
		Recipients: []string{"recipient456"},
		Content:    longContent,
	}

	// Execute
	err := service.validateSendMessageCommandBasic(ctx, cmd)

	// Assertions
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "too long")
}

func TestService_IsParticipant(t *testing.T) {
	service, _, _, _, _, _ := createTestService()

	participants := []string{"user123", "user456", "user789"}

	// Test cases
	assert.True(t, service.isParticipant("user123", participants))
	assert.True(t, service.isParticipant("user456", participants))
	assert.True(t, service.isParticipant("user789", participants))
	assert.False(t, service.isParticipant("user999", participants))
	assert.False(t, service.isParticipant("", participants))
}

func TestService_BuildActivityPubNote(t *testing.T) {
	service, _, _, _, _, _ := createTestService()

	cmd := &SendDirectMessageCommand{
		SenderID:   "sender123",
		Recipients: []string{"recipient456"},
		Content:    "Test ActivityPub note",
		Sensitive:  true,
	}

	senderAccount := createTestAccount("sender123", "alice")
	recipientAccount := createTestAccount("recipient456", "bob")

	// Create recipient accounts map
	recipientAccounts := map[string]*storage.Account{
		"recipient456": recipientAccount,
	}

	// Execute
	note := service.buildActivityPubNote(cmd, "message123", senderAccount, "conv123", recipientAccounts)

	// Assertions
	assert.NotNil(t, note)
	assert.Equal(t, "Note", note.Type)
	assert.Equal(t, "Test ActivityPub note", note.Content)
	assert.Equal(t, "conv123", note.ConversationID)
	assert.True(t, note.Sensitive)
	assert.Equal(t, fmt.Sprintf("https://example.com/users/%s", senderAccount.User.Username), note.AttributedTo)
	assert.Contains(t, note.ID, "statuses/message123")
	assert.Len(t, note.To, 1) // Should have recipient
}

func TestService_NewService(t *testing.T) {
	conversationRepo := &mockConversationRepository{}
	noteRepo := &mockNoteRepository{}
	accountRepo := &mockAccountRepository{}
	publisher := &mockPublisher{}
	federation := &mockFederationService{}
	logger := zaptest.NewLogger(t)

	// Test with all dependencies
	service := NewService(conversationRepo, noteRepo, nil, accountRepo, nil, nil, publisher, federation, logger, "example.com")
	assert.NotNil(t, service)
	assert.Equal(t, "example.com", service.domainName)

	// Test with nil logger (should default to nop logger)
	service2 := NewService(conversationRepo, noteRepo, nil, accountRepo, nil, nil, publisher, federation, nil, "example.com")
	assert.NotNil(t, service2)
}

func TestService_DeleteConversation_DeleteForMe(t *testing.T) {
	service, conversationRepo, _, _, _, _ := createTestService()

	now := time.Now().UTC()
	conversation := &models.Conversation{
		ID:           "conv123",
		Participants: []string{"sender123", "recipient456"},
		CreatedAt:    now.Add(-time.Hour),
		UpdatedAt:    now.Add(-time.Minute),
	}

	participantRecord := &models.ConversationParticipantRecord{
		Conversation: conversation,
		Unread:       false,
		DeletedAt:    nil,
	}

	conversationRepo.On("GetConversation", mock.Anything, "conv123").Return(conversation, nil)
	conversationRepo.
		On("GetConversationParticipantRecord", mock.Anything, "conv123", "sender123").
		Return(participantRecord, nil)
	conversationRepo.
		On("UpdateConversationParticipantRecord", mock.Anything, mock.MatchedBy(func(r *models.ConversationParticipantRecord) bool {
			return r != nil && r.DeletedAt != nil && !r.DeletedAt.IsZero()
		})).
		Return(nil)

	result, err := service.DeleteConversation(context.Background(), &DeleteConversationCommand{
		ConversationID: "conv123",
		UserID:         "sender123",
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	conversationRepo.AssertExpectations(t)
}

func TestService_GetConversation_HidesDeletedConversation(t *testing.T) {
	service, conversationRepo, noteRepo, _, _, _ := createTestService()

	now := time.Now().UTC()
	conversation := &models.Conversation{
		ID:           "conv123",
		Participants: []string{"sender123", "recipient456"},
		CreatedAt:    now.Add(-time.Hour),
		UpdatedAt:    now.Add(-time.Minute),
	}
	deletedAt := now.Add(-time.Second)
	participantRecord := &models.ConversationParticipantRecord{
		Conversation: conversation,
		Unread:       false,
		DeletedAt:    &deletedAt,
	}

	conversationRepo.On("GetConversation", mock.Anything, "conv123").Return(conversation, nil)
	conversationRepo.
		On("GetConversationParticipantRecord", mock.Anything, "conv123", "sender123").
		Return(participantRecord, nil)

	_, err := service.GetConversation(context.Background(), &GetConversationQuery{
		ConversationID: "conv123",
		ViewerID:       "sender123",
		Pagination:     interfaces.PaginationOptions{Limit: 10},
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrConversationNotFound)

	conversationRepo.AssertExpectations(t)
	noteRepo.AssertNotCalled(t, "GetConversationThread")
}

func TestService_DeleteMessage_CreatesViewerTombstone(t *testing.T) {
	conversationRepo := &mockConversationRepository{}
	noteRepo := &mockNoteRepository{}
	dmTombstoneRepo := &mockDirectMessageTombstoneRepository{}
	accountRepo := &mockAccountRepository{}

	service := NewService(
		conversationRepo,
		noteRepo,
		dmTombstoneRepo,
		accountRepo,
		nil,
		nil,
		&mockPublisher{},
		&mockFederationService{},
		zaptest.NewLogger(t),
		"example.com",
	)

	status := &models.Status{
		StatusID:       "m1",
		Visibility:     models.VisibilityDirect,
		AuthorUsername: "recipient456",
		ConversationID: "conv123",
		ToRecipients:   []string{"https://example.com/users/sender123"},
	}
	conversation := &models.Conversation{
		ID:           "conv123",
		Participants: []string{"sender123", "recipient456"},
	}

	noteRepo.On("GetStatus", mock.Anything, "m1").Return(status, nil)
	conversationRepo.On("GetConversation", mock.Anything, "conv123").Return(conversation, nil)
	dmTombstoneRepo.On("CreateTombstone", mock.Anything, "sender123", "m1").Return(nil)

	ok, err := service.DeleteMessage(context.Background(), &DeleteMessageCommand{
		MessageID: "m1",
		UserID:    "sender123",
	})
	require.NoError(t, err)
	assert.True(t, ok)

	noteRepo.AssertExpectations(t)
	conversationRepo.AssertExpectations(t)
	dmTombstoneRepo.AssertExpectations(t)
}

func TestService_GetConversation_FiltersTombstonedMessages(t *testing.T) {
	conversationRepo := &mockConversationRepository{}
	noteRepo := &mockNoteRepository{}
	dmTombstoneRepo := &mockDirectMessageTombstoneRepository{}
	accountRepo := &mockAccountRepository{}

	service := NewService(
		conversationRepo,
		noteRepo,
		dmTombstoneRepo,
		accountRepo,
		nil,
		nil,
		&mockPublisher{},
		&mockFederationService{},
		zaptest.NewLogger(t),
		"example.com",
	)

	now := time.Now()
	conversation := &models.Conversation{
		ID:           "conv123",
		Participants: []string{"sender123", "recipient456"},
		CreatedAt:    now.Add(-time.Hour),
		UpdatedAt:    now.Add(-time.Minute),
	}
	participantRecord := &models.ConversationParticipantRecord{
		Conversation: conversation,
		Unread:       false,
		DeletedAt:    nil,
	}

	m1 := &models.Status{
		StatusID:       "m1",
		Visibility:     models.VisibilityDirect,
		AuthorUsername: "recipient456",
		ConversationID: "conv123",
		ToRecipients:   []string{"https://example.com/users/sender123"},
		PublishedAt:    now.Add(-2 * time.Minute),
	}
	m2 := &models.Status{
		StatusID:       "m2",
		Visibility:     models.VisibilityDirect,
		AuthorUsername: "recipient456",
		ConversationID: "conv123",
		ToRecipients:   []string{"https://example.com/users/sender123"},
		PublishedAt:    now.Add(-time.Minute),
	}

	conversationRepo.On("GetConversation", mock.Anything, "conv123").Return(conversation, nil)
	conversationRepo.
		On("GetConversationParticipantRecord", mock.Anything, "conv123", "sender123").
		Return(participantRecord, nil)
	noteRepo.
		On("GetConversationThread", mock.Anything, "conv123", mock.Anything).
		Return(&interfaces.PaginatedResult[*models.Status]{Items: []*models.Status{m1, m2}, HasMore: false}, nil)
	dmTombstoneRepo.
		On("TombstonesByStatusID", mock.Anything, "sender123", mock.Anything).
		Return(map[string]bool{"m2": true}, nil)

	result, err := service.GetConversation(context.Background(), &GetConversationQuery{
		ConversationID: "conv123",
		ViewerID:       "sender123",
		Pagination:     interfaces.PaginationOptions{Limit: 50},
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, result.Messages)
	require.Len(t, result.Messages.Items, 1)
	assert.Equal(t, "m1", result.Messages.Items[0].StatusID)

	conversationRepo.AssertExpectations(t)
	noteRepo.AssertExpectations(t)
	dmTombstoneRepo.AssertExpectations(t)
}

func TestService_GetConversationLastStatus_SkipsTombstoned(t *testing.T) {
	conversationRepo := &mockConversationRepository{}
	noteRepo := &mockNoteRepository{}
	dmTombstoneRepo := &mockDirectMessageTombstoneRepository{}
	accountRepo := &mockAccountRepository{}

	service := NewService(
		conversationRepo,
		noteRepo,
		dmTombstoneRepo,
		accountRepo,
		nil,
		nil,
		&mockPublisher{},
		&mockFederationService{},
		zaptest.NewLogger(t),
		"example.com",
	)

	now := time.Now()
	latest := &models.Status{
		StatusID:       "m2",
		Visibility:     models.VisibilityDirect,
		AuthorUsername: "recipient456",
		ConversationID: "conv123",
		ToRecipients:   []string{"https://example.com/users/sender123"},
		PublishedAt:    now,
	}
	previous := &models.Status{
		StatusID:       "m1",
		Visibility:     models.VisibilityDirect,
		AuthorUsername: "recipient456",
		ConversationID: "conv123",
		ToRecipients:   []string{"https://example.com/users/sender123"},
		PublishedAt:    now.Add(-time.Minute),
	}

	noteRepo.
		On("GetConversationThreadReverse", mock.Anything, "conv123", mock.Anything).
		Return(&interfaces.PaginatedResult[*models.Status]{Items: []*models.Status{latest, previous}, HasMore: false}, nil)
	dmTombstoneRepo.
		On("TombstonesByStatusID", mock.Anything, "sender123", mock.Anything).
		Return(map[string]bool{"m2": true}, nil)

	status, err := service.GetConversationLastStatus(context.Background(), "conv123", "sender123")
	require.NoError(t, err)
	require.NotNil(t, status)
	assert.Equal(t, "m1", status.StatusID)

	noteRepo.AssertExpectations(t)
	dmTombstoneRepo.AssertExpectations(t)
}

func TestService_DeleteMessage_RequiresParticipant(t *testing.T) {
	conversationRepo := &mockConversationRepository{}
	noteRepo := &mockNoteRepository{}
	dmTombstoneRepo := &mockDirectMessageTombstoneRepository{}
	accountRepo := &mockAccountRepository{}

	service := NewService(
		conversationRepo,
		noteRepo,
		dmTombstoneRepo,
		accountRepo,
		nil,
		nil,
		&mockPublisher{},
		&mockFederationService{},
		zaptest.NewLogger(t),
		"example.com",
	)

	status := &models.Status{
		StatusID:       "m1",
		Visibility:     models.VisibilityDirect,
		AuthorUsername: "recipient456",
		ConversationID: "conv123",
		ToRecipients:   []string{"https://example.com/users/sender123"},
	}
	conversation := &models.Conversation{
		ID:           "conv123",
		Participants: []string{"recipient456", "charlie789"},
	}

	noteRepo.On("GetStatus", mock.Anything, "m1").Return(status, nil)
	conversationRepo.On("GetConversation", mock.Anything, "conv123").Return(conversation, nil)
	accountRepo.
		On("GetAccount", mock.Anything, "sender123").
		Return(&storage.Account{Actor: &activitypub.Actor{BaseObject: activitypub.BaseObject{ID: "https://example.com/users/sender123"}}}, nil)

	_, err := service.DeleteMessage(context.Background(), &DeleteMessageCommand{
		MessageID: "m1",
		UserID:    "sender123",
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNotConversationParticipant)
}

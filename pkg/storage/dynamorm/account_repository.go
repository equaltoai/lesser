// Package dynamorm provides DynamORM repository interfaces for unified account and user management operations.
package dynamorm

import (
	"context"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage"
)

// AccountRepository unifies User and Actor operations
type AccountRepository interface {
	// ===== Core Account Operations =====
	CreateAccount(ctx context.Context, username, email, passwordHash string, approved bool, actor *activitypub.Actor, privateKey string) error
	GetAccount(ctx context.Context, username string) (*storage.Account, error)
	DeleteAccount(ctx context.Context, username string) error

	// ===== User Operations (Authentication) =====
	GetUser(ctx context.Context, username string) (*storage.User, error)
	GetUserByEmail(ctx context.Context, email string) (*storage.User, error)
	UpdateUser(ctx context.Context, username string, updates map[string]interface{}) error

	// ===== Actor Operations (ActivityPub) =====
	GetActor(ctx context.Context, username string) (*activitypub.Actor, error)
	GetActorByUsername(ctx context.Context, username string) (*activitypub.Actor, error)
	GetActorPrivateKey(ctx context.Context, username string) (string, error)

	// ===== Authentication Methods =====
	ValidatePassword(ctx context.Context, username, password string) (*storage.User, error)
	GetRecentLoginAttempts(ctx context.Context, username string, since time.Time) ([]*storage.LoginAttempt, error)
	CreatePasswordResetToken(ctx context.Context, username, email string) (string, error)
	ValidatePasswordResetToken(ctx context.Context, token string) (*storage.PasswordReset, error)
	ResetPassword(ctx context.Context, token, newPasswordHash string) error
	GetUserSessions(ctx context.Context, username string) ([]*storage.Session, error)
	CreateSession(ctx context.Context, username, ipAddress, userAgent string) (*storage.Session, error)
	InvalidateSession(ctx context.Context, username, sessionID string) error
	InvalidateAllSessions(ctx context.Context, username string) error
	UpdateLastLogin(ctx context.Context, username string) error
	GetUserByRecoveryCode(ctx context.Context, recoveryCode string) (*storage.User, error)

	// ===== Social Features =====
	Follow(ctx context.Context, followerUsername, followeeUsername string) error
	Unfollow(ctx context.Context, followerUsername, followeeUsername string) error
	IsFollowing(ctx context.Context, followerUsername, followeeUsername string) (bool, error)
	GetFollowers(ctx context.Context, username string, limit int, cursor string) ([]*activitypub.Actor, string, error)
	GetFollowing(ctx context.Context, username string, limit int, cursor string) ([]*activitypub.Actor, string, error)
	Block(ctx context.Context, blockerUsername, blockedUsername string) error
	Unblock(ctx context.Context, blockerUsername, blockedUsername string) error
	IsBlocked(ctx context.Context, blockerUsername, blockedUsername string) (bool, error)
	GetBlocks(ctx context.Context, username string) ([]*storage.Block, error)
	Mute(ctx context.Context, muterUsername, mutedUsername string, notifications bool, duration time.Duration) error
	Unmute(ctx context.Context, muterUsername, mutedUsername string) error
	IsMuted(ctx context.Context, muterUsername, mutedUsername string) (bool, bool, error)
	GetMutes(ctx context.Context, username string) ([]*storage.Mute, error)
	AddBookmark(ctx context.Context, username, objectID string) error
	RemoveBookmark(ctx context.Context, username, objectID string) error
	GetBookmarks(ctx context.Context, username string, limit int, cursor string) ([]*storage.Bookmark, string, error)
	PinAccount(ctx context.Context, username, pinnedUsername string) error
	UnpinAccount(ctx context.Context, username, pinnedUsername string) error
	GetPinnedAccounts(ctx context.Context, username string) ([]*activitypub.Actor, error)

	// ===== Timeline Operations =====
	GetHomeTimeline(ctx context.Context, username string, limit int, maxID, sinceID string) ([]*storage.TimelineEntry, error)
	GetLocalTimeline(ctx context.Context, limit int, maxID, sinceID string) ([]*storage.TimelineEntry, error)
	GetPublicTimeline(ctx context.Context, limit int, maxID, sinceID string, onlyMedia bool) ([]*storage.TimelineEntry, error)
	GetHashtagTimeline(ctx context.Context, hashtag string, limit int, maxID, sinceID string) ([]*storage.TimelineEntry, error)
	GetListTimeline(ctx context.Context, username, listID string, limit int, maxID, sinceID string) ([]*storage.TimelineEntry, error)
	AddToTimeline(ctx context.Context, username string, entry *storage.TimelineEntry) error
	RemoveFromTimeline(ctx context.Context, username, objectID string) error
	GetConversations(ctx context.Context, username string, limit int, maxID, sinceID string) ([]*storage.Conversation, error)
	MuteConversation(ctx context.Context, username, conversationID string) error
	UnmuteConversation(ctx context.Context, username, conversationID string) error
	IsConversationMuted(ctx context.Context, username, conversationID string) (bool, error)
	GetTimelineMarkers(ctx context.Context, username string, timelines []string) (map[string]*storage.Marker, error)
	UpdateTimelineMarker(ctx context.Context, username, timeline, lastReadID string) error

	// ===== Search and Discovery =====
	SearchActors(ctx context.Context, query string, limit int, offset int, following bool, username string) ([]*activitypub.Actor, error)
	GetAccountSuggestions(ctx context.Context, username string, limit int) ([]*storage.AccountSuggestion, error)
	GetTrendingActors(ctx context.Context, limit int) ([]*activitypub.Actor, error)
	SearchByWebfinger(ctx context.Context, webfinger string) (*activitypub.Actor, error)
	CacheRemoteActor(ctx context.Context, actor *activitypub.Actor) error
	UpdateLastSeen(ctx context.Context, username string) error
	GetActiveUsers(ctx context.Context, since time.Time, limit int) ([]*storage.User, error)
	GetInactiveUsers(ctx context.Context, inactiveSince time.Time, limit int) ([]*storage.User, error)

	// ===== OAuth 2.0 Operations =====
	CreateAuthorizationCode(ctx context.Context, code *storage.AuthorizationCode) error
	GetAuthorizationCode(ctx context.Context, code string) (*storage.AuthorizationCode, error)
	DeleteAuthorizationCode(ctx context.Context, code string) error
	CreateRefreshToken(ctx context.Context, token *storage.RefreshToken) error
	GetRefreshToken(ctx context.Context, token string) (*storage.RefreshToken, error)
	DeleteRefreshToken(ctx context.Context, token string) error
	CreateOAuthClient(ctx context.Context, client *storage.OAuthClient) error
	GetOAuthClient(ctx context.Context, clientID string) (*storage.OAuthClient, error)
	DeleteOAuthClient(ctx context.Context, clientID string) error
	StoreOAuthState(ctx context.Context, state string, data *storage.OAuthState) error
	GetOAuthState(ctx context.Context, state string) (*storage.OAuthState, error)
	DeleteOAuthState(ctx context.Context, state string) error
	UpdateOAuthClient(ctx context.Context, clientID string, updates map[string]interface{}) error
	ListOAuthClients(ctx context.Context, limit int, cursor string) ([]*storage.OAuthClient, string, error)
	GetOAuthApp(ctx context.Context, clientID string) (*storage.OAuthApp, error)
	SaveUserAppConsent(ctx context.Context, consent *storage.UserAppConsent) error
	GetUserAppConsent(ctx context.Context, userID, appID string) (*storage.UserAppConsent, error)

	// ===== WebAuthn Operations =====
	CreateWebAuthnCredential(ctx context.Context, credential *storage.WebAuthnCredential) error
	GetWebAuthnCredential(ctx context.Context, credentialID string) (*storage.WebAuthnCredential, error)
	GetUserWebAuthnCredentials(ctx context.Context, username string) ([]*storage.WebAuthnCredential, error)
	UpdateWebAuthnLastUsed(ctx context.Context, credentialID string, signCount uint32) error
	DeleteWebAuthnCredential(ctx context.Context, credentialID string) error
	CreateWebAuthnChallenge(ctx context.Context, challenge *storage.WebAuthnChallenge) error
	GetWebAuthnChallenge(ctx context.Context, challengeID string) (*storage.WebAuthnChallenge, error)
	DeleteWebAuthnChallenge(ctx context.Context, challengeID string) error

	// ===== Wallet Operations =====
	StoreWalletChallenge(ctx context.Context, challenge *storage.WalletChallenge) error
	GetWalletChallenge(ctx context.Context, challengeID string) (*storage.WalletChallenge, error)
	StoreWalletCredential(ctx context.Context, credential *storage.WalletCredential) error
	GetWalletByAddress(ctx context.Context, walletType, address string) (*storage.WalletCredential, error)
	DeleteWalletChallenge(ctx context.Context, challengeID string) error
	GetUserWallets(ctx context.Context, username string) ([]*storage.WalletCredential, error)
	DeleteWalletCredential(ctx context.Context, username, address string) error

	// Set storage for operations not yet migrated
	SetStorage(storage interface{})
}

package repositories

import (
	"fmt"
	"reflect"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/theory-cloud/tabletheory/pkg/mocks"
)

// setupPermissiveRound08Mocks configures DynamORM mocks to succeed by default and
// populate common models used by Round 08 repository coverage tests.
func setupPermissiveRound08Mocks(mockDB *mocks.MockDB, mockQuery *mocks.MockQuery, mockUpdateBuilder *mocks.MockUpdateBuilder, baseTime time.Time) {
	mockDB.On("WithContext", mock.AnythingOfType("*context.emptyCtx")).Return(mockDB).Maybe()
	mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
	mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()

	mockQuery.On("WithContext", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Index", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Filter", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("FilterGroup", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Limit", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Cursor", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("ConsistentRead").Return(mockQuery).Maybe()
	mockQuery.On("Select", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("IfNotExists").Return(mockQuery).Maybe()
	mockQuery.On("IfExists").Return(mockQuery).Maybe()

	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		populateRound08SliceForCoverage(args.Get(0), baseTime)
	}).Return(nil).Maybe()

	mockQuery.On("Scan", mock.Anything).Run(func(args mock.Arguments) {
		populateRound08SliceForCoverage(args.Get(0), baseTime)
	}).Return(nil).Maybe()

	mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
		populateRound08StructForCoverage(args.Get(0), 0, baseTime)
	}).Return(nil).Maybe()

	mockQuery.On("Create").Return(nil).Maybe()
	mockQuery.On("Update").Return(nil).Maybe()
	mockQuery.On("Update", mock.Anything).Return(nil).Maybe()
	mockQuery.On("Delete").Return(nil).Maybe()
	mockQuery.On("Delete", mock.Anything).Return(nil).Maybe()
	mockQuery.On("Count").Return(int64(2), nil).Maybe()

	mockQuery.On("BatchCreate", mock.Anything).Return(nil).Maybe()
	mockQuery.On("BatchDelete", mock.Anything).Return(nil).Maybe()

	if mockUpdateBuilder == nil {
		mockUpdateBuilder = new(mocks.MockUpdateBuilder)
	}
	mockQuery.On("UpdateBuilder").Return(mockUpdateBuilder).Maybe()
	mockUpdateBuilder.On("Add", mock.Anything, mock.Anything).Return(mockUpdateBuilder).Maybe()
	mockUpdateBuilder.On("Condition", mock.Anything, mock.Anything, mock.Anything).Return(mockUpdateBuilder).Maybe()
	mockUpdateBuilder.On("Set", mock.Anything, mock.Anything).Return(mockUpdateBuilder).Maybe()
	mockUpdateBuilder.On("Remove", mock.Anything).Return(mockUpdateBuilder).Maybe()
	mockUpdateBuilder.On("SetIfNotExists", mock.Anything, mock.Anything, mock.Anything).Return(mockUpdateBuilder).Maybe()
	mockUpdateBuilder.On("ConditionVersion", mock.Anything).Return(mockUpdateBuilder).Maybe()
	mockUpdateBuilder.On("Execute").Return(nil).Maybe()
}

func populateRound08SliceForCoverage(target any, baseTime time.Time) {
	value := reflect.ValueOf(target)
	if value.Kind() != reflect.Ptr || value.Elem().Kind() != reflect.Slice {
		return
	}

	slice := value.Elem()
	elemType := slice.Type().Elem()
	if elemType.Kind() == reflect.Interface {
		return
	}

	baseElemType := elemType
	if baseElemType.Kind() == reflect.Ptr {
		baseElemType = baseElemType.Elem()
	}

	count := 2
	for i := range count {
		var element reflect.Value
		if elemType.Kind() == reflect.Ptr {
			element = reflect.New(baseElemType)
			populateRound08StructForCoverage(element.Interface(), i, baseTime)
		} else {
			elemPtr := reflect.New(baseElemType)
			populateRound08StructForCoverage(elemPtr.Interface(), i, baseTime)
			element = elemPtr.Elem()
		}
		slice = reflect.Append(slice, element)
	}

	value.Elem().Set(slice)
}

func populateRound08StructForCoverage(target any, idx int, baseTime time.Time) {
	now := baseTime.Add(time.Duration(idx) * time.Minute).UTC()

	switch model := target.(type) {
	case *models.Actor:
		model.Username = fmt.Sprintf("user-%d", idx+1)
		model.Actor = &activitypub.Actor{
			BaseObject: activitypub.BaseObject{
				ID: fmt.Sprintf("https://example.com/users/%s", model.Username),
			},
			PreferredUsername:         model.Username,
			ManuallyApprovesFollowers: idx%2 == 1,
		}
		model.CreatedAt = now
		model.UpdatedAt = now
		_ = model.BeforeCreate()

	case *models.Follow:
		follower := fmt.Sprintf("user-%d", idx+1)
		follow := models.NewFollow(follower, "user-2", fmt.Sprintf("https://example.com/%s/follow-%d", follower, idx+1))
		follow.Accept()
		*model = *follow

	case *models.Block:
		model.Actor = fmt.Sprintf("https://example.com/users/user-%d", idx+1)
		model.Object = "https://example.com/users/user-2"
		model.ID = fmt.Sprintf("block-%d", idx+1)
		model.Published = now
		model.CreatedAt = now
		_ = model.UpdateKeys()

	case *models.Mute:
		model.Actor = fmt.Sprintf("https://example.com/users/user-%d", idx+1)
		model.Object = "https://example.com/users/user-2"
		model.ID = fmt.Sprintf("mute-%d", idx+1)
		model.HideNotifications = idx%2 == 0
		model.Published = now
		model.CreatedAt = now
		_ = model.UpdateKeys()

	case *models.AccountPin:
		model.Username = "user-1"
		model.PinnedActorID = fmt.Sprintf("https://example.com/users/user-%d", idx+2)
		model.PinnedUsername = fmt.Sprintf("user-%d", idx+2)
		model.CreatedAt = now
		_ = model.UpdateKeys()

	case *models.Bookmark:
		model.Username = "user-1"
		model.ObjectID = fmt.Sprintf("object#status-%d", idx+1)
		model.CreatedAt = now
		model.RecordType = models.BookmarkRecordTypeTime
		_ = model.UpdateKeys()

	case *models.AuthorizationCode:
		model.Code = fmt.Sprintf("code-%d", idx+1)
		model.ClientID = "client-1"
		model.Username = "user-1"
		model.CodeChallenge = "challenge"
		model.ExpiresAt = now.Add(10 * time.Minute)
		model.Scopes = []string{"read"}
		model.CreatedAt = now
		_ = model.BeforeCreate()

	case *models.RefreshToken:
		model.Token = fmt.Sprintf("rt-%d", idx+1)
		model.ClientID = "client-1"
		model.Username = "user-1"
		model.ExpiresAt = now.Add(24 * time.Hour)
		model.Scopes = []string{"read"}
		model.CreatedAt = now
		_ = model.BeforeCreate()

	case *models.OAuthClient:
		model.ClientID = "client-1"
		model.ClientSecret = "secret"
		model.Name = "App"
		model.RedirectURIs = []string{"https://example.com/cb"}
		model.GrantTypes = []string{"authorization_code"}
		model.Scopes = []string{"read"}
		model.OwnerID = "user-1"
		model.Confidential = true
		model.CreatedAt = now
		model.UpdatedAt = now
		_ = model.BeforeCreate()

	case *models.UserAppConsent:
		model.UserID = "user-1"
		model.AppID = "client-1"
		model.Scopes = []string{"read"}
		model.CreatedAt = now
		model.UpdatedAt = now
		_ = model.UpdateKeys()

	case *models.OAuthAuthSession:
		model.SessionID = fmt.Sprintf("oauth-session-%d", idx+1)
		model.State = fmt.Sprintf("state-%d", idx+1)
		model.ClientID = "client-1"
		model.RedirectURI = "https://example.com/callback"
		model.CSRFToken = "csrf-token"
		model.Username = "user-1"
		model.FlowStep = "initiated"
		model.CreatedAt = now
		model.UpdatedAt = now
		model.LastUsedAt = now
		model.ExpiresAt = now.Add(30 * time.Minute).Unix()
		_ = model.UpdateKeys()

	case *models.AuthRefreshToken:
		model.Token = fmt.Sprintf("token-%d", idx+1)
		model.UserID = "user-1"
		model.Family = "family-1"
		model.Generation = idx + 1
		model.CreatedAt = now.Unix()
		model.ExpiresAt = now.Add(24 * time.Hour).Unix()
		model.LastUsedAt = now.Unix()
		model.Revoked = false
		model.DeviceName = "device"
		model.IPAddress = "127.0.0.1"
		_ = model.UpdateKeys()

	case *models.User:
		model.Username = "user-1"
		model.Email = "user-1@example.com"
		model.PasswordHash = "$2a$10$9Ay8tAONpS50qIdW.6P7Q.0i1o5nYVnDWAlC4AgdnbIXpNUv25B1q" // "password"
		model.Suspended = false

	case *userCoreProjection:
		model.Table = "test-table"
		model.PK = "USER#user-1"
		model.SK = models.SKMetadata
		model.Username = "user-1"
		model.PasswordHash = "$2a$10$9Ay8tAONpS50qIdW.6P7Q.0i1o5nYVnDWAlC4AgdnbIXpNUv25B1q" // "password"
		model.CreatedAt = now
		model.UpdatedAt = now
		model.Approved = true
		model.Role = "user"
		model.Version = 1

	case *userMetadataProjection:
		model.Table = "test-table"
		model.PK = "USER#user-1"
		model.SK = models.SKMetadata
		model.Metadata = map[string]interface{}{"theme": "dark"}

	case *models.UserLogin:
		model.Username = "user-1"
		model.Timestamp = now
		model.Success = idx%2 == 0
		model.IPAddress = "127.0.0.1"
		model.UserAgent = "test-agent"

	case *models.PasswordReset:
		model.Username = "user-1"
		model.Token = fmt.Sprintf("reset-%d", idx+1)
		model.Email = "user-1@example.com"
		model.CreatedAt = now
		model.ExpiresAt = now.Add(1 * time.Hour)
		model.Used = false
		_ = model.BeforeCreate()

	case *models.Session:
		model.SessionID = fmt.Sprintf("session-%d", idx+1)
		model.UserID = "USER#user-1"
		model.AccessToken = fmt.Sprintf("access-%d", idx+1)
		model.RefreshToken = fmt.Sprintf("refresh-%d", idx+1)
		model.CreatedAt = now
		model.UpdatedAt = now
		model.LastUsedAt = now
		model.ExpiresAt = now.Add(1 * time.Hour).Unix()
		model.PK = "session#" + model.SessionID
		model.SK = model.PK

	case *models.RecoveryCode:
		model.Username = "user-1"
		model.Position = idx
		model.CodeHash = "$2a$10$9Ay8tAONpS50qIdW.6P7Q.0i1o5nYVnDWAlC4AgdnbIXpNUv25B1q" // "password"
		model.CreatedAt = now
		_ = model.UpdateKeys()

	case *models.RateLimitLockout:
		model.PK = "RATELIMIT#user-1"
		model.SK = "LOCKOUT"
		model.UnlockTime = now.Add(2 * time.Minute)
		_ = model.UpdateKeys()

	case *models.LoginAttempt:
		model.PK = "RATELIMIT#user-1"
		model.Timestamp = now
		_ = model.UpdateKeys()

	case *models.CommunityNote:
		model.ID = fmt.Sprintf("note-%d", idx+1)
		model.ObjectID = "object-1"
		model.ObjectType = "Note"
		model.AuthorID = "user-1"
		model.Content = "note"
		model.VisibilityStatus = "active"
		model.CreatedAt = now
		model.UpdatedAt = now
		_ = model.UpdateKeys()

	case *models.APIRateLimit:
		model.PK = "RATELIMIT#user-1:endpoint"
		model.SK = "WINDOW#" + now.Format(time.RFC3339)
		model.Window = now.Truncate(time.Minute)
		model.Count = idx
		model.UpdatedAt = now
		model.Blocked = idx%2 == 1
		model.BlockedUntil = now.Add(2 * time.Minute)
		_ = model.UpdateKeys()

	case *models.RateLimitViolation:
		model.PK = "RATELIMIT_VIOLATION#user-1"
		model.Timestamp = now
		_ = model.UpdateKeys()

	case *models.RecoveryToken:
		model.PK = fmt.Sprintf("recovery-%d", idx+1)
		model.CreatedAt = now
		model.Data = map[string]any{"k": "v"}
		_ = model.UpdateKeys()

	case *models.ScheduledStatus:
		model.ID = fmt.Sprintf("scheduled-%d", idx+1)
		model.Username = "user-1"
		model.Status = "hello"
		model.MediaIDs = []string{"media-1", "media-2"}
		model.Sensitive = false
		model.Visibility = "public"
		model.Language = "en"
		model.ScheduledAt = now.Add(5 * time.Minute)
		model.CreatedAt = now
		model.UpdatedAt = now
		model.Published = idx%2 == 1
		_ = model.UpdateKeys()

	case *models.OAuthState:
		model.State = fmt.Sprintf("state-%d", idx+1)
		model.Provider = "github"
		model.RedirectURI = "https://example.com/callback"
		model.Username = "user-1"
		model.ClientID = "client-1"
		model.Scopes = []string{"read"}
		model.CodeChallenge = "challenge"
		model.CodeChallengeMethod = "S256"
		model.CreatedAt = now
		model.ExpiresAt = now.Add(10 * time.Minute)
		_ = model.UpdateKeys()

	case *models.WebSocketEventConnection:
		model.ConnectionID = fmt.Sprintf("conn-%d", idx+1)
		model.UserID = "user-1"
		model.ConnectedAt = now
		model.LastSeen = now
		model.TTL = now.Add(24 * time.Hour).Unix()
		_ = model.UpdateKeys()

	case *models.WebSocketEventSubscription:
		model.ConnectionID = fmt.Sprintf("conn-%d", idx+1)
		model.SubscriptionType = "notifications"
		model.Filter = map[string]any{"type": "mention"}
		model.CreatedAt = now
		model.TTL = now.Add(24 * time.Hour).Unix()
		_ = model.UpdateKeys()

	case *models.WebSocketConnection:
		model.ConnectionID = fmt.Sprintf("conn-%d", idx+1)
		model.UserID = "user-1"
		model.Username = "user-1"
		model.Streams = []string{"public"}
		model.Established = now
		model.LastActivity = now
		model.State = models.ConnectionStateConnected
		model.TTL = now.Add(24 * time.Hour).Unix()
		_ = model.UpdateKeys()

	case *models.Export:
		model.ID = fmt.Sprintf("export-%d", idx+1)
		model.Username = "user-1"
		model.Type = "archive"
		model.Format = "mastodon"
		model.Status = "completed"
		model.CreatedAt = now
		model.UpdatedAt = now
		model.UpdateKeys()

	case *models.Import:
		model.ID = fmt.Sprintf("import-%d", idx+1)
		model.Username = "user-1"
		model.Type = "followers"
		model.Mode = "merge"
		model.Status = "processing"
		model.CreatedAt = now
		model.UpdatedAt = now
		model.UpdateKeys()

	case *models.Timeline:
		model.TimelineType = "HOME"
		model.TimelineID = "user-1"
		model.EntryID = fmt.Sprintf("entry-%d", idx+1)
		model.TimelineAt = now.Add(time.Duration(idx) * time.Minute)
		model.PostID = fmt.Sprintf("post-%d", idx+1)
		model.ActorID = fmt.Sprintf("https://example.com/users/user-%d", idx+1)
		model.Visibility = "public"
		model.Language = "en"
		model.CreatedAt = now
		model.ModifiedAt = now
		_ = model.BeforeCreate()

	case *models.ConversationParticipantRecord:
		model.Conversation = &models.Conversation{
			ID:        fmt.Sprintf("conversation-%d", idx+1),
			UpdatedAt: now,
		}
		model.PK = "USER_CONVERSATIONS#user-1"
		model.SK = now.Format(time.RFC3339) + "#conversation"

	case *models.Draft:
		model.AuthorID = "user-1"
		model.ID = fmt.Sprintf("draft-%d", idx+1)
		model.ContentType = "Article"
		model.Content = "content"
		model.ContentFormat = "markdown"
		model.Status = "draft"
		model.CreatedAt = now
		model.UpdatedAt = now
		_ = model.UpdateKeys()

	case *models.Publication:
		model.ID = fmt.Sprintf("pub-%d", idx+1)
		model.Name = "Publication"
		model.Slug = "publication"
		model.ActorID = fmt.Sprintf("https://example.com/publications/%s", model.ID)
		model.CreatedAt = now
		model.UpdatedAt = now
		_ = model.UpdateKeys()

	case *models.PublicationMember:
		model.PublicationID = "pub-1"
		model.UserID = fmt.Sprintf("user-%d", idx+1)
		model.Role = "writer"
		model.DisplayName = "Display"
		model.CreatedAt = now
		model.JoinedAt = now
		model.UpdatedAt = now
		_ = model.UpdateKeys()

	case *models.Category:
		model.ID = fmt.Sprintf("cat-%d", idx+1)
		model.Name = "Category"
		model.Slug = "category"
		model.ArticleCount = idx
		model.CreatedAt = now
		model.UpdatedAt = now
		_ = model.UpdateKeys()

	case *models.Revision:
		model.ID = fmt.Sprintf("rev-%d", idx+1)
		model.ObjectID = "object-1"
		model.Version = idx + 1
		model.Content = "content"
		model.ContentHash = "hash"
		model.ChangedBy = "user-1"
		model.ChangeType = "update"
		model.CreatedAt = now
		model.UpdatedAt = now
		_ = model.UpdateKeys()

	case *models.DNSCache:
		model.Hostname = fmt.Sprintf("example-%d.com", idx+1)
		model.IPs = []string{"127.0.0.1"}
		model.ResolvedAt = now
		model.TTL = 60
		model.ExpiresAt = now.Add(1 * time.Hour).Unix()
		_ = model.UpdateKeys()

	case *models.Series:
		model.AuthorID = "user-1"
		model.ID = fmt.Sprintf("series-%d", idx+1)
		model.Title = "Series"
		model.Slug = "series"
		model.ArticleCount = idx
		model.CreatedAt = now
		model.UpdatedAt = now
		_ = model.UpdateKeys()

	case *models.Marker:
		model.Username = fmt.Sprintf("user-%d", idx+1)
		model.Timeline = "home"
		model.LastReadID = "status-1"
		model.Version = idx + 1
		model.UpdatedAt = now
		_ = model.UpdateKeys()

	case *models.Hashtag:
		model.Name = fmt.Sprintf("tag-%d", idx+1)
		model.URL = fmt.Sprintf("https://example.com/tags/%s", model.Name)
		model.UsageCount = int64(3 + idx)
		model.FirstSeen = now.Add(-2 * time.Hour)
		model.LastUsed = now
		model.CreatedAt = now.Add(-2 * time.Hour)
		model.UpdatedAt = now
		_ = model.UpdateKeys()

	case *models.HashtagFollow:
		model.UserID = "user-1"
		model.Hashtag = fmt.Sprintf("tag-%d", idx+1)
		model.NotificationsEnabled = idx%2 == 0
		model.Muted = idx%2 == 1
		model.CreatedAt = now.Add(-1 * time.Hour)
		model.UpdatedAt = now
		_ = model.UpdateKeys()

	case *models.WebAuthnCredential:
		model.ID = fmt.Sprintf("cred-%d", idx+1)
		model.UserID = "user-1"
		model.PublicKey = []byte("pk")
		model.CreatedAt = now
		model.LastUsedAt = now
		_ = model.BeforeCreate()

	case *models.WebAuthnChallenge:
		model.Challenge = fmt.Sprintf("challenge-%d", idx+1)
		model.UserID = "user-1"
		model.SessionData = []byte("session")
		model.ExpiresAt = now.Add(5 * time.Minute)
		model.Type = "authentication"
		_ = model.UpdateKeys()

	case *models.WalletCredential:
		model.Username = "user-1"
		model.Address = fmt.Sprintf("0xabc%d", idx+1)
		model.Type = "ethereum"
		model.ChainID = 1
		model.LinkedAt = now
		model.LastUsed = now
		_ = model.UpdateKeys()

	case *models.WalletChallenge:
		model.ID = fmt.Sprintf("wallet-challenge-%d", idx+1)
		model.Username = "user-1"
		model.Address = "0xabc"
		model.ChainID = 1
		model.Nonce = "nonce"
		model.Message = "message"
		model.IssuedAt = now
		model.ExpiresAt = now.Add(5 * time.Minute)
		model.RegistrationCompleted = true
		_ = model.UpdateKeys()

	case *models.WalletIndex:
		model.Username = "user-1"
		model.WalletType = "ethereum"
		model.Address = "0xabc"
		_ = model.BeforeCreate()

	case *models.Device:
		model.Username = "user-1"
		model.DeviceID = fmt.Sprintf("device-%d", idx+1)
		model.DeviceName = "device"
		model.DeviceType = "web"
		model.LastIPAddress = "127.0.0.1"
		model.LastUserAgent = "ua"
		model.CreatedAt = now
		model.LastSeenAt = now
		model.TrustLevel = "trusted"
		model.Active = true
		model.UpdateKeys()

	case *models.ProviderAccount:
		model.UserID = "user-1"
		model.Provider = "github"
		model.ProviderID = fmt.Sprintf("pid-%d", idx+1)
		model.IsActive = idx%2 == 0
		_ = model.BeforeCreate()
	}
}

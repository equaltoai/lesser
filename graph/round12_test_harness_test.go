package graph

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/services"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/equaltoai/lesser/pkg/streaming"
	pkgtesting "github.com/equaltoai/lesser/pkg/testing"
	"github.com/equaltoai/lesser/pkg/testing/inmemory"
	dynamormcore "github.com/pay-theory/dynamorm/pkg/core"
	dynamormerrors "github.com/pay-theory/dynamorm/pkg/errors"
	dynamormmocks "github.com/pay-theory/dynamorm/pkg/mocks"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type round12TestClaims struct {
	username string
}

func (c round12TestClaims) HasScope(string) bool { return true }
func (c round12TestClaims) GetUsername() string  { return c.username }

func round12AuthContext(username string) context.Context {
	return context.WithValue(context.Background(), common.ContextKeyClaims, round12TestClaims{username: username})
}

type round12PermissiveQueryState struct {
	lastPK string
	lastSK string

	autoPopulateScan  bool
	autoPopulateAll   bool
	autoPopulateCount int
	autoPopulateIndex int
}

func setupRound12PermissiveDynamormMocks(t *testing.T) (*dynamormmocks.MockDB, *dynamormmocks.MockQuery, *round12PermissiveQueryState) {
	t.Helper()

	mockDB := new(dynamormmocks.MockDB)
	mockQuery := new(dynamormmocks.MockQuery)
	mockUpdateBuilder := new(dynamormmocks.MockUpdateBuilder)
	state := &round12PermissiveQueryState{}

	mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
	mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()

	mockQuery.On("WithContext", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Index", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		field, _ := args.Get(0).(string)
		value, _ := args.Get(2).(string)
		fieldLower := strings.ToLower(field)
		if strings.HasSuffix(fieldLower, "pk") {
			state.lastPK = value
		}
		if strings.HasSuffix(fieldLower, "sk") {
			state.lastSK = value
		}
	}).Return(mockQuery).Maybe()
	mockQuery.On("Filter", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("OrFilter", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("FilterGroup", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("OrFilterGroup", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("WithCondition", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("WithConditionExpression", mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Limit", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Offset", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Cursor", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("ConsistentRead").Return(mockQuery).Maybe()
	mockQuery.On("Select", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("IfNotExists").Return(mockQuery).Maybe()
	mockQuery.On("IfExists").Return(mockQuery).Maybe()
	mockQuery.On("Create").Return(nil).Maybe()
	mockQuery.On("CreateOrUpdate").Return(nil).Maybe()
	mockQuery.On("Update").Return(nil).Maybe()
	mockQuery.On("Update", mock.Anything).Return(nil).Maybe()
	mockUpdateBuilder.On("Set", mock.Anything, mock.Anything).Return(mockUpdateBuilder).Maybe()
	mockUpdateBuilder.On("SetIfNotExists", mock.Anything, mock.Anything, mock.Anything).Return(mockUpdateBuilder).Maybe()
	mockUpdateBuilder.On("Add", mock.Anything, mock.Anything).Return(mockUpdateBuilder).Maybe()
	mockUpdateBuilder.On("Increment", mock.Anything).Return(mockUpdateBuilder).Maybe()
	mockUpdateBuilder.On("Decrement", mock.Anything).Return(mockUpdateBuilder).Maybe()
	mockUpdateBuilder.On("Remove", mock.Anything).Return(mockUpdateBuilder).Maybe()
	mockUpdateBuilder.On("Delete", mock.Anything, mock.Anything).Return(mockUpdateBuilder).Maybe()
	mockUpdateBuilder.On("AppendToList", mock.Anything, mock.Anything).Return(mockUpdateBuilder).Maybe()
	mockUpdateBuilder.On("PrependToList", mock.Anything, mock.Anything).Return(mockUpdateBuilder).Maybe()
	mockUpdateBuilder.On("RemoveFromListAt", mock.Anything, mock.Anything).Return(mockUpdateBuilder).Maybe()
	mockUpdateBuilder.On("SetListElement", mock.Anything, mock.Anything, mock.Anything).Return(mockUpdateBuilder).Maybe()
	mockUpdateBuilder.On("Condition", mock.Anything, mock.Anything, mock.Anything).Return(mockUpdateBuilder).Maybe()
	mockUpdateBuilder.On("OrCondition", mock.Anything, mock.Anything, mock.Anything).Return(mockUpdateBuilder).Maybe()
	mockUpdateBuilder.On("ConditionExists", mock.Anything).Return(mockUpdateBuilder).Maybe()
	mockUpdateBuilder.On("ConditionNotExists", mock.Anything).Return(mockUpdateBuilder).Maybe()
	mockUpdateBuilder.On("ConditionVersion", mock.Anything).Return(mockUpdateBuilder).Maybe()
	mockUpdateBuilder.On("ReturnValues", mock.Anything).Return(mockUpdateBuilder).Maybe()
	mockUpdateBuilder.On("Execute").Return(nil).Maybe()
	mockUpdateBuilder.On("ExecuteWithResult", mock.Anything).Return(nil).Maybe()
	mockQuery.On("UpdateBuilder").Return(mockUpdateBuilder).Maybe()
	mockQuery.On("Delete").Return(nil).Maybe()
	mockQuery.On("Delete", mock.Anything).Return(nil).Maybe()
	mockQuery.On("Count").Return(int64(0), nil).Maybe()
	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		if state.autoPopulateAll {
			round12PopulateSlice(args.Get(0), state)
		}
	}).Return(nil).Maybe()
	mockQuery.On("Scan", mock.Anything).Run(func(args mock.Arguments) {
		if state.autoPopulateScan {
			round12PopulateSlice(args.Get(0), state)
		}
	}).Return(nil).Maybe()
	// Default absence: treat block/mute lookups as not-found so relationship flows don't
	// incorrectly short-circuit on "blocked" when our DB mock has no backing store.
	mockQuery.On("First", mock.MatchedBy(func(dest any) bool {
		_, ok := dest.(*models.Block)
		return ok
	})).Return(dynamormerrors.ErrItemNotFound).Maybe()
	mockQuery.On("First", mock.MatchedBy(func(dest any) bool {
		_, ok := dest.(*models.Mute)
		return ok
	})).Return(dynamormerrors.ErrItemNotFound).Maybe()
	// Follow requests are directional; default the bob->alice request to "not found" so
	// Accept/RejectFollowRequest tests cover the error path realistically.
	mockQuery.On("First", mock.MatchedBy(func(dest any) bool {
		_, ok := dest.(*models.RelationshipRecord)
		if !ok {
			return false
		}
		return state.lastPK == "FOLLOW#bob" && state.lastSK == "FOLLOWING#alice"
	})).Return(dynamormerrors.ErrItemNotFound).Maybe()
	// Quote permissions: return "not found" for a dedicated test username so callers
	// can exercise create-then-update flows deterministically.
	mockQuery.On("First", mock.MatchedBy(func(dest any) bool {
		_, ok := dest.(*models.QuotePermissions)
		if !ok {
			return false
		}
		return state.lastPK == "USER#missing-perms" && state.lastSK == "QUOTE_PERMISSIONS"
	})).Return(dynamormerrors.ErrItemNotFound).Maybe()
	// Emoji: allow one stable "existing" emoji for update flows, but default lookups
	// to not-found so create flows don't falsely detect duplicates.
	mockQuery.On("First", mock.MatchedBy(func(dest any) bool {
		_, ok := dest.(*models.EmojiModel)
		if !ok {
			return false
		}
		return state.lastPK == "EMOJI#existing" && state.lastSK == "EMOJI"
	})).Run(func(args mock.Arguments) {
		round12PopulateStruct(args.Get(0), state)
	}).Return(nil).Maybe()
	mockQuery.On("First", mock.MatchedBy(func(dest any) bool {
		_, ok := dest.(*models.EmojiModel)
		return ok
	})).Return(dynamormerrors.ErrItemNotFound).Maybe()
	mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
		round12PopulateStruct(args.Get(0), state)
	}).Return(nil).Maybe()

	return mockDB, mockQuery, state
}

func round12PopulateStruct(dest any, state *round12PermissiveQueryState) {
	switch v := dest.(type) {
	case *models.User:
		username := strings.TrimPrefix(state.lastPK, "USER#")
		if strings.TrimSpace(username) == "" {
			username = "admin"
		}
		v.Username = username
		if strings.EqualFold(username, "admin") {
			v.Role = adminRoleAdmin
		} else {
			v.Role = adminRoleUser
		}
		v.Suspended = false
		return
	case *models.Actor:
		username := strings.TrimPrefix(state.lastPK, "ACTOR#")
		if strings.TrimSpace(username) == "" {
			username = "alice"
		}
		v.Username = username
		v.Actor = &activitypub.Actor{
			BaseObject: activitypub.BaseObject{
				ID:   config.Get().ActorURL(username),
				Type: activitypub.PersonType,
			},
			PreferredUsername: username,
		}
		v.NumericID = common.GenerateNumericID(username)
		v.CreatedAt = time.Now().Add(-time.Hour)
		v.UpdatedAt = time.Now().Add(-time.Minute)
		return
	case *models.RelationshipRecord:
		follower := strings.TrimPrefix(state.lastPK, "FOLLOW#")
		if strings.TrimSpace(follower) == "" {
			follower = "alice"
		}
		following := strings.TrimPrefix(state.lastSK, "FOLLOWING#")
		if strings.TrimSpace(following) == "" {
			following = "bob"
		}
		v.PK = fmt.Sprintf("FOLLOW#%s", follower)
		v.SK = fmt.Sprintf("FOLLOWING#%s", following)
		v.GSI1PK = fmt.Sprintf("FOLLOW#%s", following)
		v.GSI1SK = fmt.Sprintf("FOLLOWER#%s", follower)
		v.ActivityID = "activity-1"
		v.State = models.RelationshipAccepted
		v.CreatedAt = time.Now().Add(-time.Hour)
		v.UpdatedAt = time.Now().Add(-time.Minute)
		return
	case *models.Export:
		exportID := "export-1"
		if strings.HasPrefix(state.lastPK, "EXPORT#") {
			exportID = strings.TrimPrefix(state.lastPK, "EXPORT#")
			if strings.TrimSpace(exportID) == "" {
				exportID = "export-1"
			}
		}
		v.ID = exportID
		username := "alice"
		if strings.HasPrefix(state.lastPK, "USER#") {
			username = strings.TrimPrefix(state.lastPK, "USER#")
		}
		if strings.TrimSpace(username) == "" {
			username = "alice"
		}
		v.Username = username
		v.Type = "archive"
		v.Format = "csv"
		v.Status = "completed"
		v.CreatedAt = time.Now().Add(-time.Hour)
		v.UpdatedAt = time.Now().Add(-time.Minute)
		return
	case *models.Import:
		importID := "import-1"
		if strings.HasPrefix(state.lastPK, "IMPORT#") {
			importID = strings.TrimPrefix(state.lastPK, "IMPORT#")
			if strings.TrimSpace(importID) == "" {
				importID = "import-1"
			}
		}
		v.ID = importID
		username := "alice"
		if strings.HasPrefix(state.lastPK, "USER#") {
			username = strings.TrimPrefix(state.lastPK, "USER#")
		}
		if strings.TrimSpace(username) == "" {
			username = "alice"
		}
		v.Username = username
		v.Type = "followers"
		switch importID {
		case "processing":
			v.Status = "processing"
		default:
			v.Status = "completed"
		}
		v.Progress = 1.0
		v.Total = 1
		v.SuccessCount = 1
		v.CreatedAt = time.Now().Add(-time.Hour)
		v.UpdatedAt = time.Now().Add(-time.Minute)
		return
	case *models.Media:
		mediaID := strings.TrimPrefix(state.lastPK, "media#")
		if strings.TrimSpace(mediaID) == "" {
			mediaID = "m1"
		}
		v.MediaID = mediaID
		v.UserID = "alice"
		v.ContentType = "image/jpeg"
		v.FileName = mediaID + ".jpg"
		v.FileSize = 1024
		v.Version = "original"
		v.UploadedAt = time.Now().Add(-time.Hour)
		v.CreatedAt = time.Now().Add(-time.Hour)
		v.UpdatedAt = time.Now().Add(-time.Minute)
		v.Status = "ready"
		v.Duration = 120
		v.CDNUrl = "https://cdn.local/media/" + mediaID + ".m3u8"
		v.Variants = map[string]models.MediaVariant{
			"480p":  {CDNUrl: "https://cdn.local/media/" + mediaID + "/480.m3u8", ContentType: "application/x-mpegURL", FileSize: 123},
			"1080p": {CDNUrl: "https://cdn.local/media/" + mediaID + "/1080.m3u8", ContentType: "application/x-mpegURL", FileSize: 456},
		}
		return
	case *models.CommunityNote:
		noteID := strings.TrimPrefix(state.lastPK, "NOTE#")
		if strings.TrimSpace(noteID) == "" {
			noteID = "note-1"
		}
		v.ID = noteID
		v.ObjectID = "status-1"
		v.ObjectType = "status"
		v.AuthorID = config.Get().ActorURL("bob")
		v.Content = "note content"
		v.Language = "en"
		v.VisibilityStatus = "pending"
		v.Score = 0
		v.CreatedAt = time.Now().Add(-time.Hour)
		v.UpdatedAt = time.Now().Add(-time.Minute)
		return
	case *models.PushSubscription:
		username := strings.TrimPrefix(state.lastPK, "PUSH#")
		if strings.TrimSpace(username) == "" {
			username = "alice"
		}
		id := "sub-1"
		if strings.HasPrefix(state.lastSK, "SUB#") {
			id = strings.TrimPrefix(state.lastSK, "SUB#")
			if strings.TrimSpace(id) == "" {
				id = "sub-1"
			}
		}
		v.Username = username
		v.ID = id
		v.Endpoint = "https://push.example/endpoint/" + id
		v.Auth = "auth-" + id
		v.P256dh = "p256dh-" + id
		v.Policy = "all"
		v.Alerts = models.PushSubscriptionAlerts{
			Follow:        true,
			Favourite:     false,
			Reblog:        true,
			Mention:       true,
			Poll:          false,
			FollowRequest: true,
			Status:        true,
			Update:        false,
			AdminSignUp:   false,
			AdminReport:   false,
		}
		v.CreatedAt = time.Now().Add(-time.Hour)
		v.UpdatedAt = time.Now().Add(-time.Minute)
		return
	case *models.VAPIDKeyRecord:
		v.PK = "INSTANCE#CONFIG"
		v.SK = "VAPID_KEYS"
		v.Data = storage.VAPIDKeys{
			PublicKey:  "test-public-key",
			PrivateKey: "test-private-key",
			Subject:    "mailto:admin@localhost",
			CreatedAt:  time.Now().Add(-time.Hour).UTC(),
			UpdatedAt:  time.Now().Add(-time.Minute).UTC(),
		}
		v.UpdatedAt = time.Now().Add(-time.Minute)
		return
	case *models.EmojiModel:
		shortcode := strings.TrimPrefix(state.lastPK, "EMOJI#")
		shortcode = strings.TrimSpace(strings.TrimSuffix(shortcode, "@localhost"))
		if strings.TrimSpace(shortcode) == "" {
			shortcode = "existing"
		}
		v.Shortcode = shortcode
		v.URL = "https://cdn.local/emoji/" + shortcode + ".png"
		v.StaticURL = v.URL
		v.VisibleInPicker = true
		v.Category = "custom"
		v.Domain = ""
		v.Disabled = false
		v.CreatedAt = time.Now().Add(-24 * time.Hour)
		v.UpdatedAt = time.Now().Add(-time.Hour)
		_ = v.UpdateKeys()
		return
	case *models.ScheduledStatus:
		username := "alice"
		if strings.HasPrefix(state.lastPK, "USER#") {
			trimmed := strings.TrimPrefix(state.lastPK, "USER#")
			if idx := strings.Index(trimmed, "#"); idx >= 0 {
				if candidate := strings.TrimSpace(trimmed[:idx]); candidate != "" {
					username = candidate
				}
			} else if candidate := strings.TrimSpace(trimmed); candidate != "" {
				username = candidate
			}
		}

		id := strings.TrimPrefix(state.lastSK, "ID#")
		if strings.TrimSpace(id) == "" {
			id = fmt.Sprintf("sched_%d", state.autoPopulateIndex+1)
		}

		v.ID = id
		v.Username = username
		v.Status = "scheduled status"
		v.Visibility = "public"
		v.Language = "en"
		v.Sensitive = false
		v.ScheduledAt = time.Now().Add(time.Duration(state.autoPopulateIndex+1) * time.Hour)
		v.CreatedAt = time.Now().Add(-24 * time.Hour)
		v.UpdatedAt = time.Now().Add(-time.Hour)
		v.Published = false
		_ = v.UpdateKeys()
		return
	case *models.List:
		username := "alice"
		if strings.HasPrefix(state.lastPK, "USER_LISTS#") {
			if candidate := strings.TrimPrefix(state.lastPK, "USER_LISTS#"); strings.TrimSpace(candidate) != "" {
				username = candidate
			}
		}

		v.Username = username
		if strings.HasPrefix(state.lastPK, "LIST#") {
			v.ID = strings.TrimPrefix(state.lastPK, "LIST#")
		}
		if strings.TrimSpace(v.ID) == "" {
			v.ID = fmt.Sprintf("list_%d", state.autoPopulateIndex+1)
		}
		v.Title = "Test List"
		if strings.TrimSpace(v.RepliesPolicy) == "" {
			v.RepliesPolicy = "list"
		}
		v.Exclusive = false
		if v.CreatedAt.IsZero() {
			v.CreatedAt = time.Now().Add(-time.Hour)
		}
		v.UpdatedAt = time.Now()
		_ = v.UpdateKeys()
		return
	case *models.Conversation:
		username := "alice"
		if strings.HasPrefix(state.lastPK, "USER_CONVERSATIONS#") {
			if candidate := strings.TrimPrefix(state.lastPK, "USER_CONVERSATIONS#"); strings.TrimSpace(candidate) != "" {
				username = candidate
			}
		}

		if strings.HasPrefix(state.lastPK, "CONVERSATION#") {
			v.ID = strings.TrimPrefix(state.lastPK, "CONVERSATION#")
		}
		if strings.TrimSpace(v.ID) == "" {
			v.ID = fmt.Sprintf("conv_%d", state.autoPopulateIndex+1)
		}
		v.Participants = []string{username, "bob"}
		v.LastStatusID = "status-1"
		v.Unread = true
		if v.CreatedAt.IsZero() {
			v.CreatedAt = time.Now().Add(-time.Hour)
		}
		v.UpdatedAt = time.Now()
		v.LastMessageTime = v.UpdatedAt
		v.TotalMessageCount = 1
		_ = v.UpdateKeys()
		return
	case *models.ConversationParticipantRecord:
		if v.Conversation == nil {
			v.Conversation = &models.Conversation{}
		}
		round12PopulateStruct(v.Conversation, state)
		return
	case *models.Announcement:
		id := fmt.Sprintf("announcement-%d", state.autoPopulateIndex+1)
		v.ID = id
		v.Content = "<p>Hello</p>"
		v.Text = "Hello"
		v.PublishedAt = time.Now().Add(-time.Duration(state.autoPopulateIndex) * time.Hour)
		v.UpdatedAt = v.PublishedAt.Add(5 * time.Minute)
		v.AllDay = false
		if state.autoPopulateIndex%2 == 0 {
			start := time.Now().Add(10 * time.Minute)
			end := start.Add(1 * time.Hour)
			v.StartsAt = &start
			v.EndsAt = &end
		}
		// Provide a stable reaction list to exercise "available reactions" paths.
		v.Reactions = []models.Reaction{
			{Name: "👍"},
			{Name: ":custom:", URL: "https://cdn.local/custom.png", StaticURL: "https://cdn.local/custom.png"},
		}
		v.CreatedBy = "admin"
		_ = v.UpdateKeys()
		return
	case *models.AnnouncementDismissal:
		username := strings.TrimPrefix(state.lastPK, "USER#")
		if strings.TrimSpace(username) == "" {
			username = "alice"
		}
		v.Username = username
		// Dismiss the first auto-populated announcement by default.
		v.AnnouncementID = "announcement-1"
		v.DismissedAt = time.Now().Add(-time.Minute)
		_ = v.UpdateKeys()
		return
	case *models.AnnouncementReaction:
		announcementID := strings.TrimPrefix(state.lastPK, "ANNOUNCEMENT_REACTION#")
		if strings.TrimSpace(announcementID) == "" {
			announcementID = "announcement-1"
		}
		v.AnnouncementID = announcementID
		v.Username = "alice"
		v.EmojiName = "👍"
		v.ReactedAt = time.Now().Add(-time.Minute)
		_ = v.UpdateKeys()
		return
	default:
		// Best-effort generic population for pointer-to-struct destinations.
		rv := reflect.ValueOf(dest)
		if rv.Kind() != reflect.Ptr || rv.IsNil() {
			return
		}
		// Support ORM-style pointer-to-pointer destinations (e.g. **T), allocating the inner value
		// so downstream code doesn't panic when a mock "First" returns nil error.
		if rv.Elem().Kind() == reflect.Ptr {
			if rv.Elem().IsNil() && rv.Elem().Type().Elem().Kind() == reflect.Struct {
				rv.Elem().Set(reflect.New(rv.Elem().Type().Elem()))
			}
			return
		}
		if rv.Elem().Kind() != reflect.Struct {
			return
		}
	}
}

func round12PopulateSlice(dest any, state *round12PermissiveQueryState) {
	rv := reflect.ValueOf(dest)
	if rv.Kind() != reflect.Ptr || rv.IsNil() {
		return
	}
	slice := rv.Elem()
	if slice.Kind() != reflect.Slice {
		return
	}
	if slice.Len() > 0 {
		return
	}

	count := state.autoPopulateCount
	if count <= 0 {
		count = 2
	}

	out := reflect.MakeSlice(slice.Type(), 0, count)
	for i := 0; i < count; i++ {
		elemType := slice.Type().Elem()
		switch elemType.Kind() {
		case reflect.Ptr:
			state.autoPopulateIndex = i
			elem := reflect.New(elemType.Elem())
			round12PopulateStruct(elem.Interface(), state)
			// Apply stable ordering differences across list items.
			if i > 0 {
				if createdAtField := elem.Elem().FieldByName("CreatedAt"); createdAtField.IsValid() && createdAtField.CanSet() && createdAtField.Type() == reflect.TypeOf(time.Time{}) {
					createdAtField.Set(reflect.ValueOf(time.Now().Add(-time.Duration(i) * time.Hour)))
				}
			}
			out = reflect.Append(out, elem)
		case reflect.Struct:
			state.autoPopulateIndex = i
			elem := reflect.New(elemType)
			round12PopulateStruct(elem.Interface(), state)
			// Apply stable ordering differences across list items.
			if i > 0 {
				if createdAtField := elem.Elem().FieldByName("CreatedAt"); createdAtField.IsValid() && createdAtField.CanSet() && createdAtField.Type() == reflect.TypeOf(time.Time{}) {
					createdAtField.Set(reflect.ValueOf(time.Now().Add(-time.Duration(i) * time.Hour)))
				}
			}
			out = reflect.Append(out, elem.Elem())
		case reflect.String:
			out = reflect.Append(out, reflect.ValueOf(fmt.Sprintf("item-%d", i+1)))
		default:
			// Unsupported slice element kinds; leave empty.
			return
		}
	}

	slice.Set(out)
}

type round12ArticleRepoWithDB struct {
	*inmemory.ArticleRepository
	db dynamormcore.DB
}

func (r *round12ArticleRepoWithDB) GetDB() dynamormcore.DB { return r.db }

type round12CategoryRepoWithDB struct {
	*inmemory.CategoryRepository
	db dynamormcore.DB
}

func (r *round12CategoryRepoWithDB) GetDB() dynamormcore.DB { return r.db }

type round12PublicationRepoWithDB struct {
	*inmemory.PublicationRepository
	db dynamormcore.DB
}

func (r *round12PublicationRepoWithDB) GetDB() dynamormcore.DB { return r.db }

type round12GraphStorage struct {
	*pkgtesting.MockRepositoryStorage

	db           dynamormcore.DB
	accountRepo  *repositories.AccountRepository
	bookmarkRepo *repositories.BookmarkRepository
	mediaRepo    *repositories.MediaRepository

	markerRepo *repositories.MarkerRepository

	communityNoteRepo *repositories.CommunityNoteRepository
	costRepo          *repositories.TrackingRepository
	analyticsRepo     *repositories.TrendingRepository

	notificationRepo     interfaces.NotificationRepository
	pushSubscriptionRepo *repositories.PushSubscriptionRepository
	pollRepo             *repositories.PollRepository
	scheduledStatusRepo  *repositories.ScheduledStatusRepository

	threadRepo *repositories.ThreadRepository

	domainBlockRepo  *repositories.DomainBlockRepository
	federationRepo   *repositories.FederationRepository
	filterRepo       *repositories.FilterRepository
	importRepo       *repositories.ImportRepository
	exportRepo       *repositories.ExportRepository
	instanceRepo     *repositories.InstanceRepository
	listRepo         *repositories.ListRepository
	quoteRepo        *repositories.QuoteRepository
	searchRepo       *repositories.SearchRepository
	hashtagRepo      *repositories.HashtagRepository
	emojiRepo        *repositories.EmojiRepository
	socialRepo       *repositories.SocialRepository
	conversationRepo *repositories.ConversationRepository

	relationshipRepo interfaces.ConcreteRelationshipRepository
	announcementRepo *repositories.AnnouncementRepository
	likeRepo         *repositories.LikeRepository
}

func (s *round12GraphStorage) GetDB() dynamormcore.DB { return s.db }
func (s *round12GraphStorage) Account() *repositories.AccountRepository {
	return s.accountRepo
}
func (s *round12GraphStorage) Notification() interfaces.NotificationRepository {
	return s.notificationRepo
}
func (s *round12GraphStorage) Bookmark() *repositories.BookmarkRepository {
	return s.bookmarkRepo
}
func (s *round12GraphStorage) Media() *repositories.MediaRepository {
	return s.mediaRepo
}
func (s *round12GraphStorage) PushSubscription() *repositories.PushSubscriptionRepository {
	return s.pushSubscriptionRepo
}
func (s *round12GraphStorage) Poll() *repositories.PollRepository {
	return s.pollRepo
}
func (s *round12GraphStorage) ScheduledStatus() *repositories.ScheduledStatusRepository {
	return s.scheduledStatusRepo
}
func (s *round12GraphStorage) Thread() *repositories.ThreadRepository {
	return s.threadRepo
}
func (s *round12GraphStorage) Marker() *repositories.MarkerRepository {
	return s.markerRepo
}
func (s *round12GraphStorage) CommunityNote() *repositories.CommunityNoteRepository {
	return s.communityNoteRepo
}
func (s *round12GraphStorage) Cost() *repositories.TrackingRepository {
	return s.costRepo
}
func (s *round12GraphStorage) Analytics() *repositories.TrendingRepository {
	return s.analyticsRepo
}
func (s *round12GraphStorage) DomainBlock() *repositories.DomainBlockRepository {
	return s.domainBlockRepo
}
func (s *round12GraphStorage) Federation() *repositories.FederationRepository {
	return s.federationRepo
}
func (s *round12GraphStorage) Filter() *repositories.FilterRepository {
	return s.filterRepo
}
func (s *round12GraphStorage) Import() *repositories.ImportRepository {
	return s.importRepo
}
func (s *round12GraphStorage) Export() *repositories.ExportRepository {
	return s.exportRepo
}
func (s *round12GraphStorage) Instance() *repositories.InstanceRepository {
	return s.instanceRepo
}
func (s *round12GraphStorage) List() *repositories.ListRepository {
	return s.listRepo
}
func (s *round12GraphStorage) Quote() *repositories.QuoteRepository {
	return s.quoteRepo
}
func (s *round12GraphStorage) Search() *repositories.SearchRepository {
	return s.searchRepo
}
func (s *round12GraphStorage) Hashtag() *repositories.HashtagRepository {
	return s.hashtagRepo
}
func (s *round12GraphStorage) Emoji() *repositories.EmojiRepository {
	return s.emojiRepo
}
func (s *round12GraphStorage) Social() *repositories.SocialRepository {
	return s.socialRepo
}
func (s *round12GraphStorage) Conversation() *repositories.ConversationRepository {
	return s.conversationRepo
}
func (s *round12GraphStorage) Relationship() interfaces.ConcreteRelationshipRepository {
	return s.relationshipRepo
}
func (s *round12GraphStorage) Announcement() *repositories.AnnouncementRepository {
	return s.announcementRepo
}
func (s *round12GraphStorage) Like() *repositories.LikeRepository {
	return s.likeRepo
}

type round12SearchDeps struct {
	relationships interfaces.ConcreteRelationshipRepository
}

func (d *round12SearchDeps) GetFollowing(ctx context.Context, username string, limit int, cursor string) ([]string, string, error) {
	return d.relationships.GetFollowing(ctx, username, limit, cursor)
}

func (d *round12SearchDeps) IsBlocked(ctx context.Context, blockerActor, blockedActor string) (bool, error) {
	return d.relationships.IsBlocked(ctx, blockerActor, blockedActor)
}

func (d *round12SearchDeps) IsBlockedBidirectional(ctx context.Context, actor1, actor2 string) (bool, error) {
	return d.relationships.IsBlockedBidirectional(ctx, actor1, actor2)
}

func (d *round12SearchDeps) GetFollowers(ctx context.Context, username string, limit int, cursor string) ([]string, string, error) {
	return d.relationships.GetFollowers(ctx, username, limit, cursor)
}

func newRound12GraphResolverWithMocks(t *testing.T) (*Resolver, *round12GraphStorage, *dynamormmocks.MockDB, *dynamormmocks.MockQuery, *round12PermissiveQueryState) {
	t.Helper()

	mockDB, mockQuery, state := setupRound12PermissiveDynamormMocks(t)

	cfg := &config.Config{
		Domain:                        "localhost",
		InstanceMode:                  config.InstanceModeHybrid,
		IntegrationTestMode:           true,
		CMSLongFormPublishingEnabled:  true,
		CMSDraftSystemEnabled:         true,
		CMSRevisionHistoryEnabled:     true,
		CMSScheduledPublishingEnabled: true,
		CMSSeriesEnabled:              true,
		CMSCategoriesEnabled:          true,
	}

	articleRepo := &round12ArticleRepoWithDB{ArticleRepository: inmemory.NewArticleRepository(), db: mockDB}
	categoryRepo := &round12CategoryRepoWithDB{CategoryRepository: inmemory.NewCategoryRepository(), db: mockDB}
	publicationRepo := &round12PublicationRepoWithDB{PublicationRepository: inmemory.NewPublicationRepository(), db: mockDB}

	base := pkgtesting.NewMockRepositoryStorage(
		pkgtesting.WithArticleRepository(articleRepo),
		pkgtesting.WithCategoryRepository(categoryRepo),
		pkgtesting.WithPublicationRepository(publicationRepo),
		pkgtesting.WithLogger(zap.NewNop()),
	)

	tableName := base.GetTableName()
	accountRepo := repositories.NewAccountRepository(mockDB, tableName, "localhost", zap.NewNop())
	notificationRepo := inmemory.NewNotificationRepository()
	bookmarkRepo := repositories.NewBookmarkRepository(mockDB, tableName, zap.NewNop())
	mediaRepo := repositories.NewMediaRepository(mockDB, tableName, zap.NewNop(), nil)
	pollRepo := repositories.NewPollRepository(mockDB, tableName, zap.NewNop(), nil)
	scheduledStatusRepo := repositories.NewScheduledStatusRepository(mockDB, tableName, zap.NewNop(), nil)
	scheduledStatusRepo.SetMediaRepository(mediaRepo)
	pushSubscriptionRepo := repositories.NewPushSubscriptionRepository(mockDB, tableName, zap.NewNop(), nil, nil, "", "mailto:admin@localhost")
	markerRepo := repositories.NewMarkerRepository(mockDB, tableName, zap.NewNop(), nil)
	communityNoteRepo := repositories.NewCommunityNoteRepository(mockDB, tableName, zap.NewNop(), nil)
	costRepo := repositories.NewTrackingRepository(mockDB, tableName, zap.NewNop(), nil)
	analyticsRepo := repositories.NewTrendingRepository(mockDB, zap.NewNop(), nil)
	threadRepo := repositories.NewThreadRepository(mockDB, zap.NewNop())

	relationshipRepo := repositories.NewRelationshipRepository(mockDB, tableName, zap.NewNop())
	searchRepo := repositories.NewSearchRepository(mockDB, tableName, zap.NewNop(), nil)
	searchRepo.SetDependencies(&round12SearchDeps{relationships: relationshipRepo})

	storage := &round12GraphStorage{
		MockRepositoryStorage: base,
		db:                    mockDB,
		accountRepo:           accountRepo,
		notificationRepo:      notificationRepo,
		bookmarkRepo:          bookmarkRepo,
		mediaRepo:             mediaRepo,
		pollRepo:              pollRepo,
		scheduledStatusRepo:   scheduledStatusRepo,
		threadRepo:            threadRepo,
		pushSubscriptionRepo:  pushSubscriptionRepo,
		markerRepo:            markerRepo,
		communityNoteRepo:     communityNoteRepo,
		costRepo:              costRepo,
		analyticsRepo:         analyticsRepo,
		announcementRepo:      repositories.NewAnnouncementRepository(mockDB, tableName, zap.NewNop()),
		likeRepo:              repositories.NewLikeRepository(mockDB, tableName, zap.NewNop()),
		conversationRepo:      repositories.NewConversationRepository(mockDB, tableName, zap.NewNop(), nil),
		socialRepo:            repositories.NewSocialRepository(mockDB, tableName, zap.NewNop(), nil),
		domainBlockRepo:       repositories.NewDomainBlockRepository(mockDB, tableName, zap.NewNop()),
		federationRepo:        repositories.NewFederationRepository(mockDB, tableName, zap.NewNop(), nil, cfg),
		filterRepo:            repositories.NewFilterRepository(mockDB, tableName, zap.NewNop(), nil),
		importRepo:            repositories.NewImportRepository(mockDB, tableName, zap.NewNop()),
		exportRepo:            repositories.NewExportRepository(mockDB, tableName, zap.NewNop()),
		instanceRepo:          repositories.NewInstanceRepository(mockDB, tableName, zap.NewNop()),
		listRepo:              repositories.NewListRepository(mockDB, tableName, zap.NewNop(), nil),
		quoteRepo:             repositories.NewQuoteRepository(mockDB, tableName, zap.NewNop(), nil),
		hashtagRepo:           repositories.NewHashtagRepository(mockDB, tableName, zap.NewNop(), "localhost"),
		emojiRepo:             repositories.NewEmojiRepository(mockDB, tableName, zap.NewNop(), nil),
		searchRepo:            searchRepo,
		relationshipRepo:      relationshipRepo,
	}

	registry, err := services.NewRegistry(
		services.WithStorage(storage),
		services.WithPublisher(streaming.NewMockPublisher()),
		services.WithLogger(zap.NewNop()),
		services.WithConfig(&services.ServiceConfig{
			BaseURL:   "https://localhost",
			JWTSecret: strings.Repeat("x", 32),
			Config:    cfg,
		}),
	)
	require.NoError(t, err)

	resolver := &Resolver{
		Registry: registry,
		Config:   cfg,
		Storage:  storage,
		Logger:   zap.NewNop(),
	}

	return resolver, storage, mockDB, mockQuery, state
}

func newRound12GraphResolver(t *testing.T) (*Resolver, *round12GraphStorage) {
	t.Helper()

	resolver, storage, _, _, _ := newRound12GraphResolverWithMocks(t)
	return resolver, storage
}

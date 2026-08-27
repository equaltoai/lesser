package notes

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
	pkgerrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/services/notifications"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/equaltoai/lesser/pkg/streaming"
	testinginmemory "github.com/equaltoai/lesser/pkg/testing/inmemory"
	testingmocks "github.com/equaltoai/lesser/pkg/testing/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/v3/pkg/core"
	dynamormerrors "github.com/theory-cloud/tabletheory/v3/pkg/errors"
	"github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	"go.uber.org/zap"
)

type transactionalMockDB struct {
	*mocks.MockDB
}

func (db transactionalMockDB) TransactWrite(ctx context.Context, fn func(core.TransactionBuilder) error) error {
	_ = fn(noopTransactionBuilder{})
	return nil
}

type failingTransactDB struct {
	*mocks.MockDB
	err error
}

func (db failingTransactDB) TransactWrite(context.Context, func(core.TransactionBuilder) error) error {
	return db.err
}

type noopTransactionBuilder struct{}

func (noopTransactionBuilder) Put(_ any, _ ...core.TransactCondition) core.TransactionBuilder {
	return noopTransactionBuilder{}
}
func (noopTransactionBuilder) Create(_ any, _ ...core.TransactCondition) core.TransactionBuilder {
	return noopTransactionBuilder{}
}
func (noopTransactionBuilder) Update(_ any, _ []string, _ ...core.TransactCondition) core.TransactionBuilder {
	return noopTransactionBuilder{}
}
func (noopTransactionBuilder) UpdateWithBuilder(_ any, _ func(core.UpdateBuilder) error, _ ...core.TransactCondition) core.TransactionBuilder {
	return noopTransactionBuilder{}
}
func (noopTransactionBuilder) Delete(_ any, _ ...core.TransactCondition) core.TransactionBuilder {
	return noopTransactionBuilder{}
}
func (noopTransactionBuilder) ConditionCheck(_ any, _ ...core.TransactCondition) core.TransactionBuilder {
	return noopTransactionBuilder{}
}
func (noopTransactionBuilder) WithContext(_ context.Context) core.TransactionBuilder {
	return noopTransactionBuilder{}
}
func (noopTransactionBuilder) Execute() error                             { return nil }
func (noopTransactionBuilder) ExecuteWithContext(_ context.Context) error { return nil }

type permissiveQueryState struct {
	lastPK string
	lastSK string
}

func setupPermissiveDynamormMocks(mockDB *mocks.MockDB, mockQuery *mocks.MockQuery, mockUpdateBuilder *mocks.MockUpdateBuilder, state *permissiveQueryState) {
	mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
	mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()

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
	mockQuery.On("FilterGroup", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Limit", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Cursor", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("ConsistentRead").Return(mockQuery).Maybe()
	mockQuery.On("Select", mock.Anything).Return(mockQuery).Maybe()

	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		populateSlice(args.Get(0), state)
	}).Return(nil).Maybe()
	// Wave #1469 page-capped walks iterate with AllPaginated instead of a bare
	// All; populate the destination and report no more pages by default.
	mockQuery.On("AllPaginated", mock.Anything).Run(func(args mock.Arguments) {
		populateSlice(args.Get(0), state)
	}).Return(&core.PaginatedResult{HasMore: false}, nil).Maybe()
	mockQuery.On("Scan", mock.Anything).Run(func(args mock.Arguments) {
		populateSlice(args.Get(0), state)
	}).Return(nil).Maybe()
	mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
		populateStruct(args.Get(0), state)
	}).Return(nil).Maybe()

	mockQuery.On("Create").Return(nil).Maybe()
	mockQuery.On("Update").Return(nil).Maybe()
	mockQuery.On("Update", mock.Anything).Return(nil).Maybe()
	mockQuery.On("Delete").Return(nil).Maybe()
	mockQuery.On("Delete", mock.Anything).Return(nil).Maybe()
	mockQuery.On("Count").Return(int64(2), nil).Maybe()
	mockQuery.On("BatchGet", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		populateSlice(args.Get(1), state)
	}).Return(nil).Maybe()

	if mockUpdateBuilder != nil {
		mockQuery.On("UpdateBuilder").Return(mockUpdateBuilder).Maybe()
		mockUpdateBuilder.On("Set", mock.Anything, mock.Anything).Return(mockUpdateBuilder).Maybe()
		mockUpdateBuilder.On("SetIfNotExists", mock.Anything, mock.Anything, mock.Anything).Return(mockUpdateBuilder).Maybe()
		mockUpdateBuilder.On("Remove", mock.Anything).Return(mockUpdateBuilder).Maybe()
		mockUpdateBuilder.On("Add", mock.Anything, mock.Anything).Return(mockUpdateBuilder).Maybe()
		mockUpdateBuilder.On("Condition", mock.Anything, mock.Anything, mock.Anything).Return(mockUpdateBuilder).Maybe()
		mockUpdateBuilder.On("Execute").Return(nil).Maybe()
	}
}

func populateSlice(target any, state *permissiveQueryState) {
	v := reflect.ValueOf(target)
	if v.Kind() != reflect.Ptr || v.Elem().Kind() != reflect.Slice {
		return
	}

	sliceValue := v.Elem()
	elemType := sliceValue.Type().Elem()
	baseElemType := elemType
	if baseElemType.Kind() == reflect.Ptr {
		baseElemType = baseElemType.Elem()
	}
	if baseElemType.Kind() == reflect.Interface {
		return
	}

	for i := range 2 {
		at := time.Date(2025, 1, i+1, 0, 0, 0, 0, time.UTC)

		var element reflect.Value
		if elemType.Kind() == reflect.Ptr {
			element = reflect.New(baseElemType)
			populateStructWithTime(element.Interface(), state, at)
		} else {
			ptr := reflect.New(baseElemType)
			populateStructWithTime(ptr.Interface(), state, at)
			element = ptr.Elem()
		}

		sliceValue = reflect.Append(sliceValue, element)
	}

	v.Elem().Set(sliceValue)
}

func populateStruct(target any, state *permissiveQueryState) {
	populateStructWithTime(target, state, time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
}

func populateStructWithTime(target any, state *permissiveQueryState, at time.Time) {
	switch model := target.(type) {
	case *models.Status:
		statusID := trimStatusKey(state.lastPK)
		if statusID == "" {
			statusID = trimStatusKey(state.lastSK)
		}
		if statusID == "" {
			statusID = fmt.Sprintf("status-%d", at.Day())
		}

		model.StatusID = statusID
		model.PK = fmt.Sprintf("status#%s", statusID)
		model.SK = fmt.Sprintf("status#%s", statusID)
		model.AuthorID = "https://example.com/users/alice"
		model.AuthorUsername = "alice"
		model.Content = "hello"
		model.PublishedAt = at
		model.CreatedAt = at
		model.UpdatedAt = at
		model.ModifiedAt = at
		model.Visibility = models.VisibilityPublic

		if strings.Contains(strings.ToLower(state.lastPK), "conversation#") {
			model.Visibility = models.VisibilityDirect
		}

		lowerID := strings.ToLower(statusID)
		skipNote := false
		if strings.Contains(lowerID, "missing-author") {
			model.AuthorID = ""
			model.AuthorUsername = ""
			skipNote = true
		}
		switch {
		case strings.Contains(lowerID, "private"):
			model.Visibility = models.VisibilityPrivate
		case strings.Contains(lowerID, "unlisted"):
			model.Visibility = models.VisibilityUnlisted
		case strings.Contains(lowerID, "direct"):
			model.Visibility = models.VisibilityDirect
		}

		if strings.Contains(lowerID, "deleted") {
			model.Deleted = true
		}

		if strings.Contains(lowerID, "mentionbob") {
			model.Mentions = []string{"bob"}
		}

		model.ToRecipients = []string{"https://www.w3.org/ns/activitystreams#Public"}
		model.CcRecipients = []string{"https://example.com/users/alice/followers"}

		if !skipNote {
			model.Note = &activitypub.Note{
				BaseObject: activitypub.BaseObject{
					Type: "Note",
					ID:   fmt.Sprintf("https://example.com/users/%s/statuses/%s", model.AuthorUsername, statusID),
					To:   model.ToRecipients,
					CC:   model.CcRecipients,
				},
				AttributedTo: fmt.Sprintf("https://example.com/users/%s", model.AuthorUsername),
				Content:      model.Content,
				Visibility:   model.Visibility,
			}
		}
		_ = model.UpdateKeys()
	case *models.RelationshipRecord:
		model.PK = "FOLLOW#bob"
		model.SK = "FOLLOWING#alice"
		model.State = models.RelationshipAccepted
	case *models.Bookmark:
		model.Username = "alice"
		model.ObjectID = fmt.Sprintf("status-%d", at.Day())
		model.CreatedAt = at
		model.Locked = false
		model.RecordType = models.BookmarkRecordTypeTime
		_ = model.UpdateKeys()
	case *models.Like:
		model.Actor = fmt.Sprintf("https://example.com/users/user-%d", at.Day())
		if at.Day()%2 == 0 {
			model.Object = fmt.Sprintf("https://example.com/users/alice/statuses/fav-%d", at.Day())
		} else {
			model.Object = fmt.Sprintf("fav-%d", at.Day())
		}
	case *models.SearchSuggestion:
		model.Type = "hashtag"
		model.Term = fmt.Sprintf("go-%d", at.Day())
		model.Score = 0.75
	case *models.Conversation:
		model.ID = fmt.Sprintf("conv-%d", at.Day())
	case *models.UpdateHistory:
		model.ObjectID = fmt.Sprintf("object-%d", at.Day())
		model.Version = at.Day()
		model.UpdatedAt = at
		model.UpdatedBy = "alice"
		model.PreviousState = `{"content":"old"}`
		model.Summary = "update"
	case *models.CommunityNote:
		model.ID = fmt.Sprintf("note-%d", at.Day())
		model.ObjectID = "object-1"
		model.ObjectType = "status"
		model.AuthorID = "alice"
		model.Content = "context"
		model.CreatedAt = at
		model.UpdatedAt = at
	case *models.CommunityNoteVote:
		model.NoteID = "note-1"
		model.VoterID = fmt.Sprintf("user-%d", at.Day())
		model.VoteType = "helpful"
		model.Helpful = true
		model.Weight = 1
		model.CreatedAt = at
	case *models.User:
		username := strings.TrimPrefix(state.lastPK, "USER#")
		if username == "" {
			username = "alice"
		}
		model.Username = username
		if username == "admin" {
			model.Role = "admin"
		}
		model.CreatedAt = at
		model.UpdatedAt = at
	}
}

type stubAccountRepo struct {
	*repositories.AccountRepository
	domain    string
	accounts  map[string]*storage.Account
	missing   map[string]bool
	omitActor map[string]bool
}

func (r *stubAccountRepo) GetAccount(_ context.Context, id string) (*storage.Account, error) {
	username := strings.TrimSpace(id)
	if username == "" {
		return nil, pkgerrors.ItemNotFound("account")
	}

	if strings.Contains(username, "/users/") {
		parts := strings.Split(username, "/users/")
		if len(parts) == 2 {
			username = strings.Split(parts[1], "/")[0]
		}
	}

	if strings.Contains(username, "://") || strings.Contains(username, "/") {
		username = strings.TrimSuffix(username, "/")
		parts := strings.Split(username, "/")
		username = parts[len(parts)-1]
	}

	if r.missing != nil && r.missing[username] {
		return nil, pkgerrors.ItemNotFound("account")
	}

	if r.accounts != nil {
		if account, ok := r.accounts[username]; ok {
			return account, nil
		}
	}

	account := &storage.Account{
		User: &storage.User{Username: username},
	}
	if r.omitActor == nil || !r.omitActor[username] {
		account.Actor = &activitypub.Actor{
			BaseObject: activitypub.BaseObject{ID: fmt.Sprintf("https://%s/users/%s", r.domain, username)},
		}
	}
	return account, nil
}

type fixedAccountRepo struct {
	*repositories.AccountRepository
	username string
}

func (r *fixedAccountRepo) GetAccount(context.Context, string) (*storage.Account, error) {
	username := r.username
	if username == "" {
		username = "fallback"
	}
	return &storage.Account{User: &storage.User{Username: username}}, nil
}

type delegatingStatusRepo struct {
	interfaces.StatusRepository
	getStatus func(context.Context, string) (*models.Status, error)
}

func (r *delegatingStatusRepo) GetStatus(ctx context.Context, statusID string) (*models.Status, error) {
	if r != nil && r.getStatus != nil {
		return r.getStatus(ctx, statusID)
	}
	return r.StatusRepository.GetStatus(ctx, statusID)
}

type stubAnalytics struct {
	hashtagCalls  int
	instanceCalls int
}

func (s *stubAnalytics) RecordStatusCreation(context.Context, string, time.Time) error { return nil }
func (s *stubAnalytics) RecordHashtagUsage(context.Context, []string, string, string) error {
	s.hashtagCalls++
	return nil
}
func (s *stubAnalytics) RecordLinkShare(context.Context, []string, string, string) error { return nil }
func (s *stubAnalytics) RecordEngagement(context.Context, string, string, string) error  { return nil }
func (s *stubAnalytics) RecordInstanceActivity(context.Context, string, time.Time) error {
	s.instanceCalls++
	return nil
}

type stubSocialRepo struct {
	announces map[string]*storage.Announce
	pins      map[string]*storage.StatusPin
}

func (s *stubSocialRepo) CreateBlock(_ context.Context, _ *storage.Block) error { return nil }
func (s *stubSocialRepo) DeleteBlock(_ context.Context, _, _ string) error      { return nil }
func (s *stubSocialRepo) GetBlock(_ context.Context, _, _ string) (*storage.Block, error) {
	return nil, pkgerrors.ItemNotFound("block")
}
func (s *stubSocialRepo) IsBlocked(_ context.Context, _, _ string) (bool, error) { return false, nil }
func (s *stubSocialRepo) GetBlockedUsers(_ context.Context, _ string, _ int, _ string) ([]*storage.Block, string, error) {
	return nil, "", nil
}
func (s *stubSocialRepo) GetBlockedByUsers(_ context.Context, _ string, _ int, _ string) ([]*storage.Block, string, error) {
	return nil, "", nil
}
func (s *stubSocialRepo) CreateMute(_ context.Context, _ *storage.Mute) error { return nil }
func (s *stubSocialRepo) DeleteMute(_ context.Context, _, _ string) error     { return nil }
func (s *stubSocialRepo) GetMute(_ context.Context, _, _ string) (*storage.Mute, error) {
	return nil, pkgerrors.ItemNotFound("mute")
}
func (s *stubSocialRepo) IsMuted(_ context.Context, _, _ string) (bool, error) { return false, nil }
func (s *stubSocialRepo) GetMutedUsers(_ context.Context, _ string, _ int, _ string) ([]*storage.Mute, string, error) {
	return nil, "", nil
}
func (s *stubSocialRepo) HasUserAnnounced(_ context.Context, _, _ string) (bool, error) {
	return false, nil
}
func (s *stubSocialRepo) GetActorAnnounces(_ context.Context, _ string, _ int, _ string) ([]*storage.Announce, string, error) {
	return nil, "", nil
}
func (s *stubSocialRepo) CountObjectAnnounces(_ context.Context, _ string) (int, error) {
	return 0, nil
}
func (s *stubSocialRepo) CascadeDeleteAnnounces(_ context.Context, _ string) error { return nil }
func (s *stubSocialRepo) CreateAccountPin(_ context.Context, _ *storage.AccountPin) error {
	return nil
}
func (s *stubSocialRepo) DeleteAccountPin(_ context.Context, _, _ string) error { return nil }
func (s *stubSocialRepo) GetAccountPins(_ context.Context, _ string) ([]*storage.AccountPin, error) {
	return nil, nil
}
func (s *stubSocialRepo) GetAccountPinsPaginated(_ context.Context, _ string, _ int, _ string) ([]*storage.AccountPin, string, error) {
	return nil, "", nil
}
func (s *stubSocialRepo) IsAccountPinned(_ context.Context, _, _ string) (bool, error) {
	return false, nil
}
func (s *stubSocialRepo) CreateAccountNote(_ context.Context, _ *storage.AccountNote) error {
	return nil
}
func (s *stubSocialRepo) UpdateAccountNote(_ context.Context, _ *storage.AccountNote) error {
	return nil
}
func (s *stubSocialRepo) DeleteAccountNote(_ context.Context, _, _ string) error { return nil }
func (s *stubSocialRepo) GetAccountNote(_ context.Context, _, _ string) (*storage.AccountNote, error) {
	return nil, pkgerrors.ItemNotFound("note")
}
func (s *stubSocialRepo) GetStatusPins(_ context.Context, _ string) ([]*storage.StatusPin, error) {
	return nil, nil
}
func (s *stubSocialRepo) GetStatusPinsPaginated(_ context.Context, _ string, _ int, _ string) ([]*storage.StatusPin, string, error) {
	return nil, "", nil
}
func (s *stubSocialRepo) IsStatusPinned(_ context.Context, _, _ string) (bool, error) {
	return false, nil
}
func (s *stubSocialRepo) ReorderStatusPins(_ context.Context, _ string, _ []string) error { return nil }
func (s *stubSocialRepo) CountUserPinnedStatuses(_ context.Context, _ string) (int, error) {
	return 0, nil
}

func (s *stubSocialRepo) CreateAnnounce(_ context.Context, announce *storage.Announce) error {
	if s.announces == nil {
		s.announces = map[string]*storage.Announce{}
	}
	key := announce.Actor + "|" + announce.Object
	if _, ok := s.announces[key]; ok {
		return pkgerrors.ItemAlreadyExists("announce")
	}
	if strings.TrimSpace(announce.ID) == "" {
		announce.ID = fmt.Sprintf("announce-%d", len(s.announces)+1)
	}
	s.announces[key] = announce
	return nil
}

func (s *stubSocialRepo) DeleteAnnounce(_ context.Context, actor, object string) error {
	if s.announces == nil {
		return nil
	}
	delete(s.announces, actor+"|"+object)
	return nil
}

func (s *stubSocialRepo) GetAnnounce(_ context.Context, actor, object string) (*storage.Announce, error) {
	if s.announces != nil {
		if existing, ok := s.announces[actor+"|"+object]; ok {
			return existing, nil
		}
	}
	return nil, pkgerrors.ItemNotFound("announce")
}

func (s *stubSocialRepo) GetStatusAnnounces(_ context.Context, statusID string, _ int, _ string) ([]*storage.Announce, string, error) {
	return []*storage.Announce{
		{
			ID:     "announce-1",
			Actor:  "https://example.com/users/bob",
			Object: fmt.Sprintf("https://example.com/users/alice/statuses/%s", statusID),
		},
	}, "", nil
}

func (s *stubSocialRepo) CreateStatusPin(_ context.Context, pin *storage.StatusPin) error {
	if s.pins == nil {
		s.pins = map[string]*storage.StatusPin{}
	}
	key := pin.Username + "|" + pin.StatusID
	if _, ok := s.pins[key]; ok {
		return pkgerrors.ItemAlreadyExists("pin")
	}
	s.pins[key] = pin
	return nil
}

func (s *stubSocialRepo) DeleteStatusPin(_ context.Context, userID, statusID string) error {
	if s.pins == nil {
		return nil
	}
	delete(s.pins, userID+"|"+statusID)
	return nil
}

type socialRepoGetAnnounceErr struct {
	stubSocialRepo
	err error
}

func (s *socialRepoGetAnnounceErr) GetAnnounce(context.Context, string, string) (*storage.Announce, error) {
	return nil, s.err
}

type socialRepoDeleteAnnounceErr struct {
	stubSocialRepo
	err error
}

func (s *socialRepoDeleteAnnounceErr) DeleteAnnounce(context.Context, string, string) error {
	return s.err
}

type socialRepoCreateAnnounceErr struct {
	stubSocialRepo
	err error
}

func (s *socialRepoCreateAnnounceErr) CreateAnnounce(context.Context, *storage.Announce) error {
	return s.err
}

type socialRepoAnnounceRace struct {
	stubSocialRepo
	notFoundCalls int
	getCalls      int
}

func (s *socialRepoAnnounceRace) GetAnnounce(ctx context.Context, actor, object string) (*storage.Announce, error) {
	s.getCalls++
	if s.getCalls <= s.notFoundCalls {
		return nil, pkgerrors.ItemNotFound("announce")
	}
	return s.stubSocialRepo.GetAnnounce(ctx, actor, object)
}

type failingAnalytics struct{}

func (f failingAnalytics) RecordStatusCreation(context.Context, string, time.Time) error { return nil }
func (f failingAnalytics) RecordHashtagUsage(context.Context, []string, string, string) error {
	return fmt.Errorf("boom")
}
func (f failingAnalytics) RecordLinkShare(context.Context, []string, string, string) error {
	return nil
}
func (f failingAnalytics) RecordEngagement(context.Context, string, string, string) error { return nil }
func (f failingAnalytics) RecordInstanceActivity(context.Context, string, time.Time) error {
	return fmt.Errorf("boom")
}

type stubAccountRepoError struct {
	stubAccountRepo
	err error
}

func (r *stubAccountRepoError) GetAccount(context.Context, string) (*storage.Account, error) {
	return nil, r.err
}

type failingFederation struct{}

func (f failingFederation) QueueActivity(context.Context, *activitypub.Activity) error {
	return fmt.Errorf("boom")
}

type failingPublisher struct{}

func (f failingPublisher) PublishToUser(context.Context, string, *streaming.Event) error {
	return fmt.Errorf("boom")
}

func (f failingPublisher) PublishToStream(context.Context, string, *streaming.Event) error {
	return fmt.Errorf("boom")
}

func (f failingPublisher) PublishToConversation(context.Context, string, *streaming.Event) error {
	return fmt.Errorf("boom")
}

func (f failingPublisher) Close() error { return nil }

type stubConversationRepo struct {
	*repositories.ConversationRepository
	createMuteErr error
	deleteMuteErr error
}

func (s *stubConversationRepo) GetUserConversations(_ context.Context, _ string, _ interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Conversation], error) {
	return &interfaces.PaginatedResult[*models.Conversation]{
		Items: []*models.Conversation{
			{ID: "conv-1"},
		},
		NextCursor: "",
		HasMore:    false,
		Total:      1,
	}, nil
}

func (s *stubConversationRepo) GetUserConversationsByFolder(_ context.Context, _ string, _ models.UserConversationFolder, _ interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Conversation], error) {
	return s.GetUserConversations(context.Background(), "", interfaces.PaginationOptions{})
}

func (s *stubConversationRepo) CreateConversationMute(_ context.Context, _ *storage.ConversationMute) error {
	return s.createMuteErr
}

func (s *stubConversationRepo) DeleteConversationMute(_ context.Context, _, _ string) error {
	return s.deleteMuteErr
}

type stubScheduledRepo struct {
	created []*storage.ScheduledStatus
}

func (s *stubScheduledRepo) CreateScheduledStatus(_ context.Context, scheduled *storage.ScheduledStatus) error {
	s.created = append(s.created, scheduled)
	return nil
}

func (s *stubScheduledRepo) GetScheduledStatus(context.Context, string) (*storage.ScheduledStatus, error) {
	return nil, pkgerrors.ItemNotFound("scheduled_status")
}

func (s *stubScheduledRepo) GetScheduledStatuses(context.Context, string, int, string) ([]*storage.ScheduledStatus, string, error) {
	return []*storage.ScheduledStatus{}, "", nil
}

func (s *stubScheduledRepo) UpdateScheduledStatus(context.Context, *storage.ScheduledStatus) error {
	return nil
}
func (s *stubScheduledRepo) DeleteScheduledStatus(context.Context, string) error { return nil }

func newNotesServiceHarness(t *testing.T) (*Service, *stubPublisher, *stubFederation, *stubNotificationService, *stubAnalytics) {
	t.Helper()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	mockUpdateBuilder := new(mocks.MockUpdateBuilder)
	state := &permissiveQueryState{}
	setupPermissiveDynamormMocks(mockDB, mockQuery, mockUpdateBuilder, state)

	db := transactionalMockDB{MockDB: mockDB}

	logger := zap.NewNop()
	table := "test-table"
	domain := "example.com"

	statusRepo := repositories.NewStatusRepository(db, table, logger, nil)
	bookmarkRepo := repositories.NewBookmarkRepository(db, table, logger)
	relationshipRepo := repositories.NewRelationshipRepository(db, table, logger)
	likeRepo := repositories.NewLikeRepository(db, table, logger)
	objectRepo := repositories.NewObjectRepository(db, table, domain, logger)
	searchRepo := repositories.NewSearchRepository(db, table, logger, nil)
	communityNoteRepo := repositories.NewCommunityNoteRepository(db, table, logger, nil)
	userRepo := repositories.NewUserRepository(db, table, logger)
	pollRepo := repositories.NewPollRepository(db, table, logger, nil)

	statusRepo.SetBookmarkRepository(bookmarkRepo)
	statusRepo.SetRelationshipRepository(relationshipRepo)

	publisher := &stubPublisher{}
	federation := &stubFederation{}
	notifier := &stubNotificationService{}
	analytics := &stubAnalytics{}

	accountRepo := &stubAccountRepo{domain: domain}
	socialRepo := &stubSocialRepo{}
	conversationRepo := &stubConversationRepo{}

	service := NewService(
		statusRepo,
		accountRepo,
		bookmarkRepo,
		relationshipRepo,
		nil,
		likeRepo,
		socialRepo,
		conversationRepo,
		objectRepo,
		searchRepo,
		communityNoteRepo,
		userRepo,
		pollRepo,
		publisher,
		analytics,
		federation,
		nil,
		notifier,
		logger,
		domain,
	)

	service.scheduledRepo = &stubScheduledRepo{}

	return service, publisher, federation, notifier, analytics
}

func newNotesServiceHarnessWithDB(t *testing.T, db core.DB) *Service {
	t.Helper()

	logger := zap.NewNop()
	table := "test-table"
	domain := "example.com"

	statusRepo := repositories.NewStatusRepository(db, table, logger, nil)
	bookmarkRepo := repositories.NewBookmarkRepository(db, table, logger)
	relationshipRepo := repositories.NewRelationshipRepository(db, table, logger)
	likeRepo := repositories.NewLikeRepository(db, table, logger)
	objectRepo := repositories.NewObjectRepository(db, table, domain, logger)
	searchRepo := repositories.NewSearchRepository(db, table, logger, nil)
	communityNoteRepo := repositories.NewCommunityNoteRepository(db, table, logger, nil)
	userRepo := repositories.NewUserRepository(db, table, logger)
	pollRepo := repositories.NewPollRepository(db, table, logger, nil)

	statusRepo.SetBookmarkRepository(bookmarkRepo)
	statusRepo.SetRelationshipRepository(relationshipRepo)

	service := NewService(
		statusRepo,
		&stubAccountRepo{domain: domain},
		bookmarkRepo,
		relationshipRepo,
		nil,
		likeRepo,
		&stubSocialRepo{},
		&stubConversationRepo{},
		objectRepo,
		searchRepo,
		communityNoteRepo,
		userRepo,
		pollRepo,
		&stubPublisher{},
		&stubAnalytics{},
		&stubFederation{},
		nil,
		&stubNotificationService{},
		logger,
		domain,
	)

	service.scheduledRepo = &stubScheduledRepo{}

	return service
}

func TestService_round15_create_update_delete_smoke(t *testing.T) {
	service, publisher, federation, _, analytics := newNotesServiceHarness(t)

	ctx := context.Background()

	created, err := service.CreateNote(ctx, &CreateNoteCommand{
		AuthorID:      "alice",
		Content:       "hello #Go #Test",
		Visibility:    VisibilityPublic,
		ToRecipients:  []string{"https://www.w3.org/ns/activitystreams#Public"},
		CcRecipients:  []string{"https://example.com/users/alice/followers"},
		PollOptions:   []string{"a", "b"},
		PollExpiresIn: 60,
	})
	require.NoError(t, err)
	require.NotNil(t, created)
	require.NotNil(t, created.Note)
	assert.Equal(t, "alice", created.Note.AuthorUsername)
	assert.NotEmpty(t, created.Note.StatusID)
	assert.GreaterOrEqual(t, len(publisher.userEvents)+len(publisher.streamEvents), 1)
	assert.GreaterOrEqual(t, len(federation.activities), 1)
	assert.GreaterOrEqual(t, analytics.hashtagCalls, 1)
	assert.GreaterOrEqual(t, analytics.instanceCalls, 1)

	updated, err := service.UpdateNote(ctx, &UpdateNoteCommand{
		StatusID:  created.Note.StatusID,
		Content:   "updated",
		Sensitive: true,
		Language:  "en",
		UpdaterID: "alice",
	})
	require.NoError(t, err)
	require.NotNil(t, updated)

	require.NoError(t, service.DeleteNote(ctx, &DeleteNoteCommand{
		StatusID:  created.Note.StatusID,
		DeleterID: "alice",
	}))
}

func TestService_round15_create_note_generates_local_mention_tags_and_notifications(t *testing.T) {
	service, _, federation, notifier, _ := newNotesServiceHarness(t)
	service.accountRepo = &stubAccountRepo{
		domain:  "example.com",
		missing: map[string]bool{"missing": true},
	}

	ctx := context.Background()
	created, err := service.CreateNote(ctx, &CreateNoteCommand{
		AuthorID:     "alice",
		Content:      "hello @bob and @bob and @missing",
		Visibility:   VisibilityPublic,
		ToRecipients: []string{activitypub.PublicAddress},
		CcRecipients: []string{"https://example.com/users/alice/followers"},
	})
	require.NoError(t, err)
	require.NotNil(t, created)
	require.NotNil(t, created.Note)
	require.NotNil(t, created.Note.Note)

	require.Equal(t, []string{"https://example.com/users/bob"}, created.Note.Mentions)
	require.Len(t, created.Note.Note.Tag, 1)
	assert.Equal(t, "Mention", created.Note.Note.Tag[0].Type)
	assert.Equal(t, "https://example.com/users/bob", created.Note.Note.Tag[0].Href)
	assert.Equal(t, "@bob", created.Note.Note.Tag[0].Name)

	require.Len(t, notifier.cmds, 1)
	assert.Equal(t, "bob", notifier.cmds[0].UserID)
	assert.Equal(t, "alice", notifier.cmds[0].ActorID)
	assert.Equal(t, "mention", notifier.cmds[0].Type)
	assert.Equal(t, created.Note.StatusID, notifier.cmds[0].TargetID)

	require.Len(t, federation.activities, 1)
	federatedNote, ok := federation.activities[0].Object.(*activitypub.Note)
	require.True(t, ok)
	require.Len(t, federatedNote.Tag, 1)
	assert.Equal(t, "Mention", federatedNote.Tag[0].Type)
	assert.Equal(t, []string{"https://example.com/users/alice/followers", "https://example.com/users/bob"}, federatedNote.CC)
}

func TestService_round15_create_note_reply_creates_notification_for_parent_author(t *testing.T) {
	service, _, _, notifier, _ := newNotesServiceHarness(t)

	created, err := service.CreateNote(context.Background(), &CreateNoteCommand{
		AuthorID:     "bob",
		Content:      "replying now",
		Visibility:   VisibilityPublic,
		InReplyToID:  "status-1",
		ToRecipients: []string{activitypub.PublicAddress},
		CcRecipients: []string{"https://example.com/users/bob/followers"},
	})
	require.NoError(t, err)
	require.NotNil(t, created)

	var replyNotification *notifications.CreateNotificationCommand
	for _, cmd := range notifier.cmds {
		if cmd.Type == "reply" {
			replyNotification = cmd
			break
		}
	}

	require.NotNil(t, replyNotification)
	assert.Equal(t, "alice", replyNotification.UserID)
	assert.Equal(t, "bob", replyNotification.ActorID)
	assert.Equal(t, created.Note.StatusID, replyNotification.TargetID)
}

func TestService_round15_create_note_reply_to_remote_parent_preserves_canonical_remote_identity(t *testing.T) {
	t.Run("public reply derives remote parent recipient in cc", func(t *testing.T) {
		service, _, federation, _, _ := newNotesServiceHarness(t)
		originalRepo := service.noteRepo

		remoteParent := &models.Status{
			StatusID:       "remote-parent-public",
			AuthorID:       "https://remote.example/users/steward",
			AuthorUsername: "steward@remote.example",
			ConversationID: "remote-conversation-public",
			Visibility:     models.VisibilityPublic,
			ToRecipients:   []string{activitypub.PublicAddress},
			CcRecipients:   []string{"https://remote.example/users/steward/followers"},
			PublishedAt:    time.Now().UTC(),
			CreatedAt:      time.Now().UTC(),
			UpdatedAt:      time.Now().UTC(),
			ModifiedAt:     time.Now().UTC(),
			Note: &activitypub.Note{
				BaseObject: activitypub.BaseObject{
					ID:   "https://remote.example/users/steward/statuses/remote-parent-public",
					To:   []string{activitypub.PublicAddress},
					CC:   []string{"https://remote.example/users/steward/followers"},
					Type: activitypub.NoteType,
				},
				AttributedTo: "https://remote.example/users/steward",
				Content:      "seed",
				Visibility:   models.VisibilityPublic,
			},
		}

		service.noteRepo = &delegatingStatusRepo{
			StatusRepository: originalRepo,
			getStatus: func(_ context.Context, statusID string) (*models.Status, error) {
				if statusID == remoteParent.StatusID {
					return remoteParent, nil
				}
				return originalRepo.GetStatus(context.Background(), statusID)
			},
		}

		created, err := service.CreateNote(context.Background(), &CreateNoteCommand{
			AuthorID:    "alice",
			Content:     "reply without mention",
			Visibility:  VisibilityPublic,
			InReplyToID: remoteParent.StatusID,
		})
		require.NoError(t, err)
		require.NotNil(t, created)
		require.NotNil(t, created.Note)
		require.NotNil(t, created.Note.Note)
		require.Len(t, federation.activities, 1)

		expectedCC := []string{
			"https://example.com/users/alice/followers",
			remoteParent.AuthorID,
		}

		assert.Equal(t, remoteParent.Note.ID, created.Note.Note.InReplyTo)
		assert.Equal(t, remoteParent.ConversationID, created.Note.ConversationID)
		assert.Equal(t, remoteParent.ConversationID, created.Note.Note.ConversationID)
		assert.Equal(t, []string{activitypub.PublicAddress}, created.Note.ToRecipients)
		assert.Equal(t, expectedCC, created.Note.CcRecipients)
		assert.Empty(t, created.Note.BtoRecipients)

		assert.Equal(t, []string{activitypub.PublicAddress}, federation.activities[0].To)
		assert.Equal(t, expectedCC, federation.activities[0].CC)
		assert.Empty(t, federation.activities[0].BTo)

		federatedNote, ok := federation.activities[0].Object.(*activitypub.Note)
		require.True(t, ok)
		assert.Equal(t, remoteParent.Note.ID, federatedNote.InReplyTo)
		assert.Equal(t, remoteParent.ConversationID, federatedNote.ConversationID)
		assert.Equal(t, expectedCC, federatedNote.CC)
	})

	t.Run("private reply derives remote parent recipient in bto", func(t *testing.T) {
		service, _, federation, _, _ := newNotesServiceHarness(t)
		originalRepo := service.noteRepo

		remoteParent := &models.Status{
			StatusID:       "remote-parent-private",
			AuthorID:       "https://remote.example/users/steward",
			AuthorUsername: "steward@remote.example",
			ConversationID: "remote-conversation-private",
			Visibility:     models.VisibilityPublic,
			ToRecipients:   []string{activitypub.PublicAddress},
			CcRecipients:   []string{"https://remote.example/users/steward/followers"},
			PublishedAt:    time.Now().UTC(),
			CreatedAt:      time.Now().UTC(),
			UpdatedAt:      time.Now().UTC(),
			ModifiedAt:     time.Now().UTC(),
			Note: &activitypub.Note{
				BaseObject: activitypub.BaseObject{
					ID:   "https://remote.example/users/steward/statuses/remote-parent-private",
					To:   []string{activitypub.PublicAddress},
					CC:   []string{"https://remote.example/users/steward/followers"},
					Type: activitypub.NoteType,
				},
				AttributedTo: "https://remote.example/users/steward",
				Content:      "seed",
				Visibility:   models.VisibilityPublic,
			},
		}

		service.noteRepo = &delegatingStatusRepo{
			StatusRepository: originalRepo,
			getStatus: func(_ context.Context, statusID string) (*models.Status, error) {
				if statusID == remoteParent.StatusID {
					return remoteParent, nil
				}
				return originalRepo.GetStatus(context.Background(), statusID)
			},
		}

		created, err := service.CreateNote(context.Background(), &CreateNoteCommand{
			AuthorID:    "alice",
			Content:     "private reply without mention",
			Visibility:  models.VisibilityPrivate,
			InReplyToID: remoteParent.StatusID,
		})
		require.NoError(t, err)
		require.NotNil(t, created)
		require.NotNil(t, created.Note)
		require.NotNil(t, created.Note.Note)
		require.Len(t, federation.activities, 1)

		expectedTo := []string{"https://example.com/users/alice/followers"}
		expectedBTo := []string{remoteParent.AuthorID}

		assert.Equal(t, remoteParent.Note.ID, created.Note.Note.InReplyTo)
		assert.Equal(t, remoteParent.ConversationID, created.Note.ConversationID)
		assert.Equal(t, remoteParent.ConversationID, created.Note.Note.ConversationID)
		assert.Equal(t, expectedTo, created.Note.ToRecipients)
		assert.Empty(t, created.Note.CcRecipients)
		assert.Equal(t, expectedBTo, created.Note.BtoRecipients)

		assert.Equal(t, expectedTo, federation.activities[0].To)
		assert.Empty(t, federation.activities[0].CC)
		assert.Equal(t, expectedBTo, federation.activities[0].BTo)

		federatedNote, ok := federation.activities[0].Object.(*activitypub.Note)
		require.True(t, ok)
		assert.Equal(t, remoteParent.Note.ID, federatedNote.InReplyTo)
		assert.Equal(t, remoteParent.ConversationID, federatedNote.ConversationID)
		assert.Equal(t, expectedBTo, federatedNote.BTo)
	})
}

func TestService_round15_create_note_reply_to_remote_parent_skips_local_notification(t *testing.T) {
	service, _, _, notifier, _ := newNotesServiceHarness(t)
	originalRepo := service.noteRepo

	remoteParent := &models.Status{
		StatusID:       "remote-parent-notify",
		AuthorID:       "https://remote.example/users/steward",
		AuthorUsername: "steward@remote.example",
		ConversationID: "remote-conversation-notify",
		Visibility:     models.VisibilityPublic,
		PublishedAt:    time.Now().UTC(),
		CreatedAt:      time.Now().UTC(),
		UpdatedAt:      time.Now().UTC(),
		ModifiedAt:     time.Now().UTC(),
		Note: &activitypub.Note{
			BaseObject: activitypub.BaseObject{
				ID:   "https://remote.example/users/steward/statuses/remote-parent-notify",
				Type: activitypub.NoteType,
			},
			AttributedTo: "https://remote.example/users/steward",
			Content:      "seed",
			Visibility:   models.VisibilityPublic,
		},
	}

	service.noteRepo = &delegatingStatusRepo{
		StatusRepository: originalRepo,
		getStatus: func(_ context.Context, statusID string) (*models.Status, error) {
			if statusID == remoteParent.StatusID {
				return remoteParent, nil
			}
			return originalRepo.GetStatus(context.Background(), statusID)
		},
	}

	created, err := service.CreateNote(context.Background(), &CreateNoteCommand{
		AuthorID:    "alice",
		Content:     "reply without mention",
		Visibility:  VisibilityPublic,
		InReplyToID: remoteParent.StatusID,
	})
	require.NoError(t, err)
	require.NotNil(t, created)

	for _, cmd := range notifier.cmds {
		require.NotEqual(t, common.NotificationTypeReply, cmd.Type)
	}
}

func TestService_round15_create_note_remote_parent_resolver_reuses_resolved_parent(t *testing.T) {
	t.Run("public unresolved remote parent url is canonicalized once", func(t *testing.T) {
		service, _, federation, notifier, _ := newNotesServiceHarness(t)

		parentURL := "https://remote.example/users/steward/statuses/unresolved-parent-public"
		remoteParent := &models.Status{
			StatusID:       models.CanonicalStatusIDForDomain(parentURL, "example.com"),
			AuthorID:       "https://remote.example/users/steward",
			AuthorUsername: "steward@remote.example",
			ConversationID: "remote-conversation-public",
			Visibility:     models.VisibilityPublic,
			ToRecipients:   []string{activitypub.PublicAddress},
			CcRecipients:   []string{"https://remote.example/users/steward/followers"},
			PublishedAt:    time.Now().UTC(),
			CreatedAt:      time.Now().UTC(),
			UpdatedAt:      time.Now().UTC(),
			ModifiedAt:     time.Now().UTC(),
			Note: &activitypub.Note{
				BaseObject: activitypub.BaseObject{
					ID:   parentURL,
					To:   []string{activitypub.PublicAddress},
					CC:   []string{"https://remote.example/users/steward/followers"},
					Type: activitypub.NoteType,
				},
				AttributedTo: "https://remote.example/users/steward",
				Content:      "seed",
				Visibility:   models.VisibilityPublic,
			},
		}

		resolver := &stubReplyParentResolver{
			resolved: map[string]*ResolvedReplyParent{
				parentURL: {
					Status:             remoteParent,
					CanonicalObjectURL: parentURL,
					CanonicalStatusID:  remoteParent.StatusID,
					Visibility:         remoteParent.Visibility,
					Fetched:            true,
					Remote:             true,
				},
			},
		}
		service.replyParents = resolver

		created, err := service.CreateNote(context.Background(), &CreateNoteCommand{
			AuthorID:    "alice",
			Content:     "reply without mention",
			Visibility:  VisibilityPublic,
			InReplyToID: parentURL,
		})
		require.NoError(t, err)
		require.NotNil(t, created)
		require.NotNil(t, created.ReplyParentAcquisition)
		require.Len(t, resolver.calls, 1)
		require.Len(t, federation.activities, 1)

		assert.Equal(t, "alice", resolver.calls[0].username)
		assert.Equal(t, parentURL, resolver.calls[0].raw)
		assert.Equal(t, VisibilityPublic, resolver.calls[0].visibility)
		assert.Equal(t, remoteParent.StatusID, created.Note.InReplyToID)
		assert.Equal(t, parentURL, created.Note.Note.InReplyTo)
		assert.Equal(t, remoteParent.ConversationID, created.Note.ConversationID)
		assert.True(t, created.ReplyParentAcquisition.Fetched)
		assert.True(t, created.ReplyParentAcquisition.Remote)
		assert.Equal(t, remoteParent.StatusID, created.ReplyParentAcquisition.CanonicalStatusID)

		expectedCC := []string{
			"https://example.com/users/alice/followers",
			remoteParent.AuthorID,
		}
		assert.Equal(t, []string{activitypub.PublicAddress}, created.Note.ToRecipients)
		assert.Equal(t, expectedCC, created.Note.CcRecipients)

		federatedNote, ok := federation.activities[0].Object.(*activitypub.Note)
		require.True(t, ok)
		assert.Equal(t, parentURL, federatedNote.InReplyTo)
		assert.Equal(t, expectedCC, federatedNote.CC)

		for _, cmd := range notifier.cmds {
			require.NotEqual(t, common.NotificationTypeReply, cmd.Type)
		}
	})

	t.Run("private unresolved remote parent url preserves protected audience", func(t *testing.T) {
		service, _, federation, _, _ := newNotesServiceHarness(t)

		parentURL := "https://remote.example/users/steward/statuses/unresolved-parent-private"
		remoteParent := &models.Status{
			StatusID:       models.CanonicalStatusIDForDomain(parentURL, "example.com"),
			AuthorID:       "https://remote.example/users/steward",
			AuthorUsername: "steward@remote.example",
			ConversationID: "remote-conversation-private",
			Visibility:     models.VisibilityPrivate,
			ToRecipients:   []string{"https://remote.example/users/steward/followers"},
			PublishedAt:    time.Now().UTC(),
			CreatedAt:      time.Now().UTC(),
			UpdatedAt:      time.Now().UTC(),
			ModifiedAt:     time.Now().UTC(),
			Note: &activitypub.Note{
				BaseObject: activitypub.BaseObject{
					ID:   parentURL,
					To:   []string{"https://remote.example/users/steward/followers"},
					Type: activitypub.NoteType,
				},
				AttributedTo: "https://remote.example/users/steward",
				Content:      "seed",
				Visibility:   models.VisibilityPrivate,
			},
		}

		service.replyParents = &stubReplyParentResolver{
			resolved: map[string]*ResolvedReplyParent{
				parentURL: {
					Status:             remoteParent,
					CanonicalObjectURL: parentURL,
					CanonicalStatusID:  remoteParent.StatusID,
					Visibility:         remoteParent.Visibility,
					Fetched:            true,
					Remote:             true,
				},
			},
		}

		created, err := service.CreateNote(context.Background(), &CreateNoteCommand{
			AuthorID:    "alice",
			Content:     "private reply without mention",
			Visibility:  models.VisibilityPrivate,
			InReplyToID: parentURL,
		})
		require.NoError(t, err)
		require.NotNil(t, created)
		require.Len(t, federation.activities, 1)

		expectedTo := []string{"https://example.com/users/alice/followers"}
		expectedBTo := []string{remoteParent.AuthorID}

		assert.Equal(t, remoteParent.StatusID, created.Note.InReplyToID)
		assert.Equal(t, parentURL, created.Note.Note.InReplyTo)
		assert.Equal(t, remoteParent.ConversationID, created.Note.ConversationID)
		assert.Equal(t, expectedTo, created.Note.ToRecipients)
		assert.Equal(t, expectedBTo, created.Note.BtoRecipients)

		federatedNote, ok := federation.activities[0].Object.(*activitypub.Note)
		require.True(t, ok)
		assert.Equal(t, parentURL, federatedNote.InReplyTo)
		assert.Equal(t, expectedBTo, federatedNote.BTo)
	})
}

func TestService_round15_create_note_derives_canonical_audience_defaults(t *testing.T) {
	followers := "https://example.com/users/alice/followers"

	tests := []struct {
		name       string
		visibility string
		wantTo     []string
		wantCC     []string
	}{
		{
			name:       "public",
			visibility: VisibilityPublic,
			wantTo:     []string{activitypub.PublicAddress},
			wantCC:     []string{followers},
		},
		{
			name:       "unlisted",
			visibility: models.VisibilityUnlisted,
			wantTo:     []string{followers},
			wantCC:     []string{activitypub.PublicAddress},
		},
		{
			name:       "private",
			visibility: models.VisibilityPrivate,
			wantTo:     []string{followers},
			wantCC:     nil,
		},
		{
			name:       "direct stays explicit only",
			visibility: models.VisibilityDirect,
			wantTo:     nil,
			wantCC:     nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service, _, federation, _, _ := newNotesServiceHarness(t)

			created, err := service.CreateNote(context.Background(), &CreateNoteCommand{
				AuthorID:   "alice",
				Content:    "hello world",
				Visibility: tt.visibility,
			})
			require.NoError(t, err)
			require.NotNil(t, created)
			require.NotNil(t, created.Note)
			require.NotNil(t, created.Note.Note)

			assert.Equal(t, tt.wantTo, created.Note.ToRecipients)
			assert.Equal(t, tt.wantCC, created.Note.CcRecipients)
			assert.Equal(t, tt.wantTo, created.Note.Note.To)
			assert.Equal(t, tt.wantCC, created.Note.Note.CC)

			require.Len(t, federation.activities, 1)
			assert.Equal(t, tt.wantTo, federation.activities[0].To)
			assert.Equal(t, tt.wantCC, federation.activities[0].CC)
		})
	}
}

func TestService_round15_create_note_keeps_status_audience_aligned_after_mention_merge(t *testing.T) {
	t.Run("public remote mention aligns status note and queued activity", func(t *testing.T) {
		service, _, federation, notifier, _ := newNotesServiceHarness(t)
		service.accountRepo = &stubAccountRepo{
			domain:  "example.com",
			missing: map[string]bool{"carol@remote.example": true},
		}
		federation.resolved = map[string]*activitypub.Actor{
			"carol@remote.example": {
				BaseObject:        activitypub.BaseObject{ID: "https://remote.example/users/carol"},
				PreferredUsername: "carol",
			},
		}

		created, err := service.CreateNote(context.Background(), &CreateNoteCommand{
			AuthorID:   "alice",
			Content:    "hello @carol@remote.example",
			Visibility: VisibilityPublic,
		})
		require.NoError(t, err)
		require.NotNil(t, created)
		require.Len(t, notifier.cmds, 0)

		expectedTo := []string{activitypub.PublicAddress}
		expectedCC := []string{
			"https://example.com/users/alice/followers",
			"https://remote.example/users/carol",
		}

		assert.Equal(t, expectedTo, created.Note.ToRecipients)
		assert.Equal(t, expectedCC, created.Note.CcRecipients)
		assert.Equal(t, expectedTo, created.Note.Note.To)
		assert.Equal(t, expectedCC, created.Note.Note.CC)

		require.Len(t, federation.activities, 1)
		assert.Equal(t, expectedTo, federation.activities[0].To)
		assert.Equal(t, expectedCC, federation.activities[0].CC)

		federatedNote, ok := federation.activities[0].Object.(*activitypub.Note)
		require.True(t, ok)
		assert.Equal(t, expectedTo, federatedNote.To)
		assert.Equal(t, expectedCC, federatedNote.CC)
	})

	t.Run("direct remote mention aligns status note and queued activity", func(t *testing.T) {
		service, _, federation, notifier, _ := newNotesServiceHarness(t)
		service.accountRepo = &stubAccountRepo{
			domain:  "example.com",
			missing: map[string]bool{"carol@remote.example": true},
		}
		federation.resolved = map[string]*activitypub.Actor{
			"carol@remote.example": {
				BaseObject:        activitypub.BaseObject{ID: "https://remote.example/users/carol"},
				PreferredUsername: "carol",
			},
		}

		created, err := service.CreateNote(context.Background(), &CreateNoteCommand{
			AuthorID:   "alice",
			Content:    "hello @carol@remote.example",
			Visibility: models.VisibilityDirect,
		})
		require.NoError(t, err)
		require.NotNil(t, created)
		require.Len(t, notifier.cmds, 0)

		expectedTo := []string{"https://remote.example/users/carol"}

		assert.Equal(t, expectedTo, created.Note.ToRecipients)
		assert.Empty(t, created.Note.CcRecipients)
		assert.Equal(t, expectedTo, created.Note.Note.To)
		assert.Empty(t, created.Note.Note.CC)

		require.Len(t, federation.activities, 1)
		assert.Equal(t, expectedTo, federation.activities[0].To)
		assert.Empty(t, federation.activities[0].CC)

		federatedNote, ok := federation.activities[0].Object.(*activitypub.Note)
		require.True(t, ok)
		assert.Equal(t, expectedTo, federatedNote.To)
		assert.Empty(t, federatedNote.CC)
	})
}

func TestService_round15_create_note_keeps_truthful_explicit_recipients_stable(t *testing.T) {
	service, _, federation, _, _ := newNotesServiceHarness(t)

	expectedTo := []string{activitypub.PublicAddress}
	expectedCC := []string{
		"https://example.com/users/alice/followers",
		"https://remote.example/users/carol",
	}
	expectedBTo := []string{"https://remote.example/users/dave"}
	expectedBCC := []string{"https://remote.example/users/erin"}

	created, err := service.CreateNote(context.Background(), &CreateNoteCommand{
		AuthorID:      "alice",
		Content:       "hello world",
		Visibility:    VisibilityPublic,
		ToRecipients:  append([]string(nil), expectedTo...),
		CcRecipients:  append([]string(nil), expectedCC...),
		BtoRecipients: append([]string(nil), expectedBTo...),
		BccRecipients: append([]string(nil), expectedBCC...),
	})
	require.NoError(t, err)
	require.NotNil(t, created)
	require.NotNil(t, created.Note)
	require.NotNil(t, created.Note.Note)

	assert.Equal(t, expectedTo, created.Note.ToRecipients)
	assert.Equal(t, expectedCC, created.Note.CcRecipients)
	assert.Equal(t, expectedBTo, created.Note.BtoRecipients)
	assert.Equal(t, expectedBCC, created.Note.BccRecipients)

	assert.Equal(t, expectedTo, created.Note.Note.To)
	assert.Equal(t, expectedCC, created.Note.Note.CC)
	assert.Equal(t, expectedBTo, created.Note.Note.BTo)
	assert.Equal(t, expectedBCC, created.Note.Note.BCC)

	require.Len(t, federation.activities, 1)
	assert.Equal(t, expectedTo, federation.activities[0].To)
	assert.Equal(t, expectedCC, federation.activities[0].CC)
	assert.Equal(t, expectedBTo, federation.activities[0].BTo)
	assert.Equal(t, expectedBCC, federation.activities[0].BCC)

	federatedNote, ok := federation.activities[0].Object.(*activitypub.Note)
	require.True(t, ok)
	assert.Equal(t, expectedTo, federatedNote.To)
	assert.Equal(t, expectedCC, federatedNote.CC)
	assert.Equal(t, expectedBTo, federatedNote.BTo)
	assert.Equal(t, expectedBCC, federatedNote.BCC)
}

func TestService_round15_create_note_resolves_remote_mentions_for_federation(t *testing.T) {
	t.Run("public mentions remote actor in cc", func(t *testing.T) {
		service, _, federation, notifier, _ := newNotesServiceHarness(t)
		service.accountRepo = &stubAccountRepo{
			domain:  "example.com",
			missing: map[string]bool{"carol@remote.example": true},
		}
		federation.resolved = map[string]*activitypub.Actor{
			"carol@remote.example": {
				BaseObject:        activitypub.BaseObject{ID: "https://remote.example/users/carol"},
				PreferredUsername: "carol",
			},
		}

		created, err := service.CreateNote(context.Background(), &CreateNoteCommand{
			AuthorID:     "alice",
			Content:      "hello @carol@remote.example",
			Visibility:   VisibilityPublic,
			ToRecipients: []string{activitypub.PublicAddress},
			CcRecipients: []string{"https://example.com/users/alice/followers"},
		})
		require.NoError(t, err)
		require.NotNil(t, created)
		assert.Equal(t, []string{"https://remote.example/users/carol"}, created.Note.Mentions)
		require.Len(t, notifier.cmds, 0)

		require.Len(t, federation.activities, 1)
		federatedNote, ok := federation.activities[0].Object.(*activitypub.Note)
		require.True(t, ok)
		require.Len(t, federatedNote.Tag, 1)
		assert.Equal(t, "https://remote.example/users/carol", federatedNote.Tag[0].Href)
		assert.Equal(t, "@carol@remote.example", federatedNote.Tag[0].Name)
		assert.Equal(t, []string{"https://example.com/users/alice/followers", "https://remote.example/users/carol"}, federatedNote.CC)
	})

	t.Run("direct mentions remote actor in to", func(t *testing.T) {
		service, _, federation, notifier, _ := newNotesServiceHarness(t)
		service.accountRepo = &stubAccountRepo{
			domain:  "example.com",
			missing: map[string]bool{"carol@remote.example": true},
		}
		federation.resolved = map[string]*activitypub.Actor{
			"carol@remote.example": {
				BaseObject:        activitypub.BaseObject{ID: "https://remote.example/users/carol"},
				PreferredUsername: "carol",
			},
		}

		created, err := service.CreateNote(context.Background(), &CreateNoteCommand{
			AuthorID:   "alice",
			Content:    "hello @carol@remote.example",
			Visibility: models.VisibilityDirect,
		})
		require.NoError(t, err)
		require.NotNil(t, created)
		assert.Equal(t, []string{"https://remote.example/users/carol"}, created.Note.Mentions)
		require.Len(t, notifier.cmds, 0)

		require.Len(t, federation.activities, 1)
		federatedNote, ok := federation.activities[0].Object.(*activitypub.Note)
		require.True(t, ok)
		assert.Equal(t, []string{"https://remote.example/users/carol"}, federatedNote.To)
		assert.Empty(t, federatedNote.CC)
	})
}

func TestService_round15_html_by_contract_is_sanitized_at_write_time(t *testing.T) {
	service, _, _, _, _ := newNotesServiceHarness(t)
	ctx := context.Background()

	payload := `<script>alert(1)</script>` +
		`<img src=x onerror=alert(1)>` +
		`<a href="javascript:alert(1)">x</a>` +
		`<a href="https://example.com" onclick="alert(1)">ok</a>` +
		`<b>bold</b>`

	created, err := service.CreateNote(ctx, &CreateNoteCommand{
		AuthorID:     "alice",
		Content:      payload,
		Visibility:   VisibilityPublic,
		ToRecipients: []string{activitypub.PublicAddress},
		CcRecipients: []string{"https://example.com/users/alice/followers"},
	})
	require.NoError(t, err)
	require.NotNil(t, created)
	require.NotNil(t, created.Note)
	require.NotNil(t, created.Note.Note)

	require.NotContains(t, created.Note.Content, "<script")
	require.NotContains(t, created.Note.Content, "onerror=")
	require.NotContains(t, created.Note.Content, "onclick=")
	require.NotContains(t, created.Note.Content, "javascript:")

	require.NotContains(t, created.Note.Note.Content, "<script")
	require.NotContains(t, created.Note.Note.Content, "onerror=")
	require.NotContains(t, created.Note.Note.Content, "onclick=")
	require.NotContains(t, created.Note.Note.Content, "javascript:")

	updated, err := service.UpdateNote(ctx, &UpdateNoteCommand{
		StatusID:  created.Note.StatusID,
		Content:   payload,
		Sensitive: true,
		Language:  "en",
		UpdaterID: "alice",
	})
	require.NoError(t, err)
	require.NotNil(t, updated)
	require.NotNil(t, updated.Note)

	require.NotContains(t, updated.Note.Content, "<script")
	require.NotContains(t, updated.Note.Content, "onerror=")
	require.NotContains(t, updated.Note.Content, "onclick=")
	require.NotContains(t, updated.Note.Content, "javascript:")
}

func TestService_round15_local_note_object_projection_lifecycle(t *testing.T) {
	ctx := context.Background()
	statusRepo := testinginmemory.NewStatusRepository()
	objectRepo := testingmocks.NewMockObjectRepository()
	accountRepo := &stubAccountRepo{domain: "example.com"}
	objectRepo.On("CreateObject", mock.Anything, mock.Anything).Return(nil).Once()
	objectRepo.On("UpdateObjectWithHistory", mock.Anything, mock.Anything, "https://example.com/users/alice").Return(nil).Once()
	objectRepo.On("ReplaceObjectWithTombstone", mock.Anything, mock.Anything, activitypub.NoteType, "https://example.com/users/alice", "https://example.com/users/alice", true).Return(nil).Once()
	service := NewService(
		statusRepo,
		accountRepo,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		objectRepo,
		nil,
		nil,
		nil,
		nil,
		&stubPublisher{},
		&stubAnalytics{},
		&stubFederation{},
		nil,
		&stubNotificationService{},
		zap.NewNop(),
		"example.com",
	)

	created, err := service.CreateNote(ctx, &CreateNoteCommand{
		AuthorID:     "alice",
		Content:      "fresh local projection",
		Visibility:   VisibilityPublic,
		ToRecipients: []string{activitypub.PublicAddress},
		CcRecipients: []string{"https://example.com/users/alice/followers"},
	})
	require.NoError(t, err)
	require.NotNil(t, created)
	require.NotNil(t, created.Note)
	require.NotNil(t, created.Note.Note)

	objectID := created.Note.Note.ID
	objectRepo.AssertCalled(t, "CreateObject", mock.Anything, mock.MatchedBy(func(value any) bool {
		note, ok := value.(*activitypub.Note)
		return ok && note.ID == objectID && note.Content == "fresh local projection"
	}))

	updated, err := service.UpdateNote(ctx, &UpdateNoteCommand{
		StatusID:    created.Note.StatusID,
		Content:     "edited local projection",
		Sensitive:   true,
		SpoilerText: "cw",
		Language:    "en",
		UpdaterID:   "alice",
	})
	require.NoError(t, err)
	require.NotNil(t, updated)
	objectRepo.AssertCalled(t, "UpdateObjectWithHistory", mock.Anything, mock.MatchedBy(func(value any) bool {
		note, ok := value.(*activitypub.Note)
		return ok && note.ID == objectID && note.Content == "edited local projection" && note.Sensitive && note.Summary == "cw"
	}), "https://example.com/users/alice")

	require.NoError(t, service.DeleteNote(ctx, &DeleteNoteCommand{
		StatusID:  created.Note.StatusID,
		DeleterID: "alice",
	}))
	objectRepo.AssertCalled(t, "ReplaceObjectWithTombstone", mock.Anything, objectID, activitypub.NoteType, "https://example.com/users/alice", "https://example.com/users/alice", true)
	objectRepo.AssertExpectations(t)
}

func TestService_round15_local_note_projection_helpers_noop_without_object_repo(t *testing.T) {
	service := &Service{domainName: "example.com"}
	status := &models.Status{
		StatusID:       "status-1",
		AuthorID:       "https://example.com/users/alice",
		AuthorUsername: "alice",
		Note: &activitypub.Note{
			BaseObject: activitypub.BaseObject{
				ID:   "https://example.com/users/alice/statuses/status-1",
				Type: activitypub.NoteType,
			},
			Content:      "hello",
			AttributedTo: "https://example.com/users/alice",
		},
	}

	projected, objectID, err := service.localNoteProjectionPayload(status)
	require.NoError(t, err)
	require.Nil(t, projected)
	require.Empty(t, objectID)

	require.NoError(t, service.projectLocalNoteObjectOnCreate(context.Background(), status))
	require.NoError(t, service.syncLocalNoteObjectProjectionOnUpdate(context.Background(), status, "alice"))
	require.NoError(t, service.replaceLocalNoteObjectProjectionWithTombstone(context.Background(), status, "alice"))
}

func TestService_round15_local_note_projection_payload_validation(t *testing.T) {
	service := &Service{
		domainName: "example.com",
		objectRepo: testingmocks.NewMockObjectRepository(),
	}

	note, objectID, err := service.localNoteProjectionPayload(nil)
	require.Error(t, err)
	require.Nil(t, note)
	require.Empty(t, objectID)

	statusWithoutNote := &models.Status{StatusID: "status-1"}
	note, objectID, err = service.localNoteProjectionPayload(statusWithoutNote)
	require.Error(t, err)
	require.Nil(t, note)
	require.Empty(t, objectID)

	statusWithoutID := &models.Status{
		StatusID: "status-2",
		Note: &activitypub.Note{
			BaseObject: activitypub.BaseObject{Type: activitypub.NoteType},
		},
	}
	note, objectID, err = service.localNoteProjectionPayload(statusWithoutID)
	require.Error(t, err)
	require.Nil(t, note)
	require.Empty(t, objectID)

	validStatus := &models.Status{
		StatusID: "status-3",
		Note: &activitypub.Note{
			BaseObject: activitypub.BaseObject{
				ID:   "https://example.com/users/alice/statuses/status-3",
				Type: activitypub.NoteType,
			},
			Content:      "hello",
			AttributedTo: "https://example.com/users/alice",
		},
	}
	note, objectID, err = service.localNoteProjectionPayload(validStatus)
	require.NoError(t, err)
	require.NotNil(t, note)
	require.Equal(t, validStatus.Note.ID, objectID)
}

func TestService_round15_local_actor_id_helper(t *testing.T) {
	service := &Service{domainName: "example.com"}
	require.Empty(t, service.localActorID(""))
	require.Equal(t, "https://example.com/users/alice", service.localActorID("alice"))
}

func TestService_round15_cleanup_failed_local_projection_and_parent_helpers(t *testing.T) {
	ctx := context.Background()
	objectRepo := testingmocks.NewMockObjectRepository()
	service := &Service{
		domainName: "example.com",
		objectRepo: objectRepo,
		logger:     zap.NewNop(),
	}

	objectRepo.On("DeleteObject", mock.Anything, "https://example.com/users/alice/statuses/status-1").Return(nil).Once()
	service.cleanupFailedLocalNoteObjectProjection(ctx, &models.Status{
		StatusID: "status-1",
		Note: &activitypub.Note{
			BaseObject: activitypub.BaseObject{
				ID:   "https://example.com/users/alice/statuses/status-1",
				Type: activitypub.NoteType,
			},
			AttributedTo: "https://example.com/users/alice",
		},
	})
	service.cleanupFailedLocalNoteObjectProjection(ctx, &models.Status{StatusID: "status-2"})
	service.cleanupFailedLocalNoteObjectProjection(ctx, &models.Status{
		StatusID: "status-3",
		Note: &activitypub.Note{
			BaseObject: activitypub.BaseObject{Type: activitypub.NoteType},
		},
	})
	objectRepo.AssertExpectations(t)

	parentFromNote := &models.Status{
		StatusID: "status-1",
		AuthorID: "https://example.com/users/alice",
		Note: &activitypub.Note{
			BaseObject: activitypub.BaseObject{
				ID:   "https://example.com/users/alice/statuses/status-1",
				Type: activitypub.NoteType,
			},
			AttributedTo: "https://example.com/users/alice",
		},
	}
	require.Equal(t, parentFromNote.Note.ID, service.replyParentObjectID(parentFromNote))
	require.Equal(t, "https://example.com/users/alice", service.replyParentActorID(parentFromNote))

	parentFromURL := &models.Status{
		StatusID: "status-2",
		URLs:     []string{"mailto:ignore", "https://remote.example/objects/2"},
		AuthorID: "https://remote.example/users/bob",
	}
	require.Equal(t, "https://remote.example/objects/2", service.replyParentObjectID(parentFromURL))
	require.Equal(t, "https://remote.example/users/bob", service.replyParentActorID(parentFromURL))

	parentFromStatusID := &models.Status{
		StatusID: "https://remote.example/objects/3",
		Note:     &activitypub.Note{},
	}
	require.Equal(t, "https://remote.example/objects/3", service.replyParentObjectID(parentFromStatusID))

	localFallback := &models.Status{
		StatusID:       "status-4",
		AuthorUsername: "alice",
		AuthorID:       "https://example.com/users/alice",
	}
	require.Equal(t, "https://example.com/users/alice/statuses/status-4", service.replyParentObjectID(localFallback))

	require.Empty(t, service.replyParentObjectID(&models.Status{StatusID: "status-5"}))
	require.Empty(t, service.replyParentActorID(&models.Status{}))
}

func TestService_round15_resolve_viewer_actor_id_variants(t *testing.T) {
	service := &Service{domainName: "example.com"}

	username, actorID := service.resolveViewerActorID("")
	require.Empty(t, username)
	require.Empty(t, actorID)

	username, actorID = service.resolveViewerActorID("alice")
	require.Equal(t, "alice", username)
	require.Equal(t, "https://example.com/users/alice", actorID)

	username, actorID = service.resolveViewerActorID("https://remote.example/users/bob/")
	require.Equal(t, "bob", username)
	require.Equal(t, "https://remote.example/users/bob", actorID)

	username, actorID = service.resolveViewerActorID("https://remote.example")
	require.Equal(t, "https://remote.example", username)
	require.Equal(t, "https://remote.example", actorID)

	noDomain := &Service{}
	username, actorID = noDomain.resolveViewerActorID("carol")
	require.Equal(t, "carol", username)
	require.Equal(t, "carol", actorID)
}

func TestService_round15_community_note_content_is_escaped_at_write_time(t *testing.T) {
	service, _, _, _, _ := newNotesServiceHarness(t)
	ctx := context.Background()

	note := &storage.CommunityNote{
		ObjectID:   "obj-1",
		ObjectType: "Note",
		AuthorID:   "https://example.com/users/alice",
		Content:    `<img src=x onerror=alert(1)>hello`,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	created, err := service.CreateCommunityNote(ctx, &CreateCommunityNoteCommand{Note: note})
	require.NoError(t, err)
	require.NotNil(t, created)
	require.NotNil(t, created.Note)

	require.Contains(t, created.Note.Content, "&lt;img")
	require.NotContains(t, created.Note.Content, "<img")
}

func TestService_round15_community_note_content_is_escaped_for_read_boundaries(t *testing.T) {
	raw := &storage.CommunityNote{
		ID:      "legacy-note",
		Content: `<script>alert(1)</script><b>legacy</b>`,
	}

	safe := safeCommunityNoteForPresentation(raw)
	require.NotNil(t, safe)
	require.NotSame(t, raw, safe)
	require.NotContains(t, safe.Content, "<script")
	require.Contains(t, safe.Content, "&lt;b&gt;legacy&lt;/b&gt;")
	require.Contains(t, raw.Content, "<script>")

	notes := safeCommunityNotesForPresentation([]*storage.CommunityNote{raw, nil})
	require.Len(t, notes, 2)
	require.NotContains(t, notes[0].Content, "<script")
	require.Nil(t, notes[1])
	require.Nil(t, safeCommunityNotesForPresentation(nil))
}

func TestService_round15_view_permissions_and_timelines(t *testing.T) {
	service, _, _, _, _ := newNotesServiceHarness(t)

	ctx := context.Background()

	_, err := service.GetNoteWithViewer(ctx, &GetNoteQuery{})
	assert.ErrorIs(t, err, ErrStatusIDRequired)

	// Private note visible to follower.
	note, err := service.GetNoteWithViewer(ctx, &GetNoteQuery{
		StatusID: "private-status",
		ViewerID: "bob",
	})
	require.NoError(t, err)
	require.NotNil(t, note)

	// Private note is hidden from unauthenticated viewers.
	_, err = service.GetNote(ctx, "private-status")
	assert.ErrorIs(t, err, ErrStatusNotFound)

	// Direct note visible when mentioned.
	note, err = service.GetNoteWithViewer(ctx, &GetNoteQuery{
		StatusID: "direct-mentionbob",
		ViewerID: "bob",
	})
	require.NoError(t, err)
	require.NotNil(t, note)

	// Direct note is hidden from unauthenticated viewers (even if it mentions a user).
	_, err = service.GetNote(ctx, "direct-mentionbob")
	assert.ErrorIs(t, err, ErrStatusNotFound)

	// Direct note hidden from non-recipients.
	_, err = service.GetNoteWithViewer(ctx, &GetNoteQuery{
		StatusID: "direct-hidden",
		ViewerID: "bob",
	})
	assert.ErrorIs(t, err, ErrStatusNotFound)

	// Deleted notes should not be returned.
	_, err = service.GetNote(ctx, "deleted-status")
	assert.ErrorIs(t, err, ErrStatusNotFound)

	// Cover ListNotes for public + direct + list.
	_, err = service.ListNotes(ctx, &ListNotesQuery{
		TimelineType: VisibilityPublic,
		Pagination:   interfaces.PaginationOptions{Limit: 2},
	})
	require.NoError(t, err)

	_, err = service.ListNotes(ctx, &ListNotesQuery{
		TimelineType: "direct",
		ViewerID:     "bob",
		Pagination:   interfaces.PaginationOptions{Limit: 1},
	})
	require.NoError(t, err)

	_, err = service.ListNotes(ctx, &ListNotesQuery{
		TimelineType: "list",
		ViewerID:     "bob",
		ListID:       "list-1",
		Pagination:   interfaces.PaginationOptions{Limit: 0},
	})
	require.NoError(t, err)

	// Directly hit unsupported timeline branch.
	_, err = service.routeTimelineQuery(ctx, &ListNotesQuery{TimelineType: "nope"})
	assert.ErrorIs(t, err, ErrUnsupportedTimelineType)

	// Unknown visibility defaults to deny.
	canView, err := service.checkViewPermissions(ctx, &models.Status{Visibility: "weird"}, "bob")
	require.NoError(t, err)
	assert.False(t, canView)
}

func TestService_round15_reblog_enforces_viewer_access(t *testing.T) {
	service, _, _, _, _ := newNotesServiceHarness(t)
	ctx := context.Background()
	originalRepo := service.noteRepo
	service.noteRepo = &delegatingStatusRepo{
		StatusRepository: originalRepo,
		getStatus: func(ctx context.Context, statusID string) (*models.Status, error) {
			if statusID == "missing-status" {
				return nil, pkgerrors.ItemNotFound("status")
			}
			return originalRepo.GetStatus(ctx, statusID)
		},
	}

	relationships := testingmocks.NewMockRelationshipRepository()
	service.relationshipRepo = relationships

	relationships.On("IsFollowing", mock.Anything, "mallory", "alice").Return(false, nil).Once()
	result, err := service.ReblogNote(ctx, &ReblogNoteCommand{
		StatusID:    "private-status",
		RebloggerID: "mallory",
	})
	require.ErrorIs(t, err, ErrStatusNotFound)
	require.Nil(t, result, "visibility denial must not return the protected status content")

	relationships.On("IsFollowing", mock.Anything, "bob", "alice").Return(true, nil).Once()
	result, err = service.ReblogNote(ctx, &ReblogNoteCommand{
		StatusID:    "private-status",
		RebloggerID: "bob",
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "hello", result.Status.Content)

	result, err = service.ReblogNote(ctx, &ReblogNoteCommand{
		StatusID:    "direct-mentionbob",
		RebloggerID: "bob",
	})
	require.ErrorIs(t, err, ErrReblogStatus)
	require.Nil(t, result)

	deletedResult, deletedErr := service.ReblogNote(ctx, &ReblogNoteCommand{
		StatusID:    "deleted-status",
		RebloggerID: "bob",
	})
	missingResult, missingErr := service.ReblogNote(ctx, &ReblogNoteCommand{
		StatusID:    "missing-status",
		RebloggerID: "bob",
	})
	require.ErrorIs(t, deletedErr, ErrStatusNotFound)
	require.ErrorIs(t, missingErr, ErrStatusNotFound)
	require.Nil(t, deletedResult)
	require.Nil(t, missingResult)

	relationships.AssertExpectations(t)
}

func TestService_round15_engagements_bookmarks_pins_mutes(t *testing.T) {
	service, _, _, _, _ := newNotesServiceHarness(t)

	ctx := context.Background()

	_, err := service.BookmarkNote(ctx, &BookmarkNoteCommand{
		StatusID:     "status-1",
		BookmarkerID: "alice",
	})
	require.NoError(t, err)

	_, err = service.UnbookmarkNote(ctx, &UnbookmarkNoteCommand{
		StatusID:       "status-1",
		UnbookmarkerID: "alice",
	})
	require.NoError(t, err)

	_, err = service.GetBookmarks(ctx, &GetBookmarksQuery{
		UserID:     "alice",
		Pagination: interfaces.PaginationOptions{Limit: 1},
	})
	require.NoError(t, err)

	_, err = service.LikeNote(ctx, &LikeNoteCommand{StatusID: "status-1", LikerID: "alice"})
	require.NoError(t, err)

	_, err = service.UnlikeNote(ctx, &UnlikeNoteCommand{StatusID: "status-1", UnlikerID: "alice"})
	require.NoError(t, err)

	_, err = service.GetLikers(ctx, &GetLikersQuery{StatusID: "status-1", Pagination: interfaces.PaginationOptions{Limit: 2}})
	require.NoError(t, err)

	_, err = service.ReblogNote(ctx, &ReblogNoteCommand{StatusID: "status-1", RebloggerID: "bob"})
	require.NoError(t, err)

	_, err = service.UnreblogNote(ctx, &UnreblogNoteCommand{StatusID: "status-1", UnrebloggerID: "bob"})
	require.NoError(t, err)

	_, err = service.GetRebloggers(ctx, &GetRebloggersQuery{StatusID: "status-1", Pagination: interfaces.PaginationOptions{Limit: 2}})
	require.NoError(t, err)

	_, err = service.PinNote(ctx, &PinNoteCommand{StatusID: "status-1", PinnerID: "alice"})
	require.NoError(t, err)

	_, err = service.UnpinNote(ctx, &UnpinNoteCommand{StatusID: "status-1", PinnerID: "alice"})
	require.NoError(t, err)

	_, err = service.PinNote(ctx, &PinNoteCommand{StatusID: "status-1", PinnerID: "bob"})
	assert.Error(t, err)

	_, err = service.MuteNote(ctx, &MuteNoteCommand{StatusID: "status-1", MuterID: "bob", DurationSeconds: 0})
	require.NoError(t, err)

	_, err = service.UnmuteNote(ctx, &UnmuteNoteCommand{StatusID: "status-1", MuterID: "bob"})
	require.NoError(t, err)

	// Cover idempotent branches in mute/unmute.
	service.conversationRepo = &stubConversationRepo{
		createMuteErr: fmt.Errorf("already muted"),
		deleteMuteErr: fmt.Errorf("not found"),
	}
	_, err = service.MuteNote(ctx, &MuteNoteCommand{StatusID: "status-1", MuterID: "bob"})
	require.NoError(t, err)
	_, err = service.UnmuteNote(ctx, &UnmuteNoteCommand{StatusID: "status-1", MuterID: "bob"})
	require.NoError(t, err)
}

func TestService_round15_scheduled_search_community_and_counts(t *testing.T) {
	service, _, _, _, _ := newNotesServiceHarness(t)

	ctx := context.Background()

	_, err := service.CreateScheduledNote(ctx, &CreateScheduledNoteCommand{
		AuthorID:    "alice",
		Content:     "scheduled",
		Visibility:  VisibilityPublic,
		ScheduledAt: time.Now().Add(10 * time.Minute),
		Language:    "en",
		Sensitive:   false,
		InReplyToID: "",
		MediaIDs:    nil,
	})
	require.NoError(t, err)

	_, err = service.CreateScheduledNote(ctx, &CreateScheduledNoteCommand{
		AuthorID:    "alice",
		Content:     "scheduled",
		Visibility:  VisibilityPublic,
		ScheduledAt: time.Now().Add(-10 * time.Minute),
	})
	assert.ErrorIs(t, err, ErrScheduledTimeInPast)

	_, err = service.GetSearchSuggestions(ctx, &GetSearchSuggestionsQuery{Prefix: "go", Limit: 2})
	require.NoError(t, err)

	createdNote := &storage.CommunityNote{
		ID:         "note-1",
		ObjectID:   "object-1",
		ObjectType: "status",
		AuthorID:   "alice",
		Content:    "context",
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	_, err = service.CreateCommunityNote(ctx, &CreateCommunityNoteCommand{Note: createdNote})
	require.NoError(t, err)

	_, err = service.GetVisibleCommunityNotes(ctx, &GetVisibleCommunityNotesQuery{ObjectID: "object-1"})
	require.NoError(t, err)

	_, err = service.GetCommunityNote(ctx, &GetCommunityNoteQuery{NoteID: "note-1"})
	require.NoError(t, err)

	_, err = service.CreateCommunityNoteVote(ctx, &CreateCommunityNoteVoteCommand{
		Vote: &storage.CommunityNoteVote{
			NoteID:   "note-1",
			VoterID:  "bob",
			VoteType: "helpful",
			Helpful:  true,
			Weight:   1,
		},
	})
	require.NoError(t, err)

	_, err = service.GetCommunityNotesByAuthor(ctx, &GetCommunityNotesByAuthorQuery{AuthorID: "alice", Limit: 2})
	require.NoError(t, err)

	_, err = service.GetUpdateHistory(ctx, &GetUpdateHistoryQuery{StatusID: "status-1", Limit: 2})
	require.NoError(t, err)

	_, err = service.GetFavoritedNotes(ctx, &ListNotesQuery{ViewerID: "alice", Pagination: interfaces.PaginationOptions{Limit: 2}})
	require.NoError(t, err)

	_, err = service.CountNotesByAuthor(ctx, "alice")
	require.NoError(t, err)

	_, err = service.GetUserTimeline(ctx, "alice", interfaces.PaginationOptions{Limit: 2})
	require.NoError(t, err)

	_, err = service.CountReplies(ctx, "status-1")
	require.NoError(t, err)

	_, err = service.GetBoostCount(ctx, "status-1")
	require.NoError(t, err)

	_, err = service.GetLikeCount(ctx, "status-1")
	require.NoError(t, err)

	_, err = service.HasLiked(ctx, "alice", "status-1")
	require.NoError(t, err)

	_, err = service.HasReblogged(ctx, "alice", "status-1")
	require.NoError(t, err)

	_, err = service.IsBookmarked(ctx, "alice", "status-1")
	require.NoError(t, err)
}

func TestService_round15_getTimeline_and_routeTimelineQuery_cases(t *testing.T) {
	service, _, _, _, _ := newNotesServiceHarness(t)

	ctx := context.Background()

	_, err := service.GetTimeline(ctx, &GetTimelineQuery{
		UserID:   "alice",
		Timeline: "public",
		Limit:    2,
	})
	require.NoError(t, err)

	_, err = service.routeTimelineQuery(ctx, &ListNotesQuery{TimelineType: "home"})
	assert.ErrorIs(t, err, ErrHomeTimelineRequiresViewerID)

	// Use type assertion to access concrete method for test setup
	if concreteRepo, ok := service.noteRepo.(*repositories.StatusRepository); ok {
		concreteRepo.SetRelationshipRepository(nil)
	}
	_, err = service.routeTimelineQuery(ctx, &ListNotesQuery{TimelineType: "home", ViewerID: "alice", Pagination: interfaces.PaginationOptions{Limit: 1}})
	require.NoError(t, err)

	_, err = service.routeTimelineQuery(ctx, &ListNotesQuery{TimelineType: "user"})
	assert.ErrorIs(t, err, ErrUserTimelineRequiresAuthorID)

	_, err = service.routeTimelineQuery(ctx, &ListNotesQuery{TimelineType: "user", AuthorID: "alice", Pagination: interfaces.PaginationOptions{Limit: 1}})
	require.NoError(t, err)

	_, err = service.routeTimelineQuery(ctx, &ListNotesQuery{TimelineType: "conversations"})
	assert.ErrorIs(t, err, ErrConversationsTimelineRequiresConversationID)

	_, err = service.routeTimelineQuery(ctx, &ListNotesQuery{TimelineType: "conversations", ConversationID: "conv-1", Pagination: interfaces.PaginationOptions{Limit: 1}})
	require.NoError(t, err)

	_, err = service.routeTimelineQuery(ctx, &ListNotesQuery{TimelineType: "direct"})
	assert.ErrorIs(t, err, ErrDirectTimelineRequiresViewerID)

	_, err = service.routeTimelineQuery(ctx, &ListNotesQuery{TimelineType: "hashtag"})
	assert.ErrorIs(t, err, ErrHashtagTimelineRequiresHashtag)

	_, err = service.routeTimelineQuery(ctx, &ListNotesQuery{TimelineType: "hashtag", Hashtag: "#Go", Pagination: interfaces.PaginationOptions{Limit: 1}})
	require.NoError(t, err)

	_, err = service.routeTimelineQuery(ctx, &ListNotesQuery{TimelineType: "list"})
	assert.ErrorIs(t, err, ErrListTimelineRequiresListID)

	_, err = service.routeTimelineQuery(ctx, &ListNotesQuery{TimelineType: "local", Pagination: interfaces.PaginationOptions{Limit: 1}})
	require.NoError(t, err)
}

func TestService_round15_validation_hydration_and_delete_branches(t *testing.T) {
	service, _, _, _, _ := newNotesServiceHarness(t)

	ctx := context.Background()

	assert.ErrorIs(t, service.validateCreateCommand(ctx, nil), ErrNotesValidationFailed)
	assert.Error(t, service.validateCreateCommand(ctx, &CreateNoteCommand{AuthorID: "alice", Content: ""}))
	assert.Error(t, service.validateCreateCommand(ctx, &CreateNoteCommand{AuthorID: "", Content: "hi"}))

	longSpoiler := strings.Repeat("x", 501)
	assert.ErrorIs(t, service.validateCreateCommand(ctx, &CreateNoteCommand{
		AuthorID:    "alice",
		Content:     "ok",
		Visibility:  VisibilityPublic,
		SpoilerText: longSpoiler,
	}), ErrSpoilerTextValidationFailed)

	// Exercise in-reply-to validation + comment analytics path.
	_, err := service.CreateNote(ctx, &CreateNoteCommand{
		AuthorID:     "alice",
		Content:      "reply",
		Visibility:   VisibilityPublic,
		InReplyToID:  "status-1",
		ToRecipients: []string{"https://www.w3.org/ns/activitystreams#Public"},
		CcRecipients: []string{"https://example.com/users/alice/followers"},
	})
	require.NoError(t, err)

	// ensureStatusHydrated should error on nil / missing IDs, and reload when needed.
	_, err = service.ensureStatusHydrated(ctx, nil)
	assert.Error(t, err)

	_, err = service.ensureStatusHydrated(ctx, &models.Status{})
	assert.Error(t, err)

	reloaded, err := service.ensureStatusHydrated(ctx, &models.Status{PK: "status#hydrated-1"})
	require.NoError(t, err)
	assert.NotEmpty(t, reloaded.StatusID)

	// Missing repository while reloading should error.
	serviceNoRepo, _, _, _, _ := newNotesServiceHarness(t)
	serviceNoRepo.noteRepo = nil
	_, err = serviceNoRepo.ensureStatusHydrated(ctx, &models.Status{PK: "status#needs-reload"})
	assert.Error(t, err)

	// Missing author username after hydration should error.
	serviceNoAccount, _, _, _, _ := newNotesServiceHarness(t)
	serviceNoAccount.accountRepo = nil
	_, err = serviceNoAccount.ensureStatusHydrated(ctx, &models.Status{
		StatusID: "missing-author",
		AuthorID: "",
		Note:     &activitypub.Note{BaseObject: activitypub.BaseObject{ID: "https://example.com/statuses/1"}},
	})
	assert.Error(t, err)

	// hydrateTimelineStatuses should drop invalid entries.
	service.logger = zap.NewNop()
	hydrated := service.hydrateTimelineStatuses(ctx, []*models.Status{nil, &models.Status{StatusID: "status-1"}})
	assert.Len(t, hydrated, 1)

	// DeleteNote branches: idempotent + admin + forbidden.
	require.NoError(t, service.DeleteNote(ctx, &DeleteNoteCommand{StatusID: "deleted-status", DeleterID: "alice"}))
	require.NoError(t, service.DeleteNote(ctx, &DeleteNoteCommand{StatusID: "status-1", DeleterID: "admin"}))
	assert.Error(t, service.DeleteNote(ctx, &DeleteNoteCommand{StatusID: "status-1", DeleterID: "bob"}))
}

func TestService_round15_federation_boost_and_reblog_branch_coverage(t *testing.T) {
	service, _, _, _, _ := newNotesServiceHarness(t)

	ctx := context.Background()

	// queueFederationDelivery early returns.
	service.queueFederationDelivery(ctx, &models.Status{}, "Create")
	service.federation = nil
	service.queueFederationDelivery(ctx, &models.Status{StatusID: "s1", Note: &activitypub.Note{}}, "Create")

	service.federation = &stubFederation{}
	service.domainName = ""
	service.queueFederationDelivery(ctx, &models.Status{StatusID: "s1", Note: &activitypub.Note{}}, "Create")

	// queueFederationTombstone: ID fallback, nil recipient lists, actor fallback.
	service.domainName = "example.com"
	service.queueFederationTombstone(ctx, &models.Status{
		StatusID:     "s1",
		AuthorID:     "https://example.com/users/alice",
		ToRecipients: nil,
		CcRecipients: nil,
		Note: &activitypub.Note{
			BaseObject:   activitypub.BaseObject{ID: ""},
			AttributedTo: "",
		},
	})

	// persistBoostStatus early returns.
	service.persistBoostStatus(ctx, nil, nil, nil)
	service.noteRepo = nil
	service.persistBoostStatus(ctx, &models.Status{StatusID: "s1"}, &storage.Account{}, &storage.Announce{})

	// createReblog branches.
	service.socialRepo = &socialRepoGetAnnounceErr{
		err: pkgerrors.NewStorageError(pkgerrors.CodeBadRequest, "bad announce"),
	}
	_, err := service.createReblog(ctx, "actor", "object", "", "")
	assert.ErrorIs(t, err, ErrCreateReblog)

	service.socialRepo = &stubSocialRepo{}
	service.noteRepo = nil
	announce, err := service.createReblog(ctx, "https://example.com/users/bob", "https://example.com/users/alice/statuses/status-1", "", "")
	require.NoError(t, err)
	require.NotNil(t, announce)

	service, _, _, _, _ = newNotesServiceHarness(t)
	_, err = service.createReblog(ctx, "https://example.com/users/bob", "", "", "")
	assert.ErrorIs(t, err, ErrReblogStatus)

	service, _, _, _, _ = newNotesServiceHarness(t)
	service.socialRepo = &stubSocialRepo{
		announces: map[string]*storage.Announce{
			"https://example.com/users/bob|https://example.com/users/alice/statuses/status-1": {ID: "announce-existing"},
		},
	}
	announce, err = service.createReblog(ctx, "https://example.com/users/bob", "https://example.com/users/alice/statuses/status-1", "", "")
	require.NoError(t, err)
	require.Equal(t, "announce-existing", announce.ID)

	// deleteReblog branches.
	service, _, _, _, _ = newNotesServiceHarness(t)
	service.socialRepo = &socialRepoDeleteAnnounceErr{err: fmt.Errorf("boom")}
	assert.ErrorIs(t, service.deleteReblog(ctx, "actor", "object", ""), ErrDeleteReblog)

	service, _, _, _, _ = newNotesServiceHarness(t)
	assert.ErrorIs(t, service.deleteReblog(ctx, "actor", "", ""), ErrDeleteReblog)

	// refreshStatus and buildStatusURL fallbacks.
	fallback := &models.Status{StatusID: "fallback"}
	service.noteRepo = nil
	assert.Equal(t, fallback, service.refreshStatus(ctx, "status-1", fallback))

	service.domainName = ""
	assert.Equal(t, "https://localhost/users/alice/statuses/status-1", service.buildStatusURL(&models.Status{StatusID: "status-1", AuthorUsername: "alice"}))
	assert.Equal(t, "https://example.com/users/bob/statuses/status-1", func() string {
		service.domainName = "example.com"
		return service.buildStatusURL(&models.Status{StatusID: "status-1", AuthorID: "https://example.com/users/bob"})
	}())
}

func TestService_round15_additional_branch_coverage(t *testing.T) {
	service, _, _, _, _ := newNotesServiceHarness(t)
	ctx := context.Background()

	// checkViewPermissions: public/unlisted, unauthenticated, author, direct recipients.
	canView, err := service.checkViewPermissions(ctx, &models.Status{Visibility: models.VisibilityPublic}, "")
	require.NoError(t, err)
	assert.True(t, canView)

	canView, err = service.checkViewPermissions(ctx, &models.Status{Visibility: models.VisibilityUnlisted}, "")
	require.NoError(t, err)
	assert.True(t, canView)

	canView, err = service.checkViewPermissions(ctx, &models.Status{Visibility: models.VisibilityPrivate, AuthorUsername: "alice"}, "")
	require.NoError(t, err)
	assert.False(t, canView)

	canView, err = service.checkViewPermissions(ctx, &models.Status{Visibility: models.VisibilityPrivate, AuthorUsername: "alice"}, "alice")
	require.NoError(t, err)
	assert.True(t, canView)

	canView, err = service.checkViewPermissions(ctx, &models.Status{
		Visibility:     models.VisibilityDirect,
		AuthorUsername: "alice",
		ToRecipients:   []string{"https://example.com/users/bob"},
	}, "bob")
	require.NoError(t, err)
	assert.True(t, canView)

	canView, err = service.checkViewPermissions(ctx, &models.Status{
		Visibility:     models.VisibilityDirect,
		AuthorUsername: "alice",
		CcRecipients:   []string{"https://example.com/users/bob"},
	}, "bob")
	require.NoError(t, err)
	assert.True(t, canView)

	// validateUpdateCommand error branches.
	assert.ErrorIs(t, service.validateUpdateCommand(ctx, &UpdateNoteCommand{Content: "   "}), ErrContentCannotBeEmpty)
	assert.ErrorIs(t, service.validateUpdateCommand(ctx, &UpdateNoteCommand{Content: strings.Repeat("x", 5001)}), ErrContentTooLong)
	assert.ErrorIs(t, service.validateUpdateCommand(ctx, &UpdateNoteCommand{Content: "ok", SpoilerText: strings.Repeat("x", 161)}), ErrSpoilerTextValidationFailed)

	// lookupParentStatus error branches.
	_, err = service.lookupParentStatus(ctx, "")
	assert.Error(t, err)
	service.noteRepo = nil
	_, err = service.lookupParentStatus(ctx, "status-1")
	assert.Error(t, err)

	// handlePollCreation no-op when options empty.
	service.handlePollCreation(ctx, &CreateNoteCommand{PollOptions: nil}, "status-1")

	// recordStatusCreationAnalytics no-ops and error logging.
	service.recordStatusCreationAnalytics(ctx, nil)
	service.analytics = nil
	service.recordStatusCreationAnalytics(ctx, &models.Status{StatusID: "status-1"})

	service.analytics = failingAnalytics{}
	service.recordStatusCreationAnalytics(ctx, &models.Status{
		StatusID:    "status-1",
		AuthorID:    "alice",
		InReplyToID: "parent",
		Hashtags:    []string{"go"},
		Note:        &activitypub.Note{BaseObject: activitypub.BaseObject{ID: "object-1"}},
	})
}

func TestService_M15_GetInteractionListsRequireVisibleStatus(t *testing.T) {
	ctx := context.Background()
	statusRepo := testingmocks.NewMockStatusRepositoryInterface()
	relationshipRepo := testingmocks.NewMockRelationshipRepository()
	service := &Service{
		noteRepo:         statusRepo,
		relationshipRepo: relationshipRepo,
		logger:           zap.NewNop(),
		domainName:       "example.com",
	}

	privateStatus := &models.Status{
		StatusID:       "private-1",
		Visibility:     models.VisibilityPrivate,
		AuthorUsername: "alice",
	}
	statusRepo.On("GetStatus", mock.Anything, "private-1").Return(privateStatus, nil).Twice()
	relationshipRepo.On("IsFollowing", mock.Anything, "bob", "alice").Return(false, nil).Twice()

	_, err := service.GetLikers(ctx, &GetLikersQuery{
		StatusID:   "private-1",
		ViewerID:   "bob",
		Pagination: interfaces.PaginationOptions{Limit: 2},
	})
	require.ErrorIs(t, err, ErrStatusNotFound)

	_, err = service.GetRebloggers(ctx, &GetRebloggersQuery{
		StatusID:   "private-1",
		ViewerID:   "bob",
		Pagination: interfaces.PaginationOptions{Limit: 2},
	})
	require.ErrorIs(t, err, ErrStatusNotFound)

	statusRepo.AssertExpectations(t)
	relationshipRepo.AssertExpectations(t)
}

func TestService_round15_createReblog_and_boost_idempotency(t *testing.T) {
	ctx := context.Background()
	actorURL := "https://example.com/users/bob"
	objectURL := "https://example.com/users/alice/statuses/status-1"

	// CreateAnnounce error should surface as ErrCreateReblog.
	{
		service, _, _, _, _ := newNotesServiceHarness(t)
		service.socialRepo = &socialRepoCreateAnnounceErr{err: fmt.Errorf("boom")}
		_, err := service.createReblog(ctx, actorURL, objectURL, "", "")
		assert.ErrorIs(t, err, ErrCreateReblog)
	}

	// CreateAnnounce already exists should fall back to GetAnnounce.
	{
		service, _, _, _, _ := newNotesServiceHarness(t)
		existing := &storage.Announce{ID: "announce-race", Actor: actorURL, Object: objectURL}
		service.socialRepo = &socialRepoAnnounceRace{
			stubSocialRepo: stubSocialRepo{announces: map[string]*storage.Announce{
				actorURL + "|" + objectURL: existing,
			}},
			notFoundCalls: 1,
		}

		announce, err := service.createReblog(ctx, actorURL, objectURL, "", "")
		require.NoError(t, err)
		require.NotNil(t, announce)
		assert.Equal(t, "announce-race", announce.ID)
	}

	// Reblog engagement idempotency: StatusRepository.ReblogStatus returns "already exists".
	{
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockUpdateBuilder := new(mocks.MockUpdateBuilder)
		mockQuery.On("Create").Return(pkgerrors.ItemAlreadyExists("status_engagement")).Once()
		setupPermissiveDynamormMocks(mockDB, mockQuery, mockUpdateBuilder, &permissiveQueryState{})

		service := newNotesServiceHarnessWithDB(t, transactionalMockDB{MockDB: mockDB})
		service.socialRepo = &stubSocialRepo{}

		announce, err := service.createReblog(ctx, actorURL, objectURL, "", "")
		require.NoError(t, err)
		require.NotNil(t, announce)
	}

	// Reblog engagement non-idempotent error should surface as ErrReblogStatus.
	{
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockUpdateBuilder := new(mocks.MockUpdateBuilder)
		mockQuery.On("Create").Return(fmt.Errorf("boom")).Once()
		setupPermissiveDynamormMocks(mockDB, mockQuery, mockUpdateBuilder, &permissiveQueryState{})

		service := newNotesServiceHarnessWithDB(t, transactionalMockDB{MockDB: mockDB})
		service.socialRepo = &stubSocialRepo{}

		_, err := service.createReblog(ctx, actorURL, objectURL, "", "")
		assert.ErrorIs(t, err, ErrReblogStatus)
	}

	// persistBoostStatus idempotency: CreateBoostStatus returns "already exists".
	{
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockUpdateBuilder := new(mocks.MockUpdateBuilder)
		mockQuery.On("Create").Return(pkgerrors.ItemAlreadyExists("boost_status")).Once()
		setupPermissiveDynamormMocks(mockDB, mockQuery, mockUpdateBuilder, &permissiveQueryState{})

		service := newNotesServiceHarnessWithDB(t, transactionalMockDB{MockDB: mockDB})
		boost := service.persistBoostStatus(ctx,
			&models.Status{StatusID: "status-1", AuthorID: "https://example.com/users/alice"},
			&storage.Account{
				User:  &storage.User{Username: "bob"},
				Actor: &activitypub.Actor{BaseObject: activitypub.BaseObject{ID: actorURL}},
			},
			&storage.Announce{ID: "announce-1"},
		)
		assert.Nil(t, boost)
	}
}

func TestService_round15_getFavoritedNotes_error_branches(t *testing.T) {
	ctx := context.Background()

	{
		service, _, _, _, _ := newNotesServiceHarness(t)
		_, err := service.GetFavoritedNotes(ctx, &ListNotesQuery{})
		assert.ErrorIs(t, err, ErrViewerIDRequiredForFavoritedTimeline)
	}

	{
		service, _, _, _, _ := newNotesServiceHarness(t)
		service.accountRepo = &stubAccountRepoError{err: fmt.Errorf("boom")}
		_, err := service.GetFavoritedNotes(ctx, &ListNotesQuery{ViewerID: "alice"})
		assert.ErrorIs(t, err, ErrGetViewerAccount)
	}

	{
		service, _, _, _, _ := newNotesServiceHarness(t)
		service.likeRepo = nil
		_, err := service.GetFavoritedNotes(ctx, &ListNotesQuery{ViewerID: "alice"})
		assert.ErrorIs(t, err, ErrLikeRepositoryNotAvailable)
	}

	{
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockUpdateBuilder := new(mocks.MockUpdateBuilder)
		mockQuery.On("All", mock.Anything).Return(fmt.Errorf("boom")).Once()
		setupPermissiveDynamormMocks(mockDB, mockQuery, mockUpdateBuilder, &permissiveQueryState{})

		service := newNotesServiceHarnessWithDB(t, transactionalMockDB{MockDB: mockDB})
		_, err := service.GetFavoritedNotes(ctx, &ListNotesQuery{
			ViewerID:     "alice",
			Pagination:   interfaces.PaginationOptions{Limit: 2},
			TimelineType: "favorited",
		})
		assert.ErrorIs(t, err, ErrGetLikedObjects)
	}

	{
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockUpdateBuilder := new(mocks.MockUpdateBuilder)
		mockQuery.On("BatchGet", mock.Anything, mock.Anything).Return(fmt.Errorf("boom")).Once()
		setupPermissiveDynamormMocks(mockDB, mockQuery, mockUpdateBuilder, &permissiveQueryState{})

		service := newNotesServiceHarnessWithDB(t, transactionalMockDB{MockDB: mockDB})
		_, err := service.GetFavoritedNotes(ctx, &ListNotesQuery{ViewerID: "alice", Pagination: interfaces.PaginationOptions{Limit: 2}})
		assert.ErrorIs(t, err, ErrGetStatuses)
	}
}

func TestService_round15_viewerLikeLookupIDs(t *testing.T) {
	service, _, _, _, _ := newNotesServiceHarness(t)

	account, err := service.accountRepo.GetAccount(context.Background(), "alice")
	require.NoError(t, err)

	lookupIDs := service.viewerLikeLookupIDs(account, "alice")
	require.Equal(t, []string{
		"https://example.com/users/alice",
		"alice",
	}, lookupIDs)
}

func TestService_round15_bookmarks_update_history_and_boost_delete_edges(t *testing.T) {
	ctx := context.Background()

	// GetBookmarks empty branch.
	{
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockUpdateBuilder := new(mocks.MockUpdateBuilder)
		mockQuery.On("All", mock.Anything).Return(nil).Once()
		setupPermissiveDynamormMocks(mockDB, mockQuery, mockUpdateBuilder, &permissiveQueryState{})

		service := newNotesServiceHarnessWithDB(t, transactionalMockDB{MockDB: mockDB})
		result, err := service.GetBookmarks(ctx, &GetBookmarksQuery{UserID: "alice"})
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Empty(t, result.Notes)
	}

	// BookmarkNote/UnbookmarkNote should error when bookmark repository is missing.
	{
		service, _, _, _, _ := newNotesServiceHarness(t)
		service.bookmarkRepo = nil
		_, err := service.BookmarkNote(ctx, &BookmarkNoteCommand{StatusID: "status-1", BookmarkerID: "alice"})
		assert.ErrorIs(t, err, ErrBookmarkStatus)

		_, err = service.UnbookmarkNote(ctx, &UnbookmarkNoteCommand{StatusID: "status-1", UnbookmarkerID: "alice"})
		assert.ErrorIs(t, err, ErrUnbookmarkStatus)
	}

	// GetUpdateHistory: required param + URL path.
	{
		service, _, _, _, _ := newNotesServiceHarness(t)
		_, err := service.GetUpdateHistory(ctx, &GetUpdateHistoryQuery{})
		assert.ErrorIs(t, err, ErrStatusIDRequired)

		_, err = service.GetUpdateHistory(ctx, &GetUpdateHistoryQuery{StatusID: "https://example.com/objects/status-1", Limit: 1})
		require.NoError(t, err)
	}

	// GetUpdateHistory: repository error should return empty history (nil error).
	{
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockUpdateBuilder := new(mocks.MockUpdateBuilder)
		mockQuery.On("All", mock.Anything).Return(fmt.Errorf("boom")).Once()
		setupPermissiveDynamormMocks(mockDB, mockQuery, mockUpdateBuilder, &permissiveQueryState{})

		service := newNotesServiceHarnessWithDB(t, transactionalMockDB{MockDB: mockDB})
		res, err := service.GetUpdateHistory(ctx, &GetUpdateHistoryQuery{StatusID: "status-1", Limit: 1})
		require.NoError(t, err)
		require.NotNil(t, res)
		assert.Empty(t, res.History)
	}

	// deleteBoostStatus: result nil branch (boost record not found).
	{
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockUpdateBuilder := new(mocks.MockUpdateBuilder)
		mockQuery.On("All", mock.Anything).Return(nil).Once()
		setupPermissiveDynamormMocks(mockDB, mockQuery, mockUpdateBuilder, &permissiveQueryState{})

		service := newNotesServiceHarnessWithDB(t, transactionalMockDB{MockDB: mockDB})
		service.deleteBoostStatus(ctx, "booster", "status-1")
	}
}

func TestService_round15_mute_errors(t *testing.T) {
	ctx := context.Background()

	// muteStatus/unmuteStatus: repo missing and hard errors.
	{
		service, _, _, _, _ := newNotesServiceHarness(t)
		service.conversationRepo = nil
		assert.ErrorIs(t, service.muteStatus(ctx, "bob", "status-1", 1), ErrConversationServiceNotAvailable)
		assert.ErrorIs(t, service.unmuteStatus(ctx, "bob", "status-1"), ErrConversationServiceNotAvailable)

		service.conversationRepo = &stubConversationRepo{createMuteErr: fmt.Errorf("boom"), deleteMuteErr: fmt.Errorf("boom")}
		assert.ErrorIs(t, service.muteStatus(ctx, "bob", "status-1", 1), ErrMuteConversation)
		assert.ErrorIs(t, service.unmuteStatus(ctx, "bob", "status-1"), ErrUnmuteConversation)

		_, err := service.MuteNote(ctx, &MuteNoteCommand{StatusID: "status-1", MuterID: "bob"})
		assert.ErrorIs(t, err, ErrMuteStatus)
		_, err = service.UnmuteNote(ctx, &UnmuteNoteCommand{StatusID: "status-1", MuterID: "bob"})
		assert.ErrorIs(t, err, ErrUnmuteStatus)
	}
}

func TestService_round15_emits_and_federation_error_paths(t *testing.T) {
	ctx := context.Background()

	service, _, _, _, _ := newNotesServiceHarness(t)
	service.publisher = failingPublisher{}
	service.federation = failingFederation{}

	// emitStatusCreatedEvents should tolerate publisher failures.
	events := service.emitStatusCreatedEvents(ctx, &models.Status{StatusID: "status-1", AuthorUsername: "alice", AuthorID: "https://example.com/users/alice"})
	assert.Empty(t, events)

	// emitStatusDeletedEvents: skip branch and publish failure.
	service.emitStatusDeletedEvents(ctx, &models.Status{StatusID: "status-1", AuthorUsername: ""})
	service.emitStatusDeletedEvents(ctx, &models.Status{StatusID: "status-1", AuthorUsername: "alice", AuthorID: "https://example.com/users/alice"})

	// queueFederationTombstone: federation nil / note nil / error on queue.
	service.federation = nil
	service.queueFederationTombstone(ctx, &models.Status{StatusID: "status-1", Note: &activitypub.Note{BaseObject: activitypub.BaseObject{ID: "x"}}})

	service.federation = failingFederation{}
	service.queueFederationTombstone(ctx, &models.Status{StatusID: "status-1"})
	service.queueFederationTombstone(ctx, &models.Status{StatusID: "status-1", Note: &activitypub.Note{BaseObject: activitypub.BaseObject{ID: "x"}}})
}

func TestService_round15_misc_helpers(t *testing.T) {
	service, _, _, _, _ := newNotesServiceHarness(t)

	ctx := context.Background()

	t.Run("ensureAuthorUsername variants", func(t *testing.T) {
		status := &models.Status{AuthorID: "https://example.com/users/alice"}
		service.ensureAuthorUsername(ctx, status)
		assert.Equal(t, "alice", status.AuthorUsername)

		status = &models.Status{AuthorID: "https://example.com/@bob"}
		service.ensureAuthorUsername(ctx, status)
		assert.Equal(t, "bob", status.AuthorUsername)

		status = &models.Status{AuthorID: "https://example.com/users/charlie/"}
		service.ensureAuthorUsername(ctx, status)
		assert.Equal(t, "charlie", status.AuthorUsername)

		// Format 3: last segment fallback.
		status = &models.Status{AuthorID: "https://example.com/actors/delta"}
		service.ensureAuthorUsername(ctx, status)
		assert.Equal(t, "delta", status.AuthorUsername)

		// Final fallback: account lookup when parsing yields empty.
		serviceWithFallback, _, _, _, _ := newNotesServiceHarness(t)
		serviceWithFallback.accountRepo = &fixedAccountRepo{username: "fallback"}
		status = &models.Status{AuthorID: "/"}
		serviceWithFallback.ensureAuthorUsername(ctx, status)
		assert.Equal(t, "fallback", status.AuthorUsername)
	})

	assert.Equal(t, "abc", deriveStatusIDFromKeys("status#abc", ""))
	assert.Equal(t, "def", deriveStatusIDFromKeys("", "status#def"))
	assert.Equal(t, "", deriveStatusIDFromKeys("nope", ""))
	assert.Equal(t, "", boostStatusIDFromActors("", "status-1"))
	assert.NotEmpty(t, boostStatusIDFromActors("booster", "status-1"))

	assert.False(t, isAlreadyExistsError(nil))
	assert.True(t, isAlreadyExistsError(pkgerrors.ItemAlreadyExists("x")))
	assert.True(t, isAlreadyExistsError(fmt.Errorf("Condition check failed")))
	assert.False(t, isAlreadyExistsError(fmt.Errorf("some other error")))

	// Cover lookupParentStatus and markMediaAsUsed.
	status, err := service.lookupParentStatus(ctx, "status-1")
	require.NoError(t, err)
	require.NotNil(t, status)

	mediaRepo := &MockMediaRepository{}
	mediaRepo.On("MarkMediaUsed", mock.Anything, "media-1").Return(fmt.Errorf("boom")).Once()
	mediaRepo.On("MarkMediaUsed", mock.Anything, "media-2").Return(nil).Once()
	service.mediaRepo = mediaRepo
	service.markMediaAsUsed(ctx, "status-1", []string{"media-1", "media-2"})
	mediaRepo.AssertExpectations(t)
}

func TestService_round15_poll_and_counter_error_branches(t *testing.T) {
	ctx := context.Background()

	// Poll creation branches (PollExpiresIn default vs explicit).
	{
		service, _, _, _, _ := newNotesServiceHarness(t)

		_, err := service.CreateNote(ctx, &CreateNoteCommand{
			AuthorID:      "alice",
			Content:       "poll default expiry",
			Visibility:    VisibilityPublic,
			PollOptions:   []string{"a", "b"},
			PollExpiresIn: 0,
			ToRecipients:  []string{"https://www.w3.org/ns/activitystreams#Public"},
			CcRecipients:  []string{"https://example.com/users/alice/followers"},
		})
		require.NoError(t, err)

		_, err = service.CreateNote(ctx, &CreateNoteCommand{
			AuthorID:      "alice",
			Content:       "poll explicit expiry",
			Visibility:    VisibilityPublic,
			PollOptions:   []string{"a", "b"},
			PollExpiresIn: 60,
			ToRecipients:  []string{"https://www.w3.org/ns/activitystreams#Public"},
			CcRecipients:  []string{"https://example.com/users/alice/followers"},
		})
		require.NoError(t, err)
	}

	// handlePollCreation should tolerate poll repository failures.
	{
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockUpdateBuilder := new(mocks.MockUpdateBuilder)

		mockQuery.On("Create").Return(fmt.Errorf("boom")).Maybe()
		setupPermissiveDynamormMocks(mockDB, mockQuery, mockUpdateBuilder, &permissiveQueryState{})

		service := newNotesServiceHarnessWithDB(t, transactionalMockDB{MockDB: mockDB})
		service.handlePollCreation(ctx, &CreateNoteCommand{
			AuthorID:    "alice",
			PollOptions: []string{"a", "b"},
		}, "status-1")
	}

	// Like/Unlike counter increments should surface status repository update failures.
	{
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockUpdateBuilder := new(mocks.MockUpdateBuilder)

		mockUpdateBuilder.On("Execute").Return(fmt.Errorf("boom")).Maybe()
		setupPermissiveDynamormMocks(mockDB, mockQuery, mockUpdateBuilder, &permissiveQueryState{})

		service := newNotesServiceHarnessWithDB(t, transactionalMockDB{MockDB: mockDB})
		_, err := service.LikeNote(ctx, &LikeNoteCommand{StatusID: "status-1", LikerID: "alice"})
		assert.ErrorIs(t, err, ErrExecuteAction)

		_, err = service.UnlikeNote(ctx, &UnlikeNoteCommand{StatusID: "status-1", UnlikerID: "alice"})
		assert.ErrorIs(t, err, ErrExecuteAction)
	}
}

func TestService_round15_wrapper_error_branches(t *testing.T) {
	ctx := context.Background()

	// GetNote success path.
	{
		service, _, _, _, _ := newNotesServiceHarness(t)
		note, err := service.GetNote(ctx, "status-1")
		require.NoError(t, err)
		require.NotNil(t, note)
	}

	// GetNote/GetNoteWithViewer: repository errors and required param validation.
	{
		service, _, _, _, _ := newNotesServiceHarness(t)
		_, err := service.GetNoteWithViewer(ctx, &GetNoteQuery{StatusID: "", ViewerID: "alice"})
		assert.ErrorIs(t, err, ErrStatusIDRequired)
	}

	{
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockUpdateBuilder := new(mocks.MockUpdateBuilder)

		mockQuery.On("First", mock.Anything).Return(fmt.Errorf("boom")).Maybe()
		setupPermissiveDynamormMocks(mockDB, mockQuery, mockUpdateBuilder, &permissiveQueryState{})

		service := newNotesServiceHarnessWithDB(t, transactionalMockDB{MockDB: mockDB})
		_, err := service.GetNote(ctx, "status-1")
		assert.ErrorIs(t, err, ErrGetStatus)

		_, err = service.GetNoteWithViewer(ctx, &GetNoteQuery{StatusID: "status-1", ViewerID: "alice"})
		assert.ErrorIs(t, err, ErrGetStatus)
	}

	// checkViewPermissions: relationship repo error should surface as ErrCheckFollowingRelationship.
	{
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockUpdateBuilder := new(mocks.MockUpdateBuilder)

		mockQuery.On("First", mock.Anything).Return(fmt.Errorf("boom")).Maybe()
		setupPermissiveDynamormMocks(mockDB, mockQuery, mockUpdateBuilder, &permissiveQueryState{})

		service := newNotesServiceHarnessWithDB(t, transactionalMockDB{MockDB: mockDB})
		_, err := service.checkViewPermissions(ctx, &models.Status{Visibility: models.VisibilityPrivate, AuthorUsername: "alice"}, "bob")
		assert.ErrorIs(t, err, ErrCheckFollowingRelationship)
	}

	// Timeline GetTimeline: SinceID/MinID branches.
	{
		service, _, _, _, _ := newNotesServiceHarness(t)
		_, err := service.GetTimeline(ctx, &GetTimelineQuery{
			UserID:   "alice",
			Timeline: "public",
			Limit:    2,
			SinceID:  "since-1",
			MinID:    "min-1",
		})
		require.NoError(t, err)
	}

	// Bookmarks: Create/Delete failures and GetBookmarks error paths.
	{
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockUpdateBuilder := new(mocks.MockUpdateBuilder)

		state := &permissiveQueryState{}
		mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
			populateStruct(args.Get(0), state)
		}).Return(nil).Once()
		mockQuery.On("First", mock.Anything).Return(dynamormerrors.ErrItemNotFound).Once()
		setupPermissiveDynamormMocks(mockDB, mockQuery, mockUpdateBuilder, state)

		service := newNotesServiceHarnessWithDB(t, failingTransactDB{MockDB: mockDB, err: fmt.Errorf("boom")})
		_, err := service.BookmarkNote(ctx, &BookmarkNoteCommand{StatusID: "status-1", BookmarkerID: "alice"})
		assert.ErrorIs(t, err, ErrBookmarkStatus)
	}

	{
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockUpdateBuilder := new(mocks.MockUpdateBuilder)

		setupPermissiveDynamormMocks(mockDB, mockQuery, mockUpdateBuilder, &permissiveQueryState{})

		service := newNotesServiceHarnessWithDB(t, failingTransactDB{MockDB: mockDB, err: fmt.Errorf("boom")})
		_, err := service.UnbookmarkNote(ctx, &UnbookmarkNoteCommand{StatusID: "status-1", UnbookmarkerID: "alice"})
		assert.ErrorIs(t, err, ErrUnbookmarkStatus)
	}

	{
		service, _, _, _, _ := newNotesServiceHarness(t)
		service.bookmarkRepo = nil
		_, err := service.GetBookmarks(ctx, &GetBookmarksQuery{UserID: "alice"})
		assert.ErrorIs(t, err, ErrGetBookmarks)
	}

	{
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockUpdateBuilder := new(mocks.MockUpdateBuilder)

		mockQuery.On("All", mock.Anything).Return(fmt.Errorf("boom")).Maybe()
		setupPermissiveDynamormMocks(mockDB, mockQuery, mockUpdateBuilder, &permissiveQueryState{})

		service := newNotesServiceHarnessWithDB(t, transactionalMockDB{MockDB: mockDB})
		_, err := service.GetBookmarks(ctx, &GetBookmarksQuery{UserID: "alice", Pagination: interfaces.PaginationOptions{Limit: 2}})
		assert.ErrorIs(t, err, ErrGetBookmarks)
	}

	{
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockUpdateBuilder := new(mocks.MockUpdateBuilder)

		mockQuery.On("BatchGet", mock.Anything, mock.Anything).Return(fmt.Errorf("boom")).Maybe()
		setupPermissiveDynamormMocks(mockDB, mockQuery, mockUpdateBuilder, &permissiveQueryState{})

		service := newNotesServiceHarnessWithDB(t, transactionalMockDB{MockDB: mockDB})
		_, err := service.GetBookmarks(ctx, &GetBookmarksQuery{UserID: "alice", Pagination: interfaces.PaginationOptions{Limit: 2}})
		assert.ErrorIs(t, err, ErrGetBookmarks)
	}

	// Simple wrappers: repository failures should map to service-level errors.
	{
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockUpdateBuilder := new(mocks.MockUpdateBuilder)

		mockQuery.On("Count").Return(int64(0), fmt.Errorf("boom")).Maybe()
		setupPermissiveDynamormMocks(mockDB, mockQuery, mockUpdateBuilder, &permissiveQueryState{})

		service := newNotesServiceHarnessWithDB(t, transactionalMockDB{MockDB: mockDB})
		_, err := service.CountNotesByAuthor(ctx, "alice")
		assert.ErrorIs(t, err, ErrCountStatusesByAuthor)

		_, err = service.CountReplies(ctx, "status-1")
		assert.ErrorIs(t, err, ErrCountReplies)

		_, err = service.GetBoostCount(ctx, "status-1")
		assert.ErrorIs(t, err, ErrGetBoostCount)

		_, err = service.GetLikeCount(ctx, "status-1")
		assert.ErrorIs(t, err, ErrGetLikeCount)
	}

	{
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockUpdateBuilder := new(mocks.MockUpdateBuilder)

		mockQuery.On("All", mock.Anything).Return(fmt.Errorf("boom")).Maybe()
		setupPermissiveDynamormMocks(mockDB, mockQuery, mockUpdateBuilder, &permissiveQueryState{})

		service := newNotesServiceHarnessWithDB(t, transactionalMockDB{MockDB: mockDB})
		_, err := service.GetUserTimeline(ctx, "alice", interfaces.PaginationOptions{Limit: 2})
		assert.ErrorIs(t, err, ErrGetUserTimeline)

		_, err = service.HasReblogged(ctx, "alice", "status-1")
		assert.ErrorIs(t, err, ErrCheckUserHasReblogged)
	}

	{
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockUpdateBuilder := new(mocks.MockUpdateBuilder)

		mockQuery.On("First", mock.Anything).Return(fmt.Errorf("boom")).Maybe()
		setupPermissiveDynamormMocks(mockDB, mockQuery, mockUpdateBuilder, &permissiveQueryState{})

		service := newNotesServiceHarnessWithDB(t, transactionalMockDB{MockDB: mockDB})
		_, err := service.HasLiked(ctx, "alice", "status-1")
		assert.ErrorIs(t, err, ErrCheckUserHasLiked)

		_, err = service.IsBookmarked(ctx, "alice", "status-1")
		assert.ErrorIs(t, err, ErrCheckUserHasBookmarked)
	}
}

func TestService_round15_more_branch_coverage(t *testing.T) {
	ctx := context.Background()

	// Like/Unlike: early action failures should be surfaced (and cover createLike/deleteLike error paths).
	{
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockUpdateBuilder := new(mocks.MockUpdateBuilder)

		mockQuery.On("Create").Return(fmt.Errorf("boom")).Maybe()
		setupPermissiveDynamormMocks(mockDB, mockQuery, mockUpdateBuilder, &permissiveQueryState{})

		service := newNotesServiceHarnessWithDB(t, transactionalMockDB{MockDB: mockDB})
		_, err := service.LikeNote(ctx, &LikeNoteCommand{StatusID: "status-1", LikerID: "alice"})
		assert.ErrorIs(t, err, ErrExecuteAction)

		_, err = service.CreateCommunityNote(ctx, &CreateCommunityNoteCommand{Note: &storage.CommunityNote{ID: "note-1"}})
		assert.ErrorIs(t, err, ErrCreateCommunityNote)

		_, err = service.CreateCommunityNoteVote(ctx, &CreateCommunityNoteVoteCommand{Vote: &storage.CommunityNoteVote{NoteID: "note-1"}})
		assert.ErrorIs(t, err, ErrCreateCommunityNoteVote)
	}

	{
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockUpdateBuilder := new(mocks.MockUpdateBuilder)

		mockQuery.On("Delete").Return(fmt.Errorf("boom")).Maybe()
		setupPermissiveDynamormMocks(mockDB, mockQuery, mockUpdateBuilder, &permissiveQueryState{})

		service := newNotesServiceHarnessWithDB(t, transactionalMockDB{MockDB: mockDB})
		_, err := service.UnlikeNote(ctx, &UnlikeNoteCommand{StatusID: "status-1", UnlikerID: "alice"})
		assert.ErrorIs(t, err, ErrExecuteAction)
	}

	// deleteBoostStatus: validate missing params, repo error, and success path.
	{
		service, _, _, _, _ := newNotesServiceHarness(t)
		service.noteRepo = nil
		service.deleteBoostStatus(ctx, "booster", "status-1")
	}

	{
		service, _, _, _, _ := newNotesServiceHarness(t)
		service.deleteBoostStatus(ctx, "", "status-1")
		service.deleteBoostStatus(ctx, "booster", "")
		service.deleteBoostStatus(ctx, "booster", "status-1")
	}

	{
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockUpdateBuilder := new(mocks.MockUpdateBuilder)

		mockQuery.On("All", mock.Anything).Return(fmt.Errorf("boom")).Maybe()
		setupPermissiveDynamormMocks(mockDB, mockQuery, mockUpdateBuilder, &permissiveQueryState{})

		service := newNotesServiceHarnessWithDB(t, transactionalMockDB{MockDB: mockDB})
		service.deleteBoostStatus(ctx, "booster", "status-1")
	}

	// validateCreateScheduledNoteCommand: additional error branches.
	{
		service, _, _, _, _ := newNotesServiceHarness(t)

		assert.ErrorIs(t, service.validateCreateScheduledNoteCommand(ctx, &CreateScheduledNoteCommand{
			AuthorID:    "alice",
			Content:     "   ",
			Visibility:  VisibilityPublic,
			ScheduledAt: time.Now().Add(5 * time.Minute),
		}), ErrContentCannotBeEmpty)

		assert.ErrorIs(t, service.validateCreateScheduledNoteCommand(ctx, &CreateScheduledNoteCommand{
			AuthorID:    "alice",
			Content:     strings.Repeat("x", 501),
			Visibility:  VisibilityPublic,
			ScheduledAt: time.Now().Add(5 * time.Minute),
		}), ErrContentTooLongShort)

		assert.Error(t, service.validateCreateScheduledNoteCommand(ctx, &CreateScheduledNoteCommand{
			AuthorID:    "alice",
			Content:     "ok",
			Visibility:  "invalid",
			ScheduledAt: time.Now().Add(5 * time.Minute),
		}))
	}

	// Community notes: error branches on reads.
	{
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockUpdateBuilder := new(mocks.MockUpdateBuilder)

		mockQuery.On("All", mock.Anything).Return(fmt.Errorf("boom")).Maybe()
		setupPermissiveDynamormMocks(mockDB, mockQuery, mockUpdateBuilder, &permissiveQueryState{})

		service := newNotesServiceHarnessWithDB(t, transactionalMockDB{MockDB: mockDB})
		_, err := service.GetVisibleCommunityNotes(ctx, &GetVisibleCommunityNotesQuery{ObjectID: "object-1"})
		assert.ErrorIs(t, err, ErrGetVisibleCommunityNotes)

		_, err = service.GetCommunityNotesByAuthor(ctx, &GetCommunityNotesByAuthorQuery{AuthorID: "alice", Limit: 2})
		assert.ErrorIs(t, err, ErrGetCommunityNotesByAuthor)
	}

	{
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockUpdateBuilder := new(mocks.MockUpdateBuilder)

		mockQuery.On("First", mock.Anything).Return(fmt.Errorf("boom")).Maybe()
		setupPermissiveDynamormMocks(mockDB, mockQuery, mockUpdateBuilder, &permissiveQueryState{})

		service := newNotesServiceHarnessWithDB(t, transactionalMockDB{MockDB: mockDB})
		_, err := service.GetCommunityNote(ctx, &GetCommunityNoteQuery{NoteID: "note-1"})
		assert.ErrorIs(t, err, ErrGetCommunityNote)
	}
}

func TestService_round15_emitStatusCreatedEvents_branch_coverage(t *testing.T) {
	service, _, _, _, _ := newNotesServiceHarness(t)
	ctx := context.Background()

	events := service.emitStatusCreatedEvents(ctx, &models.Status{
		StatusID:       "status-1",
		AuthorID:       "https://example.com/users/alice",
		AuthorUsername: "alice",
		Visibility:     models.VisibilityPublic,
		Hashtags:       []string{"go"},
	})
	assert.NotEmpty(t, events)
	assert.True(t, hasEventStream(events, "public"))
	assert.True(t, hasEventStream(events, "hashtag:go"))

	events = service.emitStatusCreatedEvents(ctx, &models.Status{
		StatusID:       "direct-1",
		AuthorID:       "https://example.com/users/alice",
		AuthorUsername: "alice",
		Visibility:     models.VisibilityDirect,
		ConversationID: "conv-1",
	})
	assert.NotEmpty(t, events)
	assert.True(t, hasEventStream(events, "conversation:conv-1"))
}

func hasEventStream(events []*streaming.Event, stream string) bool {
	for _, event := range events {
		if event != nil && event.Stream == stream {
			return true
		}
	}
	return false
}

func TestService_round15_emitter_publisher_failure_branches(t *testing.T) {
	service, _, _, _, _ := newNotesServiceHarness(t)
	service.publisher = failingPublisher{}
	ctx := context.Background()

	status := &models.Status{
		StatusID:       "status-1",
		AuthorID:       "https://example.com/users/alice",
		AuthorUsername: "alice",
		Visibility:     models.VisibilityPublic,
		ReblogCount:    1,
	}

	assert.Empty(t, service.emitLikeEvents(ctx, status, "bob"))
	assert.Empty(t, service.emitUnlikeEvents(ctx, status, "bob"))
	assert.Empty(t, service.emitBookmarkEvents(ctx, status, "bob"))
	assert.Empty(t, service.emitUnbookmarkEvents(ctx, status, "bob"))
	assert.Empty(t, service.emitReblogEvents(ctx, status, "bob"))
	assert.Empty(t, service.emitUnreblogEvents(ctx, status, "bob"))
	assert.Empty(t, service.emitStatusUpdatedEvents(ctx, status))
}

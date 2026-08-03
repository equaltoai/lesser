package repositories

import (
	"fmt"
	"reflect"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/theory-cloud/tabletheory/v3/pkg/mocks"
)

func setupPermissiveRound07Mocks(mockDB *mocks.MockDB, mockQuery *mocks.MockQuery, mockUpdateBuilder *mocks.MockUpdateBuilder, baseTime time.Time) {
	mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
	mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()

	mockQuery.On("WithContext", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Index", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Filter", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Limit", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Cursor", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("ConsistentRead").Return(mockQuery).Maybe()
	mockQuery.On("Select", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("IfNotExists").Return(mockQuery).Maybe()

	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		populateRound07SliceForCoverage(args.Get(0), baseTime)
	}).Return(nil).Maybe()

	mockQuery.On("Scan", mock.Anything).Run(func(args mock.Arguments) {
		populateRound07SliceForCoverage(args.Get(0), baseTime)
	}).Return(nil).Maybe()

	mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
		populateRound07StructForCoverage(args.Get(0), 0, baseTime)
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
	mockUpdateBuilder.On("Set", mock.Anything, mock.Anything).Return(mockUpdateBuilder).Maybe()
	mockUpdateBuilder.On("Remove", mock.Anything).Return(mockUpdateBuilder).Maybe()
	mockUpdateBuilder.On("SetIfNotExists", mock.Anything, mock.Anything, mock.Anything).Return(mockUpdateBuilder).Maybe()
	mockUpdateBuilder.On("ConditionVersion", mock.Anything).Return(mockUpdateBuilder).Maybe()
	mockUpdateBuilder.On("Execute").Return(nil).Maybe()
}

func populateRound07SliceForCoverage(target any, baseTime time.Time) {
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
			populateRound07StructForCoverage(element.Interface(), i, baseTime)
		} else {
			elemPtr := reflect.New(baseElemType)
			populateRound07StructForCoverage(elemPtr.Interface(), i, baseTime)
			element = elemPtr.Elem()
		}
		slice = reflect.Append(slice, element)
	}

	value.Elem().Set(slice)
}

func populateRound07StructForCoverage(target any, idx int, baseTime time.Time) {
	now := baseTime.Add(time.Duration(idx) * time.Minute).UTC()

	switch model := target.(type) {
	case *models.Notification:
		model.ID = fmt.Sprintf("notif-%d", idx+1)
		model.UserID = "user-1"
		model.Type = "mention"
		model.ActorID = fmt.Sprintf("https://example.com/users/actor-%d", idx+1)
		model.GroupKey = fmt.Sprintf("group-%d", idx+1)
		model.CreatedAt = now
		model.UpdatedAt = now
		model.PK = "USER#user-1"
		model.SK = fmt.Sprintf("notif#%s#%s", now.Format("20060102150405"), model.ID)
		model.GSI1PK = "NOTIF_TYPE#" + model.Type
		model.GSI1SK = fmt.Sprintf("%s#%s#%s", now.Format(time.RFC3339), model.UserID, model.ID)

	case *models.NotificationPreferences:
		model.Username = "user-1"
		model.EmailEnabled = true
		model.PushEnabled = true
		model.FollowEnabled = true
		model.MentionEnabled = true
		model.ReblogEnabled = true
		model.FavoriteEnabled = true
		model.FollowNotifications = true
		model.MentionNotifications = true
		model.ReblogNotifications = true
		model.FavoriteNotifications = true
		model.UpdateKeys()

	case *models.NotificationCostTracking:
		model.PK = fmt.Sprintf("COST#%d", idx+1)
		model.SK = fmt.Sprintf("TRACK#%s", now.Format(time.RFC3339))

	case *models.PushSubscription:
		model.ID = fmt.Sprintf("sub-%d", idx+1)
		model.Username = "user-1"
		model.Endpoint = fmt.Sprintf("https://example.com/push/%d", idx+1)
		model.P256dh = "p256dh"
		model.Auth = "auth"
		model.CreatedAt = now
		model.UpdatedAt = now
		_ = model.UpdateKeys()

	case *models.VAPIDKeyRecord:
		model.PK = "INSTANCE#CONFIG"
		model.SK = "VAPID_KEYS"
		model.Data = map[string]any{
			"public_key":  "public",
			"private_key": "private",
			"subject":     "mailto:test@example.com",
			"created_at":  now.UTC().Format(time.RFC3339Nano),
			"updated_at":  now.UTC().Format(time.RFC3339Nano),
		}
		model.UpdatedAt = now

	case *models.Conversation:
		model.ID = fmt.Sprintf("conv-%d", idx+1)
		model.Participants = []string{"user-1", "user-2"}
		model.LastStatusID = fmt.Sprintf("status-%d", idx+1)
		model.TotalMessageCount = 2
		model.CreatedAt = now
		model.UpdatedAt = now
		_ = model.UpdateKeys()

	case *models.ConversationParticipantRecord:
		conversation := &models.Conversation{
			ID:           fmt.Sprintf("conv-%d", idx+1),
			Participants: []string{"user-1", "user-2"},
			LastStatusID: fmt.Sprintf("status-%d", idx+1),
			CreatedAt:    now,
			UpdatedAt:    now,
		}
		_ = conversation.UpdateKeys()
		model.Conversation = conversation
		_ = model.BeforeCreate(fmt.Sprintf("user-%d", idx+1))

	case *models.ConversationParticipantKey:
		model.ConversationID = fmt.Sprintf("conv-%d", idx+1)

	case *models.UserConversationState:
		model.ViewerID = fmt.Sprintf("user-%d", idx+1)
		model.ConversationID = fmt.Sprintf("conv-%d", idx+1)
		model.CounterpartID = "user-2"
		model.Folder = models.UserConversationFolderInbox
		model.Unread = true
		model.SortAt = now
		model.CreatedAt = now
		model.UpdatedAt = now
		_ = model.BeforeCreate()

	case *models.ConversationMessage:
		model.ConversationID = "conv-1"
		model.StatusID = fmt.Sprintf("status-%d", idx+1)
		model.SenderUsername = fmt.Sprintf("user-%d", idx+1)
		model.CreatedAt = now
		_ = model.BeforeCreate()

	case *models.ConversationStatus:
		model.ConversationID = "conv-1"
		model.UserID = "user-1"
		model.Unread = true
		model.LastReadAt = now.Add(-time.Minute)
		_ = model.BeforeCreate()

	case *models.ConversationMute:
		model.Username = "user-1"
		model.ConversationID = "conv-1"
		model.CreatedAt = now
		model.ExpiresAt = now.Add(24 * time.Hour)
		_ = model.UpdateKeys()
	}
}

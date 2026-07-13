package repositories

import (
	"fmt"
	"reflect"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/theory-cloud/tabletheory/v2/pkg/mocks"
)

func setupPermissiveAccountRepositoryMocks(mockDB *mocks.MockDB, mockQuery *mocks.MockQuery, mockUpdateBuilder *mocks.MockUpdateBuilder, baseTime time.Time) {
	mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
	mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()

	mockQuery.On("Index", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Filter", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Limit", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Cursor", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("ConsistentRead").Return(mockQuery).Maybe()
	mockQuery.On("Select", mock.Anything).Return(mockQuery).Maybe()

	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		populateAccountRepositorySliceForCoverage(args.Get(0), baseTime)
	}).Return(nil).Maybe()

	mockQuery.On("Scan", mock.Anything).Run(func(args mock.Arguments) {
		populateAccountRepositorySliceForCoverage(args.Get(0), baseTime)
	}).Return(nil).Maybe()

	mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
		populateAccountRepositoryStructForCoverage(args.Get(0), 0, baseTime)
	}).Return(nil).Maybe()

	mockQuery.On("Create").Return(nil).Maybe()
	mockQuery.On("Update").Return(nil).Maybe()
	mockQuery.On("Update", mock.Anything).Return(nil).Maybe()
	mockQuery.On("Delete").Return(nil).Maybe()
	mockQuery.On("Delete", mock.Anything).Return(nil).Maybe()
	mockQuery.On("Count").Return(int64(2), nil).Maybe()

	if mockUpdateBuilder != nil {
		mockQuery.On("UpdateBuilder").Return(mockUpdateBuilder).Maybe()
		mockUpdateBuilder.On("Set", mock.Anything, mock.Anything).Return(mockUpdateBuilder).Maybe()
		mockUpdateBuilder.On("SetIfNotExists", mock.Anything, mock.Anything, mock.Anything).Return(mockUpdateBuilder).Maybe()
		mockUpdateBuilder.On("Remove", mock.Anything).Return(mockUpdateBuilder).Maybe()
		mockUpdateBuilder.On("ConditionNotExists", mock.Anything).Return(mockUpdateBuilder).Maybe()
		mockUpdateBuilder.On("ConditionVersion", mock.Anything).Return(mockUpdateBuilder).Maybe()
		mockUpdateBuilder.On("Execute").Return(nil).Maybe()
	}
}

func populateAccountRepositorySliceForCoverage(target any, baseTime time.Time) {
	value := reflect.ValueOf(target)
	if value.Kind() != reflect.Ptr || value.Elem().Kind() != reflect.Slice {
		return
	}

	slice := value.Elem()
	elemType := slice.Type().Elem()

	// Avoid interface slices to prevent type assertion pitfalls.
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
			populateAccountRepositoryStructForCoverage(element.Interface(), i, baseTime)
		} else {
			elemPtr := reflect.New(baseElemType)
			populateAccountRepositoryStructForCoverage(elemPtr.Interface(), i, baseTime)
			element = elemPtr.Elem()
		}
		slice = reflect.Append(slice, element)
	}

	value.Elem().Set(slice)
}

func populateAccountRepositoryStructForCoverage(target any, idx int, baseTime time.Time) {
	now := baseTime.Add(time.Duration(idx) * time.Minute)

	switch model := target.(type) {
	case *models.User:
		model.Username = fmt.Sprintf("user-%d", idx+1)
		model.Role = "user"
		model.CreatedAt = now
		model.UpdatedAt = now
		_ = model.UpdateKeys()

	case *models.Actor:
		username := fmt.Sprintf("actor-%d", idx+1)
		model.Username = username
		model.CreatedAt = now
		model.UpdatedAt = now
		model.Actor = &activitypub.Actor{
			PreferredUsername: username,
			Name:              fmt.Sprintf("Actor %d", idx+1),
			BaseObject: activitypub.BaseObject{
				ID: fmt.Sprintf("https://example.com/users/%s", username),
			},
		}
		_ = model.UpdateKeys()

	case *models.Follow:
		model.FollowedUsername = fmt.Sprintf("followed-%d", idx+1)
		model.State = models.FollowStateAccepted

	case *models.TimelineEntry:
		model.TimelineType = "HOME"
		model.TimelineID = "user-1"
		model.EntryID = fmt.Sprintf("entry-%d", idx+1)
		model.PostID = fmt.Sprintf("post-%d", idx+1)
		model.ActorID = fmt.Sprintf("https://example.com/users/user-%d", idx+1)
		model.ActorHandle = fmt.Sprintf("@user-%d@example.com", idx+1)
		model.Content = "hello"
		model.ContentType = "text/plain"
		model.CreatedAt = now
		model.TimelineAt = now

	case *models.Conversation:
		model.ID = fmt.Sprintf("conv-%d", idx+1)
		model.Participants = []string{"user-1", "user-2"}
		model.LastStatusID = fmt.Sprintf("status-%d", idx+1)
		model.CreatedAt = now
		model.UpdatedAt = now

	case *models.TimelineMarker:
		model.Username = "user-1"
		model.Timeline = fmt.Sprintf("home-%d", idx+1)
		model.LastReadID = fmt.Sprintf("last-%d", idx+1)
		model.UpdatedAt = now

	case *models.List:
		model.Username = "user-1"
		model.ID = fmt.Sprintf("list-%d", idx+1)
		model.Title = "Test List"
		model.CreatedAt = now
		model.UpdatedAt = now
		_ = model.UpdateKeys()

	case *models.ConversationMute:
		model.Username = "user-1"
		model.ConversationID = fmt.Sprintf("conv-%d", idx+1)
		model.CreatedAt = now

	case *models.UserPreference:
		model.Username = "user-1"
		model.Key = fmt.Sprintf("pref-%d", idx+1)
		model.Value = "true"
		model.UpdatedAt = now
		model.UpdateKeys()

	case *models.AccountNote:
		model.Username = "user-1"
		model.TargetActorID = fmt.Sprintf("https://example.com/users/user-%d", idx+1)
		model.Note = "note"
		model.CreatedAt = now
		model.UpdatedAt = now
		_ = model.UpdateKeys()

	case *models.AccountPin:
		model.Username = "user-1"
		model.PinnedActorID = fmt.Sprintf("https://example.com/users/user-%d", idx+1)
		model.PinnedUsername = fmt.Sprintf("user-%d", idx+1)
		model.CreatedAt = now
		_ = model.UpdateKeys()

	case *models.FollowRequestState:
		model.State = "pending"

	case *models.UserDomainBlock:
		// No required fields for coverage.

	case *models.FieldVerification:
		model.Username = "user-1"
		model.FieldName = "website"
		model.FieldValue = "https://example.com"
		model.VerifiedAt = now
		model.ExpiresAt = now.Add(24 * time.Hour)
		model.UpdateKeys()

	case *models.PasswordReset:
		model.Username = "user-1"
		model.Token = "token-1"
		model.CreatedAt = now
		model.ExpiresAt = now.Add(24 * time.Hour)
		model.Used = false
		_ = model.UpdateKeys()

	case *models.UserLogin:
		model.Username = "user-1"
		model.Timestamp = now
		model.Success = true
		model.IPAddress = "127.0.0.1"
		model.UserAgent = "test"

	case *userVersionProjection:
		model.Table = "test-table"
		model.PK = "USER#user-1"
		model.SK = models.SKMetadata
		version := 1
		model.Value = &version

	case *userCoreProjection:
		model.Table = "test-table"
		model.PK = "USER#user-1"
		model.SK = models.SKMetadata
		model.Username = fmt.Sprintf("user-%d", idx+1)
		model.PasswordHash = "$2a$10$9Ay8tAONpS50qIdW.6P7Q.0i1o5nYVnDWAlC4AgdnbIXpNUv25B1q" // "password"
		model.Role = "user"
		model.CreatedAt = now
		model.UpdatedAt = now
		model.Approved = true
		model.Version = 1

	case *userMetadataProjection:
		model.Table = "test-table"
		model.PK = "USER#user-1"
		model.SK = models.SKMetadata
		model.Metadata = map[string]interface{}{"theme": "dark"}
	}
}

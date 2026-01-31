package lift

import (
	"reflect"
	"testing"
	"unsafe"

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
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/stretchr/testify/require"
)

func TestServiceRegistryAdapter(t *testing.T) {
	t.Run("nil registry returns nil", func(t *testing.T) {
		require.Nil(t, newServiceRegistry(nil))
	})

	t.Run("nil receiver returns nil", func(t *testing.T) {
		var adapter *servicesRegistryAdapter
		require.Nil(t, adapter.Accounts())
		require.Nil(t, adapter.AI())
		require.Nil(t, adapter.Conversations())
		require.Nil(t, adapter.Emoji())
		require.Nil(t, adapter.Lists())
		require.Nil(t, adapter.Media())
		require.Nil(t, adapter.Notes())
		require.Nil(t, adapter.Notifications())
		require.Nil(t, adapter.Relationships())
		require.Nil(t, adapter.Scheduled())
		require.Nil(t, adapter.Search())
	})

	t.Run("nil backing registry returns nil", func(t *testing.T) {
		adapter := &servicesRegistryAdapter{}
		require.Nil(t, adapter.Accounts())
		require.Nil(t, adapter.AI())
		require.Nil(t, adapter.Conversations())
		require.Nil(t, adapter.Emoji())
		require.Nil(t, adapter.Lists())
		require.Nil(t, adapter.Media())
		require.Nil(t, adapter.Notes())
		require.Nil(t, adapter.Notifications())
		require.Nil(t, adapter.Relationships())
		require.Nil(t, adapter.Scheduled())
		require.Nil(t, adapter.Search())
	})

	t.Run("service present returns service", func(t *testing.T) {
		reg := &services.Registry{}

		setField := func(fieldName string, value any) {
			t.Helper()
			rv := reflect.ValueOf(reg).Elem().FieldByName(fieldName)
			require.True(t, rv.IsValid(), "missing field %s", fieldName)
			reflect.NewAt(rv.Type(), unsafe.Pointer(rv.UnsafeAddr())).Elem().Set(reflect.ValueOf(value))
		}

		accountsSvc := &accounts.Service{}
		aiSvc := &ai.Service{}
		conversationsSvc := &conversations.Service{}
		emojiSvc := &emoji.Service{}
		listsSvc := &lists.Service{}
		mediaSvc := &media.Service{}
		notesSvc := &notes.Service{}
		notificationsSvc := &notifications.Service{}
		relationshipsSvc := &relationships.Service{}
		scheduledSvc := &scheduled.Service{}
		searchSvc := &search.Service{}

		setField("accountsService", accountsSvc)
		setField("aiService", aiSvc)
		setField("conversationsService", conversationsSvc)
		setField("emojiService", emojiSvc)
		setField("listsService", listsSvc)
		setField("mediaService", mediaSvc)
		setField("notesService", notesSvc)
		setField("notificationsService", notificationsSvc)
		setField("relationshipsService", relationshipsSvc)
		setField("scheduledService", scheduledSvc)
		setField("searchService", searchSvc)

		adapter := newServiceRegistry(reg)
		require.NotNil(t, adapter)
		require.Same(t, accountsSvc, adapter.Accounts())
		require.Same(t, aiSvc, adapter.AI())
		require.Same(t, conversationsSvc, adapter.Conversations())
		require.Same(t, emojiSvc, adapter.Emoji())
		require.Same(t, listsSvc, adapter.Lists())
		require.Same(t, mediaSvc, adapter.Media())
		require.Same(t, notesSvc, adapter.Notes())
		require.Same(t, notificationsSvc, adapter.Notifications())
		require.Same(t, relationshipsSvc, adapter.Relationships())
		require.Same(t, scheduledSvc, adapter.Scheduled())
		require.Same(t, searchSvc, adapter.Search())
	})

	t.Run("service missing returns nil", func(t *testing.T) {
		reg := &services.Registry{}
		adapter := newServiceRegistry(reg)
		require.NotNil(t, adapter)

		require.Nil(t, adapter.AI())
		require.Nil(t, adapter.Emoji())
		require.Nil(t, adapter.Lists())
		require.Nil(t, adapter.Media())
		require.Nil(t, adapter.Notes())
		require.Nil(t, adapter.Notifications())
		require.Nil(t, adapter.Relationships())
		require.Nil(t, adapter.Scheduled())
		require.Nil(t, adapter.Search())

		mockStorage := &MockRepositoryStorage{}
		mockStorage.On("Conversation").Return((*repositories.ConversationRepository)(nil)).Maybe()
		mockStorage.On("Status").Return((*repositories.StatusRepository)(nil)).Maybe()
		mockStorage.On("Account").Return((*repositories.AccountRepository)(nil)).Maybe()
		mockStorage.On("Audit").Return((*repositories.AuditRepository)(nil)).Maybe()

		rv := reflect.ValueOf(reg).Elem().FieldByName("storage")
		require.True(t, rv.IsValid())
		reflect.NewAt(rv.Type(), unsafe.Pointer(rv.UnsafeAddr())).Elem().Set(reflect.ValueOf(mockStorage))
		require.Nil(t, adapter.Conversations())
	})
}

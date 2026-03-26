package repositories

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/pkg/core"
	ddbErrors "github.com/theory-cloud/tabletheory/pkg/errors"
	"github.com/theory-cloud/tabletheory/pkg/mocks"
	"go.uber.org/zap"
)

type recordingConversationTransactionBuilder struct {
	created []any
	ops     []string
}

func (b *recordingConversationTransactionBuilder) Put(model any, _ ...core.TransactCondition) core.TransactionBuilder {
	b.created = append(b.created, model)
	b.ops = append(b.ops, "put")
	return b
}

func (b *recordingConversationTransactionBuilder) Create(model any, _ ...core.TransactCondition) core.TransactionBuilder {
	b.created = append(b.created, model)
	b.ops = append(b.ops, "create")
	return b
}

func (b *recordingConversationTransactionBuilder) Update(any, []string, ...core.TransactCondition) core.TransactionBuilder {
	return b
}

func (b *recordingConversationTransactionBuilder) UpdateWithBuilder(any, func(core.UpdateBuilder) error, ...core.TransactCondition) core.TransactionBuilder {
	return b
}

func (b *recordingConversationTransactionBuilder) Delete(any, ...core.TransactCondition) core.TransactionBuilder {
	return b
}

func (b *recordingConversationTransactionBuilder) ConditionCheck(any, ...core.TransactCondition) core.TransactionBuilder {
	return b
}

func (b *recordingConversationTransactionBuilder) WithContext(context.Context) core.TransactionBuilder {
	return b
}

func (b *recordingConversationTransactionBuilder) Execute() error {
	return nil
}

func (b *recordingConversationTransactionBuilder) ExecuteWithContext(context.Context) error {
	return nil
}

func TestRound44_ConversationRepository_CreateConversation_UsesTransactionalWriteSet(t *testing.T) {
	ctx := context.Background()
	repo := NewConversationRepository(new(mocks.MockDB), "test-table", zap.NewNop(), nil)
	builder := &recordingConversationTransactionBuilder{}

	repo.transactWriteFn = func(txCtx context.Context, fn func(core.TransactionBuilder) error) error {
		require.Equal(t, ctx, txCtx)
		return fn(builder)
	}

	conversation := &models.Conversation{ID: "conv-1"}
	err := repo.CreateConversation(ctx, conversation, []string{"Medic", "Arch"})
	require.NoError(t, err)

	require.Len(t, builder.created, 4)

	createdConversation, ok := builder.created[0].(*models.Conversation)
	require.True(t, ok)
	require.Equal(t, "conv-1", createdConversation.ID)
	require.Equal(t, []string{"arch", "medic"}, createdConversation.Participants)

	firstState, ok := builder.created[1].(*models.UserConversationState)
	require.True(t, ok)
	require.Equal(t, "arch", firstState.ViewerID)
	require.Equal(t, "USER_CONVERSATION_STATE#arch", firstState.PK)
	require.Equal(t, "CONVERSATION#conv-1", firstState.SK)

	secondState, ok := builder.created[2].(*models.UserConversationState)
	require.True(t, ok)
	require.Equal(t, "medic", secondState.ViewerID)
	require.Equal(t, "USER_CONVERSATION_STATE#medic", secondState.PK)
	require.Equal(t, "CONVERSATION#conv-1", secondState.SK)

	lookup, ok := builder.created[3].(*models.ConversationParticipantKey)
	require.True(t, ok)
	require.Equal(t, "CONVERSATION_PARTICIPANTS#arch,medic", lookup.PK)
	require.Equal(t, conversationParticipantLookupSK, lookup.SK)
	require.Equal(t, "conv-1", lookup.ConversationID)
}

func TestRound44_ConversationRepository_CreateConversationWithParticipantStates_UsesStableStateKeys(t *testing.T) {
	ctx := context.Background()
	repo := NewConversationRepository(new(mocks.MockDB), "test-table", zap.NewNop(), nil)
	builder := &recordingConversationTransactionBuilder{}

	repo.transactWriteFn = func(txCtx context.Context, fn func(core.TransactionBuilder) error) error {
		require.Equal(t, ctx, txCtx)
		return fn(builder)
	}

	conversation := &models.Conversation{ID: "conv-stable"}
	explicitStates := []*models.UserConversationState{
		{
			ViewerID:       "Medic",
			ConversationID: "conv-stable",
			CounterpartID:  "Arch",
			Folder:         models.UserConversationFolderInbox,
			RequestState:   models.DmRequestStateAccepted,
		},
		{
			ViewerID:       "Arch",
			ConversationID: "conv-stable",
			CounterpartID:  "Medic",
			Folder:         models.UserConversationFolderHidden,
		},
	}

	err := repo.CreateConversationWithParticipantStates(ctx, conversation, []string{"Medic", "Arch"}, explicitStates)
	require.NoError(t, err)

	require.Len(t, builder.created, 4)
	for _, item := range builder.created {
		_, isLegacyRecord := item.(*models.ConversationParticipantRecord)
		require.False(t, isLegacyRecord)
	}

	firstState, ok := builder.created[1].(*models.UserConversationState)
	require.True(t, ok)
	require.Equal(t, "arch", firstState.ViewerID)
	require.Equal(t, "USER_CONVERSATION_STATE#arch", firstState.PK)
	require.Equal(t, "CONVERSATION#conv-stable", firstState.SK)

	secondState, ok := builder.created[2].(*models.UserConversationState)
	require.True(t, ok)
	require.Equal(t, "medic", secondState.ViewerID)
	require.Equal(t, "USER_CONVERSATION_STATE#medic", secondState.PK)
	require.Equal(t, "CONVERSATION#conv-stable", secondState.SK)
	require.Equal(t, models.DmRequestStateAccepted, secondState.RequestState)
}

func TestRound44_ConversationRepository_TransactionalDirectMessageSendEnabled_ReflectsTransactionBuilderAvailability(t *testing.T) {
	repo := NewConversationRepository(new(mocks.MockDB), "test-table", zap.NewNop(), nil)
	require.False(t, repo.TransactionalDirectMessageSendEnabled())

	repo.transactWriteFn = func(context.Context, func(core.TransactionBuilder) error) error {
		return nil
	}
	require.True(t, repo.TransactionalDirectMessageSendEnabled())
}

func TestRound44_ConversationRepository_ApplyDirectMessageSend_RejectsInvalidTransitions(t *testing.T) {
	ctx := context.Background()
	repo := NewConversationRepository(new(mocks.MockDB), "test-table", zap.NewNop(), nil)

	err := repo.ApplyDirectMessageSend(ctx, nil)
	require.Error(t, err)

	err = repo.ApplyDirectMessageSend(ctx, &models.DirectMessageSendTransition{
		Conversation: &models.Conversation{ID: "conv-invalid"},
		Status: &models.Status{
			StatusID:       "status-invalid",
			AuthorID:       "alice",
			AuthorUsername: "alice",
			Content:        "hello",
			Visibility:     models.VisibilityDirect,
		},
	})
	require.Error(t, err)
}

func TestRound44_prepareDirectMessageSendTransition_UsesCreatedAtWhenPublishedAtMissing(t *testing.T) {
	createdAt := time.Date(2026, 3, 26, 16, 45, 0, 0, time.UTC)

	conversation, status, participantStates, err := prepareDirectMessageSendTransition(&models.DirectMessageSendTransition{
		Conversation: &models.Conversation{
			ID:           "conv-created-at",
			Participants: []string{"bob", "alice"},
		},
		Status: &models.Status{
			StatusID:       "status-created-at",
			AuthorID:       "alice",
			AuthorUsername: "alice",
			Content:        "hello",
			Visibility:     models.VisibilityDirect,
			CreatedAt:      createdAt,
		},
		ParticipantStates: []*models.UserConversationState{
			{
				ViewerID:       "alice",
				ConversationID: "conv-created-at",
				CounterpartID:  "bob",
				Folder:         models.UserConversationFolderInbox,
			},
			{
				ViewerID:       "bob",
				ConversationID: "conv-created-at",
				CounterpartID:  "alice",
				Folder:         models.UserConversationFolderRequests,
			},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, status)
	require.NotNil(t, conversation)
	require.Len(t, participantStates, 2)
	require.False(t, status.PublishedAt.IsZero())
	require.Equal(t, status.PublishedAt.UTC(), conversation.LastMessageTime)
	require.Equal(t, status.PublishedAt.UTC(), conversation.UpdatedAt)
	require.Equal(t, "status-created-at", conversation.LastStatusID)
}

func TestRound44_ConversationRepository_applyDirectMessageSendWithoutTransaction_UpdatesConversationWithoutStates(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewConversationRepository(mockDB, "test-table", zap.NewNop(), nil)

	conversation := &models.Conversation{
		ID:           "conv-fallback",
		Participants: []string{"alice", "bob"},
		CreatedAt:    time.Date(2026, 3, 26, 18, 0, 0, 0, time.UTC),
		UpdatedAt:    time.Date(2026, 3, 26, 18, 0, 0, 0, time.UTC),
	}

	mockDB.On("WithContext", ctx).Return(mockDB).Once()
	mockDB.On("Model", conversation).Return(mockQuery).Once()
	mockQuery.On("Update", mock.Anything).Return(nil).Once()

	err := repo.applyDirectMessageSendWithoutTransaction(ctx, conversation, nil, nil, false)
	require.NoError(t, err)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestRound44_ConversationRepository_ApplyDirectMessageSend_CreatesNewConversationWriteSet(t *testing.T) {
	ctx := context.Background()
	repo := NewConversationRepository(new(mocks.MockDB), "test-table", zap.NewNop(), nil)
	builder := &recordingConversationTransactionBuilder{}

	repo.transactWriteFn = func(txCtx context.Context, fn func(core.TransactionBuilder) error) error {
		require.Equal(t, ctx, txCtx)
		return fn(builder)
	}

	publishedAt := time.Date(2026, 3, 26, 14, 30, 0, 0, time.UTC)
	err := repo.ApplyDirectMessageSend(ctx, &models.DirectMessageSendTransition{
		Conversation: &models.Conversation{
			ID:           "conv-send",
			Participants: []string{"Medic", "Arch"},
			CreatedAt:    publishedAt,
			UpdatedAt:    publishedAt,
		},
		Status: &models.Status{
			StatusID:       "status-send",
			AuthorID:       "Medic",
			AuthorUsername: "medic",
			Content:        "hello",
			Visibility:     models.VisibilityDirect,
			ConversationID: "conv-send",
			PublishedAt:    publishedAt,
		},
		ParticipantStates: []*models.UserConversationState{
			{
				ViewerID:       "Medic",
				ConversationID: "conv-send",
				CounterpartID:  "Arch",
				Folder:         models.UserConversationFolderInbox,
				RequestState:   models.DmRequestStateAccepted,
				Unread:         false,
				LastReadAt:     &publishedAt,
				AcceptedAt:     &publishedAt,
			},
			{
				ViewerID:       "Arch",
				ConversationID: "conv-send",
				CounterpartID:  "Medic",
				Folder:         models.UserConversationFolderRequests,
				RequestState:   models.DmRequestStatePending,
				Unread:         true,
				RequestedAt:    &publishedAt,
			},
		},
		CreateConversation: true,
	})
	require.NoError(t, err)

	require.Len(t, builder.created, 5)
	require.Equal(t, []string{"create", "create", "create", "create", "create"}, builder.ops)

	conversation, ok := builder.created[0].(*models.Conversation)
	require.True(t, ok)
	require.Equal(t, "status-send", conversation.LastStatusID)
	require.EqualValues(t, 1, conversation.TotalMessageCount)
	require.Equal(t, publishedAt, conversation.LastMessageTime)

	firstState, ok := builder.created[1].(*models.UserConversationState)
	require.True(t, ok)
	require.Equal(t, "arch", firstState.ViewerID)
	require.Equal(t, "status-send", firstState.PreviewStatusID)
	require.True(t, firstState.Unread)

	secondState, ok := builder.created[2].(*models.UserConversationState)
	require.True(t, ok)
	require.Equal(t, "medic", secondState.ViewerID)
	require.Equal(t, "status-send", secondState.PreviewStatusID)
	require.False(t, secondState.Unread)

	lookup, ok := builder.created[3].(*models.ConversationParticipantKey)
	require.True(t, ok)
	require.Equal(t, "CONVERSATION_PARTICIPANTS#arch,medic", lookup.PK)

	status, ok := builder.created[4].(*models.Status)
	require.True(t, ok)
	require.Equal(t, "status-send", status.StatusID)
	require.Equal(t, "conv-send", status.ConversationID)
}

func TestRound44_ConversationRepository_ApplyDirectMessageSend_UpdatesExistingConversationWriteSet(t *testing.T) {
	ctx := context.Background()
	repo := NewConversationRepository(new(mocks.MockDB), "test-table", zap.NewNop(), nil)
	builder := &recordingConversationTransactionBuilder{}

	repo.transactWriteFn = func(txCtx context.Context, fn func(core.TransactionBuilder) error) error {
		require.Equal(t, ctx, txCtx)
		return fn(builder)
	}

	publishedAt := time.Date(2026, 3, 26, 15, 15, 0, 0, time.UTC)
	err := repo.ApplyDirectMessageSend(ctx, &models.DirectMessageSendTransition{
		Conversation: &models.Conversation{
			ID:                "conv-existing",
			Participants:      []string{"arch", "medic"},
			CreatedAt:         publishedAt.Add(-time.Hour),
			UpdatedAt:         publishedAt.Add(-time.Hour),
			TotalMessageCount: 5,
		},
		Status: &models.Status{
			StatusID:       "status-existing",
			AuthorID:       "medic",
			AuthorUsername: "medic",
			Content:        "again",
			Visibility:     models.VisibilityDirect,
			ConversationID: "conv-existing",
			PublishedAt:    publishedAt,
		},
		ParticipantStates: []*models.UserConversationState{
			{
				ViewerID:       "medic",
				ConversationID: "conv-existing",
				CounterpartID:  "arch",
				Folder:         models.UserConversationFolderInbox,
				RequestState:   models.DmRequestStateAccepted,
				Unread:         false,
				LastReadAt:     &publishedAt,
				AcceptedAt:     &publishedAt,
				CreatedAt:      publishedAt.Add(-2 * time.Hour),
			},
			{
				ViewerID:       "arch",
				ConversationID: "conv-existing",
				CounterpartID:  "medic",
				Folder:         models.UserConversationFolderInbox,
				RequestState:   models.DmRequestStateAccepted,
				Unread:         true,
				AcceptedAt:     &publishedAt,
				CreatedAt:      publishedAt.Add(-2 * time.Hour),
			},
		},
	})
	require.NoError(t, err)

	require.Len(t, builder.created, 4)
	require.Equal(t, []string{"put", "put", "put", "create"}, builder.ops)

	conversation, ok := builder.created[0].(*models.Conversation)
	require.True(t, ok)
	require.EqualValues(t, 6, conversation.TotalMessageCount)
	require.Equal(t, "status-existing", conversation.LastStatusID)

	status, ok := builder.created[3].(*models.Status)
	require.True(t, ok)
	require.Equal(t, "status-existing", status.StatusID)
}

func TestRound44_ConversationRepository_ApplyDirectMessageSend_MapsCreateRaceToAlreadyExists(t *testing.T) {
	ctx := context.Background()
	repo := NewConversationRepository(new(mocks.MockDB), "test-table", zap.NewNop(), nil)

	repo.transactWriteFn = func(context.Context, func(core.TransactionBuilder) error) error {
		return ddbErrors.ErrConditionFailed
	}

	err := repo.ApplyDirectMessageSend(ctx, &models.DirectMessageSendTransition{
		Conversation: &models.Conversation{
			ID:           "conv-race",
			Participants: []string{"alice", "bob"},
		},
		Status: &models.Status{
			StatusID:       "status-race",
			AuthorID:       "alice",
			AuthorUsername: "alice",
			Content:        "hello",
			Visibility:     models.VisibilityDirect,
			ConversationID: "conv-race",
		},
		ParticipantStates: []*models.UserConversationState{
			{
				ViewerID:       "alice",
				ConversationID: "conv-race",
				CounterpartID:  "bob",
				Folder:         models.UserConversationFolderInbox,
			},
			{
				ViewerID:       "bob",
				ConversationID: "conv-race",
				CounterpartID:  "alice",
				Folder:         models.UserConversationFolderRequests,
			},
		},
		CreateConversation: true,
	})
	require.True(t, errors.Is(err, storage.ErrAlreadyExists))
}

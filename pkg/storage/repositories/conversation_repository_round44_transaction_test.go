package repositories

import (
	"context"
	"errors"
	"fmt"
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
	updates []recordingConversationUpdateCall
}

type recordingConversationUpdateCall struct {
	model      any
	conditions []core.TransactCondition
	builder    *recordingConversationUpdateBuilder
}

type recordingConversationUpdateBuilder struct {
	sets           map[string]any
	adds           map[string]any
	setIfNotExists map[string]any
	removes        []string
}

func recordStatusStageWithPut(tx core.TransactionBuilder, status *models.Status) error {
	tx.Put(status)
	return nil
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

func (b *recordingConversationTransactionBuilder) UpdateWithBuilder(model any, fn func(core.UpdateBuilder) error, conditions ...core.TransactCondition) core.TransactionBuilder {
	updateBuilder := &recordingConversationUpdateBuilder{
		sets:           make(map[string]any),
		adds:           make(map[string]any),
		setIfNotExists: make(map[string]any),
	}
	if fn != nil {
		_ = fn(updateBuilder)
	}
	b.updates = append(b.updates, recordingConversationUpdateCall{
		model:      model,
		conditions: append([]core.TransactCondition(nil), conditions...),
		builder:    updateBuilder,
	})
	b.ops = append(b.ops, "update_builder")
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

func (b *recordingConversationUpdateBuilder) Set(field string, value any) core.UpdateBuilder {
	b.sets[field] = value
	return b
}

func (b *recordingConversationUpdateBuilder) SetIfNotExists(field string, _ any, defaultValue any) core.UpdateBuilder {
	b.setIfNotExists[field] = defaultValue
	return b
}

func (b *recordingConversationUpdateBuilder) Add(field string, value any) core.UpdateBuilder {
	b.adds[field] = value
	return b
}

func (b *recordingConversationUpdateBuilder) Increment(field string) core.UpdateBuilder {
	return b.Add(field, 1)
}

func (b *recordingConversationUpdateBuilder) Decrement(field string) core.UpdateBuilder {
	return b.Add(field, -1)
}

func (b *recordingConversationUpdateBuilder) Remove(field string) core.UpdateBuilder {
	b.removes = append(b.removes, field)
	return b
}

func (b *recordingConversationUpdateBuilder) Delete(string, any) core.UpdateBuilder {
	return b
}

func (b *recordingConversationUpdateBuilder) AppendToList(string, any) core.UpdateBuilder {
	return b
}

func (b *recordingConversationUpdateBuilder) PrependToList(string, any) core.UpdateBuilder {
	return b
}

func (b *recordingConversationUpdateBuilder) RemoveFromListAt(string, int) core.UpdateBuilder {
	return b
}

func (b *recordingConversationUpdateBuilder) SetListElement(string, int, any) core.UpdateBuilder {
	return b
}

func (b *recordingConversationUpdateBuilder) Condition(string, string, any) core.UpdateBuilder {
	return b
}

func (b *recordingConversationUpdateBuilder) OrCondition(string, string, any) core.UpdateBuilder {
	return b
}

func (b *recordingConversationUpdateBuilder) ConditionExists(string) core.UpdateBuilder {
	return b
}

func (b *recordingConversationUpdateBuilder) ConditionNotExists(string) core.UpdateBuilder {
	return b
}

func (b *recordingConversationUpdateBuilder) ConditionVersion(int64) core.UpdateBuilder {
	return b
}

func (b *recordingConversationUpdateBuilder) ReturnValues(string) core.UpdateBuilder {
	return b
}

func (b *recordingConversationUpdateBuilder) Execute() error {
	return nil
}

func (b *recordingConversationUpdateBuilder) ExecuteWithResult(any) error {
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

	err := repo.ApplyDirectMessageSend(ctx, nil, nil)
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
	}, nil)
	require.Error(t, err)
}

func TestRound44_prepareDirectMessageSendTransition_UsesCreatedAtWhenPublishedAtMissing(t *testing.T) {
	createdAt := time.Date(2026, 3, 26, 16, 45, 0, 0, time.UTC)

	prepared, err := prepareDirectMessageSendTransition(&models.DirectMessageSendTransition{
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
		CreateConversation: true,
	})
	require.NoError(t, err)
	require.NotNil(t, prepared)
	require.NotNil(t, prepared.status)
	require.NotNil(t, prepared.conversation)
	require.Len(t, prepared.participantStates, 2)
	require.False(t, prepared.status.PublishedAt.IsZero())
	require.Equal(t, prepared.status.PublishedAt.UTC(), prepared.conversation.LastMessageTime)
	require.Equal(t, prepared.status.PublishedAt.UTC(), prepared.conversation.UpdatedAt)
	require.Equal(t, "status-created-at", prepared.conversation.LastStatusID)
}

func TestRound44_prepareDirectMessageSendTransition_RejectsExistingTransitionsWithoutExpectedParticipantStates(t *testing.T) {
	_, err := prepareDirectMessageSendTransition(&models.DirectMessageSendTransition{
		Conversation: &models.Conversation{
			ID:           "conv-existing",
			Participants: []string{"bob", "alice"},
			CreatedAt:    time.Date(2026, 3, 26, 16, 45, 0, 0, time.UTC),
			UpdatedAt:    time.Date(2026, 3, 26, 16, 45, 0, 0, time.UTC),
		},
		Status: &models.Status{
			StatusID:       "status-existing",
			AuthorID:       "alice",
			AuthorUsername: "alice",
			Content:        "hello",
			Visibility:     models.VisibilityDirect,
			PublishedAt:    time.Date(2026, 3, 26, 16, 50, 0, 0, time.UTC),
		},
		ParticipantStates: []*models.UserConversationState{
			{
				ViewerID:       "alice",
				ConversationID: "conv-existing",
				CounterpartID:  "bob",
				Folder:         models.UserConversationFolderInbox,
			},
			{
				ViewerID:       "bob",
				ConversationID: "conv-existing",
				CounterpartID:  "alice",
				Folder:         models.UserConversationFolderRequests,
			},
		},
	})
	require.ErrorIs(t, err, storage.ErrInvalidInput)
}

func TestRound44_applyDirectMessageParticipantStateUpdate_RemovesClearedOptionalFields(t *testing.T) {
	updateBuilder := &recordingConversationUpdateBuilder{
		sets:           make(map[string]any),
		adds:           make(map[string]any),
		setIfNotExists: make(map[string]any),
	}

	applyDirectMessageParticipantStateUpdate(updateBuilder, &models.UserConversationState{
		CounterpartID: "bob",
		Folder:        models.UserConversationFolderInbox,
		SortAt:        time.Date(2026, 3, 26, 17, 0, 0, 0, time.UTC),
		Unread:        false,
		UpdatedAt:     time.Date(2026, 3, 26, 17, 1, 0, 0, time.UTC),
	})

	require.Equal(t, "bob", updateBuilder.sets["CounterpartID"])
	require.Equal(t, models.UserConversationFolderInbox, updateBuilder.sets["Folder"])
	require.Equal(t, false, updateBuilder.sets["Unread"])
	require.Contains(t, updateBuilder.removes, "RequestState")
	require.Contains(t, updateBuilder.removes, "PreviewStatusID")
	require.Contains(t, updateBuilder.removes, "PreviewStatusPublishedAt")
	require.Contains(t, updateBuilder.removes, "LastReadAt")
	require.Contains(t, updateBuilder.removes, "DeletedAt")
	require.Contains(t, updateBuilder.removes, "RequestedAt")
	require.Contains(t, updateBuilder.removes, "AcceptedAt")
	require.Contains(t, updateBuilder.removes, "DeclinedAt")
}

func TestRound44_directMessageSendHelpers_HandleNilInputs(t *testing.T) {
	require.Nil(t, cloneConversationModel(nil))
	require.Nil(t, directMessageSendConversationConditions(nil))
	require.Nil(t, directMessageSendParticipantStateConditions(nil))

	statusRepo := NewStatusRepository(new(mocks.MockDB), "test-table", zap.NewNop(), nil)
	require.Error(t, statusRepo.FinalizeCreatedStatus(context.Background(), nil))
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
	}, recordStatusStageWithPut)
	require.NoError(t, err)

	require.Len(t, builder.created, 5)
	require.Equal(t, []string{"create", "create", "create", "create", "put"}, builder.ops)

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
		ExpectedParticipantStates: []*models.UserConversationState{
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
				UpdatedAt:      publishedAt.Add(-2 * time.Hour),
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
				UpdatedAt:      publishedAt.Add(-2 * time.Hour),
			},
		},
	}, recordStatusStageWithPut)
	require.NoError(t, err)

	require.Len(t, builder.created, 1)
	require.Len(t, builder.updates, 3)
	require.Equal(t, []string{"update_builder", "update_builder", "update_builder", "put"}, builder.ops)

	conversationUpdate := builder.updates[0]
	conversation, ok := conversationUpdate.model.(*models.Conversation)
	require.True(t, ok)
	require.EqualValues(t, 5, conversation.TotalMessageCount)
	require.Equal(t, publishedAt.Add(-time.Hour), conversation.UpdatedAt)
	require.EqualValues(t, 1, conversationUpdate.builder.adds["TotalMessageCount"])
	require.Empty(t, conversationUpdate.builder.setIfNotExists)
	require.Equal(t, "status-existing", conversationUpdate.builder.sets["LastStatusID"])
	require.Equal(t, publishedAt, conversationUpdate.builder.sets["LastMessageTime"])
	require.Equal(t, publishedAt, conversationUpdate.builder.sets["UpdatedAt"])
	require.Contains(t, conversationUpdate.conditions, core.TransactCondition{Kind: core.TransactConditionKindPrimaryKeyExists})
	require.Contains(t, conversationUpdate.conditions, core.TransactCondition{Field: "TotalMessageCount", Operator: "=", Value: int64(5)})
	require.Contains(t, conversationUpdate.conditions, core.TransactCondition{Field: "UpdatedAt", Operator: "=", Value: publishedAt.Add(-time.Hour)})

	stateUpdatesByViewer := make(map[string]recordingConversationUpdateCall, 2)
	for _, update := range builder.updates[1:] {
		state, ok := update.model.(*models.UserConversationState)
		require.True(t, ok)
		stateUpdatesByViewer[state.ViewerID] = update
	}

	medicUpdate := stateUpdatesByViewer["medic"]
	require.Equal(t, "status-existing", medicUpdate.builder.sets["PreviewStatusID"])
	require.Equal(t, false, medicUpdate.builder.sets["Unread"])
	require.Contains(t, medicUpdate.conditions, core.TransactCondition{Kind: core.TransactConditionKindPrimaryKeyExists})
	require.Contains(t, medicUpdate.conditions, core.TransactCondition{Field: "UpdatedAt", Operator: "=", Value: publishedAt.Add(-2 * time.Hour)})

	archUpdate := stateUpdatesByViewer["arch"]
	require.Equal(t, "status-existing", archUpdate.builder.sets["PreviewStatusID"])
	require.Equal(t, true, archUpdate.builder.sets["Unread"])

	status, ok := builder.created[0].(*models.Status)
	require.True(t, ok)
	require.Equal(t, "status-existing", status.StatusID)
}

func TestRound44_ConversationRepository_ApplyDirectMessageSend_MapsExistingConversationConflictsToVersionConflict(t *testing.T) {
	ctx := context.Background()
	repo := NewConversationRepository(new(mocks.MockDB), "test-table", zap.NewNop(), nil)

	repo.transactWriteFn = func(context.Context, func(core.TransactionBuilder) error) error {
		return ddbErrors.ErrConditionFailed
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
			},
			{
				ViewerID:       "arch",
				ConversationID: "conv-existing",
				CounterpartID:  "medic",
				Folder:         models.UserConversationFolderInbox,
			},
		},
		ExpectedParticipantStates: []*models.UserConversationState{
			{
				ViewerID:       "medic",
				ConversationID: "conv-existing",
				CounterpartID:  "arch",
				Folder:         models.UserConversationFolderInbox,
				UpdatedAt:      publishedAt.Add(-2 * time.Hour),
			},
			{
				ViewerID:       "arch",
				ConversationID: "conv-existing",
				CounterpartID:  "medic",
				Folder:         models.UserConversationFolderInbox,
				UpdatedAt:      publishedAt.Add(-2 * time.Hour),
			},
		},
	}, recordStatusStageWithPut)
	require.ErrorIs(t, err, storage.ErrVersionConflict)
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
	}, recordStatusStageWithPut)
	require.True(t, errors.Is(err, storage.ErrAlreadyExists))
}

func TestRound44_ConversationRepository_ApplyDirectMessageSend_PreservesStageRootCause(t *testing.T) {
	ctx := context.Background()
	repo := NewConversationRepository(new(mocks.MockDB), "test-table", zap.NewNop(), nil)
	builder := &recordingConversationTransactionBuilder{}

	repo.transactWriteFn = func(txCtx context.Context, fn func(core.TransactionBuilder) error) error {
		require.Equal(t, ctx, txCtx)
		return fn(builder)
	}

	publishedAt := time.Now().UTC()
	rawStageErr := errors.New("dynamo attribute type mismatch")
	err := repo.ApplyDirectMessageSend(ctx, &models.DirectMessageSendTransition{
		Conversation: &models.Conversation{
			ID:              "conv-stage",
			Participants:    []string{"alice", "bob"},
			CreatedAt:       publishedAt,
			UpdatedAt:       publishedAt,
			LastMessageTime: publishedAt,
		},
		Status: &models.Status{
			StatusID:    "status-stage",
			PublishedAt: publishedAt,
		},
		ParticipantStates: []*models.UserConversationState{
			{ViewerID: "alice", ConversationID: "conv-stage", CounterpartID: "bob", Folder: models.UserConversationFolderInbox},
			{ViewerID: "bob", ConversationID: "conv-stage", CounterpartID: "alice", Folder: models.UserConversationFolderInbox},
		},
		CreateConversation: true,
	}, func(tx core.TransactionBuilder, status *models.Status) error {
		require.NotNil(t, tx)
		require.Equal(t, "status-stage", status.StatusID)
		return fmt.Errorf("stage direct message status create %s: %w", status.StatusID, rawStageErr)
	})
	require.Error(t, err)
	require.ErrorContains(t, err, "Failed to create conversation")
	require.ErrorIs(t, err, rawStageErr)
}

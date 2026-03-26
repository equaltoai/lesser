package repositories

import (
	"context"
	"testing"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/pkg/core"
	"github.com/theory-cloud/tabletheory/pkg/mocks"
	"go.uber.org/zap"
)

type recordingConversationTransactionBuilder struct {
	created []any
}

func (b *recordingConversationTransactionBuilder) Put(model any, _ ...core.TransactCondition) core.TransactionBuilder {
	b.created = append(b.created, model)
	return b
}

func (b *recordingConversationTransactionBuilder) Create(model any, _ ...core.TransactCondition) core.TransactionBuilder {
	b.created = append(b.created, model)
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

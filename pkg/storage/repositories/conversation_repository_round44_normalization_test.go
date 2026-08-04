package repositories

import (
	"context"
	stdErrors "errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/v3/pkg/core"
	theorydbErrors "github.com/theory-cloud/tabletheory/v3/pkg/errors"
	"github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	"go.uber.org/zap"
)

func TestRound44_ConversationRepository_CreateConversationWithParticipantStates_LegacyPath(t *testing.T) {
	baseTime := time.Date(2026, 3, 26, 12, 0, 0, 0, time.UTC)
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	setupPermissiveRound07Mocks(mockDB, mockQuery, nil, baseTime)

	repo := NewConversationRepository(mockDB, "test-table", zap.NewNop(), nil)
	conversation := &models.Conversation{
		ID:              "conv-legacy-explicit",
		LastStatusID:    "status-legacy",
		LastMessageTime: baseTime,
		CreatedAt:       baseTime,
		UpdatedAt:       baseTime,
	}

	explicitStates := []*models.UserConversationState{
		{
			ViewerID:       "Medic",
			ConversationID: "conv-legacy-explicit",
			CounterpartID:  "Arch",
			Folder:         models.UserConversationFolderInbox,
			RequestState:   models.DmRequestStateAccepted,
		},
		{
			ViewerID:       "Arch",
			ConversationID: "conv-legacy-explicit",
			CounterpartID:  "Medic",
			Folder:         models.UserConversationFolderHidden,
		},
	}

	require.NoError(t, repo.CreateConversationWithParticipantStates(context.Background(), conversation, []string{"Medic", "Arch"}, explicitStates))
	require.Equal(t, []string{"arch", "medic"}, conversation.Participants)
}

func TestRound44_NormalizeConversationParticipantStates_ExplicitStatesFillDerivedFieldsAndOrdering(t *testing.T) {
	createdAt := time.Date(2026, 3, 26, 9, 0, 0, 0, time.UTC)
	updatedAt := createdAt.Add(30 * time.Minute)
	lastMessageAt := createdAt.Add(time.Hour)
	conversation := &models.Conversation{
		ID:              "conv-helpers",
		Participants:    []string{"Medic", "Arch"},
		LastStatusID:    "status-helpers",
		LastMessageTime: lastMessageAt,
		CreatedAt:       createdAt,
		UpdatedAt:       updatedAt,
	}

	states, err := normalizeConversationParticipantStates(conversation, []*models.UserConversationState{
		{
			ViewerID:                 "Medic",
			Folder:                   models.UserConversationFolderInbox,
			RequestState:             models.DmRequestStateAccepted,
			PreviewStatusPublishedAt: createdAt.Add(15 * time.Minute),
		},
		{
			ViewerID: "Arch",
			Folder:   models.UserConversationFolderRequests,
		},
	})
	require.NoError(t, err)
	require.Len(t, states, 2)

	require.Equal(t, "arch", states[0].ViewerID)
	require.Equal(t, "conv-helpers", states[0].ConversationID)
	require.Equal(t, "medic", states[0].CounterpartID)
	require.Equal(t, "status-helpers", states[0].PreviewStatusID)
	require.Equal(t, lastMessageAt, states[0].PreviewStatusPublishedAt)
	require.Equal(t, lastMessageAt, states[0].SortAt)
	require.Equal(t, "USER_CONVERSATION_STATE#arch", states[0].PK)
	require.Equal(t, "CONVERSATION#conv-helpers", states[0].SK)

	require.Equal(t, "medic", states[1].ViewerID)
	require.Equal(t, "conv-helpers", states[1].ConversationID)
	require.Equal(t, "arch", states[1].CounterpartID)
	require.Equal(t, createdAt, states[1].CreatedAt)
	require.Equal(t, updatedAt, states[1].UpdatedAt)
	require.Equal(t, "status-helpers", states[1].PreviewStatusID)
	require.Equal(t, createdAt.Add(15*time.Minute), states[1].PreviewStatusPublishedAt)
	require.Equal(t, createdAt.Add(15*time.Minute), states[1].SortAt)
	require.Equal(t, "USER_CONVERSATION_STATE#medic", states[1].PK)
	require.Equal(t, "CONVERSATION#conv-helpers", states[1].SK)
}

func TestRound44_NormalizeConversationParticipantStates_RejectsInvalidInputs(t *testing.T) {
	conversation := &models.Conversation{
		ID:           "conv-invalid",
		Participants: []string{"Medic", "Arch"},
		CreatedAt:    time.Date(2026, 3, 26, 10, 0, 0, 0, time.UTC),
		UpdatedAt:    time.Date(2026, 3, 26, 10, 30, 0, 0, time.UTC),
	}

	t.Run("nil conversation is rejected", func(t *testing.T) {
		_, err := canonicalConversationParticipantsForStates(nil)
		require.ErrorIs(t, err, storage.ErrInvalidInput)
	})

	t.Run("empty participant set is rejected", func(t *testing.T) {
		_, err := canonicalConversationParticipantsForStates(&models.Conversation{ID: "conv-empty"})
		require.ErrorIs(t, err, storage.ErrInvalidInput)
	})

	t.Run("duplicate viewers are rejected", func(t *testing.T) {
		_, err := normalizeConversationParticipantStates(conversation, []*models.UserConversationState{
			{ViewerID: "Medic", CounterpartID: "Arch", Folder: models.UserConversationFolderInbox},
			{ViewerID: "medic", CounterpartID: "Arch", Folder: models.UserConversationFolderInbox},
		})
		require.ErrorIs(t, err, storage.ErrInvalidInput)
	})

	t.Run("missing participant rows are rejected", func(t *testing.T) {
		_, err := normalizeConversationParticipantStates(conversation, []*models.UserConversationState{
			{ViewerID: "Medic", CounterpartID: "Arch", Folder: models.UserConversationFolderInbox},
		})
		require.ErrorIs(t, err, storage.ErrInvalidInput)
	})

	t.Run("mismatched conversation ids are rejected", func(t *testing.T) {
		_, err := normalizeConversationParticipantStateCandidate(conversation, []string{"arch", "medic"}, &models.UserConversationState{
			ViewerID:       "Medic",
			ConversationID: "other-conv",
			CounterpartID:  "Arch",
			Folder:         models.UserConversationFolderInbox,
		})
		require.ErrorIs(t, err, storage.ErrInvalidInput)
	})

	t.Run("missing viewer ids are rejected", func(t *testing.T) {
		_, err := normalizeConversationParticipantStateCandidate(conversation, []string{"arch", "medic"}, &models.UserConversationState{
			CounterpartID: "Arch",
			Folder:        models.UserConversationFolderInbox,
		})
		require.ErrorIs(t, err, storage.ErrInvalidInput)
	})
}

func TestRound44_ConversationParticipantStateSortAt_Fallbacks(t *testing.T) {
	updatedAt := time.Date(2026, 3, 26, 11, 0, 0, 0, time.UTC)
	require.Equal(t, updatedAt, conversationParticipantStateSortAt(&models.Conversation{UpdatedAt: updatedAt}, nil))

	before := time.Now().UTC()
	sortAt := conversationParticipantStateSortAt(nil, nil)
	after := time.Now().UTC()
	require.False(t, sortAt.Before(before))
	require.False(t, sortAt.After(after))
}

func TestRound44_CreateOrUpdateUserConversationStates_PropagatesErrors(t *testing.T) {
	repo := NewConversationRepository(new(mocks.MockDB), "test-table", zap.NewNop(), nil)
	err := repo.createOrUpdateUserConversationStates(context.Background(), []*models.UserConversationState{nil})
	require.ErrorIs(t, err, storage.ErrInvalidInput)
}

func TestRound44_NewConversationTransactWriteFn_UsesTransactionalDB(t *testing.T) {
	ctx := context.Background()
	db := new(mocks.MockExtendedDB)
	builder := &recordingConversationTransactionBuilder{}

	db.On("TransactWrite", ctx, mock.Anything).Run(func(args mock.Arguments) {
		fn, ok := args.Get(1).(func(core.TransactionBuilder) error)
		require.True(t, ok)
		require.NoError(t, fn(builder))
	}).Return(nil).Once()

	transactWrite := newConversationTransactWriteFn(db)
	require.NotNil(t, transactWrite)
	require.NoError(t, transactWrite(ctx, func(tx core.TransactionBuilder) error {
		tx.Create(&models.Conversation{ID: "conv-tx"})
		return nil
	}))
	require.Len(t, builder.created, 1)
}

func TestRound44_CreateConversationLegacy_ExplicitStateError(t *testing.T) {
	baseTime := time.Date(2026, 3, 26, 14, 0, 0, 0, time.UTC)
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	setupPermissiveRound07Mocks(mockDB, mockQuery, nil, baseTime)

	repo := NewConversationRepository(mockDB, "test-table", zap.NewNop(), nil)
	conversation := &models.Conversation{
		ID:           "conv-explicit-error",
		Participants: []string{"arch", "medic"},
		CreatedAt:    baseTime,
		UpdatedAt:    baseTime,
	}
	require.NoError(t, conversation.BeforeCreate())

	err := repo.createConversationLegacy(context.Background(), zap.NewNop(), conversation, []*models.UserConversationState{nil}, true)
	require.Error(t, err)
}

func TestRound44_CreateConversation_RejectsNilConversation(t *testing.T) {
	repo := NewConversationRepository(new(mocks.MockDB), "test-table", zap.NewNop(), nil)
	err := repo.createConversation(context.Background(), nil, []string{"arch", "medic"}, nil)
	require.Error(t, err)
}

func TestRound44_CreateConversation_TransactionalConditionFailureReturnsAlreadyExists(t *testing.T) {
	repo := NewConversationRepository(new(mocks.MockDB), "test-table", zap.NewNop(), nil)
	repo.transactWriteFn = func(_ context.Context, fn func(core.TransactionBuilder) error) error {
		require.NoError(t, fn(&recordingConversationTransactionBuilder{}))
		return theorydbErrors.ErrConditionFailed
	}

	err := repo.CreateConversation(context.Background(), &models.Conversation{ID: "conv-race"}, []string{"Medic", "Arch"})
	require.ErrorIs(t, err, storage.ErrAlreadyExists)
}

func TestRound44_CreateConversation_TransactionalErrorReturnsCreateError(t *testing.T) {
	repo := NewConversationRepository(new(mocks.MockDB), "test-table", zap.NewNop(), nil)
	repo.transactWriteFn = func(_ context.Context, fn func(core.TransactionBuilder) error) error {
		require.NoError(t, fn(&recordingConversationTransactionBuilder{}))
		return stdErrors.New("transact-write-failed")
	}

	err := repo.CreateConversation(context.Background(), &models.Conversation{ID: "conv-fail"}, []string{"Medic", "Arch"})
	require.Error(t, err)
}

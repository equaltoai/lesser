package conversations_test

import (
	"context"
	"encoding/json"
	"testing"

	conversationsvc "github.com/equaltoai/lesser/pkg/services/conversations"
	notessvc "github.com/equaltoai/lesser/pkg/services/notes"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/notecontract"
	"github.com/equaltoai/lesser/pkg/streaming"
	"github.com/equaltoai/lesser/pkg/testing/inmemory"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/v3/pkg/core"
	"go.uber.org/zap"
)

type capturingCanonicalStatusRepo struct {
	*inmemory.StatusRepository
	persisted map[string]map[string]any
}

func newCapturingCanonicalStatusRepo() *capturingCanonicalStatusRepo {
	return &capturingCanonicalStatusRepo{
		StatusRepository: inmemory.NewStatusRepository(),
		persisted:        make(map[string]map[string]any),
	}
}

func (r *capturingCanonicalStatusRepo) CreateStatus(ctx context.Context, status *models.Status) error {
	if err := r.prepare(status); err != nil {
		return err
	}
	if err := r.capture(status); err != nil {
		return err
	}
	return r.StatusRepository.CreateStatus(ctx, status)
}

func (r *capturingCanonicalStatusRepo) PrepareStatusCreate(status *models.Status) error {
	if err := r.prepare(status); err != nil {
		return err
	}
	return r.capture(status)
}

func (r *capturingCanonicalStatusRepo) StageStatusCreate(_ core.TransactionBuilder, status *models.Status) error {
	return r.capture(status)
}

func (r *capturingCanonicalStatusRepo) FinalizeCreatedStatus(ctx context.Context, status *models.Status) error {
	if err := r.capture(status); err != nil {
		return err
	}
	if _, err := r.StatusRepository.GetStatus(ctx, status.StatusID); err == nil {
		return nil
	}
	return r.StatusRepository.CreateStatus(ctx, status)
}

func (r *capturingCanonicalStatusRepo) persistedNote(t *testing.T, statusID string) map[string]any {
	t.Helper()

	raw, ok := r.persisted[statusID]
	require.True(t, ok, "missing persisted note capture for %s", statusID)

	blob, err := json.Marshal(raw)
	require.NoError(t, err)

	var out map[string]any
	require.NoError(t, json.Unmarshal(blob, &out))
	return out
}

func (r *capturingCanonicalStatusRepo) prepare(status *models.Status) error {
	if status == nil {
		return nil
	}
	if status.Note != nil {
		normalized, err := notecontract.Normalize(status.Note)
		if err != nil {
			return err
		}
		status.Note = normalized
	}
	return status.BeforeCreate()
}

func (r *capturingCanonicalStatusRepo) capture(status *models.Status) error {
	if status == nil {
		return nil
	}
	raw, err := notecontract.Marshal(status.Note)
	if err != nil {
		return err
	}
	r.persisted[status.StatusID] = raw
	return nil
}

type transactionalInMemoryConversationRepo struct {
	*inmemory.ConversationRepository
}

func (transactionalInMemoryConversationRepo) TransactionalDirectMessageSendEnabled() bool {
	return true
}

func TestStatusNoteContractAcrossEntryPoints(t *testing.T) {
	ctx := context.Background()
	domain := "lesser.example"
	logger := zap.NewNop()
	publisher := streaming.NewNoopPublisher()

	statusRepo := newCapturingCanonicalStatusRepo()
	accountRepo := inmemory.NewAccountRepository()
	userRepo := inmemory.NewUserRepository()
	conversationRepo := transactionalInMemoryConversationRepo{ConversationRepository: inmemory.NewConversationRepository()}

	require.NoError(t, accountRepo.CreateAccount(ctx, &storage.Account{User: &storage.User{
		Username:    "alice",
		Email:       "alice@lesser.example",
		DisplayName: "alice",
		Approved:    true,
	}}))
	require.NoError(t, accountRepo.CreateAccount(ctx, &storage.Account{User: &storage.User{
		Username:    "bob",
		Email:       "bob@lesser.example",
		DisplayName: "bob",
		Approved:    true,
	}}))
	require.NoError(t, userRepo.UpdateUserPreferences(ctx, "bob", &storage.UserPreferences{
		Username:           "bob",
		DirectMessagesFrom: "ANYONE",
	}))

	notesService := notessvc.NewService(statusRepo, accountRepo, nil, nil, nil, nil, nil, conversationRepo, nil, nil, nil, userRepo, nil, publisher, nil, nil, nil, nil, logger, domain)
	conversationService := conversationsvc.NewService(conversationRepo, statusRepo, nil, accountRepo, nil, userRepo, nil, nil, publisher, nil, logger, domain)

	publicResult, err := notesService.CreateNote(ctx, &notessvc.CreateNoteCommand{
		AuthorID:     "alice",
		Content:      "public status contract #contract @bob",
		Visibility:   models.VisibilityPublic,
		Sensitive:    true,
		SpoilerText:  "fixture spoiler",
		ToRecipients: []string{"https://www.w3.org/ns/activitystreams#Public"},
		CcRecipients: []string{"https://lesser.example/users/alice/followers"},
	})
	require.NoError(t, err)
	require.NotNil(t, publicResult)
	require.NotNil(t, publicResult.Note)

	firstDMResult, err := conversationService.SendDirectMessage(ctx, &conversationsvc.SendDirectMessageCommand{
		SenderID:   "alice",
		Recipients: []string{"bob"},
		Content:    "shared direct status contract content",
	})
	require.NoError(t, err)
	require.NotNil(t, firstDMResult)
	require.NotNil(t, firstDMResult.Message)
	require.NotNil(t, firstDMResult.Conversation)

	secondDMResult, err := conversationService.SendMessage(ctx, &conversationsvc.SendMessageCommand{
		SenderID:       "alice",
		ConversationID: firstDMResult.Conversation.ID,
		Content:        "shared direct status contract content",
	})
	require.NoError(t, err)
	require.NotNil(t, secondDMResult)
	require.NotNil(t, secondDMResult.Message)

	publicRaw := statusRepo.persistedNote(t, publicResult.Note.StatusID)
	firstDMRaw := sanitizePersistedNoteForContractCompare(statusRepo.persistedNote(t, firstDMResult.Message.StatusID))
	secondDMRaw := sanitizePersistedNoteForContractCompare(statusRepo.persistedNote(t, secondDMResult.Message.StatusID))

	publicBase := persistedNoteBaseObject(t, publicRaw)
	require.Len(t, persistedContextValues(t, publicBase["Context"]), 2)
	require.Equal(t, []any{"https://www.w3.org/ns/activitystreams#Public"}, persistedStringSlice(t, publicBase["To"]))
	require.Equal(t, []any{
		"https://lesser.example/users/alice/followers",
		"https://lesser.example/users/bob",
	}, persistedStringSlice(t, publicBase["CC"]))
	require.Len(t, persistedTagSlice(t, publicRaw["Tag"]), 2)

	firstDMBase := persistedNoteBaseObject(t, firstDMRaw)
	require.Len(t, persistedContextValues(t, firstDMBase["Context"]), 2)
	require.Len(t, persistedStringSlice(t, firstDMBase["To"]), 1)
	require.Len(t, persistedTagSlice(t, firstDMRaw["Tag"]), 1)
	require.Equal(t, firstDMRaw, secondDMRaw)
}

func sanitizePersistedNoteForContractCompare(raw map[string]any) map[string]any {
	if raw == nil {
		return nil
	}

	blob, _ := json.Marshal(raw)
	var clone map[string]any
	_ = json.Unmarshal(blob, &clone)

	if baseObject, ok := clone["BaseObject"].(map[string]any); ok {
		delete(baseObject, "ID")
		delete(baseObject, "Published")
		delete(baseObject, "Updated")
	}

	return clone
}

func persistedNoteBaseObject(t *testing.T, raw any) map[string]any {
	t.Helper()

	typed, ok := raw.(map[string]any)
	require.True(t, ok)
	base, ok := typed["BaseObject"].(map[string]any)
	require.True(t, ok)
	return base
}

func persistedContextValues(t *testing.T, raw any) []any {
	t.Helper()

	values, ok := raw.([]any)
	require.True(t, ok)
	return values
}

func persistedStringSlice(t *testing.T, raw any) []any {
	t.Helper()

	values, ok := raw.([]any)
	require.True(t, ok)
	return values
}

func persistedTagSlice(t *testing.T, raw any) []any {
	t.Helper()

	values, ok := raw.([]any)
	require.True(t, ok)
	return values
}

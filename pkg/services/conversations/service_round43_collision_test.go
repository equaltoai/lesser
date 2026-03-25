package conversations

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

type concurrentDMConversationRepo struct {
	mockConversationRepository

	mu             sync.Mutex
	lookupRows     map[string]*models.Conversation
	conversations  map[string]*models.Conversation
	initialLookups int
	releaseInitial chan struct{}
}

func newConcurrentDMConversationRepo() *concurrentDMConversationRepo {
	return &concurrentDMConversationRepo{
		lookupRows:     map[string]*models.Conversation{},
		conversations:  map[string]*models.Conversation{},
		releaseInitial: make(chan struct{}),
	}
}

func (r *concurrentDMConversationRepo) GetConversationByParticipants(_ context.Context, participants []string) (*models.Conversation, error) {
	key := fmt.Sprintf("%v", models.CanonicalConversationParticipants(participants))

	r.mu.Lock()
	if conversation := r.lookupRows[key]; conversation != nil {
		r.mu.Unlock()
		return cloneConversation(conversation), nil
	}

	r.initialLookups++
	releaseInitial := r.releaseInitial
	if r.initialLookups == 2 {
		close(r.releaseInitial)
	}
	r.mu.Unlock()

	<-releaseInitial

	r.mu.Lock()
	defer r.mu.Unlock()
	if conversation := r.lookupRows[key]; conversation != nil {
		return cloneConversation(conversation), nil
	}
	return nil, fmt.Errorf("not found")
}

func (r *concurrentDMConversationRepo) CreateConversation(_ context.Context, conversation *models.Conversation, participants []string) error {
	key := fmt.Sprintf("%v", models.CanonicalConversationParticipants(participants))

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.lookupRows[key]; exists {
		return storage.ErrAlreadyExists
	}

	stored := cloneConversation(conversation)
	stored.Participants = append([]string(nil), models.CanonicalConversationParticipants(participants)...)
	r.lookupRows[key] = stored
	r.conversations[stored.ID] = stored
	return nil
}

func (r *concurrentDMConversationRepo) lookupRowCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.lookupRows)
}

func (r *concurrentDMConversationRepo) conversationCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.conversations)
}

func cloneConversation(conversation *models.Conversation) *models.Conversation {
	if conversation == nil {
		return nil
	}

	cloned := *conversation
	cloned.Participants = append([]string(nil), conversation.Participants...)
	return &cloned
}

func TestRound43_Service_getOrCreateDirectMessageConversation_ConcurrentCreatesResolveToOneConversation(t *testing.T) {
	ctx := context.Background()
	repo := newConcurrentDMConversationRepo()
	service := NewService(repo, nil, nil, nil, nil, nil, nil, nil, nil, nil, zaptest.NewLogger(t), "example.com")

	cmd := &SendDirectMessageCommand{
		SenderID:   "Arch",
		Recipients: []string{"Medic"},
	}

	type result struct {
		conversation *models.Conversation
		err          error
	}

	results := make(chan result, 2)
	for range 2 {
		go func() {
			conversation, err := service.getOrCreateDirectMessageConversation(ctx, cmd, "Medic")
			results <- result{conversation: conversation, err: err}
		}()
	}

	first := <-results
	second := <-results
	require.NoError(t, first.err)
	require.NoError(t, second.err)
	require.NotNil(t, first.conversation)
	require.NotNil(t, second.conversation)
	require.Equal(t, first.conversation.ID, second.conversation.ID)
	require.Equal(t, 1, repo.lookupRowCount())
	require.Equal(t, 1, repo.conversationCount())
}

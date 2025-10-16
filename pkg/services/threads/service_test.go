package threads

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/equaltoai/lesser/pkg/streaming"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap/zaptest"
)

// Mock repositories
type MockThreadRepository struct {
	mock.Mock
}

func (m *MockThreadRepository) SaveThreadSync(ctx context.Context, sync *models.ThreadSync) error {
	args := m.Called(ctx, sync)
	return args.Error(0)
}

func (m *MockThreadRepository) GetThreadSync(ctx context.Context, statusID string) (*models.ThreadSync, error) {
	args := m.Called(ctx, statusID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.ThreadSync), args.Error(1)
}

func (m *MockThreadRepository) SaveThreadNode(ctx context.Context, node *models.ThreadNode) error {
	args := m.Called(ctx, node)
	return args.Error(0)
}

func (m *MockThreadRepository) GetThreadNodes(ctx context.Context, rootStatusID string) ([]*models.ThreadNode, error) {
	args := m.Called(ctx, rootStatusID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.ThreadNode), args.Error(1)
}

func (m *MockThreadRepository) GetThreadNode(ctx context.Context, rootStatusID, statusID string) (*models.ThreadNode, error) {
	args := m.Called(ctx, rootStatusID, statusID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.ThreadNode), args.Error(1)
}

func (m *MockThreadRepository) GetThreadNodeByStatusID(ctx context.Context, statusID string) (*models.ThreadNode, error) {
	args := m.Called(ctx, statusID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.ThreadNode), args.Error(1)
}

func (m *MockThreadRepository) MarkMissingReplies(ctx context.Context, rootStatusID, parentStatusID string, replyIDs []string) error {
	args := m.Called(ctx, rootStatusID, parentStatusID, replyIDs)
	return args.Error(0)
}

func (m *MockThreadRepository) GetMissingReplies(ctx context.Context, rootStatusID string) ([]*models.MissingReply, error) {
	args := m.Called(ctx, rootStatusID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.MissingReply), args.Error(1)
}

func (m *MockThreadRepository) GetThreadContext(ctx context.Context, statusID string) (*repositories.ThreadContextResult, error) {
	args := m.Called(ctx, statusID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*repositories.ThreadContextResult), args.Error(1)
}

func (m *MockThreadRepository) SaveMissingReply(ctx context.Context, missing *models.MissingReply) error {
	args := m.Called(ctx, missing)
	return args.Error(0)
}

func (m *MockThreadRepository) DeleteMissingReply(ctx context.Context, rootStatusID, replyID string) error {
	args := m.Called(ctx, rootStatusID, replyID)
	return args.Error(0)
}

type MockObjectRepository struct {
	mock.Mock
}

func (m *MockObjectRepository) GetObject(ctx context.Context, objectID string) (any, error) {
	args := m.Called(ctx, objectID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0), args.Error(1)
}

func (m *MockObjectRepository) CreateObject(ctx context.Context, object any) error {
	args := m.Called(ctx, object)
	return args.Error(0)
}

type MockActorRepository struct {
	mock.Mock
}

func (m *MockActorRepository) GetActorByUsername(ctx context.Context, username string) (*activitypub.Actor, error) {
	args := m.Called(ctx, username)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*activitypub.Actor), args.Error(1)
}

type MockStatusRepository struct {
	mock.Mock
}

func (m *MockStatusRepository) GetReplies(ctx context.Context, parentStatusID string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Status], error) {
	args := m.Called(ctx, parentStatusID, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*interfaces.PaginatedResult[*models.Status]), args.Error(1)
}

type MockFederationClient struct {
	mock.Mock
}

func (m *MockFederationClient) FetchObject(ctx context.Context, objectURL string, signingActor *activitypub.Actor) (any, error) {
	args := m.Called(ctx, objectURL, signingActor)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0), args.Error(1)
}

func (m *MockFederationClient) FetchActor(ctx context.Context, actorURL string, signingActor *activitypub.Actor) (*activitypub.Actor, error) {
	args := m.Called(ctx, actorURL, signingActor)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*activitypub.Actor), args.Error(1)
}

type MockPublisher struct {
	mock.Mock
}

func (m *MockPublisher) PublishToStream(ctx context.Context, stream string, event *streaming.Event) error {
	args := m.Called(ctx, stream, event)
	return args.Error(0)
}

func (m *MockPublisher) Close() error {
	args := m.Called()
	return args.Error(0)
}

// Helper to create test notes
func createTestNote(id string, inReplyTo string) *activitypub.Note {
	now := time.Now()
	return &activitypub.Note{
		BaseObject: activitypub.BaseObject{
			ID:        id,
			Type:      "Note",
			Published: &now,
			InReplyTo: inReplyTo,
		},
		Content:      "Test content for " + id,
		AttributedTo: "https://example.com/users/test",
	}
}

// Test Service Creation
func TestNewService(t *testing.T) {
	threadRepo := &MockThreadRepository{}
	statusRepo := &MockStatusRepository{}
	objectRepo := &MockObjectRepository{}
	actorRepo := &MockActorRepository{}
	federation := &MockFederationClient{}
	publisher := &MockPublisher{}
	logger := zaptest.NewLogger(t)

	service := NewService(
		threadRepo,
		statusRepo,
		objectRepo,
		actorRepo,
		federation,
		publisher,
		logger,
		"example.com",
	)

	assert.NotNil(t, service)
	assert.Equal(t, "example.com", service.domainName)
}

// Test FindThreadRoot - Simple case (no parent)
func TestFindThreadRoot_NoParent(t *testing.T) {
	ctx := context.Background()
	threadRepo := &MockThreadRepository{}
	statusRepo := &MockStatusRepository{}
	objectRepo := &MockObjectRepository{}
	actorRepo := &MockActorRepository{}
	federation := &MockFederationClient{}
	publisher := &MockPublisher{}
	logger := zaptest.NewLogger(t)

	service := NewService(
		threadRepo,
		statusRepo,
		objectRepo,
		actorRepo,
		federation,
		publisher,
		logger,
		"example.com",
	)

	rootNote := createTestNote("https://example.com/note/1", "")

	root, ancestors, err := service.FindThreadRoot(ctx, rootNote)

	assert.NoError(t, err)
	assert.Equal(t, rootNote.ID, root.ID)
	assert.Empty(t, ancestors)
}

// Test FindThreadRoot - With ancestors
func TestFindThreadRoot_WithAncestors(t *testing.T) {
	ctx := context.Background()
	threadRepo := &MockThreadRepository{}
	statusRepo := &MockStatusRepository{}
	objectRepo := &MockObjectRepository{}
	actorRepo := &MockActorRepository{}
	federation := &MockFederationClient{}
	publisher := &MockPublisher{}
	logger := zaptest.NewLogger(t)

	service := NewService(
		threadRepo,
		statusRepo,
		objectRepo,
		actorRepo,
		federation,
		publisher,
		logger,
		"example.com",
	)

	// Create a chain: root -> parent -> current
	rootNote := createTestNote("https://example.com/note/1", "")
	parentNote := createTestNote("https://example.com/note/2", "https://example.com/note/1")
	currentNote := createTestNote("https://example.com/note/3", "https://example.com/note/2")

	// Mock the object repository to return notes in sequence
	objectRepo.On("GetObject", ctx, "https://example.com/note/2").Return(parentNote, nil).Once()
	objectRepo.On("GetObject", ctx, "https://example.com/note/1").Return(rootNote, nil).Once()

	root, ancestors, err := service.FindThreadRoot(ctx, currentNote)

	assert.NoError(t, err)
	assert.Equal(t, rootNote.ID, root.ID)
	assert.Len(t, ancestors, 2)
	assert.Equal(t, rootNote.ID, ancestors[0].ID)
	assert.Equal(t, parentNote.ID, ancestors[1].ID)

	objectRepo.AssertExpectations(t)
}

// Test FindThreadRoot - Circular reference detection
func TestFindThreadRoot_CircularReference(t *testing.T) {
	ctx := context.Background()
	threadRepo := &MockThreadRepository{}
	statusRepo := &MockStatusRepository{}
	objectRepo := &MockObjectRepository{}
	actorRepo := &MockActorRepository{}
	federation := &MockFederationClient{}
	publisher := &MockPublisher{}
	logger := zaptest.NewLogger(t)

	service := NewService(
		threadRepo,
		statusRepo,
		objectRepo,
		actorRepo,
		federation,
		publisher,
		logger,
		"example.com",
	)

	// Create a circular reference: note1 -> note2 -> note1
	note1 := createTestNote("https://example.com/note/1", "https://example.com/note/2")
	note2 := createTestNote("https://example.com/note/2", "https://example.com/note/1")

	objectRepo.On("GetObject", ctx, "https://example.com/note/2").Return(note2, nil).Once()
	objectRepo.On("GetObject", ctx, "https://example.com/note/1").Return(note1, nil).Once()

	_, _, err := service.FindThreadRoot(ctx, note1)

	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrCircularReference))

	objectRepo.AssertExpectations(t)
}

// Test GetThreadContext
func TestGetThreadContext(t *testing.T) {
	ctx := context.Background()
	threadRepo := &MockThreadRepository{}
	statusRepo := &MockStatusRepository{}
	objectRepo := &MockObjectRepository{}
	actorRepo := &MockActorRepository{}
	federation := &MockFederationClient{}
	publisher := &MockPublisher{}
	logger := zaptest.NewLogger(t)

	service := NewService(
		threadRepo,
		statusRepo,
		objectRepo,
		actorRepo,
		federation,
		publisher,
		logger,
		"example.com",
	)

	rootNote := createTestNote("https://example.com/note/1", "")
	requestedNote := rootNote

	// Mock the repository calls
	objectRepo.On("GetObject", ctx, "https://example.com/note/1").Return(requestedNote, nil)

	// Mock thread context
	threadCtxResult := &repositories.ThreadContextResult{
		RootStatusID:      rootNote.ID,
		RequestedStatusID: requestedNote.ID,
		Nodes:             []*models.ThreadNode{},
		MissingReplies:    []*models.MissingReply{},
		ParticipantCount:  1,
		TotalReplyCount:   0,
		MissingCount:      0,
		MaxDepth:          0,
	}
	threadRepo.On("GetThreadContext", ctx, rootNote.ID).Return(threadCtxResult, nil)

	query := ThreadContextQuery{
		NoteID:      "https://example.com/note/1",
		ViewerID:    "test",
		IncludeTree: true,
	}

	result, err := service.GetThreadContext(ctx, query)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, rootNote.ID, result.RootNote.ID)
	assert.Equal(t, requestedNote.ID, result.RequestedNote.ID)
	assert.Equal(t, 1, result.ParticipantCount)

	objectRepo.AssertExpectations(t)
	threadRepo.AssertExpectations(t)
}

// Test fetchRepliesRecursive with local replies
func TestFetchRepliesRecursive_LocalReplies(t *testing.T) {
	ctx := context.Background()
	threadRepo := &MockThreadRepository{}
	statusRepo := &MockStatusRepository{}
	objectRepo := &MockObjectRepository{}
	actorRepo := &MockActorRepository{}
	federation := &MockFederationClient{}
	publisher := &MockPublisher{}
	logger := zaptest.NewLogger(t)

	service := NewService(
		threadRepo,
		statusRepo,
		objectRepo,
		actorRepo,
		federation,
		publisher,
		logger,
		"example.com",
	)

	// Create parent and reply
	parentNote := createTestNote("https://example.com/note/1", "")
	replyStatus := &models.Status{
		StatusID:    "https://example.com/note/2",
		AuthorID:    "https://example.com/users/test",
		Content:     "Reply content",
		InReplyToID: "https://example.com/note/1",
		PublishedAt: time.Now(),
		Visibility:  "public",
	}

	// Mock local replies query for parent
	paginatedResult := &interfaces.PaginatedResult[*models.Status]{
		Items:      []*models.Status{replyStatus},
		NextCursor: "",
		HasMore:    false,
	}
	statusRepo.On("GetReplies", ctx, parentNote.ID, interfaces.PaginationOptions{Limit: 100}).Return(paginatedResult, nil)

	// Mock empty replies for the child (no further recursion)
	emptyResult := &interfaces.PaginatedResult[*models.Status]{
		Items:      []*models.Status{},
		NextCursor: "",
		HasMore:    false,
	}
	statusRepo.On("GetReplies", ctx, replyStatus.StatusID, interfaces.PaginationOptions{Limit: 100}).Return(emptyResult, nil)

	// Mock saving thread nodes
	threadRepo.On("SaveThreadNode", ctx, mock.Anything).Return(nil)

	parentNode := models.NewThreadNode(parentNote.ID, parentNote.ID, "", 0, parentNote.AttributedTo)
	syncedCount := 0
	errors := []string{}

	// Execute
	service.fetchRepliesRecursive(ctx, parentNote, parentNode, 1, 3, nil, &syncedCount, &errors)

	// Verify
	assert.Equal(t, 1, syncedCount, "Should have synced 1 reply")
	assert.Empty(t, errors, "Should have no errors")
	assert.Len(t, parentNode.ChildIDs, 1, "Parent should have 1 child")

	statusRepo.AssertExpectations(t)
	threadRepo.AssertExpectations(t)
}

// Test SyncRemoteThread - Basic sync
func TestSyncRemoteThread(t *testing.T) {
	ctx := context.Background()
	threadRepo := &MockThreadRepository{}
	statusRepo := &MockStatusRepository{}
	objectRepo := &MockObjectRepository{}
	actorRepo := &MockActorRepository{}
	federation := &MockFederationClient{}
	publisher := &MockPublisher{}
	logger := zaptest.NewLogger(t)

	service := NewService(
		threadRepo,
		statusRepo,
		objectRepo,
		actorRepo,
		federation,
		publisher,
		logger,
		"example.com",
	)

	rootNote := createTestNote("https://remote.com/note/1", "")
	signingActor := &activitypub.Actor{
		BaseObject: activitypub.BaseObject{
			ID:   "https://example.com/users/test",
			Type: "Person",
		},
		PreferredUsername: "test",
	}

	// Mock getting signing actor
	actorRepo.On("GetActorByUsername", ctx, "test").Return(signingActor, nil)

	// Mock fetching remote note
	remoteNoteMap := map[string]any{
		"id":           rootNote.ID,
		"type":         "Note",
		"content":      rootNote.Content,
		"attributedTo": rootNote.AttributedTo,
		"published":    rootNote.Published.Format(time.RFC3339),
	}
	federation.On("FetchObject", ctx, "https://remote.com/note/1", signingActor).Return(remoteNoteMap, nil)

	// Mock storing the fetched note
	objectRepo.On("CreateObject", ctx, mock.Anything).Return(nil)

	// Mock sync record operations
	threadRepo.On("GetThreadSync", ctx, rootNote.ID).Return(nil, nil)
	threadRepo.On("SaveThreadSync", ctx, mock.Anything).Return(nil)

	// Mock saving thread node
	threadRepo.On("SaveThreadNode", ctx, mock.Anything).Return(nil)

	// Mock local replies query (no local replies for remote note)
	statusRepo.On("GetReplies", ctx, rootNote.ID, interfaces.PaginationOptions{Limit: 100}).Return(nil, fmt.Errorf("not found"))

	cmd := SyncRemoteThreadCommand{
		NoteURL:      "https://remote.com/note/1",
		Depth:        3,
		ViewerID:     "test",
		ForceRefresh: false,
	}

	result, err := service.SyncRemoteThread(ctx, cmd)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.True(t, result.Success)
	assert.Equal(t, 1, result.SyncedPosts)
	assert.Equal(t, "COMPLETE", result.SyncStatus)

	actorRepo.AssertExpectations(t)
	federation.AssertExpectations(t)
	objectRepo.AssertExpectations(t)
	threadRepo.AssertExpectations(t)
	statusRepo.AssertExpectations(t)
}

// Test statusToNote conversion
func TestStatusToNote(t *testing.T) {
	threadRepo := &MockThreadRepository{}
	statusRepo := &MockStatusRepository{}
	objectRepo := &MockObjectRepository{}
	actorRepo := &MockActorRepository{}
	federation := &MockFederationClient{}
	publisher := &MockPublisher{}
	logger := zaptest.NewLogger(t)

	service := NewService(
		threadRepo,
		statusRepo,
		objectRepo,
		actorRepo,
		federation,
		publisher,
		logger,
		"example.com",
	)

	now := time.Now()
	status := &models.Status{
		StatusID:       "https://example.com/note/1",
		AuthorID:       "https://example.com/users/alice",
		Content:        "Test content",
		InReplyToID:    "https://example.com/note/0",
		PublishedAt:    now,
		Visibility:     "public",
		Sensitive:      true,
		ConversationID: "conv-123",
	}

	note := service.statusToNote(status)

	assert.Equal(t, status.StatusID, note.ID)
	assert.Equal(t, status.AuthorID, note.AttributedTo)
	assert.Equal(t, status.Content, note.Content)
	assert.Equal(t, status.InReplyToID, note.InReplyTo)
	assert.Equal(t, status.Visibility, note.Visibility)
	assert.Equal(t, status.Sensitive, note.Sensitive)
	assert.Equal(t, status.ConversationID, note.ConversationID)
	assert.NotNil(t, note.Published)
	assert.Equal(t, now, *note.Published)
}

// Test ValidateNoteURL
func TestValidateNoteURL(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{
			name:    "Valid HTTPS URL",
			url:     "https://example.com/note/1",
			wantErr: false,
		},
		{
			name:    "Valid HTTP URL",
			url:     "http://example.com/note/1",
			wantErr: false,
		},
		{
			name:    "Empty URL",
			url:     "",
			wantErr: true,
		},
		{
			name:    "Invalid scheme",
			url:     "ftp://example.com/note/1",
			wantErr: true,
		},
		{
			name:    "Missing host",
			url:     "https:///note/1",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateNoteURL(tt.url)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// Test classifyFetchError
func Test_classifyFetchError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{
			name: "404 error",
			err:  errors.New("failed with 404 not found"),
			want: models.FailureReasonNotFound,
		},
		{
			name: "410 error",
			err:  errors.New("failed with 410 gone"),
			want: models.FailureReasonDeleted,
		},
		{
			name: "403 error",
			err:  errors.New("failed with 403 forbidden"),
			want: models.FailureReasonForbidden,
		},
		{
			name: "Timeout error",
			err:  errors.New("request timeout"),
			want: models.FailureReasonTimeout,
		},
		{
			name: "Connection error",
			err:  errors.New("connection refused"),
			want: models.FailureReasonUnreachable,
		},
		{
			name: "Unknown error",
			err:  errors.New("some other error"),
			want: models.FailureReasonInvalid,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := classifyFetchError(tt.err)
			assert.Equal(t, tt.want, result)
		})
	}
}

// Test SyncMissingReplies
func TestSyncMissingReplies(t *testing.T) {
	ctx := context.Background()
	threadRepo := &MockThreadRepository{}
	statusRepo := &MockStatusRepository{}
	objectRepo := &MockObjectRepository{}
	actorRepo := &MockActorRepository{}
	federation := &MockFederationClient{}
	publisher := &MockPublisher{}
	logger := zaptest.NewLogger(t)

	service := NewService(
		threadRepo,
		statusRepo,
		objectRepo,
		actorRepo,
		federation,
		publisher,
		logger,
		"example.com",
	)

	rootNote := createTestNote("https://example.com/note/1", "")
	requestedNote := rootNote

	// Mock the repository calls
	objectRepo.On("GetObject", ctx, "https://example.com/note/1").Return(requestedNote, nil)

	// Mock missing replies
	missingReplies := []*models.MissingReply{}
	threadRepo.On("GetMissingReplies", ctx, rootNote.ID).Return(missingReplies, nil)

	cmd := SyncMissingRepliesCommand{
		NoteID:   "https://example.com/note/1",
		ViewerID: "test",
	}

	result, err := service.SyncMissingReplies(ctx, cmd)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.True(t, result.Success)
	assert.Equal(t, 0, result.SyncedReplies)

	objectRepo.AssertExpectations(t)
	threadRepo.AssertExpectations(t)
}

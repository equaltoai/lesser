package sync

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	lessertesting "github.com/equaltoai/lesser/pkg/testing"
	storagemocks "github.com/equaltoai/lesser/pkg/testing/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type stubFederationClient struct {
	fetchObjectFn  func(context.Context, string) (any, error)
	fetchRepliesFn func(context.Context, string) ([]*activitypub.Note, error)
	fetchContextFn func(context.Context, string) (*ThreadContext, error)
}

func (s *stubFederationClient) FetchObject(ctx context.Context, url string) (any, error) {
	if s.fetchObjectFn != nil {
		return s.fetchObjectFn(ctx, url)
	}
	return nil, nil
}

func (s *stubFederationClient) FetchReplies(ctx context.Context, noteURL string) ([]*activitypub.Note, error) {
	if s.fetchRepliesFn != nil {
		return s.fetchRepliesFn(ctx, noteURL)
	}
	return nil, nil
}

func (s *stubFederationClient) FetchContext(ctx context.Context, noteURL string) (*ThreadContext, error) {
	if s.fetchContextFn != nil {
		return s.fetchContextFn(ctx, noteURL)
	}
	return nil, nil
}

type memoryThreadCache struct {
	threads map[string]*Thread
}

func newMemoryThreadCache() *memoryThreadCache {
	return &memoryThreadCache{threads: make(map[string]*Thread)}
}

func (m *memoryThreadCache) GetThread(_ context.Context, conversationID string) (*Thread, error) {
	return m.threads[conversationID], nil
}

func (m *memoryThreadCache) SetThread(_ context.Context, conversationID string, thread *Thread, _ time.Duration) error {
	m.threads[conversationID] = thread
	return nil
}

type storageWithObjectRepo struct {
	*lessertesting.MockRepositoryStorage
	objectRepo interfaces.ObjectRepository
}

func (s *storageWithObjectRepo) Object() interfaces.ObjectRepository {
	return s.objectRepo
}

func TestThreadSyncer_SyncThread_UsesFreshCache(t *testing.T) {
	store := &storageWithObjectRepo{
		MockRepositoryStorage: lessertesting.NewMockRepositoryStorage(),
		objectRepo:            storagemocks.NewMockObjectRepository(),
	}
	cache := newMemoryThreadCache()
	cache.threads["c1"] = &Thread{ConversationID: "c1", LastUpdated: time.Now()}

	fed := &stubFederationClient{
		fetchContextFn: func(context.Context, string) (*ThreadContext, error) {
			t.Fatal("FetchContext should not be called when cache is fresh")
			return nil, nil
		},
	}

	syncer := NewThreadSyncer(store, fed, cache)
	err := syncer.SyncThread(context.Background(), ThreadSyncRequest{ConversationID: "c1", OriginServer: "example.com", Depth: 1})
	require.NoError(t, err)
}

func TestThreadSyncer_SyncThread_FetchContextError(t *testing.T) {
	store := &storageWithObjectRepo{
		MockRepositoryStorage: lessertesting.NewMockRepositoryStorage(),
		objectRepo:            storagemocks.NewMockObjectRepository(),
	}
	cache := newMemoryThreadCache()

	fed := &stubFederationClient{
		fetchContextFn: func(context.Context, string) (*ThreadContext, error) {
			return nil, errors.New("down")
		},
	}

	syncer := NewThreadSyncer(store, fed, cache)
	err := syncer.SyncThread(context.Background(), ThreadSyncRequest{ConversationID: "c1", OriginServer: "example.com", Depth: 1, ForceRefresh: true})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrFetchThreadContext)
}

func TestThreadSyncer_SyncThread_FetchRootError(t *testing.T) {
	store := &storageWithObjectRepo{
		MockRepositoryStorage: lessertesting.NewMockRepositoryStorage(),
		objectRepo:            storagemocks.NewMockObjectRepository(),
	}
	cache := newMemoryThreadCache()

	fed := &stubFederationClient{
		fetchContextFn: func(context.Context, string) (*ThreadContext, error) {
			return &ThreadContext{ConversationID: "c1", RootURL: "https://example.com/root"}, nil
		},
		fetchObjectFn: func(context.Context, string) (any, error) {
			return nil, errors.New("down")
		},
	}

	syncer := NewThreadSyncer(store, fed, cache)
	err := syncer.SyncThread(context.Background(), ThreadSyncRequest{ConversationID: "c1", OriginServer: "example.com", Depth: 1, ForceRefresh: true})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrFetchRootNote)
}

func TestThreadSyncer_SyncThread_InvalidRootObject(t *testing.T) {
	store := &storageWithObjectRepo{
		MockRepositoryStorage: lessertesting.NewMockRepositoryStorage(),
		objectRepo:            storagemocks.NewMockObjectRepository(),
	}
	cache := newMemoryThreadCache()

	fed := &stubFederationClient{
		fetchContextFn: func(context.Context, string) (*ThreadContext, error) {
			return &ThreadContext{ConversationID: "c1", RootURL: "https://example.com/root"}, nil
		},
		fetchObjectFn: func(context.Context, string) (any, error) {
			return "not-a-note", nil
		},
	}

	syncer := NewThreadSyncer(store, fed, cache)
	err := syncer.SyncThread(context.Background(), ThreadSyncRequest{ConversationID: "c1", OriginServer: "example.com", Depth: 1, ForceRefresh: true})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidRootObject)
}

func TestThreadSyncer_SyncThread_FetchRepliesRecursive(t *testing.T) {
	objectRepo := storagemocks.NewMockObjectRepository()
	objectRepo.On("CreateObject", mock.Anything, mock.Anything).Return(nil).Maybe()

	store := &storageWithObjectRepo{
		MockRepositoryStorage: lessertesting.NewMockRepositoryStorage(),
		objectRepo:            objectRepo,
	}
	cache := newMemoryThreadCache()

	root := &activitypub.Note{BaseObject: activitypub.BaseObject{ID: "https://origin.example/users/alice/statuses/1"}, ConversationID: "c1"}
	reply1 := &activitypub.Note{BaseObject: activitypub.BaseObject{ID: "https://origin.example/users/bob/statuses/2", InReplyTo: root.ID}}
	reply2 := &activitypub.Note{BaseObject: activitypub.BaseObject{ID: "https://origin.example/users/cat/statuses/3", InReplyTo: reply1.ID}}

	fed := &stubFederationClient{
		fetchContextFn: func(context.Context, string) (*ThreadContext, error) {
			return &ThreadContext{ConversationID: "c1", RootURL: root.ID, Participants: []string{"alice", "bob"}}, nil
		},
		fetchObjectFn: func(context.Context, string) (any, error) {
			return root, nil
		},
		fetchRepliesFn: func(_ context.Context, noteURL string) ([]*activitypub.Note, error) {
			switch noteURL {
			case root.ID:
				return []*activitypub.Note{reply1}, nil
			case reply1.ID:
				return []*activitypub.Note{reply2}, nil
			default:
				return nil, nil
			}
		},
	}

	syncer := NewThreadSyncer(store, fed, cache)
	err := syncer.SyncThread(context.Background(), ThreadSyncRequest{
		ConversationID: "c1",
		OriginServer:   "origin.example",
		Depth:          2,
		IncludeReplies: true,
		ForceRefresh:   true,
	})
	require.NoError(t, err)

	cached, err := cache.GetThread(context.Background(), "c1")
	require.NoError(t, err)
	require.NotNil(t, cached)
	assert.Equal(t, 3, cached.TotalPosts)
	assert.Len(t, cached.Replies, 2)
}

func TestThreadSyncer_SyncMissingContext_ConversationIDTriggersSync(t *testing.T) {
	objectRepo := storagemocks.NewMockObjectRepository()

	store := &storageWithObjectRepo{
		MockRepositoryStorage: lessertesting.NewMockRepositoryStorage(),
		objectRepo:            objectRepo,
	}
	cache := newMemoryThreadCache()

	noteID := "https://example.com/users/alice/statuses/1"
	note := &activitypub.Note{BaseObject: activitypub.BaseObject{ID: noteID}, ConversationID: "c1"}
	objectRepo.On("GetObject", mock.Anything, noteID).Return(note, nil).Once()
	objectRepo.On("CreateObject", mock.Anything, mock.Anything).Return(nil).Maybe()

	fedCalls := 0
	fed := &stubFederationClient{
		fetchContextFn: func(context.Context, string) (*ThreadContext, error) {
			fedCalls++
			return &ThreadContext{ConversationID: "c1", RootURL: noteID}, nil
		},
		fetchObjectFn: func(context.Context, string) (any, error) {
			return note, nil
		},
	}

	syncer := NewThreadSyncer(store, fed, cache)
	err := syncer.SyncMissingContext(context.Background(), noteID)
	require.NoError(t, err)
	assert.Equal(t, 1, fedCalls)
}

func TestThreadSyncer_SyncMissingContext_FetchesParent(t *testing.T) {
	objectRepo := storagemocks.NewMockObjectRepository()
	objectRepo.On("CreateObject", mock.Anything, mock.Anything).Return(nil).Maybe()

	store := &storageWithObjectRepo{
		MockRepositoryStorage: lessertesting.NewMockRepositoryStorage(),
		objectRepo:            objectRepo,
	}
	cache := newMemoryThreadCache()

	noteID := "https://example.com/users/alice/statuses/1"
	parentID := "https://origin.example/users/bob/statuses/2"
	note := &activitypub.Note{BaseObject: activitypub.BaseObject{ID: noteID, InReplyTo: parentID}}
	objectRepo.On("GetObject", mock.Anything, noteID).Return(note, nil).Once()

	parent := &activitypub.Note{BaseObject: activitypub.BaseObject{ID: parentID}}
	fed := &stubFederationClient{
		fetchObjectFn: func(context.Context, string) (any, error) {
			return parent, nil
		},
	}

	syncer := NewThreadSyncer(store, fed, cache)
	err := syncer.SyncMissingContext(context.Background(), noteID)
	require.NoError(t, err)

	objectRepo.AssertCalled(t, "CreateObject", mock.Anything, parent)
}

func TestThreadSyncer_fetchRepliesRecursive_EarlyReturns(t *testing.T) {
	objectRepo := storagemocks.NewMockObjectRepository()
	store := &storageWithObjectRepo{
		MockRepositoryStorage: lessertesting.NewMockRepositoryStorage(),
		objectRepo:            objectRepo,
	}

	syncer := NewThreadSyncer(store, &stubFederationClient{}, newMemoryThreadCache())

	replies, missing := syncer.fetchRepliesRecursive(context.Background(), "https://example.com/note/1", 0, map[string]bool{})
	assert.Nil(t, replies)
	assert.Nil(t, missing)

	replies, missing = syncer.fetchRepliesRecursive(context.Background(), "https://example.com/note/1", 1, map[string]bool{"https://example.com/note/1": true})
	assert.Nil(t, replies)
	assert.Nil(t, missing)
}

func TestThreadSyncer_fetchRepliesRecursive_FetchError(t *testing.T) {
	objectRepo := storagemocks.NewMockObjectRepository()
	store := &storageWithObjectRepo{
		MockRepositoryStorage: lessertesting.NewMockRepositoryStorage(),
		objectRepo:            objectRepo,
	}

	fed := &stubFederationClient{
		fetchRepliesFn: func(context.Context, string) ([]*activitypub.Note, error) {
			return nil, errors.New("down")
		},
	}

	syncer := NewThreadSyncer(store, fed, newMemoryThreadCache())
	replies, missing := syncer.fetchRepliesRecursive(context.Background(), "https://example.com/note/1", 2, map[string]bool{})
	assert.Nil(t, replies)
	assert.Nil(t, missing)
}

func TestThreadSyncer_fetchRepliesRecursive_StoreErrorAddsMissing(t *testing.T) {
	objectRepo := storagemocks.NewMockObjectRepository()

	reply := &activitypub.Note{BaseObject: activitypub.BaseObject{ID: "https://example.com/note/2"}}

	objectRepo.On("CreateObject", mock.Anything, reply).Return(errors.New("write failed")).Once()

	store := &storageWithObjectRepo{
		MockRepositoryStorage: lessertesting.NewMockRepositoryStorage(),
		objectRepo:            objectRepo,
	}

	fed := &stubFederationClient{
		fetchRepliesFn: func(context.Context, string) ([]*activitypub.Note, error) {
			return []*activitypub.Note{reply}, nil
		},
	}

	syncer := NewThreadSyncer(store, fed, newMemoryThreadCache())
	replies, missing := syncer.fetchRepliesRecursive(context.Background(), "https://example.com/note/1", 2, map[string]bool{})
	assert.Empty(t, replies)
	assert.Equal(t, []string{reply.ID}, missing)
}

func TestThreadSyncer_SyncMissingContext_ErrorBranches(t *testing.T) {
	t.Run("get_note_error", func(t *testing.T) {
		objectRepo := storagemocks.NewMockObjectRepository()
		objectRepo.On("GetObject", mock.Anything, "n1").Return(nil, errors.New("down")).Once()

		store := &storageWithObjectRepo{
			MockRepositoryStorage: lessertesting.NewMockRepositoryStorage(),
			objectRepo:            objectRepo,
		}

		syncer := NewThreadSyncer(store, &stubFederationClient{}, newMemoryThreadCache())
		err := syncer.SyncMissingContext(context.Background(), "n1")
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrGetNote)
	})

	t.Run("invalid_note_type", func(t *testing.T) {
		objectRepo := storagemocks.NewMockObjectRepository()
		objectRepo.On("GetObject", mock.Anything, "n1").Return("nope", nil).Once()

		store := &storageWithObjectRepo{
			MockRepositoryStorage: lessertesting.NewMockRepositoryStorage(),
			objectRepo:            objectRepo,
		}

		syncer := NewThreadSyncer(store, &stubFederationClient{}, newMemoryThreadCache())
		err := syncer.SyncMissingContext(context.Background(), "n1")
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrInvalidNoteType)
	})

	t.Run("fetch_parent_error", func(t *testing.T) {
		objectRepo := storagemocks.NewMockObjectRepository()

		noteID := "https://example.com/users/alice/statuses/1"
		parentID := "https://origin.example/users/bob/statuses/2"
		note := &activitypub.Note{BaseObject: activitypub.BaseObject{ID: noteID, InReplyTo: parentID}}
		objectRepo.On("GetObject", mock.Anything, noteID).Return(note, nil).Once()

		store := &storageWithObjectRepo{
			MockRepositoryStorage: lessertesting.NewMockRepositoryStorage(),
			objectRepo:            objectRepo,
		}

		fed := &stubFederationClient{
			fetchObjectFn: func(context.Context, string) (any, error) {
				return nil, errors.New("down")
			},
		}

		syncer := NewThreadSyncer(store, fed, newMemoryThreadCache())
		err := syncer.SyncMissingContext(context.Background(), noteID)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrFetchParent)
	})

	t.Run("store_parent_error", func(t *testing.T) {
		objectRepo := storagemocks.NewMockObjectRepository()

		noteID := "https://example.com/users/alice/statuses/1"
		parentID := "https://origin.example/users/bob/statuses/2"
		note := &activitypub.Note{BaseObject: activitypub.BaseObject{ID: noteID, InReplyTo: parentID}}
		objectRepo.On("GetObject", mock.Anything, noteID).Return(note, nil).Once()

		parent := &activitypub.Note{BaseObject: activitypub.BaseObject{ID: parentID}}
		objectRepo.On("CreateObject", mock.Anything, parent).Return(errors.New("write failed")).Once()

		store := &storageWithObjectRepo{
			MockRepositoryStorage: lessertesting.NewMockRepositoryStorage(),
			objectRepo:            objectRepo,
		}

		fed := &stubFederationClient{
			fetchObjectFn: func(context.Context, string) (any, error) {
				return parent, nil
			},
		}

		syncer := NewThreadSyncer(store, fed, newMemoryThreadCache())
		err := syncer.SyncMissingContext(context.Background(), noteID)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrStoreParentNote)
	})
}

func TestExtractDomain(t *testing.T) {
	assert.Equal(t, "example.com", extractDomain("https://example.com/users/alice/statuses/1"))
	assert.Equal(t, "", extractDomain("http://example.com/users/alice"))
	assert.Equal(t, "", extractDomain("not-a-url"))
}

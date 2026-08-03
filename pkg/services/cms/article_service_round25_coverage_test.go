package cms

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/cmsrender"
	"github.com/equaltoai/lesser/pkg/common"
	apperrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	dynamormcore "github.com/theory-cloud/tabletheory/v3/pkg/core"
	dynamormerrors "github.com/theory-cloud/tabletheory/v3/pkg/errors"
	dynamormMocks "github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	"go.uber.org/zap"
)

type fakeArticleRepo struct {
	db dynamormcore.DB

	articles map[string]*models.Article

	createErr error
	updateErr error
	deleteErr error

	deletedIDs []string
}

func (f *fakeArticleRepo) GetDB() dynamormcore.DB { return f.db }

func (f *fakeArticleRepo) CreateArticle(_ context.Context, article *models.Article) error {
	if f.createErr != nil {
		return f.createErr
	}
	if f.articles == nil {
		f.articles = map[string]*models.Article{}
	}
	f.articles[article.ID] = article
	return nil
}

func (f *fakeArticleRepo) GetArticle(_ context.Context, articleID string) (*models.Article, error) {
	if f.articles == nil {
		return nil, apperrors.ItemNotFoundWithID("article", articleID)
	}
	a, ok := f.articles[articleID]
	if !ok {
		return nil, apperrors.ItemNotFoundWithID("article", articleID)
	}
	return a, nil
}

func (f *fakeArticleRepo) UpdateArticle(_ context.Context, article *models.Article) error {
	if f.updateErr != nil {
		return f.updateErr
	}
	if f.articles == nil {
		f.articles = map[string]*models.Article{}
	}
	f.articles[article.ID] = article
	return nil
}

func (f *fakeArticleRepo) DeleteArticle(_ context.Context, articleID string) error {
	f.deletedIDs = append(f.deletedIDs, articleID)
	if f.deleteErr != nil {
		return f.deleteErr
	}
	if f.articles != nil {
		delete(f.articles, articleID)
	}
	return nil
}

type fakeActorRepo struct {
	actor *activitypub.Actor
	err   error
}

func (f fakeActorRepo) GetActor(_ context.Context, _ string) (*activitypub.Actor, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.actor, nil
}

type fakeFederation struct {
	followerCalls  atomic.Int32
	recipientCalls atomic.Int32
	err            error

	mu                  sync.Mutex
	followerActivities  []*activitypub.Activity
	recipientActivities []*activitypub.Activity
}

func (f *fakeFederation) DeliverToFollowers(_ context.Context, activity *activitypub.Activity, _ *activitypub.Actor) error {
	f.followerCalls.Add(1)
	f.mu.Lock()
	f.followerActivities = append(f.followerActivities, activity)
	f.mu.Unlock()
	return f.err
}

func (f *fakeFederation) DeliverToRecipients(_ context.Context, activity *activitypub.Activity, _ *activitypub.Actor) error {
	f.recipientCalls.Add(1)
	f.mu.Lock()
	f.recipientActivities = append(f.recipientActivities, activity)
	f.mu.Unlock()
	return f.err
}

func (f *fakeFederation) recipientActivity(t *testing.T, index int) *activitypub.Activity {
	t.Helper()

	f.mu.Lock()
	defer f.mu.Unlock()

	require.Less(t, index, len(f.recipientActivities))
	return f.recipientActivities[index]
}

type fakeRevisionCreator struct {
	calls atomic.Int32
}

func (f *fakeRevisionCreator) CreateRevision(_ context.Context, _ *models.Article) (*models.Revision, error) {
	f.calls.Add(1)
	return &models.Revision{Version: 1}, nil
}

func TestArticleService_Round25_CreateUpdateDeleteArticle(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db, q := newCMSMockDB(t)

	repo := &fakeArticleRepo{
		db:       db,
		articles: map[string]*models.Article{},
	}

	svc := NewArticleService(
		repo,
		fakeActorRepo{err: errors.New("no actor")}, // keep federation goroutines harmless
		nil,
		nil,
		nil,
		&fakeFederation{},
		zap.NewNop(),
	)

	t.Run("create validates inputs", func(t *testing.T) {
		require.Error(t, svc.CreateArticle(ctx, nil))
		require.Error(t, svc.CreateArticle(ctx, &models.Article{Object: models.Object{ID: "a1"}}))
		require.Error(t, svc.CreateArticle(ctx, &models.Article{Object: models.Object{ID: ""}, Slug: "slug"}))
		err := svc.CreateArticle(ctx, &models.Article{
			Object: models.Object{
				ID:      "https://example.com/articles/too-large",
				Name:    "Too Large",
				Content: strings.Repeat("a", cmsrender.MaxArticleSourceBytes+1),
			},
			Slug:          "too-large",
			ContentFormat: "markdown",
		})
		require.ErrorIs(t, err, cmsrender.ErrArticleContentTooLarge)
	})

	t.Run("create blocks legacy slug collision", func(t *testing.T) {
		slug := "my-slug"
		legacy := common.GenerateObjectID("example.com", "articles", slug)
		repo.articles[legacy] = &models.Article{Object: models.Object{ID: legacy}, Slug: slug}

		db, q = newCMSMockDB(t)
		repo.db = db

		err := svc.CreateArticle(ctx, &models.Article{
			Object: models.Object{
				ID:           "https://example.com/articles/other",
				Name:         "n",
				Published:    time.Now(),
				AttributedTo: "https://example.com/users/alice",
			},
			Slug: slug,
		})
		require.Error(t, err)
		assert.True(t, apperrors.HasCode(err, apperrors.CodeAlreadyExists))
	})

	t.Run("create rolls back slug index when repository create fails", func(t *testing.T) {
		repo.createErr = errors.New("create failed")

		db, q = newCMSMockDB(t)
		repo.db = db

		q.On("Create").Return(nil).Once() // slug index
		q.On("Delete").Return(nil).Once() // slug rollback

		err := svc.CreateArticle(ctx, &models.Article{
			Object: models.Object{
				ID:           "a2",
				Name:         "n",
				Published:    time.Now(),
				AttributedTo: "https://example.com/users/alice",
			},
			Slug: "slug-2",
		})
		require.Error(t, err)
		repo.createErr = nil
	})

	t.Run("create rolls back when index upsert fails", func(t *testing.T) {
		db, q = newCMSMockDB(t)
		repo.db = db

		q.On("Create").Return(nil).Once()                      // slug index
		q.On("Create").Return(errors.New("index fail")).Once() // first CMS index entry
		q.On("Delete").Return(nil).Maybe()

		article := &models.Article{
			Object: models.Object{
				ID:           "a3",
				Name:         "n",
				Published:    time.Now(),
				AttributedTo: "https://example.com/users/alice",
			},
			Slug: "slug-3",
		}
		err := svc.CreateArticle(ctx, article)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "index fail")
		assert.Contains(t, repo.deletedIDs, "a3")
	})

	t.Run("get by slug handles missing index and empty target id", func(t *testing.T) {
		db, q := newCMSMockDB(t)
		repo.db = db

		_, err := svc.GetArticleBySlug(ctx, "")
		require.Error(t, err)

		q.On("First", mock.Anything).Return(dynamormerrors.ErrItemNotFound).Once()
		_, err = svc.GetArticleBySlug(ctx, "missing")
		require.Error(t, err)

		db, q = newCMSMockDB(t)
		repo.db = db
		q.On("First", mock.Anything).Return(nil).Once()
		_, err = svc.GetArticleBySlug(ctx, "empty-target")
		require.Error(t, err)
	})

	t.Run("get by slug resolves article id", func(t *testing.T) {
		db, q := newCMSMockDB(t)
		repo.db = db

		repo.articles["a4"] = &models.Article{Object: models.Object{ID: "a4", Name: "n"}, Slug: "s"}
		q.On("First", mock.Anything).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*models.CMSSlugIndex)
			dest.TargetID = "a4"
		}).Return(nil).Once()

		got, err := svc.GetArticleBySlug(ctx, "s")
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, "a4", got.ID)
	})

	t.Run("tenant slug lookup rejects legacy index target from other tenant", func(t *testing.T) {
		db, q := newCMSMockDB(t)
		repo.db = db

		crossTenantID := "https://other.example/articles/shared"
		repo.articles[crossTenantID] = &models.Article{Object: models.Object{ID: crossTenantID, Name: "n"}, Slug: "shared"}

		q.On("First", mock.Anything).Return(dynamormerrors.ErrItemNotFound).Once()
		q.On("First", mock.Anything).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*models.CMSSlugIndex)
			dest.TargetID = crossTenantID
		}).Return(nil).Once()

		_, err := svc.GetArticleByTenantSlug(ctx, "example.com", "shared")
		require.Error(t, err)
		assert.True(t, apperrors.HasCode(err, apperrors.CodeNotFound))
	})

	t.Run("update snapshots revision when configured", func(t *testing.T) {
		db, q := newCMSMockDB(t)
		repo.db = db
		q.On("Create").Return(nil).Maybe()
		q.On("Delete").Return(nil).Maybe()

		repo.articles["a5"] = &models.Article{Object: models.Object{ID: "a5", Published: time.Now(), AttributedTo: "https://example.com/users/alice"}, Slug: "slug"}
		revisions := &fakeRevisionCreator{}
		svcWithRevisions := NewArticleService(repo, fakeActorRepo{err: errors.New("no actor")}, nil, nil, revisions, &fakeFederation{}, zap.NewNop())

		err := svcWithRevisions.UpdateArticle(ctx, &models.Article{
			Object: models.Object{
				ID:           "a5",
				Published:    time.Now(),
				AttributedTo: "https://example.com/users/alice",
			},
			Slug: "slug",
		})
		require.NoError(t, err)
		assert.Equal(t, int32(1), revisions.calls.Load())
	})

	t.Run("delete removes article and indexes", func(t *testing.T) {
		db, q := newCMSMockDB(t)
		repo.db = db
		q.On("Create").Return(nil).Once()
		q.On("Delete").Return(nil).Maybe()

		repo.articles["a6"] = &models.Article{Object: models.Object{ID: "a6", Published: time.Now(), AttributedTo: "https://example.com/users/alice"}, Slug: "slug"}
		err := svc.DeleteArticle(ctx, &models.Article{Object: models.Object{ID: "a6", Published: time.Now(), AttributedTo: "https://example.com/users/alice"}, Slug: "slug"})
		require.NoError(t, err)
	})
}

func TestArticleService_Round25_federationHelpersAndUsernameExtraction(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db, q := newCMSMockDB(t)
	q.On("Create").Return(nil).Maybe()
	q.On("Delete").Return(nil).Maybe()
	q.On("First", mock.Anything).Return(nil).Maybe()

	actor := &activitypub.Actor{
		BaseObject: activitypub.BaseObject{ID: "https://example.com/users/alice", Type: activitypub.PersonType},
	}
	fed := &fakeFederation{}

	svc := NewArticleService(
		&fakeArticleRepo{db: db, articles: map[string]*models.Article{}},
		fakeActorRepo{actor: actor},
		nil,
		nil,
		nil,
		fed,
		zap.NewNop(),
	)

	article := &models.Article{
		Object: models.Object{
			ID:           "https://example.com/articles/1",
			Name:         "title",
			Content:      "<p>hello</p>",
			Published:    time.Now(),
			AttributedTo: "https://example.com/users/alice",
			To:           []string{activitypub.PublicAddress},
			CC:           []string{"https://example.com/users/alice/followers"},
		},
		Slug: "slug",
	}

	svc.federateArticleWriteActivity(ctx, article, activitypub.CreateType, "create")
	assert.GreaterOrEqual(t, fed.followerCalls.Load(), int32(1))
	assert.GreaterOrEqual(t, fed.recipientCalls.Load(), int32(1))

	privateFed := &fakeFederation{}
	svc.federation = privateFed
	privateArticle := &models.Article{
		Object: models.Object{
			ID:           "https://example.com/articles/2",
			Name:         "private title",
			Content:      "<p>secret</p>",
			Published:    time.Now(),
			AttributedTo: "https://example.com/users/alice",
			To:           []string{"https://example.com/users/alice/followers"},
		},
		Slug: "private-slug",
	}

	svc.federateArticleWriteActivity(ctx, privateArticle, activitypub.CreateType, "create")
	assert.Equal(t, int32(0), privateFed.followerCalls.Load())
	assert.Equal(t, int32(1), privateFed.recipientCalls.Load())

	fed.err = errors.New("deliver failed")
	svc.federateArticleDeletion(ctx, article)

	assert.Equal(t, "alice", extractUsernameFromActorID("https://example.com/users/alice"))
	assert.Equal(t, "", extractUsernameFromActorID(""))
	assert.Equal(t, "alice", extractUsernameFromActorID("alice"))
}

func TestArticleService_M2OutboundArticleFederationPayloads(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db, q := newCMSMockDB(t)
	q.On("Create").Return(nil).Maybe()
	q.On("Delete").Return(nil).Maybe()
	q.On("First", mock.Anything).Return(nil).Maybe()

	actor := &activitypub.Actor{
		BaseObject: activitypub.BaseObject{ID: "https://example.com/users/alice", Type: activitypub.PersonType},
	}
	fed := &fakeFederation{}
	svc := NewArticleService(
		&fakeArticleRepo{db: db, articles: map[string]*models.Article{}},
		fakeActorRepo{actor: actor},
		nil,
		nil,
		nil,
		fed,
		zap.NewNop(),
	)

	published := time.Date(2026, time.May, 19, 12, 0, 0, 0, time.UTC)
	publicArticle := &models.Article{
		Object: models.Object{
			ID:           "https://example.com/articles/federation-probe",
			Type:         activitypub.ArticleType,
			Name:         "Federation Probe",
			Summary:      "Protocol-level Article summary",
			Content:      "<p>hello article federation</p>",
			Published:    published,
			Updated:      published,
			AttributedTo: "https://example.com/users/alice",
			To:           []string{activitypub.PublicAddress},
			CC:           []string{"https://example.com/users/alice/followers"},
			BTo:          []string{"https://remote.example/users/hidden"},
			BCC:          []string{"https://remote.example/users/also-hidden"},
		},
		Slug: "federation-probe",
	}

	svc.federateArticleWriteActivity(ctx, publicArticle, activitypub.CreateType, "create")
	require.Equal(t, int32(1), fed.followerCalls.Load())
	require.Equal(t, int32(1), fed.recipientCalls.Load())
	assertArticleWriteActivity(t, fed.recipientActivity(t, 0), activitypub.CreateType, publicArticle.ID)

	updateFed := &fakeFederation{}
	svc.federation = updateFed
	updatedArticle := *publicArticle
	updatedArticle.Content = "<p>updated body, same Article identity</p>"
	svc.federateArticleWriteActivity(ctx, &updatedArticle, activitypub.UpdateType, "update")
	require.Equal(t, int32(1), updateFed.followerCalls.Load())
	require.Equal(t, int32(1), updateFed.recipientCalls.Load())
	assertArticleWriteActivity(t, updateFed.recipientActivity(t, 0), activitypub.UpdateType, publicArticle.ID)

	privateFed := &fakeFederation{}
	svc.federation = privateFed
	privateArticle := &models.Article{
		Object: models.Object{
			ID:           "https://example.com/articles/followers-only",
			Type:         activitypub.ArticleType,
			Name:         "Followers Only",
			Content:      "<p>private article</p>",
			Published:    published,
			Updated:      published,
			AttributedTo: "https://example.com/users/alice",
			To:           []string{"https://example.com/users/alice/followers"},
		},
		Slug: "followers-only",
	}

	svc.federateArticleWriteActivity(ctx, privateArticle, activitypub.CreateType, "create")
	require.Equal(t, int32(0), privateFed.followerCalls.Load())
	require.Equal(t, int32(1), privateFed.recipientCalls.Load())
	privateActivity := privateFed.recipientActivity(t, 0)
	assertArticleWriteActivity(t, privateActivity, activitypub.CreateType, privateArticle.ID)
	assert.NotContains(t, privateActivity.To, activitypub.PublicAddress)
	assert.NotContains(t, privateActivity.CC, activitypub.PublicAddress)
}

func TestArticleService_M2ArticleDeleteFederationAndTombstone(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db, q := newCMSMockDB(t)
	q.On("Delete").Return(nil).Maybe()
	q.On("First", mock.Anything).Return(nil).Maybe()

	actor := &activitypub.Actor{
		BaseObject: activitypub.BaseObject{ID: "https://example.com/users/alice", Type: activitypub.PersonType},
	}
	fed := &fakeFederation{}
	repo := &fakeArticleRepo{
		db:       db,
		articles: map[string]*models.Article{},
	}
	svc := NewArticleService(repo, fakeActorRepo{actor: actor}, nil, nil, nil, fed, zap.NewNop())

	published := time.Date(2026, time.May, 19, 12, 30, 0, 0, time.UTC)
	canonicalArticle := &models.Article{
		Object: models.Object{
			ID:           "https://example.com/articles/delete-me",
			Type:         activitypub.ArticleType,
			Name:         "Delete Me",
			Content:      "<p>delete me</p>",
			Published:    published,
			Updated:      published,
			AttributedTo: "https://example.com/users/alice",
			To:           []string{activitypub.PublicAddress},
			CC:           []string{"https://example.com/users/alice/followers"},
		},
		Slug: "delete-me",
	}

	svc.federateArticleDeletion(ctx, canonicalArticle)
	require.Equal(t, int32(1), fed.followerCalls.Load())
	require.Equal(t, int32(1), fed.recipientCalls.Load())
	deleteActivity := fed.recipientActivity(t, 0)
	require.Equal(t, activitypub.DeleteType, deleteActivity.Type)
	require.Equal(t, canonicalArticle.ID, deleteActivity.Object)
	require.Equal(t, canonicalArticle.To, deleteActivity.To)
	require.Equal(t, canonicalArticle.CC, deleteActivity.CC)

	legacyFed := &fakeFederation{}
	svc.federation = legacyFed
	legacyArticle := &models.Article{
		Object: models.Object{
			ID:           "https://example.com/objects/legacy-article-id",
			Type:         activitypub.ArticleType,
			Name:         "Legacy Article",
			Content:      "<p>legacy delete</p>",
			Published:    published,
			Updated:      published,
			AttributedTo: "https://example.com/users/alice",
			To:           []string{activitypub.PublicAddress},
		},
		Slug: "legacy-article",
	}

	svc.federateArticleDeletion(ctx, legacyArticle)
	require.Equal(t, int32(1), legacyFed.followerCalls.Load())
	require.Equal(t, int32(1), legacyFed.recipientCalls.Load())
	require.Equal(t, legacyArticle.ID, legacyFed.recipientActivity(t, 0).Object)

	tombstoneDB := new(dynamormMocks.MockDB)
	tombstoneQuery := new(dynamormMocks.MockQuery)
	tombstoneDB.On("WithContext", mock.Anything).Return(tombstoneDB).Maybe()
	var capturedTombstone *models.Tombstone
	tombstoneDB.On("Model", mock.MatchedBy(func(model any) bool {
		tombstone, ok := model.(*models.Tombstone)
		if ok {
			capturedTombstone = tombstone
		}
		return ok
	})).Return(tombstoneQuery).Once()
	tombstoneDB.On("Model", mock.Anything).Return(tombstoneQuery).Maybe()
	tombstoneQuery.On("Create").Return(nil).Once()
	tombstoneQuery.On("Delete").Return(nil).Maybe()

	tombstoneRepo := &fakeArticleRepo{
		db: tombstoneDB,
		articles: map[string]*models.Article{
			canonicalArticle.ID: canonicalArticle,
		},
	}
	tombstoneSvc := NewArticleService(tombstoneRepo, fakeActorRepo{err: errors.New("no actor")}, nil, nil, nil, &fakeFederation{}, zap.NewNop())
	require.NoError(t, tombstoneSvc.DeleteArticle(ctx, canonicalArticle))
	require.NotNil(t, capturedTombstone)
	require.Equal(t, canonicalArticle.ID, capturedTombstone.ID)
	require.Equal(t, activitypub.ArticleType, capturedTombstone.FormerType)
	require.Equal(t, "https://example.com/users/alice", capturedTombstone.DeletedBy)
	require.Equal(t, "https://example.com/users/alice", capturedTombstone.AttributedTo)
	require.True(t, capturedTombstone.IsPublic)
	require.Equal(t, "Tombstone", capturedTombstone.Type)
	require.Equal(t, "OBJECT#"+canonicalArticle.ID, capturedTombstone.PK)
	require.Equal(t, "TOMBSTONE", capturedTombstone.SK)
}

func TestArticleService_DeleteArticle_UsesTransactionalTombstoneWhenAvailable(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := new(dynamormMocks.MockExtendedDB)
	q := new(dynamormMocks.MockQuery)
	builder := new(dynamormMocks.MockTransactionBuilder)
	db.TransactWriteBuilder = builder

	db.On("WithContext", mock.Anything).Return(db).Maybe()
	db.On("Model", mock.Anything).Return(q).Maybe()
	q.On("Delete").Return(nil).Maybe()

	var capturedTombstone *models.Tombstone
	builder.On("Create", mock.MatchedBy(func(model any) bool {
		tombstone, ok := model.(*models.Tombstone)
		if ok {
			capturedTombstone = tombstone
		}
		return ok
	}), mock.Anything).Return(builder).Once()

	var capturedDelete *models.Article
	builder.On("Delete", mock.MatchedBy(func(model any) bool {
		article, ok := model.(*models.Article)
		if ok {
			capturedDelete = article
		}
		return ok
	}), mock.Anything).Return(builder).Once()
	builder.On("Execute").Return(nil).Once()
	db.On("TransactWrite", mock.Anything, mock.Anything).Return(nil).Once()

	article := &models.Article{
		Object: models.Object{
			ID:           "https://example.com/articles/tx-delete",
			Type:         activitypub.ArticleType,
			Name:         "Transactional Delete",
			Published:    time.Date(2026, time.May, 20, 0, 0, 0, 0, time.UTC),
			AttributedTo: "https://example.com/users/alice",
			To:           []string{activitypub.PublicAddress},
		},
		Slug: "tx-delete",
	}
	repo := &fakeArticleRepo{
		db: db,
		articles: map[string]*models.Article{
			article.ID: article,
		},
	}
	svc := NewArticleService(repo, fakeActorRepo{err: errors.New("no actor")}, nil, nil, nil, &fakeFederation{}, zap.NewNop())

	require.NoError(t, svc.DeleteArticle(ctx, article))
	require.NotNil(t, capturedTombstone)
	require.Equal(t, article.ID, capturedTombstone.ID)
	require.Equal(t, "OBJECT#"+article.ID, capturedTombstone.PK)
	require.Equal(t, "TOMBSTONE", capturedTombstone.SK)
	require.Equal(t, "Tombstone", capturedTombstone.Type)
	require.Equal(t, activitypub.ArticleType, capturedTombstone.FormerType)
	require.NotZero(t, capturedTombstone.TTL)
	require.NotNil(t, capturedDelete)
	require.Equal(t, "object#"+article.ID, capturedDelete.PK)
	require.Equal(t, "object#"+article.ID, capturedDelete.SK)
	require.Empty(t, repo.deletedIDs)

	builder.AssertExpectations(t)
	db.AssertExpectations(t)
}

func TestArticleService_DeleteArticle_TombstoneErrorBranches(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	article := &models.Article{
		Object: models.Object{
			ID:           "https://example.com/articles/tombstone-errors",
			Published:    time.Date(2026, time.May, 20, 1, 0, 0, 0, time.UTC),
			AttributedTo: "https://example.com/users/alice",
		},
		Slug: "tombstone-errors",
	}

	t.Run("requires db for tombstone persistence", func(t *testing.T) {
		repo := &fakeArticleRepo{
			articles: map[string]*models.Article{
				article.ID: article,
			},
		}
		svc := NewArticleService(repo, fakeActorRepo{err: errors.New("no actor")}, nil, nil, nil, &fakeFederation{}, zap.NewNop())

		err := svc.DeleteArticle(ctx, article)
		require.Error(t, err)
		require.Contains(t, err.Error(), "article repository db is required")
		require.Empty(t, repo.deletedIDs)
	})

	t.Run("returns transaction failure", func(t *testing.T) {
		db := new(dynamormMocks.MockExtendedDB)
		db.On("TransactWrite", mock.Anything, mock.Anything).Return(errors.New("tx failed")).Once()

		repo := &fakeArticleRepo{
			db: db,
			articles: map[string]*models.Article{
				article.ID: article,
			},
		}
		svc := NewArticleService(repo, fakeActorRepo{err: errors.New("no actor")}, nil, nil, nil, &fakeFederation{}, zap.NewNop())

		err := svc.DeleteArticle(ctx, article)
		require.Error(t, err)
		require.Contains(t, err.Error(), "tx failed")
		require.Empty(t, repo.deletedIDs)
		db.AssertExpectations(t)
	})

	t.Run("build defaults missing former type and actor", func(t *testing.T) {
		db, _ := newCMSMockDB(t)
		svc := NewArticleService(&fakeArticleRepo{db: db}, fakeActorRepo{}, nil, nil, nil, &fakeFederation{}, zap.NewNop())

		tombstone, err := svc.buildArticleTombstone(&models.Article{
			Object: models.Object{ID: "https://example.com/articles/default-type"},
		})
		require.NoError(t, err)
		require.Equal(t, activitypub.ArticleType, tombstone.FormerType)
		require.Empty(t, tombstone.DeletedBy)
		require.Equal(t, "Article was deleted", tombstone.Summary)
		require.Equal(t, "Tombstone", tombstone.Type)
		require.Equal(t, "OBJECT#https://example.com/articles/default-type", tombstone.PK)
		require.Equal(t, "TOMBSTONE", tombstone.SK)
		require.NotZero(t, tombstone.TTL)
	})

	t.Run("persist requires tombstone", func(t *testing.T) {
		db, _ := newCMSMockDB(t)
		svc := NewArticleService(&fakeArticleRepo{db: db}, fakeActorRepo{}, nil, nil, nil, &fakeFederation{}, zap.NewNop())

		err := svc.persistArticleTombstone(ctx, nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), "article tombstone is required")
	})
}

func TestArticleService_BuildArticleTombstonePinsPublicAddressingPredicate(t *testing.T) {
	t.Parallel()

	svc := NewArticleService(&fakeArticleRepo{}, fakeActorRepo{}, nil, nil, nil, &fakeFederation{}, zap.NewNop())
	tests := []struct {
		name     string
		to       []string
		cc       []string
		isPublic bool
	}{
		{name: "legacy empty addressing defaults public", isPublic: true},
		{name: "canonical public in to", to: []string{activitypub.PublicAddress}, isPublic: true},
		{name: "canonical public in cc", cc: []string{"https://example.com/users/alice/followers", activitypub.PublicAddress}, isPublic: true},
		{name: "compact public in to", to: []string{"as:Public"}, isPublic: true},
		{name: "bare public in cc", cc: []string{" Public "}, isPublic: true},
		{name: "case variant is not public", to: []string{"public"}, isPublic: false},
		{name: "empty recipient is not public", to: []string{""}, isPublic: false},
		{name: "private recipients", to: []string{"https://example.com/users/bob"}},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tombstone, err := svc.buildArticleTombstone(&models.Article{Object: models.Object{
				ID:           "https://example.com/articles/addressed",
				Type:         activitypub.ArticleType,
				AttributedTo: "https://example.com/users/alice",
				To:           tt.to,
				CC:           tt.cc,
			}})
			require.NoError(t, err)
			require.Equal(t, tt.isPublic, tombstone.IsPublic)
		})
	}
}

func assertArticleWriteActivity(t *testing.T, activity *activitypub.Activity, activityType string, articleID string) {
	t.Helper()

	require.NotNil(t, activity)
	require.Equal(t, activityType, activity.Type)
	require.Equal(t, "https://example.com/users/alice", activity.Actor)
	require.NotEmpty(t, activity.ID)
	require.NotNil(t, activity.Published)
	require.Empty(t, activity.BTo)
	require.Empty(t, activity.BCC)

	article, ok := activity.Object.(*activitypub.Article)
	require.True(t, ok, "expected *activitypub.Article, got %T", activity.Object)
	require.Equal(t, activitypub.ArticleType, article.Type)
	require.Equal(t, articleID, article.ID)
	require.Equal(t, "https://example.com/users/alice", article.AttributedTo)
	require.NotEmpty(t, article.Name)
	require.Empty(t, article.BTo)
	require.Empty(t, article.BCC)

	raw, err := json.Marshal(activity)
	require.NoError(t, err)
	var body map[string]any
	require.NoError(t, json.Unmarshal(raw, &body))
	require.NotContains(t, body, "bto")
	require.NotContains(t, body, "bcc")
	obj, ok := body["object"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, activitypub.ArticleType, obj["type"])
	require.Equal(t, articleID, obj["id"])
	require.NotContains(t, obj, "bto")
	require.NotContains(t, obj, "bcc")
}

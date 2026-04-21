package cms

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
	apperrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	dynamormcore "github.com/theory-cloud/tabletheory/pkg/core"
	dynamormerrors "github.com/theory-cloud/tabletheory/pkg/errors"
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
}

func (f *fakeFederation) DeliverToFollowers(_ context.Context, _ *activitypub.Activity, _ *activitypub.Actor) error {
	f.followerCalls.Add(1)
	return f.err
}

func (f *fakeFederation) DeliverToRecipients(_ context.Context, _ *activitypub.Activity, _ *activitypub.Actor) error {
	f.recipientCalls.Add(1)
	return f.err
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
